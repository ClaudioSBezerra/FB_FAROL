import { ChevronDown } from 'lucide-react'
import type { LinhaEquipe } from '@/lib/metricasObjetivos'

// Tabela do resumo da equipe no mobile (Melhoria 1 do Heverton 08/10/2026):
// uma linha por CRV ou RCA com os 7 números da planilha dele, e cada linha
// abre o nível de baixo (children).
const fmt1 = (n: number) => n.toLocaleString('pt-BR', { minimumFractionDigits: 0, maximumFractionDigits: 2 })

// Uma única grade (código + 7 números) compartilhada por cabeçalho e linhas,
// pra as colunas ficarem sempre alinhadas — o layout anterior tinha duas
// linhas por registro e o cabeçalho desencontrado dos valores (Heverton
// 09/10/2026: "ainda está confuso, deixar mais harmônico").
const GRADE = 'grid grid-cols-[60px_repeat(7,minmax(0,1fr))] gap-x-1 items-center'

// Nomenclatura da sugestão do Heverton (PDF 09/10/2026): Ponderada fala em
// Ponderadas/Cobertas/Redes, Numérica em Clientes/Cobertos/Clientes. A 1ª
// coluna é só o código (Cód. Sup / Cód. RCA).
export function CabecalhoEquipe({ rotuloCod, numerica, compacto = false }: { rotuloCod: string; numerica: boolean; compacto?: boolean }) {
  if (compacto) {
    return (
      <div className="px-3 py-1 bg-slate-100 border-y text-[9px] font-semibold uppercase tracking-wide text-slate-600">{rotuloCod}</div>
    )
  }
  const col = 'text-center leading-[1.15] normal-case tracking-tighter text-[8px]'
  return (
    <div className="px-3 pt-2 pb-1.5 border-b bg-slate-50 text-[9px] font-semibold uppercase tracking-wide text-slate-500">
      <div className={GRADE}>
        <div />
        <div className="col-span-3 text-center text-slate-700 border-b-2 border-emerald-600/60 pb-0.5">Cobertura</div>
        <div className="col-span-4 text-center text-slate-700 border-b-2 border-indigo-500/60 pb-0.5">Sortimento</div>
      </div>
      <div className={`${GRADE} mt-1`}>
        <div className="text-left text-slate-800 uppercase">{rotuloCod}</div>
        <div className={col}>{numerica ? 'Qt clientes' : 'Qt ponderadas'}</div>
        <div className={col}>{numerica ? 'Qt cobertos' : 'Qt cobertas'}</div>
        <div className={col}>Falta</div>
        <div className={col}>Objetivo sortim.</div>
        <div className={col}>Realizado</div>
        <div className={col}>{numerica ? 'Qt clientes c/ sort.' : 'Qt redes c/ sort.'}</div>
        <div className={col}>Falta</div>
      </div>
    </div>
  )
}

export function LinhaTabelaEquipe({ linha, aberta, onToggle, recuo = false, children }: {
  linha: LinhaEquipe
  aberta: boolean
  onToggle: () => void
  recuo?: boolean
  children?: React.ReactNode
}) {
  const num = 'text-center text-[13px] font-semibold tabular-nums'
  return (
    <div className={`border-b last:border-0 ${recuo ? 'bg-slate-50/70 border-l-2 border-l-slate-300' : ''}`}>
      <button type="button" onClick={onToggle} className="w-full px-3 py-2.5 text-left active:bg-slate-100">
        <div className={GRADE}>
          <div className="flex items-center gap-0.5 min-w-0">
            <ChevronDown className={`w-3.5 h-3.5 shrink-0 text-muted-foreground transition-transform ${aberta ? '' : '-rotate-90'}`} />
            <span className="truncate text-[13px] font-mono font-bold" title={linha.nome}>{linha.codigo}</span>
          </div>
          <div className={num}>{linha.qt}</div>
          <div className={`${num} text-green-700`}>{linha.cobertas}</div>
          <div className={`${num} ${linha.falta > 0 ? 'text-red-700' : 'text-muted-foreground'}`}>{linha.falta}</div>
          <div className={`${num} text-muted-foreground`}>{linha.objetivoSortimento > 0 ? fmt1(linha.objetivoSortimento) : '—'}</div>
          <div className={num}>{fmt1(linha.realSortimento)}</div>
          <div className={`${num} text-green-700`}>{linha.atingindoSortimento}</div>
          <div className={`${num} ${linha.faltaSortimento > 0 ? 'text-red-700' : 'text-muted-foreground'}`}>{linha.faltaSortimento}</div>
        </div>
      </button>
      {aberta && children && <div>{children}</div>}
    </div>
  )
}
