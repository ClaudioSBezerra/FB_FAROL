package handlers

// farol_metas_calculo_numerica_test.go — cobre o motor de apuração da
// Numérica (Épico 7 addendum, ver epics.md Story 7.4/7.5): Cobertura por
// Classificação PDV, Sortimento/PPA só A e B, bimestre móvel (FR14a).

import (
	"testing"
)

// inserirClienteNumericaFixture insere um CNPJ na lista de Clientes
// Numéricas (farol.metas_clientes_numericas, migration 248) — formato
// diferente de Clientes Válidos: sem cod_princ, com Classificação PDV.
func inserirClienteNumericaFixture(t *testing.T, empresaID string, vinculoID, vigenciaID int, cnpj, classificacao string) {
	t.Helper()
	db, _ := biTestDB(t)
	_, err := db.Exec(`
		INSERT INTO farol.metas_clientes_numericas (empresa_id, vinculo_id, vigencia_id, cnpj, classificacao_pdv, cod_ggv, cod_crv, cod_rca)
		VALUES ($1, $2, $3, $4, $5, 'TNUM-GGV', 'TNUM-CRV', 'TCALC-NUM-RCA')
	`, empresaID, vinculoID, vigenciaID, cnpj, classificacao)
	if err != nil {
		t.Fatalf("inserir fixture de cliente numérica: %v", err)
	}
}

// inserirPPAFixture insere um mapeamento cod_prod → PPA (farol.metas_ppas).
func inserirPPAFixture(t *testing.T, empresaID string, vinculoID, vigenciaID int, codProd, ppaNome string) {
	t.Helper()
	db, _ := biTestDB(t)
	_, err := db.Exec(`
		INSERT INTO farol.metas_ppas (empresa_id, vinculo_id, vigencia_id, cod_prod, ppa_nome)
		VALUES ($1, $2, $3, $4, $5)
	`, empresaID, vinculoID, vigenciaID, codProd, ppaNome)
	if err != nil {
		t.Fatalf("inserir fixture de PPA: %v", err)
	}
}

// marcarJanelaBimestreMovel troca a janela_apuracao do Tipo de Métrica do
// vínculo pra 'bimestre_movel' — criarVinculoComFormula cria com o default
// 'mes_fechado', igual Cobertura/Sortimento por Rede.
func marcarJanelaBimestreMovel(t *testing.T, empresaID string, vinculoID int) {
	t.Helper()
	db, _ := biTestDB(t)
	_, err := db.Exec(`
		UPDATE farol.tipos_metrica SET janela_apuracao = 'bimestre_movel'
		WHERE id = (SELECT tipo_metrica_id FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2)
	`, vinculoID, empresaID)
	if err != nil {
		t.Fatalf("marcar janela bimestre móvel: %v", err)
	}
}

// TestCalcularRealizado_CoberturaNumerica_LimiarPorClassificacao cobre o
// FR25: 3 clientes, 1 por Classificação, cada um com limiar diferente
// (Num. A=R$100, Num. B=R$50, Num. C=R$15) — mesma tabela confirmada nas 2
// planilhas reais da JC (HC e Foods, 2026-09-29).
func TestCalcularRealizado_CoberturaNumerica_LimiarPorClassificacao(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TNUM Cobertura", "cobertura_numerica", "cliente",
		[]ParametroSchemaDTO{
			{Key: "limiar_num_a", Label: "Limiar A", Type: "number"},
			{Key: "limiar_num_b", Label: "Limiar B", Type: "number"},
			{Key: "limiar_num_c", Label: "Limiar C", Type: "number"},
		},
		map[string]any{"limiar_num_a": 100.0, "limiar_num_b": 50.0, "limiar_num_c": 15.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-09-01", "2026-09-30")

	cliA, cliB, cliC := "20000000000101", "20000000000102", "20000000000103"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cliA, cliB, cliC}) })

	inserirClienteNumericaFixture(t, empresaID, vinculoID, vigenciaID, cliA, "Num. A")
	inserirClienteNumericaFixture(t, empresaID, vinculoID, vigenciaID, cliB, "Num. B")
	inserirClienteNumericaFixture(t, empresaID, vinculoID, vigenciaID, cliC, "Num. C")

	// A compra R$99 (abaixo do limiar 100) → não coberto.
	inserirVendaFaturadaFixture(t, empresaID, cliA, "PROD1", "TCALC-NUM-RCA", "1", 99, 1, "2026-09-10")
	// B compra R$50 (exatamente o limiar) → coberto (>=).
	inserirVendaFaturadaFixture(t, empresaID, cliB, "PROD1", "TCALC-NUM-RCA", "1", 50, 1, "2026-09-10")
	// C compra R$20 (acima do limiar 15) → coberto.
	inserirVendaFaturadaFixture(t, empresaID, cliC, "PROD1", "TCALC-NUM-RCA", "1", 20, 1, "2026-09-10")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	if len(resultado.Redes) != 3 {
		t.Fatalf("len(Redes) = %d, want 3 (1 pseudo-Rede por cliente)", len(resultado.Redes))
	}
	porCNPJ := map[string]RealizadoRede{}
	for _, r := range resultado.Redes {
		porCNPJ[r.CodPrinc] = r
	}
	if r := porCNPJ[cliA]; r.Atingiu || r.Objetivo != 100 {
		t.Errorf("cliente A: Atingiu=%v Objetivo=%.2f, want Atingiu=false Objetivo=100 (R$99 < limiar R$100)", r.Atingiu, r.Objetivo)
	}
	if r := porCNPJ[cliB]; !r.Atingiu || r.Objetivo != 50 {
		t.Errorf("cliente B: Atingiu=%v Objetivo=%.2f, want Atingiu=true Objetivo=50 (R$50 >= limiar R$50)", r.Atingiu, r.Objetivo)
	}
	if r := porCNPJ[cliC]; !r.Atingiu || r.Objetivo != 15 {
		t.Errorf("cliente C: Atingiu=%v Objetivo=%.2f, want Atingiu=true Objetivo=15 (R$20 >= limiar R$15)", r.Atingiu, r.Objetivo)
	}
	// RealizadoTotal = contagem de clientes cobertos (B e C) — mesmo padrão
	// de "cobertura_rede" (FR25/CalcularRealizadoComPeriodo).
	if resultado.RealizadoTotal != 2 {
		t.Errorf("RealizadoTotal = %.0f, want 2 (B e C cobertos, A não)", resultado.RealizadoTotal)
	}
}

// TestCalcularRealizado_SortimentoNumericaPPA_SoClassificacaoAeB cobre o
// FR26: Classificação C fica de fora do Sortimento/PPA mesmo comprando;
// PPA agrupa vários cod_prod (2 itens da MESMA família = 1 PPA só); teto
// limita a contagem mesmo com mais PPAs comprados.
func TestCalcularRealizado_SortimentoNumericaPPA_SoClassificacaoAeB(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TNUM Sortimento", "sortimento_numerica_ppa", "cliente",
		[]ParametroSchemaDTO{{Key: "teto_ppas", Label: "Teto PPAs", Type: "integer"}},
		map[string]any{"teto_ppas": 2.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-09-01", "2026-09-30")

	cliA, cliC := "20000000000201", "20000000000202"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cliA, cliC}) })

	inserirClienteNumericaFixture(t, empresaID, vinculoID, vigenciaID, cliA, "Num. A")
	inserirClienteNumericaFixture(t, empresaID, vinculoID, vigenciaID, cliC, "Num. C") // não deve entrar no resultado

	// 3 PPAs cadastrados; PPA-AMACIANTES tem 2 cod_prod (2 itens da mesma família).
	inserirPPAFixture(t, empresaID, vinculoID, vigenciaID, "PROD-AMAC-1", "AMACIANTES")
	inserirPPAFixture(t, empresaID, vinculoID, vigenciaID, "PROD-AMAC-2", "AMACIANTES")
	inserirPPAFixture(t, empresaID, vinculoID, vigenciaID, "PROD-SABAO", "SABAO EM PO")
	inserirPPAFixture(t, empresaID, vinculoID, vigenciaID, "PROD-DETER", "DETERGENTE")

	// Cliente A (Num. A): compra os 2 itens de AMACIANTES (mesma família →
	// 1 PPA só) + SABAO EM PO + DETERGENTE = 3 PPAs distintos, mas o teto é
	// 2 → resultado tem que ficar em 2, não 3.
	inserirVendaFaturadaFixture(t, empresaID, cliA, "PROD-AMAC-1", "TCALC-NUM-RCA", "1", 30, 5, "2026-09-05")
	inserirVendaFaturadaFixture(t, empresaID, cliA, "PROD-AMAC-2", "TCALC-NUM-RCA", "1", 30, 5, "2026-09-06")
	inserirVendaFaturadaFixture(t, empresaID, cliA, "PROD-SABAO", "TCALC-NUM-RCA", "1", 40, 5, "2026-09-07")
	inserirVendaFaturadaFixture(t, empresaID, cliA, "PROD-DETER", "TCALC-NUM-RCA", "1", 40, 5, "2026-09-08")

	// Cliente C (Num. C) compra bastante também, mas não deve nem aparecer
	// no resultado — Classificação C fica fora do Sortimento (FR26).
	inserirVendaFaturadaFixture(t, empresaID, cliC, "PROD-SABAO", "TCALC-NUM-RCA", "1", 40, 5, "2026-09-07")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	if len(resultado.Redes) != 1 {
		t.Fatalf("len(Redes) = %d, want 1 (só cliente A — Classificação C não entra no Sortimento)", len(resultado.Redes))
	}
	r := resultado.Redes[0]
	if r.CodPrinc != cliA {
		t.Fatalf("cliente retornado = %s, want %s (cliente C não deveria aparecer)", r.CodPrinc, cliA)
	}
	if r.Valor != 2 {
		t.Errorf("Valor (qtd PPAs) = %.0f, want 2 (3 PPAs comprados, capados no teto=2)", r.Valor)
	}
	if !r.Atingiu {
		t.Errorf("Atingiu = false, want true (2 PPAs >= teto 2)")
	}
}

// TestCalcularRealizado_Numerica_BimestreMovel cobre o FR14a: uma venda do
// mês ANTERIOR à vigência entra na apuração (janela = mês corrente + mês
// anterior), o que não aconteceria numa vigência 'mes_fechado' comum.
func TestCalcularRealizado_Numerica_BimestreMovel(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TNUM Bimestre", "cobertura_numerica", "cliente",
		[]ParametroSchemaDTO{
			{Key: "limiar_num_a", Label: "Limiar A", Type: "number"},
			{Key: "limiar_num_b", Label: "Limiar B", Type: "number"},
			{Key: "limiar_num_c", Label: "Limiar C", Type: "number"},
		},
		map[string]any{"limiar_num_a": 100.0, "limiar_num_b": 50.0, "limiar_num_c": 15.0})
	t.Cleanup(cleanup)
	marcarJanelaBimestreMovel(t, empresaID, vinculoID)
	// Vigência de Setembro/2026 — bimestre móvel = Agosto+Setembro.
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-09-01", "2026-09-30")

	cli := "20000000000301"
	t.Cleanup(func() { limparVendasFaturadasFixture(t, empresaID, []string{cli}) })
	inserirClienteNumericaFixture(t, empresaID, vinculoID, vigenciaID, cli, "Num. C")

	// Metade da compra em Agosto (mês anterior), metade em Setembro — só
	// soma R$15 (limiar exato) se o bimestre móvel realmente juntar os 2
	// meses; uma janela de mês fechado (só Setembro) veria R$7 e não cobriria.
	inserirVendaFaturadaFixture(t, empresaID, cli, "PROD1", "TCALC-NUM-RCA", "1", 8, 1, "2026-08-20")
	inserirVendaFaturadaFixture(t, empresaID, cli, "PROD1", "TCALC-NUM-RCA", "1", 7, 1, "2026-09-05")

	resultado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado: %v", err)
	}
	if len(resultado.Redes) != 1 {
		t.Fatalf("len(Redes) = %d, want 1", len(resultado.Redes))
	}
	r := resultado.Redes[0]
	if r.Valor != 15 {
		t.Errorf("Valor = %.2f, want 15 (R$8 de Agosto + R$7 de Setembro — bimestre móvel precisa somar os 2 meses)", r.Valor)
	}
	if !r.Atingiu {
		t.Errorf("Atingiu = false, want true (R$15 >= limiar Num.C R$15) — bimestre móvel não juntou os meses corretamente")
	}
}

// TestCalcularRealizado_CoberturaNumerica_FluxoSoma cobre a Story 7.8 —
// visão "Faturado + Emitido" pedida sem condicional pela documentação
// final do Heverton (29/09/2026). Cliente só bate o limiar somando os 2
// fluxos: nem Faturado nem Transmitido isolados chegam lá.
func TestCalcularRealizado_CoberturaNumerica_FluxoSoma(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TNUM Soma", "cobertura_numerica", "cliente",
		[]ParametroSchemaDTO{
			{Key: "limiar_num_a", Label: "Limiar A", Type: "number"},
			{Key: "limiar_num_b", Label: "Limiar B", Type: "number"},
			{Key: "limiar_num_c", Label: "Limiar C", Type: "number"},
		},
		map[string]any{"limiar_num_a": 100.0, "limiar_num_b": 50.0, "limiar_num_c": 15.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-09-01", "2026-09-30")

	cli := "20000000000401"
	t.Cleanup(func() {
		limparVendasFaturadasFixture(t, empresaID, []string{cli})
		limparVendasTransmitidasFixture(t, empresaID, []string{cli})
	})
	inserirClienteNumericaFixture(t, empresaID, vinculoID, vigenciaID, cli, "Num. A")

	// R$60 faturado + R$50 transmitido = R$110 — só bate o limiar R$100
	// somando os 2; nenhum dos 2 isolados chega lá.
	inserirVendaFaturadaFixture(t, empresaID, cli, "PROD1", "TCALC-NUM-RCA", "1", 60, 1, "2026-09-10")
	inserirVendaTransmitidaFixture(t, empresaID, cli, "PROD1", "TCALC-NUM-RCA", "1", 50, 1, "2026-09-12")

	faturado, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado(faturado): %v", err)
	}
	if r := faturado.Redes[0]; r.Valor != 60 || r.Atingiu {
		t.Errorf("faturado isolado: Valor=%.2f Atingiu=%v, want Valor=60 Atingiu=false", r.Valor, r.Atingiu)
	}

	transmitido, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "transmitido", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado(transmitido): %v", err)
	}
	if r := transmitido.Redes[0]; r.Valor != 50 || r.Atingiu {
		t.Errorf("transmitido isolado: Valor=%.2f Atingiu=%v, want Valor=50 Atingiu=false", r.Valor, r.Atingiu)
	}

	soma, err := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, "soma", "rede")
	if err != nil {
		t.Fatalf("CalcularRealizado(soma): %v", err)
	}
	if len(soma.Redes) != 1 {
		t.Fatalf("len(Redes) = %d, want 1", len(soma.Redes))
	}
	if r := soma.Redes[0]; r.Valor != 110 || !r.Atingiu {
		t.Errorf("soma: Valor=%.2f Atingiu=%v, want Valor=110 Atingiu=true (R$60 faturado + R$50 transmitido >= limiar R$100)", r.Valor, r.Atingiu)
	}
}
