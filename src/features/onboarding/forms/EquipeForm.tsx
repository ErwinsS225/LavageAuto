/**
 * Étape 11 du parcours : l'équipe.
 *
 * Un compte d'employé dit qui peut se connecter. L'écran ajoute ce qu'un compte
 * ne disait pas : la fonction, la date d'arrivée, les horaires, la situation.
 * Tous ces champs sont facultatifs — une station tenue par son propriétaire
 * passe cette étape sans rien saisir, et c'est normal.
 *
 * La rémunération est à part : elle demande la permission dédiée
 * (`employee:compensation`, portée par le propriétaire). Sans elle, l'écran
 * écrit que la donnée est réservée — il n'affiche pas des zéros qui
 * ressembleraient à une paie.
 */
import { useState } from 'react'
import { Plus, UserRound } from 'lucide-react'

import {
  Badge,
  Button,
  Card,
  CardHeader,
  Field,
  Input,
  Modal,
  Select,
} from '@/components/ui'
import { useCreateEmployee, useEmployees, useUpdateEmployee } from '@/features/employees/api'
import { useToast } from '@/components/Toaster'
import { useAuth } from '@/hooks/useAuth'
import { ApiError } from '@/lib/api'
import type { Employee, EmploymentStatus, Role } from '@/types'

const SITUATIONS: Array<{ value: string; label: string }> = [
  { value: 'active', label: 'En poste' },
  { value: 'on_leave', label: 'En congé' },
  { value: 'departed', label: 'Parti' },
]

const ROLES: Array<{ value: string; label: string }> = [
  { value: 'manager', label: 'Gérant' },
  { value: 'cashier', label: 'Caissier' },
  { value: 'washer', label: 'Laveur' },
]

/** Fiche d'un membre de l'équipe : ce qu'il fait, depuis quand, quand. */
function FicheEmploye({ employe }: { employe: Employee }) {
  const toast = useToast()
  const miseAJour = useUpdateEmployee()
  const { can } = useAuth()
  const [fonction, setFonction] = useState(employe.job_title ?? '')
  const [arrivee, setArrivee] = useState(employe.hired_on ?? '')
  const [horaires, setHoraires] = useState(employe.work_schedule ?? '')
  const [situation, setSituation] = useState<EmploymentStatus>(employe.employment_status ?? 'active')

  async function enregistrer() {
    try {
      await miseAJour.mutateAsync({
        id: employe.id,
        job_title: fonction,
        hired_on: arrivee,
        work_schedule: horaires,
        employment_status: situation,
      })
      toast.success('Fiche mise à jour.')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "La mise à jour n'a pas abouti.")
    }
  }

  return (
    <div className="space-y-3 rounded-xl border border-line p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <UserRound size={16} className="text-ink-subtle" aria-hidden />
          <span className="font-medium">{employe.full_name}</span>
          <Badge tone="neutral">{employe.role_label ?? employe.role}</Badge>
          <Badge tone={employe.is_active ? 'success' : 'neutral'}>
            {employe.is_active ? 'Accès ouvert' : 'Accès fermé'}
          </Badge>
        </div>
        <span className="text-xs text-ink-muted">
          {employe.employment_status_label ?? 'En poste'}
        </span>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Fonction" hint="Ce que la personne fait réellement (facultatif).">
          {(id) => (
            <Input
              id={id}
              value={fonction}
              onChange={(event) => setFonction(event.target.value)}
              placeholder="Caissière principale"
            />
          )}
        </Field>
        <Field label="Date d'arrivée">
          {(id) => (
            <Input
              id={id}
              type="date"
              value={arrivee}
              onChange={(event) => setArrivee(event.target.value)}
            />
          )}
        </Field>
        <Field label="Horaires" hint="Texte libre : aucun planning n'est calculé à partir de là.">
          {(id) => (
            <Input
              id={id}
              value={horaires}
              onChange={(event) => setHoraires(event.target.value)}
              placeholder="lun–sam, 7h–16h"
            />
          )}
        </Field>
        <Field label="Situation" hint="Distincte de l'accès : une personne en congé le garde.">
          {(id) => (
            <Select
              id={id}
              value={situation}
              onChange={(event) => setSituation(event.target.value as EmploymentStatus)}
              options={SITUATIONS}
            />
          )}
        </Field>
      </div>

      <p className="text-xs text-ink-muted">
        {can('employee:compensation')
          ? employe.compensation?.mode
            ? `Rémunération : ${employe.compensation.mode_label}`
            : 'Rémunération : aucune commission déclarée.'
          : 'Rémunération : donnée réservée au propriétaire.'}
      </p>

      <Button onClick={() => void enregistrer()} disabled={miseAJour.isPending}>
        Enregistrer
      </Button>
    </div>
  )
}

export default function EquipeForm({ stationId }: { stationId: string }) {
  const toast = useToast()
  const { data, isLoading, isError, refetch } = useEmployees()
  const creation = useCreateEmployee()
  const { can } = useAuth()
  const peutGerer = can('employee:manage')
  const [ouvert, setOuvert] = useState(false)
  const [nom, setNom] = useState('')
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<Role>('cashier')
  const [titre, setTitre] = useState('')

  async function ajouter() {
    try {
      const reponse = await creation.mutateAsync({
        full_name: nom,
        email,
        role,
        station_id: stationId,
        job_title: titre,
      })
      toast.success(
        `Compte créé. Mot de passe provisoire à communiquer une seule fois : ${reponse.temporary_password}`,
      )
      setOuvert(false)
      setNom('')
      setEmail('')
      setTitre('')
    } catch (caught) {
      toast.error(caught instanceof ApiError ? caught.message : "Le compte n'a pas pu être créé.")
    }
  }

  const employes = data?.items ?? []

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="Équipe de la station"
          subtitle="Un compte par personne. Fonction, date d'arrivée et horaires sont facultatifs : laissez vide ce que vous ne savez pas encore."
          actions={
            peutGerer ? (
              <Button icon={<Plus size={15} />} onClick={() => setOuvert(true)}>
                Ajouter une personne
              </Button>
            ) : (
              <Badge tone="neutral">Lecture seule</Badge>
            )
          }
        />
        <div className="space-y-3 p-4">
          {isLoading ? <p className="text-sm text-ink-muted">Chargement de l'équipe…</p> : null}
          {isError ? (
            // « Aucun compte » et « liste illisible » ne se confondent pas :
            // la première phrase est une information, la seconde un incident.
            <div className="space-y-2">
              <p className="text-sm text-ink-muted">
                L'équipe n'a pas pu être lue depuis le serveur.
              </p>
              <Button variant="secondary" onClick={() => void refetch()}>
                Réessayer
              </Button>
            </div>
          ) : null}
          {!isLoading && !isError && employes.length === 0 ? (
            <p className="text-sm text-ink-muted">
              Aucun compte pour l'instant : votre accès suffit à ouvrir la station.
            </p>
          ) : null}
          {employes.map((employe) => (
            <FicheEmploye key={employe.id} employe={employe} />
          ))}
        </div>
      </Card>

      <Modal
        open={ouvert}
        title="Ajouter une personne à l'équipe"
        onClose={() => setOuvert(false)}
        footer={
          <>
            <Button variant="ghost" onClick={() => setOuvert(false)}>
              Annuler
            </Button>
            <Button
              onClick={() => void ajouter()}
              disabled={creation.isPending || nom.trim() === '' || email.trim() === ''}
            >
              Créer le compte
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          <Field label="Nom complet" required>
            {(id) => (
              <Input id={id} value={nom} onChange={(event) => setNom(event.target.value)} />
            )}
          </Field>
          <Field label="Adresse e-mail" required>
            {(id) => (
              <Input
                id={id}
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="prenom@exemple.ci"
              />
            )}
          </Field>
          <Field label="Rôle">
            {(id) => (
              <Select
                id={id}
                value={role}
                onChange={(event) => setRole(event.target.value as Role)}
                options={ROLES}
              />
            )}
          </Field>
          <Field label="Fonction (facultatif)">
            {(id) => (
              <Input
                id={id}
                value={titre}
                onChange={(event) => setTitre(event.target.value)}
                placeholder="Caissière principale"
              />
            )}
          </Field>
          <p className="text-xs text-ink-muted">
            Un mot de passe provisoire est généré et affiché une seule fois : la personne le change à
            sa première connexion.
          </p>
        </div>
      </Modal>
    </div>
  )
}
