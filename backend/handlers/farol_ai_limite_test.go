package handlers

import "testing"

// O ponto e vírgula é a armadilha: LIMIT concatenado depois dele vira um
// comando órfão e a query inteira falha com erro de sintaxe.
func TestGarantirLimite(t *testing.T) {
	casos := []struct{ nome, in, quer string }{
		{"já tem limit", "SELECT 1 LIMIT 10;", "SELECT 1 LIMIT 10;"},
		{"com ponto e vírgula", "SELECT 1;", "SELECT 1\nLIMIT 200;"},
		{"sem ponto e vírgula", "SELECT 1", "SELECT 1\nLIMIT 200;"},
		{"com quebra no fim", "SELECT 1;\n\n", "SELECT 1\nLIMIT 200;"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := garantirLimite(c.in); got != c.quer {
				t.Errorf("garantirLimite(%q) = %q, quer %q", c.in, got, c.quer)
			}
		})
	}
}
