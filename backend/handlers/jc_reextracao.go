// jc_reextracao.go — reextração periódica de uma janela móvel de meses.
//
// POR QUE EXISTE: devolução e cancelamento entram na origem com data
// RETROATIVA. Uma devolução lançada hoje contra uma venda de março aparece na
// view com DATA de março — e a carga diária, que busca sempre D-1, nunca vai
// encontrá-la. Medido em 06/08/2026: jan/2025 tinha 17.345 linhas de CCD
// distintas na origem contra 13.642 no nosso banco, e o próprio 31/07 ganhou 6
// devoluções entre a sonda da manhã e a carga da tarde.
//
// Recarregar uma vez conserta o passado e o problema volta na semana seguinte.
// Só uma reextração periódica sobre uma janela que ANDA resolve de forma
// estável.
//
// DESLIGADO POR PADRÃO. Sem JC_REEXTRACAO_MESES o agendador nem sobe — não faz
// sentido um job de ~2h aparecer em produção só porque o código foi deployado.
// Para ligar, defina a variável no Coolify.
//
// Env:
//
//	JC_REEXTRACAO_MESES  quantos meses para trás (vazio/0 = DESLIGADO)
//	JC_REEXTRACAO_DIA    0=domingo .. 6=sábado (default 0)
//	JC_REEXTRACAO_HORA   HH:MM no horário de Brasília (default 06:00)
package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// janelaReextracao — do primeiro dia do mês N-1 meses atrás até ONTEM.
//
// Vai só até ontem, não até hoje, pela mesma razão da carga diária: o dia
// corrente ainda está sendo escrito na origem e traria movimento parcial.
//
// `meses` conta o mês corrente. meses=3 em 07/08/2026 devolve
// 01/06/2026 → 06/08/2026: junho, julho e o pedaço de agosto.
func janelaReextracao(hoje time.Time, meses int) (de, ate time.Time) {
	if meses < 1 {
		meses = 1
	}
	ate = time.Date(hoje.Year(), hoje.Month(), hoje.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	ini := time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC)
	de = ini.AddDate(0, -(meses - 1), 0)
	return de, ate
}

// proximaReextracao — próxima ocorrência de (diaSemana, hora:minuto) DEPOIS de
// agora. Se hoje é o dia certo mas o horário já passou, pula para a semana que
// vem; se ainda não passou, é hoje mesmo.
func proximaReextracao(agora time.Time, diaSemana time.Weekday, hora, minuto int) time.Time {
	alvo := time.Date(agora.Year(), agora.Month(), agora.Day(), hora, minuto, 0, 0, agora.Location())
	delta := (int(diaSemana) - int(agora.Weekday()) + 7) % 7
	alvo = alvo.AddDate(0, 0, delta)
	if !alvo.After(agora) {
		alvo = alvo.AddDate(0, 0, 7)
	}
	return alvo
}

// jcReextracaoConfig lê as três variáveis. ok=false significa desligado.
func jcReextracaoConfig() (meses int, dia time.Weekday, hora, minuto int, ok bool) {
	raw := strings.TrimSpace(os.Getenv("JC_REEXTRACAO_MESES"))
	if raw == "" {
		return 0, 0, 0, 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		if err != nil || n != 0 {
			log.Printf("[jc:reextracao] JC_REEXTRACAO_MESES=%q inválido — reextração desligada", raw)
		}
		return 0, 0, 0, 0, false
	}
	// Teto igual ao do endpoint manual: acima disso a janela deixa de ser
	// "recente" e vira recarga histórica, que é operação com acompanhamento.
	if n > 6 {
		log.Printf("[jc:reextracao] JC_REEXTRACAO_MESES=%d é alto demais para um job semanal, limitando a 6", n)
		n = 6
	}

	dia = time.Sunday
	if v := strings.TrimSpace(os.Getenv("JC_REEXTRACAO_DIA")); v != "" {
		if d, e := strconv.Atoi(v); e == nil && d >= 0 && d <= 6 {
			dia = time.Weekday(d)
		} else {
			log.Printf("[jc:reextracao] JC_REEXTRACAO_DIA=%q inválido, usando domingo", v)
		}
	}

	hora, minuto = 6, 0
	if v := strings.TrimSpace(os.Getenv("JC_REEXTRACAO_HORA")); v != "" {
		var h, m int
		if _, e := fmt.Sscanf(v, "%d:%d", &h, &m); e == nil && h >= 0 && h <= 23 && m >= 0 && m <= 59 {
			hora, minuto = h, m
		} else {
			log.Printf("[jc:reextracao] JC_REEXTRACAO_HORA=%q inválido, usando %02d:%02d", v, hora, minuto)
		}
	}
	return n, dia, hora, minuto, true
}

// StartReextracaoJC — reextrai a janela móvel uma vez por semana.
//
// O horário default (domingo 06:00) fica DEPOIS da carga diária das 04:30 e do
// prewarm das 05:30, para não competir com nenhum dos dois. A reextração
// invalida o cache que o prewarm acabou de aquecer, mas num domingo de manhã
// isso custa pouco — e a alternativa, rodar antes, deixaria o prewarm
// aquecendo dado que a reextração ia sobrescrever em seguida.
func StartReextracaoJC(db *sql.DB) {
	if !jcConfigurado() {
		return // sem credencial não há o que agendar; a carga diária já loga isso
	}
	meses, dia, hora, minuto, ok := jcReextracaoConfig()
	if !ok {
		log.Printf("[jc:reextracao] desligada (defina JC_REEXTRACAO_MESES para ligar)")
		return
	}
	loc := tzBrasil()
	log.Printf("[jc:reextracao] agendada para %s %02d:%02d (%s), janela de %d mes(es)",
		diaSemanaPT(dia), hora, minuto, loc, meses)

	for {
		agora := time.Now().In(loc)
		prox := proximaReextracao(agora, dia, hora, minuto)
		log.Printf("[jc:reextracao] próxima execução %s (em %s)",
			prox.Format("2006-01-02 15:04"), time.Until(prox).Truncate(time.Minute))
		time.Sleep(time.Until(prox))

		de, ate := janelaReextracao(time.Now().In(loc), meses)
		log.Printf("[jc:reextracao] iniciando janela %s..%s (passo=mes)",
			de.Format("2006-01-02"), ate.Format("2006-01-02"))
		// pularExistentes=false: o objetivo é justamente REESCREVER meses que já
		// temos, para capturar a devolução que entrou retroativa neles.
		ExecutarCargaJCIntervalo(db, de, ate, false, true, "")
	}
}

func diaSemanaPT(d time.Weekday) string {
	nomes := [...]string{"domingo", "segunda", "terça", "quarta", "quinta", "sexta", "sábado"}
	return nomes[int(d)%7]
}
