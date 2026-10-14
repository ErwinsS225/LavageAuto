/**
 * Page d'accueil publique : vitrine du produit sur fond « BackgroundBeams ».
 *
 * Refonte : décor de faisceaux animés en pleine page (héro inclus), sections en
 * vitrine posées par-dessus, apparitions au défilement. Les blocs « aperçu »
 * restent des maquettes en CSS (aucune donnée réelle) : la page reste légère.
 */
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { ArrowRight, BarChart3, CheckCircle2, Droplets, ShieldCheck, Smartphone, Sparkles, Wallet } from 'lucide-react'

import { BackgroundBeams } from '@/components/BackgroundBeams'
import { Brand } from '@/components/Brand'
import { Reveal } from '@/components/ui'
import { Button } from '@/components/ui/primitives'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { DUR, EASE_SOFT, stagger } from '@/lib/motion'

const FEATURES = [
  {
    icon: <Droplets size={19} />,
    title: 'Fiches de lavage',
    text: 'Créez une fiche en quelques secondes, suivez son avancement et l’attribution aux laveurs.',
  },
  {
    icon: <Wallet size={19} />,
    title: 'Caisse fiable',
    text: 'Ouverture et clôture avec comptage, écarts signalés automatiquement au gérant.',
  },
  {
    icon: <BarChart3 size={19} />,
    title: 'Rapports clairs',
    text: 'Chiffre d’affaires, dépenses, top services et commissions — chaque jour.',
  },
  {
    icon: <ShieldCheck size={19} />,
    title: 'Rôles tracés',
    text: 'Caissier, gérant, laveur : chacun son écran, chaque action dans le journal d’audit.',
  },
]

const ETAPES = [
  // Ce qui est annoncé ici doit être ce que le produit fait : le catalogue
  // arrive **sans prix** (aucun tarif n'est inventé) et la station n'ouvre
  // qu'après la mise en route. Une promesse plus flatteuse serait un mensonge
  // que la première minute d'utilisation démentirait.
  { titre: 'Créez votre espace', texte: 'Inscription en deux minutes : votre entreprise, votre station et un catalogue de 18 prestations à tarifer à vos prix.' },
  { titre: 'Mettez en route', texte: 'Horaires, catégories de véhicules, grille tarifaire : treize étapes constatées par le serveur, puis l’activation de la station.' },
  { titre: 'Encaissez au guichet', texte: 'Fiche de lavage, encaissement espèces ou mobile money, caisse ouverte et suivie.' },
  { titre: 'Pilotez vos chiffres', texte: 'Rapports du jour, commissions des laveurs et objectif mensuel, sans tableur.' },
]

/** Barres de l'aperçu : hauteurs relatives, purement décoratives. */
const APERCU = [42, 58, 36, 72, 54, 88, 64]

export default function LandingPage() {
  const { me } = useAuth()
  usePageTitle('Accueil')

  return (
    <div className="min-h-dvh overflow-x-clip bg-canvas">
      <header className="sticky top-0 z-30 border-b border-line/70 bg-canvas/70 backdrop-blur-glass">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3">
          <Link to="/">
            <Brand />
          </Link>
          <div className="flex items-center gap-2">
            <span className="badge badge-success hidden sm:inline-flex">
              <span className="dot" aria-hidden="true" />
              Essai gratuit 14 jours
            </span>
            <Link to={me ? '/tableau-de-bord' : '/connexion'}>
              <Button variant="primary" size="sm" icon={<ArrowRight size={15} />}>
                {me ? 'Mon espace' : 'Se connecter'}
              </Button>
            </Link>
          </div>
        </div>
      </header>

      {/* --- Héros : plein écran sur le décor de faisceaux ---------------- */}
      <section className="relative flex min-h-[calc(100dvh-57px)] items-center overflow-hidden">
        <BackgroundBeams className="opacity-90" />
        {/* Voile sombre pour garder le texte lisible sur les faisceaux. */}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 bg-[radial-gradient(70%_60%_at_50%_40%,transparent_0%,var(--app-bg)_85%)]"
        />

        <div className="relative mx-auto w-full max-w-6xl px-4 py-20">
          <motion.div
            variants={stagger(0.07, 0.05)}
            initial="hidden"
            animate="visible"
            className="mx-auto max-w-2xl text-center"
          >
            <motion.span
              variants={{ hidden: { opacity: 0, y: 8 }, visible: { opacity: 1, y: 0 } }}
              className="badge badge-brand"
            >
              <Sparkles size={12} />
              Pensé pour les stations d’Afrique de l’Ouest
            </motion.span>
            <motion.h1
              variants={{ hidden: { opacity: 0, y: 12 }, visible: { opacity: 1, y: 0, transition: { duration: 0.5, ease: EASE_SOFT } } }}
              className="mt-5 text-4xl font-bold leading-[1.12] tracking-tight text-ink sm:text-5xl lg:text-6xl"
            >
              Pilotez votre station de lavage,{' '}
              <span className="bg-gradient-to-r from-brand via-info to-success bg-clip-text text-transparent">
                sans tableur.
              </span>
            </motion.h1>
            <motion.p
              variants={{ hidden: { opacity: 0, y: 12 }, visible: { opacity: 1, y: 0 } }}
              className="mx-auto mt-5 max-w-xl text-base leading-relaxed text-ink-muted sm:text-lg"
            >
              Lavages, caisse, dépenses, commissions et rapports — une application pensée pour les
              stations d’Afrique de l’Ouest, en FCFA, utilisable depuis un téléphone au guichet.
            </motion.p>
            <motion.div
              variants={{ hidden: { opacity: 0, y: 12 }, visible: { opacity: 1, y: 0 } }}
              className="mt-8 flex flex-wrap items-center justify-center gap-3"
            >
              <Link to="/inscription">
                <Button variant="primary" size="lg" icon={<Sparkles size={16} />}>
                  Commencer maintenant
                </Button>
              </Link>
              <Link to="/connexion">
                <Button variant="secondary" size="lg">
                  J’ai déjà un compte
                </Button>
              </Link>
            </motion.div>
            <motion.ul
              variants={{ hidden: { opacity: 0 }, visible: { opacity: 1 } }}
              className="mt-8 flex flex-wrap justify-center gap-x-5 gap-y-2 text-xs text-ink-muted"
            >
              {['Montants en FCFA', 'Rôles séparés par station', 'Journal d’audit'].map((item) => (
                <li key={item} className="flex items-center gap-1.5">
                  <CheckCircle2 size={14} className="text-success" />
                  {item}
                </li>
              ))}
            </motion.ul>
          </motion.div>

          {/* Aperçu de l'application : maquette en CSS, sans donnée réelle. */}
          <motion.div
            initial={{ opacity: 0, y: 24, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={{ duration: 0.6, ease: EASE_SOFT, delay: 0.35 }}
            className="relative mx-auto mt-14 max-w-xl"
          >
            <div className="card hairline-highlight overflow-hidden p-4 shadow-float">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="stat-label">Recette du jour</p>
                  <p className="stat-value mt-1 text-brand">184 500 FCFA</p>
                </div>
                <span className="badge badge-success">+12 % vs hier</span>
              </div>
              <div className="mt-4 flex h-32 items-end gap-2">
                {APERCU.map((hauteur, index) => (
                  <motion.span
                    key={index}
                    initial={{ height: 0, opacity: 0.4 }}
                    animate={{ height: `${hauteur}%`, opacity: 1 }}
                    transition={{ duration: DUR.chart, ease: EASE_SOFT, delay: 0.3 + index * 0.06 }}
                    className="flex-1 rounded-t-md bg-gradient-to-t from-brand/35 to-brand shadow-e1"
                  />
                ))}
              </div>
              <div className="mt-4 grid grid-cols-3 gap-2.5">
                {[
                  { label: 'Lavages', valeur: '37' },
                  { label: 'Caisse', valeur: 'Ouverte' },
                  { label: 'Reste dû', valeur: '6 000' },
                ].map((bloc) => (
                  <div key={bloc.label} className="panel-inset px-2.5 py-2">
                    <p className="text-[0.6875rem] text-ink-muted">{bloc.label}</p>
                    <p className="tabular mt-0.5 text-sm font-semibold text-ink">{bloc.valeur}</p>
                  </div>
                ))}
              </div>
            </div>
            <div
              aria-hidden="true"
              className="animate-float-slow absolute -right-4 -top-4 hidden rounded-card border border-line bg-surface px-3 py-2 shadow-e3 sm:block"
            >
              <p className="text-[0.6875rem] text-ink-muted">Commission laveur</p>
              <p className="tabular text-sm font-semibold text-success">500 FCFA</p>
            </div>
          </motion.div>
        </div>
      </section>

      {/* --- Contenu : sections posées sur fond clair ----------------------- */}
      <main className="relative mx-auto max-w-6xl px-4">
        {/* --- Fonctions --------------------------------------------------- */}
        <section className="py-6">
          <Reveal>
            <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ink-subtle">
              Ce que fait WashGuard
            </p>
          </Reveal>
          <motion.div
            variants={stagger(0.08, 0.05)}
            initial="hidden"
            whileInView="visible"
            viewport={{ once: true, margin: '-80px' }}
            className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-4"
          >
            {FEATURES.map((feature) => (
              <motion.div
                key={feature.title}
                variants={{ hidden: { opacity: 0, y: 14 }, visible: { opacity: 1, y: 0, transition: { duration: 0.45, ease: EASE_SOFT } } }}
                whileHover={{ y: -3 }}
                className="card card-interactive p-5"
              >
                <span className="flex h-10 w-10 items-center justify-center rounded-card border border-brand/25 bg-brand-soft text-brand shadow-e1">
                  {feature.icon}
                </span>
                <p className="mt-3 font-semibold text-ink">{feature.title}</p>
                <p className="mt-1 text-sm leading-relaxed text-ink-muted">{feature.text}</p>
              </motion.div>
            ))}
          </motion.div>
        </section>

        {/* --- Étapes ------------------------------------------------------ */}
        <section className="py-10">
          <Reveal>
            <div className="card overflow-hidden p-6 sm:p-8">
              <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ink-subtle">
                Quatre étapes, une journée
              </p>
              <div className="mt-5 grid gap-6 sm:grid-cols-2 lg:grid-cols-4">
                {ETAPES.map((etape, index) => (
                  <motion.div
                    key={etape.titre}
                    initial={{ opacity: 0, y: 12 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true, margin: '-60px' }}
                    transition={{ duration: 0.45, ease: EASE_SOFT, delay: index * 0.1 }}
                  >
                    <span className="flex h-8 w-8 items-center justify-center rounded-full border border-line bg-surface-muted text-sm font-semibold text-brand shadow-e1">
                      {index + 1}
                    </span>
                    <p className="mt-3 font-semibold text-ink">{etape.titre}</p>
                    <p className="mt-1 text-sm leading-relaxed text-ink-muted">{etape.texte}</p>
                  </motion.div>
                ))}
              </div>
            </div>
          </Reveal>
        </section>

        {/* --- Appel à l'action -------------------------------------------- */}
        <Reveal>
          <section className="relative overflow-hidden rounded-panel border border-line bg-surface p-8 text-center shadow-e3">
            <BackgroundBeams className="opacity-70" />
            <div
              aria-hidden="true"
              className="pointer-events-none absolute inset-0 bg-gradient-to-br from-brand/10 via-transparent to-success/10"
            />
            <div className="relative">
              <p className="text-xl font-bold tracking-tight text-ink sm:text-2xl">
                Ouvrez votre espace aujourd’hui
              </p>
              <p className="mx-auto mt-2 max-w-lg text-sm text-ink-muted">
                Essai gratuit de 14 jours, sans carte bancaire. Vos données restent les vôtres, et
                vous pouvez exporter vos rapports à tout moment.
              </p>
              <div className="mt-6 flex flex-wrap items-center justify-center gap-3">
                <Link to="/inscription">
                  <Button variant="primary" size="lg" icon={<ArrowRight size={16} />}>
                    Créer mon espace
                  </Button>
                </Link>
                <Link to="/connexion">
                  <Button variant="ghost" size="lg">
                    Se connecter
                  </Button>
                </Link>
              </div>
            </div>
          </section>
        </Reveal>
      </main>

      <footer className="mt-12 border-t border-line py-6">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-3 px-4 text-xs text-ink-muted">
          <Brand size="sm" />
          <p>WashGuard — la gestion simple des stations de lavage. Abidjan, Côte d’Ivoire.</p>
        </div>
      </footer>
    </div>
  )
}
