import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@/contexts/AuthContext'
import { Semaforo, type Cor } from '@/components/farol/Semaforo'
import FarolDashboard from './FarolDashboard'
import FarolRcaDetail from './FarolRcaDetail'

interface SupListItem {
  cod_supervisor: number
  nome: string
  pct: number
  cor: Cor
  vl_anterior: number
  vl_corrente: number
  qtd_rcas: number
  qtd_rcas_abaixo: number
}
interface PeriodoOut { tipo: string; ano: number; seq: number; label: string }
interface ListResp { empresa_id: string; periodo: PeriodoOut | null; supervisores: SupListItem[] }

function fmtBRL(v: number): string {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}

// ─── Lista de supervisores (entrada do Farol web) ───────────────────────────

export function FarolWebList() {
  const { cnpj } = useAuth()
  const navigate = useNavigate()
  const cnpjDigits = (cnpj || '').replace(/\D/g, '')

  const { data, isLoading, isError } = useQuery<ListResp>({
    queryKey: ['farol-supervisores', cnpjDigits],
    queryFn: () => fetch(`/api/farol/supervisores?cnpj=${cnpjDigits}`).then(r => {
      if (!r.ok) throw new Error('falha')
      return r.json()
    }),
    enabled: !!cnpjDigits,
    staleTime: 2 * 60_000,
    gcTime:    10 * 60_000,
    refetchOnWindowFocus: false,
  })

  const supervisores = useMemo<SupListItem[]>(
    () => Array.isArray(data?.supervisores) ? data!.supervisores : [],
    [data]
  )

  const [filtro, setFiltro] = useState('')
  const filtered = useMemo(() => {
    if (!filtro) return supervisores
    const q = filtro.toLowerCase()
    return supervisores.filter(s =>
      s.nome.toLowerCase().includes(q) || String(s.cod_supervisor).includes(q)
    )
  }, [supervisores, filtro])

  if (!cnpjDigits) {
    return (
      <div className="p-6 text-center text-muted-foreground">
        Empresa sem CNPJ cadastrado. Cadastre o CNPJ em <strong>Gestão de Ambiente</strong>.
      </div>
    )
  }
  if (isLoading) return <div className="p-6 text-center text-muted-foreground">Carregando...</div>
  if (isError)   return <div className="p-6 text-center text-red-600">Erro ao carregar dados.</div>

  return (
    <div className="space-y-4 max-w-5xl mx-auto">
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-xl font-bold">Farol — Supervisores</h1>
          {data?.periodo && (
            <p className="text-sm text-muted-foreground">Período: {data.periodo.label}</p>
          )}
        </div>
        <input
          type="text"
          placeholder="Filtrar por código ou nome..."
          value={filtro}
          onChange={e => setFiltro(e.target.value)}
          className="border rounded-md px-3 py-2 text-sm w-64"
        />
      </div>

      {filtered.length === 0 ? (
        <div className="bg-white border-2 border-dashed border-slate-300 rounded-xl p-8 text-center text-slate-500">
          {supervisores.length === 0 ? 'Aguardando importação dos objetivos.' : 'Nenhum supervisor encontrado.'}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {filtered.map(s => (
            <button
              key={s.cod_supervisor}
              onClick={() => navigate(`/farol/sup/${s.cod_supervisor}`)}
              className="bg-white border-2 border-slate-200 hover:border-[#003366]/50 rounded-xl p-4 flex items-center gap-4 text-left transition-colors"
            >
              <Semaforo cor={s.cor} size="md" />
              <div className="flex-1 min-w-0">
                <p className="font-semibold leading-tight truncate">
                  <span className="text-slate-500 mr-1">{s.cod_supervisor}</span>
                  {s.nome}
                </p>
                <p className="text-sm text-slate-600 mt-0.5">
                  {s.pct.toFixed(0)}% • {fmtBRL(s.vl_corrente)}
                </p>
                {s.qtd_rcas_abaixo > 0 && (
                  <div className="mt-1.5 inline-flex items-center gap-1 bg-amber-100 text-amber-800 rounded-md px-2 py-0.5 text-xs font-semibold">
                    <span>⚠</span>
                    <span>{s.qtd_rcas_abaixo} de {s.qtd_rcas} RCA(s) abaixo</span>
                  </div>
                )}
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Wrappers que reaproveitam os componentes mobile (modo embedded) ─────────

export function FarolWebDashboard() {
  const navigate = useNavigate()
  return (
    <div className="space-y-3 max-w-3xl mx-auto">
      <button
        onClick={() => navigate('/farol')}
        className="text-sm text-[#003366] hover:underline"
      >
        ← Voltar para lista
      </button>
      <FarolDashboard embedded />
    </div>
  )
}

export function FarolWebRcaDetail() {
  return (
    <div className="space-y-3 max-w-3xl mx-auto">
      <FarolRcaDetail embedded />
    </div>
  )
}
