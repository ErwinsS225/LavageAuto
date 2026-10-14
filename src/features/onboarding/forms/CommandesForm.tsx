/**
 * Étape 13 du parcours : les règles de fonctionnement des commandes.
 *
 * Chaque réglage de cet écran est **appliqué par le serveur**, à l'endroit où
 * il décide : la création d'une fiche, l'encaissement, le changement de statut.
 * Aucun interrupteur n'est décoratif — c'est la raison pour laquelle il n'y en
 * a que sept, et pas vingt.
 *
 * Le tableau du bas est le cycle de vie d'un véhicule dans la station. Il vient
 * du serveur, transitions comprises : l'écran l'affiche, il ne l'invente pas.
 */
import { useEffect, useState } from 'react'

import {
  Badge,
  Button,
  Card,
  CardHeader,
  Field,
  Input,
  SkeletonRows,
  Switch,
} from '@/components/ui'
import { useOrderRules, useUpdateOrderRules } from '@/features/maintenance/api'
import { useToast } from '@/components/Toaster'
import { useAuth } from '@/hooks/useAuth'
import { ApiError } from '@/lib/api'
import type { OrderRules } from '@/types'

const REGLAGES: Array<{
  cle: keyof OrderRules
  label: string
  hint: string
}> = [
  {
    cle: 'allow_walk_in',
    label: 'Accepter les clients de passage',
    hint: 'Désactivé : toute fiche doit être rattachée à un client enregistré.',
  },
  {
    cle: 'require_washer',
    label: 'Exiger un laveur dès la création de la fiche',
    hint: 'Désactivé : une fiche peut être enregistrée avant d’être attribuée.',
  },
  {
    cle: 'quality_check_required',
    label: 'Contrôle qualité obligatoire avant « Prête »',
    hint: 'Le véhicule passe par « Contrôle qualité » — on ne saute pas l’étape.',
  },
  {
    cle: 'release_requires_payment',
    label: 'Ne remettre le véhicule qu’une fois la fiche soldée',
    hint: 'Désactivé : la remise peut précéder le règlement (client facturé plus tard).',
  },
  {
    cle: 'allow_partial_payments',
    label: 'Accepter les acomptes',
    hint: 'Désactivé : une fiche se règle en une seule fois.',
  },
  {
    cle: 'require_cancel_reason',
    label: 'Exiger un motif pour annuler une fiche',
    hint: 'Le motif reste lisible dans l’historique de la fiche.',
  },
]

export default function CommandesForm({ stationId }: { stationId: string }) {
  const toast = useToast()
  const requete = useOrderRules(stationId)
  const enregistrement = useUpdateOrderRules(stationId)
  const { can } = useAuth()
  const peutGerer = can('settings:manage')
  const [regles, setRegles] = useState<OrderRules | null>(null)

  useEffect(() => {
    if (requete.data) setRegles(requete.data.rules)
  }, [requete.data])

  async function enregistrer() {
    if (!regles) return
    try {
      await enregistrement.mutateAsync(regles)
      toast.success('Règles enregistrées : elles s’appliquent dès la prochaine fiche.')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement n'a pas abouti.")
    }
  }

  if (!regles) {
    return (
      <Card>
        <CardHeader
          title="Comment un véhicule circule dans la station"
          subtitle="Ces règles sont appliquées par le serveur, pas seulement affichées : une fiche refusée reçoit un motif lisible."
        />
        <div className="space-y-3 p-4">
          {requete.isLoading ? (
            <SkeletonRows rows={3} />
          ) : (
            <>
              <p className="text-sm text-ink-muted">
                Les règles de la station n'ont pas pu être lues depuis le serveur.
              </p>
              <Button variant="secondary" onClick={() => void requete.refetch()}>
                Réessayer
              </Button>
            </>
          )}
        </div>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="Comment un véhicule circule dans la station"
          subtitle="Ces règles sont appliquées par le serveur, pas seulement affichées : une fiche refusée reçoit un motif lisible."
          actions={
            peutGerer ? (
              <Button onClick={() => void enregistrer()} disabled={enregistrement.isPending}>
                Enregistrer les règles
              </Button>
            ) : (
              <Badge tone="neutral">Lecture seule</Badge>
            )
          }
        />
        <div className="space-y-4 p-4">
          {REGLAGES.map((reglage) => (
            <Switch
              key={reglage.cle}
              checked={Boolean(regles[reglage.cle])}
              label={reglage.label}
              hint={reglage.hint}
              disabled={!peutGerer}
              onChange={(valeur) => setRegles({ ...regles, [reglage.cle]: valeur })}
            />
          ))}
          <Field
            label="Fiches en cours par laveur"
            hint="0 = aucune limite. Un laveur qui atteint la limite ne reçoit plus de nouvelle fiche : la file d'attente reste tenable."
          >
            {(id) => (
              <Input
                id={id}
                type="number"
                min={0}
                max={100}
                value={String(regles.max_open_orders_per_washer)}
                disabled={!peutGerer}
                onChange={(event) =>
                  setRegles({ ...regles, max_open_orders_per_washer: Number(event.target.value || 0) })
                }
              />
            )}
          </Field>
        </div>
      </Card>

      <Card>
        <CardHeader
          title="Cycle de vie d'une fiche"
          subtitle="« Reçue » et « en attente » ne font qu'un : la réception est l'acte de création de la fiche."
        />
        <div className="space-y-2 p-4">
          {(requete.data?.statuses ?? []).map((statut) => (
            <div
              key={statut.code}
              className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-line p-3"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{statut.label}</span>
                {statut.terminal ? <Badge tone="neutral">État final</Badge> : null}
              </div>
              <span className="text-xs text-ink-muted">{statut.description}</span>
            </div>
          ))}
        </div>
      </Card>
    </div>
  )
}
