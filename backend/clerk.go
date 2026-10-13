package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	clerk "github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Clerk : vérification des sessions JWT et synchronisation vers Neon.
// ---------------------------------------------------------------------------

var clerkKeyfunc keyfunc.Keyfunc

// initClerk configure le SDK Clerk (clé secrète) et charge la JWKS pour la
// vérification locale des JWT de session. Sûr à appeler même sans Clerk.
func (a *App) initClerk() error {
	if !a.Cfg.ClerkEnabled {
		return nil
	}
	clerk.SetKey(a.Cfg.ClerkSecretKey)
	if a.Cfg.ClerkJWKSURL == "" {
		return fmt.Errorf("clerk : JWKS_URL_CLERK manquante (vérification JWT impossible)")
	}
	kf, err := keyfunc.NewDefault([]string{a.Cfg.ClerkJWKSURL})
	if err != nil {
		return fmt.Errorf("clerk : chargement JWKS : %w", err)
	}
	clerkKeyfunc = kf
	log.Printf("Clerk activé (JWKS: %s)", a.Cfg.ClerkJWKSURL)
	return nil
}

// verifyClerkSessionToken valide un JWT de session Clerk et renvoie son subject
// (= clerk_user_id) et l'e-mail principal quand disponible.
func verifyClerkSessionToken(tokenStr string) (clerkUserID, email string, err error) {
	if clerkKeyfunc == nil {
		return "", "", fmt.Errorf("clerk non initialisé")
	}
	opts := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256"})}
	claims := jwt.MapClaims{}
	parser := jwt.NewParser(opts...)
	if _, _, err = parser.ParseUnverified(tokenStr, claims); err == nil {
		if azp, ok := claims["azp"].(string); ok && azp != "" && clerkAuthorizedParties != "" {
			if !containsPartie(strings.Split(clerkAuthorizedParties, ","), azp) {
				return "", "", fmt.Errorf("azp non autorisé")
			}
		}
	}
	tok, err := jwt.Parse(tokenStr, clerkKeyfunc.Keyfunc, opts...)
	if err != nil || !tok.Valid {
		return "", "", fmt.Errorf("jeton Clerk invalide : %w", err)
	}
	mc, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", fmt.Errorf("claims Clerk illisibles")
	}
	sub, _ := mc["sub"].(string)
	if sub == "" {
		return "", "", fmt.Errorf("claim sub absent")
	}
	// Email : présent dans le payload de session (o.mail / primary email).
	if m, ok := mc["mail"].(map[string]any); ok {
		if addr, ok := m["address"].(string); ok {
			email = addr
		}
	}
	return sub, email, nil
}

var clerkAuthorizedParties string

func containsPartie(list []string, v string) bool {
	for _, s := range list {
		if strings.TrimSpace(s) == v {
			return true
		}
	}
	return false
}

// syncClerkUserToNeon upserte un utilisateur Clerk dans la table users (Neon).
// Rôle par défaut : cashier, organisation/station NULL tant que le gérant
// n'a pas affecté l'employé.
func (a *App) syncClerkUserToNeon(ctx context.Context, clerkID, fullName, email, phone string, active bool) error {
	if clerkID == "" {
		return fmt.Errorf("clerk_user_id vide")
	}
	_, err := a.Pool.Exec(ctx, `
INSERT INTO users (id, organization_id, station_id, full_name, email, phone,
                   password_hash, role, is_active, must_change_password, clerk_user_id)
VALUES ($1, NULL, NULL, $2, $3, $4, 'clerk-managed', 'cashier', $5, false, $1)
ON CONFLICT (clerk_user_id) DO UPDATE SET
  full_name  = EXCLUDED.full_name,
  email      = EXCLUDED.email,
  phone      = EXCLUDED.phone,
  is_active  = EXCLUDED.is_active`,
		clerkID, fullName, email, phone, active)
	if err != nil {
		return fmt.Errorf("sync clerk→neon : %w", err)
	}
	return nil
}

// upsertClerkUserFromAPI récupère le profil complet depuis l'API Clerk
// (webhook court / synchronisation à la demande) puis le pousse dans Neon.
func (a *App) upsertClerkUserFromAPI(ctx context.Context, clerkID string) error {
	if !a.Cfg.ClerkEnabled {
		return fmt.Errorf("clerk non activé")
	}
	usr, err := user.Get(ctx, clerkID)
	if err != nil {
		return fmt.Errorf("API clerk : %w", err)
	}
	fullName := strings.TrimSpace(deref(usr.FirstName) + " " + deref(usr.LastName))
	if fullName == "" {
		fullName = "Utilisateur Clerk"
	}
	email, phone := "", ""
	for _, e := range usr.EmailAddresses {
		if usr.PrimaryEmailAddressID != nil && e.ID == *usr.PrimaryEmailAddressID {
			email = e.EmailAddress
			break
		}
	}
	for _, p := range usr.PhoneNumbers {
		if usr.PrimaryPhoneNumberID != nil && p.ID == *usr.PrimaryPhoneNumberID {
			phone = p.PhoneNumber
			break
		}
	}
	return a.syncClerkUserToNeon(ctx, clerkID, fullName, email, phone, !usr.Banned)
}

// getUserByClerkID charge l'utilisateur Neon correspondant à un ID Clerk.
func (a *App) getUserByClerkID(ctx context.Context, clerkID string) (*User, error) {
	row := a.Pool.QueryRow(ctx, `
		SELECT id, organization_id, station_id, full_name, email, phone, password_hash, role,
		       is_active, must_change_password, last_login_at, created_at,
		       commission_mode, commission_per_wash_xof, commission_percent
		FROM users WHERE clerk_user_id = $1`, clerkID)
	u, err := scanUser(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return u, err
}
