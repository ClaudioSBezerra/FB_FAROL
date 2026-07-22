import { useState, useEffect } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { PieChart, Pie, Cell, Tooltip as ReTooltip, ResponsiveContainer } from 'recharts'
import { RefreshCw, TrendingUp, TrendingDown } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────
//
// Tudo abaixo vem de GET /api/v2/farol/bi — uma única request. Antes eram 4
// (/cards ×3 + /pulso), cada uma recalculando período e positivação no servidor.

interface BiIndustria {
  label: string
  faturado: number
}

interface BiEquipe {
  key: string
  label: string
  faturado: number
  pct: number
}

interface KPI {
  total_atual: number
  total_ant: number
  total_pct: number
  total_faturado: number
  total_transmitido: number
  total_positivados: number
  total_base_cli: number
  total_positpct: number
  avg_mix: number
  verdes: number
  vermelhos: number
}

interface BiResponse {
  kpi: KPI
  industrias: BiIndustria[]   // top 8 + "Outros" já agregado no backend
  equipes: BiEquipe[]         // top 12 por faturado
  pulso: PulsoResp
  periodo: { fluxo: string; cur_label: string; ant_label: string }
  atualizado_em: string       // RFC3339 do último import concluído ('' se nenhum)
}

interface PulsoResp {
  dia_ref: string
  dia_ref_label: string
  vl_atual: number
  qt_atual: number
  vl_espelho: number
  qt_espelho: number
  espelho_label: string
  espelho_data: string
  pct: number
  pct_qt: number
  cor: 'verde' | 'amarelo' | 'vermelho'
  parcial: boolean
  sem_dado: boolean
}

// ─── Utils ────────────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v === 0) return '—'
  // Valores ABSOLUTOS (sem abreviar K/M/B) — decisão do gestor. Ex.: R$ 2.500,35
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function fmtPct(v: number) {
  if (v >= 9_999) return '>9999%'
  if (v >= 999.5) return Math.round(v) + '%'
  return v.toFixed(1) + '%'
}

function fmtNum(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 1 }) }

const MES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']

function gaugeColor(pct: number) {
  if (pct >= 100) return '#22c55e'
  if (pct >= 70)  return '#3b82f6'
  if (pct >= 50)  return '#eab308'
  return '#ef4444'
}

// ─── PulsoCard (War Room) ─────────────────────────────────────────────────────────

type Cor = 'verde' | 'amarelo' | 'vermelho'

function pulsoColor(cor: Cor): string {
  switch (cor) {
    case 'verde': return '#22c55e'
    case 'amarelo': return '#eab308'
    case 'vermelho': return '#ef4444'
  }
}

function PulsoCard({ data }: { data?: PulsoResp }) {
  if (!data || data.sem_dado) return null

  const deltaVl = data.pct - 100
  const deltaQt = data.pct_qt - 100
  const color = pulsoColor(data.cor)

  return (
    <div className="bg-slate-900 rounded-xl border border-slate-800 px-4 py-2.5 min-w-[280px]">
      <div className="flex items-center justify-between mb-2">
        <p className="text-[10px] font-bold uppercase tracking-wider text-slate-500">
          Pedidos de Ontem
        </p>
        <p className="text-[10px] text-slate-500">{data.dia_ref_label}</p>
      </div>

      <div className="flex items-center justify-between gap-4">
        {/* Valor */}
        <div className="flex-1">
          <p className="text-2xl font-black text-white tabular-nums leading-none">
            {fmtBRL(data.vl_atual)}
          </p>
          <div className="flex items-center gap-1 mt-1">
            {deltaVl >= 0 ? (
              <TrendingUp className="h-3 w-3 text-emerald-400" />
            ) : (
              <TrendingDown className="h-3 w-3 text-red-400" />
            )}
            <span className={`text-xs font-bold tabular-nums ${deltaVl >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              {deltaVl >= 0 ? '+' : ''}{deltaVl.toFixed(0)}%
            </span>
            <span className="text-[10px] text-slate-500">vs {data.espelho_label}</span>
          </div>
        </div>

        {/* Qtd pedidos */}
        <div className="text-right">
          <p className="text-xl font-black text-white tabular-nums leading-none">
            {data.qt_atual}
          </p>
          <div className="flex items-center gap-1 justify-end mt-1">
            {deltaQt >= 0 ? (
              <TrendingUp className="h-3 w-3 text-emerald-400" />
            ) : (
              <TrendingDown className="h-3 w-3 text-red-400" />
            )}
            <span className={`text-xs font-bold tabular-nums ${deltaQt >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              {deltaQt >= 0 ? '+' : ''}{deltaQt.toFixed(0)}%
            </span>
          </div>
        </div>
      </div>

      {data.parcial && (
        <div className="mt-2 pt-2 border-t border-slate-800">
          <p className="text-[9px] text-amber-400 font-medium">
            ⚠ Dado parcial — pode aumentar
          </p>
        </div>
      )}
    </div>
  )
}

// ─── BiClock ──────────────────────────────────────────────────────────────────

// O relógio marca a hora certa; a linha de baixo diz de quando é o DADO.
// Sem isso, o painel na TV mostra número de horas atrás com cara de tempo real.
const STALE_MS = 24 * 60 * 60 * 1_000

function BiClock({ atualizadoEm }: { atualizadoEm?: string }) {
  const [now, setNow] = useState(new Date())
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(t)
  }, [])

  const dado = atualizadoEm ? new Date(atualizadoEm) : null
  const valido = dado !== null && !Number.isNaN(dado.getTime())
  const velho  = valido && Date.now() - dado.getTime() > STALE_MS

  return (
    <div className="text-right">
      <p className="text-xl font-black tabular-nums text-white">
        {now.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })}
      </p>
      <p className={`text-[10px] tabular-nums ${velho ? 'text-amber-400' : 'text-slate-600'}`}>
        {valido
          ? `dados de ${dado.toLocaleString('pt-BR', {
              day: '2-digit', month: '2-digit', year: '2-digit',
              hour: '2-digit', minute: '2-digit',
            })}`
          : 'sem import registrado'}
      </p>
    </div>
  )
}

// ─── ArcGaugeDark ─────────────────────────────────────────────────────────────

function ArcGaugeDark({
  gaugeId, arcPct, centerText, label, sublabel, subvalue, prevValue, curValue, color, size = 180,
}: {
  gaugeId: string
  arcPct: number       // 0-100, controls arc fill
  centerText: string   // displayed in center
  label: string
  sublabel?: string
  subvalue?: string
  prevValue?: string   // valor anterior (comparativo) — hierarquia secundária
  curValue?: string    // valor atual faturado — hierarquia primária (grande, colorido)
  color: string
  size?: number
}) {
  const cx = size / 2, cy = size / 2
  const r = size * 0.37, sw = size * 0.065
  const total = 2 * Math.PI * r
  const arc = total * 0.75
  const filled = arc * Math.min(Math.max(arcPct, 0) / 100, 1)
  const filterId = `bi-glow-${gaugeId}`

  return (
    <div className="flex flex-col items-center">
      {/* Título acima do arco */}
      <p className="text-slate-400 uppercase tracking-widest font-bold mb-2 text-center"
        style={{ fontSize: size * 0.072 }}>
        {label}
      </p>
      <div className="relative" style={{ width: size, height: size * 0.72 }}>
        <svg width={size} height={size * 0.72} viewBox={`0 0 ${size} ${size * 0.72}`} className="overflow-visible">
          <defs>
            <filter id={filterId} x="-30%" y="-30%" width="160%" height="160%">
              <feGaussianBlur stdDeviation="4" result="blur" />
              <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
            </filter>
          </defs>
          {/* Track */}
          <circle cx={cx} cy={cy} r={r} fill="none"
            stroke="rgba(255,255,255,0.07)" strokeWidth={sw}
            strokeDasharray={`${arc} ${total - arc}`} strokeLinecap="round"
            transform={`rotate(-225 ${cx} ${cy})`} />
          {/* Fill */}
          {filled > 0.5 && (
            <circle cx={cx} cy={cy} r={r} fill="none"
              stroke={color} strokeWidth={sw}
              strokeDasharray={`${filled} ${total - filled}`} strokeLinecap="round"
              transform={`rotate(-225 ${cx} ${cy})`}
              filter={`url(#${filterId})`}
              style={{ transition: 'stroke-dasharray 1s cubic-bezier(0.4,0,0.2,1)' }}
            />
          )}
        </svg>
        {/* Percentual no centro — só o número, fonte ajustada para não vazar */}
        <div className="absolute inset-0 flex items-center justify-center overflow-hidden"
          style={{ paddingTop: size * 0.22 }}>
          <span className="font-black tabular-nums leading-none" style={{ fontSize: size * 0.155, color }}>
            {centerText}
          </span>
        </div>
      </div>
      {/* Valores apurados — espaço extra abaixo do arco */}
      <div className="text-center mt-5">
        {/* Comparativo Anterior → Atual como protagonista (grande + negrito) */}
        {curValue ? (
          <div className="flex flex-col items-center gap-1">
            {prevValue && (
              <div className="flex items-baseline justify-center gap-2">
                <span className="text-slate-500 uppercase tracking-wide font-semibold"
                  style={{ fontSize: size * 0.058 }}>
                  Anterior
                </span>
                <span className="text-slate-400 font-bold tabular-nums"
                  style={{ fontSize: size * 0.082 }}>
                  {prevValue}
                </span>
              </div>
            )}
            <div className="flex items-baseline justify-center gap-2">
              <span className="uppercase tracking-wide font-semibold"
                style={{ fontSize: size * 0.058, color }}>
                Atual
              </span>
              <span className="font-black tabular-nums" style={{ fontSize: size * 0.105, color }}>
                {curValue}
              </span>
            </div>
          </div>
        ) : (
          <>
            {sublabel && <p className="text-white font-semibold" style={{ fontSize: size * 0.088 }}>{sublabel}</p>}
            {subvalue && <p className="text-slate-500 mt-1" style={{ fontSize: size * 0.07 }}>{subvalue}</p>}
          </>
        )}
      </div>
    </div>
  )
}

// ─── IndustryDonut ────────────────────────────────────────────────────────────

const DONUT_COLORS = [
  '#6366f1', '#22c55e', '#f59e0b', '#ec4899',
  '#14b8a6', '#f97316', '#8b5cf6', '#06b6d4', '#64748b',
]

// Recebe pronto do backend: top 8 por faturado + "Outros" (cauda somada) na
// última posição — por isso a cor sai direto do índice.
function IndustryDonut({ industrias }: { industrias: BiIndustria[] }) {
  const data = industrias.map((c, i) => ({
    name: c.label, value: c.faturado, color: DONUT_COLORS[i] ?? DONUT_COLORS[8],
  }))
  const total = data.reduce((s, d) => s + d.value, 0)

  return (
    <div className="flex flex-col h-full">
      <h3 className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3 shrink-0">
        Faturado por Indústria
      </h3>
      <div className="flex flex-1 gap-4 min-h-0">
        <div className="w-[45%] shrink-0">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie data={data} cx="50%" cy="50%"
                innerRadius="52%" outerRadius="82%"
                dataKey="value" strokeWidth={0}>
                {data.map((_, i) => <Cell key={i} fill={data[i].color} />)}
              </Pie>
              <ReTooltip
                contentStyle={{ background: '#0f172a', border: '1px solid #1e293b', borderRadius: 8, fontSize: 11 }}
                formatter={(v: number | undefined) => [fmtBRL(v ?? 0), '']}
                labelStyle={{ color: '#94a3b8' }}
              />
            </PieChart>
          </ResponsiveContainer>
        </div>
        <div className="flex-1 overflow-y-auto space-y-2 min-h-0 py-1">
          {data.map((d, i) => (
            <div key={i} className="flex items-center gap-2">
              <span className="w-2.5 h-2.5 rounded-full shrink-0" style={{ background: d.color }} />
              <span className="text-[11px] text-slate-300 truncate flex-1 leading-none">{d.name}</span>
              <span className="text-[11px] text-slate-400 font-bold tabular-nums shrink-0">
                {total > 0 ? ((d.value / total) * 100).toFixed(0) + '%' : '0%'}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}

// ─── RcaRanking ───────────────────────────────────────────────────────────────

// Já chega ordenado e cortado no top 12 pelo backend.
function RcaRanking({ equipes }: { equipes: BiEquipe[] }) {
  // `?? 1` não cobria topo == 0 (início de mês, ou mês só com devolução):
  // 0/0 = NaN vira width:"NaN%" e a barra some. Valores negativos dariam
  // largura negativa. Daí o piso em 1 e o clamp de barW abaixo.
  const maxFat = Math.max(equipes[0]?.faturado ?? 0, 1)

  function barColor(pct: number) {
    if (pct >= 100) return '#22c55e'
    if (pct >= 70)  return '#eab308'
    return '#ef4444'
  }

  return (
    <div className="flex flex-col h-full">
      <h3 className="text-xs font-bold text-slate-400 uppercase tracking-widest mb-3 shrink-0">
        Ranking por Equipe
      </h3>
      <div className="flex-1 space-y-2.5 overflow-y-auto min-h-0">
        {equipes.map((c, i) => {
          const col  = barColor(c.pct)
          const barW = Math.min(Math.max((c.faturado / maxFat) * 100, 0), 100)
          return (
            <div key={c.key} className="flex items-center gap-2">
              <span className="w-5 text-[10px] font-bold text-slate-600 shrink-0 text-right">{i + 1}</span>
              <div className="flex-1 min-w-0">
                <div className="flex items-baseline justify-between mb-0.5">
                  <span className="text-xs font-semibold text-slate-200 truncate leading-none" title={c.label}>
                    {c.label.length > 18 ? c.label.slice(0, 18) + '…' : c.label}
                  </span>
                  <span className="text-xs font-black tabular-nums ml-2 shrink-0" style={{ color: col }}>
                    {fmtPct(c.pct)}
                  </span>
                </div>
                <div className="h-2 bg-slate-800 rounded-full overflow-hidden">
                  <div className="h-full rounded-full transition-all duration-700"
                    style={{ width: `${barW}%`, background: col }} />
                </div>
              </div>
              <span className="text-[10px] text-slate-500 tabular-nums shrink-0 w-14 text-right">
                {fmtBRL(c.faturado)}
              </span>
            </div>
          )
        })}
        {equipes.length === 0 && (
          <p className="text-slate-600 text-sm text-center mt-8">Sem dados</p>
        )}
      </div>
    </div>
  )
}

// ─── Hook de dados ────────────────────────────────────────────────────────────

// Polling curto porque a resposta é servida do cache do backend (TTL 10 min,
// invalidado no import): custa quase nada e encurta a janela de dado velho.
const REFETCH_MS = 5 * 60 * 1_000

type CompMode = 'ytd' | 'mtd'

// ─── FarolBI ─────────────────────────────────────────────────────────────────

async function fetchBI(compMode: CompMode, nocache = false): Promise<BiResponse> {
  const r = await fetch(`/api/v2/farol/bi?comp_mode=${compMode}${nocache ? '&nocache=1' : ''}`)
  if (!r.ok) throw new Error('Falha ao carregar o painel BI')
  return r.json()
}

export default function FarolBI() {
  const [compMode, setCompMode] = useState<CompMode>('ytd')
  const queryClient = useQueryClient()

  const { data, isLoading, isError } = useQuery<BiResponse>({
    queryKey: ['bi', compMode],
    queryFn: () => fetchBI(compMode),
    staleTime: REFETCH_MS,
    gcTime:    REFETCH_MS + 5 * 60_000,
    refetchInterval: REFETCH_MS,
    refetchOnWindowFocus: false,
  })

  // "Atualizar" tem de furar o cache do SERVIDOR de forma determinística.
  // Com refetch() + flag num ref, o React Query podia deduplicar o clique numa
  // request já em voo (montada sem nocache) — o botão parecia funcionar e
  // devolvia o mesmo dado cacheado, que é pior do que não ter botão.
  function handleRefresh() {
    fetchBI(compMode, true)
      .then(fresh => queryClient.setQueryData(['bi', compMode], fresh))
      .catch(() => queryClient.invalidateQueries({ queryKey: ['bi', compMode] }))
  }

  function handleCompModeChange(mode: CompMode) {
    if (mode !== compMode) {
      setCompMode(mode)
    }
  }

  const kpi     = data?.kpi
  const ind     = data?.industrias ?? []
  const rca     = data?.equipes ?? []
  const periodo = data?.periodo

  const mixMax = 8
  const mixPct = kpi ? Math.min((kpi.avg_mix / mixMax) * 100, 100) : 0

  return (
    <div className="-mx-4 -my-4 bg-slate-950 text-white flex flex-col select-none overflow-hidden"
      style={{ minHeight: 'calc(100vh - 88px)' }}>

      {/* Grade decorativa de fundo */}
      <div className="absolute inset-0 pointer-events-none"
        style={{
          backgroundImage: 'linear-gradient(rgba(255,255,255,.025) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.025) 1px,transparent 1px)',
          backgroundSize: '60px 60px',
        }}
      />

      {/* ── Header ────────────────────────────────────────────────────────── */}
      <header className="relative z-10 flex items-center justify-between px-8 py-4 border-b border-slate-800/60 shrink-0">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-4">
            <img src="/logo-fb.png" alt="Logo" className="h-8 w-8 rounded-lg object-cover opacity-70" />
            <div>
              <p className="text-[10px] font-bold text-slate-500 uppercase tracking-widest">Painel BI — War Room</p>
              <p className="text-sm font-semibold text-slate-200">
                {periodo
                  ? periodo.ant_label
                    ? `${periodo.cur_label} · vs ${periodo.ant_label}`
                    : periodo.cur_label
                  : 'Carregando período…'}
              </p>
            </div>
          </div>
          {/* Seletor YTD / MTD */}
          <div className="flex items-center gap-1 bg-slate-900 rounded-lg p-1 border border-slate-800">
            <button
              onClick={() => handleCompModeChange('ytd')}
              className={`px-3 py-1 rounded text-xs font-semibold transition-all ${
                compMode === 'ytd'
                  ? 'bg-blue-600 text-white shadow-lg'
                  : 'text-slate-500 hover:text-slate-300'
              }`}>
              Acumulado Ano
            </button>
            <button
              onClick={() => handleCompModeChange('mtd')}
              className={`px-3 py-1 rounded text-xs font-semibold transition-all ${
                compMode === 'mtd'
                  ? 'bg-blue-600 text-white shadow-lg'
                  : 'text-slate-500 hover:text-slate-300'
              }`}>
              Mês Atual
            </button>
          </div>
          {/* Card Pulso de Ontem */}
          <PulsoCard data={data?.pulso} />
        </div>
        <div className="flex items-center gap-6">
          <button onClick={handleRefresh}
            className="flex items-center gap-1.5 text-xs text-slate-600 hover:text-slate-300 transition-colors">
            <RefreshCw className="h-3.5 w-3.5" /> Atualizar
          </button>
          <BiClock atualizadoEm={data?.atualizado_em} />
        </div>
      </header>

      {/* ── Loading ───────────────────────────────────────────────────────── */}
      {isLoading && (
        <div className="flex-1 flex flex-col items-center justify-center gap-3">
          <div className="w-12 h-12 rounded-full border-4 border-blue-500 border-t-transparent animate-spin" />
          <p className="text-sm font-semibold text-slate-300">Carregando dados, aguarde...</p>
          <p className="text-[11px] text-slate-600">Primeiro acesso do dia pode levar alguns segundos.</p>
        </div>
      )}

      {/* Erro só toma a tela quando não há NADA para mostrar. Se já houve uma
          carga boa, o painel continua exibindo o último dado válido com uma
          faixa de aviso — numa TV, tela vermelha é pior que dado de 5 min. */}
      {isError && !isLoading && !kpi && (
        <div className="flex-1 flex items-center justify-center">
          <p className="text-red-400 text-sm">Erro ao carregar dados. Verifique a conexão.</p>
        </div>
      )}

      {isError && kpi && (
        <div className="relative z-10 px-8 py-1.5 bg-red-950/60 border-b border-red-900/60 shrink-0">
          <p className="text-[11px] text-red-300 font-semibold">
            ⚠ Falha na última atualização — exibindo os dados anteriores.
          </p>
        </div>
      )}

      {/* ── Conteúdo ──────────────────────────────────────────────────────── */}
      {!isLoading && kpi && (
        <div className="relative z-10 flex-1 grid grid-rows-[auto_1fr] p-6 gap-6"
          style={{ minHeight: 0 }}>

          {/* Linha 1: 3 gauges */}
          <div className="grid grid-cols-3 gap-6">

            {/* Gauge: Objetivo Geral */}
            <div className="bg-slate-900 rounded-2xl border border-slate-800 flex flex-col items-center justify-center py-6 px-4">
              <ArcGaugeDark
                gaugeId="obj"
                arcPct={Math.min(kpi.total_pct, 100)}
                centerText={fmtPct(kpi.total_pct)}
                label="Objetivo Geral"
                prevValue={kpi.total_ant > 0 ? fmtBRL(kpi.total_ant) : undefined}
                curValue={fmtBRL(kpi.total_faturado)}
                color={gaugeColor(kpi.total_pct)}
              />
            </div>

            {/* Gauge: Positivação */}
            <div className="bg-slate-900 rounded-2xl border border-slate-800 flex flex-col items-center justify-center py-6 px-4">
              <ArcGaugeDark
                gaugeId="pos"
                arcPct={Math.min(kpi.total_positpct, 100)}
                centerText={fmtPct(kpi.total_positpct)}
                label="Positivação"
                sublabel={`${kpi.total_positivados.toLocaleString('pt-BR')} / ${kpi.total_base_cli.toLocaleString('pt-BR')} clientes`}
                color={gaugeColor(kpi.total_positpct)}
              />
            </div>

            {/* Gauge: Mix + Farol verde/vermelho */}
            <div className="bg-slate-900 rounded-2xl border border-slate-800 flex flex-col items-center justify-center py-6 px-4">
              <ArcGaugeDark
                gaugeId="mix"
                arcPct={mixPct}
                centerText={fmtNum(kpi.avg_mix)}
                label="Mix Médio (itens)"
                sublabel={`${fmtNum(kpi.avg_mix)} itens/cliente`}
                color={gaugeColor(mixPct)}
              />
              <div className="flex gap-5 mt-3">
                <span className="flex items-center gap-1.5 text-xs font-bold text-green-400">
                  <span className="w-2 h-2 rounded-full bg-green-400 shrink-0" />
                  {kpi.verdes} verdes
                </span>
                <span className="flex items-center gap-1.5 text-xs font-bold text-red-400">
                  <span className="w-2 h-2 rounded-full bg-red-400 shrink-0" />
                  {kpi.vermelhos} vermelhos
                </span>
              </div>
            </div>
          </div>

          {/* Linha 2: Donut + Ranking */}
          <div className="grid grid-cols-2 gap-6" style={{ minHeight: 0 }}>
            <div className="bg-slate-900 rounded-2xl border border-slate-800 p-6 flex flex-col" style={{ minHeight: 0 }}>
              {ind.length > 0
                ? <IndustryDonut industrias={ind} />
                : <p className="text-slate-600 text-sm text-center my-auto">Sem dados de indústria</p>}
            </div>
            <div className="bg-slate-900 rounded-2xl border border-slate-800 p-6 flex flex-col" style={{ minHeight: 0 }}>
              {rca.length > 0
                ? <RcaRanking equipes={rca} />
                : <p className="text-slate-600 text-sm text-center my-auto">Sem dados de equipe</p>}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
