import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuth } from '@/contexts/AuthContext'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import { modules } from '@/lib/navigation'

// ─── Types ────────────────────────────────────────────────────────────────────

interface UsageRow {
  user_id:       string
  user_email:    string
  user_name:     string
  empresa_nome:  string
  modulo:        string
  total_visitas: number
  tempo_total_seg: number
  tempo_medio_seg: number
  ultima_visita: string
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

const MODULO_LABEL: Record<string, string> = Object.fromEntries(
  Object.entries(modules).map(([k, v]) => [k, v.label])
)

function fmtTempo(seg: number): string {
  if (seg < 60)   return `${seg}s`
  if (seg < 3600) return `${Math.floor(seg / 60)}m ${seg % 60}s`
  const h = Math.floor(seg / 3600)
  const m = Math.floor((seg % 3600) / 60)
  return `${h}h ${m}m`
}

function fmtDate(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleDateString('pt-BR') + ' ' + d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function SpUsoSistema() {
  const { token } = useAuth()
  const headers = { Authorization: `Bearer ${token}` }

  const [dias, setDias] = useState<7 | 30 | 90>(30)
  const [view, setView] = useState<'usuario' | 'modulo'>('usuario')

  const { data: rows = [], isLoading } = useQuery<UsageRow[]>({
    queryKey: ['sp-uso-report', dias],
    staleTime: 60_000,
    queryFn: async () => {
      const r = await fetch(`/api/sp/admin/uso?dias=${dias}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  // ── Resumo ─────────────────────────────────────────────────────────────────
  const resumo = useMemo(() => {
    const users    = new Set(rows.map(r => r.user_id)).size
    const visitas  = rows.reduce((acc, r) => acc + r.total_visitas, 0)
    const moduloMap: Record<string, number> = {}
    rows.forEach(r => { moduloMap[r.modulo] = (moduloMap[r.modulo] ?? 0) + r.total_visitas })
    const topModulo = Object.entries(moduloMap).sort((a, b) => b[1] - a[1])[0]?.[0] ?? '—'
    return { users, visitas, topModulo }
  }, [rows])

  // ── Agrupamento por usuário ────────────────────────────────────────────────
  const byUser = useMemo(() => {
    const map: Record<string, {
      user_id: string, user_email: string, user_name: string, empresa_nome: string,
      total_visitas: number, tempo_total_seg: number, ultima_visita: string,
      modulos: string[]
    }> = {}
    rows.forEach(r => {
      if (!map[r.user_id]) {
        map[r.user_id] = {
          user_id: r.user_id, user_email: r.user_email, user_name: r.user_name,
          empresa_nome: r.empresa_nome, total_visitas: 0, tempo_total_seg: 0,
          ultima_visita: r.ultima_visita, modulos: [],
        }
      }
      const u = map[r.user_id]
      u.total_visitas  += r.total_visitas
      u.tempo_total_seg += r.tempo_total_seg
      u.modulos.push(MODULO_LABEL[r.modulo] ?? r.modulo)
      if (r.ultima_visita > u.ultima_visita) u.ultima_visita = r.ultima_visita
    })
    return Object.values(map).sort((a, b) => b.ultima_visita.localeCompare(a.ultima_visita))
  }, [rows])

  // ── Agrupamento por módulo ─────────────────────────────────────────────────
  const byModulo = useMemo(() => {
    const map: Record<string, {
      modulo: string, total_visitas: number, usuarios: Set<string>,
      tempo_total_seg: number, ultima_visita: string,
    }> = {}
    rows.forEach(r => {
      if (!map[r.modulo]) {
        map[r.modulo] = {
          modulo: r.modulo, total_visitas: 0, usuarios: new Set(),
          tempo_total_seg: 0, ultima_visita: r.ultima_visita,
        }
      }
      const m = map[r.modulo]
      m.total_visitas  += r.total_visitas
      m.tempo_total_seg += r.tempo_total_seg
      m.usuarios.add(r.user_id)
      if (r.ultima_visita > m.ultima_visita) m.ultima_visita = r.ultima_visita
    })
    return Object.values(map)
      .map(m => ({ ...m, n_usuarios: m.usuarios.size }))
      .sort((a, b) => b.total_visitas - a.total_visitas)
  }, [rows])

  return (
    <div className="space-y-4">
      {/* ── Cabeçalho + controles ── */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold">Uso do Sistema</h2>
          <p className="text-xs text-muted-foreground">
            Tempo de permanência por usuário e módulo.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">Período:</span>
          {([7, 30, 90] as const).map(d => (
            <Button
              key={d}
              size="sm"
              variant={dias === d ? 'default' : 'outline'}
              className="h-7 px-3 text-xs"
              onClick={() => setDias(d)}
            >
              {d}d
            </Button>
          ))}
        </div>
      </div>

      {/* ── Cards de resumo ── */}
      <div className="grid grid-cols-3 gap-3">
        <div className="border rounded-lg px-4 py-3 bg-blue-50">
          <p className="text-xs text-muted-foreground">Usuários ativos</p>
          <p className="text-2xl font-bold text-blue-700">{resumo.users}</p>
        </div>
        <div className="border rounded-lg px-4 py-3 bg-green-50">
          <p className="text-xs text-muted-foreground">Total de visitas</p>
          <p className="text-2xl font-bold text-green-700">{resumo.visitas.toLocaleString('pt-BR')}</p>
        </div>
        <div className="border rounded-lg px-4 py-3 bg-amber-50">
          <p className="text-xs text-muted-foreground">Módulo mais acessado</p>
          <p className="text-lg font-bold text-amber-700 truncate">
            {MODULO_LABEL[resumo.topModulo] ?? resumo.topModulo}
          </p>
        </div>
      </div>

      {/* ── Toggle de visualização ── */}
      <div className="flex gap-2">
        <Button
          size="sm" variant={view === 'usuario' ? 'default' : 'outline'}
          className="h-7 text-xs"
          onClick={() => setView('usuario')}
        >
          Por usuário
        </Button>
        <Button
          size="sm" variant={view === 'modulo' ? 'default' : 'outline'}
          className="h-7 text-xs"
          onClick={() => setView('modulo')}
        >
          Por módulo
        </Button>
      </div>

      {/* ── Tabela ── */}
      {isLoading ? (
        <p className="text-sm text-muted-foreground py-8 text-center">Carregando…</p>
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground py-8 text-center">
          Nenhum dado de uso registrado nos últimos {dias} dias.
        </p>
      ) : view === 'usuario' ? (
        <Table>
          <TableHeader>
            <TableRow className="text-[11px]">
              <TableHead className="py-1.5">Usuário</TableHead>
              <TableHead className="py-1.5">Empresa</TableHead>
              <TableHead className="py-1.5">Módulos acessados</TableHead>
              <TableHead className="py-1.5 text-right">Visitas</TableHead>
              <TableHead className="py-1.5 text-right">Tempo total</TableHead>
              <TableHead className="py-1.5">Última visita</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {byUser.map(u => (
              <TableRow key={u.user_id} className="text-[11px]">
                <TableCell className="py-1.5">
                  <div className="font-medium">{u.user_name || '—'}</div>
                  <div className="text-[10px] text-muted-foreground">{u.user_email}</div>
                </TableCell>
                <TableCell className="py-1.5 text-muted-foreground">{u.empresa_nome || '—'}</TableCell>
                <TableCell className="py-1.5 text-muted-foreground text-[10px]">
                  {[...new Set(u.modulos)].join(', ')}
                </TableCell>
                <TableCell className="py-1.5 text-right font-mono">{u.total_visitas}</TableCell>
                <TableCell className="py-1.5 text-right font-mono">{fmtTempo(u.tempo_total_seg)}</TableCell>
                <TableCell className="py-1.5 font-mono text-muted-foreground">{fmtDate(u.ultima_visita)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <Table>
          <TableHeader>
            <TableRow className="text-[11px]">
              <TableHead className="py-1.5">Módulo</TableHead>
              <TableHead className="py-1.5 text-right">Usuários</TableHead>
              <TableHead className="py-1.5 text-right">Visitas</TableHead>
              <TableHead className="py-1.5 text-right">Tempo total</TableHead>
              <TableHead className="py-1.5 text-right">Tempo médio/visita</TableHead>
              <TableHead className="py-1.5">Último acesso</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {byModulo.map(m => (
              <TableRow key={m.modulo} className="text-[11px]">
                <TableCell className="py-1.5 font-medium">
                  {MODULO_LABEL[m.modulo] ?? m.modulo}
                </TableCell>
                <TableCell className="py-1.5 text-right font-mono">{m.n_usuarios}</TableCell>
                <TableCell className="py-1.5 text-right font-mono">{m.total_visitas}</TableCell>
                <TableCell className="py-1.5 text-right font-mono">{fmtTempo(m.tempo_total_seg)}</TableCell>
                <TableCell className="py-1.5 text-right font-mono">
                  {m.total_visitas > 0 ? fmtTempo(Math.round(m.tempo_total_seg / m.total_visitas)) : '—'}
                </TableCell>
                <TableCell className="py-1.5 font-mono text-muted-foreground">{fmtDate(m.ultima_visita)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}
