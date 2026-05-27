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

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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

type vendaMinRow struct {
	CodFornec      string
	NomeFornec     string
	CodGerente     string
	NomeGerente    string
	CodSupervisor  string
	NomeSupervisor string
	CodRca         string
	NomeRca        string
	CodCli         string
	NomeCli        string
	Empresa        string
	Uf             string
	CodProd        string
	NomeProd       string
	Qt             float64
	Pvenda         float64
	Estado         string
	QtcliRca       int
}

type cardItem struct {
	Key            string  `json:"key"`
	Label          string  `json:"label"`
	Level          string  `json:"level"`
	LevelLabel     string  `json:"level_label"`
	ValorAtual     float64 `json:"valor_atual"`
	ValorAnt       float64 `json:"valor_ant"`
	Pct            float64 `json:"pct"`
	Cor            string  `json:"cor"`
	Faturado       float64 `json:"faturado"`
	Transmitido    float64 `json:"transmitido"`
	Positivados    int     `json:"positivados"`
	BaseCli        int     `json:"base_cli"`
	PositPct       float64 `json:"positpct"`
	Mix            float64 `json:"mix"`
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

		travas       := loadTravas(db, spCtx.EmpresaID)
		atualRows    := fetchVendaRows(db, spCtx.EmpresaID, compMode, refAno, refMes, "atual", drillPath)
		anteriorRows := fetchVendaRows(db, spCtx.EmpresaID, compMode, refAno, refMes, "anterior", drillPath)

		cards := aggregateByLevel(atualRows, anteriorRows, currentLevel, projecaoFator, travas)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].Pct < cards[j].Pct
		})

		kpi      := computeKPI(cards)
		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		plabel   := buildPeriodoLabel(compMode, refAno, refMes)

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

// ─── fetchVendaRows ──────────────────────────────────────────────────────────
// Carrega as linhas do banco filtradas por empresa + período (calculado via compMode)
// + drill path. bucket="atual" → período de referência; bucket="anterior" → comparação.

func fetchVendaRows(db *sql.DB, empresaID, compMode string, refAno, refMes int, bucket string, drill []drillStep) []vendaMinRow {
	// args começa com empresaID; período e drill são adicionados dinamicamente.
	args := []any{empresaID}
	var periodCond string

	switch bucket {
	case "atual":
		switch compMode {
		case "ytd":
			args = append(args, refAno, refMes)
			periodCond = "tipo_base='ATUAL' AND ano=$2 AND mes<=$3"
		default: // yoy, mom — um mês específico
			args = append(args, refAno, refMes)
			periodCond = "tipo_base='ATUAL' AND ano=$2 AND mes=$3"
		}
	case "anterior":
		switch compMode {
		case "yoy":
			args = append(args, refMes)
			periodCond = "tipo_base='COMPARATIVA' AND mes=$2"
		case "ytd":
			periodCond = "tipo_base='COMPARATIVA'"
		case "mom":
			prevMes, prevAno := refMes-1, refAno
			if prevMes == 0 {
				prevMes, prevAno = 12, refAno-1
			}
			args = append(args, prevAno, prevMes)
			periodCond = "(tipo_base='ATUAL' OR tipo_base='COMPARATIVA') AND ano=$2 AND mes=$3"
		default:
			args = append(args, refMes)
			periodCond = "tipo_base='COMPARATIVA' AND mes=$2"
		}
	}

	// Drill filters — placeholders começam após os period args
	var drillParts []string
	for _, d := range drill {
		col := safeColName(d.Level)
		args = append(args, d.Value)
		drillParts = append(drillParts, fmt.Sprintf("%s=$%d", col, len(args)))
	}
	drillCond := ""
	if len(drillParts) > 0 {
		drillCond = " AND " + strings.Join(drillParts, " AND ")
	}

	query := `
		SELECT cod_fornec, nome_fornec,
		       cod_gerente, nome_gerente,
		       cod_supervisor, nome_supervisor,
		       cod_rca, nome_rca,
		       cod_cli, nome_cli,
		       empresa, uf,
		       cod_prod, nome_prod,
		       qt, pvenda, estado, qtcli_rca
		FROM vendas_importadas
		WHERE empresa_id=$1 AND ` + periodCond + drillCond

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []vendaMinRow
	for rows.Next() {
		var row vendaMinRow
		if err := rows.Scan(
			&row.CodFornec, &row.NomeFornec,
			&row.CodGerente, &row.NomeGerente,
			&row.CodSupervisor, &row.NomeSupervisor,
			&row.CodRca, &row.NomeRca,
			&row.CodCli, &row.NomeCli,
			&row.Empresa, &row.Uf,
			&row.CodProd, &row.NomeProd,
			&row.Qt, &row.Pvenda, &row.Estado, &row.QtcliRca,
		); err == nil {
			result = append(result, row)
		}
	}
	return result
}

// safeColName valida o nome da coluna para evitar SQL injection.
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

// getRowField retorna o valor de um campo da vendaMinRow por nome de coluna.
func getRowField(r *vendaMinRow, field string) string {
	switch field {
	case "cod_fornec":
		return r.CodFornec
	case "nome_fornec":
		return r.NomeFornec
	case "cod_gerente":
		return r.CodGerente
	case "nome_gerente":
		return r.NomeGerente
	case "cod_supervisor":
		return r.CodSupervisor
	case "nome_supervisor":
		return r.NomeSupervisor
	case "cod_rca":
		return r.CodRca
	case "nome_rca":
		return r.NomeRca
	case "cod_cli":
		return r.CodCli
	case "nome_cli":
		return r.NomeCli
	case "empresa":
		return r.Empresa
	case "uf":
		return r.Uf
	case "cod_prod":
		return r.CodProd
	case "nome_prod":
		return r.NomeProd
	}
	return ""
}

// ─── aggregateByLevel ────────────────────────────────────────────────────────

func aggregateByLevel(atualRows, anteriorRows []vendaMinRow, level hierLevel, projecaoFator float64, travas map[string]float64) []cardItem {
	type group struct {
		label    string
		atual    []vendaMinRow
		anterior []vendaMinRow
	}
	groups := make(map[string]*group)

	addRow := func(rows []vendaMinRow, bucket string) {
		for i := range rows {
			r := &rows[i]
			key := getRowField(r, level.Level)
			if key == "" {
				continue
			}
			g, ok := groups[key]
			if !ok {
				lbl := getRowField(r, level.NameField)
				if lbl == "" {
					lbl = key
				}
				g = &group{label: lbl}
				groups[key] = g
			}
			if bucket == "atual" {
				g.atual = append(g.atual, *r)
			} else {
				g.anterior = append(g.anterior, *r)
			}
		}
	}
	addRow(atualRows, "atual")
	addRow(anteriorRows, "anterior")

	cards := make([]cardItem, 0, len(groups))
	for key, g := range groups {
		cards = append(cards, calcCardGo(key, g.label, level, g.atual, g.anterior, projecaoFator, travas))
	}
	return cards
}

// ─── calcCardGo ──────────────────────────────────────────────────────────────
// Equivalente ao calcCard() do maquete.js — calcula todos os indicadores de um card.

func calcCardGo(key, label string, level hierLevel, atualRows, anteriorRows []vendaMinRow, projecaoFator float64, travas map[string]float64) cardItem {
	var valorAtual, faturado, transmitido, valorAnt float64
	var maxQtcliRca int

	positSet    := make(map[string]bool)
	cliDistinct := make(map[string]bool)
	type strSet = map[string]bool
	cliProds := make(map[string]strSet) // cliente → set de produtos

	for i := range atualRows {
		r := &atualRows[i]
		valorAtual += r.Pvenda
		if r.Estado == "FATURADO" {
			faturado += r.Pvenda
		} else {
			transmitido += r.Pvenda
		}
		if r.QtcliRca > maxQtcliRca {
			maxQtcliRca = r.QtcliRca
		}
		if r.CodCli == "" {
			continue
		}
		cliDistinct[r.CodCli] = true
		trava := travas[r.CodFornec]
		if trava <= 0 {
			trava = 1
		}
		if r.Qt >= trava {
			positSet[r.CodCli] = true
		}
		if r.Qt > 0 && r.CodProd != "" {
			if cliProds[r.CodCli] == nil {
				cliProds[r.CodCli] = make(strSet)
			}
			cliProds[r.CodCli][r.CodProd] = true
		}
	}
	for i := range anteriorRows {
		valorAnt += anteriorRows[i].Pvenda
	}

	valorAtual *= projecaoFator

	var pct float64
	if valorAnt > 0 {
		pct = valorAtual / valorAnt * 100
	} else if valorAtual > 0 {
		pct = 100
	}
	cor := "vermelho"
	if pct >= 100 {
		cor = "verde"
	}

	baseCli := maxQtcliRca
	if baseCli == 0 {
		baseCli = len(cliDistinct)
	}
	positivados := len(positSet)
	var positPct float64
	if baseCli > 0 {
		positPct = float64(positivados) / float64(baseCli) * 100
	}

	var mix float64
	if len(cliProds) > 0 {
		var total int
		for _, prods := range cliProds {
			total += len(prods)
		}
		mix = float64(total) / float64(len(cliProds))
	}

	return cardItem{
		Key: key, Label: label,
		Level: level.Level, LevelLabel: level.Label,
		ValorAtual: valorAtual, ValorAnt: valorAnt,
		Pct: pct, Cor: cor,
		Faturado: faturado, Transmitido: transmitido,
		Positivados: positivados, BaseCli: baseCli, PositPct: positPct,
		Mix: mix,
	}
}

// ─── loadTravas ──────────────────────────────────────────────────────────────

func loadTravas(db *sql.DB, empresaID string) map[string]float64 {
	rows, err := db.Query(
		`SELECT cod_fornec, trava_minima_qt FROM industrias_config WHERE empresa_id=$1`,
		empresaID,
	)
	if err != nil {
		return map[string]float64{}
	}
	defer rows.Close()
	result := map[string]float64{}
	for rows.Next() {
		var cod string
		var trava float64
		if rows.Scan(&cod, &trava) == nil {
			result[cod] = trava
		}
	}
	return result
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
