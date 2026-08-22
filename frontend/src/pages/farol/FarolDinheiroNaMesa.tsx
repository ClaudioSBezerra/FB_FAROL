// FarolDinheiroNaMesa — a página que o resumo semanal linka.
//
// Os mesmos números do e-mail, servidos pelo mesmo motor no backend. Se um dia
// a tela e o e-mail discordarem, é porque alguém duplicou o cálculo — e não
// vai ser aqui: as duas leem /api/v2/farol/dinheiro-na-mesa.
//
// Existe porque a fase 2 é mandar SÓ O LINK por WhatsApp. Um link que abrisse o
// painel genérico obrigaria o gestor a reconstruir o raciocínio sozinho; este
// abre direto no quadro.
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ArrowRight, ExternalLink } from 'lucide-react'
import { fmtBRL } from '@/lib/farolMoney'

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
  cobertura: {
    rcas_com_venda: number
    rcas_com_meta: number
    dias_decorridos: number
    dias_totais: number
    fonte_dias_total: string
    baseline: string
  }
}

const MOTIVO: Record<string, string> = {
  POSITIVACAO: 'positivação abaixo da equipe',
  MIX: 'mix abaixo da equipe',
}

export default function FarolDinheiroNaMesa() {
  const q = useQuery<Resumo>({
    queryKey: ['farol-dinheiro-na-mesa'],
    queryFn: async () => {
      const r = await fetch('/api/v2/farol/dinheiro-na-mesa')
      if (!r.ok) throw new Error('Falha ao carregar o resumo')
      return r.json()
    },
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })

  if (q.isLoading) {
    return <div className="p-8 text-slate-500">Calculando o ritmo do mês…</div>
  }
  if (q.error || !q.data) {
    return <div className="p-8 text-rose-700">Não foi possível carregar o resumo.</div>
  }

  const d = q.data
  const pct = d.ritmo > 0 ? (d.realizado / d.ritmo) * 100 : 0
  const saldo = d.realizado - d.ritmo
  const acima = saldo >= 0
  const grupos = d.grupos ?? []
  const top = d.top_geral ?? []
  const maiorGrupo = grupos.reduce((m, g) => Math.max(m, g.total_mesa), 0)

  return (
    <div className="max-w-5xl mx-auto px-5 py-8 text-slate-800">

      <header className="border-b-2 border-slate-200 pb-5 mb-7">
        <p className="text-xs uppercase tracking-widest font-semibold text-slate-400">
          Farol de Vendas · resumo do mês
        </p>
        <h1 className="text-2xl font-bold mt-2">Dinheiro na mesa · {d.mes}</h1>
        <p className="text-slate-500 mt-1 text-sm">
          {d.nome}, este é o quadro de <b className="text-slate-700">{d.escopo}</b>.
        </p>
      </header>

      {/* Destaque + posição do conjunto. Os dois juntos de propósito: o valor
          na mesa soma só quem está atrás, e sozinho ele lê como "estamos
          atrasados" mesmo quando o conjunto está à frente. */}
      <section className="bg-slate-50 border-l-4 border-teal-700 rounded-r p-5 mb-7">
        <div className="text-3xl font-bold tabular-nums leading-none">
          {fmtBRL(d.total_mesa)}
        </div>
        <p className="text-sm text-slate-500 mt-1.5">
          deixados de faturar em relação ao ritmo, somando apenas quem está atrás
        </p>
        {d.ritmo > 0 && (
          <p className="text-sm text-slate-600 mt-3 pt-3 border-t border-slate-200">
            No conjunto, {d.escopo} está em{' '}
            <b className={acima ? 'text-emerald-700' : 'text-orange-700'}>
              {pct.toFixed(0)}%
            </b>{' '}
            do ritmo — {acima ? 'acima' : 'abaixo'} de{' '}
            <b>{fmtBRL(Math.abs(saldo))}</b> do esperado para esta altura do mês.
          </p>
        )}
      </section>

      <section className="grid grid-cols-1 sm:grid-cols-3 gap-px bg-slate-200 border border-slate-200 rounded overflow-hidden mb-8">
        {[
          { n: d.vermelho, cor: 'text-orange-700', dot: 'bg-orange-600', lbl: 'abaixo de 70% do ritmo' },
          { n: d.amarelo, cor: 'text-amber-700', dot: 'bg-amber-500', lbl: 'entre 70% e 90%' },
          { n: d.verde, cor: 'text-emerald-700', dot: 'bg-emerald-600', lbl: 'no ritmo ou acima' },
        ].map(s => (
          <div key={s.lbl} className="bg-white p-4">
            <div className={`text-2xl font-bold tabular-nums ${s.cor}`}>{s.n}</div>
            <div className="text-xs text-slate-500 mt-1 flex items-center gap-2">
              <span className={`w-2 h-2 rounded-full ${s.dot}`} />{s.lbl}
            </div>
          </div>
        ))}
      </section>

      {grupos.length > 0 && (
        <section className="mb-8">
          <h2 className="text-base font-bold mb-1">
            {d.persona === 'ggv' ? 'Por supervisor' : 'Por GGV'}
          </h2>
          <p className="text-xs text-slate-500 mb-3">
            Clique para abrir o painel já filtrado nessa equipe.
          </p>
          <div className="border border-slate-200 rounded divide-y divide-slate-100">
            {grupos.map(g => (
              <a key={g.cod} href={g.link}
                 className="flex items-center gap-4 px-4 py-3 hover:bg-slate-50 group">
                <div className="flex-1 min-w-0">
                  <div className="font-semibold text-teal-800 group-hover:underline truncate">
                    {g.nome} <span className="text-slate-400 font-normal text-xs">· {g.cod}</span>
                  </div>
                  <div className="text-xs text-slate-500 mt-0.5">
                    {g.rcas} RCAs · {g.vermelhos} abaixo de 70%
                  </div>
                  {/* Barra proporcional ao maior: dá a leitura de peso relativo
                      sem obrigar a comparar números um a um. */}
                  <div className="h-1 bg-slate-100 rounded mt-2 overflow-hidden">
                    <div className="h-full bg-teal-700/60 rounded"
                         style={{ width: maiorGrupo > 0 ? `${(g.total_mesa / maiorGrupo) * 100}%` : '0%' }} />
                  </div>
                </div>
                <div className="text-right font-bold tabular-nums whitespace-nowrap">
                  {fmtBRL(g.total_mesa)}
                </div>
                <ExternalLink className="h-4 w-4 text-slate-300 group-hover:text-teal-700 shrink-0" />
              </a>
            ))}
          </div>
        </section>
      )}

      {top.length > 0 && (
        <section className="mb-8">
          <h2 className="text-base font-bold mb-1">Onde está o dinheiro</h2>
          <p className="text-xs text-slate-500 mb-3">
            Ordenado por reais, não por percentual: o RCA grande a 90% do ritmo
            pesa mais que o pequeno a 50%.
          </p>
          <div className="border border-slate-200 rounded divide-y divide-slate-100">
            {top.map((x, i) => (
              <div key={`${x.cod_rca}-${i}`} className="flex items-center gap-4 px-4 py-3">
                <span className="text-slate-400 tabular-nums w-5">{i + 1}</span>
                <div className="flex-1 min-w-0">
                  <div className="font-medium truncate">{x.nome_rca}</div>
                  <div className="text-xs text-slate-500 mt-0.5">
                    {MOTIVO[x.motivo] ?? 'volume geral'} · {x.atingimento.toFixed(0)}% do ritmo
                  </div>
                </div>
                <div className="text-right font-bold tabular-nums whitespace-nowrap">
                  {fmtBRL(x.dinheiro_mesa)}
                </div>
              </div>
            ))}
          </div>
          {d.resto_rcas > 0 && (
            <p className="text-sm text-slate-500 border border-dashed border-slate-300 rounded px-4 py-3 mt-3">
              + <b className="text-slate-700">{d.resto_rcas} RCAs</b> abaixo do ritmo,
              individualmente menores — <b className="text-slate-700">{fmtBRL(d.resto_valor)}</b> somados.
            </p>
          )}
        </section>
      )}

      <Link to="/farol/v2"
            className="inline-flex items-center gap-2 bg-teal-800 hover:bg-teal-900 text-white px-5 py-2.5 rounded text-sm font-medium">
        Abrir o painel completo <ArrowRight className="h-4 w-4" />
      </Link>

      {/* Metodologia no rodapé: quem age lê o topo, quem questiona o número lê
          aqui. Esconder a régua é o caminho curto para o gestor desconfiar do
          sistema inteiro na primeira divergência. */}
      <footer className="mt-9 pt-5 border-t border-slate-200 text-xs text-slate-500 leading-relaxed">
        Ritmo esperado = alvo × (dias úteis decorridos ÷ dias úteis do mês).
        Alvo: <b>{d.cobertura.baseline === 'meta' ? 'meta do mês' : 'mesmo mês do ano anterior'}</b>.
        Dias úteis contados pelo faturamento real ({d.cobertura.dias_decorridos} de{' '}
        {d.cobertura.dias_totais}, fonte: {d.cobertura.fonte_dias_total}) — considera sábado
        e feriado sem precisar de calendário. Valores em venda líquida.{' '}
        {d.cobertura.rcas_com_meta} dos {d.cobertura.rcas_com_venda} RCAs com venda no mês
        entraram no cálculo; os demais não têm base de comparação.
        <br />
        Este quadro descreve onde está o gap, não avalia desempenho: férias, licença e troca
        de território não são conhecidos pelo sistema.
      </footer>
    </div>
  )
}
