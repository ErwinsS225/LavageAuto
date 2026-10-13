/**
 * Layout public / auth minimal.
 */
import { Outlet } from 'react-router-dom'
import { ToastProvider } from '@/components/Toaster'

export default function AuthLayout() {
  return (
    <>
      <Outlet />
      <ToastProvider><span /></ToastProvider>
    </>
  )
}
