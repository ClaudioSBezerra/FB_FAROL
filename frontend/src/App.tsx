import { BrowserRouter, Routes, Route, Navigate, useLocation, Link } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Toaster } from '@/components/ui/sonner'
import CadastroGestores from './pages/CadastroGestores'
import CadastroRCAs from './pages/CadastroRCAs'
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
import { AppRail } from '@/components/AppRail'
import { CompanySwitcher } from '@/components/CompanySwitcher'
import { AjudaChat } from '@/components/AjudaChat'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { FilialProvider } from './contexts/FilialContext'
import { getActiveModule, modules } from '@/lib/navigation'
import { cn } from '@/lib/utils'

const queryClient = new QueryClient()


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

// ── Barra de abas por módulo ─────────────────────────────────────────────────
function ModuleTabs() {
  const location  = useLocation()
  const { user, spRole, group }  = useAuth()
  const isMaster  = group === 'MASTER'
  const isAdmin   = isMaster || spRole === 'admin_fbtax'
  const moduleId  = getActiveModule(location.pathname)
  const moduleCfg = modules[moduleId]

  if (!moduleCfg || moduleCfg.tabs.length === 0) return null
  if (moduleCfg.adminOnly && !isAdmin) return null

  const visibleTabs = moduleCfg.tabs.filter(t =>
    (!t.adminOnly  || isAdmin) &&
    (!t.masterOnly || isMaster)
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

  return (
    <header className="flex items-center justify-between h-12 border-b bg-white px-4 shrink-0">
      <span className="text-sm font-semibold text-foreground">
        {moduleCfg?.label ?? 'Farol'}
      </span>
      <div className="flex items-center gap-2">
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
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard/ampliar" replace />} />

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

              {/* Cadastros — Gestores e RCAs */}
              <Route path="/cadastros/gestores" element={<ProtectedRoute><CadastroGestores /></ProtectedRoute>} />
              <Route path="/cadastros/rcas"     element={<ProtectedRoute><CadastroRCAs /></ProtectedRoute>} />

              {/* Gestão de CD (gestor_filial+) */}
              <Route path="/gestao/filiais" element={<ProtectedRoute><SpAmbiente /></ProtectedRoute>} />
              <Route path="/gestao/regras"  element={<ProtectedRoute><SpAmbiente /></ProtectedRoute>} />

              {/* Configurações (admin) */}
              <Route path="/config/planos"      element={<ProtectedRoute><SpAmbiente /></ProtectedRoute>} />
              <Route path="/config/manutencao"  element={<ProtectedRoute><SpAmbiente /></ProtectedRoute>} />
              <Route path="/config/ambiente"    element={<ProtectedRoute><GestaoAmbiente /></ProtectedRoute>} />
              <Route path="/config/usuarios"    element={<MasterRoute><SpUsuarios /></MasterRoute>} />
              <Route path="/config/usuarios-admin" element={<MasterRoute><AdminUsers /></MasterRoute>} />
              <Route path="/config/audit-log"   element={<MasterRoute><SpAuditLog /></MasterRoute>} />
              <Route path="/config/empresas-bloqueio" element={<MasterRoute><SpEmpresasBloqueio /></MasterRoute>} />
              <Route path="/config/uso"         element={<MasterRoute><SpUsoSistema /></MasterRoute>} />
              <Route path="/config/destinatarios" element={<MasterRoute><SpDestinatarios /></MasterRoute>} />

              {/* Redirecionamentos de rotas antigas */}
              <Route path="/config/parametros-motor" element={<Navigate to="/gestao/regras" replace />} />
              <Route path="/config/gestao-ambiente"  element={<Navigate to="/config/ambiente" replace />} />
            </Routes>
          </div>
        </main>
      </div>
      <Toaster />
      <AjudaChat />
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
