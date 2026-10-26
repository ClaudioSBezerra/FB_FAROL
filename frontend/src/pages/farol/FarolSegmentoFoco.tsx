import { Fragment, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Check, ChevronDown, ChevronLeft, ChevronRight } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import TabelaComColunas from '@/components/farol/TabelaComColunas'
import PainelProdutos from '@/components/farol/PainelProdutos'

// Painel "Segmento Foco" (Reckitt) — positivação de grupos de produtos por
// cliente. Primeira tela = soma de todas as UFs (com filtro de UF); clicar no
// grupo abre por UF, na UF abre por GGV → Equipe (CRV) → RCA → Cliente →
// Produto. Aba "Por RCA" = medidor de RCAs (uma linha por RCA, 8 grupos).
// Desenho combinado com o Claudio em 19/10/2026.

interface Vinculo { id: number; industria_nome: string; formula_codigo: string }
interface MesValor { mes: string; objetivo: number | null; realizado: number }
interface GrupoLinha { grupo: string; meses: MesValor[]; objetivo_tri: number | null; realizado_tri: number }
interface UFBloco { uf: string; grupos: GrupoLinha[] }
interface Painel { meses_disponiveis?: string[]; ufs_ocultas?: string[]; trimestre: string; meses: string[]; meses_sem_vigencia: string[] | null; grupos: string[]; ufs: UFBloco[] }
interface DetLinha { cod: string; nome: string; cod_pai?: string; nome_pai?: string; grupos: number[]; total: number; geral: number }
interface DetProduto { grupo: string; cod_prod: string; produto: string; qtd: number; positivou: boolean }
interface Detalhe { nivel: 'ggv' | 'crv' | 'rca' | 'cliente' | 'produto'; grupos: string[]; linhas?: DetLinha[]; produtos?: DetProduto[] }
type Filtro = Record<string, string | undefined>

const FLUXOS = [
  { value: 'faturado', label: 'Faturado' },
  { value: 'transmitido', label: 'Transmitido' },
  { value: 'soma', label: 'Faturado + Transmitido' },
]
const MESES_ROTULO = ['JAN', 'FEV', 'MAR', 'ABR', 'MAI', 'JUN', 'JUL', 'AGO', 'SET', 'OUT', 'NOV', 'DEZ']
const fmt = (n: number) => n.toLocaleString('pt-BR', { maximumFractionDigits: 2 })
const rotuloMes = (m: string) => `${MESES_ROTULO[Number(m.slice(5, 7)) - 1]}/${m.slice(2, 4)}`
const mesAtual = () => new Date().toISOString().slice(0, 7)
function somaMes(mes: string, delta: number) {
  const [a, m] = mes.split('-').map(Number)
  const d = new Date(a, m - 1 + delta, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const temObjetivo = (u: UFBloco) => u.grupos.some(g => g.objetivo_tri != null || g.meses.some(m => m.objetivo != null))
const cabecalhoCel = 'px-3 py-2 text-center text-xs font-semibold text-slate-500 whitespace-nowrap'
const celula = 'px-3 py-2 text-center tabular-nums'
const NIVEIS: Record<string, string> = { ggv: 'GGV', crv: 'Equipe', rca: 'RCA', cliente: 'Cliente' }

interface Ctx {
  vinculoId: number
  mes: string
  fluxo: string
  periodo: 'mes' | 'trimestre'
  headers: Record<string, string>
  grupo: string
  onCliente: (cod: string, nome: string, f: Filtro) => void
}

// Resultado (obj/real/gap/%) — células comuns às linhas do grupo e das UFs.
function CelulasResultado({ obj, real, realComObj }: { obj: number | null; real: number; realComObj?: number }) {
  const base = realComObj ?? real
  const gap = obj != null ? base - obj : null
  const bateu = obj != null && base >= obj
  const cor = bateu ? 'text-emerald-600' : 'text-rose-600'
  return (
    <>
      <td className={celula}>{obj != null ? fmt(obj) : '—'}</td>
      <td className={`${celula} font-semibold`}>
        {fmt(real)}
        {realComObj != null && realComObj !== real && (
          <div className="text-[10px] font-normal text-slate-400">{fmt(real - realComObj)} em UF sem objetivo</div>
        )}
      </td>
      <td className={`${celula} ${gap == null ? '' : cor}`}>
        {gap == null ? '—' : bateu
          ? <Check className="inline w-5 h-5 text-green-600" strokeWidth={3.5} aria-label="Atingido" />
          : fmt(gap)}
      </td>
      <td className={`${celula} ${obj ? cor : ''}`}>{obj ? `${fmt((base / obj) * 100)}%` : '—'}</td>
    </>
  )
}

// Linhas filhas (GGV → Equipe → RCA → Cliente) de um recorte, só pro grupo aberto.
function LinhasFilhas({ ctx, filtro, recuo }: { ctx: Ctx; filtro: Filtro; recuo: number }) {
  const [abertas, setAbertas] = useState<Set<string>>(new Set())
  const { data, isLoading } = useQuery<Detalhe>({
    queryKey: ['seg-det-filhas', ctx.vinculoId, ctx.mes, ctx.fluxo, ctx.periodo, filtro],
    queryFn: async () => {
      const p = new URLSearchParams({ vinculo_id: String(ctx.vinculoId), mes: ctx.mes, fluxo: ctx.fluxo, periodo: ctx.periodo })
      Object.entries(filtro).forEach(([k, v]) => v && p.set(k, v))
      const r = await fetch(`/api/farol/segmento-foco/detalhe?${p}`, { headers: ctx.headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })
  const pad = { paddingLeft: `${recuo * 20 + 12}px` }
  if (isLoading) return <tr><td colSpan={5} className="py-2 text-xs text-slate-400" style={pad}>Carregando...</td></tr>
  if (!data || data.nivel === 'produto') return null
  const idx = data.grupos.indexOf(ctx.grupo)
  const linhas = (data.linhas ?? [])
    .map(l => ({ l, n: idx >= 0 ? l.grupos[idx] : 0 }))
    .filter(x => data.nivel !== 'cliente' || x.n > 0)
    .sort((a, b) => b.n - a.n)
  if (linhas.length === 0) return <tr><td colSpan={5} className="py-2 text-xs text-slate-400" style={pad}>Nenhum cliente positivou neste recorte</td></tr>
  const campo = data.nivel === 'ggv' ? 'cod_ggv' : data.nivel === 'crv' ? 'cod_crv' : data.nivel === 'rca' ? 'cod_rca' : 'cod_cli'
  return (
    <>
      {linhas.map(({ l, n }) => {
        const aberta = abertas.has(l.cod)
        const filtroFilho: Filtro = { ...filtro, [campo]: l.cod }
        const ehCliente = data.nivel === 'cliente'
        return (
          <Fragment key={l.cod}>
            <tr className="border-t cursor-pointer hover:bg-slate-50 bg-slate-50/40" onClick={() => {
              if (ehCliente) { ctx.onCliente(l.cod, l.nome, filtro); return }
              setAbertas(s => { const x = new Set(s); if (aberta) x.delete(l.cod); else x.add(l.cod); return x })
            }}>
              <td className="py-2 text-left" style={pad}>
                <span className="inline-flex items-center gap-1">
                  {!ehCliente && <ChevronDown className={`w-3.5 h-3.5 text-slate-400 transition-transform ${aberta ? '' : '-rotate-90'}`} />}
                  <span className="text-[10px] uppercase text-slate-400 mr-1">{NIVEIS[data.nivel]}</span>
                  {l.cod} — {l.nome}
                </span>
              </td>
              <td className={celula}>—</td>
              <td className={`${celula} font-semibold`}>{fmt(n)}</td>
              <td className={celula}>—</td>
              <td className={celula}>—</td>
            </tr>
            {aberta && !ehCliente && <LinhasFilhas ctx={ctx} filtro={filtroFilho} recuo={recuo + 1} />}
          </Fragment>
        )
      })}
    </>
  )
}

// Linha do medidor: RCA → (clique) segmentos → (clique) clientes do segmento.
function LinhaMedidorRCA({ l, grupos, ctx, uf }: { l: DetLinha; grupos: string[]; ctx: Ctx; uf: string }) {
  const [aberta, setAberta] = useState(false)
  const [grupoAberto, setGrupoAberto] = useState<string | null>(null)
  const { data, isLoading } = useQuery<Detalhe>({
    queryKey: ['seg-medidor-cli', ctx.vinculoId, ctx.mes, ctx.fluxo, ctx.periodo, l.cod, uf, grupoAberto],
    enabled: !!grupoAberto,
    queryFn: async () => {
      const p = new URLSearchParams({ vinculo_id: String(ctx.vinculoId), mes: ctx.mes, fluxo: ctx.fluxo, periodo: ctx.periodo, cod_rca: l.cod })
      if (uf) p.set('uf', uf)
      const r = await fetch(`/api/farol/segmento-foco/detalhe?${p}`, { headers: ctx.headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })
  const span = grupos.length + 1
  return (
    <>
      <tr className="border-t cursor-pointer hover:bg-slate-50" onClick={() => setAberta(a => !a)}>
        <td className="px-3 py-2 text-left whitespace-nowrap">
          <span className="inline-flex items-center gap-1">
            <ChevronDown className={`w-4 h-4 text-slate-400 transition-transform ${aberta ? '' : '-rotate-90'}`} />{l.cod} — {l.nome}
          </span>
        </td>
        <td className={`${celula} whitespace-nowrap`}>{l.cod_pai}</td>
        <td className={`${celula} font-semibold`}>{fmt(l.geral)}</td>
        {l.grupos.map((n, i) => <td key={i} className={`${celula} ${n ? '' : 'text-slate-300'}`}>{fmt(n)}</td>)}
      </tr>
      {aberta && grupos.map((g, i) => {
        const n = l.grupos[i]
        const ab = grupoAberto === g
        return (
          <Fragment key={g}>
            <tr className="border-t bg-slate-50/60 cursor-pointer hover:bg-slate-100" onClick={() => n && setGrupoAberto(ab ? null : g)}>
              <td className="py-2 text-left" style={{ paddingLeft: 36 }}>
                <span className="inline-flex items-center gap-1">
                  <ChevronDown className={`w-3.5 h-3.5 text-slate-400 transition-transform ${ab ? '' : '-rotate-90'}`} />
                  <span className="text-[10px] uppercase text-slate-400 mr-1">Segmento</span>{g}
                </span>
              </td>
              <td className={celula}></td>
              <td className={`${celula} font-semibold ${n ? '' : 'text-slate-300'}`} colSpan={span}>{fmt(n)} cliente(s)</td>
            </tr>
            {ab && isLoading && <tr><td colSpan={span + 2} className="py-2 text-xs text-slate-400" style={{ paddingLeft: 60 }}>Carregando...</td></tr>}
            {ab && data && data.nivel === 'cliente' && (() => {
              const idx = data.grupos.indexOf(g)
              const cl = (data.linhas ?? []).filter(x => idx >= 0 && x.grupos[idx] > 0)
              if (!cl.length) return <tr><td colSpan={span + 2} className="py-2 text-xs text-slate-400" style={{ paddingLeft: 60 }}>Nenhum cliente neste segmento</td></tr>
              return cl.map(c => (
                <tr key={c.cod} className="border-t cursor-pointer hover:bg-slate-100" onClick={() => ctx.onCliente(c.cod, c.nome, { cod_rca: l.cod, uf })}>
                  <td className="py-2 text-left" style={{ paddingLeft: 60 }} colSpan={span + 2}>
                    <span className="text-[10px] uppercase text-slate-400 mr-1">Cliente</span>{c.cod} — {c.nome}
                  </td>
                </tr>
              ))
            })()}
          </Fragment>
        )
      })}
    </>
  )
}

export default function FarolSegmentoFoco() {
  const { token } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])
  const [mes, setMes] = useState(mesAtual)
  const [fluxo, setFluxo] = useState('faturado')
  const [periodo, setPeriodo] = useState<'mes' | 'trimestre'>('mes')
  const [vinculoSel, setVinculoSel] = useState<number | null>(null)
  const [aba, setAba] = useState<'grupos' | 'rca'>('grupos')
  const [ufsSel, setUfsSel] = useState<Set<string>>(new Set())
  const [gruposAbertos, setGruposAbertos] = useState<Set<string>>(new Set())
  const [ufsAbertas, setUfsAbertas] = useState<Set<string>>(new Set())   // "grupo|uf"
  const [cliAberto, setCliAberto] = useState<{ cod: string; nome: string; f: Filtro } | null>(null)
  const [busca, setBusca] = useState('')

  const { data: vinculos = [] } = useQuery<Vinculo[]>({
    queryKey: ['seg-vinculos'],
    queryFn: async () => {
      const r = await fetch('/api/farol/metas-vinculos', { headers })
      if (!r.ok) throw new Error('Erro ao carregar vínculos')
      const todos = (await r.json()) as Vinculo[]
      return todos.filter(v => v.formula_codigo === 'positivacao_grupos')
    },
  })
  const vinculo = vinculos.find(v => v.id === vinculoSel) ?? vinculos[0]

  const { data: painel, isLoading, error } = useQuery<Painel>({
    queryKey: ['seg-painel', vinculo?.id, mes, fluxo],
    enabled: !!vinculo,
    queryFn: async () => {
      const r = await fetch(`/api/farol/segmento-foco?vinculo_id=${vinculo!.id}&mes=${mes}&fluxo=${fluxo}`, { headers })
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).error ?? 'Erro ao carregar o painel')
      return r.json()
    },
  })

  // Abre no último mês com vigência carregada (o mês corrente pode não ter carga ainda).
  const [ajustouMes, setAjustouMes] = useState(false)
  useEffect(() => {
    if (!painel || ajustouMes) return
    setAjustouMes(true)
    const disp = painel.meses_disponiveis ?? []
    if (disp.length && !disp.includes(mes)) setMes(disp[disp.length - 1])
  }, [painel, ajustouMes, mes])

  const idxMes = painel ? Math.max(0, painel.meses.indexOf(mes)) : 0
  const todasUFs = useMemo(() => [...(painel?.ufs ?? [])].sort((a, b) =>
    Number(temObjetivo(b)) - Number(temObjetivo(a)) || a.uf.localeCompare(b.uf, 'pt-BR')), [painel])
  const ufsVisiveis = todasUFs.filter(u => ufsSel.size === 0 || ufsSel.has(u.uf))
  const ufParam = ufsSel.size ? [...ufsSel].join(',') : ''

  const valor = (g: GrupoLinha) => ({
    obj: periodo === 'mes' ? g.meses[idxMes]?.objetivo ?? null : g.objetivo_tri,
    real: periodo === 'mes' ? g.meses[idxMes]?.realizado ?? 0 : g.realizado_tri,
  })

  // Soma por grupo nas UFs selecionadas. O % e o gap só consideram as UFs que
  // têm objetivo (senão a venda de UF sem objetivo inflaria o atingimento).
  const porGrupo = (painel?.grupos ?? []).map(grupo => {
    let obj = 0, temObj = false, real = 0, realComObj = 0
    for (const u of ufsVisiveis) {
      const g = u.grupos.find(x => x.grupo === grupo)
      if (!g) continue
      const v = valor(g)
      real += v.real
      if (v.obj != null) { obj += v.obj; temObj = true; realComObj += v.real }
    }
    return { grupo, obj: temObj ? obj : null, real, realComObj }
  })
  const tot = porGrupo.reduce((a, l) => ({
    obj: a.obj + (l.obj ?? 0), temObj: a.temObj || l.obj != null, real: a.real + l.real, realComObj: a.realComObj + l.realComObj,
  }), { obj: 0, temObj: false, real: 0, realComObj: 0 })

  const toggle = (set: Set<string>, k: string, fn: (s: Set<string>) => void) => {
    const x = new Set(set)
    if (x.has(k)) x.delete(k); else x.add(k)
    fn(x)
  }

  const ctxBase = (grupo: string): Ctx => ({
    vinculoId: vinculo!.id, mes, fluxo, periodo, headers, grupo,
    onCliente: (cod, nome, f) => setCliAberto({ cod, nome, f }),
  })

  // Aba "Por RCA" (medidor): uma linha por RCA, com a equipe e os 8 grupos.
  const { data: medidor, isLoading: carregandoMed } = useQuery<Detalhe>({
    queryKey: ['seg-medidor', vinculo?.id, mes, fluxo, periodo, ufParam],
    enabled: !!vinculo && aba === 'rca',
    queryFn: async () => {
      const p = new URLSearchParams({ vinculo_id: String(vinculo!.id), mes, fluxo, periodo, nivel: 'rca' })
      if (ufParam) p.set('uf', ufParam)
      const r = await fetch(`/api/farol/segmento-foco/detalhe?${p}`, { headers })
      if (!r.ok) throw new Error('Erro ao carregar o medidor')
      return r.json()
    },
  })

  const { data: prodCli, isLoading: carregandoProd } = useQuery<Detalhe>({
    queryKey: ['seg-produtos-cli', vinculo?.id, mes, fluxo, periodo, cliAberto],
    enabled: !!vinculo && !!cliAberto,
    queryFn: async () => {
      const p = new URLSearchParams({ vinculo_id: String(vinculo!.id), mes, fluxo, periodo })
      Object.entries({ ...cliAberto!.f, cod_cli: cliAberto!.cod }).forEach(([k, v]) => v && p.set(k, v))
      const r = await fetch(`/api/farol/segmento-foco/detalhe?${p}`, { headers })
      if (!r.ok) throw new Error('Erro ao carregar os produtos')
      return r.json()
    },
  })

  const periodoRotulo = periodo === 'mes' ? rotuloMes(mes) : `trimestre ${painel?.trimestre ?? ''}`
  const chip = (ativo: boolean, cor = 'bg-slate-800 text-white border-slate-800') =>
    `px-3 py-1 text-xs font-semibold rounded-full border ${ativo ? cor : 'bg-white text-slate-600 hover:bg-slate-50'}`

  const linhasMedidor = (medidor?.linhas ?? []).filter(l => {
    const b = busca.trim().toLowerCase()
    return !b || `${l.cod} ${l.nome} ${l.cod_pai ?? ''} ${l.nome_pai ?? ''}`.toLowerCase().includes(b)
  }).sort((a, b) => (a.cod_pai ?? '').localeCompare(b.cod_pai ?? '', 'pt-BR', { numeric: true }) || b.geral - a.geral)

  return (
    <div className="space-y-5 p-4 md:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Segmento Foco{vinculo ? ` — ${vinculo.industria_nome}` : ''}</h1>
          <p className="text-sm text-muted-foreground">Positivação de grupos de produtos por cliente: objetivo × realizado, mensal e trimestral.</p>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="outline" size="icon" onClick={() => setMes(m => somaMes(m, -1))} aria-label="Mês anterior"><ChevronLeft className="w-4 h-4" /></Button>
          <div className="w-28 text-center text-sm font-semibold">{rotuloMes(mes)}</div>
          <Button variant="outline" size="icon" onClick={() => setMes(m => somaMes(m, 1))} aria-label="Próximo mês"><ChevronRight className="w-4 h-4" /></Button>
        </div>
      </div>

      {vinculos.length === 0 && (
        <div className="border rounded-lg p-4 text-sm text-muted-foreground">
          Nenhum programa de Segmento Foco configurado. Crie o vínculo em Configurações → Vínculos (avançado) e faça a carga em Configurações → Carga do mês.
        </div>
      )}

      {vinculos.length > 1 && (
        <div className="flex gap-2">
          {vinculos.map(v => (
            <button key={v.id} onClick={() => setVinculoSel(v.id)} className={chip(vinculo?.id === v.id)}>{v.industria_nome}</button>
          ))}
        </div>
      )}

      {vinculo && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold uppercase text-slate-500">Fluxo</span>
              {FLUXOS.map(f => <button key={f.value} onClick={() => setFluxo(f.value)} className={chip(fluxo === f.value)}>{f.label}</button>)}
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs font-semibold uppercase text-slate-500">Período</span>
              {(painel?.meses ?? []).map(m => (
                <button key={m} onClick={() => { setPeriodo('mes'); setMes(m) }} className={chip(periodo === 'mes' && m === mes)}>
                  {MESES_ROTULO[Number(m.slice(5, 7)) - 1]}
                </button>
              ))}
              <button onClick={() => setPeriodo('trimestre')} className={chip(periodo === 'trimestre', 'bg-violet-700 text-white border-violet-700')}>
                Trimestre{painel ? ` (${painel.trimestre})` : ''}
              </button>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs font-semibold uppercase text-slate-500">UF</span>
            <button onClick={() => setUfsSel(new Set())} className={chip(ufsSel.size === 0)}>Todas</button>
            {todasUFs.map(u => (
              <button key={u.uf} onClick={() => toggle(ufsSel, u.uf, setUfsSel)}
                title={temObjetivo(u) ? undefined : 'Sem objetivo cadastrado'}
                className={`${chip(ufsSel.has(u.uf))} ${temObjetivo(u) ? '' : 'italic opacity-80'}`}>{u.uf}</button>
            ))}
          </div>
          <div className="flex gap-2 border-b">
            {([['grupos', 'Por grupo'], ['rca', 'Por RCA (medidor)']] as const).map(([k, rot]) => (
              <button key={k} onClick={() => setAba(k)}
                className={`px-4 py-2 text-sm font-semibold border-b-2 -mb-px ${aba === k ? 'border-violet-700 text-violet-800' : 'border-transparent text-slate-500 hover:text-slate-800'}`}>{rot}</button>
            ))}
          </div>
        </div>
      )}

      {isLoading && <p className="text-sm text-muted-foreground">Carregando...</p>}
      {error && <p className="text-sm text-red-700">{(error as Error).message}</p>}
      {painel && (painel.ufs_ocultas ?? []).length > 0 && (
        <p className="text-xs text-muted-foreground">UF oculta no painel: {(painel.ufs_ocultas ?? []).join(', ')} (aguardando a carga da base; libere em Configurações → Carga do mês).</p>
      )}
      {painel && painel.meses_sem_vigencia && painel.meses_sem_vigencia.length > 0 && (
        <div className="text-xs border border-amber-300 bg-amber-50 rounded-lg p-3 text-amber-900">
          Sem vigência cadastrada em: {painel.meses_sem_vigencia.map(rotuloMes).join(', ')}. O trimestre soma só os meses com carga — faça a carga em Configurações → Carga do mês.
        </div>
      )}

      {painel && vinculo && aba === 'grupos' && (
        <div className="border rounded-xl bg-white">
          <div className="px-4 py-2.5 border-b flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-semibold">{periodoRotulo} — {ufsSel.size ? [...ufsSel].sort().join(', ') : 'todas as UFs'}</h2>
            <span className="text-xs text-slate-500">Clique no grupo pra abrir por UF; na UF, pra descer até o cliente.</span>
          </div>
          <TabelaComColunas id="segfoco-grupos" className="overflow-auto">
            <table className="w-full text-sm">
              <thead>
                <tr>
                  <th className={`${cabecalhoCel} text-left`}>Grupo</th>
                  <th className={cabecalhoCel}>{periodo === 'mes' ? 'Objetivo' : 'Meta trimestre'}</th>
                  <th className={cabecalhoCel}>Realizado</th>
                  <th className={cabecalhoCel}>Gap</th>
                  <th className={cabecalhoCel}>%</th>
                </tr>
              </thead>
              <tbody>
                {porGrupo.map(l => {
                  const aberto = gruposAbertos.has(l.grupo)
                  return (
                    <Fragment key={l.grupo}>
                      <tr className="border-t cursor-pointer hover:bg-slate-50" onClick={() => toggle(gruposAbertos, l.grupo, setGruposAbertos)}>
                        <td className="px-3 py-2 text-left font-medium">
                          <span className="inline-flex items-center gap-1">
                            <ChevronDown className={`w-4 h-4 text-slate-400 transition-transform ${aberto ? '' : '-rotate-90'}`} />{l.grupo}
                          </span>
                        </td>
                        <CelulasResultado obj={l.obj} real={l.real} realComObj={l.realComObj} />
                      </tr>
                      {aberto && ufsVisiveis.map(u => {
                        const g = u.grupos.find(x => x.grupo === l.grupo)
                        if (!g) return null
                        const v = valor(g)
                        const kUF = `${l.grupo}|${u.uf}`
                        const ufAberta = ufsAbertas.has(kUF)
                        return (
                          <Fragment key={kUF}>
                            <tr className="border-t cursor-pointer hover:bg-slate-50 bg-slate-50/60" onClick={() => toggle(ufsAbertas, kUF, setUfsAbertas)}>
                              <td className="py-2 text-left" style={{ paddingLeft: 32 }}>
                                <span className="inline-flex items-center gap-1">
                                  <ChevronDown className={`w-3.5 h-3.5 text-slate-400 transition-transform ${ufAberta ? '' : '-rotate-90'}`} />
                                  <span className="text-[10px] uppercase text-slate-400 mr-1">UF</span>{u.uf}
                                </span>
                              </td>
                              <CelulasResultado obj={v.obj} real={v.real} />
                            </tr>
                            {ufAberta && <LinhasFilhas ctx={ctxBase(l.grupo)} filtro={{ uf: u.uf }} recuo={2} />}
                          </Fragment>
                        )
                      })}
                    </Fragment>
                  )
                })}
                <tr className="border-t bg-slate-50 font-semibold">
                  <td className="px-3 py-2 text-left">Total</td>
                  <CelulasResultado obj={tot.temObj ? tot.obj : null} real={tot.real} realComObj={tot.realComObj} />
                </tr>
              </tbody>
            </table>
          </TabelaComColunas>
          {ufsVisiveis.some(u => !temObjetivo(u)) && (
            <p className="px-4 py-2 text-xs text-muted-foreground border-t">
              Gap e % consideram só as UFs com objetivo cadastrado ({ufsVisiveis.filter(temObjetivo).map(u => u.uf).join(', ') || 'nenhuma'}); o realizado das demais UFs aparece à parte.
            </p>
          )}
        </div>
      )}

      {vinculo && aba === 'rca' && (
        <div className="border rounded-xl bg-white">
          <div className="px-4 py-2.5 border-b flex flex-wrap items-center justify-between gap-2">
            <h2 className="font-semibold">Medidor por RCA — {periodoRotulo}</h2>
            <input value={busca} onChange={e => setBusca(e.target.value)} placeholder="Filtrar equipe, RCA ou nome"
              className="border rounded-md px-3 py-1.5 text-sm w-64" />
          </div>
          {carregandoMed && <p className="p-4 text-sm text-muted-foreground">Carregando...</p>}
          {medidor && (
            <TabelaComColunas id="segfoco-medidor" className="overflow-auto max-h-[70vh]">
              <p className="px-4 pt-2 text-xs text-muted-foreground">
                Clique no RCA pra ver os segmentos e no segmento pra ver os clientes. Um cliente atendido por 2 RCAs aparece em cada um, mas conta 1 só no total. “Clientes c/ algum grupo” = clientes que positivaram pelo menos um grupo (não é a “Positivação Geral” da planilha do fornecedor){periodo === 'trimestre' ? ' (somado mês a mês)' : ''}.
              </p>
              <table className="w-full text-sm">
                <thead className="sticky top-0 bg-white shadow-[0_1px_0_0_#e2e8f0]">
                  <tr>
                    <th className={`${cabecalhoCel} text-left`}>RCA</th>
                    <th className={cabecalhoCel}>Equipe</th>
                    <th className={cabecalhoCel}>Clientes c/ algum grupo</th>
                    {(medidor.grupos ?? []).map(g => <th key={g} className={cabecalhoCel}>{g}</th>)}
                  </tr>
                </thead>
                <tbody>
                  {linhasMedidor.map(l => <LinhaMedidorRCA key={l.cod} l={l} grupos={medidor.grupos ?? []} ctx={ctxBase('')} uf={ufParam} />)}
                  {linhasMedidor.length === 0 && <tr><td colSpan={(medidor.grupos ?? []).length + 3} className="py-8 text-center text-slate-500">Nenhum RCA neste recorte</td></tr>}
                </tbody>
              </table>
            </TabelaComColunas>
          )}
        </div>
      )}

      <PainelProdutos
        aberto={!!cliAberto}
        onFechar={() => setCliAberto(null)}
        titulo={cliAberto ? `${cliAberto.cod} - ${cliAberto.nome}` : ''}
        subtitulo="Produtos dos grupos do programa que o cliente comprou no período."
        rotuloItem="Produto"
        rotuloSim="Positivou"
        rotuloNao="Não positivou"
        participio="positivado"
        subtituloResumo="neste cliente"
        carregando={carregandoProd}
        linhas={(prodCli?.produtos ?? []).map(p => ({
          chave: p.cod_prod, codigos: p.cod_prod, nome: p.produto, vendeu: p.positivou, qtd: p.qtd, aviso: p.grupo,
        }))}
      />
    </div>
  )
}
