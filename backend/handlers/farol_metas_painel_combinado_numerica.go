package handlers

// farol_metas_painel_combinado_numerica.go — Painel Combinado Cobertura +
// Sortimento pra Numérica (Épico 7, pedido do Claudio 30/09/2026: "falta
// bater o olho... igual ao que fizemos nas outras métricas"), mesmo
// espírito do Combinado por Rede (farol_metas_painel_combinado.go, ver seu
// cabeçalho pro racional completo) — junta os 2 vínculos da mesma
// Indústria (Cobertura Numérica + Sortimento Numérica) numa view só.
//
// Bem mais simples que o Combinado por Rede: lá existem 2 grãos (Rede E
// Cliente, porque uma Rede tem várias lojas); aqui só existe 1 grão —
// Cliente/CNPJ — porque a Numérica não tem Rede (FR24, CodPrinc já É o
// CNPJ). Não existe "média entre lojas" nem "Rede dona de vários CNPJs":
// cada linha do Cobertura já casa 1:1 com uma linha do Sortimento pelo
// mesmo CNPJ (exceto Classificação C, que fica de fora do Sortimento —
// FR26 — aparece com Sortimento zerado/objetivo zero nesse caso).

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PainelCombinadoNumericaCliente — 1 linha por CNPJ, Cobertura+Sortimento
// lado a lado (mesmo padrão da planilha "Resumo Numerica(s) Cliente" que o
// Carlos manda — ver farol_fechamento_numerica.go).
type PainelCombinadoNumericaCliente struct {
	CNPJ               string  `json:"cnpj"`
	CodCl              string  `json:"cod_cl,omitempty"`
	ClassificacaoPDV   string  `json:"classificacao_pdv"`
	Razao              string  `json:"razao"`
	Fantasia           string  `json:"fantasia"`
	CodGGV             string  `json:"cod_ggv"`
	NomeGGV            string  `json:"nome_ggv"`
	CodCRV             string  `json:"cod_crv"`
	NomeCRV            string  `json:"nome_crv"`
	CodRCA             string  `json:"cod_rca"`
	NomeRCA            string  `json:"nome_rca"`
	CoberturaValor     float64 `json:"cobertura_valor"`
	CoberturaObjetivo  float64 `json:"cobertura_objetivo"`
	CoberturaFalta     float64 `json:"cobertura_falta"`
	CoberturaAtingiu   bool    `json:"cobertura_atingiu"`
	SortimentoValor    float64 `json:"sortimento_valor"`
	SortimentoObjetivo float64 `json:"sortimento_objetivo"`
	SortimentoFalta    float64 `json:"sortimento_falta"`
	SortimentoAtingiu  bool    `json:"sortimento_atingiu"`
	// SortimentoAplicavel — false pra Classificação C (FR26: fora do
	// Sortimento Numérica) — o front não deve mostrar "faltam X PPAs" pra
	// quem nunca vai ser cobrado disso.
	SortimentoAplicavel bool `json:"sortimento_aplicavel"`
	// teveCompra — só usado internamente pra recalcularResumoCombinadoNumericaParaEscopo
	// respeitar o mesmo denominador (Questão #2 do PRD) depois de um filtro
	// GGV/CRV/RCA; não exposto no JSON (front não precisa disso).
	teveCompra bool
}

type PainelCombinadoNumericaResponse struct {
	IndustriaNome   string                           `json:"industria_nome"`
	Vigencia        PainelVigencia                   `json:"vigencia"`
	Cobertura       PainelMetricaResumo              `json:"cobertura"`
	Sortimento      PainelMetricaResumo              `json:"sortimento"`
	Clientes        []PainelCombinadoNumericaCliente `json:"clientes"`
	DataInicioUsada string                           `json:"data_inicio_usada"`
	DataFimUsada    string                           `json:"data_fim_usada"`
}

// calcularPainelCombinadoNumerica — mesmo racional de calcularPainelCombinado,
// simplificado pro grão único (Cliente/CNPJ) da Numérica.
func calcularPainelCombinadoNumerica(db *sql.DB, empresaID string, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID int, fluxo, dataInicioOverride, dataFimOverride string) (*PainelCombinadoNumericaResponse, error) {
	t0 := time.Now()
	defer func() {
		log.Printf("[farol:objetivos] calcularPainelCombinadoNumerica vinculoCob=%d vinculoSort=%d fluxo=%s em %v",
			vinculoCoberturaID, vinculoSortimentoID, fluxo, time.Since(t0))
	}()
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

	sortimentoPorCnpj := map[string]RealizadoRede{}
	for _, r := range realizadoSortimento.Redes {
		sortimentoPorCnpj[r.CodPrinc] = r
	}

	clientes := make([]PainelCombinadoNumericaCliente, 0, len(realizadoCobertura.Redes))
	vistos := map[string]bool{}
	for _, c := range realizadoCobertura.Redes {
		s, temSort := sortimentoPorCnpj[c.CodPrinc]
		// Classificação PDV não é campo de RealizadoRede — vem só do
		// cadastro (clienteNumericaValido), que o motor não devolve aqui.
		// Resolvida à parte abaixo, via classificacaoPdvPorCnpj.
		clientes = append(clientes, PainelCombinadoNumericaCliente{
			CNPJ: c.CodPrinc, CodCl: c.CodCl, Razao: c.Razao, Fantasia: c.Fantasia,
			CodGGV: c.CodGGV, NomeGGV: c.NomeGGV, CodCRV: c.CodCRV, NomeCRV: c.NomeCRV, CodRCA: c.CodRCA, NomeRCA: c.NomeRCA,
			CoberturaValor: c.Valor, CoberturaObjetivo: c.Objetivo, CoberturaFalta: faltaOuZero(c.Objetivo, c.Valor), CoberturaAtingiu: c.Atingiu,
			SortimentoValor: s.Valor, SortimentoObjetivo: s.Objetivo, SortimentoFalta: faltaOuZero(s.Objetivo, s.Valor), SortimentoAtingiu: s.Atingiu,
			SortimentoAplicavel: temSort,
			teveCompra:          s.TeveCompra,
		})
		vistos[c.CodPrinc] = true
	}
	// CNPJs presentes só no Sortimento (não deveria acontecer — mesma lista
	// de clientes alimenta os 2 vínculos — mas não descarta dado silenciosamente).
	for _, s := range realizadoSortimento.Redes {
		if vistos[s.CodPrinc] {
			continue
		}
		clientes = append(clientes, PainelCombinadoNumericaCliente{
			CNPJ: s.CodPrinc, CodCl: s.CodCl, Razao: s.Razao, Fantasia: s.Fantasia,
			CodGGV: s.CodGGV, NomeGGV: s.NomeGGV, CodCRV: s.CodCRV, NomeCRV: s.NomeCRV, CodRCA: s.CodRCA, NomeRCA: s.NomeRCA,
			SortimentoValor: s.Valor, SortimentoObjetivo: s.Objetivo, SortimentoFalta: faltaOuZero(s.Objetivo, s.Valor), SortimentoAtingiu: s.Atingiu,
			SortimentoAplicavel: true,
			teveCompra:          s.TeveCompra,
		})
	}

	// Classificação PDV — vem do cadastro (metas_clientes_numericas), o
	// motor não devolve isso em RealizadoRede. Resolve 1x, direto do
	// cadastro do vínculo de Cobertura (mesma lista que alimenta os 2).
	classifPorCnpj, err := classificacaoPdvPorCnpj(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID)
	if err == nil {
		for i := range clientes {
			clientes[i].ClassificacaoPDV = classifPorCnpj[clientes[i].CNPJ]
		}
	}

	sort.Slice(clientes, func(i, j int) bool { return clientes[i].CoberturaValor > clientes[j].CoberturaValor })

	return &PainelCombinadoNumericaResponse{
		IndustriaNome: industriaNome, Vigencia: vig,
		Cobertura: resumoCobertura, Sortimento: resumoSortimento, Clientes: clientes,
		DataInicioUsada: dataInicioUsada, DataFimUsada: dataFimUsada,
	}, nil
}

func classificacaoPdvPorCnpj(db *sql.DB, empresaID string, vinculoID, vigenciaID int) (map[string]string, error) {
	out := map[string]string{}
	rows, err := db.Query(`SELECT cnpj, classificacao_pdv FROM farol.metas_clientes_numericas WHERE vinculo_id = $1 AND vigencia_id = $2 AND empresa_id = $3`, vinculoID, vigenciaID, empresaID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var cnpj, classif string
		if err := rows.Scan(&cnpj, &classif); err != nil {
			return out, err
		}
		out[cnpj] = classif
	}
	return out, rows.Err()
}

// recalcularResumoCombinadoNumericaParaEscopo — mesmo racional de
// recalcularResumoCombinadoParaEscopo (Combinado por Rede), adaptado pro
// denominador de Sortimento Numérica (Questão #2 do PRD: divide pelos
// clientes com QUALQUER compra, não por todos os elegíveis — ver
// CalcularRealizadoComPeriodo).
func recalcularResumoCombinadoNumericaParaEscopo(clientesFiltrados []PainelCombinadoNumericaCliente, resumoCobertura, resumoSortimento *PainelMetricaResumo) {
	realizadoCobertura := make([]RealizadoRede, len(clientesFiltrados))
	for i, c := range clientesFiltrados {
		realizadoCobertura[i] = RealizadoRede{Atingiu: c.CoberturaAtingiu}
	}
	resumoCobertura.RealizadoTotal = recalcularTotalDeRedes(realizadoCobertura, "cobertura_numerica").RealizadoTotal

	var realizadoSortimento []RealizadoRede
	for _, c := range clientesFiltrados {
		if !c.SortimentoAplicavel {
			continue
		}
		realizadoSortimento = append(realizadoSortimento, RealizadoRede{Valor: c.SortimentoValor, TeveCompra: c.teveCompra})
	}
	resumoSortimento.RealizadoTotal = recalcularTotalDeRedes(realizadoSortimento, "sortimento_numerica_ppa").RealizadoTotal
}

// MetasPainelCombinadoNumericaHandler — GET /api/farol/metas-painel-combinado-numerica
//
//	?vinculo_cobertura_id=&vigencia_cobertura_id=&vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=
func MetasPainelCombinadoNumericaHandler(db *sql.DB) http.HandlerFunc {
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

		codGGV, codCRV, codRCA, negarEscopo := escopoHierarquiaMetas(spCtx)
		if negarEscopo {
			http.Error(w, "acesso negado: cadastro de organograma incompleto pra este usuário", http.StatusForbidden)
			return
		}
		codGGV, codCRV, codRCA = resolverFiltroDrillDown(q, codGGV, codCRV, codRCA)

		resp, err := calcularPainelCombinadoNumerica(db, spCtx.EmpresaID, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo, dataInicio, dataFim)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if codGGV != "" || codCRV != "" || codRCA != "" {
			filtrados := make([]PainelCombinadoNumericaCliente, 0, len(resp.Clientes))
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
				filtrados = append(filtrados, c)
			}
			resp.Clientes = filtrados
			recalcularResumoCombinadoNumericaParaEscopo(resp.Clientes, &resp.Cobertura, &resp.Sortimento)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// MetasPublicPainelCombinadoNumericaHandler — GET /api/farol/public/metas-painel-combinado-numerica
//
//	?cnpj=&scope=sup|rca|ggv&cod=&vinculo_cobertura_id=&vigencia_cobertura_id=&vinculo_sortimento_id=&vigencia_sortimento_id=&fluxo=
func MetasPublicPainelCombinadoNumericaHandler(db *sql.DB) http.HandlerFunc {
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
		if (scope != "sup" && scope != "rca" && scope != "ggv") || cod == "" || err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "scope (sup|rca|ggv), cod e os 4 ids de vínculo/vigência são obrigatórios"})
			return
		}
		fluxo := q.Get("fluxo")
		if fluxo == "" {
			fluxo = "faturado"
		}
		dataInicio := strings.TrimSpace(q.Get("data_inicio"))
		dataFim := strings.TrimSpace(q.Get("data_fim"))

		resp, err := calcularPainelCombinadoNumerica(db, empresaID, vinculoCoberturaID, vigenciaCoberturaID, vinculoSortimentoID, vigenciaSortimentoID, fluxo, dataInicio, dataFim)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		dentroDoEscopo := func(codGGV, codCRV, codRCA string) bool {
			switch scope {
			case "rca":
				return codRCA == cod
			case "ggv":
				return codGGV == cod
			default: // "sup"
				return codCRV == cod
			}
		}
		filtrados := make([]PainelCombinadoNumericaCliente, 0, len(resp.Clientes))
		for _, c := range resp.Clientes {
			if dentroDoEscopo(c.CodGGV, c.CodCRV, c.CodRCA) {
				filtrados = append(filtrados, c)
			}
		}
		resp.Clientes = filtrados
		recalcularResumoCombinadoNumericaParaEscopo(resp.Clientes, &resp.Cobertura, &resp.Sortimento)

		json.NewEncoder(w).Encode(resp)
	}
}
