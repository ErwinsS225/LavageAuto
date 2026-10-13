package main

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Catalogue de prestations (lecture + création).
// ---------------------------------------------------------------------------

func (a *App) handleServicesList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	includeInactive := r.URL.Query().Get("include_inactive") == "true"

	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, organization_id, station_id, name, category, description,
		       price_xof, duration_minutes, sort_order, is_active
		FROM services
		WHERE station_id = $1 AND ($2 OR is_active)
		ORDER BY sort_order, name`, stationID, includeInactive)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []Service{}
	categoriesSet := map[string]bool{}
	for rows.Next() {
		var s Service
		if err := rows.Scan(&s.ID, &s.OrganizationID, &s.StationID, &s.Name, &s.Category, &s.Description,
			&s.PriceXOF, &s.DurationMinutes, &s.SortOrder, &s.IsActive); err != nil {
			continue
		}
		items = append(items, s)
		categoriesSet[s.Category] = true
	}
	categories := []string{}
	for c := range categoriesSet {
		categories = append(categories, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "categories": categories})
}

func (a *App) handleServiceCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if isRestrictedRole(user.Role) {
		writeError(w, http.StatusForbidden, "forbidden",
			"Seul le gérant ou le propriétaire est autorisé à ajouter des prestations au catalogue.", nil)
		return
	}
	var body struct {
		StationID       string `json:"station_id"`
		Name            string `json:"name"`
		Category        string `json:"category"`
		Description     string `json:"description"`
		PriceXOF        int    `json:"price_xof"`
		DurationMinutes int    `json:"duration_minutes"`
		SortOrder       int    `json:"sort_order"`
	}
	if err := decodeBody(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Le nom de la prestation est obligatoire.", nil)
		return
	}
	stationID := body.StationID
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	category := body.Category
	if category == "" {
		category = "lavage"
	}
	duration := body.DurationMinutes
	if duration <= 0 {
		duration = 30
	}
	id := newID("srv")
	_, err := a.Pool.Exec(r.Context(), `
		INSERT INTO services (id, organization_id, station_id, name, category, description, price_xof, duration_minutes, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, deref(user.OrganizationID), stationID, body.Name, category, body.Description,
		body.PriceXOF, duration, body.SortOrder)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de la prestation.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"service.created", "service", id,
		map[string]any{"name": body.Name, "price_xof": body.PriceXOF, "actor_role": user.Role})

	var s Service
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT id, organization_id, station_id, name, category, description,
		       price_xof, duration_minutes, sort_order, is_active
		FROM services WHERE id = $1`, id,
	).Scan(&s.ID, &s.OrganizationID, &s.StationID, &s.Name, &s.Category, &s.Description,
		&s.PriceXOF, &s.DurationMinutes, &s.SortOrder, &s.IsActive)
	writeJSON(w, http.StatusCreated, s)
}

// findServiceByID charge une prestation (outil partagé).
func (a *App) findServiceByID(r *http.Request, id string) (Service, error) {
	var s Service
	err := a.Pool.QueryRow(r.Context(), `
		SELECT id, organization_id, station_id, name, category, description,
		       price_xof, duration_minutes, sort_order, is_active
		FROM services WHERE id = $1`, id,
	).Scan(&s.ID, &s.OrganizationID, &s.StationID, &s.Name, &s.Category, &s.Description,
		&s.PriceXOF, &s.DurationMinutes, &s.SortOrder, &s.IsActive)
	if err == pgx.ErrNoRows {
		return s, errNotFound
	}
	return s, err
}

var errNotFound = &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "Ressource introuvable."}
