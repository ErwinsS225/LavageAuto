/**
 * Page 404 : route inconnue.
 */
import { Link } from 'react-router-dom'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'

export default function NotFoundPage() {
  usePageTitle('Page introuvable')
  const { me } = useAuth()
  // La page de secours doit renvoyer vers une adresse qui existe : l'espace de
  // la plateforme est /administration (et non /admin).
  const home = me
    ? me.user.role === 'washer'
      ? '/espace-laveur'
      : me.user.role === 'superadmin'
        ? '/administration'
        : '/tableau-de-bord'
    : '/'

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-3 bg-surface-muted px-4 text-center">
      <p className="text-5xl font-bold text-brand">404</p>
      <p className="font-medium text-ink">Cette page n'existe pas.</p>
      <p className="max-w-sm text-sm text-ink-muted">
        Le lien est peut-être obsolète ou l'adresse mal saisie.
      </p>
      <Link to={home} className="mt-2 rounded-lg bg-brand px-4 py-2 text-sm font-medium text-white hover:bg-brand-strong">
        Retour à l'accueil
      </Link>
    </div>
  )
}
