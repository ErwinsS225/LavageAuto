import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { WashOrder } from '@/types'

export interface WasherTask extends WashOrder {
  incident_note?: string
  incident_reported_at?: string
  incident_reporter_name?: string
}

export interface WasherPerformance {
  washes_today: number
  washes_month: number
  commission_today_xof: number
  commission_month_xof: number
  commission_rate_label: string
}

export const washerTasksKey = ['washer', 'tasks'] as const
export const washerPerformanceKey = ['washer', 'performance'] as const

export function useWasherTasks() {
  return useQuery<WasherTask[]>({
    queryKey: washerTasksKey,
    queryFn: async () => {
      const res = await api.get<{ items: WasherTask[] }>('/washer/tasks')
      return res.items
    },
    refetchInterval: 15_000,
  })
}

export function useWasherPerformance() {
  return useQuery<WasherPerformance>({
    queryKey: washerPerformanceKey,
    queryFn: () => api.get<WasherPerformance>('/washer/me/performance'),
  })
}

export function useUpdateTaskStatus() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, status }: { id: string; status: 'in_progress' | 'completed' }) =>
      api.post<WasherTask>(`/washer/tasks/${id}/status`, { status }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: washerTasksKey })
      void queryClient.invalidateQueries({ queryKey: washerPerformanceKey })
      void queryClient.invalidateQueries({ queryKey: ['wash-orders'] })
      void queryClient.invalidateQueries({ queryKey: ['dashboard'] })
    },
  })
}

export function useReportTaskIncident() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, incident_note }: { id: string; incident_note: string }) =>
      api.post<WasherTask>(`/washer/tasks/${id}/incident`, { incident_note }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: washerTasksKey })
      void queryClient.invalidateQueries({ queryKey: ['wash-orders'] })
      void queryClient.invalidateQueries({ queryKey: ['wash-order'] })
    },
  })
}
