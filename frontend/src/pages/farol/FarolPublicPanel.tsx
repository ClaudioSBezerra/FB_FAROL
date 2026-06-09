import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { TrendingUp, TrendingDown, Minus, ChevronLeft } from 'lucide-react'
import {
  Breadcrumb,
  parsePeriodo, fmtMesAno, fmtBRL, fmtPct, fmtNum, fmtInt,
  type CardsResponse, type CardItem, type DrillStep, type KPI,
} from './FarolV2Dashboard'
import type { Cor } from '@/components/farol/Semaforo'

// Painel público do ION VENDAS — aberto sem login via link parametrizado
// (/m/CNPJ/SUP/cod ou /m/CNPJ/RCA/cod). Layout específico mobile-first em
// 3 seções claras (VENDA / POSITIVAÇÃO / MIX MÉDIO) conforme esboço do
// gerente. Linguagem visual herdada do CardVenda novo (paleta tonal +
// dot pulse + delta).

// ─── Paleta tonal (mesma do CardVenda em FarolV2Dashboard) ────────────────

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
const COR_TEXT: Record<Cor, string> = {
  verde:    'text-emerald-700',
  amarelo:  'text-amber-700',
  vermelho: 'text-red-700',
}
const COR_BAR: Record<Cor, string> = {
  verde:    'bg-emerald-500',
  amarelo:  'bg-amber-400',
  vermelho: 'bg-red-500',
}

// ─── Sub-componentes pequenos ────────────────────────────────────────────

function StatusDot({ cor }: { cor: Cor }) {
  return (
    <span className="relative flex h-2.5 w-2.5 shrink-0">
      {cor === 'vermelho' && (
        <span className="absolute inline-flex h-full w-full rounded-full opacity-60 bg-red-400 animate-ping" />
      )}
      <span className={`relative inline-flex rounded-full h-2.5 w-2.5 ${COR_DOT[cor]}`} />
    </span>
  )
}

function DeltaPct({ atual, anterior }: { atual: number; anterior: number }) {
  const delta = anterior > 0 ? ((atual - anterior) / anterior) * 100 : 0
  const up = delta > 0.5
  const down = delta < -0.5
  if (anterior <= 0) return null
  if (!up && !down) {
    return (
      <span className="inline-flex items-center gap-0.5 text-sm font-medium text-slate-400 tabular-nums">
        <Minus className="h-3 w-3" strokeWidth={2.5} />
        estável
      </span>
    )
  }
  return (
    <span className={`inline-flex items-center gap-0.5 text-sm font-semibold tabular-nums ${
      up ? 'text-emerald-600' : 'text-red-600'
    }`}>
      {up ? <TrendingUp className="h-3 w-3" strokeWidth={2.5} /> : <TrendingDown className="h-3 w-3" strokeWidth={2.5} />}
      {Math.abs(delta).toFixed(1)}%
    </span>
  )
}

// SectionLabel: cabeçalho de seção (VENDA, POSITIVAÇÃO, MIX MÉDIO)
//   • banner=false (padrão) → estilo discreto (texto slate-700 só com
//     uppercase). Usado nos cards individuais dos RCAs/fornecedores —
//     gestor pediu sem tarja pra não poluir.
//   • banner=true → tarja azul Farol #1e293b (mesmo do Login) + cor da coluna.
//     Usado SOMENTE no HeaderResumo (totalizador geral do supervisor)
//     pra reforçar visualmente "este é o consolidado".
const SECTION_COLOR: Record<'venda' | 'positivacao' | 'mix', string> = {
  venda:       'text-yellow-300',
  positivacao: 'text-lime-300',
  mix:         'text-fuchsia-400',
}
function SectionLabel({ children, tone = 'venda', banner = false }: {
  children: React.ReactNode
  tone?: 'venda' | 'positivacao' | 'mix'
  banner?: boolean
}) {
  if (banner) {
    return (
      <p className={`text-sm uppercase tracking-wider font-bold mb-1.5 bg-[#1e293b] ${SECTION_COLOR[tone]} px-2 py-1 rounded`}>{children}</p>
    )
  }
  return (
    <p className="text-sm uppercase tracking-wider font-bold text-slate-700 mb-1.5">{children}</p>
  )
}

// Cell: célula com label menor + valor
function Cell({ label, value, valueClass = 'text-slate-800' }: {
  label: string; value: React.ReactNode; valueClass?: string
}) {
  return (
    <div className="min-w-0">
      <p className="text-sm uppercase tracking-wide text-slate-400 font-medium leading-tight">{label}</p>
      <p className={`text-sm font-semibold tabular-nums truncate leading-tight mt-0.5 ${valueClass}`}>{value}</p>
    </div>
  )
}

// ─── HeaderResumo — substitui KPIBar no mobile (formato planilha do esboço)

function HeaderResumo({
  kpi, periodo, periodos, refAno, refMes, onPreset,
}: {
  kpi: KPI
  periodo: CardsResponse['periodo']
  periodos: string[]
  refAno: number
  refMes: number
  onPreset: (ano: number, mes: number) => void
}) {
  const mesCorrente = periodos.length > 0 ? parsePeriodo(periodos[periodos.length - 1]) : null
  const mesFechado  = periodos.length > 1 ? parsePeriodo(periodos[periodos.length - 2]) : null
  const isCorrente  = !!mesCorrente && refAno === mesCorrente.ano && refMes === mesCorrente.mes
  const isFechado   = !!mesFechado  && refAno === mesFechado.ano  && refMes === mesFechado.mes
  const antLabel    = periodo.ant_label || 'Anterior'
  const curLabel    = periodo.cur_label || 'Atual'
  const barW        = Math.min(100, kpi.total_pct)

  return (
    <div className={`relative ${COR_BG[kpi.total_cor]} border border-slate-200/60 ${COR_RING[kpi.total_cor]} rounded-xl shadow-sm overflow-hidden mb-4`}>
      {/* Barra de atingimento no topo */}
      <div className="h-1 bg-slate-100/80">
        <div className={`h-full ${COR_BAR[kpi.total_cor]} transition-all duration-500`} style={{ width: `${barW}%` }} />
      </div>

      {/* Linha 1 — presets de período */}
      {periodos.length > 0 && (
        <div className="px-4 pt-3 flex items-center gap-1.5">
          <div className="flex rounded-lg border border-slate-200 overflow-hidden shrink-0 bg-white">
            {mesCorrente && (
              <button
                onClick={() => onPreset(mesCorrente.ano, mesCorrente.mes)}
                className={`px-2.5 py-1 text-sm font-medium transition-colors ${
                  isCorrente ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
                }`}
              >Mês corrente</button>
            )}
            {mesFechado && (
              <button
                onClick={() => onPreset(mesFechado.ano, mesFechado.mes)}
                className={`px-2.5 py-1 text-sm font-medium transition-colors border-l border-slate-200 ${
                  isFechado ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
                }`}
              >Mês fechado</button>
            )}
          </div>
        </div>
      )}

      <div className="p-4 space-y-3">
        {/* SEÇÃO 1: VENDA */}
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <SectionLabel banner tone="venda">Venda</SectionLabel>
            <div className="flex items-center gap-2">
              <span className={`text-sm font-bold tabular-nums leading-none ${COR_TEXT[kpi.total_cor]}`}>
                {fmtPct(kpi.total_pct)}
              </span>
              <DeltaPct atual={kpi.total_atual} anterior={kpi.total_ant} />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-2">
            <Cell label={antLabel} value={fmtBRL(kpi.total_ant)} valueClass="text-slate-500" />
            <Cell label={curLabel} value={fmtBRL(kpi.total_atual)} />
            <Cell label="%" value={fmtPct(kpi.total_pct)} valueClass={COR_TEXT[kpi.total_cor]} />
          </div>
        </div>

        {/* SEÇÃO 2: POSITIVAÇÃO */}
        <div className="border-t border-slate-100 pt-2.5">
          <SectionLabel banner tone="positivacao">Positivação</SectionLabel>
          <div className="grid grid-cols-3 gap-2">
            <Cell label="Clientes Ativos" value={fmtInt(kpi.total_base_cli)} valueClass="text-slate-500" />
            <Cell label="Clientes Positivados" value={fmtInt(kpi.total_positivados)} />
            <Cell label="% Posit" value={fmtPct(kpi.total_positpct)} />
          </div>
        </div>

        {/* SEÇÃO 3: MIX MÉDIO */}
        <div className="border-t border-slate-100 pt-2.5">
          <SectionLabel banner tone="mix">Mix médio</SectionLabel>
          <div className="grid grid-cols-3 gap-2">
            <Cell label="Realizado" value={fmtNum(kpi.avg_mix) + ' itens/cli'} />
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── CardVendaPublic — card individual no formato do esboço ───────────────

function CardVendaPublic({ card, onClick }: { card: CardItem; onClick: () => void }) {
  const barW = Math.min(100, card.pct)
  return (
    <button
      onClick={onClick}
      className={`group relative ${COR_BG[card.cor]} border border-slate-200/60 ${COR_RING[card.cor]} rounded-xl shadow-sm hover:shadow-md transition-all duration-200 text-left w-full overflow-hidden`}
    >
      {/* Barra de progresso no topo */}
      <div className="h-1 bg-slate-100/80">
        <div className={`h-full ${COR_BAR[card.cor]} transition-all duration-500`} style={{ width: `${barW}%` }} />
      </div>

      <div className="p-4 space-y-3">
        {/* Header — nome + percentual + delta */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-2.5 min-w-0">
            <span className="mt-1.5"><StatusDot cor={card.cor} /></span>
            <div className="min-w-0">
              <p className="text-sm font-semibold text-slate-800 truncate leading-tight">{card.label}</p>
              <p className="text-sm text-slate-400 mt-0.5 font-mono">{card.key}</p>
            </div>
          </div>
          <div className="text-right shrink-0">
            <p className={`text-sm font-bold tabular-nums leading-none ${COR_TEXT[card.cor]}`}>{fmtPct(card.pct)}</p>
            <div className="mt-1 flex justify-end"><DeltaPct atual={card.valor_atual} anterior={card.valor_ant} /></div>
          </div>
        </div>

        {/* SEÇÃO 1: VENDA — Período Anterior | Período Atual | % */}
        <div className="border-t border-slate-100 pt-2.5">
          <SectionLabel tone="venda">Venda</SectionLabel>
          <div className="grid grid-cols-3 gap-2">
            <Cell label="Período Anterior" value={fmtBRL(card.valor_ant)} valueClass="text-slate-500" />
            <Cell label="Período Atual" value={fmtBRL(card.valor_atual)} />
            <Cell label="%" value={fmtPct(card.pct)} valueClass={COR_TEXT[card.cor]} />
          </div>
        </div>

        {/* SEÇÃO 2: POSITIVAÇÃO — Cl Ativos | Cl Positivado | % Posit */}
        {card.base_cli > 0 && (
          <div className="border-t border-slate-100 pt-2.5">
            <SectionLabel tone="positivacao">Positivação</SectionLabel>
            <div className="grid grid-cols-3 gap-2">
              <Cell label="Clientes Ativos" value={fmtInt(card.base_cli)} valueClass="text-slate-500" />
              <Cell label="Clientes Positivados" value={fmtInt(card.positivados)} />
              <Cell label="% Posit" value={fmtPct(card.positpct)} />
            </div>
          </div>
        )}

        {/* SEÇÃO 3: MIX MÉDIO — Realizado */}
        {card.mix > 0 && (
          <div className="border-t border-slate-100 pt-2.5">
            <SectionLabel tone="mix">Mix médio</SectionLabel>
            <div className="grid grid-cols-3 gap-2">
              <Cell label="Realizado" value={fmtNum(card.mix) + ' itens/cli'} />
            </div>
          </div>
        )}
      </div>
    </button>
  )
}

// ─── Componente principal ────────────────────────────────────────────────

export default function FarolPublicPanel() {
  const params = useParams<{ cnpj?: string; cod?: string; codRca?: string }>()

  const isRca    = !!params.codRca
  const scope    = isRca ? 'rca' : 'sup'
  const scopeCod = (isRca ? params.codRca : params.cod) || ''
  // No formato /m/:cnpj/sup/:cod o CNPJ vem em :cnpj;
  // no /m/:cod/rca/:codRca (ION: CNPJ/RCA/cod) o CNPJ vem em :cod.
  const cnpj = (params.cnpj || (isRca ? params.cod : '') || '').replace(/\D/g, '')

  const [compMode, setCompMode]   = useState('yoy')
  const [userDrill, setUserDrill] = useState<DrillStep[]>([])
  const [refAno, setRefAno]       = useState(0)
  const [refMes, setRefMes]       = useState(0)
  // Toggle de visão: V02 = "Por RCA" (default), V05 = "Por Fornecedor".
  // Só faz sentido no scope=sup (rca já está fixo num nível mais profundo).
  const [viewMode, setViewMode]   = useState<'V02' | 'V05'>('V02')

  const drillParam = JSON.stringify(userDrill)
  const { data, isLoading, error } = useQuery<CardsResponse>({
    queryKey: ['farol-public', cnpj, scope, scopeCod, viewMode, compMode, refAno, refMes, drillParam],
    queryFn: async () => {
      const p = new URLSearchParams({
        cnpj, scope, cod: scopeCod, comp_mode: compMode, view: viewMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(userDrill.length > 0 && { drill: drillParam }),
      })
      const r = await fetch(`/api/v2/farol/public/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar painel')
      return r.json()
    },
    enabled: !!cnpj && !!scopeCod,
    staleTime: 2 * 60_000, gcTime: 5 * 60_000, refetchOnWindowFocus: false,
  })

  const autoRef = useCallback((d: CardsResponse) => {
    if (refAno === 0 && d.periodo.ref_ano) {
      setRefAno(d.periodo.ref_ano)
      setRefMes(d.periodo.ref_mes)
    }
  }, [refAno])
  if (data && refAno === 0) autoRef(data)

  const baseLen     = scope === 'rca' ? 2 : 1
  const scopeStep   = data?.drill_path?.[baseLen - 1]
  const scopeLabel  = scope === 'rca' ? 'RCA' : 'Supervisor'
  const scopeNome   = scopeStep?.label || scopeCod
  const periodos    = data?.periodos ?? []

  const handleDrill = (card: CardItem) => {
    if (card.level === 'cod_prod') return
    setUserDrill(prev => [...prev, { level: card.level, value: card.key, label: card.label }])
  }
  const handleBreadcrumb = (idx: number) => {
    setUserDrill(prev => (idx < 0 ? [] : prev.slice(0, idx + 1)))
  }

  if (!cnpj || !scopeCod) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center p-6">
        <div className="bg-white border border-red-200 rounded-xl p-6 text-red-700 text-sm max-w-md text-center">
          <p className="font-semibold mb-1">Link inválido</p>
          <p>Faltam parâmetros de empresa ou código no endereço de acesso.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-slate-50 uppercase text-sm [&_*]:uppercase">
      {/* Cabeçalho do escopo */}
      <div className="bg-white border-b border-slate-200 px-4 py-3 sticky top-0 z-10">
        <p className="text-sm text-slate-400 font-medium">{scopeLabel}</p>
        <h1 className="text-sm font-bold text-slate-800 leading-tight truncate">{scopeNome}</h1>

        {/* Toggle Por RCA / Por Fornecedor — só faz sentido no escopo Supervisor */}
        {scope === 'sup' && (
          <div className="mt-2 inline-flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm">
            <button
              onClick={() => { setViewMode('V02'); setUserDrill([]) }}
              className={`px-3 py-1.5 text-sm font-medium transition-colors ${
                viewMode === 'V02' ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              Por RCA
            </button>
            <button
              onClick={() => { setViewMode('V05'); setUserDrill([]) }}
              className={`px-3 py-1.5 text-sm font-medium transition-colors border-l border-slate-200 ${
                viewMode === 'V05' ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
              }`}
            >
              Por Fornecedor
            </button>
          </div>
        )}
      </div>

      <div className="p-4">
        {/* Controles */}
        <div className="flex flex-wrap items-center gap-2 mb-4">
          <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
            {[
              { id: 'yoy', label: 'Ano a Ano' },
              { id: 'ytd', label: 'Projeção Anual' },
              { id: 'mom', label: 'Mês a Mês' },
            ].map(m => (
              <button
                key={m.id}
                onClick={() => { setCompMode(m.id); setUserDrill([]) }}
                className={`px-3 py-1.5 text-sm font-medium transition-colors ${
                  compMode === m.id ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>

          {periodos.length > 0 && (
            <select
              value={refAno > 0 ? `${refAno}-${String(refMes).padStart(2, '0')}` : ''}
              onChange={e => {
                const pp = parsePeriodo(e.target.value)
                setRefAno(pp.ano); setRefMes(pp.mes); setUserDrill([])
              }}
              className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-sm text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
            >
              {periodos.map(p => {
                const { ano, mes } = parsePeriodo(p)
                return <option key={p} value={p}>{fmtMesAno(ano, mes)}</option>
              })}
            </select>
          )}
        </div>

        {/* HEADER RESUMO — substitui KPIBar, formato planilha do esboço */}
        {data?.kpi && data.kpi.total_atual > 0 && (
          <HeaderResumo
            kpi={data.kpi}
            periodo={data.periodo}
            periodos={periodos}
            refAno={refAno}
            refMes={refMes}
            onPreset={(ano, mes) => { setRefAno(ano); setRefMes(mes); setUserDrill([]) }}
          />
        )}

        {/* Botão VOLTAR — só aparece quando há drill ativo. Mobile precisa
            de uma ação grande e clicável (Breadcrumb pequeno demais no toque). */}
        {userDrill.length > 0 && (
          <button
            onClick={() => setUserDrill(prev => prev.slice(0, -1))}
            className="w-full flex items-center justify-center gap-2 text-sm font-bold text-slate-700 bg-white border border-slate-300 rounded-lg px-4 py-3 mb-3 shadow-sm hover:bg-slate-50 active:bg-slate-100 transition-colors"
          >
            <ChevronLeft className="h-4 w-4" />
            Voltar
          </button>
        )}

        <Breadcrumb drillPath={userDrill} onNavigate={handleBreadcrumb} />

        {/* Banner do nível atual — destaca QUE tipo de dado está na tela */}
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

        {isLoading && (
          <div className="space-y-3">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="bg-white border border-slate-100 rounded-xl h-48 animate-pulse" />
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
            <p className="text-sm font-medium text-slate-500">Nenhum dado no período</p>
          </div>
        )}

        {!isLoading && !error && data && data.cards.length > 0 && (
          <div className="space-y-3">
            {data.cards.map(card => (
              <CardVendaPublic key={card.key} card={card} onClick={() => handleDrill(card)} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
