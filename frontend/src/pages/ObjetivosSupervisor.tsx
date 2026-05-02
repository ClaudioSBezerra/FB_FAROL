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
import { Search } from 'lucide-react'

// ─── Tipos ────────────────────────────────────────────────────────────────────

type TipoPeriodo = 'MENSAL' | 'TRIMESTRAL' | 'SEMESTRAL' | 'ANUAL'

interface Periodo {
  tipo_periodo: TipoPeriodo
  ano: number
  periodo_seq: number
}

interface SupRow {
  cod_supervisor: number | null
  nome_supervisor: string
  cod_fornec: string
  fornecedor: string
  qtd_rcas: number
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

function KpiCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4 min-w-0">
      <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">{label}</p>
      <p className="text-xl font-bold text-slate-800 mt-1 truncate">{value}</p>
    </div>
  )
}

function KpiGrowth({ ant, cor }: { ant: number; cor: number }) {
  const n = varNum(ant, cor)
  const up = n !== null && n > 0
  const down = n !== null && n < 0
  return (
    <div className="bg-white rounded-xl border border-slate-100 shadow-sm p-4 min-w-0">
      <p className="text-[11px] font-semibold uppercase tracking-wider text-slate-400">Crescimento</p>
      <p className={`text-xl font-bold mt-1 ${up ? 'text-emerald-600' : down ? 'text-red-500' : 'text-slate-400'}`}>
        {varPct(ant, cor)}
      </p>
      <p className="text-xs text-slate-400 mt-0.5">vs período anterior</p>
    </div>
  )
}

function GrowthChip({ ant, cor }: { ant: number; cor: number }) {
  const n = varNum(ant, cor)
  const label = varPct(ant, cor)
  if (label === '—') return <span className="text-slate-300 text-xs">—</span>
  const up = n !== null && n > 0
  const down = n !== null && n < 0
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold ${
      up   ? 'bg-emerald-50 text-emerald-700' :
      down ? 'bg-red-50 text-red-600' :
             'bg-slate-100 text-slate-500'
    }`}>
      {label}
    </span>
  )
}

// ─── Componente ───────────────────────────────────────────────────────────────

export default function ObjetivosSupervisor() {
  const [periodoKey,   setPeriodoKey]   = useState('')
  const [supFilter,    setSupFilter]    = useState('_all')
  const [fornecFilter, setFornecFilter] = useState('')

  const { data: periodos = [] } = useQuery<Periodo[]>({
    queryKey: ['objetivos-periodos'],
    queryFn:  () => fetch('/api/objetivos/periodos').then(r => r.json()),
    staleTime: 5 * 60_000,
    gcTime:    10 * 60_000,
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

  const { data: rawRows, isFetching } = useQuery<SupRow[]>({
    queryKey: ['objetivos-supervisor-all', periodoKey],
    queryFn: () => {
      if (!periodoSel) return []
      const p = new URLSearchParams({
        tipo_periodo: periodoSel.tipo_periodo,
        ano:          String(periodoSel.ano),
        periodo_seq:  String(periodoSel.periodo_seq),
      })
      return fetch(`/api/objetivos/supervisor?${p}`).then(r => r.json())
    },
    enabled: !!periodoSel,
    staleTime: 2 * 60_000,
    gcTime:    10 * 60_000,
    refetchOnWindowFocus: false,
  })
  const allRows: SupRow[] = Array.isArray(rawRows) ? rawRows : []

  const supOptions = useMemo(() => {
    const seen = new Map<string, { nome: string; cod: number | null }>()
    allRows.forEach(r => {
      const key = r.cod_supervisor != null ? String(r.cod_supervisor) : '_null'
      seen.set(key, { nome: r.nome_supervisor, cod: r.cod_supervisor })
    })
    return Array.from(seen.entries())
      .map(([key, { nome, cod }]) => ({ key, nome, cod }))
      .sort((a, b) => (a.cod ?? 0) - (b.cod ?? 0))
  }, [allRows])

  const rows = useMemo(() => allRows.filter(r => {
    const supKey = r.cod_supervisor != null ? String(r.cod_supervisor) : '_null'
    return (supFilter === '_all' || supKey === supFilter) &&
           (!fornecFilter || r.fornecedor.toLowerCase().includes(fornecFilter.toLowerCase()))
  }), [allRows, supFilter, fornecFilter])

  const totalAnt  = rows.reduce((s, r) => s + r.vl_anterior, 0)
  const totalCor  = rows.reduce((s, r) => s + r.vl_corrente, 0)
  const totalTtal = rows.reduce((s, r) => s + r.ttal_itens, 0)
  const qtdSups   = new Set(rows.map(r => r.cod_supervisor)).size
  const qtdFornc  = new Set(rows.map(r => r.cod_fornec)).size
  const sumAtivos = rows.reduce((s, r) => s + r.cl_ativos, 0)
  const sumPosit  = rows.reduce((s, r) => s + r.posit_med, 0)
  const n         = varNum(totalAnt, totalCor)

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
          <Select value={periodoKey} onValueChange={v => { setPeriodoKey(v); setSupFilter('_all'); setFornecFilter('') }}>
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
          <Label>Supervisor</Label>
          <SearchableCombobox
            className="w-64"
            placeholder="Todos os supervisores"
            searchPlaceholder="Código ou nome..."
            value={supFilter}
            onChange={setSupFilter}
            options={[
              { value: '_all', label: 'Todos os supervisores' },
              ...supOptions.map(s => ({
                value: s.key,
                label: s.cod != null ? `${s.cod} — ${s.nome}` : s.nome,
              })),
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

        {isFetching && <span className="text-xs text-slate-400 pb-2 animate-pulse">Carregando...</span>}
      </div>

      {/* ── KPIs ── */}
      {allRows.length > 0 && (
        <div className="space-y-2">
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
            <KpiCard label="Obj. Anterior" value={fmtBRL(totalAnt)} />
            <KpiCard label="Obj. Atual"    value={fmtBRL(totalCor)} />
            <KpiGrowth ant={totalAnt} cor={totalCor} />
          </div>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-2">
            <KpiCard label="Supervisores" value={String(qtdSups)} />
            <KpiCard label="Fornecedores" value={String(qtdFornc)} />
            <KpiCard label="CL Ativos"    value={sumAtivos.toLocaleString('pt-BR')} />
            <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-4 min-w-0">
              <p className="text-[11px] font-semibold uppercase tracking-wider text-orange-400">Positivados</p>
              <p className="text-xl font-bold text-orange-600 mt-1">{sumPosit.toLocaleString('pt-BR')}</p>
            </div>
            <div className="bg-white rounded-xl border border-orange-100 shadow-sm p-4 min-w-0">
              <p className="text-[11px] font-semibold uppercase tracking-wider text-orange-400">Total Itens</p>
              <p className="text-xl font-bold text-orange-600 mt-1">{totalTtal.toLocaleString('pt-BR')}</p>
            </div>
          </div>
        </div>
      )}

      {/* ── Tabela ── */}
      {periodoSel && (
        <div className="rounded-xl border border-slate-100 shadow-sm overflow-auto bg-white">
          <Table className="text-xs whitespace-nowrap">
            <TableHeader>
              <TableRow className="bg-[#003366] hover:bg-[#003366]">
                {['Supervisor','Fornecedor','Obj. Anterior','Obj. Atual','Cresc.','CL Ativos','Posit Med','% Posit','Méd Itens','Total Itens'].map((h, i) => (
                  <TableHead key={h} className={`text-slate-200 text-[11px] font-semibold tracking-wide py-3 ${
                    i >= 2 && i <= 4 ? 'text-right' : i >= 5 ? 'text-center' : ''
                  } ${i >= 8 ? 'text-orange-300' : ''}`}>
                    {h}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.length === 0 && !isFetching && (
                <TableRow>
                  <TableCell colSpan={10} className="text-center text-slate-400 py-12 text-sm">
                    Nenhum resultado encontrado.
                  </TableCell>
                </TableRow>
              )}
              {rows.map((row, i) => (
                <TableRow key={i} className="hover:bg-slate-50/70 transition-colors border-b border-slate-50">
                  <TableCell className="py-2.5">
                    {row.cod_supervisor != null && (
                      <span className="text-slate-400 text-[10px] mr-1">{row.cod_supervisor}</span>
                    )}
                    <span className="font-medium text-slate-700">{row.nome_supervisor}</span>
                  </TableCell>
                  <TableCell className="text-slate-700 py-2.5">{row.fornecedor}</TableCell>
                  <TableCell className="text-right text-slate-400 tabular-nums py-2.5">{fmtBRL(row.vl_anterior)}</TableCell>
                  <TableCell className="text-right font-semibold text-slate-800 tabular-nums py-2.5">{fmtBRL(row.vl_corrente)}</TableCell>
                  <TableCell className="text-right py-2.5">
                    <GrowthChip ant={row.vl_anterior} cor={row.vl_corrente} />
                  </TableCell>
                  <TableCell className="text-center text-slate-600 tabular-nums py-2.5">{row.cl_ativos.toLocaleString('pt-BR')}</TableCell>
                  <TableCell className="text-center text-slate-600 tabular-nums py-2.5">{row.posit_med.toLocaleString('pt-BR')}</TableCell>
                  <TableCell className="text-center text-slate-600 py-2.5">{pctPosit(row.posit_med, row.cl_ativos)}</TableCell>
                  <TableCell className="text-center font-semibold text-orange-600 tabular-nums py-2.5">
                    {medItens(row.ttal_itens, row.posit_med)}
                  </TableCell>
                  <TableCell className="text-center font-semibold text-orange-600 tabular-nums py-2.5">
                    {row.ttal_itens.toLocaleString('pt-BR')}
                  </TableCell>
                </TableRow>
              ))}
              {rows.length > 0 && (
                <TableRow className="bg-slate-50 border-t-2 border-slate-200 font-semibold">
                  <TableCell colSpan={2} className="text-slate-500 text-[11px] py-2.5">
                    {rows.length} linha{rows.length !== 1 ? 's' : ''}
                  </TableCell>
                  <TableCell className="text-right text-slate-400 tabular-nums py-2.5">{fmtBRL(totalAnt)}</TableCell>
                  <TableCell className="text-right text-slate-800 tabular-nums py-2.5">{fmtBRL(totalCor)}</TableCell>
                  <TableCell className="text-right py-2.5">
                    <GrowthChip ant={totalAnt} cor={totalCor} />
                  </TableCell>
                  <TableCell className="text-center tabular-nums py-2.5">{sumAtivos.toLocaleString('pt-BR')}</TableCell>
                  <TableCell className="text-center tabular-nums py-2.5">{sumPosit.toLocaleString('pt-BR')}</TableCell>
                  <TableCell className="text-center py-2.5">{pctPosit(sumPosit, sumAtivos)}</TableCell>
                  <TableCell className="text-center text-orange-600 tabular-nums py-2.5">{medItens(totalTtal, sumPosit)}</TableCell>
                  <TableCell className="text-center text-orange-600 tabular-nums py-2.5">{totalTtal.toLocaleString('pt-BR')}</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
