import { useState, useCallback, useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { TrendingUp, TrendingDown, Minus, ChevronLeft, UploadCloud, RefreshCw, Info, Search, SlidersHorizontal, X } from 'lucide-react'
import type { Cor } from '@/components/farol/Semaforo'
import { useAuth } from '@/contexts/AuthContext'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

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
  periodo: {
    ref_ano: number; ref_mes: number;
    label: string; comp_mode: string;
    cur_label?: string; ant_label?: string;
    comp_ano?: number; comp_mes?: number;
  }
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

const MODE_LABEL: Record<string, string> = {
  yoy: 'Ano a Ano',
  ytd: 'Projeção Anual',
  mom: 'Mês a Mês',
}
const MODE_DESC: Record<string, string> = {
  yoy: 'Compara o mesmo mês do ano atual com o mesmo mês do ano anterior.',
  ytd: 'Extrapola o acumulado do ano atual (Jan ao mês selecionado) para projetar o total anual. Compara com o total real do ano anterior.',
  mom: 'Compara o mês atual com o mês imediatamente anterior.',
}

function getPeriodoTooltips(p: CardsResponse['periodo']) {
  const mes = MES_NOMES[p.ref_mes] ?? String(p.ref_mes)
  const fator = p.ref_mes > 0 ? (12 / p.ref_mes).toFixed(1) : '1'
  switch (p.comp_mode) {
    case 'yoy':
      return {
        ant: `Faturamento de ${p.ant_label ?? '—'} — mesmo mês do ano anterior`,
        cur: `Faturamento de ${p.cur_label ?? '—'} — mês de referência atual`,
      }
    case 'ytd':
      return {
        ant: `Total faturado em ${p.ant_label ?? '—'} — ano completo (12 meses)`,
        cur: `Projeção ${p.ref_ano}: acumulado Jan–${mes}/${p.ref_ano} × ${fator}`,
      }
    case 'mom':
      return {
        ant: `Faturamento de ${p.ant_label ?? '—'} — mês anterior`,
        cur: `Faturamento de ${p.cur_label ?? '—'} — mês atual`,
      }
    default:
      return { ant: '', cur: '' }
  }
}

// ─── InfoTooltip ──────────────────────────────────────────────────────────────

function InfoTooltip({ text, iconClassName }: { text: string; iconClassName?: string }) {
  if (!text) return null
  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex items-center cursor-help">
            <Info className={`h-3 w-3 ${iconClassName ?? 'text-slate-400 hover:text-slate-600'}`} />
          </span>
        </TooltipTrigger>
        <TooltipContent side="bottom" className="max-w-xs text-xs leading-relaxed">
          {text}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

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
  const tips = getPeriodoTooltips(periodo)

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
            <p className="text-slate-400 text-xs font-medium uppercase tracking-widest mb-1">Painel Vendas</p>
            <p className="text-white/80 text-sm font-medium">{periodo.label}</p>
          </div>
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1 text-xs text-slate-300 font-semibold">
              {MODE_LABEL[periodo.comp_mode] ?? periodo.comp_mode}
              <InfoTooltip text={MODE_DESC[periodo.comp_mode] ?? ''} iconClassName="text-slate-500 hover:text-slate-300" />
            </span>
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
            <div className="absolute inset-0 flex flex-col items-center justify-center pt-10">
              <span className={`font-black tabular-nums ${
                kpi.total_pct >= 1000 ? 'text-xl'
                  : kpi.total_pct >= 100 ? 'text-3xl'
                  : 'text-4xl'
              } ${COR_TEXT[kpi.total_cor]}`}>
                {fmtPctShort(kpi.total_pct)}
              </span>
              <span className="text-slate-400 text-[10px] uppercase tracking-widest font-semibold mt-0.5">atingimento</span>
            </div>
          </div>

          {/* Divisor */}
          <div className="w-px h-20 bg-white/10 self-center hidden md:block" />

          {/* Bloco Anterior */}
          <div className="flex-shrink-0">
            <div className="flex items-center gap-1 mb-1">
              <p className="text-slate-400 text-xs uppercase tracking-widest">Total Anterior</p>
              <InfoTooltip text={tips.ant} iconClassName="text-slate-500 hover:text-slate-300" />
            </div>
            <p className="text-slate-300 text-3xl font-bold tabular-nums leading-none">{fmtBRL(kpi.total_ant)}</p>
            <p className="text-slate-500 text-xs mt-2 truncate" title={periodo.ant_label || ''}>{periodo.ant_label || '—'}</p>
          </div>

          {/* Divisor */}
          <div className="w-px h-20 bg-white/10 self-center hidden md:block" />

          {/* Bloco Atual */}
          <div className="flex-shrink-0">
            <div className="flex items-center gap-1 mb-1">
              <p className="text-slate-400 text-xs uppercase tracking-widest">
                {periodo.comp_mode === 'ytd' ? 'Projeção Anual' : 'Total Atual'}
              </p>
              <InfoTooltip text={tips.cur} iconClassName="text-slate-500 hover:text-slate-300" />
            </div>
            <p className="text-white text-4xl font-black tabular-nums leading-none">{fmtBRL(kpi.total_atual)}</p>
            <div className={`flex items-center gap-1.5 mt-2 ${isUp ? 'text-emerald-400' : 'text-red-400'}`}>
              {isUp ? <TrendingUp className="h-4 w-4" /> : <TrendingDown className="h-4 w-4" />}
              <span className="text-sm font-bold tabular-nums">{isUp ? '+' : ''}{d.toFixed(1)}%</span>
              <span className="text-slate-500 text-xs truncate" title={periodo.cur_label || ''}>{periodo.cur_label || ''}</span>
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

// ─── Stock Bar ────────────────────────────────────────────────────────────────

// Pequenas variações para simular movimentação de preço (stock chart feel)
const STOCK_NOISE = [0,2,-1,3,-2,1,-3,2,0,-1,3,-2,1,2,-1,0,3,-2,1,-1,2,3,0,-1,2,-2,1,-3,2,1,0]

function StockBar({ pct, maxPct, cor, uid }: { pct: number; maxPct: number; cor: Cor; uid: string }) {
  const W = 200, H = 30
  const col   = COR_HEX[cor]
  const ratio = Math.min(pct / maxPct, 1)
  const fillX = ratio * W
  const sid   = uid.replace(/[^a-z0-9]/gi, '-')
  const NPTS  = 30

  // Linha sobe do canto inferior-esquerdo até a altura proporcional ao atingimento
  const startY = H * 0.88
  const endY   = H * (1 - ratio * 0.76)
  const noise  = H * 0.055

  const pts: Array<[number, number]> = Array.from({ length: NPTS + 1 }, (_, i) => {
    const t = i / NPTS
    const base = startY + (endY - startY) * t
    return [t * W, Math.max(2, Math.min(H - 2, base + STOCK_NOISE[i % STOCK_NOISE.length] * noise))]
  })

  const line = pts.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`).join(' ')

  // Área de gradiente abaixo da linha (apenas na parte colorida)
  const visiblePts = pts.filter(([x]) => x <= fillX + W / NPTS)
  const area = visiblePts.length > 1
    ? `M0,${H} ${visiblePts.map(([x, y]) => `L${x.toFixed(1)},${y.toFixed(1)}`).join(' ')} L${fillX.toFixed(1)},${H}Z`
    : ''

  // Ponto brilhante na ponta da linha colorida
  const dotPt = pts.reduce((best, p) => p[0] <= fillX ? p : best, pts[0])

  return (
    <div className="flex-1 min-w-0">
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full h-7" preserveAspectRatio="none">
        <defs>
          <linearGradient id={`sg-${sid}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%"   stopColor={col} stopOpacity="0.28" />
            <stop offset="100%" stopColor={col} stopOpacity="0.02" />
          </linearGradient>
          <clipPath id={`sc-${sid}`}>
            <rect x="0" y="0" width={fillX} height={H} />
          </clipPath>
          <filter id={`glow-${sid}`} x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="2" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
        </defs>

        {/* Linha fantasma — potencial completo */}
        <path d={line} stroke="rgba(148,163,184,0.13)" strokeWidth="0.8" fill="none"
              strokeLinecap="round" strokeLinejoin="round" />

        {/* Gradiente de área */}
        {area && <path d={area} fill={`url(#sg-${sid})`} />}

        {/* Linha colorida da bolsa */}
        <path d={line} stroke={col} strokeWidth="1.6" fill="none"
              strokeLinecap="round" strokeLinejoin="round"
              clipPath={`url(#sc-${sid})`} />

        {/* Ponto brilhante na ponta */}
        {ratio > 0.02 && (
          <circle cx={dotPt[0].toFixed(1)} cy={dotPt[1].toFixed(1)} r="2.5"
                  fill={col} filter={`url(#glow-${sid})`} />
        )}

        {/* Marcador de 100% quando há ultrapassagem */}
        {maxPct > 105 && (
          <line x1={(100 / maxPct) * W} y1="2" x2={(100 / maxPct) * W} y2={H - 2}
                stroke="rgba(100,116,139,0.5)" strokeWidth="1" strokeDasharray="2,2" />
        )}
      </svg>
    </div>
  )
}

// ─── Performance Ranking ──────────────────────────────────────────────────────

function PerformanceRanking({
  cards,
  levelLabel,
  onDrill,
  wallStreet,
}: {
  cards: CardItem[]
  levelLabel: string
  onDrill: (card: CardItem) => void
  wallStreet?: boolean
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
          <div className="flex items-center gap-2">
            <h2 className="text-sm font-bold text-slate-800 uppercase tracking-wider">Ranking de Desempenho</h2>
            <span className="inline-flex items-center text-xs font-black text-red-600 bg-red-50 border border-red-200 px-2.5 py-0.5 rounded-full uppercase tracking-widest">
              {levelLabel}
            </span>
          </div>
          <p className="text-xs text-slate-400 mt-0.5">ordenado por atingimento</p>
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
              {wallStreet
                ? <StockBar pct={card.pct} maxPct={maxPct} cor={cor} uid={card.key} />
                : (
                  <div className="flex-1 min-w-0">
                    <div className="h-2.5 bg-slate-100 rounded-full overflow-hidden">
                      <div
                        className={`h-full ${COR_BAR[cor]} rounded-full transition-all duration-700`}
                        style={{ width: `${barW}%` }}
                      />
                    </div>
                  </div>
                )
              }

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
      <span className="text-slate-400 text-xs ml-1">→ por</span>
      <span className="text-sm font-black text-red-600 uppercase tracking-wide">{nextLevelLabel}</span>
    </div>
  )
}

// ─── Hook de dados ─────────────────────────────────────────────────────────────

function useCards(
  view: string, compMode: string,
  refAno: number, refMes: number,
  compAno: number, compMes: number,
  drillPath: DrillStep[],
) {
  return useQuery<CardsResponse>({
    queryKey: ['farol-v2-cards', view, compMode, refAno, refMes, compAno, compMes, JSON.stringify(drillPath)],
    queryFn: async () => {
      const params = new URLSearchParams({
        view,
        comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(compAno > 0 && compMes > 0 && { comp_ano: String(compAno), comp_mes: String(compMes) }),
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
  const { user, spRole, tipoPersona } = useAuth()
  const canImport = user?.role === 'admin' || spRole === 'admin_fbtax' || tipoPersona === 'ti'

  const [view, setView]               = useState<'V01' | 'V02' | 'V03'>('V01')
  const [compMode, setCompMode]       = useState('yoy')
  const [drillPath, setDrillPath]     = useState<DrillStep[]>([])
  const [refAno, setRefAno]           = useState(0)
  const [refreshing, setRefreshing]   = useState(false)
  const [refMes, setRefMes]       = useState(0)
  // Override do mês de comparação (mom). 0 = automático (mês anterior).
  const [compAno, setCompAno]         = useState(0)
  const [compMes, setCompMes]         = useState(0)
  // Filtros de busca (frontend-only, operam sobre os cards já em memória)
  const [filterOpen, setFilterOpen]   = useState(false)
  const [filters, setFilters]         = useState({
    fornec:  '',   // Indústria
    gerente: '',   // Gerente GGV
    sup:     '',   // Equipe / Supervisor
    rca:     '',   // RCA
    cli:     '',   // Cliente
    prod:    '',   // Produto
  })

  const { data, isLoading, error } = useCards(
    view, compMode, refAno, refMes,
    compMode === 'mom' ? compAno : 0,
    compMode === 'mom' ? compMes : 0,
    drillPath,
  )

  const autoRef = useCallback((d: CardsResponse) => {
    if (refAno === 0 && d.periodo.ref_ano) {
      setRefAno(d.periodo.ref_ano)
      setRefMes(d.periodo.ref_mes)
    }
  }, [refAno])
  if (data && refAno === 0) autoRef(data)

  const resetFilters = () => setFilters({ fornec: '', gerente: '', sup: '', rca: '', cli: '', prod: '' })

  const handleDrill = (card: CardItem) => {
    if (card.level === 'cod_prod') return
    setDrillPath(prev => [...prev, { level: card.level, value: card.key, label: card.label }])
  }

  const handleBack = () => {
    setDrillPath(prev => prev.slice(0, -1))
  }

  // Usa o level do primeiro card (nível ATUAL exibido), não o next_level (nível filho)
  const activeFilterKey = useMemo((): keyof typeof filters | null => {
    const lv = (data?.cards?.[0]?.level ?? '').toLowerCase()
    if (lv.includes('fornec'))     return 'fornec'
    if (lv.includes('gerente'))    return 'gerente'
    if (lv.includes('supervisor')) return 'sup'
    if (lv.includes('rca'))        return 'rca'
    if (lv.includes('prod'))       return 'prod'
    if (lv.includes('cli'))        return 'cli'
    return null
  }, [data?.cards])

  const activeSearch = activeFilterKey ? filters[activeFilterKey] : ''

  // Busca com suporte a wildcard %:
  //   "mercado"   → contém (padrão amigável)
  //   "mercado%"  → começa com
  //   "%mercado"  → termina com
  //   "%mercado%" → contém (explícito)
  function matchTerm(label: string, raw: string): boolean {
    const t = raw.trim().toLowerCase()
    const l = label.toLowerCase()
    if (!t) return true
    const startsWild = t.startsWith('%')
    const endsWild   = t.endsWith('%')
    const core = t.replace(/^%|%$/g, '')
    if (!core) return true
    if (startsWild && endsWild) return l.includes(core)
    if (startsWild)             return l.endsWith(core)
    if (endsWild)               return l.startsWith(core)
    return l.includes(core)
  }

  const visibleCards = useMemo(
    () => {
      if (!activeSearch.trim() || !data?.cards) return data?.cards ?? []
      return data.cards.filter(c => matchTerm(c.label, activeSearch))
    },
    [data?.cards, activeSearch]
  )

  const hasActiveFilter = Object.values(filters).some(v => v.trim() !== '')

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
            <TooltipProvider key={m.id} delayDuration={400}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    onClick={() => { setCompMode(m.id); setDrillPath([]) }}
                    className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                      compMode === m.id
                        ? 'bg-slate-800 text-white'
                        : 'text-slate-600 hover:bg-slate-50'
                    }`}
                  >
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

        {/* Seletor "Comparar com" — só no mês a mês */}
        {compMode === 'mom' && periodos.length > 0 && (
          <>
            <span className="text-xs text-slate-400">comparar com</span>
            <select
              value={compAno > 0 ? `${compAno}-${String(compMes).padStart(2, '0')}` : ''}
              onChange={e => {
                if (e.target.value === '') { setCompAno(0); setCompMes(0); return }
                const p = parsePeriodo(e.target.value)
                setCompAno(p.ano)
                setCompMes(p.mes)
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

        {/* Botão Filtros */}
        {data && (
          <button
            onClick={() => setFilterOpen(o => !o)}
            className={`flex items-center gap-1.5 h-8 px-3 rounded-lg border text-xs font-medium transition-colors shrink-0 ${
              hasActiveFilter
                ? 'border-primary bg-primary/10 text-primary'
                : filterOpen
                  ? 'border-slate-400 bg-slate-100 text-slate-700'
                  : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-50'
            }`}
          >
            <SlidersHorizontal className="h-3.5 w-3.5" />
            Filtros
            {hasActiveFilter && (
              <span className="ml-0.5 w-4 h-4 rounded-full bg-primary text-white text-[10px] flex items-center justify-center">
                {Object.values(filters).filter(v => v.trim()).length}
              </span>
            )}
          </button>
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

        {/* Botão importar — somente admin/TI */}
        {canImport && (
          <button
            onClick={() => navigate('/farol/importar')}
            className="flex items-center gap-1.5 h-8 px-3 rounded-lg border border-dashed border-slate-300 text-xs text-slate-500 hover:border-primary hover:text-primary transition-colors shrink-0"
          >
            <UploadCloud className="h-3.5 w-3.5" />
            Importar dados
          </button>
        )}
      </div>

      {/* ── Painel de filtros ─────────────────────────────────────────────── */}
      {filterOpen && data && (() => {
        const FILTER_FIELDS: Array<{ key: keyof typeof filters; label: string }> = [
          { key: 'fornec',  label: 'Indústria'     },
          { key: 'gerente', label: 'Gerente (GGV)' },
          { key: 'sup',     label: 'Equipe (SUPV)' },
          { key: 'rca',     label: 'RCA'           },
          { key: 'cli',     label: 'Cliente'       },
          { key: 'prod',    label: 'Produto'       },
        ]

        return (
          <div className="mb-4 p-4 bg-white rounded-xl border border-slate-200 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <p className="text-xs font-semibold text-slate-600 uppercase tracking-wider flex items-center gap-1.5">
                <Search className="h-3.5 w-3.5" /> Filtrar por
              </p>
              {hasActiveFilter && (
                <button onClick={resetFilters} className="text-[11px] text-red-500 hover:text-red-700 flex items-center gap-0.5">
                  <X className="h-3 w-3" /> Limpar filtros
                </button>
              )}
            </div>
            <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
              {FILTER_FIELDS.map(f => {
                const isCurrent = activeFilterKey === f.key
                return (
                  <div key={f.key}>
                    <label className={`block text-[11px] font-medium mb-1 ${isCurrent ? 'text-primary' : 'text-slate-500'}`}>
                      {f.label}
                      {isCurrent && <span className="ml-1 text-[10px] text-primary/60">● ativo</span>}
                    </label>
                    <div className="relative">
                      <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-slate-400 pointer-events-none" />
                      <input
                        type="text"
                        value={filters[f.key]}
                        onChange={e => setFilters(prev => ({ ...prev, [f.key]: e.target.value }))}
                        placeholder={`Buscar ${f.label}...`}
                        className={`w-full h-7 pl-6 pr-6 rounded-lg border text-xs transition-colors focus:outline-none focus:ring-2 focus:ring-primary/30 bg-white text-slate-700 placeholder:text-slate-400 ${
                          isCurrent ? 'border-primary/40' : 'border-slate-200'
                        }`}
                      />
                      {filters[f.key] && (
                        <button
                          onClick={() => setFilters(prev => ({ ...prev, [f.key]: '' }))}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-slate-400 hover:text-slate-600"
                        >
                          <X className="h-3 w-3" />
                        </button>
                      )}
                    </div>
                  </div>
                )
              })}
            </div>
            <p className="mt-3 text-[10px] text-slate-400">
              Dica: use <span className="font-mono bg-slate-100 px-1 rounded">%</span> como curinga —{' '}
              <span className="font-mono bg-slate-100 px-1 rounded">mini%</span> começa com,{' '}
              <span className="font-mono bg-slate-100 px-1 rounded">%mercado</span> termina com.
              Os filtros persistem durante o drill — configure antes de navegar.
            </p>
            {activeSearch && (
              <p className="mt-1 text-[11px] text-slate-500">
                Exibindo <span className="font-semibold text-primary">{visibleCards.length}</span> de {data.cards.length} {data.next_level_label?.toLowerCase() ?? 'itens'}
              </p>
            )}
          </div>
        )
      })()}

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
          {canImport ? (
            <>
              <p className="text-slate-600 text-xs mb-6">Importe um arquivo CSV para visualizar o painel executivo.</p>
              <button
                onClick={() => navigate('/farol/importar')}
                className="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-primary text-white text-sm font-semibold hover:bg-primary/90 transition-colors"
              >
                <UploadCloud className="h-4 w-4" />
                Ir para importação
              </button>
            </>
          ) : (
            <p className="text-slate-600 text-xs">Solicite ao administrador a importação dos dados.</p>
          )}
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
            cards={visibleCards}
            levelLabel={data.next_level_label}
            onDrill={handleDrill}
            wallStreet={view === 'V01'}
          />
        </>
      )}
    </div>
  )
}
