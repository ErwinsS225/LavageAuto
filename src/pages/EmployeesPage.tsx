import { useState } from 'react'
import { Banknote, KeyRound, Pencil, Plus, ShieldCheck, UserCheck, UserRound, UserX } from 'lucide-react'

import { Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, Input, Modal, PageHeader, Select, Spinner, StatCard } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import { useEmployeeActivity, useCreateEmployee, useEmployees, useResetEmployeePassword, useUpdateEmployee, type EmployeeCreated, type EmployeeInput } from '@/features/employees/api'
import { useAuth } from '@/hooks/useAuth'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import {
  commissionRateLabel,
  EMPLOYEE_COMMISSION_MODES,
  ROLES,
  roleLabel,
  washStatusLabel,
  washStatusTone,
} from '@/lib/constants'
import {
  addDays,
  formatDate,
  formatDateTime,
  formatNumber,
  formatXOF,
  parseAmountInput,
  startOfMonth,
  today,
} from '@/lib/format'
import type { CommissionMode, Employee, Role } from '@/types'

/**
 * Choix du formulaire de commission : « station » (le cas normal) ou une
 * exception propre au compte. Le serveur traduit « station » en NULL.
 */
type CommissionChoice = CommissionMode | 'station'

/** Employés : rôles, activation, activité enregistrée et mots de passe. */
export default function EmployeesPage() {
  const toast = useToast()
  const { can, me } = useAuth()
  const { stationId } = useStation()

  const [status, setStatus] = useState<'active' | 'inactive' | 'all'>('active')
  const employeesQuery = useEmployees({ status, station_id: undefined })
  const createMutation = useCreateEmployee()
  const updateMutation = useUpdateEmployee()
  const resetMutation = useResetEmployeePassword()

  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState<Employee | null>(null)
  const [form, setForm] = useState<{
    full_name: string
    residence: string
    phone: string
    role: Role
    commission_mode: CommissionChoice
    commission_per_wash: number
    commission_percent: number
  }>({
    full_name: '',
    residence: '',
    phone: '',
    role: 'cashier',
    commission_mode: 'station',
    commission_per_wash: 0,
    commission_percent: 0,
  })
  const [error, setError] = useState('')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [created, setCreated] = useState<EmployeeCreated | null>(null)
  const [reseted, setReseted] = useState<{ name: string; password: string } | null>(null)
  const [togglingEmployee, setTogglingEmployee] = useState<Employee | null>(null)
  const [activityId, setActivityId] = useState<string | null>(null)
  const [activityFrom, setActivityFrom] = useState(startOfMonth(today()))
  const [activityTo, setActivityTo] = useState(today())

  const activityQuery = useEmployeeActivity(activityId, activityFrom, activityTo)

  const canManage = can('employee:manage')
  const employees = employeesQuery.data?.items ?? []
  const subscription = me?.subscription
  const employeeLimit = subscription?.max_employees ?? 0
  const employeeUsage = subscription?.usage?.employees ?? employees.length
  const limitReached = employeeLimit > 0 && employeeUsage >= employeeLimit

  function openForm(employee?: Employee) {
    setEditing(employee ?? null)
    setForm({
      full_name: employee?.full_name ?? '',
      residence: employee?.residence ?? '',
      phone: employee?.phone ?? '',
      role: employee?.role ?? 'cashier',
      // `null` côté serveur = « suit le barème de la station », ce que le
      // formulaire traduit par le choix « station ».
      commission_mode: employee?.commission_mode ?? 'station',
      commission_per_wash: Number(employee?.commission_per_wash_xof ?? 0),
      commission_percent: Number(employee?.commission_percent ?? 0),
    })
    setError('')
    setFields({})
    setFormOpen(true)
  }

  async function submit() {
    setError('')
    setFields({})
    // Barème envoyé au serveur : « station » (valeur par défaut du formulaire)
    // se traduit par commission_mode = undefined, ce qui laisse le serveur
    // appliquer le barème de la station (-NULL). Un mode propre ('none',
    // 'per_wash', 'percent') est envoyé tel quel.
    const bareme: EmployeeInput = {
      commission_mode: form.commission_mode === 'station' ? undefined : form.commission_mode,
      commission_per_wash_xof: form.commission_per_wash,
      commission_percent: form.commission_percent,
    }
    try {
      if (editing) {
        await updateMutation.mutateAsync({
          id: editing.id,
          full_name: form.full_name,
          phone: form.phone,
          role: form.role,
          station_id: stationId || undefined,
          ...bareme,
        })
        toast.success('Employé mis à jour.')
        setFormOpen(false)
        return
      }
      const result = await createMutation.mutateAsync({
        full_name: form.full_name,
        residence: form.email,
        phone: form.phone,
        role: form.role,
        station_id: stationId || undefined,
        ...bareme,
      })
      setFormOpen(false)
      if (result.temporary_password) {
        setCreated(result)
      } else {
        toast.success('Employé créé.')
      }
    } catch (caught) {
      if (caught instanceof ApiError) {
        setError(caught.message)
        setFields(caught.fields)
      } else {
        setError("L'enregistrement a échoué.")
      }
    }
  }

  async function toggleActive() {
    if (!togglingEmployee) return
    try {
      await updateMutation.mutateAsync({ id: togglingEmployee.id, is_active: !togglingEmployee.is_active })
      toast.success(togglingEmployee.is_active ? 'Compte désactivé : il ne peut plus se connecter.' : 'Compte réactivé.')
      setTogglingEmployee(null)
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "L'opération a échoué.")
    }
  }

  async function resetPassword(employee: Employee) {
    try {
      const result = await resetMutation.mutateAsync(employee.id)
      setReseted({ name: employee.full_name, password: result.temporary_password })
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : 'La réinitialisation a échoué.')
    }
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Employés"
        subtitle="Chaque personne se connecte avec son propre compte : les lavages et encaissements sont tracés à son nom."
        actions={
          canManage ? (
            <Button variant="primary" icon={<Plus size={16} />} onClick={() => openForm()} disabled={limitReached}>
              Ajouter un employé
            </Button>
          ) : null
        }
      />

      {canManage && limitReached ? (
        <Card className="border-warning/40 bg-warning-soft/60 p-4 text-sm text-warning">
          <p className="font-medium">Limite de votre formule atteinte</p>
          <p className="mt-0.5">
            La formule « {subscription?.plan_name} » autorise {formatNumber(employeeLimit)} compte(s) ({formatNumber(employeeUsage)} utilisé(s)).
            Désactivez un employé ou passez à une formule supérieure depuis la page Abonnement.
          </p>
        </Card>
      ) : null}

      {/* Guide des 3 rôles opérationnels */}
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="rounded-xl border border-line bg-surface p-3.5 text-xs shadow-xs">
          <div className="flex items-center gap-1.5 font-bold text-ink">
            <span className="text-base">👔</span> Gérant
          </div>
          <p className="mt-1 text-ink-muted leading-relaxed">
            Supervision opérationnelle : tableau de bord, catalogue des tarifs, approbation des dépenses, contrôle des clôtures de caisse, affectation des laveurs et journal d'audit.
          </p>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5 text-xs shadow-xs">
          <div className="flex items-center gap-1.5 font-bold text-ink">
            <span className="text-base">🏷️</span> Caissier / Caissière
          </div>
          <p className="mt-1 text-ink-muted leading-relaxed">
            Accueil clients, création des fiches de lavage, encaissements (espèces, mobile money), émission des reçus, ouverture et clôture de sa caisse avec comptage de fin de shift.
          </p>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5 text-xs shadow-xs">
          <div className="flex items-center gap-1.5 font-bold text-ink">
            <span className="text-base">🧼</span> Laveur / Laveuse
          </div>
          <p className="mt-1 text-ink-muted leading-relaxed">
            Interface mobile atelier : prise en charge des véhicules attribués, mise à jour de l'état d'avancement (en cours, terminé), signalement d'incidents et suivi des commissions.
          </p>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard
          label="Comptes actifs"
          value={formatNumber(employees.filter((employee) => employee.is_active).length)}
          hint={`${employees.length} au total`}
          icon={<UserCheck size={18} />}
        />
        <StatCard
          label="Lavages enregistrés ce mois"
          value={formatNumber(employees.reduce((total, employee) => total + employee.washes_month, 0))}
          icon={<ShieldCheck size={18} />}
        />
        <StatCard
          label="Encaissements du mois"
          value={formatXOF(employees.reduce((total, employee) => total + employee.collected_month_xof, 0))}
          tone="success"
          icon={<Banknote size={18} />}
        />
      </div>

      <Card>
        <div className="flex flex-wrap items-center justify-between gap-3 p-4">
          <Select
            aria-label="Statut"
            className="w-44"
            value={status}
            onChange={(event) => setStatus(event.target.value as 'active' | 'inactive' | 'all')}
            options={[
              { value: 'active', label: 'Employés actifs' },
              { value: 'inactive', label: 'Comptes désactivés' },
              { value: 'all', label: 'Tous' },
            ]}
          />
          <p className="text-xs text-ink-muted">Les rôles Propiétaire, Gérant et Caissier sont vérifiés côté serveur.</p>
        </div>
      </Card>

      <Card>
        {employeesQuery.isLoading ? (
          <Spinner label="Chargement de l’équipe…" />
        ) : employeesQuery.isError ? (
          <ErrorState message="Impossible de charger les employés." onRetry={() => void employeesQuery.refetch()} />
        ) : employees.length === 0 ? (
          <EmptyState
            icon={<UserRound size={22} />}
            title="Aucun employé"
            description="Ajoutez les personnes qui travaillent à la station : chaque compte a son rôle et ses droits."
            action={
              canManage ? (
                <Button variant="primary" icon={<Plus size={16} />} onClick={() => openForm()}>
                  Ajouter un employé
                </Button>
              ) : null
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Employé</th>
                  <th>Rôle</th>
                  <th>Commission</th>
                  <th className="text-right">Lavages (mois)</th>
                  <th className="text-right">CA généré (mois)</th>
                  <th className="text-right">Encaissé (mois)</th>
                  <th>Dernière connexion</th>
                  <th>État</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {employees.map((employee) => (
                  <tr key={employee.id}>
                    <td>
                      <button type="button" className="cursor-pointer text-left" onClick={() => setActivityId(employee.id)}>
                        <span className="font-medium text-ink hover:text-brand">{employee.full_name}</span>
                        <span className="block text-xs text-ink-muted">
                          {employee.email}
                          {employee.phone ? ` · ${employee.phone}` : ''}
                        </span>
                      </button>
                    </td>
                    <td>
                      <span className={`badge ${employee.role === 'owner' ? 'badge-brand' : 'badge-neutral'}`}>
                        {roleLabel(employee.role)}
                      </span>
                    </td>
                    <td className="whitespace-nowrap">
                      {/* Une exception posée sur le compte est signalée : sans ce
                          repère, personne ne comprend pourquoi deux laveurs à
                          même activité ne touchent pas la même chose. */}
                      {employee.commission_mode ? (
                        <span className="badge badge-brand">
                          {commissionRateLabel(
                            employee.commission_mode,
                            employee.commission_per_wash_xof,
                            employee.commission_percent,
                          )}
                        </span>
                      ) : (
                        <span className="badge badge-neutral">Barème station</span>
                      )}
                    </td>
                    <td className="tabular text-right">{formatNumber(employee.washes_month)}</td>
                    <td className="tabular text-right">{formatXOF(employee.revenue_month_xof)}</td>
                    <td className="tabular text-right text-success">{formatXOF(employee.collected_month_xof)}</td>
                    <td className="whitespace-nowrap text-ink-muted">
                      {employee.last_login_at ? formatDateTime(employee.last_login_at) : 'Jamais connecté'}
                    </td>
                    <td>
                      <span className={`badge ${employee.is_active ? 'badge-success' : 'badge-neutral'}`}>
                        {employee.is_active ? 'Actif' : 'Désactivé'}
                      </span>
                    </td>
                    <td>
                      <div className="flex justify-end gap-1">
                        <Button size="sm" variant="ghost" title="Voir l’activité" onClick={() => setActivityId(employee.id)}>
                          <ShieldCheck size={15} />
                        </Button>
                        {canManage ? (
                          <>
                            <Button size="sm" variant="ghost" title="Modifier" onClick={() => openForm(employee)}>
                              <Pencil size={15} />
                            </Button>
                            <Button size="sm" variant="ghost" title="Réinitialiser le mot de passe" onClick={() => void resetPassword(employee)}>
                              <KeyRound size={15} />
                            </Button>
                            {employee.role !== 'owner' ? (
                              <Button
                                size="sm"
                                variant="ghost"
                                title={employee.is_active ? 'Désactiver' : 'Réactiver'}
                                onClick={() => setTogglingEmployee(employee)}
                              >
                                <UserX size={15} />
                              </Button>
                            ) : null}
                          </>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Création / modification */}
      <Modal
        open={formOpen}
        title={editing ? `Modifier ${editing.full_name}` : 'Nouvel employé'}
        description={
          editing
            ? 'Modifier le rôle change immédiatement les droits sur le serveur.'
            : 'Un mot de passe provisoire est généré : l’employé devra le changer à sa première connexion.'
        }
        onClose={() => setFormOpen(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setFormOpen(false)} disabled={createMutation.isPending || updateMutation.isPending}>
              Annuler
            </Button>
            <Button variant="primary" onClick={submit} loading={createMutation.isPending || updateMutation.isPending}>
              Enregistrer
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="Nom complet" required error={fields.full_name}>
            {(id) => (
              <Input
                id={id}
                value={form.full_name}
                onChange={(event) => setForm({ ...form, full_name: event.target.value })}
                placeholder="Ex : Awa Traoré"
                invalid={Boolean(fields.full_name)}
              />
            )}
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Adresse e-mail" required error={fields.email} hint={editing ? 'L’identifiant de connexion ne se modifie pas.' : undefined}>
              {(id) => (
                <Input
                  id={id}
                  type="email"
                  value={form.email}
                  onChange={(event) => setForm({ ...form, email: event.target.value })}
                  placeholder="prenom@exemple.ci"
                  disabled={Boolean(editing)}
                  invalid={Boolean(fields.email)}
                />
              )}
            </Field>
            <Field label="Téléphone" error={fields.phone}>
              {(id) => (
                <Input
                  id={id}
                  value={form.phone}
                  onChange={(event) => setForm({ ...form, phone: event.target.value })}
                  placeholder="07 00 00 00 00"
                />
              )}
            </Field>
          </div>
          <Field label="Rôle opérationnel" error={fields.role}>
            {(id) => {
              const allowedRoles =
                me?.user.role === 'owner' || me?.user.role === 'superadmin'
                  ? ROLES
                  : ROLES.filter((role) => role.id !== 'owner')
              return (
                <div className="space-y-2">
                  <Select
                    id={id}
                    value={form.role}
                    onChange={(event) => setForm({ ...form, role: event.target.value as Role })}
                    options={allowedRoles.map((role) => ({ value: role.id, label: role.label }))}
                  />
                  <div className="rounded-lg bg-surface-muted p-2.5 text-xs text-ink-muted">
                    <p className="font-semibold text-ink">
                      {form.role === 'manager' && '👔 Gérant : supervise l’activité, configure les tarifs, valide les dépenses et affecte les laveurs.'}
                      {form.role === 'cashier' && '🏷️ Caissier : accueille les clients, enregistre les lavages, encaisse et gère sa caisse.'}
                      {form.role === 'washer' && '🧼 Laveur : utilise l’espace mobile atelier pour prendre en charge et terminer les lavages.'}
                      {form.role === 'owner' && '🏢 Propriétaire : accès global d’administration et abonnement SaaS.'}
                    </p>
                  </div>
                </div>
              )
            }}
          </Field>

          <div className="rounded-xl border border-line bg-surface-muted/40 p-3">
            <Field
              label="Commission"
              hint="Le barème de la station s’applique par défaut. Une exception posée ici suit le compte : changer le tarif de la station ne la modifie pas."
              error={fields.commission_mode}
            >
              {(id) => (
                <Select
                  id={id}
                  value={form.commission_mode}
                  onChange={(event) =>
                    setForm({ ...form, commission_mode: event.target.value as CommissionChoice })
                  }
                  options={EMPLOYEE_COMMISSION_MODES}
                  invalid={Boolean(fields.commission_mode)}
                />
              )}
            </Field>
            {form.commission_mode === 'per_wash' ? (
              <div className="mt-3">
                <Field label="Montant versé par lavage terminé (FCFA)" hint="De 1 à 100 000 FCFA.">
                  {(id) => (
                    <Input
                      id={id}
                      inputMode="numeric"
                      value={form.commission_per_wash === 0 ? '' : form.commission_per_wash}
                      onChange={(event) =>
                        setForm({ ...form, commission_per_wash: parseAmountInput(event.target.value) })
                      }
                      placeholder="500"
                      invalid={Boolean(fields.commission_mode)}
                    />
                  )}
                </Field>
              </div>
            ) : null}
            {form.commission_mode === 'percent' ? (
              <div className="mt-3">
                <Field label="Pourcentage du chiffre d’affaires (%)" hint="De 1 à 100 %.">
                  {(id) => (
                    <Input
                      id={id}
                      inputMode="numeric"
                      value={form.commission_percent === 0 ? '' : form.commission_percent}
                      onChange={(event) =>
                        setForm({ ...form, commission_percent: parseAmountInput(event.target.value) })
                      }
                      placeholder="20"
                      invalid={Boolean(fields.commission_mode)}
                    />
                  )}
                </Field>
              </div>
            ) : null}
          </div>

          {error ? <p className="rounded-lg border border-danger/40 bg-danger-soft px-3 py-2 text-sm text-danger">{error}</p> : null}
        </div>
      </Modal>

      {/* Mot de passe provisoire affiché une seule fois */}
      <Modal
        open={Boolean(created)}
        title="Compte créé"
        description="Notez ce mot de passe provisoire : il ne sera plus affiché."
        onClose={() => setCreated(null)}
        size="sm"
        footer={
          <Button variant="primary" onClick={() => setCreated(null)}>
            J’ai noté
          </Button>
        }
      >
        <p className="text-sm text-ink-muted">
          Transmettez ces identifiants à {created?.email}. Un changement de mot de passe sera exigé à la première connexion.
        </p>
        <div className="mt-3 rounded-lg border border-line bg-surface-muted px-3 py-3 text-sm">
          <p>
            <span className="text-ink-muted">Identifiant : </span>
            <span className="font-medium">{created?.email}</span>
          </p>
          <p className="mt-1">
            <span className="text-ink-muted">Mot de passe provisoire : </span>
            <span className="font-mono font-semibold text-brand">{created?.temporary_password}</span>
          </p>
        </div>
      </Modal>

      <Modal
        open={Boolean(reseted)}
        title="Mot de passe réinitialisé"
        onClose={() => setReseted(null)}
        size="sm"
        footer={
          <Button variant="primary" onClick={() => setReseted(null)}>
            Fermer
          </Button>
        }
      >
        <p className="text-sm text-ink-muted">
          Nouveau mot de passe provisoire pour {reseted?.name}. Les sessions ouvertes de cet employé restent valables.
        </p>
        <p className="mt-3 rounded-lg border border-line bg-surface-muted px-3 py-2 text-center font-mono text-base font-semibold text-brand">
          {reseted?.password}
        </p>
      </Modal>

      {/* Activité d'un employé */}
      <Modal
        open={Boolean(activityId)}
        title={activityQuery.data ? `Activité de ${activityQuery.data.employee.full_name}` : 'Activité'}
        onClose={() => setActivityId(null)}
        size="lg"
        footer={
          <Button variant="ghost" onClick={() => setActivityId(null)}>
            Fermer
          </Button>
        }
      >
        <div className="flex flex-wrap items-end gap-3">
          <Field label="Du">
            {(id) => (
              <Input
                id={id}
                type="date"
                value={activityFrom}
                onChange={(event) => setActivityFrom(event.target.value)}
              />
            )}
          </Field>
          <Field label="Au">
            {(id) => (
              <Input id={id} type="date" value={activityTo} onChange={(event) => setActivityTo(event.target.value)} />
            )}
          </Field>
          <Button size="sm" onClick={() => { setActivityFrom(addDays(today(), -6)); setActivityTo(today()) }}>
            7 derniers jours
          </Button>
        </div>

        {activityQuery.isLoading ? (
          <Spinner />
        ) : activityQuery.isError ? (
          <ErrorState message="Impossible de charger l’activité." onRetry={() => void activityQuery.refetch()} />
        ) : activityQuery.data ? (
          <div className="mt-4 space-y-4">
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <div className="rounded-lg border border-line p-3">
                <p className="stat-label">Lavages</p>
                <p className="stat-value text-lg">{formatNumber(activityQuery.data.totals.wash_count)}</p>
              </div>
              <div className="rounded-lg border border-line p-3">
                <p className="stat-label">CA généré</p>
                <p className="stat-value text-lg">{formatXOF(activityQuery.data.totals.revenue_xof)}</p>
              </div>
              <div className="rounded-lg border border-line p-3">
                <p className="stat-label">Encaissé</p>
                <p className="stat-value text-lg text-success">{formatXOF(activityQuery.data.totals.collected_xof)}</p>
              </div>
              <div className="rounded-lg border border-line p-3">
                <p className="stat-label">Remises</p>
                <p className="stat-value text-lg">{formatXOF(activityQuery.data.totals.discount_xof)}</p>
              </div>
            </div>
            <p className="text-xs text-ink-muted">
              Période : {formatDate(activityQuery.data.range.from)} → {formatDate(activityQuery.data.range.to)} ·{' '}
              {formatNumber(activityQuery.data.totals.completed_count)} terminée(s),{' '}
              {formatNumber(activityQuery.data.totals.canceled_count)} annulée(s)
            </p>

            {activityQuery.data.orders.length === 0 ? (
              <p className="text-sm text-ink-muted">Aucun lavage enregistré sur cette période.</p>
            ) : (
              <div className="overflow-x-auto rounded-lg border border-line">
                <table className="table">
                  <thead>
                    <tr>
                      <th>Référence</th>
                      <th>Date</th>
                      <th>Client / véhicule</th>
                      <th>Statut</th>
                      <th className="text-right">Total</th>
                      <th className="text-right">Reste</th>
                    </tr>
                  </thead>
                  <tbody>
                    {activityQuery.data.orders.map((row) => (
                      <tr key={row.id}>
                        <td className="font-medium">{row.reference}</td>
                        <td className="whitespace-nowrap text-ink-muted">{formatDateTime(row.created_at)}</td>
                        <td className="text-ink-muted">
                          {row.customer_name || 'Client de passage'}
                          <span className="block text-xs">
                            {row.vehicle_category_name}
                            {row.plate ? ` · ${row.plate}` : ''}
                          </span>
                        </td>
                        <td>
                          <span className={`badge ${washStatusTone(row.status)}`}>{washStatusLabel(row.status)}</span>
                        </td>
                        <td className="tabular text-right">{formatXOF(row.total_xof)}</td>
                        <td className="tabular text-right">
                          {row.due_xof > 0 ? <span className="text-warning">{formatXOF(row.due_xof)}</span> : 'Soldé'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        ) : null}
      </Modal>

      <ConfirmDialog
        open={Boolean(togglingEmployee)}
        title={
          togglingEmployee?.is_active
            ? `Désactiver le compte de ${togglingEmployee?.full_name} ?`
            : `Réactiver le compte de ${togglingEmployee?.full_name} ?`
        }
        message={
          togglingEmployee?.is_active
            ? 'La personne ne pourra plus se connecter, mais ses lavages et encaissements déjà enregistrés sont conservés.'
            : 'La personne pourra de nouveau se connecter avec son mot de passe existant.'
        }
        confirmLabel={togglingEmployee?.is_active ? 'Désactiver' : 'Réactiver'}
        tone={togglingEmployee?.is_active ? 'danger' : 'primary'}
        loading={updateMutation.isPending}
        onCancel={() => setTogglingEmployee(null)}
        onConfirm={() => void toggleActive()}
      />
    </div>
  )
}
