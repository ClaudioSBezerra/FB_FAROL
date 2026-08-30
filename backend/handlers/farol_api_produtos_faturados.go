package handlers

// farol_api_produtos_faturados.go — Endpoint machine-to-machine consumido pelo
// SmartPick (Monitor de Faturamento sem Calibragem). Não é uma sessão de
// usuário: autenticação por API key estática, vinculada a UMA empresa fixa do
// Farol via env vars — escolha deliberada para não vazar dado entre
// clientes/tenants que podem compartilhar códigos de filial WinThor (ex: "11").
//
// GET /api/farol/produtos-faturados?empresa={cod_filial}&data_ini=YYYY-MM-DD&data_fim=YYYY-MM-DD
// Header: X-API-Key: {FAROL_API_KEY}

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type produtoFaturadoAPI struct {
	CodProd         string  `json:"cod_prod"`
	Empresa         string  `json:"empresa"`
	DataFaturamento string  `json:"data_faturamento"`
	Qt              float64 `json:"qt"`
}

// FarolAPIKeyAuth valida o header X-API-Key contra FAROL_API_KEY (comparação em
// tempo constante) e resolve a empresa_id fixa vinculada à key
// (FAROL_API_KEY_EMPRESA_ID). Sem as duas env vars configuradas, a integração
// fica desabilitada por padrão (503) — nunca aceita chamada não configurada.
func FarolAPIKeyAuth(next func(db *sql.DB, empresaID string) http.HandlerFunc) func(db *sql.DB) http.HandlerFunc {
	return func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			expectedKey := os.Getenv("FAROL_API_KEY")
			empresaID := os.Getenv("FAROL_API_KEY_EMPRESA_ID")
			if expectedKey == "" || empresaID == "" {
				http.Error(w, `{"error":"Integração não configurada"}`, http.StatusServiceUnavailable)
				return
			}

			got := r.Header.Get("X-API-Key")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expectedKey)) != 1 {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			next(db, empresaID)(w, r)
		}
	}
}

// ProdutosFaturadosAPIHandler retorna linhas de vendas_faturadas da empresa
// vinculada à API key, filtradas por filial (cod_filial, campo "empresa" da
// tabela) e janela de datas — consumido pelo SmartPick para cruzar com
// calibragem aprovada.
func ProdutosFaturadosAPIHandler(db *sql.DB, empresaID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		empresaFilial := r.URL.Query().Get("empresa")
		dataIniStr := r.URL.Query().Get("data_ini")
		dataFimStr := r.URL.Query().Get("data_fim")
		if empresaFilial == "" || dataIniStr == "" || dataFimStr == "" {
			http.Error(w, `{"error":"parâmetros obrigatórios: empresa, data_ini, data_fim"}`, http.StatusBadRequest)
			return
		}
		dataIni, err := time.Parse("2006-01-02", dataIniStr)
		if err != nil {
			http.Error(w, `{"error":"data_ini inválida (use YYYY-MM-DD)"}`, http.StatusBadRequest)
			return
		}
		dataFim, err := time.Parse("2006-01-02", dataFimStr)
		if err != nil {
			http.Error(w, `{"error":"data_fim inválida (use YYYY-MM-DD)"}`, http.StatusBadRequest)
			return
		}

		rows, err := db.Query(`
			SELECT cod_prod, empresa, data_faturamento, qt
			  FROM vendas_faturadas
			 WHERE empresa_id = $1 AND empresa = $2
			   AND data_faturamento >= $3 AND data_faturamento <= $4
			   AND cod_prod <> ''
		`, empresaID, empresaFilial, dataIni, dataFim)
		if err != nil {
			log.Printf("[farol-api] erro consultando vendas_faturadas: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		produtos := []produtoFaturadoAPI{}
		for rows.Next() {
			var p produtoFaturadoAPI
			var data time.Time
			if err := rows.Scan(&p.CodProd, &p.Empresa, &data, &p.Qt); err != nil {
				log.Printf("[farol-api] erro no scan de vendas_faturadas: %v", err)
				http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
				return
			}
			p.DataFaturamento = data.Format("2006-01-02")
			produtos = append(produtos, p)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[farol-api] erro iterando vendas_faturadas: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(produtos)
	}
}
