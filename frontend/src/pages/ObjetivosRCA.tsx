import { useState, useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { SearchableCombobox } from '@/components/ui/searchable-combobox'
import { Search, TrendingUp, TrendingDown, Minus } from 'lucide-react'

// ─── Tipos ────────────────────────────────────────────────────────────────────

type TipoPeriodo = 'MENSAL' | 'TRIMESTRAL' | 'SEMESTRAL' | 'ANUAL'

interface Periodo {
  tipo_periodo: TipoPeriodo
  ano: number
  periodo_seq: number
}

interface RCARow {
  cod_supervisor: number | null
  nome_supervisor: string
  cod_rca: number
  nome_rca: string
  cod_fornec: string
  fornecedor: string
  qtd_produtos: number
  qtd_clientes: number
  vl_anterior: number
  vl_corrente: number
}

const MESES = [
  'Jan','Fev','Mar','Abr','Mai','Jun',
  'Jul','Ago','Set','Out','Nov','Dez',
]
const TRIMESTRES = ['T1','T2','T3','T4']
const SEMESTRES  = ['S1','S2']

function periodoLabel(tipo: TipoPeriodo, seq: number, ano: number): string {
  if (tipo === 'MENSAL')     return `${MESES[seq - 1] ?? seq}/${ano}`
  if (tipo === 'TRIMESTRAL') return `${TRIMESTRES[seq - 1] ?? seq}/${ano}`
  if (tipo === 'SEMESTRAL')  return `${SEMESTRES[seq - 1] ?? seq}/${ano}`
  return String(ano)
}

function fmt(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2 })
}

function varPct(ant: number, cor: number): string {
  if (ant === 0) return cor > 0 ? '+∞' : '—'
  const pct = ((cor - ant) / ant) * 100
  return (pct >= 0 ? '+' : '') + pct.toFixed(1) + '%'
}

function varNum(ant: number, cor: number): number | null {
  if (ant === 0) return null
  return ((cor - ant) / ant) * 100
}

// ─── Cards de resumo ──────────────────────────────────────────────────────────

function StatCard({ label, value, green, red }: {
  label: string; value: string; green?: boolean; red?: boolean
}) {
  return (
    <div className="border rounded-lg p-4 bg-white space-y-1 min-w-0">
      <p className="text-xs text-muted-foreground truncate">{label}</p>
      <p className={`text-sm font-semibold leading-tight truncate ${green ? 'text-green-600' : red ? 'text-red-600' : ''}`}>
        {value}
      </p>
    </div>
  )
}

function VarCard({ ant, cor }: { ant: number; cor: number }) {
  const n = varNum(ant, cor)
  const label = varPct(ant, cor)
  const up = n !== null && n > 0
  const down = n !== null && n < 0
  const Icon = up ? TrendingUp : down ? TrendingDown : Minus
  return (
    <div className="border rounded-lg p-4 bg-white space-y-1 min-w-0">
      <p className="text-xs text-muted-foreground">Variação</p>
      <div className={`flex items-center gap-1.5 ${up ? 'text-green-600' : down ? 'text-red-600' : 'text-muted-foreground'}`}>
        <Icon className="w-4 h-4 shrink-0" />
        <span className="text-sm font-semibold">{label}</span>
      </div>
    </div>
  )
}

// ─── Componente ───────────────────────────────────────────────────────────────

export default function ObjetivosRCA() {
  const [periodoKey,   setPeriodoKey]   = useState('')
  const [rcaFilter,    setRcaFilter]    = useState('_all')
  const [fornecFilter, setFornecFilter] = useState('')

  const { data: periodos = [] } = useQuery<Periodo[]>({
    queryKey: ['objetivos-periodos'],
    queryFn:  () => fetch('/api/objetivos/periodos').then(r => r.json()),
  })

  useEffect(() => {
    if (periodos.length > 0 && !periodoKey) {
      const p = periodos[0]
      setPeriodoKey(`${p.tipo_periodo}|${p.ano}|${p.periodo_seq}`)
    }
  }, [periodos, periodoKey])

  const periodoSel = periodos.find(p =>
    `${p.tipo_periodo}|${p.ano}|${p.periodo_seq}` === periodoKey
  )

  const { data: allRows = [], isFetching } = useQuery<RCARow[]>({
    queryKey: ['objetivos-rca-all', periodoKey],
    queryFn: () => {
      if (!periodoSel) return []
      const p = new URLSearchParams({
        tipo_periodo: periodoSel.tipo_periodo,
        ano:          String(periodoSel.ano),
        periodo_seq:  String(periodoSel.periodo_seq),
      })
      return fetch(`/api/objetivos/rca-fornecedor?${p}`).then(r => r.json())
    },
    enabled: !!periodoSel,
  })

  // Opções de RCA para o dropdown
  const rcaOptions = useMemo(() => {
    const seen = new Map<number, string>()
    allRows.forEach(r => seen.set(r.cod_rca, r.nome_rca))
    return Array.from(seen.entries())
      .map(([cod, nome]) => ({ cod, nome }))
      .sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR'))
  }, [allRows])

  // Filtragem client-side
  const rows = useMemo(() => allRows.filter(r =>
    (rcaFilter === '_all' || r.cod_rca === Number(rcaFilter)) &&
    (!fornecFilter || r.fornecedor.toLowerCase().includes(fornecFilter.toLowerCase()))
  ), [allRows, rcaFilter, fornecFilter])

  const totalAnt   = rows.reduce((s, r) => s + r.vl_anterior, 0)
  const totalCor   = rows.reduce((s, r) => s + r.vl_corrente, 0)
  const totalCli   = rows.reduce((s, r) => s + (r.qtd_clientes ?? 0), 0)
  const qtdRCAs    = new Set(rows.map(r => r.cod_rca)).size
  const qtdFornc   = new Set(rows.map(r => r.cod_fornec)).size
  const n          = varNum(totalAnt, totalCor)

  if (!periodoSel && periodos.length === 0 && !isFetching) {
    return (
      <p className="text-sm text-muted-foreground py-8 text-center">
        Nenhum objetivo importado. Use a aba <strong>Importar</strong> para carregar dados.
      </p>
    )
  }

  return (
    <div className="space-y-4">

      {/* ── Filtros ── */}
      <div className="flex flex-wrap gap-3 items-end">
        <div className="space-y-1.5">
          <Label>Período</Label>
          <Select value={periodoKey} onValueChange={v => { setPeriodoKey(v); setRcaFilter('_all'); setFornecFilter('') }}>
            <SelectTrigger className="w-44">
              <SelectValue placeholder="Selecione..." />
            </SelectTrigger>
            <SelectContent>
              {periodos.map(p => {
                const k = `${p.tipo_periodo}|${p.ano}|${p.periodo_seq}`
                return (
                  <SelectItem key={k} value={k}>
                    {periodoLabel(p.tipo_periodo, p.periodo_seq, p.ano)}
                  </SelectItem>
                )
              })}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1.5">
          <Label>RCA</Label>
          <SearchableCombobox
            className="w-56"
            placeholder="Todos os RCAs"
            searchPlaceholder="Código ou nome..."
            value={rcaFilter}
            onChange={setRcaFilter}
            options={[
              { value: '_all', label: 'Todos os RCAs' },
              ...rcaOptions.map(r => ({ value: String(r.cod), label: `${r.cod} — ${r.nome}` })),
            ]}
          />
        </div>

        <div className="space-y-1.5 flex-1 min-w-44 max-w-xs">
          <Label>Fornecedor</Label>
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 w-4 h-4 text-muted-foreground" />
            <Input
              className="pl-8"
              placeholder="Filtrar por fornecedor..."
              value={fornecFilter}
              onChange={e => setFornecFilter(e.target.value)}
            />
          </div>
        </div>

        {isFetching && <span className="text-xs text-muted-foreground pb-2">Carregando...</span>}
      </div>

      {/* ── Cards de resumo ── */}
      {allRows.length > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
          <StatCard label="Valor Corrente" value={fmt(totalCor)} />
          <StatCard label="Valor Anterior" value={fmt(totalAnt)} />
          <VarCard ant={totalAnt} cor={totalCor} />
          <StatCard label="RCAs" value={String(qtdRCAs)} />
          <StatCard label="Fornecedores" value={String(qtdFornc)} />
          <StatCard label="Clientes" value={totalCli.toLocaleString('pt-BR')} />
        </div>
      )}

      {/* ── Tabela ── */}
      {periodoSel && (
        <div className="border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40">
                <TableHead>Supervisor</TableHead>
                <TableHead>RCA</TableHead>
                <TableHead>Fornecedor</TableHead>
                <TableHead className="text-center">Prod.</TableHead>
                <TableHead className="text-right">Anterior</TableHead>
                <TableHead className="text-right">Corrente</TableHead>
                <TableHead className="text-right">Var.%</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.length === 0 && !isFetching && (
                <TableRow>
                  <TableCell colSpan={8} className="text-center text-muted-foreground py-8">
                    Nenhum resultado encontrado.
                  </TableCell>
                </TableRow>
              )}
              {rows.map((row, i) => {
                const vn = varNum(row.vl_anterior, row.vl_corrente)
                return (
                  <TableRow key={i}>
                    <TableCell className="text-xs">
                      {row.cod_supervisor != null &&
                        <span className="text-muted-foreground mr-1">{row.cod_supervisor}</span>}
                      {row.nome_supervisor}
                    </TableCell>
                    <TableCell className="text-xs">
                      <span className="text-muted-foreground mr-1">{row.cod_rca}</span>
                      {row.nome_rca}
                    </TableCell>
                    <TableCell className="text-xs">{row.fornecedor}</TableCell>
                    <TableCell className="text-center text-xs">{row.qtd_produtos}</TableCell>
                    <TableCell className="text-right text-xs text-muted-foreground">{fmt(row.vl_anterior)}</TableCell>
                    <TableCell className="text-right text-xs font-medium">{fmt(row.vl_corrente)}</TableCell>
                    <TableCell className={`text-right text-xs font-medium ${vn === null ? '' : vn > 0 ? 'text-green-600' : vn < 0 ? 'text-red-600' : ''}`}>
                      {varPct(row.vl_anterior, row.vl_corrente)}
                    </TableCell>
                  </TableRow>
                )
              })}
              {rows.length > 0 && (
                <TableRow className="bg-muted/40 font-semibold">
                  <TableCell colSpan={4} className="text-xs text-muted-foreground">
                    {rows.length} linha{rows.length !== 1 ? 's' : ''}
                  </TableCell>
                  <TableCell className="text-right text-xs text-muted-foreground">{fmt(totalAnt)}</TableCell>
                  <TableCell className="text-right text-xs">{fmt(totalCor)}</TableCell>
                  <TableCell className={`text-right text-xs ${n === null ? '' : n > 0 ? 'text-green-600' : n < 0 ? 'text-red-600' : ''}`}>
                    {varPct(totalAnt, totalCor)}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
