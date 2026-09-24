export interface ModuleTab {
  label: string
  path: string
  disabled?: boolean
  danger?: boolean
  adminOnly?: boolean
  masterOnly?: boolean    // visível apenas para admin de plataforma (MASTER)
  managerOnly?: boolean   // visível para quem pode gerir usuários (TI, admin, admin_fbtax)
}

export interface ModuleConfig {
  label: string
  adminOnly?: boolean
  dev?: boolean           // módulo em desenvolvimento — exibido no rail com badge
  tabs: ModuleTab[]
}

// ─── Farol — Módulos e abas ───────────────────────────────────────────────
export const modules: Record<string, ModuleConfig> = {
  // ── Farol V2 — sistema principal de vendas ───────────────────────────────
  // Sem abas — pedido do Claudio 24/09/2026: "Comparativo Fechamento
  // Comercial" e "Envio do Resumo" apareciam aqui duplicados (a mesma
  // barra de abas vazava pra dentro de Relatórios também, por falta de um
  // módulo próprio — ver 'relatorios' abaixo, mesmo bug já corrigido pra
  // Gamificação em 23/09/2026). As duas rotas continuam funcionando por
  // URL direta; só saíram do menu.
  farol: {
    label: 'Painel Vendas',
    tabs: [],
  },
  // Relatórios — pedido do Claudio 24/09/2026: antes não existia módulo
  // próprio, então /farol/relatorios caía no fallback genérico
  // (getActiveModule) e herdava o cabeçalho/abas de "Painel Vendas" —
  // achado do Claudio: "dentro dele está como Painel de Vendas".
  relatorios: {
    label: 'Relatórios',
    tabs: [],
  },
  // ── Painel BI — desabilitado no rail (pedido do Claudio 24/09/2026: "o
  // CEO não usa computador") — ver AppRail.tsx, item comentado, não
  // apagado. A tela em si continua existindo, só não aparece mais no menu.
  bi: {
    label: 'Painel BI',
    tabs: [],
  },
  // ── Gamificação — MVP restrito a admin_fbtax (22/09/2026) ────────────────
  // Sem isto, /farol/gamificacao caía no fallback genérico de getActiveModule
  // (qualquer path começando com /farol → módulo 'farol') e herdava as abas
  // de Painel Vendas (Comparativo Fechamento, Envio do Resumo) — que não têm
  // nada a ver com Gamificação. Achado do Claudio 23/09/2026 ("menu está
  // estranho"). tabs: [] some com a barra de abas (ModuleTabs não renderiza
  // nada quando vazio) — a própria tela já tem sua navegação interna
  // (lista de campanhas → detalhe).
  gamificacao: {
    label: 'Gamificação',
    adminOnly: true,
    tabs: [],
  },
  // ── Objetivos por Indústria — visão de campo pra GGV/Supervisor (Épico 5) ─
  metas_industria: {
    label: 'Objetivos Indústria',
    tabs: [
      { label: 'Painel', path: '/farol/metas-industria' },
    ],
  },
  // ── Importar — ícone próprio no rail, só admin ou TI ─────────────────────
  importar: {
    label: 'Importar dados',
    adminOnly: true,
    tabs: [
      { label: 'Vendas (CSV)', path: '/farol/importar' },
    ],
  },
  // ── Configurações (admin only) ────────────────────────────────────────────
  config: {
    label: 'Configurações',
    adminOnly: true,
    tabs: [
      { label: 'Ambiente',          path: '/config/ambiente',         masterOnly: true },
      { label: 'Usuários',          path: '/config/usuarios',         masterOnly: true },
      { label: 'Log de Auditoria',  path: '/config/audit-log',        masterOnly: true },
      { label: 'Bloqueio Empresas', path: '/config/empresas-bloqueio', masterOnly: true },
      { label: 'Uso do Sistema',    path: '/config/uso',              masterOnly: true },
      { label: 'Limpar Dados',      path: '/config/limpar-dados',     adminOnly: true,  danger: true },
      { label: 'Sazonalidade',      path: '/config/sazonalidade',     adminOnly: true },
      { label: 'Obj. Manutenção',   path: '/objetivos/manutencao'     },
      { label: 'Indústrias',        path: '/gestao/industrias'        },
      { label: 'Tipos de Métrica',  path: '/gestao/tipos-metrica'     },
      { label: 'Objetivos por Indústria', path: '/gestao/metas-vinculos' },
    ],
  },
}

export function getActiveModule(pathname: string): string {
  if (pathname.startsWith('/farol/importar'))        return 'importar'
  if (pathname.startsWith('/farol/bi'))              return 'bi'
  if (pathname.startsWith('/farol/gamificacao'))     return 'gamificacao'
  if (pathname.startsWith('/farol/metas-industria')) return 'metas_industria'
  if (pathname.startsWith('/farol/relatorios'))      return 'relatorios'
  if (pathname.startsWith('/farol/v2'))              return 'farol'
  if (pathname.startsWith('/farol/assistente'))      return 'farol'
  if (pathname.startsWith('/farol'))                 return 'farol'
  if (pathname.startsWith('/objetivos/manutencao'))  return 'config'
  if (pathname.startsWith('/objetivos/importar'))    return 'importar'
  if (pathname.startsWith('/gestao'))                return 'config'
  if (pathname.startsWith('/config'))                return 'config'
  return 'farol'
}
