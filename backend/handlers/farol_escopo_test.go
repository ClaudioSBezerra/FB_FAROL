package handlers

import "testing"

// A regra que este arquivo protege: GGV/supervisor/RCA veem apenas o próprio
// recorte; as demais personas veem a empresa inteira. Um erro aqui não aparece
// na tela como defeito — aparece como o gestor de uma equipe lendo os números
// de outra.

func TestPersonaEscopoCol(t *testing.T) {
	casos := []struct {
		persona string
		want    string
		porque  string
	}{
		{"ggv", "cod_gerente", "GGV é o nível gerente na hierarquia da importação"},
		{"supervisor", "cod_supervisor", ""},
		{"rca", "cod_rca", ""},
		{"ceo", "", "diretoria precisa do consolidado"},
		{"diretor", "", ""},
		{"gerente_geral", "", ""},
		{"ti", "", "opera o sistema"},
		{"admin", "", ""},
		{"analista_negocios", "", "analisa o todo"},
		{"", "", "persona não preenchida não é restrição"},
	}
	for _, c := range casos {
		if got := personaEscopoCol(c.persona); got != c.want {
			t.Errorf("persona %q → %q, esperado %q (%s)", c.persona, got, c.want, c.porque)
		}
	}
}

// Persona restrita SEM cod_referencia tem de NEGAR, nunca liberar. Se o erro
// caísse para o outro lado, um cadastro incompleto viraria acesso total.
func TestEscopoSemCodigoNega(t *testing.T) {
	e := escopoDoUsuario(nil, &FarolContext{
		UserID: "u1", TipoPersona: "ggv", CodReferencia: "",
	}, "")
	if !e.Negar {
		t.Fatal("GGV sem cod_referencia deveria ser NEGADO, veio liberado")
	}
	filters := multiFilters{}
	if aplicarEscopo(filters, e) {
		t.Error("aplicarEscopo deveria devolver false (negar) para escopo sem código")
	}
}

// Contexto ausente = nega. Vale para o caso de um handler novo esquecer de
// checar spCtx antes de chamar.
func TestEscopoSemContextoNega(t *testing.T) {
	if e := escopoDoUsuario(nil, nil, ""); !e.Negar {
		t.Error("contexto nil deveria negar")
	}
}

// O escopo SOBRESCREVE o filtro do request. Sem isto, bastaria editar a URL
// para ver a equipe de outro GGV.
func TestEscopoSobrescreveFiltroDoRequest(t *testing.T) {
	e := escopoDoUsuario(nil, &FarolContext{
		UserID: "u1", TipoPersona: "ggv", CodReferencia: "347",
	}, "")

	// Simula o que chegaria de uma URL adulterada: ?cod_gerente=350
	filters := multiFilters{"cod_gerente": {"350"}}
	if !aplicarEscopo(filters, e) {
		t.Fatal("escopo válido não deveria negar")
	}
	got := filters["cod_gerente"]
	if len(got) != 1 || got[0] != "347" {
		t.Errorf("cod_gerente = %v, esperado [347] — o filtro do request venceu o escopo", got)
	}
}

// Persona sem restrição não deve ganhar filtro nenhum: o CEO continua vendo
// tudo, inclusive podendo filtrar por gerente à vontade.
func TestPersonaIrrestritaNaoGanhaFiltro(t *testing.T) {
	e := escopoDoUsuario(nil, &FarolContext{
		UserID: "u1", TipoPersona: "ceo", CodReferencia: "",
	}, "")
	if e.restrito() {
		t.Fatal("CEO não deveria ter escopo restrito")
	}
	filters := multiFilters{"cod_gerente": {"350"}}
	aplicarEscopo(filters, e)
	if len(filters["cod_gerente"]) != 1 || filters["cod_gerente"][0] != "350" {
		t.Errorf("filtro do CEO foi alterado: %v", filters["cod_gerente"])
	}
}

// admin_fbtax é cross-tenant por definição (suporte): não recebe recorte.
func TestAdminFbtaxNaoTemEscopo(t *testing.T) {
	e := escopoDoUsuario(nil, &FarolContext{
		UserID: "u1", SpRole: "admin_fbtax", TipoPersona: "ggv", CodReferencia: "347",
	}, "")
	if e.restrito() {
		t.Error("admin_fbtax não deveria ser restrito nem com persona ggv")
	}
}

// O seletor de cobertura só aceita código que o usuário realmente cobre.
// Pedir um código arbitrário cai de volta no próprio — nunca no pedido.
func TestEscopoPedidoForaDoPermitidoCaiNoProprio(t *testing.T) {
	// db nil → coberturasVigentes devolve nil, então nada é coberto.
	e := escopoDoUsuario(nil, &FarolContext{
		UserID: "u1", TipoPersona: "ggv", CodReferencia: "347",
	}, "350") // tenta ver a equipe do 350 sem ter cobertura

	if len(e.Vals) != 1 || e.Vals[0] != "347" {
		t.Errorf("Vals = %v, esperado [347] — escopo pedido indevidamente foi aceito", e.Vals)
	}
}

// escopoDimCond restringe dropdowns de PESSOA e deixa os demais intactos —
// senão o GGV escolheria uma indústria e não veria nada, ou pior, veria a
// lista de todos os RCAs da empresa.
func TestEscopoDimCondSoRestringeDimsDePessoa(t *testing.T) {
	e := escopoRecorte{Col: "cod_gerente", Vals: []string{"347"}}

	for _, dim := range []string{"gerente", "supervisor", "rca"} {
		args := []any{"emp"}
		if escopoDimCond(e, dim, "fat", "d.key", &args) == "" {
			t.Errorf("dim %q deveria ser restringida", dim)
		}
	}
	for _, dim := range []string{"fornec", "cli", "uf", "empresa", "tipo_venda"} {
		args := []any{"emp"}
		if cond := escopoDimCond(e, dim, "fat", "d.key", &args); cond != "" {
			t.Errorf("dim %q não é de pessoa, não deveria ser restringida — veio %q", dim, cond)
		}
	}
}

func TestEscopoDimCondIrrestritoNaoFiltra(t *testing.T) {
	args := []any{"emp"}
	if cond := escopoDimCond(escopoRecorte{}, "rca", "fat", "d.key", &args); cond != "" {
		t.Errorf("persona irrestrita não deveria filtrar dropdown — veio %q", cond)
	}
}
