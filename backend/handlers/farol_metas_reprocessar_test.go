package handlers

import "testing"

// Importar uma lista tem que apagar o resultado salvo da vigência (senão o
// painel só mudava no prewarm da madrugada seguinte).
func TestInvalidarSnapshotsVigencia_ApagaSoDaVigencia(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TREPROC Invalida", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigA := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-09-01", "2026-09-30")
	vigB := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-10-01", "2026-10-31")

	vazio := &RealizadoResultado{}
	for _, v := range []int{vigA, vigB} {
		for _, fluxo := range []string{"faturado", "transmitido"} {
			if err := salvarSnapshot(db, empresaID, vinculoID, v, fluxo, "rede", "", vazio, "snapshot_diario"); err != nil {
				t.Fatalf("salvarSnapshot: %v", err)
			}
		}
	}
	if err := invalidarSnapshotsVigencia(db, empresaID, vigA); err != nil {
		t.Fatalf("invalidar: %v", err)
	}
	contar := func(v int) (n int) {
		_ = db.QueryRow(`SELECT count(*) FROM farol.metas_realizados_snapshot WHERE vigencia_id = $1`, v).Scan(&n)
		return
	}
	if n := contar(vigA); n != 0 {
		t.Errorf("vigência invalidada ainda tem %d snapshot(s)", n)
	}
	if n := contar(vigB); n != 2 {
		t.Errorf("a outra vigência perdeu snapshot: %d, esperado 2", n)
	}
}
