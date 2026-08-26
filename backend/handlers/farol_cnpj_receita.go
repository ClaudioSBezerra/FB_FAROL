package handlers

// farol_cnpj_receita.go — carga do cadastro da Receita e o relatório que a
// justifica.
//
// A PERGUNTA QUE ESTE RELATÓRIO RESPONDE. A lista de clientes sem venda mistura
// "o concorrente levou" com "a loja fechou". A primeira é falha de venda e
// alguém tem que agir; a segunda não tinha o que fazer, e cobrar o supervisor
// por ela é injusto além de mandar o RCA para uma porta de aço.
//
// Medido em 25/08/2026: 8.507 clientes compraram em 2025 e nada em 2026,
// R$ 107,1 milhões. Em amostra aleatória de 40, cinco estavam baixados ou
// inaptos — e nos cinco a mudança de situação cadastral cai exatamente na
// janela em que a compra parou.

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"fb_farol/services"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/xuri/excelize/v2"
)

// Logo de ÚLTIMO RECURSO, embutida no binário. A logo de verdade vem do banco,
// por inquilino (ver logoRelatorio): o Farol é multiempresa, e marca fixa no
// código serve no máximo a uma delas — o relatório da JC saiu com a marca da FB
// justamente por isso, em 25/08/2026.
//
// Fica embutida, e não em arquivo, porque o container do Go tem só o binário:
// não enxerga frontend/public, onde o PNG vive para o navegador.
//
//go:embed assets/logo.png
var logoPNG []byte

// logoRelatorio — logo do inquilino, lida de companies.logo_data.
//
// Nunca devolve erro: relatório sem marca ainda serve, relatório que não é
// gerado não serve para nada. Qualquer problema cai no embed.
func logoRelatorio(db *sql.DB, empresaID string) ([]byte, extension.Type) {
	var dados []byte
	var mime string
	err := db.QueryRow(`SELECT logo_data, logo_mime FROM companies
	                     WHERE id = $1::uuid AND logo_data IS NOT NULL`, empresaID).Scan(&dados, &mime)
	if err != nil || len(dados) == 0 {
		return logoPNG, extension.Png
	}
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		return dados, extension.Png
	case "image/jpeg", "image/jpg":
		return dados, extension.Jpg
	default:
		// SVG e WebP existem no cadastro e o maroto não os desenha — passar
		// adiante derrubaria a geração inteira. Marca errada é menos ruim que
		// PDF que não sai.
		log.Printf("[cnpj:relatorio] logo da empresa %s em formato %q não suportado, usando a padrão", empresaID, mime)
		return logoPNG, extension.Png
	}
}

// ─── Carga ────────────────────────────────────────────────────────────────────

// Uma carga por vez. Duas rodadas simultâneas consultariam os MESMOS CNPJs (a
// fila é derivada e só muda quando a linha é gravada), dobrando o peso sobre uma
// API pública gratuita para fazer o mesmo trabalho duas vezes.
var (
	cargaCNPJMu       sync.Mutex
	cargaCNPJRodando  bool
	cargaCNPJInicio   time.Time
	cargaCNPJUltimo   services.ResultadoCargaCNPJ
	cargaCNPJTerminou time.Time
)

// CargaCNPJReceitaHandler — POST /api/v2/farol/cnpj-receita/carga?limite=&pausa_ms=
//
// Responde na hora e trabalha em segundo plano: uma rodada de 8.507 CNPJs leva
// horas, e nenhum navegador espera isso. O acompanhamento é pelo /status.
func CargaCNPJReceitaHandler(db *sql.DB) http.HandlerFunc {
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

		limite := intParam(r, "limite", 1000)
		pausa := time.Duration(intParam(r, "pausa_ms", 300)) * time.Millisecond

		cargaCNPJMu.Lock()
		if cargaCNPJRodando {
			desde := time.Since(cargaCNPJInicio).Round(time.Second)
			cargaCNPJMu.Unlock()
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]any{
				"error": "já existe uma carga em andamento",
				"desde": desde.String(),
			})
			return
		}
		cargaCNPJRodando = true
		cargaCNPJInicio = time.Now()
		cargaCNPJMu.Unlock()

		empresaID := spCtx.EmpresaID
		log.Printf("[cnpj:receita] disparo MANUAL limite=%d pausa=%v por user=%s", limite, pausa, spCtx.UserID)

		go func() {
			defer func() {
				cargaCNPJMu.Lock()
				cargaCNPJRodando = false
				cargaCNPJTerminou = time.Now()
				cargaCNPJMu.Unlock()
			}()
			res, err := services.CarregarCNPJReceita(context.Background(), db, empresaID, limite, pausa)
			if err != nil {
				log.Printf("[cnpj:receita] carga ERRO: %v", err)
			}
			cargaCNPJMu.Lock()
			cargaCNPJUltimo = res
			cargaCNPJMu.Unlock()
		}()

		json.NewEncoder(w).Encode(map[string]any{
			"status": "iniciada",
			"limite": limite,
			"pausa":  pausa.String(),
		})
	}
}

// StatusCNPJReceitaHandler — GET /api/v2/farol/cnpj-receita/status
func StatusCNPJReceitaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		out := map[string]any{}

		cargaCNPJMu.Lock()
		out["rodando"] = cargaCNPJRodando
		if cargaCNPJRodando {
			out["desde"] = time.Since(cargaCNPJInicio).Round(time.Second).String()
		}
		if !cargaCNPJTerminou.IsZero() {
			out["ultima_rodada"] = map[string]any{
				"candidatos":     cargaCNPJUltimo.Candidatos,
				"sucesso":        cargaCNPJUltimo.Sucesso,
				"nao_encontrado": cargaCNPJUltimo.NaoEncontrado,
				"falha":          cargaCNPJUltimo.Falha,
				"duracao":        cargaCNPJUltimo.Duracao.Round(time.Second).String(),
				"interrompido":   cargaCNPJUltimo.Interrompido,
				"terminou_em":    cargaCNPJTerminou.Format(time.RFC3339),
			}
		}
		cargaCNPJMu.Unlock()

		// Progresso real vem do banco, não do contador em memória: o processo
		// pode ter reiniciado no meio e o que importa é quanto da base já tem
		// cadastro, não quanto esta instância consultou.
		var consultados, comErro int
		db.QueryRow(`SELECT COUNT(*) FILTER (WHERE erro IS NULL), COUNT(*) FILTER (WHERE erro IS NOT NULL)
		               FROM farol.cnpj_receita`).Scan(&consultados, &comErro)
		var total int
		db.QueryRow(`
SELECT COUNT(*) FROM (
  SELECT DISTINCT regexp_replace(cnpj,'[^0-9]','','g') AS doc
    FROM farol.agg_fat_mkt_cli_mes WHERE empresa_id = $1 AND cnpj <> ''
) d WHERE length(d.doc) = 14`, spCtx.EmpresaID).Scan(&total)

		out["consultados"] = consultados
		out["com_erro"] = comErro
		out["total_base"] = total
		if total > 0 {
			out["pct"] = float64(consultados+comErro) / float64(total) * 100
		}
		json.NewEncoder(w).Encode(out)
	}
}

func intParam(r *http.Request, nome string, padrao int) int {
	if v := strings.TrimSpace(r.URL.Query().Get(nome)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return padrao
}

// ─── Relatório ────────────────────────────────────────────────────────────────

type clienteReceita struct {
	CNPJ          string  `json:"cnpj"`
	RazaoSocial   string  `json:"razao_social"`
	NomeCadastro  string  `json:"nome_cadastro"`
	Situacao      string  `json:"situacao"`
	SituacaoData  string  `json:"situacao_data"`
	CNAE          string  `json:"cnae"`
	Municipio     string  `json:"municipio"`
	UF            string  `json:"uf"`
	UltimaCompra  string  `json:"ultima_compra"`
	LiquidoAnt    float64 `json:"liquido_ant"`
	LiquidoAtual  float64 `json:"liquido_atual"`
	NomeGerente   string  `json:"nome_gerente"`
	NomeSupervisr string  `json:"nome_supervisor"`
	CodRCA        string  `json:"cod_rca"`

	// Sucessora provável — CNPJ ativo comprando no mesmo endereço.
	// Forca: "placa" (mesmo nome fantasia, quase certo), "endereco" (endereço
	// com até 2 ocupantes, forte) ou "galeria" (endereço concorrido, fraco —
	// mostrado para o gestor julgar, nunca somado como certeza).
	Sucessora      string `json:"sucessora"`
	SucessoraNome  string `json:"sucessora_nome"`
	SucessoraForca string `json:"sucessora_forca"`
}

type resumoSituacao struct {
	Situacao   string  `json:"situacao"`
	Clientes   int     `json:"clientes"`
	LiquidoAnt float64 `json:"liquido_ant"`
}

type relatorioReceita struct {
	Linhas     []clienteReceita `json:"linhas"`
	Resumo     []resumoSituacao `json:"resumo"`
	AnoAnt     int              `json:"ano_anterior"`
	AnoAtual   int              `json:"ano_atual"`
	GeradoEm   string           `json:"gerado_em"`
	Cobertura  float64          `json:"cobertura_pct"`
	Incompleto bool             `json:"incompleto"`

	// Reaberturas — só as de evidência forte ("placa" e "endereco"). As de
	// galeria ficam de fora da CONTA, embora apareçam na linha do cliente:
	// somar vizinho de shopping como reabertura infla o efeito, que foi
	// exatamente o erro que a checagem de 26/08/2026 descartou.
	Reaberturas      int     `json:"reaberturas"`
	ReaberturasValor float64 `json:"reaberturas_valor"`
}

// consultarClientesReceita — clientes cujo CNPJ NÃO está ativo na Receita.
//
// O dono (gerente/supervisor/RCA) vem do mês MAIS RECENTE em que o cliente
// comprou, não de uma soma: quem responde por um cliente que parou é quem o
// atendia quando ele parou.
func consultarClientesReceita(ctx context.Context, db *sql.DB, empresaID, situacao string) (relatorioReceita, error) {
	agora := time.Now()
	out := relatorioReceita{
		AnoAtual: agora.Year(),
		AnoAnt:   agora.Year() - 1,
		GeradoEm: agora.Format("02/01/2006 15:04"),
	}

	filtro := ""
	args := []any{empresaID, out.AnoAnt, out.AnoAtual}
	if s := strings.TrimSpace(strings.ToUpper(situacao)); s != "" && s != "TODAS" {
		args = append(args, s)
		filtro = fmt.Sprintf(" AND upper(r.situacao_desc) = $%d", len(args))
	}

	q := fmt.Sprintf(`
WITH base AS (
    SELECT regexp_replace(v.cnpj, '[^0-9]', '', 'g') AS doc,
           v.cod_gerente, v.cod_supervisor, v.cod_rca, v.nome_cli,
           v.ano, v.ano * 100 + v.mes AS ym, v.liquido
      FROM farol.agg_fat_v03_l3_mes v
     WHERE v.empresa_id = $1 AND v.cnpj <> ''
), dono AS (
    SELECT DISTINCT ON (doc)
           doc, cod_gerente, cod_supervisor, cod_rca, nome_cli, ym AS ultimo_ym
      FROM base ORDER BY doc, ym DESC
), val AS (
    SELECT doc,
           SUM(liquido) FILTER (WHERE ano = $2) AS liq_ant,
           SUM(liquido) FILTER (WHERE ano = $3) AS liq_atual
      FROM base GROUP BY doc
), addr AS (
    -- Ocupantes do mesmo CEP+número. Galeria e shopping têm dezenas de CNPJs no
    -- mesmo endereço, e ali "mesmo endereço" quase não informa: o ativo ao lado
    -- é vizinho, não sucessor. Medido em 26/08/2026: 239 endereços concentram
    -- 3.043 CNPJs, e eram eles que inflavam a detecção.
    SELECT cep, numero, COUNT(*) AS ocupantes
      FROM farol.cnpj_receita
     WHERE COALESCE(cep,'') <> '' AND COALESCE(numero,'') <> ''
     GROUP BY 1, 2
), suc AS (
    -- Sucessora provável: CNPJ ATIVO na Receita, comprando no ano corrente, no
    -- mesmo endereço de um que parou. É a prática que o dono da JC descreveu —
    -- fechar a empresa quando o faturamento cresce e reabrir em outro CPF.
    -- Quando existe, não houve perda: a venda mudou de linha, não sumiu.
    SELECT DISTINCT ON (p.cnpj)
           p.cnpj AS morto, a.cnpj AS sucessora,
           COALESCE(NULLIF(a.razao_social,''), a.nome_fantasia, '') AS sucessora_nome,
           CASE WHEN upper(trim(COALESCE(a.nome_fantasia,''))) = upper(trim(COALESCE(p.nome_fantasia,'')))
                     AND COALESCE(p.nome_fantasia,'') <> '' THEN 'placa'
                WHEN ad.ocupantes <= 2 THEN 'endereco'
                ELSE 'galeria' END AS forca
      FROM farol.cnpj_receita p
      JOIN addr ad ON ad.cep = p.cep AND ad.numero = p.numero
      JOIN farol.cnpj_receita a
        ON a.cep = p.cep AND a.numero = p.numero AND a.cnpj <> p.cnpj
       AND a.situacao_cod = 2
      JOIN val va ON va.doc = a.cnpj AND COALESCE(va.liq_atual,0) > 0
      -- O cliente precisa ter PARADO. Sucessora de quem continua comprando não
      -- existe: são dois negócios vivos no mesmo endereço, e tratá-los como
      -- sucessão inflou a conta em R$ 55 milhões na primeira versão (26/08/2026).
      LEFT JOIN val vp ON vp.doc = p.cnpj
     WHERE p.situacao_cod IS NOT NULL AND p.situacao_cod <> 2
       AND COALESCE(vp.liq_atual, 0) = 0
       AND COALESCE(p.cep,'') <> '' AND COALESCE(p.numero,'') <> ''
     -- Uma sucessora por cliente, a de evidência mais forte.
     ORDER BY p.cnpj,
              CASE WHEN upper(trim(COALESCE(a.nome_fantasia,''))) = upper(trim(COALESCE(p.nome_fantasia,'')))
                        AND COALESCE(p.nome_fantasia,'') <> '' THEN 1
                   WHEN ad.ocupantes <= 2 THEN 2
                   ELSE 3 END
), ger AS (
    SELECT DISTINCT ON (cod_gerente) cod_gerente, nome_gerente
      FROM farol.agg_fat_v03_l0_mes WHERE empresa_id = $1
     ORDER BY cod_gerente, ano DESC, mes DESC
), sup AS (
    SELECT DISTINCT ON (cod_supervisor) cod_supervisor, nome_supervisor
      FROM farol.agg_fat_v03_l1_mes WHERE empresa_id = $1
     ORDER BY cod_supervisor, ano DESC, mes DESC
)
SELECT r.cnpj,
       COALESCE(r.razao_social, ''), COALESCE(d.nome_cli, ''),
       COALESCE(r.situacao_desc, ''),
       COALESCE(to_char(r.situacao_data, 'DD/MM/YYYY'), ''),
       COALESCE(r.cnae_desc, ''), COALESCE(r.municipio, ''), COALESCE(r.uf, ''),
       d.ultimo_ym,
       COALESCE(v.liq_ant, 0)::float8, COALESCE(v.liq_atual, 0)::float8,
       COALESCE(g.nome_gerente, d.cod_gerente), COALESCE(s.nome_supervisor, d.cod_supervisor),
       COALESCE(d.cod_rca, ''),
       COALESCE(sc.sucessora, ''), COALESCE(sc.sucessora_nome, ''), COALESCE(sc.forca, '')
  FROM farol.cnpj_receita r
  JOIN dono d ON d.doc = r.cnpj
  LEFT JOIN val v ON v.doc = r.cnpj
  LEFT JOIN ger g ON g.cod_gerente    = d.cod_gerente
  LEFT JOIN sup s ON s.cod_supervisor = d.cod_supervisor
  LEFT JOIN suc sc ON sc.morto = r.cnpj
 WHERE r.situacao_cod IS NOT NULL AND r.situacao_cod <> 2%s
 ORDER BY COALESCE(v.liq_ant, 0) DESC`, filtro)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	acum := map[string]*resumoSituacao{}
	for rows.Next() {
		var c clienteReceita
		var ym int
		if err := rows.Scan(&c.CNPJ, &c.RazaoSocial, &c.NomeCadastro, &c.Situacao,
			&c.SituacaoData, &c.CNAE, &c.Municipio, &c.UF, &ym,
			&c.LiquidoAnt, &c.LiquidoAtual,
			&c.NomeGerente, &c.NomeSupervisr, &c.CodRCA,
			&c.Sucessora, &c.SucessoraNome, &c.SucessoraForca); err != nil {
			return out, err
		}
		c.UltimaCompra = fmt.Sprintf("%02d/%04d", ym%100, ym/100)
		out.Linhas = append(out.Linhas, c)

		r := acum[c.Situacao]
		if r == nil {
			r = &resumoSituacao{Situacao: c.Situacao}
			acum[c.Situacao] = r
		}
		r.Clientes++
		r.LiquidoAnt += c.LiquidoAnt

		if c.SucessoraForca == "placa" || c.SucessoraForca == "endereco" {
			out.Reaberturas++
			out.ReaberturasValor += c.LiquidoAnt
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	for _, v := range acum {
		out.Resumo = append(out.Resumo, *v)
	}

	// Cobertura: sem ela o relatório mente por omissão. Com 12% da base
	// consultada, "23 clientes baixados" parece o total quando é o começo —
	// e alguém decidiria em cima disso.
	var consultados, total int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM farol.cnpj_receita`).Scan(&consultados)
	db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM (
  SELECT DISTINCT regexp_replace(cnpj,'[^0-9]','','g') AS doc
    FROM farol.agg_fat_mkt_cli_mes WHERE empresa_id = $1 AND cnpj <> ''
) d WHERE length(d.doc) = 14`, empresaID).Scan(&total)
	if total > 0 {
		out.Cobertura = float64(consultados) / float64(total) * 100
	}
	out.Incompleto = out.Cobertura < 99

	return out, nil
}

// RelatorioClientesReceitaHandler — GET /api/v2/farol/relatorio/clientes-receita
//
//	?formato=json (padrão) | xlsx | pdf
//	?situacao=BAIXADA | INAPTA | SUSPENSA | TODAS
func RelatorioClientesReceitaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx := GetSpContext(r)
		if spCtx == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		situacao := r.URL.Query().Get("situacao")
		rel, err := consultarClientesReceita(r.Context(), db, spCtx.EmpresaID, situacao)
		if err != nil {
			log.Printf("[cnpj:relatorio] ERRO: %v", err)
			http.Error(w, `{"error":"falha ao consultar"}`, http.StatusInternalServerError)
			return
		}

		nome := "clientes-receita_" + time.Now().Format("20060102")
		switch strings.ToLower(r.URL.Query().Get("formato")) {
		case "xlsx":
			b, err := relatorioReceitaXLSX(rel)
			if err != nil {
				http.Error(w, `{"error":"falha ao gerar Excel"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.xlsx"`, nome))
			w.Write(b)
		case "pdf":
			logo, ext := logoRelatorio(db, spCtx.EmpresaID)
			b, err := relatorioReceitaPDF(rel, logo, ext)
			if err != nil {
				log.Printf("[cnpj:relatorio] PDF ERRO: %v", err)
				http.Error(w, `{"error":"falha ao gerar PDF"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, nome))
			w.Write(b)
		default:
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(rel)
		}
	}
}

// ─── Excel ────────────────────────────────────────────────────────────────────

func relatorioReceitaXLSX(rel relatorioReceita) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	const aba = "Clientes"
	f.SetSheetName(f.GetSheetName(0), aba)

	cab := []string{"CNPJ", "Razão Social", "Nome no cadastro", "Situação", "Desde",
		"CNAE", "Município", "UF", "Última compra",
		fmt.Sprintf("Líquido %d", rel.AnoAnt), fmt.Sprintf("Líquido %d", rel.AnoAtual),
		"Gerente", "Supervisor", "RCA",
		"Sucessora (CNPJ)", "Sucessora", "Evidência"}
	for i, h := range cab {
		cel, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(aba, cel, h)
	}
	negrito, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	f.SetRowStyle(aba, 1, 1, negrito)

	// CNPJ como TEXTO: com 14 dígitos o Excel converte para número e come o
	// zero à esquerda, quebrando qualquer PROCV que o gestor fizer depois.
	txt, _ := f.NewStyle(&excelize.Style{NumFmt: 49})
	f.SetColStyle(aba, "A", txt)
	f.SetColStyle(aba, "O", txt) // idem para o CNPJ da sucessora
	moeda, _ := f.NewStyle(&excelize.Style{NumFmt: 4}) // #,##0.00
	f.SetColStyle(aba, "J:K", moeda)

	for i, c := range rel.Linhas {
		l := i + 2
		vals := []any{c.CNPJ, c.RazaoSocial, c.NomeCadastro, c.Situacao, c.SituacaoData,
			c.CNAE, c.Municipio, c.UF, c.UltimaCompra,
			c.LiquidoAnt, c.LiquidoAtual, c.NomeGerente, c.NomeSupervisr, c.CodRCA,
			c.Sucessora, c.SucessoraNome, rotuloForca(c.SucessoraForca)}
		for j, v := range vals {
			cel, _ := excelize.CoordinatesToCellName(j+1, l)
			f.SetCellValue(aba, cel, v)
		}
	}
	larguras := map[string]float64{"A": 18, "B": 38, "C": 32, "D": 12, "E": 11,
		"F": 34, "G": 20, "H": 5, "I": 13, "J": 15, "K": 15, "L": 24, "M": 24, "N": 8,
		"O": 18, "P": 32, "Q": 22}
	for col, larg := range larguras {
		f.SetColWidth(aba, col, col, larg)
	}
	f.SetPanes(aba, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ─── PDF ──────────────────────────────────────────────────────────────────────

func relatorioReceitaPDF(rel relatorioReceita, logo []byte, ext extension.Type) ([]byte, error) {
	cfg := config.NewBuilder().
		WithPageNumber(props.PageNumber{Pattern: "Pág. {current}/{total}", Place: props.RightBottom}).
		WithLeftMargin(8).WithRightMargin(8).WithTopMargin(10).
		Build()
	mrt := maroto.New(cfg)

	pg := page.New()

	// Cabeçalho com logo
	pg.Add(
		row.New(14).Add(
			col.New(2).Add(image.NewFromBytes(logo, ext, props.Rect{Center: true, Percent: 80})),
			col.New(10).Add(
				text.New("Clientes com CNPJ irregular na Receita Federal",
					props.Text{Size: 13, Style: fontstyle.Bold, Align: align.Left, Top: 2}),
				text.New(fmt.Sprintf("Gerado em %s — fonte: Receita Federal via BrasilAPI", rel.GeradoEm),
					props.Text{Size: 8, Align: align.Left, Top: 9}),
			),
		),
	)

	// A ressalva de cobertura vai no PDF, não só na tela: o PDF é o que circula
	// por e-mail, descolado do contexto em que foi gerado.
	if rel.Incompleto {
		pg.Add(row.New(6).Add(col.New(12).Add(
			text.New(fmt.Sprintf("PARCIAL — %.0f%% da base consultada até agora. Os números abaixo tendem a crescer.", rel.Cobertura),
				props.Text{Size: 8, Style: fontstyle.BoldItalic, Align: align.Left}),
		)))
	}

	if rel.Reaberturas > 0 {
		pg.Add(row.New(5).Add(col.New(12).Add(
			text.New(fmt.Sprintf("Destes, %d são REABERTURA PROVÁVEL (R$ %s): há CNPJ ativo comprando no mesmo endereço. Não conte como perda.",
				rel.Reaberturas, moedaBR(rel.ReaberturasValor)),
				props.Text{Size: 8, Style: fontstyle.Bold}),
		)))
	}

	for _, s := range rel.Resumo {
		pg.Add(row.New(5).Add(col.New(12).Add(
			text.New(fmt.Sprintf("%s: %d cliente(s) — R$ %s faturados em %d",
				s.Situacao, s.Clientes, moedaBR(s.LiquidoAnt), rel.AnoAnt),
				props.Text{Size: 9, Style: fontstyle.Bold}),
		)))
	}

	// Cabeçalho um ponto menor que a célula: "Últ. compra" e "Reabertura" têm
	// 11 e 10 caracteres numa coluna de 16,2mm, e a 7pt não cabiam.
	cab := props.Text{Size: 6, Style: fontstyle.Bold}
	pg.Add(row.New(6).Add(
		col.New(2).Add(text.New("CNPJ", cab)),
		col.New(2).Add(text.New("Razão social", cab)),
		col.New(1).Add(text.New("Situação", cab)),
		col.New(1).Add(text.New("Desde", cab)),
		col.New(2).Add(text.New("Município/UF", cab)),
		col.New(1).Add(text.New("Últ. compra", cab)),
		col.New(1).Add(text.New("Reabertura", cab)),
		col.New(2).Add(text.New(fmt.Sprintf("Líquido %d", rel.AnoAnt), props.Text{Size: 6, Style: fontstyle.Bold, Align: align.Right})),
	))

	cel := props.Text{Size: fonteCelula}
	for _, c := range rel.Linhas {
		pg.Add(row.New(5).Add(
			col.New(2).Add(text.New(fmtCNPJ(c.CNPJ), cel)),
			col.New(2).Add(text.New(cabeNaColuna(semPrefixoCNPJ(primeiroNaoVazio(c.RazaoSocial, c.NomeCadastro)), 2), cel)),
			col.New(1).Add(text.New(c.Situacao, cel)),
			col.New(1).Add(text.New(c.SituacaoData, cel)),
			col.New(2).Add(text.New(cabeNaColunaReserva(c.Municipio, 2, 3)+"/"+c.UF, cel)),
			col.New(1).Add(text.New(c.UltimaCompra, cel)),
			col.New(1).Add(text.New(marcaReabertura(c.SucessoraForca), cel)),
			col.New(2).Add(text.New(moedaBR(c.LiquidoAnt), props.Text{Size: fonteCelula, Align: align.Right})),
		))
	}

	pg.Add(row.New(8).Add(col.New(12).Add(
		text.New("Reabertura: SIM = CNPJ ativo comprando no mesmo endereço, e com a mesma placa ou em endereço de até 2 ocupantes. "+
			"\"talvez\" = mesmo endereço, mas em galeria com muitos CNPJs, onde o vizinho se confunde com o sucessor.",
			props.Text{Size: 6, Top: 3}),
	)))

	mrt.AddPages(pg)
	doc, err := mrt.Generate()
	if err != nil {
		return nil, err
	}
	return doc.GetBytes(), nil
}

// rotuloForca — traduz o código interno para o que o gestor lê. "galeria" diz
// explicitamente que é fraco, senão vira certeza na cabeça de quem lê a planilha.
func rotuloForca(f string) string {
	switch f {
	case "placa":
		return "mesma placa e endereço"
	case "endereco":
		return "mesmo endereço"
	case "galeria":
		return "mesmo endereço (galeria — fraco)"
	}
	return ""
}

// marcaReabertura — compacta para a coluna estreita do PDF. "talvez" para o
// caso de galeria: mostrar como SIM transformaria um palpite fraco em fato para
// quem só lê a tabela.
func marcaReabertura(f string) string {
	switch f {
	case "placa", "endereco":
		return "SIM"
	case "galeria":
		return "talvez"
	}
	return ""
}

// semPrefixoCNPJ tira o CNPJ que a Receita prefixa na razão social do MEI
// ("57.463.500 RUBENS OLIVEIRA"). Na tabela o CNPJ já é a primeira coluna, então
// o prefixo só consome 11 dos ~21 caracteres que cabem — e é justamente o nome
// da pessoa, a parte que identifica a loja, que sobrava de fora.
func semPrefixoCNPJ(s string) string {
	r := []rune(strings.TrimSpace(s))
	// Formato: dd.ddd.ddd seguido de espaço.
	if len(r) < 12 || r[2] != '.' || r[6] != '.' || r[10] != ' ' {
		return s
	}
	for _, i := range []int{0, 1, 3, 4, 5, 7, 8, 9} {
		if r[i] < '0' || r[i] > '9' {
			return s
		}
	}
	return strings.TrimSpace(string(r[11:]))
}

func fmtCNPJ(d string) string {
	if len(d) != 14 {
		return d
	}
	return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
}

func corta(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// Geometria da tabela do PDF. A4 tem 210mm; com as margens de 8mm de cada lado
// sobram 194mm para as 12 colunas do maroto.
const (
	fonteCelula   = 6.5  // pontos
	larguraColMM  = 194.0 / 12.0
	// Largura média de um caractere em CAIXA ALTA, que é como vem razão social
	// da Receita. Caixa alta é ~20% mais larga que o texto misto, e usar a média
	// geral aqui foi exatamente o erro que quebrou o layout.
	larguraCharMM = fonteCelula * 0.353 * 0.63
)

// cabeNaColuna corta o texto para caber em UMA linha de `cols` colunas.
//
// Sem isso a célula quebra em duas linhas dentro de uma linha de tabela de 4,5mm
// (12,8 pontos), e a segunda linha invade a linha seguinte — a sobreposição
// relatada em 26/08/2026. Medido no PDF gerado: baselines a 7,0 e 5,7 pontos
// dentro de um passo de 12,8.
func cabeNaColuna(s string, cols int) string {
	return cabeNaColunaReserva(s, cols, 0)
}

// cabeNaColunaReserva — idem, guardando `reserva` caracteres para o que será
// concatenado depois. Sem isso, cortar o município para ocupar a coluna inteira
// e só então acrescentar "/BA" devolve o estouro pela porta dos fundos.
func cabeNaColunaReserva(s string, cols, reserva int) string {
	max := int(float64(cols)*larguraColMM/larguraCharMM) - 1 - reserva
	if max < 4 {
		max = 4
	}
	return corta(s, max)
}

func primeiroNaoVazio(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// moedaBR — separador de milhar e vírgula decimal, no formato brasileiro. O Go
// não traz isso pronto e o relatório é lido por gestor, não por programador.
func moedaBR(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	partes := strings.SplitN(s, ".", 2)
	inteiro, dec := partes[0], partes[1]
	var b strings.Builder
	for i, ch := range inteiro {
		if i > 0 && (len(inteiro)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(ch)
	}
	out := b.String() + "," + dec
	if neg {
		out = "-" + out
	}
	return out
}
