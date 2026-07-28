package handlers

import (
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

	want := "v.ano BETWEEN $2 AND $3 AND (v.ano * 100 + v.mes) BETWEEN $4 AND $5"
	if got != want {
		t.Errorf("SQL\n got: %s\nwant: %s", got, want)
	}

	wantArgs := []any{"empresa-uuid", 2026, 2026, 202601, 202607}
	if len(args) != len(wantArgs) {
		t.Fatalf("len(args) = %d, want %d (%v)", len(args), len(wantArgs), args)
	}
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
