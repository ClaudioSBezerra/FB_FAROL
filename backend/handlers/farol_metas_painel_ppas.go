package handlers

// farol_metas_painel_ppas.go — drill-down de PPAs por Cliente na Numérica:
// "quais PPAs o cliente comprou e quais não comprou". Pedido do Heverton
// 08/10/2026 (Correção 2): a visão Ponderada já abre os itens (EANs) de um
// cliente; a Numérica não tinha equivalente.
//
// Diferente do drill-down de itens da Ponderada (farol_metas_painel_itens.go,
// persistido em metas_itens_realizado e recalculado no prewarm), este roda
// POR CLIENTE, na hora: uma consulta de venda de 1 CNPJ na janela da
// vigência (índice empresa+cnpj+data, milissegundos). Persistir CNPJ×PPA
// seria ~10 mil clientes × dezenas de PPAs por indústria, vigência e fluxo,
// pra um detalhe que se abre um cliente de cada vez.
//
// A regra de "comprou" é a MESMA de contarPPAsPositivados (cálculo oficial
// do Sortimento Numérica) — quantidade líquida > 0 e, quando a embalagem é
// unidade, mínimo de 3 — porque este detalhe tem que bater com o número do
// cartão. O teto de PPAs (7 ou 15) limita a contagem oficial, não esta
// lista: aqui aparecem todos os PPAs do catálogo.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

// PainelPPALinha — 1 PPA do catálogo da vigência, na visão de 1 cliente.
type PainelPPALinha struct {
	PPA      string   `json:"ppa"`
	CodProds []string `json:"cod_prods,omitempty"`
	Qtd      float64  `json:"qtd"`
	Vendeu   bool     `json:"vendeu"`
	// AbaixoMinimo — houve venda, mas abaixo das 3 unidades que a regra do
	// programa exige; sem isto o PPA apareceria como "não comprou" com
	// quantidade > 0 e pareceria erro.
	AbaixoMinimo bool `json:"abaixo_minimo,omitempty"`
}

const ppaQtdMinimaFixa = 3

// montarLinhasPPA aplica a regra de contarPPAsPositivados a cada PPA do
// catálogo. Puro (sem banco) pra ser testado.
func montarLinhasPPA(ppas []ppaValido, linhas map[string]vendaProdutoAgregada) []PainelPPALinha {
	porPPA := map[string]*PainelPPALinha{}
	var ordem []string
	for _, p := range ppas {
		l, ok := porPPA[p.PPA]
		if !ok {
			l = &PainelPPALinha{PPA: p.PPA}
			porPPA[p.PPA] = l
			ordem = append(ordem, p.PPA)
		}
		l.CodProds = append(l.CodProds, p.CodProd)
		ag, vendido := linhas[p.CodProd]
		if !vendido || ag.Qtd <= 0 {
			continue
		}
		l.Qtd += ag.Qtd
		if exigeQuantidadeMinima(ag.Embalagem, ag.QtUnitCx) && ag.Qtd < ppaQtdMinimaFixa {
			l.AbaixoMinimo = true
			continue
		}
		l.Vendeu = true
	}
	out := make([]PainelPPALinha, 0, len(ordem))
	for _, nome := range ordem {
		l := porPPA[nome]
		if l.Vendeu {
			l.AbaixoMinimo = false // algum cod_prod do PPA valeu — o aviso não se aplica
		}
		sort.Strings(l.CodProds)
		out = append(out, *l)
	}
	// Mesma ordem do drill-down de itens: o que falta vender primeiro.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Vendeu != out[j].Vendeu {
			return !out[i].Vendeu
		}
		return out[i].PPA < out[j].PPA
	})
	return out
}

// calcularPPAsDoCliente lê a venda do CNPJ na janela da vigência (com o
// alargamento do bimestre móvel, igual CalcularRealizadoComPeriodo) e monta
// a lista de PPAs. vigenciaID é a do vínculo de Sortimento Numérica.
func calcularPPAsDoCliente(db *sql.DB, empresaID string, vigenciaID int, fluxo, cnpj string) ([]PainelPPALinha, error) {
	var vinculoID, industriaID int
	var janela, dataInicio, dataFim string
	if err := db.QueryRow(`
		SELECT v.vinculo_id, tm.janela_apuracao, v.data_inicio::text, v.data_fim::text, mv.industria_id
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE v.id = $1 AND v.empresa_id = $2 AND tm.formula_codigo = 'sortimento_numerica_ppa'
	`, vigenciaID, empresaID).Scan(&vinculoID, &janela, &dataInicio, &dataFim, &industriaID); err != nil {
		return nil, fmt.Errorf("vigência de Sortimento Numérica não encontrada")
	}
	if janela == "bimestre_movel" {
		if ini, err := time.Parse("2006-01-02", dataInicio); err == nil {
			dataInicio = ini.AddDate(0, -1, 0).Format("2006-01-02")
		}
	}
	var tiposVenda []string
	if err := scanTiposVendaValidos(db, empresaID, vinculoID, &tiposVenda); err != nil {
		return nil, err
	}
	codFornec, err := codFornecDaIndustria(db, empresaID, industriaID)
	if err != nil {
		return nil, err
	}
	ppas, err := lerPPAsValidos(db, empresaID, vigenciaID)
	if err != nil {
		return nil, err
	}
	if len(ppas) == 0 {
		return nil, fmt.Errorf("nenhum PPA importado pra esta vigência")
	}
	linhas, err := qtdPorCodProdClientesNumerica(db, empresaID, []string{cnpj}, dataInicio, dataFim, fluxo, tiposVenda, codFornec)
	if err != nil {
		return nil, err
	}
	return montarLinhasPPA(ppas, linhas[cnpj]), nil
}

func scanTiposVendaValidos(db *sql.DB, empresaID string, vinculoID int, dest *[]string) error {
	return db.QueryRow(`SELECT tipos_venda_validos FROM farol.metas_vinculos WHERE id = $1 AND empresa_id = $2`, vinculoID, empresaID).
		Scan(pq.Array(dest))
}

// clienteNumericoNoEscopo — o CNPJ pedido existe na lista da vigência E está
// dentro do organograma do usuário (mesmo princípio de
// cnpjsDoEscopoNaVigencia: nunca devolver cliente fora do escopo de login).
func clienteNumericoNoEscopo(db *sql.DB, empresaID string, vigenciaID int, cnpj, codGGV, codCRV, codRCA string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM farol.metas_clientes_numericas WHERE vigencia_id = $1 AND empresa_id = $2 AND cnpj = $3`
	args := []any{vigenciaID, empresaID, cnpj}
	for col, v := range map[string]string{"cod_ggv": codGGV, "cod_crv": codCRV, "cod_rca": codRCA} {
		if v != "" {
			args = append(args, v)
			query += fmt.Sprintf(" AND %s = $%d", col, len(args))
		}
	}
	var ok bool
	err := db.QueryRow(query+")", args...).Scan(&ok)
	return ok, err
}

// MetasPainelPPAsHandler — GET /api/farol/metas-painel-ppas
//
//	?vigencia_sortimento_id=&fluxo=&cnpj=
func MetasPainelPPAsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		vigenciaID, err := strconv.Atoi(q.Get("vigencia_sortimento_id"))
		cnpj := strings.TrimSpace(q.Get("cnpj"))
		if err != nil || cnpj == "" {
			http.Error(w, `{"error":"vigencia_sortimento_id e cnpj são obrigatórios"}`, http.StatusBadRequest)
			return
		}
		codGGV, codCRV, codRCA, negarEscopo := escopoHierarquiaMetas(spCtx)
		if negarEscopo {
			http.Error(w, `{"error":"acesso negado: cadastro de organograma incompleto pra este usuário"}`, http.StatusForbidden)
			return
		}
		responderPPAsDoCliente(w, db, spCtx.EmpresaID, vigenciaID, q.Get("fluxo"), cnpj, codGGV, codCRV, codRCA)
	}
}

// MetasPublicPainelPPAsHandler — GET /api/farol/public/metas-painel-ppas
// (mobile/ION VENDAS), escopo por ?scope=sup|rca|ggv&cod=, igual o drill-down
// de itens público.
//
//	?cnpj=<empresa>&scope=&cod=&vigencia_sortimento_id=&fluxo=&cliente_cnpj=
func MetasPublicPainelPPAsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		empresaID := resolveEmpresaCNPJ(db, q.Get("cnpj"))
		if empresaID == "" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "empresa não encontrada para este CNPJ"})
			return
		}
		scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
		cod := strings.TrimSpace(q.Get("cod"))
		vigenciaID, err := strconv.Atoi(q.Get("vigencia_sortimento_id"))
		clienteCNPJ := strings.TrimSpace(q.Get("cliente_cnpj"))
		if (scope != "sup" && scope != "rca" && scope != "ggv") || cod == "" || err != nil || clienteCNPJ == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "scope (sup|rca|ggv), cod, vigencia_sortimento_id e cliente_cnpj são obrigatórios"})
			return
		}
		codGGV, codCRV, codRCA := codsHierarquiaEscopo(scope, cod)
		responderPPAsDoCliente(w, db, empresaID, vigenciaID, q.Get("fluxo"), clienteCNPJ, codGGV, codCRV, codRCA)
	}
}

func responderPPAsDoCliente(w http.ResponseWriter, db *sql.DB, empresaID string, vigenciaID int, fluxo, cnpj, codGGV, codCRV, codRCA string) {
	if fluxo == "" {
		fluxo = "faturado"
	}
	ok, err := clienteNumericoNoEscopo(db, empresaID, vigenciaID, cnpj, codGGV, codCRV, codRCA)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
		return
	}
	if !ok {
		json.NewEncoder(w).Encode(map[string]any{"ppas": []PainelPPALinha{}})
		return
	}
	ppas, err := calcularPPAsDoCliente(db, empresaID, vigenciaID, fluxo, cnpj)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ppas": ppas})
}
