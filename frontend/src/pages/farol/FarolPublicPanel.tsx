import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import { TrendingUp, TrendingDown, Minus, ChevronLeft, ChevronRight } from 'lucide-react'
import {
  Breadcrumb,
  parsePeriodo, fmtBRL, fmtPct, fmtNum, fmtInt,
  type CardsResponse, type CardItem, type DrillStep, type KPI,
} from './FarolV2Dashboard'
import { presetRange, PRESET_LABEL_MOBILE, PRESET_ORDER, type Preset } from '@/lib/farolPresets'
import type { Cor } from '@/components/farol/Semaforo'
import { useSortedCards } from '@/components/farol/SortToggle'

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

// Cor binária pra % de positivação (não vem direta do tipo KPI compartilhado;
// derivamos local pra evitar acoplamento adicional). ≥100% verde, senão vermelho.
function corPosit(pct: number): Cor {
  return pct >= 100 ? 'verde' : 'vermelho'
}

// ─── Sub-componentes pequenos ────────────────────────────────────────────

// Dot maior pra ser visível em telas de RCA/Supervisor (público 60+).
function StatusDot({ cor }: { cor: Cor }) {
  return (
    <span className="relative flex h-4 w-4 shrink-0">
      {cor === 'vermelho' && (
        <span className="absolute inline-flex h-full w-full rounded-full opacity-60 bg-red-400 animate-ping" />
      )}
      <span className={`relative inline-flex rounded-full h-4 w-4 ${COR_DOT[cor]}`} />
    </span>
  )
}

// DeltaPct com ícone e texto maiores. Cores saturadas pra contraste.
function DeltaPct({ atual, anterior }: { atual: number; anterior: number }) {
  const delta = anterior > 0 ? ((atual - anterior) / anterior) * 100 : 0
  const up = delta > 0.5
  const down = delta < -0.5
  if (anterior <= 0) return null
  if (!up && !down) {
    return (
      <span className="inline-flex items-center gap-1 text-base font-semibold text-slate-500 tabular-nums">
        <Minus className="h-4 w-4" strokeWidth={3} />
        estável
      </span>
    )
  }
  return (
    <span className={`inline-flex items-center gap-1 text-base font-bold tabular-nums ${
      up ? 'text-emerald-700' : 'text-red-700'
    }`}>
      {up ? <TrendingUp className="h-4 w-4" strokeWidth={3} /> : <TrendingDown className="h-4 w-4" strokeWidth={3} />}
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
      <p className={`text-base uppercase tracking-wider font-extrabold mb-2 bg-[#1e293b] ${SECTION_COLOR[tone]} px-2.5 py-1.5 rounded`}>{children}</p>
    )
  }
  return (
    <p className="text-base uppercase tracking-wider font-extrabold text-slate-700 mb-2">{children}</p>
  )
}

// Cell: label CAPS + valor. Tamanhos aumentados pra leitura em mobile 60+;
// label slate-600 (não 400) pra contraste. Sem truncate — overflow-wrap quebra
// se for muito longo, mas mantém o número inteiro visível.
function Cell({ label, value, valueClass = 'text-slate-900', large = false }: {
  label: string; value: React.ReactNode; valueClass?: string; large?: boolean
}) {
  return (
    <div className="min-w-0">
      <p className="text-sm uppercase tracking-wide text-slate-600 font-semibold leading-tight">{label}</p>
      <p className={`tabular-nums leading-tight mt-1 ${large ? 'text-xl font-extrabold' : 'text-base font-bold'} ${valueClass}`}>
        {value}
      </p>
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
    <div className={`relative ${COR_BG[kpi.total_cor]} border-2 border-slate-300 ${COR_RING[kpi.total_cor]} rounded-xl shadow-md overflow-hidden mb-5`}>
      {/* Barra de atingimento no topo — mais grossa pra ser percebida */}
      <div className="h-2 bg-slate-200">
        <div className={`h-full ${COR_BAR[kpi.total_cor]} transition-all duration-500`} style={{ width: `${barW}%` }} />
      </div>

      <div className="p-5 space-y-4">
        {/* SEÇÃO 1: VENDA — hero do valor ATUAL + % grande */}
        <div>
          <SectionLabel banner tone="venda">Venda</SectionLabel>

          {/* HERO — valor atual GRANDE + % colorido GRANDE */}
          <div className="flex items-baseline justify-between gap-3 mb-3">
            <div className="min-w-0">
              <p className="text-sm uppercase tracking-wide text-slate-600 font-semibold leading-tight">{curLabel}</p>
              {/* Sem whitespace-nowrap: acima de ~R$ 1 bi o valor não cabe numa
                  linha só no clamp máximo, e nowrap sem overflow-hidden só
                  faz o texto vazar por cima do % ao lado. break-words deixa
                  quebrar em 2 linhas quando precisa, sem cortar dígito. */}
              <p className="text-[clamp(1.25rem,7vw,2.25rem)] font-black tabular-nums text-slate-900 leading-tight mt-1 break-words">
                {fmtBRL(kpi.total_atual)}
              </p>
            </div>
            <div className="text-right shrink-0">
              <p className={`text-3xl sm:text-4xl font-black tabular-nums leading-tight ${COR_TEXT[kpi.total_cor]}`}>
                {fmtPct(kpi.total_pct)}
              </p>
              <div className="mt-1 flex justify-end"><DeltaPct atual={kpi.total_atual} anterior={kpi.total_ant} /></div>
            </div>
          </div>

          {/* CONTEXTO — período anterior, discreto mas legível */}
          <div className="text-base text-slate-600 tabular-nums break-words">
            <span className="font-semibold uppercase tracking-wide text-slate-500">{antLabel}:</span>{' '}
            <span className="font-bold text-slate-700">{fmtBRL(kpi.total_ant)}</span>
          </div>
        </div>

        {/* SEÇÃO 2: POSITIVAÇÃO — escondida no nível Cliente/Produto */}
        {!hidePosit && (
        <div className="border-t-2 border-slate-200 pt-3">
          <SectionLabel banner tone="positivacao">Positivação</SectionLabel>
          <div className="grid grid-cols-3 gap-3">
            <Cell label="Cl. Ativos" value={fmtInt(kpi.total_base_cli)} />
            <Cell label="Posit. Ant." value={
              <>{fmtInt(kpi.total_positivados_ant)}<span className="block text-sm font-bold text-slate-500 leading-tight mt-0.5">{fmtPct(kpi.total_positpct_ant)}</span></>
            } />
            <Cell label="Posit. Atual" valueClass={COR_TEXT[corPosit(kpi.total_positpct)]} value={
              <>{fmtInt(kpi.total_positivados)}<span className={`block text-sm font-bold leading-tight mt-0.5 ${COR_TEXT[corPosit(kpi.total_positpct)]}`}>{fmtPct(kpi.total_positpct)}</span></>
            } />
          </div>
        </div>
        )}

        {/* SEÇÃO 3: MIX MÉDIO */}
        <div className="border-t-2 border-slate-200 pt-3">
          <SectionLabel banner tone="mix">Mix médio</SectionLabel>
          <p className="text-2xl font-extrabold text-slate-900 tabular-nums">
            {fmtNum(kpi.avg_mix)} <span className="text-base font-semibold text-slate-600">itens/cli</span>
          </p>
        </div>
      </div>
    </div>
  )
}

// ─── CardVendaPublic — card individual no formato do esboço ───────────────

function CardVendaPublic({ card, onClick }: { card: CardItem; onClick: () => void }) {
  const barW = Math.min(100, card.pct)
  // Cliente/Produto é o nível folha — não tem drill, esconde chevron.
  const isLeaf = card.level === 'cod_prod'
  return (
    <button
      onClick={onClick}
      disabled={isLeaf}
      className={`group relative ${COR_BG[card.cor]} border-2 border-slate-300 ${COR_RING[card.cor]} rounded-xl shadow-sm hover:shadow-md active:scale-[0.99] transition-all duration-150 text-left w-full overflow-hidden ${isLeaf ? '' : 'cursor-pointer'}`}
    >
      {/* Barra de progresso no topo — mais grossa, melhor visibilidade. */}
      <div className="h-2 bg-slate-200">
        <div className={`h-full ${COR_BAR[card.cor]} transition-all duration-500`} style={{ width: `${barW}%` }} />
      </div>

      <div className="p-5 space-y-4">
        {/* HEADER — nome (em 2 linhas se necessário) + chevron de drill */}
        <div className="flex items-start gap-3">
          <span className="mt-1.5"><StatusDot cor={card.cor} /></span>
          <p className="flex-1 text-lg font-bold text-slate-900 leading-snug min-w-0 break-words" title={`${card.key} - ${card.label}`}>
            {card.key} - {card.label}
          </p>
          {!isLeaf && (
            <ChevronRight className="h-6 w-6 text-slate-400 shrink-0 mt-1.5" strokeWidth={2.5} />
          )}
        </div>

        {/* SEÇÃO 1: VENDA — HERO do valor atual + % grande + comparativo abaixo */}
        <div className="border-t-2 border-slate-200 pt-3">
          <SectionLabel tone="venda">Venda</SectionLabel>

          <div className="flex items-baseline justify-between gap-3 mb-2">
            <div className="min-w-0">
              <p className="text-sm uppercase tracking-wide text-slate-600 font-semibold leading-tight">Atual</p>
              <p className="text-[clamp(1.1rem,6vw,1.875rem)] font-black tabular-nums text-slate-900 leading-tight mt-1 break-words">
                {fmtBRL(card.valor_atual)}
              </p>
            </div>
            <div className="text-right shrink-0">
              <p className={`text-2xl sm:text-3xl font-black tabular-nums leading-tight ${COR_TEXT[card.cor]}`}>
                {fmtPct(card.pct)}
              </p>
              <div className="mt-0.5 flex justify-end"><DeltaPct atual={card.valor_atual} anterior={card.valor_ant} /></div>
            </div>
          </div>

          {/* Anterior como contexto discreto (mas legível) */}
          <div className="text-base text-slate-600 tabular-nums break-words">
            <span className="font-semibold uppercase tracking-wide text-slate-500">Anterior:</span>{' '}
            <span className="font-bold text-slate-700">{fmtBRL(card.valor_ant)}</span>
          </div>
        </div>

        {/* SEÇÃO 2: POSITIVAÇÃO — Cl Ativos | Posit. Ant | Posit. Atual.
            Escondida no nível Cliente/Produto (não faz sentido). */}
        {card.base_cli > 0 && card.level !== 'cod_cli' && card.level !== 'cod_prod' && (
          <div className="border-t-2 border-slate-200 pt-3">
            <SectionLabel tone="positivacao">Positivação</SectionLabel>
            <div className="grid grid-cols-3 gap-3">
              <Cell label="Cl. Ativos" value={fmtInt(card.base_cli)} />
              <Cell label="Posit. Ant." value={
                <>{fmtInt(card.positivados_ant)}<span className="block text-sm font-bold text-slate-500 leading-tight mt-0.5">{fmtPct(card.positpct_ant)}</span></>
              } />
              <Cell label="Posit. Atual" valueClass={COR_TEXT[corPosit(card.positpct)]} value={
                <>{fmtInt(card.positivados)}<span className={`block text-sm font-bold leading-tight mt-0.5 ${COR_TEXT[corPosit(card.positpct)]}`}>{fmtPct(card.positpct)}</span></>
              } />
            </div>
          </div>
        )}

        {/* SEÇÃO 3: MIX MÉDIO */}
        {card.mix > 0 && (
          <div className="border-t-2 border-slate-200 pt-3">
            <SectionLabel tone="mix">Mix médio</SectionLabel>
            <p className="text-xl font-extrabold text-slate-900 tabular-nums">
              {fmtNum(card.mix)} <span className="text-base font-semibold text-slate-600">itens/cli</span>
            </p>
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
  const [viewMode, setViewMode]   = useState<'V02' | 'V05'>(() => {
    try {
      const stored = localStorage.getItem('farol.public.view')
      if (stored === 'V02' || stored === 'V05') return stored
    } catch { /* indisponível — usa default */ }
    return 'V02'
  })
  // Toggle de fluxo: Faturado (NFs emitidas) vs Transmitido (pedidos enviados
  // pelo RCA, ainda em rota até virar NF). Útil pro supervisor acompanhar
  // tanto a venda concretizada quanto o que está em curso.
  const [fluxo, setFluxoState] = useState<'faturado' | 'transmitido'>(() => {
    try {
      const stored = localStorage.getItem('farol.public.fluxo')
      if (stored === 'faturado' || stored === 'transmitido') return stored
    } catch { /* indisponível — usa default */ }
    return 'faturado'
  })
  const setFluxo = (f: 'faturado' | 'transmitido') => {
    setFluxoState(f); setUserDrill([])
    try { localStorage.setItem('farol.public.fluxo', f) } catch { /* ignora */ }
  }
  const setViewModePersist = (v: 'V02' | 'V05') => {
    setViewMode(v); setUserDrill([])
    try { localStorage.setItem('farol.public.view', v) } catch { /* ignora */ }
  }

  const drillParam = JSON.stringify(userDrill)
  const { data, isLoading, error } = useQuery<CardsResponse>({
    queryKey: ['farol-public', cnpj, scope, scopeCod, viewMode, fluxo, refInicio, refFim, compInicio, compFim, drillParam],
    queryFn: async () => {
      const p = new URLSearchParams({ cnpj, scope, cod: scopeCod, view: viewMode, fluxo })
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

  // Cards ordenados por venda atual desc. Sem toggle UI — o painel público
  // (RCA em campo) prioriza navegação rápida, não análise comparativa.
  const { sorted: sortedCards } =
    useSortedCards(data?.cards ?? [], 'farol.sort.public', { field: 'valor', direction: 'desc' })

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
    // Public panel usado por SUPV/RCA em mobile, público 60+. Fontes generosas,
    // sem [&_*]:uppercase global (uppercase é aplicado apenas em labels via classe).
    <div className="min-h-screen bg-slate-50 text-base">
      {/* Cabeçalho do escopo — sticky, mais alto pra acomodar os 2 toggles */}
      <div className="bg-white border-b-2 border-slate-300 px-4 py-3 sticky top-0 z-10 shadow-sm">
        <p className="text-sm uppercase tracking-wider text-slate-500 font-semibold">{scopeLabel}</p>
        <h1 className="text-lg sm:text-xl font-extrabold text-slate-900 leading-tight">{scopeNome}</h1>

        {/* Toggles de visão. Botões altos (44px+ pra toque) e fontes legíveis. */}
        <div className="mt-3 flex flex-wrap items-center gap-2">
          {/* Fluxo: Faturado (NFs) × Transmitido (pedidos em rota). */}
          <div className="inline-flex rounded-lg border-2 border-slate-300 overflow-hidden bg-white shadow-sm">
            <button
              onClick={() => setFluxo('faturado')}
              className={`px-4 py-2.5 text-base font-bold uppercase tracking-wide transition-colors ${
                fluxo === 'faturado' ? 'bg-slate-800 text-white' : 'text-slate-700 hover:bg-slate-50'
              }`}
            >
              Faturado
            </button>
            <button
              onClick={() => setFluxo('transmitido')}
              className={`px-4 py-2.5 text-base font-bold uppercase tracking-wide transition-colors border-l-2 border-slate-300 ${
                fluxo === 'transmitido' ? 'bg-emerald-700 text-white' : 'text-slate-700 hover:bg-slate-50'
              }`}
            >
              Transmitido
            </button>
          </div>

          {/* Por RCA / Por Fornecedor — só no escopo Supervisor. */}
          {scope === 'sup' && (
            <div className="inline-flex rounded-lg border-2 border-slate-300 overflow-hidden bg-white shadow-sm">
              <button
                onClick={() => setViewModePersist('V02')}
                className={`px-4 py-2.5 text-base font-bold uppercase tracking-wide transition-colors ${
                  viewMode === 'V02' ? 'bg-slate-800 text-white' : 'text-slate-700 hover:bg-slate-50'
                }`}
              >
                Por RCA
              </button>
              <button
                onClick={() => setViewModePersist('V05')}
                className={`px-4 py-2.5 text-base font-bold uppercase tracking-wide transition-colors border-l-2 border-slate-300 ${
                  viewMode === 'V05' ? 'bg-slate-800 text-white' : 'text-slate-700 hover:bg-slate-50'
                }`}
              >
                Por Fornec.
              </button>
            </div>
          )}
        </div>
      </div>

      <div className="p-4">
        {/* Presets de período: scroll horizontal em vez de wrap (mobile 60+
            cansa de buscar botão em múltiplas linhas). -mx-4 + px-4 fingem
            "sangrar" pra borda da tela e dão dica visual de "tem mais → "). */}
        <div className="mb-4 -mx-4 px-4 overflow-x-auto scrollbar-hide">
          <div className="flex items-center gap-2 w-max">
            {PRESET_ORDER.map(p => (
              <button
                key={p}
                onClick={() => applyPreset(p)}
                className={`shrink-0 px-4 py-3 text-base font-bold uppercase tracking-wide rounded-lg border-2 transition-colors ${
                  activePreset === p
                    ? 'bg-slate-800 text-white border-slate-800'
                    : 'bg-white text-slate-700 border-slate-300 hover:bg-slate-50'
                }`}
              >
                {PRESET_LABEL_MOBILE[p]}
              </button>
            ))}
          </div>
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
                {sortedCards.map(card => (
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
