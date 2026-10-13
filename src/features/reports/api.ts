/**
 * Rapports : vue d'ensemble sur période et export CSV.
 */
import { useQuery } from "@tanstack/react-query";
import { api, query } from "@/lib/api";
import type { DashboardData, ReportOverview } from "@/types";

/** Tableau de bord du jour : totaux, série, caisse, employés, alertes. */
export function useDashboard(filters: { station_id?: string } = {}) {
  return useQuery<DashboardData>({
    queryKey: ["dashboard", filters],
    queryFn: () =>
      api.get<DashboardData>(`/reports/dashboard${query({ ...filters })}`),
  });
}

export function useReportOverview(filters: {
  from: string;
  to: string;
  station_id?: string;
}) {
  return useQuery<ReportOverview>({
    queryKey: ["report-overview", filters],
    queryFn: () =>
      api.get<ReportOverview>(`/reports/overview${query({ ...filters })}`),
  });
}

/** L'export renvoie un CSV : ouverture dans un nouvel onglet / téléchargement. */
export function reportExportUrl(filters: {
  from: string;
  to: string;
  station_id?: string;
}): string {
  const search = new URLSearchParams();
  search.set("from", filters.from);
  search.set("to", filters.to);
  if (filters.station_id) search.set("station_id", filters.station_id);
  return `/api/v1/reports/export?${search.toString()}`;
}
