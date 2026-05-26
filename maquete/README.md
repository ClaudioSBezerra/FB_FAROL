# FAROL — Maquete v0

Protótipo HTML standalone do novo Farol. **Sem backend, sem build, sem auth.** Tudo roda no browser. Importação de CSV via FileReader/papaparse, processamento client-side em JavaScript puro.

Objetivo: o gestor experimentar o painel ANTES de gastar qualquer linha de Go.

---

## Como abrir

### Opção 1 — Abrir direto no Chrome

1. Duplo clique em [`index.html`](./index.html)
2. (Se quiser usar os dados de exemplo, prefira a Opção 2 — alguns browsers bloqueiam `fetch()` em `file://`)

### Opção 2 — Servidor HTTP local (recomendado)

```bash
cd maquete
python3 -m http.server 8000
# ou: npx http-server -p 8000
```

Abre [http://localhost:8000](http://localhost:8000).

### Opção 3 — Vite (se já tem o frontend rodando)

```bash
cd frontend
npm run dev
```

E acessa [http://localhost:3087/../maquete/](http://localhost:3087/../maquete/) — não funciona, melhor usar a Opção 2.

---

## Como usar

1. **Carregar dados de exemplo** (botão azul) — usa os CSVs em `sample-data/`
   OU
   **Selecionar 2 CSVs próprios** — um para Base Comparativa, um para Base Atual

2. Escolher a **Visão** nas abas:
   - **Por Fornecedor (V01)** — começa por Fornec → drill por GGV → Supv → RCA → Cliente → Produto
   - **Por RCA (V02)** — começa por Supv → drill por RCA → Fornec → Cliente → Produto
   - **Diretoria (V03)** — começa por Fornec → Empresa → UF → GGV → Supv → RCA → Cliente → Produto

3. **Clicar nos cards** para fazer drill-down. O breadcrumb mostra onde está e permite voltar clicando em qualquer nível.

4. **Cards mostram:**
   - Semáforo binário 🟢 Verde (≥100%) / 🔴 Vermelho (<100%)
   - % de atingimento (Atual / Comparativa)
   - Valor Anterior e Valor Atual
   - 👥 Positivação (clientes que compraram vs base do RCA)
   - 📦 Mix (média de itens diferentes por cliente)
   - Faturado / Transmitido (separados)

---

## Layout do CSV

Separador: `;` (ponto-e-vírgula). Cabeçalho na primeira linha.

| Campo | Descrição |
|---|---|
| PERIODO | Período da apuração + estado (ex: `2026-T1` ou `2026-T1 FATURADO`) |
| CODGERENTE / GERENTE | Código e nome do Gerente |
| CODSUPERVISOR / SUPERVISOR | Código e nome do Supervisor |
| QTRCA_SUPERVISOR | Qtd de RCAs sob o supervisor |
| CODUSUR / RCA | Código e nome do RCA |
| QTCLI_RCA | Qtd de clientes na base do RCA |
| CODFORNEC / FORNECEDOR | Código e razão do fornecedor |
| CODCLI / CLIENTE / CNPJ | Código, razão e CNPJ do cliente |
| CODRAMO / RAMO | Ramo de atividade do cliente |
| UF | Estado do cliente |
| EMPRESA | Nome da empresa do grupo (necessário para Visão 03 Diretoria) |
| CODPROD / PRODUTO / EMBALAGEM | Código, descrição e embalagem |
| QTUNIT / QTUNITCX | Qtd unitária e qtd na caixa master |
| EAN | Código de barras (DUN da caixa master) |
| QT | Quantidade vendida |
| PVENDA | Valor vendido (formato brasileiro: `3600,00`) |
| ESTADO | `FATURADO` ou `TRANSMITIDO` |

**Cliente sem venda:** inclua a linha com `QT=0` e `PVENDA=0`. Isso é essencial para o cálculo correto de positivação.

---

## O que esta maquete NÃO faz

- Não persiste dados — recarregar a página perde tudo
- Não tem login / personas com escopo automático (todas as 3 visões aparecem sempre)
- Não tem sparkline, YoY, forecast ou ranking (são features da Fase 4)
- Não usa o backend Go (intencional — é maquete)
- Trava mínima de positivação está fixa em 1 unidade

---

## Próximos passos (após aprovação do gestor)

Ver [`../.planning/ROADMAP.md`](../.planning/ROADMAP.md) — após esta maquete ser aprovada, partimos para:

- **Fase 1:** schema novo + endpoints de importação
- **Fase 2:** materialized views + endpoints de leitura
- **Fase 3:** Frontend React substituindo essa maquete, com auth por persona
- **Fase 4:** features avançadas (sparkline, YoY, forecast, ranking, multi-período)
