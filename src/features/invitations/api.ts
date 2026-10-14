/**
 * Invitations : inviter un collègue à rejoindre l'entreprise.
 *
 * Deux niveaux d'accès cohabitent ici :
 *   - la liste, la création et la révocation demandent une session
 *     (`employee:view` / `employee:manage`) ;
 *   - l'aperçu public, lui, est lu sans session par la personne invitée, et ne
 *     renvoie aucun identifiant.
 *
 * WashGuard n'envoie aucun message : l'interface fournit un lien et un code à
 * transmettre soi-même. C'est une contrainte du produit (aucun moyen d'envoi),
 * pas un oubli — l'écran le dit à l'utilisateur.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { Invitation, InvitationPreview } from '@/types'

export const invitationsKey = () => ['invitations'] as const

export function useInvitations(enabled = true) {
  return useQuery<{ items: Invitation[]; total: number }>({
    queryKey: invitationsKey(),
    queryFn: () => api.get<{ items: Invitation[]; total: number }>('/invitations'),
    enabled,
  })
}

export interface InvitationInput {
  email?: string
  phone?: string
  role: 'manager' | 'cashier' | 'washer'
  station_id?: string
  validity_days?: number
}

/** Réponse de création : le jeton n'apparaît qu'ici, une seule fois. */
export interface InvitationCreee {
  id: string
  code: string
  token: string
  role: string
  expires_at: string
  message: string
  notice: string
}

export function useCreateInvitation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: InvitationInput) => api.post<InvitationCreee>('/invitations', input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: invitationsKey() }),
  })
}

export function useRevokeInvitation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.post<{ message: string }>(`/invitations/${id}/revoke`),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: invitationsKey() }),
  })
}

/** Aperçu public d'une invitation, sans session : rien à cacher, rien à deviner. */
export function useInvitationPreview(code: string) {
  return useQuery<InvitationPreview>({
    queryKey: ['invitation-preview', code],
    queryFn: () => api.get<InvitationPreview>(`/invitations/${encodeURIComponent(code)}`),
    enabled: code.trim().length > 0,
    retry: false,
  })
}

/** Adresse à partager : elle ouvre directement la page « rejoindre ». */
export function lienInvitation(token: string): string {
  const base = typeof window === 'undefined' ? '' : window.location.origin
  return `${base}/rejoindre?jeton=${encodeURIComponent(token)}`
}

/**
 * Message prérempli pour WhatsApp. Le produit ne peut pas envoyer le message
 * lui-même : il propose le texte, la personne l'envoie depuis son compte.
 */
export function messageInvitation(preview: { organization_name: string; role_label: string }, lien: string, code: string): string {
  return `Bonjour, je vous invite à rejoindre ${preview.organization_name} sur WashGuard en tant que ${preview.role_label.toLowerCase()}. ` +
    `Lien : ${lien} — ou code à saisir sur la page « Rejoindre » : ${code}.`
}

export function lienWhatsApp(texte: string): string {
  return `https://wa.me/?text=${encodeURIComponent(texte)}`
}
