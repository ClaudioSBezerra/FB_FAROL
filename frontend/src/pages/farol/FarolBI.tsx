import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { PieChart, Pie, Cell, Tooltip as ReTooltip, ResponsiveContainer } from 'recharts'
import { RefreshCw, TrendingUp, TrendingDown } from 'lucide-react'

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

interface PulsoResp {
  dia_ref: string
  dia_ref_label: string
  vl_atual: number
  qt_atual: number
  vl_espelho: number
  qt_espelho: number
  espelho_label: string
  espelho_data: string
  pct: number
  pct_qt: number
  cor: 'verde' | 'amarelo' | 'vermelho'
  parcial: boolean
  sem_dado: boolean
}

// ─── Utils ────────────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v === 0) return '—'
  // Valores ABSOLUTOS (sem abreviar K/M/B) — decisão do gestor. Ex.: R$ 2.500,35
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 })
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

// ─── PulsoCard (War Room) ─────────────────────────────────────────────────────────

type Cor = 'verde' | 'amarelo' | 'vermelho'

function pulsoColor(cor: Cor): string {
  switch (cor) {
    case 'verde': return '#22c55e'
    case 'amarelo': return '#eab308'
    case 'vermelho': return '#ef4444'
  }
}

function PulsoCard() {
  const { data, isLoading } = useQuery<PulsoResp>({
    queryKey: ['farol-pulso-empresa'],
    queryFn: async () => {
      const r = await fetch('/api/v2/farol/pulso')
      if (!r.ok) throw new Error('Falha ao carregar pulso')
      return r.json()
    },
    staleTime: 10 * 60_000,
    refetchInterval: 10 * 60_000,
    refetchOnWindowFocus: false,
  })

  if (isLoading || !data || data.sem_dado) return null

  const deltaVl = data.pct - 100
  const deltaQt = data.pct_qt - 100
  const color = pulsoColor(data.cor)

  return (
    <div className="bg-slate-900 rounded-xl border border-slate-800 px-4 py-2.5 min-w-[280px]">
      <div className="flex items-center justify-between mb-2">
        <p className="text-[10px] font-bold uppercase tracking-wider text-slate-500">
          Pedidos de Ontem
        </p>
        <p className="text-[10px] text-slate-500">{data.dia_ref_label}</p>
      </div>

      <div className="flex items-center justify-between gap-4">
        {/* Valor */}
        <div className="flex-1">
          <p className="text-2xl font-black text-white tabular-nums leading-none">
            {fmtBRL(data.vl_atual)}
          </p>
          <div className="flex items-center gap-1 mt-1">
            {deltaVl >= 0 ? (
              <TrendingUp className="h-3 w-3 text-emerald-400" />
            ) : (
              <TrendingDown className="h-3 w-3 text-red-400" />
            )}
            <span className={`text-xs font-bold tabular-nums ${deltaVl >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              {deltaVl >= 0 ? '+' : ''}{deltaVl.toFixed(0)}%
            </span>
            <span className="text-[10px] text-slate-500">vs {data.espelho_label}</span>
          </div>
        </div>

        {/* Qtd pedidos */}
        <div className="text-right">
          <p className="text-xl font-black text-white tabular-nums leading-none">
            {data.qt_atual}
          </p>
          <div className="flex items-center gap-1 justify-end mt-1">
            {deltaQt >= 0 ? (
              <TrendingUp className="h-3 w-3 text-emerald-400" />
            ) : (
              <TrendingDown className="h-3 w-3 text-red-400" />
            )}
            <span className={`text-xs font-bold tabular-nums ${deltaQt >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              {deltaQt >= 0 ? '+' : ''}{deltaQt.toFixed(0)}%
            </span>
          </div>
        </div>
      </div>

      {data.parcial && (
        <div className="mt-2 pt-2 border-t border-slate-800">
          <p className="text-[9px] text-amber-400 font-medium">
            ⚠ Dado parcial — pode aumentar
          </p>
        </div>
      )}
    </div>
  )
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
  gaugeId, arcPct, centerText, label, sublabel, subvalue, prevValue, curValue, color, size = 180,
}: {
  gaugeId: string
  arcPct: number       // 0-100, controls arc fill
  centerText: string   // displayed in center
  label: string
  sublabel?: string
  subvalue?: string
  prevValue?: string   // valor anterior (comparativo) — hierarquia secundária
  curValue?: string    // valor atual faturado — hierarquia primária (grande, colorido)
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
        {/* Comparativo Anterior → Atual como protagonista (grande + negrito) */}
        {curValue ? (
          <div className="flex flex-col items-center gap-1">
            {prevValue && (
              <div className="flex items-baseline justify-center gap-2">
                <span className="text-slate-500 uppercase tracking-wide font-semibold"
                  style={{ fontSize: size * 0.058 }}>
                  Anterior
                </span>
                <span className="text-slate-400 font-bold tabular-nums"
                  style={{ fontSize: size * 0.082 }}>
                  {prevValue}
                </span>
              </div>
            )}
            <div className="flex items-baseline justify-center gap-2">
              <span className="uppercase tracking-wide font-semibold"
                style={{ fontSize: size * 0.058, color }}>
                Atual
              </span>
              <span className="font-black tabular-nums" style={{ fontSize: size * 0.105, color }}>
                {curValue}
              </span>
            </div>
          </div>
        ) : (
          <>
            {sublabel && <p className="text-white font-semibold" style={{ fontSize: size * 0.088 }}>{sublabel}</p>}
            {subvalue && <p className="text-slate-500 mt-1" style={{ fontSize: size * 0.07 }}>{subvalue}</p>}
          </>
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

type CompMode = 'ytd' | 'mtd'

function useBiData(view: string, compMode: CompMode) {
  return useQuery<BiResponse>({
    queryKey: ['bi-cards', view, compMode],
    queryFn: async () => {
      const r = await fetch(`/api/v2/farol/cards?view=${view}&comp_mode=${compMode}`)
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
  const [compMode, setCompMode] = useState<CompMode>('ytd')
  const queryClient = useQueryClient()

  const kpiQuery = useBiData('V03', compMode)
  const indQuery = useBiData('V01', compMode)
  const rcaQuery = useBiData('V02', compMode)

  function handleRefresh() {
    queryClient.invalidateQueries({ queryKey: ['bi-cards'] })
    setNextRefresh(Date.now() + REFETCH_MS)
  }

  function handleCompModeChange(mode: CompMode) {
    if (mode !== compMode) {
      setCompMode(mode)
    }
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
        <div className="flex items-center gap-6">
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
          {/* Seletor YTD / MTD */}
          <div className="flex items-center gap-1 bg-slate-900 rounded-lg p-1 border border-slate-800">
            <button
              onClick={() => handleCompModeChange('ytd')}
              className={`px-3 py-1 rounded text-xs font-semibold transition-all ${
                compMode === 'ytd'
                  ? 'bg-blue-600 text-white shadow-lg'
                  : 'text-slate-500 hover:text-slate-300'
              }`}>
              Acumulado Ano
            </button>
            <button
              onClick={() => handleCompModeChange('mtd')}
              className={`px-3 py-1 rounded text-xs font-semibold transition-all ${
                compMode === 'mtd'
                  ? 'bg-blue-600 text-white shadow-lg'
                  : 'text-slate-500 hover:text-slate-300'
              }`}>
              Mês Atual
            </button>
          </div>
          {/* Card Pulso de Ontem */}
          <PulsoCard />
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
                prevValue={kpi.total_ant > 0 ? fmtBRL(kpi.total_ant) : undefined}
                curValue={fmtBRL(kpi.total_faturado)}
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
