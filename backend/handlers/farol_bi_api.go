package handlers

// farol_bi_api.go — endpoint consolidado do Painel BI (War Room).
//
// GET /api/v2/farol/bi?comp_mode=ytd|mtd&fluxo=faturado|transmitido&nocache=1
//
// POR QUE EXISTE: o BI abria disparando 4 requests paralelas (/cards ×3 +
// /pulso). Cada /cards resolvia o período do zero (inferLastDay), recalculava
// positivação (3 COUNT(DISTINCT cnpj) sem cache no fixOverlappingBaseKPI) e
// ainda listava os períodos disponíveis — campo que o BI nem lê. Aqui o
// período é resolvido UMA vez, as 3 views + o pulso rodam em paralelo no
// servidor e a resposta sai enxuta (só o que a tela desenha), com cache em
// memória invalidado pelo RefreshViews.
//
// A semântica dos números é a MESMA do /cards: mesmo pr, mesmo fetchCards,
// mesmo computeKPI/fixOverlappingBaseKPI. O BI não pode divergir do Executivo.

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Quantos itens cada bloco da tela realmente mostra (o resto é peso morto no fio).
const (
	biTopIndustrias = 8  // + "Outros" agregado
	biTopEquipes    = 12 // ranking horizontal
)

type biIndustria struct {
	Label    string  `json:"label"`
	Faturado float64 `json:"faturado"`
}

type biEquipe struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Faturado float64 `json:"faturado"`
	Pct      float64 `json:"pct"`
}

type biPeriodo struct {
	Fluxo    string `json:"fluxo"`
	CurLabel string `json:"cur_label"`
	AntLabel string `json:"ant_label"`
}

type biResponse struct {
	KPI        kpiSummary    `json:"kpi"`
	Industrias []biIndustria `json:"industrias"`
	Equipes    []biEquipe    `json:"equipes"`
	Pulso      pulsoResp     `json:"pulso"`
	Periodo    biPeriodo     `json:"periodo"`
	// AtualizadoEm — RFC3339 do último import concluído. Vazio se nunca houve.
	// O front carimba a tela com isso; é o que diz ao gestor se o painel na TV
	// está mostrando dado de hoje ou de anteontem.
	AtualizadoEm string `json:"atualizado_em"`
}

// ─── Cache de resposta ───────────────────────────────────────────────────────
//
// TTL curto + invalidação explícita no RefreshViews. O TTL é só rede de
// segurança (import feito por fora, restart, etc.); o caminho normal é a
// invalidação. Chave = empresa|fluxo|comp_mode — o BI não tem drill nem filtro.

type biCacheEntry struct {
	data biResponse
	at   time.Time
}

var (
	biCacheMu sync.RWMutex
	biCache   = map[string]biCacheEntry{}
)

const biCacheTTL = 10 * time.Minute

func biCacheKey(empresaID, fluxoName, compMode string) string {
	return empresaID + "|" + fluxoName + "|" + compMode
}

func invalidateBICache(empresaID string) {
	biCacheMu.Lock()
	for k := range biCache {
		if strings.HasPrefix(k, empresaID+"|") {
			delete(biCache, k)
		}
	}
	biCacheMu.Unlock()
}

// ─── Handler ─────────────────────────────────────────────────────────────────

func FarolV2BIHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()
		fluxo := resolveFluxo(q.Get("fluxo"))
		// O painel só oferece "Acumulado Ano" (ytd) e "Mês Atual" (mtd).
		compMode := strings.ToLower(strings.TrimSpace(q.Get("comp_mode")))
		if compMode != "mtd" {
			compMode = "ytd"
		}

		key := biCacheKey(spCtx.EmpresaID, fluxo.name, compMode)
		if q.Get("nocache") != "1" {
			biCacheMu.RLock()
			e, ok := biCache[key]
			biCacheMu.RUnlock()
			if ok && time.Since(e.at) < biCacheTTL {
				log.Printf("[farol:bi] CACHE HIT empresa=%s fluxo=%s modo=%s (idade %v)",
					spCtx.EmpresaID, fluxo.name, compMode, time.Since(e.at).Round(time.Second))
				json.NewEncoder(w).Encode(e.data)
				return
			}
		}

		t0 := time.Now()
		pr := resolvePeriods(db, spCtx.EmpresaID, map[string][]string{"comp_mode": {compMode}})

		out := biResponse{
			Industrias: []biIndustria{},
			Equipes:    []biEquipe{},
			Periodo:    biPeriodo{Fluxo: fluxo.name},
		}

		// Empresa sem dados consolidados: devolve estrutura vazia (a tela mostra
		// "Sem dados" em vez de girar o spinner) e NÃO cacheia — é estado de setup.
		if pr.RefInicio.IsZero() || pr.RefFim.IsZero() {
			out.Pulso.SemDado = true
			out.Pulso.Cor = "vermelho"
			json.NewEncoder(w).Encode(out)
			return
		}

		var (
			wg         sync.WaitGroup
			kpi        kpiSummary
			indCards   []cardItem
			eqCards    []cardItem
			pulso      pulsoResp
			atualizado string
		)

		wg.Add(4)
		// KPI (gauges) — V03, exatamente como o /cards monta hoje.
		go func() { defer wg.Done(); kpi = biKPI(db, spCtx.EmpresaID, fluxo, pr) }()
		// Donut — V01 nível Fornecedor.
		go func() { defer wg.Done(); indCards = biFetchL0(db, spCtx.EmpresaID, fluxo, "V01", pr) }()
		// Ranking — V02 nível Supervisor.
		go func() { defer wg.Done(); eqCards = biFetchL0(db, spCtx.EmpresaID, fluxo, "V02", pr) }()
		// Pulso de ontem + carimbo do último import (dois SELECTs curtos).
		go func() {
			defer wg.Done()
			pulso = computePulso(db, spCtx.EmpresaID, "", nil)
			atualizado = biUltimoImport(db, spCtx.EmpresaID)
		}()
		wg.Wait()

		out.KPI = kpi
		out.Industrias = biTopIndustriasComOutros(indCards)
		out.Equipes = biTopEquipesDe(eqCards)
		out.Pulso = pulso
		out.AtualizadoEm = atualizado
		out.Periodo.CurLabel, out.Periodo.AntLabel, _ = buildPeriodoLabels(pr)

		biCacheMu.Lock()
		biCache[key] = biCacheEntry{data: out, at: time.Now()}
		biCacheMu.Unlock()

		log.Printf("[farol:bi] CACHE MISS empresa=%s fluxo=%s modo=%s — %d ind, %d equipes em %v",
			spCtx.EmpresaID, fluxo.name, compMode, len(out.Industrias), len(out.Equipes), time.Since(t0))

		json.NewEncoder(w).Encode(out)
	}
}

// ─── Blocos ──────────────────────────────────────────────────────────────────

// biFetchL0 roda fetchCards no nível raiz da view — mesmo caminho do /cards,
// sem drill nem filtro (o BI é sempre a visão da empresa inteira).
func biFetchL0(db *sql.DB, empresaID string, fluxo fluxoCtx, view string, pr periodResolution) []cardItem {
	hier, ok := hierarquias[view]
	if !ok || len(hier) == 0 {
		return nil
	}
	return fetchCards(db, empresaID, fluxo, view, pr, 0, hier[0], nil, nil)
}

// biKPI monta o totalizador dos gauges a partir da V03, replicando o que o
// FarolV2CardsHandler faz para view=V03 no nível raiz (inclusive a correção de
// positivação sobreposta).
func biKPI(db *sql.DB, empresaID string, fluxo fluxoCtx, pr periodResolution) kpiSummary {
	hier, ok := hierarquias["V03"]
	if !ok || len(hier) == 0 {
		return kpiSummary{}
	}
	level := hier[0]
	cards := fetchCards(db, empresaID, fluxo, "V03", pr, 0, level, nil, nil)
	kpi := computeKPI(cards, fluxo.name, level.Level == "cod_fornec")
	if level.Level != "cod_prod" && level.Level != "cod_cli" &&
		leafServesPositivados(fluxo, "V03", level.Level, nil, nil) {
		fixOverlappingBaseKPI(db, &kpi, fluxo, "V03", empresaID, pr, nil, nil)
	}
	return kpi
}

// biTopIndustriasComOutros — top 8 por faturado; a cauda vira uma fatia "Outros".
// Agregar aqui (e não no front) deixa o donut apenas pintando o que recebeu.
func biTopIndustriasComOutros(cards []cardItem) []biIndustria {
	sorted := make([]cardItem, len(cards))
	copy(sorted, cards)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Faturado > sorted[j].Faturado })

	out := make([]biIndustria, 0, biTopIndustrias+1)
	var outros float64
	for i, c := range sorted {
		if i < biTopIndustrias {
			out = append(out, biIndustria{Label: c.Label, Faturado: c.Faturado})
			continue
		}
		outros += c.Faturado
	}
	if outros > 0 {
		out = append(out, biIndustria{Label: "Outros", Faturado: outros})
	}
	return out
}

// biTopEquipesDe — top 12 por faturado, na ordem em que o ranking desenha.
func biTopEquipesDe(cards []cardItem) []biEquipe {
	sorted := make([]cardItem, len(cards))
	copy(sorted, cards)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Faturado > sorted[j].Faturado })
	if len(sorted) > biTopEquipes {
		sorted = sorted[:biTopEquipes]
	}
	out := make([]biEquipe, 0, len(sorted))
	for _, c := range sorted {
		out = append(out, biEquipe{Key: c.Key, Label: c.Label, Faturado: c.Faturado, Pct: c.Pct})
	}
	return out
}

// biUltimoImport — horário do último import que efetivamente entrou.
// Vazio quando a empresa nunca concluiu um import.
func biUltimoImport(db *sql.DB, empresaID string) string {
	var t sql.NullTime
	err := db.QueryRow(
		`SELECT MAX(atualizado_em) FROM vendas_import_jobs WHERE empresa_id=$1 AND status='done'`,
		empresaID).Scan(&t)
	if err != nil {
		log.Printf("[farol:bi] biUltimoImport empresa=%s ERRO: %v", empresaID, err)
		return ""
	}
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}
