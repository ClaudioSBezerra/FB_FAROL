// farolPresets.ts — lógica de presets de período COMPARTILHADA entre o painel
// executivo (FarolExecutivo / FarolExecutivoV2) e o mobile (FarolPublicPanel).
// Cada preset produz intervalos EXPLÍCITOS (ref + comparativo do MESMO tamanho),
// evitando comparações incoerentes (ex: 1 mês × 5 meses).

export type Preset = 'mes_corrente' | 'yoy' | 'ant_corrente' | 'ytd' | 'last7' | 'last30'

export const PRESET_LABEL: Record<Preset, string> = {
  ytd:          'Ano × Ano',
  yoy:          'Último mês YoY',
  ant_corrente: 'M-1 vs M-2',
  mes_corrente: 'Mês Corrente',
  last7:        '7 dias',
  last30:       '30 dias',
}

// Ordem de exibição dos botões (esquerda → direita).
export const PRESET_ORDER: Preset[] = ['yoy', 'ytd', 'ant_corrente', 'mes_corrente', 'last7', 'last30']

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
function addDays(s: string, days: number): string {
  const [y, m, d] = s.split('-').map(Number)
  const dt = new Date(Date.UTC(y, m - 1, d))
  dt.setUTCDate(dt.getUTCDate() + days)
  return dt.toISOString().slice(0, 10)
}

// presetRange — replica EXATAMENTE a lógica do painel executivo.
//   last = último mês com dados (ano, mes). Usado pelos presets baseados no
//   "último mês importado" (yoy, ant_corrente). Os baseados no calendário
//   (ytd, mes_corrente, last7, last30) usam a data de hoje.
export function presetRange(p: Preset, last?: { ano: number; mes: number }): PresetRange {
  const now = new Date()
  const todayY = now.getUTCFullYear()
  const todayM = now.getUTCMonth() + 1
  const todayD = now.getUTCDate()
  const today = ymd(todayY, todayM, todayD)

  const lastY = last?.ano ?? todayY
  const lastM = last?.mes ?? (todayM > 1 ? todayM - 1 : 12)

  switch (p) {
    case 'ytd':
      return {
        ref_inicio:  ymd(todayY, 1, 1),
        ref_fim:     today,
        comp_inicio: ymd(todayY - 1, 1, 1),
        comp_fim:    ymd(todayY - 1, 12, 31),
      }
    case 'yoy':
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(lastY - 1, lastM, 1),
        comp_fim:    ymd(lastY - 1, lastM, lastDayOfMonth(lastY - 1, lastM)),
      }
    case 'ant_corrente': {
      let prevM = lastM - 1, prevY = lastY
      if (prevM === 0) { prevM = 12; prevY-- }
      return {
        ref_inicio:  ymd(lastY, lastM, 1),
        ref_fim:     ymd(lastY, lastM, lastDayOfMonth(lastY, lastM)),
        comp_inicio: ymd(prevY, prevM, 1),
        comp_fim:    ymd(prevY, prevM, lastDayOfMonth(prevY, prevM)),
      }
    }
    case 'mes_corrente': {
      let pm = todayM - 1, py = todayY
      if (pm === 0) { pm = 12; py-- }
      const dayCap = Math.min(todayD, lastDayOfMonth(py, pm))
      return {
        ref_inicio:  ymd(todayY, todayM, 1),
        ref_fim:     today,
        comp_inicio: ymd(py, pm, 1),
        comp_fim:    ymd(py, pm, dayCap),
      }
    }
    case 'last7': {
      const fim = today
      const ini = addDays(fim, -6)
      return { ref_inicio: ini, ref_fim: fim, comp_inicio: addDays(ini, -7), comp_fim: addDays(fim, -7) }
    }
    case 'last30':
    default: {
      const fim = today
      const ini = addDays(fim, -29)
      return { ref_inicio: ini, ref_fim: fim, comp_inicio: addDays(ini, -30), comp_fim: addDays(fim, -30) }
    }
  }
}
