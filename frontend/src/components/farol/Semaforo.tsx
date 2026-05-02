type Cor = 'verde' | 'amarelo' | 'vermelho'

const STYLES: Record<Cor, { bg: string; shadow: string; symbol: string }> = {
  verde:    { bg: 'bg-emerald-500', shadow: 'shadow-emerald-200', symbol: '✓' },
  amarelo:  { bg: 'bg-amber-400',   shadow: 'shadow-amber-200',   symbol: '!' },
  vermelho: { bg: 'bg-red-500',     shadow: 'shadow-red-200',     symbol: '✕' },
}

const SIZES = {
  sm: { outer: 32, inner: 32, font: 14 },
  md: { outer: 48, inner: 48, font: 20 },
  lg: { outer: 64, inner: 64, font: 28 },
}

export function Semaforo({ cor, size = 'md' }: { cor: Cor; size?: 'sm' | 'md' | 'lg' }) {
  const { bg, shadow, symbol } = STYLES[cor]
  const { outer, font } = SIZES[size]
  return (
    <div
      className={`${bg} rounded-full flex items-center justify-center text-white font-bold shrink-0 shadow-lg ${shadow}`}
      style={{ width: outer, height: outer, fontSize: font }}
      aria-label={`Farol ${cor}`}
    >
      {symbol}
    </div>
  )
}

export type { Cor }
