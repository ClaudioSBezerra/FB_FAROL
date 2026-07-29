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
	"os"
	"strconv"
	"strings"
	"time"
)

// candidatos — o Keslley ainda não informou o nome. Ordem: defaults do 23ai,
// depois nomes clássicos, depois os plausíveis para este cliente. WINT/WINTHOR
// entram porque a JC roda WinThor/PC Sistemas, cujas instalações Oracle
// tradicionalmente usam SID (não service name) — daí a sonda testar os dois modos.
var candidatos = []string{
	"FREEPDB1", "FREE", // Oracle 23ai Free
	"ORCLPDB1", "ORCL", "ORCLCDB", "ORCLPDB", "PDB1", "PDB", // instalação padrão
	"XEPDB1", "XE", // Express
	"WINT", "WINTHOR", "WINT1", // WinThor / PC Sistemas
	"JC", "JCDB", "IA", "IADB", "PROD", "FAROL", // específicos do cliente
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

// modo de resolução do banco no connect descriptor.
type modo int

const (
	porServico modo = iota // (SERVICE_NAME=...)
	porSID                 // (SID=...) — o que instalações WinThor costumam usar
)

func (m modo) String() string {
	if m == porSID {
		return "SID"
	}
	return "SERVICE"
}

func dsn(host, port, nome, user, pass string, m modo) string {
	p, _ := strconv.Atoi(port)
	if m == porSID {
		// serviço vazio + SID nas opções → go-ora monta (SID=...) no descriptor
		return buildURL(host, p, "", user, pass, map[string]string{"SID": nome})
	}
	return buildURL(host, p, nome, user, pass, nil)
}

// nomeDesconhecido — o listener respondeu que não conhece este serviço/SID.
// É o ÚNICO erro que justifica tentar o próximo candidato: o Oracle só valida
// credencial DEPOIS de resolver o destino, então qualquer outro erro (inclusive
// senha errada) significa que o nome está CERTO.
func nomeDesconhecido(err error) bool {
	s := err.Error()
	return strings.Contains(s, "ORA-12514") || strings.Contains(s, "ORA-12505")
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
	case strings.Contains(s, "ORA-01033"), strings.Contains(s, "ORA-12528"):
		return "o banco existe mas NÃO aceita conexão ainda — PDB apenas montado (não OPEN) ou em modo restrito. É um `ALTER PLUGGABLE DATABASE ... OPEN;` do lado do Keslley."
	case strings.Contains(s, "expected code is 1"):
		return "falhou na NEGOCIAÇÃO DE PROTOCOLO, antes da autenticação — logo não é credencial. " +
			"O servidor respondeu código 4 (mensagem de erro TTC) onde o driver esperava a negociação. " +
			"Duas causas prováveis: (a) o PDB está montado mas não OPEN, e o erro real seria ORA-01033; " +
			"(b) incompatibilidade do go-ora v3 com este 23ai. Para separar as duas, rodar o binário " +
			"jc-probe-v2 (mesma sonda com go-ora v2.9.0): se ele devolver um ORA- legível, é (a) e a " +
			"mensagem diz o que pedir ao Keslley; se repetir o erro cru, é (b) e trocamos de driver."
	case strings.Contains(s, "i/o timeout"), strings.Contains(s, "connection refused"):
		return "não chegou no host. Allowlist não aplicada ao nosso IP, ou firewall. Conferir o IP de saída."
	}
	return "erro não mapeado — vale colar inteiro para análise."
}

func tentaConectar(host, port, nome, user, pass string, m modo) (*sql.DB, error) {
	db, err := sql.Open("oracle", dsn(host, port, nome, user, pass, m))
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
	// cdb1 confirmado em 29/07/2026 pelo alias ORA_POWERBI que o Keslley enviou
	// (o mesmo que o Power BI deles já usa). A varredura tinha achado PDB1, que o
	// listener aceita mas onde o IAUSER não existe — daí o ORA-01017.
	service := env("JC_SERVICE", "cdb1")
	user := env("JC_USER", "")
	pass := env("JC_PASS", "")
	schema := strings.ToUpper(env("JC_SCHEMA", "IAUSER"))

	if user == "" || pass == "" {
		fmt.Println("ERRO: defina JC_USER e JC_PASS no ambiente (a sonda não guarda credencial).")
		os.Exit(2)
	}

	fmt.Printf("═══ sonda Oracle JC ═══\nhost=%s:%s schema=%s user=%s\ndriver=%s\n\n",
		host, port, schema, user, driverVersao)

	// ── 1. Descobrir o nome do banco ────────────────────────────────────────
	//
	// O Oracle resolve o destino ANTES de checar credencial. Logo, só
	// ORA-12514/12505 ("não conheço esse nome") justifica tentar o próximo —
	// qualquer outro erro prova que o nome está certo e o problema é outro.
	var db *sql.DB
	var usado string
	var usadoModo modo
	var ultimoErr error

	lista := candidatos
	modos := []modo{porServico, porSID}
	if service != "" {
		lista = []string{service}
		fmt.Printf("JC_SERVICE=%s informado — testando só ele.\n\n", service)
	} else {
		fmt.Printf("JC_SERVICE não informado — varrendo %d nomes × 2 modos.\n", len(lista))
		fmt.Print("(o listener responde na hora; erro ≠ 12514/12505 já indica acerto)\n\n")
	}

varredura:
	for _, m := range modos {
		for _, nome := range lista {
			t0 := time.Now()
			conn, err := tentaConectar(host, port, nome, user, pass, m)
			if err == nil {
				db, usado, usadoModo = conn, nome, m
				fmt.Printf("  %-8s %-10s ✓ CONECTOU em %v\n", m, nome, time.Since(t0).Round(time.Millisecond))
				break varredura
			}
			ultimoErr = err
			if nomeDesconhecido(err) {
				continue // nome errado — único caso que vale seguir
			}
			// Nome CERTO, outro problema (credencial, protocolo, permissão).
			fmt.Printf("  %-8s %-10s ◆ NOME ACEITO — parou aqui\n", m, nome)
			usado, usadoModo = nome, m
			break varredura
		}
		if service == "" {
			fmt.Printf("  — nenhum nome respondeu no modo %s\n", m)
		}
	}

	if db == nil {
		fmt.Printf("\n✗ NÃO CONECTOU\n  último erro: %v\n  → %s\n", ultimoErr, explicaErro(ultimoErr))
		if usado != "" {
			fmt.Printf("\n  ATENÇÃO: '%s' (modo %s) foi ACEITO pelo listener — o nome do banco\n", usado, usadoModo)
			fmt.Println("  está resolvido; o que falta é o item acima.")
		} else {
			fmt.Println("\n  Nenhum candidato foi reconhecido. É pergunta direta ao Keslley:")
			fmt.Println("  \"qual o SERVICE_NAME (ou SID) da instância?\" — no servidor dele,")
			fmt.Println("  `lsnrctl services` lista em uma linha.")
		}
		os.Exit(1)
	}
	defer db.Close()
	fmt.Printf("\n✓ %s = %s  (anotar — era o parâmetro que faltava)\n\n", usadoModo, usado)

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

	// ── 4. O que já existe ──────────────────────────────────────────────────
	//
	// Duas perguntas DIFERENTES, e olhar só a primeira já nos enganou uma vez:
	//   (a) o que IAUSER é DONO  → all_objects WHERE owner = IAUSER
	//   (b) o que IAUSER ALCANÇA → all_objects (só mostra o acessível) em
	//       schemas não-Oracle
	// O caminho provável é o Keslley criar as views em outro schema e conceder
	// SELECT, ou criar sinônimos. Nesse caso (a) fica vazio e (b) mostra tudo.
	fmt.Printf("\n─── (a) objetos de que %s é DONO ───\n", schema)
	total := contaPorTipo(db, `
		SELECT object_type, COUNT(*)
		FROM all_objects WHERE owner = :1
		GROUP BY object_type ORDER BY object_type`, schema)
	if total == 0 {
		fmt.Println("  (nenhum)")
	}

	fmt.Println("\n─── (b) objetos ACESSÍVEIS em schemas de aplicação ───")
	fmt.Println("    (exclui schemas internos do Oracle via oracle_maintained)")
	acessiveis := contaPorTipo(db, `
		SELECT o.owner || '.' || o.object_type, COUNT(*)
		FROM all_objects o
		JOIN all_users u ON u.username = o.owner
		WHERE u.oracle_maintained = 'N'
		  AND o.object_type IN ('VIEW','TABLE','SYNONYM','MATERIALIZED VIEW')
		GROUP BY o.owner || '.' || o.object_type
		ORDER BY 1`)
	if acessiveis == 0 {
		fmt.Println("  (nenhum)")
	}

	if total == 0 && acessiveis == 0 {
		fmt.Println("\n✓ Rede, service name, credencial e driver OK.")
		fmt.Println("  Falta o Keslley criar as views/sinônimos e conceder SELECT.")
		return
	}

	// ── 5. Nomes — confere se as 4 views esperadas apareceram ───────────────
	//
	// Qualifica com o owner: as views podem não estar em IAUSER, e o SELECT do
	// passo 6 precisa do nome completo para funcionar.
	fmt.Println("\n─── nomes (owner.objeto) ───")
	var legiveis []string
	rows2, err := db.Query(`
		SELECT o.owner, o.object_name, o.object_type
		FROM all_objects o
		JOIN all_users u ON u.username = o.owner
		WHERE u.oracle_maintained = 'N'
		  AND o.object_type IN ('VIEW','TABLE','SYNONYM','MATERIALIZED VIEW')
		ORDER BY o.owner, o.object_type, o.object_name`)
	if err == nil {
		for rows2.Next() {
			var owner, nome, tipo string
			if rows2.Scan(&owner, &nome, &tipo) == nil {
				fmt.Printf("  %-14s %s.%s\n", tipo, owner, nome)
				legiveis = append(legiveis, owner+"."+nome)
			}
		}
		rows2.Close()
	}
	fmt.Println("\nEsperado (reunião 16/07): FATURADAS, TRANSMITIDAS, CANCELADAS, DEVOLVIDAS.")

	// ── 6. SELECT de verdade — listar não prova leitura ─────────────────────
	//
	// all_objects mostra o que EXISTE; o GRANT de SELECT é outra coisa. Só um
	// SELECT real prova que a extração vai funcionar. Fazemos ROWNUM<=1 (barato
	// em qualquer tamanho de tabela) para colher as colunas, e COUNT(*) com
	// timeout curto — se estourar, a tabela é grande, o que é informação útil
	// para dimensionar a janela de carga, não um erro.
	if len(legiveis) == 0 {
		return
	}
	fmt.Printf("\n─── SELECT real (%d objetos) ───\n", len(legiveis))
	for _, alvo := range legiveis {
		if !qualificadoSeguro(alvo) {
			fmt.Printf("  %-30s · nome fora do padrão, pulado\n", alvo)
			continue
		}
		nome := alvo

		cols, err := colunasDe(db, alvo)
		if err != nil {
			fmt.Printf("  %-24s ✗ %s\n", nome, primeiraLinha(err))
			if strings.Contains(err.Error(), "ORA-01031") || strings.Contains(err.Error(), "ORA-00942") {
				fmt.Println("       └ existe mas sem GRANT de SELECT — pedir ao Keslley")
			}
			continue
		}
		fmt.Printf("  %-24s ✓ %d colunas\n", nome, len(cols))
		fmt.Printf("       colunas: %s\n", resumoColunas(cols))

		if n, dur, err := contaLinhas(db, alvo); err != nil {
			fmt.Printf("       linhas: (COUNT não concluiu em 30s — tabela grande)\n")
		} else {
			fmt.Printf("       linhas: %d  (COUNT em %v)\n", n, dur.Round(time.Millisecond))
		}
	}
}

// contaPorTipo roda uma query de duas colunas (rótulo, contagem) e imprime.
// Devolve o total para o chamador decidir se vale seguir.
func contaPorTipo(db *sql.DB, query string, args ...any) int {
	rows, err := db.Query(query, args...)
	if err != nil {
		fmt.Printf("  ✗ não listou: %s\n", primeiraLinha(err))
		return 0
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var rotulo string
		var n int
		if rows.Scan(&rotulo, &n) == nil {
			fmt.Printf("  %-34s %d\n", rotulo, n)
			total += n
		}
	}
	return total
}

// qualificadoSeguro valida "OWNER.OBJETO" antes de interpolar em SQL (bind não
// vale para identificador).
func qualificadoSeguro(s string) bool {
	partes := strings.Split(s, ".")
	if len(partes) != 2 {
		return false
	}
	return identificadorSeguro(partes[0]) && identificadorSeguro(partes[1])
}

// identificadorSeguro — os nomes vêm do próprio dicionário do Oracle, mas eles
// entram em SQL por concatenação (bind não vale para identificador), então
// validamos antes de interpolar.
func identificadorSeguro(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '$' || r == '#'
		if !ok {
			return false
		}
	}
	return true
}

func colunasDe(db *sql.DB, alvo string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, "SELECT * FROM "+alvo+" WHERE ROWNUM <= 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}

func contaLinhas(db *sql.DB, alvo string) (int64, time.Duration, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t0 := time.Now()
	var n int64
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+alvo).Scan(&n)
	return n, time.Since(t0), err
}

func resumoColunas(cols []string) string {
	const max = 12
	if len(cols) <= max {
		return strings.Join(cols, ", ")
	}
	return strings.Join(cols[:max], ", ") + fmt.Sprintf(", … (+%d)", len(cols)-max)
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
