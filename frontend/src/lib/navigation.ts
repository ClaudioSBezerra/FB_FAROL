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
  // ── Cadastros (todos os usuários autenticados) ───────────────────────────
  cadastros: {
    label: 'Cadastros',
    tabs: [
      { label: 'Gestores', path: '/cadastros/gestores' },
      { label: 'RCAs',     path: '/cadastros/rcas' },
    ],
  },
  // ── Objetivos (todos os usuários autenticados) ──────────────────────────
  objetivos: {
    label: 'Objetivos',
    tabs: [
      { label: 'Importar', path: '/objetivos/importar' },
    ],
  },
  // ── Administração ────────────────────────────────────────────────────────
  gestao: {
    label: 'Administração',
    tabs: [
      { label: 'Ambiente', path: '/gestao/filiais' },
    ],
  },
  // ── Configurações (admin only) ────────────────────────────────────────────
  config: {
    label: 'Configurações',
    adminOnly: true,
    tabs: [
      { label: 'Ambiente',        path: '/config/ambiente',   masterOnly: true },
      { label: 'Usuários',        path: '/config/usuarios',   masterOnly: true },
      { label: 'Log de Auditoria', path: '/config/audit-log', masterOnly: true },
      { label: 'Bloqueio Empresas', path: '/config/empresas-bloqueio', masterOnly: true },
      { label: 'Uso do Sistema',   path: '/config/uso',               masterOnly: true },
      { label: 'Manutenção',      path: '/config/manutencao' },
    ],
  },
}

export function getActiveModule(pathname: string): string {
  if (pathname.startsWith('/cadastros')) return 'cadastros'
  if (pathname.startsWith('/objetivos')) return 'objetivos'
  if (pathname.startsWith('/gestao'))    return 'gestao'
  if (pathname.startsWith('/config'))    return 'config'
  return 'cadastros'
}
