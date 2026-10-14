/**
 * Formulaire de création / modification d'une fiche de lavage.
 * Gère client, véhicule, plaque, laveur, prestations, options, remise et statut.
 *
 * **Le prix vient du serveur.** Le poste de caisse ne saisit pas de montant et
 * n'en calcule aucun : il choisit une prestation, un véhicule, un usage et des
 * options, puis affiche ce que `/price-preview` répond — provenance comprise
 * (« Tarif prestation + véhicule », « Contrat client »…). Une prestation sans
 * tarif s'affiche « à tarifer » et la vente sera refusée par le serveur
 * (`409 price_required`) : c'est le gérant qui pose le prix, pas le comptoir.
 */
import { useQueries } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { Button, Field, Input, Select, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useCreateWashOrder, useUpdateWashOrder, type WashOrderDetail } from '@/features/wash-orders/api'
import { useCustomersOptions } from '@/features/customers/api'
import { useEmployeeOptions } from '@/features/employees/api'
import { useUsageCategories, useVehicleOptions } from '@/features/vehicle-categories/api'
import { pricePreviewOptions } from '@/features/services/pricing-api'
import { useStationServices } from '@/features/services/api'
import { ApiError } from '@/lib/api'
import { washStatusLabel, washTransitionsFrom } from '@/lib/constants'
import { parseAmountInput } from '@/lib/format'
import type { WashOrder, WashStatus } from '@/types'

interface FormItem {
  service_id: string
  quantity: number
  /** Options demandées : identifiant + quantité. Le supplément vient du serveur. */
  options: Array<{ option_id: string; quantity: number }>
}

export default function WashOrderForm({
  stationId,
  order,
  detail,
  onDone,
  onCancel,
}: {
  stationId: string
  order?: WashOrder
  detail?: WashOrderDetail
  onDone: () => void
  onCancel: () => void
}) {
  const toast = useToast()
  const editing = Boolean(order)
  const createMutation = useCreateWashOrder()
  const updateMutation = useUpdateWashOrder()

  const customers = useCustomersOptions({ station_id: stationId || undefined })
  const employees = useEmployeeOptions({ station_id: stationId || undefined })
  // Catégories **et** usages en un seul appel : les deux listes vont ensemble,
  // une requête séparée laisserait un état intermédiaire à l'écran.
  const vehicles = useVehicleOptions(stationId || undefined)
  // Les combinaisons déclarées par la station : une berline « taxi compteur »
  // reste une berline, mais la station peut ne pas proposer ce couple.
  const couples = useUsageCategories(stationId || undefined)
  const services = useStationServices({ station_id: stationId || undefined })

  const [customerId, setCustomerId] = useState(order?.customer_id ?? '')
  const [customerName, setCustomerName] = useState(order?.customer_name ?? '')
  const [vehicleCategoryId, setVehicleCategoryId] = useState(order?.vehicle_category_id ?? '')
  const [vehicleUsageId, setVehicleUsageId] = useState(order?.vehicle_usage_id ?? '')
  const [plate, setPlate] = useState(order?.plate ?? '')
  const [employeeId, setEmployeeId] = useState(order?.employee_id ?? '')
  const [status, setStatus] = useState<WashStatus>(order?.status ?? 'in_progress')

  // Options de statut : le statut actuel, plus les transitions que le cycle de
  // vie autorise depuis lui. Pour une nouvelle fiche, les deux seuls départs
  // possibles (« reçue » ou « en cours »).
  const optionsStatut = useMemo(() => {
    const libelle = (code: WashStatus) => ({
      value: code,
      label: washStatusLabel(code),
    })
    if (!order) {
      return [libelle('pending'), libelle('in_progress')]
    }
    const ouvertes = washTransitionsFrom(order.status).map(libelle)
    if (ouvertes.some((option) => option.value === order.status)) return ouvertes
    return [libelle(order.status), ...ouvertes]
  }, [order])
  const [discount, setDiscount] = useState(order?.discount_xof ?? 0)
  const [discountReason, setDiscountReason] = useState(order?.discount_reason ?? '')
  const [notes, setNotes] = useState(order?.notes ?? '')

  const optionsVehicules = vehicles.data
  const couplesActifs = useMemo(() => {
    const ensemble = new Set<string>()
    for (const couple of couples.data?.items ?? []) {
      if (couple.is_active) ensemble.add(`${couple.usage_code}:${couple.category_code}`)
    }
    return ensemble
  }, [couples.data])

  /**
   * La station déclare les combinaisons qu'elle propose : la caisse ne les
   * invente pas. Quand aucune n'est déclarée (étape « usages » non remplie),
   * les deux listes restent complètes — l'usage est alors informatif. L'entrée
   * déjà choisie reste toujours proposée, sinon une fiche ancienne perdrait son
   * libellé à l'ouverture.
   */
  const categories = useMemo(() => {
    const toutes = optionsVehicules?.categories ?? []
    if (couplesActifs.size === 0 || !vehicleUsageId) return toutes
    return toutes.filter(
      (categorie) => categorie.id === vehicleCategoryId || couplesActifs.has(`${vehicleUsageId}:${categorie.id}`),
    )
  }, [optionsVehicules, couplesActifs, vehicleUsageId, vehicleCategoryId])

  // La grille tarifaire est indexée par **codes** : on les résout ici, pour
  // parler au serveur le langage de ses règles. L'identifiant ne sert que de
  // repli quand le référentiel n'expose pas encore de code.
  const categorieCode = useMemo(
    () => categories.find((categorie) => categorie.id === vehicleCategoryId)?.code ?? '',
    [categories, vehicleCategoryId],
  )

  const usages = useMemo(() => {
    const tous = optionsVehicules?.usages ?? []
    if (couplesActifs.size === 0 || !vehicleCategoryId) return tous
    return tous.filter(
      (usage) => usage.id === vehicleUsageId || couplesActifs.has(`${usage.id}:${vehicleCategoryId}`),
    )
  }, [optionsVehicules, couplesActifs, vehicleCategoryId, vehicleUsageId])
  const usageCode = useMemo(
    () => usages.find((usage) => usage.id === vehicleUsageId)?.code ?? '',
    [usages, vehicleUsageId],
  )
  const [items, setItems] = useState<FormItem[]>(
    detail?.order.items?.map((item) => ({
      service_id: item.service_id ?? '',
      quantity: item.quantity,
      options:
        item.options?.map((option) => ({ option_id: option.option_id, quantity: option.quantity ?? 1 })) ?? [],
    })) ?? [],
  )
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})

  // L'aperçu du serveur, une requête par ligne : c'est lui qui sait le prix.
  // Il se relance dès que la prestation, le véhicule, l'usage ou le client
  // change — le comptoir voit donc le prix avant d'enregistrer la fiche.
  const apercus = useQueries({
    queries: items.map((item) =>
      pricePreviewOptions({
        stationId,
        serviceId: item.service_id,
        categoryCode: categorieCode,
        usageCode,
        categoryId: vehicleCategoryId,
        usageId: vehicleUsageId,
        customerId,
        enabled: Boolean(item.service_id),
      }),
    ),
  })

  /** Supplément d'options d'une ligne, par unité de prestation. */
  function supplementOptions(index: number): number {
    const apercu = apercus[index]?.data
    const item = items[index]
    if (!apercu || !item) return 0
    return item.options.reduce((total, demande) => {
      const option = apercu.options?.find((entree) => entree.id === demande.option_id)
      return total + (option ? option.price_xof * Math.max(demande.quantity, 1) : 0)
    }, 0)
  }

  /** Total d'une ligne : prestation + options, multipliés par la quantité. */
  function totalLigne(index: number): number {
    const apercu = apercus[index]?.data
    const item = items[index]
    if (!apercu || !item || apercu.price_xof === null || apercu.price_xof === undefined) return 0
    return (apercu.price_xof + supplementOptions(index)) * Math.max(item.quantity, 1)
  }

  const subtotal = items.reduce((total, _item, index) => total + totalLigne(index), 0)
  const total = Math.max(subtotal - discount, 0)

  /** Une ligne sans prix utilisable : la fiche sera refusée par le serveur. */
  const lignesSansPrix = apercus
    .map((requete, index) => ({ requete, index }))
    .filter(({ requete, index }) => items[index]?.service_id && requete.data?.needs_price)

  function addItem() {
    setItems((list) => [...list, { service_id: '', quantity: 1, options: [] }])
  }

  function updateItem(index: number, patch: Partial<FormItem>) {
    setItems((list) => list.map((item, i) => (i === index ? { ...item, ...patch } : item)))
  }

  /** Cocher une option, ou ajuster sa quantité. */
  function basculerOption(index: number, optionId: string, quantite: number) {
    setItems((list) =>
      list.map((item, i) => {
        if (i !== index) return item
        const presentes = item.options.filter((option) => option.option_id !== optionId)
        if (quantite <= 0) return { ...item, options: presentes }
        return { ...item, options: [...presentes, { option_id: optionId, quantity: quantite }] }
      }),
    )
  }

  function parseQuantity(raw: string): number {
    const digits = Number(raw.replace(/[^0-9]/g, ''))
    return Math.max(Number.isFinite(digits) && digits > 0 ? digits : 1, 1)
  }

  async function submit() {
    setError('')
    setFields({})
    if (items.length === 0) {
      setError('Ajoutez au moins une prestation.')
      return
    }
    const payload = {
      station_id: stationId || undefined,
      customer_id: customerId || undefined,
      customer_name: customerName || undefined,
      vehicle_category_id: vehicleCategoryId || undefined,
      vehicle_usage_id: vehicleUsageId || undefined,
      plate: plate || undefined,
      employee_id: employeeId || undefined,
      discount_xof: discount || undefined,
      discount_reason: discountReason || undefined,
      notes: notes || undefined,
      status,
      items: items.map((item) => ({
        service_id: item.service_id,
        quantity: item.quantity,
        // Aucun montant n'est transmis : le serveur résout le prix et les
        // suppléments, puis les recopie sur la fiche avec leur provenance.
        options: item.options.length > 0 ? item.options : undefined,
      })),
    }
    try {
      if (editing && order) {
        await updateMutation.mutateAsync({ id: order.id, ...payload })
        toast.success('Fiche mise à jour.')
      } else {
        await createMutation.mutateAsync(payload)
        toast.success('Fiche créée.')
      }
      onDone()
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError("L'enregistrement a échoué.")
      }
    }
  }

  const pending = createMutation.isPending || updateMutation.isPending

  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Client existant" hint="Laisser vide pour un client de passage.">
          {(id) => (
            <Select
              id={id}
              value={customerId}
              onChange={(event) => {
                setCustomerId(event.target.value)
                const found = customers.data?.options.find((option) => option.id === event.target.value)
                if (found) setCustomerName(found.full_name)
              }}
              options={[
                { value: '', label: 'Client de passage' },
                ...(customers.data?.options ?? []).map((option) => ({
                  value: option.id,
                  label: option.full_name,
                })),
              ]}
            />
          )}
        </Field>
        <Field label="Nom du client" error={fields.customer_name}>
          {(id) => (
            <Input
              id={id}
              value={customerName}
              onChange={(event) => setCustomerName(event.target.value)}
              placeholder="Client de passage"
            />
          )}
        </Field>
        <Field
          label="Catégorie de véhicule"
          hint={
            categories.length === 0
              ? 'Aucune catégorie active : activez-en à l’étape « Catégories de véhicules » de la mise en route.'
              : undefined
          }
        >
          {(id) => (
            <Select
              id={id}
              value={vehicleCategoryId}
              onChange={(event) => setVehicleCategoryId(event.target.value)}
              options={[
                { value: '', label: 'Non précisée' },
                ...categories.map((category) => ({
                  value: category.id,
                  label: category.name,
                })),
              ]}
            />
          )}
        </Field>
        <Field
          label="Usage du véhicule"
          hint={
            usages.length === 0
              ? 'Aucun usage activé pour cette station (étape « Usages commerciaux »).'
              : 'Ce que le client fait de son véhicule — taxi, livraison, flotte…'
          }
        >
          {(id) => (
            <Select
              id={id}
              value={vehicleUsageId}
              onChange={(event) => setVehicleUsageId(event.target.value)}
              options={[
                { value: '', label: 'Non précisé' },
                ...usages.map((usage) => ({ value: usage.id, label: usage.name })),
              ]}
            />
          )}
        </Field>
        <Field label="Plaque" error={fields.plate}>
          {(id) => (
            <Input id={id} value={plate} onChange={(event) => setPlate(event.target.value)} placeholder="Ex : 1234 AB 01" />
          )}
        </Field>
        <Field label="Laveur assigné" hint="Le caissier peut laisser vide : la fiche reste à affecter.">
          {(id) => (
            <Select
              id={id}
              value={employeeId}
              onChange={(event) => setEmployeeId(event.target.value)}
              options={[
                { value: '', label: 'Non attribué' },
                ...(employees.data?.options ?? []).map((option) => ({
                  value: option.id,
                  label: option.full_name,
                })),
              ]}
            />
          )}
        </Field>
        {/* Statut : seules les flèches ouvertes sont proposées. Le graphe est
            celui du serveur (backend/order_rules.go) — s'il refusait malgré
            tout, le motif s'afficherait en clair. */}
        <Field
          label="Statut"
          hint={
            order
              ? 'Le statut ne peut pas revenir en arrière au-delà de ce que le cycle autorise.'
              : undefined
          }
        >
          {(id) => (
            <Select
              id={id}
              value={status}
              onChange={(event) => setStatus(event.target.value as WashStatus)}
              options={optionsStatut}
            />
          )}
        </Field>
      </div>

      {/* Prestations */}
      <div className="rounded-xl border border-line p-3">
        <div className="flex items-center justify-between">
          <p className="text-xs font-semibold text-ink">Prestations</p>
          <Button size="sm" icon={<Plus size={14} />} onClick={addItem}>
            Ajouter
          </Button>
        </div>
        <div className="mt-2 space-y-2">
          {items.length === 0 ? (
            <p className="py-2 text-center text-xs text-ink-muted">Aucune prestation pour l'instant.</p>
          ) : (
            items.map((item, index) => {
              const apercu = apercus[index]?.data
              const chargement = apercus[index]?.isFetching
              const sansPrix = Boolean(item.service_id) && apercu?.needs_price
              return (
                <div key={index} className="space-y-2 rounded-lg border border-line/70 p-2">
                  <div className="grid grid-cols-[1fr_auto_auto] items-center gap-2">
                    <Select
                      aria-label="Prestation"
                      value={item.service_id}
                      onChange={(event) => updateItem(index, { service_id: event.target.value, options: [] })}
                      options={[
                        { value: '', label: 'Choisir une prestation' },
                        ...(services.data?.items ?? []).map((service) => ({
                          value: service.id,
                          label: service.needs_price
                            ? `${service.name} — à tarifer`
                            : service.is_free
                              ? `${service.name} — gratuite`
                              : `${service.name} — ${service.price_xof} FCFA`,
                        })),
                      ]}
                    />
                    <Input
                      aria-label="Quantité"
                      className="w-16 text-center"
                      inputMode="numeric"
                      value={item.quantity}
                      onChange={(event) => updateItem(index, { quantity: parseQuantity(event.target.value) })}
                    />
                    <Button
                      size="sm"
                      variant="ghost"
                      aria-label="Retirer"
                      onClick={() => setItems((list) => list.filter((_, i) => i !== index))}
                    >
                      <Trash2 size={15} />
                    </Button>
                  </div>

                  {/* Le prix affiché vient du serveur, jamais du navigateur. */}
                  <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
                    {!item.service_id ? (
                      <span className="text-ink-muted">Choisissez la prestation : le prix s'affiche alors.</span>
                    ) : chargement && !apercu ? (
                      <span className="text-ink-muted">Calcul du prix…</span>
                    ) : sansPrix ? (
                      <span className="text-warning">
                        {apercu?.message ?? 'Prestation sans tarif : la vente sera refusée.'}
                      </span>
                    ) : apercu ? (
                      <span className="text-ink-muted">
                        {apercu.price_xof?.toLocaleString('fr-FR')} FCFA × {Math.max(item.quantity, 1)}
                        {supplementOptions(index) > 0
                          ? ` + ${supplementOptions(index).toLocaleString('fr-FR')} d'options`
                          : ''}{' '}
                        · <span className="text-ink">{apercu.price_source_label}</span>
                      </span>
                    ) : (
                      <span className="text-ink-muted">Prix indisponible pour l'instant.</span>
                    )}
                    <span className="tabular font-medium text-ink">
                      {totalLigne(index).toLocaleString('fr-FR')} FCFA
                    </span>
                  </div>

                  {/* Options applicables à cette ligne : la station décide du
                      supplément, la caisse ne fait que le cocher. */}
                  {(apercu?.options?.length ?? 0) > 0 ? (
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-xs text-ink-muted">Options :</span>
                      {apercu?.options.map((option) => {
                        const choisie = item.options.find((entree) => entree.option_id === option.id)
                        return (
                          <span key={option.id} className="flex items-center gap-1">
                            <button
                              type="button"
                              aria-pressed={Boolean(choisie)}
                              onClick={() => basculerOption(index, option.id, choisie ? 0 : 1)}
                              className={`chip ${choisie ? 'chip-active' : ''}`}
                            >
                              {option.name} · +{option.price_xof.toLocaleString('fr-FR')} FCFA
                            </button>
                            {choisie ? (
                              <Input
                                aria-label={`Quantité ${option.name}`}
                                className="h-7 w-14 text-center"
                                inputMode="numeric"
                                value={choisie.quantity}
                                onChange={(event) =>
                                  basculerOption(index, option.id, parseQuantity(event.target.value))
                                }
                              />
                            ) : null}
                          </span>
                        )
                      })}
                    </div>
                  ) : null}
                </div>
              )
            })
          )}
        </div>
        <div className="mt-3 space-y-1 border-t border-line pt-2 text-sm">
          <p className="flex justify-between text-ink-muted">
            <span>Sous-total (calculé par le serveur)</span>
            <span className="tabular">{subtotal.toLocaleString('fr-FR')} FCFA</span>
          </p>
          <p className="flex justify-between font-semibold text-ink">
            <span>Total</span>
            <span className="tabular">{total.toLocaleString('fr-FR')} FCFA</span>
          </p>
          {lignesSansPrix.length > 0 ? (
            <p className="pt-1 text-xs text-warning">
              {lignesSansPrix.length} ligne(s) sans tarif : la caisse refusera la fiche. Le gérant pose le prix
              dans la grille tarifaire.
            </p>
          ) : null}
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Remise (FCFA)" hint="Une remise au-delà du plafond caissier sera refusée par le serveur.">
          {(id) => (
            <Input
              id={id}
              inputMode="numeric"
              value={discount === 0 ? '' : discount}
              onChange={(event) => setDiscount(parseAmountInput(event.target.value))}
            />
          )}
        </Field>
        <Field label="Motif de la remise" hint="Obligatoire si une remise est accordée.">
          {(id) => (
            <Input id={id} value={discountReason} onChange={(event) => setDiscountReason(event.target.value)} placeholder="Ex : client fidèle" />
          )}
        </Field>
      </div>
      <Field label="Notes">
        {(id) => <Textarea id={id} value={notes} onChange={(event) => setNotes(event.target.value)} placeholder="Ex : intérieur à reprendre" />}
      </Field>

      {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}

      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={onCancel} disabled={pending}>
          Annuler
        </Button>
        <Button variant="primary" onClick={submit} loading={pending}>
          {editing ? 'Enregistrer' : 'Créer la fiche'}
        </Button>
      </div>
    </div>
  )
}
