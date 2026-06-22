// SortToggle — Botão duplo "Valor" / "Meta" pra ordenar GRIDs do Farol.
//
// O "Farol" historicamente ordenava por COR (vermelho topo / verde fundo) +
// valor desc dentro da cor — pra destacar quem NÃO bateu meta. Mas isso
// escondia os produtos/clientes de MAIOR venda absoluta quando estavam em
// crescimento. O toggle deixa o usuário escolher: ranking puro de valor
// (padrão) ou ranking pelo farol (alertas no topo).
//
// Persistência: localStorage por chave (ex: 'farol.sort.executivo').

import { TrendingUp, AlertTriangle } from 'lucide-react'
import { cn } from '@/lib/utils'

export type SortMode = 'valor' | 'meta'

export function SortToggle({
  value, onChange, className,
}: {
  value: SortMode
  onChange: (m: SortMode) => void
  className?: string
}) {
  return (
    <div
      className={cn(
        'flex rounded-lg border border-slate-200 overflow-hidden bg-white shadow-sm shrink-0',
        className,
      )}
      title="Critério de ordenação da lista"
    >
      <button
        type="button"
        onClick={() => onChange('valor')}
        className={cn(
          'flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium transition-colors',
          value === 'valor' ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50',
        )}
      >
        <TrendingUp className="h-3.5 w-3.5" />
        Valor
      </button>
      <button
        type="button"
        onClick={() => onChange('meta')}
        className={cn(
          'flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium transition-colors',
          value === 'meta' ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50',
        )}
      >
        <AlertTriangle className="h-3.5 w-3.5" />
        Meta
      </button>
    </div>
  )
}

// Hook que reordena cards conforme o modo escolhido + persiste a preferência.
//   - "valor": valor_atual desc (puro ranking de venda)
//   - "meta":  cor (vermelho topo) + valor_atual desc dentro da cor (= ordem
//              original do backend, equivale ao Farol clássico)
import { useState, useMemo, useCallback } from 'react'

interface SortableCard {
  valor_atual: number
  cor: string
}

export function useSortedCards<T extends SortableCard>(
  cards: T[],
  storageKey: string,
  defaultMode: SortMode = 'valor',
): { sorted: T[]; mode: SortMode; setMode: (m: SortMode) => void } {
  const [mode, setModeState] = useState<SortMode>(() => {
    try {
      const stored = localStorage.getItem(storageKey)
      if (stored === 'valor' || stored === 'meta') return stored
    } catch { /* localStorage indisponível (SSR/privacy mode) — usa default */ }
    return defaultMode
  })

  const setMode = useCallback((m: SortMode) => {
    setModeState(m)
    try { localStorage.setItem(storageKey, m) } catch { /* ignora */ }
  }, [storageKey])

  const sorted = useMemo(() => {
    const arr = [...cards]
    if (mode === 'valor') {
      arr.sort((a, b) => b.valor_atual - a.valor_atual)
    } else {
      // 'meta': cor primeiro (vermelho > amarelo > verde), depois valor desc
      const corRank: Record<string, number> = { vermelho: 0, amarelo: 1, verde: 2 }
      arr.sort((a, b) => {
        const ra = corRank[a.cor] ?? 99
        const rb = corRank[b.cor] ?? 99
        if (ra !== rb) return ra - rb
        return b.valor_atual - a.valor_atual
      })
    }
    return arr
  }, [cards, mode])

  return { sorted, mode, setMode }
}
