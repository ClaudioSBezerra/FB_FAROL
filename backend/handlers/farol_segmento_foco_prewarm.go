package handlers

// farol_segmento_foco_prewarm.go — aquece o cache do Segmento Foco (Reckitt).
//
// O painel calcula ao vivo (~40 mil linhas de venda por mês e fluxo) e guarda o
// resultado em memória (segCache). Sem aquecer, o 1º acesso depois de cada
// carga ou deploy paga a conta inteira. Aqui, a cada prewarm (1x/dia às 04:30,
// depois da carga D-1 e depois de cada importação do transmitido — todos já
// passam por PrewarmMetasRealizados) o cache é limpo e refeito pros meses do
// trimestre atual e do anterior, nos 3 fluxos. A tela e o detalhe leem do
// mesmo cache (segCalcular), então os dois ficam rápidos.
//
// É cache em memória: some a cada deploy até o próximo prewarm. Opção por
// snapshot no banco ficou de fora de propósito (decisão do Claudio, 28/10/2026).

import (
	"database/sql"
	"log"
	"time"
)

var fluxosPrewarmSegmentoFoco = []string{"faturado", "transmitido", "soma"}

// segAquecer calcula (e deixa no cache) os 3 fluxos de uma vigência.
func segAquecer(db *sql.DB, empresaID string, vinculoID, vigenciaID int) (ok, falhas int) {
	for _, fluxo := range fluxosPrewarmSegmentoFoco {
		if _, err := segCalcular(db, empresaID, vinculoID, vigenciaID, fluxo); err != nil {
			// vigência sem lista de produtos importada: cadastro incompleto, não é erro operacional.
			falhas++
			continue
		}
		ok++
	}
	return
}

// PrewarmSegmentoFoco refaz o cache do Segmento Foco da empresa. Chamado de
// dentro de PrewarmMetasRealizados.
func PrewarmSegmentoFoco(db *sql.DB, empresaID string) {
	t0 := time.Now()
	rows, err := db.Query(`
		SELECT v.vinculo_id, v.id
		FROM farol.metas_vigencias v
		JOIN farol.metas_vinculos mv ON mv.id = v.vinculo_id
		JOIN farol.tipos_metrica tm ON tm.id = mv.tipo_metrica_id
		WHERE v.empresa_id = $1 AND mv.ativo = true AND tm.formula_codigo = 'positivacao_grupos'
		  AND v.data_inicio >= date_trunc('quarter', CURRENT_DATE) - INTERVAL '3 months'
		ORDER BY v.data_inicio DESC`, empresaID)
	if err != nil {
		log.Printf("[farol:segmento-foco] prewarm: falha ao listar vigências: %v", err)
		return
	}
	type par struct{ vinculo, vigencia int }
	var pares []par
	for rows.Next() {
		var p par
		if rows.Scan(&p.vinculo, &p.vigencia) == nil {
			pares = append(pares, p)
		}
	}
	rows.Close()
	if len(pares) == 0 {
		return
	}
	segInvalidarCache() // dado novo (carga/importação): descarta o que estava aquecido
	n, falhas := 0, 0
	for _, p := range pares {
		ok, f := segAquecer(db, empresaID, p.vinculo, p.vigencia)
		n += ok
		falhas += f
	}
	log.Printf("[farol:segmento-foco] prewarm empresa=%s: %d cálculos aquecidos, %d sem dados, em %v", empresaID, n, falhas, time.Since(t0).Round(time.Millisecond))
}
