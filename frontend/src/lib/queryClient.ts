import { QueryClient } from '@tanstack/react-query'

// QueryClient compartilhado entre App e AuthContext (para prefetch pós-login).
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Mantém dados frescos por 5 min — reduz refetch em navegação rápida.
      staleTime: 5 * 60_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
    },
  },
})

// Helper: dispara em background as queries do dashboard executivo padrão.
// Chamado após login bem-sucedido. Os dados ficam no cache do React Query
// quando o componente FarolExecutivo monta, o useQuery encontra cache pronto.
export function prefetchDashboardExecutivo() {
  const today = new Date()
  const y = today.getUTCFullYear()
  const m = today.getUTCMonth() + 1
  const d = today.getUTCDate()
  const ymd = (yy: number, mm: number, dd: number) =>
    `${yy}-${String(mm).padStart(2, '0')}-${String(dd).padStart(2, '0')}`

  // Preset YTD (padrão do FarolExecutivo): Jan-hoje × ano anterior completo
  const refIni  = ymd(y, 1, 1)
  const refFim  = ymd(y, m, d)
  const compIni = ymd(y - 1, 1, 1)
  const compFim = ymd(y - 1, 12, 31)

  const view  = 'V01'
  const fluxo = 'faturado'

  // 1. Cards do dashboard (a query mais pesada)
  queryClient.prefetchQuery({
    queryKey: ['farol-v2-cards', view, fluxo, refIni, refFim, compIni, compFim, '[]', '{}'],
    queryFn: async () => {
      const p = new URLSearchParams({ view, fluxo, ref_inicio: refIni, ref_fim: refFim, comp_inicio: compIni, comp_fim: compFim })
      const r = await fetch(`/api/v2/farol/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dados')
      return r.json()
    },
    staleTime: 5 * 60_000,
  }).catch(() => { /* prefetch falha silenciosa — não bloqueia login */ })

  // 2. Períodos disponíveis (usado para popular o seletor)
  queryClient.prefetchQuery({
    queryKey: ['farol-v2-periodos'],
    queryFn: async () => {
      const r = await fetch('/api/v2/farol/cards?view=V01')
      if (!r.ok) throw new Error()
      const d = await r.json()
      return { ref_ano: d.periodo?.ref_ano, ref_mes: d.periodo?.ref_mes, periodos: d.periodos }
    },
    staleTime: 60 * 60_000,
  }).catch(() => {})

  // 3. Dimensões para filtros (UF, supervisor, RCA, etc.)
  queryClient.prefetchQuery({
    queryKey: ['farol-v2-dims', fluxo, refIni, refFim],
    queryFn: async () => {
      const p = new URLSearchParams({ fluxo, ref_inicio: refIni, ref_fim: refFim })
      const r = await fetch(`/api/v2/farol/dims?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dimensões')
      return r.json()
    },
    staleTime: 5 * 60_000,
  }).catch(() => {})
}
