---
task: adequacao-novo-layout
status: aguardando-testes-end-to-end
created: 2026-07-01
---

# HANDOFF — Testes end-to-end da Fase 1 + Fase 2

**Contexto**: Fase 1 (import estendido) e Fase 2 (visões V06/V07) foram
implementadas, testadas em unit level (build + migrations locais + smoke test
das funções PL/pgSQL) e commitadas em `main`. Os commits são `7dd3e56` (Fase 1)
e `530fc9d` (Fase 2). Falta o **teste end-to-end em produção** após o Coolify
redeployar.

Este documento é o roteiro pra retomar depois do almoço.

---

## Passo 1 — Confirmar que o Coolify deployou

Após uns 3-5 minutos do último push, o container do FB_FAROL em produção
deve estar rodando com o novo binário Go + novos assets front.

```bash
# HEAD local esperado:
git log --oneline -1
# → 530fc9d feat(farol): novas visões V06 Por Rede e V07 Por Departamento — Fase 2

# No servidor Hostinger:
NEW=$(docker ps --format '{{.Names}}' | grep -E '^api-a0wcggw4wo040gwwwokckgk8')
echo "Container atual: $NEW"
docker inspect "$NEW" --format '{{.Created}}'   # → deve ser após 01/07 ~22h BRT (post-Fase 2)
```

Se o container ainda for da Fase 1 ou anterior, **aguardar mais uns minutos**
ou disparar redeploy manual no painel do Coolify.

---

## Passo 2 — Confirmar que as migrations rodaram

O backend Go aplica migrations pendentes no startup. Deve ter aplicado 181-185.

```bash
docker exec "$NEW" wget -qO- http://localhost:8087/api/health | head -c 200
# → JSON com status: running, database: connected

# Descobrir container do DB e conferir migrations rodadas:
DBC=$(docker ps --format '{{.Names}}' | grep -E '^db-a0wcggw4wo040gwwwokckgk8')
docker exec "$DBC" psql -U postgres -d fb_farol -c "
SELECT
  (SELECT COUNT(*) FROM information_schema.columns
     WHERE table_name='vendas_faturadas' AND column_name='cod_cliprinc')
    AS fase1_cliprinc,
  (SELECT COUNT(*) FROM information_schema.tables
     WHERE table_schema='public' AND table_name='vendas_ccd')
    AS fase1_ccd,
  (SELECT COUNT(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
     WHERE n.nspname='farol' AND proname='upsert_aggs_mes_v06')
    AS fase2_v06,
  (SELECT COUNT(*) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
     WHERE n.nspname='farol' AND proname='upsert_aggs_mes_v07')
    AS fase2_v07;
"
# Esperado: 1  1  1  1
```

Se algum vier 0, migration falhou — olhar logs do backend por erro SQL.

---

## Passo 3 — Reimportar 1 mês no novo layout

Precisa de um CSV do ION VENDAS **no novo formato** (com colunas `CODEPTO`,
`CODCLIPRINC`, `PVENDA_TOTAL`, etc.). Sugestão: começar por `JUN_2026_TF.csv`
que é o mês mais recente.

1. Login como usuário com perfil TI ou admin (só esses veem "Painel Importação")
2. Sobe o CSV pela interface
3. Aguarda job concluir (~3-5 min)
4. **Olha os logs do backend** — devem aparecer linhas novas:
   ```
   [import:diag] colunas do novo layout — depto="CODEPTO" ... cliprinc="CODCLIPRINC" ...
   [import:diag] roteamento: N linhas → vendas_faturadas, M → vendas_transmitidas, K → vendas_ccd
   [farol:agg] w=0 UPSERT 2026-06 OK em XXms
   [farol:agg] w=0 UPSERT V06 2026-06 OK em XXms
   [farol:agg] w=0 UPSERT V07 2026-06 OK em XXms
   ```

Se aparecer erro `column "cod_cliprinc" does not exist`, migration 181 não
rodou — repetir Passo 2.

Se o CSV **for do formato antigo** (só pra confirmar compat):
- Deve rodar normalmente
- `[import:diag] colunas do novo layout` aparece com `NÃO ENCONTRADO`
- `vendas_ccd` recebe 0 linhas
- V06/V07 continuam vazias

---

## Passo 4 — Validar dados populados no banco

```bash
docker exec "$DBC" psql -U postgres -d fb_farol <<'SQL'
\echo '═══ vendas_faturadas com colunas novas ═══'
SELECT
  COUNT(*) FILTER (WHERE cod_cliprinc <> '') AS com_rede,
  COUNT(*) FILTER (WHERE cod_depto <> '')    AS com_depto,
  COUNT(*) FILTER (WHERE fantasia <> '')     AS com_fantasia,
  COUNT(*) FILTER (WHERE pvenda_unit > 0)    AS com_pvenda_unit,
  COUNT(*)                                    AS total
FROM vendas_faturadas
WHERE data_faturamento BETWEEN '2026-06-01' AND '2026-06-30';

\echo ''
\echo '═══ vendas_ccd (arquivo _CCD.csv precisa ser importado separado) ═══'
SELECT COUNT(*) AS total, COUNT(DISTINCT evento) AS eventos
FROM vendas_ccd;

\echo ''
\echo '═══ Agg V06 populadas por mês? ═══'
SELECT ano, mes, COUNT(*) AS redes, SUM(pvenda)::numeric(14,2) AS total
FROM farol.agg_fat_v06_l0_mes
GROUP BY ano, mes ORDER BY ano, mes;

\echo ''
\echo '═══ Agg V07 populadas? ═══'
SELECT ano, mes, COUNT(*) AS deptos, SUM(pvenda)::numeric(14,2) AS total
FROM farol.agg_fat_v07_l0_mes
GROUP BY ano, mes ORDER BY ano, mes;
SQL
```

**Esperado**: colunas populadas em vendas_faturadas do mês reimportado.
Agg V06/V07 com pelo menos 1 linha por mês. Se `com_rede = 0` mas total > 0,
o CSV importado é do formato antigo — testar com CSV novo.

---

## Passo 5 — Testar V06 "Por Rede" no GRID

1. Login como `jose.costa` ou usuário CEO/gerente
2. Vai pro Painel Executivo
3. **Ctrl+Shift+R** pra forçar reload sem cache
4. No toggle de views, escolhe **"Por Rede"** (botão novo)
5. Confere:
   - GRID mostra cards com nome de rede como título
   - Coluna Positivação está **escondida** (só Venda + Mix Médio + Realizado)
   - Ordenação padrão é valor decrescente

6. Clica numa rede pra fazer drill:
   - Nível 1: Fornecedor (ex: UNILEVER, FERRERO)
   - Nível 2: Cliente (loja individual da rede — pode ter CNPJs diferentes
     compartilhando a mesma rede)
   - Nível 3: Produto (folha, não clica)

**Ponto de atenção**: se algum drill retornar 0 cards, verificar via SQL se a
agg respectiva tem dados. Ex: `agg_fat_v06_l2_mes` precisa ter rows pro
período escolhido.

---

## Passo 6 — Testar V07 "Por Departamento" no GRID

1. Toggle → **"Por Departamento"**
2. GRID mostra 1 card por departamento (ex: LIMPEZA, ALIMENTOS, HIGIENE)
3. Drill:
   - Nível 1: Seção (ex: LAVA-ROUPAS, DETERGENTES)
   - Nível 2: Categoria (ex: SABAO EM PO, SABAO LIQUIDO)
   - Nível 3: Produto
4. Confere que positivação está escondida também

---

## Passo 7 — Confirmar que V01-V05 não foram afetadas

Volta o toggle pra **"Por Indústria"**, faz um drill qualquer. Verifica:
- Valores idênticos ao que era antes (não muda nada)
- Positivação está de volta (colunas visíveis)
- Nenhum comportamento estranho

Se algo mudou nas visões antigas → alerta: pode ter regressão.

---

## Passo 8 — Cenário edge: reimportar CSV formato antigo depois do novo

Cenário: gestor sobe um `JUN_2026.csv` do formato antigo depois de ter subido
o `JUN_2026_TF.csv` novo. O que acontece?

Comportamento esperado:
- `DELETE + COPY` do processFlow apaga os dias do mês antigo (bom)
- Novo import não popula `cod_cliprinc`/`cod_depto` (colunas vazias)
- Agg V06/V07 daquele mês ficam com dados antigos (não são apagadas nem
  atualizadas porque não há dados novos pra elas)

**Isso pode ficar inconsistente.** Vale a pena adicionar no processFlow do
importer um `DELETE FROM agg_fat_v06/v07_*_mes WHERE ano=? AND mes=?` sempre
que reimportar? Considerar em conversa futura se aparecer no uso real.

---

## Rollback rápido (se necessário)

Se algo der errado depois do deploy da Fase 2:

```bash
# 1. Volta o repo pro backup pré-Fase 2
git checkout backup-2026-07-01  # backup manual da noite pré-adequacao
git push origin main --force-with-lease   # cuidado

# 2. Ou revert cirúrgico do commit da Fase 2 (mais seguro)
git revert 530fc9d
git push origin main
# → V06/V07 somem do frontend; funções PL/pgSQL ficam no banco (não atrapalham)

# 3. Se quiser dropar as agg V06/V07 do banco também:
docker exec "$DBC" psql -U postgres -d fb_farol -c "
DROP TABLE IF EXISTS farol.agg_fat_v06_l0_mes, farol.agg_fat_v06_l1_mes, farol.agg_fat_v06_l2_mes CASCADE;
DROP TABLE IF EXISTS farol.agg_trans_v06_l0_mes, farol.agg_trans_v06_l1_mes, farol.agg_trans_v06_l2_mes CASCADE;
DROP TABLE IF EXISTS farol.agg_fat_v07_l0_mes, farol.agg_fat_v07_l1_mes, farol.agg_fat_v07_l2_mes CASCADE;
DROP TABLE IF EXISTS farol.agg_trans_v07_l0_mes, farol.agg_trans_v07_l1_mes, farol.agg_trans_v07_l2_mes CASCADE;
DROP FUNCTION IF EXISTS farol.upsert_aggs_mes_v06 CASCADE;
DROP FUNCTION IF EXISTS farol.upsert_aggs_mes_v07 CASCADE;
"
```

Migrations 181-182 (Fase 1) não devem ser revertidas — as colunas
adicionadas em `vendas_*` não incomodam código existente.

---

## Contexto lateral

- **Backups**: tags `backup-2026-06-22` e `backup-2026-07-01` publicadas no GitHub.
  Tarballs locais em `~/Backups/FB_FAROL-2026-*.tar.gz`
- **Pendência ION VENDAS**: Heverton ainda precisa corrigir o exportador
  pra usar `pcnfsaid.codusur` em vez de JOIN com `pccarteira_rca`. Dedup no
  import continua protegendo. Pacote de evidências em
  `~/Downloads/duplicatas_ion_vendas/`.
- **PLAN.md** e **PLAN-FASE-2.md** no mesmo diretório detalham as decisões.
- **SUMMARY.md** e **SUMMARY-FASE-2.md** têm o registro do que foi feito.

Boa sessão pós-almoço.
