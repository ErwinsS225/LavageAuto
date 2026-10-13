/**
 * Catalogue des prestations : tarifs par station.
 */
import { useState } from 'react'
import { Pencil, Plus, Sparkles, Trash2 } from 'lucide-react'

import { Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, Input, Modal, PageHeader, Select, Spinner, StatCard } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useCreateService, useDisableService, useStationServices, useUpdateService } from '@/features/services/api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import { SERVICE_CATEGORIES } from '@/lib/constants'
import { formatXOF, parseAmountInput } from '@/lib/format'
import type { Service } from '@/types'

interface ServiceForm {
  name: string
  category: string
  price: number
  duration: number
  description: string
}

export default function ServicesPage() {
  const toast = useToast()
  const { can } = useAuth()
  const { stationId } = useStation()
  usePageTitle('Prestations')

  const servicesQuery = useStationServices({ station_id: stationId || undefined })
  const createMutation = useCreateService()
  const updateMutation = useUpdateService()
  const disableMutation = useDisableService()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Service | null>(null)
  const [disabling, setDisabling] = useState<Service | null>(null)
  const [form, setForm] = useState<ServiceForm>({ name: '', category: 'lavage', price: 0, duration: 30, description: '' })
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})

  const canManage = can('service:manage')
  const pending = createMutation.isPending || updateMutation.isPending
  const services = (servicesQuery.data?.items ?? []).filter((service) => service.is_active)
  const disabledCount = (servicesQuery.data?.items ?? []).length - services.length

  function openForm(service?: Service) {
    setEditing(service ?? null)
    setForm({
      name: service?.name ?? '',
      category: service?.category ?? 'lavage',
      price: service?.price_xof ?? 0,
      duration: service?.duration_minutes ?? 30,
      description: service?.description ?? '',
    })
    setError('')
    setFields({})
    setFormOpen(true)
  }

  async function submit() {
    setError('')
    setFields({})
    try {
      const payload = {
        station_id: stationId || undefined,
        name: form.name,
        category: form.category,
        price_xof: form.price,
        duration_minutes: form.duration,
        description: form.description,
      }
      if (editing) {
        await updateMutation.mutateAsync({ id: editing.id, ...payload })
        toast.success('Prestation mise à jour.')
      } else {
        await createMutation.mutateAsync(payload)
        toast.success('Prestation créée.')
      }
      setFormOpen(false)
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError("L'enregistrement a échoué.")
      }
    }
  }

  async function disable() {
    if (!disabling) return
    try {
      await disableMutation.mutateAsync(disabling.id)
      toast.success('Prestation désactivée : elle reste dans l’historique des fiches.')
      setDisabling(null)
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : 'La désactivation a échoué.')
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Prestations"
        subtitle="Le catalogue utilisé par les caissiers pour créer les fiches de lavage : nom, catégorie et prix."
        actions={
          canManage ? (
            <Button variant="primary" icon={<Plus size={16} />} onClick={() => openForm()}>
              Ajouter une prestation
            </Button>
          ) : null
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard label="Prestations actives" value={String(services.length)} icon={<Sparkles size={18} />} />
        <StatCard
          label="Prix minimum"
          value={services.length ? formatXOF(Math.min(...services.map((service) => service.price_xof))) : '—'}
        />
        <StatCard
          label="Prix maximum"
          value={services.length ? formatXOF(Math.max(...services.map((service) => service.price_xof))) : '—'}
          tone="success"
        />
      </div>

      <Card>
        {servicesQuery.isLoading ? (
          <Spinner label="Chargement du catalogue…" />
        ) : servicesQuery.isError ? (
          <ErrorState message="Impossible de charger les prestations." onRetry={() => void servicesQuery.refetch()} />
        ) : services.length === 0 ? (
          <EmptyState
            icon={<Sparkles size={22} />}
            title="Aucune prestation"
            description="Créez le catalogue : lavage simple, complet, polissage… Les caissiers choisiront ici lors des fiches."
            action={
              canManage ? (
                <Button variant="primary" icon={<Plus size={16} />} onClick={() => openForm()}>
                  Ajouter une prestation
                </Button>
              ) : null
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Prestation</th>
                  <th>Catégorie</th>
                  <th className="text-right">Prix</th>
                  <th className="text-right">Durée</th>
                  {canManage ? <th /> : null}
                </tr>
              </thead>
              <tbody>
                {services.map((service) => (
                  <tr key={service.id}>
                    <td>
                      <span className="font-medium text-ink">{service.name}</span>
                      {service.description ? <span className="block text-xs text-ink-muted">{service.description}</span> : null}
                    </td>
                    <td>
                      <span className="badge badge-neutral">{service.category}</span>
                    </td>
                    <td className="tabular text-right font-medium">{formatXOF(service.price_xof)}</td>
                    <td className="tabular text-right text-ink-muted">{service.duration_minutes} min</td>
                    {canManage ? (
                      <td>
                        <div className="flex justify-end gap-1">
                          <Button size="sm" variant="ghost" title="Modifier" onClick={() => openForm(service)}>
                            <Pencil size={15} />
                          </Button>
                          <Button size="sm" variant="ghost" title="Désactiver" onClick={() => setDisabling(service)}>
                            <Trash2 size={15} />
                          </Button>
                        </div>
                      </td>
                    ) : null}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {disabledCount > 0 ? (
        <p className="text-xs text-ink-muted">{disabledCount} prestation(s) désactivée(s) — conservées dans l’historique.</p>
      ) : null}

      <Modal
        open={formOpen}
        title={editing ? `Modifier ${editing.name}` : 'Nouvelle prestation'}
        onClose={() => setFormOpen(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setFormOpen(false)} disabled={pending}>
              Annuler
            </Button>
            <Button variant="primary" onClick={submit} loading={pending}>
              Enregistrer
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Nom" required error={fields.name}>
            {(id) => (
              <Input
                id={id}
                value={form.name}
                onChange={(event) => setForm({ ...form, name: event.target.value })}
                placeholder="Ex : Lavage complet berline"
                invalid={Boolean(fields.name)}
              />
            )}
          </Field>
          <Field label="Catégorie">
            {(id) => (
              <Select
                id={id}
                value={form.category}
                onChange={(event) => setForm({ ...form, category: event.target.value })}
                options={SERVICE_CATEGORIES.map((category) => ({ value: category.id, label: category.label }))}
              />
            )}
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Prix (FCFA)" required error={fields.price_xof}>
              {(id) => (
                <Input
                  id={id}
                  inputMode="numeric"
                  value={form.price === 0 ? '' : form.price}
                  onChange={(event) => setForm({ ...form, price: parseAmountInput(event.target.value) })}
                  placeholder="5000"
                  invalid={Boolean(fields.price_xof)}
                />
              )}
            </Field>
            <Field label="Durée (minutes)">
              {(id) => (
                <Input
                  id={id}
                  inputMode="numeric"
                  value={form.duration}
                  onChange={(event) => setForm({ ...form, duration: parseAmountInput(event.target.value) })}
                />
              )}
            </Field>
          </div>
          <Field label="Description">
            {(id) => (
              <Input
                id={id}
                value={form.description}
                onChange={(event) => setForm({ ...form, description: event.target.value })}
                placeholder="Ex : extérieur + intérieur + vitres"
              />
            )}
          </Field>
          {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
        </div>
      </Modal>

      <ConfirmDialog
        open={Boolean(disabling)}
        title={`Désactiver « ${disabling?.name} » ?`}
        message="Elle n'apparaîtra plus dans les formulaires, mais reste visible dans les fiches déjà créées."
        confirmLabel="Désactiver"
        tone="danger"
        loading={disableMutation.isPending}
        onCancel={() => setDisabling(null)}
        onConfirm={() => void disable()}
      />
    </div>
  )
}
