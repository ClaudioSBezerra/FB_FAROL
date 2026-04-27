import { useState, useMemo, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAuth } from '@/contexts/AuthContext'
import { Plus, Trash2, Pencil, Mail } from 'lucide-react'
import { toast } from 'sonner'

interface Environment { id: string; name: string }
interface Group       { id: string; name: string }
interface Company     { id: string; name: string; trade_name?: string }
interface SpFilial    { id: number; cod_filial: number; nome: string }
interface SpCD        { id: number; filial_id: number; nome: string }

interface Destinatario {
  id: number
  cd_id: number
  cd_nome?: string
  filial_nome?: string
  nome_completo: string
  cargo: string
  email: string
  ativo: boolean
  criado_em: string
  atualizado_em: string
}

export default function SpDestinatarios() {
  const { token } = useAuth()
  const qc = useQueryClient()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  // ── Hierarquia em cascata ────────────────────────────────────────────────
  const [envID,     setEnvID]     = useState('')
  const [groupID,   setGroupID]   = useState('')
  const [companyID, setCompanyID] = useState('')
  const [filialID,  setFilialID]  = useState('')
  const [cdID,      setCdID]      = useState('')

  const [editing, setEditing] = useState<Destinatario | null>(null)
  const [creating, setCreating] = useState(false)
  const [form, setForm] = useState({ nome_completo: '', cargo: '', email: '', ativo: true })

  // ── Cascata de queries ──────────────────────────────────────────────────
  const { data: environments = [] } = useQuery<Environment[]>({
    queryKey: ['environments'],
    queryFn: async () => {
      const r = await fetch('/api/config/environments', { headers })
      if (!r.ok) return []
      return r.json()
    },
  })

  const { data: groups = [] } = useQuery<Group[]>({
    queryKey: ['groups', envID],
    enabled: !!envID,
    queryFn: async () => {
      const r = await fetch(`/api/config/groups?environment_id=${envID}`, { headers })
      if (!r.ok) return []
      return r.json()
    },
  })

  const { data: companies = [] } = useQuery<Company[]>({
    queryKey: ['companies', groupID],
    enabled: !!groupID,
    queryFn: async () => {
      const r = await fetch(`/api/config/companies?group_id=${groupID}`, { headers })
      if (!r.ok) return []
      return r.json()
    },
  })

  const { data: filiais = [] } = useQuery<SpFilial[]>({
    queryKey: ['sp-filiais-empresa', companyID],
    enabled: !!companyID,
    queryFn: async () => {
      const r = await fetch(`/api/sp/filiais-empresa?empresa_id=${companyID}`, { headers })
      if (!r.ok) return []
      return r.json()
    },
  })

  // Endpoint admin: aceita empresa_id + filial_id para listar CDs de qualquer
  // empresa (o /api/sp/filiais/{id}/cds só funciona para a empresa do contexto)
  const { data: cds = [] } = useQuery<SpCD[]>({
    queryKey: ['sp-cds-empresa', companyID, filialID],
    enabled: !!filialID && !!companyID,
    queryFn: async () => {
      const r = await fetch(`/api/sp/cds-empresa?empresa_id=${companyID}&filial_id=${filialID}`, { headers })
      if (!r.ok) return []
      return r.json()
    },
  })

  // Se há empresa selecionada na cascata, filtra por ela; senão lista tudo (admin).
  // Se há CD, restringe ao CD específico.
  const { data: destinatarios = [] } = useQuery<Destinatario[]>({
    queryKey: ['sp-destinatarios', companyID, cdID],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (cdID)      params.set('cd_id', cdID)
      if (companyID) params.set('empresa_id', companyID)
      const qs = params.toString()
      const url = qs ? `/api/sp/admin/destinatarios?${qs}` : '/api/sp/admin/destinatarios'
      const r = await fetch(url, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  // ── Auto-seleção quando há apenas 1 opção ───────────────────────────────
  useEffect(() => { if (environments.length === 1 && !envID) setEnvID(environments[0].id) }, [environments, envID])
  useEffect(() => { if (groups.length === 1 && !groupID) setGroupID(groups[0].id) }, [groups, groupID])
  useEffect(() => { if (companies.length === 1 && !companyID) setCompanyID(companies[0].id) }, [companies, companyID])

  const invalidar = () => qc.invalidateQueries({ queryKey: ['sp-destinatarios'] })

  function resetForm() {
    setForm({ nome_completo: '', cargo: '', email: '', ativo: true })
    setCreating(false)
    setEditing(null)
  }

  // ── Mutações ─────────────────────────────────────────────────────────────
  const criarMut = useMutation({
    mutationFn: async () => {
      if (!cdID) throw new Error('Selecione Ambiente, Grupo, Empresa, Filial e CD antes de cadastrar')
      const r = await fetch('/api/sp/admin/destinatarios', {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ cd_id: Number(cdID), ...form }),
      })
      const data = await r.json()
      if (!r.ok) throw new Error(data.error ?? 'Erro')
      return data
    },
    onSuccess: () => { toast.success('Destinatário cadastrado'); resetForm(); invalidar() },
    onError: (e: Error) => toast.error(e.message),
  })

  const atualizarMut = useMutation({
    mutationFn: async (payload: { cd_id?: number }) => {
      if (!editing) throw new Error('id ausente')
      const r = await fetch(`/api/sp/admin/destinatarios/${editing.id}`, {
        method: 'PUT',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...form, ...payload }),
      })
      const data = await r.json()
      if (!r.ok) throw new Error(data.error ?? 'Erro')
    },
    onSuccess: () => { toast.success('Destinatário atualizado'); resetForm(); invalidar() },
    onError: (e: Error) => toast.error(e.message),
  })

  const removerMut = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/sp/admin/destinatarios/${id}`, { method: 'DELETE', headers })
      if (!r.ok) throw new Error()
    },
    onSuccess: () => { toast.success('Removido'); invalidar() },
    onError: () => toast.error('Erro ao remover'),
  })

  // Move para o CD selecionado atualmente nos filtros
  function moverParaSelecionado(d: Destinatario) {
    if (!cdID) {
      toast.error('Selecione Filial e CD acima para onde mover este destinatário.')
      return
    }
    if (Number(cdID) === d.cd_id) {
      toast.info('Destinatário já está neste CD.')
      return
    }
    setEditing(d)
    setForm({ nome_completo: d.nome_completo, cargo: d.cargo, email: d.email, ativo: d.ativo })
    atualizarMut.mutate({ cd_id: Number(cdID) })
  }

  function abrirEdicao(d: Destinatario) {
    setEditing(d)
    setForm({ nome_completo: d.nome_completo, cargo: d.cargo, email: d.email, ativo: d.ativo })
    setCreating(true)
  }

  const podeCriar = !!cdID
  const cdSelecionadoNome = cds.find(c => String(c.id) === cdID)?.nome
  const filialSelecionadaNome = filiais.find(f => String(f.id) === filialID)?.nome

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-base font-semibold flex items-center gap-2">
          <Mail className="h-4 w-4" /> Destinatários do Resumo Executivo
        </h2>
        <p className="text-xs text-muted-foreground mt-1">
          Selecione a hierarquia completa <strong>Ambiente → Grupo → Empresa → Filial → CD</strong> para cadastrar.
          A tabela mostra todos os destinatários da empresa selecionada.
        </p>
      </div>

      {/* Cascata de filtros */}
      <div className="grid grid-cols-5 gap-3">
        <div>
          <label className="text-xs font-medium mb-1 block">Ambiente</label>
          <Select value={envID} onValueChange={v => { setEnvID(v); setGroupID(''); setCompanyID(''); setFilialID(''); setCdID('') }}>
            <SelectTrigger className="h-8 text-xs"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {environments.map(e => <SelectItem key={e.id} value={e.id}>{e.name}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div>
          <label className="text-xs font-medium mb-1 block">Grupo</label>
          <Select value={groupID} onValueChange={v => { setGroupID(v); setCompanyID(''); setFilialID(''); setCdID('') }} disabled={!envID}>
            <SelectTrigger className="h-8 text-xs"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {groups.map(g => <SelectItem key={g.id} value={g.id}>{g.name}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div>
          <label className="text-xs font-medium mb-1 block">Empresa</label>
          <Select value={companyID} onValueChange={v => { setCompanyID(v); setFilialID(''); setCdID('') }} disabled={!groupID}>
            <SelectTrigger className="h-8 text-xs"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {companies.map(c => <SelectItem key={c.id} value={c.id}>{c.trade_name || c.name}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div>
          <label className="text-xs font-medium mb-1 block">Filial</label>
          <Select value={filialID} onValueChange={v => { setFilialID(v); setCdID('') }} disabled={!companyID}>
            <SelectTrigger className="h-8 text-xs"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {filiais.map(f => <SelectItem key={f.id} value={String(f.id)}>{f.nome} (cód. {f.cod_filial})</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div>
          <label className="text-xs font-medium mb-1 block">CD</label>
          <Select value={cdID} onValueChange={setCdID} disabled={!filialID}>
            <SelectTrigger className="h-8 text-xs"><SelectValue placeholder="Selecione" /></SelectTrigger>
            <SelectContent>
              {cds.map(cd => <SelectItem key={cd.id} value={String(cd.id)}>{cd.nome}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="flex justify-between items-center">
        <p className="text-[11px] text-muted-foreground">
          {cdID
            ? `Mostrando ${destinatarios.length} destinatário(s) do CD selecionado.`
            : `Mostrando ${destinatarios.length} destinatário(s) de toda a empresa.`}
        </p>
        <Button size="sm" disabled={!podeCriar} onClick={() => { resetForm(); setCreating(true) }}>
          <Plus className="h-3.5 w-3.5 mr-1" /> Novo destinatário
        </Button>
      </div>

      {!podeCriar && destinatarios.length === 0 && (
        <p className="text-xs text-amber-700 bg-amber-50 border border-amber-200 rounded p-2">
          Selecione a hierarquia completa para cadastrar um novo destinatário.
        </p>
      )}

      {/* Tabela */}
      {destinatarios.length === 0 ? (
        <div className="text-center py-12 border rounded-lg bg-muted/30">
          <Mail className="h-8 w-8 mx-auto text-muted-foreground mb-2" />
          <p className="text-sm text-muted-foreground">
            {cdID ? 'Nenhum destinatário cadastrado para este CD.' : 'Nenhum destinatário cadastrado.'}
          </p>
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="text-xs">
              <TableHead className="py-2 w-44">Filial / CD</TableHead>
              <TableHead className="py-2">Nome completo</TableHead>
              <TableHead className="py-2">Cargo</TableHead>
              <TableHead className="py-2">Email</TableHead>
              <TableHead className="py-2 w-20">Ativo</TableHead>
              <TableHead className="py-2 w-44">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {destinatarios.map(d => {
              const noMesmoCD = String(d.cd_id) === cdID
              return (
                <TableRow key={d.id} className={`text-xs ${!d.ativo ? 'opacity-50' : ''}`}>
                  <TableCell className="py-1.5">
                    <div className="text-[11px] font-medium">{d.filial_nome || '—'}</div>
                    <div className="text-[10px] text-muted-foreground">{d.cd_nome || `CD ${d.cd_id}`}</div>
                  </TableCell>
                  <TableCell className="py-1.5 font-medium">{d.nome_completo}</TableCell>
                  <TableCell className="py-1.5 text-muted-foreground">{d.cargo || '—'}</TableCell>
                  <TableCell className="py-1.5 font-mono text-[11px]">{d.email}</TableCell>
                  <TableCell className="py-1.5">
                    {d.ativo
                      ? <span className="inline-flex px-2 py-0.5 rounded bg-green-100 text-green-800 text-[10px] font-medium">Ativo</span>
                      : <span className="inline-flex px-2 py-0.5 rounded bg-gray-100 text-gray-600 text-[10px] font-medium">Inativo</span>}
                  </TableCell>
                  <TableCell className="py-1.5">
                    <div className="flex gap-1">
                      <Button size="sm" variant="outline" className="h-6 text-[10px] px-1.5" onClick={() => abrirEdicao(d)} title="Editar">
                        <Pencil className="h-3 w-3" />
                      </Button>
                      {cdID && !noMesmoCD && (
                        <Button
                          size="sm" variant="outline"
                          className="h-6 text-[10px] px-1.5 text-blue-700 border-blue-200 hover:bg-blue-50"
                          onClick={() => moverParaSelecionado(d)}
                          title={`Mover para ${cdSelecionadoNome ?? 'CD selecionado'}`}
                        >
                          → {filialSelecionadaNome?.slice(0, 8)}
                        </Button>
                      )}
                      <Button
                        size="sm" variant="outline"
                        className="h-6 text-[10px] px-1.5 text-red-600 border-red-200 hover:bg-red-50"
                        onClick={() => { if (confirm('Remover destinatário?')) removerMut.mutate(d.id) }}
                        title="Remover"
                      >
                        <Trash2 className="h-3 w-3" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}

      {/* Dialog */}
      <Dialog open={creating} onOpenChange={open => { if (!open) resetForm() }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editing ? 'Editar destinatário' : 'Novo destinatário'}</DialogTitle>
          </DialogHeader>
          {editing && (
            <p className="text-[11px] text-muted-foreground border-l-2 border-primary pl-2">
              Vinculado a <strong>{editing.filial_nome || '—'} / {editing.cd_nome || `CD ${editing.cd_id}`}</strong>.
              {cdID && Number(cdID) !== editing.cd_id && (
                <> Para mover, use o botão <em>→ Filial</em> na linha da tabela.</>
              )}
            </p>
          )}
          {!editing && cdSelecionadoNome && (
            <p className="text-[11px] text-muted-foreground border-l-2 border-primary pl-2">
              Será vinculado a <strong>{filialSelecionadaNome} / {cdSelecionadoNome}</strong>.
            </p>
          )}
          <div className="space-y-3">
            <div>
              <label className="text-xs font-medium block mb-1">Nome completo *</label>
              <Input value={form.nome_completo} onChange={e => setForm({ ...form, nome_completo: e.target.value })} />
            </div>
            <div>
              <label className="text-xs font-medium block mb-1">Cargo</label>
              <Input value={form.cargo} onChange={e => setForm({ ...form, cargo: e.target.value })} placeholder="ex: Gerente de Logística" />
            </div>
            <div>
              <label className="text-xs font-medium block mb-1">Email *</label>
              <Input value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} type="email" />
            </div>
            {editing && (
              <label className="flex items-center gap-2 text-xs cursor-pointer">
                <input type="checkbox" checked={form.ativo} onChange={e => setForm({ ...form, ativo: e.target.checked })} />
                Ativo (recebe emails)
              </label>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={resetForm}>Cancelar</Button>
            <Button
              onClick={() => editing ? atualizarMut.mutate({}) : criarMut.mutate()}
              disabled={!form.nome_completo || !form.email || criarMut.isPending || atualizarMut.isPending}
            >
              {editing ? 'Salvar' : 'Cadastrar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
