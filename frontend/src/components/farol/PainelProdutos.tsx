import { useMemo, useState } from 'react'
import { Check, Search, X } from 'lucide-react'
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet'
import TabelaComColunas from '@/components/farol/TabelaComColunas'

// Painel "Produtos" — último nível do painel de Objetivos (itens da Ponderada,
// PPAs da Numérica). Pedido do Heverton 19/10/2026: o modal em maiúsculas
// "estava fora do padrão do sistema"; a sugestão dele foi uma aba "Produtos"
// que abre ao clicar e some ao fechar. Aqui: painel lateral com uma aba
// "Produtos" (fecha no X, Esc ou clicando fora), resumo com barra de objetivo,
// filtro Todos/Vendidos/Não vendidos, busca e as colunas centralizadas.

export interface LinhaProduto {
  chave: string
  codigos: string
  nome: string
  vendeu: boolean
  qtd: number
  valor?: number
  aviso?: string
}

const fmt = (n: number) => n.toLocaleString('pt-BR', { maximumFractionDigits: 2 })
const fmtBRL = (n: number) => n.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })

type Filtro = 'todos' | 'sim' | 'nao'

export default function PainelProdutos({
  aberto, onFechar, titulo, subtitulo, rotuloItem, rotuloSim, rotuloNao, objetivo, rotuloObjetivo,
  linhas, carregando, erro, mensagem, nota, comValor,
}: {
  aberto: boolean
  onFechar: () => void
  titulo: string
  subtitulo?: string
  rotuloItem: string           // coluna do nome: "Produto" ou "PPA"
  rotuloSim: string            // "Vendeu" | "Comprou"
  rotuloNao: string            // "Não vendeu" | "Não comprou"
  objetivo: number
  rotuloObjetivo: string       // "EANs distintos" | "PPAs distintos"
  linhas: LinhaProduto[]
  carregando?: boolean
  erro?: string
  mensagem?: string            // substitui a tabela (ex.: classe sem Sortimento)
  nota?: React.ReactNode
  comValor?: boolean
}) {
  const [filtro, setFiltro] = useState<Filtro>('todos')
  const [busca, setBusca] = useState('')

  const vendidos = linhas.filter(l => l.vendeu).length
  const visiveis = useMemo(() => {
    const q = busca.trim().toLowerCase()
    return [...linhas]
      .filter(l => filtro === 'todos' || (filtro === 'sim' ? l.vendeu : !l.vendeu))
      .filter(l => !q || l.nome.toLowerCase().includes(q) || l.codigos.toLowerCase().includes(q))
      .sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR'))
  }, [linhas, filtro, busca])
  // "17 vendidos" / "1 vendido" — particípio concorda com a quantidade (Heverton 19/10/2026).
  const particípio = rotuloSim === 'Vendeu' ? 'vendido' : 'comprado'
  const pct = objetivo > 0 ? Math.min(100, (vendidos / objetivo) * 100) : 0
  const bateu = objetivo > 0 && vendidos >= objetivo

  const Seg = ({ v, rotulo, n }: { v: Filtro; rotulo: string; n: number }) => (
    <button
      type="button"
      onClick={() => setFiltro(v)}
      className={`px-3 py-1 text-xs font-semibold rounded-full border ${filtro === v ? 'bg-slate-800 text-white border-slate-800' : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-50'}`}
    >
      {rotulo} ({fmt(n)})
    </button>
  )

  return (
    <Sheet open={aberto} onOpenChange={o => { if (!o) { onFechar(); setFiltro('todos'); setBusca('') } }}>
      <SheetContent side="right" className="top-16 bottom-0 h-auto w-full sm:max-w-[1100px] p-0 flex flex-col gap-0 rounded-tl-2xl border-t [&>button]:hidden">
        {/* Aba "Produtos" + fechar */}
        <div className="flex items-end justify-between border-b bg-slate-50 px-4 pt-3">
          <div className="inline-flex items-center gap-2 bg-white border border-b-0 rounded-t-lg px-4 py-2 text-sm font-semibold text-slate-800 -mb-px">
            Produtos
            {!carregando && !mensagem && <span className="text-xs font-medium text-slate-500">{fmt(linhas.length)}</span>}
          </div>
          <SheetClose className="mb-2 rounded-md p-1.5 text-slate-500 hover:bg-slate-200 hover:text-slate-900" aria-label="Fechar">
            <X className="h-4 w-4" />
          </SheetClose>
        </div>

        <div className="flex-1 overflow-auto p-5 space-y-4">
          <div>
            <SheetTitle className="text-lg font-semibold text-slate-900">{titulo}</SheetTitle>
            <SheetDescription className="text-sm text-slate-500">{subtitulo ?? 'Quais itens foram comprados e quais faltam.'}</SheetDescription>
          </div>

          {mensagem ? (
            <p className="text-sm text-slate-600 py-10 text-center">{mensagem}</p>
          ) : carregando ? (
            <p className="text-sm text-slate-500 py-10 text-center">Carregando...</p>
          ) : erro ? (
            <p className="text-sm text-red-700 py-10 text-center">{erro}</p>
          ) : (
            <>
              {/* Resumo + progresso do objetivo */}
              <div className="border rounded-xl p-4 bg-white">
                <div className="flex flex-wrap items-baseline gap-x-6 gap-y-1">
                  <div><span className={`text-2xl font-bold ${bateu ? 'text-emerald-600' : 'text-slate-900'}`}>{fmt(vendidos)}</span> <span className="text-sm text-slate-500">{particípio}{vendidos > 1 ? 's' : ''} de {fmt(linhas.length)} no catálogo</span></div>
                  <div className="text-sm text-slate-600">Objetivo: <strong>{fmt(objetivo)}</strong> {rotuloObjetivo}</div>
                </div>
                <div className="mt-2 h-2 rounded-full bg-slate-100 overflow-hidden">
                  <div className={`h-full ${bateu ? 'bg-emerald-500' : 'bg-rose-500'}`} style={{ width: `${pct}%` }} />
                </div>
              </div>

              {nota && <div className="text-xs border border-amber-300 bg-amber-50 rounded-lg p-3 text-amber-900">{nota}</div>}

              <div className="flex flex-wrap items-center gap-2">
                <Seg v="todos" rotulo="Todos" n={linhas.length} />
                <Seg v="sim" rotulo={rotuloSim} n={vendidos} />
                <Seg v="nao" rotulo={rotuloNao} n={linhas.length - vendidos} />
                <div className="ml-auto relative">
                  <Search className="h-3.5 w-3.5 absolute left-2 top-2 text-slate-400" />
                  <input
                    value={busca}
                    onChange={e => setBusca(e.target.value)}
                    placeholder="Buscar código ou nome"
                    className="h-8 w-52 rounded-md border border-slate-200 pl-7 pr-2 text-sm"
                  />
                </div>
              </div>

              <TabelaComColunas id={comValor ? 'produtos-itens' : 'produtos-ppas'} className="border rounded-lg overflow-auto max-h-[calc(100vh-27rem)] min-h-40 bg-white">
                <table className="w-full text-sm table-fixed">
                  <thead className="sticky top-0 z-10 bg-white shadow-[0_1px_0_0_#e2e8f0]">
                    <tr className="text-xs font-semibold text-slate-500">
                      <th className="px-3 py-2 text-center w-[26%]">Cód. Produto</th>
                      <th className="px-3 py-2 text-left">{rotuloItem}</th>
                      <th className="px-3 py-2 text-center w-[120px]">Status</th>
                      <th className="px-3 py-2 text-center w-[70px]">Qtd</th>
                      {comValor && <th className="px-3 py-2 text-center w-[110px]">Valor</th>}
                    </tr>
                  </thead>
                  <tbody>
                    {visiveis.length === 0 && (
                      <tr><td colSpan={comValor ? 5 : 4} className="py-8 text-center text-slate-500">Nenhum item neste filtro</td></tr>
                    )}
                    {visiveis.map(l => (
                      <tr key={l.chave} className="border-t">
                        <td className="px-3 py-2 text-center font-mono text-[11px] text-slate-600 break-words">{l.codigos || '—'}</td>
                        <td className="px-3 py-2 text-left">
                          {l.nome || '—'}
                          {l.aviso && <div className="text-xs text-amber-700">{l.aviso}</div>}
                        </td>
                        <td className="px-3 py-2 text-center whitespace-nowrap">
                          {l.vendeu
                            ? <span className="inline-flex items-center gap-1 text-xs font-semibold text-emerald-700"><Check className="h-3.5 w-3.5" />{rotuloSim}</span>
                            : <span className="inline-flex items-center gap-1 text-xs font-semibold text-rose-700"><X className="h-3.5 w-3.5" />{rotuloNao}</span>}
                        </td>
                        <td className="px-3 py-2 text-center tabular-nums">{l.qtd ? fmt(l.qtd) : '—'}</td>
                        {comValor && <td className="px-3 py-2 text-center tabular-nums">{l.valor ? fmtBRL(l.valor) : '—'}</td>}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </TabelaComColunas>
            </>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
