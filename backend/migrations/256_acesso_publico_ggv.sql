-- 256_acesso_publico_ggv.sql — restringe por indústria o que o link público
-- (mobile, sem login) de um GGV enxerga. Pedido do Claudio, 19/10/2026:
-- Olacir (GGV 3) e Gilson (GGV 4) devem ver SOMENTE Unilever no mobile.
-- Sem linha aqui = sem restrição (comportamento anterior).
CREATE TABLE IF NOT EXISTS farol.acesso_publico_ggv (
    empresa_id UUID    NOT NULL,
    cod_ggv    TEXT    NOT NULL,
    industrias INTEGER[] NOT NULL,
    PRIMARY KEY (empresa_id, cod_ggv)
);

INSERT INTO farol.acesso_publico_ggv (empresa_id, cod_ggv, industrias)
SELECT c.id, g.cod, ARRAY(SELECT i.id FROM farol.industrias i
                          WHERE i.empresa_id = c.id AND i.nome IN ('UNILEVER HC','UNILEVER FOOD'))
FROM companies c, (VALUES ('3'),('4')) AS g(cod)
WHERE EXISTS (SELECT 1 FROM farol.industrias i WHERE i.empresa_id = c.id AND i.nome IN ('UNILEVER HC','UNILEVER FOOD'))
ON CONFLICT DO NOTHING;
