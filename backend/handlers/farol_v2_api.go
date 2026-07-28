package handlers

// farol_v2_api.go — API de cards do Farol 2026 (granularidade diária).
//
// GET /api/v2/farol/cards
//   Parâmetros (nova API — intervalos):
//     view         V01 | V02 | V03
//     fluxo        faturado | transmitido       (default: faturado)
//     ref_inicio   YYYY-MM-DD                   (período principal)
//     ref_fim      YYYY-MM-DD
//     comp_inicio  YYYY-MM-DD                   (comparativo — opcional)
//     comp_fim     YYYY-MM-DD
//     drill        JSON: [{"level":"cod_fornec","value":"001","label":"MARCA X"}]
//
//   Retrocompat (UI antiga ainda enviando):
//     ref_ano + ref_mes                         → converte para ref_inicio/ref_fim
//     comp_mode = yoy | mom | ytd               → deriva comp_inicio/comp_fim
//     comp_ano + comp_mes                       → idem (mom override)
//
// GET /api/v2/farol/periodos — lista de meses (YYYY-MM) com dados disponíveis
//
// Dados lidos de views materializadas pré-agregadas (migrations 158 + 159).
// 28 MVs no total: 14 para vendas_faturadas (mv_fat_*) + 14 para
// vendas_transmitidas (mv_trans_*). A API só faz SELECT/WHERE — sem GROUP BY
// pesado, apenas SUM dos totais já pré-calculados.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

// ─── Definição de hierarquias e mapeamento de views ──────────────────────────

type hierLevel struct {
	Level     string
	NameField string
	Label     string
}

var hierarquias = map[string][]hierLevel{
	// V01: visão por indústria — Fornecedor → Gerente → Supervisor → RCA → Cliente → Produto
	"V01": {
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_gerente", NameField: "nome_gerente", Label: "Gerente"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	"V02": {
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	"V03": {
		{Level: "cod_gerente", NameField: "nome_gerente", Label: "Gerência"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	// V04: visão por força de vendas — RCA → Fornecedor → Cliente → Produto
	// Usa exclusivamente tabelas agg_*_mes (migration 162); sem MVs diárias.
	"V04": {
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	// V05: visão Supervisor → Fornecedor → RCA → Cliente → Produto
	// (mig 167) — usada pelo painel mobile como toggle "Por Fornecedor"
	// alternando com V02 ("Por RCA"). Permite o gestor pivotar a análise
	// sem trocar o escopo de supervisor.
	"V05": {
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	// V06 "Por Rede" (mig 183/185) — só valor, sem positivação. Rede é
	// identificada pelo cod_cliprinc; cnpjs diferentes podem compartilhar
	// a mesma rede (padarias/redes de mercado). Drill (2026-07-21): Rede →
	// Cliente (CNPJs filhos, ordenados por valor desc) → Fornecedor → Produto.
	// Nível Rede usa a agg (rápido, com composição/líquido); Cliente/Fornecedor/
	// Produto sob a rede leem a base escopada pelo drill.
	"V06": {
		{Level: "cod_cliprinc", NameField: "nome_cliprinc", Label: "Rede"},
		{Level: "cod_cli", NameField: "nome_cli", Label: "Cliente"},
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	// V07 "Por Departamento" (mig 184/185) — hierarquia merceológica de
	// produto. Só valor, sem positivação.
	"V07": {
		{Level: "cod_depto", NameField: "depto", Label: "Departamento"},
		{Level: "cod_sec", NameField: "secao", Label: "Seção"},
		{Level: "cod_categoria", NameField: "categoria", Label: "Categoria"},
		{Level: "cod_prod", NameField: "nome_prod", Label: "Produto"},
	},
	// V08/V09 (mig 197) — hierarquias SÓ DE ROTEAMENTO: nenhuma view da UI as
	// usa; existem para o pickAggForCrossFilter servir filtro de UF por agg em
	// vez do scan de vendas_* (13-40s). V08 cobre agrupar por gerente/sup/RCA
	// com filtro UF (gerente/sup são ancestrais do RCA → orgAncestors valida);
	// V09 cobre "Por Indústria" + filtro UF e seus drills. Só entram em jogo
	// após o backfill (gate aggUFReady).
	"V08": {
		{Level: "uf", NameField: "nome_uf", Label: "UF"},
		{Level: "cod_gerente", NameField: "nome_gerente", Label: "Gerente"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
	},
	"V09": {
		{Level: "uf", NameField: "nome_uf", Label: "UF"},
		{Level: "cod_fornec", NameField: "nome_fornec", Label: "Fornecedor"},
		{Level: "cod_gerente", NameField: "nome_gerente", Label: "Gerente"},
		{Level: "cod_supervisor", NameField: "nome_supervisor", Label: "Supervisor"},
		{Level: "cod_rca", NameField: "nome_rca", Label: "RCA"},
	},
}

// Tabelas agg_*_mes (granularidade mensal, migration 162+165).
// Usadas sempre — não há mais MVs diárias.
var aggTablesFat = map[string][]string{
	"V01": {"agg_fat_v01_l0_mes", "agg_fat_v01_l1_mes", "agg_fat_v01_l2_mes", "agg_fat_v01_l3_mes", "agg_fat_v01_l4_mes"},
	"V02": {"agg_fat_v02_l0_mes", "agg_fat_v02_l1_mes", "agg_fat_v02_l2_mes", "agg_fat_v02_l3_mes"},
	"V03": {"agg_fat_v03_l0_mes", "agg_fat_v03_l1_mes", "agg_fat_v03_l2_mes", "agg_fat_v03_l3_mes"},
	"V04": {"agg_fat_v04_l0_mes", "agg_fat_v04_l1_mes", "agg_fat_v04_l2_mes"},
	"V05": {"agg_fat_v05_l0_mes", "agg_fat_v05_l1_mes", "agg_fat_v05_l2_mes", "agg_fat_v05_l3_mes"},
	// V06: só o nível Rede (l0) usa agg; Cliente/Produto sob a rede leem a base
	// escopada (drill Rede→Cliente→Produto, 2026-07-21). As agg_fat_v06_l1/l2
	// (cliprinc,fornec[,cnpj]) continuam populadas mas não são mais lidas.
	"V06": {"agg_fat_v06_l0_mes"},
	"V07": {"agg_fat_v07_l0_mes", "agg_fat_v07_l1_mes", "agg_fat_v07_l2_mes"},
	// V08/V09 (mig 197): l0 (só UF) é o mesmo grão nas duas hierarquias →
	// tabela física única (agg_*_v08_l0_mes) referenciada pelas duas.
	"V08": {"agg_fat_v08_l0_mes", "agg_fat_v08_l1_mes", "agg_fat_v08_l2_mes", "agg_fat_v08_l3_mes"},
	"V09": {"agg_fat_v08_l0_mes", "agg_fat_v09_l1_mes", "agg_fat_v09_l2_mes", "agg_fat_v09_l3_mes", "agg_fat_v09_l4_mes"},
}

var aggTablesTrans = map[string][]string{
	"V01": {"agg_trans_v01_l0_mes", "agg_trans_v01_l1_mes", "agg_trans_v01_l2_mes", "agg_trans_v01_l3_mes", "agg_trans_v01_l4_mes"},
	"V02": {"agg_trans_v02_l0_mes", "agg_trans_v02_l1_mes", "agg_trans_v02_l2_mes", "agg_trans_v02_l3_mes"},
	"V03": {"agg_trans_v03_l0_mes", "agg_trans_v03_l1_mes", "agg_trans_v03_l2_mes", "agg_trans_v03_l3_mes"},
	"V04": {"agg_trans_v04_l0_mes", "agg_trans_v04_l1_mes", "agg_trans_v04_l2_mes"},
	"V05": {"agg_trans_v05_l0_mes", "agg_trans_v05_l1_mes", "agg_trans_v05_l2_mes", "agg_trans_v05_l3_mes"},
	"V06": {"agg_trans_v06_l0_mes"},
	"V07": {"agg_trans_v07_l0_mes", "agg_trans_v07_l1_mes", "agg_trans_v07_l2_mes"},
	"V08": {"agg_trans_v08_l0_mes", "agg_trans_v08_l1_mes", "agg_trans_v08_l2_mes", "agg_trans_v08_l3_mes"},
	"V09": {"agg_trans_v08_l0_mes", "agg_trans_v09_l1_mes", "agg_trans_v09_l2_mes", "agg_trans_v09_l3_mes", "agg_trans_v09_l4_mes"},
}

// fluxoCtx — após mig 165 não há mais MVs diárias. tableName/dateCol seguem
// usados pelos handlers de detalhe (consulta direta em vendas_*).
type fluxoCtx struct {
	name      string
	tableName string
	dateCol   string
	// eventoFilter — fragmento SQL adicional (ex.: `AND v.evento IN (...)`) usado
	// pelos fluxos CCD (Cancelado/Devolvido, Cortado) que leem vendas_ccd. Vazio
	// para faturado/transmitido. Valores são literais fixos (sem injeção).
	eventoFilter string
	// isCCD — fluxo lê vendas_ccd (sem agg, sem positivação). Força scan da base.
	isCCD bool
}

func resolveFluxo(s string) fluxoCtx {
	switch {
	case strings.EqualFold(s, "transmitido") || strings.EqualFold(s, "trans"):
		return fluxoCtx{name: "transmitido", tableName: "vendas_transmitidas", dateCol: "data_transmissao"}
	case strings.EqualFold(s, "cancdev"):
		// Cancelado + Devolvido (lado faturado) — mig 182/189.
		return fluxoCtx{name: "cancdev", tableName: "vendas_ccd", dateCol: "data_evento",
			eventoFilter: "AND v.evento IN ('CANCELADO','DEVOLVIDO')", isCCD: true}
	case strings.EqualFold(s, "cortado"):
		// Cortado (lado transmitido — venda perdida).
		return fluxoCtx{name: "cortado", tableName: "vendas_ccd", dateCol: "data_evento",
			eventoFilter: "AND v.evento = 'CORTADO'", isCCD: true}
	default:
		return fluxoCtx{name: "faturado", tableName: "vendas_faturadas", dateCol: "data_faturamento"}
	}
}

// getAggTableName retorna a tabela agg_*_mes para (fluxo, view, drillIdx), ou ("", false).
func getAggTableName(fluxo fluxoCtx, view string, drillIdx int) (string, bool) {
	if fluxo.isCCD {
		return "", false // CCD não tem tabelas agg → sempre scan da base
	}
	tables := aggTablesFat
	if fluxo.name == "transmitido" {
		tables = aggTablesTrans
	}
	if levels, ok := tables[view]; ok && drillIdx >= 0 && drillIdx < len(levels) {
		return "farol." + levels[drillIdx], true
	}
	return "", false
}

// isCompleteMonthRange reporta se [start, end] cobre apenas meses calendários completos.
func isCompleteMonthRange(start, end time.Time) bool {
	if start.IsZero() || end.IsZero() {
		return false
	}
	if start.Day() != 1 {
		return false
	}
	lastDay := time.Date(end.Year(), end.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	return end.Day() == lastDay.Day()
}

// ym converte uma data no inteiro ano*100+mes usado nas cláusulas WHERE das agg_*_mes.
func ym(t time.Time) int { return t.Year()*100 + int(t.Month()) }

// buildMesCond monta `(v.ano * 100 + v.mes) BETWEEN $N AND $M` para tabelas agg_*_mes.
func buildMesCond(ymStart, ymEnd int, args *[]any) string {
	*args = append(*args, ymStart, ymEnd)
	n := len(*args)
	return fmt.Sprintf("(v.ano * 100 + v.mes) BETWEEN $%d AND $%d", n-1, n)
}

// ─── Tipos ────────────────────────────────────────────────────────────────────

type drillStep struct {
	Level string `json:"level"`
	Value string `json:"value"`
	Label string `json:"label"`
}

// composicao — valores por categoria excluída do Líquido (mig 189/190). O
// front usa como delta dos botões "Incluir X": valor exibido = liquido (=
// ValorAtual) + Σ(deltas ligados). Ver spec-venda-liquida-composicao.md.
type composicao struct {
	Bonif   float64 `json:"bonif"`
	Transf  float64 `json:"transf"`
	Remessa float64 `json:"remessa"`
	Devol   float64 `json:"devol"`
	Cancel  float64 `json:"cancel"`
}

type cardItem struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Level       string  `json:"level"`
	LevelLabel  string  `json:"level_label"`
	ValorAtual  float64 `json:"valor_atual"`
	ValorAnt    float64 `json:"valor_ant"`
	Pct         float64 `json:"pct"`
	Cor         string  `json:"cor"` // cor do KPI Venda
	Faturado    float64 `json:"faturado"`
	Transmitido float64 `json:"transmitido"`
	Plucro      float64 `json:"plucro"`
	PlucroAnt   float64 `json:"plucro_ant"`
	// Positivação — atual e comparativo + cor
	Positivados    int     `json:"positivados"`
	BaseCli        int     `json:"base_cli"`
	PositPct       float64 `json:"positpct"`
	PositivadosAnt int     `json:"positivados_ant"`
	BaseCliAnt     int     `json:"base_cli_ant"`
	PositPctAnt    float64 `json:"positpct_ant"`
	PositCor       string  `json:"posit_cor"`
	// Mix — atual e comparativo + cor
	Mix    float64 `json:"mix"`
	MixAnt float64 `json:"mix_ant"`
	MixCor string  `json:"mix_cor"`
	// Mix Total — SKUs distintos do fornecedor no período (universo)
	MixTotal    int `json:"mix_total"`
	MixTotalAnt int `json:"mix_total_ant"`
	// Venda líquida (faturado): ValorAtual/ValorAnt já são o Líquido; Comp/CompAnt
	// trazem as categorias para os botões "Incluir X" somarem no front.
	Comp    composicao `json:"comp"`
	CompAnt composicao `json:"comp_ant"`
}

type kpiSummary struct {
	// Venda
	TotalAtual       float64 `json:"total_atual"`
	TotalAnt         float64 `json:"total_ant"`
	TotalPct         float64 `json:"total_pct"`
	TotalCor         string  `json:"total_cor"`
	TotalFaturado    float64 `json:"total_faturado"`
	TotalTransmitido float64 `json:"total_transmitido"`
	TotalPlucro      float64 `json:"total_plucro"`
	TotalPlucroAnt   float64 `json:"total_plucro_ant"`
	// Positivação — atual + comparativo + cor
	TotalPositivados    int     `json:"total_positivados"`
	TotalBaseCli        int     `json:"total_base_cli"`
	TotalPositPct       float64 `json:"total_positpct"`
	TotalPositivadosAnt int     `json:"total_positivados_ant"`
	TotalBaseCliAnt     int     `json:"total_base_cli_ant"`
	TotalPositPctAnt    float64 `json:"total_positpct_ant"`
	TotalPositCor       string  `json:"total_posit_cor"`
	// Mix
	AvgMix    float64 `json:"avg_mix"`
	AvgMixAnt float64 `json:"avg_mix_ant"`
	MixCor    string  `json:"mix_cor"`
	// Mix Total agregado (universo de SKUs distintos do nível atual)
	TotalMixTotal    int `json:"total_mix_total"`
	TotalMixTotalAnt int `json:"total_mix_total_ant"`
	Verdes           int `json:"verdes"`
	Vermelhos        int `json:"vermelhos"`
	// Composição agregada (faturado) — TotalAtual/TotalAnt já são o Líquido; o
	// front soma estes deltas quando um toggle "Incluir X" está ligado.
	Comp    composicao `json:"comp"`
	CompAnt composicao `json:"comp_ant"`
}

type periodoInfo struct {
	Fluxo      string `json:"fluxo"`       // faturado | transmitido
	RefInicio  string `json:"ref_inicio"`  // YYYY-MM-DD
	RefFim     string `json:"ref_fim"`     // YYYY-MM-DD
	CompInicio string `json:"comp_inicio"` // YYYY-MM-DD (vazio se sem comparativo)
	CompFim    string `json:"comp_fim"`    // YYYY-MM-DD
	Label      string `json:"label"`
	CurLabel   string `json:"cur_label"`
	AntLabel   string `json:"ant_label"`
	// Retrocompat — preenchidos quando inferidos a partir de mês inteiro
	RefAno   int    `json:"ref_ano,omitempty"`
	RefMes   int    `json:"ref_mes,omitempty"`
	CompMode string `json:"comp_mode,omitempty"`
	CompAno  int    `json:"comp_ano,omitempty"`
	CompMes  int    `json:"comp_mes,omitempty"`
}

type cardsResponse struct {
	Cards          []cardItem  `json:"cards"`
	KPI            kpiSummary  `json:"kpi"`
	Periodo        periodoInfo `json:"periodo"`
	Periodos       []string    `json:"periodos"`
	View           string      `json:"view"`
	DrillPath      []drillStep `json:"drill_path"`
	NextLevel      string      `json:"next_level"`
	NextLevelLabel string      `json:"next_level_label"`
	Diag           cardsDiag   `json:"diag"`
}

// cardsDiag — como o recorte foi servido. Existe porque uma lista vazia era
// ambígua na tela: podia ser "recorte sem venda" ou "a consulta falhou". No
// incidente de 27/07/2026 (memória compartilhada esgotada) o painel mostrou
// 0 cards e isso se lê como "não vendeu nada" — decisão do gestor foi avisar
// explicitamente em vez de criar mais tabelas agregadas para cada combinação.
type cardsDiag struct {
	// Lento — não havia agregação para esta combinação de filtros; a consulta
	// varreu vendas_* (dezenas de segundos).
	Lento bool `json:"lento"`
	// Falhou — a consulta ERROU. Os números exibidos estão ausentes ou
	// incompletos; NÃO interpretar como ausência de venda.
	Falhou bool `json:"falhou"`
	// Combinacao — filtros que levaram ao caminho lento (ex: "cod_supervisor,uf").
	Combinacao string `json:"combinacao,omitempty"`
	// MS — duração da montagem do recorte, para a UI dimensionar o aviso.
	MS int64 `json:"ms"`
}

// ─── Parsing de datas/períodos a partir da URL ───────────────────────────────

// parseDateISO aceita YYYY-MM-DD. Retorna zero se vazio/inválido.
func parseDateISO(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// mesInteiro retorna (inicio, fim) cobrindo todo um mês (1º dia até último dia).
func mesInteiro(ano, mes int) (time.Time, time.Time) {
	inicio := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
	fim := inicio.AddDate(0, 1, -1) // último dia do mês
	return inicio, fim
}

// inferLastMonth descobre o último mês com dados (ATUAL) para a empresa.
// Lê das agg_*_v01_l0_mes (ano/mes vêm da DATA real de cada venda, já consolidada),
// NÃO de vendas_import_jobs — cujo ano/mes é a "competência" derivada do nome do
// arquivo e pode estar errada (parser cai no mês atual quando não reconhece o nome).
// Retorna (ano, mes) ou (0, 0) se vazio.
func inferLastMonth(db *sql.DB, empresaID string) (int, int) {
	var ano, mes int
	_ = db.QueryRow(`
		SELECT ano, mes FROM (
			SELECT ano, mes FROM farol.agg_fat_v01_l0_mes   WHERE empresa_id=$1
			UNION
			SELECT ano, mes FROM farol.agg_trans_v01_l0_mes WHERE empresa_id=$1
		) x ORDER BY ano DESC, mes DESC LIMIT 1
	`, empresaID).Scan(&ano, &mes)
	return ano, mes
}

// inferLastDay — último DIA importado (max data_faturamento/transmissao) da
// empresa. Usado pelo "Ano × Ano" (ytd) como fim do acumulado do ano corrente
// ("até hoje" = até o último dia com dado). Zero se não houver dados.
func inferLastDay(db *sql.DB, empresaID string) time.Time {
	var d sql.NullTime
	_ = db.QueryRow(`
		SELECT MAX(d) FROM (
			SELECT MAX(data_faturamento) AS d FROM vendas_faturadas   WHERE empresa_id=$1
			UNION ALL
			SELECT MAX(data_transmissao) AS d FROM vendas_transmitidas WHERE empresa_id=$1
		) x
	`, empresaID).Scan(&d)
	if d.Valid {
		return d.Time.UTC()
	}
	return time.Time{}
}

// deriveCompRange calcula um intervalo comparativo a partir de (refInicio, refFim)
// e do compMode (yoy | mom | ytd | mtd). Retorna (zero, zero) se mode for desconhecido.
//
//	yoy → subtrai 1 ano nas duas pontas
//	mom → range contíguo imediatamente anterior (mesma quantidade de dias)
//	ytd → 1º jan do ano anterior até a mesma data (refFim com ano-1)
//	mtd → 1º dia do mês anterior até o último dia daquele mês
func deriveCompRange(refInicio, refFim time.Time, mode string) (time.Time, time.Time) {
	switch strings.ToLower(mode) {
	case "yoy":
		return refInicio.AddDate(-1, 0, 0), refFim.AddDate(-1, 0, 0)
	case "mom":
		diasRange := int(refFim.Sub(refInicio).Hours()/24) + 1
		fim := refInicio.AddDate(0, 0, -1)
		ini := fim.AddDate(0, 0, -(diasRange - 1))
		return ini, fim
	case "ytd":
		// Acumulado do ano corrente × ANO ANTERIOR INTEIRO (01/jan a 31/dez).
		ini := time.Date(refFim.Year()-1, 1, 1, 0, 0, 0, 0, time.UTC)
		fim := time.Date(refFim.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC)
		return ini, fim
	case "mtd":
		// Mês atual (01/dia até hoje) × MÊS ANTERIOR INTEIRO
		ini := time.Date(refFim.Year(), refFim.Month()-1, 1, 0, 0, 0, 0, time.UTC)
		fim := ini.AddDate(0, 1, -1) // último dia do mês anterior
		return ini, fim
	}
	return time.Time{}, time.Time{}
}

// resolvePeriods extrai o intervalo principal e o comparativo da query,
// honrando tanto o contrato novo (datas) quanto o antigo (ano/mes/comp_mode).
// Também retorna metadados pra preencher periodoInfo (ano/mes/compMode quando aplicáveis).
type periodResolution struct {
	RefInicio  time.Time
	RefFim     time.Time
	CompInicio time.Time
	CompFim    time.Time
	// Metadados pra retrocompat no payload
	RefAno   int
	RefMes   int
	CompMode string
	CompAno  int
	CompMes  int
}

func resolvePeriods(db *sql.DB, empresaID string, q map[string][]string) periodResolution {
	get := func(k string) string {
		if vs, ok := q[k]; ok && len(vs) > 0 {
			return strings.TrimSpace(vs[0])
		}
		return ""
	}
	res := periodResolution{}

	// 1) Período principal — preferência: datas explícitas; fallback ano/mes; senão último mês
	refInicio := parseDateISO(get("ref_inicio"))
	refFim := parseDateISO(get("ref_fim"))
	if refInicio.IsZero() || refFim.IsZero() {
		refAno, _ := strconv.Atoi(get("ref_ano"))
		refMes, _ := strconv.Atoi(get("ref_mes"))
		if refAno == 0 || refMes == 0 {
			refAno, refMes = inferLastMonth(db, empresaID)
		}
		if refAno > 0 && refMes > 0 {
			refInicio, refFim = mesInteiro(refAno, refMes)
			res.RefAno = refAno
			res.RefMes = refMes
		}
	} else {
		// Se as datas cobrem um mês inteiro, preencher ano/mes pra retrocompat.
		if refInicio.Day() == 1 && refFim.AddDate(0, 0, 1).Day() == 1 &&
			refInicio.Year() == refFim.Year() && refInicio.Month() == refFim.Month() {
			res.RefAno = refInicio.Year()
			res.RefMes = int(refInicio.Month())
		}
	}
	res.RefInicio = refInicio
	res.RefFim = refFim

	// 2) Comparativo — preferência: datas explícitas; depois comp_mode; depois nada
	compInicio := parseDateISO(get("comp_inicio"))
	compFim := parseDateISO(get("comp_fim"))
	if (compInicio.IsZero() || compFim.IsZero()) && !refInicio.IsZero() && !refFim.IsZero() {
		mode := strings.ToLower(get("comp_mode"))
		// Suporte ao comp_ano/comp_mes (override mom) — produz um mês exato
		compAno, _ := strconv.Atoi(get("comp_ano"))
		compMes, _ := strconv.Atoi(get("comp_mes"))
		if compAno > 0 && compMes > 0 {
			compInicio, compFim = mesInteiro(compAno, compMes)
			res.CompMode = "mom"
			res.CompAno = compAno
			res.CompMes = compMes
		} else if mode != "" {
			// ytd ("Ano × Ano"): acumulado do ano corrente = 01/jan → ÚLTIMO DIA
			// IMPORTADO ("até hoje" = último dado). Comparativo = ANO ANTERIOR
			// INTEIRO (deriveCompRange = 01/jan–31/dez).
			if mode == "ytd" {
				last := inferLastDay(db, empresaID)
				if last.IsZero() {
					last = refFim // fallback: fim do mês de ref
				}
				refInicio = time.Date(last.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
				refFim = time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC)
				res.RefInicio = refInicio
				res.RefFim = refFim
			}

			// mtd: mês atual (01/dia -> último dado) vs mês anterior inteiro
			if mode == "mtd" {
				last := inferLastDay(db, empresaID)
				if last.IsZero() {
					last = refFim // fallback: fim do mês de ref
				}
				refInicio = time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC)
				refFim = time.Date(last.Year(), last.Month(), last.Day(), 0, 0, 0, 0, time.UTC)
				res.RefInicio = refInicio
				res.RefFim = refFim
			}
			compInicio, compFim = deriveCompRange(refInicio, refFim, mode)
			res.CompMode = mode
		}
	}
	res.CompInicio = compInicio
	res.CompFim = compFim
	return res
}

// ─── FarolV2CardsHandler — GET /api/v2/farol/cards ──────────────────────────

func FarolV2CardsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		q := r.URL.Query()

		view := strings.ToUpper(q.Get("view"))
		if view == "" {
			view = "V01"
		}
		hier, ok := hierarquias[view]
		if !ok {
			http.Error(w, `{"error":"view inválida — use V01, V02, V03 ou V04"}`, http.StatusBadRequest)
			return
		}

		fluxo := resolveFluxo(q.Get("fluxo"))

		var drillPath []drillStep
		if drillJSON := q.Get("drill"); drillJSON != "" {
			_ = json.Unmarshal([]byte(drillJSON), &drillPath)
		}
		drillIdx := len(drillPath)

		if drillIdx >= len(hier) {
			json.NewEncoder(w).Encode(cardsResponse{
				Cards: []cardItem{}, DrillPath: drillPath, View: view,
			})
			return
		}
		currentLevel := hier[drillIdx]

		pr := resolvePeriods(db, spCtx.EmpresaID, q)
		if pr.RefInicio.IsZero() || pr.RefFim.IsZero() {
			// Sem dados — devolve resposta vazia em vez de erro pra UI poder render placeholder.
			json.NewEncoder(w).Encode(cardsResponse{
				Cards: []cardItem{}, View: view, DrillPath: drillPath,
				Periodo: periodoInfo{Fluxo: fluxo.name},
			})
			return
		}

		filters := parseMultiFilters(q)
		// tipo_venda só existe no fluxo faturado. No transmitido, ignora
		// silenciosamente (a coluna não existe em vendas_transmitidas → scan
		// quebraria). Ver I/O matrix do spec: "Filtro no transmitido → ignorado".
		if fluxo.name != "faturado" {
			delete(filters, "tipo_venda")
		}
		cards, diag := fetchCards(db, spCtx.EmpresaID, fluxo, view, pr, drillIdx, currentLevel, drillPath, filters)
		kpi := computeKPI(cards, fluxo.name, currentLevel.Level == "cod_fornec")
		// Totalizador = distinct do recorte (drill+filtros) em todos os níveis com
		// positivação (não em cliente/produto, onde é escondida). Garante que, ao
		// abrir um fornecedor, o totalizador = o nº que aparecia no card dele.
		if currentLevel.Level != "cod_prod" && currentLevel.Level != "cod_cli" &&
			leafServesPositivados(fluxo, view, currentLevel.Level, drillPath, filters) {
			fixOverlappingBaseKPI(db, &kpi, fluxo, view, spCtx.EmpresaID, pr, drillPath, filters)
		}
		// O "de Y" (mix_total) do totalizador foi omitido na tela a pedido do gestor;
		// por isso NÃO recalculamos o universo aqui (queries COUNT(DISTINCT) caras).
		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		curLabel, antLabel, plabel := buildPeriodoLabels(pr)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].ValorAtual > cards[j].ValorAtual
		})

		json.NewEncoder(w).Encode(cardsResponse{
			Cards: cards,
			KPI:   kpi,
			Periodo: periodoInfo{
				Fluxo:      fluxo.name,
				RefInicio:  pr.RefInicio.Format("2006-01-02"),
				RefFim:     pr.RefFim.Format("2006-01-02"),
				CompInicio: fmtDateOrEmpty(pr.CompInicio),
				CompFim:    fmtDateOrEmpty(pr.CompFim),
				Label:      plabel,
				CurLabel:   curLabel,
				AntLabel:   antLabel,
				RefAno:     pr.RefAno,
				RefMes:     pr.RefMes,
				CompMode:   pr.CompMode,
				CompAno:    pr.CompAno,
				CompMes:    pr.CompMes,
			},
			Periodos:       periodos,
			View:           view,
			DrillPath:      drillPath,
			NextLevel:      currentLevel.Level,
			NextLevelLabel: currentLevel.Label,
			Diag:           diag,
		})
	}
}

func fmtDateOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// ─── safeColName ─────────────────────────────────────────────────────────────

var allowedCols = map[string]bool{
	"cod_fornec": true, "nome_fornec": true,
	"cod_gerente": true, "nome_gerente": true,
	"cod_supervisor": true, "nome_supervisor": true,
	"cod_rca": true, "nome_rca": true,
	"cod_cli": true, "nome_cli": true,
	"cod_prod": true, "nome_prod": true,
	"empresa": true, "uf": true,
	// V06 (mig 183) — dimensão Rede + reuso de cod_fornec/cod_cli
	"cod_cliprinc": true, "nome_cliprinc": true,
	// V07 (mig 184) — hierarquia merceológica do produto
	"cod_depto": true, "depto": true,
	"cod_sec": true, "secao": true,
	"cod_categoria": true, "categoria": true,
	// tipo_venda (mig 187) — filtro CRUZADO só do fluxo faturado. Existe apenas
	// em vendas_faturadas; nenhuma tabela agg tem a coluna, então aggServesFilters
	// retorna false e fetchCards cai em queryAggregatedVendas (scan da base), que
	// calcula todos os indicadores corretamente. Ver Spec Change Log 2026-07-21.
	"tipo_venda": true,
}

func safeColName(col string) string {
	if allowedCols[col] {
		return col
	}
	return "cod_fornec"
}

// ─── Builders de condição SQL ─────────────────────────────────────────────────

// buildRangeCond monta `v.<dateCol> BETWEEN $X AND $Y`. Apenda args.
func buildRangeCond(dateCol string, inicio, fim time.Time, args *[]any) string {
	*args = append(*args, inicio.Format("2006-01-02"), fim.Format("2006-01-02"))
	n := len(*args)
	return fmt.Sprintf("v.%s BETWEEN $%d::date AND $%d::date", dateCol, n-1, n)
}

// buildDrillCond monta os filtros de drill-path (AND v.col=$N ...).
func buildDrillCond(drillPath []drillStep, args *[]any) string {
	parts := make([]string, 0, len(drillPath))
	for _, d := range drillPath {
		col := safeColName(d.Level)
		*args = append(*args, d.Value)
		parts = append(parts, fmt.Sprintf("AND v.%s=$%d", col, len(*args)))
	}
	return strings.Join(parts, " ")
}

// multiFilters representa filtros multi-select extraídos da query string.
// Cada chave é um cod_* (allowed col), cada valor é a lista de seleções.
type multiFilters map[string][]string

// names — colunas filtradas, ordenadas, p/ log/diagnóstico (ex: "cod_fornec,uf").
func (mf multiFilters) names() string {
	if len(mf) == 0 {
		return "-"
	}
	cols := make([]string, 0, len(mf))
	for c := range mf {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return strings.Join(cols, ",")
}

// parseMultiFilters extrai dos URL params os filtros multi-select.
// Aceita:
//
//	?cod_fornec=F01,F02  ?cod_supervisor=S01  ?cod_rca=R01,R02
//	?cod_gerente=...     ?cod_cli=...         ?uf=SP,RJ  ?empresa=NORDESTE
func parseMultiFilters(q map[string][]string) multiFilters {
	mf := multiFilters{}
	// tipo_venda: filtro cruzado só do fluxo faturado. O chamador remove-o quando
	// fluxo=transmitido (a coluna não existe em vendas_transmitidas).
	cols := []string{"cod_fornec", "cod_gerente", "cod_supervisor", "cod_rca", "cod_cli", "uf", "empresa", "tipo_venda"}
	for _, c := range cols {
		raw := ""
		if vs, ok := q[c]; ok && len(vs) > 0 {
			raw = vs[0]
		}
		if raw == "" {
			continue
		}
		// permite múltiplos valores separados por vírgula
		parts := strings.Split(raw, ",")
		vals := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				vals = append(vals, p)
			}
		}
		if len(vals) > 0 {
			mf[c] = vals
		}
	}
	return mf
}

// buildMultiFilterCond — gera `AND v.col = ANY($N::text[])` por dimensão.
// É aditivo ao drill: filtros multi e drill são aplicados juntos (AND).
func buildMultiFilterCond(mf multiFilters, args *[]any) string {
	if len(mf) == 0 {
		return ""
	}
	parts := []string{}
	for col, vals := range mf {
		col = safeColName(col)
		*args = append(*args, pq.Array(vals))
		parts = append(parts, fmt.Sprintf("AND v.%s = ANY($%d::text[])", col, len(*args)))
	}
	return strings.Join(parts, " ")
}

// ─── aggResult ────────────────────────────────────────────────────────────────

type aggResult struct {
	label       string
	valor       float64
	plucro      float64
	baseCli     int
	positivados int
	mix         float64
	mixTotal    int // universo de SKUs distintos (P1: materializado em agg.mix_total)
	// Composição da venda líquida (mig 189/190) — só faturado via agg. Nos demais
	// caminhos (scan da base, produto) liquido recebe = valor e os deltas ficam 0.
	liquido float64 // Σ tipos reais − devol − cancel
	bonif   float64 // pv_bonif
	transf  float64 // pv_transf
	remessa float64 // pv_remessa
	devol   float64 // pv_devol
	cancel  float64 // pv_cancel
}

// ─── queryAggregated ─────────────────────────────────────────────────────────
// Lê a MV pré-agregada para um intervalo de datas e um nível de hierarquia.

func queryAggregated(db *sql.DB, viewName, groupCol, nameCol, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.%s                           AS key,
  MAX(v.%s)                      AS label,
  SUM(v.pvenda)                  AS valor,
  COALESCE(SUM(v.plucro), 0)     AS plucro,
  ROUND(AVG(v.base_cli))::int    AS base_cli,
  ROUND(AVG(v.positivados))::int AS positivados,
  AVG(v.mix)                     AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.%s != ''
AND %s %s
GROUP BY v.%s`,
		groupCol, nameCol,
		viewName,
		groupCol, rangeCond, drillCond,
		groupCol,
	)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryAggregated nível=%s ERRO em %v: %v", groupCol, time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			r.liquido = r.valor // sem composição neste caminho → líquido = bruto
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryAggregated view=%s nível=%s → %d grupos em %v",
		viewName, groupCol, len(result), time.Since(t0))
	return result
}

// queryAnteriorTotals — comparativo completo (pvenda + positivação + mix + base).
// Mesma shape de queryAggregated pra permitir cor por KPI no fetchCards.
func queryAnteriorTotals(db *sql.DB, viewName, groupCol, nameCol, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.%s                           AS key,
  MAX(v.%s)                      AS label,
  SUM(v.pvenda)                  AS valor,
  COALESCE(SUM(v.plucro), 0)     AS plucro,
  ROUND(AVG(v.base_cli))::int    AS base_cli,
  ROUND(AVG(v.positivados))::int AS positivados,
  AVG(v.mix)                     AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.%s != '' AND %s %s
GROUP BY v.%s`, groupCol, nameCol, viewName, groupCol, rangeCond, drillCond, groupCol)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryAnteriorTotals nível=%s ERRO em %v: %v", groupCol, time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			r.liquido = r.valor // sem composição neste caminho → líquido = bruto
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryAnteriorTotals view=%s nível=%s → %d grupos em %v",
		viewName, groupCol, len(result), time.Since(t0))
	return result
}

// queryAggregatedMes — lê tabelas agg_*_mes (granularidade mensal, migration 162).
// Usado quando o range de datas é meses completos; substitui queryAggregated/queryAnteriorTotals.
// pvenda/plucro são somados; base_cli/positivados/mix são AVG (valor típico por mês).
func queryAggregatedMes(db *sql.DB, viewName, groupCol, nameCol, mesCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	// mix_total só existe nas tabelas da migration 175 (V01 l0-l3, V02/V03/V05
	// l0-l2). Nos níveis-folha mais profundos (cliente em V01 l4, etc.) e no V04
	// a coluna NÃO existe — referenciá-la quebra a query (e some o cliente no
	// drill). Quando ausente, devolve 0.
	shortName := strings.TrimPrefix(viewName, "farol.")
	mixTotalExpr := "0"
	if aggHasMixTotal[shortName] {
		mixTotalExpr = "COALESCE(MAX(v.mix_total), 0)"
	}
	// base_cli/positivados/mix só existem em V01-V05. V06 (Por Rede) e V07 (Por
	// Departamento), introduzidas na Fase 2 do adequação novo layout, guardam
	// apenas pvenda/plucro/qt — sem métricas de positivação. Sem esse guard,
	// SELECT v.base_cli quebra a query com "column does not exist".
	baseCliExpr := "ROUND(AVG(v.base_cli))::int"
	positivadosExpr := "ROUND(AVG(v.positivados))::int"
	mixExpr := "AVG(v.mix)"
	// V08/V09 (mig 197): com filtro multi-UF há N linhas por mês (uma por UF) e
	// AVG dividiria por N — com BA+GO mostraria METADE dos positivados. Como
	// cada cliente pertence a UMA UF, somar entre UFs é exato; divide-se só
	// pelo nº de meses (média mensal, mesma semântica do AVG nas demais aggs).
	// base_cli fica no AVG: é a carteira do escopo org, constante entre UFs.
	if strings.Contains(shortName, "_v08_") || strings.Contains(shortName, "_v09_") {
		positivadosExpr = "ROUND(SUM(v.positivados)::numeric / NULLIF(COUNT(DISTINCT v.ano*100+v.mes),0))::int"
	}
	if aggWithoutPositivacao(shortName) {
		baseCliExpr = "0::int"
		positivadosExpr = "0::int"
		mixExpr = "0::float"
	}
	// Composição da venda líquida (mig 189/190) só existe nas agg_fat_*. Para
	// agg_trans_* (transmitido) as colunas não existem → devolve 0 (o transmitido
	// exibe o valor bruto, sem toggles). liquido default = valor (ajustado no scan
	// quando faturado, abaixo).
	liquidoExpr, bonifExpr, transfExpr := "0::numeric", "0::numeric", "0::numeric"
	remessaExpr, devolExpr, cancelExpr := "0::numeric", "0::numeric", "0::numeric"
	faturadoAgg := strings.HasPrefix(shortName, "agg_fat_")
	if faturadoAgg {
		liquidoExpr = "COALESCE(SUM(v.liquido), 0)"
		bonifExpr = "COALESCE(SUM(v.pv_bonif), 0)"
		transfExpr = "COALESCE(SUM(v.pv_transf), 0)"
		remessaExpr = "COALESCE(SUM(v.pv_remessa), 0)"
		devolExpr = "COALESCE(SUM(v.pv_devol), 0)"
		cancelExpr = "COALESCE(SUM(v.pv_cancel), 0)"
	}
	q := fmt.Sprintf(`
SELECT
  v.%s                           AS key,
  MAX(v.%s)                      AS label,
  SUM(v.pvenda)                  AS valor,
  COALESCE(SUM(v.plucro), 0)     AS plucro,
  %s                             AS base_cli,
  %s                             AS positivados,
  %s                             AS mix,
  %s                             AS mix_total,
  %s AS liquido, %s AS bonif, %s AS transf, %s AS remessa, %s AS devol, %s AS cancel
FROM %s v
WHERE v.empresa_id=$1 AND v.%s != ''
AND %s %s
GROUP BY v.%s`,
		groupCol, nameCol,
		baseCliExpr, positivadosExpr, mixExpr,
		mixTotalExpr,
		liquidoExpr, bonifExpr, transfExpr, remessaExpr, devolExpr, cancelExpr,
		viewName,
		groupCol, mesCond, drillCond,
		groupCol,
	)
	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:agg] queryAggregatedMes view=%s nível=%s ERRO em %v: %v", viewName, groupCol, time.Since(t0), err)
		return nil
	}
	defer rows.Close()
	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro, &r.baseCli, &r.positivados, &r.mix, &r.mixTotal,
			&r.liquido, &r.bonif, &r.transf, &r.remessa, &r.devol, &r.cancel); err == nil {
			if !faturadoAgg {
				r.liquido = r.valor // transmitido: sem líquido, exibe bruto
			}
			result[key] = r
		}
	}
	log.Printf("[farol:agg] queryAggregatedMes view=%s nível=%s → %d grupos em %v",
		viewName, groupCol, len(result), time.Since(t0))
	return result
}

// queryProdutos / queryProdutosAnterior — nível folha (cod_prod), sem MV pré-agregada.
// Lê direto da tabela base do fluxo, escopado por drill (volume pequeno).

func queryProdutos(db *sql.DB, fluxo fluxoCtx, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.cod_prod                AS key,
  MAX(v.nome_prod)          AS label,
  SUM(v.pvenda)             AS valor,
  COALESCE(SUM(v.plucro),0) AS plucro,
  0::int                    AS base_cli,
  0::int                    AS positivados,
  0::float                  AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.cod_prod != '' AND %s %s %s
GROUP BY v.cod_prod`, fluxo.tableName, rangeCond, drillCond, fluxo.eventoFilter)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryProdutos ERRO em %v: %v", time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			r.liquido = r.valor // sem composição neste caminho → líquido = bruto
			result[key] = r
		}
	}
	log.Printf("[farol:view] queryProdutos (%s) → %d produtos em %v", fluxo.tableName, len(result), time.Since(t0))
	return result
}

func queryProdutosAnterior(db *sql.DB, fluxo fluxoCtx, rangeCond, drillCond string, args []any) map[string]aggResult {
	t0 := time.Now()
	q := fmt.Sprintf(`
SELECT
  v.cod_prod                AS key,
  MAX(v.nome_prod)          AS label,
  SUM(v.pvenda)             AS valor,
  COALESCE(SUM(v.plucro),0) AS plucro,
  0::int                    AS base_cli,
  0::int                    AS positivados,
  0::float                  AS mix
FROM %s v
WHERE v.empresa_id=$1 AND v.cod_prod != '' AND %s %s %s
GROUP BY v.cod_prod`, fluxo.tableName, rangeCond, drillCond, fluxo.eventoFilter)

	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:view] queryProdutosAnterior ERRO em %v: %v", time.Since(t0), err)
		return nil
	}
	defer rows.Close()

	result := make(map[string]aggResult)
	for rows.Next() {
		var key string
		var r aggResult
		if err := rows.Scan(&key, &r.label, &r.valor, &r.plucro,
			&r.baseCli, &r.positivados, &r.mix); err == nil {
			r.liquido = r.valor // sem composição neste caminho → líquido = bruto
			result[key] = r
		}
	}
	return result
}

// colsInAggTable — colunas de dimensão presentes na tabela agg_*_lY_mes de
// (view, drillIdx). São os níveis da hierarquia da view de 0 até drillIdx.
// uf/empresa NUNCA estão em tabela agg (só em dims_mes e vendas_*).
func colsInAggTable(view string, drillIdx int) map[string]bool {
	cols := map[string]bool{}
	hier, ok := hierarquias[view]
	if !ok {
		return cols
	}
	for i := 0; i <= drillIdx && i < len(hier); i++ {
		cols[hier[i].Level] = true
	}
	return cols
}

// aggServesFilters — true se TODOS os filtros ativos podem ser aplicados na
// tabela agg de (view, drillIdx). Se algum filtro referencia coluna ausente
// (ex: cod_fornec em "Por Gerência", ou uf/empresa em qualquer view), a tabela
// agg desnormalizada não tem a coluna → query quebraria. Nesse caso fetchCards
// cai para vendas_* (queryAggregatedVendas).
func aggServesFilters(view string, drillIdx int, filters multiFilters) bool {
	if len(filters) == 0 {
		return true
	}
	cols := colsInAggTable(view, drillIdx)
	for col := range filters {
		if !cols[col] {
			return false
		}
	}
	return true
}

// orgAncestors — para cada nível organizacional, os níveis MAIS ALTOS que ele
// determina univocamente (um RCA pertence a um supervisor; um supervisor a um
// gerente; etc). Usado por pickAggForCrossFilter para garantir que reagrupar
// uma tabela agg por groupCol não colapsa linhas de métricas DISTINCT erradas:
// uma coluna que é ancestral de groupCol é constante dentro de cada groupCol.
var orgAncestors = map[string]map[string]bool{
	"cod_cli":        {"cod_rca": true, "cod_supervisor": true, "cod_gerente": true},
	"cod_rca":        {"cod_supervisor": true, "cod_gerente": true},
	"cod_supervisor": {"cod_gerente": true},
	"cod_gerente":    {},
}

// aggWithoutPositivacao — true se a tabela agg NÃO tem colunas base_cli,
// positivados, mix (V06 "Por Rede" e V07 "Por Departamento", migrations 183/184).
// Retornar 0 nessas queries evita "column does not exist".
func aggWithoutPositivacao(shortTableName string) bool {
	return strings.Contains(shortTableName, "_v06_") || strings.Contains(shortTableName, "_v07_")
}

// aggHasMixTotal — tabelas agg que receberam a coluna mix_total (migration 175).
// queryAggregatedMes faz MAX(mix_total); V04 e os níveis-folha mais profundos
// (V01 l4, V02/V03/V05 l3) NÃO têm a coluna. pickAggForCrossFilter só pode
// escolher tabelas desta lista, senão a query quebra (column does not exist).
var aggHasMixTotal = map[string]bool{
	"agg_fat_v01_l0_mes": true, "agg_fat_v01_l1_mes": true, "agg_fat_v01_l2_mes": true, "agg_fat_v01_l3_mes": true,
	"agg_fat_v02_l0_mes": true, "agg_fat_v02_l1_mes": true, "agg_fat_v02_l2_mes": true,
	"agg_fat_v03_l0_mes": true, "agg_fat_v03_l1_mes": true, "agg_fat_v03_l2_mes": true,
	"agg_fat_v05_l0_mes": true, "agg_fat_v05_l1_mes": true, "agg_fat_v05_l2_mes": true,
	"agg_trans_v01_l0_mes": true, "agg_trans_v01_l1_mes": true, "agg_trans_v01_l2_mes": true, "agg_trans_v01_l3_mes": true,
	"agg_trans_v02_l0_mes": true, "agg_trans_v02_l1_mes": true, "agg_trans_v02_l2_mes": true,
	"agg_trans_v03_l0_mes": true, "agg_trans_v03_l1_mes": true, "agg_trans_v03_l2_mes": true,
	"agg_trans_v05_l0_mes": true, "agg_trans_v05_l1_mes": true, "agg_trans_v05_l2_mes": true,
	// V08/V09 (mig 197) já nascem com mix_total (calculado inline no upsert).
	"agg_fat_v08_l0_mes": true, "agg_fat_v08_l1_mes": true, "agg_fat_v08_l2_mes": true, "agg_fat_v08_l3_mes": true,
	"agg_fat_v09_l1_mes": true, "agg_fat_v09_l2_mes": true, "agg_fat_v09_l3_mes": true, "agg_fat_v09_l4_mes": true,
	"agg_trans_v08_l0_mes": true, "agg_trans_v08_l1_mes": true, "agg_trans_v08_l2_mes": true, "agg_trans_v08_l3_mes": true,
	"agg_trans_v09_l1_mes": true, "agg_trans_v09_l2_mes": true, "agg_trans_v09_l3_mes": true, "agg_trans_v09_l4_mes": true,
}

// aggUFReady — as tabelas V08/V09 (mig 197) nascem VAZIAS até o backfill
// manual rodar (ver comentário da migration). Roteá-las vazias mostraria os
// cards zerados; até lá o filtro de UF segue no scan de vendas_* (comporta-
// mento anterior). O resultado positivo é definitivo (nunca reconsulta);
// enquanto negativo, reconsulta no máximo a cada 5min — assim o backfill é
// detectado sem reiniciar o backend.
var (
	aggUFReadyMu        sync.Mutex
	aggUFReadyVal       bool
	aggUFReadyCheckedAt time.Time
)

func aggUFReady(db *sql.DB) bool {
	aggUFReadyMu.Lock()
	defer aggUFReadyMu.Unlock()
	if aggUFReadyVal {
		return true
	}
	if time.Since(aggUFReadyCheckedAt) < 5*time.Minute {
		return false
	}
	aggUFReadyCheckedAt = time.Now()
	var ok bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM farol.agg_fat_v09_l1_mes)`).Scan(&ok); err != nil {
		log.Printf("[farol:agg] aggUFReady: probe falhou: %v", err)
		return false
	}
	if ok {
		log.Printf("[farol:agg] aggUFReady: V08/V09 populadas → filtro de UF passa a usar agg")
	}
	aggUFReadyVal = ok
	return ok
}

// pickAggForCrossFilter — quando a tabela agg da view atual NÃO serve os filtros
// (filtro cruzado, ex: filtrar por indústria em "Por Equipe"/"Por Gerência"),
// procura uma tabela agg de QUALQUER view, no MESMO grão de groupCol, que
// contenha groupCol + todas as colunas de filtro/drill — e cujas demais colunas
// sejam apenas filtros, drills ou ancestrais de groupCol (para o reagrupar não
// distorcer positivados/base_cli/mix). Retorna a tabela com MENOS colunas (mais
// específica e rápida). Isso troca o scan lento de vendas_* (2+ min) por uma
// query agg (ms) no caso comum de filtrar por fornecedor. Só falha (e cai para
// vendas_*) quando nenhuma agg tem a coluna (ex: filtro por UF/Filial).
func pickAggForCrossFilter(db *sql.DB, fluxo fluxoCtx, groupCol string, drillPath []drillStep, filters multiFilters) (string, bool) {
	// V08/V09 só depois do backfill (tabelas vazias = cards zerados).
	ufReady := aggUFReady(db)

	required := map[string]bool{groupCol: true}
	for col := range filters {
		required[col] = true
	}
	for _, d := range drillPath {
		required[d.Level] = true
	}

	// allowed — colunas que ficam CONSTANTES em cada linha de saída: groupCol,
	// os filtros/drills (fixos), e os ancestrais organizacionais de qualquer um
	// deles (um filtro/grupo fixa o descendente → o ancestral é constante). Uma
	// tabela agg cuja coluna caia fora desse conjunto reagruparia métricas
	// DISTINCT erradas → descartada. (Filtros multi-valor de níveis distintos
	// tornam positivados/mix aproximados — mesmo tradeoff dos demais cruzados.)
	allowed := map[string]bool{}
	for r := range required {
		allowed[r] = true
		for a := range orgAncestors[r] {
			allowed[a] = true
		}
	}

	tables := aggTablesFat
	if fluxo.name == "transmitido" {
		tables = aggTablesTrans
	}

	best := ""
	bestCols := 1 << 30
	for view, levels := range tables {
		if (view == "V08" || view == "V09") && !ufReady {
			continue
		}
		hier := hierarquias[view]
		for drillIdx := range levels {
			// groupCol PRECISA ser o nível mais profundo da tabela: só o nível
			// folha guarda a coluna de nome (nome_<groupCol>); níveis ancestrais
			// guardam apenas o código → MAX(nome_<groupCol>) quebraria.
			if drillIdx >= len(hier) || hier[drillIdx].Level != groupCol {
				continue
			}
			// queryAggregatedMes faz MAX(mix_total) — só tabelas da migration 175.
			if !aggHasMixTotal[levels[drillIdx]] {
				continue
			}
			cols := colsInAggTable(view, drillIdx)
			ok := true
			for r := range required { // precisa conter tudo que a query referencia
				if !cols[r] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			for c := range cols { // nenhuma coluna pode invalidar o reagrupamento
				if !allowed[c] {
					ok = false
					break
				}
			}
			if ok && len(cols) < bestCols {
				bestCols = len(cols)
				best = "farol." + levels[drillIdx]
			}
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// queryAggregatedVendas — agrega DIRETO de vendas_* (fallback para filtros
// cruzados que a tabela agg desnormalizada não consegue servir, ex: filtrar
// por fornecedor em "Por Gerência", ou por UF/Filial em qualquer view).
//
// Calcula valor/plucro/positivados/mix do período. base_cli = compradores
// distintos na janela rolling-12M do recorte (mesma ideia do base_cli V01) —
// sob filtro cruzado a semântica de "clientes ativos" passa a ser
// "compradores distintos do recorte nos últimos 12 meses".
//
// Mais lento que queryAggregatedMes (varre vendas_*), mas só roda quando há
// filtro cruzado ativo. Índices idx_v[ft]_mixtotal_* ajudam no GROUP BY.
// vendasPeriodoCacheEntry — cache de Q1 (métricas de período) do
// queryAggregatedVendas. Mesma mecânica do baseCache: TTL 30min, invalidated
// após import/consolidação. Ganho principal: ranges repetidos (presets "30
// dias", "7 dias") onde Q1 = 8-10s caem para ~50µs no cache hit.
type vendasPeriodoCacheEntry struct {
	data map[string]aggResult
	at   time.Time
}

var (
	vendasPeriodoCacheMu sync.RWMutex
	vendasPeriodoCache   = map[string]vendasPeriodoCacheEntry{}
)

func vendasPeriodoCacheKey(empresaID, fluxoName, groupCol string, periodIni, periodFim time.Time, drillPath []drillStep, filters multiFilters) string {
	var sb strings.Builder
	sb.WriteString(empresaID)
	sb.WriteByte('|')
	sb.WriteString(fluxoName)
	sb.WriteByte('|')
	sb.WriteString(groupCol)
	sb.WriteByte('|')
	sb.WriteString(periodIni.Format("2006-01-02"))
	sb.WriteByte('|')
	sb.WriteString(periodFim.Format("2006-01-02"))
	sb.WriteByte('|')
	for _, d := range drillPath {
		sb.WriteString(d.Level)
		sb.WriteByte('=')
		sb.WriteString(d.Value)
		sb.WriteByte(';')
	}
	sb.WriteByte('|')
	sb.WriteString(filters.names())
	return sb.String()
}

// invalidateVendasPeriodoCache limpa entradas de uma empresa.
func invalidateVendasPeriodoCache(empresaID string) {
	vendasPeriodoCacheMu.Lock()
	for k := range vendasPeriodoCache {
		if strings.HasPrefix(k, empresaID+"|") {
			delete(vendasPeriodoCache, k)
		}
	}
	vendasPeriodoCacheMu.Unlock()
}

// queryWithHigherWorkMem roda query dentro de uma transação com work_mem
// elevado, via scan(rows) — usado nos scans pesados do filtro cruzado
// (UF/Filial), cujo GROUP BY sobre milhões de linhas estourava pro disco
// com o work_mem padrão (EXPLAIN ANALYZE em produção 27/07/2026 mostrou
// "Sort Method: external merge" gastando ~120MB de disco por worker).
// SET LOCAL confina o aumento a esta transação — não afeta o resto do
// pool de conexões. Mesma técnica já usada em upsert_aggs_mes (SQL).
func queryWithHigherWorkMem(db *sql.DB, query string, args []any, scan func(*sql.Rows) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SET LOCAL work_mem = '256MB'`); err != nil {
		log.Printf("[farol:vendas] SET LOCAL work_mem falhou (seguindo sem): %v", err)
	}
	rows, err := tx.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if err := scan(rows); err != nil {
		return err
	}
	return tx.Commit()
}

// scanLabelExpr — expressão do rótulo no caminho de scan (vendas_*). Quase
// todo NameField é coluna real da base, mas nome_cliprinc (V06 "Por Rede") só
// existe nas agg_*_v06_*: lá ela é DERIVADA na consolidação (mig 185) a partir
// de fantasia/nome_cli. Sem esta tradução o scan referencia coluna inexistente
// e QUALQUER filtro cruzado em Por Rede devolve zero cards em silêncio — visto
// em produção 27/07/2026 ("column v.nome_cliprinc does not exist"), valia
// também para o filtro de Tipo de Venda desde a mig 187.
func scanLabelExpr(nameCol string) string {
	if nameCol == "nome_cliprinc" {
		return `COALESCE(NULLIF(MAX(v.fantasia), ''), MAX(v.nome_cli))`
	}
	return "MAX(v." + nameCol + ")"
}

// vendasPeriodoQ1 — executa Q1 (scan vendas_*) ou pega do cache por range.
type vendasPeriodoOutcome struct {
	result  map[string]aggResult
	cached  bool
	elapsed time.Duration
	// err — a consulta FALHOU (ex: shm esgotado no incidente de 27/07/2026).
	// Sem isso, o erro virava mapa vazio e a tela mostrava "0 cards", que o
	// gestor lê como "não teve venda". Resultado vazio legítimo e falha
	// PRECISAM chegar diferentes na UI.
	err error
}

func vendasPeriodoQ1(db *sql.DB, empresaID string, fluxo fluxoCtx, groupCol, nameCol string,
	periodIni, periodFim time.Time, drillPath []drillStep, filters multiFilters) vendasPeriodoOutcome {

	key := vendasPeriodoCacheKey(empresaID, fluxo.name, groupCol, periodIni, periodFim, drillPath, filters)
	vendasPeriodoCacheMu.RLock()
	if e, ok := vendasPeriodoCache[key]; ok && time.Since(e.at) < vendasPeriodoCacheTTL {
		vendasPeriodoCacheMu.RUnlock()
		return vendasPeriodoOutcome{result: e.data, cached: true, elapsed: 0}
	}
	vendasPeriodoCacheMu.RUnlock()

	t1 := time.Now()
	result := make(map[string]aggResult)
	args := []any{empresaID}
	rangeCond := buildRangeCond(fluxo.dateCol, periodIni, periodFim, &args)
	cond := buildDrillCond(drillPath, &args)
	if fc := buildMultiFilterCond(filters, &args); fc != "" {
		cond += " " + fc
	}
	if fluxo.eventoFilter != "" {
		cond += " " + fluxo.eventoFilter // fluxos CCD (cancdev/cortado)
	}
	q := fmt.Sprintf(`
SELECT v.%s AS key, %s AS label,
       SUM(v.pvenda) AS valor, COALESCE(SUM(v.plucro),0) AS plucro,
       COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0) AS positivados,
       COALESCE(COUNT(DISTINCT (v.cnpj, v.cod_prod)) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '')::numeric
         / NULLIF(COUNT(DISTINCT v.cnpj) FILTER (WHERE v.qt > 0),0)::numeric, 0) AS mix,
       COUNT(DISTINCT v.cod_prod) FILTER (WHERE v.qt > 0 AND v.cod_prod <> '') AS mix_total
FROM %s v
WHERE v.empresa_id=$1 AND v.%s <> '' AND %s %s
GROUP BY v.%s`,
		groupCol, scanLabelExpr(nameCol), fluxo.tableName, groupCol, rangeCond, cond, groupCol)
	err := queryWithHigherWorkMem(db, q, args, func(rows *sql.Rows) error {
		for rows.Next() {
			var key string
			var r aggResult
			if rows.Scan(&key, &r.label, &r.valor, &r.plucro, &r.positivados, &r.mix, &r.mixTotal) == nil {
				r.liquido = r.valor // scan da base (filtro cruzado) sem composição → líquido = bruto
				result[key] = r
			}
		}
		return nil
	})
	if err != nil {
		// NÃO grava no cache: um erro cacheado transformaria uma falha pontual
		// em "sem venda" por 20h.
		log.Printf("[farol:vendas] queryAggregatedVendas período nível=%s ERRO: %v", groupCol, err)
		return vendasPeriodoOutcome{result: result, cached: false, elapsed: time.Since(t1), err: err}
	}

	vendasPeriodoCacheMu.Lock()
	vendasPeriodoCache[key] = vendasPeriodoCacheEntry{data: result, at: time.Now()}
	vendasPeriodoCacheMu.Unlock()

	return vendasPeriodoOutcome{result: result, cached: false, elapsed: time.Since(t1)}
}

// queryAggregatedVendas devolve também o erro da consulta: um mapa vazio pode
// significar "recorte sem venda" OU "a consulta falhou", e a UI precisa
// distinguir os dois (incidente de shm em 27/07/2026 mostrava painel vazio).
func queryAggregatedVendas(db *sql.DB, empresaID string, fluxo fluxoCtx, view, groupCol, nameCol string,
	periodIni, periodFim time.Time, drillPath []drillStep, filters multiFilters) (map[string]aggResult, error) {

	t0 := time.Now()

	// Q1 — métricas do período, com cache por range.
	out := vendasPeriodoQ1(db, empresaID, fluxo, groupCol, nameCol, periodIni, periodFim, drillPath, filters)
	result := out.result
	durQ1 := out.elapsed
	q1Hit := out.cached
	qErr := out.err

	// Query 2 — base_cli = compradores distintos rolling-12M do recorte.
	// OTIMIZAÇÃO: quando a folha (agg_<fluxo>_<view>_l<N>_mes) atende os
	// filtros (sem filtro cruzado de UF/filial), usamos cachedDistinctPositivados
	// em vez de reescanear vendas_*. A folha é grão cnpj×mês (muito menor que
	// vendas_* ~5.8M linhas) e tem índices otimizados (mig 176/179). Reduz
	// 27-46s para ~50ms.
	t2 := time.Now()
	base12mIni := time.Date(periodFim.Year(), periodFim.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
	ymStart := base12mIni.Year()*100 + int(base12mIni.Month())
	ymEnd := periodFim.Year()*100 + int(periodFim.Month())

	if fluxo.isCCD || view == "V06" || view == "V07" {
		// CCD e V06/V07 (Rede/Departamento) não têm positivação — pula base_cli
		// (evita scan caro; a positivação é escondida na UI nesses casos).
	} else if leafServesPositivados(fluxo, view, groupCol, drillPath, filters) {
		// Caminho rápido: folha com cache.
		baseMap, _ := cachedDistinctPositivados(db, empresaID, fluxo, view, groupCol, ymStart, ymEnd, drillPath, filters)
		for k, v := range baseMap {
			if r, ok := result[k]; ok {
				r.baseCli = v
				result[k] = r
			}
		}
	} else {
		// Caminho fallback: scan rolling-12M em vendas_* (filtros não atendidos
		// pela folha, ex: UF/Filial). Mantido por correção — caso raro.
		bargs := []any{empresaID}
		brange := buildRangeCond(fluxo.dateCol, base12mIni, periodFim, &bargs)
		bcond := buildDrillCond(drillPath, &bargs)
		if fc := buildMultiFilterCond(filters, &bargs); fc != "" {
			bcond += " " + fc
		}
		bq := fmt.Sprintf(`
SELECT v.%s AS key, COUNT(DISTINCT v.cnpj) AS base_cli
FROM %s v
WHERE v.empresa_id=$1 AND v.%s <> '' AND v.qt > 0 AND %s %s
GROUP BY v.%s`,
			groupCol, fluxo.tableName, groupCol, brange, bcond, groupCol)
		berr := queryWithHigherWorkMem(db, bq, bargs, func(brows *sql.Rows) error {
			for brows.Next() {
				var key string
				var base int
				if brows.Scan(&key, &base) == nil {
					if r, ok := result[key]; ok {
						r.baseCli = base
						result[key] = r
					}
				}
			}
			return nil
		})
		if berr != nil {
			// Falha parcial: valores do período seguem corretos, mas "clientes
			// ativos" fica zerado. Também precisa chegar à UI — número errado
			// silencioso é pior que aviso.
			log.Printf("[farol:vendas] queryAggregatedVendas base12m nível=%s ERRO: %v", groupCol, berr)
			if qErr == nil {
				qErr = berr
			}
		}
	}
	durQ2 := time.Since(t2)

	log.Printf("[farol:vendas] queryAggregatedVendas fluxo=%s nível=%s → %d grupos em %v (Q1=%v%s Q2=%v %s)",
		fluxo.name, groupCol, len(result), time.Since(t0), durQ1,
		func() string {
			if q1Hit {
				return " (hit)"
			}
			return ""
		}(),
		durQ2,
		func() string {
			if leafServesPositivados(fluxo, view, groupCol, drillPath, filters) {
				return "via-folha"
			}
			return "via-vendas_*"
		}())
	return result, qErr
}

// ─── fetchCards ───────────────────────────────────────────────────────────────

func fetchCards(db *sql.DB, empresaID string, fluxo fluxoCtx, view string,
	pr periodResolution, drillIdx int, level hierLevel, drillPath []drillStep,
	filters multiFilters) ([]cardItem, cardsDiag) {

	t0 := time.Now()
	var diag cardsDiag
	groupCol := safeColName(level.Level)
	nameCol := safeColName(level.NameField)

	// Após mig 165: cod_prod lê direto de vendas_*; resto SEMPRE de agg_*_mes.
	// Range parcial dentro do mês expande pro mês inteiro (grão mensal).
	// EXCEÇÃO: filtro cruzado (coluna ausente na tabela agg desnormalizada) força
	// o caminho vendas_* — senão a query referenciaria coluna inexistente.
	aggName, hasAgg := getAggTableName(fluxo, view, drillIdx)
	aggOK := aggServesFilters(view, drillIdx, filters)

	// Filtro cruzado: a tabela agg da view atual não tem a coluna do filtro
	// (ex: filtrar por indústria em "Por Equipe"/"Por Gerência"). Antes isso caía
	// para o scan de vendas_* (2+ min → painel congelava). Agora tenta uma tabela
	// agg de OUTRA view, no mesmo grão, que contenha groupCol + os filtros → segue
	// rápido. Só cai para vendas_* se nenhuma agg servir (ex: filtro por UF/Filial).
	if !aggOK && groupCol != "cod_prod" && !fluxo.isCCD {
		if alt, ok := pickAggForCrossFilter(db, fluxo, groupCol, drillPath, filters); ok {
			log.Printf("[farol:agg] filtro cruzado (filtros=%s) → tabela agg alternativa %s (em vez de scan vendas_*)", filters.names(), alt)
			aggName, hasAgg, aggOK = alt, true, true
		} else {
			log.Printf("[farol:agg] filtro cruzado (filtros=%s nível=%s) SEM agg alternativa → scan vendas_* (lento)", filters.names(), groupCol)
			// Avisa a UI: esta combinação varre a base bruta (dezenas de
			// segundos). O usuário precisa saber que a demora é da combinação
			// escolhida, não de travamento do sistema.
			diag.Lento = true
			diag.Combinacao = filters.names()
		}
	}

	useAggMes := groupCol != "cod_prod" && hasAgg && aggOK

	// Range CURTO e parcial (ex: "Dia Anterior", "7 dias", "30 dias") → as
	// agg_*_mes (grão mensal) expandiriam para o mês inteiro, falseando o
	// resultado. Nesses casos lemos o diário cru via queryAggregatedVendas
	// (mesmo caminho do filtro cruzado), que respeita a data exata.
	// Ranges LONGOS parciais (YTD, MTD — começam no dia 1) seguem em agg_mes:
	// a expansão do último mês parcial é inofensiva (não há dados futuros) e
	// mantém o login rápido. Limite de 31 dias separa "atalho de dias" de
	// "acumulado de meses".
	// Discriminador: janelas deslizantes (last7/last30/dia_anterior) começam
	// FORA do dia 1; acumulados de calendário (MTD/YTD) começam no dia 1 e o
	// agg_mes os atende exato (não excluem dias com dado). Só janela deslizante
	// curta (≤31d) e parcial precisa do diário cru.
	rangeDias := int(pr.RefFim.Sub(pr.RefInicio).Hours()/24) + 1
	refDiario := rangeDias <= 31 && !isCompleteMonthRange(pr.RefInicio, pr.RefFim) &&
		(rangeDias == 1 || pr.RefInicio.Day() != 1)
	if useAggMes && refDiario {
		useAggMes = false
		aggOK = false // força o switch a cair em queryAggregatedVendas
		log.Printf("[farol:agg] range curto/parcial [%s..%s] (%dd) → diário cru (vendas_*) em vez de agg_mes",
			pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"), rangeDias)
	}

	log.Printf("[farol:agg] fetchCards empresa=%s fluxo=%s view=%s nível=%s ref=[%s..%s] comp=[%s..%s] drill=%d filters=%d",
		empresaID, fluxo.name, aggName, groupCol,
		pr.RefInicio.Format("2006-01-02"), pr.RefFim.Format("2006-01-02"),
		fmtDateOrEmpty(pr.CompInicio), fmtDateOrEmpty(pr.CompFim),
		len(drillPath), len(filters))

	// Path produto: usa vendas_* direto (buildRangeCond + drillCond por data)
	atualArgs := []any{empresaID}
	atualCond := buildRangeCond(fluxo.dateCol, pr.RefInicio, pr.RefFim, &atualArgs)
	drillCond := buildDrillCond(drillPath, &atualArgs)
	filterCond := buildMultiFilterCond(filters, &atualArgs)
	if filterCond != "" {
		drillCond = drillCond + " " + filterCond
	}

	// Path agg_mes: args ymStart/ymEnd em vez de datas
	var atualArgsMes []any
	var mesCond, drillCondMes string
	if useAggMes {
		atualArgsMes = []any{empresaID}
		mesCond = buildMesCond(ym(pr.RefInicio), ym(pr.RefFim), &atualArgsMes)
		drillCondMes = buildDrillCond(drillPath, &atualArgsMes)
		if fc := buildMultiFilterCond(filters, &atualArgsMes); fc != "" {
			drillCondMes = drillCondMes + " " + fc
		}
	}

	var atualMap, antMap map[string]aggResult
	// Erros do caminho vendas_*: cada goroutine escreve o seu (sem corrida) e
	// viram diag.Falhou depois do Wait. Sem isso, uma consulta que morre vira
	// mapa vazio indistinguível de "recorte sem venda".
	var errAtual, errAnt error
	hasComp := !pr.CompInicio.IsZero() && !pr.CompFim.IsZero()

	// P1: mix_total agora vem materializado nas agg (MAX(mix_total) em
	// queryAggregatedMes) e calculado inline em queryAggregatedVendas (filtro
	// cruzado). Não há mais scan separado de vendas_* (queryMixTotal removido).
	var wg sync.WaitGroup
	wg.Add(1)
	switch {
	case groupCol == "cod_prod":
		go func() { defer wg.Done(); atualMap = queryProdutos(db, fluxo, atualCond, drillCond, atualArgs) }()
	case !useAggMes:
		// Sem agg utilizável (filtro cruzado, OU nível sem tabela agg como
		// Cliente/Produto sob a Rede em V06) → agrega de vendas_* (tem todas as
		// colunas). Usa !useAggMes (não !aggOK): aggOK é true sem filtros mas o
		// nível pode não ter agg (hasAgg=false) → queryAggregatedMes quebraria
		// com view vazia.
		go func() {
			defer wg.Done()
			atualMap, errAtual = queryAggregatedVendas(db, empresaID, fluxo, view, groupCol, nameCol, pr.RefInicio, pr.RefFim, drillPath, filters)
		}()
	default:
		go func() {
			defer wg.Done()
			atualMap = queryAggregatedMes(db, aggName, groupCol, nameCol, mesCond, drillCondMes, atualArgsMes)
		}()
	}

	if hasComp {
		antArgs := []any{empresaID}
		antCond := buildRangeCond(fluxo.dateCol, pr.CompInicio, pr.CompFim, &antArgs)
		antDrill := buildDrillCond(drillPath, &antArgs)
		if antFc := buildMultiFilterCond(filters, &antArgs); antFc != "" {
			antDrill = antDrill + " " + antFc
		}
		wg.Add(1)
		switch {
		case groupCol == "cod_prod":
			go func() { defer wg.Done(); antMap = queryProdutosAnterior(db, fluxo, antCond, antDrill, antArgs) }()
		case !useAggMes:
			go func() {
				defer wg.Done()
				antMap, errAnt = queryAggregatedVendas(db, empresaID, fluxo, view, groupCol, nameCol, pr.CompInicio, pr.CompFim, drillPath, filters)
			}()
		default:
			antArgsMes := []any{empresaID}
			antMesCond := buildMesCond(ym(pr.CompInicio), ym(pr.CompFim), &antArgsMes)
			antDrillMes := buildDrillCond(drillPath, &antArgsMes)
			if fc := buildMultiFilterCond(filters, &antArgsMes); fc != "" {
				antDrillMes = antDrillMes + " " + fc
			}
			go func() {
				defer wg.Done()
				antMap = queryAggregatedMes(db, aggName, groupCol, nameCol, antMesCond, antDrillMes, antArgsMes)
			}()
		}
	}
	wg.Wait()

	// Positivados = COUNT(DISTINCT cnpj) por agrupador no período informado.
	// Clientes Ativos (base PROVISÓRIA do Heverton) = COUNT(DISTINCT cnpj) que já
	// compraram no recorte, considerando TODO o período disponível (não 12M, não
	// a carteira do Keslley — que segue no banco, só não é exibida). Ambos lidos
	// da folha (grão cnpj). vendas_* (filtro cruzado) e produto já contam distinto.
	if useAggMes && leafServesPositivados(fluxo, view, groupCol, drillPath, filters) {
		tPos := time.Now()
		var base, refPos, antPos map[string]int
		var baseHit, refHit, antHit bool
		var baseDur, refDur, antDur time.Duration
		var wgPos sync.WaitGroup
		wgPos.Add(1)
		go func() {
			defer wgPos.Done()
			t := time.Now()
			base, baseHit = queryBasePositivados(db, empresaID, fluxo, view, groupCol, drillPath, filters)
			baseDur = time.Since(t)
		}()
		wgPos.Add(1)
		go func() {
			defer wgPos.Done()
			t := time.Now()
			refPos, refHit = cachedDistinctPositivados(db, empresaID, fluxo, view, groupCol, ym(pr.RefInicio), ym(pr.RefFim), drillPath, filters)
			refDur = time.Since(t)
		}()
		if hasComp {
			wgPos.Add(1)
			go func() {
				defer wgPos.Done()
				t := time.Now()
				antPos, antHit = cachedDistinctPositivados(db, empresaID, fluxo, view, groupCol, ym(pr.CompInicio), ym(pr.CompFim), drillPath, filters)
				antDur = time.Since(t)
			}()
		}
		wgPos.Wait()
		// Instrumentação do bloco que ficava invisível no log: cada COUNT(DISTINCT
		// cnpj) na folha (base=0..999912 é o mais caro, sem índice pro BETWEEN
		// calculado ano*100+mes). hit=cache válido (TTL 20h); miss=query real.
		log.Printf("[farol:posit] fetchCards fluxo=%s nível=%s base=%v(hit=%t) ref=%v(hit=%t) ant=%v(hit=%t) total=%v",
			fluxo.name, groupCol, baseDur, baseHit, refDur, refHit, antDur, antHit, time.Since(tPos))

		for k, r := range atualMap {
			r.positivados = refPos[k]
			r.baseCli = base[k]
			atualMap[k] = r
		}
		if hasComp {
			for k, r := range antMap {
				r.positivados = antPos[k]
				r.baseCli = base[k]
				antMap[k] = r
			}
		}
	}

	// Cor binária: verde se atingiu ≥ 100% do anterior, vermelho caso contrário.
	// Sem comparativo, considera neutro (verde — sem alerta).
	pickCor := func(atual, ant float64) (float64, string) {
		if !hasComp {
			return 0, "verde"
		}
		var pct float64
		if ant > 0 {
			pct = atual / ant * 100
		} else if atual > 0 {
			pct = 100
		}
		if pct >= 100 {
			return pct, "verde"
		}
		return pct, "vermelho"
	}

	seen := make(map[string]bool, len(atualMap))
	cards := make([]cardItem, 0, len(atualMap)+len(antMap))

	for key, r := range atualMap {
		seen[key] = true
		ant := antMap[key] // aggResult zero se não existir

		// Venda — no faturado a base exibida é o LÍQUIDO (semáforo segue a tela);
		// o front soma os toggles por cima. No transmitido, segue o valor bruto.
		baseAtual, baseAnt := r.valor, ant.valor
		if fluxo.name == "faturado" {
			baseAtual, baseAnt = r.liquido, ant.liquido
		}
		pct, cor := pickCor(baseAtual, baseAnt)

		// Positivação — % de positivados sobre base de clientes ativos
		positPct := 0.0
		if r.baseCli > 0 {
			positPct = float64(r.positivados) / float64(r.baseCli) * 100
		}
		positPctAnt := 0.0
		if ant.baseCli > 0 {
			positPctAnt = float64(ant.positivados) / float64(ant.baseCli) * 100
		}
		_, positCor := pickCor(positPct, positPctAnt)

		// Mix médio
		_, mixCor := pickCor(r.mix, ant.mix)

		card := cardItem{
			Key: key, Label: r.label,
			Level: level.Level, LevelLabel: level.Label,
			ValorAtual: baseAtual, ValorAnt: baseAnt,
			Pct: pct, Cor: cor,
			Plucro: r.plucro, PlucroAnt: ant.plucro,
			Positivados: r.positivados, BaseCli: r.baseCli, PositPct: positPct,
			PositivadosAnt: ant.positivados, BaseCliAnt: ant.baseCli, PositPctAnt: positPctAnt,
			PositCor: positCor,
			Mix:      r.mix, MixAnt: ant.mix, MixCor: mixCor,
			MixTotal: r.mixTotal, MixTotalAnt: ant.mixTotal,
			Comp:    composicao{Bonif: r.bonif, Transf: r.transf, Remessa: r.remessa, Devol: r.devol, Cancel: r.cancel},
			CompAnt: composicao{Bonif: ant.bonif, Transf: ant.transf, Remessa: ant.remessa, Devol: ant.devol, Cancel: ant.cancel},
		}
		if fluxo.name == "transmitido" {
			card.Transmitido = baseAtual
		} else {
			card.Faturado = baseAtual
		}
		cards = append(cards, card)
	}

	// Grupos que existiam no comparativo mas zero no período principal → vermelho em tudo
	for key, ant := range antMap {
		if seen[key] || ant.valor == 0 {
			continue
		}
		baseAnt := ant.valor
		if fluxo.name == "faturado" {
			baseAnt = ant.liquido
		}
		positPctAnt := 0.0
		if ant.baseCli > 0 {
			positPctAnt = float64(ant.positivados) / float64(ant.baseCli) * 100
		}
		cards = append(cards, cardItem{
			Key: key, Label: ant.label,
			Level: level.Level, LevelLabel: level.Label,
			ValorAtual: 0, ValorAnt: baseAnt,
			Pct: 0, Cor: "vermelho",
			PlucroAnt:      ant.plucro,
			PositivadosAnt: ant.positivados, BaseCliAnt: ant.baseCli, PositPctAnt: positPctAnt,
			PositCor: "vermelho", MixAnt: ant.mix, MixCor: "vermelho",
			MixTotalAnt: ant.mixTotal,
			CompAnt:     composicao{Bonif: ant.bonif, Transf: ant.transf, Remessa: ant.remessa, Devol: ant.devol, Cancel: ant.cancel},
		})
	}

	diag.Falhou = errAtual != nil || errAnt != nil
	diag.MS = time.Since(t0).Milliseconds()

	log.Printf("[farol:view] fetchCards fluxo=%s nível=%s → %d cards (atual=%d ant-only=%d) total=%v%s",
		fluxo.name, groupCol, len(cards), len(atualMap), len(cards)-len(atualMap), time.Since(t0),
		func() string {
			switch {
			case diag.Falhou:
				return " [FALHOU — resultado incompleto]"
			case diag.Lento:
				return " [caminho lento: scan vendas_*]"
			}
			return ""
		}())
	return cards, diag
}

// NOTA: o "de Y" (universo de SKUs) do Mix foi OMITIDO da tela a pedido do
// gestor. As funções que recalculavam esse universo no totalizador
// (queryDistinctMixTotal / fornecCodInScope / queryFornecUniverse /
// applyFornecMixTotal) foram removidas por custarem COUNT(DISTINCT) sobre
// vendas_* a cada request (afetava a performance). O mix_total materializado por
// card (cheap, via agg) continua existindo caso o "de Y" volte a ser exibido.

// ─── computeKPI ──────────────────────────────────────────────────────────────

// computeKPI agrega os totais dos cards.
//
// overlappingBase deve ser true quando os cards agrupam por cod_fornec: nesse
// nível a base de clientes (base_cli) é a mesma para todos os cards (clientes
// da empresa/supervisor/RCA, independente de fornecedor). Somar base_cli
// multiplicaria a base pelo número de fornecedores — dupla contagem.
// Com overlappingBase=true usamos MAX(base_cli) e recalculamos positPct como
// média das taxas por card, recompondo positivados = positPct × base.
func computeKPI(cards []cardItem, _ string, overlappingBase bool) kpiSummary {
	var kpi kpiSummary
	var mixTotal, mixAntTotal float64
	mixCount, mixAntCount := 0, 0

	var positPctSum, positPctAntSum float64
	positCount, positAntCount := 0, 0

	for _, c := range cards {
		kpi.TotalAtual += c.ValorAtual
		kpi.TotalAnt += c.ValorAnt
		kpi.TotalFaturado += c.Faturado
		kpi.TotalTransmitido += c.Transmitido
		kpi.TotalPlucro += c.Plucro
		kpi.TotalPlucroAnt += c.PlucroAnt
		if overlappingBase {
			if c.BaseCli > kpi.TotalBaseCli {
				kpi.TotalBaseCli = c.BaseCli
			}
			if c.BaseCliAnt > kpi.TotalBaseCliAnt {
				kpi.TotalBaseCliAnt = c.BaseCliAnt
			}
			if c.BaseCli > 0 {
				positPctSum += c.PositPct
				positCount++
			}
			if c.BaseCliAnt > 0 {
				positPctAntSum += c.PositPctAnt
				positAntCount++
			}
		} else {
			kpi.TotalPositivados += c.Positivados
			kpi.TotalBaseCli += c.BaseCli
			kpi.TotalPositivadosAnt += c.PositivadosAnt
			kpi.TotalBaseCliAnt += c.BaseCliAnt
		}
		if c.Mix > 0 {
			mixTotal += c.Mix
			mixCount++
		}
		if c.MixAnt > 0 {
			mixAntTotal += c.MixAnt
			mixAntCount++
		}
		// Mix Total (universo de SKUs distintos): SUM dos cards.
		// Em V01 (cod_fornec) cada SKU pertence a apenas 1 fornec, então SUM = universo exato.
		// Em outros níveis pode haver SKU compartilhado entre cards (aproximação razoável).
		kpi.TotalMixTotal += c.MixTotal
		kpi.TotalMixTotalAnt += c.MixTotalAnt
		kpi.Comp.Bonif += c.Comp.Bonif
		kpi.Comp.Transf += c.Comp.Transf
		kpi.Comp.Remessa += c.Comp.Remessa
		kpi.Comp.Devol += c.Comp.Devol
		kpi.Comp.Cancel += c.Comp.Cancel
		kpi.CompAnt.Bonif += c.CompAnt.Bonif
		kpi.CompAnt.Transf += c.CompAnt.Transf
		kpi.CompAnt.Remessa += c.CompAnt.Remessa
		kpi.CompAnt.Devol += c.CompAnt.Devol
		kpi.CompAnt.Cancel += c.CompAnt.Cancel
		if c.Cor == "verde" {
			kpi.Verdes++
		} else {
			kpi.Vermelhos++
		}
	}
	// Venda — % e cor
	if kpi.TotalAnt > 0 {
		kpi.TotalPct = kpi.TotalAtual / kpi.TotalAnt * 100
	} else if kpi.TotalAtual > 0 {
		kpi.TotalPct = 100
	}
	kpi.TotalCor = "vermelho"
	if kpi.TotalPct >= 100 {
		kpi.TotalCor = "verde"
	}
	// Positivação — % e cor (atual vs comparativo)
	if overlappingBase {
		if positCount > 0 {
			kpi.TotalPositPct = positPctSum / float64(positCount)
		}
		if positAntCount > 0 {
			kpi.TotalPositPctAnt = positPctAntSum / float64(positAntCount)
		}
		// Reconstrói contagem absoluta a partir da média de % × base correta
		kpi.TotalPositivados = int(kpi.TotalPositPct/100*float64(kpi.TotalBaseCli) + 0.5)
		kpi.TotalPositivadosAnt = int(kpi.TotalPositPctAnt/100*float64(kpi.TotalBaseCliAnt) + 0.5)
	} else {
		if kpi.TotalBaseCli > 0 {
			kpi.TotalPositPct = float64(kpi.TotalPositivados) / float64(kpi.TotalBaseCli) * 100
		}
		if kpi.TotalBaseCliAnt > 0 {
			kpi.TotalPositPctAnt = float64(kpi.TotalPositivadosAnt) / float64(kpi.TotalBaseCliAnt) * 100
		}
	}
	kpi.TotalPositCor = "vermelho"
	if kpi.TotalPositPct >= kpi.TotalPositPctAnt {
		kpi.TotalPositCor = "verde"
	}
	// Mix médio — atual + comparativo + cor
	if mixCount > 0 {
		kpi.AvgMix = mixTotal / float64(mixCount)
	}
	if mixAntCount > 0 {
		kpi.AvgMixAnt = mixAntTotal / float64(mixAntCount)
	}
	kpi.MixCor = "vermelho"
	if kpi.AvgMix >= kpi.AvgMixAnt {
		kpi.MixCor = "verde"
	}
	return kpi
}

// queryDistinctCliPositivados retorna COUNT(DISTINCT cnpj) WHERE positivados > 0
// na tabela folha (grain cnpj) para o intervalo mensal dado.
// Corrige o KPI totalizador quando overlappingBase=true: a média de percentuais
// por fornecedor subestima o total real quando há muitos fornecedores de baixo volume.
func queryDistinctCliPositivados(db *sql.DB, fluxo fluxoCtx, view string, empresaID string, ymStart, ymEnd int, drillPath []drillStep, filters multiFilters) int {
	tables := aggTablesFat
	if fluxo.name == "transmitido" {
		tables = aggTablesTrans
	}
	tbl, ok := tables[view]
	if !ok || len(tbl) == 0 {
		return 0
	}
	leafTable := "farol." + tbl[len(tbl)-1]

	args := []any{empresaID}
	mesCond := buildMesCond(ymStart, ymEnd, &args)
	cond := buildDrillCond(drillPath, &args)
	if fc := buildMultiFilterCond(filters, &args); fc != "" {
		cond += " " + fc
	}

	q := fmt.Sprintf(`
SELECT COUNT(DISTINCT cnpj)
FROM %s v
WHERE v.empresa_id=$1 AND %s %s AND v.positivados > 0`,
		leafTable, mesCond, cond)

	var count int
	if err := db.QueryRow(q, args...).Scan(&count); err != nil {
		log.Printf("[farol:posit] queryDistinctCliPositivados view=%s ERRO: %v", view, err)
		return 0
	}
	return count
}

// leafTableFor — tabela folha (grão cnpj) da view: o nível mais profundo das
// agg_*_mes (cliente/cnpj; produto não é agg). Tem cnpj + toda a hierarquia.
func leafTableFor(fluxo fluxoCtx, view string) (string, int, bool) {
	tables := aggTablesFat
	if fluxo.name == "transmitido" {
		tables = aggTablesTrans
	}
	lvls, ok := tables[view]
	if !ok || len(lvls) == 0 {
		return "", 0, false
	}
	leafIdx := len(lvls) - 1
	return "farol." + lvls[leafIdx], leafIdx, true
}

// leafServesPositivados — true se a folha da view contém groupCol + todas as
// colunas de drill/filtro (logo dá pra contar cnpj distinto por groupCol nela).
// V06/V07 (Fase 2 novo layout) não têm métricas de positivação — retorna false
// pra evitar que queryDistinctPositivados/queryDistinctCliPositivados sejam
// chamadas em tabelas onde v.positivados/v.cnpj não existem.
func leafServesPositivados(fluxo fluxoCtx, view, groupCol string, drillPath []drillStep, filters multiFilters) bool {
	if fluxo.isCCD || view == "V06" || view == "V07" {
		return false // CCD não tem agg/positivação
	}
	_, leafIdx, ok := leafTableFor(fluxo, view)
	if !ok {
		return false
	}
	cols := colsInAggTable(view, leafIdx)
	if !cols[groupCol] {
		return false
	}
	for _, d := range drillPath {
		if !cols[d.Level] {
			return false
		}
	}
	for c := range filters {
		if !cols[c] {
			return false
		}
	}
	return true
}

// ─── Cache da "base" de clientes ativos ──────────────────────────────────────
// queryDistinctPositivados varre a folha (cnpj×mês) — cara em qualquer janela.
// Mas resultados mudam só após nova importação. Cache em memória cobre:
//   - base (ymStart=0, ymEnd=999912): idêntica entre views/requests/usuários
//   - ref e comp: idêntica entre os 3 fetchCards do login (V01/V02/V03)
//
// invalidateBaseCache é chamado após consolidação de import (ver TTL abaixo).
type baseCacheEntry struct {
	data map[string]int
	at   time.Time
}

var (
	baseCacheMu sync.RWMutex
	baseCache   = map[string]baseCacheEntry{}
)

// baseCacheTTL — o miss aqui custa 10-25s (COUNT(DISTINCT cnpj) na folha, visto
// em produção 27/07/2026: Indústrias 25s, Por Equipe 10s). Era 30min, o que
// anulava o prewarm: meia hora após o boot o cache já expirava e o próximo
// usuário pagava tudo de novo (login das 14:19 com restart às 11:13).
//
// 20h porque o aquecimento é DIÁRIO (StartDailyPrewarm, 07:30): o que foi
// aquecido de manhã precisa sobreviver ao expediente inteiro, senão a partir
// do meio da tarde os usuários voltam a pagar o recálculo. Não é risco de dado
// velho — invalidateBaseCache* roda nos pontos onde o dado muda; o TTL é teto
// de segurança, não garantia de frescor.
const baseCacheTTL = 20 * time.Hour

// vendasPeriodoCacheTTL — mesmo raciocínio para o cache de Q1 do scan de
// vendas_* (filtro cruzado, 13-40s por miss).
const vendasPeriodoCacheTTL = 20 * time.Hour

// invalidateBaseCache limpa TODAS as entradas de uma empresa. Use apenas quando
// o conjunto de meses afetados é desconhecido ou os dados foram apagados em
// bloco; para carga/consolidação de meses específicos prefira
// invalidateBaseCacheMeses, que preserva o histórico já aquecido.
func invalidateBaseCache(empresaID string) {
	baseCacheMu.Lock()
	for k := range baseCache {
		if strings.HasPrefix(k, empresaID+"|") {
			delete(baseCache, k)
		}
	}
	baseCacheMu.Unlock()
}

// ymOverlapsKeyRange — decide se a entrada de cache cujo período é [aIni,aFim]
// (em ym = ano*100+mes) é afetada por uma carga que tocou [bIni,bFim].
func ymOverlapsKeyRange(aIni, aFim, bIni, bFim int) bool {
	return aIni <= bFim && bIni <= aFim
}

// parseYMRangeField converte "202601-202607" nos dois ym. ok=false força o
// chamador a invalidar por precaução (melhor perder cache que servir dado velho).
func parseYMRangeField(f string) (ini, fim int, ok bool) {
	d := strings.IndexByte(f, '-')
	if d <= 0 {
		return 0, 0, false
	}
	ini, err1 := strconv.Atoi(f[:d])
	fim, err2 := strconv.Atoi(f[d+1:])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return ini, fim, true
}

// invalidateBaseCacheMeses limpa só as entradas cujo período TOCA os meses
// recém-carregados. A carga diária (e, em breve, horária) mexe apenas no mês
// corrente: derrubar junto o cache de 2025 e de jan-mai/2026 — dado que não
// mudou — obrigaria a reconstruir 10-25s por view no próximo acesso, todo dia.
// Chaves com período fora do intervalo carregado sobrevivem intactas.
func invalidateBaseCacheMeses(empresaID string, ymIni, ymFim int) {
	pref := empresaID + "|"
	kept, dropped := 0, 0
	baseCacheMu.Lock()
	for k := range baseCache {
		if !strings.HasPrefix(k, pref) {
			continue
		}
		// baseCacheKey: empresa|fluxo|view|groupCol|ymStart-ymEnd|drill|filters
		parts := strings.Split(k, "|")
		if len(parts) < 5 {
			delete(baseCache, k)
			dropped++
			continue
		}
		ini, fim, ok := parseYMRangeField(parts[4])
		if !ok || ymOverlapsKeyRange(ini, fim, ymIni, ymFim) {
			delete(baseCache, k)
			dropped++
			continue
		}
		kept++
	}
	baseCacheMu.Unlock()
	log.Printf("[farol:cache] baseCache invalidado p/ meses %d..%d — %d entradas removidas, %d preservadas (histórico)",
		ymIni, ymFim, dropped, kept)
}

// invalidateVendasPeriodoCacheMeses — equivalente para o cache de Q1. A chave
// guarda datas (YYYY-MM-DD) em vez de ym; converte antes de comparar.
func invalidateVendasPeriodoCacheMeses(empresaID string, ymIni, ymFim int) {
	pref := empresaID + "|"
	kept, dropped := 0, 0
	vendasPeriodoCacheMu.Lock()
	for k := range vendasPeriodoCache {
		if !strings.HasPrefix(k, pref) {
			continue
		}
		// vendasPeriodoCacheKey: empresa|fluxo|groupCol|dataIni|dataFim|drill|filters
		parts := strings.Split(k, "|")
		if len(parts) < 5 {
			delete(vendasPeriodoCache, k)
			dropped++
			continue
		}
		ini, ok1 := ymFromISODate(parts[3])
		fim, ok2 := ymFromISODate(parts[4])
		if !ok1 || !ok2 || ymOverlapsKeyRange(ini, fim, ymIni, ymFim) {
			delete(vendasPeriodoCache, k)
			dropped++
			continue
		}
		kept++
	}
	vendasPeriodoCacheMu.Unlock()
	log.Printf("[farol:cache] vendasPeriodoCache invalidado p/ meses %d..%d — %d removidas, %d preservadas",
		ymIni, ymFim, dropped, kept)
}

// ymFromISODate converte "2026-07-01" em 202607.
func ymFromISODate(s string) (int, bool) {
	if len(s) < 7 || s[4] != '-' {
		return 0, false
	}
	ano, err1 := strconv.Atoi(s[:4])
	mes, err2 := strconv.Atoi(s[5:7])
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return ano*100 + mes, true
}

// mesesRangeYM devolve o menor e o maior ym de uma lista de meses tocados.
func mesesRangeYM(meses []aggMesYM) (ymIni, ymFim int, ok bool) {
	if len(meses) == 0 {
		return 0, 0, false
	}
	ymIni, ymFim = 999912, 0
	for _, m := range meses {
		ym := m.Ano*100 + m.Mes
		if ym < ymIni {
			ymIni = ym
		}
		if ym > ymFim {
			ymFim = ym
		}
	}
	return ymIni, ymFim, true
}

func baseCacheKey(empresaID, fluxoName, view, groupCol string, ymStart, ymEnd int, drillPath []drillStep, filters multiFilters) string {
	var sb strings.Builder
	sb.WriteString(empresaID)
	sb.WriteByte('|')
	sb.WriteString(fluxoName)
	sb.WriteByte('|')
	sb.WriteString(view)
	sb.WriteByte('|')
	sb.WriteString(groupCol)
	sb.WriteByte('|')
	sb.WriteString(strconv.Itoa(ymStart))
	sb.WriteByte('-')
	sb.WriteString(strconv.Itoa(ymEnd))
	sb.WriteByte('|')
	for _, d := range drillPath {
		sb.WriteString(d.Level)
		sb.WriteByte('=')
		sb.WriteString(d.Value)
		sb.WriteByte(';')
	}
	sb.WriteByte('|')
	sb.WriteString(filters.names())
	return sb.String()
}

// queryBasePositivados retorna a base (período inteiro) com cache em memória.
func queryBasePositivados(db *sql.DB, empresaID string, fluxo fluxoCtx, view, groupCol string, drillPath []drillStep, filters multiFilters) (map[string]int, bool) {
	return cachedDistinctPositivados(db, empresaID, fluxo, view, groupCol, 0, 999912, drillPath, filters)
}

// cachedDistinctPositivados envolve queryDistinctPositivados com cache em memória.
// Ref/ant do fetchCards compartilham o cache entre views no mesmo login.
// O bool devolvido (hit) existe só para instrumentação (ver fetchCards) — sem
// ele, um cache MISS caro no "base" (histórico todo, sem índice para o BETWEEN
// calculado) é indistinguível de contenção externa no log de fetchCards.
func cachedDistinctPositivados(db *sql.DB, empresaID string, fluxo fluxoCtx, view, groupCol string, ymStart, ymEnd int, drillPath []drillStep, filters multiFilters) (map[string]int, bool) {
	key := baseCacheKey(empresaID, fluxo.name, view, groupCol, ymStart, ymEnd, drillPath, filters)
	baseCacheMu.RLock()
	if e, ok := baseCache[key]; ok && time.Since(e.at) < baseCacheTTL {
		baseCacheMu.RUnlock()
		return e.data, true
	}
	baseCacheMu.RUnlock()

	data := queryDistinctPositivados(db, empresaID, fluxo, view, groupCol, ymStart, ymEnd, drillPath, filters)
	baseCacheMu.Lock()
	baseCache[key] = baseCacheEntry{data: data, at: time.Now()}
	baseCacheMu.Unlock()
	return data, false
}

// queryDistinctPositivados — CONCEITO OFICIAL do gestor: clientes positivados =
// COUNT(DISTINCT cnpj) por agrupador no período informado (não a média mensal).
// Lê da tabela folha (grão cnpj×mês); um cliente que comprou em qualquer mês do
// período conta 1. Substitui o AVG(positivados) do queryAggregatedMes.
func queryDistinctPositivados(db *sql.DB, empresaID string, fluxo fluxoCtx, view, groupCol string, ymStart, ymEnd int, drillPath []drillStep, filters multiFilters) map[string]int {
	leaf, _, ok := leafTableFor(fluxo, view)
	if !ok {
		return nil
	}
	args := []any{empresaID}
	mesCond := buildMesCond(ymStart, ymEnd, &args)
	cond := buildDrillCond(drillPath, &args)
	if fc := buildMultiFilterCond(filters, &args); fc != "" {
		cond += " " + fc
	}
	q := fmt.Sprintf(`
SELECT v.%s AS key, COUNT(DISTINCT v.cnpj) AS positivados
FROM %s v
WHERE v.empresa_id=$1 AND v.%s <> '' AND v.positivados > 0 AND %s %s
GROUP BY v.%s`, groupCol, leaf, groupCol, mesCond, cond, groupCol)
	rows, err := db.Query(q, args...)
	if err != nil {
		log.Printf("[farol:posit] queryDistinctPositivados view=%s nível=%s ERRO: %v", view, groupCol, err)
		return nil
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var key string
		var n int
		if rows.Scan(&key, &n) == nil {
			out[key] = n
		}
	}
	return out
}

// queryCompanyBaseCli retorna o total de clientes ativos da empresa via mv_*_carteira_rca.
// Usado como denominador do KPI totalizador em fixOverlappingBaseKPI (V01 L0).
func queryCompanyBaseCli(db *sql.DB, fluxo fluxoCtx, empresaID string) int {
	table := "farol.mv_fat_carteira_rca"
	if fluxo.name == "transmitido" {
		table = "farol.mv_trans_carteira_rca"
	}
	var count int
	if err := db.QueryRow(fmt.Sprintf(
		`SELECT COALESCE(SUM(qtcli_rca),0)::INT FROM %s WHERE empresa_id=$1`, table,
	), empresaID).Scan(&count); err != nil {
		log.Printf("[farol:posit] queryCompanyBaseCli ERRO: %v", err)
	}
	return count
}

// fixOverlappingBaseKPI substitui os campos de positivação do KPI totalizador
// quando o agrupamento é por cod_fornec. Usa COUNT(DISTINCT cnpj) real em vez
// da média de percentuais, que subestima quando muitos fornecedores têm pouco volume.
// Quando drillPath está vazio (V01 L0), base_cli agora é rolling-12M por fornecedor,
// então o MAX das linhas daria a base do maior fornecedor — corrigimos com o total real.
func fixOverlappingBaseKPI(db *sql.DB, kpi *kpiSummary, fluxo fluxoCtx, view string, empresaID string, pr periodResolution, drillPath []drillStep, filters multiFilters) {
	// Totalizador = COUNT(DISTINCT cnpj) do RECORTE atual (drill+filtros), em
	// qualquer nível. Assim, ao abrir um fornecedor, o nº de clientes ativos do
	// totalizador é exatamente o que aparecia no card daquele fornecedor na tela
	// anterior. Clientes Ativos (PROVISÓRIO Heverton) = distinct no período todo;
	// positivados = distinct no período. Carteira Rotina 302 (Keslley) segue no
	// banco, só não exibida.
	base := queryDistinctCliPositivados(db, fluxo, view, empresaID, 0, 999912, drillPath, filters)
	kpi.TotalBaseCli = base
	kpi.TotalBaseCliAnt = base
	ref := queryDistinctCliPositivados(db, fluxo, view, empresaID, ym(pr.RefInicio), ym(pr.RefFim), drillPath, filters)
	kpi.TotalPositivados = ref
	if kpi.TotalBaseCli > 0 {
		kpi.TotalPositPct = float64(ref) / float64(kpi.TotalBaseCli) * 100
	}
	if !pr.CompInicio.IsZero() {
		ant := queryDistinctCliPositivados(db, fluxo, view, empresaID, ym(pr.CompInicio), ym(pr.CompFim), drillPath, filters)
		kpi.TotalPositivadosAnt = ant
		if kpi.TotalBaseCliAnt > 0 {
			kpi.TotalPositPctAnt = float64(ant) / float64(kpi.TotalBaseCliAnt) * 100
		}
	}
	kpi.TotalPositCor = "vermelho"
	if kpi.TotalPositPct >= kpi.TotalPositPctAnt {
		kpi.TotalPositCor = "verde"
	}
}

// ─── fetchPeriodosDisponiveis ─────────────────────────────────────────────────

func fetchPeriodosDisponiveis(db *sql.DB, empresaID string) []string {
	// Períodos = meses efetivamente consolidados nas agg (ano/mes pela DATA real
	// da venda). NÃO usar vendas_import_jobs.ano/mes — é a competência do nome do
	// arquivo, que pode estar errada e poluir a lista (ex: "2026-06" sem dados).
	rows, err := db.Query(`
		SELECT ano, mes FROM (
			SELECT ano, mes FROM farol.agg_fat_v01_l0_mes   WHERE empresa_id=$1
			UNION
			SELECT ano, mes FROM farol.agg_trans_v01_l0_mes WHERE empresa_id=$1
		) x
		GROUP BY ano, mes
		ORDER BY ano DESC, mes DESC
	`, empresaID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var ano, mes int
		if rows.Scan(&ano, &mes) == nil {
			result = append(result, fmt.Sprintf("%04d-%02d", ano, mes))
		}
	}
	return result
}

// ─── buildPeriodoLabels ───────────────────────────────────────────────────────

func buildPeriodoLabels(pr periodResolution) (curLabel, antLabel, label string) {
	curLabel = fmtRangeBR(pr.RefInicio, pr.RefFim)
	if pr.CompInicio.IsZero() || pr.CompFim.IsZero() {
		label = curLabel
		return
	}
	antLabel = fmtRangeBR(pr.CompInicio, pr.CompFim)
	label = fmt.Sprintf("Anterior: %s × Atual: %s", antLabel, curLabel)
	return
}

// fmtRangeBR formata um intervalo em pt-BR de forma compacta:
//
//	01/05/2026 → 31/05/2026  →  "Mai/2026"   (mês inteiro)
//	05/05/2026 → 15/05/2026  →  "05/05/2026 – 15/05/2026"
//	01/01/2026 → 31/12/2026  →  "Ano 2026"   (ano inteiro)
func fmtRangeBR(ini, fim time.Time) string {
	if ini.IsZero() || fim.IsZero() {
		return ""
	}
	if ini.Year() == fim.Year() && ini.Month() == fim.Month() &&
		ini.Day() == 1 && fim.AddDate(0, 0, 1).Day() == 1 {
		return fmtMesAno(int(ini.Month()), ini.Year())
	}
	if ini.Year() == fim.Year() && ini.Month() == 1 && ini.Day() == 1 &&
		fim.Month() == 12 && fim.Day() == 31 {
		return fmt.Sprintf("Ano %d", ini.Year())
	}
	return fmt.Sprintf("%s – %s", ini.Format("02/01/2006"), fim.Format("02/01/2006"))
}

// ─── refreshAllFarolViews ─────────────────────────────────────────────────────

// upsertAggMesYM (ano, mes) pequena struct usada na paralelização.
type aggMesYM struct{ Ano, Mes int }

// upsertAggsMesParallel chama farol.upsert_aggs_mes em N goroutines, uma por mês.
// Cada upsert_aggs_mes leva ~4min em 1M rows; rodar 4 em paralelo cai pra ~tempo/4
// no inicial. Carga diária toca 1 mês só — overhead de paralelismo é zero.
// workers=4 escolhido por equilibrar I/O do disco e CPU; pool DB tem 50 conexões.
func upsertAggsMesParallel(db *sql.DB, empresaID string, meses []aggMesYM, workers int) {
	if len(meses) == 0 {
		return
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(meses) {
		workers = len(meses)
	}

	jobs := make(chan aggMesYM, len(meses))
	for _, m := range meses {
		jobs <- m
	}
	close(jobs)

	var wg sync.WaitGroup
	wg.Add(workers)
	tStart := time.Now()
	for i := 0; i < workers; i++ {
		go func(wid int) {
			defer wg.Done()
			for m := range jobs {
				t1 := time.Now()
				if _, e := db.Exec(`SELECT farol.upsert_aggs_mes($1,$2,$3)`, empresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:agg] w=%d UPSERT %04d-%02d ERRO: %v", wid, m.Ano, m.Mes, e)
					continue
				}
				// V06/V07 (mig 185) — funções auxiliares populam agg_*_v06_*/v07_*.
				// Rodam sequencialmente após a principal; se cod_cliprinc/cod_depto
				// não existem nos dados daquele mês (CSVs no layout antigo), as
				// temp tables ficam vazias e o custo é ~0.
				if _, e := db.Exec(`SELECT farol.upsert_aggs_mes_v06($1,$2,$3)`, empresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:agg] w=%d UPSERT V06 %04d-%02d ERRO: %v", wid, m.Ano, m.Mes, e)
				}
				if _, e := db.Exec(`SELECT farol.upsert_aggs_mes_v07($1,$2,$3)`, empresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:agg] w=%d UPSERT V07 %04d-%02d ERRO: %v", wid, m.Ano, m.Mes, e)
				}
				// V08/V09 (mig 197) — aggs com UF no grão (filtro cruzado de UF).
				// Antes de upsert_venda_liquida_cols, que preenche liquido/pv_*
				// também nos níveis novos.
				if _, e := db.Exec(`SELECT farol.upsert_aggs_mes_v08_v09($1,$2,$3)`, empresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:agg] w=%d UPSERT V08/V09 %04d-%02d ERRO: %v", wid, m.Ano, m.Mes, e)
				}
				// tipo_venda (mig 188) — popula dim='tipo_venda' do fluxo faturado
				// para o dropdown do filtro cruzado. Barato (agrega poucos códigos).
				if _, e := db.Exec(`SELECT farol.upsert_tipo_venda_dims($1,$2,$3)`, empresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:agg] w=%d UPSERT tipo_venda_dims %04d-%02d ERRO: %v", wid, m.Ano, m.Mes, e)
				}
				// venda líquida (mig 190) — popula liquido/pv_* nas agg_fat a partir
				// de vendas_faturadas + vendas_ccd. Passada extra sobre o mês; roda
				// depois de todas as agg estarem populadas (v01-v07).
				if _, e := db.Exec(`SELECT farol.upsert_venda_liquida_cols($1,$2,$3)`, empresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:agg] w=%d UPSERT venda_liquida %04d-%02d ERRO: %v", wid, m.Ano, m.Mes, e)
				}
				log.Printf("[farol:agg] w=%d UPSERT %04d-%02d OK em %v", wid, m.Ano, m.Mes, time.Since(t1))
			}
		}(i)
	}
	wg.Wait()
	log.Printf("[farol:agg] upsertAggsMesParallel: %d meses, %d workers, total %v",
		len(meses), workers, time.Since(tStart))
}

// refreshAllFarolViews: após mig 165, só restam mv_*_carteira_rca como MVs
// (pequenas, refresh em ms). Tudo o mais é populado por upsert_aggs_mes
// chamado pelo handler RefreshViews depois desta função.
func refreshAllFarolViews(db *sql.DB) error {
	t0 := time.Now()

	for _, mv := range []string{"farol.mv_fat_carteira_rca", "farol.mv_trans_carteira_rca"} {
		if _, err := db.Exec(`REFRESH MATERIALIZED VIEW CONCURRENTLY ` + mv); err != nil {
			// Sem CONCURRENTLY se nunca foi populada
			if _, err2 := db.Exec(`REFRESH MATERIALIZED VIEW ` + mv); err2 != nil {
				log.Printf("[farol:view] refresh %s ERRO: %v", mv, err2)
				return err2
			}
		}
		db.Exec(`ANALYZE ` + mv)
	}

	log.Printf("[farol:view] refreshAllFarolViews (só carteiras) em %v", time.Since(t0))
	return nil
}

// ─── RefreshViewsHandler — POST /api/v2/farol/refresh-views ─────────────────

func RefreshViewsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		t0 := time.Now()
		log.Printf("[farol:view] RefreshViews início — empresa=%s user=%s", spCtx.EmpresaID, spCtx.UserID)

		if err := refreshAllFarolViews(db); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
			return
		}

		// P2.1 — Consolidação INCREMENTAL: processa só os meses PENDENTES
		// (marcados pelos imports). Se não houver pendências, faz consolidação
		// COMPLETA (fallback p/ refresh manual / rebuild total).
		var meses []aggMesYM
		incremental := false
		prows, perr := db.Query(`SELECT ano, mes FROM farol.consolidacao_pendente WHERE empresa_id=$1 ORDER BY ano, mes`, spCtx.EmpresaID)
		if perr == nil {
			for prows.Next() {
				var r aggMesYM
				if prows.Scan(&r.Ano, &r.Mes) == nil {
					meses = append(meses, r)
				}
			}
			prows.Close()
		}
		if len(meses) > 0 {
			incremental = true
			log.Printf("[farol:view] RefreshViews INCREMENTAL — %d mês(es) pendente(s): %v", len(meses), meses)
		} else if r.URL.Query().Get("full") == "1" {
			// Reconstrução COMPLETA — só roda se pedida explicitamente via
			// ?full=1. Incidente 25/07/2026: nada na UI chama isso de propósito
			// hoje (só o fim da fila de import, sem essa flag); um clique
			// duplo/retry achou a fila pendente vazia e caiu aqui por acidente,
			// reconsolidando os 17 meses (~1h40) e competindo com a consolidação
			// real que já estava rodando. Sem a flag, "sem pendentes" agora é
			// no-op — ver ramo abaixo.
			rows, err := db.Query(`
				SELECT DISTINCT ano, mes FROM (
					SELECT EXTRACT(YEAR  FROM data_faturamento)::int AS ano,
					       EXTRACT(MONTH FROM data_faturamento)::int AS mes
					  FROM vendas_faturadas WHERE empresa_id=$1
					UNION
					SELECT EXTRACT(YEAR  FROM data_transmissao)::int,
					       EXTRACT(MONTH FROM data_transmissao)::int
					  FROM vendas_transmitidas WHERE empresa_id=$1
				) t ORDER BY ano, mes`, spCtx.EmpresaID)
			if err == nil {
				for rows.Next() {
					var r aggMesYM
					if rows.Scan(&r.Ano, &r.Mes) == nil {
						meses = append(meses, r)
					}
				}
				rows.Close()
			}
			log.Printf("[farol:view] RefreshViews COMPLETA (?full=1, sem pendentes) — %d mês(es)", len(meses))
		} else {
			log.Printf("[farol:view] RefreshViews sem pendências — nada a consolidar (nenhum mês tocado desde o último refresh)")
		}

		if len(meses) > 0 {
			anosVistos := map[int]bool{}
			for _, m := range meses {
				if !anosVistos[m.Ano] {
					db.Exec(`SELECT farol.create_agg_year_partitions($1)`, m.Ano)
					anosVistos[m.Ano] = true
				}
			}
			upsertAggsMesParallel(db, spCtx.EmpresaID, meses, 4)

			// P1 — materializa mix_total (universo de SKUs) dos meses consolidados.
			for _, m := range meses {
				if _, e := db.Exec(`SELECT farol.upsert_mixtotal_mes($1,$2,$3)`, spCtx.EmpresaID, m.Ano, m.Mes); e != nil {
					log.Printf("[farol:mix] upsert_mixtotal_mes %04d-%02d ERRO: %v", m.Ano, m.Mes, e)
				}
			}

			// Limpa as pendências consolidadas (só no modo incremental;
			// na completa não havia pendências a limpar).
			if incremental {
				for _, m := range meses {
					db.Exec(`DELETE FROM farol.consolidacao_pendente WHERE empresa_id=$1 AND ano=$2 AND mes=$3`,
						spCtx.EmpresaID, m.Ano, m.Mes)
				}
			}
		}

		// Consolidação terminou → carimbo "dados de" do Painel BI.
		// É AQUI que a carga multi-arquivo (skip_refresh) de fato consolida.
		if len(meses) > 0 {
			go refreshUFMV(db) // MV de UF em background (não bloqueia o RefreshViews)
			marcaConsolidacao(db, spCtx.EmpresaID)
		}

		// Dados mudaram → invalida o cache da base de clientes ativos. Só os
		// períodos consolidados: a carga diária toca o mês corrente e não deve
		// derrubar o histórico já aquecido (2025, 2026 antigo). Sem meses
		// conhecidos (reconstrução total) cai no invalidate completo.
		if ymIni, ymFim, ok := mesesRangeYM(meses); ok {
			invalidateBaseCacheMeses(spCtx.EmpresaID, ymIni, ymFim)
			invalidateVendasPeriodoCacheMeses(spCtx.EmpresaID, ymIni, ymFim)
		} else {
			invalidateBaseCache(spCtx.EmpresaID)
			invalidateVendasPeriodoCache(spCtx.EmpresaID)
		}
		// Painel BI serve resposta pronta do cache; sem isto a TV continuaria
		// mostrando o número de antes do import até o TTL vencer.
		invalidateBICache(spCtx.EmpresaID)

		var fatRows, transRows int
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.agg_fat_v01_l0_mes WHERE empresa_id=$1`, spCtx.EmpresaID).Scan(&fatRows)
		_ = db.QueryRow(`SELECT COUNT(*) FROM farol.agg_trans_v01_l0_mes WHERE empresa_id=$1`, spCtx.EmpresaID).Scan(&transRows)
		log.Printf("[farol:view] RefreshViews concluído — fat_agg=%d trans_agg=%d, total %v",
			fatRows, transRows, time.Since(t0))
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "fat_rows": fatRows, "trans_rows": transRows,
			"duration_ms": time.Since(t0).Milliseconds(),
		})

		// Pré-aquece as MVs em background (YTD atual vs ano anterior) para que o
		// primeiro fetchCards do login não pague o custo de carregar páginas do
		// disco. Sem isso, o primeiro login após import demora 15-17s por
		// fetchCards (MVs geladas no cache do PostgreSQL).
		go prewarmAggMes(db, spCtx.EmpresaID)
	}
}

// prewarmAggMes executa queries representativas nas 3 views principais (V01/V02/V03)
// nos períodos YTD atual e anterior. Aquece o cache do PostgreSQL para que o
// primeiro fetchCards de um login seja tão rápido quanto os subsequentes.
// Roda em background — não bloqueia a response HTTP nem atrapalha a transação.
// PrewarmStartup expõe só a parte RÁPIDA do prewarm (MVs + positivados) para
// ser chamada no boot do processo (main.go), não só depois de um import.
// baseCache é só mapa em memória — todo restart do backend (deploy nosso,
// crash, redeploy do Coolify) zera o cache, e sem isto os primeiros usuários
// reais pagam do zero o custo do COUNT(DISTINCT cnpj) de positivação/base_cli
// (25-33s por view, visto em produção logo após o deploy de 27/07/2026).
//
// Inclui prewarmPeriodKeys: sem ele, só a chave "histórico completo" ficava
// quente e o 1º login após cada deploy ainda pagava ~24s nos períodos de
// referência/comparação (medido em produção 27/07/2026, logins de 16:48 e
// 17:37). Custo aferido no teste do aquecimento diário: 21s na empresa maior
// — contra os ~80s da fase de presets diários. Vale pagar em background: hoje
// esse tempo é cobrado do primeiro usuário de qualquer forma.
//
// NÃO chama prewarmDailyRanges (a fase 2 de prewarmAggMes): essa fase varre
// vendas_faturadas/transmitidas BRUTAS para os presets diários e, com o
// histórico atual (2025 completo + 2026), passou de "2-13s" para 20-59s por
// combinação — rodar isso no boot competiu com tráfego real logo após o
// deploy de 27/07/2026 (fetchCards de usuários reais subiu a 8-17s enquanto
// o prewarm rodava). FOI ESSA fase que causou a regressão, não a de períodos.
// Ela continua rodando só depois de um import (via prewarmAggMes, chamado por
// RefreshViewsHandler) e no aquecimento diário das 07:30 (PrewarmDiario).
func PrewarmStartup(db *sql.DB, empresaID string) {
	prewarmAggMesCore(db, empresaID)
	prewarmPeriodKeys(db, empresaID)
}

func prewarmAggMes(db *sql.DB, empresaID string) {
	t0 := prewarmAggMesCore(db, empresaID)

	// Fase 2 — pré-aquece Q1 (vendasPeriodoCache) dos presets diários comuns.
	// Sem isso, o 1º clique em "Dia Anterior" / "7 dias" / "30 dias" leva
	// alguns segundos. Roda em background após import; usuário não vê.
	t1 := time.Now()
	prewarmDailyRanges(db, empresaID)
	log.Printf("[farol:view] prewarmAggMes diários empresa=%s em %v (total %v)",
		empresaID, time.Since(t1), time.Since(t0))
}

// prewarmAggMesCore aquece as MVs (COUNT(*) pequeno) e o baseCache de
// positivação/base_cli (o COUNT(DISTINCT cnpj) caro). Retorna o t0 para quem
// chama medir o tempo total combinado com fases seguintes.
func prewarmAggMesCore(db *sql.DB, empresaID string) time.Time {
	t0 := time.Now()

	anoAtual := time.Now().Year()
	ymAtual := anoAtual*100 + int(time.Now().Month())
	ymAnt := (anoAtual-1)*100 + 12 // dezembro ano anterior (YTD completo)

	// 3 views × 2 fluxos × 2 períodos = 12 queries pequenas.
	// Como são em paralelo e rodam após o usuário já ter recebido resposta,
	// competem pouco com queries de usuário real.
	views := []struct {
		leaf, group string
	}{
		{"agg_fat_v01_l0_mes", "cod_fornec"},
		{"agg_fat_v02_l0_mes", "cod_supervisor"},
		{"agg_fat_v03_l0_mes", "cod_gerente"},
		{"agg_trans_v01_l0_mes", "cod_fornec"},
		{"agg_trans_v02_l0_mes", "cod_supervisor"},
		{"agg_trans_v03_l0_mes", "cod_gerente"},
	}
	var wg sync.WaitGroup
	for _, v := range views {
		for _, ym := range []int{ymAtual, ymAnt} {
			wg.Add(1)
			v, ym := v, ym
			go func() {
				defer wg.Done()
				_, _ = db.Exec(fmt.Sprintf(`
					SELECT %s, COUNT(*) FROM farol.%s
					WHERE empresa_id=$1 AND (ano*100+mes) <= $2
					GROUP BY %s`, v.group, v.leaf, v.group),
					empresaID, ym)
			}()
		}
	}
	// Aquece a folha (positivados/base_cli) — top-level de cada view × 2 fluxos.
	//
	// ANTES: rodava um db.Exec cru com a MESMA query que queryDistinctPositivados
	// faria, mas jogava o resultado fora — só esquentava o disco/shared_buffers
	// do Postgres, não o baseCache da aplicação (mapa em memória). A 1ª request
	// REAL de cada view pagava a reexecução inteira do zero: 25-34s no V01/V02
	// logo após um import grande (COUNT DISTINCT cnpj em toda a história, sem
	// filtro de período — é o "base" de queryBasePositivados, ymStart=0..999912).
	//
	// AGORA: chama queryBasePositivados de verdade, que por baixo passa por
	// cachedDistinctPositivados e GRAVA no baseCache com a mesma chave que uma
	// request real vai procurar. A 1ª visualização do usuário vira cache HIT.
	posViews := []struct{ view, group string }{
		{"V01", "cod_fornec"},
		{"V02", "cod_supervisor"},
		{"V03", "cod_gerente"},
	}
	posFluxos := []fluxoCtx{resolveFluxo("faturado"), resolveFluxo("transmitido")}
	for _, fl := range posFluxos {
		for _, v := range posViews {
			wg.Add(1)
			fl, v := fl, v
			go func() {
				defer wg.Done()
				queryBasePositivados(db, empresaID, fl, v.view, v.group, nil, nil)
			}()
		}
	}
	wg.Wait()
	log.Printf("[farol:view] prewarmAggMes MVs empresa=%s em %v", empresaID, time.Since(t0))
	return t0
}

// ymRange — período de cache em ym (ano*100+mes), inclusivo nas duas pontas.
type ymRange struct{ ini, fim int }

// periodosComuns — os intervalos que o painel efetivamente pede ao abrir.
// prewarmAggMesCore só aquece a chave "histórico completo" (ymStart=0), mas
// fetchCards consulta TAMBÉM o período de referência e o de comparação
// (cachedDistinctPositivados com ym reais). Eram justamente essas chaves que
// ficavam frias: no log de 27/07/2026 o login pagou 5,7s/6,6s/10,1s em
// Indústrias mesmo com o prewarm de boot já concluído.
func periodosComuns(agora time.Time) []ymRange {
	ano, mes := agora.Year(), int(agora.Month())
	ymAtual := ano*100 + mes
	ymMesAnt := ymAtual - 1
	if mes == 1 {
		ymMesAnt = (ano-1)*100 + 12
	}
	return []ymRange{
		{ano*100 + 1, ymAtual},              // YTD do ano corrente
		{(ano-1)*100 + 1, (ano-1)*100 + 12}, // ano anterior completo (comparativo do YTD)
		{ymAtual, ymAtual},                  // mês corrente
		{ymAtual - 100, ymAtual - 100},      // mesmo mês do ano anterior
		{ymMesAnt, ymMesAnt},                // mês anterior (fechado)
		{ymMesAnt - 100, ymMesAnt - 100},    // mês anterior do ano anterior
	}
}

// prewarmPeriodKeys aquece o baseCache dos períodos de referência/comparação.
// Complementa prewarmAggMesCore (que cobre só o "histórico completo").
func prewarmPeriodKeys(db *sql.DB, empresaID string) {
	t0 := time.Now()
	posViews := []struct{ view, group string }{
		{"V01", "cod_fornec"},
		{"V02", "cod_supervisor"},
		{"V03", "cod_gerente"},
	}
	fluxos := []fluxoCtx{resolveFluxo("faturado"), resolveFluxo("transmitido")}
	periodos := periodosComuns(time.Now())

	const maxParallel = 4
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for _, fl := range fluxos {
		for _, v := range posViews {
			for _, p := range periodos {
				wg.Add(1)
				fl, v, p := fl, v, p
				go func() {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					cachedDistinctPositivados(db, empresaID, fl, v.view, v.group, p.ini, p.fim, nil, nil)
				}()
			}
		}
	}
	wg.Wait()
	log.Printf("[farol:view] prewarmPeriodKeys empresa=%s (%d períodos × %d views × %d fluxos) em %v",
		empresaID, len(periodos), len(posViews), len(fluxos), time.Since(t0))
}

// PrewarmDiario — aquecimento COMPLETO, pensado para rodar de madrugada/início
// da manhã (StartDailyPrewarm), quando não há usuários. Diferente do
// PrewarmStartup, que é deliberadamente enxuto para não competir com tráfego
// real no boot, aqui vale pagar o custo inteiro: base + períodos + presets
// diários. Com o TTL de 20h, o que é aquecido às 07:30 cobre o expediente todo.
func PrewarmDiario(db *sql.DB, empresaID string) {
	t0 := time.Now()
	prewarmAggMesCore(db, empresaID)
	prewarmPeriodKeys(db, empresaID)
	prewarmDailyRanges(db, empresaID)
	log.Printf("[farol:view] PrewarmDiario empresa=%s COMPLETO em %v", empresaID, time.Since(t0))
}

// prewarmHoraDiaria devolve a hora/minuto do aquecimento diário. Default 07:30
// (antes do expediente e depois da janela de carga automática, 00:01-06:00).
// Ajustável por FAROL_PREWARM_HORA no formato "HH:MM".
func prewarmHoraDiaria() (hora, minuto int) {
	hora, minuto = 7, 30
	v := strings.TrimSpace(os.Getenv("FAROL_PREWARM_HORA"))
	if v == "" {
		return hora, minuto
	}
	var h, m int
	if _, err := fmt.Sscanf(v, "%d:%d", &h, &m); err != nil ||
		h < 0 || h > 23 || m < 0 || m > 59 {
		log.Printf("[farol:view] FAROL_PREWARM_HORA=%q inválido, usando %02d:%02d", v, hora, minuto)
		return hora, minuto
	}
	return h, m
}

// StartDailyPrewarm roda PrewarmDiario todo dia no horário configurado, para
// TODAS as empresas. Motivação (27/07/2026): com a carga automática diária
// entrando em produção, toda madrugada o import invalida o cache dos meses que
// tocou; sem um aquecimento agendado, o primeiro gestor a abrir o painel de
// manhã pagaria 10-25s por view. O horário local do container é America/
// Sao_Paulo (definido no Dockerfile), mas a localização é resolvida explicita-
// mente para não depender disso.
//
// Chamar com `go handlers.StartDailyPrewarm(db)` — bloqueia para sempre.
func StartDailyPrewarm(db *sql.DB) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.Local
		log.Printf("[farol:view] prewarm diário: tz America/Sao_Paulo indisponível (%v), usando hora local", err)
	}
	hora, minuto := prewarmHoraDiaria()
	log.Printf("[farol:view] prewarm diário agendado para %02d:%02d (%s)", hora, minuto, loc)

	for {
		agora := time.Now().In(loc)
		prox := time.Date(agora.Year(), agora.Month(), agora.Day(), hora, minuto, 0, 0, loc)
		if !prox.After(agora) {
			prox = prox.AddDate(0, 0, 1)
		}
		espera := time.Until(prox)
		log.Printf("[farol:view] prewarm diário: próxima execução %s (em %s)",
			prox.Format("2006-01-02 15:04"), espera.Truncate(time.Minute))
		time.Sleep(espera)

		empresas, err := listCompanyIDs(db)
		if err != nil {
			log.Printf("[farol:view] prewarm diário: falha ao listar empresas: %v", err)
			continue
		}
		t0 := time.Now()
		for _, id := range empresas {
			PrewarmDiario(db, id)
		}
		log.Printf("[farol:view] prewarm diário CONCLUÍDO: %d empresa(s) em %v", len(empresas), time.Since(t0))
	}
}

// listCompanyIDs — empresas ativas, para os jobs que varrem todas.
func listCompanyIDs(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT id::text FROM companies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// prewarmDailyRanges chama queryAggregatedVendas para os presets diários comuns
// (dia anterior, 7d, 30d) em todas as views L0 × 2 fluxos. Popula o cache de Q1
// para que o primeiro clique do usuário seja instantâneo (Q1 hit <100µs).
//
// Concorrência limitada via semaphore (semaphoreChan) para não saturar o pool
// de conexões. 6 em paralelo = mesmo padrão dos dims.
func prewarmDailyRanges(db *sql.DB, empresaID string) {
	today := time.Now().UTC()
	yesterday := today.AddDate(0, 0, -1)

	ranges := []struct {
		nome               string
		iniAtual, fimAtual time.Time
		iniComp, fimComp   time.Time
	}{
		{"dia_anterior", yesterday, yesterday, yesterday.AddDate(0, 0, -7), yesterday.AddDate(0, 0, -7)},
		{"7d", today.AddDate(0, 0, -6), today, today.AddDate(0, 0, -13), today.AddDate(0, 0, -7)},
		{"30d", today.AddDate(0, 0, -29), today, today.AddDate(0, 0, -59), today.AddDate(0, 0, -30)},
	}

	type vc struct{ view, group, name string }
	views := []vc{
		{"V01", "cod_fornec", "nome_fornec"},
		{"V02", "cod_supervisor", "nome_supervisor"},
		{"V03", "cod_gerente", "nome_gerente"},
	}
	fluxos := []fluxoCtx{resolveFluxo("faturado"), resolveFluxo("transmitido")}

	const maxParallel = 6
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for _, fl := range fluxos {
		for _, v := range views {
			for _, r := range ranges {
				fl, v, r := fl, v, r
				// Atual
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem; wg.Done() }()
					queryAggregatedVendas(db, empresaID, fl, v.view, v.group, v.name, r.iniAtual, r.fimAtual, nil, nil)
				}()
				// Comp
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem; wg.Done() }()
					queryAggregatedVendas(db, empresaID, fl, v.view, v.group, v.name, r.iniComp, r.fimComp, nil, nil)
				}()
			}
		}
	}
	wg.Wait()
}

// ─── FarolV2PeriodosHandler ──────────────────────────────────────────────────

func FarolV2PeriodosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		periodos := fetchPeriodosDisponiveis(db, spCtx.EmpresaID)
		json.NewEncoder(w).Encode(map[string]any{"periodos": periodos})
	}
}

// ─── FarolV2DimsHandler — GET /api/v2/farol/dims ────────────────────────────
//
// Retorna as opções disponíveis em cada dimensão, dentro do período + fluxo
// escolhido. Alimenta os multi-selects da UI de filtros.
//
//   GET /api/v2/farol/dims?fluxo=faturado&ref_inicio=2026-05-01&ref_fim=2026-05-31
//
// Resposta:
//   {
//     "fornec":     [{"key":"F01","label":"NESTLE BRASIL"}, ...],
//     "gerente":    [...],
//     "supervisor": [...],
//     "rca":        [...],
//     "cli":        [...],
//     "uf":         ["SP", "RJ", ...],
//     "empresa":    ["NORDESTE", "SUDESTE", ...]
//   }

type dimOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

func FarolV2DimsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		fluxo := resolveFluxo(q.Get("fluxo"))
		pr := resolvePeriods(db, spCtx.EmpresaID, q)
		if pr.RefInicio.IsZero() {
			json.NewEncoder(w).Encode(map[string]any{})
			return
		}

		// Lookup em agg_*_dims_mes (mig 165): consolidada por dim/key, usa label do mês mais recente.
		fluxoPrefix := "fat"
		if fluxo.name == "transmitido" {
			fluxoPrefix = "trans"
		}
		dimsTable := "farol.agg_" + fluxoPrefix + "_dims_mes"
		t0 := time.Now()

		// dimLevelSrc — tabela agg de nível (granularidade da própria dim) usada
		// para filtrar do dropdown opções SEM movimento real. dims_mes lista todo
		// código que apareceu em vendas_* (mesmo só com devolução/bonificação ou
		// valor zero) → poluía o filtro com RCAs/etc que não trazem nada na tela.
		// Mantém só keys com pvenda<>0 OU qt>0 em algum mês. {tableTmpl, codCol}.
		dimLevelSrc := map[string][2]string{
			"fornec":     {"agg_%s_v01_l0_mes", "cod_fornec"},
			"gerente":    {"agg_%s_v03_l0_mes", "cod_gerente"},
			"supervisor": {"agg_%s_v02_l0_mes", "cod_supervisor"},
			"rca":        {"agg_%s_v04_l0_mes", "cod_rca"},
			"cli":        {"agg_%s_v01_l4_mes", "cod_cli"},
		}

		// fetchDim(dimName) — retorna [{key, label}] para a dim solicitada.
		// codCol é só pro log; dimName mapeia "cod_X" → "X" (ex: cod_fornec → fornec).
		fetchDim := func(codCol, dimName string) []dimOption {
			td := time.Now()
			comSemMov := ""
			if src, ok := dimLevelSrc[dimName]; ok {
				lvl := fmt.Sprintf("farol."+src[0], fluxoPrefix)
				comSemMov = fmt.Sprintf(`
				   AND d.key IN (SELECT a.%s FROM %s a
				                  WHERE a.empresa_id=$1 AND (a.pvenda <> 0 OR a.qt > 0))`,
					src[1], lvl)
			}
			// ORDER BY label para todas as dims exceto cli (34k+ registros)
			orderClause := "ORDER BY label"
			if dimName == "cli" {
				orderClause = "" // clientes não precisam de ordenação alfabética
			}
			rows, err := db.Query(fmt.Sprintf(`
				SELECT d.key, MAX(d.label) AS label
				  FROM %s d
				 WHERE d.empresa_id=$1 AND d.dim=$2 AND d.key != ''%s
				 GROUP BY d.key
				 %s
			`, dimsTable, comSemMov, orderClause), spCtx.EmpresaID, dimName)
			if err != nil {
				log.Printf("[dims] %s ERRO em %v: %v", codCol, time.Since(td), err)
				return nil
			}
			defer rows.Close()
			out := []dimOption{}
			excluidos := 0
			for rows.Next() {
				var d dimOption
				if rows.Scan(&d.Key, &d.Label) == nil {
					// Sem nome na origem (vendas_*) → fora do filtro. São códigos
					// técnicos do WinThor (sem cadastro de vendedor/cliente) que só
					// confundem o gestor e não produzem card consistente.
					if strings.TrimSpace(d.Label) == "" {
						excluidos++
						continue
					}
					out = append(out, d)
				}
			}
			if excluidos > 0 {
				log.Printf("[dims] %s → %d opções (%d sem nome EXCLUÍDOS do filtro)", codCol, len(out), excluidos)
			}
			log.Printf("[dims] %s → %d opções em %v", codCol, len(out), time.Since(td))
			return out
		}

		// fetchScalar(dimName) — só keys (sem label), usado para uf/empresa
		fetchScalar := func(col, dimName string) []string {
			td := time.Now()
			rows, err := db.Query(fmt.Sprintf(`
				SELECT DISTINCT key FROM %s WHERE empresa_id=$1 AND dim=$2 AND key != '' ORDER BY key
			`, dimsTable), spCtx.EmpresaID, dimName)
			if err != nil {
				log.Printf("[dims] %s ERRO em %v: %v", col, time.Since(td), err)
				return nil
			}
			defer rows.Close()
			out := []string{}
			for rows.Next() {
				var v string
				if rows.Scan(&v) == nil {
					out = append(out, v)
				}
			}
			log.Printf("[dims] %s → %d valores em %v", col, len(out), time.Since(td))
			return out
		}

		// LAZY-LOAD do cod_cli: a dim "cli" (34k+ clientes) é a mais cara (~3s) e
		// raramente filtrada. Ela só é carregada quando o front pede ?dim=cli
		// (ao abrir o dropdown de Cliente). O dims "padrão" (sem ?dim) carrega
		// todas as OUTRAS dims rápidas, sem o cli — tira ~3s de todo login.
		onlyDim := strings.ToLower(strings.TrimSpace(q.Get("dim")))
		if onlyDim == "cli" {
			cli := fetchDim("cod_cli", "cli")
			log.Printf("[dims] fluxo=%s dim=cli total=%v", fluxo.name, time.Since(t0))
			json.NewEncoder(w).Encode(map[string]any{"cli": cli})
			return
		}

		var (
			fornec, gerente, supervisor, rca []dimOption
			uf, empresa                      []string
			wg                               sync.WaitGroup
		)
		wg.Add(6)
		go func() { defer wg.Done(); fornec = fetchDim("cod_fornec", "fornec") }()
		go func() { defer wg.Done(); gerente = fetchDim("cod_gerente", "gerente") }()
		go func() { defer wg.Done(); supervisor = fetchDim("cod_supervisor", "supervisor") }()
		go func() { defer wg.Done(); rca = fetchDim("cod_rca", "rca") }()
		go func() { defer wg.Done(); uf = fetchScalar("uf", "uf") }()
		go func() { defer wg.Done(); empresa = fetchScalar("empresa", "empresa") }()
		wg.Wait()

		log.Printf("[dims] fluxo=%s paralelo=6 (sem cli) total=%v", fluxo.name, time.Since(t0))

		resp := map[string]any{
			"fornec":     fornec,
			"gerente":    gerente,
			"supervisor": supervisor,
			"rca":        rca,
			"cli":        []dimOption{}, // lazy: carregado via ?dim=cli ao abrir dropdown
			"uf":         uf,
			"empresa":    empresa,
		}
		// tipo_venda (mig 187/188) — só no fluxo faturado. Poucos códigos, fetch
		// síncrono barato; rótulo já vem de farol.tipo_venda_label no dims_mes.
		if fluxo.name == "faturado" {
			resp["tipo_venda"] = fetchDim("tipo_venda", "tipo_venda")
		}
		json.NewEncoder(w).Encode(resp)
	}
}

// ─── Acesso público ION VENDAS ───────────────────────────────────────────────

// resolveEmpresaCNPJ resolve empresa_id a partir do CNPJ (dígitos, com ou sem máscara).
func resolveEmpresaCNPJ(db *sql.DB, cnpj string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, cnpj)
	if digits == "" {
		return ""
	}
	var id string
	_ = db.QueryRow(
		`SELECT id FROM companies WHERE regexp_replace(cnpj, '[^0-9]', '', 'g') = $1 LIMIT 1`,
		digits,
	).Scan(&id)
	return id
}

// codToDimName mapeia "cod_X" → nome da dim na agg_*_dims_mes
var codToDimName = map[string]string{
	"cod_fornec":     "fornec",
	"cod_gerente":    "gerente",
	"cod_supervisor": "supervisor",
	"cod_rca":        "rca",
	"cod_cli":        "cli",
}

// lookupNome busca o nome (label) de um código em agg_*_dims_mes (FAT + TRANS).
// nomeCol é ignorado (parâmetro legado mantido p/ compat de assinatura);
// usa o mês mais recente disponível como source of truth.
func lookupNome(db *sql.DB, empresaID, codCol, nomeCol, cod string) string {
	_ = nomeCol
	if cod == "" {
		return ""
	}
	dim, ok := codToDimName[codCol]
	if !ok {
		return cod
	}
	var nome string
	_ = db.QueryRow(`
		SELECT label FROM farol.agg_fat_dims_mes
		WHERE empresa_id=$1 AND dim=$2 AND key=$3 AND label != ''
		ORDER BY ano DESC, mes DESC LIMIT 1
	`, empresaID, dim, cod).Scan(&nome)
	if nome == "" {
		_ = db.QueryRow(`
			SELECT label FROM farol.agg_trans_dims_mes
			WHERE empresa_id=$1 AND dim=$2 AND key=$3 AND label != ''
			ORDER BY ano DESC, mes DESC LIMIT 1
		`, empresaID, dim, cod).Scan(&nome)
	}
	if nome == "" {
		nome = cod
	}
	return nome
}

// lookupParent descobre o código pai de um código (ex: supervisor de um RCA).
// Usa as tabelas agg_*_v0X_lY_mes que carregam hierarquia adjacente:
//
//	cod_rca → cod_supervisor   via agg_*_v02_l1_mes  (chave (sup, rca))
//	cod_supervisor → cod_gerente via agg_*_v03_l1_mes (chave (ger, sup))
//	cod_rca → cod_gerente      via agg_*_v01_l3_mes  (chave (forn, ger, sup, rca))
func lookupParent(db *sql.DB, empresaID, codCol, cod, parentCol string) string {
	if cod == "" {
		return ""
	}
	type query struct{ table, colCod, colParent string }
	var qs []query
	switch {
	case codCol == "cod_rca" && parentCol == "cod_supervisor":
		qs = []query{
			{"farol.agg_fat_v02_l1_mes", "cod_rca", "cod_supervisor"},
			{"farol.agg_trans_v02_l1_mes", "cod_rca", "cod_supervisor"},
		}
	case codCol == "cod_supervisor" && parentCol == "cod_gerente":
		qs = []query{
			{"farol.agg_fat_v03_l1_mes", "cod_supervisor", "cod_gerente"},
			{"farol.agg_trans_v03_l1_mes", "cod_supervisor", "cod_gerente"},
		}
	case codCol == "cod_rca" && parentCol == "cod_gerente":
		qs = []query{
			{"farol.agg_fat_v01_l3_mes", "cod_rca", "cod_gerente"},
			{"farol.agg_trans_v01_l3_mes", "cod_rca", "cod_gerente"},
		}
	default:
		return ""
	}
	var p string
	for _, q := range qs {
		_ = db.QueryRow(fmt.Sprintf(
			`SELECT %s FROM %s WHERE empresa_id=$1 AND %s=$2 AND %s != '' LIMIT 1`,
			q.colParent, q.table, q.colCod, q.colParent,
		), empresaID, cod).Scan(&p)
		if p != "" {
			return p
		}
	}
	return p
}

// FarolV2PublicCardsHandler — GET /api/v2/farol/public/cards (SEM auth)
//
//	cnpj, scope (sup|rca), cod  → escopo fixo; drill adicional opcional.
//	fluxo, ref_inicio/ref_fim, comp_inicio/comp_fim (ou ano/mes legados).
func FarolV2PublicCardsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()

		rawCNPJ := q.Get("cnpj")
		empresaID := resolveEmpresaCNPJ(db, rawCNPJ)
		if empresaID == "" {
			log.Printf("[farol:public] empresa não encontrada — cnpj=%q scope=%q cod=%q", rawCNPJ, q.Get("scope"), q.Get("cod"))
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "empresa não encontrada para este CNPJ",
				"cnpj":  rawCNPJ,
				"hint":  "verifique se companies.cnpj está preenchido para esta empresa",
			})
			return
		}

		scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
		cod := strings.TrimSpace(q.Get("cod"))
		if cod == "" || (scope != "sup" && scope != "rca") {
			log.Printf("[farol:public] params inválidos — cnpj=%q scope=%q cod=%q", rawCNPJ, scope, cod)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "scope (sup|rca) e cod obrigatórios",
				"scope": scope, "cod": cod,
			})
			return
		}
		// Aceita view=V02 (Por RCA, default) ou V05 (Por Fornecedor).
		// Ambas começam em cod_supervisor — o escopo público é sempre o supervisor.
		view := strings.ToUpper(strings.TrimSpace(q.Get("view")))
		if view != "V02" && view != "V05" {
			view = "V02"
		}
		log.Printf("[farol:public] cnpj=%q → empresa=%s scope=%s cod=%s view=%s", rawCNPJ, empresaID, scope, cod, view)
		hier := hierarquias[view]
		fluxo := resolveFluxo(q.Get("fluxo"))

		// Drill base fixado pela URL (não pode ser removido pelo usuário).
		var baseDrill []drillStep
		switch scope {
		case "sup":
			baseDrill = []drillStep{
				{Level: "cod_supervisor", Value: cod, Label: lookupNome(db, empresaID, "cod_supervisor", "nome_supervisor", cod)},
			}
		case "rca":
			sup := lookupParent(db, empresaID, "cod_rca", cod, "cod_supervisor")
			baseDrill = []drillStep{
				{Level: "cod_supervisor", Value: sup, Label: lookupNome(db, empresaID, "cod_supervisor", "nome_supervisor", sup)},
				{Level: "cod_rca", Value: cod, Label: lookupNome(db, empresaID, "cod_rca", "nome_rca", cod)},
			}
		}

		var userDrill []drillStep
		if dj := q.Get("drill"); dj != "" {
			_ = json.Unmarshal([]byte(dj), &userDrill)
		}
		drillPath := append(baseDrill, userDrill...)
		drillIdx := len(drillPath)

		if drillIdx >= len(hier) {
			json.NewEncoder(w).Encode(cardsResponse{Cards: []cardItem{}, DrillPath: drillPath, View: view})
			return
		}
		currentLevel := hier[drillIdx]

		pr := resolvePeriods(db, empresaID, q)
		if pr.RefInicio.IsZero() {
			json.NewEncoder(w).Encode(cardsResponse{Cards: []cardItem{}, View: view, DrillPath: drillPath})
			return
		}

		filters := parseMultiFilters(q)
		// tipo_venda só existe no fluxo faturado (ver handler principal).
		if fluxo.name != "faturado" {
			delete(filters, "tipo_venda")
		}
		cards, diag := fetchCards(db, empresaID, fluxo, view, pr, drillIdx, currentLevel, drillPath, filters)
		kpi := computeKPI(cards, fluxo.name, currentLevel.Level == "cod_fornec")
		if currentLevel.Level != "cod_prod" && currentLevel.Level != "cod_cli" &&
			leafServesPositivados(fluxo, view, currentLevel.Level, drillPath, filters) {
			fixOverlappingBaseKPI(db, &kpi, fluxo, view, empresaID, pr, drillPath, filters)
		}
		curLabel, antLabel, plabel := buildPeriodoLabels(pr)

		sort.Slice(cards, func(i, j int) bool {
			if cards[i].Cor != cards[j].Cor {
				return cards[i].Cor == "vermelho"
			}
			return cards[i].ValorAtual > cards[j].ValorAtual
		})

		json.NewEncoder(w).Encode(cardsResponse{
			Cards: cards,
			KPI:   kpi,
			Periodo: periodoInfo{
				Fluxo:      fluxo.name,
				RefInicio:  pr.RefInicio.Format("2006-01-02"),
				RefFim:     pr.RefFim.Format("2006-01-02"),
				CompInicio: fmtDateOrEmpty(pr.CompInicio),
				CompFim:    fmtDateOrEmpty(pr.CompFim),
				Label:      plabel,
				CurLabel:   curLabel,
				AntLabel:   antLabel,
				RefAno:     pr.RefAno, RefMes: pr.RefMes,
				CompMode: pr.CompMode, CompAno: pr.CompAno, CompMes: pr.CompMes,
			},
			Periodos:       fetchPeriodosDisponiveis(db, empresaID),
			View:           view,
			DrillPath:      drillPath,
			NextLevel:      currentLevel.Level,
			NextLevelLabel: currentLevel.Label,
			Diag:           diag,
		})
	}
}
