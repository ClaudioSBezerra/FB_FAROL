import { useRef, useState } from 'react'
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
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, Search, Upload, ChevronsUpDown, Check } from 'lucide-react'

// ─── Types ────────────────────────────────────────────────────────────────────

interface Gestor {
  cod_supervisor: number
  nome: string
  uf: string | null
  atuacao: string | null
}

interface RCA {
  cod_rca: number
  nome: string
  tipo: string
  ativo: boolean
  cod_supervisor: number | null
  gestor_nome: string | null
  uf: string | null
  regiao: string | null
  atuacao: string | null
}

const TIPOS = ['RCA', 'CRV', 'JUR', 'GGV', 'TELEVENDAS']

const EMPTY_FORM = {
  cod_rca: 0,
  nome: '',
  tipo: 'RCA',
  ativo: true,
  cod_supervisor: null as number | null,
}

// ─── API ──────────────────────────────────────────────────────────────────────

const api = {
  listRCAs: (params: Record<string, string>) => {
    const qs = new URLSearchParams(Object.fromEntries(Object.entries(params).filter(([, v]) => v !== ''))).toString()
    return fetch(`/api/cadastros/rcas${qs ? `?${qs}` : ''}`).then(r => r.json()) as Promise<RCA[]>
  },
  listGestores: () =>
    fetch('/api/cadastros/gestores').then(r => r.json()) as Promise<Gestor[]>,
  create: (body: typeof EMPTY_FORM) =>
    fetch('/api/cadastros/rcas', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  update: (id: number, body: typeof EMPTY_FORM) =>
    fetch(`/api/cadastros/rcas/${id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  delete: (id: number) =>
    fetch(`/api/cadastros/rcas/${id}`, { method: 'DELETE' }),
  upload: (file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return fetch('/api/cadastros/rcas/upload-csv', { method: 'POST', body: fd })
  },
}

// ─── Tipo badge ───────────────────────────────────────────────────────────────

const TIPO_COLORS: Record<string, string> = {
  CRV:       'bg-purple-100 text-purple-800',
  GGV:       'bg-blue-100 text-blue-800',
  JUR:       'bg-amber-100 text-amber-800',
  TELEVENDAS:'bg-cyan-100 text-cyan-800',
  RCA:       'bg-green-100 text-green-800',
}

function TipoBadge({ tipo }: { tipo: string }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${TIPO_COLORS[tipo] ?? 'bg-gray-100'}`}>
      {tipo}
    </span>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function CadastroRCAs() {
  const qc = useQueryClient()
  const fileRef = useRef<HTMLInputElement>(null)

  const [search, setSearch] = useState('')
  const [debouncedQ, setDebouncedQ] = useState('')
  const [filterUF, setFilterUF] = useState('')
  const [filterSup, setFilterSup] = useState('')
  const [filterAtivo, setFilterAtivo] = useState('')
  const [gestorOpen, setGestorOpen] = useState(false)
  const [formGestorOpen, setFormGestorOpen] = useState(false)

  const [editTarget, setEditTarget] = useState<RCA | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<RCA | null>(null)
  const [showDialog, setShowDialog] = useState(false)
  const [form, setForm] = useState(EMPTY_FORM)

  const [uploadResult, setUploadResult] = useState<{ importados: number; atualizados: number; ignorados: number } | null>(null)
  const [uploading, setUploading] = useState(false)

  const queryParams = {
    q: debouncedQ,
    uf: filterUF,
    cod_supervisor: filterSup,
    ativo: filterAtivo,
  }

  const { data: rcas = [], isLoading } = useQuery({
    queryKey: ['rcas', queryParams],
    queryFn: () => api.listRCAs(queryParams),
  })

  const { data: gestores = [] } = useQuery({
    queryKey: ['gestores'],
    queryFn: () => api.listGestores(),
  })

  const save = useMutation({
    mutationFn: () =>
      editTarget ? api.update(editTarget.cod_rca, form) : api.create(form),
    onSuccess: async (res) => {
      if (!res.ok) {
        const err = await res.json().catch(() => ({ error: 'Erro desconhecido' }))
        toast.error(err.error)
        return
      }
      toast.success(editTarget ? 'RCA atualizado' : 'RCA criado')
      qc.invalidateQueries({ queryKey: ['rcas'] })
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
      toast.success('RCA removido')
      qc.invalidateQueries({ queryKey: ['rcas'] })
      qc.invalidateQueries({ queryKey: ['gestores'] })
      setDeleteTarget(null)
    },
  })

  async function handleUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setUploading(true)
    setUploadResult(null)
    try {
      const res = await api.upload(file)
      const data = await res.json()
      if (!res.ok) { toast.error(data.error ?? 'Erro no upload'); return }
      setUploadResult(data)
      toast.success(`Importação concluída — ${data.importados} novos, ${data.atualizados} atualizados`)
      qc.invalidateQueries({ queryKey: ['rcas'] })
      qc.invalidateQueries({ queryKey: ['gestores'] })
    } catch {
      toast.error('Falha no upload')
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  function openCreate() {
    setEditTarget(null)
    setForm(EMPTY_FORM)
    setShowDialog(true)
  }

  function openEdit(rc: RCA) {
    setEditTarget(rc)
    setForm({
      cod_rca: rc.cod_rca,
      nome: rc.nome,
      tipo: rc.tipo,
      ativo: rc.ativo,
      cod_supervisor: rc.cod_supervisor,
    })
    setShowDialog(true)
  }

  function handleSearchChange(v: string) {
    setSearch(v)
    clearTimeout((window as any)._rcaSearchTimer)
    ;(window as any)._rcaSearchTimer = setTimeout(() => setDebouncedQ(v), 350)
  }

  // UFs únicas para filtro
  const ufsDisponiveis = [...new Set(gestores.map(g => g.uf).filter(Boolean))].sort() as string[]

  // Gestores filtrados pela UF selecionada (se houver), ordenados por nome
  const gestoresFiltrados = gestores
    .filter(g => !filterUF || g.uf === filterUF)
    .sort((a, b) => a.nome.localeCompare(b.nome))

  // Limpa filtro de gestor quando a UF muda e o gestor atual não pertence à nova UF
  function handleUFChange(v: string) {
    const uf = v === '_all' ? '' : v
    setFilterUF(uf)
    if (uf && filterSup) {
      const gestor = gestores.find(g => String(g.cod_supervisor) === filterSup)
      if (gestor && gestor.uf !== uf) setFilterSup('')
    }
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">RCAs</h1>
          <p className="text-sm text-muted-foreground">Representantes Comerciais Autônomos</p>
        </div>
        <div className="flex gap-2">
          <input ref={fileRef} type="file" accept=".csv,.txt" className="hidden" onChange={handleUpload} />
          <Button variant="outline" size="sm" onClick={() => fileRef.current?.click()} disabled={uploading}>
            <Upload className="w-4 h-4 mr-1" />
            {uploading ? 'Importando...' : 'Importar CSV'}
          </Button>
          <Button onClick={openCreate} size="sm">
            <Plus className="w-4 h-4 mr-1" /> Novo RCA
          </Button>
        </div>
      </div>

      {/* Resultado do upload */}
      {uploadResult && (
        <div className="flex gap-4 p-3 bg-green-50 border border-green-200 rounded-lg text-sm">
          <span>✅ <strong>{uploadResult.importados}</strong> novos</span>
          <span>🔄 <strong>{uploadResult.atualizados}</strong> atualizados</span>
          {uploadResult.ignorados > 0 && <span>⚠️ <strong>{uploadResult.ignorados}</strong> ignorados</span>}
          <button className="ml-auto text-muted-foreground hover:text-foreground" onClick={() => setUploadResult(null)}>✕</button>
        </div>
      )}

      {/* Filtros */}
      <div className="flex flex-wrap gap-3">
        <div className="relative">
          <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
          <Input
            className="pl-8 w-64"
            placeholder="Buscar por nome ou código..."
            value={search}
            onChange={e => handleSearchChange(e.target.value)}
          />
        </div>
        <Select value={filterUF || '_all'} onValueChange={handleUFChange}>
          <SelectTrigger className="w-28"><SelectValue placeholder="UF" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="_all">Todas UFs</SelectItem>
            {ufsDisponiveis.map(uf => <SelectItem key={uf} value={uf}>{uf}</SelectItem>)}
          </SelectContent>
        </Select>
        <Popover open={gestorOpen} onOpenChange={setGestorOpen}>
          <PopoverTrigger asChild>
            <Button variant="outline" role="combobox" aria-expanded={gestorOpen} className="w-72 justify-between font-normal">
              {filterSup
                ? (() => { const g = gestores.find(g => String(g.cod_supervisor) === filterSup); return g ? `${g.cod_supervisor} – ${g.nome}` : 'Gestor' })()
                : 'Todos os gestores'}
              <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-80 p-0" align="start">
            <Command>
              <CommandInput placeholder="Buscar por nome ou código..." />
              <CommandList>
                <CommandEmpty>Nenhum gestor encontrado.</CommandEmpty>
                <CommandGroup>
                  <CommandItem value="_all" onSelect={() => { setFilterSup(''); setGestorOpen(false) }}>
                    <Check className={`mr-2 h-4 w-4 ${!filterSup ? 'opacity-100' : 'opacity-0'}`} />
                    Todos os gestores
                  </CommandItem>
                  {gestoresFiltrados.map(g => (
                    <CommandItem
                      key={g.cod_supervisor}
                      value={`${g.cod_supervisor} ${g.nome}`}
                      onSelect={() => { setFilterSup(String(g.cod_supervisor)); setGestorOpen(false) }}
                    >
                      <Check className={`mr-2 h-4 w-4 ${filterSup === String(g.cod_supervisor) ? 'opacity-100' : 'opacity-0'}`} />
                      <span className="font-mono text-xs text-muted-foreground w-10 shrink-0">{g.cod_supervisor}</span>
                      <span className="truncate">{g.nome}</span>
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
        <Select value={filterAtivo || '_all'} onValueChange={v => setFilterAtivo(v === '_all' ? '' : v)}>
          <SelectTrigger className="w-28"><SelectValue placeholder="Status" /></SelectTrigger>
          <SelectContent>
            <SelectItem value="_all">Todos</SelectItem>
            <SelectItem value="true">Ativos</SelectItem>
            <SelectItem value="false">Inativos</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-24">Cód.</TableHead>
              <TableHead>Nome</TableHead>
              <TableHead className="w-28">Tipo</TableHead>
              <TableHead>Gestor / Atuação</TableHead>
              <TableHead className="w-20 text-center">Status</TableHead>
              <TableHead className="w-20" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading && (
              <TableRow><TableCell colSpan={7} className="text-center py-8 text-muted-foreground">Carregando...</TableCell></TableRow>
            )}
            {!isLoading && rcas.length === 0 && (
              <TableRow><TableCell colSpan={7} className="text-center py-8 text-muted-foreground">Nenhum RCA encontrado</TableCell></TableRow>
            )}
            {rcas.map(rc => (
              <TableRow key={rc.cod_rca}>
                <TableCell className="font-mono text-sm">{rc.cod_rca}</TableCell>
                <TableCell className="font-medium">{rc.nome}</TableCell>
                <TableCell><TipoBadge tipo={rc.tipo} /></TableCell>
                <TableCell>
                  {rc.cod_supervisor != null
                    ? <span className="text-sm">
                        <span className="font-mono text-xs text-muted-foreground mr-1.5">{rc.cod_supervisor}</span>
                        {rc.gestor_nome ?? '—'}
                      </span>
                    : <span className="text-muted-foreground">—</span>}
                </TableCell>
                <TableCell className="text-center">
                  <Badge variant={rc.ativo ? 'default' : 'secondary'}>{rc.ativo ? 'Ativo' : 'Inativo'}</Badge>
                </TableCell>
                <TableCell>
                  <div className="flex gap-1 justify-end">
                    <Button variant="ghost" size="icon" onClick={() => openEdit(rc)}>
                      <Pencil className="w-4 h-4" />
                    </Button>
                    <Button variant="ghost" size="icon" className="text-destructive" onClick={() => setDeleteTarget(rc)}>
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
            <DialogTitle>{editTarget ? 'Editar RCA' : 'Novo RCA'}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Label>Código RCA</Label>
                <Input
                  type="number"
                  value={form.cod_rca || ''}
                  onChange={e => setForm(f => ({ ...f, cod_rca: Number(e.target.value) }))}
                  disabled={!!editTarget}
                  placeholder="Ex: 4623"
                />
              </div>
            </div>
            <div className="space-y-1">
              <Label>Nome</Label>
              <Input
                value={form.nome}
                onChange={e => setForm(f => ({ ...f, nome: e.target.value }))}
                placeholder="Nome do RCA"
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1">
                <Label>Tipo</Label>
                <Select value={form.tipo} onValueChange={v => setForm(f => ({ ...f, tipo: v }))}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {TIPOS.map(t => <SelectItem key={t} value={t}>{t}</SelectItem>)}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label>Gestor</Label>
                <Popover open={formGestorOpen} onOpenChange={setFormGestorOpen}>
                  <PopoverTrigger asChild>
                    <Button variant="outline" role="combobox" className="w-full justify-between font-normal truncate">
                      {form.cod_supervisor != null
                        ? (() => { const g = gestores.find(g => g.cod_supervisor === form.cod_supervisor); return g ? `${g.cod_supervisor} – ${g.nome}` : 'Selecionar...' })()
                        : 'Selecionar...'}
                      <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-80 p-0" align="start">
                    <Command>
                      <CommandInput placeholder="Código ou nome do gestor..." />
                      <CommandList>
                        <CommandEmpty>Nenhum gestor encontrado.</CommandEmpty>
                        <CommandGroup>
                          <CommandItem value="_none" onSelect={() => { setForm(f => ({ ...f, cod_supervisor: null })); setFormGestorOpen(false) }}>
                            <Check className={`mr-2 h-4 w-4 ${form.cod_supervisor == null ? 'opacity-100' : 'opacity-0'}`} />
                            Sem gestor
                          </CommandItem>
                          {[...gestores].sort((a, b) => a.nome.localeCompare(b.nome)).map(g => (
                            <CommandItem
                              key={g.cod_supervisor}
                              value={`${g.cod_supervisor} ${g.nome}`}
                              onSelect={() => { setForm(f => ({ ...f, cod_supervisor: g.cod_supervisor })); setFormGestorOpen(false) }}
                            >
                              <Check className={`mr-2 h-4 w-4 ${form.cod_supervisor === g.cod_supervisor ? 'opacity-100' : 'opacity-0'}`} />
                              <span className="font-mono text-xs text-muted-foreground w-10 shrink-0">{g.cod_supervisor}</span>
                              <span className="truncate">{g.nome}</span>
                            </CommandItem>
                          ))}
                        </CommandGroup>
                      </CommandList>
                    </Command>
                  </PopoverContent>
                </Popover>
              </div>
            </div>
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="ativo-rca"
                checked={form.ativo}
                onChange={e => setForm(f => ({ ...f, ativo: e.target.checked }))}
                className="h-4 w-4"
              />
              <Label htmlFor="ativo-rca">Ativo</Label>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowDialog(false)}>Cancelar</Button>
            <Button onClick={() => save.mutate()} disabled={!form.nome || !form.cod_rca || save.isPending}>
              {save.isPending ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmar exclusão */}
      <AlertDialog open={!!deleteTarget} onOpenChange={v => !v && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remover RCA?</AlertDialogTitle>
            <AlertDialogDescription>
              <strong>{deleteTarget?.nome}</strong> (cód. {deleteTarget?.cod_rca}). Esta ação não pode ser desfeita.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground"
              disabled={remove.isPending}
              onClick={() => deleteTarget && remove.mutate(deleteTarget.cod_rca)}
            >
              Remover
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
