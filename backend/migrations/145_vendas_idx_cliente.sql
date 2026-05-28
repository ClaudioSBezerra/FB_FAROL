-- 145_vendas_idx_cliente.sql
-- Índice para o nível folha "Produto" do Farol.
--
-- Ao clicar num cliente, a API lista os produtos daquele cliente lendo
-- vendas_importadas (as views materializadas agregam o cod_prod no "mix").
-- O filtro é sempre empresa + período + cod_cli — este índice torna isso uma
-- varredura por faixa estreita em vez de scan do período inteiro (~170K linhas).

CREATE INDEX IF NOT EXISTS idx_vi_emp_cli_periodo
  ON vendas_importadas (empresa_id, cod_cli, tipo_base, ano, mes);

ANALYZE vendas_importadas;
