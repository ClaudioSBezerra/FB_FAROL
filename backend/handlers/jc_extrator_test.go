package handlers

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

// TestValorCSV — a conversão driver→CSV é o ponto onde dado se corrompe em
// SILÊNCIO: um float em notação científica ou uma data com hora não quebram a
// importação, só entram errados no banco. Daí o teste ser exaustivo nos tipos
// que o go-ora devolve.
func TestValorCSV(t *testing.T) {
	casos := []struct {
		nome string
		in   any
		want string
	}{
		{"nil vira vazio", nil, ""},
		{"data sem hora", time.Date(2026, 7, 28, 14, 30, 0, 0, time.UTC), "2026-07-28"},
		{"string passa direto", "NESTLE BRASIL", "NESTLE BRASIL"},
		{"bytes viram string", []byte("ABC"), "ABC"},
		{"inteiro", int64(1234), "1234"},
		{"float com decimais", 1234.56, "1234.56"},
		{"float grande SEM notacao cientifica", 1234567.89, "1234567.89"},
		{"float que seria 1e+06", 1000000.0, "1000000"},
		{"float negativo", -45.9, "-45.9"},
		{"zero", 0.0, "0"},
		{"bool true", true, "1"},
		{"bool false", false, "0"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			if got := valorCSV(c.in); got != c.want {
				t.Errorf("valorCSV(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestValorCSVFloatNuncaCientifico — garantia extra do caso mais perigoso: o
// importador usa strconv.ParseFloat, que ATÉ aceita "1.23e+06", mas o parseNum
// dele mexe em ponto/vírgula antes e destruiria a notação. Nenhum valor
// monetário plausível pode sair em notação científica.
func TestValorCSVFloatNuncaCientifico(t *testing.T) {
	valores := []float64{
		1e6, 1e7, 1e9, 0.000001, 123456789.12, 999999999999.99,
	}
	for _, v := range valores {
		got := valorCSV(v)
		if strings.ContainsAny(got, "eE") {
			t.Errorf("valorCSV(%v) = %q — notação científica quebraria o parseNum do importador", v, got)
		}
	}
}

// TestColunasJCBatemComImportador — as 40 colunas precisam ser exatamente as que
// o Keslley entregou. Se alguém reordenar ou renomear aqui sem mexer na origem,
// o CSV sai com cabeçalho certo mas dado trocado de coluna — e o importador,
// que mapeia POR NOME, importaria valores no campo errado sem erro nenhum.
func TestColunasJCBatemComImportador(t *testing.T) {
	if len(colunasJC) != 40 {
		t.Fatalf("esperava 40 colunas, tem %d", len(colunasJC))
	}
	// Amostra das que o importador procura com nome peculiar — as pegadinhas
	// reais do layout (CODEPTO com um D só; underscores que a normalização come).
	obrigatorias := []string{
		"DATA", "PERIODO", "ESTADO", "CODEPTO", "QTRCA_SUPERVISOR", "QTCLI_RCA",
		"CODUSUR", "PVENDA_TOTAL", "CONDVENDA", "DESC_CONDVENDA", "CNPJ",
	}
	for _, c := range obrigatorias {
		if indiceDe(colunasJC, c) < 0 {
			t.Errorf("coluna obrigatória %q ausente", c)
		}
	}
	vistos := map[string]bool{}
	for _, c := range colunasJC {
		if vistos[c] {
			t.Errorf("coluna duplicada: %s", c)
		}
		vistos[c] = true
	}
}

// TestCorpoResumoTrazVereditoNaPrimeiraLinha — quem lê pela notificação do
// celular vê só o começo. Se o veredito ficasse no fim, o e-mail perderia a
// função de alerta.
func TestCorpoResumoTrazVereditoNaPrimeiraLinha(t *testing.T) {
	base := time.Date(2026, 7, 28, 6, 30, 0, 0, time.UTC)

	casos := []struct {
		nome        string
		res         *ResultadoExtracao
		queroNoAsst string
		queroNoIni  string
	}{
		{
			nome: "sucesso",
			res: &ResultadoExtracao{
				DataRef: base, Inicio: base, Fim: base.Add(3 * time.Minute),
				LinhasLidas: 88067, LinhasImportad: 88067, StatusImport: "done",
				PorEstado: map[string]int{"FATURADO": 48000, "TRANSMITIDO": 40000},
			},
			queroNoAsst: "OK", queroNoIni: "OK",
		},
		{
			nome: "falha",
			res: &ResultadoExtracao{
				DataRef: base, Inicio: base, Fim: base.Add(time.Minute),
				Erro: errFake("conexão recusada"),
			},
			queroNoAsst: "FALHOU", queroNoIni: "FALHOU",
		},
		{
			nome: "sem dados",
			res: &ResultadoExtracao{
				DataRef: base, Inicio: base, Fim: base.Add(time.Second),
				StatusImport: "sem_dados",
			},
			queroNoAsst: "SEM DADOS", queroNoIni: "SEM DADOS",
		},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			assunto, corpo := corpoResumoJC(c.res)
			if !strings.Contains(assunto, c.queroNoAsst) {
				t.Errorf("assunto %q não contém %q", assunto, c.queroNoAsst)
			}
			primeira := strings.SplitN(corpo, "\n", 2)[0]
			if !strings.Contains(primeira, c.queroNoIni) {
				t.Errorf("1ª linha %q não contém %q", primeira, c.queroNoIni)
			}
			if !strings.Contains(corpo, "28/07/2026") {
				t.Error("corpo não traz o dia de referência")
			}
			if !strings.Contains(corpo, "Início") || !strings.Contains(corpo, "Conclusão") {
				t.Error("corpo precisa ter início e conclusão — foi o pedido explícito")
			}
		})
	}
}

// TestCorpoResumoDenunciaEstadoDesconhecido — se a origem passar a emitir um
// ESTADO novo, o roteamento do importador o trataria como FATURADO (é o
// default). Melhor descobrir pelo e-mail que por número errado na tela.
func TestCorpoResumoDenunciaEstadoDesconhecido(t *testing.T) {
	res := &ResultadoExtracao{
		DataRef: time.Now(), Inicio: time.Now(), Fim: time.Now(),
		LinhasLidas: 10, StatusImport: "done",
		PorEstado: map[string]int{"FATURADO": 8, "BONIFICADO": 2},
	}
	_, corpo := corpoResumoJC(res)
	if !strings.Contains(corpo, "BONIFICADO") || !strings.Contains(corpo, "não previsto") {
		t.Errorf("estado desconhecido deveria ser destacado, corpo:\n%s", corpo)
	}
}

// TestCSVIdaEVoltaPeloParserDoImportador — o teste que faltava.
//
// Em 29/07 a extração leu 152.599 linhas do Oracle, gerou o CSV, e o importador
// rejeitou TUDO com "nenhuma linha válida": eu escrevi com vírgula e ele lê com
// `;` (farol_v2_import.go:297). Nada no build acusava, e os logs da extração
// pareciam perfeitos — o erro só apareceu na coluna `importados` do job.
//
// Este teste escreve com o nosso escritor e lê com EXATAMENTE a configuração do
// importador, depois refaz o mapeamento de cabeçalho dele. Se o delimitador
// divergir de novo, quebra aqui.
func TestCSVIdaEVoltaPeloParserDoImportador(t *testing.T) {
	linha := []string{
		"2026-07-28", "202607", "FATURADO",
		"1", "GERENTE UM", "10", "SUPERVISOR X", "5",
		"100", "RCA CEM", "250",
		"F01", "NESTLE BRASIL",
		"1", "MERCEARIA", "2", "BISCOITOS", "3", "RECHEADO",
		"9", "1234", "CLIENTE TESTE", "FANTASIA TESTE",
		"7", "SUPERMERCADO", "12345678000199", "SP", "EMPRESA 1",
		"555", "BISCOITO REC 140G", "7891000100103", "CX",
		"1", "24", "10",
		"5.50", "55.00", "12.30",
		"1", "VENDA PADRAO",
	}
	if len(linha) != len(colunasJC) {
		t.Fatalf("linha de teste com %d campos, colunasJC tem %d", len(linha), len(colunasJC))
	}

	var buf bytes.Buffer
	w := novoEscritorCSVJC(&buf)
	if err := w.Write(colunasJC); err != nil {
		t.Fatalf("cabeçalho: %v", err)
	}
	if err := w.Write(linha); err != nil {
		t.Fatalf("linha: %v", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// ── Daqui pra baixo, EXATAMENTE como o importador lê ────────────────────
	r := csv.NewReader(bytes.NewReader(buf.Bytes()))
	r.Comma = ';'
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1

	header, err := r.Read()
	if err != nil {
		t.Fatalf("importador não leu o cabeçalho: %v", err)
	}
	if len(header) != len(colunasJC) {
		t.Fatalf("importador viu %d colunas, deveria ver %d — delimitador divergente?",
			len(header), len(colunasJC))
	}

	// Mesma normalização do importador (minúsculo, sem espaço, sem underscore).
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, "_", "")
		return s
	}
	colMap := map[string]int{}
	for i, h := range header {
		colMap[norm(h)] = i
	}

	dados, err := r.Read()
	if err != nil {
		t.Fatalf("importador não leu a linha de dados: %v", err)
	}

	// As colunas que o importador procura, com o nome que ele usa na busca.
	esperado := map[string]string{
		"data":            "2026-07-28",
		"estado":          "FATURADO",
		"codepto":         "1",
		"qtrcasupervisor": "5",
		"qtclirca":        "250",
		"codusur":         "100",
		"codfornec":       "F01",
		"cnpj":            "12345678000199",
		"pvenda":          "5.50",
		"pvendatotal":     "55.00",
		"plucro":          "12.30",
		"condvenda":       "1",
		"desccondvenda":   "VENDA PADRAO",
	}
	for chave, quero := range esperado {
		idx, ok := colMap[chave]
		if !ok {
			t.Errorf("importador NÃO encontraria a coluna %q", chave)
			continue
		}
		if got := dados[idx]; got != quero {
			t.Errorf("coluna %q = %q, queria %q", chave, got, quero)
		}
	}
}

// TestDelimitadorBateComOImportador — trava o valor. Se alguém mudar o
// delimitador de um lado sem o outro, quebra aqui em vez de em produção.
func TestDelimitadorBateComOImportador(t *testing.T) {
	if delimitadorCSVJC != ';' {
		t.Errorf("delimitador = %q, mas o importador lê com ';' (farol_v2_import.go:297)",
			delimitadorCSVJC)
	}
}

// TestCSVEscapaCampoComDelimitador — nome de cliente com `;` no meio (acontece)
// não pode partir a linha em duas colunas.
func TestCSVEscapaCampoComDelimitador(t *testing.T) {
	var buf bytes.Buffer
	w := novoEscritorCSVJC(&buf)
	w.Write([]string{"A", "B", "C"})
	w.Write([]string{"x", "COMERCIO; INDUSTRIA LTDA", "z"})
	w.Flush()

	r := csv.NewReader(bytes.NewReader(buf.Bytes()))
	r.Comma = ';'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.Read() // cabeçalho
	linha, err := r.Read()
	if err != nil {
		t.Fatalf("leitura: %v", err)
	}
	if len(linha) != 3 {
		t.Fatalf("campo com ';' partiu a linha em %d colunas: %v", len(linha), linha)
	}
	if linha[1] != "COMERCIO; INDUSTRIA LTDA" {
		t.Errorf("campo = %q, queria %q", linha[1], "COMERCIO; INDUSTRIA LTDA")
	}
}

// TestDestinatariosSeparadores — o `;` do Outlook fez o SMTP recusar tudo com
// "501 Bad recipient address syntax" em 29/07, porque a lista virou UM endereço.
// Separador é detalhe de digitação; o código tem que aceitar os plausíveis.
func TestDestinatariosSeparadores(t *testing.T) {
	casos := map[string]int{
		"a@x.com,b@y.com":    2,
		"a@x.com;b@y.com":    2, // Outlook — o que quebrou em produção
		"a@x.com; b@y.com":   2,
		"a@x.com , b@y.com ": 2,
		"a@x.com b@y.com":    2,
		"a@x.com":            1,
		"a@x.com;":           1,
		" ; ,a@x.com, ":      1,
	}
	for entrada, querN := range casos {
		t.Run(entrada, func(t *testing.T) {
			t.Setenv("JC_EXTRACAO_EMAILS", entrada)
			got := destinatariosJC()
			if len(got) != querN {
				t.Errorf("destinatariosJC(%q) = %v (%d), queria %d", entrada, got, len(got), querN)
			}
			for _, e := range got {
				if strings.ContainsAny(e, ",; ") {
					t.Errorf("endereço %q ainda contém separador — o SMTP recusaria", e)
				}
			}
		})
	}
}

// TestDestinatariosPadrao — sem env configurado, os dois destinos combinados.
func TestDestinatariosPadrao(t *testing.T) {
	t.Setenv("JC_EXTRACAO_EMAILS", "")
	got := destinatariosJC()
	if len(got) != 2 {
		t.Fatalf("esperava 2 destinatários padrão, veio %v", got)
	}
}

// TestDuracaoAntesDoFim — chamar Duracao() com o Fim ainda zerado subtraía de
// um time.Time zero e estourava o int64: o log da 1ª carga bem-sucedida saiu
// "em -2562047h47m16.854775808s". Duração negativa num relatório operacional
// destrói a confiança no resto dos números.
func TestDuracaoAntesDoFim(t *testing.T) {
	res := &ResultadoExtracao{Inicio: time.Now().Add(-90 * time.Second)}
	d := res.Duracao()
	if d < 0 {
		t.Errorf("Duracao() com Fim zerado = %v — negativa", d)
	}
	if d < 80*time.Second || d > 120*time.Second {
		t.Errorf("Duracao() = %v, esperava ~90s", d)
	}

	res.Fim = res.Inicio.Add(2 * time.Minute)
	if got := res.Duracao(); got != 2*time.Minute {
		t.Errorf("com Fim preenchido = %v, queria 2m", got)
	}
}

func TestMilhar(t *testing.T) {
	casos := map[int]string{0: "0", 7: "7", 999: "999", 1000: "1.000",
		88067: "88.067", 1234567: "1.234.567"}
	for in, want := range casos {
		if got := milhar(in); got != want {
			t.Errorf("milhar(%d) = %q, want %q", in, got, want)
		}
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }
