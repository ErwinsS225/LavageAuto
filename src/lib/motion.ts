/**
 * Mouvements de l'interface — une seule source pour tout le produit.
 *
 * Rien n'est animé « au feeling » dans les écrans : les durées, les courbes et
 * les ressorts sont définis ici, puis réutilisés. Trois règles :
 *
 *  1. **court** — une transition d'interface se sent plus qu'elle ne se voit :
 *     140 ms pour un survol, 220 ms pour une apparition, 420 ms pour une page,
 *     jamais plus d'une demi-seconde sauf les animations de graphiques ;
 *  2. **très doux à l'arrivée** — la courbe `EASE_SOFT` part vite et freine
 *     longuement, ce qui donne l'impression d'un objet posé plutôt que coupé ;
 *  3. **une seule idée par mouvement** — on décale légèrement, on estompe, on
 *     soulève. Jamais de rebond marqué, jamais de rotation, jamais de zoom
 *     brutal : c'est une caisse de station de lavage, pas une publicité.
 *
 * `MotionConfig reducedMotion="user"` (voir main.tsx) neutralise les
 * animations pour qui a demandé « réduire les animations » dans son système.
 */
import type { Transition, Variants } from 'framer-motion'

// ---------------------------------------------------------------------------
// Durées et courbes
// ---------------------------------------------------------------------------

/** Durées en secondes (API Framer Motion). */
export const DUR = {
  /** Survol, enfoncement d'un bouton. */
  fast: 0.14,
  /** Apparition d'un bloc, ouverture d'un menu. */
  base: 0.22,
  /** Déplacement, révélation d'une liste. */
  slow: 0.36,
  /** Changement d'écran. */
  page: 0.42,
  /** Animations de graphiques. */
  chart: 0.7,
} as const

/** Sortie très douce : rapide au départ, longue à se poser. */
export const EASE_SOFT: [number, number, number, number] = [0.22, 1, 0.36, 1]

/** Sortie standard (matérielle) : pour les déplacements courts. */
export const EASE_SNAP: [number, number, number, number] = [0.4, 0, 0.2, 1]

/** Ressort des panneaux flottants (modales, menus) : franc mais sans rebond. */
export const SPRING: Transition = { type: 'spring', stiffness: 320, damping: 32, mass: 0.9 }

/** Ressort des surfaces plus larges (tiroir, panneau latéral). */
export const SPRING_SOFT: Transition = { type: 'spring', stiffness: 210, damping: 28 }

/** Transition « apparition » par défaut : opacité + léger décalage. */
export const TRANSITION_SOFT: Transition = { duration: DUR.base, ease: EASE_SOFT }

// ---------------------------------------------------------------------------
// Variantes réutilisables
// ---------------------------------------------------------------------------

export const fadeIn: Variants = {
  hidden: { opacity: 0 },
  visible: { opacity: 1, transition: TRANSITION_SOFT },
  exit: { opacity: 0, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

export const fadeUp: Variants = {
  hidden: { opacity: 0, y: 10 },
  visible: { opacity: 1, y: 0, transition: { duration: DUR.slow, ease: EASE_SOFT } },
  exit: { opacity: 0, y: -6, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

export const fadeUpSmall: Variants = {
  hidden: { opacity: 0, y: 5 },
  visible: { opacity: 1, y: 0, transition: TRANSITION_SOFT },
}

export const scaleIn: Variants = {
  hidden: { opacity: 0, scale: 0.97 },
  visible: { opacity: 1, scale: 1, transition: { duration: DUR.base, ease: EASE_SOFT } },
  exit: { opacity: 0, scale: 0.98, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

/** Conteneur qui échelonne l'apparition de ses enfants (listes, grilles). */
export function stagger(staggerChildren = 0.045, delayChildren = 0.02): Variants {
  return {
    hidden: {},
    visible: { transition: { staggerChildren, delayChildren } },
    exit: {},
  }
}

/** Enfant d'un conteneur échelonné : une ligne de tableau, une carte… */
export const item: Variants = {
  hidden: { opacity: 0, y: 6 },
  visible: { opacity: 1, y: 0, transition: { duration: DUR.base, ease: EASE_SOFT } },
  exit: { opacity: 0, y: -4, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

/** Changement d'écran : contenu qui se pose, sans glissement latéral. */
export const pageVariants: Variants = {
  hidden: { opacity: 0, y: 8 },
  visible: { opacity: 1, y: 0, transition: { duration: DUR.page, ease: EASE_SOFT } },
  exit: { opacity: 0, y: -6, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

/** Fond assombri d'une modale : apparition douce, disparition plus vive. */
export const backdropVariants: Variants = {
  hidden: { opacity: 0 },
  visible: { opacity: 1, transition: { duration: DUR.base, ease: EASE_SNAP } },
  exit: { opacity: 0, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

/** Panneau d'une modale : léger zoom + décalage vers le haut. */
export const modalVariants: Variants = {
  hidden: { opacity: 0, scale: 0.97, y: 12 },
  visible: { opacity: 1, scale: 1, y: 0, transition: SPRING },
  exit: { opacity: 0, scale: 0.98, y: 8, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

/** Tiroir latéral (menu mobile). */
export const drawerVariants: Variants = {
  hidden: { x: '-100%' },
  visible: { x: 0, transition: SPRING_SOFT },
  exit: { x: '-100%', transition: { duration: DUR.base, ease: EASE_SNAP } },
}

/** Menu déroulant ancré sous un bouton. */
export const menuVariants: Variants = {
  hidden: { opacity: 0, y: -6, scale: 0.98 },
  visible: { opacity: 1, y: 0, scale: 1, transition: { duration: DUR.base, ease: EASE_SOFT } },
  exit: { opacity: 0, y: -4, scale: 0.99, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

/** Notification qui entre par la droite. */
export const toastVariants: Variants = {
  hidden: { opacity: 0, x: 24, scale: 0.98 },
  visible: { opacity: 1, x: 0, scale: 1, transition: SPRING },
  exit: { opacity: 0, x: 16, scale: 0.98, transition: { duration: DUR.fast, ease: EASE_SNAP } },
}

// ---------------------------------------------------------------------------
// Raccourcis d'interaction
// ---------------------------------------------------------------------------

/** Survol d'une surface : elle se soulève d'un demi-pixel visuel. */
export const hoverLift = {
  whileHover: { y: -2, transition: { duration: DUR.fast, ease: EASE_SOFT } },
  whileTap: { y: 0, scale: 0.995, transition: { duration: DUR.fast, ease: EASE_SNAP } },
} as const

/** Enfoncement d'un bouton. */
export const pressable = {
  whileHover: { y: -1 },
  whileTap: { scale: 0.975 },
  transition: { duration: DUR.fast, ease: EASE_SNAP },
} as const

/** Apparition à l'entrée dans la fenêtre (sections longues, vitrine). */
export const inView = {
  initial: 'hidden' as const,
  whileInView: 'visible' as const,
  viewport: { once: true, margin: '-60px' },
}

/**
 * Attend la fin d'un délai avant de révéler une valeur (compteurs animés).
 * Utilisé par `<AnimatedNumber />` : le rendu initial affiche la valeur finale,
 * ce qui évite tout clignotement et garde les contrôles de rendu fiables.
 */
export function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3)
}
