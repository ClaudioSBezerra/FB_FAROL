package handlers

// farol_segmento_foco_public.go — Segmento Foco (Reckitt) no mobile (link do ION VENDAS, sem login).
//
// Pedido do Heverton (24/11/2026): mesma tela/desenho do mobile da Unilever, liberada
// só pra quem está nos dados da Reckitt. Decisões do Claudio (24/11): só contagem de
// clientes (sem objetivo/rateio por equipe/RCA), escopos sup, rca e ggv, todas as equipes
// da planilha de hierarquia, equipe fora da planilha (200, 560, 580, ...) sem acesso.
//
// O escopo vem SEMPRE da URL (scope + cod): a query string só pode estreitar dentro dele,
// nunca ampliar — qualquer filtro que contradiga o escopo é recusado (403).
//
// Rotas (todas GET, ?cnpj=&scope=sup|rca|ggv&cod=):
//   /api/farol/public/segmento-foco/vinculos            → programas da Reckitt que este link enxerga
//   /api/farol/public/segmento-foco/painel              → contagem por grupo (&vinculo_id=&mes=&fluxo=)
//   /api/farol/public/segmento-foco/detalhe             → drill RCA/cliente/produto (&periodo=&cod_crv=&cod_rca=&cod_cli=&nivel=)

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

// segGGVAlias — código de GGV que o ION manda no link → código de GGV da planilha de
// hierarquia. Em TO a venda carrega o gerente 345, e a planilha chama o mesmo GGV de 2.
var segGGVAlias = map[string]string{"345": "2"}

// segPublicoEscopo valida (scope, cod) contra a hierarquia da Reckitt e devolve o escopo
// do motor. ok=false = o código não está nos dados da Reckitt (ou a equipe está fora do painel).
func segPublicoEscopo(db *sql.DB, empresaID string, vinculoID int, scope, cod string) (segEscopo, bool) {
	cod = strings.TrimSpace(cod)
	if cod == "" {
		return segEscopo{}, false
	}
	hier := segLerHierarquia(db, empresaID, vinculoID)
	if len(hier) == 0 {
		return segEscopo{}, false
	}
	ocultas := segEquipesOcultas(db, empresaID, vinculoID)
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "sup":
		if ocultas[cod] {
			return segEscopo{}, false
		}
		for _, h := range hier {
			if h.CodCRV == cod {
				return segEscopo{CodCRV: cod}, true
			}
		}
	case "rca":
		if h, ok := hier[cod]; ok && !ocultas[h.CodCRV] {
			return segEscopo{CodRCA: cod}, true
		}
	case "ggv":
		ggv := cod
		if a, ok := segGGVAlias[cod]; ok {
			ggv = a
		}
		for _, h := range hier {
			if h.CodGGV == ggv && !ocultas[h.CodCRV] {
				return segEscopo{CodGGV: ggv}, true
			}
		}
	}
	return segEscopo{}, false
}

// Obs.: a restrição por indústria do link de GGV (acesso_publico_ggv, mig 256 — GGV 3 e 4 só Unilever)
// NÃO vale para o Segmento Foco: o Claudio liberou a Reckitt no mobile para todos os GGVs (24/11/2026).
// Mexer naquela lista abriria também o painel de vendas público dos dois GGVs, o que não foi pedido.

func segPublicoErro(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// segPublicoContexto resolve empresa + escopo; escreve o erro e devolve ok=false se algo não bate.
func segPublicoContexto(db *sql.DB, w http.ResponseWriter, r *http.Request, vinculoID int) (empresaID string, sc segEscopo, ok bool) {
	q := r.URL.Query()
	empresaID = resolveEmpresaCNPJ(db, q.Get("cnpj"))
	if empresaID == "" {
		segPublicoErro(w, http.StatusNotFound, "empresa não encontrada para este CNPJ")
		return
	}
	var industriaID int
	var formula string
	if db.QueryRow(`SELECT mv.industria_id, tm.formula_codigo FROM farol.metas_vinculos mv
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE mv.id = $1 AND mv.empresa_id = $2 AND mv.ativo = TRUE`, vinculoID, empresaID).Scan(&industriaID, &formula) != nil || formula != "positivacao_grupos" {
		segPublicoErro(w, http.StatusNotFound, "programa não encontrado")
		return
	}
	_ = industriaID
	sc, found := segPublicoEscopo(db, empresaID, vinculoID, q.Get("scope"), q.Get("cod"))
	if !found {
		segPublicoErro(w, http.StatusForbidden, "sem acesso a este programa")
		return
	}
	return empresaID, sc, true
}

// SegmentoFocoPublicVinculosHandler — programas Reckitt que o link enxerga (lista vazia = sem acesso).
func SegmentoFocoPublicVinculosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		empresaID := resolveEmpresaCNPJ(db, q.Get("cnpj"))
		out := []map[string]any{}
		if empresaID == "" {
			json.NewEncoder(w).Encode(out)
			return
		}
		rows, err := db.Query(`SELECT mv.id, mv.industria_id, i.nome FROM farol.metas_vinculos mv
			JOIN farol.industrias i ON i.id = mv.industria_id
			JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
			WHERE mv.empresa_id = $1 AND mv.ativo = TRUE AND tm.formula_codigo = 'positivacao_grupos' ORDER BY i.nome`, empresaID)
		if err != nil {
			segPublicoErro(w, http.StatusInternalServerError, "database error")
			return
		}
		type v struct {
			id, ind int
			nome    string
		}
		var vs []v
		for rows.Next() {
			var x v
			if rows.Scan(&x.id, &x.ind, &x.nome) == nil {
				vs = append(vs, x)
			}
		}
		rows.Close()
		for _, x := range vs {
			if _, ok := segPublicoEscopo(db, empresaID, x.id, q.Get("scope"), q.Get("cod")); ok {
				out = append(out, map[string]any{"id": x.id, "industria_id": x.ind, "industria_nome": x.nome})
			}
		}
		json.NewEncoder(w).Encode(out)
	}
}

// SegmentoFocoPublicPainelHandler — contagem de clientes por grupo no escopo do link (sem objetivo).
func SegmentoFocoPublicPainelHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		vinculoID, mes, fluxo, err := segParamsComuns(r)
		if err != nil {
			segPublicoErro(w, http.StatusBadRequest, err.Error())
			return
		}
		empresaID, sc, ok := segPublicoContexto(db, w, r, vinculoID)
		if !ok {
			return
		}
		resp, err := segPainel(db, empresaID, vinculoID, mes, fluxo, sc)
		if err != nil {
			segPublicoErro(w, http.StatusBadRequest, err.Error())
			return
		}
		// só contagem: o objetivo é por UF × grupo, não por equipe/RCA/GGV.
		for i := range resp.UFs {
			for j := range resp.UFs[i].Grupos {
				g := &resp.UFs[i].Grupos[j]
				g.ObjetivoTri = nil
				for k := range g.Meses {
					g.Meses[k].Objetivo = nil
				}
			}
		}
		json.NewEncoder(w).Encode(resp)
	}
}

// SegmentoFocoPublicDetalheHandler — drill dentro do escopo do link.
func SegmentoFocoPublicDetalheHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		vinculoID, mes, fluxo, err := segParamsComuns(r)
		if err != nil {
			segPublicoErro(w, http.StatusBadRequest, err.Error())
			return
		}
		empresaID, sc, ok := segPublicoContexto(db, w, r, vinculoID)
		if !ok {
			return
		}
		q := r.URL.Query()
		f := segEscopo{UF: q.Get("uf"), CodGGV: q.Get("cod_ggv"), CodCRV: q.Get("cod_crv"), CodRCA: q.Get("cod_rca"), CodCli: q.Get("cod_cli"), Nivel: q.Get("nivel")}
		if !segEscopoRespeita(&f, sc) {
			segPublicoErro(w, http.StatusForbidden, "fora do escopo deste link")
			return
		}
		resp, err := segDetalhe(db, empresaID, vinculoID, mes, q.Get("periodo"), fluxo, f)
		if err != nil {
			segPublicoErro(w, http.StatusBadRequest, err.Error())
			return
		}
		json.NewEncoder(w).Encode(resp)
	}
}

// segEscopoRespeita impõe o escopo do link sobre o filtro pedido: onde o link tem um valor, o
// pedido só pode repeti-lo (ou deixar vazio); contradição = false. O pedido nunca amplia.
func segEscopoRespeita(f *segEscopo, sc segEscopo) bool {
	for _, p := range []struct {
		pedido *string
		escopo string
	}{
		{&f.CodGGV, sc.CodGGV}, {&f.CodCRV, sc.CodCRV}, {&f.CodRCA, sc.CodRCA},
	} {
		if p.escopo == "" {
			continue
		}
		if *p.pedido != "" && *p.pedido != p.escopo {
			return false
		}
		*p.pedido = p.escopo
	}
	return true
}
