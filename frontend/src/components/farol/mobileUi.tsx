import type { ReactNode } from 'react'
import { Check, X as XIcon } from 'lucide-react'

// Peças visuais do mobile (link do ION VENDAS) compartilhadas entre o painel de Objetivos da
// Unilever e o Segmento Foco da Reckitt — mesmo desenho nos dois.

// ChipRow — seleção por toque (pedido do Claudio 11/09/2026: "se possível ele tocar em vez de
// filtrar, tem que ser seleção touch") — substitui os <select> nativos, ruins de mirar com o dedo.
// Com só 1 opção nem chip mostra (nada pra escolher), a menos que `sempre`. Alvo de toque generoso
// (~44px de altura) e scroll horizontal quando a lista não cabe na tela.
export function ChipRow<T extends string>({ label, options, value, onChange, sempre }: {
  label: ReactNode
  options: Array<{ value: T; label: string }>
  value: T
  onChange: (v: T) => void
  // sempre — mostra mesmo com 1 opção só (Período da Numérica, Heverton 08/10/2026).
  sempre?: boolean
}) {
  if (!sempre && options.length <= 1) return null
  return (
    <div>
      <div className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground px-0.5 mb-1.5 flex items-center gap-2">{label}</div>
      <div className="flex gap-2 overflow-x-auto pb-1 -mx-4 px-4 scrollbar-none">
        {options.map(opt => (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            className={`shrink-0 whitespace-nowrap rounded-full border px-4 py-3 text-sm font-medium leading-none transition-colors active:scale-[0.97] ${
              value === opt.value
                ? 'border-slate-900 bg-slate-900 text-white'
                : 'border-slate-300 bg-white text-slate-700'
            }`}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </div>
  )
}

// StatusIcon — pedido do Heverton 25/09/2026: símbolo (ticado verde / X vermelho) em vez de texto —
// mais rápido de ler numa lista longa no celular.
export function StatusIcon({ atingiu, size = 'w-4 h-4' }: { atingiu: boolean; size?: string }) {
  return atingiu
    ? <Check className={`${size} shrink-0 text-green-700`} strokeWidth={3} aria-label="Atingiu" />
    : <XIcon className={`${size} shrink-0 text-red-700`} strokeWidth={3} aria-label="Não atingiu" />
}

// Rede/Cliente pelo nome (fantasia > razão), com o código como fallback só se não houver nome nenhum.
export const nomeOuCodigo = (fantasia: string, razao: string, codigo: string) => fantasia || razao || codigo
