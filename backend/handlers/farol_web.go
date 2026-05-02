package handlers

// Endpoints autenticados do Farol Web — usam empresa_id do JWT (spCtx.EmpresaID)
// em vez de resolver por CNPJ. Servem as rotas /api/farol/web/*.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ─── /api/farol/web/supervisores ────────────────────────────────────────────

func FarolWebSupervisoresHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, spCtx.EmpresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))

		type supItem struct {
			CodSupervisor int     `json:"cod_supervisor"`
			Nome          string  `json:"nome"`
			Pct           float64 `json:"pct"`
			Cor           string  `json:"cor"`
			VlAnterior    float64 `json:"vl_anterior"`
			VlCorrente    float64 `json:"vl_corrente"`
			QtdRcas       int     `json:"qtd_rcas"`
			QtdRcasAbaixo int     `json:"qtd_rcas_abaixo"`
		}
		type periodoOut struct {
			Tipo  string `json:"tipo"`
			Ano   int    `json:"ano"`
			Seq   int    `json:"seq"`
			Label string `json:"label"`
		}
		type resp struct {
			Periodo      *periodoOut `json:"periodo"`
			Supervisores []supItem   `json:"supervisores"`
		}

		out := resp{Supervisores: []supItem{}}
		if !hasPeriodo {
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		rows, err := db.Query(`
			WITH rca_agg AS (
			  SELECT cod_supervisor, MAX(nome_supervisor) AS nome_sup,
			         cod_rca,
			         SUM(vl_anterior) AS vl_ant,
			         SUM(vl_corrente) AS vl_cor
			  FROM vw_obj_rca_fornecedor
			  WHERE empresa_id = $1 AND tipo_periodo = $2 AND ano = $3 AND periodo_seq = $4
			  GROUP BY cod_supervisor, cod_rca
			)
			SELECT cod_supervisor,
			       MAX(nome_sup),
			       SUM(vl_ant),
			       SUM(vl_cor),
			       COUNT(*),
			       COUNT(*) FILTER (WHERE NOT (vl_ant = 0 AND vl_cor > 0) AND vl_ant > 0 AND (vl_cor / vl_ant) * 100 < 100)
			FROM rca_agg
			GROUP BY cod_supervisor
			ORDER BY MAX(nome_sup)`,
			spCtx.EmpresaID, tipo, ano, seq)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var item supItem
			var supNull sql.NullInt64
			var nomeNull sql.NullString
			if rows.Scan(&supNull, &nomeNull, &item.VlAnterior, &item.VlCorrente, &item.QtdRcas, &item.QtdRcasAbaixo) == nil {
				if supNull.Valid {
					item.CodSupervisor = int(supNull.Int64)
				}
				item.Nome = nomeNull.String
				item.Pct = calcPct(item.VlAnterior, item.VlCorrente)
				item.Cor = farolCor(item.Pct)
				out.Supervisores = append(out.Supervisores, item)
			}
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── /api/farol/web/sup/{cod_supervisor} ────────────────────────────────────

func FarolWebSupHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/farol/web/sup/")
		codSup, err := strconv.Atoi(strings.Trim(path, "/"))
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}

		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, spCtx.EmpresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))

		type rcaItem struct {
			CodRCA     int     `json:"cod_rca"`
			NomeRCA    string  `json:"nome_rca"`
			Pct        float64 `json:"pct"`
			Cor        string  `json:"cor"`
			VlAnterior float64 `json:"vl_anterior"`
			VlCorrente float64 `json:"vl_corrente"`
			QtdFornec  int     `json:"qtd_fornec"`
			QtdAbaixo  int     `json:"qtd_abaixo"`
		}
		type periodoOut struct {
			Tipo  string `json:"tipo"`
			Ano   int    `json:"ano"`
			Seq   int    `json:"seq"`
			Label string `json:"label"`
		}
		type farolGeral struct {
			Pct        float64 `json:"pct"`
			Cor        string  `json:"cor"`
			VlAnterior float64 `json:"vl_anterior"`
			VlCorrente float64 `json:"vl_corrente"`
		}
		type resp struct {
			CodSupervisor int        `json:"cod_supervisor"`
			Nome          string     `json:"nome"`
			Periodo       *periodoOut `json:"periodo"`
			FarolGeral    farolGeral  `json:"farol_geral"`
			Rcas          []rcaItem  `json:"rcas"`
		}

		nomeSup := nomeSupervisorPorEmpresa(db, spCtx.EmpresaID, codSup)
		out := resp{CodSupervisor: codSup, Nome: nomeSup, Rcas: []rcaItem{}}

		if !hasPeriodo {
			out.FarolGeral = farolGeral{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		rows, err := db.Query(`
			SELECT cod_rca,
			       MAX(nome_rca),
			       SUM(vl_anterior),
			       SUM(vl_corrente),
			       COUNT(*),
			       COUNT(*) FILTER (
			           WHERE NOT (vl_anterior = 0 AND vl_corrente > 0)
			             AND vl_anterior > 0
			             AND (vl_corrente / vl_anterior) * 100 < 100
			       )
			FROM vw_obj_rca_fornecedor
			WHERE empresa_id = $1
			  AND tipo_periodo = $2 AND ano = $3 AND periodo_seq = $4
			  AND cod_supervisor = $5
			GROUP BY cod_rca
			ORDER BY cod_rca`,
			spCtx.EmpresaID, tipo, ano, seq, codSup)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var totalAnt, totalCor float64
		for rows.Next() {
			var item rcaItem
			var nomeNull sql.NullString
			if rows.Scan(&item.CodRCA, &nomeNull, &item.VlAnterior, &item.VlCorrente, &item.QtdFornec, &item.QtdAbaixo) == nil {
				item.NomeRCA = nomeNull.String
				item.Pct = calcPct(item.VlAnterior, item.VlCorrente)
				item.Cor = farolCor(item.Pct)
				out.Rcas = append(out.Rcas, item)
				totalAnt += item.VlAnterior
				totalCor += item.VlCorrente
			}
		}
		out.FarolGeral = farolGeral{
			Pct:        calcPct(totalAnt, totalCor),
			Cor:        farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt,
			VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── /api/farol/web/rca/{cod_rca}?cod_supervisor=N ──────────────────────────

func FarolWebRcaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/api/farol/web/rca/")
		codRCA, err := strconv.Atoi(strings.Trim(path, "/"))
		if err != nil || codRCA <= 0 {
			http.Error(w, `{"error":"cod_rca inválido"}`, http.StatusBadRequest)
			return
		}
		codSup, _ := strconv.Atoi(r.URL.Query().Get("cod_supervisor"))

		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, spCtx.EmpresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))

		type fornec struct {
			CodFornec  string  `json:"cod_fornec"`
			Fornecedor string  `json:"fornecedor"`
			Pct        float64 `json:"pct"`
			Cor        string  `json:"cor"`
			VlAnterior float64 `json:"vl_anterior"`
			VlCorrente float64 `json:"vl_corrente"`
		}
		type periodoOut struct {
			Tipo  string `json:"tipo"`
			Ano   int    `json:"ano"`
			Seq   int    `json:"seq"`
			Label string `json:"label"`
		}
		type resumoOut struct {
			Pct        float64 `json:"pct"`
			Cor        string  `json:"cor"`
			VlAnterior float64 `json:"vl_anterior"`
			VlCorrente float64 `json:"vl_corrente"`
		}
		type resp struct {
			CodRCA         int         `json:"cod_rca"`
			NomeRCA        string      `json:"nome_rca"`
			CodSupervisor  int         `json:"cod_supervisor"`
			NomeSupervisor string      `json:"nome_supervisor"`
			Periodo        *periodoOut `json:"periodo"`
			Resumo         resumoOut   `json:"resumo"`
			Fornecedores   []fornec    `json:"fornecedores"`
		}

		var nomeRCA sql.NullString
		_ = db.QueryRow(`SELECT COALESCE(nome,'RCA '||cod_rca::text) FROM rcas WHERE empresa_id=$1 AND cod_rca=$2 LIMIT 1`,
			spCtx.EmpresaID, codRCA).Scan(&nomeRCA)
		nomeSup := ""
		if codSup > 0 {
			nomeSup = nomeSupervisorPorEmpresa(db, spCtx.EmpresaID, codSup)
		}
		out := resp{
			CodRCA:         codRCA,
			NomeRCA:        nomeRCA.String,
			CodSupervisor:  codSup,
			NomeSupervisor: nomeSup,
			Fornecedores:   []fornec{},
		}
		if out.NomeRCA == "" {
			out.NomeRCA = "RCA " + strconv.Itoa(codRCA)
		}

		if !hasPeriodo {
			out.Resumo = resumoOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		var qRows *sql.Rows
		if codSup > 0 {
			qRows, err = db.Query(`
				SELECT cod_fornec, MAX(fornecedor), SUM(vl_anterior), SUM(vl_corrente)
				FROM vw_obj_rca_fornecedor
				WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
				  AND cod_supervisor=$5 AND cod_rca=$6
				GROUP BY cod_fornec
				ORDER BY
				  CASE WHEN SUM(vl_anterior)=0 AND SUM(vl_corrente)>0 THEN 1
				       WHEN SUM(vl_anterior)>0 THEN 0 ELSE 2 END,
				  CASE WHEN SUM(vl_anterior)>0 THEN SUM(vl_corrente)/SUM(vl_anterior) ELSE NULL END ASC,
				  MAX(fornecedor)`,
				spCtx.EmpresaID, tipo, ano, seq, codSup, codRCA)
		} else {
			qRows, err = db.Query(`
				SELECT cod_fornec, MAX(fornecedor), SUM(vl_anterior), SUM(vl_corrente)
				FROM vw_obj_rca_fornecedor
				WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
				  AND cod_rca=$5
				GROUP BY cod_fornec
				ORDER BY
				  CASE WHEN SUM(vl_anterior)=0 AND SUM(vl_corrente)>0 THEN 1
				       WHEN SUM(vl_anterior)>0 THEN 0 ELSE 2 END,
				  CASE WHEN SUM(vl_anterior)>0 THEN SUM(vl_corrente)/SUM(vl_anterior) ELSE NULL END ASC,
				  MAX(fornecedor)`,
				spCtx.EmpresaID, tipo, ano, seq, codRCA)
		}
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer qRows.Close()

		var totalAnt, totalCor float64
		for qRows.Next() {
			var f fornec
			var fornNull sql.NullString
			if qRows.Scan(&f.CodFornec, &fornNull, &f.VlAnterior, &f.VlCorrente) == nil {
				f.Fornecedor = fornNull.String
				f.Pct = calcPct(f.VlAnterior, f.VlCorrente)
				f.Cor = farolCor(f.Pct)
				out.Fornecedores = append(out.Fornecedores, f)
				totalAnt += f.VlAnterior
				totalCor += f.VlCorrente
			}
		}
		out.Resumo = resumoOut{
			Pct:        calcPct(totalAnt, totalCor),
			Cor:        farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt,
			VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── /api/farol/web/periodos ─────────────────────────────────────────────────

func FarolWebPeriodosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rows, err := db.Query(`
			SELECT DISTINCT tipo_periodo, ano, periodo_seq
			FROM objetivos_importados
			WHERE empresa_id=$1
			ORDER BY ano DESC, periodo_seq DESC`, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type p struct {
			Tipo  string `json:"tipo"`
			Ano   int    `json:"ano"`
			Seq   int    `json:"seq"`
			Label string `json:"label"`
		}
		result := []p{}
		for rows.Next() {
			var item p
			if rows.Scan(&item.Tipo, &item.Ano, &item.Seq) == nil {
				item.Label = periodoLabel(item.Tipo, item.Seq, item.Ano)
				result = append(result, item)
			}
		}
		json.NewEncoder(w).Encode(result)
	}
}
