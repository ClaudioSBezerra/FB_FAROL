package handlers

import "testing"

func TestAcesso_PaineisEIndustrias(t *testing.T) {
	admin := &FarolContext{SpRole: "admin_fbtax"}
	if !admin.PodeVerPainel("reckitt") || !admin.PodeVerIndustria(99) || len(admin.FormulasBloqueadas()) != 0 {
		t.Errorf("admin_fbtax deve ver tudo: bloqueadas=%v", admin.FormulasBloqueadas())
	}
	// Gilson/Olacir: só Unilever (e só as indústrias liberadas).
	ggv := &FarolContext{SpRole: "gestor_filial", Modulos: []string{"unilever"}, IndustriasPermitidas: []int{46, 47}}
	if ggv.PodeVerPainel("vendas") || ggv.PodeVerPainel("reckitt") || !ggv.PodeVerPainel("unilever") {
		t.Errorf("GGV só com unilever: vendas=%v reckitt=%v", ggv.PodeVerPainel("vendas"), ggv.PodeVerPainel("reckitt"))
	}
	if !ggv.PodeVerIndustria(46) || ggv.PodeVerIndustria(43) {
		t.Errorf("indústrias do GGV: 46=%v 43=%v", ggv.PodeVerIndustria(46), ggv.PodeVerIndustria(43))
	}
	bloqueadas := map[string]bool{}
	for _, f := range ggv.FormulasBloqueadas() {
		bloqueadas[f] = true
	}
	if !bloqueadas[formulaSegmentoFoco] || bloqueadas["cobertura_rede"] {
		t.Errorf("GGV: Segmento Foco bloqueado e Unilever liberado — bloqueadas=%v", bloqueadas)
	}
	// Padrão (Carlos): painéis padrão, todas as indústrias, sem Reckitt.
	carlos := &FarolContext{SpRole: "gestor_geral", Modulos: []string{"vendas", "unilever", "relatorios"}}
	if carlos.PodeVerPainel("reckitt") || !carlos.PodeVerIndustria(43) || carlos.industriasFiltro() != nil {
		t.Errorf("padrão: reckitt=%v ind43=%v filtro=%v", carlos.PodeVerPainel("reckitt"), carlos.PodeVerIndustria(43), carlos.industriasFiltro())
	}
	// Reckitt liberado explicitamente.
	rk := &FarolContext{SpRole: "gestor_filial", Modulos: []string{"reckitt"}}
	if !rk.PodeVerPainel("reckitt") || rk.PodeVerPainel("unilever") {
		t.Errorf("só reckitt: reckitt=%v unilever=%v", rk.PodeVerPainel("reckitt"), rk.PodeVerPainel("unilever"))
	}
}
