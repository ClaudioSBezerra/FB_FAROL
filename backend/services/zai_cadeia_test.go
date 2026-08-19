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

// O último degrau muda com o modo, e cada um tem sua razão: no plano todos os
// modelos entram na mesma cota, então o mais barato é indiferente e vale o mais
// disponível; na API padrão só o flash não consome saldo — e foi exatamente
// saldo zerado que derrubou o glm-4.7 em 19/08/2026.
func TestUltimoRecursoPorModo(t *testing.T) {
	t.Setenv("ZAI_MODO", "coding")
	if got := ultimoRecurso(); got != "glm-4.7" {
		t.Errorf("modo coding: ultimoRecurso() = %q, quer glm-4.7", got)
	}
	t.Setenv("ZAI_MODO", "padrao")
	if got := ultimoRecurso(); got != "glm-4.7-flash" {
		t.Errorf("modo padrao: ultimoRecurso() = %q — precisa ser do tier gratuito", got)
	}
}

// Coding é o default: em 19/08/2026 a API padrão estava sem saldo e só o plano
// respondia. Se alguém inverter isso sem querer, o assistente para.
func TestModoPadraoEhCoding(t *testing.T) {
	t.Setenv("ZAI_MODO", "")
	if !modoCoding() {
		t.Error("sem ZAI_MODO o modo deveria ser coding")
	}
	t.Setenv("ZAI_MODO", "PADRAO")
	if modoCoding() {
		t.Error("ZAI_MODO=PADRAO deveria sair do modo coding (case-insensitive)")
	}
}

// O glm-5.3 responde com bloco "thinking" ANTES do texto. Ler content[0] daria
// string vazia justo no modelo que é o padrão — e o erro apareceria como "a IA
// não retornou SQL válido", apontando para o lugar errado.
func TestExtrairTextoIgnoraThinking(t *testing.T) {
	r := anthropicResp{Content: []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		{Type: "thinking", Text: ""},
		{Type: "text", Text: "```sql\nSELECT 1;\n```"},
	}}
	if got := extrairTextoAnthropic(r); got != "```sql\nSELECT 1;\n```" {
		t.Errorf("extrairTextoAnthropic = %q", got)
	}
}
