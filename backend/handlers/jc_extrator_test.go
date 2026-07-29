package handlers

import (
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
