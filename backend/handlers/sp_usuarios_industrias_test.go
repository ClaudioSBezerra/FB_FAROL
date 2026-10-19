package handlers

import "testing"

// Indústria é cadastro por empresa: ids de outra empresa (ex.: MASTER) viram os
// da empresa do usuário (JC), pelo nome.
func TestIndustriasDaEmpresa_TraduzPorNome(t *testing.T) {
	db, empresaID := biTestDB(t)
	var groupID string
	if err := db.QueryRow(`SELECT group_id::text FROM companies WHERE id = $1`, empresaID).Scan(&groupID); err != nil {
		t.Fatalf("grupo da empresa de teste: %v", err)
	}
	var outraEmpresa string
	if err := db.QueryRow(`INSERT INTO companies (group_id, name) VALUES ($1, 'TSEG OUTRA EMPRESA') RETURNING id::text`, groupID).Scan(&outraEmpresa); err != nil {
		t.Fatalf("criar empresa: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM companies WHERE id = $1::uuid`, outraEmpresa) })

	nome := "TSEG INDUSTRIA X"
	idMinha := criarIndustriaFixture(t, db, empresaID, nome)
	var idOutra int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1::uuid, $2) RETURNING id`, outraEmpresa, nome).Scan(&idOutra); err != nil {
		t.Fatalf("indústria da outra empresa: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM farol.industrias WHERE id = $1 OR id = $2`, idOutra, idMinha) })

	got, err := industriasDaEmpresa(db, empresaID, []int{idOutra, idMinha})
	if err != nil || len(got) != 1 || got[0] != int64(idMinha) {
		t.Errorf("traduzido = %v (%v), quero só %d (a da empresa do usuário, sem duplicar)", got, err, idMinha)
	}
	if _, err := industriasDaEmpresa(db, empresaID, []int{99999999}); err == nil {
		t.Errorf("id inexistente deveria dar erro")
	}
}
