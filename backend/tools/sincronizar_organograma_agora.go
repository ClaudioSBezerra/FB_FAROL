//go:build scripts

package main

// sincronizar_organograma_agora.go — dispara handlers.SincronizarOrganogramaJC
// (o mesmo sync diário de jc_organograma.go) manualmente, sem esperar o
// horário agendado (06:00) nem passar pela rota HTTP autenticada — chama a
// função exportada direto, usando as env vars que já estão no container da
// API (DATABASE_URL, JC_ORACLE_*, JC_EMPRESA_ID).
//
// Depois confere especificamente os códigos de GGV 3, 346 e 350 — o caso
// real de 20/08/2026 (Gilson Flores / Jocildo Guimarães / Divair Pires) —
// pra fechar com confirmação na hora, sem precisar de uma segunda consulta.
//
// Uso: docker exec <container_api> /tmp/sincronizar_organograma_agora
import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"fb_farol/handlers"

	_ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL não configurado no ambiente")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("abrir Postgres: %v", err)
	}
	defer db.Close()

	fmt.Println("Sincronizando organograma (CADRCA_JC)...")
	if err := handlers.SincronizarOrganogramaJC(db); err != nil {
		log.Fatalf("ERRO na sincronização: %v", err)
	}
	fmt.Println("✓ sincronização concluída")
	fmt.Println()

	fmt.Println("=== Verificação dos códigos do caso 20/08/2026 (Gilson/Jocildo/Divair) ===")
	rows, err := db.Query(`
		SELECT empresa_id::text, codigo, nome, atualizado_em, sincronizado_em
		  FROM farol.cadastro_organograma_jc
		 WHERE nivel = 'gerente' AND codigo IN ('3', '346', '350')
		 ORDER BY codigo
	`)
	if err != nil {
		log.Fatalf("consultar cadastro_organograma_jc: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var empresaID, codigo, nome string
		var atualizadoEm, sincronizadoEm sql.NullTime
		if err := rows.Scan(&empresaID, &codigo, &nome, &atualizadoEm, &sincronizadoEm); err != nil {
			fmt.Printf("  (erro no scan: %v)\n", err)
			continue
		}
		fmt.Printf("  código %-4s → %-30s (atualizado no WinThor em %v)\n", codigo, nome, atualizadoEm.Time)
		n++
	}
	if n == 0 {
		fmt.Println("  (nenhum encontrado — confira se JC_EMPRESA_ID bate com a empresa certa)")
	}
	fmt.Println()
	fmt.Println("Fim.")
}
