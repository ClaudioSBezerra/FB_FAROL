//go:build scripts

package main

// numerica_smoke_test.go — roda o motor de apuração real (handlers.CalcularRealizado)
// contra os 4 vínculos da Numérica, pra conferir o cálculo depois de um
// import (CSV via numerica_xlsx_convert.go) sem precisar passar pela UI.
// NÃO é um go test (build tag scripts, fora do `go test ./...` normal) —
// é uma ferramenta de conferência manual.
//
// Uso: DATABASE_URL=... go run -tags scripts tools/numerica_smoke_test.go

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
		dsn = "postgres://postgres:postgres@localhost:5432/fb_farol?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT mv.id, v.id, i.nome, tm.nome, mv.empresa_id::text
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.industrias i ON i.id = mv.industria_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE tm.formula_codigo LIKE '%numerica%'
		ORDER BY i.nome, tm.nome
	`)
	if err != nil {
		log.Fatal(err)
	}
	type alvo struct {
		vinculoID, vigenciaID      int
		industria, tipo, empresaID string
	}
	var alvos []alvo
	for rows.Next() {
		var a alvo
		if err := rows.Scan(&a.vinculoID, &a.vigenciaID, &a.industria, &a.tipo, &a.empresaID); err != nil {
			log.Fatal(err)
		}
		alvos = append(alvos, a)
	}
	rows.Close()

	for _, a := range alvos {
		resultado, err := handlers.CalcularRealizado(db, a.empresaID, a.vinculoID, a.vigenciaID, "faturado", "ggv")
		if err != nil {
			fmt.Printf("%-14s %-28s ERRO: %v\n", a.industria, a.tipo, err)
			continue
		}
		fmt.Printf("%-14s %-28s clientes=%-6d realizado_total=%.2f (%d grupos GGV)\n",
			a.industria, a.tipo, len(resultado.Redes), resultado.RealizadoTotal, len(resultado.Grupos))
	}
}
