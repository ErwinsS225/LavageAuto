package main

import "net/http"

// withCORS autorise le navigateur à appeler l'API (cookies compris).
// En production, la liste est restreinte par CORS_ALLOWED_ORIGINS ; en
// développement, toutes les origines sont acceptées comme le faisait Express.
func withCORS(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				allowed := len(cfg.CORSAllowedOrigins) == 0
				for _, o := range cfg.CORSAllowedOrigins {
					if o == origin {
						allowed = true
						break
					}
				}
				if allowed {
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
