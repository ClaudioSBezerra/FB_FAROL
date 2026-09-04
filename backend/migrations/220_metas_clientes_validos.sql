-- Migration 220: Clientes Válidos (Redes + RCA responsável) — Épico 3, Story 3.2
--
-- Executa a decisão de arquitetura tomada na Story 1.4
-- (_bmad-output/implementation-artifacts/1-4-hierarquia-rede-organograma.md):
-- - vinculo_id/vigencia_id como FK (Épico 2) — a lista é escopada por
--   Indústria/Fornecedor × período de vigência (FR11), não é uma hierarquia
--   global do Farol.
-- - rede_nome é TEXT livre, NÃO FK pra uma tabela mestra de Redes — a
--   definição de Rede varia por fornecedor/programa (é o fornecedor que
--   manda a lista mensal). Isso evita fechar a porta pro "Farol de
--   Compras" futuro (decisão já registrada na Story 1.4).
-- - Resolução RCA→CRV→GGV NÃO duplica dado aqui — só guarda cod_rca; subir
--   pra CRV/GGV é JOIN com a hierarquia organizacional já existente do
--   Farol (managers/users, cod_supervisor/cod_gerente).
--
-- UNIQUE (vigencia_id, cnpj): um CNPJ só pode ter UM RCA responsável dentro
-- da mesma vigência — evita ambiguidade na hora de apurar quem atende.

CREATE TABLE IF NOT EXISTS farol.metas_clientes_validos (
    id           SERIAL       PRIMARY KEY,
    empresa_id   UUID         NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    vinculo_id   INTEGER      NOT NULL REFERENCES farol.metas_vinculos(id) ON DELETE CASCADE,
    vigencia_id  INTEGER      NOT NULL REFERENCES farol.metas_vigencias(id) ON DELETE CASCADE,
    rede_nome    TEXT         NOT NULL,
    cnpj         VARCHAR(14)  NOT NULL,
    cod_rca      TEXT         NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT uq_farol_metas_clientes_validos_vigencia_cnpj UNIQUE (vigencia_id, cnpj)
);

CREATE INDEX IF NOT EXISTS idx_farol_metas_clientes_validos_vigencia ON farol.metas_clientes_validos (vigencia_id);
CREATE INDEX IF NOT EXISTS idx_farol_metas_clientes_validos_rede ON farol.metas_clientes_validos (vigencia_id, rede_nome);
CREATE INDEX IF NOT EXISTS idx_farol_metas_clientes_validos_rca ON farol.metas_clientes_validos (cod_rca);
