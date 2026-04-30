import { useState, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Search } from 'lucide-react'

// ─── Tipos ────────────────────────────────────────────────────────────────────

type TipoPeriodo = 'MENSAL' | 'TRIMESTRAL' | 'SEMESTRAL' | 'ANUAL'

interface Periodo {
  tipo_periodo: TipoPeriodo
  ano: number
  periodo_seq: number
}

interface SupervisorRow {
  cod_supervisor: number
  nome_supervisor: string
  cod_fornec: string
  fornecedor: string
  qtd_rcas: number
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

function varClass(ant: number, cor: number): string {
  if (ant === 0) return ''
  return cor >= ant ? 'text-green-600' : 'text-red-600'
}

// ─── Componente ───────────────────────────────────────────────────────────────

export default function ObjetivosSupervisor() {
  const [search, setSearch] = useState('')
  const [periodoKey, setPeriodoKey] = useState<string>('')

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

  const { data: rows = [], isFetching } = useQuery<SupervisorRow[]>({
    queryKey: ['objetivos-supervisor', periodoKey, search],
    queryFn: () => {
      if (!periodoSel) return []
      const p = new URLSearchParams({
        tipo_periodo: periodoSel.tipo_periodo,
        ano:          String(periodoSel.ano),
        periodo_seq:  String(periodoSel.periodo_seq),
        q:            search,
      })
      return fetch(`/api/objetivos/supervisor?${p}`).then(r => r.json())
    },
    enabled: !!periodoSel,
  })

  const totalAnterior = rows.reduce((s, r) => s + r.vl_anterior, 0)
  const totalCorrente = rows.reduce((s, r) => s + r.vl_corrente, 0)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-3 items-end">
        {/* Seletor de período */}
        <div className="space-y-1.5">
          <Label>Período</Label>
          <Select value={periodoKey} onValueChange={setPeriodoKey}>
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

        {/* Busca */}
        <div className="space-y-1.5 flex-1 min-w-48 max-w-xs">
          <Label>Busca</Label>
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 w-4 h-4 text-muted-foreground" />
            <Input
              className="pl-8"
              placeholder="Supervisor ou fornecedor..."
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
        </div>

        {isFetching && (
          <span className="text-xs text-muted-foreground pb-2">Carregando...</span>
        )}
      </div>

      {!periodoSel && periodos.length === 0 && (
        <p className="text-sm text-muted-foreground py-8 text-center">
          Nenhum objetivo importado ainda. Use a aba <strong>Importar</strong> para carregar dados.
        </p>
      )}

      {periodoSel && (
        <div className="border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40">
                <TableHead>Supervisor</TableHead>
                <TableHead>Fornecedor</TableHead>
                <TableHead className="text-center">RCAs</TableHead>
                <TableHead className="text-center">Produtos</TableHead>
                <TableHead className="text-center">Clientes</TableHead>
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
              {rows.map((row, i) => (
                <TableRow key={i}>
                  <TableCell className="text-xs">
                    <span className="text-muted-foreground">{row.cod_supervisor} </span>
                    {row.nome_supervisor}
                  </TableCell>
                  <TableCell className="text-xs">{row.fornecedor}</TableCell>
                  <TableCell className="text-center text-xs">{row.qtd_rcas}</TableCell>
                  <TableCell className="text-center text-xs">{row.qtd_produtos}</TableCell>
                  <TableCell className="text-center text-xs">{row.qtd_clientes}</TableCell>
                  <TableCell className="text-right text-xs">{fmt(row.vl_anterior)}</TableCell>
                  <TableCell className="text-right text-xs font-medium">{fmt(row.vl_corrente)}</TableCell>
                  <TableCell className={`text-right text-xs font-medium ${varClass(row.vl_anterior, row.vl_corrente)}`}>
                    {varPct(row.vl_anterior, row.vl_corrente)}
                  </TableCell>
                </TableRow>
              ))}
              {rows.length > 0 && (
                <TableRow className="bg-muted/40 font-semibold">
                  <TableCell colSpan={5} className="text-xs">Total ({rows.length} linhas)</TableCell>
                  <TableCell className="text-right text-xs">{fmt(totalAnterior)}</TableCell>
                  <TableCell className="text-right text-xs">{fmt(totalCorrente)}</TableCell>
                  <TableCell className={`text-right text-xs ${varClass(totalAnterior, totalCorrente)}`}>
                    {varPct(totalAnterior, totalCorrente)}
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
