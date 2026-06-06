import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { XCircle, UploadCloud, FileText, CheckCircle, AlertCircle, Loader2, Clock } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Helpers ─────────────────────────────────────────────────────────────────

const MES_MAP: Record<string, number> = {
  jan: 1, fev: 2, mar: 3, abr: 4, mai: 5, jun: 6,
  jul: 7, ago: 8, set: 9, out: 10, nov: 11, dez: 12,
}

function parseAnoMesFromFilename(name: string): { ano: number; mes: number } {
  const m = name.toLowerCase().match(/([a-z]{3})[_-](\d{4})/)
  if (m) {
    const mes = MES_MAP[m[1]]
    const ano = parseInt(m[2])
    if (mes && ano >= 2000 && ano <= 2100) return { mes, ano }
  }
  const now = new Date()
  return { mes: now.getMonth() + 1, ano: now.getFullYear() }
}

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
  status: 'pending' | 'processing' | 'done' | 'error' | 'cancelled'
  progress: number
  total_lines: number
  importados: number
  message: string
}

type ItemStatus = 'waiting' | 'uploading' | 'processing' | 'done' | 'error' | 'cancelled'

interface QueueItem {
  file: File
  status: ItemStatus
  job: ImportJob | null
  error?: string
}

// ─── Hook de períodos existentes ──────────────────────────────────────────────

function usePeriodosV2(token: string | null) {
  return useQuery<PeriodoItem[]>({
    queryKey: ['v2-periodos'],
    queryFn: () => fetch('/api/v2/vendas/periodos', {
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    }).then(r => r.json()),
    staleTime: 30_000,
    refetchOnWindowFocus: false,
    enabled: !!token,
  })
}

// ─── ImportForm ───────────────────────────────────────────────────────────────

function ImportForm({ onDone, token }: { onDone: () => void; token: string | null }) {
  const [items, setItems]           = useState<QueueItem[]>([])
  const [running, setRunning]       = useState(false)
  const [currentIdx, setCurrentIdx] = useState(-1)
  const [dragOver, setDragOver]     = useState(false)
  const [consolidating, setConsolidating] = useState<'idle' | 'running' | 'done' | 'error'>('idle')
  const fileRef      = useRef<HTMLInputElement>(null)
  const pollRef      = useRef<ReturnType<typeof setInterval> | null>(null)
  const abortRef     = useRef(false)
  const currentJobId = useRef<string | null>(null)  // ref sempre atual — evita stale closure no cancel

  useEffect(() => () => { if (pollRef.current) clearInterval(pollRef.current) }, [])

  const updateItem = (idx: number, patch: Partial<QueueItem>) =>
    setItems(prev => prev.map((it, i) => i === idx ? { ...it, ...patch } : it))

  const handleFiles = (files: FileList | null) => {
    if (!files || files.length === 0) return
    const arr = Array.from(files)
      .filter(f => f.name.endsWith('.csv') || f.name.endsWith('.txt'))
      .sort((a, b) => a.name.localeCompare(b.name))
    setItems(arr.map(file => ({ file, status: 'waiting', job: null })))
  }

  const pollJob = (jobId: string, idx: number): Promise<ImportJob> =>
    new Promise((resolve, reject) => {
      if (pollRef.current) clearInterval(pollRef.current)
      pollRef.current = setInterval(async () => {
        try {
          const r = await fetch(`/api/v2/vendas/job/${jobId}`, {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
          })
          if (!r.ok) return
          const j: ImportJob = await r.json()
          updateItem(idx, { job: j, status: j.status as ItemStatus })
          if (j.status === 'done' || j.status === 'error' || j.status === 'cancelled') {
            clearInterval(pollRef.current!)
            pollRef.current = null
            resolve(j)
          }
        } catch (e) { reject(e) }
      }, 2000)
    })

  const runQueue = async () => {
    setRunning(true)
    abortRef.current = false

    const snapshot = items // captura snapshot da fila

    for (let i = 0; i < snapshot.length; i++) {
      if (abortRef.current) {
        // marca restantes como cancelado
        setItems(prev => prev.map((it, idx) =>
          idx >= i && it.status === 'waiting' ? { ...it, status: 'cancelled' } : it
        ))
        break
      }

      setCurrentIdx(i)
      updateItem(i, { status: 'uploading' })

      try {
        const fd = new FormData()
        fd.append('file', snapshot[i].file)
        const { ano, mes } = parseAnoMesFromFilename(snapshot[i].file.name)
        const resp = await fetch(`/api/v2/vendas/import?ano=${ano}&mes=${mes}&skip_refresh=true`, {
          method: 'POST',
          headers: token ? { Authorization: `Bearer ${token}` } : {},
          body: fd,
        })

        if (!resp.ok) {
          const err = await resp.json().catch(() => ({ error: 'Erro no upload' }))
          updateItem(i, { status: 'error', error: err.error ?? 'Erro desconhecido' })
          continue
        }

        const { job_id } = await resp.json()
        currentJobId.current = job_id
        updateItem(i, { status: 'processing' })
        const job = await pollJob(job_id, i)
        currentJobId.current = null
        if (job.status === 'done') onDone()
      } catch {
        updateItem(i, { status: 'error', error: 'Falha de conexão' })
      }
    }

    setRunning(false)
    setCurrentIdx(-1)

    // Consolidação final: REFRESH das 28 MVs + upsert_aggs_mes para todos os meses
    setConsolidating('running')
    try {
      const r = await fetch('/api/v2/farol/refresh-views', {
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      })
      setConsolidating(r.ok ? 'done' : 'error')
      if (r.ok) onDone()
    } catch {
      setConsolidating('error')
    }
  }

  const handleCancel = async () => {
    abortRef.current = true
    const jobId = currentJobId.current
    if (jobId) {
      try { await fetch(`/api/v2/vendas/job/${jobId}/cancel`, {
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      }) } catch {}
    }
  }

  const resetForm = () => {
    if (pollRef.current) { clearInterval(pollRef.current); pollRef.current = null }
    setItems([])
    setRunning(false)
    setCurrentIdx(-1)
    setConsolidating('idle')
    abortRef.current = false
    currentJobId.current = null
  }

  const doneCnt    = items.filter(it => it.status === 'done').length
  const errorCnt   = items.filter(it => it.status === 'error').length
  const allSettled = !running && items.length > 0 &&
    items.every(it => ['done', 'error', 'cancelled'].includes(it.status)) &&
    consolidating !== 'idle'

  // ── Estado A: sem arquivos → drop zone ──────────────────────────────────────
  if (items.length === 0) {
    return (
      <div>
        <label className="block text-xs text-slate-500 mb-1">Arquivo(s) CSV — separador ;</label>
        <div
          onClick={() => fileRef.current?.click()}
          onDrop={e => { e.preventDefault(); setDragOver(false); handleFiles(e.dataTransfer.files) }}
          onDragOver={e => { e.preventDefault(); setDragOver(true) }}
          onDragLeave={() => setDragOver(false)}
          className={`border-2 border-dashed rounded-xl p-8 text-center cursor-pointer transition-colors ${
            dragOver
              ? 'border-primary bg-primary/5'
              : 'border-slate-200 hover:border-slate-300 bg-slate-50'
          }`}
        >
          <UploadCloud className="h-8 w-8 text-slate-300 mx-auto mb-2" />
          <p className="text-sm text-slate-500">Clique ou arraste um ou vários arquivos</p>
          <p className="text-xs text-slate-400 mt-1">Seleção múltipla · CSV com separador ; · até 1 GB por arquivo</p>
        </div>
        <input
          ref={fileRef}
          type="file"
          accept=".csv,.txt,text/csv,text/plain"
          multiple
          className="hidden"
          onChange={e => handleFiles(e.target.files)}
        />
      </div>
    )
  }

  // ── Estado B: arquivos selecionados, ainda não iniciou ───────────────────────
  if (!running && !allSettled) {
    const totalMB = items.reduce((s, it) => s + it.file.size, 0) / 1024 / 1024
    return (
      <div className="space-y-4">
        <div className="rounded-xl border border-slate-100 divide-y divide-slate-50 overflow-hidden">
          {items.map((it, i) => (
            <div key={i} className="flex items-center gap-3 px-4 py-2.5">
              <FileText className="h-4 w-4 text-slate-300 shrink-0" />
              <span className="flex-1 truncate text-sm text-slate-700">{it.file.name}</span>
              <span className="text-xs text-slate-400 tabular-nums shrink-0">
                {(it.file.size / 1024 / 1024).toFixed(1)} MB
              </span>
            </div>
          ))}
        </div>
        <p className="text-xs text-slate-400">
          {items.length} arquivo{items.length > 1 ? 's' : ''} · {totalMB.toFixed(0)} MB total · processados em sequência
        </p>
        <div className="flex gap-2">
          <button
            onClick={runQueue}
            className="flex-1 h-10 rounded-lg bg-primary text-white text-sm font-medium hover:bg-primary/90 transition-colors flex items-center justify-center gap-2"
          >
            <UploadCloud className="h-4 w-4" />
            Importar {items.length} arquivo{items.length > 1 ? 's' : ''}
          </button>
          <button
            onClick={resetForm}
            className="h-10 px-3 rounded-lg border border-slate-200 text-slate-500 text-sm hover:bg-slate-50 transition-colors"
          >
            Limpar
          </button>
        </div>
      </div>
    )
  }

  // ── Estado C/D: em execução ou concluído ─────────────────────────────────────
  return (
    <div className="space-y-3">
      {/* Cabeçalho da fila */}
      <div className="flex items-center justify-between text-xs">
        <span className="text-slate-500">
          {running
            ? `Processando ${currentIdx + 1} de ${items.length}…`
            : allSettled
              ? `${doneCnt} concluído${doneCnt !== 1 ? 's' : ''}${errorCnt > 0 ? ` · ${errorCnt} com erro` : ''}`
              : ''}
        </span>
        {running && (
          <button
            onClick={handleCancel}
            className="flex items-center gap-1 text-red-500 hover:text-red-700 font-medium"
          >
            <XCircle className="h-3.5 w-3.5" />
            Cancelar
          </button>
        )}
      </div>

      {/* Barra geral */}
      {items.length > 1 && (
        <div className="h-1.5 bg-slate-100 rounded-full overflow-hidden">
          <div
            className="h-full bg-primary/40 rounded-full transition-all duration-500"
            style={{ width: `${(doneCnt / items.length) * 100}%` }}
          />
        </div>
      )}

      {/* Lista de arquivos */}
      <div className="space-y-1.5">
        {items.map((it, i) => {
          const isCurrent = running && i === currentIdx
          const pct = it.job?.status === 'done' ? 100 : it.job?.progress ?? 0

          return (
            <div
              key={i}
              className={`rounded-lg border px-3 py-2.5 text-xs transition-colors ${
                it.status === 'done'      ? 'border-emerald-200 bg-emerald-50' :
                it.status === 'error'     ? 'border-red-200 bg-red-50' :
                it.status === 'cancelled' ? 'border-slate-200 bg-slate-50 opacity-50' :
                isCurrent                 ? 'border-primary/30 bg-primary/5' :
                'border-slate-100 bg-white'
              }`}
            >
              <div className="flex items-center gap-2">
                {it.status === 'done'      ? <CheckCircle className="h-3.5 w-3.5 text-emerald-600 shrink-0" /> :
                 it.status === 'error'     ? <AlertCircle className="h-3.5 w-3.5 text-red-600 shrink-0" /> :
                 it.status === 'cancelled' ? <XCircle className="h-3.5 w-3.5 text-slate-400 shrink-0" /> :
                 isCurrent                 ? <Loader2 className="h-3.5 w-3.5 text-primary animate-spin shrink-0" /> :
                 <Clock className="h-3.5 w-3.5 text-slate-300 shrink-0" />}

                <span className="flex-1 truncate font-medium text-slate-700">{it.file.name}</span>

                {it.status === 'done' && it.job && (
                  <span className="text-emerald-600 tabular-nums shrink-0">
                    {it.job.importados.toLocaleString('pt-BR')} linhas
                  </span>
                )}
                {it.status === 'uploading' && (
                  <span className="text-slate-400 shrink-0">Enviando…</span>
                )}
                {it.status === 'waiting' && (
                  <span className="text-slate-300 shrink-0">{(it.file.size / 1024 / 1024).toFixed(1)} MB</span>
                )}
              </div>

              {/* Barra de progresso do arquivo atual */}
              {isCurrent && (it.status === 'processing' || it.status === 'uploading') && it.job && (
                <div className="mt-2">
                  <div className="flex justify-between text-slate-400 mb-1">
                    <span>
                      {it.job.importados > 0
                        ? `${it.job.importados.toLocaleString('pt-BR')} de ~${it.job.total_lines.toLocaleString('pt-BR')}`
                        : it.job.progress >= 91 ? 'Consolidando dados…' : 'Iniciando…'}
                    </span>
                    <span className="font-semibold tabular-nums">{pct}%</span>
                  </div>
                  <div className="h-1.5 bg-white/60 rounded-full overflow-hidden border border-slate-200/60">
                    <div
                      className="h-full bg-primary rounded-full transition-all duration-500"
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                </div>
              )}

              {it.status === 'error' && it.error && (
                <p className="mt-1 text-red-600 truncate">{it.error}</p>
              )}
            </div>
          )
        })}
      </div>

      {/* Card de consolidação final */}
      {consolidating !== 'idle' && (
        <div className={`rounded-lg border px-3 py-2.5 text-xs ${
          consolidating === 'done'    ? 'border-emerald-200 bg-emerald-50' :
          consolidating === 'error'   ? 'border-red-200 bg-red-50' :
          'border-primary/30 bg-primary/5'
        }`}>
          <div className="flex items-center gap-2">
            {consolidating === 'done'  ? <CheckCircle className="h-3.5 w-3.5 text-emerald-600 shrink-0" /> :
             consolidating === 'error' ? <AlertCircle className="h-3.5 w-3.5 text-red-600 shrink-0" /> :
             <Loader2 className="h-3.5 w-3.5 text-primary animate-spin shrink-0" />}
            <span className="font-medium text-slate-700">
              {consolidating === 'running' ? 'Consolidando views e agregações…' :
               consolidating === 'done'    ? 'Consolidação concluída' :
               'Erro na consolidação'}
            </span>
          </div>
        </div>
      )}

      {allSettled && consolidating !== 'running' && (
        <button
          onClick={resetForm}
          className="text-xs text-primary hover:underline font-medium"
        >
          Importar mais arquivos
        </button>
      )}
    </div>
  )
}

// ─── PeriodosTable ────────────────────────────────────────────────────────────

function PeriodosTable({ periodos }: { periodos: PeriodoItem[] }) {
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
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// ─── FarolV2Import ────────────────────────────────────────────────────────────

export default function FarolV2Import() {
  const qc = useQueryClient()
  const { token } = useAuth()
  const { data: periodos = [], refetch } = usePeriodosV2(token)

  const handleDone = () => {
    refetch()
    qc.invalidateQueries({ queryKey: ['farol-v2-cards'] })
    qc.invalidateQueries({ queryKey: ['v2-periodos'] })
  }

  return (
    <div className="max-w-2xl">
      <div className="grid gap-6">
        {/* Card de importação */}
        <div className="bg-white border border-slate-100 rounded-xl shadow-sm p-6">
          <h2 className="text-base font-semibold text-slate-800 mb-1">Importar arquivos de Vendas</h2>
          <p className="text-xs text-slate-400 mb-4">
            Selecione um ou vários arquivos CSV. O período é detectado automaticamente a partir das datas
            do arquivo — não é necessário informar mês ou ano. Múltiplos arquivos são processados em sequência.
          </p>
          <ImportForm onDone={handleDone} token={token} />
        </div>

        {/* Card de períodos existentes */}
        <div className="bg-white border border-slate-100 rounded-xl shadow-sm p-6">
          <h2 className="text-base font-semibold text-slate-800 mb-4">Dados Importados</h2>
          <PeriodosTable periodos={periodos} />
          <p className="text-xs text-slate-400 mt-3">
            Para apagar dados use <span className="font-medium">Configurações → Limpar Dados</span>.
          </p>
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
