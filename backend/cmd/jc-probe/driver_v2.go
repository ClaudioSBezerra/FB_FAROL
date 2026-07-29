//go:build oradriver_v2

package main

import go_ora "github.com/sijms/go-ora/v2"

// go-ora v2 — linha madura (24 releases só na 2.8.x). Usada para isolar se um
// erro é bug da v3 ou problema real do servidor: se a v2 devolve um ORA- limpo
// onde a v3 devolve erro de protocolo cru, o problema é da v3.
const driverVersao = "go-ora v2.9.0"

func buildURL(host string, port int, service, user, pass string, opts map[string]string) string {
	return go_ora.BuildUrl(host, port, service, user, pass, opts)
}
