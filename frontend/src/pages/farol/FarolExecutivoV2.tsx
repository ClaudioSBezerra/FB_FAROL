import { useState, useEffect, useMemo, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  ChevronLeft, Search, X, Filter, ChevronDown, TrendingUp, TrendingDown,
  ArrowRight, Sparkles, Target, Users, Boxes,
} from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { cn } from '@/lib/utils'

// FarolExecutivo V2 — variante visual "Premium Clean Modern"
// • Mesma lógica/endpoints do FarolExecutivo (V1) — só muda a camada visual
// • Tipografia maior, hierarquia forte, glassmorphism leve, paleta slate+blue
// • Lista de drill em formato de cards horizontais com mini-bars de progresso

// ─── Tipos ────────────────────────────────────────────────────────────────────

type Cor = 'verde' | 'amarelo' | 'vermelho'
type Fluxo = 'faturado' | 'transmitido'

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
  plucro: number
  plucro_ant: number
  positivados: number
  base_cli: number
  positpct: number
  positivados_ant: number
  base_cli_ant: number
  positpct_ant: number
  posit_cor: Cor
  mix: number
  mix_ant: number
  mix_cor: Cor
  mix_total: number
  mix_total_ant: number
}

interface KPI {
  total_atual: number
  total_ant: number
  total_pct: number
  total_cor: Cor
  total_plucro: number
  total_plucro_ant: number
  total_positivados: number
  total_base_cli: number
  total_positpct: number
  total_positivados_ant: number
  total_base_cli_ant: number
  total_positpct_ant: number
  total_posit_cor: Cor
  avg_mix: number
  avg_mix_ant: number
  mix_cor: Cor
  total_mix_total: number
  total_mix_total_ant: number
}

interface CardsResponse {
  cards: CardItem[]
  kpi: KPI
  periodo: {
    fluxo?: string
    ref_inicio?: string; ref_fim?: string
    comp_inicio?: string; comp_fim?: string
    ref_ano?: number; ref_mes?: number
    label?: string
  }
  periodos: string[]
  view: string
  drill_path: DrillStep[]
  next_level: string
  next_level_label: string
}

interface DimOption { key: string; label: string }
interface DimsResponse {
  fornec?: DimOption[]
  gerente?: DimOption[]
  supervisor?: DimOption[]
  rca?: DimOption[]
  cli?: DimOption[]
  uf?: string[]
  empresa?: string[]
}

// ─── Utilitários ──────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v >= 1_000_000_000) return 'R$ ' + (v / 1_000_000_000).toFixed(2).replace('.', ',') + 'B'
  if (v >= 1_000_000)     return 'R$ ' + (v / 1_000_000).toFixed(1).replace('.', ',') + 'M'
  if (v >= 1_000)         return 'R$ ' + (v / 1_000).toFixed(0).replace(/\B(?=(\d{3})+(?!\d))/g, '.') + 'K'
  if (v === 0)            return '—'
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0 })
}
function fmtPct(v: number) {
  if (!isFinite(v) || v === 0) return '—'
  if (v >= 10000) return '>9999%'
  return Math.round(v) + '%'
}
function fmtInt(v: number) {
  if (v === 0) return '—'
  return v.toLocaleString('pt-BR', { maximumFractionDigits: 0 })
}
function fmtMix(v: number) {
  if (v === 0) return '—'
  return v.toLocaleString('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
}

// Paleta semântica V2 — mais saturada que V1
const COR_TXT: Record<Cor, string> = {
  verde:    'text-emerald-600',
  amarelo:  'text-amber-500',
  vermelho: 'text-rose-600',
}
const COR_BG: Record<Cor, string> = {
  verde:    'bg-emerald-50 ring-emerald-200',
  amarelo:  'bg-amber-50 ring-amber-200',
  vermelho: 'bg-rose-50 ring-rose-200',
}
const COR_DOT: Record<Cor, string> = {
  verde:    'bg-emerald-500',
  amarelo:  'bg-amber-500',
  vermelho: 'bg-rose-500',
}

// ─── Preset de período ───────────────────────────────────────────────────────

function ymd(y: number, m: number, d: number): string {
  return `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
}
function lastDayOfMonth(y: number, m: number): number { return new Date(y, m, 0).getDate() }
function addDays(s: string, days: number): string {
  const [y, m, d] = s.split('-').map(Number)
  const dt = new Date(Date.UTC(y, m - 1, d))
  dt.setUTCDate(dt.getUTCDate() + days)
  return dt.toISOString().slice(0, 10)
}
function todayYMD(): string { return new Date().toISOString().slice(0, 10) }
void todayYMD
function rangeDaysInclusive(ini: string, fim: string): number {
  const [yi, mi, di] = ini.split('-').map(Number)
  const [yf, mf, df] = fim.split('-').map(Number)
  return Math.round((Date.UTC(yf, mf - 1, df) - Date.UTC(yi, mi - 1, di)) / 86_400_000) + 1
}
function fmtDateBR(s: string): string {
  if (!s) return ''
  const [y, m, d] = s.split('-')
  return `${d}/${m}/${y}`
}

type Preset = 'mes_corrente' | 'yoy' | 'ant_corrente' | 'ytd' | 'last7' | 'last30'

const PRESET_LABEL: Record<Preset, string> = {
  ytd:          'Ano × Ano',
  yoy:          'Último mês YoY',
  ant_corrente: 'M-1 vs M-2',
  mes_corrente: 'Mês Corrente',
  last7:        '7 dias',
  last30:       '30 dias',
}

function presetRange(p: Preset, last?: { ano: number; mes: number }) {
  const now = new Date()
  const todayY = now.getUTCFullYear()
  const todayM = now.getUTCMonth() + 1
  const todayD = now.getUTCDate()
  const today = ymd(todayY, todayM, todayD)

  const lastY = last?.ano ?? todayY
  const lastM = last?.mes ?? (todayM > 1 ? todayM - 1 : 12)

  switch (p) {
    case 'ytd': {
      return {
        ref_inicio:  ymd(todayY, 1, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, 1, 1),
        comp_fim:    ymd(todayY - 1, 12, 31),
      }
    }
    case 'yoy': {
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(lastY - 1, lastM, 1),
        comp_fim:    ymd(lastY - 1, lastM, lastDayOfMonth(lastY - 1, lastM)),
      }
    }
    case 'ant_corrente': {
      let prevM = lastM - 1, prevY = lastY
      if (prevM === 0) { prevM = 12; prevY-- }
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(prevY, prevM, 1),
        comp_fim:    ymd(prevY, prevM, lastDayOfMonth(prevY, prevM)),
      }
    }
    case 'mes_corrente': {
      let pm = todayM - 1, py = todayY
      if (pm === 0) { pm = 12; py-- }
      const dayCap = Math.min(todayD, lastDayOfMonth(py, pm))
      return {
        ref_inicio:  ymd(todayY, todayM, 1),
        ref_fim:     today,
        comp_inicio: ymd(py, pm, 1),
        comp_fim:    ymd(py, pm, dayCap),
      }
    }
    case 'last7': {
      const fim = today
      const ini = addDays(fim, -6)
      return {
        ref_inicio:  ini,
        ref_fim:     fim,
        comp_inicio: addDays(ini, -7),
        comp_fim:    addDays(fim, -7),
      }
    }
    case 'last30':
    default: {
      const fim = today
      const ini = addDays(fim, -29)
      return {
        ref_inicio:  ini,
        ref_fim:     fim,
        comp_inicio: addDays(ini, -30),
        comp_fim:    addDays(fim, -30),
      }
    }
  }
}

// ─── KPI Hero Card (top do painel) ───────────────────────────────────────────

interface KpiHeroProps {
  kpi: KPI | undefined
  refLabel?: string
  compLabel?: string
  isLoading?: boolean
}

function deltaIcon(cor: Cor) {
  if (cor === 'verde') return <TrendingUp className="h-4 w-4" />
  if (cor === 'vermelho') return <TrendingDown className="h-4 w-4" />
  return <ArrowRight className="h-4 w-4" />
}

function KpiHero({ kpi, refLabel, compLabel, isLoading }: KpiHeroProps) {
  if (isLoading) {
    return (
      <div
        className="rounded-2xl p-6 mb-6 animate-pulse bg-gradient-to-b from-slate-100 to-slate-200 ring-1 ring-slate-300/60"
        style={{
          boxShadow:
            'inset 0 1px 0 rgba(255,255,255,0.9), inset 0 -1px 0 rgba(15,23,42,0.06), 0 12px 28px -10px rgba(15,23,42,0.18), 0 4px 10px -4px rgba(15,23,42,0.10)',
        }}
      >
        <div className="h-6 w-32 bg-slate-300/60 rounded mb-4" />
        <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i}>
              <div className="h-4 w-24 bg-slate-300/60 rounded mb-2" />
              <div className="h-10 w-40 bg-slate-300/60 rounded" />
            </div>
          ))}
        </div>
      </div>
    )
  }
  if (!kpi) return null

  return (
    <div
      className="relative rounded-2xl overflow-hidden p-6 mb-6 bg-gradient-to-b from-slate-100 via-slate-150 to-slate-300 ring-1 ring-slate-300/70"
      style={{
        backgroundImage:
          'linear-gradient(180deg, #f1f5f9 0%, #e2e8f0 55%, #cbd5e1 100%)',
        boxShadow: [
          // Highlight superior interno (luz incidente)
          'inset 0 1px 0 rgba(255,255,255,0.95)',
          'inset 0 2px 6px rgba(255,255,255,0.55)',
          // Sombra inferior interna (peso/baixo-relevo)
          'inset 0 -1px 0 rgba(15,23,42,0.08)',
          'inset 0 -8px 16px rgba(15,23,42,0.06)',
          // Sombras externas (elevação 3D)
          '0 1px 0 rgba(255,255,255,0.6)',
          '0 14px 28px -10px rgba(15,23,42,0.25)',
          '0 6px 14px -4px rgba(15,23,42,0.14)',
        ].join(', '),
      }}
    >
      {/* Glow decorativos suaves no fundo cinza */}
      <div className="absolute -top-16 -right-16 h-56 w-56 rounded-full bg-white/40 blur-3xl pointer-events-none" />
      <div className="absolute -bottom-16 -left-16 h-48 w-48 rounded-full bg-slate-400/20 blur-3xl pointer-events-none" />

      <div className="relative">
        <div className="flex items-center gap-2 mb-4">
          <div
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-white text-[11px] font-bold tracking-wider uppercase"
            style={{
              backgroundImage:
                'linear-gradient(180deg, #1e293b 0%, #0f172a 100%)',
              boxShadow:
                'inset 0 1px 0 rgba(255,255,255,0.18), 0 4px 10px -2px rgba(15,23,42,0.45)',
            }}
          >
            <Sparkles className="h-3 w-3" />
            Total Geral
          </div>
          {compLabel && refLabel && (
            <span className="text-xs text-slate-500">
              <span className="text-orange-600 font-medium">{compLabel}</span>
              <span className="mx-1.5 text-slate-300">vs</span>
              <span className="text-orange-700 font-medium">{refLabel}</span>
            </span>
          )}
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-6">
          {/* VENDA */}
          <KpiBox
            icon={<Target className="h-4 w-4" />}
            label="Venda"
            valueMain={fmtBRL(kpi.total_atual)}
            valuePrev={fmtBRL(kpi.total_ant)}
            pct={kpi.total_pct}
            cor={kpi.total_cor}
          />
          {/* POSITIVAÇÃO */}
          <KpiBox
            icon={<Users className="h-4 w-4" />}
            label="Positivação"
            valueMain={fmtPct(kpi.total_positpct)}
            valuePrev={`${fmtInt(kpi.total_positivados)} / ${fmtInt(kpi.total_base_cli)}`}
            pct={kpi.total_positpct - kpi.total_positpct_ant}
            cor={kpi.total_posit_cor}
            isPctValue
          />
          {/* MIX */}
          <KpiBox
            icon={<Boxes className="h-4 w-4" />}
            label="Mix Médio"
            valueMain={kpi.total_mix_total > 0
              ? `${fmtMix(kpi.avg_mix)} / ${fmtInt(kpi.total_mix_total)}`
              : fmtMix(kpi.avg_mix)}
            valuePrev={`Antes: ${fmtMix(kpi.avg_mix_ant)}${kpi.total_mix_total_ant > 0 ? ` / ${fmtInt(kpi.total_mix_total_ant)}` : ''}`}
            pct={(kpi.avg_mix - kpi.avg_mix_ant) / Math.max(0.001, kpi.avg_mix_ant) * 100}
            cor={kpi.mix_cor}
          />
          {/* CLIENTES ATIVOS */}
          <KpiBox
            icon={<Users className="h-4 w-4" />}
            label="Clientes Ativos"
            valueMain={fmtInt(kpi.total_base_cli)}
            valuePrev={`${fmtInt(kpi.total_positivados)} positivados`}
            pct={0}
            cor="verde"
            hideDelta
          />
        </div>
      </div>
    </div>
  )
}

interface KpiBoxProps {
  icon: React.ReactNode
  label: string
  valueMain: string
  valuePrev: string
  pct: number
  cor: Cor
  isPctValue?: boolean
  hideDelta?: boolean
}

function KpiBox({ icon, label, valueMain, valuePrev, pct, cor, hideDelta }: KpiBoxProps) {
  return (
    <div
      className="relative rounded-xl bg-white/85 backdrop-blur-sm px-4 py-3 ring-1 ring-white/80"
      style={{
        boxShadow:
          'inset 0 1px 0 rgba(255,255,255,0.9), 0 6px 14px -6px rgba(15,23,42,0.18), 0 2px 4px -2px rgba(15,23,42,0.08)',
      }}
    >
      <div className="flex items-center gap-1.5 text-slate-500 text-sm font-bold tracking-wider uppercase mb-1">
        <span className="text-slate-400">{icon}</span>
        {label}
      </div>
      <div className="text-3xl md:text-4xl font-bold tabular-nums tracking-tight text-slate-900 drop-shadow-sm">
        {valueMain}
      </div>
      <div className="flex items-center gap-2 mt-2">
        {!hideDelta && (
          <span className={cn(
            'inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-sm font-bold tabular-nums ring-1',
            COR_BG[cor], COR_TXT[cor],
          )}>
            {deltaIcon(cor)}
            {fmtPct(Math.abs(pct))}
          </span>
        )}
        <span className="text-base font-semibold tabular-nums text-slate-600 truncate">{valuePrev}</span>
      </div>
    </div>
  )
}

// ─── Linha de fornecedor — formato card horizontal moderno ───────────────────

interface CardRowProps {
  card: CardItem
  onClick?: () => void
  index: number
}

// Grid compartilhado entre o cabeçalho da lista e cada CardRow
const LISTA_GRID = 'grid grid-cols-12 gap-3 items-center'

function ListaHeader() {
  return (
    <div className="px-4 py-2 mb-2 rounded-lg bg-slate-100/70 ring-1 ring-slate-200">
      <div className={LISTA_GRID}>
        <div className="col-span-12 md:col-span-3 text-sm font-bold tracking-wider uppercase text-slate-500">
          Nome
        </div>
        <div className="hidden md:block md:col-span-3 text-sm font-bold tracking-wider uppercase text-slate-500">
          Venda
        </div>
        <div className="hidden md:block md:col-span-4 text-sm font-bold tracking-wider uppercase text-slate-500">
          Positivação
        </div>
        <div className="hidden md:block md:col-span-2 text-sm font-bold tracking-wider uppercase text-slate-500 text-right">
          Mix Médio
        </div>
      </div>
    </div>
  )
}

function CardRow({ card, onClick, index }: CardRowProps) {
  const clickable = !!onClick
  // Barra de progresso = positpct (0-100)
  const positPct = Math.max(0, Math.min(100, card.positpct))

  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!clickable}
      className={cn(
        'w-full text-left rounded-xl bg-white ring-1 ring-slate-200 p-4 transition-all',
        clickable && 'hover:ring-blue-300 hover:shadow-md hover:-translate-y-0.5 cursor-pointer',
        !clickable && 'cursor-default',
      )}
    >
      <div className={LISTA_GRID}>
        {/* Identificação (3 cols) */}
        <div className="col-span-12 md:col-span-3 flex items-center gap-3 min-w-0">
          <div className="flex-shrink-0 h-10 w-10 rounded-lg bg-gradient-to-br from-slate-100 to-slate-200 ring-1 ring-slate-200 flex items-center justify-center text-slate-600 font-bold text-sm tabular-nums">
            {String(index + 1).padStart(2, '0')}
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-1.5">
              <span className={cn('h-2.5 w-2.5 rounded-full', COR_DOT[card.cor])} />
              <h3 className="text-base font-semibold text-slate-900 truncate" title={card.label}>
                {card.label}
              </h3>
            </div>
          </div>
        </div>

        {/* Venda (3 cols) */}
        <div className="col-span-6 md:col-span-3">
          <div className="text-xl font-bold tabular-nums text-slate-900 leading-tight">
            {fmtBRL(card.valor_atual)}
          </div>
          <div className="flex items-center gap-2 mt-0.5">
            <span className={cn('inline-flex items-center gap-0.5 text-sm font-bold tabular-nums', COR_TXT[card.cor])}>
              {deltaIcon(card.cor)}{fmtPct(card.pct)}
            </span>
            <span className="text-sm font-medium text-slate-500 tabular-nums">
              vs {fmtBRL(card.valor_ant)}
            </span>
          </div>
        </div>

        {/* Positivação com barra (4 cols) */}
        <div className="col-span-6 md:col-span-4">
          <div className="flex items-baseline gap-2">
            <span className={cn('text-xl font-bold tabular-nums leading-tight', COR_TXT[card.posit_cor])}>
              {fmtPct(card.positpct)}
            </span>
            <span className="text-sm font-medium text-slate-600 tabular-nums">
              {fmtInt(card.positivados)} / {fmtInt(card.base_cli)}
            </span>
          </div>
          <div className="mt-1.5 h-2 w-full rounded-full bg-slate-100 overflow-hidden">
            <div
              className={cn(
                'h-full rounded-full transition-all',
                card.posit_cor === 'verde'    && 'bg-emerald-500',
                card.posit_cor === 'amarelo'  && 'bg-amber-500',
                card.posit_cor === 'vermelho' && 'bg-rose-500',
              )}
              style={{ width: `${positPct}%` }}
            />
          </div>
        </div>

        {/* Mix (2 cols) — "X de Y" (X = média por cliente, Y = SKUs distintos do fornec) */}
        <div className="col-span-12 md:col-span-2 flex flex-col items-end justify-center gap-0">
          <div className="flex items-baseline gap-1.5">
            <span className={cn('text-xl font-bold tabular-nums leading-tight', COR_TXT[card.mix_cor])}>
              {fmtMix(card.mix)}
            </span>
            {card.mix_total > 0 && (
              <span className="text-sm font-medium text-slate-500 tabular-nums">
                / {fmtInt(card.mix_total)}
              </span>
            )}
          </div>
          {card.mix_total_ant > 0 && (
            <span className="text-xs font-medium text-slate-400 tabular-nums">
              vs {fmtMix(card.mix_ant)} / {fmtInt(card.mix_total_ant)}
            </span>
          )}
        </div>
      </div>
    </button>
  )
}

// ─── DateRangeFilter (visual modernizado) ─────────────────────────────────────

interface DateRangeFilterProps {
  label: string
  inicio: string
  fim: string
  onChangeInicio: (v: string) => void
  onChangeFim: (v: string) => void
  accent?: string
}

function DateRangeFilter({ label, inicio, fim, onChangeInicio, onChangeFim, accent = 'bg-blue-500' }: DateRangeFilterProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [])

  const hasValue = inicio && fim
  const summary = hasValue ? `${fmtDateBR(inicio)} → ${fmtDateBR(fim)}` : 'Selecionar'

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className={cn(
          'inline-flex items-center gap-2 px-3 py-2 text-sm font-medium border rounded-lg bg-white shadow-sm transition-colors',
          hasValue ? 'border-slate-300 text-slate-900 hover:border-slate-400' : 'border-slate-200 text-slate-500 hover:bg-slate-50',
        )}
      >
        <span className={cn('inline-block h-2 w-2 rounded-full', accent)} />
        <span className="text-xs text-slate-500 font-semibold">{label}</span>
        <span>{summary}</span>
        <ChevronDown className="h-3.5 w-3.5 opacity-60" />
      </button>
      {open && (
        <div className="absolute left-0 top-full mt-1.5 z-50 bg-white border border-slate-200 rounded-xl shadow-xl p-4 min-w-[300px]">
          <div className="text-xs uppercase tracking-wider text-slate-500 font-semibold mb-2">{label}</div>
          <div className="flex gap-2 items-center">
            <input
              type="date"
              value={inicio}
              onChange={e => onChangeInicio(e.target.value)}
              className="flex-1 px-2 py-1.5 text-sm border border-slate-200 rounded-md bg-white"
            />
            <span className="text-slate-300 text-sm">→</span>
            <input
              type="date"
              value={fim}
              onChange={e => onChangeFim(e.target.value)}
              className="flex-1 px-2 py-1.5 text-sm border border-slate-200 rounded-md bg-white"
            />
          </div>
          {inicio && fim && (
            <div className="text-xs text-slate-500 mt-2">
              {rangeDaysInclusive(inicio, fim)} dia(s)
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── MultiSelect (visual modernizado) ─────────────────────────────────────────

interface MultiSelectProps {
  label: string
  options: { key: string; label: string }[]
  selected: string[]
  onChange: (next: string[]) => void
}

function MultiSelect({ label, options, selected, onChange }: MultiSelectProps) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [])

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    if (!s) return options
    return options.filter(o => o.label.toLowerCase().includes(s) || o.key.toLowerCase().includes(s))
  }, [options, q])

  const toggle = (k: string) => {
    onChange(selected.includes(k) ? selected.filter(x => x !== k) : [...selected, k])
  }

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className={cn(
          'inline-flex items-center gap-1.5 px-3 py-2 text-sm font-medium border rounded-lg bg-white shadow-sm transition-colors',
          selected.length > 0 ? 'border-blue-500 text-slate-900' : 'border-slate-200 text-slate-600 hover:bg-slate-50',
        )}
      >
        {label}
        {selected.length > 0 && (
          <span className="inline-flex items-center justify-center min-w-[20px] h-5 px-1.5 rounded-full bg-blue-600 text-white text-[11px] font-bold">
            {selected.length}
          </span>
        )}
        <ChevronDown className="h-3.5 w-3.5 opacity-60" />
      </button>

      {open && (
        <div className="absolute left-0 top-full mt-1.5 z-50 w-72 bg-white border border-slate-200 rounded-xl shadow-xl overflow-hidden">
          <div className="p-2 border-b border-slate-100">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400" />
              <input
                autoFocus
                value={q}
                onChange={e => setQ(e.target.value)}
                placeholder={`Buscar ${label.toLowerCase()}...`}
                className="w-full pl-8 pr-2 py-1.5 text-sm border border-slate-200 rounded-lg"
              />
            </div>
          </div>
          <div className="max-h-64 overflow-y-auto">
            {filtered.length === 0 && (
              <div className="px-3 py-4 text-center text-sm text-slate-400">Nenhum resultado</div>
            )}
            {filtered.map(opt => {
              const checked = selected.includes(opt.key)
              return (
                <label key={opt.key} className="flex items-center gap-2 px-3 py-2 hover:bg-slate-50 cursor-pointer text-sm">
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggle(opt.key)}
                    className="w-4 h-4 accent-blue-600"
                  />
                  <span className={cn('truncate', checked && 'font-medium text-slate-900')}>{opt.label}</span>
                </label>
              )
            })}
          </div>
          {selected.length > 0 && (
            <div className="border-t border-slate-100 p-2 flex items-center justify-between">
              <span className="text-xs text-slate-500">{selected.length} selecionado(s)</span>
              <button onClick={() => onChange([])} className="text-xs text-slate-600 hover:text-rose-600 font-medium">
                Limpar
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

interface UseCardsArgs {
  view: string
  fluxo: Fluxo
  ref_inicio: string
  ref_fim: string
  comp_inicio: string
  comp_fim: string
  drillPath: DrillStep[]
  filters: Record<string, string[]>
}

function useCards(a: UseCardsArgs) {
  return useQuery<CardsResponse>({
    queryKey: ['farol-v2-cards', a.view, a.fluxo, a.ref_inicio, a.ref_fim, a.comp_inicio, a.comp_fim,
      JSON.stringify(a.drillPath), JSON.stringify(a.filters)],
    enabled: !!a.ref_inicio && !!a.ref_fim,
    queryFn: async () => {
      const p = new URLSearchParams({
        view: a.view, fluxo: a.fluxo,
        ref_inicio: a.ref_inicio, ref_fim: a.ref_fim,
      })
      if (a.comp_inicio && a.comp_fim) { p.set('comp_inicio', a.comp_inicio); p.set('comp_fim', a.comp_fim) }
      if (a.drillPath.length > 0) p.set('drill', JSON.stringify(a.drillPath))
      Object.entries(a.filters).forEach(([k, v]) => { if (v.length > 0) p.set(k, v.join(',')) })
      const r = await fetch(`/api/v2/farol/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dados')
      return r.json()
    },
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

function useDims(fluxo: Fluxo, ref_inicio: string, ref_fim: string) {
  return useQuery<DimsResponse>({
    queryKey: ['farol-v2-dims', fluxo, ref_inicio, ref_fim],
    enabled: !!ref_inicio && !!ref_fim,
    queryFn: async () => {
      const p = new URLSearchParams({ fluxo, ref_inicio, ref_fim })
      const r = await fetch(`/api/v2/farol/dims?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dimensões')
      return r.json()
    },
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

function useUltimoPeriodo() {
  return useQuery<{ ref_ano?: number; ref_mes?: number; periodos: string[] }>({
    queryKey: ['farol-v2-periodos'],
    queryFn: async () => {
      const r = await fetch('/api/v2/farol/cards?view=V01')
      if (!r.ok) throw new Error()
      const d = await r.json() as CardsResponse
      return { ref_ano: d.periodo.ref_ano, ref_mes: d.periodo.ref_mes, periodos: d.periodos }
    },
    staleTime: 60 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// ─── FarolExecutivoV2 ────────────────────────────────────────────────────────

export default function FarolExecutivoV2() {
  const navigate = useNavigate()
  const { spRole, tipoPersona } = useAuth()
  void spRole; void tipoPersona

  const [view, setView] = useState<'V01' | 'V02' | 'V03'>('V01')
  const [fluxo, setFluxo] = useState<Fluxo>('faturado')
  const [drillPath, setDrillPath] = useState<DrillStep[]>([])
  const [search, setSearch] = useState('')
  const [activePreset, setActivePreset] = useState<Preset | null>('ytd')

  const [refInicio, setRefInicio] = useState('')
  const [refFim, setRefFim] = useState('')
  const [compInicio, setCompInicio] = useState('')
  const [compFim, setCompFim] = useState('')

  const [filters, setFilters] = useState<Record<string, string[]>>({})
  const setFilter = (col: string, vals: string[]) => {
    setFilters(prev => {
      const next = { ...prev }
      if (vals.length === 0) delete next[col]
      else next[col] = vals
      return next
    })
  }
  const clearFilters = () => setFilters({})

  const periodosQ = useUltimoPeriodo()
  useEffect(() => {
    if (refInicio || !periodosQ.data) return
    const r = presetRange('ytd', { ano: periodosQ.data.ref_ano!, mes: periodosQ.data.ref_mes! })
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }, [periodosQ.data, refInicio])

  const applyPreset = (p: Preset) => {
    setActivePreset(p)
    const ps = periodosQ.data?.periodos ?? []
    const latestStr = ps[0]
    const parsePeriodo = (s: string) => { const [y, m] = s.split('-'); return { ano: +y, mes: +m } }
    const last = latestStr
      ? parsePeriodo(latestStr)
      : (periodosQ.data?.ref_ano && periodosQ.data?.ref_mes)
        ? { ano: periodosQ.data.ref_ano, mes: periodosQ.data.ref_mes }
        : undefined
    const r = presetRange(p, last)
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }

  const { data, isLoading, error } = useCards({
    view, fluxo, ref_inicio: refInicio, ref_fim: refFim,
    comp_inicio: compInicio, comp_fim: compFim,
    drillPath, filters,
  })
  const dimsQ = useDims(fluxo, refInicio, refFim)

  const handleDrill = (card: CardItem) => {
    if (card.level === 'cod_prod') return
    setDrillPath(prev => [...prev, { level: card.level, value: card.key, label: card.label }])
  }
  const handleBack = () => setDrillPath(prev => prev.slice(0, -1))
  const handleViewChange = (v: 'V01' | 'V02' | 'V03') => { setView(v); setDrillPath([]) }

  const cards = data?.cards ?? []
  const kpi = data?.kpi

  const visibleCards = useMemo(() => {
    const s = search.trim().toLowerCase()
    if (!s) return cards
    return cards.filter(c => c.label.toLowerCase().includes(s))
  }, [cards, search])

  const FILTER_DIMS: { col: string; label: string; from: keyof DimsResponse }[] = [
    { col: 'cod_fornec',     label: 'Indústria',  from: 'fornec' },
    { col: 'cod_gerente',    label: 'Gerente',    from: 'gerente' },
    { col: 'cod_supervisor', label: 'Supervisor', from: 'supervisor' },
    { col: 'cod_rca',        label: 'RCA',        from: 'rca' },
    { col: 'cod_cli',        label: 'Cliente',    from: 'cli' },
    { col: 'uf',             label: 'UF',         from: 'uf' },
    { col: 'empresa',        label: 'Filial',     from: 'empresa' },
  ]

  const optionsFor = (from: keyof DimsResponse): { key: string; label: string }[] => {
    const v = dimsQ.data?.[from]
    if (!v) return []
    if (Array.isArray(v) && typeof v[0] === 'string') {
      return (v as string[]).map(s => ({ key: s, label: s }))
    }
    return v as DimOption[]
  }

  const totalFiltersActive = Object.values(filters).reduce((n, vs) => n + vs.length, 0)
  const refLabel  = refInicio && refFim ? `${fmtDateBR(refInicio)} → ${fmtDateBR(refFim)}` : undefined
  const compLabel = compInicio && compFim ? `${fmtDateBR(compInicio)} → ${fmtDateBR(compFim)}` : undefined

  return (
    <div className="min-h-full bg-slate-50/60 p-4 md:p-6">
      {/* ── Top bar: títuo + toggle V1 ─────────────────────────────────────── */}
      <div className="flex items-center justify-between mb-5 flex-wrap gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl md:text-3xl font-bold tracking-tight text-slate-900">
              Painel Executivo
            </h1>
            <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-blue-100 text-blue-700 text-[11px] font-bold tracking-wide">
              <Sparkles className="h-3 w-3" />
              V2 BETA
            </span>
          </div>
          <p className="text-sm text-slate-500 mt-0.5">
            Visão de objetivos por Indústria, Gerência e Equipe — com drill-down até cliente/produto.
          </p>
        </div>
        <button
          onClick={() => navigate('/farol/v2')}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-slate-600 bg-white border border-slate-200 rounded-lg hover:bg-slate-50 transition-colors"
        >
          <ChevronLeft className="h-4 w-4" />
          Voltar para V1
        </button>
      </div>

      {/* ── Linha 1: Fluxo + View ──────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-3 mb-4">
        {/* FLUXO */}
        <div className="inline-flex rounded-lg ring-1 ring-slate-200 overflow-hidden bg-white shadow-sm">
          {([
            { id: 'faturado'    as const, label: 'Faturado',    accent: 'bg-slate-900' },
            { id: 'transmitido' as const, label: 'Transmitido', accent: 'bg-emerald-600' },
          ]).map(f => (
            <button
              key={f.id}
              onClick={() => { setFluxo(f.id); setDrillPath([]) }}
              className={cn(
                'px-4 py-2 text-sm font-semibold tracking-wide transition-colors',
                fluxo === f.id ? cn(f.accent, 'text-white') : 'text-slate-600 hover:bg-slate-50',
              )}
            >
              {f.label}
            </button>
          ))}
        </div>

        {/* VIEW */}
        <div className="inline-flex rounded-lg ring-1 ring-slate-200 overflow-hidden bg-white shadow-sm">
          {([
            { id: 'V01' as const, label: 'Por Indústria' },
            { id: 'V03' as const, label: 'Por Gerência' },
            { id: 'V02' as const, label: 'Por Equipe' },
          ]).map(v => (
            <button
              key={v.id}
              onClick={() => handleViewChange(v.id)}
              className={cn(
                'px-3.5 py-2 text-sm font-medium transition-colors',
                view === v.id ? 'bg-blue-600 text-white' : 'text-slate-600 hover:bg-slate-50',
              )}
            >
              {v.label}
            </button>
          ))}
        </div>

        {/* Presets de período */}
        <div className="inline-flex flex-wrap gap-1.5">
          {(Object.keys(PRESET_LABEL) as Preset[]).map(p => (
            <button
              key={p}
              onClick={() => applyPreset(p)}
              className={cn(
                'px-2.5 py-1.5 text-xs font-semibold rounded-md transition border',
                activePreset === p
                  ? 'bg-slate-900 text-white border-slate-900'
                  : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-50',
              )}
            >
              {PRESET_LABEL[p]}
            </button>
          ))}
        </div>
      </div>

      {/* ── Linha 2: filtros + datas + busca ────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-3">
        {FILTER_DIMS.map(d => (
          <MultiSelect
            key={d.col}
            label={d.label}
            options={optionsFor(d.from)}
            selected={filters[d.col] ?? []}
            onChange={(vs) => setFilter(d.col, vs)}
          />
        ))}

        <DateRangeFilter
          label="Anterior"
          inicio={compInicio}
          fim={compFim}
          onChangeInicio={v => { setCompInicio(v); setActivePreset(null) }}
          onChangeFim={v => { setCompFim(v); setActivePreset(null) }}
          accent="bg-orange-400"
        />
        <DateRangeFilter
          label="Atual"
          inicio={refInicio}
          fim={refFim}
          onChangeInicio={v => { setRefInicio(v); setActivePreset(null) }}
          onChangeFim={v => { setRefFim(v); setActivePreset(null) }}
          accent="bg-orange-600"
        />

        {totalFiltersActive > 0 && (
          <button
            onClick={clearFilters}
            className="inline-flex items-center gap-1 px-3 py-2 text-sm font-medium text-rose-600 hover:bg-rose-50 rounded-lg transition-colors"
          >
            <X className="h-3.5 w-3.5" /> Limpar ({totalFiltersActive})
          </button>
        )}

        <div className="relative ml-auto">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400" />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder={`Buscar ${visibleCards[0]?.level_label?.toLowerCase() ?? ''}…`}
            className="pl-9 pr-8 py-2 text-sm border border-slate-200 rounded-lg bg-white shadow-sm w-64 focus:outline-none focus:ring-2 focus:ring-blue-500/40 focus:border-blue-500"
          />
          {search && (
            <button
              onClick={() => setSearch('')}
              className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-slate-400 hover:text-slate-600"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* ── Chips de filtros ativos ─────────────────────────────────────────── */}
      {totalFiltersActive > 0 && (
        <div className="flex flex-wrap items-center gap-1.5 mb-4">
          <span className="text-xs text-slate-500 font-medium flex items-center gap-1 mr-1">
            <Filter className="h-3 w-3" /> Filtros ativos:
          </span>
          {FILTER_DIMS.flatMap(d => {
            const vals = filters[d.col] ?? []
            const opts = optionsFor(d.from)
            return vals.map(v => {
              const label = opts.find(o => o.key === v)?.label ?? v
              return (
                <span key={`${d.col}:${v}`} className="inline-flex items-center gap-1.5 pl-2.5 pr-1 py-1 text-xs bg-blue-50 ring-1 ring-blue-200 rounded-full">
                  <span className="text-blue-600 font-medium">{d.label}:</span>
                  <span className="font-semibold text-slate-800">{label}</span>
                  <button
                    onClick={() => setFilter(d.col, vals.filter(x => x !== v))}
                    className="ml-0.5 p-0.5 rounded-full hover:bg-blue-100 text-blue-400 hover:text-blue-700 transition-colors"
                  >
                    <X className="h-2.5 w-2.5" />
                  </button>
                </span>
              )
            })
          })}
        </div>
      )}

      {/* ── Breadcrumb de drill ─────────────────────────────────────────────── */}
      {drillPath.length > 0 && (
        <div className="flex items-center gap-1.5 mb-4 text-sm text-slate-600">
          <button onClick={() => setDrillPath([])} className="font-medium hover:text-slate-900 hover:underline">
            Início
          </button>
          {drillPath.map((d, i) => (
            <span key={i} className="flex items-center gap-1.5">
              <span className="text-slate-300">/</span>
              <button onClick={() => setDrillPath(drillPath.slice(0, i + 1))} className="hover:text-slate-900 hover:underline truncate max-w-[220px]" title={d.label}>
                {d.label}
              </button>
            </span>
          ))}
          <button onClick={handleBack} className="ml-2 inline-flex items-center gap-1 px-2 py-1 text-xs text-slate-500 hover:text-slate-900 hover:bg-slate-100 rounded-md transition-colors">
            <ChevronLeft className="h-3 w-3" />
            Voltar
          </button>
        </div>
      )}

      {/* ── KPI Hero ────────────────────────────────────────────────────────── */}
      <KpiHero kpi={kpi} refLabel={refLabel} compLabel={compLabel} isLoading={isLoading && !kpi} />

      {/* ── Banner do próximo nível ──────────────────────────────────────────── */}
      {data && data.next_level_label && (
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <span className="text-xs font-bold uppercase tracking-wider text-slate-500">
              {data.next_level_label}
            </span>
            <span className="text-xs text-slate-400">
              {visibleCards.length} {visibleCards.length === 1 ? 'item' : 'itens'}
            </span>
          </div>
        </div>
      )}

      {/* ── Cabeçalho do grid da lista ───────────────────────────────────────── */}
      {visibleCards.length > 0 && <ListaHeader />}

      {/* ── Lista de cards ───────────────────────────────────────────────────── */}
      <div className="space-y-2">
        {isLoading && cards.length === 0 && (
          <div className="text-center text-sm text-slate-500 py-12 bg-white rounded-xl ring-1 ring-slate-200">
            Carregando…
          </div>
        )}
        {error != null && (
          <div className="text-center text-sm text-rose-600 py-12 bg-rose-50 rounded-xl ring-1 ring-rose-200">
            Erro ao carregar. {(error as Error).message}
          </div>
        )}
        {!isLoading && error == null && visibleCards.length === 0 && (
          <div className="text-center text-sm text-slate-500 py-12 bg-white rounded-xl ring-1 ring-slate-200">
            {search ? 'Nenhum resultado para a busca.' : 'Sem dados para o filtro atual.'}
          </div>
        )}
        {visibleCards.map((c, i) => (
          <CardRow
            key={c.key}
            card={c}
            index={i}
            onClick={c.level === 'cod_prod' ? undefined : () => handleDrill(c)}
          />
        ))}
      </div>
    </div>
  )
}
