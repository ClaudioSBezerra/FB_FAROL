package handlers

// farol_comparativo_rel322.go — Comparativo REL 322 (WinThor) x Farol.
//
// POR QUE ELE EXISTE. O gestor precisa validar se o Farol bate com o
// relatório oficial do WinThor ("322 — Venda Por Departamento", por
// supervisor), mas de onde a comparação é feita não se alcança nem o Oracle
// de origem (BD VM) nem o Postgres de produção do Farol — só o PDF que o
// WinThor já exporta está disponível. O usuário sobe esse PDF; o backend
// extrai as linhas por supervisor, lê o período do próprio cabeçalho e busca
// no Farol o Bruto e o Líquido para os mesmos cod_supervisor no mesmo
// intervalo. Nada é persistido — cada upload é independente.
//
// LAYOUT DO TEXTO EXTRAÍDO. github.com/ledongthuc/pdf.GetPlainText devolve
// um "token" por linha (uma palavra ou um número por linha, na ordem em que
// foram desenhados no PDF) — não uma linha de texto por linha visual da
// tabela. Cada bloco de cabeçalho (repetido a cada página) começa na linha
// "Código" e termina na linha "Todos os Períodos". Cada linha de dado é:
// código (dígitos) + descrição (um ou mais tokens não-numéricos, com espaços
// e hífens) + exatamente 11 tokens numéricos, dos quais o PRIMEIRO é o
// Vl.Vendido — confirmado batendo a soma de todas as linhas contra o valor
// declarado após "N Supervisores Listados" nos 4 PDFs de exemplo em
// /home/claudio/uploads/*.pdf (bruto: 129.957.080,67 / 1.886.956.234,67 /
// 1.269.150.898,17 / 154.725.962,23 — todos batem exatos).

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/lib/pq"
)

// ─── Tipos ──────────────────────────────────────────────────────────────────

// linhaExtraidaRel322 — uma linha de supervisor lida do PDF, antes de cruzar
// com o Farol.
type linhaExtraidaRel322 struct {
	CodSupervisor string
	Descricao     string
	VlVendido     float64
}

// rel322Parsed — resultado da extração do PDF: período + linhas de supervisor.
type rel322Parsed struct {
	PeriodoTexto string // "01/08/2026 a 26/08/2026", como impresso no PDF
	DataInicio   time.Time
	DataFim      time.Time
	Linhas       []linhaExtraidaRel322
}

// linhaComparativoRel322 — uma linha da resposta da API, já cruzada com o
// Farol. Ponteiros distinguem "0 apurado" de "não existe deste lado".
type linhaComparativoRel322 struct {
	CodSupervisor string   `json:"cod_supervisor"`
	Supervisor    string   `json:"supervisor"`
	VlVendidoPDF  *float64 `json:"vl_vendido_pdf"`
	BrutoFarol    *float64 `json:"bruto_farol"`
	LiquidoFarol  *float64 `json:"liquido_farol"`
	DiferencaPct  *float64 `json:"diferenca_pct"` // menor distância (Bruto ou Líquido) até o Vl.Vendido do PDF
	Status        string   `json:"status"`        // ok | divergencia | orfao
	Origem        string   `json:"origem"`        // pdf | farol | ambos
}

// comparativoRel322Resposta — payload devolvido ao front.
type comparativoRel322Resposta struct {
	Periodo               string                   `json:"periodo"`
	DataInicio            string                   `json:"data_inicio"`
	DataFim               string                   `json:"data_fim"`
	Linhas                []linhaComparativoRel322 `json:"linhas"`
	TotalVlVendidoPDF     float64                  `json:"total_vl_vendido_pdf"`
	TotalBrutoFarol       float64                  `json:"total_bruto_farol"`
	TotalLiquidoFarol     float64                  `json:"total_liquido_farol"`
	QtdSupervisoresPDF    int                      `json:"qtd_supervisores_pdf"`
	QtdDivergencias       int                      `json:"qtd_divergencias"`
	QtdOrfaos             int                      `json:"qtd_orfaos"`
	SemDadoFarolNoPeriodo bool                     `json:"sem_dado_farol_no_periodo"`
}

// tipoVendaReal — códigos que entram na venda real (mesma classificação do
// painel faturado — ver spec-venda-liquida-composicao.md e a migration
// 187_tipo_venda_faturado.sql). Bonificação(5)/Transferência(10)/Remessa(13)
// ficam de fora.
var tipoVendaReal = []string{"1", "4", "7", "8", "9", "11", "14", "20"}

// ─── Extração de texto do PDF ─────────────────────────────────────────────

// extrairTextoPDF — github.com/ledongthuc/pdf é lib de terceiros com
// histórico de panic em entrada malformada (não só error). Sem o recover,
// um PDF corrompido derrubaria a goroutine da requisição inteira em vez de
// virar um 422 normal — o defer converte qualquer panic em erro comum,
// mantendo a mesma mensagem que o handler já usa para esse caminho.
func extrairTextoPDF(dados []byte) (texto string, err error) {
	defer func() {
		if r := recover(); r != nil {
			texto = ""
			err = fmt.Errorf("falha ao processar PDF (arquivo corrompido?): %v", r)
		}
	}()

	reader, err := pdf.NewReader(bytes.NewReader(dados), int64(len(dados)))
	if err != nil {
		return "", fmt.Errorf("arquivo não é um PDF válido: %w", err)
	}
	txt, err := reader.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("falha ao extrair texto do PDF (PDF sem texto real / escaneado?): %w", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, txt); err != nil {
		return "", fmt.Errorf("falha ao ler texto extraído: %w", err)
	}
	return buf.String(), nil
}

// ─── Parser do REL 322 ─────────────────────────────────────────────────────
//
// Como a descrição do supervisor tem espaços e hífens
// ("GO - VALE SAO PATRICIO - LUCAS"), não dá para dividir por espaço.
// Percorre-se a sequência de tokens: um código (linha só de dígitos) inicia
// a linha; tudo que não é numérico até o próximo bloco de 11 números é
// descrição; os 11 números seguintes são Qt.Cli.Ativos, Qt.Cli.Pos., etc, na
// ordem em que o PDF os desenha — o primeiro deles é sempre o Vl.Vendido
// (confirmado empiricamente, ver comentário no topo do arquivo).

var (
	reNumTokenRel322 = regexp.MustCompile(`^-?\d[\d.]*(,\d+)?$`)
	rePaginaRel322   = regexp.MustCompile(`^Pagina\s*:\s*\d+$`)
	reCodigoRel322   = regexp.MustCompile(`^\d+$`)
	rePeriodoRel322  = regexp.MustCompile(`^(\d{2}/\d{2}/\d{4})\s+a\s+(\d{2}/\d{2}/\d{4})$`)
)

// parseRel322Texto — recebe o texto já extraído do PDF (uma "palavra"/número
// por linha) e devolve o período + as linhas de supervisor. Erro claro
// sempre que o layout não bate com o esperado — nunca um comparativo parcial
// silencioso (constraint do spec).
func parseRel322Texto(texto string) (*rel322Parsed, error) {
	var lines []string
	for _, l := range strings.Split(texto, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}

	achouTitulo := false
	achouTipo := false
	for _, l := range lines {
		if l == "322 - Venda Por Departamento" {
			achouTitulo = true
		}
		if l == "14-Por Supervisor" {
			achouTipo = true
		}
	}
	if !achouTitulo {
		return nil, fmt.Errorf("layout não reconhecido: o PDF não é o relatório \"322 - Venda Por Departamento\"")
	}
	if !achouTipo {
		return nil, fmt.Errorf("layout não reconhecido: o PDF não está no formato \"14-Por Supervisor\"")
	}

	// Remove os blocos de cabeçalho (repetidos a cada página, de "Código" até
	// "Todos os Períodos"), capturando o valor de "Periodo :" pelo caminho.
	periodoTexto := ""
	var out []string
	i := 0
	for i < len(lines) {
		if lines[i] == "Código" {
			j := i
			fechou := false
			for j < len(lines) {
				if lines[j] == "Periodo :" && j+1 < len(lines) && periodoTexto == "" {
					periodoTexto = lines[j+1]
				}
				if lines[j] == "Todos os Períodos" {
					fechou = true
					break
				}
				j++
			}
			if !fechou {
				return nil, fmt.Errorf("layout não reconhecido: cabeçalho do PDF incompleto")
			}
			i = j + 1
			continue
		}
		if rePaginaRel322.MatchString(lines[i]) {
			i++
			continue
		}
		out = append(out, lines[i])
		i++
	}

	if periodoTexto == "" {
		return nil, fmt.Errorf(`layout não reconhecido: não encontrei a linha "Periodo :" no cabeçalho`)
	}
	m := rePeriodoRel322.FindStringSubmatch(periodoTexto)
	if m == nil {
		return nil, fmt.Errorf("período %q em formato inesperado (esperado DD/MM/AAAA a DD/MM/AAAA)", periodoTexto)
	}
	dataInicio, err1 := time.Parse("02/01/2006", m[1])
	dataFim, err2 := time.Parse("02/01/2006", m[2])
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("período %q com datas inválidas", periodoTexto)
	}

	// Percorre as linhas de dado até "Supervisores Listados" (início do
	// bloco de totais, que não é uma linha de supervisor).
	var linhas []linhaExtraidaRel322
	vistos := map[string]bool{}
	i = 0
	n := len(out)
	for i < n {
		l := out[i]
		if l == "Supervisores Listados" {
			break
		}
		if !reCodigoRel322.MatchString(l) {
			return nil, fmt.Errorf("layout não reconhecido: esperava um código de supervisor e encontrei %q", l)
		}
		codigo := l
		if vistos[codigo] {
			// PDF corrompido/editado: o mesmo supervisor apareceria duas vezes
			// e o Farol dele seria somado em dobro nos totais, silenciosamente.
			return nil, fmt.Errorf("layout não reconhecido: código de supervisor %s aparece duplicado no PDF", codigo)
		}
		vistos[codigo] = true
		i++

		var desc []string
		for i < n && !reNumTokenRel322.MatchString(out[i]) {
			desc = append(desc, out[i])
			i++
		}

		var nums []string
		for i < n && reNumTokenRel322.MatchString(out[i]) && len(nums) < 11 {
			nums = append(nums, out[i])
			i++
		}
		if len(nums) != 11 {
			return nil, fmt.Errorf("layout não reconhecido: supervisor %s tem %d valor(es) numérico(s), esperava 11", codigo, len(nums))
		}

		vlVendido, err := parseNumeroBR(nums[0])
		if err != nil {
			return nil, fmt.Errorf("layout não reconhecido: Vl.Vendido inválido (%q) para o supervisor %s", nums[0], codigo)
		}

		linhas = append(linhas, linhaExtraidaRel322{
			CodSupervisor: codigo,
			Descricao:     strings.TrimSpace(strings.Join(desc, " ")),
			VlVendido:     vlVendido,
		})
	}

	if len(linhas) == 0 {
		return nil, fmt.Errorf("layout não reconhecido: nenhuma linha de supervisor encontrada no PDF")
	}

	return &rel322Parsed{
		PeriodoTexto: periodoTexto,
		DataInicio:   dataInicio,
		DataFim:      dataFim,
		Linhas:       linhas,
	}, nil
}

// parseNumeroBR — "9.223.911,54" → 9223911.54 (separador de milhar '.',
// decimal ',', como todo número do WinThor).
func parseNumeroBR(s string) (float64, error) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	if neg {
		v = -v
	}
	return v, nil
}

// ─── Cruzamento com o Farol ─────────────────────────────────────────────────

// escopoCondRel322 — monta a cláusula SQL que aplica o recorte por persona
// (farol_escopo.go) às duas CTEs de farolBrutoLiquidoPorSupervisor. Extraída
// à parte para ser testável sem *sql.DB.
//
// REGRA DE OURO (mesma de farol_escopo.go): o escopo SOBRESCREVE. Se por
// algum motivo esta função for chamada com Negar=true — não deveria, o
// handler já bloqueia antes com 403 — devolve uma condição impossível
// (1=0) em vez de nenhum filtro: falha fechado, nunca aberto.
func escopoCondRel322(escopo escopoRecorte, args []any) (string, []any) {
	if !escopo.restrito() {
		return "", args
	}
	if escopo.Negar {
		return " AND 1=0", args
	}
	args = append(args, pq.Array(escopo.Vals))
	return fmt.Sprintf(" AND %s = ANY($%d)", escopo.Col, len(args)), args
}

// farolBrutoLiquido — Bruto e Líquido por cod_supervisor no range de datas,
// já recortado pelo escopo do usuário logado (ggv/supervisor/rca só veem a
// própria equipe — mesma regra de farol_relatorios.go, ver farol_escopo.go).
//
// Bruto = SUM(pvenda) de vendas_faturadas (todas as linhas FATURADO).
// Líquido = venda real (tipo_venda em 1,4,7,8,9,11,14,20) menos
// DEVOLVIDO/CANCELADO de vendas_ccd — mesma classificação do painel faturado
// (spec-venda-liquida-composicao.md). Consulta as tabelas DIÁRIAS
// (vendas_faturadas/vendas_ccd), não os agregados mensais: o período do PDF
// pode ser parcial (mês em andamento).
func farolBrutoLiquidoPorSupervisor(ctx context.Context, db *sql.DB, empresaID string, dataInicio, dataFim time.Time, escopo escopoRecorte) (map[string]struct{ Bruto, Liquido float64 }, error) {
	args := []any{empresaID, dataInicio, dataFim, pq.Array(tipoVendaReal)}
	escopoCond, args := escopoCondRel322(escopo, args)

	q := fmt.Sprintf(`
WITH fat AS (
    SELECT trim(cod_supervisor) AS cod_supervisor,
           SUM(pvenda) AS bruto,
           SUM(pvenda) FILTER (WHERE tipo_venda = ANY($4)) AS venda_real
      FROM vendas_faturadas
     WHERE empresa_id = $1 AND data_faturamento BETWEEN $2 AND $3%s
     GROUP BY trim(cod_supervisor)
), ccd AS (
    SELECT trim(cod_supervisor) AS cod_supervisor,
           SUM(pvenda) AS descontos
      FROM vendas_ccd
     WHERE empresa_id = $1 AND data_evento BETWEEN $2 AND $3
       AND evento IN ('DEVOLVIDO', 'CANCELADO')%s
     GROUP BY trim(cod_supervisor)
)
SELECT COALESCE(fat.cod_supervisor, ccd.cod_supervisor) AS cod_supervisor,
       COALESCE(fat.bruto, 0)::float8 AS bruto,
       (COALESCE(fat.venda_real, 0) - COALESCE(ccd.descontos, 0))::float8 AS liquido
  FROM fat
  FULL OUTER JOIN ccd ON ccd.cod_supervisor = fat.cod_supervisor`, escopoCond, escopoCond)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]struct{ Bruto, Liquido float64 }{}
	for rows.Next() {
		var cod string
		var bruto, liquido float64
		if err := rows.Scan(&cod, &bruto, &liquido); err != nil {
			return nil, err
		}
		out[cod] = struct{ Bruto, Liquido float64 }{Bruto: bruto, Liquido: liquido}
	}
	return out, rows.Err()
}

// montarComparativo — busca o Farol (já recortado pelo escopo do usuário) e
// cruza com as linhas do PDF. Fina: toda a regra de status vive em
// cruzarComRel322 (pura, testável sem banco).
func montarComparativo(ctx context.Context, db *sql.DB, empresaID string, parsed *rel322Parsed, escopo escopoRecorte) (*comparativoRel322Resposta, error) {
	farol, err := farolBrutoLiquidoPorSupervisor(ctx, db, empresaID, parsed.DataInicio, parsed.DataFim, escopo)
	if err != nil {
		return nil, err
	}
	return cruzarComRel322(farol, parsed), nil
}

// cruzarComRel322 — aplica a regra de status (tolerância 0,5%, órfãos,
// período sem dado) sobre o resultado já buscado do Farol. Separada de
// montarComparativo para ser testável sem *sql.DB.
func cruzarComRel322(farol map[string]struct{ Bruto, Liquido float64 }, parsed *rel322Parsed) *comparativoRel322Resposta {
	// Período sem NENHUM dado no Farol (range futuro ou ainda não
	// importado): reflete a realidade, todas as linhas viram divergência
	// (Farol = 0), não um conjunto de órfãos — é uma condição sistêmica, não
	// um cod_supervisor isolado sem correspondente.
	semDadoNoPeriodo := len(farol) == 0

	usados := map[string]bool{}
	var linhas []linhaComparativoRel322
	var totalPDF, totalBruto, totalLiquido float64
	var qtdDivergencias, qtdOrfaos int

	for _, l := range parsed.Linhas {
		totalPDF += l.VlVendido
		vlPDF := l.VlVendido

		fl, achou := farol[l.CodSupervisor]
		if !achou && !semDadoNoPeriodo {
			// Farol tem dado no período, só não para ESTE supervisor: órfão.
			qtdOrfaos++
			linhas = append(linhas, linhaComparativoRel322{
				CodSupervisor: l.CodSupervisor,
				Supervisor:    l.Descricao,
				VlVendidoPDF:  f64p(vlPDF),
				Status:        "orfao",
				Origem:        "pdf",
			})
			continue
		}
		usados[l.CodSupervisor] = true

		bruto, liquido := fl.Bruto, fl.Liquido
		totalBruto += bruto
		totalLiquido += liquido

		pctBruto := percentDiffRel322(bruto, vlPDF)
		pctLiquido := percentDiffRel322(liquido, vlPDF)
		melhorPct := pctBruto
		if pctLiquido < melhorPct {
			melhorPct = pctLiquido
		}

		status := "divergencia"
		if melhorPct <= 0.005 {
			status = "ok"
		} else {
			qtdDivergencias++
		}

		// PDF=0 e Farol!=0 faz percentDiffRel322 devolver +Inf. encoding/json
		// não sabe serializar Infinity — se esse valor chegasse até aqui,
		// json.NewEncoder(w).Encode falharia e o cliente receberia um 200 com
		// corpo vazio/truncado, o "comparativo parcial silencioso" que o spec
		// proíbe. Vira nil (o front já trata como "—"); o status continua
		// "divergencia" — nunca "ok" nesse caso, já garantido pela tolerância.
		var diferencaPct *float64
		if !math.IsInf(melhorPct, 0) {
			diferencaPct = f64p(melhorPct * 100)
		}

		linhas = append(linhas, linhaComparativoRel322{
			CodSupervisor: l.CodSupervisor,
			Supervisor:    l.Descricao,
			VlVendidoPDF:  f64p(vlPDF),
			BrutoFarol:    f64p(bruto),
			LiquidoFarol:  f64p(liquido),
			DiferencaPct:  diferencaPct,
			Status:        status,
			Origem:        "ambos",
		})
	}

	// Supervisores com dado no Farol no período, mas ausentes do PDF —
	// órfãos do outro lado. Só faz sentido quando o Farol TEM dado no
	// período (senão já caímos no caso sistêmico acima, e todo cod_supervisor
	// do Farol estaria "ausente" por definição de len(farol)==0).
	if !semDadoNoPeriodo {
		// Iteração de map não tem ordem estável — sem ordenar, a mesma
		// resposta viria em ordem diferente a cada upload do MESMO PDF.
		var codsOrfaosFarol []string
		for cod := range farol {
			if !usados[cod] {
				codsOrfaosFarol = append(codsOrfaosFarol, cod)
			}
		}
		sort.Strings(codsOrfaosFarol)

		for _, cod := range codsOrfaosFarol {
			fl := farol[cod]
			qtdOrfaos++
			totalBruto += fl.Bruto
			totalLiquido += fl.Liquido
			linhas = append(linhas, linhaComparativoRel322{
				CodSupervisor: cod,
				BrutoFarol:    f64p(fl.Bruto),
				LiquidoFarol:  f64p(fl.Liquido),
				Status:        "orfao",
				Origem:        "farol",
			})
		}
	}

	return &comparativoRel322Resposta{
		Periodo:               parsed.PeriodoTexto,
		DataInicio:            parsed.DataInicio.Format("2006-01-02"),
		DataFim:               parsed.DataFim.Format("2006-01-02"),
		Linhas:                linhas,
		TotalVlVendidoPDF:     totalPDF,
		TotalBrutoFarol:       totalBruto,
		TotalLiquidoFarol:     totalLiquido,
		QtdSupervisoresPDF:    len(parsed.Linhas),
		QtdDivergencias:       qtdDivergencias,
		QtdOrfaos:             qtdOrfaos,
		SemDadoFarolNoPeriodo: semDadoNoPeriodo,
	}
}

// percentDiffRel322 — distância percentual entre o valor do Farol e o
// Vl.Vendido do PDF. PDF=0 e Farol=0 é 0% (bate); PDF=0 e Farol≠0 é tratado
// como divergência máxima (sem divisão por zero).
func percentDiffRel322(farolValor, pdfValor float64) float64 {
	if pdfValor == 0 {
		if farolValor == 0 {
			return 0
		}
		return math.Inf(1)
	}
	return math.Abs(farolValor-pdfValor) / math.Abs(pdfValor)
}

func f64p(v float64) *float64 { return &v }

// ─── Handler HTTP ───────────────────────────────────────────────────────────

// ComparativoRel322Handler — POST /api/v2/farol/relatorio/comparativo-rel322
//
// Multipart, campo "file": o PDF do REL 322 (WinThor, "Por Supervisor").
// Leitura pura (não persiste nada) — permissão somente_leitura, mesmo sendo
// POST, porque o upload é só o transporte da consulta, não uma escrita.
func ComparativoRel322Handler(db *sql.DB) http.HandlerFunc {
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

		// Recorte por persona (farol_escopo.go) — mesma regra que
		// /extrato-produto-cliente já aplica: sem isto, um supervisor logado
		// (sp_role=somente_leitura, liberado por esta rota) enxergaria o
		// Bruto/Líquido de TODOS os supervisores da empresa só fazendo upload
		// de um PDF que os liste. Checado ANTES de gastar trabalho com o
		// upload — negar não depende do PDF.
		escopo := escopoDoUsuario(db, spCtx, "")
		if escopo.restrito() && escopo.Negar {
			http.Error(w, "Sem escopo definido para este usuário", http.StatusForbidden)
			return
		}

		// MaxBytesReader antes do ParseMultipartForm: o segundo só define o
		// limiar de MEMÓRIA (acima disso o multipart transborda pra um
		// arquivo temporário em disco, sem teto de tamanho nenhum). 25MB dá
		// folga sobre os 20MB do ParseMultipartForm.
		r.Body = http.MaxBytesReader(w, r.Body, 25<<20)

		if err := r.ParseMultipartForm(20 << 20); err != nil {
			http.Error(w, jsonErrorRel322("falha ao ler o upload: "+err.Error()), http.StatusBadRequest)
			return
		}
		// Upload grande transborda pra disco (arquivo temporário do
		// multipart) — sem isto, ele nunca seria removido depois do request.
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		file, _, ferr := r.FormFile("file")
		if ferr != nil {
			http.Error(w, jsonErrorRel322(`arquivo não enviado (campo "file")`), http.StatusBadRequest)
			return
		}
		defer file.Close()

		dados, rerr := io.ReadAll(file)
		if rerr != nil {
			http.Error(w, jsonErrorRel322("falha ao ler o arquivo enviado"), http.StatusBadRequest)
			return
		}

		texto, perr := extrairTextoPDF(dados)
		if perr != nil {
			log.Printf("[comparativo322] extração PDF ERRO: %v", perr)
			http.Error(w, jsonErrorRel322("não foi possível ler o PDF: "+perr.Error()), http.StatusUnprocessableEntity)
			return
		}

		parsed, perr2 := parseRel322Texto(texto)
		if perr2 != nil {
			http.Error(w, jsonErrorRel322(perr2.Error()), http.StatusUnprocessableEntity)
			return
		}

		resultado, qerr := montarComparativo(r.Context(), db, spCtx.EmpresaID, parsed, escopo)
		if qerr != nil {
			log.Printf("[comparativo322] consulta Farol ERRO: %v", qerr)
			http.Error(w, jsonErrorRel322("falha ao consultar o Farol"), http.StatusInternalServerError)
			return
		}

		// Defesa em profundidade: depois do fix do DiferencaPct (+Inf → nil)
		// isto não deveria mais falhar, mas se algum outro campo algum dia
		// carregar um valor não serializável, não pode ficar invisível — sem
		// o log, o cliente recebe 200 com corpo vazio/truncado e ninguém sabe.
		if err := json.NewEncoder(w).Encode(resultado); err != nil {
			log.Printf("[comparativo322] falha ao serializar resposta: %v", err)
		}
	}
}

func jsonErrorRel322(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}
