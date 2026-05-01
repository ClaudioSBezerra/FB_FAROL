import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Trash2, ShieldAlert, CheckCircle2, Database, Users } from 'lucide-react'
import { toast } from 'sonner'

type TipoPeriodo = 'MENSAL' | 'TRIMESTRAL' | 'SEMESTRAL' | 'ANUAL'
interface Periodo { tipo_periodo: TipoPeriodo; ano: number; periodo_seq: number }

const MESES = ['Jan','Fev','Mar','Abr','Mai','Jun','Jul','Ago','Set','Out','Nov','Dez']
const TRIMESTRES = ['T1','T2','T3','T4']
const SEMESTRES  = ['S1','S2']

function periodoLabel(p: Periodo): string {
  const { tipo_periodo: t, ano, periodo_seq: s } = p
  if (t === 'MENSAL')     return `${MESES[s - 1] ?? s}/${ano}`
  if (t === 'TRIMESTRAL') return `${TRIMESTRES[s - 1] ?? s}/${ano}`
  if (t === 'SEMESTRAL')  return `${SEMESTRES[s - 1] ?? s}/${ano}`
  return String(ano)
}

export default function ObjetivosManutencao() {
  const [loading, setLoading] = useState(false)
  const queryClient = useQueryClient()

  const { data: periodos = [], refetch } = useQuery<Periodo[]>({
    queryKey: ['objetivos-periodos'],
    queryFn:  () => fetch('/api/objetivos/periodos').then(r => r.json()).then(d => Array.isArray(d) ? d : []),
  })

  async function handleLimpar() {
    setLoading(true)
    try {
      const res = await fetch('/api/objetivos/limpar', { method: 'POST' })
      const data = await res.json()
      if (!res.ok) {
        toast.error(data.error ?? 'Erro ao limpar a base')
        return
      }
      toast.success(`Base limpa — ${data.deleted.toLocaleString('pt-BR')} registros removidos`)
      // invalida todos os caches de objetivos
      queryClient.invalidateQueries({ queryKey: ['objetivos-periodos'] })
      queryClient.invalidateQueries({ queryKey: ['objetivos-rca-all'] })
      queryClient.invalidateQueries({ queryKey: ['objetivos-supervisor-all'] })
      refetch()
    } catch {
      toast.error('Erro de conexão')
    } finally {
      setLoading(false)
    }
  }

  const temDados = periodos.length > 0

  return (
    <div className="max-w-2xl space-y-6">

      {/* ── Status atual ── */}
      <div className="border rounded-lg p-4 bg-white space-y-3">
        <div className="flex items-center gap-2">
          <Database className="w-4 h-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">Estado atual da base de objetivos</h2>
        </div>

        {temDados ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">
              {periodos.length} período{periodos.length !== 1 ? 's' : ''} importado{periodos.length !== 1 ? 's' : ''}:
            </p>
            <div className="flex flex-wrap gap-1.5">
              {periodos.map(p => (
                <Badge key={`${p.tipo_periodo}|${p.ano}|${p.periodo_seq}`} variant="secondary" className="text-xs">
                  {periodoLabel(p)}
                </Badge>
              ))}
            </div>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-green-600">
            <CheckCircle2 className="w-4 h-4" />
            <span className="text-xs font-medium">Base de objetivos está vazia</span>
          </div>
        )}
      </div>

      {/* ── O que NÃO será apagado ── */}
      <div className="border rounded-lg p-4 bg-white space-y-3">
        <div className="flex items-center gap-2">
          <Users className="w-4 h-4 text-green-600" />
          <h2 className="text-sm font-semibold text-green-700">Cadastros preservados</h2>
        </div>
        <ul className="text-xs text-muted-foreground space-y-1 list-disc list-inside">
          <li>Gestores cadastrados</li>
          <li>RCAs cadastrados</li>
          <li>Usuários e configurações da empresa</li>
        </ul>
      </div>

      {/* ── Zona de perigo ── */}
      <div className="border border-red-200 rounded-lg p-4 bg-red-50 space-y-3">
        <div className="flex items-center gap-2 text-red-700">
          <ShieldAlert className="w-4 h-4" />
          <h2 className="text-sm font-semibold">Limpar base de objetivos</h2>
        </div>
        <p className="text-xs text-red-700/80">
          Remove <strong>todos</strong> os objetivos importados desta empresa. A ação não pode ser desfeita.
          Após a limpeza, reimporte o CSV para restaurar os dados.
        </p>

        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button
              variant="destructive"
              size="sm"
              disabled={!temDados || loading}
              className="gap-2"
            >
              <Trash2 className="w-4 h-4" />
              {loading ? 'Limpando...' : 'Limpar Base de Objetivos'}
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle className="flex items-center gap-2 text-red-600">
                <ShieldAlert className="w-5 h-5" />
                Confirmar limpeza da base
              </AlertDialogTitle>
              <AlertDialogDescription asChild>
                <div className="space-y-2 text-sm">
                  <p>Isso irá apagar <strong>permanentemente</strong> os seguintes dados:</p>
                  <ul className="list-disc list-inside space-y-1 text-muted-foreground">
                    {periodos.map(p => (
                      <li key={`${p.tipo_periodo}|${p.ano}|${p.periodo_seq}`}>
                        Objetivos do período <strong>{periodoLabel(p)}</strong>
                      </li>
                    ))}
                  </ul>
                  <p className="text-red-600 font-medium pt-1">Esta ação não pode ser desfeita.</p>
                </div>
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancelar</AlertDialogCancel>
              <AlertDialogAction
                className="bg-red-600 hover:bg-red-700 focus:ring-red-600"
                onClick={handleLimpar}
              >
                Sim, limpar tudo
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>

    </div>
  )
}
