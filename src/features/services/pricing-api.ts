/**
 * Grille tarifaire : référentiel, compatibilités, options, règles, matrice.
 *
 * Un principe tenu partout ici : le navigateur **demande** des prix, il n'en
 * calcule aucun. Chaque écran envoie des choix (prestation, véhicule, usage,
 * client, options) et affiche ce que le serveur répond — provenance comprise.
 * C'est ce qui permet à une facture de s'expliquer après coup.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api, query } from '@/lib/api'
import type {
  PriceMatrix,
  PricePreview,
  PriceRule,
  ServiceOption,
  ServiceTemplateCatalogue,
} from '@/types'

export const pricingKeys = {
  templates: (stationId?: string) => ['service-templates', stationId ?? ''] as const,
  matrix: (stationId?: string) => ['price-matrix', stationId ?? ''] as const,
  rules: (stationId?: string, serviceId?: string) =>
    ['price-rules', stationId ?? '', serviceId ?? ''] as const,
  options: (stationId?: string, serviceId?: string) =>
    ['service-options', stationId ?? '', serviceId ?? ''] as const,
  compat: (serviceId: string) => ['service-compatibilities', serviceId] as const,
}

/** Catalogue de référence (familles + modèles, **sans prix**) et son adoption. */
export function useServiceTemplates(stationId?: string, enabled = true) {
  return useQuery<ServiceTemplateCatalogue>({
    queryKey: pricingKeys.templates(stationId),
    queryFn: () =>
      api.get<ServiceTemplateCatalogue>(`/service-templates${query({ station_id: stationId })}`),
    enabled: Boolean(stationId) && enabled,
  })
}

/** Adopte un modèle du référentiel : la prestation naît « à tarifer ». */
export function useAdoptTemplate() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      station_id: string
      template_code: string
      categories?: string[]
      price_xof?: number
      duration_minutes?: number
    }) => api.post('/services', { ...input, name: undefined }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['services'] })
      void queryClient.invalidateQueries({ queryKey: ['service-templates'] })
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
    },
  })
}

/** Familles de prestations que la station active. */
export function useUpdateServiceFamilies() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      stationId,
      families,
    }: {
      stationId: string
      families: Array<{ code: string; is_active?: boolean; name?: string; sort_order?: number }>
    }) => api.put(`/stations/${stationId}/service-families`, { families }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['service-templates'] })
      void queryClient.invalidateQueries({ queryKey: ['service-families'] })
    },
  })
}

/** À quels véhicules une prestation s'applique (vide = tous ceux acceptés). */
export function useServiceCompatibilities(serviceId: string, enabled = true) {
  return useQuery<{
    service_id: string
    categories: string[]
    categories_disponibles: Array<{ code: string; name: string }>
    message: string
  }>({
    queryKey: pricingKeys.compat(serviceId),
    queryFn: () => api.get(`/services/${serviceId}/compatibilities`),
    enabled: Boolean(serviceId) && enabled,
  })
}

export function useSetServiceCompatibilities() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ serviceId, categories }: { serviceId: string; categories: string[] }) =>
      api.put(`/services/${serviceId}/compatibilities`, { categories }),
    onSuccess: (_payload, variables) => {
      void queryClient.invalidateQueries({ queryKey: pricingKeys.compat(variables.serviceId) })
      void queryClient.invalidateQueries({ queryKey: ['services'] })
    },
  })
}

/** Options et suppléments facturables en plus d'une prestation. */
export function useServiceOptions(stationId?: string, serviceId?: string) {
  return useQuery<{ items: ServiceOption[]; total: number }>({
    queryKey: pricingKeys.options(stationId, serviceId),
    queryFn: () =>
      api.get<{ items: ServiceOption[]; total: number }>(
        `/service-options${query({ station_id: stationId, service_id: serviceId })}`,
      ),
    enabled: Boolean(stationId),
  })
}

export interface OptionInput {
  station_id: string
  name: string
  description?: string
  price_xof: number
  duration_minutes?: number
  service_id?: string
  category_code?: string
  sort_order?: number
}

export function useCreateServiceOption() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: OptionInput) => api.post<ServiceOption>('/service-options', input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['service-options'] }),
  })
}

export function useUpdateServiceOption() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: Partial<OptionInput> & { id: string; is_active?: boolean }) =>
      api.patch(`/service-options/${id}`, input),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['service-options'] }),
  })
}

/** Désactivation : une fiche passée peut citer l'option, elle reste lisible. */
export function useDisableServiceOption() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.delete(`/service-options/${id}`),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ['service-options'] }),
  })
}

/** Règles de prix posées sur une prestation. */
export function usePriceRules(stationId?: string, serviceId?: string) {
  return useQuery<{ items: PriceRule[]; total: number }>({
    queryKey: pricingKeys.rules(stationId, serviceId),
    queryFn: () =>
      api.get<{ items: PriceRule[]; total: number }>(
        `/price-rules${query({ station_id: stationId, service_id: serviceId })}`,
      ),
    enabled: Boolean(stationId),
  })
}

export interface PriceRuleInput {
  station_id: string
  service_id: string
  category_code?: string
  usage_code?: string
  customer_id?: string
  price_xof?: number
  valid_from?: string
  valid_to?: string
  reason: string
  is_active?: boolean
}

export function useCreatePriceRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: PriceRuleInput) => api.post<PriceRule>('/price-rules', input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['price-rules'] })
      void queryClient.invalidateQueries({ queryKey: ['price-matrix'] })
      void queryClient.invalidateQueries({ queryKey: ['services'] })
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
    },
  })
}

export function useUpdatePriceRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...input }: Partial<PriceRuleInput> & { id: string }) =>
      api.patch(`/price-rules/${id}`, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['price-rules'] })
      void queryClient.invalidateQueries({ queryKey: ['price-matrix'] })
    },
  })
}

export function useDisablePriceRule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.delete(`/price-rules/${id}`),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['price-rules'] })
      void queryClient.invalidateQueries({ queryKey: ['price-matrix'] })
    },
  })
}

/** La matrice : une ligne par prestation, une colonne par véhicule. */
export function usePriceMatrix(stationId?: string) {
  return useQuery<PriceMatrix>({
    queryKey: pricingKeys.matrix(stationId),
    queryFn: () => api.get<PriceMatrix>(`/price-rules/matrix${query({ station_id: stationId })}`),
    enabled: Boolean(stationId),
  })
}

export interface MatrixInput {
  stationId: string
  reason: string
  cells?: Array<{ service_id: string; category_code: string; price_xof?: number }>
  usageCells?: Array<{ service_id: string; usage_code: string; price_xof?: number }>
  /** Tarif du catalogue (le repli), modifié depuis le même écran et le même motif. */
  genericPrices?: Array<{ service_id: string; price_xof: number }>
}

/**
 * Enregistre une grille en bloc. Le motif est exigé par le serveur : une grille
 * tarifaire est une décision d'exploitation, elle doit pouvoir s'expliquer.
 */
export function useSavePriceMatrix() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ stationId, reason, cells, usageCells, genericPrices }: MatrixInput) =>
      api.put('/price-rules/matrix', {
        station_id: stationId,
        reason,
        cells: cells ?? [],
        usage_cells: usageCells ?? [],
        generic_prices: genericPrices ?? [],
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['price-matrix'] })
      void queryClient.invalidateQueries({ queryKey: ['price-rules'] })
      void queryClient.invalidateQueries({ queryKey: ['services'] })
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
    },
  })
}

/**
 * Options de requête de l'aperçu de prix.
 *
 * Les discriminants sont des **codes** (`berline`, `taxi_compteur`) : c'est la
 * clé de la grille tarifaire. Les identifiants restent transmis en repli, pour
 * un référentiel qui n'aurait pas encore de code. Le navigateur ne calcule
 * aucun prix : il demande, et affiche la provenance renvoyée.
 */
export function pricePreviewOptions(input: {
  stationId?: string
  serviceId?: string
  categoryCode?: string
  usageCode?: string
  categoryId?: string
  usageId?: string
  customerId?: string
  enabled?: boolean
}) {
  const { stationId, serviceId, categoryCode, usageCode, categoryId, usageId, customerId } = input
  const enabled = input.enabled ?? true
  return {
    queryKey: [
      'price-preview',
      stationId ?? '',
      serviceId ?? '',
      categoryCode ?? categoryId ?? '',
      usageCode ?? usageId ?? '',
      customerId ?? '',
    ],
    queryFn: () =>
      api.get<PricePreview>(
        `/price-preview${query({
          station_id: stationId,
          service_id: serviceId,
          // Un code connu prend la main ; l'identifiant ne sert que de repli.
          category_code: categoryCode || undefined,
          usage_code: usageCode || undefined,
          category_id: categoryCode ? undefined : categoryId || undefined,
          usage_id: usageCode ? undefined : usageId || undefined,
          customer_id: customerId,
        })}`,
      ),
    enabled: Boolean(stationId) && Boolean(serviceId) && enabled,
    staleTime: 15_000,
  } as const
}

/** Aperçu d'un prix avant d'enregistrer une fiche : la caisse ne calcule rien. */
export function usePricePreview(input: Parameters<typeof pricePreviewOptions>[0]) {
  return useQuery<PricePreview>(pricePreviewOptions(input))
}
