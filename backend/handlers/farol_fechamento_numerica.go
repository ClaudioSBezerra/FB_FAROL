package handlers

// farol_fechamento_numerica.go — Comparativo Fechamento Numérica (Relatórios),
// Épico 7 addendum, 2026-09-30: mesmo princípio do Comparativo Fechamento
// Comercial (farol_fechamento_comercial.go, ver seu cabeçalho pro racional
// completo), só que pro par Numérica (Cobertura Numérica + Sortimento
// Numérica/PPA) — chave por CNPJ/Cliente, não por Rede (FR24, a Numérica
// não tem esse conceito).
//
// Formato de entrada: a aba "Resumo Numerica(s) Cliente" do modelo de
// fechamento da JC (nome da aba varia — "Numerica" ou "Numericas" entre os
// arquivos HC/Foods, achado real 2026-09-30) não traz CNPJ direto, só COD
// CL — o frontend resolve CNPJ casando com a aba "BASE LOJAS" do mesmo
// arquivo antes de mandar o CSV pra cá (mesmo espírito do cruzamento de
// Objetivo Cobertura×limiar que o fechamento Rede faz, só que mais simples:
// cada arquivo da Numérica já cobre UMA indústria só, sem duplicar linha).
//
// Fonte Farol pro comparativo: farol.metas_realizados_snapshot (mesma
// apuração oficial de qualquer tela do Painel de Metas) — os vínculos
// cobertura_numerica/sortimento_numerica_ppa já devolvem RealizadoRede com
// CodPrinc=CNPJ (truque de reuso documentado em farol_metas_calculo_numerica.go),
// então o mesmo formato de leitura do comparativo Rede serve sem alteração.

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

type fechamentoNumericaRow struct {
	linha                                             int
	industriaID                                       int
	cnpj, codCl, classificacaoPdv                     string
	razao, fantasia                                   string
	valorVenda, qtPpasVendidos                        float64
	codGGV, nomeGGV, codCRV, nomeCRV, codRCA, nomeRCA string
}

var fechamentoNumericaColunasObrigatorias = []string{"industria_id", "cnpj", "valor_venda", "qt_ppas_vendidos"}

// FechamentoNumericaImportarCSVHandler — POST .../fechamento-numerica-importar-csv?data_inicio=&data_fim=
//
// Substitui (PUT-replace) tudo que já existia pra essa indústria+período —
// sempre UMA indústria por arquivo (diferente do fechamento Rede).
func FechamentoNumericaImportarCSVHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !hasSpRole(spCtx.SpRole, "gestor_geral") {
			http.Error(w, `{"error":"Forbidden: gestor_geral necessário"}`, http.StatusForbidden)
			return
		}
		dataInicio := strings.TrimSpace(r.URL.Query().Get("data_inicio"))
		dataFim := strings.TrimSpace(r.URL.Query().Get("data_fim"))
		if dataInicio == "" || dataFim == "" {
			http.Error(w, `{"error":"parâmetros data_inicio e data_fim (query string, YYYY-MM-DD) são obrigatórios"}`, http.StatusBadRequest)
			return
		}

		rawBytes, err := lerArquivoCSV(r)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		csvReader := csv.NewReader(strings.NewReader(string(rawBytes)))
		csvReader.Comma = ';'
		csvReader.LazyQuotes = true
		csvReader.TrimLeadingSpace = true
		csvReader.FieldsPerRecord = -1

		headerRow, err := csvReader.Read()
		if err != nil {
			http.Error(w, `{"error":"falha ao ler cabeçalho CSV"}`, http.StatusBadRequest)
			return
		}
		norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
		colIdx := map[string]int{}
		for i, h := range headerRow {
			colIdx[norm(h)] = i
		}
		for _, c := range fechamentoNumericaColunasObrigatorias {
			if _, ok := colIdx[c]; !ok {
				http.Error(w, `{"error":"coluna obrigatória ausente no CSV: `+c+`"}`, http.StatusBadRequest)
				return
			}
		}
		get := func(rec []string, campo string) string {
			idx, ok := colIdx[campo]
			if !ok || idx >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[idx])
		}
		parseFloat := func(s string) float64 {
			s = strings.ReplaceAll(s, ",", ".")
			v, _ := strconv.ParseFloat(s, 64)
			return v
		}

		var rows []fechamentoNumericaRow
		var erros []fechamentoComercialLinhaErro
		industriasNoArquivo := map[int]bool{}
		linhaNum := 1
		for {
			rec, err := csvReader.Read()
			if err == io.EOF {
				break
			}
			linhaNum++
			if err != nil {
				erros = append(erros, fechamentoComercialLinhaErro{Linha: linhaNum, Erro: "linha malformada: " + err.Error()})
				continue
			}
			industriaID, iErr := strconv.Atoi(get(rec, "industria_id"))
			cnpj := get(rec, "cnpj")
			if iErr != nil || cnpj == "" {
				erros = append(erros, fechamentoComercialLinhaErro{Linha: linhaNum, Erro: "industria_id e cnpj são obrigatórios"})
				continue
			}
			row := fechamentoNumericaRow{
				linha: linhaNum, industriaID: industriaID, cnpj: cnpj,
				codCl: get(rec, "cod_cl"), classificacaoPdv: get(rec, "classificacao_pdv"),
				razao: get(rec, "razao"), fantasia: get(rec, "fantasia"),
				valorVenda:     parseFloat(get(rec, "valor_venda")),
				qtPpasVendidos: parseFloat(get(rec, "qt_ppas_vendidos")),
				codGGV:         get(rec, "cod_ggv"), nomeGGV: get(rec, "nome_ggv"),
				codCRV: get(rec, "cod_crv"), nomeCRV: get(rec, "nome_crv"),
				codRCA: get(rec, "cod_rca"), nomeRCA: get(rec, "nome_rca"),
			}
			rows = append(rows, row)
			industriasNoArquivo[industriaID] = true
		}
		if len(rows) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"erros": erros, "linhas_com_erro": len(erros)})
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		for industriaID := range industriasNoArquivo {
			var existe bool
			if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM farol.industrias WHERE id=$1 AND empresa_id=$2)`, industriaID, spCtx.EmpresaID).Scan(&existe); err != nil {
				http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
				return
			}
			if !existe {
				http.Error(w, fmt.Sprintf(`{"error":"industria_id=%d não encontrada nesta empresa"}`, industriaID), http.StatusBadRequest)
				return
			}
			if _, err := tx.Exec(`
				DELETE FROM farol.fechamento_numerica_externo
				WHERE empresa_id=$1 AND industria_id=$2 AND data_inicio=$3 AND data_fim=$4
			`, spCtx.EmpresaID, industriaID, dataInicio, dataFim); err != nil {
				http.Error(w, `{"error":"database error ao limpar dados anteriores"}`, http.StatusInternalServerError)
				return
			}
		}
		for _, row := range rows {
			if _, err := tx.Exec(`
				INSERT INTO farol.fechamento_numerica_externo
					(empresa_id, industria_id, data_inicio, data_fim, cnpj, cod_cl, classificacao_pdv, razao, fantasia,
					 valor_venda, qt_ppas_vendidos, cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca, importado_por)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
				ON CONFLICT (industria_id, data_inicio, data_fim, cnpj) DO UPDATE SET
					cod_cl=EXCLUDED.cod_cl, classificacao_pdv=EXCLUDED.classificacao_pdv,
					razao=EXCLUDED.razao, fantasia=EXCLUDED.fantasia,
					valor_venda=EXCLUDED.valor_venda, qt_ppas_vendidos=EXCLUDED.qt_ppas_vendidos,
					cod_ggv=EXCLUDED.cod_ggv, nome_ggv=EXCLUDED.nome_ggv, cod_crv=EXCLUDED.cod_crv, nome_crv=EXCLUDED.nome_crv,
					cod_rca=EXCLUDED.cod_rca, nome_rca=EXCLUDED.nome_rca, importado_em=now(), importado_por=EXCLUDED.importado_por
			`, spCtx.EmpresaID, row.industriaID, dataInicio, dataFim, row.cnpj, row.codCl, row.classificacaoPdv, row.razao, row.fantasia,
				row.valorVenda, row.qtPpasVendidos, row.codGGV, row.nomeGGV, row.codCRV, row.nomeCRV, row.codRCA, row.nomeRCA, spCtx.UserID); err != nil {
				http.Error(w, `{"error":"database error ao gravar linha `+strconv.Itoa(row.linha)+`: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}
		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "fechamento_numerica_externo", fmt.Sprintf("%s..%s", dataInicio, dataFim), "importar_csv", map[string]any{
			"linhas": len(rows), "industrias": len(industriasNoArquivo),
		}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("FechamentoNumerica: %d linhas importadas (%d indústrias, %s..%s) empresa %s por %s",
			len(rows), len(industriasNoArquivo), dataInicio, dataFim, spCtx.EmpresaID, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "linhas_importadas": len(rows), "avisos": erros, "linhas_com_erro": len(erros)})
	}
}

// ─── Comparativo ──────────────────────────────────────────────────────────────

type comparativoNumericaLinha struct {
	CNPJ             string  `json:"cnpj"`
	CodCl            string  `json:"cod_cl"`
	ClassificacaoPdv string  `json:"classificacao_pdv"`
	Razao            string  `json:"razao"`
	Fantasia         string  `json:"fantasia"`
	CodGGV           string  `json:"cod_ggv"`
	NomeGGV          string  `json:"nome_ggv"`
	CodCRV           string  `json:"cod_crv"`
	NomeCRV          string  `json:"nome_crv"`
	CodRCA           string  `json:"cod_rca"`
	NomeRCA          string  `json:"nome_rca"`
	ValorVendaExt    float64 `json:"valor_venda_externo"`
	ValorVendaFarol  float64 `json:"valor_venda_farol"`
	DiferencaValor   float64 `json:"diferenca_valor"`
	DiferencaValorP  float64 `json:"diferenca_valor_pct"`
	PpasExt          float64 `json:"ppas_externo"`
	PpasFarol        float64 `json:"ppas_farol"`
	DiferencaPpas    float64 `json:"diferenca_ppas"`
	Status           string  `json:"status"` // OK | DIVERGE | SO_EXTERNO | SO_FAROL
	// CodCliDivergente — achado real 2026-09-30: o CNPJ tem venda no
	// bimestre lançada sob um COD CLI diferente do cadastrado em
	// metas_clientes_numericas (recadastro no WinThor que o arquivo do
	// fornecedor não acompanhou). Não é bug do Farol — o Farol soma por
	// CNPJ (FR24, sem conceito de Rede/dono na Numérica), então captura a
	// venda certa; o fechamento do fornecedor, se junta por código de
	// cliente, fica cego pra ela. Só um indicador informativo, não muda o
	// Status (que continua OK/DIVERGE pela diferença de valor).
	CodCliDivergente bool     `json:"cod_cli_divergente"`
	CodCliVistos     []string `json:"cod_cli_vistos,omitempty"` // outros COD CLI vistos nas vendas, além do cadastrado
}

// FechamentoNumericaComparativoHandler — GET .../fechamento-numerica-comparativo?industria_id=&data_inicio=&data_fim=
func FechamentoNumericaComparativoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		industriaID, err := strconv.Atoi(r.URL.Query().Get("industria_id"))
		dataInicio := strings.TrimSpace(r.URL.Query().Get("data_inicio"))
		dataFim := strings.TrimSpace(r.URL.Query().Get("data_fim"))
		if err != nil || dataInicio == "" || dataFim == "" {
			http.Error(w, `{"error":"industria_id, data_inicio e data_fim são obrigatórios"}`, http.StatusBadRequest)
			return
		}

		out, err := gerarComparativoFechamentoNumerica(db, spCtx.EmpresaID, industriaID, dataInicio, dataFim)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{"linhas": out, "total": len(out)})
	}
}

// gerarComparativoFechamentoNumerica — mesmo racional de gerarComparativoFechamento
// (farol_fechamento_comercial.go), trocando cod_princ por CNPJ e
// cobertura_rede/sortimento_rede por cobertura_numerica/sortimento_numerica_ppa.
func gerarComparativoFechamentoNumerica(db *sql.DB, empresaID string, industriaID int, dataInicio, dataFim string) ([]comparativoNumericaLinha, error) {
	t0 := time.Now()
	defer func() {
		log.Printf("[farol:objetivos] gerarComparativoFechamentoNumerica industria=%d período=[%s..%s] em %v",
			industriaID, dataInicio, dataFim, time.Since(t0))
	}()
	var vinculoCobID, vigenciaCobID sql.NullInt64
	var vinculoSortID, vigenciaSortID sql.NullInt64
	_ = db.QueryRow(`
		SELECT mv.id, v.id FROM farol.metas_vinculos mv
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id AND tm.formula_codigo = 'cobertura_numerica'
		JOIN farol.metas_vigencias v ON v.vinculo_id = mv.id AND v.data_inicio = $3 AND v.data_fim = $4
		WHERE mv.empresa_id = $1 AND mv.industria_id = $2
	`, empresaID, industriaID, dataInicio, dataFim).Scan(&vinculoCobID, &vigenciaCobID)
	_ = db.QueryRow(`
		SELECT mv.id, v.id FROM farol.metas_vinculos mv
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id AND tm.formula_codigo = 'sortimento_numerica_ppa'
		JOIN farol.metas_vigencias v ON v.vinculo_id = mv.id AND v.data_inicio = $3 AND v.data_fim = $4
		WHERE mv.empresa_id = $1 AND mv.industria_id = $2
	`, empresaID, industriaID, dataInicio, dataFim).Scan(&vinculoSortID, &vigenciaSortID)

	if !vigenciaCobID.Valid && !vigenciaSortID.Valid {
		return nil, fmt.Errorf("nenhuma vigência de Cobertura Numérica ou Sortimento Numérica desta indústria bate com esse período exato — cadastre a vigência primeiro")
	}

	cobPorCnpj := map[string]RealizadoRede{}
	sortPorCnpj := map[string]RealizadoRede{}
	if vigenciaCobID.Valid {
		res, err := obterOuCongelarRealizado(db, empresaID, int(vinculoCobID.Int64), int(vigenciaCobID.Int64), "faturado", "rede")
		if err == nil {
			for _, rd := range res.Redes {
				cobPorCnpj[rd.CodPrinc] = rd
			}
		}
	}
	if vigenciaSortID.Valid {
		res, err := obterOuCongelarRealizado(db, empresaID, int(vinculoSortID.Int64), int(vigenciaSortID.Int64), "faturado", "rede")
		if err == nil {
			for _, rd := range res.Redes {
				sortPorCnpj[rd.CodPrinc] = rd
			}
		}
	}

	vinculoCadastroID := vinculoCobID
	if !vinculoCadastroID.Valid {
		vinculoCadastroID = vinculoSortID
	}
	codCliVistosPorCnpj, err := calcularCodCliDivergentes(db, empresaID, industriaID, int(vinculoCadastroID.Int64), dataInicio, dataFim)
	if err != nil {
		// Diagnóstico informativo — não trava o comparativo se falhar.
		codCliVistosPorCnpj = map[string][]string{}
	}

	rows, err := db.Query(`
		SELECT cnpj, cod_cl, classificacao_pdv, razao, fantasia, valor_venda, qt_ppas_vendidos,
		       cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca
		FROM farol.fechamento_numerica_externo
		WHERE empresa_id = $1 AND industria_id = $2 AND data_inicio = $3 AND data_fim = $4
	`, empresaID, industriaID, dataInicio, dataFim)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	externoPorCnpj := map[string]comparativoNumericaLinha{}
	var ordem []string
	for rows.Next() {
		var l comparativoNumericaLinha
		if err := rows.Scan(&l.CNPJ, &l.CodCl, &l.ClassificacaoPdv, &l.Razao, &l.Fantasia, &l.ValorVendaExt, &l.PpasExt,
			&l.CodGGV, &l.NomeGGV, &l.CodCRV, &l.NomeCRV, &l.CodRCA, &l.NomeRCA); err != nil {
			return nil, err
		}
		externoPorCnpj[l.CNPJ] = l
		ordem = append(ordem, l.CNPJ)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	vistos := map[string]bool{}
	var out []comparativoNumericaLinha
	processar := func(cnpj string) {
		if vistos[cnpj] {
			return
		}
		vistos[cnpj] = true
		l, temExterno := externoPorCnpj[cnpj]
		l.CNPJ = cnpj
		cob, temCob := cobPorCnpj[cnpj]
		sort_, temSort := sortPorCnpj[cnpj]
		if temCob {
			l.ValorVendaFarol = cob.Valor
			if l.Razao == "" {
				l.Razao, l.Fantasia = cob.Razao, cob.Fantasia
			}
			if l.CodGGV == "" {
				l.CodGGV, l.NomeGGV, l.CodCRV, l.NomeCRV, l.CodRCA, l.NomeRCA = cob.CodGGV, cob.NomeGGV, cob.CodCRV, cob.NomeCRV, cob.CodRCA, cob.NomeRCA
			}
		}
		if temSort {
			l.PpasFarol = sort_.Valor
			if l.Razao == "" {
				l.Razao, l.Fantasia = sort_.Razao, sort_.Fantasia
			}
			if l.CodGGV == "" {
				l.CodGGV, l.NomeGGV, l.CodCRV, l.NomeCRV, l.CodRCA, l.NomeRCA = sort_.CodGGV, sort_.NomeGGV, sort_.CodCRV, sort_.NomeCRV, sort_.CodRCA, sort_.NomeRCA
			}
		}
		if codigos, ok := codCliVistosPorCnpj[cnpj]; ok && len(codigos) > 0 {
			l.CodCliDivergente = true
			l.CodCliVistos = codigos
		}
		temFarol := temCob || temSort
		l.DiferencaValor = l.ValorVendaFarol - l.ValorVendaExt
		if l.ValorVendaExt != 0 {
			l.DiferencaValorP = l.DiferencaValor / l.ValorVendaExt * 100
		}
		l.DiferencaPpas = l.PpasFarol - l.PpasExt
		switch {
		case !temExterno:
			l.Status = "SO_FAROL"
		case !temFarol:
			l.Status = "SO_EXTERNO"
		case absFloat(l.DiferencaValorP) > 5 || absFloat(l.DiferencaPpas) >= 2:
			l.Status = "DIVERGE"
		default:
			l.Status = "OK"
		}
		out = append(out, l)
	}
	for _, cnpj := range ordem {
		processar(cnpj)
	}
	for cnpj := range cobPorCnpj {
		processar(cnpj)
	}
	for cnpj := range sortPorCnpj {
		processar(cnpj)
	}

	sort.Slice(out, func(i, j int) bool {
		oi, oj := statusOrdem(out[i].Status), statusOrdem(out[j].Status)
		if oi != oj {
			return oi < oj
		}
		return absFloat(out[i].DiferencaValorP) > absFloat(out[j].DiferencaValorP)
	})

	return out, nil
}

// calcularCodCliDivergentes — achado real 2026-09-30 (ver comentário de
// CodCliDivergente acima): alguns CNPJs têm venda no bimestre lançada sob
// um COD CLI diferente do cadastrado em metas_clientes_numericas — recadastro
// no WinThor que o arquivo de Fechamento do fornecedor (que junta por código
// de cliente) não acompanha. Devolve, por CNPJ, a lista de COD CLI vistos
// nas vendas que NÃO batem com o cadastrado — vazio/ausente = sem divergência.
//
// Usa a MESMA janela (bimestre móvel, se aplicável) que o motor de cálculo
// oficial usa (CalcularRealizadoComPeriodo) — senão o diagnóstico compararia
// períodos diferentes do que realmente foi apurado.
func calcularCodCliDivergentes(db *sql.DB, empresaID string, industriaID, vinculoCadastroID int, dataInicio, dataFim string) (map[string][]string, error) {
	out := map[string][]string{}
	if vinculoCadastroID == 0 {
		return out, nil
	}

	var janelaApuracao string
	if err := db.QueryRow(`
		SELECT tm.janela_apuracao FROM farol.metas_vinculos mv
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE mv.id = $1 AND mv.empresa_id = $2
	`, vinculoCadastroID, empresaID).Scan(&janelaApuracao); err != nil {
		return out, err
	}
	dataInicioJanela := dataInicio
	if janelaApuracao == "bimestre_movel" {
		if inicio, perr := time.Parse("2006-01-02", dataInicio); perr == nil {
			dataInicioJanela = inicio.AddDate(0, -1, 0).Format("2006-01-02")
		}
	}

	codFornec, err := codFornecDaIndustria(db, empresaID, industriaID)
	if err != nil {
		return out, err
	}

	query := `
		SELECT c.cnpj, array_agg(DISTINCT v.cod_cli)
		FROM farol.metas_clientes_numericas c
		JOIN vendas_faturadas v ON v.cnpj = c.cnpj AND v.empresa_id = c.empresa_id
		WHERE c.vinculo_id = $1 AND c.empresa_id = $2
		  AND v.data_faturamento BETWEEN $3 AND $4
		  AND v.cod_cli <> '' AND v.cod_cli <> c.cod_cl
	`
	args := []any{vinculoCadastroID, empresaID, dataInicioJanela, dataFim}
	if len(codFornec) > 0 {
		query += fmt.Sprintf(" AND v.cod_fornec = ANY($%d)", len(args)+1)
		args = append(args, pq.Array(codFornec))
	}
	query += " GROUP BY c.cnpj"

	rows, err := db.Query(query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var cnpj string
		var codigos pq.StringArray
		if err := rows.Scan(&cnpj, &codigos); err != nil {
			return out, err
		}
		out[cnpj] = []string(codigos)
	}
	return out, rows.Err()
}
