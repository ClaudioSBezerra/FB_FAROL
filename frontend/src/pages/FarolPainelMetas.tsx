import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useAuth } from '@/contexts/AuthContext'
import { TrendingUp, TrendingDown, Target, AlertTriangle, PackageSearch } from 'lucide-react'
import { fmtBRL } from '@/lib/farolMoney'

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
  uf: string
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
  clientes?: { cnpj: string; nome: string }[]
}

interface PainelCombinadoCliente {
  cod_princ: string
  cnpj: string
  razao: string
  fantasia: string
  uf: string
  cod_ggv: string
  nome_ggv: string
  cod_crv: string
  nome_crv: string
  cod_rca: string
  nome_rca: string
  cobertura_valor: number
  cobertura_objetivo: number
  sortimento_valor: number
  sortimento_objetivo: number
}

interface PainelCombinado {
  industria_nome: string
  vigencia: Vigencia
  cobertura: PainelMetricaResumo
  sortimento: PainelMetricaResumo
  redes: PainelCombinadoRede[]
  clientes: PainelCombinadoCliente[]
  data_inicio_usada: string
  data_fim_usada: string
}

// PainelItemLinha — 1 linha do drill-down "Itens" (Sortimento): quais EANs
// venderam/não venderam numa Rede ou Loja, com Qtd e Valor — pedido do
// Claudio em 10/09/2026.
interface PainelItemLinha {
  ean: string
  nome: string
  qtd: number
  valor: number
  vendeu: boolean
}

// AbaCombinado — as 4 abas da visão Combinado, cada uma batizada e
// estruturada igual à aba real da planilha modelo da JC ("Unico
// Acompanhamento Ponderadas Unilever"): GGVxCRV e GGVxCRVxRCA são rollups
// (contagem de Redes atingindo/faltando cada métrica); Redes e Cliente são
// linha-a-linha com os valores. Mesmo espírito do painel geral (abas fixas
// no topo, como "Por FORN.GERAL"/"Por Gerência"/"Por Equipe"), mas os
// nomes vêm das abas da planilha, não da nomenclatura do painel geral.
type AbaCombinado = 'ggv_crv' | 'ggv_crv_rca' | 'rede' | 'cliente'
const ABAS_COMBINADO: { value: AbaCombinado; label: string }[] = [
  { value: 'ggv_crv', label: 'Resumo GGVs×CRVs' },
  { value: 'ggv_crv_rca', label: 'Resumo GGVs×CRVs×RCAs' },
  { value: 'rede', label: 'Resumo Redes' },
  { value: 'cliente', label: 'Resumo Rede×Cliente' },
]

interface GrupoCombinado {
  cod_ggv: string
  nome_ggv: string
  cod_crv: string
  nome_crv: string
  cod_rca?: string
  nome_rca?: string
  qtd_redes: number
  qtd_atingindo_cobertura: number
  qtd_falta_cobertura: number
  qtd_atingindo_sortimento: number
  qtd_falta_sortimento: number
}

// agruparCombinado — rollup client-side (sem round-trip extra: as Redes já
// filtradas trazem tudo que este cálculo precisa) por par GGV+CRV ou trio
// GGV+CRV+RCA, contando quantas Redes atingem/faltam Cobertura e
// Sortimento — mesmo indicador das abas "Resumo GGvs Crvs"/"...Rcas" da
// planilha (QT REDES ATINGINDO/FALTA ATINGIR, pras duas métricas).
function agruparCombinado(redes: PainelCombinadoRede[], comRCA: boolean): GrupoCombinado[] {
  const ordem: string[] = []
  const porChave = new Map<string, GrupoCombinado>()
  for (const r of redes) {
    const chave = comRCA ? `${r.cod_ggv}|${r.cod_crv}|${r.cod_rca}` : `${r.cod_ggv}|${r.cod_crv}`
    let g = porChave.get(chave)
    if (!g) {
      g = {
        cod_ggv: r.cod_ggv, nome_ggv: r.nome_ggv, cod_crv: r.cod_crv, nome_crv: r.nome_crv,
        ...(comRCA ? { cod_rca: r.cod_rca, nome_rca: r.nome_rca } : {}),
        qtd_redes: 0, qtd_atingindo_cobertura: 0, qtd_falta_cobertura: 0,
        qtd_atingindo_sortimento: 0, qtd_falta_sortimento: 0,
      }
      porChave.set(chave, g)
      ordem.push(chave)
    }
    g.qtd_redes++
    if (r.cobertura_atingiu) g.qtd_atingindo_cobertura++; else g.qtd_falta_cobertura++
    if (r.sortimento_valor >= r.sortimento_objetivo) g.qtd_atingindo_sortimento++; else g.qtd_falta_sortimento++
  }
  return ordem.map(k => porChave.get(k)!)
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

// StatusBadge — "Coberta"/"Não coberta": VERDE pra atingido, VERMELHO pra
// não atingido (padrão de cor do Farol inteiro — o Badge variant="default"
// do shadcn usa a cor PRIMÁRIA do tema, não verde/vermelho, então precisa
// de classes explícitas aqui).
function StatusBadge({ atingiu, labelSim = 'Coberta', labelNao = 'Não coberta' }: {
  atingiu: boolean
  labelSim?: string
  labelNao?: string
}) {
  return (
    <Badge className={atingiu
      ? 'bg-emerald-100 text-emerald-700 border-emerald-200 hover:bg-emerald-100'
      : 'bg-red-100 text-red-700 border-red-200 hover:bg-red-100'}
    >
      {atingiu ? labelSim : labelNao}
    </Badge>
  )
}

// primeiroEUltimoDiaDoMes — default do filtro "Período: de/até" (pedido do
// Claudio em 10/09/2026): início do mês corrente até o final do mês
// corrente, no fuso do navegador (mesma convenção de data YYYY-MM-DD usada
// em toda a barra de filtros/vigências deste painel).
function primeiroEUltimoDiaDoMes(): { inicio: string; fim: string } {
  const hoje = new Date()
  const y = hoje.getFullYear(), m = hoje.getMonth()
  const fmtd = (d: Date) => d.toISOString().slice(0, 10)
  return { inicio: fmtd(new Date(y, m, 1)), fim: fmtd(new Date(y, m + 1, 0)) }
}

// dedup — lista de {v: código, l: rótulo} única por código, ordenada pelo
// rótulo. Alimenta os selects da barra de filtros da visão Combinada.
function dedup(items: { v: string; l: string }[]) {
  const m = new Map<string, string>()
  for (const it of items) if (it.v && !m.has(it.v)) m.set(it.v, it.l)
  return [...m.entries()].map(([v, l]) => ({ v, l })).sort((a, b) => a.l.localeCompare(b.l))
}

// FiltroSelect — select "Todos + opções" da barra de filtros da visão
// Combinada. Radix Select não aceita value="" num item, daí o sentinel.
function FiltroSelect({ label, value, onChange, opts }: {
  label: string
  value: string
  onChange: (v: string) => void
  opts: { v: string; l: string }[]
}) {
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium">{label}</label>
      <Select value={value || '__all__'} onValueChange={v => onChange(v === '__all__' ? '' : v)}>
        <SelectTrigger className="w-48"><SelectValue /></SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">Todos</SelectItem>
          {opts.map(o => <SelectItem key={o.v} value={o.v}>{o.l}</SelectItem>)}
        </SelectContent>
      </Select>
    </div>
  )
}

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

  // Filtro "Período: de/até" (pedido do Claudio em 10/09/2026) — default =
  // os bounds da vigência escolhida (que, pra vigência aberta/corrente,
  // geralmente JÁ é "início do mês corrente até final do mês corrente").
  // Editável: o usuário pode estreitar pra uma janela menor dentro da
  // vigência. O backend (farol_metas_painel_combinado.go) só recalcula ao
  // vivo quando essas datas DIFEREM dos bounds da vigência — do contrário
  // respeita o congelamento normalmente (FR17), então não custa nada
  // mandar essas datas sempre preenchidas.
  const [periodoManualInicio, setPeriodoManualInicio] = useState('')
  const [periodoManualFim, setPeriodoManualFim] = useState('')
  useEffect(() => {
    if (periodoSelecionado) {
      setPeriodoManualInicio(periodoSelecionado.cobertura.data_inicio)
      setPeriodoManualFim(periodoSelecionado.cobertura.data_fim)
    }
  }, [periodoSelecionado?.chave])

  const { data: painelCombinado, isLoading: isLoadingCombinado, isFetching: isFetchingCombinado } = useQuery<PainelCombinado>({
    queryKey: ['farol-metas-painel-combinado', industriaSelecionada?.cobertura?.id, industriaSelecionada?.sortimento?.id, periodoSelecionado?.chave, fluxo, periodoManualInicio, periodoManualFim],
    queryFn: async () => {
      const p = new URLSearchParams({
        vinculo_cobertura_id: String(industriaSelecionada!.cobertura!.id),
        vigencia_cobertura_id: String(periodoSelecionado!.cobertura.id),
        vinculo_sortimento_id: String(industriaSelecionada!.sortimento!.id),
        vigencia_sortimento_id: String(periodoSelecionado!.sortimento.id),
        fluxo,
      })
      if (periodoManualInicio && periodoManualFim) {
        p.set('data_inicio', periodoManualInicio); p.set('data_fim', periodoManualFim)
      }
      const r = await fetch(`/api/farol/metas-painel-combinado?${p}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: metrica === 'combinado' && !!periodoSelecionado && !!periodoManualInicio && !!periodoManualFim,
  })

  // ─── Barra de filtros da visão Combinada (client-side sobre .redes) ─────────
  // Todos os campos já vêm na resposta (~120 Redes), então filtrar aqui evita
  // re-fetch. Os selects são EM CASCATA: GGV limita CRV, que limita RCA, que
  // limita Rede; "Cliente" só estreita a lista de Redes (mostra a Rede que
  // contém aquele CNPJ), sem trocar a granularidade — decisão do Claudio.
  // UF é ORTOGONAL à hierarquia (não cascateia com GGV/CRV/RCA/Rede).
  const [fGGV, setFGGV] = useState('')
  const [fCRV, setFCRV] = useState('')
  const [fRCA, setFRCA] = useState('')
  const [fRede, setFRede] = useState('')
  const [fCliente, setFCliente] = useState('')
  const [fUF, setFUF] = useState('')
  const limparFiltrosCombinado = () => { setFGGV(''); setFCRV(''); setFRCA(''); setFRede(''); setFCliente(''); setFUF('') }
  useEffect(() => {
    setFGGV(''); setFCRV(''); setFRCA(''); setFRede(''); setFCliente(''); setFUF('')
  }, [industriaID, metrica, vigenciaCombinadaKey, fluxo])

  const redesCombinado = painelCombinado?.redes ?? []
  const optsGGV = useMemo(
    () => dedup(redesCombinado.map(r => ({ v: r.cod_ggv, l: `${r.cod_ggv} — ${r.nome_ggv}` }))),
    [redesCombinado],
  )
  const optsCRV = useMemo(
    () => dedup(redesCombinado.filter(r => !fGGV || r.cod_ggv === fGGV)
      .map(r => ({ v: r.cod_crv, l: `${r.cod_crv} — ${r.nome_crv}` }))),
    [redesCombinado, fGGV],
  )
  const optsRCA = useMemo(
    () => dedup(redesCombinado.filter(r => (!fGGV || r.cod_ggv === fGGV) && (!fCRV || r.cod_crv === fCRV))
      .map(r => ({ v: r.cod_rca, l: `${r.cod_rca} — ${r.nome_rca}` }))),
    [redesCombinado, fGGV, fCRV],
  )
  const optsRede = useMemo(
    () => dedup(redesCombinado
      .filter(r => (!fGGV || r.cod_ggv === fGGV) && (!fCRV || r.cod_crv === fCRV) && (!fRCA || r.cod_rca === fRCA))
      .map(r => ({ v: r.cod_princ, l: r.fantasia || r.razao || r.cod_princ }))),
    [redesCombinado, fGGV, fCRV, fRCA],
  )
  const optsCliente = useMemo(() => {
    const seen = new Map<string, string>()
    for (const r of redesCombinado.filter(r =>
      (!fGGV || r.cod_ggv === fGGV) && (!fCRV || r.cod_crv === fCRV) &&
      (!fRCA || r.cod_rca === fRCA) && (!fRede || r.cod_princ === fRede))) {
      for (const c of r.clientes ?? []) if (!seen.has(c.cnpj)) seen.set(c.cnpj, `${c.nome || c.cnpj} (${c.cnpj})`)
    }
    return [...seen.entries()].map(([v, l]) => ({ v, l })).sort((a, b) => a.l.localeCompare(b.l))
  }, [redesCombinado, fGGV, fCRV, fRCA, fRede])
  // UF — não vem no CSV de Clientes Válidos (só nas linhas de venda);
  // resolvido pelo backend a partir da venda mais recente do CNPJ "dono"
  // da Rede (ver resolverUFClientes, farol_metas_painel_combinado.go).
  // Filtro ORTOGONAL: não cascateia com GGV/CRV/RCA/Rede, só narrowing.
  const optsUF = useMemo(
    () => dedup(redesCombinado.filter(r => r.uf).map(r => ({ v: r.uf, l: r.uf }))),
    [redesCombinado],
  )

  const redesVisiveis = useMemo(
    () => redesCombinado.filter(r =>
      (!fGGV || r.cod_ggv === fGGV) &&
      (!fCRV || r.cod_crv === fCRV) &&
      (!fRCA || r.cod_rca === fRCA) &&
      (!fRede || r.cod_princ === fRede) &&
      (!fUF || r.uf === fUF) &&
      (!fCliente || (r.clientes ?? []).some(c => c.cnpj === fCliente))),
    [redesCombinado, fGGV, fCRV, fRCA, fRede, fUF, fCliente],
  )

  // TOTAL — soma só das colunas que a planilha "Resumo Redes" soma na última
  // linha (Obj. Cobertura, Valor Venda, Obj. EANs, Qt Méd. EANs, Falta EANs);
  // médias e Falta (R$) não são somadas (não faz sentido somar média).
  const totComb = useMemo(() => {
    const t = { objCob: 0, valorVenda: 0, objEan: 0, qtMedEan: 0, faltaEan: 0, cob: 0, sort: 0 }
    for (const r of redesVisiveis) {
      t.objCob += r.cobertura_objetivo
      t.valorVenda += r.cobertura_valor_total
      t.objEan += r.sortimento_objetivo
      t.qtMedEan += r.sortimento_valor
      t.faltaEan += r.sortimento_valor - r.sortimento_objetivo
      if (r.cobertura_atingiu) t.cob++
      if (r.sortimento_valor >= r.sortimento_objetivo) t.sort++
    }
    return t
  }, [redesVisiveis])

  // aba ativa da visão Combinado — ver AbaCombinado/ABAS_COMBINADO acima.
  const [abaCombinado, setAbaCombinado] = useState<AbaCombinado>('rede')

  const clientesCombinado = painelCombinado?.clientes ?? []
  const clientesVisiveis = useMemo(
    () => clientesCombinado.filter(c =>
      (!fGGV || c.cod_ggv === fGGV) &&
      (!fCRV || c.cod_crv === fCRV) &&
      (!fRCA || c.cod_rca === fRCA) &&
      (!fRede || c.cod_princ === fRede) &&
      (!fUF || c.uf === fUF) &&
      (!fCliente || c.cnpj === fCliente)),
    [clientesCombinado, fGGV, fCRV, fRCA, fRede, fUF, fCliente],
  )

  const gruposGGVCRV = useMemo(() => agruparCombinado(redesVisiveis, false), [redesVisiveis])
  const gruposGGVCRVRCA = useMemo(() => agruparCombinado(redesVisiveis, true), [redesVisiveis])

  // ─── Drill-down "Itens" (Sortimento): vendeu/não vendeu, Qtd e Valor —
  // clicar numa Rede (aba "Resumo Redes") mostra os itens de TODAS as
  // lojas dela; clicar numa loja (aba "Resumo Rede×Cliente") mostra só os
  // itens daquele CNPJ. Pedido do Claudio em 10/09/2026.
  const [itensAlvo, setItensAlvo] = useState<{ codPrinc?: string; cnpj?: string; titulo: string } | null>(null)
  const { data: itensResp, isLoading: isLoadingItens } = useQuery<{ itens: PainelItemLinha[] }>({
    queryKey: ['farol-metas-painel-itens', industriaSelecionada?.sortimento?.id, periodoSelecionado?.sortimento.id, fluxo, itensAlvo?.codPrinc, itensAlvo?.cnpj],
    queryFn: async () => {
      const p = new URLSearchParams({
        vinculo_sortimento_id: String(industriaSelecionada!.sortimento!.id),
        vigencia_sortimento_id: String(periodoSelecionado!.sortimento.id),
        fluxo,
      })
      if (itensAlvo!.cnpj) p.set('cnpj', itensAlvo!.cnpj)
      else p.set('cod_princ', itensAlvo!.codPrinc!)
      const r = await fetch(`/api/farol/metas-painel-itens?${p}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: !!itensAlvo && !!industriaSelecionada?.sortimento && !!periodoSelecionado,
  })
  const itensLista = itensResp?.itens ?? []

  // Nível 5 (CNPJ) é um drill-down de UMA Rede escolhida, não um valor de
  // `nivel` selecionável — por isso fica fora do enum NIVEIS/fetch e só lê
  // .clientes que já veio junto no Realizado da Rede.
  // Cobertura mede R$ por loja/Rede; Sortimento mede qtd de EANs — só a
  // primeira usa formatação monetária linha a linha (pedido do Claudio em
  // 10/09/2026, "colocar o R$ ao lado do Valor").
  const ehCobertura = vinculoAtivo?.formula_codigo === 'cobertura_rede'
  const linhas = redeAberta
    ? (redeAberta.clientes ?? []).map(c => ({
        // Rede/qt_lojas não se aplica no nível 5 (CNPJ é uma loja só) —
        // aqui "nome" já é a loja, sem contagem de lojas ao lado.
        nome: c.fantasia || c.razao || c.cnpj, sub: c.cnpj, valor: c.valor, marcador: undefined as boolean | undefined, drill: undefined as (() => void) | undefined,
      }))
    : nivel === 'rede'
    ? (painel?.realizado.redes ?? []).map(r => ({
        // Qt de lojas AO LADO do nome da Rede (pedido do Claudio em
        // 10/09/2026) — RCA fica isolado no "sub", sem misturar os dois.
        nome: `${r.fantasia || r.razao || r.cod_princ} (${r.qt_lojas} loja${r.qt_lojas === 1 ? '' : 's'})`,
        sub: r.nome_rca || r.cod_rca,
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
        <h1 className="text-xl font-semibold">Painel de Objetivos por Indústria</h1>
        <p className="text-sm text-muted-foreground">Objetivo × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede.</p>
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
            <label className="text-xs font-medium">Período</label>
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
            {/* Abas fixas — 1 por aba real da planilha modelo da JC, mesmo
                espírito das abas do painel geral (Por FORN.GERAL/Por
                Gerência/Por Equipe): trocar de aba só muda o RECORTE/
                granularidade exibido, os filtros abaixo continuam valendo
                em qualquer uma. */}
            <div className="flex rounded-md border border-slate-300 overflow-hidden bg-white shadow-sm w-fit">
              {ABAS_COMBINADO.map(a => (
                <button
                  key={a.value}
                  onClick={() => setAbaCombinado(a.value)}
                  className={`px-3 py-1.5 text-sm font-medium transition-colors ${
                    abaCombinado === a.value ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
                  }`}
                >
                  {a.label}
                </button>
              ))}
            </div>

            <div className="flex flex-wrap gap-2 items-end border rounded-lg p-4">
              <div className="space-y-1">
                <label className="text-xs font-medium">Período</label>
                <div className="flex items-center gap-1">
                  <input type="date" value={periodoManualInicio} onChange={e => setPeriodoManualInicio(e.target.value)}
                    className="h-10 rounded-md border border-input bg-background px-2 text-sm" />
                  <span className="text-xs text-muted-foreground">até</span>
                  <input type="date" value={periodoManualFim} onChange={e => setPeriodoManualFim(e.target.value)}
                    className="h-10 rounded-md border border-input bg-background px-2 text-sm" />
                </div>
              </div>
              <FiltroSelect label="GGV" value={fGGV} opts={optsGGV}
                onChange={v => { setFGGV(v); setFCRV(''); setFRCA(''); setFRede(''); setFCliente('') }} />
              <FiltroSelect label="Supervisor (CRV)" value={fCRV} opts={optsCRV}
                onChange={v => { setFCRV(v); setFRCA(''); setFRede(''); setFCliente('') }} />
              <FiltroSelect label="RCA" value={fRCA} opts={optsRCA}
                onChange={v => { setFRCA(v); setFRede(''); setFCliente('') }} />
              <FiltroSelect label="UF" value={fUF} opts={optsUF} onChange={setFUF} />
              <FiltroSelect label="Rede" value={fRede} opts={optsRede}
                onChange={v => { setFRede(v); setFCliente('') }} />
              <FiltroSelect label="Cliente" value={fCliente} opts={optsCliente} onChange={setFCliente} />
              {(fGGV || fCRV || fRCA || fRede || fUF || fCliente) && (
                <button className="text-xs text-primary hover:underline pb-2.5" onClick={limparFiltrosCombinado}>
                  Limpar filtros
                </button>
              )}
              {painelCombinado && (
                <button
                  className="text-xs text-primary hover:underline pb-2.5 ml-auto"
                  onClick={() => { const { inicio, fim } = primeiroEUltimoDiaDoMes(); setPeriodoManualInicio(inicio); setPeriodoManualFim(fim) }}
                >
                  Mês corrente
                </button>
              )}
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div className="border rounded-lg p-4">
                <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                  <Target className="w-4 h-4" /> Cobertura — redes cobertas
                </div>
                <div className="text-2xl font-semibold">
                  {totComb.cob} <span className="text-sm text-muted-foreground">/ {redesVisiveis.length} redes</span>
                </div>
              </div>
              <div className="border rounded-lg p-4">
                <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                  <Target className="w-4 h-4" /> Sortimento — redes no objetivo de EANs
                </div>
                <div className="text-2xl font-semibold">
                  {totComb.sort} <span className="text-sm text-muted-foreground">/ {redesVisiveis.length} redes</span>
                </div>
              </div>
            </div>

            {(abaCombinado === 'ggv_crv' || abaCombinado === 'ggv_crv_rca') && (
              <div className="border rounded-lg overflow-x-auto [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>GGV</TableHead>
                      <TableHead>CRV</TableHead>
                      {abaCombinado === 'ggv_crv_rca' && <TableHead>RCA</TableHead>}
                      <TableHead className="text-right">Qt Redes</TableHead>
                      <TableHead className="text-right">Atingindo Cobertura</TableHead>
                      <TableHead className="text-right">Falta Atingir Cobertura</TableHead>
                      <TableHead className="text-right">Atingindo EAN</TableHead>
                      <TableHead className="text-right">Falta Atingir EAN</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(abaCombinado === 'ggv_crv' ? gruposGGVCRV : gruposGGVCRVRCA).length === 0 && (
                      <TableRow><TableCell colSpan={abaCombinado === 'ggv_crv_rca' ? 8 : 7} className="text-center py-8 text-muted-foreground">Sem dados pra este recorte/filtros</TableCell></TableRow>
                    )}
                    {(abaCombinado === 'ggv_crv' ? gruposGGVCRV : gruposGGVCRVRCA).map((g, i) => (
                      <TableRow key={i}>
                        <TableCell className="text-sm whitespace-nowrap">{g.cod_ggv} — {g.nome_ggv}</TableCell>
                        <TableCell className="text-sm whitespace-nowrap">{g.cod_crv} — {g.nome_crv}</TableCell>
                        {abaCombinado === 'ggv_crv_rca' && <TableCell className="text-sm whitespace-nowrap">{g.cod_rca} — {g.nome_rca}</TableCell>}
                        <TableCell className="text-right">{g.qtd_redes}</TableCell>
                        <TableCell className="text-right text-emerald-600">{g.qtd_atingindo_cobertura}</TableCell>
                        <TableCell className="text-right text-red-600">{g.qtd_falta_cobertura}</TableCell>
                        <TableCell className="text-right text-emerald-600">{g.qtd_atingindo_sortimento}</TableCell>
                        <TableCell className="text-right text-red-600">{g.qtd_falta_sortimento}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}

            {abaCombinado === 'rede' && (
              <div className="border rounded-lg overflow-x-auto [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                <p className="text-xs text-muted-foreground px-3 pt-2">Clique numa Rede pra ver os itens que venderam e não venderam.</p>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Cód. Princ.</TableHead>
                      <TableHead>Razão</TableHead>
                      <TableHead>Fantasia</TableHead>
                      <TableHead className="text-right">Qt Lojas</TableHead>
                      <TableHead>UF</TableHead>
                      <TableHead>GGV</TableHead>
                      <TableHead>CRV</TableHead>
                      <TableHead>RCA</TableHead>
                      <TableHead className="text-right">Obj. Cobertura</TableHead>
                      <TableHead className="text-right">Valor Venda</TableHead>
                      <TableHead className="text-right">Venda Média</TableHead>
                      <TableHead className="text-right">Falta (R$)</TableHead>
                      <TableHead className="text-center">Status</TableHead>
                      <TableHead className="text-right">Obj. EANs</TableHead>
                      <TableHead className="text-right">Qt Méd. EANs</TableHead>
                      <TableHead className="text-right">Falta EANs</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {redesVisiveis.length === 0 && (
                      <TableRow><TableCell colSpan={16} className="text-center py-8 text-muted-foreground">Sem Redes pra este recorte/filtros</TableCell></TableRow>
                    )}
                    {redesVisiveis.map((r, i) => {
                      const faltaCob = r.cobertura_valor - r.cobertura_objetivo
                      const faltaEan = r.sortimento_valor - r.sortimento_objetivo
                      return (
                        <TableRow
                          key={i}
                          className="cursor-pointer hover:bg-muted/50"
                          onClick={() => setItensAlvo({ codPrinc: r.cod_princ, titulo: r.fantasia || r.razao || r.cod_princ })}
                        >
                          <TableCell className="font-mono text-xs">{r.cod_princ}</TableCell>
                          <TableCell className="text-sm">{r.razao}</TableCell>
                          <TableCell className="text-sm font-medium">
                            <span className="inline-flex items-center gap-1">
                              <PackageSearch className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                              {r.fantasia}
                            </span>
                          </TableCell>
                          <TableCell className="text-right">{r.qt_lojas}</TableCell>
                          <TableCell className="text-xs text-muted-foreground">{r.uf || '—'}</TableCell>
                          <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{r.cod_ggv} — {r.nome_ggv}</TableCell>
                          <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{r.cod_crv} — {r.nome_crv}</TableCell>
                          <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{r.cod_rca} — {r.nome_rca}</TableCell>
                          <TableCell className="text-right">{fmtBRL(r.cobertura_objetivo)}</TableCell>
                          <TableCell className="text-right">{fmtBRL(r.cobertura_valor_total)}</TableCell>
                          <TableCell className="text-right">{fmtBRL(r.cobertura_valor)}</TableCell>
                          <TableCell className={`text-right ${faltaCob >= 0 ? 'text-emerald-600' : 'text-red-600'}`}>{fmtBRL(faltaCob)}</TableCell>
                          <TableCell className="text-center" onClick={e => e.stopPropagation()}><StatusBadge atingiu={r.cobertura_atingiu} /></TableCell>
                          <TableCell className="text-right">{fmt(r.sortimento_objetivo)}</TableCell>
                          <TableCell className="text-right">{fmt(r.sortimento_valor)}</TableCell>
                          <TableCell className={`text-right ${faltaEan >= 0 ? 'text-emerald-600' : 'text-red-600'}`}>{fmt(faltaEan)}</TableCell>
                        </TableRow>
                      )
                    })}
                    {redesVisiveis.length > 0 && (
                      <TableRow className="font-semibold bg-muted/50">
                        <TableCell>TOTAL</TableCell>
                        <TableCell /><TableCell />
                        <TableCell />
                        <TableCell />
                        <TableCell /><TableCell /><TableCell />
                        <TableCell className="text-right">{fmtBRL(totComb.objCob)}</TableCell>
                        <TableCell className="text-right">{fmtBRL(totComb.valorVenda)}</TableCell>
                        <TableCell />
                        <TableCell />
                        <TableCell />
                        <TableCell className="text-right">{fmt(totComb.objEan)}</TableCell>
                        <TableCell className="text-right">{fmt(totComb.qtMedEan)}</TableCell>
                        <TableCell className={`text-right ${totComb.faltaEan >= 0 ? 'text-emerald-600' : 'text-red-600'}`}>{fmt(totComb.faltaEan)}</TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            )}

            {abaCombinado === 'cliente' && (
              <div className="border rounded-lg overflow-x-auto [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                <p className="text-xs text-muted-foreground px-3 pt-2">Clique numa loja pra ver os itens que venderam e não venderam nela.</p>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Cód. Princ.</TableHead>
                      <TableHead>CNPJ</TableHead>
                      <TableHead>Razão</TableHead>
                      <TableHead>Fantasia</TableHead>
                      <TableHead>UF</TableHead>
                      <TableHead>GGV</TableHead>
                      <TableHead>CRV</TableHead>
                      <TableHead>RCA</TableHead>
                      <TableHead className="text-right">Obj. Cobertura</TableHead>
                      <TableHead className="text-right">Valor Venda</TableHead>
                      <TableHead className="text-right">Falta (R$)</TableHead>
                      <TableHead className="text-center">Status</TableHead>
                      <TableHead className="text-right">Obj. EANs</TableHead>
                      <TableHead className="text-right">Qt EANs</TableHead>
                      <TableHead className="text-right">Falta EANs</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {clientesVisiveis.length === 0 && (
                      <TableRow><TableCell colSpan={15} className="text-center py-8 text-muted-foreground">Sem Clientes pra este recorte/filtros</TableCell></TableRow>
                    )}
                    {clientesVisiveis.map((c, i) => {
                      const faltaCob = c.cobertura_valor - c.cobertura_objetivo
                      const faltaEan = c.sortimento_valor - c.sortimento_objetivo
                      return (
                        <TableRow
                          key={i}
                          className="cursor-pointer hover:bg-muted/50"
                          onClick={() => setItensAlvo({ cnpj: c.cnpj, titulo: `${c.fantasia || c.razao || c.cnpj} (${c.cnpj})` })}
                        >
                          <TableCell className="font-mono text-xs">{c.cod_princ}</TableCell>
                          <TableCell className="font-mono text-xs">{c.cnpj}</TableCell>
                          <TableCell className="text-sm">{c.razao}</TableCell>
                          <TableCell className="text-sm font-medium">
                            <span className="inline-flex items-center gap-1">
                              <PackageSearch className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                              {c.fantasia}
                            </span>
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground">{c.uf || '—'}</TableCell>
                          <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{c.cod_ggv} — {c.nome_ggv}</TableCell>
                          <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{c.cod_crv} — {c.nome_crv}</TableCell>
                          <TableCell className="text-xs text-muted-foreground whitespace-nowrap">{c.cod_rca} — {c.nome_rca}</TableCell>
                          <TableCell className="text-right">{fmtBRL(c.cobertura_objetivo)}</TableCell>
                          <TableCell className="text-right">{fmtBRL(c.cobertura_valor)}</TableCell>
                          <TableCell className={`text-right ${faltaCob >= 0 ? 'text-emerald-600' : 'text-red-600'}`}>{fmtBRL(faltaCob)}</TableCell>
                          <TableCell className="text-center" onClick={e => e.stopPropagation()}><StatusBadge atingiu={faltaCob >= 0} /></TableCell>
                          <TableCell className="text-right">{fmt(c.sortimento_objetivo)}</TableCell>
                          <TableCell className="text-right">{fmt(c.sortimento_valor)}</TableCell>
                          <TableCell className={`text-right ${faltaEan >= 0 ? 'text-emerald-600' : 'text-red-600'}`}>{fmt(faltaEan)}</TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>
            )}
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
                <div className="border rounded-lg overflow-hidden [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
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
              <div className="text-muted-foreground text-xs mb-1">Objetivo atual (Faixa {painel.proxima_faixa?.faixa ?? painel.faixa_atual?.faixa ?? '—'})</div>
              <div className="text-2xl font-semibold">
                {(painel.proxima_faixa ?? painel.faixa_atual)?.valor_meta !== undefined ? fmt((painel.proxima_faixa ?? painel.faixa_atual)!.valor_meta) : '—'}
              </div>
            </div>
            <div className={`border rounded-lg p-4 ${painel.delta > 0 ? 'bg-amber-50' : 'bg-emerald-50'}`}>
              <div className="flex items-center gap-2 text-xs mb-1">
                {painel.delta > 0 ? <TrendingDown className="w-4 h-4 text-amber-600" /> : <TrendingUp className="w-4 h-4 text-emerald-600" />}
                Falta pra bater o objetivo
              </div>
              <div className="text-2xl font-semibold">
                {painel.delta > 0 ? fmt(painel.delta) : 'Objetivo batido'}
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

          <div className="border rounded-lg overflow-hidden [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
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
                    <TableCell className="text-right">{(nivel === 'rede' || redeAberta) && ehCobertura ? fmtBRL(l.valor) : fmt(l.valor)}</TableCell>
                    {nivel === 'rede' && !redeAberta && (
                      <TableCell className="text-center">
                        <StatusBadge atingiu={!!l.marcador} />
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

      {/* Drill-down de itens (Sortimento): clicar numa Rede (aba "Resumo
          Redes") mostra os itens de TODAS as lojas dela; clicar numa loja
          (aba "Resumo Rede×Cliente") mostra só os itens daquele CNPJ.
          Vendeu/não vendeu, Qtd e Valor — pedido do Claudio 10/09/2026. */}
      <Dialog open={!!itensAlvo} onOpenChange={open => { if (!open) setItensAlvo(null) }}>
        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Itens — {itensAlvo?.titulo}</DialogTitle>
          </DialogHeader>
          {isLoadingItens ? (
            <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
          ) : (
            <div className="border rounded-lg overflow-x-auto [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>EAN</TableHead>
                    <TableHead>Produto</TableHead>
                    <TableHead className="text-right">Qtd</TableHead>
                    <TableHead className="text-right">Valor</TableHead>
                    <TableHead className="text-center">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {itensLista.length === 0 && (
                    <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Sem itens pra esta vigência</TableCell></TableRow>
                  )}
                  {itensLista.map((it, i) => (
                    <TableRow key={i}>
                      <TableCell className="font-mono text-xs">{it.ean}</TableCell>
                      <TableCell className="text-sm">{it.nome || '—'}</TableCell>
                      <TableCell className="text-right">{fmt(it.qtd)}</TableCell>
                      <TableCell className="text-right">{fmtBRL(it.valor)}</TableCell>
                      <TableCell className="text-center"><StatusBadge atingiu={it.vendeu} labelSim="Vendeu" labelNao="Não vendeu" /></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
