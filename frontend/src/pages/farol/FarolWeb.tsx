import { useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Semaforo, type Cor } from '@/components/farol/Semaforo'

// ─── Tipos ────────────────────────────────────────────────────────────────────

interface PeriodoOut { tipo: string; ano: number; seq: number; label: string }
interface Resumo     { pct: number; cor: Cor; vl_anterior: number; vl_corrente: number }

function fmtBRL(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}
function periodoParams(p: PeriodoOut) {
  return `tipo_periodo=${p.tipo}&ano=${p.ano}&periodo_seq=${p.seq}`
}

const COR_BORDER: Record<Cor, string> = {
  verde:    'border-l-emerald-500',
  amarelo:  'border-l-amber-400',
  vermelho: 'border-l-red-500',
}
const COR_BAR: Record<Cor, string> = {
  verde:    'bg-emerald-500',
  amarelo:  'bg-amber-400',
  vermelho: 'bg-red-500',
}
const COR_TEXT: Record<Cor, string> = {
  verde:    'text-emerald-600',
  amarelo:  'text-amber-600',
  vermelho: 'text-red-600',
}

// ─── Período ─────────────────────────────────────────────────────────────────

function usePeriodos() {
  return useQuery<PeriodoOut[]>({
    queryKey: ['farol-web-periodos'],
    queryFn: () => fetch('/api/farol/web/periodos').then(r => r.json()),
    staleTime: 5 * 60_000,
    gcTime:    10 * 60_000,
    refetchOnWindowFocus: false,
  })
}
function usePeriodoSel(key: string): PeriodoOut | undefined {
  const { data: periodos = [] } = usePeriodos()
  return periodos.find(p => `${p.tipo}|${p.ano}|${p.seq}` === key) ?? periodos[0]
}
function PeriodoSelect({ value, onChange }: { value: string; onChange: (k: string) => void }) {
  const { data: periodos = [] } = usePeriodos()
  return (
    <select
      value={value}
      onChange={e => onChange(e.target.value)}
      className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-[#003366]/30"
    >
      {periodos.map(p => {
        const k = `${p.tipo}|${p.ano}|${p.seq}`
        return <option key={k} value={k}>{p.label}</option>
      })}
    </select>
  )
}

// ─── Card de entidade (supervisor ou RCA) ─────────────────────────────────────

function EntityCard({
  cor, pct, title, sub, value, badge, onClick,
}: {
  cor: Cor; pct: number; title: string; sub?: string; value: string
  badge?: string; onClick: () => void
}) {
  return (
    <button
      onClick={onClick}
      className={`relative bg-white border border-slate-100 border-l-4 ${COR_BORDER[cor]} rounded-xl shadow-sm hover:shadow-md transition-all overflow-hidden text-left w-full`}
    >
      {/* barra de progresso no topo */}
      <div className="h-1 bg-slate-100 w-full">
        <div className={`h-full ${COR_BAR[cor]} transition-all`} style={{ width: `${Math.min(pct, 100)}%` }} />
      </div>

      <div className="p-4">
        <p className="text-xs text-slate-400 truncate">{sub}</p>
        <p className="font-semibold text-slate-800 truncate mt-0.5">{title}</p>

        <p className={`text-3xl font-bold mt-3 leading-none ${COR_TEXT[cor]}`}>
          {pct.toFixed(0)}<span className="text-base font-normal text-slate-400 ml-0.5">%</span>
        </p>
        <p className="text-xs text-slate-500 mt-1">{value}</p>

        {badge && (
          <div className="mt-2.5 inline-flex items-center gap-1 bg-amber-50 text-amber-700 border border-amber-200 rounded-full px-2.5 py-0.5 text-xs font-semibold">
            <span>⚠</span> {badge}
          </div>
        )}
      </div>

      <div className="absolute right-3 bottom-4 text-slate-300">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
          <path d="m9 18 6-6-6-6"/>
        </svg>
      </div>
    </button>
  )
}

// ─── Cabeçalho com farol geral ────────────────────────────────────────────────

function FarolHeader({ title, sub, resumo, periodo }: { title: string; sub?: string; resumo: Resumo; periodo?: PeriodoOut }) {
  return (
    <div className="flex items-start justify-between gap-4 flex-wrap">
      <div>
        <h1 className="text-xl font-bold text-slate-800">{title}</h1>
        {sub && <p className="text-sm text-slate-500 mt-0.5">{sub}</p>}
        {periodo && <p className="text-xs text-slate-400 mt-1">{periodo.label}</p>}
      </div>
      <div className={`flex items-center gap-3 bg-white border border-slate-100 rounded-xl px-5 py-3 shadow-sm border-l-4 ${COR_BORDER[resumo.cor]}`}>
        <Semaforo cor={resumo.cor} size="md" />
        <div>
          <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">Realização</p>
          <p className={`text-2xl font-bold leading-none ${COR_TEXT[resumo.cor]}`}>{resumo.pct.toFixed(0)}%</p>
          <p className="text-xs text-slate-500 mt-0.5">
            {fmtBRL(resumo.vl_anterior)} → {fmtBRL(resumo.vl_corrente)}
          </p>
        </div>
      </div>
    </div>
  )
}

// ─── Tela 1: Lista de supervisores ────────────────────────────────────────────

interface SupItem {
  cod_supervisor: number; nome: string
  pct: number; cor: Cor
  vl_anterior: number; vl_corrente: number
  qtd_rcas: number; qtd_rcas_abaixo: number
}
interface SupervisoresResp { periodo: PeriodoOut | null; supervisores: SupItem[] }

export function FarolWebList() {
  const navigate = useNavigate()
  const [periodoKey, setPeriodoKey] = useState('')
  const [filtro, setFiltro] = useState('')
  const periodo = usePeriodoSel(periodoKey)

  const { data, isLoading, isError } = useQuery<SupervisoresResp>({
    queryKey: ['farol-web-supervisores', periodoKey],
    queryFn: () => {
      const qs = periodo ? periodoParams(periodo) : ''
      return fetch(`/api/farol/web/supervisores${qs ? '?' + qs : ''}`).then(r => {
        if (!r.ok) throw new Error('falha')
        return r.json()
      })
    },
    staleTime: 2 * 60_000,
    gcTime:    10 * 60_000,
    refetchOnWindowFocus: false,
  })

  const supervisores = useMemo<SupItem[]>(
    () => Array.isArray(data?.supervisores) ? data!.supervisores : [],
    [data]
  )
  const filtered = useMemo(() => {
    if (!filtro) return supervisores
    const q = filtro.toLowerCase()
    return supervisores.filter(s => s.nome.toLowerCase().includes(q) || String(s.cod_supervisor).includes(q))
  }, [supervisores, filtro])

  const periodoAtual = data?.periodo ?? periodo

  return (
    <div className="space-y-5 max-w-5xl mx-auto">

      {/* cabeçalho */}
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-xl font-bold text-slate-800">Farol de Supervisores</h1>
          {periodoAtual && (
            <p className="text-sm text-slate-400 mt-0.5">Período: {periodoAtual.label}</p>
          )}
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <PeriodoSelect value={periodoKey} onChange={setPeriodoKey} />
          <div className="relative">
            <svg className="absolute left-2.5 top-2 w-4 h-4 text-slate-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
            </svg>
            <input
              type="text" placeholder="Filtrar supervisor..." value={filtro}
              onChange={e => setFiltro(e.target.value)}
              className="h-9 rounded-lg border border-slate-200 bg-white pl-8 pr-3 text-sm text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-[#003366]/30 w-52"
            />
          </div>
        </div>
      </div>

      {isLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="bg-slate-100 rounded-xl h-28 animate-pulse" />
          ))}
        </div>
      )}

      {isError && (
        <div className="rounded-xl border border-red-100 bg-red-50 p-6 text-center text-red-600 text-sm">
          Erro ao carregar dados.
        </div>
      )}

      {!isLoading && filtered.length === 0 && (
        <div className="rounded-xl border-2 border-dashed border-slate-200 p-12 text-center text-slate-400 text-sm">
          {supervisores.length === 0 ? 'Aguardando importação dos objetivos.' : 'Nenhum supervisor encontrado.'}
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {filtered.map(s => {
          const qs = periodoAtual ? '?' + periodoParams(periodoAtual) : ''
          return (
            <EntityCard
              key={s.cod_supervisor}
              cor={s.cor}
              pct={s.pct}
              title={s.nome}
              sub={`Supervisor ${s.cod_supervisor}`}
              value={`${fmtBRL(s.vl_corrente)} atual`}
              badge={s.qtd_rcas_abaixo > 0 ? `${s.qtd_rcas_abaixo} de ${s.qtd_rcas} RCA(s) abaixo` : undefined}
              onClick={() => navigate(`/farol/sup/${s.cod_supervisor}${qs}`)}
            />
          )
        })}
      </div>
    </div>
  )
}

// ─── Tela 2: RCAs do supervisor ───────────────────────────────────────────────

interface RcaItem {
  cod_rca: number; nome_rca: string
  pct: number; cor: Cor
  vl_anterior: number; vl_corrente: number
  qtd_fornec: number; qtd_abaixo: number
}
interface SupResp {
  cod_supervisor: number; nome: string
  periodo: PeriodoOut | null
  farol_geral: Resumo
  rcas: RcaItem[]
}

export function FarolWebDashboard() {
  const { cod } = useParams<{ cod: string }>()
  const [search] = useSearchParams()
  const navigate = useNavigate()
  const qs = search.toString()

  const { data, isLoading, isError } = useQuery<SupResp>({
    queryKey: ['farol-web-sup', cod, qs],
    queryFn: () => fetch(`/api/farol/web/sup/${cod}${qs ? '?' + qs : ''}`).then(r => {
      if (!r.ok) throw new Error('falha')
      return r.json()
    }),
    enabled: !!cod,
    staleTime: 2 * 60_000,
    gcTime:    10 * 60_000,
    refetchOnWindowFocus: false,
  })

  const periodoQs = data?.periodo ? periodoParams(data.periodo) : qs

  return (
    <div className="space-y-5 max-w-5xl mx-auto">
      <button
        onClick={() => navigate('/farol')}
        className="inline-flex items-center gap-1.5 text-sm text-[#003366] hover:text-[#003366]/70 transition-colors font-medium"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
          <path d="m15 18-6-6 6-6"/>
        </svg>
        Todos os supervisores
      </button>

      {isLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {[...Array(6)].map((_, i) => <div key={i} className="bg-slate-100 rounded-xl h-28 animate-pulse" />)}
        </div>
      )}
      {isError && (
        <div className="rounded-xl border border-red-100 bg-red-50 p-6 text-center text-red-600 text-sm">
          Erro ao carregar dados.
        </div>
      )}

      {data && (
        <>
          <FarolHeader
            title={data.nome}
            sub={`Supervisor ${data.cod_supervisor}`}
            resumo={data.farol_geral}
            periodo={data.periodo ?? undefined}
          />

          {data.rcas.length === 0 ? (
            <div className="rounded-xl border-2 border-dashed border-slate-200 p-12 text-center text-slate-400 text-sm">
              Aguardando importação dos objetivos.
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              {data.rcas.map(rca => (
                <EntityCard
                  key={rca.cod_rca}
                  cor={rca.cor}
                  pct={rca.pct}
                  title={rca.nome_rca}
                  sub={`RCA ${rca.cod_rca}`}
                  value={`${fmtBRL(rca.vl_corrente)} atual`}
                  badge={rca.qtd_abaixo > 0 ? `${rca.qtd_abaixo} de ${rca.qtd_fornec} fornecedor(es) abaixo` : undefined}
                  onClick={() => navigate(`/farol/sup/${cod}/rca/${rca.cod_rca}?${periodoQs}&cod_supervisor=${cod}`)}
                />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}

// ─── Tela 3: Fornecedores do RCA ─────────────────────────────────────────────

interface FornecItem {
  cod_fornec: string; fornecedor: string
  pct: number; cor: Cor
  vl_anterior: number; vl_corrente: number
}
interface RcaResp {
  cod_rca: number; nome_rca: string
  cod_supervisor: number; nome_supervisor: string
  periodo: PeriodoOut | null
  resumo: Resumo
  fornecedores: FornecItem[]
}

export function FarolWebRcaDetail() {
  const { cod, codRca } = useParams<{ cod: string; codRca: string }>()
  const [search] = useSearchParams()
  const navigate = useNavigate()
  const qs = search.toString()

  const { data, isLoading, isError } = useQuery<RcaResp>({
    queryKey: ['farol-web-rca', cod, codRca, qs],
    queryFn: () => fetch(`/api/farol/web/rca/${codRca}?${qs}`).then(r => {
      if (!r.ok) throw new Error('falha')
      return r.json()
    }),
    enabled: !!codRca,
    staleTime: 2 * 60_000,
    gcTime:    10 * 60_000,
    refetchOnWindowFocus: false,
  })

  const periodoQs = data?.periodo ? periodoParams(data.periodo) : qs

  return (
    <div className="space-y-5 max-w-5xl mx-auto">
      <button
        onClick={() => navigate(`/farol/sup/${cod}?${periodoQs}`)}
        className="inline-flex items-center gap-1.5 text-sm text-[#003366] hover:text-[#003366]/70 transition-colors font-medium"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
          <path d="m15 18-6-6 6-6"/>
        </svg>
        {data ? `${data.cod_supervisor} — ${data.nome_supervisor}` : 'Voltar'}
      </button>

      {isLoading && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {[...Array(4)].map((_, i) => <div key={i} className="bg-slate-100 rounded-xl h-28 animate-pulse" />)}
        </div>
      )}
      {isError && (
        <div className="rounded-xl border border-red-100 bg-red-50 p-6 text-center text-red-600 text-sm">
          Erro ao carregar dados.
        </div>
      )}

      {data && (
        <>
          <FarolHeader
            title={data.nome_rca}
            sub={`RCA ${data.cod_rca} · Sup. ${data.cod_supervisor} — ${data.nome_supervisor}`}
            resumo={data.resumo}
            periodo={data.periodo ?? undefined}
          />

          {data.fornecedores.length === 0 ? (
            <div className="rounded-xl border-2 border-dashed border-slate-200 p-12 text-center text-slate-400 text-sm">
              Sem fornecedores para este RCA no período.
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              {data.fornecedores.map(f => (
                <div
                  key={f.cod_fornec}
                  className={`relative bg-white border border-slate-100 border-l-4 ${COR_BORDER[f.cor]} rounded-xl shadow-sm overflow-hidden`}
                >
                  <div className="h-1 bg-slate-100">
                    <div className={`h-full ${COR_BAR[f.cor]}`} style={{ width: `${Math.min(f.pct, 100)}%` }} />
                  </div>
                  <div className="p-4">
                    <p className="font-semibold text-slate-800 truncate">{f.fornecedor || f.cod_fornec}</p>
                    <p className={`text-3xl font-bold mt-3 leading-none ${COR_TEXT[f.cor]}`}>
                      {f.pct.toFixed(0)}<span className="text-base font-normal text-slate-400 ml-0.5">%</span>
                    </p>
                    <div className="mt-3 grid grid-cols-2 gap-2 pt-3 border-t border-slate-50 text-xs">
                      <div>
                        <p className="text-slate-400 uppercase tracking-wider text-[10px]">Obj. Anterior</p>
                        <p className="font-semibold text-slate-600 mt-0.5">{fmtBRL(f.vl_anterior)}</p>
                      </div>
                      <div>
                        <p className="text-slate-400 uppercase tracking-wider text-[10px]">Obj. Atual</p>
                        <p className="font-semibold text-slate-800 mt-0.5">{fmtBRL(f.vl_corrente)}</p>
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
