// farol_dinheiro_mesa.go — cálculo de ritmo e "dinheiro na mesa" por RCA.
//
// O CONCEITO: "estar atrás" não é `realizado < meta`. É `realizado < o que já
// deveria ter vendido a esta altura do mês`. No dia 5 ninguém bateu a meta, e
// isso não é notícia; no dia 20 com 40% do ritmo, é.
//
//	ritmo_esperado = meta × (dias_úteis_decorridos / dias_úteis_totais)
//	dinheiro_mesa  = max(ritmo_esperado − realizado, 0)
//
// E a ordenação é por REAIS ABSOLUTOS, nunca por percentual: o RCA grande a
// 90% do ritmo deixa mais dinheiro na mesa que o pequeno a 50%, e é nele que o
// supervisor precisa mexer primeiro.
//
// LÍQUIDO CONTRA LÍQUIDO (decisão do gestor, 22/08/2026): o realizado sai da
// coluna `liquido` do agregado — já descontadas devoluções e cancelamentos — e
// a meta de `objetivos_importados.vl_corrente` é lida como alvo líquido.
//
// ⚠ Enquanto o CCD estiver defasado, o líquido fica OTIMISTA e o dinheiro na
// mesa sai SUBESTIMADO. Em 18/08/2026 a defasagem medida era de R$ 18,4 mi.
package services

import (
	"database/sql"
	"fmt"
	"time"
)

// Baseline — contra o que o ritmo é medido.
//
// Em 22/08/2026 a consulta a objetivos_importados devolveu ZERO linhas para
// 2026: não há meta cadastrada. Sem alvo não existe "dinheiro na mesa" — e
// inventar um seria cobrar a equipe por número que a JC nunca definiu.
//
// Daí os dois modos. O ANO_ANTERIOR funciona hoje, com o dado que existe, e é
// exatamente a régua que o semáforo do painel V2 já usa — então o e-mail e a
// tela contam a mesma história. Quando a JC importar as metas, muda-se uma
// variável e a régua passa a ser o alvo real, sem tocar no resto.
//
// O e-mail SEMPRE declara qual régua usou. Chamar de "meta" o que é
// comparação com o ano anterior seria a mentira mais fácil de contar aqui.
type Baseline string

const (
	BaselineMeta        Baseline = "meta"         // objetivos_importados.vl_corrente
	BaselineAnoAnterior Baseline = "ano_anterior" // mesmo mês do ano passado
)

// Rotulo — como a régua se chama para o leitor do e-mail.
func (b Baseline) Rotulo() string {
	if b == BaselineMeta {
		return "meta do mês"
	}
	return "mesmo mês do ano anterior"
}

// RcaMesa — uma linha do ranking.
type RcaMesa struct {
	CodGerente    string `json:"cod_gerente"`
	CodSupervisor string `json:"cod_supervisor"`
	CodRca        string `json:"cod_rca"`
	NomeRca       string `json:"nome_rca"`

	Meta          float64 `json:"meta"` // alvo do mês, conforme a Baseline
	Realizado     float64 `json:"realizado"`
	RitmoEsperado float64 `json:"ritmo_esperado"`
	DinheiroMesa  float64 `json:"dinheiro_mesa"`
	Atingimento   float64 `json:"atingimento"` // realizado / ritmo × 100
	Faixa         string  `json:"faixa"`       // "R" | "Y" | "G"
	Motivo        string  `json:"motivo"`      // "POSITIVACAO" | "MIX" | ""

	Positivados int     `json:"positivados"`
	BaseCli     int     `json:"base_cli"`
	Mix         float64 `json:"mix"`
}

// PositivacaoPct — 0 quando a carteira é desconhecida, não divisão por zero.
func (r RcaMesa) PositivacaoPct() float64 {
	if r.BaseCli <= 0 {
		return 0
	}
	return float64(r.Positivados) / float64(r.BaseCli) * 100
}

// Cobertura — o que ficou de fora, para o e-mail poder dizer.
//
// Existe porque um ranking silenciosamente incompleto é pior que um ranking
// que se declara incompleto: o gestor olha os cinco piores e conclui que o
// resto está bem, quando na verdade metade da equipe não tinha meta cadastrada.
type Cobertura struct {
	RcasComVenda   int      `json:"rcas_com_venda"`
	RcasComMeta    int      `json:"rcas_com_meta"`
	DiasDecorridos int      `json:"dias_decorridos"`
	DiasTotais     int      `json:"dias_totais"`
	FonteDiasTotal string   `json:"fonte_dias_total"` // "historico" | "calendario"
	Baseline       Baseline `json:"baseline"`
}

// diasUteis — conta os dias a partir do DADO, não do calendário.
//
// Dia útil aqui é "dia em que a empresa faturou". Isso resolve de graça três
// coisas que um contador de calendário erraria: sábado (a JC opera), feriado
// nacional e feriado regional de Goiás — que ninguém ia lembrar de cadastrar.
//
// O total do mês vem do mesmo mês do ano anterior. Se não houver histórico
// (filial nova, primeiro ano), cai para contagem de segunda a sábado no
// calendário — aí sim ignorando feriado, e o e-mail declara qual fonte usou.
func diasUteis(db *sql.DB, empresaID string, ano, mes int, ate time.Time) (decorridos, totais int, fonte string) {
	ini := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)

	_ = db.QueryRow(`
		SELECT COUNT(DISTINCT data_faturamento) FROM vendas_faturadas
		 WHERE empresa_id=$1 AND data_faturamento >= $2 AND data_faturamento <= $3`,
		empresaID, ini, ate).Scan(&decorridos)

	iniAnt := ini.AddDate(-1, 0, 0)
	fimAnt := iniAnt.AddDate(0, 1, -1)
	_ = db.QueryRow(`
		SELECT COUNT(DISTINCT data_faturamento) FROM vendas_faturadas
		 WHERE empresa_id=$1 AND data_faturamento >= $2 AND data_faturamento <= $3`,
		empresaID, iniAnt, fimAnt).Scan(&totais)

	fonte = "historico"
	if totais == 0 {
		fonte = "calendario"
		fim := ini.AddDate(0, 1, -1)
		for d := ini; !d.After(fim); d = d.AddDate(0, 0, 1) {
			if d.Weekday() != time.Sunday {
				totais++
			}
		}
	}

	// O mês corrente pode passar do ano anterior (mês com mais dias úteis, ou
	// mutirão de fim de mês). Sem este piso, o ritmo passaria de 100% e o
	// dinheiro na mesa zeraria para a equipe inteira — parecendo que está tudo
	// bem justamente no mês em que se trabalhou mais.
	if decorridos > totais {
		totais = decorridos
	}
	return decorridos, totais, fonte
}

// ColetarDinheiroNaMesa devolve o ranking do mês, ordenado por reais na mesa.
//
// `ate` é o último dia considerado — normalmente ontem, porque o dia corrente
// ainda está sendo faturado e entraria pela metade.
//
// RCA sem alvo FICA DE FORA: sem alvo não há ritmo. Com BaselineMeta isso
// significa "sem meta cadastrada"; com BaselineAnoAnterior, "não vendia neste
// mês no ano passado" — RCA novo, que de fato não tem contra o que comparar.
// A contagem sai na Cobertura para o e-mail declarar em vez de omitir.
func ColetarDinheiroNaMesa(db *sql.DB, empresaID string, ano, mes int, ate time.Time, base Baseline) ([]RcaMesa, Cobertura, error) {
	var cob Cobertura
	cob.Baseline = base
	cob.DiasDecorridos, cob.DiasTotais, cob.FonteDiasTotal = diasUteis(db, empresaID, ano, mes, ate)
	if cob.DiasTotais == 0 {
		return nil, cob, fmt.Errorf("sem dias úteis apurados para %04d-%02d", ano, mes)
	}
	frac := float64(cob.DiasDecorridos) / float64(cob.DiasTotais)

	// O código do RCA é INTEGER nos objetivos e TEXT no agregado. Normalizo
	// pelos dígitos em vez de castar direto: '06204' e '6204' são o mesmo RCA,
	// e um cast ingênuo os trataria como diferentes — o RCA sumiria do ranking
	// sem erro nenhum, que é o pior tipo de falha.
	// MÉTRICAS do v04_l0 (uma linha por RCA), HIERARQUIA do v03_l2.
	//
	// A primeira versão lia tudo do v03_l2, que é chaveado por (gerente,
	// supervisor, RCA). RCA que atende sob dois supervisores virava DUAS linhas,
	// cada uma com um alvo inteiro — aparecia duplicado no ranking e inflava o
	// total. Visto na prévia de 22/08/2026: VALDIMAR e G F GOMES ocupando duas
	// posições cada.
	//
	// O v03_l2 continua entrando, mas só para dizer a QUEM o RCA pertence, pelo
	// supervisor de maior volume no mês. Um RCA em duas equipes é anomalia de
	// cadastro; atribuí-lo à equipe onde ele mais vendeu é a leitura menos
	// errada, e não muda nenhum total.
	alvo := `
		    SELECT cod_rca, SUM(vl_corrente) AS meta
		      FROM objetivos_importados
		     WHERE empresa_id=$1 AND tipo_periodo='MENSAL'
		       AND ano=$2 AND periodo_seq=$3
		     GROUP BY cod_rca`
	join := `ON m.cod_rca = NULLIF(regexp_replace(a.cod_rca, '[^0-9]', '', 'g'), '')::int`

	if base == BaselineAnoAnterior {
		alvo = `
		    SELECT cod_rca::text AS cod_rca, SUM(liquido) AS meta
		      FROM farol.agg_fat_v04_l0_mes
		     WHERE empresa_id=$1 AND ano=$2-1 AND mes=$3
		     GROUP BY cod_rca`
		join = `ON m.cod_rca = a.cod_rca`
	}

	rows, err := db.Query(fmt.Sprintf(`
		WITH meta AS (%s),
		dono AS (
		    SELECT DISTINCT ON (cod_rca) cod_rca, cod_gerente, cod_supervisor
		      FROM farol.agg_fat_v03_l2_mes
		     WHERE empresa_id=$1 AND ano=$2 AND mes=$3 AND cod_rca <> ''
		     ORDER BY cod_rca, liquido DESC
		)
		SELECT COALESCE(d.cod_gerente,''), COALESCE(d.cod_supervisor,''),
		       a.cod_rca, a.nome_rca,
		       COALESCE(m.meta, 0)::float8,
		       a.liquido::float8, a.positivados, a.base_cli, a.mix::float8
		  FROM farol.agg_fat_v04_l0_mes a
		  LEFT JOIN meta m %s
		  LEFT JOIN dono d ON d.cod_rca = a.cod_rca
		 WHERE a.empresa_id=$1 AND a.ano=$2 AND a.mes=$3
		   AND a.cod_rca <> ''
		   -- Cadastro morto não é mau desempenho. Mesma regra do assistente:
		   -- ranking cheio de INATIVO ensina o gestor a ignorar a lista.
		   AND a.nome_rca NOT ILIKE '%%INATIVO%%'
		   AND a.nome_rca NOT ILIKE '%%SAIU%%'`, alvo, join),
		empresaID, ano, mes)
	if err != nil {
		return nil, cob, fmt.Errorf("consultar dinheiro na mesa: %w", err)
	}
	defer rows.Close()

	var todos []RcaMesa
	for rows.Next() {
		var r RcaMesa
		if err := rows.Scan(&r.CodGerente, &r.CodSupervisor, &r.CodRca, &r.NomeRca,
			&r.Meta, &r.Realizado, &r.Positivados, &r.BaseCli, &r.Mix); err != nil {
			continue
		}
		cob.RcasComVenda++
		if r.Meta <= 0 {
			continue
		}
		cob.RcasComMeta++

		r.RitmoEsperado = r.Meta * frac
		if r.RitmoEsperado > 0 {
			r.Atingimento = r.Realizado / r.RitmoEsperado * 100
		}
		if r.DinheiroMesa = r.RitmoEsperado - r.Realizado; r.DinheiroMesa < 0 {
			r.DinheiroMesa = 0
		}
		switch {
		case r.Atingimento < 70:
			r.Faixa = "R"
		case r.Atingimento < 90:
			r.Faixa = "Y"
		default:
			r.Faixa = "G"
		}
		todos = append(todos, r)
	}

	atribuirMotivo(todos)
	ordenarPorMesa(todos)
	return todos, cob, nil
}

// atribuirMotivo — o porquê vem do DADO, não da IA.
//
// Compara o RCA com a média da PRÓPRIA equipe (mesmo supervisor), não com a
// empresa: rota de capital e rota de interior têm patamares diferentes, e
// comparar com a média geral acusaria o interior inteiro todo mês.
//
// Só dois motivos, não três. O guia original previa TICKET MÉDIO, que precisa
// de número de pedidos — e as 40 colunas da COMPRAS_FAROL_VW não trazem
// nenhuma identificação de pedido. Verificado em 19/08/2026. Enquanto a origem
// não expuser isso, ticket não é calculável e prometê-lo seria inventar.
func atribuirMotivo(rs []RcaMesa) {
	type acc struct {
		posit, mix float64
		n          int
	}
	base := map[string]*acc{}
	for _, r := range rs {
		a := base[r.CodSupervisor]
		if a == nil {
			a = &acc{}
			base[r.CodSupervisor] = a
		}
		a.posit += r.PositivacaoPct()
		a.mix += r.Mix
		a.n++
	}

	for i := range rs {
		r := &rs[i]
		if r.Faixa == "G" {
			continue // quem está no ritmo não precisa de diagnóstico
		}
		a := base[r.CodSupervisor]
		if a == nil || a.n == 0 {
			continue
		}
		mediaPosit := a.posit / float64(a.n)
		mediaMix := a.mix / float64(a.n)

		// Desvio RELATIVO: cair 10 pontos numa base de 80 é diferente de cair
		// 10 numa base de 20.
		var dPosit, dMix float64
		if mediaPosit > 0 {
			dPosit = (mediaPosit - r.PositivacaoPct()) / mediaPosit
		}
		if mediaMix > 0 {
			dMix = (mediaMix - r.Mix) / mediaMix
		}
		switch {
		case dPosit <= 0 && dMix <= 0:
			r.Motivo = "" // está na média ou acima nos dois; o gap é de volume
		case dPosit >= dMix:
			r.Motivo = "POSITIVACAO"
		default:
			r.Motivo = "MIX"
		}
	}
}

func ordenarPorMesa(rs []RcaMesa) {
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].DinheiroMesa > rs[j-1].DinheiroMesa; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
}

// FiltrarEscopo recorta o ranking pelo que a pessoa pode ver.
//
// Espelha farol_escopo.go de propósito: persona sem coluna de escopo
// (gerente_geral, diretor, ceo) enxerga tudo; ggv recorta por cod_gerente;
// supervisor por cod_supervisor. Se as duas regras divergirem, alguém recebe
// por e-mail o que não vê na tela — e a diferença aparece como suspeita de
// vazamento, não como bug de filtro.
func FiltrarEscopo(rs []RcaMesa, persona, codRef string) []RcaMesa {
	var col string
	switch persona {
	case "ggv":
		col = "cod_gerente"
	case "supervisor":
		col = "cod_supervisor"
	default:
		return rs
	}
	if codRef == "" {
		return nil // fail-closed, igual ao escopo do painel
	}
	out := make([]RcaMesa, 0, len(rs))
	for _, r := range rs {
		if (col == "cod_gerente" && r.CodGerente == codRef) ||
			(col == "cod_supervisor" && r.CodSupervisor == codRef) {
			out = append(out, r)
		}
	}
	return out
}

// TotalNaMesa soma o gap do recorte.
func TotalNaMesa(rs []RcaMesa) float64 {
	var t float64
	for _, r := range rs {
		t += r.DinheiroMesa
	}
	return t
}

// ContarFaixas devolve quantos estão em cada cor.
func ContarFaixas(rs []RcaMesa) (vermelho, amarelo, verde int) {
	for _, r := range rs {
		switch r.Faixa {
		case "R":
			vermelho++
		case "Y":
			amarelo++
		default:
			verde++
		}
	}
	return
}
