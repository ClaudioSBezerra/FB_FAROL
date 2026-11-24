package handlers

import "testing"

func TestSegPublicoEscopo(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TSEGP Reckitt", "positivacao_grupos", "cliente", nil, nil)
	t.Cleanup(cleanup)
	for _, h := range [][]string{{"PR1", "3", "91"}, {"PR2", "3", "92"}, {"PR3", "347", "524"}, {"PR4", "2", "241"}} {
		db.Exec(`INSERT INTO farol.metas_rca_hierarquia (empresa_id, vinculo_id, cod_rca, cod_ggv, cod_crv, uf) VALUES ($1,$2,$3,$4,$5,$6)`,
			empresaID, vinculoID, h[0], h[1], h[2], segUFPorGGVHierarquia[h[1]])
	}
	db.Exec(`UPDATE farol.metas_vinculos SET equipes_ocultas = '{92}' WHERE id = $1`, vinculoID)

	casos := []struct {
		scope, cod string
		ok         bool
		quero      segEscopo
	}{
		{"sup", "91", true, segEscopo{CodCRV: "91"}},
		{"sup", "92", false, segEscopo{}},  // equipe fora do painel
		{"sup", "200", false, segEscopo{}}, // equipe fora da planilha
		{"rca", "PR1", true, segEscopo{CodRCA: "PR1"}},
		{"rca", "PR2", false, segEscopo{}},  // RCA de equipe oculta
		{"rca", "XXXX", false, segEscopo{}}, // RCA fora da planilha
		{"ggv", "3", true, segEscopo{CodGGV: "3"}},
		{"ggv", "347", true, segEscopo{CodGGV: "347"}},
		{"ggv", "345", true, segEscopo{CodGGV: "2"}}, // gerente 345 da venda em TO = GGV 2 da planilha
		{"ggv", "99", false, segEscopo{}},
		{"xxx", "91", false, segEscopo{}},
		{"sup", "", false, segEscopo{}},
	}
	for _, c := range casos {
		got, ok := segPublicoEscopo(db, empresaID, vinculoID, c.scope, c.cod)
		if ok != c.ok || got != c.quero {
			t.Errorf("%s/%s: got (%+v, %v), quero (%+v, %v)", c.scope, c.cod, got, ok, c.quero, c.ok)
		}
	}
	// vínculo sem hierarquia carregada: ninguém entra.
	if _, ok := segPublicoEscopo(db, empresaID, vinculoID+999999, "sup", "91"); ok {
		t.Errorf("vínculo sem hierarquia não deveria liberar ninguém")
	}
}

func TestSegEscopoRespeita(t *testing.T) {
	sc := segEscopo{CodCRV: "91"}
	f := segEscopo{CodRCA: "PR1"} // estreitar dentro do escopo: ok, e o escopo é imposto
	if !segEscopoRespeita(&f, sc) || f.CodCRV != "91" || f.CodRCA != "PR1" {
		t.Errorf("estreitar: %+v", f)
	}
	f = segEscopo{CodCRV: "91"}
	if !segEscopoRespeita(&f, sc) {
		t.Errorf("repetir o escopo deveria passar")
	}
	f = segEscopo{CodCRV: "92"} // ampliar/trocar: recusado
	if segEscopoRespeita(&f, sc) {
		t.Errorf("trocar a equipe pelo filtro deveria ser recusado")
	}
	f = segEscopo{CodRCA: "PR9"}
	if !segEscopoRespeita(&f, segEscopo{CodRCA: "PR9"}) || f.CodRCA != "PR9" {
		t.Errorf("escopo de RCA: %+v", f)
	}
	f = segEscopo{CodRCA: "PR1"}
	if segEscopoRespeita(&f, segEscopo{CodRCA: "PR9"}) {
		t.Errorf("RCA diferente do escopo deveria ser recusado")
	}
}
