import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Semaforo } from '@/components/farol/Semaforo'
import type { Cor } from '@/components/farol/Semaforo'
import { useAuth } from '@/contexts/AuthContext'
import FarolExecutivo from './FarolExecutivo'

const PERSONAS_EXECUTIVO = new Set(['diretor', 'gerente_geral'])

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
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}
function fmtPct(v: number) { return v.toFixed(1) + '%' }
function fmtNum(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 1 }) }

function parsePeriodo(s: string): { ano: number; mes: number } {
  const [y, m] = s.split('-')
  return { ano: +y, mes: +m }
}

const MES_NOMES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']
function fmtMesAno(ano: number, mes: number) {
  return `${MES_NOMES[mes] ?? mes}/${ano}`
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
  verde: 'text-emerald-600',
  amarelo: 'text-amber-600',
  vermelho: 'text-red-600',
}

// ─── Hook de dados ─────────────────────────────────────────────────────────────

function useCards(view: string, compMode: string, refAno: number, refMes: number, drillPath: DrillStep[], enabled = true) {
  const drillParam = JSON.stringify(drillPath)
  return useQuery<CardsResponse>({
    queryKey: ['farol-v2-cards', view, compMode, refAno, refMes, drillParam],
    queryFn: async () => {
      const params = new URLSearchParams({
        view,
        comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
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

function KPIBar({ kpi, periodo }: { kpi: KPI; periodo: CardsResponse['periodo'] }) {
  return (
    <div className="bg-white border border-slate-100 rounded-xl shadow-sm p-4 mb-4">
      <p className="text-xs text-slate-400 mb-3 font-medium">{periodo.label}</p>
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4">
        <div>
          <p className="text-xs text-slate-500">Total Atual</p>
          <p className="text-lg font-bold text-slate-800">{fmtBRL(kpi.total_atual)}</p>
          <p className="text-xs text-slate-400">vs {fmtBRL(kpi.total_ant)}</p>
        </div>
        <div>
          <p className="text-xs text-slate-500">Atingimento</p>
          <p className={`text-lg font-bold ${COR_TEXT[kpi.total_cor]}`}>{fmtPct(kpi.total_pct)}</p>
          <div className="flex gap-1 mt-1">
            <span className="text-xs text-emerald-600 font-medium">{kpi.verdes}🟢</span>
            <span className="text-xs text-red-500 font-medium">{kpi.vermelhos}🔴</span>
          </div>
        </div>
        <div>
          <p className="text-xs text-slate-500">Faturado</p>
          <p className="text-base font-semibold text-slate-700">{fmtBRL(kpi.total_faturado)}</p>
        </div>
        <div>
          <p className="text-xs text-slate-500">Transmitido</p>
          <p className="text-base font-semibold text-slate-700">{fmtBRL(kpi.total_transmitido)}</p>
        </div>
        <div>
          <p className="text-xs text-slate-500">Positivação</p>
          <p className="text-base font-semibold text-slate-700">{fmtPct(kpi.total_positpct)}</p>
          <p className="text-xs text-slate-400">{kpi.total_positivados}/{kpi.total_base_cli} clientes</p>
        </div>
        <div>
          <p className="text-xs text-slate-500">Mix médio</p>
          <p className="text-base font-semibold text-slate-700">{fmtNum(kpi.avg_mix)} itens/cli</p>
        </div>
      </div>
    </div>
  )
}

function CardVenda({ card, onClick }: { card: CardItem; onClick: () => void }) {
  const barW = Math.min(100, card.pct)
  return (
    <button
      onClick={onClick}
      className={`bg-white border border-slate-100 border-l-4 ${COR_BORDER[card.cor]} rounded-xl shadow-sm hover:shadow-md transition-all text-left w-full overflow-hidden`}
    >
      {/* Barra de progresso */}
      <div className="h-1 bg-slate-100">
        <div className={`h-1 ${COR_BAR[card.cor]} transition-all`} style={{ width: `${barW}%` }} />
      </div>
      <div className="p-4">
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-2 min-w-0">
            <Semaforo cor={card.cor} size="sm" />
            <div className="min-w-0">
              <p className="text-sm font-semibold text-slate-800 truncate leading-tight">{card.label}</p>
              <p className="text-xs text-slate-400">{card.level_label} • {card.key}</p>
            </div>
          </div>
          <div className="text-right shrink-0">
            <p className={`text-lg font-bold tabular-nums ${COR_TEXT[card.cor]}`}>{fmtPct(card.pct)}</p>
          </div>
        </div>

        <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-slate-600">
          <div>
            <span className="text-slate-400">Atual: </span>
            <span className="font-medium">{fmtBRL(card.valor_atual)}</span>
          </div>
          <div>
            <span className="text-slate-400">Anterior: </span>
            <span className="font-medium">{fmtBRL(card.valor_ant)}</span>
          </div>
          <div>
            <span className="text-slate-400">Fat: </span>
            <span className="font-medium">{fmtBRL(card.faturado)}</span>
          </div>
          <div>
            <span className="text-slate-400">Trans: </span>
            <span className="font-medium">{fmtBRL(card.transmitido)}</span>
          </div>
        </div>

        {(card.base_cli > 0 || card.mix > 0) && (
          <div className="mt-2 flex gap-3 text-xs text-slate-500 border-t border-slate-50 pt-2">
            {card.base_cli > 0 && (
              <span>👥 {fmtPct(card.positpct)} posit. ({card.positivados}/{card.base_cli})</span>
            )}
            {card.mix > 0 && (
              <span>📦 {fmtNum(card.mix)} itens/cli</span>
            )}
          </div>
        )}
      </div>
    </button>
  )
}

function Breadcrumb({
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
  const { tipoPersona, spRole } = useAuth()
  const navigate = useNavigate()

  // Hooks must always run in the same order — conditional return only after all hooks
  const [view, setView]           = useState<'V01' | 'V02' | 'V03'>('V01')
  const [compMode, setCompMode]   = useState('yoy')
  const [drillPath, setDrillPath] = useState<DrillStep[]>([])
  const [refAno, setRefAno]       = useState(0)
  const [refMes, setRefMes]       = useState(0)

  const isExecutivo = !!(
    (tipoPersona && PERSONAS_EXECUTIVO.has(tipoPersona)) || spRole === 'admin_fbtax'
  )

  const { data, isLoading, error } = useCards(view, compMode, refAno, refMes, drillPath, !isExecutivo)

  // Sincroniza seleção de período quando dados chegam pela primeira vez
  const autoRef = useCallback((d: CardsResponse) => {
    if (refAno === 0 && d.periodo.ref_ano) {
      setRefAno(d.periodo.ref_ano)
      setRefMes(d.periodo.ref_mes)
    }
  }, [refAno])
  if (data && refAno === 0) autoRef(data)

  const handleDrill = (card: CardItem) => {
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

  if (isExecutivo) return <FarolExecutivo />

  return (
    <div className="min-h-full">
      {/* ── Barra de controles ─────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-4">
        {/* Abas de visão */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {(['V01', 'V02', 'V03'] as const).map(v => (
            <button
              key={v}
              onClick={() => handleViewChange(v)}
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                view === v
                  ? 'bg-primary text-white'
                  : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              {v === 'V01' ? 'Fornec →' : v === 'V02' ? 'RCA →' : 'Diretoria →'}
            </button>
          ))}
        </div>

        {/* Modo de comparação */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {[
            { id: 'yoy', label: 'YoY' },
            { id: 'ytd', label: 'YTD' },
            { id: 'mom', label: 'MoM' },
          ].map(m => (
            <button
              key={m.id}
              onClick={() => { setCompMode(m.id); setDrillPath([]) }}
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                compMode === m.id
                  ? 'bg-slate-700 text-white'
                  : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              {m.label}
            </button>
          ))}
        </div>

        {/* Seletor de período */}
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

        {/* Botão importar */}
        <button
          onClick={() => navigate('/farol/importar')}
          className="ml-auto h-8 px-3 rounded-lg border border-dashed border-slate-300 text-xs text-slate-500 hover:border-primary hover:text-primary transition-colors shrink-0"
        >
          + Importar dados
        </button>
      </div>

      {/* ── KPI Bar ─────────────────────────────────────────────────────── */}
      {data?.kpi && data.kpi.total_atual > 0 && (
        <KPIBar kpi={data.kpi} periodo={data.periodo} />
      )}

      {/* ── Breadcrumb ───────────────────────────────────────────────────── */}
      <Breadcrumb drillPath={drillPath} onNavigate={handleBreadcrumb} />

      {/* ── Nível atual ─────────────────────────────────────────────────── */}
      {data && (
        <p className="text-xs text-slate-400 mb-3">
          Exibindo por{' '}
          <span className="font-medium text-slate-600">{data.next_level_label}</span>
          {' '}· {data.cards.length} itens
        </p>
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
          <p className="text-4xl mb-3">📊</p>
          <p className="text-sm font-medium text-slate-500">Nenhum dado encontrado</p>
          <p className="text-xs mt-1">
            Importe um CSV de vendas para começar.{' '}
            <button
              onClick={() => navigate('/farol/importar')}
              className="text-primary hover:underline"
            >
              Ir para importação →
            </button>
          </p>
        </div>
      )}

      {/* ── Grid de cards ───────────────────────────────────────────────── */}
      {!isLoading && !error && data && data.cards.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {data.cards.map(card => (
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
