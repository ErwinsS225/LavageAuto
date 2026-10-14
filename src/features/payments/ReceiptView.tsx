/**
 * Reçu : ce que le client reçoit.
 *
 * Rendu à partir de **ce que le serveur a enregistré** (`GET
 * /wash-orders/{id}/receipt`) : numéro attribué une fois, mentions saisies par
 * une personne habilitée, sommes réellement reçues. Une mention vide n'est pas
 * remplacée par une valeur d'exemple : elle est signalée comme non renseignée.
 *
 * `ApercuRecu` montre la mise en page d'un reçu **avant** tout encaissement. Il
 * est étiqueté « Exemple » et ses montants sont annoncés comme tels : un aperçu
 * qui ressemblerait à une recette réelle serait un faux document.
 */
import { Badge, Card, CardHeader } from '@/components/ui'
import { formatDateTime, formatXOF } from '@/lib/format'
import type { CashSettings, Receipt } from '@/types'

function Ligne({ libelle, valeur }: { libelle: string; valeur: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4 text-sm">
      <dt className="text-ink-muted">{libelle}</dt>
      <dd className="tabular text-right font-medium text-ink">{valeur}</dd>
    </div>
  )
}

function Entete({ recu }: { recu: Receipt }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-line pb-3">
      <div>
        <p className="text-base font-semibold text-ink">{recu.station_name}</p>
        {recu.station_city ? <p className="text-xs text-ink-muted">{recu.station_city}</p> : null}
        {recu.station_phone ? <p className="text-xs text-ink-muted">{recu.station_phone}</p> : null}
        {recu.legal_name ? <p className="mt-1 text-xs text-ink">{recu.legal_name}</p> : null}
        {recu.tax_id ? <p className="text-xs text-ink-muted">N° fiscal : {recu.tax_id}</p> : null}
      </div>
      {recu.logo_url ? (
        <img src={recu.logo_url} alt="" className="h-12 w-12 rounded object-contain" />
      ) : null}
    </div>
  )
}

export function ReceiptView({ recu }: { recu: Receipt }) {
  return (
    <div className="space-y-4">
      <Entete recu={recu} />
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <p className="text-sm font-semibold text-ink">Reçu n° {recu.number}</p>
          <p className="text-xs text-ink-muted">Émis le {formatDateTime(recu.issued_at)}</p>
        </div>
        <Badge tone="success">Soldé</Badge>
      </div>

      <dl className="grid gap-2 sm:grid-cols-2">
        <Ligne libelle="Fiche" valeur={recu.order_reference} />
        <Ligne libelle="Journée" valeur={recu.business_day} />
        <Ligne libelle="Client" valeur={recu.customer_name || 'Client de passage'} />
        <Ligne libelle="Véhicule" valeur={recu.vehicle_category_name || '—'} />
        {recu.plate ? <Ligne libelle="Immatriculation" valeur={recu.plate} /> : null}
        {recu.employee_name ? <Ligne libelle="Laveur" valeur={recu.employee_name} /> : null}
      </dl>

      <div className="overflow-hidden rounded-field border border-line">
        <table className="w-full text-sm">
          <thead className="bg-surface-muted text-xs uppercase text-ink-muted">
            <tr>
              <th className="px-3 py-2 text-left">Prestation</th>
              <th className="px-3 py-2 text-right">Qté</th>
              <th className="px-3 py-2 text-right">Prix</th>
              <th className="px-3 py-2 text-right">Total</th>
            </tr>
          </thead>
          <tbody>
            {recu.lines.map((ligne, index) => (
              <tr key={index} className="border-t border-line">
                <td className="px-3 py-2">
                  {ligne.service_name}
                  {ligne.vehicle_category_name ? (
                    <span className="block text-xs text-ink-muted">{ligne.vehicle_category_name}</span>
                  ) : null}
                </td>
                <td className="tabular px-3 py-2 text-right">{ligne.quantity}</td>
                <td className="tabular px-3 py-2 text-right">{formatXOF(ligne.unit_price_xof)}</td>
                <td className="tabular px-3 py-2 text-right font-medium">{formatXOF(ligne.total_xof)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <dl className="space-y-1">
        <Ligne libelle="Sous-total" valeur={formatXOF(recu.subtotal_xof)} />
        {recu.discount_xof > 0 ? (
          <Ligne
            libelle={`Remise${recu.discount_reason ? ` (${recu.discount_reason})` : ''}`}
            valeur={`− ${formatXOF(recu.discount_xof)}`}
          />
        ) : null}
        <Ligne libelle="Total" valeur={formatXOF(recu.total_xof)} />
        <Ligne libelle="Sommes reçues" valeur={formatXOF(recu.paid_xof)} />
      </dl>

      {recu.payments.length > 0 ? (
        <div className="rounded-field border border-line px-3 py-2">
          <p className="text-xs font-medium uppercase text-ink-muted">Règlements</p>
          <ul className="mt-1 space-y-1">
            {recu.payments.map((reglement, index) => (
              <li key={index} className="flex items-baseline justify-between gap-3 text-sm">
                <span className="text-ink">
                  {reglement.method_label || reglement.method}
                  {reglement.reference ? (
                    <span className="text-ink-muted"> · {reglement.reference}</span>
                  ) : null}
                </span>
                <span className="tabular font-medium">{formatXOF(reglement.amount_xof)}</span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {recu.footer ? (
        <p className="border-t border-line pt-3 text-xs text-ink-muted">{recu.footer}</p>
      ) : null}

      {recu.missing_mentions.length > 0 ? (
        <p className="rounded-field border border-warning/40 bg-warning-soft px-3 py-2 text-xs text-warning">
          Mentions non renseignées, donc non imprimées : {recu.missing_mentions.join(', ')}. Elles se
          saisissent dans <span className="font-medium">Paramètres → Caisse et reçus</span>.
        </p>
      ) : null}
    </div>
  )
}

/**
 * Aperçu d'un reçu avant le premier encaissement : mise en page seule.
 * Étiqueté « Exemple » — les montants ne sont pas une recette.
 */
export function ApercuRecu({ settings, ville }: { settings: CashSettings; ville?: string }) {
  const exemple: Receipt = {
    number: `${settings.receipt_prefix || 'REC'}-00000000-0001`,
    issued_at: new Date().toISOString(),
    station_name: 'Nom de votre station',
    station_city: ville ?? '',
    station_phone: '—',
    legal_name: settings.receipt_legal_name,
    tax_id: settings.receipt_tax_id,
    logo_url: settings.receipt_logo_url,
    footer: settings.receipt_footer,
    order_reference: 'FICHE-0001',
    business_day: 'Exemple',
    customer_name: 'Client de passage',
    plate: 'AA-000-AA',
    vehicle_category_name: 'Berline',
    employee_name: '',
    lines: [
      {
        service_name: 'Prestation (exemple)',
        vehicle_category_name: 'Berline',
        quantity: 1,
        unit_price_xof: 0,
        total_xof: 0,
      },
    ],
    subtotal_xof: 0,
    discount_xof: 0,
    discount_reason: '',
    total_xof: 0,
    paid_xof: 0,
    due_xof: 0,
    payments: [{ amount_xof: 0, method: 'especes', method_label: 'Espèces', reference: '', paid_at: '', received_by_name: '' }],
    missing_mentions: [],
  }
  const absentes = [
    ['raison sociale', settings.receipt_legal_name],
    ['identifiant fiscal', settings.receipt_tax_id],
    ['bas de reçu', settings.receipt_footer],
    ['logo', settings.receipt_logo_url],
  ]
    .filter(([, valeur]) => !String(valeur ?? '').trim())
    .map(([libelle]) => libelle as string)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone="info" dot>
          Exemple
        </Badge>
        <span className="text-xs text-ink-muted">
          Mise en page seulement : aucun montant, aucune recette. Les valeurs se remplissent à
          l&apos;encaissement d&apos;une fiche.
        </span>
      </div>
      <div className="rounded-field border border-dashed border-line p-4">
        <Entete recu={exemple} />
        <div className="mt-3 flex items-center justify-between">
          <p className="text-sm font-semibold text-ink">
            Reçu n° {exemple.number} <span className="text-ink-muted">(exemple)</span>
          </p>
        </div>
        <dl className="mt-3 space-y-1">
          <Ligne libelle="Fiche" valeur={exemple.order_reference} />
          <Ligne libelle="Client" valeur={exemple.customer_name} />
          <Ligne libelle="Véhicule" valeur={exemple.vehicle_category_name} />
          <Ligne libelle="Prestation" valeur="1 × prix de la fiche" />
          <Ligne libelle="Total" valeur="montant réel de la fiche" />
          <Ligne libelle="Règlement" valeur="moyen réellement utilisé" />
        </dl>
        {exemple.footer ? (
          <p className="mt-3 border-t border-line pt-2 text-xs text-ink-muted">{exemple.footer}</p>
        ) : null}
      </div>
      {absentes.length > 0 ? (
        <p className="text-xs text-ink-muted">
          À compléter pour que le reçu du client porte vos mentions : {absentes.join(', ')}.
        </p>
      ) : null}
    </div>
  )
}

/**
 * Version texte du reçu : elle sert au presse-papiers, pour transmettre le
 * justificatif par messagerie. Le justificatif officiel reste celui du serveur.
 */
export function recuEnTexte(recu: Receipt): string {
  const lignes: string[] = []
  lignes.push(recu.station_name)
  if (recu.station_city) lignes.push(recu.station_city)
  if (recu.legal_name) lignes.push(recu.legal_name)
  if (recu.tax_id) lignes.push(`N° fiscal : ${recu.tax_id}`)
  lignes.push('', `Reçu n° ${recu.number} — ${formatDateTime(recu.issued_at)}`, `Fiche : ${recu.order_reference}`)
  lignes.push(`Client : ${recu.customer_name || 'Client de passage'}`)
  for (const ligne of recu.lines) {
    lignes.push(`- ${ligne.service_name} ×${ligne.quantity} : ${formatXOF(ligne.total_xof)}`)
  }
  lignes.push(`Total : ${formatXOF(recu.total_xof)}`, `Reçu : ${formatXOF(recu.paid_xof)}`)
  for (const reglement of recu.payments) {
    lignes.push(`  ${reglement.method_label || reglement.method} ${formatXOF(reglement.amount_xof)}`)
  }
  if (recu.footer) lignes.push('', recu.footer)
  return lignes.join('\n')
}

/** Cadre d'affichage d'un reçu dans un écran (fiche, caisse). */
export function ReceiptCard({ recu, titre = 'Reçu' }: { recu: Receipt; titre?: string }) {
  return (
    <Card>
      <CardHeader title={titre} subtitle={`N° ${recu.number}`} />
      <div className="p-4">
        <ReceiptView recu={recu} />
      </div>
    </Card>
  )
}
