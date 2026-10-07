//go:build scripts

package main

// explorar_cad_jc.go — ferramenta de exploração MANUAL (leitura, nada escreve),
// pra achar CADRCA_JC e CADLOG_JC que o Keslley (TI da JC) liberou em
// 21/09/2026 e hoje (07/10/2026) no Oracle deles. Não sabemos o nome EXATO
// do objeto (tabela? view? sinônimo, como IAUSER.COMPRAS_FAROL_VW que é
// sinônimo pra IAADMIN.*?) nem as colunas — essa ferramenta descobre os
// dois.
//
// Não reusa dsnJC()/colunasJC de jc_extrator.go de propósito: são função e
// variável privadas do package handlers, e esse arquivo é package main
// separado (mesma convenção dos outros tools/*.go com build tag "scripts").
// Duplica as ~10 linhas de conexão em vez de expor algo só pra isso.
//
// Uso (dentro do container da API em produção, que já tem as env vars
// JC_ORACLE_* injetadas pelo Coolify e a rota de rede liberada pro Oracle
// da JC):
//
//	go run -tags scripts tools/explorar_cad_jc.go
//
// Se o container de produção não tiver o toolchain Go (imagem final é
// alpine "FROM scratch"-like, só o binário), compile fora e copie:
//
//	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags scripts \
//	    -o explorar_cad_jc tools/explorar_cad_jc.go
//	scp explorar_cad_jc root@<host>:/tmp/
//	ssh root@<host> 'docker cp /tmp/explorar_cad_jc <container_api>:/tmp/explorar_cad_jc'
//	ssh root@<host> 'docker exec <container_api> /tmp/explorar_cad_jc'
import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
)

func envJC(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func dsnJC() (string, error) {
	user := envJC("JC_ORACLE_USER", "")
	pass := envJC("JC_ORACLE_PASS", "")
	if user == "" || pass == "" {
		return "", fmt.Errorf("JC_ORACLE_USER/JC_ORACLE_PASS não configurados no ambiente")
	}
	porta, err := strconv.Atoi(envJC("JC_ORACLE_PORT", "1521"))
	if err != nil {
		porta = 1521
	}
	opts := map[string]string{"PREFETCH_ROWS": "500"}
	return go_ora.BuildUrl(
		envJC("JC_ORACLE_HOST", "201.48.119.197"), porta,
		envJC("JC_ORACLE_SERVICE", "cdb1"),
		user, pass, opts), nil
}

// candidato — um objeto achado em ALL_TABLES/ALL_VIEWS/ALL_SYNONYMS cujo
// nome contém "CAD".
type candidato struct {
	owner, nome, tipo       string
	resolveOwner, resolveNm string // pra sinônimo: a tabela/view real por trás
}

func main() {
	dsn, err := dsnJC()
	if err != nil {
		log.Fatalf("ERRO: %v", err)
	}
	conn, err := sql.Open("oracle", dsn)
	if err != nil {
		log.Fatalf("ERRO ao abrir conexão: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		log.Fatalf("ERRO ao conectar no Oracle da JC: %v", err)
	}
	fmt.Println("✓ conectado no Oracle da JC")
	fmt.Println()

	// 1) Procura qualquer tabela/view/sinônimo visível pro nosso usuário
	// cujo nome contenha "CAD" — cobre CADRCA_JC, CADLOG_JC e qualquer
	// variação de grafia (CAD_RCA_JC, VW_CADLOG_JC etc.), e de brinde lista
	// outros CAD* que possam interessar depois (Clientes/Fornecedores/Produtos,
	// conforme a mensagem do Keslley).
	fmt.Println("=== Objetos com \"CAD\" no nome, visíveis ao nosso usuário ===")
	rows, err := conn.QueryContext(ctx, `
		SELECT owner, table_name AS nome, 'TABLE' AS tipo, '' AS res_owner, '' AS res_nome
		  FROM all_tables WHERE UPPER(table_name) LIKE '%CAD%'
		UNION ALL
		SELECT owner, view_name, 'VIEW', '', ''
		  FROM all_views WHERE UPPER(view_name) LIKE '%CAD%'
		UNION ALL
		SELECT owner, synonym_name, 'SYNONYM', table_owner, table_name
		  FROM all_synonyms WHERE UPPER(synonym_name) LIKE '%CAD%'
		ORDER BY 2, 1
	`)
	if err != nil {
		log.Fatalf("ERRO ao consultar ALL_TABLES/ALL_VIEWS/ALL_SYNONYMS: %v", err)
	}
	var candidatos []candidato
	for rows.Next() {
		var c candidato
		if err := rows.Scan(&c.owner, &c.nome, &c.tipo, &c.resolveOwner, &c.resolveNm); err != nil {
			log.Printf("  (erro no scan: %v)", err)
			continue
		}
		candidatos = append(candidatos, c)
		if c.tipo == "SYNONYM" {
			fmt.Printf("  %-10s %s.%s  → %s.%s\n", c.tipo, c.owner, c.nome, c.resolveOwner, c.resolveNm)
		} else {
			fmt.Printf("  %-10s %s.%s\n", c.tipo, c.owner, c.nome)
		}
	}
	rows.Close()
	if len(candidatos) == 0 {
		fmt.Println("  (nenhum — talvez o grant ainda não esteja liberado pro usuário JC_ORACLE_USER atual, ou o nome não contém \"CAD\")")
	}
	fmt.Println()

	// 2) Pros que claramente são CADRCA ou CADLOG (ignorando case/underscore),
	// detalha colunas + 5 linhas de amostra.
	for _, c := range candidatos {
		norm := strings.ToUpper(strings.ReplaceAll(c.nome, "_", ""))
		ehAlvo := strings.Contains(norm, "CADRCA") || strings.Contains(norm, "CADLOG")
		if !ehAlvo {
			continue
		}
		owner, nome := c.owner, c.nome
		if c.tipo == "SYNONYM" {
			owner, nome = c.resolveOwner, c.resolveNm
		}
		fmt.Printf("=== %s.%s (%s) ===\n", owner, nome, c.tipo)
		descreverColunas(ctx, conn, owner, nome)
		amostrar(ctx, conn, owner, nome)
		fmt.Println()
	}

	fmt.Println("Fim.")
}

func descreverColunas(ctx context.Context, conn *sql.DB, owner, nome string) {
	rows, err := conn.QueryContext(ctx, `
		SELECT column_name, data_type, data_length, nullable
		  FROM all_tab_columns
		 WHERE owner = :1 AND table_name = :2
		 ORDER BY column_id
	`, owner, nome)
	if err != nil {
		fmt.Printf("  (erro ao descrever colunas: %v)\n", err)
		return
	}
	defer rows.Close()
	fmt.Println("  Colunas:")
	n := 0
	for rows.Next() {
		var col, tipo, nullable string
		var tam int
		if err := rows.Scan(&col, &tipo, &tam, &nullable); err != nil {
			continue
		}
		obrigatorio := ""
		if nullable == "N" {
			obrigatorio = " NOT NULL"
		}
		fmt.Printf("    - %-30s %s(%d)%s\n", col, tipo, tam, obrigatorio)
		n++
	}
	if n == 0 {
		fmt.Println("    (nenhuma coluna encontrada — confirme owner/nome)")
	}
}

func amostrar(ctx context.Context, conn *sql.DB, owner, nome string) {
	q := fmt.Sprintf(`SELECT * FROM %s.%s WHERE ROWNUM <= 5`, owner, nome)
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		fmt.Printf("  (erro ao amostrar linhas: %v)\n", err)
		return
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		fmt.Printf("  (erro ao ler nomes de coluna: %v)\n", err)
		return
	}
	fmt.Printf("  Amostra (até 5 linhas), colunas: %s\n", strings.Join(cols, " | "))
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	n := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			fmt.Printf("    (erro no scan da linha: %v)\n", err)
			continue
		}
		partes := make([]string, len(cols))
		for i, v := range vals {
			partes[i] = fmt.Sprintf("%v", v)
		}
		fmt.Printf("    %s\n", strings.Join(partes, " | "))
		n++
	}
	if n == 0 {
		fmt.Println("    (tabela/view vazia)")
	}
}
