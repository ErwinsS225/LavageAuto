/**
 * Layout plateforme (superadmin) : navigation dédiée et garde d'accès.
 *
 * Même langage visuel que l'application : barre latérale en verre, indicateur
 * actif, transition d'écran — pour qu'un membre de l'équipe WashGuard passe de
 * l'un à l'autre sans changer de repères.
 */
import { Suspense } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { Building2, CreditCard, LayoutDashboard, LogOut, ShieldCheck } from 'lucide-react'
import clsx from 'clsx'

import { Brand } from '@/components/Brand'
import { PageFallback, Spinner } from '@/components/ui'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { pageVariants, DUR, EASE_SNAP } from '@/lib/motion'

// Ces adresses doivent être celles déclarées dans App.tsx : sinon chaque clic
// du menu de la plateforme tombait sur « page introuvable ».
const NAV = [
  { to: '/administration', label: 'Vue générale', icon: LayoutDashboard },
  { to: '/administration/entreprises', label: 'Organisations', icon: Building2 },
  { to: '/administration/formules', label: 'Formules', icon: CreditCard },
]

export default function AdminLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { me, loading, logout } = useAuth()
  usePageTitle('Plateforme WashGuard')

  if (loading) return <Spinner label="Chargement…" />
  if (!me || me.user.role !== 'superadmin') {
    return (
      <div className="app-aurora flex min-h-dvh flex-col items-center justify-center gap-3 bg-canvas px-4 text-center">
        <span className="flex h-12 w-12 items-center justify-center rounded-card border border-line bg-surface text-ink-muted shadow-e2">
          <ShieldCheck size={22} />
        </span>
        <p className="font-semibold text-ink">Espace réservé à la plateforme</p>
        <p className="max-w-sm text-sm text-ink-muted">
          Cette section nécessite un compte support WashGuard (superadmin).
        </p>
      </div>
    )
  }

  return (
    <div className="app-aurora flex min-h-dvh bg-canvas">
      <aside className="sticky top-0 hidden h-dvh w-60 shrink-0 flex-col border-r border-line bg-surface/95 backdrop-blur-glass md:flex">
        <div className="border-b border-line px-3.5 py-3">
          <Brand subtitle="Espace plateforme" tone="plateforme" />
        </div>
        <p className="px-3.5 pt-3 text-[0.6875rem] font-semibold uppercase tracking-[0.1em] text-ink-subtle">
          Plateforme WashGuard
        </p>
        <nav className="flex-1 space-y-0.5 p-3">
          {NAV.map((item, index) => (
            <motion.div
              key={item.to}
              initial={{ opacity: 0, x: -8 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: DUR.base, ease: EASE_SNAP, delay: index * 0.03 }}
            >
              <NavLink
                to={item.to}
                end={item.to === '/administration'}
                className={({ isActive }) => clsx('nav-link', isActive && 'nav-link-active')}
              >
                <item.icon size={17} />
                {item.label}
              </NavLink>
            </motion.div>
          ))}
        </nav>
        <div className="border-t border-line p-3">
          <p className="mb-2 truncate px-2 text-xs text-ink-muted">{me.user.email}</p>
          <button
            type="button"
            onClick={() => void logout().then(() => navigate('/connexion'))}
            className="nav-link w-full cursor-pointer text-left"
          >
            <LogOut size={16} /> Déconnexion
          </button>
        </div>
      </aside>
      <main className="min-w-0 flex-1 p-4 lg:p-6">
        {/* Les écrans de la plateforme sont chargés à la demande : sans cette
            limite, le premier affichage suspend le rendu et l'espace reste
            vide (même symptôme que l'écran noir). */}
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={location.pathname}
            variants={pageVariants}
            initial="hidden"
            animate="visible"
            exit="exit"
            className="mx-auto w-full max-w-[90rem]"
          >
            <Suspense fallback={<PageFallback />}>
              <Outlet />
            </Suspense>
          </motion.div>
        </AnimatePresence>
      </main>
    </div>
  )
}
