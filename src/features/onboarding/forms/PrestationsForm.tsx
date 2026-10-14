/**
 * Étape 6 « Catalogue des prestations ».
 *
 * Trois gestes, dans l'ordre où l'exploitant les fait :
 *
 *  1. **activer ses familles** — ce qu'il propose : extérieur, intérieur,
 *     formules, prestations spécialisées, par type de véhicule ;
 *  2. **reprendre des prestations du référentiel** — l'application décrit ce
 *     qu'elles comprennent et combien de temps elles prennent, jamais leur
 *     prix : un modèle n'est pas un tarif ;
 *  3. **préciser les siennes** — durée, véhicules concernés, gratuité, devis.
 *
 * Un prix inventé finirait sur une facture : il n'y en a aucun ici. Tant qu'une
 * prestation n'est pas tarifée (étape « Grille tarifaire »), le serveur refuse
 * de la vendre (`409 price_required`) et l'écran l'affiche « à tarifer ».
 */
import { Plus, Search } from 'lucide-react'
import { useMemo, useState, type FormEvent } from 'react'

import { Badge, Button, Card, CardHeader, EmptyState, Field, Input, Select, Switch } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import {
  useAdoptTemplate,
  useServiceCompatibilities,
  useServiceTemplates,
  useSetServiceCompatibilities,
  useUpdateServiceFamilies,
} from '@/features/services/pricing-api'
import { useCreateService, useStationServices, useUpdateService } from '@/features/services/api'
import { useVehicleOptions } from '@/features/vehicle-categories/api'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import type { Service, ServiceTemplate } from '@/types'

export default function PrestationsForm({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const { station } = useStation()
  const catalogue = useStationServices({ station_id: stationId })
  const referentiel = useServiceTemplates(stationId)
  const vehicules = useVehicleOptions(stationId)
  const familles = useUpdateServiceFamilies()
  const adoption = useAdoptTemplate()
  const creation = useCreateService()
  const miseAJour = useUpdateService()
  const compatibilites = useSetServiceCompatibilities()

  const [recherche, setRecherche] = useState('')
  const [familleChoisie, setFamilleChoisie] = useState<string>('')
  const [modifsFamilles, setModifsFamilles] = useState<Record<string, boolean>>({})
  // Aucun champ de prix ici : le tarif se pose dans la grille tarifaire, avec
  // son motif. Cet écran compose le catalogue, il ne le tarife pas.
  const [nouvelle, setNouvelle] = useState({
    name: '',
    duration: '30',
    is_free: false,
    is_quote_only: false,
  })
  const [prestationOuverte, setPrestationOuverte] = useState<string>('')
  const [erreur, setErreur] = useState('')

  const prestations = catalogue.data?.items ?? []
  const categoriesActives = vehicules.data?.categories ?? []
  const modeles = referentiel.data?.templates ?? []
  const famillesReferentielles = referentiel.data?.families ?? []

  /** L'état affiché d'une famille : la modification en cours, sinon le serveur. */
  const familleActive = (code: string): boolean => {
    if (code in modifsFamilles) return modifsFamilles[code]
    const enregistree = famillesReferentielles.find((f) => f.code === code)
    return Boolean(enregistree?.configured && enregistree.is_active)
  }

  const modelesAffiches = useMemo(() => {
    const terme = recherche.trim().toLowerCase()
    return modeles.filter((modele) => {
      if (familleChoisie && modele.family_code !== familleChoisie) return false
      if (!terme) return true
      return (
        modele.name.toLowerCase().includes(terme) ||
        modele.family_name.toLowerCase().includes(terme) ||
        modele.description.toLowerCase().includes(terme) ||
        modele.code.includes(terme)
      )
    })
  }, [modeles, recherche, familleChoisie])

  const tarifees = prestations.filter((item) => !item.needs_price).length
  const aTarifer = prestations.length - tarifees

  async function enregistrerFamilles() {
    const families = famillesReferentielles.map((famille) => ({
      code: famille.code,
      is_active: familleActive(famille.code),
    }))
    try {
      await familles.mutateAsync({ stationId, families })
      setModifsFamilles({})
      toast.success('Familles enregistrées.')
      onEnregistre()
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  async function adopter(modele: ServiceTemplate) {
    try {
      await adoption.mutateAsync({ station_id: stationId, template_code: modele.code })
      toast.success(`« ${modele.name} » ajoutée à votre catalogue — reste à la tarifer.`)
      onEnregistre()
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'ajout a échoué.")
    }
  }

  async function ajouter(event: FormEvent) {
    event.preventDefault()
    setErreur('')
    if (!nouvelle.name.trim()) {
      setErreur('Le nom de la prestation est obligatoire.')
      return
    }
    try {
      await creation.mutateAsync({
        station_id: stationId,
        name: nouvelle.name.trim(),
        duration_minutes: Number(nouvelle.duration) || 30,
        is_free: nouvelle.is_free,
        is_quote_only: nouvelle.is_quote_only,
      })
      setNouvelle({ name: '', duration: '30', is_free: false, is_quote_only: false })
      toast.success('Prestation ajoutée au catalogue.')
      onEnregistre()
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "L'ajout a échoué.")
    }
  }

  async function changerEtat(prestation: Service, patch: Record<string, unknown>, message: string) {
    try {
      await miseAJour.mutateAsync({ id: prestation.id, ...patch })
      toast.success(message)
      onEnregistre()
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  return (
    <div className="space-y-5">
      {/* 1. Les familles : ce que la station propose. */}
      <Card>
        <CardHeader
          title="Vos familles de prestations"
          subtitle="Activez ce que votre station propose. Une famille désactivée disparaît de la caisse — rien n'est supprimé."
          actions={
            <Button
              size="sm"
              variant="secondary"
              loading={familles.isPending}
              onClick={() => void enregistrerFamilles()}
            >
              Enregistrer les familles
            </Button>
          }
        />
        <div className="space-y-4 p-4">
          {referentiel.isLoading ? (
            <p className="text-sm text-ink-muted">Chargement du référentiel…</p>
          ) : (
            famillesReferentielles.map((famille) => {
              const total = modeles.filter((modele) => modele.family_code === famille.code).length
              return (
                <Switch
                  key={famille.code}
                  checked={familleActive(famille.code)}
                  onChange={(valeur) =>
                    setModifsFamilles((actuel) => ({ ...actuel, [famille.code]: valeur }))
                  }
                  label={`${famille.name} — ${total} prestation(s)`}
                  hint={famille.description}
                />
              )
            })
          )}
        </div>
      </Card>

      {/* 2. Le catalogue de référence : décrit, jamais tarifé. */}
      <Card>
        <CardHeader
          title="Prestations de référence"
          subtitle="À reprendre telles quelles ou à adapter : le contenu et la durée sont décrits, le prix reste le vôtre."
          actions={<Badge tone="info">{modeles.length} modèle(s)</Badge>}
        />
        <div className="space-y-4 p-4">
          <div className="flex flex-wrap gap-3">
            <div className="relative min-w-56 flex-1">
              <Search size={15} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-muted" />
              <Input
                className="pl-9"
                placeholder="Rechercher une prestation…"
                value={recherche}
                onChange={(event) => setRecherche(event.target.value)}
              />
            </div>
            <Field label="Famille" className="min-w-48">
              {(id) => (
                <Select
                  id={id}
                  value={familleChoisie}
                  onChange={(event) => setFamilleChoisie(event.target.value)}
                  options={[
                    { value: '', label: 'Toutes les familles' },
                    ...famillesReferentielles.map((famille) => ({
                      value: famille.code,
                      label: famille.name,
                    })),
                  ]}
                />
              )}
            </Field>
          </div>

          {modelesAffiches.length === 0 ? (
            <EmptyState
              title="Aucune prestation ne correspond"
              description="Élargissez la recherche, ou changez de famille."
            />
          ) : (
            <ul className="divide-y divide-line">
              {modelesAffiches.map((modele) => (
                <li key={modele.code} className="flex flex-wrap items-start justify-between gap-3 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="flex flex-wrap items-center gap-2 font-medium text-ink">
                      {modele.name}
                      {modele.is_quote_only ? <Badge tone="warning">sur devis</Badge> : null}
                      {modele.adopted ? <Badge tone="success">dans votre catalogue</Badge> : null}
                    </p>
                    <p className="mt-0.5 text-xs text-ink-muted">
                      {modele.family_name} · {modele.duration_minutes} min
                      {modele.category_codes.length > 0
                        ? ` · véhicules : ${modele.category_codes.join(', ')}`
                        : ''}
                    </p>
                    {modele.includes.length > 0 ? (
                      <p className="mt-1 text-xs text-ink-muted">Comprend : {modele.includes.join(' · ')}</p>
                    ) : null}
                    {modele.preconditions ? (
                      <p className="mt-1 text-xs text-warning">À prévoir : {modele.preconditions}</p>
                    ) : null}
                  </div>
                  {modele.adopted ? (
                    <Badge tone="neutral">reprise</Badge>
                  ) : (
                    <Button
                      size="sm"
                      variant="secondary"
                      loading={adoption.isPending}
                      onClick={() => void adopter(modele)}
                    >
                      Ajouter à mon catalogue
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </Card>

      {/* 3. Le catalogue de la station. */}
      <Card>
        <CardHeader
          title="Votre catalogue"
          subtitle={
            station
              ? `Prestations de ${station.name}. Le prix se pose à l'étape « Grille tarifaire » — jamais suggéré ici.`
              : 'Prestations facturables de la station.'
          }
          actions={
            <Badge tone={aTarifer === 0 && prestations.length > 0 ? 'success' : 'warning'}>
              {tarifees} / {prestations.length} tarifée(s)
            </Badge>
          }
        />
        <div className="p-4">
          {catalogue.isLoading ? (
            <p className="text-sm text-ink-muted">Chargement du catalogue…</p>
          ) : prestations.length === 0 ? (
            <EmptyState
              title="Aucune prestation"
              description="Reprenez une prestation de référence ci-dessus, ou créez la vôtre."
            />
          ) : (
            <ul className="divide-y divide-line">
              {prestations.map((prestation) => (
                <li key={prestation.id} className="py-3">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="font-medium text-ink">{prestation.name}</p>
                      <p className="mt-0.5 text-xs text-ink-muted">
                        {prestation.family_name || prestation.category || 'lavage'} ·{' '}
                        {prestation.duration_minutes} min
                        {prestation.is_free ? ' · gratuite' : ''}
                        {prestation.is_quote_only ? ' · sur devis' : ''}
                        {prestation.needs_price ? (
                          <>
                            {' · '}
                            <span className="text-warning">à tarifer</span>
                          </>
                        ) : (
                          <> · {prestation.price_xof > 0 ? `${prestation.price_xof.toLocaleString('fr-FR')} FCFA` : 'tarif par règles'}</>
                        )}
                      </p>
                      {prestation.includes.length > 0 ? (
                        <p className="mt-1 text-xs text-ink-muted">
                          Comprend : {prestation.includes.join(' · ')}
                        </p>
                      ) : null}
                    </div>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() =>
                        setPrestationOuverte((actuelle) => (actuelle === prestation.id ? '' : prestation.id))
                      }
                    >
                      {prestationOuverte === prestation.id ? 'Fermer' : 'Modifier'}
                    </Button>
                  </div>

                  {prestationOuverte === prestation.id ? (
                    <div className="mt-3 space-y-4 rounded-field border border-line bg-surface-muted/50 p-3">
                      <div className="grid gap-3 sm:grid-cols-3">
                        <Field label="Durée (min)">
                          {(id) => (
                            <Input
                              id={id}
                              inputMode="numeric"
                              defaultValue={prestation.duration_minutes}
                              onBlur={(event) => {
                                const valeur = Number(event.target.value.replace(/[^0-9]/g, ''))
                                if (valeur > 0 && valeur !== prestation.duration_minutes) {
                                  void changerEtat(
                                    prestation,
                                    { duration_minutes: valeur },
                                    'Durée enregistrée.',
                                  )
                                }
                              }}
                            />
                          )}
                        </Field>
                      </div>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Switch
                          checked={prestation.is_free}
                          label="Prestation gratuite"
                          hint="Elle se vend à zéro volontairement : aucune règle de prix ne s'applique."
                          onChange={(valeur) =>
                            void changerEtat(
                              prestation,
                              { is_free: valeur },
                              valeur ? 'Prestation marquée gratuite.' : 'Gratuité retirée.',
                            )
                          }
                        />
                        <Switch
                          checked={prestation.is_quote_only}
                          label="Chiffrée sur devis"
                          hint="Elle ne se vend pas au comptoir : la caisse refuse de l'encaisser."
                          onChange={(valeur) =>
                            void changerEtat(
                              prestation,
                              { is_quote_only: valeur },
                              valeur ? 'Prestation sur devis.' : 'Vente au comptoir rétablie.',
                            )
                          }
                        />
                      </div>
                      <CompatibilitesPrestation
                        prestation={prestation}
                        categories={categoriesActives}
                        onEnregistre={() => {
                          void compatibilites.reset()
                          onEnregistre()
                        }}
                      />
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </div>
      </Card>

      {/* 4. Créer une prestation propre à la station. */}
      <Card>
        <CardHeader
          title="Ajouter une prestation"
          subtitle="À vous de la décrire : le prix se pose juste après, dans la grille tarifaire."
        />
        <form className="grid gap-4 p-4 sm:grid-cols-3" onSubmit={ajouter} noValidate>
          <div className="sm:col-span-2">
            <Field label="Nom de la prestation" required>
              {(id) => (
                <Input
                  id={id}
                  value={nouvelle.name}
                  onChange={(event) => setNouvelle((actuel) => ({ ...actuel, name: event.target.value }))}
                  placeholder="Ex : Lavage complet berline"
                />
              )}
            </Field>
          </div>
          <Field label="Durée (min)">
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
            <Switch
              checked={nouvelle.is_free}
              label="Prestation gratuite"
              hint="Un contrôle offert, par exemple."
              onChange={(valeur) => setNouvelle((actuel) => ({ ...actuel, is_free: valeur }))}
            />
          </div>
          <div className="sm:col-span-2">
            <Switch
              checked={nouvelle.is_quote_only}
              label="Chiffrée sur devis"
              hint="Céramique, rénovation : le montant se décide après examen."
              onChange={(valeur) => setNouvelle((actuel) => ({ ...actuel, is_quote_only: valeur }))}
            />
          </div>
          <div className="sm:col-span-3">
            <Button type="submit" variant="primary" icon={<Plus size={15} />} loading={creation.isPending}>
              Ajouter au catalogue
            </Button>
          </div>
        </form>
        {erreur ? <p className="border-t border-line px-4 py-3 text-sm text-danger">{erreur}</p> : null}
      </Card>
    </div>
  )
}

/** À quels véhicules la prestation s'applique (vide = tous ceux acceptés). */
function CompatibilitesPrestation({
  prestation,
  categories,
  onEnregistre,
}: {
  prestation: Service
  categories: Array<{ id: string; name: string }>
  onEnregistre: () => void
}) {
  const toast = useToast()
  const existantes = useServiceCompatibilities(prestation.id)
  const enregistrement = useSetServiceCompatibilities()
  const choisies = new Set(existantes.data?.categories ?? [])

  async function basculer(code: string, actif: boolean) {
    const suivantes = new Set(choisies)
    if (actif) suivantes.add(code)
    else suivantes.delete(code)
    try {
      await enregistrement.mutateAsync({ serviceId: prestation.id, categories: [...suivantes] })
      toast.success(
        suivantes.size === 0
          ? 'Prestation proposée pour tous vos véhicules.'
          : 'Véhicules enregistrés pour cette prestation.',
      )
      onEnregistre()
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  return (
    <div>
      <p className="text-xs font-medium text-ink">Véhicules concernés</p>
      <p className="mt-1 text-xs text-ink-muted">
        Aucune case cochée = la prestation est proposée pour tous les véhicules acceptés par la station.
      </p>
      {categories.length === 0 ? (
        <p className="mt-2 text-xs text-warning">
          Aucune catégorie active : activez-en à l'étape « Catégories de véhicules ».
        </p>
      ) : (
        <div className="mt-2 flex flex-wrap gap-2">
          {categories.map((categorie) => {
            const actif = choisies.has(categorie.id)
            return (
              <button
                key={categorie.id}
                type="button"
                aria-pressed={actif}
                disabled={enregistrement.isPending || existantes.isLoading}
                onClick={() => void basculer(categorie.id, !actif)}
                className={`chip ${actif ? 'chip-active' : ''}`}
              >
                {categorie.name}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
