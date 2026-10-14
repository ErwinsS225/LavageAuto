/**
 * Rapports : chiffres consolidés sur une période et export CSV.
 */
import { useState } from 'react'
import { Download, BarChart3 } from 'lucide-react'

import { Button, Card, CardHeader, ErrorState, Field, Input, PageHeader, SkeletonRows, StatCard, Table } from '@/components/ui'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { useReportOverview, reportExportUrl } from '@/features/reports/api'
import { addDays, formatCompact, formatNumber, formatXOF, startOfMonth, today } from '@/lib/format'

export default function ReportsPage() {
  usePageTitle('Rapports')
  const { stationId } = useStation()

  const [from, setFrom] = useState(addDays(today(), -29))
  const [to, setTo] = useState(today())

  const overview = useReportOverview({ from, to, station_id: stationId || undefined })

  return (
    <div className="space-y-5">
      <PageHeader
        title="Rapports"
        subtitle="Chiffres consolidés sur la période choisie, par service, laveur et méthode de paiement."
        actions={
          <a href={reportExportUrl({ from, to, station_id: stationId || undefined })} download>
            <Button variant="secondary" icon={<Download size={16} />}>
              Export CSV
            </Button>
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
          <Button size="sm" onClick={() => { setFrom(startOfMonth(today())); setTo(today()) }}>
            Ce mois
          </Button>
          <Button size="sm" onClick={() => { setFrom(addDays(today(), -6)); setTo(today()) }}>
            7 derniers jours
          </Button>
          <Button size="sm" onClick={() => { setFrom(today()); setTo(today()) }}>
            Aujourd'hui
          </Button>
        </div>
      </Card>

      {overview.isLoading ? (
        <SkeletonRows rows={6} />
      ) : overview.isError ? (
        <ErrorState message="Impossible de calculer les rapports." onRetry={() => void overview.refetch()} />
      ) : overview.data ? (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              label="Lavages"
              value={formatNumber(overview.data.totals.wash_count)}
              hint={`${formatNumber(overview.data.totals.canceled_count)} annulé(s)`}
              icon={<BarChart3 size={18} />}
            />
            <StatCard label="Chiffre d'affaires" value={formatXOF(overview.data.totals.revenue_xof)} tone="success" />
            <StatCard label="Remises" value={formatXOF(overview.data.totals.discount_xof)} tone="warning" />
            <StatCard
              label="Résultat net"
              value={formatXOF(overview.data.totals.result_xof)}
              hint={`Dépenses : ${formatXOF(overview.data.totals.expenses_xof)}`}
              tone={overview.data.totals.result_xof >= 0 ? 'success' : 'danger'}
            />
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader title="Top services" subtitle="Par chiffre d'affaires" />
              {overview.data.by_service.length === 0 ? (
                <p className="p-4 text-sm text-ink-muted">Aucune donnée sur la période.</p>
              ) : (
                <ul className="divide-y divide-line">
                  {overview.data.by_service.slice(0, 10).map((entry) => (
                    <li key={entry.service_name} className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm">
                      <span className="min-w-0 truncate text-ink">{entry.service_name}</span>
                      <span className="shrink-0 text-ink-muted">
                        {formatNumber(entry.quantity)} ×{' '}
                        <span className="font-medium text-ink">{formatCompact(entry.revenue_xof)} FCFA</span>
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Card>

            <Card>
              <CardHeader title="Par laveur" subtitle="Lavages terminés et CA généré" />
              {overview.data.by_employee.length === 0 ? (
                <p className="p-4 text-sm text-ink-muted">Aucun lavage attribué sur la période.</p>
              ) : (
                <Table
                  columns={['Laveur', 'Lavages', 'CA', 'Encaissé']}
                  rows={overview.data.by_employee.map((entry) => [
                    entry.employee_name,
                    formatNumber(entry.wash_count),
                    formatXOF(entry.revenue_xof),
                    formatXOF(entry.collected_xof),
                  ])}
                />
              )}
            </Card>

            <Card>
              <CardHeader title="Méthodes de paiement" />
              {overview.data.by_method.length === 0 ? (
                <p className="p-4 text-sm text-ink-muted">Aucun encaissement sur la période.</p>
              ) : (
                <ul className="divide-y divide-line">
                  {overview.data.by_method.map((entry) => (
                    <li key={entry.method} className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm">
                      <span className="text-ink">{entry.method}</span>
                      <span className="text-ink-muted">
                        {formatNumber(entry.count)} opérations ·{' '}
                        <span className="font-medium text-ink">{formatXOF(entry.amount_xof)}</span>
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Card>

            <Card>
              <CardHeader title="Dépenses par catégorie" />
              {overview.data.by_expense_category.length === 0 ? (
                <p className="p-4 text-sm text-ink-muted">Aucune dépense sur la période.</p>
              ) : (
                <ul className="divide-y divide-line">
                  {overview.data.by_expense_category.map((entry) => (
                    <li key={entry.category} className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm">
                      <span className="text-ink">{entry.label || entry.category}</span>
                      <span className="font-medium text-warning">{formatXOF(entry.amount_xof)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>

          <Card>
            <CardHeader title="Série journalière" subtitle="Lavages et chiffre d'affaires par jour" />
            <Table
              columns={['Jour', 'Lavages', 'CA', 'Encaissé', 'Dépenses', 'Résultat']}
              rows={overview.data.series.map((day) => [
                day.day,
                formatNumber(day.wash_count),
                formatXOF(day.revenue_xof),
                formatXOF(day.payments_xof),
                formatXOF(day.expenses_xof),
                formatXOF(day.result_xof),
              ])}
            />
          </Card>
        </>
      ) : null}
    </div>
  )
}
