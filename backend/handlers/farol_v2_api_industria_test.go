package handlers

// farol_v2_api_industria_test.go — cobre resolveIndustriaFilter, o cross-filter
// "Indústria" que traduz farol.industrias/industria_fornecedores pro filtro
// cod_fornec já existente (ver comentário grande em farol_v2_api.go). Testes
// de integração — precisam de banco real (biTestDB, pula sem DATABASE_URL).

import (
	"strconv"
	"testing"
	"time"
)

// TestFarolV2Cards_FiltroIndustria_DedupCliente — ponta a ponta pelo
// handler HTTP real: um cliente que compra de 2 cod_fornec MAPEADOS PRA
// MESMA indústria não pode ser contado duas vezes em positivados. Essa é a
// garantia central do desenho "caminho ao vivo" (ver comentário grande em
// resolveIndustriaFilter): cod_fornec não faz parte do grão pré-agregado de
// V03, então o filtro sempre cai no scan direto de vendas_faturadas — uma
// ÚNICA passada com COUNT(DISTINCT cnpj) sobre os dois cod_fornec juntos.
func TestFarolV2Cards_FiltroIndustria_DedupCliente(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2IND DEDUP E2E"
	gerente := "T2APIGER"
	cnpj := "11111111000100"
	cod1, cod2 := "T2API01", "T2API02"
	data := mustParseData(t, "2021-03-10")

	db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
	db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_gerente = $2`, empresaID, gerente)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_gerente = $2`, empresaID, gerente)
	})

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	for _, cod := range []string{cod1, cod2} {
		if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
			empresaID, industriaID, cod); err != nil {
			t.Fatalf("vincular %s: %v", cod, err)
		}
	}

	// MESMO cliente (cnpj), MESMO gerente, cod_fornec DIFERENTES — os dois
	// mapeados pra mesma indústria acima.
	if _, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_gerente, cod_fornec, cod_cli, cnpj, qt, pvenda, tipo_venda)
		VALUES ($1,$2,$3,$4,'CLI1',$5,1,1000,'1'), ($1,$2,$3,$6,'CLI1',$5,1,500,'1')
	`, empresaID, data, gerente, cod1, cnpj, cod2); err != nil {
		t.Fatalf("insert vendas_faturadas: %v", err)
	}

	dataStr := data.Format("2006-01-02")
	url := "/api/v2/farol/cards?view=V03&fluxo=faturado&ref_inicio=" + dataStr + "&ref_fim=" + dataStr +
		"&cod_industria=" + strconv.Itoa(industriaID)
	resp := cardsGet(t, db, url, empresaID)

	var card *cardItem
	for i := range resp.Cards {
		if resp.Cards[i].Key == gerente {
			card = &resp.Cards[i]
		}
	}
	if card == nil {
		t.Fatalf("card do gerente %s não apareceu (filtro cod_industria não retornou nada?): %+v", gerente, resp.Cards)
	}
	if card.ValorAtual != 1500 {
		t.Errorf("ValorAtual = %v, want 1500 (soma dos dois cod_fornec da mesma indústria)", card.ValorAtual)
	}
	if card.Positivados != 1 {
		t.Errorf("Positivados = %d, want 1 — cliente único contado 2x (uma vez por cod_fornec) é a duplicidade que este desenho existe pra evitar", card.Positivados)
	}
}

// TestFarolV2Cards_FiltroIndustria_MesCompletoIgnoraAggCorrompida — o achado
// mais sério desta rodada: num range de MÊS CHEIO (não "curto/parcial" como
// o teste acima), fetchCards tentaria usar agg_fat_v01_l1_mes (grão
// cod_fornec×cod_gerente) como atalho pro filtro cruzado. Essa tabela SOMA/
// MEDEIA positivados pré-computados POR fornecedor — errado quando 2+
// fornecedores (a norma pro filtro "Indústria") têm cliente em comum. O
// guard `fornecMultiValor` em pickAggForCrossFilter tem que recusar essa
// tabela sempre que 2+ cod_fornec estão em jogo, forçando o scan ao vivo
// (correto) mesmo que isso custe mais lento. Prova isso inserindo uma linha
// CORROMPIDA de propósito na tabela agg (positivados=99) — se o guard
// falhar, o teste pega o 99 (ou uma média dele) em vez do 1 real.
func TestFarolV2Cards_FiltroIndustria_MesCompletoIgnoraAggCorrompida(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2IND MES CHEIO"
	gerente := "T2APIGERMES"
	cnpj := "22222222000100"
	cod1, cod2 := "T2AMES01", "T2AMES02"
	// Ano precisa cair numa partição existente de agg_fat_v01_l1_mes (RANGE
	// por ano; só 2024-2027 existem hoje).
	ano, mes := 2026, 4
	dataIni := mustParseData(t, "2026-04-01")
	dataFim := mustParseData(t, "2026-04-30")

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_gerente = $2`, empresaID, gerente)
		db.Exec(`DELETE FROM farol.agg_fat_v01_l1_mes WHERE empresa_id = $1 AND cod_gerente = $2`, empresaID, gerente)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	for _, cod := range []string{cod1, cod2} {
		if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
			empresaID, industriaID, cod); err != nil {
			t.Fatalf("vincular %s: %v", cod, err)
		}
	}

	// Dado REAL (a verdade): mesmo cliente comprando dos dois fornecedores.
	if _, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_gerente, cod_fornec, cod_cli, cnpj, qt, pvenda, tipo_venda)
		VALUES ($1,$2,$3,$4,'CLI1',$5,1,1000,'1'), ($1,$2,$3,$6,'CLI1',$5,1,500,'1')
	`, empresaID, dataIni, gerente, cod1, cnpj, cod2); err != nil {
		t.Fatalf("insert vendas_faturadas: %v", err)
	}

	// Dado CORROMPIDO de propósito na agg: positivados=99 por fornecedor. Se
	// o guard falhar e esta tabela for usada, o resultado vaza esse número
	// (ou uma média/soma dele) em vez do 1 real.
	for _, cod := range []string{cod1, cod2} {
		if _, err := db.Exec(`
			INSERT INTO farol.agg_fat_v01_l1_mes (empresa_id, ano, mes, cod_fornec, cod_gerente, nome_gerente, positivados, pvenda)
			VALUES ($1,$2,$3,$4,$5,'GERENTE TESTE',99,700)
		`, empresaID, ano, mes, cod, gerente); err != nil {
			t.Fatalf("insert agg corrompida %s: %v", cod, err)
		}
	}

	dataStr := func(d time.Time) string { return d.Format("2006-01-02") }
	url := "/api/v2/farol/cards?view=V03&fluxo=faturado&ref_inicio=" + dataStr(dataIni) + "&ref_fim=" + dataStr(dataFim) +
		"&cod_industria=" + strconv.Itoa(industriaID)
	resp := cardsGet(t, db, url, empresaID)

	var card *cardItem
	for i := range resp.Cards {
		if resp.Cards[i].Key == gerente {
			card = &resp.Cards[i]
		}
	}
	if card == nil {
		t.Fatalf("card do gerente %s não apareceu: %+v", gerente, resp.Cards)
	}
	if card.Positivados == 99 || card.Positivados >= 90 {
		t.Fatalf("Positivados = %d — vazou o valor CORROMPIDO da tabela agg (guard fornecMultiValor falhou)", card.Positivados)
	}
	if card.Positivados != 1 {
		t.Errorf("Positivados = %d, want 1 (cliente único, deduplicado pelo scan ao vivo)", card.Positivados)
	}
	if card.ValorAtual != 1500 {
		t.Errorf("ValorAtual = %v, want 1500 (dado real do scan, não os 700+700 da agg corrompida)", card.ValorAtual)
	}
}

func TestResolveIndustriaFilter(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2IND MULTI FORNEC"
	db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
	t.Cleanup(func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
	})

	var industriaID int
	if err := db.QueryRow(`
		INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id
	`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria de teste: %v", err)
	}
	for _, cod := range []string{"TV2F001", "TV2F002"} {
		if _, err := db.Exec(`
			INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)
		`, empresaID, industriaID, cod); err != nil {
			t.Fatalf("vincular cod_fornec %s: %v", cod, err)
		}
	}
	industriaIDStr := strconv.Itoa(industriaID)

	t.Run("resolve pros cod_fornec mapeados", func(t *testing.T) {
		filters := multiFilters{}
		resolveIndustriaFilter(db, empresaID, industriaIDStr, filters)
		got := map[string]bool{}
		for _, c := range filters["cod_fornec"] {
			got[c] = true
		}
		if !got["TV2F001"] || !got["TV2F002"] || len(got) != 2 {
			t.Errorf("cod_fornec = %v, want exatamente [TV2F001 TV2F002]", filters["cod_fornec"])
		}
	})

	t.Run("funde com cod_fornec já filtrado manualmente (união)", func(t *testing.T) {
		filters := multiFilters{"cod_fornec": {"OUTRO_MANUAL"}}
		resolveIndustriaFilter(db, empresaID, industriaIDStr, filters)
		got := map[string]bool{}
		for _, c := range filters["cod_fornec"] {
			got[c] = true
		}
		if !got["OUTRO_MANUAL"] || !got["TV2F001"] || !got["TV2F002"] {
			t.Errorf("esperava união com o filtro manual, veio %v", filters["cod_fornec"])
		}
	})

	t.Run("indústria sem fornecedor mapeado falha fechado (sentinela, não vazio)", func(t *testing.T) {
		nomeVazia := "TV2IND SEM FORNEC"
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nomeVazia)
		defer db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nomeVazia)
		var idVazia int
		if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nomeVazia).Scan(&idVazia); err != nil {
			t.Fatalf("criar indústria vazia: %v", err)
		}

		filters := multiFilters{}
		resolveIndustriaFilter(db, empresaID, strconv.Itoa(idVazia), filters)
		if len(filters["cod_fornec"]) == 0 {
			t.Fatal("esperava um sentinela que não bate com nada real, veio filtro vazio (mostraria tudo, não nada)")
		}
		for _, real := range []string{"TV2F001", "TV2F002"} {
			if filters["cod_fornec"][0] == real {
				t.Errorf("sentinela não pode coincidir com um cod_fornec real: %v", filters["cod_fornec"])
			}
		}
	})

	t.Run("id não-numérico falha fechado", func(t *testing.T) {
		filters := multiFilters{}
		resolveIndustriaFilter(db, empresaID, "abacate", filters)
		if len(filters["cod_fornec"]) == 0 {
			t.Fatal("esperava sentinela pra ID inválido, veio filtro vazio")
		}
	})

	t.Run("vazio não mexe no filtro", func(t *testing.T) {
		filters := multiFilters{}
		resolveIndustriaFilter(db, empresaID, "", filters)
		if _, ok := filters["cod_fornec"]; ok {
			t.Errorf("string vazia não deveria criar filtro nenhum: %+v", filters)
		}
	})
}
