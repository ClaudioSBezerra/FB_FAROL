import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ShieldCheck, UserPlus, Layers, Trash2 } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface SpUsuario {
  id: string
  email: string
  full_name: string
  sp_role: string
  tipo_persona: string
  cod_referencia: string
  is_verified: boolean
  trial_ends_at: string
  created_at: string
  environment_id: string
  environment_name: string
  group_id: string
  group_name: string
  company_id: string
  company_name: string
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

const ROLE_LABELS: Record<string, string> = {
  admin_fbtax:     'Admin FbTax',
  gestor_geral:    'Gestor Geral',
  gestor_filial:   'Gestor',
  somente_leitura: 'Somente Leitura',
}

const ROLE_COLORS: Record<string, string> = {
  admin_fbtax:     'bg-red-100 text-red-800',
  gestor_geral:    'bg-blue-100 text-blue-800',
  gestor_filial:   'bg-green-100 text-green-800',
  somente_leitura: 'bg-gray-100 text-gray-600',
}

function RoleBadge({ role }: { role: string }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${ROLE_COLORS[role] ?? 'bg-gray-100'}`}>
      {ROLE_LABELS[role] ?? role}
    </span>
  )
}

const PERSONAS: { value: string; label: string }[] = [
  { value: 'diretor',           label: 'Diretor' },
  { value: 'gerente_geral',     label: 'Gerente Geral' },
  { value: 'ggv',               label: 'GGV' },
  { value: 'supervisor',        label: 'Supervisor' },
  { value: 'rca',               label: 'RCA' },
  { value: 'ti',                label: 'TI' },
  { value: 'analista_negocios', label: 'Analista de Negócios' },
  { value: 'admin',             label: 'Admin' },
]

const PERSONA_LABEL: Record<string, string> = Object.fromEntries(PERSONAS.map(p => [p.value, p.label]))

const PERSONA_COLOR: Record<string, string> = {
  diretor:           'bg-purple-100 text-purple-800',
  gerente_geral:     'bg-blue-100 text-blue-800',
  ggv:               'bg-indigo-100 text-indigo-800',
  supervisor:        'bg-cyan-100 text-cyan-800',
  rca:               'bg-teal-100 text-teal-800',
  ti:                'bg-orange-100 text-orange-800',
  analista_negocios: 'bg-yellow-100 text-yellow-800',
  admin:             'bg-red-100 text-red-800',
}

function PersonaBadge({ persona }: { persona: string }) {
  if (!persona) return <span className="text-xs text-muted-foreground">—</span>
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${PERSONA_COLOR[persona] ?? 'bg-gray-100'}`}>
      {PERSONA_LABEL[persona] ?? persona}
    </span>
  )
}

function personaToRole(persona: string): string {
  if (['diretor', 'gerente_geral', 'ti', 'admin'].includes(persona)) return 'gestor_geral'
  if (['ggv', 'supervisor'].includes(persona)) return 'gestor_filial'
  return 'somente_leitura'
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SpUsuarios() {
  const { token } = useAuth()
  const qc = useQueryClient()

  // ── State ────────────────────────────────────────────────────────────────────
  const [roleDialog,   setRoleDialog]   = useState(false)
  const [novoDialog,   setNovoDialog]   = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<SpUsuario | null>(null)
  const [selected,     setSelected]     = useState<SpUsuario | null>(null)
  const [newRole,      setNewRole]      = useState('')
  const [editTrialDate, setEditTrialDate] = useState('')
  const [editNome,     setEditNome]     = useState('')
  const [editPersona,  setEditPersona]  = useState('')
  const [editCodRef,   setEditCodRef]   = useState('')
  const [showReassign, setShowReassign] = useState(false)

  // Campos do novo usuário
  const [novoNome,      setNovoNome]      = useState('')
  const [novoEmail,     setNovoEmail]     = useState('')
  const [novaSenha,     setNovaSenha]     = useState('')
  const [novoSpRole,    setNovoSpRole]    = useState('somente_leitura')
  const [novoPersona,   setNovoPersona]   = useState('')
  const [novoCodRef,    setNovoCodRef]    = useState('')
  const [novoTrialDate, setNovoTrialDate] = useState('2099-12-31')

  // Hierarquia do novo usuário
  const [createEnvId,     setCreateEnvId]     = useState('')
  const [createGroupId,   setCreateGroupId]   = useState('')
  const [createCompanyId, setCreateCompanyId] = useState('')
  const [environments,    setEnvironments]    = useState<{id: string; name: string}[]>([])
  const [groups,          setGroups]          = useState<{id: string; name: string}[]>([])
  const [companies,       setCompanies]       = useState<{id: string; name: string}[]>([])

  // Reassign (dialog perfil)
  const [reassignEnvId,     setReassignEnvId]     = useState('')
  const [reassignGroupId,   setReassignGroupId]   = useState('')
  const [reassignCompanyId, setReassignCompanyId] = useState('')
  const [reassignGroups,    setReassignGroups]    = useState<{id: string; name: string}[]>([])
  const [reassignCompanies, setReassignCompanies] = useState<{id: string; name: string}[]>([])

  // Vínculos multi-empresa (sem filiais)
  const [vinculosDialog,      setVinculosDialog]      = useState(false)
  const [selectedForVinculos, setSelectedForVinculos] = useState<SpUsuario | null>(null)
  const [activeComps,         setActiveComps]         = useState<Set<string>>(new Set())
  const [availableComps,      setAvailableComps]      = useState<{id: string; name: string; cnpj: string}[]>([])
  const [loadingVinculos,     setLoadingVinculos]     = useState(false)

  // ── Hierarchy fetches ────────────────────────────────────────────────────────
  useEffect(() => {
    if (!token) return
    fetch('/api/config/environments', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.ok ? r.json() : [])
      .then(d => setEnvironments(d || []))
      .catch(() => setEnvironments([]))
  }, [token])

  useEffect(() => {
    if (!createEnvId) { setGroups([]); setCreateGroupId(''); return }
    fetch(`/api/config/groups?environment_id=${createEnvId}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.ok ? r.json() : [])
      .then(d => setGroups(d || []))
      .catch(() => setGroups([]))
  }, [createEnvId, token])

  useEffect(() => {
    if (!createGroupId) { setCompanies([]); setCreateCompanyId(''); return }
    fetch(`/api/config/companies?group_id=${createGroupId}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.ok ? r.json() : [])
      .then(d => setCompanies(d || []))
      .catch(() => setCompanies([]))
  }, [createGroupId, token])

  useEffect(() => {
    if (!reassignEnvId) { setReassignGroups([]); setReassignGroupId(''); return }
    fetch(`/api/config/groups?environment_id=${reassignEnvId}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.ok ? r.json() : [])
      .then(d => setReassignGroups(d || []))
      .catch(() => setReassignGroups([]))
  }, [reassignEnvId, token])

  useEffect(() => {
    if (!reassignGroupId) { setReassignCompanies([]); setReassignCompanyId(''); return }
    fetch(`/api/config/companies?group_id=${reassignGroupId}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.ok ? r.json() : [])
      .then(d => setReassignCompanies(d || []))
      .catch(() => setReassignCompanies([]))
  }, [reassignGroupId, token])

  // ── Queries ──────────────────────────────────────────────────────────────────
  const { data: usuarios = [], isLoading } = useQuery<SpUsuario[]>({
    queryKey: ['sp-usuarios'],
    queryFn: async () => {
      const res = await fetch('/api/sp/usuarios', { headers: { Authorization: `Bearer ${token}` } })
      if (!res.ok) throw new Error('Erro ao carregar usuários')
      return res.json()
    },
  })

  // ── Mutations ────────────────────────────────────────────────────────────────
  const updateRole = useMutation({
    mutationFn: async ({ id, sp_role, full_name, tipo_persona, cod_referencia, environment_id, group_id, company_id, trial_ends_at }:
      { id: string; sp_role: string; full_name: string; tipo_persona?: string; cod_referencia?: string; environment_id?: string; group_id?: string; company_id?: string; trial_ends_at?: string }) => {
      const res = await fetch(`/api/sp/usuarios/${id}/role`, {
        method:  'PUT',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body:    JSON.stringify({ sp_role, full_name, tipo_persona, cod_referencia, environment_id, group_id, company_id, trial_ends_at }),
      })
      if (!res.ok) throw new Error((await res.text()) || 'Erro ao atualizar perfil')
    },
    onSuccess: () => {
      toast.success('Perfil atualizado')
      qc.invalidateQueries({ queryKey: ['sp-usuarios'] })
      setRoleDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const criarUsuario = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/sp/usuarios', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({
          full_name:      novoNome,
          email:          novoEmail,
          password:       novaSenha,
          sp_role:        novoSpRole,
          tipo_persona:   novoPersona,
          cod_referencia: novoCodRef,
          trial_ends_at:  novoTrialDate,
          all_filiais:    true,
          filial_ids:     [],
          ...(createEnvId     && { environment_id: createEnvId }),
          ...(createGroupId   && { group_id: createGroupId }),
          ...(createCompanyId && { company_id: createCompanyId }),
        }),
      })
      if (!res.ok) throw new Error((await res.text()) || 'Erro ao criar usuário')
    },
    onSuccess: () => {
      toast.success('Usuário criado com sucesso')
      qc.invalidateQueries({ queryKey: ['sp-usuarios'] })
      setNovoDialog(false)
      setNovoNome(''); setNovoEmail(''); setNovaSenha('')
      setNovoSpRole('somente_leitura'); setNovoPersona(''); setNovoCodRef('')
      setNovoTrialDate('2099-12-31')
      setCreateEnvId(''); setCreateGroupId(''); setCreateCompanyId('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const deletarUsuario = useMutation({
    mutationFn: async (id: string) => {
      const res = await fetch(`/api/sp/usuarios/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error((await res.text()) || 'Erro ao excluir usuário')
    },
    onSuccess: () => {
      toast.success('Usuário excluído com sucesso')
      qc.invalidateQueries({ queryKey: ['sp-usuarios'] })
      setDeleteTarget(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const saveVinculos = useMutation({
    mutationFn: async () => {
      if (!selectedForVinculos) return
      const payload = availableComps.map(c => ({
        empresa_id:  c.id,
        all_filiais: activeComps.has(c.id),
        filial_ids:  [],
      }))
      const res = await fetch(`/api/sp/usuarios/${selectedForVinculos.id}/vinculos`, {
        method:  'PUT',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body:    JSON.stringify(payload),
      })
      if (!res.ok) throw new Error((await res.json()).error ?? 'Erro ao salvar vínculos')
    },
    onSuccess: () => {
      toast.success('Empresas atualizadas')
      qc.invalidateQueries({ queryKey: ['sp-usuarios'] })
      setVinculosDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // ── Handlers ─────────────────────────────────────────────────────────────────
  function openRoleDialog(u: SpUsuario) {
    setSelected(u)
    setNewRole(u.sp_role)
    setEditNome(u.full_name)
    setEditPersona(u.tipo_persona || '')
    setEditCodRef(u.cod_referencia || '')
    setEditTrialDate(u.trial_ends_at ? u.trial_ends_at.slice(0, 10) : '')
    setShowReassign(false)
    setReassignEnvId(''); setReassignGroupId(''); setReassignCompanyId('')
    setRoleDialog(true)
  }

  async function openVinculosDialog(u: SpUsuario) {
    setSelectedForVinculos(u)
    setActiveComps(new Set())
    setAvailableComps([])
    setVinculosDialog(true)
    setLoadingVinculos(true)
    try {
      const [compsRes, vinRes] = await Promise.all([
        fetch('/api/user/companies',                          { headers: { Authorization: `Bearer ${token}` } }),
        fetch(`/api/sp/usuarios/${u.id}/vinculos`,            { headers: { Authorization: `Bearer ${token}` } }),
      ])
      const comps: {id: string; name: string; cnpj: string}[] = await compsRes.json()
      const vinculos: {empresa_id: string; all_filiais: boolean}[] = await vinRes.json()
      setAvailableComps(comps || [])
      const active = new Set(vinculos.filter(v => v.all_filiais || v.empresa_id).map(v => v.empresa_id))
      setActiveComps(active)
    } catch {
      toast.error('Erro ao carregar empresas')
    } finally {
      setLoadingVinculos(false)
    }
  }

  // ── Render ───────────────────────────────────────────────────────────────────
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Usuários Farol</h2>
        <Button size="sm" onClick={() => setNovoDialog(true)}>
          <UserPlus className="h-4 w-4 mr-1" /> Novo Usuário
        </Button>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Carregando...</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nome</TableHead>
              <TableHead>E-mail</TableHead>
              <TableHead>Empresa</TableHead>
              <TableHead>Persona</TableHead>
              <TableHead>Perfil</TableHead>
              <TableHead className="w-32">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {usuarios.map(u => (
              <TableRow key={u.id}>
                <TableCell className="font-medium">{u.full_name}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{u.email}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{u.company_name || '—'}</TableCell>
                <TableCell><PersonaBadge persona={u.tipo_persona} /></TableCell>
                <TableCell><RoleBadge role={u.sp_role} /></TableCell>
                <TableCell>
                  <div className="flex gap-1">
                    <Button size="sm" variant="outline" onClick={() => openRoleDialog(u)}>
                      <ShieldCheck className="h-3.5 w-3.5 mr-1" />
                      Perfil
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => openVinculosDialog(u)}>
                      <Layers className="h-3.5 w-3.5 mr-1" />
                      Empresas
                    </Button>
                    <Button
                      size="sm" variant="outline"
                      className="text-red-600 border-red-200 hover:bg-red-50"
                      onClick={() => setDeleteTarget(u)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {usuarios.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground text-sm py-8">
                  Nenhum usuário encontrado.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      )}

      {/* ── Dialog: novo usuário ──────────────────────────────────────────── */}
      <Dialog open={novoDialog} onOpenChange={setNovoDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Novo Usuário Farol</DialogTitle>
            <DialogDescription className="sr-only">Preencha os dados para criar um novo usuário.</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <div className="grid gap-1.5">
              <Label>Nome completo</Label>
              <Input value={novoNome} onChange={e => setNovoNome(e.target.value)} placeholder="João Silva" />
            </div>
            <div className="grid gap-1.5">
              <Label>E-mail</Label>
              <Input type="email" value={novoEmail} onChange={e => setNovoEmail(e.target.value)} placeholder="joao@empresa.com" />
            </div>
            <div className="grid gap-1.5">
              <Label>Senha inicial</Label>
              <Input type="password" value={novaSenha} onChange={e => setNovaSenha(e.target.value)} placeholder="Mínimo 6 caracteres" />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label>Persona</Label>
                <Select value={novoPersona} onValueChange={v => { setNovoPersona(v); setNovoSpRole(personaToRole(v)) }}>
                  <SelectTrigger><SelectValue placeholder="Selecione..." /></SelectTrigger>
                  <SelectContent>
                    {PERSONAS.map(p => <SelectItem key={p.value} value={p.value}>{p.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label>Perfil</Label>
                <Select value={novoSpRole} onValueChange={setNovoSpRole}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="admin_fbtax">Admin FbTax</SelectItem>
                    <SelectItem value="gestor_geral">Gestor Geral</SelectItem>
                    <SelectItem value="gestor_filial">Gestor</SelectItem>
                    <SelectItem value="somente_leitura">Somente Leitura</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label>Código de referência <span className="text-muted-foreground text-xs">(cod_supervisor, cod_rca, etc.)</span></Label>
              <Input value={novoCodRef} onChange={e => setNovoCodRef(e.target.value)} placeholder="ex: 001, 042" />
            </div>
            <div className="grid gap-1.5">
              <Label>Validade da licença</Label>
              <Input type="date" value={novoTrialDate} onChange={e => setNovoTrialDate(e.target.value)} />
            </div>

            <div className="border-t pt-3 space-y-2">
              <Label className="text-sm font-semibold">Hierarquia</Label>
              <div className="grid gap-1.5">
                <Label className="text-xs text-muted-foreground">Ambiente</Label>
                <Select value={createEnvId} onValueChange={v => { setCreateEnvId(v); setCreateGroupId(''); setCreateCompanyId('') }}>
                  <SelectTrigger><SelectValue placeholder="Selecione o ambiente..." /></SelectTrigger>
                  <SelectContent>
                    {environments.filter(e => e.id).map(e => <SelectItem key={e.id} value={e.id}>{e.name}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs text-muted-foreground">Grupo</Label>
                <Select value={createGroupId} onValueChange={v => { setCreateGroupId(v); setCreateCompanyId('') }} disabled={!createEnvId}>
                  <SelectTrigger><SelectValue placeholder={createEnvId ? 'Selecione o grupo...' : 'Selecione um ambiente primeiro'} /></SelectTrigger>
                  <SelectContent>
                    {groups.filter(g => g.id).map(g => <SelectItem key={g.id} value={g.id}>{g.name}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label className="text-xs text-muted-foreground">Empresa</Label>
                <Select value={createCompanyId} onValueChange={setCreateCompanyId} disabled={!createGroupId}>
                  <SelectTrigger><SelectValue placeholder={createGroupId ? 'Selecione a empresa...' : 'Selecione um grupo primeiro'} /></SelectTrigger>
                  <SelectContent>
                    {companies.filter(c => c.id).map(c => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setNovoDialog(false)}>Cancelar</Button>
            <Button
              disabled={criarUsuario.isPending || !novoNome || !novoEmail || !novaSenha}
              onClick={() => criarUsuario.mutate()}
            >
              {criarUsuario.isPending ? 'Criando...' : 'Criar Usuário'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Dialog: alterar perfil ─────────────────────────────────────────── */}
      <Dialog open={roleDialog} onOpenChange={setRoleDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Alterar Perfil</DialogTitle>
            <DialogDescription className="sr-only">Edite o perfil, licença e hierarquia do usuário.</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-2 max-h-[70vh] overflow-y-auto pr-1">
            <div className="grid gap-1.5">
              <Label>Nome completo</Label>
              <Input value={editNome} onChange={e => setEditNome(e.target.value)} />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label>Persona</Label>
                <Select value={editPersona} onValueChange={v => { setEditPersona(v); setNewRole(personaToRole(v)) }}>
                  <SelectTrigger><SelectValue placeholder="Selecione..." /></SelectTrigger>
                  <SelectContent>
                    {PERSONAS.map(p => <SelectItem key={p.value} value={p.value}>{p.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-1.5">
                <Label>Perfil</Label>
                <Select value={newRole} onValueChange={setNewRole}>
                  <SelectTrigger><SelectValue placeholder="Selecione o perfil" /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="admin_fbtax">Admin FbTax</SelectItem>
                    <SelectItem value="gestor_geral">Gestor Geral</SelectItem>
                    <SelectItem value="gestor_filial">Gestor</SelectItem>
                    <SelectItem value="somente_leitura">Somente Leitura</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid gap-1.5">
              <Label>Código de referência</Label>
              <Input value={editCodRef} onChange={e => setEditCodRef(e.target.value)} placeholder="cod_supervisor, cod_rca, etc." />
            </div>
            <div className="grid gap-1.5">
              <Label>Vencimento da licença</Label>
              <Input type="date" value={editTrialDate} onChange={e => setEditTrialDate(e.target.value)} />
              <p className="text-[10px] text-muted-foreground">Renovar não apaga dados — só estende o prazo de acesso.</p>
            </div>

            <div className="border-t pt-3 space-y-2">
              <div className="flex items-center justify-between">
                <Label className="text-sm font-semibold">Hierarquia</Label>
                <Button variant="outline" size="sm" className="h-7 text-xs"
                  onClick={() => { setShowReassign(!showReassign); setReassignEnvId(''); setReassignGroupId(''); setReassignCompanyId('') }}>
                  {showReassign ? 'Cancelar' : 'Alterar Hierarquia'}
                </Button>
              </div>
              <div className="text-xs text-muted-foreground space-y-0.5 bg-muted/30 rounded p-2">
                <div>Ambiente: <strong>{selected?.environment_name || '—'}</strong></div>
                <div className="ml-2">Grupo: <strong>{selected?.group_name || '—'}</strong></div>
                <div className="ml-4">Empresa: <strong>{selected?.company_name || '—'}</strong></div>
              </div>
              {showReassign && (
                <div className="space-y-2 border rounded p-3 bg-muted/10">
                  <Label className="text-xs font-medium">Nova Hierarquia</Label>
                  <div className="grid gap-1.5">
                    <Label className="text-xs text-muted-foreground">Ambiente</Label>
                    <Select value={reassignEnvId} onValueChange={v => { setReassignEnvId(v); setReassignGroupId(''); setReassignCompanyId('') }}>
                      <SelectTrigger><SelectValue placeholder="Selecione o ambiente..." /></SelectTrigger>
                      <SelectContent>
                        {environments.filter(e => e.id).map(e => <SelectItem key={e.id} value={e.id}>{e.name}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-1.5">
                    <Label className="text-xs text-muted-foreground">Grupo</Label>
                    <Select value={reassignGroupId} onValueChange={v => { setReassignGroupId(v); setReassignCompanyId('') }} disabled={!reassignEnvId}>
                      <SelectTrigger><SelectValue placeholder={reassignEnvId ? 'Selecione o grupo...' : 'Selecione um ambiente primeiro'} /></SelectTrigger>
                      <SelectContent>
                        {reassignGroups.filter(g => g.id).map(g => <SelectItem key={g.id} value={g.id}>{g.name}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-1.5">
                    <Label className="text-xs text-muted-foreground">Empresa</Label>
                    <Select value={reassignCompanyId} onValueChange={setReassignCompanyId} disabled={!reassignGroupId}>
                      <SelectTrigger><SelectValue placeholder={reassignGroupId ? 'Selecione a empresa...' : 'Selecione um grupo primeiro'} /></SelectTrigger>
                      <SelectContent>
                        {reassignCompanies.filter(c => c.id).map(c => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setRoleDialog(false)}>Cancelar</Button>
            <Button
              disabled={updateRole.isPending || !newRole || !editNome}
              onClick={() => selected && updateRole.mutate({
                id: selected.id, sp_role: newRole, full_name: editNome,
                tipo_persona: editPersona,
                cod_referencia: editCodRef,
                ...(editTrialDate && editTrialDate !== (selected.trial_ends_at?.slice(0, 10) ?? '') ? { trial_ends_at: editTrialDate } : {}),
                ...(showReassign && reassignEnvId ? { environment_id: reassignEnvId, group_id: reassignGroupId, company_id: reassignCompanyId } : {}),
              })}
            >
              {updateRole.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Dialog: vínculos multi-empresa ────────────────────────────────── */}
      <Dialog open={vinculosDialog} onOpenChange={setVinculosDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Empresas — {selectedForVinculos?.full_name}</DialogTitle>
            <DialogDescription className="sr-only">Gerencie o acesso deste usuário às empresas.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-2 max-h-[55vh] overflow-y-auto pr-1">
            {loadingVinculos ? (
              <p className="text-sm text-muted-foreground">Carregando...</p>
            ) : availableComps.length === 0 ? (
              <p className="text-sm text-muted-foreground">Nenhuma empresa disponível.</p>
            ) : (
              availableComps.map(comp => (
                <div key={comp.id} className="flex items-center gap-2 border rounded p-3">
                  <Checkbox
                    id={`comp-${comp.id}`}
                    checked={activeComps.has(comp.id)}
                    onCheckedChange={checked => {
                      setActiveComps(prev => {
                        const next = new Set(prev)
                        checked ? next.add(comp.id) : next.delete(comp.id)
                        return next
                      })
                    }}
                  />
                  <Label htmlFor={`comp-${comp.id}`} className="font-medium text-sm cursor-pointer">{comp.name}</Label>
                </div>
              ))
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setVinculosDialog(false)}>Cancelar</Button>
            <Button disabled={saveVinculos.isPending || loadingVinculos} onClick={() => saveVinculos.mutate()}>
              {saveVinculos.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Dialog: excluir usuário ──────────────────────────────────────── */}
      <Dialog open={!!deleteTarget} onOpenChange={v => { if (!v) setDeleteTarget(null) }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <Trash2 className="h-5 w-5" />
              Excluir usuário
            </DialogTitle>
            <DialogDescription>Esta ação é irreversível.</DialogDescription>
          </DialogHeader>
          <div className="py-2 text-sm">
            Deseja excluir o usuário <strong>{deleteTarget?.full_name}</strong>?
            <p className="text-xs text-muted-foreground mt-1">{deleteTarget?.email}</p>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>Cancelar</Button>
            <Button
              variant="destructive"
              disabled={deletarUsuario.isPending}
              onClick={() => deleteTarget && deletarUsuario.mutate(deleteTarget.id)}
            >
              {deletarUsuario.isPending ? 'Excluindo...' : 'Excluir'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
