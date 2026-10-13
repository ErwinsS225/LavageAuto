/**
 * Abonnement : formule en cours, usage et passage à une formule supérieure.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { Button, Card, CardHeader, PageHeader, Spinner, StatCard } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { api } from '@/lib/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { formatNumber, formatXOF } from '@/lib/format'
import { subscriptionStatusLabel, subscriptionTone } from '@/lib/subscription'
import type { Plan } from '@/types'

export default function SubscriptionPage() {
  const toast = useToast()
  const { me } = useAuth()
  usePageTitle('Abonnement')
  const queryClient = useQueryClient()

  const subscription = me?.subscription ?? null
  const plansQuery = useQuery<{ items: Plan[] }>({
    queryKey: ['plans'],
    queryFn: () => api.get('/plans'),
  })

  const requestMutation = useMutation({
    mutationFn: (planId: string) => api.post('/organizations/subscription/request', { plan_id: planId }),
    onSuccess: () => {
      toast.success('Demande enregistrée : l’équipe WashGuard vous contactera.')
      void queryClient.invalidateQueries({ queryKey: ['me'] })
    },
  })

  if (!me) return <Spinner />
  if (!subscription) {
    return (
      <div className="space-y-5">
        <PageHeader title="Abonnement" subtitle="Aucune formule active pour l'instant." />
        <p className="text-sm text-ink-muted">Contactez l'équipe WashGuard pour activer votre espace.</p>
      </div>
    )
  }

  const usage = subscription.usage

  return (
    <div className="space-y-5">
      <PageHeader
        title="Abonnement"
        subtitle="Votre formule, votre consommation et les limites associées."
      />

      <Card>
        <CardHeader
          title={subscription.plan_name || 'Formule en cours'}
          subtitle={subscription.plan_description}
          actions={
            <span className={`badge ${subscriptionTone(subscription.status)}`}>
              {subscriptionStatusLabel(subscription.status)}
            </span>
          }
        />
        <div className="grid gap-4 p-4 sm:grid-cols-4">
          <StatCard label="Prix" value={formatXOF(subscription.price_xof)} hint={`par ${subscription.billing_period || 'mois'}`} />
          <StatCard
            label="Stations"
            value={`${formatNumber(usage.stations)} / ${subscription.max_stations || '∞'}`}
          />
          <StatCard
            label="Employés"
            value={`${formatNumber(usage.employees)} / ${subscription.max_employees || '∞'}`}
          />
          <StatCard
            label="Lavages ce mois"
            value={`${formatNumber(usage.wash_orders)} / ${subscription.max_wash_orders_month || '∞'}`}
          />
        </div>
      </Card>

      {subscription.blocked_reason ? (
        <Card className="border-warning/40 bg-warning-soft/50 p-4 text-sm text-warning">{subscription.blocked_reason}</Card>
      ) : null}

      <Card>
        <CardHeader title="Formules disponibles" subtitle="Changer de formule ouvre une demande de changement." />
        {plansQuery.isLoading ? (
          <Spinner />
        ) : (
          <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">
            {(plansQuery.data?.items ?? []).map((plan) => {
              const currentPlan = plan.id === subscription.plan_id
              return (
                <div
                  key={plan.id}
                  className={`rounded-xl border p-4 ${currentPlan ? 'border-brand bg-brand-soft/30' : 'border-line'}`}
                >
                  <div className="flex items-center justify-between">
                    <p className="font-semibold text-ink">{plan.name}</p>
                    {currentPlan ? <span className="badge badge-brand">Formule actuelle</span> : null}
                  </div>
                  <p className="mt-1 text-lg font-bold text-ink">
                    {formatXOF(plan.price_xof)}
                    <span className="text-xs font-normal text-ink-muted"> / {plan.billing_period || 'mois'}</span>
                  </p>
                  <p className="mt-1 text-xs text-ink-muted">{plan.description}</p>
                  <ul className="mt-2 space-y-0.5 text-xs text-ink-muted">
                    <li>{plan.max_stations || '∞'} station(s)</li>
                    <li>{plan.max_employees || '∞'} employé(s)</li>
                    <li>{plan.max_wash_orders_month || '∞'} lavages / mois</li>
                  </ul>
                  {!currentPlan ? (
                    <Button
                      className="mt-3 w-full"
                      variant="secondary"
                      size="sm"
                      onClick={() => void requestMutation.mutateAsync(plan.id)}
                      loading={requestMutation.isPending}
                    >
                      Demander cette formule
                    </Button>
                  ) : null}
                </div>
              )
            })}
          </div>
        )}
      </Card>
    </div>
  )
}
