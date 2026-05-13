import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { FarolMobileShell, FarolHeader } from './FarolMobile'
import { Semaforo, type Cor } from '@/components/farol/Semaforo'
import { useAuth } from '@/contexts/AuthContext'

interface RcaItem {
  cod_rca: number; nome_rca: string
  pct: number; cor: Cor
  vl_anterior: number; vl_corrente: number
}
interface PeriodoOut { tipo: string; ano: number; seq: number; label: string }
interface Resumo { pct: number; cor: Cor; vl_anterior: number; vl_corrente: number }
interface Resp {
  cod_supervisor: number; nome_supervisor: string
  cod_fornec: string; fornecedor: string
  periodo: PeriodoOut | null
  resumo: Resumo
  rcas: RcaItem[]
}

function fmtBRL(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}

const CORS_BAR: Record<Cor, string> = {
  verde: 'bg-emerald-500', amarelo: 'bg-amber-400', vermelho: 'bg-red-500',
}
const CORS_BORDER: Record<Cor, string> = {
  verde: 'border-l-emerald-500', amarelo: 'border-l-amber-400', vermelho: 'border-l-red-500',
}
const CORS_TEXT: Record<Cor, string> = {
  verde: 'text-emerald-600', amarelo: 'text-amber-600', vermelho: 'text-red-500',
}

export default function FarolFornecRcas({ embedded = false }: { embedded?: boolean } = {}) {
  // Rotas:
  //   /m/:cnpj/sup/:cod/forn/:codFornec   (mobile com CNPJ)
  //   /m/:cod/forn/:codFornec             (mobile legado sem CNPJ)
  //   /farol/sup/:cod/forn/:codFornec     (web embedded — CNPJ do AuthContext)
  const { cod, codFornec, cnpj: cnpjFromUrl } = useParams<{ cod: string; codFornec: string; cnpj?: string }>()
  const { cnpj: cnpjFromAuth } = useAuth()
  const cnpj = cnpjFromUrl || (embedded ? (cnpjFromAuth || '').replace(/\D/g, '') : undefined)
  const [search] = useSearchParams()
  const navigate = useNavigate()

  const { data, isError } = useQuery<Resp>({
    queryKey: ['farol-forn-rcas', cnpj, cod, codFornec, search.toString()],
    queryFn: () => {
      const url = new URL(`/api/farol/forn-rcas/${cod}`, window.location.origin)
      if (cnpj) url.searchParams.set('cnpj', cnpj)
      if (codFornec) url.searchParams.set('cod_fornec', codFornec)
      const tp = search.get('tipo_periodo')
      if (tp)                        url.searchParams.set('tipo_periodo', tp)
      if (search.get('ano'))         url.searchParams.set('ano',         search.get('ano')!)
      if (search.get('periodo_seq')) url.searchParams.set('periodo_seq', search.get('periodo_seq')!)
      return fetch(url.toString()).then(r => {
        if (!r.ok) throw new Error('falha')
        return r.json()
      })
    },
    enabled: !!cod && !!codFornec,
    staleTime: 2 * 60_000, gcTime: 10 * 60_000, refetchOnWindowFocus: false,
  })

  const goBack = () => {
    const params = new URLSearchParams()
    const tp = search.get('tipo_periodo')
    if (tp)                        params.set('tipo_periodo', tp)
    if (search.get('ano'))         params.set('ano',         search.get('ano')!)
    if (search.get('periodo_seq')) params.set('periodo_seq', search.get('periodo_seq')!)
    const base = embedded
      ? `/farol/sup/${cod}`
      : cnpjFromUrl
        ? `/m/${cnpjFromUrl}/sup/${cod}`
        : `/m/${cod}`
    navigate(`${base}${params.toString() ? '?' + params.toString() : ''}`)
  }

  if (isError) {
    return (
      <FarolMobileShell embedded={embedded}>
        <FarolHeader title="Erro" onBack={goBack} />
        <div className="p-6 text-center">
          <p className="text-lg mb-4">Não foi possível carregar os dados.</p>
          <button onClick={goBack} className="bg-[#003366] text-white rounded-lg w-full font-semibold" style={{ minHeight: 56, fontSize: 18 }}>Voltar</button>
        </div>
      </FarolMobileShell>
    )
  }

  if (!data) {
    return (
      <FarolMobileShell embedded={embedded}>
        <FarolHeader title="Carregando..." onBack={goBack} />
        <div className="p-6 text-center text-slate-600">Aguarde...</div>
      </FarolMobileShell>
    )
  }

  const c = data.resumo.cor
  const semDados = !data.periodo || data.rcas.length === 0

  return (
    <FarolMobileShell embedded={embedded}>
      <FarolHeader title={`Fornecedor ${data.cod_fornec}`} subtitle={data.periodo?.label} onBack={goBack} />

      <div className="p-4 space-y-4">
        {/* Identificação */}
        <div>
          <h2 className="text-2xl font-bold text-slate-800 leading-tight">{data.fornecedor || '—'}</h2>
          <p className="text-sm text-slate-500 mt-1">
            Sup. {data.cod_supervisor} — {data.nome_supervisor}
          </p>
        </div>

        {/* Resumo */}
        {!semDados && (
          <div className="bg-white border border-slate-100 rounded-xl shadow-sm overflow-hidden">
            <div className="h-1.5 bg-slate-100">
              <div className={`h-full ${CORS_BAR[c]} transition-all`} style={{ width: `${Math.min(data.resumo.pct, 100)}%` }} />
            </div>
            <div className="p-5 flex items-center gap-4">
              <Semaforo cor={c} size="lg" />
              <div className="flex-1">
                <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">Total Fornecedor</p>
                <p className={`text-4xl font-bold leading-none mt-1 ${CORS_TEXT[c]}`}>
                  {data.resumo.pct.toFixed(0)}<span className="text-lg text-slate-400 font-normal ml-1">%</span>
                </p>
                <p className="text-xs text-slate-500 mt-1.5">
                  {fmtBRL(data.resumo.vl_anterior)} → {fmtBRL(data.resumo.vl_corrente)}
                </p>
              </div>
            </div>
          </div>
        )}

        {/* Lista de RCAs */}
        {semDados ? (
          <div className="bg-white border-2 border-dashed border-slate-200 rounded-xl p-8 text-center text-slate-400">
            <p className="text-base">Sem RCAs para este fornecedor no período.</p>
          </div>
        ) : (
          <>
            <div className="flex items-center justify-between">
              <h3 className="text-base font-semibold text-slate-700">RCAs</h3>
              <span className="text-xs text-slate-400 bg-slate-100 px-2 py-0.5 rounded-full">{data.rcas.length}</span>
            </div>
            <div className="space-y-2">
              {[...data.rcas].sort((a,b) => b.pct - a.pct).map(rca => (
                <div
                  key={rca.cod_rca}
                  className={`bg-white border border-slate-100 border-l-4 ${CORS_BORDER[rca.cor]} rounded-xl overflow-hidden shadow-sm`}
                >
                  <div className="h-1 bg-slate-100">
                    <div className={`h-full ${CORS_BAR[rca.cor]}`} style={{ width: `${Math.min(rca.pct, 100)}%` }} />
                  </div>
                  <div className="px-4 py-3 flex items-center gap-3">
                    <div className="flex-1 min-w-0">
                      <p className="font-bold text-slate-700 text-base">{rca.cod_rca}</p>
                      <p className="font-semibold text-slate-800 text-base leading-tight truncate">{rca.nome_rca}</p>
                      <div className="flex gap-3 mt-1 text-xs">
                        <span><span className="text-slate-400">Ant.</span> <span className="text-slate-500 font-medium">{fmtBRL(rca.vl_anterior)}</span></span>
                        <span><span className="text-slate-400">Atual</span> <span className="text-slate-800 font-semibold">{fmtBRL(rca.vl_corrente)}</span></span>
                      </div>
                    </div>
                    <p className={`text-2xl font-bold shrink-0 ${CORS_TEXT[rca.cor]}`}>
                      {rca.pct.toFixed(0)}<span className="text-sm font-normal text-slate-400">%</span>
                    </p>
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
