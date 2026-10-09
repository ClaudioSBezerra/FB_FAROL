import { HelpCircle } from 'lucide-react'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

// Ajuda (ícone ?) ao lado do rótulo Período com a janela de venda da Numérica (bimestre móvel). A vigência
// cadastrada é de 1 mês (ex.: Setembro), mas a apuração soma o mês anterior
// também (01/08 a 30/09). Pedido do Claudio 09/10/2026: "isso precisa ficar
// muito claro para o usuário". Mesma regra do backend (janelaDeApuracao).

const MESES = ['Janeiro', 'Fevereiro', 'Março', 'Abril', 'Maio', 'Junho', 'Julho', 'Agosto', 'Setembro', 'Outubro', 'Novembro', 'Dezembro']

function partes(iso: string) {
  const [a, m, d] = iso.slice(0, 10).split('-').map(Number)
  return { a, m, d }
}
const br = (iso: string) => { const p = partes(iso); return `${String(p.d).padStart(2, '0')}/${String(p.m).padStart(2, '0')}/${p.a}` }
const iso = (a: number, m: number, d: number) => `${a}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
const ultimoDia = (a: number, m: number) => new Date(a, m, 0).getDate()

function mesAnteriorInicio(vigInicio: string) {
  const { a, m } = partes(vigInicio)
  return m === 1 ? iso(a - 1, 12, 1) : iso(a, m - 1, 1)
}

interface Props {
  vigInicio: string
  vigFim: string
}

export default function AjudaJanelaApuracao({ vigInicio, vigFim }: Props) {
  if (!vigInicio || !vigFim) return null
  const ini = mesAnteriorInicio(vigInicio)
  const v = partes(vigInicio)
  const ant = partes(ini)
  // Mês seguinte: a vigência dele apura desde o dia 1 do mês atual.
  const sm = v.m === 12 ? 1 : v.m + 1
  const sa = v.m === 12 ? v.a + 1 : v.a
  const proxIni = iso(v.a, v.m, 1)
  const proxFim = iso(sa, sm, ultimoDia(sa, sm))

  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label="Ajuda: janela de apuração da Numérica"
          className="inline-flex items-center gap-1 normal-case text-sky-700 hover:text-sky-900"
        >
          <HelpCircle className="h-4 w-4" />
          <span className="text-[11px] font-medium">{br(ini)} a {br(vigFim)}</span>
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-80 normal-case text-xs space-y-2" align="start">
        <div className="font-semibold text-sm">Numérica = bimestre móvel</div>
        <p>
          Vendas de <b>{br(ini)} a {br(vigFim)}</b> ({MESES[ant.m - 1]} + {MESES[v.m - 1]}).
        </p>
        <p className="text-muted-foreground">
          A vigência cadastrada é só {MESES[v.m - 1]} ({br(vigInicio)} a {br(vigFim)}), mas o sistema sempre soma também o mês anterior.
          Quando virar {MESES[sm - 1]}, a janela passa a ser {br(proxIni)} a {br(proxFim)}.
        </p>
      </PopoverContent>
    </Popover>
  )
}
