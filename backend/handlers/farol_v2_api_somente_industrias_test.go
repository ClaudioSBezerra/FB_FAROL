package handlers

// farol_v2_api_somente_industrias_test.go — cobre o toggle "Somente
// Indústrias" (mig 228, 08/09/2026): V03 (Por Gerência) lê de tabelas novas
// pré-filtradas (agg_fat_v03_ind_*_mes); V02 (Por Equipe) reaproveita as
// tabelas originais em L2 (Fornecedor) e o handler injeta o filtro de
// cod_fornec ali. Testes de integração — precisam de banco real.

import (
	"testing"
)

// TestFarolV2Cards_SomenteIndustrias_V03_SoContaFornecedorIndustria — o card
// do Gerente, com o toggle ligado, tem que refletir SÓ a venda do fornecedor
// cadastrado como indústria — a venda do fornecedor avulso (não cadastrado)
// não pode entrar, mesmo sendo do mesmo cliente/gerente/mês.
func TestFarolV2Cards_SomenteIndustrias_V03_SoContaFornecedorIndustria(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2SI V03 GERENTE"
	gerente := "T2SIGER"
	codIndustria, codOutro := "T2SIFORNIND", "T2SIFORNOUT"
	cnpjA, cnpjB := "31111111000100", "32222222000100"
	ano, mes := 2026, 8

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_gerente = $2`, empresaID, gerente)
		db.Exec(`DELETE FROM farol.agg_fat_v03_ind_l0_mes WHERE empresa_id = $1 AND cod_gerente = $2`, empresaID, gerente)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
		empresaID, industriaID, codIndustria); err != nil {
		t.Fatalf("vincular %s: %v", codIndustria, err)
	}

	// cnpjA compra do fornecedor cadastrado como indústria (R$1000); cnpjB
	// compra de um fornecedor QUALQUER, não cadastrado (R$2000) — mesmo
	// gerente, mesmo mês. Só a venda de cnpjA pode aparecer com o toggle ligado.
	if _, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_gerente, cod_fornec, cod_cli, cnpj, qt, pvenda, tipo_venda)
		VALUES
			($1, $2, $3, $4, 'CLIA', $5, 1, 1000, '1'),
			($1, $2, $3, $6, 'CLIB', $7, 1, 2000, '1')
	`, empresaID, mustParseData(t, "2026-08-15"), gerente, codIndustria, cnpjA, codOutro, cnpjB); err != nil {
		t.Fatalf("insert vendas_faturadas: %v", err)
	}

	// Popula agg_fat_v03_ind_l0_mes (a rotina real da carga diária, mig 228).
	if _, err := db.Exec(`SELECT farol.upsert_aggs_mes_ind($1, $2, $3)`, empresaID, ano, mes); err != nil {
		t.Fatalf("upsert_aggs_mes_ind: %v", err)
	}

	url := "/api/v2/farol/cards?view=V03&fluxo=faturado&somente_industria=1&ref_inicio=2026-08-01&ref_fim=2026-08-31"
	resp := cardsGet(t, db, url, empresaID)

	if resp.View != "V03" {
		t.Errorf("View = %q, want %q — pseudo-view interna (V03I) nunca pode vazar pro JSON", resp.View, "V03")
	}

	var card *cardItem
	for i := range resp.Cards {
		if resp.Cards[i].Key == gerente {
			card = &resp.Cards[i]
		}
	}
	if card == nil {
		t.Fatalf("card do gerente %s não apareceu: %+v", gerente, resp.Cards)
	}
	if card.ValorAtual != 1000 {
		t.Errorf("ValorAtual = %v, want 1000 (só o fornecedor cadastrado como indústria; os R$2000 do fornecedor avulso não podem entrar)", card.ValorAtual)
	}
}

// TestFarolV2Cards_SomenteIndustrias_V02_L2_FiltraFornecedorNaoCadastrado —
// em "Por Equipe" (V02), drilado até um RCA, o nível Fornecedor (L2)
// reaproveita a tabela ORIGINAL (agg_fat_v02_l2_mes, já tem cod_fornec no
// grão) — o toggle só injeta um filtro, não troca de tabela. Um card de
// fornecedor NÃO cadastrado como indústria não pode aparecer na lista.
func TestFarolV2Cards_SomenteIndustrias_V02_L2_FiltraFornecedorNaoCadastrado(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2SI V02 L2"
	supervisor, rca := "T2SISUP", "T2SIRCA"
	codIndustria, codOutro := "T2SIL2IND", "T2SIL2OUT"
	ano, mes := 2026, 8

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM farol.agg_fat_v02_l2_mes WHERE empresa_id = $1 AND cod_rca = $2`, empresaID, rca)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
		empresaID, industriaID, codIndustria); err != nil {
		t.Fatalf("vincular %s: %v", codIndustria, err)
	}

	// Duas linhas já na tabela agg ORIGINAL (mesmo padrão do teste
	// "MesCompletoIgnoraAggCorrompida" acima — insere direto na agg em vez de
	// rodar a carga inteira, mais barato e controlado num teste).
	for _, cod := range []string{codIndustria, codOutro} {
		if _, err := db.Exec(`
			INSERT INTO farol.agg_fat_v02_l2_mes (empresa_id, ano, mes, cod_supervisor, cod_rca, cod_fornec, nome_fornec, pvenda, base_cli, positivados, mix)
			VALUES ($1,$2,$3,$4,$5,$6,$6,777, 10, 1, 1)
		`, empresaID, ano, mes, supervisor, rca, cod); err != nil {
			t.Fatalf("insert agg_fat_v02_l2_mes %s: %v", cod, err)
		}
	}

	drill := `[{"level":"cod_supervisor","value":"` + supervisor + `"},{"level":"cod_rca","value":"` + rca + `"}]`
	url := "/api/v2/farol/cards?view=V02&fluxo=faturado&somente_industria=1&ref_inicio=2026-08-01&ref_fim=2026-08-31&drill=" + drill
	resp := cardsGet(t, db, url, empresaID)

	achouIndustria, achouOutro := false, false
	for _, c := range resp.Cards {
		if c.Key == codIndustria {
			achouIndustria = true
		}
		if c.Key == codOutro {
			achouOutro = true
		}
	}
	if !achouIndustria {
		t.Errorf("card do fornecedor cadastrado (%s) devia aparecer, cards: %+v", codIndustria, resp.Cards)
	}
	if achouOutro {
		t.Errorf("card do fornecedor NÃO cadastrado (%s) apareceu com o toggle ligado — filtro não foi aplicado", codOutro)
	}
}

// TestFarolV2Cards_SomenteIndustrias_V01_FiltraFornecedorNaoCadastrado —
// "Por FORN.GERAL" (V01) nunca precisou de tabela nova: cod_fornec é a raiz
// da hierarquia, já está em agg_fat_v01_l0_mes. O toggle só injeta o
// filtro — um fornecedor não cadastrado como indústria some da lista.
func TestFarolV2Cards_SomenteIndustrias_V01_FiltraFornecedorNaoCadastrado(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2SI V01 L0"
	codIndustria, codOutro := "T2SIV01IND", "T2SIV01OUT"
	ano, mes := 2026, 8

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM farol.agg_fat_v01_l0_mes WHERE empresa_id = $1 AND cod_fornec IN ($2, $3)`,
			empresaID, codIndustria, codOutro)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
		empresaID, industriaID, codIndustria); err != nil {
		t.Fatalf("vincular %s: %v", codIndustria, err)
	}

	for _, cod := range []string{codIndustria, codOutro} {
		if _, err := db.Exec(`
			INSERT INTO farol.agg_fat_v01_l0_mes (empresa_id, ano, mes, cod_fornec, nome_fornec, pvenda, base_cli, positivados, mix)
			VALUES ($1,$2,$3,$4,$4,777, 10, 1, 1)
		`, empresaID, ano, mes, cod); err != nil {
			t.Fatalf("insert agg_fat_v01_l0_mes %s: %v", cod, err)
		}
	}

	url := "/api/v2/farol/cards?view=V01&fluxo=faturado&somente_industria=1&ref_inicio=2026-08-01&ref_fim=2026-08-31"
	resp := cardsGet(t, db, url, empresaID)

	achouIndustria, achouOutro := false, false
	for _, c := range resp.Cards {
		if c.Key == codIndustria {
			achouIndustria = true
		}
		if c.Key == codOutro {
			achouOutro = true
		}
	}
	if !achouIndustria {
		t.Errorf("card do fornecedor cadastrado (%s) devia aparecer, cards: %+v", codIndustria, resp.Cards)
	}
	if achouOutro {
		t.Errorf("card do fornecedor NÃO cadastrado (%s) apareceu com o toggle ligado", codOutro)
	}
}

// TestFarolV2Cards_SomenteIndustrias_V06_SoContaFornecedorIndustria — "Por
// Rede" (V06) L0 lê de agg_fat_v06_ind_l0_mes (tabela nova, mig 229): o
// card da Rede só pode refletir a venda do fornecedor indústria.
func TestFarolV2Cards_SomenteIndustrias_V06_SoContaFornecedorIndustria(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2SI V06 REDE"
	rede := "T2SIREDE"
	codIndustria, codOutro := "T2SIV06IND", "T2SIV06OUT"
	cnpjA, cnpjB := "33111111000100", "34222222000100"
	ano, mes := 2026, 8

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_cliprinc = $2`, empresaID, rede)
		db.Exec(`DELETE FROM farol.agg_fat_v06_ind_l0_mes WHERE empresa_id = $1 AND cod_cliprinc = $2`, empresaID, rede)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
		empresaID, industriaID, codIndustria); err != nil {
		t.Fatalf("vincular %s: %v", codIndustria, err)
	}

	if _, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_cliprinc, cod_fornec, cod_cli, cnpj, qt, pvenda, tipo_venda)
		VALUES
			($1, $2, $3, $4, 'CLIA', $5, 1, 1000, '1'),
			($1, $2, $3, $6, 'CLIB', $7, 1, 2000, '1')
	`, empresaID, mustParseData(t, "2026-08-15"), rede, codIndustria, cnpjA, codOutro, cnpjB); err != nil {
		t.Fatalf("insert vendas_faturadas: %v", err)
	}

	if _, err := db.Exec(`SELECT farol.upsert_aggs_mes_ind($1, $2, $3)`, empresaID, ano, mes); err != nil {
		t.Fatalf("upsert_aggs_mes_ind: %v", err)
	}

	url := "/api/v2/farol/cards?view=V06&fluxo=faturado&somente_industria=1&ref_inicio=2026-08-01&ref_fim=2026-08-31"
	resp := cardsGet(t, db, url, empresaID)

	var card *cardItem
	for i := range resp.Cards {
		if resp.Cards[i].Key == rede {
			card = &resp.Cards[i]
		}
	}
	if card == nil {
		t.Fatalf("card da rede %s não apareceu: %+v", rede, resp.Cards)
	}
	if card.ValorAtual != 1000 {
		t.Errorf("ValorAtual = %v, want 1000 (só o fornecedor indústria; os R$2000 do avulso não podem entrar)", card.ValorAtual)
	}
}

// TestFarolV2Cards_SomenteIndustrias_V07_SoContaFornecedorIndustria — "Por
// Departamento" (V07) L0 lê de agg_fat_v07_ind_l0_mes (tabela nova, mig
// 229) — é taxonomia de produto, nunca teve fornecedor no grão em nível
// nenhum, então essa é a visão que mais dependia de tabela nova.
func TestFarolV2Cards_SomenteIndustrias_V07_SoContaFornecedorIndustria(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2SI V07 DEPTO"
	depto := "T2SIDEPTO"
	codIndustria, codOutro := "T2SIV07IND", "T2SIV07OUT"
	cnpjA, cnpjB := "35111111000100", "36222222000100"
	ano, mes := 2026, 8

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_depto = $2`, empresaID, depto)
		db.Exec(`DELETE FROM farol.agg_fat_v07_ind_l0_mes WHERE empresa_id = $1 AND cod_depto = $2`, empresaID, depto)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
		empresaID, industriaID, codIndustria); err != nil {
		t.Fatalf("vincular %s: %v", codIndustria, err)
	}

	if _, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_depto, cod_fornec, cod_cli, cnpj, qt, pvenda, tipo_venda)
		VALUES
			($1, $2, $3, $4, 'CLIA', $5, 1, 1000, '1'),
			($1, $2, $3, $6, 'CLIB', $7, 1, 2000, '1')
	`, empresaID, mustParseData(t, "2026-08-15"), depto, codIndustria, cnpjA, codOutro, cnpjB); err != nil {
		t.Fatalf("insert vendas_faturadas: %v", err)
	}

	if _, err := db.Exec(`SELECT farol.upsert_aggs_mes_ind($1, $2, $3)`, empresaID, ano, mes); err != nil {
		t.Fatalf("upsert_aggs_mes_ind: %v", err)
	}

	url := "/api/v2/farol/cards?view=V07&fluxo=faturado&somente_industria=1&ref_inicio=2026-08-01&ref_fim=2026-08-31"
	resp := cardsGet(t, db, url, empresaID)

	var card *cardItem
	for i := range resp.Cards {
		if resp.Cards[i].Key == depto {
			card = &resp.Cards[i]
		}
	}
	if card == nil {
		t.Fatalf("card do departamento %s não apareceu: %+v", depto, resp.Cards)
	}
	if card.ValorAtual != 1000 {
		t.Errorf("ValorAtual = %v, want 1000 (só o fornecedor indústria; os R$2000 do avulso não podem entrar)", card.ValorAtual)
	}
}

// TestPrewarmAggMesCore_AquecePosViewsComIndustria — sem isto, o prewarm de
// boot/diário só aquecia a chave do toggle DESLIGADO (view="V03", sem
// filtro), mas o toggle entra LIGADO por padrão desde 08/09/2026: todo login
// pagava cache MISS na positivação mesmo logo após o prewarm rodar. Prova que
// prewarmAggMesCore grava, no baseCache, exatamente as chaves que uma request
// real com somente_industria=1 vai procurar — para V03 (pseudo-view V03I,
// sem filtro) e para V01 (mesma tabela, filtro cod_fornec injetado).
func TestPrewarmAggMesCore_AquecePosViewsComIndustria(t *testing.T) {
	db, empresaID := biTestDB(t)

	nome := "TV2SI PREWARM"
	codIndustria := "T2SIPREWARM"

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
		empresaID, industriaID, codIndustria); err != nil {
		t.Fatalf("vincular %s: %v", codIndustria, err)
	}

	baseCacheMu.Lock()
	baseCache = map[string]baseCacheEntry{}
	baseCacheMu.Unlock()

	prewarmAggMesCore(db, empresaID)

	fat := fluxoCtx{name: "faturado"}
	cods := industriaMappedFornecs(db, empresaID)
	if len(cods) == 0 {
		t.Fatal("industriaMappedFornecs não devolveu o fornecedor recém-vinculado")
	}

	// V03I — pseudo-view, sem filtro extra. Mesma chamada que
	// queryDistinctCliPositivados faria com viewInterno="V03I".
	if _, hit := cachedDistinctPositivados(nil, empresaID, fat, "V03I", "cod_gerente", 0, 999912, nil, nil); !hit {
		t.Error("prewarmAggMesCore não aqueceu V03I (pseudo-view) — 1º login com o toggle ligado pagaria o miss")
	}
	// V01 com o toggle ligado — mesma tabela, filtro cod_fornec injetado
	// (FarolV2CardsHandler faz isso via industriaMappedFornecs).
	if _, hit := cachedDistinctPositivados(nil, empresaID, fat, "V01", "cod_fornec", 0, 999912, nil, multiFilters{"cod_fornec": cods}); !hit {
		t.Error("prewarmAggMesCore não aqueceu V01+cod_fornec (toggle ligado) — 1º login pagaria o miss")
	}
}
