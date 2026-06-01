package handlers

import (
	"database/sql"
	"io"
	"log"
	"net/http"
)

const maxLogoSize = 5 << 20 // 5 MB

// ServeEmpresaLogoHandler serve o logotipo da empresa como imagem binária.
// GET /api/config/empresa/logo
func ServeEmpresaLogoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		userID := GetUserIDFromContext(r)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		companyID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
		if err != nil || companyID == "" {
			http.Error(w, "Empresa não encontrada", http.StatusNotFound)
			return
		}

		var logoData []byte
		var logoMime string
		err = db.QueryRow(`
			SELECT logo_data, logo_mime
			FROM companies
			WHERE id = $1::uuid AND logo_data IS NOT NULL
		`, companyID).Scan(&logoData, &logoMime)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			http.Error(w, "Erro no banco de dados", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", logoMime)
		w.Header().Set("Cache-Control", "public, max-age=86400")
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

		userID := GetUserIDFromContext(r)
		if userID == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		companyID, err := GetEffectiveCompanyID(db, userID, r.Header.Get("X-Company-ID"))
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
