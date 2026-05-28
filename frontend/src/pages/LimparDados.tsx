import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ShieldAlert, Trash2, Loader2 } from 'lucide-react'

// Módulo de limpeza inteligente — apaga dados deste cliente por tabela.
// Página em /config/limpar-dados (somente master). Backend: /api/v2/farol/cleanup*

interface CleanupItem {
  key: string
  table: string
  label: string
  description: string
  count: number
}

const CONFIRM_WORD = 'LIMPAR'

export default function LimparDados() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [confirmText, setConfirmText] = useState('')

  const { data, isLoading, error, refetch } = useQuery<{ tables: CleanupItem[] }>({
    queryKey: ['cleanup-inventory'],
    queryFn: () => fetch('/api/v2/farol/cleanup/inventory').then(r => {
      if (!r.ok) throw new Error('Falha ao carregar inventário')
      return r.json()
    }),
    refetchOnWindowFocus: false,
  })

  const tables = data?.tables ?? []
  const selectedTables = tables.filter(t => selected.has(t.key))
  const totalSelecionado = selectedTables.reduce((s, t) => s + t.count, 0)

  const toggle = (key: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      next.has(key) ? next.delete(key) : next.add(key)
      return next
    })
  }

  const naoVazias = tables.filter(t => t.count > 0)
  const allSelected = naoVazias.length > 0 && naoVazias.every(t => selected.has(t.key))
  const toggleAll = () => {
    setSelected(allSelected ? new Set() : new Set(naoVazias.map(t => t.key)))
  }

  const mutation = useMutation({
    mutationFn: async (keys: string[]) => {
      const r = await fetch('/api/v2/farol/cleanup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tables: keys }),
      })
      if (!r.ok) {
        const e = await r.json().catch(() => ({}))
        throw new Error(e.error || 'Falha na limpeza')
      }
      return r.json() as Promise<{ deleted: Record<string, number> }>
    },
    onSuccess: (res) => {
      const total = Object.values(res.deleted || {}).reduce((s, n) => s + n, 0)
      toast.success(`Limpeza concluída — ${total.toLocaleString('pt-BR')} registros removidos`)
      setSelected(new Set())
      setConfirmText('')
      refetch()
      qc.invalidateQueries({ queryKey: ['farol-v2-cards'] })
      qc.invalidateQueries({ queryKey: ['v2-periodos'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const canSubmit =
    selected.size > 0 && confirmText.trim().toUpperCase() === CONFIRM_WORD && !mutation.isPending

  return (
    <div className="max-w-2xl mx-auto p-4">
      <div className="flex items-center gap-2 mb-1">
        <ShieldAlert className="h-5 w-5 text-red-500" />
        <h1 className="text-lg font-bold text-slate-800">Limpar Dados do Cliente</h1>
      </div>
      <p className="text-xs text-slate-400 mb-5">
        Apaga os dados deste cliente por tabela. Ação irreversível — selecione com cuidado.
      </p>

      {isLoading && (
        <div className="flex items-center gap-2 text-sm text-slate-400 py-8">
          <Loader2 className="h-4 w-4 animate-spin" /> Carregando inventário…
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-xl p-4 text-sm text-red-700">
          {(error as Error).message}
        </div>
      )}

      {!isLoading && !error && (
        <>
          <div className="bg-white border border-slate-100 rounded-xl shadow-sm divide-y divide-slate-50">
            <div className="flex items-center justify-between px-4 py-2.5">
              <label className="flex items-center gap-2 text-xs font-medium text-slate-500 cursor-pointer">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={toggleAll}
                  className="rounded border-slate-300"
                />
                Selecionar todas com dados
              </label>
              <span className="text-xs text-slate-400">{tables.length} tabelas</span>
            </div>

            {tables.map(t => {
              const vazia = t.count === 0
              return (
                <label
                  key={t.key}
                  className={`flex items-start gap-3 px-4 py-3 ${vazia ? 'opacity-50' : 'cursor-pointer hover:bg-slate-50'}`}
                >
                  <input
                    type="checkbox"
                    disabled={vazia}
                    checked={selected.has(t.key)}
                    onChange={() => toggle(t.key)}
                    className="mt-0.5 rounded border-slate-300"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-sm font-medium text-slate-800">{t.label}</span>
                      <span className={`text-sm tabular-nums font-semibold ${vazia ? 'text-slate-300' : 'text-slate-600'}`}>
                        {t.count.toLocaleString('pt-BR')}
                      </span>
                    </div>
                    <p className="text-xs text-slate-400 leading-snug">{t.description}</p>
                  </div>
                </label>
              )
            })}
          </div>

          {/* Confirmação */}
          <div className="mt-5 bg-red-50 border border-red-200 rounded-xl p-4">
            <p className="text-sm text-red-700 font-medium mb-1">
              {selected.size > 0
                ? `Vai remover ${totalSelecionado.toLocaleString('pt-BR')} registros de ${selected.size} tabela(s).`
                : 'Selecione ao menos uma tabela acima.'}
            </p>
            <p className="text-xs text-red-500 mb-3">
              Para confirmar, digite <strong>{CONFIRM_WORD}</strong> no campo abaixo.
            </p>
            <div className="flex gap-2">
              <input
                value={confirmText}
                onChange={e => setConfirmText(e.target.value)}
                placeholder={CONFIRM_WORD}
                className="flex-1 h-9 rounded-lg border border-slate-200 px-3 text-sm focus:outline-none focus:ring-2 focus:ring-red-300"
              />
              <button
                disabled={!canSubmit}
                onClick={() => mutation.mutate([...selected])}
                className="h-9 px-4 rounded-lg bg-red-600 text-white text-sm font-medium flex items-center gap-2 disabled:opacity-40 hover:bg-red-700 transition-colors"
              >
                {mutation.isPending
                  ? <><Loader2 className="h-4 w-4 animate-spin" /> Limpando…</>
                  : <><Trash2 className="h-4 w-4" /> Limpar selecionados</>}
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
