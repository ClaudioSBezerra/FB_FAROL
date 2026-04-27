-- Migration 112: corrige preferred_company_id nulo em user_environments
--
-- Problema: SpCriarUsuarioHandler inseria user_environments sem preferred_company_id,
-- mesmo quando company_id era fornecido. GetEffectiveCompanyID usa esse campo para
-- encontrar a empresa ativa do usuário; sem ele, o usuário não enxerga suas filiais.
--
-- Fix: para cada user_environments com preferred_company_id IS NULL, tenta derivar
-- a empresa correta a partir de sp_user_filiais (que foi criada corretamente).
-- Só atualiza quando há exatamente uma empresa associada ao usuário naquele ambiente
-- para evitar atribuição incorreta quando há múltiplas empresas.

UPDATE user_environments ue
SET preferred_company_id = (
    SELECT uf.empresa_id
    FROM smartpick.sp_user_filiais uf
    JOIN companies c  ON c.id  = uf.empresa_id
    JOIN enterprise_groups eg ON eg.id = c.group_id
    WHERE uf.user_id = ue.user_id
      AND eg.environment_id = ue.environment_id
    GROUP BY uf.empresa_id
    HAVING COUNT(DISTINCT uf.empresa_id) = 1
    LIMIT 1
)
WHERE ue.preferred_company_id IS NULL
  AND EXISTS (
    SELECT 1
    FROM smartpick.sp_user_filiais uf
    JOIN companies c  ON c.id  = uf.empresa_id
    JOIN enterprise_groups eg ON eg.id = c.group_id
    WHERE uf.user_id = ue.user_id
      AND eg.environment_id = ue.environment_id
  );
