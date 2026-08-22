// farol_resumo.go — disparo manual e prévia do resumo semanal.
//
// A prévia existe porque a primeira versão de um e-mail que vai para a
// diretoria não deve estrear na caixa da diretoria. `previa=1` monta tudo,
// devolve o HTML e não envia nem registra nada.
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"fb_farol/services"
)

// FarolResumoSemanalHandler — POST /api/v2/farol/resumo-semanal
//
//	?previa=1              monta e devolve o HTML, não envia
//	?forcar=1              ignora o log da semana (para testar)
//	?ano=&mes=             default: mês corrente
//	?ate=AAAA-MM-DD        default: ontem
//	?baseline=meta         default: ano_anterior (não há meta cadastrada)
func FarolResumoSemanalHandler(db *sql.DB) http.HandlerFunc {
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
		// Quem tem escopo não dispara: o resumo é montado para TODOS os
		// destinatários de uma vez, e a prévia devolveria o corpo do e-mail
		// alheio — inclusive o da diretoria — para quem só pode ver a própria
		// equipe. É a mesma razão que barra GGV no text-to-SQL.
		if escopoDoUsuario(db, spCtx, "").restrito() {
			log.Printf("[farol:resumo] acesso negado — persona=%s user=%s",
				spCtx.TipoPersona, spCtx.UserID)
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}

		loc := tzBrasil()
		agora := time.Now().In(loc)
		q := r.URL.Query()

		ano, mes := agora.Year(), int(agora.Month())
		if v, err := strconv.Atoi(q.Get("ano")); err == nil && v >= 2000 && v <= 2100 {
			ano = v
		}
		if v, err := strconv.Atoi(q.Get("mes")); err == nil && v >= 1 && v <= 12 {
			mes = v
		}

		// Ontem por padrão: o dia corrente ainda está sendo faturado e entraria
		// pela metade, achatando o ritmo de todo mundo.
		ate := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
		if s := strings.TrimSpace(q.Get("ate")); s != "" {
			if t, err := time.Parse("2006-01-02", s); err == nil {
				ate = t
			} else {
				http.Error(w, `{"error":"ate inválido — use AAAA-MM-DD"}`, http.StatusBadRequest)
				return
			}
		}

		base := services.BaselineAnoAnterior
		if strings.EqualFold(q.Get("baseline"), "meta") {
			base = services.BaselineMeta
		}

		previa := q.Get("previa") == "1"
		forcar := q.Get("forcar") == "1"

		// ?para=email — cópia avulsa para conferir antes da segunda. Sai com o
		// recorte de quem pediu e idêntica à real, para poder ser
		// reencaminhada. Não entra no log do envio semanal.
		if para := strings.TrimSpace(q.Get("para")); para != "" {
			if !strings.Contains(para, "@") {
				http.Error(w, `{"error":"destinatário inválido"}`, http.StatusBadRequest)
				return
			}
			var nome string
			_ = db.QueryRow(`SELECT COALESCE(NULLIF(full_name,''), email) FROM users WHERE id=$1`,
				spCtx.UserID).Scan(&nome)

			log.Printf("[farol:resumo] CÓPIA DE TESTE para %s, recorte de user=%s persona=%s",
				para, spCtx.UserID, spCtx.TipoPersona)

			r, err := services.EnviarResumoTeste(db, spCtx.EmpresaID, ano, mes, ate, base,
				nome, spCtx.TipoPersona, spCtx.CodReferencia, para)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enviado_para": para,
				"recorte_de":   nome,
				"escopo":       r.Escopo,
				"periodo":      fmt.Sprintf("%04d-%02d", ano, mes),
				"total_mesa":   r.TotalMesa,
				"grupos":       len(r.Grupos),
				"aviso":        "cópia de teste — não conta como envio da semana",
			})
			return
		}

		log.Printf("[farol:resumo] disparo %04d-%02d ate=%s baseline=%s previa=%t forcar=%t por user=%s",
			ano, mes, ate.Format("2006-01-02"), base, previa, forcar, spCtx.UserID)

		res, err := services.EnviarResumoSemanal(db, spCtx.EmpresaID, ano, mes, ate, base, previa, forcar)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}

		enviados := 0
		for _, x := range res {
			if x.Enviado {
				enviados++
			}
		}
		log.Printf("[farol:resumo] %d destinatário(s), %d enviado(s)", len(res), enviados)

		_ = json.NewEncoder(w).Encode(map[string]any{
			"periodo":       fmt.Sprintf("%04d-%02d", ano, mes),
			"ate":           ate.Format("2006-01-02"),
			"baseline":      string(base),
			"previa":        previa,
			"destinatarios": len(res),
			"enviados":      enviados,
			"resultados":    res,
		})
	}
}

// FarolResumoPreviaHTMLHandler — GET /api/v2/farol/resumo-semanal/previa
//
// Devolve o e-mail do PRÓPRIO usuário renderizado como página, para abrir no
// navegador. Ver o HTML dentro de um JSON escapado não diz nada sobre como o
// e-mail vai chegar.
func FarolResumoPreviaHTMLHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		loc := tzBrasil()
		agora := time.Now().In(loc)
		q := r.URL.Query()

		ano, mes := agora.Year(), int(agora.Month())
		if v, err := strconv.Atoi(q.Get("ano")); err == nil && v >= 2000 && v <= 2100 {
			ano = v
		}
		if v, err := strconv.Atoi(q.Get("mes")); err == nil && v >= 1 && v <= 12 {
			mes = v
		}
		ate := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)

		todos, cob, err := services.ColetarDinheiroNaMesa(db, spCtx.EmpresaID, ano, mes, ate,
			services.BaselineAnoAnterior)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Sempre o recorte de QUEM PEDIU. Um preview que mostrasse o e-mail de
		// outra pessoa seria uma porta lateral para o escopo.
		nome := spCtx.UserID
		var full string
		if db.QueryRow(`SELECT COALESCE(NULLIF(full_name,''), email) FROM users WHERE id=$1`,
			spCtx.UserID).Scan(&full) == nil && full != "" {
			nome = full
		}
		nomes := services.NomesGerentesSupervisores(db, spCtx.EmpresaID, ano, mes)
		resumo := services.MontarResumo(todos, cob, nome, spCtx.TipoPersona, spCtx.CodReferencia,
			nomes, services.RotuloMes(ano, mes))

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(services.CorpoHTML(resumo)))
	}
}

// FarolDinheiroNaMesaHandler — GET /api/v2/farol/dinheiro-na-mesa
//
// Os mesmos números do e-mail, em JSON, para a página do painel. Reusa o motor
// inteiro: se um dia o e-mail e a tela discordarem, é porque alguém duplicou o
// cálculo — e não vai ser aqui.
//
// Sempre com o recorte de QUEM PEDIU. Não aceita parâmetro de escopo: o e-mail
// já manda o link certo para cada um, e um `?gerente=` aqui seria uma porta
// lateral para um supervisor ler a carteira do vizinho.
func FarolDinheiroNaMesaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		loc := tzBrasil()
		agora := time.Now().In(loc)
		q := r.URL.Query()

		ano, mes := agora.Year(), int(agora.Month())
		if v, err := strconv.Atoi(q.Get("ano")); err == nil && v >= 2000 && v <= 2100 {
			ano = v
		}
		if v, err := strconv.Atoi(q.Get("mes")); err == nil && v >= 1 && v <= 12 {
			mes = v
		}
		ate := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)

		var nome string
		if db.QueryRow(`SELECT COALESCE(NULLIF(full_name,''), email) FROM users WHERE id=$1`,
			spCtx.UserID).Scan(&nome) != nil {
			nome = ""
		}
		res, err := services.MontarComAno(db, spCtx.EmpresaID, nome,
			spCtx.TipoPersona, spCtx.CodReferencia, ano, mes, ate, services.BaselineAnoAnterior)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(res)
	}
}

// FarolQuadroPublicoHandler — GET /api/v2/farol/quadro/{token}
//
// SEM autenticação: o token na URL É a credencial. Foi decisão consciente de
// 22/08/2026 para o link do WhatsApp abrir sem login no celular.
//
// O que limita o estrago está no desenho, não aqui: um token por pessoa
// (rastreável e revogável isoladamente), escopo do dono, e SÓ esta tela — os
// links de "abrir o painel" continuam exigindo login. O que vaza é um quadro de
// números agregados, não o sistema.
func FarolQuadroPublicoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Não indexar: o link circula em mensagem, e mensagem vira página
		// pública quando alguém cola num grupo com prévia de link.
		w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")

		token := strings.TrimPrefix(r.URL.Path, "/api/v2/farol/quadro/")
		if len(token) != 64 {
			http.Error(w, `{"error":"link inválido"}`, http.StatusNotFound)
			return
		}

		_, nome, persona, codRef, ok := services.ResolverTokenQuadro(db, token)
		if !ok {
			// 404 e não 403: dizer "existe mas está revogado" confirmaria para
			// quem tentou que o token um dia foi válido.
			http.Error(w, `{"error":"link inválido ou revogado"}`, http.StatusNotFound)
			return
		}

		empresaID := strings.TrimSpace(os.Getenv("JC_EMPRESA_ID"))
		if empresaID == "" {
			http.Error(w, `{"error":"empresa não configurada"}`, http.StatusInternalServerError)
			return
		}

		loc := tzBrasil()
		agora := time.Now().In(loc)
		ate := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)

		res, err := services.MontarComAno(db, empresaID, nome, persona, codRef,
			agora.Year(), int(agora.Month()), ate, services.BaselineAnoAnterior)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":%q}`, err.Error()), http.StatusInternalServerError)
			return
		}

		// O link do painel sai da resposta: ele exige login, e um botão que
		// leva à tela de senha em cima de um link "que não pede senha" só gera
		// a impressão de que algo quebrou.
		res.LinkPainel = ""
		_ = json.NewEncoder(w).Encode(res)
	}
}
