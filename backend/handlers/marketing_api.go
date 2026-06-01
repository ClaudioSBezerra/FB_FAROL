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
	TotalBaseCli      int     `json:"total_base_cli"`
	TotalAtivos       int     `json:"total_ativos"`
	TotalInativos     int     `json:"total_inativos"`
	TaxaPositivacao   float64 `json:"taxa_positivacao"`
	TaxaPositivacaoAnt float64 `json:"taxa_positivacao_ant"`
	DeltaPositivacao  float64 `json:"delta_positivacao"`
	AvgMix            float64 `json:"avg_mix"`
	TotalPvenda       float64 `json:"total_pvenda"`
	TotalFaturado     float64 `json:"total_faturado"`
	TotalTransmitido  float64 `json:"total_transmitido"`
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
		kpi := fetchMktKPI(db, spCtx.EmpresaID, refAno, refMes, compMode, compAno, compMes)

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

func fetchMktKPI(db *sql.DB, empresaID string, refAno, refMes int, compMode string, compAno, compMes int) mktKPI {
	t0 := time.Now()
	atualCond := fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes=%d", refAno, refMes)
	if compMode == "ytd" {
		atualCond = fmt.Sprintf("tipo_base='ATUAL' AND ano=%d AND mes<=%d", refAno, refMes)
	}

	// Subquery GROUP BY cod_cli primeiro (hash-agg), depois agrega — muito mais
	// rápido do que COUNT(DISTINCT) diretamente na tabela grande.
	row := db.QueryRow(fmt.Sprintf(`
		SELECT
		    COUNT(*)                                   AS total_base_cli,
		    COUNT(*) FILTER (WHERE max_pos = 1)        AS total_ativos,
		    COALESCE(AVG(CASE WHEN max_pos=1 THEN avg_mix END), 0) AS avg_mix,
		    COALESCE(SUM(pvenda), 0)                   AS total_pvenda,
		    COALESCE(SUM(faturado), 0)                 AS total_faturado,
		    COALESCE(SUM(transmitido), 0)              AS total_transmitido
		FROM (
		    SELECT cod_cli,
		           MAX(positivados)   AS max_pos,
		           AVG(mix)           AS avg_mix,
		           SUM(pvenda)        AS pvenda,
		           SUM(faturado)      AS faturado,
		           SUM(transmitido)   AS transmitido
		    FROM farol.mv_farol_cli
		    WHERE empresa_id=$1 AND %s
		    GROUP BY cod_cli
		) s
	`, atualCond), empresaID)

	var k mktKPI
	_ = row.Scan(&k.TotalBaseCli, &k.TotalAtivos, &k.AvgMix, &k.TotalPvenda, &k.TotalFaturado, &k.TotalTransmitido)
	k.TotalInativos = k.TotalBaseCli - k.TotalAtivos
	if k.TotalBaseCli > 0 {
		k.TaxaPositivacao = float64(k.TotalAtivos) / float64(k.TotalBaseCli) * 100
		if k.TaxaPositivacao > 100 { k.TaxaPositivacao = 100 }
	}

	// ── Período anterior (para delta de positivação no hero band) ──────────────
	antAno, antMes := refAno-1, refMes
	if compMode == "mom" {
		antAno, antMes = resolveCompPeriod(refAno, refMes, compAno, compMes)
	}
	var antCond string
	switch compMode {
	case "ytd":
		antCond = fmt.Sprintf("tipo_base='COMPARATIVA' AND ano=%d AND mes<=%d", antAno, refMes)
	case "mom":
		antCond = fmt.Sprintf("(tipo_base='ATUAL' OR tipo_base='COMPARATIVA') AND ano=%d AND mes=%d", antAno, antMes)
	default:
		antCond = fmt.Sprintf("tipo_base='COMPARATIVA' AND ano=%d AND mes=%d", antAno, antMes)
	}
	var baseAnt, ativosAnt int
	_ = db.QueryRow(fmt.Sprintf(`
		SELECT COUNT(*), COUNT(*) FILTER (WHERE max_pos=1)
		FROM (
		    SELECT cod_cli, MAX(positivados) AS max_pos
		    FROM farol.mv_farol_cli WHERE empresa_id=$1 AND %s
		    GROUP BY cod_cli
		) s
	`, antCond), empresaID).Scan(&baseAnt, &ativosAnt)
	if baseAnt > 0 {
		k.TaxaPositivacaoAnt = float64(ativosAnt) / float64(baseAnt) * 100
		if k.TaxaPositivacaoAnt > 100 { k.TaxaPositivacaoAnt = 100 }
	}
	k.DeltaPositivacao = k.TaxaPositivacao - k.TaxaPositivacaoAnt

	log.Printf("[marketing] fetchMktKPI base=%d ativos=%d posit=%.1f%% delta=%.1fpp em %v",
		k.TotalBaseCli, k.TotalAtivos, k.TaxaPositivacao, k.DeltaPositivacao, time.Since(t0))
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

	// mv_mkt_prod_pen: pré-agrega por cod_prod apenas (sem cod_fornec),
	// evitando dupla contagem. Refresh feito pelo "Consolidar view".
	rows, err := db.Query(fmt.Sprintf(`
		SELECT cod_prod, nome_prod, cod_fornec, nome_fornec,
		       qt_positivados, pvenda, faturado, transmitido
		FROM farol.mv_mkt_prod_pen
		WHERE empresa_id=$1 AND %s AND cod_prod != ''
		ORDER BY qt_positivados DESC
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
		SELECT cod_prod, MAX(qt_positivados)
		FROM farol.mv_mkt_prod_pen
		WHERE empresa_id=$1 AND %s
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
			if c.PenetrPct > 100 { c.PenetrPct = 100 }
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
		if c.PenetrPct > 100 { c.PenetrPct = 100 }
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

// ─── Produto Detalhe ─────────────────────────────────────────────────────────
// GET /api/v2/marketing/produto-detalhe?cod_prod=X&ref_ano=X&ref_mes=X&comp_mode=X
// Retorna compradores e oportunidades para um produto específico.

type prodDetalheKPI struct {
	TotalBase          int     `json:"total_base"`
	TotalCompradores   int     `json:"total_compradores"`
	TotalOportunidades int     `json:"total_oportunidades"`
	PenetrPct          float64 `json:"penetr_pct"`
	TotalFaturado      float64 `json:"total_faturado"`
	QtCliAnt           int     `json:"qt_cli_ant"`
	DeltaPct           float64 `json:"delta_pct"`
	PotencialEstimado  float64 `json:"potencial_estimado"`
}

type prodClienteItem struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Faturado    float64 `json:"faturado"`
	Transmitido float64 `json:"transmitido"`
	NomeSup     string  `json:"nome_sup"`
	NomeRca     string  `json:"nome_rca"`
}

type prodOportunidade struct {
	Key          string  `json:"key"`
	Label        string  `json:"label"`
	FaturadoTotal float64 `json:"faturado_total"`
	NomeRca      string  `json:"nome_rca"`
	NomeSup      string  `json:"nome_sup"`
	NOutrosProd  int     `json:"n_outros_prod"`
}

type prodDetalheResponse struct {
	CodProd       string            `json:"cod_prod"`
	NomeProd      string            `json:"nome_prod"`
	NomeFornec    string            `json:"nome_fornec"`
	KPI           prodDetalheKPI    `json:"kpi"`
	Compradores   []prodClienteItem `json:"compradores"`
	Oportunidades []prodOportunidade `json:"oportunidades"`
}

func MarketingProdutoDetalheHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()
		codProd := q.Get("cod_prod")
		if codProd == "" {
			http.Error(w, `{"error":"cod_prod obrigatório"}`, http.StatusBadRequest)
			return
		}
		refAno, _  := strconv.Atoi(q.Get("ref_ano"))
		refMes, _  := strconv.Atoi(q.Get("ref_mes"))
		compMode   := q.Get("comp_mode")
		if compMode == "" { compMode = "yoy" }
		compAno, _ := strconv.Atoi(q.Get("comp_ano"))
		compMes, _ := strconv.Atoi(q.Get("comp_mes"))

		if refAno == 0 || refMes == 0 {
			_ = db.QueryRow(`
				SELECT ano, mes FROM vendas_import_jobs
				WHERE empresa_id=$1 AND tipo_base='ATUAL' AND status='done'
				ORDER BY ano DESC, mes DESC LIMIT 1
			`, spCtx.EmpresaID).Scan(&refAno, &refMes)
		}

		t0 := time.Now()

		// ── Nome do produto e indústria ─────────────────────────────────────
		var nomeProd, nomeFornec string
		_ = db.QueryRow(`
			SELECT MAX(nome_prod), MAX(nome_fornec)
			FROM vendas_importadas
			WHERE empresa_id=$1 AND cod_prod=$2 AND tipo_base='ATUAL' AND ano=$3 AND mes=$4
		`, spCtx.EmpresaID, codProd, refAno, refMes).Scan(&nomeProd, &nomeFornec)

		// ── Base total de clientes no período ──────────────────────────────
		var totalBase int
		_ = db.QueryRow(`
			SELECT COUNT(DISTINCT cod_cli) FROM farol.mv_farol_cli
			WHERE empresa_id=$1 AND tipo_base='ATUAL' AND ano=$2 AND mes=$3
		`, spCtx.EmpresaID, refAno, refMes).Scan(&totalBase)

		// ── Compradores (clientes que compraram este produto) ──────────────
		rows, err := db.Query(`
			SELECT
			    cod_cli,
			    MAX(nome_cli)                                                    AS nome,
			    MAX(COALESCE(nome_supervisor,''))                                AS nome_sup,
			    MAX(COALESCE(nome_rca,''))                                       AS nome_rca,
			    SUM(CASE WHEN estado='FATURADO'  THEN pvenda ELSE 0 END)        AS faturado,
			    SUM(CASE WHEN estado!='FATURADO' THEN pvenda ELSE 0 END)        AS transmitido
			FROM vendas_importadas
			WHERE empresa_id=$1 AND tipo_base='ATUAL' AND ano=$2 AND mes=$3
			    AND cod_prod=$4 AND qt > 0
			GROUP BY cod_cli
			ORDER BY SUM(CASE WHEN estado='FATURADO' THEN pvenda ELSE 0 END) DESC
		`, spCtx.EmpresaID, refAno, refMes, codProd)

		compradores := []prodClienteItem{}
		compradoresSet := map[string]bool{}
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var c prodClienteItem
				if err := rows.Scan(&c.Key, &c.Label, &c.NomeSup, &c.NomeRca,
					&c.Faturado, &c.Transmitido); err == nil {
					compradores = append(compradores, c)
					compradoresSet[c.Key] = true
				}
			}
		} else {
			log.Printf("[marketing:detalhe] compradores ERRO: %v", err)
		}

		// ── Período anterior (para delta) ──────────────────────────────────
		antAno, antMes := refAno-1, refMes
		if compMode == "mom" {
			antAno, antMes = resolveCompPeriod(refAno, refMes, compAno, compMes)
		}
		antTipoCond := "tipo_base='COMPARATIVA'"
		if compMode == "mom" {
			antTipoCond = "(tipo_base='ATUAL' OR tipo_base='COMPARATIVA')"
		}
		var qtCliAnt int
		_ = db.QueryRow(fmt.Sprintf(`
			SELECT COUNT(DISTINCT cod_cli) FROM vendas_importadas
			WHERE empresa_id=$1 AND %s AND ano=$2 AND mes=$3 AND cod_prod=$4 AND qt > 0
		`, antTipoCond), spCtx.EmpresaID, antAno, antMes, codProd).Scan(&qtCliAnt)

		// ── Oportunidades (ativos que NÃO compraram este produto) ──────────
		// Busca clientes positivados no período, exclui quem já comprou o produto
		oRows, err2 := db.Query(`
			SELECT
			    f.cod_cli,
			    MAX(f.nome_cli)                AS nome,
			    SUM(f.faturado)                AS faturado_total,
			    MAX(f.nome_supervisor)         AS nome_sup,
			    MAX(f.nome_rca)                AS nome_rca,
			    COUNT(DISTINCT f.cod_fornec)   AS n_outros
			FROM farol.mv_farol_cli f
			WHERE f.empresa_id=$1 AND f.tipo_base='ATUAL' AND f.ano=$2 AND f.mes=$3
			    AND f.positivados=1 AND f.cod_cli != ''
			    AND NOT EXISTS (
			        SELECT 1 FROM vendas_importadas v
			        WHERE v.empresa_id=f.empresa_id AND v.tipo_base=f.tipo_base
			            AND v.ano=f.ano AND v.mes=f.mes
			            AND v.cod_cli=f.cod_cli AND v.cod_prod=$4 AND v.qt > 0
			    )
			GROUP BY f.cod_cli
			ORDER BY SUM(f.faturado) DESC
			LIMIT 200
		`, spCtx.EmpresaID, refAno, refMes, codProd)

		oportunidades := []prodOportunidade{}
		var potencialEstimado float64
		if err2 == nil {
			defer oRows.Close()
			for oRows.Next() {
				var o prodOportunidade
				if err := oRows.Scan(&o.Key, &o.Label, &o.FaturadoTotal,
					&o.NomeSup, &o.NomeRca, &o.NOutrosProd); err == nil {
					oportunidades = append(oportunidades, o)
					potencialEstimado += o.FaturadoTotal
				}
			}
		} else {
			log.Printf("[marketing:detalhe] oportunidades ERRO: %v", err2)
		}

		// ── KPI ────────────────────────────────────────────────────────────
		var totalFaturado float64
		for _, c := range compradores {
			totalFaturado += c.Faturado
		}
		nComp := len(compradores)
		var penetrPct float64
		if totalBase > 0 {
			penetrPct = float64(nComp) / float64(totalBase) * 100
		}
		var deltaPct float64
		if qtCliAnt > 0 {
			deltaPct = float64(nComp-qtCliAnt) / float64(qtCliAnt) * 100
		}

		log.Printf("[marketing:detalhe] prod=%s comp=%d opor=%d em %v",
			codProd, nComp, len(oportunidades), time.Since(t0))

		json.NewEncoder(w).Encode(prodDetalheResponse{
			CodProd:    codProd,
			NomeProd:   nomeProd,
			NomeFornec: nomeFornec,
			KPI: prodDetalheKPI{
				TotalBase:          totalBase,
				TotalCompradores:   nComp,
				TotalOportunidades: len(oportunidades),
				PenetrPct:          penetrPct,
				TotalFaturado:      totalFaturado,
				QtCliAnt:           qtCliAnt,
				DeltaPct:           deltaPct,
				PotencialEstimado:  potencialEstimado,
			},
			Compradores:   compradores,
			Oportunidades: oportunidades,
		})
	}
}

// ─── Cliente Detalhe ──────────────────────────────────────────────────────────
// GET /api/v2/marketing/cliente-detalhe?cod_cli=X&ref_ano=X&ref_mes=X
// Retorna produtos comprados e produtos não comprados (cross-sell).

type cliDetalheKPI struct {
	TotalProdutos      int     `json:"total_produtos"`
	TotalOportunidades int     `json:"total_oportunidades"`
	TotalFaturado      float64 `json:"total_faturado"`
	TotalTransmitido   float64 `json:"total_transmitido"`
	NomeRca            string  `json:"nome_rca"`
	NomeSup            string  `json:"nome_sup"`
	PotencialEstimado  float64 `json:"potencial_estimado"`
}

type cliProdutoItem struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	NomeFornec  string  `json:"nome_fornec"`
	Faturado    float64 `json:"faturado"`
	Transmitido float64 `json:"transmitido"`
}

type cliOportunidade struct {
	Key              string  `json:"key"`
	Label            string  `json:"label"`
	NomeFornec       string  `json:"nome_fornec"`
	QtOutrosClientes int     `json:"qt_outros_clientes"`
	TicketMedio      float64 `json:"ticket_medio"`
	PenetrPct        float64 `json:"penetr_pct"`
}

type cliDetalheResponse struct {
	CodCli        string            `json:"cod_cli"`
	NomeCli       string            `json:"nome_cli"`
	KPI           cliDetalheKPI     `json:"kpi"`
	Comprados     []cliProdutoItem  `json:"comprados"`
	Oportunidades []cliOportunidade `json:"oportunidades"`
}

func MarketingClienteDetalheHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		codCli := q.Get("cod_cli")
		if codCli == "" {
			http.Error(w, `{"error":"cod_cli obrigatório"}`, http.StatusBadRequest)
			return
		}
		refAno, _ := strconv.Atoi(q.Get("ref_ano"))
		refMes, _ := strconv.Atoi(q.Get("ref_mes"))
		if refAno == 0 || refMes == 0 {
			_ = db.QueryRow(`SELECT ano, mes FROM vendas_import_jobs
				WHERE empresa_id=$1 AND tipo_base='ATUAL' AND status='done'
				ORDER BY ano DESC, mes DESC LIMIT 1`,
				spCtx.EmpresaID).Scan(&refAno, &refMes)
		}
		t0 := time.Now()

		// ── Produtos comprados por este cliente ────────────────────────────
		rows, err := db.Query(`
			SELECT
			    cod_prod,
			    MAX(nome_prod)                                             AS nome,
			    MAX(COALESCE(nome_fornec,''))                              AS nome_fornec,
			    SUM(CASE WHEN estado='FATURADO'  THEN pvenda ELSE 0 END)  AS faturado,
			    SUM(CASE WHEN estado!='FATURADO' THEN pvenda ELSE 0 END)  AS transmitido
			FROM vendas_importadas
			WHERE empresa_id=$1 AND tipo_base='ATUAL' AND ano=$2 AND mes=$3
			    AND cod_cli=$4 AND qt > 0
			GROUP BY cod_prod
			ORDER BY SUM(CASE WHEN estado='FATURADO' THEN pvenda ELSE 0 END) DESC
		`, spCtx.EmpresaID, refAno, refMes, codCli)

		comprados := []cliProdutoItem{}
		var totalFaturado, totalTransmitido float64
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var c cliProdutoItem
				if err := rows.Scan(&c.Key, &c.Label, &c.NomeFornec, &c.Faturado, &c.Transmitido); err == nil {
					comprados = append(comprados, c)
					totalFaturado += c.Faturado
					totalTransmitido += c.Transmitido
				}
			}
		}

		// Nome do cliente, RCA e supervisor
		var nomeCli, nomeRca, nomeSup string
		_ = db.QueryRow(`
			SELECT MAX(nome_cli), MAX(COALESCE(nome_rca,'')), MAX(COALESCE(nome_supervisor,''))
			FROM farol.mv_farol_cli
			WHERE empresa_id=$1 AND tipo_base='ATUAL' AND ano=$2 AND mes=$3 AND cod_cli=$4
		`, spCtx.EmpresaID, refAno, refMes, codCli).Scan(&nomeCli, &nomeRca, &nomeSup)

		// Base total de clientes (para penetrPct das oportunidades)
		var totalBase int
		_ = db.QueryRow(`SELECT COUNT(DISTINCT cod_cli) FROM farol.mv_farol_cli
			WHERE empresa_id=$1 AND tipo_base='ATUAL' AND ano=$2 AND mes=$3`,
			spCtx.EmpresaID, refAno, refMes).Scan(&totalBase)

		// ── Oportunidades: produtos mais populares que este cliente não compra ──
		oRows, err2 := db.Query(`
			SELECT
			    p.cod_prod,
			    MAX(p.nome_prod)    AS nome,
			    MAX(p.nome_fornec)  AS nome_fornec,
			    SUM(p.qt_positivados) AS qt_outros,
			    CASE WHEN SUM(p.qt_positivados) > 0
			         THEN SUM(p.faturado) / SUM(p.qt_positivados)
			         ELSE 0 END    AS ticket_medio
			FROM farol.mv_mkt_produto p
			WHERE p.empresa_id=$1 AND p.tipo_base='ATUAL' AND p.ano=$2 AND p.mes=$3
			    AND p.cod_prod != ''
			    AND NOT EXISTS (
			        SELECT 1 FROM vendas_importadas v
			        WHERE v.empresa_id=$1 AND v.tipo_base='ATUAL' AND v.ano=$2 AND v.mes=$3
			            AND v.cod_cli=$4 AND v.cod_prod=p.cod_prod AND v.qt > 0
			    )
			GROUP BY p.cod_prod
			ORDER BY SUM(p.qt_positivados) DESC
			LIMIT 100
		`, spCtx.EmpresaID, refAno, refMes, codCli)

		oportunidades := []cliOportunidade{}
		var potencialEstimado float64
		if err2 == nil {
			defer oRows.Close()
			for oRows.Next() {
				var o cliOportunidade
				if err := oRows.Scan(&o.Key, &o.Label, &o.NomeFornec,
					&o.QtOutrosClientes, &o.TicketMedio); err == nil {
					if totalBase > 0 {
						o.PenetrPct = float64(o.QtOutrosClientes) / float64(totalBase) * 100
					}
					oportunidades = append(oportunidades, o)
					potencialEstimado += o.TicketMedio
				}
			}
		}

		log.Printf("[marketing:cli-detalhe] cli=%s comprados=%d opor=%d em %v",
			codCli, len(comprados), len(oportunidades), time.Since(t0))

		json.NewEncoder(w).Encode(cliDetalheResponse{
			CodCli:  codCli,
			NomeCli: nomeCli,
			KPI: cliDetalheKPI{
				TotalProdutos:      len(comprados),
				TotalOportunidades: len(oportunidades),
				TotalFaturado:      totalFaturado,
				TotalTransmitido:   totalTransmitido,
				NomeRca:            nomeRca,
				NomeSup:            nomeSup,
				PotencialEstimado:  potencialEstimado,
			},
			Comprados:     comprados,
			Oportunidades: oportunidades,
		})
	}
}
