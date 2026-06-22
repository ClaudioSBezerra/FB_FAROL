// Sort de GRIDs do Farol — agora via setinhas clicáveis no header da tabela,
// não mais via botão Valor/Meta na toolbar (ficava perdido no meio).
//
// Uso:
//   const { sorted, sortState, setSort } = useSortedCards(
//     cards, 'farol.sort.executivo', { field: 'valor', direction: 'desc' }
//   )
//
//   <SortIndicator
//     active={sortState.field === 'valor'}
//     direction={sortState.direction}
//     onClick={() => setSort({ field: 'valor' })}
//   />

import { useState, useMemo, useCallback } from 'react'
import { ChevronUp, ChevronDown, ChevronsUpDown } from 'lucide-react'
import { cn } from '@/lib/utils'

export type SortField = 'valor' | 'pct'
export type SortDirection = 'asc' | 'desc'
export interface SortState {
  field: SortField
  direction: SortDirection
}

interface SortableCard {
  valor_atual: number
  pct: number
  cor: string
}

// Hook que reordena cards conforme campo/direção + persiste preferência.
//   - field='valor': por venda atual
//   - field='pct':   por % atingimento (cor é honrada como tiebreaker interno)
// Click no campo ativo alterna direção; click em outro campo seta o novo campo
// na direção desc por padrão.
export function useSortedCards<T extends SortableCard>(
  cards: T[],
  storageKey: string,
  defaultState: SortState = { field: 'valor', direction: 'desc' },
): {
  sorted: T[]
  sortState: SortState
  setSort: (next: Partial<SortState> & { field: SortField }) => void
} {
  const [sortState, setSortState] = useState<SortState>(() => {
    try {
      const raw = localStorage.getItem(storageKey)
      if (raw) {
        const parsed = JSON.parse(raw) as SortState
        if (
          (parsed.field === 'valor' || parsed.field === 'pct') &&
          (parsed.direction === 'asc' || parsed.direction === 'desc')
        ) return parsed
      }
    } catch { /* localStorage indisponível ou JSON inválido — usa default */ }
    return defaultState
  })

  // Click no header ativo → alterna direção; click em outro → reseta pra desc.
  const setSort = useCallback((next: Partial<SortState> & { field: SortField }) => {
    setSortState(prev => {
      const newState: SortState =
        prev.field === next.field
          ? { field: prev.field, direction: prev.direction === 'desc' ? 'asc' : 'desc' }
          : { field: next.field, direction: next.direction ?? 'desc' }
      try { localStorage.setItem(storageKey, JSON.stringify(newState)) } catch { /* ignora */ }
      return newState
    })
  }, [storageKey])

  const sorted = useMemo(() => {
    const arr = [...cards]
    const sign = sortState.direction === 'desc' ? -1 : 1
    arr.sort((a, b) => {
      if (sortState.field === 'valor') {
        return sign * (a.valor_atual - b.valor_atual)
      }
      // 'pct': itens sem comparativo (pct=0 e valor>0 caem como "novos") vão
      // pro final em desc; pra evitar ranking artificial, ordena por valor desc
      // como tiebreaker entre pcts iguais.
      const diff = a.pct - b.pct
      if (diff !== 0) return sign * diff
      return -1 * (a.valor_atual - b.valor_atual)
    })
    return arr
  }, [cards, sortState])

  return { sorted, sortState, setSort }
}

// Setinha clicável pra cabeçalhos de coluna. Mostra ↕ quando inativo, ↑/↓
// quando ativo. Pode ser usada em qualquer cabeçalho de tabela.
export function SortIndicator({
  active, direction, onClick, className,
}: {
  active: boolean
  direction: SortDirection
  onClick: () => void
  className?: string
}) {
  const Icon = !active ? ChevronsUpDown : direction === 'desc' ? ChevronDown : ChevronUp
  return (
    <button
      type="button"
      onClick={(e) => { e.stopPropagation(); onClick() }}
      className={cn(
        'inline-flex items-center justify-center align-middle ml-1 p-0.5 rounded transition-colors',
        active ? 'text-sky-600 bg-sky-50' : 'text-slate-400 hover:text-slate-600 hover:bg-slate-200/60',
        className,
      )}
      title={active
        ? (direction === 'desc' ? 'Maior → menor (clique pra inverter)' : 'Menor → maior (clique pra inverter)')
        : 'Clique pra ordenar por esta coluna'}
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  )
}
