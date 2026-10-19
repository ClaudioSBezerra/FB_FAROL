package handlers

import "testing"

// TestSegmentoFoco_PositivacaoPorGrupo cobre as regras do descritivo da
// Reckitt: mínimo de 3 unidades por produto (caixa conta pelas unidades),
// bonificação positivando com R$ 0, tipo de venda fora de {1,5,9} ignorado,
// devolução abatida, UF da matriz, objetivo por UF e os 3 fluxos.
func TestSegmentoFoco_PositivacaoPorGrupo(t *testing.T) {
	db, empresaID := biTestDB(t)
	forn := "TSEG47753"

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TSEG Reckitt", "positivacao_grupos", "cliente", nil, nil)
	t.Cleanup(cleanup)
	var industriaID int
	if err := db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID); err != nil {
		t.Fatalf("industria do vínculo: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1,$2,$3)`, empresaID, industriaID, forn); err != nil {
		t.Fatalf("vincular fornecedor: %v", err)
	}
	db.Exec(`UPDATE farol.metas_vinculos SET tipos_venda_validos = '{1,5,9}' WHERE id = $1`, vinculoID)
	vig := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_fornec = $2`, empresaID, forn)
		db.Exec(`DELETE FROM vendas_transmitidas WHERE empresa_id = $1 AND cod_fornec = $2`, empresaID, forn)
		db.Exec(`DELETE FROM vendas_ccd WHERE empresa_id = $1 AND cod_fornec = $2`, empresaID, forn)
		db.Exec(`DELETE FROM farol.industria_fornecedores WHERE industria_id = $1`, industriaID)
	})

	for _, p := range [][]string{{"TSEGA", "GRUPO UM"}, {"TSEGB", "GRUPO UM"}, {"TSEGC", "GRUPO DOIS"}} {
		if _, err := db.Exec(`INSERT INTO farol.metas_grupos_produtos (empresa_id, vinculo_id, vigencia_id, grupo, cod_prod, produto) VALUES ($1,$2,$3,$4,$5,$5)`,
			empresaID, vinculoID, vig, p[1], p[0]); err != nil {
			t.Fatalf("grupo: %v", err)
		}
	}
	db.Exec(`INSERT INTO farol.metas_grupos_objetivos (empresa_id, vinculo_id, vigencia_id, uf, grupo, objetivo) VALUES ($1,$2,$3,'GO','GRUPO UM',10),($1,$2,$3,'GO','GRUPO DOIS',5)`, empresaID, vinculoID, vig)

	venda := func(cli, princ, uf, prod, emb string, unitCx, qt float64, tipo, data string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_fornec, cod_cli, cod_cliprinc, uf, cod_prod, embalagem, qt_unit_cx, qt, pvenda, tipo_venda, cod_gerente, cod_supervisor, cod_rca)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,0,$11,'3','91','4132')`, empresaID, data, forn, cli, princ, uf, prod, emb, unitCx, qt, tipo); err != nil {
			t.Fatalf("venda: %v", err)
		}
	}
	// C1: 2 + 1 un. do mesmo produto em dias diferentes = 3 → positiva.
	venda("TC1", "TC1", "GO", "TSEGA", "UN/0001/UN", 1, 2, "1", "2026-07-03")
	venda("TC1", "TC1", "GO", "TSEGA", "UN/0001/UN", 1, 1, "1", "2026-07-20")
	// C2: 1 caixa (12 un.) → positiva.
	venda("TC2", "TC2", "GO", "TSEGB", "CX/0012/UN", 12, 1, "1", "2026-07-05")
	// C3: bonificação (5) com valor 0 → positiva o GRUPO DOIS.
	venda("TC3", "TC3", "GO", "TSEGC", "UN/0001/UN", 1, 5, "5", "2026-07-06")
	// C4: tipo 10 (transferência) não conta.
	venda("TC4", "TC4", "GO", "TSEGC", "UN/0001/UN", 1, 5, "10", "2026-07-06")
	// C5: loja em MS cuja matriz (C1) é de GO → entra em GO.
	venda("TC5", "TC1", "MS", "TSEGA", "UN/0001/UN", 1, 3, "1", "2026-07-07")
	// C6: 2 caixas devolvidas por completo → não positiva.
	venda("TC6", "TC6", "GO", "TSEGB", "CX/0012/UN", 12, 2, "1", "2026-07-08")
	db.Exec(`INSERT INTO vendas_ccd (empresa_id, data_evento, evento, cod_fornec, cod_cli, cod_prod, qt) VALUES ($1,'2026-07-09','DEVOLVIDO',$2,'TC6','TSEGB',2)`, empresaID, forn)
	// C8: só 2 unidades → não positiva.
	venda("TC8", "TC8", "GO", "TSEGA", "UN/0001/UN", 1, 2, "1", "2026-07-10")
	// C7: só no transmitido.
	if _, err := db.Exec(`INSERT INTO vendas_transmitidas (empresa_id, data_transmissao, cod_fornec, cod_cli, cod_cliprinc, uf, cod_prod, embalagem, qt_unit_cx, qt, pvenda, tipo_venda)
		VALUES ($1,'2026-07-11',$2,'TC7','TC7','GO','TSEGA','UN/0001/UN',1,3,0,'1')`, empresaID, forn); err != nil {
		t.Fatalf("transmitida: %v", err)
	}
	segInvalidarCache()

	realizado := func(fluxo, grupo string) (int, *float64) {
		t.Helper()
		p, err := segPainel(db, empresaID, vinculoID, "2026-07", fluxo, segEscopo{})
		if err != nil {
			t.Fatalf("segPainel(%s): %v", fluxo, err)
		}
		for _, u := range p.UFs {
			if u.UF != "GO" {
				continue
			}
			for _, g := range u.Grupos {
				if g.Grupo == grupo {
					return g.Meses[0].Realizado, g.Meses[0].Objetivo
				}
			}
		}
		t.Fatalf("UF GO / %s não encontrado no painel (%s)", grupo, fluxo)
		return 0, nil
	}

	if r, obj := realizado("faturado", "GRUPO UM"); r != 3 || obj == nil || *obj != 10 {
		t.Errorf("faturado GRUPO UM: realizado=%d obj=%v, quero 3 (C1, C2, C5-matriz GO) e objetivo 10", r, obj)
	}
	if r, _ := realizado("faturado", "GRUPO DOIS"); r != 1 {
		t.Errorf("faturado GRUPO DOIS: %d, quero 1 (C3 bonificação; C4 tipo 10 fora)", r)
	}
	if r, _ := realizado("transmitido", "GRUPO UM"); r != 1 {
		t.Errorf("transmitido GRUPO UM: %d, quero 1 (só C7)", r)
	}
	if r, _ := realizado("soma", "GRUPO UM"); r != 4 {
		t.Errorf("soma GRUPO UM: %d, quero 4 (faturado 3 + transmitido 1)", r)
	}

	// Drill: GGV → CRV → RCA → cliente → produto.
	d, err := segDetalhe(db, empresaID, vinculoID, "2026-07", "mes", "faturado", segEscopo{})
	if err != nil || d.Nivel != "ggv" || len(d.Linhas) != 1 || d.Linhas[0].Total != 4 {
		t.Errorf("detalhe nível GGV: %+v err=%v, quero 1 GGV com total 4 (3 do grupo um + 1 do dois)", d, err)
	}
	d, err = segDetalhe(db, empresaID, vinculoID, "2026-07", "mes", "faturado", segEscopo{CodGGV: "3", CodCRV: "91", CodRCA: "4132", CodCli: "TC1"})
	if err != nil || d.Nivel != "produto" || len(d.Produtos) != 1 || d.Produtos[0].Qtd != 3 || !d.Produtos[0].Positivou {
		t.Errorf("detalhe produto de TC1: %+v err=%v, quero TSEGA qtd 3 positivou", d, err)
	}
}

func TestSegMesesDoTrimestre(t *testing.T) {
	m, err := segMesesDoTrimestre("2026-08")
	if err != nil || m[0] != "2026-07" || m[2] != "2026-09" {
		t.Errorf("tri de agosto = %v (%v), quero jul-set", m, err)
	}
	m, _ = segMesesDoTrimestre("2026-10")
	if m[0] != "2026-10" || m[2] != "2026-12" {
		t.Errorf("tri de outubro = %v, quero out-dez", m)
	}
}

// UF oculta (flag "UF liberada?"): o Pará fica fora do painel — realizado e
// objetivo — até ser liberado.
func TestSegmentoFoco_UFOcultaPA(t *testing.T) {
	db, empresaID := biTestDB(t)
	forn := "TSEGPA"
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TSEGPA Reckitt", "positivacao_grupos", "cliente", nil, nil)
	t.Cleanup(cleanup)
	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1,$2,$3)`, empresaID, industriaID, forn)
	db.Exec(`UPDATE farol.metas_vinculos SET tipos_venda_validos = '{1,5,9}' WHERE id = $1`, vinculoID)
	vig := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_fornec = $2`, empresaID, forn)
		db.Exec(`DELETE FROM farol.industria_fornecedores WHERE industria_id = $1`, industriaID)
	})
	db.Exec(`INSERT INTO farol.metas_grupos_produtos (empresa_id, vinculo_id, vigencia_id, grupo, cod_prod, produto) VALUES ($1,$2,$3,'GRUPO UM','TSEGPAA','A')`, empresaID, vinculoID, vig)
	db.Exec(`INSERT INTO farol.metas_grupos_objetivos (empresa_id, vinculo_id, vigencia_id, uf, grupo, objetivo) VALUES ($1,$2,$3,'PA','GRUPO UM',10),($1,$2,$3,'GO','GRUPO UM',10)`, empresaID, vinculoID, vig)
	for _, c := range [][]string{{"TPA1", "PA"}, {"TGO1", "GO"}} {
		if _, err := db.Exec(`INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_fornec, cod_cli, cod_cliprinc, uf, cod_prod, embalagem, qt_unit_cx, qt, pvenda, tipo_venda, cod_gerente, cod_supervisor, cod_rca)
			VALUES ($1,'2026-07-05',$2,$3,$3,$4,'TSEGPAA','UN/0001/UN',1,3,0,'1','3','91','4132')`, empresaID, forn, c[0], c[1]); err != nil {
			t.Fatalf("venda: %v", err)
		}
	}
	segInvalidarCache()
	ufsDoPainel := func() map[string]bool {
		p, err := segPainel(db, empresaID, vinculoID, "2026-07", "faturado", segEscopo{})
		if err != nil {
			t.Fatalf("segPainel: %v", err)
		}
		out := map[string]bool{}
		for _, u := range p.UFs {
			out[u.UF] = true
		}
		return out
	}
	if u := ufsDoPainel(); u["PA"] || !u["GO"] {
		t.Errorf("PA oculta por padrão: UFs=%v, quero só GO", u)
	}
	db.Exec(`UPDATE farol.metas_vinculos SET ufs_ocultas = '{}' WHERE id = $1`, vinculoID)
	segInvalidarCache()
	if u := ufsDoPainel(); !u["PA"] || !u["GO"] {
		t.Errorf("PA liberada: UFs=%v, quero PA e GO", u)
	}
}
