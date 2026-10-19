package handlers

// farol_acesso_publico.go — restrição por indústria do link público (mobile)
// de um GGV. Tabela farol.acesso_publico_ggv (migration 256). Sem linha =
// sem restrição.

import (
	"database/sql"
	"strings"

	"github.com/lib/pq"
)

// industriasPublicasGGV devolve os ids de indústria liberados ao link público
// do GGV, ou nil quando não há restrição configurada.
func industriasPublicasGGV(db *sql.DB, empresaID, codGGV string) []int64 {
	var ids pq.Int64Array
	err := db.QueryRow(`SELECT industrias FROM farol.acesso_publico_ggv WHERE empresa_id = $1 AND cod_ggv = $2`,
		empresaID, strings.TrimSpace(codGGV)).Scan(&ids)
	if err != nil {
		return nil
	}
	if ids == nil {
		ids = pq.Int64Array{}
	}
	return []int64(ids)
}

// fornecedoresDasIndustrias lista os cod_fornec mapeados às indústrias dadas.
func fornecedoresDasIndustrias(db *sql.DB, empresaID string, ids []int64) []string {
	out := []string{}
	rows, err := db.Query(`SELECT DISTINCT cod_fornec FROM farol.industria_fornecedores
		WHERE empresa_id = $1 AND industria_id = ANY($2)`, empresaID, pq.Array(ids))
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil {
			out = append(out, c)
		}
	}
	return out
}

// restringirFiltroFornecGGV aplica em filters["cod_fornec"] a interseção com
// os fornecedores das indústrias liberadas ao GGV (se houver restrição).
func restringirFiltroFornecGGV(db *sql.DB, empresaID, codGGV string, filters multiFilters) {
	ids := industriasPublicasGGV(db, empresaID, codGGV)
	if ids == nil {
		return
	}
	permitidos := map[string]bool{}
	for _, c := range fornecedoresDasIndustrias(db, empresaID, ids) {
		permitidos[c] = true
	}
	var novo []string
	if atual, ok := filters["cod_fornec"]; ok {
		for _, c := range atual {
			if permitidos[c] {
				novo = append(novo, c)
			}
		}
	} else {
		for c := range permitidos {
			novo = append(novo, c)
		}
	}
	if len(novo) == 0 {
		novo = []string{"__nenhum__"}
	}
	filters["cod_fornec"] = novo
}
