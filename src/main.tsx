import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from '@/App'
import { AuthProvider } from '@/lib/clerk'
// Point d'entrée des styles : sans cet import, Tailwind ne génère aucune classe
// et l'application s'affiche sans style (voir src/index.css).
import './index.css'

// Feuille de style globale : charge Tailwind et les tokens de couleur du thème.
import './index.css'
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
        <AuthProvider>
          <App />
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
