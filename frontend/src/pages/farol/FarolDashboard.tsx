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

  const periodoAtual = data.periodo?.label ?? 'Sem período'
  const semDados = !data.periodo || data.rcas.length === 0
  const corGeral = data.farol_geral.cor
  const corBar = corGeral === 'verde' ? 'bg-emerald-500' : corGeral === 'amarelo' ? 'bg-amber-400' : 'bg-red-500'
  const corText = corGeral === 'verde' ? 'text-emerald-600' : corGeral === 'amarelo' ? 'text-amber-600' : 'text-red-500'

  return (
    <FarolMobileShell embedded={embedded}>
      <FarolHeader title="FAROL" subtitle={`Supervisor ${data.cod_supervisor}`} />

      <div className="p-4 space-y-4">
        {/* Saudação */}
        <div className="pt-1">
          <p className="text-sm text-slate-400">{saudacao()}</p>
          <h2 className="text-2xl font-bold text-slate-800 leading-tight mt-0.5">{data.nome}</h2>
        </div>

        {/* Seletor de período */}
        {periodos.length > 0 && (
          <div className="relative">
            <button
              onClick={() => setShowPeriodos(s => !s)}
              className="w-full bg-white border border-slate-200 rounded-xl px-4 flex items-center justify-between shadow-sm active:bg-slate-50 transition-colors"
              style={{ minHeight: 52 }}
            >
              <div className="text-left">
                <p className="text-[10px] text-slate-400 uppercase tracking-wider font-semibold">Período</p>
                <p className="text-base font-semibold text-slate-700">{periodoAtual}</p>
              </div>
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" className="text-slate-400">
                <path d="m6 9 6 6 6-6"/>
              </svg>
            </button>
            {showPeriodos && (
              <div className="absolute top-full left-0 right-0 mt-1 bg-white border border-slate-200 rounded-xl shadow-xl z-20 max-h-60 overflow-auto">
                {periodos.map(p => {
                  const k = `${p.tipo}|${p.ano}|${p.seq}`
                  const active = k === periodoKey || (!periodoKey && data.periodo?.label === p.label)
                  return (
                    <button
                      key={k}
                      onClick={() => { setPeriodoKey(k); setShowPeriodos(false) }}
                      className={`w-full px-4 py-3.5 text-left text-base transition-colors ${
                        active ? 'bg-[#003366]/5 text-[#003366] font-semibold' : 'text-slate-700 hover:bg-slate-50'
                      }`}
                    >
                      {p.label}
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        )}

        {/* Farol geral */}
        {!semDados && (
          <div className="bg-white border border-slate-100 rounded-xl shadow-sm overflow-hidden">
            <div className="h-1.5 bg-slate-100">
              <div className={`h-full ${corBar} transition-all`} style={{ width: `${Math.min(data.farol_geral.pct, 100)}%` }} />
            </div>
            <div className="p-5 flex items-center gap-4">
              <Semaforo cor={data.farol_geral.cor} size="lg" />
              <div className="flex-1">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">Farol do Período</p>
                <p className={`text-4xl font-bold leading-none mt-1 ${corText}`}>
                  {data.farol_geral.pct.toFixed(0)}<span className="text-lg text-slate-400 font-normal ml-1">%</span>
                </p>
                <p className="text-xs text-slate-500 mt-1.5">
                  {fmtBRL(data.farol_geral.vl_anterior)} → {fmtBRL(data.farol_geral.vl_corrente)}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Lista de RCAs */}
        {semDados ? (
          <div className="bg-white border-2 border-dashed border-slate-200 rounded-xl p-8 text-center text-slate-400">
            <p className="text-base">Aguardando importação dos objetivos.</p>
          </div>
        ) : (
          <>
            <div className="flex items-center justify-between">
              <h3 className="text-base font-semibold text-slate-700">Seus RCAs</h3>
              <span className="text-xs text-slate-400 bg-slate-100 px-2 py-0.5 rounded-full">{data.rcas.length}</span>
            </div>

            <div className="space-y-2">
              {data.rcas.map(rca => {
                const c = rca.cor
                const border = c === 'verde' ? 'border-l-emerald-500' : c === 'amarelo' ? 'border-l-amber-400' : 'border-l-red-500'
                const bar    = c === 'verde' ? 'bg-emerald-500'  : c === 'amarelo' ? 'bg-amber-400'  : 'bg-red-500'
                const txt    = c === 'verde' ? 'text-emerald-600': c === 'amarelo' ? 'text-amber-600': 'text-red-500'
                return (
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
                    className={`w-full bg-white border border-slate-100 border-l-4 ${border} rounded-xl overflow-hidden shadow-sm active:shadow-none active:bg-slate-50 transition-all text-left`}
                  >
                    <div className="h-1 bg-slate-100">
                      <div className={`h-full ${bar}`} style={{ width: `${Math.min(rca.pct, 100)}%` }} />
                    </div>
                    <div className="px-4 py-3 flex items-center gap-3">
                      <div className="flex-1 min-w-0">
                        <p className="text-xs text-slate-400">{rca.cod_rca}</p>
                        <p className="font-semibold text-slate-800 text-base leading-tight truncate">{rca.nome_rca}</p>
                        <p className="text-xs text-slate-500 mt-0.5">{fmtBRL(rca.vl_corrente)} atual</p>
                        {rca.qtd_abaixo > 0 && (
                          <div className="mt-1.5 inline-flex items-center gap-1 bg-amber-50 text-amber-700 border border-amber-200 rounded-full px-2.5 py-0.5 text-xs font-semibold">
                            ⚠ {rca.qtd_abaixo} de {rca.qtd_fornec} abaixo
                          </div>
                        )}
                      </div>
                      <p className={`text-2xl font-bold shrink-0 ${txt}`}>
                        {rca.pct.toFixed(0)}<span className="text-sm font-normal text-slate-400">%</span>
                      </p>
                    </div>
                  </button>
                )
              })}
            </div>
          </>
        )}

        {isFetching && (
          <p className="text-center text-xs text-slate-400 py-2 animate-pulse">Atualizando...</p>
        )}
      </div>
    </FarolMobileShell>
  )
}
