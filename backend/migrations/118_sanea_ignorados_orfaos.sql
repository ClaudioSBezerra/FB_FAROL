-- 118 — Sanea propostas com status='ignorado' órfãs (sem registro em sp_ignorados)
--
-- Bug histórico: ao reativar um produto, o handler antigo apenas deletava de
-- sp_ignorados sem voltar o status da proposta para 'pendente'. Isso fazia o
-- contador do dashboard "Produtos Ignorados" inflar indevidamente.
-- A migration restaura para 'pendente' todas as propostas órfãs.

UPDATE smartpick.sp_propostas p
   SET status = 'pendente',
       aprovado_por = NULL,
       aprovado_em  = NULL
 WHERE p.status = 'ignorado'
   AND NOT EXISTS (
     SELECT 1
       FROM smartpick.sp_ignorados i
      WHERE i.empresa_id = p.empresa_id
        AND i.cd_id      = p.cd_id
        AND i.codprod    = p.codprod
        AND i.cod_filial = p.cod_filial
   );
