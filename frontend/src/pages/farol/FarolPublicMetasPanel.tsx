import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Target, TrendingDown, TrendingUp, AlertTriangle } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────

interface MetaVinculo {
  id: number
  industria_nome: string
  tipo_metrica_nome: string
}

interface Vigencia {
  id: number
  data_inicio: string
  data_fim: string
  status: 'aberta' | 'fechada'
}

interface RealizadoRede {
  rede_nome: string
  valor: number
  atingiu: boolean
}

interface PainelFaixa {
  faixa: number
  valor_meta: number
  atingida: boolean
}

interface Realizado {
  realizado_total: number
  projecao: number
  parcial: boolean
  redes: RealizadoRede[]
}

interface Painel {
  industria_nome: string
  tipo_metrica_nome: string
  realizado: Realizado
  faixa_atual: PainelFaixa | null
  proxima_faixa: PainelFaixa | null
  delta: number
  recortes?: Record<string, Realizado>
}

const RECORTES = [
  { value: 'dia_anterior', label: 'Ontem' },
  { value: 'semana', label: 'Semana' },
  { value: 'mes', label: 'Mês' },
  { value: 'ano_corrente', label: 'Ano' },
]

// ─── Page — painel mobile público, mesmo padrão sem login de FarolPublicPanel ──

export default function FarolPublicMetasPanel() {
  const params = useParams<{ cnpj?: string; cod?: string; codRca?: string }>()
  const isRca = window.location.pathname.includes('/rca/')
  const cnpj = (params.cnpj || (isRca ? params.cod : '') || '').replace(/\D/g, '')
  const scope: 'sup' | 'rca' = isRca ? 'rca' : 'sup'
  const scopeCod = isRca ? (params.codRca || '') : (params.cod || '')

  const [vinculoID, setVinculoID] = useState('')
  const [vigenciaID, setVigenciaID] = useState('')
  const [aba, setAba] = useState<'oficiais' | 'projecao'>('oficiais')

  const { data: vinculos = [] } = useQuery<MetaVinculo[]>({
    queryKey: ['public-metas-vinculos', cnpj],
    queryFn: async () => {
      const r = await fetch(`/api/farol/public/metas-vinculos?cnpj=${cnpj}`)
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: !!cnpj,
  })

  const { data: vigencias = [] } = useQuery<Vigencia[]>({
    queryKey: ['public-metas-vigencias', cnpj, vinculoID],
    queryFn: async () => {
      const r = await fetch(`/api/farol/public/metas-vigencias?cnpj=${cnpj}&vinculo_id=${vinculoID}`)
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: !!cnpj && !!vinculoID,
  })

  const { data: painel, isLoading } = useQuery<Painel>({
    queryKey: ['public-metas-painel', cnpj, scope, scopeCod, vinculoID, vigenciaID],
    queryFn: async () => {
      const p = new URLSearchParams({ cnpj, scope, cod: scopeCod, vinculo_id: vinculoID, vigencia_id: vigenciaID, recortes: '1' })
      const r = await fetch(`/api/farol/public/metas-painel?${p}`)
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: !!cnpj && !!scopeCod && !!vinculoID && !!vigenciaID,
  })

  if (!cnpj || !scopeCod) {
    return <div className="p-6 text-center text-sm text-muted-foreground">Link inválido.</div>
  }

  return (
    <div className="min-h-screen bg-slate-50 p-4 space-y-4 max-w-md mx-auto">
      <div>
        <h1 className="text-lg font-semibold">Metas por Indústria</h1>
        <p className="text-xs text-muted-foreground">{scope === 'sup' ? 'Visão do Supervisor' : 'Visão do RCA'}</p>
      </div>

      <div className="space-y-2">
        <select
          className="w-full border rounded-md p-2 text-sm bg-white"
          value={vinculoID}
          onChange={e => { setVinculoID(e.target.value); setVigenciaID('') }}
        >
          <option value="">Selecione a Indústria...</option>
          {vinculos.map(v => (
            <option key={v.id} value={v.id}>{v.industria_nome} — {v.tipo_metrica_nome}</option>
          ))}
        </select>

        {vinculoID && (
          <select
            className="w-full border rounded-md p-2 text-sm bg-white"
            value={vigenciaID}
            onChange={e => setVigenciaID(e.target.value)}
          >
            <option value="">Selecione o período...</option>
            {vigencias.map(v => (
              <option key={v.id} value={v.id}>{v.data_inicio} – {v.data_fim}</option>
            ))}
          </select>
        )}
      </div>

      {isLoading && <p className="text-center text-sm text-muted-foreground py-8">Carregando...</p>}

      {painel && (
        <div className="space-y-3">
          <div className="flex gap-1 border-b">
            <button
              className={`flex-1 px-3 py-2.5 text-sm font-bold uppercase border-b-2 ${aba === 'oficiais' ? 'border-slate-800 text-slate-900' : 'border-transparent text-muted-foreground'}`}
              onClick={() => setAba('oficiais')}
            >
              Oficial
            </button>
            <button
              className={`flex-1 px-3 py-2.5 text-sm font-bold uppercase border-b-2 ${aba === 'projecao' ? 'border-slate-800 text-slate-900' : 'border-transparent text-muted-foreground'}`}
              onClick={() => setAba('projecao')}
            >
              Projeção
            </button>
          </div>

          {aba === 'oficiais' ? (
            <>
              <div className="bg-white border rounded-xl p-4">
                <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                  <Target className="w-4 h-4" /> Realizado
                </div>
                <div className="text-3xl font-bold">{painel.realizado.realizado_total.toLocaleString('pt-BR', { maximumFractionDigits: 2 })}</div>
                {painel.realizado.parcial && <span className="text-xs text-amber-600">Mês em andamento</span>}
              </div>

              <div className={`rounded-xl p-4 border ${painel.delta > 0 ? 'bg-amber-50 border-amber-200' : 'bg-emerald-50 border-emerald-200'}`}>
                <div className="flex items-center gap-2 text-xs mb-1">
                  {painel.delta > 0 ? <TrendingDown className="w-4 h-4 text-amber-600" /> : <TrendingUp className="w-4 h-4 text-emerald-600" />}
                  {painel.delta > 0 ? `Falta ${painel.delta.toLocaleString('pt-BR', { maximumFractionDigits: 2 })} pra bater a meta` : 'Meta batida!'}
                </div>
                {painel.proxima_faixa && (
                  <div className="text-xs text-muted-foreground">Próxima meta (Faixa {painel.proxima_faixa.faixa}): {painel.proxima_faixa.valor_meta}</div>
                )}
              </div>

              <div className="bg-white border rounded-xl overflow-hidden">
                <div className="px-3 py-2 text-xs font-medium text-muted-foreground border-b">Suas Redes</div>
                {painel.realizado.redes.length === 0 && (
                  <div className="px-3 py-4 text-sm text-muted-foreground text-center">Nenhuma Rede neste recorte</div>
                )}
                {painel.realizado.redes.map((r, i) => (
                  <div key={i} className="px-3 py-2 flex items-center justify-between border-b last:border-0 text-sm">
                    <span>{r.rede_nome}</span>
                    <span className={`text-xs px-2 py-0.5 rounded-full ${r.atingiu ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-100 text-slate-600'}`}>
                      {r.atingiu ? 'Coberta' : 'Não coberta'}
                    </span>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <>
              <div className="flex items-start gap-2 bg-amber-50 border border-amber-200 rounded-xl p-3 text-xs text-amber-800">
                <AlertTriangle className="w-4 h-4 mt-0.5 shrink-0" />
                <span>Estimativa com base no ritmo até hoje — não é um número oficial do programa.</span>
              </div>
              <div className="bg-white border rounded-xl p-4">
                <div className="text-muted-foreground text-xs mb-1">Projeção de fechamento</div>
                <div className="text-3xl font-bold">{painel.realizado.projecao.toLocaleString('pt-BR', { maximumFractionDigits: 2 })}</div>
              </div>
              {painel.recortes && (
                <div className="bg-white border rounded-xl overflow-hidden">
                  {RECORTES.map(r => (
                    <div key={r.value} className="px-3 py-2 flex items-center justify-between border-b last:border-0 text-sm">
                      <span>{r.label}</span>
                      <span className="font-semibold">{painel.recortes?.[r.value]?.realizado_total.toLocaleString('pt-BR', { maximumFractionDigits: 2 }) ?? '—'}</span>
                    </div>
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
