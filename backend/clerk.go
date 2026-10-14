package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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

var clerkKeyfunc jwt.Keyfunc

// clerkStaticKey : clé publique RSA fournie en configuration (JWKS_PUBLIC_KEY).
// Elle sert de repli quand aucune adresse JWKS n'est fournie ou joignable :
// la vérification d'une session ne dépend alors pas d'un accès réseau.
var clerkStaticKey *rsa.PublicKey

// parsePublicKeyPEM lit une clé publique au format PEM (celle que Clerk
// affiche dans son tableau de bord).
func parsePublicKeyPEM(contenu string) (*rsa.PublicKey, error) {
	bloc, _ := pem.Decode([]byte(strings.TrimSpace(contenu)))
	if bloc == nil {
		return nil, fmt.Errorf("clé publique illisible (format PEM attendu)")
	}
	pub, err := x509.ParsePKIXPublicKey(bloc.Bytes)
	if err != nil {
		return nil, fmt.Errorf("clé publique illisible : %w", err)
	}
	cle, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("clé publique non RSA")
	}
	return cle, nil
}

// authorizedPartiesClerk : origines acceptées pour le claim `azp` des sessions
// Clerk (renseignées par initClerk).
var authorizedPartiesClerk []string

// initClerk configure le SDK Clerk (clé secrète) et charge la JWKS pour la
// vérification locale des JWT de session. Sûr à appeler même sans Clerk.
func (a *App) initClerk() error {
	if !a.Cfg.ClerkEnabled {
		return nil
	}
	authorizedPartiesClerk = a.Cfg.ClerkAuthorizedParties
	clerk.SetKey(a.Cfg.ClerkSecretKey)

	// Clé statique éventuelle : elle sert de repli, jamais de priorité.
	if a.Cfg.ClerkPublicKey != "" {
		cle, err := parsePublicKeyPEM(a.Cfg.ClerkPublicKey)
		if err != nil {
			log.Printf("Clerk : JWKS_PUBLIC_KEY ignorée (%v)", err)
		} else {
			clerkStaticKey = cle
		}
	}

	if a.Cfg.ClerkJWKSURL != "" {
		kf, err := keyfunc.NewDefault([]string{a.Cfg.ClerkJWKSURL})
		if err != nil {
			// L'adresse peut être momentanément injoignable : si une clé
			// statique est fournie, on continue à vérifier les sessions.
			if clerkStaticKey == nil {
				return fmt.Errorf("clerk : chargement JWKS : %w", err)
			}
			log.Printf("Clerk : JWKS injoignable (%v) — repli sur la clé publique statique", err)
			return nil
		}
		clerkKeyfunc = kf.Keyfunc
		log.Printf("Clerk activé (JWKS: %s)", a.Cfg.ClerkJWKSURL)
		return nil
	}
	if clerkStaticKey != nil {
		log.Printf("Clerk activé (clé publique statique, sans rotation automatique)")
		return nil
	}
	return fmt.Errorf("clerk : JWKS_URL_CLERK ou JWKS_PUBLIC_KEY requise (vérification des jetons impossible)")
}

// verifyClerkSessionToken valide un JWT de session Clerk et renvoie son subject
// (= clerk_user_id) et l'e-mail principal quand disponible.
func verifyClerkSessionToken(tokenStr string) (clerkUserID, email string, err error) {
	if clerkKeyfunc == nil && clerkStaticKey == nil {
		return "", "", fmt.Errorf("clerk non initialisé")
	}
	opts := []jwt.ParserOption{jwt.WithValidMethods([]string{"RS256"})}
	// Ordre de résolution : l'adresse JWKS (qui suit la rotation des clés de
	// Clerk) puis, si elle ne répond pas, la clé publique statique. Une station
	// dont le réseau vacille peut ainsi continuer à valider les sessions.
	cle := jwt.Keyfunc(func(t *jwt.Token) (any, error) {
		if clerkKeyfunc != nil {
			if k, err := clerkKeyfunc(t); err == nil {
				return k, nil
			}
		}
		if clerkStaticKey != nil {
			return clerkStaticKey, nil
		}
		return nil, fmt.Errorf("aucune clé de vérification disponible")
	})
	tok, err := jwt.Parse(tokenStr, cle, opts...)
	if err != nil || !tok.Valid {
		return "", "", fmt.Errorf("jeton Clerk invalide : %w", err)
	}
	mc, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", fmt.Errorf("claims Clerk illisibles")
	}
	// `azp` = application à l'origine de la session. Le contrôle est fait
	// **après** vérification de signature (un jeton non signé ne doit pas
	// pouvoir décider de ses propres claims).
	if azp, ok := mc["azp"].(string); ok && azp != "" {
		if len(authorizedPartiesClerk) == 0 {
			return "", "", fmt.Errorf("azp présent mais aucune origine autorisée configurée")
		}
		if !containsPartie(authorizedPartiesClerk, azp) {
			return "", "", fmt.Errorf("azp non autorisé")
		}
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
