import type { Icon } from '@tabler/icons-react'
import {
  IconCircleCheckFilled,
  IconFolder,
  IconPlug,
  IconRocket,
  IconUsers,
} from '@tabler/icons-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export interface StepData {
  title: string
  description: string
  icon: Icon
  actionLabel: string
}

export const defaultSteps: StepData[] = [
  {
    title: 'Créer votre espace',
    description:
      'Nommez votre entreprise et personnalisez votre marque. C’est ici que votre équipe gérera l’exploitation.',
    icon: IconRocket,
    actionLabel: 'Commencer',
  },
  {
    title: 'Inviter votre équipe',
    description:
      'Ajoutez vos collaborateurs (gérants, caissiers, laveurs) pour suivre les lavages et la caisse en temps réel.',
    icon: IconUsers,
    actionLabel: 'Inviter des membres',
  },
  {
    title: 'Configurer vos tarifs et prestations',
    description:
      'Définissez vos types de véhicules et vos tarifs en FCFA pour chaque formule de lavage.',
    icon: IconFolder,
    actionLabel: 'Gérer le catalogue',
  },
  {
    title: 'Connecter vos moyens de paiement',
    description:
      'Activez Wave, Orange Money, MTN MoMo et la caisse espèces pour enregistrer les encaissements.',
    icon: IconPlug,
    actionLabel: 'Voir les paiements',
  },
]

function getIconColor(isCompleted: boolean, isActive: boolean) {
  if (isCompleted) {
    return 'text-ink-muted/30 dark:text-muted-foreground/30'
  }
  if (isActive) {
    return 'text-brand'
  }
  return 'text-ink-muted/40 dark:text-muted-foreground/40'
}

function getTitleClass(isCompleted: boolean, isActive: boolean) {
  if (isCompleted) {
    return 'text-ink-muted/50 dark:text-muted-foreground/50 line-through'
  }
  if (isActive) {
    return 'text-ink dark:text-foreground font-semibold'
  }
  return 'text-ink-muted dark:text-muted-foreground font-medium'
}

function StepIndicator({
  index,
  isCompleted,
  isActive,
}: {
  index: number
  isCompleted: boolean
  isActive: boolean
}) {
  if (isCompleted) {
    return (
      <div className="flex size-7 items-center justify-center">
        <IconCircleCheckFilled
          aria-hidden="true"
          className="size-7 text-emerald-500"
        />
      </div>
    )
  }
  return (
    <div
      className={cn(
        'flex size-7 items-center justify-center rounded-full font-semibold text-xs transition-colors',
        isActive
          ? 'bg-brand text-brand-ink shadow-e1'
          : 'bg-surface-muted text-ink-muted border border-line'
      )}
    >
      {index + 1}
    </div>
  )
}

function StepCard({
  step,
  index,
  isCompleted,
  isActive,
  onComplete,
}: {
  step: StepData
  index: number
  isCompleted: boolean
  isActive: boolean
  onComplete: () => void
}) {
  const StepIcon = step.icon

  return (
    <div
      className={cn(
        'rounded-xl border p-4 transition-all duration-200',
        isActive && 'border-brand/40 bg-surface-muted/60 shadow-e1 ring-1 ring-brand/20',
        !isActive && 'border-line bg-surface hover:border-line-strong'
      )}
    >
      <div className="flex gap-3.5">
        <div className="mt-0.5 shrink-0">
          <StepIndicator
            index={index}
            isActive={isActive}
            isCompleted={isCompleted}
          />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-start gap-3">
            <div className="min-w-0 flex-1">
              <p
                className={cn(
                  'text-sm leading-6 tracking-tight',
                  getTitleClass(isCompleted, isActive)
                )}
              >
                {step.title}
              </p>
              <p
                className={cn(
                  'mt-0.5 text-xs leading-relaxed',
                  isActive
                    ? 'text-ink-muted dark:text-muted-foreground'
                    : 'text-ink-muted/70 dark:text-muted-foreground/60'
                )}
              >
                {step.description}
              </p>
              {isActive && (
                <Button
                  className="mt-3.5"
                  onClick={onComplete}
                  size="sm"
                  variant="primary"
                >
                  <StepIcon
                    aria-hidden="true"
                    className="-ml-0.5 size-4 shrink-0"
                  />
                  {step.actionLabel}
                </Button>
              )}
            </div>
            <StepIcon
              aria-hidden="true"
              className={cn(
                'mt-0.5 size-5 shrink-0',
                getIconColor(isCompleted, isActive)
              )}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

export function Onboarding02({
  steps = defaultSteps,
  title = 'Configuration de votre station',
  subtitle = 'Complétez ces étapes pour lancer l’exploitation de votre station de lavage',
  initialCompleted = [0],
  onStepComplete,
}: {
  steps?: StepData[]
  title?: string
  subtitle?: string
  initialCompleted?: number[]
  onStepComplete?: (index: number) => void
}) {
  const [completedSteps, setCompletedSteps] = useState<Set<number>>(
    () => new Set(initialCompleted)
  )

  const currentStep = steps.findIndex((_, i) => !completedSteps.has(i))
  const completedCount = completedSteps.size
  const allDone = completedCount === steps.length

  const handleComplete = (index: number) => {
    setCompletedSteps((prev) => {
      const next = new Set(prev)
      next.add(index)
      return next
    })
    onStepComplete?.(index)
  }

  return (
    <div className="flex min-h-dvh items-center justify-center p-4">
      <div className="w-full max-w-xl">
        <div className="mb-6">
          <h3 className="font-bold text-ink text-xl tracking-tight">
            {title}
          </h3>
          <p className="mt-1 text-ink-muted text-sm leading-relaxed">
            {subtitle}
          </p>
          <div className="mt-4">
            <div className="flex items-center justify-between text-sm">
              <span className="text-ink-muted text-xs">
                {allDone ? (
                  <span className="font-semibold text-emerald-500 flex items-center gap-1.5">
                    <IconCircleCheckFilled className="size-4" />
                    Toutes les étapes sont prêtes 🎉
                  </span>
                ) : (
                  <>
                    <span className="font-semibold text-ink">
                      {completedCount}
                    </span>{' '}
                    sur {steps.length} étapes terminées
                  </>
                )}
              </span>
              <span className="text-xs font-medium text-ink-muted">
                {Math.round((completedCount / steps.length) * 100)} %
              </span>
            </div>
            <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-surface-muted border border-line/60">
              <div
                className="h-full rounded-full bg-emerald-500 transition-all duration-500"
                style={{
                  width: `${(completedCount / steps.length) * 100}%`,
                }}
              />
            </div>
          </div>
        </div>

        <div className="space-y-3">
          {steps.map((step, index) => (
            <StepCard
              index={index}
              isActive={index === currentStep}
              isCompleted={completedSteps.has(index)}
              key={step.title}
              onComplete={() => handleComplete(index)}
              step={step}
            />
          ))}
        </div>
      </div>
    </div>
  )
}
