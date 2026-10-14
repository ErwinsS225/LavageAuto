package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// routes_auth.go — inscription, connexion, déconnexion, profil courant.
// Le hachage des mots de passe est délégué à pgcrypto (crypt/gen_salt) : le
// serveur Go reste sans dépendance de cryptographie applicative, et un mot de
// passe n'est jamais stocké en clair ni renvoyé.

func hacherMotDePasse(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}, motDePasse string) (string, error) {
	var hash string
	err := db.QueryRow(ctx, `SELECT crypt($1, gen_salt('bf', 10))`, motDePasse).Scan(&hash)
	return hash, err
}

var reEMAIL = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[A-Za-z]{2,}$`)

type enteteOrg struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
	City  string `json:"city"`
}

type corpsInscription struct {
	FullName        string      `json:"full_name"`
	Email           string      `json:"email"`
	Phone           string      `json:"phone"`
	Password        string      `json:"password"`
	Organization    interface{} `json:"organization"` // objet OU chaîne (front)
	InvitationToken string      `json:"invitation_token"`
}

type organisationPayload struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
	City  string `json:"city"`
}

func (c corpsInscription) organisation() organisationPayload {
	switch v := c.Organization.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return organisationPayload{Name: "Mon entreprise"}
		}
		return organisationPayload{Name: strings.TrimSpace(v)}
	case map[string]interface{}:
		p := organisationPayload{
			Name:  chaineOuVide(v["name"]),
			Phone: chaineOuVide(v["phone"]),
			Email: chaineOuVide(v["email"]),
			City:  chaineOuVide(v["city"]),
		}
		return p
	}
	return organisationPayload{Name: "Mon entreprise"}
}

func (a *App) enregistrerRoutesAuth(rt *routeur) {
	// register/login sont publics ; logout et /me exigent la session.
	rt.routes = append(rt.routes,
		route{methode: "POST", prefixe: "/api/v1/auth/register", handler: rt.avecSession(a.inscrire), public: true},
		route{methode: "POST", prefixe: "/api/v1/auth/login", handler: a.connecter, public: true})
	rt.ajouter("POST", "/api/v1/auth/logout", a.deconnecter)
	rt.ajouter("GET", "/api/v1/auth/me", a.profilCourant)
}

func (a *App) inscrire(w http.ResponseWriter, r *http.Request, _ *sessionUtilisateur) {
	corps, ok := decoderCorps[corpsInscription](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ip := ipClient(r)
	cleIP := "register:" + ip
	if a.echecsDepasse(ctx, cleIP, a.cfg.registerMaxParHeure, time.Hour) {
		repondreErreur(w, http.StatusTooManyRequests, "too_many_requests",
			"Trop de créations de compte depuis cette adresse. Réessayez plus tard.")
		return
	}

	champs := map[string]string{}
	nom := strings.TrimSpace(corps.FullName)
	if len(nom) < 2 {
		champs["full_name"] = "Nom complet requis."
	}
	email := strings.ToLower(strings.TrimSpace(corps.Email))
	if email != "" && !reEMAIL.MatchString(email) {
		champs["email"] = "Adresse e-mail invalide."
	}
	if corps.Password != "" && len(corps.Password) < 8 {
		champs["password"] = "Le mot de passe doit compter au moins 8 caractères."
	}
	org := corps.organisation()
	if strings.TrimSpace(org.Name) == "" {
		champs["organization.name"] = "Le nom de l'entreprise est requis."
	}
	validerTelephone("organization.phone", org.Phone, champs)
	if org.Email != "" && !reEMAIL.MatchString(org.Email) {
		champs["organization.email"] = "Adresse e-mail invalide."
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}

	// Voie invitation : rattacher à l'entreprise existante, pas d'organisation neuve.
	if token := strings.TrimSpace(corps.InvitationToken); token != "" {
		a.inscrireParInvitation(ctx, w, r, corps, nom, email, token)
		return
	}
	if corps.Password == "" {
		ecrireValidation(w, map[string]string{"password": "Un mot de passe est requis pour créer un espace."})
		return
	}
	if email == "" {
		ecrireValidation(w, map[string]string{"email": "Une adresse e-mail est requise."})
		return
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)

	var existe int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = $1`, email).Scan(&existe)
	if existe > 0 {
		a.noterEchec(ctx, cleIP)
		ecrireConflit(w, http.StatusConflict, conflit{code: "email_taken", message: "Cette adresse e-mail possède déjà un compte.", fields: map[string]string{"email": "Un compte existe déjà avec cette adresse."}})
		return
	}

	var orgID string
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name, phone, email, city) VALUES ($1, $2, $3, COALESCE(NULLIF($4,''), 'Abidjan'))
		RETURNING id`, org.Name, org.Phone, org.Email, org.City).Scan(&orgID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	hash, err := hacherMotDePasse(ctx, tx, corps.Password)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	var userID string
	err = tx.QueryRow(ctx, `
		INSERT INTO users (organization_id, full_name, email, phone, role, password_hash)
		VALUES ($1, $2, $3, $4, 'owner', $5) RETURNING id`,
		orgID, nom, email, org.Phone, hash).Scan(&userID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	// Abonnement « Découverte » par défaut : le quota de stations s'appuie dessus.
	_, _ = tx.Exec(ctx, `
		INSERT INTO subscriptions (organization_id, plan_id, status, trial_ends_at, current_period_start, current_period_end)
		SELECT $1, id, 'trialing', now() + interval '14 days', now(), now() + interval '14 days'
		FROM plans WHERE code = 'decouverte' ON CONFLICT DO NOTHING`, orgID)
	_, _ = tx.Exec(ctx, `
		INSERT INTO onboarding_states (organization_id, state, started_at) VALUES ($1, 'pending', now())
		ON CONFLICT (organization_id) DO NOTHING`, orgID)
	for _, m := range mentionsRegistre {
		_, _ = tx.Exec(ctx, `
			INSERT INTO registry_fields (organization_id, field_code, status) VALUES ($1, $2, 'empty')
			ON CONFLICT (organization_id, field_code) DO NOTHING`, orgID, m.code)
	}
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	a.oublierEchecs(ctx, cleIP)
	a.journaliser(ctx, &orgID, nil, &userID, "auth.register", map[string]interface{}{"email": email})

	ws := &webSession{app: a}
	if err := ws.creer(ctx, w, userID); err != nil {
		ecrireInterne(w, err)
		return
	}
	sess := a.lireSession(r.WithContext(ctx))
	// lireSession lit le cookie posé sur w, pas la requête : relire en direct.
	sess = a.lireSessionParJeton(ctx, ws.dernier)
	if sess == nil {
		sess = &sessionUtilisateur{UserID: userID}
	}
	ecrireJSON(w, http.StatusCreated, a.payloadAuth(ctx, sess))
}

func (a *App) inscrireParInvitation(ctx context.Context, w http.ResponseWriter, r *http.Request, corps *corpsInscription, nom, email, jeton string) {
	var invID, orgID, stationID, role string
	var state string
	var expiresAt time.Time
	var found bool
	err := a.db.QueryRow(ctx, `
		SELECT i.id, i.organization_id, COALESCE(i.station_id::text, ''), i.role, i.state, i.expires_at
		FROM invitations i WHERE i.token_hash = encode(digest($1, 'sha256'), 'hex')`, jeton).
		Scan(&invID, &orgID, &stationID, &role, &state, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		repondreErreur(w, http.StatusNotFound, "invitation_unknown", "Cette invitation n'existe pas ou n'a pas été créée par ce serveur.")
		return
	}
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	found = true
	_ = found
	switch {
	case state == "accepted":
		ecrireConflit(w, http.StatusConflict, conflit{code: "invitation_used", message: "Cette invitation a déjà été utilisée."})
		return
	case state == "revoked":
		ecrireConflit(w, http.StatusConflict, conflit{code: "invitation_revoked", message: "Cette invitation a été retirée."})
		return
	case time.Now().After(expiresAt):
		ecrireConflit(w, http.StatusConflict, conflit{code: "invitation_expired", message: "Cette invitation a expiré ; demandez-en une nouvelle."})
		return
	}
	if email == "" {
		ecrireValidation(w, map[string]string{"email": "Une adresse e-mail est requise."})
		return
	}
	motDePasse := corps.Password
	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)
	var existe int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE email = $1`, email).Scan(&existe)
	if existe > 0 {
		ecrireConflit(w, http.StatusConflict, conflit{code: "invitation_requires_login", message: "Ce compte existe déjà : connectez-vous, l'invitation sera acceptée depuis votre profil."})
		return
	}
	provisoire := motDePasse == ""
	if provisoire {
		motDePasse = genererCode(10) + "-Wg" // provisoire : changement obligatoire à la première connexion
	}
	hash, err := hacherMotDePasse(ctx, tx, motDePasse)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	var userID string
	var st interface{}
	if stationID != "" {
		st = stationID
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO users (organization_id, station_id, full_name, email, phone, role, password_hash, must_change_password)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true) RETURNING id`,
		orgID, st, nom, email, strings.TrimSpace(corps.Phone), role, hash).Scan(&userID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	_, _ = tx.Exec(ctx, `UPDATE invitations SET state = 'accepted', accepted_at = now() WHERE id = $1`, invID)
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	ws := &webSession{app: a}
	if err := ws.creer(ctx, w, userID); err != nil {
		ecrireInterne(w, err)
		return
	}
	sess := a.lireSessionParJeton(ctx, ws.dernier)
	if sess == nil {
		sess = &sessionUtilisateur{UserID: userID}
	}
	payload := a.payloadAuth(ctx, sess)
	payload["invitation_accepted"] = true
	if provisoire {
		payload["must_change_password"] = true
		if a.cfg.env != "production" {
			payload["temporary_password"] = motDePasse
		}
	}
	if a.cfg.env != "production" {
		payload["session_token"] = ws.dernier
	}
	ecrireJSON(w, http.StatusCreated, payload)
}

func (a *App) connecter(w http.ResponseWriter, r *http.Request, _ *sessionUtilisateur) {
	corps, ok := decoderCorps[struct {
		Identifier string `json:"identifier"`
		Email      string `json:"email"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}](w, r)
	if !ok {
		return
	}
	identifiant := strings.ToLower(strings.TrimSpace(firstNonEmpty(corps.Identifier, corps.Email, corps.Username)))
	ctx := r.Context()
	cle := "login:" + identifiant
	if a.echecsDepasse(ctx, cle, a.cfg.authMaxFailures, 15*time.Minute) {
		repondreErreur(w, http.StatusTooManyRequests, "too_many_requests",
			"Trop d'échecs de connexion. Réessayez dans quinze minutes.")
		return
	}
	if identifiant == "" || corps.Password == "" {
		ecrireValidation(w, map[string]string{"identifier": "Identifiant et mot de passe requis."})
		return
	}
	var userID, hash string
	var actif bool
	err := a.db.QueryRow(ctx, `
		SELECT id, password_hash, is_active FROM users WHERE lower(email) = $1 OR lower(phone) = $1 LIMIT 1`,
		strings.TrimLeft(identifiant, "+")).Scan(&userID, &hash, &actif)
	if errors.Is(err, pgx.ErrNoRows) {
		a.noterEchec(ctx, cle)
		repondreErreur(w, http.StatusUnauthorized, "unauthorized", "Identifiant ou mot de passe incorrect.")
		return
	}
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	var okMotDePasse bool
	_ = a.db.QueryRow(ctx, `SELECT crypt($1, $2) = $2`, corps.Password, hash).Scan(&okMotDePasse)
	if !okMotDePasse || !actif {
		a.noterEchec(ctx, cle)
		repondreErreur(w, http.StatusUnauthorized, "unauthorized", "Identifiant ou mot de passe incorrect.")
		return
	}
	a.oublierEchecs(ctx, cle)
	ws := &webSession{app: a}
	if err := ws.creer(ctx, w, userID); err != nil {
		ecrireInterne(w, err)
		return
	}
	sess := a.lireSessionParJeton(ctx, ws.dernier)
	if sess == nil {
		repondreErreur(w, http.StatusInternalServerError, "internal", "Session ouverte mais illisible.")
		return
	}
	payload := a.payloadAuth(ctx, sess)
	if a.cfg.env != "production" {
		payload["session_token"] = ws.dernier
	}
	ecrireJSON(w, http.StatusOK, payload)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (a *App) deconnecter(w http.ResponseWriter, r *http.Request, _ *sessionUtilisateur) {
	a.fermerSession(r)
	http.SetCookie(w, &http.Cookie{Name: "washguard_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"message": "Session fermée."})
}

// webSession : petite coquille pour récupérer le jeton posé par creerSession.
type webSession struct {
	app    *App
	dernier string
}

func (ws *webSession) creer(ctx context.Context, w http.ResponseWriter, userID string) error {
	jeton := jeterJeton(32)
	expiration := time.Now().Add(time.Duration(ws.app.cfg.sessionTTLHeures) * time.Hour)
	_, err := ws.app.db.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		empreinteJeton(jeton), userID, expiration)
	if err != nil {
		return err
	}
	ws.app.poserCookie(w, jeton, expiration)
	if ws.app.cfg.env != "production" {
		appGlobale.Mu.Lock()
		appGlobale.JetonsApercu[jeton] = true
		appGlobale.Mu.Unlock()
	}
	ws.dernier = jeton
	return nil
}

func (a *App) lireSessionParJeton(ctx context.Context, jeton string) *sessionUtilisateur {
	var s sessionUtilisateur
	err := a.db.QueryRow(ctx, `
		SELECT u.id, u.organization_id, u.station_id, u.full_name, u.email, u.phone, u.role,
		       u.must_change_password
		FROM sessions ses JOIN users u ON u.id = ses.user_id
		WHERE ses.token_hash = $1 AND ses.expires_at > now() AND u.is_active`,
		empreinteJeton(jeton)).
		Scan(&s.UserID, &s.OrganizationID, &s.StationID, &s.FullName, &s.Email, &s.Phone, &s.Role, &s.MustChange)
	if err != nil {
		return nil
	}
	s.permissions = permissionsDe(s.Role)
	s.token = jeton
	return &s
}

// payloadAuth construit la réponse commune de /auth/me, register et login.
func (a *App) payloadAuth(ctx context.Context, sess *sessionUtilisateur) map[string]interface{} {
	user := map[string]interface{}{
		"id": sess.UserID, "full_name": sess.FullName, "email": sess.Email,
		"phone": sess.Phone, "role": sess.Role, "must_change_password": sess.MustChange,
	}
	permissions := append([]string{}, rolePermissions[sess.Role]...)
	out := map[string]interface{}{"user": user, "permissions": permissions}
	if sess.OrganizationID == nil || *sess.OrganizationID == "" {
		out["organization"] = nil
		out["station"] = nil
		out["subscription"] = nil
		return out
	}
	org, err := a.lireOrganisation(ctx, *sess.OrganizationID)
	if err == nil {
		out["organization"] = org
	}
	stationID := ""
	if sess.StationID != nil && *sess.StationID != "" {
		stationID = *sess.StationID
	} else if org != nil {
		if st, err := a.lireStationOrganisation(ctx, *sess.OrganizationID); err == nil {
			stationID = st.ID
		}
	}
	var station *stationEntite
	if stationID != "" {
		if st, err := a.lireStationParID(ctx, stationID); err == nil {
			station = st
			renderu := st.rendre()
			renderu["opening_hours"] = a.hoursMapPourRendu(ctx, st.ID)
			out["station"] = renderu
		}
	} else {
		out["station"] = nil
	}
	if sub := a.lireAbonnement(ctx, *sess.OrganizationID); sub != nil {
		out["subscription"] = sub
	}
	// Onboarding réservé aux profils qui configurent (jamais caissier/laveur).
	if sess.aPermission("settings:view") {
		if ob, err := a.calculerOnboarding(ctx, *sess.OrganizationID, station); err == nil {
			out["onboarding"] = ob
		}
	}
	return out
}

var _ = fmt.Sprint

func (a *App) profilCourant(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	ecrireJSON(w, http.StatusOK, a.payloadAuth(r.Context(), sess))
}

func (a *App) lireOrganisation(ctx context.Context, id string) (map[string]interface{}, error) {
	var name, phone, email, city, country, legalName, billingAddress string
	var settingsBody []byte
	var createdAt time.Time
	err := a.db.QueryRow(ctx, `
		SELECT name, phone, email, city, country, legal_name, billing_address, settings, created_at
		FROM organizations WHERE id = $1`, id).
		Scan(&name, &phone, &email, &city, &country, &legalName, &billingAddress, &settingsBody, &createdAt)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id": id, "name": name, "phone": phone, "email": email, "city": city, "country": country,
		"legal_name": legalName, "billing_address": billingAddress, "settings": decoderObjet(settingsBody),
		"created_at": createdAt.UTC().Format(time.RFC3339),
	}, nil
}

func (a *App) lireAbonnement(ctx context.Context, organizationID string) map[string]interface{} {
ligne := a.db.QueryRow(ctx, `
		SELECT p.code, p.name, p.description, p.price_xof, p.billing_period, p.max_stations,
		       p.max_employees, p.max_wash_orders_month, p.features, s.status, s.trial_ends_at,
		       s.current_period_start, s.current_period_end
		FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		WHERE s.organization_id = $1 ORDER BY s.created_at DESC LIMIT 1`, organizationID)
	var code, nom, description, periode, statut string
	var prix, maxStations, maxEmployes, maxCommandes int
	var featuresBody []byte
	var trialEnds, periodeDebut, periodeFin *time.Time
	if err := ligne.Scan(&code, &nom, &description, &prix, &periode, &maxStations, &maxEmployes,
		&maxCommandes, &featuresBody, &statut, &trialEnds, &periodeDebut, &periodeFin); err != nil {
		return nil
	}
	statutsLabels := map[string]string{
		"trialing": "Essai", "active": "Actif", "past_due": "Paiement en attente",
		"expired": "Expiré", "suspended": "Suspendu", "canceled": "Résilié",
	}
	formate := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	var usageCommandes int
	_ = a.db.QueryRow(ctx, `
		SELECT count(*) FROM wash_orders
		WHERE organization_id = $1 AND created_at >= date_trunc('month', now())`, organizationID).Scan(&usageCommandes)
	var usageStations, usageEmployes int
	_ = a.db.QueryRow(ctx, `SELECT count(*) FROM stations WHERE organization_id = $1`, organizationID).Scan(&usageStations)
	_ = a.db.QueryRow(ctx, `SELECT count(*) FROM employees WHERE organization_id = $1 AND is_active`, organizationID).Scan(&usageEmployes)
	return map[string]interface{}{
		"plan_code": code, "plan_name": nom, "plan_description": description,
		"price_xof": prix, "billing_period": periode, "status": statut,
		"status_label": statutsLabels[statut],
		"trial_ends_at": formate(trialEnds), "current_period_start": formate(periodeDebut),
		"current_period_end": formate(periodeFin),
		"max_stations": maxStations, "max_employees": maxEmployes,
		"max_wash_orders_month": maxCommandes, "features": decoderObjet(featuresBody),
		"usage": map[string]interface{}{
			"period": time.Now().UTC().Format("2006-01"), "wash_orders": usageCommandes,
			"stations": usageStations, "employees": usageEmployes,
		},
	}
}

