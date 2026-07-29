//go:build !oradriver_v2

package main

import go_ora "github.com/sijms/go-ora/v3"

// go-ora v3 — a versão anunciada com suporte a 23ai, mas com só duas releases.
// Padrão da sonda. Para comparar com a v2 (madura), compilar com
// `-tags oradriver_v2`.
//
// As duas registram o driver "oracle" no database/sql, então não podem coexistir
// no mesmo binário — daí a separação por build tag em vez de uma flag.
const driverVersao = "go-ora v3.0.1"

func buildURL(host string, port int, service, user, pass string, opts map[string]string) string {
	return go_ora.BuildUrl(host, port, service, user, pass, opts)
}
