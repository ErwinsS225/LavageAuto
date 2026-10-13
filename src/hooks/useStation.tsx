/**
 * useStation : sélection de la station de travail + thème (clair/sombre).
 * Le layout importe les trois ici : useStation, ThemeProvider, useTheme.
 */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import type { Station } from '@/types'

const STORAGE_KEY = 'washguard-station-id'

// ---------------------------------------------------------------------------
// Thème
// ---------------------------------------------------------------------------

type Theme = 'light' | 'dark'

interface ThemeContextValue {
  theme: Theme
  toggle: () => void
}

const ThemeContext = createContext<ThemeContextValue>({ theme: 'light', toggle: () => { } })

function readInitialTheme(): Theme {
  try {
    const stored = localStorage.getItem('washguard-theme')
    if (stored === 'dark' || stored === 'light') return stored
  } catch {
    // localStorage indisponible : préférence système.
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(readInitialTheme)

  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
    try {
      localStorage.setItem('washguard-theme', theme)
    } catch {
      // Ignoré : le thème se réinitialisera à la prochaine visite.
    }
  }, [theme])

  const value = useMemo<ThemeContextValue>(
    () => ({ theme, toggle: () => setTheme((current) => (current === 'dark' ? 'light' : 'dark')) }),
    [theme],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

export function useTheme(): ThemeContextValue {
  return useContext(ThemeContext)
}

// ---------------------------------------------------------------------------
// Stations
// ---------------------------------------------------------------------------

interface StationContextValue {
  stationId: string
  station: Station | null
  selectStation: (id: string) => void
}

const StationContext = createContext<StationContextValue>({
  stationId: '',
  station: null,
  selectStation: () => { },
})

export function useStation(): StationContextValue {
  return useContext(StationContext)
}

export function useStations(enabled: boolean): Station[] {
  const { setStations } = useStationSource()
  const query = useQuery<Station[]>({
    queryKey: ['stations-list'],
    queryFn: () => api.get<Station[]>('/stations'),
    enabled,
    staleTime: 5 * 60_000,
  })
  useEffect(() => {
    setStations(query.data ?? [])
  }, [query.data, setStations])
  return query.data ?? []
}

/**
 * Source partagée : le layout charge la liste et fournit la sélection à toute
 * l'application via le contexte.
 */
function useStationSource() {
  const context = useContext(StationSourceContext)
  if (!context) throw new Error('StationSourceProvider manquant')
  return context
}

const StationSourceContext = createContext<{
  stations: Station[]
  stationId: string
  selectStation: (id: string) => void
  setStations: (list: Station[]) => void
} | null>(null)

export function StationProvider({ children }: { children: ReactNode }) {
  const [stations, setStations] = useState<Station[]>([])
  const [stationId, setStationId] = useState<string>(() => localStorage.getItem(STORAGE_KEY) ?? '')

  // Fallback : première station (ou celle marquée par défaut) quand aucun choix.
  useEffect(() => {
    if (stationId || stations.length === 0) return
    const fallback = stations.find((station) => station.is_default) ?? stations[0]
    if (fallback) setStationId(fallback.id)
  }, [stationId, stations])

  const selectStation = useCallback((id: string) => {
    setStationId(id)
    try {
      localStorage.setItem(STORAGE_KEY, id)
    } catch {
      // Ignoré : le choix ne sera pas conservé entre les visites.
    }
  }, [])

  const value = useMemo(
    () => ({ stations, stationId, selectStation, setStations }),
    [stations, stationId, selectStation],
  )

  // Le contexte de consommation expose stationId + station (objet résolu).
  const station = stations.find((item) => item.id === stationId) ?? null
  const consumer = useMemo(() => ({ stationId, station, selectStation }), [stationId, station, selectStation])

  return (
    <StationSourceContext.Provider value={value}>
      <StationContext.Provider value={consumer}>{children}</StationContext.Provider>
    </StationSourceContext.Provider>
  )
}
