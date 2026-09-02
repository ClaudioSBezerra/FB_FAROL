package handlers

// farol_metas_painel.go — Painel de indicadores oficiais: Meta × Realizado
// × delta (Épico 5, Story 5.1, módulo Painel de Gestão de Metas por
// Indústria)
//
// Combina o Realizado (Épico 4, já congela/recalcula sozinho) com as
// Faixas de meta (Story 2.2) e calcula o delta explícito (FR19a) — "quanto
// falta pra bater a meta", não só os dois números lado a lado.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
)

type PainelFaixa struct {
	Faixa     int     `json:"faixa"`
	ValorMeta float64 `json:"valor_meta"`
	Atingida  bool    `json:"atingida"`
}

type PainelVigencia struct {
	ID         int    `json:"id"`
	DataInicio string `json:"data_inicio"`
	DataFim    string `json:"data_fim"`
	Status     string `json:"status"`
}

type PainelResponse struct {
	VinculoID       int                 `json:"vinculo_id"`
	IndustriaNome   string              `json:"industria_nome"`
	TipoMetricaNome string              `json:"tipo_metrica_nome"`
	Vigencia        PainelVigencia      `json:"vigencia"`
	Realizado       *RealizadoResultado `json:"realizado"`
	Faixas          []PainelFaixa       `json:"faixas"`
	FaixaAtual      *PainelFaixa        `json:"faixa_atual"`   // maior faixa já atingida (nil se nenhuma)
	ProximaFaixa    *PainelFaixa        `json:"proxima_faixa"` // menor faixa ainda não atingida (nil se já bateu todas)
	Delta           float64             `json:"delta"`         // quanto falta pra bater a próxima faixa (FR19a) — 0 se já bateu tudo
}

// MetasPainelHandler — GET /api/farol/metas-painel?vinculo_id=&vigencia_id=&fluxo=&nivel=
func MetasPainelHandler(db *sql.DB) http.HandlerFunc {
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

		var industriaNome, tipoMetricaNome string
		var vig PainelVigencia
		vig.ID = vigenciaID
		err := db.QueryRow(`
			SELECT i.nome, tm.nome, v.data_inicio::text, v.data_fim::text, v.status
			FROM farol.metas_vigencias v
			JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
			JOIN farol.industrias i ON i.id = mv.industria_id
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
			WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3
		`, vigenciaID, vinculoID, spCtx.EmpresaID).Scan(&industriaNome, &tipoMetricaNome, &vig.DataInicio, &vig.DataFim, &vig.Status)
		if err != nil {
			http.Error(w, "Vínculo/vigência não encontrado", http.StatusNotFound)
			return
		}

		realizado, err := obterOuCongelarRealizado(db, spCtx.EmpresaID, vinculoID, vigenciaID, fluxo, nivel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rows, err := db.Query(`SELECT faixa, valor_meta FROM farol.metas_faixas WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var faixas []PainelFaixa
		for rows.Next() {
			var f PainelFaixa
			if err := rows.Scan(&f.Faixa, &f.ValorMeta); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			f.Atingida = realizado.RealizadoTotal >= f.ValorMeta
			faixas = append(faixas, f)
		}
		sort.Slice(faixas, func(i, j int) bool { return faixas[i].ValorMeta < faixas[j].ValorMeta })

		resp := PainelResponse{
			VinculoID: vinculoID, IndustriaNome: industriaNome, TipoMetricaNome: tipoMetricaNome,
			Vigencia: vig, Realizado: realizado, Faixas: faixas,
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
			resp.Delta = resp.ProximaFaixa.ValorMeta - realizado.RealizadoTotal
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
