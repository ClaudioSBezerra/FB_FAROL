package handlers

// farol_segmento_foco.go — Programa "Segmento Foco" (Reckitt), 19/10/2026.
//
// Positivação de GRUPOS de produtos por cliente. Um cliente positiva um grupo
// no mês quando compra o mínimo de 3 unidades de QUALQUER produto do grupo
// (a regra por produto reproduziu 100% das flags que o fornecedor calcula na
// planilha dele, em 7 dos 8 grupos de setembro/2026; somar as unidades do
// grupo erra mais). Caixa conta pelas unidades: embalagem em caixa positiva
// com 1 unidade vendida (mesma regra do Sortimento Numérica/PPA —
// exigeQuantidadeMinima). Tipos de venda 1, 5 e 9 (vêm do vínculo); a
// bonificação (5) positiva mesmo com valor R$ 0 porque a regra é por
// quantidade, nunca por valor.
//
// Realizado = nº de (cliente × grupo) positivados no escopo. Objetivo por UF ×
// grupo × mês (tabela metas_grupos_objetivos). Trimestre = soma dos 3 meses
// do trimestre civil (um cliente positivado em meses diferentes conta mais de
// uma vez). A UF do cliente é a da MATRIZ (cod_cliprinc) — se o cliente abre
// loja em outra UF, entra na UF da matriz (decisão do Claudio, 19/10/2026).
//
// O fluxo (Faturado, Emitido, Faturado+Emitido) vem como nos demais painéis; no
// Faturado abate devolução/cancelamento (vendas_ccd), igual ao resto do
// sistema. Sem snapshot por enquanto: a conta roda ao vivo (~40 mil linhas de
// venda por mês do fornecedor) com cache curto em memória.
//
// Rotas (sp_role >= somente_leitura, escopo GGV/CRV/RCA do login):
//   GET /api/farol/segmento-foco          ?vinculo_id=&mes=YYYY-MM&fluxo=
//   GET /api/farol/segmento-foco/detalhe  ?vinculo_id=&mes=&periodo=mes|trimestre&fluxo=&uf=&cod_ggv=&cod_crv=&cod_rca=&cod_cli=

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

// ─── Tipos ────────────────────────────────────────────────────────────────────

type SegProduto struct {
	Grupo     string  `json:"grupo"`
	CodProd   string  `json:"cod_prod"`
	Produto   string  `json:"produto"`
	Qtd       float64 `json:"qtd"`
	Positivou bool    `json:"positivou"`
}

type segCliente struct {
	CodCli                                            string
	Nome                                              string
	UF                                                string
	CodGGV, NomeGGV, CodCRV, NomeCRV, CodRCA, NomeRCA string
	CodCliPrinc                                       string
	Produtos                                          []SegProduto
	Positivos                                         map[string]bool // grupo → positivou
}

type segGrupoProd struct {
	Grupo   string
	Produto string
}

// segDados — tudo que o painel precisa de UM mês (uma vigência) e UM fluxo.
type segDados struct {
	Grupos   []string // ordem de exibição
	Clientes []*segCliente
}

var (
	segCacheMu sync.Mutex
	segCache   = map[string]segCacheEntry{}
)

type segCacheEntry struct {
	dados *segDados
	at    time.Time
}

const segCacheTTL = 2 * time.Minute

func segInvalidarCache() {
	segCacheMu.Lock()
	segCache = map[string]segCacheEntry{}
	segCacheMu.Unlock()
}

// ─── Motor ────────────────────────────────────────────────────────────────────

func segLerGrupos(db *sql.DB, empresaID string, vigenciaID int) (map[string]segGrupoProd, []string, error) {
	rows, err := db.Query(`SELECT cod_prod, grupo, produto FROM farol.metas_grupos_produtos WHERE vigencia_id = $1 AND empresa_id = $2 ORDER BY id`, vigenciaID, empresaID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	porProd := map[string]segGrupoProd{}
	var ordem []string
	vistos := map[string]bool{}
	for rows.Next() {
		var cod, grupo, produto string
		if err := rows.Scan(&cod, &grupo, &produto); err != nil {
			return nil, nil, err
		}
		porProd[cod] = segGrupoProd{Grupo: grupo, Produto: produto}
		if !vistos[grupo] {
			vistos[grupo] = true
			ordem = append(ordem, grupo)
		}
	}
	return porProd, ordem, rows.Err()
}

// segCalcular monta, pra uma vigência (mês) e um fluxo, quem comprou o quê e
// quais grupos cada cliente positivou.
func segCalcular(db *sql.DB, empresaID string, vinculoID, vigenciaID int, fluxo string) (*segDados, error) {
	chave := fmt.Sprintf("%s|%d|%d|%s", empresaID, vinculoID, vigenciaID, fluxo)
	segCacheMu.Lock()
	if e, ok := segCache[chave]; ok && time.Since(e.at) < segCacheTTL {
		segCacheMu.Unlock()
		return e.dados, nil
	}
	segCacheMu.Unlock()

	var dataInicio, dataFim string
	var industriaID int
	if err := db.QueryRow(`
		SELECT v.data_inicio::text, v.data_fim::text, mv.industria_id
		FROM farol.metas_vigencias v JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		WHERE v.id = $1 AND v.vinculo_id = $2 AND v.empresa_id = $3`, vigenciaID, vinculoID, empresaID).Scan(&dataInicio, &dataFim, &industriaID); err != nil {
		return nil, fmt.Errorf("vigência não encontrada")
	}
	var tiposVenda []string
	if err := scanTiposVendaValidos(db, empresaID, vinculoID, &tiposVenda); err != nil {
		return nil, err
	}
	codFornec, err := codFornecDaIndustria(db, empresaID, industriaID)
	if err != nil {
		return nil, err
	}
	porProd, ordem, err := segLerGrupos(db, empresaID, vigenciaID)
	if err != nil {
		return nil, err
	}
	if len(porProd) == 0 {
		return nil, fmt.Errorf("nenhum produto de grupo importado pra esta vigência")
	}
	codProds := make([]string, 0, len(porProd))
	for c := range porProd {
		codProds = append(codProds, c)
	}

	type chaveCP struct{ cli, prod string }
	type acum struct {
		qtd       float64
		embalagem string
		qtUnitCx  float64
	}
	soma := map[chaveCP]*acum{}
	clientes := map[string]*segCliente{}

	somar := func(tabela, colData string) error {
		q := fmt.Sprintf(`
			SELECT v.cod_cli, v.cod_prod, SUM(v.qt), MAX(v.embalagem), COALESCE(MAX(v.qt_unit_cx),0),
			       MAX(v.nome_cli), MAX(v.uf), MAX(v.cod_cliprinc),
			       MAX(v.cod_gerente), MAX(v.nome_gerente), MAX(v.cod_supervisor), MAX(v.nome_supervisor), MAX(v.cod_rca), MAX(v.nome_rca)
			FROM %s v
			WHERE v.empresa_id = $1 AND v.%s BETWEEN $2 AND $3 AND v.cod_prod = ANY($4)`, tabela, colData)
		args := []any{empresaID, dataInicio, dataFim, pq.Array(codProds)}
		if len(tiposVenda) > 0 {
			args = append(args, pq.Array(tiposVenda))
			q += fmt.Sprintf(" AND v.tipo_venda = ANY($%d)", len(args))
		}
		if len(codFornec) > 0 {
			args = append(args, pq.Array(codFornec))
			q += fmt.Sprintf(" AND v.cod_fornec = ANY($%d)", len(args))
		}
		q += " GROUP BY v.cod_cli, v.cod_prod"
		rows, err := db.Query(q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var cli, prod, emb, nome, uf, princ, gger, ngger, gsup, nsup, grca, nrca string
			var qtd, qtUnitCx float64
			if err := rows.Scan(&cli, &prod, &qtd, &emb, &qtUnitCx, &nome, &uf, &princ, &gger, &ngger, &gsup, &nsup, &grca, &nrca); err != nil {
				return err
			}
			k := chaveCP{cli, prod}
			a := soma[k]
			if a == nil {
				a = &acum{embalagem: emb, qtUnitCx: qtUnitCx}
				soma[k] = a
			}
			a.qtd += qtd
			c := clientes[cli]
			if c == nil {
				c = &segCliente{CodCli: cli, Nome: nome, UF: uf, CodCliPrinc: princ,
					CodGGV: gger, NomeGGV: ngger, CodCRV: gsup, NomeCRV: nsup, CodRCA: grca, NomeRCA: nrca,
					Positivos: map[string]bool{}}
				clientes[cli] = c
			}
		}
		return rows.Err()
	}
	abater := func() error {
		q := `SELECT v.cod_cli, v.cod_prod, SUM(v.qt) FROM vendas_ccd v
			WHERE v.empresa_id = $1 AND v.data_evento BETWEEN $2 AND $3 AND v.cod_prod = ANY($4)
			  AND v.evento IN ('DEVOLVIDO','CANCELADO')`
		args := []any{empresaID, dataInicio, dataFim, pq.Array(codProds)}
		if len(codFornec) > 0 {
			args = append(args, pq.Array(codFornec))
			q += fmt.Sprintf(" AND v.cod_fornec = ANY($%d)", len(args))
		}
		q += " GROUP BY v.cod_cli, v.cod_prod"
		rows, err := db.Query(q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var cli, prod string
			var qtd float64
			if err := rows.Scan(&cli, &prod, &qtd); err != nil {
				return err
			}
			if a := soma[chaveCP{cli, prod}]; a != nil {
				a.qtd -= qtd
			}
		}
		return rows.Err()
	}

	switch fluxo {
	case "faturado", "":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
		if err := abater(); err != nil {
			return nil, err
		}
	case "transmitido":
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return nil, err
		}
	case "soma":
		if err := somar("vendas_faturadas", "data_faturamento"); err != nil {
			return nil, err
		}
		if err := abater(); err != nil {
			return nil, err
		}
		if err := somar("vendas_transmitidas", "data_transmissao"); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("fluxo inválido: %q (use faturado, transmitido ou soma)", fluxo)
	}

	// Produtos e positivação por grupo.
	for k, a := range soma {
		if a.qtd <= 0 {
			continue
		}
		gp := porProd[k.prod]
		pos := a.qtd >= 1
		if exigeQuantidadeMinima(a.embalagem, a.qtUnitCx) {
			pos = a.qtd >= 3
		}
		c := clientes[k.cli]
		c.Produtos = append(c.Produtos, SegProduto{Grupo: gp.Grupo, CodProd: k.prod, Produto: gp.Produto, Qtd: a.qtd, Positivou: pos})
		if pos {
			c.Positivos[gp.Grupo] = true
		}
	}

	// UF da matriz: cliente que abre loja em outra UF entra na UF da matriz.
	matrizes := map[string]bool{}
	for _, c := range clientes {
		if c.CodCliPrinc != "" && c.CodCliPrinc != c.CodCli {
			matrizes[c.CodCliPrinc] = true
		}
	}
	if len(matrizes) > 0 {
		lista := make([]string, 0, len(matrizes))
		for m := range matrizes {
			lista = append(lista, m)
		}
		ini, _ := time.Parse("2006-01-02", dataInicio)
		rows, err := db.Query(`SELECT cod_cli, MAX(uf) FROM vendas_faturadas
			WHERE empresa_id = $1 AND cod_cli = ANY($2) AND data_faturamento >= $3 GROUP BY cod_cli`,
			empresaID, pq.Array(lista), ini.AddDate(0, 0, -180).Format("2006-01-02"))
		if err == nil {
			ufMatriz := map[string]string{}
			for rows.Next() {
				var cod, uf string
				if rows.Scan(&cod, &uf) == nil && uf != "" {
					ufMatriz[cod] = uf
				}
			}
			rows.Close()
			for _, c := range clientes {
				if uf, ok := ufMatriz[c.CodCliPrinc]; ok && c.CodCliPrinc != c.CodCli {
					c.UF = uf
				}
			}
		}
	}

	out := &segDados{Grupos: ordem}
	for _, c := range clientes {
		if len(c.Produtos) > 0 {
			sort.Slice(c.Produtos, func(i, j int) bool { return c.Produtos[i].Produto < c.Produtos[j].Produto })
			out.Clientes = append(out.Clientes, c)
		}
	}
	segCacheMu.Lock()
	segCache[chave] = segCacheEntry{dados: out, at: time.Now()}
	segCacheMu.Unlock()
	return out, nil
}

// segUFsOcultas — UFs fora do painel (flag "UF liberada?" do vínculo).
func segUFsOcultas(db *sql.DB, empresaID string, vinculoID int) map[string]bool {
	out := map[string]bool{}
	var arr pq.StringArray
	if err := db.QueryRow(`SELECT ufs_ocultas FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2`, vinculoID, empresaID).Scan(&arr); err == nil {
		for _, u := range arr {
			out[strings.ToUpper(strings.TrimSpace(u))] = true
		}
	}
	return out
}

func segSemUFsOcultas(in []*segCliente, ocultas map[string]bool) []*segCliente {
	if len(ocultas) == 0 {
		return in
	}
	out := make([]*segCliente, 0, len(in))
	for _, c := range in {
		if !ocultas[strings.ToUpper(c.UF)] {
			out = append(out, c)
		}
	}
	return out
}

// ─── Objetivos ────────────────────────────────────────────────────────────────

func segObjetivos(db *sql.DB, empresaID string, vigenciaID int) (map[string]map[string]float64, error) {
	rows, err := db.Query(`SELECT uf, grupo, objetivo FROM farol.metas_grupos_objetivos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]float64{}
	for rows.Next() {
		var uf, grupo string
		var obj float64
		if err := rows.Scan(&uf, &grupo, &obj); err != nil {
			return nil, err
		}
		if out[uf] == nil {
			out[uf] = map[string]float64{}
		}
		out[uf][grupo] = obj
	}
	return out, rows.Err()
}

// ─── Painel (objetivo × realizado por UF × grupo × mês + trimestre) ───────────

type SegMesValor struct {
	Mes       string   `json:"mes"`
	Objetivo  *float64 `json:"objetivo"`
	Realizado int      `json:"realizado"`
}

type SegGrupoLinha struct {
	Grupo        string        `json:"grupo"`
	Meses        []SegMesValor `json:"meses"`
	ObjetivoTri  *float64      `json:"objetivo_tri"`
	RealizadoTri int           `json:"realizado_tri"`
}

type SegUF struct {
	UF     string          `json:"uf"`
	Grupos []SegGrupoLinha `json:"grupos"`
}

type SegPainelResp struct {
	Trimestre   string   `json:"trimestre"`
	Meses       []string `json:"meses"`
	MesesSemVig []string `json:"meses_sem_vigencia"`
	Fluxo       string   `json:"fluxo"`
	Grupos      []string `json:"grupos"`
	UFs         []SegUF  `json:"ufs"`
	UFsOcultas  []string `json:"ufs_ocultas"`
}

// segMesesDoTrimestre devolve os 3 meses (AAAA-MM) do trimestre civil de `mes`.
func segMesesDoTrimestre(mes string) ([]string, error) {
	t, err := time.Parse("2006-01", mes)
	if err != nil {
		return nil, fmt.Errorf("mes deve ser AAAA-MM")
	}
	q := (int(t.Month()) - 1) / 3
	out := make([]string, 3)
	for i := 0; i < 3; i++ {
		out[i] = fmt.Sprintf("%d-%02d", t.Year(), q*3+i+1)
	}
	return out, nil
}

func segVigenciaDoMes(db *sql.DB, empresaID string, vinculoID int, mes string) (int, bool) {
	var id int
	err := db.QueryRow(`SELECT id FROM farol.metas_vigencias WHERE vinculo_id = $1 AND empresa_id = $2 AND data_inicio = $3::date`,
		vinculoID, empresaID, mes+"-01").Scan(&id)
	return id, err == nil
}

func segContarPorUF(clientes []*segCliente) map[string]map[string]int {
	out := map[string]map[string]int{}
	for _, c := range clientes {
		for g := range c.Positivos {
			if out[c.UF] == nil {
				out[c.UF] = map[string]int{}
			}
			out[c.UF][g]++
		}
	}
	return out
}

func segPainel(db *sql.DB, empresaID string, vinculoID int, mes, fluxo string, f segEscopo) (*SegPainelResp, error) {
	meses, err := segMesesDoTrimestre(mes)
	if err != nil {
		return nil, err
	}
	ocultas := segUFsOcultas(db, empresaID, vinculoID)
	resp := &SegPainelResp{Meses: meses, Fluxo: fluxo, MesesSemVig: []string{}, Grupos: []string{}, UFs: []SegUF{}, UFsOcultas: sortedKeys(ocultas)}
	t0, _ := time.Parse("2006-01", meses[0])
	resp.Trimestre = fmt.Sprintf("%dº tri/%d", (int(t0.Month())-1)/3+1, t0.Year())

	type porMes struct {
		real map[string]map[string]int
		obj  map[string]map[string]float64
	}
	dados := make([]porMes, len(meses))
	var ordem []string
	ordemVista := map[string]bool{}
	ufs := map[string]bool{}
	for i, m := range meses {
		vig, ok := segVigenciaDoMes(db, empresaID, vinculoID, m)
		if !ok {
			resp.MesesSemVig = append(resp.MesesSemVig, m)
			continue
		}
		obj, err := segObjetivos(db, empresaID, vig)
		if err != nil {
			return nil, err
		}
		dados[i].obj = obj
		for uf := range obj {
			if ocultas[strings.ToUpper(uf)] {
				delete(obj, uf)
				continue
			}
			ufs[uf] = true
		}
		d, err := segCalcular(db, empresaID, vinculoID, vig, fluxo)
		if err != nil {
			// vigência sem lista de produtos: mostra só o objetivo, sem realizado.
			dados[i].real = map[string]map[string]int{}
		} else {
			dados[i].real = segContarPorUF(segSemUFsOcultas(f.filtrar(d.Clientes), ocultas))
			for _, g := range d.Grupos {
				if !ordemVista[g] {
					ordemVista[g] = true
					ordem = append(ordem, g)
				}
			}
			for uf := range dados[i].real {
				ufs[uf] = true
			}
		}
		for _, g := range grupoKeys(obj) {
			if !ordemVista[g] {
				ordemVista[g] = true
				ordem = append(ordem, g)
			}
		}
	}
	resp.Grupos = ordem
	lista := make([]string, 0, len(ufs))
	for uf := range ufs {
		lista = append(lista, uf)
	}
	sort.Strings(lista)
	for _, uf := range lista {
		linha := SegUF{UF: uf}
		for _, g := range ordem {
			gl := SegGrupoLinha{Grupo: g}
			var objTri float64
			temObjTri := false
			for i, m := range meses {
				mv := SegMesValor{Mes: m}
				if dados[i].real != nil {
					mv.Realizado = dados[i].real[uf][g]
				}
				if dados[i].obj != nil {
					if o, ok := dados[i].obj[uf][g]; ok {
						v := o
						mv.Objetivo = &v
						objTri += o
						temObjTri = true
					}
				}
				gl.RealizadoTri += mv.Realizado
				gl.Meses = append(gl.Meses, mv)
			}
			if temObjTri {
				gl.ObjetivoTri = &objTri
			}
			linha.Grupos = append(linha.Grupos, gl)
		}
		resp.UFs = append(resp.UFs, linha)
	}
	return resp, nil
}

func grupoKeys(obj map[string]map[string]float64) []string {
	set := map[string]bool{}
	for _, m := range obj {
		for g := range m {
			set[g] = true
		}
	}
	out := make([]string, 0, len(set))
	for g := range set {
		out = append(out, g)
	}
	sort.Strings(out)
	return out
}

// ─── Escopo ───────────────────────────────────────────────────────────────────

// segEscopo — filtro de hierarquia (login e drill). Campos vazios não filtram.
type segEscopo struct {
	UF, CodGGV, CodCRV, CodRCA, CodCli string
	Nivel                              string // opcional: força o nível listado no detalhe (ex.: "rca" sem passar por GGV/CRV)
}

// ufEm — UF aceita lista separada por vírgula ("GO,MS").
func ufEm(filtro, uf string) bool {
	for _, x := range strings.Split(filtro, ",") {
		if strings.EqualFold(strings.TrimSpace(x), uf) {
			return true
		}
	}
	return false
}

func (f segEscopo) filtrar(in []*segCliente) []*segCliente {
	if f == (segEscopo{}) || f == (segEscopo{Nivel: f.Nivel}) {
		return in
	}
	out := make([]*segCliente, 0, len(in))
	for _, c := range in {
		if f.UF != "" && !ufEm(f.UF, c.UF) {
			continue
		}
		if f.CodGGV != "" && c.CodGGV != f.CodGGV {
			continue
		}
		if f.CodCRV != "" && c.CodCRV != f.CodCRV {
			continue
		}
		if f.CodRCA != "" && c.CodRCA != f.CodRCA {
			continue
		}
		if f.CodCli != "" && c.CodCli != f.CodCli {
			continue
		}
		out = append(out, c)
	}
	return out
}

// ─── Detalhe (drill GGV → CRV → RCA → Cliente → Produto) ──────────────────────

type SegDetalheLinha struct {
	Cod     string `json:"cod"`
	Nome    string `json:"nome"`
	CodPai  string `json:"cod_pai,omitempty"` // nível RCA: código da Equipe (CRV)
	NomePai string `json:"nome_pai,omitempty"`
	Grupos  []int  `json:"grupos"`
	Total   int    `json:"total"`
	Geral   int    `json:"geral"` // clientes distintos que positivaram QUALQUER grupo
}

type SegDetalheResp struct {
	Nivel    string            `json:"nivel"` // ggv | crv | rca | cliente | produto
	Grupos   []string          `json:"grupos"`
	Linhas   []SegDetalheLinha `json:"linhas,omitempty"`
	Produtos []SegProduto      `json:"produtos,omitempty"`
}

func segDetalhe(db *sql.DB, empresaID string, vinculoID int, mes, periodo, fluxo string, f segEscopo) (*SegDetalheResp, error) {
	meses := []string{mes}
	if periodo == "trimestre" {
		var err error
		if meses, err = segMesesDoTrimestre(mes); err != nil {
			return nil, err
		}
	}
	nivel := "ggv"
	switch {
	case f.CodCli == "" && f.CodRCA == "" && (f.Nivel == "rca" || f.Nivel == "cliente"):
		nivel = f.Nivel
	case f.CodCli != "":
		nivel = "produto"
	case f.CodRCA != "":
		nivel = "cliente"
	case f.CodCRV != "":
		nivel = "rca"
	case f.CodGGV != "":
		nivel = "crv"
	}
	ocultasDet := segUFsOcultas(db, empresaID, vinculoID)
	resp := &SegDetalheResp{Nivel: nivel, Grupos: []string{}}

	var ordem []string
	ordemVista := map[string]bool{}
	type linhaAc struct {
		nome, codPai, nomePai string
		porGrp                map[string]int
		geral                 int
	}
	linhas := map[string]*linhaAc{}
	produtos := map[string]*SegProduto{}

	for _, m := range meses {
		vig, ok := segVigenciaDoMes(db, empresaID, vinculoID, m)
		if !ok {
			continue
		}
		d, err := segCalcular(db, empresaID, vinculoID, vig, fluxo)
		if err != nil {
			continue
		}
		for _, g := range d.Grupos {
			if !ordemVista[g] {
				ordemVista[g] = true
				ordem = append(ordem, g)
			}
		}
		for _, c := range segSemUFsOcultas(f.filtrar(d.Clientes), ocultasDet) {
			if nivel == "produto" {
				resp.Linhas = nil
				for _, p := range c.Produtos {
					k := p.CodProd
					if acc, ok := produtos[k]; ok {
						acc.Qtd += p.Qtd
						acc.Positivou = acc.Positivou || p.Positivou
					} else {
						cp := p
						produtos[k] = &cp
					}
				}
				continue
			}
			var cod, nome string
			switch nivel {
			case "ggv":
				cod, nome = c.CodGGV, c.NomeGGV
			case "crv":
				cod, nome = c.CodCRV, c.NomeCRV
			case "rca":
				cod, nome = c.CodRCA, c.NomeRCA
			default:
				cod, nome = c.CodCli, c.Nome
			}
			l := linhas[cod]
			if l == nil {
				l = &linhaAc{nome: nome, porGrp: map[string]int{}}
				if nivel == "rca" {
					l.codPai, l.nomePai = c.CodCRV, c.NomeCRV
				}
				linhas[cod] = l
			}
			for g := range c.Positivos {
				l.porGrp[g]++
			}
			if len(c.Positivos) > 0 {
				l.geral++
			}
		}
	}
	resp.Grupos = ordem
	if nivel == "produto" {
		for _, p := range produtos {
			resp.Produtos = append(resp.Produtos, *p)
		}
		sort.Slice(resp.Produtos, func(i, j int) bool {
			if resp.Produtos[i].Grupo != resp.Produtos[j].Grupo {
				return resp.Produtos[i].Grupo < resp.Produtos[j].Grupo
			}
			return resp.Produtos[i].Produto < resp.Produtos[j].Produto
		})
		return resp, nil
	}
	for cod, l := range linhas {
		linha := SegDetalheLinha{Cod: cod, Nome: l.nome, CodPai: l.codPai, NomePai: l.nomePai, Geral: l.geral, Grupos: make([]int, len(ordem))}
		for i, g := range ordem {
			linha.Grupos[i] = l.porGrp[g]
			linha.Total += l.porGrp[g]
		}
		resp.Linhas = append(resp.Linhas, linha)
	}
	sort.Slice(resp.Linhas, func(i, j int) bool {
		if resp.Linhas[i].Total != resp.Linhas[j].Total {
			return resp.Linhas[i].Total > resp.Linhas[j].Total
		}
		return resp.Linhas[i].Cod < resp.Linhas[j].Cod
	})
	return resp, nil
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func segParamsComuns(r *http.Request) (vinculoID int, mes, fluxo string, err error) {
	q := r.URL.Query()
	vinculoID, err = strconv.Atoi(q.Get("vinculo_id"))
	if err != nil {
		return 0, "", "", fmt.Errorf("vinculo_id é obrigatório")
	}
	mes = strings.TrimSpace(q.Get("mes"))
	if _, e := time.Parse("2006-01", mes); e != nil {
		return 0, "", "", fmt.Errorf("mes deve ser AAAA-MM")
	}
	fluxo = q.Get("fluxo")
	if fluxo == "" {
		fluxo = "faturado"
	}
	return vinculoID, mes, fluxo, nil
}

// segEscopoDoLogin — o GGV/CRV/RCA do login sobrepõe o que veio na URL.
func segEscopoDoLogin(spCtx *FarolContext, f *segEscopo) bool {
	codGGV, codCRV, codRCA, negar := escopoHierarquiaMetas(spCtx)
	if negar {
		return false
	}
	if codGGV != "" {
		f.CodGGV = codGGV
	}
	if codCRV != "" {
		f.CodCRV = codCRV
	}
	if codRCA != "" {
		f.CodRCA = codRCA
	}
	return true
}

func SegmentoFocoPainelHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		vinculoID, mes, fluxo, err := segParamsComuns(r)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		var f segEscopo
		if !segEscopoDoLogin(spCtx, &f) {
			http.Error(w, `{"error":"acesso negado: cadastro de organograma incompleto pra este usuário"}`, http.StatusForbidden)
			return
		}
		resp, err := segPainel(db, spCtx.EmpresaID, vinculoID, mes, fluxo, f)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(resp)
	}
}

func SegmentoFocoDetalheHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		vinculoID, mes, fluxo, err := segParamsComuns(r)
		if err != nil {
			http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
			return
		}
		q := r.URL.Query()
		f := segEscopo{UF: q.Get("uf"), CodGGV: q.Get("cod_ggv"), CodCRV: q.Get("cod_crv"), CodRCA: q.Get("cod_rca"), CodCli: q.Get("cod_cli"), Nivel: q.Get("nivel")}
		if !segEscopoDoLogin(spCtx, &f) {
			http.Error(w, `{"error":"acesso negado: cadastro de organograma incompleto pra este usuário"}`, http.StatusForbidden)
			return
		}
		resp, err := segDetalhe(db, spCtx.EmpresaID, vinculoID, mes, q.Get("periodo"), fluxo, f)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(resp)
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SegmentoFocoUFHandler — PUT /api/farol/segmento-foco/uf?vinculo_id=
// body {"uf":"PA","liberada":true|false}. Liberar tira a UF da lista de ocultas.
func SegmentoFocoUFHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil || r.Method != http.MethodPut {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if !hasSpRole(spCtx.SpRole, "gestor_geral") {
			http.Error(w, `{"error":"Forbidden: gestor_geral necessário"}`, http.StatusForbidden)
			return
		}
		vinculoID, err := strconv.Atoi(r.URL.Query().Get("vinculo_id"))
		var body struct {
			UF       string `json:"uf"`
			Liberada bool   `json:"liberada"`
		}
		if err != nil || json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.UF) == "" {
			http.Error(w, `{"error":"vinculo_id e {uf, liberada} são obrigatórios"}`, http.StatusBadRequest)
			return
		}
		uf := strings.ToUpper(strings.TrimSpace(body.UF))
		q := `UPDATE farol.metas_vinculos SET ufs_ocultas = array_remove(ufs_ocultas, $3) WHERE id = $1 AND empresa_id = $2`
		if !body.Liberada {
			q = `UPDATE farol.metas_vinculos SET ufs_ocultas = (SELECT ARRAY(SELECT DISTINCT x FROM unnest(array_append(ufs_ocultas, $3)) x)) WHERE id = $1 AND empresa_id = $2`
		}
		if _, err := db.Exec(q, vinculoID, spCtx.EmpresaID, uf); err != nil {
			http.Error(w, `{"error":"database error"}`, http.StatusInternalServerError)
			return
		}
		writeAuditLog(db, spCtx.EmpresaID, spCtx.UserID, "metas_vinculos", strconv.Itoa(vinculoID), "segmento_foco_uf", map[string]any{"uf": uf, "liberada": body.Liberada})
		segInvalidarCache()
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "uf": uf, "liberada": body.Liberada})
	}
}
