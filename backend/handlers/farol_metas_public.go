package handlers

// farol_metas_public.go — Painel Mobile de Metas por Indústria (Épico 6,
// Story 6.1, módulo Painel de Gestão de Metas por Indústria)
//
// Reaproveita EXATAMENTE o padrão já existente do painel público do Farol
// (FarolV2PublicCardsHandler, farol_v2_api.go): resolve empresa por CNPJ
// (resolveEmpresaCNPJ, sem alterar), sem exigir login — mesmo link
// /m/CNPJ/SUP|RCA/cod que o app ION VENDAS já usa. Escopo aqui é sempre
// restrito ao Supervisor/RCA da URL — nunca a empresa toda (diferente do
// painel admin, Épico 5, que mostra todas as Redes do vínculo).

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// MetasPublicVinculosHandler — GET /api/farol/public/metas-vinculos?cnpj=
// Lista os vínculos (Indústria × Tipo de Métrica) da empresa — sem dado
// sensível de configuração, só o necessário pra escolher qual programa ver.
func MetasPublicVinculosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		empresaID := resolveEmpresaCNPJ(db, r.URL.Query().Get("cnpj"))
		if empresaID == "" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "empresa não encontrada para este CNPJ"})
			return
		}
		rows, err := db.Query(`
			SELECT mv.id, i.nome, tm.nome
			FROM farol.metas_vinculos mv
			JOIN farol.industrias i ON i.id = mv.industria_id
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
			WHERE mv.empresa_id = $1 AND mv.ativo = TRUE
			ORDER BY i.nome
		`, empresaID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type item struct {
			ID              int    `json:"id"`
			IndustriaNome   string `json:"industria_nome"`
			TipoMetricaNome string `json:"tipo_metrica_nome"`
		}
		lista := []item{}
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.ID, &it.IndustriaNome, &it.TipoMetricaNome); err == nil {
				lista = append(lista, it)
			}
		}
		json.NewEncoder(w).Encode(lista)
	}
}

// MetasPublicVigenciasHandler — GET /api/farol/public/metas-vigencias?cnpj=&vinculo_id=
func MetasPublicVigenciasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		empresaID := resolveEmpresaCNPJ(db, r.URL.Query().Get("cnpj"))
		vinculoID, err := strconv.Atoi(r.URL.Query().Get("vinculo_id"))
		if empresaID == "" || err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "cnpj e vinculo_id são obrigatórios"})
			return
		}
		rows, err := db.Query(`
			SELECT id, data_inicio::text, data_fim::text, status
			FROM farol.metas_vigencias
			WHERE vinculo_id = $1 AND empresa_id = $2
			ORDER BY data_inicio DESC
		`, vinculoID, empresaID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		lista := []PainelVigencia{}
		for rows.Next() {
			var v PainelVigencia
			if err := rows.Scan(&v.ID, &v.DataInicio, &v.DataFim, &v.Status); err == nil {
				lista = append(lista, v)
			}
		}
		json.NewEncoder(w).Encode(lista)
	}
}

// MetasPublicPainelHandler — GET /api/farol/public/metas-painel
//
//	?cnpj=&scope=sup|rca&cod=&vinculo_id=&vigencia_id=&fluxo=&recortes=1
//
// Mesmo shape de resposta do painel admin (farol_metas_painel.go), mas
// SEMPRE recortado pro Supervisor/RCA da URL — nunca mostra a empresa
// inteira. Nível de agregação é implícito: scope=rca → mostra as Redes
// daquele RCA; scope=sup → mostra o rollup por RCA dentro daquele
// Supervisor (mesmo "nível=rca" do painel admin, só filtrado).
func MetasPublicPainelHandler(db *sql.DB) http.HandlerFunc {
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
		vinculoID, err1 := strconv.Atoi(q.Get("vinculo_id"))
		vigenciaID, err2 := strconv.Atoi(q.Get("vigencia_id"))
		if (scope != "sup" && scope != "rca") || cod == "" || err1 != nil || err2 != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "scope (sup|rca), cod, vinculo_id e vigencia_id são obrigatórios"})
			return
		}
		fluxo := q.Get("fluxo")
		if fluxo == "" {
			fluxo = "faturado"
		}

		var industriaNome, tipoMetricaNome, formulaCodigo string
		var vig PainelVigencia
		vig.ID = vigenciaID
		err := db.QueryRow(`
			SELECT i.nome, tm.nome, tm.formula_codigo, v.data_inicio::text, v.data_fim::text, v.status
			FROM farol.metas_vigencias v
			JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
			JOIN farol.industrias i ON i.id = mv.industria_id
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
			WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3
		`, vigenciaID, vinculoID, empresaID).Scan(&industriaNome, &tipoMetricaNome, &formulaCodigo, &vig.DataInicio, &vig.DataFim, &vig.Status)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "vínculo/vigência não encontrado"})
			return
		}

		// Realizado ao nível de Rede (grão atômico), depois filtra pro
		// escopo pedido — nunca expõe Redes de fora do Supervisor/RCA da URL.
		realizadoEscopo, err := calcularRealizadoEscopoPublico(db, empresaID, vinculoID, vigenciaID, fluxo, scope, cod, formulaCodigo, vig.DataInicio, vig.DataFim, "", "")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		rows, err := db.Query(`SELECT faixa, valor_meta FROM farol.metas_faixas WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var faixas []PainelFaixa
		for rows.Next() {
			var f PainelFaixa
			if err := rows.Scan(&f.Faixa, &f.ValorMeta); err == nil {
				f.Atingida = realizadoEscopo.RealizadoTotal >= f.ValorMeta
				faixas = append(faixas, f)
			}
		}
		sort.Slice(faixas, func(i, j int) bool { return faixas[i].ValorMeta < faixas[j].ValorMeta })

		resp := PainelResponse{
			VinculoID: vinculoID, IndustriaNome: industriaNome, TipoMetricaNome: tipoMetricaNome,
			Vigencia: vig, Realizado: realizadoEscopo, Faixas: faixas,
		}
		for i := range faixas {
			if faixas[i].Atingida {
				f := faixas[i]
				resp.FaixaAtual = &f
			} else if resp.ProximaFaixa == nil {
				f := faixas[i]
				resp.ProximaFaixa = &f
			}
		}
		if resp.ProximaFaixa != nil {
			resp.Delta = resp.ProximaFaixa.ValorMeta - realizadoEscopo.RealizadoTotal
		}

		if q.Get("recortes") == "1" {
			resp.Recortes = map[string]*RealizadoResultado{}
			for _, rec := range []string{"dia_anterior", "semana", "mes", "ano_corrente"} {
				di, df, rerr := calcularRecorteDatas(rec)
				if rerr != nil {
					continue
				}
				rr, rerr2 := calcularRealizadoEscopoPublico(db, empresaID, vinculoID, vigenciaID, fluxo, scope, cod, formulaCodigo, vig.DataInicio, vig.DataFim, di, df)
				if rerr2 == nil {
					resp.Recortes[rec] = rr
				}
			}
		}

		json.NewEncoder(w).Encode(resp)
	}
}

// calcularRealizadoEscopoPublico calcula o Realizado (ao vivo via
// CalcularRealizadoComPeriodo — recortes de tempo nunca passam pelo
// congelamento, mesma regra da Story 5.3) e já filtra pro escopo
// Supervisor/RCA. dataInicioOverride/dataFimOverride vazios = período
// inteiro da vigência (uso normal); preenchidos = recorte (Story 6.2).
func calcularRealizadoEscopoPublico(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, scope, cod, formulaCodigo, dataInicioVigencia, dataFimVigencia, dataInicioOverride, dataFimOverride string) (*RealizadoResultado, error) {
	var realizadoCompleto *RealizadoResultado
	var err error
	if dataInicioOverride == "" {
		realizadoCompleto, err = obterOuCongelarRealizado(db, empresaID, vinculoID, vigenciaID, fluxo, "rede")
	} else {
		realizadoCompleto, err = CalcularRealizadoComPeriodo(db, empresaID, vinculoID, vigenciaID, fluxo, "rede", dataInicioOverride, dataFimOverride)
	}
	if err != nil {
		return nil, err
	}
	redesEscopo, err := filtrarRedesPorEscopo(db, empresaID, realizadoCompleto.Redes, scope, cod)
	if err != nil {
		return nil, err
	}
	realizadoEscopo := recalcularTotalDeRedes(redesEscopo, formulaCodigo)
	realizadoEscopo.VinculoID = vinculoID
	realizadoEscopo.VigenciaID = vigenciaID
	realizadoEscopo.Fluxo = fluxo
	realizadoEscopo.Nivel = "rede"
	realizadoEscopo.Parcial = realizadoCompleto.Parcial
	realizadoEscopo.Projecao = projetarFechamento(realizadoEscopo.RealizadoTotal, dataInicioVigencia, dataFimVigencia)
	return realizadoEscopo, nil
}

// filtrarRedesPorEscopo restringe a lista de Redes ao Supervisor/RCA
// pedido — nunca expõe dado de fora daquele recorte organizacional
// (mesma preocupação de segurança do painel público de vendas existente,
// que já restringe por escopo via baseDrill fixo na URL).
func filtrarRedesPorEscopo(db *sql.DB, empresaID string, redes []RealizadoRede, scope, cod string) ([]RealizadoRede, error) {
	if scope == "rca" {
		var out []RealizadoRede
		for _, r := range redes {
			if r.CodRCA == cod {
				out = append(out, r)
			}
		}
		return out, nil
	}
	// scope == "sup": inclui toda Rede cujo RCA representante esteja sob
	// este Supervisor.
	cacheSupervisorPorRCA := map[string]string{}
	var out []RealizadoRede
	for _, r := range redes {
		if r.CodRCA == "" {
			continue
		}
		sup, ok := cacheSupervisorPorRCA[r.CodRCA]
		if !ok {
			_, resolvido, _, _, _, err := resolverHierarquiaRCA(db, empresaID, r.CodRCA)
			if err != nil {
				return nil, err
			}
			sup = resolvido
			cacheSupervisorPorRCA[r.CodRCA] = sup
		}
		if sup == cod {
			out = append(out, r)
		}
	}
	return out, nil
}

// recalcularTotalDeRedes reconstrói RealizadoTotal a partir de uma lista
// (já filtrada) de Redes — mesma lógica de agregação usada em
// CalcularRealizadoComPeriodo, extraída aqui pra reuso sem duplicar regra.
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
