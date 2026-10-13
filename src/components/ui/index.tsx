/**
 * Bibliothèque de composants UI WashGuard.
 * Styles pilotés par les tokens Tailwind du thème (bg-surface, text-ink…).
 */
import {
  forwardRef,
  useEffect,
  useId,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { Loader2, X } from 'lucide-react'

// ---------------------------------------------------------------------------
// Button
// ---------------------------------------------------------------------------

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'brand' | 'default'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: 'sm' | 'md'
  icon?: ReactNode
  loading?: boolean
}

const BUTTON_VARIANTS: Record<ButtonVariant, string> = {
  default: 'bg-brand text-white hover:bg-brand-strong focus-visible:outline-brand',
  brand: 'bg-brand text-white hover:bg-brand-strong focus-visible:outline-brand',
  primary: 'bg-brand text-white hover:bg-brand-strong focus-visible:outline-brand',
  secondary: 'border border-line bg-surface text-ink hover:bg-surface-muted focus-visible:outline-brand',
  ghost: 'text-ink-muted hover:bg-surface-muted hover:text-ink focus-visible:outline-brand',
  danger: 'bg-danger text-white hover:bg-danger-strong focus-visible:outline-danger',
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', icon, loading, className = '', children, disabled, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      className={`inline-flex items-center justify-center gap-2 rounded-lg font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${size === 'sm' ? 'px-2.5 py-1.5 text-xs' : 'px-4 py-2 text-sm'
        } ${BUTTON_VARIANTS[variant]} ${className}`}
      disabled={disabled || loading}
      {...rest}
    >
      {loading ? <Loader2 size={16} className="animate-spin" /> : icon}
      {children}
    </button>
  )
})

// ---------------------------------------------------------------------------
// Card + CardHeader
// ---------------------------------------------------------------------------

export function Card({ className = '', children }: { className?: string; children: ReactNode }) {
  return <div className={`rounded-xl border border-line bg-surface shadow-xs ${className}`}>{children}</div>
}

export function CardHeader({ title, subtitle, actions }: { title: string; subtitle?: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line px-4 py-3">
      <div>
        <p className="font-semibold text-ink">{title}</p>
        {subtitle ? <p className="text-xs text-ink-muted">{subtitle}</p> : null}
      </div>
      {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
    </div>
  )
}

/** Note de définition : éclaircissement d'un chiffre (méthode de calcul…). */
export function DefinitionNote({ children, definitions }: { children?: ReactNode; definitions?: Record<string, string> }) {
  if (definitions) {
    const entries = Object.entries(definitions)
    if (entries.length === 0) return null
    return (
      <div className="rounded-lg bg-surface-muted px-3 py-2 text-xs text-ink-muted">
        {entries.map(([term, definition]) => (
          <p key={term}>
            <span className="font-semibold text-ink">{term}</span> — {definition}
          </p>
        ))}
      </div>
    )
  }
  return <p className="rounded-lg bg-surface-muted px-3 py-2 text-xs text-ink-muted">{children}</p>
}

/** Montant aligné à droite, variante compacte utilisée dans les tableaux. */
export function Money({ value, className = '' }: { value: number; className?: string }) {
  return <span className={`tabular whitespace-nowrap ${className}`}>{formatXOFShort(value)}</span>
}

function formatXOFShort(value: number): string {
  return `${Number(value ?? 0).toLocaleString('fr-FR')} FCFA`
}

/** Barre de progression simple (objectif mensuel). */
export function Progress({
  value,
  percent,
  tone = 'brand',
  label,
  showValue,
}: {
  value?: number
  percent?: number
  tone?: 'brand' | 'success' | 'warning' | 'danger'
  label?: string
  showValue?: boolean
}) {
  const raw = value ?? percent ?? 0
  const width = Math.max(0, Math.min(100, raw))
  const color = tone === 'success' ? 'bg-success' : tone === 'warning' ? 'bg-warning' : tone === 'danger' ? 'bg-danger' : 'bg-brand'
  return (
    <div>
      <div className="h-2 w-full overflow-hidden rounded-full bg-surface-muted" role="progressbar" aria-valuenow={width} aria-valuemin={0} aria-valuemax={100}>
        <div className={`h-full rounded-full ${color}`} style={{ width: `${width}%` }} />
      </div>
      {label ? <p className="mt-1 text-xs text-ink-muted">{label}</p> : null}
      {showValue ? <p className="mt-0.5 text-right text-xs font-medium text-ink">{width}%</p> : null}
    </div>
  )
}

/** Repli pendant le chargement d'une page (React.lazy + Suspense). */
export function PageFallback() {
  return <Spinner label="Chargement de la page…" />
}

// ---------------------------------------------------------------------------
// Spinner / états
// ---------------------------------------------------------------------------

export function Spinner({ label }: { label?: string }) {
  return (
    <div className="flex items-center justify-center gap-2 py-10 text-sm text-ink-muted" role="status">
      <Loader2 size={18} className="animate-spin" />
      {label ? <span>{label}</span> : <span>Chargement…</span>}
    </div>
  )
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center gap-3 py-10 text-center">
      <p className="text-sm text-danger">{message}</p>
      {onRetry ? (
        <Button size="sm" variant="secondary" onClick={onRetry}>
          Réessayer
        </Button>
      ) : null}
    </div>
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
    <div className="flex flex-col items-center gap-2 py-12 text-center">
      {icon ? <div className="text-ink-muted">{icon}</div> : null}
      <p className="font-medium text-ink">{title}</p>
      {description ? <p className="max-w-md text-sm text-ink-muted">{description}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  )
}

/** Tableau de données simple : colonnes + lignes de cellules texte. */
export function Table({ columns, rows }: { columns: string[]; rows: Array<Array<ReactNode>> }) {
  return (
    <div className="overflow-x-auto">
      <table className="table">
        <thead>
          <tr>
            {columns.map((column, index) => (
              <th key={index} className={index > 0 ? 'text-right' : ''}>
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, rowIndex) => (
            <tr key={rowIndex}>
              {row.map((cell, cellIndex) => (
                <td key={cellIndex} className={cellIndex > 0 ? 'text-right' : ''}>
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ---------------------------------------------------------------------------
// StatCard
// ---------------------------------------------------------------------------

export function StatCard({
  label,
  value,
  hint,
  icon,
  tone,
}: {
  label: string
  value: string
  hint?: string
  icon?: ReactNode
  tone?: 'default' | 'brand' | 'success' | 'warning' | 'danger'
}) {
  const toneClass =
    tone === 'success'
      ? 'text-success'
      : tone === 'warning'
        ? 'text-warning'
        : tone === 'danger'
          ? 'text-danger'
          : tone === 'brand'
            ? 'text-brand'
            : 'text-ink'
  return (
    <div className="rounded-xl border border-line bg-surface p-4 shadow-xs">
      <div className="flex items-center justify-between gap-2">
        <p className="stat-label">{label}</p>
        {icon ? <span className="text-ink-muted">{icon}</span> : null}
      </div>
      <p className={`stat-value mt-1 ${toneClass}`}>{value}</p>
      {hint ? <p className="mt-0.5 text-xs text-ink-muted">{hint}</p> : null}
    </div>
  )
}

// ---------------------------------------------------------------------------
// PageHeader
// ---------------------------------------------------------------------------

export function PageHeader({
  title,
  subtitle,
  actions,
}: {
  title: string
  subtitle?: string
  actions?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 className="text-xl font-bold text-ink">{title}</h1>
        {subtitle ? <p className="mt-0.5 max-w-2xl text-sm text-ink-muted">{subtitle}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Modal
// ---------------------------------------------------------------------------

export function Modal({
  open,
  title,
  description,
  onClose,
  children,
  footer,
  size = 'md',
}: {
  open: boolean
  title: string
  description?: string
  onClose: () => void
  children: ReactNode
  footer?: ReactNode
  size?: 'sm' | 'md' | 'lg'
}) {
  useEffect(() => {
    if (!open) return
    const handler = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [open, onClose])

  if (!open) return null
  const width = size === 'sm' ? 'max-w-md' : size === 'lg' ? 'max-w-3xl' : 'max-w-xl'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-ink/40 backdrop-blur-sm" onClick={onClose} aria-hidden="true" />
      <div className={`relative w-full ${width} max-h-[90vh] overflow-y-auto rounded-xl border border-line bg-surface shadow-lg`}>
        <div className="flex items-start justify-between gap-3 border-b border-line p-4">
          <div>
            <h2 className="font-semibold text-ink">{title}</h2>
            {description ? <p className="mt-0.5 text-xs text-ink-muted">{description}</p> : null}
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded p-1 text-ink-muted hover:text-ink"
            aria-label="Fermer"
          >
            <X size={16} />
          </button>
        </div>
        <div className="p-4">{children}</div>
        {footer ? <div className="flex justify-end gap-2 border-t border-line p-4">{footer}</div> : null}
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// ConfirmDialog
// ---------------------------------------------------------------------------

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Confirmer',
  cancelLabel = 'Annuler',
  tone = 'primary',
  loading,
  onCancel,
  onConfirm,
  children,
}: {
  open: boolean
  title: string
  message: string
  confirmLabel?: string
  cancelLabel?: string
  tone?: 'primary' | 'danger'
  loading?: boolean
  onCancel: () => void
  onConfirm: () => void
  children?: ReactNode
}) {
  return (
    <Modal
      open={open}
      title={title}
      onClose={onCancel}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onCancel} disabled={loading}>
            {cancelLabel}
          </Button>
          <Button variant={tone} onClick={onConfirm} loading={loading}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <p className="text-sm text-ink-muted">{message}</p>
      {children}
    </Modal>
  )
}

// ---------------------------------------------------------------------------
// Field + Input + Textarea + Select
// ---------------------------------------------------------------------------

export function Field({
  label,
  children,
  required,
  hint,
  error,
}: {
  label: string
  children: (id: string) => ReactNode
  required?: boolean
  hint?: string
  error?: string
}) {
  const id = useId()
  return (
    <div>
      <label htmlFor={id} className="mb-1 block text-xs font-semibold text-ink">
        {label}
        {required ? <span className="text-danger"> *</span> : null}
      </label>
      {children(id)}
      {hint && !error ? <p className="mt-1 text-xs text-ink-muted">{hint}</p> : null}
      {error ? <p className="mt-1 text-xs text-danger">{error}</p> : null}
    </div>
  )
}

const CONTROL_CLASS = (invalid: boolean) =>
  `w-full rounded-lg border bg-surface px-3 py-2 text-sm text-ink placeholder:text-ink-muted focus:outline-2 focus:outline-offset-0 focus:outline-brand disabled:cursor-not-allowed disabled:opacity-60 ${invalid ? 'border-danger' : 'border-line'
  }`

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(
  function Input({ className = '', invalid, ...rest }, ref) {
    return <input ref={ref} className={`${CONTROL_CLASS(Boolean(invalid))} ${className}`} {...rest} />
  },
)

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }>(
  function Textarea({ className = '', invalid, ...rest }, ref) {
    return <textarea ref={ref} className={`${CONTROL_CLASS(Boolean(invalid))} min-h-20 ${className}`} {...rest} />
  },
)

export interface SelectOption {
  value: string
  label: string
}

export const Select = forwardRef<HTMLSelectElement, SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean; options: SelectOption[]; placeholder?: string }>(
  function Select({ className = '', invalid, options, placeholder, ...rest }, ref) {
    return (
      <select ref={ref} className={`${CONTROL_CLASS(Boolean(invalid))} ${className}`} {...rest}>
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    )
  },
)
