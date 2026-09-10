-- 231_ensure_vendas_ano_partition.sql
-- ════════════════════════════════════════════════════════════════════════════
-- Parte da política de retenção rolante de 2 anos em vendas_faturadas/
-- vendas_transmitidas (pedido do Claudio, 10/09/2026: "vamos trabalhar com
-- 2 anos de base... em fev/2027 a base 2025 pode ser limpa").
--
-- Esta migration é só a metade "automática" (roda no boot normal, como
-- qualquer outra): a função que garante a partição do ano corrente/futuro
-- ANTES que a importação diária tente inserir uma linha nela — mesmo
-- espírito de farol.create_agg_year_partitions (migration 165), só que pras
-- tabelas BRUTAS.
--
-- A OUTRA metade — converter vendas_faturadas/vendas_transmitidas de tabela
-- normal pra particionada por RANGE(data) sem duplicar os ~49 GB já
-- existentes em produção — NÃO está nesta migration. É um script manual
-- separado (scripts/particionar_vendas_2025_2026.sql), porque:
--   1. Antes da conversão rodar em produção, esta função falharia (não
--      existe "PARTITION OF vendas_faturadas" possível numa tabela que
--      ainda não é particionada) — então ela só passa a ter efeito DEPOIS
--      do script manual rodar. Até lá, é código morto inofensivo (só é
--      chamada, e falharia silenciosamente sendo ignorada, ver Go).
--   2. O passo de VALIDATE CONSTRAINT do script manual faz um scan de
--      leitura em ~26GB/~23GB — pode levar minutos. Não é o tipo de
--      operação pra rodar automaticamente no boot do app (que já roda
--      minicações várias vezes por dia); precisa ser um passo deliberado,
--      com confirmação explícita, rodado uma vez.
--
-- Depois que o script manual rodar (conversão feita), este mesmo boot da
-- aplicação (ou o próximo) já chama esta função pros anos correntes,
-- criando 2027/2028 se ainda não existirem — sem exigir nenhum passo manual
-- adicional pra manutenção do dia a dia daqui pra frente.
-- ════════════════════════════════════════════════════════════════════════════

CREATE OR REPLACE FUNCTION farol.ensure_vendas_ano_partition(p_ano INT) RETURNS VOID AS $$
BEGIN
    -- Se vendas_faturadas/vendas_transmitidas ainda não foram convertidas
    -- pra particionada (script manual ainda não rodou), a criação de
    -- partição não faz sentido — sai calado em vez de lançar erro toda
    -- vez que o app sobe, até o dia em que a conversão acontecer.
    IF NOT EXISTS (
        SELECT 1 FROM pg_partitioned_table pt
        JOIN pg_class c ON c.oid = pt.partrelid
        WHERE c.relname = 'vendas_faturadas'
    ) THEN
        RETURN;
    END IF;

    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF vendas_faturadas FOR VALUES FROM (%L) TO (%L)',
        'vendas_faturadas_' || p_ano, make_date(p_ano, 1, 1), make_date(p_ano + 1, 1, 1)
    );
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF vendas_transmitidas FOR VALUES FROM (%L) TO (%L)',
        'vendas_transmitidas_' || p_ano, make_date(p_ano, 1, 1), make_date(p_ano + 1, 1, 1)
    );
END;
$$ LANGUAGE plpgsql;
