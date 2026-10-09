// Métricas do Painel de Objetivos por Indústria — rótulos e resumos
// compartilhados entre o painel web (FarolPainelMetas) e o mobile
// (FarolPublicMetasPanel). Pedido do Heverton 08/10/2026 ("Ajustes IA").

export type MetricaPainel =
  | 'combinado' | 'cobertura' | 'sortimento'
  | 'combinado_numerica' | 'cobertura_numerica' | 'sortimento_numerica'

export interface VinculosDaIndustria {
  cobertura: boolean
  sortimento: boolean
  cobertura_numerica: boolean
  sortimento_numerica: boolean
}

// Só a visão combinada de cada família (Ponderada e Numérica) — as
// independentes saíram do painel. Só voltam como alternativa se a indústria
// não tem o par completo (ex: só Cobertura cadastrada), pra não ficar sem
// nenhuma opção de métrica.
export function opcoesMetrica(v: VinculosDaIndustria): Array<{ value: MetricaPainel; label: string }> {
  const out: Array<{ value: MetricaPainel; label: string }> = []
  if (v.cobertura && v.sortimento) {
    out.push({ value: 'combinado', label: 'Ponderada (Cobertura + Sortimento)' })
  } else {
    if (v.cobertura) out.push({ value: 'cobertura', label: 'Ponderada Cobertura' })
    if (v.sortimento) out.push({ value: 'sortimento', label: 'Ponderada Sortimento' })
  }
  if (v.cobertura_numerica && v.sortimento_numerica) {
    out.push({ value: 'combinado_numerica', label: 'Numérica (Cobertura + Sortimento)' })
  } else {
    if (v.cobertura_numerica) out.push({ value: 'cobertura_numerica', label: 'Numérica Cobertura' })
    if (v.sortimento_numerica) out.push({ value: 'sortimento_numerica', label: 'Numérica Sortimento' })
  }
  return out
}

interface ClienteNumResumo {
  classificacao_pdv: string
  cobertura_objetivo: number
  cobertura_atingiu: boolean
  sortimento_valor: number
  sortimento_objetivo: number
  sortimento_aplicavel: boolean
}

// Resumo dos cartões da Numérica: objetivo de cobertura por classe, objetivo
// de sortimento e quantos clientes bateram. Calculado sobre a lista que a
// tela está mostrando, então acompanha os filtros.
export function resumoNumerica(clientes: ClienteNumResumo[]) {
  const porClasse = new Map<string, number>()
  for (const c of clientes) {
    const k = (c.classificacao_pdv || '').trim() || 'Sem classe'
    if (!porClasse.has(k)) porClasse.set(k, c.cobertura_objetivo)
  }
  const classes = [...porClasse.entries()].sort((a, b) => a[0].localeCompare(b[0], 'pt-BR'))
  const aplicaveis = clientes.filter(c => c.sortimento_aplicavel)
  const objetivoSortimento = aplicaveis.reduce((m, c) => Math.max(m, c.sortimento_objetivo), 0)
  return {
    classes,
    cobertos: clientes.filter(c => c.cobertura_atingiu).length,
    totalClientes: clientes.length,
    objetivoSortimento,
    bateramSortimento: aplicaveis.filter(c => c.sortimento_valor >= c.sortimento_objetivo).length,
    totalAplicaveis: aplicaveis.length,
  }
}
