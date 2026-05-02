import { useMemo, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Semaforo, type Cor } from '@/components/farol/Semaforo'

// ─── tipos compartilhados ────────────────────────────────────────────────────

interface PeriodoOut { tipo: string; ano: number; seq: number; label: string }
interface Resumo     { pct: number; cor: Cor; vl_anterior: number; vl_corrente: number }

function fmtBRL(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}

function periodoParams(p: PeriodoOut) {
  return `tipo_periodo=${p.tipo}&ano=${p.ano}&periodo_seq=${p.seq}`
}

function usePeriodos() {
  return useQuery<PeriodoOut[]>({
    queryKey: ['farol-web-periodos'],
    queryFn: () => fetch('/api/farol/web/periodos').then(r => r.json()),
    staleTime: 5 * 60_000,
    gcTime:    10 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// ─── seletor de período ───────────────────────────────────────────────────────

function PeriodoSelect({ value, onChange }: { value: string; onChange: (k: string) => void }) {
  const { data: periodos = [] } = usePeriodos()
  return (
    <select
      value={value}
      onChange={e => onChange(e.target.value)}
      className="border rounded-md px-3 py-2 text-sm bg-white"
    >
      {periodos.map(p => {
        const k = `${p.tipo}|${p.ano}|${p.seq}`
        return <option key={k} value={k}>{p.label}</option>
      })}
    </select>
  )
}

function usePeriodoSel(key: string): PeriodoOut | undefined {
  const { data: periodos = [] } = usePeriodos()
  return periodos.find(p => `${p.tipo}|${p.ano}|${p.seq}` === key) ?? periodos[0]
}

// ─── Tela 1: Lista de supervisores ───────────────────────────────────────────

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
    <div className="space-y-4 max-w-5xl mx-auto">
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-xl font-bold">Farol — Supervisores</h1>
          {periodoAtual && <p className="text-sm text-muted-foreground">Período: {periodoAtual.label}</p>}
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <PeriodoSelect value={periodoKey} onChange={setPeriodoKey} />
          <input
            type="text" placeholder="Filtrar..." value={filtro}
            onChange={e => setFiltro(e.target.value)}
            className="border rounded-md px-3 py-2 text-sm w-52"
          />
        </div>
      </div>

      {isLoading && <p className="text-center text-muted-foreground py-8">Carregando...</p>}
      {isError  && <p className="text-center text-red-600 py-8">Erro ao carregar dados.</p>}

      {!isLoading && filtered.length === 0 && (
        <div className="bg-white border-2 border-dashed border-slate-300 rounded-xl p-8 text-center text-slate-500">
          {supervisores.length === 0 ? 'Aguardando importação dos objetivos.' : 'Nenhum supervisor encontrado.'}
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {filtered.map(s => {
          const qs = periodoAtual ? '?' + periodoParams(periodoAtual) : ''
          return (
            <button
              key={s.cod_supervisor}
              onClick={() => navigate(`/farol/sup/${s.cod_supervisor}${qs}`)}
              className="bg-white border-2 border-slate-200 hover:border-[#003366]/50 rounded-xl p-4 flex items-center gap-4 text-left transition-colors"
            >
              <Semaforo cor={s.cor} size="md" />
              <div className="flex-1 min-w-0">
                <p className="font-semibold leading-tight truncate">
                  <span className="text-slate-500 mr-1">{s.cod_supervisor}</span>{s.nome}
                </p>
                <p className="text-sm text-slate-600 mt-0.5">
                  {s.pct.toFixed(0)}% • {fmtBRL(s.vl_corrente)}
                </p>
                {s.qtd_rcas_abaixo > 0 && (
                  <div className="mt-1.5 inline-flex items-center gap-1 bg-amber-100 text-amber-800 rounded-md px-2 py-0.5 text-xs font-semibold">
                    ⚠ {s.qtd_rcas_abaixo} de {s.qtd_rcas} RCA(s) abaixo
                  </div>
                )}
              </div>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="text-slate-400 shrink-0">
                <path d="m9 18 6-6-6-6"/>
              </svg>
            </button>
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
    <div className="space-y-4 max-w-5xl mx-auto">
      <button onClick={() => navigate('/farol')} className="text-sm text-[#003366] hover:underline">
        ← Todos os supervisores
      </button>

      {isLoading && <p className="text-center text-muted-foreground py-8">Carregando...</p>}
      {isError   && <p className="text-center text-red-600 py-8">Erro ao carregar dados.</p>}

      {data && (
        <>
          <div className="flex items-center gap-4 flex-wrap justify-between">
            <div>
              <h1 className="text-xl font-bold">
                <span className="text-slate-500 mr-1">{data.cod_supervisor}</span>{data.nome}
              </h1>
              {data.periodo && <p className="text-sm text-muted-foreground">Período: {data.periodo.label}</p>}
            </div>
            <div className="flex items-center gap-3 bg-white border-2 border-slate-200 rounded-xl px-5 py-3">
              <Semaforo cor={data.farol_geral.cor} size="md" />
              <div>
                <p className="text-xs text-slate-500 uppercase tracking-wide">Farol Geral</p>
                <p className="text-2xl font-bold leading-none">{data.farol_geral.pct.toFixed(0)}%</p>
                <p className="text-xs text-slate-600 mt-0.5">
                  {fmtBRL(data.farol_geral.vl_anterior)} → {fmtBRL(data.farol_geral.vl_corrente)}
                </p>
              </div>
            </div>
          </div>

          {data.rcas.length === 0 ? (
            <div className="bg-white border-2 border-dashed border-slate-300 rounded-xl p-8 text-center text-slate-500">
              Aguardando importação dos objetivos.
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              {data.rcas.map(rca => (
                <button
                  key={rca.cod_rca}
                  onClick={() => navigate(`/farol/sup/${cod}/rca/${rca.cod_rca}?${periodoQs}&cod_supervisor=${cod}`)}
                  className="bg-white border-2 border-slate-200 hover:border-[#003366]/50 rounded-xl p-4 flex items-center gap-4 text-left transition-colors"
                >
                  <Semaforo cor={rca.cor} size="md" />
                  <div className="flex-1 min-w-0">
                    <p className="font-semibold leading-tight truncate">
                      <span className="text-slate-500 mr-1">{rca.cod_rca}</span>{rca.nome_rca}
                    </p>
                    <p className="text-sm text-slate-600 mt-0.5">
                      {rca.pct.toFixed(0)}% • {fmtBRL(rca.vl_corrente)}
                    </p>
                    {rca.qtd_abaixo > 0 && (
                      <div className="mt-1.5 inline-flex items-center gap-1 bg-amber-100 text-amber-800 rounded-md px-2 py-0.5 text-xs font-semibold">
                        ⚠ {rca.qtd_abaixo} de {rca.qtd_fornec} fornecedor(es) abaixo
                      </div>
                    )}
                  </div>
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="text-slate-400 shrink-0">
                    <path d="m9 18 6-6-6-6"/>
                  </svg>
                </button>
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
    <div className="space-y-4 max-w-5xl mx-auto">
      <button onClick={() => navigate(`/farol/sup/${cod}?${periodoQs}`)} className="text-sm text-[#003366] hover:underline">
        ← {data ? `${data.cod_supervisor} — ${data.nome_supervisor}` : 'Voltar'}
      </button>

      {isLoading && <p className="text-center text-muted-foreground py-8">Carregando...</p>}
      {isError   && <p className="text-center text-red-600 py-8">Erro ao carregar dados.</p>}

      {data && (
        <>
          <div className="flex items-center gap-4 flex-wrap justify-between">
            <div>
              <h1 className="text-xl font-bold">
                <span className="text-slate-500 mr-1">{data.cod_rca}</span>{data.nome_rca}
              </h1>
              <p className="text-sm text-muted-foreground">
                Sup. {data.cod_supervisor} — {data.nome_supervisor}
                {data.periodo && ` • ${data.periodo.label}`}
              </p>
            </div>
            <div className="flex items-center gap-3 bg-white border-2 border-slate-200 rounded-xl px-5 py-3">
              <Semaforo cor={data.resumo.cor} size="md" />
              <div>
                <p className="text-xs text-slate-500 uppercase tracking-wide">Realização</p>
                <p className="text-2xl font-bold leading-none">{data.resumo.pct.toFixed(0)}%</p>
                <p className="text-xs text-slate-600 mt-0.5">
                  {fmtBRL(data.resumo.vl_anterior)} → {fmtBRL(data.resumo.vl_corrente)}
                </p>
              </div>
            </div>
          </div>

          {data.fornecedores.length === 0 ? (
            <div className="bg-white border-2 border-dashed border-slate-300 rounded-xl p-8 text-center text-slate-500">
              Sem fornecedores para este RCA no período.
            </div>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              {data.fornecedores.map(f => (
                <div
                  key={f.cod_fornec}
                  className="bg-white border-2 border-slate-200 rounded-xl p-4 space-y-2"
                >
                  <div className="flex items-center gap-3">
                    <Semaforo cor={f.cor} size="sm" />
                    <p className="font-semibold leading-tight flex-1 min-w-0 truncate text-sm">
                      {f.fornecedor || f.cod_fornec}
                    </p>
                    <span className="font-bold text-base">{f.pct.toFixed(0)}%</span>
                  </div>
                  <div className="grid grid-cols-2 gap-1 pt-2 border-t border-slate-100 text-xs text-slate-600">
                    <div>
                      <p className="text-slate-400">Obj. Anterior</p>
                      <p className="font-semibold">{fmtBRL(f.vl_anterior)}</p>
                    </div>
                    <div>
                      <p className="text-slate-400">Obj. Atual</p>
                      <p className="font-semibold">{fmtBRL(f.vl_corrente)}</p>
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
