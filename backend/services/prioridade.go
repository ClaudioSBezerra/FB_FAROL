package services

import "math"

// CalcularPrioridade retorna um score 0..100 indicando a urgência da proposta.
//
// Combinação determinística (sem IA) sobre dados que já temos no banco:
//
//	score = curva (0..40) + delta (0..30) + alertas (0..20) + giro (0..10)
//
// Faixas:
//
//	80..100 → Crítico    (badge vermelho-escuro)
//	60..79  → Alto       (badge laranja)
//	40..59  → Médio      (badge amarelo)
//	<40     → normal     (sem badge)
func CalcularPrioridade(
	classeVenda *string,
	delta int,
	capacidadeAtual *int,
	giroDiaCx *float64,
	medVendaCx *float64,
	pontoReposicao *int,
) int {
	score := 0.0

	// (1) Curva ABC — máx 40 pontos
	switch sval(classeVenda) {
	case "A":
		score += 40
	case "B":
		score += 20
	case "C":
		score += 5
	}

	// (2) Magnitude do delta relativo à capacidade atual — máx 30 pontos
	if capacidadeAtual != nil && *capacidadeAtual > 0 {
		ratio := math.Abs(float64(delta)) / float64(*capacidadeAtual)
		score += math.Min(ratio*30, 30)
	} else if delta != 0 {
		// sem capacidade conhecida, mas há sugestão de ajuste — meio peso
		score += 15
	}

	// (3) Indicadores de alerta — máx 20 pontos
	mv := fval(medVendaCx)
	cap := iVal(capacidadeAtual)
	pr := iVal(pontoReposicao)

	// GiroCap urgência: giro/dia ≥ capacidade atual → ruptura iminente
	if cap > 0 && mv >= float64(cap) {
		score += 10
	}
	// GPRepos ajustar: giro ≥ ponto de reposição
	if pr > 0 && mv >= float64(pr) {
		score += 6
	}
	// CMEN2DDV: capacidade < 2 dias de venda
	if cap > 0 && mv > 0 && float64(cap)/mv < 2 {
		score += 4
	}

	// (4) Giro absoluto — máx 10 pontos (escala log para não saturar)
	g := fval(giroDiaCx)
	if g > 0 {
		score += math.Min(math.Log(g+1)/math.Log(50)*10, 10)
	}

	if score > 100 {
		score = 100
	}
	return int(math.Round(score))
}

// ── helpers para lidar com ponteiros nulos ───────────────────────────────────

func sval(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func fval(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func iVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
