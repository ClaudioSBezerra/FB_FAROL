import AjudaJanelaApuracao, { rotuloJanelaBimestre, textoObjetivoClasses } from '@/components/farol/AjudaJanelaApuracao'
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
import { SearchableCombobox } from '@/components/ui/searchable-combobox'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { useAuth } from '@/contexts/AuthContext'
import { TrendingUp, TrendingDown, Target, AlertTriangle, PackageSearch, Info, Check, X as XIcon, Download } from 'lucide-react'
import { BotaoComoFunciona } from '@/components/ComoFuncionaIndicadores'
import { fmtBRL } from '@/lib/farolMoney'
import { Button } from '@/components/ui/button'
import { exportToExcel } from '@/lib/exportToExcel'
import { opcoesMetrica, resumoNumerica, resumirRollup, type NivelRollup } from '@/lib/metricasObjetivos'
import { toast } from 'sonner'

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
  cod_cl?: string
  objetivo?: number
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
  cod_cli?: string
  cnpj: string
  razao: string
  fantasia: string
  uf: string
  data_ultima_compra?: string
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

// PainelCombinadoNumericaCliente/Response — Combinado pra Numérica (Épico
// 7, pedido do Claudio 30/09/2026), grão único Cliente/CNPJ (sem Rede, ver
// farol_metas_painel_combinado_numerica.go).
interface PainelCombinadoNumericaCliente {
  cnpj: string
  cod_cl?: string
  classificacao_pdv: string
  razao: string
  fantasia: string
  cod_ggv: string
  nome_ggv: string
  cod_crv: string
  nome_crv: string
  cod_rca: string
  nome_rca: string
  cobertura_valor: number
  cobertura_objetivo: number
  cobertura_falta: number
  cobertura_atingiu: boolean
  sortimento_valor: number
  sortimento_objetivo: number
  sortimento_falta: number
  sortimento_atingiu: boolean
  sortimento_aplicavel: boolean
  teve_compra: boolean
}
// PainelPPALinha — 1 PPA do drill-down de um cliente da Numérica (Correção 2
// do Heverton 08/10/2026): comprou ou não comprou no período.
interface PainelPPALinha {
  ppa: string
  cod_prods?: string[]
  qtd: number
  vendeu: boolean
  abaixo_minimo?: boolean
}
interface PainelCombinadoNumerica {
  industria_nome: string
  vigencia: Vigencia
  cobertura: PainelMetricaResumo
  sortimento: PainelMetricaResumo
  clientes: PainelCombinadoNumericaCliente[]
  data_inicio_usada: string
  data_fim_usada: string
  apuracao_inicio?: string
  apuracao_fim?: string
  apuracao_tipo?: string
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
  // data_ultima_venda — pedido do Claudio 22/09/2026: fato do PRODUTO
  // (diferente de Dt.Ult.Cmp, fato do Cliente) — quando ESTE item foi
  // vendido pela última vez, mesmo que não tenha vendido NESTA vigência.
  cod_prods?: string[]
  data_ultima_venda?: string
}

// AbaCombinado — as 4 abas da visão Combinado, cada uma batizada e
// estruturada igual à aba real da planilha modelo da JC ("Unico
// Acompanhamento Ponderadas Unilever"): GGVxCRV e GGVxCRVxRCA são rollups
// (contagem de Redes atingindo/faltando cada métrica); Redes e Cliente são
// linha-a-linha com os valores. Mesmo espírito do painel geral (abas fixas
// no topo, como "Por FORN.GERAL"/"Por Gerência"/"Por Equipe"), mas os
// nomes vêm das abas da planilha, não da nomenclatura do painel geral.
type AbaCombinado = 'ggv' | 'ggv_crv' | 'ggv_crv_rca' | 'rede' | 'cliente'
const ABAS_COMBINADO: { value: AbaCombinado; label: string }[] = [
  { value: 'ggv', label: 'Resumo GGVs' },
  { value: 'ggv_crv', label: 'Resumo GGVs×CRVs' },
  { value: 'ggv_crv_rca', label: 'Resumo GGVs×CRVs×RCAs' },
  { value: 'rede', label: 'Resumo Redes' },
  { value: 'cliente', label: 'Resumo Rede×Cliente' },
]

interface GrupoCombinado {
  cod_ggv: string
  nome_ggv: string
  cod_crv?: string
  nome_crv?: string
  cod_rca?: string
  nome_rca?: string
  qtd_redes: number
  qtd_atingindo_cobertura: number
  qtd_falta_cobertura: number
  qtd_atingindo_sortimento: number
  qtd_falta_sortimento: number
  // Objetivo de Sortimento (qtd de EANs distintos) — mesmo valor pra toda
  // Rede da vigência (é uma faixa única cadastrada, não por Rede), então
  // pegar da 1ª Rede do grupo já representa o grupo inteiro. Usado só pro
  // drill-down de Itens (ver itensAlvo abaixo) — pedido do Claudio 18/09/2026.
  sortimento_objetivo: number
}

// agruparCombinado — rollup client-side (sem round-trip extra: as Redes já
// filtradas trazem tudo que este cálculo precisa) por par GGV+CRV ou trio
// GGV+CRV+RCA, contando quantas Redes atingem/faltam Cobertura e
// Sortimento — mesmo indicador das abas "Resumo GGvs Crvs"/"...Rcas" da
// planilha (QT REDES ATINGINDO/FALTA ATINGIR, pras duas métricas).
// nivel: 'ggv' agrupa só por GGV (pedido do Heverton 29/09/2026, nova aba
// "Resumo GGVs"); 'crv' e 'rca' são os rollups que já existiam.
function agruparCombinado(redes: PainelCombinadoRede[], nivel: 'ggv' | 'crv' | 'rca'): GrupoCombinado[] {
  const ordem: string[] = []
  const porChave = new Map<string, GrupoCombinado>()
  for (const r of redes) {
    const chave = nivel === 'ggv' ? r.cod_ggv : nivel === 'rca' ? `${r.cod_ggv}|${r.cod_crv}|${r.cod_rca}` : `${r.cod_ggv}|${r.cod_crv}`
    let g = porChave.get(chave)
    if (!g) {
      g = {
        cod_ggv: r.cod_ggv, nome_ggv: r.nome_ggv,
        ...(nivel !== 'ggv' ? { cod_crv: r.cod_crv, nome_crv: r.nome_crv } : {}),
        ...(nivel === 'rca' ? { cod_rca: r.cod_rca, nome_rca: r.nome_rca } : {}),
        qtd_redes: 0, qtd_atingindo_cobertura: 0, qtd_falta_cobertura: 0,
        qtd_atingindo_sortimento: 0, qtd_falta_sortimento: 0,
        sortimento_objetivo: r.sortimento_objetivo,
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
  // Numérica (Épico 7 addendum, 2026-09-29) — mesma Indústria, métrica
  // individual à parte (fora do modo "Combinado", que é o par Rede da
  // planilha "Resumo Redes").
  cobertura_numerica?: MetaVinculo
  sortimento_numerica?: MetaVinculo
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
// Numérica não tem Rede (Épico 7 addendum) — mesma lista, só troca o
// rótulo do último nível pra não chamar Cliente/CNPJ de "Rede" na tela.
const NIVEIS_NUMERICA = NIVEIS.map(n => n.value === 'rede' ? { ...n, label: 'GGV / CRV / RCA / Cliente' } : n)

// Faturado, Transmitido e Faturado + Transmitido, em Ponderada e Numérica
// (Heverton 09/10/2026, Correção 1 — reverte o corte de 04/09 na Ponderada).
const FLUXOS = [
  { value: 'faturado', label: 'Faturado' },
  { value: 'transmitido', label: 'Transmitido' },
  { value: 'soma', label: 'Faturado + Transmitido' },
]

const fmt = (n: number) => n.toLocaleString('pt-BR', { maximumFractionDigits: 2 })

// StatusBadge — ✓ verde (atingido) / ✗ vermelho (não atingido). Pedido do
// Heverton 25/09/2026 (mesmo padrão do painel mobile): símbolo no lugar do
// descritivo "Coberta"/"Não coberta". Os rótulos ficam como texto de
// acessibilidade/tooltip.
function StatusBadge({ atingiu, labelSim = 'Coberta', labelNao = 'Não coberta', size = 'w-5 h-5' }: {
  atingiu: boolean
  labelSim?: string
  labelNao?: string
  size?: string
}) {
  return atingiu
    ? <Check className={`${size} inline-block shrink-0 text-green-700`} strokeWidth={3} aria-label={labelSim}><title>{labelSim}</title></Check>
    : <XIcon className={`${size} inline-block shrink-0 text-red-700`} strokeWidth={3} aria-label={labelNao}><title>{labelNao}</title></XIcon>
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

// rotuloRede — "código — razão (fantasia)", mesmo padrão "código — nome" que
// os filtros de GGV/Supervisor/RCA já usavam — pedido do Claudio 14/09/2026:
// o filtro de Rede só mostrava a fantasia, sem código, e quem procura pelo
// código (comum pra RCA/GGV que decora número, não nome) não achava
// digitando. Código na frente também faz o type-ahead nativo do Select
// (Radix) funcionar batendo pelo número.
function rotuloRede(cod: string, razao: string, fantasia: string): string {
  const nome = fantasia && fantasia !== razao ? `${razao} (${fantasia})` : (razao || fantasia || cod)
  return `${cod} — ${nome}`
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
// FiltroSelect — trocado de <Select> nativo pra SearchableCombobox (pedido
// do Claudio 30/09/2026: "ao escrever nos filtros, trazer pela descrição
// ou pelo número, principalmente de clientes") — com a lista de Cliente da
// Numérica passando de 10.754 itens, um <select> sem busca era inviável de
// usar (só scroll). O rótulo de cada opção já vem "código — nome" (ver
// rotuloRede), e o CommandItem do cmdk filtra por substring no texto
// inteiro — então digitar o código OU um pedaço do nome já funciona, sem
// precisar de lógica de busca própria.
function FiltroSelect({ label, value, onChange, opts }: {
  label: string
  value: string
  onChange: (v: string) => void
  opts: { v: string; l: string }[]
}) {
  return (
    <div className="space-y-1">
      <label className="text-xs font-medium">{label}</label>
      <SearchableCombobox
        className="w-64 uppercase"
        placeholder="Todos"
        searchPlaceholder="Código ou nome..."
        emptyText="Nada encontrado."
        value={value || '__all__'}
        onChange={v => onChange(v === '__all__' ? '' : v)}
        options={[{ value: '__all__', label: 'Todos' }, ...opts.map(o => ({ value: o.v, label: o.l }))]}
      />
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
  const [metrica, setMetrica] = useState<'cobertura' | 'sortimento' | 'combinado' | 'cobertura_numerica' | 'sortimento_numerica' | 'combinado_numerica'>('combinado')
  const [vigenciaID, setVigenciaID] = useState('') // modo individual (Cobertura OU Sortimento)
  const [vigenciaCombinadaKey, setVigenciaCombinadaKey] = useState('') // modo combinado — chave "data_inicio|data_fim"
  const [vigenciaCombinadaNumericaKey, setVigenciaCombinadaNumericaKey] = useState('') // combinado_numerica
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
  // filtroClienteNumerica — achado real 2026-09-30 (Claudio: "ao filtrar
  // por Cliente não traz nada"): o filtro "Cliente" reaproveitava
  // selecionarRedeIndiv, que abre `redeAberta` (drill Rede→lojas). Pra
  // Numérica isso sempre vinha vazio (RealizadoRede.Clientes não é mais
  // populado pelo backend por performance, ver farol_metas_calculo_numerica.go)
  // — o Cliente selecionado JÁ É o nível final, não existe "loja dentro
  // dele" pra abrir. Em vez de drill, filtra a própria lista de nível
  // 'rede' pra só aquele CNPJ.
  const [filtroClienteNumerica, setFiltroClienteNumerica] = useState('')

  // Ao trocar de Indústria/Vigência/Fluxo, o drill-down perdido de propósito
  // (senão um filtro de GGV ficaria "grudado" ao trocar de programa).
  useEffect(() => {
    setFiltroGGV(null); setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica('')
    setNivel(landingNivelPorPersona(tipoPersona))
  }, [industriaID, tipoPersona])

  const abrirGrupo = (codigo: string, nome: string) => {
    if (nivel === 'ggv') { setFiltroGGV({ codigo, nome }); setNivel('crv') }
    else if (nivel === 'crv') { setFiltroCRV({ codigo, nome }); setNivel('rca') }
    else if (nivel === 'rca') { setFiltroRCA({ codigo, nome }); setNivel('rede') }
  }
  const voltarPara = (destino: 'ggv' | 'crv' | 'rca') => {
    if (destino === 'ggv') { setFiltroGGV(null); setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel('ggv') }
    else if (destino === 'crv') { setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel('crv') }
    else if (destino === 'rca') { setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel('rca') }
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
      else if (v.formula_codigo === 'cobertura_numerica') ind.cobertura_numerica = v
      else if (v.formula_codigo === 'sortimento_numerica_ppa') ind.sortimento_numerica = v
    }
    return Array.from(porID.values()).sort((a, b) => a.nome.localeCompare(b.nome))
  }, [vinculos])

  // Auto-seleciona a Indústria quando só existe UMA cadastrada (pedido do
  // Claudio 11/09/2026) — poupa o clique óbvio quando não há escolha real.
  // Com 2+ (ex: HC e FOOD), continua exigindo escolha manual — não dá pra
  // adivinhar qual o usuário quer ver primeiro.
  useEffect(() => {
    if (!industriaID && industrias.length === 1) setIndustriaID(String(industrias[0].id))
  }, [industrias, industriaID])

  const industriaSelecionada = industrias.find(i => String(i.id) === industriaID)
  const metricasDisponiveis = useMemo(() => {
    // Só a visão combinada de cada família (Heverton 08/10/2026) — ver
    // opcoesMetrica em lib/metricasObjetivos.ts.
    return opcoesMetrica({
      cobertura: !!industriaSelecionada?.cobertura,
      sortimento: !!industriaSelecionada?.sortimento,
      cobertura_numerica: !!industriaSelecionada?.cobertura_numerica,
      sortimento_numerica: !!industriaSelecionada?.sortimento_numerica,
    })
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
    : metrica === 'cobertura_numerica' ? industriaSelecionada?.cobertura_numerica
    : metrica === 'sortimento_numerica' ? industriaSelecionada?.sortimento_numerica
    : undefined

  // Numérica (Épico 7 addendum, 2026-09-29): cada "Rede" que o motor
  // devolve é na verdade 1 Cliente só (CodPrinc=CNPJ, QtLojas=1) — sem
  // Rede de verdade pra descer mais um nível. Terminologia e navegação
  // ajustadas pra não repetir "Rede"/"(1 loja)" nem abrir um drill falso
  // pro mesmo Cliente de novo (mesmo padrão pedido pro resto da tela:
  // check verde/X vermelho, nomenclatura consistente).
  const ehNumerica = vinculoAtivo?.formula_codigo === 'cobertura_numerica' || vinculoAtivo?.formula_codigo === 'sortimento_numerica_ppa'
  // ehContextoNumerica — ehNumerica só cobre o modo individual (depende de
  // vinculoAtivo, que fica undefined em 'combinado_numerica'). Usado nos
  // handlers de filtro (selecionarRedeIndiv) que também valem pro
  // Combinado Numérica. Declarado aqui (não perto de niveisAtuais/linhas
  // lá embaixo) porque o bloco de filtros por seleção (GGV/CRV/RCA/Cliente
  // — mais abaixo) já precisa disso antes.
  const ehContextoNumerica = ehNumerica || metrica === 'combinado_numerica'

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
  const vigenciaSelecionada = vigencias.find(v => String(v.id) === vigenciaID)

  // Auto-seleciona a vigência VIGENTE (aberta) assim que a lista chega —
  // é sempre isso que o usuário quer ver primeiro (pedido do Claudio
  // 11/09/2026: "já carregar a tabela vigente"). Cai pra fechada mais
  // recente só se não houver nenhuma aberta (ex: vínculo sem vigência do
  // mês corrente cadastrada ainda). Reavalia sempre que a lista muda
  // (troca de indústria/vínculo já zera vigenciaID no onValueChange, mas
  // fechada não), sem sobrescrever uma escolha manual ainda válida.
  useEffect(() => {
    if (vigencias.length === 0) return
    if (vigencias.some(v => String(v.id) === vigenciaID)) return
    const preferida = vigencias.find(v => v.status === 'aberta') ?? vigencias[0]
    setVigenciaID(String(preferida.id))
  }, [vigencias])

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

  // ─── Filtros por seleção (GGV/Supervisor/RCA/Rede) no modo individual ────────
  // Pedido do Claudio 14/09/2026: até aqui só dava pra "descer" no modo
  // individual clicando linha por linha (abrirGrupo) — sem select direto,
  // achar uma Rede específica (ex: PRATIKO) exigia saber de cor o
  // caminho GGV→CRV→RCA dela. Busca a mesma lista de Redes SEM filtro
  // nenhum (nivel=rede, sem cod_ggv/cod_crv/cod_rca) só pra montar as
  // opções dos selects — independente do nivel/filtro que a tabela
  // principal está mostrando agora. Mesmo endpoint que abrirGrupo/painel
  // já usa, então nenhuma rota nova no backend.
  // Combinado Numérica não tem vinculoAtivo/vigenciaID próprios (esses só
  // existem no modo individual) — busca a vigência aberta de Cobertura
  // Numérica só pra alimentar os selects de filtro. queryKey igual ao da
  // query "de verdade" mais abaixo (Modo combinado Numérica) — o React
  // Query dedupe por chave, então isso não dispara um fetch a mais.
  const { data: vigenciasCoberturaNumOpts = [] } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', industriaSelecionada?.cobertura_numerica?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${industriaSelecionada!.cobertura_numerica!.id}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: metrica === 'combinado_numerica' && !!industriaSelecionada?.cobertura_numerica,
  })
  const vigenciaCoberturaNumericaAtual = vigenciasCoberturaNumOpts.find(v => v.status === 'aberta') ?? vigenciasCoberturaNumOpts[0]
  const vinculoIdParaOpcoes = metrica === 'combinado_numerica' ? industriaSelecionada?.cobertura_numerica?.id : vinculoAtivo?.id
  const vigenciaIdParaOpcoes = metrica === 'combinado_numerica' ? (vigenciaCoberturaNumericaAtual ? String(vigenciaCoberturaNumericaAtual.id) : '') : vigenciaID
  const { data: todasRedesResp } = useQuery<Painel>({
    queryKey: ['farol-metas-painel-todas-redes', vinculoIdParaOpcoes, vigenciaIdParaOpcoes, fluxo],
    queryFn: async () => {
      const p = new URLSearchParams({ vinculo_id: String(vinculoIdParaOpcoes), vigencia_id: vigenciaIdParaOpcoes, fluxo, nivel: 'rede' })
      const r = await fetch(`/api/farol/metas-painel?${p}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: metrica !== 'combinado' && !!vinculoIdParaOpcoes && !!vigenciaIdParaOpcoes,
  })
  const todasRedesIndiv = todasRedesResp?.realizado.redes ?? []
  const optsGGVIndiv = useMemo(
    () => dedup(todasRedesIndiv.map(r => ({ v: r.cod_ggv, l: `${r.cod_ggv} — ${r.nome_ggv}` }))),
    [todasRedesIndiv],
  )
  const optsCRVIndiv = useMemo(
    () => dedup(todasRedesIndiv.filter(r => !filtroGGV || r.cod_ggv === filtroGGV.codigo)
      .map(r => ({ v: r.cod_crv, l: `${r.cod_crv} — ${r.nome_crv}` }))),
    [todasRedesIndiv, filtroGGV],
  )
  const optsRCAIndiv = useMemo(
    () => dedup(todasRedesIndiv.filter(r => (!filtroGGV || r.cod_ggv === filtroGGV.codigo) && (!filtroCRV || r.cod_crv === filtroCRV.codigo))
      .map(r => ({ v: r.cod_rca, l: `${r.cod_rca} — ${r.nome_rca}` }))),
    [todasRedesIndiv, filtroGGV, filtroCRV],
  )
  const optsRedeIndiv = useMemo(
    () => dedup(todasRedesIndiv
      .filter(r => (!filtroGGV || r.cod_ggv === filtroGGV.codigo) && (!filtroCRV || r.cod_crv === filtroCRV.codigo) && (!filtroRCA || r.cod_rca === filtroRCA.codigo))
      // Numérica: cod_princ É o CNPJ (truque de reuso) — mostrar o CNPJ no
      // filtro confundiria com "código do cliente"; usa cod_cl (CODCLI do
      // cadastro da JC) no rótulo, mantendo cod_princ como valor de
      // seleção (é o que casa com redeAberta.cod_princ em outro lugar).
      .map(r => ({
        v: r.cod_princ,
        l: ehContextoNumerica
          ? rotuloRede(r.cod_cl || r.cod_princ, r.razao, r.fantasia)
          : rotuloRede(r.cod_princ, r.razao, r.fantasia),
      }))),
    [todasRedesIndiv, filtroGGV, filtroCRV, filtroRCA, ehContextoNumerica],
  )
  // Selecionar direto pelo select pula pro próximo nível, igual abrirGrupo
  // já faz ao clicar numa linha — mantém nivel e filtro sempre "casados".
  const selecionarGGVIndiv = (v: string) => {
    if (!v) { setFiltroGGV(null); setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel('ggv'); return }
    const nome = todasRedesIndiv.find(r => r.cod_ggv === v)?.nome_ggv ?? v
    setFiltroGGV({ codigo: v, nome }); setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel('crv')
  }
  const selecionarCRVIndiv = (v: string) => {
    if (!v) { setFiltroCRV(null); setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel(filtroGGV ? 'crv' : 'ggv'); return }
    const nome = todasRedesIndiv.find(r => r.cod_crv === v)?.nome_crv ?? v
    setFiltroCRV({ codigo: v, nome }); setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel('rca')
  }
  const selecionarRCAIndiv = (v: string) => {
    if (!v) { setFiltroRCA(null); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel(filtroCRV ? 'rca' : filtroGGV ? 'crv' : 'ggv'); return }
    const nome = todasRedesIndiv.find(r => r.cod_rca === v)?.nome_rca ?? v
    setFiltroRCA({ codigo: v, nome }); setRedeAberta(null); setFiltroClienteNumerica(''); setNivel('rede')
  }
  const selecionarRedeIndiv = (v: string) => {
    if (ehContextoNumerica) {
      // Cliente já é o nível final na Numérica — filtra a lista em vez de
      // tentar abrir um drill que não existe (ver comentário de
      // filtroClienteNumerica acima). Vale pro modo individual E pro
      // Combinado Numérica (não tem nivel pra mexer nesse 2º caso).
      setFiltroClienteNumerica(v)
      if (v && metrica !== 'combinado_numerica') setNivel('rede')
      return
    }
    if (!v) { setRedeAberta(null); return }
    const rede = todasRedesIndiv.find(r => r.cod_princ === v)
    if (rede) { setNivel('rede'); setRedeAberta(rede) }
  }

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

  // Mesmo auto-select do modo individual (ver useEffect de vigenciaID
  // acima), aplicado ao Período do modo Combinado.
  useEffect(() => {
    if (periodosCombinados.length === 0) return
    if (periodosCombinados.some(p => p.chave === vigenciaCombinadaKey)) return
    const preferido = periodosCombinados.find(p => p.cobertura.status === 'aberta') ?? periodosCombinados[0]
    setVigenciaCombinadaKey(preferido.chave)
  }, [periodosCombinados])

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

  // ─── Modo combinado Numérica — Cobertura + Sortimento juntos, 1 linha por Cliente ───
  const { data: vigenciasCoberturaNum = [] } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', industriaSelecionada?.cobertura_numerica?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${industriaSelecionada!.cobertura_numerica!.id}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: metrica === 'combinado_numerica' && !!industriaSelecionada?.cobertura_numerica,
  })
  const { data: vigenciasSortimentoNum = [] } = useQuery<Vigencia[]>({
    queryKey: ['farol-metas-vigencias', industriaSelecionada?.sortimento_numerica?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${industriaSelecionada!.sortimento_numerica!.id}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: metrica === 'combinado_numerica' && !!industriaSelecionada?.sortimento_numerica,
  })
  const periodosCombinadosNum = useMemo(() => {
    const porPeriodo = new Map(vigenciasSortimentoNum.map(v => [`${v.data_inicio}|${v.data_fim}`, v]))
    return vigenciasCoberturaNum
      .filter(vc => porPeriodo.has(`${vc.data_inicio}|${vc.data_fim}`))
      .map(vc => ({ chave: `${vc.data_inicio}|${vc.data_fim}`, cobertura: vc, sortimento: porPeriodo.get(`${vc.data_inicio}|${vc.data_fim}`)! }))
  }, [vigenciasCoberturaNum, vigenciasSortimentoNum])
  const periodoSelecionadoNum = periodosCombinadosNum.find(p => p.chave === vigenciaCombinadaNumericaKey)
  useEffect(() => {
    if (periodosCombinadosNum.length === 0) return
    if (periodosCombinadosNum.some(p => p.chave === vigenciaCombinadaNumericaKey)) return
    const preferido = periodosCombinadosNum.find(p => p.cobertura.status === 'aberta') ?? periodosCombinadosNum[0]
    setVigenciaCombinadaNumericaKey(preferido.chave)
  }, [periodosCombinadosNum])

  // Reaproveita o MESMO filtro GGV/Supervisor/RCA do modo individual (ver
  // filtroGGV/filtroCRV/filtroRCA acima) em vez de duplicar um filtro novo
  // só pra este modo — o backend já aceita cod_ggv/cod_crv/cod_rca em
  // qualquer endpoint de painel (resolverFiltroDrillDown).
  const { data: painelCombinadoNum, isLoading: isLoadingCombinadoNum } = useQuery<PainelCombinadoNumerica>({
    queryKey: ['farol-metas-painel-combinado-numerica', industriaSelecionada?.cobertura_numerica?.id, industriaSelecionada?.sortimento_numerica?.id, periodoSelecionadoNum?.chave, fluxo, filtroGGV?.codigo, filtroCRV?.codigo, filtroRCA?.codigo],
    queryFn: async () => {
      const p = new URLSearchParams({
        vinculo_cobertura_id: String(industriaSelecionada!.cobertura_numerica!.id),
        vigencia_cobertura_id: String(periodoSelecionadoNum!.cobertura.id),
        vinculo_sortimento_id: String(industriaSelecionada!.sortimento_numerica!.id),
        vigencia_sortimento_id: String(periodoSelecionadoNum!.sortimento.id),
        fluxo,
      })
      if (filtroGGV) p.set('cod_ggv', filtroGGV.codigo)
      if (filtroCRV) p.set('cod_crv', filtroCRV.codigo)
      if (filtroRCA) p.set('cod_rca', filtroRCA.codigo)
      const r = await fetch(`/api/farol/metas-painel-combinado-numerica?${p}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: metrica === 'combinado_numerica' && !!periodoSelecionadoNum,
  })
  // Cliente é filtrado no navegador (o backend do Combinado Numérica não
  // recebe esse parâmetro, só cod_ggv/cod_crv/cod_rca) — mesmo padrão do
  // modo individual.
  const clientesCombinadoNum = (painelCombinadoNum?.clientes ?? [])
    .filter(c => !filtroClienteNumerica || c.cnpj === filtroClienteNumerica)

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
      .map(r => ({ v: r.cod_princ, l: rotuloRede(r.cod_princ, r.razao, r.fantasia) }))),
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
      (!fCliente || (r.clientes ?? []).some(c => c.cnpj === fCliente)))
      // Redes por maior venda realizada (Valor Venda) — pedido do Heverton 25/09/2026.
      .sort((a, b) => b.cobertura_valor_total - a.cobertura_valor_total || a.cod_princ.localeCompare(b.cod_princ)),
    [redesCombinado, fGGV, fCRV, fRCA, fRede, fUF, fCliente],
  )

  // TOTAL — soma só das colunas que a planilha "Resumo Redes" soma na última
  // linha (Obj. Cobertura, Valor Venda, Obj. EANs, Qt Méd. EANs, Falta EANs);
  // médias e Falta (R$) não são somadas (não faz sentido somar média).
  const totComb = useMemo(() => {
    const t = { objCob: 0, valorVenda: 0, somaCobertura: 0, objEan: 0, qtMedEan: 0, faltaEan: 0, cob: 0, sort: 0 }
    for (const r of redesVisiveis) {
      t.objCob += r.cobertura_objetivo
      t.valorVenda += r.cobertura_valor_total
      t.somaCobertura += r.cobertura_valor
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
  const [abaNumerica, setAbaNumerica] = useState<'cliente' | NivelRollup>('cliente')

  const clientesCombinado = painelCombinado?.clientes ?? []
  // Ordem por REDE (maior venda primeiro) e, dentro da Rede, por LOJA (maior
  // venda primeiro) — pedido do Heverton 25/09/2026, mesmo critério do mobile.
  const clientesVisiveis = useMemo(() => {
    const filtrados = clientesCombinado.filter(c =>
      (!fGGV || c.cod_ggv === fGGV) &&
      (!fCRV || c.cod_crv === fCRV) &&
      (!fRCA || c.cod_rca === fRCA) &&
      (!fRede || c.cod_princ === fRede) &&
      (!fUF || c.uf === fUF) &&
      (!fCliente || c.cnpj === fCliente))
    const totalPorRede = new Map<string, number>()
    for (const c of filtrados) totalPorRede.set(c.cod_princ, (totalPorRede.get(c.cod_princ) ?? 0) + c.cobertura_valor)
    return filtrados.sort((a, b) =>
      (totalPorRede.get(b.cod_princ)! - totalPorRede.get(a.cod_princ)!) ||
      a.cod_princ.localeCompare(b.cod_princ) ||
      b.cobertura_valor - a.cobertura_valor)
  }, [clientesCombinado, fGGV, fCRV, fRCA, fRede, fUF, fCliente],
  )

  const gruposGGV = useMemo(() => agruparCombinado(redesVisiveis, 'ggv'), [redesVisiveis])
  const gruposGGVCRV = useMemo(() => agruparCombinado(redesVisiveis, 'crv'), [redesVisiveis])
  const gruposGGVCRVRCA = useMemo(() => agruparCombinado(redesVisiveis, 'rca'), [redesVisiveis])
  const gruposAtivos = abaCombinado === 'ggv' ? gruposGGV : abaCombinado === 'ggv_crv' ? gruposGGVCRV : gruposGGVCRVRCA
  const colSpanGrupos = abaCombinado === 'ggv' ? 6 : abaCombinado === 'ggv_crv' ? 7 : 8

  // ─── Drill-down "Itens" (Sortimento): vendeu/não vendeu, Qtd e Valor —
  // clicar numa Rede (aba "Resumo Redes") mostra os itens de TODAS as
  // lojas dela; clicar numa loja (aba "Resumo Rede×Cliente") mostra só os
  // itens daquele CNPJ. Pedido do Claudio em 10/09/2026. Estendido em
  // 18/09/2026 pra também abrir clicando numa linha das abas "Resumo
  // GGVs×CRVs"/"...×RCAs" — aí o escopo é codGGV+codCRV(+codRCA), agregando
  // TODAS as Redes daquele grupo (backend: cnpjsDoEscopoNaVigencia já
  // aceitava isso, só faltava o clique no frontend).
  // qtdRedes/qtdAtingindo só vêm preenchidos quando o dialog abre de uma
  // linha de rollup (GGVxCRV/GGVxCRVxRCA) — usados só pra deixar claro que
  // a lista de itens abaixo é a UNIÃO das lojas do grupo, não o que decide
  // atingiu/não atingiu (isso é por Rede, média por loja — ver PRD:
  // "Sortimento = média de EANs distintos entre as lojas da Rede"). Achado
  // 18/09/2026: sem essa nota, "22 vendidos ≥ Objetivo 19" ao lado de
  // "0 de 9 redes atingindo" parecia contradição — não é, são duas contas.
  const [itensAlvo, setItensAlvo] = useState<{ codPrinc?: string; cnpj?: string; codGGV?: string; codCRV?: string; codRCA?: string; titulo: string; objetivo: number; qtdRedes?: number; qtdAtingindo?: number; dataUltimaCompra?: string } | null>(null)
  const { data: itensResp, isLoading: isLoadingItens } = useQuery<{ itens: PainelItemLinha[] }>({
    queryKey: ['farol-metas-painel-itens', industriaSelecionada?.sortimento?.id, periodoSelecionado?.sortimento.id, fluxo, itensAlvo?.codPrinc, itensAlvo?.cnpj, itensAlvo?.codGGV, itensAlvo?.codCRV, itensAlvo?.codRCA],
    queryFn: async () => {
      const p = new URLSearchParams({
        vinculo_sortimento_id: String(industriaSelecionada!.sortimento!.id),
        vigencia_sortimento_id: String(periodoSelecionado!.sortimento.id),
        fluxo,
      })
      if (itensAlvo!.cnpj) p.set('cnpj', itensAlvo!.cnpj)
      else if (itensAlvo!.codPrinc) p.set('cod_princ', itensAlvo!.codPrinc)
      else {
        p.set('cod_ggv', itensAlvo!.codGGV!)
        p.set('cod_crv', itensAlvo!.codCRV!)
        if (itensAlvo!.codRCA) p.set('cod_rca', itensAlvo!.codRCA)
      }
      const r = await fetch(`/api/farol/metas-painel-itens?${p}`, { headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: !!itensAlvo && !!industriaSelecionada?.sortimento && !!periodoSelecionado,
  })
  const itensLista = itensResp?.itens ?? []

  // Drill-down de PPAs de UM cliente da Numérica (clicar na linha do
  // Combinado Numérica). Cliente Num. C não tem Sortimento (FR26) — o
  // diálogo só avisa, sem consultar.
  const [ppasAlvo, setPpasAlvo] = useState<{ cnpj: string; titulo: string; aplicavel: boolean; objetivo: number; classe: string } | null>(null)
  const { data: ppasResp, isLoading: isLoadingPpas, error: ppasErro } = useQuery<{ ppas: PainelPPALinha[] }>({
    queryKey: ['farol-metas-painel-ppas', periodoSelecionadoNum?.sortimento.id, fluxo, ppasAlvo?.cnpj],
    queryFn: async () => {
      const p = new URLSearchParams({
        vigencia_sortimento_id: String(periodoSelecionadoNum!.sortimento.id),
        fluxo,
        cnpj: ppasAlvo!.cnpj,
      })
      const r = await fetch(`/api/farol/metas-painel-ppas?${p}`, { headers })
      if (!r.ok) throw new Error((await r.json().catch(() => null))?.error || 'Falha ao carregar os PPAs')
      return r.json()
    },
    enabled: !!ppasAlvo && ppasAlvo.aplicavel && !!periodoSelecionadoNum,
  })
  const ppasLista = ppasResp?.ppas ?? []

  // Nível 5 (CNPJ) é um drill-down de UMA Rede escolhida, não um valor de
  // `nivel` selecionável — por isso fica fora do enum NIVEIS/fetch e só lê
  // .clientes que já veio junto no Realizado da Rede.
  // Cobertura mede R$ por loja/Rede; Sortimento mede qtd de EANs — só a
  // primeira usa formatação monetária linha a linha (pedido do Claudio em
  // 10/09/2026, "colocar o R$ ao lado do Valor").
  const ehCobertura = vinculoAtivo?.formula_codigo === 'cobertura_rede' || vinculoAtivo?.formula_codigo === 'cobertura_numerica'
  const niveisAtuais = ehNumerica ? NIVEIS_NUMERICA : NIVEIS
  // Numérica (inclusive a visão combinada, a única que sobrou) oferece a 3ª
  // visão Faturado + Emitido que o programa exige.
  const fluxosAtuais = FLUXOS
  const linhas = redeAberta
    ? [...(redeAberta.clientes ?? [])].sort((a, b) => b.valor - a.valor).map(c => ({
        // Rede/qt_lojas não se aplica no nível 5 (CNPJ é uma loja só) —
        // aqui "nome" já é a loja, sem contagem de lojas ao lado.
        nome: c.fantasia || c.razao || c.cnpj, sub: c.cnpj, valor: c.valor, marcador: undefined as boolean | undefined, drill: undefined as (() => void) | undefined,
      }))
    : nivel === 'rede'
    ? [...(painel?.realizado.redes ?? [])]
        .filter(r => !ehNumerica || !filtroClienteNumerica || r.cod_princ === filtroClienteNumerica)
        .sort((x, y) => ((y as { valor_total?: number }).valor_total ?? y.valor) - ((x as { valor_total?: number }).valor_total ?? x.valor)).map(r => ({
        // Qt de lojas AO LADO do nome da Rede (pedido do Claudio em
        // 10/09/2026) — RCA fica isolado no "sub", sem misturar os dois.
        // Numérica não mostra "(N loja)" (sempre 1) nem abre drill (o
        // próprio Cliente já é o nível final).
        nome: ehNumerica
          ? `${r.cod_cl ? `${r.cod_cl} — ` : ''}${r.fantasia || r.razao || r.cod_princ}`
          : `${r.fantasia || r.razao || r.cod_princ} (${r.qt_lojas} loja${r.qt_lojas === 1 ? '' : 's'})`,
        sub: r.cod_rca ? `${r.cod_rca} — ${r.nome_rca}` : r.nome_rca,
        valor: r.valor, marcador: r.atingiu as boolean | undefined, drill: ehNumerica ? undefined : () => setRedeAberta(r),
      }))
    : (painel?.realizado.grupos ?? []).map(g => ({
        nome: g.codigo && g.nome ? `${g.codigo} — ${g.nome}` : (g.nome || g.codigo),
        sub: `${g.qtd_atingindo}/${g.qtd_redes} ${ehNumerica ? 'clientes' : 'redes'} atingindo`,
        valor: g.qtd_atingindo,
        // marcador no nível de grupo (pedido do Claudio 30/09/2026): check
        // só quando TODO mundo do grupo bateu (mesmo critério binário do
        // nível Rede/Cliente — sem meio-termo), X caso contrário.
        marcador: g.qtd_redes > 0 ? g.qtd_falta_atingir === 0 : undefined,
        drill: () => abrirGrupo(g.codigo, g.nome || g.codigo),
      }))

  const podeAbrirLinha = !redeAberta && nivel !== 'rede'
  // Coluna de RCA some quando a lista já está escopada a um único RCA
  // (filtroRCA) — toda linha mostraria o mesmo nome, redundante com o
  // breadcrumb. Não afeta o nível CNPJ (redeAberta), que mostra
  // "Documento" por loja, não RCA.
  const mostrarColunaRCA = redeAberta ? true : !(nivel === 'rede' && filtroRCA)
  const nivelLabelAtual = redeAberta ? 'Rede/CNPJ' : niveisAtuais.find(n => n.value === nivel)?.label ?? nivel

  // exportarPainel — pedido do Heverton 30/09/2026: a versão WEB não tinha
  // nenhum jeito de baixar os dados em Excel (os comparativos já tinham,
  // o painel principal não). Exporta sempre no grão mais fino disponível
  // (Rede/Cliente — nunca o rollup de GGV/CRV/RCA que a tela pode estar
  // mostrando no momento), pra servir de base pra quem quiser pivotar/
  // cruzar os números fora do Farol — mesmo padrão dos Comparativos
  // (FarolComparativoFechamento(Numerica).tsx), reaproveitando o mesmo
  // exportToExcel.
  function exportarPainel() {
    const industriaNome = industriaSelecionada?.nome ?? ''
    if (metrica === 'combinado') {
      if (clientesCombinado.length === 0) { toast.error('Nada pra exportar ainda'); return }
      exportToExcel(
        clientesCombinado.map(c => ({
          'Cód. Cliente': c.cod_cli || '', CNPJ: c.cnpj, Razão: c.razao, Fantasia: c.fantasia, UF: c.uf,
          GGV: `${c.cod_ggv} — ${c.nome_ggv}`, CRV: `${c.cod_crv} — ${c.nome_crv}`, RCA: `${c.cod_rca} — ${c.nome_rca}`,
          'Cobertura (R$)': c.cobertura_valor, 'Objetivo Cobertura': c.cobertura_objetivo,
          'Sortimento (EANs)': c.sortimento_valor, 'Objetivo Sortimento': c.sortimento_objetivo,
          'Dt. Últ. Compra': c.data_ultima_compra || '',
        })),
        `Combinado_${industriaNome}_${vigenciaCombinadaKey.replace('|', '_a_')}`, 'Combinado',
      )
      return
    }
    if (metrica === 'combinado_numerica') {
      if (clientesCombinadoNum.length === 0) { toast.error('Nada pra exportar ainda'); return }
      exportToExcel(
        clientesCombinadoNum.map(c => ({
          'Cód. Cliente': c.cod_cl || '', CNPJ: c.cnpj, Classificação: c.classificacao_pdv, Razão: c.razao, Fantasia: c.fantasia,
          GGV: `${c.cod_ggv} — ${c.nome_ggv}`, CRV: `${c.cod_crv} — ${c.nome_crv}`, RCA: `${c.cod_rca} — ${c.nome_rca}`,
          'Cobertura (R$)': c.cobertura_valor, 'Objetivo Cobertura': c.cobertura_objetivo, 'Cobertura Atingiu': c.cobertura_atingiu ? 'Sim' : 'Não',
          'Sortimento (PPAs)': c.sortimento_aplicavel ? c.sortimento_valor : '', 'Objetivo Sortimento': c.sortimento_aplicavel ? c.sortimento_objetivo : '',
          'Sortimento Aplicável': c.sortimento_aplicavel ? 'Sim' : 'Não',
        })),
        `Combinado_Numerica_${industriaNome}_${vigenciaCombinadaNumericaKey.replace('|', '_a_')}`, 'Combinado Numerica',
      )
      return
    }
    const redes = painel?.realizado.redes ?? []
    if (redes.length === 0) { toast.error('Nada pra exportar ainda'); return }
    exportToExcel(
      redes.map(r => ({
        [ehNumerica ? 'Cód. Cliente' : 'Cód. Princ.']: ehNumerica ? (r.cod_cl || r.cod_princ) : r.cod_princ,
        ...(ehNumerica ? { CNPJ: r.cod_princ } : {}),
        Razão: r.razao, Fantasia: r.fantasia,
        ...(ehNumerica ? {} : { 'Qt Lojas': r.qt_lojas }),
        GGV: `${r.cod_ggv} — ${r.nome_ggv}`, CRV: `${r.cod_crv} — ${r.nome_crv}`, RCA: `${r.cod_rca} — ${r.nome_rca}`,
        [ehCobertura ? 'Valor (R$)' : 'Valor']: r.valor,
        Objetivo: r.objetivo ?? '',
        Status: r.atingiu ? 'Atingiu' : 'Não atingiu',
      })),
      `${vinculoAtivo?.tipo_metrica_nome || metrica}_${industriaNome}_${vigenciaID}`, 'Detalhe',
    )
  }

  return (
    <div className="p-6 space-y-4 uppercase text-sm [&_*]:uppercase">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Painel de Objetivos por Indústria</h1>
          <p className="text-sm text-muted-foreground">Objetivo × Realizado por Tipo de Métrica, navegável pela hierarquia GGV → CRV → RCA → Rede.</p>
        </div>
        {/* Racional do cálculo (Cobertura/Sortimento) — pedido do Claudio e
            Heverton 14/09/2026: precisa estar visível na tela, não só em
            conversa. Só aparece com Indústria selecionada (precisa saber
            qual vínculo consultar pro limiar/faixas reais). */}
        {industriaSelecionada && (
          <div className="pt-1">
            <BotaoComoFunciona
              industriaNome={industriaSelecionada.nome}
              vinculoCoberturaId={industriaSelecionada.cobertura?.id}
              vinculoSortimentoId={industriaSelecionada.sortimento?.id}
              limiarCobertura={industriaSelecionada.cobertura?.parametros_valores?.limiar_valor_medio as number | undefined}
            />
          </div>
        )}
      </div>

      <div className="flex flex-wrap gap-3 items-end border rounded-lg p-4">
        <div className="space-y-1">
          <label className="text-xs font-medium">Indústria</label>
          <Select value={industriaID} onValueChange={v => { setIndustriaID(v); setVigenciaID(''); setVigenciaCombinadaKey('') }}>
            <SelectTrigger className="w-56 uppercase"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent className="[&_*]:uppercase">
              {industrias.map(i => (
                <SelectItem key={i.id} value={String(i.id)}>{i.nome}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {industriaSelecionada && (
          <div className="space-y-1">
            <label className="text-xs font-medium">Visão</label>
            <Select value={metrica} onValueChange={v => {
              setMetrica(v as typeof metrica); setVigenciaID(''); setVigenciaCombinadaKey('')
              // Combinado Numérica reaproveita filtroGGV/CRV/RCA (mesmo
              // state do modo individual) mas não tem os controles na
              // tela pra trocar — zera ao entrar/sair pra não herdar um
              // filtro escolhido antes sem querer.
              setFiltroGGV(null); setFiltroCRV(null); setFiltroRCA(null); setFiltroClienteNumerica('')
            }}>
              <SelectTrigger className="w-64 uppercase"><SelectValue /></SelectTrigger>
              <SelectContent className="[&_*]:uppercase">
                {metricasDisponiveis.map(m => <SelectItem key={m.value} value={m.value}>{m.label}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        )}
        {metrica === 'combinado' ? (
          <div className="space-y-1">
            <label className="text-xs font-medium">Período</label>
            <Select value={vigenciaCombinadaKey} onValueChange={setVigenciaCombinadaKey}>
              <SelectTrigger className="w-56 uppercase"><SelectValue placeholder="Selecione" /></SelectTrigger>
              <SelectContent className="[&_*]:uppercase">
                {periodosCombinados.map(p => (
                  <SelectItem key={p.chave} value={p.chave}>
                    {p.cobertura.data_inicio} – {p.cobertura.data_fim} {p.cobertura.status === 'fechada' ? '(fechada)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : metrica === 'combinado_numerica' ? (
          <div className="space-y-1">
            <label className="text-xs font-medium flex items-center gap-2">
              Período
              {periodoSelecionadoNum && <AjudaJanelaApuracao vigInicio={periodoSelecionadoNum.cobertura.data_inicio} vigFim={periodoSelecionadoNum.cobertura.data_fim} />}
            </label>
            <Select value={vigenciaCombinadaNumericaKey} onValueChange={setVigenciaCombinadaNumericaKey}>
              <SelectTrigger className="w-56 uppercase"><SelectValue placeholder="Selecione" /></SelectTrigger>
              <SelectContent className="[&_*]:uppercase">
                {periodosCombinadosNum.map(p => (
                  <SelectItem key={p.chave} value={p.chave}>
                    {rotuloJanelaBimestre(p.cobertura.data_inicio, p.cobertura.data_fim)} {p.cobertura.status === 'fechada' ? '(fechada)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : (
          <div className="space-y-1">
            <label className="text-xs font-medium flex items-center gap-2">
              Vigência
              {ehNumerica && vigenciaSelecionada && <AjudaJanelaApuracao vigInicio={vigenciaSelecionada.data_inicio} vigFim={vigenciaSelecionada.data_fim} />}
            </label>
            <Select value={vigenciaID} onValueChange={setVigenciaID}>
              <SelectTrigger className="w-56 uppercase"><SelectValue placeholder="Selecione" /></SelectTrigger>
              <SelectContent className="[&_*]:uppercase">
                {vigencias.map(v => (
                  <SelectItem key={v.id} value={String(v.id)}>
                    {v.data_inicio} – {v.data_fim} {v.status === 'fechada' ? '(fechada)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        {metrica !== 'combinado' && metrica !== 'combinado_numerica' && (
          <div className="space-y-1">
            <label className="text-xs font-medium">Nível</label>
            <Select value={nivel} onValueChange={v => { voltarPara('ggv'); setNivel(v) }}>
              <SelectTrigger className="w-56 uppercase"><SelectValue /></SelectTrigger>
              <SelectContent className="[&_*]:uppercase">
                {niveisAtuais.map(n => <SelectItem key={n.value} value={n.value}>{n.label}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="space-y-1">
          <label className="text-xs font-medium">Fluxo</label>
          <Select value={fluxo} onValueChange={setFluxo}>
            <SelectTrigger className="w-48 uppercase"><SelectValue /></SelectTrigger>
            <SelectContent className="[&_*]:uppercase">
              {fluxosAtuais.map(f => <SelectItem key={f.value} value={f.value}>{f.label}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        {industriaID && (
          <Button variant="outline" size="sm" className="gap-2" onClick={exportarPainel}>
            <Download className="w-4 h-4" />
            Exportar Excel
          </Button>
        )}
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

            <TooltipProvider delayDuration={150}>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div className="border rounded-lg p-4">
                  <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                    <Target className="w-4 h-4" /> Cobertura — redes cobertas
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Info className="w-3.5 h-3.5 cursor-default" />
                      </TooltipTrigger>
                      <TooltipContent className="text-xs max-w-xs">
                        <p>Conta Redes cujo <strong>valor médio de compra entre as lojas dela</strong> ultrapassa o limiar (faixa) cadastrado — é uma média por loja, não a soma/faturamento total da Rede.</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="text-2xl font-semibold">
                    {totComb.cob} <span className="text-sm text-muted-foreground">/ {redesVisiveis.length} redes</span>{' '}
                    {redesVisiveis.length > 0 && <StatusBadge size="w-7 h-7" atingiu={totComb.cob >= redesVisiveis.length} />}
                  </div>
                  {redesVisiveis.length > 0 && (
                    <div className="text-xs text-muted-foreground mt-0.5">
                      Média: {fmtBRL(totComb.somaCobertura / redesVisiveis.length)} / {fmtBRL(redesVisiveis[0].cobertura_objetivo)}
                    </div>
                  )}
                </div>
                <div className="border rounded-lg p-4">
                  <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                    <Target className="w-4 h-4" /> Sortimento — redes no objetivo de EANs
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Info className="w-3.5 h-3.5 cursor-default" />
                      </TooltipTrigger>
                      <TooltipContent className="text-xs max-w-xs">
                        <p>Conta Redes cuja <strong>média de EANs distintos vendidos entre as lojas dela</strong> atinge o objetivo da vigência (mesmo objetivo pra toda Rede, grande ou pequena). Não é a soma/união de tudo que a Rede vendeu — uma Rede pode vender vários EANs diferentes espalhados pelas lojas e ainda assim não bater a média por loja.</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <div className="text-2xl font-semibold">
                    {totComb.sort} <span className="text-sm text-muted-foreground">/ {redesVisiveis.length}</span>{' '}
                    {redesVisiveis.length > 0 && <StatusBadge size="w-7 h-7" atingiu={totComb.sort >= redesVisiveis.length} />}
                  </div>
                  {redesVisiveis.length > 0 && (
                    <div className="text-xs text-muted-foreground mt-0.5">
                      {/* Texto explícito — pedido do Heverton 29/09/2026:
                          "1 de 86 redes bateram os 19 EANs" deixa claro que
                          é contagem (régua dura), diferente da média
                          (puxada pra baixo pelas Redes fracas/zeradas). */}
                      Redes bateram os {fmt(redesVisiveis[0].sortimento_objetivo)} EANs · Média: {fmt(totComb.qtMedEan / redesVisiveis.length)}
                    </div>
                  )}
                </div>
              </div>
            </TooltipProvider>

            {(abaCombinado === 'ggv' || abaCombinado === 'ggv_crv' || abaCombinado === 'ggv_crv_rca') && (
              <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                {/* Clicar num GGV, GGV×CRV ou GGV×CRV×RCA vai abrindo o
                    nível de baixo (igual à visão mobile: toca pra descer na
                    hierarquia) — pedido do Heverton 29/09/2026: Rede,
                    Clientes e só depois Produtos (clicar no Cliente, já em
                    Resumo Rede×Cliente, abre os itens). */}
                <p className="text-xs text-muted-foreground px-3 pt-2">
                  {abaCombinado === 'ggv' ? 'Clique num GGV pra ver as CRVs dele.' : 'Clique num grupo pra ver as Redes dele.'}
                </p>
                <Table>
                  <TableHeader className="sticky top-0 z-10 bg-white">
                    <TableRow>
                      <TableHead>GGV</TableHead>
                      {abaCombinado !== 'ggv' && <TableHead>CRV</TableHead>}
                      {abaCombinado === 'ggv_crv_rca' && <TableHead>RCA</TableHead>}
                      <TableHead className="text-right">Qt Redes</TableHead>
                      <TableHead className="text-right">Redes atingindo Cobertura</TableHead>
                      <TableHead className="text-right">Redes abaixo da Cobertura</TableHead>
                      <TableHead className="text-right">Redes atingindo Sortimento</TableHead>
                      <TableHead className="text-right">Redes abaixo do Sortimento</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {gruposAtivos.length === 0 && (
                      <TableRow><TableCell colSpan={colSpanGrupos} className="text-center py-8 text-muted-foreground">Sem dados pra este recorte/filtros</TableCell></TableRow>
                    )}
                    {gruposAtivos.map((g, i) => (
                      <TableRow
                        key={i}
                        className="cursor-pointer hover:bg-muted/50"
                        onClick={() => {
                          if (abaCombinado === 'ggv') {
                            setFGGV(g.cod_ggv); setFCRV(''); setFRCA(''); setFRede(''); setFCliente('')
                            setAbaCombinado('ggv_crv')
                            return
                          }
                          // GGV×CRV e GGV×CRV×RCA também abrem em Resumo Redes
                          // (pedido do Heverton 29/09/2026: "Rede, Clientes e
                          // depois Produtos" — GGV×CRV×RCA deixou de pular
                          // direto pros itens agregados).
                          setFGGV(g.cod_ggv); setFCRV(g.cod_crv ?? ''); setFRCA(g.cod_rca ?? ''); setFRede(''); setFCliente('')
                          setAbaCombinado('rede')
                        }}
                      >
                        <TableCell className="text-sm whitespace-nowrap">{g.cod_ggv} — {g.nome_ggv}</TableCell>
                        {abaCombinado !== 'ggv' && <TableCell className="text-sm whitespace-nowrap">{g.cod_crv} — {g.nome_crv}</TableCell>}
                        {abaCombinado === 'ggv_crv_rca' && <TableCell className="text-sm whitespace-nowrap">{g.cod_rca} — {g.nome_rca}</TableCell>}
                        <TableCell className="text-right">{g.qtd_redes}</TableCell>
                        <TableCell className="text-right text-green-700">{g.qtd_atingindo_cobertura}</TableCell>
                        <TableCell className="text-right text-red-700">{g.qtd_falta_cobertura}</TableCell>
                        <TableCell className="text-right text-green-700">{g.qtd_atingindo_sortimento}</TableCell>
                        <TableCell className="text-right text-red-700">{g.qtd_falta_sortimento}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}

            {abaCombinado === 'rede' && (
              <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                <p className="text-xs text-muted-foreground px-3 pt-2">Clique numa Rede pra ver os Clientes dela.</p>
                <Table>
                  <TableHeader className="sticky top-0 z-10 bg-white">
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
                          // Clicar na Rede mostra os Clientes dela primeiro (aba
                          // Rede×Cliente), não os itens direto — pedido do
                          // Heverton 29/09/2026, mesmo espírito do drill-down
                          // GGV→GGV×CRV: só chega em Produtos depois de passar
                          // por Cliente (clicar no Cliente lá já abre os itens).
                          onClick={() => { setFRede(r.cod_princ); setFCliente(''); setAbaCombinado('cliente') }}
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
                          <TableCell className={`text-right ${faltaCob >= 0 ? 'text-green-700' : 'text-red-700'}`}>{fmtBRL(faltaCob)}</TableCell>
                          <TableCell className="text-center" onClick={e => e.stopPropagation()}><StatusBadge atingiu={r.cobertura_atingiu} /></TableCell>
                          <TableCell className="text-right">{fmt(r.sortimento_objetivo)}</TableCell>
                          <TableCell className="text-right">{fmt(r.sortimento_valor)}</TableCell>
                          <TableCell className={`text-right whitespace-nowrap ${faltaEan >= 0 ? 'text-green-700' : 'text-red-700'}`}>{fmt(faltaEan)} <StatusBadge size="w-4 h-4" atingiu={faltaEan >= 0} labelSim="No objetivo" labelNao="Abaixo do objetivo" /></TableCell>
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
                        <TableCell className={`text-right ${totComb.faltaEan >= 0 ? 'text-green-700' : 'text-red-700'}`}>{fmt(totComb.faltaEan)}</TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </div>
            )}

            {abaCombinado === 'cliente' && (
              <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                <p className="text-xs text-muted-foreground px-3 pt-2">Clique numa loja pra ver os itens que venderam e não venderam nela.</p>
                <Table>
                  <TableHeader className="sticky top-0 z-10 bg-white">
                    <TableRow>
                      <TableHead>Cód. Princ.</TableHead>
                      <TableHead>Cód. Cliente</TableHead>
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
                      <TableRow><TableCell colSpan={16} className="text-center py-8 text-muted-foreground">Sem Clientes pra este recorte/filtros</TableCell></TableRow>
                    )}
                    {clientesVisiveis.map((c, i) => {
                      const faltaCob = c.cobertura_valor - c.cobertura_objetivo
                      const faltaEan = c.sortimento_valor - c.sortimento_objetivo
                      return (
                        <TableRow
                          key={i}
                          className="cursor-pointer hover:bg-muted/50"
                          onClick={() => setItensAlvo({ cnpj: c.cnpj, titulo: `${c.cod_cli || c.cnpj} - ${c.fantasia || c.razao || c.cnpj}`, objetivo: c.sortimento_objetivo, dataUltimaCompra: c.data_ultima_compra })}
                        >
                          <TableCell className="font-mono text-xs">{c.cod_princ}</TableCell>
                          <TableCell className="font-mono text-xs font-semibold">{c.cod_cli || '—'}</TableCell>
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
                          <TableCell className={`text-right ${faltaCob >= 0 ? 'text-green-700' : 'text-red-700'}`}>{fmtBRL(faltaCob)}</TableCell>
                          <TableCell className="text-center" onClick={e => e.stopPropagation()}><StatusBadge atingiu={faltaCob >= 0} /></TableCell>
                          <TableCell className="text-right">{fmt(c.sortimento_objetivo)}</TableCell>
                          <TableCell className="text-right">{fmt(c.sortimento_valor)}</TableCell>
                          <TableCell className={`text-right whitespace-nowrap ${faltaEan >= 0 ? 'text-green-700' : 'text-red-700'}`}>{fmt(faltaEan)} <StatusBadge size="w-4 h-4" atingiu={faltaEan >= 0} labelSim="No objetivo" labelNao="Abaixo do objetivo" /></TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </div>
            )}
          </>
        ) : null
      ) : metrica === 'combinado_numerica' ? (
        !periodoSelecionadoNum ? (
          <p className="text-sm text-muted-foreground py-8 text-center">
            {periodosCombinadosNum.length === 0
              ? 'Nenhum período com Cobertura Numérica e Sortimento Numérica cadastrados pro mesmo intervalo de datas ainda.'
              : 'Selecione a Vigência pra ver o painel combinado.'}
          </p>
        ) : isLoadingCombinadoNum ? (
          <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
        ) : painelCombinadoNum ? (
          <>
            {/* Níveis do documento do programa (GGV, GGV×CRV, GGV×CRV×RCA e
                Cliente) — voltam aqui porque só existiam na visão individual
                da Numérica, que saiu (Correção 5 do Heverton 08/10/2026).
                Mesmo espírito das abas da Ponderada: os filtros abaixo
                continuam valendo em qualquer aba. */}
            <div className="flex rounded-md border border-slate-300 overflow-hidden bg-white shadow-sm w-fit">
              {([
                { value: 'ggv', label: 'Resumo GGVs' },
                { value: 'ggv_crv', label: 'Resumo GGVs × CRVs' },
                { value: 'ggv_crv_rca', label: 'Resumo GGVs × CRVs × RCAs' },
                { value: 'cliente', label: 'Resumo Clientes' },
              ] as const).map(a => (
                <button
                  key={a.value}
                  onClick={() => setAbaNumerica(a.value)}
                  className={`px-3 py-1.5 text-sm font-medium transition-colors ${
                    abaNumerica === a.value ? 'bg-slate-700 text-white' : 'text-slate-600 hover:bg-slate-50'
                  }`}
                >
                  {a.label}
                </button>
              ))}
            </div>

            {/* Filtros GGV/Supervisor/RCA/Cliente — pedido do Claudio
                30/09/2026 ("igual às outras visões"): faltava aqui, só
                existia no modo individual e no Combinado por Rede.
                Reaproveita o MESMO state/handlers do modo individual
                (filtroGGV/CRV/RCA, filtroClienteNumerica) — GGV/CRV/RCA já
                vão pro backend via cod_ggv/cod_crv/cod_rca na query;
                Cliente filtra no navegador (ver clientesCombinadoNum). */}
            <div className="flex flex-wrap gap-2 items-end border rounded-lg p-4">
              <FiltroSelect label="GGV" value={filtroGGV?.codigo ?? ''} opts={optsGGVIndiv} onChange={selecionarGGVIndiv} />
              <FiltroSelect label="Supervisor (CRV)" value={filtroCRV?.codigo ?? ''} opts={optsCRVIndiv} onChange={selecionarCRVIndiv} />
              <FiltroSelect label="RCA" value={filtroRCA?.codigo ?? ''} opts={optsRCAIndiv} onChange={selecionarRCAIndiv} />
              <FiltroSelect label="Cliente" value={filtroClienteNumerica} opts={optsRedeIndiv} onChange={selecionarRedeIndiv} />
              {(filtroGGV || filtroCRV || filtroRCA || filtroClienteNumerica) && (
                <button className="text-xs text-primary hover:underline pb-2.5" onClick={() => { voltarPara('ggv'); setFiltroClienteNumerica('') }}>
                  Limpar filtros
                </button>
              )}
            </div>
            {/* Numérica não tem GGV/CRV/RCA em "abas" — o filtro acima já
                estreita a lista abaixo direto no backend. ehNumerica fica
                sempre false aqui (depende de vinculoAtivo, que este modo
                não tem) — já estamos dentro do branch combinado_numerica,
                então não precisa checar de novo. */}
            {(() => {
              const r = resumoNumerica(clientesCombinadoNum)
              const sortRealizado = painelCombinadoNum.sortimento.realizado_total
              return (
                <TooltipProvider delayDuration={150}>
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <div className="border rounded-lg p-4">
                      <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                        <Target className="w-4 h-4" /> Cobertura Numérica — clientes cobertos
                      </div>
                      <div className="text-2xl font-semibold flex items-center gap-2">
                        {fmt(r.cobertos)} <span className="text-sm text-muted-foreground">/ {r.totalClientes} clientes</span>
                        {r.totalClientes > 0 && <StatusBadge size="w-6 h-6" atingiu={r.cobertos >= r.totalClientes} />}
                      </div>
                      {r.classes.length > 0 && (
                        <div className="text-xs text-muted-foreground mt-0.5">
                          {textoObjetivoClasses(r.classes)}<br />Bimestre Móvel
                        </div>
                      )}
                    </div>
                    <div className="border rounded-lg p-4">
                      <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                        <Target className="w-4 h-4" /> Sortimento Numérica — média de PPAs
                      </div>
                      <div className="text-2xl font-semibold flex items-center gap-2">
                        <span>
                          {fmt(sortRealizado)}
                          {r.objetivoSortimento > 0 && <span className="text-sm font-normal text-muted-foreground"> / {fmt(r.objetivoSortimento)}</span>}
                        </span>
                        {r.objetivoSortimento > 0 && <StatusBadge size="w-6 h-6" atingiu={sortRealizado >= r.objetivoSortimento} />}
                      </div>
                      {r.objetivoSortimento > 0 && (
                        <div className="text-xs text-muted-foreground mt-0.5">
                          {r.bateramSortimento} de {r.totalAplicaveis} clientes bateram os {fmt(r.objetivoSortimento)} PPAs
                        </div>
                      )}
                    </div>
                  </div>
                </TooltipProvider>
              )
            })()}

            {abaNumerica !== 'cliente' && (() => {
              const nivel: NivelRollup = abaNumerica
              const linhas = resumirRollup(clientesCombinadoNum, nivel)
              return (
                <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                  <p className="text-xs text-muted-foreground px-3 pt-2">
                    {nivel === 'ggv' ? 'Clique num GGV pra ver as CRVs dele.' : 'Clique num grupo pra ver os Clientes dele.'}
                  </p>
                  <Table>
                    <TableHeader className="sticky top-0 z-10 bg-white">
                      <TableRow>
                        <TableHead>GGV</TableHead>
                        {nivel !== 'ggv' && <TableHead>CRV</TableHead>}
                        {nivel === 'ggv_crv_rca' && <TableHead>RCA</TableHead>}
                        <TableHead className="text-right">Qt Clientes</TableHead>
                        <TableHead className="text-right">Clientes atingindo Cobertura</TableHead>
                        <TableHead className="text-right">Clientes abaixo da Cobertura</TableHead>
                        <TableHead className="text-right">Objetivo Sortimento (PPAs)</TableHead>
                        <TableHead className="text-right">Média de PPAs</TableHead>
                        <TableHead className="text-right">Clientes atingindo Sortimento</TableHead>
                        <TableHead className="text-right">Clientes abaixo do Sortimento</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {linhas.length === 0 && (
                        <TableRow><TableCell colSpan={10} className="text-center py-8 text-muted-foreground">Sem dados pra este recorte/filtros</TableCell></TableRow>
                      )}
                      {linhas.map(g => (
                        <TableRow
                          key={`${g.cod_ggv}|${g.cod_crv}|${g.cod_rca}`}
                          className="cursor-pointer hover:bg-muted/50"
                          onClick={() => {
                            // Clicar desce um nível: filtra pelo grupo e abre a aba de baixo.
                            selecionarGGVIndiv(g.cod_ggv)
                            if (nivel === 'ggv') { setAbaNumerica('ggv_crv'); return }
                            selecionarCRVIndiv(g.cod_crv)
                            if (nivel === 'ggv_crv_rca') selecionarRCAIndiv(g.cod_rca)
                            setAbaNumerica('cliente')
                          }}
                        >
                          <TableCell className="text-sm whitespace-nowrap">{g.cod_ggv} — {g.nome_ggv}</TableCell>
                          {nivel !== 'ggv' && <TableCell className="text-sm whitespace-nowrap">{g.cod_crv} — {g.nome_crv}</TableCell>}
                          {nivel === 'ggv_crv_rca' && <TableCell className="text-sm whitespace-nowrap">{g.cod_rca} — {g.nome_rca}</TableCell>}
                          <TableCell className="text-right">{g.qt}</TableCell>
                          <TableCell className="text-right text-green-700">{g.cobertas}</TableCell>
                          <TableCell className="text-right text-red-700">{g.falta}</TableCell>
                          <TableCell className="text-right text-muted-foreground">{g.objetivoSortimento > 0 ? fmt(g.objetivoSortimento) : '—'}</TableCell>
                          <TableCell className="text-right">{fmt(g.realSortimento)}</TableCell>
                          <TableCell className="text-right text-green-700">{g.atingindoSortimento}</TableCell>
                          <TableCell className="text-right text-red-700">{g.faltaSortimento}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )
            })()}

            {abaNumerica === 'cliente' && (
            <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
              <Table>
                <TableHeader className="sticky top-0 z-10 bg-white">
                  <TableRow>
                    <TableHead>Cliente</TableHead>
                    <TableHead>Classif.</TableHead>
                    <TableHead>GGV / CRV / RCA</TableHead>
                    <TableHead className="text-right">Cobertura (R$)</TableHead>
                    <TableHead className="w-10 text-center">Status</TableHead>
                    <TableHead className="text-right">Sortimento (PPAs)</TableHead>
                    <TableHead className="w-10 text-center">Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {clientesCombinadoNum.length === 0 && (
                    <TableRow><TableCell colSpan={7} className="text-center py-8 text-muted-foreground">Sem dados pra este recorte</TableCell></TableRow>
                  )}
                  {[...clientesCombinadoNum].sort((a, b) => b.cobertura_valor - a.cobertura_valor).map(c => (
                    <TableRow
                      key={c.cnpj}
                      className="cursor-pointer hover:bg-muted/50"
                      onClick={() => setPpasAlvo({
                        cnpj: c.cnpj,
                        titulo: `${c.cod_cl || c.cnpj} - ${c.fantasia || c.razao || c.cnpj}`,
                        aplicavel: c.sortimento_aplicavel,
                        objetivo: c.sortimento_objetivo,
                        classe: c.classificacao_pdv,
                      })}
                    >
                      <TableCell className="font-medium">
                        <span className="inline-flex items-center gap-1">
                          <PackageSearch className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                          {c.cod_cl ? `${c.cod_cl} — ` : ''}{c.fantasia || c.razao || c.cnpj}
                        </span>
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">{c.classificacao_pdv || '—'}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        <div>{c.cod_ggv} — {c.nome_ggv}</div>
                        <div className="text-muted-foreground/70">{c.cod_crv} — {c.nome_crv}</div>
                        <div className="text-muted-foreground/70">{c.cod_rca} — {c.nome_rca}</div>
                      </TableCell>
                      <TableCell className="text-right">{fmtBRL(c.cobertura_valor)} <span className="text-muted-foreground">/ {fmtBRL(c.cobertura_objetivo)}</span></TableCell>
                      <TableCell className="text-center"><StatusBadge atingiu={c.cobertura_atingiu} /></TableCell>
                      <TableCell className="text-right">
                        {c.sortimento_aplicavel ? <>{fmt(c.sortimento_valor)} <span className="text-muted-foreground">/ {fmt(c.sortimento_objetivo)}</span></> : <span className="text-muted-foreground">N/A</span>}
                      </TableCell>
                      <TableCell className="text-center">{c.sortimento_aplicavel && <StatusBadge atingiu={c.sortimento_atingiu} />}</TableCell>
                    </TableRow>
                  ))}
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

          {/* Filtros por seleção — GGV/Supervisor/RCA/Rede — pra achar direto
              sem precisar clicar linha por linha (pedido do Claudio
              14/09/2026). Valem pras duas abas acima; escolher aqui move o
              nivel/drill igual clicar numa linha moveria. */}
          <div className="flex flex-wrap gap-2 items-end border rounded-lg p-4">
            <FiltroSelect label="GGV" value={filtroGGV?.codigo ?? ''} opts={optsGGVIndiv} onChange={selecionarGGVIndiv} />
            <FiltroSelect label="Supervisor (CRV)" value={filtroCRV?.codigo ?? ''} opts={optsCRVIndiv} onChange={selecionarCRVIndiv} />
            <FiltroSelect label="RCA" value={filtroRCA?.codigo ?? ''} opts={optsRCAIndiv} onChange={selecionarRCAIndiv} />
            <FiltroSelect label={ehNumerica ? 'Cliente' : 'Rede'} value={ehNumerica ? filtroClienteNumerica : (redeAberta?.cod_princ ?? '')} opts={optsRedeIndiv} onChange={selecionarRedeIndiv} />
            {(filtroGGV || filtroCRV || filtroRCA || redeAberta || filtroClienteNumerica) && (
              <button className="text-xs text-primary hover:underline pb-2.5" onClick={() => { voltarPara('ggv'); setFiltroClienteNumerica('') }}>
                Limpar filtros
              </button>
            )}
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
                <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                  <Table>
                    <TableHeader className="sticky top-0 z-10 bg-white">
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
              {/* Realizado / Objetivo atual + check/X no MESMO card (pedido
                  do Claudio 30/09/2026: "bater o olho sem fazer conta") —
                  antes eram 3 cards separados (Realizado, Objetivo atual,
                  Falta) exigindo comparar os 3 na cabeça; mesmo padrão que
                  o painel mobile já usa (FarolPublicMetasPanel.tsx). */}
              <div className="text-2xl font-semibold flex items-center gap-2">
                <span>
                  {fmt(painel.realizado.realizado_total)}
                  {(painel.proxima_faixa ?? painel.faixa_atual) && (
                    <span className="text-base font-medium text-muted-foreground"> / {fmt((painel.proxima_faixa ?? painel.faixa_atual)!.valor_meta)}</span>
                  )}
                </span>
                <StatusBadge atingiu={painel.delta <= 0} />
              </div>
              {ehCobertura && (
                <div className="text-xs text-muted-foreground mt-0.5">de {fmt(painel.realizado.redes.length)} {ehNumerica ? 'clientes' : 'redes'} no total</div>
              )}
              {painel.realizado.parcial && <Badge variant="secondary" className="mt-1">Mês em andamento</Badge>}
            </div>
            <div className="border rounded-lg p-4">
              <div className="text-muted-foreground text-xs mb-1">Objetivo atual (Faixa {painel.proxima_faixa?.faixa ?? painel.faixa_atual?.faixa ?? '—'})</div>
              <div className="text-2xl font-semibold">
                {(painel.proxima_faixa ?? painel.faixa_atual)?.valor_meta !== undefined ? fmt((painel.proxima_faixa ?? painel.faixa_atual)!.valor_meta) : '—'}
              </div>
            </div>
            <div className={`border rounded-lg p-4 ${painel.delta > 0 ? 'bg-amber-50' : 'bg-green-50'}`}>
              <div className="flex items-center gap-2 text-xs mb-1">
                {painel.delta > 0 ? <TrendingDown className="w-4 h-4 text-amber-600" /> : <TrendingUp className="w-4 h-4 text-green-700" />}
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

          <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
            <Table>
              <TableHeader className="sticky top-0 z-10 bg-white">
                <TableRow>
                  <TableHead>{redeAberta ? 'CNPJ' : nivel === 'rede' ? (ehNumerica ? 'Cliente' : 'Rede') : nivelLabelAtual}</TableHead>
                  {/* Coluna de RCA escondida quando já drilou até o RCA
                      (filtroRCA setado) — toda linha repetiria o MESMO RCA,
                      já visível no breadcrumb acima (pedido do Claudio
                      30/09/2026: "não precisa ficar repetindo o RCA"). */}
                  {mostrarColunaRCA && <TableHead>{redeAberta ? 'Documento' : nivel === 'rede' ? 'RCA' : 'Composição'}</TableHead>}
                  <TableHead className="text-right">{nivel === 'rede' || redeAberta ? 'Realizado' : ehNumerica ? 'Clientes atingindo' : 'Redes atingindo'}</TableHead>
                  {/* Status agora aparece no nível de grupo também (pedido
                      do Claudio 30/09/2026: "não está trazendo o tickado e
                      X pra essa visão nem as subsequentes") — só some no
                      nível 5 (CNPJ dentro de uma Rede aberta), que não tem
                      marcador calculado. */}
                  {!redeAberta && <TableHead className="w-24 text-center">Status</TableHead>}
                  {podeAbrirLinha && <TableHead className="w-10" />}
                </TableRow>
              </TableHeader>
              <TableBody>
                {linhas.length === 0 && (
                  <TableRow><TableCell colSpan={2 + (mostrarColunaRCA ? 1 : 0) + (!redeAberta ? 1 : 0) + (podeAbrirLinha ? 1 : 0)} className="text-center py-8 text-muted-foreground">Sem dados pra este recorte</TableCell></TableRow>
                )}
                {linhas.map((l, i) => (
                  <TableRow key={i} className={l.drill ? 'cursor-pointer hover:bg-muted/50' : undefined} onClick={l.drill}>
                    <TableCell className="font-medium">{l.nome}</TableCell>
                    {mostrarColunaRCA && <TableCell className="text-sm text-muted-foreground">{l.sub}</TableCell>}
                    <TableCell className="text-right">{(nivel === 'rede' || redeAberta) && ehCobertura ? fmtBRL(l.valor) : fmt(l.valor)}</TableCell>
                    {!redeAberta && (
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

      {/* Drill-down de PPAs de um cliente da Numérica — Correção 2 do
          Heverton 08/10/2026 (equivalente ao de itens da Ponderada). */}
      <Dialog open={!!ppasAlvo} onOpenChange={open => { if (!open) setPpasAlvo(null) }}>
        <DialogContent className="w-[95vw] max-w-3xl max-h-[85vh] overflow-y-auto uppercase text-sm [&_*]:uppercase">
          <DialogHeader>
            <DialogTitle>PPAs — {ppasAlvo?.titulo}</DialogTitle>
          </DialogHeader>
          {ppasAlvo && !ppasAlvo.aplicavel ? (
            <p className="text-sm text-muted-foreground py-6 text-center normal-case">
              O Sortimento Numérica não se aplica à classe {ppasAlvo.classe || 'deste cliente'} — só Num. A e Num. B têm PPAs a atingir.
            </p>
          ) : isLoadingPpas ? (
            <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
          ) : ppasErro ? (
            <p className="text-sm text-red-700 py-6 text-center normal-case">{(ppasErro as Error).message}</p>
          ) : (
            <>
              {ppasAlvo && (
                <div className="flex flex-wrap gap-4 text-sm border rounded-lg p-3 bg-muted/30">
                  <span><strong>{ppasLista.filter(p => p.vendeu).length}</strong> comprados</span>
                  <span><strong>{ppasLista.length}</strong> PPAs no catálogo</span>
                  <span>Objetivo: <strong>{fmt(ppasAlvo.objetivo)}</strong> PPAs distintos</span>
                </div>
              )}
              <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
                <Table>
                  <TableHeader className="sticky top-0 z-10 bg-white">
                    <TableRow>
                      <TableHead>Cód. Produto</TableHead>
                      <TableHead>PPA</TableHead>
                      <TableHead className="text-center">Status</TableHead>
                      <TableHead className="text-right">Qtd</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {ppasLista.length === 0 && (
                      <TableRow><TableCell colSpan={4} className="text-center py-8 text-muted-foreground">Sem PPAs pra esta vigência</TableCell></TableRow>
                    )}
                    {[...ppasLista].sort((x, y) => x.ppa.localeCompare(y.ppa, 'pt-BR')).map(p => (
                      <TableRow key={p.ppa}>
                        <TableCell className="font-mono text-xs whitespace-nowrap">{p.cod_prods && p.cod_prods.length > 0 ? p.cod_prods.join(', ') : '—'}</TableCell>
                        <TableCell className="text-sm">
                          {p.ppa}
                          {p.abaixo_minimo && <span className="ml-2 text-xs text-amber-700 normal-case">abaixo do mínimo de 3 un.</span>}
                        </TableCell>
                        <TableCell className="text-center"><StatusBadge atingiu={p.vendeu} labelSim="Comprou" labelNao="Não comprou" /></TableCell>
                        <TableCell className="text-right">{p.qtd ? fmt(p.qtd) : '—'}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* Drill-down de itens (Sortimento): clicar numa Rede (aba "Resumo
          Redes") mostra os itens de TODAS as lojas dela; clicar numa loja
          (aba "Resumo Rede×Cliente") mostra só os itens daquele CNPJ.
          Vendeu/não vendeu, Qtd e Valor — pedido do Claudio 10/09/2026. */}
      <Dialog open={!!itensAlvo} onOpenChange={open => { if (!open) setItensAlvo(null) }}>
        <DialogContent className="w-[95vw] max-w-5xl max-h-[85vh] overflow-y-auto uppercase text-sm [&_*]:uppercase">
          <DialogHeader>
            <DialogTitle>Itens — {itensAlvo?.titulo}</DialogTitle>
          </DialogHeader>
          {isLoadingItens ? (
            <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
          ) : (
            <>
              {/* Resumo vendeu/objetivo — pedido do Claudio 14/09/2026: a
                  lista de itens sozinha não deixava claro se bateu a meta;
                  objetivo vem da mesma faixa de meta que já aparece na
                  coluna "Obj. EANs" da tabela (Sortimento mede EANs
                  distintos vendidos, não linhas desta lista). */}
              {itensAlvo && (
                <div className="flex flex-wrap gap-4 text-sm border rounded-lg p-3 bg-muted/30">
                  <span><strong>{itensLista.filter(it => it.vendeu).length}</strong> vendidos</span>
                  <span><strong>{itensLista.length}</strong> itens no catálogo</span>
                  <span>Objetivo: <strong>{fmt(itensAlvo.objetivo)}</strong> EANs distintos</span>
                </div>
              )}
              {/* Nota só aparece vindo de uma linha de rollup (GGVxCRV/
                  GGVxCRVxRCA) — achado 18/09/2026: sem isso, "22 vendidos ≥
                  Objetivo 19" ao lado de "0 de 9 redes atingindo" parece
                  contradição. Não é: esta lista é a UNIÃO de todas as lojas
                  do grupo; quem decide atingiu/não atingiu é a MÉDIA de
                  EANs distintos por loja, calculada Rede a Rede (regra do
                  PRD, não bug) — uma Rede inteira pode passar dessa soma e
                  ainda assim nenhuma Rede dela, individualmente, bater a
                  média sozinha. */}
              {itensAlvo && itensAlvo.qtdRedes !== undefined && (
                <div className="flex items-start gap-2 text-xs border border-amber-300 bg-amber-50 rounded-lg p-3 -mt-2">
                  <span className="text-amber-800">
                    Esta lista soma <strong>todas as lojas das {itensAlvo.qtdRedes} Redes</strong> deste
                    grupo (união — vendeu se qualquer loja vendeu). O objetivo de {fmt(itensAlvo.objetivo)} EANs
                    vale <strong>por Rede</strong> (média de EANs distintos entre as lojas dela, não a soma do
                    grupo) — por isso <strong>{itensAlvo.qtdAtingindo ?? 0} de {itensAlvo.qtdRedes}</strong> Redes
                    deste grupo atingem sozinhas, mesmo esta lista somando mais que o objetivo.
                  </span>
                </div>
              )}
              <div className="border rounded-lg overflow-auto max-h-[70vh] [&>div]:overflow-visible [&_th]:sticky [&_th]:top-0 [&_th]:z-10 [&_th]:bg-white [&_th]:shadow-[0_1px_0_0_#e2e8f0] [&_th]:uppercase [&_th]:tracking-wide [&_th]:font-semibold [&_th]:text-xs">
              <Table>
                <TableHeader className="sticky top-0 z-10 bg-white">
                  <TableRow>
                    <TableHead>Cód. Produto</TableHead>
                    <TableHead>Produto</TableHead>
                    <TableHead className="text-center">Status</TableHead>
                    <TableHead className="text-right">Qtd</TableHead>
                    <TableHead className="text-right">Valor</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {itensLista.length === 0 && (
                    <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Sem itens pra esta vigência</TableCell></TableRow>
                  )}
                  {[...itensLista].sort((x, y) => (x.nome || x.ean).localeCompare(y.nome || y.ean, 'pt-BR')).map((it, i) => (
                    <TableRow key={i}>
                      <TableCell className="font-mono text-xs whitespace-nowrap">{it.cod_prods && it.cod_prods.length > 0 ? it.cod_prods.join(', ') : '—'}</TableCell>
                      <TableCell className="text-sm">{it.nome || '—'}</TableCell>
                      <TableCell className="text-center"><StatusBadge atingiu={it.vendeu} labelSim="Vendeu" labelNao="Não vendeu" /></TableCell>
                      <TableCell className="text-right">{it.qtd ? fmt(it.qtd) : '—'}</TableCell>
                      <TableCell className="text-right">{it.valor ? fmtBRL(it.valor) : '—'}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              </div>
            </>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
