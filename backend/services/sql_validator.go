package services

import (
	"fmt"
	"regexp"
	"strings"
)

// Tabelas/views que o assistente pode consultar. Qualquer SQL referenciando
// objetos fora desta lista é rejeitado.
var allowedObjects = []string{
	"farol.vw_propostas_chat",
	"farol.vw_imports_chat",
	"farol.vw_destinatarios_chat",
	"farol.vw_ignorados_chat",
	"farol.vw_resumo_executivo_chat",
	// permitir por nome curto também (sem schema) para a IA não depender disso:
	"vw_propostas_chat",
	"vw_imports_chat",
	"vw_destinatarios_chat",
	"vw_ignorados_chat",
	"vw_resumo_executivo_chat",
}

// Palavras-chave proibidas (case-insensitive). Evita DDL/DML/comandos de sistema.
var forbiddenKeywords = []string{
	"insert", "update", "delete", "drop", "truncate", "alter", "create",
	"grant", "revoke", "copy", "vacuum", "analyze", "reindex", "cluster",
	"do", "execute", "perform", "call", "notify", "listen",
	"pg_", "current_user", "session_user", "current_database",
}

// Regex para detectar identificadores de tabela após FROM/JOIN.
var rxFromJoin = regexp.MustCompile(`(?i)\b(?:from|join)\s+([a-z_][a-z0-9_.]*)`)

// ValidarSQL aplica validações de segurança no SQL gerado pela IA:
//  1. Deve começar com SELECT ou WITH (case-insensitive, ignorando whitespace)
//  2. Não pode conter ; no meio (impede stacking)
//  3. Não pode conter palavras-chave proibidas (DDL/DML)
//  4. Todos os identificadores em FROM/JOIN devem estar na lista branca
//  5. Adiciona LIMIT 100 se não houver
//
// Retorna o SQL pronto para execução ou erro descritivo.
func ValidarSQL(sql string) (string, error) {
	clean := strings.TrimSpace(sql)
	if clean == "" {
		return "", fmt.Errorf("SQL vazio")
	}
	// Remove ; final (comum)
	clean = strings.TrimRight(clean, "; \n\t")

	// 1. Deve começar com SELECT ou WITH
	lower := strings.ToLower(clean)
	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") {
		return "", fmt.Errorf("apenas SELECT/WITH são permitidos")
	}

	// 2. Não pode ter ; no meio (impede múltiplas queries)
	if strings.Contains(clean, ";") {
		return "", fmt.Errorf("não é permitido ';' no meio da query")
	}

	// 3. Palavras-chave proibidas (verifica como tokens, não substrings — usa
	//    boundaries de palavra para não dar falso positivo em nomes de coluna).
	for _, kw := range forbiddenKeywords {
		rx := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(kw) + `\b`)
		if rx.MatchString(clean) {
			return "", fmt.Errorf("palavra-chave proibida: %s", kw)
		}
	}

	// 4. Tabelas/views referenciadas devem estar na lista branca
	matches := rxFromJoin.FindAllStringSubmatch(clean, -1)
	allowed := make(map[string]bool, len(allowedObjects))
	for _, o := range allowedObjects {
		allowed[strings.ToLower(o)] = true
	}
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		obj := strings.ToLower(m[1])
		// remove possíveis aliases que viriam logo após (não capturado pelo rx)
		if !allowed[obj] {
			return "", fmt.Errorf("objeto não permitido: %s", m[1])
		}
	}

	// 5. Adiciona LIMIT 100 se não tiver LIMIT
	if !regexp.MustCompile(`(?i)\blimit\s+\d+`).MatchString(clean) {
		clean += "\nLIMIT 100"
	}

	return clean, nil
}
