package handlers

// marketing_api.go — Painel Marketing: penetração de produto e análise de clientes.
//
// GET /api/v2/marketing/cards
//   Parâmetros:
//     view        produto | cliente | fornec  (default: produto)
//     comp_mode   yoy | ytd | mom             (default: yoy)
//     ref_ano     ano de referência
//     ref_mes     mês de referência 1-12
//     comp_ano / comp_mes  override de período de comparação (apenas mom)

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ─── Tipos de resposta ────────────────────────────────────────────────────────

type mktCard struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	Fornec       string  `json:"fornec,omitempty"`
	NomeFornec   string  `json:"nome_fornec,omitempty"`
	QtClientes   int     `json:"qt_clientes"`   // clientes que positivaram (produto/fornec)
	QtCliAnt     int     `json:"qt_cli_ant"`    // período anterior
	DeltaPct     float64 `json:"delta_pct"`     // variação %
	Pvenda       float64 `json:"pvenda"`
	Faturado     float64 `json:"faturado"`
	Transmitido  float64 `json:"transmitido"`
	PenetrPct    float64 `json:"penetr_pct"`   // qt_clientes / total_base_cli * 100
	Mix          float64 `json:"mix"`           // mix de produtos (visão cliente)
	Positivado   bool    `json:"positivado"`    // para visão cliente
}

type mktKPI struct {
	TotalBaseCli    int     `json:"total_base_cli"`
	TotalAtivos     int     `json:"total_ativos"`
	TotalInativos   int     `json:"total_inativos"`
	TaxaPositivacao float64 `json:"taxa_positivacao"`
	AvgMix          float64 `json:"avg_mix"`
	TotalPvenda     float64 `json:"total_pvenda"`
	TotalFaturado   float64 `json:"total_faturado"`
	TotalTransmitido float64 `json:"total_transmitido"`
}

type mktClienteInativo struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Pvenda      float64 `json:"pvenda"`
	Transmitido float64 `json:"transmitido"`
}

type mktPeriodoInfo struct {
	RefAno   int    `json:"ref_ano"`
	RefMes   int    `json:"ref_mes"`
	Label    string `json:"label"`
	CompMode string `json:"comp_mode"`
	CurLabel string `json:"cur_label"`
	AntLabel string `json:"ant_label"`
}

type mktResponse struct {
	Cards            []mktCard           `json:"cards"`
	KPI              mktKPI              `json:"kpi"`
	ClientesInativos []mktClienteInativo `json:"clientes_inativos"`
	Periodo          mktPeriodoInfo      `json:"periodo"`
	Periodos         []string            `json:"periodos"`
	View             string              `json:"view"`
}

// ─── Handler principal ────────────────────────────────────────────────────────

func MarketingCardsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()
		view := q.Get("view")
		if view == "" {
			view = "produto"
		}
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
				ORDER BY ano DESC, mes DESC LIMIT 1
			`, spCtx.EmpresaID).Scan(&refAno, &refMes)
		}
		if refAno == 0 {
			json.NewEncoder(w).Encode(mktResponse{
				Cards: []mktCard{}, ClientesInativos: []mktClienteInativo{},
				Periodos: []string{}, View: view,
			})
			return
		}

		compAno, _ := strconv.Atoi(q.Get("comp_ano"))
		compMes, _ := strconv.Atoi(q.Get("comp_mes"))

		// Período anterior para label
		antAno, antMes := resolveCompPeriod(refAno, refMes, compAno, compMes)
		if compMode == "yoy" {
			antAno, antMes = refAno-1, refMes
		} else if compMode == "ytd" {
			antAno = refAno - 1
			antMes = 0 // ano completo
		}

		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)

		mesNomes := [...]string{"", "Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}
		curLabel := fmt.Sprintf("%s/%d", mesNomes[refMes], refAno)
		antLabel := ""
		if antMes > 0 {
			antLabel = fmt.Sprintf("%s/%d", mesNomes[antMes], antAno)
		} else if antAno > 0 {
			antLabel = fmt.Sprintf("Jan–%s/%d", mesNomes[refMes], antAno)
		}
		periodoLabel := curLabel
		if compMode == "ytd" {
			periodoLabel = fmt.Sprintf("Jan–%s/%d vs %d (projetado)", mesNomes[refMes], refAno, antAno)
		}

		// KPI geral (base de clientes do período)
		kpi := fetchMktKPI(db, spCtx.EmpresaID, refAno, refMes, compMode)

		// Cards do ranking
		var cards []mktCard
		switch view {
		case "produto":
			cards = fetchMktProduto(db, spCtx.EmpresaID, refAno, refMes, compMode, compAno, compMes, kpi.TotalBaseCli)
		case "cliente":
			cards = fetchMktCliente(db, spCtx.EmpresaID, refAno, refMes, compMode, compAno, compMes)
		case "fornec":
			cards = fetchMktFornec(db, spCtx.EmpresaID, refAno, refMes, compMode, compAno, compMes, kpi.TotalBaseCli)
		}
		if cards == nil {
			cards = []mktCard{}
		}

		// Clientes inativos (transmitidos mas não faturados no período)
		inativos := fetchClientesInativos(db, spCtx.EmpresaID, refAno, refMes)
		if inativos == nil {
			inativos = []mktClienteInativo{}
		}

		json.NewEncoder(w).Encode(mktResponse{
			Cards:            cards,
			KPI:              kpi,
			ClientesInativos: inativos,
			Periodo: mktPeriodoInfo{
				RefAno:   refAno,
				RefMes:   refMes,
				Label:    periodoLabel,
				CompMode: compMode,
				CurLabel: curLabel,
				AntLabel: antLabel,
			},
			Periodos: periodos,
			View:     view,
		})
	}
}

// ─── KPI geral ────────────────────────────────────────────────────────────────

func fetchMktKPI(db *sql.DB, empresaID string, refAno, refMes int, compMode string) mktKPI {
	t0 := time.Now()
	atualCond := fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes=%d", refAno, refMes)
	if compMode == "ytd" {
		atualCond = fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes<=%d", refAno, refMes)
	}

	row := db.QueryRow(fmt.Sprintf(`
		SELECT
		    COUNT(DISTINCT cod_cli)                                       AS total_base_cli,
		    COUNT(DISTINCT CASE WHEN positivados=1 THEN cod_cli END)      AS total_ativos,
		    COALESCE(AVG(CASE WHEN positivados=1 THEN mix END), 0)        AS avg_mix,
		    COALESCE(SUM(pvenda), 0)                                      AS total_pvenda,
		    COALESCE(SUM(faturado), 0)                                    AS total_faturado,
		    COALESCE(SUM(transmitido), 0)                                 AS total_transmitido
		FROM farol.mv_farol_cli
		WHERE empresa_id=$1 AND %s
	`, atualCond), empresaID)

	var k mktKPI
	_ = row.Scan(&k.TotalBaseCli, &k.TotalAtivos, &k.AvgMix, &k.TotalPvenda, &k.TotalFaturado, &k.TotalTransmitido)
	k.TotalInativos = k.TotalBaseCli - k.TotalAtivos
	if k.TotalBaseCli > 0 {
		k.TaxaPositivacao = float64(k.TotalAtivos) / float64(k.TotalBaseCli) * 100
	}
	log.Printf("[marketing] fetchMktKPI base=%d ativos=%d em %v", k.TotalBaseCli, k.TotalAtivos, time.Since(t0))
	return k
}

// ─── Por Produto ──────────────────────────────────────────────────────────────

func fetchMktProduto(db *sql.DB, empresaID string, refAno, refMes int, compMode string, compAno, compMes int, totalBase int) []mktCard {
	t0 := time.Now()

	// ── Query ATUAL ───────────────────────────────────────────────────────────
	var atualArgs []interface{} = []interface{}{empresaID}
	var atualWhere string
	switch compMode {
	case "ytd":
		atualArgs = append(atualArgs, refAno, refMes)
		atualWhere = "tipo_base='ATUAL' AND ano=$2 AND mes<=$3"
	default:
		atualArgs = append(atualArgs, refAno, refMes)
		atualWhere = "tipo_base='ATUAL' AND ano=$2 AND mes=$3"
	}

	rows, err := db.Query(fmt.Sprintf(`
		SELECT cod_prod, MAX(nome_prod), MAX(cod_fornec), MAX(nome_fornec),
		       SUM(qt_positivados), SUM(pvenda), SUM(faturado), SUM(transmitido)
		FROM farol.mv_mkt_produto
		WHERE empresa_id=$1 AND %s AND cod_prod != ''
		GROUP BY cod_prod
		ORDER BY SUM(qt_positivados) DESC
	`, atualWhere), atualArgs...)
	if err != nil {
		log.Printf("[marketing] fetchMktProduto ERRO atual: %v", err)
		return nil
	}
	defer rows.Close()

	type prodRow struct {
		key, label, fornec, nomeFornec string
		qtAtual                        int
		pvenda, faturado, transmitido  float64
	}
	byKey := map[string]*prodRow{}
	var order []string
	for rows.Next() {
		var p prodRow
		if err := rows.Scan(&p.key, &p.label, &p.fornec, &p.nomeFornec,
			&p.qtAtual, &p.pvenda, &p.faturado, &p.transmitido); err != nil {
			continue
		}
		byKey[p.key] = &p
		order = append(order, p.key)
	}

	// ── Query ANTERIOR (para delta) ───────────────────────────────────────────
	antAno, antMes := refAno-1, refMes
	if compMode == "mom" {
		antAno, antMes = resolveCompPeriod(refAno, refMes, compAno, compMes)
	}
	var antArgs []interface{} = []interface{}{empresaID}
	var antWhere string
	switch compMode {
	case "ytd":
		antArgs = append(antArgs, antAno, refMes)
		antWhere = "tipo_base='COMPARATIVA' AND ano=$2 AND mes<=$3"
	case "mom":
		antArgs = append(antArgs, antAno, antMes)
		antWhere = "(tipo_base='ATUAL' OR tipo_base='COMPARATIVA') AND ano=$2 AND mes=$3"
	default:
		antArgs = append(antArgs, antAno, antMes)
		antWhere = "tipo_base='COMPARATIVA' AND ano=$2 AND mes=$3"
	}

	antRows, err := db.Query(fmt.Sprintf(`
		SELECT cod_prod, SUM(qt_positivados)
		FROM farol.mv_mkt_produto
		WHERE empresa_id=$1 AND %s AND cod_prod != ''
		GROUP BY cod_prod
	`, antWhere), antArgs...)
	qtAntMap := map[string]int{}
	if err == nil {
		defer antRows.Close()
		for antRows.Next() {
			var k string
			var v int
			if err := antRows.Scan(&k, &v); err == nil {
				qtAntMap[k] = v
			}
		}
	}

	// ── Monta cards ───────────────────────────────────────────────────────────
	var cards []mktCard
	for _, k := range order {
		p := byKey[k]
		qtAnt := qtAntMap[k]
		c := mktCard{
			Key: p.key, Label: p.label, Fornec: p.fornec, NomeFornec: p.nomeFornec,
			QtClientes: p.qtAtual, QtCliAnt: qtAnt,
			Pvenda: p.pvenda, Faturado: p.faturado, Transmitido: p.transmitido,
		}
		if qtAnt > 0 {
			c.DeltaPct = float64(p.qtAtual-qtAnt) / float64(qtAnt) * 100
		}
		if totalBase > 0 {
			c.PenetrPct = float64(p.qtAtual) / float64(totalBase) * 100
		}
		cards = append(cards, c)
	}
	log.Printf("[marketing] fetchMktProduto %d produtos em %v", len(cards), time.Since(t0))
	return cards
}

// ─── Por Cliente ─────────────────────────────────────────────────────────────

func fetchMktCliente(db *sql.DB, empresaID string, refAno, refMes int, compMode string, compAno, compMes int) []mktCard {
	t0 := time.Now()
	atualCond := fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes=%d", refAno, refMes)
	if compMode == "ytd" {
		atualCond = fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes<=%d", refAno, refMes)
	}

	rows, err := db.Query(fmt.Sprintf(`
		SELECT
		    cod_cli,
		    MAX(nome_cli)                            AS nome,
		    MAX(positivados)                         AS comprou,
		    COALESCE(AVG(mix), 0)                    AS avg_mix,
		    COALESCE(SUM(pvenda), 0)                 AS pvenda,
		    COALESCE(SUM(faturado), 0)               AS faturado,
		    COALESCE(SUM(transmitido), 0)            AS transmitido
		FROM farol.mv_farol_cli
		WHERE empresa_id=$1 AND %s AND cod_cli != ''
		GROUP BY cod_cli
		ORDER BY avg_mix DESC, faturado DESC
	`, atualCond), empresaID)
	if err != nil {
		log.Printf("[marketing] fetchMktCliente ERRO: %v", err)
		return nil
	}
	defer rows.Close()

	var cards []mktCard
	for rows.Next() {
		var c mktCard
		var comprou int
		if err := rows.Scan(&c.Key, &c.Label, &comprou, &c.Mix, &c.Pvenda, &c.Faturado, &c.Transmitido); err != nil {
			continue
		}
		c.Positivado = comprou == 1
		c.QtClientes = comprou
		if comprou == 1 {
			c.PenetrPct = 100
		}
		cards = append(cards, c)
	}
	log.Printf("[marketing] fetchMktCliente %d clientes em %v", len(cards), time.Since(t0))
	return cards
}

// ─── Por Fornecedor/Indústria ────────────────────────────────────────────────

func fetchMktFornec(db *sql.DB, empresaID string, refAno, refMes int, compMode string, compAno, compMes int, totalBase int) []mktCard {
	t0 := time.Now()
	atualCond := fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes=%d", refAno, refMes)
	if compMode == "ytd" {
		atualCond = fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes<=%d", refAno, refMes)
	}

	rows, err := db.Query(fmt.Sprintf(`
		SELECT
		    cod_fornec,
		    MAX(nome_fornec)                          AS nome,
		    COALESCE(SUM(positivados), 0)             AS qt_ativos,
		    COALESCE(SUM(base_cli), 0)                AS base,
		    COALESCE(AVG(mix), 0)                     AS avg_mix,
		    COALESCE(SUM(pvenda), 0)                  AS pvenda,
		    COALESCE(SUM(faturado), 0)                AS faturado,
		    COALESCE(SUM(transmitido), 0)             AS transmitido
		FROM farol.mv_v01_l0
		WHERE empresa_id=$1 AND %s AND cod_fornec != ''
		GROUP BY cod_fornec
		ORDER BY qt_ativos DESC
	`, atualCond), empresaID)
	if err != nil {
		log.Printf("[marketing] fetchMktFornec ERRO: %v", err)
		return nil
	}
	defer rows.Close()

	var cards []mktCard
	for rows.Next() {
		var c mktCard
		var qtAtivos, base int
		if err := rows.Scan(&c.Key, &c.Label, &qtAtivos, &base, &c.Mix, &c.Pvenda, &c.Faturado, &c.Transmitido); err != nil {
			continue
		}
		c.QtClientes = qtAtivos
		if totalBase > 0 {
			c.PenetrPct = float64(qtAtivos) / float64(totalBase) * 100
		} else if base > 0 {
			c.PenetrPct = float64(qtAtivos) / float64(base) * 100
		}
		cards = append(cards, c)
	}
	log.Printf("[marketing] fetchMktFornec %d fornecedores em %v", len(cards), time.Since(t0))
	return cards
}

// ─── Clientes Inativos ───────────────────────────────────────────────────────
// Clientes que apareceram no período (dados transmitidos) mas não faturaram nada.

func fetchClientesInativos(db *sql.DB, empresaID string, refAno, refMes int) []mktClienteInativo {
	t0 := time.Now()
	rows, err := db.Query(`
		SELECT
		    cod_cli,
		    MAX(nome_cli)           AS nome,
		    COALESCE(SUM(pvenda), 0)     AS pvenda,
		    COALESCE(SUM(transmitido),0) AS transmitido
		FROM farol.mv_farol_cli
		WHERE empresa_id=$1
		    AND tipo_base='ATUAL' AND ano=$2 AND mes=$3
		    AND cod_cli != ''
		GROUP BY cod_cli
		HAVING MAX(positivados) = 0
		ORDER BY transmitido DESC
		LIMIT 100
	`, empresaID, refAno, refMes)
	if err != nil {
		log.Printf("[marketing] fetchClientesInativos ERRO: %v", err)
		return nil
	}
	defer rows.Close()

	var result []mktClienteInativo
	for rows.Next() {
		var c mktClienteInativo
		if err := rows.Scan(&c.Key, &c.Label, &c.Pvenda, &c.Transmitido); err != nil {
			continue
		}
		result = append(result, c)
	}
	log.Printf("[marketing] fetchClientesInativos %d inativos em %v", len(result), time.Since(t0))
	return result
}
