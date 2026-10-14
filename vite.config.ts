import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    host: '0.0.0.0',
    port: 3000,
    // Vite refuse les requêtes dont l'en-tête Host n'est pas local (protection
    // contre le rebinding DNS) : un aperçu servi sous un domaine distant est
    // donc bloqué tant qu'il n'est pas déclaré. Les hôtes attendus se listent
    // dans VITE_ALLOWED_HOSTS, séparés par des virgules — rien n'est élargi par
    // défaut, et la production n'est pas concernée (elle sert dist/ derrière
    // l'API Go).
    allowedHosts: process.env.VITE_ALLOWED_HOSTS
      ? process.env.VITE_ALLOWED_HOSTS.split(',').map((hote) => hote.trim()).filter(Boolean)
      : undefined,
    // En développement, l'API Go tourne sur le port 8080 (voir scripts/dev.sh).
    // Vite relaie /api et /health vers elle : le front n'a qu'une seule origine.
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 900,
  },
})
