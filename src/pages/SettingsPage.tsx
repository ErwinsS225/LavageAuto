/**
 * Paramètres : la même matière que la mise en route, mais pour la vie courante
 * — entreprise, station (localisation et capacités), horaires, règles métier.
 *
 * Les formulaires sont ceux du parcours d'onboarding, pas des copies : une
 * correction de validation ne doit pas exister en deux versions. Tous écrivent
 * dans la base de l'entreprise, et le serveur reste seul juge de ce qui est
 * valide (notamment pour les horaires, refusés jour par jour).
 */
import { useState } from 'react'

import { Badge, Button, Card, CardHeader, Field, Input, PageHeader, Select, Spinner } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import InvitationsPanel from '@/features/invitations/InvitationsPanel'
import EntrepriseForm from '@/features/onboarding/forms/EntrepriseForm'
import HorairesForm from '@/features/onboarding/forms/HorairesForm'
import StationForm from '@/features/onboarding/forms/StationForm'
import CashSettingsForm from '@/features/payments/CashSettingsForm'
import MoyensPanel from '@/features/payments/MoyensPanel'
import { useOrganisationDetaillee } from '@/features/profile/api'
import { useUpdateStation } from '@/features/stations/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import { parseAmountInput } from '@/lib/format'

type Onglet = 'station' | 'horaires' | 'caisse' | 'entreprise' | 'invitations'

const ONGLETS: Array<{ id: Onglet; label: string; permission?: string }> = [
  { id: 'station', label: 'Station' },
  { id: 'horaires', label: 'Horaires' },
  // La caisse et les reçus se paramètrent avec les droits de gestion ; un
  // caissier n'a rien à y faire, et ne verrait donc pas l'onglet.
  { id: 'caisse', label: 'Caisse et reçus', permission: 'settings:view' },
  { id: 'entreprise', label: 'Entreprise' },
  // Les invitations ne concernent que ceux qui gèrent l'équipe.
  { id: 'invitations', label: 'Invitations', permission: 'employee:view' },
]

export default function SettingsPage() {
  const toast = useToast()
  const { can } = useAuth()
  const { stationId, station } = useStation()
  usePageTitle('Paramètres')

  const canManage = can('settings:manage')
  const [onglet, setOnglet] = useState<Onglet>('station')
  const [version, setVersion] = useState(0)
  const organisation = useOrganisationDetaillee(canManage)

  const updateMutation = useUpdateStation()
  const [regles, setRegles] = useState(() => ({
    maxDiscount: station?.settings.max_cashier_discount_xof ?? 1000,
    requireCash: station?.settings.require_open_cash_session ?? true,
    goal: station?.settings.monthly_goal_xof ?? 0,
  }))
  const [error, setError] = useState('')

  async function enregistrerRegles() {
    if (!stationId) return
    setError('')
    try {
      await updateMutation.mutateAsync({
        id: stationId,
        // Le pied de reçu et les autres réglages de caisse sont enregistrés
        // dans l'onglet « Caisse et reçus », par leur propre route validée.
        settings: {
          max_cashier_discount_xof: regles.maxDiscount,
          require_open_cash_session: regles.requireCash,
          monthly_goal_xof: regles.goal,
        },
      })
      toast.success('Règles métier enregistrées.')
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  if (!stationId) return <Spinner label="Chargement des paramètres…" />

  if (!canManage) {
    return (
      <div className="space-y-5">
        <PageHeader title="Paramètres" subtitle="Informations de la station." />
        <Card>
          <CardHeader
            title="Modification réservée au propriétaire"
            subtitle="Votre compte peut consulter ces informations, mais pas les modifier."
            actions={<Badge tone="neutral">Lecture seule</Badge>}
          />
          <dl className="grid gap-4 p-4 sm:grid-cols-2">
            {[
              ['Nom', station?.name],
              ['Téléphone', station?.phone],
              ['Adresse', station?.address],
              ['Ville', station?.city],
            ].map(([libelle, valeur]) => (
              <div key={libelle as string}>
                <dt className="text-xs text-ink-muted">{libelle}</dt>
                <dd className="text-sm text-ink">{valeur || 'Non renseigné'}</dd>
              </div>
            ))}
          </dl>
        </Card>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Paramètres"
        subtitle="Entreprise, station, horaires et règles métier. Tout est enregistré dans votre espace."
      />

      <div className="flex flex-wrap gap-2" role="tablist" aria-label="Sections des paramètres">
        {ONGLETS.filter((item) => !item.permission || can(item.permission)).map((item) => (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={onglet === item.id}
            onClick={() => setOnglet(item.id)}
            className={'chip ' + (onglet === item.id ? 'chip-active' : '')}
          >
            {item.label}
          </button>
        ))}
      </div>

      {onglet === 'station' && (
        <>
          {/* La clé force la relecture des données enregistrées après chaque
              enregistrement : l'écran affiche la base, pas la saisie. */}
          <StationForm
            key={`station-${version}`}
            stationId={stationId}
            onEnregistre={() => setVersion((valeur) => valeur + 1)}
          />

          <Card>
            <CardHeader title="Règles métier" subtitle="Ce que les comptes opérationnels peuvent faire" />
            <div className="grid gap-4 p-4 sm:grid-cols-2">
              <Field label="Plafond de remise caissier (FCFA)" hint="Au-delà, le serveur refuse et demande l'accord du gérant.">
                {(id) => (
                  <Input
                    id={id}
                    inputMode="numeric"
                    value={regles.maxDiscount}
                    onChange={(event) =>
                      setRegles((actuel) => ({ ...actuel, maxDiscount: parseAmountInput(event.target.value) }))
                    }
                  />
                )}
              </Field>
              <Field label="Objectif mensuel (FCFA)" hint="0 = aucun objectif suivi.">
                {(id) => (
                  <Input
                    id={id}
                    inputMode="numeric"
                    value={regles.goal}
                    onChange={(event) => setRegles((actuel) => ({ ...actuel, goal: parseAmountInput(event.target.value) }))}
                  />
                )}
              </Field>
              <Field label="Caisse ouverte obligatoire" hint="Empêche l'encaissement sans session de caisse.">
                {(id) => (
                  <Select
                    id={id}
                    value={regles.requireCash ? 'oui' : 'non'}
                    onChange={(event) => setRegles((actuel) => ({ ...actuel, requireCash: event.target.value === 'oui' }))}
                    options={[
                      { value: 'oui', label: 'Oui — il faut ouvrir la caisse avant d’encaisser' },
                      { value: 'non', label: 'Non — encaissement libre' },
                    ]}
                  />
                )}
              </Field>
              <div className="sm:col-span-2">
                <Button variant="primary" loading={updateMutation.isPending} onClick={() => void enregistrerRegles()}>
                  Enregistrer les règles
                </Button>
              </div>
            </div>
            {error ? <p className="border-t border-line px-4 py-3 text-sm text-danger">{error}</p> : null}
          </Card>
        </>
      )}

      {onglet === 'horaires' && (
        <HorairesForm
          key={`horaires-${version}`}
          stationId={stationId}
          onEnregistre={() => setVersion((valeur) => valeur + 1)}
        />
      )}

      {onglet === 'caisse' && (
        <>
          <MoyensPanel key={`moyens-${version}`} stationId={stationId} />
          <CashSettingsForm
            key={`caisse-${version}`}
            stationId={stationId}
            ville={station?.city}
          />
        </>
      )}

      {onglet === 'invitations' && <InvitationsPanel />}

      {onglet === 'entreprise' && (
        organisation.isLoading ? (
          <Spinner label="Chargement de la fiche d'entreprise…" />
        ) : organisation.data ? (
          <EntrepriseForm
            key={`entreprise-${version}`}
            donnees={organisation.data}
            onEnregistre={() => setVersion((valeur) => valeur + 1)}
          />
        ) : (
          <Card>
            <CardHeader title="Fiche indisponible" subtitle="Elle n'a pas pu être lue depuis le serveur." />
            <div className="p-4">
              <Button variant="secondary" onClick={() => void organisation.refetch()}>
                Réessayer
              </Button>
            </div>
          </Card>
        )
      )}
    </div>
  )
}
