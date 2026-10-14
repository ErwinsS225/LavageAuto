/**
 * Mot de passe oublié : explication de la procédure, sans remise à zéro
 * automatique.
 *
 * Le serveur WashGuard n'envoie pas d'e-mail et n'expose pas de remise à zéro
 * publique (elle se fait par le gérant, depuis Employés → Réinitialiser le
 * mot de passe, ou par le support). Cette page existe pour que le lien
 * « Mot de passe oublié ? » de la connexion mène à une explication utile
 * plutôt qu'à une page introuvable.
 */
import { Link } from 'react-router-dom'

import AuthHeader from '@/components/auth/AuthHeader'
import { usePageTitle } from '@/hooks/usePageTitle'

export default function ForgotPasswordPage() {
  usePageTitle('Mot de passe oublié')

  return (
    <>
      <AuthHeader />
      <div className="mx-auto flex w-full max-w-sm flex-col justify-center md:mx-0">
        <h1 className="text-2xl font-semibold tracking-tight text-ink md:text-3xl">
          Mot de passe oublié
        </h1>
        <p className="mt-1.5 text-sm leading-relaxed text-ink-muted">
          La remise à zéro se fait par une personne de votre station, pour garder la
          trace de l'opération.
        </p>

        <div className="mt-6 space-y-3 text-sm leading-relaxed text-ink-muted">
          <p>
            <span className="font-medium text-ink">Employé de la station :</span> demandez au
            propriétaire ou au gérant de réinitialiser votre mot de passe depuis l'écran
            Employés. Vous recevrez un mot de passe provisoire à changer à la première
            connexion.
          </p>
          <p>
            <span className="font-medium text-ink">Propriétaire ou gérant :</span> ouvrez
            l'écran Employés, choisissez la personne concernée puis « Réinitialiser le mot de
            passe ». Vous pouvez aussi changer votre propre mot de passe depuis Mon compte.
          </p>
          <p>
            <span className="font-medium text-ink">Compte de plateforme :</span> contactez le
            support WashGuard.
          </p>
        </div>

        <Link
          to="/connexion"
          className="link-soft mt-6 text-sm font-medium"
        >
          Revenir à la connexion
        </Link>
      </div>
    </>
  )
}
