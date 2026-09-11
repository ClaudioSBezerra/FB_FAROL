package handlers

// farol_metas_congelamento.go — Congelamento de mês fechado (Épico 4,
// Story 4.3, módulo Painel de Gestão de Metas por Indústria)
//
// Até a Story 4.2, o Realizado era sempre calculado ao vivo — não havia
// "congelamento" de verdade porque nada era persistido. Esta story mudou
// isso pra vigência FECHADA: congela no primeiro cálculo (snapshot) e só
// muda de novo por reprocessamento manual explícito (FR17).
//
// NFR3 (reprodutibilidade): servir do snapshot em vez de recalcular é o que
// garante "mesmo snapshot de dados, resultado não muda entre duas
// execuções" — mesmo que a base de vendas mude depois (planilha do
// fornecedor corrigida), o número já congelado não se move sozinho.
//
// ─── Ajuste 2026-09-11 (decisão do Claudio) ───────────────────────────────
// Vigência ABERTA (o mês corrente — o que praticamente todo mundo olha o
// dia inteiro, web E mobile via ION VENDAS) também passa a servir de
// snapshot gravado no banco, em vez de recalcular ao vivo a cada request.
// Motivo: CalcularRealizado varre vendas_faturadas/vendas_transmitidas pra
// TODOS os clientes válidos do vínculo (ex: 379 CNPJs da Unilever) — com
// dezenas de pessoas abrindo o painel/app no mesmo dia, era o mesmo cálculo
// caro repetido dezenas de vezes, quando o dado-fonte só muda 1x/dia (carga
// JC). O snapshot da vigência aberta é renovado 1x/dia pelo prewarm
// (farol_metas_prewarm.go), logo depois da carga diária — mesma cadência
// que já vale pra agg_mes/baseCache no Painel Geral, só que persistido (não
// em memória) porque este projeto tem deploy frequente.
//
// Isso relaxa deliberadamente "vigência aberta reflete dado novo na hora"
// — nunca foi FR/NFR numerado (PRD só exige congelamento pra mês FECHADO,
// FR17/NFR3); era só o efeito colateral de não existir cache nenhum até
// aqui. Primeiro acesso a uma combinação sem snapshot ainda calcula ao vivo
// e grava na hora (auto-cura, mesmo padrão do congelamento de fechada) —
// então nada fica em branco esperando o prewarm rodar.
//
// Os 4 recortes de tempo (FR21: dia_anterior/semana/mes/ano_corrente) e a
// vigência aberta usam a MESMA tabela, distinguidos pela coluna `recorte`
// (migration 234) — motivo='snapshot_diario' pra não confundir com o
// congelamento de verdade ('congelamento_automatico'/'reprocessamento_manual').

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// obterOuCongelarRealizado é o ponto de entrada real do endpoint de leitura
// pra vigência inteira (recorte "") — serve do snapshot (aberta OU
// fechada), calculando e gravando na primeira vez que uma combinação ainda
// não tem snapshot.
func obterOuCongelarRealizado(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel string) (*RealizadoResultado, error) {
	var status string
	if err := db.QueryRow(`SELECT status FROM farol.metas_vigencias WHERE id = $1 AND vinculo_id = $2 AND empresa_id = $3`, vigenciaID, vinculoID, empresaID).Scan(&status); err != nil {
		return nil, err
	}

	motivoAutomatico := "congelamento_automatico"
	if status == "aberta" {
		motivoAutomatico = "snapshot_diario"
	}
	return obterOuCalcularSnapshot(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, "", "", "", motivoAutomatico)
}

// obterOuCalcularRecorte é o equivalente de obterOuCongelarRealizado pros 4
// recortes de tempo (FR21) — dataInicio/dataFim vêm de calcularRecorteDatas.
// Antes de 2026-09-11 os recortes eram SEMPRE ao vivo; agora seguem o mesmo
// snapshot diário da vigência aberta (ver cabeçalho do arquivo).
func obterOuCalcularRecorte(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel, recorte string) (*RealizadoResultado, error) {
	dataInicio, dataFim, err := calcularRecorteDatas(recorte)
	if err != nil {
		return nil, err
	}
	return obterOuCalcularSnapshot(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, recorte, dataInicio, dataFim, "snapshot_diario")
}

// obterOuCalcularSnapshot é o núcleo compartilhado: tenta servir da linha já
// gravada (vigencia_id, fluxo, nivel, recorte); no miss, calcula ao vivo e
// grava (auto-cura — cobre o primeiro acesso do dia, antes do prewarm
// rodar, e qualquer vínculo/vigência novo que o prewarm ainda não viu).
func obterOuCalcularSnapshot(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel, recorte, dataInicioOverride, dataFimOverride, motivoAutomatico string) (*RealizadoResultado, error) {
	var raw []byte
	err := db.QueryRow(`
		SELECT resultado_json FROM farol.metas_realizados_snapshot
		WHERE vigencia_id = $1 AND fluxo = $2 AND nivel = $3 AND recorte = $4 AND empresa_id = $5
	`, vigenciaID, fluxo, nivel, recorte, empresaID).Scan(&raw)
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

	resultado, cerr := CalcularRealizadoComPeriodo(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, dataInicioOverride, dataFimOverride)
	if cerr != nil {
		return nil, cerr
	}
	if serr := salvarSnapshot(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, recorte, resultado, motivoAutomatico); serr != nil {
		log.Printf("MetasCongelamento: falha ao gravar snapshot (vinculo=%d vigencia=%d recorte=%q): %v", vinculoID, vigenciaID, recorte, serr)
	}
	return resultado, nil
}

func salvarSnapshot(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo, nivel, recorte string, resultado *RealizadoResultado, motivo string) error {
	raw, err := json.Marshal(resultado)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO farol.metas_realizados_snapshot (empresa_id, vinculo_id, vigencia_id, fluxo, nivel, recorte, resultado_json, motivo, calculado_em)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (vigencia_id, fluxo, nivel, recorte)
		DO UPDATE SET resultado_json = $7, motivo = $8, calculado_em = now()
	`, empresaID, vinculoID, vigenciaID, fluxo, nivel, recorte, raw, motivo)
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
		if err := salvarSnapshot(db, spCtx.EmpresaID, vinculoID, vigenciaID, fluxo, nivel, "", resultado, "reprocessamento_manual"); err != nil {
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
