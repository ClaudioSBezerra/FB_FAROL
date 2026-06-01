import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { TrendingUp, TrendingDown, Minus, Users, UserCheck, UserX, Package, ChevronLeft, Target, Zap } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

// ─── Tipos ────────────────────────────────────────────────────────────────────

interface MktCard {
  key: string
  label: string
  fornec?: string
  nome_fornec?: string
  qt_clientes: number
  qt_cli_ant: number
  delta_pct: number
  pvenda: number
  faturado: number
  transmitido: number
  penetr_pct: number
  mix: number
  positivado: boolean
}

interface MktKPI {
  total_base_cli: number
  total_ativos: number
  total_inativos: number
  taxa_positivacao: number
  taxa_positivacao_ant: number
  delta_positivacao: number
  avg_mix: number
  total_pvenda: number
  total_faturado: number
  total_transmitido: number
}

interface ClienteInativo {
  key: string
  label: string
  pvenda: number
  transmitido: number
}

interface MktResponse {
  cards: MktCard[]
  kpi: MktKPI
  clientes_inativos: ClienteInativo[]
  periodo: {
    ref_ano: number; ref_mes: number
    label: string; comp_mode: string
    cur_label: string; ant_label: string
  }
  periodos: string[]
  view: string
}

// ─── Utilitários ──────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v >= 1_000_000_000) return 'R$ ' + (v / 1_000_000_000).toFixed(2).replace('.', ',') + 'B'
  if (v >= 1_000_000)     return 'R$ ' + (v / 1_000_000).toFixed(1).replace('.', ',') + 'M'
  if (v >= 1_000)         return 'R$ ' + (v / 1_000).toFixed(0).replace(/\B(?=(\d{3})+(?!\d))/g, '.') + 'K'
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0 })
}
function fmtPct(v: number) { return v.toFixed(1) + '%' }
function fmtNum(v: number) { return v.toLocaleString('pt-BR', { maximumFractionDigits: 1 }) }

function parsePeriodo(s: string) {
  const [y, m] = s.split('-')
  return { ano: +y, mes: +m }
}
const MES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']
function fmtMesAno(ano: number, mes: number) { return `${MES[mes] ?? mes}/${String(ano).slice(2)}` }

// Cor de penetração: verde ≥ 50%, amarelo ≥ 20%, vermelho < 20%
function corPenetr(pct: number): { bar: string; text: string } {
  if (pct >= 50) return { bar: 'bg-emerald-500', text: 'text-emerald-600' }
  if (pct >= 20) return { bar: 'bg-amber-400',   text: 'text-amber-600'  }
  return           { bar: 'bg-red-500',     text: 'text-red-600'    }
}

const MODE_DESC: Record<string, string> = {
  yoy: 'Compara o mesmo mês do ano atual com o mesmo mês do ano anterior.',
  ytd: 'Extrapola o acumulado do ano atual para projetar o total anual.',
  mom: 'Compara o mês atual com o mês imediatamente anterior.',
}

// ─── Tipos detalhe de produto ─────────────────────────────────────────────────

interface ProdDetalheKPI {
  total_base: number
  total_compradores: number
  total_oportunidades: number
  penetr_pct: number
  total_faturado: number
  qt_cli_ant: number
  delta_pct: number
  potencial_estimado: number
}
interface ProdClienteItem {
  key: string; label: string
  faturado: number; transmitido: number
  nome_sup: string; nome_rca: string
}
interface ProdOportunidade {
  key: string; label: string
  faturado_total: number
  nome_rca: string; nome_sup: string
  n_outros_prod: number
}
interface ProdDetalheResponse {
  cod_prod: string; nome_prod: string; nome_fornec: string
  kpi: ProdDetalheKPI
  compradores: ProdClienteItem[]
  oportunidades: ProdOportunidade[]
}

// ─── Tipos detalhe de cliente ─────────────────────────────────────────────────

interface CliDetalheKPI {
  total_produtos: number
  total_oportunidades: number
  total_faturado: number
  total_transmitido: number
  nome_rca: string
  nome_sup: string
  potencial_estimado: number
}
interface CliProdutoItem {
  key: string; label: string; nome_fornec: string
  faturado: number; transmitido: number
}
interface CliOportunidade {
  key: string; label: string; nome_fornec: string
  qt_outros_clientes: number; ticket_medio: number; penetr_pct: number
}
interface CliDetalheResponse {
  cod_cli: string; nome_cli: string
  kpi: CliDetalheKPI
  comprados: CliProdutoItem[]
  oportunidades: CliOportunidade[]
}

// ─── Hook de dados ─────────────────────────────────────────────────────────────

function useMktCards(view: string, compMode: string, refAno: number, refMes: number, compAno: number, compMes: number) {
  return useQuery<MktResponse>({
    queryKey: ['marketing-cards', view, compMode, refAno, refMes, compAno, compMes],
    queryFn: async () => {
      const p = new URLSearchParams({
        view, comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(compAno > 0 && compMes > 0 && { comp_ano: String(compAno), comp_mes: String(compMes) }),
      })
      const r = await fetch(`/api/v2/marketing/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dados de Marketing')
      return r.json()
    },
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

function useProdDetalhe(
  codProd: string, compMode: string, refAno: number, refMes: number, compAno: number, compMes: number
) {
  return useQuery<ProdDetalheResponse>({
    queryKey: ['mkt-prod-detalhe', codProd, compMode, refAno, refMes],
    queryFn: async () => {
      const p = new URLSearchParams({
        cod_prod: codProd, comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(compAno > 0 && compMes > 0 && { comp_ano: String(compAno), comp_mes: String(compMes) }),
      })
      const r = await fetch(`/api/v2/marketing/produto-detalhe?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar detalhe do produto')
      return r.json()
    },
    enabled: codProd !== '',
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// ─── Produto Detalhe ──────────────────────────────────────────────────────────

function ProdutoDetalhe({
  codProd, nomeProd, nomeFornec, compMode, refAno, refMes, compAno, compMes, onVoltar,
}: {
  codProd: string; nomeProd: string; nomeFornec: string
  compMode: string; refAno: number; refMes: number; compAno: number; compMes: number
  onVoltar: () => void
}) {
  const { data, isLoading, error } = useProdDetalhe(codProd, compMode, refAno, refMes, compAno, compMes)

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 mb-5">
        <button onClick={onVoltar}
          className="flex items-center gap-1.5 text-sm text-slate-500 hover:text-primary transition-colors font-medium">
          <ChevronLeft className="h-4 w-4" /> Voltar
        </button>
        <span className="text-slate-300">/</span>
        <span className="text-sm font-semibold text-slate-700 truncate">{nomeProd || codProd}</span>
        {nomeFornec && <span className="text-xs text-slate-400">· {nomeFornec}</span>}
      </div>

      {isLoading && (
        <div className="space-y-4">
          <div className="bg-white rounded-2xl h-24 animate-pulse border border-slate-100" />
          <div className="grid grid-cols-2 gap-4">
            <div className="bg-white rounded-2xl h-80 animate-pulse border border-slate-100" />
            <div className="bg-white rounded-2xl h-80 animate-pulse border border-slate-100" />
          </div>
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-6 text-red-700 text-sm">
          {(error as Error).message}
        </div>
      )}

      {data && (
        <>
          {/* KPI strip */}
          <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-5 gap-3 mb-6">
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Compradores</p>
              <p className="text-2xl font-black text-slate-800 tabular-nums">{data.kpi.total_compradores}</p>
              <p className="text-[11px] text-slate-500 mt-0.5">{fmtPct(data.kpi.penetr_pct)} da base</p>
            </div>
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Oportunidades</p>
              <p className="text-2xl font-black text-amber-600 tabular-nums">{data.kpi.total_oportunidades}</p>
              <p className="text-[11px] text-slate-500 mt-0.5">{data.kpi.total_base - data.kpi.total_compradores - data.kpi.total_oportunidades > 0
                ? `+${data.kpi.total_base - data.kpi.total_compradores - data.kpi.total_oportunidades} sem dados`
                : 'clientes ativos sem este produto'}</p>
            </div>
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Faturado</p>
              <p className="text-2xl font-black text-slate-800 tabular-nums">{fmtBRL(data.kpi.total_faturado)}</p>
              <p className="text-[11px] text-slate-500 mt-0.5">neste produto</p>
            </div>
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Variação clientes</p>
              <p className={`text-2xl font-black tabular-nums ${data.kpi.delta_pct >= 0 ? 'text-emerald-600' : 'text-red-600'}`}>
                {data.kpi.delta_pct >= 0 ? '+' : ''}{data.kpi.delta_pct.toFixed(1)}%
              </p>
              <p className="text-[11px] text-slate-500 mt-0.5">vs {data.kpi.qt_cli_ant} período ant.</p>
            </div>
            <div className="bg-amber-50 rounded-xl border border-amber-200 shadow-sm p-4 hidden lg:block">
              <p className="text-[11px] text-amber-600 uppercase tracking-wider mb-1 flex items-center gap-1">
                <Zap className="h-3 w-3" /> Potencial estimado
              </p>
              <p className="text-2xl font-black text-amber-700 tabular-nums">{fmtBRL(data.kpi.potencial_estimado)}</p>
              <p className="text-[11px] text-amber-600 mt-0.5">faturamento total das oportunidades</p>
            </div>
          </div>

          {/* Duas colunas */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
            {/* Compradores */}
            <div className="bg-white rounded-2xl shadow-sm border border-emerald-100 overflow-hidden">
              <div className="flex items-center gap-2 px-5 py-4 border-b border-emerald-50 bg-emerald-50/50">
                <UserCheck className="h-4 w-4 text-emerald-600" />
                <div>
                  <h3 className="text-sm font-bold text-emerald-800">Já compram este produto</h3>
                  <p className="text-xs text-emerald-600">{data.compradores.length} clientes · ordenado por faturado</p>
                </div>
              </div>
              <div className="divide-y divide-slate-50 max-h-[480px] overflow-y-auto">
                {data.compradores.map((c, i) => (
                  <div key={c.key} className="flex items-center gap-3 px-5 py-3 hover:bg-emerald-50/30 transition-colors">
                    <span className="w-5 text-xs font-bold text-slate-300 flex-shrink-0">{i + 1}</span>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-semibold text-slate-800 truncate">{c.label}</p>
                      <p className="text-[10px] text-slate-400 truncate">
                        {c.nome_rca && `RCA: ${c.nome_rca}`}
                        {c.nome_sup && ` · Sup: ${c.nome_sup}`}
                      </p>
                    </div>
                    <div className="text-right flex-shrink-0">
                      <p className="text-sm font-bold text-emerald-700 tabular-nums">{fmtBRL(c.faturado)}</p>
                      {c.transmitido > 0 && (
                        <p className="text-[10px] text-slate-400 tabular-nums">{fmtBRL(c.transmitido)} transm.</p>
                      )}
                    </div>
                  </div>
                ))}
                {data.compradores.length === 0 && (
                  <p className="px-5 py-8 text-sm text-slate-400 text-center">Nenhum comprador no período</p>
                )}
              </div>
            </div>

            {/* Oportunidades */}
            <div className="bg-white rounded-2xl shadow-sm border border-amber-100 overflow-hidden">
              <div className="flex items-center gap-2 px-5 py-4 border-b border-amber-50 bg-amber-50/50">
                <Target className="h-4 w-4 text-amber-600" />
                <div>
                  <h3 className="text-sm font-bold text-amber-800">Oportunidades de ativação</h3>
                  <p className="text-xs text-amber-600">
                    {data.oportunidades.length} clientes ativos que <strong>não compram</strong> este produto · ordenado por potencial
                  </p>
                </div>
              </div>
              <div className="divide-y divide-slate-50 max-h-[480px] overflow-y-auto">
                {data.oportunidades.map((o, i) => (
                  <div key={o.key} className="flex items-center gap-3 px-5 py-3 hover:bg-amber-50/30 transition-colors">
                    <span className="w-5 text-xs font-bold text-slate-300 flex-shrink-0">{i + 1}</span>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-semibold text-slate-800 truncate">{o.label}</p>
                      <p className="text-[10px] text-slate-400 truncate">
                        {o.nome_rca && `RCA: ${o.nome_rca}`}
                        {o.n_outros_prod > 0 && ` · ${o.n_outros_prod} outras indústrias`}
                      </p>
                    </div>
                    <div className="text-right flex-shrink-0">
                      <p className="text-sm font-bold text-amber-700 tabular-nums">{fmtBRL(o.faturado_total)}</p>
                      <p className="text-[10px] text-slate-400">total com distribuidora</p>
                    </div>
                  </div>
                ))}
                {data.oportunidades.length === 0 && (
                  <p className="px-5 py-8 text-sm text-slate-400 text-center">Todos os clientes ativos já compram este produto</p>
                )}
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

function useCliDetalhe(codCli: string, refAno: number, refMes: number) {
  return useQuery<CliDetalheResponse>({
    queryKey: ['mkt-cli-detalhe', codCli, refAno, refMes],
    queryFn: async () => {
      const p = new URLSearchParams({
        cod_cli: codCli,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
      })
      const r = await fetch(`/api/v2/marketing/cliente-detalhe?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar detalhe do cliente')
      return r.json()
    },
    enabled: codCli !== '',
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// ─── Cliente Detalhe ──────────────────────────────────────────────────────────

function ClienteDetalhe({
  codCli, nomeCli, refAno, refMes, onVoltar,
}: {
  codCli: string; nomeCli: string
  refAno: number; refMes: number
  onVoltar: () => void
}) {
  const { data, isLoading, error } = useCliDetalhe(codCli, refAno, refMes)

  return (
    <div>
      {/* Breadcrumb */}
      <div className="flex items-center gap-2 mb-5">
        <button onClick={onVoltar}
          className="flex items-center gap-1.5 text-sm text-slate-500 hover:text-primary transition-colors font-medium">
          <ChevronLeft className="h-4 w-4" /> Voltar
        </button>
        <span className="text-slate-300">/</span>
        <span className="text-sm font-semibold text-slate-700 truncate">{nomeCli || codCli}</span>
        {data?.kpi.nome_rca && <span className="text-xs text-slate-400">· RCA: {data.kpi.nome_rca}</span>}
      </div>

      {isLoading && (
        <div className="space-y-4">
          <div className="bg-white rounded-2xl h-24 animate-pulse border border-slate-100" />
          <div className="grid grid-cols-2 gap-4">
            <div className="bg-white rounded-2xl h-80 animate-pulse border border-slate-100" />
            <div className="bg-white rounded-2xl h-80 animate-pulse border border-slate-100" />
          </div>
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-6 text-red-700 text-sm">
          {(error as Error).message}
        </div>
      )}

      {data && (
        <>
          {/* KPI strip */}
          <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-5 gap-3 mb-6">
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Produtos comprados</p>
              <p className="text-2xl font-black text-slate-800 tabular-nums">{data.kpi.total_produtos}</p>
              <p className="text-[11px] text-slate-500 mt-0.5">no período</p>
            </div>
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Oportunidades</p>
              <p className="text-2xl font-black text-amber-600 tabular-nums">{data.kpi.total_oportunidades}</p>
              <p className="text-[11px] text-slate-500 mt-0.5">produtos populares não comprados</p>
            </div>
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Faturado</p>
              <p className="text-2xl font-black text-slate-800 tabular-nums">{fmtBRL(data.kpi.total_faturado)}</p>
              {data.kpi.total_transmitido > 0 && (
                <p className="text-[11px] text-slate-500 mt-0.5">{fmtBRL(data.kpi.total_transmitido)} transm.</p>
              )}
            </div>
            <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4">
              <p className="text-[11px] text-slate-400 uppercase tracking-wider mb-1">Responsável</p>
              <p className="text-sm font-bold text-slate-800 truncate">{data.kpi.nome_rca || '—'}</p>
              <p className="text-[11px] text-slate-500 truncate">Sup: {data.kpi.nome_sup || '—'}</p>
            </div>
            <div className="bg-amber-50 rounded-xl border border-amber-200 shadow-sm p-4 hidden lg:block">
              <p className="text-[11px] text-amber-600 uppercase tracking-wider mb-1 flex items-center gap-1">
                <Zap className="h-3 w-3" /> Potencial cross-sell
              </p>
              <p className="text-2xl font-black text-amber-700 tabular-nums">{fmtBRL(data.kpi.potencial_estimado)}</p>
              <p className="text-[11px] text-amber-600 mt-0.5">ticket médio dos produtos não comprados</p>
            </div>
          </div>

          {/* Duas colunas */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
            {/* Produtos comprados */}
            <div className="bg-white rounded-2xl shadow-sm border border-emerald-100 overflow-hidden">
              <div className="flex items-center gap-2 px-5 py-4 border-b border-emerald-50 bg-emerald-50/50">
                <Package className="h-4 w-4 text-emerald-600" />
                <div>
                  <h3 className="text-sm font-bold text-emerald-800">Produtos comprados</h3>
                  <p className="text-xs text-emerald-600">{data.comprados.length} produtos · ordenado por faturado</p>
                </div>
              </div>
              <div className="divide-y divide-slate-50 max-h-[480px] overflow-y-auto">
                {data.comprados.map((c, i) => (
                  <div key={c.key} className="flex items-center gap-3 px-5 py-3 hover:bg-emerald-50/30 transition-colors">
                    <span className="w-5 text-xs font-bold text-slate-300 flex-shrink-0">{i + 1}</span>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-semibold text-slate-800 truncate">{c.label}</p>
                      <p className="text-[10px] text-slate-400 truncate">{c.nome_fornec}</p>
                    </div>
                    <div className="text-right flex-shrink-0">
                      <p className="text-sm font-bold text-emerald-700 tabular-nums">{fmtBRL(c.faturado)}</p>
                      {c.transmitido > 0 && (
                        <p className="text-[10px] text-slate-400 tabular-nums">{fmtBRL(c.transmitido)} transm.</p>
                      )}
                    </div>
                  </div>
                ))}
                {data.comprados.length === 0 && (
                  <p className="px-5 py-8 text-sm text-slate-400 text-center">Nenhum produto comprado no período</p>
                )}
              </div>
            </div>

            {/* Oportunidades cross-sell */}
            <div className="bg-white rounded-2xl shadow-sm border border-amber-100 overflow-hidden">
              <div className="flex items-center gap-2 px-5 py-4 border-b border-amber-50 bg-amber-50/50">
                <Target className="h-4 w-4 text-amber-600" />
                <div>
                  <h3 className="text-sm font-bold text-amber-800">Oportunidades de cross-sell</h3>
                  <p className="text-xs text-amber-600">
                    Produtos populares que <strong>este cliente não compra</strong> · ordenado por popularidade
                  </p>
                </div>
              </div>
              <div className="divide-y divide-slate-50 max-h-[480px] overflow-y-auto">
                {data.oportunidades.map((o, i) => (
                  <div key={o.key} className="flex items-center gap-3 px-5 py-3 hover:bg-amber-50/30 transition-colors">
                    <span className="w-5 text-xs font-bold text-slate-300 flex-shrink-0">{i + 1}</span>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-semibold text-slate-800 truncate">{o.label}</p>
                      <p className="text-[10px] text-slate-400 truncate">
                        {o.nome_fornec} · {o.qt_outros_clientes} clientes compram · {fmtPct(o.penetr_pct)} da base
                      </p>
                    </div>
                    <div className="text-right flex-shrink-0">
                      <p className="text-sm font-bold text-amber-700 tabular-nums">{fmtBRL(o.ticket_medio)}</p>
                      <p className="text-[10px] text-slate-400">ticket médio</p>
                    </div>
                  </div>
                ))}
                {data.oportunidades.length === 0 && (
                  <p className="px-5 py-8 text-sm text-slate-400 text-center">Cliente compra todos os produtos disponíveis</p>
                )}
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

// ─── Hero Band ────────────────────────────────────────────────────────────────

function MktHeroBand({ kpi, periodo }: { kpi: MktKPI; periodo: MktResponse['periodo'] }) {
  const cx = 80, cy = 80, r = 56, sw = 10
  const total = 2 * Math.PI * r
  const arc = total * 0.75
  // Clamp a 100% e reduz ligeiramente no máximo para evitar artefato visual
  // do strokeLinecap="round" quando filled = arc (caps se sobrepõem)
  const clampedPct = Math.min(kpi.taxa_positivacao, 100)
  const filled = arc * (clampedPct / 100) * (clampedPct >= 99.5 ? 0.998 : 1)
  const col = clampedPct >= 75 ? '#10b981' : clampedPct >= 50 ? '#f59e0b' : '#ef4444'
  const delta = kpi.delta_positivacao

  return (
    <div className="relative bg-gradient-to-br from-slate-950 via-slate-900 to-slate-900 rounded-2xl overflow-hidden mb-6 shadow-2xl">
      <div className="absolute inset-0 opacity-[0.04]"
        style={{
          backgroundImage: 'linear-gradient(rgba(255,255,255,.5) 1px,transparent 1px),linear-gradient(90deg,rgba(255,255,255,.5) 1px,transparent 1px)',
          backgroundSize: '40px 40px'
        }}
      />
      <div className="relative px-8 py-8">
        {/* Header */}
        <div className="flex items-start justify-between mb-6">
          <div>
            <p className="text-slate-400 text-xs font-medium uppercase tracking-widest mb-1">Painel Marketing</p>
            <p className="text-white/80 text-sm font-medium">{periodo.label}</p>
          </div>
          <div className="flex items-center gap-3">
            <span className="flex items-center gap-1 text-xs font-semibold text-emerald-400 bg-emerald-400/10 border border-emerald-400/20 px-2 py-0.5 rounded-full">
              <UserCheck className="h-3 w-3" /> {kpi.total_ativos} ativos
            </span>
            <span className="flex items-center gap-1 text-xs font-semibold text-red-400 bg-red-400/10 border border-red-400/20 px-2 py-0.5 rounded-full">
              <UserX className="h-3 w-3" /> {kpi.total_inativos} inativos
            </span>
          </div>
        </div>

        {/* Métricas */}
        <div className="flex items-end gap-10 flex-wrap">
          {/* Gauge de positivação */}
          <div className="relative flex-shrink-0">
            <svg width="160" height="115" viewBox="0 0 160 115" className="overflow-visible">
              <defs>
                <filter id="mkt-glow" x="-30%" y="-30%" width="160%" height="160%">
                  <feGaussianBlur stdDeviation="3" result="blur" />
                  <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
                </filter>
              </defs>
              <circle cx={cx} cy={cy} r={r} fill="none"
                stroke="rgba(255,255,255,0.07)" strokeWidth={sw}
                strokeDasharray={`${arc} ${total - arc}`} strokeLinecap="round"
                transform={`rotate(-225 ${cx} ${cy})`} />
              {filled > 1 && (
                <circle cx={cx} cy={cy} r={r} fill="none"
                  stroke={col} strokeWidth={sw}
                  strokeDasharray={`${filled} ${total - filled}`} strokeLinecap="round"
                  transform={`rotate(-225 ${cx} ${cy})`}
                  filter="url(#mkt-glow)"
                  style={{ transition: 'stroke-dasharray 0.8s cubic-bezier(0.4,0,0.2,1)' }}
                />
              )}
            </svg>
            <div className="absolute inset-0 flex flex-col items-center justify-center pt-10">
              <span className="text-3xl font-black tabular-nums" style={{ color: col }}>
                {fmtPct(clampedPct)}
              </span>
              <span className="text-slate-400 text-[10px] uppercase tracking-widest font-semibold mt-0.5">positivação</span>
              {kpi.taxa_positivacao_ant > 0 && (
                <span className={`text-[10px] font-semibold mt-1 ${delta >= 0 ? 'text-emerald-400' : 'text-red-400'}`}>
                  {delta >= 0 ? '+' : ''}{delta.toFixed(1)}pp
                </span>
              )}
            </div>
          </div>

          <div className="w-px h-20 bg-white/10 self-center hidden md:block" />

          {/* Clientes */}
          <div className="flex-shrink-0">
            <div className="flex items-center gap-1 mb-1">
              <p className="text-slate-400 text-xs uppercase tracking-widest">Base de Clientes</p>
            </div>
            <p className="text-white text-4xl font-black tabular-nums leading-none">{kpi.total_base_cli.toLocaleString('pt-BR')}</p>
            <div className="flex gap-3 mt-2">
              <span className="text-xs text-emerald-400 font-semibold flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 inline-block" />
                {kpi.total_ativos} compraram
              </span>
              <span className="text-xs text-red-400 font-semibold flex items-center gap-1">
                <span className="w-1.5 h-1.5 rounded-full bg-red-400 inline-block" />
                {kpi.total_inativos} não compraram
              </span>
            </div>
          </div>

          <div className="w-px h-20 bg-white/10 self-center hidden md:block" />

          {/* KPIs secundários */}
          <div className="flex gap-8 flex-wrap">
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Mix Médio</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtNum(kpi.avg_mix)}</p>
              <p className="text-slate-500 text-[11px] mt-0.5">itens/cliente</p>
            </div>
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Faturado</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtBRL(kpi.total_faturado)}</p>
            </div>
            <div>
              <p className="text-slate-400 text-[11px] uppercase tracking-wider mb-1">Transmitido</p>
              <p className="text-white text-2xl font-bold tabular-nums">{fmtBRL(kpi.total_transmitido)}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Ranking de Penetração ────────────────────────────────────────────────────

function MktRanking({ cards, view, onSelectProd, onSelectCli }: {
  cards: MktCard[]
  view: string
  onSelectProd: (card: MktCard) => void
  onSelectCli: (card: MktCard) => void
}) {
  // Escala sempre 0-100 — penetr_pct já vem capado a 100 do backend
  const maxPenetr = 100
  const medals = ['🥇', '🥈', '🥉']

  const headerLabel = view === 'produto' ? 'Produto'
    : view === 'cliente' ? 'Cliente'
    : 'Indústria'

  const metricLabel = view === 'cliente' ? 'Mix Produtos' : 'Clientes Positivados'

  return (
    <div className="bg-white rounded-2xl shadow-sm border border-slate-100 overflow-hidden">
      <div className="flex items-center justify-between px-6 py-4 border-b border-slate-50">
        <div>
          <h2 className="text-sm font-bold text-slate-800 uppercase tracking-wider">
            Penetração de Mercado
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Por <span className="font-black text-red-600 uppercase tracking-wide">{headerLabel}</span>
            {' '}· ordenado por {metricLabel.toLowerCase()}
          </p>
        </div>
        <div className="flex gap-3 text-[11px]">
          <span className="flex items-center gap-1 text-emerald-600 font-medium">
            <span className="w-2 h-2 rounded-full bg-emerald-500 inline-block" /> ≥ 50%
          </span>
          <span className="flex items-center gap-1 text-amber-600 font-medium">
            <span className="w-2 h-2 rounded-full bg-amber-400 inline-block" /> ≥ 20%
          </span>
          <span className="flex items-center gap-1 text-red-500 font-medium">
            <span className="w-2 h-2 rounded-full bg-red-500 inline-block" /> &lt; 20%
          </span>
        </div>
      </div>

      <div className="divide-y divide-slate-50">
        {cards.map((card, idx) => {
          const safePct = Math.min(card.penetr_pct, 100)
          const barW = safePct  // maxPenetr = 100, então barW = penetr_pct diretamente
          const { bar, text } = corPenetr(safePct)
          const d = card.delta_pct

          return (
            <div
              key={card.key}
              onClick={() => {
                if (view === 'produto') onSelectProd(card)
                if (view === 'cliente') onSelectCli(card)
              }}
              className={`flex items-center gap-4 px-6 py-3.5 transition-colors ${
                view === 'produto' || view === 'cliente'
                  ? 'cursor-pointer hover:bg-amber-50/40 group'
                  : ''
              }`}
            >
              {/* Rank */}
              <div className="w-8 flex-shrink-0 text-center">
                {idx < 3
                  ? <span className="text-xl">{medals[idx]}</span>
                  : <span className="text-sm font-bold text-slate-300">{idx + 1}</span>}
              </div>

              {/* Nome */}
              <div className="w-44 flex-shrink-0">
                <p className="text-sm font-semibold text-slate-800 truncate">{card.label}</p>
                {card.nome_fornec && (
                  <p className="text-[10px] text-slate-400 truncate">{card.nome_fornec}</p>
                )}
                {!card.nome_fornec && (
                  <p className="text-[10px] text-slate-400">{card.key}</p>
                )}
              </div>

              {/* Barra de penetração */}
              <div className="flex-1 min-w-0">
                <div className="h-2.5 bg-slate-100 rounded-full overflow-hidden">
                  <div className={`h-full ${bar} rounded-full transition-all duration-700`}
                    style={{ width: `${barW}%` }} />
                </div>
              </div>

              {/* % Penetração */}
              <div className="w-16 flex-shrink-0 text-right">
                <span className={`text-base font-black tabular-nums ${text}`}>
                  {view === 'cliente'
                    ? fmtNum(card.mix)
                    : fmtPct(safePct)}
                </span>
                {view !== 'cliente' && (
                  <p className="text-[10px] text-slate-400 tabular-nums">{card.qt_clientes} cli</p>
                )}
              </div>

              {/* Faturado */}
              <div className="w-28 flex-shrink-0 text-right hidden sm:block">
                <p className="text-sm font-semibold text-slate-700 tabular-nums">{fmtBRL(card.faturado)}</p>
                <p className="text-[10px] text-slate-400">faturado</p>
              </div>

              {/* Delta vs período anterior */}
              {view !== 'cliente' && (
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
              )}

              {/* Mix (apenas cliente) */}
              {view === 'cliente' && (
                <div className="w-20 flex-shrink-0 text-right hidden md:block">
                  <p className="text-xs font-semibold text-slate-600 tabular-nums">
                    {fmtNum(card.mix)} itens
                  </p>
                  <p className="text-[10px] text-slate-400">mix</p>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ─── Clientes Inativos ────────────────────────────────────────────────────────

function ClientesInativos({ clientes }: { clientes: ClienteInativo[] }) {
  if (clientes.length === 0) return null
  return (
    <div className="bg-white rounded-2xl shadow-sm border border-red-100 overflow-hidden mt-6">
      <div className="flex items-center gap-2 px-6 py-4 border-b border-red-50 bg-red-50/40">
        <UserX className="h-4 w-4 text-red-500" />
        <div>
          <h2 className="text-sm font-bold text-red-700">Clientes Inativos no Período</h2>
          <p className="text-xs text-red-400 mt-0.5">
            {clientes.length} clientes com pedidos transmitidos mas <strong>sem faturamento</strong> — oportunidade de reativação
          </p>
        </div>
      </div>
      <div className="divide-y divide-slate-50 max-h-72 overflow-y-auto">
        {clientes.map((c, idx) => (
          <div key={c.key} className="flex items-center gap-4 px-6 py-3 hover:bg-red-50/20 transition-colors">
            <span className="w-6 text-center text-xs font-bold text-slate-300">{idx + 1}</span>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-semibold text-slate-800 truncate">{c.label}</p>
              <p className="text-[10px] text-slate-400">{c.key}</p>
            </div>
            <div className="text-right">
              <p className="text-sm font-semibold text-amber-600 tabular-nums">{fmtBRL(c.transmitido)}</p>
              <p className="text-[10px] text-slate-400">valor transmitido</p>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

// ─── FarolMarketing ───────────────────────────────────────────────────────────

export default function FarolMarketing() {
  const [view, setView]               = useState<'produto' | 'cliente' | 'fornec'>('produto')
  const [compMode, setCompMode]       = useState('yoy')
  const [refAno, setRefAno]           = useState(0)
  const [refMes, setRefMes]           = useState(0)
  const [compAno, setCompAno]         = useState(0)
  const [selectedProd, setSelectedProd] = useState<MktCard | null>(null)
  const [selectedCli,  setSelectedCli]  = useState<MktCard | null>(null)
  const [compMes, setCompMes]   = useState(0)
  const [search, setSearch]     = useState('')

  const { data, isLoading, error } = useMktCards(
    view, compMode, refAno, refMes,
    compMode === 'mom' ? compAno : 0,
    compMode === 'mom' ? compMes : 0,
  )

  // Auto-seleciona período ao receber dados pela primeira vez
  if (data && refAno === 0 && data.periodo.ref_ano) {
    setRefAno(data.periodo.ref_ano)
    setRefMes(data.periodo.ref_mes)
  }

  const periodos = data?.periodos ?? []

  const visibleCards = useMemo(() => {
    if (!data?.cards) return []
    const term = search.trim().toLowerCase()
    if (!term) return data.cards
    return data.cards.filter(c =>
      c.label.toLowerCase().includes(term) ||
      (c.nome_fornec ?? '').toLowerCase().includes(term)
    )
  }, [data?.cards, search])

  return (
    <div className="min-h-full">
      {/* ── Controles ─────────────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-5">
        {/* Visão */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {([
            { id: 'produto'  as const, label: 'Por Produto',   icon: <Package className="h-3 w-3" /> },
            { id: 'cliente'  as const, label: 'Por Cliente',   icon: <Users className="h-3 w-3" /> },
            { id: 'fornec'   as const, label: 'Por Indústria', icon: null },
          ]).map(v => (
            <button key={v.id} onClick={() => { setView(v.id); setSearch('') }}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium transition-colors ${
                view === v.id ? 'bg-primary text-white' : 'text-slate-600 hover:bg-slate-50'
              }`}>
              {v.icon}{v.label}
            </button>
          ))}
        </div>

        {/* Comparação */}
        <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
          {[
            { id: 'yoy', label: 'Ano a Ano' },
            { id: 'ytd', label: 'Acumulado Anual' },
            { id: 'mom', label: 'Mês a Mês' },
          ].map(m => (
            <TooltipProvider key={m.id} delayDuration={400}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button onClick={() => setCompMode(m.id)}
                    className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                      compMode === m.id ? 'bg-slate-800 text-white' : 'text-slate-600 hover:bg-slate-50'
                    }`}>
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
            onChange={e => { const p = parsePeriodo(e.target.value); setRefAno(p.ano); setRefMes(p.mes) }}
            className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
          >
            {periodos.map(p => {
              const { ano, mes } = parsePeriodo(p)
              return <option key={p} value={p}>{fmtMesAno(ano, mes)}</option>
            })}
          </select>
        )}

        {/* Comparar com (mom) */}
        {compMode === 'mom' && periodos.length > 0 && (
          <>
            <span className="text-xs text-slate-400">comparar com</span>
            <select
              value={compAno > 0 ? `${compAno}-${String(compMes).padStart(2, '0')}` : ''}
              onChange={e => {
                if (!e.target.value) { setCompAno(0); setCompMes(0); return }
                const p = parsePeriodo(e.target.value); setCompAno(p.ano); setCompMes(p.mes)
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

        {/* Busca */}
        {data && data.cards.length > 0 && (
          <input type="text" value={search} onChange={e => setSearch(e.target.value)}
            placeholder={`Buscar ${view === 'produto' ? 'produto' : view === 'cliente' ? 'cliente' : 'indústria'}...`}
            className="h-8 px-3 rounded-lg border border-slate-200 bg-white text-xs text-slate-700 shadow-sm placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-primary/30 w-44 ml-auto"
          />
        )}
      </div>

      {/* ── Loading ────────────────────────────────────────────────────── */}
      {isLoading && (
        <div className="space-y-4">
          <div className="bg-slate-900 rounded-2xl h-44 animate-pulse" />
          <div className="bg-white rounded-2xl h-64 animate-pulse border border-slate-100" />
        </div>
      )}

      {/* ── Erro ───────────────────────────────────────────────────────── */}
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-6 text-red-700 text-sm">
          <p className="font-semibold mb-1">Erro ao carregar painel de marketing</p>
          <p>{(error as Error).message}</p>
        </div>
      )}

      {/* ── Sem dados ──────────────────────────────────────────────────── */}
      {!isLoading && !error && data?.cards.length === 0 && (
        <div className="bg-gradient-to-br from-slate-950 to-slate-900 rounded-2xl p-16 text-center shadow-2xl">
          <p className="text-slate-400 text-sm">Nenhum dado disponível para o período selecionado.</p>
        </div>
      )}

      {/* ── Detalhe de produto ─────────────────────────────────────────── */}
      {selectedProd && (
        <ProdutoDetalhe
          codProd={selectedProd.key}
          nomeProd={selectedProd.label}
          nomeFornec={selectedProd.nome_fornec ?? ''}
          compMode={compMode}
          refAno={refAno} refMes={refMes}
          compAno={compAno} compMes={compMes}
          onVoltar={() => setSelectedProd(null)}
        />
      )}

      {/* ── Detalhe de cliente ─────────────────────────────────────────── */}
      {selectedCli && (
        <ClienteDetalhe
          codCli={selectedCli.key}
          nomeCli={selectedCli.label}
          refAno={refAno} refMes={refMes}
          onVoltar={() => setSelectedCli(null)}
        />
      )}

      {/* ── Conteúdo ───────────────────────────────────────────────────── */}
      {!selectedProd && !selectedCli && !isLoading && !error && data && data.cards.length > 0 && (
        <>
          <MktHeroBand kpi={data.kpi} periodo={data.periodo} />
          <MktRanking cards={visibleCards} view={view} onSelectProd={setSelectedProd} onSelectCli={setSelectedCli} />
          <ClientesInativos clientes={data.clientes_inativos} />
        </>
      )}
    </div>
  )
}
