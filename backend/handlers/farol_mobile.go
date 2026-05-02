package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func farolCor(pct float64) string {
	if pct >= 100 {
		return "verde"
	}
	if pct >= 75 {
		return "amarelo"
	}
	return "vermelho"
}

func calcPct(ant, cor float64) float64 {
	if ant == 0 {
		return 0
	}
	return (cor / ant) * 100
}

// resolveSupervisor busca empresa_id e nome para o cod_supervisor.
// Como cod_supervisor pode existir em várias empresas, retorna a primeira encontrada.
func resolveSupervisor(db *sql.DB, codSup int) (empresaID, nome string, ok bool) {
	err := db.QueryRow(`
		SELECT empresa_id, COALESCE(nome, 'Supervisor ' || cod_supervisor::text)
		FROM gestores
		WHERE cod_supervisor = $1
		ORDER BY empresa_id
		LIMIT 1`, codSup).Scan(&empresaID, &nome)
	if err != nil {
		return "", "", false
	}
	return empresaID, nome, true
}

// resolvePeriodo retorna tipo/ano/seq selecionados ou o mais recente disponível.
func resolvePeriodo(db *sql.DB, empresaID string, tipoQ, anoQ, seqQ string) (tipo string, ano, seq int, ok bool) {
	tipoQ = strings.ToUpper(strings.TrimSpace(tipoQ))
	if tipoQ != "" && anoQ != "" && seqQ != "" {
		ano, errA := strconv.Atoi(anoQ)
		seq, errS := strconv.Atoi(seqQ)
		if errA == nil && errS == nil {
			return tipoQ, ano, seq, true
		}
	}
	// fallback: mais recente
	err := db.QueryRow(`
		SELECT tipo_periodo, ano, periodo_seq
		FROM objetivos_importados
		WHERE empresa_id = $1
		GROUP BY tipo_periodo, ano, periodo_seq
		ORDER BY ano DESC, periodo_seq DESC
		LIMIT 1`, empresaID).Scan(&tipo, &ano, &seq)
	if err != nil {
		return "", 0, 0, false
	}
	return tipo, ano, seq, true
}

// ─── /api/farol/sup/{cod_supervisor} ─────────────────────────────────────────

// FarolSupervisorHandler retorna o dashboard mobile do supervisor:
// dados do supervisor, farol agregado e lista de RCAs com semáforo.
func FarolSupervisorHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// /api/farol/sup/701 → cod = 701
		path := strings.TrimPrefix(r.URL.Path, "/api/farol/sup/")
		path = strings.Trim(path, "/")
		codSup, err := strconv.Atoi(path)
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}

		empresaID, nomeSup, ok := resolveSupervisor(db, codSup)
		if !ok {
			http.Error(w, `{"error":"supervisor não encontrado"}`, http.StatusNotFound)
			return
		}

		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, empresaID,
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
			CodSupervisor int          `json:"cod_supervisor"`
			Nome          string       `json:"nome"`
			EmpresaID     string       `json:"empresa_id"`
			Periodo       *periodoOut  `json:"periodo"`
			FarolGeral    farolGeral   `json:"farol_geral"`
			Rcas          []rcaItem    `json:"rcas"`
		}

		out := resp{
			CodSupervisor: codSup,
			Nome:          nomeSup,
			EmpresaID:     empresaID,
			Rcas:          []rcaItem{},
		}

		if !hasPeriodo {
			// sem dados ainda → resposta vazia mas válida
			out.FarolGeral = farolGeral{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}

		out.Periodo = &periodoOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		// Agrega por RCA (somando todos fornecedores da view)
		rows, err := db.Query(`
			SELECT cod_rca,
			       MAX(nome_rca) AS nome_rca,
			       SUM(vl_anterior) AS vl_ant,
			       SUM(vl_corrente) AS vl_cor
			FROM vw_obj_rca_fornecedor
			WHERE empresa_id = $1
			  AND tipo_periodo = $2
			  AND ano = $3
			  AND periodo_seq = $4
			  AND cod_supervisor = $5
			GROUP BY cod_rca
			ORDER BY cod_rca`,
			empresaID, tipo, ano, seq, codSup)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var totalAnt, totalCor float64
		for rows.Next() {
			var item rcaItem
			var nomeNull sql.NullString
			if rows.Scan(&item.CodRCA, &nomeNull, &item.VlAnterior, &item.VlCorrente) == nil {
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

// ─── /api/farol/rca/{cod_rca}?cod_supervisor=N ──────────────────────────────

// FarolRcaDetailHandler retorna o detalhe do RCA com fornecedores comparativos.
func FarolRcaDetailHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/farol/rca/")
		path = strings.Trim(path, "/")
		codRCA, err := strconv.Atoi(path)
		if err != nil || codRCA <= 0 {
			http.Error(w, `{"error":"cod_rca inválido"}`, http.StatusBadRequest)
			return
		}

		codSup, err := strconv.Atoi(r.URL.Query().Get("cod_supervisor"))
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}

		empresaID, nomeSup, ok := resolveSupervisor(db, codSup)
		if !ok {
			http.Error(w, `{"error":"supervisor não encontrado"}`, http.StatusNotFound)
			return
		}

		tipo, ano, seq, hasPeriodo := resolvePeriodo(db, empresaID,
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

		out := resp{
			CodRCA:         codRCA,
			CodSupervisor:  codSup,
			NomeSupervisor: nomeSup,
			Fornecedores:   []fornec{},
		}

		// resolve nome do RCA
		var nomeRCA sql.NullString
		_ = db.QueryRow(`
			SELECT COALESCE(nome, 'RCA ' || cod_rca::text)
			FROM rcas WHERE empresa_id = $1 AND cod_rca = $2
			LIMIT 1`, empresaID, codRCA).Scan(&nomeRCA)
		if nomeRCA.Valid {
			out.NomeRCA = nomeRCA.String
		} else {
			out.NomeRCA = "RCA " + strconv.Itoa(codRCA)
		}

		if !hasPeriodo {
			out.Resumo = resumoOut{Cor: "vermelho"}
			json.NewEncoder(w).Encode(out)
			return
		}

		out.Periodo = &periodoOut{Tipo: tipo, Ano: ano, Seq: seq, Label: periodoLabel(tipo, seq, ano)}

		rows, err := db.Query(`
			SELECT cod_fornec, fornecedor, vl_anterior, vl_corrente
			FROM vw_obj_rca_fornecedor
			WHERE empresa_id = $1
			  AND tipo_periodo = $2
			  AND ano = $3
			  AND periodo_seq = $4
			  AND cod_supervisor = $5
			  AND cod_rca = $6
			ORDER BY fornecedor`,
			empresaID, tipo, ano, seq, codSup, codRCA)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var totalAnt, totalCor float64
		for rows.Next() {
			var f fornec
			var fornNull sql.NullString
			if rows.Scan(&f.CodFornec, &fornNull, &f.VlAnterior, &f.VlCorrente) == nil {
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

// ─── /api/farol/periodos/{cod_supervisor} ───────────────────────────────────

// FarolPeriodosHandler lista períodos disponíveis para o supervisor.
func FarolPeriodosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/farol/periodos/")
		path = strings.Trim(path, "/")
		codSup, err := strconv.Atoi(path)
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}

		empresaID, _, ok := resolveSupervisor(db, codSup)
		if !ok {
			http.Error(w, `{"error":"supervisor não encontrado"}`, http.StatusNotFound)
			return
		}

		rows, err := db.Query(`
			SELECT DISTINCT tipo_periodo, ano, periodo_seq
			FROM objetivos_importados
			WHERE empresa_id = $1 AND cod_supervisor = $2
			ORDER BY ano DESC, periodo_seq DESC`,
			empresaID, codSup)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type periodo struct {
			Tipo  string `json:"tipo"`
			Ano   int    `json:"ano"`
			Seq   int    `json:"seq"`
			Label string `json:"label"`
		}
		result := make([]periodo, 0)
		for rows.Next() {
			var p periodo
			if rows.Scan(&p.Tipo, &p.Ano, &p.Seq) == nil {
				p.Label = periodoLabel(p.Tipo, p.Seq, p.Ano)
				result = append(result, p)
			}
		}
		json.NewEncoder(w).Encode(result)
	}
}

// periodoLabel — mesma lógica do frontend, em PT-BR.
var (
	farolMeses      = []string{"Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}
	farolTrimestres = []string{"T1", "T2", "T3", "T4"}
	farolSemestres  = []string{"S1", "S2"}
)

func periodoLabel(tipo string, seq, ano int) string {
	idx := seq - 1
	switch tipo {
	case "MENSAL":
		if idx >= 0 && idx < len(farolMeses) {
			return farolMeses[idx] + "/" + strconv.Itoa(ano)
		}
	case "TRIMESTRAL":
		if idx >= 0 && idx < len(farolTrimestres) {
			return farolTrimestres[idx] + "/" + strconv.Itoa(ano)
		}
	case "SEMESTRAL":
		if idx >= 0 && idx < len(farolSemestres) {
			return farolSemestres[idx] + "/" + strconv.Itoa(ano)
		}
	}
	return strconv.Itoa(ano)
}
