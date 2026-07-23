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
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Quantos itens cada bloco da tela realmente mostra (o resto é peso morto no fio).
const (
	biTopIndustrias = 8 // + "Outros" agregado
	biPareto        = 5 // "top 5 indústrias = X%" (concentração)
)

type biIndustria struct {
	Label    string  `json:"label"`
	Faturado float64 `json:"faturado"`
	// Pct/Cor = atingimento vs período anterior (YoY), já calculados na V01.
	// A fatia "Outros" fica com Cor vazia (não faz sentido colorir a cauda).
	Pct float64 `json:"pct"`
	Cor string  `json:"cor"`
}

// biUF — faturado por estado (UF do cliente) com atingimento YoY.
type biUF struct {
	Estado      string  `json:"estado"`
	Faturado    float64 `json:"faturado"`
	FaturadoAnt float64 `json:"faturado_ant"`
	Pct         float64 `json:"pct"`
	Cor         string  `json:"cor"`
}

type biPeriodo struct {
	Fluxo    string `json:"fluxo"`
	CurLabel string `json:"cur_label"`
	AntLabel string `json:"ant_label"`
}

type biResponse struct {
	KPI        kpiSummary    `json:"kpi"`
	Industrias []biIndustria `json:"industrias"`
	UFs        []biUF        `json:"ufs"`
	// ConcentracaoTop5 — % do faturado total de indústria nas 5 maiores (0-100).
	// Sinal de risco de dependência para o CEO.
	ConcentracaoTop5 float64   `json:"concentracao_top5"`
	Pulso            pulsoResp `json:"pulso"`
	Periodo          biPeriodo `json:"periodo"`
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
			UFs:        []biUF{},
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
			ufs        []biUF
			ufOK       bool
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
		// Indústria (donut + Pareto) — V01 nível Fornecedor.
		go bloco("industrias", func() { indCards = biFetchL0(db, spCtx.EmpresaID, fluxo, "V01", pr) })
		// UF — faturado por estado (scan de vendas_*, dimensão sem agg).
		go bloco("ufs", func() { ufs, ufOK = biFaturadoPorUF(db, spCtx.EmpresaID, fluxo, pr) })
		// Pulso de ontem + carimbo do último import (dois SELECTs curtos).
		go bloco("pulso", func() {
			pulso = computePulso(db, spCtx.EmpresaID, "", nil)
			atualizado = biUltimoImport(db, spCtx.EmpresaID)
		})
		wg.Wait()

		out.KPI = kpi
		out.Industrias, out.ConcentracaoTop5 = biIndustriasEConcentracao(indCards)
		if ufs != nil { // falha do scan devolve nil; mantém [] p/ o JSON não virar null
			out.UFs = ufs
		}
		out.Pulso = pulso
		out.AtualizadoEm = atualizado
		out.Periodo.CurLabel, out.Periodo.AntLabel, _ = buildPeriodoLabels(pr)

		// Não cachear resposta degradada — congelaria um painel quebrado por 10
		// min DEPOIS do banco voltar. Dois gatilhos:
		//  - tudo vazio: os blocos de agg devolvem vazio tanto em falha quanto em
		//    "sem venda"; só o conjunto todo zerado é sinal forte de falha (ou
		//    empresa em setup, que também não é caso de uso do cache).
		//  - UF falhou: é o único bloco cujo erro conseguimos distinguir de
		//    "sem dado" (os de agg passam por fetchCards, que engole o erro).
		vazio := len(out.Industrias) == 0 && len(out.UFs) == 0 && kpi.TotalFaturado == 0
		if vazio || !ufOK {
			log.Printf("[farol:bi] resposta degradada (vazio=%t ufOK=%t) empresa=%s fluxo=%s modo=%s — não cacheada",
				vazio, ufOK, spCtx.EmpresaID, fluxo.name, compMode)
		} else if !biStore(spCtx.EmpresaID, key, gen, out) {
			log.Printf("[farol:bi] descartado: dados invalidados durante o cálculo (empresa=%s)", spCtx.EmpresaID)
		}

		log.Printf("[farol:bi] CACHE MISS empresa=%s fluxo=%s modo=%s — %d ind, %d ufs em %v",
			spCtx.EmpresaID, fluxo.name, compMode, len(out.Industrias), len(out.UFs), time.Since(t0))

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

// biIndustriasEConcentracao — top 8 por valor (cauda vira "Outros"), levando
// junto o atingimento (pct/cor) que a V01 já calcula, e o Pareto: quanto do
// faturado de indústria está nas 5 maiores.
//
// Só valores POSITIVOS entram no donut e no Pareto. Líquido negativo (mês com
// devolução > venda numa indústria) não vira fatia — o PieChart não desenha
// ângulo negativo — e não pode entrar no denominador do Pareto: numerador
// só-positivo sobre um total que soma negativos daria concentração > 100%.
func biIndustriasEConcentracao(cards []cardItem) ([]biIndustria, float64) {
	sorted := biOrdenaPorValor(cards)

	var total, top5 float64
	positivos := 0
	for _, c := range sorted {
		v := biValor(c)
		if v <= 0 {
			continue
		}
		total += v
		if positivos < biPareto { // 5 maiores positivos (sorted é desc)
			top5 += v
		}
		positivos++
	}

	out := make([]biIndustria, 0, biTopIndustrias+1)
	var outros float64
	emitidos := 0
	for _, c := range sorted {
		v := biValor(c)
		if v <= 0 {
			continue // fatia (top-8 ou cauda) só existe para valor positivo
		}
		if emitidos < biTopIndustrias {
			out = append(out, biIndustria{Label: c.Label, Faturado: v, Pct: c.Pct, Cor: c.Cor})
			emitidos++
			continue
		}
		outros += v
	}
	// A fatia "Outros" não recebe cor de atingimento.
	if outros > 0 {
		out = append(out, biIndustria{Label: "Outros", Faturado: outros})
	}

	concentracao := 0.0
	if total > 0 {
		concentracao = top5 / total * 100 // ∈ [0,100]: top5 ⊆ total (ambos positivos)
	}
	return out, concentracao
}

// biFaturadoPorUF — faturado por estado (UF do cliente) com atingimento YoY.
//
// Faturado: lê a MV farol.mv_fat_uf_mes (mig 194), que precomputa o LÍQUIDO por
// UF com a mesma fórmula por-linha da mig 190 (líquido = venda_real − devol −
// cancel). O total por UF reconcilia com os gauges/donut no caso comum; pode
// diferir por dois cascos raros que os aggs tratam diferente: (1) linhas de
// chave vazia — os aggs excluem (cod_fornec/gerente <> ”), a MV inclui (órfãos
// viram sentinela 99999999, não vazio, então raro); (2) devolução/cancelamento
// sem faturado casado no mês — o agg só subtrai via JOIN por chave, a MV
// subtrai por UF sempre. Ambos negligenciáveis; ver deferred-work.
// Grão mensal, então o range parcial expande pro mês inteiro, igual ao resto
// do painel (BI só faz ytd/mtd).
//
// Transmitido: não há MV; o transmitido exibe BRUTO em todo o painel, então
// scan de vendas_transmitidas (SUM pvenda) por UF é coerente.
//
// Retorna `ok=false` quando a query do período ATUAL falha (erro de banco). É o
// único bloco cujo erro o handler consegue distinguir de "sem dado" — os blocos
// de agg passam por fetchCards, que engole o erro. O handler usa esse sinal
// para NÃO cachear uma resposta com UF vazio por falha (evita congelar o bloco
// por 10 min depois do banco voltar).
func biFaturadoPorUF(db *sql.DB, empresaID string, fluxo fluxoCtx, pr periodResolution) (result []biUF, ok bool) {
	hasComp := !pr.CompInicio.IsZero() && !pr.CompFim.IsZero()

	scan := func(inicio, fim time.Time) (map[string]float64, error) {
		args := []any{empresaID}
		var q string
		if fluxo.name == "transmitido" {
			cond := buildRangeCond(fluxo.dateCol, inicio, fim, &args) // v.<dateCol> BETWEEN ...
			q = fmt.Sprintf(`
				SELECT COALESCE(NULLIF(v.uf,''),'—') AS uf, COALESCE(SUM(v.pvenda),0) AS fat
				FROM %s v
				WHERE v.empresa_id=$1 AND %s
				GROUP BY 1`, fluxo.tableName, cond)
		} else {
			// MV: uf já vem coalescida; grão mensal via buildMesCond (usa alias v).
			cond := buildMesCond(ym(inicio), ym(fim), &args)
			q = fmt.Sprintf(`
				SELECT v.uf, COALESCE(SUM(v.liquido),0) AS fat
				FROM farol.mv_fat_uf_mes v
				WHERE v.empresa_id=$1 AND %s
				GROUP BY v.uf`, cond)
		}
		rows, err := db.Query(q, args...)
		if err != nil {
			log.Printf("[farol:bi] biFaturadoPorUF (%s) ERRO: %v", fluxo.name, err)
			return nil, err
		}
		defer rows.Close()
		m := make(map[string]float64)
		for rows.Next() {
			var uf string
			var fat float64
			if rows.Scan(&uf, &fat) == nil {
				m[uf] = fat
			}
		}
		return m, nil
	}

	var (
		atual, ant map[string]float64
		errAtual   error
		wg         sync.WaitGroup
	)
	wg.Add(1)
	go func() { defer wg.Done(); atual, errAtual = scan(pr.RefInicio, pr.RefFim) }()
	if hasComp {
		wg.Add(1)
		// Erro no comparativo é tolerado: só apaga o atingimento (cor vira verde
		// neutro para os UFs sem `ant`). Não invalida o bloco.
		go func() { defer wg.Done(); ant, _ = scan(pr.CompInicio, pr.CompFim) }()
	}
	wg.Wait()

	if errAtual != nil {
		return nil, false
	}

	out := make([]biUF, 0, len(atual))
	for uf, fat := range atual {
		antFat := ant[uf]
		out = append(out, biUF{
			Estado: uf, Faturado: fat, FaturadoAnt: antFat,
			Pct: biPct(fat, antFat, hasComp), Cor: biCor(fat, antFat, hasComp),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Faturado > out[j].Faturado })
	return out, true
}

// biPct / biCor — régua idêntica ao pickCor do fetchCards (farol_v2_api.go):
// sem comparativo → neutro/verde; senão verde se atual ≥ anterior.
func biPct(atual, ant float64, hasComp bool) float64 {
	if !hasComp {
		return 0
	}
	if ant > 0 {
		return atual / ant * 100
	}
	if atual > 0 {
		return 100
	}
	return 0
}

func biCor(atual, ant float64, hasComp bool) string {
	if biPct(atual, ant, hasComp) >= 100 || !hasComp {
		return "verde"
	}
	return "vermelho"
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

// refreshUFMV recomputa a MV de faturado por UF (mig 194). CONCURRENTLY não
// bloqueia leituras; cai para o REFRESH comum se não houver índice único ainda
// (primeira vez / ambiente sem a mig). Chamada nos mesmos pontos que consolidam
// as agg. Falha aqui não derruba a consolidação — só loga.
func refreshUFMV(db *sql.DB) {
	if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY farol.mv_fat_uf_mes`); err != nil {
		// Loga a falha do CONCURRENTLY (senão o fallback bloqueante fica
		// invisível): pode ser refresh concorrente de outro import, ou a 1ª
		// carga antes do índice único. O REFRESH comum toma ACCESS EXCLUSIVE
		// e trava a leitura do bloco UF durante a consolidação.
		log.Printf("[farol:bi] refreshUFMV CONCURRENTLY falhou (%v) → REFRESH bloqueante", err)
		if _, err2 := db.Exec(`REFRESH MATERIALIZED VIEW farol.mv_fat_uf_mes`); err2 != nil {
			log.Printf("[farol:bi] refreshUFMV ERRO: %v", err2)
		}
	}
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
