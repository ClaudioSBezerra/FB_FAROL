import { useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { CheckCircle2, ChevronLeft, ChevronRight, CircleDashed, FileUp, Lock, RefreshCw, Upload } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { parseClientesNumericasXlsx, parseClientesValidosXlsx, parsePPAsXlsx } from '@/lib/metasXlsx'

// Carga do mês — tela única do administrador (pedido do Claudio 09/10/2026:
// "menu o mais simples possível", uma ABA de carga por indústria, cada uma
// com sua métrica e seu layout). Por indústria e por programa (Ponderada /
// Numérica): 1) cria a vigência do mês com as faixas, 2) solta o arquivo da
// JC, 3) confere o checklist, reprocessa ou fecha. Usa os mesmos endpoints
// das telas antigas (Configurações → Objetivos por Indústria, que continua
// existindo pra cadastro de vínculos e parâmetros).

interface Industria { id: number; nome: string }
interface Faixa { faixa: number; valor_meta: number }
interface CargaVinculo {
  vinculo_id: number
  industria_id: number
  industria_nome: string
  tipo_metrica_nome: string
  formula_codigo: string
  vigencia_id: number | null
  data_inicio?: string
  data_fim?: string
  status?: string
  faixas: Faixa[]
  clientes: number
  itens: number
  ppas: number
  clientes_numerica: number
  calculado_em: string | null
}

type Programa = 'Ponderada' | 'Numérica'
const programaDe = (formula: string): Programa => (formula.includes('numerica') ? 'Numérica' : 'Ponderada')

// O que cada vínculo precisa ter carregado pra a vigência do mês funcionar.
function pendencias(v: CargaVinculo): string[] {
  if (v.vigencia_id == null) return ['vigência']
  const out: string[] = []
  if (programaDe(v.formula_codigo) === 'Numérica') {
    if (v.clientes_numerica === 0) out.push('clientes')
    if (v.formula_codigo === 'sortimento_numerica_ppa' && v.ppas === 0) out.push('PPAs')
  } else {
    if (v.clientes === 0) out.push('clientes')
    if (v.formula_codigo === 'sortimento_rede' && v.itens === 0) out.push('itens')
  }
  return out
}

const mesAtual = () => new Date().toISOString().slice(0, 7)
function somaMes(mes: string, delta: number) {
  const [a, m] = mes.split('-').map(Number)
  const d = new Date(a, m - 1 + delta, 1)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}
function limitesDoMes(mes: string) {
  const [a, m] = mes.split('-').map(Number)
  const ultimo = new Date(a, m, 0).getDate()
  return { inicio: `${mes}-01`, fim: `${mes}-${String(ultimo).padStart(2, '0')}` }
}
const rotuloMes = (mes: string) => {
  const [a, m] = mes.split('-').map(Number)
  const t = new Date(a, m - 1, 1).toLocaleDateString('pt-BR', { month: 'long', year: 'numeric' }).replace(' de ', ' de ')
  return t.charAt(0).toUpperCase() + t.slice(1)
}

export default function ConfigCargaMes() {
  const { token } = useAuth()
  const qc = useQueryClient()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])
  const [mes, setMes] = useState(mesAtual)
  const [industriaID, setIndustriaID] = useState<number | null>(null)

  const { data: industrias = [] } = useQuery<Industria[]>({
    queryKey: ['farol-industrias'],
    queryFn: async () => {
      const r = await fetch('/api/farol/industrias', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })
  const buscarStatus = (m: string) => async (): Promise<CargaVinculo[]> => {
    const r = await fetch(`/api/farol/metas-carga-status?mes=${m}`, { headers })
    if (!r.ok) throw new Error(await r.text())
    return r.json()
  }
  const { data: status = [], isLoading } = useQuery({ queryKey: ['carga-status', mes], queryFn: buscarStatus(mes) })
  const { data: statusAnterior = [] } = useQuery({ queryKey: ['carga-status', somaMes(mes, -1)], queryFn: buscarStatus(somaMes(mes, -1)) })
  const recarregar = () => qc.invalidateQueries({ queryKey: ['carga-status'] })

  const porIndustria = useMemo(() => {
    const m = new Map<number, CargaVinculo[]>()
    for (const v of status) m.set(v.industria_id, [...(m.get(v.industria_id) ?? []), v])
    return m
  }, [status])

  const lista = useMemo(
    () => [...industrias].sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR')),
    [industrias],
  )
  const selecionada = industriaID ?? lista.find(i => porIndustria.has(i.id))?.id ?? lista[0]?.id ?? null
  const vinculos = selecionada != null ? porIndustria.get(selecionada) ?? [] : []

  function corDaAba(id: number): string {
    const vs = porIndustria.get(id)
    if (!vs || vs.length === 0) return 'bg-slate-300'
    const pend = vs.reduce((n, v) => n + pendencias(v).length, 0)
    if (pend === 0) return 'bg-emerald-500'
    return vs.some(v => v.vigencia_id != null) ? 'bg-amber-500' : 'bg-rose-500'
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">Carga do mês</h1>
          <p className="text-sm text-muted-foreground">Uma aba por indústria: crie a vigência, envie o arquivo da JC e confira.</p>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="outline" size="icon" onClick={() => setMes(m => somaMes(m, -1))} aria-label="Mês anterior"><ChevronLeft className="w-4 h-4" /></Button>
          <div className="w-44 text-center text-sm font-semibold">{rotuloMes(mes)}</div>
          <Button variant="outline" size="icon" onClick={() => setMes(m => somaMes(m, 1))} aria-label="Próximo mês"><ChevronRight className="w-4 h-4" /></Button>
        </div>
      </div>

      {/* Uma aba por indústria — bolinha: verde pronto, âmbar em andamento, vermelho sem nada, cinza sem programa */}
      <div className="flex gap-1 overflow-x-auto border-b pb-0">
        {lista.map(i => (
          <button
            key={i.id}
            onClick={() => setIndustriaID(i.id)}
            className={`shrink-0 flex items-center gap-1.5 px-3 py-2 text-sm border-b-2 -mb-px whitespace-nowrap ${
              selecionada === i.id ? 'border-violet-600 text-violet-700 font-semibold' : 'border-transparent text-slate-600 hover:text-slate-900'
            }`}
          >
            <span className={`w-2 h-2 rounded-full ${corDaAba(i.id)}`} />
            {i.nome}
          </button>
        ))}
      </div>

      {isLoading && <p className="text-sm text-muted-foreground">Carregando...</p>}
      {!isLoading && selecionada != null && vinculos.length === 0 && (
        <div className="border rounded-lg p-4 text-sm text-muted-foreground">
          Esta indústria ainda não tem programa configurado. Cadastre o vínculo (métrica e parâmetros) em
          Configurações → <Link className="underline" to="/gestao/metas-vinculos">Objetivos por Indústria</Link>.
        </div>
      )}

      {(['Ponderada', 'Numérica'] as Programa[]).map(prog => {
        const doPrograma = vinculos.filter(v => programaDe(v.formula_codigo) === prog)
        if (doPrograma.length === 0) return null
        return (
          <ProgramaCard
            key={`${selecionada}-${prog}-${mes}`}
            programa={prog}
            mes={mes}
            vinculos={doPrograma}
            anteriores={statusAnterior.filter(a => doPrograma.some(v => v.vinculo_id === a.vinculo_id))}
            headers={headers}
            onMudou={recarregar}
          />
        )
      })}
    </div>
  )
}

// ─── Um programa (Ponderada ou Numérica) de uma indústria ────────────────────

async function enviarCSV(url: string, csv: string, nome: string, headers: Record<string, string>) {
  const body = new FormData()
  body.append('file', new File([csv], nome, { type: 'text/csv' }))
  const r = await fetch(url, { method: 'POST', headers, body })
  const data = await r.json().catch(() => ({}))
  if (!r.ok) {
    const erros = Array.isArray(data?.erros)
      ? data.erros.slice(0, 8).map((e: { linha: number; erro: string }) => `Linha ${e.linha || '-'}: ${e.erro}`).join('\n')
      : (data?.error ?? `Erro ${r.status}`)
    throw new Error(erros)
  }
  return data as Record<string, unknown>
}

function ProgramaCard({ programa, mes, vinculos, anteriores, headers, onMudou }: {
  programa: Programa
  mes: string
  vinculos: CargaVinculo[]
  anteriores: CargaVinculo[]
  headers: Record<string, string>
  onMudou: () => void
}) {
  const { inicio, fim } = limitesDoMes(mes)
  const [faixas, setFaixas] = useState<Record<number, string[]>>({})
  const [ocupado, setOcupado] = useState('')
  const [confirmarFechar, setConfirmarFechar] = useState(false)
  const arquivoRef = useRef<HTMLInputElement>(null)
  const itensRef = useRef<HTMLInputElement>(null)

  const faltaVigencia = vinculos.filter(v => v.vigencia_id == null)
  const comVigencia = vinculos.filter(v => v.vigencia_id != null)
  const abertas = comVigencia.filter(v => v.status === 'aberta')
  const totalPend = vinculos.reduce((n, v) => n + pendencias(v).length, 0)
  const pronto = totalPend === 0

  // Faixas pré-preenchidas com as do mês anterior (o administrador só ajusta).
  const faixasDe = (v: CargaVinculo): string[] => {
    if (faixas[v.vinculo_id]) return faixas[v.vinculo_id]
    const ant = anteriores.find(a => a.vinculo_id === v.vinculo_id)
    const base = ant?.faixas?.length ? ant.faixas.map(f => String(f.valor_meta)) : ['', '', '']
    return base
  }

  async function executar(rotulo: string, fn: () => Promise<void>) {
    setOcupado(rotulo)
    try { await fn() } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Erro', { style: { whiteSpace: 'pre-line' } })
    } finally { setOcupado(''); onMudou() }
  }

  const criarVigencias = () => executar('vigencia', async () => {
    for (const v of faltaVigencia) {
      const valores = faixasDe(v).map(s => Number(s.replace(',', '.')))
      if (valores.length === 0 || valores.some(n => !Number.isFinite(n) || n <= 0)) {
        throw new Error(`Preencha as faixas de ${v.tipo_metrica_nome}`)
      }
      const r = await fetch('/api/farol/metas-vigencias', {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          vinculo_id: v.vinculo_id, data_inicio: inicio, data_fim: fim,
          faixas: valores.map((valor_meta, i) => ({ faixa: i + 1, valor_meta })),
        }),
      })
      if (!r.ok) throw new Error(`${v.tipo_metrica_nome}: ${await r.text()}`)
    }
    toast.success(`${faltaVigencia.length} vigência(s) criada(s) para ${inicio.slice(0, 7)}`)
  })

  const enviarArquivo = (file: File) => executar('arquivo', async () => {
    if (abertas.length === 0) throw new Error('Crie a vigência do mês antes de enviar o arquivo')
    let total = 0
    if (programa === 'Ponderada') {
      const csv = await parseClientesValidosXlsx(file)
      for (const v of abertas) {
        const d = await enviarCSV(`/api/farol/metas-clientes-validos-importar-csv?vinculo_id=${v.vinculo_id}&vigencia_id=${v.vigencia_id}`, csv, 'clientes.csv', headers)
        total = Number(d.clientes_importados ?? 0)
        if (Array.isArray(d.avisos) && d.avisos.length) toast.warning(`${v.tipo_metrica_nome}: ${d.avisos.length} linha(s) com aviso ficaram de fora`)
      }
      toast.success(`${total} cliente(s) importado(s) em ${abertas.length} vínculo(s)`)
    } else {
      const csv = await parseClientesNumericasXlsx(file)
      for (const v of abertas) {
        const d = await enviarCSV(`/api/farol/metas-clientes-numericas-importar-csv?vinculo_id=${v.vinculo_id}&vigencia_id=${v.vigencia_id}`, csv, 'clientes_numericas.csv', headers)
        total = Number(d.clientes_importados ?? 0)
      }
      const comPPA = abertas.filter(v => v.formula_codigo === 'sortimento_numerica_ppa')
      let ppas = 0
      if (comPPA.length > 0) {
        const csvPPA = await parsePPAsXlsx(file)
        for (const v of comPPA) {
          const d = await enviarCSV(`/api/farol/metas-ppas-importar-csv?vinculo_id=${v.vinculo_id}&vigencia_id=${v.vigencia_id}`, csvPPA, 'ppas.csv', headers)
          ppas = Number(d.ppas_importados ?? 0)
        }
      }
      toast.success(`${total} cliente(s) e ${ppas} linha(s) de PPA importados`)
    }
  })

  const enviarItens = (file: File) => executar('itens', async () => {
    const alvo = abertas.filter(v => v.formula_codigo === 'sortimento_rede')
    if (alvo.length === 0) throw new Error('Nenhuma vigência aberta de Sortimento por Rede')
    const csv = await file.text()
    let n = 0
    for (const v of alvo) {
      const d = await enviarCSV(`/api/farol/metas-itens-validos-importar-csv?vinculo_id=${v.vinculo_id}&vigencia_id=${v.vigencia_id}`, csv, 'itens.csv', headers)
      n = Number(d.itens_importados ?? 0)
    }
    toast.success(`${n} item(ns) importado(s)`)
  })

  const reprocessar = () => executar('reprocessar', async () => {
    for (const v of comVigencia) {
      const r = await fetch(`/api/farol/metas-vigencias/${v.vigencia_id}/reprocessar`, { method: 'POST', headers })
      if (!r.ok) throw new Error(await r.text())
    }
    toast.success('Reprocessamento iniciado — os números atualizam em alguns minutos')
  })

  const fechar = () => executar('fechar', async () => {
    for (const v of abertas) {
      const r = await fetch(`/api/farol/metas-vigencias/${v.vigencia_id}/fechar`, { method: 'POST', headers })
      if (!r.ok) throw new Error(await r.text())
    }
    toast.success('Mês fechado — resultado congelado')
  })

  const temItens = vinculos.some(v => v.formula_codigo === 'sortimento_rede')
  return (
    <div className="border rounded-xl bg-white">
      <div className="flex items-center justify-between gap-3 px-4 py-3 border-b">
        <div className="flex items-center gap-2">
          <h2 className="font-semibold">{programa === 'Ponderada' ? 'Ponderada (por Rede)' : 'Numérica (por Cliente)'}</h2>
          {pronto
            ? <Badge className="bg-emerald-600"><CheckCircle2 className="w-3 h-3 mr-1" />Pronto</Badge>
            : <Badge variant="secondary"><CircleDashed className="w-3 h-3 mr-1" />Falta {totalPend}</Badge>}
        </div>
        <Link className="text-xs underline text-violet-700" to="/farol/metas-industria">Abrir painel</Link>
      </div>

      {/* 1. Vigência */}
      <div className="px-4 py-3 space-y-2">
        <div className="text-xs font-semibold uppercase text-slate-500">1. Vigência e faixas — {inicio.split('-').reverse().join('/')} a {fim.split('-').reverse().join('/')}</div>
        {vinculos.map(v => (
          <div key={v.vinculo_id} className="flex flex-wrap items-center gap-3 text-sm">
            <div className="w-56 font-medium">{v.tipo_metrica_nome}</div>
            {v.vigencia_id != null ? (
              <>
                <Badge variant={v.status === 'aberta' ? 'default' : 'secondary'}>{v.status === 'aberta' ? 'Aberta' : 'Fechada'}</Badge>
                <span className="font-mono text-xs text-slate-600">{v.faixas.map(f => f.valor_meta).join(' / ')}</span>
              </>
            ) : (
              <div className="flex items-center gap-1">
                {faixasDe(v).map((val, i) => (
                  <Input
                    key={i} value={val} inputMode="decimal" className="w-20 h-8 text-xs" placeholder={`Faixa ${i + 1}`}
                    onChange={e => setFaixas(f => { const n = [...faixasDe(v)]; n[i] = e.target.value; return { ...f, [v.vinculo_id]: n } })}
                  />
                ))}
              </div>
            )}
          </div>
        ))}
        {faltaVigencia.length > 0 && (
          <Button size="sm" onClick={criarVigencias} disabled={ocupado !== ''}>
            {ocupado === 'vigencia' ? 'Criando...' : `Criar vigência${faltaVigencia.length > 1 ? 's' : ''} do mês`}
          </Button>
        )}
      </div>

      {/* 2. Arquivo da JC */}
      <div className="px-4 py-3 border-t space-y-2">
        <div className="text-xs font-semibold uppercase text-slate-500">2. Arquivo da JC</div>
        <div className="flex flex-wrap items-center gap-2">
          <input ref={arquivoRef} type="file" accept=".xlsx,.xls" className="hidden"
            onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) enviarArquivo(f) }} />
          <Button variant="outline" size="sm" onClick={() => arquivoRef.current?.click()} disabled={ocupado !== '' || abertas.length === 0}>
            <FileUp className="w-4 h-4 mr-1" />
            {ocupado === 'arquivo' ? 'Importando...' : programa === 'Ponderada' ? 'Enviar clientes (.xlsx da JC)' : 'Enviar clientes e PPAs (.xlsx da JC)'}
          </Button>
          {temItens && (
            <>
              <input ref={itensRef} type="file" accept=".csv" className="hidden"
                onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) enviarItens(f) }} />
              <Button variant="outline" size="sm" onClick={() => itensRef.current?.click()} disabled={ocupado !== '' || abertas.length === 0}>
                <Upload className="w-4 h-4 mr-1" />{ocupado === 'itens' ? 'Importando...' : 'Enviar itens (CSV ean;cod_prod)'}
              </Button>
            </>
          )}
        </div>
        <ul className="text-xs text-slate-600 space-y-0.5">
          {comVigencia.map(v => {
            const p = pendencias(v)
            return (
              <li key={v.vinculo_id} className="flex items-center gap-2">
                {p.length === 0 ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" /> : <CircleDashed className="w-3.5 h-3.5 text-amber-600" />}
                <span className="w-56">{v.tipo_metrica_nome}</span>
                {programa === 'Numérica'
                  ? <span>{v.clientes_numerica.toLocaleString('pt-BR')} clientes{v.formula_codigo === 'sortimento_numerica_ppa' && ` · ${v.ppas} PPAs`}</span>
                  : <span>{v.clientes.toLocaleString('pt-BR')} clientes{v.formula_codigo === 'sortimento_rede' && ` · ${v.itens} itens`}</span>}
                {p.length > 0 && <span className="text-amber-700">falta: {p.join(', ')}</span>}
              </li>
            )
          })}
        </ul>
      </div>

      {/* 3. Conferir, reprocessar, fechar */}
      <div className="px-4 py-3 border-t flex flex-wrap items-center gap-2">
        <div className="text-xs font-semibold uppercase text-slate-500 mr-2">3. Fechamento</div>
        <Button variant="outline" size="sm" onClick={reprocessar} disabled={ocupado !== '' || comVigencia.length === 0}>
          <RefreshCw className="w-4 h-4 mr-1" />{ocupado === 'reprocessar' ? 'Iniciando...' : 'Reprocessar'}
        </Button>
        <Button variant="outline" size="sm" onClick={() => setConfirmarFechar(true)} disabled={ocupado !== '' || abertas.length === 0}>
          <Lock className="w-4 h-4 mr-1" />Fechar mês
        </Button>
        <span className="text-xs text-muted-foreground">Fechar congela o resultado e bloqueia novas cargas.</span>
      </div>

      <AlertDialog open={confirmarFechar} onOpenChange={setConfirmarFechar}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Fechar {rotuloMes(mes)} — {programa}?</AlertDialogTitle>
            <AlertDialogDescription>
              O resultado fica congelado e não será mais recalculado nem aceitará novas listas. Só um reprocessamento
              manual altera depois. Feche só depois da validação do fechamento.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction onClick={fechar}>Fechar mês</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
