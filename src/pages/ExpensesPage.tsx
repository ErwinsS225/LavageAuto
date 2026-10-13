/**
 * Dépenses : enregistrement des sorties d'argent et annulation avec motif.
 */
import { useState } from 'react'
import { Plus, Receipt, Ban } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, Input, Modal, PageHeader, Select, Spinner, StatCard, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { api, ApiError, query } from '@/lib/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { formatDateTime, formatXOF, parseAmountInput } from '@/lib/format'
import { EXPENSE_CATEGORIES } from '@/lib/constants'
import { gestureKey } from '@/lib/idempotence'
import type { Expense } from '@/types'

const METHODS = [
  { value: 'especes', label: 'Espèces' },
  { value: 'mobile_money', label: 'Mobile Money' },
  { value: 'carte', label: 'Carte' },
  { value: 'autre', label: 'Autre' },
]

export default function ExpensesPage() {
  const toast = useToast()
  const { can } = useAuth()
  const { stationId } = useStation()
  usePageTitle('Dépenses')
  const queryClient = useQueryClient()
  const refresh = () => void queryClient.invalidateQueries({ queryKey: ['expenses'] })

  const listQuery = useQuery<{ expenses: Expense[] }>({
    queryKey: ['expenses', stationId],
    queryFn: () => api.get(`/expenses${query({ station_id: stationId || undefined })}`),
  })

  const createMutation = useMutation({
    mutationFn: (input: Record<string, unknown>) => api.post('/expenses', input),
    onSuccess: () => {
      toast.success('Dépense enregistrée.')
      refresh()
    },
  })
  const voidMutation = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) => api.post(`/expenses/${id}/void`, { reason }),
    onSuccess: () => {
      toast.success('Dépense annulée.')
      refresh()
    },
  })

  const [formOpen, setFormOpen] = useState(false)
  const [form, setForm] = useState({ label: '', category: 'autre', amount: 0, method: 'especes', supplier: '', reference: '', note: '' })
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [voiding, setVoiding] = useState<Expense | null>(null)
  const [voidReason, setVoidReason] = useState('')

  const canManage = can('expense:create')
  const expenses = listQuery.data?.expenses ?? []
  const valid = expenses.filter((expense) => expense.status === 'valid')

  async function submit() {
    setError('')
    setFields({})
    if (!form.label.trim()) {
      setError('Le libellé est obligatoire.')
      return
    }
    if (form.amount <= 0) {
      setError('Le montant doit être positif.')
      return
    }
    try {
      await createMutation.mutateAsync({
        station_id: stationId || undefined,
        label: form.label,
        category: form.category,
        amount_xof: form.amount,
        method: form.method,
        supplier: form.supplier || undefined,
        reference: form.reference || undefined,
        note: form.note || undefined,
        request_id: gestureKey('expense', form.label, form.amount),
      })
      setFormOpen(false)
      setForm({ label: '', category: 'autre', amount: 0, method: 'especes', supplier: '', reference: '', note: '' })
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError("L'enregistrement a échoué.")
      }
    }
  }

  async function voidExpense() {
    if (!voiding || !voidReason.trim()) return
    try {
      await voidMutation.mutateAsync({ id: voiding.id, reason: voidReason })
      setVoiding(null)
      setVoidReason('')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'annulation a échoué.")
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Dépenses"
        subtitle="Eau, produits, salaires, transport : chaque sortie est tracée et affecte le résultat du jour."
        actions={
          canManage ? (
            <Button variant="primary" icon={<Plus size={16} />} onClick={() => setFormOpen(true)}>
              Enregistrer une dépense
            </Button>
          ) : null
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard label="Dépenses valides" value={formatXOF(valid.reduce((total, expense) => total + expense.amount_xof, 0))} icon={<Receipt size={18} />} tone="warning" />
        <StatCard label="Nombre d'opérations" value={String(valid.length)} />
        <StatCard
          label="Dont en espèces"
          value={formatXOF(valid.filter((expense) => expense.method === 'especes').reduce((total, expense) => total + expense.amount_xof, 0))}
          hint="Impacte le caisse espèces du jour"
        />
      </div>

      <Card>
        {listQuery.isLoading ? (
          <Spinner label="Chargement des dépenses…" />
        ) : listQuery.isError ? (
          <ErrorState message="Impossible de charger les dépenses." onRetry={() => void listQuery.refetch()} />
        ) : expenses.length === 0 ? (
          <EmptyState icon={<Receipt size={22} />} title="Aucune dépense" description="Enregistrez vos premières sorties d'argent pour suivre le résultat réel de la station." />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Date</th>
                  <th>Libellé</th>
                  <th>Catégorie</th>
                  <th>Méthode</th>
                  <th className="text-right">Montant</th>
                  <th>Par</th>
                  <th>État</th>
                  {can('expense:void') ? <th /> : null}
                </tr>
              </thead>
              <tbody>
                {expenses.map((expense) => (
                  <tr key={expense.id} className={expense.status === 'voided' ? 'opacity-50' : ''}>
                    <td className="whitespace-nowrap text-ink-muted">{formatDateTime(expense.spent_at || expense.created_at)}</td>
                    <td className="font-medium text-ink">
                      {expense.label}
                      {expense.supplier ? <span className="block text-xs text-ink-muted">{expense.supplier}</span> : null}
                    </td>
                    <td>
                      <span className="badge badge-neutral">{expense.category}</span>
                    </td>
                    <td className="text-ink-muted">{expense.method}</td>
                    <td className="tabular text-right font-medium">{formatXOF(expense.amount_xof)}</td>
                    <td className="text-ink-muted">{expense.recorded_by_name}</td>
                    <td>
                      <span className={`badge ${expense.status === 'valid' ? 'badge-success' : 'badge-danger'}`}>
                        {expense.status === 'valid' ? 'Valide' : 'Annulée'}
                      </span>
                      {expense.void_reason ? <span className="block text-xs text-ink-muted">{expense.void_reason}</span> : null}
                    </td>
                    {can('expense:void') ? (
                      <td>
                        {expense.status === 'valid' ? (
                          <div className="flex justify-end">
                            <Button size="sm" variant="ghost" title="Annuler" onClick={() => setVoiding(expense)}>
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

      <Modal
        open={formOpen}
        title="Enregistrer une dépense"
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
          <Field label="Libellé" required error={fields.label}>
            {(id) => (
              <Input
                id={id}
                value={form.label}
                onChange={(event) => setForm({ ...form, label: event.target.value })}
                placeholder="Ex : Achat de savon"
                invalid={Boolean(fields.label)}
              />
            )}
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Catégorie">
              {(id) => (
                <Select
                  id={id}
                  value={form.category}
                  onChange={(event) => setForm({ ...form, category: event.target.value })}
                  options={EXPENSE_CATEGORIES.map((category) => ({ value: category.id, label: category.label }))}
                />
              )}
            </Field>
            <Field label="Montant (FCFA)" required error={fields.amount_xof}>
              {(id) => (
                <Input
                  id={id}
                  inputMode="numeric"
                  value={form.amount === 0 ? '' : form.amount}
                  onChange={(event) => setForm({ ...form, amount: parseAmountInput(event.target.value) })}
                  invalid={Boolean(fields.amount_xof)}
                />
              )}
            </Field>
          </div>
          <Field label="Méthode de paiement">
            {(id) => (
              <Select id={id} value={form.method} onChange={(event) => setForm({ ...form, method: event.target.value })} options={METHODS} />
            )}
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Fournisseur">
              {(id) => <Input id={id} value={form.supplier} onChange={(event) => setForm({ ...form, supplier: event.target.value })} />}
            </Field>
            <Field label="Référence">
              {(id) => <Input id={id} value={form.reference} onChange={(event) => setForm({ ...form, reference: event.target.value })} />}
            </Field>
          </div>
          <Field label="Note">
            {(id) => <Textarea id={id} value={form.note} onChange={(event) => setForm({ ...form, note: event.target.value })} />}
          </Field>
          {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(voiding)}
        title={`Annuler « ${voiding?.label} » (${formatXOF(voiding?.amount_xof ?? 0)}) ?`}
        message="La dépense sera retirée du résultat du jour. Le motif est conservé dans le journal d'audit."
        confirmLabel="Annuler la dépense"
        tone="danger"
        loading={voidMutation.isPending}
        onCancel={() => setVoiding(null)}
        onConfirm={() => void voidExpense()}
      >
        <Field label="Motif" required>
          {(id) => <Input id={id} value={voidReason} onChange={(event) => setVoidReason(event.target.value)} placeholder="Ex : doublon de saisie" />}
        </Field>
      </ConfirmDialog>
    </div>
  )
}
