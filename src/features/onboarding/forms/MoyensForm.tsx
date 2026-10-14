/**
 * Étape 10 du parcours : moyens de paiement et ce que le reçu porte.
 *
 * Une caisse sans moyen actif ne peut rien encaisser : l'étape est donc
 * obligatoire. Elle est franchie par défaut (les espèces sont actives dès la
 * création de la station) — mais elle ne peut pas être cochée sans qu'un moyen
 * soit réellement actif, et c'est ce que le serveur constate.
 *
 * L'aperçu du reçu est étiqueté « Exemple » : il montre la mise en page, pas une
 * recette.
 */
import { Badge, Card, CardHeader } from '@/components/ui'
import { ApercuRecu } from '@/features/payments/ReceiptView'
import CashSettingsForm from '@/features/payments/CashSettingsForm'
import { useCashSettings } from '@/features/payments/api'
import MoyensPanel from '@/features/payments/MoyensPanel'
import { useAuth } from '@/hooks/useAuth'

export default function MoyensForm({
  stationId,
  ville,
  onEnregistre,
}: {
  stationId: string
  ville?: string
  onEnregistre?: () => void
}) {
  const { can } = useAuth()
  const reglages = useCashSettings(stationId)
  // La caisse se configure avec les droits de gestion ; un caissier qui ouvre
  // le parcours voit la mise en page du reçu, pas les champs de configuration.
  const peutGerer = can('settings:manage')

  return (
    <div className="space-y-4">
      <MoyensPanel stationId={stationId} onEnregistre={onEnregistre} />

      {peutGerer ? (
        <CashSettingsForm stationId={stationId} ville={ville} />
      ) : reglages.data ? (
        <Card>
          <CardHeader
            title="Reçu remis au client"
            subtitle="Ces mentions sont gérées par le propriétaire de la station."
            actions={<Badge tone="neutral">Lecture seule</Badge>}
          />
          <div className="p-4">
            <ApercuRecu settings={reglages.data.settings} ville={ville} />
          </div>
        </Card>
      ) : null}
    </div>
  )
}
