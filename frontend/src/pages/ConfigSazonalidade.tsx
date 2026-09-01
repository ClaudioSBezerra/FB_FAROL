import { useMemo, useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { useAuth } from '@/contexts/AuthContext'
import { CalendarClock, Loader2 } from 'lucide-react'
import { toast } from 'sonner'

interface SpFilial { id: number; cod_filial: number; nome: string }

interface GerarResultado {
  ok: boolean
  anos_processados: number[]
  erros: number
  duration_ms: number
}

const ANO_ATUAL = new Date().getFullYear()
// Últimos 6 anos (inclui o atual) — faixa plausível de histórico sem
// depender de uma consulta extra só pra listar anos com dado.
const ANOS_DISPONIVEIS = Array.from({ length: 6 }, (_, i) => ANO_ATUAL - i)

export default function ConfigSazonalidade() {
  const { token } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  const [filialID, setFilialID] = useState('all')
  const [ano, setAno] = useState(String(ANO_ATUAL))

  const { data: filiais = [] } = useQuery<SpFilial[]>({
    queryKey: ['filiais'],
    queryFn: async () => {
      const r = await fetch('/api/filiais', { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
  })

  const gerarMutation = useMutation({
    mutationFn: async () => {
      const filial = filiais.find(f => String(f.id) === filialID)
      const body: { cod_filial?: number; ano?: number } = {}
      if (filial) body.cod_filial = filial.cod_filial
      if (ano !== 'all') body.ano = Number(ano)

      const r = await fetch('/api/v2/farol/sazonalidade/gerar', {
        method: 'POST',
        headers: { ...headers, 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const data = await r.json()
      if (!r.ok) throw new Error(data.error ?? 'Erro ao gerar sazonalidade')
      return data as GerarResultado
    },
    onSuccess: data => {
      const anos = data.anos_processados.join(', ')
      if (data.erros > 0) {
        toast.warning(`Concluído com ${data.erros} erro(s) — ano(s) ${anos}. Veja os logs do servidor.`)
      } else {
        toast.success(`Sazonalidade gerada — ano(s) ${anos} em ${(data.duration_ms / 1000).toFixed(1)}s`)
      }
    },
    onError: (e: Error) => toast.error(e.message),
  })

  return (
    <div className="max-w-2xl space-y-4">
      <div className="flex items-center gap-2">
        <CalendarClock className="h-5 w-5 text-amber-600" />
        <div>
          <h1 className="text-base font-semibold">Sazonalidade por Produto</h1>
          <p className="text-xs text-muted-foreground">
            Gera/reprocessa a sazonalidade persistida (Produto × Filial × Ano) sob demanda.
            A importação diária já roda isso automaticamente de madrugada — use esta tela só
            pra forçar um reprocessamento fora do ciclo normal (ex.: dado corrigido numa filial
            específica, ou pra conferir o resultado sem esperar o próximo import).
          </p>
        </div>
      </div>

      <div className="rounded-lg border p-4 space-y-4">
        <div className="flex flex-wrap gap-3 items-end">
          <div>
            <label className="text-xs font-medium mb-1 block">Filial</label>
            <Select value={filialID} onValueChange={setFilialID}>
              <SelectTrigger className="w-56"><SelectValue placeholder="Selecione" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Todas as filiais</SelectItem>
                {filiais.map(f => (
                  <SelectItem key={f.id} value={String(f.id)}>{f.nome} (cód. {f.cod_filial})</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div>
            <label className="text-xs font-medium mb-1 block">Ano</label>
            <Select value={ano} onValueChange={setAno}>
              <SelectTrigger className="w-40"><SelectValue placeholder="Selecione" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Todos os anos</SelectItem>
                {ANOS_DISPONIVEIS.map(a => (
                  <SelectItem key={a} value={String(a)}>{a}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Button
            onClick={() => gerarMutation.mutate()}
            disabled={gerarMutation.isPending}
          >
            {gerarMutation.isPending
              ? <><Loader2 className="h-4 w-4 mr-1.5 animate-spin" />Gerando…</>
              : 'Gerar Sazonalidade'}
          </Button>
        </div>

        <p className="text-[11px] text-muted-foreground">
          "Todas as filiais" ou "Todos os anos" processam mais dado e podem demorar mais —
          o botão fica desabilitado até terminar. A ação é segura pra repetir (idempotente):
          rodar de novo só atualiza os números, nunca duplica.
        </p>
      </div>
    </div>
  )
}
