type Cor = 'verde' | 'amarelo' | 'vermelho'

const STYLES: Record<Cor, { bg: string; ring: string; icon: string }> = {
  verde:     { bg: 'bg-green-500',  ring: 'ring-green-200',  icon: '✓' },
  amarelo:   { bg: 'bg-yellow-400', ring: 'ring-yellow-200', icon: '!' },
  vermelho:  { bg: 'bg-red-500',    ring: 'ring-red-200',    icon: '✕' },
}

export function Semaforo({ cor, size = 'md' }: { cor: Cor; size?: 'sm' | 'md' | 'lg' }) {
  const px = size === 'lg' ? 64 : size === 'sm' ? 32 : 48
  const fontSize = size === 'lg' ? 32 : size === 'sm' ? 16 : 24
  const s = STYLES[cor]
  return (
    <div
      className={`${s.bg} ${s.ring} rounded-full flex items-center justify-center text-white font-bold ring-4 shrink-0`}
      style={{ width: px, height: px, fontSize }}
      aria-label={`Farol ${cor}`}
    >
      {s.icon}
    </div>
  )
}

export type { Cor }
