import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { useAuth } from '@/contexts/AuthContext'
import { TrendingUp, TrendingDown, Target, AlertTriangle } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────

interface MetaVinculo {
  id: number
  industria_id: number
  industria_nome: string
  tipo_metrica_nome: string
  formula_codigo: string
}

interface Vigencia {
  id: number
  data_inicio: string
  data_fim: string
  status: 'aberta' | 'fechada'
}

interface RealizadoCliente {
  cnpj: string
  razao: string
  fantasia: string
  valor: number
}

interface RealizadoRede {
  cod_princ: string
  razao: string
  fantasia: string
  qt_lojas: number
  cod_ggv: string
  nome_ggv: string
  cod_crv: string
  nome_crv: string
  cod_rca: string
  nome_rca: string
  valor: number
  valor_total: number
  atingiu: boolean
  clientes?: RealizadoCliente[]
}

interface RealizadoGrupo {
  codigo: string
  nome: string
  realizado_total: number
  qtd_redes: number
  qtd_atingindo: number
  qtd_falta_atingir: number
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
  recortes?: Record<string, Realizado>
}

// ─── Types — visão combinada (Cobertura + Sortimento numa linha por Rede,
// pedido da JC em 2026-09-03, mesmo formato da planilha "Resumo Redes") ───────

interface PainelMetricaResumo {
  vinculo_id: number
  vigencia_id: number
  realizado_total: number
  projecao: number
  parcial: boolean
  faixas: PainelFaixa[]
  faixa_atual: PainelFaixa | null
  proxima_faixa: PainelFaixa | null
  delta: number
}

interface PainelCombinadoRede {
  cod_princ: string
  razao: string
  fantasia: string
  qt_lojas: number
  cod_ggv: string
  nome_ggv: string
  cod_crv: string
  nome_crv: string
  cod_rca: string
  nome_rca: string
  cobertura_valor: number
  cobertura_valor_total: number
  cobertura_objetivo: number
  cobertura_falta: number
  cobertura_atingiu: boolean
  sortimento_valor: number
  sortimento_objetivo: number
  sortimento_falta: number
}

interface PainelCombinado {
  industria_nome: string
  vigencia: Vigencia
  cobertura: PainelMetricaResumo
  sortimento: PainelMetricaResumo
  redes: PainelCombinadoRede[]
}

interface Industria {
  id: number
  nome: string
  cobertura?: MetaVinculo
  sortimento?: MetaVinculo
}

const RECORTES = [
  { value: 'dia_anterior', label: 'Dia anterior' },
  { value: 'semana', label: 'Última semana' },
  { value: 'mes', label: 'Mês corrente' },
  { value: 'ano_corrente', label: 'Ano corrente' },
]

// 5 níveis hierárquicos do painel (GGV → CRV → RCA → Rede → CNPJ), pedido
// direto do Heverton (2026-09-04). "ggv"/"crv"/"rca" mostram CONTAGEM de
// Redes atingindo/faltando (não médias — modelo real da JC, abas "Resumo
// GGvs Crvs"/"...Rcas"); "rede" mostra valor médio + status; "cnpj" abre a
// lista de lojas de UMA Rede selecionada (drill-down, não um nível
// selecionável direto).
const NIVEIS = [
  { value: 'ggv', label: 'GGV' },
  { value: 'crv', label: 'GGV / CRV' },
  { value: 'rca', label: 'GGV / CRV / RCA' },
  { value: 'rede', label: 'GGV / CRV / RCA / Rede' },
]

// Só 2 visões (orientação do Heverton, 2026-09-04: "somente 2 visões...
// mesma filosofia do Farol V1 em uso hoje") — a 3ª visão "Faturado +
// Emitido" que uma sessão anterior chegou a implementar foi removida.
const FLUXOS = [
  { value: 'faturado', label: 'Faturado' },
  { value: 'transmitido', label: 'Transmitido' },
]

const fmt = (n: number) => n.toLocaleString('pt-BR', { maximumFractionDigits: 2 })

// ─── Page ─────────────────────────────────────────────────────────────────────

// landingNivelPorPersona decide onde o usuário logado "cai" no painel — o
// escopo obrigatório de login (farol_escopo.go) já restringe os DADOS
// (o backend nunca deixa escapar fora do organograma dele); isto só decide
// o NÍVEL inicial de exibição, pra bater com "GGV cai no total dele, com
// opção de abrir" (orientação do Heverton, 2026-09-04):
// - GGV: cai no nível "ggv" (o backend devolve só o próprio grupo, 1 linha)
// - Supervisor: cai no nível "crv" (só o próprio CRV, 1 linha)
// - RCA: cai direto nas Redes dele (nível "rede")
// - Sem persona restrita (gerente_geral/diretor/ti/...): lista de GGVs.
function landingNivelPorPersona(tipoPersona: string | null): string {
  switch (tipoPersona) {
    case 'ggv': return 'ggv'
    case 'supervisor': return 'crv'
    case 'rca': return 'rede'
    default: return 'ggv'
  }
}

export default function FarolPainelMetas() {
  const { token, tipoPersona } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  const [industriaID, setIndustriaID] = useState('')
  const [metrica, setMetrica] = useState<'cobertura' | 'sortimento' | 'combinado'>('combinado')
  const [vigenciaID, setVigenciaID] = useState('') // modo individual (Cobertura OU Sortimento)
  const [vigenciaCombinadaKey, setVigenciaCombinadaKey] = useState('') // modo combinado — chave "data_inicio|data_fim"
  const [nivel, setNivel] = useState(() => landingNivelPorPersona(tipoPersona))
  const [fluxo, setFluxo] = useState('faturado')
  const [aba, setAba] = useState<'oficiais' | 'projecao'>('oficiais')

  // Drill-down: GGV → CRV → RCA → Rede → CNPJ. O backend só permite
  // ESTREITAR dentro do escopo de login (farol_escopo.go) — um GGV que
  // tentasse setar outro cod_ggv aqui seria ignorado no servidor.
  const [filtroGGV, setFiltroGGV] = useState<{ codigo: string; nome: string } | null>(null)
  const [filtroCRV, setFiltroCRV] = useState<{ codigo: string; nome: string } | null>(null)
  const [filtroRCA, setFiltroRCA] = useState<{ codigo: string; nome: string } | null>(null)
  const [redeAberta, setRedeAberta] = useState<RealizadoRede | null>(null) // nível 5 (CNPJ) — Rede escolhida

  // Ao trocar de Indústria/Vigência/Fluxo, o drill-down perdido de propósito
  // (senão um filtro de GGV ficaria "grudado" ao trocar de programa).
  useEffect(() => {
    setFiltroGGV(null); setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null)
    setNivel(landingNivelPorPersona(tipoPersona))
  }, [industriaID, tipoPersona])

  const abrirGrupo = (codigo: string, nome: string) => {
    if (nivel === 'ggv') { setFiltroGGV({ codigo, nome }); setNivel('crv') }
    else if (nivel === 'crv') { setFiltroCRV({ codigo, nome }); setNivel('rca') }
    else if (nivel === 'rca') { setFiltroRCA({ codigo, nome }); setNivel('rede') }
  }
  const voltarPara = (destino: 'ggv' | 'crv' | 'rca') => {
    if (destino === 'ggv') { setFiltroGGV(null); setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setNivel('ggv') }
    else if (destino === 'crv') { setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setNivel('crv') }
    else if (destino === 'rca') { setFiltroRCA(null); setRedeAberta(null); setNivel('rca') }
  }

  const { data: vinculos = [] } = useQuery<MetaVinculo[]>({
    queryKey: ['farol-metas-vinculos'],
    queryFn: async () => {
      const r = await fetch('/api/farol/metas-vinculos', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const industrias = useMemo<Industria[]>(() => {
    const porID = new Map<number, Industria>()
    for (const v of vinculos) {
      if (!porID.has(v.industria_id)) porID.set(v.industria_id, { id: v.industria_id, nome: v.industria_nome })
      const ind = porID.get(v.industria_id)!
      if (v.formula_codigo === 'cobertura_rede') ind.cobertura = v
      else if (v.formula_codigo === 'sortimento_rede') ind.sortimento = v
    }
    return Array.from(porID.values()).sort((a, b) => a.nome.localeCompare(b.nome))
  }, [vinculos])

  const industriaSelecionada = industrias.find(i => String(i.id) === industriaID)
  const metricasDisponiveis = useMemo(() => {
    const opcoes: Array<{ value: typeof metrica; label: string }> = []
    if (industriaSelecionada?.cobertura && industriaSelecionada?.sortimento) {
      opcoes.push({ value: 'combinado', label: 'Combinado (igual à planilha)' })
    }
    if (industriaSelecionada?.cobertura) opcoes.push({ value: 'cobertura', label: industriaSelecionada.cobertura.tipo_metrica_nome })
    if (industriaSelecionada?.sortimento) opcoes.push({ value: 'sortimento', label: industriaSelecionada.sortimento.tipo_metrica_nome })
    return opcoes
  }, [industriaSelecionada])

  // Garante que a métrica escolhida ainda existe pra indústria atual (ex:
  // trocou de indústria e a anterior tinha Combinado mas esta não tem).
  useEffect(() => {
    if (metricasDisponiveis.length === 0) return
    if (!metricasDisponiveis.some(m => m.value === metrica)) {
      setMetrica(metricasDisponiveis[0].value)
    }
  }, [metricasDisponiveis, metrica])

  const vinculoAtivo = metrica === 'cobertura' ? industriaSelecionada?.cobertura
    : metrica === 'sortimento' ? industriaSelecionada?.sortimento
    : undefined

  // ─── Modo individual (Cobertura OU Sortimento) — mesmo fluxo de sempre ───────

  const { data: vigencias = [] } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', vinculoAtivo?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${vinculoAtivo!.id}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: metrica !== 'combinado' && !!vinculoAtivo,
  })

  const { data: painel, isLoading, isFetching } = useQuery<Painel>({
    queryKey: ['farol-metas-painel', vinculoAtivo?.id, vigenciaID, nivel, fluxo, filtroGGV?.codigo, filtroCRV?.codigo, filtroRCA?.codigo, redeAberta?.cod_princ],
    queryFn: async () => {
      const p = new URLSearchParams({ vinculo_id: String(vinculoAtivo!.id), vigencia_id: vigenciaID, fluxo, nivel, recortes: '1' })
      if (filtroGGV) p.set('cod_ggv', filtroGGV.codigo)
      if (filtroCRV) p.set('cod_crv', filtroCRV.codigo)
      if (filtroRCA) p.set('cod_rca', filtroRCA.codigo)
      if (redeAberta) p.set('cod_princ', redeAberta.cod_princ)
      const r = await fetch(`/api/farol/metas-painel?${p}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: metrica !== 'combinado' && !!vinculoAtivo && !!vigenciaID,
  })

  // ─── Modo combinado — Cobertura + Sortimento juntos, uma linha por Rede ──────

  const { data: vigenciasCobertura = [] } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', industriaSelecionada?.cobertura?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${industriaSelecionada!.cobertura!.id}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: metrica === 'combinado' && !!industriaSelecionada?.cobertura,
  })
  const { data: vigenciasSortimento = [] } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', industriaSelecionada?.sortimento?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${industriaSelecionada!.sortimento!.id}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: metrica === 'combinado' && !!industriaSelecionada?.sortimento,
  })

  const periodosCombinados = useMemo(() => {
    const porPeriodo = new Map(vigenciasSortimento.map(v => [`${v.data_inicio}|${v.data_fim}`, v]))
    return vigenciasCobertura
      .filter(vc => porPeriodo.has(`${vc.data_inicio}|${vc.data_fim}`))
      .map(vc => ({ chave: `${vc.data_inicio}|${vc.data_fim}`, cobertura: vc, sortimento: porPeriodo.get(`${vc.data_inicio}|${vc.data_fim}`)! }))
  }, [vigenciasCobertura, vigenciasSortimento])

  const periodoSelecionado = periodosCombinados.find(p => p.chave === vigenciaCombinadaKey)

  const { data: painelCombinado, isLoading: isLoadingCombinado, isFetching: isFetchingCombinado } = useQuery<PainelCombinado>({
    queryKey: ['farol-metas-painel-combinado', industriaSelecionada?.cobertura?.id, industriaSelecionada?.sortimento?.id, periodoSelecionado?.chave, fluxo],
    queryFn: async () => {
      const p = new URLSearchParams({
        vinculo_cobertura_id: String(industriaSelecionada!.cobertura!.id),
        vigencia_cobertura_id: String(periodoSelecionado!.cobertura.id),
        vinculo_sortimento_id: String(industriaSelecionada!.sortimento!.id),
        vigencia_sortimento_id: String(periodoSelecionado!.sortimento.id),
        fluxo,
      })
      const r = await fetch(`/api/farol/metas-painel-combinado?${p}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: metrica === 'combinado' && !!periodoSelecionado,
  })

  // Nível 5 (CNPJ) é um drill-down de UMA Rede escolhida, não um valor de
  // `nivel` selecionável — por isso fica fora do enum NIVEIS/fetch e só lê
  // .clientes que já veio junto no Realizado da Rede.
  const linhas = redeAberta
    ? (redeAberta.clientes ?? []).map(c => ({
        nome: c.fantasia || c.razao || c.cnpj, sub: c.cnpj, valor: c.valor, marcador: undefined as boolean | undefined, drill: undefined as (() => void) | undefined,
      }))
    : nivel === 'rede'
    ? (painel?.realizado.redes ?? []).map(r => ({
        nome: r.fantasia || r.razao || r.cod_princ, sub: `${r.nome_rca || r.cod_rca} · ${r.qt_lojas} loja(s)`,
        valor: r.valor, marcador: r.atingiu as boolean | undefined, drill: () => setRedeAberta(r),
      }))
    : (painel?.realizado.grupos ?? []).map(g => ({
        nome: g.nome || g.codigo, sub: `${g.qtd_atingindo}/${g.qtd_redes} redes atingindo`,
        valor: g.qtd_atingindo, marcador: undefined as boolean | undefined, drill: () => abrirGrupo(g.codigo, g.nome || g.codigo),
      }))

  const podeAbrirLinha = !redeAberta && nivel !== 'rede'
  const nivelLabelAtual = redeAberta ? 'Rede/CNPJ' : NIVEIS.find(n => n.value === nivel)?.label ?? nivel

  return (
    <div className="p-6 space-y-4">
      <div>
        <h1 className="text-xl font-semibold">Painel de Metas por Indústria</h1>
        <p className="text-sm text-muted-foreground">Meta × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede.</p>
      </div>

      <div className="flex flex-wrap gap-3 items-end border rounded-lg p-4">
        <div className="space-y-1">
          <label className="text-xs font-medium">Indústria</label>
          <Select value={industriaID} onValueChange={v => { setIndustriaID(v); setVigenciaID(''); setVigenciaCombinadaKey('') }}>
            <SelectTrigger className="w-56"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {industrias.map(i => (
                <SelectItem key={i.id} value={String(i.id)}>{i.nome}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {industriaSelecionada && (
          <div className="space-y-1">
            <label className="text-xs font-medium">Visão</label>
            <Select value={metrica} onValueChange={v => { setMetrica(v as typeof metrica); setVigenciaID(''); setVigenciaCombinadaKey('') }}>
              <SelectTrigger className="w-64"><SelectValue /></SelectTrigger>
              <SelectContent>
                {metricasDisponiveis.map(m => <SelectItem key={m.value} value={m.value}>{m.label}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        )}
        {metrica === 'combinado' ? (
          <div className="space-y-1">
            <label className="text-xs font-medium">Vigência</label>
            <Select value={vigenciaCombinadaKey} onValueChange={setVigenciaCombinadaKey}>
              <SelectTrigger className="w-56"><SelectValue placeholder="Selecione" /></SelectTrigger>
              <SelectContent>
                {periodosCombinados.map(p => (
                  <SelectItem key={p.chave} value={p.chave}>
                    {p.cobertura.data_inicio} – {p.cobertura.data_fim} {p.cobertura.status === 'fechada' ? '(fechada)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : (
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
        )}
        {metrica !== 'combinado' && (
          <div className="space-y-1">
            <label className="text-xs font-medium">Nível</label>
            <Select value={nivel} onValueChange={v => { voltarPara('ggv'); setNivel(v) }}>
              <SelectTrigger className="w-56"><SelectValue /></SelectTrigger>
              <SelectContent>
                {NIVEIS.map(n => <SelectItem key={n.value} value={n.value}>{n.label}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="space-y-1">
          <label className="text-xs font-medium">Fluxo</label>
          <Select value={fluxo} onValueChange={setFluxo}>
            <SelectTrigger className="w-48"><SelectValue /></SelectTrigger>
            <SelectContent>
              {FLUXOS.map(f => <SelectItem key={f.value} value={f.value}>{f.label}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
      </div>

      {!industriaID ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Selecione a Indústria pra ver o painel.</p>
      ) : metrica === 'combinado' ? (
        !periodoSelecionado ? (
          <p className="text-sm text-muted-foreground py-8 text-center">
            {periodosCombinados.length === 0
              ? 'Nenhum período com Cobertura e Sortimento cadastrados pro mesmo intervalo de datas ainda.'
              : 'Selecione a Vigência pra ver o painel combinado.'}
          </p>
        ) : isLoadingCombinado || isFetchingCombinado ? (
          <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
        ) : painelCombinado ? (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div className="border rounded-lg p-4">
                <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                  <Target className="w-4 h-4" /> Cobertura — redes cobertas
                </div>
                <div className="text-2xl font-semibold">
                  {fmt(painelCombinado.cobertura.realizado_total)}
                  {painelCombinado.cobertura.proxima_faixa && (
                    <span className="text-sm text-muted-foreground"> / {fmt(painelCombinado.cobertura.proxima_faixa.valor_meta)} (Faixa {painelCombinado.cobertura.proxima_faixa.faixa})</span>
                  )}
                </div>
              </div>
              <div className="border rounded-lg p-4">
                <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                  <Target className="w-4 h-4" /> Sortimento — média de EANs
                </div>
                <div className="text-2xl font-semibold">
                  {fmt(painelCombinado.sortimento.realizado_total)}
                  {painelCombinado.sortimento.proxima_faixa && (
                    <span className="text-sm text-muted-foreground"> / {fmt(painelCombinado.sortimento.proxima_faixa.valor_meta)} (Faixa {painelCombinado.sortimento.proxima_faixa.faixa})</span>
                  )}
                </div>
              </div>
            </div>

            <div className="border rounded-lg overflow-hidden">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Rede</TableHead>
                    <TableHead>RCA</TableHead>
                    <TableHead className="text-right">Objetivo Cobertura</TableHead>
                    <TableHead className="text-right">Valor Venda Média</TableHead>
                    <TableHead className="text-right">Falta (R$)</TableHead>
                    <TableHead className="text-right">Objetivo EANs</TableHead>
                    <TableHead className="text-right">Qt Média EANs</TableHead>
                    <TableHead className="text-right">Falta EANs</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {painelCombinado.redes.length === 0 && (
                    <TableRow><TableCell colSpan={8} className="text-center py-8 text-muted-foreground">Sem dados pra este período</TableCell></TableRow>
                  )}
                  {painelCombinado.redes.map((r, i) => (
                    <TableRow key={i}>
                      <TableCell className="font-medium">{r.fantasia || r.razao || r.cod_princ}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">{r.nome_rca || r.cod_rca} · {r.qt_lojas} loja(s)</TableCell>
                      <TableCell className="text-right">{fmt(r.cobertura_objetivo)}</TableCell>
                      <TableCell className="text-right">{fmt(r.cobertura_valor)}</TableCell>
                      <TableCell className="text-right">{r.cobertura_atingiu ? <Badge variant="default">Coberta</Badge> : fmt(r.cobertura_falta)}</TableCell>
                      <TableCell className="text-right">{fmt(r.sortimento_objetivo)}</TableCell>
                      <TableCell className="text-right">{fmt(r.sortimento_valor)}</TableCell>
                      <TableCell className="text-right">{fmt(r.sortimento_falta)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </>
        ) : null
      ) : isLoading || isFetching ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
      ) : painel ? (
        <>
          <div className="flex gap-1 border-b">
            <button
              className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px ${aba === 'oficiais' ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground'}`}
              onClick={() => setAba('oficiais')}
            >
              Indicadores oficiais
            </button>
            <button
              className={`px-3 py-2 text-sm font-medium border-b-2 -mb-px ${aba === 'projecao' ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground'}`}
              onClick={() => setAba('projecao')}
            >
              Projeção
            </button>
          </div>

          {aba === 'projecao' ? (
            <div className="space-y-4">
              <div className="flex items-start gap-2 bg-amber-50 border border-amber-200 rounded-lg p-3 text-sm text-amber-800">
                <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
                <span>
                  Projeção de fechamento — <strong>estimativa</strong> com base no ritmo de realização até hoje (não é um número oficial do
                  programa; os indicadores oficiais ficam na outra aba).
                </span>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div className="border rounded-lg p-4">
                  <div className="text-muted-foreground text-xs mb-1">Realizado até hoje</div>
                  <div className="text-2xl font-semibold">{fmt(painel.realizado.realizado_total)}</div>
                </div>
                <div className="border rounded-lg p-4 bg-slate-50">
                  <div className="text-muted-foreground text-xs mb-1">Projeção de fechamento (estimativa)</div>
                  <div className="text-2xl font-semibold">{fmt(painel.realizado.projecao)}</div>
                </div>
              </div>

              {painel.recortes && (
                <div className="border rounded-lg overflow-hidden">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Recorte</TableHead>
                        <TableHead className="text-right">Realizado no período</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {RECORTES.map(r => (
                        <TableRow key={r.value}>
                          <TableCell className="font-medium">{r.label}</TableCell>
                          <TableCell className="text-right">
                            {painel.recortes?.[r.value]?.realizado_total !== undefined ? fmt(painel.recortes[r.value].realizado_total) : '—'}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </div>
          ) : (
          <>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div className="border rounded-lg p-4">
              <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                <Target className="w-4 h-4" /> Realizado
              </div>
              <div className="text-2xl font-semibold">{fmt(painel.realizado.realizado_total)}</div>
              {painel.realizado.parcial && <Badge variant="secondary" className="mt-1">Mês em andamento</Badge>}
            </div>
            <div className="border rounded-lg p-4">
              <div className="text-muted-foreground text-xs mb-1">Meta atual (Faixa {painel.proxima_faixa?.faixa ?? painel.faixa_atual?.faixa ?? '—'})</div>
              <div className="text-2xl font-semibold">
                {(painel.proxima_faixa ?? painel.faixa_atual)?.valor_meta !== undefined ? fmt((painel.proxima_faixa ?? painel.faixa_atual)!.valor_meta) : '—'}
              </div>
            </div>
            <div className={`border rounded-lg p-4 ${painel.delta > 0 ? 'bg-amber-50' : 'bg-emerald-50'}`}>
              <div className="flex items-center gap-2 text-xs mb-1">
                {painel.delta > 0 ? <TrendingDown className="w-4 h-4 text-amber-600" /> : <TrendingUp className="w-4 h-4 text-emerald-600" />}
                Falta pra bater a meta
              </div>
              <div className="text-2xl font-semibold">
                {painel.delta > 0 ? fmt(painel.delta) : 'Meta batida'}
              </div>
            </div>
          </div>

          {/* Breadcrumb do drill-down GGV → CRV → RCA → Rede → CNPJ */}
          {(filtroGGV || filtroCRV || filtroRCA || redeAberta) && (
            <div className="flex flex-wrap items-center gap-1 text-sm text-muted-foreground">
              <button className="hover:underline text-primary" onClick={() => voltarPara('ggv')}>GGVs</button>
              {filtroGGV && (<><span>/</span><button className={`hover:underline ${!filtroCRV && !redeAberta ? 'font-medium text-foreground' : 'text-primary'}`} onClick={() => voltarPara('crv')}>{filtroGGV.nome}</button></>)}
              {filtroCRV && (<><span>/</span><button className={`hover:underline ${!filtroRCA && !redeAberta ? 'font-medium text-foreground' : 'text-primary'}`} onClick={() => voltarPara('rca')}>{filtroCRV.nome}</button></>)}
              {filtroRCA && (<><span>/</span><span className={!redeAberta ? 'font-medium text-foreground' : ''}>{filtroRCA.nome}</span></>)}
              {redeAberta && (<><span>/</span><span className="font-medium text-foreground">{redeAberta.fantasia || redeAberta.razao || redeAberta.cod_princ}</span></>)}
            </div>
          )}

          <div className="border rounded-lg overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{redeAberta ? 'CNPJ' : nivel === 'rede' ? 'Rede' : nivelLabelAtual}</TableHead>
                  <TableHead>{redeAberta ? 'Documento' : nivel === 'rede' ? 'RCA' : 'Composição'}</TableHead>
                  <TableHead className="text-right">{nivel === 'rede' || redeAberta ? 'Realizado' : 'Redes atingindo'}</TableHead>
                  {(nivel === 'rede' || redeAberta) && !redeAberta && <TableHead className="w-24 text-center">Status</TableHead>}
                  {podeAbrirLinha && <TableHead className="w-10" />}
                </TableRow>
              </TableHeader>
              <TableBody>
                {linhas.length === 0 && (
                  <TableRow><TableCell colSpan={3 + (nivel === 'rede' && !redeAberta ? 1 : 0) + (podeAbrirLinha ? 1 : 0)} className="text-center py-8 text-muted-foreground">Sem dados pra este recorte</TableCell></TableRow>
                )}
                {linhas.map((l, i) => (
                  <TableRow key={i} className={l.drill ? 'cursor-pointer hover:bg-muted/50' : undefined} onClick={l.drill}>
                    <TableCell className="font-medium">{l.nome}</TableCell>
                    <TableCell className="text-sm text-muted-foreground">{l.sub}</TableCell>
                    <TableCell className="text-right">{fmt(l.valor)}</TableCell>
                    {nivel === 'rede' && !redeAberta && (
                      <TableCell className="text-center">
                        <Badge variant={l.marcador ? 'default' : 'secondary'}>{l.marcador ? 'Coberta' : 'Não coberta'}</Badge>
                      </TableCell>
                    )}
                    {podeAbrirLinha && <TableCell className="text-center text-muted-foreground">›</TableCell>}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          </>
          )}
        </>
      ) : null}
    </div>
  )
}
