import { useState, useCallback } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { TrendingUp, TrendingDown, Minus, ChevronLeft, UploadCloud, RefreshCw } from 'lucide-react'
import type { Cor } from '@/components/farol/Semaforo'

// ─── Tipos ────────────────────────────────────────────────────────────────────

interface DrillStep { level: string; value: string; label: string }

interface CardItem {
  key: string
  label: string
  level: string
  level_label: string
  valor_atual: number
  valor_ant: number
  pct: number
  cor: Cor
  faturado: number
  transmitido: number
  positivados: number
  base_cli: number
  positpct: number
  mix: number
}

interface KPI {
  total_atual: number
  total_ant: number
  total_pct: number
  total_cor: Cor
  total_faturado: number
  total_transmitido: number
  total_positivados: number
  total_base_cli: number
  total_positpct: number
  avg_mix: number
  verdes: number
  vermelhos: number
}

interface CardsResponse {
  cards: CardItem[]
  kpi: KPI
  periodo: { ref_ano: number; ref_mes: number; label: string; comp_mode: string }
  periodos: string[]
  view: string
  drill_path: DrillStep[]
  next_level: string
  next_level_label: string
}

// ─── Utilitários ──────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v >= 1_000_000_000) return 'R$ ' + (v / 1_000_000_000).toFixed(2).replace('.', ',') + 'B'
  if (v >= 1_000_000)     return 'R$ ' + (v / 1_000_000).toFixed(1).replace('.', ',') + 'M'
  if (v >= 1_000)         return 'R$ ' + (v / 1_000).toFixed(0).replace(/\B(?=(\d{3})+(?!\d))/g, '.') + 'K'
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0 })
}
function fmtBRLFull(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}
function fmtPct(v: number) { return v.toFixed(1) + '%' }
// Pct compacto para exibição dentro de elementos pequenos (gauge, badge)
function fmtPctShort(v: number) {
  if (v >= 10_000) return '>9999%'
  if (v >= 1_000)  return (v / 1_000).toFixed(1).replace('.', ',') + 'K%'
  return Math.round(v) + '%'
}
function fmtNum(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 1 }) }

function parsePeriodo(s: string): { ano: number; mes: number } {
  const [y, m] = s.split('-')
  return { ano: +y, mes: +m }
}

const MES_NOMES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']
function fmtMesAno(ano: number, mes: number) { return `${MES_NOMES[mes] ?? mes}/${String(ano).slice(2)}` }

const COR_HEX: Record<Cor, string> = {
  verde:    '#10b981',
  amarelo:  '#f59e0b',
  vermelho: '#ef4444',
}
const COR_BAR: Record<Cor, string> = {
  verde:    'bg-emerald-500',
  amarelo:  'bg-amber-400',
  vermelho: 'bg-red-500',
}
const COR_TEXT: Record<Cor, string> = {
  verde:    'text-emerald-400',
  amarelo:  'text-amber-400',
  vermelho: 'text-red-400',
}
const COR_TEXT_LIGHT: Record<Cor, string> = {
  verde:    'text-emerald-600',
  amarelo:  'text-amber-600',
  vermelho: 'text-red-500',
}

function delta(atual: number, ant: number) {
  if (ant === 0) return 0
  return ((atual - ant) / Math.abs(ant)) * 100
}

// ─── SVG Arc Gauge ────────────────────────────────────────────────────────────

function ArcGauge({ pct, cor, size = 148 }: { pct: number; cor: Cor; size?: number }) {
  const cx = size / 2, cy = size / 2
  const r = size * 0.37
  const sw = size * 0.065
  const total = 2 * Math.PI * r
  const arcLen = total * 0.75
  const filled = arcLen * Math.min(Math.max(pct, 0) / 100, 1)
  const col = COR_HEX[cor]

  return (
    <svg width={size} height={size * 0.72} viewBox={`0 0 ${size} ${size * 0.72}`} className="overflow-visible">
      {/* glow filter */}
      <defs>
        <filter id="glow" x="-30%" y="-30%" width="160%" height="160%">
          <feGaussianBlur stdDeviation="3" result="blur" />
          <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
        </filter>
      </defs>
      {/* track */}
      <circle
        cx={cx} cy={cy} r={r}
        fill="none"
        stroke="rgba(255,255,255,0.07)"
        strokeWidth={sw}
        strokeDasharray={`${arcLen} ${total - arcLen}`}
        strokeLinecap="round"
        transform={`rotate(-225 ${cx} ${cy})`}
      />
      {/* progress */}
      {filled > 1 && (
        <circle
          cx={cx} cy={cy} r={r}
          fill="none"
          stroke={col}
          strokeWidth={sw}
          strokeDasharray={`${filled} ${total - filled}`}
          strokeLinecap="round"
          transform={`rotate(-225 ${cx} ${cy})`}
          filter="url(#glow)"
          style={{ transition: 'stroke-dasharray 0.8s cubic-bezier(0.4,0,0.2,1)' }}
        />
      )}
    </svg>
  )
}

// ─── Hero Band ────────────────────────────────────────────────────────────────

function HeroBand({ kpi, periodo }: { kpi: KPI; periodo: CardsResponse['periodo'] }) {
  const d = delta(kpi.total_atual, kpi.total_ant)
  const isUp = d >= 0

  return (
    <div className="relative bg-gradient-to-br from-slate-950 via-slate-900 to-slate-900 rounded-2xl overflow-hidden mb-6 shadow-2xl">
      {/* subtle grid texture */}
      <div
        className="absolute inset-0 opacity-[0.04]"
        style={{
          backgroundImage: 'linear-gradient(rgba(255,255,255,.5) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,.5) 1px, transparent 1px)',
          backgroundSize: '40px 40px'
        }}
      />

      <div className="relative px-8 py-8">
        {/* Header row */}
        <div className="flex items-start justify-between mb-6">
          <div>
            <p className="text-slate-400 text-xs font-medium uppercase tracking-widest mb-1">Painel Executivo</p>
            <p className="text-white/80 text-sm font-medium">{periodo.label}</p>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-xs text-slate-500 uppercase tracking-wider">{periodo.comp_mode.toUpperCase()}</span>
            <div className="flex items-center gap-1.5">
              <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-400 bg-emerald-400/10 border border-emerald-400/20 px-2 py-0.5 rounded-full">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 inline-block" />
                {kpi.verdes} verde{kpi.verdes !== 1 ? 's' : ''}
              </span>
              <span className="inline-flex items-center gap-1 text-xs font-semibold text-red-400 bg-red-400/10 border border-red-400/20 px-2 py-0.5 rounded-full">
                <span className="w-1.5 h-1.5 rounded-full bg-red-400 inline-block" />
                {kpi.vermelhos} em risco
              </span>
            </div>
          </div>
        </div>

        {/* Main metrics */}
        <div className="flex items-end gap-10 flex-wrap">
          {/* Arc gauge + main % */}
          <div className="relative flex-shrink-0">
            <ArcGauge pct={kpi.total_pct} cor={kpi.total_cor} size={160} />
            <div className="absolute inset-0 flex flex-col items-center justify-center pb-4">
              <span className={`font-black tabular-nums ${kpi.total_pct >= 1000 ? 'text-xl' : 'text-4xl'} ${COR_TEXT[kpi.total_cor]}`}>
                {fmtPctShort(kpi.total_pct)}
              </span>
              <span className="text-slate-400 text-[10px] uppercase tracking-widest font-semibold mt-0.5">atingimento</span>
            </div>
          </div>

          {/* Divisor */}
          <div className="w-px h-20 bg-white/10 self-center hidden md:block" />

          {/* Revenue block */}
          <div className="flex-shrink-0">
            <p className="text-slate-400 text-xs uppercase tracking-widest mb-1">Total Atual</p>
            <p className="text-white text-4xl font-black tabular-nums leading-none">{fmtBRL(kpi.total_atual)}</p>
            <div className={`flex items-center gap-1.5 mt-2 ${isUp ? 'text-emerald-400' : 'text-red-400'}`}>
              {isUp ? <TrendingUp className="h-4 w-4" /> : <TrendingDown className="h-4 w-4" />}
              <span className="text-sm font-bold tabular-nums">{isUp ? '+' : ''}{d.toFixed(1)}%</span>
              <span className="text-slate-500 text-xs">vs {fmtBRL(kpi.total_ant)}</span>
            </div>
          </div>

          {/* Divisor */}
          <div className="w-px h-20 bg-white/10 self-center hidden lg:block" />

          {/* Secondary metrics */}
          <div className="flex gap-8 flex-wrap">
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Faturado</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtBRL(kpi.total_faturado)}</p>
            </div>
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Transmitido</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtBRL(kpi.total_transmitido)}</p>
            </div>
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Positivação</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtPct(kpi.total_positpct)}</p>
              <p className="text-slate-500 text-[11px] mt-0.5">{kpi.total_positivados}/{kpi.total_base_cli} clientes</p>
            </div>
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Mix médio</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtNum(kpi.avg_mix)}</p>
              <p className="text-slate-500 text-[11px] mt-0.5">itens/cliente</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Performance Ranking ──────────────────────────────────────────────────────

function PerformanceRanking({
  cards,
  levelLabel,
  onDrill,
}: {
  cards: CardItem[]
  levelLabel: string
  onDrill: (card: CardItem) => void
}) {
  const sorted = [...cards].sort((a, b) => b.pct - a.pct)
  const maxPct = Math.max(...sorted.map(c => c.pct), 100)

  const medals = ['🥇', '🥈', '🥉']
  const threshold = { verde: 100, amarelo: 80 }

  return (
    <div className="bg-white rounded-2xl shadow-sm border border-slate-100 overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between px-6 py-4 border-b border-slate-50">
        <div>
          <h2 className="text-sm font-bold text-slate-800 uppercase tracking-wider">Ranking de Desempenho</h2>
          <p className="text-xs text-slate-400 mt-0.5">Por {levelLabel} · ordenado por atingimento</p>
        </div>
        <div className="flex gap-3 text-[11px]">
          <span className="flex items-center gap-1 text-emerald-600 font-medium">
            <span className="w-2 h-2 rounded-full bg-emerald-500 inline-block" /> ≥ 100%
          </span>
          <span className="flex items-center gap-1 text-amber-600 font-medium">
            <span className="w-2 h-2 rounded-full bg-amber-400 inline-block" /> ≥ 80%
          </span>
          <span className="flex items-center gap-1 text-red-500 font-medium">
            <span className="w-2 h-2 rounded-full bg-red-500 inline-block" /> &lt; 80%
          </span>
        </div>
      </div>

      {/* Rows */}
      <div className="divide-y divide-slate-50">
        {sorted.map((card, idx) => {
          const barW = Math.min((card.pct / maxPct) * 100, 100)
          const d = delta(card.valor_atual, card.valor_ant)
          const cor: Cor = card.pct >= threshold.verde ? 'verde' : card.pct >= threshold.amarelo ? 'amarelo' : 'vermelho'
          const isLast3 = idx >= sorted.length - 3 && sorted.length > 3

          return (
            <button
              key={card.key}
              onClick={() => onDrill(card)}
              className={`w-full flex items-center gap-4 px-6 py-4 hover:bg-slate-50/80 transition-colors text-left group ${isLast3 ? 'bg-red-50/30 hover:bg-red-50/50' : ''}`}
            >
              {/* Rank */}
              <div className="w-8 flex-shrink-0 text-center">
                {idx < 3 ? (
                  <span className="text-xl">{medals[idx]}</span>
                ) : (
                  <span className="text-sm font-bold text-slate-300">{idx + 1}</span>
                )}
              </div>

              {/* Name */}
              <div className="w-36 flex-shrink-0">
                <p className="text-sm font-semibold text-slate-800 truncate group-hover:text-primary transition-colors">{card.label}</p>
                <p className="text-[10px] text-slate-400">{card.key}</p>
              </div>

              {/* Bar */}
              <div className="flex-1 min-w-0">
                <div className="h-2.5 bg-slate-100 rounded-full overflow-hidden">
                  <div
                    className={`h-full ${COR_BAR[cor]} rounded-full transition-all duration-700`}
                    style={{ width: `${barW}%` }}
                  />
                </div>
              </div>

              {/* % */}
              <div className="w-16 flex-shrink-0 text-right">
                <span className={`text-base font-black tabular-nums ${COR_TEXT_LIGHT[cor]}`}>{fmtPct(card.pct)}</span>
              </div>

              {/* Value */}
              <div className="w-28 flex-shrink-0 text-right hidden sm:block">
                <p className="text-sm font-semibold text-slate-700 tabular-nums">{fmtBRL(card.valor_atual)}</p>
                <p className="text-[10px] text-slate-400 tabular-nums">vs {fmtBRL(card.valor_ant)}</p>
              </div>

              {/* Delta */}
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

              {/* Positivação (se disponível) */}
              {card.base_cli > 0 && (
                <div className="w-24 flex-shrink-0 text-right hidden lg:block">
                  <p className="text-xs font-semibold text-slate-600 tabular-nums">{fmtPct(card.positpct)}</p>
                  <p className="text-[10px] text-slate-400">{card.positivados}/{card.base_cli} cli</p>
                </div>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}

// ─── Drill breadcrumb (minimalista) ──────────────────────────────────────────

function DrillBreadcrumb({
  drillPath,
  nextLevelLabel,
  onBack,
}: {
  drillPath: DrillStep[]
  nextLevelLabel: string
  onBack: () => void
}) {
  if (drillPath.length === 0) return null
  const last = drillPath[drillPath.length - 1]
  return (
    <div className="flex items-center gap-2 mb-4">
      <button
        onClick={onBack}
        className="flex items-center gap-1.5 text-sm text-slate-500 hover:text-primary transition-colors font-medium"
      >
        <ChevronLeft className="h-4 w-4" />
        Voltar
      </button>
      <span className="text-slate-300">/</span>
      <span className="text-sm font-semibold text-slate-700">{last.label}</span>
      <span className="text-slate-400 text-xs ml-1">→ por {nextLevelLabel}</span>
    </div>
  )
}

// ─── Hook de dados ─────────────────────────────────────────────────────────────

function useCards(view: string, compMode: string, refAno: number, refMes: number, drillPath: DrillStep[]) {
  return useQuery<CardsResponse>({
    queryKey: ['farol-v2-cards', view, compMode, refAno, refMes, JSON.stringify(drillPath)],
    queryFn: async () => {
      const params = new URLSearchParams({
        view,
        comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(drillPath.length > 0 && { drill: JSON.stringify(drillPath) }),
      })
      const r = await fetch(`/api/v2/farol/cards?${params}`)
      if (!r.ok) throw new Error('Falha ao carregar dados')
      return r.json()
    },
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// ─── FarolExecutivo ───────────────────────────────────────────────────────────

export default function FarolExecutivo() {
  const navigate    = useNavigate()
  const queryClient = useQueryClient()

  const [view, setView]               = useState<'V01' | 'V02' | 'V03'>('V01')
  const [compMode, setCompMode]       = useState('yoy')
  const [drillPath, setDrillPath]     = useState<DrillStep[]>([])
  const [refAno, setRefAno]           = useState(0)
  const [refreshing, setRefreshing]   = useState(false)
  const [refMes, setRefMes]       = useState(0)

  const { data, isLoading, error } = useCards(view, compMode, refAno, refMes, drillPath)

  const autoRef = useCallback((d: CardsResponse) => {
    if (refAno === 0 && d.periodo.ref_ano) {
      setRefAno(d.periodo.ref_ano)
      setRefMes(d.periodo.ref_mes)
    }
  }, [refAno])
  if (data && refAno === 0) autoRef(data)

  const handleDrill = (card: CardItem) => {
    if (card.level === 'cod_prod') return // Produto é o nível folha
    setDrillPath(prev => [...prev, { level: card.level, value: card.key, label: card.label }])
  }

  const handleBack = () => {
    setDrillPath(prev => prev.slice(0, -1))
  }

  const periodos = data?.periodos ?? []

  const handleRefreshViews = async () => {
    setRefreshing(true)
    try {
      await fetch('/api/v2/farol/refresh-views', { method: 'POST' })
      await queryClient.invalidateQueries({ queryKey: ['farol-v2-cards'] })
      setRefAno(0) // força re-detect do período após refresh
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <div className="min-h-full">
      {/* ── Controles ────────────────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-5">
        {/* Visão */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {([
            { id: 'V01' as const, label: 'Por Indústria' },
            { id: 'V03' as const, label: 'Por Gerência' },
            { id: 'V02' as const, label: 'Por Equipe' },
          ]).map(v => (
            <button
              key={v.id}
              onClick={() => { setView(v.id); setDrillPath([]) }}
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                view === v.id
                  ? 'bg-primary text-white'
                  : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              {v.label}
            </button>
          ))}
        </div>

        {/* Comparação */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {[
            { id: 'yoy', label: 'Ano a Ano' },
            { id: 'ytd', label: 'Projeção Anual' },
            { id: 'mom', label: 'Mês a Mês' },
          ].map(m => (
            <button
              key={m.id}
              onClick={() => { setCompMode(m.id); setDrillPath([]) }}
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                compMode === m.id
                  ? 'bg-slate-800 text-white'
                  : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>

        {/* Período */}
        {periodos.length > 0 && (
          <select
            value={refAno > 0 ? `${refAno}-${String(refMes).padStart(2, '0')}` : ''}
            onChange={e => {
              const p = parsePeriodo(e.target.value)
              setRefAno(p.ano)
              setRefMes(p.mes)
              setDrillPath([])
            }}
            className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          >
            {periodos.map(p => {
              const { ano, mes } = parsePeriodo(p)
              return <option key={p} value={p}>{fmtMesAno(ano, mes)}</option>
            })}
          </select>
        )}

        {/* Botão consolidar view */}
        <button
          onClick={handleRefreshViews}
          disabled={refreshing}
          title="Reconstrói a view de dados (necessário após deploy ou importação sem refresh automático)"
          className="ml-auto flex items-center gap-1.5 h-8 px-3 rounded-lg border border-dashed border-amber-300 text-xs text-amber-600 hover:border-amber-500 hover:text-amber-700 transition-colors shrink-0 disabled:opacity-50"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} />
          {refreshing ? 'Consolidando...' : 'Consolidar view'}
        </button>

        {/* Botão importar */}
        <button
          onClick={() => navigate('/farol/importar')}
          className="flex items-center gap-1.5 h-8 px-3 rounded-lg border border-dashed border-slate-300 text-xs text-slate-500 hover:border-primary hover:text-primary transition-colors shrink-0"
        >
          <UploadCloud className="h-3.5 w-3.5" />
          Importar dados
        </button>
      </div>

      {/* ── Loading skeleton ─────────────────────────────────────────────── */}
      {isLoading && (
        <div className="space-y-4">
          <div className="bg-slate-900 rounded-2xl h-44 animate-pulse" />
          <div className="bg-white rounded-2xl h-64 animate-pulse border border-slate-100" />
        </div>
      )}

      {/* ── Erro ─────────────────────────────────────────────────────────── */}
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-6 text-red-700 text-sm">
          <p className="font-semibold mb-1">Erro ao carregar painel</p>
          <p>{(error as Error).message}</p>
        </div>
      )}

      {/* ── Sem dados ────────────────────────────────────────────────────── */}
      {!isLoading && !error && data?.cards.length === 0 && (
        <div className="bg-gradient-to-br from-slate-950 to-slate-900 rounded-2xl p-16 text-center shadow-2xl">
          <p className="text-slate-400 text-sm font-medium mb-2">Nenhum dado disponível</p>
          <p className="text-slate-600 text-xs mb-6">Importe um arquivo CSV para visualizar o painel executivo.</p>
          <button
            onClick={() => navigate('/farol/importar')}
            className="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold hover:bg-primary/90 transition-colors"
          >
            <UploadCloud className="h-4 w-4" />
            Ir para importação
          </button>
        </div>
      )}

      {/* ── Dashboard executivo ───────────────────────────────────────────── */}
      {!isLoading && !error && data && data.cards.length > 0 && (
        <>
          <HeroBand kpi={data.kpi} periodo={data.periodo} />

          <DrillBreadcrumb
            drillPath={drillPath}
            nextLevelLabel={data.next_level_label}
            onBack={handleBack}
          />

          <PerformanceRanking
            cards={data.cards}
            levelLabel={data.next_level_label}
            onDrill={handleDrill}
          />
        </>
      )}
    </div>
  )
}
