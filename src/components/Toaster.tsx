/**
 * Toaster : notifications éphémères (succès, erreur, info).
 * Contexte React simple — pas de dépendance externe.
 */
import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { CheckCircle2, Info, TriangleAlert, X, XCircle } from 'lucide-react'

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

const TONES: Record<ToastTone, { icon: ReactNode; className: string }> = {
  success: { icon: <CheckCircle2 size={18} />, className: 'border-success/40 text-success' },
  error: { icon: <XCircle size={18} />, className: 'border-danger/40 text-danger' },
  info: { icon: <Info size={18} />, className: 'border-brand/40 text-brand' },
  warning: { icon: <TriangleAlert size={18} />, className: 'border-warning/40 text-warning' },
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const counter = useRef(0)

  const push = useCallback((tone: ToastTone, message: string) => {
    counter.current += 1
    const id = counter.current
    setToasts((list) => [...list, { id, tone, message }])
    window.setTimeout(() => {
      setToasts((list) => list.filter((toast) => toast.id !== id))
    }, 4500)
  }, [])

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
        {toasts.map((toast) => (
          <div
            key={toast.id}
            className={`pointer-events-auto flex w-full max-w-md items-center gap-2.5 rounded-xl border bg-surface px-4 py-3 text-sm shadow-lg ${TONES[toast.tone].className}`}
            role="status"
          >
            <span className="shrink-0">{TONES[toast.tone].icon}</span>
            <p className="min-w-0 flex-1 break-words text-ink">{toast.message}</p>
            <button
              type="button"
              className="shrink-0 rounded p-0.5 text-ink-muted hover:text-ink"
              onClick={() => setToasts((list) => list.filter((item) => item.id !== toast.id))}
              aria-label="Fermer"
            >
              <X size={15} />
            </button>
          </div>
        ))}
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
