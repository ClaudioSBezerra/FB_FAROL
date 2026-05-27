import { useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

// ─── Tipos ────────────────────────────────────────────────────────────────────

interface PeriodoItem {
  tipo_base: string
  ano: number
  mes: number
  label: string
  total: number
}

interface ImportProgress {
  total?: number
  processed?: number
  importados?: number
  done?: boolean
  error?: string
}

const MES_NOMES = ['', 'Jan', 'Fev', 'Mar', 'Abr', 'Mai', 'Jun', 'Jul', 'Ago', 'Set', 'Out', 'Nov', 'Dez']
function mesOptions() {
  return Array.from({ length: 12 }, (_, i) => ({ value: i + 1, label: MES_NOMES[i + 1] }))
}
function anoOptions() {
  const cur = new Date().getFullYear()
  return [cur + 1, cur, cur - 1, cur - 2].map(a => ({ value: a, label: String(a) }))
}

// ─── Hook de períodos existentes ──────────────────────────────────────────────

function usePeriodosV2() {
  return useQuery<PeriodoItem[]>({
    queryKey: ['v2-periodos'],
    queryFn: () => fetch('/api/v2/vendas/periodos').then(r => r.json()),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
  })
}

// ─── ImportForm ───────────────────────────────────────────────────────────────

function ImportForm({ onDone }: { onDone: () => void }) {
  const [tipoBase, setTipoBase]   = useState<'ATUAL' | 'COMPARATIVA'>('ATUAL')
  const [ano, setAno]             = useState(new Date().getFullYear())
  const [mes, setMes]             = useState(new Date().getMonth() + 1)
  const [file, setFile]           = useState<File | null>(null)
  const [progress, setProgress]   = useState<ImportProgress | null>(null)
  const [importing, setImporting] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const handleImport = async () => {
    if (!file) return
    setImporting(true)
    setProgress({ total: 0 })

    const fd = new FormData()
    fd.append('file', file)

    const url = `/api/v2/vendas/import?tipo_base=${tipoBase}&ano=${ano}&mes=${mes}`
    const resp = await fetch(url, { method: 'POST', body: fd })

    const reader = resp.body!.getReader()
    const dec = new TextDecoder()
    let buf = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buf += dec.decode(value, { stream: true })
      const lines = buf.split('\n')
      buf = lines.pop() ?? ''
      for (const line of lines) {
        if (!line.startsWith('data:')) continue
        try {
          const ev: ImportProgress = JSON.parse(line.slice(5).trim())
          setProgress(ev)
          if (ev.done || ev.error) {
            setImporting(false)
            if (ev.done) onDone()
            return
          }
        } catch { /* skip */ }
      }
    }
    setImporting(false)
  }

  const pct = progress?.total && progress.total > 0
    ? Math.round(((progress.processed ?? 0) / progress.total) * 100)
    : 0

  return (
    <div className="space-y-4">
      {/* Tipo de base */}
      <div className="flex rounded-lg border border-slate-200 overflow-hidden w-full sm:w-auto">
        {(['ATUAL', 'COMPARATIVA'] as const).map(t => (
          <button
            key={t}
            disabled={importing}
            onClick={() => setTipoBase(t)}
            className={`flex-1 px-4 py-2 text-sm font-medium transition-colors disabled:opacity-60 ${
              tipoBase === t
                ? 'bg-primary text-white'
                : 'text-slate-600 hover:bg-slate-50'
            }`}
          >
            {t === 'ATUAL' ? 'Base Atual' : 'Base Comparativa'}
          </button>
        ))}
      </div>

      {/* Período */}
      <div className="flex gap-2 flex-wrap">
        <div>
          <label className="block text-xs text-slate-500 mb-1">Mês</label>
          <select
            disabled={importing}
            value={mes}
            onChange={e => setMes(+e.target.value)}
            className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30 disabled:opacity-60"
          >
            {mesOptions().map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </div>
        <div>
          <label className="block text-xs text-slate-500 mb-1">Ano</label>
          <select
            disabled={importing}
            value={ano}
            onChange={e => setAno(+e.target.value)}
            className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30 disabled:opacity-60"
          >
            {anoOptions().map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </div>
      </div>

      {/* Arquivo */}
      <div>
        <label className="block text-xs text-slate-500 mb-1">Arquivo CSV (separador: ;)</label>
        <div
          onClick={() => !importing && fileRef.current?.click()}
          className={`border-2 border-dashed rounded-xl p-6 text-center cursor-pointer transition-colors ${
            file
              ? 'border-primary/40 bg-primary/5'
              : 'border-slate-200 hover:border-slate-300 bg-slate-50'
          } ${importing ? 'opacity-60 cursor-not-allowed' : ''}`}
        >
          <p className="text-2xl mb-1">{file ? '📄' : '📂'}</p>
          {file ? (
            <>
              <p className="text-sm font-medium text-slate-700">{file.name}</p>
              <p className="text-xs text-slate-400">{(file.size / 1024 / 1024).toFixed(1)} MB</p>
            </>
          ) : (
            <>
              <p className="text-sm text-slate-500">Clique para selecionar ou arraste o arquivo</p>
              <p className="text-xs text-slate-400 mt-1">CSV com separador ponto-e-vírgula (;)</p>
            </>
          )}
        </div>
        <input
          ref={fileRef}
          type="file"
          accept=".csv,text/csv"
          className="hidden"
          onChange={e => setFile(e.target.files?.[0] ?? null)}
        />
      </div>

      {/* Progresso */}
      {progress && (
        <div className="space-y-2">
          {progress.error ? (
            <div className="bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-700">
              ⚠ {progress.error}
            </div>
          ) : progress.done ? (
            <div className="bg-emerald-50 border border-emerald-200 rounded-lg p-3 text-sm text-emerald-700">
              ✓ Importação concluída — {(progress.importados ?? 0).toLocaleString('pt-BR')} linhas importadas
            </div>
          ) : (
            <>
              <div className="flex justify-between text-xs text-slate-500">
                <span>{pct}% — {(progress.processed ?? 0).toLocaleString('pt-BR')} linhas processadas</span>
                <span>{(progress.importados ?? 0).toLocaleString('pt-BR')} importadas</span>
              </div>
              <div className="h-2 bg-slate-100 rounded-full overflow-hidden">
                <div
                  className="h-2 bg-primary transition-all rounded-full"
                  style={{ width: `${pct}%` }}
                />
              </div>
            </>
          )}
        </div>
      )}

      {/* Botão */}
      <button
        disabled={!file || importing}
        onClick={handleImport}
        className="w-full h-10 rounded-lg bg-primary text-white text-sm font-medium disabled:opacity-50 hover:bg-primary/90 transition-colors"
      >
        {importing ? 'Importando...' : 'Importar CSV'}
      </button>
    </div>
  )
}

// ─── PeriodosTable ────────────────────────────────────────────────────────────

function PeriodosTable({ periodos, onDelete }: { periodos: PeriodoItem[]; onDelete: (p: PeriodoItem) => void }) {
  if (periodos.length === 0) {
    return <p className="text-sm text-slate-400 text-center py-8">Nenhum dado importado ainda.</p>
  }
  return (
    <table className="w-full text-sm">
      <thead>
        <tr className="border-b border-slate-100">
          <th className="text-left py-2 text-xs text-slate-400 font-medium">Tipo</th>
          <th className="text-left py-2 text-xs text-slate-400 font-medium">Período</th>
          <th className="text-right py-2 text-xs text-slate-400 font-medium">Linhas</th>
          <th className="py-2" />
        </tr>
      </thead>
      <tbody className="divide-y divide-slate-50">
        {periodos.map(p => (
          <tr key={`${p.tipo_base}-${p.ano}-${p.mes}`}>
            <td className="py-2">
              <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-medium ${
                p.tipo_base === 'ATUAL'
                  ? 'bg-blue-50 text-blue-700'
                  : 'bg-amber-50 text-amber-700'
              }`}>
                {p.tipo_base}
              </span>
            </td>
            <td className="py-2 text-slate-700">{p.label}</td>
            <td className="py-2 text-right text-slate-500 tabular-nums">
              {p.total.toLocaleString('pt-BR')}
            </td>
            <td className="py-2 text-right">
              <button
                onClick={() => onDelete(p)}
                className="text-xs text-red-400 hover:text-red-600 transition-colors"
              >
                Remover
              </button>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// ─── FarolV2Import ────────────────────────────────────────────────────────────

export default function FarolV2Import() {
  const qc = useQueryClient()
  const { data: periodos = [], refetch } = usePeriodosV2()

  const handleDone = () => {
    refetch()
    qc.invalidateQueries({ queryKey: ['farol-v2-cards'] })
    qc.invalidateQueries({ queryKey: ['v2-periodos'] })
  }

  const handleDelete = async (p: PeriodoItem) => {
    if (!confirm(`Remover ${p.tipo_base} ${p.label}? Esta ação não pode ser desfeita.`)) return
    await fetch(
      `/api/v2/vendas/clear?tipo_base=${p.tipo_base}&ano=${p.ano}&mes=${p.mes}`,
      { method: 'DELETE' }
    )
    refetch()
    qc.invalidateQueries({ queryKey: ['farol-v2-cards'] })
  }

  return (
    <div className="max-w-2xl">
      <div className="grid gap-6">
        {/* Card de importação */}
        <div className="bg-white border border-slate-100 rounded-xl shadow-sm p-6">
          <h2 className="text-base font-semibold text-slate-800 mb-1">Importar CSV de Vendas</h2>
          <p className="text-xs text-slate-400 mb-4">
            Carregue a Base Atual (ano corrente) ou Base Comparativa (ano anterior).
            Dados do mesmo período serão substituídos.
          </p>
          <ImportForm onDone={handleDone} />
        </div>

        {/* Card de períodos existentes */}
        <div className="bg-white border border-slate-100 rounded-xl shadow-sm p-6">
          <h2 className="text-base font-semibold text-slate-800 mb-4">Dados Importados</h2>
          <PeriodosTable periodos={periodos} onDelete={handleDelete} />
        </div>

        {/* Info sobre formato CSV */}
        <div className="bg-slate-50 border border-slate-100 rounded-xl p-5 text-xs text-slate-500 space-y-1">
          <p className="font-semibold text-slate-600 mb-2">Formato do CSV</p>
          <p>Separador: <code className="bg-white px-1 rounded">;  (ponto-e-vírgula)</code></p>
          <p>Colunas obrigatórias: <code className="bg-white px-1 rounded">CODUSUR, CODFORNEC, CODCLI, QT, PVENDA</code></p>
          <p>Estado: campo <code className="bg-white px-1 rounded">ESTADO</code> ou detectado via <code className="bg-white px-1 rounded">PERIODO</code> (contendo "TRANS" = transmitido)</p>
          <p>Clientes sem venda: inclua a linha com <code className="bg-white px-1 rounded">QT=0, PVENDA=0</code> para positivação correta</p>
        </div>
      </div>
    </div>
  )
}
