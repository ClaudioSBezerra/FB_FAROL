import { ReactNode, createContext, useContext } from 'react'

// Quando embedded=true, os componentes Farol assumem que estão renderizados
// dentro de um layout web (AppLayout) e suprimem o cabeçalho mobile próprio
// e o paddings de safe-area.
const EmbeddedCtx = createContext<boolean>(false)
export const useEmbedded = () => useContext(EmbeddedCtx)

export function FarolMobileShell({ children, embedded = false }: { children: ReactNode; embedded?: boolean }) {
  if (embedded) {
    return (
      <EmbeddedCtx.Provider value={true}>
        <div className="bg-slate-50 text-black rounded-lg" style={{ fontSize: 16 }}>
          <div className="max-w-3xl mx-auto">{children}</div>
        </div>
      </EmbeddedCtx.Provider>
    )
  }
  return (
    <EmbeddedCtx.Provider value={false}>
      <div
        className="min-h-screen bg-slate-50 text-black"
        style={{
          fontSize: '18px',
          paddingBottom: 'env(safe-area-inset-bottom)',
          paddingTop: 'env(safe-area-inset-top)',
        }}
      >
        <div className="max-w-md mx-auto">{children}</div>
      </div>
    </EmbeddedCtx.Provider>
  )
}

export function FarolHeader({ title, subtitle, onBack }: { title: string; subtitle?: string; onBack?: () => void }) {
  const embedded = useEmbedded()
  if (embedded) {
    // No web layout o header global do AppLayout já existe; aqui só o botão voltar (se houver)
    if (!onBack && !subtitle) return null
    return (
      <div className="flex items-center gap-3 px-4 py-2 border-b bg-white">
        {onBack && (
          <button
            onClick={onBack}
            className="flex items-center justify-center bg-slate-100 hover:bg-slate-200 rounded-md text-slate-700"
            style={{ width: 36, height: 36 }}
            aria-label="Voltar"
          >
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="m15 18-6-6 6-6"/>
            </svg>
          </button>
        )}
        {subtitle && <span className="text-sm text-slate-600">{subtitle}</span>}
      </div>
    )
  }
  return (
    <header className="sticky top-0 z-10 bg-[#003366] text-white shadow">
      <div className="flex items-center px-4 py-4 gap-3" style={{ minHeight: 64 }}>
        {onBack && (
          <button
            onClick={onBack}
            className="flex items-center justify-center bg-white/10 hover:bg-white/20 active:bg-white/30 rounded-lg"
            style={{ width: 48, height: 48 }}
            aria-label="Voltar"
          >
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="m15 18-6-6 6-6"/>
            </svg>
          </button>
        )}
        <div className="flex-1 min-w-0">
          <h1 className="text-xl font-bold truncate">{title}</h1>
          {subtitle && <p className="text-sm opacity-80 truncate">{subtitle}</p>}
        </div>
      </div>
    </header>
  )
}
