import { Suspense, useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  BarChart3,
  Car,
  ChevronDown,
  Contact,
  CreditCard,
  Droplets,
  HandCoins,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  Receipt,
  Settings,
  Sparkles,
  Sun,
  UserRound,
  Users,
  Wallet,
  X,
} from 'lucide-react'
import clsx from 'clsx'

import { PageFallback } from '@/components/ui'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { ThemeProvider, useStation, useTheme } from '@/hooks/useStation.tsx'
import { useStations } from '@/features/stations/api'
import { formatDate } from '@/lib/format'
import { subscriptionStatusLabel, subscriptionTone } from '@/lib/subscription'

interface NavItem {
  to: string
  label: string
  icon: typeof LayoutDashboard
  permission?: string
}

/** Ordre de navigation demandé : tableau de bord, lavages, prestations... */
const NAV_ITEMS: NavItem[] = [
  { to: '/tableau-de-bord', label: 'Tableau de bord', icon: LayoutDashboard, permission: 'dashboard:view' },
  { to: '/espace-laveur', label: 'Mes lavages', icon: Droplets, permission: 'washer:view' },
  { to: '/lavages', label: 'Lavages', icon: Car, permission: 'wash:view' },
  { to: '/prestations', label: 'Prestations', icon: Sparkles, permission: 'service:view' },
  { to: '/clients', label: 'Clients', icon: Contact, permission: 'customer:view' },
  { to: '/employes', label: 'Employés', icon: Users, permission: 'employee:view' },
  { to: '/caisse', label: 'Caisse', icon: Wallet, permission: 'cash:view' },
  { to: '/depenses', label: 'Dépenses', icon: Receipt, permission: 'expense:view' },
  { to: '/rapports', label: 'Rapports', icon: BarChart3, permission: 'report:view' },
  { to: '/commissions', label: 'Commissions', icon: HandCoins, permission: 'commission:view' },
  { to: '/parametres', label: 'Paramètres', icon: Settings, permission: 'settings:view' },
  { to: '/abonnement', label: 'Abonnement', icon: CreditCard, permission: 'subscription:view' },
]

/** Coquille de l'application : barre latérale, en-tête et contenu. */
export default function AppLayout() {
  return (
    <ThemeProvider>
      <Shell />
    </ThemeProvider>
  )
}

function Shell() {
  const { me, logout } = useAuth()
  const { theme, toggle } = useTheme()
  const { stationId, selectStation } = useStation()
  const location = useLocation()
  const navigate = useNavigate()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  const stationsQuery = useStations(Boolean(me))
  // Un caissier ou un gérant rattaché à une station n'en voit qu'une seule.
  const stations = stationsQuery.data ?? []

  // Le menu suit exactement les permissions renvoyées par le serveur : un
  // caissier ne voit pas les rapports ni les paramètres, par exemple.
  const visibleItems = NAV_ITEMS.filter((item) => !item.permission || me?.permissions.includes(item.permission))

  // Le titre de l'onglet suit la même liste que la barre latérale : un écran
  // ajouté au menu a donc son titre, sans rien inscrire de plus.
  const pageCourante = NAV_ITEMS.find((item) => location.pathname.startsWith(item.to))
  usePageTitle(pageCourante?.label ?? 'WashGuard')

  useEffect(() => {
    setSidebarOpen(false)
    setMenuOpen(false)
  }, [location.pathname])

  if (!me) return null

  const organization = me.organization
  const station = me.station
  const subscription = me.subscription
  // Un compte de plateforme n'a ni entreprise ni abonnement : on n'affiche pas
  // le bloc correspondant plutôt que de casser la page.
  const blocked = Boolean(
    subscription && ['expired', 'suspended', 'canceled'].includes(subscription.status),
  )

  async function handleLogout() {
    await logout()
    navigate('/connexion', { replace: true })
  }

  return (
    <div className="flex min-h-full bg-canvas">
      {/* Barre latérale */}
      <aside
        className={clsx(
          'fixed inset-y-0 left-0 z-30 flex w-64 flex-col border-r border-line bg-surface transition-transform lg:static lg:translate-x-0',
          sidebarOpen ? 'translate-x-0' : '-translate-x-full',
        )}
      >
        <div className="flex items-center justify-between border-b border-line px-4 py-3.5">
          <Link to="/tableau-de-bord" className="flex items-center gap-2 text-brand">
            <Droplets size={22} />
            <span className="font-semibold">WashGuard</span>
          </Link>
          <button
            type="button"
            className="cursor-pointer rounded-lg p-1.5 text-ink-muted hover:bg-surface-muted lg:hidden"
            onClick={() => setSidebarOpen(false)}
            aria-label="Fermer le menu"
          >
            <X size={18} />
          </button>
        </div>

        <div className="border-b border-line px-4 py-3">
          <p className="truncate text-sm font-medium text-ink">{organization?.name ?? 'WashGuard'}</p>
          <p className="truncate text-xs text-ink-muted">{station?.name ?? 'Aucune station'}</p>
        </div>

        <nav className="flex-1 space-y-1 overflow-y-auto p-3">
          {visibleItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) => clsx('nav-link', isActive && 'nav-link-active')}
            >
              <item.icon size={17} />
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="border-t border-line p-3">
          {subscription ? (
            <div className="mb-2 rounded-lg bg-surface-muted p-2.5">
              <div className="flex items-center justify-between gap-2">
                <span className="text-xs font-medium text-ink-muted">Abonnement</span>
                <span className={clsx('badge', subscriptionTone(subscription.status))}>
                  {subscriptionStatusLabel(subscription.status)}
                </span>
              </div>
              <p className="mt-1 truncate text-xs text-ink">
                {subscription.plan_name || 'Aucune formule'}
                {subscription.trial_ends_at ? ` — essai jusqu’au ${formatDate(subscription.trial_ends_at)}` : ''}
              </p>
            </div>
          ) : null}
          <button
            type="button"
            onClick={handleLogout}
            className="nav-link w-full cursor-pointer text-left"
          >
            <LogOut size={17} />
            Se déconnecter
          </button>
        </div>
      </aside>

      {sidebarOpen ? (
        <button
          type="button"
          aria-label="Fermer le menu"
          className="fixed inset-0 z-20 cursor-default bg-black/50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      ) : null}

      {/* Contenu */}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-10 flex items-center gap-3 border-b border-line bg-surface/95 px-4 py-2.5 backdrop-blur">
          <button
            type="button"
            className="cursor-pointer rounded-lg p-2 text-ink-muted hover:bg-surface-muted lg:hidden"
            onClick={() => setSidebarOpen(true)}
            aria-label="Ouvrir le menu"
          >
            <Menu size={18} />
          </button>

          <div className="flex-1" />

          {stations.length > 1 ? (
            <label className="flex items-center gap-2 text-xs text-ink-muted">
              <span className="hidden sm:inline">Station</span>
              <select
                value={stationId}
                onChange={(event) => selectStation(event.target.value)}
                className="input max-w-52 py-1.5 text-xs"
              >
                {stations.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          ) : null}

          <button
            type="button"
            onClick={toggle}
            className="cursor-pointer rounded-lg p-2 text-ink-muted transition hover:bg-surface-muted hover:text-ink"
            aria-label={theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
            title={theme === 'dark' ? 'Thème clair' : 'Thème sombre'}
          >
            {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
          </button>

          <div className="relative">
            <button
              type="button"
              onClick={() => setMenuOpen((open) => !open)}
              className="flex cursor-pointer items-center gap-2 rounded-lg border border-line px-2.5 py-1.5 text-sm transition hover:bg-surface-muted"
            >
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-brand-soft text-xs font-semibold text-brand">
                {me.user.full_name.slice(0, 1).toUpperCase()}
              </span>
              <span className="hidden max-w-28 truncate sm:inline">{me.user.full_name}</span>
              <ChevronDown size={14} className="text-ink-muted" />
            </button>
            {menuOpen ? (
              <div className="absolute right-0 z-20 mt-1.5 w-60 rounded-xl border border-line bg-surface p-1.5 shadow-xl">
                <div className="border-b border-line px-2.5 pb-2 pt-1">
                  <p className="truncate text-sm font-medium text-ink">{me.user.full_name}</p>
                  <p className="truncate text-xs text-ink-muted">{me.user.email}</p>
                  <p className="mt-0.5 text-xs text-brand">{me.user.role_label ?? me.user.role}</p>
                </div>
                <Link
                  to="/mon-compte"
                  className="nav-link mt-1"
                  onClick={() => setMenuOpen(false)}
                >
                  <UserRound size={16} />
                  Mon compte
                </Link>
                <button type="button" onClick={handleLogout} className="nav-link w-full cursor-pointer text-left">
                  <LogOut size={16} />
                  Se déconnecter
                </button>
              </div>
            ) : null}
          </div>
        </header>

        {me.must_change_password ? (
          <div className="border-b border-brand/40 bg-brand-soft px-4 py-2.5 text-sm text-brand">
            Votre mot de passe est provisoire.{' '}
            <Link to="/mon-compte" className="font-medium underline">
              Choisissez maintenant votre mot de passe
            </Link>{' '}
            pour sécuriser votre compte.
          </div>
        ) : null}

        {blocked && subscription ? (
          <div className="border-b border-warning/40 bg-warning-soft px-4 py-2.5 text-sm text-warning">
            Votre abonnement est {subscriptionStatusLabel(subscription.status).toLowerCase()} : la saisie est bloquée,
            mais vos données restent consultables.{' '}
            <Link to="/abonnement" className="font-medium underline">
              Voir les formules
            </Link>
          </div>
        ) : null}

        <main className="min-w-0 flex-1 p-4 sm:p-6">
          <Suspense fallback={<PageFallback />}>
            <Outlet />
          </Suspense>
        </main>
      </div>
    </div>
  )
}
