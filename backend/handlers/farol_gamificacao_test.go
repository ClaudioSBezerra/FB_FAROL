package handlers

// farol_gamificacao_test.go — cobre o motor de pontuação do MVP de
// Gamificação (pedido do José Costa via Claudio 22/09/2026): pontos
// somados de Cobertura/Sortimento já existentes + o caso real que motivou
// o 3º tipo de regra (produto_especifico) — incentivo por SKU específico
// (ex: microgarrafa de vodka), não a Rede inteira.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/lib/pq"
)

func gamifReq(method, url, empresaID, userID string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, url, &buf)
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: userID, SpRole: "admin_fbtax", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func criarGamifCampanhaFixture(t *testing.T, empresaID string, industriaID int, inicio, fim string) int {
	t.Helper()
	db, _ := biTestDB(t)
	var id int
	if err := db.QueryRow(`
		INSERT INTO farol.gamif_campanhas (empresa_id, industria_id, nome, data_inicio, data_fim, created_by)
		VALUES ($1, $2, 'Campanha Teste', $3, $4, 'teste') RETURNING id
	`, empresaID, industriaID, inicio, fim).Scan(&id); err != nil {
		t.Fatalf("criar fixture de campanha: %v", err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM farol.gamif_campanhas WHERE id = $1`, id) })
	return id
}

func criarGamifRegraFixture(t *testing.T, campanhaID int, tipo string, vinculoID, vigenciaID int, codProds []string, qtdMinima, pontos, valorBonus float64) int {
	t.Helper()
	db, _ := biTestDB(t)
	var vID, vgID any
	if vinculoID != 0 {
		vID = vinculoID
	}
	if vigenciaID != 0 {
		vgID = vigenciaID
	}
	if codProds == nil {
		codProds = []string{}
	}
	var id int
	if err := db.QueryRow(`
		INSERT INTO farol.gamif_regras (campanha_id, tipo, descricao, vinculo_id, vigencia_id, cod_prods, qtd_minima, fluxo, pontos, valor_bonus)
		VALUES ($1, $2, 'regra teste', $3, $4, $5, $6, 'faturado', $7, $8) RETURNING id
	`, campanhaID, tipo, vID, vgID, pq.Array(codProds), qtdMinima, pontos, valorBonus).Scan(&id); err != nil {
		t.Fatalf("criar fixture de regra (%s): %v", tipo, err)
	}
	return id
}

// TestCalcularPontuacaoCampanha_CoberturaAtingida_SoQuemBateuOLimiar cobre o
// caso base (reaproveita o Realizado já existente): 2 Redes do mesmo RCA,
// só 1 bate o limiar de Cobertura — o RCA ganha pontos só daquela Rede, não
// das duas.
func TestCalcularPontuacaoCampanha_CoberturaAtingida_SoQuemBateuOLimiar(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM Cobertura", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjBate, cnpjNaoBate := "80000000000101", "80000000000102"
	// limparVendasFaturadasFixture só apaga cod_rca LIKE 'TCALC%' — não
	// serve pro prefixo TGAM usado aqui (achado rodando este teste várias
	// vezes: lixo de execuções antigas acumulava e fazia RCA-P2 pontuar
	// indevidamente num teste irmão). Limpeza própria por cod_rca exato.
	t.Cleanup(func() { db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA1'`, empresaID) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE BATE", cnpjBate, "TGAM-RCA1")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE NAO BATE", cnpjNaoBate, "TGAM-RCA1")
	inserirVendaFaturadaFixture(t, empresaID, cnpjBate, "PRODG1", "TGAM-RCA1", "1", 500, 1, "2026-08-10")
	inserirVendaFaturadaFixture(t, empresaID, cnpjNaoBate, "PRODG1", "TGAM-RCA1", "1", 10, 1, "2026-08-10")

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "cobertura_atingida", vinculoID, vigenciaID, nil, 0, 10, 300)

	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	var pontos, bonus float64
	if err := db.QueryRow(`SELECT pontos_total, bonus_total FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND cod_rca = 'TGAM-RCA1'`, campanhaID).
		Scan(&pontos, &bonus); err != nil {
		t.Fatalf("ler pontuação: %v", err)
	}
	if pontos != 10 || bonus != 300 {
		t.Errorf("pontos/bonus = %v/%v, want 10/300 (só 1 das 2 Redes bateu o limiar)", pontos, bonus)
	}
}

// TestCalcularPontuacaoCampanha_CoberturaAtingida_PremiaPorLojaNaoPelaMediaDaRede
// — achado real do Claudio 22/09/2026: "cada RCA é responsável pelo cliente
// e pela Rede dele... esse acompanhamento tem que ser no último nível".
// Uma Rede com 2 lojas onde só 1 bate o limiar tem MÉDIA abaixo do limiar
// (Rede.Atingiu = false) — mas aquela loja específica bateu a própria
// meta, e o RCA não controla a outra loja isoladamente. A regra tem que
// premiar a loja que bateu, mesmo com a Rede como um todo não batendo.
func TestCalcularPontuacaoCampanha_CoberturaAtingida_PremiaPorLojaNaoPelaMediaDaRede(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM Loja", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjBateSozinha, cnpjZerada := "80000000000301", "80000000000302"
	t.Cleanup(func() { db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA-LOJA'`, empresaID) })
	// MESMA Rede (cod_princ "REDE MISTA") pras 2 lojas — a média das duas
	// fica em 75 (150+0)/2, abaixo do limiar 100, então a Rede como um
	// todo NÃO atinge — só a loja cnpjBateSozinha bate individualmente.
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE MISTA", cnpjBateSozinha, "TGAM-RCA-LOJA")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE MISTA", cnpjZerada, "TGAM-RCA-LOJA")
	inserirVendaFaturadaFixture(t, empresaID, cnpjBateSozinha, "PRODLOJA", "TGAM-RCA-LOJA", "1", 150, 1, "2026-08-10")
	// cnpjZerada não compra nada — Valor = 0 no Realizado.

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "cobertura_atingida", vinculoID, vigenciaID, nil, 0, 10, 300)

	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	// Confirma a premissa: a Rede como um todo NÃO atinge (média 75 < 100).
	realizado, err := obterOuCongelarRealizado(db, empresaID, vinculoID, vigenciaID, "faturado", "rede")
	if err != nil {
		t.Fatalf("obterOuCongelarRealizado: %v", err)
	}
	if len(realizado.Redes) != 1 || realizado.Redes[0].Atingiu {
		t.Fatalf("premissa do teste furou: Rede.Atingiu deveria ser false (média 75 < 100), Redes=%+v", realizado.Redes)
	}

	var pontos, bonus float64
	if err := db.QueryRow(`SELECT pontos_total, bonus_total FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND cod_rca = 'TGAM-RCA-LOJA'`, campanhaID).
		Scan(&pontos, &bonus); err != nil {
		t.Fatalf("ler pontuação (a Rede não atingiu, mas 1 loja sim — RCA deveria pontuar mesmo assim): %v", err)
	}
	if pontos != 10 || bonus != 300 {
		t.Errorf("pontos/bonus = %v/%v, want 10/300 (premia a 1 loja que bateu, mesmo a Rede como um todo não batendo)", pontos, bonus)
	}
}

// TestCalcularPontuacaoCampanha_ProdutoEspecifico_SoQuemBateuAQuantidade —
// caso real que motivou o 3º tipo de regra (José Costa/vodka): incentivo
// por SKU específico, não pela Rede/Cobertura inteira. RCA1 vende o
// suficiente do produto-alvo, RCA2 vende pouco — só RCA1 pontua.
func TestCalcularPontuacaoCampanha_ProdutoEspecifico_SoQuemBateuAQuantidade(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM Produto", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpj1, cnpj2 := "80000000000201", "80000000000202"
	// Mesmo achado do teste acima: limparVendasFaturadasFixture não cobre
	// o prefixo TGAM — limpeza própria pelos 2 cod_rca exatos deste teste.
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca IN ('TGAM-RCA-P1', 'TGAM-RCA-P2')`, empresaID)
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE P1", cnpj1, "TGAM-RCA-P1")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE P2", cnpj2, "TGAM-RCA-P2")

	// Produto-alvo (a "microgarrafa"): RCA-P1 vende 15 unidades (bate o
	// mínimo de 10), RCA-P2 vende só 3 (não bate). Um produto FORA da
	// campanha (PRODOUTRO) não deveria contar pra ninguém.
	inserirVendaFaturadaFixture(t, empresaID, cnpj1, "MICROGARRAFA", "TGAM-RCA-P1", "1", 150, 15, "2026-08-15")
	inserirVendaFaturadaFixture(t, empresaID, cnpj2, "MICROGARRAFA", "TGAM-RCA-P2", "1", 30, 3, "2026-08-15")
	inserirVendaFaturadaFixture(t, empresaID, cnpj1, "PRODOUTRO", "TGAM-RCA-P1", "1", 1000, 100, "2026-08-15")

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "produto_especifico", 0, 0, []string{"MICROGARRAFA"}, 10, 50, 300)

	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	var pontosP1, bonusP1 float64
	if err := db.QueryRow(`SELECT pontos_total, bonus_total FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND cod_rca = 'TGAM-RCA-P1'`, campanhaID).
		Scan(&pontosP1, &bonusP1); err != nil {
		t.Fatalf("ler pontuação RCA-P1: %v", err)
	}
	if pontosP1 != 50 || bonusP1 != 300 {
		t.Errorf("RCA-P1: pontos/bonus = %v/%v, want 50/300 (vendeu 15 >= mínimo 10)", pontosP1, bonusP1)
	}

	var countP2 int
	db.QueryRow(`SELECT count(*) FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND cod_rca = 'TGAM-RCA-P2'`, campanhaID).Scan(&countP2)
	if countP2 != 0 {
		t.Errorf("RCA-P2 não deveria pontuar (vendeu só 3, mínimo é 10) — achou %d linha(s)", countP2)
	}
}

// TestGamifRankingHandler_VisaoDoRCA_NaoVazaNomeDeQuemEstaNaFrente — decisão
// do Claudio 22/09/2026: o RCA vê SÓ a própria posição, nunca quem está na
// frente. Testa direto a query/lógica de agregação usada pelo handler
// (via inserção manual de gamif_pontuacao, sem precisar rodar o motor
// inteiro de novo).
func TestGamifRanking_VisaoDoRCA_SoAPropriaPosicao(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM Ranking", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")

	inserirPontuacao := func(codRCA, nomeRCA string, pontos float64) {
		db.Exec(`INSERT INTO farol.gamif_pontuacao (empresa_id, campanha_id, cod_rca, nome_rca, pontos_total, bonus_total, detalhe) VALUES ($1,$2,$3,$4,$5,0,'[]')`,
			empresaID, campanhaID, codRCA, nomeRCA, pontos)
	}
	inserirPontuacao("RCA-1", "Primeiro Lugar", 100)
	inserirPontuacao("RCA-2", "Segundo Lugar", 50)
	inserirPontuacao("RCA-3", "Terceiro Lugar", 10)

	rows, err := db.Query(`
		SELECT cod_rca, nome_rca, pontos_total, RANK() OVER (ORDER BY pontos_total DESC) AS posicao, COUNT(*) OVER () AS total
		FROM farol.gamif_pontuacao WHERE campanha_id = $1 ORDER BY pontos_total DESC
	`, campanhaID)
	if err != nil {
		t.Fatalf("query ranking: %v", err)
	}
	defer rows.Close()
	posicoes := map[string]int{}
	var total int
	for rows.Next() {
		var codRCA, nomeRCA string
		var pontos float64
		var pos int
		if err := rows.Scan(&codRCA, &nomeRCA, &pontos, &pos, &total); err != nil {
			t.Fatalf("scan: %v", err)
		}
		posicoes[codRCA] = pos
	}
	if posicoes["RCA-2"] != 2 {
		t.Errorf("posição de RCA-2 = %d, want 2", posicoes["RCA-2"])
	}
	if total != 3 {
		t.Errorf("total_rcas = %d, want 3", total)
	}
	// A garantia de "não vaza nome de quem está na frente" é estrutural no
	// handler (GamifMinhaPosicaoResponse não tem campo pra outros RCAs) —
	// aqui confirmamos que a posição em si está correta, base do que o
	// handler expõe.
	_ = json.RawMessage(nil)
}

// TestGamifRegraItemHandler_PUT_EditaValoresExistentes — achado real do
// Claudio 22/09/2026: "uma vez criada não estou conseguindo editar para
// ajustar" — o handler só tinha DELETE, faltava PUT. Cobre o caso de uso
// real: criar uma regra com pontos/bônus errados e corrigir sem precisar
// excluir e recriar (perdendo o id/histórico).
func TestGamifRegraItemHandler_PUT_EditaValoresExistentes(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM Edit", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")
	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	regraID := criarGamifRegraFixture(t, campanhaID, "cobertura_atingida", vinculoID, vigenciaID, nil, 0, 10, 300)

	handler := GamifRegraItemHandler(db)
	req := gamifReq(http.MethodPut, "/api/farol/gamif-regras/x", empresaID, "teste", GamifRegraRequest{
		Tipo: "cobertura_atingida", Descricao: "ajustada", VinculoID: vinculoID, VigenciaID: vigenciaID,
		Pontos: 25, ValorBonus: 450,
	})
	// pathSegment lê o path literal, não usa mux — precisa do id de verdade na URL.
	req.URL.Path = "/api/farol/gamif-regras/" + strconv.Itoa(regraID)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", w.Code, w.Body.String())
	}

	var descricao string
	var pontos, bonus float64
	if err := db.QueryRow(`SELECT descricao, pontos, valor_bonus FROM farol.gamif_regras WHERE id = $1`, regraID).
		Scan(&descricao, &pontos, &bonus); err != nil {
		t.Fatalf("ler regra editada: %v", err)
	}
	if descricao != "ajustada" || pontos != 25 || bonus != 450 {
		t.Errorf("regra após PUT = (%q, %v, %v), want (ajustada, 25, 450)", descricao, pontos, bonus)
	}
}

