// jc_organograma.go — sincronização diária de Gerente(GGV)/Supervisor(CRV)/RCA
// a partir do CADRCA_JC (Oracle da JC, mesma conexão read-only de
// jc_extrator.go), liberado pelo Keslley/TI em 21/09/2026.
//
// PROBLEMA REAL (Claudio, 07/10/2026): um código de Supervisor pertencia a
// uma pessoa em 2025; a equipe mudou, o código foi reaproveitado para outra
// pessoa, mas o Farol continuou mostrando o nome antigo em alguns lugares.
// Causa raiz: até aqui não existia cadastro autoritativo — o nome era
// efeito colateral do CSV de venda do dia (farol_v2_api.go, lookupNome/
// fetchDim leem de agg_*_dims_mes) ou de um cadastro manual nunca
// re-sincronizado (gestores/rcas, cadastros.go). Se o código não tem venda
// recente, ou se ninguém re-sobe o CSV manual, o nome nunca atualiza.
//
// Este arquivo lê o CADRCA_JC inteiro (pequeno — organograma da empresa
// toda, não venda) todo dia e grava em DOIS lugares:
//  1. farol.cadastro_organograma_jc (migration 250) — nova fonte primária
//     que lookupNome/fetchDim passam a consultar primeiro.
//  2. gestores/rcas (migration 121/122) — mesmas tabelas que já alimentam
//     os deep links do ION VENDAS (/m/CNPJ/SUP/cod, /m/CNPJ/RCA/cod,
//     farol_mobile.go/farol_web.go), só o campo nome — uf/regiao/ativo
//     continuam curados manualmente.
//
// CADLOG_JC (log de alterações, liberado hoje 07/10/2026) não é usado aqui
// — serviria para corrigir retroativamente o nome de um mês já fechado em
// agg_*_dims_mes, mas isso é um reforço futuro, não o problema atual.
package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// registroOrganograma — uma linha do CADRCA_JC (grão = 1 por CODUSUR/RCA,
// com GERENTE/SUPERVISOR denormalizados na mesma linha).
type registroOrganograma struct {
	codGerente    string
	gerente       string
	codSupervisor string
	supervisor    string
	codUsur       string
	rca           string
	dtReg         time.Time
}

// maisRecente — nome + data do registro mais recente visto pra um código,
// dentre possivelmente várias linhas (um gerente/supervisor aparece em uma
// linha por RCA sob ele).
type maisRecente struct {
	nome  string
	dtReg time.Time
}

func atualizaSeMaisRecente(m map[string]maisRecente, cod, nome string, dt time.Time) {
	if cod == "" || nome == "" {
		return
	}
	atual, ok := m[cod]
	if !ok || dt.After(atual.dtReg) {
		m[cod] = maisRecente{nome: nome, dtReg: dt}
	}
}

// SincronizarOrganogramaJC — lê IAUSER.CADRCA_JC inteiro e grava o nome
// ATUAL de cada código de gerente/supervisor/rca. Idempotente (UPSERT),
// seguro de rodar quantas vezes quiser.
func SincronizarOrganogramaJC(db *sql.DB) error {
	empresaID := strings.TrimSpace(os.Getenv("JC_EMPRESA_ID"))
	if empresaID == "" {
		return fmt.Errorf("JC_EMPRESA_ID não configurado")
	}
	dsn, err := dsnJC()
	if err != nil {
		return err
	}
	t0 := time.Now()
	conn, err := sql.Open("oracle", dsn)
	if err != nil {
		return fmt.Errorf("abrir conexão Oracle: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("conectar no Oracle da JC: %w", err)
	}

	// IAUSER.CADRCA_JC — sinônimo pra IAADMIN.CADRCA_JC, mesmo padrão de
	// IAUSER.COMPRAS_FAROL_VW que jc_extrator.go já usa. Tabela pequena
	// (organograma da empresa inteira, não venda) — scan completo é barato.
	rows, err := conn.QueryContext(ctx, `
		SELECT CODGERENTE, GERENTE, CODSUPERVISOR, SUPERVISOR, CODUSUR, RCA, DT_REG
		FROM IAUSER.CADRCA_JC
	`)
	if err != nil {
		return fmt.Errorf("consultar CADRCA_JC: %w", err)
	}
	defer rows.Close()

	gerentes := map[string]maisRecente{}
	supervisores := map[string]maisRecente{}
	rcasMap := map[string]maisRecente{}
	linhas := 0
	for rows.Next() {
		var codGer, ger, codSup, sup, codUsur, rca sql.NullString
		var dtReg sql.NullTime
		if err := rows.Scan(&codGer, &ger, &codSup, &sup, &codUsur, &rca, &dtReg); err != nil {
			log.Printf("[jc:organograma] erro no scan (linha ignorada): %v", err)
			continue
		}
		linhas++
		dt := dtReg.Time
		atualizaSeMaisRecente(gerentes, codGer.String, ger.String, dt)
		atualizaSeMaisRecente(supervisores, codSup.String, sup.String, dt)
		atualizaSeMaisRecente(rcasMap, codUsur.String, rca.String, dt)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("ler CADRCA_JC: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("abrir transação: %w", err)
	}
	defer tx.Rollback()

	grava := func(nivel string, m map[string]maisRecente) (int, error) {
		n := 0
		for cod, v := range m {
			if _, err := tx.Exec(`
				INSERT INTO farol.cadastro_organograma_jc (empresa_id, nivel, codigo, nome, atualizado_em)
				VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (empresa_id, nivel, codigo) DO UPDATE SET
					nome = EXCLUDED.nome, atualizado_em = EXCLUDED.atualizado_em, sincronizado_em = now()
				WHERE EXCLUDED.atualizado_em >= farol.cadastro_organograma_jc.atualizado_em
			`, empresaID, nivel, cod, v.nome, v.dtReg); err != nil {
				return n, fmt.Errorf("gravar %s código %s: %w", nivel, cod, err)
			}
			n++
		}
		return n, nil
	}

	nGer, err := grava("gerente", gerentes)
	if err != nil {
		return err
	}
	nSup, err := grava("supervisor", supervisores)
	if err != nil {
		return err
	}
	nRca, err := grava("rca", rcasMap)
	if err != nil {
		return err
	}

	// gestores/rcas (migration 121/122) — mesmas tabelas que os deep links
	// do ION VENDAS já leem (farol_mobile.go, farol_web.go). Só o nome;
	// uf/regiao/cod_filial/ativo continuam curados manualmente, por isso
	// INSERT ... ON CONFLICT DO UPDATE SET nome (não toca o resto).
	//
	// Achado real 2026-10-07: em produção essas tabelas NÃO EXISTEM — a
	// migration que as cria está marcada como já executada em
	// schema_migrations, mas o "schema limpo" da Reescrita 2026 (ver
	// CLAUDE.md) aparentemente as descartou sem reexecutar a migration.
	// Checa a existência ANTES de tentar gravar: se não existir, pula esse
	// bloco (best-effort) em vez de abortar a transação inteira e perder
	// também o cadastro_organograma_jc, que é o alvo principal.
	var gestoresExiste, rcasExiste bool
	_ = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='gestores')`).Scan(&gestoresExiste)
	_ = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='rcas')`).Scan(&rcasExiste)
	if !gestoresExiste || !rcasExiste {
		log.Printf("[jc:organograma] AVISO: tabela gestores=%v rcas=%v (esperada=true) — pulando sync pros deep links do ION VENDAS, só cadastro_organograma_jc foi atualizado", gestoresExiste, rcasExiste)
	}
	if gestoresExiste {
		for cod, v := range supervisores {
			codInt, err := strconv.Atoi(cod)
			if err != nil {
				continue
			}
			if _, err := tx.Exec(`
				INSERT INTO gestores (empresa_id, cod_supervisor, nome)
				VALUES ($1, $2, $3)
				ON CONFLICT (empresa_id, cod_supervisor) DO UPDATE SET
					nome = EXCLUDED.nome, updated_at = now()
				WHERE gestores.nome IS DISTINCT FROM EXCLUDED.nome
			`, empresaID, codInt, v.nome); err != nil {
				return fmt.Errorf("gravar gestores (supervisor) código %s: %w", cod, err)
			}
		}
	}
	if rcasExiste {
		for cod, v := range rcasMap {
			codInt, err := strconv.Atoi(cod)
			if err != nil {
				continue
			}
			if _, err := tx.Exec(`
				INSERT INTO rcas (empresa_id, cod_rca, nome)
				VALUES ($1, $2, $3)
				ON CONFLICT (empresa_id, cod_rca) DO UPDATE SET
					nome = EXCLUDED.nome, updated_at = now()
				WHERE rcas.nome IS DISTINCT FROM EXCLUDED.nome
			`, empresaID, codInt, v.nome); err != nil {
				return fmt.Errorf("gravar rcas código %s: %w", cod, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	log.Printf("[jc:organograma] sincronizado: %d linhas CADRCA_JC → %d gerente(s), %d supervisor(es), %d rca(s) em %v",
		linhas, nGer, nSup, nRca, time.Since(t0))
	return nil
}
