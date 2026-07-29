//go:build oradriver_v3

package main

import go_ora "github.com/sijms/go-ora/v3"

// go-ora v3 — NÃO funciona contra o Oracle 23ai da JC: morre na negociação de
// protocolo TCP antes da autenticação (tcp_protocol_nego.go:45, "received code 4
// and expected code is 1"). Mantida atrás de build tag só para reconferir em
// versões futuras — se uma v3.1+ corrigir, vale reavaliar, porque é a linha que
// recebe os tipos novos do 23ai (VECTOR/JSON/BOOLEAN). Nada disso é usado pela
// extração, que lê só colunas escalares.
//
// As duas registram o driver "oracle" no database/sql e não podem coexistir num
// binário — daí build tag em vez de flag.
const driverVersao = "go-ora v3.0.1"

func buildURL(host string, port int, service, user, pass string, opts map[string]string) string {
	return go_ora.BuildUrl(host, port, service, user, pass, opts)
}
