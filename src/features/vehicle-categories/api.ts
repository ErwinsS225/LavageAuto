/**
 * Catégories de véhicules et usages commerciaux.
 *
 * Deux référentiels distincts, comme côté serveur : la **catégorie** décrit le
 * véhicule (berline, gbaka, moto), l'**usage** décrit ce qu'on en fait (taxi
 * compteur, livraison, flotte). Une berline « taxi compteur » reste une
 * berline : c'est le serveur qui applique cette règle, l'interface ne fait que
 * proposer les combinaisons qu'il accepte.
 *
 * Le référentiel est commun au produit ; ce que la station active lui
 * appartient. L'interface n'invente donc jamais une liste : elle affiche ce que
 * le serveur renvoie, y compris une liste vide.
 */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, query } from "@/lib/api";
import type {
  UsageCategorie,
  VehicleCatalogue,
  VehicleCategory,
  VehicleOptions,
  VehicleUsage,
} from "@/types";

/** Clés de cache : un enregistrement d'activation doit rafraîchir les deux
 *  écrans qui lisent ces listes (mise en route, caisse) et l'état d'avancement. */
export const vehicleKeys = {
  categories: ["vehicle-categories"] as const,
  usages: ["vehicle-usages"] as const,
  options: ["vehicle-category-options"] as const,
  catalogue: ["vehicle-catalogue"] as const,
  couples: ["vehicle-usage-categories"] as const,
};

/**
 * Ce que la caisse propose : les catégories **et** les usages en un seul appel.
 * Deux requêtes séparées laisseraient un état intermédiaire à l'écran (une
 * catégorie choisie, la liste d'usages pas encore arrivée).
 */
export function useVehicleOptions(stationId?: string) {
  return useQuery<VehicleOptions>({
    queryKey: [...vehicleKeys.options, stationId ?? ""],
    queryFn: () =>
      api.get<VehicleOptions>(`/vehicle-category-options${query({ station_id: stationId })}`),
    staleTime: 5 * 60_000,
  });
}

/** Le référentiel complet + ce que la station en a fait (écran d'activation). */
export function useVehicleCatalogue(stationId?: string, actif = true) {
  return useQuery<VehicleCatalogue>({
    queryKey: [...vehicleKeys.catalogue, stationId ?? ""],
    queryFn: () =>
      api.get<VehicleCatalogue>(`/vehicle-catalogue${query({ station_id: stationId })}`),
    enabled: actif && Boolean(stationId),
    staleTime: 60_000,
  });
}

function invaliderTout(queryClient: ReturnType<typeof useQueryClient>, stationId: string) {
  for (const cle of [
    vehicleKeys.categories,
    vehicleKeys.usages,
    vehicleKeys.options,
    vehicleKeys.catalogue,
    vehicleKeys.couples,
  ]) {
    void queryClient.invalidateQueries({ queryKey: cle });
  }
  // L'avancement de la mise en route dépend de ces données : il se recalcule.
  void queryClient.invalidateQueries({ queryKey: ["onboarding"] });
  void queryClient.invalidateQueries({ queryKey: ["station-profile", stationId] });
}

export interface CategorieACocher {
  code: string;
  is_active: boolean;
  name?: string;
  sort_order?: number;
}

/** Activation en bloc des catégories du référentiel (écran d'activation). */
export function useSaveStationCategories(stationId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (categories: CategorieACocher[]) =>
      api.put<{ items: VehicleCategory[]; actives: number; message: string }>(
        `/stations/${stationId}/vehicle-categories`,
        { categories },
      ),
    onSuccess: () => invaliderTout(queryClient, stationId),
  });
}

export function useSaveStationUsages(stationId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (usages: { code: string; is_active: boolean; name?: string; sort_order?: number }[]) =>
      api.put<{ items: VehicleUsage[]; actifs: number; message: string }>(
        `/stations/${stationId}/vehicle-usages`,
        { usages },
      ),
    onSuccess: () => invaliderTout(queryClient, stationId),
  });
}

/** Catégorie propre à la station (le référentiel du produit n'est pas modifié). */
export function useCreateVehicleCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { station_id: string; name: string; family?: string; description?: string; aliases?: string[] }) =>
      api.post<{ items: VehicleCategory[] }>("/vehicle-categories", input),
    onSuccess: (_data, variables) => invaliderTout(queryClient, variables.station_id),
  });
}

export function useUpdateVehicleCategory() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      code,
      station_id,
      ...input
    }: {
      code: string;
      station_id: string;
      name?: string;
      description?: string;
      aliases?: string[];
      sort_order?: number;
      is_active?: boolean;
    }) => api.patch<{ items: VehicleCategory[] }>(`/vehicle-categories/${code}`, input),
    onSuccess: (_data, variables) => invaliderTout(queryClient, variables.station_id),
  });
}

export function useUsageCategories(stationId?: string, actif = true) {
  return useQuery<{ items: UsageCategorie[]; total: number; station_id: string }>({
    queryKey: [...vehicleKeys.couples, stationId ?? ""],
    queryFn: () =>
      api.get<{ items: UsageCategorie[]; total: number; station_id: string }>(
        `/vehicle-usage-categories${query({ station_id: stationId })}`,
      ),
    enabled: actif && Boolean(stationId),
    staleTime: 60_000,
  });
}

/** Déclare les combinaisons véhicule × usage réellement proposées. */
export function useSaveUsageCategories(stationId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (couples: { usage_code: string; category_code: string }[]) =>
      api.put<{ items: UsageCategorie[]; total: number }>(
        `/stations/${stationId}/vehicle-usage-categories`,
        { couples },
      ),
    onSuccess: () => invaliderTout(queryClient, stationId),
  });
}
