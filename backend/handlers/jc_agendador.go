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

// CargaJCManualHandler — POST /api/v2/jc/carga?data=2026-07-28
//
// Existe para três coisas: testar antes do primeiro disparo automático,
// reprocessar um dia que falhou, e cobrir buraco quando o JOB da origem atrasar.
// Sem `data`, assume D-1. Roda em background e devolve na hora, porque a carga
// leva minutos e o proxy cortaria a conexão.
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
		data := time.Now().In(loc).AddDate(0, 0, -1)
		if s := strings.TrimSpace(r.URL.Query().Get("data")); s != "" {
			t, err := time.Parse("2006-01-02", s)
			if err != nil {
				http.Error(w, `{"error":"data inválida — use AAAA-MM-DD"}`, http.StatusBadRequest)
				return
			}
			data = t
		}
		dia := time.Date(data.Year(), data.Month(), data.Day(), 0, 0, 0, 0, time.UTC)

		log.Printf("[jc:carga] disparo MANUAL para %s por user=%s", dia.Format("2006-01-02"), spCtx.UserID)
		go ExecutarCargaJC(db, dia)

		json.NewEncoder(w).Encode(map[string]any{
			"iniciado": true,
			"data":     dia.Format("2006-01-02"),
			"aviso":    "roda em background; o resumo chega por e-mail ao terminar",
		})
	}
}
