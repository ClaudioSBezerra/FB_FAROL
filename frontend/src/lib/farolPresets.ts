// farolPresets.ts — lógica de presets de período COMPARTILHADA entre o painel
// executivo (FarolExecutivo / FarolExecutivoV2) e o mobile (FarolPublicPanel).
// Cada preset produz intervalos EXPLÍCITOS (ref + comparativo do MESMO tamanho),
// evitando comparações incoerentes (ex: 1 mês × 5 meses).

export type Preset = 'mes_corrente' | 'yoy' | 'ant_corrente' | 'ytd' | 'dia_anterior'

export const PRESET_LABEL: Record<Preset, string> = {
  ytd:          'Ano × Ano',
  yoy:          'Último mês YoY',
  ant_corrente: 'M-1 vs M-2',
  mes_corrente: 'Mês Corrente',
  dia_anterior: 'Dia Anterior',
}

// Rótulos em linguagem simples para o app de campo (SUPV/RCA) — sem jargão.
export const PRESET_LABEL_MOBILE: Record<Preset, string> = {
  yoy:          'Mês vs Ano Passado',
  ytd:          'Acumulado do Ano',
  ant_corrente: 'Mês vs Mês Passado',
  mes_corrente: 'Mês Atual',
  dia_anterior: 'Dia Anterior',
}

// Ordem de exibição dos botões (esquerda → direita) — só o painel mobile usa
// (FarolExecutivo tem sua própria lista local). "Mês Atual" removido em
// 28/08/2026 a pedido do Heverton — os RCAs/SUPVs no campo não usavam esse
// recorte. "7 dias"/"30 dias" removidos do TIPO Preset inteiro em 08/09/2026
// (pedido do Claudio) — nenhuma versão (web/mobile) usa mais esses recortes.
export const PRESET_ORDER: Preset[] = ['dia_anterior', 'yoy', 'ytd', 'ant_corrente']

export interface PresetRange {
  ref_inicio: string
  ref_fim: string
  comp_inicio: string
  comp_fim: string
}

function ymd(y: number, m: number, d: number): string {
  return `${y}-${String(m).padStart(2, '0')}-${String(d).padStart(2, '0')}`
}
function lastDayOfMonth(y: number, m: number): number { return new Date(y, m, 0).getDate() }

// presetRange — replica EXATAMENTE a lógica do painel executivo.
//   hoje = último dia com dado REAL importado (YYYY-MM-DD), não o relógio do
//   navegador — a base pode estar 1+ dia atrasada (ex: hoje 09/09, base só
//   até 08/09). Vem de periodo.ultimo_dia_importado (backend, inferLastDay).
//   Se ainda não carregou (fetch inicial / empresa sem dado nenhum), cai pro
//   dia real do navegador como fallback.
//   "Último mês" (preset yoy) = último mês CALENDÁRIO 100% completo em
//   relação a `hoje` — só é o próprio mês de `hoje` quando `hoje` for o
//   último dia daquele mês; senão é o mês anterior.
export function presetRange(p: Preset, hoje?: string): PresetRange {
  const today = hoje && hoje.length > 0 ? hoje : (() => {
    const now = new Date()
    return ymd(now.getUTCFullYear(), now.getUTCMonth() + 1, now.getUTCDate())
  })()
  const [todayY, todayM, todayD] = today.split('-').map(Number)

  const hojeEhUltimoDiaDoMes = todayD === lastDayOfMonth(todayY, todayM)
  let lastY = todayY, lastM = todayM
  if (!hojeEhUltimoDiaDoMes) {
    lastM = todayM - 1
    if (lastM === 0) { lastM = 12; lastY-- }
  }

  switch (p) {
    case 'ytd': {
      // Jan até hoje do ano corrente × MESMO período (Jan até o mesmo dia) do
      // ano anterior — período vs período (era: ano anterior INTEIRO, jan-dez,
      // comparando faixas de tamanhos diferentes — ex. 8 meses × 12 meses).
      const dayCap = Math.min(todayD, lastDayOfMonth(todayY - 1, todayM))
      return {
        ref_inicio:  ymd(todayY, 1, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, 1, 1),
        comp_fim:    ymd(todayY - 1, todayM, dayCap),
      }
    }
    case 'yoy':
      // Último mês 100% importado × mesmo mês do ano anterior — ambos completos.
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(lastY - 1, lastM, 1),
        comp_fim:    ymd(lastY - 1, lastM, lastDayOfMonth(lastY - 1, lastM)),
      }
    case 'ant_corrente': {
      // Mês corrente até hoje × MESMO período do mês anterior, mesmo ano —
      // mês vs mês, calendário de hoje (era: "os 2 últimos meses 100%
      // importados" completos, sem relação com o dia corrente).
      let prevM = todayM - 1, prevY = todayY
      if (prevM === 0) { prevM = 12; prevY-- }
      const dayCap = Math.min(todayD, lastDayOfMonth(prevY, prevM))
      return {
        ref_inicio:  ymd(todayY, todayM, 1),
        ref_fim:     today,
        comp_inicio: ymd(prevY, prevM, 1),
        comp_fim:    ymd(prevY, prevM, dayCap),
      }
    }
    case 'mes_corrente': {
      // Dia 1 até hoje do mês corrente × MESMO período do ANO ANTERIOR.
      const dayCap = Math.min(todayD, lastDayOfMonth(todayY - 1, todayM))
      return {
        ref_inicio:  ymd(todayY, todayM, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, todayM, 1),
        comp_fim:    ymd(todayY - 1, todayM, dayCap),
      }
    }
    case 'dia_anterior':
    default: {
      // Último dia com dado real importado × mesmo dia do ANO ANTERIOR.
      // `today` JÁ É esse último dia importado (não o dia do relógio) — não
      // subtrai mais 1 aqui, senão fica um dia atrasado do que a base tem.
      const [oy, om, od] = today.split('-').map(Number)
      const dayCap = Math.min(od, lastDayOfMonth(oy - 1, om))
      const anoAnterior = ymd(oy - 1, om, dayCap)
      return { ref_inicio: today, ref_fim: today, comp_inicio: anoAnterior, comp_fim: anoAnterior }
    }
  }
}
