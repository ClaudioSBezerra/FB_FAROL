package handlers

// farol_v2_cleanup.go — Módulo de limpeza inteligente.
//
// Apaga dados de UM cliente (empresa), com escolha por tabela. Todas as tabelas
// alvo têm empresa_id (UUID) → a limpeza é sempre escopada à empresa do contexto.
//
//   GET  /api/v2/farol/cleanup/inventory  → contagem de linhas por tabela
//   POST /api/v2/farol/cleanup            → apaga as tabelas selecionadas
//
// Ao limpar a tabela de vendas, as views materializadas são reconstruídas para o
// painel refletir imediatamente (senão mostraria dados já apagados).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type cleanupTableSpec struct {
	Key          string `json:"key"`
	Table        string `json:"table"` // físico — allowlist, nunca vem do usuário
	Label        string `json:"label"`
	Description  string `json:"description"`
	RefreshViews bool   `json:"-"`
	// AggFluxo, quando preenchido ("fat"|"trans"), faz a limpeza purgar TODAS as
	// tabelas agregadas daquele fluxo (agg_*_mes + dims + mkt) em vez de um DELETE
	// simples. Table aponta só para a tabela representativa usada na contagem.
	AggFluxo string `json:"-"`
}

// Allowlist de tabelas limpáveis. Só estas podem ser apagadas, e só por empresa_id.
var cleanupTables = []cleanupTableSpec{
	{Key: "vendas_faturadas", Table: "vendas_faturadas", Label: "Vendas faturadas",
		Description: "Base de FATURAMENTO (NF emitida). Limpar exige reimportar.", RefreshViews: true},
	{Key: "vendas_transmitidas", Table: "vendas_transmitidas", Label: "Vendas transmitidas",
		Description: "Base de TRANSMISSÃO (pedido digitado pelo RCA). Limpar exige reimportar.", RefreshViews: true},
	// vendas_ccd (mig 182, novo layout jul/2026) — eventos negativos (Cortado,
	// Cancelado, Devolvido) exportados em arquivos _CCD.csv. Ainda não tem
	// agg_*_mes própria; DELETE direto sem RefreshViews basta.
	{Key: "vendas_ccd", Table: "vendas_ccd", Label: "Vendas CCD (Cort/Canc/Dev)",
		Description: "Base de eventos negativos (Cortado/Cancelado/Devolvido) do novo layout ION VENDAS. Limpar exige reimportar os arquivos _CCD.csv."},
	{Key: "agg_faturado", Table: "farol.agg_fat_v01_l0_mes", Label: "Painel agregado — Faturado",
		Description:  "Tabelas que alimentam o painel (faturado). Limpar zera o painel mesmo com as vendas já apagadas — útil para remover dados fantasma sem reimportar.",
		RefreshViews: true, AggFluxo: "fat"},
	{Key: "agg_transmitido", Table: "farol.agg_trans_v01_l0_mes", Label: "Painel agregado — Transmitido",
		Description:  "Tabelas que alimentam o painel (transmitido). Limpar zera o painel mesmo com as vendas já apagadas.",
		RefreshViews: true, AggFluxo: "trans"},
	{Key: "objetivos", Table: "objetivos_importados", Label: "Objetivos importados",
		Description: "Modelo antigo de objetivos. Não usado pelo painel novo."},
	{Key: "jobs", Table: "vendas_import_jobs", Label: "Histórico de importações",
		Description: "Registros dos jobs de importação. Não afeta os dados de vendas."},
	{Key: "industrias", Table: "industrias_config", Label: "Configuração de indústrias",
		Description: "Programa de distribuição e trava mínima por fornecedor."},
}

func cleanupSpecByKey(key string) *cleanupTableSpec {
	for i := range cleanupTables {
		if cleanupTables[i].Key == key {
			return &cleanupTables[i]
		}
	}
	return nil
}

// aggTablesForFluxo retorna TODAS as tabelas agregadas derivadas de vendas_<fluxo>
// (agg_*_v0X_lY_mes + dims + marketing). Limpar vendas_* sem limpar estas deixa
// o painel mostrando dados fantasma: o painel lê das agg_*_mes, e upsert_aggs_mes
// só faz INSERT/UPSERT — nunca apaga linhas órfãs quando a origem some.
// fluxoPrefix: "fat" ou "trans".
func aggTablesForFluxo(fluxoPrefix string) []string {
	src := aggTablesFat
	if fluxoPrefix == "trans" {
		src = aggTablesTrans
	}
	seen := map[string]bool{}
	out := []string{}
	for _, lvls := range src {
		for _, t := range lvls {
			if !seen[t] {
				seen[t] = true
				out = append(out, "farol."+t)
			}
		}
	}
	out = append(out,
		"farol.agg_"+fluxoPrefix+"_dims_mes",
		"farol.agg_"+fluxoPrefix+"_mkt_cli_mes",
		"farol.agg_"+fluxoPrefix+"_mkt_produto_mes",
	)
	return out
}

// purgeAggFluxo apaga todas as tabelas agregadas do fluxo dado e retorna o
// total de linhas removidas. Usado ao limpar vendas_* e pela ação "Limpar
// VIEWS". Quando adminTruncate=true, usa TRUNCATE (zera TODAS empresas —
// requer perfil admin e single-tenant efetivo); senão DELETE por empresa_id.
func purgeAggFluxo(db *sql.DB, empresaID, fluxoPrefix string, adminTruncate bool) int64 {
	var purged int64
	for _, at := range aggTablesForFluxo(fluxoPrefix) {
		if !tableExists(db, at) {
			continue
		}
		var an int64
		var aerr error
		if adminTruncate {
			an, aerr = truncateAllTenants(db, at)
		} else {
			an, aerr = deleteRowsForTenant(db, at, empresaID)
		}
		if aerr != nil {
			log.Printf("[cleanup] empresa=%s purge %s ERRO: %v", empresaID, at, aerr)
			continue
		}
		purged += an
	}
	return purged
}

func tableExists(db *sql.DB, name string) bool {
	var reg sql.NullString
	_ = db.QueryRow(`SELECT to_regclass($1)`, name).Scan(&reg)
	return reg.Valid
}

// deleteRowsForTenant — DELETE por empresa_id. Preserva dados de outros
// tenants (multi-tenant safe). Gera bloat no Postgres (tuples ficam como
// dead até VACUUM). É o padrão do "Limpar dados do cliente".
func deleteRowsForTenant(db *sql.DB, table, empresaID string) (int64, error) {
	res, err := db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE empresa_id=$1`, table), empresaID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// truncateAllTenants — TRUNCATE RESTART IDENTITY, ZERA TODA A TABELA
// (dados de TODAS as empresas). Instantâneo, devolve espaço em disco.
//
// ⚠️ APENAS para admin em ambientes single-tenant (dev, staging, cliente único)
// ou quando se sabe que essa é a única empresa da instância. Verificação de
// perfil é feita no handler (RequireAdmin).
//
// Retorna a contagem que existia antes do TRUNCATE (pra reportar no log).
func truncateAllTenants(db *sql.DB, table string) (int64, error) {
	var n int64
	_ = db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s`, table)).Scan(&n)
	if _, err := db.Exec(fmt.Sprintf(`TRUNCATE TABLE %s RESTART IDENTITY`, table)); err != nil {
		return 0, err
	}
	return n, nil
}

// CleanupInventoryHandler — GET /api/v2/farol/cleanup/inventory
func CleanupInventoryHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		type item struct {
			cleanupTableSpec
			Count int64 `json:"count"`
		}
		out := make([]item, 0, len(cleanupTables))
		for _, t := range cleanupTables {
			var n int64
			if tableExists(db, t.Table) {
				_ = db.QueryRow(
					fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE empresa_id=$1`, t.Table),
					spCtx.EmpresaID,
				).Scan(&n)
			}
			out = append(out, item{cleanupTableSpec: t, Count: n})
		}
		json.NewEncoder(w).Encode(map[string]any{"empresa_id": spCtx.EmpresaID, "tables": out})
	}
}

// CleanupExecuteHandler — POST /api/v2/farol/cleanup  {"tables":["vendas",...]}
func CleanupExecuteHandler(db *sql.DB) http.HandlerFunc {
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
		if !RequireWrite(spCtx, w) {
			return
		}

		var body struct {
			Tables         []string `json:"tables"`
			AdminTruncate  bool     `json:"admin_truncate,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Tables) == 0 {
			http.Error(w, `{"error":"informe as tabelas a limpar"}`, http.StatusBadRequest)
			return
		}

		// admin_truncate zera a tabela inteira (todas empresas). Restrito ao
		// perfil admin_fbtax (super-usuário cross-tenant). Devolve espaço em
		// disco imediatamente — ideal para dev/staging/instância single-tenant.
		if body.AdminTruncate && !spCtx.IsAdminFbtax() {
			http.Error(w, `{"error":"admin_truncate exige perfil admin_fbtax"}`, http.StatusForbidden)
			return
		}
		modo := "delete-tenant"
		if body.AdminTruncate {
			modo = "TRUNCATE-ALL"
		}
		log.Printf("[cleanup] empresa=%s user=%s modo=%s tabelas=%v", spCtx.EmpresaID, spCtx.UserID, modo, body.Tables)

		deleted := make(map[string]int64)
		needRefresh := false
		for _, key := range body.Tables {
			spec := cleanupSpecByKey(key)
			if spec == nil {
				continue // ignora chaves desconhecidas — allowlist
			}

			// Ação "Limpar VIEWS": purga todas as agg do fluxo (sem DELETE simples).
			// Habilitada mesmo com vendas_* já vazias — remove dados fantasma.
			if spec.AggFluxo != "" {
				purged := purgeAggFluxo(db, spCtx.EmpresaID, spec.AggFluxo, body.AdminTruncate)
				deleted[spec.Key] = purged
				needRefresh = true
				log.Printf("[cleanup] empresa=%s Limpar VIEWS agg_%s_* purgadas=%d modo=%s", spCtx.EmpresaID, spec.AggFluxo, purged, modo)
				continue
			}

			if !tableExists(db, spec.Table) {
				continue
			}
			var n int64
			var err error
			if body.AdminTruncate {
				n, err = truncateAllTenants(db, spec.Table)
			} else {
				n, err = deleteRowsForTenant(db, spec.Table, spCtx.EmpresaID)
			}
			if err != nil {
				log.Printf("[cleanup] empresa=%s tabela=%s ERRO: %v", spCtx.EmpresaID, spec.Table, err)
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			deleted[spec.Key] = n
			if spec.RefreshViews {
				needRefresh = true
			}
			log.Printf("[cleanup] empresa=%s tabela=%s removidas=%d modo=%s", spCtx.EmpresaID, spec.Table, n, modo)

			// CRÍTICO: limpar vendas_* sem limpar as agg_*_mes deixa o painel
			// mostrando dados fantasma (o painel lê das agg, e upsert_aggs_mes só
			// faz UPSERT — nunca remove órfãs). Purga as agregadas do mesmo fluxo.
			if spec.Key == "vendas_faturadas" || spec.Key == "vendas_transmitidas" {
				prefix := "fat"
				if spec.Key == "vendas_transmitidas" {
					prefix = "trans"
				}
				purged := purgeAggFluxo(db, spCtx.EmpresaID, prefix, body.AdminTruncate)
				deleted[spec.Key+"_agg"] = purged
				log.Printf("[cleanup] empresa=%s agg_%s_* purgadas=%d modo=%s", spCtx.EmpresaID, prefix, purged, modo)
			}
		}

		if needRefresh {
			if err := refreshAllFarolViews(db); err != nil {
				log.Printf("[cleanup] empresa=%s REFRESH falhou: %v", spCtx.EmpresaID, err)
			}
		}

		writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "dados", "all", "limpar_dados",
			map[string]any{"deleted": deleted})

		json.NewEncoder(w).Encode(map[string]any{"ok": true, "deleted": deleted})
	}
}
