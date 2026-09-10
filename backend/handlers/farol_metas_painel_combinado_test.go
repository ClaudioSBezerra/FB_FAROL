package handlers

// farol_metas_painel_combinado_test.go — cobre os itens pedidos pelo
// Claudio em 10/09/2026: filtro de Período (de/até) na visão Combinado, e
// que esse filtro NUNCA bypassa o congelamento de uma vigência FECHADA
// quando as datas batem exatamente com os bounds da vigência (FR17).

import (
	"testing"
)

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
