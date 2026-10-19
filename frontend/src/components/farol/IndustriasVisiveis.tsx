import { useQuery } from '@tanstack/react-query'
import { Checkbox } from '@/components/ui/checkbox'
import { useAuth } from '@/contexts/AuthContext'

// Indústrias que o usuário enxerga nos painéis e nas cargas (users.industrias_permitidas).
// Nenhuma marcada = TODAS (padrão). Usado na edição de usuário (19/10/2026).
// empresaId = empresa do USUÁRIO editado (não a do admin logado): indústria é cadastro por empresa.
export default function IndustriasVisiveis({ valor, onChange, empresaId }: { valor: number[]; onChange: (v: number[]) => void; empresaId?: string }) {
  const { token } = useAuth()
  const { data: industrias = [] } = useQuery<Array<{ id: number; nome: string }>>({
    queryKey: ['farol-industrias-lista', empresaId ?? ''],
    queryFn: async () => {
      const headers: Record<string, string> = { Authorization: `Bearer ${token}` }
      if (empresaId) headers['X-Company-ID'] = empresaId
      const r = await fetch('/api/farol/industrias', { headers })
      if (!r.ok) return []
      return r.json()
    },
  })
  const ordenadas = [...industrias].sort((a, b) => a.nome.localeCompare(b.nome, 'pt-BR'))
  const alternar = (id: number) => onChange(valor.includes(id) ? valor.filter(x => x !== id) : [...valor, id])
  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">Indústrias que ele vê</span>
        <span className="text-xs text-muted-foreground">{valor.length === 0 ? 'Todas (nenhuma marcada)' : `${valor.length} marcada(s)`}</span>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-1 max-h-40 overflow-auto border rounded-md p-2">
        {ordenadas.map(i => (
          <label key={i.id} className="flex items-center gap-2 text-sm cursor-pointer">
            <Checkbox checked={valor.includes(i.id)} onCheckedChange={() => alternar(i.id)} />
            <span className="truncate">{i.nome}</span>
          </label>
        ))}
      </div>
      {valor.length > 0 && (
        <button type="button" className="text-xs text-violet-700 hover:underline mt-1" onClick={() => onChange([])}>Limpar (voltar a todas)</button>
      )}
    </div>
  )
}
