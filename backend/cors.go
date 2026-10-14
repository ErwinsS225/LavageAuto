package main

import (
	"net/http"
	"strings"
)

// originAutorisee : liste explicite si elle est fournie ; sinon, en
// développement uniquement, les origines locales (le front Vite en fait
// partie). Aucun autre cas ne reçoit d'en-tête CORS : une API avec cookies ne
// doit jamais répondre « toutes origines ».
func originAutorisee(cfg Config, origin string) bool {
	if len(cfg.CORSAllowedOrigins) > 0 {
		for _, o := range cfg.CORSAllowedOrigins {
			if strings.EqualFold(strings.TrimRight(o, "/"), strings.TrimRight(origin, "/")) {
				return true
			}
		}
		return false
	}
	if cfg.Env == "production" {
		return false
	}
	return strings.HasPrefix(origin, "http://localhost:") ||
		strings.HasPrefix(origin, "http://127.0.0.1:") ||
		strings.HasPrefix(origin, "http://[::1]:")
}

// withCORS autorise le navigateur à appeler l'API (cookies compris).
// En production, la liste est restreinte par CORS_ALLOWED_ORIGINS ; en
// développement, toutes les origines sont acceptées comme le faisait Express.
func withCORS(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				// La réponse dépend de l'origine : indispensable au cache.
				w.Header().Add("Vary", "Origin")
				if originAutorisee(cfg, origin) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
