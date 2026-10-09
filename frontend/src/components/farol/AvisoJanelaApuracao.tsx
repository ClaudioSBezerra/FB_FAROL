import { CalendarRange } from 'lucide-react'

// Aviso fixo da janela de venda da Numérica (bimestre móvel). A vigência
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
  // Janela devolvida pelo backend; se ausente (resposta antiga em cache), é recalculada.
  apuracaoInicio?: string
  apuracaoFim?: string
  periodoManual?: boolean
  className?: string
}

export default function AvisoJanelaApuracao({ vigInicio, vigFim, apuracaoInicio, apuracaoFim, periodoManual, className = '' }: Props) {
  if (!vigInicio || !vigFim) return null
  const ini = apuracaoInicio || mesAnteriorInicio(vigInicio)
  const fim = apuracaoFim || vigFim
  const v = partes(vigInicio)
  const ant = partes(ini)

  if (periodoManual) {
    return (
      <div className={`flex gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900 ${className}`}>
        <CalendarRange className="h-4 w-4 shrink-0 mt-0.5" />
        <span><b>Período manual:</b> vendas de {br(ini)} a {br(fim)}. Não é o bimestre móvel do programa.</span>
      </div>
    )
  }

  // Mês seguinte: a vigência dele começa no dia 1 do mês seguinte e apura desde o dia 1 do mês atual.
  const sm = v.m === 12 ? 1 : v.m + 1
  const sa = v.m === 12 ? v.a + 1 : v.a
  const proxIni = iso(v.a, v.m, 1)
  const proxFim = iso(sa, sm, ultimoDia(sa, sm))

  return (
    <div className={`flex gap-2 rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-900 ${className}`}>
      <CalendarRange className="h-4 w-4 shrink-0 mt-0.5" />
      <div className="space-y-0.5">
        <div>
          <b>Numérica = bimestre móvel:</b> vendas de <b>{br(ini)} a {br(fim)}</b> ({MESES[ant.m - 1]} + {MESES[v.m - 1]}).
        </div>
        <div className="text-sky-800">
          A vigência cadastrada é só {MESES[v.m - 1]} ({br(vigInicio)} a {br(vigFim)}), mas o sistema sempre soma também o mês anterior.
          Quando virar {MESES[sm - 1]}, a janela passa a ser {br(proxIni)} a {br(proxFim)}.
        </div>
      </div>
    </div>
  )
}
