package handlers

// farol_metas_ppas_csv_test.go — cobre a Story 7.3 do Épico 7 addendum (ver
// epics.md): import de PPAs, incluindo o caso de 2 cod_prod na mesma família.

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

const ppasHeader = "cod_prod;ppa_nome;ean;embalagem;regiao;ae;bu"

func ppasImportReq(empresaID, userID, vinculoID, vigenciaID string, csvContent string) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "ppas.csv")
	fw.Write([]byte(csvContent))
	mw.Close()

	url := fmt.Sprintf("/api/farol/metas-ppas-importar-csv?vinculo_id=%s&vigencia_id=%s", vinculoID, vigenciaID)
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: userID, SpRole: "gestor_geral", EmpresaID: empresaID, AllFiliais: true,
	})
	return r.WithContext(ctx)
}

func TestMetasPPAs_ImportarLoteValido_DuasVariantesMesmaFamilia(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TPPA Valido")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-01-01", "2026-01-31")

	csvContent := ppasHeader + "\n" +
		"456708;MAIZENA CREMOGEMA 180GR;7891150068278;CX/0024/UN;Centro Norte;JC;FR\n" +
		"456709;MAIZENA CREMOGEMA 180GR;7891150068261;CX/0048/UN;Centro Norte;JC;FR\n" +
		"456710;CALDO KNORR 3L;7891150012363;UN;Centro Norte;JC;FR\n"
	w := httptest.NewRecorder()
	MetasPPAsImportarCSVHandler(db)(w, ppasImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusOK {
		t.Fatalf("import válido → status %d, body=%s", w.Code, w.Body.String())
	}
	var n, nFamilia int
	db.QueryRow(`SELECT count(*) FROM farol.metas_ppas WHERE vigencia_id = $1`, vigenciaID).Scan(&n)
	if n != 3 {
		t.Errorf("esperava 3 cod_prod importados, veio %d", n)
	}
	db.QueryRow(`SELECT count(*) FROM farol.metas_ppas WHERE vigencia_id = $1 AND ppa_nome = 'MAIZENA CREMOGEMA 180GR'`, vigenciaID).Scan(&nFamilia)
	if nFamilia != 2 {
		t.Errorf("esperava 2 cod_prod na família MAIZENA CREMOGEMA 180GR, veio %d", nFamilia)
	}
}

func TestMetasPPAs_SemPPANome_400NadaImportado(t *testing.T) {
	db, empresaID := biTestDB(t)
	userID := tipoMetricaTestUserID(t, db)
	vinculoID, cleanup := criarVinculoFixture(t, db, empresaID, "TPPA SemNome")
	t.Cleanup(cleanup)
	vigenciaID := criarVigenciaFixture(t, db, empresaID, vinculoID, "2026-02-01", "2026-02-28")

	csvContent := ppasHeader + "\n" +
		"456708;MAIZENA CREMOGEMA 180GR;7891150068278;CX/0024/UN;Centro Norte;JC;FR\n" +
		"456709;;7891150068261;CX/0048/UN;Centro Norte;JC;FR\n" // sem ppa_nome
	w := httptest.NewRecorder()
	MetasPPAsImportarCSVHandler(db)(w, ppasImportReq(empresaID, userID, fmt.Sprint(vinculoID), fmt.Sprint(vigenciaID), csvContent))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("sem ppa_nome → status %d, want 400. body=%s", w.Code, w.Body.String())
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM farol.metas_ppas WHERE vigencia_id = $1`, vigenciaID).Scan(&n)
	if n != 0 {
		t.Errorf("esperava 0 PPAs importados (lote atômico), veio %d", n)
	}
}
