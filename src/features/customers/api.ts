/**
 * Clients : options pour les formulaires (liste compacte).
 */
import { useQuery } from "@tanstack/react-query";
import { api, query } from "@/lib/api";

export interface CustomerOption {
  id: string;
  full_name: string;
  phone: string;
  plate: string;
}

export function useCustomersOptions(
  filters: { station_id?: string; search?: string } = {},
) {
  return useQuery<{ options: CustomerOption[] }>({
    queryKey: ["customers-options", filters],
    queryFn: () =>
      api.get<{ options: CustomerOption[] }>(
        `/customers/options${query({ ...filters })}`,
      ),
    staleTime: 60_000,
  });
}
