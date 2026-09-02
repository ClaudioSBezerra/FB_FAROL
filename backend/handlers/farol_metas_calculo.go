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

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/lib/pq"
)

// ─── Tipos ────────────────────────────────────────────────────────────────────

type RealizadoRede struct {
	RedeNome string  `json:"rede_nome"`
	CodRCA   string  `json:"cod_rca"` // RCA "representante" da Rede — ver resolverRedeRCA
	Valor    float64 `json:"valor"`
	Atingiu  bool    `json:"atingiu"` // só significativo pra métricas do tipo limiar (ex: Cobertura)
}

type RealizadoGrupo struct {
	Codigo         string  `json:"codigo"`
	Nome           string  `json:"nome"`
	RealizadoTotal float64 `json:"realizado_total"`
	QtdRedes       int     `json:"qtd_redes"`
	Projecao       float64 `json:"projecao"` // fechamento projetado deste nível — calculado a partir do RealizadoTotal DESTE grupo, nunca somando projeções de Redes/níveis filhos (FR18a)
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

// clienteValido é uma linha crua de farol.metas_clientes_validos.
type clienteValido struct {
	RedeNome string
	CNPJ     string
	CodRCA   string
}

// itemValido é uma linha crua de farol.metas_itens_validos, já com o EAN
// que ele representa (um EAN pode ter 2+ linhas, uma por cod_prod/embalagem).
type itemValido struct {
	EAN           string
	CodProd       string
	TipoEmbalagem string
}

// itemInfo é o mapa cod_prod -> {EAN, exige quantidade mínima} usado pra
// checar a regra de positivação do Sortimento (FR12).
type itemInfo struct {
	ean         string
	exigeMinimo bool // só true quando tipo_embalagem == "UN"
}

// ─── Ponto de entrada — dispatch por formula_codigo ───────────────────────────

// CalcularRealizado calcula o Realizado de um vínculo/vigência, no nível
// hierárquico pedido, lendo o fluxo indicado (faturado/transmitido/soma),
// sobre o período INTEIRO da vigência — uso normal (Épicos 4/5.1/5.2).
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
	var tiposVendaValidos []string
	err := db.QueryRow(`
		SELECT tm.formula_codigo, v.data_inicio::text, v.data_fim::text
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3
	`, vigenciaID, vinculoID, empresaID).Scan(&formulaCodigo, &dataInicioVigencia, &dataFimVigencia)
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
		redes, err = calcularCoberturaPorRede(db, empresaID, clientes, parametros, dataInicio, dataFim, fluxo, tiposVendaValidos)
	case "sortimento_rede":
		itens, ierr := lerItensValidos(db, empresaID, vigenciaID)
		if ierr != nil {
			return nil, ierr
		}
		if len(itens) == 0 {
			return nil, fmt.Errorf("nenhum Item Válido importado pra esta vigência — importe a lista antes de calcular (Épico 3)")
		}
		redes, err = calcularSortimentoPorRede(db, empresaID, clientes, itens, parametros, dataInicio, dataFim, fluxo, tiposVendaValidos)
	case "":
		return nil, fmt.Errorf("este Tipo de Métrica não tem calculadora implementada (formula_codigo vazio)")
	default:
		return nil, fmt.Errorf("formula_codigo desconhecido: %q — nenhuma calculadora registrada", formulaCodigo)
	}
	if err != nil {
		return nil, err
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
		resultado.Grupos, err = agregarPorNivel(db, empresaID, redes, nivel, formulaCodigo)
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

func lerClientesValidos(db *sql.DB, empresaID string, vigenciaID int) ([]clienteValido, error) {
	rows, err := db.Query(`SELECT rede_nome, cnpj, cod_rca FROM farol.metas_clientes_validos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []clienteValido
	for rows.Next() {
		var c clienteValido
		if err := rows.Scan(&c.RedeNome, &c.CNPJ, &c.CodRCA); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func lerItensValidos(db *sql.DB, empresaID string, vigenciaID int) ([]itemValido, error) {
	rows, err := db.Query(`SELECT ean, cod_prod, tipo_embalagem FROM farol.metas_itens_validos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []itemValido
	for rows.Next() {
		var i itemValido
		if err := rows.Scan(&i.EAN, &i.CodProd, &i.TipoEmbalagem); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

// agruparPorRede indexa os Clientes Válidos por Rede, preservando ordem de
// primeira aparição (determinístico).
func agruparPorRede(clientes []clienteValido) (ordem []string, porRede map[string][]clienteValido) {
	porRede = map[string][]clienteValido{}
	for _, c := range clientes {
		if _, ok := porRede[c.RedeNome]; !ok {
			ordem = append(ordem, c.RedeNome)
		}
		porRede[c.RedeNome] = append(porRede[c.RedeNome], c)
	}
	return
}

// redeRCARepresentante resolve o RCA "dono" de uma Rede como o RCA do
// primeiro CNPJ (ordem alfabética) — regra determinística e simples,
// documentada como decisão desta story: uma Rede pode ter CNPJs de RCAs
// diferentes (multi-loja), e a apuração por Rede (FR18a) não deve
// fragmentar o cálculo por RCA — mas o painel (Épico 5/6) precisa de UM
// RCA pra "ligar e cobrar" (UJ-1). Fica registrado aqui como assunção a
// validar com o Claudio/PM quando o painel estiver pronto pra revisão.
func redeRCARepresentante(clientesDaRede []clienteValido) string {
	if len(clientesDaRede) == 0 {
		return ""
	}
	menor := clientesDaRede[0]
	for _, c := range clientesDaRede[1:] {
		if c.CNPJ < menor.CNPJ {
			menor = c
		}
	}
	return menor.CodRCA
}

// ─── Cobertura por Rede ────────────────────────────────────────────────────────

func calcularCoberturaPorRede(db *sql.DB, empresaID string, clientes []clienteValido, parametros map[string]any, dataInicio, dataFim, fluxo string, tiposVenda []string) ([]RealizadoRede, error) {
	limiar, ok := numeroDeParametro(parametros, "limiar_valor_medio")
	if !ok {
		return nil, fmt.Errorf("vínculo não tem o parâmetro limiar_valor_medio preenchido — obrigatório pra Cobertura por Rede")
	}

	ordem, porRede := agruparPorRede(clientes)
	var out []RealizadoRede
	for _, redeNome := range ordem {
		clientesDaRede := porRede[redeNome]
		var somaCompras float64
		for _, c := range clientesDaRede {
			valor, err := somaPvendaCliente(db, empresaID, c.CNPJ, dataInicio, dataFim, fluxo, tiposVenda)
			if err != nil {
				return nil, err
			}
			somaCompras += valor
		}
		media := somaCompras / float64(len(clientesDaRede))
		out = append(out, RealizadoRede{
			RedeNome: redeNome, CodRCA: redeRCARepresentante(clientesDaRede),
			Valor: media, Atingiu: media >= limiar,
		})
	}
	return out, nil
}

// somaPvendaCliente soma pvenda (Faturado/Transmitido/Soma) de um CNPJ no
// período, respeitando o(s) tipo(s) de venda válido(s) do vínculo (FR16) —
// vazio = sem filtro de tipo_venda ("Líquido" padrão do Farol, que já é o
// conteúdo natural de vendas_faturadas/vendas_transmitidas sem filtro
// adicional).
func somaPvendaCliente(db *sql.DB, empresaID, cnpj, dataInicio, dataFim, fluxo string, tiposVenda []string) (float64, error) {
	var total float64
	somar := func(tabela, colData string) error {
		query := fmt.Sprintf(`
			SELECT COALESCE(SUM(pvenda), 0) FROM %s
			WHERE empresa_id = $1 AND cnpj = $2 AND %s BETWEEN $3 AND $4
		`, tabela, colData)
		args := []any{empresaID, cnpj, dataInicio, dataFim}
		if len(tiposVenda) > 0 {
			query += " AND tipo_venda = ANY($5)"
			args = append(args, pq.Array(tiposVenda))
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
	case "soma":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return 0, err
		}
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("fluxo inválido: %q (use faturado, transmitido ou soma)", fluxo)
	}
	return total, nil
}

// ─── Sortimento por Rede ────────────────────────────────────────────────────

func calcularSortimentoPorRede(db *sql.DB, empresaID string, clientes []clienteValido, itens []itemValido, parametros map[string]any, dataInicio, dataFim, fluxo string, tiposVenda []string) ([]RealizadoRede, error) {
	qtdMinima, ok := numeroDeParametro(parametros, "qtd_minima_positivacao")
	if !ok {
		return nil, fmt.Errorf("vínculo não tem o parâmetro qtd_minima_positivacao preenchido — obrigatório pra Sortimento por Rede")
	}

	// cod_prod -> {ean, exige quantidade mínima (só quando embalagem = UN)}
	itensPorCodProd := map[string]itemInfo{}
	for _, it := range itens {
		itensPorCodProd[it.CodProd] = itemInfo{ean: it.EAN, exigeMinimo: it.TipoEmbalagem == "UN"}
	}

	ordem, porRede := agruparPorRede(clientes)
	var out []RealizadoRede
	for _, redeNome := range ordem {
		clientesDaRede := porRede[redeNome]
		var somaEANsPorLoja float64
		for _, c := range clientesDaRede {
			qtdEANs, err := contarEANsPositivadosCliente(db, empresaID, c.CNPJ, dataInicio, dataFim, fluxo, tiposVenda, itensPorCodProd, qtdMinima)
			if err != nil {
				return nil, err
			}
			somaEANsPorLoja += qtdEANs
		}
		media := somaEANsPorLoja / float64(len(clientesDaRede))
		out = append(out, RealizadoRede{RedeNome: redeNome, CodRCA: redeRCARepresentante(clientesDaRede), Valor: media})
	}
	return out, nil
}

// contarEANsPositivadosCliente conta quantos EANs distintos (da lista de
// Itens Válidos) um CNPJ comprou no período, respeitando a regra de
// quantidade mínima (só pra itens UN — FR12).
func contarEANsPositivadosCliente(db *sql.DB, empresaID, cnpj, dataInicio, dataFim, fluxo string, tiposVenda []string, itensPorCodProd map[string]itemInfo, qtdMinima float64) (float64, error) {
	linhas, err := qtdPorCodProdCliente(db, empresaID, cnpj, dataInicio, dataFim, fluxo, tiposVenda)
	if err != nil {
		return 0, err
	}
	eansPositivados := map[string]bool{}
	for codProd, qtd := range linhas {
		info, ok := itensPorCodProd[codProd]
		if !ok {
			continue // produto vendido não está na lista de Itens Válidos deste programa
		}
		if info.exigeMinimo && qtd < qtdMinima {
			continue
		}
		eansPositivados[info.ean] = true
	}
	return float64(len(eansPositivados)), nil
}

// qtdPorCodProdCliente soma a quantidade vendida (qt) por cod_prod, pra um
// CNPJ no período — base pra aplicar a regra de quantidade mínima.
func qtdPorCodProdCliente(db *sql.DB, empresaID, cnpj, dataInicio, dataFim, fluxo string, tiposVenda []string) (map[string]float64, error) {
	out := map[string]float64{}
	somar := func(tabela, colData string) error {
		query := fmt.Sprintf(`
			SELECT cod_prod, SUM(qt) FROM %s
			WHERE empresa_id = $1 AND cnpj = $2 AND %s BETWEEN $3 AND $4 AND cod_prod <> ''
		`, tabela, colData)
		args := []any{empresaID, cnpj, dataInicio, dataFim}
		if len(tiposVenda) > 0 {
			query += " AND tipo_venda = ANY($5)"
			args = append(args, pq.Array(tiposVenda))
		}
		query += " GROUP BY cod_prod"
		rows, err := db.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var codProd string
			var qt float64
			if err := rows.Scan(&codProd, &qt); err != nil {
				return err
			}
			out[codProd] += qt
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
	case "soma":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
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

// ─── Rollup por nível hierárquico ──────────────────────────────────────────────

// agregarPorNivel agrupa os resultados de Rede (grão atômico) por
// RCA/CRV/GGV, sem NUNCA somar um valor já agregado de outro nível — cada
// grupo é montado a partir da lista de Redes crua (mesmo princípio do
// FR18a e dos bugs de totalizador já documentados no Farol).
func agregarPorNivel(db *sql.DB, empresaID string, redes []RealizadoRede, nivel, formulaCodigo string) ([]RealizadoGrupo, error) {
	codigoDoNivel := map[string]string{} // cod_rca -> código do nível pedido (ele mesmo, cod_supervisor ou cod_gerente)
	nomeDoNivel := map[string]string{}
	for _, r := range redes {
		if r.CodRCA == "" {
			continue
		}
		if _, ok := codigoDoNivel[r.CodRCA]; ok {
			continue
		}
		codRCA, codSup, nomeSup, codGer, nomeGer, err := resolverHierarquiaRCA(db, empresaID, r.CodRCA)
		if err != nil {
			return nil, err
		}
		switch nivel {
		case "rca":
			codigoDoNivel[r.CodRCA] = codRCA
			nomeDoNivel[r.CodRCA] = codRCA
		case "crv":
			codigoDoNivel[r.CodRCA] = codSup
			nomeDoNivel[r.CodRCA] = nomeSup
		case "ggv":
			codigoDoNivel[r.CodRCA] = codGer
			nomeDoNivel[r.CodRCA] = nomeGer
		default:
			return nil, fmt.Errorf("nível inválido: %q (use rede, rca, crv ou ggv)", nivel)
		}
	}

	ordem := []string{}
	somaPorGrupo := map[string]float64{}
	countPorGrupo := map[string]int{}
	nomePorGrupo := map[string]string{}
	for _, r := range redes {
		codigo := codigoDoNivel[r.CodRCA]
		if codigo == "" {
			codigo = "(sem RCA resolvido)"
		}
		if _, ok := somaPorGrupo[codigo]; !ok {
			ordem = append(ordem, codigo)
			nomePorGrupo[codigo] = nomeDoNivel[r.CodRCA]
		}
		countPorGrupo[codigo]++
		switch formulaCodigo {
		case "cobertura_rede":
			if r.Atingiu {
				somaPorGrupo[codigo]++
			}
		case "sortimento_rede":
			somaPorGrupo[codigo] += r.Valor
		}
	}

	var out []RealizadoGrupo
	for _, codigo := range ordem {
		total := somaPorGrupo[codigo]
		if formulaCodigo == "sortimento_rede" && countPorGrupo[codigo] > 0 {
			total = total / float64(countPorGrupo[codigo])
		}
		out = append(out, RealizadoGrupo{Codigo: codigo, Nome: nomePorGrupo[codigo], RealizadoTotal: total, QtdRedes: countPorGrupo[codigo]})
	}
	return out, nil
}

// resolverHierarquiaRCA descobre CRV/GGV de um RCA lendo a linha de venda
// mais recente que o cita — a hierarquia organizacional no Farol V2 vem
// denormalizada em cada linha de vendas_faturadas/transmitidas (não existe
// tabela separada de RCA→Supervisor→Gerente usada pelo motor V2; ver
// farol_v2_api.go, que já deriva hierarquia da mesma forma).
func resolverHierarquiaRCA(db *sql.DB, empresaID, codRCA string) (rca, supervisor, nomeSupervisor, gerente, nomeGerente string, err error) {
	rca = codRCA
	row := db.QueryRow(`
		SELECT cod_supervisor, nome_supervisor, cod_gerente, nome_gerente
		FROM vendas_faturadas
		WHERE empresa_id = $1 AND cod_rca = $2
		ORDER BY data_faturamento DESC LIMIT 1
	`, empresaID, codRCA)
	err = row.Scan(&supervisor, &nomeSupervisor, &gerente, &nomeGerente)
	if err == sql.ErrNoRows {
		row = db.QueryRow(`
			SELECT cod_supervisor, nome_supervisor, cod_gerente, nome_gerente
			FROM vendas_transmitidas
			WHERE empresa_id = $1 AND cod_rca = $2
			ORDER BY data_transmissao DESC LIMIT 1
		`, empresaID, codRCA)
		err = row.Scan(&supervisor, &nomeSupervisor, &gerente, &nomeGerente)
	}
	if err == sql.ErrNoRows {
		// RCA sem nenhuma venda no histórico — hierarquia não resolvível a
		// partir do dado denormalizado. Não é erro fatal: o nível fica com
		// código vazio, agrupado como "(sem RCA resolvido)".
		return codRCA, "", "", "", "", nil
	}
	if err != nil {
		return "", "", "", "", "", err
	}
	return codRCA, supervisor, nomeSupervisor, gerente, nomeGerente, nil
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

// ─── MetasRealizadoHandler — GET /api/farol/metas-realizado ──────────────

// GET /api/farol/metas-realizado?vinculo_id=&vigencia_id=&fluxo=faturado|transmitido|soma&nivel=rede|rca|crv|ggv
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
