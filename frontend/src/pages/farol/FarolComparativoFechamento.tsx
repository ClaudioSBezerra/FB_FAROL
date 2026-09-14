import { useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CheckCircle2, Download, FileUp, Scale, Upload } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'sonner'
import { exportToExcel } from '@/lib/exportToExcel'
import { BotaoComoFunciona } from '@/components/ComoFuncionaIndicadores'
import type * as XLSXType from 'xlsx'

// Comparativo Fechamento Comercial (Painel Vendas) — pedido do Claudio
// 14/09/2026, depois de uma sessão inteira validando manualmente (via script
// ad-hoc + Excel) se o fechamento que o fornecedor manda por fora (ex:
// Carlos/JC) batia com o motor do Farol (Painel de Objetivos por
// Indústria). Mesmo espírito do Comparativo REL 322 (FarolComparativoRel322.tsx,
// WinThor × Farol × VM): sobe um arquivo, o Farol cruza linha a linha contra
// a apuração oficial dele mesmo e classifica OK/Divergência/Órfã.
//
// Diferença de fonte: aqui não é upload-e-descarta (REL 322) — o fechamento
// importado FICA GRAVADO (farol.fechamento_comercial_externo), porque o
// pedido explícito era reaproveitar isso como comparativo permanente, não
// uma consulta avulsa. Reimportar substitui (mesmo PUT-replace de
// Itens/Clientes Válidos).
//
// Formato de entrada: aba "Resumo Redes" do modelo real da JC (mesmo
// arquivo que já alimenta Itens/Clientes Válidos, ver ConfigMetasVinculos.tsx)
// — cada Rede aparece 2x quando a empresa tem 2+ indústrias no programa (ex:
// Unilever HC + Foods), uma linha por indústria, diferenciadas só pelo
// "Objetivo Cobertura". A separação acontece AQUI (client-side), casando
// esse valor com o limiar_valor_medio cadastrado em cada vínculo de
// Cobertura — sem isso a 1ª tentativa manual desta investigação comparou
// HC do Farol com Foods do fornecedor por engano (achado real, 14/09/2026).

interface MetaVinculo {
  id: number
  industria_id: number
  industria_nome: string
  formula_codigo: string
  parametros_valores: Record<string, unknown>
}

interface ComparativoLinha {
  cod_princ: string
  razao: string
  fantasia: string
  qt_lojas: number
  cod_ggv: string
  nome_ggv: string
  cod_crv: string
  nome_crv: string
  cod_rca: string
  nome_rca: string
  valor_venda_externo: number
  valor_venda_farol: number
  diferenca_valor: number
  diferenca_valor_pct: number
  eans_externo: number
  eans_farol: number
  diferenca_eans: number
  status: 'OK' | 'DIVERGE' | 'SO_EXTERNO' | 'SO_FAROL'
}

const RESUMO_REDES_COLS: { field: string; candidates: string[] }[] = [
  { field: 'cod_princ', candidates: ['COD PRINC', 'COD_PRINC', 'CODPRINC'] },
  { field: 'razao', candidates: ['RAZAO', 'RAZÃO'] },
  { field: 'fantasia', candidates: ['FANTASIA'] },
  { field: 'qt_lojas', candidates: ['QT LOJAS', 'QT_LOJAS'] },
  { field: 'objetivo_cobertura', candidates: ['OBJETIVO COBERTURA'] },
  { field: 'valor_venda', candidates: ['VALOR VENDA'] },
  { field: 'objetivo_eans', candidates: ['OBJETIVO EANS'] },
  { field: 'qt_eans_vendidos', candidates: ['QT MEDIA DE EANS VENDIDOS'] },
  { field: 'cod_ggv', candidates: ['COD GGV'] },
  { field: 'nome_ggv', candidates: ['NOME GGV'] },
  { field: 'cod_crv', candidates: ['COD CRV'] },
  { field: 'nome_crv', candidates: ['NOME CRV'] },
  { field: 'cod_rca', candidates: ['COD RCA'] },
  { field: 'nome_rca', candidates: ['NOME RCA'] },
]

function normalizaHeader(s: unknown): string {
  return String(s ?? '').trim().toUpperCase().replace(/\s+/g, ' ')
}

function csvEscape(v: string): string {
  if (/[;"\n\r]/.test(v)) return '"' + v.replace(/"/g, '""') + '"'
  return v
}

// parseFechamentoComercialXlsx — lê a aba "Resumo Redes" no navegador,
// separa as linhas duplicadas por indústria (casando Objetivo Cobertura com
// o limiar_valor_medio de cada vínculo de Cobertura) e devolve o CSV
// (industria_id incluso) que o endpoint de importação espera.
async function parseFechamentoComercialXlsx(file: File, vinculosCobertura: MetaVinculo[]): Promise<string> {
  const XLSX = await import('xlsx')
  const buf = await file.arrayBuffer()
  const wb = XLSX.read(buf, { type: 'array', cellText: false, cellDates: false })
  const abaNome = wb.SheetNames.find(n => normalizaHeader(n).includes('RESUMO REDES')) ?? wb.SheetNames[0]
  if (!abaNome) throw new Error('Planilha sem nenhuma aba — arquivo .xlsx inválido')
  const sheet = wb.Sheets[abaNome]
  const linhas = XLSX.utils.sheet_to_json<unknown[]>(sheet, { header: 1, raw: true, defval: '' })
  if (linhas.length === 0) throw new Error(`Aba "${abaNome}" está vazia`)

  const headerRow = (linhas[0] as unknown[]).map(normalizaHeader)
  const idxPorCampo: Record<string, number> = {}
  const faltando: string[] = []
  for (const { field, candidates } of RESUMO_REDES_COLS) {
    const idx = headerRow.findIndex(h => candidates.some(c => h.startsWith(c)))
    if (idx === -1) {
      if (field !== 'cod_princ' && field !== 'valor_venda' && field !== 'qt_eans_vendidos' && field !== 'objetivo_cobertura') continue
      faltando.push(candidates[0])
      continue
    }
    idxPorCampo[field] = idx
  }
  if (faltando.length > 0) {
    throw new Error(`Aba "${abaNome}" sem a(s) coluna(s): ${faltando.join(', ')}`)
  }
  if (vinculosCobertura.length === 0) {
    throw new Error('Nenhum vínculo de Cobertura cadastrado — sem ele não dá pra saber a qual indústria cada linha pertence')
  }

  const num = (v: unknown): number => {
    if (typeof v === 'number') return v
    const s = String(v ?? '').trim().replace(/\./g, '').replace(',', '.')
    return Number(s) || 0
  }

  const header = ['industria_id', ...RESUMO_REDES_COLS.map(c => c.field)]
  const out = [header.join(';')]
  const semIndustria = new Set<number>()
  for (let i = 1; i < linhas.length; i++) {
    const row = linhas[i] as unknown[]
    if (!row || row.length === 0) continue
    const codPrinc = String(row[idxPorCampo['cod_princ']] ?? '').trim()
    if (!codPrinc) continue
    const objCobertura = num(row[idxPorCampo['objetivo_cobertura']])
    const vinculo = vinculosCobertura.find(v => num(v.parametros_valores['limiar_valor_medio']) === objCobertura)
    if (!vinculo) {
      semIndustria.add(objCobertura)
      continue
    }
    const valores = [String(vinculo.industria_id)]
    for (const { field } of RESUMO_REDES_COLS) {
      const idx = idxPorCampo[field]
      const raw = idx === undefined ? '' : row[idx]
      valores.push(csvEscape(String(raw ?? '').trim()))
    }
    out.push(valores.join(';'))
  }
  if (out.length === 1) {
    throw new Error(
      semIndustria.size > 0
        ? `Nenhuma linha casou com o Objetivo Cobertura de um vínculo cadastrado (valores vistos na planilha: ${[...semIndustria].join(', ')}) — confira se limiar_valor_medio está certo em Objetivos por Indústria`
        : 'Nenhuma linha válida encontrada na aba'
    )
  }
  return out.join('\r\n')
}

function fmtBRL(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 })
}
function fmtPct(v: number) {
  return `${v.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}%`
}
function fmtNum(v: number) {
  return v.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

function seloStatus(status: ComparativoLinha['status']) {
  switch (status) {
    case 'OK': return { texto: 'OK', classe: 'bg-emerald-50 text-emerald-700 border-emerald-200' }
    case 'DIVERGE': return { texto: 'Divergência', classe: 'bg-red-50 text-red-700 border-red-200' }
    case 'SO_EXTERNO': return { texto: 'Só no Fechamento', classe: 'bg-amber-50 text-amber-700 border-amber-200' }
    default: return { texto: 'Só no Farol', classe: 'bg-amber-50 text-amber-700 border-amber-200' }
  }
}
function linhaFundo(status: ComparativoLinha['status']) {
  if (status === 'DIVERGE') return 'bg-red-50/40'
  if (status === 'SO_EXTERNO' || status === 'SO_FAROL') return 'bg-amber-50/40'
  return ''
}

export default function FarolComparativoFechamento() {
  const { token } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])
  const inputRef = useRef<HTMLInputElement>(null)

  const hoje = new Date()
  const primeiroDiaMes = new Date(hoje.getFullYear(), hoje.getMonth(), 1).toISOString().slice(0, 10)
  const ultimoDiaMes = new Date(hoje.getFullYear(), hoje.getMonth() + 1, 0).toISOString().slice(0, 10)

  const [dataInicio, setDataInicio] = useState(primeiroDiaMes)
  const [dataFim, setDataFim] = useState(ultimoDiaMes)
  const [industriaID, setIndustriaID] = useState('')
  const [arquivo, setArquivo] = useState<File | null>(null)
  const [importando, setImportando] = useState(false)
  const [comparando, setComparando] = useState(false)
  const [erro, setErro] = useState<string | null>(null)
  const [linhas, setLinhas] = useState<ComparativoLinha[] | null>(null)

  const { data: vinculos = [] } = useQuery<MetaVinculo[]>({
    queryKey: ['farol-metas-vinculos'],
    queryFn: async () => {
      const r = await fetch('/api/farol/metas-vinculos', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })
  const industrias = useMemo(() => {
    const m = new Map<number, string>()
    for (const v of vinculos) m.set(v.industria_id, v.industria_nome)
    return [...m.entries()].map(([id, nome]) => ({ id, nome }))
  }, [vinculos])
  const vinculosCobertura = useMemo(() => vinculos.filter(v => v.formula_codigo === 'cobertura_rede'), [vinculos])

  async function buscarComparativo(industria: string) {
    setComparando(true)
    setErro(null)
    try {
      const p = new URLSearchParams({ industria_id: industria, data_inicio: dataInicio, data_fim: dataFim })
      const r = await fetch(`/api/farol/fechamento-comercial-comparativo?${p}`, { headers })
      const body = await r.json().catch(() => null)
      if (!r.ok) throw new Error(body?.error || `Falha ao buscar o comparativo (HTTP ${r.status})`)
      setLinhas(body.linhas ?? [])
    } catch (e) {
      setErro(e instanceof Error ? e.message : 'Falha ao buscar o comparativo')
      setLinhas(null)
    } finally {
      setComparando(false)
    }
  }

  async function importarEComparar() {
    if (!arquivo) { toast.error('Selecione o arquivo (.xlsx) do fechamento'); return }
    if (!dataInicio || !dataFim) { toast.error('Informe o período (Data Início e Data Fim)'); return }
    setImportando(true)
    setErro(null)
    setLinhas(null)
    try {
      const csvText = await parseFechamentoComercialXlsx(arquivo, vinculosCobertura)
      const csvFile = new File([csvText], arquivo.name.replace(/\.xlsx?$/i, '.csv'), { type: 'text/csv' })
      const form = new FormData()
      form.append('file', csvFile)
      const p = new URLSearchParams({ data_inicio: dataInicio, data_fim: dataFim })
      const r = await fetch(`/api/farol/fechamento-comercial-importar-csv?${p}`, { method: 'POST', headers, body: form })
      const body = await r.json().catch(() => null)
      if (!r.ok) throw new Error(body?.error || `Falha ao importar (HTTP ${r.status})`)
      toast.success(`${body.linhas_importadas} linha(s) importada(s)`)
      // Já tenta mostrar o comparativo da indústria selecionada (ou a
      // primeira encontrada no arquivo, se nenhuma estava escolhida ainda).
      const industriaParaMostrar = industriaID || String(industrias[0]?.id ?? '')
      if (industriaParaMostrar) {
        setIndustriaID(industriaParaMostrar)
        await buscarComparativo(industriaParaMostrar)
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : 'Falha ao importar'
      setErro(msg)
      toast.error(msg, { style: { whiteSpace: 'pre-line' } })
    } finally {
      setImportando(false)
    }
  }

  function onSelecionarArquivo(files: FileList | null) {
    const f = files?.[0] ?? null
    if (f && !/\.xlsx?$/i.test(f.name)) {
      toast.error('Envie o .xlsx do fechamento (aba "Resumo Redes")')
      return
    }
    setArquivo(f)
  }

  const totais = useMemo(() => {
    const l = linhas ?? []
    const t = {
      valorExt: 0, valorFarol: 0, eansExt: 0, eansFarol: 0,
      ok: 0, diverge: 0, soExterno: 0, soFarol: 0,
    }
    for (const r of l) {
      t.valorExt += r.valor_venda_externo
      t.valorFarol += r.valor_venda_farol
      t.eansExt += r.eans_externo
      t.eansFarol += r.eans_farol
      if (r.status === 'OK') t.ok++
      else if (r.status === 'DIVERGE') t.diverge++
      else if (r.status === 'SO_EXTERNO') t.soExterno++
      else t.soFarol++
    }
    return t
  }, [linhas])

  function exportar() {
    if (!linhas || linhas.length === 0) { toast.error('Nada pra exportar ainda — busque o comparativo primeiro'); return }
    const industriaNome = industrias.find(i => String(i.id) === industriaID)?.nome ?? industriaID
    exportToExcel(
      linhas.map(l => ({
        'Cod Princ': l.cod_princ, Razão: l.razao, Fantasia: l.fantasia, 'Qt Lojas': l.qt_lojas,
        GGV: `${l.cod_ggv} — ${l.nome_ggv}`, CRV: `${l.cod_crv} — ${l.nome_crv}`, RCA: `${l.cod_rca} — ${l.nome_rca}`,
        'Valor Venda (Fechamento)': l.valor_venda_externo, 'Valor Venda (Farol)': l.valor_venda_farol,
        'Diferença R$': l.diferenca_valor, 'Diferença %': l.diferenca_valor_pct,
        'Qt EANs (Fechamento)': l.eans_externo, 'Qt EANs (Farol)': l.eans_farol, 'Diferença EANs': l.diferenca_eans,
        Status: seloStatus(l.status).texto,
      })),
      `Comparativo_Fechamento_${industriaNome}_${dataInicio}_a_${dataFim}`,
      'Comparativo',
    )
  }

  return (
    <div className="p-6 space-y-6">
      <div className="bg-white rounded-xl border border-slate-200 p-6 shadow-sm">
        <div className="flex items-center justify-between gap-3 mb-1">
          <div className="flex items-center gap-2">
            <Scale className="h-5 w-5 text-slate-600" />
            <h2 className="text-lg font-semibold text-slate-900">Comparativo Fechamento Comercial</h2>
          </div>
          {/* Racional do cálculo — pedido do Claudio e Heverton 14/09/2026,
              mesmo componente usado no Painel de Objetivos por Indústria. */}
          {industriaID && (
            <BotaoComoFunciona
              industriaNome={industrias.find(i => String(i.id) === industriaID)?.nome ?? ''}
              vinculoCoberturaId={vinculosCobertura.find(v => String(v.industria_id) === industriaID)?.id}
              vinculoSortimentoId={vinculos.find(v => v.formula_codigo === 'sortimento_rede' && String(v.industria_id) === industriaID)?.id}
              limiarCobertura={vinculosCobertura.find(v => String(v.industria_id) === industriaID)?.parametros_valores?.limiar_valor_medio as number | undefined}
            />
          )}
        </div>
        <p className="text-sm text-slate-500 mb-4">
          Sobe o fechamento que o fornecedor manda por fora (aba "Resumo Redes" do modelo da JC) e compara, Rede a
          Rede, com a apuração oficial do Farol (Painel de Objetivos por Indústria) pro mesmo período. Fica gravado —
          reimportar substitui o fechamento anterior desse período.
        </p>

        <div className="mb-4 flex flex-wrap items-end gap-4">
          <div>
            <label className="block text-xs uppercase tracking-wide text-slate-500 font-semibold mb-1.5">Data Início</label>
            <input type="date" value={dataInicio} onChange={e => { setDataInicio(e.target.value); setLinhas(null) }}
              className="h-10 rounded-md border border-input bg-background px-2 text-sm" />
          </div>
          <div>
            <label className="block text-xs uppercase tracking-wide text-slate-500 font-semibold mb-1.5">Data Fim</label>
            <input type="date" value={dataFim} onChange={e => { setDataFim(e.target.value); setLinhas(null) }}
              className="h-10 rounded-md border border-input bg-background px-2 text-sm" />
          </div>
          {industrias.length > 0 && (
            <div>
              <label className="block text-xs uppercase tracking-wide text-slate-500 font-semibold mb-1.5">Indústria</label>
              <Select value={industriaID} onValueChange={v => { setIndustriaID(v); setLinhas(null); buscarComparativo(v) }}>
                <SelectTrigger className="w-56"><SelectValue placeholder="Selecione" /></SelectTrigger>
                <SelectContent>
                  {industrias.map(i => <SelectItem key={i.id} value={String(i.id)}>{i.nome}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>

        <div
          className="flex flex-wrap items-center gap-4 rounded-lg border border-dashed border-slate-300 bg-slate-50 p-4"
          onDragOver={e => e.preventDefault()}
          onDrop={e => { e.preventDefault(); onSelecionarArquivo(e.dataTransfer.files) }}
        >
          <FileUp className="h-8 w-8 text-slate-400 shrink-0" />
          <div className="flex-1 min-w-[200px]">
            <div className="text-sm font-medium text-slate-700">
              {arquivo ? arquivo.name : 'Arraste o .xlsx do fechamento aqui ou clique para escolher'}
            </div>
            <div className="text-xs text-slate-400">Modelo da JC — aba com "Resumo Redes" no nome</div>
          </div>
          <input ref={inputRef} type="file" accept=".xlsx,.xls" className="hidden" onChange={e => onSelecionarArquivo(e.target.files)} />
          <Button variant="outline" onClick={() => inputRef.current?.click()}>Escolher arquivo</Button>
          <Button onClick={importarEComparar} disabled={!arquivo || importando || comparando} className="gap-2">
            <Upload className="h-4 w-4" />
            {importando ? 'Importando…' : 'Importar e Comparar'}
          </Button>
          <Button variant="outline" onClick={exportar} disabled={!linhas || linhas.length === 0} className="gap-2">
            <Download className="h-4 w-4" />
            Exportar Excel
          </Button>
        </div>
      </div>

      {erro && (
        <div className="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 p-4">
          <AlertTriangle className="h-5 w-5 text-red-600 shrink-0 mt-0.5" />
          <div className="text-sm text-red-900 whitespace-pre-line">{erro}</div>
        </div>
      )}

      {comparando && !linhas && (
        <p className="text-sm text-slate-500">Carregando comparativo...</p>
      )}

      {linhas && (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-xs text-slate-500">
              {linhas.length} rede(s) · {totais.ok} OK · {totais.diverge} divergência(s) · {totais.soExterno + totais.soFarol} órfã(s)
            </span>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white p-5">
            <div className="text-xs uppercase tracking-wide text-slate-500 font-semibold mb-3">Totais</div>
            <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
              <div>
                <div className="text-xs text-slate-500">Valor Venda (Fechamento)</div>
                <div className="text-xl font-bold text-slate-900">{fmtBRL(totais.valorExt)}</div>
              </div>
              <div>
                <div className="text-xs text-slate-500">Valor Venda (Farol)</div>
                <div className="text-xl font-bold text-slate-900">{fmtBRL(totais.valorFarol)}</div>
              </div>
              <div>
                <div className="text-xs text-slate-500">Diferença</div>
                <div className="text-lg font-semibold text-slate-700">
                  {fmtBRL(totais.valorFarol - totais.valorExt)} ({fmtPct(totais.valorExt ? (totais.valorFarol - totais.valorExt) / totais.valorExt * 100 : 0)})
                </div>
              </div>
              <div>
                <div className="text-xs text-slate-500">Qt EANs médio (Fechamento × Farol)</div>
                <div className="text-lg font-semibold text-slate-700">{fmtNum(totais.eansExt)} × {fmtNum(totais.eansFarol)}</div>
              </div>
            </div>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-slate-50 border-b border-slate-200">
                  <tr className="text-left text-xs uppercase tracking-wide text-slate-500">
                    <th className="px-4 py-3 font-medium">Cód. Princ.</th>
                    <th className="px-4 py-3 font-medium">Razão / Fantasia</th>
                    <th className="px-4 py-3 font-medium">GGV</th>
                    <th className="px-4 py-3 font-medium text-right">Valor Venda (Fech.)</th>
                    <th className="px-4 py-3 font-medium text-right">Valor Venda (Farol)</th>
                    <th className="px-4 py-3 font-medium text-right">Dif. %</th>
                    <th className="px-4 py-3 font-medium text-right">EANs (Fech.)</th>
                    <th className="px-4 py-3 font-medium text-right">EANs (Farol)</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {linhas.length === 0 && (
                    <tr><td colSpan={9} className="text-center py-8 text-slate-400">Nenhuma rede pra este período/indústria</td></tr>
                  )}
                  {linhas.map(l => {
                    const selo = seloStatus(l.status)
                    return (
                      <tr key={l.cod_princ} className={`hover:bg-slate-50 ${linhaFundo(l.status)}`}>
                        <td className="px-4 py-2.5 font-mono text-xs text-slate-600 whitespace-nowrap">{l.cod_princ}</td>
                        <td className="px-4 py-2.5 text-slate-900">
                          <div>{l.razao || '—'}</div>
                          {l.fantasia && l.fantasia !== l.razao && <div className="text-xs text-slate-400">{l.fantasia}</div>}
                        </td>
                        <td className="px-4 py-2.5 text-xs text-slate-500 whitespace-nowrap">{l.cod_ggv} — {l.nome_ggv}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-900">{fmtBRL(l.valor_venda_externo)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtBRL(l.valor_venda_farol)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-500">{fmtPct(l.diferenca_valor_pct)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-900">{fmtNum(l.eans_externo)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtNum(l.eans_farol)}</td>
                        <td className="px-4 py-2.5">
                          <span className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium ${selo.classe}`}>
                            {l.status === 'OK' && <CheckCircle2 className="h-3 w-3" />}
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
            Valor Venda (Farol) = soma (não média) entre as lojas da Rede, mesma coluna "Valor Venda" do modelo da JC.
            Qt EANs = média de EANs distintos vendidos por loja (Sortimento). Uma linha é "OK" quando a diferença de
            valor está a até 5% e a de EANs a menos de 2 — acima disso vira "Divergência". "Só no Fechamento"/"Só no
            Farol" marca Rede que só aparece de um lado (fora da base de Clientes Válidos carregada, ou fora do
            arquivo do fornecedor).
          </p>
        </>
      )}
    </div>
  )
}
