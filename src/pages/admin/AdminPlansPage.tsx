/**
 * Plateforme — gestion des formules d'abonnement : prix et limites.
 */
import { useState } from 'react'
import { Pencil } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { Button, Card, CardHeader, ErrorState, Field, Input, Modal, PageHeader, Spinner, Table } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { api, ApiError } from '@/lib/api'
import { usePageTitle } from '@/hooks/usePageTitle'
import { formatNumber, formatXOF, parseAmountInput } from '@/lib/format'
import type { Plan } from '@/types'

interface PlanForm {
  name: string
  description: string
  price: number
  max_stations: number
  max_employees: number
  max_wash_orders_month: number
  trial_days: number
}

export default function AdminPlansPage() {
  const toast = useToast()
  usePageTitle('Formules')
  const queryClient = useQueryClient()

  const plansQuery = useQuery<{ items: Plan[] }>({
    queryKey: ['admin-plans-manage'],
    queryFn: () => api.get('/platform/plans'),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, ...input }: PlanForm & { id: string }) => api.patch(`/platform/plans/${id}`, input),
    onSuccess: () => {
      toast.success('Formule mise à jour.')
      void queryClient.invalidateQueries({ queryKey: ['admin-plans-manage'] })
      void queryClient.invalidateQueries({ queryKey: ['admin-plans'] })
    },
  })

  const [editing, setEditing] = useState<Plan | null>(null)
  const [form, setForm] = useState<PlanForm>({
    name: '',
    description: '',
    price: 0,
    max_stations: 0,
    max_employees: 0,
    max_wash_orders_month: 0,
    trial_days: 0,
  })
  const [error, setError] = useState('')

  function openForm(plan: Plan) {
    setEditing(plan)
    setForm({
      name: plan.name,
      description: plan.description,
      price: plan.price_xof,
      max_stations: plan.max_stations,
      max_employees: plan.max_employees,
      max_wash_orders_month: plan.max_wash_orders_month,
      trial_days: plan.trial_days,
    })
    setError('')
  }

  async function submit() {
    if (!editing) return
    try {
      await updateMutation.mutateAsync({ id: editing.id, ...form })
      setEditing(null)
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="Formules" subtitle="Tarifs et limites de chaque formule d'abonnement." />

      {plansQuery.isLoading ? (
        <Spinner label="Chargement des formules…" />
      ) : plansQuery.isError ? (
        <ErrorState message="Impossible de charger les formules." onRetry={() => void plansQuery.refetch()} />
      ) : (
        <Card>
          <CardHeader title="Formules proposées" subtitle={`${plansQuery.data?.items.length ?? 0} formule(s)`} />
          <Table
            columns={['Formule', 'Prix', 'Stations', 'Employés', 'Lavages / mois', 'Essai', 'Abonnés', '']}
            rows={(plansQuery.data?.items ?? []).map((plan) => [
              plan.name,
              formatXOF(plan.price_xof),
              formatNumber(plan.max_stations),
              formatNumber(plan.max_employees),
              formatNumber(plan.max_wash_orders_month),
              `${plan.trial_days} j`,
              formatNumber(plan.organization_count),
              <Button key={plan.id} size="sm" variant="ghost" icon={<Pencil size={15} />} onClick={() => openForm(plan)}>
                Modifier
              </Button>,
            ])}
          />
        </Card>
      )}

      <Modal
        open={Boolean(editing)}
        title={editing ? `Modifier ${editing.name}` : 'Formule'}
        onClose={() => setEditing(null)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setEditing(null)} disabled={updateMutation.isPending}>
              Annuler
            </Button>
            <Button variant="primary" onClick={submit} loading={updateMutation.isPending}>
              Enregistrer
            </Button>
          </>
        }
      >
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Nom" required>
            {(id) => <Input id={id} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} />}
          </Field>
          <Field label="Prix (FCFA)" required>
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={form.price === 0 ? '' : form.price}
                onChange={(event) => setForm({ ...form, price: parseAmountInput(event.target.value) })}
              />
            )}
          </Field>
          <Field label="Description">
            {(id) => <Input id={id} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />}
          </Field>
          <Field label="Essai (jours)">
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={form.trial_days}
                onChange={(event) => setForm({ ...form, trial_days: parseAmountInput(event.target.value) })}
              />
            )}
          </Field>
          <Field label="Max stations" hint="0 = illimité">
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={form.max_stations}
                onChange={(event) => setForm({ ...form, max_stations: parseAmountInput(event.target.value) })}
              />
            )}
          </Field>
          <Field label="Max employés" hint="0 = illimité">
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={form.max_employees}
                onChange={(event) => setForm({ ...form, max_employees: parseAmountInput(event.target.value) })}
              />
            )}
          </Field>
          <Field label="Max lavages / mois" hint="0 = illimité">
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={form.max_wash_orders_month}
                onChange={(event) => setForm({ ...form, max_wash_orders_month: parseAmountInput(event.target.value) })}
              />
            )}
          </Field>
        </div>
        {error ? <p className="mt-3 rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
      </Modal>
    </div>
  )
}
