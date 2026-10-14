/**
 * Étape « usages commerciaux » : ce que le client fait de son véhicule.
 *
 * L'usage est un référentiel **distinct** des catégories : une berline « taxi
 * compteur » reste une berline. La station active les usages qu'elle rencontre,
 * puis déclare, pour chacun, les véhicules concernés — c'est cette combinaison
 * que la caisse proposera. Aucun prix n'est demandé ici : la grille tarifaire
 * par couple arrive avec l'étape des tarifs, et l'application n'invente aucun
 * montant.
 */
import { Search } from 'lucide-react'
import { useMemo, useState } from 'react'

import { Badge, Button, Card, CardHeader, Input, Switch } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import {
  useSaveStationUsages,
  useSaveUsageCategories,
  useUsageCategories,
  useVehicleCatalogue,
} from '@/features/vehicle-categories/api'
import { ApiError } from '@/lib/api'

type Couple = { usage_code: string; category_code: string }

export default function UsagesForm({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const catalogue = useVehicleCatalogue(stationId)
  const couplesQuery = useUsageCategories(stationId)
  const enregistrerUsages = useSaveStationUsages(stationId)
  const enregistrerCouples = useSaveUsageCategories(stationId)

  const [recherche, setRecherche] = useState('')
  const [actifs, setActifs] = useState<Record<string, boolean>>({})
  const [couplesModifies, setCouplesModifies] = useState<Record<string, boolean>>({})
  const [erreur, setErreur] = useState('')

  const usages = catalogue.data?.usages ?? []
  const categoriesActives = (catalogue.data?.categories ?? []).filter((entree) => entree.configured && entree.is_active)

  /** Les couples enregistrés dont l'état local n'a pas encore été touché. */
  const couplesServeur = useMemo(() => {
    const ensemble = new Set<string>()
    for (const couple of couplesQuery.data?.items ?? []) {
      if (couple.is_active) ensemble.add(`${couple.usage_code}:${couple.category_code}`)
    }
    return ensemble
  }, [couplesQuery.data])

  function coupleActif(usageCode: string, categorieCode: string) {
    const cle = `${usageCode}:${categorieCode}`
    if (cle in couplesModifies) return couplesModifies[cle]
    return couplesServeur.has(cle)
  }

  const filtrees = usages.filter((usage) => {
    const terme = recherche.trim().toLowerCase()
    if (!terme) return true
    return [usage.name, usage.family, usage.description, ...(usage.aliases ?? [])].some((champ) =>
      (champ ?? '').toLowerCase().includes(terme),
    )
  })

  const familles = useMemo(() => {
    const vues: string[] = []
    for (const usage of filtrees) {
      if (!vues.includes(usage.family)) vues.push(usage.family)
    }
    return vues
  }, [filtrees])

  const estActif = (usage: { code: string; configured: boolean; is_active: boolean }) =>
    actifs[usage.code] ?? (usage.configured && usage.is_active)
  const nbActifs = usages.filter(estActif).length
  const nbCouples = usages.reduce(
    (total, usage) =>
      total + categoriesActives.filter((categorie) => coupleActif(usage.code, categorie.code)).length,
    0,
  )

  async function enregistrer() {
    setErreur('')
    try {
      const actifsEffectifs = usages
        .filter(estActif)
        .map((usage) => usage.code)

      await enregistrerUsages.mutateAsync(
        usages.map((usage, index) => ({
          code: usage.code,
          is_active: actifsEffectifs.includes(usage.code),
          sort_order: (index + 1) * 10,
        })),
      )

      // Un couple ne peut être déclaré que si l'usage **et** la catégorie sont
      // actifs : le serveur refuse le reste, et le message le dirait.
      const couples: Couple[] = []
      for (const usage of usages) {
        if (!actifsEffectifs.includes(usage.code)) continue
        for (const categorie of categoriesActives) {
          if (coupleActif(usage.code, categorie.code)) {
            couples.push({ usage_code: usage.code, category_code: categorie.code })
          }
        }
      }
      await enregistrerCouples.mutateAsync(couples)

      setActifs({})
      setCouplesModifies({})
      toast.success(`${actifsEffectifs.length} usage(s) et ${couples.length} combinaison(s) enregistrés.`)
      onEnregistre()
    } catch (caught) {
      if (caught instanceof ApiError && Object.keys(caught.fields).length > 0) {
        setErreur(
          `${caught.message} (${Object.entries(caught.fields)
            .map(([cle, valeur]) => `${cle} : ${valeur}`)
            .join(' · ')})`,
        )
        return
      }
      setErreur(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué. Réessayez.")
    }
  }

  if (catalogue.isLoading) {
    return <p className="text-sm text-ink-muted">Chargement des usages…</p>
  }

  return (
    <Card>
      <CardHeader
        title="Usages commerciaux"
        subtitle="Ce que le client fait de son véhicule. Un usage ne remplace pas la catégorie : il s’y ajoute."
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Badge tone={nbActifs > 0 ? 'brand' : 'warning'}>{nbActifs} usage(s)</Badge>
            <Badge tone={nbCouples > 0 ? 'brand' : 'warning'}>{nbCouples} combinaison(s)</Badge>
          </div>
        }
      />

      <div className="space-y-4 p-4 pt-0">
        {categoriesActives.length === 0 ? (
          <p className="rounded-field border border-line bg-surface-muted/60 px-3 py-2 text-sm text-ink-muted">
            Activez d’abord au moins une catégorie de véhicule (étape précédente) : c’est ce qui permet d’associer
            un usage à un type de véhicule.
          </p>
        ) : null}

        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-[220px] flex-1">
            <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-subtle" aria-hidden />
            <Input
              value={recherche}
              onChange={(event) => setRecherche(event.target.value)}
              placeholder="Rechercher (taxi, livraison, flotte…)"
              className="pl-8"
              aria-label="Rechercher un usage"
            />
          </div>
          <Button onClick={() => void enregistrer()} disabled={enregistrerUsages.isPending || enregistrerCouples.isPending}>
            {enregistrerUsages.isPending || enregistrerCouples.isPending ? 'Enregistrement…' : 'Enregistrer les usages'}
          </Button>
        </div>

        {erreur ? <p className="text-sm text-danger">{erreur}</p> : null}

        <div className="space-y-4">
          {familles.map((famille) => (
            <div key={famille} className="space-y-2">
              <p className="text-xs font-semibold uppercase tracking-wide text-ink-subtle">{famille}</p>
              <ul className="divide-y divide-line rounded-field border border-line">
                {filtrees
                  .filter((usage) => usage.family === famille)
                  .map((usage) => {
                    const actif = estActif(usage)
                    return (
                      <li key={usage.code} className="space-y-3 px-3 py-3">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div className="min-w-[200px] flex-1">
                            <p className="text-sm font-medium text-ink">{usage.name}</p>
                            <p className="text-xs text-ink-muted">{usage.description}</p>
                          </div>
                          <Switch
                            label={actif ? 'Activé' : 'Désactivé'}
                            checked={actif}
                            onChange={(valeur) => setActifs((prec) => ({ ...prec, [usage.code]: valeur }))}
                          />
                        </div>

                        {actif && categoriesActives.length > 0 ? (
                          <div>
                            <p className="mb-1 text-xs text-ink-muted">Véhicules concernés par cet usage :</p>
                            <div className="flex flex-wrap gap-2">
                              {categoriesActives.map((categorie) => {
                                const coche = coupleActif(usage.code, categorie.code)
                                return (
                                  <button
                                    key={categorie.code}
                                    type="button"
                                    className={'chip ' + (coche ? 'chip-active' : '')}
                                    aria-pressed={coche}
                                    onClick={() =>
                                      setCouplesModifies((prec) => ({
                                        ...prec,
                                        [`${usage.code}:${categorie.code}`]: !coche,
                                      }))
                                    }
                                  >
                                    {categorie.local_name?.trim() || categorie.name}
                                  </button>
                                )
                              })}
                            </div>
                          </div>
                        ) : null}
                      </li>
                    )
                  })}
              </ul>
            </div>
          ))}
        </div>

        <p className="text-xs text-ink-muted">
          Aucun tarif n’est demandé ici : le prix par couple (véhicule × usage) se règle à l’étape des tarifs,
          avec vos propres montants.
        </p>
      </div>
    </Card>
  )
}
