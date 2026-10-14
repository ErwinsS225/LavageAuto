/**
 * Employés : liste, activité, création, mise à jour, réinitialisation.
 */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, query } from "@/lib/api";
import type {
  CommissionMode,
  Employee,
  EmploymentStatus,
  Role,
} from "@/types";

export const employeesKey = (filters?: Record<string, unknown>) =>
  ["employees", filters ?? {}] as const;

export interface EmployeeListResponse {
  items: Employee[];
  total: number;
}

export function useEmployees(
  filters: { status?: string; station_id?: string } = {},
) {
  return useQuery<EmployeeListResponse>({
    queryKey: employeesKey(filters as Record<string, unknown>),
    queryFn: () =>
      api.get<EmployeeListResponse>(`/employees${query({ ...filters })}`),
  });
}

export interface EmployeeOption {
  id: string;
  full_name: string;
  role: Role;
  /** Libellé lisible du rôle (« Caissier », « Laveur »…), fourni par le serveur. */
  role_label: string;
}

export function useEmployeeOptions(filters: { station_id?: string } = {}) {
  return useQuery<{ options: EmployeeOption[] }>({
    queryKey: ["employee-options", filters],
    queryFn: () =>
      api.get<{ options: EmployeeOption[] }>(
        `/employees/options${query({ ...filters })}`,
      ),
    staleTime: 60_000,
  });
}

export interface EmployeeActivityResponse {
  employee: { id: string; full_name: string };
  range: { from: string; to: string };
  totals: {
    wash_count: number;
    completed_count: number;
    canceled_count: number;
    revenue_xof: number;
    collected_xof: number;
    discount_xof: number;
  };
  orders: Array<{
    id: string;
    reference: string;
    created_at: string;
    customer_name: string;
    vehicle_category_name: string;
    plate: string;
    status: string;
    total_xof: number;
    due_xof: number;
  }>;
}

export function useEmployeeActivity(
  id: string | null,
  from: string,
  to: string,
) {
  return useQuery<EmployeeActivityResponse>({
    queryKey: ["employee-activity", id, from, to],
    queryFn: () =>
      api.get<EmployeeActivityResponse>(
        `/employees/${id}/activity${query({ from, to })}`,
      ),
    enabled: Boolean(id),
  });
}

export interface EmployeeInput {
  full_name: string;
  email?: string;
  phone?: string;
  role: Role;
  station_id?: string;
  /**
   * Description de la personne : facultative, et volontairement séparée de la
   * rémunération, qui demande une permission dédiée côté serveur.
   */
  job_title?: string;
  hired_on?: string;
  work_schedule?: string;
  employment_status?: EmploymentStatus;
  commission_mode?: CommissionMode;
  commission_per_wash_xof?: number;
  commission_percent?: number;
}

/** Résultat de création : mot de passe provisoire à communiquer une fois. */
export interface EmployeeCreated {
  id: string;
  email: string;
  temporary_password: string;
}

export function useCreateEmployee() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: EmployeeInput) =>
      api.post<EmployeeCreated>("/employees", input),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: ["employees"] }),
  });
}

export interface EmployeeUpdateInput extends Partial<EmployeeInput> {
  id: string;
  is_active?: boolean;
}

export function useUpdateEmployee() {
  return useMutation({
    mutationFn: ({ id, ...input }: EmployeeUpdateInput) =>
      api.patch<Employee>("/employees/" + id, input),
  });
}

export function useResetEmployeePassword() {
  return useMutation({
    mutationFn: (id: string) =>
      api.post<EmployeeCreated>(`/employees/${id}/reset-password`),
  });
}
