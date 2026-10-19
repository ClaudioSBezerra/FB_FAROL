-- 255_normaliza_industrias_usuario.sql — indústria é cadastro POR EMPRESA.
-- users.industrias_permitidas gravado com ids de OUTRA empresa (ex.: os da MASTER,
-- que a tela de usuários listava quando o admin logado era da MASTER) não batia
-- com nenhum vínculo da JC e o usuário ficava sem dado (19/10/2026). Troca cada
-- id pelo da indústria de mesmo nome na empresa preferida do usuário. Se não
-- houver equivalente pra nenhum id, deixa como está. Idempotente.
UPDATE users u
   SET industrias_permitidas = m.ids
  FROM (
        SELECT u2.id AS user_id, array_agg(DISTINCT x.id ORDER BY x.id) AS ids
          FROM users u2
          JOIN LATERAL (SELECT preferred_company_id FROM user_environments
                         WHERE user_id = u2.id AND preferred_company_id IS NOT NULL LIMIT 1) ue ON true
          CROSS JOIN LATERAL unnest(u2.industrias_permitidas) AS p(id)
          JOIN farol.industrias o ON o.id = p.id
          JOIN farol.industrias x ON x.empresa_id = ue.preferred_company_id
                                 AND (x.id = o.id OR upper(x.nome) = upper(o.nome))
         WHERE u2.industrias_permitidas IS NOT NULL
         GROUP BY u2.id
       ) m
 WHERE m.user_id = u.id
   AND u.industrias_permitidas IS DISTINCT FROM m.ids;
