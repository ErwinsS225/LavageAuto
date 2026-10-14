/**
 * Écran de mise en route (/onboarding).
 *
 * Refonte complète basée sur le composant @blocks-so/onboarding-02 :
 *  - Indicateurs circulaires avec icône de coche émeraude (IconCircleCheckFilled)
 *  - Titres barrés avec typographie douce une fois validés
 *  - Barres de progression dynamique en émeraude
 *  - Cartes d'étapes interactives avec accordéon inline pour chaque formulaire
 *  - Détection automatique et passage à l'étape suivante après enregistrement
 *  - Validation serveur et activation finale avec IconRocket
 */
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import type { Icon } from '@tabler/icons-react'
import {
  IconArrowRight,
  IconBriefcase,
  IconBuilding,
  IconBuildingStore,
  IconCar,
  IconChevronDown,
  IconChevronUp,
  IconCircleCheckFilled,
  IconClock,
  IconCoins,
  IconCreditCard,
  IconFilter,
  IconMapPin,
  IconRocket,
  IconShieldCheck,
  IconSparkles,
  IconTag,
  IconTool,
  IconUserPlus,
  IconUsers,
} from '@tabler/icons-react'

import { Badge, Button, Card, CardHeader, SkeletonRows } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useOrganisationDetaillee, useStationProfile } from '@/features/profile/api'
import { useActivateOnboarding, useOnboarding } from '@/features/onboarding/api'
import EntrepriseForm from '@/features/onboarding/forms/EntrepriseForm'
import HorairesForm from '@/features/onboarding/forms/HorairesForm'
import MaterielForm from '@/features/onboarding/forms/MaterielForm'
import EquipeForm from '@/features/onboarding/forms/EquipeForm'
import CommandesForm from '@/features/onboarding/forms/CommandesForm'
import MoyensForm from '@/features/onboarding/forms/MoyensForm'
import OptionsForm from '@/features/onboarding/forms/OptionsForm'
import PrestationsForm from '@/features/onboarding/forms/PrestationsForm'
import TarifsForm from '@/features/onboarding/forms/TarifsForm'
import StationForm from '@/features/onboarding/forms/StationForm'
import UsagesForm from '@/features/onboarding/forms/UsagesForm'
import VehiculesForm from '@/features/onboarding/forms/VehiculesForm'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import { cn } from '@/lib/utils'

/** Association d'une icône Tabler adaptée pour chaque étape du lavage automobile */
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

function getIconColor(isCompleted: boolean, isActive: boolean) {
  if (isCompleted) {
    return 'text-emerald-500/80 dark:text-emerald-400/80'
  }
  if (isActive) {
    return 'text-brand'
  }
  return 'text-ink-muted/50 dark:text-ink-muted/40'
}

function getTitleClass(isCompleted: boolean, isActive: boolean) {
  if (isCompleted) {
    return 'text-ink-muted/50 dark:text-muted-foreground/50 line-through'
  }
  if (isActive) {
    return 'text-ink dark:text-foreground font-semibold'
  }
  return 'text-ink/80 dark:text-foreground/80 font-medium'
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
      <div className="flex size-7 items-center justify-center shrink-0">
        <IconCircleCheckFilled
          aria-hidden="true"
          className="size-7 text-emerald-500 animate-in fade-in zoom-in-75 duration-200"
        />
      </div>
    )
  }
  return (
    <div
      className={cn(
        'flex size-7 items-center justify-center rounded-full font-semibold text-xs shrink-0 transition-colors',
        isActive
          ? 'bg-brand text-brand-ink shadow-e1 ring-2 ring-brand/30'
          : 'bg-surface-muted text-ink-muted border border-line'
      )}
    >
      {index + 1}
    </div>
  )
}

export default function OnboardingPage() {
  const navigate = useNavigate()
  const toast = useToast()
  usePageTitle('Mise en route de votre station')
  const { can, me } = useAuth()
  const { stationId, stations } = useStation()

  const { data, isError, error, refetch } = useOnboarding(can('settings:view'), me?.onboarding ?? null)
  const activation = useActivateOnboarding()

  const [etapeOuverte, setEtapeOuverte] = useState<string>('')
  const [filtre, setFiltre] = useState<'tous' | 'obligatoires' | 'a_faire'>('tous')

  const etapes = data?.steps ?? []
  const stationCourante = stationId || stations[0]?.id || ''
  const peutActiver = can('organization:manage')
  const etatActif = data?.state === 'active'

  // Première étape non satisfaite par défaut
  const premiereARemplir = useMemo(
    () => etapes.find((e) => !e.done && e.form)?.code ?? etapes[0]?.code ?? '',
    [etapes],
  )

  useEffect(() => {
    if (!etapeOuverte && premiereARemplir) {
      setEtapeOuverte(premiereARemplir)
    }
  }, [etapeOuverte, premiereARemplir])

  const etapeActiveData = etapes.find((item) => item.code === etapeOuverte)
  const organisation = useOrganisationDetaillee(can('settings:view') && etapeActiveData?.form === 'entreprise')
  const profilStation = useStationProfile(
    stationCourante,
    Boolean(stationCourante) && (etapeActiveData?.form === 'station' || etapeActiveData?.form === 'horaires'),
  )

  // Statistiques de progression
  const totalEtapes = etapes.length
  const completedCount = etapes.filter((e) => e.done).length
  const requiredCount = etapes.filter((e) => e.required).length
  const requiredDoneCount = etapes.filter((e) => e.required && e.done).length
  const allRequiredDone = data?.can_activate || (requiredCount > 0 && requiredDoneCount === requiredCount)
  const pourcent = data?.progress?.percent ?? Math.round((completedCount / (totalEtapes || 1)) * 100)

  // Filtrage des étapes
  const etapesAffichees = useMemo(() => {
    if (filtre === 'obligatoires') return etapes.filter((e) => e.required)
    if (filtre === 'a_faire') return etapes.filter((e) => !e.done)
    return etapes
  }, [etapes, filtre])

  function apresEnregistrement(code: string) {
    void refetch()
    toast.success('Informations enregistrées avec succès !')
    // Trouver l'étape suivante non terminée
    const indexActuel = etapes.findIndex((e) => e.code === code)
    const suivante = etapes.slice(indexActuel + 1).find((e) => !e.done && e.form)
    if (suivante) {
      setEtapeOuverte(suivante.code)
    }
  }

  async function activer() {
    try {
      await activation.mutateAsync()
      toast.success('Félicitations ! Votre station de lavage est désormais active et prête pour l’exploitation.')
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

  if (!data) {
    if (isError) {
      const message = error instanceof Error ? error.message : "Impossible de charger la mise en route."
      return (
        <div className="mx-auto w-full max-w-xl py-12">
          <Card>
            <CardHeader title="Mise en route momentanément indisponible" subtitle={message} />
            <div className="p-4 pt-0">
              <Button variant="secondary" onClick={() => void refetch()}>
                Réessayer
              </Button>
            </div>
          </Card>
        </div>
      )
    }
    return (
      <div className="mx-auto w-full max-w-xl py-12 space-y-4">
        <SkeletonRows rows={5} />
      </div>
    )
  }

  return (
    <div className="flex min-h-dvh flex-col items-center justify-start py-8 px-4 sm:px-6">
      <div className="w-full max-w-2xl space-y-6">

        {/* =========================================================================
            HEADER DE MISE EN ROUTE (@blocks-so/onboarding-02 style)
            ========================================================================= */}
        <div className="rounded-2xl border border-line bg-surface p-6 shadow-e2 transition-colors">
          <div className="flex items-start justify-between gap-4">
            <div>
              <div className="flex items-center gap-2">
                <span className="flex size-8 items-center justify-center rounded-lg bg-brand-soft text-brand">
                  <IconRocket className="size-5" />
                </span>
                <h1 className="text-xl font-bold tracking-tight text-ink">
                  Mise en route de votre station
                </h1>
              </div>
              <p className="mt-2 text-sm leading-relaxed text-ink-muted">
                Configurez pas à pas vos tarifs, vos horaires, vos véhicules et vos moyens de paiement.
                Chaque étape est enregistrée et validée directement par le système.
              </p>
            </div>

            {etatActif ? (
              <Badge tone="success" dot className="shrink-0">
                En exploitation
              </Badge>
            ) : allRequiredDone ? (
              <Badge tone="success" dot className="shrink-0 animate-pulse">
                Prête à activer
              </Badge>
            ) : (
              <Badge tone="brand" dot className="shrink-0">
                {pourcent} %
              </Badge>
            )}
          </div>

          {/* Barre de progression avec repères chiffrés */}
          <div className="mt-5 border-t border-line/60 pt-4">
            <div className="flex items-center justify-between text-xs">
              <span className="text-ink-muted">
                {allRequiredDone ? (
                  <span className="font-semibold text-emerald-600 dark:text-emerald-400 flex items-center gap-1.5">
                    <IconCircleCheckFilled className="size-4" />
                    Toutes les étapes requises sont prêtes 🎉
                  </span>
                ) : (
                  <>
                    <span className="font-semibold text-ink">
                      {requiredDoneCount}
                    </span>{' '}
                    sur <span className="font-semibold text-ink">{requiredCount}</span> étapes obligatoires complétées
                  </>
                )}
              </span>
              <span className="font-semibold text-ink">
                {completedCount} / {totalEtapes} au total
              </span>
            </div>

            <div className="mt-2.5 h-2 w-full overflow-hidden rounded-full bg-surface-muted border border-line/50">
              <div
                className="h-full rounded-full bg-gradient-to-r from-brand to-emerald-500 transition-all duration-500"
                style={{ width: `${Math.max(4, pourcent)}%` }}
              />
            </div>
          </div>

          {/* Filtres rapides */}
          <div className="mt-4 flex flex-wrap items-center gap-1.5 pt-1 text-xs">
            <span className="inline-flex items-center gap-1 text-ink-subtle mr-1">
              <IconFilter className="size-3.5" /> Filtrer :
            </span>
            <button
              type="button"
              onClick={() => setFiltre('tous')}
              className={cn(
                'rounded-lg px-2.5 py-1 font-medium transition-colors',
                filtre === 'tous'
                  ? 'bg-brand text-brand-ink shadow-e1'
                  : 'bg-surface-muted text-ink-muted hover:text-ink'
              )}
            >
              Toutes ({etapes.length})
            </button>
            <button
              type="button"
              onClick={() => setFiltre('obligatoires')}
              className={cn(
                'rounded-lg px-2.5 py-1 font-medium transition-colors',
                filtre === 'obligatoires'
                  ? 'bg-brand text-brand-ink shadow-e1'
                  : 'bg-surface-muted text-ink-muted hover:text-ink'
              )}
            >
              Obligatoires ({requiredCount})
            </button>
            <button
              type="button"
              onClick={() => setFiltre('a_faire')}
              className={cn(
                'rounded-lg px-2.5 py-1 font-medium transition-colors',
                filtre === 'a_faire'
                  ? 'bg-brand text-brand-ink shadow-e1'
                  : 'bg-surface-muted text-ink-muted hover:text-ink'
              )}
            >
              À compléter ({etapes.filter((e) => !e.done).length})
            </button>
          </div>
        </div>

        {/* =========================================================================
            LISTE DES CARTES D'ÉTAPES (@blocks-so/onboarding-02 style)
            ========================================================================= */}
        <div className="space-y-3">
          {etapesAffichees.map((step, index) => {
            const isCompleted = step.done
            const isOpen = etapeOuverte === step.code
            const StepIcon = ICONS_ETAPES[step.code] || IconSparkles
            const absoluteIndex = etapes.findIndex((e) => e.code === step.code)

            return (
              <div
                key={step.code}
                className={cn(
                  'rounded-xl border transition-all duration-200 overflow-hidden',
                  isOpen
                    ? 'border-brand/40 bg-surface shadow-e2 ring-1 ring-brand/20'
                    : isCompleted
                      ? 'border-line/70 bg-surface/80 hover:border-line-strong'
                      : 'border-line bg-surface hover:border-line-strong shadow-e1'
                )}
              >
                {/* En-tête de la carte d'étape */}
                <div
                  className="p-4 cursor-pointer select-none"
                  onClick={() => setEtapeOuverte(isOpen ? '' : step.code)}
                >
                  <div className="flex gap-3.5">
                    {/* Indicateur circulaire (index ou coche verte) */}
                    <div className="mt-0.5 shrink-0">
                      <StepIndicator
                        index={absoluteIndex >= 0 ? absoluteIndex : index}
                        isActive={isOpen}
                        isCompleted={isCompleted}
                      />
                    </div>

                    <div className="min-w-0 flex-1">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <p className={cn('text-sm leading-6 tracking-tight', getTitleClass(isCompleted, isOpen))}>
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
                              isOpen
                                ? 'text-ink-muted'
                                : isCompleted
                                  ? 'text-ink-muted/50'
                                  : 'text-ink-muted/80'
                            )}
                          >
                            {step.description}
                          </p>

                          {/* Bouton d'action standard si non ouvert */}
                          {!isOpen && step.form && (
                            <div className="mt-2.5 flex items-center gap-2">
                              <Button
                                size="sm"
                                variant={isCompleted ? 'secondary' : 'primary'}
                                onClick={(e) => {
                                  e.stopPropagation()
                                  setEtapeOuverte(step.code)
                                }}
                              >
                                <StepIcon aria-hidden="true" className="-ml-0.5 size-3.5 shrink-0" />
                                {isCompleted ? 'Modifier la saisie' : 'Configurer'}
                              </Button>
                            </div>
                          )}
                        </div>

                        {/* Icône du domaine et flèche d'expansion */}
                        <div className="flex items-center gap-2 shrink-0">
                          <StepIcon
                            aria-hidden="true"
                            className={cn('size-5 shrink-0 transition-colors', getIconColor(isCompleted, isOpen))}
                          />
                          <span className="text-ink-subtle hover:text-ink transition-colors">
                            {isOpen ? <IconChevronUp className="size-4" /> : <IconChevronDown className="size-4" />}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                {/* =====================================================================
                    FORMULAIRE INLINE DÉPLOYABLE (Accordéon)
                    ===================================================================== */}
                <AnimatePresence initial={false}>
                  {isOpen && (
                    <motion.div
                      initial={{ height: 0, opacity: 0 }}
                      animate={{ height: 'auto', opacity: 1 }}
                      exit={{ height: 0, opacity: 0 }}
                      transition={{ duration: 0.22, ease: [0.22, 1, 0.36, 1] }}
                      className="overflow-hidden border-t border-line/70 bg-surface-muted/30"
                    >
                      <div className="p-4 sm:p-5 space-y-4">
                        {step.hint && !isCompleted && (
                          <div className="rounded-lg border border-warning/30 bg-warning-soft/60 px-3.5 py-2.5 text-xs text-warning leading-relaxed flex items-center gap-2">
                            <span>💡</span>
                            <span>{step.hint}</span>
                          </div>
                        )}

                        {/* Rendu dynamique du formulaire relié à l'étape */}
                        {step.form === 'entreprise' && organisation.data && (
                          <EntrepriseForm
                            donnees={organisation.data}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'station' && stationCourante && profilStation.data && (
                          <StationForm
                            stationId={stationCourante}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'horaires' && stationCourante && (
                          <HorairesForm
                            stationId={stationCourante}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'vehicules' && stationCourante && (
                          <VehiculesForm
                            stationId={stationCourante}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'usages' && stationCourante && (
                          <UsagesForm
                            stationId={stationCourante}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'prestations' && stationCourante && (
                          <PrestationsForm
                            stationId={stationCourante}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'options' && stationCourante && (
                          <OptionsForm
                            stationId={stationCourante}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'tarifs' && stationCourante && (
                          <TarifsForm
                            stationId={stationCourante}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'moyens' && stationCourante && (
                          <MoyensForm
                            stationId={stationCourante}
                            ville={stations.find((item) => item.id === stationCourante)?.city}
                            onEnregistre={() => apresEnregistrement(step.code)}
                          />
                        )}
                        {step.form === 'equipe' && stationCourante && (
                          <EquipeForm stationId={stationCourante} />
                        )}
                        {step.form === 'materiel' && stationCourante && (
                          <MaterielForm stationId={stationCourante} />
                        )}
                        {step.form === 'commandes' && stationCourante && (
                          <CommandesForm stationId={stationCourante} />
                        )}

                        {step.code === 'espace' && (
                          <div className="grid gap-3 sm:grid-cols-2 text-xs">
                            <div className="rounded-lg border border-line bg-surface p-3.5">
                              <p className="font-semibold text-ink">Espace propriétaire créé</p>
                              <p className="mt-1 text-ink-muted leading-relaxed">
                                Votre compte a été initialisé en tant que propriétaire. Vous avez le contrôle total sur cette entreprise.
                              </p>
                            </div>
                            <div className="rounded-lg border border-line bg-surface p-3.5">
                              <p className="font-semibold text-ink">Inviter vos équipes</p>
                              <p className="mt-1 text-ink-muted leading-relaxed">
                                Vous pourrez ajouter vos gérants et caissiers dès l’étape Équipe ou depuis le menu Employés.
                              </p>
                              <Link
                                to="/rejoindre"
                                className="mt-2 inline-flex items-center gap-1 font-medium text-brand hover:underline"
                              >
                                <IconUserPlus className="size-3.5" />
                                Page des invitations
                              </Link>
                            </div>
                          </div>
                        )}

                        {!step.form && step.code !== 'espace' && (
                          <div className="rounded-lg border border-line bg-surface p-4 text-xs text-ink-muted">
                            Cette étape est constatée automatiquement d'après les données de votre compte.
                          </div>
                        )}
                      </div>
                    </motion.div>
                  )}
                </AnimatePresence>
              </div>
            )
          })}
        </div>

        {/* =========================================================================
            SECTION D'ACTIVATION FINALE
            ========================================================================= */}
        <div className="rounded-2xl border border-line bg-surface p-6 shadow-e2 transition-colors">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div className="space-y-1">
              <h3 className="text-base font-bold text-ink flex items-center gap-2">
                <IconRocket className="size-5 text-brand" />
                {etatActif ? 'Station en cours d’exploitation' : 'Activer l’exploitation de la station'}
              </h3>
              <p className="text-xs text-ink-muted leading-relaxed max-w-lg">
                {etatActif
                  ? 'Votre station est active. Vous pouvez créer des fiches, encaisser et suivre votre caisse.'
                  : allRequiredDone
                    ? 'Toutes les étapes obligatoires sont prêtes ! Vous pouvez lancer l’exploitation immédiatement.'
                    : `Il reste des étapes obligatoires à compléter avant l'activation (${requiredDoneCount}/${requiredCount}).`}
              </p>
            </div>

            <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-2.5 shrink-0">
              {etatActif ? (
                <Button
                  variant="primary"
                  onClick={() => navigate('/tableau-de-bord')}
                  className="w-full sm:w-auto"
                >
                  Accéder au tableau de bord
                  <IconArrowRight className="size-4 ml-1" />
                </Button>
              ) : (
                <Button
                  variant="primary"
                  loading={activation.isPending}
                  disabled={!allRequiredDone || !peutActiver}
                  onClick={() => void activer()}
                  className="w-full sm:w-auto shadow-e2"
                >
                  <IconRocket className="size-4 -ml-0.5" />
                  Activer ma station
                </Button>
              )}
            </div>
          </div>

          <div className="mt-4 pt-4 border-t border-line/60 flex items-center justify-between text-xs text-ink-muted">
            <span>
              Besoin de vérifier l'ensemble des données ?
            </span>
            <Link
              to="/mise-en-route/recapitulatif"
              className="font-medium text-brand hover:underline inline-flex items-center gap-1"
            >
              Voir le récapitulatif complet
              <IconArrowRight className="size-3" />
            </Link>
          </div>
        </div>

      </div>
    </div>
  )
}
