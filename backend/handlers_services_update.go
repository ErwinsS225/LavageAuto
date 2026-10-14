package main

import (
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Catalogue de prestations (mise à jour + désactivation).
//
// Une prestation appartient à une entreprise : la charger sans filtre
// `organization_id` revenait à laisser une entreprise modifier le tarif d'une
// autre. C'est ce que corrige getOwnedService.
//
// Depuis le lot 5, une prestation ne porte plus forcément son prix : elle peut
// être tarifée par règles (prestation × véhicule × usage, contrat client) ou
// être gratuite par décision. Modifier `price_xof` reste possible — c'est le
// **tarif du catalogue**, celui qui sert de repli — mais on ne mélange pas les
// deux registres : une prestation gratuite ne porte pas de prix.
// ---------------------------------------------------------------------------

func (a *App) handleServiceUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	id := r.PathValue("id")

	service, err := a.getOwnedService(r, id, orgID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != service.StationID {
		writeAPIError(w, errStationForbidden)
		return
	}

	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	// Champs de texte : une seule requête par champ, et une erreur remontée —
	// l'écran ne doit jamais croire avoir enregistré ce qui a échoué.
	allowed := map[string]string{
		"name": "name", "category": "category", "description": "description",
		"family_code": "family_code", "equipment_notes": "equipment_notes",
		"safety_notes": "safety_notes", "preconditions": "preconditions",
	}
	erreurs := map[string]string{}
	for field, column := range allowed {
		v, ok := body[field]
		if !ok {
			continue
		}
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE services SET `+column+` = $1 WHERE id = $2 AND organization_id = $3`,
			toText(v), id, orgID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
	}
	for field, column := range map[string]string{"sort_order": "sort_order", "duration_minutes": "duration_minutes"} {
		v, ok := body[field].(float64)
		if !ok {
			continue
		}
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE services SET `+column+` = $1 WHERE id = $2 AND organization_id = $3`,
			int(v), id, orgID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
	}
	for field, column := range map[string]string{"is_active": "is_active", "is_free": "is_free", "is_quote_only": "is_quote_only"} {
		v, ok := body[field].(bool)
		if !ok {
			continue
		}
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE services SET `+column+` = $1 WHERE id = $2 AND organization_id = $3`,
			v, id, orgID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
	}
	if brut, ok := body["includes"]; ok {
		liste := []string{}
		if items, ok := brut.([]any); ok {
			for _, item := range items {
				if valeur := strings.TrimSpace(toText(item)); valeur != "" {
					liste = append(liste, valeur)
				}
			}
		}
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE services SET includes = $1 WHERE id = $2 AND organization_id = $3`,
			liste, id, orgID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
	}

	// Le tarif ne se modifie plus par cette route : il se pose dans la grille
	// tarifaire (motif obligatoire, audit systématique). `0` reste accepté —
	// c'est la remise « à tarifer », qui n'invente aucun prix.
	if v, ok := body["price_xof"].(float64); ok && int(v) != 0 {
		erreurs["price_xof"] = "Le tarif se modifie dans la grille tarifaire, avec son motif."
	}
	// Marquer « gratuit » efface le tarif du catalogue : deux registres
	// incompatibles, et le prix résolu doit rester sans ambiguïté.
	if gratuit, ok := body["is_free"].(bool); ok && gratuit {
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE services SET price_xof = 0 WHERE id = $1 AND organization_id = $2`, id, orgID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucune modification n'a été enregistrée : corrigez les champs signalés.", erreurs)
		return
	}

	a.addAudit(r.Context(), orgID, service.StationID, user.FullName,
		"service.updated", "service", id,
		map[string]any{"name": service.Name, "old_price_xof": service.PriceXOF, "actor_role": user.Role})

	updated, err := a.serviceAvecPrix(r.Context(), id, orgID, service.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *App) handleServiceDisable(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	id := r.PathValue("id")

	service, err := a.getOwnedService(r, id, orgID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != service.StationID {
		writeAPIError(w, errStationForbidden)
		return
	}
	// Désactivation, jamais de suppression : une fiche passée cite encore cette
	// prestation, et l'historique doit rester lisible.
	_, _ = a.Pool.Exec(r.Context(),
		`UPDATE services SET is_active = false WHERE id = $1 AND organization_id = $2`, id, orgID)
	a.addAudit(r.Context(), orgID, service.StationID, user.FullName,
		"service.disabled", "service", id,
		map[string]any{"name": service.Name, "actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"message": "Prestation désactivée."})
}
