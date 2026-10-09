import { ChevronDown } from 'lucide-react'
import type { LinhaEquipe } from '@/lib/metricasObjetivos'

// Tabela do resumo da equipe no mobile (Melhoria 1 do Heverton 08/10/2026):
// uma linha por CRV ou RCA com os 7 números da planilha dele, e cada linha
// abre o nível de baixo (children).
const fmt1 = (n: number) => n.toLocaleString('pt-BR', { minimumFractionDigits: 0, maximumFractionDigits: 2 })

export function CabecalhoEquipe({ rotuloQt }: { rotuloQt: string }) {
  const col = 'text-center leading-tight'
  return (
    <div className="px-3 pt-2 pb-1 border-b bg-slate-50 text-[9px] font-semibold uppercase tracking-wide text-muted-foreground">
      <div className="grid grid-cols-7 gap-1">
        <div className="col-span-3 text-center">Cobertura</div>
        <div className="col-span-4 text-center">Sortimento</div>
      </div>
      <div className="grid grid-cols-7 gap-1 mt-0.5">
        <div className={col}>{rotuloQt}</div>
        <div className={col}>Cobertas</div>
        <div className={col}>Falta</div>
        <div className={col}>Objet.</div>
        <div className={col}>Real</div>
        <div className={col}>Ating.</div>
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
    <div className={`border-b last:border-0 ${recuo ? 'bg-slate-50/70' : ''}`}>
      <button type="button" onClick={onToggle} className="w-full px-3 py-2 text-left space-y-1 active:bg-slate-100">
        <div className="flex items-center gap-1.5 min-w-0">
          <ChevronDown className={`w-3.5 h-3.5 shrink-0 text-muted-foreground transition-transform ${aberta ? '' : '-rotate-90'}`} />
          <span className="truncate text-sm font-medium"><span className="font-mono font-semibold">{linha.codigo}</span> - {linha.nome}</span>
        </div>
        <div className="grid grid-cols-7 gap-1 pl-5">
          <div className={num}>{linha.qt}</div>
          <div className={`${num} text-green-700`}>{linha.cobertas}</div>
          <div className={`${num} ${linha.falta > 0 ? 'text-red-700' : 'text-muted-foreground'}`}>{linha.falta}</div>
          <div className={`${num} text-muted-foreground`}>{linha.objetivoSortimento > 0 ? fmt1(linha.objetivoSortimento) : '—'}</div>
          <div className={num}>{fmt1(linha.realSortimento)}</div>
          <div className={`${num} text-green-700`}>{linha.atingindoSortimento}</div>
          <div className={`${num} ${linha.faltaSortimento > 0 ? 'text-red-700' : 'text-muted-foreground'}`}>{linha.faltaSortimento}</div>
        </div>
      </button>
      {aberta && children && <div className={recuo ? 'pl-3' : ''}>{children}</div>}
    </div>
  )
}
