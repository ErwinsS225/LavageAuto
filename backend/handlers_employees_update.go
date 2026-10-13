package main

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Employés (mise à jour + réinitialisation de mot de passe).
// ---------------------------------------------------------------------------

func (a *App) handleEmployeeUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if isRestrictedRole(user.Role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Seul le gérant ou le propriétaire peut modifier les employés.", nil)
		return
	}
	id := r.PathValue("id")
	var targetRole string
	err := a.Pool.QueryRow(r.Context(), `SELECT role FROM users WHERE id = $1`, id).Scan(&targetRole)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Employé introuvable.", nil)
		return
	}
	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if user.Role == "manager" {
		if v, ok := body["role"].(string); ok && v == "owner" {
			writeError(w, http.StatusForbidden, "forbidden",
				"Un gérant ne peut pas attribuer le rôle Propriétaire.", nil)
			return
		}
	}
	allowed := map[string]string{
		"full_name": "full_name", "phone": "phone", "role": "role",
		"is_active": "is_active", "station_id": "station_id", "commission_mode": "commission_mode",
	}
	for field, column := range allowed {
		if v, ok := body[field]; ok {
			_, _ = a.Pool.Exec(r.Context(), `UPDATE users SET `+column+` = $1 WHERE id = $2`, v, id)
		}
	}
	if v, ok := body["commission_per_wash_xof"].(float64); ok {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE users SET commission_per_wash_xof = $1 WHERE id = $2`, int(v), id)
	}
	if v, ok := body["commission_percent"].(float64); ok {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE users SET commission_percent = $1 WHERE id = $2`, v, id)
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), "", user.FullName,
		"employee.updated", "employee", id,
		map[string]any{"actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"message": "Employé mis à jour."})
}

func (a *App) handleEmployeeResetPassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if isRestrictedRole(user.Role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Seul le gérant ou le propriétaire peut réinitialiser un mot de passe.", nil)
		return
	}
	id := r.PathValue("id")
	var fullName string
	err := a.Pool.QueryRow(r.Context(), `SELECT full_name FROM users WHERE id = $1`, id).Scan(&fullName)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Employé introuvable.", nil)
		return
	}
	newPass := tempPassword()
	hash, err := hashPassword(newPass)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	_, _ = a.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, must_change_password = true WHERE id = $2`, hash, id)
	a.addAudit(r.Context(), deref(user.OrganizationID), "", user.FullName,
		"employee.password_reset", "employee", id,
		map[string]any{"employee_name": fullName, "actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"temporary_password": newPass, "message": "Mot de passe réinitialisé."})
}
