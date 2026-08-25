import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, Building2, Download, FileSpreadsheet, FileText, RefreshCw } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'

// Relatório "Clientes com CNPJ irregular na Receita".
//
// POR QUE ELE EXISTE. A lista de clientes sem venda mistura duas coisas que
// pedem ações opostas: o concorrente levou (falha de venda, alguém age) e a
// loja fechou (não havia o que fazer). Amostra de 25/08/2026 em 40 clientes que
// compraram em 2025 e nada em 2026: cinco baixados ou inaptos, e nos cinco a
// mudança de situação cadastral cai na janela em que a compra parou.

interface Cliente {
  cnpj: string
  razao_social: string
  nome_cadastro: string
  situacao: string
  situacao_data: string
  cnae: string
  municipio: string
  uf: string
  ultima_compra: string
  liquido_ant: number
  liquido_atual: number
  nome_gerente: string
  nome_supervisor: string
  cod_rca: string
}

interface Resumo {
  situacao: string
  clientes: number
  liquido_ant: number
}

interface Relatorio {
  linhas: Cliente[] | null
  resumo: Resumo[] | null
  ano_anterior: number
  ano_atual: number
  gerado_em: string
  cobertura_pct: number
  incompleto: boolean
}

function fmtBRL(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', maximumFractionDigits: 0 })
}

function fmtCNPJ(d: string) {
  if (!d || d.length !== 14) return d
  return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8, 12)}-${d.slice(12)}`
}

// Cores literais, não construídas: o Tailwind varre o código-fonte para decidir
// que classes gerar, e uma string montada em tempo de execução não existe no CSS.
function corSituacao(s: string) {
  const t = (s || '').toUpperCase()
  if (t === 'BAIXADA') return 'bg-red-50 text-red-700 border-red-200'
  if (t === 'INAPTA') return 'bg-amber-50 text-amber-700 border-amber-200'
  if (t === 'SUSPENSA') return 'bg-orange-50 text-orange-700 border-orange-200'
  return 'bg-slate-50 text-slate-700 border-slate-200'
}

const SITUACOES = ['TODAS', 'BAIXADA', 'INAPTA', 'SUSPENSA', 'NULA'] as const

export default function FarolRelatorioReceita() {
  const { token } = useAuth()
  const [situacao, setSituacao] = useState<string>('TODAS')
  const [baixando, setBaixando] = useState<'pdf' | 'xlsx' | null>(null)

  const { data, isLoading, refetch, isFetching } = useQuery<Relatorio>({
    queryKey: ['relatorio-receita', situacao],
    queryFn: async () => {
      const res = await fetch(`/api/v2/farol/relatorio/clientes-receita?situacao=${situacao}`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error('Falha ao carregar o relatório')
      return res.json()
    },
  })

  // O download não pode ser um link simples: a rota exige o cabeçalho de
  // autorização, e <a href> não carrega cabeçalho. Buscamos o arquivo, viramos
  // blob e disparamos o clique.
  async function baixar(formato: 'pdf' | 'xlsx') {
    setBaixando(formato)
    try {
      const res = await fetch(
        `/api/v2/farol/relatorio/clientes-receita?situacao=${situacao}&formato=${formato}`,
        { headers: { Authorization: `Bearer ${token}` } },
      )
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `clientes-receita_${new Date().toISOString().slice(0, 10)}.${formato}`
      a.click()
      URL.revokeObjectURL(url)
      toast.success(`${formato.toUpperCase()} gerado`)
    } catch (e) {
      toast.error(`Não foi possível gerar o ${formato.toUpperCase()}`)
    } finally {
      setBaixando(null)
    }
  }

  const linhas = data?.linhas ?? []
  const resumo = data?.resumo ?? []
  const totalPerdido = resumo.reduce((s, r) => s + r.liquido_ant, 0)
  const totalClientes = resumo.reduce((s, r) => s + r.clientes, 0)

  return (
    <div className="space-y-6">
      <div className="bg-white rounded-xl border border-slate-200 p-6 shadow-sm">
        <div className="flex items-center gap-2 mb-1">
          <Building2 className="h-5 w-5 text-slate-600" />
          <h2 className="text-lg font-semibold text-slate-900">Clientes com CNPJ irregular na Receita</h2>
        </div>
        <p className="text-sm text-slate-500 mb-6">
          Separa o cliente que o concorrente levou daquele que deixou de existir. Quem está baixado ou
          inapto não é meta a recuperar — e visitar não adianta.
        </p>

        <div className="flex flex-wrap items-end gap-4">
          <div className="space-y-2">
            <Label>Situação cadastral</Label>
            <div className="flex flex-wrap gap-2">
              {SITUACOES.map(s => (
                <button
                  key={s}
                  onClick={() => setSituacao(s)}
                  className={
                    situacao === s
                      ? 'px-3 py-1.5 rounded-lg text-sm font-medium border bg-slate-900 text-white border-slate-900'
                      : 'px-3 py-1.5 rounded-lg text-sm font-medium border bg-white text-slate-600 border-slate-200 hover:bg-slate-50'
                  }
                >
                  {s === 'TODAS' ? 'Todas as irregulares' : s}
                </button>
              ))}
            </div>
          </div>
          <div className="ml-auto flex gap-2">
            <Button variant="outline" onClick={() => refetch()} disabled={isFetching}>
              <RefreshCw className={isFetching ? 'h-4 w-4 mr-2 animate-spin' : 'h-4 w-4 mr-2'} />
              Atualizar
            </Button>
            <Button variant="outline" onClick={() => baixar('xlsx')} disabled={!linhas.length || baixando !== null}>
              <FileSpreadsheet className="h-4 w-4 mr-2" />
              {baixando === 'xlsx' ? 'Gerando…' : 'Excel'}
            </Button>
            <Button onClick={() => baixar('pdf')} disabled={!linhas.length || baixando !== null}>
              <Download className="h-4 w-4 mr-2" />
              {baixando === 'pdf' ? 'Gerando…' : 'PDF'}
            </Button>
          </div>
        </div>
      </div>

      {/* A ressalva de cobertura vem ANTES dos números. Com a base parcialmente
          consultada, o total parece definitivo quando ainda é o começo. */}
      {data?.incompleto && (
        <div className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
          <AlertTriangle className="h-5 w-5 text-amber-600 shrink-0 mt-0.5" />
          <div className="text-sm text-amber-900">
            <strong>Consulta ainda parcial: {data.cobertura_pct.toFixed(0)}% da base.</strong>{' '}
            Os números abaixo são o que já foi conferido na Receita e tendem a crescer conforme a carga avança.
          </div>
        </div>
      )}

      {!isLoading && !linhas.length && (
        <div className="rounded-xl border border-slate-200 bg-white p-10 text-center">
          <FileText className="h-8 w-8 text-slate-300 mx-auto mb-3" />
          <p className="text-slate-600 font-medium">Nenhum cliente irregular encontrado</p>
          <p className="text-sm text-slate-400 mt-1">
            Ou a carga do cadastro da Receita ainda não rodou, ou todos os clientes consultados estão ativos.
          </p>
        </div>
      )}

      {linhas.length > 0 && (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="rounded-xl border border-slate-200 bg-white p-5">
              <div className="text-xs uppercase tracking-wide text-slate-500">Clientes irregulares</div>
              <div className="mt-1 text-3xl font-bold text-slate-900">{totalClientes.toLocaleString('pt-BR')}</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-5">
              <div className="text-xs uppercase tracking-wide text-slate-500">
                Faturado com eles em {data?.ano_anterior}
              </div>
              <div className="mt-1 text-3xl font-bold text-slate-900">{fmtBRL(totalPerdido)}</div>
            </div>
            {resumo.slice(0, 2).map(r => (
              <div key={r.situacao} className="rounded-xl border border-slate-200 bg-white p-5">
                <div className="text-xs uppercase tracking-wide text-slate-500">{r.situacao}</div>
                <div className="mt-1 text-3xl font-bold text-slate-900">{r.clientes.toLocaleString('pt-BR')}</div>
                <div className="text-sm text-slate-500">{fmtBRL(r.liquido_ant)}</div>
              </div>
            ))}
          </div>

          <div className="rounded-xl border border-slate-200 bg-white overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-slate-50 border-b border-slate-200">
                  <tr className="text-left text-xs uppercase tracking-wide text-slate-500">
                    <th className="px-4 py-3 font-medium">CNPJ</th>
                    <th className="px-4 py-3 font-medium">Cliente</th>
                    <th className="px-4 py-3 font-medium">Situação</th>
                    <th className="px-4 py-3 font-medium">Desde</th>
                    <th className="px-4 py-3 font-medium">Cidade</th>
                    <th className="px-4 py-3 font-medium">Últ. compra</th>
                    <th className="px-4 py-3 font-medium text-right">Líquido {data?.ano_anterior}</th>
                    <th className="px-4 py-3 font-medium text-right">Líquido {data?.ano_atual}</th>
                    <th className="px-4 py-3 font-medium">Supervisor</th>
                    <th className="px-4 py-3 font-medium">RCA</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {linhas.map(c => (
                    <tr key={c.cnpj} className="hover:bg-slate-50">
                      <td className="px-4 py-2.5 font-mono text-xs text-slate-600 whitespace-nowrap">
                        {fmtCNPJ(c.cnpj)}
                      </td>
                      <td className="px-4 py-2.5 text-slate-900">
                        {c.razao_social || c.nome_cadastro}
                        {c.cnae && <div className="text-xs text-slate-400">{c.cnae}</div>}
                      </td>
                      <td className="px-4 py-2.5">
                        <span className={`inline-block rounded-md border px-2 py-0.5 text-xs font-medium ${corSituacao(c.situacao)}`}>
                          {c.situacao}
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-slate-600 whitespace-nowrap">{c.situacao_data}</td>
                      <td className="px-4 py-2.5 text-slate-600 whitespace-nowrap">
                        {c.municipio}{c.uf ? `/${c.uf}` : ''}
                      </td>
                      <td className="px-4 py-2.5 text-slate-600 whitespace-nowrap">{c.ultima_compra}</td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-slate-900">{fmtBRL(c.liquido_ant)}</td>
                      <td className="px-4 py-2.5 text-right tabular-nums text-slate-500">{fmtBRL(c.liquido_atual)}</td>
                      <td className="px-4 py-2.5 text-slate-600">{c.nome_supervisor}</td>
                      <td className="px-4 py-2.5 text-slate-600">{c.cod_rca}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          <p className="text-xs text-slate-400">
            Fonte: Receita Federal via BrasilAPI (dump mensal). Gerado em {data?.gerado_em}.
          </p>
        </>
      )}
    </div>
  )
}
