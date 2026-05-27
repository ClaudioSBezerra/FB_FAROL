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
// Dados lidos de farol.mv_farol_resumo (materialized view) — GROUP BY feito no
// Postgres, não em Go. A view é refreshed após cada importação.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"
	"strconv"
	"strings"
)

// ─── Definição de hierarquias ────────────────────────────────────────────────

type hierLevel struct {
	Level     string
	NameField string
	Label     string
}

var hierarquias = map[string][]hierLevel{
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
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "empresa", NameField: "empresa", Label: "Empresa"},
		{Level: "uf", NameField: "uf", Label: "UF"},
		{Level: "cod_gerente", NameField: "nome_gerente", Label: "Gerente"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
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
				SELECT ano, mes FROM vendas_importadas
				WHERE empresa_id=$1 AND tipo_base='ATUAL'
				ORDER BY ano DESC, mes DESC LIMIT 1
			`, spCtx.EmpresaID).Scan(&refAno, &refMes)
		}
		if refAno == 0 {
			json.NewEncoder(w).Encode(cardsResponse{Cards: []cardItem{}, View: view, DrillPath: drillPath})
			return
		}

		projecaoFator := 1.0
		if compMode == "ytd" && refMes > 0 {
			projecaoFator = 12.0 / float64(refMes)
		}

		cards    := fetchCards(db, spCtx.EmpresaID, compMode, refAno, refMes, currentLevel, drillPath, projecaoFator)
		kpi      := computeKPI(cards)
		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		plabel   := buildPeriodoLabel(compMode, refAno, refMes)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].Pct < cards[j].Pct
		})

		json.NewEncoder(w).Encode(cardsResponse{
			Cards:          cards,
			KPI:            kpi,
			Periodo:        periodoInfo{RefAno: refAno, RefMes: refMes, Label: plabel, CompMode: compMode},
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
	"empresa": true, "uf": true,
	"cod_prod": true, "nome_prod": true,
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

// buildAntCond monta a cláusula WHERE de período para o bucket anterior.
func buildAntCond(compMode string, refAno, refMes int, args *[]any) string {
	switch compMode {
	case "yoy":
		*args = append(*args, refMes)
		return fmt.Sprintf("v.tipo_base='COMPARATIVA' AND v.mes=$%d", len(*args))
	case "ytd":
		return "v.tipo_base='COMPARATIVA'" // sem filtro de mês (acumula o ano todo)
	case "mom":
		prevMes, prevAno := refMes-1, refAno
		if prevMes == 0 {
			prevMes, prevAno = 12, refAno-1
		}
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
// Executa a query principal contra mv_farol_resumo para o bucket "atual".
// Retorna um map[key]aggResult (uma entrada por valor do groupCol).
//
// A query usa CTEs para calcular positivados (com trava por fornecedor) e mix
// (média de produtos distintos por cliente), evitando carregar linhas brutas
// no Go — o Postgres agrupa e entrega apenas as N colunas do nível atual.

func queryAggregated(db *sql.DB, groupCol, nameCol, atualCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
WITH
  pos AS (
    -- Clientes positivados: compraram acima da trava mínima do fornecedor
    SELECT v.%s AS k,
           COUNT(DISTINCT CASE WHEN v.qt >= COALESCE(ic.trava_minima_qt, 1) THEN v.cod_cli END) AS n
    FROM farol.mv_farol_resumo v
    LEFT JOIN industrias_config ic
           ON ic.empresa_id = v.empresa_id AND ic.cod_fornec = v.cod_fornec
    WHERE v.empresa_id=$1 AND v.%s != '' AND v.cod_cli != ''
    AND %s %s
    GROUP BY v.%s
  ),
  mix_inner AS (
    -- Produtos distintos por (grupo, cliente) para calcular a média
    SELECT v.%s AS k, v.cod_cli, COUNT(DISTINCT v.cod_prod) AS np
    FROM farol.mv_farol_resumo v
    WHERE v.empresa_id=$1 AND v.%s != '' AND v.cod_cli != ''
    AND v.qt > 0 AND v.cod_prod != ''
    AND %s %s
    GROUP BY v.%s, v.cod_cli
  ),
  mix AS (
    SELECT k, AVG(np)::float AS avg_mix FROM mix_inner GROUP BY k
  )
SELECT
  v.%s                                                                      AS key,
  MAX(v.%s)                                                                 AS label,
  SUM(v.pvenda)                                                             AS valor,
  SUM(CASE WHEN v.estado = 'FATURADO' THEN v.pvenda ELSE 0 END)            AS faturado,
  SUM(CASE WHEN v.estado != 'FATURADO' THEN v.pvenda ELSE 0 END)           AS transmitido,
  CASE WHEN MAX(v.qtcli_rca) > 0
       THEN MAX(v.qtcli_rca)::int
       ELSE COUNT(DISTINCT v.cod_cli)::int END                              AS base_cli,
  COALESCE(MAX(pos.n), 0)                                                   AS positivados,
  COALESCE(MAX(mix.avg_mix), 0)                                             AS mix
FROM farol.mv_farol_resumo v
LEFT JOIN pos  ON pos.k  = v.%s
LEFT JOIN mix  ON mix.k  = v.%s
WHERE v.empresa_id=$1 AND v.%s != ''
AND %s %s
GROUP BY v.%s`,
		// pos CTE
		groupCol,
		groupCol, atualCond, drillCond,
		groupCol,
		// mix_inner CTE
		groupCol,
		groupCol,
		atualCond, drillCond,
		groupCol,
		// main SELECT
		groupCol, nameCol,
		groupCol, groupCol,
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

func queryAnteriorTotals(db *sql.DB, groupCol, antCond, drillCond string, args []any) map[string]float64 {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT v.%s AS key, SUM(v.pvenda) AS valor_ant
FROM farol.mv_farol_resumo v
WHERE v.empresa_id=$1 AND v.%s != '' AND %s %s
GROUP BY v.%s`, groupCol, groupCol, antCond, drillCond, groupCol)

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

// ─── fetchCards ───────────────────────────────────────────────────────────────
// Orquestra as duas queries (atual + anterior) e monta os cardItems finais.

func fetchCards(db *sql.DB, empresaID, compMode string, refAno, refMes int, level hierLevel, drillPath []drillStep, projecaoFator float64) []cardItem {
	t0       := time.Now()
	groupCol := safeColName(level.Level)
	nameCol  := safeColName(level.NameField)

	log.Printf("[farol:view] fetchCards empresa=%s nível=%s compMode=%s ref=%04d-%02d drill=%d",
		empresaID, groupCol, compMode, refAno, refMes, len(drillPath))

	// Condições e args do bucket atual
	atualArgs := []any{empresaID}
	atualCond := buildAtualCond(compMode, refAno, refMes, &atualArgs)
	drillCond := buildDrillCond(drillPath, &atualArgs)

	// Condições e args do bucket anterior (args independentes para evitar conflito de $N)
	antArgs  := []any{empresaID}
	antCond  := buildAntCond(compMode, refAno, refMes, &antArgs)
	antDrill := buildDrillCond(drillPath, &antArgs)

	atualMap := queryAggregated(db, groupCol, nameCol, atualCond, drillCond, atualArgs)
	antMap   := queryAnteriorTotals(db, groupCol, antCond, antDrill, antArgs)

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
	rows, err := db.Query(`
		SELECT DISTINCT ano, mes FROM vendas_importadas
		WHERE empresa_id=$1
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

// ─── buildPeriodoLabel ────────────────────────────────────────────────────────

func buildPeriodoLabel(compMode string, refAno, refMes int) string {
	cur := fmtMesAno(refMes, refAno)
	switch compMode {
	case "yoy":
		return fmt.Sprintf("%s vs %s", cur, fmtMesAno(refMes, refAno-1))
	case "ytd":
		return fmt.Sprintf("Projeção %d (%s–%s) vs Total %d", refAno, fmtMesAno(1, refAno), cur, refAno-1)
	case "mom":
		pm, py := refMes-1, refAno
		if pm == 0 {
			pm, py = 12, refAno-1
		}
		return fmt.Sprintf("%s vs %s", cur, fmtMesAno(pm, py))
	}
	return cur
}

// ─── RefreshViewsHandler — POST /api/v2/farol/refresh-views ─────────────────
// Dispara REFRESH MATERIALIZED VIEW CONCURRENTLY na mv_farol_resumo.
// Necessário após deploy inicial ou quando a view ficou desatualizada.

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
		log.Printf("[farol:view] RefreshViews início — solicitado por empresa=%s user=%s", spCtx.EmpresaID, spCtx.UserID)

		if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY farol.mv_farol_resumo`); err != nil {
			log.Printf("[farol:view] RefreshViews ERRO em %v: %v", time.Since(t0), err)
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}

		var rowCount int
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.mv_farol_resumo`).Scan(&rowCount)
		db.Exec(`ANALYZE farol.mv_farol_resumo`)
		log.Printf("[farol:view] RefreshViews concluído — %d linhas, ANALYZE OK, total %v", rowCount, time.Since(t0))
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
