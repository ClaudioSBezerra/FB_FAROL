package handlers

import "testing"

func TestMontarLinhasPPA(t *testing.T) {
	ppas := []ppaValido{
		{CodProd: "1", PPA: "AMACIANTE 900ML"},
		{CodProd: "2", PPA: "AMACIANTE 900ML"}, // 2ª variante do mesmo PPA
		{CodProd: "3", PPA: "SABAO 1KG"},
		{CodProd: "4", PPA: "CALDO 3L"},
		{CodProd: "5", PPA: "TEMPERO"},
	}
	linhas := map[string]vendaProdutoAgregada{
		"1": {Qtd: 1, Embalagem: "UN"},                  // abaixo do mínimo...
		"2": {Qtd: 5, Embalagem: "UN"},                  // ...mas a outra variante vale: PPA vendeu
		"3": {Qtd: 2, Embalagem: "UN"},                  // só abaixo do mínimo
		"4": {Qtd: 1, Embalagem: "CX/12", QtUnitCx: 12}, // caixa: sem mínimo
		"9": {Qtd: 50, Embalagem: "UN"},                 // fora do catálogo: ignorado
		"5": {Qtd: -3, Embalagem: "UN"},                 // líquido negativo (devolução)
	}
	out := montarLinhasPPA(ppas, linhas)
	if len(out) != 4 {
		t.Fatalf("esperava 4 PPAs, veio %d: %+v", len(out), out)
	}
	por := map[string]PainelPPALinha{}
	for _, l := range out {
		por[l.PPA] = l
	}
	if l := por["AMACIANTE 900ML"]; !l.Vendeu || l.AbaixoMinimo || l.Qtd != 6 || len(l.CodProds) != 2 {
		t.Errorf("AMACIANTE: %+v", l)
	}
	if l := por["SABAO 1KG"]; l.Vendeu || !l.AbaixoMinimo || l.Qtd != 2 {
		t.Errorf("SABAO deveria estar abaixo do mínimo: %+v", l)
	}
	if l := por["CALDO 3L"]; !l.Vendeu || l.AbaixoMinimo {
		t.Errorf("CALDO em caixa deveria valer com 1: %+v", l)
	}
	if l := por["TEMPERO"]; l.Vendeu || l.AbaixoMinimo || l.Qtd != 0 {
		t.Errorf("TEMPERO com líquido negativo não comprou: %+v", l)
	}
	// o que falta vender vem primeiro, depois ordem alfabética
	if out[0].Vendeu || out[1].Vendeu || !out[2].Vendeu || !out[3].Vendeu {
		t.Errorf("ordem errada (não vendidos primeiro): %+v", out)
	}
	if out[0].PPA != "SABAO 1KG" || out[1].PPA != "TEMPERO" || out[2].PPA != "AMACIANTE 900ML" {
		t.Errorf("ordem alfabética dentro do grupo: %+v", out)
	}
}
