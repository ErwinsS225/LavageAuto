/**
 * Moyens de paiement acceptés par la station.
 *
 * La liste vient du serveur : elle est amorcée à la création de la station
 * (espèces actives, le reste disponible) et la station l'ajuste — renommer
 * « Orange Money » en « OM », désactiver ce qu'elle n'accepte pas, ajouter un
 * moyen local. C'est la **famille** qui décide du compteur de caisse : renommer
 * ne change aucun total.
 *
 * Désactiver n'efface pas : un encaissement passé cite son moyen par
 * identifiant et par libellé recopié, il reste donc lisible.
 */
import { useEffect, useState } from 'react'

import { Badge, Button, Card, CardHeader, Field, Input, Select, SkeletonRows, Switch } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useCreatePaymentMethod, usePaymentMethods, useUpdatePaymentMethod } from '@/features/payments/api'
import { ApiError } from '@/lib/api'
import { useAuth } from '@/hooks/useAuth'
import type { PaymentFamily } from '@/types'

const FAMILLES = [
  { value: 'mobile_money', label: 'Mobile money' },
  { value: 'card', label: 'Carte bancaire' },
  { value: 'transfer', label: 'Virement' },
  { value: 'cash', label: 'Espèces' },
  { value: 'deferred', label: 'Paiement différé' },
  { value: 'other', label: 'Autre' },
]

export default function MoyensPanel({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre?: () => void
}) {
  const toast = useToast()
  const { can } = useAuth()
  const gestion = can('settings:manage')

  const moyens = usePaymentMethods(stationId)
  const basculer = useUpdatePaymentMethod()
  const creer = useCreatePaymentMethod()

  const [nouveauNom, setNouveauNom] = useState('')
  const [nouvelleFamille, setNouvelleFamille] = useState<PaymentFamily>('mobile_money')
  const [erreur, setErreur] = useState('')
  const [noms, setNoms] = useState<Record<string, string>>({})

  const items = moyens.data?.items ?? []

  useEffect(() => {
    setNoms(Object.fromEntries(items.map((moyen) => [moyen.id, moyen.name])))
  }, [moyens.data])

  async function changer(corps: Parameters<typeof basculer.mutateAsync>[0]) {
    setErreur('')
    try {
      await basculer.mutateAsync(corps)
      onEnregistre?.()
    } catch (caught) {
      const message = caught instanceof ApiError ? caught.message : "La modification n'a pas abouti."
      setErreur(message)
      // Une désactivation refusée laisse la case cochée : l'écran ne doit pas
      // afficher un état que le serveur a refusé.
      setNoms((precedent) => ({ ...precedent }))
    } finally {
      void moyens.refetch()
    }
  }

  async function ajouter() {
    if (!nouveauNom.trim()) {
      setErreur('Donnez un nom au moyen de paiement.')
      return
    }
    setErreur('')
    try {
      await creer.mutateAsync({
        station_id: stationId,
        name: nouveauNom.trim(),
        family: nouvelleFamille,
        is_active: true,
      })
      setNouveauNom('')
      toast.success(`« ${nouveauNom.trim()} » ajouté à la caisse.`)
      onEnregistre?.()
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "L'ajout n'a pas abouti.")
    }
  }

  return (
    <Card>
      <CardHeader
        title="Moyens de paiement acceptés"
        subtitle="Ce que la caisse propose à l'encaissement. Seuls les moyens actifs sont acceptés."
        actions={
          <Badge tone={items.some((moyen) => moyen.is_active) ? 'success' : 'danger'}>
            {items.filter((moyen) => moyen.is_active).length} actif(s)
          </Badge>
        }
      />
      <div className="space-y-3 p-4">
        {moyens.isLoading ? (
          <SkeletonRows rows={4} />
        ) : items.length === 0 ? (
          <p className="text-sm text-ink-muted">
            Aucun moyen de paiement n&apos;est déclaré pour cette station. Ajoutez-en un ci-dessous :
            sans moyen actif, la caisse ne peut rien encaisser.
          </p>
        ) : (
          <ul className="divide-y divide-line">
            {items.map((moyen) => (
              <li key={moyen.id} className="flex flex-wrap items-center gap-3 py-3">
                <div className="min-w-[12rem] flex-1">
                  <p className="text-sm font-medium text-ink">{moyen.name}</p>
                  <p className="text-xs text-ink-muted">
                    {moyen.family_label}
                    {moyen.requires_reference ? ' · référence exigée' : ''}
                    {moyen.kind === 'personnalisee' ? ' · créé par la station' : ''}
                  </p>
                </div>

                {gestion ? (
                  <>
                    <div className="flex items-center gap-2">
                      <Input
                        aria-label={`Nom affiché pour ${moyen.name}`}
                        value={noms[moyen.id] ?? moyen.name}
                        onChange={(event) =>
                          setNoms((precedent) => ({ ...precedent, [moyen.id]: event.target.value }))
                        }
                        className="w-44"
                      />
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={
                          basculer.isPending ||
                          !(noms[moyen.id] ?? moyen.name).trim() ||
                          (noms[moyen.id] ?? moyen.name) === moyen.name
                        }
                        onClick={() =>
                          void changer({
                            id: moyen.id,
                            station_id: stationId,
                            name: (noms[moyen.id] ?? moyen.name).trim(),
                          })
                        }
                      >
                        Renommer
                      </Button>
                    </div>
                    <Switch
                      checked={moyen.is_active}
                      disabled={basculer.isPending}
                      label={moyen.is_active ? 'Accepté' : 'Refusé'}
                      onChange={(actif) =>
                        void changer({ id: moyen.id, station_id: stationId, is_active: actif })
                      }
                    />
                  </>
                ) : (
                  <Badge tone={moyen.is_active ? 'success' : 'neutral'}>
                    {moyen.is_active ? 'Accepté' : 'Refusé'}
                  </Badge>
                )}
              </li>
            ))}
          </ul>
        )}

        {gestion ? (
          <div className="rounded-field border border-line bg-surface-muted/50 p-3">
            <p className="text-sm font-medium text-ink">Ajouter un moyen que la station accepte</p>
            <div className="mt-3 grid gap-3 sm:grid-cols-[2fr,1fr,auto] sm:items-end">
              <Field label="Nom affiché" hint="Ex : Airtel Money, Chèque, Bon de lavage.">
                {(id) => (
                  <Input
                    id={id}
                    value={nouveauNom}
                    onChange={(event) => setNouveauNom(event.target.value)}
                    placeholder="Airtel Money"
                  />
                )}
              </Field>
              <Field label="Famille comptable" hint="Elle décide du compteur de caisse alimenté.">
                {(id) => (
                  <Select
                    id={id}
                    value={nouvelleFamille}
                    onChange={(event) => setNouvelleFamille(event.target.value as PaymentFamily)}
                    options={FAMILLES}
                  />
                )}
              </Field>
              <Button variant="primary" loading={creer.isPending} onClick={() => void ajouter()}>
                Ajouter
              </Button>
            </div>
          </div>
        ) : null}

        {erreur ? (
          <p className="rounded-field border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">
            {erreur}
          </p>
        ) : null}
      </div>
    </Card>
  )
}
