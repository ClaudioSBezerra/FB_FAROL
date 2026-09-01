package handlers

// farol_api_produtos_faturados.go — Endpoint machine-to-machine consumido pelo
// SmartPick (Monitor de Faturamento sem Calibragem). Não é uma sessão de
// usuário: autenticação por API key estática, vinculada a UMA empresa fixa do
// Farol via env vars — escolha deliberada para não vazar dado entre
// clientes/tenants que podem compartilhar códigos de filial WinThor (ex: "11").
//
// GET /api/farol/produtos-faturados?empresa={cod_filial}&data_ini=YYYY-MM-DD&data_fim=YYYY-MM-DD
// Header: X-API-Key: {FAROL_API_KEY}

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type produtoFaturadoAPI struct {
	CodProd         string  `json:"cod_prod"`
	Empresa         string  `json:"empresa"`
	DataFaturamento string  `json:"data_faturamento"`
	Qt              float64 `json:"qt"`
	// CodDepto/CodSec (mig 184, coluna direta de vendas_faturadas) — adicionados
	// 31/08/2026 pro SmartPick casar produto→Seção e cruzar com o índice
	// sazonal de /api/farol/sazonalidade-secao, sem precisar de uma segunda
	// chamada de "dimensão de produto" que não existe hoje.
	CodDepto string `json:"cod_depto,omitempty"`
	CodSec   string `json:"cod_sec,omitempty"`
}

// FarolAPIKeyAuth valida o header X-API-Key contra FAROL_API_KEY (comparação em
// tempo constante) e resolve a empresa_id fixa vinculada à key
// (FAROL_API_KEY_EMPRESA_ID). Sem as duas env vars configuradas, a integração
// fica desabilitada por padrão (503) — nunca aceita chamada não configurada.
func FarolAPIKeyAuth(next func(db *sql.DB, empresaID string) http.HandlerFunc) func(db *sql.DB) http.HandlerFunc {
	return func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")

			expectedKey := os.Getenv("FAROL_API_KEY")
			empresaID := os.Getenv("FAROL_API_KEY_EMPRESA_ID")
			if expectedKey == "" || empresaID == "" {
				http.Error(w, `{"error":"Integração não configurada"}`, http.StatusServiceUnavailable)
				return
			}

			got := r.Header.Get("X-API-Key")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expectedKey)) != 1 {
				http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
				return
			}

			next(db, empresaID)(w, r)
		}
	}
}

// ProdutosFaturadosAPIHandler retorna linhas de vendas_faturadas da empresa
// vinculada à API key, filtradas por filial (cod_filial, campo "empresa" da
// tabela) e janela de datas — consumido pelo SmartPick para cruzar com
// calibragem aprovada.
func ProdutosFaturadosAPIHandler(db *sql.DB, empresaID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		empresaFilial := r.URL.Query().Get("empresa")
		dataIniStr := r.URL.Query().Get("data_ini")
		dataFimStr := r.URL.Query().Get("data_fim")
		if empresaFilial == "" || dataIniStr == "" || dataFimStr == "" {
			http.Error(w, `{"error":"parâmetros obrigatórios: empresa, data_ini, data_fim"}`, http.StatusBadRequest)
			return
		}
		dataIni, err := time.Parse("2006-01-02", dataIniStr)
		if err != nil {
			http.Error(w, `{"error":"data_ini inválida (use YYYY-MM-DD)"}`, http.StatusBadRequest)
			return
		}
		dataFim, err := time.Parse("2006-01-02", dataFimStr)
		if err != nil {
			http.Error(w, `{"error":"data_fim inválida (use YYYY-MM-DD)"}`, http.StatusBadRequest)
			return
		}

		rows, err := db.Query(`
			SELECT cod_prod, empresa, data_faturamento, qt, cod_depto, cod_sec
			  FROM vendas_faturadas
			 WHERE empresa_id = $1 AND empresa = $2
			   AND data_faturamento >= $3 AND data_faturamento <= $4
			   AND cod_prod <> ''
		`, empresaID, empresaFilial, dataIni, dataFim)
		if err != nil {
			log.Printf("[farol-api] erro consultando vendas_faturadas: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		produtos := []produtoFaturadoAPI{}
		for rows.Next() {
			var p produtoFaturadoAPI
			var data time.Time
			if err := rows.Scan(&p.CodProd, &p.Empresa, &data, &p.Qt, &p.CodDepto, &p.CodSec); err != nil {
				log.Printf("[farol-api] erro no scan de vendas_faturadas: %v", err)
				http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
				return
			}
			p.DataFaturamento = data.Format("2006-01-02")
			produtos = append(produtos, p)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[farol-api] erro iterando vendas_faturadas: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(produtos)
	}
}

// ─── Sazonalidade por Seção ──────────────────────────────────────────────────
//
// GET /api/farol/sazonalidade-secao?empresa={cod_filial}
// Header: X-API-Key: {FAROL_API_KEY}  (mesmo esquema do endpoint acima)
//
// Índice sazonal = venda do mês / média mensal do ano, calculado sobre 2025
// (único ano fechado hoje — 31/08/2026). Nível Seção, não Categoria (Claudio
// reportou que o cadastro de Categoria não está saneado — muitos produtos sem
// categoria correta) nem Indústria (mistura produtos de sazonalidade bem
// diferente sob o mesmo fornecedor — ex: Alpargatas vende sandália de verão E
// bota de inverno). Por FILIAL, não por empresa inteira: sazonalidade pode ser
// regional (ex: Protetor Solar pica em julho em Goiás/Tocantins por causa da
// temporada de praia fluvial do Rio Araguaia — o oposto do padrão litorâneo de
// verão — achado real analisando os dados em 31/08/2026, não teria aparecido
// numa média nacional).
//
// Consumido pelo SmartPick (Monitor de Faturamento sem Calibragem) pra marcar
// produtos cujo pico de faturamento é sazonal (não necessariamente falta de
// calibragem de picking) — ver spec/memória do lado do SmartPick. Plano de
// longo prazo (Claudio, 31/08/2026): o SmartPick vai ter um CRUD próprio de
// índices por Departamento×Seção, ajustável manualmente; este endpoint é o
// valor calculado que alimenta esse cadastro até ele existir.
type sazonalidadeSecaoAPI struct {
	CodDepto   string      `json:"cod_depto"`
	Depto      string      `json:"depto"`
	CodSec     string      `json:"cod_sec"`
	Secao      string      `json:"secao"`
	Indices    [12]float64 `json:"indices"`  // indices[0] = janeiro ... indices[11] = dezembro
	MesPico    int         `json:"mes_pico"` // 1-12; 0 se não há dado suficiente
	IndicePico float64     `json:"indice_pico"`
}

func SazonalidadeSecaoAPIHandler(db *sql.DB, empresaID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		empresaFilial := r.URL.Query().Get("empresa")
		if empresaFilial == "" {
			http.Error(w, `{"error":"parâmetro obrigatório: empresa"}`, http.StatusBadRequest)
			return
		}

		rows, err := db.Query(`
			WITH mensal AS (
				SELECT cod_depto, cod_sec, EXTRACT(MONTH FROM data_faturamento)::int AS mes,
				       SUM(pvenda) AS venda_mes
				  FROM vendas_faturadas
				 WHERE empresa_id = $1 AND empresa = $2
				   AND data_faturamento >= '2025-01-01' AND data_faturamento < '2026-01-01'
				   AND cod_sec <> ''
				 GROUP BY cod_depto, cod_sec, EXTRACT(MONTH FROM data_faturamento)
			),
			media AS (
				SELECT cod_depto, cod_sec, AVG(venda_mes) AS media_anual
				  FROM mensal
				 GROUP BY cod_depto, cod_sec
			),
			labels AS (
				SELECT DISTINCT ON (cod_depto, cod_sec) cod_depto, depto, cod_sec, secao
				  FROM vendas_faturadas
				 WHERE empresa_id = $1 AND empresa = $2 AND cod_sec <> ''
				 ORDER BY cod_depto, cod_sec, data_faturamento DESC
			)
			SELECT l.cod_depto, l.depto, l.cod_sec, l.secao, m.mes,
			       ROUND((m.venda_mes / NULLIF(a.media_anual,0))::numeric, 3) AS indice
			  FROM mensal m
			  JOIN media a USING (cod_depto, cod_sec)
			  JOIN labels l USING (cod_depto, cod_sec)
			 ORDER BY l.cod_depto, l.cod_sec, m.mes
		`, empresaID, empresaFilial)
		if err != nil {
			log.Printf("[farol-api] erro consultando sazonalidade por seção: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		porSecao := map[string]*sazonalidadeSecaoAPI{}
		var ordem []string
		for rows.Next() {
			var codDepto, depto, codSec, secao string
			var mes int
			var indice sql.NullFloat64
			if err := rows.Scan(&codDepto, &depto, &codSec, &secao, &mes, &indice); err != nil {
				log.Printf("[farol-api] erro no scan de sazonalidade por seção: %v", err)
				http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
				return
			}
			key := codDepto + "|" + codSec
			s, ok := porSecao[key]
			if !ok {
				s = &sazonalidadeSecaoAPI{CodDepto: codDepto, Depto: depto, CodSec: codSec, Secao: secao}
				porSecao[key] = s
				ordem = append(ordem, key)
			}
			if mes >= 1 && mes <= 12 {
				s.Indices[mes-1] = indice.Float64
				if indice.Float64 > s.IndicePico {
					s.IndicePico = indice.Float64
					s.MesPico = mes
				}
			}
		}
		if err := rows.Err(); err != nil {
			log.Printf("[farol-api] erro iterando sazonalidade por seção: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}

		out := make([]sazonalidadeSecaoAPI, 0, len(ordem))
		for _, key := range ordem {
			out = append(out, *porSecao[key])
		}
		json.NewEncoder(w).Encode(out)
	}
}

// ─── Sazonalidade por Produto ────────────────────────────────────────────────
//
// GET /api/farol/sazonalidade-produto?empresa={cod_filial}&ano={opcional}
// Header: X-API-Key: {FAROL_API_KEY}  (mesmo esquema dos endpoints acima)
//
// Diferente de SazonalidadeSecaoAPIHandler (calculado ao vivo, ano 2025
// hardcoded), este endpoint só LÊ farol.agg_sazonalidade_produto_ano (mig
// 212) — persistida, atualizada de forma assíncrona pelo pipeline de import
// (upsertSazonalidadeProdutoAnos). SELECT indexado, rápido, sem agregação em
// tempo real; não sofre do mesmo risco de timeout já visto na versão por
// Seção sob carga pesada.
//
// ano opcional: quando ausente, resolve o último ano com o ano fechado (12
// meses de dado) — sem hardcode de calendário, migra sozinho pro ano
// seguinte assim que ele fechar.
type sazonalidadeProdutoAPI struct {
	CodProd        string   `json:"cod_prod"`
	NomeProd       string   `json:"nome_prod"`
	Empresa        string   `json:"empresa"`
	Ano            int      `json:"ano"`
	CodDepto       string   `json:"cod_depto"`
	CodSec         string   `json:"cod_sec"`
	Sazonal        bool     `json:"sazonal"`
	MesPico        *int     `json:"mes_pico,omitempty"` // 1-12; ausente se sem dado suficiente
	IndicePico     *float64 `json:"indice_pico,omitempty"`
	QtMesPico      float64  `json:"qt_mes_pico"`
	QtTotalAno     float64  `json:"qt_total_ano"`
	PvendaMesPico  float64  `json:"pvenda_mes_pico"`
	PvendaTotalAno float64  `json:"pvenda_total_ano"`
	MesesComDado   int      `json:"meses_com_dado"`
}

func SazonalidadeProdutoAPIHandler(db *sql.DB, empresaID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		empresaFilial := r.URL.Query().Get("empresa")
		if empresaFilial == "" {
			http.Error(w, `{"error":"parâmetro obrigatório: empresa"}`, http.StatusBadRequest)
			return
		}

		var ano int
		if anoStr := r.URL.Query().Get("ano"); anoStr != "" {
			v, err := strconv.Atoi(anoStr)
			if err != nil {
				http.Error(w, `{"error":"parâmetro ano inválido"}`, http.StatusBadRequest)
				return
			}
			ano = v
		} else {
			// Sem hardcode de calendário: último ano com os 12 meses presentes
			// pra essa empresa — pula sozinho pro ano seguinte assim que ele
			// fechar, sem deploy. Se nenhum ano estiver totalmente fechado ainda
			// (empresa nova), cai no ano mais recente disponível (parcial).
			err := db.QueryRow(`
				SELECT COALESCE(
					(SELECT MAX(ano) FROM farol.agg_sazonalidade_produto_ano
					  WHERE empresa_id = $1 AND meses_com_dado = 12),
					(SELECT MAX(ano) FROM farol.agg_sazonalidade_produto_ano
					  WHERE empresa_id = $1)
				)
			`, empresaID).Scan(&ano)
			if err != nil || ano == 0 {
				http.Error(w, `{"error":"nenhum dado de sazonalidade disponível ainda"}`, http.StatusNotFound)
				return
			}
		}

		rows, err := db.Query(`
			SELECT cod_prod, nome_prod, empresa, ano, cod_depto, cod_sec, sazonal,
			       mes_pico, indice_pico, qt_mes_pico, qt_total_ano,
			       pvenda_mes_pico, pvenda_total_ano, meses_com_dado
			  FROM farol.agg_sazonalidade_produto_ano
			 WHERE empresa_id = $1 AND empresa = $2 AND ano = $3
		`, empresaID, empresaFilial, ano)
		if err != nil {
			log.Printf("[farol-api] erro consultando sazonalidade por produto: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		out := []sazonalidadeProdutoAPI{}
		for rows.Next() {
			var it sazonalidadeProdutoAPI
			var mesPico sql.NullInt64
			var indicePico sql.NullFloat64
			if err := rows.Scan(
				&it.CodProd, &it.NomeProd, &it.Empresa, &it.Ano, &it.CodDepto, &it.CodSec, &it.Sazonal,
				&mesPico, &indicePico, &it.QtMesPico, &it.QtTotalAno,
				&it.PvendaMesPico, &it.PvendaTotalAno, &it.MesesComDado,
			); err != nil {
				log.Printf("[farol-api] erro no scan de sazonalidade por produto: %v", err)
				http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
				return
			}
			if mesPico.Valid {
				v := int(mesPico.Int64)
				it.MesPico = &v
			}
			if indicePico.Valid {
				it.IndicePico = &indicePico.Float64
			}
			out = append(out, it)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[farol-api] erro iterando sazonalidade por produto: %v", err)
			http.Error(w, `{"error":"Erro interno"}`, http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(out)
	}
}
