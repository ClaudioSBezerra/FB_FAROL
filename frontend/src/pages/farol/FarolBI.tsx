import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { PieChart, Pie, Cell, Tooltip as ReTooltip, ResponsiveContainer } from 'recharts'
import { RefreshCw } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────

interface CardItem {
  key: string
  label: string
  pct: number
  faturado: number
  positivados: number
  base_cli: number
  positpct: number
  mix: number
}

interface KPI {
  total_atual: number
  total_ant: number
  total_pct: number
  total_faturado: number
  total_transmitido: number
  total_positivados: number
  total_base_cli: number
  total_positpct: number
  avg_mix: number
  verdes: number
  vermelhos: number
}

interface BiResponse {
  cards: CardItem[]
  kpi: KPI
  periodo: {
    ref_ano: number; ref_mes: number
    label: string; cur_label: string; ant_label: string
    fluxo: string
  }
}

// ─── Utils ────────────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v >= 1_000_000_000) return 'R$ ' + (v / 1_000_000_000).toFixed(2).replace('.', ',') + 'B'
  if (v >= 1_000_000)     return 'R$ ' + (v / 1_000_000).toFixed(1).replace('.', ',') + 'M'
  if (v >= 1_000)         return 'R$ ' + (v / 1_000).toFixed(0).replace(/\B(?=(\d{3})+(?!\d))/g, '.') + 'K'
  return 'R$ ' + v.toFixed(0)
}

function fmtPct(v: number) {
  if (v >= 9_999) return '>9999%'
  if (v >= 999.5) return Math.round(v) + '%'
  return v.toFixed(1) + '%'
}

function fmtNum(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 1 }) }

const MES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']

function gaugeColor(pct: number) {
  if (pct >= 100) return '#22c55e'
  if (pct >= 70)  return '#3b82f6'
  if (pct >= 50)  return '#eab308'
  return '#ef4444'
}

// ─── BiClock ──────────────────────────────────────────────────────────────────

function BiClock({ nextRefresh }: { nextRefresh: number }) {
  const [now, setNow] = useState(new Date())
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(t)
  }, [])

  const secsLeft = Math.max(0, Math.ceil((nextRefresh - Date.now()) / 1000))
  const mins = Math.floor(secsLeft / 60)
  const secs = secsLeft % 60

  return (
    <div className="text-right">
      <p className="text-xl font-black tabular-nums text-white">
        {now.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}
      </p>
      <p className="text-[10px] text-slate-600 tabular-nums">
        refresh em {mins}:{String(secs).padStart(2, '0')}
      </p>
    </div>
  )
}

// ─── ArcGaugeDark ─────────────────────────────────────────────────────────────

function ArcGaugeDark({
  gaugeId, arcPct, centerText, label, sublabel, subvalue, color, size = 180,
}: {
  gaugeId: string
  arcPct: number       // 0-100, controls arc fill
  centerText: string   // displayed in center
  label: string
  sublabel: string
  subvalue?: string
  color: string
  size?: number
}) {
  const cx = size / 2, cy = size / 2
  const r = size * 0.37, sw = size * 0.065
  const total = 2 * Math.PI * r
  const arc = total * 0.75
  const filled = arc * Math.min(Math.max(arcPct, 0) / 100, 1)
  const filterId = `bi-glow-${gaugeId}`

  return (
    <div className="flex flex-col items-center">
      {/* Título acima do arco */}
      <p className="text-slate-400 uppercase tracking-widest font-bold mb-2 text-center"
        style={{ fontSize: size * 0.072 }}>
        {label}
      </p>
      <div className="relative" style={{ width: size, height: size * 0.72 }}>
        <svg width={size} height={size * 0.72} viewBox={`0 0 ${size} ${size * 0.72}`} className="overflow-visible">
          <defs>
            <filter id={filterId} x="-30%" y="-30%" width="160%" height="160%">
              <feGaussianBlur stdDeviation="4" result="blur" />
              <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
            </filter>
          </defs>
          {/* Track */}
          <circle cx={cx} cy={cy} r={r} fill="none"
            stroke="rgba(255,255,255,0.07)" strokeWidth={sw}
            strokeDasharray={`${arc} ${total - arc}`} strokeLinecap="round"
            transform={`rotate(-225 ${cx} ${cy})`} />
          {/* Fill */}
          {filled > 0.5 && (
            <circle cx={cx} cy={cy} r={r} fill="none"
              stroke={color} strokeWidth={sw}
              strokeDasharray={`${filled} ${total - filled}`} strokeLinecap="round"
              transform={`rotate(-225 ${cx} ${cy})`}
              filter={`url(#${filterId})`}
              style={{ transition: 'stroke-dasharray 1s cubic-bezier(0.4,0,0.2,1)' }}
            />
          )}
        </svg>
        {/* Percentual no centro — só o número, fonte ajustada para não vazar */}
        <div className="absolute inset-0 flex items-center justify-center overflow-hidden"
          style={{ paddingTop: size * 0.22 }}>
          <span className="font-black tabular-nums leading-none" style={{ fontSize: size * 0.155, color }}>
            {centerText}
          </span>
        </div>
      </div>
      {/* Valores apurados — espaço extra abaixo do arco */}
      <div className="text-center mt-5">
        <p className="text-white font-semibold" style={{ fontSize: size * 0.088 }}>{sublabel}</p>
        {subvalue && (
          <p className="text-slate-500 mt-1" style={{ fontSize: size * 0.07 }}>{subvalue}</p>
        )}
      </div>
    </div>
  )
}

// ─── IndustryDonut ────────────────────────────────────────────────────────────

const DONUT_COLORS = [
  '#6366f1', '#22c55e', '#f59e0b', '#ec4899',
  '#14b8a6', '#f97316', '#8b5cf6', '#06b6d4', '#64748b',
]

function IndustryDonut({ cards }: { cards: CardItem[] }) {
  const sorted = [...cards].sort((a, b) => b.faturado - a.faturado)
  const top8   = sorted.slice(0, 8)
  const outros = sorted.slice(8).reduce((s, c) => s + c.faturado, 0)
  const data = [
    ...top8.map((c, i) => ({ name: c.label, value: c.faturado, color: DONUT_COLORS[i] })),
    ...(outros > 0 ? [{ name: 'Outros', value: outros, color: DONUT_COLORS[8] }] : []),
  ]
  const total = data.reduce((s, d) => s + d.value, 0)

  return (
    <div className="flex flex-col h-full">
      <h3 className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3 shrink-0">
        Faturado por Indústria
      </h3>
      <div className="flex flex-1 gap-4 min-h-0">
        <div className="w-[45%] shrink-0">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie data={data} cx="50%" cy="50%"
                innerRadius="52%" outerRadius="82%"
                dataKey="value" strokeWidth={0}>
                {data.map((_, i) => <Cell key={i} fill={data[i].color} />)}
              </Pie>
              <ReTooltip
                contentStyle={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: 8, fontSize: 11 }}
                formatter={(v: number | undefined) => [fmtBRL(v ?? 0), '']}
                labelStyle={{ color: '#94a3b8' }}
              />
            </PieChart>
          </ResponsiveContainer>
        </div>
        <div className="flex-1 overflow-y-auto space-y-2 min-h-0 py-1">
          {data.map((d, i) => (
            <div key={i} className="flex items-center gap-2">
              <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: d.color }} />
              <span className="text-[11px] text-slate-300 truncate flex-1 leading-none">{d.name}</span>
              <span className="text-[11px] text-slate-400 font-bold tabular-nums shrink-0">
                {total > 0 ? ((d.value / total) * 100).toFixed(0) + '%' : '0%'}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ─── RcaRanking ───────────────────────────────────────────────────────────────

function RcaRanking({ cards }: { cards: CardItem[] }) {
  const sorted = [...cards].sort((a, b) => b.faturado - a.faturado).slice(0, 12)
  const maxFat = sorted[0]?.faturado ?? 1

  function barColor(pct: number) {
    if (pct >= 100) return '#22c55e'
    if (pct >= 70)  return '#eab308'
    return '#ef4444'
  }

  return (
    <div className="flex flex-col h-full">
      <h3 className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3 shrink-0">
        Ranking por Equipe
      </h3>
      <div className="flex-1 space-y-2.5 overflow-y-auto min-h-0">
        {sorted.map((c, i) => {
          const col  = barColor(c.pct)
          const barW = (c.faturado / maxFat) * 100
          return (
            <div key={c.key} className="flex items-center gap-2">
              <span className="w-5 text-[10px] font-bold text-slate-600 shrink-0 text-right">{i + 1}</span>
              <div className="flex-1 min-w-0">
                <div className="flex items-baseline justify-between mb-0.5">
                  <span className="text-xs font-semibold text-slate-200 truncate leading-none" title={c.label}>
                    {c.label.length > 18 ? c.label.slice(0, 18) + '…' : c.label}
                  </span>
                  <span className="text-xs font-black tabular-nums ml-2 shrink-0" style={{ color: col }}>
                    {fmtPct(c.pct)}
                  </span>
                </div>
                <div className="h-2 bg-slate-800 rounded-full overflow-hidden">
                  <div className="h-full rounded-full transition-all duration-700"
                    style={{ width: `${barW}%`, background: col }} />
                </div>
              </div>
              <span className="text-[10px] text-slate-500 tabular-nums shrink-0 w-14 text-right">
                {fmtBRL(c.faturado)}
              </span>
            </div>
          )
        })}
        {sorted.length === 0 && (
          <p className="text-slate-600 text-sm text-center mt-8">Sem dados</p>
        )}
      </div>
    </div>
  )
}

// ─── Hook de dados ────────────────────────────────────────────────────────────

const REFETCH_MS = 60 * 60 * 1_000

function useBiData(view: string) {
  return useQuery<BiResponse>({
    queryKey: ['bi-cards', view],
    queryFn: async () => {
      const r = await fetch(`/api/v2/farol/cards?view=${view}&comp_mode=yoy`)
      if (!r.ok) throw new Error(`Falha ao carregar BI (${view})`)
      return r.json()
    },
    staleTime: REFETCH_MS,
    gcTime:    REFETCH_MS + 5 * 60_000,
    refetchInterval: REFETCH_MS,
    refetchOnWindowFocus: false,
  })
}

// ─── FarolBI ─────────────────────────────────────────────────────────────────

export default function FarolBI() {
  const [nextRefresh, setNextRefresh] = useState(Date.now() + REFETCH_MS)
  const queryClient = useQueryClient()

  const kpiQuery = useBiData('V03')
  const indQuery = useBiData('V01')
  const rcaQuery = useBiData('V02')

  function handleRefresh() {
    queryClient.invalidateQueries({ queryKey: ['bi-cards'] })
    setNextRefresh(Date.now() + REFETCH_MS)
  }

  const kpi    = kpiQuery.data?.kpi
  const ind    = indQuery.data?.cards ?? []
  const rca    = rcaQuery.data?.cards ?? []
  const periodo = kpiQuery.data?.periodo

  const isLoading = kpiQuery.isLoading || indQuery.isLoading || rcaQuery.isLoading
  const isError   = kpiQuery.isError   || indQuery.isError   || rcaQuery.isError

  const mixMax = 8
  const mixPct = kpi ? Math.min((kpi.avg_mix / mixMax) * 100, 100) : 0

  return (
    <div className="-mx-4 -my-4 bg-slate-950 text-white flex flex-col select-none overflow-hidden"
      style={{ minHeight: 'calc(100vh - 88px)' }}>

      {/* Grade decorativa de fundo */}
      <div className="absolute inset-0 pointer-events-none"
        style={{
          backgroundImage: 'linear-gradient(rgba(255,255,255,.025) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.025) 1px,transparent 1px)',
          backgroundSize: '60px 60px',
        }}
      />

      {/* ── Header ────────────────────────────────────────────────────────── */}
      <header className="relative z-10 flex items-center justify-between px-8 py-4 border-b border-slate-800/60 shrink-0">
        <div className="flex items-center gap-4">
          <img src="/logo-fb.png" alt="Logo" className="h-8 w-8 rounded-lg object-cover opacity-70" />
          <div>
            <p className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Painel BI — War Room</p>
            <p className="text-sm font-semibold text-slate-200">
              {periodo
                ? periodo.ant_label
                  ? `${periodo.cur_label} · vs ${periodo.ant_label}`
                  : periodo.cur_label
                : 'Carregando período…'}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-6">
          <button onClick={handleRefresh}
            className="flex items-center gap-1.5 text-xs text-slate-600 hover:text-slate-300 transition-colors">
            <RefreshCw className="h-3.5 w-3.5" /> Atualizar
          </button>
          <BiClock nextRefresh={nextRefresh} />
        </div>
      </header>

      {/* ── Loading ───────────────────────────────────────────────────────── */}
      {isLoading && (
        <div className="flex-1 flex items-center justify-center">
          <div className="w-10 h-10 rounded-full border-2 border-blue-500 border-t-transparent animate-spin" />
        </div>
      )}

      {isError && !isLoading && (
        <div className="flex-1 flex items-center justify-center">
          <p className="text-red-400 text-sm">Erro ao carregar dados. Verifique a conexão.</p>
        </div>
      )}

      {/* ── Conteúdo ──────────────────────────────────────────────────────── */}
      {!isLoading && !isError && kpi && (
        <div className="relative z-10 flex-1 grid grid-rows-[auto_1fr] p-6 gap-6"
          style={{ minHeight: 0 }}>

          {/* Linha 1: 3 gauges */}
          <div className="grid grid-cols-3 gap-6">

            {/* Gauge: Objetivo Geral */}
            <div className="bg-slate-900 rounded-2xl border border-slate-800 flex flex-col items-center justify-center py-6 px-4">
              <ArcGaugeDark
                gaugeId="obj"
                arcPct={Math.min(kpi.total_pct, 100)}
                centerText={fmtPct(kpi.total_pct)}
                label="Objetivo Geral"
                sublabel={fmtBRL(kpi.total_faturado) + ' faturado'}
                subvalue={kpi.total_ant > 0 ? 'ref. ' + fmtBRL(kpi.total_ant) : undefined}
                color={gaugeColor(kpi.total_pct)}
              />
            </div>

            {/* Gauge: Positivação */}
            <div className="bg-slate-900 rounded-2xl border border-slate-800 flex flex-col items-center justify-center py-6 px-4">
              <ArcGaugeDark
                gaugeId="pos"
                arcPct={Math.min(kpi.total_positpct, 100)}
                centerText={fmtPct(kpi.total_positpct)}
                label="Positivação"
                sublabel={`${kpi.total_positivados.toLocaleString('pt-BR')} / ${kpi.total_base_cli.toLocaleString('pt-BR')} clientes`}
                color={gaugeColor(kpi.total_positpct)}
              />
            </div>

            {/* Gauge: Mix + Farol verde/vermelho */}
            <div className="bg-slate-900 rounded-2xl border border-slate-800 flex flex-col items-center justify-center py-6 px-4">
              <ArcGaugeDark
                gaugeId="mix"
                arcPct={mixPct}
                centerText={fmtNum(kpi.avg_mix)}
                label="Mix Médio (itens)"
                sublabel={`${fmtNum(kpi.avg_mix)} itens/cliente`}
                color={gaugeColor(mixPct)}
              />
              <div className="flex gap-5 mt-3">
                <span className="flex items-center gap-1.5 text-xs font-bold text-green-400">
                  <span className="w-2 h-2 rounded-full bg-green-400 shrink-0" />
                  {kpi.verdes} verdes
                </span>
                <span className="flex items-center gap-1.5 text-xs font-bold text-red-400">
                  <span className="w-2 h-2 rounded-full bg-red-400 shrink-0" />
                  {kpi.vermelhos} vermelhos
                </span>
              </div>
            </div>
          </div>

          {/* Linha 2: Donut + Ranking */}
          <div className="grid grid-cols-2 gap-6" style={{ minHeight: 0 }}>
            <div className="bg-slate-900 rounded-2xl border border-slate-800 p-6 flex flex-col" style={{ minHeight: 0 }}>
              {ind.length > 0
                ? <IndustryDonut cards={ind} />
                : <p className="text-slate-600 text-sm text-center my-auto">Sem dados de indústria</p>}
            </div>
            <div className="bg-slate-900 rounded-2xl border border-slate-800 p-6 flex flex-col" style={{ minHeight: 0 }}>
              {rca.length > 0
                ? <RcaRanking cards={rca} />
                : <p className="text-slate-600 text-sm text-center my-auto">Sem dados de equipe</p>}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
