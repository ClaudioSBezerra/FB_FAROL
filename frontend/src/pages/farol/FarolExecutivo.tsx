import { useState, useEffect, useMemo, useRef } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  ChevronLeft, UploadCloud, RefreshCw, Search, X, Calendar, Filter, ChevronDown,
} from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { cn } from '@/lib/utils'

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

// ─── Tons ────────────────────────────────────────────────────────────────────

const HEADER_BG = 'bg-slate-600'
const HEADER_TXT_FAINT = 'text-slate-300'
const BTN_PRIMARY_BG = 'bg-slate-700'

// Cores vibrantes para o cabeçalho da LISTA (e do TOTAL)
const COL_VENDA_BG       = 'bg-blue-700'
const COL_POSITIVACAO_BG = 'bg-emerald-600'
const COL_MIX_BG         = 'bg-amber-600'
const COL_NOME_BG        = 'bg-slate-700'

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

const COR_TXT: Record<Cor, string> = {
  verde:    'text-emerald-600',
  amarelo:  'text-amber-500',
  vermelho: 'text-red-600',
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
function addYears(s: string, n: number): string {
  const [y, m, d] = s.split('-').map(Number)
  return ymd(y + n, m, d)
}
function todayYMD(): string { return new Date().toISOString().slice(0, 10) }
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

type Preset = 'yoy' | 'mom' | 'ytd' | 'last30' | 'custom'

function presetRange(p: Preset, last?: { ano: number; mes: number }) {
  const now = new Date()
  const refY = last?.ano ?? now.getUTCFullYear()
  const refM = last?.mes ?? (now.getUTCMonth() + 1)
  const refIni = ymd(refY, refM, 1)
  const refFim = ymd(refY, refM, lastDayOfMonth(refY, refM))

  switch (p) {
    case 'mom': {
      let pm = refM - 1, py = refY
      if (pm === 0) { pm = 12; py-- }
      return {
        ref_inicio: refIni, ref_fim: refFim,
        comp_inicio: ymd(py, pm, 1),
        comp_fim: ymd(py, pm, lastDayOfMonth(py, pm)),
      }
    }
    case 'ytd':
      return {
        ref_inicio: ymd(refY, 1, 1), ref_fim: refFim,
        comp_inicio: ymd(refY - 1, 1, 1),
        comp_fim: ymd(refY - 1, refM, lastDayOfMonth(refY - 1, refM)),
      }
    case 'last30': {
      const fim = todayYMD()
      const ini = addDays(fim, -29)
      return { ref_inicio: ini, ref_fim: fim, comp_inicio: addYears(ini, -1), comp_fim: addYears(fim, -1) }
    }
    case 'custom':
    case 'yoy':
    default:
      return { ref_inicio: refIni, ref_fim: refFim, comp_inicio: addYears(refIni, -1), comp_fim: addYears(refFim, -1) }
  }
}

// ─── Cabeçalho colorido + subtítulos (reutilizado em Total e Lista) ─────────

const GRID_COLS = 'grid-cols-[minmax(180px,2fr)_3fr_3fr_1.2fr]'

function ColumnsHeader() {
  return (
    <>
      {/* Linha colorida: nomes dos grupos */}
      <div className={cn('grid', GRID_COLS)}>
        <div className={cn(COL_NOME_BG, 'text-white px-3 py-2 text-xs uppercase tracking-wider font-bold')}>
          Nome
        </div>
        <div className={cn(COL_VENDA_BG, 'text-white px-3 py-2 text-xs uppercase tracking-wider font-bold text-center')}>
          Venda
        </div>
        <div className={cn(COL_POSITIVACAO_BG, 'text-white px-3 py-2 text-xs uppercase tracking-wider font-bold text-center')}>
          Positivação
        </div>
        <div className={cn(COL_MIX_BG, 'text-white px-3 py-2 text-xs uppercase tracking-wider font-bold text-center')}>
          Mix Médio
        </div>
      </div>
      {/* Linha clara: subtítulos */}
      <div className={cn('grid', GRID_COLS, 'bg-slate-50 border-y border-slate-200')}>
        <div className="px-3 py-1.5 text-[10px] uppercase tracking-wide text-slate-400 font-medium">
          {/* vazio */}
        </div>
        <div className="grid grid-cols-3 gap-1 px-2 py-1.5 text-[10px] uppercase tracking-wide text-slate-500 font-semibold text-center">
          <div>Período Anterior</div>
          <div>Período Atual</div>
          <div>%</div>
        </div>
        <div className="grid grid-cols-3 gap-1 px-2 py-1.5 text-[10px] uppercase tracking-wide text-slate-500 font-semibold text-center">
          <div>Clientes Ativos</div>
          <div>Clientes Positivados</div>
          <div>% Posit.</div>
        </div>
        <div className="px-2 py-1.5 text-[10px] uppercase tracking-wide text-slate-500 font-semibold text-center">
          Realizado
        </div>
      </div>
    </>
  )
}

// ─── Linha de dados (Total OU fornecedor) ────────────────────────────────────

interface RowProps {
  card: CardItem
  isTotal?: boolean
  onClick?: () => void
}

function DataRow({ card, isTotal = false, onClick }: RowProps) {
  const clickable = !!onClick
  const valueNum = isTotal
    ? 'text-base font-bold tabular-nums text-slate-800'
    : 'text-sm font-bold tabular-nums text-slate-800'
  const valueLabelCls = isTotal
    ? 'text-base font-bold'
    : 'text-sm font-semibold'

  return (
    <div
      role={clickable ? 'button' : undefined}
      onClick={onClick}
      className={cn(
        'grid', GRID_COLS,
        'border-b border-slate-100 last:border-b-0 bg-white',
        clickable && 'cursor-pointer hover:bg-slate-50 transition-colors',
        isTotal && 'border-b-2 border-slate-200',
      )}
    >
      {/* Nome */}
      <div className={cn('px-3 py-2.5 flex items-center', isTotal && 'bg-slate-50')}>
        <span className={cn('text-slate-800 truncate', valueLabelCls)} title={card.label}>
          {card.label}
        </span>
        {!isTotal && (
          <span className="ml-2 text-[10px] uppercase tracking-wider text-slate-400 shrink-0">
            {card.level_label}
          </span>
        )}
      </div>

      {/* VENDA */}
      <div className="grid grid-cols-3 gap-1 px-2 py-2.5 items-center">
        <div className={cn(valueNum, 'text-center')}>{fmtBRL(card.valor_ant)}</div>
        <div className={cn(valueNum, 'text-center')}>{fmtBRL(card.valor_atual)}</div>
        <div className={cn('text-center font-bold tabular-nums', isTotal ? 'text-lg' : 'text-sm', COR_TXT[card.cor])}>
          {fmtPct(card.pct)}
        </div>
      </div>

      {/* POSITIVAÇÃO */}
      <div className="grid grid-cols-3 gap-1 px-2 py-2.5 items-center">
        <div className={cn(valueNum, 'text-center')}>{fmtInt(card.base_cli)}</div>
        <div className={cn(valueNum, 'text-center')}>{fmtInt(card.positivados)}</div>
        <div className={cn('text-center font-bold tabular-nums', isTotal ? 'text-lg' : 'text-sm', COR_TXT[card.posit_cor])}>
          {fmtPct(card.positpct)}
        </div>
      </div>

      {/* MIX MÉDIO */}
      <div className="px-2 py-2.5 flex items-center justify-center">
        <span className={cn('font-bold tabular-nums', isTotal ? 'text-lg' : 'text-sm', COR_TXT[card.mix_cor])}>
          {fmtMix(card.mix)}
        </span>
      </div>
    </div>
  )
}

// ─── MultiSelect ─────────────────────────────────────────────────────────────

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
          'inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium border rounded-md bg-white shadow-sm',
          selected.length > 0 ? 'border-slate-600 text-slate-900' : 'border-slate-300 text-slate-600 hover:bg-slate-50',
        )}
      >
        {label}
        {selected.length > 0 && (
          <span className={cn('inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full text-white text-[10px] font-bold', BTN_PRIMARY_BG)}>
            {selected.length}
          </span>
        )}
        <ChevronDown className="h-3 w-3 opacity-60" />
      </button>

      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 w-72 bg-white border border-slate-200 rounded-md shadow-lg overflow-hidden">
          <div className="p-2 border-b border-slate-100">
            <div className="relative">
              <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-slate-400" />
              <input
                autoFocus
                value={q}
                onChange={e => setQ(e.target.value)}
                placeholder={`Buscar ${label.toLowerCase()}...`}
                className="w-full pl-7 pr-2 py-1.5 text-xs border border-slate-200 rounded"
              />
            </div>
          </div>
          <div className="max-h-64 overflow-y-auto">
            {filtered.length === 0 && (
              <div className="px-3 py-4 text-center text-xs text-slate-400">Nenhum resultado</div>
            )}
            {filtered.map(opt => {
              const checked = selected.includes(opt.key)
              return (
                <label key={opt.key} className="flex items-center gap-2 px-3 py-1.5 hover:bg-slate-50 cursor-pointer text-xs">
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() => toggle(opt.key)}
                    className="w-3.5 h-3.5 accent-slate-700"
                  />
                  <span className={cn('truncate', checked && 'font-medium text-slate-900')}>{opt.label}</span>
                </label>
              )
            })}
          </div>
          {selected.length > 0 && (
            <div className="border-t border-slate-100 p-2 flex items-center justify-between">
              <span className="text-[10px] text-slate-500">{selected.length} selecionado(s)</span>
              <button onClick={() => onChange([])} className="text-[10px] text-slate-500 hover:text-red-600 font-medium">
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

// ─── FarolExecutivo ──────────────────────────────────────────────────────────

export default function FarolExecutivo() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { user, spRole, tipoPersona } = useAuth()
  const canImport = user?.role === 'admin' || spRole === 'admin_fbtax' || tipoPersona === 'ti'

  const [view, setView] = useState<'V01' | 'V02' | 'V03'>('V01')
  const [fluxo, setFluxo] = useState<Fluxo>('faturado')
  const [drillPath, setDrillPath] = useState<DrillStep[]>([])
  const [refreshing, setRefreshing] = useState(false)
  const [search, setSearch] = useState('')
  const [activePreset, setActivePreset] = useState<Preset>('yoy')

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
    if (refInicio || !periodosQ.data?.ref_ano) return
    const r = presetRange('yoy', { ano: periodosQ.data.ref_ano!, mes: periodosQ.data.ref_mes! })
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }, [periodosQ.data, refInicio])

  const { data, isLoading, error } = useCards({
    view, fluxo, ref_inicio: refInicio, ref_fim: refFim,
    comp_inicio: compInicio, comp_fim: compFim,
    drillPath, filters,
  })
  const dimsQ = useDims(fluxo, refInicio, refFim)

  const applyPreset = (p: Preset) => {
    setActivePreset(p)
    if (p === 'custom') return
    const last = periodosQ.data ? { ano: periodosQ.data.ref_ano!, mes: periodosQ.data.ref_mes! } : undefined
    const r = presetRange(p, last)
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }

  const handleDrill = (card: CardItem) => {
    if (card.level === 'cod_prod') return
    setDrillPath(prev => [...prev, { level: card.level, value: card.key, label: card.label }])
  }
  const handleBack = () => setDrillPath(prev => prev.slice(0, -1))
  const handleViewChange = (v: 'V01' | 'V02' | 'V03') => { setView(v); setDrillPath([]) }

  const cards = data?.cards ?? []
  const kpi = data?.kpi

  // Constrói o "card total" virtual a partir do KPI pra reaproveitar o componente DataRow
  const totalCard: CardItem | null = kpi ? {
    key: '__total__', label: 'TOTAL',
    level: '', level_label: '',
    valor_atual: kpi.total_atual,
    valor_ant: kpi.total_ant,
    pct: kpi.total_pct,
    cor: kpi.total_cor,
    plucro: kpi.total_plucro, plucro_ant: kpi.total_plucro_ant,
    positivados: kpi.total_positivados, base_cli: kpi.total_base_cli,
    positpct: kpi.total_positpct,
    positivados_ant: kpi.total_positivados_ant, base_cli_ant: kpi.total_base_cli_ant,
    positpct_ant: kpi.total_positpct_ant,
    posit_cor: kpi.total_posit_cor,
    mix: kpi.avg_mix, mix_ant: kpi.avg_mix_ant, mix_cor: kpi.mix_cor,
  } : null

  const visibleCards = useMemo(() => {
    const s = search.trim().toLowerCase()
    if (!s) return cards
    return cards.filter(c => c.label.toLowerCase().includes(s))
  }, [cards, search])

  const handleRefreshViews = async () => {
    setRefreshing(true)
    try {
      await fetch('/api/v2/farol/refresh-views', { method: 'POST' })
      await queryClient.invalidateQueries({ queryKey: ['farol-v2-cards'] })
    } finally {
      setRefreshing(false)
    }
  }

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

  return (
    <div className="min-h-full p-4 md:p-6 bg-slate-50">
      {/* ── Seletor de FLUXO (acima de tudo) ────────────────────────────────── */}
      <div className="flex items-center gap-3 mb-3">
        <span className="text-xs uppercase tracking-wider font-bold text-slate-500">Fluxo:</span>
        <div className="flex rounded-md border-2 border-slate-300 overflow-hidden bg-white shadow-sm">
          {([
            { id: 'faturado'    as const, label: 'Faturado',    color: 'bg-blue-700' },
            { id: 'transmitido' as const, label: 'Transmitido', color: 'bg-emerald-700' },
          ]).map(f => (
            <button
              key={f.id}
              onClick={() => { setFluxo(f.id); setDrillPath([]) }}
              className={cn(
                'px-5 py-2 text-sm font-bold uppercase tracking-wide transition-colors',
                fluxo === f.id ? cn(f.color, 'text-white') : 'text-slate-600 hover:bg-slate-50',
              )}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      {/* ── Faixa de PERÍODO ─────────────────────────────────────────────────── */}
      <div className="bg-white border border-slate-200 rounded-lg shadow-sm mb-4">
        <div className={cn('px-4 py-2 text-white flex items-center justify-between flex-wrap gap-2', HEADER_BG)}>
          <div className="flex items-center gap-2">
            <Calendar className="h-4 w-4 text-red-400" />
            <span className="text-xs uppercase tracking-wider font-bold text-red-400">Período</span>
          </div>
          <div className="flex gap-1 flex-wrap">
            {([
              { id: 'yoy'    as const, label: 'Ano a Ano',       active: 'bg-blue-800 text-white',     inactive: 'bg-slate-700 text-blue-200 hover:bg-blue-900' },
              { id: 'mom'    as const, label: 'Mês a Mês',       active: 'bg-sky-500 text-white',      inactive: 'bg-slate-700 text-sky-200 hover:bg-sky-700' },
              { id: 'ytd'    as const, label: 'Projeção Anual',  active: 'bg-emerald-600 text-white',  inactive: 'bg-slate-700 text-emerald-200 hover:bg-emerald-700' },
              { id: 'last30' as const, label: 'Últimos 30 dias', active: 'bg-amber-500 text-white',    inactive: 'bg-slate-700 text-amber-200 hover:bg-amber-600' },
              { id: 'custom' as const, label: 'Personalizado',   active: 'bg-violet-600 text-white',   inactive: 'bg-slate-700 text-violet-200 hover:bg-violet-700' },
            ]).map(p => (
              <button
                key={p.id}
                onClick={() => applyPreset(p.id)}
                className={cn(
                  'px-2 py-1 text-[11px] font-semibold rounded transition',
                  activePreset === p.id ? p.active : p.inactive,
                )}
              >
                {p.label}
              </button>
            ))}
          </div>
        </div>
        <div className="px-4 py-3 grid grid-cols-1 md:grid-cols-[1fr_auto_1fr] gap-3 items-end">
          <div>
            <label className="text-[11px] uppercase tracking-wider text-orange-600 font-bold block mb-1">
              Base Anterior
            </label>
            <div className="flex gap-1 items-center">
              <input
                type="date"
                value={compInicio}
                onChange={e => { setCompInicio(e.target.value); setActivePreset('custom') }}
                className="flex-1 px-2 py-1.5 text-xs border-2 border-orange-400 rounded bg-white"
              />
              <span className="text-orange-500 text-xs px-1 font-bold">→</span>
              <input
                type="date"
                value={compFim}
                onChange={e => { setCompFim(e.target.value); setActivePreset('custom') }}
                className="flex-1 px-2 py-1.5 text-xs border-2 border-orange-400 rounded bg-white"
              />
            </div>
            {compInicio && compFim && (
              <div className="text-[10px] text-slate-500 mt-1">
                {rangeDaysInclusive(compInicio, compFim)} dia(s) — {fmtDateBR(compInicio)} a {fmtDateBR(compFim)}
              </div>
            )}
          </div>

          <div className="text-slate-400 text-xs font-bold uppercase tracking-wider pb-2 text-center hidden md:block">vs</div>

          <div>
            <label className="text-[11px] uppercase tracking-wider text-orange-600 font-bold block mb-1">
              Base Atual
            </label>
            <div className="flex gap-1 items-center">
              <input
                type="date"
                value={refInicio}
                onChange={e => { setRefInicio(e.target.value); setActivePreset('custom') }}
                className="flex-1 px-2 py-1.5 text-xs border-2 border-orange-500 rounded bg-white font-medium"
              />
              <span className="text-orange-500 text-xs px-1 font-bold">→</span>
              <input
                type="date"
                value={refFim}
                onChange={e => { setRefFim(e.target.value); setActivePreset('custom') }}
                className="flex-1 px-2 py-1.5 text-xs border-2 border-orange-500 rounded bg-white font-medium"
              />
            </div>
            {refInicio && refFim && (
              <div className="text-[10px] text-slate-500 mt-1">
                {rangeDaysInclusive(refInicio, refFim)} dia(s) — {fmtDateBR(refInicio)} a {fmtDateBR(refFim)}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* ── Controles secundários ───────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-3">
        <div className="flex rounded-md border border-slate-300 overflow-hidden bg-white shadow-sm">
          {([
            { id: 'V01' as const, label: 'Por Indústria' },
            { id: 'V03' as const, label: 'Por Gerência' },
            { id: 'V02' as const, label: 'Por Equipe' },
          ]).map(v => (
            <button
              key={v.id}
              onClick={() => handleViewChange(v.id)}
              className={cn(
                'px-3 py-1.5 text-xs font-medium transition-colors',
                view === v.id ? cn(BTN_PRIMARY_BG, 'text-white') : 'text-slate-600 hover:bg-slate-50',
              )}
            >
              {v.label}
            </button>
          ))}
        </div>

        {FILTER_DIMS.map(d => (
          <MultiSelect
            key={d.col}
            label={d.label}
            options={optionsFor(d.from)}
            selected={filters[d.col] ?? []}
            onChange={(vs) => setFilter(d.col, vs)}
          />
        ))}

        {totalFiltersActive > 0 && (
          <button
            onClick={clearFilters}
            className="inline-flex items-center gap-1 px-2 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 rounded-md"
          >
            <X className="h-3 w-3" /> Limpar filtros ({totalFiltersActive})
          </button>
        )}

        <div className="flex-1" />

        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400" />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder={`Buscar ${visibleCards[0]?.level_label?.toLowerCase() ?? ''}...`}
            className="pl-7 pr-7 py-1.5 text-xs border border-slate-300 rounded-md bg-white shadow-sm w-48"
          />
          {search && (
            <button
              onClick={() => setSearch('')}
              className="absolute right-1 top-1/2 -translate-y-1/2 p-0.5 text-slate-400 hover:text-slate-600"
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </div>

        {canImport && (
          <>
            <button
              onClick={handleRefreshViews}
              disabled={refreshing}
              className="inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-slate-600 border border-slate-300 rounded-md bg-white shadow-sm hover:bg-slate-50 disabled:opacity-50"
            >
              <RefreshCw className={cn('h-3 w-3', refreshing && 'animate-spin')} />
              Consolidar
            </button>
            <button
              onClick={() => navigate('/farol/importar')}
              className={cn('inline-flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-white rounded-md shadow-sm hover:opacity-90', BTN_PRIMARY_BG)}
            >
              <UploadCloud className="h-3 w-3" />
              Importar
            </button>
          </>
        )}
      </div>

      {/* ── Chips dos filtros ativos ────────────────────────────────────────── */}
      {totalFiltersActive > 0 && (
        <div className="flex flex-wrap items-center gap-1 mb-3">
          <span className="text-[10px] uppercase tracking-wider text-slate-500 font-semibold mr-1">
            <Filter className="h-3 w-3 inline -mt-0.5" /> Filtros ativos:
          </span>
          {FILTER_DIMS.flatMap(d => {
            const vals = filters[d.col] ?? []
            const opts = optionsFor(d.from)
            return vals.map(v => {
              const label = opts.find(o => o.key === v)?.label ?? v
              return (
                <span key={`${d.col}:${v}`} className="inline-flex items-center gap-1 px-2 py-0.5 text-[11px] bg-slate-100 border border-slate-200 rounded-full">
                  <span className="text-slate-500">{d.label}:</span>
                  <span className="font-medium text-slate-800">{label}</span>
                  <button onClick={() => setFilter(d.col, vals.filter(x => x !== v))} className="ml-0.5 text-slate-400 hover:text-red-600">
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
        <div className="flex items-center gap-1 mb-3 text-xs text-slate-600">
          <button onClick={() => setDrillPath([])} className="hover:text-slate-900 hover:underline">
            Início
          </button>
          {drillPath.map((d, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="text-slate-400">›</span>
              <button onClick={() => setDrillPath(drillPath.slice(0, i + 1))} className="hover:text-slate-900 hover:underline truncate max-w-[200px]" title={d.label}>
                {d.label}
              </button>
            </span>
          ))}
          <button onClick={handleBack} className="ml-2 inline-flex items-center gap-1 text-slate-500 hover:text-slate-900">
            <ChevronLeft className="h-3 w-3" />
            Voltar
          </button>
        </div>
      )}

      {/* ── TOTAL (card único com cabeçalho próprio) ────────────────────────── */}
      {totalCard && (
        <div className="bg-white border border-slate-200 rounded-lg overflow-hidden mb-4 shadow-sm">
          <ColumnsHeader />
          <DataRow card={totalCard} isTotal />
        </div>
      )}

      {/* ── LISTA de fornecedores/GGV/equipe ─────────────────────────────────── */}
      <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-sm">
        {/* Cabeçalho colorido da lista (mesmo padrão do total) */}
        <ColumnsHeader />

        {/* Linhas */}
        {isLoading && (
          <div className="text-center text-sm text-slate-500 py-8">Carregando…</div>
        )}
        {error != null && (
          <div className="text-center text-sm text-red-600 py-8">
            Erro ao carregar. {(error as Error).message}
          </div>
        )}
        {!isLoading && error == null && visibleCards.length === 0 && (
          <div className="text-center text-sm text-slate-500 py-8">
            {search ? 'Nenhum resultado para a busca.' : 'Sem dados para o filtro atual.'}
          </div>
        )}
        {visibleCards.map(c => (
          <DataRow
            key={c.key}
            card={c}
            onClick={c.level === 'cod_prod' ? undefined : () => handleDrill(c)}
          />
        ))}
      </div>
    </div>
  )
}
