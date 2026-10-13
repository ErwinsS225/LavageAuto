/**
 * Paramètres de la station : identité, horaires et règles métier.
 */
import { useState } from 'react'

import { Button, Card, CardHeader, Field, Input, PageHeader, Select, Spinner, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useUpdateStation, useStations } from '@/features/stations/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import { parseAmountInput } from '@/lib/format'

export default function SettingsPage() {
  const toast = useToast()
  const { can, me } = useAuth()
  const { stationId, station } = useStation()
  usePageTitle('Paramètres')

  const stationsQuery = useStations(Boolean(me))
  const stations = stationsQuery.data ?? []
  const current = stations.find((item) => item.id === stationId) ?? station
  const updateMutation = useUpdateStation()

  const [name, setName] = useState(current?.name ?? '')
  const [phone, setPhone] = useState(current?.phone ?? '')
  const [address, setAddress] = useState(current?.address ?? '')
  const [city, setCity] = useState(current?.city ?? '')
  const [maxDiscount, setMaxDiscount] = useState(current?.settings.max_cashier_discount_xof ?? 1000)
  const [requireCash, setRequireCash] = useState(current?.settings.require_open_cash_session ?? true)
  const [goal, setGoal] = useState(current?.settings.monthly_goal_xof ?? 0)
  const [receiptFooter, setReceiptFooter] = useState(current?.settings.receipt_footer ?? '')
  const [error, setError] = useState('')

  const canManage = can('settings:manage')

  async function submit() {
    if (!current) return
    setError('')
    try {
      await updateMutation.mutateAsync({
        id: current.id,
        name,
        phone,
        address,
        city,
        settings: {
          max_cashier_discount_xof: maxDiscount,
          require_open_cash_session: requireCash,
          monthly_goal_xof: goal,
          receipt_footer: receiptFooter,
        },
      })
      toast.success('Paramètres enregistrés.')
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  if (stationsQuery.isLoading) return <Spinner label="Chargement des paramètres…" />
  if (!current) {
    return <p className="text-sm text-ink-muted">Aucune station sélectionnée.</p>
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Paramètres"
        subtitle="Identité de la station et règles appliquées aux caissiers et laveurs."
        actions={canManage ? <Button variant="primary" onClick={submit} loading={updateMutation.isPending}>Enregistrer</Button> : null}
      />

      <Card>
        <CardHeader title="Identité" subtitle="Visible sur les reçus et rapports" />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <Field label="Nom de la station" required>
            {(id) => <Input id={id} value={name} onChange={(event) => setName(event.target.value)} disabled={!canManage} />}
          </Field>
          <Field label="Téléphone">
            {(id) => <Input id={id} value={phone} onChange={(event) => setPhone(event.target.value)} disabled={!canManage} />}
          </Field>
          <Field label="Adresse">
            {(id) => <Input id={id} value={address} onChange={(event) => setAddress(event.target.value)} disabled={!canManage} />}
          </Field>
          <Field label="Ville">
            {(id) => <Input id={id} value={city} onChange={(event) => setCity(event.target.value)} disabled={!canManage} />}
          </Field>
        </div>
      </Card>

      <Card>
        <CardHeader title="Règles métier" subtitle="Ce que les comptes opérationnels peuvent faire" />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <Field
            label="Plafond de remise caissier (FCFA)"
            hint="Au-delà, le serveur refuse et demande l'accord du gérant."
          >
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={maxDiscount}
                onChange={(event) => setMaxDiscount(parseAmountInput(event.target.value))}
                disabled={!canManage}
              />
            )}
          </Field>
          <Field label="Objectif mensuel (FCFA)" hint="0 = aucun objectif suivi.">
            {(id) => (
              <Input id={id} inputMode="numeric" value={goal} onChange={(event) => setGoal(parseAmountInput(event.target.value))} disabled={!canManage} />
            )}
          </Field>
          <Field label="Caisse ouverte obligatoire" hint="Empêche l'encaissement sans session de caisse.">
            {(id) => (
              <Select
                id={id}
                value={requireCash ? 'oui' : 'non'}
                onChange={(event) => setRequireCash(event.target.value === 'oui')}
                disabled={!canManage}
                options={[
                  { value: 'oui', label: 'Oui — il faut ouvrir la caisse avant d’encaisser' },
                  { value: 'non', label: 'Non — encaissement libre' },
                ]}
              />
            )}
          </Field>
          <Field label="Pied de reçu">
            {(id) => <Textarea id={id} value={receiptFooter} onChange={(event) => setReceiptFooter(event.target.value)} disabled={!canManage} />}
          </Field>
        </div>
      </Card>

      {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
    </div>
  )
}
