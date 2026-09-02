import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { useAuth } from '@/contexts/AuthContext'
import { TrendingUp, TrendingDown, Target } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────

interface MetaVinculo {
  id: number
  industria_nome: string
  tipo_metrica_nome: string
}

interface Vigencia {
  id: number
  data_inicio: string
  data_fim: string
  status: 'aberta' | 'fechada'
}

interface RealizadoRede {
  rede_nome: string
  cod_rca: string
  valor: number
  atingiu: boolean
}

interface RealizadoGrupo {
  codigo: string
  nome: string
  realizado_total: number
  qtd_redes: number
  projecao: number
}

interface Realizado {
  realizado_total: number
  projecao: number
  redes: RealizadoRede[]
  grupos?: RealizadoGrupo[]
  parcial: boolean
}

interface PainelFaixa {
  faixa: number
  valor_meta: number
  atingida: boolean
}

interface Painel {
  industria_nome: string
  tipo_metrica_nome: string
  vigencia: Vigencia
  realizado: Realizado
  faixas: PainelFaixa[]
  faixa_atual: PainelFaixa | null
  proxima_faixa: PainelFaixa | null
  delta: number
}

const NIVEIS = [
  { value: 'rede', label: 'Redes' },
  { value: 'rca', label: 'RCA' },
  { value: 'crv', label: 'CRV (Supervisor)' },
  { value: 'ggv', label: 'GGV' },
]

const FLUXOS = [
  { value: 'faturado', label: 'Faturado' },
  { value: 'transmitido', label: 'Transmitido' },
  { value: 'soma', label: 'Soma' },
]

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function FarolPainelMetas() {
  const { token } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  const [vinculoID, setVinculoID] = useState('')
  const [vigenciaID, setVigenciaID] = useState('')
  const [nivel, setNivel] = useState('rede')
  const [fluxo, setFluxo] = useState('faturado')

  const { data: vinculos = [] } = useQuery<MetaVinculo[]>({
    queryKey: ['farol-metas-vinculos'],
    queryFn: async () => {
      const r = await fetch('/api/farol/metas-vinculos', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const { data: vigencias = [] } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', vinculoID],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${vinculoID}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: !!vinculoID,
  })

  const { data: painel, isLoading, isFetching } = useQuery<Painel>({
    queryKey: ['farol-metas-painel', vinculoID, vigenciaID, nivel, fluxo],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-painel?vinculo_id=${vinculoID}&vigencia_id=${vigenciaID}&fluxo=${fluxo}&nivel=${nivel}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: !!vinculoID && !!vigenciaID,
  })

  const linhas = nivel === 'rede'
    ? (painel?.realizado.redes ?? []).map(r => ({ nome: r.rede_nome, sub: r.cod_rca, valor: r.valor, marcador: r.atingiu }))
    : (painel?.realizado.grupos ?? []).map(g => ({ nome: g.nome || g.codigo, sub: `${g.qtd_redes} Rede(s)`, valor: g.realizado_total, marcador: undefined }))

  return (
    <div className="p-6 space-y-4">
      <div>
        <h1 className="text-xl font-semibold">Painel de Metas por Indústria</h1>
        <p className="text-sm text-muted-foreground">Meta × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede.</p>
      </div>

      <div className="flex flex-wrap gap-3 items-end border rounded-lg p-4">
        <div className="space-y-1">
          <label className="text-xs font-medium">Indústria / Tipo de Métrica</label>
          <Select value={vinculoID} onValueChange={v => { setVinculoID(v); setVigenciaID('') }}>
            <SelectTrigger className="w-64"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {vinculos.map(v => (
                <SelectItem key={v.id} value={String(v.id)}>{v.industria_nome} — {v.tipo_metrica_nome}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <label className="text-xs font-medium">Vigência</label>
          <Select value={vigenciaID} onValueChange={setVigenciaID}>
            <SelectTrigger className="w-56"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {vigencias.map(v => (
                <SelectItem key={v.id} value={String(v.id)}>
                  {v.data_inicio} – {v.data_fim} {v.status === 'fechada' ? '(fechada)' : ''}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <label className="text-xs font-medium">Nível</label>
          <Select value={nivel} onValueChange={setNivel}>
            <SelectTrigger className="w-44"><SelectValue /></SelectTrigger>
            <SelectContent>
              {NIVEIS.map(n => <SelectItem key={n.value} value={n.value}>{n.label}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <label className="text-xs font-medium">Fluxo</label>
          <Select value={fluxo} onValueChange={setFluxo}>
            <SelectTrigger className="w-40"><SelectValue /></SelectTrigger>
            <SelectContent>
              {FLUXOS.map(f => <SelectItem key={f.value} value={f.value}>{f.label}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
      </div>

      {!vinculoID || !vigenciaID ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Selecione a Indústria/Tipo de Métrica e a Vigência pra ver o painel.</p>
      ) : isLoading || isFetching ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
      ) : painel ? (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div className="border rounded-lg p-4">
              <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                <Target className="w-4 h-4" /> Realizado
              </div>
              <div className="text-2xl font-semibold">{painel.realizado.realizado_total.toLocaleString('pt-BR', { maximumFractionDigits: 2 })}</div>
              {painel.realizado.parcial && <Badge variant="secondary" className="mt-1">Mês em andamento</Badge>}
            </div>
            <div className="border rounded-lg p-4">
              <div className="text-muted-foreground text-xs mb-1">Meta atual (Faixa {painel.proxima_faixa?.faixa ?? painel.faixa_atual?.faixa ?? '—'})</div>
              <div className="text-2xl font-semibold">
                {(painel.proxima_faixa ?? painel.faixa_atual)?.valor_meta.toLocaleString('pt-BR', { maximumFractionDigits: 2 }) ?? '—'}
              </div>
            </div>
            <div className={`border rounded-lg p-4 ${painel.delta > 0 ? 'bg-amber-50' : 'bg-emerald-50'}`}>
              <div className="flex items-center gap-2 text-xs mb-1">
                {painel.delta > 0 ? <TrendingDown className="w-4 h-4 text-amber-600" /> : <TrendingUp className="w-4 h-4 text-emerald-600" />}
                Falta pra bater a meta
              </div>
              <div className="text-2xl font-semibold">
                {painel.delta > 0 ? painel.delta.toLocaleString('pt-BR', { maximumFractionDigits: 2 }) : 'Meta batida'}
              </div>
            </div>
          </div>

          <div className="border rounded-lg overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{nivel === 'rede' ? 'Rede' : NIVEIS.find(n => n.value === nivel)?.label}</TableHead>
                  <TableHead>{nivel === 'rede' ? 'RCA' : 'Composição'}</TableHead>
                  <TableHead className="text-right">Realizado</TableHead>
                  {nivel === 'rede' && <TableHead className="w-24 text-center">Status</TableHead>}
                </TableRow>
              </TableHeader>
              <TableBody>
                {linhas.length === 0 && (
                  <TableRow><TableCell colSpan={4} className="text-center py-8 text-muted-foreground">Sem dados pra este recorte</TableCell></TableRow>
                )}
                {linhas.map((l, i) => (
                  <TableRow key={i}>
                    <TableCell className="font-medium">{l.nome}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{l.sub}</TableCell>
                    <TableCell className="text-right">{l.valor.toLocaleString('pt-BR', { maximumFractionDigits: 2 })}</TableCell>
                    {nivel === 'rede' && (
                      <TableCell className="text-center">
                        <Badge variant={l.marcador ? 'default' : 'secondary'}>{l.marcador ? 'Coberta' : 'Não coberta'}</Badge>
                      </TableCell>
                    )}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      ) : null}
    </div>
  )
}
