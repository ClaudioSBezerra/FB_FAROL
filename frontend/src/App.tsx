import { BrowserRouter, Routes, Route, Navigate, useLocation, Link } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import { queryClient } from '@/lib/queryClient'
import { Toaster } from '@/components/ui/sonner'
import ObjetivosManutencao from './pages/ObjetivosManutencao'
import GestaoAmbiente from './pages/GestaoAmbiente'
import AdminUsers from './pages/AdminUsers'
import SpUsuarios from './pages/SpUsuarios'
import SpAmbiente from './pages/SpAmbiente'
import SpUploadCSV from './pages/SpUploadCSV'
import SpIgnorados from './pages/SpIgnorados'
import SpDashboard from './pages/SpDashboard'
import SpGerarPDF from './pages/SpGerarPDF'
import SpHistorico from './pages/SpHistorico'
import SpReincidencia from './pages/SpReincidencia'
import SpResultados from './pages/SpResultados'
import SpResumoExecutivo from './pages/SpResumoExecutivo'
import SpDestinatarios from './pages/SpDestinatarios'
import SpAuditLog from './pages/SpAuditLog'
import SpEmpresasBloqueio from './pages/SpEmpresasBloqueio'
import SpUsoSistema from './pages/SpUsoSistema'
import { useUsageTracker } from './hooks/useUsageTracker'
import Login from './pages/Login'
import Register from './pages/Register'
import ForgotPassword from './pages/ForgotPassword'
import ResetPassword from './pages/ResetPassword'
import FarolPublicPanel from './pages/farol/FarolPublicPanel'
import LimparDados from './pages/LimparDados'
import ConfigSazonalidade from './pages/ConfigSazonalidade'
import GestaoIndustrias from './pages/GestaoIndustrias'
import ConfigTiposMetrica from './pages/ConfigTiposMetrica'
import ConfigMetasVinculos from './pages/ConfigMetasVinculos'
import FarolPainelMetas from './pages/FarolPainelMetas'
import { FarolWebList, FarolWebDashboard, FarolWebRcaDetail, FarolWebFornecRcas, FarolWebFornecSups } from './pages/farol/FarolWeb'
import FarolV2Dashboard from './pages/farol/FarolV2Dashboard'
import FarolDinheiroNaMesa from './pages/farol/FarolDinheiroNaMesa'
import FarolResumoEnvio from './pages/farol/FarolResumoEnvio'
import FarolV2Import from './pages/farol/FarolV2Import'
import FarolUsuarios from './pages/farol/FarolUsuarios'
import FarolBI from './pages/farol/FarolBI'
import FarolAssistente from './pages/farol/FarolAssistente'
import FarolRelatorios from './pages/farol/FarolRelatorios'
import { AppRail } from '@/components/AppRail'
import { CompanySwitcher } from '@/components/CompanySwitcher'
import { AjudaChat } from '@/components/AjudaChat'
import { FarolAjudaChat } from '@/components/farol/FarolAjudaChat'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { FilialProvider } from './contexts/FilialContext'
import { getActiveModule, modules } from '@/lib/navigation'
import { cn } from '@/lib/utils'

// queryClient compartilhado vem de @/lib/queryClient (também usado pelo AuthContext para prefetch)


function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  return <>{children}</>
}

function AdminRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, user } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  if (user?.role !== 'admin') return <Navigate to="/" replace />
  return <>{children}</>
}

function MasterRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, group } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  if (group !== 'MASTER') return <Navigate to="/" replace />
  return <>{children}</>
}

// AdminOrTIRoute — libera para admin de plataforma, admin_fbtax do Farol, ou
// usuário com tipo_persona='ti'. Usado pela tela de Importar.
function AdminOrTIRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, user, spRole, tipoPersona } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  const ok = user?.role === 'admin' || spRole === 'admin_fbtax' || tipoPersona === 'ti'
  if (!ok) return <Navigate to="/" replace />
  return <>{children}</>
}

// TIOnlyRoute — para personas 'ti', só permite acessar /farol/importar e /farol/relatorios.
// Para qualquer outra rota, redireciona para /farol/importar.
function TIOnlyRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, tipoPersona } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  if (tipoPersona === 'ti') {
    // TI só pode acessar /farol/importar e /farol/relatorios
    const allowedPaths = ['/farol/importar', '/farol/relatorios']
    if (!allowedPaths.includes(location.pathname)) {
      return <Navigate to="/farol/importar" replace />
    }
  }
  return <>{children}</>
}

// AdminFbtaxRoute — libera para admin de plataforma (MASTER) ou admin_fbtax do Farol.
// Usado por telas de limpeza de dados e outras operações administrativas de tenant.
function AdminFbtaxRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, user, spRole } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  const ok = user?.role === 'admin' || spRole === 'admin_fbtax'
  if (!ok) return <Navigate to="/" replace />
  return <>{children}</>
}

function ManagerRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading, canManageUsers } = useAuth()
  const location = useLocation()
  if (loading) return null
  if (!isAuthenticated) return <Navigate to="/login" state={{ from: location }} replace />
  if (!canManageUsers) return <Navigate to="/" replace />
  return <>{children}</>
}

// ── Barra de abas por módulo ─────────────────────────────────────────────────
function ModuleTabs() {
  const location  = useLocation()
  const { user, spRole, group, canManageUsers }  = useAuth()
  const isMaster  = group === 'MASTER'
  const isAdmin   = isMaster || spRole === 'admin_fbtax'
  const moduleId  = getActiveModule(location.pathname)
  const moduleCfg = modules[moduleId]

  if (!moduleCfg || moduleCfg.tabs.length === 0) return null
  if (moduleCfg.adminOnly && !isAdmin) return null

  const visibleTabs = moduleCfg.tabs.filter(t =>
    (!t.adminOnly    || isAdmin) &&
    (!t.masterOnly   || isMaster) &&
    (!t.managerOnly  || canManageUsers)
  )

  return (
    <div key={moduleId} className="border-b bg-white px-4 flex items-center gap-0.5 overflow-x-auto shrink-0 h-10">
      {visibleTabs.map(tab => {
        const isActive   = location.pathname === tab.path
        const isDisabled = tab.disabled
        return isDisabled ? (
          <span
            key={tab.path}
            className="px-3 py-1.5 text-xs rounded-md text-muted-foreground/50 cursor-not-allowed whitespace-nowrap"
          >
            {tab.label}
          </span>
        ) : (
          <Link
            key={tab.path}
            to={tab.path}
            className={cn(
              'px-3 py-1.5 text-xs rounded-md whitespace-nowrap transition-colors',
              isActive
                ? tab.danger
                  ? 'bg-red-50 text-red-700 font-semibold'
                  : 'bg-primary/10 text-primary font-semibold'
                : tab.danger
                  ? 'text-red-500 hover:bg-red-50 hover:text-red-700'
                  : 'text-muted-foreground hover:bg-gray-100 hover:text-foreground'
            )}
          >
            {tab.label}
          </Link>
        )
      })}
    </div>
  )
}

// ── Cabeçalho ─────────────────────────────────────────────────────────────────
function AppHeader() {
  const location  = useLocation()
  const moduleId  = getActiveModule(location.pathname)
  const moduleCfg = modules[moduleId]
  const { company } = useAuth()

  return (
    <header className="flex items-center justify-between h-12 border-b bg-white px-4 shrink-0">
      <span className="text-sm font-semibold text-foreground">
        {moduleCfg?.label ?? 'Farol'}
      </span>
      <div className="flex items-center gap-2">
        {/* Empresa ativa — sempre visível, independente de quantas empresas existem */}
        {company && (
          <span
            className="flex items-center gap-1.5 text-xs font-medium text-sky-700 bg-sky-50 border border-sky-200 px-2.5 py-1 rounded-full max-w-[220px]"
            title={company}
          >
            <svg className="h-3 w-3 shrink-0 text-sky-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M3 21V7l9-4 9 4v14M9 21V12h6v9" />
            </svg>
            <span className="truncate">{company}</span>
          </span>
        )}
        <CompanySwitcher compact />
      </div>
    </header>
  )
}

// ── Layout principal ─────────────────────────────────────────────────────────
function AppLayout() {
  useUsageTracker()   // E1: rastreia tempo de permanência por módulo
  return (
    <div className="flex h-screen overflow-hidden bg-background">
      <AppRail />
      <div className="flex flex-col flex-1 min-w-0">
        <AppHeader />
        <ModuleTabs />
        <main className="flex-1 overflow-auto">
          <div className="p-4">
            <TIOnlyRoute>
              <Routes>
              <Route path="/" element={<Navigate to="/farol/v2" replace />} />

              {/* Painel de Calibragem */}
              <Route path="/dashboard/ampliar"    element={<ProtectedRoute><SpDashboard /></ProtectedRoute>} />
              <Route path="/dashboard/reduzir"    element={<ProtectedRoute><SpDashboard /></ProtectedRoute>} />
              <Route path="/dashboard/calibrados" element={<ProtectedRoute><SpDashboard /></ProtectedRoute>} />
              <Route path="/dashboard/curva-a"    element={<ProtectedRoute><SpDashboard /></ProtectedRoute>} />

              {/* Produtos Ignorados */}
              <Route path="/dashboard/ignorados" element={<ProtectedRoute><SpIgnorados /></ProtectedRoute>} />

              {/* Redirecionamentos de rotas antigas do dashboard */}
              <Route path="/dashboard/urgencia/falta"  element={<Navigate to="/dashboard/ampliar" replace />} />
              <Route path="/dashboard/urgencia/espaco" element={<Navigate to="/dashboard/reduzir" replace />} />

              {/* Upload CSV (Epic 4) */}
              <Route path="/upload/csv" element={<ProtectedRoute><SpUploadCSV /></ProtectedRoute>} />
              <Route path="/upload/log" element={<ProtectedRoute><SpUploadCSV /></ProtectedRoute>} />

              {/* Histórico (Epic 7) */}
              <Route path="/historico"            element={<ProtectedRoute><SpHistorico /></ProtectedRoute>} />
              <Route path="/historico/compliance" element={<ProtectedRoute><SpHistorico /></ProtectedRoute>} />

              {/* Reincidência (Epic 8) */}
              <Route path="/reincidencia" element={<ProtectedRoute><SpReincidencia /></ProtectedRoute>} />

              {/* Painel de Resultados (Epic 9) */}
              <Route path="/resultados" element={<ProtectedRoute><SpResultados /></ProtectedRoute>} />

              {/* Resumo Executivo Semanal (IA) */}
              <Route path="/resumos" element={<ProtectedRoute><SpResumoExecutivo /></ProtectedRoute>} />

              {/* PDF (Epic 6) */}
              <Route path="/pdf/gerar" element={<ProtectedRoute><SpGerarPDF /></ProtectedRoute>} />

              {/* Objetivos — Em Desenvolvimento (rotas mantidas para compatibilidade) */}
              {/* Farol V2 — novo sistema de vendas (Reescrita 2026) */}
              <Route path="/farol/v2"         element={<ProtectedRoute><FarolV2Dashboard /></ProtectedRoute>} />
              {/* Página que o resumo semanal linka. Protegida: o quadro da empresa
                  inteira não fica atrás de URL que circula em conversa. */}
              <Route path="/farol/dinheiro-na-mesa" element={<ProtectedRoute><FarolDinheiroNaMesa /></ProtectedRoute>} />
              <Route path="/farol/resumo/envio"     element={<ProtectedRoute><FarolResumoEnvio /></ProtectedRoute>} />
              <Route path="/farol/bi"         element={<ProtectedRoute><FarolBI /></ProtectedRoute>} />
              <Route path="/farol/assistente" element={<ProtectedRoute><FarolAssistente /></ProtectedRoute>} />
              <Route path="/farol/importar"   element={<AdminOrTIRoute><FarolV2Import /></AdminOrTIRoute>} />
              <Route path="/farol/usuarios"   element={<ManagerRoute><FarolUsuarios /></ManagerRoute>} />
              {/* Relatórios: a página inteira era admin/TI-only. Comparativo REL 322
                  passou a valer pra QUALQUER usuário com acesso web (pedido do
                  Claudio em 28/08/2026) — Extrato de Produtos por Cliente e
                  Clientes com CNPJ irregular continuam admin/TI-only, mas esse
                  recorte agora é feito DENTRO de FarolRelatorios.tsx (abas
                  escondidas + aba padrão), não mais no gate da rota. */}
              <Route path="/farol/relatorios" element={<ProtectedRoute><FarolRelatorios /></ProtectedRoute>} />

              {/* Farol legado — versão web (mesma visão do mobile, autenticada) */}
              <Route path="/farol"                              element={<ProtectedRoute><FarolWebList /></ProtectedRoute>} />
              <Route path="/farol/forn/:codFornec"              element={<ProtectedRoute><FarolWebFornecSups /></ProtectedRoute>} />
              <Route path="/farol/sup/:cod"                     element={<ProtectedRoute><FarolWebDashboard /></ProtectedRoute>} />
              <Route path="/farol/sup/:cod/forn/:codFornec"     element={<ProtectedRoute><FarolWebFornecRcas /></ProtectedRoute>} />
              <Route path="/farol/sup/:cod/rca/:codRca"         element={<ProtectedRoute><FarolWebRcaDetail /></ProtectedRoute>} />
              <Route path="/objetivos/manutencao"  element={<ProtectedRoute><ObjetivosManutencao /></ProtectedRoute>} />

              {/* Gestão de CD (gestor_filial+) */}
              <Route path="/gestao/filiais" element={<ProtectedRoute><SpAmbiente /></ProtectedRoute>} />
              <Route path="/gestao/regras"  element={<ProtectedRoute><SpAmbiente /></ProtectedRoute>} />
              <Route path="/gestao/industrias" element={<ProtectedRoute><GestaoIndustrias /></ProtectedRoute>} />
              <Route path="/gestao/tipos-metrica" element={<ProtectedRoute><ConfigTiposMetrica /></ProtectedRoute>} />
              <Route path="/gestao/metas-vinculos" element={<ProtectedRoute><ConfigMetasVinculos /></ProtectedRoute>} />
              <Route path="/farol/metas-industria" element={<ProtectedRoute><FarolPainelMetas /></ProtectedRoute>} />

              {/* Configurações (admin) */}
              <Route path="/config/planos"      element={<ProtectedRoute><SpAmbiente /></ProtectedRoute>} />
              <Route path="/config/manutencao"  element={<Navigate to="/config/limpar-dados" replace />} />
              <Route path="/config/ambiente"    element={<ProtectedRoute><GestaoAmbiente /></ProtectedRoute>} />
              <Route path="/config/usuarios"    element={<MasterRoute><SpUsuarios /></MasterRoute>} />
              <Route path="/config/usuarios-admin" element={<MasterRoute><AdminUsers /></MasterRoute>} />
              <Route path="/config/audit-log"   element={<MasterRoute><SpAuditLog /></MasterRoute>} />
              <Route path="/config/empresas-bloqueio" element={<MasterRoute><SpEmpresasBloqueio /></MasterRoute>} />
              <Route path="/config/limpar-dados" element={<AdminFbtaxRoute><LimparDados /></AdminFbtaxRoute>} />
              <Route path="/config/sazonalidade" element={<AdminFbtaxRoute><ConfigSazonalidade /></AdminFbtaxRoute>} />
              <Route path="/config/uso"         element={<MasterRoute><SpUsoSistema /></MasterRoute>} />
              <Route path="/config/destinatarios" element={<MasterRoute><SpDestinatarios /></MasterRoute>} />

              {/* Redirecionamentos de rotas antigas */}
              <Route path="/config/parametros-motor" element={<Navigate to="/gestao/regras" replace />} />
              <Route path="/config/gestao-ambiente"  element={<Navigate to="/config/ambiente" replace />} />
            </Routes>
            </TIOnlyRoute>
          </div>
        </main>
      </div>
      <Toaster />
      {/* <AjudaChat /> */}
      <FarolAjudaChat />
    </div>
  )
}

// ── App root ─────────────────────────────────────────────────────────────────
function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <AuthProvider>
          <Routes>
            <Route path="/login"           element={<Login />} />
            <Route path="/register"        element={<Register />} />
            <Route path="/forgot-password" element={<ForgotPassword />} />
            <Route path="/reset-senha"     element={<ResetPassword />} />

            {/* Farol ION VENDAS (público — link parametrizado, sem login).
                Os redirects /701 → /m/701 e /CNPJ/SUP|RCA/cod → /m/... são feitos
                server-side em main.go. Todas as rotas /m/... abrem o painel novo
                (FarolPublicPanel) escopado por CNPJ + SUPV/RCA, sobre as views novas.
                A rota /m/:cod/rca/:codRca atende ambos formatos (cod=supervisor OU CNPJ). */}
            <Route path="/m/:cnpj/sup/:cod"                 element={<FarolPublicPanel />} />
            <Route path="/m/:cnpj/sup/:cod/forn/:codFornec" element={<FarolPublicPanel />} />
            <Route path="/m/:cod"                           element={<FarolPublicPanel />} />
            <Route path="/m/:cod/forn/:codFornec"           element={<FarolPublicPanel />} />
            <Route path="/m/:cod/rca/:codRca"               element={<FarolPublicPanel />} />

            {/* Quadro por token, sem login — o link que vai por WhatsApp.
                Fica junto das rotas /m/... porque é o mesmo padrão: token na
                URL é a credencial, e a tela é só leitura. */}
            <Route path="/q/:token"                         element={<FarolDinheiroNaMesa />} />
            <Route path="/*" element={
              <ProtectedRoute>
                <FilialProvider>
                  <AppLayout />
                </FilialProvider>
              </ProtectedRoute>
            } />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

export default App
