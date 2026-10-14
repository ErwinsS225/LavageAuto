/**
 * Formulaires : libellé, champ, aide, erreur.
 *
 * Les champs partagent la même mécanique que les cartes : un contour, une ombre
 * intérieure très légère (le champ paraît creusé), et au focus un anneau diffus
 * plutôt qu'un trait dur.
 */
import {
  forwardRef,
  useId,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { motion } from 'framer-motion'
import clsx from 'clsx'

import { DUR, EASE_SNAP } from '@/lib/motion'

export function Field({
  label,
  children,
  required,
  hint,
  error,
  className,
}: {
  label: string
  children: (id: string) => ReactNode
  required?: boolean
  hint?: string
  error?: string
  className?: string
}) {
  const id = useId()
  return (
    <div className={className}>
      <label htmlFor={id} className="mb-1.5 block text-xs font-medium text-ink">
        {label}
        {required ? <span className="text-danger"> *</span> : null}
      </label>
      {children(id)}
      {hint && !error ? <p className="mt-1.5 text-xs text-ink-muted">{hint}</p> : null}
      {error ? (
        <motion.p
          initial={{ opacity: 0, y: -3 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: DUR.fast, ease: EASE_SNAP }}
          className="mt-1.5 text-xs text-danger"
        >
          {error}
        </motion.p>
      ) : null}
    </div>
  )
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }>(
  function Input({ className = '', invalid, ...rest }, ref) {
    return <input ref={ref} className={clsx('input', invalid && 'border-danger', className)} {...rest} />
  },
)

export const Textarea = forwardRef<
  HTMLTextAreaElement,
  TextareaHTMLAttributes<HTMLTextAreaElement> & { invalid?: boolean }
>(function Textarea({ className = '', invalid, ...rest }, ref) {
  return <textarea ref={ref} className={clsx('textarea min-h-20', invalid && 'border-danger', className)} {...rest} />
})

export interface SelectOption {
  value: string
  label: string
}

export const Select = forwardRef<
  HTMLSelectElement,
  SelectHTMLAttributes<HTMLSelectElement> & { invalid?: boolean; options: SelectOption[]; placeholder?: string }
>(function Select({ className = '', invalid, options, placeholder, ...rest }, ref) {
  return (
    <select ref={ref} className={clsx('select', invalid && 'border-danger', className)} {...rest}>
      {placeholder ? <option value="">{placeholder}</option> : null}
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  )
})

/**
 * Interrupteur : piste creusée, curseur qui glisse avec un ressort court.
 * Rendu accessible (role switch) et utilisable au clavier.
 */
export function Switch({
  checked,
  onChange,
  label,
  hint,
  disabled,
}: {
  checked: boolean
  onChange: (value: boolean) => void
  label: string
  hint?: string
  disabled?: boolean
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="min-w-0">
        <p className="text-sm font-medium text-ink">{label}</p>
        {hint ? <p className="mt-0.5 text-xs text-ink-muted">{hint}</p> : null}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={clsx(
          'relative h-6 w-11 shrink-0 rounded-full border transition-colors duration-200 ease-soft',
          'disabled:cursor-not-allowed disabled:opacity-50',
          checked
            ? 'border-brand/40 bg-brand/85 shadow-[inset_0_1px_2px_rgba(0,0,0,.25)]'
            : 'border-line bg-surface-inset shadow-[inset_0_1px_2px_rgba(15,23,36,.08)]',
        )}
      >
        <motion.span
          initial={false}
          animate={{ x: checked ? 18 : 0 }}
          transition={{ type: 'spring', stiffness: 480, damping: 34 }}
          className="absolute left-0.5 top-0.5 h-5 w-5 rounded-full bg-white shadow-e2"
        />
      </button>
    </div>
  )
}
