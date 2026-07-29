// jc_agendador.go — agendamento diário da carga do JC + gatilho manual.
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// jcHoraExtracao — horário do disparo diário. Default 06:30.
//
// A escolha depende do JOB que popula as tabelas no lado do JC: a view é COMUM
// (lê as tabelas base direto), então extrair antes do JOB terminar traz o dia
// incompleto — e pior, silenciosamente, porque o resultado seria um número
// menor, não um erro. 06:30 fica depois da janela 00:01-06:00 combinada com
// eles. Ajustar por JC_EXTRACAO_HORA quando o horário real do JOB for confirmado.
func jcHoraExtracao() (hora, minuto int) {
	hora, minuto = 6, 30
	v := strings.TrimSpace(os.Getenv("JC_EXTRACAO_HORA"))
	if v == "" {
		return hora, minuto
	}
	var h, m int
	if _, err := fmt.Sscanf(v, "%d:%d", &h, &m); err != nil ||
		h < 0 || h > 23 || m < 0 || m > 59 {
		log.Printf("[jc:carga] JC_EXTRACAO_HORA=%q inválido, usando %02d:%02d", v, hora, minuto)
		return hora, minuto
	}
	return h, m
}

// jcConfigurado — sem credencial ou empresa não há o que agendar. Distinguir
// "desligado de propósito" de "quebrado" evita log de erro todo dia em ambiente
// de desenvolvimento, onde essas variáveis não existem.
func jcConfigurado() bool {
	return strings.TrimSpace(os.Getenv("JC_ORACLE_USER")) != "" &&
		strings.TrimSpace(os.Getenv("JC_ORACLE_PASS")) != "" &&
		strings.TrimSpace(os.Getenv("JC_EMPRESA_ID")) != ""
}

// StartCargaJCDiaria roda a carga todo dia no horário configurado, sempre para
// D-1 (dia fechado). Segue o mesmo formato do StartDailyPrewarm.
func StartCargaJCDiaria(db *sql.DB) {
	if !jcConfigurado() {
		log.Printf("[jc:carga] desativada — faltam JC_ORACLE_USER/JC_ORACLE_PASS/JC_EMPRESA_ID")
		return
	}
	loc := tzBrasil()
	hora, minuto := jcHoraExtracao()
	log.Printf("[jc:carga] agendada para %02d:%02d (%s), sempre D-1", hora, minuto, loc)

	for {
		agora := time.Now().In(loc)
		prox := time.Date(agora.Year(), agora.Month(), agora.Day(), hora, minuto, 0, 0, loc)
		if !prox.After(agora) {
			prox = prox.AddDate(0, 0, 1)
		}
		espera := time.Until(prox)
		log.Printf("[jc:carga] próxima execução %s (em %s)",
			prox.Format("2006-01-02 15:04"), espera.Truncate(time.Minute))
		time.Sleep(espera)

		// D-1 relativo ao horário de Brasília, não ao UTC do container: rodando
		// 06:30 BRT (09:30 UTC), o "ontem" em UTC seria o mesmo dia, e a carga
		// pegaria o dia errado.
		ontem := time.Now().In(loc).AddDate(0, 0, -1)
		ExecutarCargaJC(db, time.Date(ontem.Year(), ontem.Month(), ontem.Day(), 0, 0, 0, 0, time.UTC))
	}
}

// CargaJCManualHandler — carga sob demanda, um dia ou um intervalo.
//
//	POST /api/v2/jc/carga?data=2026-07-28                     um dia
//	POST /api/v2/jc/carga?de=2026-07-01&ate=2026-07-29        intervalo
//	POST /api/v2/jc/carga?de=...&ate=...&pular_existentes=1   backfill
//
// Sem parâmetro, assume D-1. Existe para testar antes do primeiro disparo
// automático, reprocessar dia que falhou, e cobrir buraco quando o JOB da origem
// atrasar. Roda em background e devolve na hora, porque a carga leva minutos e o
// proxy cortaria a conexão.
//
// No intervalo os dias são processados em SEQUÊNCIA e sai UM e-mail no fim —
// paralelizar competiria com o JOB de replicação do lado deles, e 29 e-mails
// separados escondem o que importa.
func CargaJCManualHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !RequireWrite(spCtx, w) {
			return
		}
		if !jcConfigurado() {
			http.Error(w, `{"error":"carga JC não configurada no ambiente"}`, http.StatusPreconditionFailed)
			return
		}

		loc := tzBrasil()
		q := r.URL.Query()
		soData := func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		}

		// ── Modo intervalo ──────────────────────────────────────────────────
		deStr := strings.TrimSpace(q.Get("de"))
		ateStr := strings.TrimSpace(q.Get("ate"))
		if deStr != "" || ateStr != "" {
			if deStr == "" || ateStr == "" {
				http.Error(w, `{"error":"informe 'de' E 'ate' (AAAA-MM-DD)"}`, http.StatusBadRequest)
				return
			}
			de, errDe := time.Parse("2006-01-02", deStr)
			ate, errAte := time.Parse("2006-01-02", ateStr)
			if errDe != nil || errAte != nil {
				http.Error(w, `{"error":"datas inválidas — use AAAA-MM-DD"}`, http.StatusBadRequest)
				return
			}
			de, ate = soData(de), soData(ate)
			if ate.Before(de) {
				http.Error(w, `{"error":"'ate' anterior a 'de'"}`, http.StatusBadRequest)
				return
			}
			dias := int(ate.Sub(de).Hours()/24) + 1
			if dias > jcMaxDiasIntervalo {
				http.Error(w, fmt.Sprintf(
					`{"error":"intervalo de %d dias excede o limite de %d — cada dia custa ~5min"}`,
					dias, jcMaxDiasIntervalo), http.StatusBadRequest)
				return
			}
			pular := q.Get("pular_existentes") == "1" || strings.EqualFold(q.Get("pular_existentes"), "true")

			log.Printf("[jc:carga] disparo MANUAL INTERVALO %s..%s (%d dias, pular_existentes=%t) por user=%s",
				de.Format("2006-01-02"), ate.Format("2006-01-02"), dias, pular, spCtx.UserID)
			go ExecutarCargaJCIntervalo(db, de, ate, pular)

			json.NewEncoder(w).Encode(map[string]any{
				"iniciado":         true,
				"de":               de.Format("2006-01-02"),
				"ate":              ate.Format("2006-01-02"),
				"dias":             dias,
				"pular_existentes": pular,
				"estimativa":       fmt.Sprintf("~%d min", dias*5),
				"aviso":            "roda em background, um dia por vez; UM e-mail com o consolidado no fim",
			})
			return
		}

		// ── Modo um dia ─────────────────────────────────────────────────────
		data := time.Now().In(loc).AddDate(0, 0, -1)
		if s := strings.TrimSpace(q.Get("data")); s != "" {
			t, err := time.Parse("2006-01-02", s)
			if err != nil {
				http.Error(w, `{"error":"data inválida — use AAAA-MM-DD"}`, http.StatusBadRequest)
				return
			}
			data = t
		}
		dia := soData(data)

		log.Printf("[jc:carga] disparo MANUAL para %s por user=%s", dia.Format("2006-01-02"), spCtx.UserID)
		go ExecutarCargaJC(db, dia)

		json.NewEncoder(w).Encode(map[string]any{
			"iniciado": true,
			"data":     dia.Format("2006-01-02"),
			"aviso":    "roda em background; o resumo chega por e-mail ao terminar",
		})
	}
}
