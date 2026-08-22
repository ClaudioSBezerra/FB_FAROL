package services

import "testing"

// A ordenação é por REAIS, não por percentual. É a regra que sustenta o
// conceito inteiro: o RCA grande a 90% do ritmo deixa mais dinheiro na mesa que
// o pequeno a 50%, e é nele que o supervisor mexe primeiro. Ordenar por
// atingimento colocaria o pequeno no topo e o gestor perderia o dia com ele.
func TestOrdenaPorReaisNaoPorPercentual(t *testing.T) {
	rs := []RcaMesa{
		{CodRca: "pequeno", DinheiroMesa: 1_000, Atingimento: 50},
		{CodRca: "grande", DinheiroMesa: 80_000, Atingimento: 90},
		{CodRca: "medio", DinheiroMesa: 12_000, Atingimento: 70},
	}
	ordenarPorMesa(rs)
	if rs[0].CodRca != "grande" || rs[1].CodRca != "medio" || rs[2].CodRca != "pequeno" {
		t.Errorf("ordem por reais quebrou: %s, %s, %s", rs[0].CodRca, rs[1].CodRca, rs[2].CodRca)
	}
}

// O motivo compara o RCA com a MÉDIA DA PRÓPRIA EQUIPE, não com a empresa.
// Rota de capital e rota de interior têm patamares diferentes; comparar com a
// média geral acusaria o interior inteiro todo mês.
func TestMotivoComparaComAPropriaEquipe(t *testing.T) {
	rs := []RcaMesa{
		// Equipe A: positivação alta. Quem cair aqui é POSITIVACAO.
		{CodSupervisor: "A", Faixa: "R", Positivados: 20, BaseCli: 100, Mix: 5},
		{CodSupervisor: "A", Faixa: "G", Positivados: 80, BaseCli: 100, Mix: 5},
		{CodSupervisor: "A", Faixa: "G", Positivados: 80, BaseCli: 100, Mix: 5},
		// Equipe B: mesma positivação do primeiro da equipe A (20%), mas aqui
		// isso é a média — o que cai é o mix.
		{CodSupervisor: "B", Faixa: "R", Positivados: 20, BaseCli: 100, Mix: 1},
		{CodSupervisor: "B", Faixa: "G", Positivados: 20, BaseCli: 100, Mix: 8},
		{CodSupervisor: "B", Faixa: "G", Positivados: 20, BaseCli: 100, Mix: 8},
	}
	atribuirMotivo(rs)

	if rs[0].Motivo != "POSITIVACAO" {
		t.Errorf("equipe A: esperava POSITIVACAO, veio %q", rs[0].Motivo)
	}
	if rs[3].Motivo != "MIX" {
		t.Errorf("equipe B: mesma positivação da equipe A, mas ali é a média — esperava MIX, veio %q", rs[3].Motivo)
	}
}

// Quem está no ritmo não recebe diagnóstico: apontar "causa" para quem vai bem
// é ruído que ensina o gestor a ignorar a coluna.
func TestVerdeNaoRecebeMotivo(t *testing.T) {
	rs := []RcaMesa{
		{CodSupervisor: "A", Faixa: "G", Positivados: 1, BaseCli: 100, Mix: 1},
		{CodSupervisor: "A", Faixa: "G", Positivados: 90, BaseCli: 100, Mix: 9},
	}
	atribuirMotivo(rs)
	for i, r := range rs {
		if r.Motivo != "" {
			t.Errorf("linha %d verde recebeu motivo %q", i, r.Motivo)
		}
	}
}

// O escopo do e-mail tem que ser o MESMO do painel. Se divergirem, alguém
// recebe por e-mail o que não vê na tela — e isso aparece como suspeita de
// vazamento, não como bug de filtro.
func TestEscopoEspelhaOPainel(t *testing.T) {
	rs := []RcaMesa{
		{CodRca: "1", CodGerente: "347", CodSupervisor: "10"},
		{CodRca: "2", CodGerente: "347", CodSupervisor: "11"},
		{CodRca: "3", CodGerente: "350", CodSupervisor: "12"},
	}

	if got := FiltrarEscopo(rs, "gerente_geral", ""); len(got) != 3 {
		t.Errorf("gerente_geral deveria ver tudo, viu %d", len(got))
	}
	if got := FiltrarEscopo(rs, "ggv", "347"); len(got) != 2 {
		t.Errorf("ggv 347 deveria ver 2 RCAs, viu %d", len(got))
	}
	if got := FiltrarEscopo(rs, "supervisor", "12"); len(got) != 1 || got[0].CodRca != "3" {
		t.Errorf("supervisor 12 deveria ver só o RCA 3, viu %+v", got)
	}
	// Fail-closed: persona com escopo e sem código não vê nada — mesma regra
	// do escopoDoUsuario, que nega em vez de liberar.
	if got := FiltrarEscopo(rs, "ggv", ""); len(got) != 0 {
		t.Errorf("ggv sem cod_referencia deveria ver 0, viu %d", len(got))
	}
}

// A régua precisa se declarar. Chamar de "meta" o que é comparação com o ano
// anterior seria a mentira mais fácil de contar aqui — e a mais cara, porque o
// gestor cobraria a equipe por um alvo que ninguém definiu.
func TestBaselineSeDeclara(t *testing.T) {
	if BaselineMeta.Rotulo() == BaselineAnoAnterior.Rotulo() {
		t.Error("as duas réguas precisam ter rótulos distintos no e-mail")
	}
	if BaselineAnoAnterior.Rotulo() != "mesmo mês do ano anterior" {
		t.Errorf("rótulo do ano anterior não pode sugerir meta: %q", BaselineAnoAnterior.Rotulo())
	}
}

func TestPositivacaoNaoDivideporZero(t *testing.T) {
	r := RcaMesa{Positivados: 10, BaseCli: 0}
	if got := r.PositivacaoPct(); got != 0 {
		t.Errorf("carteira desconhecida deveria dar 0, deu %v", got)
	}
}
