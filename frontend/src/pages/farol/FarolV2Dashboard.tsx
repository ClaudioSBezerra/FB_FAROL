import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { ChevronDown, Users, Package, TrendingUp, TrendingDown, Minus } from 'lucide-react'
import type { Cor } from '@/components/farol/Semaforo'
import { useAuth } from '@/contexts/AuthContext'
import { FilialSelector } from '@/components/FilialSelector'
import {
  Popover, PopoverContent, PopoverTrigger,
} from '@/components/ui/popover'
import FarolExecutivo from './FarolExecutivo'
import { useSortedCards } from '@/components/farol/SortToggle'

// Personas que abrem o painel em TABELA (FarolExecutivo) — hoje o painel web
// de fato. O layout em cards deste arquivo é o legado, mantido para o que não
// se encaixa na tabela.
//
// GGV e supervisor entraram em 17/08/2026, quando o escopo por persona os
// deixou usar o web pela primeira vez: eles caíram no layout de cards e a
// diferença saltou aos olhos: a proposta nunca foi dar a eles uma tela
// diferente, e sim a MESMA tela, com os números recortados na equipe deles.
// O recorte é responsabilidade do servidor (farol_escopo.go), então a tabela
// funciona igual — só chega menos dado.
const PERSONAS_EXECUTIVO = new Set(['ceo', 'diretor', 'gerente_geral', 'ggv', 'supervisor'])

// ─── Tipos ────────────────────────────────────────────────────────────────────

export interface DrillStep { level: string; value: string; label: string }

export interface CardItem {
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
  positivados_ant: number
  base_cli: number
  positpct: number
  positpct_ant: number
  mix: number
}

export interface KPI {
  total_atual: number
  total_ant: number
  total_pct: number
  total_cor: Cor
  total_faturado: number
  total_transmitido: number
  total_positivados: number
  total_positivados_ant: number
  total_base_cli: number
  total_positpct: number
  total_positpct_ant: number
  avg_mix: number
  verdes: number
  vermelhos: number
}

export interface CardsResponse {
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

// import + export, NÃO `export ... from`: a forma de re-export puro entrega os
// símbolos a quem importa deste arquivo mas não cria binding LOCAL, e este
// módulo usa <BRLValue> no KPIBar e no CardVenda. O resultado foi
// "ReferenceError: BRLValue is not defined" derrubando a tela inteira —
// visível só para quem cai no layout de cards (GGV/supervisor), porque as
// personas executivas renderizam FarolExecutivo, que importa direto.
import { fmtBRL, BRLValue } from '@/lib/farolMoney'

export { fmtBRL, BRLValue }
export function fmtPct(v: number) { return v.toFixed(1) + '%' }
export function fmtNum(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 1 }) }
export function fmtInt(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 0 }) }

export function parsePeriodo(s: string): { ano: number; mes: number } {
  const [y, m] = s.split('-')
  return { ano: +y, mes: +m }
}

const MES_NOMES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']
export function fmtMesAno(ano: number, mes: number) {
  return `${MES_NOMES[mes] ?? mes}/${ano}`
}

function fmtRange(ano: number, mes: number): string {
  if (!ano || !mes) return '--/--/---- a --/--/----'
  const lastDay = new Date(ano, mes, 0).getDate()
  const m = String(mes).padStart(2, '0')
  return `01/${m}/${ano} a ${String(lastDay).padStart(2, '0')}/${m}/${ano}`
}

function autoCompPeriodo(refAno: number, refMes: number, compMode: string): { ano: number; mes: number } {
  if (compMode === 'yoy' || compMode === 'ytd') return { ano: refAno - 1, mes: refMes }
  if (refMes > 1) return { ano: refAno, mes: refMes - 1 }
  return { ano: refAno - 1, mes: 12 }
}

// ─── PeriodRangeFilter ────────────────────────────────────────────────────────

function PeriodRangeFilter({
  periodos, refAno, refMes, setRefAno, setRefMes,
  compMode, compAno, compMes, setCompAno, setCompMes,
  onClearDrill,
}: {
  periodos: string[]
  refAno: number; refMes: number
  setRefAno: (v: number) => void; setRefMes: (v: number) => void
  compMode: string
  compAno: number; compMes: number
  setCompAno: (v: number) => void; setCompMes: (v: number) => void
  onClearDrill: () => void
}) {
  const [open, setOpen] = useState(false)
  const auto = autoCompPeriodo(refAno, refMes, compMode)
  const effComp = compMode === 'mom' && compAno > 0
    ? { ano: compAno, mes: compMes }
    : auto

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button className="h-8 inline-flex items-center gap-1.5 rounded-lg border border-slate-200 bg-white px-2.5 shadow-sm text-sm text-slate-700 hover:bg-slate-50 transition-colors shrink-0 whitespace-nowrap">
          <span className="text-sm font-semibold text-slate-400 uppercase tracking-wide">Atual</span>
          <span className="tabular-nums">{fmtRange(refAno, refMes)}</span>
          <span className="text-slate-300 px-0.5 font-light">×</span>
          <span className="text-sm font-semibold text-slate-400 uppercase tracking-wide">Ant.</span>
          <span className="tabular-nums text-slate-500">{fmtRange(effComp.ano, effComp.mes)}</span>
          <ChevronDown className="h-3 w-3 ml-0.5 text-slate-400 shrink-0" />
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-3" align="start">
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1">
            <span className="text-sm font-semibold text-slate-500 uppercase tracking-wide">Base Atual</span>
            <select
              value={refAno > 0 ? `${refAno}-${String(refMes).padStart(2, '0')}` : ''}
              onChange={e => {
                const p = parsePeriodo(e.target.value)
                setRefAno(p.ano); setRefMes(p.mes); onClearDrill()
                setOpen(false)
              }}
              className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-sm text-slate-700 focus:outline-none focus:ring-2 focus:ring-primary/30 w-full"
            >
              {periodos.map(p => {
                const { ano, mes } = parsePeriodo(p)
                return <option key={p} value={p}>{fmtRange(ano, mes)}</option>
              })}
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <span className="text-sm font-semibold text-slate-500 uppercase tracking-wide">Base Anterior</span>
            {compMode === 'mom' ? (
              <select
                value={compAno > 0 ? `${compAno}-${String(compMes).padStart(2, '0')}` : ''}
                onChange={e => {
                  if (!e.target.value) { setCompAno(0); setCompMes(0) }
                  else { const p = parsePeriodo(e.target.value); setCompAno(p.ano); setCompMes(p.mes) }
                }}
                className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-sm text-slate-700 focus:outline-none focus:ring-2 focus:ring-primary/30 w-full"
              >
                <option value="">Auto — {fmtRange(auto.ano, auto.mes)}</option>
                {periodos.map(p => {
                  const { ano, mes } = parsePeriodo(p)
                  return <option key={p} value={p}>{fmtRange(ano, mes)}</option>
                })}
              </select>
            ) : (
              <div className="h-8 flex items-center px-2 rounded-lg border border-slate-100 bg-slate-50 text-sm text-slate-500 tabular-nums">
                {fmtRange(auto.ano, auto.mes)}
                <span className="ml-1.5 text-sm text-slate-400">(automático)</span>
              </div>
            )}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}

// Paleta tonal — fundo claro com texto profundo. Mais elegante que cor saturada.
const COR_BG: Record<Cor, string> = {
  verde:    'bg-gradient-to-br from-emerald-50/60 via-white to-white',
  amarelo:  'bg-gradient-to-br from-amber-50/60 via-white to-white',
  vermelho: 'bg-gradient-to-br from-red-50/70 via-white to-white',
}
const COR_RING: Record<Cor, string> = {
  verde:    'ring-1 ring-emerald-100 hover:ring-emerald-200',
  amarelo:  'ring-1 ring-amber-100 hover:ring-amber-200',
  vermelho: 'ring-1 ring-red-200 hover:ring-red-300',
}
const COR_DOT: Record<Cor, string> = {
  verde:    'bg-emerald-500',
  amarelo:  'bg-amber-500',
  vermelho: 'bg-red-500',
}
const COR_DOT_PING: Record<Cor, string> = {
  verde:    '',
  amarelo:  '',
  vermelho: 'animate-ping bg-red-400',
}
const COR_CHIP: Record<Cor, string> = {
  verde:    'bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-200',
  amarelo:  'bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-200',
  vermelho: 'bg-red-50 text-red-700 ring-1 ring-inset ring-red-200',
}
const COR_BORDER: Record<Cor, string> = {
  verde: 'border-l-emerald-500',
  amarelo: 'border-l-amber-400',
  vermelho: 'border-l-red-500',
}
const COR_BAR: Record<Cor, string> = {
  verde: 'bg-emerald-500',
  amarelo: 'bg-amber-400',
  vermelho: 'bg-red-500',
}
const COR_TEXT: Record<Cor, string> = {
  verde: 'text-emerald-700',
  amarelo: 'text-amber-700',
  vermelho: 'text-red-700',
}

// ─── Hook de dados ─────────────────────────────────────────────────────────────

function useCards(
  view: string, compMode: string,
  refAno: number, refMes: number,
  compAno: number, compMes: number,
  drillPath: DrillStep[], enabled = true,
) {
  const drillParam = JSON.stringify(drillPath)
  return useQuery<CardsResponse>({
    queryKey: ['farol-v2-cards', view, compMode, refAno, refMes, compAno, compMes, drillParam],
    queryFn: async () => {
      const params = new URLSearchParams({
        view,
        comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(compAno > 0 && compMes > 0 && { comp_ano: String(compAno), comp_mes: String(compMes) }),
        ...(drillPath.length > 0 && { drill: drillParam }),
      })
      const r = await fetch(`/api/v2/farol/cards?${params}`)
      if (!r.ok) throw new Error('Falha ao carregar dados do Farol')
      return r.json()
    },
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
    enabled,
  })
}

// ─── Componentes ──────────────────────────────────────────────────────────────

export function KPIBar({
  kpi, periodo, periodos, refAno, refMes, onPreset,
}: {
  kpi: KPI
  periodo: CardsResponse['periodo']
  periodos: string[]
  refAno: number
  refMes: number
  onPreset: (ano: number, mes: number) => void
}) {
  const antLabel = periodo.ant_label || 'Anterior'
  const curLabel = periodo.cur_label || 'Atual'

  const mesCorrente = periodos.length > 0 ? parsePeriodo(periodos[periodos.length - 1]) : null
  const mesFechado  = periodos.length > 1 ? parsePeriodo(periodos[periodos.length - 2]) : null
  const isCorrente  = !!mesCorrente && refAno === mesCorrente.ano && refMes === mesCorrente.mes
  const isFechado   = !!mesFechado  && refAno === mesFechado.ano  && refMes === mesFechado.mes

  return (
    <div className="bg-white border border-slate-100 rounded-xl shadow-sm p-4 mb-4">
      {/* Presets de período — substituem o texto "periodo.label" */}
      {periodos.length > 0 && (
        <div className="flex items-center gap-1.5 mb-3">
          <div className="flex rounded-lg border border-slate-200 overflow-hidden shrink-0">
            {mesCorrente && (
              <button
                onClick={() => onPreset(mesCorrente.ano, mesCorrente.mes)}
                className={`px-2.5 py-1 text-sm font-medium transition-colors ${isCorrente ? 'bg-primary text-white' : 'bg-white text-slate-600 hover:bg-slate-50'}`}
              >
                Mês corrente
              </button>
            )}
            {mesFechado && (
              <button
                onClick={() => onPreset(mesFechado.ano, mesFechado.mes)}
                className={`px-2.5 py-1 text-sm font-medium transition-colors border-l border-slate-200 ${isFechado ? 'bg-slate-700 text-white' : 'bg-white text-slate-600 hover:bg-slate-50'}`}
              >
                Mês fechado
              </button>
            )}
          </div>
        </div>
      )}
      {/* min-w-0 em cada célula: sem isso, um total ≥ R$ 1 bi ("R$ 1.234.567.890,12",
          uma só palavra pro CSS — Intl.NumberFormat não deixa quebrar no espaço)
          usa a largura intrínseca do texto como piso do item de grid e empurra/
          sobrepõe a coluna vizinha. break-words dá uma saída de quebra se, mesmo
          encolhido até o piso da coluna, ainda não couber numa linha. */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-7 gap-4">
        <div className="min-w-0">
          <p className="text-sm text-slate-500">Total Anterior</p>
          <p className="text-sm font-bold font-bold text-slate-500 break-words"><BRLValue v={kpi.total_ant} /></p>
          <p className="text-sm text-slate-400 truncate" title={antLabel}>{antLabel}</p>
        </div>
        <div className="min-w-0">
          <p className="text-sm text-slate-500">Total Atual</p>
          <p className="text-sm font-bold font-bold text-slate-800 break-words"><BRLValue v={kpi.total_atual} /></p>
          <p className="text-sm text-slate-400 truncate" title={curLabel}>{curLabel}</p>
        </div>
        <div className="min-w-0">
          <p className="text-sm text-slate-500">Atingimento</p>
          <p className={`text-sm font-bold font-bold ${COR_TEXT[kpi.total_cor]}`}>{fmtPct(kpi.total_pct)}</p>
          <div className="flex gap-1.5 mt-1">
            <span className={`inline-flex items-center gap-1 text-sm font-semibold px-1.5 py-0.5 rounded-md ${COR_CHIP.verde}`}>
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
              {kpi.verdes}
            </span>
            <span className={`inline-flex items-center gap-1 text-sm font-semibold px-1.5 py-0.5 rounded-md ${COR_CHIP.vermelho}`}>
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full rounded-full bg-red-400 opacity-60 animate-ping" />
                <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-red-500" />
              </span>
              {kpi.vermelhos}
            </span>
          </div>
        </div>
        <div className="min-w-0">
          <p className="text-sm text-slate-500">Faturado</p>
          <p className="text-sm font-semibold text-slate-700 break-words"><BRLValue v={kpi.total_faturado} /></p>
        </div>
        <div className="min-w-0">
          <p className="text-sm text-slate-500">Transmitido</p>
          <p className="text-sm font-semibold text-slate-700 break-words"><BRLValue v={kpi.total_transmitido} /></p>
        </div>
        <div className="min-w-0">
          <p className="text-sm text-slate-500">Positivação</p>
          <p className="text-sm font-semibold text-slate-700">{fmtPct(kpi.total_positpct)}</p>
          <p className="text-sm text-slate-400">{kpi.total_positivados}/{kpi.total_base_cli} clientes</p>
        </div>
        <div className="min-w-0">
          <p className="text-sm text-slate-500">Mix médio</p>
          <p className="text-sm font-semibold text-slate-700">{fmtNum(kpi.avg_mix)} itens/cli</p>
        </div>
      </div>
    </div>
  )
}

export function CardVenda({ card, onClick }: { card: CardItem; onClick: () => void }) {
  const barW = Math.min(100, card.pct)
  // Delta absoluto vs anterior (em pp) — interpretação como melhora/piora do atingimento
  const delta = card.valor_ant > 0
    ? ((card.valor_atual - card.valor_ant) / card.valor_ant) * 100
    : 0
  const deltaUp = delta > 0.5
  const deltaDown = delta < -0.5
  return (
    <button
      onClick={onClick}
      className={`group relative ${COR_BG[card.cor]} border border-slate-200/60 ${COR_RING[card.cor]} rounded-xl shadow-sm hover:shadow-md hover:-translate-y-0.5 transition-all duration-200 text-left w-full overflow-hidden`}
    >
      {/* Barra de progresso refinada */}
      <div className="h-1 bg-slate-100/80">
        <div className={`h-full ${COR_BAR[card.cor]} transition-all duration-500`} style={{ width: `${barW}%` }} />
      </div>

      <div className="p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-2.5 min-w-0">
            {/* Dot pulse — vermelho pulsa, verde/amarelo estático */}
            <span className="relative flex h-2.5 w-2.5 mt-1.5 shrink-0">
              {card.cor === 'vermelho' && (
                <span className={`absolute inline-flex h-full w-full rounded-full opacity-60 ${COR_DOT_PING[card.cor]}`} />
              )}
              <span className={`relative inline-flex rounded-full h-2.5 w-2.5 ${COR_DOT[card.cor]}`} />
            </span>
            <div className="min-w-0">
              <p className="text-sm font-semibold text-slate-800 truncate leading-tight">{card.label}</p>
              <p className="text-sm text-slate-400 mt-0.5 font-mono">{card.key}</p>
            </div>
          </div>
          {/* Percentual + delta */}
          <div className="text-right shrink-0">
            <p className={`text-sm font-bold font-bold tabular-nums leading-none ${COR_TEXT[card.cor]}`}>{fmtPct(card.pct)}</p>
            {(deltaUp || deltaDown) && (
              <p className={`mt-1 inline-flex items-center gap-0.5 text-sm font-medium tabular-nums ${
                deltaUp ? 'text-emerald-600' : 'text-red-600'
              }`}>
                {deltaUp
                  ? <TrendingUp className="h-3 w-3" strokeWidth={2.5} />
                  : <TrendingDown className="h-3 w-3" strokeWidth={2.5} />}
                {Math.abs(delta).toFixed(1)}%
              </p>
            )}
            {!deltaUp && !deltaDown && card.valor_ant > 0 && (
              <p className="mt-1 inline-flex items-center gap-0.5 text-sm font-medium text-slate-400 tabular-nums">
                <Minus className="h-3 w-3" strokeWidth={2.5} />
                estável
              </p>
            )}
          </div>
        </div>

        {/* Métricas primárias — Atual + Anterior alinhados. min-w-0 nas células +
            break-words no valor: mesmo motivo do totalizador acima — fornecedor/
            gerência grande pode passar de R$ 1 bi e esse valor não tem espaço
            que o CSS possa usar pra quebrar sozinho. */}
        <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
          <div className="min-w-0 space-y-0.5">
            <p className="text-sm uppercase tracking-wide text-slate-400 font-medium">Atual</p>
            <p className="text-sm font-semibold text-slate-800 tabular-nums break-words"><BRLValue v={card.valor_atual} /></p>
          </div>
          <div className="min-w-0 space-y-0.5">
            <p className="text-sm uppercase tracking-wide text-slate-400 font-medium">Anterior</p>
            <p className="text-sm font-semibold text-slate-500 tabular-nums break-words"><BRLValue v={card.valor_ant} /></p>
          </div>
        </div>

        {/* Métricas secundárias — Fat + Trans em fonte menor. flex-wrap: se os
            dois valores juntos não couberem numa linha, Trans desce pra
            próxima em vez de estourar a largura do card. */}
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-slate-500 tabular-nums">
          <span className="break-words"><span className="text-slate-400">Fat</span> <span className="font-medium text-slate-600"><BRLValue v={card.faturado} /></span></span>
          <span className="break-words"><span className="text-slate-400">Trans</span> <span className="font-medium text-slate-600"><BRLValue v={card.transmitido} /></span></span>
        </div>

        {/* Rodapé — Positivação + Mix com ícones Lucide */}
        {(card.base_cli > 0 || card.mix > 0) && (
          <div className="mt-2.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-slate-500 border-t border-slate-100/80 pt-2">
            {card.base_cli > 0 && (
              <span className="inline-flex items-center gap-1">
                <Users className="h-3 w-3 text-slate-400" strokeWidth={2} />
                <span className="font-medium tabular-nums">{fmtPct(card.positpct)}</span>
                <span className="text-slate-400 tabular-nums">({card.positivados}/{card.base_cli})</span>
              </span>
            )}
            {card.mix > 0 && (
              <span className="inline-flex items-center gap-1">
                <Package className="h-3 w-3 text-slate-400" strokeWidth={2} />
                <span className="font-medium tabular-nums">{fmtNum(card.mix)}</span>
                <span className="text-slate-400">itens/cli</span>
              </span>
            )}
          </div>
        )}
      </div>
    </button>
  )
}

export function Breadcrumb({
  drillPath,
  onNavigate,
}: {
  drillPath: DrillStep[]
  onNavigate: (idx: number) => void
}) {
  if (drillPath.length === 0) return null
  return (
    <nav className="flex items-center gap-1 text-sm mb-3 flex-wrap">
      <button
        onClick={() => onNavigate(-1)}
        className="text-primary hover:underline font-medium"
      >
        Início
      </button>
      {drillPath.map((d, i) => (
        <span key={i} className="flex items-center gap-1">
          <span className="text-slate-400">/</span>
          {i < drillPath.length - 1 ? (
            <button
              onClick={() => onNavigate(i)}
              className="text-primary hover:underline"
            >
              {d.label}
            </button>
          ) : (
            <span className="text-slate-600 font-medium">{d.label}</span>
          )}
        </span>
      ))}
    </nav>
  )
}

// ─── FarolV2Dashboard ─────────────────────────────────────────────────────────

export default function FarolV2Dashboard() {
  const { tipoPersona, spRole, user } = useAuth()
  const canImport = user?.role === 'admin' || spRole === 'admin_fbtax' || tipoPersona === 'ti'
  const navigate = useNavigate()

  // Hooks must always run in the same order — conditional return only after all hooks
  const [view, setView]           = useState<'V01' | 'V02' | 'V03'>('V01')
  const [compMode, setCompMode]   = useState('yoy')
  const [drillPath, setDrillPath] = useState<DrillStep[]>([])
  const [refAno, setRefAno]       = useState(0)
  const [refMes, setRefMes]       = useState(0)
  // Override do período de comparação (só para mom). 0 = automático (mês anterior).
  const [compAno, setCompAno]     = useState(0)
  const [compMes, setCompMes]     = useState(0)

  const isExecutivo = !!(
    (tipoPersona && PERSONAS_EXECUTIVO.has(tipoPersona)) || spRole === 'admin_fbtax'
  )

  const { data, isLoading, error } = useCards(
    view, compMode, refAno, refMes,
    compMode === 'mom' ? compAno : 0,
    compMode === 'mom' ? compMes : 0,
    drillPath, !isExecutivo,
  )

  // Sincroniza seleção de período quando dados chegam pela primeira vez
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

  const handleBreadcrumb = (idx: number) => {
    if (idx < 0) {
      setDrillPath([])
    } else {
      setDrillPath(prev => prev.slice(0, idx + 1))
    }
  }

  const handleViewChange = (v: 'V01' | 'V02' | 'V03') => {
    setView(v)
    setDrillPath([])
  }

  const periodos = data?.periodos ?? []

  // Cards ordenados por venda atual desc (ranking absoluto). Sem toggle UI
  // — o painel mobile/dashboard é navegação por drill, não análise comparativa.
  const { sorted: sortedCards } =
    useSortedCards(data?.cards ?? [], 'farol.sort.dashboard', { field: 'valor', direction: 'desc' })

  if (isExecutivo) return <FarolExecutivo />

  return (
    <div className="min-h-full uppercase text-sm [&_*]:uppercase">
      {/* ── Barra de controles ─────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-4">

        {/* ── Filtros de dados: Filial + Mês ── */}
        <FilialSelector />

        {periodos.length > 0 && (
          <PeriodRangeFilter
            periodos={periodos}
            refAno={refAno} refMes={refMes}
            setRefAno={setRefAno} setRefMes={setRefMes}
            compMode={compMode}
            compAno={compAno} compMes={compMes}
            setCompAno={setCompAno} setCompMes={setCompMes}
            onClearDrill={() => setDrillPath([])}
          />
        )}

        {/* ── Configuração de visão ── */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {([
            // Renomeado de "Por Indústria" em 28/08/2026 — este nível agrupa por
            // cod_fornec cru (ver App.tsx FarolExecutivo.tsx pro histórico
            // completo da decisão e o novo cross-filter "FORN DIST" canônico,
            // que era "Indústria" até virar "FORN DIST" no mesmo dia).
            { id: 'V01' as const, label: 'Por FORN.GERAL' },
            { id: 'V03' as const, label: 'Por Gerência' },
            { id: 'V02' as const, label: 'Por Equipe' },
          ]).map(v => (
            <button
              key={v.id}
              onClick={() => handleViewChange(v.id)}
              className={`px-3 py-1.5 text-sm font-medium transition-colors ${
                view === v.id
                  ? 'bg-primary text-white'
                  : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              {v.label}
            </button>
          ))}
        </div>

        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {[
            { id: 'yoy', label: 'Ano a Ano' },
            { id: 'ytd', label: 'Projeção Anual' },
            { id: 'mom', label: 'Mês a Mês' },
          ].map(m => (
            <button
              key={m.id}
              onClick={() => { setCompMode(m.id); setDrillPath([]) }}
              className={`px-3 py-1.5 text-sm font-medium transition-colors ${
                compMode === m.id
                  ? 'bg-slate-700 text-white'
                  : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>

        {/* Botão importar — somente admin/TI */}
        {canImport && (
          <button
            onClick={() => navigate('/farol/importar')}
            className="ml-auto h-8 px-3 rounded-lg border border-dashed border-slate-300 text-sm text-slate-500 hover:border-primary hover:text-primary transition-colors shrink-0"
          >
            + Importar dados
          </button>
        )}
      </div>

      {/* ── KPI Bar ─────────────────────────────────────────────────────── */}
      {data?.kpi && data.kpi.total_atual > 0 && (
        <KPIBar
          kpi={data.kpi}
          periodo={data.periodo}
          periodos={periodos}
          refAno={refAno}
          refMes={refMes}
          onPreset={(ano, mes) => { setRefAno(ano); setRefMes(mes); setDrillPath([]) }}
        />
      )}

      {/* ── Breadcrumb ───────────────────────────────────────────────────── */}
      <Breadcrumb drillPath={drillPath} onNavigate={handleBreadcrumb} />

      {/* ── Banner do nível atual — destaca QUE tipo de dado está na tela ── */}
      {data && (
        <div className="bg-gradient-to-r from-indigo-50 via-sky-50 to-white border border-sky-200 rounded-lg px-4 py-2.5 mb-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <span className="inline-flex items-center px-2.5 py-1 rounded-md bg-sky-600 text-white text-sm font-bold uppercase tracking-wider shadow-sm">
              {data.next_level_label}
            </span>
          </div>
          <span className="text-sm text-slate-500 tabular-nums shrink-0">
            {data.cards.length} {data.cards.length === 1 ? 'item' : 'itens'}
          </span>
        </div>
      )}

      {/* ── Estados de carregamento / erro / vazio ───────────────────────── */}
      {isLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="bg-white border border-slate-100 rounded-xl h-40 animate-pulse" />
          ))}
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-xl p-6 text-red-700 text-sm">
          <p className="font-semibold mb-1">Erro ao carregar painel</p>
          <p>{(error as Error).message}</p>
        </div>
      )}

      {!isLoading && !error && data?.cards.length === 0 && (
        <div className="bg-white border border-dashed border-slate-200 rounded-xl p-12 text-center text-slate-400">
          <p className="text-sm font-bold mb-3">📊</p>
          <p className="text-sm font-medium text-slate-500">Nenhum dado encontrado</p>
          <p className="text-sm mt-1">
            {canImport ? (
              <>
                Importe um CSV de vendas para começar.{' '}
                <button
                  onClick={() => navigate('/farol/importar')}
                  className="text-primary hover:underline"
                >
                  Ir para importação →
                </button>
              </>
            ) : (
              <>Solicite ao administrador a importação dos dados.</>
            )}
          </p>
        </div>
      )}

      {/* ── Grid de cards ───────────────────────────────────────────────── */}
      {!isLoading && !error && data && sortedCards.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {sortedCards.map(card => (
            <CardVenda
              key={card.key}
              card={card}
              onClick={() => handleDrill(card)}
            />
          ))}
        </div>
      )}
    </div>
  )
}
