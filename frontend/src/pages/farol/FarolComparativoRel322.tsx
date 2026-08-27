import { useRef, useState } from 'react'
import { AlertTriangle, CheckCircle2, Download, FileUp, Scale, Upload } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'

// Comparativo REL 322 (WinThor) x Farol.
//
// POR QUE ELE EXISTE. O gestor precisa validar se o Farol bate com o
// relatório oficial do WinThor, mas de onde ele está não alcança nem o
// Oracle de origem nem o Postgres de produção — só o PDF que o WinThor já
// exporta está disponível. Sobe-se o PDF aqui; o Farol lê o período do
// próprio cabeçalho e cruza Bruto/Líquido por supervisor. Nada fica salvo:
// cada upload é uma consulta independente.

interface LinhaComparativo {
  cod_supervisor: string
  supervisor: string
  vl_vendido_pdf: number | null
  bruto_farol: number | null
  liquido_farol: number | null
  diferenca_pct: number | null
  status: 'ok' | 'divergencia' | 'orfao'
  origem: 'pdf' | 'farol' | 'ambos'
}

interface ComparativoResposta {
  periodo: string
  data_inicio: string
  data_fim: string
  linhas: LinhaComparativo[]
  total_vl_vendido_pdf: number
  total_bruto_farol: number
  total_liquido_farol: number
  qtd_supervisores_pdf: number
  qtd_divergencias: number
  qtd_orfaos: number
  sem_dado_farol_no_periodo: boolean
}

function fmtBRL(v: number | null) {
  if (v === null) return '—'
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function fmtPct(v: number | null) {
  if (v === null) return '—'
  return `${v.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}%`
}

function seloStatus(l: LinhaComparativo) {
  if (l.status === 'orfao') {
    const lado = l.origem === 'pdf' ? 'só no PDF' : 'só no Farol'
    return { texto: `Órfã — ${lado}`, classe: 'bg-amber-50 text-amber-700 border-amber-200' }
  }
  if (l.status === 'ok') {
    return { texto: 'OK', classe: 'bg-emerald-50 text-emerald-700 border-emerald-200' }
  }
  return { texto: 'Divergência', classe: 'bg-red-50 text-red-700 border-red-200' }
}

function linhaFundo(l: LinhaComparativo) {
  if (l.status === 'orfao') return 'bg-amber-50/40'
  if (l.status === 'divergencia') return 'bg-red-50/40'
  return ''
}

export default function FarolComparativoRel322() {
  const { token } = useAuth()
  const inputRef = useRef<HTMLInputElement>(null)
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [carregando, setCarregando] = useState(false)
  const [baixandoPDF, setBaixandoPDF] = useState(false)
  const [erro, setErro] = useState<string | null>(null)
  const [resultado, setResultado] = useState<ComparativoResposta | null>(null)

  async function enviar() {
    if (!arquivo) {
      toast.error('Selecione o PDF do REL 322 (WinThor)')
      return
    }
    setCarregando(true)
    setErro(null)
    setResultado(null)
    try {
      const form = new FormData()
      form.append('file', arquivo)
      const res = await fetch('/api/v2/farol/relatorio/comparativo-rel322', {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: form,
      })
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error(body?.error || `Falha ao processar o PDF (HTTP ${res.status})`)
      }
      setResultado(body as ComparativoResposta)
    } catch (e: any) {
      setErro(e?.message || 'Falha ao processar o PDF')
      toast.error('Não foi possível montar o comparativo')
    } finally {
      setCarregando(false)
    }
  }

  // baixarPDF — reenvia o MESMO arquivo já selecionado com ?formato=pdf: o
  // backend reprocessa o upload no mesmo request (nada fica salvo, mesmo
  // padrão do resto da feature) e devolve o PDF do RESULTADO do comparativo.
  // Não é um <a href> simples: a rota exige o cabeçalho de autorização.
  async function baixarPDF() {
    if (!arquivo) {
      toast.error('Selecione o PDF do REL 322 (WinThor)')
      return
    }
    setBaixandoPDF(true)
    try {
      const form = new FormData()
      form.append('file', arquivo)
      const res = await fetch('/api/v2/farol/relatorio/comparativo-rel322?formato=pdf', {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: form,
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        throw new Error(body?.error || `Falha ao gerar o PDF (HTTP ${res.status})`)
      }
      const blob = await res.blob()
      if (blob.size === 0) {
        throw new Error('PDF vazio devolvido pelo servidor')
      }
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      // O nome inclui o período do PDF de origem — vem do Content-Disposition
      // que o backend já monta com data_inicio/data_fim, então funciona mesmo
      // se "Baixar PDF" for clicado antes de "Comparar" (resultado ainda nulo).
      const disposicao = res.headers.get('content-disposition') ?? ''
      const nomeMatch = disposicao.match(/filename="?([^"]+)"?/)
      const periodo = resultado ? `${resultado.data_inicio}_a_${resultado.data_fim}` : new Date().toISOString().slice(0, 10)
      a.download = nomeMatch?.[1] || `comparativo-rel322_${periodo}.pdf`
      // Precisa estar no DOM antes do clique — em alguns navegadores um <a>
      // solto não dispara o download. Revoga a URL blob: um pouco depois
      // (não na hora): revogar cedo demais pode cancelar o download em
      // navegadores que resolvem a URL de forma assíncrona.
      document.body.appendChild(a)
      a.click()
      a.remove()
      setTimeout(() => URL.revokeObjectURL(url), 1000)
      toast.success('PDF gerado')
    } catch (e: any) {
      toast.error(e?.message || 'Não foi possível gerar o PDF')
    } finally {
      setBaixandoPDF(false)
    }
  }

  function onSelecionarArquivo(files: FileList | null) {
    const f = files?.[0] ?? null
    // O <input accept="application/pdf"> só filtra no caminho de clicar e
    // escolher — arrastar qualquer arquivo (pasta, imagem etc.) passa direto
    // e só falharia depois de um round-trip pro servidor.
    if (f && f.type !== 'application/pdf') {
      toast.error('Envie o PDF do REL 322 (WinThor)')
      return
    }
    setArquivo(f)
    setResultado(null)
    setErro(null)
  }

  const linhas = resultado?.linhas ?? []

  return (
    <div className="space-y-6">
      <div className="bg-white rounded-xl border border-slate-200 p-6 shadow-sm">
        <div className="flex items-center gap-2 mb-1">
          <Scale className="h-5 w-5 text-slate-600" />
          <h2 className="text-lg font-semibold text-slate-900">Comparativo REL 322 (WinThor) x Farol</h2>
        </div>
        <p className="text-sm text-slate-500 mb-6">
          Sobe o PDF que o WinThor exporta ("322 — Venda Por Departamento", por supervisor) e o Farol cruza,
          supervisor a supervisor, o Vl.Vendido do relatório com o Bruto e o Líquido apurados no mesmo período.
          Nada é salvo — cada upload é uma consulta independente.
        </p>

        <div
          className="flex flex-wrap items-center gap-4 rounded-lg border border-dashed border-slate-300 bg-slate-50 p-4"
          onDragOver={e => e.preventDefault()}
          onDrop={e => {
            e.preventDefault()
            onSelecionarArquivo(e.dataTransfer.files)
          }}
        >
          <FileUp className="h-8 w-8 text-slate-400 shrink-0" />
          <div className="flex-1 min-w-[200px]">
            <div className="text-sm font-medium text-slate-700">
              {arquivo ? arquivo.name : 'Arraste o PDF aqui ou clique para escolher'}
            </div>
            <div className="text-xs text-slate-400">Só o PDF gerado pelo WinThor (texto real, não digitalizado)</div>
          </div>
          <input
            ref={inputRef}
            type="file"
            accept="application/pdf"
            className="hidden"
            onChange={e => onSelecionarArquivo(e.target.files)}
          />
          <Button variant="outline" onClick={() => inputRef.current?.click()}>
            Escolher arquivo
          </Button>
          <Button onClick={enviar} disabled={!arquivo || carregando || baixandoPDF} className="gap-2">
            <Upload className="h-4 w-4" />
            {carregando ? 'Processando…' : 'Comparar'}
          </Button>
          <Button variant="outline" onClick={baixarPDF} disabled={!arquivo || carregando || baixandoPDF} className="gap-2">
            <Download className="h-4 w-4" />
            {baixandoPDF ? 'Gerando…' : 'Baixar PDF'}
          </Button>
        </div>
      </div>

      {erro && (
        <div className="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 p-4">
          <AlertTriangle className="h-5 w-5 text-red-600 shrink-0 mt-0.5" />
          <div className="text-sm text-red-900">{erro}</div>
        </div>
      )}

      {resultado?.sem_dado_farol_no_periodo && (
        <div className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
          <AlertTriangle className="h-5 w-5 text-amber-600 shrink-0 mt-0.5" />
          <div className="text-sm text-amber-900">
            <strong>O Farol não tem NENHUM dado importado no período {resultado.periodo}.</strong>{' '}
            Bruto e Líquido aparecem como R$ 0,00 para todas as linhas — isso reflete a realidade (range futuro
            ou período ainda não importado), não é um erro do comparativo.
          </div>
        </div>
      )}

      {resultado && (
        <>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
            <div className="rounded-xl border border-slate-200 bg-white p-5">
              <div className="text-xs uppercase tracking-wide text-slate-500">Período (do PDF)</div>
              <div className="mt-1 text-lg font-bold text-slate-900">{resultado.periodo}</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-5">
              <div className="text-xs uppercase tracking-wide text-slate-500">Total Vl.Vendido (PDF)</div>
              <div className="mt-1 text-2xl font-bold text-slate-900">{fmtBRL(resultado.total_vl_vendido_pdf)}</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-5">
              <div className="text-xs uppercase tracking-wide text-slate-500">Total Bruto (Farol)</div>
              <div className="mt-1 text-2xl font-bold text-slate-900">{fmtBRL(resultado.total_bruto_farol)}</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-5">
              <div className="text-xs uppercase tracking-wide text-slate-500">Total Líquido (Farol)</div>
              <div className="mt-1 text-2xl font-bold text-slate-900">{fmtBRL(resultado.total_liquido_farol)}</div>
            </div>
            <div className="rounded-xl border border-slate-200 bg-white p-5">
              <div className="text-xs uppercase tracking-wide text-slate-500">Divergências / Órfãs</div>
              <div className="mt-1 text-2xl font-bold text-slate-900">
                {resultado.qtd_divergencias} / {resultado.qtd_orfaos}
              </div>
              <div className="text-xs text-slate-400">{resultado.qtd_supervisores_pdf} supervisores no PDF</div>
            </div>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-slate-50 border-b border-slate-200">
                  <tr className="text-left text-xs uppercase tracking-wide text-slate-500">
                    <th className="px-4 py-3 font-medium">Código</th>
                    <th className="px-4 py-3 font-medium">Supervisor</th>
                    <th className="px-4 py-3 font-medium text-right">Vl.Vendido (PDF)</th>
                    <th className="px-4 py-3 font-medium text-right">Bruto (Farol)</th>
                    <th className="px-4 py-3 font-medium text-right">Líquido (Farol)</th>
                    <th className="px-4 py-3 font-medium text-right">% diferença</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {linhas.map(l => {
                    const selo = seloStatus(l)
                    return (
                      <tr key={`${l.cod_supervisor}-${l.origem}`} className={`hover:bg-slate-50 ${linhaFundo(l)}`}>
                        <td className="px-4 py-2.5 font-mono text-xs text-slate-600 whitespace-nowrap">{l.cod_supervisor}</td>
                        <td className="px-4 py-2.5 text-slate-900">{l.supervisor || '—'}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-900">{fmtBRL(l.vl_vendido_pdf)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtBRL(l.bruto_farol)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtBRL(l.liquido_farol)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-500">{fmtPct(l.diferenca_pct)}</td>
                        <td className="px-4 py-2.5">
                          <span className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium ${selo.classe}`}>
                            {l.status === 'ok' && <CheckCircle2 className="h-3 w-3" />}
                            {selo.texto}
                          </span>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>

          <p className="text-xs text-slate-400">
            Bruto = soma de todas as vendas faturadas no período. Líquido = venda real (exclui bonificação,
            transferência e remessa) menos devolvido/cancelado — mesma composição do painel Faturado. Uma linha
            é "OK" quando Bruto OU Líquido do Farol está a até 0,5% do Vl.Vendido do PDF.
          </p>
        </>
      )}
    </div>
  )
}
