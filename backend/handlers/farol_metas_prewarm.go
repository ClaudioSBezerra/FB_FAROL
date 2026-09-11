package handlers

// farol_metas_prewarm.go — Prewarm diário do Painel de Objetivos por
// Indústria (decisão do Claudio, 2026-09-11).
//
// Recalcula e grava (UPSERT) o snapshot de TODA vigência aberta ativa —
// vigência inteira (recorte "") + os 4 recortes de tempo (FR21) — pros dois
// fluxos (faturado/transmitido) e os 4 níveis de apuração (rede/ggv/crv/
// rca). Isso é o que faz o painel (web e o público mobile via ION VENDAS)
// nunca pagar o cálculo ao vivo — ver farol_metas_congelamento.go pro
// racional completo da troca "sempre ao vivo" → "snapshot renovado 1x/dia".
//
// Nível NÃO muda a query cara (CalcularRealizadoComPeriodo sempre varre
// TODOS os clientes válidos do vínculo pra montar as Redes — nivel só
// reagrupa em memória depois, ver agregarPorNivel) — mas o snapshot é
// gravado por nível porque é assim que a tabela/leitura já funcionavam
// desde a migration 223 (congelamento de fechada). Recalcular os 4 níveis
// aqui é redundante mas barato — só acontece 1x/dia, nunca por request.
//
// Chamado de dentro de PrewarmDiario (farol_v2_api.go, roda 1x/dia,
// FAROL_PREWARM_HORA — default 07:30, ajustado pra 04:30 em produção) e
// também depois de qualquer carga manual/backfill via jc:carga — idempotente
// (UPSERT), então rodar de novo no mesmo dia só refaz o mesmo trabalho.

import (
	"database/sql"
	"log"
	"time"
)

var (
	fluxosPrewarmObjetivos   = []string{"faturado", "transmitido"}
	niveisPrewarmObjetivos   = []string{"rede", "ggv", "crv", "rca"}
	recortesPrewarmObjetivos = []string{"", "dia_anterior", "semana", "mes", "ano_corrente"}
)

type vinculoVigenciaAberta struct {
	VinculoID  int
	VigenciaID int
}

// listarVinculosAtivosComVigenciaAberta — só vínculos ATIVOS (cadastro em
// uso) com vigência ABERTA (mês corrente) valem prewarm; vigência fechada
// já congela sozinha no primeiro acesso (migration 223) e não muda mais.
func listarVinculosAtivosComVigenciaAberta(db *sql.DB, empresaID string) ([]vinculoVigenciaAberta, error) {
	rows, err := db.Query(`
		SELECT v.vinculo_id, v.id
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		WHERE v.empresa_id = $1 AND v.status = 'aberta' AND mv.ativo = true
	`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vinculoVigenciaAberta
	for rows.Next() {
		var vv vinculoVigenciaAberta
		if err := rows.Scan(&vv.VinculoID, &vv.VigenciaID); err != nil {
			return nil, err
		}
		out = append(out, vv)
	}
	return out, rows.Err()
}

// prewarmMetasRealizados varre os vínculos ativos com vigência aberta da
// empresa e grava (recorte "" + os 4 recortes, cada fluxo × nível) o
// snapshot no banco. Erros de UM vínculo/combinação só logam e seguem pras
// próximas — uma Indústria com Cliente Válido faltando não pode travar o
// prewarm das outras.
func prewarmMetasRealizados(db *sql.DB, empresaID string) {
	t0 := time.Now()
	vinculos, err := listarVinculosAtivosComVigenciaAberta(db, empresaID)
	if err != nil {
		log.Printf("[farol:objetivos] prewarm: falha ao listar vínculos ativos: %v", err)
		return
	}
	if len(vinculos) == 0 {
		return
	}
	n, falhas := 0, 0
	for _, vv := range vinculos {
		for _, fluxo := range fluxosPrewarmObjetivos {
			for _, nivel := range niveisPrewarmObjetivos {
				for _, recorte := range recortesPrewarmObjetivos {
					var resultado *RealizadoResultado
					var cerr error
					if recorte == "" {
						resultado, cerr = CalcularRealizadoComPeriodo(db, empresaID, vv.VinculoID, vv.VigenciaID, fluxo, nivel, "", "")
					} else {
						var di, df string
						di, df, cerr = calcularRecorteDatas(recorte)
						if cerr == nil {
							resultado, cerr = CalcularRealizadoComPeriodo(db, empresaID, vv.VinculoID, vv.VigenciaID, fluxo, nivel, di, df)
						}
					}
					if cerr != nil {
						// Vínculo sem Cliente Válido/Item Válido importado ainda —
						// não é erro operacional, é cadastro incompleto (Épico 3).
						falhas++
						continue
					}
					motivo := "snapshot_diario"
					if err := salvarSnapshot(db, empresaID, vv.VinculoID, vv.VigenciaID, fluxo, nivel, recorte, resultado, motivo); err != nil {
						log.Printf("[farol:objetivos] prewarm: falha ao gravar snapshot vinculo=%d vigencia=%d fluxo=%s nivel=%s recorte=%q: %v",
							vv.VinculoID, vv.VigenciaID, fluxo, nivel, recorte, err)
						falhas++
						continue
					}
					n++
				}
			}
		}
	}
	log.Printf("[farol:objetivos] prewarm: %d snapshot(s) gravado(s) (%d combinação(ões) sem cadastro/erro) em %v",
		n, falhas, time.Since(t0))
}
