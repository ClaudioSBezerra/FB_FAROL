import { useState, useRef, useEffect } from 'react'
import { useLocation } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAuth } from '@/contexts/AuthContext'
import { MessageCircle, X, Send, Loader2, Trash2, GraduationCap, BookOpen, Database, ChevronDown, ChevronRight } from 'lucide-react'

type ChatMode = 'tutorial' | 'dados'

interface DataResult {
  reply: string
  sql: string
  columns: string[]
  rows: Record<string, unknown>[]
  truncado: boolean
}

interface Message {
  role: 'user' | 'assistant'
  content: string
  data?: DataResult     // presente apenas em respostas do modo dados
}

const PAGE_LABELS: Record<string, string> = {
  '/dashboard/ampliar':    'Painel de Calibragem — Ampliar Slot',
  '/dashboard/reduzir':    'Painel de Calibragem — Reduzir Slot',
  '/dashboard/calibrados': 'Painel de Calibragem — Já Calibrados',
  '/dashboard/curva-a':    'Painel de Calibragem — Curva A Revisar',
  '/dashboard/ignorados':  'Painel de Calibragem — Produtos Ignorados',
  '/upload/csv':           'Importação CSV — Upload',
  '/upload/log':           'Importação CSV — Log de Importação',
  '/historico':            'Histórico de Calibragem',
  '/historico/compliance': 'Histórico — Compliance',
  '/reincidencia':         'Reincidência de Calibragem',
  '/resultados':           'Painel de Resultados',
  '/resumos':              'Resumos Executivos (IA)',
  '/pdf/gerar':            'Geração de PDF',
  '/gestao/filiais':       'Administração — Filiais e CDs',
  '/gestao/regras':        'Administração — Regras de Calibragem',
}

// Renderização básica de markdown: **bold**, \n, listas, ## headings
function renderMarkdown(text: string) {
  const lines = text.split('\n')
  return lines.map((line, i) => {
    if (line.startsWith('## ')) {
      return <h3 key={i} className="text-xs font-semibold mt-2 mb-1">{line.slice(3)}</h3>
    }
    const parts = line.split(/(\*\*[^*]+\*\*)/g).map((part, j) => {
      if (part.startsWith('**') && part.endsWith('**')) {
        return <strong key={j}>{part.slice(2, -2)}</strong>
      }
      return part
    })
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
  content: `Olá! Sou o assistente do **Farol**. 👋

No modo **Tutorial** posso te ajudar com:
- Como calibrar, aprovar ou rejeitar propostas
- O que significam os indicadores e alertas
- Como importar um CSV
- Regras da Curva A, Curva ABC e delta

Mude para o modo **Dados** se quiser que eu consulte o sistema (ex: *"quantas propostas pendentes no CD FL 11?"*).`,
}

const WELCOME_DADOS: Message = {
  role: 'assistant',
  content: `Modo **Dados** ativado. 📊

Pergunte qualquer coisa sobre o estado atual do sistema. Exemplos:
- *"Quantas propostas pendentes temos no CD FL 11?"*
- *"Top 10 produtos com maior delta"*
- *"Quem importou CSV essa semana?"*
- *"Listar destinatários ativos do CD FL 11"*

Eu gero a consulta SQL, executo no banco em modo somente-leitura e mostro o resultado em tabela.`,
}

// ─── Renderização de tabela do resultado de dados ───────────────────────────
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
                  <td key={c} className="px-2 py-1 whitespace-nowrap">
                    {formatCell(row[c])}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {data.truncado && (
        <p className="text-[10px] italic text-amber-700">⚠ Resultado limitado em 100 linhas.</p>
      )}
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

function formatCell(v: unknown): string {
  if (v == null) return '—'
  if (typeof v === 'object') return JSON.stringify(v)
  if (typeof v === 'number') return v.toLocaleString('pt-BR')
  return String(v)
}

// ─── Componente principal ───────────────────────────────────────────────────
const STORAGE_KEY = 'sp-ajuda-chat-state-v1'

interface PersistedState {
  mode: ChatMode
  messages: Message[]
}

function loadPersisted(): PersistedState | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    return JSON.parse(raw) as PersistedState
  } catch { return null }
}

export function AjudaChat() {
  const { token } = useAuth()
  const location  = useLocation()
  const [open, setOpen]       = useState(false)
  const persisted = loadPersisted()
  const [mode, setMode]       = useState<ChatMode>(persisted?.mode ?? 'tutorial')
  const [messages, setMessages] = useState<Message[]>(
    persisted?.messages?.length
      ? persisted.messages
      : [persisted?.mode === 'dados' ? WELCOME_DADOS : WELCOME_TUTORIAL]
  )
  const [input, setInput]     = useState('')
  const [loading, setLoading] = useState(false)

  // Persiste estado no localStorage a cada mudança
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ mode, messages }))
    } catch { /* quota/incognito */ }
  }, [mode, messages])
  const bottomRef = useRef<HTMLDivElement>(null)
  const inputRef  = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) setTimeout(() => inputRef.current?.focus(), 50)
  }, [open])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, loading])

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
        const pageCtx = PAGE_LABELS[location.pathname] ?? location.pathname
        const apiMessages = history
          .filter(m => m.role !== 'assistant' || (m !== WELCOME_TUTORIAL && m !== WELCOME_DADOS))
          .slice(-6)
          .map(m => ({ role: m.role, content: m.content }))
        const res = await fetch('/api/sp/ajuda/chat', {
          method:  'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
          body:    JSON.stringify({ messages: apiMessages, context: pageCtx }),
        })
        const data = await res.json()
        if (!res.ok) throw new Error(data.error ?? 'Erro desconhecido')
        setMessages(prev => [...prev, { role: 'assistant', content: data.reply }])
      } else {
        // modo dados — envia histórico das últimas 4 trocas (com SQL anterior
        // anexado na mensagem do assistente para a IA poder fazer follow-ups).
        const histMsgs = history
          .filter(m => m !== WELCOME_DADOS && m !== WELCOME_TUTORIAL)
          .slice(-8) // até 4 pares user/assistant
          .map(m => ({
            role: m.role,
            content: m.role === 'assistant' && m.data
              ? `${m.content}\n\n[SQL executada]:\n${m.data.sql}`
              : m.content,
          }))
        const res = await fetch('/api/sp/ajuda/dados', {
          method:  'POST',
          headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
          body:    JSON.stringify({ pergunta: text, historico: histMsgs }),
        })
        const data = await res.json()
        if (!res.ok) throw new Error(data.error ?? 'Erro desconhecido')
        setMessages(prev => [...prev, { role: 'assistant', content: data.reply, data }])
      }
    } catch (err) {
      setMessages(prev => [
        ...prev,
        { role: 'assistant', content: `Desculpe, ocorreu um erro: ${(err as Error).message}` },
      ])
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      {/* Botão flutuante */}
      {!open && (
        <button
          onClick={() => setOpen(true)}
          className="fixed bottom-5 right-5 z-50 flex items-center gap-2 bg-primary text-primary-foreground shadow-lg rounded-full pl-4 pr-5 py-3 text-sm font-medium hover:bg-primary/90 transition-all hover:scale-105 active:scale-95"
          title="Assistente Farol"
        >
          <GraduationCap className="h-5 w-5" />
          Assistente
        </button>
      )}

      {/* Painel */}
      {open && (
        <div className="fixed bottom-5 right-5 z-50 flex flex-col w-[480px] h-[620px] bg-white border rounded-2xl shadow-2xl overflow-hidden">

          {/* Cabeçalho */}
          <div className="flex items-center justify-between px-4 py-3 bg-primary text-primary-foreground shrink-0">
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
                className="p-1.5 rounded hover:bg-white/20 transition-colors"
                title="Limpar conversa"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
              <button
                onClick={() => setOpen(false)}
                className="p-1.5 rounded hover:bg-white/20 transition-colors"
                title="Fechar"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
          </div>

          {/* Toggle de modo */}
          <div className="flex border-b bg-gray-50 shrink-0">
            <button
              onClick={() => trocarModo('tutorial')}
              className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                mode === 'tutorial'
                  ? 'bg-white text-primary border-b-2 border-primary'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <BookOpen className="h-3.5 w-3.5" /> Tutorial
            </button>
            <button
              onClick={() => trocarModo('dados')}
              className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                mode === 'dados'
                  ? 'bg-white text-primary border-b-2 border-primary'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              <Database className="h-3.5 w-3.5" /> Consulta de dados
            </button>
          </div>

          {/* Mensagens */}
          <div className="flex-1 overflow-y-auto p-3 space-y-3 bg-gray-50">
            {messages.map((msg, i) => (
              <div
                key={i}
                className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                {msg.role === 'assistant' && (
                  <div className="w-6 h-6 rounded-full bg-primary flex items-center justify-center shrink-0 mr-2 mt-0.5">
                    <MessageCircle className="h-3.5 w-3.5 text-primary-foreground" />
                  </div>
                )}
                <div
                  className={`${msg.data ? 'max-w-[95%]' : 'max-w-[85%]'} px-3 py-2 rounded-2xl text-xs leading-relaxed ${
                    msg.role === 'user'
                      ? 'bg-primary text-primary-foreground rounded-tr-sm'
                      : 'bg-white border rounded-tl-sm text-foreground shadow-sm'
                  }`}
                >
                  {msg.role === 'assistant'
                    ? renderMarkdown(msg.content)
                    : msg.content}
                  {msg.data && <ResultTable data={msg.data} />}
                </div>
              </div>
            ))}
            {loading && (
              <div className="flex justify-start">
                <div className="w-6 h-6 rounded-full bg-primary flex items-center justify-center shrink-0 mr-2 mt-0.5">
                  <MessageCircle className="h-3.5 w-3.5 text-primary-foreground" />
                </div>
                <div className="bg-white border rounded-2xl rounded-tl-sm px-3 py-2 shadow-sm">
                  <Loader2 className="h-4 w-4 animate-spin text-primary" />
                </div>
              </div>
            )}
            <div ref={bottomRef} />
          </div>

          {/* Input */}
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
            <Button
              size="sm"
              className="h-8 w-8 p-0 shrink-0"
              onClick={send}
              disabled={!input.trim() || loading}
            >
              <Send className="h-3.5 w-3.5" />
            </Button>
          </div>
        </div>
      )}
    </>
  )
}
