package handlers

// farol_bi_api_test.go — testes de integração do endpoint consolidado do BI.
//
// Exigem banco real: rodam somente com DATABASE_URL apontando para uma base
// com dados consolidados. Sem a variável, o pacote inteiro é pulado (t.Skip),
// para não quebrar build/CI em máquina sem Postgres.
//
//	cd backend && set -a && source .env && set +a && go test ./handlers -run TestBI -v
//
// O teste que importa é o de PARIDADE: o painel BI não pode divergir do
// Executivo. Ele bate o payload do /bi contra o do /cards (V03/V01/V02),
// reproduzindo o que o front fazia antes (ordenar por faturado, top 8/12).

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func biTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL não definida — teste de integração pulado")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("abrir conexão: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("banco inacessível (%v) — teste pulado", err)
	}
	var empresaID string
	if err := db.QueryRow(`SELECT id::text FROM companies ORDER BY created_at LIMIT 1`).Scan(&empresaID); err != nil {
		t.Skipf("nenhuma empresa na base (%v) — teste pulado", err)
	}
	return db, empresaID
}

// biReq monta uma request já autenticada, injetando o FarolContext direto —
// evita depender de login/senha para exercitar o handler.
func biReq(url, empresaID string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, url, nil)
	ctx := context.WithValue(r.Context(), SpContextKey, &FarolContext{
		UserID: "teste", SpRole: "gestor_geral", EmpresaID: empresaID,
		AllFiliais: true, Modulos: []string{"vendas", "bi"},
	})
	return r.WithContext(ctx)
}

func biGet(t *testing.T, db *sql.DB, url, empresaID string) (biResponse, time.Duration) {
	t.Helper()
	w := httptest.NewRecorder()
	t0 := time.Now()
	FarolV2BIHandler(db)(w, biReq(url, empresaID))
	elapsed := time.Since(t0)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s → status %d, esperado 200", url, w.Code)
	}
	var out biResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("payload inválido: %v", err)
	}
	return out, elapsed
}

func cardsGet(t *testing.T, db *sql.DB, url, empresaID string) cardsResponse {
	t.Helper()
	w := httptest.NewRecorder()
	FarolV2CardsHandler(db)(w, biReq(url, empresaID))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s → status %d, esperado 200", url, w.Code)
	}
	var out cardsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("payload /cards inválido: %v", err)
	}
	return out
}

func quaseIgual(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// TestBIParidadeComCards — o critério de aceite central da mudança.
func TestBIParidadeComCards(t *testing.T) {
	db, empresaID := biTestDB(t)
	defer db.Close()

	for _, modo := range []string{"ytd", "mtd"} {
		t.Run(modo, func(t *testing.T) {
			invalidateBICache(empresaID) // sempre do zero
			bi, _ := biGet(t, db, "/api/v2/farol/bi?comp_mode="+modo, empresaID)

			// ── KPI dos gauges vem da V03 ────────────────────────────────────
			c3 := cardsGet(t, db, "/api/v2/farol/cards?view=V03&comp_mode="+modo, empresaID)
			if !quaseIgual(bi.KPI.TotalFaturado, c3.KPI.TotalFaturado) {
				t.Errorf("total_faturado divergente: bi=%.2f cards=%.2f", bi.KPI.TotalFaturado, c3.KPI.TotalFaturado)
			}
			if !quaseIgual(bi.KPI.TotalPct, c3.KPI.TotalPct) {
				t.Errorf("total_pct divergente: bi=%.4f cards=%.4f", bi.KPI.TotalPct, c3.KPI.TotalPct)
			}
			if !quaseIgual(bi.KPI.TotalAnt, c3.KPI.TotalAnt) {
				t.Errorf("total_ant divergente: bi=%.2f cards=%.2f", bi.KPI.TotalAnt, c3.KPI.TotalAnt)
			}
			if bi.KPI.TotalPositivados != c3.KPI.TotalPositivados {
				t.Errorf("positivados divergente: bi=%d cards=%d", bi.KPI.TotalPositivados, c3.KPI.TotalPositivados)
			}
			if bi.KPI.TotalBaseCli != c3.KPI.TotalBaseCli {
				t.Errorf("base_cli divergente: bi=%d cards=%d", bi.KPI.TotalBaseCli, c3.KPI.TotalBaseCli)
			}
			if !quaseIgual(bi.KPI.TotalPositPct, c3.KPI.TotalPositPct) {
				t.Errorf("positpct divergente: bi=%.4f cards=%.4f", bi.KPI.TotalPositPct, c3.KPI.TotalPositPct)
			}
			if !quaseIgual(bi.KPI.AvgMix, c3.KPI.AvgMix) {
				t.Errorf("avg_mix divergente: bi=%.4f cards=%.4f", bi.KPI.AvgMix, c3.KPI.AvgMix)
			}
			if bi.KPI.Verdes != c3.KPI.Verdes || bi.KPI.Vermelhos != c3.KPI.Vermelhos {
				t.Errorf("farol divergente: bi=%d/%d cards=%d/%d",
					bi.KPI.Verdes, bi.KPI.Vermelhos, c3.KPI.Verdes, c3.KPI.Vermelhos)
			}

			// ── Donut: top 8 da V01 + "Outros", com atingimento (pct/cor) ────
			c1 := cardsGet(t, db, "/api/v2/farol/cards?view=V01&comp_mode="+modo, empresaID)
			esperado, _ := biIndustriasEConcentracao(c1.Cards)
			if len(bi.Industrias) != len(esperado) {
				t.Fatalf("indústrias: bi=%d itens, esperado=%d", len(bi.Industrias), len(esperado))
			}
			for i := range esperado {
				if bi.Industrias[i].Label != esperado[i].Label ||
					!quaseIgual(bi.Industrias[i].Faturado, esperado[i].Faturado) ||
					bi.Industrias[i].Cor != esperado[i].Cor {
					t.Errorf("indústria[%d] divergente: bi=%q/%.2f/%s esperado=%q/%.2f/%s", i,
						bi.Industrias[i].Label, bi.Industrias[i].Faturado, bi.Industrias[i].Cor,
						esperado[i].Label, esperado[i].Faturado, esperado[i].Cor)
				}
			}

			// ── Pareto: top 5 / total, coerente com o payload ────────────────
			if bi.ConcentracaoTop5 < 0 || bi.ConcentracaoTop5 > 100.01 {
				t.Errorf("concentracao_top5 fora de [0,100]: %.2f", bi.ConcentracaoTop5)
			}

			// Sanidade: um painel com tudo zerado passaria em todas as
			// comparações acima. Exigimos que a base de teste tenha dado.
			if bi.KPI.TotalFaturado == 0 && len(bi.Industrias) == 0 {
				t.Fatal("payload vazio — comparação seria vacuamente verdadeira")
			}
			t.Logf("%s: faturado=%.2f pct=%.2f%% posit=%d/%d ind=%d ufs=%d top5=%.1f%%",
				modo, bi.KPI.TotalFaturado, bi.KPI.TotalPct,
				bi.KPI.TotalPositivados, bi.KPI.TotalBaseCli,
				len(bi.Industrias), len(bi.UFs), bi.ConcentracaoTop5)
		})
	}
}

// TestBICache — segunda chamada sai do cache; nocache=1 e invalidação furam.
func TestBICache(t *testing.T) {
	db, empresaID := biTestDB(t)
	defer db.Close()

	invalidateBICache(empresaID)
	_, frio := biGet(t, db, "/api/v2/farol/bi?comp_mode=ytd", empresaID)
	_, quente := biGet(t, db, "/api/v2/farol/bi?comp_mode=ytd", empresaID)

	if quente > frio/5 {
		t.Errorf("cache não parece ativo: frio=%v quente=%v (esperado quente << frio)", frio, quente)
	}
	t.Logf("frio=%v quente=%v (ganho %.0f×)", frio, quente, float64(frio)/math.Max(float64(quente), 1))

	// nocache=1 recomputa: tem de custar na ordem do frio, não do quente.
	_, forcado := biGet(t, db, "/api/v2/farol/bi?comp_mode=ytd&nocache=1", empresaID)
	if forcado < frio/10 {
		t.Errorf("nocache=1 não recomputou: %v (frio foi %v)", forcado, frio)
	}
	t.Logf("nocache=1 → %v", forcado)
}

// TestBIInvalidacaoDescartaCalculoVelho — o cálculo que começou ANTES de uma
// invalidação não pode gravar no cache depois dela.
func TestBIInvalidacaoDescartaCalculoVelho(t *testing.T) {
	db, empresaID := biTestDB(t)
	defer db.Close()

	invalidateBICache(empresaID)
	gen := biGeneration(empresaID)
	invalidateBICache(empresaID) // import terminou "durante o cálculo"

	if biStore(empresaID, biCacheKey(empresaID, "faturado", "ytd"), gen, biResponse{}) {
		t.Error("gravou no cache um resultado calculado antes da invalidação")
	}
	if !biStore(empresaID, biCacheKey(empresaID, "faturado", "ytd"), biGeneration(empresaID), biResponse{}) {
		t.Error("recusou gravação com geração corrente")
	}
	invalidateBICache(empresaID)
}

// TestBICarimboPrefereConsolidacao — o "dados de" tem de vir do fim da
// CONSOLIDAÇÃO, não do fim do upload. Usa empresa fictícia para não mexer na
// linha da empresa real (a tabela não tem FK para companies).
func TestBICarimboPrefereConsolidacao(t *testing.T) {
	db, _ := biTestDB(t)
	defer db.Close()

	var existe bool
	_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables
		WHERE table_schema='farol' AND table_name='consolidacao_log')`).Scan(&existe)
	if !existe {
		t.Skip("migration 193 ainda não aplicada — teste pulado")
	}

	fake := "00000000-0000-4000-8000-0000000f0001"
	defer db.Exec(`DELETE FROM farol.consolidacao_log WHERE empresa_id=$1`, fake)

	// Sem linha e sem jobs → vazio, não uma data inventada.
	db.Exec(`DELETE FROM farol.consolidacao_log WHERE empresa_id=$1`, fake)
	if got := biUltimoImport(db, fake); got != "" {
		t.Errorf("empresa sem consolidação e sem import → %q, esperado vazio", got)
	}

	// Depois de consolidar, o carimbo aparece e é recente.
	marcaConsolidacao(db, fake)
	got := biUltimoImport(db, fake)
	if got == "" {
		t.Fatal("após marcaConsolidacao, carimbo veio vazio")
	}
	ts, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("carimbo %q não é RFC3339: %v", got, err)
	}
	if d := time.Since(ts); d > time.Minute || d < -time.Minute {
		t.Errorf("carimbo %v está a %v de agora — esperado ~0", ts, d)
	}

	// Segunda consolidação avança o carimbo (UPSERT, não INSERT duplicado).
	time.Sleep(1100 * time.Millisecond)
	marcaConsolidacao(db, fake)
	got2 := biUltimoImport(db, fake)
	if got2 == got {
		t.Errorf("segunda consolidação não avançou o carimbo (%q)", got2)
	}
}

// TestBIFaturadoPorUF — o bloco geográfico: soma por UF tem de bater com o
// SUM(pvenda) total do período, e vir ordenada desc, com cor pela régua YoY.
func TestBIFaturadoPorUF(t *testing.T) {
	db, empresaID := biTestDB(t)
	defer db.Close()

	// Faturado lê a MV mv_fat_uf_mes (mig 194), que depende de vendas_ccd +
	// tipo_venda — só existe num banco com migrations em dia. Onde a MV não
	// existe (dev desatualizado), cai para o transmitido, cujo caminho é um
	// scan de vendas_transmitidas e valida a mesma mecânica (alias, cor, ordem).
	fluxo := "faturado"
	var mvExiste bool
	// MV não aparece em information_schema.tables — usar pg_matviews.
	_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM pg_matviews
		WHERE schemaname='farol' AND matviewname='mv_fat_uf_mes')`).Scan(&mvExiste)
	if !mvExiste {
		fluxo = "transmitido"
		t.Logf("mv_fat_uf_mes ausente — validando o caminho %s", fluxo)
	}

	invalidateBICache(empresaID)
	out, _ := biGet(t, db, "/api/v2/farol/bi?comp_mode=ytd&fluxo="+fluxo, empresaID)
	if len(out.UFs) == 0 {
		t.Skip("base sem UF no período — nada a verificar")
	}

	// Ordenação desc por faturado.
	for i := 1; i < len(out.UFs); i++ {
		if out.UFs[i].Faturado > out.UFs[i-1].Faturado {
			t.Errorf("UFs fora de ordem: [%d]=%.2f > [%d]=%.2f",
				i, out.UFs[i].Faturado, i-1, out.UFs[i-1].Faturado)
		}
	}
	// Cor idêntica à régua do pickCor (mesma de biCor): verde sse pct>=100,
	// onde pct = atual/ant (ant>0) | 100 (ant<=0 e atual>0) | 0 (senão).
	for _, u := range out.UFs {
		verde := (u.FaturadoAnt > 0 && u.Faturado >= u.FaturadoAnt) ||
			(u.FaturadoAnt <= 0 && u.Faturado > 0)
		esperada := "vermelho"
		if verde {
			esperada = "verde"
		}
		if u.Cor != esperada {
			t.Errorf("UF %s: cor=%s, esperado=%s (fat=%.2f ant=%.2f)",
				u.Estado, u.Cor, esperada, u.Faturado, u.FaturadoAnt)
		}
	}

	var somaUF float64
	for _, u := range out.UFs {
		somaUF += u.Faturado
	}
	if somaUF <= 0 {
		t.Error("soma do faturado por UF veio zero/negativa")
	}

	// Reconciliação (o motivo de existir da MV): no faturado, a soma por UF tem
	// de bater com o faturado do KPI (ambos são líquido). Pode diferir por
	// órfãos/eventos sem faturado casado (ver deferred-work) — por isso 1% de
	// tolerância em vez de igualdade exata.
	if fluxo == "faturado" && out.KPI.TotalFaturado > 0 {
		diff := somaUF - out.KPI.TotalFaturado
		if diff < 0 {
			diff = -diff
		}
		if diff > out.KPI.TotalFaturado*0.01 {
			t.Errorf("soma UF (%.2f) diverge do faturado do KPI (%.2f) além de 1%%",
				somaUF, out.KPI.TotalFaturado)
		}
	}
	t.Logf("UFs=%d somaUF=%.2f kpiFat=%.2f (top: %s %.2f)",
		len(out.UFs), somaUF, out.KPI.TotalFaturado, out.UFs[0].Estado, out.UFs[0].Faturado)
}

// TestBIFluxoInvalidoNaoVaraParaCCD — ?fluxo=cancdev cairia em scan da base.
func TestBIFluxoInvalidoNaoVaraParaCCD(t *testing.T) {
	db, empresaID := biTestDB(t)
	defer db.Close()

	invalidateBICache(empresaID)
	out, _ := biGet(t, db, "/api/v2/farol/bi?comp_mode=ytd&fluxo=cancdev", empresaID)
	if out.Periodo.Fluxo != "faturado" {
		t.Errorf("fluxo inválido virou %q, esperado coerção para faturado", out.Periodo.Fluxo)
	}
}

// TestBITransmitidoTemValores — regressão: usar cardItem.Faturado (vazio no
// fluxo transmitido) zerava donut e ranking inteiros.
func TestBITransmitidoTemValores(t *testing.T) {
	db, empresaID := biTestDB(t)
	defer db.Close()

	invalidateBICache(empresaID)
	out, _ := biGet(t, db, "/api/v2/farol/bi?comp_mode=ytd&fluxo=transmitido", empresaID)
	if out.Periodo.Fluxo != "transmitido" {
		t.Fatalf("fluxo=%q, esperado transmitido", out.Periodo.Fluxo)
	}
	if len(out.Industrias) == 0 {
		t.Skip("base sem dado transmitido — nada a verificar")
	}
	var soma float64
	for _, i := range out.Industrias {
		soma += i.Faturado
	}
	if soma == 0 {
		t.Error("fluxo transmitido devolveu todas as indústrias zeradas")
	}
	if !sort.SliceIsSorted(out.Industrias, func(a, b int) bool {
		return out.Industrias[a].Faturado > out.Industrias[b].Faturado
	}) {
		// "Outros" é a última fatia e pode quebrar a ordem legitimamente.
		if len(out.Industrias) > 0 && out.Industrias[len(out.Industrias)-1].Label != "Outros" {
			t.Error("indústrias fora de ordem decrescente")
		}
	}
}
