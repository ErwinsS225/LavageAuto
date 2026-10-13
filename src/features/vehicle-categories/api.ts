/**
 * Catégories de véhicules (référentiel global).
 */
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { VehicleCategory } from "@/types";

export function useVehicleCategories() {
  return useQuery<{ items: VehicleCategory[] }>({
    queryKey: ["vehicle-categories"],
    queryFn: () => api.get<{ items: VehicleCategory[] }>("/vehicle-categories"),
    staleTime: 10 * 60_000,
  });
}
