import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { HelpCircle } from 'lucide-react'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { useAuth } from '@/contexts/AuthContext'

// ComoFuncionaIndicadores — explica o RACIONAL de Cobertura e Sortimento
// (não só "o que é", o CÁLCULO passo a passo com exemplo) — pedido do
// Claudio 14/09/2026: usuário e Heverton precisam ver isso na tela, não só
// em conversa. Botão "?" (visível) + atalho F1 (o pedido original citava
// F1) — o F1 só ativa enquanto o componente está montado, sem sequestrar a
// tecla do resto do app.
//
// Números reais (limiar de Cobertura, faixas de Sortimento) vêm do próprio
// cadastro do vínculo/vigência — nunca hardcoded aqui, pra não desatualizar
// sozinho se a JC mudar a meta num mês (só os EXEMPLOS narrativos, que são
// didáticos e não precisam bater com o período atual, ficam fixos).

interface Faixa { faixa: number; valor_meta: number }
interface VigenciaComFaixas { id: number; data_inicio: string; data_fim: string; status: 'aberta' | 'fechada'; faixas: Faixa[] }

function fmtBRL(v: number) {
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 0, maximumFractionDigits: 0 })
}
function fmtNum(v: number) {
  return v.toLocaleString('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 })
}
function faixaTexto(faixas: Faixa[] | undefined, fmt: (v: number) => string): string {
  if (!faixas || faixas.length === 0) return 'consulte as faixas cadastradas para o período'
  const ordenadas = [...faixas].sort((a, b) => b.valor_meta - a.valor_meta)
  return ordenadas.map(f => `${fmt(f.valor_meta)} (Faixa ${f.faixa})`).join(', ')
}

export function BotaoComoFunciona({
  industriaNome, vinculoCoberturaId, vinculoSortimentoId, limiarCobertura,
}: {
  industriaNome: string
  vinculoCoberturaId?: number
  vinculoSortimentoId?: number
  limiarCobertura?: number
}) {
  const [open, setOpen] = useState(false)

  // F1 abre o mesmo diálogo — só enquanto este botão está na tela (cada
  // instância registra e desregistra o próprio listener), então não
  // compete com outra tela que também tenha um BotaoComoFunciona (só o
  // último montado reage, comportamento aceitável pra um atalho de ajuda).
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'F1') {
        e.preventDefault()
        setOpen(true)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        title="Como funciona o cálculo (F1)"
        className="inline-flex items-center gap-1 text-xs text-slate-500 hover:text-slate-800 border border-slate-300 rounded-full px-2 py-1 hover:bg-slate-50"
      >
        <HelpCircle className="h-3.5 w-3.5" /> Como funciona o cálculo
      </button>
      <ComoFuncionaDialog
        open={open} onOpenChange={setOpen} industriaNome={industriaNome}
        vinculoCoberturaId={vinculoCoberturaId} vinculoSortimentoId={vinculoSortimentoId}
        limiarCobertura={limiarCobertura}
      />
    </>
  )
}

export function ComoFuncionaDialog({
  open, onOpenChange, industriaNome, vinculoCoberturaId, vinculoSortimentoId, limiarCobertura,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  industriaNome: string
  vinculoCoberturaId?: number
  vinculoSortimentoId?: number
  limiarCobertura?: number
}) {
  const { token } = useAuth()
  const headers = useMemo(() => ({ Authorization: `Bearer ${token}` }), [token])

  const { data: vigenciasCobertura = [] } = useQuery<VigenciaComFaixas[]>({
    queryKey: ['farol-metas-vigencias-faixas', vinculoCoberturaId],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${vinculoCoberturaId}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: open && !!vinculoCoberturaId,
  })
  const { data: vigenciasSortimento = [] } = useQuery<VigenciaComFaixas[]>({
    queryKey: ['farol-metas-vigencias-faixas', vinculoSortimentoId],
    queryFn: async () => {
      const r = await fetch(`/api/farol/metas-vigencias?vinculo_id=${vinculoSortimentoId}`, { headers })
      if (!r.ok) throw new Error()
      return r.json()
    },
    enabled: open && !!vinculoSortimentoId,
  })
  const vigenciaAtualCob = vigenciasCobertura.find(v => v.status === 'aberta') ?? vigenciasCobertura[0]
  const vigenciaAtualSort = vigenciasSortimento.find(v => v.status === 'aberta') ?? vigenciasSortimento[0]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[85vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Como funcionam os indicadores — {industriaNome}</DialogTitle>
        </DialogHeader>
        <div className="space-y-6 text-sm text-slate-700">
          <section className="space-y-2">
            <h3 className="font-semibold text-slate-900">Métrica 1 — Cobertura (Positivação)</h3>
            <p>
              A Rede (conjunto de CNPJs/lojas) é considerada <strong>coberta (positivada)</strong> quando, no mês,
              comprou {limiarCobertura ? <>acima de <strong>{fmtBRL(limiarCobertura)}</strong></> : 'acima do limiar cadastrado'} na{' '}
              <strong>média entre suas lojas/CNPJs</strong> — soma total comprada dividida pela quantidade de lojas
              da Rede, não a soma bruta.
            </p>
            <div className="rounded-lg bg-slate-50 border border-slate-200 p-3 text-xs">
              <strong>Exemplo:</strong> uma Rede com 4 lojas comprou: Loja A R$&nbsp;1.000, Loja B R$&nbsp;0, Loja C
              R$&nbsp;20.000, Loja D R$&nbsp;40.000. Média = (1.000+0+20.000+40.000) ÷ 4 = <strong>R$&nbsp;15.250</strong>.
            </div>
            <p>
              Apuração total do distribuidor (soma de quantas Redes bateram o limiar): a lista de faixas cadastrada
              hoje é <strong>{faixaTexto(vigenciaAtualCob?.faixas, v => `${v} redes`)}</strong>
              {vigenciaAtualCob && <> (período {vigenciaAtualCob.data_inicio} a {vigenciaAtualCob.data_fim})</>}.
            </p>
          </section>

          <section className="space-y-2 border-t border-slate-100 pt-4">
            <h3 className="font-semibold text-slate-900">Métrica 2 — Sortimento (Mix Positivado)</h3>
            <p>
              Existe uma lista de <strong>Itens Válidos</strong> (EANs), enviada mensalmente pelo fornecedor. Cada
              Rede precisa comprar, na <strong>média de suas lojas</strong> durante o mês, uma quantidade de EANs
              distintos dessa lista acima da meta.
            </p>
            <div className="rounded-lg bg-slate-50 border border-slate-200 p-3 text-xs space-y-1">
              <div>
                <strong>Exemplo da média:</strong> Loja 1 comprou 50 EANs; Loja 2 comprou 30 EANs diferentes dos da
                Loja 1. Média da Rede = (50+30) ÷ 2 = <strong>40 EANs</strong>. Quando o mesmo EAN se repete entre
                lojas da Rede, ele conta só <strong>1 vez</strong> (não soma duplicado).
              </div>
              <div>
                <strong>Regra de quantidade mínima:</strong> pra um EAN contar como positivado NA LOJA, é preciso
                vender pelo menos <strong>3 unidades</strong> — isso vale só pra produtos vendidos "UN". Produtos
                vendidos em caixa/pacote/display já ultrapassam 3 unidades numa venda só, então essa regra não se
                aplica a eles.
              </div>
            </div>
            <p>
              Apuração total do distribuidor: <strong>média de TODAS as Redes da lista</strong> (as que não
              compraram nada entram com 0, puxando a média pra baixo — não são descartadas do cálculo). A lista de
              faixas cadastrada hoje é <strong>{faixaTexto(vigenciaAtualSort?.faixas, v => `${fmtNum(v)} EANs`)}</strong>
              {vigenciaAtualSort && <> (período {vigenciaAtualSort.data_inicio} a {vigenciaAtualSort.data_fim})</>}.
            </p>
          </section>
        </div>
      </DialogContent>
    </Dialog>
  )
}
