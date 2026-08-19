// farolMoney — formatação de valores em R$ compartilhada entre as visões do
// Farol (cards, mobile, tabela executiva). Vive fora de FarolV2Dashboard.tsx
// pra FarolExecutivo.tsx poder importar sem criar dependência circular (V2Dashboard
// importa FarolExecutivo como sub-view).

export function fmtBRL(v: number) {
  if (v === 0) return '—'
  // Valores ABSOLUTOS (sem abreviar K/M/B) — decisão do gestor. Ex.: R$ 2.500,35
  return v.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL', minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

// BRLValue — mesmo texto do fmtBRL, mas com pontos de quebra explícitos
// (<wbr/>) depois de cada separador de milhar. Sem isso, valores ≥ R$ 1 bi
// (ex: "R$ 1.851.348.206,03") são uma única "palavra" pro navegador: o
// break-words (overflow-wrap) que protege contra sobreposição decide sozinho
// ONDE quebrar quando o texto não cabe, e escolhe o ponto mais tarde possível
// — que cai bem no meio dos centavos ("R$ 1.851.348.206," numa linha, "03"
// isolado na de baixo, visto em produção com zoom do navegador aumentado em
// 10/08/2026). Com <wbr/> só entre grupos de milhar, o navegador prefere
// esses pontos — nunca separa a vírgula decimal do resto do número — e só
// cai no break-words como último recurso se mesmo assim não couber.
// brlHeroClass — tamanho do valor "hero" conforme o COMPRIMENTO do número.
//
// No mobile o hero divide a linha com o % de atingimento, que fica fixo à
// direita (shrink-0). Com fonte de tamanho único, "R$ 10.000.000,00" — 16
// caracteres — é exatamente onde o valor deixa de caber num aparelho de ~400px,
// e aí o <wbr/> acima faz o que foi desenhado pra fazer: quebra num separador
// de milhar. Correto pro caso de R$ 1 bi que ele resolve, indesejado aqui.
//
// Reportado na visão Ano a Ano (19/08/2026), e faz sentido que apareça ali
// primeiro: o período atual é um ano inteiro, então os valores sobem uma ordem
// de grandeza em relação ao mês.
//
// Encolher todo mundo pra caber o caso grande tiraria o impacto do hero pro RCA
// em campo, que enxerga milhares — então o tamanho acompanha o texto: número
// curto continua grande, número longo desce só o necessário pra caber. As
// classes são literais porque o Tailwind lê o código-fonte pra decidir o que
// gerar; montar a string dinamicamente faria a classe não existir no CSS.
export function brlHeroClass(v: number, base: 'kpi' | 'card') {
  const n = fmtBRL(v).length
  if (base === 'kpi') {
    if (n <= 13) return 'text-[clamp(1.25rem,7vw,2.25rem)]'    // até R$ 999.999,99
    if (n <= 16) return 'text-[clamp(1.1rem,5.4vw,1.875rem)]'  // até R$ 99.999.999,99
    if (n <= 17) return 'text-[clamp(1rem,4.6vw,1.5rem)]'      // até R$ 999.999.999,99
    return 'text-[clamp(0.9rem,3.9vw,1.25rem)]'                // R$ 1 bi ou mais
  }
  if (n <= 13) return 'text-[clamp(1.1rem,6vw,1.875rem)]'
  if (n <= 16) return 'text-[clamp(1rem,4.8vw,1.5rem)]'
  if (n <= 17) return 'text-[clamp(0.95rem,4.1vw,1.375rem)]'
  return 'text-[clamp(0.85rem,3.5vw,1.125rem)]'
}

export function BRLValue({ v }: { v: number }) {
  const s = fmtBRL(v)
  if (s === '—') return <>{s}</>
  const groups = s.split('.')
  return (
    <>
      {groups.map((g, i) => (
        <span key={i}>
          {g}
          {i < groups.length - 1 && <>.<wbr /></>}
        </span>
      ))}
    </>
  )
}
