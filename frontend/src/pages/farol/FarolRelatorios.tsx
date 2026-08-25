import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Search, FileText, Download, Calendar } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import FarolRelatorioReceita from './FarolRelatorioReceita'

// Função para formatar data para input date (YYYY-MM-DD)
function formatDateForInput(date: Date): string {
  return date.toISOString().split('T')[0]
}

// Função para obter data de 1 ano atrás
function getOneYearAgo(): string {
  const date = new Date()
  date.setFullYear(date.getFullYear() - 1)
  return formatDateForInput(date)
}

// Função para obter data de hoje
function getToday(): string {
  return formatDateForInput(new Date())
}

// ─── Types ────────────────────────────────────────────────────────────────────

interface Venda {
  ano: number
  mes: number
  periodo: string
  nome_prod: string
  nome_cli: string
  cnpj: string
  cod_fornec: string
  nome_fornec: string
  qt_nfs: number
  qt_total: number
  valor_venda: number
  valor_lucro: number
  ticket_medio_nf: number
  preco_medio_unit: number
}

interface RelatorioResponse {
  dados: Venda[]
  produto: { cod: string; nome: string }
  cliente: { cod: string; nome: string }
}

// ─── Utils ────────────────────────────────────────────────────────────────────

function fmtBRL(v: number) {
  if (v === 0) return 'R$ 0,00'
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function fmtNum(v: number) {
  return v.toLocaleString('pt-BR', { maximumFractionDigits: 2 })
}

function fmtInt(v: number) {
  return v.toLocaleString('pt-BR', { maximumFractionDigits: 0 })
}

// ─── Component ─────────────────────────────────────────────────────────────────

export default function FarolRelatorios() {
  const { token } = useAuth()
  const [codProduto, setCodProduto] = useState('')
  const [codCliente, setCodCliente] = useState('')
  const [dataInicio, setDataInicio] = useState(getOneYearAgo())
  const [dataFim, setDataFim] = useState(getToday())
  const [searchExecuted, setSearchExecuted] = useState(false)
  const [aba, setAba] = useState<'extrato' | 'receita'>('extrato')

  const { data, isLoading, refetch } = useQuery<RelatorioResponse>({
    queryKey: ['relatorio-extrato', codProduto, codCliente, dataInicio, dataFim],
    queryFn: async () => {
      if (!codProduto || !codCliente) return { dados: [], produto: { cod: '', nome: '' }, cliente: { cod: '', nome: '' } }
      const params = new URLSearchParams({
        cod_produto: codProduto,
        cod_cliente: codCliente,
        data_inicio: dataInicio,
        data_fim: dataFim,
      })
      const res = await fetch(`/api/v2/farol/relatorio/extrato-produto-cliente?${params}`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error('Erro ao buscar dados')
      return res.json()
    },
    enabled: searchExecuted && !!codProduto && !!codCliente,
  })

  const handleSearch = () => {
    if (!codProduto.trim()) {
      toast.error('Informe o código do produto')
      return
    }
    if (!codCliente.trim()) {
      toast.error('Informe o código do cliente')
      return
    }
    setSearchExecuted(true)
    refetch()
  }

  const handleExport = () => {
    if (!data?.dados.length) return
    const headers = ['Ano', 'Mês', 'Período', 'Produto', 'Cliente', 'CNPJ', 'Fornecedor', 'Qtd NFs', 'Qtd Total', 'Valor Venda', 'Valor Lucro', 'Ticket Médio', 'Preço Médio Unit']
    const rows = data.dados.map(d => [
      d.ano,
      d.mes,
      d.periodo,
      d.nome_prod,
      d.nome_cli,
      d.cnpj,
      d.nome_fornec,
      d.qt_nfs,
      d.qt_total,
      d.valor_venda,
      d.valor_lucro,
      d.ticket_medio_nf,
      d.preco_medio_unit,
    ])
    const csv = [headers, ...rows].map(row => row.map(cell => `"${cell}"`).join(',')).join('\n')
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `extrato_${codProduto}_${codCliente}_${Date.now()}.csv`
    link.click()
    URL.revokeObjectURL(url)
    toast.success('Arquivo exportado com sucesso')
  }

  // Totais
  const totals = data?.dados.reduce(
    (acc, d) => ({
      qt_nfs: acc.qt_nfs + d.qt_nfs,
      qt_total: acc.qt_total + d.qt_total,
      valor_venda: acc.valor_venda + d.valor_venda,
      valor_lucro: acc.valor_lucro + d.valor_lucro,
    }),
    { qt_nfs: 0, qt_total: 0, valor_venda: 0, valor_lucro: 0 }
  )

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Relatórios</h1>
          <p className="text-sm text-slate-500">Relatórios gerenciais e operacionais</p>
        </div>
      </div>

      {/* Seleção do relatório */}
      <div className="flex gap-2 border-b border-slate-200">
        {([
          { id: 'extrato', label: 'Extrato de Produtos por Cliente' },
          { id: 'receita', label: 'Clientes com CNPJ irregular' },
        ] as const).map(t => (
          <button
            key={t.id}
            onClick={() => setAba(t.id)}
            className={
              aba === t.id
                ? '-mb-px px-4 py-2.5 text-sm font-medium border-b-2 border-slate-900 text-slate-900'
                : '-mb-px px-4 py-2.5 text-sm font-medium border-b-2 border-transparent text-slate-500 hover:text-slate-700'
            }
          >
            {t.label}
          </button>
        ))}
      </div>

      {aba === 'receita' && <FarolRelatorioReceita />}

      {aba === 'extrato' && (<>
      {/* Filtros */}
      <div className="bg-white rounded-xl border border-slate-200 p-6 shadow-sm">
        <div className="flex items-center gap-2 mb-4">
          <FileText className="h-5 w-5 text-slate-600" />
          <h2 className="text-lg font-semibold text-slate-900">Extrato de Produtos por Cliente</h2>
        </div>
        <p className="text-sm text-slate-500 mb-6">
          Análise detalhada de vendas de um produto específico para um cliente, com histórico mês a mês.
        </p>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-4">
          <div className="space-y-2">
            <Label htmlFor="cod_produto">Código do Produto</Label>
            <Input
              id="cod_produto"
              placeholder="Ex: 467601"
              value={codProduto}
              onChange={e => setCodProduto(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="cod_cliente">Código do Cliente</Label>
            <Input
              id="cod_cliente"
              placeholder="Ex: 184264"
              value={codCliente}
              onChange={e => setCodCliente(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="data_inicio">Data Início</Label>
            <Input
              id="data_inicio"
              type="date"
              value={dataInicio}
              onChange={e => setDataInicio(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="data_fim">Data Fim</Label>
            <Input
              id="data_fim"
              type="date"
              value={dataFim}
              onChange={e => setDataFim(e.target.value)}
            />
          </div>
          <div className="flex items-end">
            <Button onClick={handleSearch} className="w-full gap-2">
              <Search className="h-4 w-4" />
              Buscar
            </Button>
          </div>
        </div>
      </div>

      {/* Resultados */}
      {searchExecuted && (
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          {isLoading ? (
            <div className="p-8 text-center text-slate-500">Carregando...</div>
          ) : !data?.dados.length ? (
            <div className="p-8 text-center text-slate-500">
              Nenhum dado encontrado para o produto/cliente informado.
            </div>
          ) : (
            <>
              {/* Info do produto/cliente */}
              <div className="bg-slate-50 px-6 py-4 border-b border-slate-200 flex flex-wrap items-center justify-between gap-4">
                <div className="space-y-1">
                  <div className="text-xs text-slate-500">Produto</div>
                  <div className="font-semibold text-slate-900">
                    {data.produto.cod} - {data.produto.nome}
                  </div>
                </div>
                <div className="space-y-1">
                  <div className="text-xs text-slate-500">Cliente</div>
                  <div className="font-semibold text-slate-900">
                    {data.cliente.cod} - {data.cliente.nome}
                  </div>
                </div>
                <Button onClick={handleExport} size="sm" variant="outline" className="gap-2">
                  <Download className="h-4 w-4" />
                  Exportar CSV
                </Button>
              </div>

              {/* Totais */}
              <div className="bg-slate-900 text-white px-6 py-3 grid grid-cols-4 gap-4 text-sm">
                <div>
                  <div className="text-slate-400 text-xs">Total Venda</div>
                  <div className="font-bold">{fmtBRL(totals!.valor_venda)}</div>
                </div>
                <div>
                  <div className="text-slate-400 text-xs">Total Lucro</div>
                  <div className="font-bold">{fmtBRL(totals!.valor_lucro)}</div>
                </div>
                <div>
                  <div className="text-slate-400 text-xs">Qtd. NFs</div>
                  <div className="font-bold">{fmtInt(totals!.qt_nfs)}</div>
                </div>
                <div>
                  <div className="text-slate-400 text-xs">Qtd. Itens</div>
                  <div className="font-bold">{fmtNum(totals!.qt_total)}</div>
                </div>
              </div>

              {/* Tabela */}
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="bg-slate-50 border-b border-slate-200">
                    <tr>
                      <th className="px-4 py-3 text-left font-semibold text-slate-700">Período</th>
                      <th className="px-4 py-3 text-left font-semibold text-slate-700">Fornecedor</th>
                      <th className="px-4 py-3 text-right font-semibold text-slate-700">NFs</th>
                      <th className="px-4 py-3 text-right font-semibold text-slate-700">Qtd.</th>
                      <th className="px-4 py-3 text-right font-semibold text-slate-700">Venda</th>
                      <th className="px-4 py-3 text-right font-semibold text-slate-700">Lucro</th>
                      <th className="px-4 py-3 text-right font-semibold text-slate-700">Ticket Médio</th>
                      <th className="px-4 py-3 text-right font-semibold text-slate-700">Preço Unit.</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {data.dados.map((d, i) => (
                      <tr key={i} className="hover:bg-slate-50">
                        <td className="px-4 py-3">
                          <div className="font-medium text-slate-900">{d.periodo}</div>
                          <div className="text-xs text-slate-500">{d.ano}/{d.mes}</div>
                        </td>
                        <td className="px-4 py-3">
                          <div className="text-xs text-slate-500">{d.cod_fornec}</div>
                          <div className="text-slate-700 truncate max-w-[150px]" title={d.nome_fornec}>{d.nome_fornec}</div>
                        </td>
                        <td className="px-4 py-3 text-right tabular-nums">{fmtInt(d.qt_nfs)}</td>
                        <td className="px-4 py-3 text-right tabular-nums">{fmtNum(d.qt_total)}</td>
                        <td className="px-4 py-3 text-right font-medium tabular-nums">{fmtBRL(d.valor_venda)}</td>
                        <td className="px-4 py-3 text-right tabular-nums">{fmtBRL(d.valor_lucro)}</td>
                        <td className="px-4 py-3 text-right tabular-nums">{fmtBRL(d.ticket_medio_nf)}</td>
                        <td className="px-4 py-3 text-right tabular-nums">{fmtBRL(d.preco_medio_unit)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </div>
      )}
      </>)}
    </div>
  )
}
