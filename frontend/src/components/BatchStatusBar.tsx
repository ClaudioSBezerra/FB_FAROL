// BatchStatusBar — visualização do progresso de aprovação de um lote de calibragem
// Uso: <BatchStatusBar resumo={resumo} /> (dados já carregados pelo pai)
// Uso mini: <BatchStatusMini jobId="uuid" /> (busca internamente, para tabelas)

import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@/contexts/AuthContext'

export interface LoteResumo {
  total_pendente:   number
  total_aprovada:   number
  total_rejeitada:  number
  falta_pendente?:  number
  espaco_pendente?: number
  calibrado_total?: number
  curva_a_mantida?: number
}

// ─── Barra principal ─────────────────────────────────────────────────────────

export function BatchStatusBar({ resumo }: { resumo: LoteResumo }) {
  const total = resumo.total_pendente + resumo.total_aprovada + resumo.total_rejeitada
  if (total === 0) return null

  const pct = (n: number) => total > 0 ? (n / total) * 100 : 0
  const pctAprov  = pct(resumo.total_aprovada)
  const pctRejeit = pct(resumo.total_rejeitada)
  const pctPend   = pct(resumo.total_pendente)

  const concluido = resumo.total_aprovada + resumo.total_rejeitada
  const pctConcluido = Math.round(pct(concluido))

  return (
    <div className="border rounded-lg px-4 py-3 bg-muted/20 space-y-2">
      {/* ── Linha de resumo ── */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
        <span className="font-semibold text-foreground">
          Lote: <span className="text-base font-bold">{total}</span> previstas
        </span>
        <span className="text-muted-foreground">•</span>
        <span className="text-green-700 font-medium">
          ✓ {resumo.total_aprovada} aprovadas
          <span className="text-muted-foreground font-normal ml-1">({Math.round(pctAprov)}%)</span>
        </span>
        <span className="text-red-600 font-medium">
          ✗ {resumo.total_rejeitada} rejeitadas
          <span className="text-muted-foreground font-normal ml-1">({Math.round(pctRejeit)}%)</span>
        </span>
        <span className="text-amber-600 font-medium">
          ◌ {resumo.total_pendente} pendentes
          <span className="text-muted-foreground font-normal ml-1">({Math.round(pctPend)}%)</span>
        </span>
        {pctConcluido > 0 && (
          <>
            <span className="text-muted-foreground">•</span>
            <span className="text-muted-foreground">
              Concluído: <strong className="text-foreground">{pctConcluido}%</strong>
            </span>
          </>
        )}
      </div>

      {/* ── Barra de progresso ── */}
      <div className="w-full h-2.5 rounded-full bg-gray-200 overflow-hidden flex">
        {pctAprov > 0 && (
          <div
            style={{ width: `${pctAprov}%` }}
            className="bg-green-500 transition-all duration-500"
            title={`Aprovadas: ${resumo.total_aprovada}`}
          />
        )}
        {pctRejeit > 0 && (
          <div
            style={{ width: `${pctRejeit}%` }}
            className="bg-red-400 transition-all duration-500"
            title={`Rejeitadas: ${resumo.total_rejeitada}`}
          />
        )}
        {pctPend > 0 && (
          <div
            style={{ width: `${pctPend}%` }}
            className="bg-amber-300 transition-all duration-500"
            title={`Pendentes: ${resumo.total_pendente}`}
          />
        )}
      </div>

      {/* ── Legenda da barra ── */}
      <div className="flex gap-3 text-[10px] text-muted-foreground">
        <span className="flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-sm bg-green-500" />Aprovadas
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-sm bg-red-400" />Rejeitadas
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block w-2 h-2 rounded-sm bg-amber-300" />Pendentes
        </span>
      </div>
    </div>
  )
}

// ─── Versão mini para tabelas (busca própria por job_id) ─────────────────────

export function BatchStatusMini({ jobId }: { jobId: string }) {
  const { token } = useAuth()

  const { data: resumo, isLoading } = useQuery<LoteResumo>({
    queryKey: ['sp-propostas-resumo-job', jobId],
    staleTime: 30_000,
    queryFn: async () => {
      const r = await fetch(`/api/sp/propostas/resumo?job_id=${jobId}`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  if (isLoading) return <span className="text-[10px] text-muted-foreground">…</span>
  if (!resumo) return null

  const total = resumo.total_pendente + resumo.total_aprovada + resumo.total_rejeitada
  if (total === 0) return <span className="text-[10px] text-muted-foreground">—</span>

  const pct = (n: number) => total > 0 ? (n / total) * 100 : 0

  return (
    <div className="space-y-1 min-w-[120px]">
      {/* Mini barra */}
      <div className="w-full h-1.5 rounded-full bg-gray-200 overflow-hidden flex">
        {resumo.total_aprovada > 0 && (
          <div style={{ width: `${pct(resumo.total_aprovada)}%` }} className="bg-green-500" />
        )}
        {resumo.total_rejeitada > 0 && (
          <div style={{ width: `${pct(resumo.total_rejeitada)}%` }} className="bg-red-400" />
        )}
        {resumo.total_pendente > 0 && (
          <div style={{ width: `${pct(resumo.total_pendente)}%` }} className="bg-amber-300" />
        )}
      </div>
      {/* Contadores */}
      <div className="flex gap-2 text-[10px]">
        <span className="text-green-700">✓{resumo.total_aprovada}</span>
        <span className="text-red-500">✗{resumo.total_rejeitada}</span>
        <span className="text-amber-600">◌{resumo.total_pendente}</span>
        <span className="text-muted-foreground">/{total}</span>
      </div>
    </div>
  )
}
