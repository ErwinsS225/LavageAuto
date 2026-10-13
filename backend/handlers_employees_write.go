package main

import (
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Employés (création).
// ---------------------------------------------------------------------------

func (a *App) handleEmployeeCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if isRestrictedRole(user.Role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Seul le gérant ou le propriétaire est autorisé à ajouter des employés.", nil)
		return
	}
	var body struct {
		StationID            string   `json:"station_id"`
		FullName             string   `json:"full_name"`
		Email                string   `json:"email"`
		Phone                string   `json:"phone"`
		Password             string   `json:"password"`
		Role                 string   `json:"role"`
		CommissionMode       *string  `json:"commission_mode"`
		CommissionPerWashXOF *int     `json:"commission_per_wash_xof"`
		CommissionPercent    *float64 `json:"commission_percent"`
	}
	if err := decodeBody(r, &body); err != nil || body.FullName == "" || body.Email == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Nom et e-mail obligatoires.", nil)
		return
	}
	role := body.Role
	if role == "" {
		role = "cashier"
	}
	if user.Role == "manager" && (role == "owner" || role == "superadmin") {
		writeError(w, http.StatusForbidden, "forbidden",
			"Un gérant ne peut créer que des comptes Gérant, Caissier ou Laveur.", nil)
		return
	}
	stationID := body.StationID
	if stationID == "" {
		stationID = deref(user.StationID)
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
		                   password_hash, role, must_change_password,
		                   commission_mode, commission_per_wash_xof, commission_percent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		id, deref(user.OrganizationID), stationID, body.FullName,
		strings.ToLower(body.Email), body.Phone, hash, role, generated,
		body.CommissionMode, body.CommissionPerWashXOF, body.CommissionPercent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "E-mail déjà utilisé ou erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"employee.created", "employee", id,
		map[string]any{"full_name": body.FullName, "email": body.Email, "role": role, "actor_role": user.Role})

	resp := map[string]any{
		"id": id, "message": "Employé créé.", "email": strings.ToLower(body.Email),
		"role": role, "password_generated": generated,
	}
	if generated {
		resp["temporary_password"] = plain
	}
	writeJSON(w, http.StatusCreated, resp)
}

