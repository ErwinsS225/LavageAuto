/**
 * Écran récapitulatif de mise en route (/mise-en-route/recapitulatif).
 *
 * Dernier écran avant l'activation : tout ce qui a été saisi, ce qui manque
 * encore, et le bouton qui ouvre l'exploitation. Trois règles d'affichage :
 *
 *  1. **Le serveur décide.** Le bouton est désactivé quand un prérequis
 *     manque, mais ce n'est qu'un confort : c'est /onboarding/activate qui
 *     refuse, et il refuse en nommant ce qui manque. L'écran ne prétend jamais
 *     savoir mieux que lui.
 *  2. **Rien n'est inventé.** Chaque ligne affiche le constat du serveur
 *     (« 4 catégories actives », « aucune journée d'ouverture déclarée »).
 *     Aucun montant n'apparaît avant la première fiche — il n'y en a pas.
 *  3. **Le facultatif est dit facultatif.** Les réglages qui ne bloquent pas
 *     l'ouverture sont marqués comme tels, et restent atteignables après.
 */
import { ArrowLeft, CheckCircle2, Circle, Rocket, ShieldCheck } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'

import { Badge, Button, Card, CardHeader, ErrorState, PageHeader, Progress, SkeletonRows } from '@/components/ui'
import { MotionItem, MotionStagger } from '@/components/ui/motion-blocks'
import { useToast } from '@/components/Toaster'
import { SetupPanel } from '@/features/onboarding/SetupPanel'
import { useActivateOnboarding, useOnboardingSummary } from '@/features/onboarding/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { ApiError } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import type { OnboardingStep } from '@/types'

const ETIQUETTES: Record<OnboardingStep['state'], { texte: string; ton: 'success' | 'brand' | 'warning' | 'neutral' }> = {
  completed: { texte: 'Terminée', ton: 'success' },
  in_progress: { texte: 'En cours', ton: 'brand' },
  not_started: { texte: 'À faire', ton: 'neutral' },
  needs_fix: { texte: 'À corriger', ton: 'warning' },
  blocked: { texte: 'Bloquée', ton: 'warning' },
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

  async function activer() {
    try {
      await activation.mutateAsync()
      toast.success('Station activée. Bonne exploitation !')
      navigate('/tableau-de-bord', { replace: true })
    } catch (caught) {
      // Le refus du serveur nomme les étapes manquantes : on l'affiche tel quel.
      const message =
        caught instanceof ApiError
          ? caught.message
          : "L'activation a échoué. Vérifiez votre connexion puis réessayez."
      toast.error(message)
      void refetch()
    }
  }

  return (
    <MotionStagger className="mx-auto w-full max-w-4xl space-y-6">
      <MotionItem>
        <PageHeader
          title="Récapitulatif de mise en route"
          subtitle="Tout ce qui a été renseigné, ce qui manque encore, et l'activation. Les constats viennent du serveur : ils portent sur les données réellement enregistrées."
          actions={
            <Link to="/onboarding">
              <Button variant="secondary" size="sm" icon={<ArrowLeft size={14} />}>
                Revenir au parcours
              </Button>
            </Link>
          }
        />
      </MotionItem>

      {!peutLire ? (
        <MotionItem>
          <Card>
            <CardHeader
              title="Récapitulatif réservé"
              subtitle="Votre profil n'a pas accès à la configuration de la station : demandez au propriétaire ou au gérant."
            />
          </Card>
        </MotionItem>
      ) : isLoading ? (
        <MotionItem>
          <SkeletonRows rows={4} />
        </MotionItem>
      ) : isError || !data ? (
        <MotionItem>
          <ErrorState
            message="Le récapitulatif n'a pas pu être lu."
            onRetry={() => void refetch()}
          />
        </MotionItem>
      ) : (
        <>
          <MotionItem>
            <Card>
              <CardHeader
                title={`Avancement : ${data.progress.done} / ${data.progress.total} étapes obligatoires`}
                subtitle={
                  active
                    ? `Station activée${data.activated_by?.activated_at ? ` le ${formatDateTime(data.activated_by.activated_at)}` : ''}.`
                    : 'L’exploitation s’ouvre à l’activation, une fois les prérequis satisfaits.'
                }
                actions={<Badge tone={active ? 'success' : 'brand'}>{data.progress.percent} %</Badge>}
              />
              <div className="p-4 pt-0">
                <Progress percent={data.progress.percent} tone={active ? 'success' : 'brand'} />
              </div>
            </Card>
          </MotionItem>

          <MotionItem>
            <Card>
              <CardHeader
                title={active ? 'Espace en exploitation' : 'Activer la station'}
                subtitle={
                  active
                    ? 'Vous pouvez encaisser, enregistrer des lavages et suivre votre activité.'
                    : peutActiver
                      ? bloqueurs.length > 0
                        ? `Il reste ${bloqueurs.length} élément(s) obligatoire(s) : ${bloqueurs.map((point) => point.title).join(', ')}.`
                        : 'Tous les prérequis sont satisfaits : l’activation est vérifiée une dernière fois par le serveur.'
                      : 'Seul un profil autorisé à gérer l’entreprise peut activer la station.'
                }
                actions={active ? <ShieldCheck size={18} className="text-success" aria-hidden /> : <Rocket size={18} className="text-brand" aria-hidden />}
              />
              <div className="space-y-3 p-4 pt-0">
                {active ? (
                  <div className="space-y-2">
                    <p className="text-sm text-ink-muted">
                      Activée par {data.activated_by?.activated_by_name || 'un profil autorisé'}
                      {data.activated_by?.activated_at ? ` le ${formatDateTime(data.activated_by.activated_at)}` : ''}.
                    </p>
                    <Link to="/tableau-de-bord">
                      <Button variant="primary">Ouvrir le tableau de bord</Button>
                    </Link>
                  </div>
                ) : (
                  <>
                    <Button
                      variant="primary"
                      icon={<Rocket size={15} />}
                      loading={activation.isPending}
                      disabled={!setup?.activation.can_activate || !peutActiver}
                      onClick={() => void activer()}
                    >
                      Activer ma station
                    </Button>
                    {bloqueurs.length > 0 ? (
                      <ul className="space-y-1">
                        {bloqueurs.map((point) => (
                          <li key={point.code} className="text-sm text-warning">
                            {point.title} — {point.hint}
                          </li>
                        ))}
                      </ul>
                    ) : null}
                    <p className="text-sm text-ink-muted">
                      L’activation engage l’exploitation : le serveur revérifie chaque prérequis dans une seule
                      transaction, et refuse en nommant ce qui manque.
                    </p>
                  </>
                )}
              </div>
            </Card>
          </MotionItem>

          {setup ? (
            <MotionItem>
              <SetupPanel setup={setup} />
            </MotionItem>
          ) : null}

          <MotionItem>
            <Card>
              <CardHeader
                title="Les 13 étapes du parcours"
                subtitle="Chaque étape est calculée sur les données réelles : « terminée » veut dire que la donnée existe, pas qu’un bouton a été cliqué."
              />
              <ul className="divide-y divide-line">
                {etapes.map((etape, index) => {
                  const etiquette = ETIQUETTES[etape.state] ?? ETIQUETTES.not_started
                  return (
                    <li key={etape.code} className="flex flex-wrap items-start justify-between gap-3 p-4">
                      <div className="flex min-w-0 items-start gap-3">
                        {etape.done ? (
                          <CheckCircle2 size={16} className="mt-0.5 shrink-0 text-success" aria-hidden />
                        ) : (
                          <Circle size={16} className="mt-0.5 shrink-0 text-ink-subtle" aria-hidden />
                        )}
                        <div className="min-w-0">
                          <p className="text-sm font-medium text-ink">
                            {index + 1}. {etape.title}{' '}
                            {etape.required ? (
                              <Badge tone="neutral">Obligatoire</Badge>
                            ) : (
                              <Badge tone="neutral">Facultatif</Badge>
                            )}
                          </p>
                          <p className="text-xs text-ink-muted">{etape.detail}</p>
                          {!etape.done ? <p className="text-xs text-warning">{etape.hint}</p> : null}
                        </div>
                      </div>
                      <Badge tone={etiquette.ton} dot>{etiquette.texte}</Badge>
                    </li>
                  )
                })}
              </ul>
            </Card>
          </MotionItem>
        </>
      )}
    </MotionStagger>
  )
}
