package handlers

import "testing"

func TestJanelaDeApuracao(t *testing.T) {
	cases := []struct{ janela, ini, fim, wantIni, wantFim string }{
		{"bimestre_movel", "2026-09-01", "2026-09-30", "2026-08-01", "2026-09-30"},
		{"bimestre_movel", "2026-10-01", "2026-10-31", "2026-09-01", "2026-10-31"},
		{"bimestre_movel", "2027-01-01", "2027-01-31", "2026-12-01", "2027-01-31"},
		{"mes_fechado", "2026-09-01", "2026-09-30", "2026-09-01", "2026-09-30"},
	}
	for _, c := range cases {
		i, f := janelaDeApuracao(c.janela, c.ini, c.fim)
		if i != c.wantIni || f != c.wantFim {
			t.Errorf("%s %s..%s = %s..%s, quero %s..%s", c.janela, c.ini, c.fim, i, f, c.wantIni, c.wantFim)
		}
	}
}
