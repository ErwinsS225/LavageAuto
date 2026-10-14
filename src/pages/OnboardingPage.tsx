/**
 * Écran de mise en route (/onboarding).
 *
 * Un assistant, pas une liste de liens : chaque étape ouvre le formulaire qui
 * l'alimente, et l'enregistrement écrit réellement dans la base de l'entreprise.
 * Ce qui décide qu'une étape est terminée reste le serveur — il recalcule tout à
 * partir des données (station active, téléphone, horaires décrits, prestations
 * tarifées), jamais d'un clic sur « Terminer ».
 */
import { ArrowRight, CheckCircle2, Circle, Rocket, UserPlus } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'

import { Badge, Button, Card, CardHeader, PageHeader, Progress, SkeletonRows } from '@/components/ui'
import { MotionItem, MotionStagger } from '@/components/ui/motion-blocks'
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
import type { OnboardingStep } from '@/types'

const ETIQUETTES: Record<OnboardingStep['state'], { texte: string; ton: 'success' | 'brand' | 'warning' | 'neutral' }> = {
  completed: { texte: 'Terminée', ton: 'success' },
  in_progress: { texte: 'En cours', ton: 'brand' },
  not_started: { texte: 'À faire', ton: 'neutral' },
  needs_fix: { texte: 'À corriger', ton: 'warning' },
  blocked: { texte: 'Bloquée', ton: 'warning' },
}

export default function OnboardingPage() {
  const navigate = useNavigate()
  const toast = useToast()
  usePageTitle('Mise en route')
  const { can, me } = useAuth()
  const { stationId, stations } = useStation()

  const { data, isError, error, refetch } = useOnboarding(can('settings:view'), me?.onboarding ?? null)
  const activation = useActivateOnboarding()
  const [etapeActive, setEtapeActive] = useState('')

  const etapes = data?.steps ?? []
  const pourcent = data?.progress.percent ?? 0
  const peutActiver = can('organization:manage')
  const stationCourante = stationId || stations[0]?.id || ''

  // L'étape affichée : la première non satisfaite, sauf choix explicite.
  const premiereARemplir = useMemo(
    () => etapes.find((etape) => !etape.done && etape.form)?.code ?? etapes[0]?.code ?? '',
    [etapes],
  )
  useEffect(() => {
    if (etapeActive) return
    if (premiereARemplir) setEtapeActive(premiereARemplir)
  }, [etapeActive, premiereARemplir])

  const etape = etapes.find((item) => item.code === etapeActive)
  const organisation = useOrganisationDetaillee(can('settings:view') && etape?.form === 'entreprise')
  // Le profil complet n'est utile qu'aux deux étapes qui décrivent le lieu ;
  // les autres lisent leurs propres listes.
  const profilStation = useStationProfile(
    stationCourante,
    Boolean(stationCourante) && (etape?.form === 'station' || etape?.form === 'horaires'),
  )

  const reste = (data?.missing ?? []).map((item) => item.title)
  const etatActif = data?.state === 'active'

  /** Après un enregistrement : on rafraîchit l'état et on propose la suite. */
  function apresEnregistrement(code: string) {
    void refetch()
    const position = etapes.findIndex((item) => item.code === code)
    const suivante = etapes.slice(position + 1).find((item) => item.form)
    if (suivante) setEtapeActive(suivante.code)
  }

  async function activer() {
    try {
      await activation.mutateAsync()
      toast.success('Station activée. Bonne exploitation !')
      navigate('/tableau-de-bord', { replace: true })
    } catch (caught) {
      // Le serveur est la source de vérité : son refus nomme l'étape manquante.
      const message =
        caught instanceof ApiError
          ? caught.message
          : "L'activation a échoué. Vérifiez votre connexion puis réessayez."
      toast.error(message)
      void refetch()
    }
  }

  if (!data) {
    if (isError) {
      const message =
        error instanceof Error ? error.message : "L'état de la mise en route n'a pas pu être lu."
      return (
        <div className="mx-auto w-full max-w-2xl">
          <Card>
            <CardHeader title="Mise en route indisponible" subtitle={message} />
            <Button variant="secondary" onClick={() => void refetch()}>
              Réessayer
            </Button>
          </Card>
        </div>
      )
    }
    return (
      <div className="mx-auto w-full max-w-3xl">
        <SkeletonRows rows={4} />
      </div>
    )
  }

  return (
    <MotionStagger className="mx-auto w-full max-w-4xl space-y-6">
      <MotionItem>
        <PageHeader
          title="Mise en route de votre station"
          subtitle="Chaque étape se remplit ici et s'enregistre dans votre espace. Vous ne saisissez que vos propres informations : l'application ne propose ni tarif ni donnée d'exemple."
        />
      </MotionItem>

      <MotionItem>
        <Card>
          <CardHeader
            title={`Avancement : ${data.progress.done} / ${data.progress.total} étapes obligatoires`}
            subtitle={
              etatActif
                ? 'Station activée : votre espace est en exploitation.'
                : "L'exploitation s'ouvre à la dernière étape."
            }
            actions={<Badge tone={etatActif ? 'success' : 'brand'}>{pourcent} %</Badge>}
          />
          <div className="p-4 pt-0">
            <Progress percent={pourcent} tone={etatActif ? 'success' : 'brand'} />
          </div>
        </Card>
      </MotionItem>

      {/* Navigation entre les étapes : l'état de chacune vient du serveur. */}
      <MotionItem>
        <div className="flex flex-wrap gap-2">
          {etapes.map((item, index) => {
            const active = item.code === etapeActive
            return (
              <button
                key={item.code}
                type="button"
                onClick={() => setEtapeActive(item.code)}
                className={
                  'chip flex items-center gap-2 ' + (active ? 'chip-active' : '')
                }
                aria-current={active}
              >
                {item.done ? (
                  <CheckCircle2 size={14} className="text-success" aria-hidden />
                ) : (
                  <Circle size={14} className="text-ink-subtle" aria-hidden />
                )}
                <span className="text-xs">
                  {index + 1}. {item.title}
                </span>
              </button>
            )
          })}
        </div>
      </MotionItem>

      {etape && (
        <MotionItem>
          <Card>
            <CardHeader
              title={etape.title}
              subtitle={etape.description}
              actions={
                <div className="flex flex-wrap items-center gap-2">
                  <Badge tone={(ETIQUETTES[etape.state] ?? ETIQUETTES.not_started).ton} dot>
                    {(ETIQUETTES[etape.state] ?? ETIQUETTES.not_started).texte}
                  </Badge>
                  {!etape.required && <Badge tone="neutral">Facultatif</Badge>}
                </div>
              }
            />
            <div className="space-y-4 p-4">
              {etape.detail ? (
                <p className="text-sm text-ink-muted">
                  <span className="font-medium text-ink">Constaté :</span> {etape.detail}
                </p>
              ) : null}
              {!etape.done && etape.hint ? <p className="text-sm text-warning">{etape.hint}</p> : null}

              {/* Le formulaire de l'étape, alimenté par les données réelles. */}
              {etape.form === 'entreprise' && organisation.data && (
                <EntrepriseForm
                  donnees={organisation.data}
                  onEnregistre={() => apresEnregistrement(etape.code)}
                />
              )}
              {etape.form === 'station' && stationCourante && profilStation.data && (
                <StationForm stationId={stationCourante} onEnregistre={() => apresEnregistrement(etape.code)} />
              )}
              {etape.form === 'horaires' && stationCourante && (
                <HorairesForm stationId={stationCourante} onEnregistre={() => apresEnregistrement(etape.code)} />
              )}
              {etape.form === 'vehicules' && stationCourante && (
                <VehiculesForm stationId={stationCourante} onEnregistre={() => apresEnregistrement(etape.code)} />
              )}
              {etape.form === 'usages' && stationCourante && (
                <UsagesForm stationId={stationCourante} onEnregistre={() => apresEnregistrement(etape.code)} />
              )}
              {etape.form === 'prestations' && stationCourante && (
                <PrestationsForm stationId={stationCourante} onEnregistre={() => apresEnregistrement(etape.code)} />
              )}
              {etape.form === 'options' && stationCourante && (
                <OptionsForm stationId={stationCourante} onEnregistre={() => apresEnregistrement(etape.code)} />
              )}
              {etape.form === 'tarifs' && stationCourante && (
                <TarifsForm stationId={stationCourante} onEnregistre={() => apresEnregistrement(etape.code)} />
              )}
              {etape.form === 'moyens' && stationCourante && (
                <MoyensForm
                  stationId={stationCourante}
                  ville={stations.find((item) => item.id === stationCourante)?.city}
                  onEnregistre={() => apresEnregistrement(etape.code)}
                />
              )}
              {etape.form === 'equipe' && stationCourante && (
                <EquipeForm stationId={stationCourante} />
              )}
              {etape.form === 'materiel' && stationCourante && (
                <MaterielForm stationId={stationCourante} />
              )}
              {etape.form === 'commandes' && stationCourante && (
                <CommandesForm stationId={stationCourante} />
              )}
              {/* Étape « espace » : rien à saisir, mais deux chemins possibles.
                  Celui de l'invitation n'est pas dans ce parcours — il se vit
                  depuis un lien reçu, avant même d'avoir un compte. */}
              {etape.code === 'espace' && (
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="rounded-field border border-line bg-surface-muted/60 px-4 py-3">
                    <p className="text-sm font-medium text-ink">Créer mon entreprise</p>
                    <p className="mt-1 text-xs leading-relaxed text-ink-muted">
                      C'est ce que vous venez de faire : votre compte est <strong>propriétaire</strong> de
                      cette entreprise. Il n'y a pas de second propriétaire à nommer.
                    </p>
                  </div>
                  <div className="rounded-field border border-line px-4 py-3">
                    <p className="text-sm font-medium text-ink">Rejoindre une entreprise existante</p>
                    <p className="mt-1 text-xs leading-relaxed text-ink-muted">
                      Se fait avec une invitation, avant la création du compte — le vôtre est déjà créé.
                      Vous pouvez néanmoins ouvrir la page pour vérifier une invitation reçue.
                    </p>
                    <Link
                      to="/rejoindre"
                      className="mt-2 inline-flex items-center gap-1 text-xs font-medium text-brand hover:underline"
                    >
                      <UserPlus size={14} aria-hidden />
                      J'ai reçu une invitation
                    </Link>
                  </div>
                </div>
              )}

              {!etape.form && etape.code !== 'espace' && (
                <p className="text-sm text-ink-muted">
                  Cette étape n'a rien à saisir : elle est constatée sur ce qui existe déjà.
                </p>
              )}
            </div>
          </Card>
        </MotionItem>
      )}

      <MotionItem>
        <Card>
          <CardHeader
            title={etatActif ? 'Espace en exploitation' : 'Activer la station'}
            subtitle={
              etatActif
                ? 'Vous pouvez encaisser, enregistrer des lavages et suivre votre activité.'
                : reste.length > 0
                  ? `Il reste : ${reste.join(', ')}.`
                  : 'Toutes les étapes obligatoires sont satisfaites : vous pouvez activer.'
            }
          />
          <div className="space-y-3 p-4 pt-0">
            {etatActif ? (
              <Button variant="primary" icon={<ArrowRight size={15} />} onClick={() => navigate('/tableau-de-bord')}>
                Aller au tableau de bord
              </Button>
            ) : (
              <>
                <Button
                  variant="primary"
                  icon={<Rocket size={15} />}
                  loading={activation.isPending}
                  disabled={!data.can_activate || !peutActiver}
                  onClick={() => void activer()}
                >
                  Activer ma station
                </Button>
                {!peutActiver && (
                  <p className="text-sm text-ink-muted">
                    Seul le propriétaire du compte peut activer la station.
                  </p>
                )}
                {peutActiver && !data.can_activate && (
                  <p className="text-sm text-ink-muted">
                    Le bouton s'active dès que toutes les étapes obligatoires sont satisfaites. L'activation
                    est revérifiée côté serveur, étape par étape.
                  </p>
                )}
              </>
            )}
          </div>
        </Card>
      </MotionItem>
    </MotionStagger>
  )
}
