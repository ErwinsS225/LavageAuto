/**
 * Panneau « Invitations » (Paramètres → Invitations).
 *
 * Le partage est manuel, et c'est assumé : le backend n'a aucun moyen d'envoi
 * (ni e-mail, ni SMS, ni API WhatsApp). L'écran ne fait donc pas semblant
 * d'envoyer quoi que ce soit — il donne le lien, le code, et un bouton qui
 * ouvre WhatsApp avec le message prérempli, que l'utilisateur envoie lui-même.
 */
import { Check, Copy, Plus, Share2, Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'

import { Badge, Button, Card, CardHeader, EmptyState, Field, Input, Select } from '@/components/ui'
import { useToast } from '@/components/Toaster'
import {
  lienInvitation,
  lienWhatsApp,
  messageInvitation,
  useCreateInvitation,
  useInvitations,
  useRevokeInvitation,
  type InvitationCreee,
} from '@/features/invitations/api'
import { useAuth } from '@/hooks/useAuth'
import { useStation } from '@/hooks/useStation'
import { ApiError } from '@/lib/api'
import type { Invitation } from '@/types'

const TONS: Record<Invitation['state'], 'success' | 'brand' | 'warning' | 'neutral'> = {
  pending: 'brand',
  accepted: 'success',
  expired: 'warning',
  revoked: 'neutral',
}

const ROLES = [
  { value: 'cashier', label: 'Caissier — encaisse et enregistre les lavages' },
  { value: 'washer', label: 'Laveur — exécute les lavages' },
  { value: 'manager', label: 'Gérant — gère la station au quotidien' },
]

export default function InvitationsPanel() {
  const toast = useToast()
  const { can } = useAuth()
  const { stationId } = useStation()
  const liste = useInvitations(can('employee:view'))
  const creation = useCreateInvitation()
  const revocation = useRevokeInvitation()

  const [canal, setCanal] = useState<'email' | 'phone'>('email')
  const [destinataire, setDestinataire] = useState('')
  const [role, setRole] = useState<'manager' | 'cashier' | 'washer'>('cashier')
  const [erreur, setErreur] = useState('')
  const [creee, setCreee] = useState<InvitationCreee | null>(null)
  const [copie, setCopie] = useState('')

  const peutGerer = can('employee:manage')
  const invitations = liste.data?.items ?? []
  const enAttente = invitations.filter((invitation) => invitation.state === 'pending').length

  async function creer(event: FormEvent) {
    event.preventDefault()
    setErreur('')
    if (!destinataire.trim()) {
      setErreur(canal === 'email' ? "Indiquez l'adresse e-mail." : 'Indiquez le numéro de téléphone.')
      return
    }
    try {
      const reponse = await creation.mutateAsync({
        [canal]: destinataire.trim(),
        role,
        station_id: stationId || undefined,
      })
      setCreee(reponse)
      setDestinataire('')
      toast.success('Invitation créée. Transmettez-la vous-même : WashGuard n’envoie aucun message.')
    } catch (caught) {
      setErreur(caught instanceof ApiError ? caught.message : "La création de l'invitation a échoué.")
    }
  }

  async function copier(texte: string, nom: string) {
    try {
      await navigator.clipboard.writeText(texte)
      setCopie(nom)
      toast.success('Copié.')
      window.setTimeout(() => setCopie(''), 2000)
    } catch {
      toast.error('La copie n’a pas fonctionné : sélectionnez le texte à la main.')
    }
  }

  return (
    <div className="space-y-5">
      <Card>
        <CardHeader
          title="Inviter une collègue ou un collègue"
          subtitle="Le lien et le code désignent la même invitation. WashGuard n’envoie aucun message : c’est vous qui le transmettez."
          actions={<Badge tone={enAttente > 0 ? 'brand' : 'neutral'}>{enAttente} en attente</Badge>}
        />
        <form className="grid gap-4 p-4 sm:grid-cols-4" onSubmit={creer} noValidate>
          <Field label="Envoyer par">
            {(id) => (
              <Select
                id={id}
                value={canal}
                onChange={(event) => setCanal(event.target.value as 'email' | 'phone')}
                disabled={!peutGerer}
                options={[
                  { value: 'email', label: 'E-mail' },
                  { value: 'phone', label: 'Téléphone (WhatsApp)' },
                ]}
              />
            )}
          </Field>
          <div className="sm:col-span-2">
            <Field
              label={canal === 'email' ? 'Adresse e-mail' : 'Numéro de téléphone'}
              hint={canal === 'phone' ? 'Format international, ex : +225 07 00 00 00 00.' : undefined}
              required
            >
              {(id) => (
                <Input
                  id={id}
                  type={canal === 'email' ? 'email' : 'tel'}
                  value={destinataire}
                  onChange={(event) => setDestinataire(event.target.value)}
                  disabled={!peutGerer}
                />
              )}
            </Field>
          </div>
          <Field label="Rôle">
            {(id) => (
              <Select
                id={id}
                value={role}
                onChange={(event) => setRole(event.target.value as typeof role)}
                disabled={!peutGerer}
                options={ROLES}
              />
            )}
          </Field>
          <div className="sm:col-span-4">
            <Button type="submit" variant="primary" icon={<Plus size={15} />} loading={creation.isPending} disabled={!peutGerer}>
              Créer l’invitation
            </Button>
          </div>
        </form>
        {erreur ? <p className="border-t border-line px-4 py-3 text-sm text-danger">{erreur}</p> : null}
      </Card>

      {creee && (
        <Card className="border-brand/40">
          <CardHeader
            title="À transmettre maintenant"
            subtitle="Le lien complet n’est affiché qu’une fois : il n’est pas conservé côté serveur. Le code, lui, reste consultable."
            actions={<Badge tone="brand">Nouvelle</Badge>}
          />
          <div className="space-y-3 p-4">
            <div className="flex flex-wrap items-end gap-2">
              <div className="min-w-0 flex-1">
                <p className="text-xs text-ink-muted">Lien d’invitation</p>
                <p className="truncate rounded-field border border-line bg-surface-inset px-3 py-2 font-mono text-xs text-ink">
                  {lienInvitation(creee.token)}
                </p>
              </div>
              <Button
                variant="secondary"
                size="sm"
                icon={copie === 'lien' ? <Check size={14} /> : <Copy size={14} />}
                onClick={() => void copier(lienInvitation(creee.token), 'lien')}
              >
                Copier le lien
              </Button>
            </div>
            <div className="flex flex-wrap items-end gap-2">
              <div>
                <p className="text-xs text-ink-muted">Code à dicter au téléphone</p>
                <p className="rounded-field border border-line bg-surface-inset px-3 py-2 font-mono text-lg tracking-[0.3em] text-ink">
                  {creee.code}
                </p>
              </div>
              <Button
                variant="secondary"
                size="sm"
                icon={copie === 'code' ? <Check size={14} /> : <Copy size={14} />}
                onClick={() => void copier(creee.code, 'code')}
              >
                Copier le code
              </Button>
              {/* Un lien, pas un bouton : ouvrir WhatsApp est une navigation.
                  Les classes reprennent la variante secondaire du composant Button. */}
              <a
                href={lienWhatsApp(
                  messageInvitation(
                    { organization_name: 'notre station', role_label: creee.role },
                    lienInvitation(creee.token),
                    creee.code,
                  ),
                )}
                target="_blank"
                rel="noreferrer"
                className="inline-flex h-8 items-center gap-1.5 rounded-[0.6rem] border border-line bg-surface px-2.5 text-xs font-medium text-ink shadow-e1 transition-[background-color,border-color,box-shadow] duration-200 ease-soft hover:border-line-strong hover:bg-surface-muted hover:shadow-e2"
              >
                <Share2 size={14} aria-hidden />
                Partager sur WhatsApp
              </a>
            </div>
            <p className="text-xs text-ink-subtle">{creee.notice}</p>
          </div>
        </Card>
      )}

      <Card>
        <CardHeader
          title="Invitations de l’entreprise"
          subtitle="Une invitation acceptée a créé un compte : elle ne peut plus être réutilisée."
        />
        <div className="p-4">
          {liste.isLoading ? (
            <p className="text-sm text-ink-muted">Chargement des invitations…</p>
          ) : invitations.length === 0 ? (
            <EmptyState
              title="Aucune invitation"
              description="Créez une invitation ci-dessus : vous obtiendrez un lien et un code à transmettre à la personne concernée."
            />
          ) : (
            <ul className="divide-y divide-line">
              {invitations.map((invitation) => (
                <li key={invitation.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                  <div className="min-w-0">
                    <p className="font-medium text-ink">
                      {invitation.email || invitation.phone}
                      <span className="ml-2 text-xs font-normal text-ink-muted">{invitation.role_label}</span>
                    </p>
                    <p className="mt-0.5 text-xs text-ink-muted">
                      Code <span className="font-mono">{invitation.code}</span> · créée le{' '}
                      {new Date(invitation.created_at).toLocaleDateString('fr-FR')}
                      {invitation.state === 'pending'
                        ? ` · expire le ${new Date(invitation.expires_at).toLocaleDateString('fr-FR')}`
                        : invitation.accepted_at
                          ? ` · acceptée le ${new Date(invitation.accepted_at).toLocaleDateString('fr-FR')}`
                          : ''}
                      {invitation.invited_by_name ? ` · par ${invitation.invited_by_name}` : ''}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge tone={TONS[invitation.state]} dot>
                      {invitation.state_label}
                    </Badge>
                    {invitation.state === 'pending' && (
                      <>
                        <Button
                          variant="ghost"
                          size="sm"
                          icon={copie === invitation.id ? <Check size={14} /> : <Copy size={14} />}
                          onClick={() => void copier(invitation.code, invitation.id)}
                        >
                          Copier le code
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          icon={<Trash2 size={14} />}
                          loading={revocation.isPending}
                          onClick={async () => {
                            try {
                              await revocation.mutateAsync(invitation.id)
                              toast.success('Invitation révoquée.')
                            } catch (caught) {
                              toast.error(caught instanceof ApiError ? caught.message : 'La révocation a échoué.')
                            }
                          }}
                        >
                          Révoquer
                        </Button>
                      </>
                    )}
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
