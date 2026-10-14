/**
 * Dialogue d'encaissement d'une fiche de lavage.
 *
 * Le moyen de paiement proposé est celui que **la station a déclaré** (Orange
 * Money, Wave, carte…) : c'est le serveur qui en déduit la famille comptable,
 * l'exigence de référence et la permission du différé. Le montant est
 * pré-rempli au reste dû ; la fiche se clôt quand le solde est couvert par une
 * somme réellement reçue.
 *
 * Quand l'encaissement solde la fiche, le reçu émis s'affiche dans la foulée :
 * le client le reçoit au comptoir, sans second passage par un autre écran.
 */
import { useEffect, useMemo, useState } from 'react'

import { Badge, Button, Field, Input, Modal, Select, Spinner } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { ReceiptView, recuEnTexte } from '@/features/payments/ReceiptView'
import { usePaymentMethods } from '@/features/payments/api'
import { useAddWashOrderPayment } from '@/features/wash-orders/api'
import { useStation } from '@/hooks/useStation'
import { api, ApiError } from '@/lib/api'
import { gestureKey } from '@/lib/idempotence'
import { parseAmountInput } from '@/lib/format'
import type { Receipt, WashOrder } from '@/types'

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
  const { stationId } = useStation()
  const mutation = useAddWashOrderPayment()
  const moyens = usePaymentMethods(stationId, true, open)

  const [methodId, setMethodId] = useState('')
  const [amount, setAmount] = useState(order.due_xof)
  const [reference, setReference] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const [recu, setRecu] = useState<Receipt | null>(null)

  const actifs = moyens.data?.items ?? []
  const choisi = useMemo(() => actifs.find((moyen) => moyen.id === methodId), [actifs, methodId])

  useEffect(() => {
    setAmount(order.due_xof)
    setReference('')
    setNote('')
    setError('')
    setRecu(null)
  }, [order])

  // Le moyen par défaut est le premier actif — les espèces, dans l'ordre du
  // référentiel : le geste le plus courant au comptoir.
  useEffect(() => {
    if (methodId || actifs.length === 0) return
    setMethodId(actifs[0].id)
  }, [actifs, methodId])

  async function submit() {
    setError('')
    if (!choisi) {
      setError("Aucun moyen de paiement n'est actif sur cette station.")
      return
    }
    if (amount <= 0) {
      setError('Saisissez un montant positif.')
      return
    }
    if (choisi.requires_reference && !reference.trim()) {
      setError(`« ${choisi.name} » exige une référence (numéro de transaction, bordereau…).`)
      return
    }
    try {
      const reponse = await mutation.mutateAsync({
        id: order.id,
        amount_xof: amount,
        payment_method_id: choisi.id,
        reference: reference || undefined,
        note: note || undefined,
        request_id: gestureKey('payment', order.id, amount, choisi.id),
      })
      if (reponse.order.status === 'completed') {
        // Le serveur vient d'attribuer le numéro de reçu : on l'affiche tel
        // quel, sans le reconstituer côté navigateur.
        const charge = await api.get<{ receipt: Receipt }>(`/wash-orders/${order.id}/receipt`)
        setRecu(charge.receipt)
        toast.success(`Fiche soldée. Reçu n° ${charge.receipt.number}.`)
        return
      }
      toast.success('Encaissement enregistré.')
      onClose()
    } catch (caught) {
      if (caught instanceof ApiError) {
        // Le serveur nomme le champ fautif (référence manquante, moyen
        // désactivé…) : on l'affiche tel quel plutôt qu'un message générique.
        setError(caught.fields.reference ?? caught.fields.payment_method_id ?? caught.message)
        return
      }
      setError("L'encaissement a échoué.")
    }
  }

  async function copierRecu() {
    if (!recu) return
    try {
      await navigator.clipboard.writeText(recuEnTexte(recu))
      toast.success('Reçu copié : collez-le dans le message au client.')
    } catch {
      toast.error('La copie a été refusée par le navigateur.')
    }
  }

  if (recu) {
    return (
      <Modal
        open={open}
        title={`Reçu n° ${recu.number}`}
        description="Justificatif remis au client."
        onClose={onClose}
        footer={
          <>
            <Button variant="ghost" onClick={onClose}>
              Fermer
            </Button>
            <Button variant="secondary" onClick={() => void copierRecu()}>
              Copier le reçu
            </Button>
          </>
        }
      >
        <ReceiptView recu={recu} />
      </Modal>
    )
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
          <Button variant="primary" onClick={() => void submit()} loading={mutation.isPending}>
            Encaisser
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {moyens.isLoading ? (
          <Spinner label="Chargement des moyens de paiement…" />
        ) : actifs.length === 0 ? (
          <p className="rounded-field border border-warning/40 bg-warning-soft px-3 py-2 text-sm text-warning">
            Aucun moyen de paiement n&apos;est actif : activez-en un dans{' '}
            <span className="font-medium">Paramètres → Caisse et reçus</span>.
          </p>
        ) : (
          <Field label="Moyen de paiement" required hint={choisi ? `Famille : ${choisi.family_label}` : undefined}>
            {(id) => (
              <Select
                id={id}
                value={methodId}
                onChange={(event) => setMethodId(event.target.value)}
                options={actifs.map((moyen) => ({ value: moyen.id, label: moyen.name }))}
              />
            )}
          </Field>
        )}
        {choisi?.family === 'deferred' ? (
          <p className="rounded-field border border-warning/40 bg-warning-soft px-3 py-2 text-xs text-warning">
            <Badge tone="warning">Engagement</Badge> Un paiement différé n&apos;est pas un encaissement :
            la fiche reste due, aucun compteur de caisse ne bouge, et le reçu n&apos;est pas émis.
          </p>
        ) : null}
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
        <Field
          label="Référence"
          required={choisi?.requires_reference}
          hint={choisi?.requires_reference ? 'Obligatoire pour ce moyen de paiement.' : 'Ex : numéro de transaction.'}
        >
          {(id) => <Input id={id} value={reference} onChange={(event) => setReference(event.target.value)} />}
        </Field>
        <Field label="Note">
          {(id) => <Input id={id} value={note} onChange={(event) => setNote(event.target.value)} />}
        </Field>
        {error ? (
          <p className="rounded-field border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p>
        ) : null}
      </div>
    </Modal>
  )
}
