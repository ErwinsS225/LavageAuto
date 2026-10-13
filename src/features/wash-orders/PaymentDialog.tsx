/**
 * Dialogue d'encaissement d'une fiche de lavage.
 * Montant pré-rempli au reste dû ; clôture automatique côté serveur si soldé.
 */
import { useEffect, useState } from 'react'
import { Button, Field, Input, Modal, Select } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useAddWashOrderPayment } from '@/features/wash-orders/api'
import { PAYMENT_METHOD_OPTIONS } from '@/lib/constants'
import { gestureKey } from '@/lib/idempotence'
import { parseAmountInput } from '@/lib/format'
import type { PaymentMethod, WashOrder } from '@/types'

export default function PaymentDialog({
  open,
  order,
  onClose,
}: {
  open: boolean
  order: WashOrder
  onClose: () => void
}) {
  const toast = useToast()
  const mutation = useAddWashOrderPayment()

  const [method, setMethod] = useState<PaymentMethod>('especes')
  const [amount, setAmount] = useState(order.due_xof)
  const [reference, setReference] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    setAmount(order.due_xof)
    setMethod('especes')
    setReference('')
    setNote('')
    setError('')
  }, [order])

  async function submit() {
    setError('')
    if (amount <= 0) {
      setError('Saisissez un montant positif.')
      return
    }
    try {
      await mutation.mutateAsync({
        id: order.id,
        amount_xof: amount,
        method,
        reference: reference || undefined,
        note: note || undefined,
        request_id: gestureKey('payment', order.id, amount, method),
      })
      toast.success('Encaissement enregistré.')
      onClose()
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "L'encaissement a échoué.")
    }
  }

  return (
    <Modal
      open={open}
      title={`Encaisser ${order.reference}`}
      description={`Reste dû : ${order.due_xof} FCFA`}
      onClose={onClose}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={mutation.isPending}>
            Annuler
          </Button>
          <Button variant="primary" onClick={submit} loading={mutation.isPending}>
            Encaisser
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Field label="Méthode" required>
          {(id) => (
            <Select
              id={id}
              value={method}
              onChange={(event) => setMethod(event.target.value as PaymentMethod)}
              options={PAYMENT_METHOD_OPTIONS}
            />
          )}
        </Field>
        <Field label="Montant (FCFA)" required>
          {(id) => (
            <Input
              id={id}
              inputMode="numeric"
              value={amount === 0 ? '' : amount}
              onChange={(event) => setAmount(parseAmountInput(event.target.value))}
            />
          )}
        </Field>
        <Field label="Référence" hint="Ex : numéro de transaction Mobile Money.">
          {(id) => <Input id={id} value={reference} onChange={(event) => setReference(event.target.value)} />}
        </Field>
        <Field label="Note">
          {(id) => <Input id={id} value={note} onChange={(event) => setNote(event.target.value)} />}
        </Field>
        {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
      </div>
    </Modal>
  )
}
