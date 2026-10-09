package handlers

// farol_metas_reprocessar.go — invalidação e reprocessamento do Realizado de
// uma vigência inteira.
//
// Dois usos:
//  1. Importação de lista (Clientes, Itens, Clientes Numérica, PPAs): o
//     resultado salvo em metas_realizados_snapshot foi calculado com a lista
//     ANTIGA. Sem invalidar, o painel só mudava no prewarm da madrugada
//     seguinte (achado 09/10/2026, ao escrever o manual do administrador).
//     Aqui o snapshot é apagado na hora — a próxima leitura já se auto-cura
//     (obterOuCalcularSnapshot) — e o recálculo é reaquecido em background.
//  2. Botão "Reprocessar" do administrador (POST
//     /api/farol/metas-vigencias/{id}/reprocessar), inclusive em vigência
//     FECHADA (FR17: reprocessar mês fechado é ação explícita de gestor,
//     auditada).

import (
	"database/sql"
	"log"
	"sync"
	"time"
)

// invalidarSnapshotsVigencia apaga todos os snapshots (todo fluxo/nível/recorte)
// da vigência. Síncrono e barato: o que sobra é recalculado sob demanda.
func invalidarSnapshotsVigencia(db *sql.DB, empresaID string, vigenciaID int) error {
	_, err := db.Exec(`DELETE FROM farol.metas_realizados_snapshot WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
	return err
}

var (
	reaquecimentoMu     sync.Mutex
	reaquecimentoEstado = map[int]int{} // vigenciaID → 1 rodando, 2 rodando + novo pedido pendente
)

// agendarReaquecimentoVigencia recalcula, em background, os snapshots base
// (recorte "") da vigência — o que o prewarm faria de madrugada. Pedidos
// repetidos enquanto um recálculo roda viram UMA rodada extra (importar 4
// listas em sequência não empilha 4 recálculos pesados da Numérica).
func agendarReaquecimentoVigencia(db *sql.DB, empresaID string, vinculoID, vigenciaID int, incluiItens bool) {
	reaquecimentoMu.Lock()
	if reaquecimentoEstado[vigenciaID] != 0 {
		reaquecimentoEstado[vigenciaID] = 2
		reaquecimentoMu.Unlock()
		return
	}
	reaquecimentoEstado[vigenciaID] = 1
	reaquecimentoMu.Unlock()

	go func() {
		for {
			reprocessarVigencia(db, empresaID, vinculoID, vigenciaID, incluiItens)
			reaquecimentoMu.Lock()
			if reaquecimentoEstado[vigenciaID] == 2 {
				reaquecimentoEstado[vigenciaID] = 1
				reaquecimentoMu.Unlock()
				continue
			}
			delete(reaquecimentoEstado, vigenciaID)
			reaquecimentoMu.Unlock()
			return
		}
	}()
}

func reprocessarVigencia(db *sql.DB, empresaID string, vinculoID, vigenciaID int, incluiItens bool) {
	t0 := time.Now()
	var formula string
	if err := db.QueryRow(`
		SELECT tm.formula_codigo FROM farol.metas_vinculos mv
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE mv.id = $1 AND mv.empresa_id = $2`, vinculoID, empresaID).Scan(&formula); err != nil {
		log.Printf("[farol:objetivos] reprocessar vigencia=%d: vínculo %d não encontrado: %v", vigenciaID, vinculoID, err)
		return
	}
	fluxos := fluxosPrewarmObjetivos
	if formula == "cobertura_numerica" || formula == "sortimento_numerica_ppa" {
		fluxos = fluxosPrewarmNumerica
	}
	if incluiItens {
		for _, fluxo := range fluxos {
			if err := RecalcularItensRealizado(db, empresaID, vinculoID, vigenciaID, fluxo); err != nil {
				log.Printf("[farol:objetivos] reprocessar vigencia=%d: itens fluxo=%s: %v", vigenciaID, fluxo, err)
			}
		}
	}
	n, falhas := 0, 0
	for _, fluxo := range fluxos {
		for _, nivel := range niveisPrewarmObjetivos {
			resultado, err := CalcularRealizadoComPeriodo(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, "", "")
			if err != nil {
				// Lista ainda incompleta (ex.: Clientes importado, Itens não) —
				// não é erro operacional, a leitura se auto-cura quando completar.
				falhas++
				continue
			}
			if err := salvarSnapshot(db, empresaID, vinculoID, vigenciaID, fluxo, nivel, "", resultado, "reprocessamento_manual"); err != nil {
				log.Printf("[farol:objetivos] reprocessar vigencia=%d: gravar snapshot fluxo=%s nivel=%s: %v", vigenciaID, fluxo, nivel, err)
				falhas++
				continue
			}
			n++
		}
	}
	log.Printf("[farol:objetivos] reprocessar vigencia=%d vinculo=%d: %d snapshot(s) regravado(s), %d sem cálculo, em %v", vigenciaID, vinculoID, n, falhas, time.Since(t0))
}

// aposImportarLista é o gancho dos 4 importadores de lista: apaga o resultado
// salvo e reaquece em background. Erro só loga — a importação já deu certo.
func aposImportarLista(db *sql.DB, empresaID string, vinculoID, vigenciaID int) {
	if err := invalidarSnapshotsVigencia(db, empresaID, vigenciaID); err != nil {
		log.Printf("[farol:objetivos] aposImportarLista vigencia=%d: falha ao invalidar snapshots: %v", vigenciaID, err)
		return
	}
	agendarReaquecimentoVigencia(db, empresaID, vinculoID, vigenciaID, false)
}
