/**
 * Thème clair / sombre : classe `dark` sur <html>, préférence persistée.
 * Le CSS (Tailwind) utilise dark: et les tokens bg-surface, text-ink, etc.
 */
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'

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
    // localStorage indisponible : on retombe sur les préférences système.
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
