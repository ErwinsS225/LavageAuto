package main

import (
	"net/http"
	"net/mail"
	"strings"
)

// ---------------------------------------------------------------------------
// Employés : création d'un compte.
// ---------------------------------------------------------------------------

// tableauRoles : rôles qu'un créateur est autorisé à attribuer.
// Le rôle « superadmin » (support WashGuard) ne s'attribue jamais par l'API
// d'une entreprise : il ne peut venir que du seed de la plateforme.
func roleAutorise(pourRole, role string) bool {
	switch role {
	case "owner":
		return pourRole == "owner"
	case "manager", "cashier", "washer":
		return pourRole == "owner" || pourRole == "manager"
	default:
		return false
	}
}

func (a *App) handleEmployeeCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		StationID            string   `json:"station_id"`
		FullName             string   `json:"full_name"`
		Email                string   `json:"email"`
		Phone                string   `json:"phone"`
		Password             string   `json:"password"`
		Role                 string   `json:"role"`
		JobTitle             string   `json:"job_title"`
		HiredOn              string   `json:"hired_on"`
		WorkSchedule         string   `json:"work_schedule"`
		EmploymentStatus     string   `json:"employment_status"`
		CommissionMode       *string  `json:"commission_mode"`
		CommissionPerWashXOF *int     `json:"commission_per_wash_xof"`
		CommissionPercent    *float64 `json:"commission_percent"`
	}
	if err := decodeBody(r, &body); err != nil || body.FullName == "" || body.Email == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Nom et e-mail obligatoires.", nil)
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Adresse e-mail invalide.",
			map[string]string{"email": "Format d'e-mail invalide"})
		return
	}
	role := body.Role
	if role == "" {
		role = "cashier"
	}
	if !roleAutorise(user.Role, role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Ce rôle ne peut pas être attribué par votre profil.", nil)
		return
	}
	if body.Password != "" && len(body.Password) < minPasswordLength {
		writeAPIError(w, passwordTooShort("password"))
		return
	}
	// La rémunération est une donnée réservée : elle demande la permission
	// dédiée, en plus du droit de créer un compte.
	if !hasPermission(user, "employee:compensation") &&
		(body.CommissionMode != nil || body.CommissionPerWashXOF != nil || body.CommissionPercent != nil) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Les informations de rémunération demandent une permission dédiée.",
			map[string]string{"commission_mode": "Permission « employee:compensation » requise"})
		return
	}
	situation := orDefault(strings.TrimSpace(body.EmploymentStatus), "active")
	if _, connu := libellesSituationEmploye[situation]; !connu {
		writeError(w, http.StatusBadRequest, "validation_error", "Situation inconnue.",
			map[string]string{"employment_status": "Valeurs acceptées : active, on_leave, departed"})
		return
	}
	embauche, err := dateOuNil(body.HiredOn, "hired_on")
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if err := a.checkPlanLimit(r, user, limitEmployees); err != nil {
		writeAPIError(w, err)
		return
	}

	stationID, err := a.resolveStation(r.Context(), user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// Un laveur sans station n'apparaît dans aucune file de travail : l'écran
	// le dit, le serveur le refuse.
	if role == "washer" && stationID == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Un laveur doit être rattaché à une station.", map[string]string{"station_id": "Station requise"})
		return
	}

	plain := body.Password
	generated := false
	if plain == "" {
		plain = tempPassword()
		generated = true
	}
	hash, err := hashPassword(plain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	id := newID("usr")
	_, err = a.Pool.Exec(r.Context(), `
		INSERT INTO users (id, organization_id, station_id, full_name, email, phone,
		                   password_hash, role, must_change_password, job_title, hired_on,
		                   work_schedule, employment_status,
		                   commission_mode, commission_per_wash_xof, commission_percent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		id, deref(user.OrganizationID), stationID, body.FullName,
		strings.ToLower(body.Email), body.Phone, hash, role, generated,
		strings.TrimSpace(body.JobTitle), embauche, strings.TrimSpace(body.WorkSchedule), situation,
		body.CommissionMode, body.CommissionPerWashXOF, body.CommissionPercent)
	if err != nil {
		writeError(w, http.StatusConflict, "conflict", "Cet e-mail est déjà utilisé par un compte.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"employee.created", "employee", id,
		map[string]any{"full_name": body.FullName, "email": strings.ToLower(body.Email), "role": role, "actor_role": user.Role})

	resp := map[string]any{
		"id": id, "message": "Employé créé.", "email": strings.ToLower(body.Email),
		"role": role, "password_generated": generated,
	}
	if generated {
		resp["temporary_password"] = plain
	}
	writeJSON(w, http.StatusCreated, resp)
}
