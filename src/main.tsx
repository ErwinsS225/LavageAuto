import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MotionConfig } from 'framer-motion'

import App from '@/App'
import ErrorBoundary from '@/components/ErrorBoundary'
import { ToastProvider } from '@/components/Toaster'
import { AuthProvider } from '@/lib/clerk'
import { StationProvider, ThemeProvider } from '@/hooks/useStation'
// Point d'entrée des styles : compile Tailwind et définit les tokens du thème
// (clair/sombre). Sans cet import, le build ne produit aucune feuille de style
// et l'application s'affiche sans aucun style (voir src/index.css).
import './index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { refetchOnWindowFocus: false, retry: 1 },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ThemeProvider>
          {/* Un seul fournisseur de notifications pour toute l'application :
              les écrans publics (connexion, inscription) affichent aussi leurs
              messages, tout en restant utilisables sans. */}
          <ToastProvider>
            <AuthProvider>
              <StationProvider>
                {/* Sans cette barrière, une erreur de rendu laisse une page
                    vide (« écran noir ») au lieu d'un message exploitable. */}
                <ErrorBoundary>
                  {/* reducedMotion="user" : les animations sont neutralisées
                      pour qui a demandé « réduire les animations » dans son
                      système. Les transitions restent alors instantanées
                      plutôt que supprimées. */}
                  <MotionConfig reducedMotion="user">
                    <App />
                  </MotionConfig>
                </ErrorBoundary>
              </StationProvider>
            </AuthProvider>
          </ToastProvider>
        </ThemeProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
