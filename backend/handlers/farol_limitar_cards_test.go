package handlers

import (
	"fmt"
	"testing"
)

func TestLimitarCards(t *testing.T) {
	var cards []cardItem
	for i := 1; i <= 50; i++ {
		cards = append(cards, cardItem{Key: fmt.Sprint(1000 + i), Label: fmt.Sprintf("REDE %02d", i), ValorAtual: float64(i), Pct: float64(100 - i)})
	}
	cards = append(cards, cardItem{Key: "9", Label: "Supermercado Évora", ValorAtual: 0, ValorAnt: 7})

	// Abaixo do limite (ou sem limite): devolve tudo intacto, busca ignorada.
	for _, lim := range []int{0, 51, 500} {
		out, total, filtrados, trunc := limitarCards(cards, lim, "rede 01", "valor", "desc")
		if len(out) != 51 || total != 51 || filtrados != 51 || trunc || out[0].Key != "1001" {
			t.Fatalf("limite=%d não devia mexer: len=%d total=%d filtrados=%d trunc=%v", lim, len(out), total, filtrados, trunc)
		}
	}

	out, total, filtrados, trunc := limitarCards(cards, 10, "", "valor", "desc")
	if len(out) != 10 || total != 51 || filtrados != 51 || !trunc || out[0].Label != "REDE 50" || out[9].Label != "REDE 41" {
		t.Fatalf("top 10 por valor desc errado: %d %d %d %v %q..%q", len(out), total, filtrados, trunc, out[0].Label, out[len(out)-1].Label)
	}
	out, _, _, _ = limitarCards(cards, 10, "", "valor", "asc")
	if out[0].Key != "9" || out[1].Label != "REDE 01" {
		t.Fatalf("valor asc errado: %q, %q", out[0].Label, out[1].Label)
	}
	out, _, _, _ = limitarCards(cards, 10, "", "pct", "desc")
	if out[0].Label != "REDE 01" {
		t.Fatalf("pct desc errado: %q", out[0].Label)
	}
	// Busca por nome (sem diferenciar maiúsculas) ou por código; total continua o do recorte.
	out, total, filtrados, trunc = limitarCards(cards, 10, "  supermercado ", "valor", "desc")
	if len(out) != 1 || out[0].Key != "9" || total != 51 || filtrados != 1 || trunc {
		t.Fatalf("busca por nome errada: %d %d %d %v", len(out), total, filtrados, trunc)
	}
	out, _, filtrados, trunc = limitarCards(cards, 10, "100", "valor", "desc")
	if filtrados != 9 || trunc || out[0].Key != "1009" {
		t.Fatalf("busca por código errada: filtrados=%d trunc=%v primeiro=%s", filtrados, trunc, out[0].Key)
	}
	if len(cards) != 51 || cards[0].Key != "1001" {
		t.Fatalf("limitarCards com busca não pode alterar a lista original")
	}
}
