package handlers

// sp_admin.go — Operações administrativas Farol
//
// DELETE /api/sp/admin/limpar-calibragem   (admin_fbtax)
//   Apaga TODOS os dados de calibragem da empresa ativa.
//   Preserva: filiais, CDs, parâmetros do motor, plano, usuários.
//
// POST /api/sp/admin/purgar-csv-antigos   (gestor_geral)
//   Remove importações CSV (sp_csv_jobs + sp_enderecos cascata) mais antigas que
//   retencao_csv_meses meses, conforme configurado nos parâmetros do motor do CD.
//   sp_propostas e sp_historico são preservados.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// SpLimparCalibragemHandler apaga todos os dados de calibragem e importação da empresa ativa.
// DELETE /api/sp/admin/limpar-calibragem
func SpLimparCalibragemHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.IsAdminFbtax() {
			http.Error(w, "Forbidden: apenas admin_fbtax pode limpar dados", http.StatusForbidden)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Erro ao iniciar transação", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// Coleta os file_paths dos jobs para remoção do disco
		rows, err := tx.Query(`
			SELECT file_path FROM farol.sp_csv_jobs
			WHERE empresa_id = $1 AND file_path IS NOT NULL
		`, spCtx.EmpresaID)
		var arquivos []string
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var p string
				if rows.Scan(&p) == nil {
					arquivos = append(arquivos, p)
				}
			}
		}

		// Limpa tabelas na ordem correta (respeitando FK)
		// sp_enderecos e sp_propostas não têm empresa_id diretamente; filtram via job_id
		tabelas := []struct {
			nome string
			sql  string
		}{
			{"sp_historico", `DELETE FROM farol.sp_historico WHERE empresa_id = $1`},
			{"sp_propostas", `DELETE FROM farol.sp_propostas WHERE empresa_id = $1`},
			{"sp_enderecos", `
				DELETE FROM farol.sp_enderecos
				WHERE job_id IN (
					SELECT id FROM farol.sp_csv_jobs WHERE empresa_id = $1
				)`},
			{"sp_csv_jobs", `DELETE FROM farol.sp_csv_jobs WHERE empresa_id = $1`},
		}

		totais := map[string]int64{}
		for _, t := range tabelas {
			res, execErr := tx.Exec(t.sql, spCtx.EmpresaID)
			if execErr != nil {
				log.Printf("SpLimparCalibragem: erro em %s: %v", t.nome, execErr)
				http.Error(w, "Erro ao limpar "+t.nome+": "+execErr.Error(), http.StatusInternalServerError)
				return
			}
			n, _ := res.RowsAffected()
			totais[t.nome] = n
		}

		// H3 fix: audit log dentro da tx — se falhar, rollback cancela a limpeza.
		// "arquivos_a_remover" é a meta; remoção real vem após o commit (best-effort).
		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "calibragem", "all", "limpar_dados", map[string]any{
			"sp_csv_jobs": totais["sp_csv_jobs"], "sp_enderecos": totais["sp_enderecos"],
			"sp_propostas": totais["sp_propostas"], "sp_historico": totais["sp_historico"],
			"arquivos_a_remover": len(arquivos),
		}); err != nil {
			log.Printf("SpLimparCalibragem: falha ao gravar auditoria — abortando: %v", err)
			http.Error(w, "Erro ao gravar auditoria: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Erro ao confirmar limpeza", http.StatusInternalServerError)
			return
		}

		// Remove arquivos CSV do disco APÓS o commit (best-effort).
		// Se falhar, DB já está consistente e uma purga posterior pode limpar.
		removidos := 0
		for _, p := range arquivos {
			if p == "" {
				continue
			}
			clean := filepath.Clean(p)
			if err := os.Remove(clean); err == nil {
				removidos++
			}
		}

		log.Printf("SpLimparCalibragem: empresa=%s limpeza OK — jobs=%d enderecos=%d propostas=%d historico=%d arquivos=%d (by %s)",
			spCtx.EmpresaID,
			totais["sp_csv_jobs"], totais["sp_enderecos"],
			totais["sp_propostas"], totais["sp_historico"],
			removidos, spCtx.UserID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"message":            "Dados de calibragem removidos com sucesso",
			"sp_csv_jobs":        totais["sp_csv_jobs"],
			"sp_enderecos":       totais["sp_enderecos"],
			"sp_propostas":       totais["sp_propostas"],
			"sp_historico":       totais["sp_historico"],
			"arquivos_removidos": removidos,
		})
	}
}

// ─── Purga de importações antigas ────────────────────────────────────────────

// SpPurgarCsvAntigosHandler remove jobs CSV (e endereços via cascade) mais antigos
// que retencao_csv_meses meses, conforme o parâmetro de cada CD.
// sp_propostas e sp_historico são preservados (auditoria permanente).
// POST /api/sp/admin/purgar-csv-antigos
// Body (opcional): { "cd_id": 123 }  — sem cd_id purga todos os CDs da empresa
func SpPurgarCsvAntigosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil || !spCtx.CanApprove() {
			http.Error(w, "Forbidden: gestor_geral+ necessário", http.StatusForbidden)
			return
		}

		var body struct {
			CdID *int `json:"cd_id"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		// Identifica CDs e seus respectivos limites de retenção
		cdFilter := ""
		args := []any{spCtx.EmpresaID}
		if body.CdID != nil {
			cdFilter = " AND cd.id = $2"
			args = append(args, *body.CdID)
		}

		rows, err := db.Query(fmt.Sprintf(`
			SELECT cd.id, COALESCE(mp.retencao_csv_meses, 6)
			FROM farol.sp_centros_dist cd
			LEFT JOIN farol.sp_motor_params mp ON mp.cd_id = cd.id
			WHERE cd.empresa_id = $1%s AND cd.ativo = true
		`, cdFilter), args...)
		if err != nil {
			http.Error(w, "Erro ao carregar CDs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type cdRetencao struct {
			cdID   int
			meses  int
		}
		var cds []cdRetencao
		for rows.Next() {
			var c cdRetencao
			if rows.Scan(&c.cdID, &c.meses) == nil {
				cds = append(cds, c)
			}
		}
		rows.Close()

		totalJobs := int64(0)
		totalArquivos := 0

		for _, c := range cds {
			// Coleta file_paths dos jobs a remover
			fpRows, err := db.Query(`
				SELECT file_path FROM farol.sp_csv_jobs
				WHERE empresa_id = $1 AND cd_id = $2
				  AND status = 'done'
				  AND created_at < now() - ($3 || ' months')::interval
				  AND file_path IS NOT NULL
			`, spCtx.EmpresaID, c.cdID, c.meses)
			if err == nil {
				for fpRows.Next() {
					var fp string
					if fpRows.Scan(&fp) == nil && fp != "" {
						clean := filepath.Clean(fp)
						if os.Remove(clean) == nil {
							totalArquivos++
						}
					}
				}
				fpRows.Close()
			}

			// Remove os jobs (sp_enderecos cascata via FK ON DELETE CASCADE)
			res, err := db.Exec(`
				DELETE FROM farol.sp_csv_jobs
				WHERE empresa_id = $1 AND cd_id = $2
				  AND status = 'done'
				  AND created_at < now() - ($3 || ' months')::interval
			`, spCtx.EmpresaID, c.cdID, c.meses)
			if err == nil {
				n, _ := res.RowsAffected()
				totalJobs += n
			}
		}

		log.Printf("SpPurgarCsvAntigos: empresa=%s jobs_removidos=%d arquivos_removidos=%d (by %s)",
			spCtx.EmpresaID, totalJobs, totalArquivos, spCtx.UserID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"message":            "Purga concluída",
			"jobs_removidos":     totalJobs,
			"arquivos_removidos": totalArquivos,
		})
	}
}
