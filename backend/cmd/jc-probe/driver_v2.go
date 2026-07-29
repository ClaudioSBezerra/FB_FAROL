//go:build !oradriver_v3

package main

import go_ora "github.com/sijms/go-ora/v2"

// go-ora v2 — DRIVER PADRÃO desde 29/07/2026, decidido por teste contra o
// servidor real da JC (Oracle 23ai): a v3.0.1 morre na negociação de protocolo
// TCP ("received code 4 and expected code is 1", tcp_protocol_nego.go:45),
// enquanto a v2.9.0 chega à autenticação e devolve ORA- legível.
//
// A v3 é a versão anunciada com suporte a 23ai, mas tem só duas releases; a v2
// tem 24 só na linha 2.8.x. Escolha vale para o extrator, não só para a sonda.
// Para reproduzir o teste: compilar com `-tags oradriver_v3`.
const driverVersao = "go-ora v2.9.0"

func buildURL(host string, port int, service, user, pass string, opts map[string]string) string {
	return go_ora.BuildUrl(host, port, service, user, pass, opts)
}
