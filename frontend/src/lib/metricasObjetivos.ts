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

// ─── Resumo da equipe (mobile GGV/Supervisor) ───────────────────────────────
// Melhoria 1 do Heverton 08/10/2026: quem está puxando ou derrubando o número.
// Uma "unidade" é 1 Rede (Ponderada) ou 1 Cliente (Numérica).

export interface UnidadeEquipe {
  cod_crv: string
  nome_crv: string
  cod_rca: string
  nome_rca: string
  cobertura_atingiu: boolean
  // Ponderada: sempre true. Numérica: false pra Num. C (FR26).
  sortimento_aplicavel: boolean
  sortimento_valor: number
  sortimento_objetivo: number
  // Denominador da média de Sortimento. Ponderada: sempre true (média entre
  // todas as Redes). Numérica: comprou qualquer produto da indústria no
  // bimestre (Questão #2 do PRD), igual o cartão do topo.
  teve_compra: boolean
}

export interface LinhaEquipe {
  codigo: string
  nome: string
  qt: number
  cobertas: number
  falta: number
  objetivoSortimento: number
  realSortimento: number
  atingindoSortimento: number
  faltaSortimento: number
}

export function resumirEquipe(unidades: UnidadeEquipe[], por: 'crv' | 'rca'): LinhaEquipe[] {
  const grupos = new Map<string, { nome: string; us: UnidadeEquipe[] }>()
  for (const u of unidades) {
    const codigo = por === 'crv' ? u.cod_crv : u.cod_rca
    const nome = por === 'crv' ? u.nome_crv : u.nome_rca
    const g = grupos.get(codigo) ?? { nome, us: [] }
    g.us.push(u)
    grupos.set(codigo, g)
  }
  const linhas: LinhaEquipe[] = []
  for (const [codigo, g] of grupos) {
    const aplicaveis = g.us.filter(u => u.sortimento_aplicavel)
    const comCompra = aplicaveis.filter(u => u.teve_compra).length
    const atingindo = aplicaveis.filter(u => u.sortimento_valor >= u.sortimento_objetivo).length
    const cobertas = g.us.filter(u => u.cobertura_atingiu).length
    linhas.push({
      codigo,
      nome: g.nome,
      qt: g.us.length,
      cobertas,
      falta: g.us.length - cobertas,
      objetivoSortimento: aplicaveis.reduce((m, u) => Math.max(m, u.sortimento_objetivo), 0),
      realSortimento: comCompra > 0 ? aplicaveis.reduce((s, u) => s + u.sortimento_valor, 0) / comCompra : 0,
      atingindoSortimento: atingindo,
      faltaSortimento: aplicaveis.length - atingindo,
    })
  }
  // Maior buraco primeiro: é o que mais derruba o número.
  return linhas.sort((a, b) => b.falta - a.falta || a.nome.localeCompare(b.nome, 'pt-BR'))
}
