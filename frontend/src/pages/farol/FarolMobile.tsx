import { ReactNode, createContext, useContext } from 'react'

const EmbeddedCtx = createContext<boolean>(false)
export const useEmbedded = () => useContext(EmbeddedCtx)

export function FarolMobileShell({ children, embedded = false }: { children: ReactNode; embedded?: boolean }) {
  if (embedded) {
    return (
      <EmbeddedCtx.Provider value={true}>
        <div className="text-slate-800 max-w-3xl mx-auto">{children}</div>
      </EmbeddedCtx.Provider>
    )
  }
  return (
    <EmbeddedCtx.Provider value={false}>
      <div
        className="min-h-screen bg-slate-50 text-slate-800"
        style={{
          paddingBottom: 'env(safe-area-inset-bottom)',
          paddingTop: 'env(safe-area-inset-top)',
        }}
      >
        <div className="max-w-md mx-auto">{children}</div>
      </div>
    </EmbeddedCtx.Provider>
  )
}

export function FarolHeader({
  title, subtitle, onBack,
}: {
  title: string; subtitle?: string; onBack?: () => void
}) {
  const embedded = useEmbedded()

  if (embedded) {
    if (!onBack && !subtitle) return null
    return (
      <div className="flex items-center gap-3 px-4 py-3 border-b border-slate-100 bg-white">
        {onBack && (
          <button
            onClick={onBack}
            className="flex items-center justify-center w-8 h-8 rounded-lg bg-slate-100 hover:bg-slate-200 text-slate-600 transition-colors"
            aria-label="Voltar"
          >
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
              <path d="m15 18-6-6 6-6"/>
            </svg>
          </button>
        )}
        {subtitle && <span className="text-sm text-slate-500">{subtitle}</span>}
      </div>
    )
  }

  return (
    <header className="sticky top-0 z-10 bg-[#003366] shadow-lg">
      <div className="flex items-center px-4 gap-3" style={{ minHeight: 60, paddingTop: 'env(safe-area-inset-top)' }}>
        {onBack && (
          <button
            onClick={onBack}
            className="flex items-center justify-center bg-white/10 hover:bg-white/20 active:bg-white/30 rounded-xl transition-colors"
            style={{ width: 44, height: 44 }}
            aria-label="Voltar"
          >
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="white" strokeWidth="2.5">
              <path d="m15 18-6-6 6-6"/>
            </svg>
          </button>
        )}
        <div className="flex-1 min-w-0">
          <h1 className="text-base font-bold text-white truncate">{title}</h1>
          {subtitle && <p className="text-xs text-white/60 truncate mt-0.5">{subtitle}</p>}
        </div>
      </div>
    </header>
  )
}
