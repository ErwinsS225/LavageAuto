/**
 * États de l'interface : chargement, vide, erreur — et compteurs animés.
 *
 * Un écran de station de lavage est souvent consulté dans l'urgence : chaque
 * état dit ce qui se passe et, quand c'est possible, propose l'action qui
 * débloque (réessayer, enregistrer le premier lavage…).
 */
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { motion } from 'framer-motion'
import { Loader2, RotateCcw, TriangleAlert } from 'lucide-react'
import clsx from 'clsx'

import { Button } from '@/components/ui/primitives'
import { easeOutCubic, fadeUpSmall, DUR, EASE_SOFT, TRANSITION_SOFT } from '@/lib/motion'

/** Durée d'apparition par défaut des blocs de contenu. */
export const CONTENT_TRANSITION = { duration: DUR.base, ease: EASE_SOFT }

/** Repli pendant le chargement d'une page (React.lazy + Suspense). */
export function PageFallback() {
  return <Spinner label="Chargement de la page…" />
}

export function Spinner({ label, className }: { label?: string; className?: string }) {
  return (
    <div
      className={clsx('flex items-center justify-center gap-2.5 py-10 text-sm text-ink-muted', className)}
      role="status"
    >
      <Loader2 size={17} className="animate-spin text-brand" />
      <span>{label ?? 'Chargement…'}</span>
    </div>
  )
}

/** Bloc de chargement : imite la forme du contenu attendu (pas de saut visuel). */
export function Skeleton({ className }: { className?: string }) {
  return <div className={clsx('skeleton h-4 w-full', className)} />
}

export function SkeletonRows({ rows = 4 }: { rows?: number }) {
  return (
    <div className="space-y-2.5 p-4" aria-hidden="true">
      {Array.from({ length: rows }).map((_, index) => (
        <Skeleton key={index} className={index === 0 ? 'h-5 w-1/3' : 'h-4'} />
      ))}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <motion.div
      variants={fadeUpSmall}
      initial="hidden"
      animate="visible"
      className="flex flex-col items-center gap-3 py-10 text-center"
    >
      <span className="flex h-10 w-10 items-center justify-center rounded-full border border-danger/30 bg-danger-soft text-danger shadow-e1">
        <TriangleAlert size={18} />
      </span>
      <p className="max-w-md text-sm text-ink">{message}</p>
      {onRetry ? (
        <Button size="sm" variant="secondary" icon={<RotateCcw size={14} />} onClick={onRetry}>
          Réessayer
        </Button>
      ) : null}
    </motion.div>
  )
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode
  title: string
  description?: string
  action?: ReactNode
}) {
  return (
    <motion.div
      variants={fadeUpSmall}
      initial="hidden"
      animate="visible"
      transition={TRANSITION_SOFT}
      className="flex flex-col items-center gap-2 px-6 py-12 text-center"
    >
      {icon ? (
        <span className="mb-1 flex h-12 w-12 items-center justify-center rounded-card border border-line bg-surface-muted text-ink-muted shadow-e1">
          {icon}
        </span>
      ) : null}
      <p className="font-semibold text-ink">{title}</p>
      {description ? <p className="max-w-md text-sm text-ink-muted">{description}</p> : null}
      {action ? <div className="mt-3">{action}</div> : null}
    </motion.div>
  )
}

/**
 * Valeur qui compte jusqu'à sa cible (chiffres tabulaires, 700 ms).
 *
 * Le premier rendu affiche déjà la valeur finale : aucun clignotement, et les
 * contrôles de rendu hors navigateur voient la bonne donnée. L'animation ne
 * démarre que si le navigateur sait animer (`requestAnimationFrame`).
 */
export function AnimatedNumber({
  value,
  format = (n: number) => Math.round(n).toLocaleString('fr-FR'),
  duration = 700,
  className,
}: {
  value: number
  format?: (value: number) => string
  duration?: number
  className?: string
}) {
  const [affiche, setAffiche] = useState(value)
  const frame = useRef<number | null>(null)
  const depart = useRef(value)

  useEffect(() => {
    const cible = Number.isFinite(value) ? value : 0
    const origine = depart.current
    if (origine === cible) return
    if (typeof requestAnimationFrame !== 'function') {
      depart.current = cible
      setAffiche(cible)
      return
    }

    const debut = Date.now()
    const pas = () => {
      const progression = Math.min(1, (Date.now() - debut) / duration)
      setAffiche(origine + (cible - origine) * easeOutCubic(progression))
      if (progression < 1) {
        frame.current = requestAnimationFrame(pas)
      } else {
        depart.current = cible
      }
    }
    frame.current = requestAnimationFrame(pas)
    return () => {
      if (frame.current !== null && typeof cancelAnimationFrame === 'function') {
        cancelAnimationFrame(frame.current)
      }
      depart.current = cible
    }
  }, [value, duration])

  return <span className={clsx('tabular', className)}>{format(affiche)}</span>
}
