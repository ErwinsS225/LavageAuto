import { Suspense, useEffect, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { BarChart3, Car, ChevronDown, Contact, CreditCard, Droplets, HandCoins, LayoutDashboard, LogOut, Menu, Moon, Receipt, Rocket, Settings, SlidersHorizontal, Sparkles, Sun, UserRound, Users, Wallet, X } from 'lucide-react'
import clsx from 'clsx'

import { Brand } from '@/components/Brand'
import { PageFallback } from '@/components/ui'
import { IconButton } from '@/components/ui/primitives'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation, useTheme } from '@/hooks/useStation'
import { formatDate } from '@/lib/format'
import { drawerVariants, menuVariants, pageVariants, DUR, EASE_SNAP } from '@/lib/motion'
import { subscriptionStatusLabel, subscriptionTone } from '@/lib/subscription'

interface NavItem {
  to: string
  label: string
  icon: typeof LayoutDashboard
  permission?: string
  /** Rôles concernés lorsque l'écran est personnel (ex. : l'espace du laveur,
   *  dont la permission est partagée avec l'encadrement). */
  roles?: string[]
}

/** Ordre de navigation demandé : tableau de bord, lavages, prestations... */
const NAV_ITEMS: NavItem[] = [
  // Mise en route : visible seulement tant que la station n'est pas activée
  // (voir visibleItems) et pour les profils qui la configurent.
  { to: '/onboarding', label: 'Mise en route', icon: Rocket, permission: 'settings:view' },
  { to: '/tableau-de-bord', label: 'Tableau de bord', icon: LayoutDashboard, permission: 'dashboard:view' },
  // Écran personnel du laveur : les autres rôles possèdent la permission
  // (ils peuvent ouvrir l'adresse), mais « Mes lavages » ne veut rien dire
  // pour un propriétaire ou un caissier, qui ont déjà l'écran Lavages.
  { to: '/espace-laveur', label: 'Mes lavages', icon: Droplets, permission: 'washer:view', roles: ['washer'] },
  { to: '/lavages', label: 'Lavages', icon: Car, permission: 'wash:view' },
  { to: '/prestations', label: 'Prestations', icon: Sparkles, permission: 'service:view' },
  { to: '/tarifs', label: 'Tarifs', icon: SlidersHorizontal, permission: 'service:view' },
  { to: '/clients', label: 'Clients', icon: Contact, permission: 'customer:view' },
  { to: '/employes', label: 'Employés', icon: Users, permission: 'employee:view' },
  { to: '/caisse', label: 'Caisse', icon: Wallet, permission: 'cash:view' },
  { to: '/depenses', label: 'Dépenses', icon: Receipt, permission: 'expense:view' },
  { to: '/rapports', label: 'Rapports', icon: BarChart3, permission: 'report:view' },
  { to: '/commissions', label: 'Commissions', icon: HandCoins, permission: 'commission:view' },
  { to: '/parametres', label: 'Paramètres', icon: Settings, permission: 'settings:view' },
  { to: '/abonnement', label: 'Abonnement', icon: CreditCard, permission: 'subscription:view' },
]

/**
 * Pendant la mise en route, le menu se limite aux écrans qui servent à la
 * terminer (et à l'abonnement) : inutile de proposer une caisse qui refusera
 * l'ouverture. Le serveur applique la même règle de son côté.
 */
const MENU_MISE_EN_ROUTE = ['/onboarding', '/prestations', '/tarifs', '/parametres', '/abonnement']

/** Coquille de l'application : barre latérale, en-tête et contenu. */
export default function AppLayout() {
  return <Shell />
}

function Shell() {
  const { me, logout } = useAuth()
  const { theme, toggle } = useTheme()
  const { stationId, selectStation, stations } = useStation()
  const location = useLocation()
  const navigate = useNavigate()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [menuOpen, setMenuOpen] = useState(false)
  // Un caissier ou un gérant rattaché à une station n'en voit qu'une seule.

  // Le menu suit exactement les permissions renvoyées par le serveur : un
  // caissier ne voit pas les rapports ni les paramètres, par exemple.
  const miseEnRoute = me?.onboarding?.state
  const enConfiguration = miseEnRoute === 'pending' || miseEnRoute === 'in_progress'
  const visibleItems = NAV_ITEMS.filter(
    (item) =>
      (!item.permission || me?.permissions.includes(item.permission)) &&
      (!item.roles || item.roles.includes(me?.user.role ?? '')) &&
      // Une fois la station activée, l'entrée « Mise en route » disparaît
      // d'elle-même ; avant, le menu se limite aux écrans de la configuration.
      (item.to !== '/onboarding' || enConfiguration) &&
      (!enConfiguration || MENU_MISE_EN_ROUTE.includes(item.to)),
  )

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

  const navigation = (
    <nav className="flex-1 space-y-0.5 overflow-y-auto px-3 py-3">
      {visibleItems.map((item, index) => (
        <motion.div
          key={item.to}
          initial={{ opacity: 0, x: -8 }}
          animate={{ opacity: 1, x: 0 }}
          transition={{ duration: DUR.base, ease: EASE_SNAP, delay: Math.min(index * 0.025, 0.2) }}
        >
          <NavLink to={item.to} className={({ isActive }) => clsx('nav-link', isActive && 'nav-link-active')}>
            <item.icon size={17} />
            {item.label}
          </NavLink>
        </motion.div>
      ))}
    </nav>
  )

  const piedDeBarre = (
    <div className="border-t border-line px-3 py-3">
      {subscription ? (
        <div className="panel mb-2 px-3 py-2.5">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-medium text-ink-muted">Abonnement</span>
            <span className={clsx('badge', subscriptionTone(subscription.status))}>
              {subscriptionStatusLabel(subscription.status)}
            </span>
          </div>
          <p className="mt-1.5 truncate text-xs text-ink">
            {subscription.plan_name || 'Aucune formule'}
            {subscription.trial_ends_at ? ` — essai jusqu’au ${formatDate(subscription.trial_ends_at)}` : ''}
          </p>
        </div>
      ) : null}
      <button type="button" onClick={handleLogout} className="nav-link w-full cursor-pointer text-left">
        <LogOut size={17} />
        Se déconnecter
      </button>
    </div>
  )

  const enTeteDeBarre = (
    <div className="flex items-center justify-between gap-2 border-b border-line px-3.5 py-3">
      <Link to="/tableau-de-bord" className="min-w-0">
        <Brand />
      </Link>
      <IconButton
        label="Fermer le menu"
        variant="ghost"
        className="lg:hidden"
        icon={<X size={17} />}
        onClick={() => setSidebarOpen(false)}
      />
    </div>
  )

  return (
    <div className="app-aurora flex min-h-dvh bg-canvas">
      {/* Barre latérale — fixe sur grand écran */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-64 flex-col border-r border-line bg-surface/95 backdrop-blur-glass lg:flex">
        {enTeteDeBarre}
        <div className="border-b border-line px-3.5 py-2.5">
          <p className="truncate text-sm font-medium text-ink">{organization?.name ?? 'WashGuard'}</p>
          <p className="truncate text-xs text-ink-muted">{station?.name ?? 'Aucune station'}</p>
        </div>
        {navigation}
        {piedDeBarre}
      </aside>

      {/* Barre latérale — tiroir sur mobile */}
      <AnimatePresence>
        {sidebarOpen ? (
          <>
            <motion.button
              type="button"
              aria-label="Fermer le menu"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={{ duration: DUR.fast, ease: EASE_SNAP }}
              className="fixed inset-0 z-40 cursor-default bg-ink/50 backdrop-blur-[3px] lg:hidden"
              onClick={() => setSidebarOpen(false)}
            />
            <motion.aside
              variants={drawerVariants}
              initial="hidden"
              animate="visible"
              exit="exit"
              className="fixed inset-y-0 left-0 z-50 flex w-64 flex-col border-r border-line bg-surface shadow-float lg:hidden"
            >
              {enTeteDeBarre}
              <div className="border-b border-line px-3.5 py-2.5">
                <p className="truncate text-sm font-medium text-ink">{organization?.name ?? 'WashGuard'}</p>
                <p className="truncate text-xs text-ink-muted">{station?.name ?? 'Aucune station'}</p>
              </div>
              {navigation}
              {piedDeBarre}
            </motion.aside>
          </>
        ) : null}
      </AnimatePresence>

      {/* Contenu */}
      <div className="flex min-w-0 flex-1 flex-col lg:pl-64">
        <header className="sticky top-0 z-20 flex items-center gap-2 border-b border-line bg-surface/85 px-3 py-2.5 backdrop-blur-glass sm:px-4">
          <IconButton
            label="Ouvrir le menu"
            variant="ghost"
            className="lg:hidden"
            icon={<Menu size={18} />}
            onClick={() => setSidebarOpen(true)}
          />

          <div className="flex-1" />

          {stations.length > 1 ? (
            <label className="flex items-center gap-2 text-xs text-ink-muted">
              <span className="hidden sm:inline">Station</span>
              <select
                value={stationId}
                onChange={(event) => selectStation(event.target.value)}
                className="select w-auto max-w-52 py-1.5 text-xs"
              >
                {stations.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          ) : null}

          <IconButton
            label={theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
            variant="ghost"
            icon={theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
            onClick={toggle}
          />

          <div className="relative">
            <motion.button
              type="button"
              whileTap={{ scale: 0.97 }}
              onClick={() => setMenuOpen((open) => !open)}
              className="flex cursor-pointer items-center gap-2 rounded-field border border-line bg-surface px-2.5 py-1.5 text-sm shadow-e1 transition-colors duration-200 ease-soft hover:border-line-strong hover:bg-surface-muted"
            >
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-brand-soft text-xs font-semibold text-brand ring-1 ring-brand/20">
                {me.user.full_name.slice(0, 1).toUpperCase()}
              </span>
              <span className="hidden max-w-28 truncate sm:inline">{me.user.full_name}</span>
              <motion.span animate={{ rotate: menuOpen ? 180 : 0 }} transition={{ duration: DUR.fast }}>
                <ChevronDown size={14} className="text-ink-muted" />
              </motion.span>
            </motion.button>
            <AnimatePresence>
              {menuOpen ? (
                <motion.div
                  variants={menuVariants}
                  initial="hidden"
                  animate="visible"
                  exit="exit"
                  className="card-glass absolute right-0 z-30 mt-2 w-60 overflow-hidden p-1.5 shadow-float"
                >
                  <div className="border-b border-line px-2.5 pb-2 pt-1">
                    <p className="truncate text-sm font-medium text-ink">{me.user.full_name}</p>
                    <p className="truncate text-xs text-ink-muted">{me.user.email}</p>
                    <p className="mt-0.5 text-xs text-brand">{me.user.role_label ?? me.user.role}</p>
                  </div>
                  <Link to="/mon-compte" className="nav-link mt-1" onClick={() => setMenuOpen(false)}>
                    <UserRound size={16} />
                    Mon compte
                  </Link>
                  <button type="button" onClick={handleLogout} className="nav-link w-full cursor-pointer text-left">
                    <LogOut size={16} />
                    Se déconnecter
                  </button>
                </motion.div>
              ) : null}
            </AnimatePresence>
          </div>
        </header>

        <AnimatePresence initial={false}>
          {me.must_change_password ? (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              exit={{ opacity: 0, height: 0 }}
              transition={{ duration: DUR.base, ease: EASE_SNAP }}
              className="border-b border-brand/30 bg-brand-soft px-4 text-sm text-brand"
            >
              <p className="py-2.5">
                Votre mot de passe est provisoire.{' '}
                <Link to="/mon-compte" className="font-medium underline">
                  Choisissez maintenant votre mot de passe
                </Link>{' '}
                pour sécuriser votre compte.
              </p>
            </motion.div>
          ) : null}
        </AnimatePresence>

        <AnimatePresence initial={false}>
          {blocked && subscription ? (
            <motion.div
              initial={{ opacity: 0, height: 0 }}
              animate={{ opacity: 1, height: 'auto' }}
              exit={{ opacity: 0, height: 0 }}
              transition={{ duration: DUR.base, ease: EASE_SNAP }}
              className="border-b border-warning/30 bg-warning-soft px-4 text-sm text-warning"
            >
              <p className="py-2.5">
                Votre abonnement est {subscriptionStatusLabel(subscription.status).toLowerCase()} : la saisie est
                bloquée, mais vos données restent consultables.{' '}
                <Link to="/abonnement" className="font-medium underline">
                  Voir les formules
                </Link>
              </p>
            </motion.div>
          ) : null}
        </AnimatePresence>

        <main className="min-w-0 flex-1 px-4 py-5 sm:px-6 sm:py-6">
          {/* Transition d'écran : le contenu se pose (fondu + 8 px), sans
              glissement latéral qui donnerait un effet de diaporama. */}
          <AnimatePresence mode="wait" initial={false}>
            <motion.div
              key={location.pathname}
              variants={pageVariants}
              initial="hidden"
              animate="visible"
              exit="exit"
              className="mx-auto w-full max-w-[100rem]"
            >
              <Suspense fallback={<PageFallback />}>
                <Outlet />
              </Suspense>
            </motion.div>
          </AnimatePresence>
        </main>
      </div>
    </div>
  )
}
