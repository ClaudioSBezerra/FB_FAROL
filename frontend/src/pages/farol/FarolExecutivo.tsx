import { useState, useEffect, useMemo, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  ChevronLeft, Search, X, Calendar, Filter, ChevronDown,
} from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { cn } from '@/lib/utils'
import { BRLValue } from '@/lib/farolMoney'
import { LoadingState } from '@/components/farol/LoadingState'
import { SortIndicator, useSortedCards, type SortState } from '@/components/farol/SortToggle'

// ─── Tipos ────────────────────────────────────────────────────────────────────

type Cor = 'verde' | 'amarelo' | 'vermelho'
type Fluxo = 'faturado' | 'transmitido' | 'cancdev' | 'cortado'

interface DrillStep { level: string; value: string; label: string }

interface CardItem {
  key: string
  label: string
  level: string
  level_label: string
  valor_atual: number
  valor_ant: number
  pct: number
  cor: Cor
  plucro: number
  plucro_ant: number
  positivados: number
  base_cli: number
  positpct: number
  positivados_ant: number
  base_cli_ant: number
  positpct_ant: number
  posit_cor: Cor
  mix: number
  mix_ant: number
  mix_cor: Cor
  mix_total: number
  mix_total_ant: number
  comp?: Composicao      // deltas das categorias (faturado) p/ os toggles "Incluir X"
  comp_ant?: Composicao
}

// Composição da venda líquida (mig 189/190). valor_atual/valor_ant já são o
// Líquido; estes deltas são somados quando o botão "Incluir X" está ligado.
interface Composicao { bonif: number; transf: number; remessa: number; devol: number; cancel: number }

// Categorias somáveis ao Líquido via botões "Incluir X" (só fluxo faturado).
const INCLUIR_CATS = [
  { key: 'bonif',   label: 'Bonificação' },
  { key: 'transf',  label: 'Transferência' },
  { key: 'remessa', label: 'Remessa' },
  { key: 'devol',   label: 'Devoluções' },
  { key: 'cancel',  label: 'Canceladas' },
] as const
type CompKey = typeof INCLUIR_CATS[number]['key']

// Soma dos deltas das categorias ligadas (0 se comp ausente — transmitido/scan).
function sumDelta(comp: Composicao | undefined, inc: Set<CompKey>): number {
  if (!comp || inc.size === 0) return 0
  let d = 0
  if (inc.has('bonif'))   d += comp.bonif
  if (inc.has('transf'))  d += comp.transf
  if (inc.has('remessa')) d += comp.remessa
  if (inc.has('devol'))   d += comp.devol
  if (inc.has('cancel'))  d += comp.cancel
  return d
}

// Replica farol_v2_api.pickCor: verde se atual ≥ anterior; neutro (verde) sem
// comparativo. Usado no recálculo client-side quando um toggle está ligado.
function corFor(atual: number, ant: number, hasComp: boolean): { pct: number; cor: Cor } {
  if (!hasComp) return { pct: 0, cor: 'verde' }
  let pct = 0
  if (ant > 0) pct = (atual / ant) * 100
  else if (atual > 0) pct = 100
  return { pct, cor: pct >= 100 ? 'verde' : 'vermelho' }
}

interface KPI {
  total_atual: number
  total_ant: number
  total_pct: number
  total_cor: Cor
  total_plucro: number
  total_plucro_ant: number
  total_positivados: number
  total_base_cli: number
  total_positpct: number
  total_positivados_ant: number
  total_base_cli_ant: number
  total_positpct_ant: number
  total_posit_cor: Cor
  avg_mix: number
  avg_mix_ant: number
  mix_cor: Cor
  total_mix_total: number
  total_mix_total_ant: number
  comp?: Composicao
  comp_ant?: Composicao
}

interface CardsResponse {
  cards: CardItem[]
  kpi: KPI
  periodo: {
    fluxo?: string
    ref_inicio?: string; ref_fim?: string
    comp_inicio?: string; comp_fim?: string
    ref_ano?: number; ref_mes?: number
    label?: string
  }
  periodos: string[]
  view: string
  drill_path: DrillStep[]
  next_level: string
  next_level_label: string
  // diag — como o backend serviu este recorte. Sem isso, uma lista vazia é
  // ambígua na tela: "não teve venda" ou "a consulta falhou"? (incidente
  // 27/07/2026: consulta morria e o painel mostrava 0 cards em silêncio).
  diag?: {
    lento: boolean
    falhou: boolean
    combinacao?: string
    ms: number
  }
}

interface DimOption { key: string; label: string }
interface DimsResponse {
  fornec?: DimOption[]
  gerente?: DimOption[]
  supervisor?: DimOption[]
  rca?: DimOption[]
  cli?: DimOption[]
  uf?: string[]
  // empresa passou a vir com rótulo na mig 204 ({key:"20", label:"JC CONCEICAO
  // DO JACUIPE-BA"}). optionsFor já aceitava as duas formas, então a mudança no
  // backend não exigiu nada aqui além do tipo.
  empresa?: DimOption[]
  tipo_venda?: DimOption[] // mig 187/188 (faturado) e 203 (transmitido)
}

// ─── Tons ────────────────────────────────────────────────────────────────────

const HEADER_BG = 'bg-slate-600'
const HEADER_TXT_FAINT = 'text-slate-300'
const BTN_PRIMARY_BG = 'bg-slate-700'

// Cabeçalho da LISTA / TOTAL — tarja TURQUESA forte (cyan-800)
// Cores vivas (300/400) sobre cyan-800 — contraste alto, fácil ler à distância:
//   NOME:        branco          (peso/identidade)
//   VENDA:       yellow-300      (amarelo vivo — valor monetário)
//   POSITIVAÇÃO: lime-300        (verde brilhante — sucesso/clientes)
//   MIX:         fuchsia-400     (rosa vivo — variedade; NÃO usa ciano, evita conflito)
// Azul oficial do Farol (mesmo da página de Login — logo Target #1e293b)
const TARJA_BG = 'bg-[#1e293b]'
const COL_NOME_TXT        = 'text-white'
const COL_VENDA_TXT       = 'text-yellow-300'
const COL_POSITIVACAO_TXT = 'text-lime-300'
const COL_MIX_TXT         = 'text-fuchsia-400'

// ─── Utilitários ──────────────────────────────────────────────────────────────

function fmtPct(v: number) {
  if (!isFinite(v) || v === 0) return '—'
  if (v >= 10000) return '>9999%'
  return Math.round(v) + '%'
}
function fmtInt(v: number) {
  if (v === 0) return '—'
  return v.toLocaleString('pt-BR', { maximumFractionDigits: 0 })
}
function fmtMix(v: number) {
  if (v === 0) return '—'
  return v.toLocaleString('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
}

const COR_TXT: Record<Cor, string> = {
  verde:    'text-emerald-600',
  amarelo:  'text-amber-500',
  vermelho: 'text-red-600',
}

// Cores FORTES para a linha Total (fundo cinza): verde/vermelho mais saturados
// e escuros + extrabold p/ máximo contraste sobre o gradiente slate.
// Filtros de SELEÇÃO ÚNICA — selecionar outro valor TROCA o atual em vez de
// somar. Hoje só Filial (`empresa`), por um motivo de desempenho que é
// estrutural, não um detalhe de implementação a ser "consertado" depois:
//
// Com UMA filial, pickAggForCrossFilter roteia para as aggs V10/V11 e o card
// sai em ~25 ms. Com DUAS OU MAIS ele é obrigado a recusar a agg — 23% dos
// clientes compram de mais de uma filial e a soma das linhas do grão os
// contaria uma vez por filial — e cai no scan de vendas_*: 97 s medidos em
// produção 13/08/2026, com aviso de lentidão na tela.
//
// Índice não resolve: EXPLAIN do mesmo recorte mostrou Parallel Seq Scan
// porque duas filiais já são ~7,8 M linhas (um terço da base), fração em que
// varrer é legitimamente mais rápido que usar índice.
//
// A alternativa seria relaxar o guard e aceitar Mix Médio aproximado com 2+
// filiais. O gestor preferiu analisar uma filial por vez e manter todos os
// números exatos — decisão de 13/08/2026.
const SINGLE_SELECT_COLS = new Set(['empresa'])

const COR_TXT_TOTAL: Record<Cor, string> = {
  verde:    'text-emerald-800',
  amarelo:  'text-amber-700',
  vermelho: 'text-red-700',
}

// ─── Preset de período ───────────────────────────────────────────────────────

function ymd(y: number, m: number, d: number): string {
  return `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
}
function lastDayOfMonth(y: number, m: number): number { return new Date(y, m, 0).getDate() }
function addDays(s: string, days: number): string {
  const [y, m, d] = s.split('-').map(Number)
  const dt = new Date(Date.UTC(y, m - 1, d))
  dt.setUTCDate(dt.getUTCDate() + days)
  return dt.toISOString().slice(0, 10)
}
function addYears(s: string, n: number): string {
  const [y, m, d] = s.split('-').map(Number)
  return ymd(y + n, m, d)
}
function todayYMD(): string { return new Date().toISOString().slice(0, 10) }
function rangeDaysInclusive(ini: string, fim: string): number {
  const [yi, mi, di] = ini.split('-').map(Number)
  const [yf, mf, df] = fim.split('-').map(Number)
  return Math.round((Date.UTC(yf, mf - 1, df) - Date.UTC(yi, mi - 1, di)) / 86_400_000) + 1
}
function fmtDateBR(s: string): string {
  if (!s) return ''
  const [y, m, d] = s.split('-')
  return `${d}/${m}/${y}`
}

// Presets (ordem da barra, da esquerda para a direita):
//  ytd          — ano anterior completo (jan-dez) × jan até hoje, ano corrente
//  yoy          — último mês 100% importado × mesmo mês ano anterior
//  ant_corrente — dois últimos meses completos carregados (M-1 vs M-2)
//  mes_corrente — dia 1 até hoje, mês corrente × mesmo período do ano anterior
//  dia_anterior — ontem × mesmo dia da semana 7 dias antes (régua do Pulso)
//  last7        — últimos 7 dias × 7 dias anteriores
//  last30       — últimos 30 dias × 30 dias anteriores
type Preset = 'mes_corrente' | 'yoy' | 'ant_corrente' | 'ytd' | 'dia_anterior' | 'last7' | 'last30'

const PRESET_LABEL: Record<Preset, string> = {
  ytd:          'Ano × Ano',
  yoy:          'Último mês YoY',
  ant_corrente: 'M-1 vs M-2',
  mes_corrente: 'Mês Corrente',
  dia_anterior: 'Dia Anterior',
  last7:        '7 dias',
  last30:       '30 dias',
}

function presetRange(p: Preset, last?: { ano: number; mes: number }) {
  const now = new Date()
  const todayY = now.getUTCFullYear()
  const todayM = now.getUTCMonth() + 1  // 1..12
  const todayD = now.getUTCDate()
  const today = ymd(todayY, todayM, todayD)

  // Último mês 100% importado — fallback para o mês anterior ao corrente
  const lastY = last?.ano ?? todayY
  const lastM = last?.mes ?? (todayM > 1 ? todayM - 1 : 12)

  switch (p) {
    case 'ytd': {
      // Ano anterior INTEIRO × Janeiro até hoje do ano corrente
      return {
        ref_inicio:  ymd(todayY, 1, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, 1, 1),
        comp_fim:    ymd(todayY - 1, 12, 31),
      }
    }
    case 'yoy': {
      // Último mês 100% importado × mesmo mês do ano anterior — ambos completos
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(lastY - 1, lastM, 1),
        comp_fim:    ymd(lastY - 1, lastM, lastDayOfMonth(lastY - 1, lastM)),
      }
    }
    case 'ant_corrente': {
      // M-1 vs M-2: dois últimos meses completos carregados
      let prevM = lastM - 1, prevY = lastY
      if (prevM === 0) { prevM = 12; prevY-- }
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(prevY, prevM, 1),
        comp_fim:    ymd(prevY, prevM, lastDayOfMonth(prevY, prevM)),
      }
    }
    case 'mes_corrente': {
      // Dia 1 até hoje do mês corrente × MESMO período do ANO ANTERIOR
      // (decisão do gestor — só WEB; mobile mantém em farolPresets.ts).
      const dayCap = Math.min(todayD, lastDayOfMonth(todayY - 1, todayM))
      return {
        ref_inicio:  ymd(todayY, todayM, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, todayM, 1),
        comp_fim:    ymd(todayY - 1, todayM, dayCap),
      }
    }
    case 'dia_anterior': {
      // Ontem × mesmo dia da semana 7 dias antes (régua do Pulso — evita
      // falso alarme de fim de semana). Um único dia em cada ponta.
      const ontem = addDays(today, -1)
      return {
        ref_inicio:  ontem,
        ref_fim:     ontem,
        comp_inicio: addDays(ontem, -7),
        comp_fim:    addDays(ontem, -7),
      }
    }
    case 'last7': {
      const fim = today
      const ini = addDays(fim, -6)
      return {
        ref_inicio:  ini,
        ref_fim:     fim,
        comp_inicio: addDays(ini, -7),
        comp_fim:    addDays(fim, -7),
      }
    }
    case 'last30':
    default: {
      const fim = today
      const ini = addDays(fim, -29)
      return {
        ref_inicio:  ini,
        ref_fim:     fim,
        comp_inicio: addDays(ini, -30),
        comp_fim:    addDays(fim, -30),
      }
    }
  }
}

// ─── Cabeçalho colorido + subtítulos (reutilizado em Total e Lista) ─────────

// Venda em 5fr (era 3fr) — igualado à Positivação: Positivação tem mais
// colunas (5) mas cada uma é curta (inteiro/percentual); Venda tem só 3, mas
// R$ 1.851.348.206,03 (TOTAL, empresa inteira) não cabe no espaço que sobrava
// de 3fr ÷ 3 colunas. Visto em produção 10/08/2026 com zoom reduzido: mesmo
// com break-words + <wbr/> corretos (sem sobrepor nem cortar centavo), o
// valor ainda quebrava em 2 linhas por falta de largura, não por falta de
// ponto de quebra decente.
const GRID_COLS        = 'grid-cols-[minmax(180px,2fr)_5fr_5fr_1fr]'
const GRID_COLS_NOPOS  = 'grid-cols-[minmax(180px,2fr)_3fr_1.4fr]'
// Positivação não faz sentido no nível Cliente/Produto → coluna some.
const gridCols = (hidePosit?: boolean) => (hidePosit ? GRID_COLS_NOPOS : GRID_COLS)

function ColumnsHeader({
  hidePosit,
  sortState,
  onSortChange,
  search,
  onSearchChange,
  searchPlaceholder,
}: {
  hidePosit?: boolean
  sortState?: SortState
  onSortChange?: (next: { field: 'valor' | 'pct' }) => void
  search?: string
  onSearchChange?: (v: string) => void
  searchPlaceholder?: string
}) {
  const GC = gridCols(hidePosit)
  // Sort/search são opcionais — quando ausentes, header é apenas decorativo.
  const interactive = !!sortState && !!onSortChange
  return (
    <>
      {/* Tarja preta única; cores apenas nos textos */}
      <div className={cn('grid', GC, TARJA_BG)}>
        <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold flex items-center gap-2', COL_NOME_TXT)}>
          <span>Nome</span>
          {/* Busca embutida na coluna Nome — antes ficava perdida na toolbar. */}
          {onSearchChange !== undefined && (
            <div className="relative ml-auto normal-case">
              <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-slate-400" />
              <input
                value={search ?? ''}
                onChange={e => onSearchChange(e.target.value)}
                placeholder={searchPlaceholder ?? 'Buscar...'}
                className="pl-7 pr-7 py-1 text-sm font-normal text-slate-700 bg-white border border-slate-300 rounded w-48"
              />
              {search && (
                <button
                  onClick={() => onSearchChange('')}
                  className="absolute right-1 top-1/2 -translate-y-1/2 p-0.5 text-slate-400 hover:text-slate-600"
                  type="button"
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </div>
          )}
        </div>
        <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold text-center', COL_VENDA_TXT)}>
          Venda
        </div>
        {!hidePosit && (
          <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold text-center', COL_POSITIVACAO_TXT)}>
            Positivação
          </div>
        )}
        <div className={cn('px-3 py-2 text-sm uppercase tracking-wider font-bold text-center', COL_MIX_TXT)}>
          Mix Médio
        </div>
      </div>
      {/* Linha clara: subtítulos. "Período Atual" e "%" são clicáveis pra ordenar.
          items-start: sem isso, o grid (align-items:stretch por padrão) esticava
          os 4 blocos (Nome/Venda/Positivação/Realizado) até a altura do mais alto
          — "% POSIT. ATUAL X ANTERIOR" quebra em 2 linhas e é o mais alto. "Período
          Anterior" é <div> simples (texto fica no topo da célula esticada mesmo
          assim), mas "Período Atual" e "%" são inline-flex items-center (têm a
          setinha de ordenação do lado) — dentro de uma célula esticada mais alta
          que o próprio conteúdo, o items-center centraliza o texto NO MEIO dessa
          altura extra, descendo em relação ao "Período Anterior" ao lado. */}
      <div className={cn('grid items-start', GC, 'bg-slate-50 border-y border-slate-200')}>
        <div className="px-3 py-1.5 text-sm uppercase tracking-wide text-slate-400 font-medium">
          {/* vazio */}
        </div>
        <div className="grid grid-cols-[2fr_2fr_1fr] gap-1 px-2 py-1.5 text-sm uppercase tracking-wide text-slate-500 font-semibold text-center">
          <div>Período Anterior</div>
          <div className="inline-flex items-center justify-center">
            <span>Período Atual</span>
            {interactive && (
              <SortIndicator
                active={sortState!.field === 'valor'}
                direction={sortState!.direction}
                onClick={() => onSortChange!({ field: 'valor' })}
              />
            )}
          </div>
          <div className="inline-flex items-center justify-center">
            <span>%</span>
            {interactive && (
              <SortIndicator
                active={sortState!.field === 'pct'}
                direction={sortState!.direction}
                onClick={() => onSortChange!({ field: 'pct' })}
              />
            )}
          </div>
        </div>
        {!hidePosit && (
          <div className="grid grid-cols-5 gap-1 px-2 py-1.5 text-sm uppercase tracking-wide text-slate-500 font-semibold text-center">
            <div>Clientes Ativos</div>
            <div>% Posit. Atual</div>
            <div>Posit. Anterior</div>
            <div>Posit. Atual</div>
            <div>% Posit. Atual X Anterior</div>
          </div>
        )}
        <div className="px-2 py-1.5 text-sm uppercase tracking-wide text-slate-500 font-semibold text-center">
          Realizado
        </div>
      </div>
    </>
  )
}

// ─── Linha de dados (Total OU fornecedor) ────────────────────────────────────

interface RowProps {
  card: CardItem
  isTotal?: boolean
  onClick?: () => void
  hidePosit?: boolean
}

function DataRow({ card, isTotal = false, onClick, hidePosit }: RowProps) {
  const clickable = !!onClick
  const valueNum = isTotal
    ? 'text-sm font-bold tabular-nums text-slate-900'
    : 'text-sm font-bold tabular-nums text-slate-800'
  const valueLabelCls = isTotal
    ? 'text-sm font-extrabold'
    : 'text-sm font-semibold'

  return (
    <div
      role={clickable ? 'button' : undefined}
      onClick={onClick}
      className={cn(
        'grid', gridCols(hidePosit),
        'border-b border-slate-100 last:border-b-0',
        isTotal
          ? 'bg-gradient-to-r from-slate-400 via-slate-300 to-slate-500 border-b-2 border-slate-500'
          : 'bg-white',
        clickable && 'cursor-pointer hover:bg-slate-50 transition-colors',
      )}
    >
      {/* Nome — "código - descrição" (exceto no Total). Vale p/ Indústria, GGV,
          Supervisor, RCA, Cliente e Produto. */}
      <div className="px-3 py-2.5 flex items-center">
        <span className={cn('truncate', valueLabelCls, isTotal ? 'text-slate-900 uppercase tracking-wider' : 'text-slate-800')} title={isTotal ? card.label : `${card.key} - ${card.label}`}>
          {isTotal ? card.label : `${card.key} - ${card.label}`}
        </span>
      </div>

      {/* VENDA — min-w-0 nas células + break-words no valor: sem isso, o TOTAL
          (a soma da empresa inteira, o maior número da tabela) passa de
          R$ 1 bi, vira uma "palavra" mais larga que o terço da coluna
          (grid-cols-3, só 4px de gap) e cola no valor vizinho — foi o que
          apareceu em produção 10/08/2026 (R$ 1.851.348.206,08 grudado em
          R$ 1.137.776.288,06, sem espaço nenhum entre os dois). */}
      <div className="grid grid-cols-[2fr_2fr_1fr] gap-1 px-2 py-2.5 items-center min-w-0">
        <div className={cn(valueNum, 'text-center min-w-0 break-words')}><BRLValue v={card.valor_ant} /></div>
        <div className={cn(valueNum, 'text-center min-w-0 break-words')}><BRLValue v={card.valor_atual} /></div>
        <div className={cn('text-center tabular-nums', isTotal ? 'text-base font-extrabold' : 'text-sm font-bold', isTotal ? COR_TXT_TOTAL[card.cor] : COR_TXT[card.cor])}>
          {fmtPct(card.pct)}
        </div>
      </div>

      {/* POSITIVAÇÃO — Clientes Ativos (carteira) + positivados Anterior × Atual + % penetração.
          Escondida no nível Cliente/Produto (não faz sentido). min-w-0 aqui
          também: base_cli/positivados são inteiros, bem mais curtos que os
          valores em R$, mas a carteira somada da empresa inteira ainda pode
          passar de 6 dígitos e vale a mesma proteção. */}
      {!hidePosit && (
        <div className="grid grid-cols-5 gap-1 px-2 py-2.5 items-center min-w-0">
          <div className={cn(valueNum, 'text-center min-w-0 break-words')}>{fmtInt(card.base_cli)}</div>
          {/* % Posit. Atual (colorido — destaque) */}
          <div className={cn('text-center tabular-nums', isTotal ? 'text-base font-extrabold' : 'text-sm font-bold', isTotal ? COR_TXT_TOTAL[card.posit_cor] : COR_TXT[card.posit_cor])}>
            {fmtPct(card.positpct)}
          </div>
          {/* Posit. Anterior (cinza) */}
          <div className={cn(valueNum, 'text-center min-w-0 break-words')}>{fmtInt(card.positivados_ant)}</div>
          {/* Posit. Atual */}
          <div className={cn(valueNum, 'text-center min-w-0 break-words')}>{fmtInt(card.positivados)}</div>
          {/* % Posit. Atual X Anterior (mesmo dado, apenas label diferente) */}
          <div className={cn('text-center tabular-nums', isTotal ? 'text-base font-extrabold' : 'text-sm font-bold', isTotal ? COR_TXT_TOTAL[card.posit_cor] : COR_TXT[card.posit_cor])}>
            {fmtPct(card.positpct)}
          </div>
        </div>
      )}

      {/* MIX MÉDIO — só a média de SKUs/cliente. O "de Y" (universo) foi omitido
          a pedido do gestor (gera confusão; muitas variáveis influenciam o total). */}
      {/* No Totalizador (isTotal) o fundo é cinza → mix em preto (verde sumia). */}
      <div className="px-2 py-2.5 flex items-center justify-center gap-1 flex-wrap">
        <span className={cn('font-bold tabular-nums', isTotal ? 'text-sm font-bold text-slate-900' : cn('text-sm', COR_TXT[card.mix_cor]))}>
          {fmtMix(card.mix)}
        </span>
      </div>
    </div>
  )
}

// ─── DateRangeFilter ─────────────────────────────────────────────────────────

interface DateRangeFilterProps {
  label: string
  inicio: string
  fim: string
  onChangeInicio: (v: string) => void
  onChangeFim: (v: string) => void
  borderColor?: string
}

function DateRangeFilter({ label, inicio, fim, onChangeInicio, onChangeFim, borderColor = 'border-slate-300' }: DateRangeFilterProps) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [])

  const hasValue = inicio && fim
  const summary = hasValue ? `${fmtDateBR(inicio)} → ${fmtDateBR(fim)}` : '—'

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className={cn(
          'inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium border rounded-md bg-white shadow-sm',
          hasValue ? 'border-slate-600 text-slate-900' : 'border-slate-300 text-slate-600 hover:bg-slate-50',
        )}
      >
        <span className="text-sm text-slate-400 font-semibold uppercase tracking-wide">{label}:</span>
        <span>{summary}</span>
        <ChevronDown className="h-3 w-3 opacity-60" />
      </button>
      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 bg-white border border-slate-200 rounded-md shadow-lg p-3 min-w-[290px]">
          <div className="text-sm uppercase tracking-wider text-slate-500 font-semibold mb-2">{label}</div>
          <div className="flex gap-2 items-center">
            <input
              type="date"
              value={inicio}
              onChange={e => onChangeInicio(e.target.value)}
              className={cn('flex-1 px-2 py-1.5 text-sm border-2 rounded bg-white', borderColor)}
            />
            <span className="text-slate-400 text-sm font-bold">→</span>
            <input
              type="date"
              value={fim}
              onChange={e => onChangeFim(e.target.value)}
              className={cn('flex-1 px-2 py-1.5 text-sm border-2 rounded bg-white', borderColor)}
            />
          </div>
          {inicio && fim && (
            <div className="text-sm text-slate-500 mt-1.5">
              {rangeDaysInclusive(inicio, fim)} dia(s) — {fmtDateBR(inicio)} a {fmtDateBR(fim)}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── MultiSelect ─────────────────────────────────────────────────────────────

interface MultiSelectProps {
  label: string
  options: { key: string; label: string }[]
  selected: string[]
  onChange: (next: string[]) => void
  onOpen?: () => void   // disparado ao abrir (usado p/ lazy-load de Cliente)
  loading?: boolean     // exibe "Carregando..." no lugar da lista
  // single: uma opção por vez (ver SINGLE_SELECT_COLS). Só muda a APRESENTAÇÃO
  // — quem garante a regra é o setFilter. Sem isto o usuário veria caixas de
  // marcar e a anterior "desmarcando sozinha" ao clicar na próxima.
  single?: boolean
}

function MultiSelect({ label, options, selected, onChange, onOpen, loading, single }: MultiSelectProps) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [])

  const filtered = useMemo(() => {
    const s = q.trim().toLowerCase()
    if (!s) return options
    return options.filter(o => o.label.toLowerCase().includes(s) || o.key.toLowerCase().includes(s))
  }, [options, q])

  const toggle = (k: string) => {
    onChange(selected.includes(k) ? selected.filter(x => x !== k) : [...selected, k])
  }

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => { const next = !o; if (next) onOpen?.(); return next })}
        className={cn(
          'inline-flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium border rounded-md bg-white shadow-sm',
          selected.length > 0 ? 'border-slate-600 text-slate-900' : 'border-slate-300 text-slate-600 hover:bg-slate-50',
        )}
      >
        {label}
        {selected.length > 0 && (
          <span className={cn('inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full text-white text-sm font-bold', BTN_PRIMARY_BG)}>
            {selected.length}
          </span>
        )}
        <ChevronDown className="h-3 w-3 opacity-60" />
      </button>

      {open && (
        <div className="absolute left-0 top-full mt-1 z-50 w-72 bg-white border border-slate-200 rounded-md shadow-lg overflow-hidden">
          <div className="p-2 border-b border-slate-100">
            <div className="relative">
              <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-slate-400" />
              <input
                autoFocus
                value={q}
                onChange={e => setQ(e.target.value)}
                placeholder={`Buscar ${label.toLowerCase()}...`}
                className="w-full pl-7 pr-2 py-1.5 text-sm border border-slate-200 rounded"
              />
            </div>
          </div>
          <div className="max-h-64 overflow-y-auto">
            {loading && (
              <div className="px-3 py-4 text-center text-sm text-slate-400">Carregando...</div>
            )}
            {!loading && filtered.length === 0 && (
              <div className="px-3 py-4 text-center text-sm text-slate-400">Nenhum resultado</div>
            )}
            {!loading && filtered.map(opt => {
              const checked = selected.includes(opt.key)
              return (
                <label key={opt.key} className="flex items-center gap-2 px-3 py-1.5 hover:bg-slate-50 cursor-pointer text-sm">
                  <input
                    type={single ? 'radio' : 'checkbox'}
                    checked={checked}
                    onChange={() => toggle(opt.key)}
                    className="w-3.5 h-3.5 accent-slate-700"
                  />
                  <span className={cn('truncate', checked && 'font-medium text-slate-900')}>{opt.label}</span>
                </label>
              )
            })}
          </div>
          {single && (
            <div className="border-t border-slate-100 px-3 py-1.5 text-sm text-slate-500 normal-case">
              Uma por vez — escolher outra troca a atual.
            </div>
          )}
          {selected.length > 0 && (
            <div className="border-t border-slate-100 p-2 flex items-center justify-between">
              <span className="text-sm text-slate-500">{selected.length} selecionado(s)</span>
              <button onClick={() => onChange([])} className="text-sm text-slate-500 hover:text-red-600 font-medium">
                Limpar
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ─── Hooks ───────────────────────────────────────────────────────────────────

interface UseCardsArgs {
  view: string
  fluxo: Fluxo
  ref_inicio: string
  ref_fim: string
  comp_inicio: string
  comp_fim: string
  drillPath: DrillStep[]
  filters: Record<string, string[]>
}

function useCards(a: UseCardsArgs) {
  return useQuery<CardsResponse>({
    queryKey: ['farol-v2-cards', a.view, a.fluxo, a.ref_inicio, a.ref_fim, a.comp_inicio, a.comp_fim,
      JSON.stringify(a.drillPath), JSON.stringify(a.filters)],
    enabled: !!a.ref_inicio && !!a.ref_fim,
    queryFn: async () => {
      const p = new URLSearchParams({
        view: a.view, fluxo: a.fluxo,
        ref_inicio: a.ref_inicio, ref_fim: a.ref_fim,
      })
      if (a.comp_inicio && a.comp_fim) { p.set('comp_inicio', a.comp_inicio); p.set('comp_fim', a.comp_fim) }
      if (a.drillPath.length > 0) p.set('drill', JSON.stringify(a.drillPath))
      Object.entries(a.filters).forEach(([k, v]) => { if (v.length > 0) p.set(k, v.join(',')) })
      const r = await fetch(`/api/v2/farol/cards?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dados')
      return r.json()
    },
    staleTime: 2 * 60_000,
    gcTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

function useDims(fluxo: Fluxo, ref_inicio: string, ref_fim: string) {
  return useQuery<DimsResponse>({
    queryKey: ['farol-v2-dims', fluxo, ref_inicio, ref_fim],
    enabled: !!ref_inicio && !!ref_fim,
    queryFn: async () => {
      const p = new URLSearchParams({ fluxo, ref_inicio, ref_fim })
      const r = await fetch(`/api/v2/farol/dims?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar dimensões')
      return r.json()
    },
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// Lazy-load do dropdown de Cliente (cod_cli): 34k+ opções, ~3s. Só busca quando
// `enabled` vira true (ao abrir o dropdown de Cliente pela primeira vez).
function useDimsCli(fluxo: Fluxo, ref_inicio: string, ref_fim: string, enabled: boolean) {
  return useQuery<{ cli: DimOption[] }>({
    queryKey: ['farol-v2-dims-cli', fluxo, ref_inicio, ref_fim],
    enabled: enabled && !!ref_inicio && !!ref_fim,
    queryFn: async () => {
      const p = new URLSearchParams({ fluxo, ref_inicio, ref_fim, dim: 'cli' })
      const r = await fetch(`/api/v2/farol/dims?${p}`)
      if (!r.ok) throw new Error('Falha ao carregar clientes')
      return r.json()
    },
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  })
}

function useUltimoPeriodo() {
  return useQuery<{ ref_ano?: number; ref_mes?: number; periodos: string[] }>({
    queryKey: ['farol-v2-periodos'],
    queryFn: async () => {
      const r = await fetch('/api/v2/farol/cards?view=V01')
      if (!r.ok) throw new Error()
      const d = await r.json() as CardsResponse
      return { ref_ano: d.periodo.ref_ano, ref_mes: d.periodo.ref_mes, periodos: d.periodos }
    },
    staleTime: 60 * 60_000,
    refetchOnWindowFocus: false,
  })
}

// ─── FarolExecutivo ──────────────────────────────────────────────────────────

export default function FarolExecutivo() {
  // navigate / canImport / refreshing removidos junto com os botões
  // "Importar" e "Consolidar" — gestor pediu pra retirar do painel (usuários
  // clicavam sem querer). Ações administrativas seguem disponíveis no menu.
  const { spRole, tipoPersona } = useAuth()
  void spRole; void tipoPersona // mantidos pra futuras gates de UI

  const [view, setView] = useState<'V01' | 'V02' | 'V03' | 'V06' | 'V07'>('V01')
  const [fluxo, setFluxo] = useState<Fluxo>('faturado')
  // Toggles "Incluir X" (venda líquida). Vazio = Líquido puro (padrão).
  const [incluir, setIncluir] = useState<Set<CompKey>>(() => new Set())
  const [drillPath, setDrillPath] = useState<DrillStep[]>([])
  const [search, setSearch] = useState('')
  const [activePreset, setActivePreset] = useState<Preset | null>('ytd')

  const [refInicio, setRefInicio] = useState('')
  const [refFim, setRefFim] = useState('')
  const [compInicio, setCompInicio] = useState('')
  const [compFim, setCompFim] = useState('')

  const [filters, setFilters] = useState<Record<string, string[]>>({})
  const setFilter = (col: string, vals: string[]) => {
    setFilters(prev => {
      const next = { ...prev }
      let v = vals
      if (SINGLE_SELECT_COLS.has(col) && v.length > 1) {
        // Troca em vez de acumular: fica só a que o usuário ACABOU de clicar
        // (o toggle do MultiSelect anexa no fim, mas comparar com o estado
        // anterior é imune à ordem). Comportamento de rádio, sem trocar o
        // componente.
        const anterior = prev[col] ?? []
        const recemClicada = v.find(x => !anterior.includes(x))
        v = [recemClicada ?? v[v.length - 1]]
      }
      if (v.length === 0) delete next[col]
      else next[col] = v
      return next
    })
  }
  const clearFilters = () => setFilters({})

  const periodosQ = useUltimoPeriodo()
  useEffect(() => {
    if (refInicio || !periodosQ.data) return
    // Default ao entrar: "Ano × Ano" (ano anterior completo vs ano atual até hoje)
    const r = presetRange('ytd', { ano: periodosQ.data.ref_ano!, mes: periodosQ.data.ref_mes! })
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }, [periodosQ.data, refInicio])

  const applyPreset = (p: Preset) => {
    setActivePreset(p)
    // periodos vem DESC (mais recente primeiro); ref_ano/ref_mes pode ser 0 se
    // ainda sem dados no momento do fetch — usar periodos[0] como fonte primária.
    const ps = periodosQ.data?.periodos ?? []
    const latestStr = ps[0]
    const parsePeriodo = (s: string) => { const [y, m] = s.split('-'); return { ano: +y, mes: +m } }
    const last = latestStr
      ? parsePeriodo(latestStr)
      : (periodosQ.data?.ref_ano && periodosQ.data?.ref_mes)
        ? { ano: periodosQ.data.ref_ano, mes: periodosQ.data.ref_mes }
        : undefined
    const r = presetRange(p, last)
    setRefInicio(r.ref_inicio); setRefFim(r.ref_fim)
    setCompInicio(r.comp_inicio); setCompFim(r.comp_fim)
  }

  const { data, isLoading, error } = useCards({
    view, fluxo, ref_inicio: refInicio, ref_fim: refFim,
    comp_inicio: compInicio, comp_fim: compFim,
    drillPath, filters,
  })
  const dimsQ = useDims(fluxo, refInicio, refFim)
  // Lazy-load do dropdown de Cliente: só ativa após o usuário abri-lo uma vez.
  const [cliEnabled, setCliEnabled] = useState(false)
  const dimsCliQ = useDimsCli(fluxo, refInicio, refFim, cliEnabled)

  const handleDrill = (card: CardItem) => {
    if (card.level === 'cod_prod') return
    setDrillPath(prev => [...prev, { level: card.level, value: card.key, label: card.label }])
  }
  const handleBack = () => setDrillPath(prev => prev.slice(0, -1))
  const handleViewChange = (v: 'V01' | 'V02' | 'V03' | 'V06' | 'V07') => { setView(v); setDrillPath([]) }

  const kpi = data?.kpi
  const hasComp = !!data?.periodo?.comp_inicio
  const anyToggle = incluir.size > 0

  // Venda líquida: valor_atual/valor_ant já vêm como Líquido. Quando há toggle
  // ligado, soma os deltas da composição e RECALCULA pct/cor (semáforo segue a
  // tela). Sem toggle, usa o que o servidor mandou (Líquido puro).
  const cards = useMemo<CardItem[]>(() => {
    const raw = data?.cards ?? []
    if (!anyToggle) return raw
    return raw.map(c => {
      const va  = c.valor_atual + sumDelta(c.comp, incluir)
      const van = c.valor_ant   + sumDelta(c.comp_ant, incluir)
      const { pct, cor } = corFor(va, van, hasComp)
      return { ...c, valor_atual: va, valor_ant: van, pct, cor }
    })
  }, [data?.cards, incluir, anyToggle, hasComp])

  // Constrói o "card total" virtual a partir do KPI pra reaproveitar o componente DataRow
  const totalCard: CardItem | null = kpi ? (() => {
    const va  = kpi.total_atual + sumDelta(kpi.comp, incluir)
    const van = kpi.total_ant   + sumDelta(kpi.comp_ant, incluir)
    const { pct, cor } = anyToggle ? corFor(va, van, hasComp) : { pct: kpi.total_pct, cor: kpi.total_cor }
    return {
      key: '__total__', label: 'TOTAL',
      level: '', level_label: '',
      valor_atual: va, valor_ant: van, pct, cor,
      plucro: kpi.total_plucro, plucro_ant: kpi.total_plucro_ant,
      positivados: kpi.total_positivados, base_cli: kpi.total_base_cli,
      positpct: kpi.total_positpct,
      positivados_ant: kpi.total_positivados_ant, base_cli_ant: kpi.total_base_cli_ant,
      positpct_ant: kpi.total_positpct_ant,
      posit_cor: kpi.total_posit_cor,
      mix: kpi.avg_mix, mix_ant: kpi.avg_mix_ant, mix_cor: kpi.mix_cor,
      mix_total: kpi.total_mix_total, mix_total_ant: kpi.total_mix_total_ant,
    }
  })() : null

  // Filtro por busca textual ANTES da ordenação — a ordenação é a última etapa.
  const filteredCards = useMemo(() => {
    const s = search.trim().toLowerCase()
    if (!s) return cards
    return cards.filter(c => c.label.toLowerCase().includes(s))
  }, [cards, search])

  // Ordenação via setinhas no header do GRID — clique em "Período Atual" ou "%"
  // (ver ColumnsHeader). Default = valor desc. Preferência persistida.
  const { sorted: visibleCards, sortState, setSort } =
    useSortedCards(filteredCards, 'farol.sort.executivo', { field: 'valor', direction: 'desc' })

  // Nível atual sendo listado (cards) → esconde Positivação em Cliente/Produto.
  const curLevel = cards[0]?.level ?? ''
  // Fluxos CCD (Cancelado/Devolvido, Cortado) — só valor do evento, sem
  // positivação nem toggles de composição.
  const isCCD = fluxo === 'cancdev' || fluxo === 'cortado'
  // Positivação escondida nas folhas cliente/produto, nas views V06/V07
  // (Fase 2) e nos fluxos CCD.
  const hidePosit = isCCD || view === 'V06' || view === 'V07' ||
                    curLevel === 'cod_cli' || curLevel === 'cod_prod'

  // handleRefreshViews removido junto com o botão Consolidar.

  const FILTER_DIMS: { col: string; label: string; from: keyof DimsResponse }[] = [
    { col: 'cod_fornec',     label: 'Indústria',  from: 'fornec' },
    { col: 'cod_gerente',    label: 'Gerente',    from: 'gerente' },
    { col: 'cod_supervisor', label: 'Supervisor', from: 'supervisor' },
    { col: 'cod_rca',        label: 'RCA',        from: 'rca' },
    { col: 'cod_cli',        label: 'Cliente',    from: 'cli' },
    { col: 'uf',             label: 'UF',         from: 'uf' },
    // Filial — RESTAURADO em 06/08/2026. Foi removido em 27/07 sob a premissa
    // de que `empresa` era "texto livre do CSV"; a premissa estava errada. Os
    // valores são os códigos de filial do WinThor (1, 11, 12, 13, 15, 18, 20,
    // 28, 32, 33), os mesmos que a Rotina 1464 filtra. O nome da coluna vem do
    // layout do ION VENDAS e é infeliz — por isso só o RÓTULO vira "Filial";
    // a coluna e o query param seguem `empresa` (compatibilidade de dados).
    { col: 'empresa',        label: 'Filial',     from: 'empresa' },
    // Tipo de Venda: faturado (mig 187) e transmitido (mig 203, pedido em
    // reunião 17/08/2026). Fica de fora de cancel./devol. e cortado, onde o
    // próprio fluxo já é o recorte por evento.
    ...(fluxo === 'faturado' || fluxo === 'transmitido'
      ? [{ col: 'tipo_venda', label: 'Tipo de Venda', from: 'tipo_venda' as const }]
      : []),
  ]

  const optionsFor = (from: keyof DimsResponse): { key: string; label: string }[] => {
    // Cliente vem do hook lazy (dimsCliQ); as demais do dims padrão.
    if (from === 'cli') return dimsCliQ.data?.cli ?? []
    const v = dimsQ.data?.[from]
    if (!v) return []
    if (Array.isArray(v) && typeof v[0] === 'string') {
      return (v as string[]).map(s => ({ key: s, label: s }))
    }
    return v as DimOption[]
  }

  const totalFiltersActive = Object.values(filters).reduce((n, vs) => n + vs.length, 0)

  return (
    <div className="min-h-full p-4 md:p-6 bg-slate-50 uppercase text-sm [&_*]:uppercase">

      {/* ── Seletor de FLUXO (acima de tudo) ────────────────────────────────── */}
      <div className="mb-3">
        <div className="inline-flex rounded-md border-2 border-slate-300 overflow-hidden bg-white shadow-sm">
          {([
            { id: 'faturado'    as const, label: 'Faturado',    color: 'bg-[#1e293b]' },
            { id: 'transmitido' as const, label: 'Transmitido', color: 'bg-emerald-700' },
            { id: 'cancdev'     as const, label: 'Cancel./Devol.', color: 'bg-rose-700' },
            { id: 'cortado'     as const, label: 'Cortado',     color: 'bg-amber-700' },
          ]).map(f => (
            <button
              key={f.id}
              onClick={() => {
                setFluxo(f.id); setDrillPath([]); setIncluir(new Set())
                // Limpa Tipo de Venda ao ir para um fluxo que não o suporta —
                // senão o filtro ficaria ativo e invisível, com a tela mostrando
                // um recorte que o usuário não vê mais na barra.
                if (f.id !== 'faturado' && f.id !== 'transmitido') setFilter('tipo_venda', [])
              }}
              className={cn(
                'px-5 py-2 text-sm font-bold uppercase tracking-wide transition-colors',
                fluxo === f.id ? cn(f.color, 'text-white') : 'text-slate-600 hover:bg-slate-50',
              )}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      {/* ── Atalhos de período ──────────────────────────────────────────────── */}
      <div className="flex items-center gap-1.5 mb-1.5 flex-wrap">
        <span className="text-sm uppercase tracking-wider text-slate-400 font-semibold flex items-center gap-1 mr-0.5">
          <Calendar className="h-3 w-3" />
        </span>
        {([
          { id: 'ytd'          as const, tip: 'Ano anterior INTEIRO (Jan-Dez) × Jan até hoje do ano atual' },
          { id: 'yoy'          as const, tip: 'Último mês 100% importado × Mesmo mês do ano anterior (ambos completos)' },
          { id: 'ant_corrente' as const, tip: 'Dois últimos meses completos carregados (M-1 vs M-2)' },
          { id: 'mes_corrente' as const, tip: 'Dia 1 até hoje do mês corrente × mesmo período do ano anterior' },
          { id: 'dia_anterior' as const, tip: 'Ontem × mesmo dia da semana 7 dias antes (evita falso alarme de fim de semana)' },
          { id: 'last7'        as const, tip: 'Últimos 7 dias × 7 dias anteriores' },
          { id: 'last30'       as const, tip: 'Últimos 30 dias × 30 dias anteriores' },
        ]).map(p => (
          <button
            key={p.id}
            onClick={() => applyPreset(p.id)}
            title={p.tip}
            className={cn(
              'px-2.5 py-1 text-sm font-semibold rounded transition border',
              activePreset === p.id
                ? 'bg-slate-700 text-white border-slate-700 shadow-sm'
                : 'bg-white text-slate-600 border-slate-300 hover:bg-slate-50 hover:border-slate-400',
            )}
          >
            {PRESET_LABEL[p.id]}
          </button>
        ))}
      </div>

      {/* ── Strip de período ativo ───────────────────────────────────────────── */}
      {compInicio && compFim && refInicio && refFim && (
        <div className="flex items-center gap-2 mb-2 px-3 py-1.5 bg-white border border-slate-200 rounded-md text-sm shadow-sm flex-wrap">
          {activePreset && (
            <span className="font-bold text-slate-700 mr-1">{PRESET_LABEL[activePreset]}:</span>
          )}
          <span className="text-slate-400 font-medium">Anterior</span>
          <span className="font-semibold text-orange-600">
            {fmtDateBR(compInicio)} → {fmtDateBR(compFim)}
          </span>
          <span className="text-slate-300">({rangeDaysInclusive(compInicio, compFim)} dias)</span>
          <span className="text-slate-400 font-bold mx-1">vs</span>
          <span className="text-slate-400 font-medium">Atual</span>
          <span className="font-semibold text-orange-700">
            {fmtDateBR(refInicio)} → {fmtDateBR(refFim)}
          </span>
          <span className="text-slate-300">({rangeDaysInclusive(refInicio, refFim)} dias)</span>
        </div>
      )}

      {/* ── Controles secundários ───────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-3">
        <div className="flex rounded-md border border-slate-300 overflow-hidden bg-white shadow-sm">
          {([
            { id: 'V01' as const, label: 'Por Indústria' },
            { id: 'V03' as const, label: 'Por Gerência' },
            { id: 'V02' as const, label: 'Por Equipe' },
            { id: 'V06' as const, label: 'Por Rede' },
            { id: 'V07' as const, label: 'Por Departamento' },
          ]).map(v => (
            <button
              key={v.id}
              onClick={() => handleViewChange(v.id)}
              className={cn(
                'px-3 py-1.5 text-sm font-medium transition-colors',
                view === v.id ? cn(BTN_PRIMARY_BG, 'text-white') : 'text-slate-600 hover:bg-slate-50',
              )}
            >
              {v.label}
            </button>
          ))}
        </div>

        {FILTER_DIMS.map(d => {
          const opts = optionsFor(d.from)
          // Filtro com uma opção só não filtra nada — é o próprio escopo do
          // usuário aparecendo como se fosse escolha. Para o GGV, "Gerente"
          // listaria apenas ele mesmo (o servidor já recorta as dims). Some,
          // em vez de ocupar espaço prometendo uma decisão que não existe.
          //
          // Exceção: a dim de Cliente carrega sob demanda (lazy) e começa
          // vazia; escondê-la por estar "vazia" a tiraria da tela para todo
          // mundo, para sempre.
          if (d.from !== 'cli' && opts.length <= 1 && (filters[d.col] ?? []).length === 0) {
            return null
          }
          return (
            <MultiSelect
              key={d.col}
              label={d.label}
              options={opts}
              selected={filters[d.col] ?? []}
              onChange={(vs) => setFilter(d.col, vs)}
              onOpen={d.from === 'cli' ? () => setCliEnabled(true) : undefined}
              loading={d.from === 'cli' && cliEnabled && dimsCliQ.isLoading}
              single={SINGLE_SELECT_COLS.has(d.col)}
            />
          )
        })}

        <DateRangeFilter
          label="Período Anterior"
          inicio={compInicio}
          fim={compFim}
          onChangeInicio={v => { setCompInicio(v); setActivePreset(null) }}
          onChangeFim={v => { setCompFim(v); setActivePreset(null) }}
          borderColor="border-orange-400"
        />
        <DateRangeFilter
          label="Período Atual"
          inicio={refInicio}
          fim={refFim}
          onChangeInicio={v => { setRefInicio(v); setActivePreset(null) }}
          onChangeFim={v => { setRefFim(v); setActivePreset(null) }}
          borderColor="border-orange-500"
        />

        {totalFiltersActive > 0 && (
          <button
            onClick={clearFilters}
            className="inline-flex items-center gap-1 px-2 py-1.5 text-sm font-medium text-red-600 hover:bg-red-50 rounded-md"
          >
            <X className="h-3 w-3" /> Limpar filtros ({totalFiltersActive})
          </button>
        )}

        {/* Busca foi movida pra DENTRO do header do GRID (coluna Nome) —
            antes ficava perdida no fim da toolbar entre os filtros. */}

        {/* Botões Importar e Consolidar removidos a pedido do gestor —
            usuários estavam clicando sem querer. Ações de import/consolidação
            ficam restritas ao menu de administração (não neste painel). */}
      </div>

      {/* ── Composição do faturado: Líquido (padrão) + botões "Incluir X" ─────
          Só no fluxo faturado e sem filtro de Tipo de Venda ativo (esse filtro
          isola um tipo, tornando os toggles redundantes). Recálculo é client-side
          (venda líquida Fase 3). */}
      {fluxo === 'faturado' && !(filters['tipo_venda']?.length) && (
        <div className="flex flex-wrap items-center gap-1.5 mb-3">
          <span className="text-sm uppercase tracking-wider text-slate-500 font-semibold mr-1">
            Faturado:{' '}
            <span className="font-bold text-slate-700">
              {incluir.size === 0 ? 'Líquido' : 'Líquido + incluídos'}
            </span>
          </span>
          {INCLUIR_CATS.map(cat => {
            const on = incluir.has(cat.key)
            return (
              <button
                key={cat.key}
                onClick={() => setIncluir(prev => {
                  const n = new Set(prev)
                  if (n.has(cat.key)) n.delete(cat.key); else n.add(cat.key)
                  return n
                })}
                title={on ? `Remover ${cat.label} do total` : `Incluir ${cat.label} no total`}
                className={cn(
                  'px-2.5 py-1 text-xs font-semibold uppercase tracking-wide rounded-md border transition-colors',
                  on ? 'bg-slate-700 text-white border-slate-700'
                     : 'bg-white text-slate-600 border-slate-300 hover:bg-slate-50',
                )}
              >
                {on ? '✓ ' : '+ '}{cat.label}
              </button>
            )
          })}
          {incluir.size > 0 && (
            <button
              onClick={() => setIncluir(new Set())}
              className="inline-flex items-center gap-1 px-2 py-1 text-xs font-medium text-red-600 hover:bg-red-50 rounded-md"
            >
              <X className="h-3 w-3" /> Voltar ao Líquido
            </button>
          )}
        </div>
      )}

      {/* ── Chips dos filtros ativos ────────────────────────────────────────── */}
      {totalFiltersActive > 0 && (
        <div className="flex flex-wrap items-center gap-1 mb-3">
          <span className="text-sm uppercase tracking-wider text-slate-500 font-semibold mr-1">
            <Filter className="h-3 w-3 inline -mt-0.5" /> Filtros ativos:
          </span>
          {FILTER_DIMS.flatMap(d => {
            const vals = filters[d.col] ?? []
            const opts = optionsFor(d.from)
            return vals.map(v => {
              const label = opts.find(o => o.key === v)?.label ?? v
              return (
                <span key={`${d.col}:${v}`} className="inline-flex items-center gap-1 px-2 py-0.5 text-sm bg-slate-100 border border-slate-200 rounded-full">
                  <span className="text-slate-500">{d.label}:</span>
                  <span className="font-medium text-slate-800">{label}</span>
                  <button onClick={() => setFilter(d.col, vals.filter(x => x !== v))} className="ml-0.5 text-slate-400 hover:text-red-600">
                    <X className="h-2.5 w-2.5" />
                  </button>
                </span>
              )
            })
          })}
        </div>
      )}

      {/* ── Breadcrumb de drill ─────────────────────────────────────────────── */}
      {drillPath.length > 0 && (
        <div className="flex items-center gap-1 mb-3 text-sm text-slate-600">
          <button onClick={() => setDrillPath([])} className="hover:text-slate-900 hover:underline">
            Início
          </button>
          {drillPath.map((d, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="text-slate-400">›</span>
              <button onClick={() => setDrillPath(drillPath.slice(0, i + 1))} className="hover:text-slate-900 hover:underline truncate max-w-[200px]" title={d.label}>
                {d.label}
              </button>
            </span>
          ))}
          <button onClick={handleBack} className="ml-2 inline-flex items-center gap-1 text-slate-500 hover:text-slate-900">
            <ChevronLeft className="h-3 w-3" />
            Voltar
          </button>
        </div>
      )}

      {/* ── Legenda do período: a quem "Anterior"/"Atual" se referem ────────── */}
      {data?.periodo?.label && (
        <div className="mb-2 inline-flex items-center gap-1.5 text-sm font-semibold text-slate-600 bg-white border border-slate-200 rounded-md px-3 py-1.5 shadow-sm">
          <Calendar className="h-3.5 w-3.5 text-slate-400" />
          {data.periodo.label}
        </div>
      )}

      {/* ── TOTAL (card único com cabeçalho próprio — header não interativo) ── */}
      {totalCard && (
        <div className="bg-white border border-slate-200 rounded-lg overflow-hidden mb-4 shadow-sm">
          <ColumnsHeader hidePosit={hidePosit} />
          <DataRow card={totalCard} isTotal hidePosit={hidePosit} />
        </div>
      )}

      {/* ── Banner do nível atual — destaca QUE tipo de dado está listado ── */}
      {data && data.next_level_label && (
        <div className="bg-gradient-to-r from-indigo-50 via-sky-50 to-white border border-sky-200 rounded-lg px-4 py-2.5 mb-4 flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <span className="inline-flex items-center px-2.5 py-1 rounded-md bg-sky-600 text-white text-sm font-bold uppercase tracking-wider shadow-sm">
              {data.next_level_label}
            </span>
          </div>
          <span className="text-sm text-slate-500 tabular-nums shrink-0">
            {visibleCards.length} {visibleCards.length === 1 ? 'item' : 'itens'}
          </span>
        </div>
      )}

      {/* ── AVISOS de diagnóstico do recorte ──────────────────────────────────
          O backend informa (campo `diag`) quando a consulta FALHOU ou quando
          precisou varrer a base bruta. Sem isso a tela era ambígua: consulta
          morta e "não teve venda" apareciam idênticas — 0 cards silenciosos
          (incidente 27/07/2026). Ver cardsDiag em farol_v2_api.go. */}
      {data?.diag?.falhou && (
        <div className="border border-red-300 bg-red-50 rounded-lg px-4 py-3 mb-4 flex items-start gap-3">
          <span className="text-red-600 text-lg leading-none mt-0.5" aria-hidden>⚠</span>
          <div className="text-sm text-red-800">
            <strong className="font-semibold">Não foi possível calcular este recorte.</strong>{' '}
            Os números abaixo estão incompletos ou ausentes —{' '}
            <strong className="font-semibold">não</strong> interprete como ausência de venda.
            Tente de novo em alguns instantes; se continuar, avise o suporte.
          </div>
        </div>
      )}
      {data?.diag?.lento && !data?.diag?.falhou && (
        <div className="border border-amber-300 bg-amber-50 rounded-lg px-4 py-3 mb-4 flex items-start gap-3">
          <span className="text-amber-600 text-lg leading-none mt-0.5" aria-hidden>⏱</span>
          <div className="text-sm text-amber-900">
            <strong className="font-semibold">Esta combinação de filtros é lenta.</strong>{' '}
            {(() => {
              const cols = (data.diag?.combinacao ?? '').split(',').filter(Boolean)
              const nomes = cols.map(c => FILTER_DIMS.find(d => d.col === c)?.label ?? c)
              const seg = Math.round((data.diag?.ms ?? 0) / 1000)
              return (
                <>
                  Cruzar {nomes.length > 1 ? <strong>{nomes.join(' + ')}</strong> : <strong>{nomes[0]}</strong>}{' '}
                  não tem atalho pré-calculado, então a consulta precisa varrer toda a base
                  {seg > 0 && <> — levou {seg}s</>}. Os números estão corretos. Para resposta
                  imediata, use um filtro de cada vez.
                </>
              )
            })()}
          </div>
        </div>
      )}

      {/* ── LISTA de fornecedores/GGV/equipe ─────────────────────────────────── */}
      <div className="bg-white border border-slate-200 rounded-lg overflow-hidden shadow-sm">
        {/* Cabeçalho da lista — interativo: setinhas pra ordenar + busca embutida */}
        <ColumnsHeader
          hidePosit={hidePosit}
          sortState={sortState}
          onSortChange={setSort}
          search={search}
          onSearchChange={setSearch}
          searchPlaceholder={`Buscar ${visibleCards[0]?.level_label?.toLowerCase() ?? ''}...`}
        />

        {/* Linhas */}
        {(isLoading || !refInicio) && (
          <LoadingState message="Carregando dados, aguarde..." hint="Primeiro acesso do dia pode levar alguns segundos." cards={0} />
        )}
        {error != null && (
          <div className="text-center text-sm text-red-600 py-8">
            Erro ao carregar. {(error as Error).message}
          </div>
        )}
        {!isLoading && refInicio && error == null && visibleCards.length === 0 && (
          <div className="text-center text-sm py-8">
            {/* Lista vazia por FALHA nunca pode ler como "não vendeu nada". */}
            {data?.diag?.falhou ? (
              <span className="text-red-700">
                A consulta falhou — este recorte não pôde ser calculado.
                <br />
                <span className="text-slate-500">Isto não significa que não houve venda.</span>
              </span>
            ) : (
              <span className="text-slate-500">
                {search ? 'Nenhum resultado para a busca.' : 'Sem dados para o filtro atual.'}
              </span>
            )}
          </div>
        )}
        {visibleCards.map(c => (
          <DataRow
            key={c.key}
            card={c}
            hidePosit={hidePosit}
            onClick={c.level === 'cod_prod' ? undefined : () => handleDrill(c)}
          />
        ))}
      </div>
    </div>
  )
}
