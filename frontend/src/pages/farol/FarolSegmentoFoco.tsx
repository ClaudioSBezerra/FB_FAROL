import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import TabelaComColunas from '@/components/farol/TabelaComColunas'
import PainelProdutos from '@/components/farol/PainelProdutos'

// Painel "Segmento Foco" (Reckitt) — positivação de grupos de produtos por
// cliente. Objetivo × realizado por UF e grupo, mensal e trimestral (o
// trimestre soma os 3 meses), com drill GGV → CRV → RCA → Cliente → Produto.
// Mesma ideia do painel de Objetivos da Unilever, painel novo (19/10/2026).

interface Vinculo { id: number; industria_nome: string; formula_codigo: string }
interface MesValor { mes: string; objetivo: number | null; realizado: number }
interface GrupoLinha { grupo: string; meses: MesValor[]; objetivo_tri: number | null; realizado_tri: number }
interface UFBloco { uf: string; grupos: GrupoLinha[] }
interface Painel { ufs_ocultas?: string[]; trimestre: string; meses: string[]; meses_sem_vigencia: string[] | null; grupos: string[]; ufs: UFBloco[] }
interface DetLinha { cod: string; nome: string; grupos: number[]; total: number }
interface DetProduto { grupo: string; cod_prod: string; produto: string; qtd: number; positivou: boolean }
interface Detalhe { nivel: 'ggv' | 'crv' | 'rca' | 'cliente' | 'produto'; grupos: string[]; linhas?: DetLinha[]; produtos?: DetProduto[] }

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

// UFs com objetivo primeiro (as do programa); as demais (clientes de outras
// UFs) vão pro fim, sob um título à parte.
const temObjetivo = (u: UFBloco) => u.grupos.some(g => g.objetivo_tri != null || g.meses.some(m => m.objetivo != null))
const ordenarUFs = (ufs: UFBloco[]) =>
  [...ufs].sort((a, b) => Number(temObjetivo(b)) - Number(temObjetivo(a)) || a.uf.localeCompare(b.uf, 'pt-BR'))

interface Drill { uf: string; ggv?: { cod: string; nome: string }; crv?: { cod: string; nome: string }; rca?: { cod: string; nome: string }; cli?: { cod: string; nome: string } }

export default function FarolSegmentoFoco() {
  const { token } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])
  const [mes, setMes] = useState(mesAtual)
  const [fluxo, setFluxo] = useState('faturado')
  const [periodo, setPeriodo] = useState<'mes' | 'trimestre'>('mes')
  const [vinculoSel, setVinculoSel] = useState<number | null>(null)
  const [drill, setDrill] = useState<Drill | null>(null)
  const detalheRef = useRef<HTMLDivElement>(null)
  // Ao abrir/descer no detalhe, leva a tela até ele (fica abaixo dos blocos de UF).
  useEffect(() => {
    if (drill) detalheRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, [drill?.uf, drill?.ggv?.cod, drill?.crv?.cod, drill?.rca?.cod])   // eslint-disable-line react-hooks/exhaustive-deps

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

  const qs = (d: Drill) => {
    const p = new URLSearchParams({ vinculo_id: String(vinculo!.id), mes, fluxo, periodo, uf: d.uf })
    if (d.ggv) p.set('cod_ggv', d.ggv.cod)
    if (d.crv) p.set('cod_crv', d.crv.cod)
    if (d.rca) p.set('cod_rca', d.rca.cod)
    if (d.cli) p.set('cod_cli', d.cli.cod)
    return p.toString()
  }
  const { data: detalhe, isLoading: carregandoDet } = useQuery<Detalhe>({
    queryKey: ['seg-detalhe', vinculo?.id, mes, fluxo, periodo, drill],
    enabled: !!vinculo && !!drill,
    queryFn: async () => {
      const r = await fetch(`/api/farol/segmento-foco/detalhe?${qs(drill!)}`, { headers })
      if (!r.ok) throw new Error('Erro ao carregar o detalhe')
      return r.json()
    },
  })

  const idxMes = painel ? Math.max(0, painel.meses.indexOf(mes)) : 0
  const abrir = (uf: string) => setDrill({ uf })
  const descer = (l: DetLinha) => {
    if (!drill || !detalhe) return
    const item = { cod: l.cod, nome: l.nome }
    if (detalhe.nivel === 'ggv') setDrill({ ...drill, ggv: item })
    else if (detalhe.nivel === 'crv') setDrill({ ...drill, crv: item })
    else if (detalhe.nivel === 'rca') setDrill({ ...drill, rca: item })
    else if (detalhe.nivel === 'cliente') setDrill({ ...drill, cli: item })
  }
  const subir = (ate: 'uf' | 'ggv' | 'crv' | 'rca') => {
    if (!drill) return
    const base: Drill = { uf: drill.uf }
    if (ate !== 'uf') base.ggv = drill.ggv
    if (ate === 'crv' || ate === 'rca') base.crv = drill.crv
    if (ate === 'rca') base.rca = drill.rca
    setDrill(base)
  }

  const cabecalhoCel = 'px-3 py-2 text-center text-xs font-semibold text-slate-500 whitespace-nowrap'
  const celula = 'px-3 py-2 text-center tabular-nums'

  return (
    <div className="space-y-5 p-4 md:p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Segmento Foco{vinculo ? ` — ${vinculo.industria_nome}` : ''}</h1>
          <p className="text-sm text-muted-foreground">Positivação de grupos de produtos por cliente: objetivo × realizado por UF, mensal e trimestral.</p>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="outline" size="icon" onClick={() => { setMes(m => somaMes(m, -1)); setDrill(null) }} aria-label="Mês anterior"><ChevronLeft className="w-4 h-4" /></Button>
          <div className="w-28 text-center text-sm font-semibold">{rotuloMes(mes)}</div>
          <Button variant="outline" size="icon" onClick={() => { setMes(m => somaMes(m, 1)); setDrill(null) }} aria-label="Próximo mês"><ChevronRight className="w-4 h-4" /></Button>
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
            <button key={v.id} onClick={() => { setVinculoSel(v.id); setDrill(null) }}
              className={`px-3 py-1 text-xs font-semibold rounded-full border ${vinculo?.id === v.id ? 'bg-slate-800 text-white border-slate-800' : 'bg-white text-slate-600'}`}>
              {v.industria_nome}
            </button>
          ))}
        </div>
      )}

      {vinculo && (
        <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
          <div className="flex items-center gap-2">
            <span className="text-xs font-semibold uppercase text-slate-500">Fluxo</span>
            {FLUXOS.map(f => (
              <button key={f.value} onClick={() => setFluxo(f.value)}
                className={`px-3 py-1 text-xs font-semibold rounded-full border ${fluxo === f.value ? 'bg-slate-800 text-white border-slate-800' : 'bg-white text-slate-600 hover:bg-slate-50'}`}>
                {f.label}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <span className="text-xs font-semibold uppercase text-slate-500">Período</span>
            {(painel?.meses ?? []).map(m => (
              <button key={m} onClick={() => { setPeriodo('mes'); setMes(m) }}
                className={`px-3 py-1 text-xs font-semibold rounded-full border ${periodo === 'mes' && m === mes ? 'bg-slate-800 text-white border-slate-800' : 'bg-white text-slate-600 hover:bg-slate-50'}`}>
                {MESES_ROTULO[Number(m.slice(5, 7)) - 1]}
              </button>
            ))}
            <button onClick={() => setPeriodo('trimestre')}
              className={`px-3 py-1 text-xs font-semibold rounded-full border ${periodo === 'trimestre' ? 'bg-violet-700 text-white border-violet-700' : 'bg-white text-slate-600 hover:bg-slate-50'}`}>
              Trimestre{painel ? ` (${painel.trimestre})` : ''}
            </button>
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

      {painel && ordenarUFs(painel.ufs ?? []).map((u, iUF, todas) => {
        const semObjetivo = !temObjetivo(u)
        const primeiraSem = semObjetivo && (iUF === 0 || temObjetivo(todas[iUF - 1]))
        const linhas = u.grupos.map(g => {
          const obj = periodo === 'mes' ? g.meses[idxMes]?.objetivo : g.objetivo_tri
          const real = periodo === 'mes' ? g.meses[idxMes]?.realizado ?? 0 : g.realizado_tri
          return { grupo: g.grupo, obj, real }
        })
        const totObj = linhas.reduce((s, l) => s + (l.obj ?? 0), 0)
        const totReal = linhas.reduce((s, l) => s + l.real, 0)
        const linhaUI = (rotulo: string, obj: number | null | undefined, real: number, forte = false) => {
          const gap = obj != null ? real - obj : null
          const bateu = obj != null && real >= obj
          return (
            <tr key={rotulo} className={`border-t ${forte ? 'bg-slate-50 font-semibold' : ''}`}>
              <td className="px-3 py-2 text-left">{rotulo}</td>
              <td className={celula}>{obj != null ? fmt(obj) : '—'}</td>
              <td className={`${celula} font-semibold`}>{fmt(real)}</td>
              <td className={`${celula} ${gap == null ? '' : bateu ? 'text-emerald-600' : 'text-rose-600'}`}>
                {gap == null ? '—' : bateu ? 'Atingido' : fmt(gap)}
              </td>
              <td className={`${celula} ${obj ? (bateu ? 'text-emerald-600' : 'text-rose-600') : ''}`}>{obj ? `${fmt((real / obj) * 100)}%` : '—'}</td>
            </tr>
          )
        }
        return (
          <div key={u.uf} className="space-y-2">
          {primeiraSem && <div className="pt-2 text-xs font-semibold uppercase tracking-wide text-slate-500">Outras UFs — sem objetivo cadastrado</div>}
          <div className="border rounded-xl bg-white">
            <div className="flex items-center justify-between px-4 py-2.5 border-b">
              <h2 className="font-semibold">UF {u.uf}</h2>
              <button className="text-xs underline text-violet-700" onClick={() => abrir(u.uf)}>Detalhar por GGV → cliente</button>
            </div>
            <TabelaComColunas id="segfoco-uf" className="overflow-auto">
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
                  {linhas.map(l => linhaUI(l.grupo, l.obj, l.real))}
                  {linhaUI('Total', totObj || null, totReal, true)}
                </tbody>
              </table>
            </TabelaComColunas>
          </div>
          </div>
        )
      })}

      {/* Detalhamento por hierarquia */}
      {drill && (
        <div ref={detalheRef} className="border rounded-xl bg-white scroll-mt-4">
          <div className="flex flex-wrap items-center gap-1 px-4 py-2.5 border-b text-sm">
            <span className="text-xs font-semibold uppercase text-slate-500 mr-2">Detalhe {periodo === 'mes' ? rotuloMes(mes) : `trimestre ${painel?.trimestre ?? ''}`}</span>
            <button className="underline text-violet-700" onClick={() => subir('uf')}>UF {drill.uf}</button>
            {drill.ggv && <><span>›</span><button className="underline text-violet-700" onClick={() => subir('ggv')}>{drill.ggv.cod} {drill.ggv.nome}</button></>}
            {drill.crv && <><span>›</span><button className="underline text-violet-700" onClick={() => subir('crv')}>{drill.crv.cod} {drill.crv.nome}</button></>}
            {drill.rca && <><span>›</span><button className="underline text-violet-700" onClick={() => subir('rca')}>{drill.rca.cod} {drill.rca.nome}</button></>}
            {drill.cli && <><span>›</span><span>{drill.cli.cod} {drill.cli.nome}</span></>}
            <button className="ml-auto text-xs text-slate-500 hover:text-slate-800" onClick={() => setDrill(null)}>Fechar detalhe</button>
          </div>
          {carregandoDet && <p className="p-4 text-sm text-muted-foreground">Carregando...</p>}
          {detalhe && detalhe.nivel !== 'produto' && (
            <TabelaComColunas id="segfoco-detalhe" className="overflow-auto max-h-[60vh]">
              <p className="px-4 pt-2 text-xs text-muted-foreground">
                {detalhe.nivel === 'cliente' ? 'Clique num cliente pra ver os produtos.' : 'Clique numa linha pra descer um nível.'} Os números são clientes positivados em cada grupo.
              </p>
              <table className="w-full text-sm">
                <thead className="sticky top-0 bg-white shadow-[0_1px_0_0_#e2e8f0]">
                  <tr>
                    <th className={`${cabecalhoCel} text-left`}>{({ ggv: 'GGV', crv: 'CRV', rca: 'RCA', cliente: 'Cliente' } as Record<string, string>)[detalhe.nivel]}</th>
                    {(detalhe.grupos ?? []).map(g => <th key={g} className={cabecalhoCel}>{g}</th>)}
                    <th className={cabecalhoCel}>Total</th>
                  </tr>
                </thead>
                <tbody>
                  {(detalhe.linhas ?? []).map(l => (
                    <tr key={l.cod} className="border-t cursor-pointer hover:bg-slate-50" onClick={() => descer(l)}>
                      <td className="px-3 py-2 text-left whitespace-nowrap">{l.cod} — {l.nome}</td>
                      {l.grupos.map((n, i) => <td key={i} className={`${celula} ${n ? '' : 'text-slate-300'}`}>{fmt(n)}</td>)}
                      <td className={`${celula} font-semibold`}>{fmt(l.total)}</td>
                    </tr>
                  ))}
                  {(detalhe.linhas ?? []).length === 0 && <tr><td colSpan={(detalhe.grupos ?? []).length + 2} className="py-8 text-center text-slate-500">Nenhuma positivação neste recorte</td></tr>}
                </tbody>
              </table>
            </TabelaComColunas>
          )}
        </div>
      )}

      <PainelProdutos
        aberto={!!drill?.cli}
        onFechar={() => drill && setDrill({ ...drill, cli: undefined })}
        titulo={drill?.cli ? `${drill.cli.cod} - ${drill.cli.nome}` : ''}
        subtitulo="Produtos dos grupos do programa que o cliente comprou no período."
        rotuloItem="Produto"
        rotuloSim="Positivou"
        rotuloNao="Não positivou"
        participio="positivado"
        subtituloResumo="neste cliente"
        carregando={carregandoDet}
        linhas={(detalhe?.produtos ?? []).map(p => ({
          chave: p.cod_prod, codigos: p.cod_prod, nome: p.produto, vendeu: p.positivou, qtd: p.qtd, aviso: p.grupo,
        }))}
      />
    </div>
  )
}
