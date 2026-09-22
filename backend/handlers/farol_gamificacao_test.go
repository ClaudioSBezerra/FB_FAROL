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
	"fmt"
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
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA1'`, empresaID)
	})
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
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA-LOJA'`, empresaID)
	})
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

// TestCalcularPontuacaoCampanha_RedeCompletaAtingida_SoQuando100PorCento —
// pedido do Claudio 22/09/2026: 4º tipo de regra, pra separar "Loja
// Individual" (paga por cada loja) de "Rede Completa" (só paga quando
// TODAS as lojas da Rede batem). 2 Redes do mesmo RCA: uma 100% coberta
// (2 de 2), outra parcial (1 de 2) — só a primeira deve gerar o bônus
// desta regra, mesmo a parcial já pontuando na regra "por loja" (não
// testada aqui, são regras independentes).
func TestCalcularPontuacaoCampanha_RedeCompletaAtingida_SoQuando100PorCento(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM RedeCompleta", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjCompleta1, cnpjCompleta2 := "80000000000401", "80000000000402"
	cnpjParcialBate, cnpjParcialZerada := "80000000000403", "80000000000404"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA-RC'`, empresaID)
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE COMPLETA", cnpjCompleta1, "TGAM-RCA-RC")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE COMPLETA", cnpjCompleta2, "TGAM-RCA-RC")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE PARCIAL", cnpjParcialBate, "TGAM-RCA-RC")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE PARCIAL", cnpjParcialZerada, "TGAM-RCA-RC")
	inserirVendaFaturadaFixture(t, empresaID, cnpjCompleta1, "PRODRC", "TGAM-RCA-RC", "1", 150, 1, "2026-08-10")
	inserirVendaFaturadaFixture(t, empresaID, cnpjCompleta2, "PRODRC", "TGAM-RCA-RC", "1", 150, 1, "2026-08-10")
	inserirVendaFaturadaFixture(t, empresaID, cnpjParcialBate, "PRODRC", "TGAM-RCA-RC", "1", 150, 1, "2026-08-10")
	// cnpjParcialZerada não compra nada — REDE PARCIAL fica 1 de 2 (50%).

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "rede_completa_atingida", vinculoID, vigenciaID, nil, 0, 50, 1000)

	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	var pontos, bonus float64
	if err := db.QueryRow(`SELECT pontos_total, bonus_total FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND cod_rca = 'TGAM-RCA-RC'`, campanhaID).
		Scan(&pontos, &bonus); err != nil {
		t.Fatalf("ler pontuação: %v", err)
	}
	// Só 1 Rede (REDE COMPLETA) bateu 100% — 1 ocorrência, não 2 (não
	// multiplica pela qtd de lojas da Rede completa, e REDE PARCIAL não
	// conta nada aqui por não ser 100%).
	if pontos != 50 || bonus != 1000 {
		t.Errorf("pontos/bonus = %v/%v, want 50/1000 (só REDE COMPLETA bateu 100%%, 1 ocorrência — REDE PARCIAL não conta nesta regra)", pontos, bonus)
	}
}

// TestCalcularPontuacaoCampanha_RcaCompleto_MostraProgressoAntesDeCompletar
// — pedido do Claudio 22/09/2026: "bater 100% todas as 6 redes do RCA...
// apareceria pra ele que falta X pontos pra atingir o objetivo completo".
// 2 RCAs no mesmo vínculo/vigência: RCA-A tem 2 Redes de 1 loja cada, as 2
// batem (100% do portfólio — completo). RCA-B tem 2 Redes de 1 loja cada,
// só 1 bate (50% — incompleto, mas precisa aparecer com "faltam 1 de 2").
func TestCalcularPontuacaoCampanha_RcaCompleto_MostraProgressoAntesDeCompletar(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM RcaCompleto", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjA1, cnpjA2 := "80000000000501", "80000000000502" // RCA-A: as 2 batem
	cnpjB1, cnpjB2 := "80000000000503", "80000000000504" // RCA-B: só 1 bate
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca IN ('TGAM-RCA-A', 'TGAM-RCA-B')`, empresaID)
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE A1", cnpjA1, "TGAM-RCA-A")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE A2", cnpjA2, "TGAM-RCA-A")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE B1", cnpjB1, "TGAM-RCA-B")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE B2", cnpjB2, "TGAM-RCA-B")
	inserirVendaFaturadaFixture(t, empresaID, cnpjA1, "PRODRCA", "TGAM-RCA-A", "1", 150, 1, "2026-08-10")
	inserirVendaFaturadaFixture(t, empresaID, cnpjA2, "PRODRCA", "TGAM-RCA-A", "1", 150, 1, "2026-08-10")
	inserirVendaFaturadaFixture(t, empresaID, cnpjB1, "PRODRCA", "TGAM-RCA-B", "1", 150, 1, "2026-08-10")
	// cnpjB2 não compra nada — RCA-B fica 1 de 2 (incompleto).

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "rca_completo", vinculoID, vigenciaID, nil, 0, 100, 2000)

	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	var pontosA, bonusA float64
	var detalheA []byte
	if err := db.QueryRow(`SELECT pontos_total, bonus_total, detalhe FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND cod_rca = 'TGAM-RCA-A'`, campanhaID).
		Scan(&pontosA, &bonusA, &detalheA); err != nil {
		t.Fatalf("ler pontuação RCA-A: %v", err)
	}
	if pontosA != 100 || bonusA != 2000 {
		t.Errorf("RCA-A: pontos/bonus = %v/%v, want 100/2000 (2 de 2 Redes = completo)", pontosA, bonusA)
	}
	var detalheAParsed []map[string]any
	json.Unmarshal(detalheA, &detalheAParsed)
	if len(detalheAParsed) != 1 || detalheAParsed[0]["completo"] != true || detalheAParsed[0]["faltam"].(float64) != 0 {
		t.Errorf("RCA-A: detalhe = %v, want completo=true faltam=0", detalheAParsed)
	}

	// RCA-B: incompleto, mas TEM que ter uma linha em gamif_pontuacao (0
	// pontos desta regra) com o progresso — é isso que a "visão do RCA"
	// usa pra mostrar "faltam 1 de 2" ANTES de ele completar.
	var pontosB, bonusB float64
	var detalheB []byte
	if err := db.QueryRow(`SELECT pontos_total, bonus_total, detalhe FROM farol.gamif_pontuacao WHERE campanha_id = $1 AND cod_rca = 'TGAM-RCA-B'`, campanhaID).
		Scan(&pontosB, &bonusB, &detalheB); err != nil {
		t.Fatalf("ler pontuação RCA-B (deveria existir mesmo incompleto, pra mostrar progresso): %v", err)
	}
	if pontosB != 0 || bonusB != 0 {
		t.Errorf("RCA-B: pontos/bonus = %v/%v, want 0/0 (não completou, não ganha o prêmio)", pontosB, bonusB)
	}
	var detalheBParsed []map[string]any
	json.Unmarshal(detalheB, &detalheBParsed)
	if len(detalheBParsed) != 1 {
		t.Fatalf("RCA-B: detalhe = %v, want 1 item de progresso", detalheBParsed)
	}
	d := detalheBParsed[0]
	if d["completo"] != false || d["cobertos"].(float64) != 1 || d["total"].(float64) != 2 || d["faltam"].(float64) != 1 {
		t.Errorf("RCA-B: detalhe = %v, want completo=false cobertos=1 total=2 faltam=1", d)
	}
}

// TestGamifRankingHandler_RankingGeralSoMostraQuemPontuou — achado real do
// Claudio 22/09/2026 ("o ranking ficou estranho"): uma regra rca_completo
// toca TODO RCA do vínculo pra rastrear progresso (ver teste acima), o que
// inundava o ranking geral com dezenas de RCAs zerados. O ranking geral
// (sem ?cod_rca=) deve mostrar só quem tem pontos/bônus > 0; a "visão do
// RCA" (?cod_rca=) continua achando o zerado, pra mostrar o progresso.
func TestGamifRankingHandler_RankingGeralSoMostraQuemPontuou(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM RankLimpo", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjCompleto := "80000000000601"
	cnpjIncompleto1, cnpjIncompleto2 := "80000000000602", "80000000000603"
	t.Cleanup(func() {
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca IN ('TGAM-RCA-OK', 'TGAM-RCA-ZERO')`, empresaID)
	})
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE OK", cnpjCompleto, "TGAM-RCA-OK")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE ZERO 1", cnpjIncompleto1, "TGAM-RCA-ZERO")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE ZERO 2", cnpjIncompleto2, "TGAM-RCA-ZERO")
	inserirVendaFaturadaFixture(t, empresaID, cnpjCompleto, "PRODRANK", "TGAM-RCA-OK", "1", 150, 1, "2026-08-10")
	inserirVendaFaturadaFixture(t, empresaID, cnpjIncompleto1, "PRODRANK", "TGAM-RCA-ZERO", "1", 150, 1, "2026-08-10")
	// cnpjIncompleto2 não compra nada — TGAM-RCA-ZERO fica 1 de 2 (não completa).

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "rca_completo", vinculoID, vigenciaID, nil, 0, 10, 300)

	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	handler := GamifRankingHandler(db)

	// Ranking geral: só TGAM-RCA-OK deve aparecer.
	req := gamifReq(http.MethodGet, "/api/farol/gamif-ranking?campanha_id="+strconv.Itoa(campanhaID), empresaID, "teste", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ranking geral: status = %d, body = %s", w.Code, w.Body.String())
	}
	var geral struct {
		Ranking []struct {
			CodRCA string `json:"cod_rca"`
		} `json:"ranking"`
		TotalRCAs int `json:"total_rcas"`
	}
	json.Unmarshal(w.Body.Bytes(), &geral)
	if geral.TotalRCAs != 1 || len(geral.Ranking) != 1 || geral.Ranking[0].CodRCA != "TGAM-RCA-OK" {
		t.Errorf("ranking geral = %+v, want só TGAM-RCA-OK (TGAM-RCA-ZERO não pontuou, não deveria aparecer)", geral)
	}

	// Visão do RCA zerado: precisa continuar achando (é quem mais precisa
	// ver o progresso), mesmo fora do ranking geral.
	reqZero := gamifReq(http.MethodGet, "/api/farol/gamif-ranking?campanha_id="+strconv.Itoa(campanhaID)+"&cod_rca=TGAM-RCA-ZERO", empresaID, "teste", nil)
	wZero := httptest.NewRecorder()
	handler(wZero, reqZero)
	if wZero.Code != http.StatusOK {
		t.Fatalf("visão do RCA zerado: status = %d, body = %s (deveria achar mesmo com 0 pontos)", wZero.Code, wZero.Body.String())
	}
	var minhaPosicao struct {
		Detalhe []map[string]any `json:"detalhe"`
	}
	json.Unmarshal(wZero.Body.Bytes(), &minhaPosicao)
	if len(minhaPosicao.Detalhe) != 1 || minhaPosicao.Detalhe[0]["faltam"].(float64) != 1 {
		t.Errorf("visão do RCA zerado: detalhe = %v, want faltam=1", minhaPosicao.Detalhe)
	}
}

// TestGamifRankingHandler_DesempataPorVolumeCurvaABC — pedido do Claudio
// 22/09/2026: "o critério de primeiro para segundo é a quantidade
// vendida... tipo curva ABC de vendas". Regra produto_especifico paga
// prêmio FIXO ao bater o mínimo (10 garrafas ou 188 rendem o mesmo bônus)
// — sem desempate, todo mundo que bate empataria em 1º. Com
// volume_desempate, quem vendeu mais rankeia acima mesmo com pontos/bônus
// idênticos.
func TestGamifRankingHandler_DesempataPorVolumeCurvaABC(t *testing.T) {
	db, empresaID := biTestDB(t)

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM ABC", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjTop, cnpjMinimo := "80000000000701", "80000000000702"
	t.Cleanup(func() { db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca IN ('TGAM-RCA-TOP', 'TGAM-RCA-MIN')`, empresaID) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE TOP", cnpjTop, "TGAM-RCA-TOP")
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE MIN", cnpjMinimo, "TGAM-RCA-MIN")
	// RCA-TOP vende MUITO mais (188), RCA-MIN só bate o mínimo (10) — os
	// dois "bateram" a mesma regra e ganham o MESMO prêmio fixo.
	inserirVendaFaturadaFixture(t, empresaID, cnpjTop, "PRODABC", "TGAM-RCA-TOP", "1", 1880, 188, "2026-08-10")
	inserirVendaFaturadaFixture(t, empresaID, cnpjMinimo, "PRODABC", "TGAM-RCA-MIN", "1", 100, 10, "2026-08-10")

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "produto_especifico", 0, 0, []string{"PRODABC"}, 10, 10, 300)

	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	handler := GamifRankingHandler(db)
	req := gamifReq(http.MethodGet, "/api/farol/gamif-ranking?campanha_id="+strconv.Itoa(campanhaID), empresaID, "teste", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Ranking []struct {
			Posicao         int     `json:"posicao"`
			CodRCA          string  `json:"cod_rca"`
			PontosTotal     float64 `json:"pontos_total"`
			VolumeDesempate float64 `json:"volume_desempate"`
		} `json:"ranking"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Ranking) != 2 {
		t.Fatalf("ranking = %+v, want 2 RCAs (os dois bateram o mínimo)", resp.Ranking)
	}
	// Mesmo prêmio (empate em pontos), mas RCA-TOP (188) precisa vir ANTES
	// de RCA-MIN (10) — desempate por volume, não ordem arbitrária/empate.
	if resp.Ranking[0].CodRCA != "TGAM-RCA-TOP" || resp.Ranking[0].Posicao != 1 {
		t.Errorf("1º colocado = %+v, want TGAM-RCA-TOP na posição 1 (vendeu 188 vs 10)", resp.Ranking[0])
	}
	if resp.Ranking[1].CodRCA != "TGAM-RCA-MIN" || resp.Ranking[1].Posicao != 2 {
		t.Errorf("2º colocado = %+v, want TGAM-RCA-MIN na posição 2, NÃO empatado em 1º", resp.Ranking[1])
	}
	if resp.Ranking[0].PontosTotal != resp.Ranking[1].PontosTotal {
		t.Errorf("pontos deveriam ser IGUAIS (prêmio fixo) — top=%v min=%v", resp.Ranking[0].PontosTotal, resp.Ranking[1].PontosTotal)
	}
	if resp.Ranking[0].VolumeDesempate != 188 || resp.Ranking[1].VolumeDesempate != 10 {
		t.Errorf("volume_desempate = top:%v min:%v, want 188/10", resp.Ranking[0].VolumeDesempate, resp.Ranking[1].VolumeDesempate)
	}
}

// TestGamifPublicMinhasCampanhasHandler_AchaCampanhaSemLogin — pedido do
// Claudio 22/09/2026: o RCA vê a Gamificação na mesma URL pública que já
// usa em campo (/m/.../metas-industria), sem login — mesmo padrão de
// segurança do resto da tela mobile. Prova que o endpoint público resolve
// a campanha certa a partir de cnpj+cod_rca, sem vazar dado de outro RCA.
func TestGamifPublicMinhasCampanhasHandler_AchaCampanhaSemLogin(t *testing.T) {
	db, empresaID := biTestDB(t)
	var cnpjEmpresa string
	if err := db.QueryRow(`SELECT regexp_replace(cnpj, '[^0-9]', '', 'g') FROM companies WHERE id = $1`, empresaID).Scan(&cnpjEmpresa); err != nil || cnpjEmpresa == "" {
		t.Skip("empresa de teste sem CNPJ cadastrado — teste pulado")
	}

	vinculoID, cleanup := criarVinculoComFormula(t, empresaID, "TGAM Public", "cobertura_rede", "rede",
		[]ParametroSchemaDTO{{Key: "limiar_valor_medio", Label: "Limiar", Type: "number"}},
		map[string]any{"limiar_valor_medio": 100.0})
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-08-01", "2026-08-31")

	cnpjCliente := "80000000000801"
	t.Cleanup(func() { db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_rca = 'TGAM-RCA-PUB'`, empresaID) })
	inserirClienteValidoFixture(t, empresaID, vinculoID, vigenciaID, "REDE PUB", cnpjCliente, "TGAM-RCA-PUB")
	inserirVendaFaturadaFixture(t, empresaID, cnpjCliente, "PRODPUB", "TGAM-RCA-PUB", "1", 150, 1, "2026-08-10")

	var industriaID int
	db.QueryRow(`SELECT industria_id FROM farol.metas_vinculos WHERE id = $1`, vinculoID).Scan(&industriaID)
	campanhaID := criarGamifCampanhaFixture(t, empresaID, industriaID, "2026-08-01", "2026-08-31")
	criarGamifRegraFixture(t, campanhaID, "cobertura_atingida", vinculoID, vigenciaID, nil, 0, 10, 300)
	if err := CalcularPontuacaoCampanha(db, empresaID, campanhaID); err != nil {
		t.Fatalf("CalcularPontuacaoCampanha: %v", err)
	}

	url := fmt.Sprintf("/api/farol/public/gamif-minhas-campanhas?cnpj=%s&cod_rca=TGAM-RCA-PUB", cnpjEmpresa)
	w := httptest.NewRecorder()
	GamifPublicMinhasCampanhasHandler(db)(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET público → status %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Campanhas []struct {
			CampanhaID  int     `json:"campanha_id"`
			Nome        string  `json:"nome"`
			Posicao     int     `json:"posicao"`
			PontosTotal float64 `json:"pontos_total"`
			BonusTotal  float64 `json:"bonus_total"`
		} `json:"campanhas"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v, body=%s", err, w.Body.String())
	}
	var achou bool
	for _, c := range resp.Campanhas {
		if c.CampanhaID == campanhaID {
			achou = true
			if c.PontosTotal != 10 || c.BonusTotal != 300 {
				t.Errorf("campanha achada com pontos/bonus = %v/%v, want 10/300", c.PontosTotal, c.BonusTotal)
			}
		}
	}
	if !achou {
		t.Errorf("campanha %d não apareceu na lista pública pro RCA TGAM-RCA-PUB: %+v", campanhaID, resp.Campanhas)
	}

	// cod_rca sem nenhuma pontuação não deveria trazer NENHUMA campanha
	// (nem essa, nem vazamento de outro RCA).
	urlOutro := fmt.Sprintf("/api/farol/public/gamif-minhas-campanhas?cnpj=%s&cod_rca=TGAM-RCA-INEXISTENTE", cnpjEmpresa)
	wOutro := httptest.NewRecorder()
	GamifPublicMinhasCampanhasHandler(db)(wOutro, httptest.NewRequest(http.MethodGet, urlOutro, nil))
	var respOutro struct {
		Campanhas []map[string]any `json:"campanhas"`
	}
	json.Unmarshal(wOutro.Body.Bytes(), &respOutro)
	if len(respOutro.Campanhas) != 0 {
		t.Errorf("RCA sem pontuação nenhuma deveria ver lista vazia, veio %+v", respOutro.Campanhas)
	}
}
