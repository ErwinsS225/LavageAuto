package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Invitations : une personne rejoint une entreprise qui existe déjà.
//
// Le choix de fond : **WashGuard n'envoie rien**. Ni e-mail, ni SMS, ni message
// WhatsApp — le produit ne dispose d'aucun moyen d'envoi et ne prétend pas en
// avoir. Le propriétaire obtient donc deux choses à transmettre lui-même : un
// code court (dictable au téléphone) et un lien (partageable, notamment par
// WhatsApp depuis l'interface). C'est dit dans l'écran, pas caché dans une
// infobulle.
// ---------------------------------------------------------------------------

// Rôles qu'une invitation peut attribuer. « owner » en est exclu : la propriété
// d'une entreprise ne s'obtient pas par un lien.
var rolesInvitation = map[string]bool{"manager": true, "cashier": true, "washer": true}

// alphabetCode évite 0/O, 1/I/L : un code se dicte au téléphone et se recopie à
// la main sans hésitation.
const alphabetCode = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

const (
	// dureeInvitation : deux semaines, au-delà une invitation oubliée traîne
	// dans la liste sans que personne ne sache si elle est encore valable.
	dureeInvitation = 14 * 24 * time.Hour
	// dureeInvitationMax : on ne stocke pas une invitation éternelle.
	dureeInvitationMax = 90 * 24 * time.Hour
	longueurCode       = 8
)

// invitation : une ligne de la table, telle que lue pour être servie.
type invitation struct {
	ID         string
	OrgID      string
	StationID  string
	Email      string
	Phone      string
	Role       string
	Code       string
	ExpiresAt  time.Time
	InvitedBy  string
	AcceptedAt *time.Time
	AcceptedBy string
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// nouvelAlphabetCode : un code court, sans caractères ambigus.
func nouveauCode() string {
	buffer := make([]byte, longueurCode)
	if _, err := rand.Read(buffer); err != nil {
		// rand.Read n'échoue pas en pratique ; si cela arrivait, mieux vaut un
		// code imprévisible qu'aucun code.
		return "INV00000"
	}
	for i, octet := range buffer {
		buffer[i] = alphabetCode[int(octet)%len(alphabetCode)]
	}
	return string(buffer)
}

// empreinteToken : seule l'empreinte du jeton est stockée.
func empreinteToken(token string) string {
	somme := sha256.Sum256([]byte(token))
	return hex.EncodeToString(somme[:])
}

// normaliserCode : un code dicté arrive en minuscules, avec des espaces ou des
// tirets. On le remet dans sa forme canonique avant toute recherche.
func normaliserCode(saisie string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(saisie)) {
		if strings.ContainsRune(alphabetCode, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// etatInvitation : l'état se calcule, il ne se stocke pas — « expirée » dépend
// de l'heure de lecture, et une colonne « status » finirait par mentir.
func etatInvitation(inv invitation) string {
	switch {
	case inv.RevokedAt != nil:
		return "revoked"
	case inv.AcceptedAt != nil:
		return "accepted"
	case inv.ExpiresAt.Before(time.Now()):
		return "expired"
	default:
		return "pending"
	}
}

func etatInvitationLabelFR(etat string) string {
	switch etat {
	case "revoked":
		return "Révoquée"
	case "accepted":
		return "Acceptée"
	case "expired":
		return "Expirée"
	default:
		return "En attente"
	}
}

func invitationToJSON(inv invitation) map[string]any {
	etat := etatInvitation(inv)
	return map[string]any{
		"id":              inv.ID,
		"station_id":      inv.StationID,
		"email":           inv.Email,
		"phone":           inv.Phone,
		"role":            inv.Role,
		"role_label":      roleLabelFR(inv.Role),
		"code":            inv.Code,
		"state":           etat,
		"state_label":     etatInvitationLabelFR(etat),
		"expires_at":      inv.ExpiresAt.Format(time.RFC3339),
		"accepted_at":     formatNullableTime(inv.AcceptedAt),
		"revoked_at":      formatNullableTime(inv.RevokedAt),
		"created_at":      inv.CreatedAt.Format(time.RFC3339),
		"invited_by_name": inv.InvitedBy,
	}
}

func formatNullableTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// lireInvitationByCode : recherche par code court (forme canonique).
func (a *App) lireInvitationByCode(ctx context.Context, code string) (invitation, bool, error) {
	return a.lireInvitation(ctx, `WHERE i.code = $1`, normaliserCode(code))
}

// lireInvitationByToken : recherche par jeton long, via son empreinte.
func (a *App) lireInvitationByToken(ctx context.Context, token string) (invitation, bool, error) {
	if strings.TrimSpace(token) == "" {
		return invitation{}, false, nil
	}
	return a.lireInvitation(ctx, `WHERE i.token_hash = $1`, empreinteToken(token))
}

func (a *App) lireInvitation(ctx context.Context, filtre string, valeur string) (invitation, bool, error) {
	var inv invitation
	err := a.Pool.QueryRow(ctx, `
		SELECT i.id, i.organization_id, COALESCE(i.station_id,''), i.email, i.phone, i.role,
		       i.code, i.expires_at, COALESCE(u.full_name,''), i.accepted_at,
		       COALESCE(i.accepted_by,''), i.revoked_at, i.created_at
		FROM invitations i
		LEFT JOIN users u ON u.id = i.invited_by
		`+filtre, valeur).
		Scan(&inv.ID, &inv.OrgID, &inv.StationID, &inv.Email, &inv.Phone, &inv.Role,
			&inv.Code, &inv.ExpiresAt, &inv.InvitedBy, &inv.AcceptedAt,
			&inv.AcceptedBy, &inv.RevokedAt, &inv.CreatedAt)
	if err == pgx.ErrNoRows {
		return invitation{}, false, nil
	}
	if err != nil {
		return invitation{}, false, err
	}
	return inv, true, nil
}

func (a *App) lireInvitationParID(ctx context.Context, orgID, id string) (invitation, bool, error) {
	var inv invitation
	err := a.Pool.QueryRow(ctx, `
		SELECT i.id, i.organization_id, COALESCE(i.station_id,''), i.email, i.phone, i.role,
		       i.code, i.expires_at, COALESCE(u.full_name,''), i.accepted_at,
		       COALESCE(i.accepted_by,''), i.revoked_at, i.created_at
		FROM invitations i
		LEFT JOIN users u ON u.id = i.invited_by
		WHERE i.id = $1 AND i.organization_id = $2`, id, orgID).
		Scan(&inv.ID, &inv.OrgID, &inv.StationID, &inv.Email, &inv.Phone, &inv.Role,
			&inv.Code, &inv.ExpiresAt, &inv.InvitedBy, &inv.AcceptedAt,
			&inv.AcceptedBy, &inv.RevokedAt, &inv.CreatedAt)
	if err == pgx.ErrNoRows {
		return invitation{}, false, nil
	}
	if err != nil {
		return invitation{}, false, err
	}
	return inv, true, nil
}

// ---------------------------------------------------------------------------
// POST /api/v1/invitations — créer une invitation.
// ---------------------------------------------------------------------------

func (a *App) handleInvitationCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		StationID string `json:"station_id"`
		Email     string `json:"email"`
		Phone     string `json:"phone"`
		Role      string `json:"role"`
		Days      int    `json:"validity_days"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	email := strings.ToLower(strings.TrimSpace(body.Email))
	phone := strings.TrimSpace(body.Phone)
	champs := map[string]string{}
	if email == "" && phone == "" {
		champs["email"] = "Indiquez une adresse e-mail ou un numéro de téléphone."
		champs["phone"] = "Indiquez une adresse e-mail ou un numéro de téléphone."
	}
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			champs["email"] = "Format d'e-mail invalide."
		}
	}
	if phone != "" && len(normaliserTelephone(phone)) < 8 {
		champs["phone"] = "Numéro de téléphone trop court."
	}
	role := body.Role
	if role == "" {
		role = "cashier"
	}
	if !rolesInvitation[role] {
		champs["role"] = "Rôle invitable invalide : gérant, caissier ou laveur."
	}
	if !roleAutorise(user.Role, role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Ce rôle ne peut pas être attribué par votre profil.", nil)
		return
	}
	if len(champs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"L'invitation n'a pas pu être préparée : corrigez les champs signalés.", champs)
		return
	}

	// La station : celle demandée si elle est dans l'entreprise, sinon celle du
	// compte, sinon celle par défaut. Un gérant peut couvrir toute l'entreprise
	// (aucune station) ; un caissier ou un laveur doit en avoir une, sinon son
	// compte ne servirait à rien.
	stationID, err := a.stationDeTravail(r.Context(), user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" && role != "manager" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Une station est nécessaire pour ce rôle.",
			map[string]string{"station_id": "Choisissez la station de travail."})
		return
	}

	if err := a.checkPlanLimitForOrg(r.Context(), deref(user.OrganizationID), limitEmployees); err != nil {
		writeAPIError(w, err)
		return
	}

	duree := dureeInvitation
	if body.Days > 0 {
		duree = time.Duration(body.Days) * 24 * time.Hour
		if duree > dureeInvitationMax {
			duree = dureeInvitationMax
		}
	}

	// Le code et le jeton sont tirés jusqu'à obtenir des valeurs libres : une
	// collision est improbable, mais un doublon ferait échouer la création avec
	// une erreur incompréhensible pour l'utilisateur.
	orgID := deref(user.OrganizationID)
	token := ""
	code := ""
	for essai := 0; essai < 8; essai++ {
		candidatCode := nouveauCode()
		var pris bool
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM invitations WHERE code = $1)`, candidatCode).Scan(&pris); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if pris {
			continue
		}
		candidatToken := newToken()
		code, token = candidatCode, candidatToken
		break
	}
	if token == "" {
		// Aucun code libre après huit essais : le tirage aléatoire est en cause,
		// pas l'utilisateur. On allonge le code pour sortir de l'impasse.
		code = nouveauCode() + nouveauCode()[:2]
		token = newToken()
	}

	id := newID("inv")
	var station *string
	if stationID != "" {
		station = &stationID
	}
	var emailCol, phoneCol any
	if email != "" {
		emailCol = email
	}
	if phone != "" {
		phoneCol = phone
	}
	// Deux colonnes NOT NULL à défaut vide : une invitation par téléphone n'a
	// pas d'e-mail, et inversement.
	_, err = a.Pool.Exec(r.Context(), `
		INSERT INTO invitations
		  (id, organization_id, station_id, email, phone, role, token_hash, code, expires_at, invited_by)
		VALUES ($1,$2,$3,COALESCE($4,''),COALESCE($5,''),$6,$7,$8,$9,$10)`,
		id, orgID, station, emailCol, phoneCol, role,
		empreinteToken(token), code, time.Now().Add(duree), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de l'invitation.", nil)
		return
	}

	a.addAudit(r.Context(), orgID, stationID, user.FullName, "invitation.created", "invitation", id,
		map[string]any{"role": role, "email": email, "phone": phone})

	// Le jeton en clair n'est renvoyé qu'ici, une seule fois : il n'est pas
	// stocké et ne pourra pas être relu.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"code":       code,
		"token":      token,
		"role":       role,
		"expires_at": time.Now().Add(duree).Format(time.RFC3339),
		"message":    "Invitation créée. Transmettez-la vous-même : WashGuard n'envoie aucun message.",
		"notice": "Le lien complet n'est affiché qu'une fois. Vous pouvez aussi dicter le code, " +
			"il désigne la même invitation.",
	})
}

// ---------------------------------------------------------------------------
// GET /api/v1/invitations — liste de l'entreprise.
// ---------------------------------------------------------------------------

func (a *App) handleInvitationsList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	rows, err := a.Pool.Query(r.Context(), `
		SELECT i.id, i.organization_id, COALESCE(i.station_id,''), i.email, i.phone, i.role,
		       i.code, i.expires_at, COALESCE(u.full_name,''), i.accepted_at,
		       COALESCE(i.accepted_by,''), i.revoked_at, i.created_at
		FROM invitations i
		LEFT JOIN users u ON u.id = i.invited_by
		WHERE i.organization_id = $1
		ORDER BY i.created_at DESC LIMIT 100`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var inv invitation
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.StationID, &inv.Email, &inv.Phone, &inv.Role,
			&inv.Code, &inv.ExpiresAt, &inv.InvitedBy, &inv.AcceptedAt,
			&inv.AcceptedBy, &inv.RevokedAt, &inv.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		items = append(items, invitationToJSON(inv))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// ---------------------------------------------------------------------------
// POST /api/v1/invitations/{id}/revoke — retirer une invitation en attente.
// ---------------------------------------------------------------------------

func (a *App) handleInvitationRevoke(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	inv, trouve, err := a.lireInvitationParID(r.Context(), orgID, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if !trouve {
		writeError(w, http.StatusNotFound, "not_found", "Invitation introuvable.", nil)
		return
	}
	etat := etatInvitation(inv)
	if etat == "accepted" {
		writeError(w, http.StatusConflict, "conflict",
			"Cette invitation a déjà été acceptée : le compte existe. Désactivez-le depuis la liste des employés.", nil)
		return
	}
	if etat == "revoked" {
		writeJSON(w, http.StatusOK, map[string]any{"message": "Invitation déjà révoquée.", "state": "revoked"})
		return
	}
	if _, err := a.Pool.Exec(r.Context(),
		`UPDATE invitations SET revoked_at = now() WHERE id = $1 AND organization_id = $2`,
		inv.ID, orgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), orgID, inv.StationID, user.FullName, "invitation.revoked", "invitation", inv.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Invitation révoquée.", "state": "revoked"})
}

// ---------------------------------------------------------------------------
// GET /api/v1/invitations/{code} — aperçu public, sans session.
//
// C'est la seule route d'entreprise accessible sans compte. Elle ne renvoie
// donc **jamais** d'identifiant : le nom de l'entreprise et sa ville suffisent
// à ce que l'invité reconnaisse son employeur, et ne permettent pas d'énumérer
// le système. Un code inventé ne révèle rien (404 uniforme).
// ---------------------------------------------------------------------------

func (a *App) handleInvitationPreview(w http.ResponseWriter, r *http.Request) {
	saisie := r.PathValue("code")
	if jeton := r.URL.Query().Get("jeton"); jeton != "" {
		saisie = jeton
	}

	// Le jeton long (43 caractères) et le code court (8) ne peuvent pas se
	// confondre : la longueur décide de la recherche, sans table d'index.
	var (
		inv    invitation
		trouve bool
		err    error
	)
	if normaliserCode(saisie) != "" && len(normaliserCode(saisie)) == longueurCode {
		inv, trouve, err = a.lireInvitationByCode(r.Context(), saisie)
	} else {
		inv, trouve, err = a.lireInvitationByToken(r.Context(), saisie)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if !trouve {
		writeError(w, http.StatusNotFound, "invitation_unknown",
			"Ce code ne correspond à aucune invitation. Vérifiez la saisie auprès de la personne qui vous a invité.", nil)
		return
	}

	var orgName, ville string
	if err := a.Pool.QueryRow(r.Context(),
		`SELECT name, city FROM organizations WHERE id = $1`, inv.OrgID).Scan(&orgName, &ville); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	etat := etatInvitation(inv)
	payload := map[string]any{
		"organization_name": orgName,
		"city":              ville,
		"role":              inv.Role,
		"role_label":        roleLabelFR(inv.Role),
		"state":             etat,
		"state_label":       etatInvitationLabelFR(etat),
		"expires_at":        inv.ExpiresAt.Format(time.RFC3339),
		"usable":            etat == "pending",
	}
	switch etat {
	case "pending":
		payload["message"] = "Invitation valable. Créez votre compte ou connectez-vous pour la rejoindre."
	case "expired":
		payload["message"] = "Cette invitation a expiré. Demandez-en une nouvelle à votre employeur."
	case "revoked":
		payload["message"] = "Cette invitation a été retirée par votre employeur."
	case "accepted":
		payload["message"] = "Cette invitation a déjà été acceptée : connectez-vous avec le compte créé."
	}
	// Aucun identifiant dans cette réponse : ni organisation, ni station, ni
	// utilisateur. L'aperçu sert à reconnaître l'employeur, pas à explorer la base.
	writeJSON(w, http.StatusOK, payload)
}

// normaliserTelephone : ne garde que les chiffres, pour comparer deux écritures
// du même numéro (« +225 07 07…” et « 2250707…”).
func normaliserTelephone(valeur string) string {
	var b strings.Builder
	for _, r := range valeur {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Inscription par invitation : rejoindre une entreprise existante.
// ---------------------------------------------------------------------------

// inscrireParInvitation crée le compte de la personne invitée.
//
// Ce chemin ne crée **aucune** entreprise, **aucune** station et ne touche pas
// à l'abonnement : il ajoute un utilisateur à ce qui existe déjà, avec le rôle
// et la station portés par l'invitation. C'est la différence de fond avec
// l'inscription classique — et la raison pour laquelle elle a son propre code.
func (a *App) inscrireParInvitation(w http.ResponseWriter, r *http.Request,
	token, fullName, email, phone, password string) {

	ctx := r.Context()
	fullName = strings.TrimSpace(fullName)
	email = strings.ToLower(strings.TrimSpace(email))
	if fullName == "" || email == "" || password == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Nom, e-mail et mot de passe sont obligatoires.", nil)
		return
	}
	if len(password) < minPasswordLength {
		writeAPIError(w, passwordTooShort("password"))
		return
	}
	if _, err := mail.ParseAddress(email); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Adresse e-mail invalide.",
			map[string]string{"email": "Format d'e-mail invalide"})
		return
	}

	inv, trouve, err := a.lireInvitationByToken(ctx, token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if !trouve {
		writeError(w, http.StatusNotFound, "invitation_unknown",
			"Cette invitation est introuvable. Vérifiez le lien ou le code.", nil)
		return
	}
	switch etatInvitation(inv) {
	case "expired":
		writeError(w, http.StatusConflict, "invitation_expired",
			"Cette invitation a expiré. Demandez-en une nouvelle à votre employeur.", nil)
		return
	case "revoked":
		writeError(w, http.StatusConflict, "invitation_revoked",
			"Cette invitation a été retirée par votre employeur.", nil)
		return
	case "accepted":
		writeError(w, http.StatusConflict, "invitation_used",
			"Cette invitation a déjà été utilisée. Connectez-vous avec le compte créé, ou demandez une nouvelle invitation.", nil)
		return
	}

	if existing, err := a.getUserByEmail(ctx, email); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	} else if existing != nil {
		// Un compte WashGuard appartient à une seule entreprise : on ne
		// rattache pas un compte existant à une seconde. La personne doit
		// utiliser une autre adresse, ou se connecter si c'est bien son compte.
		writeError(w, http.StatusConflict, "invitation_requires_login",
			"Un compte existe déjà avec cette adresse e-mail. Connectez-vous avec, ou faites-vous inviter sur une autre adresse.", nil)
		return
	}

	if err := a.checkPlanLimitForOrg(ctx, inv.OrgID, limitEmployees); err != nil {
		writeAPIError(w, err)
		return
	}

	hash, err := hashPassword(password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	userID := newID("usr")
	var station *string
	if inv.StationID != "" {
		station = &inv.StationID
	}
	if phone == "" {
		phone = inv.Phone
	}

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, organization_id, station_id, full_name, email, phone, password_hash, role)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		userID, inv.OrgID, station, fullName, email, phone, hash, inv.Role); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création du compte.", nil)
		return
	}

	// L'invitation est marquée acceptée dans la même transaction : un compte
	// créé sans invitation consommée laisserait un lien utilisable par un tiers.
	if _, err := tx.Exec(ctx, `
		UPDATE invitations SET accepted_at = now(), accepted_by = $1
		WHERE id = $2 AND accepted_at IS NULL AND revoked_at IS NULL`, userID, inv.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, inv.OrgID, inv.StationID, fullName, "invitation.accepted", "invitation", inv.ID,
		map[string]any{"role": inv.Role, "email": email})

	session := newToken()
	_ = a.createSession(ctx, session, userID)
	a.setSessionCookie(w, r, session)

	user, _ := a.getUserByID(ctx, userID)
	payload, err := a.buildMePayload(ctx, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	// `invitation_accepted` dit à l'écran d'où vient ce compte : il rejoint une
	// entreprise existante, il n'en a pas créé une.
	payload["invitation_accepted"] = true
	writeJSON(w, http.StatusCreated, payload)
}
