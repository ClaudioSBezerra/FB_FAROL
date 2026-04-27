// useUsageTracker — rastreia tempo de permanência por página/módulo
// Envia POST /api/sp/uso a cada troca de rota e no fechamento da aba.
// Não bloqueia a navegação (fire-and-forget com keepalive).

import { useEffect, useRef } from 'react'
import { useLocation } from 'react-router-dom'
import { useAuth } from '@/contexts/AuthContext'
import { getActiveModule } from '@/lib/navigation'

function getOrCreateSessaoId(): string {
  let id = sessionStorage.getItem('sp_sessao_id')
  if (!id) {
    id = crypto.randomUUID()
    sessionStorage.setItem('sp_sessao_id', id)
  }
  return id
}

export function useUsageTracker() {
  const { token, isAuthenticated } = useAuth()
  const location = useLocation()

  // Refs mantêm os valores atuais sem re-criar os listeners
  const tokenRef    = useRef(token)
  const enterRef    = useRef(Date.now())
  const pathRef     = useRef(location.pathname)

  // Mantém o token sempre atualizado no ref
  useEffect(() => { tokenRef.current = token }, [token])

  function flush(path: string) {
    const t = tokenRef.current
    if (!t || !isAuthenticated) return

    const duracao = Math.round((Date.now() - enterRef.current) / 1000)
    if (duracao < 2) return   // ignora visitas instantâneas (ex: redirect)

    const modulo = getActiveModule(path)

    fetch('/api/sp/uso', {
      method: 'POST',
      headers: { Authorization: `Bearer ${t}`, 'Content-Type': 'application/json' },
      body: JSON.stringify({
        modulo,
        caminho: path,
        duracao_seg: duracao,
        sessao_id: getOrCreateSessaoId(),
      }),
      keepalive: true,  // sobrevive ao fechamento da página
    }).catch(() => {})  // nunca lança — rastreamento não deve quebrar a UI
  }

  // ── Troca de rota: envia duração da página anterior ──────────────────────
  useEffect(() => {
    const prev = pathRef.current
    if (prev !== location.pathname) {
      flush(prev)
      pathRef.current = location.pathname
      enterRef.current = Date.now()
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname])

  // ── Fechamento/saída da aba: envia duração da página atual ────────────────
  useEffect(() => {
    const handleUnload = () => flush(pathRef.current)
    window.addEventListener('beforeunload', handleUnload)
    return () => window.removeEventListener('beforeunload', handleUnload)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
}
