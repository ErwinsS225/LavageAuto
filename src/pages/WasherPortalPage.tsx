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

import { Button, Card, EmptyState, Modal, Spinner, Textarea } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useAuth } from '@/hooks/useAuth'
import { formatTime, formatXOF } from '@/lib/format'
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
  const pendingTasks = tasks.filter((t) => t.status === 'pending' || t.status === 'in_progress')
  const todayCompletedTasks = tasks.filter((t) => t.status === 'completed' && t.business_day === todayStr)
  const allTasks = tasks

  const displayedTasks =
    activeTab === 'pending' ? pendingTasks : activeTab === 'completed' ? todayCompletedTasks : allTasks

  async function handleStatusChange(task: WasherTask, nextStatus: 'in_progress' | 'completed') {
    setUpdatingId(task.id)
    try {
      await statusMutation.mutateAsync({ id: task.id, status: nextStatus })
      if (nextStatus === 'in_progress') {
        toast.success(`Véhicule ${task.plate || task.reference} pris en charge ! Bon lavage.`)
      } else {
        toast.success(`Lavage terminé pour ${task.plate || task.reference} ! Prêt pour inspection.`)
      }
    } catch {
      toast.error("Impossible de mettre à jour l'avancement.")
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
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-line bg-gradient-to-r from-brand-soft via-surface to-surface p-4 sm:p-5">
        <div className="flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-brand text-brand-ink shadow-sm">
            <Droplets size={26} />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold text-ink">Espace Laveur</h1>
              <span className="badge badge-brand">Poste Lavage</span>
            </div>
            <p className="text-xs text-ink-muted">
              Bonjour <span className="font-semibold text-ink">{me?.user.full_name}</span> · {me?.station?.name}
            </p>
          </div>
        </div>
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
      </div>

      {/* Mes indicateurs de performance personnels */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Card className="p-3.5">
          <div className="flex items-center gap-2 text-xs font-medium text-ink-muted">
            <Clock size={14} className="text-warning" />
            <span>À laver</span>
          </div>
          <p className="mt-1 text-2xl font-bold tabular text-ink">{pendingTasks.length}</p>
          <p className="text-[11px] text-ink-muted">En attente / en cours</p>
        </Card>

        <Card className="p-3.5">
          <div className="flex items-center gap-2 text-xs font-medium text-ink-muted">
            <CheckCircle2 size={14} className="text-success" />
            <span>Faits aujourd’hui</span>
          </div>
          <p className="mt-1 text-2xl font-bold tabular text-success">{perf?.washes_today ?? todayCompletedTasks.length}</p>
          <p className="text-[11px] text-ink-muted">Véhicules nettoyés</p>
        </Card>

        <Card className="p-3.5">
          <div className="flex items-center gap-2 text-xs font-medium text-ink-muted">
            <Flame size={14} className="text-brand" />
            <span>Total ce mois</span>
          </div>
          <p className="mt-1 text-2xl font-bold tabular text-brand">{perf?.washes_month ?? allTasks.filter((t) => t.status === 'completed').length}</p>
          <p className="text-[11px] text-ink-muted">Lavages comptabilisés</p>
        </Card>

        <Card className="p-3.5">
          <div className="flex items-center gap-2 text-xs font-medium text-ink-muted">
            <HandCoins size={14} className="text-amber-500" />
            <span>Mes commissions</span>
          </div>
          <p className="mt-1 text-2xl font-bold tabular text-amber-500">
            {formatXOF(perf?.commission_month_xof ?? (perf?.washes_month ?? 0) * 500)}
          </p>
          <p className="text-[11px] text-ink-muted">{perf?.commission_rate_label ?? '500 FCFA / lavage'}</p>
        </Card>
      </div>

      {/* Sélecteur d'onglets */}
      <div className="flex rounded-xl border border-line bg-surface-muted p-1">
        <button
          type="button"
          onClick={() => setActiveTab('pending')}
          className={`flex flex-1 items-center justify-center gap-2 rounded-lg py-2.5 text-xs font-semibold transition ${
            activeTab === 'pending' ? 'bg-surface text-ink shadow-sm' : 'text-ink-muted hover:text-ink'
          }`}
        >
          <Clock size={15} />
          <span>Mes tâches à faire</span>
          {pendingTasks.length > 0 && (
            <span className="rounded-full bg-warning px-1.5 py-0.2 text-[10px] font-bold text-black">
              {pendingTasks.length}
            </span>
          )}
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('completed')}
          className={`flex flex-1 items-center justify-center gap-2 rounded-lg py-2.5 text-xs font-semibold transition ${
            activeTab === 'completed' ? 'bg-surface text-ink shadow-sm' : 'text-ink-muted hover:text-ink'
          }`}
        >
          <CheckCircle2 size={15} />
          <span>Terminés aujourd’hui</span>
          <span className="text-[11px] text-ink-muted">({todayCompletedTasks.length})</span>
        </button>

        <button
          type="button"
          onClick={() => setActiveTab('all')}
          className={`flex flex-1 items-center justify-center gap-2 rounded-lg py-2.5 text-xs font-semibold transition ${
            activeTab === 'all' ? 'bg-surface text-ink shadow-sm' : 'text-ink-muted hover:text-ink'
          }`}
        >
          <Car size={15} />
          <span>Historique</span>
        </button>
      </div>

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
                        <Button
                          variant="primary"
                          loading={isUpdating}
                          icon={<CheckCircle2 size={16} />}
                          onClick={() => void handleStatusChange(task, 'completed')}
                          className="min-w-44 bg-success text-white hover:bg-emerald-600 text-sm font-bold"
                        >
                          Terminer le lavage
                        </Button>
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
