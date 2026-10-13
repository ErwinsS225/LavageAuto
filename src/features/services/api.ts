/**
 * Catalogue des prestations (tarifs) d'une station.
 */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, query } from "@/lib/api";
import type { Service } from "@/types";

export const servicesKey = (filters?: Record<string, unknown>) =>
  ["services", filters ?? {}] as const;

export function useStationServices(filters: { station_id?: string } = {}) {
  return useQuery<{ items: Service[]; total: number }>({
    queryKey: servicesKey(filters as Record<string, unknown>),
    queryFn: () =>
      api.get<{ items: Service[]; total: number }>(
        `/services${query({ ...filters })}`,
      ),
  });
}

export interface ServiceInput {
  station_id?: string;
  name: string;
  category?: string;
  description?: string;
  price_xof: number;
  duration_minutes?: number;
  sort_order?: number;
  is_active?: boolean;
}

export function useCreateService() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ServiceInput) => api.post<Service>("/services", input),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: ["services"] }),
  });
}

export function useUpdateService() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: ServiceInput & { id: string }) =>
      api.patch<Service>(`/services/${id}`, input),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: ["services"] }),
  });
}

/** Désactivation (soft delete) : le service reste dans l'historique. */
export function useDisableService() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.delete(`/services/${id}`),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: ["services"] }),
  });
}
