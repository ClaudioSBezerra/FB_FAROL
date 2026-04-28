import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel,
  AlertDialogContent, AlertDialogDescription, AlertDialogFooter,
  AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, Search } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────

interface Gestor {
  cod_supervisor: number
  nome: string
  uf: string | null
  regiao: string | null
  atuacao: string | null
  ativo: boolean
  qtd_rcas: number
}

const EMPTY_GESTOR: Omit<Gestor, 'qtd_rcas'> = {
  cod_supervisor: 0,
  nome: '',
  uf: null,
  regiao: null,
  atuacao: null,
  ativo: true,
}

// ─── API ──────────────────────────────────────────────────────────────────────

const api = {
  list: (q: string) =>
    fetch(`/api/cadastros/gestores${q ? `?q=${encodeURIComponent(q)}` : ''}`).then(r => r.json()) as Promise<Gestor[]>,
  create: (body: Partial<Gestor>) =>
    fetch('/api/cadastros/gestores', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  update: (id: number, body: Partial<Gestor>) =>
    fetch(`/api/cadastros/gestores/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  delete: (id: number) =>
    fetch(`/api/cadastros/gestores/${id}`, { method: 'DELETE' }),
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function CadastroGestores() {
  const qc = useQueryClient()
  const [search, setSearch] = useState('')
  const [debouncedQ, setDebouncedQ] = useState('')
  const [editTarget, setEditTarget] = useState<Gestor | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Gestor | null>(null)
  const [showDialog, setShowDialog] = useState(false)
  const [form, setForm] = useState(EMPTY_GESTOR)

  const { data: gestores = [], isLoading } = useQuery({
    queryKey: ['gestores', debouncedQ],
    queryFn: () => api.list(debouncedQ),
  })

  const save = useMutation({
    mutationFn: () =>
      editTarget
        ? api.update(editTarget.cod_supervisor, form)
        : api.create(form),
    onSuccess: async (res) => {
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Erro desconhecido' }))
        toast.error(err.error)
        return
      }
      toast.success(editTarget ? 'Gestor atualizado' : 'Gestor criado')
      qc.invalidateQueries({ queryKey: ['gestores'] })
      setShowDialog(false)
    },
  })

  const remove = useMutation({
    mutationFn: (id: number) => api.delete(id),
    onSuccess: async (res) => {
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Erro desconhecido' }))
        toast.error(err.error)
        return
      }
      toast.success('Gestor removido')
      qc.invalidateQueries({ queryKey: ['gestores'] })
      setDeleteTarget(null)
    },
  })

  function openCreate() {
    setEditTarget(null)
    setForm(EMPTY_GESTOR)
    setShowDialog(true)
  }

  function openEdit(g: Gestor) {
    setEditTarget(g)
    setForm({ cod_supervisor: g.cod_supervisor, nome: g.nome, uf: g.uf, regiao: g.regiao, atuacao: g.atuacao, ativo: g.ativo })
    setShowDialog(true)
  }

  function handleSearchChange(v: string) {
    setSearch(v)
    clearTimeout((window as any)._gestorSearchTimer)
    ;(window as any)._gestorSearchTimer = setTimeout(() => setDebouncedQ(v), 350)
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Gestores</h1>
          <p className="text-sm text-muted-foreground">Supervisores e GGVs vinculados às equipes de RCAs</p>
        </div>
        <Button onClick={openCreate} size="sm">
          <Plus className="w-4 h-4 mr-1" /> Novo Gestor
        </Button>
      </div>

      <div className="relative max-w-sm">
        <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          className="pl-8"
          placeholder="Buscar por nome ou código..."
          value={search}
          onChange={e => handleSearchChange(e.target.value)}
        />
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-24">Cód.</TableHead>
              <TableHead>Nome</TableHead>
              <TableHead className="w-16">UF</TableHead>
              <TableHead>Região</TableHead>
              <TableHead>Atuação</TableHead>
              <TableHead className="w-20 text-center">RCAs</TableHead>
              <TableHead className="w-20 text-center">Status</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow><TableCell colSpan={8} className="text-center py-8 text-muted-foreground">Carregando...</TableCell></TableRow>
            )}
            {!isLoading && gestores.length === 0 && (
              <TableRow><TableCell colSpan={8} className="text-center py-8 text-muted-foreground">Nenhum gestor encontrado</TableCell></TableRow>
            )}
            {gestores.map(g => (
              <TableRow key={g.cod_supervisor}>
                <TableCell className="font-mono text-sm">{g.cod_supervisor}</TableCell>
                <TableCell className="font-medium">{g.nome}</TableCell>
                <TableCell>{g.uf ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell>{g.regiao ?? <span className="text-muted-foreground">—</span>}</TableCell>
                <TableCell>
                  {g.atuacao
                    ? <Badge variant="outline" className="text-xs">{g.atuacao}</Badge>
                    : <span className="text-muted-foreground">—</span>}
                </TableCell>
                <TableCell className="text-center">{g.qtd_rcas}</TableCell>
                <TableCell className="text-center">
                  <Badge variant={g.ativo ? 'default' : 'secondary'}>{g.ativo ? 'Ativo' : 'Inativo'}</Badge>
                </TableCell>
                <TableCell>
                  <div className="flex gap-1 justify-end">
                    <Button variant="ghost" size="icon" onClick={() => openEdit(g)}>
                      <Pencil className="w-4 h-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="text-destructive" onClick={() => setDeleteTarget(g)}>
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      {/* Modal CRUD */}
      <Dialog open={showDialog} onOpenChange={setShowDialog}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editTarget ? 'Editar Gestor' : 'Novo Gestor'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Label>Código</Label>
                <Input
                  type="number"
                  value={form.cod_supervisor || ''}
                  onChange={e => setForm(f => ({ ...f, cod_supervisor: Number(e.target.value) }))}
                  disabled={!!editTarget}
                  placeholder="Ex: 100"
                />
              </div>
              <div className="space-y-1">
                <Label>UF</Label>
                <Input
                  value={form.uf ?? ''}
                  onChange={e => setForm(f => ({ ...f, uf: e.target.value.toUpperCase().slice(0, 2) || null }))}
                  placeholder="GO"
                  maxLength={2}
                />
              </div>
            </div>
            <div className="space-y-1">
              <Label>Nome completo (campo SUPERVISOR)</Label>
              <Input
                value={form.nome}
                onChange={e => setForm(f => ({ ...f, nome: e.target.value }))}
                placeholder="Ex: GO - CALDAS NOVAS - LILIAM PEREIRA"
              />
            </div>
            <div className="space-y-1">
              <Label>Região</Label>
              <Input
                value={form.regiao ?? ''}
                onChange={e => setForm(f => ({ ...f, regiao: e.target.value || null }))}
                placeholder="Ex: CALDAS NOVAS"
              />
            </div>
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="ativo-gestor"
                checked={form.ativo}
                onChange={e => setForm(f => ({ ...f, ativo: e.target.checked }))}
                className="h-4 w-4"
              />
              <Label htmlFor="ativo-gestor">Ativo</Label>
            </div>
            {form.uf && form.regiao && (
              <p className="text-xs text-muted-foreground">
                Atuação gerada: <strong>{form.uf} - {form.regiao}</strong>
              </p>
            )}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDialog(false)}>Cancelar</Button>
            <Button onClick={() => save.mutate()} disabled={!form.nome || !form.cod_supervisor || save.isPending}>
              {save.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmar exclusão */}
      <AlertDialog open={!!deleteTarget} onOpenChange={v => !v && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remover gestor?</AlertDialogTitle>
            <AlertDialogDescription>
              <strong>{deleteTarget?.nome}</strong> (cód. {deleteTarget?.cod_supervisor})<br />
              {(deleteTarget?.qtd_rcas ?? 0) > 0
                ? `Este gestor possui ${deleteTarget?.qtd_rcas} RCAs vinculados. Remova os vínculos antes de excluir.`
                : 'Esta ação não pode ser desfeita.'}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground"
              disabled={(deleteTarget?.qtd_rcas ?? 0) > 0 || remove.isPending}
              onClick={() => deleteTarget && remove.mutate(deleteTarget.cod_supervisor)}
            >
              Remover
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
