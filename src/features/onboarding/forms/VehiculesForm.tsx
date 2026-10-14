/**
 * Étape « catégories de véhicules » : ce que la station accepte de laver.
 *
 * Le référentiel est commun à toutes les stations (citadine, berline, moto,
 * gbaka, camion…) : il est **affiché**, jamais recopié. La station coche ce
 * qu'elle lave, peut renommer une entrée (« berline » → « voiture de ville »)
 * et en créer une qui n'existe pas dans le référentiel.
 *
 * Décocher ne supprime rien : les fiches passées gardent le nom qu'elles
 * portaient. C'est ce que dit le texte de l'écran, parce que c'est ce que fait
 * le serveur.
 */
import { Plus, RotateCcw, Search } from 'lucide-react'
import { useMemo, useState, type FormEvent } from 'react'

import { Badge, Button, Card, CardHeader, Field, Input, Switch } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import {
  useCreateVehicleCategory,
  useSaveStationCategories,
  useUpdateVehicleCategory,
  useVehicleCatalogue,
} from '@/features/vehicle-categories/api'
import { ApiError } from '@/lib/api'

/** État local d'une ligne : coché, et nom local s'il a été modifié. */
type Ligne = { actif: boolean; nomLocal: string; ordre: number }

export default function VehiculesForm({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const catalogue = useVehicleCatalogue(stationId)
  const enregistrement = useSaveStationCategories(stationId)
  const creation = useCreateVehicleCategory()
  const renommage = useUpdateVehicleCategory()

  const [recherche, setRecherche] = useState('')
  const [modifs, setModifs] = useState<Record<string, Ligne>>({})
  const [nouvelle, setNouvelle] = useState({ nom: '', famille: '', description: '' })
  const [erreur, setErreur] = useState('')

  const entrees = catalogue.data?.categories ?? []

  /** Les lignes telles qu'elles seront envoyées : le référentiel, puis les
   *  modifications locales. L'état n'existe pas avant la réponse du serveur. */
  const lignes = useMemo(() => {
    return entrees.map((entree) => {
      const locale = modifs[entree.code]
      return {
        entree,
        actif: locale ? locale.actif : entree.configured ? entree.is_active : false,
        nomLocal: locale ? locale.nomLocal : (entree.local_name ?? ''),
        ordre: locale ? locale.ordre : entree.sort_order,
      }
    })
  }, [entrees, modifs])

  const filtrees = lignes.filter((ligne) => {
    const terme = recherche.trim().toLowerCase()
    if (!terme) return true
    const champs = [ligne.entree.name, ligne.entree.family, ligne.entree.description, ...(ligne.entree.aliases ?? [])]
    return champs.some((champ) => (champ ?? '').toLowerCase().includes(terme))
  })

  const familles = useMemo(() => {
    const vues: string[] = []
    for (const ligne of filtrees) {
      if (!vues.includes(ligne.entree.family)) vues.push(ligne.entree.family)
    }
    return vues
  }, [filtrees])

  const cochees = lignes.filter((ligne) => ligne.actif).length

  function basculer(code: string, actif: boolean) {
    setModifs((prec) => {
      const courante = prec[code] ?? { actif, nomLocal: '', ordre: 0 }
      return { ...prec, [code]: { ...courante, actif } }
    })
  }

  function renommerLocal(code: string, nomLocal: string) {
    setModifs((prec) => {
      const courante = prec[code] ?? { actif: true, nomLocal: '', ordre: 0 }
      return { ...prec, [code]: { ...courante, nomLocal } }
    })
  }

  /** Monter ou descendre une ligne : l'ordre part vers le serveur en bloc. */
  function deplacer(code: string, direction: -1 | 1) {
    const ordonnees = [...lignes].sort((a, b) => a.ordre - b.ordre)
    const position = ordonnees.findIndex((ligne) => ligne.entree.code === code)
    const cible = position + direction
    if (position < 0 || cible < 0 || cible >= ordonnees.length) return
    const courante = ordonnees[position]
    const voisine = ordonnees[cible]
    setModifs((prec) => ({
      ...prec,
      [courante.entree.code]: { actif: courante.actif, nomLocal: courante.nomLocal, ordre: voisine.ordre },
      [voisine.entree.code]: { actif: voisine.actif, nomLocal: voisine.nomLocal, ordre: courante.ordre },
    }))
  }

  function reinitialiser(code: string) {
    setModifs((prec) => {
      const copie = { ...prec }
      delete copie[code]
      return copie
    })
  }

  async function enregistrer() {
    setErreur('')
    try {
      const reponse = await enregistrement.mutateAsync(
        lignes.map((ligne, index) => ({
          code: ligne.entree.code,
          is_active: ligne.actif,
          name: ligne.nomLocal.trim(),
          sort_order: ligne.ordre || (index + 1) * 10,
        })),
      )
      setModifs({})
      toast.success(`${reponse.actives} catégorie(s) active(s) enregistrée(s).`)
      onEnregistre()
    } catch (caught) {
      setErreur(
        caught instanceof ApiError ? caught.message : "L'enregistrement a échoué. Réessayez.",
      )
    }
  }

  async function ajouter(event: FormEvent) {
    event.preventDefault()
    setErreur('')
    if (!nouvelle.nom.trim()) {
      setErreur('Le nom de la catégorie est obligatoire.')
      return
    }
    try {
      await creation.mutateAsync({
        station_id: stationId,
        name: nouvelle.nom.trim(),
        family: nouvelle.famille.trim(),
        description: nouvelle.description.trim(),
      })
      setNouvelle({ nom: '', famille: '', description: '' })
      toast.success('Catégorie ajoutée à votre station.')
      onEnregistre()
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "L'ajout a échoué.")
    }
  }

  async function retablirNom(code: string, nom: string) {
    try {
      await renommage.mutateAsync({ code, station_id: stationId, name: nom })
      reinitialiser(code)
      toast.success('Nom du référentiel rétabli.')
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : 'Le renommage a échoué.')
    }
  }

  if (catalogue.isLoading) {
    return <p className="text-sm text-ink-muted">Chargement du référentiel…</p>
  }

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          title="Référentiel des véhicules"
          subtitle={catalogue.data?.message_referentiel ?? 'Ce que votre station accepte de laver.'}
          actions={<Badge tone={cochees > 0 ? 'brand' : 'warning'}>{cochees} active(s)</Badge>}
        />

        <div className="space-y-4 p-4 pt-0">
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-[220px] flex-1">
              <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-subtle" aria-hidden />
              <Input
                value={recherche}
                onChange={(event) => setRecherche(event.target.value)}
                placeholder="Rechercher (4x4, moto, gbaka…)"
                className="pl-8"
                aria-label="Rechercher une catégorie"
              />
            </div>
            <Button onClick={() => void enregistrer()} disabled={enregistrement.isPending || lignes.length === 0}>
              {enregistrement.isPending ? 'Enregistrement…' : 'Enregistrer les catégories'}
            </Button>
          </div>

          {erreur ? <p className="text-sm text-danger">{erreur}</p> : null}

          {lignes.length === 0 ? (
            <p className="text-sm text-ink-muted">
              Le référentiel n’a pas pu être lu. Vérifiez que votre station est bien rattachée à votre compte.
            </p>
          ) : (
            familles.map((famille) => (
              <div key={famille} className="space-y-2">
                <p className="text-xs font-semibold uppercase tracking-wide text-ink-subtle">{famille}</p>
                <ul className="divide-y divide-line rounded-field border border-line">
                  {filtrees
                    .filter((ligne) => ligne.entree.family === famille)
                    .map((ligne) => {
                      const renomme = ligne.nomLocal.trim() !== '' && ligne.nomLocal.trim() !== ligne.entree.name
                      return (
                        <li key={ligne.entree.code} className="space-y-2 px-3 py-3">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div className="min-w-[200px] flex-1">
                              <p className="text-sm font-medium text-ink">{ligne.nomLocal.trim() || ligne.entree.name}</p>
                              <p className="text-xs text-ink-muted">{ligne.entree.description}</p>
                            </div>
                            <div className="flex items-center gap-2">
                              <button
                                type="button"
                                className="chip"
                                onClick={() => deplacer(ligne.entree.code, -1)}
                                aria-label={`Monter ${ligne.entree.name}`}
                              >
                                ↑
                              </button>
                              <button
                                type="button"
                                className="chip"
                                onClick={() => deplacer(ligne.entree.code, 1)}
                                aria-label={`Descendre ${ligne.entree.name}`}
                              >
                                ↓
                              </button>
                              <Switch
                                label={ligne.actif ? 'Activée' : 'Désactivée'}
                                checked={ligne.actif}
                                onChange={(actif) => basculer(ligne.entree.code, actif)}
                              />
                            </div>
                          </div>

                          {ligne.actif && (
                            <div className="flex flex-wrap items-end gap-2">
                              <div className="min-w-[200px] flex-1">
                                <Field label="Nom affiché par votre station" hint="Laissez vide pour garder le nom du référentiel.">
                                  {(id) => (
                                    <Input
                                      id={id}
                                      value={ligne.nomLocal}
                                      placeholder={ligne.entree.name}
                                      onChange={(event) => renommerLocal(ligne.entree.code, event.target.value)}
                                    />
                                  )}
                                </Field>
                              </div>
                              {renomme || ligne.entree.configured ? (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() =>
                                    renomme
                                      ? reinitialiser(ligne.entree.code)
                                      : void retablirNom(ligne.entree.code, '')
                                  }
                                >
                                  <RotateCcw size={14} aria-hidden /> Nom du référentiel
                                </Button>
                              ) : null}
                            </div>
                          )}
                        </li>
                      )
                    })}
                </ul>
              </div>
            ))
          )}

          <p className="text-xs text-ink-muted">
            Désactiver une catégorie ne touche pas les fiches passées : elles gardent le nom qu’elles portaient.
            Le référentiel ci-dessus est commun à toutes les stations ; vos choix n’appartiennent qu’à la vôtre.
          </p>
        </div>
      </Card>

      <Card>
        <CardHeader
          title="Ajouter une catégorie propre à votre station"
          subtitle="Pour un véhicule que le référentiel ne couvre pas. Elle vous appartient et ne modifie rien pour les autres."
        />
        <form className="space-y-3 p-4 pt-0" onSubmit={ajouter}>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Nom de la catégorie" required>
              {(id) => (
                <Input
                  id={id}
                  value={nouvelle.nom}
                  onChange={(event) => setNouvelle({ ...nouvelle, nom: event.target.value })}
                  placeholder="Ex : tricycle de livraison"
                />
              )}
            </Field>
            <Field label="Famille" hint="Sert au regroupement à l’écran. Facultatif.">
              {(id) => (
                <Input
                  id={id}
                  value={nouvelle.famille}
                  onChange={(event) => setNouvelle({ ...nouvelle, famille: event.target.value })}
                  placeholder="Deux-roues et assimilés"
                />
              )}
            </Field>
          </div>
          <Field label="Description" hint="Ce que la catégorie recouvre. Facultatif.">
            {(id) => (
              <Input
                id={id}
                value={nouvelle.description}
                onChange={(event) => setNouvelle({ ...nouvelle, description: event.target.value })}
              />
            )}
          </Field>
          <Button type="submit" variant="secondary" disabled={creation.isPending}>
            <Plus size={14} aria-hidden /> {creation.isPending ? 'Ajout…' : 'Ajouter cette catégorie'}
          </Button>
        </form>
      </Card>
    </div>
  )
}
