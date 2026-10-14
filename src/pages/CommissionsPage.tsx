/**
 * Commissions : barème par laveur, lavages terminés, montants dus.
 */
import { useState } from 'react'
import { HandCoins } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'

import { Button, Card, CardHeader, ErrorState, Field, Input, PageHeader, SkeletonRows, StatCard, Table } from '@/components/ui'
import { api, query } from '@/lib/api'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { formatNumber, formatXOF, startOfMonth, today } from '@/lib/format'
import { commissionRateLabel } from '@/lib/constants'
import type { CommissionReport } from '@/types'

export default function CommissionsPage() {
  usePageTitle('Commissions')
  const { stationId } = useStation()

  const [from, setFrom] = useState(startOfMonth(today()))
  const [to, setTo] = useState(today())

  const report = useQuery<CommissionReport>({
    queryKey: ['commissions', from, to, stationId],
    queryFn: () => api.get(`/commissions${query({ from, to, station_id: stationId || undefined })}`),
  })

  const data = report.data
  const exportUrl = `/api/v1/commissions/export?from=${from}&to=${to}${stationId ? `&station_id=${stationId}` : ''}`

  return (
    <div className="space-y-5">
      <PageHeader
        title="Commissions"
        subtitle="Ce que la station doit à chaque laveur sur la période, selon le barème en vigueur."
        actions={
          <a href={exportUrl} download>
            <Button variant="secondary">Export CSV</Button>
          </a>
        }
      />

      <Card className="p-4">
        <div className="flex flex-wrap items-end gap-3">
          <Field label="Du">
            {(id) => <Input id={id} type="date" value={from} onChange={(event) => setFrom(event.target.value)} />}
          </Field>
          <Field label="Au">
            {(id) => <Input id={id} type="date" value={to} onChange={(event) => setTo(event.target.value)} />}
          </Field>
        </div>
      </Card>

      {report.isLoading ? (
        <SkeletonRows rows={6} />
      ) : report.isError ? (
        <ErrorState message="Impossible de calculer les commissions." onRetry={() => void report.refetch()} />
      ) : data ? (
        <>
          <div className="grid gap-4 sm:grid-cols-3">
            <StatCard label="Laveurs concernés" value={formatNumber(data.totals.employee_count)} icon={<HandCoins size={18} />} />
            <StatCard
              label="Lavages terminés"
              value={formatNumber(data.totals.wash_count)}
              hint={`CA : ${formatXOF(data.totals.revenue_xof)}`}
            />
            <StatCard label="Commissions dues" value={formatXOF(data.totals.commission_xof)} tone="warning" />
          </div>

          {data.goal ? (
            <Card className="p-4">
              <p className="text-sm font-medium text-ink">
                Objectif du mois : {formatXOF(data.goal.achieved_xof)} / {formatXOF(data.goal.target_xof)} ({data.goal.percent}%)
              </p>
            </Card>
          ) : null}

          <Card>
            <CardHeader title="Détail par laveur" subtitle={`Période du ${data.period.from} au ${data.period.to}`} />
            {data.items.length === 0 ? (
              <p className="p-4 text-sm text-ink-muted">Aucun laveur avec activité sur la période.</p>
            ) : (
              <Table
                columns={['Laveur', 'Barème', 'Lavages', 'CA généré', 'Commission']}
                rows={data.items.map((item) => [
                  item.employee_name || 'Non attribué',
                  commissionRateLabel(item.mode, item.per_wash_xof, item.percent),
                  formatNumber(item.wash_count),
                  formatXOF(item.revenue_xof),
                  formatXOF(item.commission_xof),
                ])}
              />
            )}
          </Card>
        </>
      ) : null}
    </div>
  )
}
