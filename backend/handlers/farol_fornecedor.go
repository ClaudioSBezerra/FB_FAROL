package handlers

// Endpoints da aba "POR FORNECEDOR" do dashboard do supervisor.
// Versão pública (mobile) usa CNPJ; versão web usa empresa_id do JWT.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ─── Tipos compartilhados ────────────────────────────────────────────────────

type fornecAggItem struct {
	CodFornec     string  `json:"cod_fornec"`
	Fornecedor    string  `json:"fornecedor"`
	Pct           float64 `json:"pct"`
	Cor           string  `json:"cor"`
	VlAnterior    float64 `json:"vl_anterior"`
	VlCorrente    float64 `json:"vl_corrente"`
	QtdRcas       int     `json:"qtd_rcas"`
	QtdRcasAbaixo int     `json:"qtd_rcas_abaixo"`
}

type rcaAggItem struct {
	CodRCA     int     `json:"cod_rca"`
	NomeRCA    string  `json:"nome_rca"`
	Pct        float64 `json:"pct"`
	Cor        string  `json:"cor"`
	VlAnterior float64 `json:"vl_anterior"`
	VlCorrente float64 `json:"vl_corrente"`
}

type periodoFarolOut struct {
	Tipo  string `json:"tipo"`
	Ano   int    `json:"ano"`
	Seq   int    `json:"seq"`
	Label string `json:"label"`
}

type resumoFarolOut struct {
	Pct        float64 `json:"pct"`
	Cor        string  `json:"cor"`
	VlAnterior float64 `json:"vl_anterior"`
	VlCorrente float64 `json:"vl_corrente"`
}

// ─── Query helpers ───────────────────────────────────────────────────────────

func queryFornecedoresPorSup(db *sql.DB, empresaID, tipo string, ano, seq, codSup int) ([]fornecAggItem, float64, float64, error) {
	rows, err := db.Query(`
		WITH rca_agg AS (
		  SELECT cod_fornec,
		         MAX(fornecedor) AS forn_nome,
		         cod_rca,
		         SUM(vl_anterior) AS vl_ant,
		         SUM(vl_corrente) AS vl_cor
		  FROM vw_obj_rca_fornecedor
		  WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
		    AND cod_supervisor=$5
		  GROUP BY cod_fornec, cod_rca
		)
		SELECT cod_fornec,
		       MAX(forn_nome),
		       SUM(vl_ant),
		       SUM(vl_cor),
		       COUNT(*) AS qtd_rcas,
		       COUNT(*) FILTER (
		           WHERE NOT (vl_ant = 0 AND vl_cor > 0)
		             AND vl_ant > 0
		             AND (vl_cor / vl_ant) * 100 < 100
		       ) AS qtd_abaixo
		FROM rca_agg
		GROUP BY cod_fornec
		ORDER BY MAX(forn_nome)`,
		empresaID, tipo, ano, seq, codSup)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()
	var totalAnt, totalCor float64
	items := []fornecAggItem{}
	for rows.Next() {
		var it fornecAggItem
		var nomeNull sql.NullString
		if rows.Scan(&it.CodFornec, &nomeNull, &it.VlAnterior, &it.VlCorrente, &it.QtdRcas, &it.QtdRcasAbaixo) == nil {
			it.Fornecedor = nomeNull.String
			it.Pct = calcPct(it.VlAnterior, it.VlCorrente)
			it.Cor = farolCor(it.Pct)
			items = append(items, it)
			totalAnt += it.VlAnterior
			totalCor += it.VlCorrente
		}
	}
	return items, totalAnt, totalCor, nil
}

func queryRcasPorSupFornec(db *sql.DB, empresaID, tipo string, ano, seq, codSup int, codFornec string) ([]rcaAggItem, float64, float64, string, error) {
	var fornecNome string
	_ = db.QueryRow(`
		SELECT MAX(fornecedor) FROM vw_obj_rca_fornecedor
		WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
		  AND cod_supervisor=$5 AND cod_fornec=$6`,
		empresaID, tipo, ano, seq, codSup, codFornec).Scan(&fornecNome)

	rows, err := db.Query(`
		SELECT cod_rca, MAX(nome_rca),
		       SUM(vl_anterior), SUM(vl_corrente)
		FROM vw_obj_rca_fornecedor
		WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
		  AND cod_supervisor=$5 AND cod_fornec=$6
		GROUP BY cod_rca
		ORDER BY cod_rca`,
		empresaID, tipo, ano, seq, codSup, codFornec)
	if err != nil {
		return nil, 0, 0, fornecNome, err
	}
	defer rows.Close()
	var totalAnt, totalCor float64
	items := []rcaAggItem{}
	for rows.Next() {
		var it rcaAggItem
		var nomeNull sql.NullString
		if rows.Scan(&it.CodRCA, &nomeNull, &it.VlAnterior, &it.VlCorrente) == nil {
			it.NomeRCA = nomeNull.String
			it.Pct = calcPct(it.VlAnterior, it.VlCorrente)
			it.Cor = farolCor(it.Pct)
			items = append(items, it)
			totalAnt += it.VlAnterior
			totalCor += it.VlCorrente
		}
	}
	return items, totalAnt, totalCor, fornecNome, nil
}

// ─── Handlers públicos (mobile via CNPJ) ─────────────────────────────────────

// GET /api/farol/sup-forn/{cod}?cnpj=...
func FarolSupFornecedoresHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/api/farol/sup-forn/")
		codSup, err := strconv.Atoi(strings.Trim(path, "/"))
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}
		cnpj := normalizeCnpjQuery(r.URL.Query().Get("cnpj"))
		empresaID, nomeSup, ok := resolveSupervisor(db, codSup, cnpj)
		if !ok {
			http.Error(w, `{"error":"supervisor não encontrado"}`, http.StatusNotFound)
			return
		}
		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, empresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))

		type resp struct {
			CodSupervisor int               `json:"cod_supervisor"`
			Nome          string            `json:"nome"`
			Periodo       *periodoFarolOut  `json:"periodo"`
			FarolGeral    resumoFarolOut    `json:"farol_geral"`
			Fornecedores  []fornecAggItem   `json:"fornecedores"`
		}
		out := resp{CodSupervisor: codSup, Nome: nomeSup, Fornecedores: []fornecAggItem{}}
		if !hasPeriodo {
			out.FarolGeral = resumoFarolOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoFarolOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		items, totalAnt, totalCor, err := queryFornecedoresPorSup(db, empresaID, tipo, ano, seq, codSup)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		out.Fornecedores = items
		out.FarolGeral = resumoFarolOut{
			Pct:        calcPct(totalAnt, totalCor),
			Cor:        farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt,
			VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// GET /api/farol/forn-rcas/{cod_supervisor}?cod_fornec=...&cnpj=...
func FarolFornecRcasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/api/farol/forn-rcas/")
		codSup, err := strconv.Atoi(strings.Trim(path, "/"))
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}
		codFornec := strings.TrimSpace(r.URL.Query().Get("cod_fornec"))
		if codFornec == "" {
			http.Error(w, `{"error":"cod_fornec obrigatório"}`, http.StatusBadRequest)
			return
		}
		cnpj := normalizeCnpjQuery(r.URL.Query().Get("cnpj"))
		empresaID, nomeSup, ok := resolveSupervisor(db, codSup, cnpj)
		if !ok {
			http.Error(w, `{"error":"supervisor não encontrado"}`, http.StatusNotFound)
			return
		}
		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, empresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))

		type resp struct {
			CodSupervisor  int              `json:"cod_supervisor"`
			NomeSupervisor string           `json:"nome_supervisor"`
			CodFornec      string           `json:"cod_fornec"`
			Fornecedor     string           `json:"fornecedor"`
			Periodo        *periodoFarolOut `json:"periodo"`
			Resumo         resumoFarolOut   `json:"resumo"`
			Rcas           []rcaAggItem     `json:"rcas"`
		}
		out := resp{CodSupervisor: codSup, NomeSupervisor: nomeSup, CodFornec: codFornec, Rcas: []rcaAggItem{}}
		if !hasPeriodo {
			out.Resumo = resumoFarolOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoFarolOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		items, totalAnt, totalCor, fornecNome, err := queryRcasPorSupFornec(db, empresaID, tipo, ano, seq, codSup, codFornec)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		out.Fornecedor = fornecNome
		out.Rcas = items
		out.Resumo = resumoFarolOut{
			Pct:        calcPct(totalAnt, totalCor),
			Cor:        farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt,
			VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── Empresa (web autenticado): /api/farol/web/fornecedores ─────────────────

// Lista todos os fornecedores da empresa com totais agregados de todos os supervisores.
func FarolWebFornecedoresEmpresaHandler(db *sql.DB) http.HandlerFunc {
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

		type fornecEmpresaItem struct {
			CodFornec     string  `json:"cod_fornec"`
			Fornecedor    string  `json:"fornecedor"`
			Pct           float64 `json:"pct"`
			Cor           string  `json:"cor"`
			VlAnterior    float64 `json:"vl_anterior"`
			VlCorrente    float64 `json:"vl_corrente"`
			QtdSups       int     `json:"qtd_sups"`
			QtdSupsAbaixo int     `json:"qtd_sups_abaixo"`
		}
		type resp struct {
			Periodo      *periodoFarolOut    `json:"periodo"`
			FarolGeral   resumoFarolOut      `json:"farol_geral"`
			Fornecedores []fornecEmpresaItem `json:"fornecedores"`
		}
		out := resp{Fornecedores: []fornecEmpresaItem{}}
		if !hasPeriodo {
			out.FarolGeral = resumoFarolOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoFarolOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		// Agrega por (cod_fornec, cod_supervisor) e depois por cod_fornec.
		// Contabiliza supervisores abaixo de 100% por fornecedor.
		rows, err := db.Query(`
			WITH sup_agg AS (
			  SELECT cod_fornec, MAX(fornecedor) AS forn_nome,
			         cod_supervisor,
			         SUM(vl_anterior) AS vl_ant,
			         SUM(vl_corrente) AS vl_cor
			  FROM vw_obj_rca_fornecedor
			  WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
			  GROUP BY cod_fornec, cod_supervisor
			)
			SELECT cod_fornec,
			       MAX(forn_nome),
			       SUM(vl_ant),
			       SUM(vl_cor),
			       COUNT(*) AS qtd_sups,
			       COUNT(*) FILTER (
			           WHERE NOT (vl_ant = 0 AND vl_cor > 0)
			             AND vl_ant > 0
			             AND (vl_cor / vl_ant) * 100 < 100
			       ) AS qtd_sups_abaixo
			FROM sup_agg
			GROUP BY cod_fornec
			ORDER BY MAX(forn_nome)`,
			spCtx.EmpresaID, tipo, ano, seq)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var totalAnt, totalCor float64
		for rows.Next() {
			var it fornecEmpresaItem
			var nomeNull sql.NullString
			if rows.Scan(&it.CodFornec, &nomeNull, &it.VlAnterior, &it.VlCorrente, &it.QtdSups, &it.QtdSupsAbaixo) == nil {
				it.Fornecedor = nomeNull.String
				it.Pct = calcPct(it.VlAnterior, it.VlCorrente)
				it.Cor = farolCor(it.Pct)
				out.Fornecedores = append(out.Fornecedores, it)
				totalAnt += it.VlAnterior
				totalCor += it.VlCorrente
			}
		}
		out.FarolGeral = resumoFarolOut{
			Pct: calcPct(totalAnt, totalCor), Cor: farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt, VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── Fornecedor → Supervisores (web): /api/farol/web/forn/:cf/supervisores ──

func FarolWebFornecSupervisoresHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		// path: /api/farol/web/forn/<cf>/supervisores
		path := strings.TrimPrefix(r.URL.Path, "/api/farol/web/forn/")
		path = strings.TrimSuffix(path, "/supervisores")
		codFornec := strings.Trim(path, "/")
		if codFornec == "" {
			http.Error(w, `{"error":"cod_fornec inválido"}`, http.StatusBadRequest)
			return
		}
		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, spCtx.EmpresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))

		type supItem struct {
			CodSupervisor int     `json:"cod_supervisor"`
			NomeSupervisor string `json:"nome_supervisor"`
			Pct           float64 `json:"pct"`
			Cor           string  `json:"cor"`
			VlAnterior    float64 `json:"vl_anterior"`
			VlCorrente    float64 `json:"vl_corrente"`
		}
		type resp struct {
			CodFornec    string           `json:"cod_fornec"`
			Fornecedor   string           `json:"fornecedor"`
			Periodo      *periodoFarolOut `json:"periodo"`
			Resumo       resumoFarolOut   `json:"resumo"`
			Supervisores []supItem        `json:"supervisores"`
		}
		out := resp{CodFornec: codFornec, Supervisores: []supItem{}}
		if !hasPeriodo {
			out.Resumo = resumoFarolOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoFarolOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		// Nome do fornecedor
		var fornecNome sql.NullString
		_ = db.QueryRow(`
			SELECT MAX(fornecedor) FROM vw_obj_rca_fornecedor
			WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
			  AND cod_fornec=$5`,
			spCtx.EmpresaID, tipo, ano, seq, codFornec).Scan(&fornecNome)
		out.Fornecedor = fornecNome.String

		rows, err := db.Query(`
			SELECT cod_supervisor, MAX(nome_supervisor),
			       SUM(vl_anterior), SUM(vl_corrente)
			FROM vw_obj_rca_fornecedor
			WHERE empresa_id=$1 AND tipo_periodo=$2 AND ano=$3 AND periodo_seq=$4
			  AND cod_fornec=$5
			GROUP BY cod_supervisor
			ORDER BY MAX(nome_supervisor)`,
			spCtx.EmpresaID, tipo, ano, seq, codFornec)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var totalAnt, totalCor float64
		for rows.Next() {
			var it supItem
			var supNull sql.NullInt64
			var nomeNull sql.NullString
			if rows.Scan(&supNull, &nomeNull, &it.VlAnterior, &it.VlCorrente) == nil {
				if supNull.Valid {
					it.CodSupervisor = int(supNull.Int64)
				}
				it.NomeSupervisor = nomeNull.String
				it.Pct = calcPct(it.VlAnterior, it.VlCorrente)
				it.Cor = farolCor(it.Pct)
				out.Supervisores = append(out.Supervisores, it)
				totalAnt += it.VlAnterior
				totalCor += it.VlCorrente
			}
		}
		out.Resumo = resumoFarolOut{
			Pct: calcPct(totalAnt, totalCor), Cor: farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt, VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── Handlers autenticados existentes (web via JWT) ─────────────────────────

// GET /api/farol/web/sup-forn/{cod}
func FarolWebSupFornecedoresHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/farol/web/sup-forn/")
		codSup, err := strconv.Atoi(strings.Trim(path, "/"))
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}
		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, spCtx.EmpresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))
		nomeSup := nomeSupervisorPorEmpresa(db, spCtx.EmpresaID, codSup)

		type resp struct {
			CodSupervisor int              `json:"cod_supervisor"`
			Nome          string           `json:"nome"`
			Periodo       *periodoFarolOut `json:"periodo"`
			FarolGeral    resumoFarolOut   `json:"farol_geral"`
			Fornecedores  []fornecAggItem  `json:"fornecedores"`
		}
		out := resp{CodSupervisor: codSup, Nome: nomeSup, Fornecedores: []fornecAggItem{}}
		if !hasPeriodo {
			out.FarolGeral = resumoFarolOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoFarolOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}
		items, totalAnt, totalCor, err := queryFornecedoresPorSup(db, spCtx.EmpresaID, tipo, ano, seq, codSup)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		out.Fornecedores = items
		out.FarolGeral = resumoFarolOut{
			Pct:        calcPct(totalAnt, totalCor),
			Cor:        farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt,
			VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}

// GET /api/farol/web/forn-rcas/{cod_supervisor}?cod_fornec=...
func FarolWebFornecRcasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/farol/web/forn-rcas/")
		codSup, err := strconv.Atoi(strings.Trim(path, "/"))
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}
		codFornec := strings.TrimSpace(r.URL.Query().Get("cod_fornec"))
		if codFornec == "" {
			http.Error(w, `{"error":"cod_fornec obrigatório"}`, http.StatusBadRequest)
			return
		}
		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, spCtx.EmpresaID,
			r.URL.Query().Get("tipo_periodo"),
			r.URL.Query().Get("ano"),
			r.URL.Query().Get("periodo_seq"))
		nomeSup := nomeSupervisorPorEmpresa(db, spCtx.EmpresaID, codSup)

		type resp struct {
			CodSupervisor  int              `json:"cod_supervisor"`
			NomeSupervisor string           `json:"nome_supervisor"`
			CodFornec      string           `json:"cod_fornec"`
			Fornecedor     string           `json:"fornecedor"`
			Periodo        *periodoFarolOut `json:"periodo"`
			Resumo         resumoFarolOut   `json:"resumo"`
			Rcas           []rcaAggItem     `json:"rcas"`
		}
		out := resp{CodSupervisor: codSup, NomeSupervisor: nomeSup, CodFornec: codFornec, Rcas: []rcaAggItem{}}
		if !hasPeriodo {
			out.Resumo = resumoFarolOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}
		out.Periodo = &periodoFarolOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}
		items, totalAnt, totalCor, fornecNome, err := queryRcasPorSupFornec(db, spCtx.EmpresaID, tipo, ano, seq, codSup, codFornec)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		out.Fornecedor = fornecNome
		out.Rcas = items
		out.Resumo = resumoFarolOut{
			Pct:        calcPct(totalAnt, totalCor),
			Cor:        farolCor(calcPct(totalAnt, totalCor)),
			VlAnterior: totalAnt,
			VlCorrente: totalCor,
		}
		json.NewEncoder(w).Encode(out)
	}
}
