// FarolDinheiroNaMesa — a página que o resumo semanal linka.
//
// Os mesmos números do e-mail, servidos pelo mesmo motor no backend. Se um dia
// a tela e o e-mail discordarem, é porque alguém duplicou o cálculo — e não vai
// ser aqui: as duas leem /api/v2/farol/dinheiro-na-mesa.
//
// ⚠ ÚNICA TELA ESCURA DO FAROL. O CLAUDE.md manda manter o padrão "Clean
// Professional", que é claro; esta é exceção deliberada, decidida em 22/08/2026
// pelo dono da JC, que preferiu este estilo. A exceção está registrada lá.
//
// A justificativa de produto: esta tela é de CAMPO. Chega por WhatsApp, é lida
// no celular, muitas vezes fora do escritório, e compete com a atenção de um
// aplicativo de mensagem. Fundo escuro com um número grande em âmbar tem outra
// presença nesse contexto. As telas de trabalho continuam claras.
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'

interface RcaMesa {
  cod_rca: string
  nome_rca: string
  dinheiro_mesa: number
  atingimento: number
  faixa: string
  motivo: string
}

interface Grupo {
  cod: string
  nome: string
  link: string
  total_mesa: number
  rcas: number
  vermelhos: number
}

interface Resumo {
  nome: string
  persona: string
  escopo: string
  mes: string
  total_mesa: number
  realizado: number
  ritmo: number
  vermelho: number
  amarelo: number
  verde: number
  grupos: Grupo[] | null
  top_geral: RcaMesa[] | null
  resto_rcas: number
  resto_valor: number
  ano?: {
    rotulo: string
    realizado: number
    alvo: number
    total_mesa: number
    vermelho: number
    amarelo: number
    verde: number
    grupos: Grupo[] | null
    top_geral: RcaMesa[] | null
    resto_rcas: number
    resto_valor: number
  } | null
  projecao?: {
    ano: number
    ano_ant: number
    ultimo_mes: number
    ano_anterior: number
    crescimento_pct: number
    mes_pct: number
    piso: number
    ritmo: number
    conservador: number
  } | null
  cobertura: {
    rcas_com_venda: number
    rcas_com_meta: number
    dias_decorridos: number
    dias_totais: number
    fonte_dias_total: string
    baseline: string
  }
}

const MOTIVO: Record<string, { tag: string; texto: string; cor: string }> = {
  POSITIVACAO: { tag: 'POSITIVAÇÃO', texto: 'positivação abaixo da média da equipe', cor: 'rgba(229,84,75,.14);color:#E5544B' },
  MIX:         { tag: 'MIX',         texto: 'mix abaixo da média da equipe',         cor: 'rgba(232,163,61,.14);color:#E8A33D' },
}

// Fonte carregada só aqui. O resto do app não usa Archivo nem IBM Plex, e
// pendurar 3 famílias no index.html faria toda tela pagar o download por causa
// de uma. Idempotente: se já está no head, não repete.
function useFontesDoQuadro() {
  useEffect(() => {
    const id = 'fontes-dinheiro-na-mesa'
    if (document.getElementById(id)) return
    const l = document.createElement('link')
    l.id = id
    l.rel = 'stylesheet'
    l.href = 'https://fonts.googleapis.com/css2?family=Archivo:wght@600;700;800&family=IBM+Plex+Mono:wght@500;600&family=IBM+Plex+Sans:wght@400;500;600&display=swap'
    document.head.appendChild(l)
  }, [])
}

const brl = (v: number) => v.toLocaleString('pt-BR', { maximumFractionDigits: 0 })

export default function FarolDinheiroNaMesa() {
  useFontesDoQuadro()

  // Duas portas para a mesma tela. Com token na URL (/q/:token) ela abre sem
  // login, que é o caminho do link do WhatsApp; sem token, usa a sessão.
  //
  // O token manda no que aparece: o backend resolve o dono dele e devolve o
  // recorte daquela pessoa. Não há como pedir o quadro de outro trocando o
  // parâmetro, porque não existe parâmetro de escopo.
  const { token } = useParams<{ token?: string }>()
  const publico = !!token
  const [periodo, setPeriodo] = useState<'mes' | 'ano'>('mes')

  const q = useQuery<Resumo>({
    queryKey: ['farol-dinheiro-na-mesa', token ?? 'sessao'],
    queryFn: async () => {
      const url = publico
        ? `/api/v2/farol/quadro/${token}`
        : '/api/v2/farol/dinheiro-na-mesa'
      const r = await fetch(url)
      if (!r.ok) throw new Error('Falha ao carregar o resumo')
      return r.json()
    },
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })

  const mono = { fontFamily: '"IBM Plex Mono",ui-monospace,monospace', fontVariantNumeric: 'tabular-nums' } as const
  const display = { fontFamily: '"Archivo",system-ui,sans-serif' } as const

  if (q.isLoading || q.error || !q.data) {
    return (
      <div style={{ background: '#0E1621', minHeight: '100vh', color: '#8195A6',
                    fontFamily: '"IBM Plex Sans",system-ui,sans-serif', padding: '48px 20px' }}>
        {q.isLoading
          ? 'Calculando o ritmo do mês…'
          : publico
            ? 'Este link não é mais válido. Peça um novo ao administrador do Farol.'
            : 'Não foi possível carregar o quadro.'}
      </div>
    )
  }

  const d = q.data

  // Uma vista só, alimentada pelo mês ou pelo ano. Sem isto o corpo da página
  // teria dois caminhos paralelos, e a primeira mudança de layout esqueceria um
  // deles.
  const ano = d.ano ?? null
  const verAno = periodo === 'ano' && !!ano
  const vista = verAno && ano
    ? { rotulo: ano.rotulo, realizado: ano.realizado, ritmo: ano.alvo,
        total_mesa: ano.total_mesa, vermelho: ano.vermelho, amarelo: ano.amarelo,
        verde: ano.verde, grupos: ano.grupos ?? [], top: ano.top_geral ?? [],
        resto_rcas: ano.resto_rcas, resto_valor: ano.resto_valor }
    : { rotulo: d.mes, realizado: d.realizado, ritmo: d.ritmo,
        total_mesa: d.total_mesa, vermelho: d.vermelho, amarelo: d.amarelo,
        verde: d.verde, grupos: d.grupos ?? [], top: d.top_geral ?? [],
        resto_rcas: d.resto_rcas, resto_valor: d.resto_valor }

  const grupos = vista.grupos
  const top = vista.top
  const maiorGrupo = grupos.reduce((m, g) => Math.max(m, g.total_mesa), 0)
  const maiorRca = top.reduce((m, x) => Math.max(m, x.dinheiro_mesa), 0)

  // O alvo do MÊS INTEIRO, reconstruído a partir do ritmo: o backend manda o
  // esperado até hoje, e a barra de progresso precisa do total para mostrar
  // onde a equipe está no mês, não só contra a fração decorrida.
  // A barra de progresso saiu junto com o herói antigo: ela era uma terceira
  // representação do mesmo fato que os dois percentuais já dizem melhor.
  const { dias_decorridos: dd, dias_totais: dt } = d.cobertura
  const pctRitmo = vista.ritmo > 0 ? (vista.realizado / vista.ritmo) * 100 : 0
  const saldo = vista.realizado - vista.ritmo

  return (
    <div style={{ background: '#0E1621', minHeight: '100vh', color: '#EAF0F5',
                  fontFamily: '"IBM Plex Sans",system-ui,sans-serif',
                  WebkitFontSmoothing: 'antialiased', padding: '24px 16px 56px' }}>
      <div style={{ maxWidth: 1080, margin: '0 auto' }}>

        {/* ── topo ── */}
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                      gap: 16, flexWrap: 'wrap', borderBottom: '1px solid #26343F',
                      paddingBottom: 16, marginBottom: 28 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <span style={{ width: 11, height: 11, borderRadius: '50%', background: '#E8A33D',
                           boxShadow: '0 0 0 4px rgba(232,163,61,.14)' }} />
            <b style={{ ...display, fontWeight: 800, letterSpacing: '.5px', fontSize: 14 }}>DINHEIRO NA MESA</b>
            <span style={{ color: '#8195A6', fontSize: 13 }}>· Farol de Vendas</span>
          </div>
          <div style={{ textAlign: 'right', fontSize: 12.5, color: '#8195A6', lineHeight: 1.5 }}>
            <b style={{ color: '#EAF0F5' }}>{d.nome}</b><br />
            {d.escopo} · {vista.rotulo}
          </div>
        </div>

        {/* ── OS DOIS PERCENTUAIS ──
            Herói do quadro. A versão anterior punha "R$ 14.052.906 na mesa" no
            topo — número que só significa algo depois de explicado, porque soma
            apenas quem está atrás. "96%" qualquer pessoa entende sem pensar.
            A posição responde "estamos bem ou mal"; o dinheiro na mesa responde
            "onde mexer". Nesta ordem. */}
        <div style={{ display: 'grid', gridTemplateColumns: ano ? 'repeat(auto-fit,minmax(150px,1fr))' : '1fr',
                      gap: 1, background: '#26343F', border: '1px solid #26343F',
                      borderRadius: 14, overflow: 'hidden', marginBottom: 22 }}>
          {[
            ...(ano ? [{ id: 'ano' as const, rot: 'ANO', sub: ano.rotulo,
                         real: ano.realizado, alvo: ano.alvo }] : []),
            { id: 'mes' as const, rot: d.mes.split('/')[0].toUpperCase(), sub: `${dd} de ${dt} dias úteis`,
              real: d.realizado, alvo: d.ritmo },
          ].map(c => {
            const pct = c.alvo > 0 ? (c.real / c.alvo) * 100 : 0
            const dif = c.real - c.alvo
            const bom = pct >= 100
            const cor = bom ? '#3DC98B' : pct >= 90 ? '#E8C13D' : '#E5544B'
            const ativo = periodo === c.id
            return (
              <button key={c.id} onClick={() => setPeriodo(c.id)}
                      style={{ background: ativo ? '#1B2836' : '#15202D', border: 0,
                               borderTop: `3px solid ${ativo ? cor : 'transparent'}`,
                               padding: '22px 20px 20px', textAlign: 'left', cursor: 'pointer',
                               color: 'inherit', font: 'inherit' }}>
                <div style={{ ...display, fontWeight: 700, fontSize: 11, letterSpacing: 2,
                              color: '#5E7080' }}>{c.rot}</div>
                <div style={{ ...mono, fontWeight: 600, color: cor,
                              fontSize: 'clamp(46px,13vw,76px)', lineHeight: 1,
                              letterSpacing: '-2px', marginTop: 6 }}>
                  {pct.toFixed(0)}%
                </div>
                <div style={{ fontSize: 14, color: cor, marginTop: 4, fontWeight: 600 }}>
                  {bom ? '▲' : '▼'} {bom ? 'acima' : 'abaixo'} do alvo
                </div>
                <div style={{ ...mono, fontSize: 13, color: '#8195A6', marginTop: 8 }}>
                  {bom ? '+' : '−'} R$ {brl(Math.abs(dif))}
                </div>
                <div style={{ fontSize: 11.5, color: '#5E7080', marginTop: 6 }}>{c.sub}</div>
              </button>
            )
          })}
        </div>

        {/* Uma frase com a leitura inteira. É o que a pessoa repete depois de
            fechar a tela, então precisa caber numa respiração. */}
        <p style={{ fontSize: 15.5, color: '#8195A6', lineHeight: 1.6, margin: '0 0 26px' }}>
          {ano && ano.alvo > 0 && (ano.realizado / ano.alvo) >= 1
            ? <>O ano está construído. </>
            : <>O ano está apertado. </>}
          {pctRitmo >= 100
            ? <>{vista.rotulo} segue no ritmo.</>
            : <>{vista.rotulo} está devendo <b style={{ color: '#EAF0F5' }}>R$ {brl(Math.abs(saldo))}</b>
                {!verAno && dt > dd && <> e faltam <b style={{ color: '#EAF0F5' }}>{dt - dd} dias úteis</b></>}.</>}
          {' '}<b style={{ color: '#E5544B' }}>{vista.vermelho}</b> de{' '}
          {vista.vermelho + vista.amarelo + vista.verde} RCAs abaixo de 70% do ritmo.
        </p>

        {/* ── onde mexer ──
            O dinheiro na mesa vem AQUI, já com o rótulo que explica o que ele é.
            "Mal distribuído" em vez de "deixado de faturar": a empresa pode
            estar acima do alvo e ainda ter dinheiro na mesa, e o rótulo antigo
            fazia isso parecer contradição. */}
        <div style={{ background: '#15202D', border: '1px solid #26343F', borderRadius: 14,
                      padding: '18px 20px', marginBottom: 30 }}>
          <div style={{ ...display, fontWeight: 700, fontSize: 11, letterSpacing: 1.5,
                        color: '#E8A33D', textTransform: 'uppercase' }}>
            Onde mexer
          </div>
          <div style={{ ...mono, fontWeight: 600, fontSize: 'clamp(26px,7vw,38px)',
                        marginTop: 8, lineHeight: 1 }}>
            R$ {brl(vista.total_mesa)}
          </div>
          <div style={{ fontSize: 13.5, color: '#8195A6', marginTop: 8, lineHeight: 1.55 }}>
            é o que os RCAs atrás do ritmo somam de gap. Não é dívida da empresa —
            quem está à frente já compensa boa parte. É o que está{' '}
            <b style={{ color: '#EAF0F5' }}>mal distribuído</b>, e onde a ação rende.
          </div>
        </div>

        {/* ── por GGV ── */}
        {grupos.length > 0 && (
          <>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                          marginBottom: 16, flexWrap: 'wrap', gap: 8 }}>
              <h2 style={{ ...display, fontWeight: 700, fontSize: 16, margin: 0 }}>
                {d.persona === 'ggv' ? 'Por supervisor' : 'Por GGV'}
              </h2>
              {!publico && (
                <span style={{ fontSize: 12.5, color: '#5E7080' }}>toque para abrir o painel da equipe</span>
              )}
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 1, background: '#26343F',
                          border: '1px solid #26343F', borderRadius: 14, overflow: 'hidden', marginBottom: 34 }}>
              {grupos.map(g => (
                // Sem link no modo público, pela mesma razão do botão.
                <a key={g.cod} href={publico ? undefined : g.link}
                   style={{ background: '#15202D', padding: '16px 18px', textDecoration: 'none',
                            color: 'inherit', display: 'flex', gap: 16, alignItems: 'center',
                            cursor: publico ? 'default' : 'pointer' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 600, fontSize: 15 }}>
                      {g.nome} <span style={{ color: '#5E7080', fontWeight: 400, fontSize: 12 }}>· {g.cod}</span>
                    </div>
                    <div style={{ fontSize: 12.5, color: '#8195A6', marginTop: 5 }}>
                      {g.rcas} RCAs · {g.vermelhos} abaixo de 70%
                    </div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{ ...mono, fontWeight: 600, fontSize: 17 }}>R$ {brl(g.total_mesa)}</div>
                    <div style={{ height: 4, background: '#1B2836', borderRadius: 3, marginTop: 8,
                                  width: 110, overflow: 'hidden', marginLeft: 'auto' }}>
                      <div style={{ height: '100%', borderRadius: 3, background: '#E8A33D',
                                    width: maiorGrupo > 0 ? `${(g.total_mesa / maiorGrupo) * 100}%` : '0%' }} />
                    </div>
                  </div>
                </a>
              ))}
            </div>
          </>
        )}

        {/* ── onde está o dinheiro ── */}
        {top.length > 0 && (
          <>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                          marginBottom: 16, flexWrap: 'wrap', gap: 8 }}>
              <h2 style={{ ...display, fontWeight: 700, fontSize: 16, margin: 0 }}>Onde está o dinheiro</h2>
              <span style={{ fontSize: 12.5, color: '#5E7080' }}>ordenado por reais — não por % de atingimento</span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 1, background: '#26343F',
                          border: '1px solid #26343F', borderRadius: 14, overflow: 'hidden' }}>
              {top.map((x, i) => {
                const m = MOTIVO[x.motivo]
                return (
                  <div key={`${x.cod_rca}-${i}`}
                       style={{ background: '#15202D', padding: '16px 18px', display: 'flex',
                                gap: 16, alignItems: 'center' }}>
                    <div style={{ ...mono, fontWeight: 600, fontSize: 15, color: '#5E7080',
                                  width: 22, textAlign: 'center' }}>{i + 1}</div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 600, fontSize: 15, letterSpacing: '.2px' }}>{x.nome_rca}</div>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 5,
                                    fontSize: 12.5, color: '#8195A6', flexWrap: 'wrap' }}>
                        {m && (
                          <span style={{ ...display, fontWeight: 700, fontSize: 10, letterSpacing: 1,
                                         padding: '3px 8px', borderRadius: 5, lineHeight: 1,
                                         background: m.cor.split(';')[0], color: m.cor.split(':')[1] }}>
                            {m.tag}
                          </span>
                        )}
                        <span>{m ? m.texto : 'volume geral abaixo do ritmo'} · {x.atingimento.toFixed(0)}% do ritmo</span>
                      </div>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <div style={{ ...mono, fontWeight: 600, fontSize: 17 }}>R$ {brl(x.dinheiro_mesa)}</div>
                      <div style={{ height: 4, background: '#1B2836', borderRadius: 3, marginTop: 8,
                                    width: 110, overflow: 'hidden', marginLeft: 'auto' }}>
                        <div style={{ height: '100%', borderRadius: 3, background: '#E8A33D',
                                      width: maiorRca > 0 ? `${(x.dinheiro_mesa / maiorRca) * 100}%` : '0%' }} />
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>

            {vista.resto_rcas > 0 && (
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                            gap: 12, marginTop: 14, padding: '15px 18px', border: '1px dashed #26343F',
                            borderRadius: 12, color: '#8195A6', flexWrap: 'wrap' }}>
                <span style={{ fontSize: 13.5 }}>
                  + <b style={{ color: '#EAF0F5' }}>{vista.resto_rcas} RCAs</b> abaixo do ritmo,
                  individualmente menores — detalhe no painel completo
                </span>
                <span style={{ ...mono, fontWeight: 600, color: '#EAF0F5', fontSize: 15 }}>
                  R$ {brl(vista.resto_valor)}
                </span>
              </div>
            )}
          </>
        )}

        {/* ── onde o ano fecha ──
            Três cenários, não um. Projeção com número único vira promessa; com
            a faixa, quem lê enxerga o intervalo e sabe onde está a aposta.
            A sazonalidade vem do ano passado como molde — regra de três sobre
            dias decorridos daria número errado com cara de precisão. */}
        {d.projecao && d.projecao.ano_anterior > 0 && (
          <div style={{ marginTop: 36 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                          marginBottom: 16, flexWrap: 'wrap', gap: 8 }}>
              <h2 style={{ ...display, fontWeight: 700, fontSize: 16, margin: 0 }}>
                Onde o ano fecha
              </h2>
              <span style={{ fontSize: 12.5, color: '#5E7080' }}>
                sazonalidade de {d.projecao.ano_ant} como molde
              </span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 1, background: '#26343F',
                          border: '1px solid #26343F', borderRadius: 14, overflow: 'hidden' }}>
              {[
                { rot: 'Piso',        nota: `resto do ano repete ${d.projecao.ano_ant}`, v: d.projecao.piso },
                { rot: 'Ritmo atual', nota: 'resto do ano como o mês corrente',          v: d.projecao.ritmo },
                { rot: 'Conservador', nota: 'mantém o crescimento acumulado',            v: d.projecao.conservador },
              ].map(c => (
                <div key={c.rot} style={{ background: '#15202D', padding: '16px 18px',
                                          display: 'flex', gap: 16, alignItems: 'center' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 600, fontSize: 15 }}>{c.rot}</div>
                    <div style={{ fontSize: 12.5, color: '#8195A6', marginTop: 4 }}>{c.nota}</div>
                  </div>
                  <div style={{ textAlign: 'right' }}>
                    <div style={{ ...mono, fontWeight: 600, fontSize: 17 }}>R$ {brl(c.v)}</div>
                    <div style={{ fontSize: 11.5, color: '#5E7080', marginTop: 3 }}>
                      {((c.v / d.projecao!.ano_anterior - 1) * 100).toFixed(1)}% vs {d.projecao!.ano_ant}
                    </div>
                  </div>
                </div>
              ))}
              <div style={{ background: '#1B2836', padding: '14px 18px', display: 'flex',
                            gap: 16, alignItems: 'center' }}>
                <div style={{ flex: 1, color: '#8195A6', fontSize: 13.5 }}>
                  {d.projecao.ano_ant} fechado
                </div>
                <div style={{ ...mono, fontWeight: 600, fontSize: 15, color: '#8195A6' }}>
                  R$ {brl(d.projecao.ano_anterior)}
                </div>
              </div>
            </div>

            {/* Os dois percentuais vivem na mesma tela e divergem por escopo.
                Sem esta nota, o leitor conclui que um deles está errado. */}
            <div style={{ fontSize: 12, color: '#5E7080', marginTop: 12, lineHeight: 1.65 }}>
              Acumulado até {['','janeiro','fevereiro','março','abril','maio','junho','julho',
                'agosto','setembro','outubro','novembro','dezembro'][d.projecao.ultimo_mes]}:{' '}
              <b style={{ color: '#8195A6' }}>{d.projecao.crescimento_pct.toFixed(1)}%</b> do mesmo
              período de {d.projecao.ano_ant}. O mês corrente projeta{' '}
              <b style={{ color: '#8195A6' }}>{d.projecao.mes_pct.toFixed(1)}%</b> do mesmo mês — se
              ele desacelerou, o ano tende ao piso.
              <br />
              Estes percentuais são da <b style={{ color: '#8195A6' }}>empresa inteira</b>, incluindo
              códigos que faturavam no ano passado e hoje não operam. O percentual do quadro acima
              conta só quem está em campo, e por isso é maior.
            </div>
          </div>
        )}

        {/* O semáforo de três cartões saiu: a frase do topo já dá o número que
            importa ("95 de 762 abaixo de 70%"), e amarelo e verde ocupavam
            espaço sem mudar decisão nenhuma. Menos bloco, leitura mais rápida. */}

        {/* No modo público o botão some: ele leva ao painel, que exige login,
            e uma tela de senha em cima de um link "que não pede senha" lê como
            defeito. Quem quiser o detalhe entra pelo app. */}
        {!publico && (
          <Link to="/farol/v2"
                style={{ display: 'block', textAlign: 'center', marginTop: 28, background: '#E8A33D',
                         color: '#0E1621', padding: '14px 20px', borderRadius: 8, textDecoration: 'none',
                         ...display, fontWeight: 700, fontSize: 14 }}>
            Abrir o painel completo
          </Link>
        )}

        {/* A metodologia fica no rodapé: quem age lê o topo, quem questiona o
            número lê aqui. Esconder a régua é o caminho curto para o gestor
            desconfiar do sistema inteiro na primeira divergência. */}
        <div style={{ marginTop: 28, paddingTop: 20, borderTop: '1px solid #26343F',
                      fontSize: 12, color: '#5E7080', lineHeight: 1.7 }}>
          Ritmo esperado = alvo × (dias úteis decorridos ÷ dias úteis do mês).
          Alvo: <b style={{ color: '#8195A6' }}>
            {d.cobertura.baseline === 'meta' ? 'meta do mês' : 'mesmo mês do ano anterior'}
          </b>. Dias úteis contados pelo faturamento real ({dd} de {dt}, fonte:{' '}
          {d.cobertura.fonte_dias_total}) — considera sábado e feriado sem precisar de calendário.
          Valores em venda líquida. {d.cobertura.rcas_com_meta} dos {d.cobertura.rcas_com_venda} RCAs
          com venda no mês entraram no cálculo; os demais não têm base de comparação.
          <br />
          Este quadro descreve onde está o gap, não avalia desempenho: férias, licença e troca de
          território não são conhecidos pelo sistema.
        </div>

      </div>
    </div>
  )
}
