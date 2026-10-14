/**
 * Caisse : session du jour (ouverture / clôture / réouverture) et
 * encaissements avec annulation possible.
 */
import { useState } from 'react'
import { Ban, Lock, LockOpen, Wallet } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { Button, Card, CardHeader, ConfirmDialog, EmptyState, ErrorState, Field, Input, Modal, PageHeader, SkeletonRows, Spinner, StatCard, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { api, ApiError } from '@/lib/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { formatDateTime, formatXOF } from '@/lib/format'
import { parseAmountInput } from '@/lib/format'
import { methodLabel } from '@/lib/constants'
import type { CashMovement, CashSession, PaymentRow } from '@/types'

/**
 * Nom du moyen tel que la station l'a déclaré (« Orange Money », « Wave »…).
 * Les encaissements antérieurs au référentiel n'ont que la famille : on la
 * nomme alors, plutôt que d'afficher la valeur technique.
 */
function nomMoyen(payment: PaymentRow): string {
  return payment.payment_method_name || methodLabel(payment.method)
}

export default function CashPage() {
  const toast = useToast()
  const { can } = useAuth()
  const { stationId } = useStation()
  usePageTitle('Caisse')

  const queryClient = useQueryClient()
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['cash-current'] })
    void queryClient.invalidateQueries({ queryKey: ['payments'] })
    void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
  }

  // L'API renvoie la session sous `session` (et `last_session` pour la
  // précédente) ; `open` et `is_open` disent seulement s'il y en a une. Lire
  // `open` comme la session ferait croire l'écran à une caisse fermée.
  const currentQuery = useQuery<{
    open: boolean
    is_open: boolean
    session: CashSession | null
    last_session: CashSession | null
    movements: CashMovement[]
  }>({
    queryKey: ['cash-current', stationId],
    queryFn: () => api.get(`/cash/current${stationId ? `?station_id=${stationId}` : ''}`),
  })
  const paymentsQuery = useQuery<{ items: PaymentRow[]; total: number }>({
    queryKey: ['payments', stationId],
    queryFn: () => api.get(`/cash/payments${stationId ? `?station_id=${stationId}` : ''}`),
  })

  const openMutation = useMutation({
    mutationFn: (input: { opening_float_xof: number; note?: string }) =>
      api.post<CashSession>('/cash/sessions', { station_id: stationId || undefined, ...input }),
    onSuccess: () => {
      toast.success('Caisse ouverte.')
      refresh()
    },
  })
  const closeMutation = useMutation({
    mutationFn: ({ id, counted, note }: { id: string; counted: number; note?: string }) =>
      api.post<CashSession>(`/cash/sessions/${id}/close`, { counted_cash_xof: counted, note }),
    onSuccess: () => {
      toast.success('Caisse clôturée.')
      refresh()
    },
  })
  const reopenMutation = useMutation({
    mutationFn: (id: string) => api.post<CashSession>(`/cash/sessions/${id}/reopen`),
    onSuccess: () => {
      toast.success('Session réouverte.')
      refresh()
    },
  })
  const voidMutation = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      api.post(`/cash/payments/${id}/void`, { reason }),
    onSuccess: () => {
      toast.success('Encaissement annulé.')
      refresh()
    },
  })

  const session = currentQuery.data?.session ?? null
  const last = currentQuery.data?.last_session ?? null
  const movements = currentQuery.data?.movements ?? []
  const payments = paymentsQuery.data?.items ?? []

  const [openModal, setOpenModal] = useState(false)
  const [float, setFloat] = useState(0)
  const [openNote, setOpenNote] = useState('')
  const [closeModal, setCloseModal] = useState(false)
  const [counted, setCounted] = useState(0)
  const [closeNote, setCloseNote] = useState('')
  const [voiding, setVoiding] = useState<PaymentRow | null>(null)
  const [voidReason, setVoidReason] = useState('')
  const [error, setError] = useState('')

  const canOpen = can('cash:open')
  const canReopen = can('cash:reopen')

  async function openSession() {
    setError('')
    try {
      await openMutation.mutateAsync({ opening_float_xof: float, note: openNote || undefined })
      setOpenModal(false)
      setFloat(0)
      setOpenNote('')
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "L'ouverture a échoué.")
    }
  }

  async function closeSession() {
    if (!session) return
    setError('')
    try {
      await closeMutation.mutateAsync({ id: session.id, counted, note: closeNote || undefined })
      setCloseModal(false)
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : 'La clôture a échoué.')
    }
  }

  async function voidPayment() {
    if (!voiding) return
    if (!voidReason.trim()) {
      setError('Motif obligatoire.')
      return
    }
    try {
      await voidMutation.mutateAsync({ id: voiding.id, reason: voidReason })
      setVoiding(null)
      setVoidReason('')
      setError('')
    } catch (caught) {
      setError(caught instanceof ApiError ? caught.message : "L'annulation a échoué.")
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Caisse"
        subtitle="La session du jour suit espèces et paiements mobiles. On clôture en comptant les espèces réelles."
        actions={
          session ? (
            canOpen || canReopen ? (
              <Button variant="primary" icon={<Lock size={16} />} onClick={() => { setCounted(session.expected_cash_xof); setCloseModal(true) }}>
                Clôturer la caisse
              </Button>
            ) : null
          ) : (
            canOpen ? (
              <Button variant="primary" icon={<LockOpen size={16} />} onClick={() => setOpenModal(true)}>
                Ouvrir la caisse
              </Button>
            ) : null
          )
        }
      />

      {currentQuery.isLoading ? (
        <SkeletonRows rows={6} />
      ) : currentQuery.isError ? (
        <ErrorState message="Impossible de charger la caisse." onRetry={() => void currentQuery.refetch()} />
      ) : session ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatCard label="Fond de caisse" value={formatXOF(session.opening_float_xof)} icon={<Wallet size={18} />} />
          <StatCard label="Espèces reçues" value={formatXOF(session.cash_in_xof)} tone="success" />
          <StatCard label="Mobile money" value={formatXOF(session.mobile_money_xof)} />
          <StatCard
            label="Espèces attendues"
            value={formatXOF(session.expected_cash_xof)}
            hint={`Ouverte à ${formatDateTime(session.opened_at)} par ${session.opened_by_name}`}
            tone="warning"
          />
        </div>
      ) : (
        <Card className="p-6 text-center">
          <p className="font-medium text-ink">Aucune session ouverte</p>
          <p className="mt-1 text-sm text-ink-muted">
            {last
              ? `Dernière clôture : ${last.closed_at ? formatDateTime(last.closed_at) : '—'} — fond de caisse précédent ${formatXOF(last.counted_cash_xof ?? 0)}.`
              : 'Ouvrez la caisse pour commencer à encaisser.'}
          </p>
          {canReopen && last ? (
            <Button className="mt-4" variant="secondary" onClick={() => void reopenMutation.mutateAsync(last.id)} loading={reopenMutation.isPending}>
              Réouvrir la dernière session
            </Button>
          ) : null}
        </Card>
      )}

      <Card>
        <CardHeader title="Encaissements récents" subtitle={`${paymentsQuery.data?.total ?? 0} au total`} />
        {paymentsQuery.isLoading ? (
          <Spinner />
        ) : payments.length === 0 ? (
          <EmptyState icon={<Wallet size={20} />} title="Aucun encaissement" description="Les paiements apparaissent ici dès la première fiche encaissée." />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Date</th>
                  <th>Fiche</th>
                  <th>Client</th>
                  <th>Méthode</th>
                  <th className="text-right">Montant</th>
                  <th>Par</th>
                  <th>État</th>
                  {can('payment:void') ? <th /> : null}
                </tr>
              </thead>
              <tbody>
                {payments.slice(0, 50).map((payment) => (
                  <tr key={payment.id} className={payment.status === 'voided' ? 'opacity-50' : ''}>
                    <td className="whitespace-nowrap text-ink-muted">{formatDateTime(payment.paid_at)}</td>
                    <td className="font-medium">{payment.wash_order_reference || '—'}</td>
                    <td className="text-ink-muted">{payment.customer_name || 'Client de passage'}</td>
                    <td className="text-ink-muted">{nomMoyen(payment)}</td>
                    <td className="tabular text-right font-medium">{formatXOF(payment.amount_xof)}</td>
                    <td className="text-ink-muted">{payment.received_by_name}</td>
                    <td>
                      <span className={`badge ${payment.status === 'valid' ? 'badge-success' : 'badge-danger'}`}>
                        {payment.status === 'valid' ? 'Valide' : 'Annulé'}
                      </span>
                    </td>
                    {can('payment:void') ? (
                      <td>
                        {payment.status === 'valid' ? (
                          <div className="flex justify-end">
                            <Button size="sm" variant="ghost" title="Annuler" onClick={() => setVoiding(payment)}>
                              <Ban size={15} />
                            </Button>
                          </div>
                        ) : null}
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Mouvements de la session */}
      {session && movements.length > 0 ? (
        <Card>
          <CardHeader title="Mouvements de la session" />
          <ul className="divide-y divide-line">
            {movements.map((movement) => (
              <li key={movement.kind + movement.id} className="flex items-center justify-between gap-3 px-4 py-2.5 text-sm">
                <div>
                  <p className="text-ink">{movement.label}</p>
                  <p className="text-xs text-ink-muted">
                    {formatDateTime(movement.at)} · {movement.actor}
                  </p>
                </div>
                <span className={`tabular font-medium ${movement.kind === 'payment' ? 'text-success' : 'text-warning'}`}>
                  {movement.kind === 'payment' ? '+' : '−'}
                  {formatXOF(movement.amount_xof)}
                </span>
              </li>
            ))}
          </ul>
        </Card>
      ) : null}

      {/* Ouverture */}
      <Modal
        open={openModal}
        title="Ouvrir la caisse"
        description="Le fond de caisse est l'argent disponible au démarrage du shift."
        onClose={() => setOpenModal(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setOpenModal(false)} disabled={openMutation.isPending}>
              Annuler
            </Button>
            <Button variant="primary" onClick={openSession} loading={openMutation.isPending}>
              Ouvrir
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Fond de caisse (FCFA)" required>
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={float === 0 ? '' : float}
                onChange={(event) => setFloat(parseAmountInput(event.target.value))}
                placeholder="10000"
              />
            )}
          </Field>
          <Field label="Note (optionnel)">
            {(id) => <Input id={id} value={openNote} onChange={(event) => setOpenNote(event.target.value)} />}
          </Field>
          {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
        </div>
      </Modal>

      {/* Clôture */}
      <Modal
        open={closeModal}
        title="Clôturer la caisse"
        description="Comptez les espèces réellement présentes : l'écart est calculé et signalé."
        onClose={() => setCloseModal(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setCloseModal(false)} disabled={closeMutation.isPending}>
              Annuler
            </Button>
            <Button variant="primary" onClick={closeSession} loading={closeMutation.isPending}>
              Clôturer
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field
            label="Espèces comptées (FCFA)"
            required
            hint={session ? `Théorique : ${formatXOF(session.expected_cash_xof)}` : undefined}
          >
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={counted === 0 ? '' : counted}
                onChange={(event) => setCounted(parseAmountInput(event.target.value))}
              />
            )}
          </Field>
          {session ? (
            <p className="text-sm">
              Écart :{' '}
              <span className={`font-semibold ${counted - session.expected_cash_xof === 0 ? 'text-success' : 'text-warning'}`}>
                {formatXOF(counted - session.expected_cash_xof)}
              </span>
            </p>
          ) : null}
          <Field label="Note de clôture (optionnel)">
            {(id) => <Textarea id={id} value={closeNote} onChange={(event) => setCloseNote(event.target.value)} />}
          </Field>
          {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
        </div>
      </Modal>

      {/* Annulation d'encaissement */}
      <ConfirmDialog
        open={Boolean(voiding)}
        title={`Annuler l'encaissement de ${voiding ? formatXOF(voiding.amount_xof) : ''} ?`}
        message="Le montant sera retiré de la caisse du jour. Un motif est enregistré dans le journal d'audit."
        confirmLabel="Annuler l'encaissement"
        tone="danger"
        loading={voidMutation.isPending}
        onCancel={() => { setVoiding(null); setError('') }}
        onConfirm={() => void voidPayment()}
      >
        <Field label="Motif" required>
          {(id) => <Input id={id} value={voidReason} onChange={(event) => setVoidReason(event.target.value)} placeholder="Ex : erreur de saisie" />}
        </Field>
        {error ? <p className="mt-2 rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
      </ConfirmDialog>
    </div>
  )
}
