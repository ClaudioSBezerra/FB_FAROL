---
title: 'Comparativo REL 322 x Farol — exportação em PDF'
type: 'feature'
created: '2026-08-27'
status: 'done'
review_loop_iteration: 0
context: []
baseline_commit: '9cfef7799ab2dd8e587ab29022829d709406e560'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O comparativo REL 322 x Farol (spec-comparativo-rel322.md, em produção) só existe na tela — sem persistência, o gestor não tem como levar o resultado pra fora (e-mail, arquivo) depois de fechar a aba.

**Approach:** Acrescentar `?formato=pdf` ao endpoint existente (mesmo padrão de `/relatorio/clientes-receita`), reprocessando o mesmo upload no mesmo request e devolvendo um PDF do **resultado** do comparativo (não do PDF do WinThor enviado), seguindo a estrutura já existente em `relatorioReceitaPDF`. Botão "Baixar PDF" na tela atual.

## Boundaries & Constraints

**Always:**
- `?formato=pdf` reprocessa o mesmo upload no mesmo request (sem persistência, consistente com o resto da feature) e devolve exatamente os mesmos dados — já recortados por escopo de persona — que `formato=json` (padrão) devolveria pro mesmo upload.
- PDF segue a estrutura de `relatorioReceitaPDF`: logo do inquilino via `logoRelatorio(db, empresaID)`, cabeçalho com o período do PDF de origem, tabela com `cabeNaColuna`/`fonteCelula` (evita a sobreposição de linha já corrigida noutro relatório), rodapé explicando a tolerância de 0,5%.
- Colunas: Código, Supervisor, Vl.Vendido (PDF), Bruto (Farol), Líquido (Farol), % dif., Status — status em texto (OK/DIVERGÊNCIA/ÓRFÃ), o PDF não tem o selo colorido da tela.
- Nome do arquivo baixado inclui o período do PDF de origem.
- Quando `sem_dado_farol_no_periodo` é true, o PDF ainda é gerado, com a mesma ressalva textual no topo que o relatório de receita usa pra cobertura parcial — nunca vira erro.

**Never:**
- Não gera PDF do relatório do WinThor enviado — só do resultado do comparativo.
- Não muda o contrato de `formato=json` (padrão) já em produção.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| PDF do comparativo normal | upload válido + `formato=pdf` | download com todas as linhas e status, mesmos dados do JSON | N/A |
| Sem dado do Farol no período | período sem import + `formato=pdf` | PDF gerado com ressalva no topo, Bruto/Líquido = 0 | N/A |
| Falha na geração do PDF (maroto) | erro interno do gerador | erro claro, mesmo padrão do XLSX/PDF do relatório de receita | HTTP 500 com JSON de erro |

</frozen-after-approval>

## Code Map

- `backend/handlers/farol_cnpj_receita.go:539-626` (`relatorioReceitaPDF`) -- template exato de estrutura de PDF a replicar (cabeçalho, ressalva, tabela, rodapé)
- `backend/handlers/farol_cnpj_receita.go:60-80` (`logoRelatorio`) -- reaproveitar direto, mesmo pacote `handlers`
- `backend/handlers/farol_cnpj_receita.go:688-749` (`cabeNaColuna`, `cabeNaColunaReserva`, `fonteCelula`, `moedaBR`) -- reaproveitar sem duplicar
- `backend/handlers/farol_comparativo_rel322.go:529-610` (`ComparativoRel322Handler`) -- onde entra o dispatch por `?formato=`
- `frontend/src/pages/farol/FarolComparativoRel322.tsx:68-115` (componente + função `enviar`) -- acrescentar botão "Baixar PDF"

## Tasks & Acceptance

**Execution:**
- [x] `backend/handlers/farol_comparativo_rel322.go` -- nova função `comparativoRel322PDF(resultado *comparativoRel322Resposta, logo []byte, ext extension.Type) ([]byte, error)`, reaproveitando `cabeNaColuna`/`moedaBR`/`fonteCelula`/maroto já usados em `farol_cnpj_receita.go` -- gera o PDF do resultado
- [x] `backend/handlers/farol_comparativo_rel322.go` -- `ComparativoRel322Handler` lê `formato := r.URL.Query().Get("formato")`, despacha `pdf` vs `json` (default `json`, mesmo padrão do relatório de receita); chama `logoRelatorio(db, spCtx.EmpresaID)` só quando `pdf` -- expõe o novo formato sem quebrar o atual
- [x] `frontend/src/pages/farol/FarolComparativoRel322.tsx` -- botão "Baixar PDF" que reenvia o MESMO arquivo já selecionado com `?formato=pdf`, baixa o blob com nome incluindo o período -- UI
- [x] Teste cobrindo a I/O Matrix: PDF gerado sem erro para um comparativo normal e para `sem_dado_farol_no_periodo=true`

**Acceptance Criteria:**
- Given um comparativo processado com sucesso, when o usuário pede `formato=pdf`, then recebe um PDF com as mesmas linhas/status do JSON equivalente, com logo do inquilino e sem sobreposição de linha.
- Given `sem_dado_farol_no_periodo=true`, when `formato=pdf`, then o PDF inclui a ressalva no topo em vez de virar erro.

## Design Notes

`cabeNaColuna`, `fonteCelula`, `moedaBR`, `logoRelatorio` já são funções/constantes do pacote `handlers` — só chamar, não reimplementar. O comparativo tem no máximo ~50-90 linhas, bem abaixo do volume que `relatorioReceitaPDF` já pagina automaticamente (milhares de clientes) — sem paginação manual nova.

## Verification

**Commands:**
- `cd backend && go build ./...` -- compila sem erro
- `cd backend && go test ./handlers/... -run ComparativoRel322` -- inclui os novos testes de PDF

**Manual checks:**
- Upload de um dos PDFs de exemplo em `/home/claudio/uploads/*.pdf` com `?formato=pdf` e abrir o arquivo baixado, conferindo logo, tabela e ressalva quando aplicável.

## Suggested Review Order

**Geração do PDF**

- `comparativoRel322PDF` — a função nova: mesma estrutura de `relatorioReceitaPDF` (cabeçalho, ressalva PARCIAL, tabela, rodapé), reaproveitando `cabeNaColuna`/`moedaBR`/`fonteCelula` sem duplicar.
  [`farol_comparativo_rel322.go:657`](../../backend/handlers/farol_comparativo_rel322.go#L657)

- `pctOuTracoRel322` — defesa em profundidade contra `+Inf`/`NaN` no PDF (achado dos 3 revisores; a causa raiz já era filtrada em `cruzarComRel322`, isto é o cinto e suspensório).
  [`farol_comparativo_rel322.go:750`](../../backend/handlers/farol_comparativo_rel322.go#L750)

- `valorOuTracoRel322` / `statusTextoRel322` — "—" para campo ausente em vez de `R$ 0,00` falso; status em texto (sem o selo colorido da tela).
  [`farol_comparativo_rel322.go:737`](../../backend/handlers/farol_comparativo_rel322.go#L737)

**Dispatch por formato no handler**

- `ComparativoRel322Handler` — `?formato=pdf` gera e devolve o PDF (com log se a escrita falhar); ausência do parâmetro mantém o contrato JSON intacto.
  [`farol_comparativo_rel322.go:540`](../../backend/handlers/farol_comparativo_rel322.go#L540)

**Frontend — botão de download**

- `baixarPDF` — reenvia o mesmo arquivo com `?formato=pdf`; download robusto (anexa ao DOM, `blob.size` checado, revoke adiado) depois do achado de 2 revisores sobre o padrão anterior.
  [`FarolComparativoRel322.tsx:110`](../../frontend/src/pages/farol/FarolComparativoRel322.tsx#L110)

**Testes — a parte que a revisão mais reforçou**

- `TestComparativoRel322_Handler_FormatoPDF` / `_FormatoJSONPadrao` — únicos testes que passam pela rota HTTP de verdade (multipart real); sem eles, trocar o parâmetro ou vazar o Content-Type não quebrava nada.
  [`farol_comparativo_rel322_test.go:923`](../../backend/handlers/farol_comparativo_rel322_test.go#L923)

- `TestComparativoRel322_PDF_Normal` / `_SemDadoNoPeriodo` — agora extraem o texto do PDF gerado (via `extrairTextoPDF`) e checam o conteúdo real, não só "gerou sem erro".
  [`farol_comparativo_rel322_test.go:681`](../../backend/handlers/farol_comparativo_rel322_test.go#L681)

- `TestComparativoRel322_PctOuTraco_InfNaN` — cobre a defesa em profundidade do item acima.
  [`farol_comparativo_rel322_test.go:825`](../../backend/handlers/farol_comparativo_rel322_test.go#L825)
