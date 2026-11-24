import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronDown, Target } from 'lucide-react'
import { ChipRow, StatusIcon } from '@/components/farol/mobileUi'

// Segmento Foco (Reckitt) no mobile — link do ION VENDAS, sem login. Pedido do Heverton
// (24/11/2026): mesmo desenho do mobile da Unilever. Decisões do Claudio: só contagem de clientes
// (sem objetivo/rateio por equipe ou RCA), escopos sup, rca e ggv.
//   Supervisor: contagem da equipe + RCAs → segmentos → clientes → produtos
//   GGV:        contagem do GGV + equipes → RCAs → segmentos → clientes → produtos
//   RCA:        contagem do RCA + "Seus clientes" (positivou/não por segmento) → produtos

type Scope = 'sup' | 'rca' | 'ggv'
interface MesValor { mes: string; realizado: number }
interface GrupoLinha { grupo: string; meses: MesValor[]; realizado_tri: number }
interface Painel { meses: string[]; meses_disponiveis?: string[]; meses_sem_produtos?: string[]; meses_sem_vigencia?: string[]; trimestre: string; grupos: string[]; ufs: { uf: string; grupos: GrupoLinha[] }[] }
interface DetLinha { cod: string; nome: string; cod_pai?: string; nome_pai?: string; grupos: number[]; total: number; geral: number }
interface DetProduto { grupo: string; cod_prod: string; produto: string; qtd: number; positivou: boolean }
interface Detalhe { nivel: string; grupos: string[]; linhas?: DetLinha[]; produtos?: DetProduto[] }

const FLUXOS = [
  { value: 'faturado', label: 'Faturado' },
  { value: 'transmitido', label: 'Transmitido' },
  { value: 'soma', label: 'Faturado + Transmitido' },
]
const MESES = ['JAN', 'FEV', 'MAR', 'ABR', 'MAI', 'JUN', 'JUL', 'AGO', 'SET', 'OUT', 'NOV', 'DEZ']
const fmt = (n: number) => n.toLocaleString('pt-BR', { maximumFractionDigits: 0 })
const mesAtual = () => new Date().toISOString().slice(0, 7)
// Primeiro mês (AAAA-MM) do trimestre civil que contém `m`.
const iniTri = (m: string) => `${m.slice(0, 4)}-${String(Math.floor((Number(m.slice(5, 7)) - 1) / 3) * 3 + 1).padStart(2, '0')}`
const rotuloTri = (m: string) => `${Math.floor((Number(m.slice(5, 7)) - 1) / 3) + 1}º tri/${m.slice(2, 4)}`
const somaMeses = (m: string, d: number) => { const t = new Date(Number(m.slice(0, 4)), Number(m.slice(5, 7)) - 1 + d, 1); return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}` }
const NIVEL_ROTULO: Record<string, string> = { crv: 'Equipe', rca: 'RCA', cliente: 'Cliente' }

interface Ctx { cnpj: string; scope: Scope; cod: string; vinculoId: number; mes: string; fluxo: string; periodo: 'mes' | 'trimestre' }

async function buscar<T>(rota: string, ctx: Ctx, extra: Record<string, string> = {}): Promise<T> {
  const p = new URLSearchParams({
    cnpj: ctx.cnpj, scope: ctx.scope, cod: ctx.cod, vinculo_id: String(ctx.vinculoId), mes: ctx.mes, fluxo: ctx.fluxo, ...extra,
  })
  if (rota === 'detalhe') p.set('periodo', ctx.periodo)
  const r = await fetch(`/api/farol/public/segmento-foco/${rota}?${p}`)
  if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error ?? 'Erro ao carregar')
  return r.json()
}

function useDetalhe(ctx: Ctx, extra: Record<string, string>, enabled = true) {
  return useQuery<Detalhe>({
    queryKey: ['public-seg-detalhe', ctx, extra],
    queryFn: () => buscar<Detalhe>('detalhe', ctx, extra),
    enabled,
  })
}

// Produtos do cliente (último nível).
function ProdutosDoCliente({ ctx, extra }: { ctx: Ctx; extra: Record<string, string> }) {
  const { data, isLoading } = useDetalhe(ctx, extra)
  if (isLoading) return <div className="px-3 py-2 text-xs text-muted-foreground">Carregando...</div>
  const itens = data?.produtos ?? []
  if (itens.length === 0) return <div className="px-3 py-2 text-xs text-muted-foreground">Nenhum produto dos segmentos neste período</div>
  return (
    <div className="bg-slate-50 border-t">
      {itens.map(p => (
        <div key={`${p.grupo}|${p.cod_prod}`} className="flex items-center gap-2 px-3 py-1.5 border-b last:border-0 text-xs">
          <StatusIcon atingiu={p.positivou} />
          <div className="min-w-0 flex-1">
            <div className="truncate">{p.produto}</div>
            <div className="text-[10px] text-muted-foreground">{p.grupo} · {fmt(p.qtd)} un.</div>
          </div>
        </div>
      ))}
    </div>
  )
}

// 1 cliente: nome + (opcional) selos dos segmentos; abre os produtos.
function LinhaCliente({ ctx, linha, grupos, extra, mostrarSelos }: { ctx: Ctx; linha: DetLinha; grupos: string[]; extra: Record<string, string>; mostrarSelos: boolean }) {
  const [aberto, setAberto] = useState(false)
  return (
    <div className="border-b last:border-0">
      <button type="button" onClick={() => setAberto(a => !a)} className="w-full text-left px-3 py-2.5 active:bg-slate-50">
        <div className="flex items-center gap-2">
          <ChevronDown className={`w-4 h-4 shrink-0 text-slate-400 transition-transform ${aberto ? '' : '-rotate-90'}`} />
          <span className="text-sm font-medium flex-1 min-w-0 truncate">{linha.cod} — {linha.nome}</span>
          {mostrarSelos && <span className="text-xs font-semibold shrink-0">{linha.grupos.filter(n => n > 0).length}/{grupos.length}</span>}
        </div>
        {mostrarSelos && (
          <div className="flex flex-wrap gap-1 mt-1.5 pl-6">
            {grupos.map((g, i) => (
              <span key={g} className={`text-[10px] rounded px-1.5 py-0.5 border ${linha.grupos[i] > 0 ? 'bg-green-50 border-green-300 text-green-800 font-semibold' : 'bg-white border-slate-200 text-slate-300'}`}>{g}</span>
            ))}
          </div>
        )}
      </button>
      {aberto && <ProdutosDoCliente ctx={ctx} extra={{ ...extra, cod_cli: linha.cod }} />}
    </div>
  )
}

// RCA (supervisor/GGV): abre os segmentos; o segmento abre os clientes que positivaram nele.
function BlocoRCA({ ctx, linha, grupos, extra, recuo }: { ctx: Ctx; linha: DetLinha; grupos: string[]; extra: Record<string, string>; recuo?: boolean }) {
  const [aberto, setAberto] = useState(false)
  const [grupoAberto, setGrupoAberto] = useState<number | null>(null)
  const filtro = { ...extra, cod_rca: linha.cod }
  const { data, isLoading } = useDetalhe(ctx, filtro, grupoAberto !== null)
  return (
    <div className="border-b last:border-0">
      <button type="button" onClick={() => setAberto(a => !a)} className={`w-full text-left py-2.5 pr-3 active:bg-slate-50 flex items-center gap-2 ${recuo ? 'pl-6' : 'pl-3'}`}>
        <ChevronDown className={`w-4 h-4 shrink-0 text-slate-400 transition-transform ${aberto ? '' : '-rotate-90'}`} />
        <span className="text-sm font-medium flex-1 min-w-0 truncate">{linha.cod} — {linha.nome}</span>
        <span className="text-sm font-bold shrink-0">{fmt(linha.geral)}</span>
      </button>
      {aberto && (
        <div className="bg-slate-50 border-t">
          {grupos.map((g, i) => {
            const n = linha.grupos[i] ?? 0
            const ab = grupoAberto === i
            return (
              <div key={g} className="border-b last:border-0">
                <button type="button" disabled={n === 0} onClick={() => setGrupoAberto(ab ? null : i)} className="w-full text-left pl-9 pr-3 py-2 flex items-center gap-2 disabled:opacity-60">
                  <ChevronDown className={`w-3.5 h-3.5 shrink-0 text-slate-400 transition-transform ${ab ? '' : '-rotate-90'}`} />
                  <span className="text-xs flex-1">{g}</span>
                  <span className={`text-xs font-semibold ${n ? '' : 'text-slate-300'}`}>{fmt(n)} cliente(s)</span>
                </button>
                {ab && isLoading && <div className="pl-12 py-2 text-xs text-muted-foreground">Carregando...</div>}
                {ab && data && (
                  <div className="bg-white border-t">
                    {(data.linhas ?? []).filter(c => (c.grupos[i] ?? 0) > 0).map(c => (
                      <LinhaCliente key={c.cod} ctx={ctx} linha={c} grupos={grupos} extra={filtro} mostrarSelos={false} />
                    ))}
                    {(data.linhas ?? []).filter(c => (c.grupos[i] ?? 0) > 0).length === 0 && <div className="pl-12 py-2 text-xs text-muted-foreground">Nenhum cliente neste segmento</div>}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

// Equipe (GGV): abre os RCAs dela.
function BlocoEquipe({ ctx, linha, grupos }: { ctx: Ctx; linha: DetLinha; grupos: string[] }) {
  const [aberto, setAberto] = useState(false)
  const filtro = { cod_crv: linha.cod }
  const { data, isLoading } = useDetalhe(ctx, filtro, aberto)
  return (
    <div className="border-b last:border-0">
      <button type="button" onClick={() => setAberto(a => !a)} className="w-full text-left px-3 py-2.5 active:bg-slate-50 flex items-center gap-2">
        <ChevronDown className={`w-4 h-4 shrink-0 text-slate-400 transition-transform ${aberto ? '' : '-rotate-90'}`} />
        <span className="text-sm font-medium flex-1 min-w-0 truncate">{linha.cod} — {linha.nome}</span>
        <span className="text-sm font-bold shrink-0">{fmt(linha.geral)}</span>
      </button>
      {aberto && isLoading && <div className="pl-9 py-2 text-xs text-muted-foreground">Carregando...</div>}
      {aberto && data && (data.linhas ?? []).map(r => <BlocoRCA key={r.cod} ctx={ctx} linha={r} grupos={grupos} extra={filtro} recuo />)}
    </div>
  )
}

export default function SegmentoFocoMobile({ cnpj, scope, cod, vinculoId, industriaNome }: { cnpj: string; scope: Scope; cod: string; vinculoId: number; industriaNome: string }) {
  const [mes, setMes] = useState(mesAtual)
  const [periodo, setPeriodo] = useState<'mes' | 'trimestre'>('mes')
  const [fluxo, setFluxo] = useState('faturado')
  const [ajustou, setAjustou] = useState(false)
  // RCA: filtro de segmento e situação sobre "Seus clientes"
  const [segFiltro, setSegFiltro] = useState('todos')
  const [situacao, setSituacao] = useState<'sim' | 'falta'>('sim')
  const ctx: Ctx = { cnpj, scope, cod, vinculoId, mes, fluxo, periodo }

  const { data: painel, isLoading, error } = useQuery<Painel>({
    queryKey: ['public-seg-painel', cnpj, scope, cod, vinculoId, mes, fluxo],
    queryFn: () => buscar<Painel>('painel', ctx),
  })
  // Abre no último mês com vigência carregada (o corrente pode não ter carga ainda).
  useEffect(() => {
    if (!painel || ajustou) return
    setAjustou(true)
    const disp = painel.meses_disponiveis ?? []
    if (disp.length && !disp.includes(mes)) setMes(disp[disp.length - 1])
  }, [painel, ajustou, mes])

  // Trimestres selecionáveis: do primeiro com carga (ou o corrente) até o corrente — dá pra voltar a
  // qualquer trimestre e ir até o corrente mesmo sem carga (a tela avisa).
  const trimestres = useMemo(() => {
    const disp = painel?.meses_disponiveis ?? []
    const fim = iniTri(mesAtual())
    let t = iniTri(disp.length ? disp[0] : fim)
    const out: string[] = []
    while (t <= fim && out.length < 8) { out.push(t); t = somaMeses(t, 3) }
    if (!out.includes(iniTri(mes))) out.push(iniTri(mes))
    return out.sort()
  }, [painel, mes])
  const irParaTrimestre = (ini: string) => {
    const disp = (painel?.meses_disponiveis ?? []).filter(m => iniTri(m) === ini)
    setPeriodo('mes')
    setMes(disp.length ? disp[disp.length - 1] : ini)
  }
  const semProdutos = painel
    ? (periodo === 'mes' ? [mes] : painel.meses).filter(m => (painel.meses_sem_produtos ?? []).includes(m) || (painel.meses_sem_vigencia ?? []).includes(m))
    : []

  const idxMes = painel ? Math.max(0, painel.meses.indexOf(mes)) : 0
  const totais = useMemo(() => (painel?.grupos ?? []).map(g => {
    let n = 0
    for (const u of painel!.ufs) {
      const l = u.grupos.find(x => x.grupo === g)
      if (l) n += periodo === 'mes' ? (l.meses[idxMes]?.realizado ?? 0) : l.realizado_tri
    }
    return { grupo: g, n }
  }), [painel, periodo, idxMes])

  const { data: base, isLoading: carregandoBase } = useDetalhe(ctx, {}, !!painel)
  const grupos = base?.grupos ?? painel?.grupos ?? []
  const periodoRotulo = periodo === 'mes' ? `${MESES[Number(mes.slice(5, 7)) - 1]}/${mes.slice(2, 4)}` : `Trimestre ${painel?.trimestre ?? ''}`

  const idxSeg = segFiltro === 'todos' ? -1 : grupos.indexOf(segFiltro)
  const clientesRCA = useMemo(() => {
    const l = base?.nivel === 'cliente' ? (base.linhas ?? []) : []
    const f = idxSeg < 0 ? l : l.filter(c => (situacao === 'sim') === ((c.grupos[idxSeg] ?? 0) > 0))
    const qt = (c: DetLinha) => c.grupos.filter(n => n > 0).length
    return [...f].sort((a, b) => qt(b) - qt(a) || a.nome.localeCompare(b.nome, 'pt-BR'))
  }, [base, idxSeg, situacao])

  return (
    <div className="space-y-4">
      <div className="space-y-3">
        <ChipRow
          label="Trimestre"
          options={trimestres.map(t => ({ value: t, label: rotuloTri(t) }))}
          value={iniTri(mes)}
          onChange={irParaTrimestre}
        />
        <ChipRow
          label="Período"
          sempre
          options={[...(painel?.meses ?? [mes]).map(m => ({ value: m, label: MESES[Number(m.slice(5, 7)) - 1] })), { value: 'tri', label: 'Trimestre' }]}
          value={periodo === 'trimestre' ? 'tri' : mes}
          onChange={v => { if (v === 'tri') setPeriodo('trimestre'); else { setPeriodo('mes'); setMes(v) } }}
        />
        <ChipRow label="Visão" options={FLUXOS} value={fluxo} onChange={setFluxo} />
      </div>

      {isLoading && <p className="text-center text-sm text-muted-foreground py-8">Carregando...</p>}
      {error && <p className="text-center text-sm text-red-700 py-4">{(error as Error).message}</p>}

      {painel && (
        <div className="space-y-3">
          <div className="text-xs text-muted-foreground flex items-center gap-1"><Target className="w-3.5 h-3.5" /> {industriaNome} — clientes positivados por segmento · {periodoRotulo}</div>
          {semProdutos.length > 0 && (
            <div className="text-xs border border-amber-300 bg-amber-50 rounded-lg p-3 text-amber-900">
              Sem carga da lista de produtos dos segmentos em {semProdutos.map(m => MESES[Number(m.slice(5, 7)) - 1]).join(', ')}.
              Os números deste período aparecem quando a lista do trimestre for carregada.
            </div>
          )}
          <div className="grid grid-cols-2 gap-2">
            {totais.map(t => (
              <div key={t.grupo} className="bg-white border rounded-xl p-3">
                <div className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground leading-tight min-h-[2rem]">{t.grupo}</div>
                <div className="text-2xl font-bold">{fmt(t.n)}</div>
              </div>
            ))}
          </div>

          {scope === 'rca' ? (
            <div className="space-y-2">
              <ChipRow
                label="Segmento"
                options={[{ value: 'todos', label: 'Todos' }, ...grupos.map(g => ({ value: g, label: g }))]}
                value={segFiltro}
                onChange={setSegFiltro}
              />
              {segFiltro !== 'todos' && (
                <ChipRow
                  label="Situação"
                  sempre
                  options={[{ value: 'sim', label: 'Positivaram' }, { value: 'falta', label: 'Faltam positivar' }]}
                  value={situacao}
                  onChange={v => setSituacao(v as 'sim' | 'falta')}
                />
              )}
              <h2 className="text-sm font-semibold px-0.5 pt-1">Seus clientes</h2>
              <div className="bg-white border rounded-xl overflow-hidden">
                {carregandoBase && <div className="px-3 py-4 text-sm text-muted-foreground text-center">Carregando...</div>}
                {!carregandoBase && clientesRCA.length === 0 && <div className="px-3 py-4 text-sm text-muted-foreground text-center">Nenhum cliente neste recorte</div>}
                {clientesRCA.map(c => <LinhaCliente key={c.cod} ctx={ctx} linha={c} grupos={grupos} extra={{}} mostrarSelos />)}
              </div>
            </div>
          ) : (
            <div className="space-y-2">
              <h2 className="text-sm font-semibold px-0.5">
                {scope === 'ggv' ? 'Sua gerência — por Equipe, RCA e Cliente' : 'Sua equipe — por RCA e Cliente'}
                <span className="font-normal text-xs text-muted-foreground"> (clientes que positivaram algum segmento)</span>
              </h2>
              <div className="bg-white border rounded-xl overflow-hidden">
                {carregandoBase && <div className="px-3 py-4 text-sm text-muted-foreground text-center">Carregando...</div>}
                {!carregandoBase && (base?.linhas ?? []).length === 0 && <div className="px-3 py-4 text-sm text-muted-foreground text-center">Nada neste recorte</div>}
                {(base?.linhas ?? []).map(l => scope === 'ggv'
                  ? <BlocoEquipe key={l.cod} ctx={ctx} linha={l} grupos={grupos} />
                  : <BlocoRCA key={l.cod} ctx={ctx} linha={l} grupos={grupos} extra={{}} />)}
              </div>
              <p className="text-[11px] text-muted-foreground px-0.5">
                Um cliente atendido por 2 RCAs aparece em cada um, mas conta 1 só no total da equipe. Toque no {NIVEL_ROTULO[base?.nivel ?? 'rca'] ?? 'item'} para abrir.
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
