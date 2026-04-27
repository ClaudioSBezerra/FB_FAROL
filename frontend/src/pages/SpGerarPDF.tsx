import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Popover, PopoverContent, PopoverTrigger,
} from '@/components/ui/popover'
import { Checkbox } from '@/components/ui/checkbox'
import { toast } from 'sonner'
import { FileDown, Loader2, ChevronsUpDown } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface SpFilial { id: number; cod_filial: number; nome: string }
interface SpCD     { id: number; filial_id: number; nome: string }
interface SpCSVJob { id: string; filename: string; status: string; created_at: string }

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SpGerarPDF() {
  const { token } = useAuth()
  const headers = { Authorization: `Bearer ${token}` }

  const [filialID,    setFilialID]    = useState<string>('')
  const [cdID,        setCdID]        = useState<string>('')
  const [jobID,       setJobID]       = useState<string>('')
  const [selectedRuas, setSelectedRuas] = useState<number[]>([]) // [] = todas
  const [downloading, setDownloading] = useState(false)

  // ── Queries ────────────────────────────────────────────────────────────────
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

  const { data: jobs = [] } = useQuery<SpCSVJob[]>({
    queryKey: ['sp-csv-jobs', cdID],
    enabled: !!cdID,
    queryFn: async () => {
      const r = await fetch(`/api/sp/csv/jobs?cd_id=${cdID}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })
  const doneJobs = jobs.filter(j => j.status === 'done')

  // ── Ruas disponíveis (aprovadas no CD ou job selecionado) ─────────────────
  const ruasParams = jobID ? `job_id=${jobID}` : cdID ? `cd_id=${cdID}` : ''
  const { data: ruasDisponiveis = [] } = useQuery<number[]>({
    queryKey: ['sp-propostas-ruas', cdID, jobID],
    enabled: !!cdID,
    staleTime: 60_000,
    queryFn: async () => {
      const r = await fetch(`/api/sp/propostas/ruas?${ruasParams}`, { headers })
      if (!r.ok) return []
      return r.json()
    },
  })

  // ── Helpers de seleção de rua ─────────────────────────────────────────────
  function toggleRua(rua: number) {
    setSelectedRuas(prev =>
      prev.includes(rua) ? prev.filter(r => r !== rua) : [...prev, rua].sort((a, b) => a - b)
    )
  }

  function toggleTodas() {
    setSelectedRuas(prev => prev.length === ruasDisponiveis.length ? [] : [...ruasDisponiveis])
  }

  const todasSelecionadas = selectedRuas.length === 0 || selectedRuas.length === ruasDisponiveis.length
  const ruasBtnLabel = selectedRuas.length === 0 || selectedRuas.length === ruasDisponiveis.length
    ? 'Todas as ruas'
    : `Ruas: ${selectedRuas.join(', ')}`

  // ── Download do PDF ────────────────────────────────────────────────────────
  async function handleDownload() {
    if (!cdID) { toast.error('Selecione um CD'); return }

    const params = new URLSearchParams()
    if (jobID) params.set('job_id', jobID)
    else       params.set('cd_id', cdID)

    // Aplica filtro de rua apenas quando seleção parcial
    if (selectedRuas.length > 0 && selectedRuas.length < ruasDisponiveis.length) {
      params.set('rua', selectedRuas.join(','))
    }

    setDownloading(true)
    try {
      const res = await fetch(`/api/sp/pdf/calibracao?${params}`, { headers })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || 'Erro ao gerar PDF')
      }
      const blob  = await res.blob()
      const url   = URL.createObjectURL(blob)
      const a     = document.createElement('a')
      const cd    = cds.find(c => String(c.id) === cdID)
      const fname = res.headers.get('Content-Disposition')
        ?.match(/filename="([^"]+)"/)?.[1]
        ?? `calibracao_${cd?.nome ?? cdID}.pdf`

      a.href     = url
      a.download = fname
      a.click()
      URL.revokeObjectURL(url)
      toast.success('PDF gerado com sucesso')
    } catch (e: unknown) {
      toast.error(e instanceof Error ? e.message : 'Erro ao baixar PDF')
    } finally {
      setDownloading(false)
    }
  }

  // ── Render ─────────────────────────────────────────────────────────────────
  const selectedJob = doneJobs.find(j => j.id === jobID)
  const selectedCD  = cds.find(c => String(c.id) === cdID)

  return (
    <div className="space-y-6 max-w-lg">
      <div className="border rounded-lg p-4 space-y-4">
        <h3 className="text-sm font-semibold">Gerar relatório de calibragem</h3>
        <p className="text-xs text-muted-foreground">
          O PDF incluirá todas as propostas <strong>aprovadas</strong> do CD ou importação
          selecionados, organizadas por curva (A / B / C) com cap. atual, nova cap. e ação
          (<strong>+N cx</strong> = ampliar slot / <strong>−N cx</strong> = reduzir slot).
        </p>

        <div className="grid gap-3">
          {/* Filial */}
          <div>
            <label className="text-xs font-medium mb-1 block">Filial</label>
            <Select value={filialID} onValueChange={v => { setFilialID(v); setCdID(''); setJobID(''); setSelectedRuas([]) }}>
              <SelectTrigger><SelectValue placeholder="Selecione a filial" /></SelectTrigger>
              <SelectContent>
                {filiais.map(f => (
                  <SelectItem key={f.id} value={String(f.id)}>
                    {f.nome} (cód. {f.cod_filial})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* CD */}
          <div>
            <label className="text-xs font-medium mb-1 block">Centro de Distribuição</label>
            <Select value={cdID} onValueChange={v => { setCdID(v); setJobID(''); setSelectedRuas([]) }} disabled={!filialID}>
              <SelectTrigger><SelectValue placeholder="Selecione o CD" /></SelectTrigger>
              <SelectContent>
                {cds.map(cd => (
                  <SelectItem key={cd.id} value={String(cd.id)}>{cd.nome}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Job (opcional) */}
          {doneJobs.length > 0 && (
            <div>
              <label className="text-xs font-medium mb-1 block">
                Importação <span className="text-muted-foreground font-normal">(opcional — padrão: todas aprovadas do CD)</span>
              </label>
              <Select value={jobID} onValueChange={v => { setJobID(v); setSelectedRuas([]) }}>
                <SelectTrigger><SelectValue placeholder="Todas as importações aprovadas" /></SelectTrigger>
                <SelectContent>
                  {doneJobs.map(j => (
                    <SelectItem key={j.id} value={j.id}>
                      {j.filename.length > 36 ? j.filename.slice(0, 34) + '…' : j.filename}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Seletor de Rua (aparece quando há ruas disponíveis) */}
          {cdID && ruasDisponiveis.length > 0 && (
            <div>
              <label className="text-xs font-medium mb-1 block">
                Rua{' '}
                <span className="text-muted-foreground font-normal">(opcional — padrão: todas)</span>
              </label>
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    variant="outline"
                    role="combobox"
                    className="w-full justify-between text-sm font-normal"
                  >
                    <span className={todasSelecionadas ? 'text-muted-foreground' : ''}>
                      {ruasBtnLabel}
                    </span>
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-64 p-2" align="start">
                  <div className="space-y-1 max-h-56 overflow-y-auto">
                    {/* Opção "Todas" */}
                    <label className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-accent cursor-pointer text-sm font-medium">
                      <Checkbox
                        checked={todasSelecionadas}
                        onCheckedChange={toggleTodas}
                      />
                      Todas as ruas ({ruasDisponiveis.length})
                    </label>
                    <div className="border-t my-1" />
                    {ruasDisponiveis.map(rua => (
                      <label
                        key={rua}
                        className="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-accent cursor-pointer text-sm"
                      >
                        <Checkbox
                          checked={selectedRuas.includes(rua) || selectedRuas.length === 0}
                          onCheckedChange={() => {
                            // Primeiro clique individual: seleciona só essa
                            if (selectedRuas.length === 0) {
                              setSelectedRuas([rua])
                            } else {
                              toggleRua(rua)
                            }
                          }}
                        />
                        Rua {rua}
                      </label>
                    ))}
                  </div>
                  {selectedRuas.length > 0 && selectedRuas.length < ruasDisponiveis.length && (
                    <div className="border-t pt-2 mt-1">
                      <button
                        className="text-xs text-muted-foreground hover:text-foreground underline w-full text-left px-2"
                        onClick={() => setSelectedRuas([])}
                      >
                        Limpar seleção (voltar para todas)
                      </button>
                    </div>
                  )}
                </PopoverContent>
              </Popover>
            </div>
          )}

          <Button disabled={!cdID || downloading} onClick={handleDownload} className="w-full">
            {downloading
              ? <><Loader2 className="h-4 w-4 mr-2 animate-spin" />Gerando PDF...</>
              : <><FileDown className="h-4 w-4 mr-2" />Baixar PDF</>
            }
          </Button>
        </div>
      </div>

      {/* Preview do que será gerado */}
      {cdID && (
        <div className="border rounded-lg p-4 bg-muted/30 space-y-1 text-xs text-muted-foreground">
          <p className="font-medium text-foreground text-sm">O PDF incluirá:</p>
          <p>• CD: <strong>{selectedCD?.nome ?? cdID}</strong></p>
          {selectedJob
            ? <p>• Importação: <strong>{selectedJob.filename}</strong></p>
            : <p>• Todas as propostas aprovadas do CD</p>
          }
          {selectedRuas.length > 0 && selectedRuas.length < ruasDisponiveis.length
            ? <p>• Ruas: <strong>{selectedRuas.join(', ')}</strong> ({selectedRuas.length} de {ruasDisponiveis.length})</p>
            : ruasDisponiveis.length > 0
              ? <p>• Ruas: <strong>todas ({ruasDisponiveis.length})</strong></p>
              : null
          }
          <p>• Agrupamento por curva (A → B → C), ordenado por endereço</p>
          <p>• Colunas: Endereço, Produto, Cap.Atual, Nova Cap., Ação (+N cx / -N cx), Justificativa</p>
        </div>
      )}
    </div>
  )
}
