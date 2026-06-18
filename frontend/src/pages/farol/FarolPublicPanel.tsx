import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { TrendingUp, TrendingDown, Minus, ChevronLeft } from 'lucide-react'
import {
  Breadcrumb,
  parsePeriodo, fmtBRL, fmtPct, fmtNum, fmtInt,
  type CardsResponse, type CardItem, type DrillStep, type KPI,
} from './FarolV2Dashboard'
import { presetRange, PRESET_LABEL_MOBILE, PRESET_ORDER, type Preset } from '@/lib/farolPresets'
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
  kpi, periodo, hidePosit,
}: {
  kpi: KPI
  periodo: CardsResponse['periodo']
  hidePosit?: boolean
}) {
  const antLabel = periodo.ant_label || 'Anterior'
  const curLabel = periodo.cur_label || 'Atual'
  const barW     = Math.min(100, kpi.total_pct)

  return (
    <div className={`relative ${COR_BG[kpi.total_cor]} border border-slate-200/60 ${COR_RING[kpi.total_cor]} rounded-xl shadow-sm overflow-hidden mb-4`}>
      {/* Barra de atingimento no topo */}
      <div className="h-1 bg-slate-100/80">
        <div className={`h-full ${COR_BAR[kpi.total_cor]} transition-all duration-500`} style={{ width: `${barW}%` }} />
      </div>

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

        {/* SEÇÃO 2: POSITIVAÇÃO — escondida no nível Cliente/Produto */}
        {!hidePosit && (
        <div className="border-t border-slate-100 pt-2.5">
          <SectionLabel banner tone="positivacao">Positivação</SectionLabel>
          <div className="grid grid-cols-5 gap-2">
            <Cell label="Cl. Ativos" value={fmtInt(kpi.total_base_cli)} valueClass="text-slate-500" />
            <Cell label="Posit. Ant" value={fmtInt(kpi.total_positivados_ant)} valueClass="text-slate-500" />
            <Cell label="% Ant" value={fmtPct(kpi.total_positpct_ant)} valueClass="text-slate-500" />
            <Cell label="Posit. Atual" value={fmtInt(kpi.total_positivados)} />
            <Cell label="% Atual" value={fmtPct(kpi.total_positpct)} />
          </div>
        </div>
        )}

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
              <p className="text-sm font-semibold text-slate-800 truncate leading-tight" title={`${card.key} - ${card.label}`}>
                {card.key} - {card.label}
              </p>
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

        {/* SEÇÃO 2: POSITIVAÇÃO — Cl Ativos | Posit. Ant | Posit. Atual | % Posit.
            Escondida no nível Cliente/Produto (não faz sentido). */}
        {card.base_cli > 0 && card.level !== 'cod_cli' && card.level !== 'cod_prod' && (
          <div className="border-t border-slate-100 pt-2.5">
            <SectionLabel tone="positivacao">Positivação</SectionLabel>
            <div className="grid grid-cols-5 gap-2">
              <Cell label="Cl. Ativos" value={fmtInt(card.base_cli)} valueClass="text-slate-500" />
              <Cell label="Posit. Ant" value={fmtInt(card.positivados_ant)} valueClass="text-slate-500" />
              <Cell label="% Ant" value={fmtPct(card.positpct_ant)} valueClass="text-slate-500" />
              <Cell label="Posit. Atual" value={fmtInt(card.positivados)} />
              <Cell label="% Atual" value={fmtPct(card.positpct)} />
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

  const [userDrill, setUserDrill] = useState<DrillStep[]>([])
  // Mesma lógica de presets do painel executivo (intervalos explícitos coerentes).
  const [activePreset, setActivePreset] = useState<Preset>('yoy')
  const [refInicio, setRefInicio]   = useState('')
  const [refFim, setRefFim]         = useState('')
  const [compInicio, setCompInicio] = useState('')
  const [compFim, setCompFim]       = useState('')
  // Toggle de visão: V02 = "Por RCA" (default), V05 = "Por Fornecedor".
  // Só faz sentido no scope=sup (rca já está fixo num nível mais profundo).
  const [viewMode, setViewMode]   = useState<'V02' | 'V05'>('V02')

  const drillParam = JSON.stringify(userDrill)
  const { data, isLoading, error } = useQuery<CardsResponse>({
    queryKey: ['farol-public', cnpj, scope, scopeCod, viewMode, refInicio, refFim, compInicio, compFim, drillParam],
    queryFn: async () => {
      const p = new URLSearchParams({ cnpj, scope, cod: scopeCod, view: viewMode })
      if (refInicio && refFim) { p.set('ref_inicio', refInicio); p.set('ref_fim', refFim) }
      if (compInicio && compFim) { p.set('comp_inicio', compInicio); p.set('comp_fim', compFim) }
      if (userDrill.length > 0) p.set('drill', drillParam)
      const r = await fetch(`/api/v2/farol/public/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar painel')
      return r.json()
    },
    enabled: !!cnpj && !!scopeCod,
    staleTime: 2 * 60_000, gcTime: 5 * 60_000, refetchOnWindowFocus: false,
  })

  // Ao carregar, aplica o preset default (Último mês YoY) usando o último mês
  // com dados (periodos[0], ordem DESC do backend) como âncora.
  const periodos = data?.periodos ?? []
  if (refInicio === '' && periodos.length > 0) {
    const last = parsePeriodo(periodos[0])
    const r = presetRange('yoy', last)
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }

  const applyPreset = (p: Preset) => {
    setActivePreset(p)
    const last = periodos.length > 0 ? parsePeriodo(periodos[0]) : undefined
    const r = presetRange(p, last)
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
    setUserDrill([])
  }

  const baseLen     = scope === 'rca' ? 2 : 1
  const scopeStep   = data?.drill_path?.[baseLen - 1]
  const scopeLabel  = scope === 'rca' ? 'RCA' : 'Supervisor'
  const scopeNome   = scopeStep?.label || scopeCod

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
        {/* Controles — mesmos presets do painel executivo (intervalos coerentes) */}
        <div className="flex flex-wrap items-center gap-1.5 mb-4">
          {PRESET_ORDER.map(p => (
            <button
              key={p}
              onClick={() => applyPreset(p)}
              className={`px-2.5 py-1.5 text-sm font-medium rounded-lg border transition-colors ${
                activePreset === p
                  ? 'bg-slate-700 text-white border-slate-700'
                  : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-50'
              }`}
            >
              {PRESET_LABEL_MOBILE[p]}
            </button>
          ))}
        </div>

        {/* HEADER RESUMO — sempre visível (mesmo zerado) para o supervisor ter o
            totalizador do período, com a carteira e o comparativo. */}
        {data?.kpi && (
          <HeaderResumo
            kpi={data.kpi}
            periodo={data.periodo}
            hidePosit={data.cards[0]?.level === 'cod_cli' || data.cards[0]?.level === 'cod_prod'}
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

        {/* Período sem vendas (ex: mês atual ainda sem faturamento): não faz
            sentido listar todos os RCAs zerados — mostra só o aviso. */}
        {(() => {
          const semVendas = !!data?.kpi && data.kpi.total_atual === 0

          if (isLoading) {
            return (
              <div className="space-y-3">
                {[...Array(4)].map((_, i) => (
                  <div key={i} className="bg-white border border-slate-100 rounded-xl h-48 animate-pulse" />
                ))}
              </div>
            )
          }
          if (error) {
            return (
              <div className="bg-red-50 border border-red-200 rounded-xl p-6 text-red-700 text-sm">
                <p className="font-semibold mb-1">Erro ao carregar painel</p>
                <p>{(error as Error).message}</p>
              </div>
            )
          }
          if (!data) return null
          if (semVendas || data.cards.length === 0) {
            return (
              <div className="bg-white border border-dashed border-slate-200 rounded-xl p-12 text-center text-slate-400">
                <p className="text-sm font-bold mb-3">📊</p>
                <p className="text-sm font-medium text-slate-500">Sem vendas registradas neste período</p>
              </div>
            )
          }
          return (
            <>
              {/* Banner do nível atual — destaca QUE tipo de dado está na tela */}
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

              <div className="space-y-3">
                {data.cards.map(card => (
                  <CardVendaPublic key={card.key} card={card} onClick={() => handleDrill(card)} />
                ))}
              </div>
            </>
          )
        })()}
      </div>
    </div>
  )
}
