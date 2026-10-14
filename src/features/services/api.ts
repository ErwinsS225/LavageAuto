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
  /**
   * Obligatoire à la création (le serveur refuse un nom vide) ; facultatif en
   * mise à jour, où l'on ne transmet que ce qui change — un tarif, par exemple.
   */
  name?: string;
  category?: string;
  description?: string;
  /**
   * Jamais transmis : depuis le lot 5, un prix de vente ne s'écrit que par la
   * grille tarifaire (`PUT /price-rules/matrix`), avec son motif et sa trace.
   * Une prestation naît donc toujours « à tarifer » (`price_xof = 0`).
   */
  price_xof?: number | null;
  duration_minutes?: number;
  sort_order?: number;
  is_active?: boolean;
  /** Gratuité décidée : elle efface le tarif du catalogue. */
  is_free?: boolean;
  /** Prestation chiffrée sur devis : invendable au comptoir. */
  is_quote_only?: boolean;
  /** Reprendre un modèle du référentiel (jamais un prix : un modèle n'en a pas). */
  template_code?: string;
  /** Véhicules concernés — vide = tous ceux que la station accepte. */
  categories?: string[];
  includes?: string[];
  equipment_notes?: string;
  safety_notes?: string;
  preconditions?: string;
  /** Motif d'une modification de tarif (le serveur l'exige pour les règles). */
  reason?: string;
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
