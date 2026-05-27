import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { XCircle, UploadCloud, FileText, CheckCircle, AlertCircle, Loader2 } from 'lucide-react'

// ─── Tipos ────────────────────────────────────────────────────────────────────

interface PeriodoItem {
  tipo_base: string
  ano: number
  mes: number
  label: string
  total: number
}

interface ImportJob {
  id: string
  tipo_base: string
  ano: number
  mes: number
  status: 'pending' | 'processing' | 'done' | 'error' | 'cancelled'
  progress: number      // 0-100
  total_lines: number
  importados: number
  message: string
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
  const [tipoBase, setTipoBase] = useState<'ATUAL' | 'COMPARATIVA'>('ATUAL')
  const [ano, setAno]           = useState(new Date().getFullYear())
  const [mes, setMes]           = useState(new Date().getMonth() + 1)
  const [file, setFile]         = useState<File | null>(null)
  const [job, setJob]           = useState<ImportJob | null>(null)
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const isActive = uploading || (job && (job.status === 'pending' || job.status === 'processing'))

  // Para o polling quando o componente é desmontado
  useEffect(() => () => { if (pollRef.current) clearInterval(pollRef.current) }, [])

  const startPolling = (jobId: string) => {
    if (pollRef.current) clearInterval(pollRef.current)
    pollRef.current = setInterval(async () => {
      try {
        const r = await fetch(`/api/v2/vendas/job/${jobId}`)
        if (!r.ok) return
        const j: ImportJob = await r.json()
        setJob(j)
        if (j.status === 'done' || j.status === 'error' || j.status === 'cancelled') {
          clearInterval(pollRef.current!)
          pollRef.current = null
          if (j.status === 'done') onDone()
        }
      } catch { /* ignora erros de rede transitórios */ }
    }, 2000)
  }

  const handleImport = async () => {
    if (!file) return
    setUploading(true)
    setJob(null)

    const fd = new FormData()
    fd.append('file', file)

    try {
      const url = `/api/v2/vendas/import?tipo_base=${tipoBase}&ano=${ano}&mes=${mes}`
      const resp = await fetch(url, { method: 'POST', body: fd })
      if (!resp.ok) {
        const err = await resp.json().catch(() => ({ error: 'Erro desconhecido' }))
        setJob({ id: '', tipo_base: tipoBase, ano, mes, status: 'error',
          progress: 0, total_lines: 0, importados: 0, message: err.error ?? 'Erro no upload' })
        return
      }
      const { job_id } = await resp.json()
      // Faz o primeiro fetch imediatamente para mostrar o estado "pending"
      const initResp = await fetch(`/api/v2/vendas/job/${job_id}`)
      if (initResp.ok) setJob(await initResp.json())
      startPolling(job_id)
    } catch (e) {
      setJob({ id: '', tipo_base: tipoBase, ano, mes, status: 'error',
        progress: 0, total_lines: 0, importados: 0, message: 'Falha de conexão' })
    } finally {
      setUploading(false)
    }
  }

  const handleCancel = async () => {
    if (!job?.id) return
    try {
      await fetch(`/api/v2/vendas/job/${job.id}/cancel`, { method: 'POST' })
      // O polling detecta o status 'cancelled' e para automaticamente
    } catch { /* ignora */ }
  }

  const resetForm = () => {
    if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null }
    setJob(null)
    setFile(null)
    setUploading(false)
  }

  const pct = job
    ? job.status === 'done' ? 100 : job.progress
    : 0

  return (
    <div className="space-y-4">
      {/* Tipo de base */}
      <div className="flex rounded-lg border border-slate-200 overflow-hidden w-full sm:w-auto">
        {(['ATUAL', 'COMPARATIVA'] as const).map(t => (
          <button
            key={t}
            disabled={!!isActive}
            onClick={() => setTipoBase(t)}
            className={`flex-1 px-4 py-2 text-sm font-medium transition-colors disabled:opacity-60 ${
              tipoBase === t ? 'bg-primary text-white' : 'text-slate-600 hover:bg-slate-50'
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
            disabled={!!isActive}
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
            disabled={!!isActive}
            value={ano}
            onChange={e => setAno(+e.target.value)}
            className="h-9 rounded-lg border border-slate-200 bg-white px-3 text-sm shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/30 disabled:opacity-60"
          >
            {anoOptions().map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
          </select>
        </div>
      </div>

      {/* Drop zone */}
      {!isActive && !job && (
        <div>
          <label className="block text-xs text-slate-500 mb-1">Arquivo CSV (separador: ;)</label>
          <div
            onClick={() => fileRef.current?.click()}
            className={`border-2 border-dashed rounded-xl p-6 text-center cursor-pointer transition-colors ${
              file
                ? 'border-primary/40 bg-primary/5'
                : 'border-slate-200 hover:border-slate-300 bg-slate-50'
            }`}
          >
            {file ? (
              <>
                <FileText className="h-8 w-8 text-primary mx-auto mb-2" />
                <p className="text-sm font-medium text-slate-700">{file.name}</p>
                <p className="text-xs text-slate-400">{(file.size / 1024 / 1024).toFixed(1)} MB</p>
              </>
            ) : (
              <>
                <UploadCloud className="h-8 w-8 text-slate-300 mx-auto mb-2" />
                <p className="text-sm text-slate-500">Clique ou arraste o arquivo</p>
                <p className="text-xs text-slate-400 mt-1">CSV com separador ; · até 1 GB</p>
              </>
            )}
          </div>
          <input
            ref={fileRef}
            type="file"
            accept=".csv,.txt,text/csv,text/plain"
            className="hidden"
            onChange={e => setFile(e.target.files?.[0] ?? null)}
          />
        </div>
      )}

      {/* Painel de progresso ─────────────────────────────────────────────────── */}
      {(isActive || job) && (
        <div className={`rounded-xl border p-4 space-y-3 ${
          job?.status === 'error'     ? 'bg-red-50 border-red-200' :
          job?.status === 'cancelled' ? 'bg-amber-50 border-amber-200' :
          job?.status === 'done'      ? 'bg-emerald-50 border-emerald-200' :
          'bg-slate-50 border-slate-200'
        }`}>

          {/* Linha de status */}
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              {job?.status === 'done' ? (
                <CheckCircle className="h-4 w-4 text-emerald-600 shrink-0" />
              ) : job?.status === 'error' ? (
                <AlertCircle className="h-4 w-4 text-red-600 shrink-0" />
              ) : job?.status === 'cancelled' ? (
                <XCircle className="h-4 w-4 text-amber-600 shrink-0" />
              ) : (
                <Loader2 className="h-4 w-4 text-primary animate-spin shrink-0" />
              )}
              <span className={`text-sm font-medium ${
                job?.status === 'done'      ? 'text-emerald-700' :
                job?.status === 'error'     ? 'text-red-700' :
                job?.status === 'cancelled' ? 'text-amber-700' :
                'text-slate-700'
              }`}>
                {uploading
                  ? 'Enviando arquivo...'
                  : job?.status === 'pending'     ? 'Aguardando processamento...'
                  : job?.status === 'processing'  ? 'Processando...'
                  : job?.status === 'done'        ? 'Importação concluída'
                  : job?.status === 'error'       ? 'Erro na importação'
                  : job?.status === 'cancelled'   ? 'Cancelado'
                  : ''}
              </span>
            </div>

            {/* Botão cancelar — só durante processamento */}
            {(job?.status === 'pending' || job?.status === 'processing') && (
              <button
                onClick={handleCancel}
                className="flex items-center gap-1 text-xs text-red-500 hover:text-red-700 font-medium transition-colors shrink-0"
              >
                <XCircle className="h-3.5 w-3.5" />
                Cancelar
              </button>
            )}
          </div>

          {/* Barra de progresso */}
          {(isActive || job?.status === 'processing' || job?.status === 'done') && (
            <div>
              <div className="flex justify-between text-xs text-slate-500 mb-1">
                <span>
                  {job?.importados
                    ? `${job.importados.toLocaleString('pt-BR')} linhas`
                    : uploading ? 'Enviando...' : 'Iniciando...'}
                  {job?.total_lines ? ` de ~${job.total_lines.toLocaleString('pt-BR')}` : ''}
                </span>
                <span className="font-semibold tabular-nums">{pct}%</span>
              </div>
              <div className="h-2 bg-white/60 rounded-full overflow-hidden border border-slate-200/60">
                <div
                  className={`h-full rounded-full transition-all duration-500 ${
                    job?.status === 'done' ? 'bg-emerald-500' : 'bg-primary'
                  }`}
                  style={{ width: uploading ? '5%' : `${pct}%` }}
                />
              </div>
            </div>
          )}

          {/* Mensagem de erro */}
          {job?.message && (job.status === 'error' || job.status === 'cancelled') && (
            <p className="text-xs text-slate-600">{job.message}</p>
          )}

          {/* Ação pós-conclusão */}
          {(job?.status === 'done' || job?.status === 'error' || job?.status === 'cancelled') && (
            <button
              onClick={resetForm}
              className="text-xs text-primary hover:underline font-medium"
            >
              {job.status === 'done' ? 'Importar outro arquivo' : 'Tentar novamente'}
            </button>
          )}
        </div>
      )}

      {/* Botão importar */}
      {!isActive && !job && (
        <button
          disabled={!file || uploading}
          onClick={handleImport}
          className="w-full h-10 rounded-lg bg-primary text-white text-sm font-medium disabled:opacity-50 hover:bg-primary/90 transition-colors flex items-center justify-center gap-2"
        >
          <UploadCloud className="h-4 w-4" />
          Importar arquivo
        </button>
      )}
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
          <h2 className="text-base font-semibold text-slate-800 mb-1">Importar arquivo de Vendas</h2>
          <p className="text-xs text-slate-400 mb-4">
            Carregue a Base Atual (ano corrente) ou Base Comparativa (ano anterior).
            Dados do mesmo período serão substituídos. Arquivos grandes são processados em segundo plano — você pode cancelar a qualquer momento.
          </p>
          <ImportForm onDone={handleDone} />
        </div>

        {/* Card de períodos existentes */}
        <div className="bg-white border border-slate-100 rounded-xl shadow-sm p-6">
          <h2 className="text-base font-semibold text-slate-800 mb-4">Dados Importados</h2>
          <PeriodosTable periodos={periodos} onDelete={handleDelete} />
        </div>

        {/* Formato */}
        <div className="bg-slate-50 border border-slate-100 rounded-xl p-5 text-xs text-slate-500 space-y-1">
          <p className="font-semibold text-slate-600 mb-2">Formato do arquivo</p>
          <p>Separador: <code className="bg-white px-1 rounded">;  (ponto-e-vírgula)</code></p>
          <p>Colunas obrigatórias: <code className="bg-white px-1 rounded">CODUSUR, CODFORNEC, CODCLI, QT, PVENDA</code></p>
          <p>Estado: campo <code className="bg-white px-1 rounded">ESTADO</code> ou detectado via <code className="bg-white px-1 rounded">PERIODO</code> (contendo "TRANS" = transmitido)</p>
          <p>Clientes sem venda: inclua a linha com <code className="bg-white px-1 rounded">QT=0, PVENDA=0</code> para positivação correta</p>
          <p>Codificação: UTF-8 ou Latin-1 (Windows-1252) — detectado automaticamente</p>
        </div>
      </div>
    </div>
  )
}
