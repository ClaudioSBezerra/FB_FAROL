package services

import (
	"database/sql"
	"log"
	"time"
)

// StartResumoWorker dispara o gerador automático de resumos executivos.
//
// A cada 1h verifica todos os CDs ativos. Se um CD:
//   - tem destinatários ativos
//   - não tem relatório criado nos últimos 6 dias
//   - é segunda-feira entre 7h e 9h (BRT)
//
// gera o resumo da última semana e envia por email.
func StartResumoWorker(getDB func() *sql.DB) {
	go func() {
		log.Printf("[ResumoWorker] started")
		// Pequeno delay inicial para não disputar com migrations
		time.Sleep(60 * time.Second)
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		runOnce := func() {
			db := getDB()
			if db == nil {
				return
			}
			processarResumosSemanais(db)
		}

		// Roda imediatamente uma vez
		runOnce()
		for range ticker.C {
			runOnce()
		}
	}()
}

func processarResumosSemanais(db *sql.DB) {
	loc := time.FixedZone("BRT", -3*3600)
	now := time.Now().In(loc)

	// Janela de execução: segunda-feira (Weekday 1) entre 07h e 09h BRT
	if now.Weekday() != time.Monday || now.Hour() < 7 || now.Hour() >= 9 {
		return
	}

	rows, err := db.Query(`
		SELECT c.id
		  FROM farol.sp_centros_dist c
		 WHERE c.ativo = TRUE
		   AND EXISTS (
		     SELECT 1 FROM farol.sp_destinatarios_resumo d
		      WHERE d.cd_id = c.id AND d.ativo = TRUE
		   )
		   AND NOT EXISTS (
		     SELECT 1 FROM farol.sp_relatorios_semanais r
		      WHERE r.cd_id = c.id
		        AND r.criado_em > NOW() - INTERVAL '6 days'
		   )
	`)
	if err != nil {
		log.Printf("[ResumoWorker] erro listando CDs: %v", err)
		return
	}
	defer rows.Close()

	cdIDs := []int{}
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			cdIDs = append(cdIDs, id)
		}
	}

	if len(cdIDs) == 0 {
		return
	}
	log.Printf("[ResumoWorker] gerando resumo para %d CDs", len(cdIDs))

	for _, cdID := range cdIDs {
		gerarEEnviar(db, cdID)
		// pequeno espaçamento entre CDs para não sobrecarregar a Z.AI
		time.Sleep(3 * time.Second)
	}
}

func gerarEEnviar(db *sql.DB, cdID int) {
	id, _, _, err := GerarResumoExecutivo(db, cdID, "worker")
	if err != nil {
		log.Printf("[ResumoWorker] CD %d falhou ao gerar: %v", cdID, err)
		return
	}
	log.Printf("[ResumoWorker] CD %d resumo %d gerado, enviando emails…", cdID, id)

	enviados, err := EnviarResumoPorEmail(db, id)
	erroMsg := ""
	if err != nil {
		erroMsg = err.Error()
		log.Printf("[ResumoWorker] CD %d envio falhou: %v", cdID, err)
	}
	if len(enviados) > 0 {
		_ = MarcarEnviado(db, id, enviados, erroMsg)
		log.Printf("[ResumoWorker] CD %d resumo %d enviado para %d destinatários", cdID, id, len(enviados))
	}
}
