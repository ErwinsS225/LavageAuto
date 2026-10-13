/**
 * Plateforme — vue générale : organisations, revenus, répartition par formule.
 */
import { useQuery } from '@tanstack/react-query'
import { Building2, ShieldCheck, TrendingUp, Users } from 'lucide-react'

import { Card, CardHeader, ErrorState, PageHeader, Spinner, StatCard, Table } from '@/components/ui'
import { api } from '@/lib/api'
import { usePageTitle } from '@/hooks/usePageTitle'
import { formatDate, formatNumber, formatXOF } from '@/lib/format'
import { subscriptionStatusLabel, subscriptionTone } from '@/lib/subscription'
import type { AdminOverview } from '@/types'

export default function AdminOverviewPage() {
  usePageTitle('Plateforme')
  const query = useQuery<AdminOverview>({
    queryKey: ['admin-overview'],
    queryFn: () => api.get('/platform/overview'),
  })

  return (
    <div className="space-y-5">
      <PageHeader title="Vue générale" subtitle="État de la plateforme WashGuard." />

      {query.isLoading ? (
        <Spinner label="Chargement des indicateurs…" />
      ) : query.isError ? (
        <ErrorState message="Impossible de charger la vue générale." onRetry={() => void query.refetch()} />
      ) : query.data ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              label="Organisations"
              value={formatNumber(query.data.organizations)}
              hint={`${formatNumber(query.data.active_organizations)} active(s), ${formatNumber(query.data.suspended_organizations)} suspendue(s)`}
              icon={<Building2 size={18} />}
            />
            <StatCard label="Comptes utilisateurs" value={formatNumber(query.data.users)} icon={<Users size={18} />} />
            <StatCard
              label="Lavages du mois"
              value={formatNumber(query.data.wash_orders_month)}
              hint={`CA : ${formatXOF(query.data.revenue_month_xof)}`}
              icon={<TrendingUp size={18} />}
            />
            <StatCard label="Revenu mensuel (MRR)" value={formatXOF(query.data.mrr_xof)} tone="success" icon={<ShieldCheck size={18} />} />
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader title="Répartition par formule" />
              {query.data.plans.length === 0 ? (
                <p className="p-4 text-sm text-ink-muted">Aucun abonnement actif.</p>
              ) : (
                <Table
                  columns={['Formule', 'Prix', 'Organisations']}
                  rows={query.data.plans.map((plan) => [plan.plan_name, formatXOF(plan.price_xof), formatNumber(plan.count)])}
                />
              )}
            </Card>

            <Card>
              <CardHeader title="Dernières inscriptions" />
              {query.data.recent_organizations.length === 0 ? (
                <p className="p-4 text-sm text-ink-muted">Aucune organisation enregistrée.</p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Organisation</th>
                        <th>Ville</th>
                        <th>Formule</th>
                        <th>État</th>
                        <th>Inscrite le</th>
                      </tr>
                    </thead>
                    <tbody>
                      {query.data.recent_organizations.map((organization) => (
                        <tr key={organization.id}>
                          <td className="font-medium text-ink">{organization.name}</td>
                          <td className="text-ink-muted">{organization.city || '—'}</td>
                          <td className="text-ink-muted">{organization.plan_name || '—'}</td>
                          <td>
                            <span className={`badge ${subscriptionTone(organization.subscription_status)}`}>
                              {subscriptionStatusLabel(organization.subscription_status)}
                            </span>
                          </td>
                          <td className="whitespace-nowrap text-ink-muted">{formatDate(organization.created_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </Card>
          </div>
        </>
      ) : null}
    </div>
  )
}
