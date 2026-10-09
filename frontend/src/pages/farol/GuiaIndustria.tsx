import { useMemo, useState } from 'react'
import { ChevronDown, ChevronUp, Download, HelpCircle } from 'lucide-react'
import {
  INTRO, TOPICOS, PERGUNTAS, STATUS_ROTULO, ESCOPO_ROTULO,
  type Escopo, type Status,
} from '@/content/guiaIndustria'

const COR_STATUS: Record<Status, string> = {
  ok: 'bg-emerald-50 text-emerald-700 border-emerald-200',
  parcial: 'bg-amber-50 text-amber-700 border-amber-200',
  diferente: 'bg-sky-50 text-sky-700 border-sky-200',
  pendente: 'bg-rose-50 text-rose-700 border-rose-200',
}

const COR_ESCOPO: Record<Escopo, string> = {
  ambos: 'bg-slate-100 text-slate-600',
  web: 'bg-violet-50 text-violet-700',
  mobile: 'bg-indigo-50 text-indigo-700',
}

export default function GuiaIndustria() {
  const [escopo, setEscopo] = useState<'todos' | 'web' | 'mobile'>('todos')
  const [status, setStatus] = useState<Status | 'todos'>('todos')
  const [abertos, setAbertos] = useState<Set<string>>(new Set())

  const visiveis = useMemo(
    () => TOPICOS.filter(t =>
      (escopo === 'todos' || t.escopo === 'ambos' || t.escopo === escopo) &&
      (status === 'todos' || t.status === status)),
    [escopo, status],
  )
  const grupos = useMemo(() => {
    const m = new Map<string, typeof TOPICOS>()
    for (const t of visiveis) m.set(t.grupo, [...(m.get(t.grupo) ?? []), t])
    return [...m.entries()]
  }, [visiveis])
  const perguntas = PERGUNTAS.filter(p => escopo === 'todos' || p.escopo === 'ambos' || p.escopo === escopo)

  const contagem = (s: Status) => TOPICOS.filter(t => t.status === s).length
  const alternar = (id: string) =>
    setAbertos(prev => { const n = new Set(prev); if (n.has(id)) n.delete(id); else n.add(id); return n })

  return (
    <div className="space-y-5">
      <div className="bg-white rounded-2xl border border-slate-200 shadow-sm p-5">
        <h2 className="text-base font-bold text-slate-800">{INTRO.titulo}</h2>
        <p className="text-sm text-slate-600 mt-2">{INTRO.resumo}</p>
        <p className="text-sm text-slate-500 mt-2">{INTRO.comoLer}</p>
        <a
          href="/guia-industria-farol.pdf"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-2 mt-3 px-3 py-1.5 rounded-lg bg-violet-600 text-white text-xs font-semibold hover:bg-violet-700"
        >
          <Download className="h-3.5 w-3.5" /> Baixar em PDF
        </a>
      </div>

      {/* Placar */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        {(Object.keys(STATUS_ROTULO) as Status[]).map(s => (
          <button
            key={s}
            onClick={() => setStatus(status === s ? 'todos' : s)}
            className={`text-left rounded-xl border p-3 transition-shadow ${COR_STATUS[s]} ${status === s ? 'ring-2 ring-offset-1 ring-slate-400' : 'hover:shadow'}`}
          >
            <div className="text-2xl font-bold">{contagem(s)}</div>
            <div className="text-xs font-medium">{STATUS_ROTULO[s]}</div>
          </button>
        ))}
      </div>

      {/* Filtro de versão */}
      <div className="flex items-center gap-2 text-sm">
        <span className="text-slate-500">Mostrar:</span>
        {(['todos', 'web', 'mobile'] as const).map(e => (
          <button
            key={e}
            onClick={() => setEscopo(e)}
            className={`px-3 py-1 rounded-full border text-xs font-semibold ${escopo === e ? 'bg-slate-800 text-white border-slate-800' : 'bg-white text-slate-600 border-slate-200 hover:bg-slate-50'}`}
          >
            {e === 'todos' ? 'Tudo' : e === 'web' ? 'Versão Web' : 'Versão Mobile'}
          </button>
        ))}
      </div>

      {/* Tópicos */}
      {grupos.map(([grupo, itens]) => (
        <section key={grupo}>
          <h3 className="text-xs font-bold uppercase tracking-wide text-slate-500 mb-2">{grupo}</h3>
          <div className="space-y-2">
            {itens.map(t => {
              const aberto = abertos.has(t.id)
              return (
                <div key={t.id} className="bg-white rounded-xl border border-slate-200 overflow-hidden">
                  <button onClick={() => alternar(t.id)} className="w-full flex items-center gap-3 px-4 py-3 text-left">
                    <span className="flex-1 text-sm font-semibold text-slate-800">{t.titulo}</span>
                    <span className={`text-[10px] font-semibold px-2 py-0.5 rounded-full ${COR_ESCOPO[t.escopo]}`}>{ESCOPO_ROTULO[t.escopo]}</span>
                    <span className={`text-[10px] font-semibold px-2 py-0.5 rounded-full border ${COR_STATUS[t.status]}`}>{STATUS_ROTULO[t.status]}</span>
                    {aberto ? <ChevronUp className="h-4 w-4 text-slate-400" /> : <ChevronDown className="h-4 w-4 text-slate-400" />}
                  </button>
                  {aberto && (
                    <div className="border-t border-slate-100 px-4 py-3 grid md:grid-cols-2 gap-4 text-sm">
                      <div>
                        <div className="text-[11px] font-bold uppercase text-slate-400 mb-1">Era para ser</div>
                        <p className="text-slate-700">{t.previsto}</p>
                      </div>
                      <div>
                        <div className="text-[11px] font-bold uppercase text-slate-400 mb-1">O que ficou</div>
                        <p className="text-slate-700">{t.ficou}</p>
                      </div>
                      {t.nota && (
                        <p className="md:col-span-2 text-xs text-amber-700 bg-amber-50 border border-amber-100 rounded-lg px-3 py-2">{t.nota}</p>
                      )}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </section>
      ))}
      {grupos.length === 0 && <p className="text-sm text-slate-500">Nenhum tópico com esse filtro.</p>}

      {/* Perguntas */}
      <section>
        <h3 className="text-xs font-bold uppercase tracking-wide text-slate-500 mb-2 flex items-center gap-1.5">
          <HelpCircle className="h-3.5 w-3.5" /> Perguntas para a reunião ({perguntas.length})
        </h3>
        <div className="space-y-2">
          {perguntas.map(p => (
            <div key={p.n} className="bg-white rounded-xl border border-slate-200 px-4 py-3 text-sm">
              <div className="flex items-center gap-2">
                <span className="w-6 h-6 rounded-full bg-violet-600 text-white text-xs font-bold flex items-center justify-center">{p.n}</span>
                <span className="font-semibold text-slate-800 flex-1">{p.titulo}</span>
                <span className={`text-[10px] font-semibold px-2 py-0.5 rounded-full ${COR_ESCOPO[p.escopo]}`}>{ESCOPO_ROTULO[p.escopo]}</span>
              </div>
              <p className="text-slate-600 mt-2">{p.contexto}</p>
              <p className="text-slate-500 mt-1"><b>Hoje:</b> {p.hoje}</p>
              <p className="text-slate-800 mt-1"><b>Precisamos decidir:</b> {p.decidir}</p>
              {p.decisao && <p className="text-emerald-700 bg-emerald-50 border border-emerald-100 rounded-lg px-3 py-2 mt-2"><b>Decisão:</b> {p.decisao}</p>}
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
