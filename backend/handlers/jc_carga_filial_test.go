package handlers

import (
	"os"
	"strings"
	"testing"
	"time"
)

// A regra que este arquivo protege: quando a carga é de UMA filial, o DELETE
// prévio TAMBÉM precisa ser. Se o filtro chegar na extração mas não no
// apagamento, carregar a filial 12 apaga o dia inteiro das outras 11 e regrava
// só a 12 — perda silenciosa de milhões de linhas, que só apareceria quando
// alguém estranhasse um total.
func TestDeletePrevioRespeitaFilial(t *testing.T) {
	fonte, err := os.ReadFile("farol_v2_import.go")
	if err != nil {
		t.Fatalf("ler farol_v2_import.go: %v", err)
	}
	src := string(fonte)

	i := strings.Index(src, "delSQL := fmt.Sprintf(`DELETE FROM %s WHERE empresa_id=$1")
	if i < 0 {
		t.Fatal("não achei o DELETE prévio — se ele foi reescrito, revise se ainda restringe por filial")
	}
	trecho := src[i : i+700]

	if !strings.Contains(trecho, "filialFiltro") {
		t.Error("o DELETE prévio não consulta filialFiltro: uma carga de filial apagaria as demais")
	}
	if !strings.Contains(trecho, `delSQL += " AND empresa = $3"`) {
		t.Error("o DELETE prévio não acrescenta o predicado de filial")
	}
}

// O filtro tem de chegar na consulta ao Oracle — senão a extração traz tudo e o
// DELETE restrito grava só parte, deixando o período inconsistente.
func TestExtratorFiltraPorFilial(t *testing.T) {
	fonte, err := os.ReadFile("jc_extrator.go")
	if err != nil {
		t.Fatalf("ler jc_extrator.go: %v", err)
	}
	src := string(fonte)
	if !strings.Contains(src, `q += " AND EMPRESA = :3"`) {
		t.Error("o extrator não restringe por EMPRESA quando a filial é informada")
	}
	if !strings.Contains(src, "func ExtrairPeriodoJC(ctx context.Context, de, ate time.Time, filial string,") {
		t.Error("ExtrairPeriodoJC deveria receber a filial")
	}
}

// Vazio = todas as filiais. É o comportamento de sempre e vale para a carga
// diária, o backfill e a reextração — nenhum deles passa filial.
func TestFilialVaziaMantemComportamentoAntigo(t *testing.T) {
	for _, arq := range []string{"jc_agendador.go", "jc_reextracao.go", "jc_carga.go"} {
		fonte, err := os.ReadFile(arq)
		if err != nil {
			t.Fatalf("ler %s: %v", arq, err)
		}
		if strings.Contains(string(fonte), "ExecutarCargaJCIntervalo(db,") &&
			!strings.Contains(string(fonte), `, ""`) &&
			!strings.Contains(string(fonte), ", filial)") {
			t.Errorf("%s: chamada sem escopo de filial explícito", arq)
		}
	}
}

// O e-mail do intervalo precisa dizer o escopo. Em 19/08/2026 chegou um resumo
// de 3,7M de linhas e não dava para saber, sem abrir o log do container, se
// aquilo era a empresa inteira ou uma filial só — informação que separa uma
// carga de rotina de um incidente.
func TestResumoIntervaloDeclaraEscopoDeFilial(t *testing.T) {
	de := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ate := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	inicio := time.Date(2026, 8, 18, 15, 5, 49, 0, time.UTC)

	t.Run("todas as filiais", func(t *testing.T) {
		assunto, corpo := corpoResumoIntervaloJC(de, ate, inicio, nil, nil, "")
		if !strings.Contains(corpo, "Escopo    : todas as filiais") {
			t.Errorf("corpo não declara que foram todas as filiais:\n%s", corpo)
		}
		if strings.Contains(assunto, "filial") {
			t.Errorf("assunto não deveria citar filial quando são todas: %q", assunto)
		}
	})

	t.Run("filial única", func(t *testing.T) {
		assunto, corpo := corpoResumoIntervaloJC(de, ate, inicio, nil, nil, "12")
		if !strings.Contains(corpo, "Escopo    : SOMENTE filial 12") {
			t.Errorf("corpo não declara a restrição de filial:\n%s", corpo)
		}
		if !strings.Contains(assunto, "filial 12") {
			t.Errorf("assunto deveria destacar a restrição: %q", assunto)
		}
	})
}
