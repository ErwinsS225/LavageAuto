/**
 * Étape 8 « Grille tarifaire ».
 *
 * C'est ici que le propriétaire saisit **ses** prix — une fois — et que le
 * serveur décide ensuite, toujours pareil, pour les mêmes entrées. L'écran
 * n'invente aucun montant : les cases vides affichent ce qui s'appliquera
 * réellement (« hérité », « à tarifer »), et la matrice montre d'où vient
 * chaque prix.
 *
 * Ordre de priorité appliqué par le serveur (`backend/pricing.go`) :
 *
 *   1. contrat client ;
 *   2. prestation + type de véhicule + usage ;
 *   3. prestation + type de véhicule (ou prestation + usage) ;
 *   4. tarif du catalogue, si la station l'autorise.
 *
 * Une modification porte toujours un **motif** : une hausse de tarif doit
 * pouvoir s'expliquer des mois plus tard. Les règles particulières (contrat,
 * période de validité) se posent plus bas, avec la même exigence.
 */
import { Save } from 'lucide-react'
import { useMemo, useState } from 'react'

import { Badge, Button, Card, CardHeader, EmptyState, Field, Input, Select, Switch } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import {
  useCreatePriceRule,
  useDisablePriceRule,
  usePriceMatrix,
  usePriceRules,
  useSavePriceMatrix,
  useUpdatePriceRule,
  type PriceRuleInput,
} from '@/features/services/pricing-api'
import { useCustomersOptions } from '@/features/customers/api'
import { useVehicleOptions } from '@/features/vehicle-categories/api'
import { ApiError } from '@/lib/api'
import { parseAmountInput } from '@/lib/format'
import type { PriceMatrixRow } from '@/types'

/** Saisie en cours : clé « service|catégorie » → montant tapé ('' = vide). */
type Saisies = Record<string, string>

export default function TarifsForm({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const matrice = usePriceMatrix(stationId)
  const enregistrement = useSavePriceMatrix()
  const regles = usePriceRules(stationId)

  const [motif, setMotif] = useState('')
  const [saisies, setSaisies] = useState<Saisies>({})
  const [parUsage, setParUsage] = useState(false)

  const lignes = matrice.data?.services ?? []
  const categories = matrice.data?.categories ?? []
  const usages = matrice.data?.usages ?? []
  const generiqueAutorise = matrice.data?.generique_autorise ?? true
  const aTarifer = matrice.data?.a_tarifer ?? 0

  /** Ce qui s'applique aujourd'hui à une case : règle, gratuité, ou héritage. */
  function etatCase(ligne: PriceMatrixRow, code: string, usage = false) {
    const cellules = usage ? ligne.usage_cells : ligne.cells
    const trouvee = cellules.find((cellule) =>
      usage ? cellule.usage_code === code : cellule.category_code === code,
    )
    return trouvee
  }

  function valeurCase(ligne: PriceMatrixRow, code: string, usage = false): string {
    const cle = `${ligne.service_id}|${usage ? 'u' : 'c'}|${code}`
    if (cle in saisies) return saisies[cle]
    const cellule = etatCase(ligne, code, usage)
    return cellule && cellule.price_xof !== null ? String(cellule.price_xof) : ''
  }

  function valeurCatalogue(ligne: PriceMatrixRow): string {
    const cle = `${ligne.service_id}|g|`
    if (cle in saisies) return saisies[cle]
    return ligne.generic_price_xof > 0 ? String(ligne.generic_price_xof) : ''
  }

  function changer(cle: string, valeur: string) {
    setSaisies((actuel) => ({ ...actuel, [cle]: valeur }))
  }

  const modifications = Object.keys(saisies).length

  /** Ce que la case affiche quand elle est vide : le repli réel. */
  function herite(ligne: PriceMatrixRow, _code: string): string {
    if (ligne.is_free) return 'gratuite'
    if (ligne.needs_price) return 'à tarifer'
    if (ligne.generic_price_xof > 0 && generiqueAutorise) return 'hérité du catalogue'
    return 'aucun prix'
  }

  async function enregistrer() {
    if (modifications === 0) {
      toast.info('Aucune modification à enregistrer.')
      return
    }
    if (!motif.trim()) {
      toast.error('Indiquez le motif : une grille tarifaire doit pouvoir s’expliquer.')
      return
    }
    const cells: Array<{ service_id: string; category_code: string; price_xof?: number }> = []
    const usageCells: Array<{ service_id: string; usage_code: string; price_xof?: number }> = []
    const genericPrices: Array<{ service_id: string; price_xof: number }> = []
    for (const [cle, valeur] of Object.entries(saisies)) {
      const [serviceID, type, code] = cle.split('|')
      if (type === 'g') {
        if (valeur !== '') genericPrices.push({ service_id: serviceID, price_xof: parseAmountInput(valeur) })
        continue
      }
      // Case vidée : la règle est désactivée, la vente repasse au niveau suivant.
      const montant = valeur === '' ? undefined : parseAmountInput(valeur)
      if (type === 'c') cells.push({ service_id: serviceID, category_code: code, price_xof: montant })
      else usageCells.push({ service_id: serviceID, usage_code: code, price_xof: montant })
    }
    try {
      const reponse = (await enregistrement.mutateAsync({
        stationId,
        reason: motif.trim(),
        cells,
        usageCells,
        genericPrices,
      })) as { creees: number; modifiees: number; desactivees: number }
      setSaisies({})
      toast.success(
        `Grille enregistrée : ${reponse.creees} prix posé(s), ${reponse.modifiees} mis à jour, ${reponse.desactivees} retiré(s).`,
      )
      onEnregistre()
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  const colonnes = parUsage ? usages : categories

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          title="Matrice des prix"
          subtitle="Une ligne par prestation, une colonne par type de véhicule. Une case vide laisse le prix remonter au niveau suivant."
          icon={<Save size={18} />}
          actions={
            <Badge tone={aTarifer === 0 ? 'success' : 'warning'}>
              {aTarifer === 0 ? 'tout est tarifé' : `${aTarifer} prestation(s) sans prix`}
            </Badge>
          }
        />
        <div className="space-y-4 p-4">
          {matrice.isLoading ? (
            <p className="text-sm text-ink-muted">Chargement de la grille…</p>
          ) : lignes.length === 0 ? (
            <EmptyState
              title="Aucune prestation active"
              description="Ajoutez d'abord vos prestations à l'étape « Catalogue des prestations »."
            />
          ) : (
            <>
              <div className="flex flex-wrap items-end justify-between gap-4">
                <Field label="Motif des modifications" className="min-w-64 flex-1" required>
                  {(id) => (
                    <Input
                      id={id}
                      value={motif}
                      onChange={(event) => setMotif(event.target.value)}
                      placeholder="Ex : grille de saison, hausse du shampoing"
                    />
                  )}
                </Field>
                <Switch
                  checked={parUsage}
                  label="Tarifer par usage"
                  hint="Taxi compteur, livraison, flotte : même véhicule, prix différent."
                  onChange={setParUsage}
                />
              </div>

              {colonnes.length === 0 ? (
                <p className="rounded-field border border-line bg-surface-muted/60 px-4 py-3 text-sm text-ink-muted">
                  {parUsage
                    ? "Aucun usage commercial n'est activé : déclarez-en à l'étape « Usages commerciaux »."
                    : "Aucun véhicule n'est activé : déclarez-en à l'étape « Catégories de véhicules »."}
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>Prestation</th>
                        {colonnes.map((colonne) => (
                          <th key={colonne.code} className="text-right">
                            {colonne.name}
                          </th>
                        ))}
                        {parUsage ? null : <th className="text-right">Tarif catalogue</th>}
                      </tr>
                    </thead>
                    <tbody>
                      {lignes.map((ligne) => (
                        <tr key={ligne.service_id}>
                          <td>
                            <p className="font-medium text-ink">{ligne.name}</p>
                            <p className="text-xs text-ink-muted">
                              {ligne.family_name || 'lavage'} · {ligne.duration_minutes} min
                              {ligne.is_free ? ' · gratuite' : ''}
                              {ligne.is_quote_only ? ' · sur devis' : ''}
                            </p>
                          </td>
                          {colonnes.map((colonne) => {
                            const cle = `${ligne.service_id}|${parUsage ? 'u' : 'c'}|${colonne.code}`
                            const valeur = valeurCase(ligne, colonne.code, parUsage)
                            const cellule = etatCase(ligne, colonne.code, parUsage)
                            return (
                              <td key={colonne.code} className="text-right">
                                <Input
                                  aria-label={`${ligne.name} — ${colonne.name}`}
                                  inputMode="numeric"
                                  className="h-9 w-28 text-right"
                                  disabled={ligne.is_free || ligne.is_quote_only}
                                  value={valeur}
                                  placeholder={
                                    cellule && cellule.price_xof !== null
                                      ? undefined
                                      : herite(ligne, colonne.code)
                                  }
                                  onChange={(event) => changer(cle, event.target.value)}
                                />
                              </td>
                            )
                          })}
                          {parUsage ? null : (
                            <td className="text-right">
                              <Input
                                aria-label={`${ligne.name} — tarif catalogue`}
                                inputMode="numeric"
                                className="h-9 w-28 text-right"
                                disabled={ligne.is_free || ligne.is_quote_only}
                                value={valeurCatalogue(ligne)}
                                placeholder={herite(ligne, 'catalogue')}
                                onChange={(event) => changer(`${ligne.service_id}|g|`, event.target.value)}
                              />
                            </td>
                          )}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              {!generiqueAutorise ? (
                <p className="text-xs text-warning">
                  Cette station vend uniquement par règles : le tarif du catalogue n'est pas appliqué.
                </p>
              ) : null}

              <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line pt-3">
                <p className="text-xs text-ink-muted">
                  {modifications === 0
                    ? 'Aucune case modifiée pour l’instant.'
                    : `${modifications} case(s) modifiée(s) — enregistrées seulement au clic.`}
                </p>
                <Button
                  variant="primary"
                  loading={enregistrement.isPending}
                  disabled={modifications === 0}
                  onClick={() => void enregistrer()}
                >
                  Enregistrer la grille
                </Button>
              </div>
            </>
          )}
        </div>
      </Card>

      <RegleParticuliere stationId={stationId} onEnregistre={onEnregistre} />

      <Card>
        <CardHeader
          title="Règles en vigueur"
          subtitle="Ce qui n'est pas dans la matrice : contrats clients, tarifs par usage, périodes."
          actions={<Badge tone="neutral">{regles.data?.total ?? 0} règle(s)</Badge>}
        />
        <div className="p-4">
          {(regles.data?.items.length ?? 0) === 0 ? (
            <EmptyState
              title="Aucune règle particulière"
              description="Seul le tarif du catalogue s'applique. Posez une règle pour un client, un usage ou une période."
            />
          ) : (
            <ul className="divide-y divide-line">
              {regles.data?.items.map((regle) => (
                <li key={regle.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                  <div className="min-w-0">
                    <p className="font-medium text-ink">
                      {regle.service_name} · {regle.price_xof.toLocaleString('fr-FR')} FCFA
                    </p>
                    <p className="mt-0.5 text-xs text-ink-muted">
                      {regle.specificity_label}
                      {regle.customer_name ? ` · ${regle.customer_name}` : ''}
                      {regle.category_name ? ` · ${regle.category_name}` : ''}
                      {regle.usage_name ? ` · ${regle.usage_name}` : ''}
                      {regle.valid_from || regle.valid_to
                        ? ` · du ${regle.valid_from ?? '…'} au ${regle.valid_to ?? '…'}`
                        : ''}
                    </p>
                    {regle.reason ? (
                      <p className="mt-0.5 text-xs text-ink-muted">Motif : {regle.reason}</p>
                    ) : null}
                  </div>
                  <div className="flex items-end gap-2">
                    <ModifierPrixRegle
                      id={regle.id}
                      prixActuel={regle.price_xof}
                      onEnregistre={onEnregistre}
                    />
                    <RetirerRegle id={regle.id} onEnregistre={onEnregistre} />
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Card>
    </div>
  )
}

/** Changer le tarif d'une règle existante : le motif reste exigé par le serveur. */
function ModifierPrixRegle({
  id,
  prixActuel,
  onEnregistre,
}: {
  id: string
  prixActuel: number
  onEnregistre: () => void
}) {
  const toast = useToast()
  const miseAJour = useUpdatePriceRule()
  const [prix, setPrix] = useState('')
  const [motif, setMotif] = useState('')

  return (
    <>
      <Field label="Nouveau tarif (FCFA)">
        {(champId) => (
          <Input
            id={champId}
            inputMode="numeric"
            className="h-9 w-28 text-right"
            value={prix}
            placeholder={String(prixActuel)}
            onChange={(event) => setPrix(event.target.value)}
          />
        )}
      </Field>
      <Field label="Motif" required>
        {(champId) => (
          <Input
            id={champId}
            className="h-9 w-56"
            value={motif}
            onChange={(event) => setMotif(event.target.value)}
            placeholder="Pourquoi ce prix change"
          />
        )}
      </Field>
      <Button
        type="button"
        size="sm"
        variant="secondary"
        disabled={prix === '' || !motif.trim()}
        loading={miseAJour.isPending}
        onClick={() => {
          void (async () => {
            try {
              await miseAJour.mutateAsync({ id, price_xof: parseAmountInput(prix), reason: motif.trim() })
              setPrix('')
              setMotif('')
              toast.success('Tarif de la règle mis à jour.')
              onEnregistre()
            } catch (caught) {
              toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
            }
          })()
        }}
      >
        Enregistrer
      </Button>
    </>
  )
}

/** Désactivation d'une règle : elle reste dans l'historique, inactive. */
function RetirerRegle({ id, onEnregistre }: { id: string; onEnregistre: () => void }) {
  const toast = useToast()
  const desactivation = useDisablePriceRule()
  return (
    <Button
      size="sm"
      variant="ghost"
      loading={desactivation.isPending}
      onClick={() => {
        void (async () => {
          try {
            await desactivation.mutateAsync(id)
            toast.success('Règle désactivée : le niveau suivant reprend la main.')
            onEnregistre()
          } catch (caught) {
            toast.error(caught instanceof ApiError ? caught.message : 'La désactivation a échoué.')
          }
        })()
      }}
    >
      Désactiver
    </Button>
  )
}

/**
 * Règle particulière : un contrat client, un tarif par usage, une période.
 * Le serveur exige un motif, et refuse deux règles actives identiques — deux
 * prix pour la même ligne ne se départagent pas en silence.
 */
function RegleParticuliere({
  stationId,
  onEnregistre,
}: {
  stationId: string
  onEnregistre: () => void
}) {
  const toast = useToast()
  const matrice = usePriceMatrix(stationId)
  const vehicules = useVehicleOptions(stationId)
  const clients = useCustomersOptions({ station_id: stationId })
  const creation = useCreatePriceRule()

  const [saisie, setSaisie] = useState<PriceRuleInput>({
    station_id: stationId,
    service_id: '',
    category_code: '',
    usage_code: '',
    customer_id: '',
    price_xof: 0,
    reason: '',
    valid_from: '',
    valid_to: '',
  })
  const [prix, setPrix] = useState('')
  const [erreur, setErreur] = useState('')

  const services = useMemo(
    () => (matrice.data?.services ?? []).map((ligne) => ({ value: ligne.service_id, label: ligne.name })),
    [matrice.data],
  )

  async function enregistrer() {
    setErreur('')
    if (!saisie.service_id) {
      setErreur('Choisissez la prestation concernée.')
      return
    }
    if (prix === '') {
      setErreur('Indiquez le tarif de la règle.')
      return
    }
    if (!saisie.reason.trim()) {
      setErreur('Indiquez le motif : un tarif particulier doit s’expliquer.')
      return
    }
    const corps = {
      ...saisie,
      station_id: stationId,
      price_xof: parseAmountInput(prix),
      customer_id: saisie.customer_id || undefined,
      category_code: saisie.category_code || undefined,
      usage_code: saisie.usage_code || undefined,
      valid_from: saisie.valid_from || undefined,
      valid_to: saisie.valid_to || undefined,
    }
    try {
      await creation.mutateAsync(corps)
      toast.success('Règle posée : elle s’appliquera selon sa précision.')
      setPrix('')
      setSaisie((actuel) => ({ ...actuel, reason: '', customer_id: '' }))
      onEnregistre()
    } catch (caught) {
      if (caught instanceof ApiError && caught.code === 'price_rule_conflict') {
        setErreur(
          'Une règle active porte déjà exactement ces critères : modifiez-la, ou désactivez-la avant d’en créer une autre.',
        )
        return
      }
      setErreur(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  return (
    <Card>
      <CardHeader
        title="Poser une règle particulière"
        subtitle="Contrat client, tarif par usage, ou prix valable du … au … Le serveur choisit toujours la règle la plus précise."
      />
      <div className="grid gap-4 p-4 sm:grid-cols-4">
        <div className="sm:col-span-2">
          <Field label="Prestation" required>
            {(id) => (
              <Select
                id={id}
                value={saisie.service_id}
                placeholder="Choisir une prestation"
                onChange={(event) => setSaisie((actuel) => ({ ...actuel, service_id: event.target.value }))}
                options={services}
              />
            )}
          </Field>
        </div>
        <Field label="Tarif de la règle (FCFA)" required>
          {(id) => (
            <Input
              id={id}
              inputMode="numeric"
              value={prix}
              onChange={(event) => setPrix(event.target.value)}
            />
          )}
        </Field>
        <Field label="Client (contrat)">
          {(id) => (
            <Select
              id={id}
              value={saisie.customer_id ?? ''}
              onChange={(event) => setSaisie((actuel) => ({ ...actuel, customer_id: event.target.value }))}
              options={[
                { value: '', label: 'Tous les clients' },
                ...(clients.data?.options ?? []).map((client) => ({
                  value: client.id,
                  label: client.full_name,
                })),
              ]}
            />
          )}
        </Field>
        <Field label="Type de véhicule">
          {(id) => (
            <Select
              id={id}
              value={saisie.category_code ?? ''}
              onChange={(event) => setSaisie((actuel) => ({ ...actuel, category_code: event.target.value }))}
              options={[
                { value: '', label: 'Tous les véhicules' },
                ...(vehicules.data?.categories ?? []).map((categorie) => ({
                  value: categorie.id,
                  label: categorie.name,
                })),
              ]}
            />
          )}
        </Field>
        <Field label="Usage du véhicule">
          {(id) => (
            <Select
              id={id}
              value={saisie.usage_code ?? ''}
              onChange={(event) => setSaisie((actuel) => ({ ...actuel, usage_code: event.target.value }))}
              options={[
                { value: '', label: 'Tous les usages' },
                ...(vehicules.data?.usages ?? []).map((usage) => ({
                  value: usage.id,
                  label: usage.name,
                })),
              ]}
            />
          )}
        </Field>
        <Field label="Valable à partir du" hint="Facultatif — utile pour un tarif de saison.">
          {(id) => (
            <Input
              id={id}
              type="date"
              value={saisie.valid_from ?? ''}
              onChange={(event) => setSaisie((actuel) => ({ ...actuel, valid_from: event.target.value }))}
            />
          )}
        </Field>
        <Field label="Jusqu'au" hint="Facultatif.">
          {(id) => (
            <Input
              id={id}
              type="date"
              value={saisie.valid_to ?? ''}
              onChange={(event) => setSaisie((actuel) => ({ ...actuel, valid_to: event.target.value }))}
            />
          )}
        </Field>
        <div className="sm:col-span-4">
          <Field label="Motif" required hint="Il sera conservé : un prix doit pouvoir s'expliquer.">
            {(id) => (
              <Input
                id={id}
                value={saisie.reason}
                onChange={(event) => setSaisie((actuel) => ({ ...actuel, reason: event.target.value }))}
                placeholder="Ex : contrat flotte de 12 véhicules, signé le 1er octobre"
              />
            )}
          </Field>
        </div>
        <div className="sm:col-span-4">
          <Button variant="secondary" loading={creation.isPending} onClick={() => void enregistrer()}>
            Poser la règle
          </Button>
        </div>
        {erreur ? <p className="sm:col-span-4 text-sm text-danger">{erreur}</p> : null}
      </div>
    </Card>
  )
}
