/**
 * Formulaire de création / modification d'une fiche de lavage.
 * Gère client, véhicule, plaque, laveur, prestations, remise et statut.
 */
import { useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { Button, Field, Input, Select, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useCreateWashOrder, useUpdateWashOrder, type WashOrderDetail } from '@/features/wash-orders/api'
import { useCustomersOptions } from '@/features/customers/api'
import { useEmployeeOptions } from '@/features/employees/api'
import { useVehicleCategories } from '@/features/vehicle-categories/api'
import { useStationServices } from '@/features/services/api'
import { ApiError } from '@/lib/api'
import { parseAmountInput } from '@/lib/format'
import type { WashOrder, WashStatus } from '@/types'

interface FormItem {
  service_id: string
  quantity: number
  unit_price: number
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
  const vehicles = useVehicleCategories()
  const services = useStationServices({ station_id: stationId || undefined })

  const [customerId, setCustomerId] = useState(order?.customer_id ?? '')
  const [customerName, setCustomerName] = useState(order?.customer_name ?? '')
  const [vehicleCategoryId, setVehicleCategoryId] = useState(order?.vehicle_category_id ?? '')
  const [plate, setPlate] = useState(order?.plate ?? '')
  const [employeeId, setEmployeeId] = useState(order?.employee_id ?? '')
  const [status, setStatus] = useState<WashStatus>(order?.status ?? 'in_progress')
  const [discount, setDiscount] = useState(order?.discount_xof ?? 0)
  const [discountReason, setDiscountReason] = useState(order?.discount_reason ?? '')
  const [notes, setNotes] = useState(order?.notes ?? '')
  const [items, setItems] = useState<FormItem[]>(
    detail?.order.items?.map((item) => ({
      service_id: item.service_id ?? '',
      quantity: item.quantity,
      unit_price: item.unit_price_xof,
    })) ?? [],
  )
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})

  const subtotal = items.reduce((total, item) => total + item.unit_price * Math.max(item.quantity, 1), 0)
  const total = Math.max(subtotal - discount, 0)

  function addItem() {
    const service = services.data?.items.find((entry) => entry.is_active)
    setItems((list) => [
      ...list,
      { service_id: service?.id ?? '', quantity: 1, unit_price: service?.price_xof ?? 0 },
    ])
  }

  function updateItem(index: number, patch: Partial<FormItem>) {
    setItems((list) => list.map((item, i) => (i === index ? { ...item, ...patch } : item)))
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
      plate: plate || undefined,
      employee_id: employeeId || undefined,
      discount_xof: discount || undefined,
      discount_reason: discountReason || undefined,
      notes: notes || undefined,
      status,
      items: items.map((item) => ({
        service_id: item.service_id,
        quantity: item.quantity,
        unit_price_xof: item.unit_price || undefined,
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
        <Field label="Catégorie de véhicule">
          {(id) => (
            <Select
              id={id}
              value={vehicleCategoryId}
              onChange={(event) => setVehicleCategoryId(event.target.value)}
              options={[
                { value: '', label: 'Voiture (défaut)' },
                ...(vehicles.data?.items ?? []).map((category) => ({
                  value: category.id,
                  label: category.name,
                })),
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
        <Field label="Statut">
          {(id) => (
            <Select
              id={id}
              value={status}
              onChange={(event) => setStatus(event.target.value as WashStatus)}
              options={[
                { value: 'in_progress', label: 'En cours' },
                { value: 'completed', label: 'Terminé' },
                { value: 'pending', label: 'En attente' },
              ]}
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
            items.map((item, index) => (
              <div key={index} className="grid grid-cols-[1fr_auto_auto_auto] items-center gap-2">
                <Select
                  aria-label="Prestation"
                  value={item.service_id}
                  onChange={(event) => {
                    const found = services.data?.items.find((entry) => entry.id === event.target.value)
                    updateItem(index, { service_id: event.target.value, unit_price: found?.price_xof ?? item.unit_price })
                  }}
                  options={[
                    { value: '', label: 'Prestation libre' },
                    ...(services.data?.items ?? []).map((service) => ({
                      value: service.id,
                      label: `${service.name} — ${service.price_xof} FCFA`,
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
                <Input
                  aria-label="Prix unitaire"
                  className="w-24 text-right"
                  inputMode="numeric"
                  value={item.unit_price}
                  onChange={(event) => updateItem(index, { unit_price: parseAmountInput(event.target.value) })}
                />
                <Button size="sm" variant="ghost" aria-label="Retirer" onClick={() => setItems((list) => list.filter((_, i) => i !== index))}>
                  <Trash2 size={15} />
                </Button>
              </div>
            ))
          )}
        </div>
        <div className="mt-3 space-y-1 border-t border-line pt-2 text-sm">
          <p className="flex justify-between text-ink-muted">
            <span>Sous-total</span>
            <span className="tabular">{subtotal} FCFA</span>
          </p>
          <p className="flex justify-between font-semibold text-ink">
            <span>Total</span>
            <span className="tabular">{total} FCFA</span>
          </p>
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
