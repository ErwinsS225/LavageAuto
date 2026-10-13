package main

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Stations et catégories de véhicules.
// ---------------------------------------------------------------------------

func scanStation(rows pgx.Rows) (*Station, error) {
	var s Station
	var openingRaw, settingsRaw []byte
	var createdAt time.Time
	err := rows.Scan(&s.ID, &s.OrganizationID, &s.Name, &s.Code, &s.Phone, &s.Address, &s.City,
		&s.Timezone, &s.Currency, &s.LogoURL, &openingRaw, &settingsRaw, &s.IsDefault, &s.IsActive, &createdAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt = createdAt
	s.OpeningHours = map[string]any{}
	s.Settings = map[string]any{}
	parseJSONB(openingRaw, &s.OpeningHours)
	parseJSONB(settingsRaw, &s.Settings)
	return &s, nil
}

func (a *App) handleStationsList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, organization_id, name, code, phone, address, city, timezone, currency,
		       logo_url, opening_hours, settings, is_default, is_active, created_at
		FROM stations WHERE organization_id = $1 ORDER BY is_default DESC, name`, user.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		st, err := scanStation(rows)
		if err != nil {
			continue
		}
		items = append(items, stationToJSON(st))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (a *App) handleStationCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		Name     string `json:"name"`
		Code     string `json:"code"`
		Phone    string `json:"phone"`
		Address  string `json:"address"`
		City     string `json:"city"`
		Timezone string `json:"timezone"`
	}
	if err := decodeBody(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Le nom de la station est obligatoire.", nil)
		return
	}
	code := body.Code
	if code == "" {
		code = defaultStationCode(body.Name)
	}
	city := body.City
	if city == "" {
		city = "Abidjan"
	}
	tz := body.Timezone
	if tz == "" {
		tz = "Africa/Abidjan"
	}
	id := newID("st")
	_, err := a.Pool.Exec(r.Context(), `
		INSERT INTO stations (id, organization_id, name, code, phone, address, city, timezone, settings)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`,
		id, deref(user.OrganizationID), body.Name, code, body.Phone, body.Address, city, tz,
		`{"commission_mode":"per_wash","commission_per_wash_xof":500,"monthly_goal_xof":1000000}`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de la station.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), id, user.FullName, "station.created", "station", id, nil)
	st, _ := a.getStation(r.Context(), id)
	writeJSON(w, http.StatusCreated, stationToJSON(st))
}

func (a *App) handleStationUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	st, err := a.getStation(r.Context(), id)
	if err != nil || st == nil || st.OrganizationID != deref(user.OrganizationID) {
		writeError(w, http.StatusNotFound, "not_found", "Station introuvable.", nil)
		return
	}
	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	allowed := map[string]string{
		"name": "name", "code": "code", "phone": "phone", "address": "address",
		"city": "city", "logo_url": "logo_url",
	}
	for field, column := range allowed {
		if v, ok := body[field]; ok {
			_, _ = a.Pool.Exec(r.Context(), `UPDATE stations SET `+column+` = $1 WHERE id = $2`, v, id)
		}
	}
	if v, ok := body["settings"].(map[string]any); ok {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE stations SET settings = $1::jsonb WHERE id = $2`, jsonOrNull(v), id)
	}
	if v, ok := body["opening_hours"].(map[string]any); ok {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE stations SET opening_hours = $1::jsonb WHERE id = $2`, jsonOrNull(v), id)
	}
	if v, ok := body["is_active"].(bool); ok {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE stations SET is_active = $1 WHERE id = $2`, v, id)
	}
	a.addAudit(r.Context(), st.OrganizationID, id, user.FullName, "station.updated", "station", id, nil)
	updated, _ := a.getStation(r.Context(), id)
	writeJSON(w, http.StatusOK, stationToJSON(updated))
}

func (a *App) handleVehicleCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, name, description, sort_order, is_active
		FROM vehicle_categories WHERE is_active ORDER BY sort_order`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var vc VehicleCategory
		if err := rows.Scan(&vc.ID, &vc.Name, &vc.Description, &vc.SortOrder, &vc.IsActive); err != nil {
			continue
		}
		items = append(items, map[string]any{
			"id": vc.ID, "name": vc.Name, "description": vc.Description,
			"sort_order": vc.SortOrder, "is_active": vc.IsActive,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// scanAuditRows lit les lignes d'audit (réutilisé par plusieurs handlers).
func scanAuditRows(rows pgx.Rows) []map[string]any {
	items := []map[string]any{}
	for rows.Next() {
		var e AuditEntry
		var detailsRaw []byte
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.StationID, &e.ActorName,
			&e.Action, &e.Entity, &e.EntityID, &detailsRaw, &e.CreatedAt); err != nil {
			continue
		}
		e.Details = map[string]any{}
		parseJSONB(detailsRaw, &e.Details)
		items = append(items, map[string]any{
			"id": e.ID, "organization_id": e.OrganizationID, "station_id": e.StationID,
			"actor_name": e.ActorName, "action": e.Action, "entity": e.Entity,
			"entity_id": e.EntityID, "details": e.Details,
			"created_at": e.CreatedAt.Format(time.RFC3339),
		})
	}
	return items
}
