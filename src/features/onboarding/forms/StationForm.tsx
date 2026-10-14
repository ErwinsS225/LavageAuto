/**
 * Étape « station » : où elle se trouve, et ce qu'elle peut faire.
 *
 * Les capacités et les équipements sont volontairement **à trois états** :
 * oui / non / non renseigné. « Pas d'aspiration » et « on ne sait pas encore »
 * ne veulent pas dire la même chose, et l'application ne doit pas transformer
 * l'un en l'autre — un rapport qui annoncerait « aucune aspiration » alors que
 * personne n'a répondu serait faux.
 */
import { useEffect, useState, type FormEvent } from 'react'

import { Badge, Button, Card, CardHeader, Field, Input, Select, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useSaveStationProfile, useStationProfile } from '@/features/profile/api'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import { parseAmountInput } from '@/lib/format'
import type { StationSiteProfile } from '@/types'

/** Une capacité binaire : oui, non, ou pas de réponse. */
type TriEtat = 'non_renseigne' | 'oui' | 'non'

function versTriEtat(valeur: boolean | null | undefined): TriEtat {
  if (valeur === true) return 'oui'
  if (valeur === false) return 'non'
  return 'non_renseigne'
}

function depuisTriEtat(valeur: TriEtat): boolean | null {
  if (valeur === 'oui') return true
  if (valeur === 'non') return false
  return null
}

const OPTIONS_TRI_ETAT = [
  { value: 'non_renseigne', label: 'Non renseigné' },
  { value: 'oui', label: 'Oui' },
  { value: 'non', label: 'Non' },
]

export default function StationForm({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const { station } = useStation()
  const profil = useStationProfile(stationId)
  const enregistrement = useSaveStationProfile()

  const [adresse, setAdresse] = useState({
    name: '', code: '', phone: '', city: '', country: '',
    district: '', commune: '', district_area: '', street: '', landmark: '',
    latitude: '', longitude: '', billing_address: '',
  })
  const [site, setSite] = useState<StationSiteProfile | null>(null)
  const [erreur, setErreur] = useState('')

  const stationChargee = profil.data?.station
  useEffect(() => {
    if (!stationChargee) return
    setAdresse({
      name: stationChargee.name ?? '',
      code: stationChargee.code ?? '',
      phone: stationChargee.phone ?? '',
      city: stationChargee.city ?? '',
      country: stationChargee.country ?? "Côte d'Ivoire",
      district: stationChargee.district ?? '',
      commune: stationChargee.commune ?? '',
      district_area: stationChargee.district_area ?? '',
      street: stationChargee.street ?? '',
      landmark: stationChargee.landmark ?? '',
      latitude: stationChargee.latitude ?? '',
      longitude: stationChargee.longitude ?? '',
      billing_address: stationChargee.billing_address ?? '',
    })
  }, [stationChargee])
  useEffect(() => {
    if (profil.data?.site) setSite(profil.data.site)
  }, [profil.data?.site])

  function majAdresse(champ: string, valeur: string) {
    setAdresse((actuel) => ({ ...actuel, [champ]: valeur }))
  }

  function majSite(champ: keyof StationSiteProfile, valeur: unknown) {
    setSite((actuel) => (actuel ? { ...actuel, [champ]: valeur } : actuel))
  }

  function basculerTypeLavage(type: string) {
    setSite((actuel) => {
      if (!actuel) return actuel
      const present = actuel.wash_types?.includes(type)
      return {
        ...actuel,
        wash_types: present
          ? actuel.wash_types.filter((item) => item !== type)
          : [...(actuel.wash_types ?? []), type],
      }
    })
  }

  async function enregistrer(event: FormEvent) {
    event.preventDefault()
    setErreur('')
    if (!adresse.name.trim()) {
      setErreur('Le nom de la station est obligatoire.')
      return
    }
    try {
      await enregistrement.mutateAsync({
        id: stationId,
        ...adresse,
        site: site ?? {},
      })
      toast.success('Fiche de la station enregistrée.')
      onEnregistre()
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  if (profil.isLoading) {
    return <p className="text-sm text-ink-muted">Chargement de la fiche de la station…</p>
  }
  if (profil.isError || !stationChargee || !site) {
    return (
      <Card>
        <CardHeader
          title="Fiche indisponible"
          subtitle="La station n'a pas pu être lue. Vérifiez la connexion puis réessayez."
        />
        <div className="p-4">
          <Button variant="secondary" onClick={() => void profil.refetch()}>
            Réessayer
          </Button>
        </div>
      </Card>
    )
  }

  const typesProposes = profil.data?.types_lavage_proposes ?? []

  return (
    <form className="space-y-5" onSubmit={enregistrer} noValidate>
      <Card>
        <CardHeader
          title="Localisation"
          subtitle={
            station
              ? 'Ce que vos clients cherchent : la commune, le quartier, un repère.'
              : 'Adresse de la station exploitée.'
          }
        />
        <div className="grid gap-4 p-4 sm:grid-cols-2">
          <Field label="Nom de la station" required>
            {(id) => <Input id={id} value={adresse.name} onChange={(event) => majAdresse('name', event.target.value)} />}
          </Field>
          <Field label="Code court" hint="Deux ou trois lettres, pour les rapports.">
            {(id) => <Input id={id} value={adresse.code} onChange={(event) => majAdresse('code', event.target.value)} />}
          </Field>
          <Field label="Ville" required>
            {(id) => <Input id={id} value={adresse.city} onChange={(event) => majAdresse('city', event.target.value)} />}
          </Field>
          <Field label="Pays">
            {(id) => <Input id={id} value={adresse.country} onChange={(event) => majAdresse('country', event.target.value)} />}
          </Field>
          <Field label="Commune">
            {(id) => <Input id={id} value={adresse.commune} onChange={(event) => majAdresse('commune', event.target.value)} placeholder="Ex : Cocody" />}
          </Field>
          <Field label="Quartier">
            {(id) => <Input id={id} value={adresse.district_area} onChange={(event) => majAdresse('district_area', event.target.value)} placeholder="Ex : Angré" />}
          </Field>
          <Field label="Rue / repère d'adresse">
            {(id) => <Input id={id} value={adresse.street} onChange={(event) => majAdresse('street', event.target.value)} />}
          </Field>
          <Field label="Point de repère" hint="Ce qu'on dit au téléphone pour venir.">
            {(id) => <Input id={id} value={adresse.landmark} onChange={(event) => majAdresse('landmark', event.target.value)} placeholder="Ex : en face de la pharmacie" />}
          </Field>
          <Field label="Téléphone de la station">
            {(id) => <Input id={id} value={adresse.phone} onChange={(event) => majAdresse('phone', event.target.value)} />}
          </Field>
          <Field label="Adresse de facturation" hint="Si elle diffère de l'adresse du site.">
            {(id) => <Input id={id} value={adresse.billing_address} onChange={(event) => majAdresse('billing_address', event.target.value)} />}
          </Field>
          <Field label="Latitude" hint="Facultatif, en degrés décimaux.">
            {(id) => <Input id={id} value={adresse.latitude} onChange={(event) => majAdresse('latitude', event.target.value)} placeholder="5.3489" />}
          </Field>
          <Field label="Longitude" hint="Facultatif, en degrés décimaux.">
            {(id) => <Input id={id} value={adresse.longitude} onChange={(event) => majAdresse('longitude', event.target.value)} placeholder="-4.0123" />}
          </Field>
        </div>
      </Card>

      <Card>
        <CardHeader
          title="Ce que la station peut faire"
          subtitle="Ces valeurs sont des déclarations de votre part, pas des mesures : l'application les présente comme telles."
        />
        <div className="space-y-4 p-4">
          <div>
            <p className="text-sm font-medium text-ink">Types de lavage pratiqués</p>
            <p className="mt-0.5 text-xs text-ink-muted">Cochez ce que vous proposez réellement.</p>
            <div className="mt-2 flex flex-wrap gap-2">
              {typesProposes.map((type) => {
                const actif = site.wash_types?.includes(type)
                return (
                  <button
                    key={type}
                    type="button"
                    onClick={() => basculerTypeLavage(type)}
                    className={
                      'chip ' + (actif ? 'chip-active' : '')
                    }
                    aria-pressed={actif}
                  >
                    {type}
                  </button>
                )
              })}
            </div>
            {site.wash_types?.length ? (
              <p className="mt-2 text-xs text-ink-muted">
                Déclaré : {site.wash_types.join(' · ')}
              </p>
            ) : (
              <p className="mt-2 text-xs text-warning">Aucun type de lavage déclaré pour l'instant.</p>
            )}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Pistes de lavage" hint="Nombre de véhicules que vous pouvez traiter en parallèle sur une même aire.">
              {(id) => (
                <Input
                  id={id}
                  inputMode="numeric"
                  value={site.bays ?? ''}
                  onChange={(event) =>
                    majSite('bays', event.target.value === '' ? null : parseAmountInput(event.target.value))
                  }
                />
              )}
            </Field>
            <Field label="Véhicules simultanés" hint="Combien de véhicules sont traités en même temps, tout compris.">
              {(id) => (
                <Input
                  id={id}
                  inputMode="numeric"
                  value={site.simultaneous_vehicles ?? ''}
                  onChange={(event) =>
                    majSite('simultaneous_vehicles', event.target.value === '' ? null : parseAmountInput(event.target.value))
                  }
                />
              )}
            </Field>
            <Field label="Source d'eau">
              {(id) => (
                <Input
                  id={id}
                  value={site.water_source}
                  onChange={(event) => majSite('water_source', event.target.value)}
                  placeholder="Ex : raccordement + forage"
                />
              )}
            </Field>
            <Field label="Réserve d'eau (litres)">
              {(id) => (
                <Input
                  id={id}
                  inputMode="numeric"
                  value={site.water_reserve_liters ?? ''}
                  onChange={(event) =>
                    majSite('water_reserve_liters', event.target.value === '' ? null : parseAmountInput(event.target.value))
                  }
                />
              )}
            </Field>
            <Field label="Alimentation électrique">
              {(id) => (
                <Input
                  id={id}
                  value={site.power_supply}
                  onChange={(event) => majSite('power_supply', event.target.value)}
                  placeholder="Ex : réseau CIE"
                />
              )}
            </Field>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            {(
              [
                ['has_vacuum', 'Aspiration sur place'],
                ['has_drying', 'Séchage'],
                ['has_detailing', 'Detailing / polissage'],
                ['has_waiting_room', 'Salle d’attente'],
                ['has_storage', 'Local de stockage'],
                ['has_generator', 'Groupe électrogène'],
              ] as Array<[keyof StationSiteProfile, string]>
            ).map(([champ, libelle]) => (
              <Field key={String(champ)} label={libelle}>
                {(id) => (
                  <Select
                    id={id}
                    value={versTriEtat(site[champ] as boolean | null)}
                    onChange={(event) => majSite(champ, depuisTriEtat(event.target.value as TriEtat))}
                    options={OPTIONS_TRI_ETAT}
                  />
                )}
              </Field>
            ))}
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Observations" hint="Ce qu'un nouveau gérant doit savoir en arrivant.">
              {(id) => <Textarea id={id} value={site.notes} onChange={(event) => majSite('notes', event.target.value)} />}
            </Field>
            <Field label="Contraintes du site" hint="Eau, électricité, accès, voisinage…">
              {(id) => <Textarea id={id} value={site.constraints} onChange={(event) => majSite('constraints', event.target.value)} />}
            </Field>
          </div>
        </div>
      </Card>

      {erreur ? (
        <p className="rounded-field border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger">{erreur}</p>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" variant="primary" loading={enregistrement.isPending}>
          Enregistrer la station
        </Button>
        <Badge tone="neutral">Capacités déclarées, non mesurées</Badge>
      </div>
    </form>
  )
}
