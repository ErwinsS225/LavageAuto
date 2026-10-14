/**
 * Marque WashGuard : la pastille de logo et le mot-symbole.
 *
 * Le même composant sert dans la barre latérale, sur les écrans publics et dans
 * l'espace plateforme : une seule définition évite les variantes qui divergent
 * (les écrans d'authentification portaient encore un autre nom et une autre
 * couleur que le reste du produit).
 */
import { Droplets, ShieldCheck } from 'lucide-react'
import clsx from 'clsx'

export function LogoMark({ size = 'md', tone = 'brand' }: { size?: 'sm' | 'md' | 'lg'; tone?: 'brand' | 'plateforme' }) {
  const dimensions =
    size === 'sm' ? 'h-7 w-7 rounded-[0.55rem]' : size === 'lg' ? 'h-11 w-11 rounded-card' : 'h-9 w-9 rounded-card'
  const icone = size === 'sm' ? 15 : size === 'lg' ? 22 : 18
  return (
    <span
      className={clsx(
        'relative flex shrink-0 items-center justify-center text-white shadow-e2',
        dimensions,
        tone === 'brand' ? 'btn-brand' : 'bg-surface-muted text-brand ring-1 ring-line',
      )}
    >
      {tone === 'brand' ? <Droplets size={icone} /> : <ShieldCheck size={icone} />}
    </span>
  )
}

export function Brand({
  size = 'md',
  subtitle,
  tone = 'brand',
  className,
}: {
  size?: 'sm' | 'md' | 'lg'
  subtitle?: string
  tone?: 'brand' | 'plateforme'
  className?: string
}) {
  return (
    <span className={clsx('flex items-center gap-2.5', className)}>
      <LogoMark size={size} tone={tone} />
      <span className="min-w-0">
        <span
          className={clsx(
            'block font-semibold tracking-tight text-ink',
            size === 'lg' ? 'text-lg' : size === 'sm' ? 'text-sm' : 'text-base',
          )}
        >
          WashGuard
        </span>
        {subtitle ? <span className="block truncate text-xs text-ink-muted">{subtitle}</span> : null}
      </span>
    </span>
  )
}
