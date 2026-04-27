-- Migration 113: corrige dados do usuário treinamento@treinamento.com.br
--
-- Problema: usuário criado antes do fix de preferred_company_id; filial foi
-- criada no contexto errado (empresa MASTER em vez de empresa Treinamento).
--
-- Ações:
--   1. Localiza usuário e empresa pelo email / nome
--   2. Cria filial "Treinamento" em sp_filiais para a empresa correta (se ausente)
--   3. Cria CD "CD Treinamento" em sp_centros_dist (se ausente)
--   4. Cria sp_motor_params para o CD (se ausente)
--   5. Substitui sp_user_filiais por vínculo com all_filiais = TRUE na empresa certa
--   6. Corrige user_environments.preferred_company_id

DO $$
DECLARE
    v_user_id    uuid;
    v_company_id uuid;
    v_filial_id  int;
    v_cd_id      int;
    v_env_id     uuid;
BEGIN

    -- 1. Localiza usuário
    SELECT id INTO v_user_id
    FROM users
    WHERE email = 'treinamento@treinamento.com.br';

    IF v_user_id IS NULL THEN
        RAISE NOTICE 'migration 113: usuário treinamento@treinamento.com.br não encontrado — sem efeito';
        RETURN;
    END IF;

    -- 2. Localiza empresa: tenta primeiro via user_environments + chain, depois por nome
    SELECT uf.empresa_id INTO v_company_id
    FROM smartpick.sp_user_filiais uf
    WHERE uf.user_id = v_user_id
    ORDER BY uf.created_at DESC
    LIMIT 1;

    IF v_company_id IS NULL THEN
        SELECT c.id INTO v_company_id
        FROM companies c
        WHERE lower(c.name) = 'treinamento'
        ORDER BY c.created_at DESC
        LIMIT 1;
    END IF;

    IF v_company_id IS NULL THEN
        RAISE NOTICE 'migration 113: empresa Treinamento não encontrada — sem efeito';
        RETURN;
    END IF;

    -- 3. Garante que existe pelo menos 1 filial ativa para essa empresa
    SELECT id INTO v_filial_id
    FROM smartpick.sp_filiais
    WHERE empresa_id = v_company_id AND ativo = TRUE
    LIMIT 1;

    IF v_filial_id IS NULL THEN
        INSERT INTO smartpick.sp_filiais (empresa_id, cod_filial, nome)
        VALUES (v_company_id, 1, 'Treinamento')
        RETURNING id INTO v_filial_id;
        RAISE NOTICE 'migration 113: filial Treinamento criada (id=%)', v_filial_id;
    END IF;

    -- 4. Garante que existe pelo menos 1 CD ativo para essa filial
    SELECT id INTO v_cd_id
    FROM smartpick.sp_centros_dist
    WHERE filial_id = v_filial_id AND empresa_id = v_company_id AND ativo = TRUE
    LIMIT 1;

    IF v_cd_id IS NULL THEN
        INSERT INTO smartpick.sp_centros_dist (filial_id, empresa_id, nome, descricao, criado_por)
        VALUES (v_filial_id, v_company_id, 'CD Treinamento', 'CD criado automaticamente para treinamento', v_user_id)
        RETURNING id INTO v_cd_id;
        RAISE NOTICE 'migration 113: CD Treinamento criado (id=%)', v_cd_id;
    END IF;

    -- 5. Garante parâmetros do motor para o CD
    INSERT INTO smartpick.sp_motor_params (cd_id, empresa_id)
    VALUES (v_cd_id, v_company_id)
    ON CONFLICT DO NOTHING;

    -- 6. Substitui sp_user_filiais por vínculo limpo: all_filiais = TRUE
    DELETE FROM smartpick.sp_user_filiais WHERE user_id = v_user_id;

    INSERT INTO smartpick.sp_user_filiais (user_id, empresa_id, filial_id, all_filiais)
    VALUES (v_user_id, v_company_id, NULL, TRUE);

    RAISE NOTICE 'migration 113: sp_user_filiais corrigido (all_filiais=TRUE, empresa=%)', v_company_id;

    -- 7. Corrige user_environments.preferred_company_id
    SELECT eg.environment_id INTO v_env_id
    FROM companies c
    JOIN enterprise_groups eg ON eg.id = c.group_id
    WHERE c.id = v_company_id
    LIMIT 1;

    IF v_env_id IS NOT NULL THEN
        INSERT INTO user_environments (user_id, environment_id, role, preferred_company_id)
        VALUES (v_user_id, v_env_id, 'user', v_company_id)
        ON CONFLICT (user_id, environment_id)
        DO UPDATE SET preferred_company_id = v_company_id;
        RAISE NOTICE 'migration 113: preferred_company_id definido (env=%)', v_env_id;
    ELSE
        -- env_id não encontrado: insere user_environments direto sem env (Strategy C cobrirá)
        RAISE NOTICE 'migration 113: environment_id não encontrado para empresa % — Strategy C usará sp_user_filiais', v_company_id;
    END IF;

    RAISE NOTICE 'migration 113: concluída para user=% empresa=%', v_user_id, v_company_id;

END $$;
