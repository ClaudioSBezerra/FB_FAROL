import { useRef, useState } from 'react'
import { AlertTriangle, CheckCircle2, ChevronDown, Download, FileUp, Scale, Upload } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'

// Comparativo REL 322 (WinThor) x Farol x VM (base de origem).
//
// POR QUE ELE EXISTE. O gestor precisa validar se o Farol bate com o
// relatório oficial do WinThor. Sobe-se o PDF aqui; o Farol lê o período e a
// filial do próprio cabeçalho e cruza, por supervisor, o Vl.Vendido do
// relatório com o Líquido apurado no Farol E com o Líquido consultado AO VIVO
// na base Oracle de origem (VM) — a mesma que alimenta o Farol todo dia. As
// três diferenças percentuais (PDF×VM, PDF×Farol, VM×Farol) separam "o
// WinThor já gerou errado" de "o import pro Farol perdeu algo no caminho".
// Nada é salvo — cada upload é uma consulta independente.
//
// A consulta à VM é AO VIVO e pode levar até ~1-2 MINUTOS (custo fixo da
// view Oracle, independente do período) — o spinner avisa disso. Se a VM
// falhar (Oracle indisponível), o comparativo PDF×Farol continua valendo;
// só a coluna/diagnóstico da VM fica ausente.

interface LinhaComparativo {
  cod_supervisor: string
  supervisor: string
  vl_vendido_pdf: number | null
  liquido_farol: number | null
  liquido_vm: number | null
  diferenca_pdf_vm_pct: number | null
  diferenca_pdf_farol_pct: number | null
  diferenca_vm_farol_pct: number | null
  status: 'ok' | 'divergencia' | 'orfao'
  origem: 'pdf' | 'farol' | 'ambos'
}

type FluxoComparativo = 'faturado' | 'transmitido'

interface ComparativoResposta {
  periodo: string
  data_inicio: string
  data_fim: string
  fluxo: FluxoComparativo
  filiais: string[]
  linhas: LinhaComparativo[]
  total_vl_vendido_pdf: number
  total_liquido_farol: number
  total_liquido_vm: number | null
  total_diferenca_pdf_vm_pct: number | null
  total_diferenca_pdf_farol_pct: number | null
  total_diferenca_vm_farol_pct: number | null
  qtd_supervisores_pdf: number
  qtd_divergencias: number
  qtd_orfaos: number
  sem_dado_farol_no_periodo: boolean
  vm_indisponivel: boolean
  vm_erro?: string
}

// TIPO_VENDA_OPTIONS — espelha farol.tipo_venda_label (migration 191). Fixo
// porque é uma classificação de negócio rara de mudar; evita depender de um
// fetch de dims (que é por-período e some tipos sem movimento recente — aqui
// o objetivo é o oposto: deixar o gestor escolher QUALQUER tipo pra investigar).
const TIPO_VENDA_OPTIONS = [
  { key: '1',  label: 'Normal' },
  { key: '4',  label: 'Simples Fatura' },
  { key: '5',  label: 'Bonificação' },
  { key: '7',  label: 'Entrega Futura' },
  { key: '8',  label: 'Simples Entrega' },
  { key: '9',  label: 'CFOP Específico' },
  { key: '10', label: 'Transferência' },
  { key: '11', label: 'Venda com Troca' },
  { key: '13', label: 'Remessa Manifesto' },
  { key: '14', label: 'Venda Manifesto' },
  { key: '20', label: 'Consignada' },
]

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

function fluxoLabel(f: FluxoComparativo) {
  return f === 'transmitido' ? 'Transmitido' : 'Faturado'
}

// TipoVendaSelect — multi-seleção simples (só ~11 opções, sem busca).
function TipoVendaSelect({ selected, onChange }: { selected: string[]; onChange: (next: string[]) => void }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  function toggle(k: string) {
    onChange(selected.includes(k) ? selected.filter(x => x !== k) : [...selected, k])
  }

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className={`inline-flex items-center gap-1.5 px-3 py-2 text-sm font-medium border rounded-md bg-white shadow-sm ${
          selected.length > 0 ? 'border-slate-600 text-slate-900' : 'border-slate-300 text-slate-600 hover:bg-slate-50'
        }`}
      >
        Tipo de Venda
        {selected.length > 0 && (
          <span className="inline-flex items-center justify-center min-w-[18px] h-[18px] px-1 rounded-full bg-slate-800 text-white text-xs font-bold">
            {selected.length}
          </span>
        )}
        <ChevronDown className="h-3.5 w-3.5 opacity-60" />
      </button>
      {open && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setOpen(false)} />
          <div className="absolute left-0 top-full mt-1 z-50 w-64 bg-white border border-slate-200 rounded-md shadow-lg overflow-hidden">
            <div className="max-h-64 overflow-y-auto">
              {TIPO_VENDA_OPTIONS.map(opt => {
                const checked = selected.includes(opt.key)
                return (
                  <label key={opt.key} className="flex items-center gap-2 px-3 py-1.5 hover:bg-slate-50 cursor-pointer text-sm">
                    <input type="checkbox" checked={checked} onChange={() => toggle(opt.key)} className="w-3.5 h-3.5 accent-slate-700" />
                    <span className={checked ? 'font-medium text-slate-900' : 'text-slate-600'}>{opt.label}</span>
                  </label>
                )
              })}
            </div>
            {selected.length > 0 && (
              <div className="border-t border-slate-100 p-2">
                <button className="text-xs text-slate-500 hover:text-slate-800" onClick={() => onChange([])}>
                  Limpar seleção (volta ao padrão)
                </button>
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

export default function FarolComparativoRel322() {
  const { token } = useAuth()
  const inputRef = useRef<HTMLInputElement>(null)
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [carregando, setCarregando] = useState(false)
  const [baixandoPDF, setBaixandoPDF] = useState(false)
  const [erro, setErro] = useState<string | null>(null)
  const [resultado, setResultado] = useState<ComparativoResposta | null>(null)
  // Fluxo do PDF de origem — o WinThor não se autodeclara (o REL 322 tem o
  // mesmo layout nos dois casos), então é sempre escolha explícita do
  // usuário aqui. Default Faturado: quem não mexer no toggle mantém o
  // comportamento já em produção.
  const [fluxo, setFluxo] = useState<FluxoComparativo>('faturado')
  // Tipo de Venda — vazio = default do backend (tipoVendaReal no Faturado,
  // sem filtro no Transmitido). Seleção sobrescreve os dois.
  const [tipoVenda, setTipoVenda] = useState<string[]>([])

  function queryString() {
    const params = new URLSearchParams({ fluxo })
    if (tipoVenda.length > 0) params.set('tipo_venda', tipoVenda.join(','))
    return params.toString()
  }

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
      const res = await fetch(`/api/v2/farol/relatorio/comparativo-rel322?${queryString()}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
        body: form,
      })
      const body = await res.json().catch(() => null)
      if (!res.ok) {
        throw new Error(body?.error || `Falha ao processar o PDF (HTTP ${res.status})`)
      }
      setResultado(body as ComparativoResposta)
      if (body?.vm_indisponivel) {
        toast.warning('Base de origem (VM) indisponível agora — o comparativo PDF×Farol foi montado normalmente')
      }
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
  // baixarPDF — reenvia o RESULTADO JÁ CALCULADO (JSON) em vez de re-subir o
  // PDF do WinThor: o backend só desenha o PDF a partir desses dados, sem
  // reprocessar o upload nem refazer as consultas ao Farol/VM. Antes disso,
  // "Baixar PDF" pagava de novo o custo da consulta à VM (~1-2min fixo) só
  // pra gerar o mesmo resultado que a tela já mostrava — por isso exige um
  // "Comparar" primeiro (não dá mais pra baixar sem antes ver o resultado).
  async function baixarPDF() {
    if (!resultado) {
      toast.error('Clique em "Comparar" primeiro')
      return
    }
    setBaixandoPDF(true)
    try {
      const res = await fetch(`/api/v2/farol/relatorio/comparativo-rel322?formato=pdf`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify(resultado),
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
      // que o backend já monta com data_inicio/data_fim.
      const disposicao = res.headers.get('content-disposition') ?? ''
      const nomeMatch = disposicao.match(/filename="?([^"]+)"?/)
      const periodo = `${resultado.data_inicio}_a_${resultado.data_fim}`
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

  // trocarFluxo — trocar o toggle depois de já existir um resultado na tela
  // precisa limpar esse resultado (mesmo comportamento de trocar o arquivo):
  // senão o toggle mostraria um fluxo enquanto a tabela ainda reflete o
  // fluxo anterior, e um "Baixar PDF" nesse meio-tempo bateria com o toggle
  // (fluxo novo), não com o que está na tela (fluxo antigo) — inconsistência
  // que a spec proíbe explicitamente.
  function trocarFluxo(f: FluxoComparativo) {
    setFluxo(f)
    setResultado(null)
    setErro(null)
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
        <p className="text-sm text-slate-500 mb-4">
          Sobe o PDF que o WinThor exporta ("322 — Venda Por Departamento", por supervisor) e o Farol cruza,
          supervisor a supervisor, o Vl.Vendido do relatório com o Líquido apurado no Farol e com o Líquido consultado
          AO VIVO na base Oracle de origem (VM). Nada é salvo — cada upload é uma consulta independente.
        </p>

        <div className="mb-4 flex flex-wrap items-end gap-6">
          <div>
            <label className="block text-xs uppercase tracking-wide text-slate-500 font-semibold mb-1.5">
              Base do PDF gerado no WinThor
            </label>
            {/* Toggle Faturado x Transmitido — o WinThor não indica no PDF qual base foi
                usada pra gerar o REL 322, então a escolha é sempre explícita do usuário
                aqui (nunca inferida do conteúdo). Padrão visual copiado de
                FarolPublicPanel.tsx (mesmo toggle de fluxo usado no painel do RCA/Supervisor). */}
            <div className="inline-flex rounded-lg border-2 border-slate-300 overflow-hidden bg-white shadow-sm">
              <button
                type="button"
                onClick={() => trocarFluxo('faturado')}
                className={`px-4 py-2 text-sm font-bold uppercase tracking-wide transition-colors ${
                  fluxo === 'faturado' ? 'bg-slate-800 text-white' : 'text-slate-700 hover:bg-slate-50'
                }`}
              >
                Faturado
              </button>
              <button
                type="button"
                onClick={() => trocarFluxo('transmitido')}
                className={`px-4 py-2 text-sm font-bold uppercase tracking-wide transition-colors border-l-2 border-slate-300 ${
                  fluxo === 'transmitido' ? 'bg-emerald-700 text-white' : 'text-slate-700 hover:bg-slate-50'
                }`}
              >
                Transmitido
              </button>
            </div>
          </div>

          <div>
            <label className="block text-xs uppercase tracking-wide text-slate-500 font-semibold mb-1.5">
              Tipo de Venda (opcional)
            </label>
            <TipoVendaSelect selected={tipoVenda} onChange={vs => { setTipoVenda(vs); setResultado(null); setErro(null) }} />
          </div>
        </div>

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
            {carregando ? 'Consultando (PDF, Farol e base de origem)…' : 'Comparar'}
          </Button>
          <Button variant="outline" onClick={baixarPDF} disabled={!resultado || carregando || baixandoPDF} className="gap-2" title={!resultado ? 'Clique em "Comparar" primeiro' : undefined}>
            <Download className="h-4 w-4" />
            {baixandoPDF ? 'Gerando…' : 'Baixar PDF'}
          </Button>
        </div>
        {(carregando || baixandoPDF) && (
          <p className="mt-2 text-xs text-slate-400">
            A consulta à base de origem (VM) é ao vivo e pode levar até 1-2 minutos — é o custo fixo dessa consulta,
            não uma trava. Se ela falhar, o comparativo PDF×Farol aparece normalmente mesmo assim.
          </p>
        )}
      </div>

      {erro && (
        <div className="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 p-4">
          <AlertTriangle className="h-5 w-5 text-red-600 shrink-0 mt-0.5" />
          <div className="text-sm text-red-900">{erro}</div>
        </div>
      )}

      {resultado?.vm_indisponivel && (
        <div className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
          <AlertTriangle className="h-5 w-5 text-amber-600 shrink-0 mt-0.5" />
          <div className="text-sm text-amber-900">
            <strong>Base de origem (VM) indisponível nesta consulta.</strong>{' '}
            {resultado.vm_erro || 'Não foi possível consultar a VM agora.'} O comparativo PDF×Farol abaixo continua
            valendo normalmente — só as colunas de Líquido (VM) e as diferenças que dependem dela aparecem como "—".
          </div>
        </div>
      )}

      {resultado?.sem_dado_farol_no_periodo && (
        <div className="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4">
          <AlertTriangle className="h-5 w-5 text-amber-600 shrink-0 mt-0.5" />
          <div className="text-sm text-amber-900">
            <strong>O Farol não tem NENHUM dado importado no período {resultado.periodo}.</strong>{' '}
            Líquido aparece como R$ 0,00 para todas as linhas — isso reflete a realidade (range futuro ou período
            ainda não importado), não é um erro do comparativo.
          </div>
        </div>
      )}

      {resultado && (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <div className="rounded-xl border border-slate-200 bg-white px-4 py-2.5">
              <div className="text-xs uppercase tracking-wide text-slate-500">Período (do PDF)</div>
              <div className="text-sm font-bold text-slate-900">{resultado.periodo}</div>
            </div>
            <span
              className={`inline-flex items-center rounded-md border px-2 py-1 text-xs font-semibold uppercase tracking-wide ${
                resultado.fluxo === 'transmitido'
                  ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
                  : 'bg-slate-100 text-slate-700 border-slate-200'
              }`}
            >
              Fluxo: {fluxoLabel(resultado.fluxo)}
            </span>
            {resultado.filiais.length > 0 && (
              <span className="text-xs text-slate-500">Filial(is): {resultado.filiais.join(', ')}</span>
            )}
            <span className="text-xs text-slate-500 ml-auto">
              {resultado.qtd_supervisores_pdf} supervisores no PDF · {resultado.qtd_divergencias} divergência(s) · {resultado.qtd_orfaos} órfã(s)
            </span>
          </div>

          {/* Quadro comparativo TOTAL — mesma estrutura das linhas (3 valores +
              3 diferenças), sobre a soma de tudo: é o primeiro número que o
              gestor bate contra o "N Supervisores Listados" do WinThor. */}
          <div className="rounded-xl border border-slate-200 bg-white p-5">
            <div className="text-xs uppercase tracking-wide text-slate-500 font-semibold mb-3">Totais</div>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-4">
              <div>
                <div className="text-xs text-slate-500">Vl.Vendido (PDF)</div>
                <div className="text-2xl font-bold text-slate-900">{fmtBRL(resultado.total_vl_vendido_pdf)}</div>
              </div>
              <div>
                <div className="text-xs text-slate-500">Líquido (Farol)</div>
                <div className="text-2xl font-bold text-slate-900">{fmtBRL(resultado.total_liquido_farol)}</div>
              </div>
              <div>
                <div className="text-xs text-slate-500">Líquido (VM)</div>
                <div className="text-2xl font-bold text-slate-900">{fmtBRL(resultado.total_liquido_vm)}</div>
              </div>
            </div>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 pt-3 border-t border-slate-100">
              <div>
                <div className="text-xs text-slate-500">% PDF × VM</div>
                <div className="text-lg font-semibold text-slate-700">{fmtPct(resultado.total_diferenca_pdf_vm_pct)}</div>
              </div>
              <div>
                <div className="text-xs text-slate-500">% PDF × Farol</div>
                <div className="text-lg font-semibold text-slate-700">{fmtPct(resultado.total_diferenca_pdf_farol_pct)}</div>
              </div>
              <div>
                <div className="text-xs text-slate-500">% VM × Farol</div>
                <div className="text-lg font-semibold text-slate-700">{fmtPct(resultado.total_diferenca_vm_farol_pct)}</div>
              </div>
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
                    <th className="px-4 py-3 font-medium text-right">Líquido (Farol)</th>
                    <th className="px-4 py-3 font-medium text-right">Líquido (VM)</th>
                    <th className="px-4 py-3 font-medium text-right">% PDF×VM</th>
                    <th className="px-4 py-3 font-medium text-right">% PDF×Farol</th>
                    <th className="px-4 py-3 font-medium text-right">% VM×Farol</th>
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
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtBRL(l.liquido_farol)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtBRL(l.liquido_vm)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-500">{fmtPct(l.diferenca_pdf_vm_pct)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-500">{fmtPct(l.diferenca_pdf_farol_pct)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-500">{fmtPct(l.diferenca_vm_farol_pct)}</td>
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
            {resultado.fluxo === 'transmitido' ? (
              <>
                Fluxo Transmitido: Líquido (Farol) = soma de todos os pedidos transmitidos no período menos Cortado
                (vendas perdidas) — pode vir negativo quando o Cortado supera o total. Uma linha é "OK" quando
                Líquido (Farol) está a até 0,5% do Vl.Vendido do PDF.
              </>
            ) : (
              <>
                Fluxo Faturado: Líquido (Farol) = venda real (por padrão exclui bonificação, transferência e remessa —
                ajustável pelo filtro Tipo de Venda acima) menos devolvido/cancelado. Uma linha é "OK" quando Líquido
                (Farol) está a até 0,5% do Vl.Vendido do PDF.
              </>
            )}
            {' '}Líquido (VM) vem AO VIVO da base de origem (WinThor/Oracle) — é diagnóstico adicional e nunca decide
            o Status. % PDF×VM e % VM×Farol ajudam a isolar se uma divergência já vem do próprio relatório WinThor ou
            surgiu na importação pro Farol.
          </p>
        </>
      )}
    </div>
  )
}
