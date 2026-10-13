/**
 * Layout plateforme (superadmin) : navigation dédiée et garde d'accès.
 */
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { Building2, CreditCard, LayoutDashboard, LogOut, ShieldCheck } from 'lucide-react'

import { ToastProvider, useToast as Toaster } from '@/components/Toaster'
import { Spinner } from '@/components/ui'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'

const NAV = [
  { to: '/admin', label: 'Vue générale', icon: LayoutDashboard },
  { to: '/admin/organisations', label: 'Organisations', icon: Building2 },
  { to: '/admin/formules', label: 'Formules', icon: CreditCard },
]

export default function AdminLayout() {
  const navigate = useNavigate()
  const { me, loading, logout } = useAuth()
  usePageTitle('Plateforme WashGuard')

  if (loading) return <Spinner label="Chargement…" />
  if (!me || me.user.role !== 'superadmin') {
    return (
      <div className="flex min-h-dvh flex-col items-center justify-center gap-3 bg-surface-muted px-4 text-center">
        <ShieldCheck size={28} className="text-ink-muted" />
        <p className="font-medium text-ink">Espace réservé à la plateforme</p>
        <p className="max-w-sm text-sm text-ink-muted">
          Cette section nécessite un compte support WashGuard (superadmin).
        </p>
      </div>
    )
  }

  return (
    <div className="flex min-h-dvh bg-surface-muted">
      <aside className="hidden w-56 shrink-0 flex-col border-r border-line bg-surface md:flex">
        <div className="border-b border-line p-4">
          <p className="flex items-center gap-2 font-bold text-brand">
            <ShieldCheck size={20} /> Plateforme
          </p>
          <p className="mt-0.5 text-xs text-ink-muted">{me.user.email}</p>
        </div>
        <nav className="flex-1 space-y-0.5 p-3">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === '/admin'}
              className={({ isActive }) =>
                `flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium ${isActive ? 'bg-brand/10 text-brand' : 'text-ink-muted hover:bg-surface-muted hover:text-ink'
                }`
              }
            >
              <item.icon size={17} />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="border-t border-line p-3">
          <button
            type="button"
            onClick={() => void logout().then(() => navigate('/connexion'))}
            className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-ink-muted hover:bg-surface-muted hover:text-ink"
          >
            <LogOut size={16} /> Déconnexion
          </button>
        </div>
      </aside>
      <main className="min-w-0 flex-1 p-4 lg:p-6">
        <Outlet />
      </main>
      <ToastProvider><span /></ToastProvider>
    </div>
  )
}
