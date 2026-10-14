/**
 * Écran récapitulatif de mise en route (/mise-en-route/recapitulatif).
 *
 * Refonte complète dans le style @blocks-so/onboarding-02 :
 *  - Coche circulaire émeraude (IconCircleCheckFilled)
 *  - Indicateurs de numérotation propres et fluides
 *  - Synthèse par étape avec icônes de domaine
 *  - Bloc d'activation sécurisé avec bouton Rocket
 */
import { Link, useNavigate } from 'react-router-dom'
import type { Icon } from '@tabler/icons-react'
import {
  IconArrowLeft,
  IconArrowRight,
  IconBriefcase,
  IconBuilding,
  IconBuildingStore,
  IconCar,
  IconCircleCheckFilled,
  IconClock,
  IconCoins,
  IconCreditCard,
  IconMapPin,
  IconRocket,
  IconShieldCheck,
  IconSparkles,
  IconTag,
  IconTool,
  IconUsers,
} from '@tabler/icons-react'

import { Button, Card, CardHeader, ErrorState, SkeletonRows } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { SetupPanel } from '@/features/onboarding/SetupPanel'
import { useActivateOnboarding, useOnboardingSummary } from '@/features/onboarding/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { ApiError } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

const ICONS_ETAPES: Record<string, Icon> = {
  espace: IconBuildingStore,
  entreprise: IconBuilding,
  station: IconMapPin,
  horaires: IconClock,
  vehicules: IconCar,
  usages: IconBriefcase,
  prestations: IconSparkles,
  options: IconTag,
  tarifs: IconCoins,
  moyens: IconCreditCard,
  equipe: IconUsers,
  materiel: IconTool,
  commandes: IconShieldCheck,
}

function StepIndicator({
  index,
  isCompleted,
}: {
  index: number
  isCompleted: boolean
}) {
  if (isCompleted) {
    return (
      <div className="flex size-7 items-center justify-center shrink-0">
        <IconCircleCheckFilled
          aria-hidden="true"
          className="size-7 text-emerald-500"
        />
      </div>
    )
  }
  return (
    <div className="flex size-7 items-center justify-center rounded-full font-semibold text-xs shrink-0 bg-surface-muted text-ink-muted border border-line">
      {index + 1}
    </div>
  )
}

export default function ActivationPage() {
  const navigate = useNavigate()
  const toast = useToast()
  usePageTitle('Récapitulatif de mise en route')
  const { can } = useAuth()
  const peutLire = can('settings:view')
  const { data, isError, isLoading, refetch } = useOnboardingSummary(peutLire)
  const activation = useActivateOnboarding()

  const etapes = data?.steps ?? []
  const setup = data?.setup
  const active = setup?.activation.active ?? false
  const bloqueurs = setup?.activation.blockers ?? []
  const peutActiver = can('organization:manage')

  const totalCount = etapes.length
  const completedCount = etapes.filter((e) => e.done).length
  const requiredCount = etapes.filter((e) => e.required).length
  const requiredDoneCount = etapes.filter((e) => e.required && e.done).length
  const allRequiredDone = setup?.activation.can_activate ?? (requiredCount > 0 && requiredDoneCount === requiredCount)

  async function activer() {
    try {
      await activation.mutateAsync()
      toast.success('Félicitations ! Votre station est maintenant active et ouverte à l’exploitation.')
      navigate('/tableau-de-bord', { replace: true })
    } catch (caught) {
      const message =
        caught instanceof ApiError
          ? caught.message
          : "L'activation a échoué. Vérifiez vos prérequis puis réessayez."
      toast.error(message)
      void refetch()
    }
  }

  return (
    <div className="flex min-h-dvh flex-col items-center justify-start py-8 px-4 sm:px-6">
      <div className="w-full max-w-2xl space-y-6">

        {/* =========================================================================
            HEADER DE RÉCAPITULATIF
            ========================================================================= */}
        <div className="rounded-2xl border border-line bg-surface p-6 shadow-e2">
          <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <span className="flex size-8 items-center justify-center rounded-lg bg-brand-soft text-brand">
                  <IconShieldCheck className="size-5" />
                </span>
                <h1 className="text-xl font-bold tracking-tight text-ink">
                  Récapitulatif avant activation
                </h1>
              </div>
              <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                Vérifiez la conformité de vos informations enregistrées avant d’ouvrir officiellement votre station de lavage.
              </p>
            </div>

            <Link to="/onboarding" className="shrink-0">
              <Button variant="secondary" size="sm" icon={<IconArrowLeft className="size-4" />}>
                Modifier les étapes
              </Button>
            </Link>
          </div>

          {/* Indicateur de statut */}
          <div className="mt-5 border-t border-line/60 pt-4">
            <div className="flex items-center justify-between text-xs">
              <span className="text-ink-muted">
                {allRequiredDone ? (
                  <span className="font-semibold text-emerald-600 dark:text-emerald-400 flex items-center gap-1.5">
                    <IconCircleCheckFilled className="size-4" />
                    Toutes les étapes obligatoires sont satisfaites
                  </span>
                ) : (
                  <>
                    <span className="font-semibold text-ink">{requiredDoneCount}</span> sur{' '}
                    <span className="font-semibold text-ink">{requiredCount}</span> étapes obligatoires complétées
                  </>
                )}
              </span>
              <span className="font-semibold text-ink">
                {completedCount} / {totalCount} au total
              </span>
            </div>

            <div className="mt-2.5 h-2 w-full overflow-hidden rounded-full bg-surface-muted border border-line/50">
              <div
                className="h-full rounded-full bg-gradient-to-r from-brand to-emerald-500 transition-all duration-500"
                style={{ width: `${Math.max(5, Math.round((completedCount / (totalCount || 1)) * 100))}%` }}
              />
            </div>
          </div>
        </div>

        {!peutLire ? (
          <Card>
            <CardHeader
              title="Récapitulatif réservé"
              subtitle="Votre profil ne dispose pas des droits de consultation de la mise en route."
            />
          </Card>
        ) : isLoading ? (
          <div className="space-y-4">
            <SkeletonRows rows={4} />
          </div>
        ) : isError || !data ? (
          <ErrorState
            message="Le récapitulatif n'a pas pu être lu."
            onRetry={() => void refetch()}
          />
        ) : (
          <>
            {/* CARTE D'ACTIVATION FINALE */}
            <div className="rounded-2xl border border-line bg-surface p-6 shadow-e2 transition-colors">
              <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                <div className="space-y-1">
                  <h3 className="text-base font-bold text-ink flex items-center gap-2">
                    <IconRocket className="size-5 text-brand" />
                    {active ? 'Station en exploitation' : 'Activer définitivement la station'}
                  </h3>
                  <p className="text-xs text-ink-muted leading-relaxed max-w-lg">
                    {active
                      ? `Station activée${data.activated_by?.activated_at ? ` le ${formatDateTime(data.activated_by.activated_at)}` : ''} par ${data.activated_by?.activated_by_name || 'le propriétaire'}.`
                      : peutActiver
                        ? allRequiredDone
                          ? 'Le serveur a validé l’ensemble des données. Vous pouvez ouvrir la station maintenant.'
                          : `Il reste ${bloqueurs.length} élément(s) obligatoire(s) à compléter.`
                        : 'Seul le propriétaire du compte est autorisé à activer l’exploitation.'}
                  </p>
                </div>

                <div className="shrink-0">
                  {active ? (
                    <Button
                      variant="primary"
                      onClick={() => navigate('/tableau-de-bord')}
                    >
                      Aller au tableau de bord
                      <IconArrowRight className="size-4 ml-1" />
                    </Button>
                  ) : (
                    <Button
                      variant="primary"
                      loading={activation.isPending}
                      disabled={!allRequiredDone || !peutActiver}
                      onClick={() => void activer()}
                      className="shadow-e2"
                    >
                      <IconRocket className="size-4 -ml-0.5" />
                      Activer ma station
                    </Button>
                  )}
                </div>
              </div>

              {bloqueurs.length > 0 && !active && (
                <div className="mt-4 pt-4 border-t border-line/60 space-y-2">
                  <p className="text-xs font-semibold text-warning">Points bloquants avant activation :</p>
                  <ul className="space-y-1.5">
                    {bloqueurs.map((b) => (
                      <li key={b.code} className="text-xs text-warning/90 flex items-center gap-2">
                        <span>⚠️</span>
                        <span>
                          <strong>{b.title}</strong> — {b.hint}
                        </span>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>

            {/* SETUP PANEL SI DISPONIBLE */}
            {setup && (
              <SetupPanel setup={setup} />
            )}

            {/* LISTE RÉCAPITULATIVE DES ÉTAPES (@blocks-so/onboarding-02 style) */}
            <div className="space-y-2.5">
              <div className="flex items-center justify-between px-1">
                <h3 className="text-sm font-bold text-ink">
                  Détail des {etapes.length} étapes
                </h3>
                <span className="text-xs text-ink-muted">
                  Validé en temps réel
                </span>
              </div>

              <div className="space-y-2.5">
                {etapes.map((step, index) => {
                  const StepIcon = ICONS_ETAPES[step.code] || IconSparkles
                  const isCompleted = step.done

                  return (
                    <div
                      key={step.code}
                      className={cn(
                        'rounded-xl border p-4 transition-all duration-200 bg-surface',
                        isCompleted
                          ? 'border-line/70'
                          : 'border-line shadow-e1'
                      )}
                    >
                      <div className="flex gap-3.5">
                        <div className="mt-0.5 shrink-0">
                          <StepIndicator index={index} isCompleted={isCompleted} />
                        </div>

                        <div className="min-w-0 flex-1">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center gap-2">
                                <p
                                  className={cn(
                                    'text-sm leading-6 tracking-tight',
                                    isCompleted
                                      ? 'text-ink-muted/50 dark:text-muted-foreground/50 line-through'
                                      : 'text-ink font-semibold'
                                  )}
                                >
                                  {step.title}
                                </p>
                                {!step.required && (
                                  <span className="rounded px-1.5 py-0.5 text-[0.6875rem] font-medium bg-surface-muted text-ink-subtle border border-line">
                                    Facultatif
                                  </span>
                                )}
                                {step.detail && (
                                  <span className="rounded px-1.5 py-0.5 text-[0.6875rem] font-medium bg-brand-soft text-brand">
                                    {step.detail}
                                  </span>
                                )}
                              </div>

                              <p
                                className={cn(
                                  'mt-0.5 text-xs leading-relaxed',
                                  isCompleted ? 'text-ink-muted/50' : 'text-ink-muted/80'
                                )}
                              >
                                {step.description}
                              </p>
                              {!isCompleted && step.hint && (
                                <p className="mt-1 text-xs text-warning">{step.hint}</p>
                              )}
                            </div>

                            <StepIcon
                              aria-hidden="true"
                              className={cn(
                                'size-5 shrink-0 mt-0.5',
                                isCompleted ? 'text-emerald-500/80' : 'text-ink-muted/50'
                              )}
                            />
                          </div>
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </div>
          </>
        )}

      </div>
    </div>
  )
}
