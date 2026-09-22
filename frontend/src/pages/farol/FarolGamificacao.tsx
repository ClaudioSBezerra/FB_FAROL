import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { toast } from 'sonner'
import { Plus, Trash2, Trophy, RefreshCw, ArrowLeft, Eye, Pencil } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Gamificação — MVP (pedido do José Costa, CEO da JC, via Claudio
// 22/09/2026): "a data de última venda não vai fazer o RCA vender mais" —
// pontos, ranking e bônus (inclusive em R$) por campanha, no estilo
// Mercado Livre/iFood. Acesso restrito a admin_fbtax (ver FbtaxAdminRoute
// em App.tsx e o rail em AppRail.tsx) — só o Claudio vê este menu por
// enquanto. O MVP CALCULA e MOSTRA o bônus; não paga nada automaticamente
// (RCAs são autônomos — processo de pagamento fica fora de escopo aqui).

interface Industria { id: number; nome: string }
interface MetaVinculo { id: number; industria_id: number; formula_codigo: string; tipo_metrica_nome: string }
interface Vigencia { id: number; vinculo_id: number; data_inicio: string; data_fim: string; status: string }

interface GamifRegra {
  id: number
  campanha_id: number
  tipo: 'cobertura_atingida' | 'sortimento_atingido' | 'produto_especifico'
  descricao: string
  vinculo_id?: number
  vigencia_id?: number
  cod_prods?: string[]
  qtd_minima?: number
  fluxo: string
  pontos: number
  valor_bonus: number
}

interface GamifCampanha {
  id: number
  industria_id: number
  industria_nome: string
  nome: string
  data_inicio: string
  data_fim: string
  status: 'ativa' | 'encerrada'
  created_at: string
  regras?: GamifRegra[]
}

interface GamifRankingLinha {
  posicao: number
  cod_rca: string
  nome_rca: string
  pontos_total: number
  bonus_total: number
}

interface GamifMinhaPosicao {
  cod_rca: string
  posicao: number
  total_rcas: number
  pontos_total: number
  bonus_total: number
}

const fmtBRL = (n: number) => (n ?? 0).toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })
const fmt = (n: number) => (n ?? 0).toLocaleString('pt-BR', { maximumFractionDigits: 2 })

const TIPO_LABEL: Record<string, string> = {
  cobertura_atingida: 'Cobertura atingida (por Rede)',
  sortimento_atingido: 'Sortimento atingido (por Rede)',
  produto_especifico: 'Produto específico (por RCA)',
}

export default function FarolGamificacao() {
  const { token } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }), [token])
  const qc = useQueryClient()

  const [campanhaSelecionada, setCampanhaSelecionada] = useState<number | null>(null)
  const [novaCampanhaOpen, setNovaCampanhaOpen] = useState(false)
  const [novaRegraOpen, setNovaRegraOpen] = useState(false)
  // regraEditando — achado do Claudio 22/09/2026: "uma vez criada não
  // estou conseguindo editar" — faltava o modo edição no dialog (só
  // existia criar). null = criando regra nova; number = editando esse id.
  const [regraEditando, setRegraEditando] = useState<number | null>(null)
  const [rcaSimulado, setRcaSimulado] = useState('')

  const { data: campanhas } = useQuery<GamifCampanha[]>({
    queryKey: ['gamif-campanhas'],
    queryFn: async () => (await fetch('/api/farol/gamif-campanhas', { headers })).json(),
  })

  const { data: industrias } = useQuery<Industria[]>({
    queryKey: ['industrias'],
    queryFn: async () => (await fetch('/api/farol/industrias', { headers })).json(),
  })

  const { data: vinculos } = useQuery<MetaVinculo[]>({
    queryKey: ['metas-vinculos'],
    queryFn: async () => (await fetch('/api/farol/metas-vinculos', { headers })).json(),
    enabled: novaRegraOpen,
  })

  const { data: campanhaDetalhe } = useQuery<GamifCampanha>({
    queryKey: ['gamif-campanha', campanhaSelecionada],
    queryFn: async () => (await fetch(`/api/farol/gamif-campanhas/${campanhaSelecionada}`, { headers })).json(),
    enabled: !!campanhaSelecionada,
  })

  const { data: rankingResp, isFetching: carregandoRanking } = useQuery<{ ranking: GamifRankingLinha[]; total_rcas: number }>({
    queryKey: ['gamif-ranking', campanhaSelecionada],
    queryFn: async () => (await fetch(`/api/farol/gamif-ranking?campanha_id=${campanhaSelecionada}`, { headers })).json(),
    enabled: !!campanhaSelecionada,
  })

  const { data: minhaPosicao } = useQuery<GamifMinhaPosicao | null>({
    queryKey: ['gamif-minha-posicao', campanhaSelecionada, rcaSimulado],
    queryFn: async () => {
      const r = await fetch(`/api/farol/gamif-ranking?campanha_id=${campanhaSelecionada}&cod_rca=${encodeURIComponent(rcaSimulado)}`, { headers })
      if (!r.ok) return null
      return r.json()
    },
    enabled: !!campanhaSelecionada && !!rcaSimulado,
  })

  // ─── Criar Campanha ─────────────────────────────────────────────────────
  const [formCampanha, setFormCampanha] = useState({ industria_id: '', nome: '', data_inicio: '', data_fim: '' })
  const criarCampanha = useMutation({
    mutationFn: async () => {
      const r = await fetch('/api/farol/gamif-campanhas', {
        method: 'POST', headers,
        body: JSON.stringify({ ...formCampanha, industria_id: Number(formCampanha.industria_id) }),
      })
      if (!r.ok) throw new Error(await r.text())
      return r.json()
    },
    onSuccess: () => {
      toast.success('Campanha criada')
      qc.invalidateQueries({ queryKey: ['gamif-campanhas'] })
      setNovaCampanhaOpen(false)
      setFormCampanha({ industria_id: '', nome: '', data_inicio: '', data_fim: '' })
    },
    onError: (e: Error) => toast.error(e.message || 'Erro ao criar campanha'),
  })

  const excluirCampanha = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/farol/gamif-campanhas/${id}`, { method: 'DELETE', headers })
      if (!r.ok) throw new Error(await r.text())
    },
    onSuccess: () => {
      toast.success('Campanha excluída')
      qc.invalidateQueries({ queryKey: ['gamif-campanhas'] })
      setCampanhaSelecionada(null)
    },
    onError: (e: Error) => toast.error(e.message || 'Erro ao excluir'),
  })

  // ─── Criar Regra ────────────────────────────────────────────────────────
  const [formRegra, setFormRegra] = useState({
    tipo: 'cobertura_atingida' as GamifRegra['tipo'],
    descricao: '', vinculo_id: '', vigencia_id: '',
    cod_prods: '', qtd_minima: '', pontos: '', valor_bonus: '',
  })
  const vinculosDaIndustria = (vinculos ?? []).filter(v =>
    v.industria_id === campanhaDetalhe?.industria_id &&
    v.formula_codigo === (formRegra.tipo === 'sortimento_atingido' ? 'sortimento_rede' : 'cobertura_rede')
  )
  const { data: vigenciasDoVinculo } = useQuery<Vigencia[]>({
    queryKey: ['metas-vigencias', formRegra.vinculo_id],
    queryFn: async () => (await fetch(`/api/farol/metas-vigencias?vinculo_id=${formRegra.vinculo_id}`, { headers })).json(),
    enabled: !!formRegra.vinculo_id,
  })

  const fecharDialogRegra = () => {
    setNovaRegraOpen(false)
    setRegraEditando(null)
    setFormRegra({ tipo: 'cobertura_atingida', descricao: '', vinculo_id: '', vigencia_id: '', cod_prods: '', qtd_minima: '', pontos: '', valor_bonus: '' })
  }

  const abrirEdicaoRegra = (rg: GamifRegra) => {
    setRegraEditando(rg.id)
    setFormRegra({
      tipo: rg.tipo,
      descricao: rg.descricao ?? '',
      vinculo_id: rg.vinculo_id ? String(rg.vinculo_id) : '',
      vigencia_id: rg.vigencia_id ? String(rg.vigencia_id) : '',
      cod_prods: (rg.cod_prods ?? []).join('\n'),
      qtd_minima: rg.qtd_minima ? String(rg.qtd_minima) : '',
      pontos: String(rg.pontos ?? ''),
      valor_bonus: String(rg.valor_bonus ?? ''),
    })
    setNovaRegraOpen(true)
  }

  const salvarRegra = useMutation({
    mutationFn: async () => {
      const body: Record<string, unknown> = {
        campanha_id: campanhaSelecionada,
        tipo: formRegra.tipo,
        descricao: formRegra.descricao,
        pontos: Number(formRegra.pontos) || 0,
        valor_bonus: Number(formRegra.valor_bonus) || 0,
      }
      if (formRegra.tipo === 'produto_especifico') {
        body.cod_prods = formRegra.cod_prods.split(/[\n,]/).map(s => s.trim()).filter(Boolean)
        body.qtd_minima = Number(formRegra.qtd_minima) || 0
      } else {
        body.vinculo_id = Number(formRegra.vinculo_id)
        body.vigencia_id = Number(formRegra.vigencia_id)
      }
      const url = regraEditando ? `/api/farol/gamif-regras/${regraEditando}` : '/api/farol/gamif-regras'
      const r = await fetch(url, { method: regraEditando ? 'PUT' : 'POST', headers, body: JSON.stringify(body) })
      if (!r.ok) throw new Error(await r.text())
      return r.json().catch(() => ({}))
    },
    onSuccess: () => {
      toast.success(regraEditando ? 'Regra atualizada' : 'Regra criada')
      qc.invalidateQueries({ queryKey: ['gamif-campanha', campanhaSelecionada] })
      fecharDialogRegra()
    },
    onError: (e: Error) => toast.error(e.message || 'Erro ao salvar regra'),
  })

  const excluirRegra = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/farol/gamif-regras/${id}`, { method: 'DELETE', headers })
      if (!r.ok) throw new Error(await r.text())
    },
    onSuccess: () => {
      toast.success('Regra excluída')
      qc.invalidateQueries({ queryKey: ['gamif-campanha', campanhaSelecionada] })
    },
    onError: (e: Error) => toast.error(e.message || 'Erro ao excluir'),
  })

  const recalcular = useMutation({
    mutationFn: async () => {
      const r = await fetch(`/api/farol/gamif-campanhas-calcular?campanha_id=${campanhaSelecionada}`, { method: 'POST', headers })
      if (!r.ok) throw new Error(await r.text())
    },
    onSuccess: () => {
      toast.success('Pontuação recalculada')
      qc.invalidateQueries({ queryKey: ['gamif-ranking', campanhaSelecionada] })
    },
    onError: (e: Error) => toast.error(e.message || 'Erro ao recalcular'),
  })

  // ─── Tela: detalhe de uma campanha ──────────────────────────────────────
  if (campanhaSelecionada && campanhaDetalhe) {
    return (
      <div className="p-6 max-w-5xl mx-auto space-y-4">
        <Button variant="ghost" size="sm" onClick={() => { setCampanhaSelecionada(null); setRcaSimulado('') }}>
          <ArrowLeft className="w-4 h-4 mr-1" /> Campanhas
        </Button>

        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-xl font-bold flex items-center gap-2"><Trophy className="w-5 h-5 text-amber-500" /> {campanhaDetalhe.nome}</h1>
            <p className="text-sm text-muted-foreground">
              {campanhaDetalhe.industria_nome} · {campanhaDetalhe.data_inicio} – {campanhaDetalhe.data_fim} ·{' '}
              <Badge variant={campanhaDetalhe.status === 'ativa' ? 'default' : 'secondary'}>{campanhaDetalhe.status}</Badge>
            </p>
          </div>
          <Button onClick={() => recalcular.mutate()} disabled={recalcular.isPending}>
            <RefreshCw className={`w-4 h-4 mr-1 ${recalcular.isPending ? 'animate-spin' : ''}`} /> Recalcular pontuação
          </Button>
        </div>

        {/* Regras */}
        <div className="border rounded-lg overflow-hidden">
          <div className="px-3 py-2 border-b flex items-center justify-between bg-muted/30">
            <span className="text-sm font-medium">Regras de pontuação</span>
            <Button size="sm" variant="outline" onClick={() => { setRegraEditando(null); setNovaRegraOpen(true) }}><Plus className="w-3.5 h-3.5 mr-1" /> Nova regra</Button>
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Tipo</TableHead>
                <TableHead>Descrição</TableHead>
                <TableHead>Alvo</TableHead>
                <TableHead className="text-right">Pontos</TableHead>
                <TableHead className="text-right">Bônus (R$)</TableHead>
                <TableHead></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(campanhaDetalhe.regras ?? []).length === 0 && (
                <TableRow><TableCell colSpan={6} className="text-center py-6 text-muted-foreground">Nenhuma regra ainda</TableCell></TableRow>
              )}
              {(campanhaDetalhe.regras ?? []).map(rg => (
                <TableRow key={rg.id}>
                  <TableCell className="text-sm">{TIPO_LABEL[rg.tipo]}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{rg.descricao || '—'}</TableCell>
                  <TableCell className="text-xs font-mono text-muted-foreground">
                    {rg.tipo === 'produto_especifico' ? `${(rg.cod_prods ?? []).join(', ')} (mín. ${fmt(rg.qtd_minima ?? 0)})` : `vínculo ${rg.vinculo_id} / vigência ${rg.vigencia_id}`}
                  </TableCell>
                  <TableCell className="text-right">{fmt(rg.pontos)}</TableCell>
                  <TableCell className="text-right">{fmtBRL(rg.valor_bonus)}</TableCell>
                  <TableCell className="flex gap-1">
                    <Button variant="ghost" size="sm" onClick={() => abrirEdicaoRegra(rg)}><Pencil className="w-3.5 h-3.5" /></Button>
                    <Button variant="ghost" size="sm" onClick={() => excluirRegra.mutate(rg.id)}><Trash2 className="w-3.5 h-3.5 text-red-500" /></Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>

        {/* Ranking */}
        <div className="border rounded-lg overflow-hidden">
          <div className="px-3 py-2 border-b bg-muted/30 text-sm font-medium">
            Ranking ({rankingResp?.total_rcas ?? 0} RCA{(rankingResp?.total_rcas ?? 0) === 1 ? '' : 's'})
          </div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-14">#</TableHead>
                <TableHead>RCA</TableHead>
                <TableHead className="text-right">Pontos</TableHead>
                <TableHead className="text-right">Bônus (R$)</TableHead>
                <TableHead className="w-32"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {carregandoRanking && (
                <TableRow><TableCell colSpan={5} className="text-center py-6 text-muted-foreground">Carregando...</TableCell></TableRow>
              )}
              {!carregandoRanking && (rankingResp?.ranking ?? []).length === 0 && (
                <TableRow><TableCell colSpan={5} className="text-center py-6 text-muted-foreground">Ninguém pontuou ainda — crie regras e clique em "Recalcular pontuação"</TableCell></TableRow>
              )}
              {(rankingResp?.ranking ?? []).map(l => (
                <TableRow key={l.cod_rca}>
                  <TableCell className="font-bold text-muted-foreground">{l.posicao}º</TableCell>
                  <TableCell className="text-sm">{l.nome_rca || l.cod_rca} <span className="text-xs text-muted-foreground font-mono">({l.cod_rca})</span></TableCell>
                  <TableCell className="text-right font-medium">{fmt(l.pontos_total)}</TableCell>
                  <TableCell className="text-right font-medium text-emerald-700">{fmtBRL(l.bonus_total)}</TableCell>
                  <TableCell>
                    <Button variant="ghost" size="sm" onClick={() => setRcaSimulado(l.cod_rca)}>
                      <Eye className="w-3.5 h-3.5 mr-1" /> Visão do RCA
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>

        {/* Visão do RCA — pedido do Claudio 22/09/2026: mostra a posição, nunca quem está na frente */}
        {rcaSimulado && (
          <div className="border-2 border-dashed border-primary/40 rounded-lg p-4 bg-primary/5">
            <div className="flex items-center justify-between mb-2">
              <span className="text-sm font-medium">Simulando o que {rcaSimulado} veria</span>
              <Button variant="ghost" size="sm" onClick={() => setRcaSimulado('')}>fechar</Button>
            </div>
            {minhaPosicao ? (
              <div className="text-center py-4">
                <div className="text-4xl font-bold text-primary">{minhaPosicao.posicao}º</div>
                <div className="text-sm text-muted-foreground mb-3">de {minhaPosicao.total_rcas} RCAs</div>
                <div className="flex justify-center gap-6 text-sm">
                  <span><strong>{fmt(minhaPosicao.pontos_total)}</strong> pontos</span>
                  <span className="text-emerald-700"><strong>{fmtBRL(minhaPosicao.bonus_total)}</strong> em bônus</span>
                </div>
                <p className="text-xs text-muted-foreground mt-3">Ele NÃO vê nomes de quem está na frente ou atrás — só a própria posição.</p>
              </div>
            ) : (
              <p className="text-sm text-muted-foreground text-center py-4">Este RCA ainda não pontuou nesta campanha.</p>
            )}
          </div>
        )}

        {/* Dialog: nova regra / editar regra */}
        <Dialog open={novaRegraOpen} onOpenChange={open => { if (!open) fecharDialogRegra() }}>
          <DialogContent className="max-w-lg">
            <DialogHeader><DialogTitle>{regraEditando ? 'Editar regra de pontuação' : 'Nova regra de pontuação'}</DialogTitle></DialogHeader>
            <div className="space-y-3">
              <div>
                <Label>Tipo</Label>
                <Select value={formRegra.tipo} onValueChange={(v: GamifRegra['tipo']) => setFormRegra(f => ({ ...f, tipo: v, vinculo_id: '', vigencia_id: '' }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="cobertura_atingida">Cobertura atingida (por Rede)</SelectItem>
                    <SelectItem value="sortimento_atingido">Sortimento atingido (por Rede)</SelectItem>
                    <SelectItem value="produto_especifico">Produto específico (por RCA)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label>Descrição</Label>
                <Input value={formRegra.descricao} onChange={e => setFormRegra(f => ({ ...f, descricao: e.target.value }))} placeholder='Ex: "Venda de microgarrafas Absolut"' />
              </div>

              {formRegra.tipo !== 'produto_especifico' ? (
                <>
                  <div>
                    <Label>Vínculo ({formRegra.tipo === 'sortimento_atingido' ? 'Sortimento' : 'Cobertura'} da indústria da campanha)</Label>
                    <Select value={formRegra.vinculo_id} onValueChange={v => setFormRegra(f => ({ ...f, vinculo_id: v, vigencia_id: '' }))}>
                      <SelectTrigger><SelectValue placeholder="Selecione..." /></SelectTrigger>
                      <SelectContent>
                        {vinculosDaIndustria.map(v => <SelectItem key={v.id} value={String(v.id)}>{v.tipo_metrica_nome} (#{v.id})</SelectItem>)}
                      </SelectContent>
                    </Select>
                    {vinculosDaIndustria.length === 0 && <p className="text-xs text-amber-600 mt-1">Nenhum vínculo desse tipo cadastrado pra {campanhaDetalhe.industria_nome}.</p>}
                  </div>
                  <div>
                    <Label>Vigência</Label>
                    <Select value={formRegra.vigencia_id} onValueChange={v => setFormRegra(f => ({ ...f, vigencia_id: v }))} disabled={!formRegra.vinculo_id}>
                      <SelectTrigger><SelectValue placeholder="Selecione..." /></SelectTrigger>
                      <SelectContent>
                        {(vigenciasDoVinculo ?? []).map(v => <SelectItem key={v.id} value={String(v.id)}>{v.data_inicio} – {v.data_fim} ({v.status})</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                </>
              ) : (
                <>
                  <div>
                    <Label>Códigos de produto (cod_prod), um por linha ou separados por vírgula</Label>
                    <Textarea value={formRegra.cod_prods} onChange={e => setFormRegra(f => ({ ...f, cod_prods: e.target.value }))} placeholder={'Ex: 461410\n461553'} rows={3} />
                  </div>
                  <div>
                    <Label>Quantidade mínima vendida (no período da campanha)</Label>
                    <Input type="number" value={formRegra.qtd_minima} onChange={e => setFormRegra(f => ({ ...f, qtd_minima: e.target.value }))} />
                  </div>
                </>
              )}

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label>Pontos</Label>
                  <Input type="number" value={formRegra.pontos} onChange={e => setFormRegra(f => ({ ...f, pontos: e.target.value }))} />
                </div>
                <div>
                  <Label>Bônus (R$)</Label>
                  <Input type="number" value={formRegra.valor_bonus} onChange={e => setFormRegra(f => ({ ...f, valor_bonus: e.target.value }))} />
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button variant="ghost" onClick={fecharDialogRegra}>Cancelar</Button>
              <Button onClick={() => salvarRegra.mutate()} disabled={salvarRegra.isPending}>{regraEditando ? 'Salvar alterações' : 'Criar regra'}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    )
  }

  // ─── Tela: lista de campanhas ───────────────────────────────────────────
  return (
    <div className="p-6 max-w-4xl mx-auto space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold flex items-center gap-2"><Trophy className="w-5 h-5 text-amber-500" /> Gamificação</h1>
          <p className="text-sm text-muted-foreground">MVP restrito — pontos, ranking e bônus por campanha. Não paga nada automaticamente.</p>
        </div>
        <Button onClick={() => setNovaCampanhaOpen(true)}><Plus className="w-4 h-4 mr-1" /> Nova campanha</Button>
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nome</TableHead>
              <TableHead>Indústria</TableHead>
              <TableHead>Período</TableHead>
              <TableHead>Status</TableHead>
              <TableHead></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {(campanhas ?? []).length === 0 && (
              <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Nenhuma campanha ainda</TableCell></TableRow>
            )}
            {(campanhas ?? []).map(c => (
              <TableRow key={c.id} className="cursor-pointer hover:bg-muted/50" onClick={() => setCampanhaSelecionada(c.id)}>
                <TableCell className="font-medium">{c.nome}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{c.industria_nome}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{c.data_inicio} – {c.data_fim}</TableCell>
                <TableCell><Badge variant={c.status === 'ativa' ? 'default' : 'secondary'}>{c.status}</Badge></TableCell>
                <TableCell>
                  <Button variant="ghost" size="sm" onClick={e => { e.stopPropagation(); excluirCampanha.mutate(c.id) }}>
                    <Trash2 className="w-3.5 h-3.5 text-red-500" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <Dialog open={novaCampanhaOpen} onOpenChange={setNovaCampanhaOpen}>
        <DialogContent>
          <DialogHeader><DialogTitle>Nova campanha</DialogTitle></DialogHeader>
          <div className="space-y-3">
            <div>
              <Label>Indústria</Label>
              <Select value={formCampanha.industria_id} onValueChange={v => setFormCampanha(f => ({ ...f, industria_id: v }))}>
                <SelectTrigger><SelectValue placeholder="Selecione..." /></SelectTrigger>
                <SelectContent>
                  {(industrias ?? []).map(i => <SelectItem key={i.id} value={String(i.id)}>{i.nome}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div>
              <Label>Nome da campanha</Label>
              <Input value={formCampanha.nome} onChange={e => setFormCampanha(f => ({ ...f, nome: e.target.value }))} placeholder='Ex: "Piloto Setembro — Unilever"' />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label>Início</Label>
                <Input type="date" value={formCampanha.data_inicio} onChange={e => setFormCampanha(f => ({ ...f, data_inicio: e.target.value }))} />
              </div>
              <div>
                <Label>Fim</Label>
                <Input type="date" value={formCampanha.data_fim} onChange={e => setFormCampanha(f => ({ ...f, data_fim: e.target.value }))} />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setNovaCampanhaOpen(false)}>Cancelar</Button>
            <Button onClick={() => criarCampanha.mutate()} disabled={criarCampanha.isPending}>Criar</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
