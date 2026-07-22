# Trabalho adiado

Itens encontrados durante revisões que **não** foram causados pela mudança em curso.

## 2026-07-22 — spec-bi-performance-refresh

- **`VendasClearHandler` não invalida `baseCache` nem `vendasPeriodoCache`**
  (`backend/handlers/farol_v2_import.go`, próximo ao `refreshAllFarolViews`).
  Ao limpar vendas, o painel Executivo pode continuar servindo contagem de
  clientes/positivação dos dados apagados por até 30 min (TTL do `baseCache`).
  O import normal invalida os dois; a limpeza não. Pré-existente — só o cache
  do BI foi acertado ali.

- **Thundering herd no vencimento dos caches em memória** (`baseCache`,
  `vendasPeriodoCache`, `biCache`). N requests simultâneas no instante do miss
  disparam N computações idênticas. Padrão já existente no projeto; o BI tem
  poucos espectadores, então o custo real é baixo. Só vale mexer se aparecer
  pico de CPU no Postgres após import.
