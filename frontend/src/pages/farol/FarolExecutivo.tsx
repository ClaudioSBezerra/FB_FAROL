import { useState, useEffect, useMemo, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ChevronLeft, Search, X, Calendar, Filter, ChevronDown,
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

// ─── Tons ────────────────────────────────────────────────────────────────────

const HEADER_BG = 'bg-slate-600'
const HEADER_TXT_FAINT = 'text-slate-300'
const BTN_PRIMARY_BG = 'bg-slate-700'

// Cabeçalho da LISTA / TOTAL — tarja TURQUESA forte (cyan-800)
// Cores vivas (300/400) sobre cyan-800 — contraste alto, fácil ler à distância:
//   NOME:        branco          (peso/identidade)
//   VENDA:       yellow-300      (amarelo vivo — valor monetário)
//   POSITIVAÇÃO: lime-300        (verde brilhante — sucesso/clientes)
//   MIX:         fuchsia-400     (rosa vivo — variedade; NÃO usa ciano, evita conflito)
// Azul oficial do Farol (mesmo da página de Login — logo Target #1e293b)
const TARJA_BG = 'bg-[#1e293b]'
const COL_NOME_TXT        = 'text-white'
const COL_VENDA_TXT       = 'text-yellow-300'
const COL_POSITIVACAO_TXT = 'text-lime-300'
const COL_MIX_TXT         = 'text-fuchsia-400'

// ─── Utilitários ──────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v === 0) return '—'
  // Valores ABSOLUTOS (sem abreviar K/M/B) — decisão do gestor. Ex.: R$ 2.500,35
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 })
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

// Cores FORTES para a linha Total (fundo cinza): verde/vermelho mais saturados
// e escuros + extrabold p/ máximo contraste sobre o gradiente slate.
const COR_TXT_TOTAL: Record<Cor, string> = {
  verde:    'text-emerald-800',
  amarelo:  'text-amber-700',
  vermelho: 'text-red-700',
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

// Presets (ordem da barra, da esquerda para a direita):
//  ytd          — ano anterior completo (jan-dez) × jan até hoje, ano corrente
//  yoy          — último mês 100% importado × mesmo mês ano anterior
//  ant_corrente — dois últimos meses completos carregados (M-1 vs M-2)
//  mes_corrente — dia 1 até hoje, mês corrente × mesmo período do ano anterior
//  last7        — últimos 7 dias × 7 dias anteriores
//  last30       — últimos 30 dias × 30 dias anteriores
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
  const todayM = now.getUTCMonth() + 1  // 1..12
  const todayD = now.getUTCDate()
  const today = ymd(todayY, todayM, todayD)

  // Último mês 100% importado — fallback para o mês anterior ao corrente
  const lastY = last?.ano ?? todayY
  const lastM = last?.mes ?? (todayM > 1 ? todayM - 1 : 12)

  switch (p) {
    case 'ytd': {
      // Ano anterior INTEIRO × Janeiro até hoje do ano corrente
      return {
        ref_inicio:  ymd(todayY, 1, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, 1, 1),
        comp_fim:    ymd(todayY - 1, 12, 31),
      }
    }
    case 'yoy': {
      // Último mês 100% importado × mesmo mês do ano anterior — ambos completos
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(lastY - 1, lastM, 1),
        comp_fim:    ymd(lastY - 1, lastM, lastDayOfMonth(lastY - 1, lastM)),
      }
    }
    case 'ant_corrente': {
      // M-1 vs M-2: dois últimos meses completos carregados
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
      // Dia 1 até hoje do mês corrente × MESMO período do ANO ANTERIOR
      // (decisão do gestor — só WEB; mobile mantém em farolPresets.ts).
      const dayCap = Math.min(todayD, lastDayOfMonth(todayY - 1, todayM))
      return {
        ref_inicio:  ymd(todayY, todayM, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, todayM, 1),
        comp_fim:    ymd(todayY - 1, todayM, dayCap),
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

// ─── Cabeçalho colorido + subtítulos (reutilizado em Total e Lista) ─────────

const GRID_COLS        = 'grid-cols-[minmax(180px,2fr)_3fr_5fr_1fr]'
const GRID_COLS_NOPOS  = 'grid-cols-[minmax(180px,2fr)_3fr_1.4fr]'
// Positivação não faz sentido no nível Cliente/Produto → coluna some.
const gridCols = (hidePosit?: boolean) => (hidePosit ? GRID_COLS_NOPOS : GRID_COLS)

function ColumnsHeader({ hidePosit }: { hidePosit?: boolean }) {
  const GC = gridCols(hidePosit)
  return (
    <>
      {/* Tarja preta única; cores apenas nos textos */}
      <div className={cn('grid', GC, TARJA_BG)}>
        <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold', COL_NOME_TXT)}>
          Nome
        </div>
        <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold text-center', COL_VENDA_TXT)}>
          Venda
        </div>
        {!hidePosit && (
          <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold text-center', COL_POSITIVACAO_TXT)}>
            Positivação
          </div>
        )}
        <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold text-center', COL_MIX_TXT)}>
          Mix Médio
        </div>
      </div>
      {/* Linha clara: subtítulos */}
      <div className={cn('grid', GC, 'bg-slate-50 border-y border-slate-200')}>
        <div className="px-3 py-1.5 text-sm uppercase tracking-wide text-slate-400 font-medium">
          {/* vazio */}
        </div>
        <div className="grid grid-cols-3 gap-1 px-2 py-1.5 text-sm uppercase tracking-wide text-slate-500 font-semibold text-center">
          <div>Período Anterior</div>
          <div>Período Atual</div>
          <div>%</div>
        </div>
        {!hidePosit && (
          <div className="grid grid-cols-5 gap-1 px-2 py-1.5 text-sm uppercase tracking-wide text-slate-500 font-semibold text-center">
            <div>Clientes Ativos</div>
            <div>Posit. Anterior</div>
            <div>% Ant.</div>
            <div>Posit. Atual</div>
            <div>% Atual</div>
          </div>
        )}
        <div className="px-2 py-1.5 text-sm uppercase tracking-wide text-slate-500 font-semibold text-center">
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
  hidePosit?: boolean
}

function DataRow({ card, isTotal = false, onClick, hidePosit }: RowProps) {
  const clickable = !!onClick
  const valueNum = isTotal
    ? 'text-sm font-bold tabular-nums text-slate-900'
    : 'text-sm font-bold tabular-nums text-slate-800'
  const valueLabelCls = isTotal
    ? 'text-sm font-extrabold'
    : 'text-sm font-semibold'

  return (
    <div
      role={clickable ? 'button' : undefined}
      onClick={onClick}
      className={cn(
        'grid', gridCols(hidePosit),
        'border-b border-slate-100 last:border-b-0',
        isTotal
          ? 'bg-gradient-to-r from-slate-400 via-slate-300 to-slate-500 border-b-2 border-slate-500'
          : 'bg-white',
        clickable && 'cursor-pointer hover:bg-slate-50 transition-colors',
      )}
    >
      {/* Nome — "código - descrição" (exceto no Total). Vale p/ Indústria, GGV,
          Supervisor, RCA, Cliente e Produto. */}
      <div className="px-3 py-2.5 flex items-center">
        <span className={cn('truncate', valueLabelCls, isTotal ? 'text-slate-900 uppercase tracking-wider' : 'text-slate-800')} title={isTotal ? card.label : `${card.key} - ${card.label}`}>
          {isTotal ? card.label : `${card.key} - ${card.label}`}
        </span>
      </div>

      {/* VENDA */}
      <div className="grid grid-cols-3 gap-1 px-2 py-2.5 items-center">
        <div className={cn(valueNum, 'text-center')}>{fmtBRL(card.valor_ant)}</div>
        <div className={cn(valueNum, 'text-center')}>{fmtBRL(card.valor_atual)}</div>
        <div className={cn('text-center tabular-nums', isTotal ? 'text-base font-extrabold' : 'text-sm font-bold', isTotal ? COR_TXT_TOTAL[card.cor] : COR_TXT[card.cor])}>
          {fmtPct(card.pct)}
        </div>
      </div>

      {/* POSITIVAÇÃO — Clientes Ativos (carteira) + positivados Anterior × Atual + % penetração.
          Escondida no nível Cliente/Produto (não faz sentido). */}
      {!hidePosit && (
        <div className="grid grid-cols-5 gap-1 px-2 py-2.5 items-center">
          <div className={cn(valueNum, 'text-center')}>{fmtInt(card.base_cli)}</div>
          {/* cada % ao lado da sua métrica: Anterior (cinza) e Atual (colorido) */}
          <div className={cn(valueNum, 'text-center')}>{fmtInt(card.positivados_ant)}</div>
          <div className={cn('text-center tabular-nums text-sm font-semibold', isTotal ? 'text-slate-700' : 'text-slate-500')}>
            {fmtPct(card.positpct_ant)}
          </div>
          <div className={cn(valueNum, 'text-center')}>{fmtInt(card.positivados)}</div>
          <div className={cn('text-center tabular-nums', isTotal ? 'text-base font-extrabold' : 'text-sm font-bold', isTotal ? COR_TXT_TOTAL[card.posit_cor] : COR_TXT[card.posit_cor])}>
            {fmtPct(card.positpct)}
          </div>
        </div>
      )}

      {/* MIX MÉDIO — só a média de SKUs/cliente. O "de Y" (universo) foi omitido
          a pedido do gestor (gera confusão; muitas variáveis influenciam o total). */}
      {/* No Totalizador (isTotal) o fundo é cinza → mix em preto (verde sumia). */}
      <div className="px-2 py-2.5 flex items-center justify-center gap-1 flex-wrap">
        <span className={cn('font-bold tabular-nums', isTotal ? 'text-sm font-bold text-slate-900' : cn('text-sm', COR_TXT[card.mix_cor]))}>
          {fmtMix(card.mix)}
        </span>
      </div>
    </div>
  )
}

// ─── DateRangeFilter ─────────────────────────────────────────────────────────

interface DateRangeFilterProps {
  label: string
  inicio: string
  fim: string
  onChangeInicio: (v: string) => void
  onChangeFim: (v: string) => void
  borderColor?: string
}

function DateRangeFilter({ label, inicio, fim, onChangeInicio, onChangeFim, borderColor = 'border-slate-300' }: DateRangeFilterProps) {
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
  const summary = hasValue ? `${fmtDateBR(inicio)} → ${fmtDateBR(fim)}` : '—'

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className={cn(
          'inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium border rounded-md bg-white shadow-sm',
          hasValue ? 'border-slate-600 text-slate-900' : 'border-slate-300 text-slate-600 hover:bg-slate-50',
        )}
      >
        <span className="text-sm text-slate-400 font-semibold uppercase tracking-wide">{label}:</span>
        <span>{summary}</span>
        <ChevronDown className="h-3 w-3 opacity-60" />
      </button>
      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 bg-white border border-slate-200 rounded-md shadow-lg p-3 min-w-[290px]">
          <div className="text-sm uppercase tracking-wider text-slate-500 font-semibold mb-2">{label}</div>
          <div className="flex gap-2 items-center">
            <input
              type="date"
              value={inicio}
              onChange={e => onChangeInicio(e.target.value)}
              className={cn('flex-1 px-2 py-1.5 text-sm border-2 rounded bg-white', borderColor)}
            />
            <span className="text-slate-400 text-sm font-bold">→</span>
            <input
              type="date"
              value={fim}
              onChange={e => onChangeFim(e.target.value)}
              className={cn('flex-1 px-2 py-1.5 text-sm border-2 rounded bg-white', borderColor)}
            />
          </div>
          {inicio && fim && (
            <div className="text-sm text-slate-500 mt-1.5">
              {rangeDaysInclusive(inicio, fim)} dia(s) — {fmtDateBR(inicio)} a {fmtDateBR(fim)}
            </div>
          )}
        </div>
      )}
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
          'inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium border rounded-md bg-white shadow-sm',
          selected.length > 0 ? 'border-slate-600 text-slate-900' : 'border-slate-300 text-slate-600 hover:bg-slate-50',
        )}
      >
        {label}
        {selected.length > 0 && (
          <span className={cn('inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full text-white text-sm font-bold', BTN_PRIMARY_BG)}>
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
                className="w-full pl-7 pr-2 py-1.5 text-sm border border-slate-200 rounded"
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
                <label key={opt.key} className="flex items-center gap-2 px-3 py-1.5 hover:bg-slate-50 cursor-pointer text-sm">
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
              <span className="text-sm text-slate-500">{selected.length} selecionado(s)</span>
              <button onClick={() => onChange([])} className="text-sm text-slate-500 hover:text-red-600 font-medium">
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
  // navigate / canImport / refreshing removidos junto com os botões
  // "Importar" e "Consolidar" — gestor pediu pra retirar do painel (usuários
  // clicavam sem querer). Ações administrativas seguem disponíveis no menu.
  const { spRole, tipoPersona } = useAuth()
  void spRole; void tipoPersona // mantidos pra futuras gates de UI

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
    // Default ao entrar: "Ano × Ano" (ano anterior completo vs ano atual até hoje)
    const r = presetRange('ytd', { ano: periodosQ.data.ref_ano!, mes: periodosQ.data.ref_mes! })
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }, [periodosQ.data, refInicio])

  const applyPreset = (p: Preset) => {
    setActivePreset(p)
    // periodos vem DESC (mais recente primeiro); ref_ano/ref_mes pode ser 0 se
    // ainda sem dados no momento do fetch — usar periodos[0] como fonte primária.
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
    mix_total: kpi.total_mix_total, mix_total_ant: kpi.total_mix_total_ant,
  } : null

  const visibleCards = useMemo(() => {
    const s = search.trim().toLowerCase()
    if (!s) return cards
    return cards.filter(c => c.label.toLowerCase().includes(s))
  }, [cards, search])

  // Nível atual sendo listado (cards) → esconde Positivação em Cliente/Produto.
  const curLevel = cards[0]?.level ?? ''
  const hidePosit = curLevel === 'cod_cli' || curLevel === 'cod_prod'

  // handleRefreshViews removido junto com o botão Consolidar.

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
    <div className="min-h-full p-4 md:p-6 bg-slate-50 uppercase text-sm [&_*]:uppercase">

      {/* ── Seletor de FLUXO (acima de tudo) ────────────────────────────────── */}
      <div className="mb-3">
        <div className="inline-flex rounded-md border-2 border-slate-300 overflow-hidden bg-white shadow-sm">
          {([
            { id: 'faturado'    as const, label: 'Faturado',    color: 'bg-[#1e293b]' },
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

      {/* ── Atalhos de período ──────────────────────────────────────────────── */}
      <div className="flex items-center gap-1.5 mb-1.5 flex-wrap">
        <span className="text-sm uppercase tracking-wider text-slate-400 font-semibold flex items-center gap-1 mr-0.5">
          <Calendar className="h-3 w-3" />
        </span>
        {([
          { id: 'ytd'          as const, tip: 'Ano anterior INTEIRO (Jan-Dez) × Jan até hoje do ano atual' },
          { id: 'yoy'          as const, tip: 'Último mês 100% importado × Mesmo mês do ano anterior (ambos completos)' },
          { id: 'ant_corrente' as const, tip: 'Dois últimos meses completos carregados (M-1 vs M-2)' },
          { id: 'mes_corrente' as const, tip: 'Dia 1 até hoje do mês corrente × mesmo período do ano anterior' },
          { id: 'last7'        as const, tip: 'Últimos 7 dias × 7 dias anteriores' },
          { id: 'last30'       as const, tip: 'Últimos 30 dias × 30 dias anteriores' },
        ]).map(p => (
          <button
            key={p.id}
            onClick={() => applyPreset(p.id)}
            title={p.tip}
            className={cn(
              'px-2.5 py-1 text-sm font-semibold rounded transition border',
              activePreset === p.id
                ? 'bg-slate-700 text-white border-slate-700 shadow-sm'
                : 'bg-white text-slate-600 border-slate-300 hover:bg-slate-50 hover:border-slate-400',
            )}
          >
            {PRESET_LABEL[p.id]}
          </button>
        ))}
      </div>

      {/* ── Strip de período ativo ───────────────────────────────────────────── */}
      {compInicio && compFim && refInicio && refFim && (
        <div className="flex items-center gap-2 mb-2 px-3 py-1.5 bg-white border border-slate-200 rounded-md text-sm shadow-sm flex-wrap">
          {activePreset && (
            <span className="font-bold text-slate-700 mr-1">{PRESET_LABEL[activePreset]}:</span>
          )}
          <span className="text-slate-400 font-medium">Anterior</span>
          <span className="font-semibold text-orange-600">
            {fmtDateBR(compInicio)} → {fmtDateBR(compFim)}
          </span>
          <span className="text-slate-300">({rangeDaysInclusive(compInicio, compFim)} dias)</span>
          <span className="text-slate-400 font-bold mx-1">vs</span>
          <span className="text-slate-400 font-medium">Atual</span>
          <span className="font-semibold text-orange-700">
            {fmtDateBR(refInicio)} → {fmtDateBR(refFim)}
          </span>
          <span className="text-slate-300">({rangeDaysInclusive(refInicio, refFim)} dias)</span>
        </div>
      )}

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
                'px-3 py-1.5 text-sm font-medium transition-colors',
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

        <DateRangeFilter
          label="Período Anterior"
          inicio={compInicio}
          fim={compFim}
          onChangeInicio={v => { setCompInicio(v); setActivePreset(null) }}
          onChangeFim={v => { setCompFim(v); setActivePreset(null) }}
          borderColor="border-orange-400"
        />
        <DateRangeFilter
          label="Período Atual"
          inicio={refInicio}
          fim={refFim}
          onChangeInicio={v => { setRefInicio(v); setActivePreset(null) }}
          onChangeFim={v => { setRefFim(v); setActivePreset(null) }}
          borderColor="border-orange-500"
        />

        {totalFiltersActive > 0 && (
          <button
            onClick={clearFilters}
            className="inline-flex items-center gap-1 px-2 py-1.5 text-sm font-medium text-red-600 hover:bg-red-50 rounded-md"
          >
            <X className="h-3 w-3" /> Limpar filtros ({totalFiltersActive})
          </button>
        )}

        {/* Busca encostada com os demais filtros (antes ficava perdida
            no canto direito por causa de um spacer flex-1 — removido) */}
        <div className="relative">
          <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400" />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder={`Buscar ${visibleCards[0]?.level_label?.toLowerCase() ?? ''}...`}
            className="pl-7 pr-7 py-1.5 text-sm border border-slate-300 rounded-md bg-white shadow-sm w-56"
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

        {/* Botões Importar e Consolidar removidos a pedido do gestor —
            usuários estavam clicando sem querer. Ações de import/consolidação
            ficam restritas ao menu de administração (não neste painel). */}
      </div>

      {/* ── Chips dos filtros ativos ────────────────────────────────────────── */}
      {totalFiltersActive > 0 && (
        <div className="flex flex-wrap items-center gap-1 mb-3">
          <span className="text-sm uppercase tracking-wider text-slate-500 font-semibold mr-1">
            <Filter className="h-3 w-3 inline -mt-0.5" /> Filtros ativos:
          </span>
          {FILTER_DIMS.flatMap(d => {
            const vals = filters[d.col] ?? []
            const opts = optionsFor(d.from)
            return vals.map(v => {
              const label = opts.find(o => o.key === v)?.label ?? v
              return (
                <span key={`${d.col}:${v}`} className="inline-flex items-center gap-1 px-2 py-0.5 text-sm bg-slate-100 border border-slate-200 rounded-full">
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
        <div className="flex items-center gap-1 mb-3 text-sm text-slate-600">
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

      {/* ── Legenda do período: a quem "Anterior"/"Atual" se referem ────────── */}
      {data?.periodo?.label && (
        <div className="mb-2 inline-flex items-center gap-1.5 text-sm font-semibold text-slate-600 bg-white border border-slate-200 rounded-md px-3 py-1.5 shadow-sm">
          <Calendar className="h-3.5 w-3.5 text-slate-400" />
          {data.periodo.label}
        </div>
      )}

      {/* ── TOTAL (card único com cabeçalho próprio) ────────────────────────── */}
      {totalCard && (
        <div className="bg-white border border-slate-200 rounded-lg overflow-hidden mb-4 shadow-sm">
          <ColumnsHeader hidePosit={hidePosit} />
          <DataRow card={totalCard} isTotal hidePosit={hidePosit} />
        </div>
      )}

      {/* ── Banner do nível atual — destaca QUE tipo de dado está listado ── */}
      {data && data.next_level_label && (
        <div className="bg-gradient-to-r from-indigo-50 via-sky-50 to-white border border-sky-200 rounded-lg px-4 py-2.5 mb-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <span className="inline-flex items-center px-2.5 py-1 rounded-md bg-sky-600 text-white text-sm font-bold uppercase tracking-wider shadow-sm">
              {data.next_level_label}
            </span>
          </div>
          <span className="text-sm text-slate-500 tabular-nums shrink-0">
            {visibleCards.length} {visibleCards.length === 1 ? 'item' : 'itens'}
          </span>
        </div>
      )}

      {/* ── LISTA de fornecedores/GGV/equipe ─────────────────────────────────── */}
      <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-sm">
        {/* Cabeçalho colorido da lista (mesmo padrão do total) */}
        <ColumnsHeader hidePosit={hidePosit} />

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
            hidePosit={hidePosit}
            onClick={c.level === 'cod_prod' ? undefined : () => handleDrill(c)}
          />
        ))}
      </div>
    </div>
  )
}
