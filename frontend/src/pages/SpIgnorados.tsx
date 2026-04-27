import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from 'sonner'
import { RefreshCw, RotateCcw } from 'lucide-react'
import { useAuth } from '@/contexts/AuthContext'

interface SpFilial { id: number; cod_filial: number; nome: string }
interface SpCD     { id: number; filial_id: number; nome: string }

interface Ignorado {
  id: number
  cd_id: number
  codprod: number
  cod_filial: number
  produto: string | null
  tipo_descricao: string | null
  ignorado_por: string | null
  created_at: string
}

export default function SpIgnorados() {
  const { token } = useAuth()
  const qc = useQueryClient()

  const [filialID, setFilialID]   = useState('')
  const [cdID, setCdID]           = useState('')
  const [reativarId, setReativarId] = useState<number | null>(null)

  const headers = { Authorization: `Bearer ${token}` }

  const { data: filiais = [] } = useQuery<SpFilial[]>({
    queryKey: ['sp-filiais'],
    queryFn: () => fetch('/api/sp/filiais', { headers }).then(r => r.json()),
  })

  const { data: cds = [] } = useQuery<SpCD[]>({
    queryKey: ['sp-cds', filialID],
    queryFn: () => fetch(`/api/sp/cds?filial_id=${filialID}`, { headers }).then(r => r.json()),
    enabled: !!filialID,
  })

  const { data: ignorados = [], isFetching, refetch } = useQuery<Ignorado[]>({
    queryKey: ['sp-ignorados', cdID],
    queryFn: () => fetch(`/api/sp/ignorados${cdID ? `?cd_id=${cdID}` : ''}`, { headers }).then(r => r.json()),
  })

  const reativarMut = useMutation({
    mutationFn: (id: number) =>
      fetch(`/api/sp/ignorados/${id}`, { method: 'DELETE', headers }).then(async r => {
        if (!r.ok) throw new Error((await r.json()).error ?? 'Erro ao reativar')
      }),
    onSuccess: () => {
      toast.success('Produto reativado — será considerado na próxima calibragem')
      qc.invalidateQueries({ queryKey: ['sp-ignorados'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const fmtDate = (iso: string) =>
    new Date(iso).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' })

  return (
    <div className="space-y-4 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-base font-semibold">Produtos Ignorados</h1>
          <p className="text-xs text-muted-foreground mt-0.5">
            Produtos excluídos da calibragem. Não geram proposta de ajuste nem de redução.
          </p>
        </div>
        <Button size="sm" variant="outline" onClick={() => refetch()} disabled={isFetching}>
          <RefreshCw className={`h-3.5 w-3.5 mr-1.5 ${isFetching ? 'animate-spin' : ''}`} />
          Atualizar
        </Button>
      </div>

      {/* Filtros */}
      <div className="flex gap-2 flex-wrap">
        <Select value={filialID || 'all'} onValueChange={v => { setFilialID(v === 'all' ? '' : v); setCdID('') }}>
          <SelectTrigger className="h-8 text-xs w-44">
            <SelectValue placeholder="Filial" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Todas as filiais</SelectItem>
            {filiais.map(f => (
              <SelectItem key={f.id} value={String(f.id)}>{f.nome}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={cdID || 'all'} onValueChange={v => setCdID(v === 'all' ? '' : v)} disabled={!filialID}>
          <SelectTrigger className="h-8 text-xs w-44">
            <SelectValue placeholder="CD" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Todos os CDs</SelectItem>
            {cds.map(c => (
              <SelectItem key={c.id} value={String(c.id)}>{c.nome}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Tabela */}
      {ignorados.length === 0 ? (
        <div className="text-center text-sm text-muted-foreground py-12 border rounded-md">
          Nenhum produto ignorado encontrado.
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="text-[11px]">
              <TableHead className="py-1.5">Produto</TableHead>
              <TableHead className="py-1.5">Cód.</TableHead>
              <TableHead className="py-1.5">Filial</TableHead>
              <TableHead className="py-1.5">Tipo</TableHead>
              <TableHead className="py-1.5">Ignorado por</TableHead>
              <TableHead className="py-1.5">Data</TableHead>
              <TableHead className="py-1.5 w-24">Ação</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {ignorados.map(item => (
              <TableRow key={item.id} className="text-[11px]">
                <TableCell className="py-1 max-w-[160px] truncate" title={item.produto ?? ''}>
                  {item.produto || '—'}
                </TableCell>
                <TableCell className="py-1 font-mono">{item.codprod}</TableCell>
                <TableCell className="py-1">{item.cod_filial}</TableCell>
                <TableCell className="py-1 text-muted-foreground max-w-[140px] truncate" title={item.tipo_descricao ?? ''}>
                  {item.tipo_descricao || '—'}
                </TableCell>
                <TableCell className="py-1 text-muted-foreground">{item.ignorado_por || '—'}</TableCell>
                <TableCell className="py-1 text-muted-foreground whitespace-nowrap">{fmtDate(item.created_at)}</TableCell>
                <TableCell className="py-1">
                  <Button
                    size="sm" variant="outline"
                    className="h-6 text-[10px] px-1.5 text-blue-700 border-blue-200 hover:bg-blue-50"
                    onClick={() => setReativarId(item.id)}
                    disabled={reativarMut.isPending}
                  >
                    <RotateCcw className="h-3 w-3 mr-0.5" />Reativar
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <AlertDialog open={reativarId !== null} onOpenChange={open => { if (!open) setReativarId(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Reativar produto?</AlertDialogTitle>
            <AlertDialogDescription>
              O produto voltará a ser considerado na próxima execução do motor de calibragem.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancelar</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => { if (reativarId !== null) reativarMut.mutate(reativarId); setReativarId(null) }}
            >
              Reativar
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
