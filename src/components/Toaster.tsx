/**
 * Toaster : notifications éphémères (succès, erreur, info, alerte).
 *
 * Contexte React simple — pas de dépendance externe — mais présentation
 * soignée : verre dépoli, icône dans une pastille teintée, barre de vie qui se
 * vide, entrée par un ressort court, sortie en 140 ms. Les notifications
 * s'empilent sans jamais dépasser le bord de l'écran.
 */
import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { CheckCircle2, Info, TriangleAlert, X, XCircle } from 'lucide-react'
import clsx from 'clsx'

import { toastVariants, DUR, EASE_SOFT } from '@/lib/motion'

export type ToastTone = 'success' | 'error' | 'info' | 'warning'

export interface Toast {
  id: number
  tone: ToastTone
  message: string
}

interface ToastContextValue {
  success: (message: string) => void
  error: (message: string) => void
  info: (message: string) => void
  warning: (message: string) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const TONES: Record<ToastTone, { icon: ReactNode; ring: string; bar: string }> = {
  success: { icon: <CheckCircle2 size={16} />, ring: 'border-success/30 bg-success-soft text-success', bar: 'bg-success' },
  error: { icon: <XCircle size={16} />, ring: 'border-danger/30 bg-danger-soft text-danger', bar: 'bg-danger' },
  info: { icon: <Info size={16} />, ring: 'border-brand/30 bg-brand-soft text-brand', bar: 'bg-brand' },
  warning: { icon: <TriangleAlert size={16} />, ring: 'border-warning/30 bg-warning-soft text-warning', bar: 'bg-warning' },
}

/** Durée de vie d'une notification, en millisecondes. */
const VIE_MS = 4500

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const counter = useRef(0)

  const retirer = useCallback((id: number) => {
    setToasts((list) => list.filter((toast) => toast.id !== id))
  }, [])

  const push = useCallback(
    (tone: ToastTone, message: string) => {
      counter.current += 1
      const id = counter.current
      setToasts((list) => [...list, { id, tone, message }])
      window.setTimeout(() => retirer(id), VIE_MS)
    },
    [retirer],
  )

  const value = useMemo<ToastContextValue>(
    () => ({
      success: (message) => push('success', message),
      error: (message) => push('error', message),
      info: (message) => push('info', message),
      warning: (message) => push('warning', message),
    }),
    [push],
  )

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-4 z-100 flex flex-col items-center gap-2 px-4">
        <AnimatePresence initial={false}>
          {toasts.map((toast) => (
            <motion.div
              key={toast.id}
              layout
              variants={toastVariants}
              initial="hidden"
              animate="visible"
              exit="exit"
              role="status"
              className="card-glass pointer-events-auto relative flex w-full max-w-md items-start gap-2.5 overflow-hidden px-3.5 py-3 text-sm shadow-float"
            >
              <span
                className={clsx(
                  'mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full border',
                  TONES[toast.tone].ring,
                )}
              >
                {TONES[toast.tone].icon}
              </span>
              <p className="min-w-0 flex-1 break-words leading-relaxed text-ink">{toast.message}</p>
              <button
                type="button"
                className="shrink-0 cursor-pointer rounded p-0.5 text-ink-muted transition-colors duration-200 ease-soft hover:text-ink"
                onClick={() => retirer(toast.id)}
                aria-label="Fermer"
              >
                <X size={15} />
              </button>
              {/* Barre de vie : se vide sur la durée d'affichage. */}
              <motion.span
                initial={{ scaleX: 1 }}
                animate={{ scaleX: 0 }}
                transition={{ duration: VIE_MS / 1000, ease: 'linear' }}
                className={clsx('absolute inset-x-0 bottom-0 h-0.5 origin-left', TONES[toast.tone].bar)}
                style={{ opacity: 0.5 }}
              />
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ToastContext.Provider>
  )
}

/** Hook d'usage : const toast = useToast(); toast.success('…'). */
export function useToast(): ToastContextValue {
  const context = useContext(ToastContext)
  if (!context) {
    // Sans provider, on n'échoue pas : les messages sont perdus silencieusement.
    return { success: () => { }, error: () => { }, info: () => { }, warning: () => { } }
  }
  return context
}

/** Durée d'affichage par défaut, exportée pour les tests. */
export const TOAST_DURATION_MS = VIE_MS
export const TOAST_TRANSITION = { duration: DUR.fast, ease: EASE_SOFT }
