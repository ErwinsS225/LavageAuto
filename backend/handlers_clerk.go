package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	svix "github.com/svix/svix-webhooks/go"
)

// ---------------------------------------------------------------------------
// Endpoints Clerk : authentification par session JWT + webhook de synchro.
// ---------------------------------------------------------------------------

// handleClerkLogin accepte un jeton de session Clerk (header Authorization:
// Bearer <jwt>), le vérifie contre la JWKS, synchronise l'utilisateur dans
// Neon et ouvre une session applicative (cookie washguard_session).
func (a *App) handleClerkLogin(w http.ResponseWriter, r *http.Request) {
	if !a.Cfg.ClerkEnabled {
		writeError(w, http.StatusNotImplemented, "clerk_disabled", "Clerk n'est pas configuré sur ce serveur.", nil)
		return
	}
	token := bearerToken(r)
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Jeton Clerk manquant.", nil)
		return
	}
	clerkID, email, err := verifyClerkSessionToken(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Jeton Clerk invalide ou expiré.", nil)
		return
	}

	// L'utilisateur doit exister dans Neon (créé via webhook ou synchro API).
	ctx := r.Context()
	usr, err := a.getUserByClerkID(ctx, clerkID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if usr == nil {
		// Synchro à la demande depuis l'API Clerk puis nouvelle tentative.
		if err := a.upsertClerkUserFromAPI(ctx, clerkID); err != nil {
			writeError(w, http.StatusForbidden, "forbidden", "Aucun compte employé associé à ce compte Clerk.", nil)
			return
		}
		if usr, err = a.getUserByClerkID(ctx, clerkID); err != nil || usr == nil {
			writeError(w, http.StatusForbidden, "forbidden", "Aucun compte employé associé à ce compte Clerk.", nil)
			return
		}
	}
	if !usr.IsActive {
		writeError(w, http.StatusForbidden, "forbidden", "Ce compte a été désactivé.", nil)
		return
	}
	_ = email // l'e-mail de la session Clerk est déjà couvert par la synchro

	tokenApp := newToken()
	if err := a.createSession(ctx, tokenApp, usr.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Impossible de créer la session.", nil)
		return
	}
	a.setSessionCookie(w, r, tokenApp)
	_, _ = a.Pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, usr.ID)

	payload, err := a.buildMePayload(ctx, usr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// handleClerkWebhook reçoit les événements Clerk signés Svix et synchronise
// la table users de Neon : user.created / user.updated / user.deleted.
func (a *App) handleClerkWebhook(w http.ResponseWriter, r *http.Request) {
	secret := r.Context().Value(clerkWebhookSecretKey).(string)
	if secret == "" {
		writeError(w, http.StatusNotImplemented, "clerk_disabled", "Webhook Clerk non configuré.", nil)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	wh, err := svix.NewWebhook(secret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Webhook mal configuré.", nil)
		return
	}
	headers := http.Header{}
	for _, h := range []string{"svix-id", "svix-timestamp", "svix-signature"} {
		headers.Set(h, r.Header.Get(h))
	}
	var payload map[string]any
	if err := wh.Verify(body, headers); err != nil {
		// Aucun détail d'en-tête n'est journalisé : la signature est un secret.
		log.Printf("webhook clerk : signature refusée (id=%q)", r.Header.Get("svix-id"))
		writeError(w, http.StatusUnauthorized, "unauthorized", "Signature webhook invalide.", nil)
		return
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "JSON invalide.", nil)
		return
	}

	eventType, _ := payload["type"].(string)
	data, _ := payload["data"].(map[string]any)
	clerkID, _ := data["id"].(string)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch eventType {
	case "user.created", "user.updated":
		if clerkID != "" {
			fullName := clerkFullName(data)
			email, phone := clerkPrimaryContacts(data)
			active := true
			if b, ok := data["banned"].(bool); ok {
				active = !b
			}
			if err := a.syncClerkUserToNeon(ctx, clerkID, fullName, email, phone, active); err != nil {
				log.Printf("webhook clerk : synchro %s impossible : %v", clerkID, err)
				writeError(w, http.StatusInternalServerError, "internal", "Synchro impossible.", nil)
				return
			}
		}
	case "user.deleted":
		if clerkID != "" {
			_, _ = a.Pool.Exec(ctx, `UPDATE users SET is_active = false WHERE clerk_user_id = $1`, clerkID)
		}
	default:
		// Événements ignorés silencieusement (session.*, email.*, …).
	}
	writeJSON(w, http.StatusOK, map[string]any{"received": true})
}

func clerkFullName(data map[string]any) string {
	first, _ := data["first_name"].(string)
	last, _ := data["last_name"].(string)
	name := first + " " + last
	if name == " " {
		return "Utilisateur Clerk"
	}
	return name
}

func clerkPrimaryContacts(data map[string]any) (email, phone string) {
	primaryEmailID, _ := data["primary_email_address_id"].(string)
	primaryPhoneID, _ := data["primary_phone_number_id"].(string)
	if list, ok := data["email_addresses"].([]any); ok {
		for _, raw := range list {
			m, _ := raw.(map[string]any)
			id, _ := m["id"].(string)
			addr, _ := m["email_address"].(string)
			if id == primaryEmailID && addr != "" {
				email = addr
			}
		}
	}
	if list, ok := data["phone_numbers"].([]any); ok {
		for _, raw := range list {
			m, _ := raw.(map[string]any)
			id, _ := m["id"].(string)
			num, _ := m["phone_number"].(string)
			if id == primaryPhoneID && num != "" {
				phone = num
			}
		}
	}
	return email, phone
}
