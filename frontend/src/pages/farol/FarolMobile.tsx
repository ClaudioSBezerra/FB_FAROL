import { ReactNode } from 'react'

export function FarolMobileShell({ children }: { children: ReactNode }) {
  return (
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
  )
}

export function FarolHeader({ title, subtitle, onBack }: { title: string; subtitle?: string; onBack?: () => void }) {
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
