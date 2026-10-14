package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "washguard_session"

// hashPassword / checkPassword — bcrypt remplace les mots de passe en clair
// du server.ts (faille de sécurité corrigée).
func hashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// newToken génère un jeton de session aléatoire (256 bits).
func newToken() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// hashToken : la base ne conserve que l'empreinte SHA-256 du jeton. Une copie
// de la table sessions ne permet donc pas de rejouer une session.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// newIDfabrique un identifiant texte préfixé, comme le faisait `x-${Date.now()}`.
func newID(prefix string) string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return prefix + "-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(buf)
}

var clerkWebhookSecretKey contextKey = "clerk_webhook_secret"

// bearerToken extrait un jeton "Authorization: Bearer …".
func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// userFromRequest charge l'utilisateur derrière le jeton de session.
// Deux sources sont acceptées : le cookie de session applicative (local) ou
// un jeton de session Clerk (JWT vérifié contre la JWKS).
func (a *App) userFromRequest(r *http.Request) (*User, error) {
	token := ""
	if c, err := r.Cookie(sessionCookieName); err == nil {
		token = c.Value
	}
	if token == "" {
		token = bearerToken(r)
	}
	if token == "" {
		return nil, errUnauthorized
	}

	ctx := r.Context()
	var userID string
	err := a.Pool.QueryRow(ctx,
		`SELECT user_id FROM sessions WHERE token = $1 AND expires_at > now()`, hashToken(token),
	).Scan(&userID)
	if err != nil {
		// Pas une session locale : si un JWT Clerk est fourni, on l'accepte.
		if bearer := bearerToken(r); bearer != "" && a.Cfg.ClerkEnabled && clerkKeyfunc != nil {
			if clerkID, _, verr := verifyClerkSessionToken(bearer); verr == nil {
				if u, gerr := a.getUserByClerkID(ctx, clerkID); gerr == nil && u != nil && u.IsActive {
					return u, nil
				}
			}
			return nil, errUnauthorized
		}
		return nil, errUnauthorized
	}
	user, err := a.getUserByID(ctx, userID)
	if err != nil || user == nil || !user.IsActive {
		return nil, errUnauthorized
	}
	if err := a.organizationAllowed(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// organizationAllowed refuse l'accès quand l'entreprise est suspendue ou
// archivée : la vitrine l'annonce, le serveur doit l'appliquer.
func (a *App) organizationAllowed(ctx context.Context, user *User) error {
	if user.OrganizationID == nil {
		// Compte plateforme (superadmin) : pas d'entreprise.
		return nil
	}
	var status string
	if err := a.Pool.QueryRow(ctx,
		`SELECT status FROM organizations WHERE id = $1`, *user.OrganizationID).Scan(&status); err != nil {
		return nil
	}
	if status != "active" {
		return &apiError{
			Status:  http.StatusForbidden,
			Code:    "organization_suspended",
			Message: "L'accès de votre entreprise est suspendu. Contactez le support WashGuard.",
		}
	}
	return nil
}

type contextKey string

const userContextKey contextKey = "user"

var errUnauthorized = &apiError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Vous devez vous connecter."}

// apiError porte un code HTTP + code métier pour les réponses d'erreur.
type apiError struct {
	Status  int
	Code    string
	Message string
	Fields  map[string]string
}

func (e *apiError) Error() string { return e.Message }

// requireAuth protège une route : sans session valide, 401.
func (a *App) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := a.userFromRequest(r)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

// currentUser récupère l'utilisateur posé par requireAuth.
func currentUser(r *http.Request) *User {
	u, _ := r.Context().Value(userContextKey).(*User)
	return u
}

// setSessionCookie pose le cookie de session (HttpOnly, Secure en HTTPS).
func (a *App) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := a.Cfg.SessionCookieSecure || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	// SameSite=Lax par défaut : le navigateur n'envoie plus le cookie sur une
	// requête POST venue d'un autre site (protection CSRF de base). SameSite
	// = None n'est utilisé que si l'exploitant le demande explicitement, pour
	// un front servi sur un autre domaine.
	sameSite := http.SameSiteLaxMode
	if a.Cfg.SessionCookieSameSite == "none" && secure {
		sameSite = http.SameSiteNoneMode
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   a.Cfg.SessionTTLHours * 3600,
	})
}

// createSession insère le jeton en base (persistance réelle vs Map en mémoire).
func (a *App) createSession(ctx context.Context, token, userID string) error {
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO sessions (token, user_id, expires_at) VALUES ($1, $2, now() + make_interval(hours => $3))`,
		hashToken(token), userID, a.Cfg.SessionTTLHours)
	return err
}

// purgeExpiredSessions supprime les sessions expirées (appelé au démarrage
// puis périodiquement : la table ne grossit plus sans fin).
func (a *App) purgeExpiredSessions(ctx context.Context) {
	_, _ = a.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
}

// getUserByID charge un utilisateur par identifiant.
func (a *App) getUserByID(ctx context.Context, id string) (*User, error) {
	row := a.Pool.QueryRow(ctx, `
		SELECT id, organization_id, station_id, full_name, email, phone, password_hash, role,
		       is_active, must_change_password, last_login_at, created_at,
		       commission_mode, commission_per_wash_xof, commission_percent
		FROM users WHERE id = $1`, id)
	return scanUser(row)
}

// getUserByEmail charge un utilisateur par e-mail (insensible à la casse).
func (a *App) getUserByEmail(ctx context.Context, email string) (*User, error) {
	row := a.Pool.QueryRow(ctx, `
		SELECT id, organization_id, station_id, full_name, email, phone, password_hash, role,
		       is_active, must_change_password, last_login_at, created_at,
		       commission_mode, commission_per_wash_xof, commission_percent
		FROM users WHERE lower(email) = lower($1)`, email)
	return scanUser(row)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.OrganizationID, &u.StationID, &u.FullName, &u.Email, &u.Phone,
		&u.PasswordHash, &u.Role, &u.IsActive, &u.MustChangePassword, &u.LastLoginAt, &u.CreatedAt,
		&u.CommissionMode, &u.CommissionPerWashXOF, &u.CommissionPercent)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
