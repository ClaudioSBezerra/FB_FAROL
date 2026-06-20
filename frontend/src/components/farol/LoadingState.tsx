// LoadingState — placeholder visual claro enquanto dados são buscados.
// Mostra spinner + mensagem explícita para o usuário não achar que a tela
// travou (especialmente no 1º acesso após login, quando MVs estão frias).

interface LoadingStateProps {
  message?: string
  hint?: string
  cards?: number
}

export function LoadingState({ message = 'Carregando dados, aguarde...', hint, cards = 6 }: LoadingStateProps) {
  return (
    <div className="space-y-4">
      {/* Mensagem clara no topo */}
      <div className="flex items-center gap-3 px-1">
        <div className="w-5 h-5 rounded-full border-2 border-[#003366] border-t-transparent animate-spin shrink-0" />
        <p className="text-sm font-semibold text-slate-700">{message}</p>
      </div>
      {hint && <p className="text-xs text-slate-400 px-1">{hint}</p>}

      {/* Skeleton cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
        {[...Array(cards)].map((_, i) => (
          <div key={i} className="bg-slate-100 rounded-xl h-28 animate-pulse" style={{ animationDelay: `${i * 80}ms` }} />
        ))}
      </div>
    </div>
  )
}

// LoadingOverlay — versão fullscreen bloqueante, para login/refresh críticos.
export function LoadingOverlay({ message = 'Carregando dados, aguarde...' }: { message?: string }) {
  return (
    <div className="fixed inset-0 bg-white/80 backdrop-blur-sm z-50 flex items-center justify-center">
      <div className="text-center">
        <div className="w-12 h-12 rounded-full border-4 border-[#003366] border-t-transparent animate-spin mx-auto" />
        <p className="mt-4 text-base font-semibold text-slate-700">{message}</p>
        <p className="mt-1 text-xs text-slate-400">Isto pode levar alguns segundos no primeiro acesso do dia.</p>
      </div>
    </div>
  )
}
