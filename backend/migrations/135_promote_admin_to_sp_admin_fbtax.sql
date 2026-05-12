-- Migration 135: promove usuários admin da plataforma para sp_role='admin_fbtax'
--
-- Por padrão (migration 101), novos usuários recebem sp_role='somente_leitura'.
-- Isso causa 403 Forbidden em endpoints que exigem perfil SmartPick (sp_role).
-- Esta migration garante que quem é 'admin' no nível plataforma também tenha
-- o perfil mais alto no módulo SmartPick.
--
-- Idempotente: só promove quem ainda não está como admin_fbtax.

UPDATE public.users
SET sp_role = 'admin_fbtax'
WHERE role = 'admin'
  AND sp_role IS DISTINCT FROM 'admin_fbtax';
