package handlers

// farol_relatorios.go — API de relatórios gerenciais do Farol
//
// GET /api/v2/farol/relatorio/extrato-produto-cliente
//   Relatório de extrato de vendas de um produto para um cliente, mês a mês

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// ─── Tipos ─────────────────────────────────────────────────────────────────────

type VendaExtrato struct {
	Ano             int64   `json:"ano"`
	Mes             int64   `json:"mes"`
	Periodo         string  `json:"periodo"`
	NomeProd        string  `json:"nome_prod"`
	NomeCli         string  `json:"nome_cli"`
	CNPJ            string  `json:"cnpj"`
	CodFornec       string  `json:"cod_fornec"`
	NomeFornec      string  `json:"nome_fornec"`
	QtNFs           int64   `json:"qt_nfs"`
	QtTotal         float64 `json:"qt_total"`
	ValorVenda      float64 `json:"valor_venda"`
	ValorLucro      float64 `json:"valor_lucro"`
	TicketMedioNF   float64 `json:"ticket_medio_nf"`
	PrecoMedioUnit  float64 `json:"preco_medio_unit"`
}

type RelatorioExtratoResponse struct {
	Dados    []VendaExtrato `json:"dados"`
	Produto  struct {
		Cod  string `json:"cod"`
		Nome string `json:"nome"`
	} `json:"produto"`
	Cliente struct {
		Cod  string `json:"cod"`
		Nome string `json:"nome"`
	} `json:"cliente"`
}

// ─── Handler ───────────────────────────────────────────────────────────────────

// ExtratoProdutoClienteHandler — relatório de vendas de um produto para um cliente
func ExtratoProdutoClienteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Apenas autenticados
		userID := r.Context().Value("user_id")
		if userID == nil {
			http.Error(w, "Não autorizado", http.StatusUnauthorized)
			return
		}

		empresaID := r.Context().Value("company_id")
		if empresaID == nil {
			http.Error(w, "Empresa não encontrada", http.StatusBadRequest)
			return
		}

		// Parâmetros
		codProduto := r.URL.Query().Get("cod_produto")
		codCliente := r.URL.Query().Get("cod_cliente")

		if codProduto == "" {
			http.Error(w, "cod_produto é obrigatório", http.StatusBadRequest)
			return
		}
		if codCliente == "" {
			http.Error(w, "cod_cliente é obrigatório", http.StatusBadRequest)
			return
		}

		// Query principal
		query := `
			SELECT
				EXTRACT(YEAR FROM vf.data_faturamento)::INTEGER AS ano,
				EXTRACT(MONTH FROM vf.data_faturamento)::INTEGER AS mes,
				EXTRACT(YEAR FROM vf.data_faturamento)::INTEGER || '/' || LPAD(EXTRACT(MONTH FROM vf.data_faturamento)::TEXT, 2, '0') AS periodo,
				vf.nome_prod,
				vf.nome_cli,
				vf.cnpj,
				vf.cod_fornec,
				vf.nome_fornec,
				COUNT(*) AS qt_nfs,
				COALESCE(SUM(vf.qt), 0) AS qt_total,
				COALESCE(SUM(vf.pvenda), 0) AS valor_venda,
				COALESCE(SUM(vf.plucro), 0) AS valor_lucro,
				CASE WHEN COUNT(*) > 0 THEN COALESCE(SUM(vf.pvenda), 0) / COUNT(*) ELSE 0 END AS ticket_medio_nf,
				CASE WHEN COALESCE(SUM(vf.qt), 0) > 0 THEN COALESCE(SUM(vf.pvenda), 0) / COALESCE(SUM(vf.qt), 1) ELSE 0 END AS preco_medio_unit
			FROM public.vendas_faturadas vf
			WHERE vf.empresa_id = $1
				AND vf.cod_prod = $2
				AND vf.cod_cli = $3
				AND vf.data_faturamento BETWEEN '2025-01-01' AND '2026-12-31'
			GROUP BY
				EXTRACT(YEAR FROM vf.data_faturamento),
				EXTRACT(MONTH FROM vf.data_faturamento),
				vf.nome_prod,
				vf.nome_cli,
				vf.cnpj,
				vf.cod_fornec,
				vf.nome_fornec
			ORDER BY ano DESC, mes DESC
		`

		rows, err := db.Query(query, empresaID, codProduto, codCliente)
		if err != nil {
			log.Printf("[relatorio] erro na query: %v", err)
			http.Error(w, "Erro ao buscar dados", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var dados []VendaExtrato
		var nomeProd, nomeCli string

		for rows.Next() {
			var v VendaExtrato
			var ano, mes sql.NullInt64
			var qtTotal, valorVenda, valorLucro, ticketMedioNF, precoMedioUnit sql.NullFloat64

			err := rows.Scan(
				&ano, &mes, &v.Periodo,
				&v.NomeProd, &v.NomeCli, &v.CNPJ,
				&v.CodFornec, &v.NomeFornec,
				&v.QtNFs,
				&qtTotal, &valorVenda, &valorLucro,
				&ticketMedioNF, &precoMedioUnit,
			)
			if err != nil {
				log.Printf("[relatorio] erro no scan: %v", err)
				continue
			}

			if ano.Valid {
				v.Ano = ano.Int64
			}
			if mes.Valid {
				v.Mes = mes.Int64
			}
			if qtTotal.Valid {
				v.QtTotal = qtTotal.Float64
			}
			if valorVenda.Valid {
				v.ValorVenda = valorVenda.Float64
			}
			if valorLucro.Valid {
				v.ValorLucro = valorLucro.Float64
			}
			if ticketMedioNF.Valid {
				v.TicketMedioNF = ticketMedioNF.Float64
			}
			if precoMedioUnit.Valid {
				v.PrecoMedioUnit = precoMedioUnit.Float64
			}

			if nomeProd == "" {
				nomeProd = v.NomeProd
			}
			if nomeCli == "" {
				nomeCli = v.NomeCli
			}

			dados = append(dados, v)
		}

		// Montar resposta
		resp := RelatorioExtratoResponse{
			Dados: dados,
		}
		resp.Produto.Cod = codProduto
		resp.Produto.Nome = nomeProd
		resp.Cliente.Cod = codCliente
		resp.Cliente.Nome = nomeCli

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

