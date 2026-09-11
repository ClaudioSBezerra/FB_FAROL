import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Target, TrendingDown, TrendingUp, AlertTriangle } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────

interface MetaVinculo {
  id: number
  industria_id: number
  industria_nome: string
  tipo_metrica_nome: string
  formula_codigo: string
}

interface Vigencia {
  id: number
  data_inicio: string
  data_fim: string
  status: 'aberta' | 'fechada'
}

interface RealizadoRede {
  cod_princ: string
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

// ─── Types — visão combinada (Cobertura + Sortimento juntos, mesmo pedido
// da JC aplicado ao mobile pra manter paridade com o painel web) ─────────────

interface PainelMetricaResumo {
  realizado_total: number
  projecao: number
  parcial: boolean
  faixa_atual: PainelFaixa | null
  proxima_faixa: PainelFaixa | null
  delta: number
}

interface PainelCombinadoRede {
  cod_princ: string
  cod_rca: string
  cobertura_valor: number
  cobertura_objetivo: number
  cobertura_falta: number
  cobertura_atingiu: boolean
  sortimento_valor: number
  sortimento_objetivo: number
  sortimento_falta: number
}

interface PainelCombinado {
  industria_nome: string
  cobertura: PainelMetricaResumo
  sortimento: PainelMetricaResumo
  redes: PainelCombinadoRede[]
}

interface Industria {
  id: number
  nome: string
  cobertura?: MetaVinculo
  sortimento?: MetaVinculo
}

const RECORTES = [
  { value: 'dia_anterior', label: 'Ontem' },
  { value: 'semana', label: 'Semana' },
  { value: 'mes', label: 'Mês' },
  { value: 'ano_corrente', label: 'Ano' },
]

// Só 2 visões (orientação do Heverton, 2026-09-04: "mesma filosofia do
// Farol V1 em uso hoje") — Faturado (notas emitidas) e Transmitido
// (pedido em carteira, ainda não faturado).
const FLUXOS = [
  { value: 'faturado', label: 'Faturado' },
  { value: 'transmitido', label: 'Transmitido' },
]

const fmt = (n: number) => n.toLocaleString('pt-BR', { maximumFractionDigits: 2 })

// ─── ChipRow — seleção por toque (pedido do Claudio 11/09/2026: "se possível
// ele tocar em vez de filtrar, tem que ser seleção touch") — substitui os
// <select> nativos, ruins de mirar com o dedo e que escondem as opções atrás
// de mais um toque. Com só 1 opção nem chip mostra (nada pra escolher).
// Alvo de toque generoso (padding vertical ~12px, min ~44px de altura) e
// scroll horizontal quando a lista não cabe na tela (ex: histórico de
// vigências fechadas).
function ChipRow<T extends string>({ label, options, value, onChange }: {
  label: string
  options: Array<{ value: T; label: string }>
  value: T
  onChange: (v: T) => void
}) {
  if (options.length <= 1) return null
  return (
    <div>
      <div className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground px-0.5 mb-1.5">{label}</div>
      <div className="flex gap-2 overflow-x-auto pb-1 -mx-4 px-4 scrollbar-none">
        {options.map(opt => (
          <button
            key={opt.value}
            type="button"
            onClick={() => onChange(opt.value)}
            className={`shrink-0 whitespace-nowrap rounded-full border px-4 py-3 text-sm font-medium leading-none transition-colors active:scale-[0.97] ${
              value === opt.value
                ? 'border-slate-900 bg-slate-900 text-white'
                : 'border-slate-300 bg-white text-slate-700'
            }`}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </div>
  )
}

// ─── Page — painel mobile público, mesmo padrão sem login de FarolPublicPanel ──

export default function FarolPublicMetasPanel() {
  const params = useParams<{ cnpj?: string; cod?: string; codRca?: string }>()
  const isRca = window.location.pathname.includes('/rca/')
  const cnpj = (params.cnpj || (isRca ? params.cod : '') || '').replace(/\D/g, '')
  const scope: 'sup' | 'rca' = isRca ? 'rca' : 'sup'
  const scopeCod = isRca ? (params.codRca || '') : (params.cod || '')

  const [industriaID, setIndustriaID] = useState('')
  const [metrica, setMetrica] = useState<'cobertura' | 'sortimento' | 'combinado'>('combinado')
  const [vigenciaID, setVigenciaID] = useState('')
  const [vigenciaCombinadaKey, setVigenciaCombinadaKey] = useState('')
  const [fluxo, setFluxo] = useState('faturado')
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

  const industrias = useMemo<Industria[]>(() => {
    const porID = new Map<number, Industria>()
    for (const v of vinculos) {
      if (!porID.has(v.industria_id)) porID.set(v.industria_id, { id: v.industria_id, nome: v.industria_nome })
      const ind = porID.get(v.industria_id)!
      if (v.formula_codigo === 'cobertura_rede') ind.cobertura = v
      else if (v.formula_codigo === 'sortimento_rede') ind.sortimento = v
    }
    return Array.from(porID.values()).sort((a, b) => a.nome.localeCompare(b.nome))
  }, [vinculos])

  // Auto-seleciona a Indústria quando só existe UMA (pedido do Claudio
  // 11/09/2026: "o SUPV/RCA tem que entrar já carregando... sem ter que
  // informar") — com 2+ indústrias ainda mostra os chips de toque pra
  // escolher, mas nunca deixa a tela vazia esperando 1 escolha óbvia.
  useEffect(() => {
    if (!industriaID && industrias.length === 1) setIndustriaID(String(industrias[0].id))
  }, [industrias, industriaID])

  const industriaSelecionada = industrias.find(i => String(i.id) === industriaID)
  const metricasDisponiveis = useMemo(() => {
    const opcoes: Array<{ value: typeof metrica; label: string }> = []
    if (industriaSelecionada?.cobertura && industriaSelecionada?.sortimento) {
      opcoes.push({ value: 'combinado', label: 'Combinado (Cobertura + Sortimento)' })
    }
    if (industriaSelecionada?.cobertura) opcoes.push({ value: 'cobertura', label: industriaSelecionada.cobertura.tipo_metrica_nome })
    if (industriaSelecionada?.sortimento) opcoes.push({ value: 'sortimento', label: industriaSelecionada.sortimento.tipo_metrica_nome })
    return opcoes
  }, [industriaSelecionada])

  useEffect(() => {
    if (metricasDisponiveis.length === 0) return
    if (!metricasDisponiveis.some(m => m.value === metrica)) {
      setMetrica(metricasDisponiveis[0].value)
    }
  }, [metricasDisponiveis, metrica])

  const vinculoAtivo = metrica === 'cobertura' ? industriaSelecionada?.cobertura
    : metrica === 'sortimento' ? industriaSelecionada?.sortimento
    : undefined

  // ─── Modo individual ──────────────────────────────────────────────────────

  const { data: vigencias = [] } = useQuery<Vigencia[]>({
    queryKey: ['public-metas-vigencias', cnpj, vinculoAtivo?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/public/metas-vigencias?cnpj=${cnpj}&vinculo_id=${vinculoAtivo!.id}`)
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: !!cnpj && metrica !== 'combinado' && !!vinculoAtivo,
  })

  // Auto-seleciona a vigência VIGENTE (aberta) assim que a lista chega —
  // mesmo racional do painel web (ver FarolPainelMetas.tsx), aqui ainda
  // mais importante: o RCA em campo não pode ficar preso escolhendo período
  // antes de ver o número.
  useEffect(() => {
    if (vigencias.length === 0) return
    if (vigencias.some(v => String(v.id) === vigenciaID)) return
    const preferida = vigencias.find(v => v.status === 'aberta') ?? vigencias[0]
    setVigenciaID(String(preferida.id))
  }, [vigencias])

  const { data: painel, isLoading } = useQuery<Painel>({
    queryKey: ['public-metas-painel', cnpj, scope, scopeCod, vinculoAtivo?.id, vigenciaID, fluxo],
    queryFn: async () => {
      const p = new URLSearchParams({ cnpj, scope, cod: scopeCod, vinculo_id: String(vinculoAtivo!.id), vigencia_id: vigenciaID, fluxo, recortes: '1' })
      const r = await fetch(`/api/farol/public/metas-painel?${p}`)
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: !!cnpj && !!scopeCod && metrica !== 'combinado' && !!vinculoAtivo && !!vigenciaID,
  })

  // ─── Modo combinado ───────────────────────────────────────────────────────

  const { data: vigenciasCobertura = [] } = useQuery<Vigencia[]>({
    queryKey: ['public-metas-vigencias', cnpj, industriaSelecionada?.cobertura?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/public/metas-vigencias?cnpj=${cnpj}&vinculo_id=${industriaSelecionada!.cobertura!.id}`)
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: !!cnpj && metrica === 'combinado' && !!industriaSelecionada?.cobertura,
  })
  const { data: vigenciasSortimento = [] } = useQuery<Vigencia[]>({
    queryKey: ['public-metas-vigencias', cnpj, industriaSelecionada?.sortimento?.id],
    queryFn: async () => {
      const r = await fetch(`/api/farol/public/metas-vigencias?cnpj=${cnpj}&vinculo_id=${industriaSelecionada!.sortimento!.id}`)
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: !!cnpj && metrica === 'combinado' && !!industriaSelecionada?.sortimento,
  })

  const periodosCombinados = useMemo(() => {
    const porPeriodo = new Map(vigenciasSortimento.map(v => [`${v.data_inicio}|${v.data_fim}`, v]))
    return vigenciasCobertura
      .filter(vc => porPeriodo.has(`${vc.data_inicio}|${vc.data_fim}`))
      .map(vc => ({ chave: `${vc.data_inicio}|${vc.data_fim}`, cobertura: vc, sortimento: porPeriodo.get(`${vc.data_inicio}|${vc.data_fim}`)! }))
  }, [vigenciasCobertura, vigenciasSortimento])

  const periodoSelecionado = periodosCombinados.find(p => p.chave === vigenciaCombinadaKey)

  // Mesmo auto-select acima, aplicado ao Período do modo Combinado.
  useEffect(() => {
    if (periodosCombinados.length === 0) return
    if (periodosCombinados.some(p => p.chave === vigenciaCombinadaKey)) return
    const preferido = periodosCombinados.find(p => p.cobertura.status === 'aberta') ?? periodosCombinados[0]
    setVigenciaCombinadaKey(preferido.chave)
  }, [periodosCombinados])

  const { data: painelCombinado, isLoading: isLoadingCombinado } = useQuery<PainelCombinado>({
    queryKey: ['public-metas-painel-combinado', cnpj, scope, scopeCod, industriaSelecionada?.cobertura?.id, industriaSelecionada?.sortimento?.id, periodoSelecionado?.chave, fluxo],
    queryFn: async () => {
      const p = new URLSearchParams({
        cnpj, scope, cod: scopeCod,
        vinculo_cobertura_id: String(industriaSelecionada!.cobertura!.id),
        vigencia_cobertura_id: String(periodoSelecionado!.cobertura.id),
        vinculo_sortimento_id: String(industriaSelecionada!.sortimento!.id),
        vigencia_sortimento_id: String(periodoSelecionado!.sortimento.id),
        fluxo,
      })
      const r = await fetch(`/api/farol/public/metas-painel-combinado?${p}`)
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    enabled: !!cnpj && !!scopeCod && metrica === 'combinado' && !!periodoSelecionado,
  })

  if (!cnpj || !scopeCod) {
    return <div className="p-6 text-center text-sm text-muted-foreground">Link inválido.</div>
  }

  return (
    <div className="min-h-screen bg-slate-50 p-4 space-y-4 max-w-md mx-auto">
      <div>
        <h1 className="text-lg font-semibold">Objetivos por Indústria</h1>
        <p className="text-xs text-muted-foreground">{scope === 'sup' ? 'Visão do Supervisor' : 'Visão do RCA'}</p>
      </div>

      <div className="space-y-3">
        <ChipRow
          label="Indústria"
          options={industrias.map(i => ({ value: String(i.id), label: i.nome }))}
          value={industriaID}
          onChange={v => { setIndustriaID(v); setVigenciaID(''); setVigenciaCombinadaKey('') }}
        />

        {industriaSelecionada && (
          <ChipRow
            label="Métrica"
            options={metricasDisponiveis.map(m => ({ value: m.value, label: m.label }))}
            value={metrica}
            onChange={v => { setMetrica(v); setVigenciaID(''); setVigenciaCombinadaKey('') }}
          />
        )}

        {industriaSelecionada && metrica === 'combinado' && (
          <ChipRow
            label="Período"
            options={periodosCombinados.map(p => ({ value: p.chave, label: `${p.cobertura.data_inicio} – ${p.cobertura.data_fim}` }))}
            value={vigenciaCombinadaKey}
            onChange={setVigenciaCombinadaKey}
          />
        )}
        {industriaSelecionada && metrica !== 'combinado' && (
          <ChipRow
            label="Período"
            options={vigencias.map(v => ({ value: String(v.id), label: `${v.data_inicio} – ${v.data_fim}` }))}
            value={vigenciaID}
            onChange={setVigenciaID}
          />
        )}
        {industriaSelecionada && (
          <ChipRow
            label="Visão"
            options={FLUXOS}
            value={fluxo}
            onChange={setFluxo}
          />
        )}
      </div>

      {(isLoading || isLoadingCombinado) && <p className="text-center text-sm text-muted-foreground py-8">Carregando...</p>}

      {metrica === 'combinado' ? (
        painelCombinado && (
          <div className="space-y-3">
            <div className="grid grid-cols-1 gap-2">
              <div className="bg-white border rounded-xl p-4">
                <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                  <Target className="w-4 h-4" /> Cobertura — redes cobertas
                </div>
                <div className="text-2xl font-bold">{fmt(painelCombinado.cobertura.realizado_total)}</div>
              </div>
              <div className="bg-white border rounded-xl p-4">
                <div className="flex items-center gap-2 text-muted-foreground text-xs mb-1">
                  <Target className="w-4 h-4" /> Sortimento — média de EANs
                </div>
                <div className="text-2xl font-bold">{fmt(painelCombinado.sortimento.realizado_total)}</div>
              </div>
            </div>

            <div className="bg-white border rounded-xl overflow-hidden">
              <div className="px-3 py-2 text-xs font-medium text-muted-foreground border-b">Suas Redes — Cobertura e Sortimento</div>
              {painelCombinado.redes.length === 0 && (
                <div className="px-3 py-4 text-sm text-muted-foreground text-center">Nenhuma Rede neste recorte</div>
              )}
              {painelCombinado.redes.map((r, i) => (
                <div key={i} className="px-3 py-2.5 border-b last:border-0 text-sm space-y-1">
                  <div className="flex items-center justify-between">
                    <span className="font-medium">{r.cod_princ}</span>
                    <span className={`text-xs px-2 py-0.5 rounded-full ${r.cobertura_atingiu ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-100 text-slate-600'}`}>
                      {r.cobertura_atingiu ? 'Coberta' : 'Não coberta'}
                    </span>
                  </div>
                  <div className="text-xs text-muted-foreground flex justify-between">
                    <span>Cobertura: {fmt(r.cobertura_valor)} / {fmt(r.cobertura_objetivo)}</span>
                    <span>Sortimento: {fmt(r.sortimento_valor)} / {fmt(r.sortimento_objetivo)}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )
      ) : painel && (
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
                <div className="text-3xl font-bold">{fmt(painel.realizado.realizado_total)}</div>
                {painel.realizado.parcial && <span className="text-xs text-amber-600">Mês em andamento</span>}
              </div>

              <div className={`rounded-xl p-4 border ${painel.delta > 0 ? 'bg-amber-50 border-amber-200' : 'bg-emerald-50 border-emerald-200'}`}>
                <div className="flex items-center gap-2 text-xs mb-1">
                  {painel.delta > 0 ? <TrendingDown className="w-4 h-4 text-amber-600" /> : <TrendingUp className="w-4 h-4 text-emerald-600" />}
                  {painel.delta > 0 ? `Falta ${fmt(painel.delta)} pra bater o objetivo` : 'Objetivo batido!'}
                </div>
                {painel.proxima_faixa && (
                  <div className="text-xs text-muted-foreground">Próximo objetivo (Faixa {painel.proxima_faixa.faixa}): {painel.proxima_faixa.valor_meta}</div>
                )}
              </div>

              <div className="bg-white border rounded-xl overflow-hidden">
                <div className="px-3 py-2 text-xs font-medium text-muted-foreground border-b">Suas Redes</div>
                {painel.realizado.redes.length === 0 && (
                  <div className="px-3 py-4 text-sm text-muted-foreground text-center">Nenhuma Rede neste recorte</div>
                )}
                {painel.realizado.redes.map((r, i) => (
                  <div key={i} className="px-3 py-2 flex items-center justify-between border-b last:border-0 text-sm">
                    <span>{r.cod_princ}</span>
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
                <div className="text-3xl font-bold">{fmt(painel.realizado.projecao)}</div>
              </div>
              {painel.recortes && (
                <div className="bg-white border rounded-xl overflow-hidden">
                  {RECORTES.map(r => (
                    <div key={r.value} className="px-3 py-2 flex items-center justify-between border-b last:border-0 text-sm">
                      <span>{r.label}</span>
                      <span className="font-semibold">{painel.recortes?.[r.value]?.realizado_total !== undefined ? fmt(painel.recortes[r.value].realizado_total) : '—'}</span>
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
