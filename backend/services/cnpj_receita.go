package services

// cnpj_receita.go — enriquecimento do cadastro de clientes pela Receita Federal.
//
// POR QUE. A lista de "clientes sem venda" mistura duas coisas que exigem ações
// opostas: cliente que o concorrente levou (falha de venda, alguém tem que agir)
// e cliente que fechou as portas (não havia o que fazer, e mandar o RCA lá é
// rota desperdiçada). Amostra de 25/08/2026 em 40 clientes que compraram em 2025
// e nada em 2026: 2 BAIXADA, 3 INAPTA — e nos cinco a mudança de situação
// cadastral cai na janela em que a compra parou.
//
// FONTE. BrasilAPI, que serve o dump mensal da Receita. Pública, sem cadastro e
// sem chave. Justamente por ser gratuita e comunitária, este worker é lento de
// propósito: ver `PausaPadrao`.
//
// RETOMÁVEL POR CONSTRUÇÃO. Cada CNPJ é gravado assim que chega, não ao final.
// A fila é derivada — "está em vendas e não está em cnpj_receita" —, então não
// existe estado paralelo para dessincronizar. Interromper no meio custa, no
// máximo, o CNPJ que estava em voo.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// PausaPadrao entre consultas. Medido em 25/08/2026: latência média de 0,62s
	// e nenhum bloqueio a ~1,1 req/s. Não acelere sem medir de novo — 47 mil
	// consultas contra uma API pública gratuita é abuso se feito com pressa, e
	// o custo de sermos bloqueados é perder a fonte inteira.
	PausaPadrao = 300 * time.Millisecond

	// LimiteFalhasSeguidas — falhas de rede em sequência significam que a outra
	// ponta caiu ou nos cortou. Insistir só piora; paramos e o que já entrou fica.
	LimiteFalhasSeguidas = 8

	brasilAPIBase = "https://brasilapi.com.br/api/cnpj/v1/"
)

// ResultadoCargaCNPJ — o que aconteceu numa rodada.
type ResultadoCargaCNPJ struct {
	Candidatos    int
	Sucesso       int
	NaoEncontrado int
	Falha         int
	Duracao       time.Duration
	// Interrompido — motivo de ter parado antes de esgotar a fila. Vazio =
	// terminou por conta própria. Precisa chegar a quem disparou: uma rodada
	// que parou no CNPJ 300 de 8.507 não pode parecer uma rodada completa.
	Interrompido string
}

// respostaCNPJ — só os campos que persistimos. O `qsa` da API é deliberadamente
// ignorado: nome de sócio e CPF mascarado são pessoa natural, e para análise de
// venda não servem a nada (ver migração 208).
type respostaCNPJ struct {
	CNPJ                  string          `json:"cnpj"`
	RazaoSocial           *string         `json:"razao_social"`
	NomeFantasia          *string         `json:"nome_fantasia"`
	SituacaoCadastral     *int            `json:"situacao_cadastral"`
	DescSituacao          *string         `json:"descricao_situacao_cadastral"`
	DataSituacao          *string         `json:"data_situacao_cadastral"`
	MotivoSituacao        *string         `json:"descricao_motivo_situacao_cadastral"`
	CNAEFiscal            *int64          `json:"cnae_fiscal"`
	CNAEFiscalDesc        *string         `json:"cnae_fiscal_descricao"`
	CNAEsSecundarios      json.RawMessage `json:"cnaes_secundarios"`
	NaturezaJuridica      *string         `json:"natureza_juridica"`
	Porte                 *string         `json:"porte"`
	CapitalSocial         *float64        `json:"capital_social"`
	DataInicioAtividade   *string         `json:"data_inicio_atividade"`
	IdentificadorMatriz   *int            `json:"identificador_matriz_filial"`
	Municipio             *string         `json:"municipio"`
	UF                    *string         `json:"uf"`
	CEP                   *string         `json:"cep"`
	Logradouro            *string         `json:"logradouro"`
	Numero                *string         `json:"numero"`
	Bairro                *string         `json:"bairro"`
	MunicipioIBGE         *int            `json:"codigo_municipio_ibge"`
	Telefone              *string         `json:"ddd_telefone_1"`
	Email                 *string         `json:"email"`
	OpcaoSimples          *bool           `json:"opcao_pelo_simples"`
	OpcaoMEI              *bool           `json:"opcao_pelo_mei"`
	RegimeTributario      json.RawMessage `json:"regime_tributario"`
	// Mensagem — a API devolve 404 com corpo {"message": "..."} para CNPJ que
	// não existe no dump. Distinguir isso de falha de rede importa: um é
	// definitivo e vira linha com erro, o outro merece nova tentativa.
	Mensagem *string `json:"message"`
}

// CarregarCNPJReceita processa até `limite` CNPJs ainda não consultados.
//
// Ordem da fila: clientes que pararam de comprar primeiro (menor `ultimo_ano`).
// São eles que respondem a pergunta que motivou a tabela; os ativos entram
// depois, para segmentação por CNAE, que é análise nova e pode esperar.
func CarregarCNPJReceita(ctx context.Context, db *sql.DB, empresaID string, limite int, pausa time.Duration) (ResultadoCargaCNPJ, error) {
	t0 := time.Now()
	res := ResultadoCargaCNPJ{}

	if pausa <= 0 {
		pausa = PausaPadrao
	}
	if limite <= 0 {
		limite = 1000
	}

	docs, err := filaCNPJ(ctx, db, empresaID, limite)
	if err != nil {
		return res, fmt.Errorf("montar fila: %w", err)
	}
	res.Candidatos = len(docs)
	if len(docs) == 0 {
		log.Printf("[cnpj:receita] fila vazia — nada a consultar")
		return res, nil
	}
	log.Printf("[cnpj:receita] iniciando %d CNPJ(s), pausa de %v entre consultas (~%s estimados)",
		len(docs), pausa, estimativa(len(docs), pausa))

	cli := &http.Client{Timeout: 25 * time.Second}
	falhasSeguidas := 0

	for i, doc := range docs {
		select {
		case <-ctx.Done():
			res.Interrompido = "contexto cancelado"
			res.Duracao = time.Since(t0)
			log.Printf("[cnpj:receita] INTERROMPIDO em %d/%d: %s", i, len(docs), res.Interrompido)
			return res, nil
		default:
		}

		r, status, err := consultarCNPJ(ctx, cli, doc)
		switch {
		case err != nil:
			falhasSeguidas++
			res.Falha++
			log.Printf("[cnpj:receita] %s ERRO (%d seguida(s)): %v", doc, falhasSeguidas, err)
			if falhasSeguidas >= LimiteFalhasSeguidas {
				res.Interrompido = fmt.Sprintf("%d falhas seguidas — a fonte caiu ou nos cortou", falhasSeguidas)
				res.Duracao = time.Since(t0)
				log.Printf("[cnpj:receita] INTERROMPIDO em %d/%d: %s", i+1, len(docs), res.Interrompido)
				return res, nil
			}
			// Recuo crescente: cada falha seguida dobra a espera, até 60s. Se for
			// limite de taxa, insistir no mesmo ritmo garante continuar bloqueado.
			espera := pausa * time.Duration(1<<uint(falhasSeguidas))
			if espera > 60*time.Second {
				espera = 60 * time.Second
			}
			time.Sleep(espera)
			continue

		case status == http.StatusNotFound:
			falhasSeguidas = 0
			res.NaoEncontrado++
			// Grava com erro para a fila não devolver este CNPJ para sempre. É
			// resultado definitivo: o documento não está no dump da Receita.
			if e := gravarErro(ctx, db, doc, "nao_encontrado"); e != nil {
				log.Printf("[cnpj:receita] %s gravar 'nao_encontrado' falhou: %v", doc, e)
			}

		default:
			falhasSeguidas = 0
			if e := gravarCNPJ(ctx, db, doc, r); e != nil {
				res.Falha++
				log.Printf("[cnpj:receita] %s gravar falhou: %v", doc, e)
			} else {
				res.Sucesso++
			}
		}

		if (i+1)%250 == 0 {
			log.Printf("[cnpj:receita] %d/%d (ok=%d nao_encontrado=%d falha=%d) em %v",
				i+1, len(docs), res.Sucesso, res.NaoEncontrado, res.Falha, time.Since(t0).Round(time.Second))
		}
		time.Sleep(pausa)
	}

	res.Duracao = time.Since(t0)
	log.Printf("[cnpj:receita] CONCLUÍDO %d CNPJ(s): ok=%d nao_encontrado=%d falha=%d em %v",
		len(docs), res.Sucesso, res.NaoEncontrado, res.Falha, res.Duracao.Round(time.Second))
	return res, nil
}

func estimativa(n int, pausa time.Duration) string {
	// 0,62s de latência medida em produção 25/08/2026, mais a pausa.
	d := time.Duration(n) * (620*time.Millisecond + pausa)
	return d.Round(time.Minute).String()
}

// filaCNPJ — quem está em vendas e ainda não tem linha em cnpj_receita.
//
// A fila é DERIVADA, não materializada: não existe tabela de pendências para
// sair de sincronia com a realidade. Cliente novo entra sozinho na próxima
// rodada; CNPJ já resolvido some da fila por já ter linha.
func filaCNPJ(ctx context.Context, db *sql.DB, empresaID string, limite int) ([]string, error) {
	const q = `
WITH docs AS (
    SELECT regexp_replace(cnpj, '[^0-9]', '', 'g') AS doc,
           MAX(ano) AS ultimo_ano
      FROM farol.agg_fat_mkt_cli_mes
     WHERE empresa_id = $1 AND cnpj <> ''
     GROUP BY 1
)
SELECT d.doc
  FROM docs d
  LEFT JOIN farol.cnpj_receita r ON r.cnpj = d.doc
 WHERE length(d.doc) = 14
   AND r.cnpj IS NULL
 ORDER BY d.ultimo_ano, d.doc
 LIMIT $2`

	rows, err := db.QueryContext(ctx, q, empresaID, limite)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

func consultarCNPJ(ctx context.Context, cli *http.Client, doc string) (*respostaCNPJ, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, brasilAPIBase+doc, nil)
	if err != nil {
		return nil, 0, err
	}
	// Identificação honesta: se estivermos pesando demais, o mantenedor da API
	// consegue saber de onde vem em vez de simplesmente bloquear.
	req.Header.Set("User-Agent", "FB_FAROL/1.2 (+https://farol.fbtax.cloud)")
	req.Header.Set("Accept", "application/json")

	resp, err := cli.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, resp.StatusCode, nil
	}
	// 429 e 5xx são temporários e viram erro para entrar no recuo crescente.
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, trecho(body, 120))
	}

	var r respostaCNPJ
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("json inválido: %w", err)
	}
	return &r, resp.StatusCode, nil
}

// trecho — primeiros n bytes do corpo, para a mensagem de erro. Apara ANTES de
// medir: fatiar a string aparada usando o tamanho da original entra em pânico
// quando o corpo tem espaço em volta, e num caminho de erro isso derruba o
// worker justamente quando ele já está lidando com problema.
func trecho(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n]
	}
	return s
}

func gravarErro(ctx context.Context, db *sql.DB, doc, motivo string) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO farol.cnpj_receita (cnpj, erro, consultado_em)
VALUES ($1, $2, NOW())
ON CONFLICT (cnpj) DO UPDATE SET erro = EXCLUDED.erro, consultado_em = NOW()`, doc, motivo)
	return err
}

func gravarCNPJ(ctx context.Context, db *sql.DB, doc string, r *respostaCNPJ) error {
	if r == nil {
		return fmt.Errorf("resposta vazia")
	}
	var cnae *string
	if r.CNAEFiscal != nil {
		s := fmt.Sprintf("%d", *r.CNAEFiscal)
		cnae = &s
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO farol.cnpj_receita (
    cnpj, razao_social, nome_fantasia,
    situacao_cod, situacao_desc, situacao_data, situacao_motivo,
    cnae_cod, cnae_desc, cnaes_secundarios, natureza_juridica, porte,
    capital_social, data_inicio_atividade, matriz_filial,
    municipio, uf, cep, logradouro, numero, bairro, municipio_ibge,
    telefone, email, opcao_simples, opcao_mei, regime_tributario,
    consultado_em, fonte, erro)
VALUES ($1,$2,$3,$4,$5,$6::date,$7,$8,$9,$10::jsonb,$11,$12,$13,$14::date,$15,
        $16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27::jsonb,NOW(),'brasilapi',NULL)
ON CONFLICT (cnpj) DO UPDATE SET
    razao_social=EXCLUDED.razao_social, nome_fantasia=EXCLUDED.nome_fantasia,
    situacao_cod=EXCLUDED.situacao_cod, situacao_desc=EXCLUDED.situacao_desc,
    situacao_data=EXCLUDED.situacao_data, situacao_motivo=EXCLUDED.situacao_motivo,
    cnae_cod=EXCLUDED.cnae_cod, cnae_desc=EXCLUDED.cnae_desc,
    cnaes_secundarios=EXCLUDED.cnaes_secundarios,
    natureza_juridica=EXCLUDED.natureza_juridica, porte=EXCLUDED.porte,
    capital_social=EXCLUDED.capital_social,
    data_inicio_atividade=EXCLUDED.data_inicio_atividade,
    matriz_filial=EXCLUDED.matriz_filial,
    municipio=EXCLUDED.municipio, uf=EXCLUDED.uf, cep=EXCLUDED.cep,
    logradouro=EXCLUDED.logradouro, numero=EXCLUDED.numero, bairro=EXCLUDED.bairro,
    municipio_ibge=EXCLUDED.municipio_ibge,
    telefone=EXCLUDED.telefone, email=EXCLUDED.email,
    opcao_simples=EXCLUDED.opcao_simples, opcao_mei=EXCLUDED.opcao_mei,
    regime_tributario=EXCLUDED.regime_tributario,
    consultado_em=NOW(), erro=NULL`,
		doc, r.RazaoSocial, r.NomeFantasia,
		r.SituacaoCadastral, r.DescSituacao, vazioParaNulo(r.DataSituacao), r.MotivoSituacao,
		cnae, r.CNAEFiscalDesc, jsonOuNulo(r.CNAEsSecundarios), r.NaturezaJuridica, r.Porte,
		r.CapitalSocial, vazioParaNulo(r.DataInicioAtividade), r.IdentificadorMatriz,
		r.Municipio, r.UF, r.CEP, r.Logradouro, r.Numero, r.Bairro, r.MunicipioIBGE,
		r.Telefone, r.Email, r.OpcaoSimples, r.OpcaoMEI, jsonOuNulo(r.RegimeTributario))
	return err
}

// vazioParaNulo — a API devolve "" (não null) em datas ausentes, e "" não é
// data válida para o Postgres. Sem isto, o INSERT quebra em quem tem cadastro
// incompleto — justamente os casos mais interessantes.
func vazioParaNulo(s *string) any {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return *s
}

func jsonOuNulo(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}
