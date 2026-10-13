package main

import (
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Auth : login, logout, changement de mot de passe.
// ---------------------------------------------------------------------------

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	user, err := a.getUserByEmail(r.Context(), strings.TrimSpace(body.Email))
	if err != nil || user == nil || !checkPassword(user.PasswordHash, body.Password) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "E-mail ou mot de passe incorrect.", nil)
		return
	}
	if !user.IsActive {
		writeError(w, http.StatusForbidden, "forbidden", "Ce compte a été désactivé.", nil)
		return
	}

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
	writeJSON(w, http.StatusOK, payload)
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

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		_, _ = a.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE token = $1`, c.Value)
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
	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "validation_error", "Le nouveau mot de passe doit contenir au moins 8 caractères.",
			map[string]string{"new_password": "8 caractères minimum"})
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
	writeJSON(w, http.StatusOK, map[string]any{"message": "Mot de passe mis à jour avec succès."})
}
