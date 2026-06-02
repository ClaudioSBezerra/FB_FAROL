import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import { UserPlus, Pencil, LayoutGrid } from 'lucide-react'
import { Separator } from '@/components/ui/separator'
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
  modulos: string[]
}

const MODULOS = [
  { value: 'vendas',    label: 'Painel Vendas' },
  { value: 'marketing', label: 'Painel Marketing' },
  { value: 'bi',        label: 'Painel BI' },
]

// ─── Constants ────────────────────────────────────────────────────────────────

const PERSONAS: { value: string; label: string }[] = [
  { value: 'ceo',               label: 'CEO' },
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
  ceo:               'bg-amber-100 text-amber-800',
  diretor:           'bg-purple-100 text-purple-800',
  gerente_geral:     'bg-blue-100 text-blue-800',
  ggv:               'bg-indigo-100 text-indigo-800',
  supervisor:        'bg-cyan-100 text-cyan-800',
  rca:               'bg-teal-100 text-teal-800',
  ti:                'bg-orange-100 text-orange-800',
  analista_negocios: 'bg-yellow-100 text-yellow-800',
  admin:             'bg-red-100 text-red-800',
}

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

function PersonaBadge({ persona }: { persona: string }) {
  if (!persona) return <span className="text-xs text-muted-foreground">—</span>
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${PERSONA_COLOR[persona] ?? 'bg-gray-100'}`}>
      {PERSONA_LABEL[persona] ?? persona}
    </span>
  )
}

function RoleBadge({ role }: { role: string }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${ROLE_COLORS[role] ?? 'bg-gray-100'}`}>
      {ROLE_LABELS[role] ?? role}
    </span>
  )
}

const MODULO_COLORS: Record<string, string> = {
  vendas:    'bg-blue-50 text-blue-700 border-blue-200',
  marketing: 'bg-pink-50 text-pink-700 border-pink-200',
  bi:        'bg-violet-50 text-violet-700 border-violet-200',
}

function ModulosBadges({ modulos }: { modulos: string[] }) {
  if (!modulos?.length) return <span className="text-xs text-muted-foreground">—</span>
  return (
    <div className="flex flex-wrap gap-1">
      {modulos.map(m => {
        const cfg = MODULOS.find(x => x.value === m)
        return (
          <span key={m} className={`inline-flex items-center px-1.5 py-0.5 rounded border text-[10px] font-medium ${MODULO_COLORS[m] ?? 'bg-gray-50 text-gray-600 border-gray-200'}`}>
            {cfg?.label ?? m}
          </span>
        )
      })}
    </div>
  )
}

// sp_role sugerido por persona (espelha PersonaToSpRole no backend)
function personaToRole(persona: string): string {
  if (['ceo', 'diretor', 'gerente_geral', 'ti', 'admin'].includes(persona)) return 'gestor_geral'
  if (['ggv', 'supervisor'].includes(persona)) return 'gestor_filial'
  return 'somente_leitura'
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function FarolUsuarios() {
  const { token, spRole } = useAuth()
  const qc = useQueryClient()
  const canAssignAdminFbtax = spRole === 'admin_fbtax'

  // ── Dialogs ──────────────────────────────────────────────────────────────────
  const [novoDialog, setNovoDialog] = useState(false)
  const [editDialog, setEditDialog] = useState(false)
  const [selected,   setSelected]   = useState<SpUsuario | null>(null)

  // Novo usuário
  const [novoNome,      setNovoNome]      = useState('')
  const [novoEmail,     setNovoEmail]     = useState('')
  const [novaSenha,     setNovaSenha]     = useState('')
  const [novoPersona,   setNovoPersona]   = useState('')
  const [novoSpRole,    setNovoSpRole]    = useState('somente_leitura')
  const [novoCodRef,    setNovoCodRef]    = useState('')
  const [novoTrialDate, setNovoTrialDate] = useState('2099-12-31')
  const [novoModulos,   setNovoModulos]   = useState<string[]>(['vendas'])

  // Editar usuário
  const [editNome,    setEditNome]    = useState('')
  const [editPersona, setEditPersona] = useState('')
  const [editRole,    setEditRole]    = useState('')
  const [editCodRef,  setEditCodRef]  = useState('')
  const [editModulos, setEditModulos] = useState<string[]>(['vendas'])

  // ── Query ────────────────────────────────────────────────────────────────────
  const { data: usuarios = [], isLoading } = useQuery<SpUsuario[]>({
    queryKey: ['farol-usuarios'],
    queryFn: async () => {
      const res = await fetch('/api/sp/usuarios', { headers: { Authorization: `Bearer ${token}` } })
      if (!res.ok) throw new Error('Erro ao carregar usuários')
      return res.json()
    },
  })

  // ── Mutations ────────────────────────────────────────────────────────────────
  const criar = useMutation({
    mutationFn: async () => {
      const res = await fetch('/api/sp/usuarios', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({
          full_name:      novoNome,
          email:          novoEmail,
          password:       novaSenha,
          tipo_persona:   novoPersona,
          sp_role:        novoSpRole,
          cod_referencia: novoCodRef,
          trial_ends_at:  novoTrialDate,
          all_filiais:    ['ceo', 'diretor', 'gerente_geral', 'ti', 'admin'].includes(novoPersona),
          modulos:        novoModulos,
        }),
      })
      if (!res.ok) throw new Error((await res.text()) || 'Erro ao criar usuário')
      return res.json()
    },
    onSuccess: () => {
      toast.success('Usuário criado com sucesso')
      qc.invalidateQueries({ queryKey: ['farol-usuarios'] })
      setNovoDialog(false)
      resetNovo()
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const atualizar = useMutation({
    mutationFn: async () => {
      if (!selected) return
      const res = await fetch(`/api/sp/usuarios/${selected.id}/role`, {
        method:  'PUT',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({
          full_name:      editNome,
          sp_role:        editRole,
          tipo_persona:   editPersona,
          cod_referencia: editCodRef,
          modulos:        editModulos,
        }),
      })
      if (!res.ok) throw new Error((await res.text()) || 'Erro ao atualizar')
    },
    onSuccess: () => {
      toast.success('Usuário atualizado')
      qc.invalidateQueries({ queryKey: ['farol-usuarios'] })
      setEditDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  // ── Helpers ──────────────────────────────────────────────────────────────────
  function resetNovo() {
    setNovoNome(''); setNovoEmail(''); setNovaSenha('')
    setNovoPersona(''); setNovoSpRole('somente_leitura')
    setNovoCodRef(''); setNovoTrialDate('2099-12-31')
    setNovoModulos(['vendas'])
  }

  function openEdit(u: SpUsuario) {
    setSelected(u)
    setEditNome(u.full_name)
    setEditPersona(u.tipo_persona)
    setEditRole(u.sp_role)
    setEditCodRef(u.cod_referencia)
    setEditModulos(u.modulos?.length > 0 ? u.modulos : ['vendas'])
    setEditDialog(true)
  }

  function toggleModulo(list: string[], mod: string): string[] {
    return list.includes(mod) ? list.filter(m => m !== mod) : [...list, mod]
  }

  function onNovoPersonaChange(persona: string) {
    setNovoPersona(persona)
    setNovoSpRole(personaToRole(persona))
  }

  function onEditPersonaChange(persona: string) {
    setEditPersona(persona)
    setEditRole(personaToRole(persona))
  }

  const roleOptions = canAssignAdminFbtax
    ? ['admin_fbtax', 'gestor_geral', 'gestor_filial', 'somente_leitura']
    : ['gestor_geral', 'gestor_filial', 'somente_leitura']

  // ── Render ───────────────────────────────────────────────────────────────────
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">Usuários</h2>
          <p className="text-sm text-muted-foreground">Gerencie os usuários do seu ambiente</p>
        </div>
        <Button size="sm" onClick={() => setNovoDialog(true)}>
          <UserPlus className="w-4 h-4 mr-1" /> Novo usuário
        </Button>
      </div>

      {/* Tabela */}
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nome</TableHead>
              <TableHead>E-mail</TableHead>
              <TableHead>Persona</TableHead>
              <TableHead>Perfil</TableHead>
              <TableHead>Módulos</TableHead>
              <TableHead>Cód. Ref.</TableHead>
              <TableHead>Licença até</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow><TableCell colSpan={8} className="text-center py-8 text-muted-foreground">Carregando...</TableCell></TableRow>
            ) : usuarios.length === 0 ? (
              <TableRow><TableCell colSpan={8} className="text-center py-8 text-muted-foreground">Nenhum usuário encontrado</TableCell></TableRow>
            ) : usuarios.map(u => (
              <TableRow key={u.id}>
                <TableCell className="font-medium">{u.full_name}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{u.email}</TableCell>
                <TableCell><PersonaBadge persona={u.tipo_persona} /></TableCell>
                <TableCell><RoleBadge role={u.sp_role} /></TableCell>
                <TableCell><ModulosBadges modulos={u.modulos} /></TableCell>
                <TableCell className="text-xs text-muted-foreground">{u.cod_referencia || '—'}</TableCell>
                <TableCell className="text-xs text-muted-foreground">
                  {u.trial_ends_at ? new Date(u.trial_ends_at).toLocaleDateString('pt-BR') : '—'}
                </TableCell>
                <TableCell>
                  <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => openEdit(u)}>
                    <Pencil className="w-3.5 h-3.5" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Dialog — novo usuário */}
      <Dialog open={novoDialog} onOpenChange={setNovoDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Novo usuário</DialogTitle>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <div className="space-y-1">
              <Label>Nome completo</Label>
              <Input value={novoNome} onChange={e => setNovoNome(e.target.value)} placeholder="João da Silva" />
            </div>
            <div className="space-y-1">
              <Label>E-mail</Label>
              <Input type="email" value={novoEmail} onChange={e => setNovoEmail(e.target.value)} placeholder="joao@empresa.com" />
            </div>
            <div className="space-y-1">
              <Label>Senha</Label>
              <Input type="password" value={novaSenha} onChange={e => setNovaSenha(e.target.value)} placeholder="Mínimo 8 caracteres" />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Label>Persona</Label>
                <Select value={novoPersona} onValueChange={onNovoPersonaChange}>
                  <SelectTrigger><SelectValue placeholder="Selecione" /></SelectTrigger>
                  <SelectContent>
                    {PERSONAS.map(p => <SelectItem key={p.value} value={p.value}>{p.label}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label>Perfil de acesso</Label>
                <Select value={novoSpRole} onValueChange={setNovoSpRole}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {roleOptions.map(r => <SelectItem key={r} value={r}>{ROLE_LABELS[r]}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="space-y-1">
              <Label>Código de referência <span className="text-muted-foreground text-xs">(opcional — cod_supervisor, cod_rca, etc.)</span></Label>
              <Input value={novoCodRef} onChange={e => setNovoCodRef(e.target.value)} placeholder="ex: 001, 042" />
            </div>
            <div className="pt-1">
              <Separator className="mb-3" />
              <div className="flex items-center gap-1.5 mb-2">
                <LayoutGrid className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">Módulos de acesso</span>
              </div>
              <div className="flex flex-wrap gap-4">
                {MODULOS.map(m => (
                  <div key={m.value} className="flex items-center gap-2">
                    <Checkbox
                      id={`novo-mod-${m.value}`}
                      checked={novoModulos.includes(m.value)}
                      onCheckedChange={() => setNovoModulos(toggleModulo(novoModulos, m.value))}
                    />
                    <label htmlFor={`novo-mod-${m.value}`} className="text-sm cursor-pointer select-none">{m.label}</label>
                  </div>
                ))}
              </div>
            </div>
            <div className="space-y-1">
              <Label>Licença até</Label>
              <Input type="date" value={novoTrialDate} onChange={e => setNovoTrialDate(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => { setNovoDialog(false); resetNovo() }}>Cancelar</Button>
            <Button onClick={() => criar.mutate()} disabled={criar.isPending}>
              {criar.isPending ? 'Criando...' : 'Criar usuário'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Dialog — editar usuário */}
      <Dialog open={editDialog} onOpenChange={setEditDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Editar usuário</DialogTitle>
          </DialogHeader>
          {selected && (
            <div className="space-y-3 py-2">
              <div className="text-sm text-muted-foreground">{selected.email}</div>
              <div className="space-y-1">
                <Label>Nome completo</Label>
                <Input value={editNome} onChange={e => setEditNome(e.target.value)} />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <Label>Persona</Label>
                  <Select value={editPersona} onValueChange={onEditPersonaChange}>
                    <SelectTrigger><SelectValue placeholder="Selecione" /></SelectTrigger>
                    <SelectContent>
                      {PERSONAS.map(p => <SelectItem key={p.value} value={p.value}>{p.label}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1">
                  <Label>Perfil de acesso</Label>
                  <Select value={editRole} onValueChange={setEditRole}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {roleOptions.map(r => <SelectItem key={r} value={r}>{ROLE_LABELS[r]}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="space-y-1">
                <Label>Código de referência</Label>
                <Input value={editCodRef} onChange={e => setEditCodRef(e.target.value)} placeholder="cod_supervisor, cod_rca, etc." />
              </div>
              <div className="pt-1">
                <Separator className="mb-3" />
                <div className="flex items-center gap-1.5 mb-2">
                  <LayoutGrid className="h-3.5 w-3.5 text-muted-foreground" />
                  <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">Módulos de acesso</span>
                </div>
                <div className="flex flex-wrap gap-4">
                  {MODULOS.map(m => (
                    <div key={m.value} className="flex items-center gap-2">
                      <Checkbox
                        id={`edit-mod-${m.value}`}
                        checked={editModulos.includes(m.value)}
                        onCheckedChange={() => setEditModulos(toggleModulo(editModulos, m.value))}
                      />
                      <label htmlFor={`edit-mod-${m.value}`} className="text-sm cursor-pointer select-none">{m.label}</label>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditDialog(false)}>Cancelar</Button>
            <Button onClick={() => atualizar.mutate()} disabled={atualizar.isPending}>
              {atualizar.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
