package handlers

// farol_metas_calculo_numerica.go — Motor de Apuração da Numérica (Épico 7
// addendum, 2026-09-29 — ver _bmad-output/planning-artifacts/epics.md,
// Story 7.4/7.5): Cobertura e Sortimento por Cliente/CNPJ direto, sem Rede.
//
// Trick de reuso: cada Cliente vira um RealizadoRede "de 1" (CodPrinc=CNPJ,
// QtLojas=1, Clientes=[ele mesmo]). Isso NÃO é o mesmo "Cliente = Rede de 1"
// já descartado como insuficiente durante o addendum do PRD (Questão 4/5) —
// lá a objeção era tratar a Numérica como se tivesse os MESMOS parâmetros
// fixos do motor Rede (1 limiar só por vínculo), o que é errado porque o
// limiar de Cobertura varia por Classificação PDV do cliente. Aqui os
// limiares/regras são resolvidos DENTRO destas funções, específicos da
// Numérica — só o FORMATO de retorno (RealizadoRede/RealizadoCliente) é
// reaproveitado, pra ganhar de graça toda a camada de agregação/painel já
// pronta (agregarPorNivel, filtrarRedesPorHierarquia, painel web/mobile,
// snapshot) — nenhuma dessas funções sabe, nem precisa saber, que atrás de
// um "CodPrinc" pode não haver Rede nenhuma.

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
)

// clienteNumericaValido é uma linha crua de farol.metas_clientes_numericas
// (migration 248) — formato diferente de clienteValido: sem cod_princ
// (FR24), com Classificação PDV, e GGV/CRV/RCA vêm PRONTOS do arquivo da
// JC (não resolvidos por JOIN com a hierarquia organizacional do Farol).
type clienteNumericaValido struct {
	CNPJ             string
	CodCl            string
	ClassificacaoPDV string
	Razao            string
	Fantasia         string
	CodGGV           string
	NomeGGV          string
	CodCRV           string
	NomeCRV          string
	CodRCA           string
	NomeRCA          string
}

// ppaValido é uma linha crua de farol.metas_ppas — cod_prod → PPA (nome da
// família). EAN/embalagem/região existem na tabela mas não são lidos aqui
// (não usados pelo cálculo, ver comentário da migration 248).
type ppaValido struct {
	CodProd string
	PPA     string
}

func lerClientesNumericos(db *sql.DB, empresaID string, vigenciaID int) ([]clienteNumericaValido, error) {
	rows, err := db.Query(`
		SELECT cnpj, cod_cl, classificacao_pdv, razao, fantasia, cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca
		FROM farol.metas_clientes_numericas WHERE vigencia_id = $1 AND empresa_id = $2
	`, vigenciaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []clienteNumericaValido
	for rows.Next() {
		var c clienteNumericaValido
		if err := rows.Scan(&c.CNPJ, &c.CodCl, &c.ClassificacaoPDV, &c.Razao, &c.Fantasia, &c.CodGGV, &c.NomeGGV, &c.CodCRV, &c.NomeCRV, &c.CodRCA, &c.NomeRCA); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func lerPPAsValidos(db *sql.DB, empresaID string, vigenciaID int) ([]ppaValido, error) {
	rows, err := db.Query(`SELECT cod_prod, ppa_nome FROM farol.metas_ppas WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ppaValido
	for rows.Next() {
		var p ppaValido
		if err := rows.Scan(&p.CodProd, &p.PPA); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func cnpjsDeClientesNumericos(clientes []clienteNumericaValido) []string {
	out := make([]string, len(clientes))
	for i, c := range clientes {
		out[i] = c.CNPJ
	}
	return out
}

// ─── Cobertura Numérica (FR25) ──────────────────────────────────────────────

// calcularCoberturaNumerica calcula, por Cliente/CNPJ, se ele bateu o
// limiar de compra da sua Classificação PDV no período (bimestre móvel,
// resolvido pelo chamador — ver ajusteJanelaApuracao em farol_metas_calculo.go).
func calcularCoberturaNumerica(db *sql.DB, empresaID string, clientes []clienteNumericaValido, parametros map[string]any, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) ([]RealizadoRede, error) {
	limiarPorClasse := map[string]float64{}
	for classe, chave := range map[string]string{"Num. A": "limiar_num_a", "Num. B": "limiar_num_b", "Num. C": "limiar_num_c"} {
		v, ok := numeroDeParametro(parametros, chave)
		if !ok {
			return nil, fmt.Errorf("vínculo não tem o parâmetro %s preenchido — obrigatório pra Cobertura Numérica", chave)
		}
		limiarPorClasse[classe] = v
	}

	cnpjs := cnpjsDeClientesNumericos(clientes)
	valoresPorCliente, err := somaPvendaClientesNumerica(db, empresaID, cnpjs, dataInicio, dataFim, fluxo, tiposVenda, codFornec)
	if err != nil {
		return nil, err
	}

	out := make([]RealizadoRede, 0, len(clientes))
	for _, c := range clientes {
		limiar, ok := limiarPorClasse[c.ClassificacaoPDV]
		if !ok {
			return nil, fmt.Errorf("cliente %s tem Classificação PDV desconhecida: %q", c.CNPJ, c.ClassificacaoPDV)
		}
		valor := valoresPorCliente[c.CNPJ] // ausente = 0 (nenhuma venda no período)
		atingiu := valor >= limiar
		// Clientes (RealizadoRede.Clientes) NÃO é preenchido aqui de
		// propósito — achado de performance 2026-09-30: CodPrinc já É o
		// CNPJ (truque de pseudo-Rede), então um Clientes:[self] só
		// duplicaria os MESMOS campos (CNPJ/Razao/Fantasia/Valor/Atingiu)
		// dentro do próprio JSON do snapshot, quase dobrando o tamanho do
		// blob pros ~10.754 clientes reais — e sem nenhum consumidor real:
		// o front desliga o drill-down de Cliente pra Numérica (ehNumerica
		// ? undefined : ...) e nenhuma regra de Gamificação hoje aponta
		// pra vínculo Numérica (confirmado em PRD, farol_gamificacao.go
		// dependeria disso se apontasse). Se um dia precisar, resolver
		// separado — não reintroduzir a duplicação.
		out = append(out, RealizadoRede{
			CodPrinc: c.CNPJ, CodCl: c.CodCl, Razao: c.Razao, Fantasia: c.Fantasia, QtLojas: 1,
			CodGGV: c.CodGGV, NomeGGV: c.NomeGGV, CodCRV: c.CodCRV, NomeCRV: c.NomeCRV, CodRCA: c.CodRCA, NomeRCA: c.NomeRCA,
			Valor: valor, ValorTotal: valor, Objetivo: limiar, Atingiu: atingiu,
		})
	}
	return out, nil
}

// ─── Sortimento Numérica / PPA (FR26) ───────────────────────────────────────

// calcularSortimentoNumericaPPA calcula, por Cliente/CNPJ Classificação A
// ou B (Classificação C não entra nesta métrica — FR26), quantos PPAs
// distintos ele comprou no período (bimestre móvel).
func calcularSortimentoNumericaPPA(db *sql.DB, empresaID string, clientes []clienteNumericaValido, ppas []ppaValido, parametros map[string]any, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) ([]RealizadoRede, error) {
	teto, ok := numeroDeParametro(parametros, "teto_ppas")
	if !ok {
		return nil, fmt.Errorf("vínculo não tem o parâmetro teto_ppas preenchido — obrigatório pra Sortimento Numérica")
	}

	ppaDoCodProd := map[string]string{}
	for _, p := range ppas {
		ppaDoCodProd[p.CodProd] = p.PPA
	}

	var elegiveis []clienteNumericaValido
	for _, c := range clientes {
		if c.ClassificacaoPDV == "Num. A" || c.ClassificacaoPDV == "Num. B" {
			elegiveis = append(elegiveis, c)
		}
	}
	cnpjs := cnpjsDeClientesNumericos(elegiveis)

	linhasPorCliente, err := qtdPorCodProdClientesNumerica(db, empresaID, cnpjs, dataInicio, dataFim, fluxo, tiposVenda, codFornec)
	if err != nil {
		return nil, err
	}

	out := make([]RealizadoRede, 0, len(elegiveis))
	for _, c := range elegiveis {
		qtdPPAs := contarPPAsPositivados(linhasPorCliente[c.CNPJ], ppaDoCodProd, teto)
		atingiu := qtdPPAs >= teto
		// TeveCompra: qualquer produto desta Indústria no período, não só
		// produto da lista de PPAs — linhasPorCliente já vem de
		// qtdPorCodProdClientesNumerica sem filtro de PPA (só cod_prod <>
		// ''), então "tem alguma linha" já é exatamente essa pergunta.
		teveCompra := len(linhasPorCliente[c.CNPJ]) > 0
		// Clientes não preenchido — ver comentário equivalente em
		// calcularCoberturaNumerica (achado de performance 2026-09-30).
		out = append(out, RealizadoRede{
			CodPrinc: c.CNPJ, CodCl: c.CodCl, Razao: c.Razao, Fantasia: c.Fantasia, QtLojas: 1,
			CodGGV: c.CodGGV, NomeGGV: c.NomeGGV, CodCRV: c.CodCRV, NomeCRV: c.NomeCRV, CodRCA: c.CodRCA, NomeRCA: c.NomeRCA,
			Valor: qtdPPAs, Objetivo: teto, Atingiu: atingiu, TeveCompra: teveCompra,
		})
	}
	return out, nil
}

// contarPPAsPositivados espelha contarEANsPositivados (Sortimento por
// Rede), trocando "grupo de EAN por componente conexo" por PPA (dado
// EXPLÍCITO da planilha mensal, não inferido) e com TETO: "loja comprou 7
// PPAs mês anterior + 8 diferentes esse mês = considera que comprou 15" —
// mesmo com catálogo maior, a contagem não passa do teto (15 ou 7,
// conforme indústria — FR26). Quantidade mínima FIXA em 3 unidades (não é
// parâmetro configurável do vínculo, diferente do qtd_minima_positivacao
// do Sortimento por Rede — o documento-fonte da Numérica declara "3" como
// regra fixa do programa), reaproveitando a mesma leitura de
// embalagem/qt_unit_cx da venda bruta que exigeQuantidadeMinima já usa.
func contarPPAsPositivados(linhas map[string]vendaProdutoAgregada, ppaDoCodProd map[string]string, teto float64) float64 {
	const qtdMinimaFixa = 3
	ppasPositivados := map[string]bool{}
	for codProd, agregada := range linhas {
		ppa, ok := ppaDoCodProd[codProd]
		if !ok {
			continue // produto vendido não está na lista de PPAs deste programa
		}
		if agregada.Qtd <= 0 {
			continue // líquido (Faturado - devolvido/cancelado) zerado ou negativo: não positivou
		}
		if exigeQuantidadeMinima(agregada.Embalagem, agregada.QtUnitCx) && agregada.Qtd < qtdMinimaFixa {
			continue
		}
		ppasPositivados[ppa] = true
	}
	n := float64(len(ppasPositivados))
	if n > teto {
		n = teto
	}
	return n
}

// ─── Consultas de venda CNPJ-direto (sem par cnpj+cod_cliprinc) ────────────
//
// A Numérica não tem Rede (FR24) — não existe "dono" de CNPJ a desambiguar
// como no motor Rede (ver filtrarPorClienteEDono). Estas funções somam toda
// venda do CNPJ no período, qualquer registro de origem que ele tenha.

func somaPvendaClientesNumerica(db *sql.DB, empresaID string, cnpjs []string, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) (map[string]float64, error) {
	out := map[string]float64{}
	if len(cnpjs) == 0 {
		return out, nil
	}
	somar := func(tabela, colData string) error {
		t0 := time.Now()
		query := fmt.Sprintf(`
			SELECT v.cnpj, SUM(v.pvenda) FROM %s v
			WHERE v.empresa_id = $1 AND v.cnpj = ANY($2) AND v.%s BETWEEN $3 AND $4
		`, tabela, colData)
		args := []any{empresaID, pq.Array(cnpjs), dataInicio, dataFim}
		if len(tiposVenda) > 0 {
			query += fmt.Sprintf(" AND v.tipo_venda = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(tiposVenda))
		}
		if len(codFornec) > 0 {
			query += fmt.Sprintf(" AND v.cod_fornec = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(codFornec))
		}
		query += " GROUP BY v.cnpj"
		rows, err := db.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var cnpj string
			var valor float64
			if err := rows.Scan(&cnpj, &valor); err != nil {
				return err
			}
			out[cnpj] += valor
			n++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		log.Printf("[farol:objetivos:numerica] somaPvendaClientesNumerica tabela=%s cnpjs=%d período=[%s..%s] → %d linhas em %v",
			tabela, len(cnpjs), dataInicio, dataFim, n, time.Since(t0))
		return nil
	}
	switch fluxo {
	case "faturado":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
		if err := subtrairDevolucaoCancelamentoNumerica(db, empresaID, cnpjs, dataInicio, dataFim, codFornec, out); err != nil {
			return nil, err
		}
	case "transmitido":
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return nil, err
		}
	case "soma":
		// Story 7.8 — visão "Faturado + Emitido" pedida pela documentação
		// final do Heverton (29/09/2026), sem condicional. Reaproveita a
		// MESMA semântica do "soma" que já existiu no motor Rede até ser
		// cortado em 04/09/2026 (Heverton pediu só 2 visões pra Rede,
		// "mesma filosofia do Farol V1") — soma direta de Faturado líquido
		// (já abatido de devolução/cancelamento) + Transmitido, sem
		// deduplicar pedido que progrediu de Transmitido pra Faturado no
		// mesmo período (mesmo comportamento histórico, não é bug novo).
		// Só entra aqui pra Numérica: as funções Rede equivalentes
		// (somaPvendaClientes) continuam sem este case de propósito — a
		// decisão de reverter o corte pro motor Rede é separada (ver PRD,
		// Questões em aberto #5).
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
		if err := subtrairDevolucaoCancelamentoNumerica(db, empresaID, cnpjs, dataInicio, dataFim, codFornec, out); err != nil {
			return nil, err
		}
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("fluxo inválido: %q (use faturado, transmitido ou soma)", fluxo)
	}
	return out, nil
}

func subtrairDevolucaoCancelamentoNumerica(db *sql.DB, empresaID string, cnpjs []string, dataInicio, dataFim string, codFornec []string, out map[string]float64) error {
	t0 := time.Now()
	query := `
		SELECT v.cnpj, SUM(v.pvenda) FROM vendas_ccd v
		WHERE v.empresa_id = $1 AND v.cnpj = ANY($2) AND v.data_evento BETWEEN $3 AND $4
		  AND v.evento IN ('DEVOLVIDO', 'CANCELADO')
	`
	args := []any{empresaID, pq.Array(cnpjs), dataInicio, dataFim}
	if len(codFornec) > 0 {
		query += fmt.Sprintf(" AND v.cod_fornec = ANY($%d)", len(args)+1)
		args = append(args, pq.Array(codFornec))
	}
	query += " GROUP BY v.cnpj"
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var cnpj string
		var valor float64
		if err := rows.Scan(&cnpj, &valor); err != nil {
			return err
		}
		out[cnpj] -= valor
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("[farol:objetivos:numerica] subtrairDevolucaoCancelamentoNumerica cnpjs=%d período=[%s..%s] → %d linhas em %v",
		len(cnpjs), dataInicio, dataFim, n, time.Since(t0))
	return nil
}

func qtdPorCodProdClientesNumerica(db *sql.DB, empresaID string, cnpjs []string, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) (map[string]map[string]vendaProdutoAgregada, error) {
	out := map[string]map[string]vendaProdutoAgregada{}
	if len(cnpjs) == 0 {
		return out, nil
	}
	somar := func(tabela, colData string) error {
		t0 := time.Now()
		query := fmt.Sprintf(`
			SELECT v.cnpj, v.cod_prod, SUM(v.qt), MAX(v.embalagem), MAX(v.qt_unit_cx) FROM %s v
			WHERE v.empresa_id = $1 AND v.cnpj = ANY($2) AND v.%s BETWEEN $3 AND $4 AND v.cod_prod <> ''
		`, tabela, colData)
		args := []any{empresaID, pq.Array(cnpjs), dataInicio, dataFim}
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
			var cnpj, codProd, embalagem string
			var qt, qtUnitCx float64
			if err := rows.Scan(&cnpj, &codProd, &qt, &embalagem, &qtUnitCx); err != nil {
				return err
			}
			porCliente, ok := out[cnpj]
			if !ok {
				porCliente = map[string]vendaProdutoAgregada{}
				out[cnpj] = porCliente
			}
			agregada := porCliente[codProd]
			agregada.Qtd += qt
			agregada.Embalagem = embalagem
			agregada.QtUnitCx = qtUnitCx
			porCliente[codProd] = agregada
			n++
		}
		if err := rows.Err(); err != nil {
			return err
		}
		log.Printf("[farol:objetivos:numerica] qtdPorCodProdClientesNumerica tabela=%s cnpjs=%d período=[%s..%s] → %d linhas em %v",
			tabela, len(cnpjs), dataInicio, dataFim, n, time.Since(t0))
		return nil
	}
	switch fluxo {
	case "faturado":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
		if err := subtrairDevolucaoCancelamentoQtdNumerica(db, empresaID, cnpjs, dataInicio, dataFim, codFornec, out); err != nil {
			return nil, err
		}
	case "transmitido":
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return nil, err
		}
	case "soma":
		// Story 7.8 — ver comentário equivalente em somaPvendaClientesNumerica.
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
		if err := subtrairDevolucaoCancelamentoQtdNumerica(db, empresaID, cnpjs, dataInicio, dataFim, codFornec, out); err != nil {
			return nil, err
		}
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("fluxo inválido: %q (use faturado, transmitido ou soma)", fluxo)
	}
	return out, nil
}

func subtrairDevolucaoCancelamentoQtdNumerica(db *sql.DB, empresaID string, cnpjs []string, dataInicio, dataFim string, codFornec []string, out map[string]map[string]vendaProdutoAgregada) error {
	t0 := time.Now()
	query := `
		SELECT v.cnpj, v.cod_prod, SUM(v.qt), MAX(v.embalagem), MAX(v.qt_unit_cx) FROM vendas_ccd v
		WHERE v.empresa_id = $1 AND v.cnpj = ANY($2) AND v.data_evento BETWEEN $3 AND $4
		  AND v.cod_prod <> '' AND v.evento IN ('DEVOLVIDO', 'CANCELADO')
	`
	args := []any{empresaID, pq.Array(cnpjs), dataInicio, dataFim}
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
		var cnpj, codProd, embalagem string
		var qt, qtUnitCx float64
		if err := rows.Scan(&cnpj, &codProd, &qt, &embalagem, &qtUnitCx); err != nil {
			return err
		}
		porCliente, ok := out[cnpj]
		if !ok {
			porCliente = map[string]vendaProdutoAgregada{}
			out[cnpj] = porCliente
		}
		agregada := porCliente[codProd]
		agregada.Qtd -= qt
		if agregada.Embalagem == "" {
			agregada.Embalagem = embalagem
		}
		if agregada.QtUnitCx == 0 {
			agregada.QtUnitCx = qtUnitCx
		}
		porCliente[codProd] = agregada
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("[farol:objetivos:numerica] subtrairDevolucaoCancelamentoQtdNumerica cnpjs=%d período=[%s..%s] → %d linhas em %v",
		len(cnpjs), dataInicio, dataFim, n, time.Since(t0))
	return nil
}
