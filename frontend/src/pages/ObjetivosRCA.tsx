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
  cl_ativos: number
  posit_med: number
  ttal_itens: number
  vl_anterior: number
  vl_corrente: number
}

const MESES = ['Jan','Fev','Mar','Abr','Mai','Jun','Jul','Ago','Set','Out','Nov','Dez']
const TRIMESTRES = ['T1','T2','T3','T4']
const SEMESTRES  = ['S1','S2']

function periodoLabel(tipo: TipoPeriodo, seq: number, ano: number): string {
  if (tipo === 'MENSAL')     return `${MESES[seq - 1] ?? seq}/${ano}`
  if (tipo === 'TRIMESTRAL') return `${TRIMESTRES[seq - 1] ?? seq}/${ano}`
  if (tipo === 'SEMESTRAL')  return `${SEMESTRES[seq - 1] ?? seq}/${ano}`
  return String(ano)
}

function fmtBRL(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
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

function pctPosit(posit: number, ativos: number): string {
  if (ativos === 0) return '—'
  return ((posit / ativos) * 100).toFixed(1) + '%'
}

function medItens(ttal: number, posit: number): string {
  if (posit === 0) return '—'
  return (ttal / posit).toFixed(1)
}

// ─── Cards ────────────────────────────────────────────────────────────────────

function StatCard({ label, value, highlight }: { label: string; value: string; highlight?: boolean }) {
  return (
    <div className={`border rounded-lg p-3 space-y-0.5 min-w-0 ${highlight ? 'bg-red-50 border-red-300' : 'bg-white'}`}>
      <p className="text-xs text-muted-foreground truncate">{label}</p>
      <p className={`text-sm font-semibold leading-tight truncate ${highlight ? 'text-red-700' : ''}`}>{value}</p>
    </div>
  )
}

function VarCard({ ant, cor }: { ant: number; cor: number }) {
  const n = varNum(ant, cor)
  const up = n !== null && n > 0
  const down = n !== null && n < 0
  const Icon = up ? TrendingUp : down ? TrendingDown : Minus
  return (
    <div className="border rounded-lg p-3 bg-white space-y-0.5 min-w-0">
      <p className="text-xs text-muted-foreground">% Cresc.</p>
      <div className={`flex items-center gap-1 ${up ? 'text-green-600' : down ? 'text-red-600' : 'text-muted-foreground'}`}>
        <Icon className="w-3.5 h-3.5 shrink-0" />
        <span className="text-sm font-semibold">{varPct(ant, cor)}</span>
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

  const rcaOptions = useMemo(() => {
    const seen = new Map<number, string>()
    allRows.forEach(r => seen.set(r.cod_rca, r.nome_rca))
    return Array.from(seen.entries())
      .map(([cod, nome]) => ({ cod, nome }))
      .sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR'))
  }, [allRows])

  const rows = useMemo(() => allRows.filter(r =>
    (rcaFilter === '_all' || r.cod_rca === Number(rcaFilter)) &&
    (!fornecFilter || r.fornecedor.toLowerCase().includes(fornecFilter.toLowerCase()))
  ), [allRows, rcaFilter, fornecFilter])

  const totalAnt   = rows.reduce((s, r) => s + r.vl_anterior, 0)
  const totalCor   = rows.reduce((s, r) => s + r.vl_corrente, 0)
  const totalTtal  = rows.reduce((s, r) => s + r.ttal_itens, 0)
  const qtdRCAs    = new Set(rows.map(r => r.cod_rca)).size
  const qtdFornc   = new Set(rows.map(r => r.cod_fornec)).size
  // somas brutas; COUNT(DISTINCT) real está na view por grupo
  const sumAtivos  = rows.reduce((s, r) => s + r.cl_ativos, 0)
  const sumPosit   = rows.reduce((s, r) => s + r.posit_med, 0)
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
        <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-8 gap-2">
          <StatCard label="Venda Anterior" value={fmtBRL(totalAnt)} />
          <StatCard label="Venda Corrente" value={fmtBRL(totalCor)} />
          <VarCard ant={totalAnt} cor={totalCor} />
          <StatCard label="RCAs"           value={String(qtdRCAs)} />
          <StatCard label="Fornecedores"   value={String(qtdFornc)} />
          <StatCard label="CL Ativos"      value={sumAtivos.toLocaleString('pt-BR')} />
          <StatCard label="Positivados"    value={sumPosit.toLocaleString('pt-BR')} highlight />
          <StatCard label="Total Itens"    value={totalTtal.toLocaleString('pt-BR')} highlight />
        </div>
      )}

      {/* ── Tabela ── */}
      {periodoSel && (
        <div className="border rounded-lg overflow-auto">
          <Table className="text-xs whitespace-nowrap">
            <TableHeader>
              <TableRow className="bg-[#003366] text-white hover:bg-[#003366]">
                <TableHead className="text-white font-semibold">Supervisor</TableHead>
                <TableHead className="text-white font-semibold">RCA</TableHead>
                <TableHead className="text-white font-semibold">Fornecedor</TableHead>
                <TableHead className="text-white font-semibold text-right">Venda 24</TableHead>
                <TableHead className="text-white font-semibold text-right">Venda 25</TableHead>
                <TableHead className="text-white font-semibold text-right">% Cresc</TableHead>
                <TableHead className="text-white font-semibold text-center">CL Ativos</TableHead>
                <TableHead className="text-white font-semibold text-center">Posit Med</TableHead>
                <TableHead className="text-white font-semibold text-center">% Posit</TableHead>
                <TableHead className="bg-red-600 text-white font-semibold text-center">Méd Itens CL</TableHead>
                <TableHead className="bg-red-600 text-white font-semibold text-center">Total Itens</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.length === 0 && !isFetching && (
                <TableRow>
                  <TableCell colSpan={11} className="text-center text-muted-foreground py-8">
                    Nenhum resultado encontrado.
                  </TableCell>
                </TableRow>
              )}
              {rows.map((row, i) => {
                const vn = varNum(row.vl_anterior, row.vl_corrente)
                return (
                  <TableRow key={i} className="even:bg-muted/20">
                    <TableCell>
                      {row.cod_supervisor != null &&
                        <span className="text-muted-foreground mr-1">{row.cod_supervisor}</span>}
                      {row.nome_supervisor}
                    </TableCell>
                    <TableCell>
                      <span className="text-muted-foreground mr-1">{row.cod_rca}</span>
                      {row.nome_rca}
                    </TableCell>
                    <TableCell>{row.fornecedor}</TableCell>
                    <TableCell className="text-right text-muted-foreground">{fmtBRL(row.vl_anterior)}</TableCell>
                    <TableCell className="text-right font-medium">{fmtBRL(row.vl_corrente)}</TableCell>
                    <TableCell className={`text-right font-medium ${vn === null ? '' : vn > 0 ? 'text-green-600' : vn < 0 ? 'text-red-600' : ''}`}>
                      {varPct(row.vl_anterior, row.vl_corrente)}
                    </TableCell>
                    <TableCell className="text-center">{row.cl_ativos.toLocaleString('pt-BR')}</TableCell>
                    <TableCell className="text-center">{row.posit_med.toLocaleString('pt-BR')}</TableCell>
                    <TableCell className="text-center">{pctPosit(row.posit_med, row.cl_ativos)}</TableCell>
                    <TableCell className="text-center font-medium text-red-700">
                      {medItens(row.ttal_itens, row.posit_med)}
                    </TableCell>
                    <TableCell className="text-center font-medium text-red-700">
                      {row.ttal_itens.toLocaleString('pt-BR')}
                    </TableCell>
                  </TableRow>
                )
              })}
              {rows.length > 0 && (
                <TableRow className="bg-[#003366]/10 font-semibold">
                  <TableCell colSpan={3} className="text-muted-foreground">
                    {rows.length} linha{rows.length !== 1 ? 's' : ''}
                  </TableCell>
                  <TableCell className="text-right text-muted-foreground">{fmtBRL(totalAnt)}</TableCell>
                  <TableCell className="text-right">{fmtBRL(totalCor)}</TableCell>
                  <TableCell className={`text-right ${n === null ? '' : n > 0 ? 'text-green-600' : n < 0 ? 'text-red-600' : ''}`}>
                    {varPct(totalAnt, totalCor)}
                  </TableCell>
                  <TableCell className="text-center">{sumAtivos.toLocaleString('pt-BR')}</TableCell>
                  <TableCell className="text-center">{sumPosit.toLocaleString('pt-BR')}</TableCell>
                  <TableCell className="text-center">{pctPosit(sumPosit, sumAtivos)}</TableCell>
                  <TableCell className="text-center text-red-700">{medItens(totalTtal, sumPosit)}</TableCell>
                  <TableCell className="text-center text-red-700">{totalTtal.toLocaleString('pt-BR')}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
