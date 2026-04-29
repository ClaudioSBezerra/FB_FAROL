import { useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from 'sonner'
import { Plus, Copy, Settings2, Trash2, ChevronDown, ChevronRight, AlertTriangle, HelpCircle } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface SpFilial {
  id: number
  cod_filial: number
  nome: string
  ativo: boolean
  num_cds: number
  created_at: string
}

interface SpCD {
  id: number
  filial_id: number
  nome: string
  descricao: string
  ativo: boolean
  fonte_cd_id?: number
  created_at: string
}

interface SpMotorParams {
  id: number
  cd_id: number
  dias_analise: number
  curva_a_max_est: number
  curva_b_max_est: number
  curva_c_max_est: number
  fator_seguranca: number
  curva_a_nunca_reduz: boolean
  min_capacidade: number
  retencao_csv_meses: number
  updated_at: string
}

interface SpPlano {
  plano: string
  max_filiais: number
  max_cds: number
  max_usuarios: number
  ativo: boolean
  valido_ate: string | null
  usado_filiais: number
  usado_cds: number
  usado_usuarios: number
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function UsageBar({ used, max }: { used: number; max: number }) {
  if (max === -1) return <span className="text-xs text-muted-foreground">{used} / ∞</span>
  const pct = Math.min(100, Math.round((used / max) * 100))
  const color = pct >= 90 ? 'bg-red-500' : pct >= 70 ? 'bg-yellow-500' : 'bg-green-500'
  return (
    <div className="flex items-center gap-2">
      <div className="w-24 h-1.5 rounded-full bg-gray-200">
        <div className={`h-full rounded-full ${color}`} style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs text-muted-foreground">{used}/{max}</span>
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SpAmbiente() {
  const { token, spRole } = useAuth()
  const qc = useQueryClient()
  const location = useLocation()
  const path = location.pathname
  const isFiliais   = path === '/gestao/filiais'
  const isRegras    = path === '/gestao/regras' || path === '/config/parametros-motor'
  const isPlanos    = path === '/config/planos'
  const isManutencao = path === '/config/manutencao'

  // ── State ────────────────────────────────────────────────────────────────────
  const [expandedFilial, setExpandedFilial] = useState<number | null>(null)
  const [paramsCD,       setParamsCD]       = useState<number | null>(null)

  const [filialDialog,   setFilialDialog]   = useState(false)
  const [cdDialog,       setCdDialog]       = useState<{ filialID: number } | null>(null)
  const [dupDialog,      setDupDialog]      = useState<SpCD | null>(null)
  const [paramsDialog,   setParamsDialog]   = useState<SpMotorParams | null>(null)

  const [newFilialCod,   setNewFilialCod]   = useState('')
  const [newFilialNome,  setNewFilialNome]  = useState('')
  const [newCDNome,      setNewCDNome]      = useState('')
  const [newCDDesc,      setNewCDDesc]      = useState('')
  const [dupNome,        setDupNome]        = useState('')
  const [editParams,     setEditParams]     = useState<Partial<SpMotorParams>>({})
  const [limparDialog,        setLimparDialog]        = useState(false)
  const [limparConfirm,       setLimparConfirm]        = useState('')
  const [limparCadDialog,     setLimparCadDialog]      = useState(false)
  const [limparCadConfirm,    setLimparCadConfirm]     = useState('')

  // ── Simulador da fórmula ─────────────────────────────────────────────────────
  const [showHelp,        setShowHelp]        = useState(true)
  const [simGiro,         setSimGiro]         = useState(25)
  const [simMaster,       setSimMaster]       = useState(6)
  const [simCurva,        setSimCurva]        = useState<'A' | 'B' | 'C'>('B')
  const [simDias,         setSimDias]         = useState(15)
  const [simFator,        setSimFator]        = useState(1.10)
  const [simCapAtual,     setSimCapAtual]     = useState(60)
  const [simMinCap,       setSimMinCap]       = useState(1)
  const [simNuncaReduz,   setSimNuncaReduz]   = useState(true)
  const [simNormaPalete,  setSimNormaPalete]  = useState(0)

  // resultados do simulador (derivados — sem useMemo para manter simples)
  const simCaixasGiro   = Math.ceil(simGiro / Math.max(simMaster, 1))
  const simSugestaoRaw  = Math.ceil(simCaixasGiro * simDias * simFator)
  const simSugestaoMin  = Math.max(simSugestaoRaw, simMinCap)
  const simNormaAplicada = simNormaPalete > 1 && simSugestaoMin % simNormaPalete !== 0
  const simSugestaoNorma = simNormaAplicada
    ? (Math.floor(simSugestaoMin / simNormaPalete) + 1) * simNormaPalete
    : simSugestaoMin
  const simCurvaALifted  = simCurva === 'A' && simNuncaReduz && simSugestaoNorma < simCapAtual
  const simSugestaoFinal = simCurvaALifted ? simCapAtual : simSugestaoNorma
  const simDelta         = simSugestaoFinal - simCapAtual
  const simCalibrado     = simCapAtual > 0 && Math.abs(simDelta) / simCapAtual <= 0.05

  // ── Queries ──────────────────────────────────────────────────────────────────
  const headers = { Authorization: `Bearer ${token}` }

  const { data: filiais = [], isLoading: loadingFiliais } = useQuery<SpFilial[]>({
    queryKey: ['sp-filiais'],
    queryFn: async () => {
      const r = await fetch('/api/sp/filiais', { headers })
      if (!r.ok) throw new Error('Erro ao carregar filiais')
      return r.json()
    },
  })

  const { data: cds = [] } = useQuery<SpCD[]>({
    queryKey: ['sp-cds', expandedFilial],
    enabled: expandedFilial !== null,
    queryFn: async () => {
      const r = await fetch(`/api/sp/filiais/${expandedFilial}/cds`, { headers })
      if (!r.ok) throw new Error('Erro ao carregar CDs')
      return r.json()
    },
  })

  const { data: plano, isLoading: loadingPlano, isError: erroPlano } = useQuery<SpPlano>({
    queryKey: ['sp-plano'],
    queryFn: async () => {
      const r = await fetch('/api/sp/plano', { headers })
      if (!r.ok) throw new Error('Erro ao carregar plano')
      return r.json()
    },
  })

  type CdRow = SpCD & { filial_nome: string; cod_filial: number; params?: SpMotorParams }

  // Todos os CDs da empresa + params do motor (usado na view de Regras de Calibragem)
  const { data: todosOsCds = [] } = useQuery<CdRow[]>({
    queryKey: ['sp-todos-cds'],
    enabled: isRegras,
    queryFn: async () => {
      const filiaisR = await fetch('/api/sp/filiais', { headers })
      if (!filiaisR.ok) throw new Error()
      const filiaisData: SpFilial[] = await filiaisR.json()
      const results: CdRow[] = []
      await Promise.all(filiaisData.map(async f => {
        const r = await fetch(`/api/sp/filiais/${f.id}/cds`, { headers })
        if (!r.ok) return
        const cdsData: SpCD[] = await r.json()
        await Promise.all(cdsData.map(async cd => {
          let params: SpMotorParams | undefined
          try {
            const pr = await fetch(`/api/sp/cds/${cd.id}/params`, { headers })
            if (pr.ok) params = await pr.json()
          } catch { /* sem params ainda */ }
          results.push({ ...cd, filial_nome: f.nome, cod_filial: f.cod_filial, params })
        }))
      }))
      return results.sort((a, b) => a.filial_nome.localeCompare(b.filial_nome) || a.nome.localeCompare(b.nome))
    },
  })

  // ── Mutations ────────────────────────────────────────────────────────────────
  const criarFilial = useMutation({
    mutationFn: async () => {
      const r = await fetch('/api/sp/filiais', {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ cod_filial: parseInt(newFilialCod), nome: newFilialNome }),
      })
      if (!r.ok) throw new Error((await r.json()).error ?? 'Erro ao criar filial')
    },
    onSuccess: () => {
      toast.success('Filial criada')
      qc.invalidateQueries({ queryKey: ['sp-filiais'] })
      qc.invalidateQueries({ queryKey: ['sp-plano'] })
      setFilialDialog(false); setNewFilialCod(''); setNewFilialNome('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const criarCD = useMutation({
    mutationFn: async () => {
      const r = await fetch(`/api/sp/filiais/${cdDialog!.filialID}/cds`, {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ nome: newCDNome, descricao: newCDDesc }),
      })
      if (!r.ok) throw new Error((await r.json()).error ?? 'Erro ao criar CD')
    },
    onSuccess: () => {
      toast.success('CD criado')
      qc.invalidateQueries({ queryKey: ['sp-cds', cdDialog!.filialID] })
      qc.invalidateQueries({ queryKey: ['sp-plano'] })
      setCdDialog(null); setNewCDNome(''); setNewCDDesc('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const duplicarCD = useMutation({
    mutationFn: async () => {
      const r = await fetch(`/api/sp/cds/${dupDialog!.id}/duplicar`, {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ nome: dupNome }),
      })
      if (!r.ok) throw new Error((await r.json()).error ?? 'Erro ao duplicar CD')
    },
    onSuccess: () => {
      toast.success('CD duplicado')
      qc.invalidateQueries({ queryKey: ['sp-cds', dupDialog!.filial_id] })
      qc.invalidateQueries({ queryKey: ['sp-plano'] })
      setDupDialog(null); setDupNome('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const salvarParams = useMutation({
    mutationFn: async () => {
      const r = await fetch(`/api/sp/cds/${paramsDialog!.cd_id}/params`, {
        method: 'PUT',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify(editParams),
      })
      if (!r.ok) throw new Error((await r.json()).error ?? 'Erro ao salvar parâmetros')
    },
    onSuccess: () => {
      toast.success('Parâmetros atualizados')
      qc.invalidateQueries({ queryKey: ['sp-params', paramsCD] })
      qc.invalidateQueries({ queryKey: ['sp-todos-cds'] })
      setParamsDialog(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const desativarFilial = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/sp/filiais/${id}`, {
        method: 'DELETE', headers,
      })
      if (!r.ok) throw new Error('Erro ao desativar filial')
    },
    onSuccess: () => {
      toast.success('Filial desativada')
      qc.invalidateQueries({ queryKey: ['sp-filiais'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const limparCalibragem = useMutation({
    mutationFn: async () => {
      const r = await fetch('/api/sp/admin/limpar-calibragem', {
        method: 'DELETE', headers,
      })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    onSuccess: (data: Record<string, unknown>) => {
      toast.success(`Limpeza concluída: ${data.sp_propostas ?? 0} propostas, ${data.sp_enderecos ?? 0} endereços, ${data.sp_csv_jobs ?? 0} jobs removidos`)
      qc.invalidateQueries({ queryKey: ['sp-plano'] })
      qc.invalidateQueries({ queryKey: ['sp-propostas'] })
      qc.invalidateQueries({ queryKey: ['sp-propostas-resumo'] })
      qc.invalidateQueries({ queryKey: ['sp-csv-jobs'] })
      qc.invalidateQueries({ queryKey: ['sp-historico'] })
      setLimparDialog(false)
      setLimparConfirm('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const limparCadastros = useMutation({
    mutationFn: async () => {
      const r = await fetch('/api/cadastros/limpar', { method: 'DELETE', headers })
      if (!r.ok) throw new Error(await r.text())
      return r.json() as Promise<{ gestores_removidos: number; rcas_removidos: number; vinculos_removidos: number }>
    },
    onSuccess: (data) => {
      toast.success(`Cadastros limpos: ${data.gestores_removidos} gestores, ${data.rcas_removidos} RCAs, ${data.vinculos_removidos} vínculos removidos`)
      qc.invalidateQueries({ queryKey: ['cadastros-gestores'] })
      qc.invalidateQueries({ queryKey: ['cadastros-rcas'] })
      setLimparCadDialog(false)
      setLimparCadConfirm('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  function openParams(cd: SpCD) {
    setParamsCD(cd.id)
    fetch(`/api/sp/cds/${cd.id}/params`, { headers })
      .then(r => r.json())
      .then((p: SpMotorParams) => { setParamsDialog(p); setEditParams(p) })
      .catch(() => toast.error('Erro ao carregar parâmetros'))
  }

  // ── Render: Regras de Calibragem ─────────────────────────────────────────────
  if (isRegras) {
    return (
      <div className="space-y-4">
        <div>
          <h2 className="text-sm font-semibold">Regras de Calibragem</h2>
          <p className="text-xs text-muted-foreground mt-1">
            Defina por CD como o algoritmo calcula a sugestão de capacidade:
            quantos dias de venda analisar, limite de estoque por curva (A/B/C),
            fator de segurança e regras especiais.
          </p>
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Filial</TableHead>
              <TableHead>CD</TableHead>
              <TableHead className="text-right">Dias Análise</TableHead>
              <TableHead className="text-right">A (dias)</TableHead>
              <TableHead className="text-right">B (dias)</TableHead>
              <TableHead className="text-right">C (dias)</TableHead>
              <TableHead className="text-right">Fat. Seg.</TableHead>
              <TableHead className="text-right">Cap. Mín.</TableHead>
              <TableHead className="text-right">Retenção</TableHead>
              <TableHead className="w-24"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {todosOsCds.length === 0 && (
              <TableRow>
                <TableCell colSpan={10} className="text-center text-sm text-muted-foreground py-8">
                  Nenhum CD cadastrado.
                </TableCell>
              </TableRow>
            )}
            {todosOsCds.map(cd => (
              <TableRow key={cd.id}>
                <TableCell className="text-xs text-muted-foreground">{cd.filial_nome}</TableCell>
                <TableCell className="text-sm font-medium">{cd.nome}</TableCell>
                <TableCell className="text-xs text-right">{cd.params?.dias_analise ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell className="text-xs text-right">{cd.params?.curva_a_max_est ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell className="text-xs text-right">{cd.params?.curva_b_max_est ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell className="text-xs text-right">{cd.params?.curva_c_max_est ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell className="text-xs text-right">{cd.params ? cd.params.fator_seguranca.toFixed(2) : <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell className="text-xs text-right">{cd.params?.min_capacidade ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell className="text-xs text-right">{cd.params?.retencao_csv_meses ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell>
                  <Button size="sm" variant="outline" className="h-7 text-xs"
                    onClick={() => openParams(cd)}>
                    <Settings2 className="h-3.5 w-3.5 mr-1" /> Editar
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>

        {/* ── Painel de Ajuda: Fórmula + Simulador ───────────────────────────── */}
        <div className="border rounded-lg overflow-hidden">
          <button
            className="w-full flex items-center justify-between px-4 py-3 bg-blue-50 hover:bg-blue-100 transition-colors text-left"
            onClick={() => setShowHelp(h => !h)}
          >
            <div className="flex items-center gap-2">
              <HelpCircle className="h-4 w-4 text-blue-600" />
              <span className="text-sm font-medium text-blue-900">Como funciona a fórmula de calibragem</span>
            </div>
            {showHelp
              ? <ChevronDown className="h-4 w-4 text-blue-600" />
              : <ChevronRight className="h-4 w-4 text-blue-600" />}
          </button>

          {showHelp && (
            <div className="p-4 space-y-6 bg-white">

              {/* ── Fórmula ────────────────────────────────────────────────────── */}
              <div className="space-y-3">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Fórmula</p>
                <div className="bg-slate-900 text-green-400 rounded-md px-4 py-3 font-mono text-sm text-center">
                  Sugestão = ⌈ ⌈ Giro_dia ÷ Unid/cx ⌉ × Dias_curva × Fator_seg ⌉ → múltiplo Norma_Palete
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <div className="flex items-start gap-2 text-xs">
                    <span className="font-mono bg-amber-100 text-amber-800 px-1.5 py-0.5 rounded shrink-0">Giro_dia</span>
                    <span className="text-muted-foreground">
                      Acessos diários ao picking — <strong>primário:</strong> QTACESSO_PICKING_PERIODO_90 ÷ QT_DIAS
                      (Curva ABC de Acesso gerada pelo WMS/JC)
                    </span>
                  </div>
                  <div className="flex items-start gap-2 text-xs">
                    <span className="font-mono bg-blue-100 text-blue-800 px-1.5 py-0.5 rounded shrink-0">Unid/cx</span>
                    <span className="text-muted-foreground">Unidades por caixa (QTUNITCX do CSV)</span>
                  </div>
                  <div className="flex items-start gap-2 text-xs">
                    <span className="font-mono bg-green-100 text-green-800 px-1.5 py-0.5 rounded shrink-0">Dias_curva</span>
                    <span className="text-muted-foreground">Dias máx. de estoque: vem de CLASSEVENDA_DIAS do CSV; fallback = parâmetros A/B/C abaixo</span>
                  </div>
                  <div className="flex items-start gap-2 text-xs">
                    <span className="font-mono bg-purple-100 text-purple-800 px-1.5 py-0.5 rounded shrink-0">Fator_seg</span>
                    <span className="text-muted-foreground">Margem de segurança configurada neste CD (ex: 1.10 = +10% sobre a média)</span>
                  </div>
                  <div className="flex items-start gap-2 text-xs">
                    <span className="font-mono bg-rose-100 text-rose-800 px-1.5 py-0.5 rounded shrink-0">Norma_Palete</span>
                    <span className="text-muted-foreground">
                      Caixas por palete (NORMA_PALETE do CSV). Quando &gt; 1, a sugestão é arredondada
                      para cima ao múltiplo mais próximo: ⌈n ÷ np⌉ × np
                    </span>
                  </div>
                </div>
              </div>

              {/* ── Prioridade das fontes ───────────────────────────────────────── */}
              <div className="space-y-2">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Prioridade das fontes de dados (CSV)</p>
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-1">
                    <p className="text-xs font-medium">Giro diário (unid/dia)</p>
                    <ol className="text-xs text-muted-foreground space-y-0.5 list-decimal list-inside">
                      <li>QTACESSO_PICKING_PERIODO_90 ÷ QT_DIAS <span className="text-green-600 font-medium">(preferido — acesso ao picking)</span></li>
                      <li>MED_VENDA_DIAS</li>
                      <li>MED_VENDA_DIAS_CX × Unid/cx</li>
                      <li>MED_VENDA_CX_ANOANT × Unid/cx</li>
                    </ol>
                  </div>
                  <div className="space-y-1">
                    <p className="text-xs font-medium">Curva ABC</p>
                    <p className="text-xs text-muted-foreground">
                      Campo <span className="font-mono">CLASSEVENDA</span> preenchido pelo WMS/JC com base nos acessos
                      ao picking (substitui a curva de venda).
                      Exibida na coluna <em>Curva</em> do painel como <strong>Curva ABC de Acesso ao Picking</strong>.
                    </p>
                    <p className="text-xs font-medium mt-2">Dias da curva</p>
                    <ol className="text-xs text-muted-foreground space-y-0.5 list-decimal list-inside">
                      <li>CLASSEVENDA_DIAS do CSV <span className="text-green-600 font-medium">(WMS decide)</span></li>
                      <li>Parâmetros A/B/C configurados acima</li>
                    </ol>
                  </div>
                </div>
              </div>

              {/* ── Regras especiais ────────────────────────────────────────────── */}
              <div className="space-y-2">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Regras especiais</p>
                <div className="grid grid-cols-2 gap-3">
                  <div className="bg-blue-50 border border-blue-100 rounded-md p-3 space-y-1">
                    <p className="text-xs font-semibold text-blue-800">Calibrado (≥ 95% assertividade)</p>
                    <p className="text-xs text-blue-700">
                      Quando a sugestão calculada difere em até 5% da capacidade atual do slot,
                      a proposta é marcada como <strong>Calibrado</strong> — não exige aprovação
                      e sinaliza que o slot já está bem dimensionado.
                    </p>
                    <p className="text-xs font-mono text-blue-600">|sugestão − cap_atual| ÷ cap_atual ≤ 0.05</p>
                  </div>
                  <div className="bg-slate-50 border border-slate-200 rounded-md p-3 space-y-1">
                    <p className="text-xs font-semibold text-slate-700">Produtos Ignorados</p>
                    <p className="text-xs text-muted-foreground">
                      Produtos cadastrados na lista de <strong>Produtos Ignorados</strong> são pulados pelo motor
                      a cada nova calibragem — nenhuma proposta é gerada para eles.
                      O gestor pode reativá-los a qualquer momento na aba <em>Produtos Ignorados</em>.
                    </p>
                  </div>
                  <div className="bg-amber-50 border border-amber-100 rounded-md p-3 space-y-1">
                    <p className="text-xs font-semibold text-amber-800">Curva A — nunca reduz</p>
                    <p className="text-xs text-amber-700">
                      Quando ativado, produtos de Curva A não têm capacidade reduzida pelo motor:
                      se a sugestão for menor que a capacidade atual, mantém-se a capacidade atual.
                    </p>
                  </div>
                  <div className="bg-rose-50 border border-rose-100 rounded-md p-3 space-y-1">
                    <p className="text-xs font-semibold text-rose-800">Norma Palete</p>
                    <p className="text-xs text-rose-700">
                      Garante que a sugestão final seja múltiplo exato do tamanho do palete,
                      facilitando a reposição sem fracionamento. Aplicado após a fórmula base
                      e o mínimo absoluto.
                    </p>
                    <p className="text-xs font-mono text-rose-600">sugestão = (⌊s ÷ np⌋ + 1) × np  (quando s não é múltiplo)</p>
                  </div>
                </div>
              </div>

              {/* ── Simulador ──────────────────────────────────────────────────── */}
              <div className="space-y-3">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Simulador interativo</p>

                <div className="grid grid-cols-4 gap-3">
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Giro/dia (unid.)</label>
                    <input type="number" value={simGiro} min={0}
                      onChange={e => setSimGiro(+e.target.value)}
                      className="w-full h-8 border rounded px-2 text-sm" />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Unid. por cx</label>
                    <input type="number" value={simMaster} min={1}
                      onChange={e => setSimMaster(+e.target.value)}
                      className="w-full h-8 border rounded px-2 text-sm" />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Curva</label>
                    <select value={simCurva}
                      onChange={e => setSimCurva(e.target.value as 'A' | 'B' | 'C')}
                      className="w-full h-8 border rounded px-2 text-sm bg-white">
                      <option value="A">A — alto giro</option>
                      <option value="B">B — médio</option>
                      <option value="C">C — baixo giro</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Dias da curva</label>
                    <input type="number" value={simDias} min={1}
                      onChange={e => setSimDias(+e.target.value)}
                      className="w-full h-8 border rounded px-2 text-sm" />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Fator segurança</label>
                    <input type="number" step="0.01" value={simFator} min={1}
                      onChange={e => setSimFator(+e.target.value)}
                      className="w-full h-8 border rounded px-2 text-sm" />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Cap. atual (cx)</label>
                    <input type="number" value={simCapAtual} min={0}
                      onChange={e => setSimCapAtual(+e.target.value)}
                      className="w-full h-8 border rounded px-2 text-sm" />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Cap. mínima (cx)</label>
                    <input type="number" value={simMinCap} min={1}
                      onChange={e => setSimMinCap(+e.target.value)}
                      className="w-full h-8 border rounded px-2 text-sm" />
                  </div>
                  <div className="space-y-1">
                    <label className="text-xs text-muted-foreground">Norma Palete (cx/pal.)</label>
                    <input type="number" value={simNormaPalete} min={0}
                      onChange={e => setSimNormaPalete(+e.target.value)}
                      className="w-full h-8 border rounded px-2 text-sm"
                      placeholder="0 = não aplica" />
                  </div>
                  <div className="flex items-end pb-1.5">
                    <label className="flex items-center gap-1.5 text-xs text-muted-foreground cursor-pointer">
                      <input type="checkbox" checked={simNuncaReduz}
                        onChange={e => setSimNuncaReduz(e.target.checked)} />
                      Curva A nunca reduz
                    </label>
                  </div>
                </div>

                {/* Resultado passo a passo */}
                {(() => {
                  let step = 1
                  return (
                    <div className="bg-slate-50 border rounded-md p-3 font-mono text-xs space-y-1.5">
                      <div className="flex gap-2">
                        <span className="text-slate-400 w-5 shrink-0">{step++}.</span>
                        <span>
                          caixas_giro = ⌈{simGiro} ÷ {simMaster}⌉ ={' '}
                          <strong className="text-amber-700">{simCaixasGiro} cx/dia</strong>
                        </span>
                      </div>
                      <div className="flex gap-2">
                        <span className="text-slate-400 w-5 shrink-0">{step++}.</span>
                        <span>
                          dias_curva = <strong className="text-green-700">{simDias} dias</strong>{' '}
                          <span className="text-slate-400">(Curva {simCurva})</span>
                        </span>
                      </div>
                      <div className="flex gap-2">
                        <span className="text-slate-400 w-5 shrink-0">{step++}.</span>
                        <span>
                          sugestão = ⌈{simCaixasGiro} × {simDias} × {simFator.toFixed(2)}⌉ ={' '}
                          ⌈{(simCaixasGiro * simDias * simFator).toFixed(2)}⌉ ={' '}
                          <strong className="text-blue-700">{simSugestaoRaw} cx</strong>
                        </span>
                      </div>
                      {simSugestaoMin > simSugestaoRaw && (
                        <div className="flex gap-2 text-amber-700">
                          <span className="text-slate-400 w-5 shrink-0">{step++}.</span>
                          <span>
                            cap. mínima aplicada: {simSugestaoRaw} →{' '}
                            <strong>{simSugestaoMin} cx</strong>
                          </span>
                        </div>
                      )}
                      {simNormaAplicada && (
                        <div className="flex gap-2 text-rose-700">
                          <span className="text-slate-400 w-5 shrink-0">{step++}.</span>
                          <span>
                            norma palete (×{simNormaPalete}): {simSugestaoMin} →{' '}
                            <strong>{simSugestaoNorma} cx</strong>
                            <span className="text-slate-400 ml-1">(⌊{simSugestaoMin}÷{simNormaPalete}⌋+1)×{simNormaPalete}</span>
                          </span>
                        </div>
                      )}
                      {simCurvaALifted && (
                        <div className="flex gap-2 text-amber-700">
                          <span className="text-slate-400 w-5 shrink-0">{step++}.</span>
                          <span>
                            Curva A nunca reduz: {simSugestaoNorma} →{' '}
                            <strong>{simSugestaoFinal} cx</strong>
                          </span>
                        </div>
                      )}
                      <div className="border-t pt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
                        <span className="text-slate-500">Sugestão final:</span>
                        <strong className="text-base">{simSugestaoFinal} cx</strong>
                        <span className="text-slate-400">|</span>
                        <span className="text-slate-500">Cap. atual:</span>
                        <strong>{simCapAtual} cx</strong>
                        <span className="text-slate-400">|</span>
                        <span className="text-slate-500">Delta:</span>
                        <strong className={
                          simDelta > 0 ? 'text-red-600'
                          : simDelta < 0 ? 'text-amber-600'
                          : 'text-green-600'
                        }>
                          {simDelta > 0 ? '+' : ''}{simDelta} cx
                          {' — '}
                          {simDelta > 0 ? 'FALTA (ampliar slot)' : simDelta < 0 ? 'Excesso (reduzir slot)' : 'OK'}
                        </strong>
                        {simCalibrado && (
                          <span className="ml-2 px-2 py-0.5 bg-blue-100 text-blue-800 rounded text-xs font-medium">
                            Calibrado (≤5%)
                          </span>
                        )}
                      </div>
                    </div>
                  )
                })()}
              </div>

            </div>
          )}
        </div>

        {/* Reutiliza o dialog de parâmetros existente */}
        <Dialog open={!!paramsDialog} onOpenChange={() => setParamsDialog(null)}>
          <DialogContent className="max-w-md">
            <DialogHeader><DialogTitle>Regras de Calibragem — {todosOsCds.find(c => c.id === paramsDialog?.cd_id)?.nome}</DialogTitle></DialogHeader>
            {paramsDialog && (
              <div className="grid grid-cols-2 gap-3 py-2">
                <div className="grid gap-1.5">
                  <Label>Dias de Análise</Label>
                  <Input type="number" value={editParams.dias_analise ?? 90}
                    onChange={e => setEditParams(p => ({ ...p, dias_analise: +e.target.value }))} />
                  <p className="text-xs text-muted-foreground">Janela de vendas para calcular o giro médio</p>
                </div>
                <div className="grid gap-1.5">
                  <Label>Fator de Segurança</Label>
                  <Input type="number" step="0.01" value={editParams.fator_seguranca ?? 1.10}
                    onChange={e => setEditParams(p => ({ ...p, fator_seguranca: +e.target.value }))} />
                  <p className="text-xs text-muted-foreground">Ex: 1.10 = +10% sobre a média de vendas</p>
                </div>
                <div className="grid gap-1.5">
                  <Label>Curva A — máx. dias estoque</Label>
                  <Input type="number" value={editParams.curva_a_max_est ?? 7}
                    onChange={e => setEditParams(p => ({ ...p, curva_a_max_est: +e.target.value }))} />
                </div>
                <div className="grid gap-1.5">
                  <Label>Curva B — máx. dias estoque</Label>
                  <Input type="number" value={editParams.curva_b_max_est ?? 15}
                    onChange={e => setEditParams(p => ({ ...p, curva_b_max_est: +e.target.value }))} />
                </div>
                <div className="grid gap-1.5">
                  <Label>Curva C — máx. dias estoque</Label>
                  <Input type="number" value={editParams.curva_c_max_est ?? 30}
                    onChange={e => setEditParams(p => ({ ...p, curva_c_max_est: +e.target.value }))} />
                </div>
                <div className="grid gap-1.5">
                  <Label>Capacidade mínima absoluta</Label>
                  <Input type="number" value={editParams.min_capacidade ?? 1}
                    onChange={e => setEditParams(p => ({ ...p, min_capacidade: +e.target.value }))} />
                </div>
                <div className="col-span-2 flex items-center gap-2 pt-1">
                  <input type="checkbox" id="curva-a-nunca-r"
                    checked={editParams.curva_a_nunca_reduz ?? true}
                    onChange={e => setEditParams(p => ({ ...p, curva_a_nunca_reduz: e.target.checked }))} />
                  <Label htmlFor="curva-a-nunca-r" className="cursor-pointer">
                    Curva A: nunca reduzir capacidade
                  </Label>
                </div>
                <div className="col-span-2 border-t pt-3 mt-1">
                  <p className="text-xs font-semibold text-muted-foreground mb-2 uppercase tracking-wide">Retenção de Dados</p>
                </div>
                <div className="grid gap-1.5 col-span-2">
                  <Label>Retenção de importações (meses)</Label>
                  <Input type="number" min={1} max={60}
                    value={editParams.retencao_csv_meses ?? 6}
                    onChange={e => setEditParams(p => ({ ...p, retencao_csv_meses: +e.target.value }))} />
                  <p className="text-xs text-muted-foreground">
                    Dados brutos de CSV são removidos após este período. Propostas e histórico são preservados.
                  </p>
                </div>
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={() => setParamsDialog(null)}>Cancelar</Button>
              <Button disabled={salvarParams.isPending} onClick={() => salvarParams.mutate()}>
                {salvarParams.isPending ? 'Salvando...' : 'Salvar'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    )
  }

  // ── Shared: dialogs (filiais, CDs, params) ──────────────────────────────────
  const sharedDialogs = (
    <>
      <Dialog open={!!paramsDialog} onOpenChange={() => setParamsDialog(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Regras de Calibragem — {todosOsCds.find(c => c.id === paramsDialog?.cd_id)?.nome ?? 'CD'}</DialogTitle>
          </DialogHeader>
          {paramsDialog && (
            <div className="grid grid-cols-2 gap-3 py-2">
              <div className="grid gap-1.5">
                <Label>Dias de Análise</Label>
                <Input type="number" value={editParams.dias_analise ?? 90}
                  onChange={e => setEditParams(p => ({ ...p, dias_analise: +e.target.value }))} />
                <p className="text-xs text-muted-foreground">Janela de vendas para o giro médio</p>
              </div>
              <div className="grid gap-1.5">
                <Label>Fator de Segurança</Label>
                <Input type="number" step="0.01" value={editParams.fator_seguranca ?? 1.10}
                  onChange={e => setEditParams(p => ({ ...p, fator_seguranca: +e.target.value }))} />
                <p className="text-xs text-muted-foreground">1.10 = +10% sobre a média</p>
              </div>
              <div className="grid gap-1.5">
                <Label>Curva A — máx. dias estoque</Label>
                <Input type="number" value={editParams.curva_a_max_est ?? 7}
                  onChange={e => setEditParams(p => ({ ...p, curva_a_max_est: +e.target.value }))} />
              </div>
              <div className="grid gap-1.5">
                <Label>Curva B — máx. dias estoque</Label>
                <Input type="number" value={editParams.curva_b_max_est ?? 15}
                  onChange={e => setEditParams(p => ({ ...p, curva_b_max_est: +e.target.value }))} />
              </div>
              <div className="grid gap-1.5">
                <Label>Curva C — máx. dias estoque</Label>
                <Input type="number" value={editParams.curva_c_max_est ?? 30}
                  onChange={e => setEditParams(p => ({ ...p, curva_c_max_est: +e.target.value }))} />
              </div>
              <div className="grid gap-1.5">
                <Label>Capacidade mínima absoluta</Label>
                <Input type="number" value={editParams.min_capacidade ?? 1}
                  onChange={e => setEditParams(p => ({ ...p, min_capacidade: +e.target.value }))} />
              </div>
              <div className="col-span-2 flex items-center gap-2 pt-1">
                <input type="checkbox" id="curva-a-nunca"
                  checked={editParams.curva_a_nunca_reduz ?? true}
                  onChange={e => setEditParams(p => ({ ...p, curva_a_nunca_reduz: e.target.checked }))} />
                <Label htmlFor="curva-a-nunca" className="cursor-pointer">
                  Curva A: nunca reduzir capacidade
                </Label>
              </div>
              <div className="col-span-2 border-t pt-3 mt-1">
                <p className="text-xs font-semibold text-muted-foreground mb-2 uppercase tracking-wide">Retenção de Dados</p>
              </div>
              <div className="grid gap-1.5 col-span-2">
                <Label>Retenção de importações (meses)</Label>
                <Input type="number" min={1} max={60}
                  value={editParams.retencao_csv_meses ?? 6}
                  onChange={e => setEditParams(p => ({ ...p, retencao_csv_meses: +e.target.value }))} />
                <p className="text-xs text-muted-foreground">
                  Dados brutos de CSV removidos após este período. Propostas e histórico preservados.
                </p>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setParamsDialog(null)}>Cancelar</Button>
            <Button disabled={salvarParams.isPending} onClick={() => salvarParams.mutate()}>
              {salvarParams.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={filialDialog} onOpenChange={setFilialDialog}>
        <DialogContent className="max-w-sm">
          <DialogHeader><DialogTitle>Nova Filial</DialogTitle></DialogHeader>
          <div className="space-y-3 py-2">
            <div className="grid gap-1.5">
              <Label>Código WMS (CODFILIAL)</Label>
              <Input type="number" value={newFilialCod} onChange={e => setNewFilialCod(e.target.value)} placeholder="11" />
            </div>
            <div className="grid gap-1.5">
              <Label>Nome</Label>
              <Input value={newFilialNome} onChange={e => setNewFilialNome(e.target.value)} placeholder="Filial SP" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setFilialDialog(false)}>Cancelar</Button>
            <Button disabled={criarFilial.isPending || !newFilialCod || !newFilialNome}
              onClick={() => criarFilial.mutate()}>
              {criarFilial.isPending ? 'Criando...' : 'Criar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!cdDialog} onOpenChange={() => setCdDialog(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader><DialogTitle>Novo Centro de Distribuição</DialogTitle></DialogHeader>
          <div className="space-y-3 py-2">
            <div className="grid gap-1.5">
              <Label>Nome do CD</Label>
              <Input value={newCDNome} onChange={e => setNewCDNome(e.target.value)} placeholder="CD Principal" />
            </div>
            <div className="grid gap-1.5">
              <Label>Descrição (opcional)</Label>
              <Textarea value={newCDDesc} onChange={e => setNewCDDesc(e.target.value)} rows={2} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCdDialog(null)}>Cancelar</Button>
            <Button disabled={criarCD.isPending || !newCDNome} onClick={() => criarCD.mutate()}>
              {criarCD.isPending ? 'Criando...' : 'Criar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={!!dupDialog} onOpenChange={() => setDupDialog(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader><DialogTitle>Duplicar CD</DialogTitle></DialogHeader>
          <div className="space-y-3 py-2">
            <p className="text-sm text-muted-foreground">Copia "{dupDialog?.nome}" com todos os parâmetros.</p>
            <div className="grid gap-1.5">
              <Label>Nome do novo CD</Label>
              <Input value={dupNome} onChange={e => setDupNome(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDupDialog(null)}>Cancelar</Button>
            <Button disabled={duplicarCD.isPending || !dupNome} onClick={() => duplicarCD.mutate()}>
              {duplicarCD.isPending ? 'Duplicando...' : 'Duplicar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )

  // ── Render: Filiais e CDs ────────────────────────────────────────────────────
  if (isFiliais) return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
            <h3 className="text-sm font-semibold">Filiais cadastradas</h3>
            <Button size="sm" onClick={() => setFilialDialog(true)}>
              <Plus className="h-4 w-4 mr-1" /> Nova Filial
            </Button>
          </div>

          {loadingFiliais ? (
            <p className="text-sm text-muted-foreground">Carregando...</p>
          ) : (
            <div className="border rounded-md divide-y">
              {filiais.length === 0 && (
                <p className="p-4 text-sm text-muted-foreground text-center">Nenhuma filial cadastrada.</p>
              )}
              {filiais.map(f => (
                <div key={f.id}>
                  {/* Linha da filial */}
                  <div className="flex items-center gap-2 px-4 py-2.5 hover:bg-gray-50">
                    <button
                      className="p-0.5 text-muted-foreground"
                      onClick={() => setExpandedFilial(expandedFilial === f.id ? null : f.id)}
                    >
                      {expandedFilial === f.id
                        ? <ChevronDown className="h-4 w-4" />
                        : <ChevronRight className="h-4 w-4" />}
                    </button>
                    <span className="font-medium text-sm flex-1">{f.nome}</span>
                    <Badge variant="outline" className="text-xs">Cód. {f.cod_filial}</Badge>
                    <span className="text-xs text-muted-foreground">{f.num_cds} CD(s)</span>
                    {!f.ativo && <Badge variant="secondary" className="text-xs">Inativo</Badge>}
                    <Button size="icon" variant="ghost" className="h-7 w-7 text-red-500"
                      onClick={() => desativarFilial.mutate(f.id)}>
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>

                  {/* CDs da filial (expansível) */}
                  {expandedFilial === f.id && (
                    <div className="bg-gray-50 px-8 py-2 space-y-1">
                      <div className="flex justify-between items-center mb-2">
                        <span className="text-xs text-muted-foreground font-medium">Centros de Distribuição</span>
                        <Button size="sm" variant="outline" className="h-7 text-xs"
                          onClick={() => setCdDialog({ filialID: f.id })}>
                          <Plus className="h-3 w-3 mr-1" /> Novo CD
                        </Button>
                      </div>
                      {cds.filter(cd => cd.filial_id === f.id).length === 0 && (
                        <p className="text-xs text-muted-foreground italic">Nenhum CD cadastrado.</p>
                      )}
                      {cds.filter(cd => cd.filial_id === f.id).map(cd => (
                        <div key={cd.id} className="flex items-center gap-2 py-1">
                          <span className="text-sm flex-1">{cd.nome}</span>
                          {cd.fonte_cd_id && (
                            <Badge variant="outline" className="text-[10px]">cópia</Badge>
                          )}
                          {!cd.ativo && <Badge variant="secondary" className="text-xs">Inativo</Badge>}
                          <Button size="icon" variant="ghost" className="h-7 w-7" title="Parâmetros"
                            onClick={() => openParams(cd)}>
                            <Settings2 className="h-3.5 w-3.5" />
                          </Button>
                          <Button size="icon" variant="ghost" className="h-7 w-7" title="Duplicar"
                            onClick={() => { setDupDialog(cd); setDupNome(cd.nome + ' (cópia)') }}>
                            <Copy className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
      {sharedDialogs}
    </div>
  )

  // ── Render: Plano e Limites ──────────────────────────────────────────────────
  if (isPlanos) return (
    <div className="space-y-4">
      {loadingPlano && <p className="text-sm text-muted-foreground py-4">Carregando plano...</p>}
      {erroPlano && <p className="text-sm text-destructive py-4">Erro ao carregar informações do plano.</p>}
      {plano && (
        <div className="space-y-4">
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium">Plano atual:</span>
            <Badge className="capitalize">{plano.plano}</Badge>
            {!plano.ativo && <Badge variant="destructive">Inativo</Badge>}
            {plano.valido_ate && (
              <span className="text-xs text-muted-foreground">
                Válido até: {new Date(plano.valido_ate).toLocaleDateString('pt-BR')}
              </span>
            )}
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Recurso</TableHead>
                <TableHead>Uso</TableHead>
                <TableHead>Limite</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow>
                <TableCell>Filiais</TableCell>
                <TableCell><UsageBar used={plano.usado_filiais} max={plano.max_filiais} /></TableCell>
                <TableCell className="text-sm">{plano.max_filiais === -1 ? 'Ilimitado' : plano.max_filiais}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>CDs</TableCell>
                <TableCell><UsageBar used={plano.usado_cds} max={plano.max_cds} /></TableCell>
                <TableCell className="text-sm">{plano.max_cds === -1 ? 'Ilimitado' : plano.max_cds}</TableCell>
              </TableRow>
              <TableRow>
                <TableCell>Usuários</TableCell>
                <TableCell><UsageBar used={plano.usado_usuarios} max={plano.max_usuarios} /></TableCell>
                <TableCell className="text-sm">{plano.max_usuarios === -1 ? 'Ilimitado' : plano.max_usuarios}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )

  // ── Render: Manutenção ───────────────────────────────────────────────────────
  if (isManutencao) return (
    <div className="space-y-4 max-w-lg">
      <div className="border rounded-md p-4 space-y-3">
        <div className="flex items-start gap-3">
          <AlertTriangle className="h-5 w-5 text-amber-500 shrink-0 mt-0.5" />
          <div className="space-y-1">
            <p className="text-sm font-medium">Limpar Dados de Calibragem</p>
            <p className="text-xs text-muted-foreground">
              Remove todos os imports de CSV, endereços, propostas e histórico de calibragem.
              <strong> Preserva</strong> filiais, CDs, parâmetros do motor e usuários.
            </p>
          </div>
        </div>
        {spRole === 'admin_fbtax' && (
          <Button variant="destructive" size="sm" onClick={() => setLimparDialog(true)}>
            <Trash2 className="h-4 w-4 mr-1.5" />
            Limpar dados de calibragem
          </Button>
        )}
      </div>

      <Dialog open={limparDialog} onOpenChange={v => { setLimparDialog(v); if (!v) setLimparConfirm('') }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="h-5 w-5" />
              Limpar dados de calibragem
            </DialogTitle>
            <DialogDescription className="sr-only">
              Confirmação obrigatória para limpar todos os dados de calibragem da empresa.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <p className="text-sm text-muted-foreground">
              Esta ação irá remover <strong>todos</strong> os imports, endereços, propostas e
              histórico de calibragem da empresa. Os cadastros (filiais, CDs e parâmetros)
              serão mantidos. <strong className="text-destructive">Essa operação é irreversível.</strong>
            </p>
            <div className="space-y-1.5">
              <Label className="text-sm">
                Digite <span className="font-mono font-bold text-destructive">CONFIRMO</span> para habilitar a limpeza
              </Label>
              <Input
                value={limparConfirm}
                onChange={e => setLimparConfirm(e.target.value)}
                placeholder="CONFIRMO"
                className="font-mono"
                autoComplete="off"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setLimparDialog(false); setLimparConfirm('') }}>
              Cancelar
            </Button>
            <Button
              variant="destructive"
              disabled={limparConfirm !== 'CONFIRMO' || limparCalibragem.isPending}
              onClick={() => limparCalibragem.mutate()}
            >
              {limparCalibragem.isPending ? 'Limpando...' : 'Confirmar limpeza'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Limpeza de Cadastros ── */}
      <div className="border rounded-md p-4 space-y-3">
        <div className="flex items-start gap-3">
          <AlertTriangle className="h-5 w-5 text-amber-500 shrink-0 mt-0.5" />
          <div className="space-y-1">
            <p className="text-sm font-medium">Limpar Cadastros de RCAs e Gestores</p>
            <p className="text-xs text-muted-foreground">
              Remove todos os RCAs, Gestores e vínculos entre eles.
              <strong> Use antes de reimportar</strong> o CSV de RCAS_ATIVOS do zero.
              <strong className="text-destructive"> Irreversível.</strong>
            </p>
          </div>
        </div>
        <Button variant="destructive" size="sm" onClick={() => setLimparCadDialog(true)}>
          <Trash2 className="h-4 w-4 mr-1.5" />
          Limpar cadastros
        </Button>
      </div>

      <Dialog open={limparCadDialog} onOpenChange={v => { setLimparCadDialog(v); if (!v) setLimparCadConfirm('') }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="h-5 w-5" />
              Limpar Cadastros de RCAs e Gestores
            </DialogTitle>
            <DialogDescription className="sr-only">
              Confirmação obrigatória para limpar todos os cadastros de RCAs e Gestores.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <p className="text-sm text-muted-foreground">
              Esta ação irá remover <strong>todos</strong> os RCAs, Gestores e vínculos cadastrados.
              Utilize quando precisar reimportar o CSV completo do zero.{' '}
              <strong className="text-destructive">Essa operação é irreversível.</strong>
            </p>
            <div className="space-y-1.5">
              <Label className="text-sm">
                Digite <span className="font-mono font-bold text-destructive">CONFIRMO</span> para habilitar a limpeza
              </Label>
              <Input
                value={limparCadConfirm}
                onChange={e => setLimparCadConfirm(e.target.value)}
                placeholder="CONFIRMO"
                className="font-mono"
                autoComplete="off"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setLimparCadDialog(false); setLimparCadConfirm('') }}>
              Cancelar
            </Button>
            <Button
              variant="destructive"
              disabled={limparCadConfirm !== 'CONFIRMO' || limparCadastros.isPending}
              onClick={() => limparCadastros.mutate()}
            >
              {limparCadastros.isPending ? 'Limpando...' : 'Confirmar limpeza'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )

  return null
}
