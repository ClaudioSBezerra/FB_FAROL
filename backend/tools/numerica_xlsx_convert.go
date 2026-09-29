//go:build scripts

package main

// numerica_xlsx_convert.go — converte o arquivo XLSX que a JC manda todo
// mês ("Unico Acompanhamento numericas Unilever HC/foods_DDMMAAAA.xlsx",
// abas "BASE LOJAS" e "BASE PPA's") direto pro formato CSV que os
// endpoints de import da Numérica esperam (Épico 7 addendum, Story 7.2/7.3
// — ver farol_metas_clientes_numericas_csv.go / farol_metas_ppas_csv.go).
//
// Existe porque o arquivo real da JC não é o CSV que os handlers exigem —
// é a planilha de trabalho inteira (com abas de resumo/cálculo que não nos
// interessam). Em vez de pedir um formato novo pro Carlos todo mês, este
// conversor lê a MESMA estrutura de sempre (confirmada em 2026-09-29 contra
// os arquivos reais HC e Foods) e já produz os 2 CSVs prontos pra upload.
//
// Uso:
//
//	go run -tags scripts tools/numerica_xlsx_convert.go \
//	    -in "/home/claudio/uploads/Unico Acompanhamento numericas Unilever HC_29092026.xlsx" \
//	    -out-clientes /tmp/clientes_numericas_hc.csv \
//	    -out-ppas /tmp/ppas_hc.csv
//
// Os 2 CSVs de saída já estão no formato exato esperado por
// POST /api/farol/metas-clientes-numericas-importar-csv e
// POST /api/farol/metas-ppas-importar-csv (multipart, campo "file").
//
// ⚠ NÃO valida regra de negócio (CNPJ duplicado, Classificação fora do
// enum etc.) — isso já é feito pelos handlers de import na hora do upload.
// Este conversor só traduz formato de arquivo.

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

var soDigitos = regexp.MustCompile(`\D`)

func main() {
	in := flag.String("in", "", "caminho do XLSX da JC (obrigatório)")
	outClientes := flag.String("out-clientes", "", "caminho do CSV de Clientes Numéricas a gerar (obrigatório)")
	outPPAs := flag.String("out-ppas", "", "caminho do CSV de PPAs a gerar (obrigatório)")
	abaLojas := flag.String("aba-lojas", "BASE LOJAS", "nome da aba de Clientes (BASE LOJAS)")
	abaPPAs := flag.String("aba-ppas", "BASE PPA's", "nome da aba de PPAs (BASE PPA's)")
	flag.Parse()

	if *in == "" || *outClientes == "" || *outPPAs == "" {
		fmt.Fprintln(os.Stderr, "uso: go run -tags scripts tools/numerica_xlsx_convert.go -in ARQUIVO.xlsx -out-clientes clientes.csv -out-ppas ppas.csv")
		os.Exit(1)
	}

	f, err := excelize.OpenFile(*in)
	if err != nil {
		log.Fatalf("abrir %s: %v", *in, err)
	}
	defer f.Close()

	nClientes, err := converterClientesNumericas(f, *abaLojas, *outClientes)
	if err != nil {
		log.Fatalf("converter %s: %v", *abaLojas, err)
	}
	nPPAs, err := converterPPAs(f, *abaPPAs, *outPPAs)
	if err != nil {
		log.Fatalf("converter %s: %v", *abaPPAs, err)
	}

	log.Printf("OK: %d clientes → %s | %d linhas de PPA → %s", nClientes, *outClientes, nPPAs, *outPPAs)
}

// converterClientesNumericas lê a aba BASE LOJAS (CNPJ, CODCL,
// Classificação PDV, RAZAO, FANTASIA, GGV COD, GGV NOME, CRV COD, CRV NOME,
// RCA COD, RCA NOME — confirmado nos 2 arquivos reais, HC e Foods,
// 2026-09-29) e escreve o CSV no formato de
// farol_metas_clientes_numericas_csv.go.
func converterClientesNumericas(f *excelize.File, aba, caminhoSaida string) (int, error) {
	rows, err := f.GetRows(aba)
	if err != nil {
		return 0, fmt.Errorf("ler aba %q: %w", aba, err)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("aba %q vazia", aba)
	}
	idx, err := indiceColunas(rows[0], map[string][]string{
		"cnpj":          {"CNPJ"},
		"cod_cl":        {"CODCL", "COD CL"},
		"classificacao": {"Classificação PDV", "CLASSIFICAÇÃO PDV"},
		"razao":         {"RAZAO", "RAZÃO"},
		"fantasia":      {"FANTASIA"},
		"cod_ggv":       {"GGV COD"},
		"nome_ggv":      {"GGV NOME"},
		"cod_crv":       {"CRV COD"},
		"nome_crv":      {"CRV NOME"},
		"cod_rca":       {"RCA COD"},
		"nome_rca":      {"RCA NOME"},
	})
	if err != nil {
		return 0, err
	}

	out, err := os.Create(caminhoSaida)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	w.Comma = ';'
	defer w.Flush()

	if err := w.Write([]string{"cnpj", "cod_cl", "classificacao_pdv", "razao", "fantasia", "cod_ggv", "nome_ggv", "cod_crv", "nome_crv", "cod_rca", "nome_rca"}); err != nil {
		return 0, err
	}

	n := 0
	for _, r := range rows[1:] {
		cnpj := cnpj14(get(r, idx["cnpj"]))
		if cnpj == "" {
			continue // linha vazia/rodapé
		}
		if err := w.Write([]string{
			cnpj, get(r, idx["cod_cl"]), get(r, idx["classificacao"]), get(r, idx["razao"]), get(r, idx["fantasia"]),
			get(r, idx["cod_ggv"]), get(r, idx["nome_ggv"]), get(r, idx["cod_crv"]), get(r, idx["nome_crv"]),
			get(r, idx["cod_rca"]), get(r, idx["nome_rca"]),
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// converterPPAs lê a aba BASE PPA's (Região do Sortimento, AE, BU, PPA,
// Descrição, EAN Regular, Cod JC, Embalagem) e escreve o CSV no formato de
// farol_metas_ppas_csv.go. "Cod JC" é o cod_prod (código interno) —
// confirmado contra o cadastro de produto do Farol.
func converterPPAs(f *excelize.File, aba, caminhoSaida string) (int, error) {
	rows, err := f.GetRows(aba)
	if err != nil {
		return 0, fmt.Errorf("ler aba %q: %w", aba, err)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("aba %q vazia", aba)
	}
	idx, err := indiceColunas(rows[0], map[string][]string{
		"regiao":    {"Região do Sortimento"},
		"ae":        {"AE"},
		"bu":        {"BU"},
		"ppa":       {"PPA", "PPA / FAMILIA", "PPA/FAMILIA"},
		"ean":       {"EAN Regular", "EAN"},
		"cod_prod":  {"Cod JC", "COD JC"},
		"embalagem": {"Embalagem"},
	})
	if err != nil {
		return 0, err
	}

	out, err := os.Create(caminhoSaida)
	if err != nil {
		return 0, err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	w.Comma = ';'
	defer w.Flush()

	if err := w.Write([]string{"cod_prod", "ppa_nome", "ean", "embalagem", "regiao", "ae", "bu"}); err != nil {
		return 0, err
	}

	n, semCodigo := 0, 0
	for _, r := range rows[1:] {
		codProd := strings.TrimSpace(get(r, idx["cod_prod"]))
		ppa := strings.TrimSpace(get(r, idx["ppa"]))
		if ppa == "" {
			continue
		}
		// "0" é o placeholder da JC pra "produto ainda não mapeado pro
		// código interno do Farol" (achado real 2026-09-29: 32 das 162
		// linhas do arquivo HC vêm assim) — puxar isso pro import quebraria
		// a UNIQUE(vigencia_id, cod_prod) na 2ª linha "0" (rejeitaria o
		// lote inteiro, import é atômico). Pula com aviso; o PPA "família"
		// continua existindo com os outros cod_prod mapeados dela.
		if codProd == "" || codProd == "0" {
			semCodigo++
			continue
		}
		if err := w.Write([]string{
			codProd, ppa, get(r, idx["ean"]), get(r, idx["embalagem"]),
			get(r, idx["regiao"]), get(r, idx["ae"]), get(r, idx["bu"]),
		}); err != nil {
			return n, err
		}
		n++
	}
	if semCodigo > 0 {
		log.Printf("aviso: %d linha(s) de PPA sem cod_prod (vazio ou \"0\", produto ainda não mapeado pela JC) ignorada(s)", semCodigo)
	}
	return n, nil
}

// indiceColunas resolve, pro cabeçalho real da planilha, o índice de cada
// coluna lógica — aceita mais de um nome possível por coluna (a JC já
// mudou "COD CL" pra "CODCL" entre um arquivo e outro, por exemplo).
func indiceColunas(header []string, candidatos map[string][]string) (map[string]int, error) {
	porNome := map[string]int{}
	for i, h := range header {
		porNome[strings.ToLower(strings.TrimSpace(h))] = i
	}
	out := map[string]int{}
	var faltando []string
	for chave, nomes := range candidatos {
		achou := false
		for _, nome := range nomes {
			if i, ok := porNome[strings.ToLower(nome)]; ok {
				out[chave] = i
				achou = true
				break
			}
		}
		if !achou {
			out[chave] = -1
			// cod_cl e as colunas de PPA (regiao/ae/bu/ean/embalagem) são
			// informativas — não travam a conversão se a planilha não
			// trouxer (ver comentário da migration 248).
			if chave != "cod_cl" && chave != "regiao" && chave != "ae" && chave != "bu" && chave != "ean" && chave != "embalagem" {
				faltando = append(faltando, chave)
			}
		}
	}
	if len(faltando) > 0 {
		return nil, fmt.Errorf("coluna(s) obrigatória(s) não encontrada(s) no cabeçalho: %v (cabeçalho real: %v)", faltando, header)
	}
	return out, nil
}

func get(row []string, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// cnpj14 normaliza o CNPJ (a planilha traz como número, sem zero à
// esquerda — ex.: 1697060000178 tem 13 dígitos, faltando o zero inicial)
// pro formato de 14 dígitos que os handlers de import exigem.
func cnpj14(v string) string {
	digitos := soDigitos.ReplaceAllString(v, "")
	if digitos == "" {
		return ""
	}
	for len(digitos) < 14 {
		digitos = "0" + digitos
	}
	return digitos
}
