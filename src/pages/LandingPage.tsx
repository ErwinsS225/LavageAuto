/**
 * Page d'accueil publique : présentation du produit et liens connexion.
 */
import { Link } from 'react-router-dom'
import { BarChart3, Droplets, ShieldCheck, Wallet } from 'lucide-react'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'

const FEATURES = [
  {
    icon: <Droplets size={20} />,
    title: 'Fiches de lavage',
    text: 'Créez une fiche en quelques secondes, suivez son avancement et l’attribution aux laveurs.',
  },
  {
    icon: <Wallet size={20} />,
    title: 'Caisse fiable',
    text: 'Ouverture et clôture avec comptage, écarts signalés automatiquement au gérant.',
  },
  {
    icon: <BarChart3 size={20} />,
    title: 'Rapports clairs',
    text: 'Chiffre d’affaires, dépenses, top services et commissions — chaque jour.',
  },
  {
    icon: <ShieldCheck size={20} />,
    title: 'Rôles tracés',
    text: 'Caissier, gérant, laveur : chacun son écran, chaque action dans le journal d’audit.',
  },
]

export default function LandingPage() {
  const { me } = useAuth()
  usePageTitle('Accueil')

  return (
    <div className="min-h-dvh bg-surface-muted">
      <header className="mx-auto flex max-w-5xl items-center justify-between px-4 py-4">
        <span className="flex items-center gap-2 font-bold text-brand">
          <Droplets size={22} /> WashGuard
        </span>
        <div className="flex gap-2">
          {me ? (
            <Link to="/tableau-de-bord" className="rounded-lg bg-brand px-4 py-2 text-sm font-medium text-white hover:bg-brand-strong">
              Mon espace
            </Link>
          ) : (
            <Link to="/connexion" className="rounded-lg bg-brand px-4 py-2 text-sm font-medium text-white hover:bg-brand-strong">
              Se connecter
            </Link>
          )}
        </div>
      </header>

      <main className="mx-auto max-w-5xl px-4 pb-16">
        <section className="py-12 text-center sm:py-20">
          <h1 className="text-3xl font-bold text-ink sm:text-4xl">
            Pilotez votre station de lavage, sans tableur.
          </h1>
          <p className="mx-auto mt-4 max-w-xl text-ink-muted">
            Lavages, caisse, dépenses, commissions et rapports — une application pensée pour les
            stations d’Afrique de l’Ouest, en FCFA.
          </p>
          <Link
            to="/connexion"
            className="mt-8 inline-block rounded-lg bg-brand px-6 py-3 font-medium text-white hover:bg-brand-strong"
          >
            Commencer maintenant
          </Link>
        </section>

        <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {FEATURES.map((feature) => (
            <div key={feature.title} className="rounded-xl border border-line bg-surface p-5 shadow-xs">
              <div className="text-brand">{feature.icon}</div>
              <p className="mt-2 font-semibold text-ink">{feature.title}</p>
              <p className="mt-1 text-sm text-ink-muted">{feature.text}</p>
            </div>
          ))}
        </section>
      </main>

      <footer className="border-t border-line py-6 text-center text-xs text-ink-muted">
        WashGuard — la gestion simple des stations de lavage.
      </footer>
    </div>
  )
}
