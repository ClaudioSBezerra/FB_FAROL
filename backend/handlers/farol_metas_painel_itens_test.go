package handlers

// farol_metas_painel_itens_test.go — cobre o drill-down de itens por
// Rede/Loja pedido pelo Claudio em 10/09/2026: "clicar na rede traz os
// itens que venderam e não venderam" (Quantidade e Valor).
//
// Desde a migration 236 (14/09/2026) o cálculo saiu do request e virou
// RecalcularItensRealizado (grava farol.metas_itens_realizado) +
// calcularItensPorEscopo (só lê de lá) — os testes abaixo primeiro rodam o
// recálculo com os fixtures, depois conferem a leitura.

import "testing"

// TestItensRealizado_VendeuENaoVendeu cobre o caso central: um EAN com 2
// cod_prod (variantes) soma Qtd/Valor das duas; um EAN nunca vendido
// aparece com Vendeu=false, Qtd=0, Valor=0 — não é descartado da lista.
func TestItensRealizado_VendeuENaoVendeu(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TITENS Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 1.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")

	cnpj1, cnpj2 := "70000000000101", "70000000000102"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cnpj1, cnpj2}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE ITENS", cnpj1, "TCALC-RCAITENS")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE ITENS", cnpj2, "TCALC-RCAITENS")

	db.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigenciaID)
	db.Exec(`INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod) VALUES
		($1,$2,$3,'EAN-VENDIDO','PRODV1'),
		($1,$2,$3,'EAN-VENDIDO','PRODV2'),
		($1,$2,$3,'EAN-NUNCA-VENDIDO','PRODNUNCA')`, empresaID, vinculoID, vigenciaID)

	// EAN-VENDIDO: cnpj1 compra PRODV1 (qtd 2, R$20), cnpj2 compra PRODV2
	// (qtd 3, R$45) — mesma EAN, duas variantes de cod_prod, soma os dois.
	inserirVendaFaturadaFixture(t, empresaID, cnpj1, "PRODV1", "TCALC-RCAITENS", "1", 20, 2, "2026-07-05")
	inserirVendaFaturadaFixture(t, empresaID, cnpj2, "PRODV2", "TCALC-RCAITENS", "1", 45, 3, "2026-07-10")
	// PRODNUNCA nunca é vendido por ninguém.

	if err := RecalcularItensRealizado(db, empresaID, vinculoID, vigenciaID, "faturado"); err != nil {
		t.Fatalf("RecalcularItensRealizado: %v", err)
	}

	itens, err := calcularItensPorEscopo(db, empresaID, vigenciaID, "faturado", []string{cnpj1, cnpj2})
	if err != nil {
		t.Fatalf("calcularItensPorEscopo: %v", err)
	}
	if len(itens) != 2 {
		t.Fatalf("len(itens) = %d, want 2 (EAN-VENDIDO + EAN-NUNCA-VENDIDO)", len(itens))
	}

	porEan := map[string]PainelItemLinha{}
	for _, it := range itens {
		porEan[it.EAN] = it
	}

	vendido, ok := porEan["EAN-VENDIDO"]
	if !ok {
		t.Fatalf("EAN-VENDIDO ausente do resultado")
	}
	if !vendido.Vendeu {
		t.Errorf("EAN-VENDIDO.Vendeu = false, want true")
	}
	if vendido.Qtd != 5 { // 2 (cnpj1/PRODV1) + 3 (cnpj2/PRODV2)
		t.Errorf("EAN-VENDIDO.Qtd = %.0f, want 5 (soma das 2 variantes de cod_prod)", vendido.Qtd)
	}
	if vendido.Valor != 65 { // 20 + 45
		t.Errorf("EAN-VENDIDO.Valor = %.2f, want 65 (soma das 2 variantes)", vendido.Valor)
	}

	naoVendido, ok := porEan["EAN-NUNCA-VENDIDO"]
	if !ok {
		t.Fatalf("EAN-NUNCA-VENDIDO ausente do resultado — item nunca vendido não pode ser descartado")
	}
	if naoVendido.Vendeu {
		t.Errorf("EAN-NUNCA-VENDIDO.Vendeu = true, want false")
	}
	if naoVendido.Qtd != 0 || naoVendido.Valor != 0 {
		t.Errorf("EAN-NUNCA-VENDIDO Qtd/Valor = %.2f/%.2f, want 0/0", naoVendido.Qtd, naoVendido.Valor)
	}

	// Escopo de UMA loja só (cnpj1): EAN-VENDIDO deve refletir só a compra
	// do cnpj1 (PRODV1: qtd 2, R$20), não a soma das duas lojas.
	itensLoja1, err := calcularItensPorEscopo(db, empresaID, vigenciaID, "faturado", []string{cnpj1})
	if err != nil {
		t.Fatalf("calcularItensPorEscopo (loja1): %v", err)
	}
	var vendidoLoja1 PainelItemLinha
	for _, it := range itensLoja1 {
		if it.EAN == "EAN-VENDIDO" {
			vendidoLoja1 = it
		}
	}
	if vendidoLoja1.Qtd != 2 || vendidoLoja1.Valor != 20 {
		t.Errorf("escopo loja1: EAN-VENDIDO Qtd/Valor = %.0f/%.2f, want 2/20 (só a compra do cnpj1)", vendidoLoja1.Qtd, vendidoLoja1.Valor)
	}
}

// TestCnpjsDoEscopoNaVigencia_PorGGVCRV_SemCodPrincNemCnpj cobre o fix de
// 18/09/2026: clicar numa linha das abas "Resumo GGVs×CRVs"/"...×RCAs" (sem
// nenhuma Rede/Loja específica, só cod_ggv+cod_crv) precisa devolver os
// CNPJs de TODAS as Redes daquele par — antes, o if/else de
// cnpjsDoEscopoNaVigencia forçava um filtro cod_princ vazio nesse caso
// (nenhum dos dois preenchido) e a lista sempre voltava vazia.
func TestCnpjsDoEscopoNaVigencia_PorGGVCRV_SemCodPrincNemCnpj(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TITENSGGV Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 1.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")

	// inserirClienteValidoFixture grava sempre cod_ggv="TCALC-GGV" e
	// cod_crv="TCALC-CRV" — 2 Redes diferentes (REDE A/REDE B) caindo no
	// MESMO par GGV×CRV, pra confirmar que o agrupamento junta as duas.
	cnpj1, cnpj2 := "70000000000201", "70000000000202"
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE A", cnpj1, "TCALC-RCAITENSGGV")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE B", cnpj2, "TCALC-RCAITENSGGV")

	cnpjs, err := cnpjsDoEscopoNaVigencia(db, empresaID, vigenciaID, "", "", "TCALC-GGV", "TCALC-CRV", "")
	if err != nil {
		t.Fatalf("cnpjsDoEscopoNaVigencia: %v", err)
	}
	achou := map[string]bool{}
	for _, c := range cnpjs {
		achou[c] = true
	}
	if !achou[cnpj1] || !achou[cnpj2] {
		t.Errorf("cnpjsDoEscopoNaVigencia(cod_ggv=TCALC-GGV, cod_crv=TCALC-CRV) = %v, want conter %s e %s (as 2 Redes do par)", cnpjs, cnpj1, cnpj2)
	}

	// cod_crv de outro par não deveria trazer nada.
	vazio, err := cnpjsDoEscopoNaVigencia(db, empresaID, vigenciaID, "", "", "TCALC-GGV", "CRV-INEXISTENTE", "")
	if err != nil {
		t.Fatalf("cnpjsDoEscopoNaVigencia (crv inexistente): %v", err)
	}
	if len(vazio) != 0 {
		t.Errorf("cnpjsDoEscopoNaVigencia com cod_crv inexistente devolveu %d cnpjs, want 0", len(vazio))
	}
}

// TestRecalcularItensRealizado_VinculoCobertura_NoOp confirma que vínculos
// de Cobertura (sem lista de Itens Válidos — a única métrica que tem é
// Sortimento) não geram nenhuma linha em metas_itens_realizado.
func TestRecalcularItensRealizado_VinculoCobertura_NoOp(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TITENS Cobertura", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")

	if err := RecalcularItensRealizado(db, empresaID, vinculoID, vigenciaID, "faturado"); err != nil {
		t.Fatalf("RecalcularItensRealizado (Cobertura): esperava no-op silencioso, veio erro: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM farol.metas_itens_realizado WHERE vigencia_id = $1`, vigenciaID).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("metas_itens_realizado tem %d linha(s) pra vínculo de Cobertura, want 0", n)
	}
}
