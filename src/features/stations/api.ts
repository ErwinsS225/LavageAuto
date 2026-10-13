/**
 * Stations : liste et mise à jour des paramètres.
 */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { Station } from "@/types";

export const stationsKey = (filters?: Record<string, unknown>) =>
  ["stations", filters ?? {}] as const;

export function useStations(enabled: boolean) {
  return useQuery<Station[]>({
    queryKey: stationsKey(),
    queryFn: () => api.get<Station[]>("/stations"),
    enabled,
    staleTime: 5 * 60_000,
  });
}

export interface StationUpdateInput {
  name?: string;
  phone?: string;
  address?: string;
  city?: string;
  is_default?: boolean;
  is_active?: boolean;
  settings?: Partial<Station["settings"]>;
}

export function useUpdateStation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...input }: StationUpdateInput & { id: string }) =>
      api.patch<Station>(`/stations/${id}`, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["stations"] });
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
}
