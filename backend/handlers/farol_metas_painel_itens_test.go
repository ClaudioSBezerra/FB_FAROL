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

// TestItensRealizado_CodProdComDoisEANs_ContaUmaVezSo cobre o achado de
// 18-19/09/2026 (conferência com o Carlos, cliente real 154161 SUPERMERCADO
// SOUSA): a base de Itens Válidos pode ter o MESMO cod_prod cadastrado sob
// 2 EANs diferentes (confirmado pelo Carlos: "pode ter 2 EANs válidos").
// Antes do fix, uma única venda desse cod_prod virava 2 linhas em
// metas_itens_realizado (uma por EAN) e contava 2x em contarEANsPositivados
// — agrupando por componente conexo (agruparItensPorComponente), agora
// conta 1 vez só, e o outro EAN da base (nunca vendido por nenhum cod_prod)
// continua contando como item distinto.
func TestItensRealizado_CodProdComDoisEANs_ContaUmaVezSo(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TITENSDUP Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 1.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpj := "70000000000301"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cnpj}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE DUP", cnpj, "TCALC-RCADUP")

	// EAN-DUPLICADO tem 2 cod_prod DIFERENTES, cada um mapeado a um EAN
	// PRÓPRIO — mas os dois EANs também aparecem cruzados: PRODDUP1 está em
	// EAN-A e EAN-B (mesmo padrão achado na base real: cod_prod 477328 em
	// 7891150044906 E 7891150107489). EAN-OUTRO nunca é vendido.
	db.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigenciaID)
	db.Exec(`INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod) VALUES
		($1,$2,$3,'EAN-A','PRODDUP1'),
		($1,$2,$3,'EAN-B','PRODDUP1'),
		($1,$2,$3,'EAN-OUTRO','PRODNUNCA')`, empresaID, vinculoID, vigenciaID)

	// 1 única venda de PRODDUP1 — qtd 1, R$50.
	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODDUP1", "TCALC-RCADUP", "1", 50, 1, "2026-08-05")

	if err := RecalcularItensRealizado(db, empresaID, vinculoID, vigenciaID, "faturado"); err != nil {
		t.Fatalf("RecalcularItensRealizado: %v", err)
	}

	itens, err := calcularItensPorEscopo(db, empresaID, vigenciaID, "faturado", []string{cnpj})
	if err != nil {
		t.Fatalf("calcularItensPorEscopo: %v", err)
	}
	// 2 grupos esperados: {EAN-A,EAN-B} (1 grupo só, canônico = EAN-A por
	// ordem alfabética) + EAN-OUTRO — NUNCA 3 (seria o bug: 1 linha por EAN
	// cru, contando a mesma venda 2x).
	if len(itens) != 2 {
		t.Fatalf("len(itens) = %d, want 2 (1 grupo do cod_prod duplicado + EAN-OUTRO nunca vendido) — itens: %+v", len(itens), itens)
	}

	var vendido *PainelItemLinha
	for i := range itens {
		if itens[i].Vendeu {
			vendido = &itens[i]
		}
	}
	if vendido == nil {
		t.Fatalf("nenhum item marcado como vendido — esperava 1: %+v", itens)
	}
	if vendido.EAN != "EAN-A" {
		t.Errorf("EAN do grupo vendido = %q, want EAN-A (canônico = menor EAN do componente)", vendido.EAN)
	}
	if vendido.Qtd != 1 || vendido.Valor != 50 {
		t.Errorf("grupo vendido Qtd/Valor = %.0f/%.2f, want 1/50 (a venda não pode ser contada 2x só porque o cod_prod tem 2 EANs)", vendido.Qtd, vendido.Valor)
	}

	// Confere que o indicador OFICIAL (calcularSortimentoPorRede, via
	// CalcularRealizado) também conta 1 só, não 2 — é o número que decide
	// "atingiu"/"não atingiu" de verdade, não o drill-down.
	var tiposVenda []string
	db.QueryRow(`SELECT tipos_venda_validos FROM farol.metas_vinculos WHERE id = $1`, vinculoID)
	itensValidos, err := lerItensValidos(db, empresaID, vigenciaID)
	if err != nil {
		t.Fatalf("lerItensValidos: %v", err)
	}
	clientes, err := lerClientesValidos(db, empresaID, vigenciaID)
	if err != nil {
		t.Fatalf("lerClientesValidos: %v", err)
	}
	redes, err := calcularSortimentoPorRede(db, empresaID, clientes, itensValidos,
		map[string]any{"qtd_minima_positivacao": 1.0}, "2026-08-01", "2026-08-31", "faturado", tiposVenda, nil)
	if err != nil {
		t.Fatalf("calcularSortimentoPorRede: %v", err)
	}
	if len(redes) != 1 {
		t.Fatalf("len(redes) = %d, want 1", len(redes))
	}
	if redes[0].Valor != 1 {
		t.Errorf("Sortimento oficial da Rede = %.1f, want 1 (1 grupo positivado, não 2 — mesma venda não pode contar em dobro)", redes[0].Valor)
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

// TestItensRealizado_DataUltimaVenda — pedido do Claudio 22/09/2026: cada
// PRODUTO tem sua PRÓPRIA data de última venda (diferente de "Dt.Ult.Cmp",
// que é fato do Cliente — ver resolverDataUltimaCompraClientes). Cobre 3
// casos: item vendido DENTRO da vigência, item "Não coberto" nesta
// vigência mas vendido em período anterior (mostra a data histórica, não
// vazio), e item vendido DEPOIS do fim da vigência (não pode vazar pro
// passado — vigência de Agosto não pode saber de uma venda de Setembro).
func TestItensRealizado_DataUltimaVenda(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TDUV Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 1.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpj := "70000000000201"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cnpj}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE DUV", cnpj, "TCALC-RCADUV")

	db.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigenciaID)
	db.Exec(`INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod) VALUES
		($1,$2,$3,'EAN-VENDIDO-AGOSTO','PRODV1'),
		($1,$2,$3,'EAN-VENDIDO-SO-ANTES','PRODV2'),
		($1,$2,$3,'EAN-SO-VENDIDO-DEPOIS','PRODV3'),
		($1,$2,$3,'EAN-NUNCA-VENDIDO','PRODNUNCA')`, empresaID, vinculoID, vigenciaID)

	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODV1", "TCALC-RCADUV", "1", 20, 2, "2026-08-15")
	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODV2", "TCALC-RCADUV", "1", 10, 1, "2026-06-20") // antes da vigência
	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODV3", "TCALC-RCADUV", "1", 30, 3, "2026-09-05") // depois da vigência

	if err := RecalcularItensRealizado(db, empresaID, vinculoID, vigenciaID, "faturado"); err != nil {
		t.Fatalf("RecalcularItensRealizado: %v", err)
	}
	itens, err := calcularItensPorEscopo(db, empresaID, vigenciaID, "faturado", []string{cnpj})
	if err != nil {
		t.Fatalf("calcularItensPorEscopo: %v", err)
	}
	porEan := map[string]PainelItemLinha{}
	for _, it := range itens {
		porEan[it.EAN] = it
	}

	vendidoAgosto := porEan["EAN-VENDIDO-AGOSTO"]
	if !vendidoAgosto.Vendeu || vendidoAgosto.DataUltimaVenda != "2026-08-15" {
		t.Errorf("EAN-VENDIDO-AGOSTO: Vendeu=%v DataUltimaVenda=%q, want Vendeu=true DataUltimaVenda=2026-08-15",
			vendidoAgosto.Vendeu, vendidoAgosto.DataUltimaVenda)
	}

	vendidoAntes := porEan["EAN-VENDIDO-SO-ANTES"]
	if vendidoAntes.Vendeu {
		t.Errorf("EAN-VENDIDO-SO-ANTES: Vendeu = true, want false (venda de junho, fora da vigência de agosto)")
	}
	if vendidoAntes.DataUltimaVenda != "2026-06-20" {
		t.Errorf("EAN-VENDIDO-SO-ANTES: DataUltimaVenda = %q, want 2026-06-20 (histórico, mesmo não tendo vendido NESTA vigência)", vendidoAntes.DataUltimaVenda)
	}

	soVendidoDepois := porEan["EAN-SO-VENDIDO-DEPOIS"]
	if soVendidoDepois.Vendeu {
		t.Errorf("EAN-SO-VENDIDO-DEPOIS: Vendeu = true, want false (venda de setembro, fora da vigência de agosto)")
	}
	if soVendidoDepois.DataUltimaVenda != "" {
		t.Errorf("EAN-SO-VENDIDO-DEPOIS: DataUltimaVenda = %q, want vazio (venda de setembro não pode vazar pra quem olha agosto)", soVendidoDepois.DataUltimaVenda)
	}

	nuncaVendido := porEan["EAN-NUNCA-VENDIDO"]
	if nuncaVendido.DataUltimaVenda != "" {
		t.Errorf("EAN-NUNCA-VENDIDO: DataUltimaVenda = %q, want vazio", nuncaVendido.DataUltimaVenda)
	}
}
