-- Migration 222: formula_codigo em Tipos de Métrica — Épico 4, Story 4.1
--
-- Lacuna descoberta ao desenhar o motor de apuração: parametros_schema
-- (Story 1.1) descreve QUAIS parâmetros um Tipo de Métrica exige, mas nunca
-- disse QUAL algoritmo Go calcula o Realizado dele. Isso é inevitável num
-- framework genérico: o SHAPE dos parâmetros é livre (teste de
-- generalidade do FR1), mas a matemática de calcular "Cobertura" é
-- diferente da de "Sortimento" — precisa de código específico em algum
-- lugar. formula_codigo é esse identificador: o motor de apuração faz
-- switch nele (ver farol_metas_calculo.go). Um Tipo de Métrica novo (ex: o
-- hipotético "Frequência de Visita" da Story 1.1) SEMPRE vai exigir uma
-- calculadora Go nova quando alguém quiser realmente apurá-lo — isso não é
-- falha do framework, é reconhecer honestamente que "genérico no dado" não
-- é o mesmo que "genérico no cálculo".
--
-- Vazio ('') = sem calculadora implementada ainda (o tipo existe no
-- catálogo, mas apurar ele retorna erro claro, não um resultado errado).

ALTER TABLE farol.tipos_metrica
    ADD COLUMN IF NOT EXISTS formula_codigo TEXT NOT NULL DEFAULT '';

UPDATE farol.tipos_metrica SET formula_codigo = 'cobertura_rede' WHERE nome = 'Cobertura por Rede' AND formula_codigo = '';
UPDATE farol.tipos_metrica SET formula_codigo = 'sortimento_rede' WHERE nome = 'Sortimento por Rede' AND formula_codigo = '';
