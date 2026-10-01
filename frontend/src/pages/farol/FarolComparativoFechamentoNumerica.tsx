import { useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CheckCircle2, Download, FileUp, Info, Scale, Upload } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'sonner'
import { exportToExcel } from '@/lib/exportToExcel'
import { formatCNPJ } from '@/lib/formatFilial'

// Comparativo Fechamento Numérica — Épico 7 addendum, 2026-09-30, mesmo
// espírito do Comparativo Fechamento Comercial (FarolComparativoFechamento.tsx,
// ver seu cabeçalho pro racional completo) — só que pro par Numérica
// (Cobertura Numérica + Sortimento Numérica/PPA), chave por CNPJ/Cliente
// (a Numérica não tem Rede, FR24), não por cod_princ.
//
// Diferença de leitura do arquivo: a aba "Resumo Numerica(s) Cliente" (nome
// varia — "Numerica" no arquivo HC, "Numericas" no Foods, achado real
// 2026-09-30) não traz CNPJ direto, só COD CL — resolvido aqui casando com
// a aba "BASE LOJAS" do MESMO arquivo. Cada arquivo do Carlos já cobre UMA
// indústria só (sem duplicar linha por indústria como o fechamento Rede) —
// por isso a indústria é escolhida manualmente (Select), sem a lógica de
// "casar objetivo com limiar" que o fechamento Rede precisa.

interface MetaVinculo {
  id: number
  industria_id: number
  industria_nome: string
  formula_codigo: string
}

interface ComparativoLinha {
  cnpj: string
  cod_cl: string
  classificacao_pdv: string
  razao: string
  fantasia: string
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
  ppas_externo: number
  ppas_farol: number
  diferenca_ppas: number
  status: 'OK' | 'DIVERGE' | 'SO_EXTERNO' | 'SO_FAROL'
  // cod_cli_divergente — achado real 2026-09-30: o CNPJ tem venda no
  // bimestre lançada sob um COD CLI diferente do cadastrado (recadastro no
  // WinThor que o fechamento do fornecedor não acompanha) — explica boa
  // parte das divergências sem ser erro do Farol, ver farol_fechamento_numerica.go.
  cod_cli_divergente: boolean
  cod_cli_vistos?: string[]
}

const RESUMO_NUMERICA_COLS: { field: string; candidates: string[] }[] = [
  { field: 'classificacao_pdv', candidates: ['CLASSIFICAÇÃO PDV', 'CLASSIFICACAO PDV'] },
  { field: 'cod_cl', candidates: ['COD CL'] },
  { field: 'razao', candidates: ['RAZAO', 'RAZÃO'] },
  { field: 'fantasia', candidates: ['FANTASIA'] },
  { field: 'valor_venda', candidates: ['VALOR VENDA'] },
  { field: 'qt_ppas_vendidos', candidates: ["QT DE PPA'S VENDIDOS", 'QT DE PPAS VENDIDOS'] },
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
// ehNumC — Carlos (JC) argumentou 01/10/2026 que o grupo "Num. C" (menor
// relevância comercial, maior volume de CNPJs) não precisa ser conferido
// no fechamento. Normaliza pra aceitar variação de espaço/ponto/caixa
// ("NUM. C", "NUM C", "NUMC").
function ehNumC(classificacao: string): boolean {
  return normalizaHeader(classificacao).replace(/[.\s]/g, '') === 'NUMC'
}
function normalizaCnpj(v: unknown): string {
  return String(v ?? '').replace(/\D/g, '').padStart(14, '0')
}
function csvEscape(v: string): string {
  if (/[;"\n\r]/.test(v)) return '"' + v.replace(/"/g, '""') + '"'
  return v
}

// parseFechamentoNumericaXlsx — lê "BASE LOJAS" (CNPJ×COD CL) e a aba
// "Resumo Numerica(s) Cliente" (nome flexível) no navegador, devolve o CSV
// que o endpoint de importação espera (industria_id já resolvido pelo
// Select do usuário, não pela planilha — cada arquivo já é de UMA indústria).
async function parseFechamentoNumericaXlsx(file: File, industriaID: string): Promise<string> {
  const XLSX = await import('xlsx')
  const buf = await file.arrayBuffer()
  const wb = XLSX.read(buf, { type: 'array', cellText: false, cellDates: false })

  const abaLojasNome = wb.SheetNames.find(n => normalizaHeader(n).includes('BASE LOJAS'))
  if (!abaLojasNome) throw new Error('Arquivo sem aba "BASE LOJAS" — precisa dela pra resolver o CNPJ de cada COD CL')
  const lojas = XLSX.utils.sheet_to_json<Record<string, unknown>>(wb.Sheets[abaLojasNome], { defval: '' })
  const cnpjPorCodCl = new Map<string, string>()
  for (const l of lojas) {
    const codCl = String(l['CODCL'] ?? l['COD CL'] ?? '').trim()
    if (codCl) cnpjPorCodCl.set(codCl, normalizaCnpj(l['CNPJ']))
  }
  if (cnpjPorCodCl.size === 0) throw new Error('Aba "BASE LOJAS" sem nenhuma linha com CODCL/CNPJ preenchidos')

  const abaResumoNome = wb.SheetNames.find(n => {
    const h = normalizaHeader(n)
    return h.includes('RESUMO') && h.includes('NUMERIC')
  })
  if (!abaResumoNome) throw new Error('Arquivo sem aba "Resumo Numerica(s) Cliente" — confira se é o arquivo de Fechamento certo')
  const sheet = wb.Sheets[abaResumoNome]
  const linhas = XLSX.utils.sheet_to_json<unknown[]>(sheet, { header: 1, raw: true, defval: '' })
  if (linhas.length === 0) throw new Error(`Aba "${abaResumoNome}" está vazia`)

  const headerRow = (linhas[0] as unknown[]).map(normalizaHeader)
  const idxPorCampo: Record<string, number> = {}
  const faltando: string[] = []
  for (const { field, candidates } of RESUMO_NUMERICA_COLS) {
    const idx = headerRow.findIndex(h => candidates.some(c => h.startsWith(c)))
    if (idx === -1) {
      if (field !== 'cod_cl' && field !== 'valor_venda' && field !== 'qt_ppas_vendidos') continue
      faltando.push(candidates[0])
      continue
    }
    idxPorCampo[field] = idx
  }
  if (faltando.length > 0) {
    throw new Error(`Aba "${abaResumoNome}" sem a(s) coluna(s): ${faltando.join(', ')}`)
  }

  const num = (v: unknown): number => {
    if (typeof v === 'number') return v
    const s = String(v ?? '').trim().replace(/\./g, '').replace(',', '.')
    return Number(s) || 0
  }

  const header = ['industria_id', 'cnpj', ...RESUMO_NUMERICA_COLS.map(c => c.field)]
  const out = [header.join(';')]
  let semCnpj = 0
  for (let i = 1; i < linhas.length; i++) {
    const row = linhas[i] as unknown[]
    if (!row || row.length === 0) continue
    const codCl = String(row[idxPorCampo['cod_cl']] ?? '').trim()
    if (!codCl) continue
    const cnpj = cnpjPorCodCl.get(codCl)
    if (!cnpj) {
      semCnpj++
      continue
    }
    const valores = [industriaID, cnpj]
    for (const { field } of RESUMO_NUMERICA_COLS) {
      const idx = idxPorCampo[field]
      const raw = idx === undefined ? '' : row[idx]
      valores.push(csvEscape(String(raw ?? '').trim()))
    }
    out.push(valores.join(';'))
  }
  if (out.length === 1) {
    throw new Error(
      semCnpj > 0
        ? `Nenhum COD CL da aba "${abaResumoNome}" foi encontrado na "BASE LOJAS" (${semCnpj} linha(s) sem CNPJ resolvido)`
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
    case 'OK': return { texto: 'OK', classe: 'bg-green-50 text-green-800 border-green-300' }
    case 'DIVERGE': return { texto: 'Divergência', classe: 'bg-red-50 text-red-800 border-red-300' }
    case 'SO_EXTERNO': return { texto: 'Só no Fechamento', classe: 'bg-amber-50 text-amber-700 border-amber-200' }
    default: return { texto: 'Só no Farol', classe: 'bg-amber-50 text-amber-700 border-amber-200' }
  }
}
function linhaFundo(status: ComparativoLinha['status']) {
  if (status === 'DIVERGE') return 'bg-red-50/40'
  if (status === 'SO_EXTERNO' || status === 'SO_FAROL') return 'bg-amber-50/40'
  return ''
}

export default function FarolComparativoFechamentoNumerica() {
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
  const [conferirNumC, setConferirNumC] = useState(true)

  const { data: vinculos = [] } = useQuery<MetaVinculo[]>({
    queryKey: ['farol-metas-vinculos'],
    queryFn: async () => {
      const r = await fetch('/api/farol/metas-vinculos', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })
  const vinculosCoberturaNumerica = useMemo(() => vinculos.filter(v => v.formula_codigo === 'cobertura_numerica'), [vinculos])
  const industrias = useMemo(() => {
    const m = new Map<number, string>()
    for (const v of vinculosCoberturaNumerica) m.set(v.industria_id, v.industria_nome)
    return [...m.entries()].map(([id, nome]) => ({ id, nome }))
  }, [vinculosCoberturaNumerica])

  async function buscarComparativo(industria: string) {
    setComparando(true)
    setErro(null)
    try {
      const p = new URLSearchParams({ industria_id: industria, data_inicio: dataInicio, data_fim: dataFim })
      const r = await fetch(`/api/farol/fechamento-numerica-comparativo?${p}`, { headers })
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
    if (!industriaID) { toast.error('Selecione a Indústria deste arquivo'); return }
    setImportando(true)
    setErro(null)
    setLinhas(null)
    try {
      const csvText = await parseFechamentoNumericaXlsx(arquivo, industriaID)
      const csvFile = new File([csvText], arquivo.name.replace(/\.xlsx?$/i, '.csv'), { type: 'text/csv' })
      const form = new FormData()
      form.append('file', csvFile)
      const p = new URLSearchParams({ data_inicio: dataInicio, data_fim: dataFim })
      const r = await fetch(`/api/farol/fechamento-numerica-importar-csv?${p}`, { method: 'POST', headers, body: form })
      const body = await r.json().catch(() => null)
      if (!r.ok) throw new Error(body?.error || `Falha ao importar (HTTP ${r.status})`)
      toast.success(`${body.linhas_importadas} linha(s) importada(s)`)
      await buscarComparativo(industriaID)
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
      toast.error('Envie o .xlsx do fechamento (aba "Resumo Numerica(s) Cliente")')
      return
    }
    setArquivo(f)
  }

  const numCCount = useMemo(() => (linhas ?? []).filter(l => ehNumC(l.classificacao_pdv)).length, [linhas])
  const linhasFiltradas = useMemo(() => {
    const l = linhas ?? []
    return conferirNumC ? l : l.filter(r => !ehNumC(r.classificacao_pdv))
  }, [linhas, conferirNumC])

  const totais = useMemo(() => {
    const l = linhasFiltradas
    const t = {
      valorExt: 0, valorFarol: 0, ppasExt: 0, ppasFarol: 0,
      ok: 0, diverge: 0, soExterno: 0, soFarol: 0, codCliDivergente: 0,
    }
    for (const r of l) {
      t.valorExt += r.valor_venda_externo
      t.valorFarol += r.valor_venda_farol
      t.ppasExt += r.ppas_externo
      t.ppasFarol += r.ppas_farol
      if (r.cod_cli_divergente) t.codCliDivergente++
      if (r.status === 'OK') t.ok++
      else if (r.status === 'DIVERGE') t.diverge++
      else if (r.status === 'SO_EXTERNO') t.soExterno++
      else t.soFarol++
    }
    return t
  }, [linhasFiltradas])

  function exportar() {
    if (linhasFiltradas.length === 0) { toast.error('Nada pra exportar ainda — busque o comparativo primeiro'); return }
    const industriaNome = industrias.find(i => String(i.id) === industriaID)?.nome ?? industriaID
    exportToExcel(
      linhasFiltradas.map(l => ({
        CNPJ: formatCNPJ(l.cnpj), 'Cód. Cliente': l.cod_cl, Classificação: l.classificacao_pdv,
        Razão: l.razao, Fantasia: l.fantasia,
        GGV: `${l.cod_ggv} — ${l.nome_ggv}`, CRV: `${l.cod_crv} — ${l.nome_crv}`, RCA: `${l.cod_rca} — ${l.nome_rca}`,
        'Valor Venda (Fechamento)': l.valor_venda_externo, 'Valor Venda (Farol)': l.valor_venda_farol,
        'Diferença R$': l.diferenca_valor, 'Diferença %': l.diferenca_valor_pct,
        'Qt PPAs (Fechamento)': l.ppas_externo, 'Qt PPAs (Farol)': l.ppas_farol, 'Diferença PPAs': l.diferenca_ppas,
        Status: seloStatus(l.status).texto,
        'Cód. Cliente Divergente': l.cod_cli_divergente ? `Sim (${(l.cod_cli_vistos ?? []).join(', ')})` : 'Não',
      })),
      `Comparativo_Fechamento_Numerica_${industriaNome}_${dataInicio}_a_${dataFim}`,
      'Comparativo',
    )
  }

  return (
    <div className="p-6 space-y-6">
      <div className="bg-white rounded-xl border border-slate-200 p-6 shadow-sm">
        <div className="flex items-center gap-2 mb-1">
          <Scale className="h-5 w-5 text-slate-600" />
          <h2 className="text-lg font-semibold text-slate-900">Comparativo Fechamento Numérica</h2>
        </div>
        <p className="text-sm text-slate-500 mb-4">
          Sobe o fechamento que o fornecedor manda por fora (aba "Resumo Numerica(s) Cliente" do modelo da JC) e
          compara, Cliente/CNPJ a Cliente/CNPJ, com a apuração oficial do Farol (Cobertura Numérica + Sortimento
          Numérica) pro mesmo período. Fica gravado — reimportar substitui o fechamento anterior desse período.
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
          <div>
            <label className="block text-xs uppercase tracking-wide text-slate-500 font-semibold mb-1.5">Indústria</label>
            <Select value={industriaID} onValueChange={v => { setIndustriaID(v); setLinhas(null); if (v) buscarComparativo(v) }}>
              <SelectTrigger className="w-56"><SelectValue placeholder="Selecione" /></SelectTrigger>
              <SelectContent>
                {industrias.map(i => <SelectItem key={i.id} value={String(i.id)}>{i.nome}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
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
            <div className="text-xs text-slate-400">Modelo da JC — arquivo com aba "Resumo Numerica(s) Cliente" + "BASE LOJAS"</div>
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
          {numCCount > 0 && (
            <label className="flex items-start gap-2 rounded-lg border border-slate-200 bg-slate-50 p-3 text-sm text-slate-700 cursor-pointer w-fit">
              <input
                type="checkbox"
                className="mt-0.5 h-4 w-4"
                checked={conferirNumC}
                onChange={e => setConferirNumC(e.target.checked)}
              />
              <span>
                Conferir grupo <strong>Num. C</strong> também ({numCCount} cliente{numCCount === 1 ? '' : 's'})
                <span className="block text-xs text-slate-400">
                  Carlos (JC) sugeriu não conferir Num. C — desmarque pra ver o efeito nos totais e nas divergências.
                </span>
              </span>
            </label>
          )}

          <div className="flex flex-wrap items-center gap-3">
            <span className="text-xs text-slate-500">
              {linhasFiltradas.length} cliente(s){!conferirNumC && numCCount > 0 && <> (Num. C excluído)</>} · {totais.ok} OK · {totais.diverge} divergência(s) · {totais.soExterno + totais.soFarol} órfã(s)
              {totais.codCliDivergente > 0 && <> · {totais.codCliDivergente} com Cód. Cliente divergente (provável recadastro no WinThor)</>}
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
                <div className="text-xs text-slate-500">Qt PPAs total (Fechamento × Farol)</div>
                <div className="text-lg font-semibold text-slate-700">{fmtNum(totais.ppasExt)} × {fmtNum(totais.ppasFarol)}</div>
              </div>
            </div>
          </div>

          <div className="rounded-xl border border-slate-200 bg-white overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-slate-50 border-b border-slate-200">
                  <tr className="text-left text-xs uppercase tracking-wide text-slate-500">
                    <th className="px-4 py-3 font-medium">CNPJ</th>
                    <th className="px-4 py-3 font-medium">Razão / Fantasia</th>
                    <th className="px-4 py-3 font-medium">Classif.</th>
                    <th className="px-4 py-3 font-medium">GGV / CRV / RCA</th>
                    <th className="px-4 py-3 font-medium text-right">Valor Venda (Fech.)</th>
                    <th className="px-4 py-3 font-medium text-right">Valor Venda (Farol)</th>
                    <th className="px-4 py-3 font-medium text-right">Dif. %</th>
                    <th className="px-4 py-3 font-medium text-right">PPAs (Fech.)</th>
                    <th className="px-4 py-3 font-medium text-right">PPAs (Farol)</th>
                    <th className="px-4 py-3 font-medium">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {linhasFiltradas.length === 0 && (
                    <tr><td colSpan={10} className="text-center py-8 text-slate-400">Nenhum cliente pra este período/indústria</td></tr>
                  )}
                  {linhasFiltradas.map(l => {
                    const selo = seloStatus(l.status)
                    return (
                      <tr key={l.cnpj} className={`hover:bg-slate-50 ${linhaFundo(l.status)}`}>
                        <td className="px-4 py-2.5 font-mono text-xs text-slate-600 whitespace-nowrap">{formatCNPJ(l.cnpj)}</td>
                        <td className="px-4 py-2.5 text-slate-900">
                          <div>{l.cod_cl ? `${l.cod_cl} — ` : ''}{l.razao || '—'}</div>
                          {l.fantasia && l.fantasia !== l.razao && <div className="text-xs text-slate-400">{l.fantasia}</div>}
                        </td>
                        <td className="px-4 py-2.5 text-xs text-slate-500 whitespace-nowrap">{l.classificacao_pdv || '—'}</td>
                        <td className="px-4 py-2.5 text-xs text-slate-500 whitespace-nowrap">
                          <div>{l.cod_ggv} — {l.nome_ggv}</div>
                          <div className="text-slate-400">{l.cod_crv} — {l.nome_crv}</div>
                          <div className="text-slate-400">{l.cod_rca} — {l.nome_rca}</div>
                        </td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-900">{fmtBRL(l.valor_venda_externo)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtBRL(l.valor_venda_farol)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-500">{fmtPct(l.diferenca_valor_pct)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-900">{fmtNum(l.ppas_externo)}</td>
                        <td className="px-4 py-2.5 text-right tabular-nums text-slate-700">{fmtNum(l.ppas_farol)}</td>
                        <td className="px-4 py-2.5">
                          <div className="flex flex-col items-start gap-1">
                            <span className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium ${selo.classe}`}>
                              {l.status === 'OK' && <CheckCircle2 className="h-3 w-3" />}
                              {selo.texto}
                            </span>
                            {l.cod_cli_divergente && (
                              <span
                                className="inline-flex items-center gap-1 rounded-md border border-sky-200 bg-sky-50 px-2 py-0.5 text-xs font-medium text-sky-700"
                                title={`Cliente tem venda no período lançada sob COD CLI diferente do cadastrado (${l.cod_cl}): ${(l.cod_cli_vistos ?? []).join(', ')}. Provável recadastro no WinThor não refletido no arquivo do fornecedor — não é erro do Farol.`}
                              >
                                <Info className="h-3 w-3" />
                                Cód. Cliente divergente
                              </span>
                            )}
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>

          <p className="text-xs text-slate-400">
            Valor Venda (Farol) = soma líquida de vendas do CNPJ no bimestre móvel (fluxo Faturado). Qt PPAs = famílias
            de produto distintas positivadas, já capadas no teto do programa. Uma linha é "OK" quando a diferença de
            valor está a até 5% e a de PPAs a menos de 2 — acima disso vira "Divergência". "Só no Fechamento"/"Só no
            Farol" marca Cliente que só aparece de um lado.
          </p>
        </>
      )}
    </div>
  )
}
