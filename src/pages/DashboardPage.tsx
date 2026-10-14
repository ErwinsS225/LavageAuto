import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import {
  AlertTriangle,
  Banknote,
  BarChart3,
  Car,
  History,
  Info,
  LockKeyhole,
  LockOpen,
  Printer,
  Receipt,
  ShieldCheck,
  Sparkles,
  Target,
  TrendingUp,
  UserCheck,
  UserPlus,
  Wallet,
} from 'lucide-react'
import {
  BarChart,
  CartesianGrid,
  Label,
  Legend,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import {
  axisXProps,
  ChartGradients,
  axisYProps,
  ChartBar,
  ChartDonut,
  ChartLegend,
  ChartTooltip,
  DonutCell,
  gridProps,
} from '@/components/charts'

import {
  Button,
  Card,
  CardHeader,
  DefinitionNote,
  EmptyState,
  ErrorState,
  Progress,
  ScreenHero,
  Segmented,
  Spinner,
  StatCard,
} from '@/components/ui'
import { SetupPanel } from '@/features/onboarding/SetupPanel'
import { useDashboard } from '@/features/reports/api'
import { useWashOrders } from '@/features/wash-orders/api'
import { useAuth } from '@/hooks/useAuth'
import { useStation } from '@/hooks/useStation'
import { methodLabel, washStatusLabel, washStatusTone } from '@/lib/constants'
import {
  formatCompact,
  formatDateTime,
  formatDayShort,
  formatMonthKey,
  formatNumber,
  formatXOF,
  today,
} from '@/lib/format'
import type { DashboardData, Me } from '@/types'

/**
 * Tableau de bord adapté au profil connecté :
 * - Caissier : vue opérationnelle simplifiée (caisse du shift, file d'attente atelier, encaissements, pas de paie confidentielle)
 * - Gérant / Propriétaire : vue complète de supervision (rentabilité, CA, marges, performance équipe, audits)
 */
export default function DashboardPage() {
  const { me, can } = useAuth()
  const { stationId } = useStation()
  const dashboard = useDashboard({ station_id: stationId })

  if (!stationId) {
    return (
      <EmptyState
        title="Aucune station active"
        description="Créez une station dans les paramètres pour commencer à enregistrer des lavages."
        action={
          <Link to="/parametres">
            <Button variant="primary">Ouvrir les paramètres</Button>
          </Link>
        }
      />
    )
  }

  if (dashboard.isLoading) return <Spinner label="Chargement de la journée…" />
  if (dashboard.isError || !dashboard.data) {
    return <ErrorState message="Impossible de charger le tableau de bord." onRetry={() => void dashboard.refetch()} />
  }

  const isCashier = me?.user.role === 'cashier'

  return isCashier ? (
    <CashierDashboardView data={dashboard.data} stationId={stationId} me={me} />
  ) : (
    <ManagerDashboardView data={dashboard.data} me={me} can={can} />
  )
}

// ---------------------------------------------------------------------------
// 1. VUE CAISSIER : Accueil, guichet, caisse et file d'attente des véhicules
// ---------------------------------------------------------------------------

function CashierDashboardView({ data, stationId, me }: { data: DashboardData; stationId: string; me: Me | null }) {
  const todayStr = today()
  const ordersQuery = useWashOrders({ station_id: stationId, from: todayStr, to: todayStr, limit: 100 })
  const orders = ordersQuery.data?.items ?? []

  const [queueTab, setQueueTab] = useState<'all' | 'pending' | 'in_progress' | 'completed'>('all')

  const cash = data.cash
  const isCashOpen = Boolean(cash?.is_open)

  const pendingOrders = orders.filter((o) => o.status === 'pending')
  const inProgressOrders = orders.filter((o) => o.status === 'in_progress')
  const completedOrders = orders.filter((o) => o.status === 'completed')

  const displayedOrders =
    queueTab === 'pending'
      ? pendingOrders
      : queueTab === 'in_progress'
        ? inProgressOrders
        : queueTab === 'completed'
          ? completedOrders
          : orders

  return (
    <div className="space-y-5">
      {/* En-tête Caissier */}
      <ScreenHero
        icon={<Wallet size={22} />}
        title="Guichet & Caisse"
        badge={<span className="badge badge-brand">Poste caissier</span>}
        subtitle={
          <>
            Bonjour <span className="font-semibold text-ink">{me?.user.full_name}</span> · {me?.station?.name} ·{' '}
            {new Date(data.date).toLocaleDateString('fr-FR', { weekday: 'long', day: '2-digit', month: 'long' })}
          </>
        }
        actions={
          <>
            <Link to="/lavages">
              <Button variant="primary" icon={<Car size={16} />}>
                Nouveau lavage
              </Button>
            </Link>
            <Link to="/clients">
              <Button variant="ghost" icon={<UserPlus size={16} />}>
                Nouveau client
              </Button>
            </Link>
          </>
        }
      />

      {/* Alerte et état de la caisse du jour */}
      {!cash || !isCashOpen ? (
        <motion.div
          initial={{ opacity: 0, y: -6 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.32, ease: [0.22, 1, 0.36, 1] }}
          className="flex flex-wrap items-center justify-between gap-3 rounded-card border border-warning/35 bg-warning-soft p-4 text-warning shadow-e2"
        >
          <div className="flex items-start gap-3">
            <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-card border border-warning/35 bg-surface/60">
              <AlertTriangle size={18} />
            </span>
            <div>
              <p className="font-semibold text-ink">Votre caisse n'est pas encore ouverte ce matin</p>
              <p className="text-xs text-ink-muted">
                Pour assurer la traçabilité des espèces et éviter les écarts, ouvrez votre session avec le fond initial avant d'encaisser.
              </p>
            </div>
          </div>
          <Link to="/caisse">
            <Button variant="primary" icon={<LockOpen size={16} />}>
              Ouvrir ma caisse (fond initial)
            </Button>
          </Link>
        </motion.div>
      ) : (
        <Card className="border-success/30 bg-success-soft/20">
          <div className="p-4 sm:p-5">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line/60 pb-3">
              <div className="flex items-center gap-2 text-sm font-semibold text-success">
                <span className="flex h-2.5 w-2.5 rounded-full bg-success animate-pulse" />
                Caisse en service — Ouverte par {cash.opened_by_name}
              </div>
              <div className="flex items-center gap-2">
                <Link to="/caisse">
                  <Button size="sm" variant="default" icon={<LockKeyhole size={14} />}>
                    Clôturer ma caisse (comptage)
                  </Button>
                </Link>
                <Link to="/caisse">
                  <Button size="sm" variant="primary" icon={<Banknote size={14} />}>
                    Encaissement libre / acompte
                  </Button>
                </Link>
              </div>
            </div>

            <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
              <div className="card p-3">
                <p className="text-xs text-ink-muted">Fond initial</p>
                <p className="mt-1 text-lg font-bold tabular text-ink">{formatXOF(cash.opening_float_xof)}</p>
                <p className="text-[10px] text-ink-muted">à l'ouverture</p>
              </div>
              <div className="card p-3">
                <p className="text-xs text-ink-muted">Espèces perçues</p>
                <p className="mt-1 text-lg font-bold tabular text-success">+{formatXOF(cash.cash_in_xof)}</p>
                <p className="text-[10px] text-ink-muted">en caisse physique</p>
              </div>
              <div className="card p-3">
                <p className="text-xs text-ink-muted">Mobile Money</p>
                <p className="mt-1 text-lg font-bold tabular text-brand">{formatXOF(cash.mobile_money_xof)}</p>
                <p className="text-[10px] text-ink-muted">Wave / Orange / MTN</p>
              </div>
              <div className="rounded-xl border border-brand/40 bg-brand-soft/30 p-3">
                <p className="text-xs font-medium text-brand">Solde espèces attendu</p>
                <p className="mt-1 text-lg font-extrabold tabular text-brand">{formatXOF(cash.expected_cash_xof)}</p>
                <p className="text-[10px] text-ink-muted">à justifier à la clôture</p>
              </div>
            </div>
          </div>
        </Card>
      )}

      {/* Raccourcis d'action au guichet */}
      <motion.div
        variants={{ hidden: {}, visible: { transition: { staggerChildren: 0.06 } } }}
        initial="hidden"
        animate="visible"
        className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <motion.div
          variants={{
            hidden: { opacity: 0, y: 10 },
            visible: { opacity: 1, y: 0, transition: { duration: 0.32, ease: [0.22, 1, 0.36, 1] } },
          }}
        >
          <Link
            to="/lavages"
            className="card card-interactive group flex h-full flex-col items-center justify-center p-4 text-center no-underline"
          >
          <div className="btn-brand flex h-11 w-11 items-center justify-center rounded-card transition group-hover:scale-105 shadow-e2">
            <Car size={22} />
          </div>
          <span className="mt-2.5 font-semibold text-sm text-ink">Nouveau lavage</span>
          <span className="text-[11px] text-ink-muted">Saisie rapide & prestations</span>
          </Link>
        </motion.div>

        <motion.div
          variants={{
            hidden: { opacity: 0, y: 10 },
            visible: { opacity: 1, y: 0, transition: { duration: 0.32, ease: [0.22, 1, 0.36, 1] } },
          }}
        >
          <Link
            to="/clients"
            className="card card-interactive group flex h-full flex-col items-center justify-center p-4 text-center no-underline"
          >
          <div className="flex h-11 w-11 items-center justify-center rounded-card border border-line bg-surface-muted text-ink shadow-e1 transition group-hover:scale-105">
            <UserPlus size={22} />
          </div>
          <span className="mt-2.5 font-semibold text-sm text-ink">Client & véhicule</span>
          <span className="text-[11px] text-ink-muted">Enregistrer ou rechercher</span>
          </Link>
        </motion.div>

        <motion.div
          variants={{
            hidden: { opacity: 0, y: 10 },
            visible: { opacity: 1, y: 0, transition: { duration: 0.32, ease: [0.22, 1, 0.36, 1] } },
          }}
        >
          <Link
            to="/caisse"
            className="card card-interactive group flex h-full flex-col items-center justify-center p-4 text-center no-underline"
          >
          <div className="flex h-11 w-11 items-center justify-center rounded-card border border-line bg-surface-muted text-ink shadow-e1 transition group-hover:scale-105">
            <Wallet size={22} />
          </div>
          <span className="mt-2.5 font-semibold text-sm text-ink">Mouvements de caisse</span>
          <span className="text-[11px] text-ink-muted">Dépenses & comptage</span>
          </Link>
        </motion.div>

        <motion.div
          variants={{
            hidden: { opacity: 0, y: 10 },
            visible: { opacity: 1, y: 0, transition: { duration: 0.32, ease: [0.22, 1, 0.36, 1] } },
          }}
        >
          <Link
            to="/prestations"
            className="card card-interactive group flex h-full flex-col items-center justify-center p-4 text-center no-underline"
          >
          <div className="flex h-11 w-11 items-center justify-center rounded-card border border-line bg-surface-muted text-ink shadow-e1 transition group-hover:scale-105">
            <Sparkles size={22} />
          </div>
          <span className="mt-2.5 font-semibold text-sm text-ink">Tarifs du catalogue</span>
          <span className="text-[11px] text-ink-muted">Consulter les prix officiels</span>
          </Link>
        </motion.div>
      </motion.div>

      {/* File d'attente atelier en temps réel (pour renseigner les clients au comptoir) */}
      <Card>
        <CardHeader
          title="File d'attente atelier en direct"
          subtitle="Suivez l'avancement des véhicules pour renseigner instantanément les clients"
          actions={
            <Segmented
              layoutId="file-atelier"
              value={queueTab}
              onChange={setQueueTab}
              options={[
                { value: 'all', label: 'Tous', count: orders.length },
                { value: 'pending', label: 'En attente', count: pendingOrders.length },
                { value: 'in_progress', label: 'En cours', count: inProgressOrders.length },
                { value: 'completed', label: 'Prêts', count: completedOrders.length },
              ]}
            />
          }
        />

        {ordersQuery.isLoading ? (
          <Spinner label="Chargement des véhicules…" />
        ) : displayedOrders.length === 0 ? (
          <div className="p-8 text-center text-sm text-ink-muted">
            <Car size={32} className="mx-auto mb-2 text-ink-muted/50" />
            Aucun véhicule dans cette section pour l'instant.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Véhicule / Plaque</th>
                  <th>Client</th>
                  <th>Prestations</th>
                  <th>Laveur affecté</th>
                  <th>État avancement</th>
                  <th className="text-right">Montant</th>
                  <th className="text-right">Reste à payer</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {displayedOrders.map((ord) => (
                  <tr key={ord.id}>
                    <td>
                      <span className="font-semibold text-ink">{ord.plate || 'Sans plaque'}</span>
                      <span className="block text-xs text-ink-muted">{ord.vehicle_category_name || 'Voiture'}</span>
                    </td>
                    <td>
                      <span className="text-ink">{ord.customer_name || 'Client de passage'}</span>
                      <span className="block text-[11px] text-ink-muted">{ord.reference}</span>
                    </td>
                    <td className="max-w-44 truncate text-xs text-ink-muted" title={ord.services_label}>
                      {ord.services_label || 'Lavage'}
                    </td>
                    <td>
                      {ord.employee_name && ord.employee_name !== 'Non attribué' ? (
                        <span className="inline-flex items-center gap-1 text-xs font-medium text-ink">
                          <UserCheck size={13} className="text-brand" /> {ord.employee_name}
                        </span>
                      ) : (
                        <span className="text-xs text-warning">Non attribué</span>
                      )}
                    </td>
                    <td>
                      <span className={`badge ${washStatusTone(ord.status)}`}>
                        {washStatusLabel(ord.status)}
                      </span>
                    </td>
                    <td className="tabular text-right font-medium text-ink">
                      {formatXOF(ord.total_xof)}
                    </td>
                    <td className="tabular text-right">
                      {ord.due_xof > 0 ? (
                        <span className="font-semibold text-warning">{formatXOF(ord.due_xof)}</span>
                      ) : (
                        <span className="font-medium text-success">Réglé</span>
                      )}
                    </td>
                    <td>
                      <div className="flex justify-end gap-1">
                        {ord.due_xof > 0 && ord.status !== 'canceled' ? (
                          <Link to="/lavages">
                            <Button size="sm" variant="primary">
                              Encaisser
                            </Button>
                          </Link>
                        ) : (
                          <Link to="/lavages">
                            <Button size="sm" variant="ghost" icon={<Printer size={13} />}>
                              Reçu
                            </Button>
                          </Link>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Rappel des responsabilités et sécurité caisse */}
      <div className="flex items-start gap-3 panel p-4 text-xs text-ink-muted">
        <ShieldCheck size={18} className="mt-0.5 shrink-0 text-brand" />
        <div className="space-y-0.5 leading-relaxed">
          <p className="font-semibold text-ink">Sécurité opérationnelle & règles du poste caissier</p>
          <p>
            • Les tarifs des prestations sont fixés par le gérant et ne peuvent être altérés au guichet.
            <br />
            • Les remises accordées aux clients sont plafonnées à 1 000 FCFA. Au-delà, l'accord préalable du gérant est obligatoire.
            <br />
            • Toute annulation de fiche ou remboursement est tracé dans le journal d'audit de la station.
          </p>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// 2. VUE GÉRANT / PROPRIÉTAIRE : Supervision générale, rentabilité & audit
// ---------------------------------------------------------------------------

function ManagerDashboardView({
  data,
  me,
  can,
}: {
  data: DashboardData
  me: Me | null
  can: (permission: string) => boolean
}) {
  // Les montants du jour arrivent **ou non** : le serveur ne les envoie pas
  // quand la station n'a aucune fiche, ni quand le profil n'a pas accès aux
  // paiements. L'écran s'adapte au lieu d'afficher une colonne de zéros.
  const todayData = data.today ?? null
  const cash = data.cash
  const cashLisible = Object.prototype.hasOwnProperty.call(data, 'cash')
  const alerts = data.alerts ?? []
  const goal = data.goal ?? null
  const setup = data.setup
  // Avant la première fiche (ou avant l'activation), l'écran d'arrivée raconte
  // la station : ce qu'elle est, ce qu'elle accepte, ce qui reste à régler.
  const enMiseEnRoute = Boolean(setup) && (!setup!.activation.active || data.no_activity === true)

  const series = useMemo(
    () =>
      (data.series ?? []).map((day) => ({
        day: day.day,
        label: formatDayShort(day.day),
        ca: day.revenue_xof,
        encaisse: day.payments_xof,
        depenses: day.expenses_xof,
      })),
    [data.series],
  )

  const isOwner = me?.user.role === 'owner'

  return (
    <div className="space-y-5">
      {/* En-tête de gestion */}
      <ScreenHero
        icon={<TrendingUp size={22} />}
        title="Tableau de bord de gestion"
        badge={<span className="badge badge-brand">{isOwner ? 'Propriétaire' : 'Gérant opérationnel'}</span>}
        subtitle={
          <>
            Activité supervisée du{' '}
            {new Date(data.date).toLocaleDateString('fr-FR', {
              weekday: 'long',
              day: '2-digit',
              month: 'long',
              year: 'numeric',
            })}
            .
          </>
        }
        tone="neutral"
        actions={
          <>
            <Link to="/rapports">
              <Button variant="secondary" size="sm" icon={<BarChart3 size={15} />}>
                Rapports
              </Button>
            </Link>
            {can('settings:view') ? (
              <Link to="/parametres">
                <Button variant="ghost" size="sm" icon={<History size={15} />}>
                  Journal d'audit
                </Button>
              </Link>
            ) : null}
          </>
        }
      />
      {alerts.length > 0 ? (
        <div className="space-y-2">
          {alerts.map((alert, index) => {
            const ton =
              alert.level === 'danger'
                ? 'border-danger/30 bg-danger-soft text-danger'
                : alert.level === 'warning'
                  ? 'border-warning/30 bg-warning-soft text-warning'
                  : 'border-brand/30 bg-brand-soft text-brand'
            return (
              <motion.div
                key={alert.message}
                initial={{ opacity: 0, y: -4 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.3, ease: [0.22, 1, 0.36, 1], delay: Math.min(index * 0.05, 0.2) }}
                className={`flex items-start gap-2.5 rounded-field border px-3.5 py-2.5 text-sm shadow-e1 ${ton}`}
              >
                <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-current/30 bg-surface/40">
                  {alert.level === 'info' ? <Info size={12} /> : <AlertTriangle size={12} />}
                </span>
                <span className="leading-relaxed">{alert.message}</span>
              </motion.div>
            )
          })}
        </div>
      ) : null}

      {/* État de la station : identité, horaires, catalogue, prérequis.
          C'est le même bloc `setup` que le récapitulatif de mise en route —
          un seul calcul serveur, donc deux écrans cohérents. */}
      {setup && enMiseEnRoute ? <SetupPanel setup={setup} /> : null}
      {data.no_activity_note ? (
        <p className="rounded-field border border-line bg-surface-muted/60 px-4 py-3 text-sm text-ink-muted">
          {data.no_activity_note}
        </p>
      ) : null}
      {setup && !enMiseEnRoute ? <SetupPanel setup={setup} compact /> : null}

      {/* Indicateurs clés financiers */}
      {todayData ? (
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label="Chiffre d’affaires du jour"
          value={formatXOF(todayData.revenue_xof)}
          hint={`${formatNumber(todayData.wash_count)} lavage(s) · ${formatNumber(todayData.service_count)} prestation(s)`}
          tone="brand"
          icon={<TrendingUp size={16} />}
        />
        <StatCard
          label="Encaissements du jour"
          value={formatXOF(todayData.payments_xof)}
          hint={`Espèces : ${formatXOF(todayData.cash_xof)} · Mobile Money : ${formatXOF(todayData.mobile_money_xof)}`}
          icon={<Banknote size={16} />}
        />
        <StatCard
          label="Dépenses du jour"
          value={formatXOF(todayData.expenses_xof)}
          hint={`dont espèces : ${formatXOF(todayData.expenses_cash_xof)}`}
          icon={<Receipt size={16} />}
        />
        <StatCard
          label="Résultat simplifié"
          value={formatXOF(todayData.result_xof)}
          hint={`Chiffre d’affaires − dépenses`}
          tone={todayData.result_xof >= 0 ? 'success' : 'danger'}
        />
      </div>
      ) : null}

      {goal ? (
        <Card>
          <div className="flex flex-wrap items-end gap-4 p-4">
            <div className="flex items-center gap-2 text-sm font-medium text-ink">
              <Target size={16} className="text-brand" />
              Objectif du mois — {formatMonthKey(goal.month)}
              {goal.reached ? <span className="badge badge-success">Atteint</span> : null}
            </div>
            <div className="min-w-40 flex-1">
              <Progress
                value={goal.percent}
                tone={goal.reached ? 'success' : 'brand'}
                label={
                  goal.reached
                    ? `Objectif de ${formatXOF(goal.target_xof)} dépassé`
                    : `Reste ${formatXOF(goal.remaining_xof)} en ${goal.days_left} jour(s)`
                }
                showValue
              />
            </div>
            <p className="text-sm text-ink-muted">
              <span className="tabular font-medium text-ink">{formatXOF(goal.achieved_xof)}</span> sur{' '}
              <span className="tabular text-ink">{formatXOF(goal.target_xof)}</span>
            </p>
            {can('commission:view') ? (
              <Link to="/commissions">
                <Button size="sm" variant="ghost">
                  Voir les commissions
                </Button>
              </Link>
            ) : null}
          </div>
        </Card>
      ) : null}

      <div className="grid gap-4 lg:grid-cols-3">
        {data.series ? (
        <Card className="lg:col-span-2">
          <CardHeader
            title="Activité des 7 derniers jours"
            subtitle="Chiffre d’affaires, encaissements et dépenses (en FCFA)"
          />
          <div className="h-72 p-3">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={series} margin={{ top: 8, right: 8, bottom: 0, left: 0 }} barGap={4}>
                <defs>
                  <ChartGradients ids={['gradCa', 'gradEncaisse', 'gradDepenses']} />
                </defs>
                <CartesianGrid {...gridProps} />
                <XAxis dataKey="label" {...axisXProps} />
                <YAxis {...axisYProps} tickFormatter={(value) => formatCompact(Number(value))} />
                <Tooltip
                  content={<ChartTooltip formatValue={(value) => formatXOF(value)} />}
                  cursor={{ fill: 'color-mix(in oklab, var(--brand) 8%, transparent)' }}
                  animationDuration={140}
                />
                <Legend content={<ChartLegend />} />
                <ChartBar dataKey="ca" name="Chiffre d’affaires" gradientId="gradCa" />
                <ChartBar dataKey="encaisse" name="Encaissements" gradientId="gradEncaisse" delay={0.1} />
                <ChartBar dataKey="depenses" name="Dépenses" gradientId="gradDepenses" delay={0.2} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card>
        ) : null}

        {cashLisible ? (
        <Card>
          <CardHeader title="Contrôle de Caisse" subtitle={cash?.is_open ? 'En cours de service' : 'Fermée'} />
          <div className="space-y-3 p-4">
            {cash?.is_open ? (
              <>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-ink-muted">Fond de caisse</span>
                  <span className="tabular text-ink">{formatXOF(cash.opening_float_xof)}</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-ink-muted">Espèces reçues</span>
                  <span className="tabular text-ink">{formatXOF(cash.cash_in_xof)}</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-ink-muted">Mobile Money</span>
                  <span className="tabular text-ink">{formatXOF(cash.mobile_money_xof)}</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span className="text-ink-muted">Dépenses (espèces)</span>
                  <span className="tabular text-ink">−{formatXOF(cash.expenses_xof)}</span>
                </div>
                <div className="flex items-center justify-between border-t border-line pt-2 text-sm font-medium">
                  <span className="text-ink">Solde théorique</span>
                  <span className="tabular text-brand">{formatXOF(cash.expected_cash_xof)}</span>
                </div>
                <p className="text-xs text-ink-muted">
                  Ouverte par {cash.opened_by_name} à {formatDateTime(cash.opened_at)}
                </p>
                <Link to="/caisse" className="block">
                  <Button className="w-full" size="sm">
                    Superviser la caisse
                  </Button>
                </Link>
              </>
            ) : (
              <>
                <p className="text-sm text-ink-muted">
                  Aucune caisse ouverte actuellement.
                </p>
                <Link to="/caisse" className="block">
                  <Button variant="primary" className="w-full" size="sm" icon={<Wallet size={15} />}>
                    Ouvrir la caisse
                  </Button>
                </Link>
              </>
            )}
            {todayData ? (
              <div className="flex items-center justify-between rounded-lg bg-surface-muted px-3 py-2 text-xs text-ink-muted">
                <span>Reste à encaisser aujourd'hui</span>
                <span className="tabular font-medium text-ink">{formatXOF(todayData.due_xof)}</span>
              </div>
            ) : null}
          </div>
        </Card>
        ) : null}
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        {data.payment_methods ? (
        <Card>
          <CardHeader title="Moyens de paiement" subtitle="Répartition du jour" />
          <div className="h-64 p-3">
            {(data.payment_methods ?? []).length === 0 ? (
              <EmptyState title="Aucun encaissement" description="Les encaissements du jour apparaîtront ici." />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <ChartDonut
                    data={(data.payment_methods ?? []).map((item) => ({
                      name: methodLabel(item.method),
                      value: item.amount_xof,
                    }))}
                    dataKey="value"
                    nameKey="name"
                    innerRadius="58%"
                    outerRadius="86%"
                  >
                    {(data.payment_methods ?? []).map((item, index) => (
                      <DonutCell key={item.method} index={index} />
                    ))}
                    <Label
                      content={({ viewBox }) => {
                        const box = viewBox as { cx?: number; cy?: number } | undefined
                        if (!box?.cx || !box?.cy) return null
                        return (
                          <text x={box.cx} y={box.cy} textAnchor="middle" dominantBaseline="middle">
                            <tspan
                              x={box.cx}
                              y={box.cy}
                              style={{ fill: 'var(--ink)', fontSize: 17, fontWeight: 600 }}
                              className="tabular"
                            >
                              {formatCompact(todayData?.payments_xof ?? 0)}
                            </tspan>
                            <tspan x={box.cx} y={box.cy + 18} style={{ fill: 'var(--ink-muted)', fontSize: 11 }}>
                              encaissé
                            </tspan>
                          </text>
                        )
                      }}
                    />
                  </ChartDonut>
                  <Tooltip
                    content={<ChartTooltip formatValue={(value) => formatXOF(value)} />}
                    animationDuration={140}
                  />
                  <Legend content={<ChartLegend />} />
                </PieChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>
        ) : null}

        {data.top_services ? (
        <Card>
          <CardHeader title="Prestations les plus vendues" subtitle="Aujourd’hui" />
          <div className="p-4">
            {(data.top_services ?? []).length === 0 ? (
              <p className="text-sm text-ink-muted">Aucune prestation enregistrée aujourd’hui.</p>
            ) : (
              <ul className="space-y-2.5">
                {(data.top_services ?? []).map((service) => (
                  <li key={service.service_name} className="flex items-center justify-between gap-3 text-sm">
                    <span className="flex min-w-0 items-center gap-2">
                      <Sparkles size={15} className="shrink-0 text-brand" />
                      <span className="truncate text-ink">{service.service_name}</span>
                    </span>
                    <span className="whitespace-nowrap text-ink-muted">
                      {formatNumber(service.quantity)} × <span className="tabular text-ink">{formatXOF(service.revenue_xof)}</span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </Card>
        ) : null}

        {data.employees ? (
        <Card>
          <CardHeader
            title="Rendement de l’équipe"
            subtitle="Lavages réalisés & CA généré"
            actions={
              <Link to="/employes">
                <Button size="sm" variant="ghost">
                  Équipe
                </Button>
              </Link>
            }
          />
          <div className="p-4">
            {(data.employees ?? []).length === 0 ? (
              <p className="text-sm text-ink-muted">Aucune activité enregistrée aujourd’hui.</p>
            ) : (
              <ul className="space-y-3">
                {(data.employees ?? []).map((employee) => (
                  <li key={employee.employee_id || employee.employee_name} className="rounded-lg border border-line p-2.5 text-sm">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-semibold text-ink">{employee.employee_name || 'Non attribué'}</span>
                      <span className="badge badge-brand tabular">{formatNumber(employee.wash_count)} lavage(s)</span>
                    </div>
                    <div className="mt-1.5 flex items-center justify-between text-xs text-ink-muted">
                      <span>CA : <strong className="tabular text-ink">{formatXOF(employee.revenue_xof)}</strong></span>
                      <span>Encaissé : <strong className="tabular text-success">{formatXOF(employee.collected_xof)}</strong></span>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </Card>
        ) : null}
      </div>

      <DefinitionNote definitions={data.definitions} />
    </div>
  )
}
