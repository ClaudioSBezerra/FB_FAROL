package handlers

// farol_metas_carga.go — painel de status da "Carga do mês": pra cada vínculo
// (Indústria × tipo de métrica) mostra se a vigência do mês existe, suas
// faixas e o que já foi carregado (Clientes, Itens, Clientes Numérica, PPAs).
// Alimenta a tela única Configurações → Carga do mês, pensada pra 21
// indústrias (pedido do Claudio 09/10/2026: "menu o mais simples possível").
//
// GET /api/farol/metas-carga-status?mes=YYYY-MM   (sp_role >= gestor_geral)

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

type CargaFaixa struct {
	Faixa     int     `json:"faixa"`
	ValorMeta float64 `json:"valor_meta"`
}

type CargaVinculo struct {
	VinculoID       int          `json:"vinculo_id"`
	IndustriaID     int          `json:"industria_id"`
	IndustriaNome   string       `json:"industria_nome"`
	TipoMetricaNome string       `json:"tipo_metrica_nome"`
	FormulaCodigo   string       `json:"formula_codigo"`
	VigenciaID      *int         `json:"vigencia_id"`
	DataInicio      string       `json:"data_inicio,omitempty"`
	DataFim         string       `json:"data_fim,omitempty"`
	Status          string       `json:"status,omitempty"`
	Faixas          []CargaFaixa `json:"faixas"`
	Clientes        int          `json:"clientes"`
	Itens           int          `json:"itens"`
	PPAs            int          `json:"ppas"`
	ClientesNum     int          `json:"clientes_numerica"`
	CalculadoEm     *string      `json:"calculado_em"`
}

func MetasCargaStatusHandler(db *sql.DB) http.HandlerFunc {
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
		if !hasSpRole(spCtx.SpRole, "gestor_geral") {
			http.Error(w, "Forbidden: gestor_geral necessário", http.StatusForbidden)
			return
		}
		mes := r.URL.Query().Get("mes")
		ini, err := time.Parse("2006-01", mes)
		if err != nil {
			http.Error(w, "mes deve ser YYYY-MM", http.StatusBadRequest)
			return
		}
		inicio := ini.Format("2006-01-02")

		rows, err := db.Query(`
			SELECT mv.id, mv.industria_id, i.nome, tm.nome, tm.formula_codigo,
			       v.id, v.data_inicio::text, v.data_fim::text, v.status,
			       COALESCE((SELECT count(*) FROM farol.metas_clientes_validos c WHERE c.vigencia_id = v.id), 0),
			       COALESCE((SELECT count(*) FROM farol.metas_itens_validos t WHERE t.vigencia_id = v.id), 0),
			       COALESCE((SELECT count(*) FROM farol.metas_ppas p WHERE p.vigencia_id = v.id), 0),
			       COALESCE((SELECT count(*) FROM farol.metas_clientes_numericas n WHERE n.vigencia_id = v.id), 0),
			       (SELECT max(s.calculado_em)::text FROM farol.metas_realizados_snapshot s WHERE s.vigencia_id = v.id)
			FROM farol.metas_vinculos mv
			JOIN farol.industrias i ON i.id = mv.industria_id
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
			LEFT JOIN farol.metas_vigencias v ON v.vinculo_id = mv.id AND v.empresa_id = mv.empresa_id AND v.data_inicio = $2::date
			WHERE mv.empresa_id = $1 AND mv.ativo = true
			ORDER BY i.nome, tm.formula_codigo, mv.id`, spCtx.EmpresaID, inicio)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := []CargaVinculo{}
		for rows.Next() {
			var c CargaVinculo
			var vid sql.NullInt64
			var di, df, st, calc sql.NullString
			if err := rows.Scan(&c.VinculoID, &c.IndustriaID, &c.IndustriaNome, &c.TipoMetricaNome, &c.FormulaCodigo,
				&vid, &di, &df, &st, &c.Clientes, &c.Itens, &c.PPAs, &c.ClientesNum, &calc); err != nil {
				http.Error(w, "Database error", http.StatusInternalServerError)
				return
			}
			c.Faixas = []CargaFaixa{}
			if vid.Valid {
				v := int(vid.Int64)
				c.VigenciaID = &v
				c.DataInicio, c.DataFim, c.Status = di.String, df.String, st.String
			}
			if calc.Valid {
				s := calc.String
				c.CalculadoEm = &s
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		// Faixas das vigências encontradas (1 consulta só).
		frows, err := db.Query(`
			SELECT f.vigencia_id, f.faixa, f.valor_meta FROM farol.metas_faixas f
			JOIN farol.metas_vigencias v ON v.id = f.vigencia_id
			WHERE v.empresa_id = $1 AND v.data_inicio = $2::date ORDER BY f.vigencia_id, f.faixa`, spCtx.EmpresaID, inicio)
		if err == nil {
			defer frows.Close()
			porVig := map[int][]CargaFaixa{}
			for frows.Next() {
				var vid int
				var f CargaFaixa
				if frows.Scan(&vid, &f.Faixa, &f.ValorMeta) == nil {
					porVig[vid] = append(porVig[vid], f)
				}
			}
			for i := range out {
				if out[i].VigenciaID != nil {
					out[i].Faixas = porVig[*out[i].VigenciaID]
					if out[i].Faixas == nil {
						out[i].Faixas = []CargaFaixa{}
					}
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}
