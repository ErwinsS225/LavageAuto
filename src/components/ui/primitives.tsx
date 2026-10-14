/**
 * Primitives : boutons et pastilles.
 *
 * Le survol ne change pas seulement la couleur : le bouton se soulève d'un
 * pixel, son ombre s'approfondit, et l'appui le renfonce franchement (0,975).
 * C'est ce qui donne la sensation de « toucher » plutôt que de survoler.
 */
import { forwardRef, type ReactNode } from 'react'
import { motion, type HTMLMotionProps } from 'framer-motion'
import { Loader2 } from 'lucide-react'
import clsx from 'clsx'

import { EASE_SNAP, DUR } from '@/lib/motion'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'brand' | 'default' | 'soft'

/**
 * Les gestionnaires `onDrag*` et d'animation de React entrent en conflit avec
 * ceux de Framer Motion : on les retire du type, l'API publique des boutons ne
 * s'en sert pas.
 */
type MotionButtonProps = Omit<
  HTMLMotionProps<'button'>,
  | 'ref'
  | 'children'
  | 'onDrag'
  | 'onDragStart'
  | 'onDragEnd'
  | 'onAnimationStart'
  | 'onAnimationEnd'
  | 'onAnimationIteration'
>

interface ButtonProps extends MotionButtonProps {
  children?: ReactNode
  variant?: ButtonVariant
  size?: 'sm' | 'md' | 'lg'
  icon?: ReactNode
  loading?: boolean
  /** Occupe toute la largeur disponible (formulaires, pieds de modale). */
  block?: boolean
}

/** Classes de chaque variante : fond, contour, ombre, survol, focus. */
const VARIANTS: Record<ButtonVariant, string> = {
  // Dégradé de marque + reflet qui balaie au survol (voir .btn-sheen).
  primary: 'btn-brand btn-sheen',
  brand: 'btn-brand btn-sheen',
  default: 'btn-brand btn-sheen',
  secondary:
    'border border-line bg-surface text-ink shadow-e1 hover:border-line-strong hover:bg-surface-muted hover:shadow-e2',
  soft: 'bg-surface-muted text-ink hover:bg-surface-hover shadow-e1',
  ghost: 'text-ink-muted hover:bg-surface-muted hover:text-ink hover:shadow-e1',
  danger: 'bg-danger text-white shadow-e1 hover:bg-danger-strong hover:shadow-e2',
}

const SIZES = {
  sm: 'h-8 px-2.5 text-xs gap-1.5 rounded-[0.6rem]',
  md: 'h-10 px-3.5 text-sm gap-2 rounded-field',
  lg: 'h-11 px-5 text-sm gap-2 rounded-card',
} as const

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', icon, loading, className, children, disabled, block, ...rest },
  ref,
) {
  return (
    <motion.button
      ref={ref}
      whileHover={disabled || loading ? undefined : { y: -1 }}
      whileTap={disabled || loading ? undefined : { scale: 0.972 }}
      transition={{ duration: DUR.fast, ease: EASE_SNAP }}
      className={clsx(
        'relative inline-flex select-none items-center justify-center font-medium',
        'transition-[background-color,color,border-color,box-shadow,filter] duration-200 ease-soft',
        'disabled:cursor-not-allowed disabled:opacity-55 disabled:shadow-none disabled:filter-none',
        SIZES[size],
        VARIANTS[variant],
        block && 'w-full',
        className,
      )}
      disabled={disabled || loading}
      {...rest}
    >
      {loading ? <Loader2 size={size === 'sm' ? 13 : 15} className="animate-spin" /> : icon}
      {children}
    </motion.button>
  )
})

/** Bouton carré, pour les actions d'icône (fermer, rafraîchir, filtrer). */
export function IconButton({
  label,
  icon,
  className,
  variant = 'ghost',
  size = 'md',
  ...rest
}: MotionButtonProps & {
  label: string
  icon: ReactNode
  variant?: 'ghost' | 'secondary' | 'soft'
  size?: 'sm' | 'md'
}) {
  const variantes = {
    ghost: 'text-ink-muted hover:bg-surface-muted hover:text-ink',
    secondary: 'border border-line bg-surface text-ink-muted hover:text-ink hover:bg-surface-muted shadow-e1',
    soft: 'bg-surface-muted text-ink-muted hover:text-ink hover:bg-surface-hover',
  } as const
  return (
    <motion.button
      type="button"
      aria-label={label}
      title={label}
      whileTap={{ scale: 0.94 }}
      className={clsx(
        'inline-flex items-center justify-center rounded-field transition-colors duration-200 ease-soft',
        'disabled:cursor-not-allowed disabled:opacity-50',
        size === 'sm' ? 'h-7 w-7' : 'h-9 w-9',
        variantes[variant],
        className,
      )}
      {...rest}
    >
      {icon}
    </motion.button>
  )
}

/**
 * Sélecteur segmenté (filtres d'onglets, files d'attente).
 *
 * Le fond de l'onglet actif est un élément animé partagé (`layoutId`) : il
 * glisse d'un onglet à l'autre au lieu de clignoter. Deux sélecteurs sur la
 * même page doivent recevoir des `layoutId` différents.
 */
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  layoutId,
  className,
  size = 'sm',
  equal,
}: {
  value: T
  onChange: (value: T) => void
  options: Array<{ value: T; label: string; icon?: ReactNode; count?: number }>
  layoutId: string
  className?: string
  size?: 'sm' | 'md'
  /** Chaque onglet occupe la même largeur (barres d'onglets pleine largeur). */
  equal?: boolean
}) {
  return (
    <div role="tablist" className={clsx('panel-inset flex flex-wrap gap-0.5 p-0.5', className)}>
      {options.map((option) => {
        const actif = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="tab"
            aria-selected={actif}
            onClick={() => onChange(option.value)}
            className={clsx(
              'relative flex cursor-pointer items-center justify-center gap-2 rounded-[0.5rem] font-medium',
              'transition-colors duration-200 ease-soft',
              size === 'sm' ? 'px-2.5 py-1.5 text-xs' : 'px-3 py-2.5 text-xs font-semibold',
              equal && 'flex-1',
              actif ? 'text-ink' : 'text-ink-muted hover:text-ink',
            )}
          >
            {actif ? (
              <motion.span
                layoutId={layoutId}
                transition={{ type: 'spring', stiffness: 420, damping: 34 }}
                className="absolute inset-0 rounded-[0.5rem] border border-line bg-surface shadow-e1"
              />
            ) : null}
            <span className="relative flex items-center gap-1.5">
              {option.icon}
              {option.label}
              {typeof option.count === 'number' && option.count > 0 ? (
                <span className="rounded-full bg-warning-soft px-1.5 text-[0.625rem] font-bold text-warning ring-1 ring-warning/25">
                  {option.count}
                </span>
              ) : null}
            </span>
          </button>
        )
      })}
    </div>
  )
}

export type BadgeTone = 'neutral' | 'brand' | 'success' | 'warning' | 'danger' | 'info'

/** Pastille d'état : petite, arrondie, avec une ombre intérieure discrète. */
export function Badge({
  children,
  tone = 'neutral',
  className,
  dot,
}: {
  children: ReactNode
  tone?: BadgeTone
  className?: string
  dot?: boolean
}) {
  return (
    <span className={clsx('badge', `badge-${tone}`, className)}>
      {dot ? <span className="dot" aria-hidden="true" /> : null}
      {children}
    </span>
  )
}

/**
 * Associe une tonalité d'état (texte renvoyée par l'API, ex. « badge-success »)
 * aux classes du badge. Les helpers métier renvoient déjà la classe complète :
 * on la passe telle quelle plutôt que de traduire.
 */
export function toneClass(tone: string | undefined, fallback = 'badge-neutral'): string {
  if (!tone) return fallback
  return tone.startsWith('badge') ? tone : `badge-${tone}`
}
