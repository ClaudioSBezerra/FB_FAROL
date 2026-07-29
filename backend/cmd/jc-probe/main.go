// jc-probe — sonda de conectividade com o Oracle do grupo JC.
//
// Valida, em ordem, as quatro coisas que podem estar erradas ANTES de existir
// qualquer extrator: alcance de rede/allowlist, service name, credencial, e
// permissão de leitura nos objetos. Cada falha tem diagnóstico próprio — sem
// isso, um ORA genérico manda a gente investigar o lado errado.
//
// NÃO contém credencial: tudo vem de env. Rodar:
//
//	JC_USER=IAUSER JC_PASS='...' go run -tags '' ./cmd/jc-probe
//
// Env aceitos (host/porta já têm o default decidido em 28/07/2026):
//
//	JC_HOST     default 201.48.119.197  (o de menor jitter dos dois liberados)
//	JC_PORT     default 1521
//	JC_SERVICE  se vazio, tenta os service names comuns do 23ai
//	JC_USER     obrigatório
//	JC_PASS     obrigatório
//	JC_SCHEMA   default IAUSER — owner cujos objetos serão listados
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/sijms/go-ora/v3"
)

// servicesComuns — o Keslley ainda não informou o service name. Estes são os
// defaults de instalação mais prováveis num 23ai; a sonda reporta qual pegou.
var servicesComuns = []string{"FREEPDB1", "FREE", "ORCLPDB1", "ORCL", "ORCLCDB", "XEPDB1", "XE"}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func dsn(host, port, service, user, pass string) string {
	u := &url.URL{
		Scheme: "oracle",
		User:   url.UserPassword(user, pass), // escapa senha com caractere especial
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + service,
	}
	return u.String()
}

// explicaErro traduz os ORA- que importam aqui. O valor da sonda está em dizer
// DE QUEM é o problema — nosso, do Keslley, ou de rede.
func explicaErro(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "ORA-01017"):
		return "credencial inválida (usuário ou senha). A senha foi trocada? Pedir a atual ao Keslley."
	case strings.Contains(s, "ORA-12514"), strings.Contains(s, "ORA-12505"):
		return "o listener não conhece esse service name — é o parâmetro a descobrir, não a credencial."
	case strings.Contains(s, "ORA-28040"), strings.Contains(s, "ORA-01005"):
		return "protocolo de autenticação recusado. É o risco que levantamos: SQLNET.ALLOWED_LOGON_VERSION_SERVER restritivo no 23ai. Ajuste no lado do Keslley, ou trocar go-ora v3 → godror."
	case strings.Contains(s, "ORA-01031"), strings.Contains(s, "ORA-00942"):
		return "conectou e autenticou, mas sem permissão/objeto. Falta GRANT do Keslley — a parte de rede está OK."
	case strings.Contains(s, "ORA-12541"):
		return "sem listener na porta. Serviço parado do lado deles."
	case strings.Contains(s, "i/o timeout"), strings.Contains(s, "connection refused"):
		return "não chegou no host. Allowlist não aplicada ao nosso IP, ou firewall. Conferir o IP de saída."
	}
	return "erro não mapeado — vale colar inteiro para análise."
}

func tentaConectar(host, port, service, user, pass string) (*sql.DB, error) {
	db, err := sql.Open("oracle", dsn(host, port, service, user, pass))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func main() {
	host := env("JC_HOST", "201.48.119.197")
	port := env("JC_PORT", "1521")
	service := env("JC_SERVICE", "")
	user := env("JC_USER", "")
	pass := env("JC_PASS", "")
	schema := strings.ToUpper(env("JC_SCHEMA", "IAUSER"))

	if user == "" || pass == "" {
		fmt.Println("ERRO: defina JC_USER e JC_PASS no ambiente (a sonda não guarda credencial).")
		os.Exit(2)
	}

	fmt.Printf("═══ sonda Oracle JC ═══\nhost=%s:%s schema=%s user=%s\n\n", host, port, schema, user)

	// ── 1. Conexão ──────────────────────────────────────────────────────────
	var db *sql.DB
	var usado string
	var ultimoErr error

	candidatos := servicesComuns
	if service != "" {
		candidatos = []string{service}
	} else {
		fmt.Println("JC_SERVICE não informado — testando os defaults comuns do 23ai:")
	}

	for _, s := range candidatos {
		t0 := time.Now()
		conn, err := tentaConectar(host, port, s, user, pass)
		if err != nil {
			if service == "" {
				fmt.Printf("  %-10s ✗ %s\n", s, primeiraLinha(err))
			}
			ultimoErr = err
			// Credencial ou protocolo errado não melhora trocando de service.
			e := err.Error()
			if strings.Contains(e, "ORA-01017") || strings.Contains(e, "ORA-28040") {
				break
			}
			continue
		}
		db, usado = conn, s
		fmt.Printf("  %-10s ✓ CONECTOU em %v\n", s, time.Since(t0).Round(time.Millisecond))
		break
	}

	if db == nil {
		fmt.Printf("\n✗ FALHOU\n  erro: %v\n  → %s\n", ultimoErr, explicaErro(ultimoErr))
		os.Exit(1)
	}
	defer db.Close()
	fmt.Printf("\n✓ service name = %s  (anotar — é o que faltava)\n\n", usado)

	// ── 2. Sessão viva ──────────────────────────────────────────────────────
	var um int
	if err := db.QueryRow("SELECT 1 FROM dual").Scan(&um); err != nil {
		fmt.Printf("✗ SELECT 1 FROM dual falhou: %v\n  → %s\n", err, explicaErro(err))
		os.Exit(1)
	}
	fmt.Println("✓ SELECT 1 FROM dual — sessão funcional")

	// ── 3. Versão (pode faltar permissão em v$version; não é bloqueante) ────
	var banner string
	if err := db.QueryRow("SELECT banner FROM v$version WHERE ROWNUM = 1").Scan(&banner); err != nil {
		fmt.Printf("· versão indisponível (sem GRANT em v$version, normal): %s\n", primeiraLinha(err))
	} else {
		fmt.Printf("✓ versão: %s\n", banner)
	}

	// ── 4. O que o Keslley já criou ─────────────────────────────────────────
	fmt.Printf("\n─── objetos visíveis no schema %s ───\n", schema)
	rows, err := db.Query(`
		SELECT object_type, COUNT(*)
		FROM all_objects WHERE owner = :1
		GROUP BY object_type ORDER BY object_type`, schema)
	if err != nil {
		fmt.Printf("✗ não listou: %v\n  → %s\n", err, explicaErro(err))
		os.Exit(1)
	}
	total := 0
	for rows.Next() {
		var tipo string
		var n int
		if rows.Scan(&tipo, &n) == nil {
			fmt.Printf("  %-20s %d\n", tipo, n)
			total += n
		}
	}
	rows.Close()

	if total == 0 {
		fmt.Println("  (nenhum) — Keslley ainda não criou views/sinônimos, ou faltam GRANTs.")
		fmt.Println("\n✓ Rede, service name e credencial OK. Falta só o lado dele.")
		return
	}

	// ── 5. Nomes — confere se as 4 views esperadas apareceram ───────────────
	fmt.Println("\n─── nomes ───")
	rows2, err := db.Query(`
		SELECT object_name, object_type
		FROM all_objects WHERE owner = :1
		  AND object_type IN ('VIEW','TABLE','SYNONYM','MATERIALIZED VIEW')
		ORDER BY object_type, object_name`, schema)
	if err == nil {
		for rows2.Next() {
			var nome, tipo string
			if rows2.Scan(&nome, &tipo) == nil {
				fmt.Printf("  %-14s %s\n", tipo, nome)
			}
		}
		rows2.Close()
	}
	fmt.Println("\nEsperado (reunião 16/07): FATURADAS, TRANSMITIDAS, CANCELADAS, DEVOLVIDAS.")
}

func primeiraLinha(err error) string {
	s := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(s, '\n'); i > 0 {
		s = s[:i]
	}
	if len(s) > 110 {
		s = s[:110] + "…"
	}
	return s
}
