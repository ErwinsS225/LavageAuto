/**
 * Blocs animés réutilisables par les écrans.
 *
 * - `MotionStagger` échelonne l'apparition de ses enfants (listes, grilles) ;
 * - `MotionItem` marque un enfant échelonnable ;
 * - `Reveal` fait apparaître un bloc à l'entrée dans la fenêtre (vitrine) ;
 * - `MotionPage` porte la transition d'écran.
 *
 * Tout retombe proprement à l'état final : une animation ne doit jamais laisser
 * un élément à demi transparent.
 */
import type { ReactNode } from 'react'
import { motion } from 'framer-motion'
import clsx from 'clsx'

import { inView, item, pageVariants, stagger, TRANSITION_SOFT } from '@/lib/motion'

export function MotionPage({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <motion.div
      variants={pageVariants}
      initial="hidden"
      animate="visible"
      exit="exit"
      className={clsx('space-y-4 sm:space-y-5', className)}
    >
      {children}
    </motion.div>
  )
}

/** Conteneur échelonné : ses enfants `MotionItem` se posent l'un après l'autre. */
export function MotionStagger({
  children,
  className,
  step = 0.045,
  delay = 0.02,
  as = 'div',
}: {
  children: ReactNode
  className?: string
  step?: number
  delay?: number
  as?: 'div' | 'section'
}) {
  const Composant = as === 'section' ? motion.section : motion.div
  return (
    <Composant variants={stagger(step, delay)} initial="hidden" animate="visible" className={className}>
      {children}
    </Composant>
  )
}

export function MotionItem({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <motion.div variants={item} className={className}>
      {children}
    </motion.div>
  )
}

/** Apparition à l'entrée dans la fenêtre — pour les pages longues (vitrine). */
export function Reveal({
  children,
  className,
  delay = 0,
}: {
  children: ReactNode
  className?: string
  delay?: number
}) {
  return (
    <motion.div
      {...inView}
      variants={{
        hidden: { opacity: 0, y: 14 },
        visible: { opacity: 1, y: 0, transition: { ...TRANSITION_SOFT, duration: 0.5, delay } },
      }}
      className={className}
    >
      {children}
    </motion.div>
  )
}

/** Chiffre qui s'ajuste en douceur quand la donnée change. */
export function ValuePulse({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <motion.span
      key={typeof children === 'string' ? children : undefined}
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={TRANSITION_SOFT}
      className={className}
    >
      {children}
    </motion.span>
  )
}
