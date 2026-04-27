import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@/contexts/AuthContext'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'

interface AuditEntry {
  id: number
  user_email: string | null
  user_name: string | null
  entidade: string
  entidade_id: string
  acao: string
  payload: Record<string, unknown> | null
  created_at: string
}

const ACAO_LABELS: Record<string, { label: string; color: string }> = {
  limpar_dados:        { label: 'Limpeza de dados',   color: 'bg-red-100 text-red-800' },
  criar_usuario:       { label: 'Criar usuário',      color: 'bg-green-100 text-green-800' },
  excluir_usuario:     { label: 'Excluir usuário',    color: 'bg-red-100 text-red-800' },
  bloquear_empresa:    { label: 'Bloquear empresa',   color: 'bg-red-100 text-red-800' },
  desbloquear_empresa: { label: 'Desbloquear empresa', color: 'bg-green-100 text-green-800' },
  renovar_licenca:     { label: 'Renovar licença',    color: 'bg-blue-100 text-blue-800' },
}

function formatDate(iso: string) {
  const d = new Date(iso)
  return d.toLocaleDateString('pt-BR') + ' ' + d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
}

export default function SpAuditLog() {
  const { token } = useAuth()
  const headers = { Authorization: `Bearer ${token}` }

  const { data: entries = [], isLoading } = useQuery<AuditEntry[]>({
    queryKey: ['sp-audit-log'],
    staleTime: 30_000,
    queryFn: async () => {
      const r = await fetch('/api/sp/admin/audit-log?limit=500', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold">Log de Auditoria</h2>
      <p className="text-xs text-muted-foreground">
        Registro de ações administrativas: limpeza de dados, cadastro e exclusão de usuários.
      </p>

      {isLoading ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Carregando...</p>
      ) : entries.length === 0 ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Nenhum registro de auditoria encontrado.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="text-[11px]">
              <TableHead className="py-1.5 w-36">Data/Hora</TableHead>
              <TableHead className="py-1.5">Usuário</TableHead>
              <TableHead className="py-1.5">Ação</TableHead>
              <TableHead className="py-1.5">Detalhes</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map(e => {
              const cfg = ACAO_LABELS[e.acao] ?? { label: e.acao, color: 'bg-gray-100 text-gray-800' }
              // L5 fix: renderiza todo o payload como "chave: valor" (exceto null/undefined)
              const labelMap: Record<string, string> = {
                email: 'Email', full_name: 'Nome', sp_role: 'Perfil',
                sp_csv_jobs: 'Jobs', sp_enderecos: 'Endereços',
                sp_propostas: 'Propostas', sp_historico: 'Histórico',
                arquivos_a_remover: 'Arquivos', motivo: 'Motivo',
                trial_ends_at: 'Nova licença até',
              }
              const details: string[] = e.payload
                ? Object.entries(e.payload)
                    .filter(([, v]) => v !== null && v !== undefined && v !== '')
                    .map(([k, v]) => `${labelMap[k] ?? k}: ${v}`)
                : []
              return (
                <TableRow key={e.id} className="text-[11px]">
                  <TableCell className="py-1.5 font-mono text-muted-foreground">{formatDate(e.created_at)}</TableCell>
                  <TableCell className="py-1.5">
                    <div className="text-xs font-medium">{e.user_name ?? '—'}</div>
                    <div className="text-[10px] text-muted-foreground">{e.user_email ?? ''}</div>
                  </TableCell>
                  <TableCell className="py-1.5">
                    <span className={`inline-flex px-2 py-0.5 rounded text-[10px] font-semibold ${cfg.color}`}>
                      {cfg.label}
                    </span>
                  </TableCell>
                  <TableCell className="py-1.5 text-[11px] text-muted-foreground">
                    {details.length > 0 ? details.join(' | ') : '—'}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
