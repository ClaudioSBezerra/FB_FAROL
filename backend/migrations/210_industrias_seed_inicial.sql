-- Migration 210: seed inicial de Indústrias — lista real enviada pelo
-- Claudio (imagem WhatsApp, 27/08/2026), com os cod_fornec que hoje
-- aparecem espalhados por filial pro MESMO fabricante.
--
-- Renegociação da migration 209 (que dizia explicitamente "não popula via
-- migration/seed, só pela tela"): o Claudio pediu pra já entrar carregado,
-- em vez de digitar ~21 linhas manualmente na tela /gestao/industrias.
-- Depois deste seed, qualquer ajuste (nova indústria, novo cod_fornec,
-- correção) é pela tela — esta migration não roda de novo.
--
-- rotulo só é preenchido quando o cod_fornec precisa de contexto (o
-- fabricante tem MAIS de um código, um por grupo de filiais); nos casos de
-- código único não há ambiguidade a esclarecer.
--
-- ON CONFLICT DO NOTHING em tudo: se uma indústria ou cod_fornec já existir
-- (cadastrado manualmente antes deste deploy, ou re-execução manual), o
-- seed não sobrescreve nem duplica.

DO $$
DECLARE
    r RECORD;
    v_id INTEGER;
BEGIN
    FOR r IN SELECT id AS empresa_id FROM companies LOOP

        -- FINI
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'FINI', 'SANCHEZ CANO LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'FINI';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '37974') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- ALPARGATAS
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'ALPARGATAS', 'ALPARGATAS S.A.') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'ALPARGATAS';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '19263') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- RECKITT CORE (2 códigos — MTZ/MS/BA e PA)
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'RECKITT CORE', 'RECKITT BENCKISER BRASIL LTDA - CORE') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'RECKITT CORE';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '47753', 'MTZ/MS/BA') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '44957', 'PA') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- E.H VESTACY (2 códigos)
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'E.H - VESTACY', 'E.H. (BRASIL) LTDA-VESTACY') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'E.H - VESTACY';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '52682', 'MTZ/MS/BA') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '45603', 'PA') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- FERRERO - FILIAL 18
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'FERRERO - FILIAL 18', 'FERRERO DO BRASIL IND. E DOC. ALIM. LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'FERRERO - FILIAL 18';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '37133', 'FILIAL 18') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- UNILEVER HC
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'UNILEVER HC', 'UNILEVER BRASIL LTDA-396') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'UNILEVER HC';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '396') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- UNILEVER FOOD
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'UNILEVER FOOD', 'UNILEVER BESTFOODS BRASIL LTDA-131') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'UNILEVER FOOD';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '131') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- PG
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'PG', 'PROCTER & GAMBLE INDUSTRIAL E COMERCIAL LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'PG';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '47') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- DIAGEO
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'DIAGEO', 'DIAGEO BRASIL LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'DIAGEO';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '91') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- SANTHER (só código PA)
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'SANTHER', 'SANTHER FAB PAPEL SANTA TEREZINHA 31475') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'SANTHER';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '31475', 'PA') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- MARILAN
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'MARILAN', 'MARILAN ALIMENTOS S/A') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'MARILAN';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '48293') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- SELMI
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'SELMI', 'PASTIFICIO SELMI S/A') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'SELMI';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '45345') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- SELMI TODESCHINI (só código PA)
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'SELMI TODESCHINI', 'PASTIFICIO SELMI S/A (TODESCHINI)') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'SELMI TODESCHINI';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '44783', 'PA') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- DURACELL
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'DURACELL', 'DURACELL COMERCIAL E IMPORTADORA DO BRASIL LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'DURACELL';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '45010') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- SOFTYS (só código PA)
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'SOFTYS', 'SOFTYS BRASIL LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'SOFTYS';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec, rotulo) VALUES (r.empresa_id, v_id, '45203', 'PA') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- SALSARETTI
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'SALSARETTI', 'STELLA D''ORO ALIMENTOS LTDA (SALSARETTI ALIMENTOS)') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'SALSARETTI';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '48377') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- BARRIND - BOLD
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'BARRIND - BOLD', 'BARRIND INDUSTRIA E COMERCIO DE ALIMENTOS LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'BARRIND - BOLD';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '53578') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- HERO - QUEENSBERRY
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'HERO - QUEENSBERRY', 'HERO BRASIL S.A.') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'HERO - QUEENSBERRY';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '39850') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- HENKEL
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'HENKEL', 'HENKEL LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'HENKEL';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '31355') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- MOET
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'MOET', 'MOET HENESSY DO BRASIL VINHOS DEST.LTDA') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'MOET';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '202') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

        -- SEARA
        INSERT INTO farol.industrias (empresa_id, nome, razao_social) VALUES (r.empresa_id, 'SEARA', 'SEARA ALIMENTOS LTDA (MARGARINAS/MAIONESES)') ON CONFLICT (empresa_id, nome) DO NOTHING;
        SELECT id INTO v_id FROM farol.industrias WHERE empresa_id = r.empresa_id AND nome = 'SEARA';
        INSERT INTO farol.industria_fornecedores (empresa_id, industria_id, cod_fornec) VALUES (r.empresa_id, v_id, '48430') ON CONFLICT (empresa_id, cod_fornec) DO NOTHING;

    END LOOP;
END $$;
