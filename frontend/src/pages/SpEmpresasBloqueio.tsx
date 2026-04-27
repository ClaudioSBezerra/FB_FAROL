import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '@/contexts/AuthContext'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from '@/components/ui/dialog'
import { toast } from 'sonner'
import { Lock, Unlock, ShieldAlert } from 'lucide-react'

interface Empresa {
  id: string
  name: string
  trade_name: string
  group_id: string
  group_name: string
  blocked_at?: string | null
  blocked_reason?: string | null
  blocked_by_email?: string | null
}

function formatDate(iso?: string | null) {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleDateString('pt-BR') + ' ' + d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
}

export default function SpEmpresasBloqueio() {
  const { token, companyId } = useAuth()
  const qc = useQueryClient()
  const headers = { Authorization: `Bearer ${token}` }

  const [dialogEmpresa, setDialogEmpresa] = useState<Empresa | null>(null)
  const [motivo, setMotivo] = useState('')

  const { data: empresas = [], isLoading } = useQuery<Empresa[]>({
    queryKey: ['sp-empresas-bloqueio'],
    queryFn: async () => {
      const r = await fetch('/api/sp/admin/empresas', { headers })
      if (!r.ok) throw new Error('Falha ao carregar empresas')
      return r.json()
    },
  })

  const bloquear = useMutation({
    mutationFn: async ({ id, motivo }: { id: string; motivo: string }) => {
      const r = await fetch(`/api/sp/admin/empresas/${id}/bloquear`, {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify({ motivo }),
      })
      if (!r.ok) throw new Error((await r.text()) || 'Falha ao bloquear')
    },
    onSuccess: () => {
      toast.success('Empresa bloqueada')
      qc.invalidateQueries({ queryKey: ['sp-empresas-bloqueio'] })
      setDialogEmpresa(null)
      setMotivo('')
    },
    onError: (e: Error) => toast.error(e.message),
  })

  const desbloquear = useMutation({
    mutationFn: async (id: string) => {
      const r = await fetch(`/api/sp/admin/empresas/${id}/desbloquear`, {
        method: 'POST',
        headers,
      })
      if (!r.ok) throw new Error((await r.text()) || 'Falha ao desbloquear')
    },
    onSuccess: () => {
      toast.success('Empresa desbloqueada')
      qc.invalidateQueries({ queryKey: ['sp-empresas-bloqueio'] })
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold flex items-center gap-2">
        <ShieldAlert className="h-5 w-5 text-amber-600" /> Bloqueio de Empresas
      </h2>
      <p className="text-xs text-muted-foreground">
        Bloquear impede que usuários da empresa acessem o sistema, <strong>sem apagar os dados</strong>.
        Desbloquear restaura o acesso imediatamente. Sua empresa ativa não pode ser bloqueada.
      </p>

      {isLoading ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
      ) : empresas.length === 0 ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Nenhuma empresa encontrada.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="text-[11px]">
              <TableHead className="py-1.5">Grupo</TableHead>
              <TableHead className="py-1.5">Empresa</TableHead>
              <TableHead className="py-1.5">Status</TableHead>
              <TableHead className="py-1.5">Motivo</TableHead>
              <TableHead className="py-1.5">Bloqueada em</TableHead>
              <TableHead className="py-1.5 w-32">Ação</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {empresas.map(e => {
              const isBlocked = !!e.blocked_at
              const isActiveCompany = e.id === companyId
              return (
                <TableRow key={e.id} className={`text-[11px] ${isBlocked ? 'bg-red-50/50' : ''}`}>
                  <TableCell className="py-1.5">{e.group_name || '—'}</TableCell>
                  <TableCell className="py-1.5">
                    <div className="font-medium">{e.name}</div>
                    {e.trade_name && <div className="text-[10px] text-muted-foreground">{e.trade_name}</div>}
                  </TableCell>
                  <TableCell className="py-1.5">
                    {isBlocked ? (
                      <span className="inline-flex px-2 py-0.5 rounded text-[10px] font-semibold bg-red-100 text-red-800">
                        Bloqueada
                      </span>
                    ) : (
                      <span className="inline-flex px-2 py-0.5 rounded text-[10px] font-semibold bg-green-100 text-green-800">
                        Ativa
                      </span>
                    )}
                  </TableCell>
                  <TableCell className="py-1.5 max-w-[200px] truncate" title={e.blocked_reason ?? ''}>
                    {e.blocked_reason ?? '—'}
                  </TableCell>
                  <TableCell className="py-1.5 font-mono text-muted-foreground">
                    {formatDate(e.blocked_at)}
                    {e.blocked_by_email && (
                      <div className="text-[10px]">por {e.blocked_by_email}</div>
                    )}
                  </TableCell>
                  <TableCell className="py-1.5">
                    {isActiveCompany ? (
                      <span className="text-[10px] text-muted-foreground italic">empresa ativa</span>
                    ) : isBlocked ? (
                      <Button
                        size="sm" variant="outline"
                        className="h-7 text-[10px] text-green-700 border-green-200 hover:bg-green-50"
                        disabled={desbloquear.isPending}
                        onClick={() => desbloquear.mutate(e.id)}
                      >
                        <Unlock className="h-3 w-3 mr-1" /> Desbloquear
                      </Button>
                    ) : (
                      <Button
                        size="sm" variant="outline"
                        className="h-7 text-[10px] text-red-700 border-red-200 hover:bg-red-50"
                        onClick={() => { setDialogEmpresa(e); setMotivo('') }}
                      >
                        <Lock className="h-3 w-3 mr-1" /> Bloquear
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}

      {/* Dialog para confirmar bloqueio com motivo */}
      <Dialog open={!!dialogEmpresa} onOpenChange={open => { if (!open) { setDialogEmpresa(null); setMotivo('') } }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>Bloquear {dialogEmpresa?.name}</DialogTitle>
          </DialogHeader>
          <p className="text-xs text-muted-foreground">
            Informe o motivo do bloqueio. Essa informação ficará registrada no audit log e será exibida quando um usuário
            da empresa tentar acessar o sistema.
          </p>
          <Input
            placeholder="Ex.: Inadimplência desde 2026-03-01"
            value={motivo}
            onChange={e => setMotivo(e.target.value)}
            className="text-xs"
            autoFocus
          />
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={() => { setDialogEmpresa(null); setMotivo('') }}>
              Cancelar
            </Button>
            <Button
              variant="destructive" size="sm"
              disabled={!motivo.trim() || bloquear.isPending}
              onClick={() => dialogEmpresa && bloquear.mutate({ id: dialogEmpresa.id, motivo: motivo.trim() })}
            >
              {bloquear.isPending ? 'Bloqueando...' : 'Confirmar bloqueio'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
