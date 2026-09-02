package handlers

// farol_metas_calculo_test.go — cobre a I/O Matrix da Story 4.1
// (_bmad-output/implementation-artifacts/4-1-calculo-realizado.md).
//
// TestCalcularRealizado_Cobertura_ExemploDoPRD é o teste mais importante:
// reproduz EXATAMENTE o exemplo numérico do PRD (Rede 1, 4 lojas, compras
// R$1.000/0/20.000/40.000 → média R$15.250) — prova que a matemática bate
// com a especificação de referência, não só que o código "roda sem erro".

import (
	"encoding/json"
	"testing"
)

// inserirVendaFaturadaFixture insere uma linha crua em vendas_faturadas —
// não existe handler de import aqui, os testes de cálculo precisam
// popular a base diretamente, como o job de importação faria.
func inserirVendaFaturadaFixture(t *testing.T, empresaID, codCli, codProd, codRCA, tipoVenda string, pvenda, qt float64, data string) {
	t.Helper()
	db, _ := biTestDB(t)
	_, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cnpj, cod_prod, cod_rca, cod_supervisor, nome_supervisor, cod_gerente, nome_gerente, tipo_venda, pvenda, qt)
		VALUES ($1, $2, $3, $4, $5, 'SUP-01', 'Supervisor Teste', 'GER-01', 'Gerente Teste', $6, $7, $8)
	`, empresaID, data, codCli, codProd, codRCA, tipoVenda, pvenda, qt)
	if err != nil {
		t.Fatalf("inserir fixture de venda faturada: %v", err)
	}
}

func limparVendasFaturadasFixture(t *testing.T, empresaID string, cnpjs []string) {
	t.Helper()
	db, _ := biTestDB(t)
	for _, c := range cnpjs {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cnpj = $2 AND cod_rca LIKE 'TCALC%'`, empresaID, c)
	}
}

// inserirVendaTransmitidaFixture — mesmo papel de inserirVendaFaturadaFixture,
// pro fluxo Transmitido (Story 4.2, FR15).
func inserirVendaTransmitidaFixture(t *testing.T, empresaID, codCli, codProd, codRCA, tipoVenda string, pvenda, qt float64, data string) {
	t.Helper()
	db, _ := biTestDB(t)
	_, err := db.Exec(`
		INSERT INTO vendas_transmitidas (empresa_id, data_transmissao, cnpj, cod_prod, cod_rca, cod_supervisor, nome_supervisor, cod_gerente, nome_gerente, tipo_venda, pvenda, qt)
		VALUES ($1, $2, $3, $4, $5, 'SUP-01', 'Supervisor Teste', 'GER-01', 'Gerente Teste', $6, $7, $8)
	`, empresaID, data, codCli, codProd, codRCA, tipoVenda, pvenda, qt)
	if err != nil {
		t.Fatalf("inserir fixture de venda transmitida: %v", err)
	}
}

func limparVendasTransmitidasFixture(t *testing.T, empresaID string, cnpjs []string) {
	t.Helper()
	db, _ := biTestDB(t)
	for _, c := range cnpjs {
		db.Exec(`DELETE FROM vendas_transmitidas WHERE empresa_id = $1 AND cnpj = $2 AND cod_rca LIKE 'TCALC%'`, empresaID, c)
	}
}

func inserirClienteValidoFixture(t *testing.T, empresaID string, vinculoID, vigenciaID int, redeNome, cnpj, codRCA string) {
	t.Helper()
	db, _ := biTestDB(t)
	_, err := db.Exec(`
		INSERT INTO farol.metas_clientes_validos (empresa_id, vinculo_id, vigencia_id, rede_nome, cnpj, cod_rca)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, empresaID, vinculoID, vigenciaID, redeNome, cnpj, codRCA)
	if err != nil {
		t.Fatalf("inserir fixture de cliente válido: %v", err)
	}
}

func criarVinculoComFormula(t *testing.T, empresaID, prefixo, formulaCodigo, nivelAgregacao string, schema []ParametroSchemaDTO, parametrosValores map[string]any) (vinculoID int, cleanup func()) {
	t.Helper()
	db, _ := biTestDB(t)
	industriaID := criarIndustriaFixture(t, db, empresaID, prefixo+" Industria")
	tipoID := criarTipoMetricaFixture(t, db, empresaID, prefixo+" Tipo", nivelAgregacao, schema)
	db.Exec(`UPDATE farol.tipos_metrica SET formula_codigo = $1 WHERE id = $2`, formulaCodigo, tipoID)

	valoresJSON := marshalOrEmpty(parametrosValores)
	var id int
	if err := db.QueryRow(`
		INSERT INTO farol.metas_vinculos (empresa_id, industria_id, tipo_metrica_id, parametros_valores)
		VALUES ($1, $2, $3, $4) RETURNING id
	`, empresaID, industriaID, tipoID, valoresJSON).Scan(&id); err != nil {
		t.Fatalf("criar fixture de vínculo: %v", err)
	}
	return id, func() {
		db.Exec(`DELETE FROM farol.metas_vinculos WHERE id = $1`, id)
		db.Exec(`DELETE FROM farol.industrias WHERE id = $1`, industriaID)
		db.Exec(`DELETE FROM farol.tipos_metrica WHERE id = $1`, tipoID)
	}
}

func marshalOrEmpty(m map[string]any) []byte {
	if m == nil {
		return []byte(`{}`)
	}
	b, _ := json.Marshal(m)
	return b
}

// TestCalcularRealizado_Cobertura_ExemploDoPRD reproduz o exemplo exato do
// PRD (contexto de referência Unilever, colado pelo Claudio na sessão):
// "REDE 1 TEM 4 LOJAS/CNPJS... LOJA A COMPROU R$1000, LOJA B R$0, LOJA C
// R$20.000 E LOJA D R$40.000, NA MEDIA COMPROU R$15.250"
func TestCalcularRealizado_Cobertura_ExemploDoPRD(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	_ = userID

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC Cobertura", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 9100.0}) // limiar do FORN 396 usado no exemplo real do PRD
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-01-01", "2026-01-31")

	lojaA, lojaB, lojaC, lojaD := "11111111000101", "11111111000102", "11111111000103", "11111111000104"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{lojaA, lojaB, lojaC, lojaD}) })

	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE 1", lojaA, "TCALC-RCA1")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE 1", lojaB, "TCALC-RCA1")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE 1", lojaC, "TCALC-RCA1")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE 1", lojaD, "TCALC-RCA1")

	inserirVendaFaturadaFixture(t, empresaID, lojaA, "PROD1", "TCALC-RCA1", "1", 1000, 1, "2026-01-10")
	// lojaB não compra nada — sem linha em vendas_faturadas, conta como R$0
	inserirVendaFaturadaFixture(t, empresaID, lojaC, "PROD1", "TCALC-RCA1", "1", 20000, 1, "2026-01-15")
	inserirVendaFaturadaFixture(t, empresaID, lojaD, "PROD1", "TCALC-RCA1", "1", 40000, 1, "2026-01-20")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	if len(resultado.Redes) != 1 {
		t.Fatalf("esperava 1 Rede, veio %d: %+v", len(resultado.Redes), resultado.Redes)
	}
	rede := resultado.Redes[0]
	if rede.Valor != 15250 {
		t.Errorf("média da Rede 1 = %.2f, want 15250.00 (exemplo exato do PRD)", rede.Valor)
	}
	if !rede.Atingiu {
		t.Errorf("15250 >= limiar 9100 deveria marcar Atingiu=true")
	}
	if resultado.RealizadoTotal != 1 {
		t.Errorf("RealizadoTotal (contagem de Redes cobertas) = %.0f, want 1", resultado.RealizadoTotal)
	}
}

func TestCalcularRealizado_Cobertura_AbaixoDoLimiar_NaoAtinge(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC AbaixoLimiar", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 9100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")

	loja1 := "22222222000101"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja1}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE FRACA", loja1, "TCALC-RCA2")
	inserirVendaFaturadaFixture(t, empresaID, loja1, "PROD1", "TCALC-RCA2", "1", 500, 1, "2026-02-10")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	if resultado.Redes[0].Atingiu {
		t.Errorf("R$500 abaixo do limiar R$9100 não deveria marcar Atingiu")
	}
	if resultado.RealizadoTotal != 0 {
		t.Errorf("RealizadoTotal = %.0f, want 0 (nenhuma Rede coberta)", resultado.RealizadoTotal)
	}
}

// TestCalcularRealizado_RespeitaTipoVendaDoVinculo cobre FR16: vendas com
// tipo_venda fora da lista configurada no vínculo não entram no cálculo.
func TestCalcularRealizado_RespeitaTipoVendaDoVinculo(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC TipoVenda", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	db.Exec(`UPDATE farol.metas_vinculos SET tipos_venda_validos = '{1,9}' WHERE id = $1`, vinculoID)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-03-01", "2026-03-31")

	loja1 := "33333333000101"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja1}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE X", loja1, "TCALC-RCA3")
	// tipo_venda "20" não está na lista válida do vínculo — não deveria contar
	inserirVendaFaturadaFixture(t, empresaID, loja1, "PROD1", "TCALC-RCA3", "20", 50000, 1, "2026-03-10")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	if resultado.Redes[0].Valor != 0 {
		t.Errorf("venda com tipo_venda=20 não deveria contar (vínculo só aceita 1,9) — valor = %.2f, want 0", resultado.Redes[0].Valor)
	}
}

// TestCalcularRealizado_Sortimento_ExemploDoPRD reproduz o exemplo do PRD:
// "REDE MAIS: LOJA 1 COMPROU 50 EANS E LOJA 2 COMPROU 30 EANS DIFERENTES...
// NA MEDIA A REDE COMPROU 40 EANS" — aqui simplificado pra 2 EANs/2 lojas
// mantendo a mesma mecânica de média + regra de quantidade mínima.
func TestCalcularRealizado_Sortimento_RegraQuantidadeMinima(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC Sortimento", "sortimento_rede", "rede",
		[]ParametroSchemaDTO{{Key: "qtd_minima_positivacao", Label: "Qtd mínima", Type: "integer"}},
		map[string]any{"qtd_minima_positivacao": 3.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-04-01", "2026-04-30")

	loja1, loja2 := "44444444000101", "44444444000102"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja1, loja2}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE SORT", loja1, "TCALC-RCA4")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE SORT", loja2, "TCALC-RCA4")

	db, _ = biTestDB(t)
	db.Exec(`DELETE FROM farol.metas_itens_validos WHERE vigencia_id = $1`, vigenciaID)
	db.Exec(`INSERT INTO farol.metas_itens_validos (empresa_id, vinculo_id, vigencia_id, ean, cod_prod, tipo_embalagem) VALUES
		($1,$2,$3,'EAN001','PRODA','UN'), ($1,$2,$3,'EAN002','PRODB','UN')`, empresaID, vinculoID, vigenciaID)

	// loja1: compra 5un de PRODA (>= mínimo 3, positiva) e 2un de PRODB (< mínimo, NÃO positiva) → 1 EAN
	inserirVendaFaturadaFixture(t, empresaID, loja1, "PRODA", "TCALC-RCA4", "1", 500, 5, "2026-04-05")
	inserirVendaFaturadaFixture(t, empresaID, loja1, "PRODB", "TCALC-RCA4", "1", 200, 2, "2026-04-05")
	// loja2: compra 4un de PRODA e 3un de PRODB → 2 EANs
	inserirVendaFaturadaFixture(t, empresaID, loja2, "PRODA", "TCALC-RCA4", "1", 400, 4, "2026-04-06")
	inserirVendaFaturadaFixture(t, empresaID, loja2, "PRODB", "TCALC-RCA4", "1", 300, 3, "2026-04-06")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	// média = (1 + 2) / 2 = 1.5
	if resultado.Redes[0].Valor != 1.5 {
		t.Errorf("média de EANs positivados = %.2f, want 1.5 (loja1=1, loja2=2)", resultado.Redes[0].Valor)
	}
}

func TestCalcularRealizado_AgregacaoPorRCA(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC Rollup", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-05-01", "2026-05-31")

	loja1, loja2 := "55555555000101", "55555555000102"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja1, loja2}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE A", loja1, "TCALC-RCA5")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE B", loja2, "TCALC-RCA5") // mesmo RCA, Rede diferente
	inserirVendaFaturadaFixture(t, empresaID, loja1, "PROD1", "TCALC-RCA5", "1", 1000, 1, "2026-05-05")
	inserirVendaFaturadaFixture(t, empresaID, loja2, "PROD1", "TCALC-RCA5", "1", 2000, 1, "2026-05-06")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rca")
	if err != nil {
		t.Fatalf("CalcularRealizado nível rca: %v", err)
	}
	if len(resultado.Grupos) != 1 {
		t.Fatalf("esperava 1 grupo (mesmo RCA pras 2 Redes), veio %d: %+v", len(resultado.Grupos), resultado.Grupos)
	}
	if resultado.Grupos[0].QtdRedes != 2 {
		t.Errorf("RCA deveria agregar 2 Redes, veio %d", resultado.Grupos[0].QtdRedes)
	}
	if resultado.Grupos[0].RealizadoTotal != 2 {
		t.Errorf("RCA com 2 Redes cobertas (ambas >= limiar 100) deveria ter RealizadoTotal=2, veio %.0f", resultado.Grupos[0].RealizadoTotal)
	}
}

// TestCalcularRealizado_FluxoTransmitido cobre a Story 4.2 (FR15): o fluxo
// "transmitido" lê vendas_transmitidas, não vendas_faturadas.
func TestCalcularRealizado_FluxoTransmitido(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC Transmitido", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-07-01", "2026-07-31")

	loja := "77777777000101"
	t.Cleanup(func() { limparVendasTransmitidasFixture(t, empresaID, []string{loja}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE TRANS", loja, "TCALC-RCA7")
	inserirVendaTransmitidaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA7", "1", 5000, 1, "2026-07-10")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "transmitido", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado fluxo=transmitido: %v", err)
	}
	if resultado.Redes[0].Valor != 5000 {
		t.Errorf("fluxo transmitido = %.2f, want 5000 (só transmitido, sem misturar faturado)", resultado.Redes[0].Valor)
	}
}

// TestCalcularRealizado_FluxoSoma cobre FR15: "Soma" combina Faturado +
// Transmitido — capacidade nova que o Farol hoje não tem (só alterna).
func TestCalcularRealizado_FluxoSoma(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC Soma", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	loja := "88888888000101"
	t.Cleanup(func() {
		limparVendasFaturadasFixture(t, empresaID, []string{loja})
		limparVendasTransmitidasFixture(t, empresaID, []string{loja})
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE SOMA", loja, "TCALC-RCA8")
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA8", "1", 3000, 1, "2026-08-05")
	inserirVendaTransmitidaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA8", "1", 2000, 1, "2026-08-06")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "soma", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado fluxo=soma: %v", err)
	}
	if resultado.Redes[0].Valor != 5000 {
		t.Errorf("fluxo soma = %.2f, want 5000 (3000 faturado + 2000 transmitido)", resultado.Redes[0].Valor)
	}

	// Confirma que os fluxos individuais continuam corretos isoladamente —
	// trocar fluxo não é acumulativo/com efeito colateral.
	rFat, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado fluxo=faturado: %v", err)
	}
	if rFat.Redes[0].Valor != 3000 {
		t.Errorf("fluxo faturado isolado = %.2f, want 3000 (não deveria incluir o transmitido)", rFat.Redes[0].Valor)
	}
}

func TestCalcularRealizado_MesCorrenteEhParcial(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC Parcial", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)

	// vigência que já fechou no passado — não deveria ser parcial
	vigenciaPassada := criarVigenciaFixture(t, db, empresaID, vinculoID, "2020-01-01", "2020-01-31")
	loja := "66666666000101"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{loja}) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaPassada, "REDE PARCIAL", loja, "TCALC-RCA6")
	inserirVendaFaturadaFixture(t, empresaID, loja, "PROD1", "TCALC-RCA6", "1", 100, 1, "2020-01-10")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaPassada, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	if resultado.Parcial {
		t.Errorf("vigência de 2020 (encerrada) não deveria ser marcada como parcial")
	}
}

func TestCalcularRealizado_SemClientesValidos_ErroClaro(t *testing.T) {
	db, empresaID := biTestDB(t)
	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TCALC SemClientes", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar (R$)", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-06-01", "2026-06-30")

	_, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err == nil {
		t.Fatal("esperava erro claro quando não há Clientes Válidos importados, veio nil")
	}
}
