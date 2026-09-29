# Addendum — Painel de Gestão de Metas por Indústria

Conteúdo técnico/aprofundado que não cabe no corpo do PRD, mas foi levantado durante a conversa e serve pra arquitetura/épicos depois.

## Importação de Metas — mecanismo/transporte

- **Fase 1 (este PRD):** upload de CSV pelo admin, com os valores de meta por indústria/tipo de métrica/faixa/período.
- **Fase 2 (fora de escopo, registrado como direção futura):** integração direta com a base Oracle da JC como fonte de metas, substituindo o CSV manual. Ver memória de sessão `infra_dev_vm.md` — já existe um Oracle da JC alcançável a partir de produção (usado hoje pra outra integração, "Oracle da JC alcançável de produção via sonda manual do Claudio").

## Numérica — documento-fonte completo (addendum 2026-09-29)

Texto colado pelo usuário, documento "PROGRAMA ÚNICO – UNILEVER" (Word), reproduzido aqui na íntegra pra quem for desenhar arquitetura/stories não perder nenhum detalhe que não coube nos FRs do corpo do PRD.

### Temas gerais

- Fornecedores: 131 – Unilever Foods, 396 – Unilever HC. Resultados **independentes** entre os dois — o de um não interfere no outro (mesmo princípio já registrado no PRD original pra Cobertura/Sortimento por Rede).
- Escopo: UF GO, GGVs GO, GO FOOD, V7, DF, TELEVENDAS, SITE, BALCAO ANAPOLIS, BALCAO JH.
- "O desafio de cada métrica pode ser variável em cada mês" — reforça a necessidade de modelo configurável (já coberto por FR5 — vigência/faixas mês a mês).
- Hierarquia de navegação pedida: GGV, GGV/RCA, GGV/CRV/RCA, GGV/CRV/RCA/Cliente Rede, GGV/CRV/RCA/Cliente Rede/Cliente CNPJ. (Ver Questão em aberto #1 no corpo do PRD — o nível "Cliente Rede" parece conflitar com "Numérica são CNPJs individuais".)
- PPA = família de itens/EANs. EAN tributável = código de barras = item.

### Painel 1: Único Numéricas

- Apuração mensal.
- Lista de "Clientes Numéricas" pré-definida pelo fornecedor — CNPJs individuais, enviada mensalmente com GGV/CRV/RCA responsável.

### Métrica 1 — Cobertura das Numéricas (Positivação)

- 3 faixas de cliente: Num A (R$100), Num B (R$50), Num C (R$15) — valor mínimo de compra no bimestre móvel pra ser considerado coberto/positivado.
- **Não considera venda Tipo 5 (bonificada).**
- Apuração por Cliente/CNPJ no bimestre móvel. Exemplo do documento: "apuração de Setembro: considera Agosto e Setembro; apuração de Outubro: considera Outubro e Setembro" *(sic — o segundo exemplo do documento-fonte tem uma inconsistência: deveria ser "Setembro e Outubro" pra manter o padrão "mês corrente + mês anterior" do primeiro exemplo; tratado como erro de digitação do documento-fonte, não como regra alternativa)*.
- Exemplo 2 do documento: cliente Num.A precisa comprar R$100+ no bimestre móvel pra ser coberto.
- Meta agregada do distribuidor, em faixas: 4.870 lojas positivadas (Faixa 3), 5.681 lojas positivadas (Faixa 2, inferido pela posição — documento não rotula explicitamente), 6.493 lojas positivadas (Faixa 1). Ver Questão em aberto #3 — fornecedor não especificado nessa parte do texto.

### Métrica 2 — Sortimento (PPA Positivado)

- Só clientes Numérica A e B são apurados (C fica de fora).
- Tipos de venda considerados: 1, 9 e 5 (bonificado) — **inclui** bonificado, ao contrário da Cobertura.
- Também apurado no bimestre móvel.
- Lista de itens/EANs considerados = PPA (família de itens). Planilha mensal traz código interno (cod_prod), descrição, embalagem de venda, e a qual PPA cada item pertence.
- Cada loja precisa comprar, na faixa máxima, 15 PPAs diferentes no bimestre móvel. Exemplo do documento: "loja comprou 7 PPAs mês anterior + 8 PPAs esse mês = considera que a loja comprou 15 PPAs" (soma de PPAs distintos entre os 2 meses, não reinicia a cada mês). Segundo exemplo: comprar 2 itens diferentes (X em Agosto, Y em Setembro) da MESMA família (ex: Amaciantes) conta como **1 PPA só**, não 2.
- Regra de quantidade mínima pro item ser considerado positivado na loja: pelo menos 3 unidades pra produtos vendidos em "UN"; produtos vendidos em CX/Pacote/Display já contam automaticamente (1 unidade de venda já é maior que 3un implicitamente).
- Apuração total do distribuidor = **soma de PPAs vendidos por cliente ÷ quantidade de clientes COM COMPRA** (não ÷ todos os clientes da lista) — ver Questão em aberto #2, precisa confirmar se "com compra" é "comprou algum PPA" ou "teve qualquer venda no período".
- Metas agregadas do distribuidor, em faixas, por fornecedor:
  - 396-Unilever HC: 7,33 PPA (Faixa 3), 8,46 PPA (Faixa 2), 15 PPA (Faixa 1).
  - 131-Unilever Foods: 3,11 PPA (Faixa 3), 3,59 PPA (Faixa 2), 7 PPA (Faixa 1).

### Planilhas reais — estrutura de dados observada (2026-09-29)

Confirmado direto nos arquivos `Unico Acompanhamento numericas Unilever HC_29092026.xlsx` e `..._foods_29092026.xlsx` (10.754 linhas cada, mesmo CNPJs nos dois — a base de clientes é compartilhada entre os 2 fornecedores, só a apuração é independente):

- Aba `BASE LOJAS`: `CNPJ, CODCL, Classificação PDV, RAZAO, FANTASIA, GGV COD, GGV NOME, CRV COD, CRV NOME, RCA COD, RCA NOME` — sem `COD PRINC`.
- Aba `Resumo Numericas Cliente` (Foods) / `Resumo Rede Cliente` (HC, nome de aba desatualizado mas mesmo layout): `Classificação PDV, COD CL, RAZAO, FANTASIA, OBJETIVO COBERTURA, VALOR VENDA, FALTA (R$) COBERTURA, OBJETIVO PPA's, QT DE PPA's VENDIDOS, FALTA PPA's, COD GGV, NOME GGV, COD CRV, NOME CRV, COD RCA, NOME RCA`.
- Combinações reais de (Classificação, Objetivo Cobertura, Objetivo PPA) — idênticas nos 2 arquivos pro Objetivo Cobertura, diferentes pro Objetivo PPA:
  - Num. C → R$15 Cobertura / 7 PPA (Foods) ou 15 PPA (HC) — 8.985 clientes.
  - Num. B → R$50 Cobertura / 7 PPA (Foods) ou 15 PPA (HC) — 1.319 clientes.
  - Num. A → R$100 Cobertura / 7 PPA (Foods) ou 15 PPA (HC) — 450 clientes.
- Aba `BASE PPA's`: `Região do Sortimento, AE, BU, PPA, Descrição, EAN Regular, Cod JC, Embalagem` — 77 linhas (Foods), mapeando 15 PPAs distintos pra múltiplos EAN/cod_prod cada. Só 1 valor de "Região do Sortimento" observado ("Centro Norte") — ver Questão em aberto #4.
- Aba `mira`: `Data, Periodo, Periodo_Fechado_Aberto` — 1 linha, "28/08/2026, Agosto/2026, Parcial". Indica que os arquivos analisados são um snapshot de apuração de Agosto/2026, não Setembro.
