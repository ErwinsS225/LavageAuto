/**
 * Page d'accueil publique : présentation du produit et liens connexion.
 *
 * C'est la seule page où l'on prend le temps d'un décor : halos de marque en
 * fond, toile quadrillée en haut, aperçu du produit dans une carte en verre, et
 * apparitions au défilement. Les blocs « aperçu » sont des maquettes en CSS
 * (aucune donnée réelle, aucun graphique embarqué) : la page reste légère.
 */
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { ArrowRight, BarChart3, CheckCircle2, Droplets, ShieldCheck, Sparkles, Wallet } from 'lucide-react'

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
  { titre: 'Créez votre espace', texte: 'Inscription en deux minutes : entreprise, station et catalogue d’Abidjan déjà tarifé.' },
  { titre: 'Encaissez au guichet', texte: 'Fiche de lavage, encaissement espèces ou mobile money, caisse ouverte et suivie.' },
  { titre: 'Pilotez vos chiffres', texte: 'Rapports du jour, commissions des laveurs et objectif mensuel, sans tableur.' },
]

/** Barres de l'aperçu : hauteurs relatives, purement décoratives. */
const APERCU = [42, 58, 36, 72, 54, 88, 64]

export default function LandingPage() {
  const { me } = useAuth()
  usePageTitle('Accueil')

  return (
    <div className="app-aurora min-h-dvh bg-canvas">
      <header className="sticky top-0 z-30 border-b border-line/70 bg-surface/80 backdrop-blur-glass">
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

      <main className="mx-auto max-w-6xl px-4">
        {/* --- Héros ------------------------------------------------------- */}
        <section className="relative grid items-center gap-10 py-14 lg:grid-cols-2 lg:py-20">
          <div className="relative grid-lines absolute inset-x-0 -top-4 h-64" aria-hidden="true" />
          <motion.div
            variants={stagger(0.07, 0.05)}
            initial="hidden"
            animate="visible"
            className="relative"
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
              className="mt-4 text-3xl font-bold leading-[1.15] tracking-tight text-ink sm:text-[2.6rem]"
            >
              Pilotez votre station de lavage,{' '}
              <span className="bg-gradient-to-r from-brand to-brand-strong bg-clip-text text-transparent">
                sans tableur.
              </span>
            </motion.h1>
            <motion.p
              variants={{ hidden: { opacity: 0, y: 12 }, visible: { opacity: 1, y: 0 } }}
              className="mt-4 max-w-xl text-[0.95rem] leading-relaxed text-ink-muted"
            >
              Lavages, caisse, dépenses, commissions et rapports — une application pensée pour les
              stations d’Afrique de l’Ouest, en FCFA, utilisable depuis un téléphone au guichet.
            </motion.p>
            <motion.div
              variants={{ hidden: { opacity: 0, y: 12 }, visible: { opacity: 1, y: 0 } }}
              className="mt-7 flex flex-wrap items-center gap-3"
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
              className="mt-7 flex flex-wrap gap-x-5 gap-y-2 text-xs text-ink-muted"
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
            initial={{ opacity: 0, y: 20, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            transition={{ duration: 0.6, ease: EASE_SOFT, delay: 0.15 }}
            className="relative"
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
        </section>

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
                Trois étapes, une journée
              </p>
              <div className="mt-5 grid gap-6 sm:grid-cols-3">
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
