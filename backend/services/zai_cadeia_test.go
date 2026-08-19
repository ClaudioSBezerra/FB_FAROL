package services

import (
	"fmt"
	"testing"
)

// A cadeia de reserva existe porque o padrão passou a ser um modelo PAGO.
// Conta sem saldo não pode derrubar o assistente — tem que degradar para o
// gratuito, senão o sintoma (erro em toda pergunta) não lembra em nada a
// causa (fatura).
func TestDeveTentarReserva(t *testing.T) {
	casos := []struct {
		nome string
		err  error
		quer bool
	}{
		{"sem erro", nil, false},
		{"429 cota por minuto", fmt.Errorf("429 rate limit"), true},
		{"402 sem saldo", fmt.Errorf("Z.AI HTTP 402: payment required"), true},
		{"saldo insuficiente", fmt.Errorf("Z.AI HTTP 400: insufficient balance"), true},
		{"quota esgotada", fmt.Errorf("quota exceeded for this month"), true},
		{"prompt inválido não reescala", fmt.Errorf("Z.AI HTTP 400: invalid request"), false},
		{"rede não reescala", fmt.Errorf("Z.AI request error: dial tcp timeout"), false},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := deveTentarReserva(c.err); got != c.quer {
				t.Errorf("deveTentarReserva(%v) = %v, quer %v", c.err, got, c.quer)
			}
		})
	}
}

// O último degrau tem que ser gratuito, sempre. Se alguém trocar esta
// constante por um modelo pago, a rede de proteção deixa de proteger.
func TestUltimoDegrauEhGratuito(t *testing.T) {
	if ZAIModelGratuito != "glm-4.7-flash" && ZAIModelGratuito != "glm-4.5-flash" {
		t.Errorf("ZAIModelGratuito = %q — precisa ser um modelo do tier gratuito", ZAIModelGratuito)
	}
}
