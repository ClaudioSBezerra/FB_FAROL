package handlers

// farol_acesso.go — "quais painéis e indústrias este usuário vê" (19/10/2026).
//
// Painéis = users.modulos: vendas | unilever | reckitt | relatorios | bi.
// Indústrias = users.industrias_permitidas (NULL = todas). admin_fbtax
// (inclui o admin de plataforma) vê tudo. O Painel Reckitt nunca é liberado
// por padrão: só com o módulo 'reckitt' explícito.

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/lib/pq"
)

// formulasSegmentoFoco — Tipos de Métrica do programa Reckitt.
const formulaSegmentoFoco = "positivacao_grupos"

func (c *FarolContext) temModulo(m string) bool {
	mods := c.Modulos
	if len(mods) == 0 { // contexto sem módulos (testes/legado) = painéis padrão
		mods = []string{"vendas", "unilever", "relatorios"}
	}
	for _, x := range mods {
		if x == m {
			return true
		}
	}
	return false
}

// PodeVerPainel — o usuário tem o painel/módulo `m` (admin_fbtax sempre tem).
func (c *FarolContext) PodeVerPainel(m string) bool {
	return c != nil && (c.IsAdminFbtax() || c.temModulo(m))
}

// PodeVerIndustria — lista nula = todas as indústrias.
func (c *FarolContext) PodeVerIndustria(id int) bool {
	if c == nil {
		return false
	}
	if c.IsAdminFbtax() || c.IndustriasPermitidas == nil {
		return true
	}
	for _, x := range c.IndustriasPermitidas {
		if x == id {
			return true
		}
	}
	return false
}

// FormulasBloqueadas — fórmulas dos vínculos que o usuário NÃO pode ver:
// Unilever (Ponderada/Numérica) sem o painel 'unilever'; Segmento Foco sem o
// painel 'reckitt'. Fórmulas de outras indústrias (ainda sem painel próprio)
// seguem visíveis — a restrição é por painel conhecido.
func (c *FarolContext) FormulasBloqueadas() []string {
	out := []string{}
	if !c.PodeVerPainel("unilever") {
		out = append(out, "cobertura_rede", "sortimento_rede", "cobertura_numerica", "sortimento_numerica_ppa")
	}
	if !c.PodeVerPainel("reckitt") {
		out = append(out, formulaSegmentoFoco)
	}
	return out
}

// industriasFiltro — argumento SQL ($n::int[]) com as indústrias permitidas; vazio = todas.
func (c *FarolContext) industriasFiltro() any {
	if c.IsAdminFbtax() || c.IndustriasPermitidas == nil {
		return nil
	}
	ids := make([]int64, len(c.IndustriasPermitidas))
	for i, v := range c.IndustriasPermitidas {
		ids[i] = int64(v)
	}
	return pq.Array(ids)
}

// ExigeModulo recusa (403) quem não tem o painel/módulo. Usar DENTRO de FarolAuthMiddleware.
func ExigeModulo(modulo string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !GetSpContext(r).PodeVerPainel(modulo) {
			http.Error(w, "Forbidden: painel não liberado pra este usuário", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// ExigeVinculoPermitido checa os vínculos citados na query (vinculo_id,
// vinculo_cobertura_id, vinculo_sortimento_id): a fórmula precisa do painel certo
// e a indústria precisa estar na lista do usuário.
func ExigeVinculoPermitido(db *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil || spCtx.IsAdminFbtax() {
			next(w, r)
			return
		}
		q := r.URL.Query()
		for _, k := range []string{"vinculo_id", "vinculo_cobertura_id", "vinculo_sortimento_id"} {
			v := q.Get(k)
			if v == "" {
				continue
			}
			id, err := strconv.Atoi(v)
			if err != nil {
				continue
			}
			var industriaID int
			var formula string
			if err := db.QueryRow(`SELECT mv.industria_id, tm.formula_codigo FROM farol.metas_vinculos mv
				JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id WHERE mv.id = $1 AND mv.empresa_id = $2`, id, spCtx.EmpresaID).Scan(&industriaID, &formula); err != nil {
				continue // vínculo inexistente: o handler responde o erro de sempre
			}
			painel := "unilever"
			if formula == formulaSegmentoFoco {
				painel = "reckitt"
			}
			if !spCtx.PodeVerPainel(painel) || !spCtx.PodeVerIndustria(industriaID) {
				http.Error(w, "Forbidden: indústria/painel não liberado pra este usuário", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}
