package handlers

import (
	"database/sql"
	"io"
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

const maxLogoSize = 5 << 20 // 5 MB

// logoCompanyID resolve qual empresa usar.
// Admin com X-Company-ID pode acessar qualquer empresa diretamente.
func logoCompanyID(db *sql.DB, r *http.Request) (string, error) {
	userID := GetUserIDFromContext(r)
	if userID == "" {
		return "", nil
	}
	requested := r.Header.Get("X-Company-ID")
	// Admin pode acessar empresa específica sem verificação de vínculo
	claims, _ := r.Context().Value(ClaimsKey).(jwt.MapClaims)
	if role, _ := claims["role"].(string); role == "admin" && requested != "" {
		return requested, nil
	}
	return GetEffectiveCompanyID(db, userID, requested)
}

// ServeEmpresaLogoHandler serve o logotipo da empresa como imagem binária.
// GET /api/config/empresa/logo
func ServeEmpresaLogoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		companyID, err := logoCompanyID(db, r)
		if err != nil || companyID == "" {
			http.NotFound(w, r)
			return
		}

		var logoData []byte
		var logoMime string
		err = db.QueryRow(`
			SELECT logo_data, logo_mime
			FROM companies
			WHERE id = $1::uuid AND logo_data IS NOT NULL
		`, companyID).Scan(&logoData, &logoMime)

		// Fallback de grupo: a empresa do usuário pode não ter logo próprio
		// (ex: membro cuja sessão resolve p/ outra empresa do mesmo grupo onde
		// o admin subiu o logo). Um grupo normalmente compartilha uma marca,
		// então buscamos o logo mais recente de uma empresa-irmã do mesmo grupo.
		if err == sql.ErrNoRows {
			err = db.QueryRow(`
				SELECT s.logo_data, s.logo_mime
				FROM companies c
				JOIN companies s ON s.group_id = c.group_id
				WHERE c.id = $1::uuid
				  AND c.group_id IS NOT NULL
				  AND s.logo_data IS NOT NULL
				ORDER BY s.updated_at DESC NULLS LAST, s.created_at DESC
				LIMIT 1
			`, companyID).Scan(&logoData, &logoMime)
		}

		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("ServeEmpresaLogo: companyID=%s erro DB: %v", companyID, err)
			http.Error(w, "Erro no banco de dados", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", logoMime)
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(logoData)
	}
}

// UploadEmpresaLogoHandler processa o upload do logotipo (apenas admin).
// POST /api/config/empresa/logo
func UploadEmpresaLogoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		companyID, err := logoCompanyID(db, r)
		if err != nil || companyID == "" {
			http.Error(w, "Empresa não encontrada", http.StatusNotFound)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxLogoSize)
		if err := r.ParseMultipartForm(maxLogoSize); err != nil {
			http.Error(w, "Arquivo muito grande (máx 5 MB)", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("logo")
		if err != nil {
			http.Error(w, "Campo 'logo' não encontrado no formulário", http.StatusBadRequest)
			return
		}
		defer file.Close()

		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "Erro ao ler arquivo", http.StatusInternalServerError)
			return
		}

		mime := header.Header.Get("Content-Type")
		if mime == "" {
			mime = "image/jpeg"
		}

		_, err = db.Exec(`
			UPDATE companies SET logo_data=$1, logo_mime=$2, logo_nome=$3 WHERE id=$4::uuid
		`, data, mime, header.Filename, companyID)
		if err != nil {
			log.Printf("UploadEmpresaLogo: erro ao salvar companyID=%s: %v", companyID, err)
			http.Error(w, "Erro ao salvar logo", http.StatusInternalServerError)
			return
		}

		log.Printf("UploadEmpresaLogo: logo atualizado companyID=%s mime=%s size=%dB", companyID, mime, len(data))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"OK"}`))
	}
}
