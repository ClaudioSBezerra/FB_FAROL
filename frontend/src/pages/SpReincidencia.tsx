import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { AlertTriangle, RefreshCw, Repeat2 } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface SpFilial { id: number; cod_filial: number; nome: string }
interface SpCD     { id: number; filial_id: number; nome: string }

interface ReincidenciaItem {
  cd_id: number
  cd_nome: string
  filial_nome: string
  cod_filial: number
  codprod: number
  produto: string
  rua: number | null
  predio: number | null
  apto: number | null
  classe_venda: string | null
  capacidade: number | null
  ultima_sugestao: number | null
  ultimo_delta: number | null
  ciclos_repetidos: number
  primeiro_ciclo: string
  ultimo_ciclo: string
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function ClasseBadge({ classe }: { classe: string | null }) {
  if (!classe) return <span className="text-muted-foreground text-xs">—</span>
  const colors: Record<string, string> = {
    A: 'bg-red-100 text-red-800',
    B: 'bg-yellow-100 text-yellow-800',
    C: 'bg-green-100 text-green-800',
  }
  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${colors[classe] ?? 'bg-gray-100 text-gray-700'}`}>
      {classe}
    </span>
  )
}

function DeltaBadge({ delta }: { delta: number | null }) {
  if (delta === null) return <span className="text-muted-foreground text-xs">—</span>
  if (delta > 0) return <span className="text-red-600 font-medium text-xs">+{delta} cx</span>
  if (delta < 0) return <span className="text-yellow-600 font-medium text-xs">{delta} cx</span>
  return <span className="text-green-600 font-medium text-xs">OK</span>
}

function CiclosBadge({ ciclos }: { ciclos: number }) {
  if (ciclos >= 4) return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold bg-red-100 text-red-800">
      <AlertTriangle className="h-3 w-3" /> {ciclos}×
    </span>
  )
  if (ciclos >= 3) return (
    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold bg-orange-100 text-orange-800">
      {ciclos}×
    </span>
  )
  return (
    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-yellow-100 text-yellow-800">
      {ciclos}×
    </span>
  )
}

function fmtDate(iso: string) {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString('pt-BR', { day: '2-digit', month: '2-digit', year: '2-digit' })
}

function fmtEnd(rua: number | null, predio: number | null, apto: number | null) {
  const parts = [rua, predio, apto].filter(v => v != null)
  return parts.length > 0 ? parts.join('-') : '—'
}

// ─── Resumo ───────────────────────────────────────────────────────────────────

function ResumoCard({ items }: { items: ReincidenciaItem[] }) {
  const total = items.length
  const criticos  = items.filter(i => i.ciclos_repetidos >= 4).length
  const atencao   = items.filter(i => i.ciclos_repetidos === 3).length
  const por2ciclos = items.filter(i => i.ciclos_repetidos === 2).length

  const prodsCds = new Set(items.map(i => i.cd_id)).size

  return (
    <div className="grid grid-cols-2 gap-3 mb-4 sm:grid-cols-4">
      <div className="rounded-lg border bg-white p-3">
        <div className="text-2xl font-bold text-foreground">{total}</div>
        <div className="text-xs text-muted-foreground mt-0.5">Endereços reincidentes</div>
      </div>
      <div className="rounded-lg border bg-red-50 border-red-200 p-3">
        <div className="text-2xl font-bold text-red-700">{criticos}</div>
        <div className="text-xs text-red-600 mt-0.5">Crítico (4+ ciclos)</div>
      </div>
      <div className="rounded-lg border bg-orange-50 border-orange-200 p-3">
        <div className="text-2xl font-bold text-orange-700">{atencao}</div>
        <div className="text-xs text-orange-600 mt-0.5">Atenção (3 ciclos)</div>
      </div>
      <div className="rounded-lg border bg-yellow-50 border-yellow-200 p-3">
        <div className="text-2xl font-bold text-yellow-700">{por2ciclos}</div>
        <div className="text-xs text-yellow-600 mt-0.5">Recente (2 ciclos)</div>
      </div>
    </div>
  )
}

// ─── Componente principal ─────────────────────────────────────────────────────

export default function SpReincidencia() {
  const { token } = useAuth()

  const [cdID,      setCdID]      = useState<string>('todos')
  const [minCiclos, setMinCiclos] = useState<string>('2')
  const [classeFilter, setClasseFilter] = useState<string>('todas')

  // Filiais e CDs
  const { data: filiais = [] } = useQuery<SpFilial[]>({
    queryKey: ['filiais'],
    queryFn: () =>
      fetch('/api/sp/filiais', { headers: { Authorization: `Bearer ${token}` } }).then(r => r.json()),
  })

  const { data: cds = [] } = useQuery<SpCD[]>({
    queryKey: ['cds-all'],
    queryFn: () =>
      fetch('/api/sp/cds?todos=1', { headers: { Authorization: `Bearer ${token}` } }).then(r => r.json()),
  })

  // Dados de reincidência
  const params = new URLSearchParams({ min_ciclos: minCiclos })
  if (cdID !== 'todos') params.set('cd_id', cdID)

  const { data: items = [], isLoading, refetch } = useQuery<ReincidenciaItem[]>({
    queryKey: ['reincidencia', cdID, minCiclos],
    queryFn: () =>
      fetch(`/api/sp/reincidencia?${params}`, { headers: { Authorization: `Bearer ${token}` } }).then(r => r.json()),
  })

  // Filtragem local por classe
  const filtered = classeFilter === 'todas'
    ? items
    : items.filter(i => (i.classe_venda ?? 'C') === classeFilter)

  // Agrupa por CD para subheaders
  const byCd = filtered.reduce<Record<number, { nome: string; filial: string; items: ReincidenciaItem[] }>>(
    (acc, it) => {
      if (!acc[it.cd_id]) acc[it.cd_id] = { nome: it.cd_nome, filial: it.filial_nome, items: [] }
      acc[it.cd_id].items.push(it)
      return acc
    },
    {}
  )

  const cdOptions = cds.map(cd => {
    const filial = filiais.find(f => f.id === cd.filial_id)
    return { value: String(cd.id), label: `${filial?.nome ?? ''} — ${cd.nome}` }
  })

  return (
    <div className="space-y-4">

      {/* Cabeçalho */}
      <div className="flex items-center gap-2">
        <Repeat2 className="h-5 w-5 text-orange-600" />
        <div>
          <h1 className="text-base font-semibold">Reincidência de Calibragem</h1>
          <p className="text-xs text-muted-foreground">
            Produtos com sugestão de ajuste em múltiplos ciclos que <strong>nunca foram alterados no Winthor</strong> (mesma capacidade em todas as cargas).
          </p>
        </div>
      </div>

      {/* Filtros */}
      <div className="flex flex-wrap gap-2 items-center">
        <Select value={cdID} onValueChange={setCdID}>
          <SelectTrigger className="h-8 text-xs w-56">
            <SelectValue placeholder="Todos os CDs" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="todos">Todos os CDs</SelectItem>
            {cdOptions.map(o => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={minCiclos} onValueChange={setMinCiclos}>
          <SelectTrigger className="h-8 text-xs w-44">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="2">Mín. 2 ciclos</SelectItem>
            <SelectItem value="3">Mín. 3 ciclos</SelectItem>
            <SelectItem value="4">Mín. 4 ciclos</SelectItem>
          </SelectContent>
        </Select>

        <Select value={classeFilter} onValueChange={setClasseFilter}>
          <SelectTrigger className="h-8 text-xs w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="todas">Todas as curvas</SelectItem>
            <SelectItem value="A">Curva A</SelectItem>
            <SelectItem value="B">Curva B</SelectItem>
            <SelectItem value="C">Curva C</SelectItem>
          </SelectContent>
        </Select>

        <button
          onClick={() => refetch()}
          className="h-8 px-3 rounded-md border text-xs text-muted-foreground hover:bg-gray-50 flex items-center gap-1"
        >
          <RefreshCw className="h-3 w-3" /> Atualizar
        </button>

        {filtered.length > 0 && (
          <span className="text-xs text-muted-foreground ml-auto">
            {filtered.length} endereço(s)
          </span>
        )}
      </div>

      {/* Resumo */}
      {!isLoading && items.length > 0 && <ResumoCard items={items} />}

      {/* Estado vazio */}
      {!isLoading && filtered.length === 0 && (
        <div className="rounded-lg border bg-green-50 border-green-200 p-6 text-center">
          <div className="text-green-700 font-medium text-sm">Nenhuma reincidência encontrada</div>
          <div className="text-xs text-green-600 mt-1">
            Todos os endereços foram ajustados no Winthor após as sugestões, ou ainda não há importações suficientes.
          </div>
        </div>
      )}

      {/* Loading */}
      {isLoading && (
        <div className="text-center py-8 text-sm text-muted-foreground">Carregando...</div>
      )}

      {/* Tabela por CD */}
      {!isLoading && Object.entries(byCd).map(([cdId, group]) => (
        <div key={cdId} className="rounded-lg border overflow-hidden">
          {/* Subheader do CD */}
          <div className="bg-gray-50 border-b px-4 py-2 flex items-center justify-between">
            <div>
              <span className="font-semibold text-sm">{group.filial}</span>
              <span className="text-muted-foreground text-sm"> — {group.nome}</span>
            </div>
            <Badge variant="secondary" className="text-xs">
              {group.items.length} endereço(s)
            </Badge>
          </div>

          <Table>
            <TableHeader>
              <TableRow className="text-xs">
                <TableHead className="w-12">Ciclos</TableHead>
                <TableHead className="w-16">Curva</TableHead>
                <TableHead className="w-20">Cód.</TableHead>
                <TableHead>Produto</TableHead>
                <TableHead className="w-24">Endereço</TableHead>
                <TableHead className="w-24 text-right">Cap. Atual</TableHead>
                <TableHead className="w-24 text-right">Última Sugestão</TableHead>
                <TableHead className="w-20 text-right">Ação</TableHead>
                <TableHead className="w-24">1º Ciclo</TableHead>
                <TableHead className="w-24">Últ. Ciclo</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {group.items.map((it, idx) => (
                <TableRow
                  key={`${it.codprod}-${it.rua}-${it.predio}-${it.apto}-${idx}`}
                  className={it.ciclos_repetidos >= 4 ? 'bg-red-50/40' : it.ciclos_repetidos >= 3 ? 'bg-orange-50/40' : ''}
                >
                  <TableCell>
                    <CiclosBadge ciclos={it.ciclos_repetidos} />
                  </TableCell>
                  <TableCell>
                    <ClasseBadge classe={it.classe_venda} />
                  </TableCell>
                  <TableCell className="text-xs font-mono">{it.codprod}</TableCell>
                  <TableCell className="text-xs max-w-[200px] truncate" title={it.produto}>{it.produto}</TableCell>
                  <TableCell className="text-xs font-mono text-center">
                    {fmtEnd(it.rua, it.predio, it.apto)}
                  </TableCell>
                  <TableCell className="text-xs text-right">
                    {it.capacidade != null ? `${it.capacidade} cx` : '—'}
                  </TableCell>
                  <TableCell className="text-xs text-right">
                    {it.ultima_sugestao != null ? `${it.ultima_sugestao} cx` : '—'}
                  </TableCell>
                  <TableCell className="text-right">
                    <DeltaBadge delta={it.ultimo_delta} />
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{fmtDate(it.primeiro_ciclo)}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{fmtDate(it.ultimo_ciclo)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ))}

    </div>
  )
}
