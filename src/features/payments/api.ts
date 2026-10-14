/**
 * Caisse : moyens de paiement acceptés, paramètres de caisse, reçus.
 *
 * Deux principes tenus ici :
 *   - le moyen de paiement envoyé à l'encaissement est un **identifiant**
 *     (`payment_method_id`) : c'est le serveur qui décide la famille comptable,
 *     l'exigence de référence et la permission du différé ;
 *   - le reçu est lu tel qu'il sera remis. Aucun montant n'est recalculé côté
 *     navigateur, et aucune mention absente n'est remplacée par une valeur.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, query, ApiError } from '@/lib/api'
import type { CashSettings, PaymentFamily, PaymentMethodDeclared, Receipt } from '@/types'

export const paymentKeys = {
  methods: (stationId?: string, activesSeulement = false) =>
    ['payment-methods', stationId ?? '', activesSeulement ? 'actifs' : 'tous'] as const,
  cashSettings: (stationId?: string) => ['cash-settings', stationId ?? ''] as const,
  receipt: (orderId?: string) => ['receipt', orderId ?? ''] as const,
}

/** Moyens de paiement déclarés par la station. */
export function usePaymentMethods(stationId?: string, activesSeulement = false, enabled = true) {
  return useQuery<{ items: PaymentMethodDeclared[]; total: number; actifs: number }>({
    queryKey: paymentKeys.methods(stationId, activesSeulement),
    queryFn: () =>
      api.get(
        `/payment-methods${query({
          station_id: stationId,
          actives: activesSeulement ? 1 : undefined,
        })}`,
      ),
    enabled: Boolean(stationId) && enabled,
  })
}

function invaliderMoyens(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ['payment-methods'] })
  void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
}

/** Active / désactive un moyen, le renomme ou le réordonne. */
export function useUpdatePaymentMethod() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      id: string
      station_id: string
      name?: string
      is_active?: boolean
      requires_reference?: boolean
      sort_order?: number
    }) => {
      const { id, ...corps } = input
      return api.patch<PaymentMethodDeclared>(`/payment-methods/${id}`, corps)
    },
    onSuccess: () => invaliderMoyens(queryClient),
  })
}

/** Ajoute un moyen propre à la station (« Airtel Money », « Chèque »…). */
export function useCreatePaymentMethod() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      station_id: string
      name: string
      family: PaymentFamily
      requires_reference?: boolean
      is_active?: boolean
    }) => api.post<PaymentMethodDeclared>('/payment-methods', input),
    onSuccess: () => invaliderMoyens(queryClient),
  })
}

/** Paramètres de caisse et identité des reçus. */
export function useCashSettings(stationId?: string, enabled = true) {
  return useQuery<{ station_id: string; settings: CashSettings; roles_connus: Array<{ code: string; label: string }> }>({
    queryKey: paymentKeys.cashSettings(stationId),
    queryFn: () => api.get(`/stations/${stationId}/cash-settings`),
    enabled: Boolean(stationId) && enabled,
  })
}

export function useSaveCashSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: { stationId: string; settings: Partial<CashSettings> }) =>
      api.put<{ station_id: string; settings: CashSettings; message: string }>(
        `/stations/${input.stationId}/cash-settings`,
        input.settings,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['cash-settings'] })
      void queryClient.invalidateQueries({ queryKey: ['station'] })
    },
  })
}

/**
 * Reçu d'une fiche soldée. `receipt_not_issued` n'est pas une erreur à
 * masquer : l'écran dit que la fiche n'est pas soldée, ce qui est l'information
 * utile au comptoir.
 */
export function useReceipt(orderId?: string, enabled = true) {
  return useQuery<{ receipt: Receipt }>({
    queryKey: paymentKeys.receipt(orderId),
    queryFn: () => api.get(`/wash-orders/${orderId}/receipt`),
    enabled: Boolean(orderId) && enabled,
    retry: (compteur, erreur) => !(erreur instanceof ApiError && erreur.status === 404) && compteur < 2,
  })
}

/** Émission explicite (fiche soldée avant la mise à jour, ou réimpression). */
export function useIssueReceipt() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (orderId: string) => api.post<{ receipt: Receipt }>(`/wash-orders/${orderId}/receipt`),
    onSuccess: (_data, orderId) => {
      void queryClient.invalidateQueries({ queryKey: paymentKeys.receipt(orderId) })
    },
  })
}
