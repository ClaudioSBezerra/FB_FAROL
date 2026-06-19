package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// farol_pulso.go — "Pulso de Ontem" (grão diário).
//
// Responde à pergunta do CEO/supervisor: "as vendas caíram ontem?".
// Todo o resto do FAROL roda em grão mensal (agg_*_mes); o pulso lê o diário
// cru de vendas_transmitidas (pedidos = indicador antecedente do faturamento).
//
// Métricas: Vlr Transmitido (SUM pvenda) + Qtd Pedidos (COUNT DISTINCT cnpj =
// clientes que transmitiram no dia; a tabela é 1 linha por item, sem nº de
// pedido, então o CNPJ distinto é o proxy honesto de "pedidos do dia").
//
// Régua de comparação: MESMO DIA DA SEMANA, 7 DIAS ATRÁS.
//   Ontem (qua) × quarta passada. Compara dia-da-semana igual → elimina o falso
//   alarme de fim de semana/segunda sem precisar contar dia útil nem feriados.
//
// "Ontem" = MAX(data_transmissao) do recorte (último dia com pedido importado),
// não o calendário — evita mostrar zero num dia ainda não importado.
//
// FUTURO: quando existir objetivo diário por RCA, troca-se a régua de "7 dias
// atrás" para "realizado vs meta do dia" — computePulso já isola isso.

type pulsoResp struct {
	DiaRef       string  `json:"dia_ref"`       // YYYY-MM-DD do último dia com venda
	DiaRefLabel  string  `json:"dia_ref_label"` // "18/jun · qua"
	VlAtual      float64 `json:"vl_atual"`      // transmitido do dia ref
	QtAtual      int     `json:"qt_atual"`      // pedidos (cnpj distinto) do dia ref
	VlEspelho    float64 `json:"vl_espelho"`    // transmitido 7 dias antes
	QtEspelho    int     `json:"qt_espelho"`    // pedidos 7 dias antes
	EspelhoLabel string  `json:"espelho_label"` // "qua passada (11/jun)"
	EspelhoData  string  `json:"espelho_data"`  // YYYY-MM-DD do dia espelho
	Pct          float64 `json:"pct"`           // vl_atual / vl_espelho * 100 (foco em valor)
	PctQt        float64 `json:"pct_qt"`        // qt_atual / qt_espelho * 100
	Cor          string  `json:"cor"`           // verde/amarelo/vermelho (pelo valor)
	Parcial      bool    `json:"parcial"`       // true se dia_ref não é hoje (dado pode crescer)
	SemDado      bool    `json:"sem_dado"`      // true se não há transmissão no recorte
}

var mesAbrev = []string{"", "jan", "fev", "mar", "abr", "mai", "jun", "jul", "ago", "set", "out", "nov", "dez"}
var diaSemAbrev = []string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}

// diaTotais retorna SUM(pvenda) e COUNT(DISTINCT cnpj) de um dia específico no
// recorte. extraCond/extraArgs aplicam filtro de supervisor/rca ($3, $4...).
func diaTotais(db *sql.DB, empresaID string, dia time.Time, extraCond string, extraArgs []any) (float64, int) {
	args := append([]any{empresaID, dia}, extraArgs...)
	var v float64
	var qt int
	_ = db.QueryRow(`
		SELECT COALESCE(SUM(pvenda),0),
		       COUNT(DISTINCT cnpj) FILTER (WHERE cnpj <> '')
		FROM vendas_transmitidas
		WHERE empresa_id=$1 AND data_transmissao = $2`+extraCond, args...).Scan(&v, &qt)
	return v, qt
}

// computePulso compara o último dia com venda × o mesmo dia 7 dias antes.
// extraCond usa placeholders a partir de $3 (após empresa, dia); extraArgs traz
// os valores. Para a query de MAX (só empresa em $1), os placeholders são
// deslocados de $3→$2.
func computePulso(db *sql.DB, empresaID, extraCond string, extraArgs []any) pulsoResp {
	var out pulsoResp

	// 1) Dia de referência = último dia com transmissão no recorte.
	argsMax := append([]any{empresaID}, extraArgs...)
	condMax := shiftPlaceholders(extraCond, 2, 1) // $3→$2, $4→$3
	var diaRefN sql.NullTime
	err := db.QueryRow(`
		SELECT MAX(data_transmissao)
		FROM vendas_transmitidas
		WHERE empresa_id=$1`+condMax, argsMax...).Scan(&diaRefN)
	if err != nil || !diaRefN.Valid {
		out.SemDado = true
		out.Cor = "vermelho"
		return out
	}
	diaRef := diaRefN.Time
	diaEsp := diaRef.AddDate(0, 0, -7)

	// 2) Totais do dia ref e do espelho (7 dias antes).
	out.VlAtual, out.QtAtual = diaTotais(db, empresaID, diaRef, extraCond, extraArgs)
	out.VlEspelho, out.QtEspelho = diaTotais(db, empresaID, diaEsp, extraCond, extraArgs)

	// 3) Labels, percentuais e cor (cor segue o VALOR, métrica principal).
	out.DiaRef = diaRef.Format("2006-01-02")
	out.DiaRefLabel = diaRef.Format("02") + "/" + mesAbrev[int(diaRef.Month())] +
		" · " + diaSemAbrev[int(diaRef.Weekday())]
	out.EspelhoData = diaEsp.Format("2006-01-02")
	out.EspelhoLabel = diaSemAbrev[int(diaEsp.Weekday())] + " passada (" +
		diaEsp.Format("02") + "/" + mesAbrev[int(diaEsp.Month())] + ")"
	out.Pct = calcPct(out.VlEspelho, out.VlAtual)
	out.PctQt = calcPct(float64(out.QtEspelho), float64(out.QtAtual))
	out.Cor = farolCor(out.Pct)

	// Parcial: dia de referência não é hoje (UTC) → dado ainda pode crescer.
	hoje := time.Now().UTC()
	out.Parcial = !(diaRef.Year() == hoje.Year() && diaRef.YearDay() == hoje.YearDay())

	return out
}

// shiftPlaceholders reescreve placeholders $N de extraCond. extraCond chega com
// placeholders a partir de $(from+1); desloca para $(toBase+1)... Ex:
// shift(cond, 2, 1) leva $3→$2, $4→$3.
func shiftPlaceholders(extraCond string, from, toBase int) string {
	if extraCond == "" {
		return ""
	}
	out := extraCond
	// Do maior pro menor pra não colidir (até 4 filtros).
	for k := 4; k >= 1; k-- {
		src := "$" + strconv.Itoa(from+k)
		dst := "$" + strconv.Itoa(toBase+k)
		out = strings.ReplaceAll(out, src, dst)
	}
	return out
}

// FarolPulsoHandler — GET /api/farol/sup-pulso/{cod}?cnpj=&cod_rca=
func FarolPulsoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// path: /api/farol/sup-pulso/{cod}
		p := strings.TrimPrefix(r.URL.Path, "/api/farol/sup-pulso/")
		p = strings.Trim(p, "/")
		codSup, err := strconv.Atoi(p)
		if err != nil || codSup <= 0 {
			http.Error(w, `{"error":"cod_supervisor inválido"}`, http.StatusBadRequest)
			return
		}

		cnpj := normalizeCnpjQuery(r.URL.Query().Get("cnpj"))
		empresaID, _, ok := resolveSupervisor(db, codSup, cnpj)
		if !ok {
			http.Error(w, `{"error":"supervisor não encontrado"}`, http.StatusNotFound)
			return
		}

		// Filtros a partir de $3 (após empresa, dia em diaTotais).
		extraCond := " AND cod_supervisor=$3"
		extraArgs := []any{strconv.Itoa(codSup)}
		if rcaQ := strings.TrimSpace(r.URL.Query().Get("cod_rca")); rcaQ != "" {
			if rca, e := strconv.Atoi(rcaQ); e == nil && rca > 0 {
				extraCond += " AND cod_rca=$4"
				extraArgs = append(extraArgs, strconv.Itoa(rca))
			}
		}

		out := computePulso(db, empresaID, extraCond, extraArgs)
		json.NewEncoder(w).Encode(out)
	}
}

// FarolPulsoEmpresaHandler — GET /api/v2/farol/pulso (empresa inteira, p/ BI).
// Usa o contexto autenticado (spCtx) em vez de CNPJ público.
func FarolPulsoEmpresaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		out := computePulso(db, spCtx.EmpresaID, "", nil)
		json.NewEncoder(w).Encode(out)
	}
}
