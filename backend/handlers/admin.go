package handlers

// admin.go — Gestão de usuários (Farol)
// Handlers de ambiente/grupo/empresa estão em environment.go (preservados integralmente).
// Handlers APU02 (LimparApuracao, ResetDB, RefreshViews) removidos na Story 1.3.

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// CreateUserRequest struct
type CreateUserRequest struct {
	FullName      string `json:"full_name"`
	Email         string `json:"email"`
	Password      string `json:"password"`
	Role          string `json:"role"`
	EnvironmentID string `json:"environment_id"`
	GroupID       string `json:"group_id"`
	CompanyID     string `json:"company_id"`
}

// CreateUserHandler creates a new user directly (Admin only)
func CreateUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.Email == "" || req.Password == "" || req.FullName == "" {
			http.Error(w, "Missing required fields", http.StatusBadRequest)
			return
		}

		hash, err := HashPassword(req.Password)
		if err != nil {
			http.Error(w, "Error hashing password", http.StatusInternalServerError)
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}

		trialEnds := time.Now().Add(time.Hour * 24 * 14)
		var userID string
		err = db.QueryRow(`
			INSERT INTO users (email, password_hash, full_name, trial_ends_at, is_verified, role)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, req.Email, hash, req.FullName, trialEnds, true, req.Role).Scan(&userID)
		if err != nil {
			log.Printf("Error creating user: %v", err)
			http.Error(w, "Error creating user (email might be taken)", http.StatusConflict)
			return
		}

		if req.EnvironmentID != "" {
			_, _ = db.Exec("INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'user')", userID, req.EnvironmentID)
			if req.CompanyID != "" {
				_, _ = db.Exec("UPDATE companies SET owner_id = $1 WHERE id = $2", userID, req.CompanyID)
			}
		} else {
			var envID string
			if err = db.QueryRow("INSERT INTO environments (name, description) VALUES ($1, 'Ambiente Padrão') RETURNING id", "Ambiente de "+req.FullName).Scan(&envID); err == nil {
				var groupID string
				_ = db.QueryRow("INSERT INTO enterprise_groups (environment_id, name, description) VALUES ($1, 'Grupo Padrão', 'Grupo Inicial') RETURNING id", envID).Scan(&groupID)
				_, _ = db.Exec("INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'admin')", userID, envID)
				if groupID != "" {
					_, _ = db.Exec("INSERT INTO companies (group_id, name, trade_name, owner_id) VALUES ($1, $2, $2, $3)", groupID, "Empresa de "+req.FullName, userID)
				}
			}
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"message": "User created successfully", "id": userID})
	}
}

// AdminUser extends User with hierarchy info for admin listing
type AdminUser struct {
	ID              string    `json:"id"`
	Email           string    `json:"email"`
	FullName        string    `json:"full_name"`
	IsVerified      bool      `json:"is_verified"`
	TrialEndsAt     time.Time `json:"trial_ends_at"`
	Role            string    `json:"role"`
	CreatedAt       string    `json:"created_at"`
	EnvironmentID   *string   `json:"environment_id"`
	EnvironmentName *string   `json:"environment_name"`
	GroupID         *string   `json:"group_id"`
	GroupName       *string   `json:"group_name"`
	CompanyID       *string   `json:"company_id"`
	CompanyName     *string   `json:"company_name"`
}

// ListUsersHandler returns all users with hierarchy info (Admin only)
func ListUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query(`
			SELECT DISTINCT ON (u.id)
			       u.id, u.email, u.full_name, u.is_verified, u.trial_ends_at, u.role, u.created_at,
			       e.id, e.name,
			       eg.id, eg.name,
			       c.id, c.name
			FROM users u
			LEFT JOIN user_environments ue ON u.id = ue.user_id
			LEFT JOIN environments e ON ue.environment_id = e.id
			LEFT JOIN enterprise_groups eg ON eg.environment_id = e.id
			LEFT JOIN companies c ON c.group_id = eg.id
			ORDER BY u.id, (c.owner_id = u.id) DESC NULLS LAST, c.created_at ASC NULLS LAST
		`)
		if err != nil {
			log.Printf("ListUsers error: %v", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var users []AdminUser
		for rows.Next() {
			var u AdminUser
			if err := rows.Scan(
				&u.ID, &u.Email, &u.FullName, &u.IsVerified, &u.TrialEndsAt, &u.Role, &u.CreatedAt,
				&u.EnvironmentID, &u.EnvironmentName,
				&u.GroupID, &u.GroupName,
				&u.CompanyID, &u.CompanyName,
			); err != nil {
				log.Printf("ListUsers scan error: %v", err)
				continue
			}
			users = append(users, u)
		}
		if users == nil {
			users = []AdminUser{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	}
}

// PromoteUserRequest struct
type PromoteUserRequest struct {
	Role       string `json:"role"`
	ExtendDays int    `json:"extend_days"`
	IsOfficial bool   `json:"is_official"`
}

// PromoteUserHandler updates user role or trial (Admin only)
func PromoteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.URL.Query().Get("id")
		if userID == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		var req PromoteUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		if req.Role != "" {
			if _, err := db.Exec("UPDATE users SET role = $1 WHERE id = $2", req.Role, userID); err != nil {
				http.Error(w, "Failed to update role", http.StatusInternalServerError)
				return
			}
		}

		if req.IsOfficial {
			newEnd := time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)
			if _, err := db.Exec("UPDATE users SET trial_ends_at = $1 WHERE id = $2", newEnd, userID); err != nil {
				http.Error(w, "Failed to update trial status", http.StatusInternalServerError)
				return
			}
		} else if req.ExtendDays > 0 {
			var currentEnd time.Time
			if err := db.QueryRow("SELECT trial_ends_at FROM users WHERE id = $1", userID).Scan(&currentEnd); err != nil {
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}
			if currentEnd.Before(time.Now()) {
				currentEnd = time.Now()
			}
			newEnd := currentEnd.Add(time.Duration(req.ExtendDays) * 24 * time.Hour)
			if _, err := db.Exec("UPDATE users SET trial_ends_at = $1 WHERE id = $2", newEnd, userID); err != nil {
				http.Error(w, "Failed to update trial", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"})
	}
}

// ReassignUserRequest struct
type ReassignUserRequest struct {
	UserID        string `json:"user_id"`
	EnvironmentID string `json:"environment_id"`
	GroupID       string `json:"group_id"`
	CompanyID     string `json:"company_id"`
}

// ReassignUserHandler re-links a user to a different hierarchy (Admin only)
func ReassignUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ReassignUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if req.UserID == "" || req.EnvironmentID == "" {
			http.Error(w, "user_id and environment_id are required", http.StatusBadRequest)
			return
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		_, _ = tx.Exec("UPDATE companies SET owner_id = NULL WHERE owner_id = $1", req.UserID)
		if _, err = tx.Exec("DELETE FROM user_environments WHERE user_id = $1", req.UserID); err != nil {
			http.Error(w, "Failed to remove old environment link", http.StatusInternalServerError)
			return
		}

		if req.CompanyID != "" {
			_, err = tx.Exec(`
				INSERT INTO user_environments (user_id, environment_id, role, preferred_company_id)
				VALUES ($1, $2, 'user', $3)
			`, req.UserID, req.EnvironmentID, req.CompanyID)
		} else {
			_, err = tx.Exec("INSERT INTO user_environments (user_id, environment_id, role) VALUES ($1, $2, 'user')", req.UserID, req.EnvironmentID)
		}
		if err != nil {
			http.Error(w, "Failed to link to new environment", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "Failed to commit changes", http.StatusInternalServerError)
			return
		}

		log.Printf("ReassignUser: User %s reassigned to environment %s", req.UserID, req.EnvironmentID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "User reassigned successfully"})
	}
}

// DeleteUserHandler deletes a user (Admin only)
func DeleteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetID := r.URL.Query().Get("id")
		if targetID == "" {
			http.Error(w, "User ID required", http.StatusBadRequest)
			return
		}

		claims, ok := r.Context().Value(ClaimsKey).(jwt.MapClaims)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if claims["user_id"].(string) == targetID {
			http.Error(w, "Cannot delete yourself", http.StatusBadRequest)
			return
		}

		if _, err := db.Exec("DELETE FROM users WHERE id = $1", targetID); err != nil {
			http.Error(w, "Failed to delete user", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "User deleted successfully"})
	}
}

// DiagnoseBIHandler retorna contagens de vendas vs agg_*_mes por empresa
// e força um repopulate via farol.upsert_aggs_mes. Útil quando o BI mostra
// 0 cards mas as views legacy (mv_fat_*) têm dados — indica que as agg_*_mes
// não foram populadas.
//
// GET  /api/admin/diagnose-bi          → só diagnostica
// POST /api/admin/diagnose-bi?action=repopulate → diagnostica + repopula
func DiagnoseBIHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		out := map[string]any{}

		// 1. Migrations 16x aplicadas
		migs := []string{}
		rows, _ := db.Query(`SELECT filename FROM schema_migrations WHERE filename LIKE '16%' ORDER BY filename`)
		if rows != nil {
			for rows.Next() {
				var f string
				if rows.Scan(&f) == nil {
					migs = append(migs, f)
				}
			}
			rows.Close()
		}
		out["migrations_16x"] = migs

		// 2. Contagens por empresa
		type empCount struct {
			Empresa string `json:"empresa_id"`
			Rows    int    `json:"rows"`
		}
		countBy := func(q string) []empCount {
			res := []empCount{}
			rs, err := db.Query(q)
			if err != nil {
				log.Printf("[diagnose-bi] erro %q: %v", q, err)
				return res
			}
			defer rs.Close()
			for rs.Next() {
				var e empCount
				if rs.Scan(&e.Empresa, &e.Rows) == nil {
					res = append(res, e)
				}
			}
			return res
		}
		out["vendas_faturadas"] = countBy(`SELECT empresa_id::text, COUNT(*) FROM vendas_faturadas GROUP BY empresa_id`)
		out["vendas_transmitidas"] = countBy(`SELECT empresa_id::text, COUNT(*) FROM vendas_transmitidas GROUP BY empresa_id`)
		out["mv_fat_v01_l0"] = countBy(`SELECT empresa_id::text, COUNT(*) FROM farol.mv_fat_v01_l0 GROUP BY empresa_id`)
		out["agg_fat_v01_l0_mes"] = countBy(`SELECT empresa_id::text, COUNT(*) FROM farol.agg_fat_v01_l0_mes GROUP BY empresa_id`)
		out["mv_fat_carteira_rca"] = countBy(`SELECT empresa_id::text, COUNT(*) FROM farol.mv_fat_carteira_rca GROUP BY empresa_id`)

		// 3. Função upsert_aggs_mes existe?
		var fnExists bool
		_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='farol' AND p.proname='upsert_aggs_mes')`).Scan(&fnExists)
		out["fn_upsert_aggs_mes_exists"] = fnExists

		// 4. Ação repopulate (opcional)
		if r.URL.Query().Get("action") == "repopulate" {
			t0 := time.Now()
			upserts := []map[string]any{}
			rs, err := db.Query(`
				SELECT empresa_id, EXTRACT(YEAR FROM data_faturamento)::int, EXTRACT(MONTH FROM data_faturamento)::int FROM vendas_faturadas
				UNION
				SELECT empresa_id, EXTRACT(YEAR FROM data_transmissao)::int, EXTRACT(MONTH FROM data_transmissao)::int FROM vendas_transmitidas`)
			if err != nil {
				out["repopulate_error"] = err.Error()
			} else {
				defer rs.Close()
				anosSeen := map[int]bool{}
				type ym struct {
					Empresa string
					Ano     int
					Mes     int
				}
				var jobs []ym
				for rs.Next() {
					var j ym
					if rs.Scan(&j.Empresa, &j.Ano, &j.Mes) == nil {
						jobs = append(jobs, j)
					}
				}
				for _, j := range jobs {
					if !anosSeen[j.Ano] {
						if _, e := db.Exec(`SELECT farol.create_agg_year_partitions($1)`, j.Ano); e != nil {
							log.Printf("[diagnose-bi] create_year(%d) erro: %v", j.Ano, e)
						}
						anosSeen[j.Ano] = true
					}
					t1 := time.Now()
					_, e := db.Exec(`SELECT farol.upsert_aggs_mes($1::uuid, $2, $3)`, j.Empresa, j.Ano, j.Mes)
					rec := map[string]any{
						"empresa": j.Empresa, "ano": j.Ano, "mes": j.Mes,
						"duration_ms": time.Since(t1).Milliseconds(),
					}
					if e != nil {
						rec["error"] = e.Error()
					}
					upserts = append(upserts, rec)
				}
				out["repopulate_jobs"] = upserts
				out["repopulate_duration_ms"] = time.Since(t0).Milliseconds()
				out["agg_fat_v01_l0_mes_AFTER"] = countBy(`SELECT empresa_id::text, COUNT(*) FROM farol.agg_fat_v01_l0_mes GROUP BY empresa_id`)
			}
		}

		json.NewEncoder(w).Encode(out)
	}
}
