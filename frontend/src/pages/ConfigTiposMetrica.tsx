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
import { Plus, Pencil, Trash2, X } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

// ─── Types ────────────────────────────────────────────────────────────────────

interface ParametroSchema {
  key: string
  label: string
  type: string
}

interface TipoMetrica {
  id: number
  nome: string
  descricao?: string
  nivel_agregacao: string
  parametros_schema: ParametroSchema[]
  ativo: boolean
  created_at: string
}

const NIVEIS_AGREGACAO = [
  { value: 'ggv', label: 'GGV' },
  { value: 'crv', label: 'CRV (Supervisor)' },
  { value: 'rca', label: 'RCA' },
  { value: 'rede', label: 'Rede' },
  { value: 'cliente', label: 'Cliente / CNPJ' },
]

const TIPOS_PARAMETRO = [
  { value: 'number', label: 'Número (R$, decimal)' },
  { value: 'integer', label: 'Inteiro' },
  { value: 'text', label: 'Texto' },
]

const EMPTY_FORM = {
  nome: '',
  descricao: '',
  nivel_agregacao: 'rede',
  ativo: true,
  parametros_schema: [{ key: '', label: '', type: 'number' }] as ParametroSchema[],
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function ConfigTiposMetrica() {
  const { token } = useAuth()
  const qc = useQueryClient()
  const headers = { Authorization: `Bearer ${token}` }

  const [editTarget, setEditTarget] = useState<TipoMetrica | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<TipoMetrica | null>(null)
  const [showDialog, setShowDialog] = useState(false)
  const [form, setForm] = useState(EMPTY_FORM)

  const { data: tipos = [], isLoading } = useQuery<TipoMetrica[]>({
    queryKey: ['farol-tipos-metrica'],
    queryFn: async () => {
      const r = await fetch('/api/farol/tipos-metrica', { headers })
      if (!r.ok) throw new Error('Erro ao carregar Tipos de Métrica')
      return r.json()
    },
  })

  function buildPayload() {
    return {
      nome: form.nome,
      descricao: form.descricao,
      nivel_agregacao: form.nivel_agregacao,
      ativo: form.ativo,
      parametros_schema: form.parametros_schema.filter(p => p.key.trim() !== ''),
    }
  }

  const save = useMutation({
    mutationFn: async () => {
      const url = editTarget ? `/api/farol/tipos-metrica/${editTarget.id}` : '/api/farol/tipos-metrica'
      const r = await fetch(url, {
        method: editTarget ? 'PUT' : 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify(buildPayload()),
      })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao salvar Tipo de Métrica')
    },
    onSuccess: () => {
      toast.success(editTarget ? 'Tipo de Métrica atualizado' : 'Tipo de Métrica criado')
      qc.invalidateQueries({ queryKey: ['farol-tipos-metrica'] })
      setShowDialog(false)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const remove = useMutation({
    mutationFn: async (id: number) => {
      const r = await fetch(`/api/farol/tipos-metrica/${id}`, { method: 'DELETE', headers })
      if (!r.ok) throw new Error((await r.text()) || 'Erro ao remover Tipo de Métrica')
    },
    onSuccess: () => {
      toast.success('Tipo de Métrica removido')
      qc.invalidateQueries({ queryKey: ['farol-tipos-metrica'] })
      setDeleteTarget(null)
    },
    onError: (e: Error) => toast.error(e.message),
  })

  function openCreate() {
    setEditTarget(null)
    setForm(EMPTY_FORM)
    setShowDialog(true)
  }

  function openEdit(t: TipoMetrica) {
    setEditTarget(t)
    setForm({
      nome: t.nome,
      descricao: t.descricao ?? '',
      nivel_agregacao: t.nivel_agregacao,
      ativo: t.ativo,
      parametros_schema: t.parametros_schema.length > 0
        ? t.parametros_schema.map(p => ({ ...p }))
        : [{ key: '', label: '', type: 'number' }],
    })
    setShowDialog(true)
  }

  function updateParametro(idx: number, field: keyof ParametroSchema, value: string) {
    setForm(f => ({
      ...f,
      parametros_schema: f.parametros_schema.map((row, i) => i === idx ? { ...row, [field]: value } : row),
    }))
  }

  function addParametroRow() {
    setForm(f => ({ ...f, parametros_schema: [...f.parametros_schema, { key: '', label: '', type: 'number' }] }))
  }

  function removeParametroRow(idx: number) {
    setForm(f => ({ ...f, parametros_schema: f.parametros_schema.filter((_, i) => i !== idx) }))
  }

  const nivelLabel = (v: string) => NIVEIS_AGREGACAO.find(n => n.value === v)?.label ?? v

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Tipos de Métrica</h1>
          <p className="text-sm text-muted-foreground">
            Catálogo de formas de cálculo reutilizáveis (ex: Cobertura por Rede, Sortimento por Rede) que uma ou mais indústrias podem
            usar em suas metas — cada uma com seus próprios parâmetros.
          </p>
        </div>
        <Button onClick={openCreate} size="sm">
          <Plus className="w-4 h-4 mr-1" /> Novo Tipo de Métrica
        </Button>
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nome</TableHead>
              <TableHead>Nível de Agregação</TableHead>
              <TableHead>Parâmetros</TableHead>
              <TableHead className="w-20 text-center">Status</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Carregando...</TableCell></TableRow>
            )}
            {!isLoading && tipos.length === 0 && (
              <TableRow><TableCell colSpan={5} className="text-center py-8 text-muted-foreground">Nenhum Tipo de Métrica cadastrado</TableCell></TableRow>
            )}
            {tipos.map(t => (
              <TableRow key={t.id}>
                <TableCell className="font-medium">{t.nome}</TableCell>
                <TableCell className="text-sm text-muted-foreground">{nivelLabel(t.nivel_agregacao)}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {t.parametros_schema.length === 0 && <span className="text-muted-foreground text-sm">—</span>}
                    {t.parametros_schema.map(p => (
                      <span key={p.key} className="inline-flex items-center gap-1 font-mono text-xs bg-slate-100 px-1.5 py-0.5 rounded" title={p.type}>
                        {p.label || p.key}
                      </span>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="text-center">
                  <Badge variant={t.ativo ? 'default' : 'secondary'}>{t.ativo ? 'Ativo' : 'Inativo'}</Badge>
                </TableCell>
                <TableCell>
                  <div className="flex gap-1 justify-end">
                    <Button variant="ghost" size="icon" onClick={() => openEdit(t)}>
                      <Pencil className="w-4 h-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="text-destructive" onClick={() => setDeleteTarget(t)}>
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
            <DialogTitle>{editTarget ? 'Editar Tipo de Métrica' : 'Novo Tipo de Métrica'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-1">
              <Label>Nome</Label>
              <Input
                value={form.nome}
                onChange={e => setForm(f => ({ ...f, nome: e.target.value }))}
                placeholder="Ex: Cobertura por Rede"
              />
            </div>
            <div className="space-y-1">
              <Label>Descrição (opcional)</Label>
              <Input
                value={form.descricao}
                onChange={e => setForm(f => ({ ...f, descricao: e.target.value }))}
                placeholder="Ex: Média de compra por loja acima de um limiar em R$"
              />
            </div>
            <div className="space-y-1">
              <Label>Nível de Agregação</Label>
              <Select value={form.nivel_agregacao} onValueChange={v => setForm(f => ({ ...f, nivel_agregacao: v }))}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  {NIVEIS_AGREGACAO.map(n => (
                    <SelectItem key={n.value} value={n.value}>{n.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-1.5">
              <Label>Parâmetros exigidos de cada instância</Label>
              <p className="text-xs text-muted-foreground">
                Qualquer forma de cálculo cabe aqui — sem precisar mudar o sistema, só descrever quais parâmetros ela exige.
              </p>
              <div className="space-y-2">
                {form.parametros_schema.map((row, idx) => (
                  <div key={idx} className="flex gap-2 items-center">
                    <Input
                      className="w-28 font-mono"
                      value={row.key}
                      onChange={e => updateParametro(idx, 'key', e.target.value)}
                      placeholder="key"
                    />
                    <Input
                      value={row.label}
                      onChange={e => updateParametro(idx, 'label', e.target.value)}
                      placeholder="Rótulo (ex: Limiar de valor médio)"
                    />
                    <Select value={row.type} onValueChange={v => updateParametro(idx, 'type', v)}>
                      <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
                      <SelectContent>
                        {TIPOS_PARAMETRO.map(tp => (
                          <SelectItem key={tp.value} value={tp.value}>{tp.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Button
                      variant="ghost" size="icon"
                      disabled={form.parametros_schema.length === 1}
                      onClick={() => removeParametroRow(idx)}
                    >
                      <X className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
              </div>
              <Button variant="outline" size="sm" onClick={addParametroRow}>
                <Plus className="w-4 h-4 mr-1" /> Adicionar parâmetro
              </Button>
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="ativo-tipo-metrica"
                checked={form.ativo}
                onChange={e => setForm(f => ({ ...f, ativo: e.target.checked }))}
                className="h-4 w-4"
              />
              <Label htmlFor="ativo-tipo-metrica">Ativo</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDialog(false)}>Cancelar</Button>
            <Button
              onClick={() => save.mutate()}
              disabled={!form.nome.trim() || form.parametros_schema.every(p => !p.key.trim()) || save.isPending}
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
            <AlertDialogTitle>Remover Tipo de Métrica?</AlertDialogTitle>
            <AlertDialogDescription>
              <strong>{deleteTarget?.nome}</strong> será removido. Se algum vínculo Indústria/Fornecedor já usa este tipo, isso pode
              quebrar a apuração — confirme antes de remover. Esta ação não pode ser desfeita.
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
