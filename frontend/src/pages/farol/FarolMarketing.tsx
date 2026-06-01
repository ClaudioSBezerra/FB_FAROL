import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { TrendingUp, TrendingDown, Minus, Users, UserCheck, UserX, Package } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

// ─── Tipos ────────────────────────────────────────────────────────────────────

interface MktCard {
  key: string
  label: string
  fornec?: string
  nome_fornec?: string
  qt_clientes: number
  qt_cli_ant: number
  delta_pct: number
  pvenda: number
  faturado: number
  transmitido: number
  penetr_pct: number
  mix: number
  positivado: boolean
}

interface MktKPI {
  total_base_cli: number
  total_ativos: number
  total_inativos: number
  taxa_positivacao: number
  avg_mix: number
  total_pvenda: number
  total_faturado: number
  total_transmitido: number
}

interface ClienteInativo {
  key: string
  label: string
  pvenda: number
  transmitido: number
}

interface MktResponse {
  cards: MktCard[]
  kpi: MktKPI
  clientes_inativos: ClienteInativo[]
  periodo: {
    ref_ano: number; ref_mes: number
    label: string; comp_mode: string
    cur_label: string; ant_label: string
  }
  periodos: string[]
  view: string
}

// ─── Utilitários ──────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v >= 1_000_000_000) return 'R$ ' + (v / 1_000_000_000).toFixed(2).replace('.', ',') + 'B'
  if (v >= 1_000_000)     return 'R$ ' + (v / 1_000_000).toFixed(1).replace('.', ',') + 'M'
  if (v >= 1_000)         return 'R$ ' + (v / 1_000).toFixed(0).replace(/\B(?=(\d{3})+(?!\d))/g, '.') + 'K'
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0 })
}
function fmtPct(v: number) { return v.toFixed(1) + '%' }
function fmtNum(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 1 }) }

function parsePeriodo(s: string) {
  const [y, m] = s.split('-')
  return { ano: +y, mes: +m }
}
const MES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']
function fmtMesAno(ano: number, mes: number) { return `${MES[mes] ?? mes}/${String(ano).slice(2)}` }

// Cor de penetração: verde ≥ 50%, amarelo ≥ 20%, vermelho < 20%
function corPenetr(pct: number): { bar: string; text: string } {
  if (pct >= 50) return { bar: 'bg-emerald-500', text: 'text-emerald-600' }
  if (pct >= 20) return { bar: 'bg-amber-400',   text: 'text-amber-600'  }
  return           { bar: 'bg-red-500',     text: 'text-red-600'    }
}

const MODE_DESC: Record<string, string> = {
  yoy: 'Compara o mesmo mês do ano atual com o mesmo mês do ano anterior.',
  ytd: 'Extrapola o acumulado do ano atual para projetar o total anual.',
  mom: 'Compara o mês atual com o mês imediatamente anterior.',
}

// ─── Hook de dados ─────────────────────────────────────────────────────────────

function useMktCards(view: string, compMode: string, refAno: number, refMes: number, compAno: number, compMes: number) {
  return useQuery<MktResponse>({
    queryKey: ['marketing-cards', view, compMode, refAno, refMes, compAno, compMes],
    queryFn: async () => {
      const p = new URLSearchParams({
        view, comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(compAno > 0 && compMes > 0 && { comp_ano: String(compAno), comp_mes: String(compMes) }),
      })
      const r = await fetch(`/api/v2/marketing/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dados de Marketing')
      return r.json()
    },
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// ─── Hero Band ────────────────────────────────────────────────────────────────

function MktHeroBand({ kpi, periodo }: { kpi: MktKPI; periodo: MktResponse['periodo'] }) {
  const cx = 80, cy = 80, r = 56, sw = 10
  const total = 2 * Math.PI * r
  const arc = total * 0.75
  const filled = arc * Math.min(kpi.taxa_positivacao / 100, 1)
  const col = kpi.taxa_positivacao >= 75 ? '#10b981' : kpi.taxa_positivacao >= 50 ? '#f59e0b' : '#ef4444'

  return (
    <div className="relative bg-gradient-to-br from-slate-950 via-slate-900 to-slate-900 rounded-2xl overflow-hidden mb-6 shadow-2xl">
      <div className="absolute inset-0 opacity-[0.04]"
        style={{
          backgroundImage: 'linear-gradient(rgba(255,255,255,.5) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.5) 1px,transparent 1px)',
          backgroundSize: '40px 40px'
        }}
      />
      <div className="relative px-8 py-8">
        {/* Header */}
        <div className="flex items-start justify-between mb-6">
          <div>
            <p className="text-slate-400 text-xs font-medium uppercase tracking-widest mb-1">Painel Marketing</p>
            <p className="text-white/80 text-sm font-medium">{periodo.label}</p>
          </div>
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1 text-xs font-semibold text-emerald-400 bg-emerald-400/10 border border-emerald-400/20 px-2 py-0.5 rounded-full">
              <UserCheck className="h-3 w-3" /> {kpi.total_ativos} ativos
            </span>
            <span className="flex items-center gap-1 text-xs font-semibold text-red-400 bg-red-400/10 border border-red-400/20 px-2 py-0.5 rounded-full">
              <UserX className="h-3 w-3" /> {kpi.total_inativos} inativos
            </span>
          </div>
        </div>

        {/* Métricas */}
        <div className="flex items-end gap-10 flex-wrap">
          {/* Gauge de positivação */}
          <div className="relative flex-shrink-0">
            <svg width="160" height="115" viewBox="0 0 160 115" className="overflow-visible">
              <defs>
                <filter id="mkt-glow" x="-30%" y="-30%" width="160%" height="160%">
                  <feGaussianBlur stdDeviation="3" result="blur" />
                  <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
                </filter>
              </defs>
              <circle cx={cx} cy={cy} r={r} fill="none"
                stroke="rgba(255,255,255,0.07)" strokeWidth={sw}
                strokeDasharray={`${arc} ${total - arc}`} strokeLinecap="round"
                transform={`rotate(-225 ${cx} ${cy})`} />
              {filled > 1 && (
                <circle cx={cx} cy={cy} r={r} fill="none"
                  stroke={col} strokeWidth={sw}
                  strokeDasharray={`${filled} ${total - filled}`} strokeLinecap="round"
                  transform={`rotate(-225 ${cx} ${cy})`}
                  filter="url(#mkt-glow)"
                  style={{ transition: 'stroke-dasharray 0.8s cubic-bezier(0.4,0,0.2,1)' }}
                />
              )}
            </svg>
            <div className="absolute inset-0 flex flex-col items-center justify-center pt-10">
              <span className="text-3xl font-black tabular-nums" style={{ color: col }}>
                {fmtPct(kpi.taxa_positivacao)}
              </span>
              <span className="text-slate-400 text-[10px] uppercase tracking-widest font-semibold mt-0.5">positivação</span>
            </div>
          </div>

          <div className="w-px h-20 bg-white/10 self-center hidden md:block" />

          {/* Clientes */}
          <div className="flex-shrink-0">
            <div className="flex items-center gap-1 mb-1">
              <p className="text-slate-400 text-xs uppercase tracking-widest">Base de Clientes</p>
            </div>
            <p className="text-white text-4xl font-black tabular-nums leading-none">{kpi.total_base_cli.toLocaleString('pt-BR')}</p>
            <div className="flex gap-3 mt-2">
              <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 inline-block" />
                {kpi.total_ativos} compraram
              </span>
              <span className="text-xs text-red-400 font-semibold flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full bg-red-400 inline-block" />
                {kpi.total_inativos} não compraram
              </span>
            </div>
          </div>

          <div className="w-px h-20 bg-white/10 self-center hidden md:block" />

          {/* KPIs secundários */}
          <div className="flex gap-8 flex-wrap">
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Mix Médio</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtNum(kpi.avg_mix)}</p>
              <p className="text-slate-500 text-[11px] mt-0.5">itens/cliente</p>
            </div>
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Faturado</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtBRL(kpi.total_faturado)}</p>
            </div>
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Transmitido</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtBRL(kpi.total_transmitido)}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Ranking de Penetração ────────────────────────────────────────────────────

function MktRanking({ cards, view }: { cards: MktCard[]; view: string }) {
  const maxPenetr = Math.max(...cards.map(c => c.penetr_pct), 1)
  const medals = ['🥇', '🥈', '🥉']

  const headerLabel = view === 'produto' ? 'Produto'
    : view === 'cliente' ? 'Cliente'
    : 'Indústria'

  const metricLabel = view === 'cliente' ? 'Mix Produtos' : 'Clientes Positivados'

  return (
    <div className="bg-white rounded-2xl shadow-sm border border-slate-100 overflow-hidden">
      <div className="flex items-center justify-between px-6 py-4 border-b border-slate-50">
        <div>
          <h2 className="text-sm font-bold text-slate-800 uppercase tracking-wider">
            Penetração de Mercado
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Por <span className="font-black text-red-600 uppercase tracking-wide">{headerLabel}</span>
            {' '}· ordenado por {metricLabel.toLowerCase()}
          </p>
        </div>
        <div className="flex gap-3 text-[11px]">
          <span className="flex items-center gap-1 text-emerald-600 font-medium">
            <span className="w-2 h-2 rounded-full bg-emerald-500 inline-block" /> ≥ 50%
          </span>
          <span className="flex items-center gap-1 text-amber-600 font-medium">
            <span className="w-2 h-2 rounded-full bg-amber-400 inline-block" /> ≥ 20%
          </span>
          <span className="flex items-center gap-1 text-red-500 font-medium">
            <span className="w-2 h-2 rounded-full bg-red-500 inline-block" /> &lt; 20%
          </span>
        </div>
      </div>

      <div className="divide-y divide-slate-50">
        {cards.map((card, idx) => {
          const barW = Math.min((card.penetr_pct / maxPenetr) * 100, 100)
          const { bar, text } = corPenetr(card.penetr_pct)
          const d = card.delta_pct

          return (
            <div key={card.key} className="flex items-center gap-4 px-6 py-3.5">
              {/* Rank */}
              <div className="w-8 flex-shrink-0 text-center">
                {idx < 3
                  ? <span className="text-xl">{medals[idx]}</span>
                  : <span className="text-sm font-bold text-slate-300">{idx + 1}</span>}
              </div>

              {/* Nome */}
              <div className="w-44 flex-shrink-0">
                <p className="text-sm font-semibold text-slate-800 truncate">{card.label}</p>
                {card.nome_fornec && (
                  <p className="text-[10px] text-slate-400 truncate">{card.nome_fornec}</p>
                )}
                {!card.nome_fornec && (
                  <p className="text-[10px] text-slate-400">{card.key}</p>
                )}
              </div>

              {/* Barra de penetração */}
              <div className="flex-1 min-w-0">
                <div className="h-2.5 bg-slate-100 rounded-full overflow-hidden">
                  <div className={`h-full ${bar} rounded-full transition-all duration-700`}
                    style={{ width: `${barW}%` }} />
                </div>
              </div>

              {/* % Penetração */}
              <div className="w-16 flex-shrink-0 text-right">
                <span className={`text-base font-black tabular-nums ${text}`}>
                  {view === 'cliente'
                    ? fmtNum(card.mix)
                    : fmtPct(card.penetr_pct)}
                </span>
                {view !== 'cliente' && (
                  <p className="text-[10px] text-slate-400 tabular-nums">{card.qt_clientes} cli</p>
                )}
              </div>

              {/* Faturado */}
              <div className="w-28 flex-shrink-0 text-right hidden sm:block">
                <p className="text-sm font-semibold text-slate-700 tabular-nums">{fmtBRL(card.faturado)}</p>
                <p className="text-[10px] text-slate-400">faturado</p>
              </div>

              {/* Delta vs período anterior */}
              {view !== 'cliente' && (
                <div className="w-20 flex-shrink-0 text-right hidden md:flex items-center justify-end">
                  {d > 0.5 ? (
                    <span className="flex items-center gap-0.5 text-emerald-600 text-xs font-bold">
                      <TrendingUp className="h-3.5 w-3.5" />{'+' + d.toFixed(1) + '%'}
                    </span>
                  ) : d < -0.5 ? (
                    <span className="flex items-center gap-0.5 text-red-500 text-xs font-bold">
                      <TrendingDown className="h-3.5 w-3.5" />{d.toFixed(1) + '%'}
                    </span>
                  ) : (
                    <span className="flex items-center gap-0.5 text-slate-400 text-xs">
                      <Minus className="h-3.5 w-3.5" />estável
                    </span>
                  )}
                </div>
              )}

              {/* Mix (apenas cliente) */}
              {view === 'cliente' && (
                <div className="w-20 flex-shrink-0 text-right hidden md:block">
                  <p className="text-xs font-semibold text-slate-600 tabular-nums">
                    {fmtNum(card.mix)} itens
                  </p>
                  <p className="text-[10px] text-slate-400">mix</p>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ─── Clientes Inativos ────────────────────────────────────────────────────────

function ClientesInativos({ clientes }: { clientes: ClienteInativo[] }) {
  if (clientes.length === 0) return null
  return (
    <div className="bg-white rounded-2xl shadow-sm border border-red-100 overflow-hidden mt-6">
      <div className="flex items-center gap-2 px-6 py-4 border-b border-red-50 bg-red-50/40">
        <UserX className="h-4 w-4 text-red-500" />
        <div>
          <h2 className="text-sm font-bold text-red-700">Clientes Inativos no Período</h2>
          <p className="text-xs text-red-400 mt-0.5">
            {clientes.length} clientes com pedidos transmitidos mas <strong>sem faturamento</strong> — oportunidade de reativação
          </p>
        </div>
      </div>
      <div className="divide-y divide-slate-50 max-h-72 overflow-y-auto">
        {clientes.map((c, idx) => (
          <div key={c.key} className="flex items-center gap-4 px-6 py-3 hover:bg-red-50/20 transition-colors">
            <span className="w-6 text-center text-xs font-bold text-slate-300">{idx + 1}</span>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-semibold text-slate-800 truncate">{c.label}</p>
              <p className="text-[10px] text-slate-400">{c.key}</p>
            </div>
            <div className="text-right">
              <p className="text-sm font-semibold text-amber-600 tabular-nums">{fmtBRL(c.transmitido)}</p>
              <p className="text-[10px] text-slate-400">valor transmitido</p>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

// ─── FarolMarketing ───────────────────────────────────────────────────────────

export default function FarolMarketing() {
  const [view, setView]         = useState<'produto' | 'cliente' | 'fornec'>('produto')
  const [compMode, setCompMode] = useState('yoy')
  const [refAno, setRefAno]     = useState(0)
  const [refMes, setRefMes]     = useState(0)
  const [compAno, setCompAno]   = useState(0)
  const [compMes, setCompMes]   = useState(0)
  const [search, setSearch]     = useState('')

  const { data, isLoading, error } = useMktCards(
    view, compMode, refAno, refMes,
    compMode === 'mom' ? compAno : 0,
    compMode === 'mom' ? compMes : 0,
  )

  // Auto-seleciona período ao receber dados pela primeira vez
  if (data && refAno === 0 && data.periodo.ref_ano) {
    setRefAno(data.periodo.ref_ano)
    setRefMes(data.periodo.ref_mes)
  }

  const periodos = data?.periodos ?? []

  const visibleCards = useMemo(() => {
    if (!data?.cards) return []
    const term = search.trim().toLowerCase()
    if (!term) return data.cards
    return data.cards.filter(c =>
      c.label.toLowerCase().includes(term) ||
      (c.nome_fornec ?? '').toLowerCase().includes(term)
    )
  }, [data?.cards, search])

  return (
    <div className="min-h-full">
      {/* ── Controles ─────────────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-5">
        {/* Visão */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {([
            { id: 'produto'  as const, label: 'Por Produto',   icon: <Package className="h-3 w-3" /> },
            { id: 'cliente'  as const, label: 'Por Cliente',   icon: <Users className="h-3 w-3" /> },
            { id: 'fornec'   as const, label: 'Por Indústria', icon: null },
          ]).map(v => (
            <button key={v.id} onClick={() => { setView(v.id); setSearch('') }}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium transition-colors ${
                view === v.id ? 'bg-primary text-white' : 'text-slate-600 hover:bg-slate-50'
              }`}>
              {v.icon}{v.label}
            </button>
          ))}
        </div>

        {/* Comparação */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {[
            { id: 'yoy', label: 'Ano a Ano' },
            { id: 'ytd', label: 'Acumulado Anual' },
            { id: 'mom', label: 'Mês a Mês' },
          ].map(m => (
            <TooltipProvider key={m.id} delayDuration={400}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button onClick={() => setCompMode(m.id)}
                    className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                      compMode === m.id ? 'bg-slate-800 text-white' : 'text-slate-600 hover:bg-slate-50'
                    }`}>
                    {m.label}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom" className="max-w-xs text-xs leading-relaxed">
                  {MODE_DESC[m.id]}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          ))}
        </div>

        {/* Período */}
        {periodos.length > 0 && (
          <select
            value={refAno > 0 ? `${refAno}-${String(refMes).padStart(2, '0')}` : ''}
            onChange={e => { const p = parsePeriodo(e.target.value); setRefAno(p.ano); setRefMes(p.mes) }}
            className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          >
            {periodos.map(p => {
              const { ano, mes } = parsePeriodo(p)
              return <option key={p} value={p}>{fmtMesAno(ano, mes)}</option>
            })}
          </select>
        )}

        {/* Comparar com (mom) */}
        {compMode === 'mom' && periodos.length > 0 && (
          <>
            <span className="text-xs text-slate-400">comparar com</span>
            <select
              value={compAno > 0 ? `${compAno}-${String(compMes).padStart(2, '0')}` : ''}
              onChange={e => {
                if (!e.target.value) { setCompAno(0); setCompMes(0); return }
                const p = parsePeriodo(e.target.value); setCompAno(p.ano); setCompMes(p.mes)
              }}
              className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
            >
              <option value="">Mês anterior (auto)</option>
              {periodos.map(p => {
                const { ano, mes } = parsePeriodo(p)
                return <option key={p} value={p}>{fmtMesAno(ano, mes)}</option>
              })}
            </select>
          </>
        )}

        {/* Busca */}
        {data && data.cards.length > 0 && (
          <input type="text" value={search} onChange={e => setSearch(e.target.value)}
            placeholder={`Buscar ${view === 'produto' ? 'produto' : view === 'cliente' ? 'cliente' : 'indústria'}...`}
            className="h-8 px-3 rounded-lg border border-slate-200 bg-white text-xs text-slate-700 shadow-sm placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-primary/30 w-44 ml-auto"
          />
        )}
      </div>

      {/* ── Loading ────────────────────────────────────────────────────── */}
      {isLoading && (
        <div className="space-y-4">
          <div className="bg-slate-900 rounded-2xl h-44 animate-pulse" />
          <div className="bg-white rounded-2xl h-64 animate-pulse border border-slate-100" />
        </div>
      )}

      {/* ── Erro ───────────────────────────────────────────────────────── */}
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-6 text-red-700 text-sm">
          <p className="font-semibold mb-1">Erro ao carregar painel de marketing</p>
          <p>{(error as Error).message}</p>
        </div>
      )}

      {/* ── Sem dados ──────────────────────────────────────────────────── */}
      {!isLoading && !error && data?.cards.length === 0 && (
        <div className="bg-gradient-to-br from-slate-950 to-slate-900 rounded-2xl p-16 text-center shadow-2xl">
          <p className="text-slate-400 text-sm">Nenhum dado disponível para o período selecionado.</p>
        </div>
      )}

      {/* ── Conteúdo ───────────────────────────────────────────────────── */}
      {!isLoading && !error && data && data.cards.length > 0 && (
        <>
          <MktHeroBand kpi={data.kpi} periodo={data.periodo} />
          <MktRanking cards={visibleCards} view={view} />
          <ClientesInativos clientes={data.clientes_inativos} />
        </>
      )}
    </div>
  )
}
