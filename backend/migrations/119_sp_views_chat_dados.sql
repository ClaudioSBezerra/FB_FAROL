-- 119 — Views otimizadas para o assistente de IA conversacional (Text-to-SQL)
--
-- O assistente gera SQL a partir de perguntas em linguagem natural. Para
-- minimizar a superfície de ataque, ele só pode consultar essas views
-- documentadas (não as tabelas brutas). Cada query passa por:
--   1. Validador: rejeita tudo que não é SELECT puro
--   2. Transação READ ONLY com statement_timeout=5s
--   3. Filtro automático por empresa_id no WHERE (injetado em runtime)
-- As views já fazem JOINs comuns para que a IA não precise gerar JOINs
-- complicados.

-- ─── vw_propostas_chat ───────────────────────────────────────────────────────
-- Lista todas as propostas com nome do CD/filial, departamento e seção.
CREATE OR REPLACE VIEW smartpick.vw_propostas_chat AS
SELECT
    p.id,
    p.empresa_id,
    p.cd_id,
    cd.nome              AS cd_nome,
    p.cod_filial,
    f.nome               AS filial_nome,
    p.codprod,
    p.produto,
    e.departamento,
    e.secao,
    p.classe_venda,
    p.capacidade_atual,
    p.sugestao_calibragem,
    p.delta,
    p.status,
    p.justificativa,
    e.qt_giro_dia        AS giro_dia_cx,
    e.med_venda_cx,
    e.ponto_reposicao,
    e.participacao,
    p.created_at,
    p.aprovado_em,
    p.aprovado_por
  FROM smartpick.sp_propostas p
  JOIN smartpick.sp_centros_dist cd ON cd.id = p.cd_id
  JOIN smartpick.sp_filiais     f  ON f.id = cd.filial_id
  LEFT JOIN smartpick.sp_enderecos e ON e.id = p.endereco_id;

COMMENT ON VIEW smartpick.vw_propostas_chat IS
  'Propostas de calibragem com nome do CD, filial, depto e seção. Para o assistente IA.';

-- ─── vw_imports_chat ─────────────────────────────────────────────────────────
-- Histórico de imports CSV com nome do CD/filial e usuário.
CREATE OR REPLACE VIEW smartpick.vw_imports_chat AS
SELECT
    j.id::text           AS job_id,
    j.empresa_id,
    j.cd_id,
    cd.nome              AS cd_nome,
    f.nome               AS filial_nome,
    j.filename,
    COALESCE(u.email, '') AS uploaded_by_email,
    j.total_linhas,
    j.linhas_ok,
    j.linhas_erro,
    j.status,
    j.created_at,
    j.finished_at
  FROM smartpick.sp_csv_jobs j
  JOIN smartpick.sp_centros_dist cd ON cd.id = j.cd_id
  JOIN smartpick.sp_filiais     f  ON f.id = cd.filial_id
  LEFT JOIN public.users         u  ON u.id = j.uploaded_by;

COMMENT ON VIEW smartpick.vw_imports_chat IS
  'Imports CSV com nome do CD, filial e email do usuário.';

-- ─── vw_destinatarios_chat ───────────────────────────────────────────────────
-- Lista destinatários do resumo executivo com hierarquia.
CREATE OR REPLACE VIEW smartpick.vw_destinatarios_chat AS
SELECT
    d.id,
    cd.empresa_id,
    d.cd_id,
    cd.nome              AS cd_nome,
    f.nome               AS filial_nome,
    d.nome_completo,
    d.cargo,
    d.email,
    d.ativo,
    d.criado_em
  FROM smartpick.sp_destinatarios_resumo d
  JOIN smartpick.sp_centros_dist cd ON cd.id = d.cd_id
  LEFT JOIN smartpick.sp_filiais f ON f.id = cd.filial_id;

COMMENT ON VIEW smartpick.vw_destinatarios_chat IS
  'Destinatários do resumo executivo com nome do CD/filial.';

-- ─── vw_ignorados_chat ───────────────────────────────────────────────────────
-- Produtos ignorados.
CREATE OR REPLACE VIEW smartpick.vw_ignorados_chat AS
SELECT
    i.id,
    i.empresa_id,
    i.cd_id,
    cd.nome              AS cd_nome,
    f.nome               AS filial_nome,
    i.codprod,
    i.produto,
    COALESCE(ti.descricao, '') AS tipo,
    COALESCE(uu.email, '')     AS ignorado_por_email,
    i.created_at         AS ignorado_em
  FROM smartpick.sp_ignorados i
  JOIN smartpick.sp_centros_dist cd ON cd.id = i.cd_id
  LEFT JOIN smartpick.sp_filiais  f  ON f.id = cd.filial_id
  LEFT JOIN smartpick.sp_tipo_ignorado ti ON ti.id = i.tipo_ignorado_id
  LEFT JOIN public.users          uu ON uu.id = i.ignorado_por;

COMMENT ON VIEW smartpick.vw_ignorados_chat IS
  'Produtos ignorados com nome do CD, filial e usuário que ignorou.';

-- ─── vw_resumo_executivo_chat ────────────────────────────────────────────────
-- Resumos semanais já gerados.
CREATE OR REPLACE VIEW smartpick.vw_resumo_executivo_chat AS
SELECT
    r.id,
    cd.empresa_id,
    r.cd_id,
    cd.nome              AS cd_nome,
    f.nome               AS filial_nome,
    r.periodo_inicio,
    r.periodo_fim,
    r.criado_em,
    r.enviado_em,
    array_length(r.enviado_para, 1) AS qtd_enviados
  FROM smartpick.sp_relatorios_semanais r
  JOIN smartpick.sp_centros_dist cd ON cd.id = r.cd_id
  LEFT JOIN smartpick.sp_filiais f ON f.id = cd.filial_id;

COMMENT ON VIEW smartpick.vw_resumo_executivo_chat IS
  'Resumos executivos semanais gerados.';
