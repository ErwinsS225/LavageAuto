/**
 * Grille tarifaire de la station.
 *
 * Cette page n'invente rien : elle donne accès aux **décisions** de prix, et à
 * elles seules. Trois registres, un seul principe — un prix se pose avec un
 * motif, et le serveur reste le seul à le calculer :
 *
 *  1. la grille par type de véhicule (et par usage commercial) ;
 *  2. les options facturées en plus d'une prestation ;
 *  3. les règles particulières (contrat client, période de validité).
 */
import { SlidersHorizontal, Sparkles, Tags } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'

import {
  Badge,
  Button,
  Card,
  CardHeader,
  EmptyState,
  PageHeader,
  Segmented,
  SkeletonRows,
  StatCard,
} from '@/components/ui'
import OptionsForm from '@/features/onboarding/forms/OptionsForm'
import TarifsForm from '@/features/onboarding/forms/TarifsForm'
import { usePriceMatrix } from '@/features/services/pricing-api'
import { useStationServices } from '@/features/services/api'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'

type Onglet = 'grille' | 'options'

export default function TarifsPage() {
  usePageTitle('Tarifs')
  const { stationId, station } = useStation()
  const [parametres, setParametres] = useSearchParams()
  const [onglet, setOnglet] = useState<Onglet>(
    parametres.get('onglet') === 'options' ? 'options' : 'grille',
  )

  const matrice = usePriceMatrix(stationId || undefined)
  const prestations = useStationServices({ station_id: stationId || undefined })

  const aTarifer = matrice.data?.a_tarifer ?? 0
  const total = prestations.data?.items?.length ?? 0
  const catalogue = prestations.data?.items ?? []
  const gratuites = catalogue.filter((service) => service.is_free).length
  const surDevis = catalogue.filter((service) => service.is_quote_only).length

  function changerOnglet(valeur: Onglet) {
    setOnglet(valeur)
    setParametres(valeur === 'options' ? { onglet: 'options' } : {}, { replace: true })
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Tarifs"
        subtitle={
          station
            ? `Les prix de ${station.name}. Chaque modification porte un motif : une grille doit pouvoir s'expliquer.`
            : 'Les prix de votre station. Chaque modification porte un motif.'
        }
        actions={
          <Segmented
            value={onglet}
            onChange={changerOnglet}
            layoutId="tarifs-onglets"
            options={[
              { value: 'grille', label: 'Grille tarifaire' },
              { value: 'options', label: 'Options et suppléments' },
            ]}
          />
        }
      />

      <div className="grid gap-4 sm:grid-cols-4">
        <StatCard label="Prestations au catalogue" value={String(total)} icon={<Sparkles size={18} />} />
        <StatCard
          label="À tarifer"
          value={String(aTarifer)}
          tone={aTarifer > 0 ? 'warning' : 'success'}
          hint={aTarifer > 0 ? 'Ces prestations ne se vendent pas encore.' : 'Tout est vendable.'}
        />
        <StatCard label="Prestations gratuites" value={String(gratuites)} hint="Décision explicite, jamais un oubli." />
        <StatCard label="Sur devis" value={String(surDevis)} hint="Le montant se décide après examen." />
      </div>

      {!stationId ? (
        <Card>
          <EmptyState
            icon={<SlidersHorizontal size={22} />}
            title="Aucune station sélectionnée"
            description="Choisissez une station pour consulter et modifier sa grille tarifaire."
          />
        </Card>
      ) : onglet === 'grille' ? (
        <TarifsForm stationId={stationId} onEnregistre={() => undefined} />
      ) : prestations.isLoading ? (
        <Card>
          <SkeletonRows rows={4} />
        </Card>
      ) : (
        <>
          <Card>
            <CardHeader
              title="Options et suppléments"
              subtitle="Ce qui se facture en plus d'une prestation : lavage des tapis, cire, désinfection."
              actions={<Badge tone="neutral">Facturé au lavage</Badge>}
            />
          </Card>
          <OptionsForm stationId={stationId} onEnregistre={() => undefined} />
        </>
      )}

      <Card>
        <CardHeader
          title="Comment un prix est choisi"
          subtitle="Ordre appliqué par le serveur, du plus précis au plus général."
          actions={<Tags size={16} className="text-ink-muted" />}
        />
        <ol className="space-y-2 px-4 pb-4 text-sm text-ink-muted">
          <li>
            <span className="font-medium text-ink">1. Contrat client</span> — un tarif négocié pour un client précis.
          </li>
          <li>
            <span className="font-medium text-ink">2. Prestation + véhicule + usage</span> — la case la plus précise
            de la grille.
          </li>
          <li>
            <span className="font-medium text-ink">3. Prestation + véhicule</span> (ou prestation + usage) — le tarif
            le plus courant.
          </li>
          <li>
            <span className="font-medium text-ink">4. Tarif du catalogue</span> — le repli, que vous pouvez retirer
            dans les paramètres de la station.
          </li>
        </ol>
        <p className="border-t border-line px-4 py-3 text-xs text-ink-muted">
          La caisse ne calcule jamais un montant : elle demande au serveur, qui renvoie le prix et son origine. C'est
          ce qui permet à une facture de s'expliquer des mois plus tard.
        </p>
      </Card>

      {aTarifer > 0 ? (
        <Card>
          <CardHeader
            title="Prestations qui attendent un tarif"
            subtitle="Tant qu'elles n'en ont pas, la vente est refusée avec un message clair."
            actions={<Badge tone="warning">{aTarifer}</Badge>}
          />
          <ul className="divide-y divide-line text-sm">
            {(matrice.data?.services ?? [])
              .filter((ligne) => ligne.needs_price)
              .slice(0, 12)
              .map((ligne) => (
                <li key={ligne.service_id} className="flex items-center justify-between gap-3 px-4 py-2">
                  <span className="text-ink">{ligne.name}</span>
                  <span className="text-xs text-ink-muted">{ligne.family_name}</span>
                </li>
              ))}
          </ul>
          {aTarifer > 12 ? (
            <p className="border-t border-line px-4 py-2 text-xs text-ink-muted">
              … et {aTarifer - 12} autre(s), visibles dans la grille.
            </p>
          ) : null}
        </Card>
      ) : null}

      <div className="flex justify-end">
        <Button variant="ghost" onClick={() => changerOnglet(onglet === 'grille' ? 'options' : 'grille')}>
          {onglet === 'grille' ? 'Voir les options et suppléments' : 'Revenir à la grille tarifaire'}
        </Button>
      </div>
    </div>
  )
}
