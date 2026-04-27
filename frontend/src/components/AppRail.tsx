import { LayoutDashboard, Upload, Settings, LogOut, KeyRound, History, FileDown, Repeat2, Building2, TrendingUp } from 'lucide-react'
import { useLocation, useNavigate } from 'react-router-dom'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useAuth } from '@/contexts/AuthContext'
import { getActiveModule } from '@/lib/navigation'
import { CompanySwitcher } from '@/components/CompanySwitcher'
import { useState } from 'react'
import { toast } from 'sonner'

const mainItems = [
  { id: 'dashboard',    icon: LayoutDashboard, label: 'Painel de Calibragem', path: '/dashboard/urgencia/falta' },
  { id: 'upload',       icon: Upload,          label: 'Importação CSV',       path: '/upload/csv' },
  { id: 'historico',    icon: History,         label: 'Histórico',            path: '/historico' },
  { id: 'pdf',          icon: FileDown,        label: 'Gerar PDF',            path: '/pdf/gerar' },
  { id: 'reincidencia', icon: Repeat2,         label: 'Reincidência',         path: '/reincidencia' },
  { id: 'resultados',   icon: TrendingUp,      label: 'Painel de Resultados', path: '/resultados' },
  // Administração: visível para todos (admin + gestores)
  { id: 'gestao',       icon: Building2,       label: 'Administração',        path: '/gestao/filiais' },
] as const

export function AppRail() {
  const location = useLocation()
  const navigate = useNavigate()
  const { user, company, logout, token, spRole } = useAuth()
  // isAdmin: plataforma admin OU admin_fbtax Farol → acessa configurações e manutenção
  const isAdmin = user?.role === 'admin' || spRole === 'admin_fbtax'
  const active = getActiveModule(location.pathname)
  // Optimistic: mostra enquanto spRole carrega (null); esconde apenas se confirmado somente_leitura
  const canAccessResultados = spRole !== 'somente_leitura'

  const [pwDialog,  setPwDialog]  = useState(false)
  const [pwCurrent, setPwCurrent] = useState('')
  const [pwNew,     setPwNew]     = useState('')
  const [pwConfirm, setPwConfirm] = useState('')
  const [pwLoading, setPwLoading] = useState(false)

  async function handleChangePassword() {
    if (pwNew !== pwConfirm) { toast.error('As senhas não coincidem'); return }
    if (pwNew.length < 6)    { toast.error('Mínimo 6 caracteres'); return }
    setPwLoading(true)
    try {
      const res = await fetch('/api/auth/change-password', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body:    JSON.stringify({ current_password: pwCurrent, new_password: pwNew }),
      })
      const data = await res.json()
      if (!res.ok) { toast.error(data.error || 'Erro ao alterar senha'); return }
      toast.success('Senha alterada com sucesso')
      setPwDialog(false); setPwCurrent(''); setPwNew(''); setPwConfirm('')
    } catch {
      toast.error('Erro de conexão')
    } finally {
      setPwLoading(false)
    }
  }

  const initials = user?.full_name
    ?.split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map(n => n[0])
    .join('')
    .toUpperCase() ?? '?'

  const trialDate = user?.trial_ends_at
    ? new Date(user.trial_ends_at).toLocaleDateString('pt-BR')
    : null

  return (
    <TooltipProvider delayDuration={200}>
      <div className="flex flex-col w-14 shrink-0 border-r bg-white h-screen z-20">

        {/* Logo */}
        <div className="flex items-center justify-center h-14 border-b shrink-0">
          <img src="/logo-fb.png" alt="Fortes Bezerra" className="size-8 rounded-lg object-cover" />
        </div>

        {/* Nav principal */}
        <nav className="flex flex-col items-center gap-1 p-2 flex-1 pt-3">
          {mainItems.filter(item => item.id !== 'resultados' || canAccessResultados).map(item => (
              <Tooltip key={item.id}>
                <TooltipTrigger asChild>
                  <button
                    onClick={() => navigate(item.path)}
                    className={cn(
                      'flex items-center justify-center w-10 h-10 rounded-lg transition-colors',
                      active === item.id
                        ? 'bg-primary/10 text-primary'
                        : 'text-muted-foreground hover:bg-gray-100 hover:text-foreground'
                    )}
                  >
                    <item.icon className="h-5 w-5" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="right" className="text-xs">
                  {item.label}
                </TooltipContent>
              </Tooltip>
            ))}
        </nav>

        {/* Config (admin) + User */}
        <div className="pb-1">
          <span className="text-[9px] text-muted-foreground/50 select-none">v{__APP_VERSION__}</span>
        </div>

        <div className="flex flex-col items-center gap-1 p-2 border-t shrink-0">
          {isAdmin && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={() => navigate('/config/planos')}
                className={cn(
                  'flex items-center justify-center w-10 h-10 rounded-lg transition-colors',
                  active === 'config'
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:bg-gray-100 hover:text-foreground'
                )}
              >
                <Settings className="h-5 w-5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="right" className="text-xs">Configurações</TooltipContent>
          </Tooltip>
          )}

          <DropdownMenu>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuTrigger asChild>
                  <button className="flex items-center justify-center w-10 h-10 rounded-lg bg-primary/10 text-primary text-xs font-bold hover:bg-primary/20 transition-colors">
                    {initials}
                  </button>
                </DropdownMenuTrigger>
              </TooltipTrigger>
              <TooltipContent side="right" className="text-xs">{user?.full_name}</TooltipContent>
            </Tooltip>

            <DropdownMenuContent side="right" align="end" className="w-52">
              <DropdownMenuLabel className="pb-1">
                <p className="text-xs font-semibold leading-tight truncate">{user?.full_name}</p>
                <p className="text-[10px] text-muted-foreground font-normal truncate mt-0.5">{company}</p>
              </DropdownMenuLabel>

              {trialDate && (
                <>
                  <DropdownMenuSeparator />
                  <div className="px-2 py-1">
                    <span className="inline-flex items-center text-[10px] bg-yellow-50 text-yellow-700 border border-yellow-200 px-1.5 py-0.5 rounded font-medium">
                      Licença vence: {trialDate}
                    </span>
                  </div>
                </>
              )}

              <DropdownMenuSeparator />
              <DropdownMenuItem className="text-xs cursor-pointer" onClick={() => setPwDialog(true)}>
                <KeyRound className="mr-2 h-3.5 w-3.5" />
                Trocar senha
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <div className="px-1 py-0.5">
                <CompanySwitcher compact />
              </div>
              <DropdownMenuSeparator />
              <DropdownMenuItem className="text-xs text-red-600 focus:text-red-600 cursor-pointer" onClick={logout}>
                <LogOut className="mr-2 h-3.5 w-3.5" />
                Sair
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <Dialog
        open={pwDialog}
        onOpenChange={o => { setPwDialog(o); if (!o) { setPwCurrent(''); setPwNew(''); setPwConfirm('') } }}
      >
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Trocar Senha</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-1.5">
              <Label htmlFor="pw-current">Senha atual</Label>
              <Input id="pw-current" type="password" value={pwCurrent} onChange={e => setPwCurrent(e.target.value)} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="pw-new">Nova senha</Label>
              <Input id="pw-new" type="password" value={pwNew} onChange={e => setPwNew(e.target.value)} />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="pw-confirm">Confirmar nova senha</Label>
              <Input
                id="pw-confirm"
                type="password"
                value={pwConfirm}
                onChange={e => setPwConfirm(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleChangePassword()}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPwDialog(false)}>Cancelar</Button>
            <Button onClick={handleChangePassword} disabled={pwLoading || !pwCurrent || !pwNew || !pwConfirm}>
              {pwLoading ? 'Salvando...' : 'Salvar'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </TooltipProvider>
  )
}
