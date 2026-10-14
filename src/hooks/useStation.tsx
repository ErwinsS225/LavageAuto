/**
 * Station de travail (sélection partagée) et thème clair/sombre.
 *
 * Le fournisseur charge la liste des stations **une seule fois** et la met à
 * disposition de toute l'application : le sélecteur de l'en-tête, les écrans
 * qui filtrent par station et les formulaires lisent tous la même source.
 */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'

import { useAuth } from '@/hooks/useAuth'
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
// Station courante
// ---------------------------------------------------------------------------

interface StationContextValue {
  stationId: string
  station: Station | null
  stations: Station[]
  selectStation: (id: string) => void
  loading: boolean
}

const StationContext = createContext<StationContextValue>({
  stationId: '',
  station: null,
  stations: [],
  selectStation: () => { },
  loading: false,
})

export function useStation(): StationContextValue {
  return useContext(StationContext)
}

export function StationProvider({ children }: { children: ReactNode }) {
  // La station de la session (`/auth/me`) est connue **avant** la liste des
  // stations. Elle sert de repli : sans lui, un écran qui a besoin d'un
  // identifiant de station restait vide le temps du second appel — ou
  // définitivement si la liste ne se chargeait pas.
  const { me } = useAuth()
  const [stationChoisie, setStationId] = useState<string>(() => {
    try {
      return localStorage.getItem(STORAGE_KEY) ?? ''
    } catch {
      return ''
    }
  })

  // Le serveur renvoie un tableau de stations (celles de l'entreprise, ou la
  // seule station du compte pour un caissier).
  const query = useQuery<Station[]>({
    queryKey: ['stations'],
    queryFn: () => api.get<Station[]>('/stations'),
    staleTime: 5 * 60_000,
  })
  const stations = useMemo(() => query.data ?? [], [query.data])

  const stationId = stationChoisie || me?.station?.id || ''

  // Aucun choix explicite : la station marquée par défaut, sinon la première.
  useEffect(() => {
    if (stationChoisie && stations.some((station) => station.id === stationChoisie)) return
    if (stations.length === 0) return
    const fallback = stations.find((station) => station.is_default) ?? stations[0]
    if (fallback) setStationId(fallback.id)
  }, [stationChoisie, stations])

  const selectStation = useCallback((id: string) => {
    setStationId(id)
    try {
      localStorage.setItem(STORAGE_KEY, id)
    } catch {
      // Ignoré : le choix ne sera pas conservé entre les visites.
    }
  }, [])

  const station = stations.find((item) => item.id === stationId) ?? null

  const value = useMemo<StationContextValue>(
    () => ({
      stationId,
      // La station complète n'existe que dans la liste ; le repli de session ne
      // fournit que l'identifiant, donc `station` peut rester null un instant.
      station: station ?? me?.station ?? null,
      stations,
      selectStation,
      loading: query.isLoading,
    }),
    [stationId, station, stations, selectStation, query.isLoading, me],
  )

  return <StationContext.Provider value={value}>{children}</StationContext.Provider>
}
