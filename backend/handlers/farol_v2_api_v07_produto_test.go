package handlers

import (
	"net/url"
	"testing"
)

// "Por Departamento" (V07) estende até o Produto — pedido do Heverton
// 13/10/2026. A JC nunca preencheu cod_categoria, então a hierarquia virou
// Departamento → Seção → Produto; o Produto lê a base escopada pelo drill.
func TestFarolV2Cards_V07_DrillAteProduto(t *testing.T) {
	db, empresaID := biTestDB(t)

	depto, sec := "T7DEPTO", "T7SEC"
	fornInd, fornOutro := "T7FORNIND", "T7FORNOUT"
	nome := "TV07 PROD IND"

	limpar := func() {
		db.Exec(`DELETE FROM farol.industrias WHERE empresa_id = $1 AND nome = $2`, empresaID, nome)
		db.Exec(`DELETE FROM vendas_faturadas WHERE empresa_id = $1 AND cod_depto = $2`, empresaID, depto)
	}
	limpar()
	t.Cleanup(limpar)

	var industriaID int
	if err := db.QueryRow(`INSERT INTO farol.industrias (empresa_id, nome) VALUES ($1, $2) RETURNING id`, empresaID, nome).Scan(&industriaID); err != nil {
		t.Fatalf("criar indústria: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES ($1, $2, $3)`,
		empresaID, industriaID, fornInd); err != nil {
		t.Fatalf("vincular fornecedor: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO vendas_faturadas (empresa_id, data_faturamento, cod_depto, cod_sec, cod_prod, nome_prod, cod_fornec, cod_cli, cnpj, qt, pvenda, tipo_venda)
		VALUES
			($1, $2, $3, $4, 'T7PROD1', 'PRODUTO UM', $5, 'C1', '35111111000100', 1, 700, '1'),
			($1, $2, $3, $4, 'T7PROD1', 'PRODUTO UM', $6, 'C2', '36222222000100', 1, 300, '1'),
			($1, $2, $3, $4, 'T7PROD2', 'PRODUTO DOIS', $5, 'C1', '35111111000100', 1, 200, '1')
	`, empresaID, mustParseData(t, "2026-08-15"), depto, sec, fornInd, fornOutro); err != nil {
		t.Fatalf("insert vendas_faturadas: %v", err)
	}

	drill := `[{"level":"cod_depto","value":"` + depto + `","label":"D"},{"level":"cod_sec","value":"` + sec + `","label":"S"}]`
	base := "/api/v2/farol/cards?view=V07&fluxo=faturado&ref_inicio=2026-08-01&ref_fim=2026-08-31&drill=" + url.QueryEscape(drill)

	valorDe := func(resp cardsResponse, cod string) (float64, bool) {
		for _, c := range resp.Cards {
			if c.Key == cod {
				return c.ValorAtual, true
			}
		}
		return 0, false
	}

	// Sem filtro: os 3 lançamentos entram.
	resp := cardsGet(t, db, base, empresaID)
	if resp.NextLevel != "cod_prod" {
		t.Errorf("NextLevel sob a Seção = %q, quero cod_prod (Produto)", resp.NextLevel)
	}
	if v, ok := valorDe(resp, "T7PROD1"); !ok || v != 1000 {
		t.Errorf("PROD1 = %v (achou=%v), quero 1000", v, ok)
	}
	if v, ok := valorDe(resp, "T7PROD2"); !ok || v != 200 {
		t.Errorf("PROD2 = %v (achou=%v), quero 200", v, ok)
	}

	// Somente Indústrias: o fornecedor avulso (R$300 do PROD1) não pode entrar.
	resp = cardsGet(t, db, base+"&somente_industria=1", empresaID)
	if v, ok := valorDe(resp, "T7PROD1"); !ok || v != 700 {
		t.Errorf("PROD1 em Somente Indústrias = %v (achou=%v), quero 700", v, ok)
	}
}
