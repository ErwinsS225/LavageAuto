/**
 * Bibliothèque de composants UI WashGuard — point d'entrée unique.
 *
 * Les écrans importent depuis `@/components/ui` : les composants sont rangés par
 * famille (primitives, surfaces, données, formulaires, surépaisseurs, états,
 * blocs animés) mais l'import reste le même partout, ce qui évite de réécrire
 * chaque page le jour où l'organisation interne change.
 */
export {
  Badge,
  Button,
  IconButton,
  Segmented,
  toneClass,
  type BadgeTone,
  type ButtonVariant,
} from '@/components/ui/primitives'
export {
  Card,
  CardHeader,
  DefinitionNote,
  Money,
  Progress,
  PageHeader,
  ScreenHero,
  SectionTitle,
  StatCard,
  type StatTone,
} from '@/components/ui/surfaces'
export { Table, TableFrame, MotionRow, rowDelay } from '@/components/ui/data'
export { EmptyState, ErrorState, PageFallback, Skeleton, SkeletonRows, Spinner, AnimatedNumber } from '@/components/ui/feedback'
export { Field, Input, Select, Switch, Textarea, type SelectOption } from '@/components/ui/forms'
export { ConfirmDialog, Modal } from '@/components/ui/overlays'
export { MotionItem, MotionPage, MotionStagger, Reveal, ValuePulse } from '@/components/ui/motion-blocks'
