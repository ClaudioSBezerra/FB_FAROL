---
title: 'Módulo Objetivos — Importação de Objetivos de Vendas'
type: 'feature'
created: '2026-04-29'
status: 'in-progress'
baseline_commit: 'ca81167b9e8e182f6007be1a6a443fab23073866'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O sistema não possui tabela nem interface para importar objetivos de vendas por RCA × Produto × Fornecedor, impossibilitando o acompanhamento de metas da equipe comercial por supervisor e por RCA.

**Approach:** Criar a tabela `objetivos_importados` (isolada por `empresa_id`) que recebe CSV de 12 colunas com valores de período anterior e corrente, 3 views de agregação (detalhe por produto, acumulado por fornecedor, acumulado por supervisor), endpoint de upload com seleção de tipo de período, e página de importação no novo módulo "Objetivos" do AppRail.

## Boundaries & Constraints

**Always:**
- Todos os dados isolados por `empresa_id` (mesmo padrão de `cadastros`).
- CSV separado por `;`; colunas mapeadas por **posição** (imune a renomeação): `[0]=CODSUPERVISOR`, `[1]=CODUSUR(cod_rca)`, `[2]=CODEPTO`, `[3]=DEPARTAMENTO`, `[4]=CODSEC`, `[5]=SECAO`, `[6]=CODFORNEC`, `[7]=FORNECEDOR`, `[8]=CODPROD`, `[9]=CODCLI(qtd_clientes)`, `[10]=vl_anterior`, `[11]=vl_corrente`.
- `tipo_periodo` ∈ {MENSAL, TRIMESTRAL, SEMESTRAL, ANUAL}; `periodo_seq` = 1–12 / 1–4 / 1–2 / 1 respectivamente.
- Upsert por chave composta `(empresa_id, tipo_periodo, ano, periodo_seq, cod_supervisor, cod_rca, cod_depto, cod_sec, cod_fornec, cod_prod)`; re-import substitui valores (ON CONFLICT DO UPDATE).
- SAVEPOINT por linha (mesmo padrão de `UploadCadastrosCSVHandler`).
- `CODCLI` = contagem de clientes ativos no período — armazenado como `qtd_clientes INTEGER`.
- `vl_anterior` = col[10] (valor período anterior); `vl_corrente` = col[11] (valor período corrente).
- 3 views criadas no banco: `vw_obj_rca_produto` (join sem GROUP BY extra), `vw_obj_rca_fornecedor` (agrupa por cod_rca × cod_fornec), `vw_obj_supervisor` (agrupa por cod_supervisor × cod_fornec, soma toda a equipe).
- Views fazem LEFT JOIN em `gestores` e `rcas` filtrando por `empresa_id`.
- Novo módulo "Objetivos" no AppRail: ícone `Target`, label "Objetivos", rota `/objetivos/importar`.
- Rotas registradas com `withSP(..., "gestor_filial")`.

**Ask First:**
- Se quiser tela de histórico de imports (lista de jobs com data/usuário/contagens).
- Se quiser validar que `cod_rca` e `cod_supervisor` existem nas tabelas de cadastro antes de importar (agora: aceita qualquer valor).

**Never:**
- Não dividir os valores por subperíodos — o arquivo já traz o valor integral do período escolhido.
- Não criar telas de visualização das views neste sprint — apenas importação.

## I/O & Edge-Case Matrix

| Cenário | Input | Esperado | Erro |
|---------|-------|----------|------|
| CSV válido, MENSAL jan/2025 | tipo=MENSAL, ano=2025, seq=1, arquivo com N linhas | N importados, views consultáveis | — |
| Re-import mesmo período | mesmo arquivo resubmetido | todos atualizados, zero duplicatas | — |
| Linha com < 12 colunas | registro incompleto | linha ignorada, `ignorados++` | — |
| vl_anterior ou vl_corrente não numérico | valor = "N/D" | linha ignorada | — |
| tipo_periodo inválido | tipo=BIMESTRAL | HTTP 400 "tipo_periodo inválido" | mensagem clara |
| periodo_seq fora do range | MENSAL com seq=13 | HTTP 400 "periodo_seq inválido para MENSAL (1–12)" | mensagem clara |

</frozen-after-approval>

## Code Map

- `backend/migrations/123_create_objetivos.sql` — tabela `objetivos_importados` + índices + 3 views
- `backend/handlers/objetivos.go` — `ObjetivosImportHandler` (upload CSV com SAVEPOINTs + validação de params)
- `backend/main.go` — rota `POST /api/objetivos/upload-csv` via `withSP`
- `frontend/src/lib/navigation.ts` — módulo `objetivos`
- `frontend/src/components/AppRail.tsx` — item Objetivos no `mainItems`
- `frontend/src/App.tsx` — rota ProtectedRoute `/objetivos/importar`
- `frontend/src/pages/ObjetivosImportar.tsx` — formulário de importação

## Tasks & Acceptance

**Execution:**
- [x] `backend/migrations/123_create_objetivos.sql` -- criar `objetivos_importados` com colunas (id bigserial PK, empresa_id UUID FK NOT NULL, tipo_periodo TEXT CHECK IN ('MENSAL','TRIMESTRAL','SEMESTRAL','ANUAL'), ano INT, periodo_seq INT, cod_supervisor INT, cod_rca INT NOT NULL, cod_depto TEXT, departamento TEXT, cod_sec TEXT, secao TEXT, cod_fornec TEXT NOT NULL, fornecedor TEXT, cod_prod TEXT NOT NULL, qtd_clientes INT DEFAULT 0, vl_anterior NUMERIC(15,2) DEFAULT 0, vl_corrente NUMERIC(15,2) DEFAULT 0, importado_em TIMESTAMPTZ DEFAULT NOW()); UNIQUE na chave composta; índices em (empresa_id), (empresa_id, cod_rca), (empresa_id, cod_supervisor), (empresa_id, cod_fornec); 3 views -- base de dados do módulo
- [x] `backend/handlers/objetivos.go` -- `ObjetivosImportHandler`: lê `tipo_periodo`, `ano`, `periodo_seq` dos query params; valida range de `periodo_seq` por tipo; parse CSV por posição (sep=`;`); SAVEPOINT por linha; upsert com ON CONFLICT (chave composta) DO UPDATE SET vl_anterior, vl_corrente, qtd_clientes, fornecedor, departamento, secao; retorna JSON `{importados, atualizados, ignorados}` -- toda lógica de import em arquivo dedicado
- [x] `backend/main.go` -- registrar `POST /api/objetivos/upload-csv` com `withSP(handlers.ObjetivosImportHandler, "gestor_filial")` após definição de `withSP` -- expor endpoint autenticado
- [x] `frontend/src/lib/navigation.ts` -- adicionar módulo `objetivos` (label "Objetivos", path "/objetivos/importar", sem adminOnly) -- visível a todos no AppRail
- [x] `frontend/src/components/AppRail.tsx` -- adicionar `{ id: 'objetivos', icon: Target, label: 'Objetivos', path: '/objetivos/importar' }` ao `mainItems` -- botão na barra lateral
- [x] `frontend/src/App.tsx` -- adicionar `<Route path="/objetivos/importar" element={<ProtectedRoute><ObjetivosImportar /></ProtectedRoute>} />` -- rota funcional
- [x] `frontend/src/pages/ObjetivosImportar.tsx` -- formulário: Select `tipo_periodo` (4 opções) + seletor de período dinâmico (mês+ano / trimestre+ano / semestre+ano / só ano) + input file CSV + botão Importar + card de resultado `{importados, atualizados, ignorados}` com mesmo estilo do card em `CadastroRCAs.tsx` -- interface de importação

**Acceptance Criteria:**
- Dado CSV válido com tipo=MENSAL/jan/2025, quando importado, então `SELECT COUNT(*) FROM objetivos_importados WHERE empresa_id=X AND tipo_periodo='MENSAL' AND ano=2025 AND periodo_seq=1` = número de linhas do CSV.
- Dado re-import do mesmo CSV sem alterações, quando importado, então zero duplicatas (COUNT permanece igual).
- Dado `tipo_periodo=BIMESTRAL`, quando POST, então HTTP 400.
- Dado `periodo_seq=13` com tipo MENSAL, quando POST, então HTTP 400.
- Dado import bem-sucedido, quando `SELECT * FROM vw_obj_rca_fornecedor WHERE empresa_id=X LIMIT 1`, então retorna linha com `nome_rca` preenchido e `vl_corrente = SUM` dos produtos daquele RCA × fornecedor.
- Dado import bem-sucedido, quando `SELECT * FROM vw_obj_supervisor WHERE empresa_id=X LIMIT 1`, então retorna linha com `qtd_rcas > 0` e `vl_corrente = SUM` de toda a equipe do supervisor.

## Design Notes

**Seletor de período dinâmico (frontend):**
```
MENSAL      → <Select mês Jan–Dez (1–12)> + <Input ano>
TRIMESTRAL  → <Select T1/T2/T3/T4 (1–4)> + <Input ano>
SEMESTRAL   → <Select S1/S2 (1–2)>        + <Input ano>
ANUAL       → <Input ano> apenas           (periodo_seq fixo = 1)
```

**Views — hierarquia de agregação:**
```
objetivos_importados  (granular: empresa × período × supervisor × RCA × depto × sec × fornecedor × produto)
  └─ vw_obj_rca_produto     LEFT JOIN gestores+rcas, sem GROUP BY extra (substitui nome_supervisor/nome_rca)
  └─ vw_obj_rca_fornecedor  GROUP BY (empresa_id, tipo_periodo, ano, periodo_seq, cod_supervisor, cod_rca, cod_fornec)
  └─ vw_obj_supervisor      GROUP BY (empresa_id, tipo_periodo, ano, periodo_seq, cod_supervisor, cod_fornec)
```

## Verification

**Commands:**
- `cd /home/claudio/projetos/FB_FAROL/backend && go build ./...` -- expected: sem erros
- `grep -r "objetivos" frontend/src/App.tsx frontend/src/lib/navigation.ts frontend/src/components/AppRail.tsx` -- expected: módulo e rota presentes

**Manual checks:**
- Upload de CSV de teste (tipo=MENSAL, jan/2025): confirmar `SELECT COUNT(*) FROM objetivos_importados` = número de linhas do CSV.
- `SELECT cod_rca, fornecedor, SUM(vl_corrente) FROM vw_obj_rca_fornecedor GROUP BY 1,2 LIMIT 5` deve retornar totais por RCA × fornecedor.
- `SELECT cod_supervisor, nome_supervisor, COUNT(DISTINCT cod_rca) FROM vw_obj_supervisor GROUP BY 1,2 LIMIT 5` deve mostrar equipe do supervisor agregada.

## Spec Change Log
