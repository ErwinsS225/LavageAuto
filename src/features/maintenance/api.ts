/**
 * Matériel, stock et règles de commande : appels à l'API.
 *
 * Le serveur décide de tout ce qui compte : un article sans quantité est
 * « non renseigné » (jamais « 0 »), une quantité ne s'écrit pas à la main mais
 * bouge par un mouvement, et une règle de commande est appliquée là où elle
 * décide. Ces hooks ne font que transporter — ils ne recalculent rien.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type {
  Equipment,
  EquipmentResponse,
  OrderRules,
  OrderRulesResponse,
  StockMovement,
  StockResponse,
} from '@/types'

// ---------------------------------------------------------------------------
// Matériel
// ---------------------------------------------------------------------------

export function equipmentKey(stationId: string) {
  return ['equipment', stationId] as const
}

export function useEquipment(stationId: string | null) {
  return useQuery<EquipmentResponse>({
    queryKey: equipmentKey(stationId ?? ''),
    queryFn: () => api.get<EquipmentResponse>(`/stations/${stationId}/equipment`),
    enabled: Boolean(stationId),
  })
}

export interface EquipmentInput {
  template_code?: string
  name?: string
  quantity?: number
  state?: Equipment['state']
  acquired_on?: string
  last_service_on?: string
  next_service_on?: string
  cost_xof?: number
  notes?: string
}

export function useCreateEquipment(stationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: EquipmentInput) =>
      api.post<{ id: string }>(`/stations/${stationId}/equipment`, input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: equipmentKey(stationId) }),
  })
}

export function useUpdateEquipment(stationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: Partial<EquipmentInput> & { id: string; is_active?: boolean }) =>
      api.patch<{ ok: boolean }>(`/equipment/${id}`, input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: equipmentKey(stationId) }),
  })
}

// ---------------------------------------------------------------------------
// Stock
// ---------------------------------------------------------------------------

export function stockKey(stationId: string) {
  return ['stock', stationId] as const
}

export function useStock(stationId: string | null) {
  return useQuery<StockResponse>({
    queryKey: stockKey(stationId ?? ''),
    queryFn: () => api.get<StockResponse>(`/stations/${stationId}/stock`),
    enabled: Boolean(stationId),
  })
}

export interface StockItemInput {
  name: string
  category_code?: string
  internal_code?: string
  unit?: string
  /** Omis = « je ne sais pas encore ». */
  quantity?: number
  unit_cost_xof?: number
  min_quantity?: number
  supplier?: string
  location?: string
  expires_on?: string
  notes?: string
}

export function useCreateStockItem(stationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: StockItemInput) =>
      api.post<{ id: string }>(`/stations/${stationId}/stock/items`, input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: stockKey(stationId) }),
  })
}

export function useUpdateStockItem(stationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: Partial<StockItemInput> & { id: string; is_active?: boolean; quantity_known?: boolean }) =>
      api.patch<{ ok: boolean }>(`/stock/items/${id}`, input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: stockKey(stationId) }),
  })
}

export function useStockMovements(itemId: string | null) {
  return useQuery<{ item_name: string; unit: string; items: StockMovement[] }>({
    queryKey: ['stock-movements', itemId ?? ''],
    queryFn: () =>
      api.get<{ item_name: string; unit: string; items: StockMovement[] }>(
        `/stock/items/${itemId}/movements`,
      ),
    enabled: Boolean(itemId),
  })
}

export interface StockMovementInput {
  kind: StockMovement['kind']
  quantity: number
  unit_cost_xof?: number
  reason?: string
}

export function useCreateStockMovement(stationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ itemId, ...input }: StockMovementInput & { itemId: string }) =>
      api.post<StockMovement>(`/stock/items/${itemId}/movements`, input),
    onSuccess: (_rep, variables) => {
      void queryClient.invalidateQueries({ queryKey: stockKey(stationId) })
      void queryClient.invalidateQueries({ queryKey: ['stock-movements', variables.itemId] })
    },
  })
}

// ---------------------------------------------------------------------------
// Règles de fonctionnement des commandes
// ---------------------------------------------------------------------------

export function orderRulesKey(stationId: string) {
  return ['order-rules', stationId] as const
}

export function useOrderRules(stationId: string | null) {
  return useQuery<OrderRulesResponse>({
    queryKey: orderRulesKey(stationId ?? ''),
    queryFn: () => api.get<OrderRulesResponse>(`/stations/${stationId}/order-rules`),
    enabled: Boolean(stationId),
  })
}

export function useUpdateOrderRules(stationId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: Partial<OrderRules>) =>
      api.put<OrderRulesResponse>(`/stations/${stationId}/order-rules`, input),
    onSuccess: (reponse) => {
      queryClient.setQueryData(orderRulesKey(stationId), reponse)
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
    },
  })
}
