import { useState, useRef, useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAuth } from '@/contexts/AuthContext'
import { MessageCircle, X, Send, Loader2, Trash2, GraduationCap, BookOpen, Database, ChevronDown, ChevronRight } from 'lucide-react'

// Assistente de treinamento do FAROL — mesmo formato do SMARTPICK (AjudaChat).
//   Tutorial → /api/v2/farol/ai/chat  (explica uso + como dados são
//              carregados/calculados/exibidos; não consulta o banco)
//   Dados    → /api/v2/farol/ai/query (text-to-SQL existente; mostra tabela)

type ChatMode = 'tutorial' | 'dados'

interface DataResult {
  columns: string[]
  rows: Record<string, unknown>[]
  sql: string
  rowCount: number
}

interface Message {
  role: 'user' | 'assistant'
  content: string
  data?: DataResult
}

function renderMarkdown(text: string) {
  const lines = text.split('\n')
  return lines.map((line, i) => {
    if (line.startsWith('## ')) {
      return <h3 key={i} className="text-xs font-semibold mt-2 mb-1">{line.slice(3)}</h3>
    }
    const parts = line.split(/(\*\*[^*]+\*\*)/g).map((part, j) =>
      part.startsWith('**') && part.endsWith('**')
        ? <strong key={j}>{part.slice(2, -2)}</strong>
        : part,
    )
    const isBullet = line.trimStart().startsWith('- ') || line.trimStart().startsWith('• ')
    const isNumbered = /^\d+\./.test(line.trimStart())
    return (
      <span key={i} className={`block ${isBullet || isNumbered ? 'pl-3' : ''} ${i > 0 ? 'mt-0.5' : ''}`}>
        {parts}
      </span>
    )
  })
}

const WELCOME_TUTORIAL: Message = {
  role: 'assistant',
  content: `Olá! Sou o assistente do **Farol de Vendas**. 👋

No modo **Tutorial** te explico:
- Como usar os fluxos (Faturado, Transmitido, Cancel./Devol., Cortado) e as visões (Indústria, Gerência, Equipe, Rede, Departamento)
- O que é **Venda Líquida** e os botões "Incluir Bonificação/Transferência/..."
- Como os dados são **carregados, calculados e exibidos** (positivação, mix, semáforo)

Mude para **Consulta de dados** se quiser um número real (ex.: *"top 10 indústrias por faturamento"*).`,
}

const WELCOME_DADOS: Message = {
  role: 'assistant',
  content: `Modo **Consulta de dados** ativado. 📊

Pergunte em português. Exemplos:
- *"Top 10 indústrias por valor faturado"*
- *"RCAs com menor positivação"*
- *"Clientes com maior mix"*

Eu gero a consulta SQL, executo em modo somente-leitura e mostro a tabela.`,
}

function formatCell(v: unknown): string {
  if (v == null) return '—'
  if (typeof v === 'object') return JSON.stringify(v)
  if (typeof v === 'number') return v.toLocaleString('pt-BR')
  return String(v)
}

function ResultTable({ data }: { data: DataResult }) {
  const [showSQL, setShowSQL] = useState(false)
  if (data.rows.length === 0) {
    return <p className="text-xs italic text-muted-foreground mt-2">Nenhum resultado encontrado.</p>
  }
  return (
    <div className="mt-2 space-y-2">
      <div className="overflow-x-auto border rounded">
        <table className="text-[11px] w-full">
          <thead>
            <tr className="bg-gray-100 border-b">
              {data.columns.map(c => (
                <th key={c} className="text-left px-2 py-1 font-semibold whitespace-nowrap">{c}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.rows.map((row, i) => (
              <tr key={i} className="border-b last:border-0 odd:bg-gray-50">
                {data.columns.map(c => (
                  <td key={c} className="px-2 py-1 whitespace-nowrap">{formatCell(row[c])}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <button
        onClick={() => setShowSQL(s => !s)}
        className="flex items-center gap-1 text-[10px] text-muted-foreground hover:text-foreground"
      >
        {showSQL ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        Ver SQL gerada
      </button>
      {showSQL && (
        <pre className="text-[10px] bg-gray-900 text-green-300 p-2 rounded overflow-x-auto whitespace-pre-wrap">{data.sql}</pre>
      )}
    </div>
  )
}

const STORAGE_KEY = 'farol-ajuda-chat-state-v1'

interface PersistedState { mode: ChatMode; messages: Message[] }

function loadPersisted(): PersistedState | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as PersistedState) : null
  } catch { return null }
}

export function FarolAjudaChat() {
  const { token } = useAuth()
  const location = useLocation()
  const [open, setOpen] = useState(false)
  const persisted = loadPersisted()
  const [mode, setMode] = useState<ChatMode>(persisted?.mode ?? 'tutorial')
  const [messages, setMessages] = useState<Message[]>(
    persisted?.messages?.length
      ? persisted.messages
      : [persisted?.mode === 'dados' ? WELCOME_DADOS : WELCOME_TUTORIAL],
  )
  const [input, setInput] = useState('')
  const [loading, setLoading] = useState(false)
  const bottomRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode, messages })) } catch { /* quota */ }
  }, [mode, messages])
  useEffect(() => { if (open) setTimeout(() => inputRef.current?.focus(), 50) }, [open])
  useEffect(() => { bottomRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [messages, loading])

  function trocarModo(novo: ChatMode) {
    setMode(novo)
    setMessages([novo === 'tutorial' ? WELCOME_TUTORIAL : WELCOME_DADOS])
  }

  async function send() {
    const text = input.trim()
    if (!text || loading) return
    setInput('')
    const userMsg: Message = { role: 'user', content: text }
    const history = [...messages, userMsg]
    setMessages(history)
    setLoading(true)
    try {
      if (mode === 'tutorial') {
        const apiMessages = history
          .filter(m => m !== WELCOME_TUTORIAL && m !== WELCOME_DADOS)
          .slice(-6)
          .map(m => ({ role: m.role, content: m.content }))
        const res = await fetch('/api/v2/farol/ai/chat', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
          body: JSON.stringify({ messages: apiMessages, context: location.pathname }),
        })
        const data = await res.json()
        if (!res.ok) throw new Error(data.error ?? 'Erro desconhecido')
        setMessages(prev => [...prev, { role: 'assistant', content: data.reply }])
      } else {
        const res = await fetch('/api/v2/farol/ai/query', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
          body: JSON.stringify({ pergunta: text }),
        })
        const data = await res.json()
        if (!res.ok) throw new Error(data.error ?? 'Erro desconhecido')
        const result: DataResult = {
          columns: data.columns ?? [],
          rows: data.rows ?? [],
          sql: data.sql ?? '',
          rowCount: data.row_count ?? (data.rows?.length ?? 0),
        }
        const reply = result.rowCount === 0 ? 'Nenhum resultado encontrado.' : `${result.rowCount} resultado(s):`
        setMessages(prev => [...prev, { role: 'assistant', content: reply, data: result }])
      }
    } catch (err) {
      setMessages(prev => [...prev, { role: 'assistant', content: `Desculpe, ocorreu um erro: ${(err as Error).message}` }])
    } finally {
      setLoading(false)
    }
  }

  // Só aparece nas páginas do Farol (não em SMARTPICK/outros produtos do app).
  if (!location.pathname.startsWith('/farol')) return null

  return (
    <>
      {!open && (
        <button
          onClick={() => setOpen(true)}
          className="fixed bottom-5 right-5 z-50 flex items-center gap-2 bg-slate-800 text-white shadow-lg rounded-full pl-4 pr-5 py-3 text-sm font-medium hover:bg-slate-700 transition-all hover:scale-105 active:scale-95"
          title="Assistente Farol"
        >
          <GraduationCap className="h-5 w-5" />
          Assistente
        </button>
      )}

      {open && (
        <div className="fixed bottom-5 right-5 z-50 flex flex-col w-[480px] h-[620px] max-w-[95vw] max-h-[85vh] bg-white border rounded-2xl shadow-2xl overflow-hidden">
          <div className="flex items-center justify-between px-4 py-3 bg-slate-800 text-white shrink-0">
            <div className="flex items-center gap-2">
              <GraduationCap className="h-5 w-5" />
              <div>
                <p className="text-sm font-semibold leading-tight">Assistente Farol</p>
                <p className="text-[10px] opacity-75 leading-tight">
                  {mode === 'tutorial' ? 'Modo tutorial' : 'Modo dados — consulta o sistema'}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-1">
              <button
                onClick={() => setMessages([mode === 'tutorial' ? WELCOME_TUTORIAL : WELCOME_DADOS])}
                className="p-1.5 rounded hover:bg-white/20 transition-colors" title="Limpar conversa"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
              <button onClick={() => setOpen(false)} className="p-1.5 rounded hover:bg-white/20 transition-colors" title="Fechar">
                <X className="h-4 w-4" />
              </button>
            </div>
          </div>

          <div className="flex border-b bg-gray-50 shrink-0">
            <button
              onClick={() => trocarModo('tutorial')}
              className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                mode === 'tutorial' ? 'bg-white text-slate-800 border-b-2 border-slate-800' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <BookOpen className="h-3.5 w-3.5" /> Tutorial
            </button>
            <button
              onClick={() => trocarModo('dados')}
              className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                mode === 'dados' ? 'bg-white text-slate-800 border-b-2 border-slate-800' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Database className="h-3.5 w-3.5" /> Consulta de dados
            </button>
          </div>

          <div className="flex-1 overflow-y-auto p-3 space-y-3 bg-gray-50">
            {messages.map((msg, i) => (
              <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                {msg.role === 'assistant' && (
                  <div className="w-6 h-6 rounded-full bg-slate-800 flex items-center justify-center shrink-0 mr-2 mt-0.5">
                    <MessageCircle className="h-3.5 w-3.5 text-white" />
                  </div>
                )}
                <div
                  className={`${msg.data ? 'max-w-[95%]' : 'max-w-[85%]'} px-3 py-2 rounded-2xl text-xs leading-relaxed ${
                    msg.role === 'user'
                      ? 'bg-slate-800 text-white rounded-tr-sm'
                      : 'bg-white border rounded-tl-sm text-foreground shadow-sm'
                  }`}
                >
                  {msg.role === 'assistant' ? renderMarkdown(msg.content) : msg.content}
                  {msg.data && <ResultTable data={msg.data} />}
                </div>
              </div>
            ))}
            {loading && (
              <div className="flex justify-start">
                <div className="w-6 h-6 rounded-full bg-slate-800 flex items-center justify-center shrink-0 mr-2 mt-0.5">
                  <MessageCircle className="h-3.5 w-3.5 text-white" />
                </div>
                <div className="bg-white border rounded-2xl rounded-tl-sm px-3 py-2 shadow-sm">
                  <Loader2 className="h-4 w-4 animate-spin text-slate-800" />
                </div>
              </div>
            )}
            <div ref={bottomRef} />
          </div>

          <div className="shrink-0 border-t bg-white px-3 py-2.5 flex gap-2 items-center">
            <Input
              ref={inputRef}
              value={input}
              onChange={e => setInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() } }}
              placeholder={mode === 'tutorial' ? 'Digite sua dúvida...' : 'Pergunte sobre os dados...'}
              className="text-xs h-8 flex-1"
              disabled={loading}
            />
            <Button size="sm" className="h-8 w-8 p-0 shrink-0" onClick={send} disabled={!input.trim() || loading}>
              <Send className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      )}
    </>
  )
}
