import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { ShieldCheck, Building2, UserPlus, Layers, Trash2 } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface SpUsuario {
  id: string
  email: string
  full_name: string
  sp_role: string
  is_verified: boolean
  trial_ends_at: string
  created_at: string
  environment_id: string
  environment_name: string
  group_id: string
  group_name: string
  company_id: string
  company_name: string
  all_filiais: boolean
  filial_ids: number[]
}

interface Filial {
  id: number
  nome: string
  cod_filial: string
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

const ROLE_LABELS: Record<string, string> = {
  admin_fbtax:     'Admin FbTax',
  gestor_geral:    'Gestor Geral',
  gestor_filial:   'Gestor de Filial',
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

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SpUsuarios() {
  const { token } = useAuth()
  const qc = useQueryClient()

  // ── State ────────────────────────────────────────────────────────────────────
  const [roleDialog,    setRoleDialog]    = useState(false)
  const [filiaisDialog, setFiliaisDialog] = useState(false)
  const [novoDialog,    setNovoDialog]    = useState(false)
  const [deleteTarget,  setDeleteTarget]  = useState<SpUsuario | null>(null)
  const [selected,      setSelected]      = useState<SpUsuario | null>(null)
  const [newRole,       setNewRole]       = useState('')
  const [editTrialDate, setEditTrialDate] = useState('')
  const [allFiliais,    setAllFiliais]    = useState(false)
  const [chosenFiliais, setChosenFiliais] = useState<number[]>([])

  // Campos do novo usuário
  const [novoNome,       setNovoNome]       = useState('')
  const [novoEmail,      setNovoEmail]      = useState('')
  const [novaSenha,      setNovaSenha]      = useState('')
  const [novoSpRole,     setNovoSpRole]     = useState('somente_leitura')
  const [novoTrialDate,  setNovoTrialDate]  = useState('2099-12-31')
  const [novoAllFiliais, setNovoAllFiliais] = useState(false)
  const [novoFiliais,    setNovoFiliais]    = useState<number[]>([])

  // Hierarquia do novo usuário
  const [createEnvId,     setCreateEnvId]     = useState('')
  const [createGroupId,   setCreateGroupId]   = useState('')
  const [createCompanyId, setCreateCompanyId] = useState('')
  const [environments,    setEnvironments]    = useState<{id: string; name: string}[]>([])
  const [groups,          setGroups]          = useState<{id: string; name: string}[]>([])
  const [companies,       setCompanies]       = useState<{id: string; name: string}[]>([])

  // Campo de edição de nome (dialog de perfil)
  const [editNome, setEditNome] = useState('')

  // Reatribuição de hierarquia no dialog de Perfil
  const [showReassign,       setShowReassign]       = useState(false)
  const [reassignEnvId,      setReassignEnvId]      = useState('')
  const [reassignGroupId,    setReassignGroupId]    = useState('')
  const [reassignCompanyId,  setReassignCompanyId]  = useState('')
  const [reassignGroups,     setReassignGroups]     = useState<{id: string; name: string}[]>([])
  const [reassignCompanies,  setReassignCompanies]  = useState<{id: string; name: string}[]>([])

  // ── Estado do dialog multi-empresa ───────────────────────────────────────────
  interface VinculoState { all: boolean; filiais: number[] }
  const [vinculosDialog,      setVinculosDialog]      = useState(false)
  const [selectedForVinculos, setSelectedForVinculos] = useState<SpUsuario | null>(null)
  const [vinculoMap,          setVinculoMap]          = useState<Record<string, VinculoState>>({})
  const [availableComps,      setAvailableComps]      = useState<{id: string; name: string; cnpj: string}[]>([])
  const [companyFiliais,      setCompanyFiliais]      = useState<Record<string, Filial[]>>({})
  const [loadingVinculos,     setLoadingVinculos]     = useState(false)

  // ── Hierarchy fetch ──────────────────────────────────────────────────────────
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

  // Hierarquia do reassign (dialog Perfil)
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
      const res = await fetch('/api/sp/usuarios', {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error('Erro ao carregar usuários')
      return res.json()
    },
  })

  // filiais para o dialog de vincular filiais de usuários existentes (empresa ativa do admin)
  const { data: filiais = [] } = useQuery<Filial[]>({
    queryKey: ['filiais'],
    queryFn: async () => {
      const res = await fetch('/api/filiais', {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (!res.ok) throw new Error('Erro ao carregar filiais')
      return res.json()
    },
  })

  // filiais para o form de criação: usa a empresa-alvo quando selecionada,
  // evitando vincular filiais da empresa errada ao criar usuário para outra empresa
  const { data: filiaisCreate = [] } = useQuery<Filial[]>({
    queryKey: ['filiais-empresa-create', createCompanyId],
    queryFn: async () => {
      const url = createCompanyId
        ? `/api/sp/filiais-empresa?empresa_id=${createCompanyId}`
        : '/api/filiais'
      const res = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
      if (!res.ok) return []
      return res.json()
    },
  })

  // ── Mutations ────────────────────────────────────────────────────────────────
  const updateRole = useMutation({
    mutationFn: async ({ id, sp_role, full_name, environment_id, group_id, company_id, trial_ends_at }:
      { id: string; sp_role: string; full_name: string; environment_id?: string; group_id?: string; company_id?: string; trial_ends_at?: string }) => {
      const res = await fetch(`/api/sp/usuarios/${id}/role`, {
        method:  'PUT',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body:    JSON.stringify({ sp_role, full_name, environment_id, group_id, company_id, trial_ends_at }),
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
          full_name: novoNome,
          email: novoEmail,
          password: novaSenha,
          sp_role: novoSpRole,
          trial_ends_at: novoTrialDate,
          all_filiais: novoAllFiliais,
          filial_ids: novoAllFiliais ? [] : novoFiliais,
          ...(createEnvId     && { environment_id: createEnvId }),
          ...(createGroupId   && { group_id: createGroupId }),
          ...(createCompanyId && { company_id: createCompanyId }),
        }),
      })
      if (!res.ok) {
        const txt = await res.text()
        throw new Error(txt || 'Erro ao criar usuário')
      }
    },
    onSuccess: () => {
      toast.success('Usuário criado com sucesso')
      qc.invalidateQueries({ queryKey: ['sp-usuarios'] })
      setNovoDialog(false)
      setNovoNome(''); setNovoEmail(''); setNovaSenha('')
      setNovoSpRole('somente_leitura'); setNovoTrialDate('2099-12-31')
      setNovoAllFiliais(false); setNovoFiliais([])
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

  const updateFiliais = useMutation({
    mutationFn: async ({ id, all_filiais, filial_ids }: { id: string; all_filiais: boolean; filial_ids: number[] }) => {
      const res = await fetch(`/api/sp/usuarios/${id}/filiais`, {
        method:  'PUT',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body:    JSON.stringify({ all_filiais, filial_ids }),
      })
      if (!res.ok) throw new Error((await res.json()).error ?? 'Erro ao vincular filiais')
    },
    onSuccess: () => {
      toast.success('Filiais atualizadas')
      qc.invalidateQueries({ queryKey: ['sp-usuarios'] })
      setFiliaisDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const saveVinculos = useMutation({
    mutationFn: async () => {
      if (!selectedForVinculos) return
      const payload = availableComps.map(c => {
        const v = vinculoMap[c.id]
        if (!v) return { empresa_id: c.id, all_filiais: false, filial_ids: [] }
        return { empresa_id: c.id, all_filiais: v.all, filial_ids: v.all ? [] : v.filiais }
      })
      const res = await fetch(`/api/sp/usuarios/${selectedForVinculos.id}/vinculos`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify(payload),
      })
      if (!res.ok) throw new Error((await res.json()).error ?? 'Erro ao salvar vínculos')
    },
    onSuccess: () => {
      toast.success('Vínculos atualizados')
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
    setEditTrialDate(u.trial_ends_at ? u.trial_ends_at.slice(0, 10) : '')
    setShowReassign(false)
    setReassignEnvId('')
    setReassignGroupId('')
    setReassignCompanyId('')
    setRoleDialog(true)
  }

  async function openVinculosDialog(u: SpUsuario) {
    setSelectedForVinculos(u)
    setVinculoMap({})
    setAvailableComps([])
    setCompanyFiliais({})
    setVinculosDialog(true)
    setLoadingVinculos(true)
    try {
      const [compsRes, vinRes] = await Promise.all([
        fetch('/api/user/companies', { headers: { Authorization: `Bearer ${token}` } }),
        fetch(`/api/sp/usuarios/${u.id}/vinculos`, { headers: { Authorization: `Bearer ${token}` } }),
      ])
      const comps: {id: string; name: string; cnpj: string}[] = await compsRes.json()
      const vinculos: {empresa_id: string; all_filiais: boolean; filial_ids: number[]}[] = await vinRes.json()
      setAvailableComps(comps || [])
      const map: Record<string, VinculoState> = {}
      for (const v of (vinculos || [])) {
        map[v.empresa_id] = { all: v.all_filiais, filiais: v.filial_ids || [] }
      }
      setVinculoMap(map)
    } catch {
      toast.error('Erro ao carregar empresas')
    } finally {
      setLoadingVinculos(false)
    }
  }

  async function loadCompanyFiliais(empresaId: string) {
    if (companyFiliais[empresaId] !== undefined) return
    setCompanyFiliais(prev => ({ ...prev, [empresaId]: [] })) // mark as loading
    try {
      const res = await fetch(`/api/sp/filiais-empresa?empresa_id=${empresaId}`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      const data: Filial[] = await res.json()
      setCompanyFiliais(prev => ({ ...prev, [empresaId]: data || [] }))
    } catch {
      setCompanyFiliais(prev => ({ ...prev, [empresaId]: [] }))
    }
  }

  function openFiliaisDialog(u: SpUsuario) {
    setSelected(u)
    setAllFiliais(u.all_filiais)
    setChosenFiliais(u.filial_ids ?? [])
    setFiliaisDialog(true)
  }

  function toggleFilial(id: number) {
    setChosenFiliais(prev =>
      prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]
    )
  }

  function toggleNovoFilial(id: number) {
    setNovoFiliais(prev => prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id])
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
              <TableHead>Perfil Farol</TableHead>
              <TableHead>Filiais</TableHead>
              <TableHead className="w-40">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {usuarios.map(u => (
              <TableRow key={u.id}>
                <TableCell className="font-medium">{u.full_name}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{u.email}</TableCell>
                <TableCell><RoleBadge role={u.sp_role} /></TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {u.all_filiais
                    ? <Badge variant="outline">Todas</Badge>
                    : u.filial_ids?.length
                      ? <span>{u.filial_ids.length} filial(is)</span>
                      : <span className="text-gray-400">—</span>
                  }
                </TableCell>
                <TableCell>
                  <div className="flex gap-1">
                    <Button size="sm" variant="outline" onClick={() => openRoleDialog(u)}>
                      <ShieldCheck className="h-3.5 w-3.5 mr-1" />
                      Perfil
                    </Button>
                    <Button size="sm" variant="outline" onClick={() => openFiliaisDialog(u)}>
                      <Building2 className="h-3.5 w-3.5 mr-1" />
                      Filiais
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
                <TableCell colSpan={5} className="text-center text-muted-foreground text-sm py-8">
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
            <DialogDescription className="sr-only">Preencha os dados para criar um novo usuário no sistema.</DialogDescription>
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
            <div className="grid gap-1.5">
              <Label>Perfil Farol</Label>
              <Select value={novoSpRole} onValueChange={setNovoSpRole}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="admin_fbtax">Admin FbTax</SelectItem>
                  <SelectItem value="gestor_geral">Gestor Geral</SelectItem>
                  <SelectItem value="gestor_filial">Gestor de Filial</SelectItem>
                  <SelectItem value="somente_leitura">Somente Leitura</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label>Validade (data)</Label>
              <Input type="date" value={novoTrialDate} onChange={e => setNovoTrialDate(e.target.value)} />
            </div>

            {/* Hierarquia */}
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
                <Select value={createCompanyId} onValueChange={v => { setCreateCompanyId(v); setNovoFiliais([]) }} disabled={!createGroupId}>
                  <SelectTrigger><SelectValue placeholder={createGroupId ? 'Selecione a empresa...' : 'Selecione um grupo primeiro'} /></SelectTrigger>
                  <SelectContent>
                    {companies.filter(c => c.id).map(c => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <Checkbox id="novo-all" checked={novoAllFiliais} onCheckedChange={v => setNovoAllFiliais(!!v)} />
                <Label htmlFor="novo-all">Acesso a todas as filiais</Label>
              </div>
              {!novoAllFiliais && (
                <div className="space-y-1.5 max-h-40 overflow-y-auto border rounded p-2">
                  {!createCompanyId && (
                    <p className="text-xs text-muted-foreground italic">Selecione a empresa acima para ver as filiais disponíveis.</p>
                  )}
                  {createCompanyId && filiaisCreate.length === 0 && (
                    <p className="text-xs text-muted-foreground">Nenhuma filial cadastrada para esta empresa.</p>
                  )}
                  {filiaisCreate.map(f => (
                    <div key={f.id} className="flex items-center gap-2">
                      <Checkbox id={`nf-${f.id}`} checked={novoFiliais.includes(f.id)} onCheckedChange={() => toggleNovoFilial(f.id)} />
                      <Label htmlFor={`nf-${f.id}`} className="text-sm cursor-pointer">
                        {f.nome} <span className="text-muted-foreground">({f.cod_filial})</span>
                      </Label>
                    </div>
                  ))}
                </div>
              )}
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
            <DialogTitle>Alterar Perfil Farol</DialogTitle>
            <DialogDescription className="sr-only">Edite o perfil, licença e hierarquia do usuário.</DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-2 max-h-[70vh] overflow-y-auto pr-1">
            <div className="grid gap-1.5">
              <Label>Nome completo</Label>
              <Input value={editNome} onChange={e => setEditNome(e.target.value)} placeholder="Nome do usuário" />
            </div>
            <div className="grid gap-1.5">
              <Label>Perfil Farol</Label>
              <Select value={newRole} onValueChange={setNewRole}>
                <SelectTrigger>
                  <SelectValue placeholder="Selecione o perfil" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="admin_fbtax">Admin FbTax</SelectItem>
                  <SelectItem value="gestor_geral">Gestor Geral</SelectItem>
                  <SelectItem value="gestor_filial">Gestor de Filial</SelectItem>
                  <SelectItem value="somente_leitura">Somente Leitura</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="grid gap-1.5">
              <Label>Vencimento da licença</Label>
              <Input
                type="date"
                value={editTrialDate}
                onChange={e => setEditTrialDate(e.target.value)}
              />
              <p className="text-[10px] text-muted-foreground">
                Renovar a licença não apaga dados — só estende o prazo de acesso do usuário.
              </p>
            </div>

            {/* Hierarquia atual */}
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
                id: selected.id,
                sp_role: newRole,
                full_name: editNome,
                ...(editTrialDate && editTrialDate !== (selected.trial_ends_at?.slice(0, 10) ?? '') ? { trial_ends_at: editTrialDate } : {}),
                ...(showReassign && reassignEnvId ? {
                  environment_id: reassignEnvId,
                  group_id: reassignGroupId,
                  company_id: reassignCompanyId,
                } : {}),
              })}
            >
              {updateRole.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Dialog: vínculos multi-empresa ────────────────────────────────── */}
      <Dialog open={vinculosDialog} onOpenChange={setVinculosDialog}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Empresas — {selectedForVinculos?.full_name}</DialogTitle>
            <DialogDescription className="sr-only">Gerencie o acesso deste usuário às empresas e suas filiais.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2 py-2 max-h-[55vh] overflow-y-auto pr-1">
            {loadingVinculos ? (
              <p className="text-sm text-muted-foreground">Carregando...</p>
            ) : availableComps.length === 0 ? (
              <p className="text-sm text-muted-foreground">Nenhuma empresa disponível.</p>
            ) : (
              availableComps.map(comp => {
                const v = vinculoMap[comp.id]
                const isActive = !!v
                return (
                  <div key={comp.id} className="border rounded p-3 space-y-2">
                    <div className="flex items-center gap-2">
                      <Checkbox
                        id={`comp-${comp.id}`}
                        checked={isActive}
                        onCheckedChange={checked => {
                          if (checked) {
                            setVinculoMap(prev => ({ ...prev, [comp.id]: { all: true, filiais: [] } }))
                          } else {
                            setVinculoMap(prev => { const n = { ...prev }; delete n[comp.id]; return n })
                          }
                        }}
                      />
                      <Label htmlFor={`comp-${comp.id}`} className="font-medium text-sm cursor-pointer">{comp.name}</Label>
                    </div>
                    {isActive && (
                      <div className="ml-6 space-y-2">
                        <div className="flex items-center gap-2">
                          <Checkbox
                            id={`all-${comp.id}`}
                            checked={v.all}
                            onCheckedChange={checked => {
                              setVinculoMap(prev => ({ ...prev, [comp.id]: { ...prev[comp.id], all: !!checked } }))
                              if (!checked) loadCompanyFiliais(comp.id)
                            }}
                          />
                          <Label htmlFor={`all-${comp.id}`} className="text-sm cursor-pointer">Todas as filiais</Label>
                        </div>
                        {!v.all && (
                          <div className="space-y-1 max-h-36 overflow-y-auto border rounded p-2">
                            {companyFiliais[comp.id] === undefined ? (
                              <p className="text-xs text-muted-foreground">Carregando filiais...</p>
                            ) : companyFiliais[comp.id].length === 0 ? (
                              <p className="text-xs text-muted-foreground">Nenhuma filial cadastrada.</p>
                            ) : (
                              companyFiliais[comp.id].map(f => (
                                <div key={f.id} className="flex items-center gap-2">
                                  <Checkbox
                                    id={`vf-${comp.id}-${f.id}`}
                                    checked={v.filiais.includes(f.id)}
                                    onCheckedChange={() =>
                                      setVinculoMap(prev => {
                                        const cur = prev[comp.id].filiais
                                        const next = cur.includes(f.id) ? cur.filter(x => x !== f.id) : [...cur, f.id]
                                        return { ...prev, [comp.id]: { ...prev[comp.id], filiais: next } }
                                      })
                                    }
                                  />
                                  <Label htmlFor={`vf-${comp.id}-${f.id}`} className="text-xs cursor-pointer">
                                    {f.nome} <span className="text-muted-foreground">({f.cod_filial})</span>
                                  </Label>
                                </div>
                              ))
                            )}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setVinculosDialog(false)}>Cancelar</Button>
            <Button
              disabled={saveVinculos.isPending || loadingVinculos}
              onClick={() => saveVinculos.mutate()}
            >
              {saveVinculos.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* ── Dialog: vincular filiais ───────────────────────────────────────── */}
      <Dialog open={filiaisDialog} onOpenChange={setFiliaisDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Acesso às Filiais</DialogTitle>
            <DialogDescription className="sr-only">Defina quais filiais este usuário pode acessar.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <p className="text-sm text-muted-foreground">{selected?.full_name}</p>

            <div className="flex items-center gap-2">
              <Checkbox
                id="all-filiais"
                checked={allFiliais}
                onCheckedChange={(v) => setAllFiliais(!!v)}
              />
              <Label htmlFor="all-filiais">Acesso a todas as filiais</Label>
            </div>

            {!allFiliais && (
              <div className="space-y-2 max-h-52 overflow-y-auto border rounded p-2">
                {filiais.length === 0 && (
                  <p className="text-xs text-muted-foreground">Nenhuma filial cadastrada ainda.</p>
                )}
                {filiais.map(f => (
                  <div key={f.id} className="flex items-center gap-2">
                    <Checkbox
                      id={`f-${f.id}`}
                      checked={chosenFiliais.includes(f.id)}
                      onCheckedChange={() => toggleFilial(f.id)}
                    />
                    <Label htmlFor={`f-${f.id}`} className="text-sm cursor-pointer">
                      {f.nome} <span className="text-muted-foreground">({f.cod_filial})</span>
                    </Label>
                  </div>
                ))}
              </div>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setFiliaisDialog(false)}>Cancelar</Button>
            <Button
              disabled={updateFiliais.isPending}
              onClick={() => selected && updateFiliais.mutate({
                id: selected.id,
                all_filiais: allFiliais,
                filial_ids: allFiliais ? [] : chosenFiliais,
              })}
            >
              {updateFiliais.isPending ? 'Salvando...' : 'Salvar'}
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
            <DialogDescription>
              Esta ação é irreversível. Todos os vínculos de filial e ambiente serão removidos.
            </DialogDescription>
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
