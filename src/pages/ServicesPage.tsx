/**
 * Catalogue des prestations.
 *
 * Une règle tient cet écran : **aucun prix ne se saisit ici**. Le catalogue
 * décrit ce que la station propose (nom, durée, contenu, véhicules concernés,
 * gratuité, devis) ; les tarifs se décident dans la grille tarifaire, avec leur
 * motif et leur historique. Un prix tapé ici échapperait à cette trace — et
 * finirait par contredire la grille.
 */
import { Pencil, Plus, SlidersHorizontal, Sparkles, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'

import {
  Badge,
  Button,
  Card,
  CardHeader,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  Field,
  Input,
  Modal,
  PageHeader,
  Select,
  SkeletonRows,
  StatCard,
  Switch,
} from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useCreateService, useDisableService, useStationServices, useUpdateService } from '@/features/services/api'
import { useServiceCompatibilities, useSetServiceCompatibilities, useServiceTemplates } from '@/features/services/pricing-api'
import { useAuth } from '@/hooks/useAuth'
import { usePageTitle } from '@/hooks/usePageTitle'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import type { Service } from '@/types'

interface ServiceForm {
  name: string
  description: string
  templateCode: string
  duration: number
  isFree: boolean
  isQuoteOnly: boolean
}

const FORM_VIDE: ServiceForm = {
  name: '',
  description: '',
  templateCode: '',
  duration: 30,
  isFree: false,
  isQuoteOnly: false,
}

export default function ServicesPage() {
  const toast = useToast()
  const { can } = useAuth()
  const { stationId, station } = useStation()
  usePageTitle('Prestations')

  const servicesQuery = useStationServices({ station_id: stationId || undefined })
  const modeles = useServiceTemplates(stationId || undefined)
  const createMutation = useCreateService()
  const updateMutation = useUpdateService()
  const disableMutation = useDisableService()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Service | null>(null)
  const [disabling, setDisabling] = useState<Service | null>(null)
  const [vehiculesDe, setVehiculesDe] = useState<Service | null>(null)
  const [form, setForm] = useState<ServiceForm>(FORM_VIDE)
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})

  const canManage = can('service:manage')
  const pending = createMutation.isPending || updateMutation.isPending
  const services = servicesQuery.data?.items ?? []
  const actives = services.filter((service) => service.is_active)
  const inactives = services.length - actives.length
  const aTarifer = actives.filter((service) => service.needs_price).length
  const gratuites = actives.filter((service) => service.is_free).length

  function openForm(service?: Service) {
    setEditing(service ?? null)
    setForm(
      service
        ? {
            name: service.name,
            description: service.description,
            templateCode: service.template_code,
            duration: service.duration_minutes,
            isFree: service.is_free,
            isQuoteOnly: service.is_quote_only,
          }
        : FORM_VIDE,
    )
    setError('')
    setFields({})
    setFormOpen(true)
  }

  /** Reprendre un modèle remplit l'écran : l'exploitant garde le dernier mot. */
  function reprendreModele(code: string) {
    const modele = (modeles.data?.templates ?? []).find((item) => item.code === code)
    if (!modele) {
      setForm((actuel) => ({ ...actuel, templateCode: '' }))
      return
    }
    setForm({
      name: modele.name,
      description: modele.description,
      templateCode: modele.code,
      duration: modele.duration_minutes,
      isFree: false,
      isQuoteOnly: modele.is_quote_only,
    })
  }

  async function submit() {
    setError('')
    setFields({})
    try {
      if (editing) {
        await updateMutation.mutateAsync({
          id: editing.id,
          name: form.name,
          description: form.description,
          duration_minutes: form.duration,
          is_free: form.isFree,
          is_quote_only: form.isQuoteOnly,
        })
        toast.success('Prestation mise à jour.')
      } else {
        await createMutation.mutateAsync({
          station_id: stationId || undefined,
          name: form.name,
          description: form.description,
          duration_minutes: form.duration,
          is_free: form.isFree,
          is_quote_only: form.isQuoteOnly,
          template_code: form.templateCode || undefined,
        })
        toast.success(
          form.isFree
            ? 'Prestation ajoutée : elle se vend à zéro.'
            : 'Prestation ajoutée au catalogue — elle reste à tarifer dans la grille.',
        )
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
        subtitle={
          station
            ? `Le catalogue de ${station.name}. Les prix se règlent dans la grille tarifaire.`
            : 'Le catalogue utilisé par les caissiers. Les prix se règlent dans la grille tarifaire.'
        }
        actions={
          <div className="flex flex-wrap gap-2">
            <Link to="/tarifs">
              <Button variant="ghost" icon={<SlidersHorizontal size={16} />}>
                Grille tarifaire
              </Button>
            </Link>
            {canManage ? (
              <Button variant="primary" icon={<Plus size={16} />} onClick={() => openForm()}>
                Ajouter une prestation
              </Button>
            ) : null}
          </div>
        }
      />

      <div className="grid gap-4 sm:grid-cols-4">
        <StatCard label="Prestations actives" value={String(actives.length)} icon={<Sparkles size={18} />} />
        <StatCard
          label="À tarifer"
          value={String(aTarifer)}
          tone={aTarifer > 0 ? 'warning' : 'success'}
          hint={aTarifer > 0 ? 'Invendables tant qu’aucun prix ne les couvre.' : 'Toutes sont vendables.'}
        />
        <StatCard label="Gratuites" value={String(gratuites)} hint="Décision explicite." />
        <StatCard label="Désactivées" value={String(inactives)} hint="Conservées dans l’historique." />
      </div>

      <Card>
        <CardHeader
          title="Catalogue de la station"
          subtitle="Ce que la caisse peut proposer. Le prix affiché est celui que le serveur appliquera."
          actions={
            aTarifer > 0 ? <Badge tone="warning">{aTarifer} à tarifer</Badge> : <Badge tone="success">Tout est tarifé</Badge>
          }
        />
        {servicesQuery.isLoading ? (
          <SkeletonRows rows={6} />
        ) : servicesQuery.isError ? (
          <ErrorState message="Impossible de charger les prestations." onRetry={() => void servicesQuery.refetch()} />
        ) : actives.length === 0 ? (
          <EmptyState
            icon={<Sparkles size={22} />}
            title="Aucune prestation"
            description="Reprenez une prestation de référence ou décrivez la vôtre. Le tarif se posera ensuite, dans la grille."
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
                  <th>Famille</th>
                  <th className="text-right">Prix appliqué</th>
                  <th className="text-right">Durée</th>
                  {canManage ? <th /> : null}
                </tr>
              </thead>
              <tbody>
                {actives.map((service) => (
                  <tr key={service.id}>
                    <td>
                      <span className="font-medium text-ink">{service.name}</span>
                      {service.description ? (
                        <span className="block text-xs text-ink-muted">{service.description}</span>
                      ) : null}
                    </td>
                    <td>
                      <Badge tone="neutral">{service.family_name || service.family_code || '—'}</Badge>
                      {service.template_code ? (
                        <span className="block text-xs text-ink-muted">référence</span>
                      ) : null}
                    </td>
                    <td className="tabular text-right">
                      {service.is_free ? (
                        <Badge tone="info">gratuite</Badge>
                      ) : service.is_quote_only ? (
                        <Badge tone="neutral">sur devis</Badge>
                      ) : service.needs_price ? (
                        <Badge tone="warning">à tarifer</Badge>
                      ) : service.price_xof > 0 ? (
                        <span className="font-medium text-ink">
                          {service.price_xof.toLocaleString('fr-FR')} FCFA
                          <span className="block text-xs font-normal text-ink-muted">tarif du catalogue</span>
                        </span>
                      ) : (
                        // Un prix par véhicule ne s'affiche pas « au niveau » :
                        // la caisse le résoudra, la grille le montre.
                        <Badge tone="success">tarif par véhicule</Badge>
                      )}
                    </td>
                    <td className="tabular text-right text-ink-muted">{service.duration_minutes} min</td>
                    {canManage ? (
                      <td>
                        <div className="flex justify-end gap-1">
                          <Button size="sm" variant="ghost" title="Véhicules concernés" onClick={() => setVehiculesDe(service)}>
                            <SlidersHorizontal size={15} />
                          </Button>
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
        <p className="border-t border-line px-4 py-3 text-xs text-ink-muted">
          Un prix ne se saisit pas ici : passez par la <Link to="/tarifs" className="link-soft font-medium">grille tarifaire</Link>. Le
          serveur reste seul juge du montant d’une fiche, et il en indique toujours l’origine.
        </p>
      </Card>

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
          {!editing ? (
            <Field
              label="Reprendre une prestation de référence"
              hint="Le contenu et la durée sont décrits — jamais le prix : il reste le vôtre."
            >
              {(id) => (
                <Select
                  id={id}
                  value={form.templateCode}
                  onChange={(event) => reprendreModele(event.target.value)}
                  options={(modeles.data?.templates ?? []).map((modele) => ({
                    value: modele.code,
                    label: `${modele.name} — ${modele.family_name}`,
                  }))}
                  placeholder="Description libre, sans modèle"
                />
              )}
            </Field>
          ) : null}
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
          <div className="grid grid-cols-2 gap-3">
            <Field label="Durée (minutes)">
              {(id) => (
                <Input
                  id={id}
                  inputMode="numeric"
                  value={form.duration}
                  onChange={(event) => setForm({ ...form, duration: Number(event.target.value) || 0 })}
                />
              )}
            </Field>
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
          </div>
          <Switch
            checked={form.isFree}
            label="Prestation gratuite"
            hint="Elle se vend à zéro, par décision — aucun tarif ne peut la couvrir."
            onChange={(valeur) => setForm({ ...form, isFree: valeur })}
          />
          <Switch
            checked={form.isQuoteOnly}
            label="Chiffrée sur devis"
            hint="Céramique, rénovation : elle ne se vend pas au comptoir."
            onChange={(valeur) => setForm({ ...form, isQuoteOnly: valeur })}
          />
          <p className="rounded-lg border border-line bg-surface-muted px-3 py-2 text-xs text-ink-muted">
            Ce formulaire ne demande pas de prix : la prestation naît « à tarifer » et devient vendable dès que la
            grille la couvre.
          </p>
          {error ? (
            <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p>
          ) : null}
        </div>
      </Modal>

      {vehiculesDe ? (
        <VehiculesPrestation service={vehiculesDe} onClose={() => setVehiculesDe(null)} />
      ) : null}

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

/** À quels véhicules une prestation s'applique (vide = tous ceux acceptés). */
function VehiculesPrestation({ service, onClose }: { service: Service; onClose: () => void }) {
  const toast = useToast()
  const existantes = useServiceCompatibilities(service.id)
  const enregistrement = useSetServiceCompatibilities()
  const choisies = new Set(existantes.data?.categories ?? [])

  async function basculer(code: string, actif: boolean) {
    const suivantes = new Set(choisies)
    if (actif) suivantes.add(code)
    else suivantes.delete(code)
    try {
      await enregistrement.mutateAsync({ serviceId: service.id, categories: [...suivantes] })
      toast.success(
        suivantes.size === 0
          ? 'Prestation proposée pour tous vos véhicules.'
          : 'Véhicules enregistrés pour cette prestation.',
      )
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'enregistrement a échoué.")
    }
  }

  return (
    <Modal
      open
      title={`Véhicules concernés par « ${service.name} »`}
      onClose={onClose}
      footer={
        <Button variant="ghost" onClick={onClose}>
          Fermer
        </Button>
      }
    >
      {(existantes.data?.categories_disponibles ?? []).length === 0 ? (
        <p className="text-sm text-ink-muted">
          Activez d’abord vos types de véhicules (étape « Catégories ») : une prestation ne peut pas viser un véhicule
          que la station ne lave pas.
        </p>
      ) : (
        <div className="space-y-2">
          {(existantes.data?.categories_disponibles ?? []).map((categorie) => (
            <Switch
              key={categorie.code}
              checked={choisies.has(categorie.code)}
              label={categorie.name}
              onChange={(valeur) => void basculer(categorie.code, valeur)}
            />
          ))}
          <p className="text-xs text-ink-muted">
            Aucun véhicule coché = la prestation est proposée pour tous ceux que la station accepte.
          </p>
        </div>
      )}
    </Modal>
  )
}
