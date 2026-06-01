package handlers

// farol_v2_api.go — API de cards do novo Farol 2026.
//
// GET /api/v2/farol/cards
//   Parâmetros:
//     view        V01 | V02 | V03
//     comp_mode   yoy | ytd | mom  (default: yoy)
//     ref_ano     ano de referência (default: mais recente no banco)
//     ref_mes     mês de referência 1-12 (default: mais recente)
//     drill       JSON: [{"level":"cod_fornec","value":"001","label":"MARCA X"}]
//
// GET /api/v2/farol/periodos — anos+meses disponíveis no banco
//
// Dados lidos de views materializadas pré-agregadas (uma por nível de drill,
// migration 143). A API só faz SELECT/WHERE — sem GROUP BY. Refresh após import.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
	"strconv"
	"strings"
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
		{Level: "cod_fornec",     NameField: "nome_fornec",    Label: "Fornecedor"},
		{Level: "cod_gerente",    NameField: "nome_gerente",   Label: "Gerente"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca",        NameField: "nome_rca",       Label: "RCA"},
		{Level: "cod_cli",        NameField: "nome_cli",       Label: "Cliente"},
		{Level: "cod_prod",       NameField: "nome_prod",      Label: "Produto"},
	},
	// V02: visão por equipe (força de vendas) — Supervisor → RCA → Fornecedor → Cliente → Produto
	"V02": {
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca",        NameField: "nome_rca",       Label: "RCA"},
		{Level: "cod_fornec",     NameField: "nome_fornec",    Label: "Fornecedor"},
		{Level: "cod_cli",        NameField: "nome_cli",       Label: "Cliente"},
		{Level: "cod_prod",       NameField: "nome_prod",      Label: "Produto"},
	},
	// V03: visão de gerência (organização) — Gerente/GGV → Supervisor → RCA → Cliente → Produto
	"V03": {
		{Level: "cod_gerente",    NameField: "nome_gerente",    Label: "Gerência"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca",        NameField: "nome_rca",       Label: "RCA"},
		{Level: "cod_cli",        NameField: "nome_cli",       Label: "Cliente"},
		{Level: "cod_prod",       NameField: "nome_prod",      Label: "Produto"},
	},
}

// viewPorNivel mapeia (view, drillIdx) → nome da view materializada pré-agregada.
// Cada view tem as métricas (pvenda, faturado, transmitido, base_cli, positivados, mix)
// já calculadas — a API só lê as linhas prontas com um WHERE simples.
var viewPorNivel = map[string][]string{
	"V01": {"mv_v01_l0", "mv_v01_l1", "mv_v01_l2", "mv_v01_l3", "mv_farol_cli"},
	"V02": {"mv_v02_l0", "mv_v02_l1", "mv_v02_l2", "mv_farol_cli"},
	"V03": {"mv_v03_l0", "mv_v03_l1", "mv_v03_l2", "mv_v03_l3"},
}

// AllSummaryViews lista TODAS as views em ordem de REFRESH (base primeiro).
var AllSummaryViews = []string{
	"farol.mv_farol_cli",
	"farol.mv_v01_l0", "farol.mv_v01_l1", "farol.mv_v01_l2", "farol.mv_v01_l3",
	"farol.mv_v02_l0", "farol.mv_v02_l1", "farol.mv_v02_l2",
	"farol.mv_v03_l0", "farol.mv_v03_l1", "farol.mv_v03_l2", "farol.mv_v03_l3",
	"farol.mv_mkt_produto",
	"farol.mv_mkt_prod_pen",
}

func getViewName(view string, drillIdx int) string {
	if levels, ok := viewPorNivel[view]; ok && drillIdx >= 0 && drillIdx < len(levels) {
		return "farol." + levels[drillIdx]
	}
	return "farol.mv_farol_cli"
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
	Cor         string  `json:"cor"`
	Faturado    float64 `json:"faturado"`
	Transmitido float64 `json:"transmitido"`
	Positivados int     `json:"positivados"`
	BaseCli     int     `json:"base_cli"`
	PositPct    float64 `json:"positpct"`
	Mix         float64 `json:"mix"`
}

type kpiSummary struct {
	TotalAtual       float64 `json:"total_atual"`
	TotalAnt         float64 `json:"total_ant"`
	TotalPct         float64 `json:"total_pct"`
	TotalCor         string  `json:"total_cor"`
	TotalFaturado    float64 `json:"total_faturado"`
	TotalTransmitido float64 `json:"total_transmitido"`
	TotalPositivados int     `json:"total_positivados"`
	TotalBaseCli     int     `json:"total_base_cli"`
	TotalPositPct    float64 `json:"total_positpct"`
	AvgMix           float64 `json:"avg_mix"`
	Verdes           int     `json:"verdes"`
	Vermelhos        int     `json:"vermelhos"`
}

type periodoInfo struct {
	RefAno   int    `json:"ref_ano"`
	RefMes   int    `json:"ref_mes"`
	Label    string `json:"label"`
	CompMode string `json:"comp_mode"`
	CurLabel string `json:"cur_label"`           // ex.: "Mai/2026"
	AntLabel string `json:"ant_label"`           // ex.: "Mai/2025" (yoy) | "Total 2025" (ytd) | "Abr/2026" (mom)
	CompAno  int    `json:"comp_ano,omitempty"`  // override do mom — quando setado pelo usuário
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

		compMode := q.Get("comp_mode")
		if compMode == "" {
			compMode = "yoy"
		}

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

		refAno, _ := strconv.Atoi(q.Get("ref_ano"))
		refMes, _ := strconv.Atoi(q.Get("ref_mes"))
		if refAno == 0 || refMes == 0 {
			_ = db.QueryRow(`
				SELECT ano, mes FROM vendas_import_jobs
				WHERE empresa_id=$1 AND tipo_base='ATUAL' AND status='done'
				ORDER BY ano DESC, mes DESC LIMIT 1
			`, spCtx.EmpresaID).Scan(&refAno, &refMes)
		}
		if refAno == 0 {
			json.NewEncoder(w).Encode(cardsResponse{Cards: []cardItem{}, View: view, DrillPath: drillPath})
			return
		}

		// Override opcional do período de comparação (só usado em mom).
		compAno, _ := strconv.Atoi(q.Get("comp_ano"))
		compMes, _ := strconv.Atoi(q.Get("comp_mes"))

		projecaoFator := 1.0
		if compMode == "ytd" && refMes > 0 {
			projecaoFator = 12.0 / float64(refMes)
		}

		cards    := fetchCards(db, spCtx.EmpresaID, view, compMode, refAno, refMes, compAno, compMes, drillIdx, currentLevel, drillPath, projecaoFator)
		kpi      := computeKPI(cards)
		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		curLabel, antLabel, plabel := buildPeriodoLabels(compMode, refAno, refMes, compAno, compMes)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].Pct < cards[j].Pct
		})

		json.NewEncoder(w).Encode(cardsResponse{
			Cards:          cards,
			KPI:            kpi,
			Periodo:        periodoInfo{RefAno: refAno, RefMes: refMes, Label: plabel, CompMode: compMode, CurLabel: curLabel, AntLabel: antLabel, CompAno: compAno, CompMes: compMes},
			Periodos:       periodos,
			View:           view,
			DrillPath:      drillPath,
			NextLevel:      currentLevel.Level,
			NextLevelLabel: currentLevel.Label,
		})
	}
}

// ─── safeColName ─────────────────────────────────────────────────────────────
// Valida nomes de coluna antes de interpolá-los no SQL para evitar injeção.

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

// buildAtualCond monta a cláusula WHERE de período para o bucket atual.
// Apenda os args necessários em *args e retorna o fragmento SQL com $N.
func buildAtualCond(compMode string, refAno, refMes int, args *[]any) string {
	switch compMode {
	case "ytd":
		*args = append(*args, refAno, refMes)
		n := len(*args)
		return fmt.Sprintf("v.tipo_base='ATUAL' AND v.ano=$%d AND v.mes<=$%d", n-1, n)
	default: // yoy, mom — mês exato
		*args = append(*args, refAno, refMes)
		n := len(*args)
		return fmt.Sprintf("v.tipo_base='ATUAL' AND v.ano=$%d AND v.mes=$%d", n-1, n)
	}
}

// resolveCompPeriod retorna (ano, mes) do bucket "anterior" para o mom.
// Aplica override do usuário se compAno+compMes vierem setados; senão usa mês-1.
func resolveCompPeriod(refAno, refMes, compAno, compMes int) (int, int) {
	if compAno > 0 && compMes > 0 {
		return compAno, compMes
	}
	prevMes, prevAno := refMes-1, refAno
	if prevMes == 0 {
		prevMes, prevAno = 12, refAno-1
	}
	return prevAno, prevMes
}

// buildAntCond monta a cláusula WHERE de período para o bucket anterior.
// Para mom aceita override explícito (compAno/compMes) — quando 0, calcula mês-1.
// yoy/ytd comparam contra refAno-1 para evitar somar múltiplos anos de COMPARATIVA.
func buildAntCond(compMode string, refAno, refMes, compAno, compMes int, args *[]any) string {
	switch compMode {
	case "yoy":
		*args = append(*args, refAno-1, refMes)
		n := len(*args)
		return fmt.Sprintf("v.tipo_base='COMPARATIVA' AND v.ano=$%d AND v.mes=$%d", n-1, n)
	case "ytd":
		*args = append(*args, refAno-1)
		return fmt.Sprintf("v.tipo_base='COMPARATIVA' AND v.ano=$%d", len(*args))
	case "mom":
		prevAno, prevMes := resolveCompPeriod(refAno, refMes, compAno, compMes)
		*args = append(*args, prevAno, prevMes)
		n := len(*args)
		return fmt.Sprintf("(v.tipo_base='ATUAL' OR v.tipo_base='COMPARATIVA') AND v.ano=$%d AND v.mes=$%d", n-1, n)
	default:
		*args = append(*args, refMes)
		return fmt.Sprintf("v.tipo_base='COMPARATIVA' AND v.mes=$%d", len(*args))
	}
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

// ─── aggResult — resultado de uma linha agregada no SQL ──────────────────────

type aggResult struct {
	label       string
	valor       float64
	faturado    float64
	transmitido float64
	baseCli     int
	positivados int
	mix         float64
}

// ─── queryAggregated ─────────────────────────────────────────────────────────
// Lê a view pré-agregada para o bucket "atual".
// Cada view (migration 143) tem UMA linha por grupo com métricas pré-calculadas.
// Não há GROUP BY aqui — apenas SELECT + WHERE sobre linhas prontas.
// Tempo esperado: <10ms independente do volume de dados.

func queryAggregated(db *sql.DB, viewName, groupCol, nameCol, atualCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	// GROUP BY é necessário para o modo ytd (mes <= N), onde a view tem uma linha
	// por mês e a query retorna N linhas por grupo. Nos modos yoy/mom (mes exato),
	// o GROUP BY é inócuo — cada grupo já tem uma única linha.
	q := fmt.Sprintf(`
SELECT
  v.%s                           AS key,
  MAX(v.%s)                      AS label,
  SUM(v.pvenda)                  AS valor,
  SUM(v.faturado)                AS faturado,
  SUM(v.transmitido)             AS transmitido,
  ROUND(AVG(v.base_cli))::int    AS base_cli,
  ROUND(AVG(v.positivados))::int AS positivados,
  AVG(v.mix)                     AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.%s != ''
AND %s %s
GROUP BY v.%s`,
		groupCol, nameCol,
		viewName,
		groupCol, atualCond, drillCond,
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
		if err := rows.Scan(&key, &r.label, &r.valor, &r.faturado, &r.transmitido,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryAggregated nível=%s → %d grupos em %v", groupCol, len(result), time.Since(t0))
	return result
}

// ─── queryAnteriorTotals ──────────────────────────────────────────────────────
// Busca apenas o total de pvenda por grupo para o bucket anterior.
// Query simples: uma linha por grupo.

func queryAnteriorTotals(db *sql.DB, viewName, groupCol, antCond, drillCond string, args []any) map[string]float64 {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT v.%s AS key, SUM(v.pvenda) AS valor_ant
FROM %s v
WHERE v.empresa_id=$1 AND v.%s != '' AND %s %s
GROUP BY v.%s`, groupCol, viewName, groupCol, antCond, drillCond, groupCol)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryAnteriorTotals nível=%s ERRO em %v: %v", groupCol, time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var key string
		var val float64
		if rows.Scan(&key, &val) == nil {
			result[key] = val
		}
	}
	log.Printf("[farol:view] queryAnteriorTotals nível=%s → %d grupos em %v", groupCol, len(result), time.Since(t0))
	return result
}

// ─── queryProdutos / queryProdutosAnterior ───────────────────────────────────
// Nível folha "Produto": não existe view pré-agregada (o cod_prod foi agregado
// no mix). Lê de vendas_importadas, escopado ao cliente do drill (poucas linhas).

func queryProdutos(db *sql.DB, atualCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.cod_prod                                                   AS key,
  MAX(v.nome_prod)                                             AS label,
  SUM(v.pvenda)                                                AS valor,
  SUM(CASE WHEN v.estado = 'FATURADO'  THEN v.pvenda ELSE 0 END) AS faturado,
  SUM(CASE WHEN v.estado != 'FATURADO' THEN v.pvenda ELSE 0 END) AS transmitido,
  0::int                                                       AS base_cli,
  0::int                                                       AS positivados,
  0::float                                                     AS mix
FROM vendas_importadas v
WHERE v.empresa_id=$1 AND v.cod_prod != '' AND %s %s
GROUP BY v.cod_prod`, atualCond, drillCond)

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
		if err := rows.Scan(&key, &r.label, &r.valor, &r.faturado, &r.transmitido,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryProdutos → %d produtos em %v", len(result), time.Since(t0))
	return result
}

func queryProdutosAnterior(db *sql.DB, antCond, drillCond string, args []any) map[string]float64 {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT v.cod_prod AS key, SUM(v.pvenda) AS valor_ant
FROM vendas_importadas v
WHERE v.empresa_id=$1 AND v.cod_prod != '' AND %s %s
GROUP BY v.cod_prod`, antCond, drillCond)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryProdutosAnterior ERRO em %v: %v", time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var key string
		var val float64
		if rows.Scan(&key, &val) == nil {
			result[key] = val
		}
	}
	return result
}

// ─── fetchCards ───────────────────────────────────────────────────────────────
// Orquestra as duas queries (atual + anterior) e monta os cardItems finais.

func fetchCards(db *sql.DB, empresaID, view, compMode string, refAno, refMes, compAno, compMes, drillIdx int, level hierLevel, drillPath []drillStep, projecaoFator float64) []cardItem {
	t0       := time.Now()
	groupCol := safeColName(level.Level)
	nameCol  := safeColName(level.NameField)
	viewName := getViewName(view, drillIdx)

	log.Printf("[farol:view] fetchCards empresa=%s view=%s nível=%s compMode=%s ref=%04d-%02d drill=%d",
		empresaID, viewName, groupCol, compMode, refAno, refMes, len(drillPath))

	// Condições e args do bucket atual
	atualArgs := []any{empresaID}
	atualCond := buildAtualCond(compMode, refAno, refMes, &atualArgs)
	drillCond := buildDrillCond(drillPath, &atualArgs)

	// Condições e args do bucket anterior (args independentes para evitar conflito de $N)
	antArgs  := []any{empresaID}
	antCond  := buildAntCond(compMode, refAno, refMes, compAno, compMes, &antArgs)
	antDrill := buildDrillCond(drillPath, &antArgs)

	// As duas queries são independentes (buckets atual e anterior) — rodam em paralelo.
	// O nível Produto não existe nas views pré-agregadas (mix agrega o cod_prod),
	// então lê direto de vendas_importadas, escopado ao cliente do drill (volume pequeno).
	var atualMap map[string]aggResult
	var antMap map[string]float64
	var wg sync.WaitGroup
	wg.Add(2)
	if groupCol == "cod_prod" {
		go func() { defer wg.Done(); atualMap = queryProdutos(db, atualCond, drillCond, atualArgs) }()
		go func() { defer wg.Done(); antMap = queryProdutosAnterior(db, antCond, antDrill, antArgs) }()
	} else {
		go func() { defer wg.Done(); atualMap = queryAggregated(db, viewName, groupCol, nameCol, atualCond, drillCond, atualArgs) }()
		go func() { defer wg.Done(); antMap = queryAnteriorTotals(db, viewName, groupCol, antCond, antDrill, antArgs) }()
	}
	wg.Wait()

	seen := make(map[string]bool, len(atualMap))
	cards := make([]cardItem, 0, len(atualMap)+len(antMap))

	for key, r := range atualMap {
		seen[key] = true
		ant        := antMap[key]
		valorAtual := r.valor * projecaoFator

		pct := 0.0
		if ant > 0 {
			pct = valorAtual / ant * 100
		} else if valorAtual > 0 {
			pct = 100
		}
		cor := "vermelho"
		if pct >= 100 {
			cor = "verde"
		}

		positPct := 0.0
		if r.baseCli > 0 {
			positPct = float64(r.positivados) / float64(r.baseCli) * 100
		}

		cards = append(cards, cardItem{
			Key: key, Label: r.label,
			Level: level.Level, LevelLabel: level.Label,
			ValorAtual: valorAtual, ValorAnt: ant,
			Pct: pct, Cor: cor,
			Faturado:    r.faturado * projecaoFator,
			Transmitido: r.transmitido * projecaoFator,
			Positivados: r.positivados, BaseCli: r.baseCli, PositPct: positPct,
			Mix: r.mix,
		})
	}

	// Grupos que venderam no período anterior mas zero no atual → vermelho
	for key, ant := range antMap {
		if seen[key] || ant == 0 {
			continue
		}
		cards = append(cards, cardItem{
			Key: key, Label: key,
			Level: level.Level, LevelLabel: level.Label,
			ValorAtual: 0, ValorAnt: ant, Pct: 0, Cor: "vermelho",
		})
	}

	log.Printf("[farol:view] fetchCards nível=%s → %d cards (atual=%d ant-only=%d) total=%v",
		groupCol, len(cards), len(atualMap), len(cards)-len(atualMap), time.Since(t0))
	return cards
}

// ─── computeKPI ──────────────────────────────────────────────────────────────

func computeKPI(cards []cardItem) kpiSummary {
	var kpi kpiSummary
	var mixTotal float64
	mixCount := 0
	for _, c := range cards {
		kpi.TotalAtual += c.ValorAtual
		kpi.TotalAnt += c.ValorAnt
		kpi.TotalFaturado += c.Faturado
		kpi.TotalTransmitido += c.Transmitido
		kpi.TotalPositivados += c.Positivados
		kpi.TotalBaseCli += c.BaseCli
		if c.Mix > 0 {
			mixTotal += c.Mix
			mixCount++
		}
		if c.Cor == "verde" {
			kpi.Verdes++
		} else {
			kpi.Vermelhos++
		}
	}
	if kpi.TotalAnt > 0 {
		kpi.TotalPct = kpi.TotalAtual / kpi.TotalAnt * 100
	} else if kpi.TotalAtual > 0 {
		kpi.TotalPct = 100
	}
	kpi.TotalCor = "vermelho"
	if kpi.TotalPct >= 100 {
		kpi.TotalCor = "verde"
	}
	if kpi.TotalBaseCli > 0 {
		kpi.TotalPositPct = float64(kpi.TotalPositivados) / float64(kpi.TotalBaseCli) * 100
	}
	if mixCount > 0 {
		kpi.AvgMix = mixTotal / float64(mixCount)
	}
	return kpi
}

// ─── fetchPeriodosDisponiveis ─────────────────────────────────────────────────

func fetchPeriodosDisponiveis(db *sql.DB, empresaID string) []string {
	// vendas_import_jobs é minúscula (~10-100 linhas/empresa) e tem índice em
	// (empresa_id, status). Evita SELECT DISTINCT sobre vendas_importadas (1M+ linhas).
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
// Produz três rótulos derivados do modo + período:
//   curLabel — descreve o bucket atual (ex.: "Mai/2026" ou "Projeção 2026 (Jan–Mai)")
//   antLabel — descreve o bucket anterior  (ex.: "Mai/2025" | "Total 2025" | "Abr/2026")
//   label    — string única "Anterior: X × Atual: Y" pra exibir no cabeçalho
//
// O compAno/compMes só importa no mom (quando o usuário escolheu o mês de comparação).

func buildPeriodoLabels(compMode string, refAno, refMes, compAno, compMes int) (curLabel, antLabel, label string) {
	cur := fmtMesAno(refMes, refAno)
	switch compMode {
	case "yoy":
		curLabel = cur
		antLabel = fmtMesAno(refMes, refAno-1)
		label = fmt.Sprintf("Ano Anterior: %s × Ano Atual: %s", antLabel, curLabel)
	case "ytd":
		curLabel = fmt.Sprintf("Projeção %d (%s–%s)", refAno, fmtMesAno(1, refAno), cur)
		antLabel = fmt.Sprintf("Total %d", refAno-1)
		label = fmt.Sprintf("Período Anterior: %s × %s", antLabel, curLabel)
	case "mom":
		pa, pm := resolveCompPeriod(refAno, refMes, compAno, compMes)
		curLabel = cur
		antLabel = fmtMesAno(pm, pa)
		label = fmt.Sprintf("Mês Anterior: %s × Mês Atual: %s", antLabel, curLabel)
	default:
		curLabel = cur
		antLabel = ""
		label = cur
	}
	return
}

// refreshAllFarolViews recria todas as views materializadas: base (mv_farol_cli)
// primeiro — as de resumo dependem dela — depois as de resumo em paralelo.
// Usado após importação, após limpeza e pelo botão "Consolidar view".
func refreshAllFarolViews(db *sql.DB) error {
	t0 := time.Now()
	if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY farol.mv_farol_cli`); err != nil {
		log.Printf("[farol:view] refresh mv_farol_cli ERRO em %v: %v", time.Since(t0), err)
		return err
	}

	summaryViews := AllSummaryViews[1:] // tudo exceto mv_farol_cli (índice 0)
	var wg sync.WaitGroup
	errs := make([]error, len(summaryViews))
	for i, vw := range summaryViews {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			_, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY ` + name)
			if err != nil {
				// MV nunca populada (WITH NO DATA) → tenta sem CONCURRENTLY
				log.Printf("[farol:view] refresh CONCURRENTLY %s falhou (%v), tentando sem CONCURRENTLY", name, err)
				_, err = db.Exec(`REFRESH MATERIALIZED VIEW ` + name)
			}
			if err != nil {
				errs[idx] = err
				log.Printf("[farol:view] refresh %s ERRO: %v", name, err)
			} else {
				db.Exec(`ANALYZE ` + name)
			}
		}(i, vw)
	}
	wg.Wait()
	db.Exec(`ANALYZE farol.mv_farol_cli`)

	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	log.Printf("[farol:view] refreshAllFarolViews concluído em %v", time.Since(t0))
	return nil
}

// ─── RefreshViewsHandler — POST /api/v2/farol/refresh-views ─────────────────
// REFRESH CONCURRENTLY de mv_farol_cli (base) e depois as views de resumo em
// paralelo. Necessário após deploy inicial ou quando as views desatualizaram.

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

		var rowCount int
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.mv_farol_cli`).Scan(&rowCount)
		log.Printf("[farol:view] RefreshViews concluído — %d linhas base, total %v", rowCount, time.Since(t0))
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "rows": rowCount, "duration_ms": time.Since(t0).Milliseconds()})
	}
}

// ─── FarolV2PeriodosHandler — GET /api/v2/farol/periodos ─────────────────────

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

// ─── Acesso público ION VENDAS ───────────────────────────────────────────────
// O app ION abre um link parametrizado SEM login (RCAs em campo) que cai no
// painel novo já escopado ao supervisor ou ao RCA. Mesmas views, mesma
// nomenclatura do painel principal — apenas com o escopo fixado pela URL.

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

// lookupNome busca o nome de um código na view base (ex.: cod_supervisor → nome_supervisor).
func lookupNome(db *sql.DB, empresaID, codCol, nomeCol, cod string) string {
	if cod == "" {
		return ""
	}
	codCol, nomeCol = safeColName(codCol), safeColName(nomeCol)
	var nome string
	q := fmt.Sprintf(
		`SELECT %s FROM farol.mv_farol_cli WHERE empresa_id=$1 AND %s=$2 AND %s!='' LIMIT 1`,
		nomeCol, codCol, nomeCol)
	_ = db.QueryRow(q, empresaID, cod).Scan(&nome)
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
		`SELECT %s FROM farol.mv_farol_cli WHERE empresa_id=$1 AND %s=$2 AND %s!='' LIMIT 1`,
		parentCol, codCol, parentCol)
	_ = db.QueryRow(q, empresaID, cod).Scan(&p)
	return p
}

// FarolV2PublicCardsHandler — GET /api/v2/farol/public/cards (SEM auth)
//   cnpj, scope (sup|rca), cod  → escopo fixo; drill adicional opcional.
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

		// ION sempre usa a visão por equipe (força de vendas).
		view := "V02"
		hier := hierarquias[view]

		compMode := q.Get("comp_mode")
		if compMode == "" {
			compMode = "yoy"
		}

		refAno, _ := strconv.Atoi(q.Get("ref_ano"))
		refMes, _ := strconv.Atoi(q.Get("ref_mes"))
		if refAno == 0 || refMes == 0 {
			_ = db.QueryRow(`
				SELECT ano, mes FROM vendas_import_jobs
				WHERE empresa_id=$1 AND tipo_base='ATUAL' AND status='done'
				ORDER BY ano DESC, mes DESC LIMIT 1`, empresaID).Scan(&refAno, &refMes)
		}
		if refAno == 0 {
			json.NewEncoder(w).Encode(cardsResponse{Cards: []cardItem{}, View: view})
			return
		}

		projecaoFator := 1.0
		if compMode == "ytd" && refMes > 0 {
			projecaoFator = 12.0 / float64(refMes)
		}

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

		// Drill adicional do usuário, apensado após o escopo fixo.
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

		// Override opcional do período de comparação (mom).
		compAno, _ := strconv.Atoi(q.Get("comp_ano"))
		compMes, _ := strconv.Atoi(q.Get("comp_mes"))

		cards := fetchCards(db, empresaID, view, compMode, refAno, refMes, compAno, compMes, drillIdx, currentLevel, drillPath, projecaoFator)
		kpi := computeKPI(cards)
		curLabel, antLabel, plabel := buildPeriodoLabels(compMode, refAno, refMes, compAno, compMes)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].Pct < cards[j].Pct
		})

		json.NewEncoder(w).Encode(cardsResponse{
			Cards:          cards,
			KPI:            kpi,
			Periodo:        periodoInfo{RefAno: refAno, RefMes: refMes, Label: plabel, CompMode: compMode, CurLabel: curLabel, AntLabel: antLabel, CompAno: compAno, CompMes: compMes},
			Periodos:       fetchPeriodosDisponiveis(db, empresaID),
			View:           view,
			DrillPath:      drillPath,
			NextLevel:      currentLevel.Level,
			NextLevelLabel: currentLevel.Label,
		})
	}
}
