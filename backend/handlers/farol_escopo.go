package handlers

// farol_escopo.go — recorte obrigatório por persona no ambiente WEB autenticado.
//
// POR QUE EXISTE: até 14/08/2026 o Farol web não tinha escopo hierárquico
// nenhum. `spCtx` era lido 25 vezes em farol_v2_api.go e SEMPRE só para
// EmpresaID (o tenant); TipoPersona e CodReferencia nunca eram consultados.
// Na prática isso não vazava dados porque o SyncUsuarios cria GGV/supervisor/
// RCA com sp_role='somente_leitura' e as rotas do Farol exigem 'gestor_filial'
// — eles eram barrados na porta (403). Mas bastava elevar o papel de alguém
// para destravar o acesso e ele passaria a ver a empresa inteira.
//
// O dado necessário sempre esteve lá: o SyncUsuarios já grava tipo_persona
// ('ggv'|'supervisor'|'rca') e cod_referencia (o cod_gerente/cod_supervisor/
// cod_rca da importação). Este arquivo é o que finalmente os usa.
//
// REGRA DE OURO: o escopo SOBRESCREVE o que vier do request. Nunca confiar em
// filtro mandado pelo cliente — quem controla a URL controlaria o próprio
// escopo.

import (
	"database/sql"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

// escopoRecorte — o que este usuário pode enxergar.
//
// Col vazio = sem restrição (CEO, diretor, gerente geral, TI, admin, analista).
// Negar = persona restrita cujo cod_referencia está vazio: o acesso é NEGADO,
// nunca liberado. Sem isso, um cadastro incompleto viraria acesso total — o
// erro cairia para o lado errado.
type escopoRecorte struct {
	Col      string   // cod_gerente | cod_supervisor | cod_rca
	Vals     []string // códigos que o usuário pode ver (>1 durante cobertura de férias)
	Negar    bool
	Persona  string // guardado para log/diagnóstico
	Cobertos []string
}

func (e escopoRecorte) restrito() bool { return e.Col != "" }

// personaEscopoCol — a coluna da hierarquia que limita cada persona.
// Personas ausentes deste switch veem a empresa inteira, por decisão:
// ceo/diretor/gerente_geral precisam do consolidado, ti/admin operam o
// sistema, analista_negocios analisa o todo.
func personaEscopoCol(persona string) string {
	switch persona {
	case "ggv":
		return "cod_gerente"
	case "supervisor":
		return "cod_supervisor"
	case "rca":
		return "cod_rca"
	}
	return ""
}

// escopoDoUsuario resolve o recorte do usuário logado, incluindo as coberturas
// de férias vigentes hoje. `escopoPedido` vem do seletor da tela ("ver a equipe
// de quem estou cobrindo"): só é aceito se estiver no conjunto permitido —
// validar aqui é o que impede alguém de digitar outro código na URL.
func escopoDoUsuario(db *sql.DB, spCtx *FarolContext, escopoPedido string) escopoRecorte {
	if spCtx == nil {
		// Sem contexto não há como saber quem é: nega.
		return escopoRecorte{Col: "cod_gerente", Negar: true}
	}
	if spCtx.IsAdminFbtax() {
		return escopoRecorte{}
	}
	col := personaEscopoCol(spCtx.TipoPersona)
	if col == "" {
		return escopoRecorte{}
	}

	proprio := strings.TrimSpace(spCtx.CodReferencia)
	if proprio == "" {
		log.Printf("[farol:escopo] persona=%s user=%s SEM cod_referencia → acesso negado",
			spCtx.TipoPersona, spCtx.UserID)
		return escopoRecorte{Col: col, Negar: true, Persona: spCtx.TipoPersona}
	}

	cobertos := coberturasVigentes(db, spCtx.EmpresaID, spCtx.UserID, col)

	// Seletor de alternância: mostra UMA equipe por vez. Somar as duas mudaria
	// os totais do substituto durante as férias e seria lido como se a meta
	// dele tivesse crescido (decisão de 14/08/2026).
	escolhido := proprio
	if p := strings.TrimSpace(escopoPedido); p != "" && p != proprio {
		if contemString(cobertos, p) {
			escolhido = p
		} else {
			log.Printf("[farol:escopo] user=%s pediu escopo=%q fora do permitido — usando o próprio (%s)",
				spCtx.UserID, p, proprio)
		}
	}

	return escopoRecorte{
		Col: col, Vals: []string{escolhido},
		Persona: spCtx.TipoPersona, Cobertos: cobertos,
	}
}

func contemString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// aplicarEscopo grava o recorte nos filtros, SOBRESCREVENDO o que veio do
// request. Devolve false quando o acesso deve ser negado — nesse caso o
// handler responde vazio em vez de consultar.
func aplicarEscopo(filters multiFilters, e escopoRecorte) bool {
	if !e.restrito() {
		return true
	}
	if e.Negar {
		return false
	}
	filters[e.Col] = append([]string(nil), e.Vals...)
	return true
}

// escopoDimCond — restringe as OPÇÕES de um dropdown ao escopo do usuário.
// Sem isto o recorte funcionaria nos números mas o GGV continuaria vendo a
// lista dos 10 gerentes e dos 1.190 RCAs da empresa nos filtros — vazamento
// de organograma, e ainda por cima com opções que devolveriam tela vazia.
//
// A hierarquia inteira (gerente → supervisor → RCA) vive na agg_*_v03_l2_mes,
// então uma única subconsulta genérica resolve todas as combinações: os
// DESCENDENTES quando a dim é mais funda que o escopo (GGV → seus RCAs) e os
// ANCESTRAIS quando é mais rasa (supervisor → seu gerente).
//
// Dims que não são de pessoa (fornecedor, cliente, UF, filial, departamento)
// seguem completas: o GGV pode escolher qualquer indústria, e os números que
// voltarem já estarão recortados pela equipe dele.
func escopoDimCond(e escopoRecorte, dimName, fluxoPrefix, keyCol string, args *[]any) string {
	if !e.restrito() || e.Negar || len(e.Vals) == 0 {
		return ""
	}
	dimCol := map[string]string{
		"gerente":    "cod_gerente",
		"supervisor": "cod_supervisor",
		"rca":        "cod_rca",
	}[dimName]
	if dimCol == "" {
		return "" // dim que não é de pessoa
	}
	*args = append(*args, pq.Array(e.Vals))
	return " AND " + keyCol + " IN (SELECT DISTINCT " + dimCol +
		" FROM farol.agg_" + fluxoPrefix + "_v03_l2_mes" +
		" WHERE empresa_id=$1 AND " + e.Col + " = ANY($" + strconv.Itoa(len(*args)) + "))"
}

// ─── Reescrita de escopo para aproveitar agg ─────────────────────────────────
//
// Filtrar por GERENTE em "Indústrias" não tem agg que sirva: seria preciso uma
// tabela com folha em cod_fornec contendo cod_gerente, e nenhuma das onze views
// tem essa combinação. O resultado é o scan de vendas_* — 37s medidos em
// produção 14/08/2026 com o GGV 347.
//
// O supervisor não sofre disso porque a V05 é exatamente [supervisor → fornec].
// Como todo supervisor pertence a um GGV, "vendas do gerente X" e "vendas dos
// supervisores de X" são o mesmo conjunto — então trocamos o filtro e a V05
// passa a servir.
//
// A troca só é válida enquanto o vínculo for não-ambíguo. Se um supervisor
// apareceu sob dois gerentes diferentes no histórico (mudou de equipe), filtrar
// por ele traria também o período do gerente antigo — número errado, que é pior
// que lento. Nesse caso desistimos e deixamos seguir pelo scan.

type hierCacheEntry struct {
	sups []string
	ok   bool
	at   time.Time
}

var (
	hierCacheMu sync.RWMutex
	hierCache   = map[string]hierCacheEntry{}
)

// TTL de 1h: o organograma muda com cadastro novo (importação diária), não a
// cada request.
const hierCacheTTL = time.Hour

// supervisoresDoGerente devolve os supervisores dos gerentes dados e se a
// troca é segura (ok=false quando algum deles serviu a mais de um gerente).
func supervisoresDoGerente(db *sql.DB, empresaID, fluxoPrefix string, gerentes []string) ([]string, bool) {
	if db == nil || len(gerentes) == 0 {
		return nil, false
	}
	key := empresaID + "|" + fluxoPrefix + "|" + strings.Join(gerentes, ",")
	hierCacheMu.RLock()
	if e, ok := hierCache[key]; ok && time.Since(e.at) < hierCacheTTL {
		hierCacheMu.RUnlock()
		return e.sups, e.ok
	}
	hierCacheMu.RUnlock()

	// n_gerentes conta em QUANTOS gerentes distintos cada supervisor apareceu
	// em toda a história — é o que denuncia a troca de equipe.
	rows, err := db.Query(`
		SELECT s.cod_supervisor,
		       (SELECT COUNT(DISTINCT a.cod_gerente)
		          FROM farol.agg_`+fluxoPrefix+`_v03_l1_mes a
		         WHERE a.empresa_id = $1 AND a.cod_supervisor = s.cod_supervisor) AS n_gerentes
		  FROM (SELECT DISTINCT cod_supervisor
		          FROM farol.agg_`+fluxoPrefix+`_v03_l1_mes
		         WHERE empresa_id = $1 AND cod_gerente = ANY($2) AND cod_supervisor <> '') s`,
		empresaID, pq.Array(gerentes))
	if err != nil {
		log.Printf("[farol:escopo] supervisoresDoGerente ERRO (seguindo sem reescrita): %v", err)
		return nil, false
	}
	defer rows.Close()

	var sups []string
	seguro := true
	for rows.Next() {
		var cod string
		var n int
		if rows.Scan(&cod, &n) != nil {
			continue
		}
		if n > 1 {
			// Supervisor com histórico em mais de um gerente: a troca deixaria
			// de ser equivalente.
			log.Printf("[farol:escopo] supervisor=%s serviu %d gerentes — reescrita desabilitada", cod, n)
			seguro = false
		}
		sups = append(sups, cod)
	}
	if len(sups) == 0 {
		seguro = false
	}

	hierCacheMu.Lock()
	hierCache[key] = hierCacheEntry{sups: sups, ok: seguro, at: time.Now()}
	hierCacheMu.Unlock()
	return sups, seguro
}

// reescreverGerenteParaSupervisores devolve uma cópia dos filtros com
// cod_gerente trocado pelos supervisores correspondentes. ok=false quando a
// troca não se aplica ou não é segura.
func reescreverGerenteParaSupervisores(db *sql.DB, empresaID string, fluxo fluxoCtx, filters multiFilters) (multiFilters, bool) {
	gerentes := filters["cod_gerente"]
	if len(gerentes) == 0 {
		return nil, false
	}
	// Filtro de supervisor já presente: a troca criaria duas condições sobre a
	// mesma coluna e mudaria o recorte.
	if len(filters["cod_supervisor"]) > 0 {
		return nil, false
	}
	fluxoPrefix := "fat"
	if fluxo.name == "transmitido" {
		fluxoPrefix = "trans"
	}
	sups, ok := supervisoresDoGerente(db, empresaID, fluxoPrefix, gerentes)
	if !ok {
		return nil, false
	}
	novo := multiFilters{}
	for k, v := range filters {
		if k == "cod_gerente" {
			continue
		}
		novo[k] = v
	}
	novo["cod_supervisor"] = sups
	return novo, true
}

// tentarReescritaGerente — reescreve o filtro de gerente e verifica se ALGUMA
// agg passa a servir. Devolve os filtros novos e a tabela escolhida. Se a
// reescrita não render agg, devolve ok=false e o chamador segue no scan: trocar
// o filtro sem ganho só tornaria o log confuso.
func tentarReescritaGerente(db *sql.DB, empresaID string, fluxo fluxoCtx, groupCol string,
	drillPath []drillStep, filters multiFilters) (multiFilters, string, bool) {

	novo, ok := reescreverGerenteParaSupervisores(db, empresaID, fluxo, filters)
	if !ok {
		return nil, "", false
	}
	alt, ok := pickAggForCrossFilter(db, fluxo, groupCol, drillPath, novo)
	if !ok {
		return nil, "", false
	}
	return novo, alt, true
}

// ─── Cobertura de férias ─────────────────────────────────────────────────────
//
// Enquanto um GGV/supervisor está de férias, outra pessoa passa a enxergar a
// equipe dele — somente visualização, que é tudo o que o Farol oferece a estes
// perfis. A vigência é por data: fora do intervalo a linha simplesmente deixa
// de valer, sem ninguém precisar lembrar de revogar.

type coberturaCacheEntry struct {
	cods []string
	at   time.Time
}

var (
	coberturaCacheMu sync.RWMutex
	coberturaCache   = map[string]coberturaCacheEntry{}
)

// TTL curto: uma cobertura recém-cadastrada precisa valer quase de imediato
// (alguém sai de férias hoje), e a consulta é barata — índice por usuário numa
// tabela que terá dezenas de linhas, não milhões.
const coberturaCacheTTL = 2 * time.Minute

func coberturasVigentes(db *sql.DB, empresaID, userID, col string) []string {
	if db == nil || userID == "" {
		return nil
	}
	key := empresaID + "|" + userID + "|" + col
	coberturaCacheMu.RLock()
	if e, ok := coberturaCache[key]; ok && time.Since(e.at) < coberturaCacheTTL {
		coberturaCacheMu.RUnlock()
		return e.cods
	}
	coberturaCacheMu.RUnlock()

	rows, err := db.Query(`
		SELECT cod FROM farol.cobertura_escopo
		 WHERE empresa_id = $1::uuid AND user_id = $2::uuid AND nivel = $3
		   AND CURRENT_DATE BETWEEN inicio AND fim
		 ORDER BY cod`, empresaID, userID, col)
	if err != nil {
		// Tabela ausente (migration ainda não aplicada) não pode derrubar o
		// acesso de quem não usa cobertura: segue com o escopo próprio.
		log.Printf("[farol:escopo] coberturasVigentes ERRO (seguindo sem cobertura): %v", err)
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if rows.Scan(&c) == nil && c != "" {
			out = append(out, c)
		}
	}

	coberturaCacheMu.Lock()
	coberturaCache[key] = coberturaCacheEntry{cods: out, at: time.Now()}
	coberturaCacheMu.Unlock()
	return out
}

// invalidateCoberturaCache — chamar ao cadastrar/remover cobertura.
func invalidateCoberturaCache(empresaID string) {
	coberturaCacheMu.Lock()
	for k := range coberturaCache {
		if strings.HasPrefix(k, empresaID+"|") {
			delete(coberturaCache, k)
		}
	}
	coberturaCacheMu.Unlock()
}
