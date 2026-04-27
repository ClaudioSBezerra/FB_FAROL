import { useState, useEffect, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/AuthContext'
import { Sparkles, Loader2, Mail, FileText, ChevronRight, Send } from 'lucide-react'
import { toast } from 'sonner'

interface SpFilial { id: number; cod_filial: number; nome: string }
interface SpCD     { id: number; filial_id: number; nome: string }

interface ResumoListItem {
  id: number
  cd_id: number
  periodo_inicio: string
  periodo_fim: string
  criado_em: string
  enviado_em?: string
  enviado_para_count: number
}

interface KPIs {
  cd_nome: string
  filial_nome: string
  periodo_inicio: string
  periodo_fim: string
  total_propostas: number
  total_aprovadas: number
  total_rejeitadas: number
  total_pendentes: number
  total_ignorados: number
  ampliar_slot: number
  reduzir_slot: number
  calibrados: number
  curva_a_revisar: number
  taxa_aprovacao_pct: number
  taxa_compliance_pct: number
  top_motivos_rejeicao: Array<{ label: string; valor: number }>
  top_deptos_pendentes: Array<{ label: string; valor: number }>
  top_produtos_criticos: Array<{ codprod: number; produto: string; departamento: string; classe_venda: string; delta: number }>
  alertas_urgencia: number
  alertas_ajustar: number
  alertas_cap_menor: number
  imports_periodo: Array<{
    job_id: string
    filename: string
    status: string
    uploaded_by: string
    uploaded_em: string
    total_linhas: number
    linhas_ok: number
    linhas_erro: number
  }>
  sem_atividade: boolean
}

interface ResumoDetalhe {
  id: number
  cd_id: number
  periodo_inicio: string
  periodo_fim: string
  dados: KPIs
  narrativa_md: string
  criado_em: string
  enviado_em?: string
  erro_envio?: string
}

// Renderização markdown simples (mesma estratégia do AjudaChat)
function renderMarkdown(text: string) {
  const lines = text.split('\n')
  return lines.map((line, i) => {
    if (line.startsWith('## ')) {
      return <h3 key={i} className="text-sm font-semibold mt-3 mb-1 text-foreground">{line.slice(3)}</h3>
    }
    if (line.startsWith('# ')) {
      return <h2 key={i} className="text-base font-bold mt-3 mb-2">{line.slice(2)}</h2>
    }
    const isBullet = line.trimStart().startsWith('- ') || line.trimStart().startsWith('• ')
    const content = isBullet ? line.replace(/^\s*[-•]\s*/, '') : line
    const parts = content.split(/(\*\*[^*]+\*\*)/g).map((p, j) =>
      p.startsWith('**') && p.endsWith('**') ? <strong key={j}>{p.slice(2, -2)}</strong> : p
    )
    if (isBullet) {
      return <li key={i} className="ml-4 text-xs leading-relaxed">{parts}</li>
    }
    if (line.trim() === '') return <div key={i} className="h-2" />
    return <p key={i} className="text-xs leading-relaxed">{parts}</p>
  })
}

export default function SpResumoExecutivo() {
  const { token, spRole, group } = useAuth()
  const qc = useQueryClient()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  const podeGerar = group === 'MASTER' || spRole === 'admin_fbtax' || spRole === 'gestor_geral'
  const isMaster = podeGerar // mantém compat com botão "Enviar por email"

  const [filialID, setFilialID] = useState('')
  const [cdID, setCdID]         = useState('')
  const [resumoSel, setResumoSel] = useState<number | null>(null)

  const { data: filiais = [] } = useQuery<SpFilial[]>({
    queryKey: ['filiais'],
    queryFn: async () => {
      const r = await fetch('/api/filiais', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const { data: cds = [] } = useQuery<SpCD[]>({
    queryKey: ['sp-cds-filial', filialID],
    enabled: !!filialID,
    queryFn: async () => {
      const r = await fetch(`/api/sp/filiais/${filialID}/cds`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const { data: resumos = [] } = useQuery<ResumoListItem[]>({
    queryKey: ['sp-resumos', cdID],
    enabled: !!cdID,
    queryFn: async () => {
      const r = await fetch(`/api/sp/relatorios?cd_id=${cdID}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const { data: detalhe } = useQuery<ResumoDetalhe>({
    queryKey: ['sp-resumo-item', resumoSel],
    enabled: !!resumoSel,
    queryFn: async () => {
      const r = await fetch(`/api/sp/relatorios/${resumoSel}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const gerarMutation = useMutation({
    mutationFn: async () => {
      const r = await fetch(`/api/sp/relatorios/gerar?cd_id=${cdID}`, { method: 'POST', headers })
      const data = await r.json()
      if (!r.ok) throw new Error(data.error ?? 'Erro ao gerar')
      return data as { id: number }
    },
    onSuccess: data => {
      toast.success('Resumo gerado com sucesso')
      qc.invalidateQueries({ queryKey: ['sp-resumos'] })
      setResumoSel(data.id)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const enviarMutation = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/sp/relatorios/${id}/enviar`, { method: 'POST', headers })
      const data = await r.json()
      if (!r.ok) throw new Error(data.error ?? 'Erro ao enviar')
      return data as { enviados: string[]; total: number }
    },
    onSuccess: data => {
      toast.success(`Enviado para ${data.total} destinatário(s)`)
      qc.invalidateQueries({ queryKey: ['sp-resumos'] })
      qc.invalidateQueries({ queryKey: ['sp-resumo-item'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // Auto-seleciona o resumo mais recente quando carrega a lista
  useEffect(() => {
    if (resumos.length && !resumoSel) setResumoSel(resumos[0].id)
  }, [resumos, resumoSel])

  return (
    <div className="space-y-4">
      {/* Filtros */}
      <div className="flex flex-wrap gap-3 items-end">
        <div>
          <label className="text-xs font-medium mb-1 block">Filial</label>
          <Select value={filialID} onValueChange={v => { setFilialID(v); setCdID(''); setResumoSel(null) }}>
            <SelectTrigger className="w-48"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {filiais.map(f => (
                <SelectItem key={f.id} value={String(f.id)}>{f.nome} (cód. {f.cod_filial})</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div>
          <label className="text-xs font-medium mb-1 block">CD</label>
          <Select value={cdID} onValueChange={v => { setCdID(v); setResumoSel(null) }} disabled={!filialID}>
            <SelectTrigger className="w-40"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {cds.map(cd => <SelectItem key={cd.id} value={String(cd.id)}>{cd.nome}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>

        {podeGerar && cdID && (
          <Button
            size="sm"
            onClick={() => gerarMutation.mutate()}
            disabled={gerarMutation.isPending}
          >
            {gerarMutation.isPending
              ? <><Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" />Gerando…</>
              : <><Sparkles className="h-3.5 w-3.5 mr-1" />Gerar Resumo Semanal</>}
          </Button>
        )}
      </div>

      {!cdID && (
        <p className="text-xs text-muted-foreground">Selecione filial e CD para visualizar os resumos.</p>
      )}

      {cdID && resumos.length === 0 && !gerarMutation.isPending && (
        <div className="text-center py-12 border rounded-lg bg-muted/30">
          <FileText className="h-8 w-8 mx-auto text-muted-foreground mb-2" />
          <p className="text-sm text-muted-foreground">
            Nenhum resumo gerado para este CD ainda.
          </p>
          {podeGerar ? (
            <p className="text-xs text-muted-foreground mt-1">
              Clique em "Gerar Resumo Semanal" acima para criar o primeiro.
            </p>
          ) : (
            <p className="text-xs text-muted-foreground mt-1">
              Aguarde o gestor geral ou administrador gerar o primeiro resumo.
            </p>
          )}
        </div>
      )}

      {/* Layout em 2 colunas: lista | detalhe */}
      {cdID && resumos.length > 0 && (
        <div className="grid grid-cols-12 gap-4">
          {/* Lista de resumos */}
          <div className="col-span-3 space-y-1">
            <h3 className="text-xs font-semibold text-muted-foreground mb-2 uppercase">Histórico</h3>
            {resumos.map(r => (
              <button
                key={r.id}
                onClick={() => setResumoSel(r.id)}
                className={`w-full text-left p-2 rounded text-xs border transition-colors ${
                  resumoSel === r.id ? 'bg-primary/10 border-primary' : 'hover:bg-muted border-border'
                }`}
              >
                <div className="font-medium flex items-center justify-between">
                  <span>{new Date(r.periodo_fim).toLocaleDateString('pt-BR')}</span>
                  <ChevronRight className="h-3 w-3" />
                </div>
                <div className="text-[10px] text-muted-foreground">
                  {new Date(r.periodo_inicio).toLocaleDateString('pt-BR')} → {new Date(r.periodo_fim).toLocaleDateString('pt-BR')}
                </div>
                {r.enviado_em ? (
                  <div className="text-[10px] text-green-600 flex items-center gap-1 mt-0.5">
                    <Mail className="h-2.5 w-2.5" /> {r.enviado_para_count} enviados
                  </div>
                ) : (
                  <div className="text-[10px] text-muted-foreground mt-0.5">Não enviado</div>
                )}
              </button>
            ))}
          </div>

          {/* Detalhe */}
          <div className="col-span-9">
            {!detalhe && resumoSel && (
              <div className="p-8 text-center text-muted-foreground"><Loader2 className="h-5 w-5 animate-spin mx-auto" /></div>
            )}
            {detalhe && (
              <div className="border rounded-lg bg-white">
                {/* Cabeçalho */}
                <div className="border-b p-4 flex items-start justify-between gap-3">
                  <div>
                    <h2 className="text-base font-semibold">
                      Resumo Executivo — {detalhe.dados.cd_nome}
                    </h2>
                    <p className="text-xs text-muted-foreground">
                      {detalhe.dados.filial_nome} · período {new Date(detalhe.periodo_inicio).toLocaleDateString('pt-BR')} a {new Date(detalhe.periodo_fim).toLocaleDateString('pt-BR')}
                    </p>
                    {detalhe.dados.sem_atividade && (
                      <span className="inline-flex items-center gap-1 mt-1 px-2 py-0.5 rounded bg-amber-100 text-amber-800 text-[10px] font-medium">
                        ⚠ Sem movimentação de calibragem no período
                      </span>
                    )}
                    {detalhe.enviado_em && (
                      <p className="text-[11px] text-green-700 flex items-center gap-1 mt-1">
                        <Mail className="h-3 w-3" /> Enviado em {new Date(detalhe.enviado_em).toLocaleString('pt-BR')}
                      </p>
                    )}
                  </div>
                  {isMaster && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={enviarMutation.isPending}
                      onClick={() => enviarMutation.mutate(detalhe.id)}
                    >
                      {enviarMutation.isPending
                        ? <><Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" />Enviando…</>
                        : <><Send className="h-3.5 w-3.5 mr-1" />{detalhe.enviado_em ? 'Reenviar' : 'Enviar por email'}</>}
                    </Button>
                  )}
                </div>

                {/* KPI strip */}
                <div className="grid grid-cols-4 gap-2 p-3 border-b bg-gray-50">
                  <KPI label="Propostas no período" value={detalhe.dados.total_propostas} />
                  <KPI label="Aprovadas" value={detalhe.dados.total_aprovadas} color="text-green-700" />
                  <KPI label="Rejeitadas" value={detalhe.dados.total_rejeitadas} color="text-red-600" />
                  <KPI label="Pendentes" value={detalhe.dados.total_pendentes} color="text-yellow-700" />

                  <KPI label="Ampliar slot" value={detalhe.dados.ampliar_slot} color="text-red-700" />
                  <KPI label="Reduzir slot" value={detalhe.dados.reduzir_slot} color="text-yellow-700" />
                  <KPI label="Calibrados" value={detalhe.dados.calibrados} color="text-blue-700" />
                  <KPI label="Curva A revisar" value={detalhe.dados.curva_a_revisar} color="text-amber-700" />

                  <KPI label="Taxa de aprovação" value={`${detalhe.dados.taxa_aprovacao_pct.toFixed(0)}%`} />
                  <KPI label="Compliance" value={`${detalhe.dados.taxa_compliance_pct.toFixed(0)}%`} />
                  <KPI label="Ignorados ativos" value={detalhe.dados.total_ignorados} />
                  <KPI label="Alertas críticos" value={detalhe.dados.alertas_urgencia + detalhe.dados.alertas_ajustar + detalhe.dados.alertas_cap_menor} color="text-red-700" />
                </div>

                {/* Narrativa IA */}
                <div className="p-4 border-b">
                  <div className="flex items-center gap-2 mb-2">
                    <Sparkles className="h-4 w-4 text-primary" />
                    <h3 className="text-xs font-semibold uppercase text-muted-foreground">Análise da IA</h3>
                  </div>
                  <div className="prose prose-sm max-w-none">
                    {renderMarkdown(detalhe.narrativa_md ?? '')}
                  </div>
                </div>

                {/* Tabelas detalhadas */}
                <div className="grid grid-cols-2 gap-4 p-4">
                  <DetalheLista
                    titulo="Top motivos de rejeição"
                    itens={detalhe.dados.top_motivos_rejeicao ?? []}
                  />
                  <DetalheLista
                    titulo="Top departamentos pendentes"
                    itens={detalhe.dados.top_deptos_pendentes ?? []}
                  />
                </div>

                {(detalhe.dados.top_produtos_criticos ?? []).length > 0 && (
                  <div className="p-4 border-t">
                    <h3 className="text-xs font-semibold uppercase text-muted-foreground mb-2">Top 10 produtos críticos (Curva A)</h3>
                    <div className="overflow-x-auto">
                      <table className="text-xs w-full">
                        <thead>
                          <tr className="border-b text-left text-muted-foreground">
                            <th className="py-1">Cód.</th>
                            <th>Produto</th>
                            <th>Depto</th>
                            <th className="text-right">Δ</th>
                          </tr>
                        </thead>
                        <tbody>
                          {(detalhe.dados.top_produtos_criticos ?? []).map(p => (
                            <tr key={p.codprod} className="border-b last:border-0">
                              <td className="py-1 font-mono">{p.codprod}</td>
                              <td className="truncate max-w-[280px]">{p.produto}</td>
                              <td className="text-muted-foreground">{p.departamento}</td>
                              <td className={`text-right font-semibold ${p.delta > 0 ? 'text-red-600' : 'text-yellow-700'}`}>
                                {p.delta > 0 ? `+${p.delta}` : p.delta} CX
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}

                {(detalhe.dados.imports_periodo ?? []).length > 0 && (
                  <div className="p-4 border-t">
                    <h3 className="text-xs font-semibold uppercase text-muted-foreground mb-2">
                      Importações do período ({(detalhe.dados.imports_periodo ?? []).length})
                    </h3>
                    <div className="overflow-x-auto">
                      <table className="text-xs w-full">
                        <thead>
                          <tr className="border-b text-left text-muted-foreground">
                            <th className="py-1">Data</th>
                            <th>Arquivo</th>
                            <th>Importado por</th>
                            <th className="text-right">Linhas</th>
                            <th>Status</th>
                          </tr>
                        </thead>
                        <tbody>
                          {(detalhe.dados.imports_periodo ?? []).map(imp => {
                            const cor = imp.status === 'failed' ? 'text-red-600'
                              : imp.status === 'done' ? 'text-green-700'
                              : 'text-yellow-700'
                            return (
                              <tr key={imp.job_id} className="border-b last:border-0">
                                <td className="py-1 font-mono text-[11px]">{imp.uploaded_em}</td>
                                <td className="truncate max-w-[240px]" title={imp.filename}>{imp.filename}</td>
                                <td className="text-muted-foreground text-[11px]">{imp.uploaded_by}</td>
                                <td className="text-right">{imp.total_linhas.toLocaleString('pt-BR')}</td>
                                <td className={`font-semibold ${cor}`}>{imp.status}</td>
                              </tr>
                            )
                          })}
                        </tbody>
                      </table>
                    </div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function KPI({ label, value, color = 'text-foreground' }: { label: string; value: number | string; color?: string }) {
  return (
    <div className="bg-white border rounded p-2">
      <div className="text-[10px] text-muted-foreground uppercase">{label}</div>
      <div className={`text-base font-bold ${color}`}>{typeof value === 'number' ? value.toLocaleString('pt-BR') : value}</div>
    </div>
  )
}

function DetalheLista({ titulo, itens }: { titulo: string; itens: Array<{ label: string; valor: number }> }) {
  if (!itens || itens.length === 0) {
    return (
      <div>
        <h3 className="text-xs font-semibold uppercase text-muted-foreground mb-2">{titulo}</h3>
        <p className="text-xs text-muted-foreground">Sem dados no período</p>
      </div>
    )
  }
  const max = Math.max(...itens.map(i => i.valor))
  return (
    <div>
      <h3 className="text-xs font-semibold uppercase text-muted-foreground mb-2">{titulo}</h3>
      <div className="space-y-1">
        {itens.map(it => (
          <div key={it.label} className="flex items-center gap-2 text-xs">
            <div className="flex-1 truncate" title={it.label}>{it.label}</div>
            <div className="w-24 bg-gray-100 rounded h-3 relative">
              <div className="bg-primary h-3 rounded" style={{ width: `${(it.valor / max) * 100}%` }} />
            </div>
            <div className="w-10 text-right font-semibold">{it.valor}</div>
          </div>
        ))}
      </div>
    </div>
  )
}
