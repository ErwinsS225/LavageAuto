package main

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Clients (liste + création).
// ---------------------------------------------------------------------------

func (a *App) handleCustomersList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	search := lower(r.URL.Query().Get("search"))
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}

	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, organization_id, station_id, full_name, phone, plate, vehicle_model,
		       notes, is_active, created_at
		FROM customers
		WHERE organization_id = $1
		  AND ($2 = 'all' OR ($2 = 'active' AND is_active) OR ($2 = 'inactive' AND NOT is_active))
		  AND ($3 = '' OR lower(full_name) LIKE '%' || $3 || '%'
		            OR lower(phone) LIKE '%' || $3 || '%'
		            OR lower(plate) LIKE '%' || $3 || '%')
		ORDER BY created_at DESC`,
		user.OrganizationID, status, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var c Customer
		var createdAt time.Time
		if err := rows.Scan(&c.ID, &c.OrganizationID, &c.StationID, &c.FullName, &c.Phone, &c.Plate,
			&c.VehicleModel, &c.Notes, &c.IsActive, &createdAt); err != nil {
			continue
		}
		c.CreatedAt = createdAt
		_ = a.Pool.QueryRow(r.Context(), `
			SELECT count(*), COALESCE(sum(total_xof),0), max(created_at)
			FROM wash_orders WHERE customer_id = $1 AND status <> 'canceled'`, c.ID,
		).Scan(&c.WashCount, &c.TotalSpentXOF, &c.LastVisitAt)

		items = append(items, map[string]any{
			"id": c.ID, "organization_id": c.OrganizationID, "station_id": c.StationID,
			"full_name": c.FullName, "phone": c.Phone, "plate": c.Plate,
			"vehicle_model": c.VehicleModel, "notes": c.Notes, "is_active": c.IsActive,
			"created_at":      c.CreatedAt.Format(time.RFC3339),
			"wash_count":      c.WashCount, "total_spent_xof": c.TotalSpentXOF,
			"last_visit_at": c.LastVisitAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "page_size": len(items)})
}

func (a *App) handleCustomerCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		StationID    string `json:"station_id"`
		FullName     string `json:"full_name"`
		Phone        string `json:"phone"`
		Plate        string `json:"plate"`
		VehicleModel string `json:"vehicle_model"`
		Notes        string `json:"notes"`
	}
	if err := decodeBody(r, &body); err != nil || body.FullName == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Le nom du client est obligatoire.", nil)
		return
	}
	stationID := body.StationID
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	id := newID("cust")
	_, err := a.Pool.Exec(r.Context(), `
		INSERT INTO customers (id, organization_id, station_id, full_name, phone, plate, vehicle_model, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		id, deref(user.OrganizationID), stationID, body.FullName, body.Phone, body.Plate, body.VehicleModel, body.Notes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création du client.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName, "customer.created", "customer", id, nil)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "organization_id": deref(user.OrganizationID), "station_id": stationID,
		"full_name": body.FullName, "phone": body.Phone, "plate": body.Plate,
		"vehicle_model": body.VehicleModel, "notes": body.Notes, "is_active": true,
		"wash_count": 0, "total_spent_xof": 0, "last_visit_at": nil,
	})
}

// lower helper pour la recherche insensible à la casse.
func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
