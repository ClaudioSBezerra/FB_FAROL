import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { FarolMobileShell, FarolHeader } from './FarolMobile'
import { Semaforo, type Cor } from '@/components/farol/Semaforo'

interface FornecItem {
  cod_fornec: string
  fornecedor: string
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
interface ResumoOut {
  pct: number
  cor: Cor
  vl_anterior: number
  vl_corrente: number
}
interface RcaResp {
  cod_rca: number
  nome_rca: string
  cod_supervisor: number
  nome_supervisor: string
  periodo: PeriodoOut | null
  resumo: ResumoOut
  fornecedores: FornecItem[]
}

function fmtBRL(v: number): string {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}

function fmtPct(pct: number, ant: number): string {
  if (ant === 0) return '—'
  const cresc = pct - 100
  return (cresc >= 0 ? '+' : '') + cresc.toFixed(0) + '%'
}

export default function FarolRcaDetail() {
  const { cod, codRca } = useParams<{ cod: string; codRca: string }>()
  const [search] = useSearchParams()
  const navigate = useNavigate()

  const { data, isError } = useQuery<RcaResp>({
    queryKey: ['farol-rca', cod, codRca, search.toString()],
    queryFn: () => {
      const url = new URL(`/api/farol/rca/${codRca}`, window.location.origin)
      url.searchParams.set('cod_supervisor', cod ?? '')
      const tp = search.get('tipo_periodo')
      if (tp)                     url.searchParams.set('tipo_periodo', tp)
      if (search.get('ano'))         url.searchParams.set('ano',         search.get('ano')!)
      if (search.get('periodo_seq')) url.searchParams.set('periodo_seq', search.get('periodo_seq')!)
      return fetch(url.toString()).then(r => {
        if (!r.ok) throw new Error('falha ao carregar')
        return r.json()
      })
    },
    enabled: !!cod && !!codRca,
  })

  const goBack = () => {
    const params = new URLSearchParams()
    const tp = search.get('tipo_periodo')
    if (tp)                        params.set('tipo_periodo', tp)
    if (search.get('ano'))         params.set('ano', search.get('ano')!)
    if (search.get('periodo_seq')) params.set('periodo_seq', search.get('periodo_seq')!)
    navigate(`/m/${cod}${params.toString() ? '?' + params.toString() : ''}`)
  }

  if (isError) {
    return (
      <FarolMobileShell>
        <FarolHeader title="Erro" onBack={goBack} />
        <div className="p-6 text-center">
          <p className="text-lg mb-4">Não foi possível carregar os dados do RCA.</p>
          <button
            onClick={goBack}
            className="bg-[#003366] text-white rounded-lg w-full font-semibold"
            style={{ minHeight: 56, fontSize: 18 }}
          >
            Voltar
          </button>
        </div>
      </FarolMobileShell>
    )
  }

  if (!data) {
    return (
      <FarolMobileShell>
        <FarolHeader title="Carregando..." onBack={goBack} />
        <div className="p-6 text-center text-slate-600">Aguarde...</div>
      </FarolMobileShell>
    )
  }

  const semDados = !data.periodo || data.fornecedores.length === 0

  return (
    <FarolMobileShell>
      <FarolHeader title={`RCA ${data.cod_rca}`} subtitle={data.periodo?.label} onBack={goBack} />

      <div className="p-4 space-y-4">
        {/* Identificação */}
        <div>
          <h2 className="font-bold leading-tight" style={{ fontSize: 24 }}>
            {data.nome_rca}
          </h2>
          <p className="text-sm text-slate-600 mt-1">
            Sup. {data.cod_supervisor} — {data.nome_supervisor}
          </p>
        </div>

        {/* Card resumo */}
        {!semDados && (
          <div className="bg-white border-2 border-slate-200 rounded-xl p-5 shadow-sm">
            <div className="flex items-center gap-4 mb-4">
              <Semaforo cor={data.resumo.cor} size="lg" />
              <div>
                <p className="text-xs text-slate-500 uppercase tracking-wide">Realização</p>
                <p className="font-bold leading-none" style={{ fontSize: 36 }}>
                  {data.resumo.pct.toFixed(0)}%
                </p>
              </div>
            </div>
            <div className="space-y-1 text-base">
              <div className="flex justify-between">
                <span className="text-slate-600">Anterior:</span>
                <span className="font-semibold">{fmtBRL(data.resumo.vl_anterior)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-slate-600">Atual:</span>
                <span className="font-semibold">{fmtBRL(data.resumo.vl_corrente)}</span>
              </div>
              <div className="flex justify-between pt-1 border-t border-slate-100">
                <span className="text-slate-600">Crescimento:</span>
                <span className="font-bold">{fmtPct(data.resumo.pct, data.resumo.vl_anterior)}</span>
              </div>
            </div>
          </div>
        )}

        {/* Lista de fornecedores */}
        {semDados ? (
          <div className="bg-white border-2 border-dashed border-slate-300 rounded-xl p-8 text-center text-slate-500">
            <p style={{ fontSize: 18 }}>Sem objetivos para este RCA no período.</p>
          </div>
        ) : (
          <>
            <div className="flex items-baseline justify-between pt-2">
              <h3 className="font-bold" style={{ fontSize: 18 }}>Fornecedores</h3>
              <span className="text-sm text-slate-500">({data.fornecedores.length})</span>
            </div>

            <div className="space-y-3">
              {data.fornecedores.map(f => (
                <div
                  key={f.cod_fornec}
                  className="bg-white border-2 border-slate-200 rounded-xl p-4 space-y-2"
                >
                  <div className="flex items-center gap-3">
                    <Semaforo cor={f.cor} size="sm" />
                    <p className="font-semibold leading-tight flex-1 min-w-0 truncate" style={{ fontSize: 17 }}>
                      {f.fornecedor || f.cod_fornec}
                    </p>
                    <span className="font-bold" style={{ fontSize: 18 }}>{f.pct.toFixed(0)}%</span>
                  </div>
                  <div className="grid grid-cols-2 gap-2 pt-2 border-t border-slate-100 text-sm">
                    <div>
                      <p className="text-slate-500 text-xs">Anterior</p>
                      <p className="font-semibold">{fmtBRL(f.vl_anterior)}</p>
                    </div>
                    <div>
                      <p className="text-slate-500 text-xs">Atual</p>
                      <p className="font-semibold">{fmtBRL(f.vl_corrente)}</p>
                    </div>
                  </div>
                  <div className="text-sm pt-1">
                    <span className="text-slate-600">Crescimento: </span>
                    <span className={`font-bold ${
                      f.cor === 'verde' ? 'text-green-600' :
                      f.cor === 'amarelo' ? 'text-yellow-700' :
                      'text-red-600'
                    }`}>
                      {fmtPct(f.pct, f.vl_anterior)}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </FarolMobileShell>
  )
}
