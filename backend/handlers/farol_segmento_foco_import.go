package handlers

// farol_segmento_foco_import.go — importação das 2 listas do Segmento Foco:
//   POST /api/farol/metas-grupos-produtos-importar-csv?vinculo_id=&vigencia_id=
//        CSV ';': grupo;cod_prod;produto;ean;embalagem   (obrigatórios: grupo, cod_prod)
//   POST /api/farol/metas-grupos-objetivos-importar-csv?vinculo_id=&vigencia_id=
//        CSV ';': uf;grupo;objetivo                      (todos obrigatórios)
// Reimportar SUBSTITUI a lista da vigência; vigência fechada bloqueia. Atômico
// (tudo ou nada), como Clientes Numérica/PPAs.

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type segLinhaErro struct {
	Linha int    `json:"linha"`
	Erro  string `json:"erro"`
}

// segLerCSV valida vinculo/vigência (aberta, do vínculo) e devolve as linhas
// do CSV como mapas coluna→valor, já normalizadas.
func segLerCSV(db *sql.DB, w http.ResponseWriter, r *http.Request, obrigatorias []string) (spCtx *FarolContext, vinculoID, vigenciaID int, linhas []map[string]string, linhaNum []int, ok bool) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	spCtx = GetSpContext(r)
	if spCtx == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if !hasSpRole(spCtx.SpRole, "gestor_geral") {
		http.Error(w, `{"error":"Forbidden: gestor_geral necessário"}`, http.StatusForbidden)
		return
	}
	var err1, err2 error
	vinculoID, err1 = strconv.Atoi(r.URL.Query().Get("vinculo_id"))
	vigenciaID, err2 = strconv.Atoi(r.URL.Query().Get("vigencia_id"))
	if err1 != nil || err2 != nil {
		http.Error(w, `{"error":"vinculo_id e vigencia_id (query string) são obrigatórios"}`, http.StatusBadRequest)
		return
	}
	var vinc int
	var status string
	if err := db.QueryRow(`SELECT vinculo_id, status FROM farol.metas_vigencias WHERE id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID).Scan(&vinc, &status); err != nil {
		http.Error(w, `{"error":"vigencia_id não encontrada"}`, http.StatusBadRequest)
		return
	}
	if vinc != vinculoID {
		http.Error(w, `{"error":"vigencia_id não pertence ao vinculo_id informado"}`, http.StatusBadRequest)
		return
	}
	if status == "fechada" {
		http.Error(w, `{"error":"vigência fechada — lista não pode ser reimportada"}`, http.StatusForbidden)
		return
	}
	raw, err := lerArquivoCSV(r)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}
	cr := csv.NewReader(strings.NewReader(string(raw)))
	cr.Comma = ';'
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		http.Error(w, `{"error":"falha ao ler cabeçalho CSV"}`, http.StatusBadRequest)
		return
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, c := range obrigatorias {
		if _, has := idx[c]; !has {
			http.Error(w, `{"error":"coluna obrigatória ausente no CSV: `+c+`"}`, http.StatusBadRequest)
			return
		}
	}
	n := 1
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		n++
		if err != nil {
			continue
		}
		m := map[string]string{}
		for col, i := range idx {
			if i < len(rec) {
				m[col] = strings.TrimSpace(rec[i])
			}
		}
		linhas = append(linhas, m)
		linhaNum = append(linhaNum, n)
	}
	ok = true
	return
}

func segResponderErros(w http.ResponseWriter, erros []segLinhaErro) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{"erros": erros, "linhas_com_erro": len(erros)})
}

func SegmentoFocoGruposImportarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx, vinculoID, vigenciaID, linhas, nums, ok := segLerCSV(db, w, r, []string{"grupo", "cod_prod"})
		if !ok {
			return
		}
		var erros []segLinhaErro
		vistos := map[string]bool{}
		type row struct{ grupo, cod, produto, ean, emb string }
		var rows []row
		for i, l := range linhas {
			grupo := strings.Join(strings.Fields(l["grupo"]), " ")
			cod := strings.TrimSuffix(l["cod_prod"], ".0")
			if cod == "" || grupo == "" {
				erros = append(erros, segLinhaErro{nums[i], "grupo e cod_prod são obrigatórios"})
				continue
			}
			if vistos[cod] {
				continue // o mesmo cod_prod pode repetir entre abas (matriz/PA); vale a 1ª
			}
			vistos[cod] = true
			rows = append(rows, row{strings.ToUpper(grupo), cod, l["produto"], l["ean"], l["embalagem"]})
		}
		if len(erros) > 0 {
			segResponderErros(w, erros)
			return
		}
		if len(rows) == 0 {
			http.Error(w, `{"error":"CSV vazio — nenhuma linha de dado encontrada"}`, http.StatusBadRequest)
			return
		}
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM farol.metas_grupos_produtos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID); err != nil {
			http.Error(w, `{"error":"database error ao limpar lista anterior"}`, http.StatusInternalServerError)
			return
		}
		for _, x := range rows {
			if _, err := tx.Exec(`INSERT INTO farol.metas_grupos_produtos (empresa_id, vinculo_id, vigencia_id, grupo, cod_prod, produto, ean, embalagem)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, spCtx.EmpresaID, vinculoID, vigenciaID, x.grupo, x.cod, x.produto, x.ean, x.emb); err != nil {
				http.Error(w, `{"error":"database error: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}
		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_grupos_produtos", strconv.Itoa(vigenciaID), "importar_csv",
			map[string]any{"vinculo_id": vinculoID, "vigencia_id": vigenciaID, "linhas": len(rows)}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		segInvalidarCache()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "produtos_importados": len(rows)})
	}
}

func SegmentoFocoObjetivosImportarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spCtx, vinculoID, vigenciaID, linhas, nums, ok := segLerCSV(db, w, r, []string{"uf", "grupo", "objetivo"})
		if !ok {
			return
		}
		var erros []segLinhaErro
		type row struct {
			uf, grupo string
			obj       float64
		}
		var rows []row
		vistos := map[string]int{}
		for i, l := range linhas {
			uf := strings.ToUpper(strings.TrimSpace(l["uf"]))
			grupo := strings.ToUpper(strings.Join(strings.Fields(l["grupo"]), " "))
			obj, err := strconv.ParseFloat(strings.ReplaceAll(l["objetivo"], ",", "."), 64)
			if uf == "" || grupo == "" || err != nil || obj < 0 {
				erros = append(erros, segLinhaErro{nums[i], fmt.Sprintf("uf, grupo e objetivo numérico são obrigatórios (uf=%q grupo=%q objetivo=%q)", uf, grupo, l["objetivo"])})
				continue
			}
			k := uf + "|" + grupo
			if prim, dup := vistos[k]; dup {
				erros = append(erros, segLinhaErro{nums[i], fmt.Sprintf("%s/%s duplicado (já na linha %d)", uf, grupo, prim)})
				continue
			}
			vistos[k] = nums[i]
			rows = append(rows, row{uf, grupo, obj})
		}
		if len(erros) > 0 {
			segResponderErros(w, erros)
			return
		}
		if len(rows) == 0 {
			http.Error(w, `{"error":"CSV vazio — nenhuma linha de dado encontrada"}`, http.StatusBadRequest)
			return
		}
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`DELETE FROM farol.metas_grupos_objetivos WHERE vigencia_id = $1 AND empresa_id = $2`, vigenciaID, spCtx.EmpresaID); err != nil {
			http.Error(w, `{"error":"database error ao limpar lista anterior"}`, http.StatusInternalServerError)
			return
		}
		for _, x := range rows {
			if _, err := tx.Exec(`INSERT INTO farol.metas_grupos_objetivos (empresa_id, vinculo_id, vigencia_id, uf, grupo, objetivo)
				VALUES ($1,$2,$3,$4,$5,$6)`, spCtx.EmpresaID, vinculoID, vigenciaID, x.uf, x.grupo, x.obj); err != nil {
				http.Error(w, `{"error":"database error: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
		}
		if err := writeAuditLogTx(tx, spCtx.EmpresaID, spCtx.UserID, "metas_grupos_objetivos", strconv.Itoa(vigenciaID), "importar_csv",
			map[string]any{"vinculo_id": vinculoID, "vigencia_id": vigenciaID, "linhas": len(rows)}); err != nil {
			http.Error(w, `{"error":"erro ao gravar auditoria"}`, http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, `{"error":"commit error"}`, http.StatusInternalServerError)
			return
		}
		segInvalidarCache()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "objetivos_importados": len(rows)})
	}
}
