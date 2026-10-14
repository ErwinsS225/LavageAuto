/**
 * Étape 7 « Options, formules et suppléments ».
 *
 * Une option est ce qui se facture **en plus** d'une prestation : lavage des
 * tapis, cire, désinfection. Une formule est une prestation qui regroupe
 * plusieurs opérations — elle se crée à l'étape précédente, et se tarife dans la
 * grille.
 *
 * Deux règles tenues ici :
 *
 *  - le supplément est décidé par la station, jamais envoyé par la caisse ;
 *  - une option peut viser une prestation précise, un type de véhicule, ou
 *    toute la station — et le serveur vérifie qu'elle s'applique bien à la
 *    ligne vendue.
 *
 * Sans option déclarée, la caisse n'en propose aucune : elle n'en invente pas.
 */
import { Plus, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'

import { Badge, Button, Card, CardHeader, EmptyState, Field, Input, Select } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import {
  useCreateServiceOption,
  useDisableServiceOption,
  useServiceOptions,
  useUpdateServiceOption,
} from '@/features/services/pricing-api'
import { useStationServices } from '@/features/services/api'
import { useVehicleOptions } from '@/features/vehicle-categories/api'
import { ApiError } from '@/lib/api'
import { parseAmountInput } from '@/lib/format'
import type { ServiceOption } from '@/types'

export default function OptionsForm({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const options = useServiceOptions(stationId)
  const prestations = useStationServices({ station_id: stationId })
  const vehicules = useVehicleOptions(stationId)
  const creation = useCreateServiceOption()
  const miseAJour = useUpdateServiceOption()
  const desactivation = useDisableServiceOption()

  const [nouvelle, setNouvelle] = useState({
    name: '',
    price: '',
    description: '',
    service_id: '',
    category_code: '',
    duration: '0',
  })
  const [prixModifies, setPrixModifies] = useState<Record<string, string>>({})
  const [erreur, setErreur] = useState('')

  const liste = options.data?.items ?? []
  const services = prestations.data?.items ?? []
  const categories = vehicules.data?.categories ?? []

  function portee(option: ServiceOption): string {
    if (option.service_id) {
      const service = services.find((entree) => entree.id === option.service_id)
      return service ? `Sur « ${service.name} »` : 'Sur une prestation'
    }
    if (option.category_code) {
      const categorie = categories.find((entree) => entree.id === option.category_code)
      return categorie ? `Véhicules : ${categorie.name}` : `Véhicules : ${option.category_code}`
    }
    return 'Toutes les prestations'
  }

  async function ajouter(event: FormEvent) {
    event.preventDefault()
    setErreur('')
    if (!nouvelle.name.trim()) {
      setErreur("Le nom de l'option est obligatoire.")
      return
    }
    if (nouvelle.price === '') {
      setErreur('Indiquez le supplément : le serveur ne l’invente pas.')
      return
    }
    try {
      await creation.mutateAsync({
        station_id: stationId,
        name: nouvelle.name.trim(),
        description: nouvelle.description.trim(),
        price_xof: parseAmountInput(nouvelle.price),
        duration_minutes: Number(nouvelle.duration) || 0,
        service_id: nouvelle.service_id || undefined,
        category_code: nouvelle.category_code || undefined,
      })
      setNouvelle({
        name: '',
        price: '',
        description: '',
        service_id: '',
        category_code: '',
        duration: '0',
      })
      toast.success('Option ajoutée — la caisse la proposera sur les lignes concernées.')
      onEnregistre()
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "L'ajout a échoué.")
    }
  }

  async function enregistrerPrix(option: ServiceOption) {
    const saisie = prixModifies[option.id]
    if (saisie === undefined) return
    try {
      await miseAJour.mutateAsync({ id: option.id, price_xof: parseAmountInput(saisie) })
      setPrixModifies((actuel) => {
        const copie = { ...actuel }
        delete copie[option.id]
        return copie
      })
      toast.success(`Supplément de « ${option.name} » enregistré.`)
      onEnregistre()
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  async function desactiver(option: ServiceOption) {
    try {
      await desactivation.mutateAsync(option.id)
      toast.success(`« ${option.name} » désactivée. Les fiches passées la conservent.`)
      onEnregistre()
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : 'La désactivation a échoué.')
    }
  }

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          title="Vos options et suppléments"
          subtitle="Ce qui s'ajoute à une prestation, facturé par lavage. Sans supplément saisi, la caisse n'en propose aucun."
          actions={<Badge tone={liste.length > 0 ? 'success' : 'neutral'}>{liste.length} option(s)</Badge>}
        />
        <div className="p-4">
          {options.isLoading ? (
            <p className="text-sm text-ink-muted">Chargement des options…</p>
          ) : liste.length === 0 ? (
            <EmptyState
              title="Aucune option déclarée"
              description="Ajoutez par exemple « Lavage des tapis » ou « Cire de protection », avec le supplément que vous facturez."
            />
          ) : (
            <ul className="divide-y divide-line">
              {liste.map((option) => (
                <li key={option.id} className="flex flex-wrap items-end justify-between gap-3 py-3">
                  <div className="min-w-0">
                    <p className="font-medium text-ink">{option.name}</p>
                    <p className="mt-0.5 text-xs text-ink-muted">
                      {portee(option)}
                      {option.duration_minutes > 0 ? ` · +${option.duration_minutes} min` : ''}
                      {option.description ? ` · ${option.description}` : ''}
                    </p>
                  </div>
                  <div className="flex items-end gap-2">
                    <Field label="Supplément (FCFA)">
                      {(id) => (
                        <Input
                          id={id}
                          inputMode="numeric"
                          className="h-9 w-32"
                          value={prixModifies[option.id] ?? String(option.price_xof)}
                          onChange={(event) =>
                            setPrixModifies((actuel) => ({ ...actuel, [option.id]: event.target.value }))
                          }
                        />
                      )}
                    </Field>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      disabled={prixModifies[option.id] === undefined}
                      loading={miseAJour.isPending}
                      onClick={() => void enregistrerPrix(option)}
                    >
                      Enregistrer
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-label={`Désactiver ${option.name}`}
                      loading={desactivation.isPending}
                      onClick={() => void desactiver(option)}
                    >
                      <Trash2 size={15} />
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Card>

      <Card>
        <CardHeader
          title="Ajouter une option"
          subtitle="Le supplément vient de vous : l'application n'en suggère aucun."
        />
        <form className="grid gap-4 p-4 sm:grid-cols-4" onSubmit={ajouter} noValidate>
          <div className="sm:col-span-2">
            <Field label="Nom de l'option" required>
              {(id) => (
                <Input
                  id={id}
                  value={nouvelle.name}
                  onChange={(event) => setNouvelle((actuel) => ({ ...actuel, name: event.target.value }))}
                  placeholder="Ex : Lavage des tapis"
                />
              )}
            </Field>
          </div>
          <Field label="Supplément (FCFA)" required>
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={nouvelle.price}
                onChange={(event) => setNouvelle((actuel) => ({ ...actuel, price: event.target.value }))}
              />
            )}
          </Field>
          <Field label="Durée ajoutée (min)">
            {(id) => (
              <Input
                id={id}
                inputMode="numeric"
                value={nouvelle.duration}
                onChange={(event) => setNouvelle((actuel) => ({ ...actuel, duration: event.target.value }))}
              />
            )}
          </Field>
          <div className="sm:col-span-2">
            <Field label="Limitée à une prestation" hint="Laisser vide : l'option s'applique à toutes les prestations.">
              {(id) => (
                <Select
                  id={id}
                  value={nouvelle.service_id}
                  onChange={(event) =>
                    setNouvelle((actuel) => ({ ...actuel, service_id: event.target.value }))
                  }
                  options={[
                    { value: '', label: 'Toutes les prestations' },
                    ...services.map((service) => ({ value: service.id, label: service.name })),
                  ]}
                />
              )}
            </Field>
          </div>
          <div className="sm:col-span-2">
            <Field
              label="Limitée à un type de véhicule"
              hint="Laisser vide : l'option s'applique à tous les véhicules acceptés."
            >
              {(id) => (
                <Select
                  id={id}
                  value={nouvelle.category_code}
                  onChange={(event) =>
                    setNouvelle((actuel) => ({ ...actuel, category_code: event.target.value }))
                  }
                  options={[
                    { value: '', label: 'Tous les véhicules' },
                    ...categories.map((categorie) => ({ value: categorie.id, label: categorie.name })),
                  ]}
                />
              )}
            </Field>
          </div>
          <div className="sm:col-span-4">
            <Field label="Description (facultatif)">
              {(id) => (
                <Input
                  id={id}
                  value={nouvelle.description}
                  onChange={(event) =>
                    setNouvelle((actuel) => ({ ...actuel, description: event.target.value }))
                  }
                  placeholder="Ce que l'option comprend, en une ligne"
                />
              )}
            </Field>
          </div>
          <div className="sm:col-span-4">
            <Button type="submit" variant="primary" icon={<Plus size={15} />} loading={creation.isPending}>
              Ajouter l'option
            </Button>
          </div>
        </form>
        {erreur ? <p className="border-t border-line px-4 py-3 text-sm text-danger">{erreur}</p> : null}
      </Card>
    </div>
  )
}
