package main

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Catalogue de prestations (mise à jour + désactivation).
// ---------------------------------------------------------------------------

func (a *App) handleServiceUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if isRestrictedRole(user.Role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Seul le gérant ou le propriétaire est autorisé à modifier les tarifs du catalogue.", nil)
		return
	}
	id := r.PathValue("id")
	var oldPrice int
	err := a.Pool.QueryRow(r.Context(), `SELECT price_xof FROM services WHERE id = $1`, id).Scan(&oldPrice)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Prestation introuvable.", nil)
		return
	}
	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	allowed := map[string]string{
		"name": "name", "category": "category", "description": "description",
		"sort_order": "sort_order", "is_active": "is_active",
	}
	for field, column := range allowed {
		if v, ok := body[field]; ok {
			_, _ = a.Pool.Exec(r.Context(), `UPDATE services SET `+column+` = $1 WHERE id = $2`, v, id)
		}
	}
	if v, ok := body["price_xof"].(float64); ok {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE services SET price_xof = $1 WHERE id = $2`, int(v), id)
	}
	if v, ok := body["duration_minutes"].(float64); ok {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE services SET duration_minutes = $1 WHERE id = $2`, int(v), id)
	}
	var name string
	_ = a.Pool.QueryRow(r.Context(), `SELECT name FROM services WHERE id = $1`, id).Scan(&name)
	a.addAudit(r.Context(), deref(user.OrganizationID), "", user.FullName,
		"service.updated", "service", id,
		map[string]any{"name": name, "old_price_xof": oldPrice, "actor_role": user.Role})

	var s Service
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT id, organization_id, station_id, name, category, description,
		       price_xof, duration_minutes, sort_order, is_active
		FROM services WHERE id = $1`, id,
	).Scan(&s.ID, &s.OrganizationID, &s.StationID, &s.Name, &s.Category, &s.Description,
		&s.PriceXOF, &s.DurationMinutes, &s.SortOrder, &s.IsActive)
	writeJSON(w, http.StatusOK, s)
}

func (a *App) handleServiceDisable(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if isRestrictedRole(user.Role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Seul le gérant ou le propriétaire est autorisé à désactiver une prestation du catalogue.", nil)
		return
	}
	id := r.PathValue("id")
	var name string
	err := a.Pool.QueryRow(r.Context(), `SELECT name FROM services WHERE id = $1`, id).Scan(&name)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"message": "Prestation désactivée."})
		return
	}
	_, _ = a.Pool.Exec(r.Context(), `UPDATE services SET is_active = false WHERE id = $1`, id)
	a.addAudit(r.Context(), deref(user.OrganizationID), "", user.FullName,
		"service.disabled", "service", id,
		map[string]any{"name": name, "actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"message": "Prestation désactivée."})
}
