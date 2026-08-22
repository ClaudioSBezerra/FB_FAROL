// FarolResumoEnvio — cadastro de telefone, link por token e disparo do WhatsApp.
//
// O disparo é MANUAL, por wa.me: o botão abre o WhatsApp com a mensagem pronta
// para aquele número, e a pessoa toca em enviar. Não é integração e não
// pretende ser — para cinco destinatários uma vez por semana, a API oficial da
// Meta exige verificação de empresa, número dedicado e template aprovado, o que
// custaria semanas para mandar cinco mensagens.
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { MessageCircle, Copy, Check, Link2, XCircle, Eye } from 'lucide-react'

interface Destinatario {
  user_id: string
  nome: string
  email: string
  persona: string
  telefone: string
  link: string
  acessos: number
  ultimo_acesso: string
}

const ROTULO_PERSONA: Record<string, string> = {
  ceo: 'CEO', diretor: 'Diretor', gerente_geral: 'Gerente Geral',
  ggv: 'GGV', supervisor: 'Supervisor',
}

export default function FarolResumoEnvio() {
  const qc = useQueryClient()
  const [rascunho, setRascunho] = useState<Record<string, string>>({})
  const [copiado, setCopiado] = useState('')

  const lista = useQuery<Destinatario[]>({
    queryKey: ['farol-resumo-envio'],
    queryFn: async () => {
      const r = await fetch('/api/v2/farol/resumo/envio')
      if (!r.ok) throw new Error('Falha ao carregar')
      return r.json()
    },
  })

  const acao = useMutation({
    mutationFn: async (a: { userId: string; acao?: string; telefone?: string }) => {
      const url = '/api/v2/farol/resumo/envio' + (a.acao ? `?acao=${a.acao}` : '')
      const r = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ user_id: a.userId, telefone: a.telefone ?? '' }),
      })
      if (!r.ok) throw new Error((await r.json()).error ?? 'Falha')
      return r.json()
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['farol-resumo-envio'] }),
  })

  // A mensagem já vai escrita. Sem isso a pessoa digitaria algo diferente para
  // cada um, e o que chega a cinco gestores deixaria de ser a mesma coisa.
  const linkWhats = (d: Destinatario) => {
    const texto = `Olá ${d.nome.split(' ')[0]}, seu quadro do Farol desta semana — Dinheiro na Mesa:\n\n${d.link}`
    return `https://wa.me/${d.telefone}?text=${encodeURIComponent(texto)}`
  }

  if (lista.isLoading) return <div className="p-8 text-slate-500">Carregando…</div>
  if (lista.error) return <div className="p-8 text-rose-700">Falha ao carregar os destinatários.</div>

  const ds = lista.data ?? []

  return (
    <div className="max-w-4xl mx-auto px-4 sm:px-5 py-6 sm:py-8 text-slate-800">
      <header className="border-b-2 border-slate-200 pb-5 mb-6">
        <p className="text-xs uppercase tracking-widest font-semibold text-slate-400">
          Farol de Vendas · resumo semanal
        </p>
        <h1 className="text-xl sm:text-2xl font-bold mt-2">Envio do quadro</h1>
        <p className="text-slate-500 mt-1 text-sm">
          O e-mail sai sozinho toda segunda de manhã. O WhatsApp é manual:
          o botão abre a conversa com a mensagem pronta.
        </p>
      </header>

      {ds.length === 0 && (
        <p className="text-slate-500">
          Ninguém com o resumo semanal ligado. Ative em Usuários para aparecer aqui.
        </p>
      )}

      <div className="space-y-3">
        {ds.map(d => {
          const tel = rascunho[d.user_id] ?? d.telefone
          const telMudou = tel !== d.telefone
          const temLink = !!d.link
          const podeEnviar = temLink && d.telefone.length >= 12

          return (
            <div key={d.user_id} className="border border-slate-200 rounded-lg p-4">
              <div className="flex items-start justify-between gap-3 flex-wrap">
                <div className="min-w-0">
                  <div className="font-semibold">{d.nome}</div>
                  <div className="text-xs text-slate-500 mt-0.5">
                    {d.email} · {ROTULO_PERSONA[d.persona] ?? d.persona}
                  </div>
                </div>
                {/* Contador de aberturas: é o detector de vazamento, não
                    estatística. Link pessoal com muitos acessos circulou. */}
                {temLink && (
                  <div className="text-xs text-slate-400 flex items-center gap-1.5 whitespace-nowrap">
                    <Eye className="h-3.5 w-3.5" />
                    {d.acessos} {d.acessos === 1 ? 'abertura' : 'aberturas'}
                    {d.ultimo_acesso && ` · ${d.ultimo_acesso}`}
                  </div>
                )}
              </div>

              <div className="flex items-end gap-2 mt-4 flex-wrap">
                <div>
                  <label className="block text-xs text-slate-500 mb-1">
                    WhatsApp — 55 + DDD + número
                  </label>
                  <input
                    value={tel}
                    onChange={e => setRascunho({ ...rascunho, [d.user_id]: e.target.value })}
                    placeholder="5562999998888"
                    inputMode="numeric"
                    className="border border-slate-300 rounded px-3 py-2 text-sm w-52 tabular-nums
                               focus:outline-none focus:ring-2 focus:ring-teal-600/30 focus:border-teal-600"
                  />
                </div>
                {telMudou && (
                  <button
                    onClick={() => acao.mutate({ userId: d.user_id, telefone: tel })}
                    disabled={acao.isPending}
                    className="bg-slate-700 hover:bg-slate-800 text-white text-sm px-4 py-2 rounded">
                    Salvar
                  </button>
                )}
              </div>

              <div className="flex items-center gap-2 mt-4 flex-wrap">
                {!temLink ? (
                  <button
                    onClick={() => acao.mutate({ userId: d.user_id, acao: 'gerar' })}
                    disabled={acao.isPending}
                    className="flex items-center gap-2 border border-slate-300 hover:bg-slate-50
                               text-sm px-4 py-2 rounded">
                    <Link2 className="h-4 w-4" /> Gerar link
                  </button>
                ) : (
                  <>
                    <a
                      href={podeEnviar ? linkWhats(d) : undefined}
                      target="_blank" rel="noreferrer"
                      className={`flex items-center gap-2 text-sm px-4 py-2 rounded text-white ${
                        podeEnviar ? 'bg-emerald-600 hover:bg-emerald-700' : 'bg-slate-300 cursor-not-allowed'
                      }`}
                      title={podeEnviar ? '' : 'Cadastre o telefone primeiro'}>
                      <MessageCircle className="h-4 w-4" /> Enviar no WhatsApp
                    </a>
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(d.link)
                        setCopiado(d.user_id)
                        setTimeout(() => setCopiado(''), 1800)
                      }}
                      className="flex items-center gap-2 border border-slate-300 hover:bg-slate-50
                                 text-sm px-4 py-2 rounded">
                      {copiado === d.user_id
                        ? <><Check className="h-4 w-4 text-emerald-600" /> Copiado</>
                        : <><Copy className="h-4 w-4" /> Copiar link</>}
                    </button>
                    <button
                      onClick={() => {
                        if (confirm(`Revogar o link de ${d.nome}? Ele para de funcionar imediatamente, inclusive se já tiver sido enviado.`))
                          acao.mutate({ userId: d.user_id, acao: 'revogar' })
                      }}
                      className="flex items-center gap-2 text-rose-700 hover:bg-rose-50
                                 text-sm px-3 py-2 rounded">
                      <XCircle className="h-4 w-4" /> Revogar
                    </button>
                  </>
                )}
              </div>

              {temLink && (
                <p className="text-[11px] text-slate-400 mt-3 break-all font-mono">{d.link}</p>
              )}
            </div>
          )
        })}
      </div>

      <div className="mt-8 border border-amber-200 bg-amber-50 rounded-lg p-4 text-sm text-amber-900">
        <p className="font-semibold mb-1">O link abre sem senha</p>
        <p className="text-amber-800">
          Quem receber vê o quadro daquela pessoa, sem login — foi assim que
          decidimos para o WhatsApp não ter fricção. Por isso o link é individual:
          se um vazar, revogue só o dele. O contador de aberturas acima é o que
          denuncia um link que circulou.
        </p>
      </div>
    </div>
  )
}
