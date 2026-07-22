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
	// biGen — contador de invalidações por empresa, protegido por biCacheMu.
	// Existe por causa de uma corrida real: o cálculo leva dezenas de segundos;
	// se o import terminar NO MEIO dele, a invalidação limpa um cache vazio e
	// logo depois a goroutine grava o resultado PRÉ-import, que então vive o TTL
	// inteiro. Comparar a geração antes de gravar descarta esse resultado velho.
	biGen = map[string]uint64{}
)

const biCacheTTL = 10 * time.Minute

func biCacheKey(empresaID, fluxoName, compMode string) string {
	return empresaID + "|" + fluxoName + "|" + compMode
}

func biGeneration(empresaID string) uint64 {
	biCacheMu.RLock()
	defer biCacheMu.RUnlock()
	return biGen[empresaID]
}

// biStore grava a resposta apenas se nenhuma invalidação ocorreu desde `gen`.
func biStore(empresaID, key string, gen uint64, data biResponse) bool {
	biCacheMu.Lock()
	defer biCacheMu.Unlock()
	if biGen[empresaID] != gen {
		return false
	}
	biCache[key] = biCacheEntry{data: data, at: time.Now()}
	return true
}

func invalidateBICache(empresaID string) {
	biCacheMu.Lock()
	for k := range biCache {
		if strings.HasPrefix(k, empresaID+"|") {
			delete(biCache, k)
		}
	}
	biGen[empresaID]++
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
		// Só faturado/transmitido. resolveFluxo também aceita cancdev/cortado,
		// que não têm agg: cairiam em scan da base 3× em janela YTD — qualquer
		// URL forjada viraria um tiro de performance no Postgres.
		fluxo := resolveFluxo(q.Get("fluxo"))
		if fluxo.name != "faturado" && fluxo.name != "transmitido" {
			fluxo = resolveFluxo("faturado")
		}
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
		// Geração ANTES de computar — ver comentário em biGen.
		gen := biGeneration(spCtx.EmpresaID)
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
			out.Periodo.CurLabel = "Sem dados"
			out.AtualizadoEm = biUltimoImport(db, spCtx.EmpresaID)
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

		// Cada bloco roda isolado: um panic aqui dentro NÃO pode derrubar o
		// processo. O recover do net/http só cobre a goroutine do handler, e
		// este binário serve também as URLs do ION VENDAS em campo.
		bloco := func(nome string, fn func()) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[farol:bi] PANIC no bloco %s: %v", nome, rec)
				}
			}()
			fn()
		}

		wg.Add(4)
		// KPI (gauges) — V03, exatamente como o /cards monta hoje.
		go bloco("kpi", func() { kpi = biKPI(db, spCtx.EmpresaID, fluxo, pr) })
		// Donut — V01 nível Fornecedor.
		go bloco("industrias", func() { indCards = biFetchL0(db, spCtx.EmpresaID, fluxo, "V01", pr) })
		// Ranking — V02 nível Supervisor.
		go bloco("equipes", func() { eqCards = biFetchL0(db, spCtx.EmpresaID, fluxo, "V02", pr) })
		// Pulso de ontem + carimbo do último import (dois SELECTs curtos).
		go bloco("pulso", func() {
			pulso = computePulso(db, spCtx.EmpresaID, "", nil)
			atualizado = biUltimoImport(db, spCtx.EmpresaID)
		})
		wg.Wait()

		out.KPI = kpi
		out.Industrias = biTopIndustriasComOutros(indCards)
		out.Equipes = biTopEquipesDe(eqCards)
		out.Pulso = pulso
		out.AtualizadoEm = atualizado
		out.Periodo.CurLabel, out.Periodo.AntLabel, _ = buildPeriodoLabels(pr)

		// As queries abaixo logam e devolvem vazio em vez de erro (padrão do
		// fetchCards), então "tudo zerado" é indistinguível de falha de banco.
		// Cachear isso congelaria um painel zerado por 10 min DEPOIS do banco
		// voltar. Empresa legitimamente sem venda no período também não cacheia
		// — o custo é um miss por request, e ela não é o caso de uso do BI.
		degradado := len(out.Industrias) == 0 && len(out.Equipes) == 0 && kpi.TotalFaturado == 0
		if degradado {
			log.Printf("[farol:bi] resposta VAZIA empresa=%s fluxo=%s modo=%s — não cacheada (pode ser falha de query)",
				spCtx.EmpresaID, fluxo.name, compMode)
		} else if !biStore(spCtx.EmpresaID, key, gen, out) {
			log.Printf("[farol:bi] descartado: dados invalidados durante o cálculo (empresa=%s)", spCtx.EmpresaID)
		}

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

// biValor — o valor do card no fluxo pedido. cardItem.Faturado só é preenchido
// no fluxo faturado (no transmitido o valor vai para .Transmitido), então usar
// .Faturado direto zeraria donut e ranking inteiros em ?fluxo=transmitido.
// ValorAtual carrega o mesmo número nos dois fluxos.
func biValor(c cardItem) float64 { return c.ValorAtual }

// biOrdenaPorValor devolve uma cópia ordenada por valor desc. SliceStable
// porque empate é comum (vários zerados no começo do mês) e ordem instável
// congelada no cache faz o ranking "mudar sozinho" entre refreshes.
func biOrdenaPorValor(cards []cardItem) []cardItem {
	sorted := make([]cardItem, len(cards))
	copy(sorted, cards)
	sort.SliceStable(sorted, func(i, j int) bool { return biValor(sorted[i]) > biValor(sorted[j]) })
	return sorted
}

// biTopIndustriasComOutros — top 8 por valor; a cauda vira uma fatia "Outros".
// Agregar aqui (e não no front) deixa o donut apenas pintando o que recebeu.
func biTopIndustriasComOutros(cards []cardItem) []biIndustria {
	sorted := biOrdenaPorValor(cards)

	out := make([]biIndustria, 0, biTopIndustrias+1)
	var outros float64
	for i, c := range sorted {
		if i < biTopIndustrias {
			out = append(out, biIndustria{Label: c.Label, Faturado: biValor(c)})
			continue
		}
		outros += biValor(c)
	}
	// Cauda negativa (devolução > venda) não vira fatia: o donut não desenha
	// ângulo negativo. Mesma regra que o front aplicava antes.
	if outros > 0 {
		out = append(out, biIndustria{Label: "Outros", Faturado: outros})
	}
	return out
}

// biTopEquipesDe — top 12 por valor, na ordem em que o ranking desenha.
func biTopEquipesDe(cards []cardItem) []biEquipe {
	sorted := biOrdenaPorValor(cards)
	if len(sorted) > biTopEquipes {
		sorted = sorted[:biTopEquipes]
	}
	out := make([]biEquipe, 0, len(sorted))
	for _, c := range sorted {
		out = append(out, biEquipe{Key: c.Key, Label: c.Label, Faturado: biValor(c), Pct: c.Pct})
	}
	return out
}

// biUltimoImport — de quando é o dado que está na tela.
//
// Fonte primária: farol.consolidacao_log (mig 193), gravado quando a
// consolidação TERMINA. É o carimbo honesto: enquanto o upsert_aggs_mes não
// roda, os números na tela continuam sendo os da carga anterior.
//
// Fallback: MAX(atualizado_em) dos jobs concluídos — usado só enquanto a
// empresa não passou por nenhuma consolidação após a mig 193. Esse critério
// carimba o fim do UPLOAD, e no import multi-arquivo (skip_refresh) o upload
// fecha bem antes da consolidação; por isso ele é fallback, não fonte.
//
// Vazio quando não há nem um nem outro.
func biUltimoImport(db *sql.DB, empresaID string) string {
	var t sql.NullTime
	err := db.QueryRow(
		`SELECT concluido_em FROM farol.consolidacao_log WHERE empresa_id=$1`,
		empresaID).Scan(&t)
	if err == nil && t.Valid {
		return t.Time.Format(time.RFC3339)
	}
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[farol:bi] consolidacao_log empresa=%s ERRO: %v", empresaID, err)
	}

	if err := db.QueryRow(
		`SELECT MAX(atualizado_em) FROM vendas_import_jobs WHERE empresa_id=$1 AND status='done'`,
		empresaID).Scan(&t); err != nil {
		log.Printf("[farol:bi] biUltimoImport (fallback) empresa=%s ERRO: %v", empresaID, err)
		return ""
	}
	if !t.Valid {
		return ""
	}
	return t.Time.Format(time.RFC3339)
}

// marcaConsolidacao registra que a consolidação da empresa terminou AGORA.
// Chamada nos dois caminhos que rodam upsert_aggs_mes (import e RefreshViews).
// Falha aqui não pode derrubar a consolidação — só loga: o pior efeito é o
// painel exibir um carimbo mais antigo do que a realidade.
func marcaConsolidacao(db *sql.DB, empresaID string) {
	if _, err := db.Exec(`
		INSERT INTO farol.consolidacao_log (empresa_id, concluido_em)
		VALUES ($1, now())
		ON CONFLICT (empresa_id) DO UPDATE SET concluido_em = EXCLUDED.concluido_em`,
		empresaID); err != nil {
		log.Printf("[farol:bi] marcaConsolidacao empresa=%s ERRO: %v", empresaID, err)
	}
}
