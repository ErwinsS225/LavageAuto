/**
 * useStation : station de travail courante (persistée localement).
 * Par défaut : la station du compte, sinon la première de l'organisation.
 */
import { useCallback, useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { Station } from "@/types";

const STORAGE_KEY = "washguard-station-id";

interface StationsResponse {
  items: Station[];
  total: number;
}

export function useStation() {
  const [stationId, setStationId] = useState<string>(
    () => localStorage.getItem(STORAGE_KEY) ?? "",
  );

  const query = useQuery<StationsResponse>({
    queryKey: ["stations"],
    queryFn: () => api.get<StationsResponse>("/stations"),
    staleTime: 5 * 60_000,
  });

  const stations = query.data?.items ?? [];

  // Pas de choix explicite : on prend la station du compte puis la première.
  useEffect(() => {
    if (stationId) return;
    if (stations.length === 0) return;
    const fallback =
      stations.find((station) => station.is_default) ?? stations[0];
    if (fallback) setStationId(fallback.id);
  }, [stationId, stations]);

  const selectStation = useCallback((id: string) => {
    setStationId(id);
    try {
      localStorage.setItem(STORAGE_KEY, id);
    } catch {
      // Ignoré : le choix ne sera pas conservé entre les visites.
    }
  }, []);

  const station = stations.find((item) => item.id === stationId) ?? null;

  return {
    stationId,
    station,
    stations,
    selectStation,
    loading: query.isLoading,
  };
}
