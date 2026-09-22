package handlers

// farol_metas_painel_combinado_test.go — cobre os itens pedidos pelo
// Claudio em 10/09/2026: filtro de Período (de/até) na visão Combinado, e
// que esse filtro NUNCA bypassa o congelamento de uma vigência FECHADA
// quando as datas batem exatamente com os bounds da vigência (FR17).

import (
	"testing"
)

// TestResolverUFClientes_QualquerFornecedor confirma o comportamento
// deliberado e inalterado de resolverUFClientes: UF vem da venda mais
// recente em QUALQUER fornecedor (é geografia do cliente, não muda por
// indústria) — ao contrário de resolverDataUltimaCompraClientes abaixo.
func TestResolverUFClientes_QualquerFornecedor(t *testing.T) {
	db, empresaID := biTestDB(t)

	cnpj := "11111111000101"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cnpj = $2 AND cod_rca = 'TUF-TEST'`, empresaID, cnpj)
	})
	_, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cnpj, cod_cliprinc, cod_prod, cod_rca, cod_supervisor, nome_supervisor, cod_gerente, nome_gerente, tipo_venda, cod_fornec, pvenda, qt, uf)
		VALUES ($1, '2026-08-10', $2, $2, 'PROD-TUF', 'TUF-TEST', 'SUP', 'Sup', 'GER', 'Ger', '1', '999', 100, 1, 'SP')
	`, empresaID, cnpj)
	if err != nil {
		t.Fatalf("inserir fixture: %v", err)
	}

	out, err := resolverUFClientes(db, empresaID, []string{cnpj})
	if err != nil {
		t.Fatalf("resolverUFClientes: %v", err)
	}
	if out[cnpj] != "SP" {
		t.Errorf("UF = %q, esperava SP (mesmo com cod_fornec 999, não filtrado)", out[cnpj])
	}
}

// TestResolverDataUltimaCompraClientes_FiltraPorFornecedorEPeriodo — 2
// achados reais do Claudio no mesmo dia (22/09/2026):
//  1. RCA 3691 via "Dt.Ult.Cmp" = 21/09 na tela de UNILEVER FOOD, mas essa
//     data era de uma venda de UNILEVER HC (cod_fornec 396) — a última
//     compra REAL de FOOD (cod_fornec 131) tinha sido bem antes.
//  2. Olhando o PERÍODO de Agosto/2026 (mês fechado), a tela mostrava uma
//     data de Setembro — "não faz sentido olhar Agosto e ver Setembro".
//
// resolverDataUltimaCompraClientes precisa filtrar por cod_fornec/tipos de
// venda do vínculo E respeitar o período sendo calculado — as 3 vendas
// abaixo (fornecedor errado, fora do período, e a certa) provam os 2 casos
// de uma vez.
func TestResolverDataUltimaCompraClientes_FiltraPorFornecedorEPeriodo(t *testing.T) {
	db, empresaID := biTestDB(t)

	cnpj := "22222222000102"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cnpj = $2 AND cod_rca = 'TDUF-TEST'`, empresaID, cnpj)
	})
	inserir := func(data, codFornec string) {
		_, err := db.Exec(`
			INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cnpj, cod_cliprinc, cod_prod, cod_rca, cod_supervisor, nome_supervisor, cod_gerente, nome_gerente, tipo_venda, cod_fornec, pvenda, qt, uf)
			VALUES ($1, $2, $3, $3, 'PROD-TDUF', 'TDUF-TEST', 'SUP', 'Sup', 'GER', 'Ger', '1', $4, 100, 1, 'GO')
		`, empresaID, data, cnpj, codFornec)
		if err != nil {
			t.Fatalf("inserir fixture: %v", err)
		}
	}
	inserir("2026-09-21", "396") // UNILEVER HC — mais recente, mas de OUTRA indústria
	inserir("2026-09-16", "131") // UNILEVER FOOD — fornecedor certo, mas FORA do período de Agosto
	inserir("2026-08-25", "131") // UNILEVER FOOD dentro de Agosto — a resposta certa pra vigência de Agosto

	// Vigência de Agosto (o caso que expôs o bug): nem a venda de HC nem a
	// de FOOD de Setembro podem vencer — só a de 25/08.
	outAgosto, err := resolverDataUltimaCompraClientes(db, empresaID, []string{cnpj}, "2026-08-01", "2026-08-31", nil, []string{"131"})
	if err != nil {
		t.Fatalf("resolverDataUltimaCompraClientes (agosto): %v", err)
	}
	if got := formatarDataUltimaCompra(outAgosto[cnpj]); got != "2026-08-25" {
		t.Errorf("agosto: DataUltimaCompra = %q, esperava 2026-08-25 (única venda de FOOD dentro do período)", got)
	}

	// Vigência de Setembro, mesmo fornecedor: agora a de 16/09 é a certa —
	// a de HC (396) continua fora por ser de outro fornecedor.
	outSetembro, err := resolverDataUltimaCompraClientes(db, empresaID, []string{cnpj}, "2026-09-01", "2026-09-30", nil, []string{"131"})
	if err != nil {
		t.Fatalf("resolverDataUltimaCompraClientes (setembro): %v", err)
	}
	if got := formatarDataUltimaCompra(outSetembro[cnpj]); got != "2026-09-16" {
		t.Errorf("setembro: DataUltimaCompra = %q, esperava 2026-09-16 (venda de FOOD dentro do período, não a de HC)", got)
	}

	// Sem filtro de cod_fornec, mesmo período de Setembro — aí a de HC
	// (21/09) vence, por ser a mais recente de qualquer fornecedor DENTRO
	// do período.
	outSemFiltro, err := resolverDataUltimaCompraClientes(db, empresaID, []string{cnpj}, "2026-09-01", "2026-09-30", nil, nil)
	if err != nil {
		t.Fatalf("resolverDataUltimaCompraClientes sem filtro de fornecedor: %v", err)
	}
	if got := formatarDataUltimaCompra(outSemFiltro[cnpj]); got != "2026-09-21" {
		t.Errorf("sem filtro de cod_fornec: DataUltimaCompra = %q, esperava 2026-09-21", got)
	}
}

// TestCalcularPainelCombinado_PeriodoManual_EstreitaOCalculo confirma que
// passar data_inicio/data_fim MENORES que a vigência inteira realmente
// recalcula só aquela janela — não é um filtro cosmético.
func TestCalcularPainelCombinado_PeriodoManual_EstreitaOCalculo(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoCob, cleanupCob := criarVinculoComFormula(t, empresaID, "TPC Cobertura", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanupCob)
	vigCob := criarVigenciaFixture(t, db, empresaID, vinculoCob, "2026-05-01", "2026-05-31")

	vinculoSort, cleanupSort := criarVinculoComFormula(t, empresaID, "TPC Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 1.0})
	t.Cleanup(cleanupSort)
	vigSort := criarVigenciaFixture(t, db, empresaID, vinculoSort, "2026-05-01", "2026-05-31")
	db.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1,$2,1,1)`, empresaID, vigSort)

	cnpj := "50000000000199"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cnpj}) })
	inserirClienteValidoFixture(t, empresaID, vinculoCob, vigCob, "REDE PC", cnpj, "TCALC-RCAPC")
	inserirClienteValidoFixture(t, empresaID, vinculoSort, vigSort, "REDE PC", cnpj, "TCALC-RCAPC")
	db.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigSort)
	db.Exec(`INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod) VALUES ($1,$2,$3,'EANPC','PRODPC')`,
		empresaID, vinculoSort, vigSort)

	// R$50 no dia 5 (abaixo do limiar 100), +R$80 no dia 20 (total R$130,
	// acima do limiar) — só a janela completa do mês bate a Cobertura.
	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODPC", "TCALC-RCAPC", "1", 50, 1, "2026-05-05")
	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODPC", "TCALC-RCAPC", "1", 80, 1, "2026-05-20")

	// Vigência inteira (sem override) — R$130, atinge o limiar de R$100.
	respCompleto, err := calcularPainelCombinado(db, empresaID, vinculoCob, vigCob, vinculoSort, vigSort, "faturado", "", "")
	if err != nil {
		t.Fatalf("calcularPainelCombinado (sem override): %v", err)
	}
	if len(respCompleto.Redes) != 1 || respCompleto.Redes[0].CoberturaValor != 130 {
		t.Fatalf("sem override: cobertura_valor = %+v, want 130", respCompleto.Redes)
	}
	if !respCompleto.Redes[0].CoberturaAtingiu {
		t.Errorf("sem override: deveria ter atingido o limiar (R$130 >= R$100)")
	}
	if respCompleto.DataInicioUsada != "2026-05-01" || respCompleto.DataFimUsada != "2026-05-31" {
		t.Errorf("DataInicioUsada/DataFimUsada = %s/%s, want 2026-05-01/2026-05-31", respCompleto.DataInicioUsada, respCompleto.DataFimUsada)
	}

	// Override 01–10/05 — só pega a venda do dia 5 (R$50), abaixo do limiar.
	respEstreito, err := calcularPainelCombinado(db, empresaID, vinculoCob, vigCob, vinculoSort, vigSort, "faturado", "2026-05-01", "2026-05-10")
	if err != nil {
		t.Fatalf("calcularPainelCombinado (override 01-10): %v", err)
	}
	if len(respEstreito.Redes) != 1 || respEstreito.Redes[0].CoberturaValor != 50 {
		t.Fatalf("com override 01-10: cobertura_valor = %+v, want 50", respEstreito.Redes)
	}
	if respEstreito.Redes[0].CoberturaAtingiu {
		t.Errorf("com override 01-10: NÃO deveria ter atingido o limiar (R$50 < R$100)")
	}
	if respEstreito.DataInicioUsada != "2026-05-01" || respEstreito.DataFimUsada != "2026-05-10" {
		t.Errorf("DataInicioUsada/DataFimUsada = %s/%s, want 2026-05-01/2026-05-10", respEstreito.DataInicioUsada, respEstreito.DataFimUsada)
	}
}

// TestCalcularPainelCombinado_PeriodoIgualAosBoundsNaoBypassaCongelamento é
// o teste de invariante crítico (FR17): se o front mandar data_inicio/
// data_fim que batem EXATAMENTE com os bounds da vigência (o caso comum —
// usuário não mexeu no filtro), uma vigência FECHADA deve continuar
// servindo do snapshot congelado, não recalcular ao vivo.
func TestCalcularPainelCombinado_PeriodoIgualAosBoundsNaoBypassaCongelamento(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoCob, cleanupCob := criarVinculoComFormula(t, empresaID, "TPCF Cobertura", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanupCob)
	vigCob := criarVigenciaFixture(t, db, empresaID, vinculoCob, "2026-06-01", "2026-06-30")

	vinculoSort, cleanupSort := criarVinculoComFormula(t, empresaID, "TPCF Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 1.0})
	t.Cleanup(cleanupSort)
	vigSort := criarVigenciaFixture(t, db, empresaID, vinculoSort, "2026-06-01", "2026-06-30")
	db.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1,$2,1,1)`, empresaID, vigSort)

	cnpj := "60000000000199"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cnpj}) })
	inserirClienteValidoFixture(t, empresaID, vinculoCob, vigCob, "REDE PCF", cnpj, "TCALC-RCAPCF")
	inserirClienteValidoFixture(t, empresaID, vinculoSort, vigSort, "REDE PCF", cnpj, "TCALC-RCAPCF")
	db.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigSort)
	db.Exec(`INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod) VALUES ($1,$2,$3,'EANPCF','PRODPCF')`,
		empresaID, vinculoSort, vigSort)

	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODPCF", "TCALC-RCAPCF", "1", 130, 1, "2026-06-05")

	// Fecha as duas vigências e congela (primeiro cálculo cria o snapshot).
	db.Exec(`UPDATE farol.metas_vigencias SET status = 'fechada' WHERE id = $1`, vigCob)
	db.Exec(`UPDATE farol.metas_vigencias SET status = 'fechada' WHERE id = $1`, vigSort)
	if _, err := calcularPainelCombinado(db, empresaID, vinculoCob, vigCob, vinculoSort, vigSort, "faturado", "", ""); err != nil {
		t.Fatalf("congelamento inicial: %v", err)
	}

	// Venda NOVA depois do congelamento — não deveria aparecer em NENHUMA
	// consulta subsequente da vigência fechada (nem sem override, nem com
	// override == bounds da vigência).
	inserirVendaFaturadaFixture(t, empresaID, cnpj, "PRODPCF", "TCALC-RCAPCF", "1", 900, 1, "2026-06-20")

	respSemOverride, err := calcularPainelCombinado(db, empresaID, vinculoCob, vigCob, vinculoSort, vigSort, "faturado", "", "")
	if err != nil {
		t.Fatalf("calcularPainelCombinado (sem override, fechada): %v", err)
	}
	if respSemOverride.Redes[0].CoberturaValor != 130 {
		t.Fatalf("sem override, vigência fechada: cobertura_valor = %.0f, want 130 (congelado, venda de R$900 não deveria contar)", respSemOverride.Redes[0].CoberturaValor)
	}

	respOverrideIgual, err := calcularPainelCombinado(db, empresaID, vinculoCob, vigCob, vinculoSort, vigSort, "faturado", "2026-06-01", "2026-06-30")
	if err != nil {
		t.Fatalf("calcularPainelCombinado (override == bounds, fechada): %v", err)
	}
	if respOverrideIgual.Redes[0].CoberturaValor != 130 {
		t.Fatalf("override == bounds, vigência fechada: cobertura_valor = %.0f, want 130 (deveria continuar servindo do snapshot, não recalcular ao vivo)", respOverrideIgual.Redes[0].CoberturaValor)
	}

	// Override DIFERENTE dos bounds — aí sim é uma janela deliberadamente
	// menor, deve recalcular ao vivo e enxergar a venda nova.
	respOverrideDiferente, err := calcularPainelCombinado(db, empresaID, vinculoCob, vigCob, vinculoSort, vigSort, "faturado", "2026-06-01", "2026-06-25")
	if err != nil {
		t.Fatalf("calcularPainelCombinado (override diferente, fechada): %v", err)
	}
	if respOverrideDiferente.Redes[0].CoberturaValor != 1030 {
		t.Fatalf("override diferente dos bounds: cobertura_valor = %.0f, want 1030 (130+900, ao vivo)", respOverrideDiferente.Redes[0].CoberturaValor)
	}
}

// TestCalcularPainelCombinado_ClienteTrazDataUltimaCompra — pedido do
// Claudio 22/09/2026 (visão do RCA no drill-down de Cliente, "Dt.Ult.Cmp").
// TestResolverUFClientes_* já cobre a função isolada; este cobre a cadeia
// inteira até a resposta que o front consome (calcularCoberturaPorRede →
// RealizadoCliente.DataUltimaCompra → montarClientesCombinado →
// PainelCombinadoCliente.DataUltimaCompra), com uma venda de verdade
// (fixture não seta uf por padrão — aqui insere direto pra ter o dado que
// a data depende).
func TestCalcularPainelCombinado_ClienteTrazDataUltimaCompra(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoCob, cleanupCob := criarVinculoComFormula(t, empresaID, "TDUC Cobertura", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanupCob)
	vigCob := criarVigenciaFixture(t, db, empresaID, vinculoCob, "2026-08-01", "2026-08-31")

	vinculoSort, cleanupSort := criarVinculoComFormula(t, empresaID, "TDUC Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 1.0})
	t.Cleanup(cleanupSort)
	vigSort := criarVigenciaFixture(t, db, empresaID, vinculoSort, "2026-08-01", "2026-08-31")
	db.Exec(`INSERT INTO farol.metas_faixas (empresa_id, vigencia_id, faixa, valor_meta) VALUES ($1,$2,1,1)`, empresaID, vigSort)

	cnpj := "50000000000280"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cnpj = $2 AND cod_rca = 'TDUC-RCA'`, empresaID, cnpj)
	})
	inserirClienteValidoFixture(t, empresaID, vinculoCob, vigCob, "REDE DUC", cnpj, "TDUC-RCA")
	inserirClienteValidoFixture(t, empresaID, vinculoSort, vigSort, "REDE DUC", cnpj, "TDUC-RCA")
	db.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigSort)
	db.Exec(`INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod) VALUES ($1,$2,$3,'EANDUC','PRODDUC')`,
		empresaID, vinculoSort, vigSort)

	codCliprinc := codPrincDoClienteValidoFixture(t, empresaID, cnpj)
	inserirVendaComUF := func(data string) {
		_, err := db.Exec(`
			INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cnpj, cod_cliprinc, cod_prod, cod_rca, cod_supervisor, nome_supervisor, cod_gerente, nome_gerente, tipo_venda, pvenda, qt, uf)
			VALUES ($1, $2, $3, $4, 'PRODDUC', 'TDUC-RCA', 'SUP', 'Sup', 'GER', 'Ger', '1', 150, 1, 'GO')
		`, empresaID, data, cnpj, codCliprinc)
		if err != nil {
			t.Fatalf("inserir venda com uf: %v", err)
		}
	}
	inserirVendaComUF("2026-08-05")
	inserirVendaComUF("2026-08-22") // a mais recente — deve vencer

	resp, err := calcularPainelCombinado(db, empresaID, vinculoCob, vigCob, vinculoSort, vigSort, "faturado", "", "")
	if err != nil {
		t.Fatalf("calcularPainelCombinado: %v", err)
	}
	var cliente *PainelCombinadoCliente
	for i := range resp.Clientes {
		if resp.Clientes[i].CNPJ == cnpj {
			cliente = &resp.Clientes[i]
		}
	}
	if cliente == nil {
		t.Fatalf("cliente %s não apareceu em resp.Clientes (%+v)", cnpj, resp.Clientes)
	}
	if cliente.DataUltimaCompra != "2026-08-22" {
		t.Errorf("DataUltimaCompra = %q, want 2026-08-22 (a venda mais recente do cliente, não a primeira)", cliente.DataUltimaCompra)
	}
}
