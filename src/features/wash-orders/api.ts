import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, query } from '@/lib/api'
import type { PaymentMethod, WashOrder, WashOrderPayment, WashStatus } from '@/types'

export const washOrdersKey = (filters?: Record<string, unknown>) => ['wash-orders', filters ?? {}] as const
export const washOrderKey = (id: string) => ['wash-order', id] as const

export interface WashOrderFilters {
  from?: string
  to?: string
  status?: WashStatus | ''
  station_id?: string
  employee_id?: string
  customer_id?: string
  search?: string
  open_only?: boolean
  limit?: number
}

export interface WashOrderTotals {
  revenue_xof: number
  paid_xof: number
  due_xof: number
  discount_xof: number
}

export interface WashOrdersResponse {
  items: WashOrder[]
  total: number
  page_size: number
  range: { from: string; to: string }
  totals: WashOrderTotals
}

export function useWashOrders(filters: WashOrderFilters) {
  return useQuery<WashOrdersResponse>({
    queryKey: washOrdersKey(filters as Record<string, unknown>),
    queryFn: () =>
      api.get<WashOrdersResponse>(
        `/wash-orders${query({
          from: filters.from,
          to: filters.to,
          status: filters.status,
          station_id: filters.station_id,
          employee_id: filters.employee_id,
          customer_id: filters.customer_id,
          search: filters.search,
          open_only: filters.open_only ? 'true' : undefined,
          limit: filters.limit ?? 100,
        })}`,
      ),
  })
}

export interface WashOrderDetail {
  order: WashOrder
  /** Type partagé avec la fiche : une seule définition des encaissements. */
  payments: WashOrderPayment[]
}

export function useWashOrder(id: string | null) {
  return useQuery<WashOrderDetail>({
    queryKey: washOrderKey(id ?? ''),
    queryFn: () => api.get<WashOrderDetail>(`/wash-orders/${id}`),
    enabled: Boolean(id),
  })
}

export interface WashOrderItemInput {
  service_id: string
  quantity: number
  unit_price_xof?: number
}

export interface CreateWashOrderInput {
  station_id?: string
  customer_id?: string
  customer_name?: string
  vehicle_category_id?: string
  /** Usage commercial : second référentiel, indépendant de la catégorie. */
  vehicle_usage_id?: string
  plate?: string
  employee_id?: string
  discount_xof?: number
  discount_reason?: string
  notes?: string
  status?: WashStatus | 'completed'
  items: WashOrderItemInput[]
}

export function useCreateWashOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateWashOrderInput) => api.post<WashOrder>('/wash-orders', input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['wash-orders'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['subscription'] })
    },
  })
}

export interface UpdateWashOrderInput {
  customer_id?: string | null
  customer_name?: string
  vehicle_category_id?: string | null
  vehicle_usage_id?: string | null
  plate?: string
  employee_id?: string | null
  discount_xof?: number
  discount_reason?: string
  notes?: string
  status?: WashStatus
  items?: WashOrderItemInput[]
}

export function useUpdateWashOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: UpdateWashOrderInput & { id: string }) =>
      api.patch<WashOrder>(`/wash-orders/${id}`, input),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['wash-orders'] })
      void queryClient.invalidateQueries({ queryKey: washOrderKey(variables.id) })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    },
  })
}

export function useAssignWashOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, employee_id }: { id: string; employee_id: string }) =>
      api.post<WashOrder>(`/wash-orders/${id}/assign`, { employee_id }),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['wash-orders'] })
      void queryClient.invalidateQueries({ queryKey: washOrderKey(variables.id) })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['washer-tasks'] })
    },
  })
}

export function useCancelWashOrder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      api.post<WashOrder>(`/wash-orders/${id}/cancel`, { reason }),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['wash-orders'] })
      void queryClient.invalidateQueries({ queryKey: washOrderKey(variables.id) })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['cash'] })
    },
  })
}

export interface PaymentInput {
  amount_xof: number
  /**
   * Moyen **déclaré** par la station (Orange Money, Wave, chèque…). C'est la
   * voie normale : le serveur en déduit la famille comptable, l'exigence de
   * référence et la permission du différé, et recopie le libellé choisi.
   */
  payment_method_id?: string
  /** Famille canonique, quand aucun moyen déclaré n'existe. */
  method?: PaymentMethod
  reference?: string
  note?: string
  /** Clé du geste : voir src/lib/idempotence.ts. */
  request_id?: string
}

/** Encaissement rattaché à une fiche : clôture automatique si tout est payé. */
export function useAddWashOrderPayment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: PaymentInput & { id: string }) =>
      api.post<{ order: WashOrder }>(`/wash-orders/${id}/payments`, input),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: ['wash-orders'] })
      void queryClient.invalidateQueries({ queryKey: washOrderKey(variables.id) })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
      void queryClient.invalidateQueries({ queryKey: ['cash'] })
      void queryClient.invalidateQueries({ queryKey: ['payments'] })
    },
  })
}
