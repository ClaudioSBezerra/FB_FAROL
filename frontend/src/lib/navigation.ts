export interface ModuleTab {
  label: string
  path: string
  disabled?: boolean
  danger?: boolean
  adminOnly?: boolean
  masterOnly?: boolean   // visível apenas para admin de plataforma (MASTER)
}

export interface ModuleConfig {
  label: string
  adminOnly?: boolean
  tabs: ModuleTab[]
}

// ─── Farol — Módulos e abas ───────────────────────────────────────────────
export const modules: Record<string, ModuleConfig> = {
  dashboard: {
    label: 'Painel de Calibragem',
    tabs: [
      { label: 'Ampliar Slot',       path: '/dashboard/ampliar' },
      { label: 'Reduzir Slot',       path: '/dashboard/reduzir' },
      { label: 'Já Calibrados',      path: '/dashboard/calibrados' },
      { label: 'Curva A — Revisar',  path: '/dashboard/curva-a' },
      { label: 'Produtos Ignorados', path: '/dashboard/ignorados' },
    ],
  },
  upload: {
    label: 'Importação CSV',
    tabs: [
      { label: 'Upload CSV',        path: '/upload/csv' },
      { label: 'Log de Importação', path: '/upload/log' },
    ],
  },
  historico: {
    label: 'Histórico',
    tabs: [
      { label: 'Histórico de Calibragem', path: '/historico' },
      { label: 'Compliance',              path: '/historico/compliance' },
    ],
  },
  pdf: {
    label: 'PDF',
    tabs: [
      { label: 'Gerar PDF', path: '/pdf/gerar' },
    ],
  },
  reincidencia: {
    label: 'Reincidência',
    tabs: [
      { label: 'Reincidência de Calibragem', path: '/reincidencia' },
    ],
  },
  resultados: {
    label: 'Painel de Resultados',
    tabs: [
      { label: 'Resultados e Métricas 4 Ciclos', path: '/resultados' },
      { label: 'Resumos Executivos (IA)',        path: '/resumos' },
    ],
  },
  // ── Administração (gestor_filial+ — oculto para admin) ───────────────────
  gestao: {
    label: 'Administração',
    tabs: [
      { label: 'Filiais e CDs',        path: '/gestao/filiais' },
      { label: 'Regras de Calibragem', path: '/gestao/regras' },
    ],
  },
  // ── Configurações (admin only) ────────────────────────────────────────────
  config: {
    label: 'Configurações',
    adminOnly: true,
    tabs: [
      { label: 'Plano e Limites', path: '/config/planos',    masterOnly: true },
      { label: 'Ambiente',        path: '/config/ambiente',   masterOnly: true },
      { label: 'Usuários',        path: '/config/usuarios',   masterOnly: true },
      { label: 'Log de Auditoria', path: '/config/audit-log', masterOnly: true },
      { label: 'Bloqueio Empresas', path: '/config/empresas-bloqueio', masterOnly: true },
      { label: 'Uso do Sistema',   path: '/config/uso',               masterOnly: true },
      { label: 'Destinatários Resumo', path: '/config/destinatarios', masterOnly: true },
      { label: 'Manutenção',      path: '/config/manutencao' },
    ],
  },
}

export function getActiveModule(pathname: string): string {
  if (pathname === '/') return 'dashboard'
  if (pathname.startsWith('/dashboard')) return 'dashboard'
  if (pathname.startsWith('/upload')) return 'upload'
  if (pathname.startsWith('/historico')) return 'historico'
  if (pathname.startsWith('/pdf')) return 'pdf'
  if (pathname.startsWith('/reincidencia')) return 'reincidencia'
  if (pathname.startsWith('/resultados')) return 'resultados'
  if (pathname.startsWith('/resumos'))    return 'resultados'
  if (pathname.startsWith('/gestao')) return 'gestao'
  if (pathname.startsWith('/config')) return 'config'
  return 'dashboard'
}
