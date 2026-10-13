import { useMemo, useState } from 'react'
import { AlertTriangle, Banknote, Car, Eye, Filter, Plus, Printer, XCircle } from 'lucide-react'

import { Button, Card, ConfirmDialog, EmptyState, ErrorState, Input, Modal, Money, PageHeader, Select, Spinner, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useEmployeeOptions } from '@/features/employees/api'
import PaymentDialog from '@/features/wash-orders/PaymentDialog'
import WashOrderForm from '@/features/wash-orders/WashOrderForm'
import { useAssignWashOrder, useCancelWashOrder, useWashOrder, useWashOrders, type WashOrderFilters } from '@/features/wash-orders/api'
import { useStation } from '@/hooks/useStation'
import { useAuth } from '@/hooks/useAuth'
import { ApiError } from '@/lib/api'
import { WASH_STATUSES, methodLabel, washStatusLabel, washStatusTone } from '@/lib/constants'
import { addDays, formatDateTime, formatTime, formatXOF, startOfMonth, startOfWeek, today } from '@/lib/format'
import type {WashStatus,  WashOrder } from '@/types'

type RangePreset = 'today' | 'yesterday' | 'week' | 'month' | 'custom'

const PRESETS: Array<{ id: RangePreset; label: string }> = [
  { id: 'today', label: "Aujourd'hui" },
  { id: 'yesterday', label: 'Hier' },
  { id: 'week', label: 'Cette semaine' },
  { id: 'month', label: 'Ce mois' },
  { id: 'custom', label: 'Autre période' },
]

/** Lavages : saisie rapide, suivi des encaissements et historique. */
export default function WashOrdersPage() {
  const toast = useToast()
  const { can, me } = useAuth()
  const { stationId } = useStation()

  const [preset, setPreset] = useState<RangePreset>('today')
  const [from, setFrom] = useState(today())
  const [to, setTo] = useState(today())
  const [status, setStatus] = useState('')
  const [search, setSearch] = useState('')
  const [openOnly, setOpenOnly] = useState(false)

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<WashOrder | null>(null)
  const [detailId, setDetailId] = useState<string | null>(null)
  const [paymentOrder, setPaymentOrder] = useState<WashOrder | null>(null)
  const [cancelOrder, setCancelOrder] = useState<WashOrder | null>(null)
  const [cancelReason, setCancelReason] = useState('')
  const [cancelError, setCancelError] = useState('')

  // Affectation rapide d'un laveur (responsabilité gérant)
  const [assignOrder, setAssignOrder] = useState<WashOrder | null>(null)
  const [assignEmployeeId, setAssignEmployeeId] = useState('')
  const [assigning, setAssigning] = useState(false)

  const employeesQuery = useEmployeeOptions({ station_id: stationId })
  const assignMutation = useAssignWashOrder()

  const range = useMemo(() => {
    const day = today()
    switch (preset) {
      case 'yesterday':
        return { from: addDays(day, -1), to: addDays(day, -1) }
      case 'week':
        return { from: startOfWeek(day), to: day }
      case 'month':
        return { from: startOfMonth(day), to: day }
      case 'custom':
        return { from, to }
      default:
        return { from: day, to: day }
    }
  }, [preset, from, to])

  const filters: WashOrderFilters = useMemo(
    () => ({
      ...range,
      station_id: stationId || undefined,
      status: (status as WashOrderFilters['status']) || undefined,
      search: search.trim() || undefined,
      open_only: openOnly || undefined,
      limit: 200,
    }),
    [range, stationId, status, search, openOnly],
  )

  const ordersQuery = useWashOrders(filters)
  const detailQuery = useWashOrder(detailId)
  const cancelMutation = useCancelWashOrder()

  const orders = ordersQuery.data?.items ?? []
  const totals = ordersQuery.data?.totals

  const canCreate = can('wash:create')
  const canPay = can('payment:create')
  const canCancel = can('wash:cancel')
  const canAssign = can('wash:assign') || me?.user.role === 'manager' || me?.user.role === 'owner'

  async function confirmAssign() {
    if (!assignOrder) return
    setAssigning(true)
    try {
      await assignMutation.mutateAsync({ id: assignOrder.id, employee_id: assignEmployeeId })
      toast.success(`Laveur affecté avec succès à la fiche ${assignOrder.reference}.`)
      setAssignOrder(null)
    } catch {
      toast.error("Impossible d'affecter le laveur.")
    } finally {
      setAssigning(false)
    }
  }

  async function confirmCancel() {
    if (!cancelOrder) return
    setCancelError('')
    try {
      await cancelMutation.mutateAsync({ id: cancelOrder.id, reason: cancelReason })
      toast.success(`Fiche ${cancelOrder.reference} annulée. L’opération est conservée dans l’historique.`)
      setCancelOrder(null)
      setCancelReason('')
    } catch (caught) {
      setCancelError(caught instanceof ApiError ? caught.message : "L'annulation a échoué.")
    }
  }

  if (!stationId) {
    return (
      <EmptyState
        title="Aucune station active"
        description="Créez une station dans les paramètres avant d’enregistrer des lavages."
      />
    )
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Lavages"
        subtitle="Enregistrez un lavage en quelques secondes, suivez les paiements et l’historique."
        actions={
          canCreate ? (
            <Button
              variant="primary"
              icon={<Plus size={16} />}
              onClick={() => {
                setEditing(null)
                setFormOpen(true)
              }}
            >
              Nouveau lavage
            </Button>
          ) : null
        }
      />

      <Card>
        <div className="flex flex-wrap items-end gap-3 p-4">
          <div className="flex flex-wrap gap-1.5">
            {PRESETS.map((item) => (
              <Button
                key={item.id}
                size="sm"
                variant={preset === item.id ? 'primary' : 'ghost'}
                onClick={() => setPreset(item.id)}
              >
                {item.label}
              </Button>
            ))}
          </div>

          {preset === 'custom' ? (
            <div className="flex items-end gap-2">
              <Input
                type="date"
                aria-label="Du"
                value={from}
                onChange={(event) => setFrom(event.target.value)}
                className="w-40"
              />
              <span className="pb-2 text-sm text-ink-muted">au</span>
              <Input
                type="date"
                aria-label="Au"
                value={to}
                onChange={(event) => setTo(event.target.value)}
                className="w-40"
              />
            </div>
          ) : null}

          <div className="min-w-44 flex-1">
            <Input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Référence, plaque ou client…"
            />
          </div>

          <Select
            aria-label="Statut"
            className="w-40"
            value={status}
            onChange={(event) => setStatus(event.target.value as WashStatus)}
            placeholder="Tous les statuts"
            options={WASH_STATUSES.map((item) => ({ value: item.id, label: item.label }))}
          />

          <label className="flex cursor-pointer items-center gap-2 pb-2 text-sm text-ink-muted">
            <input
              type="checkbox"
              checked={openOnly}
              onChange={(event) => setOpenOnly(event.target.checked)}
              className="h-4 w-4 accent-[var(--brand)]"
            />
            <Filter size={14} />
            Non soldés uniquement
          </label>
        </div>
      </Card>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Card className="p-4">
          <p className="stat-label">Chiffre d’affaires</p>
          <p className="stat-value mt-1">{formatXOF(totals?.revenue_xof ?? 0)}</p>
          <p className="mt-1 text-xs text-ink-muted">{ordersQuery.data?.total ?? 0} fiche(s) sur la période</p>
        </Card>
        <Card className="p-4">
          <p className="stat-label">Encaissé</p>
          <p className="stat-value mt-1 text-success">{formatXOF(totals?.paid_xof ?? 0)}</p>
          <p className="mt-1 text-xs text-ink-muted">paiements validés</p>
        </Card>
        <Card className="p-4">
          <p className="stat-label">Reste à encaisser</p>
          <p className={`stat-value mt-1 ${(totals?.due_xof ?? 0) > 0 ? 'text-warning' : ''}`}>
            {formatXOF(totals?.due_xof ?? 0)}
          </p>
          <p className="mt-1 text-xs text-ink-muted">fiches non soldées</p>
        </Card>
        <Card className="p-4">
          <p className="stat-label">Remises accordées</p>
          <p className="stat-value mt-1">{formatXOF(totals?.discount_xof ?? 0)}</p>
          <p className="mt-1 text-xs text-ink-muted">tracées dans le journal</p>
        </Card>
      </div>

      <Card>
        {ordersQuery.isLoading ? (
          <Spinner label="Chargement des lavages…" />
        ) : ordersQuery.isError ? (
          <ErrorState message="Impossible de charger les lavages." onRetry={() => void ordersQuery.refetch()} />
        ) : orders.length === 0 ? (
          <EmptyState
            icon={<Car size={22} />}
            title="Aucun lavage sur cette période"
            description="Changez la période ou enregistrez un nouveau lavage."
            action={
              canCreate ? (
                <Button variant="primary" icon={<Plus size={16} />} onClick={() => setFormOpen(true)}>
                  Nouveau lavage
                </Button>
              ) : null
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Référence</th>
                  <th>Date</th>
                  <th>Client / véhicule</th>
                  <th>Prestations</th>
                  <th>Employé</th>
                  <th>Statut</th>
                  <th className="text-right">Total</th>
                  <th className="text-right">Reste</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {orders.map((order) => (
                  <tr key={order.id}>
                    <td className="whitespace-nowrap font-medium text-ink">{order.reference}</td>
                    <td className="whitespace-nowrap text-ink-muted">
                      {formatDateTime(order.created_at).split(' à ')[0]}
                      <span className="ml-1 text-xs">{formatTime(order.created_at)}</span>
                    </td>
                    <td>
                      <span className="text-ink">{order.customer_name || 'Client de passage'}</span>
                      <span className="block text-xs text-ink-muted">
                        {order.vehicle_category_name || 'Catégorie non précisée'}
                        {order.plate ? ` · ${order.plate}` : ''}
                      </span>
                      {order.incident_note ? (
                        <span className="mt-1 inline-flex items-center gap-1 rounded bg-warning-soft px-1.5 py-0.5 text-[10px] font-semibold text-warning">
                          <AlertTriangle size={11} /> Incident consigné
                        </span>
                      ) : null}
                    </td>
                    <td>
                      {order.employee_name && order.employee_name !== 'Non attribué' ? (
                        <div className="flex items-center gap-1.5">
                          <span className="text-xs font-medium text-ink">{order.employee_name}</span>
                          {canAssign && order.status !== 'canceled' ? (
                            <button
                              type="button"
                              onClick={() => {
                                setAssignOrder(order)
                                setAssignEmployeeId(order.employee_id || '')
                              }}
                              className="text-[11px] font-medium text-brand hover:underline"
                              title="Changer le laveur affecté"
                            >
                              modifier
                            </button>
                          ) : null}
                        </div>
                      ) : (
                        <div className="flex items-center gap-1.5">
                          <span className="text-xs text-warning">Non attribué</span>
                          {canAssign && order.status !== 'canceled' ? (
                            <button
                              type="button"
                              onClick={() => {
                                setAssignOrder(order)
                                setAssignEmployeeId('')
                              }}
                              className="rounded bg-brand-soft px-1.5 py-0.5 text-[10px] font-semibold text-brand hover:bg-brand hover:text-brand-ink"
                            >
                              + Affecter
                            </button>
                          ) : null}
                        </div>
                      )}
                    </td>
                    <td className="tabular text-right">
                      {formatXOF(order.total_xof)}
                      {order.discount_xof > 0 ? (
                        <span className="block text-xs text-ink-muted">remise {formatXOF(order.discount_xof)}</span>
                      ) : null}
                    </td>
                    <td className="tabular text-right">
                      {order.due_xof > 0 ? <span className="text-warning">{formatXOF(order.due_xof)}</span> : <span className="text-success">Soldé</span>}
                    </td>
                    <td>
                      <div className="flex justify-end gap-1">
                        <Button size="sm" variant="ghost" onClick={() => setDetailId(order.id)} title="Voir le détail">
                          <Eye size={15} />
                        </Button>
                        {canPay && order.due_xof > 0 && order.status !== 'canceled' ? (
                          <Button size="sm" variant="primary" icon={<Banknote size={15} />} onClick={() => setPaymentOrder(order)}>
                            Encaisser
                          </Button>
                        ) : null}
                        {canCancel && order.status !== 'canceled' ? (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => {
                              setCancelOrder(order)
                              setCancelReason('')
                              setCancelError('')
                            }}
                            title="Annuler la fiche"
                          >
                            <XCircle size={15} />
                          </Button>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Saisie / modification */}
      <Modal
        open={formOpen}
        title={editing ? `Modifier la fiche ${editing.reference}` : 'Nouveau lavage'}
        description={editing ? 'Les prix déjà appliqués sont conservés.' : 'La référence et la date sont générées automatiquement.'}
        onClose={() => {
          setFormOpen(false)
          setEditing(null)
        }}
        size="lg"
      >
        <WashOrderForm
          stationId={stationId}
          order={editing ?? undefined}
          detail={editing ? detailQuery.data : undefined}
          onDone={() => {
            setFormOpen(false)
            setEditing(null)
          }}
          onCancel={() => {
            setFormOpen(false)
            setEditing(null)
          }}
        />
      </Modal>

      {/* Détail d'une fiche */}
      <Modal
        open={Boolean(detailId)}
        title={detailQuery.data ? `Fiche ${detailQuery.data.order.reference}` : 'Fiche de lavage'}
        onClose={() => setDetailId(null)}
        size="lg"
        footer={
          detailQuery.data ? (
            <>
              <Button
                variant="ghost"
                icon={<Printer size={15} />}
                onClick={() => window.print()}
              >
                Imprimer
              </Button>
              {can('wash:update') && detailQuery.data.order.status !== 'canceled' ? (
                <Button
                  onClick={() => {
                    setEditing(detailQuery.data?.order ?? null)
                    setFormOpen(true)
                    setDetailId(null)
                  }}
                >
                  Modifier
                </Button>
              ) : null}
              {canPay && (detailQuery.data.order.due_xof ?? 0) > 0 && detailQuery.data.order.status !== 'canceled' ? (
                <Button
                  variant="primary"
                  icon={<Banknote size={15} />}
                  onClick={() => {
                    setPaymentOrder(detailQuery.data?.order ?? null)
                    setDetailId(null)
                  }}
                >
                  Encaisser
                </Button>
              ) : null}
            </>
          ) : null
        }
      >
        {detailQuery.isLoading ? (
          <Spinner />
        ) : detailQuery.data ? (
          <div className="space-y-4 text-sm">
            <div className="grid gap-3 sm:grid-cols-2">
              <div>
                <p className="stat-label">Statut</p>
                <span className={`badge ${washStatusTone(detailQuery.data.order.status)}`}>
                  {washStatusLabel(detailQuery.data.order.status)}
                </span>
              </div>
              <div>
                <p className="stat-label">Enregistré le</p>
                <p>{formatDateTime(detailQuery.data.order.created_at)}</p>
              </div>
              <div>
                <p className="stat-label">Client</p>
                <p>{detailQuery.data.order.customer_name || 'Client de passage'}</p>
              </div>
              <div>
                <p className="stat-label">Véhicule</p>
                <p>
                  {detailQuery.data.order.vehicle_category_name || '—'}
                  {detailQuery.data.order.plate ? ` · ${detailQuery.data.order.plate}` : ''}
                </p>
              </div>
              <div>
                <p className="stat-label">Employé responsable</p>
                <p>{detailQuery.data.order.employee_name || '—'}</p>
              </div>
              <div>
                <p className="stat-label">Enregistré par</p>
                <p>{detailQuery.data.order.created_by_name || '—'}</p>
              </div>
            </div>

            {detailQuery.data.order.incident_note ? (
              <div className="rounded-xl border border-warning/50 bg-warning-soft p-3.5 text-xs text-warning">
                <div className="flex items-center gap-1.5 font-bold text-sm">
                  <AlertTriangle size={16} />
                  Constat / Incident signalé par le laveur
                </div>
                <p className="mt-1.5 text-ink leading-relaxed">{detailQuery.data.order.incident_note}</p>
                {detailQuery.data.order.incident_reporter_name ? (
                  <p className="mt-1 text-[11px] text-ink-muted">
                    Signalé par {detailQuery.data.order.incident_reporter_name}
                  </p>
                ) : null}
              </div>
            ) : null}

            <div>
              <p className="stat-label">Prestations</p>
              <ul className="divide-y divide-line rounded-lg border border-line">
                {(detailQuery.data.order.items ?? []).map((item, index) => (
                  <li key={item.id ?? index} className="flex items-center justify-between gap-3 px-3 py-2">
                    <span>
                      {item.service_name}
                      {item.quantity > 1 ? <span className="ml-1 text-ink-muted">× {item.quantity}</span> : null}
                    </span>
                    <span className="tabular">{formatXOF(item.line_total_xof)}</span>
                  </li>
                ))}
              </ul>
            </div>

            <div className="rounded-lg border border-line bg-surface-muted p-3">
              <div className="flex justify-between">
                <span className="text-ink-muted">Sous-total</span>
                <span className="tabular">{formatXOF(detailQuery.data.order.subtotal_xof)}</span>
              </div>
              {detailQuery.data.order.discount_xof > 0 ? (
                <div className="mt-1 flex justify-between">
                  <span className="text-ink-muted">
                    Remise{detailQuery.data.order.discount_reason ? ` (${detailQuery.data.order.discount_reason})` : ''}
                  </span>
                  <span className="tabular">−{formatXOF(detailQuery.data.order.discount_xof)}</span>
                </div>
              ) : null}
              <div className="mt-1 flex justify-between font-semibold">
                <span>Total</span>
                <Money value={detailQuery.data.order.total_xof} />
              </div>
              <div className="mt-1 flex justify-between">
                <span className="text-ink-muted">Payé</span>
                <Money value={detailQuery.data.order.paid_xof} />
              </div>
              <div className="mt-1 flex justify-between">
                <span className="text-ink-muted">Reste à payer</span>
                <Money value={detailQuery.data.order.due_xof} className="text-warning" />
              </div>
            </div>

            <div>
              <p className="stat-label">Encaissements</p>
              {detailQuery.data.payments.length === 0 ? (
                <p className="text-ink-muted">Aucun encaissement enregistré pour cette fiche.</p>
              ) : (
                <ul className="divide-y divide-line rounded-lg border border-line">
                  {detailQuery.data.payments.map((payment) => (
                    <li key={payment.id} className="flex items-center justify-between gap-3 px-3 py-2">
                      <span>
                        {methodLabel(payment.method)}
                        <span className="block text-xs text-ink-muted">
                          {formatDateTime(payment.paid_at)} · {payment.received_by_name || '—'}
                        </span>
                      </span>
                      <span className={`tabular ${payment.status === 'voided' ? 'text-danger line-through' : ''}`}>
                        {formatXOF(payment.amount_xof)}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {detailQuery.data.order.notes ? (
              <div>
                <p className="stat-label">Notes</p>
                <p className="text-ink-muted">{detailQuery.data.order.notes}</p>
              </div>
            ) : null}
            {detailQuery.data.order.status === 'canceled' && detailQuery.data.order.cancel_reason ? (
              <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-danger">
                Fiche annulée : {detailQuery.data.order.cancel_reason}
              </p>
            ) : null}
          </div>
        ) : (
          <ErrorState message="Fiche introuvable." />
        )}
      </Modal>

      {/* Encaissement */}
      {paymentOrder ? (
        <PaymentDialog
          open
          order={paymentOrder}
          onClose={() => {
            setPaymentOrder(null)
            if (detailId) void detailQuery.refetch()
          }}
        />
      ) : null}

      {/* Annulation */}
      <ConfirmDialog
        open={Boolean(cancelOrder)}
        title={cancelOrder ? `Annuler la fiche ${cancelOrder.reference} ?` : 'Annuler la fiche'}
        message={
          cancelOrder && cancelOrder.paid_xof > 0
            ? 'Des encaissements existent : ils seront annulés en conservant une trace dans le journal. Le motif est obligatoire.'
            : 'La fiche reste visible dans l’historique avec le motif d’annulation.'
        }
        confirmLabel="Annuler la fiche"
        cancelLabel="Retour"
        tone="danger"
        loading={cancelMutation.isPending}
        onCancel={() => setCancelOrder(null)}
        onConfirm={() => void confirmCancel()}
      >
        <Textarea
          value={cancelReason}
          onChange={(event) => setCancelReason(event.target.value)}
          placeholder="Motif : erreur de saisie, client parti, véhicule déjà lavé…"
        />
        {cancelError ? <p className="error-text">{cancelError}</p> : null}
      </ConfirmDialog>

      {/* Affectation d'un laveur (responsabilité gérant) */}
      <Modal
        open={Boolean(assignOrder)}
        title={`Affecter un laveur — Fiche ${assignOrder?.reference}`}
        description="Sélectionnez le laveur ou l'employé responsable de l'exécution de cette prestation."
        onClose={() => setAssignOrder(null)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setAssignOrder(null)} disabled={assigning}>
              Annuler
            </Button>
            <Button variant="primary" onClick={confirmAssign} loading={assigning}>
              Confirmer l'affectation
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="rounded-xl border border-line bg-surface-muted/50 p-3 text-xs">
            <p className="font-semibold text-ink">
              Véhicule : {assignOrder?.plate || assignOrder?.vehicle_category_name || 'Voiture'} · Client :{' '}
              {assignOrder?.customer_name || 'Client de passage'}
            </p>
            <p className="mt-1 text-ink-muted">Prestations : {assignOrder?.services_label || 'Lavage'}</p>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-semibold text-ink">Laveur ou équipier assigné :</label>
            <Select
              value={assignEmployeeId}
              onChange={(event) => setAssignEmployeeId(event.target.value)}
              options={[
                { value: '', label: '— Aucun (Non attribué) —' },
                ...(employeesQuery.data?.options ?? []).map((emp) => ({
                  value: emp.id,
                  label: `${emp.full_name} (${emp.role_label || emp.role})`,
                })),
              ]}
            />
            <p className="text-[11px] text-ink-muted">
              Le laveur sélectionné verra immédiatement ce véhicule sur son smartphone ou tablette atelier.
            </p>
          </div>
        </div>
      </Modal>
    </div>
  )
}
