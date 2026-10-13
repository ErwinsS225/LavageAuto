/**
 * Plateforme — gestion des organisations : recherche, suspension,
 * changement de formule et détail.
 */
import { useState } from 'react'
import { Building2, Search } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { Button, Card, CardHeader, EmptyState, ErrorState, Input, Modal, PageHeader, Select, Spinner, Table } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { api, ApiError, query } from '@/lib/api'
import { usePageTitle } from '@/hooks/usePageTitle'
import { formatDate, formatNumber } from '@/lib/format'
import { subscriptionStatusLabel, subscriptionTone } from '@/lib/subscription'
import type { AdminOrganization } from '@/types'

export default function AdminOrganizationsPage() {
  const toast = useToast()
  usePageTitle('Organisations')
  const queryClient = useQueryClient()
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['admin-orgs'] })

  const [search, setSearch] = useState('')
  const [detailId, setDetailId] = useState<string | null>(null)

  const listQuery = useQuery<{ items: AdminOrganization[]; total: number }>({
    queryKey: ['admin-orgs', search],
    queryFn: () => api.get(`/platform/organizations${query({ search: search || undefined })}`),
  })
  const detailQuery = useQuery<AdminOrganization>({
    queryKey: ['admin-org', detailId],
    queryFn: () => api.get(`/platform/organizations/${detailId}`),
    enabled: Boolean(detailId),
  })
  const plansQuery = useQuery<{ items: Array<{ id: string; name: string; price_xof: number }> }>({
    queryKey: ['admin-plans'],
    queryFn: () => api.get('/platform/plans'),
    staleTime: 60_000,
  })

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api.patch(`/platform/organizations/${id}`, { status }),
    onSuccess: () => {
      toast.success('Statut mis à jour.')
      refresh()
    },
  })
  const planMutation = useMutation({
    mutationFn: ({ id, planId }: { id: string; planId: string }) =>
      api.post(`/platform/organizations/${id}/subscription`, { plan_id: planId }),
    onSuccess: () => {
      toast.success('Formule mise à jour.')
      refresh()
      void queryClient.invalidateQueries({ queryKey: ['admin-org'] })
    },
  })

  const organizations = listQuery.data?.items ?? []

  async function changePlan(organizationId: string, planId: string) {
    try {
      await planMutation.mutateAsync({ id: organizationId, planId })
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : 'Le changement a échoué.')
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader title="Organisations" subtitle="Toutes les entreprises clientes de la plateforme." />

      <Card>
        <div className="border-b border-line p-3">
          <div className="relative">
            <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-muted" />
            <Input className="pl-9" placeholder="Rechercher une organisation…" value={search} onChange={(event) => setSearch(event.target.value)} />
          </div>
        </div>

        {listQuery.isLoading ? (
          <Spinner label="Chargement…" />
        ) : listQuery.isError ? (
          <ErrorState message="Impossible de charger les organisations." onRetry={() => void listQuery.refetch()} />
        ) : organizations.length === 0 ? (
          <EmptyState icon={<Building2 size={22} />} title="Aucune organisation" />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Organisation</th>
                  <th>Ville</th>
                  <th>Formule</th>
                  <th>État</th>
                  <th className="text-right">Stations</th>
                  <th className="text-right">Employés</th>
                  <th>Inscrite le</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {organizations.map((organization) => (
                  <tr key={organization.id}>
                    <td>
                      <button type="button" className="cursor-pointer text-left font-medium text-ink hover:text-brand" onClick={() => setDetailId(organization.id)}>
                        {organization.name}
                        <span className="block text-xs font-normal text-ink-muted">{organization.owner_name || '—'}</span>
                      </button>
                    </td>
                    <td className="text-ink-muted">{organization.city || '—'}</td>
                    <td className="text-ink-muted">{organization.plan_name || '—'}</td>
                    <td>
                      <span className={`badge ${subscriptionTone(organization.subscription_status)}`}>
                        {subscriptionStatusLabel(organization.subscription_status)}
                      </span>
                    </td>
                    <td className="tabular text-right">{formatNumber(organization.station_count)}</td>
                    <td className="tabular text-right">{formatNumber(organization.employee_count)}</td>
                    <td className="whitespace-nowrap text-ink-muted">{formatDate(organization.created_at)}</td>
                    <td>
                      <Button
                        size="sm"
                        variant={organization.status === 'active' ? 'ghost' : 'primary'}
                        onClick={() =>
                          void statusMutation.mutateAsync({
                            id: organization.id,
                            status: organization.status === 'active' ? 'suspended' : 'active',
                          })
                        }
                        loading={statusMutation.isPending}
                      >
                        {organization.status === 'active' ? 'Suspendre' : 'Réactiver'}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Détail */}
      <Modal
        open={Boolean(detailId)}
        title={detailQuery.data ? detailQuery.data.name : 'Organisation'}
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
            <Table
              columns={['Champ', 'Valeur']}
              rows={[
                ['Propriétaire', `${detailQuery.data.owner_name || '—'} (${detailQuery.data.owner_email || '—'})`],
                ['Téléphone', detailQuery.data.phone || '—'],
                ['E-mail', detailQuery.data.email || '—'],
                ['Ville', detailQuery.data.city || '—'],
                ['Stations', formatNumber(detailQuery.data.station_count)],
                ['Employés', formatNumber(detailQuery.data.employee_count)],
                ['Lavages du mois', formatNumber(detailQuery.data.wash_orders_month)],
                ['Inscrite le', formatDate(detailQuery.data.created_at)],
              ]}
            />
            <Card>
              <CardHeader title="Changer de formule" />
              <div className="p-4">
                <Select
                  aria-label="Formule"
                  value={detailQuery.data.plan_id}
                  onChange={(event) => void changePlan(detailQuery.data!.id, event.target.value)}
                  options={(plansQuery.data?.items ?? []).map((plan) => ({ value: plan.id, label: `${plan.name} — ${plan.price_xof} FCFA` }))}
                />
              </div>
            </Card>
          </div>
        ) : null}
      </Modal>
    </div>
  )
}
