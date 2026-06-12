-- Migration 026 — DESATIVADA no FB_FAROL.
--
-- Originalmente populava a tabela `cfop` (Código Fiscal de Operações e
-- Prestações) com descrições contábeis. Isso é função do FB_APU (Apuração
-- Fiscal), não do Farol de Vendas. As tabelas cfop, sped, efd_icms etc. são
-- legado herdado do fork do FB_APU mas não são consumidas pelo FAROL.
--
-- A migration foi esvaziada (no-op) para destravar o startup do banco —
-- vários descritivos de CFOP excediam VARCHAR(100) e quebravam o INSERT.
-- A tabela cfop continua existindo (criada pela 009) mas fica vazia neste
-- ambiente. Nenhum código do FAROL lê dela.
--
-- Se em algum momento o FAROL precisar dos CFOPs, restaurar este arquivo e
-- garantir que cfop.descricao_cfop seja VARCHAR(255) ou maior.

SELECT 1; -- no-op para a migration ser válida
