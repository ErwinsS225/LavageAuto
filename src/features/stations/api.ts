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

export interface StationCreateInput {
  name: string
  code?: string
  phone?: string
  address?: string
  city?: string
  commune?: string
  district_area?: string
  timezone?: string
  bays?: number
  water_source?: string
  water_reserve_liters?: number
  wash_types?: string[]
}

export function useCreateStation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: StationCreateInput) => api.post<Station>('/stations', input),
    onSuccess: (station) => {
      void queryClient.invalidateQueries({ queryKey: ['stations'] })
      void queryClient.invalidateQueries({ queryKey: ['me'] })
      void queryClient.invalidateQueries({ queryKey: ['onboarding'] })
      return station
    },
  })
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

