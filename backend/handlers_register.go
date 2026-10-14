package main

import (
	"context"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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
		// InvitationToken : quand il est présent, l'inscription **rejoint** une
		// entreprise existante au lieu d'en créer une (voir inscrireParInvitation).
		InvitationToken string `json:"invitation_token"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if body.InvitationToken != "" {
		a.inscrireParInvitation(w, r, body.InvitationToken, body.FullName, body.Email, body.Phone, body.Password)
		return
	}
	if body.OrganizationName == "" || body.FullName == "" || body.Email == "" || body.Password == "" || body.StationName == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Tous les champs obligatoires doivent être renseignés.", nil)
		return
	}
	if len(body.Password) < minPasswordLength {
		writeAPIError(w, passwordTooShort("password"))
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

	// onboarding_state = 'pending' : le compte neuf doit décrire sa station avant
	// de pouvoir exploiter (les entreprises déjà en service, elles, ont été
	// marquées « active » par la migration 0004).
	_, err = tx.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, email, phone, city, country, currency, timezone, onboarding_state)
		VALUES ($1,$2,$3,$4,$5,$6,'Côte d''Ivoire','XOF','Africa/Abidjan','pending')`,
		orgID, body.OrganizationName, uniqueOrgSlug(ctx, tx, body.OrganizationName),
		strings.ToLower(body.Email), body.Phone, city)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de l'organisation.", nil)
		return
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO stations (id, organization_id, name, code, phone, city, settings, is_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,true)`,
		stationID, orgID, body.StationName, defaultStationCode(body.StationName), body.Phone, city,
		defaultStationSettings(1_000_000))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de la station.", nil)
		return
	}

	// Aucune prestation n'est créée ici : les tarifs d'un lavage appartiennent à
	// son exploitant. Inventer « Lavage simple 2 000 F » ferait figurer sur la
	// première facture un prix que personne n'a décidé. Le parcours de mise en
	// route demande donc les prestations et leurs prix (étapes « prestations »
	// et « tarifs »), et une prestation sans tarif ne peut pas être encaissée.
	// Formule d'essai : la crée si le seed ne l'a pas faite.
	_, _ = tx.Exec(ctx, `
		INSERT INTO plans (id, code, name, description, price_xof, trial_days, sort_order, max_stations, max_employees)
		VALUES ('plan-essai','essai','Essai','Découverte complète pendant 14 jours, une station, deux utilisateurs.',0,14,1,1,2)
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

	// Le compte neuf reçoit le catalogue de prestations **sans prix** : les
	// lavages courants y sont décrits (ce qu'ils comprennent, combien de temps
	// ils prennent), il reste à saisir les tarifs. Une base neuve ne doit
	// jamais afficher un prix que personne n'a décidé.
	if err := a.equiperCatalogueNeuf(ctx, orgID, stationID); err != nil {
		log.Printf("[inscription] catalogue de prestations non équipé pour %s : %v", orgID, err)
	}
	// Les moyens de paiement de la station : le référentiel est posé d'emblée,
	// les espèces étant actives. La caisse peut donc encaisser dès la première
	// fiche, sans qu'un prix ni un moyen n'aient été inventés.
	if err := a.equiperMoyensPaiement(ctx, orgID, stationID); err != nil {
		log.Printf("[inscription] moyens de paiement non équipés pour %s : %v", orgID, err)
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

// uniqueOrgSlug : identifiant lisible, unique. Deux entreprises peuvent porter
// le même nom (deux villes, deux enseignes voisines) — l'inscription ne doit
// pas échouer pour cette raison.
func uniqueOrgSlug(ctx context.Context, tx pgx.Tx, name string) string {
	base := slugify(name)
	if base == "" {
		base = "entreprise"
	}
	slug := base
	for i := 2; i <= 25; i++ {
		var taken bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM organizations WHERE slug = $1)`, slug).Scan(&taken); err != nil || !taken {
			return slug
		}
		slug = base + "-" + itoa(i)
	}
	return base + "-" + newID("")[len(newID(""))-4:]
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
