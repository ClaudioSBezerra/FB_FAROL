import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ShieldAlert, Trash2, Loader2, Zap } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

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
const CONFIRM_WORD_ADMIN = 'TRUNCATE'

export default function LimparDados() {
  const qc = useQueryClient()
  const { spRole } = useAuth()
  const isAdminFbtax = spRole === 'admin_fbtax'
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [confirmText, setConfirmText] = useState('')
  const [adminMode, setAdminMode] = useState(false)

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
    mutationFn: async ({ keys, admin }: { keys: string[]; admin: boolean }) => {
      const r = await fetch('/api/v2/farol/cleanup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tables: keys, admin_truncate: admin }),
      })
      if (!r.ok) {
        const e = await r.json().catch(() => ({}))
        throw new Error(e.error || 'Falha na limpeza')
      }
      return r.json() as Promise<{ deleted: Record<string, number> }>
    },
    onSuccess: (res) => {
      const total = Object.values(res.deleted || {}).reduce((s, n) => s + n, 0)
      const modo = adminMode ? ' (TRUNCATE — espaço em disco liberado)' : ''
      toast.success(`Limpeza concluída${modo} — ${total.toLocaleString('pt-BR')} registros removidos`)
      setSelected(new Set())
      setConfirmText('')
      setAdminMode(false)
      refetch()
      qc.invalidateQueries({ queryKey: ['farol-v2-cards'] })
      qc.invalidateQueries({ queryKey: ['v2-periodos'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const requiredWord = adminMode ? CONFIRM_WORD_ADMIN : CONFIRM_WORD
  const canSubmit =
    selected.size > 0 && confirmText.trim().toUpperCase() === requiredWord && !mutation.isPending

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

          {/* Modo Admin — TRUNCATE completo (só visível pra admin_fbtax) */}
          {isAdminFbtax && (
            <div className="mt-5 bg-purple-50 border-2 border-purple-300 rounded-xl p-4">
              <div className="flex items-start justify-between gap-3 mb-2">
                <div className="flex items-center gap-2 min-w-0">
                  <Zap className="h-4 w-4 text-purple-600 shrink-0" />
                  <span className="text-sm font-bold text-purple-900">Modo Administrador (TRUNCATE)</span>
                </div>
                <label className="flex items-center gap-2 shrink-0 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={adminMode}
                    onChange={e => { setAdminMode(e.target.checked); setConfirmText('') }}
                    className="rounded border-purple-400"
                  />
                  <span className="text-xs font-medium text-purple-800">Ativar</span>
                </label>
              </div>
              <p className="text-xs text-purple-800 leading-relaxed">
                Usa <strong>TRUNCATE</strong> em vez de DELETE — zera a tabela INTEIRA (todas as empresas)
                e devolve o espaço em disco na hora. Ideal em instâncias single-tenant ou dev/staging.
                Cuidado: em ambientes multi-cliente, apaga dados de todos os clientes.
              </p>
            </div>
          )}

          {/* Confirmação */}
          <div className={`mt-5 rounded-xl p-4 border ${adminMode ? 'bg-purple-50 border-purple-300' : 'bg-red-50 border-red-200'}`}>
            <p className={`text-sm font-medium mb-1 ${adminMode ? 'text-purple-900' : 'text-red-700'}`}>
              {selected.size > 0
                ? adminMode
                  ? `Vai TRUNCAR ${selected.size} tabela(s) — zera dados de todas as empresas.`
                  : `Vai remover ${totalSelecionado.toLocaleString('pt-BR')} registros de ${selected.size} tabela(s).`
                : 'Selecione ao menos uma tabela acima.'}
            </p>
            <p className={`text-xs mb-3 ${adminMode ? 'text-purple-700' : 'text-red-500'}`}>
              Para confirmar, digite <strong>{requiredWord}</strong> no campo abaixo.
            </p>
            <div className="flex gap-2">
              <input
                value={confirmText}
                onChange={e => setConfirmText(e.target.value)}
                placeholder={requiredWord}
                className={`flex-1 h-9 rounded-lg border px-3 text-sm focus:outline-none focus:ring-2 ${adminMode ? 'border-purple-200 focus:ring-purple-300' : 'border-slate-200 focus:ring-red-300'}`}
              />
              <button
                disabled={!canSubmit}
                onClick={() => mutation.mutate({ keys: [...selected], admin: adminMode })}
                className={`h-9 px-4 rounded-lg text-white text-sm font-medium flex items-center gap-2 disabled:opacity-40 transition-colors ${adminMode ? 'bg-purple-700 hover:bg-purple-800' : 'bg-red-600 hover:bg-red-700'}`}
              >
                {mutation.isPending
                  ? <><Loader2 className="h-4 w-4 animate-spin" /> {adminMode ? 'Truncando…' : 'Limpando…'}</>
                  : adminMode
                    ? <><Zap className="h-4 w-4" /> TRUNCATE selecionados</>
                    : <><Trash2 className="h-4 w-4" /> Limpar selecionados</>}
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}
