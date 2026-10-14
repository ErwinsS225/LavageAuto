package main

import (
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Auth : login, logout, changement de mot de passe.
// ---------------------------------------------------------------------------

// minPasswordLength : longueur minimale appliquée partout (inscription,
// création d'employé, changement de mot de passe). Un seul endroit à changer.
const minPasswordLength = 8

// passwordTooShort renvoie une erreur de validation homogène.
func passwordTooShort(field string) *apiError {
	return &apiError{
		Status:  http.StatusBadRequest,
		Code:    "validation_error",
		Message: "Le mot de passe doit contenir au moins 8 caractères.",
		Fields:  map[string]string{field: "8 caractères minimum"},
	}
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	email := strings.TrimSpace(body.Email)

	// Deux compteurs : par adresse IP et par compte. Ils ne comptent que les
	// échecs — une connexion normale n'est jamais ralentie.
	ipKey := "ip:" + clientIP(r)
	mailKey := "id:" + lower(email)
	for _, key := range []string{ipKey, mailKey} {
		if blocked, retry := loginLimiter.exceeded(key); blocked {
			writeTooManyAttempts(w, retry)
			return
		}
	}
	fail := func() {
		loginLimiter.charge(ipKey)
		loginLimiter.charge(mailKey)
		writeError(w, http.StatusUnauthorized, "unauthorized", "E-mail ou mot de passe incorrect.", nil)
	}

	user, err := a.getUserByEmail(r.Context(), email)
	if err != nil || user == nil || !checkPassword(user.PasswordHash, body.Password) {
		// Message identique dans tous les cas : on n'indique jamais si
		// l'adresse existe.
		fail()
		return
	}
	if !user.IsActive {
		writeError(w, http.StatusForbidden, "forbidden", "Ce compte a été désactivé.", nil)
		return
	}
	if err := a.organizationAllowed(r.Context(), user); err != nil {
		writeAPIError(w, err)
		return
	}
	loginLimiter.forget(ipKey)
	loginLimiter.forget(mailKey)

	token := newToken()
	if err := a.createSession(r.Context(), token, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Impossible de créer la session.", nil)
		return
	}
	a.setSessionCookie(w, r, token)
	_, _ = a.Pool.Exec(r.Context(), `UPDATE users SET last_login_at = now() WHERE id = $1`, user.ID)

	payload, err := a.buildMePayload(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	// Hors production, le jeton accompagne le cookie (voir apercu_session.go) :
	// un navigateur qui refuse le cookie tiers — aperçu dans un cadre — peut
	// alors présenter le jeton en en-tête.
	writeJSON(w, http.StatusOK, a.avecJetonApercu(payload, token))
}

// handleMe renvoie l'utilisateur courant, son organisation, sa station,
// son abonnement et ses permissions.
func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	payload, err := a.buildMePayload(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// handleLogout ferme la session, quelle que soit la façon dont elle a été
// présentée : le cookie de session applicative, et — hors production seulement,
// voir apercu_session.go — le même jeton passé en `Authorization: Bearer`. Sans
// cette seconde voie, une déconnexion depuis l'aperçu laisserait la session
// ouverte côté serveur : le jeton resterait valable.
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	jetons := []string{}
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		jetons = append(jetons, c.Value)
	}
	if bearer := bearerToken(r); bearer != "" && a.apercuAutorise() {
		jetons = append(jetons, bearer)
	}
	for _, token := range jetons {
		_, _ = a.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE token = $1`, hashToken(token))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if !checkPassword(user.PasswordHash, body.CurrentPassword) {
		writeError(w, http.StatusBadRequest, "validation_error", "Mot de passe actuel incorrect.",
			map[string]string{"current_password": "Mot de passe incorrect"})
		return
	}
	if len(body.NewPassword) < minPasswordLength {
		writeAPIError(w, passwordTooShort("new_password"))
		return
	}
	hash, err := hashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	_, err = a.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, must_change_password = false WHERE id = $2`, hash, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	// Les autres sessions du compte sont fermées : un mot de passe changé
	// déconnecte les appareils qui utilisaient encore l'ancien.
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		_, _ = a.Pool.Exec(r.Context(),
			`DELETE FROM sessions WHERE user_id = $1 AND token <> $2`, user.ID, hashToken(c.Value))
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"user.password_changed", "user", user.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Mot de passe mis à jour avec succès."})
}
