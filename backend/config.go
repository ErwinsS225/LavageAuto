package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// loadDotEnv charge le fichier .env (projet ou backend) s'il existe.
// Les variables déjà présentes dans l'environnement ne sont pas écrasées.
// Le parseur maison tolère les espaces autour de '=' et les valeurs
// multi-lignes (clé publique PEM), que godotenv gère mal.
func loadDotEnv() {
	candidates := []string{
		".env",
		filepath.Join("..", ".env"),
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			applyEnvFile(string(data))
			return
		}
	}
}

// applyEnvFile parse un contenu .env et pose les variables manquantes.
func applyEnvFile(content string) {
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// Valeur multi-lignes (PEM) : on concatène tant que la valeur ne
		// se termine pas de manière crédible (fin de clé publique).
		if strings.HasPrefix(value, "-----BEGIN") && !strings.Contains(value, "END PUBLIC KEY-----") {
			for i+1 < len(lines) {
				i++
				value += "\n" + lines[i]
				if strings.Contains(lines[i], "-----END") {
					break
				}
			}
		}
		value = strings.Trim(value, `"'`)
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

// Config regroupe toute la configuration lue dans l'environnement (.env).
type Config struct {
	Env         string // development | production
	Host        string
	Port        int
	DatabaseURL string
	// Neon (base distante) — NEON_DATABASE_URL est prioritaire sur DATABASE_URL.
	NeonDatabaseURL string
	// Clerk (authentification).
	ClerkSecretKey         string
	ClerkPublishableKey    string
	ClerkJWKSURL           string
	ClerkPublicKey         string
	ClerkAuthorizedParties []string
	ClerkEnabled           bool
	StaticDir              string
	CORSAllowedOrigins     []string
	SessionCookieSecure    bool
	SessionCookieSameSite  string // "lax" (défaut) ou "none" (front sur un autre domaine)
	SessionTTLHours        int
	AuthMaxFailures        int // échecs de connexion tolérés par fenêtre
	RegisterMaxPerHour     int // inscriptions tolérées par heure et par IP
	SeedDemo               bool
	PlatformAdminPassword  string
}

func loadConfig() Config {
	cfg := Config{
		Env:                   envOr("APP_ENV", "development"),
		Host:                  envOr("API_HOST", "0.0.0.0"),
		Port:                  envInt("API_PORT", 3000),
		DatabaseURL:           envOr("DATABASE_URL", os.Getenv("NEON_DATABASE_URL")),
		StaticDir:             os.Getenv("STATIC_DIR"),
		SessionCookieSecure:   os.Getenv("SESSION_COOKIE_SECURE") == "true",
		SessionCookieSameSite: strings.ToLower(envOr("SESSION_COOKIE_SAMESITE", "lax")),
		SessionTTLHours:       envInt("SESSION_TTL_HOURS", 720), // 30 jours
		AuthMaxFailures:       envInt("AUTH_MAX_FAILURES", 10),
		RegisterMaxPerHour:    envInt("REGISTER_MAX_PER_HOUR", 5),
		SeedDemo:              os.Getenv("SEED_DEMO") != "false",
		// Compte de démonstration de l'espace plateforme : il n'est créé que
		// hors production (voir seedDemo) et sert aux essais locaux.
		PlatformAdminPassword: envOr("PLATFORM_ADMIN_PASSWORD", "admin1234"),
	}
	if raw := os.Getenv("CORS_ALLOWED_ORIGINS"); raw != "" {
		for _, o := range splitCSV(raw) {
			cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, o)
		}
	}

	// Clerk : secret + JWKS. Actif dès que CLERK_SECRET_KEY est définie.
	cfg.ClerkSecretKey = os.Getenv("CLERK_SECRET_KEY")
	cfg.ClerkPublishableKey = os.Getenv("NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY")
	cfg.ClerkJWKSURL = envOr("JWKS_URL_CLERK", os.Getenv("NEXT_PUBLIC_CLERK_API_URL"))
	// Clé publique statique (PEM, optionnelle) : elle permet de vérifier les
	// jetons sans interroger l'adresse JWKS — utile quand la station n'a pas
	// d'accès réseau permanent. L'adresse JWKS reste prioritaire car elle
	// suit la rotation des clés de Clerk.
	cfg.ClerkPublicKey = envOr("JWKS_PUBLIC_KEY", os.Getenv("CLERK_PUBLIC_KEY"))
	// Origines autorisées du jeton Clerk (claim `azp`) : la variable dédiée si
	// elle est posée, sinon l'URL publique de l'application et les origines
	// déjà déclarées pour le CORS. Sans cette liste, un jeton Clerk émis pour
	// une autre application serait accepté.
	for _, o := range splitCSV(os.Getenv("CLERK_AUTHORIZED_PARTIES")) {
		cfg.ClerkAuthorizedParties = append(cfg.ClerkAuthorizedParties, o)
	}
	if appURL := os.Getenv("APP_URL"); appURL != "" {
		cfg.ClerkAuthorizedParties = append(cfg.ClerkAuthorizedParties, strings.TrimRight(appURL, "/"))
	}
	cfg.ClerkAuthorizedParties = append(cfg.ClerkAuthorizedParties, cfg.CORSAllowedOrigins...)
	cfg.ClerkEnabled = cfg.ClerkSecretKey != ""
	if cfg.ClerkEnabled && cfg.ClerkJWKSURL == "" && cfg.ClerkPublishableKey != "" {
		cfg.ClerkJWKSURL = jwksFromPublishableKey(cfg.ClerkPublishableKey)
	}
	return cfg
}

// jwksFromPublishableKey déduit l'URL JWKS depuis la clé publishable
// (pk_live_xxx → https://api.clerk.com/v1/live/…/jwks.json).
func jwksFromPublishableKey(pk string) string {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(pk, "pk_"))
	if err != nil {
		return ""
	}
	parts := strings.SplitN(string(decoded), "#", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("https://%s/.well-known/jwks.json", parts[1])
}

func (c Config) ListenAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func splitCSV(s string) []string {
	var out []string
	current := ""
	for _, r := range s {
		if r == ',' {
			if current != "" {
				out = append(out, current)
			}
			current = ""
			continue
		}
		if r != ' ' {
			current += string(r)
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}
