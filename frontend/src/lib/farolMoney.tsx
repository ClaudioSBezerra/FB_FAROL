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
