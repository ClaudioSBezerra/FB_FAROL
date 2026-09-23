-- Escala de pagamento EDITÁVEL por campanha — pedido do Claudio 23/09/2026:
-- "podemos precisar editar a configuração da premiação... Você colocou via
-- UPDATE e não está acessível na tela". Os cortes/multiplicadores de
-- migration 244 (bronze 60%→30%, prata 75%→50%, ouro 100%→100%, diamante
-- 120%→120%) eram fixos no código Go. Agora viram linhas em
-- farol.gamif_niveis, editáveis por campanha (cada campanha pode ter sua
-- própria escala, com um número variável de níveis — inclusive nomes
-- livres tipo "Platina").
CREATE TABLE farol.gamif_niveis (
    id SERIAL PRIMARY KEY,
    campanha_id INT NOT NULL REFERENCES farol.gamif_campanhas(id) ON DELETE CASCADE,
    nome TEXT NOT NULL,
    percentual_minimo NUMERIC(7,2) NOT NULL,
    multiplicador NUMERIC(6,3) NOT NULL,
    ordem INT NOT NULL
);

CREATE INDEX idx_gamif_niveis_campanha ON farol.gamif_niveis (campanha_id, ordem);

COMMENT ON TABLE farol.gamif_niveis IS 'Escala de pagamento por nível (Bronze/Prata/Ouro/Diamante ou o que o admin definir), editável por campanha. ordem = posição crescente por percentual_minimo, usada pro frontend escolher a cor do troféu (1º=bronze, 2º=prata, 3º=ouro, 4º=diamante, 5º+ cicla outras cores).';
COMMENT ON COLUMN farol.gamif_niveis.percentual_minimo IS 'corte de entrada (>=) em % do objetivo — ex.: 60 = precisa atingir 60% pra entrar neste nível';
COMMENT ON COLUMN farol.gamif_niveis.multiplicador IS 'fração do pontos/valor_bonus da regra que este nível paga — ex.: 0.30 = 30% do valor cheio';

-- Backfill: toda campanha já existente ganha a escala padrão (a mesma que
-- estava fixa no código) — sem isso, campanhas ativas hoje ficariam sem
-- NENHUM nível configurado, e ninguém pontuaria no próximo recálculo.
INSERT INTO farol.gamif_niveis (campanha_id, nome, percentual_minimo, multiplicador, ordem)
SELECT id, 'Bronze', 60, 0.30, 1 FROM farol.gamif_campanhas
UNION ALL
SELECT id, 'Prata', 75, 0.50, 2 FROM farol.gamif_campanhas
UNION ALL
SELECT id, 'Ouro', 100, 1.00, 3 FROM farol.gamif_campanhas
UNION ALL
SELECT id, 'Diamante', 120, 1.20, 4 FROM farol.gamif_campanhas;

-- gamif_pontuacao ganha nivel_ordem — a posição (1,2,3...) do nível batido
-- dentro da escala DAQUELA campanha, pra frontend/mobile colorir o troféu
-- sem precisar buscar a lista de níveis de novo (nomes agora são livres,
-- não dá mais pra colorir só pelo nome "bronze"/"prata"/etc.).
ALTER TABLE farol.gamif_pontuacao ADD COLUMN nivel_ordem INT;
COMMENT ON COLUMN farol.gamif_pontuacao.nivel_ordem IS 'posição (1=mais baixo) do nivel_principal dentro da escala da campanha — usada só pra escolher a cor do troféu, NULL quando percentual_principal não bateu nenhum nível';
