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
// ─── Migration 236 (14/09/2026) — de "ao vivo" pra "agregado persistido" ────
// Até aqui esta tela calculava tudo por request: somava Qtd/Valor por
// cod_prod pros CNPJs do escopo pedido, e pros itens SEM nenhuma venda ali
// buscava o nome mais recente escaneando vendas_faturadas/vendas_transmitidas
// inteiras (sem filtro de data — nome de produto não muda com o tempo, então
// nunca dava pra restringir por período). Isso mediu 51s em produção pra uma
// rede de 1 loja só. Além do gargalo, o Claudio pediu (mesma sessão) pra
// reaproveitar esse "o que não vendeu" em painéis futuros, principalmente a
// visão do RCA no mobile (ION VENDAS).
//
// RecalcularItensRealizado agora faz esse cálculo 1x pra TODOS os clientes
// válidos da vigência (não só o escopo de uma request) e grava em
// farol.metas_itens_realizado (grão CNPJ×EAN) — chamado no prewarm diário
// (farol_metas_prewarm.go) e logo após reimportar Itens/Clientes Válidos
// (a lista mudou, o cálculo anterior fica obsoleto na hora). A leitura
// (calcularItensPorEscopo) virou um SELECT simples filtrado pelos CNPJs do
// escopo, sem nenhum cálculo nem scan de vendas_* na hora da request.
//
// Nome do produto: vendas_faturadas/transmitidas trazem nome_prod, mas só
// pra quem TEM venda no escopo/período pedido. Item sem nenhuma venda ali
// cai no fallback de nomesHistoricosPorCodProd — não existe cadastro de
// produto independente de venda no Farol, só o que vem denormalizado na
// própria linha de venda (migration 168). Essa busca lê de
// farol.agg_sazonalidade_produto_ano (grão empresa×ano×cod_prod, ~54 mil
// linhas nesta empresa, cod_prod na chave primária) em vez de vendas_*
// direto (milhões de linhas, particionado por data, sem índice que sirva
// um filtro só por cod_prod).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// qtdValorPorCodProdPorCliente soma quantidade E valor (pvenda) vendidos por
// cod_prod, POR CNPJ (GROUP BY cnpj, cod_prod) — grão fino que
// RecalcularItensRealizado precisa pra gravar 1 linha por CNPJ×EAN. Mesmo
// princípio de qtdPorCodProdClientes (farol_metas_calculo.go): 1 consulta
// cobrindo todos os CNPJs da vigência, não 1 por CNPJ.
func qtdValorPorCodProdPorCliente(db *sql.DB, empresaID string, clientes []clienteValido, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) (map[string]map[string]itemAgregado, error) {
	out := map[string]map[string]itemAgregado{}
	if len(clientes) == 0 {
		return out, nil
	}
	cnpjs, codPrincs := cnpjCodPrincPares(clientes)
	somar := func(tabela, colData string) error {
		t0 := time.Now()
		joinSQL, joinArgs := filtrarPorClienteEDono("v", 2, cnpjs, codPrincs)
		query := fmt.Sprintf(`
			SELECT v.cnpj, v.cod_prod, SUM(v.qt), SUM(v.pvenda), MAX(v.nome_prod) FROM %s v
			%s
			WHERE v.empresa_id = $1 AND v.%s BETWEEN $4 AND $5 AND v.cod_prod <> ''
		`, tabela, joinSQL, colData)
		args := append([]any{empresaID}, joinArgs...)
		args = append(args, dataInicio, dataFim)
		if len(tiposVenda) > 0 {
			query += fmt.Sprintf(" AND v.tipo_venda = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(tiposVenda))
		}
		if len(codFornec) > 0 {
			query += fmt.Sprintf(" AND v.cod_fornec = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(codFornec))
		}
		query += " GROUP BY v.cnpj, v.cod_prod"
		rows, err := db.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var cnpj, codProd, nome string
			var qt, valor float64
			if err := rows.Scan(&cnpj, &codProd, &qt, &valor, &nome); err != nil {
				return err
			}
			porCliente, ok := out[cnpj]
			if !ok {
				porCliente = map[string]itemAgregado{}
				out[cnpj] = porCliente
			}
			a := porCliente[codProd]
			a.Qtd += qt
			a.Valor += valor
			if nome != "" {
				a.Nome = nome
			}
			porCliente[codProd] = a
			n++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		log.Printf("[farol:objetivos] qtdValorPorCodProdPorCliente tabela=%s cnpjs=%d período=[%s..%s] → %d linhas em %v",
			tabela, len(cnpjs), dataInicio, dataFim, n, time.Since(t0))
		return nil
	}
	switch fluxo {
	case "faturado":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
		if err := subtrairDevolucaoCancelamentoItens(db, empresaID, cnpjs, codPrincs, dataInicio, dataFim, codFornec, out); err != nil {
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

// subtrairDevolucaoCancelamentoItens espelha subtrairDevolucaoCancelamentoQtd
// (farol_metas_calculo.go) pro grão Qtd+Valor deste drill-down — mesmo
// racional: só Faturado, líquido direto (sem separar bruto/devolução em
// campo à parte), sem filtro de tipo_venda (vendas_ccd não tem essa coluna),
// e mesmo filtro por cod_cliprinc (ver filtrarPorClienteEDono).
func subtrairDevolucaoCancelamentoItens(db *sql.DB, empresaID string, cnpjs, codPrincs []string, dataInicio, dataFim string, codFornec []string, out map[string]map[string]itemAgregado) error {
	t0 := time.Now()
	joinSQL, joinArgs := filtrarPorClienteEDono("v", 2, cnpjs, codPrincs)
	query := fmt.Sprintf(`
		SELECT v.cnpj, v.cod_prod, SUM(v.qt), SUM(v.pvenda), MAX(v.nome_prod) FROM vendas_ccd v
		%s
		WHERE v.empresa_id = $1 AND v.data_evento BETWEEN $4 AND $5
		  AND v.cod_prod <> '' AND v.evento IN ('DEVOLVIDO', 'CANCELADO')
	`, joinSQL)
	args := append([]any{empresaID}, joinArgs...)
	args = append(args, dataInicio, dataFim)
	if len(codFornec) > 0 {
		query += fmt.Sprintf(" AND v.cod_fornec = ANY($%d)", len(args)+1)
		args = append(args, pq.Array(codFornec))
	}
	query += " GROUP BY v.cnpj, v.cod_prod"
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var cnpj, codProd, nome string
		var qt, valor float64
		if err := rows.Scan(&cnpj, &codProd, &qt, &valor, &nome); err != nil {
			return err
		}
		porCliente, ok := out[cnpj]
		if !ok {
			porCliente = map[string]itemAgregado{}
			out[cnpj] = porCliente
		}
		a := porCliente[codProd]
		a.Qtd -= qt
		a.Valor -= valor
		if a.Nome == "" && nome != "" {
			a.Nome = nome
		}
		porCliente[codProd] = a
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("[farol:objetivos] subtrairDevolucaoCancelamentoItens cnpjs=%d período=[%s..%s] → %d grupos em %v",
		len(cnpjs), dataInicio, dataFim, n, time.Since(t0))
	return nil
}

// nomesHistoricosPorCodProd resolve o nome de exibição de cod_prods que não
// tiveram NENHUMA venda, de nenhum cliente, na vigência inteira — pega o
// nome mais recente daquele cod_prod (qualquer cliente/período do Farol).
// Lista vazia não gera query.
func nomesHistoricosPorCodProd(db *sql.DB, empresaID string, codProds []string) (map[string]string, error) {
	out := map[string]string{}
	if len(codProds) == 0 {
		return out, nil
	}
	rows, err := db.Query(`
		SELECT DISTINCT ON (cod_prod) cod_prod, nome_prod
		FROM farol.agg_sazonalidade_produto_ano
		WHERE empresa_id = $1 AND cod_prod = ANY($2) AND nome_prod <> ''
		ORDER BY cod_prod, ano DESC
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

// RecalcularItensRealizado recalcula e grava (replace total, por
// vigência+fluxo) o realizado por CNPJ×EAN de uma vigência de Sortimento —
// TODOS os clientes válidos dela, não um escopo/request específico. Vínculo
// de Cobertura (sem itens_validos) é no-op silencioso — quem chama (prewarm,
// import de Itens/Clientes Válidos) não precisa checar formula_codigo antes.
//
// Chamada de: PrewarmMetasRealizados (1x/dia, junto do snapshot de
// Realizado) e logo após reimportar farol.metas_itens_validos ou
// farol.metas_clientes_validos de uma vigência (a lista mudou, o cálculo
// anterior fica obsoleto na hora — sem isso o diálogo "Itens" mostraria
// dado velho até o prewarm do dia seguinte rodar).
func RecalcularItensRealizado(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo string) error {
	var dataInicio, dataFim, formulaCodigo string
	var industriaID int
	if err := db.QueryRow(`
		SELECT v.data_inicio::text, v.data_fim::text, mv.industria_id, tm.formula_codigo
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3
	`, vigenciaID, vinculoID, empresaID).Scan(&dataInicio, &dataFim, &industriaID, &formulaCodigo); err != nil {
		return fmt.Errorf("vínculo/vigência não encontrado: %w", err)
	}
	if formulaCodigo != "sortimento_rede" {
		return nil // Cobertura não tem itens_validos — nada a fazer.
	}

	t0 := time.Now()
	itens, err := lerItensValidos(db, empresaID, vigenciaID)
	if err != nil {
		return err
	}
	if len(itens) == 0 {
		return nil // vigência sem Itens Válidos importados ainda.
	}
	clientes, err := lerClientesValidos(db, empresaID, vigenciaID)
	if err != nil {
		return err
	}
	if len(clientes) == 0 {
		return nil // vigência sem Clientes Válidos importados ainda.
	}

	var tiposVendaValidos []string
	if err := db.QueryRow(`SELECT tipos_venda_validos FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2`, vinculoID, empresaID).
		Scan(pq.Array(&tiposVendaValidos)); err != nil {
		return fmt.Errorf("erro ao ler tipos_venda_validos: %w", err)
	}
	codFornec, err := codFornecDaIndustria(db, empresaID, industriaID)
	if err != nil {
		return err
	}

	linhasPorCliente, err := qtdValorPorCodProdPorCliente(db, empresaID, clientes, dataInicio, dataFim, fluxo, tiposVendaValidos, codFornec)
	if err != nil {
		return err
	}

	// Nome dos itens que NENHUM cliente da vigência vendeu (só esses
	// precisam do fallback histórico — os outros já vieram com nome de
	// alguma venda real acima).
	nomePorCodProd := map[string]string{}
	for _, linhas := range linhasPorCliente {
		for codProd, a := range linhas {
			if a.Nome != "" {
				nomePorCodProd[codProd] = a.Nome
			}
		}
	}
	var semNenhumaVenda []string
	for _, it := range itens {
		if _, ok := nomePorCodProd[it.CodProd]; !ok {
			semNenhumaVenda = append(semNenhumaVenda, it.CodProd)
		}
	}
	nomesFallback, err := nomesHistoricosPorCodProd(db, empresaID, semNenhumaVenda)
	if err != nil {
		return err
	}
	nomeDoCodProd := func(codProd string) string {
		if n := nomePorCodProd[codProd]; n != "" {
			return n
		}
		return nomesFallback[codProd]
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM farol.metas_itens_realizado WHERE vigencia_id = $1 AND fluxo = $2`, vigenciaID, fluxo); err != nil {
		return err
	}
	stmt, err := tx.Prepare(pq.CopyInSchema("farol", "metas_itens_realizado",
		"empresa_id", "vinculo_id", "vigencia_id", "fluxo", "cnpj", "ean", "nome", "qtd", "valor", "vendeu"))
	if err != nil {
		return err
	}
	for _, c := range clientes {
		porCodProd := linhasPorCliente[c.CNPJ]
		// Agrega por EAN antes de gravar — um EAN pode ter N cod_prod (mesmo
		// princípio de contarEANsPositivados em farol_metas_calculo.go); sem
		// isso a mesma loja geraria 2 linhas pro mesmo EAN e violaria o
		// UNIQUE (vigencia_id, fluxo, cnpj, ean).
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
			if l, ok2 := porCodProd[it.CodProd]; ok2 {
				a.Qtd += l.Qtd
				a.Valor += l.Valor
				if l.Qtd > 0 {
					a.Vendeu = true
				}
			}
			if a.Nome == "" {
				a.Nome = nomeDoCodProd(it.CodProd)
			}
		}
		for _, ean := range ordem {
			a := porEan[ean]
			if _, err := stmt.Exec(empresaID, vinculoID, vigenciaID, fluxo, c.CNPJ, ean, a.Nome, a.Qtd, a.Valor, a.Vendeu); err != nil {
				return err
			}
		}
	}
	if _, err := stmt.Exec(); err != nil {
		return err
	}
	if err := stmt.Close(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("[farol:objetivos] RecalcularItensRealizado vinculo=%d vigencia=%d fluxo=%s → %d clientes × %d itens em %v",
		vinculoID, vigenciaID, fluxo, len(clientes), len(itens), time.Since(t0))
	return nil
}

// calcularItensPorEscopo lê o drill-down de itens (vendeu/não vendeu, Qtd,
// Valor) já persistido em farol.metas_itens_realizado, pro conjunto de
// CNPJs pedido — Rede inteira (todas as lojas) ou uma loja só, dependendo
// de quantos cnpjs o chamador passar. Soma por EAN (bool_or pro "vendeu",
// já que uma Rede tem várias lojas — basta 1 vender pra contar "vendeu" na
// visão agregada da Rede).
func calcularItensPorEscopo(db *sql.DB, empresaID string, vigenciaID int, fluxo string, cnpjs []string) ([]PainelItemLinha, error) {
	rows, err := db.Query(`
		SELECT ean, MAX(nome) FILTER (WHERE nome <> ''), SUM(qtd), SUM(valor), bool_or(vendeu)
		FROM farol.metas_itens_realizado
		WHERE empresa_id = $1 AND vigencia_id = $2 AND fluxo = $3 AND cnpj = ANY($4)
		GROUP BY ean
		ORDER BY bool_or(vendeu), MAX(nome), ean
	`, empresaID, vigenciaID, fluxo, pq.Array(cnpjs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PainelItemLinha
	for rows.Next() {
		var it PainelItemLinha
		var nome sql.NullString
		if err := rows.Scan(&it.EAN, &nome, &it.Qtd, &it.Valor, &it.Vendeu); err != nil {
			return nil, err
		}
		it.Nome = nome.String
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("nenhum Item Válido calculado ainda pra esta vigência — aguarde o próximo prewarm ou reimporte os Itens Válidos")
	}
	return out, nil
}

// cnpjsDoEscopoNaVigencia resolve os CNPJs de uma Rede (cod_princ) ou de
// uma loja só (cnpj), dentro do escopo de login (GGV/CRV/RCA da persona) —
// mesmo princípio de segurança de filtrarRedesPorHierarquia: nunca deixa
// escapar CNPJ fora do organograma do usuário.
func cnpjsDoEscopoNaVigencia(db *sql.DB, empresaID string, vigenciaID int, codPrinc, cnpjUnico, codGGV, codCRV, codRCA string) ([]string, error) {
	query := `SELECT cnpj FROM farol.metas_clientes_validos WHERE vigencia_id = $1 AND empresa_id = $2`
	args := []any{vigenciaID, empresaID}
	// cnpjUnico/codPrinc são independentes (não if/else): um drill-down de
	// GGV×CRV (ou GGV×CRV×RCA) chega aqui sem nenhum dos dois — só com
	// codGGV/codCRV abaixo — e o if/else antigo forçava "AND cod_princ = ''"
	// nesse caso, que nunca bate com nada (bug achado 18/09/2026, pedido do
	// Claudio pra estender o drill-down de itens pras abas de rollup).
	if cnpjUnico != "" {
		query += fmt.Sprintf(" AND cnpj = $%d", len(args)+1)
		args = append(args, cnpjUnico)
	} else if codPrinc != "" {
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
//	?vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=&cod_princ=              (Rede)
//	?vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=&cnpj=                   (Loja)
//	?vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=&cod_ggv=&cod_crv=       (Resumo GGVs×CRVs)
//	?vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=&cod_ggv=&cod_crv=&cod_rca= (Resumo GGVs×CRVs×RCAs)
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
		_, err1 := strconv.Atoi(q.Get("vinculo_sortimento_id"))
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

		codGGV, codCRV, codRCA, negarEscopo := escopoHierarquiaMetas(spCtx)
		if negarEscopo {
			http.Error(w, `{"error":"acesso negado: cadastro de organograma incompleto pra este usuário"}`, http.StatusForbidden)
			return
		}
		codGGV, codCRV, codRCA = resolverFiltroDrillDown(q, codGGV, codCRV, codRCA)

		// Aceita 4 formas de escopo — Rede, Loja, ou o rollup de uma linha
		// das abas "Resumo GGVs×CRVs"/"...×RCAs" (cod_ggv+cod_crv, com ou sem
		// cod_rca). Pedido do Claudio em 18/09/2026: o drill-down de itens
		// que já existia pra Rede/Cliente também abrir clicando numa linha
		// dessas duas abas de rollup.
		if codPrinc == "" && cnpjUnico == "" && codGGV == "" && codCRV == "" {
			http.Error(w, `{"error":"informe cod_princ (Rede), cnpj (Loja) ou cod_ggv+cod_crv (Resumo GGVs×CRVs)"}`, http.StatusBadRequest)
			return
		}

		cnpjs, err := cnpjsDoEscopoNaVigencia(db, spCtx.EmpresaID, vigenciaID, codPrinc, cnpjUnico, codGGV, codCRV, codRCA)
		if err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		if len(cnpjs) == 0 {
			json.NewEncoder(w).Encode(map[string]any{"itens": []PainelItemLinha{}})
			return
		}

		itens, err := calcularItensPorEscopo(db, spCtx.EmpresaID, vigenciaID, fluxo, cnpjs)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"itens": itens})
	}
}
