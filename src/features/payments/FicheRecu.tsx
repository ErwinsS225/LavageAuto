/**
 * Reçu rattaché à une fiche, dans le détail de la fiche.
 *
 * Le reçu est **lu depuis le serveur** : c'est lui qui attribue le numéro, une
 * seule fois, au moment où la fiche est soldée. Ici on ne fait que l'afficher,
 * le copier pour l'envoyer au client, ou le réémettre (idempotent) si la fiche
 * a été soldée avant la mise en service de la numérotation.
 */
import { useState } from 'react'

import { Button, Card, CardHeader, SkeletonRows } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { ReceiptView, recuEnTexte } from '@/features/payments/ReceiptView'
import { useIssueReceipt, useReceipt } from '@/features/payments/api'
import { useAuth } from '@/hooks/useAuth'
import { ApiError } from '@/lib/api'

export default function FicheRecu({
  orderId,
  soldee,
  annulee,
}: {
  orderId: string
  /** Une fiche soldée a un reçu ; une fiche due n'en a pas encore. */
  soldee: boolean
  annulee: boolean
}) {
  const toast = useToast()
  const { can } = useAuth()
  const emission = useIssueReceipt()
  const [erreur, setErreur] = useState('')
  const recuQuery = useReceipt(orderId, soldee && !annulee)

  // Une fiche annulée n'a pas de reçu : le justificatif serait faux.
  if (annulee) return null

  if (!soldee) {
    return (
      <div>
        <p className="stat-label">Reçu</p>
        <p className="text-ink-muted">
          Il reste une somme à encaisser : le reçu sera émis automatiquement quand la fiche sera soldée.
        </p>
      </div>
    )
  }

  if (recuQuery.isLoading) {
    return (
      <Card>
        <CardHeader title="Reçu" subtitle="Lecture du justificatif…" />
        <SkeletonRows rows={4} />
      </Card>
    )
  }

  const recu = recuQuery.data?.receipt
  const nonEmis = recuQuery.error instanceof ApiError && recuQuery.error.status === 404

  async function copier() {
    if (!recu) return
    try {
      await navigator.clipboard.writeText(recuEnTexte(recu))
      toast.success('Reçu copié : collez-le dans le message au client.')
    } catch {
      toast.error('La copie a été refusée par le navigateur.')
    }
  }

  async function emettre() {
    setErreur('')
    try {
      await emission.mutateAsync(orderId)
      toast.success('Reçu émis.')
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "L'émission du reçu a échoué.")
    }
  }

  if (!recu || nonEmis) {
    return (
      <Card>
        <CardHeader
          title="Reçu"
          subtitle={
            nonEmis
              ? "Cette fiche a été soldée avant la numérotation des reçus : aucun numéro ne lui a été attribué."
              : "Le reçu n'a pas pu être lu."
          }
        />
        <div className="flex flex-wrap items-center gap-2 p-4">
          {can('payment:view') ? (
            <Button variant="secondary" loading={emission.isPending} onClick={() => void emettre()}>
              Émettre le reçu
            </Button>
          ) : null}
          <Button variant="ghost" onClick={() => void recuQuery.refetch()}>
            Relire
          </Button>
        </div>
        {erreur ? <p className="border-t border-line px-4 py-3 text-sm text-danger">{erreur}</p> : null}
      </Card>
    )
  }

  return (
    <Card>
      <CardHeader
        title={`Reçu n° ${recu.number}`}
        subtitle={`Émis le ${recu.issued_at ? new Date(recu.issued_at).toLocaleDateString('fr-FR') : '—'}`}
        actions={
          <Button size="sm" variant="secondary" onClick={() => void copier()}>
            Copier
          </Button>
        }
      />
      <div className="p-4">
        <ReceiptView recu={recu} />
      </div>
    </Card>
  )
}
