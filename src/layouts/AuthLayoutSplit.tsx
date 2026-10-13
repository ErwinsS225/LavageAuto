/**
 * Layout split LavPro pour pages auth (sans la carte centrée centrée existante).
 * Utilisé quand on remplace complètement les pages connexion/inscription.
 */
import { Outlet } from 'react-router-dom'
import { ToastProvider } from '@/components/Toaster'

export default function AuthLayoutSplit() {
  return (
    <>
      <Outlet />
      <ToastProvider>
        <span />
      </ToastProvider>
    </>
  )
}
