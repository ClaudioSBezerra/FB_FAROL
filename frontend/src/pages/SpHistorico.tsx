import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from 'sonner'
import { RefreshCw, AlertTriangle, CheckCircle2, Clock, XCircle, UploadCloud, Hourglass, Ban } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface SpFilial { id: number; cod_filial: number; nome: string }

interface Historico {
  id: number
  job_id: string | null
  cd_id: number
  cd_nome: string
  filial_nome: string
  total_propostas: number
  aprovadas: number
  rejeitadas: number
  pendentes: number
  curva_a: number
  curva_b: number
  curva_c: number
  executado_em: string
  concluido_em: string | null
  status: string
  observacao: string | null
}

interface ComplianceCD {
  cd_id: number
  cd_nome: string
  filial_nome: string
  cod_filial: number
  ultima_calibragem: string | null
  dias_desde_ultima: number | null
  ultimo_status: string | null
  total_ciclos: number
  ultimo_import_em: string | null
  dias_desde_import: number | null
  total_imports: number
  propostas_pendentes: number
  dias_oldest_pendente: number | null
  ultimo_gestor_nome: string | null
  status_compliance: 'ok' | 'atencao' | 'critico' | 'aguardando_motor' | 'nunca_iniciado'
  alerta: boolean
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function fmtDate(s?: string | null) {
  if (!s) return '—'
  return new Date(s).toLocaleString('pt-BR')
}

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { cls: string; label: string; Icon: React.ElementType }> = {
    em_andamento: { cls: 'bg-blue-100 text-blue-800',   label: 'Em andamento', Icon: Clock },
    concluido:    { cls: 'bg-green-100 text-green-800', label: 'Concluído',    Icon: CheckCircle2 },
    nao_executado:{ cls: 'bg-gray-100 text-gray-600',   label: 'Não executado',Icon: XCircle },
  }
  const { cls, label, Icon } = map[status] ?? map.nao_executado
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium ${cls}`}>
      <Icon className="h-3 w-3" />{label}
    </span>
  )
}

function TaxaBarra({ aprovadas, rejeitadas, pendentes }: {
  aprovadas: number; rejeitadas: number; pendentes: number
}) {
  const total = aprovadas + rejeitadas + pendentes
  if (total === 0) return <span className="text-xs text-muted-foreground">—</span>
  const pct = (n: number) => Math.round((n / total) * 100)
  return (
    <div className="flex items-center gap-2">
      <div className="flex h-2 w-24 rounded overflow-hidden bg-gray-100">
        <div className="bg-green-500"  style={{ width: `${pct(aprovadas)}%` }} />
        <div className="bg-red-400"    style={{ width: `${pct(rejeitadas)}%` }} />
        <div className="bg-yellow-300" style={{ width: `${pct(pendentes)}%` }} />
      </div>
      <span className="text-[10px] text-muted-foreground">{pct(aprovadas)}%</span>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SpHistorico() {
  const { token } = useAuth()
  const qc = useQueryClient()
  const headers = { Authorization: `Bearer ${token}` }

  const [filialID, setFilialID] = useState<string>('')

  // ── Queries ────────────────────────────────────────────────────────────────
  const { data: filiais = [] } = useQuery<SpFilial[]>({
    queryKey: ['filiais'],
    queryFn: async () => {
      const r = await fetch('/api/filiais', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const histParams = new URLSearchParams()
  if (filialID) histParams.set('filial_id', filialID)

  const {
    data: historico = [],
    refetch: refetchHistorico,
  } = useQuery<Historico[]>({
    queryKey: ['sp-historico', filialID],
    queryFn: async () => {
      const r = await fetch(`/api/sp/historico?${histParams}&limit=100`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const compParams = new URLSearchParams()
  if (filialID) compParams.set('filial_id', filialID)

  const {
    data: compliance = [],
    refetch: refetchCompliance,
  } = useQuery<ComplianceCD[]>({
    queryKey: ['sp-compliance', filialID],
    queryFn: async () => {
      const r = await fetch(`/api/sp/historico/compliance?${compParams}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  // ── Fechar ciclo ───────────────────────────────────────────────────────────
  const fecharMutation = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/sp/historico/${id}/fechar`, {
        method: 'POST', headers,
      })
      if (!r.ok) throw new Error((await r.json()).error ?? 'Erro')
    },
    onSuccess: () => {
      toast.success('Ciclo fechado com sucesso')
      qc.invalidateQueries({ queryKey: ['sp-historico'] })
      qc.invalidateQueries({ queryKey: ['sp-compliance'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const alertas   = compliance.filter(c => c.alerta).length
  const criticos  = compliance.filter(c => c.status_compliance === 'critico').length
  const atencao   = compliance.filter(c => c.status_compliance === 'atencao').length
  const nunca     = compliance.filter(c => c.status_compliance === 'nunca_iniciado').length
  const aguardando= compliance.filter(c => c.status_compliance === 'aguardando_motor').length
  const ok        = compliance.filter(c => c.status_compliance === 'ok').length

  // ── Render ─────────────────────────────────────────────────────────────────
  return (
    <div className="space-y-4">
      {/* Filtro filial */}
      <div className="flex items-end gap-3">
        <div>
          <label className="text-xs font-medium mb-1 block">Filial</label>
          <Select value={filialID} onValueChange={setFilialID}>
            <SelectTrigger className="w-52">
              <SelectValue placeholder="Todas as filiais" />
            </SelectTrigger>
            <SelectContent>
              {filiais.map(f => (
                <SelectItem key={f.id} value={String(f.id)}>
                  {f.nome} (cód. {f.cod_filial})
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button size="sm" variant="outline" onClick={() => { refetchHistorico(); refetchCompliance() }}>
          <RefreshCw className="h-3.5 w-3.5 mr-1" /> Atualizar
        </Button>
      </div>

      <Tabs defaultValue="historico">
        <TabsList>
          <TabsTrigger value="historico">Histórico de Calibragem</TabsTrigger>
          <TabsTrigger value="compliance">
            Compliance
            {alertas > 0 && (
              <span className="ml-1.5 bg-red-500 text-white text-[10px] rounded-full px-1.5 py-0.5">
                {alertas}
              </span>
            )}
          </TabsTrigger>
        </TabsList>

        {/* ── Aba: Histórico ────────────────────────────────────────────── */}
        <TabsContent value="historico" className="space-y-3">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>CD / Filial</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="text-right">Total</TableHead>
                <TableHead>Taxa aprovação</TableHead>
                <TableHead>Curvas A·B·C</TableHead>
                <TableHead>Executado em</TableHead>
                <TableHead>Concluído em</TableHead>
                <TableHead className="w-24">Ação</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {historico.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-sm text-muted-foreground py-10">
                    Nenhum ciclo de calibragem registrado.
                  </TableCell>
                </TableRow>
              )}
              {historico.map(h => (
                <TableRow key={h.id}>
                  <TableCell className="text-xs">
                    <span className="font-medium">{h.cd_nome}</span>
                    <span className="text-muted-foreground ml-1">· {h.filial_nome}</span>
                  </TableCell>
                  <TableCell><StatusBadge status={h.status} /></TableCell>
                  <TableCell className="text-xs text-right">{h.total_propostas}</TableCell>
                  <TableCell>
                    <TaxaBarra
                      aprovadas={h.aprovadas}
                      rejeitadas={h.rejeitadas}
                      pendentes={h.pendentes}
                    />
                  </TableCell>
                  <TableCell className="text-xs font-mono text-muted-foreground">
                    {h.curva_a}·{h.curva_b}·{h.curva_c}
                  </TableCell>
                  <TableCell className="text-xs">{fmtDate(h.executado_em)}</TableCell>
                  <TableCell className="text-xs">{fmtDate(h.concluido_em)}</TableCell>
                  <TableCell>
                    {h.status === 'em_andamento' && (
                      <Button
                        size="sm" variant="outline" className="h-7 text-xs"
                        disabled={fecharMutation.isPending}
                        onClick={() => fecharMutation.mutate(h.id)}
                      >
                        Fechar ciclo
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TabsContent>

        {/* ── Aba: Compliance ───────────────────────────────────────────── */}
        <TabsContent value="compliance" className="space-y-4">
          {/* Resumo semáforo */}
          <div className="flex flex-wrap gap-2">
            {criticos   > 0 && <CompliancePill color="red"    label="Crítico"          count={criticos}   />}
            {nunca      > 0 && <CompliancePill color="gray"   label="Nunca iniciado"   count={nunca}      />}
            {aguardando > 0 && <CompliancePill color="blue"   label="Aguardando motor" count={aguardando} />}
            {atencao    > 0 && <CompliancePill color="yellow" label="Atenção"          count={atencao}    />}
            {ok         > 0 && <CompliancePill color="green"  label="Em dia"           count={ok}         />}
          </div>

          <div className="grid gap-3 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3">
            {compliance.length === 0 && (
              <p className="text-sm text-muted-foreground col-span-3 py-6 text-center">
                Nenhum CD ativo encontrado.
              </p>
            )}
            {[...compliance]
              .sort((a, b) => statusOrder(a.status_compliance) - statusOrder(b.status_compliance))
              .map(c => <ComplianceCard key={c.cd_id} c={c} />)
            }
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ─── Helpers de compliance ────────────────────────────────────────────────────

function statusOrder(s: string) {
  const order: Record<string, number> = {
    critico: 1, nunca_iniciado: 2, aguardando_motor: 3, atencao: 4, ok: 5,
  }
  return order[s] ?? 9
}

const statusMeta: Record<string, {
  dot: string; border: string; bg: string;
  Icon: React.ElementType; label: string; msg: (c: ComplianceCD) => string
}> = {
  critico: {
    dot: 'bg-red-500', border: 'border-red-300', bg: 'bg-red-50',
    Icon: AlertTriangle, label: 'Crítico',
    msg: c => c.total_ciclos === 0
      ? `Importou há ${c.dias_desde_import ?? '?'} dias — motor não foi rodado`
      : c.dias_oldest_pendente != null && c.dias_oldest_pendente > 14
        ? `Propostas pendentes há ${c.dias_oldest_pendente} dias sem aprovação`
        : `Sem calibragem há ${c.dias_desde_ultima ?? '?'} dias`,
  },
  nunca_iniciado: {
    dot: 'bg-gray-400', border: 'border-gray-200', bg: 'bg-gray-50',
    Icon: Ban, label: 'Nunca iniciado',
    msg: () => 'Nenhum arquivo CSV importado até o momento',
  },
  aguardando_motor: {
    dot: 'bg-blue-500', border: 'border-blue-200', bg: 'bg-blue-50',
    Icon: Hourglass, label: 'Aguardando motor',
    msg: c => `CSV importado há ${c.dias_desde_import ?? '?'} dias — motor de calibragem não executado`,
  },
  atencao: {
    dot: 'bg-yellow-400', border: 'border-yellow-200', bg: 'bg-yellow-50',
    Icon: Clock, label: 'Atenção',
    msg: c => c.dias_oldest_pendente != null && c.dias_oldest_pendente > 7
      ? `Propostas pendentes há ${c.dias_oldest_pendente} dias sem aprovação`
      : `Sem calibragem há ${c.dias_desde_ultima ?? '?'} dias`,
  },
  ok: {
    dot: 'bg-green-500', border: 'border-green-200', bg: 'bg-white',
    Icon: CheckCircle2, label: 'Em dia',
    msg: c => `Último ciclo há ${c.dias_desde_ultima ?? 0} dia(s)`,
  },
}

function CompliancePill({ color, label, count }: { color: string; label: string; count: number }) {
  const cls: Record<string, string> = {
    red:    'bg-red-100 text-red-700 border-red-200',
    gray:   'bg-gray-100 text-gray-600 border-gray-200',
    blue:   'bg-blue-100 text-blue-700 border-blue-200',
    yellow: 'bg-yellow-100 text-yellow-700 border-yellow-200',
    green:  'bg-green-100 text-green-700 border-green-200',
  }
  return (
    <span className={`inline-flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full border ${cls[color]}`}>
      <span className={`w-2 h-2 rounded-full ${color === 'red' ? 'bg-red-500' : color === 'gray' ? 'bg-gray-400' : color === 'blue' ? 'bg-blue-500' : color === 'yellow' ? 'bg-yellow-400' : 'bg-green-500'}`} />
      {count} {label}
    </span>
  )
}

function ComplianceCard({ c }: { c: ComplianceCD }) {
  const meta = statusMeta[c.status_compliance] ?? statusMeta.ok
  const { Icon } = meta

  return (
    <div className={`border ${meta.border} ${meta.bg} rounded-lg p-3 space-y-2`}>
      {/* Header */}
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-semibold truncate">{c.cd_nome}</p>
          <p className="text-xs text-muted-foreground">{c.filial_nome} · cód. {c.cod_filial}</p>
        </div>
        <div className="flex items-center gap-1.5 shrink-0">
          <span className={`inline-block w-2.5 h-2.5 rounded-full ${meta.dot}`} />
          <span className="text-[10px] font-medium text-muted-foreground">{meta.label}</span>
        </div>
      </div>

      {/* Motivo */}
      <div className="flex items-start gap-1.5 text-xs">
        <Icon className={`h-3.5 w-3.5 shrink-0 mt-0.5 ${c.status_compliance === 'ok' ? 'text-green-600' : c.status_compliance === 'critico' ? 'text-red-600' : 'text-yellow-600'}`} />
        <span className={c.status_compliance === 'critico' ? 'text-red-700 font-medium' : 'text-muted-foreground'}>
          {meta.msg(c)}
        </span>
      </div>

      {/* Métricas */}
      <div className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-[11px]">
        <MetricRow label="Última calibragem"
          value={c.ultima_calibragem ? new Date(c.ultima_calibragem).toLocaleDateString('pt-BR') : 'Nunca'}
          alert={!c.ultima_calibragem}
        />
        <MetricRow label="Ciclos completos" value={String(c.total_ciclos)} />
        <MetricRow label="Último import"
          value={c.ultimo_import_em ? `${c.dias_desde_import ?? 0}d atrás` : 'Nunca'}
          alert={!c.ultimo_import_em}
          sub={c.total_imports > 1 ? `(${c.total_imports} imports)` : undefined}
        />
        {c.propostas_pendentes > 0 && (
          <MetricRow label="Pendentes"
            value={`${c.propostas_pendentes} propostas`}
            sub={c.dias_oldest_pendente != null ? `mais antiga: ${c.dias_oldest_pendente}d` : undefined}
            alert={c.dias_oldest_pendente != null && c.dias_oldest_pendente > 7}
          />
        )}
      </div>

      {/* Gestor */}
      {c.ultimo_gestor_nome && (
        <p className="text-[10px] text-muted-foreground border-t pt-1.5 truncate">
          Gestor: <span className="font-medium text-foreground">{c.ultimo_gestor_nome}</span>
        </p>
      )}

      {/* Aviso de import duplicado */}
      {c.total_imports >= 3 && c.total_ciclos <= 1 && (
        <p className="text-[10px] text-amber-700 bg-amber-50 border border-amber-200 rounded px-1.5 py-1 flex items-center gap-1">
          <UploadCloud className="h-3 w-3 shrink-0" />
          {c.total_imports} importações sem ciclos — verificar duplicatas
        </p>
      )}
    </div>
  )
}

function MetricRow({ label, value, sub, alert }: {
  label: string; value: string; sub?: string; alert?: boolean
}) {
  return (
    <div>
      <span className="text-muted-foreground">{label}: </span>
      <span className={alert ? 'text-red-600 font-semibold' : 'font-medium'}>{value}</span>
      {sub && <span className="text-muted-foreground ml-1">{sub}</span>}
    </div>
  )
}
