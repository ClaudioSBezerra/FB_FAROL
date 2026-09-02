package handlers

// farol_metas_congelamento.go — Congelamento de mês fechado (Épico 4,
// Story 4.3, módulo Painel de Gestão de Metas por Indústria)
//
// Até a Story 4.2, o Realizado era sempre calculado ao vivo — não havia
// "congelamento" de verdade porque nada era persistido. Esta story muda
// isso: vigência ABERTA continua sempre ao vivo (é o mês corrente, parcial
// por natureza); vigência FECHADA congela no primeiro cálculo (snapshot) e
// só muda de novo por reprocessamento manual explícito (FR17).
//
// NFR3 (reprodutibilidade): servir do snapshot em vez de recalcular é o que
// garante "mesmo snapshot de dados, resultado não muda entre duas
// execuções" — mesmo que a base de vendas mude depois (planilha do
// fornecedor corrigida), o número já congelado não se move sozinho.

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// obterOuCongelarRealizado é o ponto de entrada real do endpoint de leitura
// — decide entre calcular ao vivo (vigência aberta) ou servir/gravar o
// snapshot congelado (vigência fechada).
func obterOuCongelarRealizado(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel string) (*RealizadoResultado, error) {
	var status string
	if err := db.QueryRow(`SELECT status FROM farol.metas_vigencias WHERE id = $1 AND vinculo_id = $2 AND empresa_id = $3`, vigenciaID, vinculoID, empresaID).Scan(&status); err != nil {
		return nil, err
	}

	if status == "aberta" {
		return CalcularRealizado(db, empresaID, vinculoID, vigenciaID, fluxo, nivel)
	}

	// Vigência fechada: tenta servir do snapshot já congelado.
	var raw []byte
	err := db.QueryRow(`
		SELECT resultado_json FROM farol.metas_realizados_snapshot
		WHERE vigencia_id = $1 AND fluxo = $2 AND nivel = $3 AND empresa_id = $4
	`, vigenciaID, fluxo, nivel, empresaID).Scan(&raw)
	if err == nil {
		var resultado RealizadoResultado
		if jerr := json.Unmarshal(raw, &resultado); jerr != nil {
			return nil, jerr
		}
		return &resultado, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Primeiro acesso a esta combinação depois do fechamento — calcula e
	// congela agora. Não é recálculo automático de um snapshot já existente
	// (isso FR17 proíbe); é a criação do snapshot inicial.
	resultado, cerr := CalcularRealizado(db, empresaID, vinculoID, vigenciaID, fluxo, nivel)
	if cerr != nil {
		return nil, cerr
	}
	if serr := salvarSnapshot(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, resultado, "congelamento_automatico"); serr != nil {
		log.Printf("MetasCongelamento: falha ao gravar snapshot (vinculo=%d vigencia=%d): %v", vinculoID, vigenciaID, serr)
	}
	return resultado, nil
}

func salvarSnapshot(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel string, resultado *RealizadoResultado, motivo string) error {
	raw, err := json.Marshal(resultado)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO farol.metas_realizados_snapshot (empresa_id, vinculo_id, vigencia_id, fluxo, nivel, resultado_json, motivo, calculado_em)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (vigencia_id, fluxo, nivel)
		DO UPDATE SET resultado_json = $6, motivo = $7, calculado_em = now()
	`, empresaID, vinculoID, vigenciaID, fluxo, nivel, raw, motivo)
	return err
}

// ─── MetasRealizadoReprocessarHandler — POST .../metas-realizado/reprocessar ──

// POST /api/farol/metas-realizado/reprocessar?vinculo_id=&vigencia_id=&fluxo=&nivel=
// Único jeito de um snapshot já congelado mudar (FR17) — ação explícita de
// um gestor, sempre auditada (NFR1).
func MetasRealizadoReprocessarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if !hasSpRole(spCtx.SpRole, "gestor_geral") {
			http.Error(w, "Forbidden: gestor_geral necessário — reprocessamento de mês fechado é ação de gestor", http.StatusForbidden)
			return
		}
		vinculoID, err1 := strconv.Atoi(r.URL.Query().Get("vinculo_id"))
		vigenciaID, err2 := strconv.Atoi(r.URL.Query().Get("vigencia_id"))
		if err1 != nil || err2 != nil {
			http.Error(w, "vinculo_id e vigencia_id são obrigatórios", http.StatusBadRequest)
			return
		}
		fluxo := r.URL.Query().Get("fluxo")
		if fluxo == "" {
			fluxo = "faturado"
		}
		nivel := r.URL.Query().Get("nivel")
		if nivel == "" {
			nivel = "rede"
		}

		resultado, err := CalcularRealizado(db, spCtx.EmpresaID, vinculoID, vigenciaID, fluxo, nivel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := salvarSnapshot(db, spCtx.EmpresaID, vinculoID, vigenciaID, fluxo, nivel, resultado, "reprocessamento_manual"); err != nil {
			http.Error(w, "erro ao gravar snapshot: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "metas_realizados_snapshot", strconv.Itoa(vigenciaID), "reprocessar_manual", map[string]any{
			"vinculo_id": vinculoID, "fluxo": fluxo, "nivel": nivel, "realizado_total": resultado.RealizadoTotal,
		})
		log.Printf("MetasCongelamento: reprocessamento manual vinculo=%d vigencia=%d fluxo=%s nivel=%s por %s", vinculoID, vigenciaID, fluxo, nivel, spCtx.UserID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resultado)
	}
}
