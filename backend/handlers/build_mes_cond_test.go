package handlers

import (
	"strings"
	"testing"
	"time"
)

// TestBuildMesCondArgsEIndices — buildMesCond passou a emitir 4 placeholders
// (v.ano BETWEEN + a expressão ano*100+mes) para habilitar partition pruning
// nas agg_* (PARTITION BY RANGE (ano), mig 162). Como ele APENDA em args já
// populado ($1 = empresa_id), um erro de índice aqui não quebra o build — vira
// query errada em produção. Daí o teste.
func TestBuildMesCondArgsEIndices(t *testing.T) {
	args := []any{"empresa-uuid"} // $1 já ocupado, como nos call sites reais
	got := buildMesCond(202601, 202607, &args)

	// Mesmo ano → ano + mes, SEM a expressão (ela pioraria a estimativa).
	want := "v.ano BETWEEN $2 AND $3 AND v.mes BETWEEN $4 AND $5"
	if got != want {
		t.Errorf("SQL\n got: %s\nwant: %s", got, want)
	}

	wantArgs := []any{"empresa-uuid", 2026, 2026, 1, 7}
	if len(args) != len(wantArgs) {
		t.Fatalf("len(args) = %d, want %d (%v)", len(args), len(wantArgs), args)
	}
	for i := range wantArgs {
		if args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %v, want %v", i, args[i], wantArgs[i])
		}
	}
}

// TestBuildMesCondCruzandoAnoUsaExpressao — cruzando o ano não há range simples
// equivalente, então a expressão é insubstituível e PRECISA aparecer.
func TestBuildMesCondCruzandoAnoUsaExpressao(t *testing.T) {
	args := []any{"e"}
	got := buildMesCond(202511, 202602, &args)

	want := "v.ano BETWEEN $2 AND $3 AND (v.ano * 100 + v.mes) BETWEEN $4 AND $5"
	if got != want {
		t.Errorf("SQL\n got: %s\nwant: %s", got, want)
	}
	wantArgs := []any{"e", 2025, 2026, 202511, 202602}
	for i := range wantArgs {
		if args[i] != wantArgs[i] {
			t.Errorf("args[%d] = %v, want %v", i, args[i], wantArgs[i])
		}
	}
}

// TestBuildMesCondAnoDerivado — o predicado redundante só é seguro se o ano
// derivado nunca excluir uma linha que a expressão incluiria. Verifica os casos
// de borda: range dentro de um ano, cruzando o ano, e o "histórico completo"
// (ymStart=0, ymEnd=999912) usado por queryBasePositivados.
func TestBuildMesCondAnoDerivado(t *testing.T) {
	casos := []struct {
		nome           string
		ymIni, ymFim   int
		anoIni, anoFim int
	}{
		{"dentro do ano", 202601, 202607, 2026, 2026},
		{"cruzando o ano", 202511, 202602, 2025, 2026},
		{"mes unico", 202606, 202606, 2026, 2026},
		{"historico completo", 0, 999912, 0, 9999},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			args := []any{"e"}
			buildMesCond(c.ymIni, c.ymFim, &args)
			if args[1] != c.anoIni {
				t.Errorf("anoIni = %v, want %d", args[1], c.anoIni)
			}
			if args[2] != c.anoFim {
				t.Errorf("anoFim = %v, want %d", args[2], c.anoFim)
			}
		})
	}
}

// TestBuildMesCondImplicadoPelaExpressao — prova exaustiva (todos os meses de
// 2024..2028) de que o predicado redundante `ano BETWEEN` nunca exclui uma
// linha aceita pela expressão. Se falhasse, o pruning mudaria RESULTADO, não
// só plano — o tipo de bug que só aparece como número errado no painel.
func TestBuildMesCondImplicadoPelaExpressao(t *testing.T) {
	ranges := [][2]int{
		{202601, 202607}, {202511, 202602}, {202606, 202606},
		{202501, 202512}, {202412, 202801}, {0, 999912},
	}
	for _, r := range ranges {
		ymIni, ymFim := r[0], r[1]
		anoIni, anoFim := ymIni/100, ymFim/100
		for ano := 2024; ano <= 2028; ano++ {
			for mes := 1; mes <= 12; mes++ {
				ymLinha := ano*100 + mes
				incluidaPelaExpressao := ymLinha >= ymIni && ymLinha <= ymFim
				aceitaPeloAno := ano >= anoIni && ano <= anoFim
				if incluidaPelaExpressao && !aceitaPeloAno {
					t.Errorf("range [%d..%d]: linha ano=%d mes=%d passa na expressão mas o predicado de ano a exclui",
						ymIni, ymFim, ano, mes)
				}
			}
		}
	}
}

// TestBuildMesCondPredicadoDeMes — o `mes BETWEEN` só é emitido quando o range
// cabe num único ano; cruzando o ano não há range simples equivalente e ele
// PRECISA ser omitido (senão [2025-11..2026-02] viraria mes 11..2, vazio).
func TestBuildMesCondPredicadoDeMes(t *testing.T) {
	casos := []struct {
		nome           string
		ymIni, ymFim   int
		querMes        bool
		mesIni, mesFim int
	}{
		{"mes unico", 202606, 202606, true, 6, 6},
		{"ytd mesmo ano", 202601, 202607, true, 1, 7},
		{"ano cheio", 202501, 202512, true, 1, 12},
		{"cruza o ano", 202511, 202602, false, 0, 0},
		{"historico completo", 0, 999912, false, 0, 0},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			args := []any{"e"}
			got := buildMesCond(c.ymIni, c.ymFim, &args)
			temMes := strings.Contains(got, "v.mes BETWEEN")
			if temMes != c.querMes {
				t.Fatalf("emitiu mes=%t, queria %t — SQL: %s", temMes, c.querMes, got)
			}
			// Layout dos args: [$1 pré-existente, anoIni, anoFim, X, Y]
			// onde X,Y = mesIni,mesFim (mesmo ano) ou ymIni,ymFim (cruzando).
			if len(args) != 5 {
				t.Fatalf("esperava 5 args, tem %d: %v", len(args), args)
			}
			if !c.querMes {
				return
			}
			if args[3] != c.mesIni || args[4] != c.mesFim {
				t.Errorf("mes = [%v..%v], want [%d..%d]", args[3], args[4], c.mesIni, c.mesFim)
			}
		})
	}
}

// TestBuildMesCondMesNaoAlteraResultado — a prova que sustenta a REMOÇÃO da
// expressão no caso de mesmo ano. `ano BETWEEN Y AND Y AND mes BETWEEN m1..m2`
// tem que aceitar EXATAMENTE as mesmas linhas que `(ano*100+mes) BETWEEN ym1
// AND ym2`. Se divergisse, o painel mostraria número ERRADO — não é questão de
// plano, é de resultado. Varre todos os meses de 2024..2028.
func TestBuildMesCondMesNaoAlteraResultado(t *testing.T) {
	// Só ranges dentro de um mesmo ano: são os únicos que trocam a expressão.
	ranges := [][2]int{
		{202601, 202607}, {202606, 202606}, {202501, 202512}, {202503, 202509},
	}
	for _, r := range ranges {
		ymIni, ymFim := r[0], r[1]
		anoIni, anoFim := ymIni/100, ymFim/100
		mesIni, mesFim := ymIni%100, ymFim%100
		for ano := 2024; ano <= 2028; ano++ {
			for mes := 1; mes <= 12; mes++ {
				ymLinha := ano*100 + mes
				novo := ano >= anoIni && ano <= anoFim && mes >= mesIni && mes <= mesFim
				antigo := ymLinha >= ymIni && ymLinha <= ymFim
				if novo != antigo {
					t.Errorf("range [%d..%d]: ano=%d mes=%d divergiu (novo=%t antigo=%t)",
						ymIni, ymFim, ano, mes, novo, antigo)
				}
			}
		}
	}
}

// TestYmConsistente — ym() é a fonte dos valores que buildMesCond divide por
// 100 para achar o ano; se mudar de formato, o pruning derivaria ano errado.
func TestYmConsistente(t *testing.T) {
	d := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	if got := ym(d); got != 202607 {
		t.Errorf("ym = %d, want 202607", got)
	}
	if got := ym(d) / 100; got != 2026 {
		t.Errorf("ano derivado = %d, want 2026", got)
	}
}
