import React, { useEffect, useState } from 'react'
import { useAuth } from '@/contexts/AuthContext'
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer,
  AreaChart, Area,
} from 'recharts'
import { Progress } from '@/components/ui/progress'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'

// ─── Metas contratuais ────────────────────────────────────────────────────────
const METAS = {
  pct_calibrados:            70,   // >70% SKUs calibrados no 1º ciclo
  reducao_ofensores_ab:      60,   // 60–80% redução ofensores A/B
  pct_realocado:             70,   // 70%+ realocação de caixas ociosas
  reducao_acessos_emergencia: 50,  // 50–70% redução reposições emergenciais
  reducao_acessos_total:     15,   // 15–30% redução acessos picking total
}

// ─── Interfaces ───────────────────────────────────────────────────────────────
interface CicloKPI {
  job_id: string
  ciclo_num: number
  criado_em: string
  total_enderecos: number
  calibrados_ok: number
  pct_calibrados: number
  ofensores_falta_ab: number
  caixas_ociosas: number
  caixas_aprovadas: number
  pct_realocado: number
  acessos_emergencia: number
  acessos_total: number
}

interface SpResultadosCD {
  cd_id: number
  cd_nome: string
  filial_nome: string
  ciclos: CicloKPI[]
}

interface SpResultadosResponse {
  empresa: CicloKPI | null
  cds: SpResultadosCD[]
}

interface HistoricoKPI {
  job_id: string
  criado_em: string
  total_enderecos: number
  calibrados_ok: number
  pct_calibrados: number
  ofensores_falta: number
  ofensores_espaco: number
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

function calcReducao(ciclos: CicloKPI[], campo: keyof CicloKPI): number | null {
  if (ciclos.length < 2) return null
  const base  = ciclos[ciclos.length - 1][campo] as number
  const atual = ciclos[0][campo] as number
  if (base === 0) return null
  return ((base - atual) / base) * 100
}

function renderReducao(pct: number | null) {
  if (pct === null) return <span className="text-muted-foreground text-xs">—</span>
  const abs = Math.abs(pct).toFixed(1)
  return pct >= 0
    ? <span className="text-green-600 text-xs font-medium">▼ {abs}%</span>
    : <span className="text-red-500 text-xs font-medium">▲ {abs}%</span>
}

function fmt(n: number | undefined | null, decimals = 1): string {
  if (n == null) return '—'
  return Number(n).toFixed(decimals)
}

function fmtDate(iso: string): string {
  // "2026-04-13T..." → "13/04"
  return iso.substring(8, 10) + '/' + iso.substring(5, 7)
}

// ─── Gráfico de Barras — comparativo entre importações ────────────────────────

function ComparativoChart({
  cdID, token,
}: {
  cdID: string
  token: string
}) {
  const [pontos, setPontos] = useState<HistoricoKPI[]>([])
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (!cdID) return
    setLoading(true)
    fetch(`/api/sp/resultados/historico?cd_id=${cdID}`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then(r => r.json())
      .then(d => setPontos(d.pontos ?? []))
      .catch(() => setPontos([]))
      .finally(() => setLoading(false))
  }, [cdID, token])

  if (loading) {
    return (
      <div className="flex items-center justify-center h-48 text-sm text-muted-foreground">
        Carregando histórico...
      </div>
    )
  }

  if (pontos.length === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-sm text-muted-foreground">
        Nenhuma importação encontrada para este CD.
      </div>
    )
  }

  if (pontos.length === 1) {
    return (
      <div className="flex items-center justify-center h-48 text-sm text-muted-foreground">
        Apenas 1 importação — importe novamente para comparar a evolução.
      </div>
    )
  }

  const chartData = pontos.map((p, idx) => ({
    label: `Imp. ${idx + 1}\n${fmtDate(p.criado_em)}`,
    data: fmtDate(p.criado_em),
    calibrados: p.calibrados_ok,
    falta: p.ofensores_falta,
    espaco: p.ofensores_espaco,
  }))

  return (
    <ResponsiveContainer width="100%" height={280}>
      <BarChart data={chartData} margin={{ top: 8, right: 8, left: -8, bottom: 4 }} barCategoryGap="20%">
        <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
        <XAxis dataKey="data" tick={{ fontSize: 11 }} />
        <YAxis tick={{ fontSize: 11 }} />
        <Tooltip
          contentStyle={{ fontSize: 12 }}
          formatter={(val: number, name: string) => [
            val.toLocaleString('pt-BR'),
            name,
          ]}
        />
        <Legend wrapperStyle={{ fontSize: 12 }} />
        <Bar dataKey="calibrados" name="SKUs Calibrados"   fill="#22c55e" radius={[3, 3, 0, 0]} />
        <Bar dataKey="falta"      name="Ofensores Falta"   fill="#ef4444" radius={[3, 3, 0, 0]} />
        <Bar dataKey="espaco"     name="Ofensores Espaço"  fill="#f97316" radius={[3, 3, 0, 0]} />
      </BarChart>
    </ResponsiveContainer>
  )
}

// ─── KpiCard ─────────────────────────────────────────────────────────────────
interface KpiCardProps {
  titulo: string
  valor: string
  unidade: string
  meta: number
  metaLabel: React.ReactNode
  progresso: number
  trend?: number[]
  reducao?: number | null
  detalhe?: string
}

function KpiCard({ titulo, valor, unidade, meta, metaLabel, progresso, trend, reducao, detalhe }: KpiCardProps) {
  const sparkData = trend?.map((v, i) => ({ i, v })) ?? []
  const clampedPct = Math.min(100, Math.max(0, progresso))

  return (
    <div className="rounded-lg border bg-white p-3 flex flex-col gap-2 shadow-sm">
      <div className="flex items-start justify-between gap-2">
        <p className="text-[11px] text-muted-foreground leading-tight">{titulo}</p>
        {reducao !== undefined && <div className="shrink-0">{renderReducao(reducao)}</div>}
      </div>

      <div className="flex flex-col gap-0.5">
        <div className="flex items-end gap-1">
          <span className="text-2xl font-bold leading-none">{valor}</span>
          <span className="text-xs text-muted-foreground mb-0.5">{unidade}</span>
        </div>
        {detalhe && (
          <span className="text-[10px] text-muted-foreground">{detalhe}</span>
        )}
      </div>

      <Progress value={clampedPct} className="h-1.5" />

      <div className="flex items-center justify-between">
        <span className="text-[10px] text-muted-foreground">{metaLabel}</span>
        {sparkData.length >= 2 && (
          <ResponsiveContainer width={80} height={28}>
            <AreaChart data={sparkData} margin={{ top: 2, right: 0, left: 0, bottom: 2 }}>
              <Area
                type="monotone"
                dataKey="v"
                stroke="#6366f1"
                fill="#e0e7ff"
                strokeWidth={1.5}
                dot={false}
                isAnimationActive={false}
              />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  )
}

// ─── KPIs do ciclo atual de um CD ────────────────────────────────────────────
function CdKpis({ cd }: { cd: SpResultadosCD }) {
  const ciclos = cd.ciclos
  const atual  = ciclos[0] ?? null
  const sparkBase = [...ciclos].reverse()

  if (!atual) return null

  return (
    <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-2">
      <KpiCard
        titulo="SKUs calibrados (%)"
        valor={fmt(atual.pct_calibrados)}
        unidade="%"
        meta={METAS.pct_calibrados}
        metaLabel={`Meta: >${METAS.pct_calibrados}%`}
        progresso={(atual.pct_calibrados / METAS.pct_calibrados) * 100}
        trend={sparkBase.map(c => c.pct_calibrados)}
      />
      <KpiCard
        titulo="Ofensores falta (A/B)"
        valor={String(atual.ofensores_falta_ab)}
        unidade="SKUs"
        meta={METAS.reducao_ofensores_ab}
        metaLabel={<EntenderBadge />}
        progresso={(() => {
          const r = calcReducao(ciclos, 'ofensores_falta_ab')
          return r !== null ? Math.min(100, (r / METAS.reducao_ofensores_ab) * 100) : 0
        })()}
        trend={sparkBase.map(c => c.ofensores_falta_ab)}
        reducao={calcReducao(ciclos, 'ofensores_falta_ab')}
        detalhe={`de ${atual.total_enderecos} endereços`}
      />
      <KpiCard
        titulo="Caixas ociosas realocadas (%)"
        valor={fmt(atual.pct_realocado)}
        unidade="%"
        meta={METAS.pct_realocado}
        metaLabel={`Meta: ≥${METAS.pct_realocado}%`}
        progresso={(atual.pct_realocado / METAS.pct_realocado) * 100}
        trend={sparkBase.map(c => c.pct_realocado)}
        detalhe={atual.caixas_ociosas > 0 ? `${atual.caixas_aprovadas} de ${atual.caixas_ociosas} cx` : undefined}
      />
      <KpiCard
        titulo="Acessos emergenciais (90d)"
        valor={String(atual.acessos_emergencia)}
        unidade="acessos"
        meta={METAS.reducao_acessos_emergencia}
        metaLabel={<EntenderBadge />}
        progresso={(() => {
          const r = calcReducao(ciclos, 'acessos_emergencia')
          return r !== null ? Math.min(100, (r / METAS.reducao_acessos_emergencia) * 100) : 0
        })()}
        trend={sparkBase.map(c => c.acessos_emergencia)}
        reducao={calcReducao(ciclos, 'acessos_emergencia')}
      />
      <KpiCard
        titulo="Acessos picking total (90d)"
        valor={String(atual.acessos_total)}
        unidade="acessos"
        meta={METAS.reducao_acessos_total}
        metaLabel={<EntenderBadge />}
        progresso={(() => {
          const r = calcReducao(ciclos, 'acessos_total')
          return r !== null ? Math.min(100, (r / METAS.reducao_acessos_total) * 100) : 0
        })()}
        trend={sparkBase.map(c => c.acessos_total)}
        reducao={calcReducao(ciclos, 'acessos_total')}
      />
    </div>
  )
}

function EntenderBadge() {
  return (
    <span className="text-[10px] font-semibold text-indigo-600 bg-indigo-50 px-1.5 py-0.5 rounded cursor-pointer hover:bg-indigo-100 transition-colors">
      Entender melhor
    </span>
  )
}

// ─── Página principal ─────────────────────────────────────────────────────────
export default function SpResultados() {
  const { token } = useAuth()
  const [data,    setData]    = useState<SpResultadosResponse | null>(null)
  const [cdID,    setCdID]    = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [error,   setError]   = useState<string | null>(null)

  // Carrega KPI cards
  useEffect(() => {
    if (!token) return
    setLoading(true)
    setError(null)
    const url = cdID ? `/api/sp/resultados?cd_id=${cdID}` : '/api/sp/resultados'
    fetch(url, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => {
        if (!r.ok) throw new Error(`Erro ${r.status}`)
        return r.json() as Promise<SpResultadosResponse>
      })
      .then(d => setData(d))
      .catch(e => setError(String(e)))
      .finally(() => setLoading(false))
  }, [token, cdID])

  const cds    = data?.cds ?? []
  const cdOpts = cds

  // Auto-seleciona CD quando há apenas um
  useEffect(() => {
    if (!cdID && cds.length === 1) {
      setCdID(String(cds[0].cd_id))
    }
  }, [cds]) // eslint-disable-line react-hooks/exhaustive-deps

  const selectedCd = cdID ? cds.find(c => String(c.cd_id) === cdID) : null

  return (
    <div className="flex flex-col gap-6">

      {/* ── Seletor de CD ──────────────────────────────────────────────── */}
      <div className="flex items-center gap-3 flex-wrap">
        <Select value={cdID || 'all'} onValueChange={v => setCdID(v === 'all' ? '' : v)}>
          <SelectTrigger className="w-64 text-xs">
            <SelectValue placeholder="Selecione um CD" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Todos os CDs</SelectItem>
            {cdOpts.map(cd => (
              <SelectItem key={cd.cd_id} value={String(cd.cd_id)}>
                {cd.filial_nome} — {cd.cd_nome}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {selectedCd && (
          <span className="text-xs text-muted-foreground">
            {selectedCd.ciclos.length} ciclo{selectedCd.ciclos.length !== 1 ? 's' : ''} disponível{selectedCd.ciclos.length !== 1 ? 'is' : ''}
          </span>
        )}
      </div>

      {/* ── Loading / erro ─────────────────────────────────────────────── */}
      {loading && <p className="text-sm text-muted-foreground">Carregando...</p>}
      {error   && <p className="text-sm text-red-500">{error}</p>}

      {/* ── Sem dados ──────────────────────────────────────────────────── */}
      {!loading && !error && cds.length === 0 && (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">
            Nenhum dado disponível. Importe e processe um CSV para ver os resultados.
          </p>
        </div>
      )}

      {/* ── Gráfico comparativo entre importações (elemento principal) ─── */}
      {!loading && cdID && token && (
        <section>
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-3">
            Evolução por Importação
          </h2>
          <div className="rounded-lg border bg-white p-4 shadow-sm">
            <div className="flex gap-4 mb-3 flex-wrap">
              <span className="flex items-center gap-1.5 text-xs">
                <span className="w-3 h-3 rounded-sm inline-block" style={{ background: '#22c55e' }} />
                SKUs Calibrados
              </span>
              <span className="flex items-center gap-1.5 text-xs">
                <span className="w-3 h-3 rounded-sm inline-block" style={{ background: '#ef4444' }} />
                Ofensores Falta (A/B)
              </span>
              <span className="flex items-center gap-1.5 text-xs">
                <span className="w-3 h-3 rounded-sm inline-block" style={{ background: '#f97316' }} />
                Ofensores Espaço
              </span>
            </div>
            <ComparativoChart cdID={cdID} token={token} />
          </div>
        </section>
      )}

      {/* Mensagem para selecionar CD quando há múltiplos */}
      {!loading && !cdID && cds.length > 1 && (
        <div className="rounded-lg border border-dashed p-6 text-center">
          <p className="text-sm text-muted-foreground">
            Selecione um CD acima para ver o gráfico comparativo entre importações.
          </p>
        </div>
      )}

      {/* ── KPIs do ciclo atual (ciclo mais recente do CD selecionado) ─── */}
      {!loading && selectedCd && (
        <section>
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-3">
            KPIs Contratuais — Ciclo Atual
          </h2>
          <CdKpis cd={selectedCd} />
        </section>
      )}

      {/* ── Visão Empresa Consolidada (apenas sem filtro de CD) ─────────── */}
      {!loading && !cdID && data?.empresa && (
        <section>
          <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-3">
            Empresa Consolidada — Ciclo Atual
          </h2>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-2">
            <KpiCard
              titulo="SKUs calibrados (%)"
              valor={fmt(data.empresa.pct_calibrados)}
              unidade="%"
              meta={METAS.pct_calibrados}
              metaLabel={`Meta: >${METAS.pct_calibrados}%`}
              progresso={(data.empresa.pct_calibrados / METAS.pct_calibrados) * 100}
            />
            <KpiCard
              titulo="Ofensores falta (A/B)"
              valor={String(data.empresa.ofensores_falta_ab)}
              unidade="SKUs"
              meta={METAS.reducao_ofensores_ab}
              metaLabel={<EntenderBadge />}
              progresso={0}
              detalhe={`de ${data.empresa.total_enderecos} endereços`}
            />
            <KpiCard
              titulo="Caixas ociosas realocadas (%)"
              valor={fmt(data.empresa.pct_realocado)}
              unidade="%"
              meta={METAS.pct_realocado}
              metaLabel={`Meta: ≥${METAS.pct_realocado}%`}
              progresso={(data.empresa.pct_realocado / METAS.pct_realocado) * 100}
              detalhe={data.empresa.caixas_ociosas > 0 ? `${data.empresa.caixas_aprovadas} de ${data.empresa.caixas_ociosas} cx` : undefined}
            />
            <KpiCard
              titulo="Acessos emergenciais (90d)"
              valor={String(data.empresa.acessos_emergencia)}
              unidade="acessos"
              meta={METAS.reducao_acessos_emergencia}
              metaLabel={<EntenderBadge />}
              progresso={0}
            />
            <KpiCard
              titulo="Acessos picking total (90d)"
              valor={String(data.empresa.acessos_total)}
              unidade="acessos"
              meta={METAS.reducao_acessos_total}
              metaLabel={<EntenderBadge />}
              progresso={0}
            />
          </div>
        </section>
      )}

    </div>
  )
}
