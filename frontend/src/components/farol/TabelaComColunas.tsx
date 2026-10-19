import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { Columns3 } from 'lucide-react'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Checkbox } from '@/components/ui/checkbox'

// Tabela com colunas ocultáveis, tipo Excel — pedido do Heverton 19/10/2026
// ("poder ocultar ou reexibir colunas, em todas as visões").
//
// Funciona lendo o cabeçalho que a própria tabela já renderiza (última linha
// do <thead>) e escondendo a coluna por CSS (nth-child) — assim as 7+ tabelas
// do painel ganham o recurso sem reescrever célula por célula. A 1ª coluna
// (o nome da linha) nunca esconde. A escolha fica salva por tabela no
// navegador (localStorage) pelo rótulo da coluna; colunas com rótulo repetido
// ("Status", "Status") viram "Status", "Status (2)".

const CHAVE = (id: string) => `farol-colunas-ocultas:${id}`

function lerOcultas(id: string): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(CHAVE(id)) ?? '[]')
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []
  } catch { return [] }
}

function lerRotulos(raiz: HTMLElement | null): string[] {
  const linhas = raiz?.querySelectorAll('thead tr')
  const ultima = linhas && linhas.length ? linhas[linhas.length - 1] : null
  if (!ultima) return []
  const vistos = new Map<string, number>()
  return Array.from(ultima.children).map(th => {
    const base = (th.textContent ?? '').trim() || 'Coluna'
    const n = (vistos.get(base) ?? 0) + 1
    vistos.set(base, n)
    return n === 1 ? base : `${base} (${n})`
  })
}

export default function TabelaComColunas({ id, className = '', children }: {
  id: string
  className?: string
  children: React.ReactNode
}) {
  const raiz = useRef<HTMLDivElement>(null)
  const [rotulos, setRotulos] = useState<string[]>([])
  const [ocultas, setOcultas] = useState<string[]>(() => lerOcultas(id))

  // Relê o cabeçalho a cada render (barato) e só atualiza o estado se mudou.
  useLayoutEffect(() => {
    const atual = lerRotulos(raiz.current)
    setRotulos(prev => (prev.length === atual.length && prev.every((r, i) => r === atual[i]) ? prev : atual))
  })

  useEffect(() => {
    try { localStorage.setItem(CHAVE(id), JSON.stringify(ocultas)) } catch { /* sem storage: só não lembra */ }
  }, [id, ocultas])

  const css = useMemo(() => {
    const regras: string[] = []
    rotulos.forEach((r, i) => {
      if (i > 0 && ocultas.includes(r)) regras.push(`[data-tcol="${id}"] tr > :nth-child(${i + 1}){display:none}`)
    })
    return regras.join('\n')
  }, [rotulos, ocultas, id])

  const alternar = (r: string) => setOcultas(o => (o.includes(r) ? o.filter(x => x !== r) : [...o, r]))
  const escondidas = rotulos.filter((r, i) => i > 0 && ocultas.includes(r)).length

  return (
    <div>
      {rotulos.length > 2 && (
        <div className="flex justify-end mb-1">
          <Popover>
            <PopoverTrigger asChild>
              <button
                type="button"
                className="inline-flex items-center gap-1.5 text-xs font-medium text-slate-600 hover:text-slate-900 border rounded-md px-2.5 py-1 bg-white"
              >
                <Columns3 className="h-3.5 w-3.5" />
                Colunas{escondidas > 0 && <span className="text-amber-700">({escondidas} oculta{escondidas > 1 ? 's' : ''})</span>}
              </button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-64 p-2 normal-case">
              <div className="text-[11px] font-semibold uppercase tracking-wide text-slate-500 px-1 pb-1">Mostrar colunas</div>
              <div className="max-h-72 overflow-auto">
                {rotulos.map((r, i) => (
                  <label key={r} className={`flex items-center gap-2 px-1 py-1 text-sm rounded ${i === 0 ? 'text-slate-400' : 'cursor-pointer hover:bg-slate-50'}`}>
                    <Checkbox
                      checked={i === 0 || !ocultas.includes(r)}
                      disabled={i === 0}
                      onCheckedChange={() => alternar(r)}
                    />
                    <span className="truncate" title={r}>{r}</span>
                  </label>
                ))}
              </div>
              {escondidas > 0 && (
                <button type="button" className="mt-1 w-full text-xs text-violet-700 hover:underline py-1" onClick={() => setOcultas([])}>
                  Mostrar todas
                </button>
              )}
            </PopoverContent>
          </Popover>
        </div>
      )}
      {css && <style>{css}</style>}
      <div ref={raiz} data-tcol={id} className={className}>
        {children}
      </div>
    </div>
  )
}
