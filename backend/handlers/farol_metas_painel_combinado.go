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
)

type PainelCombinadoRede struct {
	CodPrinc            string  `json:"cod_princ"`
	Razao               string  `json:"razao"`
	Fantasia            string  `json:"fantasia"`
	QtLojas             int     `json:"qt_lojas"`
	CodGGV              string  `json:"cod_ggv"`
	NomeGGV             string  `json:"nome_ggv"`
	CodCRV              string  `json:"cod_crv"`
	NomeCRV             string  `json:"nome_crv"`
	CodRCA              string  `json:"cod_rca"`
	NomeRCA             string  `json:"nome_rca"`
	CoberturaValor      float64 `json:"cobertura_valor"`
	CoberturaValorTotal float64 `json:"cobertura_valor_total"`
	CoberturaObjetivo   float64 `json:"cobertura_objetivo"`
	CoberturaFalta      float64 `json:"cobertura_falta"`
	CoberturaAtingiu    bool    `json:"cobertura_atingiu"`
	SortimentoValor     float64 `json:"sortimento_valor"`
	SortimentoObjetivo  float64 `json:"sortimento_objetivo"`
	SortimentoFalta     float64 `json:"sortimento_falta"`
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
	IndustriaNome string                `json:"industria_nome"`
	Vigencia      PainelVigencia        `json:"vigencia"`
	Cobertura     PainelMetricaResumo   `json:"cobertura"`
	Sortimento    PainelMetricaResumo   `json:"sortimento"`
	Redes         []PainelCombinadoRede `json:"redes"`
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
// painel público mobile: calcula as duas métricas (via
// obterOuCongelarRealizado — respeita o congelamento de mês fechado, Story
// 4.3) e monta as linhas por Rede.
func calcularPainelCombinado(db *sql.DB, empresaID string, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID int, fluxo string) (*PainelCombinadoResponse, error) {
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

	realizadoCobertura, err := obterOuCongelarRealizado(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID, fluxo, "rede")
	if err != nil {
		return nil, err
	}
	realizadoSortimento, err := obterOuCongelarRealizado(db, empresaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo, "rede")
	if err != nil {
		return nil, err
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
		})
	}

	return &PainelCombinadoResponse{
		IndustriaNome: industriaNome, Vigencia: vig,
		Cobertura: resumoCobertura, Sortimento: resumoSortimento, Redes: redes,
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

		// Escopo obrigatório de login (farol_escopo.go) + drill-down pedido
		// na URL — mesmo princípio do painel individual (farol_metas_painel.go).
		codGGV, codCRV, codRCA, negarEscopo := escopoHierarquiaMetas(spCtx)
		if negarEscopo {
			http.Error(w, "acesso negado: cadastro de organograma incompleto pra este usuário", http.StatusForbidden)
			return
		}
		codGGV, codCRV, codRCA = resolverFiltroDrillDown(q, codGGV, codCRV, codRCA)

		resp, err := calcularPainelCombinado(db, spCtx.EmpresaID, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo)
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

		resp, err := calcularPainelCombinado(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo)
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

		json.NewEncoder(w).Encode(resp)
	}
}
