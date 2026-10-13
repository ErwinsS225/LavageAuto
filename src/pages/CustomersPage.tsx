/**
 * Clients : liste, recherche et fiche détaillée (historique de lavages).
 */
import { useState } from 'react'
import { Eye, Plus, Search, UserRound } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { Button, Card, EmptyState, ErrorState, Field, Input, Modal, PageHeader, Spinner, StatCard } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { api, ApiError, query } from '@/lib/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { formatDate, formatNumber, formatXOF } from '@/lib/format'
import type { Customer, CustomerDetail } from '@/types'

export default function CustomersPage() {
  const toast = useToast()
  const queryClient = useQueryClient()
  const { can } = useAuth()
  const { stationId } = useStation()
  usePageTitle('Clients')

  const [search, setSearch] = useState('')
  const [detailId, setDetailId] = useState<string | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState({ full_name: '', phone: '', plate: '', vehicle_model: '', notes: '' })
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})

  const listQuery = useQuery<{ items: Customer[]; total: number }>({
    queryKey: ['customers', search, stationId],
    queryFn: () => api.get(`/customers${query({ search: search || undefined, station_id: stationId || undefined })}`),
  })
  const detailQuery = useQuery<CustomerDetail>({
    queryKey: ['customer', detailId],
    queryFn: () => api.get(`/customers/${detailId}`),
    enabled: Boolean(detailId),
  })

  const createMutation = useMutation({
    mutationFn: (input: typeof form) => api.post('/customers', { ...input, station_id: stationId || undefined }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['customers'] })
    },
  })

  const canManage = can('customer:manage')
  const customers = listQuery.data?.items ?? []

  async function submit() {
    setError('')
    setFields({})
    try {
      await createMutation.mutateAsync(form)
      toast.success('Client ajouté.')
      setFormOpen(false)
      setForm({ full_name: '', phone: '', plate: '', vehicle_model: '', notes: '' })
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError("L'enregistrement a échoué.")
      }
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Clients"
        subtitle="Réservez les habitués : plaques, véhicules et historique regroupés par personne."
        actions={
          canManage ? (
            <Button variant="primary" icon={<Plus size={16} />} onClick={() => setFormOpen(true)}>
              Ajouter un client
            </Button>
          ) : null
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard label="Clients enregistrés" value={formatNumber(listQuery.data?.total ?? 0)} icon={<UserRound size={18} />} />
        <StatCard
          label="Lavages cumulés"
          value={formatNumber(customers.reduce((total, customer) => total + customer.wash_count, 0))}
        />
        <StatCard
          label="Dépenses cumulées"
          value={formatXOF(customers.reduce((total, customer) => total + customer.total_spent_xof, 0))}
          tone="success"
        />
      </div>

      <Card>
        <div className="border-b border-line p-3">
          <div className="relative">
            <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-muted" />
            <Input
              className="pl-9"
              placeholder="Rechercher un nom, une plaque…"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
        </div>

        {listQuery.isLoading ? (
          <Spinner label="Chargement des clients…" />
        ) : listQuery.isError ? (
          <ErrorState message="Impossible de charger les clients." onRetry={() => void listQuery.refetch()} />
        ) : customers.length === 0 ? (
          <EmptyState
            icon={<UserRound size={22} />}
            title="Aucun client"
            description="Ajoutez vos habitués pour pré-remplir les fiches de lavage et suivre leurs visites."
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Client</th>
                  <th>Téléphone</th>
                  <th>Véhicule</th>
                  <th className="text-right">Visites</th>
                  <th className="text-right">Dépenses</th>
                  <th>Dernière visite</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {customers.map((customer) => (
                  <tr key={customer.id}>
                    <td className="font-medium text-ink">{customer.full_name}</td>
                    <td className="text-ink-muted">{customer.phone || '—'}</td>
                    <td className="text-ink-muted">
                      {customer.vehicle_model || '—'}
                      {customer.plate ? ` · ${customer.plate}` : ''}
                    </td>
                    <td className="tabular text-right">{formatNumber(customer.wash_count)}</td>
                    <td className="tabular text-right">{formatXOF(customer.total_spent_xof)}</td>
                    <td className="whitespace-nowrap text-ink-muted">
                      {customer.last_visit_at ? formatDate(customer.last_visit_at) : '—'}
                    </td>
                    <td>
                      <div className="flex justify-end">
                        <Button size="sm" variant="ghost" title="Voir la fiche" onClick={() => setDetailId(customer.id)}>
                          <Eye size={15} />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Création */}
      <Modal
        open={formOpen}
        title="Nouveau client"
        onClose={() => setFormOpen(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setFormOpen(false)} disabled={createMutation.isPending}>
              Annuler
            </Button>
            <Button variant="primary" onClick={submit} loading={createMutation.isPending}>
              Enregistrer
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Nom complet" required error={fields.full_name}>
            {(id) => (
              <Input
                id={id}
                value={form.full_name}
                onChange={(event) => setForm({ ...form, full_name: event.target.value })}
                invalid={Boolean(fields.full_name)}
              />
            )}
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Téléphone">
              {(id) => <Input id={id} value={form.phone} onChange={(event) => setForm({ ...form, phone: event.target.value })} />}
            </Field>
            <Field label="Plaque">
              {(id) => <Input id={id} value={form.plate} onChange={(event) => setForm({ ...form, plate: event.target.value })} />}
            </Field>
          </div>
          <Field label="Modèle de véhicule">
            {(id) => <Input id={id} value={form.vehicle_model} onChange={(event) => setForm({ ...form, vehicle_model: event.target.value })} />}
          </Field>
          <Field label="Notes">
            {(id) => <Input id={id} value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} />}
          </Field>
          {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
        </div>
      </Modal>

      {/* Détail */}
      <Modal
        open={Boolean(detailId)}
        title={detailQuery.data ? `Fiche de ${detailQuery.data.customer.full_name}` : 'Fiche client'}
        onClose={() => setDetailId(null)}
        size="lg"
        footer={
          <Button variant="ghost" onClick={() => setDetailId(null)}>
            Fermer
          </Button>
        }
      >
        {detailQuery.isLoading ? (
          <Spinner />
        ) : detailQuery.data ? (
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
              <div className="rounded-lg border border-line p-3">
                <p className="stat-label">Visites</p>
                <p className="stat-value">{formatNumber(detailQuery.data.customer.wash_count)}</p>
              </div>
              <div className="rounded-lg border border-line p-3">
                <p className="stat-label">Dépenses</p>
                <p className="stat-value">{formatXOF(detailQuery.data.customer.total_spent_xof)}</p>
              </div>
              <div className="col-span-2 rounded-lg border border-line p-3">
                <p className="stat-label">Contact</p>
                <p className="text-sm text-ink">
                  {detailQuery.data.customer.phone || '—'}
                  {detailQuery.data.customer.plate ? ` · ${detailQuery.data.customer.plate}` : ''}
                </p>
              </div>
            </div>
            {detailQuery.data.history.length === 0 ? (
              <p className="text-sm text-ink-muted">Aucun lavage enregistré pour ce client.</p>
            ) : (
              <div className="overflow-x-auto rounded-lg border border-line">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Référence</th>
                      <th>Date</th>
                      <th>Statut</th>
                      <th className="text-right">Total</th>
                      <th className="text-right">Reste</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detailQuery.data.history.map((row) => (
                      <tr key={row.id}>
                        <td className="font-medium">{row.reference}</td>
                        <td className="whitespace-nowrap text-ink-muted">{formatDate(row.created_at)}</td>
                        <td className="text-ink-muted">{row.status}</td>
                        <td className="tabular text-right">{formatXOF(row.total_xof)}</td>
                        <td className="tabular text-right">
                          {row.due_xof > 0 ? <span className="text-warning">{formatXOF(row.due_xof)}</span> : 'Soldé'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        ) : null}
      </Modal>
    </div>
  )
}
