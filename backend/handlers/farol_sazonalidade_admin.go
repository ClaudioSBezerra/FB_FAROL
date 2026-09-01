package handlers

// farol_sazonalidade_admin.go — geração manual de sazonalidade por produto,
// sob demanda, pra tela admin "Configuração → Sazonalidade" (gestor_geral+).
//
// A importação diária roda automática de madrugada e já dispara a
// consolidação (incluindo sazonalidade) — este endpoint existe pro caso de
// precisar forçar/reprocessar sem esperar o próximo ciclo (ex.: dado
// corrigido retroativamente numa filial, ou só pra conferir o resultado sem
// esperar o import). Nunca é chamado pelo pipeline automático.
//
// POST /api/v2/farol/sazonalidade/gerar
// Header: Authorization: Bearer <token> (sessão gestor_geral+)
// Body: {"cod_filial": 11, "ano": 2025}  — os dois campos são opcionais:
//   cod_filial ausente → processa TODAS as filiais da empresa
//   ano ausente        → processa TODOS os anos com dado na empresa/filial

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

func GerarSazonalidadeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		// Checagem redundante à do withSP na rota (defesa em profundidade —
		// ação escreve em massa em tabelas agregadas, vale a garantia extra).
		if !hasSpRole(spCtx.SpRole, "gestor_geral") {
			http.Error(w, `{"error":"Forbidden: gestor_geral necessário"}`, http.StatusForbidden)
			return
		}

		var body struct {
			CodFilial *int `json:"cod_filial"`
			Ano       *int `json:"ano"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body) // corpo vazio = {} = ambos nil, válido
		}

		var filial *string
		if body.CodFilial != nil {
			s := strconv.Itoa(*body.CodFilial)
			filial = &s
		}

		anos, err := resolverAnosParaGerar(db, spCtx.EmpresaID, filial, body.Ano)
		if err != nil {
			log.Printf("[sazonalidade-admin] erro resolvendo anos (empresa=%s filial=%v): %v", spCtx.EmpresaID, filial, err)
			http.Error(w, `{"error":"erro consultando anos disponíveis"}`, http.StatusInternalServerError)
			return
		}
		if len(anos) == 0 {
			http.Error(w, `{"error":"nenhum dado de vendas encontrado pro filtro informado"}`, http.StatusNotFound)
			return
		}

		t0 := time.Now()
		erros := 0
		for _, ano := range anos {
			for mes := 1; mes <= 12; mes++ {
				if _, e := db.Exec(`SELECT farol.upsert_aggs_mes_v12($1,$2,$3,$4)`, spCtx.EmpresaID, ano, mes, filial); e != nil {
					log.Printf("[sazonalidade-admin] upsert_aggs_mes_v12 %d-%02d (filial=%v) ERRO: %v", ano, mes, filial, e)
					erros++
				}
			}
			if _, e := db.Exec(`SELECT farol.upsert_sazonalidade_produto_ano($1,$2,$3)`, spCtx.EmpresaID, ano, filial); e != nil {
				log.Printf("[sazonalidade-admin] upsert_sazonalidade_produto_ano %d (filial=%v) ERRO: %v", ano, filial, e)
				erros++
			}
		}

		log.Printf("[sazonalidade-admin] gerado por %s: filial=%v anos=%v erros=%d em %v",
			spCtx.UserID, filial, anos, erros, time.Since(t0))

		json.NewEncoder(w).Encode(map[string]any{
			"ok":               erros == 0,
			"anos_processados": anos,
			"erros":            erros,
			"duration_ms":      time.Since(t0).Milliseconds(),
		})
	}
}

// resolverAnosParaGerar devolve os anos a processar: [*anoPedido] se
// informado, ou todo ano com venda faturada pra empresa/filial (escopo do
// filtro de filial, se houver) quando ausente.
func resolverAnosParaGerar(db *sql.DB, empresaID string, filial *string, anoPedido *int) ([]int, error) {
	if anoPedido != nil {
		return []int{*anoPedido}, nil
	}

	query := `SELECT DISTINCT EXTRACT(YEAR FROM data_faturamento)::int
	            FROM vendas_faturadas
	           WHERE empresa_id = $1`
	args := []any{empresaID}
	if filial != nil {
		query += " AND empresa = $2"
		args = append(args, *filial)
	}
	query += " ORDER BY 1"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var anos []int
	for rows.Next() {
		var a int
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		anos = append(anos, a)
	}
	return anos, rows.Err()
}
