import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'
import {
  CardVenda, KPIBar, Breadcrumb,
  parsePeriodo, fmtMesAno,
  type CardsResponse, type CardItem, type DrillStep,
} from './FarolV2Dashboard'

// Painel público do ION VENDAS — aberto sem login via link parametrizado
// (/m/CNPJ/SUP/cod ou /m/CNPJ/RCA/cod). Reusa os cards do painel principal,
// já escopado ao supervisor ou RCA. Mesmas views novas, mesma nomenclatura.

export default function FarolPublicPanel() {
  const params = useParams<{ cnpj?: string; cod?: string; codRca?: string }>()

  const isRca    = !!params.codRca
  const scope    = isRca ? 'rca' : 'sup'
  const scopeCod = (isRca ? params.codRca : params.cod) || ''
  // No formato /m/:cnpj/sup/:cod o CNPJ vem em :cnpj;
  // no /m/:cod/rca/:codRca (ION: CNPJ/RCA/cod) o CNPJ vem em :cod.
  const cnpj = (params.cnpj || (isRca ? params.cod : '') || '').replace(/\D/g, '')

  const [compMode, setCompMode]   = useState('yoy')
  const [userDrill, setUserDrill] = useState<DrillStep[]>([])
  const [refAno, setRefAno]       = useState(0)
  const [refMes, setRefMes]       = useState(0)

  const drillParam = JSON.stringify(userDrill)
  const { data, isLoading, error } = useQuery<CardsResponse>({
    queryKey: ['farol-public', cnpj, scope, scopeCod, compMode, refAno, refMes, drillParam],
    queryFn: async () => {
      const p = new URLSearchParams({
        cnpj, scope, cod: scopeCod, comp_mode: compMode,
        ...(refAno > 0 && { ref_ano: String(refAno) }),
        ...(refMes > 0 && { ref_mes: String(refMes) }),
        ...(userDrill.length > 0 && { drill: drillParam }),
      })
      const r = await fetch(`/api/v2/farol/public/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar painel')
      return r.json()
    },
    enabled: !!cnpj && !!scopeCod,
    staleTime: 2 * 60_000, gcTime: 5 * 60_000, refetchOnWindowFocus: false,
  })

  const autoRef = useCallback((d: CardsResponse) => {
    if (refAno === 0 && d.periodo.ref_ano) {
      setRefAno(d.periodo.ref_ano)
      setRefMes(d.periodo.ref_mes)
    }
  }, [refAno])
  if (data && refAno === 0) autoRef(data)

  const baseLen     = scope === 'rca' ? 2 : 1
  const scopeStep   = data?.drill_path?.[baseLen - 1]
  const scopeLabel  = scope === 'rca' ? 'RCA' : 'Supervisor'
  const scopeNome   = scopeStep?.label || scopeCod
  const periodos    = data?.periodos ?? []

  const handleDrill = (card: CardItem) => {
    if (card.level === 'cod_prod') return // Produto é o nível folha
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
    <div className="min-h-screen bg-slate-50">
      {/* Cabeçalho do escopo */}
      <div className="bg-white border-b border-slate-200 px-4 py-3 sticky top-0 z-10">
        <p className="text-xs text-slate-400 font-medium">{scopeLabel}</p>
        <h1 className="text-lg font-bold text-slate-800 leading-tight truncate">{scopeNome}</h1>
      </div>

      <div className="p-4">
        {/* Controles */}
        <div className="flex flex-wrap items-center gap-2 mb-4">
          <div className="flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0">
            {[
              { id: 'yoy', label: 'Ano a Ano' },
              { id: 'ytd', label: 'Projeção Anual' },
              { id: 'mom', label: 'Mês a Mês' },
            ].map(m => (
              <button
                key={m.id}
                onClick={() => { setCompMode(m.id); setUserDrill([]) }}
                className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                  compMode === m.id ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
                }`}
              >
                {m.label}
              </button>
            ))}
          </div>

          {periodos.length > 0 && (
            <select
              value={refAno > 0 ? `${refAno}-${String(refMes).padStart(2, '0')}` : ''}
              onChange={e => {
                const pp = parsePeriodo(e.target.value)
                setRefAno(pp.ano); setRefMes(pp.mes); setUserDrill([])
              }}
              className="h-8 rounded-lg border border-slate-200 bg-white px-2 text-xs text-slate-700 shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30"
            >
              {periodos.map(p => {
                const { ano, mes } = parsePeriodo(p)
                return <option key={p} value={p}>{fmtMesAno(ano, mes)}</option>
              })}
            </select>
          )}
        </div>

        {data?.kpi && data.kpi.total_atual > 0 && (
          <KPIBar kpi={data.kpi} periodo={data.periodo} />
        )}

        <Breadcrumb drillPath={userDrill} onNavigate={handleBreadcrumb} />

        {data && (
          <p className="text-xs text-slate-400 mb-3">
            Exibindo por{' '}
            <span className="font-medium text-slate-600">{data.next_level_label}</span>
            {' '}· {data.cards.length} itens
          </p>
        )}

        {isLoading && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {[...Array(6)].map((_, i) => (
              <div key={i} className="bg-white border border-slate-100 rounded-xl h-40 animate-pulse" />
            ))}
          </div>
        )}

        {error && (
          <div className="bg-red-50 border border-red-200 rounded-xl p-6 text-red-700 text-sm">
            <p className="font-semibold mb-1">Erro ao carregar painel</p>
            <p>{(error as Error).message}</p>
          </div>
        )}

        {!isLoading && !error && data?.cards.length === 0 && (
          <div className="bg-white border border-dashed border-slate-200 rounded-xl p-12 text-center text-slate-400">
            <p className="text-4xl mb-3">📊</p>
            <p className="text-sm font-medium text-slate-500">Nenhum dado no período</p>
          </div>
        )}

        {!isLoading && !error && data && data.cards.length > 0 && (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {data.cards.map(card => (
              <CardVenda key={card.key} card={card} onClick={() => handleDrill(card)} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
