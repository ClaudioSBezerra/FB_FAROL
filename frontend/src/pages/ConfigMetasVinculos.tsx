import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel,
  AlertDialogContent, AlertDialogDescription, AlertDialogFooter,
  AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2 } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface ParametroSchema {
  key: string
  label: string
  type: string
}

interface Industria { id: number; nome: string }

interface TipoMetrica {
  id: number
  nome: string
  nivel_agregacao: string
  parametros_schema: ParametroSchema[]
}

interface MetaVinculo {
  id: number
  industria_id: number
  industria_nome: string
  tipo_metrica_id: number
  tipo_metrica_nome: string
  parametros_schema: ParametroSchema[]
  parametros_valores: Record<string, unknown>
  ativo: boolean
}

const EMPTY_FORM = {
  industria_id: '',
  tipo_metrica_id: '',
  ativo: true,
  parametros_valores: {} as Record<string, string>,
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ConfigMetasVinculos() {
  const { token } = useAuth()
  const qc = useQueryClient()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  const [editTarget, setEditTarget] = useState<MetaVinculo | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<MetaVinculo | null>(null)
  const [showDialog, setShowDialog] = useState(false)
  const [form, setForm] = useState(EMPTY_FORM)

  const { data: vinculos = [], isLoading } = useQuery<MetaVinculo[]>({
    queryKey: ['farol-metas-vinculos'],
    queryFn: async () => {
      const r = await fetch('/api/farol/metas-vinculos', { headers })
      if (!r.ok) throw new Error('Erro ao carregar vínculos')
      return r.json()
    },
  })

  const { data: industrias = [] } = useQuery<Industria[]>({
    queryKey: ['farol-industrias'],
    queryFn: async () => {
      const r = await fetch('/api/farol/industrias', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const { data: tiposMetrica = [] } = useQuery<TipoMetrica[]>({
    queryKey: ['farol-tipos-metrica'],
    queryFn: async () => {
      const r = await fetch('/api/farol/tipos-metrica', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const tipoSelecionado = tiposMetrica.find(t => String(t.id) === form.tipo_metrica_id)

  const save = useMutation({
    mutationFn: async () => {
      const url = editTarget ? `/api/farol/metas-vinculos/${editTarget.id}` : '/api/farol/metas-vinculos'
      const parametrosValores: Record<string, unknown> = {}
      for (const [k, v] of Object.entries(form.parametros_valores)) {
        const schemaParam = tipoSelecionado?.parametros_schema.find(p => p.key === k)
        parametrosValores[k] = schemaParam?.type === 'number' || schemaParam?.type === 'integer' ? Number(v) : v
      }
      const r = await fetch(url, {
        method: editTarget ? 'PUT' : 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({
          industria_id: Number(form.industria_id),
          tipo_metrica_id: Number(form.tipo_metrica_id),
          ativo: form.ativo,
          parametros_valores: parametrosValores,
        }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao salvar vínculo')
    },
    onSuccess: () => {
      toast.success(editTarget ? 'Vínculo atualizado' : 'Vínculo criado')
      qc.invalidateQueries({ queryKey: ['farol-metas-vinculos'] })
      setShowDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const remove = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/farol/metas-vinculos/${id}`, { method: 'DELETE', headers })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao remover vínculo')
    },
    onSuccess: () => {
      toast.success('Vínculo removido')
      qc.invalidateQueries({ queryKey: ['farol-metas-vinculos'] })
      setDeleteTarget(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  function openCreate() {
    setEditTarget(null)
    setForm(EMPTY_FORM)
    setShowDialog(true)
  }

  function openEdit(v: MetaVinculo) {
    setEditTarget(v)
    const valores: Record<string, string> = {}
    for (const p of v.parametros_schema) {
      valores[p.key] = String(v.parametros_valores[p.key] ?? '')
    }
    setForm({
      industria_id: String(v.industria_id),
      tipo_metrica_id: String(v.tipo_metrica_id),
      ativo: v.ativo,
      parametros_valores: valores,
    })
    setShowDialog(true)
  }

  function onTipoChange(tipoId: string) {
    const tipo = tiposMetrica.find(t => String(t.id) === tipoId)
    const valores: Record<string, string> = {}
    for (const p of tipo?.parametros_schema ?? []) {
      valores[p.key] = ''
    }
    setForm(f => ({ ...f, tipo_metrica_id: tipoId, parametros_valores: valores }))
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Configuração de Metas por Indústria</h1>
          <p className="text-sm text-muted-foreground">
            Vincula uma Indústria a um Tipo de Métrica, com os valores de parâmetro específicos dela (ex: limiar de Cobertura).
            Valores de meta por faixa/vigência ficam em outra tela.
          </p>
        </div>
        <Button onClick={openCreate} size="sm" disabled={industrias.length === 0 || tiposMetrica.length === 0}>
          <Plus className="w-4 h-4 mr-1" /> Novo Vínculo
        </Button>
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Indústria</TableHead>
              <TableHead>Tipo de Métrica</TableHead>
              <TableHead>Parâmetros</TableHead>
              <TableHead className="w-20 text-center">Status</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Carregando...</TableCell></TableRow>
            )}
            {!isLoading && vinculos.length === 0 && (
              <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Nenhum vínculo cadastrado</TableCell></TableRow>
            )}
            {vinculos.map(v => (
              <TableRow key={v.id}>
                <TableCell className="font-medium">{v.industria_nome}</TableCell>
                <TableCell>{v.tipo_metrica_nome}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {v.parametros_schema.map(p => (
                      <span key={p.key} className="inline-flex items-center gap-1 font-mono text-xs bg-slate-100 px-1.5 py-0.5 rounded">
                        {p.label}: {String(v.parametros_valores[p.key] ?? '—')}
                      </span>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-center">
                  <Badge variant={v.ativo ? 'default' : 'secondary'}>{v.ativo ? 'Ativo' : 'Inativo'}</Badge>
                </TableCell>
                <TableCell>
                  <div className="flex gap-1 justify-end">
                    <Button variant="ghost" size="icon" onClick={() => openEdit(v)}>
                      <Pencil className="w-4 h-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="text-destructive" onClick={() => setDeleteTarget(v)}>
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
            <DialogTitle>{editTarget ? 'Editar Vínculo' : 'Novo Vínculo'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1">
              <Label>Indústria</Label>
              <Select value={form.industria_id} onValueChange={v => setForm(f => ({ ...f, industria_id: v }))}>
                <SelectTrigger><SelectValue placeholder="Selecione" /></SelectTrigger>
                <SelectContent>
                  {industrias.map(i => (
                    <SelectItem key={i.id} value={String(i.id)}>{i.nome}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label>Tipo de Métrica</Label>
              <Select value={form.tipo_metrica_id} onValueChange={onTipoChange}>
                <SelectTrigger><SelectValue placeholder="Selecione" /></SelectTrigger>
                <SelectContent>
                  {tiposMetrica.map(t => (
                    <SelectItem key={t.id} value={String(t.id)}>{t.nome}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {tipoSelecionado && tipoSelecionado.parametros_schema.length > 0 && (
              <div className="space-y-2 border-t pt-3">
                <Label className="text-xs text-muted-foreground">Parâmetros de "{tipoSelecionado.nome}"</Label>
                {tipoSelecionado.parametros_schema.map(p => (
                  <div key={p.key} className="space-y-1">
                    <Label className="text-sm">{p.label}</Label>
                    <Input
                      type={p.type === 'number' || p.type === 'integer' ? 'number' : 'text'}
                      value={form.parametros_valores[p.key] ?? ''}
                      onChange={e => setForm(f => ({
                        ...f,
                        parametros_valores: { ...f.parametros_valores, [p.key]: e.target.value },
                      }))}
                    />
                  </div>
                ))}
              </div>
            )}

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="ativo-vinculo"
                checked={form.ativo}
                onChange={e => setForm(f => ({ ...f, ativo: e.target.checked }))}
                className="h-4 w-4"
              />
              <Label htmlFor="ativo-vinculo">Ativo</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDialog(false)}>Cancelar</Button>
            <Button
              onClick={() => save.mutate()}
              disabled={!form.industria_id || !form.tipo_metrica_id || save.isPending}
            >
              {save.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmar exclusão */}
      <AlertDialog open={!!deleteTarget} onOpenChange={v => !v && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remover vínculo?</AlertDialogTitle>
            <AlertDialogDescription>
              O vínculo entre <strong>{deleteTarget?.industria_nome}</strong> e <strong>{deleteTarget?.tipo_metrica_nome}</strong> será removido.
              Esta ação não pode ser desfeita.
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
