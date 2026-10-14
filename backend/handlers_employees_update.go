package main

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Employés : mise à jour et réinitialisation de mot de passe.
//
// Le compte visé est chargé **avec** son entreprise : sans ce filtre, un
// gérant pouvait réinitialiser le mot de passe de n'importe quel compte de la
// base (support WashGuard compris). C'était le défaut le plus grave du projet.
// ---------------------------------------------------------------------------

// getOwnedEmployee charge un employé de l'entreprise de l'appelant.
func (a *App) getOwnedEmployee(r *http.Request, id string) (*User, error) {
	user := currentUser(r)
	target, err := scanUser(a.Pool.QueryRow(r.Context(), `
		SELECT id, organization_id, station_id, full_name, email, phone, password_hash, role,
		       is_active, must_change_password, last_login_at, created_at,
		       commission_mode, commission_per_wash_xof, commission_percent
		FROM users WHERE id = $1 AND organization_id = $2`, id, deref(user.OrganizationID)))
	if err == pgx.ErrNoRows {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, errNotFound
	}
	// Un gérant rattaché à une station ne gère que les comptes de sa station.
	if forced := scopedStationID(user); forced != "" && deref(target.StationID) != forced {
		return nil, errNotFound
	}
	return target, nil
}

func (a *App) handleEmployeeUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	target, err := a.getOwnedEmployee(r, id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// Un gérant ne touche ni au propriétaire ni aux comptes de la plateforme.
	if user.Role == "manager" && (target.Role == "owner" || target.Role == "superadmin") {
		writeError(w, http.StatusForbidden, "forbidden",
			"Un gérant ne peut pas modifier le compte du propriétaire.", nil)
		return
	}
	if target.Role == "superadmin" && user.Role != "superadmin" {
		writeError(w, http.StatusForbidden, "forbidden", "Compte protégé.", nil)
		return
	}

	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if v, ok := body["role"].(string); ok && v != target.Role {
		if !roleAutorise(user.Role, v) {
			writeError(w, http.StatusForbidden, "forbidden",
				"Ce rôle ne peut pas être attribué par votre profil.", nil)
			return
		}
	}
	if v, ok := body["station_id"].(string); ok && v != "" {
		if err := a.stationAutorisee(r.Context(), deref(user.OrganizationID), v); err != nil {
			writeAPIError(w, err)
			return
		}
		if forced := scopedStationID(user); forced != "" && forced != v {
			writeAPIError(w, errStationForbidden)
			return
		}
	}

	// Les données de rémunération ne se modifient pas avec les droits de
	// gestion d'équipe : il faut la permission dédiée. Un gérant qui n'a pas
	// cette permission gère les comptes, pas les salaires.
	peutRemunerer := hasPermission(user, "employee:compensation")
	for _, champ := range []string{"commission_mode", "commission_percent", "commission_per_wash_xof"} {
		if _, present := body[champ]; present && !peutRemunerer {
			writeError(w, http.StatusForbidden, "forbidden",
				"Les informations de rémunération demandent une permission dédiée.",
				map[string]string{champ: "Permission « employee:compensation » requise"})
			return
		}
	}

	if v, ok := body["employment_status"].(string); ok {
		if _, connu := libellesSituationEmploye[v]; !connu {
			writeError(w, http.StatusBadRequest, "validation_error", "Situation inconnue.",
				map[string]string{"employment_status": "Valeurs acceptées : active, on_leave, departed"})
			return
		}
	}
	if v, ok := body["hired_on"].(string); ok && v != "" {
		if _, err := dateOuNil(v, "hired_on"); err != nil {
			writeAPIError(w, err)
			return
		}
	}

	allowed := map[string]string{
		"full_name": "full_name", "phone": "phone", "role": "role",
		"is_active": "is_active", "station_id": "station_id",
		"job_title": "job_title", "work_schedule": "work_schedule",
		"employment_status": "employment_status", "hired_on": "hired_on",
		"commission_mode": "commission_mode", "commission_percent": "commission_percent",
	}
	changed := []string{}
	for field, column := range allowed {
		if v, ok := body[field]; ok {
			if texte, estTexte := v.(string); estTexte && texte == "" && column == "hired_on" {
				// Une date effacée redevient NULL : « je ne sais pas » n'est pas
				// le 1ᵉʳ janvier 1970.
				if _, err := a.Pool.Exec(r.Context(),
					`UPDATE users SET hired_on = NULL WHERE id = $1 AND organization_id = $2`,
					id, deref(user.OrganizationID)); err == nil {
					changed = append(changed, field)
				}
				continue
			}
			if _, err := a.Pool.Exec(r.Context(), `UPDATE users SET `+column+` = $1 WHERE id = $2 AND organization_id = $3`, v, id, deref(user.OrganizationID)); err != nil {
				writeError(w, http.StatusBadRequest, "validation_error", "Valeur refusée pour "+field+".", nil)
				return
			}
			changed = append(changed, field)
		}
	}
	if v, ok := body["commission_per_wash_xof"].(float64); ok && peutRemunerer {
		_, _ = a.Pool.Exec(r.Context(),
			`UPDATE users SET commission_per_wash_xof = $1 WHERE id = $2 AND organization_id = $3`,
			int(v), id, deref(user.OrganizationID))
		changed = append(changed, "commission_per_wash_xof")
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"employee.updated", "employee", id,
		map[string]any{"actor_role": user.Role, "fields": changed})
	writeJSON(w, http.StatusOK, map[string]any{"message": "Employé mis à jour."})
}

func (a *App) handleEmployeeResetPassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	target, err := a.getOwnedEmployee(r, id)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if user.Role == "manager" && (target.Role == "owner" || target.Role == "superadmin") {
		writeError(w, http.StatusForbidden, "forbidden",
			"Un gérant ne peut pas réinitialiser le mot de passe du propriétaire.", nil)
		return
	}
	newPass := tempPassword()
	hash, err := hashPassword(newPass)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	_, err = a.Pool.Exec(r.Context(),
		`UPDATE users SET password_hash = $1, must_change_password = true WHERE id = $2 AND organization_id = $3`,
		hash, id, deref(user.OrganizationID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	// Les sessions ouvertes du compte sont fermées : l'ancien mot de passe ne
	// laisse aucune porte ouverte.
	_, _ = a.Pool.Exec(r.Context(), `DELETE FROM sessions WHERE user_id = $1`, id)
	a.addAudit(r.Context(), deref(user.OrganizationID), deref(target.StationID), user.FullName,
		"employee.password_reset", "employee", id,
		map[string]any{"employee_name": target.FullName, "actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"temporary_password": newPass, "message": "Mot de passe réinitialisé."})
}
