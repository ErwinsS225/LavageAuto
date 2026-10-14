package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Limitation de débit : protège les routes d'authentification contre les
// tentatives en boucle (mot de passe deviné à la main) sans dépendance
// externe. Fenêtre glissante simple, en mémoire du processus.
//
// Le compteur ne compte que les **échecs** : une connexion réussie remet les
// compteurs à zéro, et une consultation normale n'est jamais ralentie.
// ---------------------------------------------------------------------------

type limiterEntry struct {
	failures []time.Time
	blocked  time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	entries  map[string]*limiterEntry
	limit    int           // échecs autorisés par fenêtre
	window   time.Duration // durée de la fenêtre
	cooldown time.Duration // blocage après dépassement
}

func newRateLimiter(limit int, window, cooldown time.Duration) *rateLimiter {
	return &rateLimiter{
		entries:  map[string]*limiterEntry{},
		limit:    limit,
		window:   window,
		cooldown: cooldown,
	}
}

// exceeded indique, sans consommer de tentative, que la clé est bloquée.
func (l *rateLimiter) exceeded(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		return false, 0
	}
	now := time.Now()
	if now.Before(entry.blocked) {
		return true, entry.blocked.Sub(now)
	}
	// Purge des échecs sortis de la fenêtre.
	kept := entry.failures[:0]
	for _, at := range entry.failures {
		if now.Sub(at) < l.window {
			kept = append(kept, at)
		}
	}
	entry.failures = kept
	if len(entry.failures) >= l.limit {
		entry.blocked = now.Add(l.cooldown)
		entry.failures = entry.failures[:0]
		return true, l.cooldown
	}
	return false, 0
}

// charge enregistre un échec pour la clé.
func (l *rateLimiter) charge(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		entry = &limiterEntry{}
		l.entries[key] = entry
	}
	entry.failures = append(entry.failures, time.Now())
}

// forget efface le compteur (connexion réussie).
func (l *rateLimiter) forget(key string) {
	l.mu.Lock()
	delete(l.entries, key)
	l.mu.Unlock()
}

// loginLimiter : 10 échecs par quart d'heure et par couple IP + adresse, puis
// 15 minutes de pause. Assez large pour une faute de frappe, trop court pour
// deviner un mot de passe.
var loginLimiter = newRateLimiter(10, 15*time.Minute, 15*time.Minute)

// registerLimiter : 5 inscriptions par heure et par IP (une station n'a aucune
// raison d'en créer plus, un script malveillant n'a plus de porte).
var registerLimiter = newRateLimiter(5, time.Hour, time.Hour)

// configureRateLimits applique les valeurs de la configuration (les limites
// par défaut protègent sans gêner l'exploitation ; elles restent réglables
// pour les environnements de recette).
func configureRateLimits(cfg Config) {
	if cfg.AuthMaxFailures > 0 {
		loginLimiter.limit = cfg.AuthMaxFailures
	}
	if cfg.RegisterMaxPerHour > 0 {
		registerLimiter.limit = cfg.RegisterMaxPerHour
	}
}

// clientIP lit l'adresse réelle derrière un éventuel proxy inverse.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := splitCSV(forwarded)
		if len(parts) > 0 {
			return parts[0]
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitRegistration limite les créations de compte anonymes par IP.
func (a *App) rateLimitRegistration(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if blocked, retry := registerLimiter.exceeded("reg:" + clientIP(r)); blocked {
			writeTooManyAttempts(w, retry)
			return
		}
		registerLimiter.charge("reg:" + clientIP(r))
		next(w, r)
	}
}

// writeTooManyAttempts : réponse homogène du limiteur.
func writeTooManyAttempts(w http.ResponseWriter, retry time.Duration) {
	minutes := int(retry.Minutes()) + 1
	writeError(w, http.StatusTooManyRequests, "too_many_requests",
		"Trop de tentatives. Merci de réessayer dans "+itoa(minutes)+" minute(s).", nil)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
