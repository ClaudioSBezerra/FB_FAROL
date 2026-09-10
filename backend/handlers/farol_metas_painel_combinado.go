package handlers

// farol_metas_painel_combinado.go — Painel combinado Cobertura + Sortimento
// (Épico 5/6, ajuste pós-alinhamento com a JC em 2026-09-03) — a planilha
// real da Unilever ("Resumo Redes") mostra as duas métricas lado a lado,
// uma linha por Rede: OBJETIVO COBERTURA · VALOR VENDA MÉDIA · FALTA (R$) ·
// OBJETIVO EANS · QT MÉDIA EANs · FALTA EANs. Este arquivo reaproveita o
// motor de apuração (farol_metas_calculo.go) chamando-o uma vez por
// métrica e fazendo o merge por Rede — a arquitetura de vínculo (1
// vínculo = 1 Tipo de Métrica) não muda, isto é só uma view de leitura que
// junta dois vínculos da MESMA indústria.
//
// "Objetivo" aqui é o alvo POR REDE (não a meta agregada do distribuidor):
// Cobertura usa o limiar_valor_medio do vínculo (ex: R$9.100); Sortimento
// usa a maior faixa cadastrada na vigência (ex: 39 EANs) — o descritivo da
// Unilever declara esse número como o que "cada rede precisa" bater, além
// de ser também o teto da apuração agregada do distribuidor.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// PainelClienteRef — loja/CNPJ de uma Rede, só o suficiente pro filtro
// "Cliente" da barra de filtros do painel (narrow da lista de Redes, não
// detalhamento por loja). Vem de RealizadoRede.Clientes, que o motor de
// Cobertura já calcula e o snapshot de mês fechado preserva.
type PainelClienteRef struct {
	CNPJ string `json:"cnpj"`
	Nome string `json:"nome"`
}

type PainelCombinadoRede struct {
	CodPrinc string `json:"cod_princ"`
	Razao    string `json:"razao"`
	Fantasia string `json:"fantasia"`
	QtLojas  int    `json:"qt_lojas"`
	// UF — não existe no CSV de Clientes Válidos nem em cadastro de cliente
	// algum (só nas linhas de VENDA); resolvido a partir da venda mais
	// recente do CNPJ "dono" da Rede (ver resolverUFClientes), em qualquer
	// período/fornecedor do Farol. Vazio se o dono nunca vendeu nada.
	UF                  string             `json:"uf"`
	CodGGV              string             `json:"cod_ggv"`
	NomeGGV             string             `json:"nome_ggv"`
	CodCRV              string             `json:"cod_crv"`
	NomeCRV             string             `json:"nome_crv"`
	CodRCA              string             `json:"cod_rca"`
	NomeRCA             string             `json:"nome_rca"`
	CoberturaValor      float64            `json:"cobertura_valor"`
	CoberturaValorTotal float64            `json:"cobertura_valor_total"`
	CoberturaObjetivo   float64            `json:"cobertura_objetivo"`
	CoberturaFalta      float64            `json:"cobertura_falta"`
	CoberturaAtingiu    bool               `json:"cobertura_atingiu"`
	SortimentoValor     float64            `json:"sortimento_valor"`
	SortimentoObjetivo  float64            `json:"sortimento_objetivo"`
	SortimentoFalta     float64            `json:"sortimento_falta"`
	Clientes            []PainelClienteRef `json:"clientes,omitempty"`
}

func refsDeClientes(cs []RealizadoCliente) []PainelClienteRef {
	if len(cs) == 0 {
		return nil
	}
	out := make([]PainelClienteRef, 0, len(cs))
	for _, c := range cs {
		nome := c.Fantasia
		if nome == "" {
			nome = c.Razao
		}
		out = append(out, PainelClienteRef{CNPJ: c.CNPJ, Nome: nome})
	}
	return out
}

// PainelCombinadoCliente — 1 linha por CNPJ/loja (aba "Resumo Rede Cliente"
// do modelo V1 da JC): mesma Cobertura+Sortimento lado a lado da linha de
// Rede, mas no grão de uma loja só — por isso "objetivo" aqui é comparado
// direto contra o valor da loja (sem média entre lojas da Rede).
type PainelCombinadoCliente struct {
	CodPrinc           string  `json:"cod_princ"`
	CNPJ               string  `json:"cnpj"`
	Razao              string  `json:"razao"`
	Fantasia           string  `json:"fantasia"`
	UF                 string  `json:"uf"` // ver PainelCombinadoRede.UF — aqui é exato (o CNPJ do próprio cliente, não um "dono" aproximado)
	CodGGV             string  `json:"cod_ggv"`
	NomeGGV            string  `json:"nome_ggv"`
	CodCRV             string  `json:"cod_crv"`
	NomeCRV            string  `json:"nome_crv"`
	CodRCA             string  `json:"cod_rca"`
	NomeRCA            string  `json:"nome_rca"`
	CoberturaValor     float64 `json:"cobertura_valor"`
	CoberturaObjetivo  float64 `json:"cobertura_objetivo"`
	SortimentoValor    float64 `json:"sortimento_valor"`
	SortimentoObjetivo float64 `json:"sortimento_objetivo"`
}

// resolverUFClientes resolve o UF de cada CNPJ a partir da venda mais
// RECENTE dele — qualquer período, qualquer fornecedor, faturado OU
// transmitido (o que vier depois na ordenação) — decisão do Claudio em
// 10/09/2026: não existe UF na lista de Clientes Válidos nem em cadastro
// de cliente algum, só nas linhas de venda (migration 156). CNPJ que nunca
// vendeu nada no Farol simplesmente não aparece no mapa.
func resolverUFClientes(db *sql.DB, empresaID string, cnpjs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(cnpjs) == 0 {
		return out, nil
	}
	rows, err := db.Query(`
		SELECT DISTINCT ON (cnpj) cnpj, uf FROM (
			SELECT cnpj, uf, data_faturamento AS data FROM vendas_faturadas
				WHERE empresa_id = $1 AND cnpj = ANY($2) AND uf <> ''
			UNION ALL
			SELECT cnpj, uf, data_transmissao AS data FROM vendas_transmitidas
				WHERE empresa_id = $1 AND cnpj = ANY($2) AND uf <> ''
		) x
		ORDER BY cnpj, data DESC
	`, empresaID, pq.Array(cnpjs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cnpj, uf string
		if err := rows.Scan(&cnpj, &uf); err != nil {
			return nil, err
		}
		out[cnpj] = uf
	}
	return out, rows.Err()
}

// montarClientesCombinado explode as Redes já mescladas (`redes`, que já
// resolveu o dono GGV/CRV/RCA e cobre tanto o caso normal quanto Redes
// presentes só numa das duas métricas) em 1 linha por CNPJ, juntando o
// valor de Cobertura e o de Sortimento daquele CNPJ especificamente — as
// duas métricas têm sua PRÓPRIA lista de Clientes Válidos (vínculos
// diferentes), então o cruzamento é por CNPJ dentro da mesma Rede, não uma
// suposição de que as duas listas são idênticas.
func montarClientesCombinado(redes []PainelCombinadoRede, realizadoCobertura, realizadoSortimento *RealizadoResultado, ufPorCliente map[string]string) []PainelCombinadoCliente {
	contextoPorRede := make(map[string]PainelCombinadoRede, len(redes))
	for _, r := range redes {
		contextoPorRede[r.CodPrinc] = r
	}
	sortimentoPorRedeCliente := map[string]map[string]RealizadoCliente{}
	for _, r := range realizadoSortimento.Redes {
		m := make(map[string]RealizadoCliente, len(r.Clientes))
		for _, c := range r.Clientes {
			m[c.CNPJ] = c
		}
		sortimentoPorRedeCliente[r.CodPrinc] = m
	}

	var out []PainelCombinadoCliente
	visto := map[string]bool{}
	for _, rede := range realizadoCobertura.Redes {
		ctx := contextoPorRede[rede.CodPrinc]
		sortMap := sortimentoPorRedeCliente[rede.CodPrinc]
		for _, c := range rede.Clientes {
			s := sortMap[c.CNPJ]
			out = append(out, PainelCombinadoCliente{
				CodPrinc: rede.CodPrinc, CNPJ: c.CNPJ, Razao: c.Razao, Fantasia: c.Fantasia,
				UF:     ufPorCliente[c.CNPJ],
				CodGGV: ctx.CodGGV, NomeGGV: ctx.NomeGGV, CodCRV: ctx.CodCRV, NomeCRV: ctx.NomeCRV,
				CodRCA: ctx.CodRCA, NomeRCA: ctx.NomeRCA,
				CoberturaValor: c.Valor, CoberturaObjetivo: ctx.CoberturaObjetivo,
				SortimentoValor: s.Valor, SortimentoObjetivo: ctx.SortimentoObjetivo,
			})
			visto[c.CNPJ] = true
		}
	}
	// CNPJs presentes só na lista de Sortimento — mesmo princípio de não
	// descartar dado silenciosamente usado no nível Rede.
	for _, rede := range realizadoSortimento.Redes {
		ctx := contextoPorRede[rede.CodPrinc]
		for _, c := range rede.Clientes {
			if visto[c.CNPJ] {
				continue
			}
			out = append(out, PainelCombinadoCliente{
				CodPrinc: rede.CodPrinc, CNPJ: c.CNPJ, Razao: c.Razao, Fantasia: c.Fantasia,
				UF:     ufPorCliente[c.CNPJ],
				CodGGV: ctx.CodGGV, NomeGGV: ctx.NomeGGV, CodCRV: ctx.CodCRV, NomeCRV: ctx.NomeCRV,
				CodRCA: ctx.CodRCA, NomeRCA: ctx.NomeRCA,
				CoberturaObjetivo: ctx.CoberturaObjetivo,
				SortimentoValor:   c.Valor, SortimentoObjetivo: ctx.SortimentoObjetivo,
			})
			visto[c.CNPJ] = true
		}
	}
	return out
}

// PainelMetricaResumo é o mesmo resumo (faixa atual/próxima/delta) que o
// painel de métrica única já expõe (PainelResponse) — extraído aqui pra
// reuso sem duplicar a lógica de "qual faixa foi atingida".
type PainelMetricaResumo struct {
	VinculoID      int           `json:"vinculo_id"`
	VigenciaID     int           `json:"vigencia_id"`
	RealizadoTotal float64       `json:"realizado_total"`
	Projecao       float64       `json:"projecao"`
	Parcial        bool          `json:"parcial"`
	Faixas         []PainelFaixa `json:"faixas"`
	FaixaAtual     *PainelFaixa  `json:"faixa_atual"`
	ProximaFaixa   *PainelFaixa  `json:"proxima_faixa"`
	Delta          float64       `json:"delta"`
}

type PainelCombinadoResponse struct {
	IndustriaNome string                   `json:"industria_nome"`
	Vigencia      PainelVigencia           `json:"vigencia"`
	Cobertura     PainelMetricaResumo      `json:"cobertura"`
	Sortimento    PainelMetricaResumo      `json:"sortimento"`
	Redes         []PainelCombinadoRede    `json:"redes"`
	Clientes      []PainelCombinadoCliente `json:"clientes"` // aba "Resumo Rede Cliente" — 1 linha por CNPJ/loja
	// DataInicioUsada/DataFimUsada — o período REALMENTE calculado: os
	// bounds da vigência (padrão) ou o override pedido em data_inicio/
	// data_fim (narrowing "de: até:", pedido do Claudio em 10/09/2026— ver
	// resolverPeriodoCombinado). O front usa isto pra saber o que preencher
	// nos campos de data por padrão.
	DataInicioUsada string `json:"data_inicio_usada"`
	DataFimUsada    string `json:"data_fim_usada"`
}

// montarResumoMetrica calcula faixa_atual/proxima_faixa/delta a partir do
// realizado e das faixas cadastradas — mesma regra de MetasPainelHandler
// (compara por valor_meta, nunca pelo número ordinal da faixa).
func montarResumoMetrica(db *sql.DB, empresaID string, vinculoID, vigenciaID int, realizado *RealizadoResultado) (PainelMetricaResumo, error) {
	rows, err := db.Query(`SELECT faixa, valor_meta FROM farol.metas_faixas WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
	if err != nil {
		return PainelMetricaResumo{}, err
	}
	defer rows.Close()
	var faixas []PainelFaixa
	for rows.Next() {
		var f PainelFaixa
		if err := rows.Scan(&f.Faixa, &f.ValorMeta); err != nil {
			return PainelMetricaResumo{}, err
		}
		f.Atingida = realizado.RealizadoTotal >= f.ValorMeta
		faixas = append(faixas, f)
	}
	sort.Slice(faixas, func(i, j int) bool { return faixas[i].ValorMeta < faixas[j].ValorMeta })

	resumo := PainelMetricaResumo{
		VinculoID: vinculoID, VigenciaID: vigenciaID,
		RealizadoTotal: realizado.RealizadoTotal, Projecao: realizado.Projecao, Parcial: realizado.Parcial,
		Faixas: faixas,
	}
	for i := range faixas {
		if faixas[i].Atingida {
			f := faixas[i]
			resumo.FaixaAtual = &f
		} else if resumo.ProximaFaixa == nil {
			f := faixas[i]
			resumo.ProximaFaixa = &f
		}
	}
	if resumo.ProximaFaixa != nil {
		resumo.Delta = resumo.ProximaFaixa.ValorMeta - realizado.RealizadoTotal
	}
	return resumo, nil
}

// maiorValorMeta devolve a maior faixa cadastrada (o "teto" da métrica) —
// pro Sortimento é o objetivo por Rede que o descritivo da Unilever
// declara (ex: 39 EANs), independente de qual faixa ordinal carrega esse
// valor.
func maiorValorMeta(faixas []PainelFaixa) float64 {
	var maior float64
	for _, f := range faixas {
		if f.ValorMeta > maior {
			maior = f.ValorMeta
		}
	}
	return maior
}

func faltaOuZero(objetivo, valor float64) float64 {
	if valor >= objetivo {
		return 0
	}
	return objetivo - valor
}

// calcularPainelCombinado é o núcleo compartilhado pelo painel web e pelo
// painel público mobile: calcula as duas métricas e monta as linhas por
// Rede.
//
// dataInicioOverride/dataFimOverride ("Período: de/até", pedido do Claudio
// em 10/09/2026) permitem estreitar o cálculo pra uma janela MENOR que a
// vigência inteira — mesmo princípio dos "recortes" já existentes no
// painel de métrica única (farol_metas_painel.go, FR21): SEMPRE ao vivo,
// nunca passam pelo congelamento (Story 4.3), porque são leitura de
// momentum dentro do período, não o número oficial da vigência. Por isso,
// quando o override bate EXATAMENTE com os bounds da vigência (o caso
// comum — usuário não mexeu no filtro), o cálculo cai de volta em
// obterOuCongelarRealizado pra não pagar o custo de recalcular ao vivo (e,
// mais importante, pra não bypassar o congelamento de um mês FECHADO só
// porque o front preencheu as datas por padrão com os mesmos bounds).
func calcularPainelCombinado(db *sql.DB, empresaID string, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID int, fluxo, dataInicioOverride, dataFimOverride string) (*PainelCombinadoResponse, error) {
	var industriaNome string
	var vig PainelVigencia
	vig.ID = vigenciaCoberturaID
	err := db.QueryRow(`
		SELECT i.nome, v.data_inicio::text, v.data_fim::text, v.status
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.industrias i ON i.id = mv.industria_id
		WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3
	`, vigenciaCoberturaID, vinculoCoberturaID, empresaID).Scan(&industriaNome, &vig.DataInicio, &vig.DataFim, &vig.Status)
	if err != nil {
		return nil, err
	}

	usaPeriodoManual := dataInicioOverride != "" && dataFimOverride != "" &&
		(dataInicioOverride != vig.DataInicio || dataFimOverride != vig.DataFim)
	dataInicioUsada, dataFimUsada := vig.DataInicio, vig.DataFim

	var realizadoCobertura, realizadoSortimento *RealizadoResultado
	if usaPeriodoManual {
		dataInicioUsada, dataFimUsada = dataInicioOverride, dataFimOverride
		realizadoCobertura, err = CalcularRealizadoComPeriodo(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID, fluxo, "rede", dataInicioOverride, dataFimOverride)
		if err != nil {
			return nil, err
		}
		realizadoSortimento, err = CalcularRealizadoComPeriodo(db, empresaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo, "rede", dataInicioOverride, dataFimOverride)
		if err != nil {
			return nil, err
		}
	} else {
		realizadoCobertura, err = obterOuCongelarRealizado(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID, fluxo, "rede")
		if err != nil {
			return nil, err
		}
		realizadoSortimento, err = obterOuCongelarRealizado(db, empresaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo, "rede")
		if err != nil {
			return nil, err
		}
	}

	resumoCobertura, err := montarResumoMetrica(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID, realizadoCobertura)
	if err != nil {
		return nil, err
	}
	resumoSortimento, err := montarResumoMetrica(db, empresaID, vinculoSortimentoID, vigenciaSortimentoID, realizadoSortimento)
	if err != nil {
		return nil, err
	}

	parametrosCobertura, err := lerParametrosValoresVinculo(db, empresaID, vinculoCoberturaID)
	if err != nil {
		return nil, err
	}
	limiarCobertura, _ := numeroDeParametro(parametrosCobertura, "limiar_valor_medio")
	objetivoSortimento := maiorValorMeta(resumoSortimento.Faixas)

	sortimentoPorRede := map[string]RealizadoRede{}
	for _, r := range realizadoSortimento.Redes {
		sortimentoPorRede[r.CodPrinc] = r
	}

	redes := make([]PainelCombinadoRede, 0, len(realizadoCobertura.Redes))
	vistas := map[string]bool{}
	for _, c := range realizadoCobertura.Redes {
		s := sortimentoPorRede[c.CodPrinc]
		redes = append(redes, PainelCombinadoRede{
			CodPrinc: c.CodPrinc, Razao: c.Razao, Fantasia: c.Fantasia, QtLojas: c.QtLojas,
			CodGGV: c.CodGGV, NomeGGV: c.NomeGGV, CodCRV: c.CodCRV, NomeCRV: c.NomeCRV, CodRCA: c.CodRCA, NomeRCA: c.NomeRCA,
			CoberturaValor: c.Valor, CoberturaValorTotal: c.ValorTotal, CoberturaObjetivo: limiarCobertura,
			CoberturaFalta: faltaOuZero(limiarCobertura, c.Valor), CoberturaAtingiu: c.Atingiu,
			SortimentoValor: s.Valor, SortimentoObjetivo: objetivoSortimento,
			SortimentoFalta: faltaOuZero(objetivoSortimento, s.Valor),
			Clientes:        refsDeClientes(c.Clientes),
		})
		vistas[c.CodPrinc] = true
	}
	// Redes presentes só na lista de Sortimento (listas de Clientes Válidos
	// divergentes entre os dois vínculos) — não deveria acontecer no
	// processo normal (mesma lista mensal importada pros dois), mas não
	// descartamos dado silenciosamente se acontecer.
	for _, s := range realizadoSortimento.Redes {
		if vistas[s.CodPrinc] {
			continue
		}
		redes = append(redes, PainelCombinadoRede{
			CodPrinc: s.CodPrinc, Razao: s.Razao, Fantasia: s.Fantasia, QtLojas: s.QtLojas,
			CodGGV: s.CodGGV, NomeGGV: s.NomeGGV, CodCRV: s.CodCRV, NomeCRV: s.NomeCRV, CodRCA: s.CodRCA, NomeRCA: s.NomeRCA,
			CoberturaObjetivo: limiarCobertura,
			SortimentoValor:   s.Valor, SortimentoObjetivo: objetivoSortimento,
			SortimentoFalta: faltaOuZero(objetivoSortimento, s.Valor),
			Clientes:        refsDeClientes(s.Clientes),
		})
	}

	// UF (ver resolverUFClientes) — coletado de TODOS os CNPJs (cobertura +
	// sortimento) antes de montar Clientes/preencher Redes.
	var todosCnpjs []string
	for _, r := range realizadoCobertura.Redes {
		for _, c := range r.Clientes {
			todosCnpjs = append(todosCnpjs, c.CNPJ)
		}
	}
	for _, r := range realizadoSortimento.Redes {
		for _, c := range r.Clientes {
			todosCnpjs = append(todosCnpjs, c.CNPJ)
		}
	}
	ufPorCliente, err := resolverUFClientes(db, empresaID, todosCnpjs)
	if err != nil {
		return nil, err
	}
	// UF da Rede = UF do "dono" (primeiro CNPJ da lista, mesma aproximação
	// já aceita pra GGV/CRV/RCA — ver redeRepresentante em
	// farol_metas_calculo.go): uma Rede pode ter lojas em UFs diferentes,
	// mas o indicador aqui é só pro filtro, não um dado oficial.
	for i := range redes {
		if len(redes[i].Clientes) > 0 {
			redes[i].UF = ufPorCliente[redes[i].Clientes[0].CNPJ]
		}
	}

	clientes := montarClientesCombinado(redes, realizadoCobertura, realizadoSortimento, ufPorCliente)

	return &PainelCombinadoResponse{
		IndustriaNome: industriaNome, Vigencia: vig,
		Cobertura: resumoCobertura, Sortimento: resumoSortimento, Redes: redes, Clientes: clientes,
		DataInicioUsada: dataInicioUsada, DataFimUsada: dataFimUsada,
	}, nil
}

// MetasPainelCombinadoHandler — GET /api/farol/metas-painel-combinado
//
//	?vinculo_cobertura_id=&vigencia_cobertura_id=&vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=
func MetasPainelCombinadoHandler(db *sql.DB) http.HandlerFunc {
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
		q := r.URL.Query()
		vinculoCoberturaID, err1 := strconv.Atoi(q.Get("vinculo_cobertura_id"))
		vigenciaCoberturaID, err2 := strconv.Atoi(q.Get("vigencia_cobertura_id"))
		vinculoSortimentoID, err3 := strconv.Atoi(q.Get("vinculo_sortimento_id"))
		vigenciaSortimentoID, err4 := strconv.Atoi(q.Get("vigencia_sortimento_id"))
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			http.Error(w, "vinculo_cobertura_id, vigencia_cobertura_id, vinculo_sortimento_id e vigencia_sortimento_id são obrigatórios", http.StatusBadRequest)
			return
		}
		fluxo := q.Get("fluxo")
		if fluxo == "" {
			fluxo = "faturado"
		}
		dataInicio := strings.TrimSpace(q.Get("data_inicio"))
		dataFim := strings.TrimSpace(q.Get("data_fim"))

		// Escopo obrigatório de login (farol_escopo.go) + drill-down pedido
		// na URL — mesmo princípio do painel individual (farol_metas_painel.go).
		codGGV, codCRV, codRCA, negarEscopo := escopoHierarquiaMetas(spCtx)
		if negarEscopo {
			http.Error(w, "acesso negado: cadastro de organograma incompleto pra este usuário", http.StatusForbidden)
			return
		}
		codGGV, codCRV, codRCA = resolverFiltroDrillDown(q, codGGV, codCRV, codRCA)

		resp, err := calcularPainelCombinado(db, spCtx.EmpresaID, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo, dataInicio, dataFim)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if codGGV != "" || codCRV != "" || codRCA != "" {
			filtradas := make([]PainelCombinadoRede, 0, len(resp.Redes))
			for _, rede := range resp.Redes {
				if codGGV != "" && rede.CodGGV != codGGV {
					continue
				}
				if codCRV != "" && rede.CodCRV != codCRV {
					continue
				}
				if codRCA != "" && rede.CodRCA != codRCA {
					continue
				}
				filtradas = append(filtradas, rede)
			}
			resp.Redes = filtradas

			clientesFiltrados := make([]PainelCombinadoCliente, 0, len(resp.Clientes))
			for _, c := range resp.Clientes {
				if codGGV != "" && c.CodGGV != codGGV {
					continue
				}
				if codCRV != "" && c.CodCRV != codCRV {
					continue
				}
				if codRCA != "" && c.CodRCA != codRCA {
					continue
				}
				clientesFiltrados = append(clientesFiltrados, c)
			}
			resp.Clientes = clientesFiltrados
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// MetasPublicPainelCombinadoHandler — GET /api/farol/public/metas-painel-combinado
//
//	?cnpj=&scope=sup|rca&cod=&vinculo_cobertura_id=&vigencia_cobertura_id=&vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=
//
// Mesmo recorte de segurança do painel público de métrica única
// (farol_metas_public.go): nunca expõe Rede fora do Supervisor/RCA da URL.
func MetasPublicPainelCombinadoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		empresaID := resolveEmpresaCNPJ(db, q.Get("cnpj"))
		if empresaID == "" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "empresa não encontrada para este CNPJ"})
			return
		}
		scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
		cod := strings.TrimSpace(q.Get("cod"))
		vinculoCoberturaID, err1 := strconv.Atoi(q.Get("vinculo_cobertura_id"))
		vigenciaCoberturaID, err2 := strconv.Atoi(q.Get("vigencia_cobertura_id"))
		vinculoSortimentoID, err3 := strconv.Atoi(q.Get("vinculo_sortimento_id"))
		vigenciaSortimentoID, err4 := strconv.Atoi(q.Get("vigencia_sortimento_id"))
		if (scope != "sup" && scope != "rca") || cod == "" || err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "scope (sup|rca), cod e os 4 ids de vínculo/vigência são obrigatórios"})
			return
		}
		fluxo := q.Get("fluxo")
		if fluxo == "" {
			fluxo = "faturado"
		}
		dataInicio := strings.TrimSpace(q.Get("data_inicio"))
		dataFim := strings.TrimSpace(q.Get("data_fim"))

		resp, err := calcularPainelCombinado(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo, dataInicio, dataFim)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		redesFiltradas := make([]PainelCombinadoRede, 0, len(resp.Redes))
		for _, rede := range resp.Redes {
			if scope == "rca" {
				if rede.CodRCA == cod {
					redesFiltradas = append(redesFiltradas, rede)
				}
				continue
			}
			// scope == "sup": CRV é o dono importado do CSV de Clientes
			// Válidos (ver farol_metas_calculo.go) — não precisa mais de JOIN
			// em vendas pra resolver.
			if rede.CodCRV == cod {
				redesFiltradas = append(redesFiltradas, rede)
			}
		}
		resp.Redes = redesFiltradas

		// Mesmo recorte pra Clientes — endpoint SEM auth, nunca pode vazar
		// CNPJ fora do Supervisor/RCA da URL.
		clientesFiltrados := make([]PainelCombinadoCliente, 0, len(resp.Clientes))
		for _, c := range resp.Clientes {
			if scope == "rca" {
				if c.CodRCA == cod {
					clientesFiltrados = append(clientesFiltrados, c)
				}
				continue
			}
			if c.CodCRV == cod {
				clientesFiltrados = append(clientesFiltrados, c)
			}
		}
		resp.Clientes = clientesFiltrados

		json.NewEncoder(w).Encode(resp)
	}
}
