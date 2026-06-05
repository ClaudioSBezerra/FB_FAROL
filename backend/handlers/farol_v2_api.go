package handlers

// farol_v2_api.go — API de cards do Farol 2026 (granularidade diária).
//
// GET /api/v2/farol/cards
//   Parâmetros (nova API — intervalos):
//     view         V01 | V02 | V03
//     fluxo        faturado | transmitido       (default: faturado)
//     ref_inicio   YYYY-MM-DD                   (período principal)
//     ref_fim      YYYY-MM-DD
//     comp_inicio  YYYY-MM-DD                   (comparativo — opcional)
//     comp_fim     YYYY-MM-DD
//     drill        JSON: [{"level":"cod_fornec","value":"001","label":"MARCA X"}]
//
//   Retrocompat (UI antiga ainda enviando):
//     ref_ano + ref_mes                         → converte para ref_inicio/ref_fim
//     comp_mode = yoy | mom | ytd               → deriva comp_inicio/comp_fim
//     comp_ano + comp_mes                       → idem (mom override)
//
// GET /api/v2/farol/periodos — lista de meses (YYYY-MM) com dados disponíveis
//
// Dados lidos de views materializadas pré-agregadas (migrations 158 + 159).
// 28 MVs no total: 14 para vendas_faturadas (mv_fat_*) + 14 para
// vendas_transmitidas (mv_trans_*). A API só faz SELECT/WHERE — sem GROUP BY
// pesado, apenas SUM dos totais já pré-calculados.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

// ─── Definição de hierarquias e mapeamento de views ──────────────────────────

type hierLevel struct {
	Level     string
	NameField string
	Label     string
}

var hierarquias = map[string][]hierLevel{
	// V01: visão por indústria — Fornecedor → Gerente → Supervisor → RCA → Cliente → Produto
	"V01": {
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_gerente", NameField: "nome_gerente", Label: "Gerente"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	"V02": {
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	"V03": {
		{Level: "cod_gerente", NameField: "nome_gerente", Label: "Gerência"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
}

// Mapas de MVs por fluxo + view + drillIdx → nome da MV de leitura.
// Cada lista é indexada pelo drillIdx atual (0 = root, N = penúltimo nível).
// O último nível (produto) lê direto da tabela base (vendas_faturadas/transmitidas).
var viewPorNivelFat = map[string][]string{
	"V01": {"mv_fat_v01_l0", "mv_fat_v01_l1", "mv_fat_v01_l2", "mv_fat_v01_l3", "mv_fat_cli"},
	"V02": {"mv_fat_v02_l0", "mv_fat_v02_l1", "mv_fat_v02_l2", "mv_fat_cli"},
	"V03": {"mv_fat_v03_l0", "mv_fat_v03_l1", "mv_fat_v03_l2", "mv_fat_v03_l3"},
}

var viewPorNivelTrans = map[string][]string{
	"V01": {"mv_trans_v01_l0", "mv_trans_v01_l1", "mv_trans_v01_l2", "mv_trans_v01_l3", "mv_trans_cli"},
	"V02": {"mv_trans_v02_l0", "mv_trans_v02_l1", "mv_trans_v02_l2", "mv_trans_cli"},
	"V03": {"mv_trans_v03_l0", "mv_trans_v03_l1", "mv_trans_v03_l2", "mv_trans_v03_l3"},
}

// AllFatViews / AllTransViews — 14 views por fluxo, em ordem de REFRESH
// (base primeiro). O importador faz REFRESH dos dois fluxos em paralelo.
var AllFatViews = []string{
	"farol.mv_fat_cli",
	"farol.mv_fat_v01_l0", "farol.mv_fat_v01_l1", "farol.mv_fat_v01_l2", "farol.mv_fat_v01_l3",
	"farol.mv_fat_v02_l0", "farol.mv_fat_v02_l1", "farol.mv_fat_v02_l2",
	"farol.mv_fat_v03_l0", "farol.mv_fat_v03_l1", "farol.mv_fat_v03_l2", "farol.mv_fat_v03_l3",
	"farol.mv_fat_mkt_produto",
	"farol.mv_fat_mkt_prod_pen",
}

var AllTransViews = []string{
	"farol.mv_trans_cli",
	"farol.mv_trans_v01_l0", "farol.mv_trans_v01_l1", "farol.mv_trans_v01_l2", "farol.mv_trans_v01_l3",
	"farol.mv_trans_v02_l0", "farol.mv_trans_v02_l1", "farol.mv_trans_v02_l2",
	"farol.mv_trans_v03_l0", "farol.mv_trans_v03_l1", "farol.mv_trans_v03_l2", "farol.mv_trans_v03_l3",
	"farol.mv_trans_mkt_produto",
	"farol.mv_trans_mkt_prod_pen",
}

var AllSummaryViews = append(append([]string{}, AllFatViews...), AllTransViews...)

// fluxoCtx encapsula a escolha "faturado vs transmitido" e os artefatos derivados:
//   tableName  → vendas_faturadas | vendas_transmitidas
//   dateCol    → data_faturamento | data_transmissao
//   viewPorNivel → mapa pra escolher a MV correta no drill
//   baseView   → MV de cliente (mv_fat_cli ou mv_trans_cli)
type fluxoCtx struct {
	name         string
	tableName    string
	dateCol      string
	viewPorNivel map[string][]string
	baseView     string
}

func resolveFluxo(s string) fluxoCtx {
	if strings.EqualFold(s, "transmitido") || strings.EqualFold(s, "trans") {
		return fluxoCtx{
			name:         "transmitido",
			tableName:    "vendas_transmitidas",
			dateCol:      "data_transmissao",
			viewPorNivel: viewPorNivelTrans,
			baseView:     "farol.mv_trans_cli",
		}
	}
	return fluxoCtx{
		name:         "faturado",
		tableName:    "vendas_faturadas",
		dateCol:      "data_faturamento",
		viewPorNivel: viewPorNivelFat,
		baseView:     "farol.mv_fat_cli",
	}
}

func getViewName(fluxo fluxoCtx, view string, drillIdx int) string {
	if levels, ok := fluxo.viewPorNivel[view]; ok && drillIdx >= 0 && drillIdx < len(levels) {
		return "farol." + levels[drillIdx]
	}
	return fluxo.baseView
}

// ─── Tipos ────────────────────────────────────────────────────────────────────

type drillStep struct {
	Level string `json:"level"`
	Value string `json:"value"`
	Label string `json:"label"`
}

type cardItem struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Level       string  `json:"level"`
	LevelLabel  string  `json:"level_label"`
	ValorAtual  float64 `json:"valor_atual"`
	ValorAnt    float64 `json:"valor_ant"`
	Pct         float64 `json:"pct"`
	Cor         string  `json:"cor"` // cor do KPI Venda
	Faturado    float64 `json:"faturado"`
	Transmitido float64 `json:"transmitido"`
	Plucro      float64 `json:"plucro"`
	PlucroAnt   float64 `json:"plucro_ant"`
	// Positivação — atual e comparativo + cor
	Positivados    int     `json:"positivados"`
	BaseCli        int     `json:"base_cli"`
	PositPct       float64 `json:"positpct"`
	PositivadosAnt int     `json:"positivados_ant"`
	BaseCliAnt     int     `json:"base_cli_ant"`
	PositPctAnt    float64 `json:"positpct_ant"`
	PositCor       string  `json:"posit_cor"`
	// Mix — atual e comparativo + cor
	Mix    float64 `json:"mix"`
	MixAnt float64 `json:"mix_ant"`
	MixCor string  `json:"mix_cor"`
}

type kpiSummary struct {
	// Venda
	TotalAtual       float64 `json:"total_atual"`
	TotalAnt         float64 `json:"total_ant"`
	TotalPct         float64 `json:"total_pct"`
	TotalCor         string  `json:"total_cor"`
	TotalFaturado    float64 `json:"total_faturado"`
	TotalTransmitido float64 `json:"total_transmitido"`
	TotalPlucro      float64 `json:"total_plucro"`
	TotalPlucroAnt   float64 `json:"total_plucro_ant"`
	// Positivação — atual + comparativo + cor
	TotalPositivados    int     `json:"total_positivados"`
	TotalBaseCli        int     `json:"total_base_cli"`
	TotalPositPct       float64 `json:"total_positpct"`
	TotalPositivadosAnt int     `json:"total_positivados_ant"`
	TotalBaseCliAnt     int     `json:"total_base_cli_ant"`
	TotalPositPctAnt    float64 `json:"total_positpct_ant"`
	TotalPositCor       string  `json:"total_posit_cor"`
	// Mix
	AvgMix    float64 `json:"avg_mix"`
	AvgMixAnt float64 `json:"avg_mix_ant"`
	MixCor    string  `json:"mix_cor"`
	Verdes    int     `json:"verdes"`
	Vermelhos int     `json:"vermelhos"`
}

type periodoInfo struct {
	Fluxo      string `json:"fluxo"`       // faturado | transmitido
	RefInicio  string `json:"ref_inicio"`  // YYYY-MM-DD
	RefFim     string `json:"ref_fim"`     // YYYY-MM-DD
	CompInicio string `json:"comp_inicio"` // YYYY-MM-DD (vazio se sem comparativo)
	CompFim    string `json:"comp_fim"`    // YYYY-MM-DD
	Label      string `json:"label"`
	CurLabel   string `json:"cur_label"`
	AntLabel   string `json:"ant_label"`
	// Retrocompat — preenchidos quando inferidos a partir de mês inteiro
	RefAno   int    `json:"ref_ano,omitempty"`
	RefMes   int    `json:"ref_mes,omitempty"`
	CompMode string `json:"comp_mode,omitempty"`
	CompAno  int    `json:"comp_ano,omitempty"`
	CompMes  int    `json:"comp_mes,omitempty"`
}

type cardsResponse struct {
	Cards          []cardItem  `json:"cards"`
	KPI            kpiSummary  `json:"kpi"`
	Periodo        periodoInfo `json:"periodo"`
	Periodos       []string    `json:"periodos"`
	View           string      `json:"view"`
	DrillPath      []drillStep `json:"drill_path"`
	NextLevel      string      `json:"next_level"`
	NextLevelLabel string      `json:"next_level_label"`
}

// ─── Parsing de datas/períodos a partir da URL ───────────────────────────────

// parseDateISO aceita YYYY-MM-DD. Retorna zero se vazio/inválido.
func parseDateISO(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// mesInteiro retorna (inicio, fim) cobrindo todo um mês (1º dia até último dia).
func mesInteiro(ano, mes int) (time.Time, time.Time) {
	inicio := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
	fim := inicio.AddDate(0, 1, -1) // último dia do mês
	return inicio, fim
}

// inferLastMonth descobre o último mês com dados importados (ATUAL) para a empresa.
// Lê de vendas_import_jobs (tabela pequena). Retorna (ano, mes) ou (0, 0) se vazio.
func inferLastMonth(db *sql.DB, empresaID string) (int, int) {
	var ano, mes int
	_ = db.QueryRow(`
		SELECT ano, mes FROM vendas_import_jobs
		 WHERE empresa_id=$1 AND status='done'
		 ORDER BY ano DESC, mes DESC LIMIT 1
	`, empresaID).Scan(&ano, &mes)
	return ano, mes
}

// deriveCompRange calcula um intervalo comparativo a partir de (refInicio, refFim)
// e do compMode (yoy | mom | ytd). Retorna (zero, zero) se mode for desconhecido.
//   yoy → subtrai 1 ano nas duas pontas
//   mom → range contíguo imediatamente anterior (mesma quantidade de dias)
//   ytd → 1º jan do ano anterior até a mesma data (refFim com ano-1)
func deriveCompRange(refInicio, refFim time.Time, mode string) (time.Time, time.Time) {
	switch strings.ToLower(mode) {
	case "yoy":
		return refInicio.AddDate(-1, 0, 0), refFim.AddDate(-1, 0, 0)
	case "mom":
		diasRange := int(refFim.Sub(refInicio).Hours()/24) + 1
		fim := refInicio.AddDate(0, 0, -1)
		ini := fim.AddDate(0, 0, -(diasRange - 1))
		return ini, fim
	case "ytd":
		ini := time.Date(refFim.Year()-1, 1, 1, 0, 0, 0, 0, time.UTC)
		fim := time.Date(refFim.Year()-1, refFim.Month(), refFim.Day(), 0, 0, 0, 0, time.UTC)
		return ini, fim
	}
	return time.Time{}, time.Time{}
}

// resolvePeriods extrai o intervalo principal e o comparativo da query,
// honrando tanto o contrato novo (datas) quanto o antigo (ano/mes/comp_mode).
// Também retorna metadados pra preencher periodoInfo (ano/mes/compMode quando aplicáveis).
type periodResolution struct {
	RefInicio  time.Time
	RefFim     time.Time
	CompInicio time.Time
	CompFim    time.Time
	// Metadados pra retrocompat no payload
	RefAno   int
	RefMes   int
	CompMode string
	CompAno  int
	CompMes  int
}

func resolvePeriods(db *sql.DB, empresaID string, q map[string][]string) periodResolution {
	get := func(k string) string {
		if vs, ok := q[k]; ok && len(vs) > 0 {
			return strings.TrimSpace(vs[0])
		}
		return ""
	}
	res := periodResolution{}

	// 1) Período principal — preferência: datas explícitas; fallback ano/mes; senão último mês
	refInicio := parseDateISO(get("ref_inicio"))
	refFim := parseDateISO(get("ref_fim"))
	if refInicio.IsZero() || refFim.IsZero() {
		refAno, _ := strconv.Atoi(get("ref_ano"))
		refMes, _ := strconv.Atoi(get("ref_mes"))
		if refAno == 0 || refMes == 0 {
			refAno, refMes = inferLastMonth(db, empresaID)
		}
		if refAno > 0 && refMes > 0 {
			refInicio, refFim = mesInteiro(refAno, refMes)
			res.RefAno = refAno
			res.RefMes = refMes
		}
	} else {
		// Se as datas cobrem um mês inteiro, preencher ano/mes pra retrocompat.
		if refInicio.Day() == 1 && refFim.AddDate(0, 0, 1).Day() == 1 &&
			refInicio.Year() == refFim.Year() && refInicio.Month() == refFim.Month() {
			res.RefAno = refInicio.Year()
			res.RefMes = int(refInicio.Month())
		}
	}
	res.RefInicio = refInicio
	res.RefFim = refFim

	// 2) Comparativo — preferência: datas explícitas; depois comp_mode; depois nada
	compInicio := parseDateISO(get("comp_inicio"))
	compFim := parseDateISO(get("comp_fim"))
	if (compInicio.IsZero() || compFim.IsZero()) && !refInicio.IsZero() && !refFim.IsZero() {
		mode := strings.ToLower(get("comp_mode"))
		// Suporte ao comp_ano/comp_mes (override mom) — produz um mês exato
		compAno, _ := strconv.Atoi(get("comp_ano"))
		compMes, _ := strconv.Atoi(get("comp_mes"))
		if compAno > 0 && compMes > 0 {
			compInicio, compFim = mesInteiro(compAno, compMes)
			res.CompMode = "mom"
			res.CompAno = compAno
			res.CompMes = compMes
		} else if mode != "" {
			compInicio, compFim = deriveCompRange(refInicio, refFim, mode)
			res.CompMode = mode
		}
	}
	res.CompInicio = compInicio
	res.CompFim = compFim
	return res
}

// ─── FarolV2CardsHandler — GET /api/v2/farol/cards ──────────────────────────

func FarolV2CardsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()

		view := strings.ToUpper(q.Get("view"))
		if view == "" {
			view = "V01"
		}
		hier, ok := hierarquias[view]
		if !ok {
			http.Error(w, `{"error":"view inválida — use V01, V02 ou V03"}`, http.StatusBadRequest)
			return
		}

		fluxo := resolveFluxo(q.Get("fluxo"))

		var drillPath []drillStep
		if drillJSON := q.Get("drill"); drillJSON != "" {
			_ = json.Unmarshal([]byte(drillJSON), &drillPath)
		}
		drillIdx := len(drillPath)

		if drillIdx >= len(hier) {
			json.NewEncoder(w).Encode(cardsResponse{
				Cards: []cardItem{}, DrillPath: drillPath, View: view,
			})
			return
		}
		currentLevel := hier[drillIdx]

		pr := resolvePeriods(db, spCtx.EmpresaID, q)
		if pr.RefInicio.IsZero() || pr.RefFim.IsZero() {
			// Sem dados — devolve resposta vazia em vez de erro pra UI poder render placeholder.
			json.NewEncoder(w).Encode(cardsResponse{
				Cards: []cardItem{}, View: view, DrillPath: drillPath,
				Periodo: periodoInfo{Fluxo: fluxo.name},
			})
			return
		}

		filters := parseMultiFilters(q)
		cards := fetchCards(db, spCtx.EmpresaID, fluxo, view, pr, drillIdx, currentLevel, drillPath, filters)
		kpi := computeKPI(cards, fluxo.name)
		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		curLabel, antLabel, plabel := buildPeriodoLabels(pr)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].Pct < cards[j].Pct
		})

		json.NewEncoder(w).Encode(cardsResponse{
			Cards: cards,
			KPI:   kpi,
			Periodo: periodoInfo{
				Fluxo:      fluxo.name,
				RefInicio:  pr.RefInicio.Format("2006-01-02"),
				RefFim:     pr.RefFim.Format("2006-01-02"),
				CompInicio: fmtDateOrEmpty(pr.CompInicio),
				CompFim:    fmtDateOrEmpty(pr.CompFim),
				Label:      plabel,
				CurLabel:   curLabel,
				AntLabel:   antLabel,
				RefAno:     pr.RefAno,
				RefMes:     pr.RefMes,
				CompMode:   pr.CompMode,
				CompAno:    pr.CompAno,
				CompMes:    pr.CompMes,
			},
			Periodos:       periodos,
			View:           view,
			DrillPath:      drillPath,
			NextLevel:      currentLevel.Level,
			NextLevelLabel: currentLevel.Label,
		})
	}
}

func fmtDateOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// ─── safeColName ─────────────────────────────────────────────────────────────

var allowedCols = map[string]bool{
	"cod_fornec": true, "nome_fornec": true,
	"cod_gerente": true, "nome_gerente": true,
	"cod_supervisor": true, "nome_supervisor": true,
	"cod_rca": true, "nome_rca": true,
	"cod_cli": true, "nome_cli": true,
	"cod_prod": true, "nome_prod": true,
	"empresa": true, "uf": true,
}

func safeColName(col string) string {
	if allowedCols[col] {
		return col
	}
	return "cod_fornec"
}

// ─── Builders de condição SQL ─────────────────────────────────────────────────

// buildRangeCond monta `v.<dateCol> BETWEEN $X AND $Y`. Apenda args.
func buildRangeCond(dateCol string, inicio, fim time.Time, args *[]any) string {
	*args = append(*args, inicio.Format("2006-01-02"), fim.Format("2006-01-02"))
	n := len(*args)
	return fmt.Sprintf("v.%s BETWEEN $%d::date AND $%d::date", dateCol, n-1, n)
}

// buildDrillCond monta os filtros de drill-path (AND v.col=$N ...).
func buildDrillCond(drillPath []drillStep, args *[]any) string {
	parts := make([]string, 0, len(drillPath))
	for _, d := range drillPath {
		col := safeColName(d.Level)
		*args = append(*args, d.Value)
		parts = append(parts, fmt.Sprintf("AND v.%s=$%d", col, len(*args)))
	}
	return strings.Join(parts, " ")
}

// multiFilters representa filtros multi-select extraídos da query string.
// Cada chave é um cod_* (allowed col), cada valor é a lista de seleções.
type multiFilters map[string][]string

// parseMultiFilters extrai dos URL params os filtros multi-select.
// Aceita:
//   ?cod_fornec=F01,F02  ?cod_supervisor=S01  ?cod_rca=R01,R02
//   ?cod_gerente=...     ?cod_cli=...         ?uf=SP,RJ  ?empresa=NORDESTE
func parseMultiFilters(q map[string][]string) multiFilters {
	mf := multiFilters{}
	cols := []string{"cod_fornec", "cod_gerente", "cod_supervisor", "cod_rca", "cod_cli", "uf", "empresa"}
	for _, c := range cols {
		raw := ""
		if vs, ok := q[c]; ok && len(vs) > 0 {
			raw = vs[0]
		}
		if raw == "" {
			continue
		}
		// permite múltiplos valores separados por vírgula
		parts := strings.Split(raw, ",")
		vals := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				vals = append(vals, p)
			}
		}
		if len(vals) > 0 {
			mf[c] = vals
		}
	}
	return mf
}

// buildMultiFilterCond — gera `AND v.col = ANY($N::text[])` por dimensão.
// É aditivo ao drill: filtros multi e drill são aplicados juntos (AND).
func buildMultiFilterCond(mf multiFilters, args *[]any) string {
	if len(mf) == 0 {
		return ""
	}
	parts := []string{}
	for col, vals := range mf {
		col = safeColName(col)
		*args = append(*args, pq.Array(vals))
		parts = append(parts, fmt.Sprintf("AND v.%s = ANY($%d::text[])", col, len(*args)))
	}
	return strings.Join(parts, " ")
}

// ─── aggResult ────────────────────────────────────────────────────────────────

type aggResult struct {
	label       string
	valor       float64
	plucro      float64
	baseCli     int
	positivados int
	mix         float64
}

// ─── queryAggregated ─────────────────────────────────────────────────────────
// Lê a MV pré-agregada para um intervalo de datas e um nível de hierarquia.

func queryAggregated(db *sql.DB, viewName, groupCol, nameCol, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.%s                           AS key,
  MAX(v.%s)                      AS label,
  SUM(v.pvenda)                  AS valor,
  COALESCE(SUM(v.plucro), 0)     AS plucro,
  ROUND(AVG(v.base_cli))::int    AS base_cli,
  ROUND(AVG(v.positivados))::int AS positivados,
  AVG(v.mix)                     AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.%s != ''
AND %s %s
GROUP BY v.%s`,
		groupCol, nameCol,
		viewName,
		groupCol, rangeCond, drillCond,
		groupCol,
	)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryAggregated nível=%s ERRO em %v: %v", groupCol, time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryAggregated view=%s nível=%s → %d grupos em %v",
		viewName, groupCol, len(result), time.Since(t0))
	return result
}

// queryAnteriorTotals — comparativo completo (pvenda + positivação + mix + base).
// Mesma shape de queryAggregated pra permitir cor por KPI no fetchCards.
func queryAnteriorTotals(db *sql.DB, viewName, groupCol, nameCol, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.%s                           AS key,
  MAX(v.%s)                      AS label,
  SUM(v.pvenda)                  AS valor,
  COALESCE(SUM(v.plucro), 0)     AS plucro,
  ROUND(AVG(v.base_cli))::int    AS base_cli,
  ROUND(AVG(v.positivados))::int AS positivados,
  AVG(v.mix)                     AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.%s != '' AND %s %s
GROUP BY v.%s`, groupCol, nameCol, viewName, groupCol, rangeCond, drillCond, groupCol)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryAnteriorTotals nível=%s ERRO em %v: %v", groupCol, time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryAnteriorTotals view=%s nível=%s → %d grupos em %v",
		viewName, groupCol, len(result), time.Since(t0))
	return result
}

// queryProdutos / queryProdutosAnterior — nível folha (cod_prod), sem MV pré-agregada.
// Lê direto da tabela base do fluxo, escopado por drill (volume pequeno).

func queryProdutos(db *sql.DB, fluxo fluxoCtx, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.cod_prod                AS key,
  MAX(v.nome_prod)          AS label,
  SUM(v.pvenda)             AS valor,
  COALESCE(SUM(v.plucro),0) AS plucro,
  0::int                    AS base_cli,
  0::int                    AS positivados,
  0::float                  AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.cod_prod != '' AND %s %s
GROUP BY v.cod_prod`, fluxo.tableName, rangeCond, drillCond)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryProdutos ERRO em %v: %v", time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryProdutos (%s) → %d produtos em %v", fluxo.tableName, len(result), time.Since(t0))
	return result
}

func queryProdutosAnterior(db *sql.DB, fluxo fluxoCtx, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.cod_prod                AS key,
  MAX(v.nome_prod)          AS label,
  SUM(v.pvenda)             AS valor,
  COALESCE(SUM(v.plucro),0) AS plucro,
  0::int                    AS base_cli,
  0::int                    AS positivados,
  0::float                  AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.cod_prod != '' AND %s %s
GROUP BY v.cod_prod`, fluxo.tableName, rangeCond, drillCond)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryProdutosAnterior ERRO em %v: %v", time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			result[key] = r
		}
	}
	return result
}

// ─── fetchCards ───────────────────────────────────────────────────────────────

func fetchCards(db *sql.DB, empresaID string, fluxo fluxoCtx, view string,
	pr periodResolution, drillIdx int, level hierLevel, drillPath []drillStep,
	filters multiFilters) []cardItem {

	t0 := time.Now()
	groupCol := safeColName(level.Level)
	nameCol := safeColName(level.NameField)
	viewName := getViewName(fluxo, view, drillIdx)

	log.Printf("[farol:view] fetchCards empresa=%s fluxo=%s view=%s nível=%s ref=[%s..%s] comp=[%s..%s] drill=%d filters=%d",
		empresaID, fluxo.name, viewName, groupCol,
		pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"),
		fmtDateOrEmpty(pr.CompInicio), fmtDateOrEmpty(pr.CompFim),
		len(drillPath), len(filters))

	// Bucket "atual" (período principal)
	atualArgs := []any{empresaID}
	atualCond := buildRangeCond(fluxo.dateCol, pr.RefInicio, pr.RefFim, &atualArgs)
	drillCond := buildDrillCond(drillPath, &atualArgs)
	filterCond := buildMultiFilterCond(filters, &atualArgs)
	if filterCond != "" {
		drillCond = drillCond + " " + filterCond
	}

	var atualMap, antMap map[string]aggResult
	hasComp := !pr.CompInicio.IsZero() && !pr.CompFim.IsZero()

	var wg sync.WaitGroup
	wg.Add(1)
	if groupCol == "cod_prod" {
		go func() { defer wg.Done(); atualMap = queryProdutos(db, fluxo, atualCond, drillCond, atualArgs) }()
	} else {
		go func() {
			defer wg.Done()
			atualMap = queryAggregated(db, viewName, groupCol, nameCol, atualCond, drillCond, atualArgs)
		}()
	}

	if hasComp {
		antArgs := []any{empresaID}
		antCond := buildRangeCond(fluxo.dateCol, pr.CompInicio, pr.CompFim, &antArgs)
		antDrill := buildDrillCond(drillPath, &antArgs)
		antFilterCond := buildMultiFilterCond(filters, &antArgs)
		if antFilterCond != "" {
			antDrill = antDrill + " " + antFilterCond
		}
		wg.Add(1)
		if groupCol == "cod_prod" {
			go func() { defer wg.Done(); antMap = queryProdutosAnterior(db, fluxo, antCond, antDrill, antArgs) }()
		} else {
			go func() {
				defer wg.Done()
				antMap = queryAnteriorTotals(db, viewName, groupCol, nameCol, antCond, antDrill, antArgs)
			}()
		}
	}
	wg.Wait()

	// Cor binária: verde se atingiu ≥ 100% do anterior, vermelho caso contrário.
	// Sem comparativo, considera neutro (verde — sem alerta).
	pickCor := func(atual, ant float64) (float64, string) {
		if !hasComp {
			return 0, "verde"
		}
		var pct float64
		if ant > 0 {
			pct = atual / ant * 100
		} else if atual > 0 {
			pct = 100
		}
		if pct >= 100 {
			return pct, "verde"
		}
		return pct, "vermelho"
	}

	seen := make(map[string]bool, len(atualMap))
	cards := make([]cardItem, 0, len(atualMap)+len(antMap))

	for key, r := range atualMap {
		seen[key] = true
		ant := antMap[key] // aggResult zero se não existir

		// Venda
		pct, cor := pickCor(r.valor, ant.valor)

		// Positivação — % de positivados sobre base de clientes ativos
		positPct := 0.0
		if r.baseCli > 0 {
			positPct = float64(r.positivados) / float64(r.baseCli) * 100
		}
		positPctAnt := 0.0
		if ant.baseCli > 0 {
			positPctAnt = float64(ant.positivados) / float64(ant.baseCli) * 100
		}
		_, positCor := pickCor(positPct, positPctAnt)

		// Mix médio
		_, mixCor := pickCor(r.mix, ant.mix)

		card := cardItem{
			Key: key, Label: r.label,
			Level: level.Level, LevelLabel: level.Label,
			ValorAtual: r.valor, ValorAnt: ant.valor,
			Pct: pct, Cor: cor,
			Plucro: r.plucro, PlucroAnt: ant.plucro,
			Positivados: r.positivados, BaseCli: r.baseCli, PositPct: positPct,
			PositivadosAnt: ant.positivados, BaseCliAnt: ant.baseCli, PositPctAnt: positPctAnt,
			PositCor: positCor,
			Mix:      r.mix, MixAnt: ant.mix, MixCor: mixCor,
		}
		if fluxo.name == "transmitido" {
			card.Transmitido = r.valor
		} else {
			card.Faturado = r.valor
		}
		cards = append(cards, card)
	}

	// Grupos que existiam no comparativo mas zero no período principal → vermelho em tudo
	for key, ant := range antMap {
		if seen[key] || ant.valor == 0 {
			continue
		}
		positPctAnt := 0.0
		if ant.baseCli > 0 {
			positPctAnt = float64(ant.positivados) / float64(ant.baseCli) * 100
		}
		cards = append(cards, cardItem{
			Key: key, Label: ant.label,
			Level: level.Level, LevelLabel: level.Label,
			ValorAtual: 0, ValorAnt: ant.valor,
			Pct: 0, Cor: "vermelho",
			PlucroAnt:      ant.plucro,
			PositivadosAnt: ant.positivados, BaseCliAnt: ant.baseCli, PositPctAnt: positPctAnt,
			PositCor: "vermelho", MixAnt: ant.mix, MixCor: "vermelho",
		})
	}

	log.Printf("[farol:view] fetchCards fluxo=%s nível=%s → %d cards (atual=%d ant-only=%d) total=%v",
		fluxo.name, groupCol, len(cards), len(atualMap), len(cards)-len(atualMap), time.Since(t0))
	return cards
}

// ─── computeKPI ──────────────────────────────────────────────────────────────

// computeKPI agrega os totais dos cards. fluxoName não é mais usado (os valores
// Faturado/Transmitido já vêm preenchidos nos cards por fluxo) mas mantido na
// assinatura por simetria/legibilidade do call site.
func computeKPI(cards []cardItem, _ string) kpiSummary {
	var kpi kpiSummary
	var mixTotal, mixAntTotal float64
	mixCount, mixAntCount := 0, 0
	for _, c := range cards {
		kpi.TotalAtual += c.ValorAtual
		kpi.TotalAnt += c.ValorAnt
		kpi.TotalFaturado += c.Faturado
		kpi.TotalTransmitido += c.Transmitido
		kpi.TotalPlucro += c.Plucro
		kpi.TotalPlucroAnt += c.PlucroAnt
		kpi.TotalPositivados += c.Positivados
		kpi.TotalBaseCli += c.BaseCli
		kpi.TotalPositivadosAnt += c.PositivadosAnt
		kpi.TotalBaseCliAnt += c.BaseCliAnt
		if c.Mix > 0 {
			mixTotal += c.Mix
			mixCount++
		}
		if c.MixAnt > 0 {
			mixAntTotal += c.MixAnt
			mixAntCount++
		}
		if c.Cor == "verde" {
			kpi.Verdes++
		} else {
			kpi.Vermelhos++
		}
	}
	// Venda — % e cor
	if kpi.TotalAnt > 0 {
		kpi.TotalPct = kpi.TotalAtual / kpi.TotalAnt * 100
	} else if kpi.TotalAtual > 0 {
		kpi.TotalPct = 100
	}
	kpi.TotalCor = "vermelho"
	if kpi.TotalPct >= 100 {
		kpi.TotalCor = "verde"
	}
	// Positivação — % e cor (atual vs comparativo)
	if kpi.TotalBaseCli > 0 {
		kpi.TotalPositPct = float64(kpi.TotalPositivados) / float64(kpi.TotalBaseCli) * 100
	}
	if kpi.TotalBaseCliAnt > 0 {
		kpi.TotalPositPctAnt = float64(kpi.TotalPositivadosAnt) / float64(kpi.TotalBaseCliAnt) * 100
	}
	kpi.TotalPositCor = "vermelho"
	if kpi.TotalPositPct >= kpi.TotalPositPctAnt {
		kpi.TotalPositCor = "verde"
	}
	// Mix médio — atual + comparativo + cor
	if mixCount > 0 {
		kpi.AvgMix = mixTotal / float64(mixCount)
	}
	if mixAntCount > 0 {
		kpi.AvgMixAnt = mixAntTotal / float64(mixAntCount)
	}
	kpi.MixCor = "vermelho"
	if kpi.AvgMix >= kpi.AvgMixAnt {
		kpi.MixCor = "verde"
	}
	return kpi
}

// ─── fetchPeriodosDisponiveis ─────────────────────────────────────────────────

func fetchPeriodosDisponiveis(db *sql.DB, empresaID string) []string {
	rows, err := db.Query(`
		SELECT ano, mes FROM vendas_import_jobs
		WHERE empresa_id=$1 AND status='done'
		GROUP BY ano, mes
		ORDER BY ano DESC, mes DESC
	`, empresaID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var ano, mes int
		if rows.Scan(&ano, &mes) == nil {
			result = append(result, fmt.Sprintf("%04d-%02d", ano, mes))
		}
	}
	return result
}

// ─── buildPeriodoLabels ───────────────────────────────────────────────────────

func buildPeriodoLabels(pr periodResolution) (curLabel, antLabel, label string) {
	curLabel = fmtRangeBR(pr.RefInicio, pr.RefFim)
	if pr.CompInicio.IsZero() || pr.CompFim.IsZero() {
		label = curLabel
		return
	}
	antLabel = fmtRangeBR(pr.CompInicio, pr.CompFim)
	label = fmt.Sprintf("Anterior: %s × Atual: %s", antLabel, curLabel)
	return
}

// fmtRangeBR formata um intervalo em pt-BR de forma compacta:
//   01/05/2026 → 31/05/2026  →  "Mai/2026"   (mês inteiro)
//   05/05/2026 → 15/05/2026  →  "05/05/2026 – 15/05/2026"
//   01/01/2026 → 31/12/2026  →  "Ano 2026"   (ano inteiro)
func fmtRangeBR(ini, fim time.Time) string {
	if ini.IsZero() || fim.IsZero() {
		return ""
	}
	if ini.Year() == fim.Year() && ini.Month() == fim.Month() &&
		ini.Day() == 1 && fim.AddDate(0, 0, 1).Day() == 1 {
		return fmtMesAno(int(ini.Month()), ini.Year())
	}
	if ini.Year() == fim.Year() && ini.Month() == 1 && ini.Day() == 1 &&
		fim.Month() == 12 && fim.Day() == 31 {
		return fmt.Sprintf("Ano %d", ini.Year())
	}
	return fmt.Sprintf("%s – %s", ini.Format("02/01/2006"), fim.Format("02/01/2006"))
}

// ─── refreshAllFarolViews ─────────────────────────────────────────────────────

func refreshAllFarolViews(db *sql.DB) error {
	t0 := time.Now()

	// Refresh PRIMEIRO as MVs de carteira (1 linha por RCA) — base hierárquica
	// usada pelos sub-SELECTs nas derivadas v01_l0..l2, v02_l0 e v03_l0..l1.
	// São pequenas (~centenas de linhas), refresh rápido.
	for _, mv := range []string{"farol.mv_fat_carteira_rca", "farol.mv_trans_carteira_rca"} {
		if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY ` + mv); err != nil {
			// Sem CONCURRENTLY se nunca foi populada
			if _, err2 := db.Exec(`REFRESH MATERIALIZED VIEW ` + mv); err2 != nil {
				log.Printf("[farol:view] refresh %s ERRO: %v", mv, err2)
				return err2
			}
		}
		db.Exec(`ANALYZE ` + mv)
	}


	refreshFlow := func(views []string) []error {
		errs := make([]error, len(views))
		// base primeiro (índice 0)
		base := views[0]
		if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY ` + base); err != nil {
			log.Printf("[farol:view] refresh CONCURRENTLY %s falhou (%v), tentando sem CONCURRENTLY", base, err)
			if _, err2 := db.Exec(`REFRESH MATERIALIZED VIEW ` + base); err2 != nil {
				errs[0] = err2
				log.Printf("[farol:view] refresh %s ERRO: %v", base, err2)
				return errs
			}
		}
		db.Exec(`ANALYZE ` + base)

		// summary em paralelo
		var wg sync.WaitGroup
		for i := 1; i < len(views); i++ {
			wg.Add(1)
			go func(idx int, name string) {
				defer wg.Done()
				_, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY ` + name)
				if err != nil {
					_, err = db.Exec(`REFRESH MATERIALIZED VIEW ` + name)
				}
				if err != nil {
					errs[idx] = err
					log.Printf("[farol:view] refresh %s ERRO: %v", name, err)
				} else {
					db.Exec(`ANALYZE ` + name)
				}
			}(i, views[i])
		}
		wg.Wait()
		return errs
	}

	// Os dois fluxos podem rodar em paralelo (independentes)
	var fatErrs, transErrs []error
	var wgFlows sync.WaitGroup
	wgFlows.Add(2)
	go func() { defer wgFlows.Done(); fatErrs = refreshFlow(AllFatViews) }()
	go func() { defer wgFlows.Done(); transErrs = refreshFlow(AllTransViews) }()
	wgFlows.Wait()

	for _, e := range fatErrs {
		if e != nil {
			return e
		}
	}
	for _, e := range transErrs {
		if e != nil {
			return e
		}
	}
	log.Printf("[farol:view] refreshAllFarolViews concluído em %v", time.Since(t0))
	return nil
}

// ─── RefreshViewsHandler — POST /api/v2/farol/refresh-views ─────────────────

func RefreshViewsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		t0 := time.Now()
		log.Printf("[farol:view] RefreshViews início — empresa=%s user=%s", spCtx.EmpresaID, spCtx.UserID)

		if err := refreshAllFarolViews(db); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}

		var fatRows, transRows int
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.mv_fat_cli`).Scan(&fatRows)
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.mv_trans_cli`).Scan(&transRows)
		log.Printf("[farol:view] RefreshViews concluído — fat=%d trans=%d, total %v",
			fatRows, transRows, time.Since(t0))
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "fat_rows": fatRows, "trans_rows": transRows,
			"duration_ms": time.Since(t0).Milliseconds(),
		})
	}
}

// ─── FarolV2PeriodosHandler ──────────────────────────────────────────────────

func FarolV2PeriodosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		json.NewEncoder(w).Encode(map[string]any{"periodos": periodos})
	}
}

// ─── FarolV2DimsHandler — GET /api/v2/farol/dims ────────────────────────────
//
// Retorna as opções disponíveis em cada dimensão, dentro do período + fluxo
// escolhido. Alimenta os multi-selects da UI de filtros.
//
//   GET /api/v2/farol/dims?fluxo=faturado&ref_inicio=2026-05-01&ref_fim=2026-05-31
//
// Resposta:
//   {
//     "fornec":     [{"key":"F01","label":"NESTLE BRASIL"}, ...],
//     "gerente":    [...],
//     "supervisor": [...],
//     "rca":        [...],
//     "cli":        [...],
//     "uf":         ["SP", "RJ", ...],
//     "empresa":    ["NORDESTE", "SUDESTE", ...]
//   }

type dimOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

func FarolV2DimsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		fluxo := resolveFluxo(q.Get("fluxo"))
		pr := resolvePeriods(db, spCtx.EmpresaID, q)
		if pr.RefInicio.IsZero() {
			json.NewEncoder(w).Encode(map[string]any{})
			return
		}

		baseView := fluxo.baseView
		dateCol := fluxo.dateCol
		refIni := pr.RefInicio.Format("2006-01-02")
		refFim := pr.RefFim.Format("2006-01-02")
		t0 := time.Now()

		fetchDim := func(codCol, nameCol string) []dimOption {
			td := time.Now()
			rows, err := db.Query(fmt.Sprintf(`
				SELECT %s AS key, MAX(%s) AS label
				  FROM %s
				 WHERE empresa_id=$1
				   AND %s BETWEEN $2::date AND $3::date
				   AND %s != ''
				 GROUP BY %s
				 ORDER BY label
			`, codCol, nameCol, baseView, dateCol, codCol, codCol),
				spCtx.EmpresaID, refIni, refFim)
			if err != nil {
				log.Printf("[dims] %s ERRO em %v: %v", codCol, time.Since(td), err)
				return nil
			}
			defer rows.Close()
			out := []dimOption{}
			for rows.Next() {
				var d dimOption
				if rows.Scan(&d.Key, &d.Label) == nil {
					out = append(out, d)
				}
			}
			log.Printf("[dims] %s → %d opções em %v", codCol, len(out), time.Since(td))
			return out
		}

		fetchScalar := func(col string) []string {
			td := time.Now()
			rows, err := db.Query(fmt.Sprintf(`
				SELECT DISTINCT %s FROM %s
				 WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date AND %s != ''
				 ORDER BY %s
			`, col, baseView, dateCol, col, col),
				spCtx.EmpresaID, refIni, refFim)
			if err != nil {
				log.Printf("[dims] %s ERRO em %v: %v", col, time.Since(td), err)
				return nil
			}
			defer rows.Close()
			out := []string{}
			for rows.Next() {
				var v string
				if rows.Scan(&v) == nil {
					out = append(out, v)
				}
			}
			log.Printf("[dims] %s → %d valores em %v", col, len(out), time.Since(td))
			return out
		}

		// 7 GROUP BYs sobre mv_*_cli (a MV mais granular) — serial seria ~7×
		// o tempo de 1 query. Roda em paralelo: cada goroutine pega sua própria
		// *sql.Rows do pool. Limite prático = MaxOpenConns do db.
		var (
			fornec, gerente, supervisor, rca, cli []dimOption
			uf, empresa                           []string
			wg                                    sync.WaitGroup
		)
		wg.Add(7)
		go func() { defer wg.Done(); fornec = fetchDim("cod_fornec", "nome_fornec") }()
		go func() { defer wg.Done(); gerente = fetchDim("cod_gerente", "nome_gerente") }()
		go func() { defer wg.Done(); supervisor = fetchDim("cod_supervisor", "nome_supervisor") }()
		go func() { defer wg.Done(); rca = fetchDim("cod_rca", "nome_rca") }()
		go func() { defer wg.Done(); cli = fetchDim("cod_cli", "nome_cli") }()
		go func() { defer wg.Done(); uf = fetchScalar("uf") }()
		go func() { defer wg.Done(); empresa = fetchScalar("empresa") }()
		wg.Wait()

		log.Printf("[dims] fluxo=%s ref=[%s..%s] paralelo=7 total=%v",
			fluxo.name, refIni, refFim, time.Since(t0))

		resp := map[string]any{
			"fornec":     fornec,
			"gerente":    gerente,
			"supervisor": supervisor,
			"rca":        rca,
			"cli":        cli,
			"uf":         uf,
			"empresa":    empresa,
		}
		json.NewEncoder(w).Encode(resp)
	}
}

// ─── Acesso público ION VENDAS ───────────────────────────────────────────────

// resolveEmpresaCNPJ resolve empresa_id a partir do CNPJ (dígitos, com ou sem máscara).
func resolveEmpresaCNPJ(db *sql.DB, cnpj string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, cnpj)
	if digits == "" {
		return ""
	}
	var id string
	_ = db.QueryRow(
		`SELECT id FROM companies WHERE regexp_replace(cnpj, '[^0-9]', '', 'g') = $1 LIMIT 1`,
		digits,
	).Scan(&id)
	return id
}

// lookupNome busca o nome de um código na view base de FATURADO (a mais completa).
func lookupNome(db *sql.DB, empresaID, codCol, nomeCol, cod string) string {
	if cod == "" {
		return ""
	}
	codCol, nomeCol = safeColName(codCol), safeColName(nomeCol)
	var nome string
	q := fmt.Sprintf(
		`SELECT %s FROM farol.mv_fat_cli WHERE empresa_id=$1 AND %s=$2 AND %s!='' LIMIT 1`,
		nomeCol, codCol, nomeCol)
	_ = db.QueryRow(q, empresaID, cod).Scan(&nome)
	if nome == "" {
		// Fallback: tenta transmitido (caso o código só exista lá)
		q2 := fmt.Sprintf(
			`SELECT %s FROM farol.mv_trans_cli WHERE empresa_id=$1 AND %s=$2 AND %s!='' LIMIT 1`,
			nomeCol, codCol, nomeCol)
		_ = db.QueryRow(q2, empresaID, cod).Scan(&nome)
	}
	if nome == "" {
		nome = cod
	}
	return nome
}

// lookupParent descobre o código pai de um código (ex.: o supervisor de um RCA).
func lookupParent(db *sql.DB, empresaID, codCol, cod, parentCol string) string {
	codCol, parentCol = safeColName(codCol), safeColName(parentCol)
	var p string
	q := fmt.Sprintf(
		`SELECT %s FROM farol.mv_fat_cli WHERE empresa_id=$1 AND %s=$2 AND %s!='' LIMIT 1`,
		parentCol, codCol, parentCol)
	_ = db.QueryRow(q, empresaID, cod).Scan(&p)
	if p == "" {
		q2 := fmt.Sprintf(
			`SELECT %s FROM farol.mv_trans_cli WHERE empresa_id=$1 AND %s=$2 AND %s!='' LIMIT 1`,
			parentCol, codCol, parentCol)
		_ = db.QueryRow(q2, empresaID, cod).Scan(&p)
	}
	return p
}

// FarolV2PublicCardsHandler — GET /api/v2/farol/public/cards (SEM auth)
//   cnpj, scope (sup|rca), cod  → escopo fixo; drill adicional opcional.
//   fluxo, ref_inicio/ref_fim, comp_inicio/comp_fim (ou ano/mes legados).
func FarolV2PublicCardsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()

		empresaID := resolveEmpresaCNPJ(db, q.Get("cnpj"))
		if empresaID == "" {
			http.Error(w, `{"error":"empresa não encontrada para este CNPJ"}`, http.StatusNotFound)
			return
		}

		scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
		cod := strings.TrimSpace(q.Get("cod"))
		if cod == "" || (scope != "sup" && scope != "rca") {
			http.Error(w, `{"error":"scope (sup|rca) e cod obrigatórios"}`, http.StatusBadRequest)
			return
		}

		view := "V02"
		hier := hierarquias[view]
		fluxo := resolveFluxo(q.Get("fluxo"))

		// Drill base fixado pela URL (não pode ser removido pelo usuário).
		var baseDrill []drillStep
		switch scope {
		case "sup":
			baseDrill = []drillStep{
				{Level: "cod_supervisor", Value: cod, Label: lookupNome(db, empresaID, "cod_supervisor", "nome_supervisor", cod)},
			}
		case "rca":
			sup := lookupParent(db, empresaID, "cod_rca", cod, "cod_supervisor")
			baseDrill = []drillStep{
				{Level: "cod_supervisor", Value: sup, Label: lookupNome(db, empresaID, "cod_supervisor", "nome_supervisor", sup)},
				{Level: "cod_rca", Value: cod, Label: lookupNome(db, empresaID, "cod_rca", "nome_rca", cod)},
			}
		}

		var userDrill []drillStep
		if dj := q.Get("drill"); dj != "" {
			_ = json.Unmarshal([]byte(dj), &userDrill)
		}
		drillPath := append(baseDrill, userDrill...)
		drillIdx := len(drillPath)

		if drillIdx >= len(hier) {
			json.NewEncoder(w).Encode(cardsResponse{Cards: []cardItem{}, DrillPath: drillPath, View: view})
			return
		}
		currentLevel := hier[drillIdx]

		pr := resolvePeriods(db, empresaID, q)
		if pr.RefInicio.IsZero() {
			json.NewEncoder(w).Encode(cardsResponse{Cards: []cardItem{}, View: view, DrillPath: drillPath})
			return
		}

		filters := parseMultiFilters(q)
		cards := fetchCards(db, empresaID, fluxo, view, pr, drillIdx, currentLevel, drillPath, filters)
		kpi := computeKPI(cards, fluxo.name)
		curLabel, antLabel, plabel := buildPeriodoLabels(pr)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].Pct < cards[j].Pct
		})

		json.NewEncoder(w).Encode(cardsResponse{
			Cards: cards,
			KPI:   kpi,
			Periodo: periodoInfo{
				Fluxo:      fluxo.name,
				RefInicio:  pr.RefInicio.Format("2006-01-02"),
				RefFim:     pr.RefFim.Format("2006-01-02"),
				CompInicio: fmtDateOrEmpty(pr.CompInicio),
				CompFim:    fmtDateOrEmpty(pr.CompFim),
				Label:      plabel,
				CurLabel:   curLabel,
				AntLabel:   antLabel,
				RefAno:     pr.RefAno, RefMes: pr.RefMes,
				CompMode: pr.CompMode, CompAno: pr.CompAno, CompMes: pr.CompMes,
			},
			Periodos:       fetchPeriodosDisponiveis(db, empresaID),
			View:           view,
			DrillPath:      drillPath,
			NextLevel:      currentLevel.Level,
			NextLevelLabel: currentLevel.Label,
		})
	}
}
