/**
 * Fiche de l'entreprise et de la station : ce que remplissent les formulaires
 * du parcours de mise en route.
 *
 * Chaque enregistrement invalide l'état d'onboarding : la progression affichée
 * doit suivre ce qui est réellement en base, pas ce que l'écran vient
 * d'envoyer. C'est le serveur qui décide si une étape est satisfaite.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { OpeningException, OpeningHour, Organization, RegistryField, Station, StationProfile, StationSiteProfile } from '@/types'

/** Fiche complète de l'entreprise : identité, facturation et état des mentions. */
export interface OrganisationDetaillee {
  organization: Organization
  registry: RegistryField[]
}

export function useOrganisationDetaillee(enabled = true) {
  return useQuery<OrganisationDetaillee>({
    queryKey: ['organization', 'detail'],
    queryFn: () => api.get<OrganisationDetaillee>('/organizations/current'),
    enabled,
  })
}

export interface OrganisationInput {
  [champ: string]: unknown
  /** Mentions administratives : { rccm: 'CI-ABJ-…', tax_id: '…' }. */
  registry?: Record<string, string>
}

/** Enregistre la fiche d'entreprise (identité, facturation, mentions). */
export function useSaveOrganisation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: OrganisationInput) =>
      api.patch<{ message: string; fields: string[] }>('/organizations/current', input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['organization'] })
      void queryClient.invalidateQueries({ queryKey: ['me'] })
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
    },
  })
}

/** Fiche d'une station : localisation, capacités, horaires. */
export function useStationProfile(stationId: string, enabled = true) {
  return useQuery<StationProfile>({
    queryKey: ['station-profile', stationId],
    queryFn: () => api.get<StationProfile>(`/stations/${stationId}/profile`),
    enabled: enabled && Boolean(stationId),
  })
}

export interface StationProfileInput {
  name?: string
  code?: string
  phone?: string
  city?: string
  country?: string
  district?: string
  commune?: string
  district_area?: string
  street?: string
  landmark?: string
  latitude?: string
  longitude?: string
  billing_address?: string
  site?: Partial<StationSiteProfile>
}

export function useSaveStationProfile() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: StationProfileInput & { id: string }) =>
      api.patch<Station>(`/stations/${id}`, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['station-profile'] })
      void queryClient.invalidateQueries({ queryKey: ['stations'] })
      void queryClient.invalidateQueries({ queryKey: ['me'] })
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
    },
  })
}

export interface HorairesInput {
  days: OpeningHour[]
  exceptions: OpeningException[]
}

/**
 * Enregistre les horaires. Le serveur valide avant d'écrire (fermeture avant
 * ouverture, pause hors plage, deux exceptions le même jour) et répond avec les
 * jours réellement enregistrés.
 */
export function useSaveOpeningHours(stationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: HorairesInput) =>
      api.put<{ days: OpeningHour[]; exceptions: OpeningException[]; message: string }>(
        `/stations/${stationId}/opening-hours`,
        input,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['station-profile'] })
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
      void queryClient.invalidateQueries({ queryKey: ['me'] })
    },
  })
}
