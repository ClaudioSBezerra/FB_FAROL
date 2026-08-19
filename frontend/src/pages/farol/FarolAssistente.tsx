import { useState, useRef, useEffect } from 'react'
import { Sparkles, Send, Download, RotateCcw, Clock, ChevronDown, ChevronUp, Copy, Check, BarChart3, Table2 } from 'lucide-react'
import { fmtCell, buildChartSpec } from '@/lib/farolAiFormat'
import AssistenteChart from './AssistenteChart'

// ─── Tipos ────────────────────────────────────────────────────────────────────

interface QueryResult {
  pergunta: string
  sql: string
  columns: string[]
  rows: Record<string, unknown>[]
  row_count: number
  model: string
  error?: string
}

// ─── Sugestões de consultas ───────────────────────────────────────────────────

const SUGESTOES = [
  { label: 'Melhor cliente de GO e o que ele deixou de comprar', query: 'Qual foi o melhor cliente em Goiás e quais produtos ele comprou em 2025 e não comprou em 2026?' },
  { label: 'Top 10 RCAs de 2026', query: 'Quais são os 10 RCAs com maior faturamento em 2026?' },
  { label: 'Faturamento mês a mês em 2026', query: 'Mostre o faturamento total mês a mês em 2026' },
  { label: 'Top 15 indústrias', query: 'Quais são as 15 maiores indústrias por faturamento líquido em 2026?' },
  { label: 'Clientes que sumiram', query: 'Quais clientes compraram em 2025 e não compraram nada em 2026?' },
  { label: 'Ranking de supervisores', query: 'Compare o faturamento por supervisor em 2026, do maior para o menor' },
  { label: 'Faturamento por filial', query: 'Qual o faturamento por filial em 2026?' },
  { label: 'Bonificação por indústria', query: 'Quanto foi concedido de bonificação por indústria em 2026?' },
]

const HISTORY_KEY = 'farol_ai_history'
const MAX_HISTORY = 12

function loadHistory(): string[] {
  try { return JSON.parse(localStorage.getItem(HISTORY_KEY) ?? '[]') } catch { return [] }
}
function saveHistory(h: string[]) {
  localStorage.setItem(HISTORY_KEY, JSON.stringify(h.slice(0, MAX_HISTORY)))
}

// ─── Formatação de células ────────────────────────────────────────────────────


// ─── Componente principal ─────────────────────────────────────────────────────

export default function FarolAssistente() {
  const [pergunta, setPergunta]     = useState('')
  const [result, setResult]         = useState<QueryResult | null>(null)
  const [medida, setMedida]         = useState('')
  const [verGrafico, setVerGrafico] = useState(true)
  const [loading, setLoading]       = useState(false)
  const [showSQL, setShowSQL]       = useState(false)
  const [copied, setCopied]         = useState(false)
  const [exporting, setExporting]   = useState(false)
  const [history, setHistory]       = useState<string[]>(loadHistory)
  const [showHistory, setShowHistory] = useState(false)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => { inputRef.current?.focus() }, [])

  // Spec do gráfico é derivada, não estado: recalcular é barato e evita a
  // classe de bug em que a tabela mostra um resultado e o gráfico, o anterior.
  const spec = result && !result.error ? buildChartSpec(result.columns, result.rows) : null

  // Medida padrão = primeira do spec. Reposiciona a cada resultado novo, senão
  // uma consulta herdaria a coluna escolhida na anterior — que pode nem existir.
  useEffect(() => {
    setMedida(spec ? spec.medidas[0] : '')
    setVerGrafico(true)
  }, [result])   // eslint-disable-line react-hooks/exhaustive-deps

  async function handleQuery(q?: string) {
    const text = (q ?? pergunta).trim()
    if (!text || loading) return
    setLoading(true)
    setResult(null)
    setShowSQL(false)

    try {
      const r = await fetch('/api/v2/farol/ai/query', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pergunta: text }),
      })
      const data: QueryResult = await r.json()
      if (!r.ok) {
        setResult({ pergunta: text, sql: '', columns: [], rows: [], row_count: 0, model: '', error: (data as { error?: string }).error ?? 'Erro desconhecido' })
      } else {
        setResult(data)
        const newHistory = [text, ...history.filter(h => h !== text)].slice(0, MAX_HISTORY)
        setHistory(newHistory)
        saveHistory(newHistory)
      }
    } catch (e) {
      setResult({ pergunta: text, sql: '', columns: [], rows: [], row_count: 0, model: '', error: String(e) })
    } finally {
      setLoading(false)
    }
  }

  async function handleExport() {
    if (!result || exporting) return
    setExporting(true)
    try {
      const r = await fetch('/api/v2/farol/ai/export', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pergunta: result.pergunta }),
      })
      if (!r.ok) { alert('Erro ao exportar'); return }
      const blob = await r.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `farol_consulta_${Date.now()}.xlsx`
      a.click()
      URL.revokeObjectURL(url)
    } finally {
      setExporting(false)
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleQuery()
    }
  }

  function copySQL() {
    if (!result?.sql) return
    navigator.clipboard.writeText(result.sql)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="min-h-full max-w-5xl mx-auto">
      {/* ── Cabeçalho ───────────────────────────────────────────────────── */}
      <div className="flex items-center gap-3 mb-6">
        <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-violet-600 to-indigo-600 flex items-center justify-center shadow-lg">
          <Sparkles className="h-5 w-5 text-white" />
        </div>
        <div>
          <h1 className="text-lg font-bold text-slate-800">Assistente de Consultas</h1>
          <p className="text-xs text-slate-500">Pergunte em português — a IA gera o SQL e traz os dados</p>
        </div>
        <span className="ml-auto text-[10px] font-mono bg-violet-50 text-violet-600 border border-violet-200 px-2 py-0.5 rounded-full">
          GLM Z.AI
        </span>
      </div>

      {/* ── Input ───────────────────────────────────────────────────────── */}
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm overflow-hidden mb-5">
        <textarea
          ref={inputRef}
          value={pergunta}
          onChange={e => setPergunta(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Ex: Quais são os 10 RCAs com menor positivação no período atual?"
          rows={3}
          className="w-full px-5 pt-4 pb-2 text-sm text-slate-800 placeholder:text-slate-400 resize-none focus:outline-none"
        />
        <div className="flex items-center justify-between px-5 pb-4">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setShowHistory(h => !h)}
              className="flex items-center gap-1 text-xs text-slate-400 hover:text-slate-600 transition-colors"
            >
              <Clock className="h-3.5 w-3.5" />
              {history.length > 0 ? `${history.length} recentes` : 'Histórico'}
            </button>
            {pergunta && (
              <button onClick={() => setPergunta('')}
                className="text-xs text-slate-400 hover:text-slate-600">
                <RotateCcw className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
          <button
            onClick={() => handleQuery()}
            disabled={!pergunta.trim() || loading}
            className="flex items-center gap-2 px-4 py-2 rounded-lg bg-violet-600 text-white text-sm font-semibold hover:bg-violet-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? (
              <>
                <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                Consultando...
              </>
            ) : (
              <><Send className="h-4 w-4" /> Consultar</>
            )}
          </button>
        </div>

        {/* Histórico */}
        {showHistory && history.length > 0 && (
          <div className="border-t border-slate-100 px-5 py-3 space-y-1.5">
            {history.map((h, i) => (
              <button key={i} onClick={() => { setPergunta(h); setShowHistory(false) }}
                className="w-full text-left text-xs text-slate-600 hover:text-violet-700 hover:bg-violet-50 px-2 py-1 rounded transition-colors truncate">
                {h}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* ── Sugestões ───────────────────────────────────────────────────── */}
      {!result && !loading && (
        <div className="mb-6">
          <p className="text-xs text-slate-400 mb-2 font-medium uppercase tracking-wider">Sugestões</p>
          <div className="flex flex-wrap gap-2">
            {SUGESTOES.map(s => (
              <button
                key={s.label}
                onClick={() => { setPergunta(s.query); setTimeout(() => handleQuery(s.query), 0) }}
                className="text-xs px-3 py-1.5 rounded-full border border-slate-200 bg-white text-slate-600 hover:border-violet-400 hover:text-violet-700 hover:bg-violet-50 transition-colors"
              >
                {s.label}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* ── Loading ─────────────────────────────────────────────────────── */}
      {loading && (
        <div className="bg-white rounded-2xl border border-violet-100 p-8 text-center shadow-sm">
          <div className="w-8 h-8 border-3 border-violet-200 border-t-violet-600 rounded-full animate-spin mx-auto mb-3" />
          <p className="text-sm text-slate-600 font-medium">Gerando SQL e consultando os dados...</p>
          <p className="text-xs text-slate-400 mt-1">A IA está interpretando sua pergunta</p>
        </div>
      )}

      {/* ── Erro ────────────────────────────────────────────────────────── */}
      {result?.error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-5 text-sm text-red-700">
          <p className="font-semibold mb-1">Não foi possível processar a consulta</p>
          <p>{result.error}</p>
          <p className="text-xs mt-2 text-red-500">Tente reformular a pergunta ou seja mais específico.</p>
        </div>
      )}

      {/* ── Resultado ───────────────────────────────────────────────────── */}
      {result && !result.error && (
        <div className="bg-white rounded-2xl border border-slate-100 shadow-sm overflow-hidden">
          {/* Header do resultado */}
          <div className="flex items-center justify-between px-6 py-4 border-b border-slate-50">
            <div>
              <p className="text-sm font-semibold text-slate-800">
                {result.row_count} {result.row_count === 1 ? 'resultado' : 'resultados'}
              </p>
              <p className="text-xs text-slate-400 truncate max-w-md mt-0.5">"{result.pergunta}"</p>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-[10px] text-slate-400 font-mono hidden sm:block">{result.model}</span>
              <button
                onClick={() => setShowSQL(s => !s)}
                className="flex items-center gap-1 text-xs text-slate-500 hover:text-slate-700 px-2 py-1 rounded border border-slate-200 transition-colors"
              >
                SQL {showSQL ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
              </button>
              <button
                onClick={handleExport}
                disabled={exporting}
                className="flex items-center gap-1.5 text-xs font-semibold px-3 py-1.5 rounded-lg bg-emerald-600 text-white hover:bg-emerald-700 transition-colors disabled:opacity-50"
              >
                <Download className="h-3.5 w-3.5" />
                {exporting ? 'Gerando...' : 'Excel'}
              </button>
            </div>
          </div>

          {/* SQL collapsível */}
          {showSQL && (
            <div className="px-6 py-3 bg-slate-950 text-slate-300 text-xs font-mono border-b border-slate-800 relative">
              <button onClick={copySQL}
                className="absolute top-3 right-4 text-slate-500 hover:text-slate-300 transition-colors">
                {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
              </button>
              <pre className="whitespace-pre-wrap pr-6 overflow-x-auto">{result.sql}</pre>
            </div>
          )}

          {/* Gráfico — só quando o formato do resultado comporta */}
          {spec && medida && (
            <div className="border-b border-slate-100">
              <div className="flex items-center gap-1 px-4 pt-3">
                <button
                  onClick={() => setVerGrafico(true)}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors ${
                    verGrafico ? 'bg-violet-50 text-violet-700' : 'text-slate-400 hover:text-slate-600'
                  }`}
                >
                  <BarChart3 className="h-3.5 w-3.5" /> Gráfico
                </button>
                <button
                  onClick={() => setVerGrafico(false)}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors ${
                    !verGrafico ? 'bg-violet-50 text-violet-700' : 'text-slate-400 hover:text-slate-600'
                  }`}
                >
                  <Table2 className="h-3.5 w-3.5" /> Só tabela
                </button>
              </div>
              {verGrafico && (
                <AssistenteChart spec={spec} rows={result.rows} medida={medida} onMedida={setMedida} />
              )}
            </div>
          )}

          {/* Tabela */}
          {result.rows.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-slate-100 bg-slate-50">
                    {result.columns.map(col => (
                      <th key={col}
                        className="px-4 py-3 text-left font-semibold text-slate-600 uppercase tracking-wider whitespace-nowrap">
                        {col}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-50">
                  {result.rows.map((row, i) => (
                    <tr key={i} className={i % 2 === 1 ? 'bg-slate-50/50' : ''}>
                      {result.columns.map(col => (
                        <td key={col} className="px-4 py-2.5 text-slate-700 whitespace-nowrap tabular-nums">
                          {fmtCell(row[col], col)}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="px-6 py-8 text-center text-slate-400 text-sm">
              A consulta não retornou resultados para o período atual.
            </div>
          )}

          {/* Rodapé */}
          {result.rows.length > 0 && (
            <div className="px-6 py-3 border-t border-slate-50 bg-slate-50/40 flex items-center justify-between">
              <p className="text-[11px] text-slate-400">
                {result.row_count} linha{result.row_count !== 1 ? 's' : ''} · Enter para nova consulta
              </p>
              <button onClick={handleExport} disabled={exporting}
                className="flex items-center gap-1.5 text-xs text-emerald-700 hover:text-emerald-800 font-medium">
                <Download className="h-3.5 w-3.5" />
                Baixar planilha Excel
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
