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
  const { dias_decorridos: dd, dias_totais: dt } = d.cobertura
  // No ano fechado o alvo JÁ é cheio — não há mês pela metade para ratear.
  const alvoMes = verAno ? vista.ritmo : (dd > 0 ? (d.ritmo * dt) / dd : 0)
  const pctMes = alvoMes > 0 ? (vista.realizado / alvoMes) * 100 : 0
  const pctRitmo = vista.ritmo > 0 ? (vista.realizado / vista.ritmo) * 100 : 0
  const saldo = vista.realizado - vista.ritmo
  const acima = saldo >= 0
  const marcaEsperada = verAno ? 100 : (dt > 0 ? (dd / dt) * 100 : 0)

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

        {/* ── comparativo mês × ano ──
            Os dois lado a lado antes de qualquer detalhe. O mês responde "como
            estamos agora"; o ano, "como está o acumulado". Vistos separados em
            telas diferentes, viram duas conversas; juntos, viram uma. */}
        {ano && (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(240px,1fr))',
                        gap: 1, background: '#26343F', border: '1px solid #26343F',
                        borderRadius: 14, overflow: 'hidden', marginBottom: 30 }}>
            {[
              { id: 'mes' as const, rot: d.mes, real: d.realizado, alvo: d.ritmo,
                mesa: d.total_mesa, nota: `${dd} de ${dt} dias úteis` },
              { id: 'ano' as const, rot: ano.rotulo, real: ano.realizado, alvo: ano.alvo,
                mesa: ano.total_mesa, nota: 'meses fechados' },
            ].map(c => {
              const pct = c.alvo > 0 ? (c.real / c.alvo) * 100 : 0
              const ativo = periodo === c.id
              return (
                <button key={c.id} onClick={() => setPeriodo(c.id)}
                        style={{ background: ativo ? '#1B2836' : '#15202D', border: 0,
                                 borderTop: `3px solid ${ativo ? '#E8A33D' : 'transparent'}`,
                                 padding: '18px 20px', textAlign: 'left', cursor: 'pointer',
                                 color: 'inherit', font: 'inherit' }}>
                  <div style={{ ...display, fontWeight: 700, fontSize: 11, letterSpacing: 1.5,
                                textTransform: 'uppercase',
                                color: ativo ? '#E8A33D' : '#5E7080' }}>{c.rot}</div>
                  <div style={{ ...mono, fontWeight: 600, fontSize: 22, marginTop: 8 }}>
                    R$ {brl(c.mesa)}
                  </div>
                  <div style={{ fontSize: 12.5, color: '#8195A6', marginTop: 4 }}>
                    na mesa · <b style={{ color: pct >= 100 ? '#3DC98B' : '#E5544B' }}>
                      {pct.toFixed(0)}%
                    </b> do alvo
                  </div>
                  <div style={{ fontSize: 11.5, color: '#5E7080', marginTop: 6 }}>
                    R$ {brl(c.real)} de R$ {brl(c.alvo)} · {c.nota}
                  </div>
                </button>
              )
            })}
          </div>
        )}

        {/* ── herói ── */}
        <div style={{ ...display, fontWeight: 700, letterSpacing: '2.5px', fontSize: 12,
                      color: '#E8A33D', textTransform: 'uppercase', marginBottom: 14 }}>
          Deixado de faturar · {vista.rotulo}
        </div>
        <div style={{ ...mono, fontWeight: 600, fontSize: 'clamp(40px,11vw,104px)',
                      lineHeight: .92, letterSpacing: '-1px', wordBreak: 'break-word' }}>
          <span style={{ color: '#E8A33D', fontSize: '.42em', verticalAlign: '.28em', marginRight: '.12em' }}>R$</span>
          {brl(vista.total_mesa)}
        </div>
        <div style={{ height: 4, borderRadius: 3, marginTop: 18,
                      background: 'linear-gradient(90deg,#E8A33D,rgba(232,163,61,0))',
                      width: `${Math.min(100, 100 - pctRitmo / 2)}%` }} />
        <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap', marginTop: 16,
                      color: '#8195A6', fontSize: 14, alignItems: 'center' }}>
          <span>somando apenas quem está atrás do ritmo</span>
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, background: '#15202D',
                         border: '1px solid #26343F', borderRadius: 100, padding: '5px 13px', fontSize: 12.5 }}>
            <span style={{ width: 7, height: 7, borderRadius: '50%', background: '#E5544B' }} />
            {vista.vermelho} RCAs abaixo de 70%
          </span>
        </div>

        {/* ── progresso do mês ──
            A marca âmbar é onde a equipe DEVERIA estar hoje. Sem ela, a barra
            só diz "faltam X%" e some a informação que importa: se o atraso é
            do calendário ou do desempenho. */}
        <div style={{ margin: '30px 0 40px' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline',
                        marginBottom: 9, fontSize: 13, color: '#8195A6', gap: 12, flexWrap: 'wrap' }}>
            <span>Realizado <b style={{ color: '#EAF0F5' }}>R$ {brl(vista.realizado)}</b> de <b style={{ color: '#EAF0F5' }}>R$ {brl(alvoMes)}</b></span>
            <span><b style={{ color: acima ? '#3DC98B' : '#E5544B' }}>{pctRitmo.toFixed(0)}%</b> do {verAno ? 'alvo' : 'ritmo'}{!verAno && ` · ${dd} de ${dt} dias úteis`}</span>
          </div>
          <div style={{ height: 12, background: '#15202D', border: '1px solid #26343F',
                        borderRadius: 100, overflow: 'hidden', position: 'relative' }}>
            <div style={{ height: '100%', width: `${Math.min(100, pctMes)}%`, borderRadius: 100,
                          background: 'linear-gradient(90deg,#2ea877,#3DC98B)' }} />
            <div style={{ position: 'absolute', top: -5, bottom: -5, width: 2, background: '#E8A33D',
                          left: `${Math.min(100, marcaEsperada)}%`, opacity: .55 }} />
          </div>
          <div style={{ fontSize: 12.5, color: '#5E7080', marginTop: 8 }}>
            {verAno ? 'Meses fechados: alvo cheio, sem rateio de dias. ' : 'A marca âmbar é onde a equipe deveria estar hoje. '}{acima
              ? `Está R$ ${brl(Math.abs(saldo))} à frente.`
              : `Faltam R$ ${brl(Math.abs(saldo))} para alcançá-la.`}
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

        {/* ── semáforo ── */}
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(180px,1fr))',
                      gap: 1, background: '#26343F', border: '1px solid #26343F',
                      borderRadius: 14, overflow: 'hidden', marginTop: 36 }}>
          {[
            { n: vista.vermelho, cor: '#E5544B', lbl: 'Vermelho · abaixo de 70% do ritmo' },
            { n: vista.amarelo,  cor: '#E8C13D', lbl: 'Amarelo · entre 70% e 90%' },
            { n: vista.verde,    cor: '#3DC98B', lbl: 'Verde · no ritmo ou acima' },
          ].map(s => (
            <div key={s.lbl} style={{ background: '#15202D', padding: '18px 20px' }}>
              <div style={{ ...mono, fontWeight: 600, fontSize: 30, color: s.cor }}>{s.n}</div>
              <div style={{ fontSize: 12.5, color: '#8195A6', marginTop: 4, display: 'flex',
                            alignItems: 'center', gap: 7 }}>
                <span style={{ width: 8, height: 8, borderRadius: '50%', background: s.cor }} />
                {s.lbl}
              </div>
            </div>
          ))}
        </div>

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
