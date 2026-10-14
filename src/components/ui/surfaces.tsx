/**
 * Surfaces : cartes, en-têtes, indicateurs, notes et barres de progression.
 *
 * Toutes les surfaces héritent d'un même langage : un contour d'un pixel, une
 * ombre de contact, un reflet intérieur en haut, et une transition très douce
 * quand elles apparaissent. Placées dans conteneur `MotionStagger`, elles se
 * posent l'une après l'autre au lieu d'apparaître en bloc.
 */
import type { ReactNode } from 'react'
import { motion } from 'framer-motion'
import clsx from 'clsx'

import { fadeUpSmall, DUR, EASE_SOFT, TRANSITION_SOFT } from '@/lib/motion'

// ---------------------------------------------------------------------------
// Card
// ---------------------------------------------------------------------------

export function Card({
  className,
  children,
  interactive,
  glass,
  delay = 0,
}: {
  className?: string
  children: ReactNode
  /** Se soulève au survol (à réserver aux cartes cliquables). */
  interactive?: boolean
  /** Verre dépoli, pour les blocs posés sur un fond coloré. */
  glass?: boolean
  /** Décalage d'apparition, quand les cartes ne sont pas dans un conteneur. */
  delay?: number
}) {
  return (
    <motion.div
      variants={fadeUpSmall}
      initial="hidden"
      animate="visible"
      transition={{ ...TRANSITION_SOFT, delay }}
      className={clsx(glass ? 'card-glass' : 'card', interactive && 'card-interactive', className)}
    >
      {children}
    </motion.div>
  )
}

/** En-tête de carte : titre, sous-titre, icône facultative, actions à droite. */
export function CardHeader({
  title,
  subtitle,
  actions,
  icon,
  className,
}: {
  title: string
  subtitle?: string
  actions?: ReactNode
  icon?: ReactNode
  className?: string
}) {
  return (
    <div
      className={clsx(
        'flex flex-wrap items-center justify-between gap-3 border-b border-line px-4 py-3.5',
        className,
      )}
    >
      <div className="flex min-w-0 items-start gap-3">
        {icon ? (
          <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-field border border-line bg-surface-muted text-brand shadow-e1">
            {icon}
          </span>
        ) : null}
        <div className="min-w-0">
          <p className="truncate font-semibold text-ink">{title}</p>
          {subtitle ? <p className="mt-0.5 text-xs text-ink-muted">{subtitle}</p> : null}
        </div>
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  )
}

/**
 * Note de définition : éclaircit un chiffre (méthode de calcul, périmètre).
 * Fond creusé et texte discret — l'information est là sans crier.
 */
export function DefinitionNote({
  children,
  definitions,
}: {
  children?: ReactNode
  definitions?: Record<string, string>
}) {
  if (definitions) {
    const entries = Object.entries(definitions)
    if (entries.length === 0) return null
    return (
      <div className="panel-inset px-3 py-2.5 text-xs text-ink-muted">
        {entries.map(([term, definition]) => (
          <p key={term} className="leading-relaxed">
            <span className="font-semibold text-ink">{term}</span> — {definition}
          </p>
        ))}
      </div>
    )
  }
  return <p className="panel-inset px-3 py-2.5 text-xs leading-relaxed text-ink-muted">{children}</p>
}

/** Montant aligné, chiffres tabulaires : les colonnes restent droites. */
export function Money({ value, className = '' }: { value: number; className?: string }) {
  return <span className={clsx('tabular whitespace-nowrap', className)}>{formatXOFShort(value)}</span>
}

function formatXOFShort(value: number): string {
  return `${Number(value ?? 0).toLocaleString('fr-FR')} FCFA`
}

// ---------------------------------------------------------------------------
// Indicateurs
// ---------------------------------------------------------------------------

const TONES = {
  default: 'text-ink',
  brand: 'text-brand',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
} as const

const ICON_TONES = {
  default: 'border-line bg-surface-muted text-ink-muted',
  brand: 'border-brand/30 bg-brand-soft text-brand',
  success: 'border-success/30 bg-success-soft text-success',
  warning: 'border-warning/30 bg-warning-soft text-warning',
  danger: 'border-danger/30 bg-danger-soft text-danger',
} as const

export type StatTone = keyof typeof TONES

/**
 * Carte d'indicateur : libellé, valeur, précision, icône dans une pastille.
 * `progress` ajoute une jauge (objectif mensuel, taux d'encaissement…).
 */
export function StatCard({
  label,
  value,
  hint,
  icon,
  tone = 'default',
  progress,
  delay = 0,
  className,
}: {
  label: string
  value: ReactNode
  hint?: string
  icon?: ReactNode
  tone?: StatTone
  progress?: number
  delay?: number
  className?: string
}) {
  return (
    <motion.div
      variants={fadeUpSmall}
      initial="hidden"
      animate="visible"
      transition={{ ...TRANSITION_SOFT, delay }}
      whileHover={{ y: -2 }}
      className={clsx('card card-interactive hairline-highlight relative p-4', className)}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="stat-label">{label}</p>
          <p className={clsx('stat-value mt-1.5', TONES[tone])}>{value}</p>
          {hint ? <p className="mt-1 text-xs text-ink-muted">{hint}</p> : null}
        </div>
        {icon ? (
          <span
            className={clsx(
              'flex h-9 w-9 shrink-0 items-center justify-center rounded-field border shadow-e1',
              ICON_TONES[tone],
            )}
          >
            {icon}
          </span>
        ) : null}
      </div>
      {typeof progress === 'number' ? (
        <div className="mt-3">
          <Progress value={progress} />
        </div>
      ) : null}
    </motion.div>
  )
}

/**
 * Barre de progression : la largeur s'anime, la couleur suit la tonalité.
 * `value` et `percent` sont acceptés tous les deux (compatibilité).
 */
export function Progress({
  value,
  percent,
  tone = 'brand',
  label,
  showValue,
  className,
}: {
  value?: number
  percent?: number
  tone?: 'brand' | 'success' | 'warning' | 'danger'
  label?: string
  showValue?: boolean
  className?: string
}) {
  const raw = value ?? percent ?? 0
  const width = Math.max(0, Math.min(100, raw))
  const fill =
    tone === 'success'
      ? 'bg-success'
      : tone === 'warning'
        ? 'bg-warning'
        : tone === 'danger'
          ? 'bg-danger'
          : 'bg-brand'
  return (
    <div className={className}>
      <div
        className="h-1.5 w-full overflow-hidden rounded-full bg-surface-inset shadow-[inset_0_1px_2px_rgba(15,23,36,.08)]"
        role="progressbar"
        aria-valuenow={width}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <motion.div
          className={clsx('h-full rounded-full shadow-e1', fill)}
          initial={{ width: 0 }}
          animate={{ width: `${width}%` }}
          transition={{ duration: DUR.chart, ease: EASE_SOFT }}
        />
      </div>
      {label ? <p className="mt-1.5 text-xs text-ink-muted">{label}</p> : null}
      {showValue ? <p className="mt-0.5 text-right text-xs font-medium text-ink">{Math.round(width)}%</p> : null}
    </div>
  )
}

// ---------------------------------------------------------------------------
// En-tête de page
// ---------------------------------------------------------------------------

/** Titre d'écran + actions. Le sous-titre explique le chiffre, pas l'écran. */
export function PageHeader({
  title,
  subtitle,
  actions,
  meta,
}: {
  title: string
  subtitle?: string
  actions?: ReactNode
  meta?: ReactNode
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: -6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={TRANSITION_SOFT}
      className="flex flex-wrap items-start justify-between gap-3"
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="text-[1.375rem] font-bold leading-tight tracking-tight text-ink">{title}</h1>
          {meta}
        </div>
        {subtitle ? <p className="mt-1 max-w-2xl text-sm text-ink-muted">{subtitle}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </motion.div>
  )
}

/** Séparateur de section : un filet qui s'estompe aux extrémités. */
export function SectionTitle({ children, actions }: { children: ReactNode; actions?: ReactNode }) {
  return (
    <div className="flex items-center gap-3">
      <p className="text-xs font-semibold uppercase tracking-[0.08em] text-ink-subtle">{children}</p>
      <div className="divider-soft h-px flex-1" />
      {actions}
    </div>
  )
}

// ---------------------------------------------------------------------------
// En-tête d'écran « héros »
// ---------------------------------------------------------------------------

/**
 * Bandeau d'accueil d'un écran : icône, titre, rôle, contexte du jour et
 * actions. C'est la première chose que l'utilisateur voit en arrivant au
 * guichet : il doit dire où l'on est, pour qui, et à quelle date — sans avoir
 * à lire le reste.
 */
export function ScreenHero({
  icon,
  title,
  badge,
  subtitle,
  actions,
  tone = 'brand',
}: {
  icon: ReactNode
  title: string
  badge?: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
  tone?: 'brand' | 'neutral'
}) {
  return (
    <motion.div
      variants={fadeUpSmall}
      initial="hidden"
      animate="visible"
      className={clsx(
        'card hairline-highlight relative overflow-hidden p-4 sm:p-5',
        tone === 'brand'
          ? 'bg-gradient-to-r from-brand-soft via-surface to-surface'
          : 'bg-gradient-to-r from-surface-muted via-surface to-surface',
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3.5">
          <span
            className={clsx(
              'flex h-12 w-12 shrink-0 items-center justify-center rounded-card shadow-e2',
              tone === 'brand' ? 'btn-brand' : 'border border-line bg-surface text-brand',
            )}
          >
            {icon}
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-[1.3rem] font-bold leading-tight tracking-tight text-ink">{title}</h1>
              {badge}
            </div>
            {subtitle ? <p className="mt-1 text-sm text-ink-muted">{subtitle}</p> : null}
          </div>
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </div>
    </motion.div>
  )
}
