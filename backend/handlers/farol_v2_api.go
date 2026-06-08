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
	// V04: visão por força de vendas — RCA → Fornecedor → Cliente → Produto
	// Usa exclusivamente tabelas agg_*_mes (migration 162); sem MVs diárias.
	"V04": {
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
}

// Tabelas agg_*_mes (granularidade mensal, migration 162+165).
// Usadas sempre — não há mais MVs diárias.
var aggTablesFat = map[string][]string{
	"V01": {"agg_fat_v01_l0_mes", "agg_fat_v01_l1_mes", "agg_fat_v01_l2_mes", "agg_fat_v01_l3_mes", "agg_fat_v01_l4_mes"},
	"V02": {"agg_fat_v02_l0_mes", "agg_fat_v02_l1_mes", "agg_fat_v02_l2_mes", "agg_fat_v02_l3_mes"},
	"V03": {"agg_fat_v03_l0_mes", "agg_fat_v03_l1_mes", "agg_fat_v03_l2_mes", "agg_fat_v03_l3_mes"},
	"V04": {"agg_fat_v04_l0_mes", "agg_fat_v04_l1_mes", "agg_fat_v04_l2_mes"},
}

var aggTablesTrans = map[string][]string{
	"V01": {"agg_trans_v01_l0_mes", "agg_trans_v01_l1_mes", "agg_trans_v01_l2_mes", "agg_trans_v01_l3_mes", "agg_trans_v01_l4_mes"},
	"V02": {"agg_trans_v02_l0_mes", "agg_trans_v02_l1_mes", "agg_trans_v02_l2_mes", "agg_trans_v02_l3_mes"},
	"V03": {"agg_trans_v03_l0_mes", "agg_trans_v03_l1_mes", "agg_trans_v03_l2_mes", "agg_trans_v03_l3_mes"},
	"V04": {"agg_trans_v04_l0_mes", "agg_trans_v04_l1_mes", "agg_trans_v04_l2_mes"},
}

// fluxoCtx — após mig 165 não há mais MVs diárias. tableName/dateCol seguem
// usados pelos handlers de detalhe (consulta direta em vendas_*).
type fluxoCtx struct {
	name      string
	tableName string
	dateCol   string
}

func resolveFluxo(s string) fluxoCtx {
	if strings.EqualFold(s, "transmitido") || strings.EqualFold(s, "trans") {
		return fluxoCtx{
			name:      "transmitido",
			tableName: "vendas_transmitidas",
			dateCol:   "data_transmissao",
		}
	}
	return fluxoCtx{
		name:      "faturado",
		tableName: "vendas_faturadas",
		dateCol:   "data_faturamento",
	}
}

// getAggTableName retorna a tabela agg_*_mes para (fluxo, view, drillIdx), ou ("", false).
func getAggTableName(fluxo fluxoCtx, view string, drillIdx int) (string, bool) {
	tables := aggTablesFat
	if fluxo.name == "transmitido" {
		tables = aggTablesTrans
	}
	if levels, ok := tables[view]; ok && drillIdx >= 0 && drillIdx < len(levels) {
		return "farol." + levels[drillIdx], true
	}
	return "", false
}

// isCompleteMonthRange reporta se [start, end] cobre apenas meses calendários completos.
func isCompleteMonthRange(start, end time.Time) bool {
	if start.IsZero() || end.IsZero() {
		return false
	}
	if start.Day() != 1 {
		return false
	}
	lastDay := time.Date(end.Year(), end.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	return end.Day() == lastDay.Day()
}

// ym converte uma data no inteiro ano*100+mes usado nas cláusulas WHERE das agg_*_mes.
func ym(t time.Time) int { return t.Year()*100 + int(t.Month()) }

// buildMesCond monta `(v.ano * 100 + v.mes) BETWEEN $N AND $M` para tabelas agg_*_mes.
func buildMesCond(ymStart, ymEnd int, args *[]any) string {
	*args = append(*args, ymStart, ymEnd)
	n := len(*args)
	return fmt.Sprintf("(v.ano * 100 + v.mes) BETWEEN $%d AND $%d", n-1, n)
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
			http.Error(w, `{"error":"view inválida — use V01, V02, V03 ou V04"}`, http.StatusBadRequest)
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
		kpi := computeKPI(cards, fluxo.name, currentLevel.Level == "cod_fornec")
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

// queryAggregatedMes — lê tabelas agg_*_mes (granularidade mensal, migration 162).
// Usado quando o range de datas é meses completos; substitui queryAggregated/queryAnteriorTotals.
// pvenda/plucro são somados; base_cli/positivados/mix são AVG (valor típico por mês).
func queryAggregatedMes(db *sql.DB, viewName, groupCol, nameCol, mesCond, drillCond string, args []any) map[string]aggResult {
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
		groupCol, mesCond, drillCond,
		groupCol,
	)
	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:agg] queryAggregatedMes view=%s nível=%s ERRO em %v: %v", viewName, groupCol, time.Since(t0), err)
		return nil
	}
	defer rows.Close()
	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro, &r.baseCli, &r.positivados, &r.mix); err == nil {
			result[key] = r
		}
	}
	log.Printf("[farol:agg] queryAggregatedMes view=%s nível=%s → %d grupos em %v",
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

	// Após mig 165: cod_prod lê direto de vendas_*; resto SEMPRE de agg_*_mes.
	// Range parcial dentro do mês expande pro mês inteiro (grão mensal).
	aggName, hasAgg := getAggTableName(fluxo, view, drillIdx)
	useAggMes := groupCol != "cod_prod" && hasAgg

	log.Printf("[farol:agg] fetchCards empresa=%s fluxo=%s view=%s nível=%s ref=[%s..%s] comp=[%s..%s] drill=%d filters=%d",
		empresaID, fluxo.name, aggName, groupCol,
		pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"),
		fmtDateOrEmpty(pr.CompInicio), fmtDateOrEmpty(pr.CompFim),
		len(drillPath), len(filters))

	// Path produto: usa vendas_* direto (buildRangeCond + drillCond por data)
	atualArgs := []any{empresaID}
	atualCond := buildRangeCond(fluxo.dateCol, pr.RefInicio, pr.RefFim, &atualArgs)
	drillCond := buildDrillCond(drillPath, &atualArgs)
	filterCond := buildMultiFilterCond(filters, &atualArgs)
	if filterCond != "" {
		drillCond = drillCond + " " + filterCond
	}

	// Path agg_mes: args ymStart/ymEnd em vez de datas
	var atualArgsMes []any
	var mesCond, drillCondMes string
	if useAggMes {
		atualArgsMes = []any{empresaID}
		mesCond = buildMesCond(ym(pr.RefInicio), ym(pr.RefFim), &atualArgsMes)
		drillCondMes = buildDrillCond(drillPath, &atualArgsMes)
		if fc := buildMultiFilterCond(filters, &atualArgsMes); fc != "" {
			drillCondMes = drillCondMes + " " + fc
		}
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
			atualMap = queryAggregatedMes(db, aggName, groupCol, nameCol, mesCond, drillCondMes, atualArgsMes)
		}()
	}

	if hasComp {
		antArgs := []any{empresaID}
		antCond := buildRangeCond(fluxo.dateCol, pr.CompInicio, pr.CompFim, &antArgs)
		antDrill := buildDrillCond(drillPath, &antArgs)
		if antFc := buildMultiFilterCond(filters, &antArgs); antFc != "" {
			antDrill = antDrill + " " + antFc
		}
		wg.Add(1)
		if groupCol == "cod_prod" {
			go func() { defer wg.Done(); antMap = queryProdutosAnterior(db, fluxo, antCond, antDrill, antArgs) }()
		} else {
			antArgsMes := []any{empresaID}
			antMesCond := buildMesCond(ym(pr.CompInicio), ym(pr.CompFim), &antArgsMes)
			antDrillMes := buildDrillCond(drillPath, &antArgsMes)
			if fc := buildMultiFilterCond(filters, &antArgsMes); fc != "" {
				antDrillMes = antDrillMes + " " + fc
			}
			go func() {
				defer wg.Done()
				antMap = queryAggregatedMes(db, aggName, groupCol, nameCol, antMesCond, antDrillMes, antArgsMes)
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

// computeKPI agrega os totais dos cards.
//
// overlappingBase deve ser true quando os cards agrupam por cod_fornec: nesse
// nível a base de clientes (base_cli) é a mesma para todos os cards (clientes
// da empresa/supervisor/RCA, independente de fornecedor). Somar base_cli
// multiplicaria a base pelo número de fornecedores — dupla contagem.
// Com overlappingBase=true usamos MAX(base_cli) e recalculamos positPct como
// média das taxas por card, recompondo positivados = positPct × base.
func computeKPI(cards []cardItem, _ string, overlappingBase bool) kpiSummary {
	var kpi kpiSummary
	var mixTotal, mixAntTotal float64
	mixCount, mixAntCount := 0, 0

	var positPctSum, positPctAntSum float64
	positCount, positAntCount := 0, 0

	for _, c := range cards {
		kpi.TotalAtual += c.ValorAtual
		kpi.TotalAnt += c.ValorAnt
		kpi.TotalFaturado += c.Faturado
		kpi.TotalTransmitido += c.Transmitido
		kpi.TotalPlucro += c.Plucro
		kpi.TotalPlucroAnt += c.PlucroAnt
		if overlappingBase {
			if c.BaseCli > kpi.TotalBaseCli {
				kpi.TotalBaseCli = c.BaseCli
			}
			if c.BaseCliAnt > kpi.TotalBaseCliAnt {
				kpi.TotalBaseCliAnt = c.BaseCliAnt
			}
			if c.BaseCli > 0 {
				positPctSum += c.PositPct
				positCount++
			}
			if c.BaseCliAnt > 0 {
				positPctAntSum += c.PositPctAnt
				positAntCount++
			}
		} else {
			kpi.TotalPositivados += c.Positivados
			kpi.TotalBaseCli += c.BaseCli
			kpi.TotalPositivadosAnt += c.PositivadosAnt
			kpi.TotalBaseCliAnt += c.BaseCliAnt
		}
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
	if overlappingBase {
		if positCount > 0 {
			kpi.TotalPositPct = positPctSum / float64(positCount)
		}
		if positAntCount > 0 {
			kpi.TotalPositPctAnt = positPctAntSum / float64(positAntCount)
		}
		// Reconstrói contagem absoluta a partir da média de % × base correta
		kpi.TotalPositivados = int(kpi.TotalPositPct/100*float64(kpi.TotalBaseCli) + 0.5)
		kpi.TotalPositivadosAnt = int(kpi.TotalPositPctAnt/100*float64(kpi.TotalBaseCliAnt) + 0.5)
	} else {
		if kpi.TotalBaseCli > 0 {
			kpi.TotalPositPct = float64(kpi.TotalPositivados) / float64(kpi.TotalBaseCli) * 100
		}
		if kpi.TotalBaseCliAnt > 0 {
			kpi.TotalPositPctAnt = float64(kpi.TotalPositivadosAnt) / float64(kpi.TotalBaseCliAnt) * 100
		}
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

// upsertAggMesYM (ano, mes) pequena struct usada na paralelização.
type aggMesYM struct{ Ano, Mes int }

// upsertAggsMesParallel chama farol.upsert_aggs_mes em N goroutines, uma por mês.
// Cada upsert_aggs_mes leva ~4min em 1M rows; rodar 4 em paralelo cai pra ~tempo/4
// no inicial. Carga diária toca 1 mês só — overhead de paralelismo é zero.
// workers=4 escolhido por equilibrar I/O do disco e CPU; pool DB tem 50 conexões.
func upsertAggsMesParallel(db *sql.DB, empresaID string, meses []aggMesYM, workers int) {
	if len(meses) == 0 {
		return
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(meses) {
		workers = len(meses)
	}

	jobs := make(chan aggMesYM, len(meses))
	for _, m := range meses {
		jobs <- m
	}
	close(jobs)

	var wg sync.WaitGroup
	wg.Add(workers)
	tStart := time.Now()
	for i := 0; i < workers; i++ {
		go func(wid int) {
			defer wg.Done()
			for m := range jobs {
				t1 := time.Now()
				if _, e := db.Exec(`SELECT farol.upsert_aggs_mes($1,$2,$3)`, empresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:agg] w=%d UPSERT %04d-%02d ERRO: %v", wid, m.Ano, m.Mes, e)
				} else {
					log.Printf("[farol:agg] w=%d UPSERT %04d-%02d OK em %v", wid, m.Ano, m.Mes, time.Since(t1))
				}
			}
		}(i)
	}
	wg.Wait()
	log.Printf("[farol:agg] upsertAggsMesParallel: %d meses, %d workers, total %v",
		len(meses), workers, time.Since(tStart))
}

// refreshAllFarolViews: após mig 165, só restam mv_*_carteira_rca como MVs
// (pequenas, refresh em ms). Tudo o mais é populado por upsert_aggs_mes
// chamado pelo handler RefreshViews depois desta função.
func refreshAllFarolViews(db *sql.DB) error {
	t0 := time.Now()

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

	log.Printf("[farol:view] refreshAllFarolViews (só carteiras) em %v", time.Since(t0))
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

		// Popula agg_*_mes para todos os meses presentes nos dados da empresa.
		rows, err := db.Query(`
			SELECT DISTINCT ano, mes FROM (
				SELECT EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
				       EXTRACT(MONTH FROM data_faturamento)::int AS mes
				  FROM vendas_faturadas WHERE empresa_id=$1
				UNION
				SELECT EXTRACT(YEAR  FROM data_transmissao)::int,
				       EXTRACT(MONTH FROM data_transmissao)::int
				  FROM vendas_transmitidas WHERE empresa_id=$1
			) t ORDER BY ano, mes`, spCtx.EmpresaID)
		if err == nil {
			var meses []aggMesYM
			for rows.Next() {
				var r aggMesYM
				if rows.Scan(&r.Ano, &r.Mes) == nil {
					meses = append(meses, r)
				}
			}
			rows.Close()
			anosVistos := map[int]bool{}
			for _, m := range meses {
				if !anosVistos[m.Ano] {
					db.Exec(`SELECT farol.create_agg_year_partitions($1)`, m.Ano)
					anosVistos[m.Ano] = true
				}
			}
			upsertAggsMesParallel(db, spCtx.EmpresaID, meses, 4)
		}

		var fatRows, transRows int
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.agg_fat_v01_l0_mes WHERE empresa_id=$1`, spCtx.EmpresaID).Scan(&fatRows)
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.agg_trans_v01_l0_mes WHERE empresa_id=$1`, spCtx.EmpresaID).Scan(&transRows)
		log.Printf("[farol:view] RefreshViews concluído — fat_agg=%d trans_agg=%d, total %v",
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

		// Lookup em agg_*_dims_mes (mig 165): consolidada por dim/key, usa label do mês mais recente.
		dimsTable := "farol.agg_fat_dims_mes"
		if fluxo.name == "transmitido" {
			dimsTable = "farol.agg_trans_dims_mes"
		}
		t0 := time.Now()

		// fetchDim(dimName) — retorna [{key, label}] para a dim solicitada.
		// codCol é só pro log; dimName mapeia "cod_X" → "X" (ex: cod_fornec → fornec).
		fetchDim := func(codCol, dimName string) []dimOption {
			td := time.Now()
			rows, err := db.Query(fmt.Sprintf(`
				SELECT key, MAX(label) AS label
				  FROM %s
				 WHERE empresa_id=$1 AND dim=$2 AND key != ''
				 GROUP BY key
				 ORDER BY label
			`, dimsTable), spCtx.EmpresaID, dimName)
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

		// fetchScalar(dimName) — só keys (sem label), usado para uf/empresa
		fetchScalar := func(col, dimName string) []string {
			td := time.Now()
			rows, err := db.Query(fmt.Sprintf(`
				SELECT DISTINCT key FROM %s WHERE empresa_id=$1 AND dim=$2 AND key != '' ORDER BY key
			`, dimsTable), spCtx.EmpresaID, dimName)
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

		fetchCli := func() []dimOption { return fetchDim("cod_cli", "cli") }

		var (
			fornec, gerente, supervisor, rca, cli []dimOption
			uf, empresa                           []string
			wg                                    sync.WaitGroup
		)
		wg.Add(7)
		go func() { defer wg.Done(); fornec = fetchDim("cod_fornec", "fornec") }()
		go func() { defer wg.Done(); gerente = fetchDim("cod_gerente", "gerente") }()
		go func() { defer wg.Done(); supervisor = fetchDim("cod_supervisor", "supervisor") }()
		go func() { defer wg.Done(); rca = fetchDim("cod_rca", "rca") }()
		go func() { defer wg.Done(); cli = fetchCli() }()
		go func() { defer wg.Done(); uf = fetchScalar("uf", "uf") }()
		go func() { defer wg.Done(); empresa = fetchScalar("empresa", "empresa") }()
		wg.Wait()

		log.Printf("[dims] fluxo=%s paralelo=7 total=%v", fluxo.name, time.Since(t0))

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

// codToDimName mapeia "cod_X" → nome da dim na agg_*_dims_mes
var codToDimName = map[string]string{
	"cod_fornec":     "fornec",
	"cod_gerente":    "gerente",
	"cod_supervisor": "supervisor",
	"cod_rca":        "rca",
	"cod_cli":        "cli",
}

// lookupNome busca o nome (label) de um código em agg_*_dims_mes (FAT + TRANS).
// nomeCol é ignorado (parâmetro legado mantido p/ compat de assinatura);
// usa o mês mais recente disponível como source of truth.
func lookupNome(db *sql.DB, empresaID, codCol, nomeCol, cod string) string {
	_ = nomeCol
	if cod == "" {
		return ""
	}
	dim, ok := codToDimName[codCol]
	if !ok {
		return cod
	}
	var nome string
	_ = db.QueryRow(`
		SELECT label FROM farol.agg_fat_dims_mes
		WHERE empresa_id=$1 AND dim=$2 AND key=$3 AND label != ''
		ORDER BY ano DESC, mes DESC LIMIT 1
	`, empresaID, dim, cod).Scan(&nome)
	if nome == "" {
		_ = db.QueryRow(`
			SELECT label FROM farol.agg_trans_dims_mes
			WHERE empresa_id=$1 AND dim=$2 AND key=$3 AND label != ''
			ORDER BY ano DESC, mes DESC LIMIT 1
		`, empresaID, dim, cod).Scan(&nome)
	}
	if nome == "" {
		nome = cod
	}
	return nome
}

// lookupParent descobre o código pai de um código (ex: supervisor de um RCA).
// Usa as tabelas agg_*_v0X_lY_mes que carregam hierarquia adjacente:
//
//	cod_rca → cod_supervisor   via agg_*_v02_l1_mes  (chave (sup, rca))
//	cod_supervisor → cod_gerente via agg_*_v03_l1_mes (chave (ger, sup))
//	cod_rca → cod_gerente      via agg_*_v01_l3_mes  (chave (forn, ger, sup, rca))
func lookupParent(db *sql.DB, empresaID, codCol, cod, parentCol string) string {
	if cod == "" {
		return ""
	}
	type query struct{ table, colCod, colParent string }
	var qs []query
	switch {
	case codCol == "cod_rca" && parentCol == "cod_supervisor":
		qs = []query{
			{"farol.agg_fat_v02_l1_mes", "cod_rca", "cod_supervisor"},
			{"farol.agg_trans_v02_l1_mes", "cod_rca", "cod_supervisor"},
		}
	case codCol == "cod_supervisor" && parentCol == "cod_gerente":
		qs = []query{
			{"farol.agg_fat_v03_l1_mes", "cod_supervisor", "cod_gerente"},
			{"farol.agg_trans_v03_l1_mes", "cod_supervisor", "cod_gerente"},
		}
	case codCol == "cod_rca" && parentCol == "cod_gerente":
		qs = []query{
			{"farol.agg_fat_v01_l3_mes", "cod_rca", "cod_gerente"},
			{"farol.agg_trans_v01_l3_mes", "cod_rca", "cod_gerente"},
		}
	default:
		return ""
	}
	var p string
	for _, q := range qs {
		_ = db.QueryRow(fmt.Sprintf(
			`SELECT %s FROM %s WHERE empresa_id=$1 AND %s=$2 AND %s != '' LIMIT 1`,
			q.colParent, q.table, q.colCod, q.colParent,
		), empresaID, cod).Scan(&p)
		if p != "" {
			return p
		}
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

		rawCNPJ := q.Get("cnpj")
		empresaID := resolveEmpresaCNPJ(db, rawCNPJ)
		if empresaID == "" {
			log.Printf("[farol:public] empresa não encontrada — cnpj=%q scope=%q cod=%q", rawCNPJ, q.Get("scope"), q.Get("cod"))
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "empresa não encontrada para este CNPJ",
				"cnpj":  rawCNPJ,
				"hint":  "verifique se companies.cnpj está preenchido para esta empresa",
			})
			return
		}

		scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
		cod := strings.TrimSpace(q.Get("cod"))
		if cod == "" || (scope != "sup" && scope != "rca") {
			log.Printf("[farol:public] params inválidos — cnpj=%q scope=%q cod=%q", rawCNPJ, scope, cod)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "scope (sup|rca) e cod obrigatórios",
				"scope": scope, "cod": cod,
			})
			return
		}
		log.Printf("[farol:public] cnpj=%q → empresa=%s scope=%s cod=%s", rawCNPJ, empresaID, scope, cod)

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
		kpi := computeKPI(cards, fluxo.name, currentLevel.Level == "cod_fornec")
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
