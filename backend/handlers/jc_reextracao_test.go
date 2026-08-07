package handlers

import (
	"testing"
	"time"
)

// A janela precisa terminar em ONTEM (o dia corrente ainda está sendo escrito
// na origem) e começar no PRIMEIRO DIA do mês N-1 meses atrás. Contagem de mês
// é onde mora o off-by-one: meses=3 tem que devolver 3 meses contando o
// corrente, não 4.
func TestJanelaReextracao(t *testing.T) {
	casos := []struct {
		nome    string
		hoje    string
		meses   int
		wantDe  string
		wantAte string
	}{
		{"3 meses no meio do mês", "2026-08-07", 3, "2026-06-01", "2026-08-06"},
		{"1 mês = só o corrente", "2026-08-07", 1, "2026-08-01", "2026-08-06"},
		{"vira o ano para trás", "2026-02-10", 3, "2025-12-01", "2026-02-09"},
		{"dia 1º: ontem é o mês anterior", "2026-08-01", 2, "2026-07-01", "2026-07-31"},
		{"meses inválido cai em 1", "2026-08-07", 0, "2026-08-01", "2026-08-06"},
	}
	for _, c := range casos {
		hoje, _ := time.Parse("2006-01-02", c.hoje)
		de, ate := janelaReextracao(hoje, c.meses)
		if de.Format("2006-01-02") != c.wantDe || ate.Format("2006-01-02") != c.wantAte {
			t.Errorf("%s: janela = %s..%s, esperado %s..%s", c.nome,
				de.Format("2006-01-02"), ate.Format("2006-01-02"), c.wantDe, c.wantAte)
		}
	}
}

// O agendamento não pode disparar duas vezes no mesmo dia nem pular a semana
// quando o horário ainda não chegou.
func TestProximaReextracao(t *testing.T) {
	casos := []struct {
		nome  string
		agora string // 2006-01-02 15:04
		dia   time.Weekday
		hora  int
		want  string
	}{
		// 2026-08-07 é uma sexta-feira.
		{"sexta → próximo domingo", "2026-08-07 10:00", time.Sunday, 6, "2026-08-09 06:00"},
		{"domingo antes da hora → hoje", "2026-08-09 03:00", time.Sunday, 6, "2026-08-09 06:00"},
		{"domingo depois da hora → semana que vem", "2026-08-09 07:00", time.Sunday, 6, "2026-08-16 06:00"},
		{"exatamente na hora → semana que vem (não redispara)", "2026-08-09 06:00", time.Sunday, 6, "2026-08-16 06:00"},
		{"outro dia da semana", "2026-08-07 10:00", time.Wednesday, 2, "2026-08-12 02:00"},
	}
	for _, c := range casos {
		agora, err := time.Parse("2006-01-02 15:04", c.agora)
		if err != nil {
			t.Fatalf("%s: data de teste inválida: %v", c.nome, err)
		}
		got := proximaReextracao(agora, c.dia, c.hora, 0)
		if got.Format("2006-01-02 15:04") != c.want {
			t.Errorf("%s: próxima = %s, esperado %s", c.nome, got.Format("2006-01-02 15:04"), c.want)
		}
		if !got.After(agora) {
			t.Errorf("%s: próxima execução não está no futuro", c.nome)
		}
	}
}

// Sem a variável, o job não pode subir: um trabalho de ~2h não deve começar a
// rodar em produção só porque o código foi deployado.
func TestReextracaoDesligadaPorPadrao(t *testing.T) {
	t.Setenv("JC_REEXTRACAO_MESES", "")
	if _, _, _, _, ok := jcReextracaoConfig(); ok {
		t.Error("sem JC_REEXTRACAO_MESES a reextração tem que ficar desligada")
	}
	t.Setenv("JC_REEXTRACAO_MESES", "abacaxi")
	if _, _, _, _, ok := jcReextracaoConfig(); ok {
		t.Error("valor inválido tem que desligar, não assumir default")
	}
	t.Setenv("JC_REEXTRACAO_MESES", "99")
	meses, dia, hora, _, ok := jcReextracaoConfig()
	if !ok || meses != 6 {
		t.Errorf("valor alto tem que ser limitado a 6, veio %d (ok=%v)", meses, ok)
	}
	if dia != time.Sunday || hora != 6 {
		t.Errorf("defaults esperados domingo 06:00, veio %v %02d", dia, hora)
	}
}
