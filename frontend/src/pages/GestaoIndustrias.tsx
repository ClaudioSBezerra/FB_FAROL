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
import { Plus, Pencil, Trash2, X } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface IndustriaFornecedor {
  cod_fornec: string
  rotulo?: string
}

interface Industria {
  id: number
  nome: string
  razao_social?: string
  ativo: boolean
  created_at: string
  fornecedores: IndustriaFornecedor[]
}

const EMPTY_FORM = {
  nome: '',
  razao_social: '',
  ativo: true,
  fornecedores: [{ cod_fornec: '', rotulo: '' }] as IndustriaFornecedor[],
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function GestaoIndustrias() {
  const { token } = useAuth()
  const qc = useQueryClient()
  const headers = { Authorization: `Bearer ${token}` }

  const [editTarget, setEditTarget] = useState<Industria | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Industria | null>(null)
  const [showDialog, setShowDialog] = useState(false)
  const [form, setForm] = useState(EMPTY_FORM)

  const { data: industrias = [], isLoading } = useQuery<Industria[]>({
    queryKey: ['farol-industrias'],
    queryFn: async () => {
      const r = await fetch('/api/farol/industrias', { headers })
      if (!r.ok) throw new Error('Erro ao carregar indústrias')
      return r.json()
    },
  })

  function buildPayload() {
    return {
      nome: form.nome,
      razao_social: form.razao_social,
      ativo: form.ativo,
      fornecedores: form.fornecedores.filter(f => f.cod_fornec.trim() !== ''),
    }
  }

  const save = useMutation({
    mutationFn: async () => {
      const url = editTarget ? `/api/farol/industrias/${editTarget.id}` : '/api/farol/industrias'
      const r = await fetch(url, {
        method: editTarget ? 'PUT' : 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify(buildPayload()),
      })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao salvar indústria')
    },
    onSuccess: () => {
      toast.success(editTarget ? 'Indústria atualizada' : 'Indústria criada')
      qc.invalidateQueries({ queryKey: ['farol-industrias'] })
      setShowDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const remove = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/farol/industrias/${id}`, { method: 'DELETE', headers })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao remover indústria')
    },
    onSuccess: () => {
      toast.success('Indústria removida')
      qc.invalidateQueries({ queryKey: ['farol-industrias'] })
      setDeleteTarget(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  function openCreate() {
    setEditTarget(null)
    setForm(EMPTY_FORM)
    setShowDialog(true)
  }

  function openEdit(ind: Industria) {
    setEditTarget(ind)
    setForm({
      nome: ind.nome,
      razao_social: ind.razao_social ?? '',
      ativo: ind.ativo,
      fornecedores: ind.fornecedores.length > 0
        ? ind.fornecedores.map(f => ({ cod_fornec: f.cod_fornec, rotulo: f.rotulo ?? '' }))
        : [{ cod_fornec: '', rotulo: '' }],
    })
    setShowDialog(true)
  }

  function updateFornecedor(idx: number, field: 'cod_fornec' | 'rotulo', value: string) {
    setForm(f => ({
      ...f,
      fornecedores: f.fornecedores.map((row, i) => i === idx ? { ...row, [field]: value } : row),
    }))
  }

  function addFornecedorRow() {
    setForm(f => ({ ...f, fornecedores: [...f.fornecedores, { cod_fornec: '', rotulo: '' }] }))
  }

  function removeFornecedorRow(idx: number) {
    setForm(f => ({ ...f, fornecedores: f.fornecedores.filter((_, i) => i !== idx) }))
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Indústrias</h1>
          <p className="text-sm text-muted-foreground">
            Mapeia códigos de fornecedor (WinThor) que pertencem ao mesmo fabricante, quando o cadastro varia por filial.
          </p>
        </div>
        <Button onClick={openCreate} size="sm">
          <Plus className="w-4 h-4 mr-1" /> Nova Indústria
        </Button>
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nome</TableHead>
              <TableHead>Razão Social</TableHead>
              <TableHead>Códigos de fornecedor</TableHead>
              <TableHead className="w-20 text-center">Status</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Carregando...</TableCell></TableRow>
            )}
            {!isLoading && industrias.length === 0 && (
              <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Nenhuma indústria cadastrada</TableCell></TableRow>
            )}
            {industrias.map(ind => (
              <TableRow key={ind.id}>
                <TableCell className="font-medium">{ind.nome}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{ind.razao_social || '—'}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {ind.fornecedores.length === 0 && <span className="text-muted-foreground text-sm">—</span>}
                    {ind.fornecedores.map(f => (
                      <span key={f.cod_fornec} className="inline-flex items-center gap-1 font-mono text-xs bg-slate-100 px-1.5 py-0.5 rounded" title={f.rotulo}>
                        {f.cod_fornec}
                      </span>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-center">
                  <Badge variant={ind.ativo ? 'default' : 'secondary'}>{ind.ativo ? 'Ativo' : 'Inativo'}</Badge>
                </TableCell>
                <TableCell>
                  <div className="flex gap-1 justify-end">
                    <Button variant="ghost" size="icon" onClick={() => openEdit(ind)}>
                      <Pencil className="w-4 h-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="text-destructive" onClick={() => setDeleteTarget(ind)}>
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
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>{editTarget ? 'Editar Indústria' : 'Nova Indústria'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1">
              <Label>Nome</Label>
              <Input
                value={form.nome}
                onChange={e => setForm(f => ({ ...f, nome: e.target.value }))}
                placeholder="Ex: UNILEVER HC"
              />
            </div>
            <div className="space-y-1">
              <Label>Razão Social (opcional)</Label>
              <Input
                value={form.razao_social}
                onChange={e => setForm(f => ({ ...f, razao_social: e.target.value }))}
                placeholder="Ex: UNILEVER BRASIL LTDA-396"
              />
            </div>

            <div className="space-y-1.5">
              <Label>Códigos de fornecedor (cod_fornec)</Label>
              <div className="space-y-2">
                {form.fornecedores.map((row, idx) => (
                  <div key={idx} className="flex gap-2 items-center">
                    <Input
                      className="w-32 font-mono"
                      value={row.cod_fornec}
                      onChange={e => updateFornecedor(idx, 'cod_fornec', e.target.value)}
                      placeholder="cod_fornec"
                    />
                    <Input
                      value={row.rotulo ?? ''}
                      onChange={e => updateFornecedor(idx, 'rotulo', e.target.value)}
                      placeholder="Anotação (opcional, ex: MTZ/MS/BA)"
                    />
                    <Button
                      variant="ghost" size="icon"
                      disabled={form.fornecedores.length === 1}
                      onClick={() => removeFornecedorRow(idx)}
                    >
                      <X className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
              </div>
              <Button variant="outline" size="sm" onClick={addFornecedorRow}>
                <Plus className="w-4 h-4 mr-1" /> Adicionar código
              </Button>
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="ativo-industria"
                checked={form.ativo}
                onChange={e => setForm(f => ({ ...f, ativo: e.target.checked }))}
                className="h-4 w-4"
              />
              <Label htmlFor="ativo-industria">Ativo</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDialog(false)}>Cancelar</Button>
            <Button onClick={() => save.mutate()} disabled={!form.nome.trim() || save.isPending}>
              {save.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmar exclusão */}
      <AlertDialog open={!!deleteTarget} onOpenChange={v => !v && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remover indústria?</AlertDialogTitle>
            <AlertDialogDescription>
              <strong>{deleteTarget?.nome}</strong> e seus {deleteTarget?.fornecedores.length ?? 0} código(s) de fornecedor vinculados serão removidos. Esta ação não pode ser desfeita.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground"
              disabled={remove.isPending}
              onClick={() => deleteTarget && remove.mutate(deleteTarget.id)}
            >
              Remover
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
