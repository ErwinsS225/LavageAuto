/**
 * Onboarding : appels à l'API /onboarding.
 *
 * Le serveur calcule l'avancement à partir des données réelles : le front ne
 * fait qu'afficher ce qu'il reçoit et transmettre les saisies. Activer la
 * station est une décision du serveur (il revérifie chaque prérequis) : cette
 * couche ne peut donc pas activer un espace par erreur.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { Onboarding } from '@/types'

/**
 * État du parcours pour l'entreprise de la session.
 *
 * `affiche` : l'état déjà reçu avec /auth/me. Il s'affiche immédiatement (pas
 * d'écran d'attente après connexion) et le serveur est quand même interrogé au
 * montage, pour que la progression affichée reste celle des données réelles.
 */
export function useOnboarding(enabled = true, affiche: Onboarding | null = null) {
  return useQuery<Onboarding>({
    queryKey: ['onboarding'],
    queryFn: () => api.get<Onboarding>('/onboarding'),
    enabled,
    retry: false,
    placeholderData: affiche ?? undefined,
  })
}

/** Enregistre les choix d'une étape (ils ne la déclarent pas « faite »). */
export function useSaveOnboardingStep() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ code, data }: { code: string; data: Record<string, unknown> }) =>
      api.patch<Onboarding>(`/onboarding/steps/${code}`, { data }),
    onSuccess: (payload) => {
      queryClient.setQueryData(['onboarding'], payload)
      void queryClient.invalidateQueries({ queryKey: ['me'] })
    },
  })
}

/** Demande l'activation : le serveur refuse si un prérequis manque. */
export function useActivateOnboarding() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => api.post<Onboarding>('/onboarding/activate'),
    onSuccess: (payload) => {
      queryClient.setQueryData(['onboarding'], payload)
      void queryClient.invalidateQueries({ queryKey: ['me'] })
      void queryClient.invalidateQueries({ queryKey: ['stations'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    },
  })
}
