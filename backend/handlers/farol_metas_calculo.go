package handlers

// farol_metas_calculo.go — Motor de Apuração: cálculo do Realizado
// (Épico 4, Story 4.1, módulo Painel de Gestão de Metas por Indústria)
//
// Lê Faturado/Transmitido do Farol (vendas_faturadas/vendas_transmitidas,
// dado bruto — não os agg_* pré-calculados, que não conhecem o conceito de
// Rede deste módulo) e calcula o Realizado por Rede, depois agrega por
// RCA/CRV/GGV. Nunca soma valor pré-agregado de um nível pra derivar outro
// (mesmo cuidado do bug de totalizador já documentado no Farol e do
// princípio FR18a) — cada nível é somado a partir do dado de Rede, que é o
// grão atômico aqui.
//
// Dispatch por formula_codigo (migration 222): cada Tipo de Métrica
// aponta pra uma função Go que sabe calcular aquele shape específico. O
// parametros_schema (Story 1.1) é genérico no DADO; a matemática em si
// exige código — isso é inerente, não uma falha do framework genérico.
//
// ─── Ajuste 2026-09-04 (orientação direta do Heverton) ────────────────────
// Três mudanças estruturais em cima do motor original:
//
//  1. Hierarquia GGV/CRV/RCA vem do CSV de Clientes Válidos importado
//     (migration 224), não mais de JOIN na linha de venda mais recente do
//     RCA. Motivo: o parágrafo novo do descritivo Unilever — "a apuração
//     precisa ser referente ao total vendido ao cliente, independente de
//     quem vendeu (GGV, CRV, RCA)" — deixou claro que o "dono" do cliente
//     pra fins de rollup é o da planilha mensal do fornecedor, não quem
//     efetivamente vendeu (podem divergir, e a JC confirmou que 6 de 134
//     redes reais têm lojas com RCAs diferentes — "base inconsistente,
//     ajustando na origem", sem pedir tratamento especial nosso).
//  2. Cobertura E Sortimento passam a filtrar por cod_fornec — bug real
//     corrigido: antes somava TODA venda do CNPJ no período, não só a do
//     fornecedor da Indústria do vínculo (ex: 396 Unilever HC).
//  3. A regra de quantidade mínima do Sortimento (3+ unidades pra item
//     "UN") não vem mais do CSV de Itens Válidos (campo tipo_embalagem,
//     removido na migration 225) — vem do cadastro de produto já
//     importado na carga diária de vendas (embalagem/qt_unit_cx). Ver
//     exigeQuantidadeMinima abaixo pro racional completo dessa assunção.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

// ─── Tipos ────────────────────────────────────────────────────────────────────

// RealizadoCliente é o grão mais fino do painel (nível 5, "Rede/CNPJ") — o
// valor daquela loja isolada, comparado ao MESMO objetivo da Rede (o
// descritivo não define um objetivo por loja, só por Rede — ver modelo V1
// da JC, aba "Resumo Rede Cliente", que replica as mesmas colunas de
// indicador da Rede em cada linha de CNPJ).
type RealizadoCliente struct {
	CNPJ     string  `json:"cnpj"`
	Razao    string  `json:"razao"`
	Fantasia string  `json:"fantasia"`
	Valor    float64 `json:"valor"`
}

type RealizadoRede struct {
	CodPrinc   string  `json:"cod_princ"` // Rede = COD_PRINC (decisão 2026-09-04)
	Razao      string  `json:"razao"`
	Fantasia   string  `json:"fantasia"`
	QtLojas    int     `json:"qt_lojas"`
	CodGGV     string  `json:"cod_ggv"`
	NomeGGV    string  `json:"nome_ggv"`
	CodCRV     string  `json:"cod_crv"`
	NomeCRV    string  `json:"nome_crv"`
	CodRCA     string  `json:"cod_rca"` // RCA "representante" da Rede — ver redeRepresentante
	NomeRCA    string  `json:"nome_rca"`
	Valor      float64 `json:"valor"`       // média entre lojas (Cobertura: R$; Sortimento: qtd EANs)
	ValorTotal float64 `json:"valor_total"` // só Cobertura: soma (não-média) entre lojas — coluna "VALOR VENDA" do modelo V1
	Atingiu    bool    `json:"atingiu"`     // Cobertura: valor médio >= limiar do vínculo. Sortimento: valor médio >= maior faixa cadastrada (ver CalcularRealizadoComPeriodo)

	Clientes []RealizadoCliente `json:"clientes,omitempty"` // nível 5 — só populado quando o chamador pede (ver incluirClientes)
}

type RealizadoGrupo struct {
	Codigo          string  `json:"codigo"`
	Nome            string  `json:"nome"`
	RealizadoTotal  float64 `json:"realizado_total"` // mantido por compatibilidade — mesmo valor de QtdAtingindo (contagem de redes), NUNCA mais uma média nos níveis de grupo (ver ponto 2026-09-04 acima)
	QtdRedes        int     `json:"qtd_redes"`
	QtdAtingindo    int     `json:"qtd_atingindo"`
	QtdFaltaAtingir int     `json:"qtd_falta_atingir"`
	Projecao        float64 `json:"projecao"` // fechamento projetado deste nível — calculado a partir do RealizadoTotal DESTE grupo, nunca somando projeções de Redes/níveis filhos (FR18a)
}

type RealizadoResultado struct {
	VinculoID      int              `json:"vinculo_id"`
	VigenciaID     int              `json:"vigencia_id"`
	Fluxo          string           `json:"fluxo"`
	Nivel          string           `json:"nivel"`
	RealizadoTotal float64          `json:"realizado_total"`
	Projecao       float64          `json:"projecao"` // FR18: ritmo linear (realizado ÷ dias decorridos × dias totais do período)
	Redes          []RealizadoRede  `json:"redes"`
	Grupos         []RealizadoGrupo `json:"grupos,omitempty"`
	Parcial        bool             `json:"parcial"` // true quando o período pedido inclui o mês corrente (ainda em andamento)
}

// clienteValido é uma linha crua de farol.metas_clientes_validos — já com o
// trio GGV/CRV/RCA importado (migration 224), não mais derivado por JOIN em
// vendas.
type clienteValido struct {
	CodPrinc string
	CNPJ     string
	Razao    string
	Fantasia string
	CodGGV   string
	NomeGGV  string
	CodCRV   string
	NomeCRV  string
	CodRCA   string
	NomeRCA  string
}

// itemValido é uma linha crua de farol.metas_itens_validos — só EAN e
// cod_prod desde a migration 225 (tipo_embalagem removido, ver cabeçalho).
type itemValido struct {
	EAN     string
	CodProd string
}

// ─── Ponto de entrada — dispatch por formula_codigo ───────────────────────────

// CalcularRealizado calcula o Realizado de um vínculo/vigência, no nível
// hierárquico pedido, lendo o fluxo indicado (faturado/transmitido), sobre
// o período INTEIRO da vigência — uso normal (Épicos 4/5.1/5.2).
func CalcularRealizado(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel string) (*RealizadoResultado, error) {
	return CalcularRealizadoComPeriodo(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, "", "")
}

// CalcularRealizadoComPeriodo permite sobrescrever a janela de datas usada
// pra ler vendas (Story 5.3, FR21 — "recortes de tempo": dia anterior,
// semana, mês, ano corrente) sem mudar a vigência em si. A PROJEÇÃO de
// fechamento continua sempre calculada sobre o período INTEIRO da vigência
// — projetar o fechamento da vigência com base só em "ontem" não faria
// sentido; recorte afeta o Realizado exibido, não a base da projeção.
func CalcularRealizadoComPeriodo(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel, dataInicioOverride, dataFimOverride string) (*RealizadoResultado, error) {
	var formulaCodigo, dataInicioVigencia, dataFimVigencia string
	var industriaID int
	var tiposVendaValidos []string
	err := db.QueryRow(`
		SELECT tm.formula_codigo, v.data_inicio::text, v.data_fim::text, mv.industria_id
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3
	`, vigenciaID, vinculoID, empresaID).Scan(&formulaCodigo, &dataInicioVigencia, &dataFimVigencia, &industriaID)
	if err != nil {
		return nil, fmt.Errorf("vínculo/vigência não encontrado: %w", err)
	}
	dataInicio, dataFim := dataInicioVigencia, dataFimVigencia
	if dataInicioOverride != "" && dataFimOverride != "" {
		dataInicio, dataFim = dataInicioOverride, dataFimOverride
	}
	if err := db.QueryRow(`SELECT tipos_venda_validos FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2`, vinculoID, empresaID).
		Scan(pq.Array(&tiposVendaValidos)); err != nil {
		return nil, fmt.Errorf("erro ao ler tipos_venda_validos: %w", err)
	}

	parametros, err := lerParametrosValoresVinculo(db, empresaID, vinculoID)
	if err != nil {
		return nil, err
	}

	// Cobertura E Sortimento contam só venda do(s) fornecedor(es) da
	// Indústria do vínculo (ex: 396 pra Unilever HC) — bug corrigido
	// 2026-09-04, ver cabeçalho do arquivo. Vínculo sem NENHUM cod_fornec
	// mapeado (cadastro de Indústria incompleto) cai sem filtro — loga pra
	// não mascarar o problema de cadastro, mas não derruba o cálculo.
	codFornec, err := codFornecDaIndustria(db, empresaID, industriaID)
	if err != nil {
		return nil, err
	}

	clientes, err := lerClientesValidos(db, empresaID, vigenciaID)
	if err != nil {
		return nil, err
	}
	if len(clientes) == 0 {
		return nil, fmt.Errorf("nenhum Cliente Válido importado pra esta vigência — importe a lista antes de calcular (Épico 3)")
	}

	var redes []RealizadoRede
	switch formulaCodigo {
	case "cobertura_rede":
		redes, err = calcularCoberturaPorRede(db, empresaID, clientes, parametros, dataInicio, dataFim, fluxo, tiposVendaValidos, codFornec)
	case "sortimento_rede":
		itens, ierr := lerItensValidos(db, empresaID, vigenciaID)
		if ierr != nil {
			return nil, ierr
		}
		if len(itens) == 0 {
			return nil, fmt.Errorf("nenhum Item Válido importado pra esta vigência — importe a lista antes de calcular (Épico 3)")
		}
		redes, err = calcularSortimentoPorRede(db, empresaID, clientes, itens, parametros, dataInicio, dataFim, fluxo, tiposVendaValidos, codFornec)
	case "":
		return nil, fmt.Errorf("este Tipo de Métrica não tem calculadora implementada (formula_codigo vazio)")
	default:
		return nil, fmt.Errorf("formula_codigo desconhecido: %q — nenhuma calculadora registrada", formulaCodigo)
	}
	if err != nil {
		return nil, err
	}

	// Sortimento não tem "limiar" nos parametros_valores do vínculo (esse
	// conceito só existe pra Cobertura) — o "atingiu" por Rede é contra a
	// maior Faixa cadastrada na vigência (mesma leitura que
	// farol_metas_painel_combinado.go faz pra "objetivo por Rede").
	if formulaCodigo == "sortimento_rede" {
		objetivo, ferr := maiorFaixaCadastrada(db, empresaID, vigenciaID)
		if ferr != nil {
			return nil, ferr
		}
		for i := range redes {
			redes[i].Atingiu = redes[i].Valor >= objetivo
		}
	}

	resultado := &RealizadoResultado{
		VinculoID: vinculoID, VigenciaID: vigenciaID, Fluxo: fluxo, Nivel: nivel, Redes: redes,
		Parcial: periodoIncluiHoje(dataFim),
	}
	switch formulaCodigo {
	case "cobertura_rede":
		count := 0
		for _, r := range redes {
			if r.Atingiu {
				count++
			}
		}
		resultado.RealizadoTotal = float64(count)
	case "sortimento_rede":
		var soma float64
		for _, r := range redes {
			soma += r.Valor
		}
		if len(redes) > 0 {
			resultado.RealizadoTotal = soma / float64(len(redes))
		}
	}

	resultado.Projecao = projetarFechamento(resultado.RealizadoTotal, dataInicioVigencia, dataFimVigencia)

	if nivel != "rede" && nivel != "" {
		resultado.Grupos, err = agregarPorNivel(redes, nivel)
		if err != nil {
			return nil, err
		}
		// FR18a: projeção de cada grupo a partir do REALIZADO PRÓPRIO daquele
		// grupo — nunca somando as projeções das Redes que o compõem.
		for i := range resultado.Grupos {
			resultado.Grupos[i].Projecao = projetarFechamento(resultado.Grupos[i].RealizadoTotal, dataInicioVigencia, dataFimVigencia)
		}
	}

	return resultado, nil
}

// ─── Leitura de listas válidas ────────────────────────────────────────────────

func lerParametrosValoresVinculo(db *sql.DB, empresaID string, vinculoID int) (map[string]any, error) {
	var raw []byte
	if err := db.QueryRow(`SELECT parametros_valores FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2`, vinculoID, empresaID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("vínculo não encontrado: %w", err)
	}
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m, nil
}

// codFornecDaIndustria resolve os cod_fornec mapeados pra uma Indústria
// (farol.industria_fornecedores, cadastro já existente — ver
// handlers/farol_industrias.go). Lista vazia = nenhum mapeamento cadastrado
// (cadastro de Indústria incompleto); o chamador decide não filtrar nesse
// caso em vez de devolver silenciosamente zero linhas.
func codFornecDaIndustria(db *sql.DB, empresaID string, industriaID int) ([]string, error) {
	rows, err := db.Query(`SELECT cod_fornec FROM farol.industria_fornecedores WHERE industria_id = $1 AND empresa_id = $2`, industriaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// maiorFaixaCadastrada devolve o maior valor_meta cadastrado numa vigência
// — o "objetivo por Rede" do Sortimento (o descritivo Unilever declara esse
// número como o que "cada rede precisa" bater, além de ser o teto da
// apuração agregada do distribuidor).
func maiorFaixaCadastrada(db *sql.DB, empresaID string, vigenciaID int) (float64, error) {
	var maior sql.NullFloat64
	if err := db.QueryRow(`SELECT MAX(valor_meta) FROM farol.metas_faixas WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID).Scan(&maior); err != nil {
		return 0, err
	}
	return maior.Float64, nil
}

func lerClientesValidos(db *sql.DB, empresaID string, vigenciaID int) ([]clienteValido, error) {
	rows, err := db.Query(`
		SELECT cod_princ, cnpj, razao, fantasia, cod_ggv, nome_ggv, cod_crv, nome_crv, cod_rca, nome_rca
		FROM farol.metas_clientes_validos WHERE vigencia_id = $1 AND empresa_id = $2
	`, vigenciaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []clienteValido
	for rows.Next() {
		var c clienteValido
		if err := rows.Scan(&c.CodPrinc, &c.CNPJ, &c.Razao, &c.Fantasia, &c.CodGGV, &c.NomeGGV, &c.CodCRV, &c.NomeCRV, &c.CodRCA, &c.NomeRCA); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func lerItensValidos(db *sql.DB, empresaID string, vigenciaID int) ([]itemValido, error) {
	rows, err := db.Query(`SELECT ean, cod_prod FROM farol.metas_itens_validos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []itemValido
	for rows.Next() {
		var i itemValido
		if err := rows.Scan(&i.EAN, &i.CodProd); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

// agruparPorRede indexa os Clientes Válidos por Rede (COD_PRINC),
// preservando ordem de primeira aparição (determinístico).
func agruparPorRede(clientes []clienteValido) (ordem []string, porRede map[string][]clienteValido) {
	porRede = map[string][]clienteValido{}
	for _, c := range clientes {
		if _, ok := porRede[c.CodPrinc]; !ok {
			ordem = append(ordem, c.CodPrinc)
		}
		porRede[c.CodPrinc] = append(porRede[c.CodPrinc], c)
	}
	return
}

// redeRepresentante resolve o dono (GGV/CRV/RCA) "oficial" de uma Rede como
// o dono do primeiro CNPJ em ordem alfabética — regra determinística e
// simples, mesma assunção já documentada antes da migration 224 (só a
// FONTE do dado mudou: antes vinha de JOIN em vendas, agora vem do CSV de
// Clientes Válidos importado). Uma Rede pode ter CNPJs com donos diferentes
// (6 de 134 redes reais têm isso, confirmado contra a planilha oficial da
// JC em 2026-09-03) — a JC respondeu "base inconsistente, ajustando na
// origem" e não pediu tratamento especial, então a apuração por Rede
// (FR18a) não fragmenta o cálculo: só o "dono pra exibir/agrupar" usa essa
// regra de desempate.
func redeRepresentante(clientesDaRede []clienteValido) clienteValido {
	menor := clientesDaRede[0]
	for _, c := range clientesDaRede[1:] {
		if c.CNPJ < menor.CNPJ {
			menor = c
		}
	}
	return menor
}

// ─── Cobertura por Rede ────────────────────────────────────────────────────────

func calcularCoberturaPorRede(db *sql.DB, empresaID string, clientes []clienteValido, parametros map[string]any, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) ([]RealizadoRede, error) {
	limiar, ok := numeroDeParametro(parametros, "limiar_valor_medio")
	if !ok {
		return nil, fmt.Errorf("vínculo não tem o parâmetro limiar_valor_medio preenchido — obrigatório pra Cobertura por Rede")
	}

	ordem, porRede := agruparPorRede(clientes)
	var out []RealizadoRede
	for _, codPrinc := range ordem {
		clientesDaRede := porRede[codPrinc]
		var somaCompras float64
		clientesResultado := make([]RealizadoCliente, 0, len(clientesDaRede))
		for _, c := range clientesDaRede {
			valor, err := somaPvendaCliente(db, empresaID, c.CNPJ, dataInicio, dataFim, fluxo, tiposVenda, codFornec)
			if err != nil {
				return nil, err
			}
			somaCompras += valor
			clientesResultado = append(clientesResultado, RealizadoCliente{CNPJ: c.CNPJ, Razao: c.Razao, Fantasia: c.Fantasia, Valor: valor})
		}
		media := somaCompras / float64(len(clientesDaRede))
		dono := redeRepresentante(clientesDaRede)
		out = append(out, RealizadoRede{
			CodPrinc: codPrinc, Razao: dono.Razao, Fantasia: dono.Fantasia, QtLojas: len(clientesDaRede),
			CodGGV: dono.CodGGV, NomeGGV: dono.NomeGGV, CodCRV: dono.CodCRV, NomeCRV: dono.NomeCRV,
			CodRCA: dono.CodRCA, NomeRCA: dono.NomeRCA,
			Valor: media, ValorTotal: somaCompras, Atingiu: media >= limiar,
			Clientes: clientesResultado,
		})
	}
	return out, nil
}

// somaPvendaCliente soma pvenda (Faturado/Transmitido) de um CNPJ no
// período, respeitando o(s) tipo(s) de venda válido(s) do vínculo (FR16) —
// vazio = sem filtro de tipo_venda ("Líquido" padrão do Farol, que já é o
// conteúdo natural de vendas_faturadas/vendas_transmitidas sem filtro
// adicional) — e o(s) cod_fornec da Indústria do vínculo (ex: 396 Unilever
// HC): sem esse filtro a "média de compras" da Rede incluiria compras de
// QUALQUER fornecedor, não só o do programa (bug corrigido 2026-09-04).
func somaPvendaCliente(db *sql.DB, empresaID, cnpj, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) (float64, error) {
	var total float64
	somar := func(tabela, colData string) error {
		query := fmt.Sprintf(`
			SELECT COALESCE(SUM(pvenda), 0) FROM %s
			WHERE empresa_id = $1 AND cnpj = $2 AND %s BETWEEN $3 AND $4
		`, tabela, colData)
		args := []any{empresaID, cnpj, dataInicio, dataFim}
		if len(tiposVenda) > 0 {
			query += fmt.Sprintf(" AND tipo_venda = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(tiposVenda))
		}
		if len(codFornec) > 0 {
			query += fmt.Sprintf(" AND cod_fornec = ANY($%d)", len(args)+1)
			args = append(args, pq.Array(codFornec))
		}
		var v float64
		if err := db.QueryRow(query, args...).Scan(&v); err != nil {
			return err
		}
		total += v
		return nil
	}
	switch fluxo {
	case "faturado":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return 0, err
		}
	case "transmitido":
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("fluxo inválido: %q (use faturado ou transmitido)", fluxo)
	}
	return total, nil
}

// ─── Sortimento por Rede ────────────────────────────────────────────────────

func calcularSortimentoPorRede(db *sql.DB, empresaID string, clientes []clienteValido, itens []itemValido, parametros map[string]any, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) ([]RealizadoRede, error) {
	qtdMinima, ok := numeroDeParametro(parametros, "qtd_minima_positivacao")
	if !ok {
		return nil, fmt.Errorf("vínculo não tem o parâmetro qtd_minima_positivacao preenchido — obrigatório pra Sortimento por Rede")
	}

	// cod_prod -> ean (um EAN pode ter N cod_prod — BASE EANS da JC mostra
	// itens com mais de 2 códigos JC pro mesmo produto, ver dúvida C
	// resolvida 2026-09-04: "pode ter mais")
	eanPorCodProd := map[string]string{}
	for _, it := range itens {
		eanPorCodProd[it.CodProd] = it.EAN
	}

	ordem, porRede := agruparPorRede(clientes)
	var out []RealizadoRede
	for _, codPrinc := range ordem {
		clientesDaRede := porRede[codPrinc]
		var somaEANsPorLoja float64
		clientesResultado := make([]RealizadoCliente, 0, len(clientesDaRede))
		for _, c := range clientesDaRede {
			qtdEANs, err := contarEANsPositivadosCliente(db, empresaID, c.CNPJ, dataInicio, dataFim, fluxo, tiposVenda, codFornec, eanPorCodProd, qtdMinima)
			if err != nil {
				return nil, err
			}
			somaEANsPorLoja += qtdEANs
			clientesResultado = append(clientesResultado, RealizadoCliente{CNPJ: c.CNPJ, Razao: c.Razao, Fantasia: c.Fantasia, Valor: qtdEANs})
		}
		media := somaEANsPorLoja / float64(len(clientesDaRede))
		dono := redeRepresentante(clientesDaRede)
		out = append(out, RealizadoRede{
			CodPrinc: codPrinc, Razao: dono.Razao, Fantasia: dono.Fantasia, QtLojas: len(clientesDaRede),
			CodGGV: dono.CodGGV, NomeGGV: dono.NomeGGV, CodCRV: dono.CodCRV, NomeCRV: dono.NomeCRV,
			CodRCA: dono.CodRCA, NomeRCA: dono.NomeRCA,
			Valor: media, Clientes: clientesResultado,
			// Atingiu é preenchido depois, em CalcularRealizadoComPeriodo —
			// depende da maior Faixa cadastrada na vigência, que esta função
			// não conhece (é dado de Épico 2, não de Cliente/Item Válido).
		})
	}
	return out, nil
}

// vendaProdutoAgregada é o resultado de qtdPorCodProdCliente — quantidade
// vendida de um cod_prod no período, mais os atributos de embalagem do
// PRODUTO (não da venda) usados pra decidir a regra de quantidade mínima.
type vendaProdutoAgregada struct {
	Qtd       float64
	Embalagem string
	QtUnitCx  float64
}

// contarEANsPositivadosCliente conta quantos EANs distintos (da lista de
// Itens Válidos) um CNPJ comprou no período, respeitando a regra de
// quantidade mínima (FR12) — ver exigeQuantidadeMinima.
func contarEANsPositivadosCliente(db *sql.DB, empresaID, cnpj, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string, eanPorCodProd map[string]string, qtdMinima float64) (float64, error) {
	linhas, err := qtdPorCodProdCliente(db, empresaID, cnpj, dataInicio, dataFim, fluxo, tiposVenda, codFornec)
	if err != nil {
		return 0, err
	}
	eansPositivados := map[string]bool{}
	for codProd, agregada := range linhas {
		ean, ok := eanPorCodProd[codProd]
		if !ok {
			continue // produto vendido não está na lista de Itens Válidos deste programa
		}
		if exigeQuantidadeMinima(agregada.Embalagem, agregada.QtUnitCx) && agregada.Qtd < qtdMinima {
			continue
		}
		eansPositivados[ean] = true
	}
	return float64(len(eansPositivados)), nil
}

// exigeQuantidadeMinima decide se um item exige a venda de 3+ unidades pra
// contar como positivado no Sortimento (FR12: "isso no caso dos produtos
// que vendemos UN, visto que nos casos dos produtos que vendemos
// CAIXA/PACOTE/DISPLAY naturalmente a venda de uma dessas unidades já é
// maior que 3UN").
//
// ASSUNÇÃO NOVA, NÃO VALIDADA CONTRA DADO REAL (2026-09-04): até
// 2026-09-04 essa informação vinha de um campo tipo_embalagem no CSV
// mensal de Itens Válidos (removido, migration 225) — orientação do
// Heverton foi usar o cadastro de produto já existente na carga diária de
// vendas (colunas embalagem/qt_unit_cx, migration 168). NENHUMA venda real
// dos fornecedores 396 (Unilever HC) ou 131 (Unilever Foods) foi importada
// ainda no ambiente de dev no momento desta mudança (só 10 linhas de
// demo/PRODDEMO*, com embalagem/qt_unit_cx vazios/zero) — não deu pra
// inspecionar uma amostra real antes de codar a regra.
//
// A regra abaixo foi derivada do ÚNICO formato documentado no código pro
// mesmo nome de campo ("EMBALAGEM", vindo do layout WinThor/ERP —
// migration 100_sp_schema.sql, tabela smartpick.sp_enderecos, mesma origem
// de dado, comentário: `embalagem TEXT, -- EMBALAGEM (ex: "UN/0001/UN")`):
// o primeiro token antes da barra é a unidade de venda. Se `embalagem`
// vier vazio (dado ainda não carregado pra aquele produto), usa
// `qt_unit_cx <= 1` como sinal de reserva — uma "caixa" de 1 unidade só é,
// na prática, uma venda unitária.
//
// PRECISA SER VALIDADA contra uma venda real de HC/Foods assim que a
// primeira carga com esses fornecedores acontecer no ambiente — não tratar
// esta função como definitiva sem essa checagem.
func exigeQuantidadeMinima(embalagem string, qtUnitCx float64) bool {
	embalagem = strings.TrimSpace(embalagem)
	if embalagem != "" {
		token := embalagem
		if idx := strings.Index(embalagem, "/"); idx >= 0 {
			token = embalagem[:idx]
		}
		return strings.EqualFold(strings.TrimSpace(token), "UN")
	}
	return qtUnitCx <= 1
}

// qtdPorCodProdCliente soma a quantidade vendida (qt) por cod_prod, pra um
// CNPJ no período — base pra aplicar a regra de quantidade mínima. Também
// traz embalagem/qt_unit_cx (atributo do PRODUTO, constante por cod_prod —
// por isso MAX() em vez de GROUP BY em mais colunas) pra
// exigeQuantidadeMinima decidir a regra. Filtra por cod_fornec da Indústria
// do vínculo (mesmo bug corrigido de Cobertura, ver somaPvendaCliente).
func qtdPorCodProdCliente(db *sql.DB, empresaID, cnpj, dataInicio, dataFim, fluxo string, tiposVenda, codFornec []string) (map[string]vendaProdutoAgregada, error) {
	out := map[string]vendaProdutoAgregada{}
	somar := func(tabela, colData string) error {
		query := fmt.Sprintf(`
			SELECT cod_prod, SUM(qt), MAX(embalagem), MAX(qt_unit_cx) FROM %s
			WHERE empresa_id = $1 AND cnpj = $2 AND %s BETWEEN $3 AND $4 AND cod_prod <> ''
		`, tabela, colData)
		args := []any{empresaID, cnpj, dataInicio, dataFim}
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
			var codProd, embalagem string
			var qt, qtUnitCx float64
			if err := rows.Scan(&codProd, &qt, &embalagem, &qtUnitCx); err != nil {
				return err
			}
			agregada := out[codProd]
			agregada.Qtd += qt
			agregada.Embalagem = embalagem
			agregada.QtUnitCx = qtUnitCx
			out[codProd] = agregada
		}
		return nil
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

// ─── Rollup por nível hierárquico ──────────────────────────────────────────────

// agregarPorNivel agrupa os resultados de Rede (grão atômico) por
// RCA/CRV/GGV, sem NUNCA somar um valor já agregado de outro nível — cada
// grupo é montado a partir da lista de Redes crua (mesmo princípio do
// FR18a e dos bugs de totalizador já documentados no Farol).
//
// Desde 2026-09-04 não faz mais consulta ao banco (a hierarquia já vem
// embutida em cada RealizadoRede, importada do CSV de Clientes Válidos —
// ver cabeçalho do arquivo) e o indicador exposto nos 3 níveis de grupo
// (rca/crv/ggv) é SEMPRE contagem de redes atingindo/faltando, pras duas
// métricas — nunca mais uma média nesses níveis (só o nível "rede" mostra
// médias/valores). Isso bate com o modelo real da JC (abas "Resumo GGvs
// Crvs" e "Resumo GGvs Crvs Rcas": QT REDES ATINGINDO/FALTA ATINGIR).
func agregarPorNivel(redes []RealizadoRede, nivel string) ([]RealizadoGrupo, error) {
	chave := func(r RealizadoRede) (codigo, nome string) {
		switch nivel {
		case "rca":
			return r.CodRCA, r.NomeRCA
		case "crv":
			return r.CodCRV, r.NomeCRV
		case "ggv":
			return r.CodGGV, r.NomeGGV
		}
		return "", ""
	}
	if nivel != "rca" && nivel != "crv" && nivel != "ggv" {
		return nil, fmt.Errorf("nível inválido: %q (use rede, rca, crv ou ggv)", nivel)
	}

	ordem := []string{}
	nomePorGrupo := map[string]string{}
	qtdPorGrupo := map[string]int{}
	atingindoPorGrupo := map[string]int{}
	for _, r := range redes {
		codigo, nome := chave(r)
		if codigo == "" {
			codigo = "(sem dono resolvido)"
		}
		if _, ok := qtdPorGrupo[codigo]; !ok {
			ordem = append(ordem, codigo)
			nomePorGrupo[codigo] = nome
		}
		qtdPorGrupo[codigo]++
		if r.Atingiu {
			atingindoPorGrupo[codigo]++
		}
	}

	var out []RealizadoGrupo
	for _, codigo := range ordem {
		atingindo := atingindoPorGrupo[codigo]
		total := qtdPorGrupo[codigo]
		out = append(out, RealizadoGrupo{
			Codigo: codigo, Nome: nomePorGrupo[codigo],
			RealizadoTotal: float64(atingindo), QtdRedes: total,
			QtdAtingindo: atingindo, QtdFaltaAtingir: total - atingindo,
		})
	}
	return out, nil
}

// ─── Helpers pequenos ──────────────────────────────────────────────────────────

func numeroDeParametro(parametros map[string]any, chave string) (float64, bool) {
	v, ok := parametros[chave]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		var f float64
		if _, err := fmt.Sscanf(n, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// projetarFechamento aplica o método v1 (ritmo linear) do FR18:
// projeção = realizado ÷ dias decorridos no período × dias totais do
// período. Exemplo do PRD: R$45.000 em 15 dias de um mês de 30 → projeção
// R$90.000. Se o período já terminou, dias decorridos = dias totais, e a
// projeção vira o próprio realizado (nada a extrapolar).
func projetarFechamento(realizado float64, dataInicio, dataFim string) float64 {
	inicio, err1 := time.Parse("2006-01-02", dataInicio)
	fim, err2 := time.Parse("2006-01-02", dataFim)
	if err1 != nil || err2 != nil || !fim.After(inicio.Add(-24*time.Hour)) {
		return realizado
	}
	hoje := time.Now().Truncate(24 * time.Hour)

	diasTotais := int(fim.Sub(inicio).Hours()/24) + 1
	diasDecorridos := diasTotais
	if hoje.Before(fim) {
		d := int(hoje.Sub(inicio).Hours()/24) + 1
		if d < 1 {
			d = 1
		}
		diasDecorridos = d
	}
	if diasDecorridos <= 0 || diasTotais <= 0 {
		return realizado
	}
	return realizado / float64(diasDecorridos) * float64(diasTotais)
}

// calcularRecorteDatas resolve os 4 recortes de tempo do FR21 (dia
// anterior, semana, mês, ano corrente) em [data_inicio, data_fim] —
// independente da vigência, sempre relativo a hoje.
func calcularRecorteDatas(recorte string) (dataInicio, dataFim string, err error) {
	hoje := time.Now().Truncate(24 * time.Hour)
	fmtd := func(t time.Time) string { return t.Format("2006-01-02") }
	switch recorte {
	case "dia_anterior":
		ontem := hoje.AddDate(0, 0, -1)
		return fmtd(ontem), fmtd(ontem), nil
	case "semana":
		return fmtd(hoje.AddDate(0, 0, -6)), fmtd(hoje), nil
	case "mes":
		inicioMes := time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, hoje.Location())
		return fmtd(inicioMes), fmtd(hoje), nil
	case "ano_corrente":
		inicioAno := time.Date(hoje.Year(), 1, 1, 0, 0, 0, 0, hoje.Location())
		return fmtd(inicioAno), fmtd(hoje), nil
	default:
		return "", "", fmt.Errorf("recorte inválido: %q (use dia_anterior, semana, mes ou ano_corrente)", recorte)
	}
}

// periodoIncluiHoje diz se o período apurado ainda está "em andamento" —
// mês corrente é sempre parcial (não há como já ter todas as vendas do mês
// se o mês não terminou). Comparação por data (não por "é o mês corrente")
// pra funcionar igual com vigências de qualquer duração, não só mensais.
func periodoIncluiHoje(dataFim string) bool {
	fim, err := time.Parse("2006-01-02", dataFim)
	if err != nil {
		return false
	}
	hoje := time.Now().Truncate(24 * time.Hour)
	return !fim.Before(hoje)
}

// ─── Filtro de escopo hierárquico (drill-down GGV → CRV → RCA → Rede) ─────────

// filtrarRedesPorHierarquia restringe uma lista de Redes já calculada aos
// códigos pedidos — usado tanto pro drill-down do painel (Épico 5, cliente
// escolhe "abrir" um GGV/CRV/RCA) quanto pro escopo obrigatório de login
// (farol_escopo.go: um GGV só pode abrir o próprio código). Campos vazios
// não filtram (não restringe).
func filtrarRedesPorHierarquia(redes []RealizadoRede, codGGV, codCRV, codRCA string) []RealizadoRede {
	if codGGV == "" && codCRV == "" && codRCA == "" {
		return redes
	}
	out := make([]RealizadoRede, 0, len(redes))
	for _, r := range redes {
		if codGGV != "" && r.CodGGV != codGGV {
			continue
		}
		if codCRV != "" && r.CodCRV != codCRV {
			continue
		}
		if codRCA != "" && r.CodRCA != codRCA {
			continue
		}
		out = append(out, r)
	}
	return out
}

// aplicarFiltroHierarquiaEEscopo filtra Redes por hierarquia (drill-down
// pedido pelo usuário — cod_ggv/cod_crv/cod_rca — combinado com o escopo
// obrigatório de login, farol_escopo.go) e recalcula total/grupos/projeção
// a partir do resultado filtrado. Usado pelo painel WEB autenticado
// (Épico 5) depois que CalcularRealizado/obterOuCongelarRealizado já rodou
// sobre a lista completa — o congelamento é sempre da EMPRESA inteira,
// nunca por escopo, senão o snapshot de um GGV divergiria do de outro pro
// mesmo mês (mesma vigência, mesmo cálculo, só a exibição é recortada).
func aplicarFiltroHierarquiaEEscopo(resultado *RealizadoResultado, formulaCodigo, nivel, codGGV, codCRV, codRCA, dataInicioVigencia, dataFimVigencia string) error {
	if codGGV == "" && codCRV == "" && codRCA == "" {
		return nil
	}
	resultado.Redes = filtrarRedesPorHierarquia(resultado.Redes, codGGV, codCRV, codRCA)
	recalculado := recalcularTotalDeRedes(resultado.Redes, formulaCodigo)
	resultado.RealizadoTotal = recalculado.RealizadoTotal
	resultado.Projecao = projetarFechamento(resultado.RealizadoTotal, dataInicioVigencia, dataFimVigencia)
	if nivel != "rede" && nivel != "" {
		grupos, err := agregarPorNivel(resultado.Redes, nivel)
		if err != nil {
			return err
		}
		for i := range grupos {
			grupos[i].Projecao = projetarFechamento(grupos[i].RealizadoTotal, dataInicioVigencia, dataFimVigencia)
		}
		resultado.Grupos = grupos
	} else {
		resultado.Grupos = nil
	}
	return nil
}

// resolverFiltroDrillDown combina o escopo obrigatório de login com o
// pedido de drill-down explícito na URL (?cod_ggv=&cod_crv=&cod_rca=) — o
// request só pode ESTREITAR dentro do escopo já resolvido, nunca escapar
// dele (mesma regra de ouro de farol_escopo.go: escopo sobrescreve o
// request, não o contrário).
func resolverFiltroDrillDown(q map[string][]string, codGGV, codCRV, codRCA string) (string, string, string) {
	get := func(k string) string {
		if v, ok := q[k]; ok && len(v) > 0 {
			return strings.TrimSpace(v[0])
		}
		return ""
	}
	if v := get("cod_ggv"); v != "" && (codGGV == "" || v == codGGV) {
		codGGV = v
	}
	if v := get("cod_crv"); v != "" && (codCRV == "" || v == codCRV) {
		codCRV = v
	}
	if v := get("cod_rca"); v != "" && (codRCA == "" || v == codRCA) {
		codRCA = v
	}
	return codGGV, codCRV, codRCA
}

// recalcularTotalDeRedes reconstrói RealizadoTotal a partir de uma lista
// (já filtrada) de Redes — mesma lógica de agregação usada em
// CalcularRealizadoComPeriodo, extraída aqui pra reuso sem duplicar regra
// (usada tanto pelo painel público — farol_metas_public.go — quanto pelo
// escopo do painel web autenticado).
func recalcularTotalDeRedes(redes []RealizadoRede, formulaCodigo string) *RealizadoResultado {
	resultado := &RealizadoResultado{Redes: redes}
	if redes == nil {
		resultado.Redes = []RealizadoRede{}
	}
	switch formulaCodigo {
	case "cobertura_rede":
		count := 0
		for _, r := range redes {
			if r.Atingiu {
				count++
			}
		}
		resultado.RealizadoTotal = float64(count)
	case "sortimento_rede":
		var soma float64
		for _, r := range redes {
			soma += r.Valor
		}
		if len(redes) > 0 {
			resultado.RealizadoTotal = soma / float64(len(redes))
		}
	}
	return resultado
}

// ─── MetasRealizadoHandler — GET /api/farol/metas-realizado ──────────────

// GET /api/farol/metas-realizado?vinculo_id=&vigencia_id=&fluxo=faturado|transmitido&nivel=rede|rca|crv|ggv
func MetasRealizadoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		vinculoID, err1 := strconv.Atoi(r.URL.Query().Get("vinculo_id"))
		vigenciaID, err2 := strconv.Atoi(r.URL.Query().Get("vigencia_id"))
		if err1 != nil || err2 != nil {
			http.Error(w, "vinculo_id e vigencia_id são obrigatórios", http.StatusBadRequest)
			return
		}
		fluxo := r.URL.Query().Get("fluxo")
		if fluxo == "" {
			fluxo = "faturado"
		}
		nivel := r.URL.Query().Get("nivel")
		if nivel == "" {
			nivel = "rede"
		}

		resultado, err := obterOuCongelarRealizado(db, spCtx.EmpresaID, vinculoID, vigenciaID, fluxo, nivel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resultado)
	}
}
