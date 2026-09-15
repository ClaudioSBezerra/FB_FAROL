package handlers

// farol_mcp.go — Servidor MCP (Model Context Protocol) pros agentes de IA
// da JC Distribuição no Paperclip (CEO/CTO/CFO + gerador de relatórios),
// pedido do Claudio 15/09/2026: "ficou muito completo pro agente buscar
// esse dado na base dele — vamos criar MCP com dado pré-pronto".
//
// Decisão de arquitetura: o MCP mora DENTRO deste backend (não um serviço
// novo) — reusa direto as funções internas do motor de apuração
// (obterOuCongelarRealizado, gerarComparativoFechamento) em vez de bater
// via HTTP nos próprios endpoints do Farol ou duplicar query. v1 (15/09)
// cobre só Objetivos por Indústria — é o que está em foco agora (BI e
// Dinheiro na Mesa ficaram de fora, ver decisão do Claudio na mesma
// sessão: BI ainda sem adoção real, Dinheiro na Mesa não vingou).
//
// Transporte: Streamable HTTP (github.com/modelcontextprotocol/go-sdk),
// montado como mais uma rota deste MESMO servidor (não um processo novo) —
// evita criar um segundo serviço/domínio no Coolify só pra isso. Exposto
// pra fora (Paperclip é SaaS externo), por isso exige token — ver
// authMiddleware abaixo. Escopo de empresa é FIXO (uma instância deste
// MCP serve UMA empresa só, a JC Distribuição) — nunca aceita empresa_id
// vindo do agente, só do env var no boot do servidor.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ─── Setup / autenticação ───────────────────────────────────────────────────

// NewMCPHandler monta o handler HTTP do servidor MCP, escopado a UMA
// empresa (lida de FAROL_MCP_EMPRESA_ID no boot). Devolve ok=false (sem
// montar nada) se as env vars obrigatórias não estiverem setadas — quem
// chama (main.go) só registra a rota se ok=true, pra não expor um MCP
// sem token por engano.
func NewMCPHandler(getDB func() *sql.DB) (h http.Handler, ok bool) {
	empresaID := strings.TrimSpace(os.Getenv("FAROL_MCP_EMPRESA_ID"))
	token := strings.TrimSpace(os.Getenv("FAROL_MCP_TOKEN"))
	if empresaID == "" || token == "" {
		log.Printf("[farol:mcp] desligado — defina FAROL_MCP_EMPRESA_ID e FAROL_MCP_TOKEN pra habilitar")
		return nil, false
	}

	// getServer roda a cada nova sessão MCP (não no boot do processo) — por
	// isso resolve getDB() aqui, não uma vez só: initDBAsync conecta em
	// background, então no boot getDB() ainda pode devolver nil.
	getServer := func(*http.Request) *mcp.Server {
		server := mcp.NewServer(&mcp.Implementation{Name: "farol-jc-distribuicao", Version: "1.0.0"}, nil)
		db := getDB()
		registrarToolObjetivosIndustria(server, db, empresaID)
		registrarToolComparativoFechamento(server, db, empresaID)
		return server
	}

	streamable := mcp.NewStreamableHTTPHandler(getServer, nil)
	return authMiddleware(token, streamable), true
}

// authMiddleware exige `Authorization: Bearer <token>` — o MCP expõe dado
// comercial real (venda, meta, cliente) pra fora via internet (Paperclip é
// SaaS externo), não pode ficar aberto só porque "é read-only".
func authMiddleware(token string, next http.Handler) http.Handler {
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got == "" || got != want {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Helpers compartilhados pelas tools ────────────────────────────────────

// resolverIndustriaID acha o ID de farol.industrias por nome parcial (o
// agente de IA não sabe o ID interno, só o nome de negócio — ex:
// "Unilever HC"). Erro lista os nomes candidatos quando não acha exatamente
// um, pro agente conseguir se corrigir sozinho na próxima chamada.
func resolverIndustriaID(db *sql.DB, empresaID, industria string) (int, string, error) {
	rows, err := db.Query(`
		SELECT id, nome FROM farol.industrias
		WHERE empresa_id = $1 AND ativo = true AND nome ILIKE $2
		ORDER BY nome
	`, empresaID, "%"+strings.TrimSpace(industria)+"%")
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	type cand struct {
		id   int
		nome string
	}
	var candidatos []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.id, &c.nome); err != nil {
			return 0, "", err
		}
		candidatos = append(candidatos, c)
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	switch len(candidatos) {
	case 0:
		nomes, _ := listarNomesIndustrias(db, empresaID)
		return 0, "", fmt.Errorf("nenhuma indústria encontrada pra %q — indústrias cadastradas: %s", industria, strings.Join(nomes, ", "))
	case 1:
		return candidatos[0].id, candidatos[0].nome, nil
	default:
		var nomes []string
		for _, c := range candidatos {
			nomes = append(nomes, c.nome)
		}
		return 0, "", fmt.Errorf("%q bate com mais de uma indústria — seja mais específico: %s", industria, strings.Join(nomes, ", "))
	}
}

func listarNomesIndustrias(db *sql.DB, empresaID string) ([]string, error) {
	rows, err := db.Query(`SELECT nome FROM farol.industrias WHERE empresa_id = $1 AND ativo = true ORDER BY nome`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// parsePeriodo aceita "AAAA-MM" (ex: "2026-08") e devolve o primeiro e o
// último dia do mês, no formato que as queries de vigência esperam
// (DATE, "AAAA-MM-DD").
func parsePeriodo(periodo string) (dataInicio, dataFim string, err error) {
	t, err := time.Parse("2006-01", strings.TrimSpace(periodo))
	if err != nil {
		return "", "", fmt.Errorf("período inválido: %q — use o formato AAAA-MM, ex: 2026-08", periodo)
	}
	primeiro := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	ultimo := primeiro.AddDate(0, 1, -1)
	return primeiro.Format("2006-01-02"), ultimo.Format("2006-01-02"), nil
}

// resolverVinculoVigencia acha o vínculo+vigência de um formula_codigo
// ('cobertura_rede'/'sortimento_rede') pra uma indústria+período EXATOS —
// mesmo critério que o Comparativo Fechamento já usa (farol_fechamento_comercial.go).
// ok=false (sem erro) é o caso normal de "essa indústria não tem vigência
// desse tipo de métrica" (ex: só Sortimento cadastrado, sem Cobertura).
func resolverVinculoVigencia(db *sql.DB, empresaID string, industriaID int, formulaCodigo, dataInicio, dataFim string) (vinculoID, vigenciaID int, ok bool, err error) {
	var v, vg sql.NullInt64
	err = db.QueryRow(`
		SELECT mv.id, vig.id FROM farol.metas_vinculos mv
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id AND tm.formula_codigo = $5
		JOIN farol.metas_vigencias vig ON vig.vinculo_id = mv.id AND vig.data_inicio = $3 AND vig.data_fim = $4
		WHERE mv.empresa_id = $1 AND mv.industria_id = $2
	`, empresaID, industriaID, dataInicio, dataFim, formulaCodigo).Scan(&v, &vg)
	if err != nil {
		return 0, 0, false, nil
	}
	if !v.Valid || !vg.Valid {
		return 0, 0, false, nil
	}
	return int(v.Int64), int(vg.Int64), true, nil
}

// ─── Tool: farol_objetivos_industria ────────────────────────────────────────

type objetivosIndustriaInput struct {
	Industria string `json:"industria" jsonschema:"Nome (ou parte do nome) da indústria/fornecedor cadastrado no Farol — ex: 'Unilever HC', 'Unilever Food'"`
	Periodo   string `json:"periodo" jsonschema:"Período no formato AAAA-MM, ex: 2026-08"`
	Fluxo     string `json:"fluxo,omitempty" jsonschema:"faturado ou transmitido — padrão: faturado"`
}

type redeObjetivo struct {
	CodPrinc string `json:"cod_princ"`
	Razao    string `json:"razao"`
	Fantasia string `json:"fantasia"`
	QtLojas  int    `json:"qt_lojas"`
	NomeGGV  string `json:"nome_ggv,omitempty"`
	NomeCRV  string `json:"nome_crv,omitempty"`
	NomeRCA  string `json:"nome_rca,omitempty"`

	CoberturaValorTotal *float64 `json:"cobertura_valor_total,omitempty"`
	CoberturaValorMedio *float64 `json:"cobertura_valor_medio_por_loja,omitempty"`
	CoberturaAtingiu    *bool    `json:"cobertura_atingiu,omitempty"`

	SortimentoMedioEans *float64 `json:"sortimento_media_eans_por_loja,omitempty"`
	SortimentoAtingiu   *bool    `json:"sortimento_atingiu,omitempty"`
}

type objetivosIndustriaOutput struct {
	Industria string `json:"industria"`
	Periodo   string `json:"periodo"`
	Fluxo     string `json:"fluxo"`

	TotalRedes               int `json:"total_redes"`
	CoberturaRedesAtingindo  int `json:"cobertura_redes_atingindo,omitempty"`
	CoberturaRedesFaltando   int `json:"cobertura_redes_faltando,omitempty"`
	SortimentoRedesAtingindo int `json:"sortimento_redes_atingindo,omitempty"`
	SortimentoRedesFaltando  int `json:"sortimento_redes_faltando,omitempty"`

	Redes []redeObjetivo `json:"redes"`
}

func registrarToolObjetivosIndustria(server *mcp.Server, db *sql.DB, empresaID string) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "farol_objetivos_industria",
		Description: "Resultado de Cobertura e Sortimento por Rede de um programa de Objetivos por Indústria " +
			"(ex: Unilever HC/Food) num período — quantas Redes estão atingindo a meta e quanto falta, por Rede.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in objetivosIndustriaInput) (*mcp.CallToolResult, objetivosIndustriaOutput, error) {
		var out objetivosIndustriaOutput
		fluxo := strings.TrimSpace(in.Fluxo)
		if fluxo == "" {
			fluxo = "faturado"
		}
		if fluxo != "faturado" && fluxo != "transmitido" {
			return nil, out, fmt.Errorf("fluxo inválido: %q (use 'faturado' ou 'transmitido')", in.Fluxo)
		}

		industriaID, nomeCanonico, err := resolverIndustriaID(db, empresaID, in.Industria)
		if err != nil {
			return nil, out, err
		}
		dataInicio, dataFim, err := parsePeriodo(in.Periodo)
		if err != nil {
			return nil, out, err
		}

		out.Industria = nomeCanonico
		out.Periodo = in.Periodo
		out.Fluxo = fluxo

		vinculoCob, vigenciaCob, temCob, err := resolverVinculoVigencia(db, empresaID, industriaID, "cobertura_rede", dataInicio, dataFim)
		if err != nil {
			return nil, out, err
		}
		vinculoSort, vigenciaSort, temSort, err := resolverVinculoVigencia(db, empresaID, industriaID, "sortimento_rede", dataInicio, dataFim)
		if err != nil {
			return nil, out, err
		}
		if !temCob && !temSort {
			return nil, out, fmt.Errorf("%s não tem vigência de Cobertura nem Sortimento cadastrada pro período %s", nomeCanonico, in.Periodo)
		}

		porRede := map[string]*redeObjetivo{}
		ordem := []string{}
		pegar := func(cp string) *redeObjetivo {
			if r, ok := porRede[cp]; ok {
				return r
			}
			r := &redeObjetivo{CodPrinc: cp}
			porRede[cp] = r
			ordem = append(ordem, cp)
			return r
		}

		if temCob {
			res, err := obterOuCongelarRealizado(db, empresaID, vinculoCob, vigenciaCob, fluxo, "rede")
			if err != nil {
				return nil, out, fmt.Errorf("erro calculando Cobertura: %w", err)
			}
			for _, rd := range res.Redes {
				r := pegar(rd.CodPrinc)
				r.Razao, r.Fantasia, r.QtLojas = rd.Razao, rd.Fantasia, rd.QtLojas
				r.NomeGGV, r.NomeCRV, r.NomeRCA = rd.NomeGGV, rd.NomeCRV, rd.NomeRCA
				valorTotal, valorMedio, atingiu := rd.ValorTotal, rd.Valor, rd.Atingiu
				r.CoberturaValorTotal, r.CoberturaValorMedio, r.CoberturaAtingiu = &valorTotal, &valorMedio, &atingiu
				if atingiu {
					out.CoberturaRedesAtingindo++
				} else {
					out.CoberturaRedesFaltando++
				}
			}
		}
		if temSort {
			res, err := obterOuCongelarRealizado(db, empresaID, vinculoSort, vigenciaSort, fluxo, "rede")
			if err != nil {
				return nil, out, fmt.Errorf("erro calculando Sortimento: %w", err)
			}
			for _, rd := range res.Redes {
				r := pegar(rd.CodPrinc)
				r.Razao, r.Fantasia, r.QtLojas = rd.Razao, rd.Fantasia, rd.QtLojas
				r.NomeGGV, r.NomeCRV, r.NomeRCA = rd.NomeGGV, rd.NomeCRV, rd.NomeRCA
				media, atingiu := rd.Valor, rd.Atingiu
				r.SortimentoMedioEans, r.SortimentoAtingiu = &media, &atingiu
				if atingiu {
					out.SortimentoRedesAtingindo++
				} else {
					out.SortimentoRedesFaltando++
				}
			}
		}

		for _, cp := range ordem {
			out.Redes = append(out.Redes, *porRede[cp])
		}
		out.TotalRedes = len(out.Redes)

		return nil, out, nil
	})
}

// ─── Tool: farol_comparativo_fechamento ─────────────────────────────────────

type comparativoFechamentoInput struct {
	Industria string `json:"industria" jsonschema:"Nome (ou parte do nome) da indústria — ex: 'Unilever HC'"`
	Periodo   string `json:"periodo" jsonschema:"Período no formato AAAA-MM, ex: 2026-08"`
}

type comparativoFechamentoOutput struct {
	Industria   string             `json:"industria"`
	Periodo     string             `json:"periodo"`
	Total       int                `json:"total"`
	Divergentes int                `json:"divergentes"`
	Linhas      []comparativoLinha `json:"linhas"`
}

func registrarToolComparativoFechamento(server *mcp.Server, db *sql.DB, empresaID string) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "farol_comparativo_fechamento",
		Description: "Compara, Rede a Rede, o fechamento que a indústria (fornecedor) reportou por fora com o que o " +
			"Farol calculou pro mesmo período — mostra onde os números divergem e por quanto.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in comparativoFechamentoInput) (*mcp.CallToolResult, comparativoFechamentoOutput, error) {
		var out comparativoFechamentoOutput
		industriaID, nomeCanonico, err := resolverIndustriaID(db, empresaID, in.Industria)
		if err != nil {
			return nil, out, err
		}
		dataInicio, dataFim, err := parsePeriodo(in.Periodo)
		if err != nil {
			return nil, out, err
		}

		linhas, err := gerarComparativoFechamento(db, empresaID, industriaID, dataInicio, dataFim)
		if err != nil {
			return nil, out, err
		}

		out.Industria = nomeCanonico
		out.Periodo = in.Periodo
		out.Linhas = linhas
		out.Total = len(linhas)
		for _, l := range linhas {
			if l.Status != "OK" {
				out.Divergentes++
			}
		}

		return nil, out, nil
	})
}
