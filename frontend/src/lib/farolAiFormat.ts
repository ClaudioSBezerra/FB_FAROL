// farolAiFormat — classificação e formatação das colunas que o Assistente
// recebe de volta do banco.
//
// O problema central: o SQL é escrito pela IA, então não existe schema
// conhecido. Tudo que temos é o ALIAS da coluna e o valor em texto (NUMERIC
// chega do Postgres como string). Toda decisão de formato e de gráfico sai daí.

import { fmtBRL } from './farolMoney'

export type ColKind = 'moeda' | 'contagem' | 'percentual' | 'periodo' | 'codigo' | 'texto'

// classifyCol — a ORDEM dos testes é a regra, não detalhe de implementação.
// Código e período vêm antes de dinheiro porque "tipo_venda" e "cod_vendedor"
// casariam com o padrão monetário por conterem "venda".
export function classifyCol(col: string): ColKind {
  const c = col.toLowerCase()
  if (/(pct|percent|taxa|_perc)/.test(c)) return 'percentual'
  if (/^(ano|mes|id)$/.test(c)) return 'periodo'
  if (/(cod_|_cod|codigo|cnpj|ean|tipo_)/.test(c)) return 'codigo'
  if (/(^qt$|qtd|quantid|positivad|clientes|base_cli|^mix$|pedidos|notas|itens)/.test(c)) return 'contagem'
  if (/(fatur|venda|valor|ticket|transmitid|liquid|bruto|bonific|transfer|remessa|devol|cancel|receita|custo|lucro|verba|total|saldo)/.test(c)) return 'moeda'
  return 'texto'
}

export function parseNum(v: unknown): number | null {
  if (v === null || v === undefined) return null
  const n = parseFloat(String(v))
  return isNaN(n) ? null : n
}

// fmtCell — valor de célula na tabela. Moeda sai ABSOLUTA, com centavos, igual
// ao resto do Farol (decisão do gestor registrada em farolMoney).
export function fmtCell(value: unknown, col: string): string {
  if (value === null || value === undefined) return '—'
  const s = String(value)
  const kind = classifyCol(col)
  const n = parseNum(value)

  if (kind === 'percentual' && n !== null) return n.toFixed(1).replace('.', ',') + '%'
  if (kind === 'periodo' || kind === 'codigo') return s
  if (kind === 'contagem' && n !== null) return n.toLocaleString('pt-BR', { maximumFractionDigits: 2 })
  if (kind === 'moeda' && n !== null) return fmtBRL(n)
  return s
}

// fmtEixo — rótulo de EIXO, onde o valor absoluto não cabe. "R$ 173.859.219,92"
// ocupa a largura de meio gráfico; num eixo o número é escala, não leitura
// precisa — quem quer o centavo lê o tooltip ou a tabela. Por isso aqui, e só
// aqui, abreviamos.
export function fmtEixo(v: number, kind: ColKind): string {
  if (kind === 'percentual') return v.toFixed(0) + '%'
  const abs = Math.abs(v)
  const pre = kind === 'moeda' ? 'R$ ' : ''
  if (abs >= 1_000_000_000) return pre + (v / 1_000_000_000).toLocaleString('pt-BR', { maximumFractionDigits: 1 }) + ' bi'
  if (abs >= 1_000_000) return pre + (v / 1_000_000).toLocaleString('pt-BR', { maximumFractionDigits: 1 }) + ' mi'
  if (abs >= 1_000) return pre + (v / 1_000).toLocaleString('pt-BR', { maximumFractionDigits: 0 }) + ' mil'
  return pre + v.toLocaleString('pt-BR', { maximumFractionDigits: 0 })
}

// ─── Decisão do gráfico ──────────────────────────────────────────────────────

export interface ChartSpec {
  forma: 'barra' | 'linha'
  labelCol: string          // dimensão no eixo de categoria
  medidas: string[]         // colunas numéricas oferecidas (a 1ª é a padrão)
  periodo?: { ano?: string; mes?: string }
}

// buildChartSpec — decide SE cabe gráfico e QUAL. Devolve null quando não cabe;
// tabela sozinha é resposta legítima e melhor que um gráfico forçado.
//
// Regras:
//   • precisa de exatamente uma dimensão de rótulo e ao menos uma medida
//   • ano/mes presentes ⇒ série temporal ⇒ linha
//   • senão ⇒ ranking ⇒ barra horizontal (nome de cliente/RCA é longo demais
//     pra caber embaixo de uma barra vertical)
//   • 1 linha só não vira gráfico — é um número, e número se lê na tabela
export function buildChartSpec(columns: string[], rows: Record<string, unknown>[]): ChartSpec | null {
  if (rows.length < 2) return null

  const medidas: string[] = []
  const rotulos: string[] = []
  let colAno: string | undefined
  let colMes: string | undefined

  for (const col of columns) {
    const kind = classifyCol(col)
    const c = col.toLowerCase()
    if (c === 'ano') { colAno = col; continue }
    if (c === 'mes') { colMes = col; continue }

    if (kind === 'moeda' || kind === 'contagem' || kind === 'percentual') {
      // Só conta como medida se a maioria das linhas realmente tiver número —
      // um alias monetário sobre coluna de texto não vira eixo.
      const comNumero = rows.filter(r => parseNum(r[col]) !== null).length
      if (comNumero >= rows.length * 0.8) { medidas.push(col); continue }
    }
    if (kind === 'texto') rotulos.push(col)
  }

  if (medidas.length === 0) return null

  if (colAno || colMes) {
    return {
      forma: 'linha',
      labelCol: colMes ?? colAno!,
      medidas,
      periodo: { ano: colAno, mes: colMes },
    }
  }

  if (rotulos.length === 0) return null
  return { forma: 'barra', labelCol: rotulos[0], medidas }
}
