import { lazy } from 'react'
import { Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom'

import { Spinner } from '@/components/ui'
import { useAuth } from '@/hooks/useAuth'
import AdminLayout from '@/layouts/AdminLayout'
import AppLayout from '@/layouts/AppLayout'
import AuthLayout from '@/layouts/AuthLayout'
import { sessionVientDExpirer } from '@/lib/session-signal'

// Les pages de connexion et les pages de secours sont chargées tout de suite :
// ce sont les seules qu'un visiteur voit avant d'être identifié.
import LandingPage from '@/pages/LandingPage'
import NotFoundPage from '@/pages/NotFoundPage'
import LoginPageNew from '@/pages/LoginPageNew'
import RegisterPageNew from '@/pages/RegisterPageNew'
import ForgotPasswordPage from '@/pages/ForgotPasswordPage'
import JoinPage from '@/pages/JoinPage'

// Les pages internes sont chargées à la demande : l'ouverture de session reste
// rapide, même sur une connexion mobile lente (le graphique est le plus lourd).
const DashboardPage = lazy(() => import('@/pages/DashboardPage'))
const WashOrdersPage = lazy(() => import('@/pages/WashOrdersPage'))
const ServicesPage = lazy(() => import('@/pages/ServicesPage'))
const TarifsPage = lazy(() => import('@/pages/TarifsPage'))
const CustomersPage = lazy(() => import('@/pages/CustomersPage'))
const EmployeesPage = lazy(() => import('@/pages/EmployeesPage'))
const CashPage = lazy(() => import('@/pages/CashPage'))
const ExpensesPage = lazy(() => import('@/pages/ExpensesPage'))
const ReportsPage = lazy(() => import('@/pages/ReportsPage'))
const CommissionsPage = lazy(() => import('@/pages/CommissionsPage'))
const SettingsPage = lazy(() => import('@/pages/SettingsPage'))
const SubscriptionPage = lazy(() => import('@/pages/SubscriptionPage'))
const AccountPage = lazy(() => import('@/pages/AccountPage'))
const OnboardingPage = lazy(() => import('@/pages/OnboardingPage'))
const WasherPortalPage = lazy(() => import('@/pages/WasherPortalPage'))
const AdminOverviewPage = lazy(() => import('@/pages/admin/AdminOverviewPage'))
const AdminOrganizationsPage = lazy(() => import('@/pages/admin/AdminOrganizationsPage'))
const AdminPlansPage = lazy(() => import('@/pages/admin/AdminPlansPage'))

/**
 * Porte d'entrée des pages internes.
 *
 * - aucune session : retour à la connexion ;
 * - compte de la plateforme : il n'a pas de station, on l'envoie vers
 *   l'espace d'administration (ses pages n'ont pas de tableau de bord) ;
 * - mot de passe provisoire non changé : on impose la page « Mon compte » ;
 * - permission manquante : retour au tableau de bord (le serveur refuse aussi).
 */
/**
 * Écrans autorisés tant que la mise en route n'est pas terminée : ceux qui
 * permettent de la terminer, plus le compte et l'abonnement. Les autres
 * (lavages, caisse, rapports…) échoueraient de toute façon : le serveur refuse
 * les écritures d'exploitation d'une station non activée (requireOnboarding).
 */
const ECRANS_DE_MISE_EN_ROUTE = ['/onboarding', '/parametres', '/prestations', '/tarifs', '/mon-compte', '/abonnement']

function RequireAuth({ permission }: { permission?: string }) {
  const { me, loading } = useAuth()
  const location = useLocation()

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner label="Ouverture de votre espace…" />
      </div>
    )
  }
  if (!me) {
    // Une session qui vient d'être refusée ramène à la connexion avec la
    // raison : « vous devez vous connecter » sans explication n'aide personne.
    const expiree = sessionVientDExpirer()
    return <Navigate to="/connexion" replace state={expiree ? { session: 'expiree' } : undefined} />
  }
  if (me.user.role === 'superadmin') {
    return <Navigate to="/administration" replace />
  }
  if (me.must_change_password && location.pathname !== '/mon-compte') {
    return <Navigate to="/mon-compte" replace />
  }
  const etatOnboarding = me.onboarding?.state
  const enConfiguration = etatOnboarding === 'pending' || etatOnboarding === 'in_progress'
  if (enConfiguration && !ECRANS_DE_MISE_EN_ROUTE.some((chemin) => location.pathname.startsWith(chemin))) {
    return <Navigate to="/onboarding" replace />
  }
  if (permission && !me.permissions.includes(permission)) {
    return <Navigate to={me.user.role === 'washer' ? '/espace-laveur' : '/tableau-de-bord'} replace />
  }
  return <Outlet />
}

function RequireSuperadmin() {
  const { me, loading } = useAuth()
  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner label="Vérification des droits…" />
      </div>
    )
  }
  if (!me || me.user.role !== 'superadmin') {
    return <Navigate to={me ? '/tableau-de-bord' : '/connexion'} replace />
  }
  return <Outlet />
}

/**
 * Racine du site.
 *
 * Un visiteur découvre WashGuard (page de présentation) ; une personne déjà
 * identifiée est conduite directement à son espace, sans repasser par la
 * vitrine.
 */
function HomePage() {
  const { me, loading } = useAuth()
  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner />
      </div>
    )
  }
  if (!me) return <LandingPage />
  if (me.user.role === 'superadmin') return <Navigate to="/administration" replace />
  if (me.user.role === 'washer') return <Navigate to="/espace-laveur" replace />
  if (me.onboarding?.state === 'pending' || me.onboarding?.state === 'in_progress') {
    return <Navigate to="/onboarding" replace />
  }
  return <Navigate to="/tableau-de-bord" replace />
}

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />

      <Route element={<AuthLayout />}>
        <Route path="/connexion" element={<LoginPageNew />} />
        <Route path="/inscription" element={<RegisterPageNew />} />
        {/* Le lien « Mot de passe oublié ? » de la connexion pointait vers une
            adresse qui n'existait pas : la page explique la procédure réelle. */}
        <Route path="/mot-de-passe-oublie" element={<ForgotPasswordPage />} />
        {/* Rejoindre une entreprise : public, car l'invité n'a pas encore de
            compte. Le lien reçu porte le jeton (`?jeton=…`), ou l'on saisit le
            code court dicté au téléphone. */}
        <Route path="/rejoindre" element={<JoinPage />} />
      </Route>

      <Route element={<RequireAuth />}>
        <Route element={<AppLayout />}>
          <Route element={<RequireAuth permission="settings:view" />}>
            <Route path="/onboarding" element={<OnboardingPage />} />
          </Route>
          <Route element={<RequireAuth permission="dashboard:view" />}>
            <Route path="/tableau-de-bord" element={<DashboardPage />} />
          </Route>
          <Route element={<RequireAuth permission="wash:view" />}>
            <Route path="/lavages" element={<WashOrdersPage />} />
          </Route>
          <Route element={<RequireAuth permission="service:view" />}>
            <Route path="/prestations" element={<ServicesPage />} />
            <Route path="/tarifs" element={<TarifsPage />} />
          </Route>
          <Route element={<RequireAuth permission="customer:view" />}>
            <Route path="/clients" element={<CustomersPage />} />
          </Route>
          <Route element={<RequireAuth permission="employee:view" />}>
            <Route path="/employes" element={<EmployeesPage />} />
          </Route>
          <Route element={<RequireAuth permission="cash:view" />}>
            <Route path="/caisse" element={<CashPage />} />
          </Route>
          <Route element={<RequireAuth permission="expense:view" />}>
            <Route path="/depenses" element={<ExpensesPage />} />
          </Route>
          <Route element={<RequireAuth permission="report:view" />}>
            <Route path="/rapports" element={<ReportsPage />} />
          </Route>
          {/* La paie des laveurs n'est pas un chiffre comme les autres : elle est
              réservée au propriétaire et au gérant (permission commission:view). */}
          <Route element={<RequireAuth permission="commission:view" />}>
            <Route path="/commissions" element={<CommissionsPage />} />
          </Route>
          <Route element={<RequireAuth permission="settings:view" />}>
            <Route path="/parametres" element={<SettingsPage />} />
          </Route>
          <Route element={<RequireAuth permission="subscription:view" />}>
            <Route path="/abonnement" element={<SubscriptionPage />} />
          </Route>
          <Route element={<RequireAuth permission="washer:view" />}>
            <Route path="/espace-laveur" element={<WasherPortalPage />} />
          </Route>
          <Route path="/mon-compte" element={<AccountPage />} />
        </Route>
      </Route>

      <Route element={<RequireSuperadmin />}>
        <Route element={<AdminLayout />}>
          <Route path="/administration" element={<AdminOverviewPage />} />
          <Route path="/administration/entreprises" element={<AdminOrganizationsPage />} />
          <Route path="/administration/formules" element={<AdminPlansPage />} />
        </Route>
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}
