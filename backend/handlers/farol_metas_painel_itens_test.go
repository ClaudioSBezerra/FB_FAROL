package handlers

// farol_metas_painel_itens_test.go — cobre o drill-down de itens por
// Rede/Loja pedido pelo Claudio em 10/09/2026: "clicar na rede traz os
// itens que venderam e não venderam" (Quantidade e Valor).

import "testing"

// TestCalcularItensPorEscopo_VendeuENaoVendeu cobre o caso central: um EAN
// com 2 cod_prod (variantes) soma Qtd/Valor das duas; um EAN nunca vendido
// aparece com Vendeu=false, Qtd=0, Valor=0 — não é descartado da lista.
func TestCalcularItensPorEscopo_VendeuENaoVendeu(t *testing.T) {
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

	itens, err := calcularItensPorEscopo(db, empresaID, vinculoID, vigenciaID, "faturado", []string{cnpj1, cnpj2})
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
	itensLoja1, err := calcularItensPorEscopo(db, empresaID, vinculoID, vigenciaID, "faturado", []string{cnpj1})
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

// TestCalcularItensPorEscopo_VinculoNaoSortimento_Erro confirma que o
// drill-down recusa vínculos de Cobertura (só existe pra Sortimento — a
// única métrica com lista de Itens Válidos).
func TestCalcularItensPorEscopo_VinculoNaoSortimento_Erro(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TITENS Cobertura", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")

	_, err := calcularItensPorEscopo(db, empresaID, vinculoID, vigenciaID, "faturado", []string{"70000000000199"})
	if err == nil {
		t.Fatalf("esperava erro pra vínculo de Cobertura, veio nil")
	}
}
