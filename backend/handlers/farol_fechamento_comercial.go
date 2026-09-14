package handlers

// farol_fechamento_comercial.go — Comparativo Fechamento Comercial (Painel
// Vendas), pedido do Claudio 14/09/2026: comparar lado a lado o fechamento
// que o fornecedor manda por fora (planilha própria, ex: Carlos/JC) com o
// que o motor do Farol calculou pro mesmo período — depois de uma sessão
// inteira validando manualmente esse comparativo pra concluir se a base de
// Itens/Clientes Válidos batia (ver farol_metas_calculo.go), virou pedido
// pra ser tela permanente em vez de relatório ad-hoc.
//
// Formato de entrada: o "Resumo Redes" que o fornecedor manda tem os
// números de Cobertura E Sortimento NA MESMA LINHA por Rede — mas quando o
// programa cobre 2+ indústrias (ex: Unilever HC + Foods), CADA Rede aparece
// 2x no arquivo, uma vez por indústria, distinguidas só pelo valor de
// "Objetivo Cobertura" (ex: 9100 pra HC, 1500 pra Foods — achado nesta
// mesma investigação: a duplicata pegou o comparativo de vírgula errado na
// 1ª tentativa manual, comparando HC do Farol com Foods do fornecedor).
// A separação por indústria acontece no NAVEGADOR (frontend), casando o
// "Objetivo Cobertura" de cada linha com o limiar_valor_medio cadastrado em
// cada vínculo de Cobertura da empresa — o backend só recebe o CSV já com
// industria_id resolvido, não tenta adivinhar.
//
// Fonte Farol pro comparativo: farol.metas_realizados_snapshot (mesma
// apuração oficial que qualquer outra tela do Painel de Metas usa) — não
// existe cálculo próprio aqui, só leitura + junção com o que foi importado.

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
)

type fechamentoComercialLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

type fechamentoComercialRow struct {
	linha             int
	industriaID       int
	codPrinc          string
	razao             string
	fantasia          string
	qtLojas           int
	objetivoCobertura float64
	valorVenda        float64
	objetivoEans      float64
	qtEansVendidos    float64
	codGGV, nomeGGV   string
	codCRV, nomeCRV   string
	codRCA, nomeRCA   string
}

var fechamentoComercialColunas = []string{
	"industria_id", "cod_princ", "razao", "fantasia", "qt_lojas",
	"objetivo_cobertura", "valor_venda", "objetivo_eans", "qt_eans_vendidos",
	"cod_ggv", "nome_ggv", "cod_crv", "nome_crv", "cod_rca", "nome_rca",
}

// FechamentoComercialImportarCSVHandler — POST .../fechamento-comercial-importar-csv?data_inicio=&data_fim=
//
// Substitui (PUT-replace) tudo que já existia pra esse intervalo de datas —
// em TODAS as indústrias presentes no CSV, não só uma (mesmo arquivo do
// fornecedor cobre todas de uma vez).
func FechamentoComercialImportarCSVHandler(db *sql.DB) http.HandlerFunc {
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
		for _, c := range []string{"industria_id", "cod_princ", "valor_venda", "qt_eans_vendidos"} {
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

		var rows []fechamentoComercialRow
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
			codPrinc := get(rec, "cod_princ")
			if iErr != nil || codPrinc == "" {
				erros = append(erros, fechamentoComercialLinhaErro{Linha: linhaNum, Erro: "industria_id e cod_princ são obrigatórios"})
				continue
			}
			qtLojas, _ := strconv.Atoi(get(rec, "qt_lojas"))
			row := fechamentoComercialRow{
				linha: linhaNum, industriaID: industriaID, codPrinc: codPrinc,
				razao: get(rec, "razao"), fantasia: get(rec, "fantasia"), qtLojas: qtLojas,
				objetivoCobertura: parseFloat(get(rec, "objetivo_cobertura")),
				valorVenda:        parseFloat(get(rec, "valor_venda")),
				objetivoEans:      parseFloat(get(rec, "objetivo_eans")),
				qtEansVendidos:    parseFloat(get(rec, "qt_eans_vendidos")),
				codGGV:            get(rec, "cod_ggv"), nomeGGV: get(rec, "nome_ggv"),
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
			// Confere que a indústria existe e é desta empresa antes de
			// aceitar linhas dela — evita gravar lixo com industria_id de
			// outra empresa (a UNIQUE constraint não pega isso sozinha).
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
				DELETE FROM farol.fechamento_comercial_externo
				WHERE empresa_id=$1 AND industria_id=$2 AND data_inicio=$3 AND data_fim=$4
			`, spCtx.EmpresaID, industriaID, dataInicio, dataFim); err != nil {
				http.Error(w, `{"error":"database error ao limpar dados anteriores"}`, http.StatusInternalServerError)
				return
			}
		}
		for _, row := range rows {
			if _, err := tx.Exec(`
				INSERT INTO farol.fechamento_comercial_externo
					(empresa_id, industria_id, data_inicio, data_fim, cod_princ, razao, fantasia, qt_lojas,
					 objetivo_cobertura, valor_venda, objetivo_eans, qt_eans_vendidos,
					 cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca, importado_por)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
				ON CONFLICT (industria_id, data_inicio, data_fim, cod_princ) DO UPDATE SET
					razao=EXCLUDED.razao, fantasia=EXCLUDED.fantasia, qt_lojas=EXCLUDED.qt_lojas,
					objetivo_cobertura=EXCLUDED.objetivo_cobertura, valor_venda=EXCLUDED.valor_venda,
					objetivo_eans=EXCLUDED.objetivo_eans, qt_eans_vendidos=EXCLUDED.qt_eans_vendidos,
					cod_ggv=EXCLUDED.cod_ggv, nome_ggv=EXCLUDED.nome_ggv, cod_crv=EXCLUDED.cod_crv, nome_crv=EXCLUDED.nome_crv,
					cod_rca=EXCLUDED.cod_rca, nome_rca=EXCLUDED.nome_rca, importado_em=now(), importado_por=EXCLUDED.importado_por
			`, spCtx.EmpresaID, row.industriaID, dataInicio, dataFim, row.codPrinc, row.razao, row.fantasia, row.qtLojas,
				row.objetivoCobertura, row.valorVenda, row.objetivoEans, row.qtEansVendidos,
				row.codGGV, row.nomeGGV, row.codCRV, row.nomeCRV, row.codRCA, row.nomeRCA, spCtx.UserID); err != nil {
				http.Error(w, `{"error":"database error ao gravar linha `+strconv.Itoa(row.linha)+`: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}
		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "fechamento_comercial_externo", fmt.Sprintf("%s..%s", dataInicio, dataFim), "importar_csv", map[string]any{
			"linhas": len(rows), "industrias": len(industriasNoArquivo),
		}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		log.Printf("FechamentoComercial: %d linhas importadas (%d indústrias, %s..%s) empresa %s por %s",
			len(rows), len(industriasNoArquivo), dataInicio, dataFim, spCtx.EmpresaID, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "linhas_importadas": len(rows), "avisos": erros, "linhas_com_erro": len(erros)})
	}
}

// ─── Comparativo ──────────────────────────────────────────────────────────────

type comparativoLinha struct {
	CodPrinc        string  `json:"cod_princ"`
	Razao           string  `json:"razao"`
	Fantasia        string  `json:"fantasia"`
	QtLojas         int     `json:"qt_lojas"`
	CodGGV          string  `json:"cod_ggv"`
	NomeGGV         string  `json:"nome_ggv"`
	CodCRV          string  `json:"cod_crv"`
	NomeCRV         string  `json:"nome_crv"`
	CodRCA          string  `json:"cod_rca"`
	NomeRCA         string  `json:"nome_rca"`
	ValorVendaExt   float64 `json:"valor_venda_externo"`
	ValorVendaFarol float64 `json:"valor_venda_farol"`
	DiferencaValor  float64 `json:"diferenca_valor"`
	DiferencaValorP float64 `json:"diferenca_valor_pct"`
	EansExt         float64 `json:"eans_externo"`
	EansFarol       float64 `json:"eans_farol"`
	DiferencaEans   float64 `json:"diferenca_eans"`
	Status          string  `json:"status"` // OK | DIVERGE | SO_EXTERNO | SO_FAROL
}

// FechamentoComercialComparativoHandler — GET .../fechamento-comercial-comparativo?industria_id=&data_inicio=&data_fim=
//
// Casa o Fechamento Externo importado com a apuração oficial do Farol
// (farol.metas_realizados_snapshot, mesmo dado que qualquer outra tela lê —
// nível "rede", vigência inteira) pra essa indústria+período. Exige que
// exista vigência de Cobertura E/OU Sortimento com essas MESMAS datas
// (data_inicio/data_fim batendo exato) — não tenta adivinhar aproximação.
func FechamentoComercialComparativoHandler(db *sql.DB) http.HandlerFunc {
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

		// Acha o vínculo de Cobertura e o de Sortimento desta indústria (0
		// ou 1 de cada — uq_farol_metas_vinculos_empresa_industria_tipo já
		// garante isso) com vigência EXATA pro período pedido.
		var vinculoCobID, vigenciaCobID sql.NullInt64
		var vinculoSortID, vigenciaSortID sql.NullInt64
		_ = db.QueryRow(`
			SELECT mv.id, v.id FROM farol.metas_vinculos mv
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id AND tm.formula_codigo = 'cobertura_rede'
			JOIN farol.metas_vigencias v ON v.vinculo_id = mv.id AND v.data_inicio = $3 AND v.data_fim = $4
			WHERE mv.empresa_id = $1 AND mv.industria_id = $2
		`, spCtx.EmpresaID, industriaID, dataInicio, dataFim).Scan(&vinculoCobID, &vigenciaCobID)
		_ = db.QueryRow(`
			SELECT mv.id, v.id FROM farol.metas_vinculos mv
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id AND tm.formula_codigo = 'sortimento_rede'
			JOIN farol.metas_vigencias v ON v.vinculo_id = mv.id AND v.data_inicio = $3 AND v.data_fim = $4
			WHERE mv.empresa_id = $1 AND mv.industria_id = $2
		`, spCtx.EmpresaID, industriaID, dataInicio, dataFim).Scan(&vinculoSortID, &vigenciaSortID)

		if !vigenciaCobID.Valid && !vigenciaSortID.Valid {
			http.Error(w, `{"error":"nenhuma vigência de Cobertura ou Sortimento desta indústria bate com esse período exato — cadastre a vigência primeiro"}`, http.StatusBadRequest)
			return
		}

		cobPorRede := map[string]RealizadoRede{}
		sortPorRede := map[string]RealizadoRede{}
		if vigenciaCobID.Valid {
			res, err := obterOuCongelarRealizado(db, spCtx.EmpresaID, int(vinculoCobID.Int64), int(vigenciaCobID.Int64), "faturado", "rede")
			if err == nil {
				for _, rd := range res.Redes {
					cobPorRede[rd.CodPrinc] = rd
				}
			}
		}
		if vigenciaSortID.Valid {
			res, err := obterOuCongelarRealizado(db, spCtx.EmpresaID, int(vinculoSortID.Int64), int(vigenciaSortID.Int64), "faturado", "rede")
			if err == nil {
				for _, rd := range res.Redes {
					sortPorRede[rd.CodPrinc] = rd
				}
			}
		}

		rows, err := db.Query(`
			SELECT cod_princ, razao, fantasia, qt_lojas, valor_venda, qt_eans_vendidos,
			       cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca
			FROM farol.fechamento_comercial_externo
			WHERE empresa_id = $1 AND industria_id = $2 AND data_inicio = $3 AND data_fim = $4
		`, spCtx.EmpresaID, industriaID, dataInicio, dataFim)
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		externoPorRede := map[string]comparativoLinha{}
		var ordem []string
		for rows.Next() {
			var l comparativoLinha
			if err := rows.Scan(&l.CodPrinc, &l.Razao, &l.Fantasia, &l.QtLojas, &l.ValorVendaExt, &l.EansExt,
				&l.CodGGV, &l.NomeGGV, &l.CodCRV, &l.NomeCRV, &l.CodRCA, &l.NomeRCA); err != nil {
				http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
				return
			}
			externoPorRede[l.CodPrinc] = l
			ordem = append(ordem, l.CodPrinc)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}

		vistos := map[string]bool{}
		var out []comparativoLinha
		processar := func(cp string) {
			if vistos[cp] {
				return
			}
			vistos[cp] = true
			l, temExterno := externoPorRede[cp]
			l.CodPrinc = cp
			cob, temCob := cobPorRede[cp]
			sort_, temSort := sortPorRede[cp]
			if temCob {
				l.ValorVendaFarol = cob.ValorTotal
				if l.Razao == "" {
					l.Razao, l.Fantasia = cob.Razao, cob.Fantasia
				}
				if l.QtLojas == 0 {
					l.QtLojas = cob.QtLojas
				}
				if l.CodGGV == "" {
					l.CodGGV, l.NomeGGV, l.CodCRV, l.NomeCRV, l.CodRCA, l.NomeRCA = cob.CodGGV, cob.NomeGGV, cob.CodCRV, cob.NomeCRV, cob.CodRCA, cob.NomeRCA
				}
			}
			if temSort {
				l.EansFarol = sort_.Valor
				if l.Razao == "" {
					l.Razao, l.Fantasia = sort_.Razao, sort_.Fantasia
				}
				if l.QtLojas == 0 {
					l.QtLojas = sort_.QtLojas
				}
				if l.CodGGV == "" {
					l.CodGGV, l.NomeGGV, l.CodCRV, l.NomeCRV, l.CodRCA, l.NomeRCA = sort_.CodGGV, sort_.NomeGGV, sort_.CodCRV, sort_.NomeCRV, sort_.CodRCA, sort_.NomeRCA
				}
			}
			temFarol := temCob || temSort
			l.DiferencaValor = l.ValorVendaFarol - l.ValorVendaExt
			if l.ValorVendaExt != 0 {
				l.DiferencaValorP = l.DiferencaValor / l.ValorVendaExt * 100
			}
			l.DiferencaEans = l.EansFarol - l.EansExt
			switch {
			case !temExterno:
				l.Status = "SO_FAROL"
			case !temFarol:
				l.Status = "SO_EXTERNO"
			case absFloat(l.DiferencaValorP) > 5 || absFloat(l.DiferencaEans) >= 2:
				l.Status = "DIVERGE"
			default:
				l.Status = "OK"
			}
			out = append(out, l)
		}
		for _, cp := range ordem {
			processar(cp)
		}
		for cp := range cobPorRede {
			processar(cp)
		}
		for cp := range sortPorRede {
			processar(cp)
		}

		sort.Slice(out, func(i, j int) bool {
			oi, oj := statusOrdem(out[i].Status), statusOrdem(out[j].Status)
			if oi != oj {
				return oi < oj
			}
			return absFloat(out[i].DiferencaValorP) > absFloat(out[j].DiferencaValorP)
		})

		json.NewEncoder(w).Encode(map[string]any{"linhas": out, "total": len(out)})
	}
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func statusOrdem(s string) int {
	switch s {
	case "DIVERGE":
		return 0
	case "SO_EXTERNO":
		return 1
	case "SO_FAROL":
		return 2
	default:
		return 3
	}
}
