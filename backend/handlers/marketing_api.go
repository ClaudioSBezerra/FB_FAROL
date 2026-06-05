package handlers

// marketing_api.go — Painel Marketing (granularidade diária).
//
// GET /api/v2/marketing/cards
//   Parâmetros (nova API):
//     view         produto | cliente | fornec  (default: produto)
//     fluxo        faturado | transmitido      (default: faturado)
//     ref_inicio   YYYY-MM-DD
//     ref_fim      YYYY-MM-DD
//     comp_inicio  YYYY-MM-DD                  (opcional)
//     comp_fim     YYYY-MM-DD
//
//   Retrocompat:
//     ref_ano + ref_mes                        → vira ref_inicio/ref_fim
//     comp_mode = yoy|mom|ytd                  → deriva comp_inicio/comp_fim
//     comp_ano + comp_mes                      → idem (mom override)
//
// "Clientes inativos" — clientes que transmitiram pedidos mas NÃO faturaram
// no mesmo período. Cross-query entre vendas_transmitidas e vendas_faturadas.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ─── Tipos de resposta ────────────────────────────────────────────────────────

type mktCard struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Fornec      string  `json:"fornec,omitempty"`
	NomeFornec  string  `json:"nome_fornec,omitempty"`
	Ean         string  `json:"ean,omitempty"`
	QtClientes  int     `json:"qt_clientes"` // clientes positivados no período
	QtCliAnt    int     `json:"qt_cli_ant"`
	DeltaPct    float64 `json:"delta_pct"`
	Pvenda      float64 `json:"pvenda"`
	Faturado    float64 `json:"faturado"`    // = pvenda se fluxo=faturado, 0 se trans
	Transmitido float64 `json:"transmitido"` // = pvenda se fluxo=transmitido, 0 se fat
	Plucro      float64 `json:"plucro"`
	PenetrPct   float64 `json:"penetr_pct"`
	Mix         float64 `json:"mix"`
	Positivado  bool    `json:"positivado"`
}

type mktKPI struct {
	TotalBaseCli       int     `json:"total_base_cli"`
	TotalAtivos        int     `json:"total_ativos"`
	TotalInativos      int     `json:"total_inativos"`
	TaxaPositivacao    float64 `json:"taxa_positivacao"`
	TaxaPositivacaoAnt float64 `json:"taxa_positivacao_ant"`
	DeltaPositivacao   float64 `json:"delta_positivacao"`
	AvgMix             float64 `json:"avg_mix"`
	TotalPvenda        float64 `json:"total_pvenda"`
	TotalFaturado      float64 `json:"total_faturado"`
	TotalTransmitido   float64 `json:"total_transmitido"`
	TotalPlucro        float64 `json:"total_plucro"`
}

type mktClienteInativo struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Pvenda      float64 `json:"pvenda"`
	Transmitido float64 `json:"transmitido"`
}

type mktResponse struct {
	Cards            []mktCard           `json:"cards"`
	KPI              mktKPI              `json:"kpi"`
	ClientesInativos []mktClienteInativo `json:"clientes_inativos"`
	Periodo          periodoInfo         `json:"periodo"`
	Periodos         []string            `json:"periodos"`
	View             string              `json:"view"`
}

// MV de marketing por fluxo (penetração de produto, agregação por produto+fornec)
func mvMktProdPen(fluxo fluxoCtx) string {
	if fluxo.name == "transmitido" {
		return "farol.mv_trans_mkt_prod_pen"
	}
	return "farol.mv_fat_mkt_prod_pen"
}

func mvMktProduto(fluxo fluxoCtx) string {
	if fluxo.name == "transmitido" {
		return "farol.mv_trans_mkt_produto"
	}
	return "farol.mv_fat_mkt_produto"
}

func mvV01L0(fluxo fluxoCtx) string {
	if fluxo.name == "transmitido" {
		return "farol.mv_trans_v01_l0"
	}
	return "farol.mv_fat_v01_l0"
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
		fluxo := resolveFluxo(q.Get("fluxo"))
		pr := resolvePeriods(db, spCtx.EmpresaID, q)

		if pr.RefInicio.IsZero() {
			json.NewEncoder(w).Encode(mktResponse{
				Cards: []mktCard{}, ClientesInativos: []mktClienteInativo{},
				Periodos: []string{}, View: view,
				Periodo: periodoInfo{Fluxo: fluxo.name},
			})
			return
		}

		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		curLabel, antLabel, plabel := buildPeriodoLabels(pr)

		kpi := fetchMktKPI(db, spCtx.EmpresaID, fluxo, pr)

		var cards []mktCard
		switch view {
		case "produto":
			cards = fetchMktProduto(db, spCtx.EmpresaID, fluxo, pr, kpi.TotalBaseCli)
		case "cliente":
			cards = fetchMktCliente(db, spCtx.EmpresaID, fluxo, pr)
		case "fornec":
			cards = fetchMktFornec(db, spCtx.EmpresaID, fluxo, pr, kpi.TotalBaseCli)
		}
		if cards == nil {
			cards = []mktCard{}
		}

		inativos := fetchClientesInativos(db, spCtx.EmpresaID, pr)
		if inativos == nil {
			inativos = []mktClienteInativo{}
		}

		json.NewEncoder(w).Encode(mktResponse{
			Cards:            cards,
			KPI:              kpi,
			ClientesInativos: inativos,
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
			Periodos: periodos,
			View:     view,
		})
	}
}

// ─── KPI geral ────────────────────────────────────────────────────────────────

func fetchMktKPI(db *sql.DB, empresaID string, fluxo fluxoCtx, pr periodResolution) mktKPI {
	t0 := time.Now()
	baseView := fluxo.baseView
	dateCol := fluxo.dateCol

	row := db.QueryRow(fmt.Sprintf(`
		SELECT
		    COUNT(*)                                                      AS total_base_cli,
		    COUNT(*) FILTER (WHERE max_pos = 1)                          AS total_ativos,
		    COALESCE(AVG(CASE WHEN max_pos=1 THEN avg_mix END), 0)       AS avg_mix,
		    COALESCE(SUM(pvenda), 0)                                     AS total_pvenda,
		    COALESCE(SUM(plucro), 0)                                     AS total_plucro
		FROM (
		    SELECT cod_cli,
		           MAX(positivados)   AS max_pos,
		           AVG(mix)           AS avg_mix,
		           SUM(pvenda)        AS pvenda,
		           SUM(plucro)        AS plucro
		    FROM %s
		    WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
		    GROUP BY cod_cli
		) s
	`, baseView, dateCol), empresaID, pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"))

	var k mktKPI
	_ = row.Scan(&k.TotalBaseCli, &k.TotalAtivos, &k.AvgMix, &k.TotalPvenda, &k.TotalPlucro)
	k.TotalInativos = k.TotalBaseCli - k.TotalAtivos
	if fluxo.name == "transmitido" {
		k.TotalTransmitido = k.TotalPvenda
	} else {
		k.TotalFaturado = k.TotalPvenda
	}
	if k.TotalBaseCli > 0 {
		k.TaxaPositivacao = float64(k.TotalAtivos) / float64(k.TotalBaseCli) * 100
		if k.TaxaPositivacao > 100 {
			k.TaxaPositivacao = 100
		}
	}

	// Período anterior (delta de positivação no hero band)
	if !pr.CompInicio.IsZero() && !pr.CompFim.IsZero() {
		var baseAnt, ativosAnt int
		_ = db.QueryRow(fmt.Sprintf(`
			SELECT COUNT(*), COUNT(*) FILTER (WHERE max_pos=1)
			FROM (
			    SELECT cod_cli, MAX(positivados) AS max_pos
			    FROM %s WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
			    GROUP BY cod_cli
			) s
		`, baseView, dateCol), empresaID,
			pr.CompInicio.Format("2006-01-02"), pr.CompFim.Format("2006-01-02"),
		).Scan(&baseAnt, &ativosAnt)
		if baseAnt > 0 {
			k.TaxaPositivacaoAnt = float64(ativosAnt) / float64(baseAnt) * 100
			if k.TaxaPositivacaoAnt > 100 {
				k.TaxaPositivacaoAnt = 100
			}
		}
		k.DeltaPositivacao = k.TaxaPositivacao - k.TaxaPositivacaoAnt
	}

	log.Printf("[marketing] fetchMktKPI fluxo=%s base=%d ativos=%d posit=%.1f%% delta=%.1fpp em %v",
		fluxo.name, k.TotalBaseCli, k.TotalAtivos, k.TaxaPositivacao, k.DeltaPositivacao, time.Since(t0))
	return k
}

// ─── Por Produto ──────────────────────────────────────────────────────────────

func fetchMktProduto(db *sql.DB, empresaID string, fluxo fluxoCtx, pr periodResolution, totalBase int) []mktCard {
	t0 := time.Now()
	mv := mvMktProdPen(fluxo)
	dateCol := fluxo.dateCol

	rows, err := db.Query(fmt.Sprintf(`
		SELECT cod_prod, nome_prod, cod_fornec, nome_fornec, ean,
		       qt_positivados, pvenda, plucro
		FROM %s
		WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date AND cod_prod != ''
		ORDER BY qt_positivados DESC
	`, mv, dateCol), empresaID, pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"))
	if err != nil {
		log.Printf("[marketing] fetchMktProduto ERRO atual: %v", err)
		return nil
	}
	defer rows.Close()

	type prodRow struct {
		key, label, fornec, nomeFornec, ean string
		qtAtual                             int
		pvenda, plucro                      float64
	}
	byKey := map[string]*prodRow{}
	var order []string
	for rows.Next() {
		var p prodRow
		if err := rows.Scan(&p.key, &p.label, &p.fornec, &p.nomeFornec, &p.ean,
			&p.qtAtual, &p.pvenda, &p.plucro); err != nil {
			continue
		}
		byKey[p.key] = &p
		order = append(order, p.key)
	}

	// Período anterior (delta) — só se houver comp definido
	qtAntMap := map[string]int{}
	if !pr.CompInicio.IsZero() && !pr.CompFim.IsZero() {
		antRows, err := db.Query(fmt.Sprintf(`
			SELECT cod_prod, MAX(qt_positivados)
			FROM %s
			WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
			GROUP BY cod_prod
		`, mv, dateCol), empresaID,
			pr.CompInicio.Format("2006-01-02"), pr.CompFim.Format("2006-01-02"))
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
	}

	var cards []mktCard
	for _, k := range order {
		p := byKey[k]
		qtAnt := qtAntMap[k]
		c := mktCard{
			Key: p.key, Label: p.label,
			Fornec: p.fornec, NomeFornec: p.nomeFornec, Ean: p.ean,
			QtClientes: p.qtAtual, QtCliAnt: qtAnt,
			Pvenda: p.pvenda, Plucro: p.plucro,
		}
		if fluxo.name == "transmitido" {
			c.Transmitido = p.pvenda
		} else {
			c.Faturado = p.pvenda
		}
		if qtAnt > 0 {
			c.DeltaPct = float64(p.qtAtual-qtAnt) / float64(qtAnt) * 100
		}
		if totalBase > 0 {
			c.PenetrPct = float64(p.qtAtual) / float64(totalBase) * 100
			if c.PenetrPct > 100 {
				c.PenetrPct = 100
			}
		}
		cards = append(cards, c)
	}
	log.Printf("[marketing] fetchMktProduto fluxo=%s %d produtos em %v", fluxo.name, len(cards), time.Since(t0))
	return cards
}

// ─── Por Cliente ─────────────────────────────────────────────────────────────

func fetchMktCliente(db *sql.DB, empresaID string, fluxo fluxoCtx, pr periodResolution) []mktCard {
	t0 := time.Now()
	baseView := fluxo.baseView
	dateCol := fluxo.dateCol

	rows, err := db.Query(fmt.Sprintf(`
		SELECT
		    cod_cli,
		    MAX(nome_cli)               AS nome,
		    MAX(positivados)            AS comprou,
		    COALESCE(AVG(mix), 0)       AS avg_mix,
		    COALESCE(SUM(pvenda), 0)    AS pvenda,
		    COALESCE(SUM(plucro), 0)    AS plucro
		FROM %s
		WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date AND cod_cli != ''
		GROUP BY cod_cli
		ORDER BY avg_mix DESC, pvenda DESC
	`, baseView, dateCol), empresaID,
		pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"))
	if err != nil {
		log.Printf("[marketing] fetchMktCliente ERRO: %v", err)
		return nil
	}
	defer rows.Close()

	var cards []mktCard
	for rows.Next() {
		var c mktCard
		var comprou int
		if err := rows.Scan(&c.Key, &c.Label, &comprou, &c.Mix, &c.Pvenda, &c.Plucro); err != nil {
			continue
		}
		c.Positivado = comprou == 1
		c.QtClientes = comprou
		if comprou == 1 {
			c.PenetrPct = 100
		}
		if fluxo.name == "transmitido" {
			c.Transmitido = c.Pvenda
		} else {
			c.Faturado = c.Pvenda
		}
		cards = append(cards, c)
	}
	log.Printf("[marketing] fetchMktCliente fluxo=%s %d clientes em %v", fluxo.name, len(cards), time.Since(t0))
	return cards
}

// ─── Por Fornecedor/Indústria ────────────────────────────────────────────────

func fetchMktFornec(db *sql.DB, empresaID string, fluxo fluxoCtx, pr periodResolution, totalBase int) []mktCard {
	t0 := time.Now()
	mv := mvV01L0(fluxo)
	dateCol := fluxo.dateCol

	rows, err := db.Query(fmt.Sprintf(`
		SELECT
		    cod_fornec,
		    MAX(nome_fornec)            AS nome,
		    COALESCE(SUM(positivados),0) AS qt_ativos,
		    COALESCE(SUM(base_cli),0)    AS base,
		    COALESCE(AVG(mix),0)         AS avg_mix,
		    COALESCE(SUM(pvenda),0)      AS pvenda,
		    COALESCE(SUM(plucro),0)      AS plucro
		FROM %s
		WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date AND cod_fornec != ''
		GROUP BY cod_fornec
		ORDER BY qt_ativos DESC
	`, mv, dateCol), empresaID,
		pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"))
	if err != nil {
		log.Printf("[marketing] fetchMktFornec ERRO: %v", err)
		return nil
	}
	defer rows.Close()

	var cards []mktCard
	for rows.Next() {
		var c mktCard
		var qtAtivos, base int
		if err := rows.Scan(&c.Key, &c.Label, &qtAtivos, &base, &c.Mix, &c.Pvenda, &c.Plucro); err != nil {
			continue
		}
		c.QtClientes = qtAtivos
		if totalBase > 0 {
			c.PenetrPct = float64(qtAtivos) / float64(totalBase) * 100
		} else if base > 0 {
			c.PenetrPct = float64(qtAtivos) / float64(base) * 100
		}
		if c.PenetrPct > 100 {
			c.PenetrPct = 100
		}
		if fluxo.name == "transmitido" {
			c.Transmitido = c.Pvenda
		} else {
			c.Faturado = c.Pvenda
		}
		cards = append(cards, c)
	}
	log.Printf("[marketing] fetchMktFornec fluxo=%s %d fornecedores em %v", fluxo.name, len(cards), time.Since(t0))
	return cards
}

// ─── Clientes Inativos ───────────────────────────────────────────────────────
// Clientes que transmitiram pedidos no período mas NÃO faturaram.
// Cross-query entre as duas bases.

func fetchClientesInativos(db *sql.DB, empresaID string, pr periodResolution) []mktClienteInativo {
	t0 := time.Now()
	rows, err := db.Query(`
		SELECT
		    t.cod_cli,
		    MAX(t.nome_cli)              AS nome,
		    COALESCE(SUM(t.pvenda), 0)   AS transmitido
		FROM farol.mv_trans_cli t
		WHERE t.empresa_id=$1
		  AND t.data_transmissao BETWEEN $2::date AND $3::date
		  AND t.cod_cli != ''
		GROUP BY t.cod_cli
		HAVING NOT EXISTS (
		    SELECT 1 FROM farol.mv_fat_cli f
		     WHERE f.empresa_id=$1 AND f.cod_cli=t.cod_cli
		       AND f.data_faturamento BETWEEN $2::date AND $3::date
		       AND f.positivados=1
		)
		ORDER BY transmitido DESC
		LIMIT 100
	`, empresaID, pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"))
	if err != nil {
		log.Printf("[marketing] fetchClientesInativos ERRO: %v", err)
		return nil
	}
	defer rows.Close()

	var result []mktClienteInativo
	for rows.Next() {
		var c mktClienteInativo
		if err := rows.Scan(&c.Key, &c.Label, &c.Transmitido); err != nil {
			continue
		}
		c.Pvenda = c.Transmitido
		result = append(result, c)
	}
	log.Printf("[marketing] fetchClientesInativos %d inativos em %v", len(result), time.Since(t0))
	return result
}

// ─── Produto Detalhe ─────────────────────────────────────────────────────────

type prodDetalheKPI struct {
	TotalBase          int     `json:"total_base"`
	TotalCompradores   int     `json:"total_compradores"`
	TotalOportunidades int     `json:"total_oportunidades"`
	PenetrPct          float64 `json:"penetr_pct"`
	TotalFaturado      float64 `json:"total_faturado"`
	TotalPlucro        float64 `json:"total_plucro"`
	QtCliAnt           int     `json:"qt_cli_ant"`
	DeltaPct           float64 `json:"delta_pct"`
	PotencialEstimado  float64 `json:"potencial_estimado"`
}

type prodClienteItem struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Faturado    float64 `json:"faturado"`
	Transmitido float64 `json:"transmitido"`
	Plucro      float64 `json:"plucro"`
	NomeSup     string  `json:"nome_sup"`
	NomeRca     string  `json:"nome_rca"`
}

type prodOportunidade struct {
	Key           string  `json:"key"`
	Label         string  `json:"label"`
	FaturadoTotal float64 `json:"faturado_total"`
	NomeRca       string  `json:"nome_rca"`
	NomeSup       string  `json:"nome_sup"`
	NOutrosProd   int     `json:"n_outros_prod"`
}

type prodDetalheResponse struct {
	CodProd       string             `json:"cod_prod"`
	NomeProd      string             `json:"nome_prod"`
	NomeFornec    string             `json:"nome_fornec"`
	Ean           string             `json:"ean,omitempty"`
	KPI           prodDetalheKPI     `json:"kpi"`
	Compradores   []prodClienteItem  `json:"compradores"`
	Oportunidades []prodOportunidade `json:"oportunidades"`
	Periodo       periodoInfo        `json:"periodo"`
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
		fluxo := resolveFluxo(q.Get("fluxo"))
		pr := resolvePeriods(db, spCtx.EmpresaID, q)
		if pr.RefInicio.IsZero() {
			json.NewEncoder(w).Encode(prodDetalheResponse{CodProd: codProd})
			return
		}

		t0 := time.Now()
		refIni := pr.RefInicio.Format("2006-01-02")
		refFim := pr.RefFim.Format("2006-01-02")

		// Nome do produto, indústria e EAN — pega da tabela base do fluxo
		var nomeProd, nomeFornec, ean string
		_ = db.QueryRow(fmt.Sprintf(`
			SELECT MAX(nome_prod), MAX(nome_fornec), MAX(ean)
			FROM %s
			WHERE empresa_id=$1 AND cod_prod=$2 AND %s BETWEEN $3::date AND $4::date
		`, fluxo.tableName, fluxo.dateCol),
			spCtx.EmpresaID, codProd, refIni, refFim).Scan(&nomeProd, &nomeFornec, &ean)

		// Base total de clientes no período (fluxo)
		var totalBase int
		_ = db.QueryRow(fmt.Sprintf(`
			SELECT COUNT(DISTINCT cod_cli) FROM %s
			WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
		`, fluxo.baseView, fluxo.dateCol),
			spCtx.EmpresaID, refIni, refFim).Scan(&totalBase)

		// Compradores
		rows, err := db.Query(fmt.Sprintf(`
			SELECT
			    cod_cli,
			    MAX(nome_cli)                    AS nome,
			    MAX(COALESCE(nome_supervisor,'')) AS nome_sup,
			    MAX(COALESCE(nome_rca,''))        AS nome_rca,
			    COALESCE(SUM(pvenda),0)           AS pvenda,
			    COALESCE(SUM(plucro),0)           AS plucro
			FROM %s
			WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
			    AND cod_prod=$4 AND qt > 0
			GROUP BY cod_cli
			ORDER BY pvenda DESC
		`, fluxo.tableName, fluxo.dateCol),
			spCtx.EmpresaID, refIni, refFim, codProd)

		compradores := []prodClienteItem{}
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var c prodClienteItem
				var pv float64
				if err := rows.Scan(&c.Key, &c.Label, &c.NomeSup, &c.NomeRca,
					&pv, &c.Plucro); err == nil {
					if fluxo.name == "transmitido" {
						c.Transmitido = pv
					} else {
						c.Faturado = pv
					}
					compradores = append(compradores, c)
				}
			}
		} else {
			log.Printf("[marketing:detalhe] compradores ERRO: %v", err)
		}

		// Comparativo — qt clientes que compraram no período anterior
		var qtCliAnt int
		if !pr.CompInicio.IsZero() && !pr.CompFim.IsZero() {
			_ = db.QueryRow(fmt.Sprintf(`
				SELECT COUNT(DISTINCT cod_cli) FROM %s
				WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
				    AND cod_prod=$4 AND qt > 0
			`, fluxo.tableName, fluxo.dateCol),
				spCtx.EmpresaID,
				pr.CompInicio.Format("2006-01-02"), pr.CompFim.Format("2006-01-02"),
				codProd).Scan(&qtCliAnt)
		}

		// Oportunidades — clientes positivados no período que NÃO compraram este produto
		oRows, err2 := db.Query(fmt.Sprintf(`
			SELECT
			    f.cod_cli,
			    MAX(f.nome_cli)                  AS nome,
			    SUM(f.pvenda)                    AS pvenda_total,
			    MAX(f.nome_supervisor)           AS nome_sup,
			    MAX(f.nome_rca)                  AS nome_rca,
			    COUNT(DISTINCT f.cod_fornec)     AS n_outros
			FROM %s f
			WHERE f.empresa_id=$1
			    AND f.%s BETWEEN $2::date AND $3::date
			    AND f.positivados=1 AND f.cod_cli != ''
			    AND NOT EXISTS (
			        SELECT 1 FROM %s v
			        WHERE v.empresa_id=f.empresa_id
			            AND v.%s BETWEEN $2::date AND $3::date
			            AND v.cod_cli=f.cod_cli AND v.cod_prod=$4 AND v.qt > 0
			    )
			GROUP BY f.cod_cli
			ORDER BY SUM(f.pvenda) DESC
			LIMIT 200
		`, fluxo.baseView, fluxo.dateCol, fluxo.tableName, fluxo.dateCol),
			spCtx.EmpresaID, refIni, refFim, codProd)

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

		// KPI
		var totalFaturado, totalPlucro float64
		for _, c := range compradores {
			if fluxo.name == "transmitido" {
				totalFaturado += c.Transmitido
			} else {
				totalFaturado += c.Faturado
			}
			totalPlucro += c.Plucro
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

		log.Printf("[marketing:detalhe] fluxo=%s prod=%s comp=%d opor=%d em %v",
			fluxo.name, codProd, nComp, len(oportunidades), time.Since(t0))

		curLabel, antLabel, plabel := buildPeriodoLabels(pr)
		json.NewEncoder(w).Encode(prodDetalheResponse{
			CodProd:    codProd,
			NomeProd:   nomeProd,
			NomeFornec: nomeFornec,
			Ean:        ean,
			KPI: prodDetalheKPI{
				TotalBase:          totalBase,
				TotalCompradores:   nComp,
				TotalOportunidades: len(oportunidades),
				PenetrPct:          penetrPct,
				TotalFaturado:      totalFaturado,
				TotalPlucro:        totalPlucro,
				QtCliAnt:           qtCliAnt,
				DeltaPct:           deltaPct,
				PotencialEstimado:  potencialEstimado,
			},
			Compradores:   compradores,
			Oportunidades: oportunidades,
			Periodo: periodoInfo{
				Fluxo:      fluxo.name,
				RefInicio:  refIni, RefFim: refFim,
				CompInicio: fmtDateOrEmpty(pr.CompInicio), CompFim: fmtDateOrEmpty(pr.CompFim),
				Label: plabel, CurLabel: curLabel, AntLabel: antLabel,
				RefAno: pr.RefAno, RefMes: pr.RefMes,
				CompMode: pr.CompMode, CompAno: pr.CompAno, CompMes: pr.CompMes,
			},
		})
	}
}

// ─── Cliente Detalhe ──────────────────────────────────────────────────────────

type cliDetalheKPI struct {
	TotalProdutos      int     `json:"total_produtos"`
	TotalOportunidades int     `json:"total_oportunidades"`
	TotalFaturado      float64 `json:"total_faturado"`
	TotalTransmitido   float64 `json:"total_transmitido"`
	TotalPlucro        float64 `json:"total_plucro"`
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
	Plucro      float64 `json:"plucro"`
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
	Periodo       periodoInfo       `json:"periodo"`
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
		fluxo := resolveFluxo(q.Get("fluxo"))
		pr := resolvePeriods(db, spCtx.EmpresaID, q)
		if pr.RefInicio.IsZero() {
			json.NewEncoder(w).Encode(cliDetalheResponse{CodCli: codCli})
			return
		}

		t0 := time.Now()
		refIni := pr.RefInicio.Format("2006-01-02")
		refFim := pr.RefFim.Format("2006-01-02")

		// Produtos comprados pelo cliente (no fluxo escolhido)
		rows, err := db.Query(fmt.Sprintf(`
			SELECT
			    cod_prod,
			    MAX(nome_prod)                  AS nome,
			    MAX(COALESCE(nome_fornec,''))   AS nome_fornec,
			    COALESCE(SUM(pvenda),0)         AS pvenda,
			    COALESCE(SUM(plucro),0)         AS plucro
			FROM %s
			WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
			    AND cod_cli=$4 AND qt > 0
			GROUP BY cod_prod
			ORDER BY pvenda DESC
		`, fluxo.tableName, fluxo.dateCol),
			spCtx.EmpresaID, refIni, refFim, codCli)

		comprados := []cliProdutoItem{}
		var totalFaturado, totalTransmitido, totalPlucro float64
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var c cliProdutoItem
				var pv float64
				if err := rows.Scan(&c.Key, &c.Label, &c.NomeFornec, &pv, &c.Plucro); err == nil {
					if fluxo.name == "transmitido" {
						c.Transmitido = pv
						totalTransmitido += pv
					} else {
						c.Faturado = pv
						totalFaturado += pv
					}
					totalPlucro += c.Plucro
					comprados = append(comprados, c)
				}
			}
		}

		// Nome do cliente, RCA, supervisor — da MV base do fluxo
		var nomeCli, nomeRca, nomeSup string
		_ = db.QueryRow(fmt.Sprintf(`
			SELECT MAX(nome_cli), MAX(COALESCE(nome_rca,'')), MAX(COALESCE(nome_supervisor,''))
			FROM %s
			WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date AND cod_cli=$4
		`, fluxo.baseView, fluxo.dateCol),
			spCtx.EmpresaID, refIni, refFim, codCli).Scan(&nomeCli, &nomeRca, &nomeSup)

		// Base total de clientes
		var totalBase int
		_ = db.QueryRow(fmt.Sprintf(`
			SELECT COUNT(DISTINCT cod_cli) FROM %s
			WHERE empresa_id=$1 AND %s BETWEEN $2::date AND $3::date
		`, fluxo.baseView, fluxo.dateCol),
			spCtx.EmpresaID, refIni, refFim).Scan(&totalBase)

		// Oportunidades — produtos populares que este cliente não compra
		oRows, err2 := db.Query(fmt.Sprintf(`
			SELECT
			    p.cod_prod,
			    MAX(p.nome_prod)        AS nome,
			    MAX(p.nome_fornec)      AS nome_fornec,
			    SUM(p.qt_positivados)   AS qt_outros,
			    CASE WHEN SUM(p.qt_positivados) > 0
			         THEN SUM(p.pvenda) / SUM(p.qt_positivados)
			         ELSE 0 END         AS ticket_medio
			FROM %s p
			WHERE p.empresa_id=$1 AND p.%s BETWEEN $2::date AND $3::date
			    AND p.cod_prod != ''
			    AND NOT EXISTS (
			        SELECT 1 FROM %s v
			        WHERE v.empresa_id=p.empresa_id AND v.%s BETWEEN $2::date AND $3::date
			            AND v.cod_cli=$4 AND v.cod_prod=p.cod_prod AND v.qt > 0
			    )
			GROUP BY p.cod_prod
			ORDER BY SUM(p.qt_positivados) DESC
			LIMIT 100
		`, mvMktProduto(fluxo), fluxo.dateCol, fluxo.tableName, fluxo.dateCol),
			spCtx.EmpresaID, refIni, refFim, codCli)

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

		log.Printf("[marketing:cli-detalhe] fluxo=%s cli=%s comprados=%d opor=%d em %v",
			fluxo.name, codCli, len(comprados), len(oportunidades), time.Since(t0))

		curLabel, antLabel, plabel := buildPeriodoLabels(pr)
		json.NewEncoder(w).Encode(cliDetalheResponse{
			CodCli:  codCli,
			NomeCli: nomeCli,
			KPI: cliDetalheKPI{
				TotalProdutos:      len(comprados),
				TotalOportunidades: len(oportunidades),
				TotalFaturado:      totalFaturado,
				TotalTransmitido:   totalTransmitido,
				TotalPlucro:        totalPlucro,
				NomeRca:            nomeRca,
				NomeSup:            nomeSup,
				PotencialEstimado:  potencialEstimado,
			},
			Comprados:     comprados,
			Oportunidades: oportunidades,
			Periodo: periodoInfo{
				Fluxo:      fluxo.name,
				RefInicio:  refIni, RefFim: refFim,
				CompInicio: fmtDateOrEmpty(pr.CompInicio), CompFim: fmtDateOrEmpty(pr.CompFim),
				Label: plabel, CurLabel: curLabel, AntLabel: antLabel,
				RefAno: pr.RefAno, RefMes: pr.RefMes,
				CompMode: pr.CompMode, CompAno: pr.CompAno, CompMes: pr.CompMes,
			},
		})
	}
}
