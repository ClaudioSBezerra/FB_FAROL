import { useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { FarolMobileShell, FarolHeader } from './FarolMobile'
import { Semaforo, type Cor } from '@/components/farol/Semaforo'
import { useAuth } from '@/contexts/AuthContext'

interface RcaItem {
  cod_rca: number
  nome_rca: string
  pct: number
  cor: Cor
  vl_anterior: number
  vl_corrente: number
  qtd_fornec: number
  qtd_abaixo: number
}
interface FarolGeral {
  pct: number
  cor: Cor
  vl_anterior: number
  vl_corrente: number
}
interface PeriodoOut {
  tipo: string
  ano: number
  seq: number
  label: string
}
interface SupResp {
  cod_supervisor: number
  nome: string
  empresa_id: string
  periodo: PeriodoOut | null
  farol_geral: FarolGeral
  rcas: RcaItem[]
}
interface PeriodoItem {
  tipo: string
  ano: number
  seq: number
  label: string
}

function fmtBRL(v: number): string {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}

function saudacao(): string {
  const h = new Date().getHours()
  if (h < 12) return 'Bom dia,'
  if (h < 18) return 'Boa tarde,'
  return 'Boa noite,'
}

export default function FarolDashboard({ embedded = false }: { embedded?: boolean } = {}) {
  const { cod, cnpj: cnpjFromUrl } = useParams<{ cod: string; cnpj?: string }>()
  const { cnpj: cnpjFromAuth } = useAuth()
  // No fluxo web embedded sem CNPJ na URL, usa o CNPJ da empresa autenticada.
  const cnpj = cnpjFromUrl || (embedded ? (cnpjFromAuth || '').replace(/\D/g, '') : undefined)
  const navigate = useNavigate()
  const [periodoKey, setPeriodoKey] = useState<string>('')
  const [showPeriodos, setShowPeriodos] = useState(false)

  const { data: periodos = [] } = useQuery<PeriodoItem[]>({
    queryKey: ['farol-periodos', cnpj, cod],
    queryFn: () => {
      const url = new URL(`/api/farol/periodos/${cod}`, window.location.origin)
      if (cnpj) url.searchParams.set('cnpj', cnpj)
      return fetch(url.toString()).then(r => r.json())
    },
    enabled: !!cod,
  })

  const periodoSel = periodoKey
    ? periodos.find(p => `${p.tipo}|${p.ano}|${p.seq}` === periodoKey)
    : undefined

  const { data, isFetching, isError } = useQuery<SupResp>({
    queryKey: ['farol-sup', cnpj, cod, periodoKey],
    queryFn: () => {
      const url = new URL(`/api/farol/sup/${cod}`, window.location.origin)
      if (cnpj) url.searchParams.set('cnpj', cnpj)
      if (periodoSel) {
        url.searchParams.set('tipo_periodo', periodoSel.tipo)
        url.searchParams.set('ano',          String(periodoSel.ano))
        url.searchParams.set('periodo_seq',  String(periodoSel.seq))
      }
      return fetch(url.toString()).then(r => {
        if (!r.ok) throw new Error('falha ao carregar')
        return r.json()
      })
    },
    enabled: !!cod,
  })

  if (isError) {
    return (
      <FarolMobileShell embedded={embedded}>
        <FarolHeader title="Erro" />
        <div className="p-6 text-center">
          <p className="text-lg mb-4">Não foi possível carregar os dados.</p>
          <button
            onClick={() => window.location.reload()}
            className="bg-[#003366] text-white rounded-lg w-full font-semibold"
            style={{ minHeight: 56, fontSize: 18 }}
          >
            Tentar novamente
          </button>
        </div>
      </FarolMobileShell>
    )
  }

  if (!data) {
    return (
      <FarolMobileShell embedded={embedded}>
        <FarolHeader title="Carregando..." />
        <div className="p-6 text-center text-slate-600">Aguarde...</div>
      </FarolMobileShell>
    )
  }

  const periodoLabel = data.periodo?.label ?? 'Sem período'
  const semDados = !data.periodo || data.rcas.length === 0

  return (
    <FarolMobileShell embedded={embedded}>
      <FarolHeader title="FAROL" subtitle="Painel do Supervisor" />

      <div className="p-4 space-y-4">
        {/* Saudação */}
        <div>
          <p className="text-base text-slate-600">{saudacao()}</p>
          <h2 className="font-bold leading-tight" style={{ fontSize: 26 }}>
            {data.nome}
          </h2>
          <p className="text-sm text-slate-500 mt-1">Supervisor {data.cod_supervisor}</p>
        </div>

        {/* Seletor de período */}
        {periodos.length > 0 && (
          <div className="relative">
            <button
              onClick={() => setShowPeriodos(s => !s)}
              className="w-full bg-white border-2 border-slate-200 rounded-lg px-4 flex items-center justify-between active:bg-slate-50"
              style={{ minHeight: 56, fontSize: 18 }}
            >
              <div className="text-left">
                <p className="text-xs text-slate-500">Período</p>
                <p className="font-semibold">{periodoLabel}</p>
              </div>
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                <path d="m6 9 6 6 6-6"/>
              </svg>
            </button>
            {showPeriodos && (
              <div className="absolute top-full left-0 right-0 mt-1 bg-white border-2 border-slate-200 rounded-lg shadow-lg z-20 max-h-60 overflow-auto">
                {periodos.map(p => {
                  const k = `${p.tipo}|${p.ano}|${p.seq}`
                  const active = k === periodoKey || (!periodoKey && data.periodo?.label === p.label)
                  return (
                    <button
                      key={k}
                      onClick={() => { setPeriodoKey(k); setShowPeriodos(false) }}
                      className={`w-full px-4 py-3 text-left active:bg-slate-100 ${active ? 'bg-blue-50 text-[#003366] font-semibold' : ''}`}
                      style={{ minHeight: 56, fontSize: 18 }}
                    >
                      {p.label}
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        )}

        {/* Card do farol geral */}
        {!semDados && (
          <div className="bg-white border-2 border-slate-200 rounded-xl p-5 shadow-sm">
            <p className="text-sm text-slate-500 mb-3 text-center uppercase tracking-wide">Farol do Período</p>
            <div className="flex flex-col items-center gap-3">
              <Semaforo cor={data.farol_geral.cor} size="lg" />
              <p className="font-bold" style={{ fontSize: 36 }}>
                {data.farol_geral.pct.toFixed(0)}%
              </p>
              <div className="text-center text-sm text-slate-600 space-y-0.5">
                <p>Anterior: <span className="font-semibold">{fmtBRL(data.farol_geral.vl_anterior)}</span></p>
                <p>Atual:    <span className="font-semibold">{fmtBRL(data.farol_geral.vl_corrente)}</span></p>
              </div>
            </div>
          </div>
        )}

        {/* Lista de RCAs */}
        {semDados ? (
          <div className="bg-white border-2 border-dashed border-slate-300 rounded-xl p-8 text-center text-slate-500">
            <p style={{ fontSize: 18 }}>Aguardando importação dos objetivos.</p>
          </div>
        ) : (
          <>
            <div className="flex items-baseline justify-between pt-2">
              <h3 className="font-bold" style={{ fontSize: 18 }}>Seus RCAs</h3>
              <span className="text-sm text-slate-500">({data.rcas.length})</span>
            </div>

            <div className="space-y-3">
              {data.rcas.map(rca => (
                <button
                  key={rca.cod_rca}
                  onClick={() => {
                    const params = new URLSearchParams()
                    if (periodoSel) {
                      params.set('tipo_periodo', periodoSel.tipo)
                      params.set('ano',          String(periodoSel.ano))
                      params.set('periodo_seq',  String(periodoSel.seq))
                    }
                    if (cnpj) params.set('cod_supervisor', cod ?? '')
                    let base: string
                    if (embedded) {
                      base = `/farol/sup/${cod}/rca/${rca.cod_rca}`
                    } else if (cnpjFromUrl) {
                      base = `/m/${cnpjFromUrl}/rca/${rca.cod_rca}`
                    } else {
                      base = `/m/${cod}/rca/${rca.cod_rca}`
                    }
                    navigate(`${base}?${params}`)
                  }}
                  className="w-full bg-white border-2 border-slate-200 rounded-xl p-4 flex items-center gap-4 active:bg-slate-50 active:border-slate-300 text-left"
                  style={{ minHeight: 80 }}
                >
                  <Semaforo cor={rca.cor} size="md" />
                  <div className="flex-1 min-w-0">
                    <p className="font-semibold leading-tight truncate" style={{ fontSize: 18 }}>
                      <span className="text-slate-500 mr-1">{rca.cod_rca}</span>
                      {rca.nome_rca}
                    </p>
                    <p className="text-sm text-slate-600 mt-0.5">
                      {rca.pct.toFixed(0)}% • {fmtBRL(rca.vl_corrente)}
                    </p>
                    {rca.qtd_abaixo > 0 && (
                      <div className="mt-1.5 inline-flex items-center gap-1 bg-amber-100 text-amber-800 rounded-md px-2 py-0.5 text-xs font-semibold">
                        <span>⚠</span>
                        <span>{rca.qtd_abaixo} de {rca.qtd_fornec} abaixo da meta</span>
                      </div>
                    )}
                  </div>
                  <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="text-slate-400 shrink-0">
                    <path d="m9 18 6-6-6-6"/>
                  </svg>
                </button>
              ))}
            </div>
          </>
        )}

        {isFetching && (
          <p className="text-center text-sm text-slate-500 py-2">Atualizando...</p>
        )}
      </div>
    </FarolMobileShell>
  )
}
