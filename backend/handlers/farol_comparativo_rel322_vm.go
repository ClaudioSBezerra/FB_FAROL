package handlers

// farol_comparativo_rel322_vm.go — o terceiro lado do comparativo REL 322:
// a base Oracle de ORIGEM (a mesma que o JC lê todo dia, ver
// jc_extrator.go — "banco paralelo 26ai", credenciais JC_ORACLE_*),
// consultada AO VIVO no momento do comparativo. Junto com o PDF do WinThor e
// o Farol (Postgres), separa "o WinThor já gerou o relatório errado" de "o
// import pro Farol perdeu alguma coisa no meio do caminho".
//
// NÃO é chamado em nenhum outro lugar do sistema — é específico deste
// comparativo. NÃO reimplementa a extração da JC (jc_extrator.go faz um
// dump completo pro CSV de import diário); aqui é só um SELECT agregado,
// direto na mesma view/credenciais, sem gravar nada.
//
// CUSTO CONHECIDO: a mesma view custa ~1 MINUTO FIXO por consulta,
// independente do volume (comentário de jc_extrator.go, medido em 29/07) —
// é o JOIN se montando, não volume de dado. Por isso os timeouts abaixo são
// generosos (20s ping + 100s query) e a chamada é sempre não-fatal: se a VM
// falhar ou estourar o tempo, o comparativo PDF×Farol continua valendo
// (VMIndisponivel=true na resposta) — nunca bloqueia o resultado principal.
//
// AVISO — esta query NUNCA foi exercitada contra o Oracle real (ambiente de
// dev não tem as credenciais JC_ORACLE_*): a sintaxe (binds posicionais
// `:N`, CASE WHEN, LIKE, UPPER) é Oracle padrão e foi revisada com cuidado,
// mas vale confirmar contra a base real (ou com quem mantém a integração da
// JC) antes de confiar cegamente no primeiro resultado em produção.

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// colOracleRel322 — traduz a coluna de escopo do Postgres (farol_escopo.go)
// pro nome equivalente na view Oracle da JC (colunasJC, jc_extrator.go).
// cod_rca não bate 1:1: a view chama a coluna de CODUSUR (nome herdado do
// layout do ION VENDAS), embora vire cod_rca depois de importada pro Postgres
// (ver farol_v2_import.go: `col(-1, "codusur", "cod_rca", "codrca")`).
func colOracleRel322(col string) string {
	switch col {
	case "cod_gerente":
		return "CODGERENTE"
	case "cod_rca":
		return "CODUSUR"
	default:
		return "CODSUPERVISOR"
	}
}

// montarQueryVM — monta o SELECT + args da consulta à VM. Extraída de
// vmLiquidoPorSupervisor pra ser testável SEM Oracle (o pacote de testes
// confere que os placeholders ":N" aparecem em ORDEM ESTRITAMENTE CRESCENTE
// no texto final — é a invariante cuja violação causou o ORA-00932 real em
// produção, ver AVISO grande abaixo).
//
// ESTADO contém "TRANS"/"CORT"/"CANCEL"/"DEVOL", senão é FATURADO por
// padrão — por isso os predicados usam LIKE, não igualdade exata, replicando
// o Contains() do Go (detectEvento, farol_v2_import.go) em vez de assumir
// literais fixos. CONDVENDA é o tipo_venda cru; tiposVenda já vem resolvido
// por tipoVendaSelecionado (default de cada fluxo quando o usuário não mexe
// no filtro da tela). filiais filtra por EMPRESA (mesma coluna que
// farol_v2_api.go usa como "Filial" no Postgres — nome herdado do layout do
// ION VENDAS).
//
// ORDEM DE bind() É CRÍTICA: apesar do nome ":N" sugerir bind por número, o
// go-ora (confirmado no código-fonte: command.go, useNamedParameters só roda
// quando o driver.NamedValue.Name vem preenchido — nunca é o caso aqui, que
// usa args posicionais simples) NÃO faz remapeamento por nome. Os valores
// são enviados na ORDEM de `args`, e o Oracle os associa aos ":N" na ORDEM
// EM QUE ELES APARECEM NO TEXTO final da query — não pelo dígito escrito
// depois dos dois-pontos. Por isso TODO bind() abaixo acontece na MESMA
// ORDEM em que o placeholder correspondente aparece na string final montada
// no fmt.Sprintf do fim da função (SELECT primeiro, depois WHERE) — nunca na
// ordem que seria mais conveniente pro código Go. Bug real já visto em
// produção: os binds de data apareciam DEPOIS, no WHERE, enquanto o SELECT
// (montado antes no código, mas ANTES no texto também) usava números de
// bind maiores — o Oracle associava a data a uma comparação de CONDVENDA
// (NUMBER), gerando ORA-00932.
func montarQueryVM(objeto string, dataInicio, dataFim time.Time, filiais, tiposVenda []string, escopo escopoRecorte, fluxo fluxoCtx) (string, []any) {
	ini := time.Date(dataInicio.Year(), dataInicio.Month(), dataInicio.Day(), 0, 0, 0, 0, time.UTC)
	fimExclusivo := time.Date(dataFim.Year(), dataFim.Month(), dataFim.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

	var args []any
	bind := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf(":%d", len(args))
	}

	// 1) SELECT list — somaNumerador e somaSubtrai são os primeiros
	// placeholders a aparecer no texto final.
	var somaNumerador, somaSubtrai, filtroScan string
	if fluxo.name == "transmitido" {
		// Numerador: TRANS por LIKE (não igualdade exata — replica o
		// Contains() de detectEvento). CONDVENDA só entra se o usuário
		// selecionou algo no filtro "Tipo de Venda" (tiposVenda vem nil pro
		// default do Transmitido — soma incondicional, como sempre foi).
		//
		// transPH tem que ser bindado ANTES do loop de tiposVenda — no texto
		// final ("LIKE %s%s") ele aparece primeiro, e bind() precisa ser
		// chamado na mesma ordem em que os placeholders aparecem no texto
		// (ver comentário grande acima sobre a ordem ser crítica).
		transPH := bind("%TRANS%")
		condvendaCond := ""
		if len(tiposVenda) > 0 {
			var ph []string
			for _, tv := range tiposVenda {
				n, convErr := strconv.Atoi(tv)
				if convErr != nil {
					continue // código não-numérico: ignora em vez de quebrar a query inteira
				}
				ph = append(ph, bind(n))
			}
			if len(ph) > 0 {
				condvendaCond = " AND CONDVENDA IN (" + strings.Join(ph, ", ") + ")"
			}
		}
		somaNumerador = fmt.Sprintf("SUM(CASE WHEN UPPER(ESTADO) LIKE %s%s THEN PVENDA ELSE 0 END)",
			transPH, condvendaCond)
		// Subtrai Cortado: só entra quando NÃO é Transmitido (mesma
		// precedência do detectEvento — TRANS é checado primeiro). Os dois
		// bind() aqui são argumentos diretos do mesmo Sprintf — Go avalia
		// argumentos de função da esquerda pra direita, então a ordem já
		// bate com o texto sem precisar de variável intermediária.
		somaSubtrai = fmt.Sprintf("SUM(CASE WHEN UPPER(ESTADO) NOT LIKE %s AND UPPER(ESTADO) LIKE %s THEN PVENDA ELSE 0 END)",
			bind("%TRANS%"), bind("%CORT%"))
	} else {
		// Faturado: numerador é "venda real" — linhas que NÃO são
		// TRANS/CORT/CANCEL/DEVOL (default FATURADO do detectEvento) E cujo
		// CONDVENDA está no conjunto selecionado (tiposVenda nunca vem vazio
		// aqui — tipoVendaSelecionado já resolveu pro default tipoVendaReal).
		//
		// transPH/cortPH bindados ANTES do loop de tiposVenda pelo mesmo
		// motivo do bloco Transmitido acima: no texto final eles aparecem
		// antes de "CONDVENDA IN (...)".
		transPH := bind("%TRANS%")
		cortPH := bind("%CORT%")
		var tvPH []string
		for _, tv := range tiposVenda {
			n, convErr := strconv.Atoi(tv)
			if convErr != nil {
				continue
			}
			tvPH = append(tvPH, bind(n))
		}
		condvendaCond := "1 = 0" // nenhum tipo válido selecionado: não conta nada, em vez de contar tudo por engano
		if len(tvPH) > 0 {
			condvendaCond = "CONDVENDA IN (" + strings.Join(tvPH, ", ") + ")"
		}
		somaNumerador = fmt.Sprintf(
			"SUM(CASE WHEN UPPER(ESTADO) NOT LIKE %s AND UPPER(ESTADO) NOT LIKE %s AND %s THEN PVENDA ELSE 0 END)",
			transPH, cortPH, condvendaCond)
		somaSubtrai = fmt.Sprintf("SUM(CASE WHEN UPPER(ESTADO) LIKE %s OR UPPER(ESTADO) LIKE %s THEN PVENDA ELSE 0 END)",
			bind("%CANCEL%"), bind("%DEVOL%"))
	}

	// 2) WHERE DATA >= .. AND DATA < .. — vem depois do SELECT no texto
	// final, então bind() só acontece agora.
	dataIniPH := bind(ini)
	dataFimPH := bind(fimExclusivo)

	// 3) Resto do WHERE — nesta ordem exata no texto final: filtroScan,
	// depois filialCond, depois escopoCond.
	if fluxo.name == "transmitido" {
		filtroScan = fmt.Sprintf("(UPPER(ESTADO) LIKE %s OR UPPER(ESTADO) LIKE %s)", bind("%TRANS%"), bind("%CORT%"))
	} else {
		filtroScan = fmt.Sprintf("UPPER(ESTADO) NOT LIKE %s AND UPPER(ESTADO) NOT LIKE %s", bind("%TRANS%"), bind("%CORT%"))
	}

	var filialCond string
	if len(filiais) > 0 {
		var ph []string
		for _, f := range filiais {
			ph = append(ph, bind(f))
		}
		filialCond = " AND EMPRESA IN (" + strings.Join(ph, ", ") + ")"
	}

	var escopoCond string
	if escopo.restrito() {
		var ph []string
		for _, v := range escopo.Vals {
			ph = append(ph, bind(v))
		}
		if len(ph) > 0 {
			escopoCond = fmt.Sprintf(" AND %s IN (%s)", colOracleRel322(escopo.Col), strings.Join(ph, ", "))
		} else {
			// escopo restrito sem nenhum valor liberado — mesma regra de
			// escopoCondRel322: falha fechado, nunca aberto.
			escopoCond = " AND 1 = 0"
		}
	}

	q := fmt.Sprintf(`
SELECT TRIM(CODSUPERVISOR),
       %s - %s
  FROM %s
 WHERE DATA >= %s AND DATA < %s
   AND %s%s%s
 GROUP BY TRIM(CODSUPERVISOR)`,
		somaNumerador, somaSubtrai, objeto, dataIniPH, dataFimPH, filtroScan, filialCond, escopoCond)

	return q, args
}

// vmLiquidoPorSupervisor — Líquido por cod_supervisor consultado AO VIVO na
// VM (base Oracle de origem, a mesma que o JC lê todo dia). Abre a conexão,
// monta a query (montarQueryVM) e escaneia o resultado — a construção da
// query em si é pura e testável separadamente.
func vmLiquidoPorSupervisor(ctx context.Context, empresaID string, dataInicio, dataFim time.Time, filiais, tiposVenda []string, escopo escopoRecorte, fluxo fluxoCtx) (map[string]float64, error) {
	_ = empresaID // a VM não tem conceito de empresa/tenant Farol — o recorte é só filial+escopo+período, igual ao Postgres

	if escopo.restrito() && escopo.Negar {
		return map[string]float64{}, nil
	}

	dsn, err := dsnJC()
	if err != nil {
		return nil, err
	}
	conn, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir conexão com a base de origem (VM): %w", err)
	}
	defer conn.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, 20*time.Second)
	defer cancelPing()
	if err := conn.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("conectar na base de origem (VM): %w", err)
	}

	objeto := envJC("JC_ORACLE_OBJETO", "IAUSER.COMPRAS_FAROL_VW")
	q, args := montarQueryVM(objeto, dataInicio, dataFim, filiais, tiposVenda, escopo, fluxo)

	queryCtx, cancelQuery := context.WithTimeout(ctx, 100*time.Second)
	defer cancelQuery()
	rows, err := conn.QueryContext(queryCtx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("consultar a base de origem (VM): %w", err)
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var cod string
		var liquido float64
		if err := rows.Scan(&cod, &liquido); err != nil {
			return nil, fmt.Errorf("ler linha da base de origem (VM): %w", err)
		}
		out[strings.TrimSpace(cod)] = liquido
	}
	return out, rows.Err()
}
