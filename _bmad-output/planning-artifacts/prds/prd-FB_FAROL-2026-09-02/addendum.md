# Addendum — Painel de Gestão de Metas por Indústria

Conteúdo técnico/aprofundado que não cabe no corpo do PRD, mas foi levantado durante a conversa e serve pra arquitetura/épicos depois.

## Importação de Metas — mecanismo/transporte

- **Fase 1 (este PRD):** upload de CSV pelo admin, com os valores de meta por indústria/tipo de métrica/faixa/período.
- **Fase 2 (fora de escopo, registrado como direção futura):** integração direta com a base Oracle da JC como fonte de metas, substituindo o CSV manual. Ver memória de sessão `infra_dev_vm.md` — já existe um Oracle da JC alcançável a partir de produção (usado hoje pra outra integração, "Oracle da JC alcançável de produção via sonda manual do Claudio").
