---
title: 'Módulo Cadastros — Gestores, RCAs e Relacionamento'
type: 'feature'
created: '2026-04-29'
status: 'in-review'
baseline_commit: '3b4f19a79434cf04bdd441cbd63a9d8cc6855d17'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O sistema não possui cadastro de Gestores (supervisores/GGVs), RCAs nem o vínculo entre eles, que são a base do Farol para gestão de metas por equipe e território.

**Approach:** Criar 3 tabelas (`gestores`, `rcas`, `gestor_rca`), com extração automática de `uf` (2 chars) e `regiao` do campo SUPERVISOR do CSV, campo calculado `atuacao = UF + " - " + REGIAO`, e módulo "Cadastros" no frontend com CRUD e upload de CSV (formato RCAS_ATIVOS).

## Boundaries & Constraints

**Always:**
- `gestores`: `cod_supervisor`, `nome` (texto original do CSV), `uf` (2 chars), `regiao` (texto), `atuacao` (gerado: `uf || ' - ' || regiao`), `ativo`.
- `rcas`: `cod_rca`, `nome`, `cod_filial`, `tipo`, `ativo`.
- `gestor_rca`: tabela de relacionamento (`cod_supervisor` FK, `cod_rca` FK) — UF e região acessíveis via join para ambos.
- Upload CSV faz upsert por `cod_supervisor`/`cod_rca` + recria vínculos em `gestor_rca`.
- Extração de `uf`: 2 chars antes do primeiro ` -` ou `- ` no campo SUPERVISOR.
- Extração de `regiao`: parte entre o separador de UF e o último separador `- ` ou ` - ` (ex: `"GO - CALDAS NOVAS - LILIAM"` → regiao=`CALDAS NOVAS`). Se não houver segundo separador, `regiao = NULL`.
- `atuacao` calculado na API: `uf || ' - ' || regiao` (omitido se um dos dois for NULL).
- Tipo RCA extraído do nome: `(CRV)`, `(JUR)`, `(GGV)`, prefixo `TELEV-`, default `RCA`.
- `ativo = false` quando nome do RCA contém `-SAIU`.
- Painel acessível a todos os usuários autenticados (`ProtectedRoute`, sem `adminOnly`).

**Ask First:**
- Se quiser paginação server-side (>500 registros por página).
- Se quiser extrair também o nome pessoal do gestor (parte após o último separador) como campo separado.

**Never:**
- Não apagar registros existentes no upload — apenas upsert e re-link vínculos.
- Não criar hierarquia de metas neste sprint — apenas cadastro base.

## I/O & Edge-Case Matrix

| Cenário | Input | Comportamento Esperado | Erro |
|---------|-------|------------------------|------|
| Upload CSV válido | `"GO - CALDAS NOVAS - LILIAM PEREIRA"` | gestor: uf=`GO`, regiao=`CALDAS NOVAS`, atuacao=`GO - CALDAS NOVAS` | — |
| Sem região no supervisor | `"TO - EDILSON GIL DA ROCHA"` (só 1 separador) | uf=`TO`, regiao=`NULL`, atuacao=`NULL` | — |
| Supervisor sem UF | `"BASE INATIVO SINTEGRA/RECEITA"` | uf=`NULL`, regiao=`NULL` | — |
| MARANHÃO | `"MARANHÃO- DIVAIR PIRES "` | uf=`MA`, regiao=`NULL` | — |
| V7 com UF | `"V7 - GO NORTE/METROP - WALISTON"` | uf=`GO`, regiao=`GO NORTE/METROP` | — |
| RCA `-SAIU` | `"ANDERSON SALES QUINTINO-SAIU"` | `ativo=false` | — |
| Campo CODFILIAL vazio | linha sem 5ª coluna | `cod_filial=NULL` | — |
| Deletar gestor com RCAs | DELETE `/api/cadastros/gestores/{id}` | 409 — vínculos existentes impedem exclusão | Mensagem clara |
| Re-upload CSV | mesmo arquivo | upsert sem duplicar, vínculos recriados | — |

</frozen-after-approval>

## Code Map

- `backend/migrations/121_create_cadastros.sql` — tabelas `gestores`, `rcas`, `gestor_rca`
- `backend/handlers/cadastros.go` — handlers CRUD (gestores + rcas) + upload CSV
- `backend/main.go` — rotas `/api/cadastros/*`
- `frontend/src/pages/CadastroGestores.tsx` — CRUD gestores
- `frontend/src/pages/CadastroRCAs.tsx` — CRUD RCAs + upload CSV
- `frontend/src/lib/navigation.ts` — módulo `cadastros` sem adminOnly
- `frontend/src/App.tsx` — rotas `/cadastros/*`

## Tasks & Acceptance

**Execution:**
- [x] `backend/migrations/121_create_cadastros.sql` -- criar `gestores` (cod_supervisor PK, nome, uf, regiao, ativo, timestamps), `rcas` (cod_rca PK, nome, cod_filial, tipo, ativo, timestamps), `gestor_rca` (cod_supervisor FK, cod_rca FK, PK composta) -- estrutura base do módulo
- [x] `backend/handlers/cadastros.go` -- implementar: CRUD gestores (list/create/update/delete), CRUD rcas (list/create/update/delete), `UploadCadastrosCSVHandler` (parse `;`, extração uf+regiao+tipo, upsert em transação, re-link gestor_rca). Campo `atuacao` como GENERATED ALWAYS STORED no PostgreSQL. -- toda lógica Cadastros em um arquivo
- [x] `backend/main.go` -- registrar `GET/POST /api/cadastros/gestores`, `PUT/DELETE /api/cadastros/gestores/{id}`, `GET/POST /api/cadastros/rcas`, `PUT/DELETE /api/cadastros/rcas/{id}`, `POST /api/cadastros/rcas/upload-csv` com `withAuth(..., "")` -- expor endpoints
- [x] `frontend/src/lib/navigation.ts` -- adicionar módulo `cadastros` (sem adminOnly), abas `Gestores` (`/cadastros/gestores`) e `RCAs` (`/cadastros/rcas`) -- visível a todos no AppRail
- [x] `frontend/src/App.tsx` -- rotas `ProtectedRoute` para `/cadastros/gestores` e `/cadastros/rcas` -- navegação funcional
- [x] `frontend/src/pages/CadastroGestores.tsx` -- tabela (Cód, Nome, UF, Região, Atuação, Qtd RCAs, Ativo) + busca + modal CRUD -- gestão de gestores
- [x] `frontend/src/pages/CadastroRCAs.tsx` -- tabela (Cód, Nome, Tipo, Filial, Gestor/Atuação, Ativo) + busca + filtros (UF, gestor, ativo) + modal CRUD + botão "Importar CSV" com exibição de resultado -- gestão de RCAs

**Acceptance Criteria:**
- Dado CSV com `"GO - CALDAS NOVAS - LILIAM PEREIRA"`, quando importado, então gestor tem `uf=GO`, `regiao=CALDAS NOVAS`, `atuacao=GO - CALDAS NOVAS`.
- Dado CSV com `"MARANHÃO- DIVAIR PIRES "`, quando importado, então `uf=MA`, `regiao=NULL`.
- Dado CSV com `"TO - EDILSON GIL DA ROCHA"` (sem segunda região), quando importado, então `uf=TO`, `regiao=NULL`.
- Dado `"V7 - GO NORTE/METROP - WALISTON"`, quando importado, então `uf=GO`.
- Dado RCA `"PEDRO HENRIQUE ALVES DE SOUSA-SAIU"`, quando importado, então `ativo=false`.
- Dado gestor com RCAs vinculados, quando DELETE, então retorna 409 sem apagar.
- Dado usuário autenticado (qualquer role), quando acessa `/cadastros/gestores`, então vê tabela carregada.
- Dado usuário autenticado, quando acessa `/cadastros/rcas`, então coluna `Atuação` exibe `uf - regiao` do gestor vinculado.

## Design Notes

**Extração uf/regiao do campo SUPERVISOR:**
```
1. Trim + ToUpper(campo)
2. "MARANHÃO"/"MARANHAO" no início → uf="MA", regiao=NULL
3. Pegar token antes do primeiro " - " ou "- " → candidato UF
4. Se len(candidato)==2 letras → uf=candidato; prefixo "V7 - " → ignorar e reaplicar na parte seguinte
5. Parte restante após separar UF: split pelo ÚLTIMO " - " ou "- "
   - Se tem 2 partes: parte esquerda (trim) → regiao
   - Se tem 1 parte: regiao=NULL (é o nome do gestor, não uma região)
6. atuacao = uf + " - " + regiao (apenas se ambos não-null)
```

**gestor_rca:** tabela simples, sem dados extras. Permite futuramente um RCA ter múltiplos gestores ou um gestor herdar região de outro.

## Verification

**Commands:**
- `cd /home/claudio/projetos/FB_FAROL/backend && go build ./...` -- expected: sem erros
- `grep -r "cadastros" frontend/src/App.tsx frontend/src/lib/navigation.ts` -- expected: rotas e módulo presentes

**Manual checks:**
- Upload RCAS_ATIVOS.csv: supervisor `100` deve ter `uf=GO`, `regiao=CALDAS NOVAS`, `atuacao=GO - CALDAS NOVAS`
- Supervisor `241` deve ter `uf=TO`, `regiao=NULL`
- RCA `5632` (`PEDRO HENRIQUE ALVES DE SOUSA-SAIU`) deve ter `ativo=false`

## Spec Change Log

- 2026-04-29: Revisão pós-elicitação — modelo alterado para 3 tabelas (`gestores`, `rcas`, `gestor_rca`); campo `atuacao` adicionado como `uf || ' - ' || regiao`; `nome` do gestor mantém texto original do CSV; extração de regiao refinada (parte entre UF e nome do gestor).
