package main

import (
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Auth : inscription (organisation + station + propriétaire).
// ---------------------------------------------------------------------------

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrganizationName string `json:"organization_name"`
		FullName         string `json:"full_name"`
		Email            string `json:"email"`
		Phone            string `json:"phone"`
		Password         string `json:"password"`
		StationName      string `json:"station_name"`
		City             string `json:"city"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if body.OrganizationName == "" || body.FullName == "" || body.Email == "" || body.Password == "" || body.StationName == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Tous les champs obligatoires doivent être renseignés.", nil)
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Adresse e-mail invalide.",
			map[string]string{"email": "Format d'e-mail invalide"})
		return
	}

	existing, err := a.getUserByEmail(r.Context(), body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if existing != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Un compte existe déjà avec cette adresse e-mail.",
			map[string]string{"email": "E-mail déjà utilisé"})
		return
	}

	hash, err := hashPassword(body.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	ctx := r.Context()
	city := body.City
	if city == "" {
		city = "Abidjan"
	}
	orgID := newID("org")
	stationID := newID("st")
	userID := newID("usr")

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, email, phone, city, country, currency, timezone)
		VALUES ($1,$2,$3,$4,$5,$6,'Côte d''Ivoire','XOF','Africa/Abidjan')`,
		orgID, body.OrganizationName, slugify(body.OrganizationName), strings.ToLower(body.Email), body.Phone, city)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de l'organisation.", nil)
		return
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stations (id, organization_id, name, code, phone, city, settings, is_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,true)`,
		stationID, orgID, body.StationName, defaultStationCode(body.StationName), body.Phone, city,
		`{"commission_mode":"per_wash","commission_per_wash_xof":500,"monthly_goal_xof":1000000}`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de la station.", nil)
		return
	}

	// Formule d'essai : la crée si le seed ne l'a pas faite.
	_, _ = tx.Exec(ctx, `
		INSERT INTO plans (id, code, name, description, price_xof, trial_days, sort_order)
		VALUES ('plan-essai','essai','Essai','Découverte complète pendant 14 jours, une station, deux utilisateurs.',0,14,1)
		ON CONFLICT (id) DO NOTHING`)
	trialEnd := time.Now().Add(14 * 24 * time.Hour)
	_, err = tx.Exec(ctx, `
		INSERT INTO subscriptions (id, organization_id, plan_id, status, trial_ends_at, current_period_start, current_period_end)
		VALUES ($1,$2,'plan-essai','trialing',$3,now(),$3)`,
		newID("sub"), orgID, trialEnd)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de l'abonnement.", nil)
		return
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, organization_id, station_id, full_name, email, phone, password_hash, role)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'owner')`,
		userID, orgID, stationID, body.FullName, strings.ToLower(body.Email), body.Phone, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création du compte.", nil)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	token := newToken()
	_ = a.createSession(ctx, token, userID)
	a.setSessionCookie(w, r, token)

	user, _ := a.getUserByID(ctx, userID)
	payload, err := a.buildMePayload(ctx, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusCreated, payload)
}

// defaultStationCode : les 3 premières lettres majuscules du nom.
func defaultStationCode(name string) string {
	letters := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' {
			return r - 32
		}
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, name)
	if len(letters) > 3 {
		letters = letters[:3]
	}
	if letters == "" {
		letters = "STA"
	}
	return letters
}
