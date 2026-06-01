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
  farol: {
    label: 'Painel Vendas',
    tabs: [
      { label: 'Painel Vendas',  path: '/farol/v2' },
      { label: 'Assistente IA',  path: '/farol/assistente' },
      { label: 'Usuários',       path: '/farol/usuarios', managerOnly: true },
    ],
  },
  // ── Painel Marketing — módulo separado ───────────────────────────────────
  marketing: {
    label: 'Painel Marketing',
    tabs: [
      { label: 'Marketing', path: '/farol/marketing' },
    ],
  },
  // ── Painel BI — War Room para CEO/Diretoria ───────────────────────────────
  bi: {
    label: 'Painel BI',
    tabs: [],
  },
  // ── Importar — ícone próprio no rail, só admin ou TI ─────────────────────
  importar: {
    label: 'Importar dados',
    adminOnly: true,
    tabs: [
      { label: 'Vendas (CSV)', path: '/farol/importar' },
    ],
  },
  // ── Objetivo RCA (em desenvolvimento) ────────────────────────────────────
  obj_rca: {
    label: 'Objetivo RCA',
    dev: true,
    tabs: [
      { label: 'Painel', path: '/objetivos/rca', disabled: true },
    ],
  },
  // ── Objetivo Supervisor (em desenvolvimento) ─────────────────────────────
  obj_supervisor: {
    label: 'Objetivo Supervisor',
    dev: true,
    tabs: [
      { label: 'Painel', path: '/objetivos/supervisor', disabled: true },
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
      { label: 'Obj. Manutenção',   path: '/objetivos/manutencao'     },
    ],
  },
}

export function getActiveModule(pathname: string): string {
  if (pathname.startsWith('/farol/importar'))        return 'importar'
  if (pathname.startsWith('/farol/marketing'))       return 'marketing'
  if (pathname.startsWith('/farol/bi'))              return 'bi'
  if (pathname.startsWith('/farol/v2'))              return 'farol'
  if (pathname.startsWith('/farol/assistente'))      return 'farol'
  if (pathname.startsWith('/farol'))                 return 'farol'
  if (pathname.startsWith('/objetivos/rca'))         return 'obj_rca'
  if (pathname.startsWith('/objetivos/supervisor'))  return 'obj_supervisor'
  if (pathname.startsWith('/objetivos/manutencao'))  return 'config'
  if (pathname.startsWith('/objetivos/importar'))    return 'importar'
  if (pathname.startsWith('/gestao'))                return 'config'
  if (pathname.startsWith('/config'))                return 'config'
  return 'farol'
}
