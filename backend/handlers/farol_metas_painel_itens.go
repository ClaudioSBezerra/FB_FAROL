package handlers

// farol_metas_painel_itens.go — Drill-down de itens (Sortimento) por Rede
// ou por Loja: "quais EANs venderam e quais não venderam", com Quantidade
// e Valor — pedido do Claudio em 10/09/2026 pra complementar a visão
// Combinado (clicar numa Rede mostra os itens agregados de TODAS as lojas
// dela; clicar numa loja mostra só os itens daquele CNPJ).
//
// Reaproveita o motor de Sortimento (farol_metas_calculo.go): mesma regra
// de "produto vendido não está na lista de Itens Válidos → ignora" e mesmo
// filtro por cod_fornec/tipos_venda_validos do vínculo. Diferença: aqui o
// grão de saída é POR EAN (Qtd/Valor/Vendeu), não "quantos EANs distintos"
// — é uma ferramenta operacional ("o que falta vender"), não o indicador
// oficial (que seguiu intocado em farol_metas_calculo.go).
//
// Nome do produto: vendas_faturadas/transmitidas trazem nome_prod, mas só
// pra quem TEM venda no escopo/período pedido. Item sem nenhuma venda ali
// cai no fallback de nomesHistoricosPorCodProd (última venda conhecida
// daquele cod_prod, em QUALQUER cliente/período) — sem isso, um item nunca
// vendido àquele cliente ficaria sem nome nenhum (não existe cadastro de
// produto independente de venda no Farol, só o que vem denormalizado na
// própria linha de venda, migration 168).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// PainelItemLinha — 1 EAN do drill-down: se vendeu ou não no escopo pedido
// (Rede inteira ou uma loja só), quanto vendeu (Qtd) e por quanto (Valor).
type PainelItemLinha struct {
	EAN    string  `json:"ean"`
	Nome   string  `json:"nome"`
	Qtd    float64 `json:"qtd"`
	Valor  float64 `json:"valor"`
	Vendeu bool    `json:"vendeu"`
}

type itemAgregado struct {
	Qtd   float64
	Valor float64
	Nome  string
}

// somaQtdValorPorCodProd soma quantidade E valor (pvenda) vendidos por
// cod_prod, pros CNPJs pedidos, numa única consulta agregada — mesmo
// princípio de somaPvendaClientes/qtdPorCodProdClientes (farol_metas_calculo.go):
// 1 consulta cobrindo todos os CNPJs, não 1 por CNPJ.
func somaQtdValorPorCodProd(db *sql.DB, empresaID string, cnpjs []string, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) (map[string]itemAgregado, error) {
	out := map[string]itemAgregado{}
	if len(cnpjs) == 0 {
		return out, nil
	}
	somar := func(tabela, colData string) error {
		query := fmt.Sprintf(`
			SELECT cod_prod, SUM(qt), SUM(pvenda), MAX(nome_prod) FROM %s
			WHERE empresa_id = $1 AND cnpj = ANY($2) AND %s BETWEEN $3 AND $4 AND cod_prod <> ''
		`, tabela, colData)
		args := []any{empresaID, pq.Array(cnpjs), dataInicio, dataFim}
		if len(tiposVenda) > 0 {
			query += fmt.Sprintf(" AND tipo_venda = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(tiposVenda))
		}
		if len(codFornec) > 0 {
			query += fmt.Sprintf(" AND cod_fornec = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(codFornec))
		}
		query += " GROUP BY cod_prod"
		rows, err := db.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var codProd, nome string
			var qt, valor float64
			if err := rows.Scan(&codProd, &qt, &valor, &nome); err != nil {
				return err
			}
			a := out[codProd]
			a.Qtd += qt
			a.Valor += valor
			if nome != "" {
				a.Nome = nome
			}
			out[codProd] = a
		}
		return rows.Err()
	}
	switch fluxo {
	case "faturado":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
	case "transmitido":
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("fluxo inválido: %q (use faturado ou transmitido)", fluxo)
	}
	return out, nil
}

// nomesHistoricosPorCodProd resolve o nome de exibição de cod_prods que
// não tiveram nenhuma venda no escopo/período pedido — pega a venda mais
// recente daquele cod_prod em QUALQUER cliente/período do Farol (não existe
// cadastro de produto independente de venda). Lista vazia não gera query.
func nomesHistoricosPorCodProd(db *sql.DB, empresaID string, codProds []string) (map[string]string, error) {
	out := map[string]string{}
	if len(codProds) == 0 {
		return out, nil
	}
	rows, err := db.Query(`
		SELECT DISTINCT ON (cod_prod) cod_prod, nome_prod FROM (
			SELECT cod_prod, nome_prod, data_faturamento AS data FROM vendas_faturadas
				WHERE empresa_id = $1 AND cod_prod = ANY($2) AND nome_prod <> ''
			UNION ALL
			SELECT cod_prod, nome_prod, data_transmissao AS data FROM vendas_transmitidas
				WHERE empresa_id = $1 AND cod_prod = ANY($2) AND nome_prod <> ''
		) x
		ORDER BY cod_prod, data DESC
	`, empresaID, pq.Array(codProds))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var codProd, nome string
		if err := rows.Scan(&codProd, &nome); err != nil {
			return nil, err
		}
		out[codProd] = nome
	}
	return out, rows.Err()
}

// calcularItensPorEscopo monta o drill-down de itens (vendeu/não vendeu,
// Qtd, Valor) pro conjunto de CNPJs pedido — Rede inteira (todas as lojas)
// ou uma loja só, dependendo de quantos cnpjs o chamador passar. Agrupa por
// EAN (não por cod_prod): um EAN pode ter N cod_prod — ver cabeçalho do
// arquivo de Sortimento — e o indicador de "vendeu" é por EAN, igual ao
// oficial (contarEANsPositivados).
func calcularItensPorEscopo(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo string, cnpjs []string) ([]PainelItemLinha, error) {
	var dataInicio, dataFim, formulaCodigo string
	var industriaID int
	err := db.QueryRow(`
		SELECT v.data_inicio::text, v.data_fim::text, mv.industria_id, tm.formula_codigo
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3
	`, vigenciaID, vinculoID, empresaID).Scan(&dataInicio, &dataFim, &industriaID, &formulaCodigo)
	if err != nil {
		return nil, fmt.Errorf("vínculo/vigência não encontrado: %w", err)
	}
	if formulaCodigo != "sortimento_rede" {
		return nil, fmt.Errorf("drill-down de itens só existe pro Tipo de Métrica Sortimento (formula_codigo=sortimento_rede)")
	}

	var tiposVendaValidos []string
	if err := db.QueryRow(`SELECT tipos_venda_validos FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2`, vinculoID, empresaID).
		Scan(pq.Array(&tiposVendaValidos)); err != nil {
		return nil, fmt.Errorf("erro ao ler tipos_venda_validos: %w", err)
	}
	codFornec, err := codFornecDaIndustria(db, empresaID, industriaID)
	if err != nil {
		return nil, err
	}
	itens, err := lerItensValidos(db, empresaID, vigenciaID)
	if err != nil {
		return nil, err
	}
	if len(itens) == 0 {
		return nil, fmt.Errorf("nenhum Item Válido importado pra esta vigência")
	}

	linhasPorCodProd, err := somaQtdValorPorCodProd(db, empresaID, cnpjs, dataInicio, dataFim, fluxo, tiposVendaValidos, codFornec)
	if err != nil {
		return nil, err
	}

	var faltantes []string
	for _, it := range itens {
		if _, ok := linhasPorCodProd[it.CodProd]; !ok {
			faltantes = append(faltantes, it.CodProd)
		}
	}
	nomesFallback, err := nomesHistoricosPorCodProd(db, empresaID, faltantes)
	if err != nil {
		return nil, err
	}

	type acc struct {
		Qtd, Valor float64
		Nome       string
		Vendeu     bool
	}
	porEan := map[string]*acc{}
	var ordem []string
	for _, it := range itens {
		a, ok := porEan[it.EAN]
		if !ok {
			a = &acc{}
			porEan[it.EAN] = a
			ordem = append(ordem, it.EAN)
		}
		if l, ok := linhasPorCodProd[it.CodProd]; ok {
			a.Qtd += l.Qtd
			a.Valor += l.Valor
			if l.Qtd > 0 {
				a.Vendeu = true
			}
			if a.Nome == "" && l.Nome != "" {
				a.Nome = l.Nome
			}
		}
		if a.Nome == "" {
			a.Nome = nomesFallback[it.CodProd]
		}
	}

	out := make([]PainelItemLinha, 0, len(ordem))
	for _, ean := range ordem {
		a := porEan[ean]
		out = append(out, PainelItemLinha{EAN: ean, Nome: a.Nome, Qtd: a.Qtd, Valor: a.Valor, Vendeu: a.Vendeu})
	}
	// Não vendidos primeiro — é o que direciona a ação (o que falta vender);
	// dentro de cada grupo, ordem alfabética pelo nome (EAN como desempate).
	sort.Slice(out, func(i, j int) bool {
		if out[i].Vendeu != out[j].Vendeu {
			return !out[i].Vendeu
		}
		if out[i].Nome != out[j].Nome {
			return out[i].Nome < out[j].Nome
		}
		return out[i].EAN < out[j].EAN
	})
	return out, nil
}

// cnpjsDoEscopoNaVigencia resolve os CNPJs de uma Rede (cod_princ) ou de
// uma loja só (cnpj), dentro do escopo de login (GGV/CRV/RCA da persona) —
// mesmo princípio de segurança de filtrarRedesPorHierarquia: nunca deixa
// escapar CNPJ fora do organograma do usuário.
func cnpjsDoEscopoNaVigencia(db *sql.DB, empresaID string, vigenciaID int, codPrinc, cnpjUnico, codGGV, codCRV, codRCA string) ([]string, error) {
	query := `SELECT cnpj FROM farol.metas_clientes_validos WHERE vigencia_id = $1 AND empresa_id = $2`
	args := []any{vigenciaID, empresaID}
	if cnpjUnico != "" {
		query += fmt.Sprintf(" AND cnpj = $%d", len(args)+1)
		args = append(args, cnpjUnico)
	} else {
		query += fmt.Sprintf(" AND cod_princ = $%d", len(args)+1)
		args = append(args, codPrinc)
	}
	if codGGV != "" {
		query += fmt.Sprintf(" AND cod_ggv = $%d", len(args)+1)
		args = append(args, codGGV)
	}
	if codCRV != "" {
		query += fmt.Sprintf(" AND cod_crv = $%d", len(args)+1)
		args = append(args, codCRV)
	}
	if codRCA != "" {
		query += fmt.Sprintf(" AND cod_rca = $%d", len(args)+1)
		args = append(args, codRCA)
	}
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var cnpj string
		if err := rows.Scan(&cnpj); err != nil {
			return nil, err
		}
		out = append(out, cnpj)
	}
	return out, rows.Err()
}

// MetasPainelItensHandler — GET /api/farol/metas-painel-itens
//
//	?vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=&cod_princ=  (Rede)
//	?vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=&cnpj=       (Loja)
func MetasPainelItensHandler(db *sql.DB) http.HandlerFunc {
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
		q := r.URL.Query()
		vinculoID, err1 := strconv.Atoi(q.Get("vinculo_sortimento_id"))
		vigenciaID, err2 := strconv.Atoi(q.Get("vigencia_sortimento_id"))
		if err1 != nil || err2 != nil {
			http.Error(w, `{"error":"vinculo_sortimento_id e vigencia_sortimento_id são obrigatórios"}`, http.StatusBadRequest)
			return
		}
		fluxo := q.Get("fluxo")
		if fluxo == "" {
			fluxo = "faturado"
		}
		codPrinc := strings.TrimSpace(q.Get("cod_princ"))
		cnpjUnico := strings.TrimSpace(q.Get("cnpj"))
		if codPrinc == "" && cnpjUnico == "" {
			http.Error(w, `{"error":"informe cod_princ (Rede) ou cnpj (Loja)"}`, http.StatusBadRequest)
			return
		}

		codGGV, codCRV, codRCA, negarEscopo := escopoHierarquiaMetas(spCtx)
		if negarEscopo {
			http.Error(w, `{"error":"acesso negado: cadastro de organograma incompleto pra este usuário"}`, http.StatusForbidden)
			return
		}
		codGGV, codCRV, codRCA = resolverFiltroDrillDown(q, codGGV, codCRV, codRCA)

		cnpjs, err := cnpjsDoEscopoNaVigencia(db, spCtx.EmpresaID, vigenciaID, codPrinc, cnpjUnico, codGGV, codCRV, codRCA)
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		if len(cnpjs) == 0 {
			json.NewEncoder(w).Encode(map[string]any{"itens": []PainelItemLinha{}})
			return
		}

		itens, err := calcularItensPorEscopo(db, spCtx.EmpresaID, vinculoID, vigenciaID, fluxo, cnpjs)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"itens": itens})
	}
}
