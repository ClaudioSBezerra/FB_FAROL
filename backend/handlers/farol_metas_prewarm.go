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
	fluxosPrewarmObjetivos = []string{"faturado", "transmitido", "soma"}
	// fluxosPrewarmNumerica — Story 7.8 (30/09/2026): a Numérica ganhou de
	// volta a 3ª visão "Faturado + Emitido" (fluxo "soma"), só pra ela (ver
	// farol_metas_calculo_numerica.go). Sem isso no prewarm, essa visão
	// ficaria sempre na auto-cura (1º acesso do dia calcula ao vivo,
	// TODOS os clientes do vínculo) em vez de já vir pronta do snapshot —
	// mesmo risco de lentidão que a troca pra snapshot diário resolveu em
	// 2026-09-11 (ver farol_metas_congelamento.go) pros outros 2 fluxos.
	fluxosPrewarmNumerica    = []string{"faturado", "transmitido", "soma"}
	niveisPrewarmObjetivos   = []string{"rede", "ggv", "crv", "rca"}
	recortesPrewarmObjetivos = []string{"", "dia_anterior", "semana", "mes", "ano_corrente"}
)

type vinculoVigenciaAberta struct {
	VinculoID     int
	VigenciaID    int
	FormulaCodigo string
}

// listarVinculosAtivosComVigenciaAberta — só vínculos ATIVOS (cadastro em
// uso) com vigência ABERTA (mês corrente) valem prewarm; vigência fechada
// já congela sozinha no primeiro acesso (migration 223) e não muda mais.
// FormulaCodigo vem junto pra decidir a lista de fluxos a prewarmar (ver
// fluxosPrewarmNumerica acima) sem precisar de uma 2ª query por vínculo.
func listarVinculosAtivosComVigenciaAberta(db *sql.DB, empresaID string) ([]vinculoVigenciaAberta, error) {
	rows, err := db.Query(`
		SELECT v.vinculo_id, v.id, tm.formula_codigo
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE v.empresa_id = $1 AND v.status = 'aberta' AND mv.ativo = true
	`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vinculoVigenciaAberta
	for rows.Next() {
		var vv vinculoVigenciaAberta
		if err := rows.Scan(&vv.VinculoID, &vv.VigenciaID, &vv.FormulaCodigo); err != nil {
			return nil, err
		}
		out = append(out, vv)
	}
	return out, rows.Err()
}

// PrewarmMetasRealizados varre os vínculos ativos com vigência aberta da
// empresa e grava (recorte "" + os 4 recortes, cada fluxo × nível) o
// snapshot no banco. Erros de UM vínculo/combinação só logam e seguem pras
// próximas — uma Indústria com Cliente Válido faltando não pode travar o
// prewarm das outras.
func PrewarmMetasRealizados(db *sql.DB, empresaID string) {
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
		fluxos := fluxosPrewarmObjetivos
		if vv.FormulaCodigo == "cobertura_numerica" || vv.FormulaCodigo == "sortimento_numerica_ppa" {
			fluxos = fluxosPrewarmNumerica
		}
		// Itens Realizados (migration 236) — 1x por vínculo×fluxo, fora do
		// loop de nível/recorte abaixo (não varia com isso, é sempre a
		// vigência inteira). No-op silencioso pra vínculo de Cobertura ou
		// Numérica (RecalcularItensRealizado só existe pra sortimento_rede
		// hoje — drill-down de itens da Numérica/PPA ainda não foi
		// construído, gap conhecido e separado desta story).
		for _, fluxo := range fluxos {
			if err := RecalcularItensRealizado(db, empresaID, vv.VinculoID, vv.VigenciaID, fluxo); err != nil {
				log.Printf("[farol:objetivos] prewarm: falha ao recalcular Itens Realizado vinculo=%d vigencia=%d fluxo=%s: %v",
					vv.VinculoID, vv.VigenciaID, fluxo, err)
			}
		}
		for _, fluxo := range fluxos {
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
