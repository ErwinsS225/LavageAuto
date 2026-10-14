package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Rôles et permissions — la matrice de référence du serveur. Le front lit la
// même liste via /auth/me pour masquer des écrans, mais le refus vient
// toujours d'ici.
var rolePermissions = map[string][]string{
	"superadmin": {
		"platform:manage", "organization:manage", "settings:view", "settings:manage",
		"station:view", "subscription:view", "subscription:manage", "audit:view",
		"employee:view", "employee:manage", "employee:compensation",
		"service:view", "service:manage", "customer:view", "customer:create", "customer:manage",
		"wash:view", "wash:create", "wash:update", "wash:assign", "wash:cancel",
		"cash:view", "cash:open", "cash:close", "cash:reopen",
		"payment:view", "payment:create", "payment:void", "payment:create_deferred",
		"expense:view", "expense:create", "expense:approve", "expense:void",
		"dashboard:view", "report:view", "report:export", "commission:view",
	},
	"owner": {
		"organization:manage", "settings:view", "settings:manage",
		"station:view", "subscription:view", "subscription:manage", "audit:view",
		"employee:view", "employee:manage", "employee:compensation",
		"service:view", "service:manage", "customer:view", "customer:create", "customer:manage",
		"wash:view", "wash:create", "wash:update", "wash:assign", "wash:cancel",
		"cash:view", "cash:open", "cash:close", "cash:reopen",
		"payment:view", "payment:create", "payment:void", "payment:create_deferred",
		"expense:view", "expense:create", "expense:approve", "expense:void",
		"dashboard:view", "report:view", "report:export", "commission:view",
	},
	"manager": {
		"settings:view", "settings:manage", "station:view", "subscription:view",
		"employee:view", "employee:manage",
		"service:view", "service:manage", "customer:view", "customer:create", "customer:manage",
		"wash:view", "wash:create", "wash:update", "wash:assign", "wash:cancel",
		"cash:view", "cash:open", "cash:close", "cash:reopen",
		"payment:view", "payment:create", "payment:void",
		"expense:view", "expense:create", "expense:approve",
		"dashboard:view", "report:view", "report:export", "commission:view",
	},
	"cashier": {
		"wash:view", "wash:create", "wash:update",
		"customer:view", "customer:create",
		"cash:view", "cash:open", "cash:close",
		"payment:view", "payment:create",
		"service:view", "employee:view",
	},
	"washer": {
		"washer:view", "washer:update_status", "washer:report_incident", "washer:personal_stats",
	},
}

type sessionUtilisateur struct {
	UserID         string
	OrganizationID *string
	StationID      *string
	FullName       string
	Email          string
	Phone          string
	Role           string
	MustChange     bool
	permissions    map[string]bool
	token          string
}

func (s *sessionUtilisateur) aPermission(p string) bool { return s.permissions[p] }

func permissionsDe(role string) map[string]bool {
	m := map[string]bool{}
	for _, p := range rolePermissions[role] {
		m[p] = true
	}
	return m
}

func empreinteJeton(jeton string) string {
	sum := sha256.Sum256([]byte(jeton))
	return hex.EncodeToString(sum[:])
}

// creerSession ouvre une session applicative : jeton aléatoire, seule son
// empreinte est stockée. Hors production, le jeton est aussi renvoyé dans le
// corps (environnement d'aperçu en cadre — voir docs/api.md).
func (a *App) creerSession(ctx context.Context, w http.ResponseWriter, userID string) error {
	jeton := jeterJeton(32)
	expiration := time.Now().Add(time.Duration(a.cfg.sessionTTLHeures) * time.Hour)
	_, err := a.db.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		empreinteJeton(jeton), userID, expiration)
	if err != nil {
		return err
	}
	a.poserCookie(w, jeton, expiration)
	if a.cfg.env != "production" {
		appGlobale.Mu.Lock()
		appGlobale.JetonsApercu[jeton] = true
		appGlobale.Mu.Unlock()
	}
	return nil
}

func (a *App) poserCookie(w http.ResponseWriter, jeton string, expiration time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "washguard_session",
		Value:    jeton,
		Path:     "/",
		Expires:  expiration,
		HttpOnly: true,
		Secure:   a.cfg.cookieSecure,
		SameSite: siteCookie(a.cfg.cookieSameSite),
	})
}

func siteCookie(valeur string) http.SameSite {
	switch strings.ToLower(valeur) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// jetonsApercu garde la trace des jetons distribués hors production pour les
// accepter en Authorization: Bearer quand le cookie tiers ne survit pas.
// En production, cette carte reste vide et le bearer n'est jamais accepté.
func (a *App) jetonsApercuMap() map[string]bool { return appGlobale.JetonsApercu }

func (a *App) lireSession(r *http.Request) *sessionUtilisateur {
	jeton := ""
	if c, err := r.Cookie("washguard_session"); err == nil {
		jeton = c.Value
	}
	if jeton == "" && a.cfg.env != "production" {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if bearer != "" && bearer != r.Header.Get("Authorization") && appGlobale.jetonConnu(bearer) {
			jeton = bearer
		}
	}
	if jeton == "" {
		return nil
	}
	ctx, annuler := context.WithTimeout(r.Context(), 5*time.Second)
	defer annuler()
	var s sessionUtilisateur
	err := a.db.QueryRow(ctx, `
		SELECT u.id, u.organization_id, u.station_id, u.full_name, u.email, u.phone, u.role,
		       u.must_change_password
		FROM sessions ses JOIN users u ON u.id = ses.user_id
		WHERE ses.token_hash = $1 AND ses.expires_at > now() AND u.is_active`,
		empreinteJeton(jeton)).
		Scan(&s.UserID, &s.OrganizationID, &s.StationID, &s.FullName, &s.Email, &s.Phone, &s.Role, &s.MustChange)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return nil
	}
	s.permissions = permissionsDe(s.Role)
	s.token = jeton
	return &s
}

func (a *App) fermerSession(r *http.Request) {
	jeton := ""
	if c, err := r.Cookie("washguard_session"); err == nil {
		jeton = c.Value
	}
	if jeton == "" {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if bearer != "" && appGlobale.jetonConnu(bearer) {
			jeton = bearer
		}
	}
	if jeton == "" {
		return
	}
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	_, _ = a.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, empreinteJeton(jeton))
	appGlobale.oublierJeton(jeton)
}

// --- Anti-abus ---------------------------------------------------------------

// échecsDepasse compte les tentatives ratées par clé (identifiant ou IP) dans
// une fenêtre glissante d'un quart d'heure.
func (a *App) echecsDepasse(ctx context.Context, cle string, max int, fenetre time.Duration) bool {
	var count int
	var started time.Time
	err := a.db.QueryRow(ctx, `SELECT count, window_started_at FROM auth_failures WHERE key = $1`, cle).Scan(&count, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil || time.Since(started) > fenetre {
		return false
	}
	return count >= max
}

func (a *App) noterEchec(ctx context.Context, cle string) {
	_, _ = a.db.Exec(ctx, `
		INSERT INTO auth_failures (key, count, window_started_at) VALUES ($1, 1, now())
		ON CONFLICT (key) DO UPDATE SET
		  count = CASE WHEN auth_failures.window_started_at < now() - interval '15 minutes' THEN 1
		               ELSE auth_failures.count + 1 END,
		  window_started_at = CASE WHEN auth_failures.window_started_at < now() - interval '15 minutes' THEN now()
		                           ELSE auth_failures.window_started_at END`,
		cle)
}

func (a *App) oublierEchecs(ctx context.Context, cle string) {
	_, _ = a.db.Exec(ctx, `DELETE FROM auth_failures WHERE key = $1`, cle)
}

func ipClient(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		return strings.TrimSpace(strings.Split(v, ",")[0])
	}
	return r.RemoteAddr
}

func (e *AppExt) jetonConnu(jeton string) bool {
	e.Mu.Lock()
	defer e.Mu.Unlock()
	return e.JetonsApercu[jeton]
}

func (e *AppExt) oublierJeton(jeton string) {
	e.Mu.Lock()
	delete(e.JetonsApercu, jeton)
	e.Mu.Unlock()
}
