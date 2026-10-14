import { useState } from 'react'
import {
  AlertTriangle,
  Car,
  CheckCircle2,
  Clock,
  Droplets,
  Flame,
  HandCoins,
  Play,
  RotateCw,
  ShieldAlert,
  Sparkles,
} from 'lucide-react'

import { Button, Card, EmptyState, Modal, ScreenHero, Segmented, Spinner, StatCard, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { ApiError } from '@/lib/api'
import { useAuth } from '@/hooks/useAuth'
import { formatNumber, formatTime, formatXOF } from '@/lib/format'
import {
  useReportTaskIncident,
  useUpdateTaskStatus,
  useWasherPerformance,
  useWasherTasks,
  type WasherTask,
} from '@/features/washer/api'

const QUICK_INCIDENTS = [
  'Rayure préexistante carrosserie',
  'Rétroviseur fêlé ou abîmé',
  'Impact ou bosse sur portière',
  'Objet précieux laissé dans le véhicule',
  'Taches très tenaces sur les sièges',
  'Pare-brise fissuré',
]

export default function WasherPortalPage() {
  const { me } = useAuth()
  const toast = useToast()

  const tasksQuery = useWasherTasks()
  const perfQuery = useWasherPerformance()
  const statusMutation = useUpdateTaskStatus()
  const incidentMutation = useReportTaskIncident()

  const [activeTab, setActiveTab] = useState<'pending' | 'completed' | 'all'>('pending')
  const [incidentTask, setIncidentTask] = useState<WasherTask | null>(null)
  const [incidentText, setIncidentText] = useState('')
  const [updatingId, setUpdatingId] = useState<string | null>(null)

  const tasks = tasksQuery.data ?? []
  const todayStr = new Date().toISOString().slice(0, 10)

  // Filter tasks
  // « En cours » au sens du laveur : tout ce qui n'est pas encore livré, y
  // compris le contrôle qualité et un véhicule prêt qui attend son client.
  const enCours = ['pending', 'in_progress', 'quality_check', 'ready', 'disputed']
  const pendingTasks = tasks.filter((t) => enCours.includes(t.status))
  const todayCompletedTasks = tasks.filter((t) => t.status === 'completed' && t.business_day === todayStr)
  const allTasks = tasks

  const displayedTasks =
    activeTab === 'pending' ? pendingTasks : activeTab === 'completed' ? todayCompletedTasks : allTasks

  async function handleStatusChange(
    task: WasherTask,
    nextStatus: 'in_progress' | 'quality_check' | 'completed',
  ) {
    setUpdatingId(task.id)
    try {
      await statusMutation.mutateAsync({ id: task.id, status: nextStatus })
      if (nextStatus === 'in_progress') {
        toast.success(`Véhicule ${task.plate || task.reference} pris en charge ! Bon lavage.`)
      } else if (nextStatus === 'quality_check') {
        toast.success(`Lavage terminé pour ${task.plate || task.reference} : véhicule au contrôle qualité.`)
      } else {
        toast.success(`Fiche ${task.reference} clôturée.`)
      }
    } catch (caught) {
      // Le serveur explique son refus (fiche non encaissée, transition
      // interdite…) : on le laisse parler plutôt que d'inventer une raison.
      toast.error(
        caught instanceof ApiError ? caught.message : "Impossible de mettre à jour l'avancement.",
      )
    } finally {
      setUpdatingId(null)
    }
  }

  async function handleReportIncident() {
    if (!incidentTask || !incidentText.trim()) return
    try {
      await incidentMutation.mutateAsync({ id: incidentTask.id, incident_note: incidentText.trim() })
      toast.success(`Incident consigné pour ${incidentTask.plate || incidentTask.reference}. Le gérant a été notifié.`)
      setIncidentTask(null)
      setIncidentText('')
    } catch {
      toast.error("Échec de l'enregistrement de l'incident.")
    }
  }

  const perf = perfQuery.data

  return (
    <div className="mx-auto max-w-4xl space-y-5 pb-12">
      {/* En-tête Laveur */}
      <ScreenHero
        icon={<Droplets size={22} />}
        title="Espace Laveur"
        badge={<span className="badge badge-brand">Poste lavage</span>}
        subtitle={
          <>
            Bonjour <span className="font-semibold text-ink">{me?.user.full_name}</span> · {me?.station?.name}
          </>
        }
        actions={
          <Button
            size="sm"
            variant="ghost"
            icon={<RotateCw size={14} className={tasksQuery.isFetching ? 'animate-spin' : ''} />}
            onClick={() => {
              void tasksQuery.refetch()
              void perfQuery.refetch()
            }}
          >
            Actualiser
          </Button>
        }
      />

      {/* Mes indicateurs de performance personnels */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatCard
          label="À laver"
          value={formatNumber(pendingTasks.length)}
          hint="En attente / en cours"
          icon={<Clock size={16} />}
          tone="warning"
        />
        <StatCard
          label="Faits aujourd’hui"
          value={formatNumber(perf?.washes_today ?? todayCompletedTasks.length)}
          hint="Véhicules nettoyés"
          icon={<CheckCircle2 size={16} />}
          tone="success"
          delay={0.05}
        />
        <StatCard
          label="Total ce mois"
          value={formatNumber(perf?.washes_month ?? allTasks.filter((t) => t.status === 'completed').length)}
          hint="Lavages comptabilisés"
          icon={<Flame size={16} />}
          tone="brand"
          delay={0.1}
        />
        <StatCard
          label="Mes commissions"
          value={formatXOF(perf?.commission_month_xof ?? (perf?.washes_month ?? 0) * 500)}
          hint={perf?.commission_rate_label ?? '500 FCFA / lavage'}
          icon={<HandCoins size={16} />}
          tone="warning"
          delay={0.15}
        />
      </div>

      {/* Sélecteur d'onglets : trois vues, indicateur animé partagé */}
      <Segmented
        layoutId="espace-laveur"
        size="md"
        equal
        value={activeTab}
        onChange={setActiveTab}
        options={[
          { value: 'pending', label: 'Mes tâches à faire', icon: <Clock size={15} />, count: pendingTasks.length },
          { value: 'completed', label: 'Terminés aujourd’hui', icon: <CheckCircle2 size={15} /> },
          { value: 'all', label: 'Historique', icon: <Car size={15} /> },
        ]}
      />

      {/* Liste des fiches */}
      {tasksQuery.isLoading ? (
        <div className="py-12 text-center">
          <Spinner label="Chargement de vos lavages..." />
        </div>
      ) : displayedTasks.length === 0 ? (
        <EmptyState
          icon={<Sparkles size={32} className="text-brand" />}
          title={
            activeTab === 'pending'
              ? 'Aucun véhicule en attente !'
              : activeTab === 'completed'
              ? 'Aucun lavage terminé aujourd’hui'
              : 'Aucun historique de lavage'
          }
          description={
            activeTab === 'pending'
              ? 'Tous les véhicules attribués ont été pris en charge ou terminés. Prenez une pause ou demandez au gérant de vous affecter le prochain client.'
              : 'Les lavages complétés apparaîtront ici.'
          }
        />
      ) : (
        <div className="space-y-3.5">
          {displayedTasks.map((task) => {
            const isPending = task.status === 'pending'
            const isInProgress = task.status === 'in_progress'
            const isCompleted = task.status === 'completed'
            const isQualityCheck = task.status === 'quality_check'
            const isReady = task.status === 'ready'
            const isUpdating = updatingId === task.id

            return (
              <Card
                key={task.id}
                className={`overflow-hidden transition ${
                  isInProgress
                    ? 'border-brand/70 bg-brand-soft/20 ring-1 ring-brand/30'
                    : isPending
                    ? 'border-line hover:border-line-strong'
                    : 'opacity-90'
                }`}
              >
                <div className="p-4 sm:p-5">
                  {/* Ligne haute : Plaque & Statut */}
                  <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line/70 pb-3">
                    <div className="flex items-center gap-2.5">
                      <span className="inline-flex items-center gap-1.5 rounded-lg border-2 border-ink bg-canvas px-3 py-1 font-mono text-base font-black tracking-wider text-ink shadow-xs">
                        <Car size={16} className="text-brand" />
                        {task.plate || 'SANS PLAQUE'}
                      </span>
                      <span className="text-xs font-medium text-ink-muted">#{task.reference}</span>
                    </div>

                    <div className="flex items-center gap-2">
                      {isPending && <span className="badge badge-warning">En attente de lavage</span>}
                      {isInProgress && (
                        <span className="badge badge-brand animate-pulse">
                          <Droplets size={12} /> Lavage en cours
                        </span>
                      )}
                      {isCompleted && (
                        <span className="badge badge-success">
                          <CheckCircle2 size={12} /> Terminé
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Détails véhicule et prestations */}
                  <div className="mt-3 grid gap-3 sm:grid-cols-2">
                    <div>
                      <p className="text-xs font-medium text-ink-muted">Véhicule & Client</p>
                      <p className="font-semibold text-ink">
                        {task.vehicle_category_name || 'Voiture'}
                        {task.customer_name ? ` · ${task.customer_name}` : ''}
                      </p>
                      <p className="mt-0.5 text-xs text-ink-muted">
                        Entré à {formatTime(task.created_at)} · Affecté à vous
                      </p>
                    </div>

                    <div>
                      <p className="text-xs font-medium text-ink-muted">Prestations demandées</p>
                      <ul className="mt-1 space-y-1">
                        {(task.items && task.items.length > 0
                          ? task.items
                          : [{ service_name: task.services_label || 'Lavage complet' }]
                        ).map((it, idx) => (
                          <li
                            key={idx}
                            className="flex items-center gap-2 text-xs font-medium text-ink"
                          >
                            <span className="h-1.5 w-1.5 rounded-full bg-brand" />
                            {it.service_name}
                          </li>
                        ))}
                      </ul>
                    </div>
                  </div>

                  {/* Notes & instructions particulières */}
                  {task.notes ? (
                    <div className="mt-3 rounded-lg border border-line bg-surface-muted p-2.5 text-xs text-ink">
                      <span className="font-semibold text-ink-muted">Instructions client :</span> {task.notes}
                    </div>
                  ) : null}

                  {/* Incident signalé */}
                  {task.incident_note ? (
                    <div className="mt-3 flex items-start gap-2 rounded-lg border border-warning/50 bg-warning-soft p-2.5 text-xs text-warning">
                      <AlertTriangle size={15} className="mt-0.5 shrink-0" />
                      <div>
                        <p className="font-bold">Incident / État des lieux consigné :</p>
                        <p>{task.incident_note}</p>
                      </div>
                    </div>
                  ) : null}

                  {/* Actions de travail du laveur */}
                  <div className="mt-4 flex flex-wrap items-center justify-between gap-2.5 border-t border-line/70 pt-3.5">
                    <div>
                      {!task.incident_note && !isCompleted ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          icon={<ShieldAlert size={14} className="text-warning" />}
                          onClick={() => {
                            setIncidentTask(task)
                            setIncidentText('')
                          }}
                        >
                          Signaler un incident (rayure, choc...)
                        </Button>
                      ) : null}
                    </div>

                    <div className="flex items-center gap-2">
                      {isPending && (
                        <Button
                          variant="primary"
                          loading={isUpdating}
                          icon={<Play size={16} />}
                          onClick={() => void handleStatusChange(task, 'in_progress')}
                          className="min-w-44 text-sm font-bold"
                        >
                          Prendre en charge
                        </Button>
                      )}

                      {isInProgress && (
                        <>
                          <Button
                            variant="primary"
                            loading={isUpdating}
                            icon={<CheckCircle2 size={16} />}
                            onClick={() => void handleStatusChange(task, 'quality_check')}
                            className="min-w-44 bg-success text-white hover:bg-emerald-600 text-sm font-bold"
                          >
                            Lavage terminé
                          </Button>
                          {/* Clôturer une fiche suppose qu'elle soit encaissée :
                              le serveur refuse sinon, avec le montant restant. */}
                          <Button
                            variant="ghost"
                            loading={isUpdating}
                            onClick={() => void handleStatusChange(task, 'completed')}
                          >
                            Clôturer la fiche
                          </Button>
                        </>
                      )}

                      {isQualityCheck && (
                        <Button
                          variant="ghost"
                          loading={isUpdating}
                          onClick={() => void handleStatusChange(task, 'in_progress')}
                        >
                          Reprendre le lavage
                        </Button>
                      )}

                      {isReady && (
                        <span className="text-xs font-medium text-ink-muted">
                          Véhicule prêt : remise au client par la caisse.
                        </span>
                      )}

                      {isCompleted && (
                        <span className="text-xs font-medium text-success">
                          ✓ Terminé le {task.business_day} {task.completed_at ? `à ${formatTime(task.completed_at)}` : ''}
                        </span>
                      )}
                    </div>
                  </div>
                </div>
              </Card>
            )
          })}
        </div>
      )}

      {/* Modal Signalement d'incident */}
      <Modal
        open={Boolean(incidentTask)}
        title="Signaler un incident sur le véhicule"
        description="Indiquez tout choc, rayure préexistante ou objet sensible avant ou pendant le lavage pour protéger votre responsabilité."
        onClose={() => setIncidentTask(null)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setIncidentTask(null)}>
              Annuler
            </Button>
            <Button
              variant="primary"
              loading={incidentMutation.isPending}
              disabled={!incidentText.trim()}
              icon={<ShieldAlert size={16} />}
              onClick={() => void handleReportIncident()}
            >
              Enregistrer l’incident
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="rounded-lg border border-line bg-surface-muted p-3 text-xs">
            <p className="font-semibold text-ink">
              Véhicule : {incidentTask?.plate || incidentTask?.reference} (
              {incidentTask?.vehicle_category_name})
            </p>
            <p className="text-ink-muted">Client : {incidentTask?.customer_name || 'Client de passage'}</p>
          </div>

          <div>
            <label className="label">Sélection rapide du constat :</label>
            <div className="flex flex-wrap gap-1.5">
              {QUICK_INCIDENTS.map((item) => (
                <button
                  key={item}
                  type="button"
                  onClick={() =>
                    setIncidentText((prev) => (prev ? `${prev} · ${item}` : item))
                  }
                  className="rounded-lg border border-line bg-surface px-2.5 py-1 text-xs text-ink hover:bg-surface-hover"
                >
                  + {item}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="label">Description précise de l’anomalie :</label>
            <Textarea
              rows={3}
              value={incidentText}
              onChange={(e) => setIncidentText(e.target.value)}
              placeholder="Exemple : Rayure profonde visible sur le pare-chocs arrière gauche côté passager..."
            />
            <p className="help">
              Cette mention sera visible par le caissier et le gérant sur la fiche du client et dans le journal d’audit.
            </p>
          </div>
        </div>
      </Modal>
    </div>
  )
}
