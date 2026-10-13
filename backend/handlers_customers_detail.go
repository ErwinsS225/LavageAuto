package main

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Clients (détail, mise à jour, archivage).
// ---------------------------------------------------------------------------

func (a *App) handleCustomerDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var c Customer
	var createdAt time.Time
	err := a.Pool.QueryRow(r.Context(), `
		SELECT id, organization_id, station_id, full_name, phone, plate, vehicle_model,
		       notes, is_active, created_at
		FROM customers WHERE id = $1`, id,
	).Scan(&c.ID, &c.OrganizationID, &c.StationID, &c.FullName, &c.Phone, &c.Plate,
		&c.VehicleModel, &c.Notes, &c.IsActive, &createdAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Client introuvable.", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	c.CreatedAt = createdAt
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT count(*), COALESCE(sum(total_xof),0), max(created_at)
		FROM wash_orders WHERE customer_id = $1 AND status <> 'canceled'`, c.ID,
	).Scan(&c.WashCount, &c.TotalSpentXOF, &c.LastVisitAt)

	rows, _ := a.Pool.Query(r.Context(), `
		SELECT id, reference, created_at, status, total_xof, vehicle_category_name, plate
		FROM wash_orders WHERE customer_id = $1 ORDER BY created_at DESC LIMIT 100`, c.ID)
	history := []map[string]any{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var oID, ref, status, vcat, plate string
			var oCreatedAt time.Time
			var total int
			if err := rows.Scan(&oID, &ref, &oCreatedAt, &status, &total, &vcat, &plate); err != nil {
				continue
			}
			paid, due := 0, 0
			if status == "completed" {
				paid = total
			} else {
				due = total
			}
			history = append(history, map[string]any{
				"id": oID, "reference": ref, "created_at": oCreatedAt.Format(time.RFC3339),
				"status": status, "total_xof": total, "paid_xof": paid, "due_xof": due,
				"vehicle_category_name": vcat, "plate": plate,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"customer": map[string]any{
			"id": c.ID, "organization_id": c.OrganizationID, "station_id": c.StationID,
			"full_name": c.FullName, "phone": c.Phone, "plate": c.Plate,
			"vehicle_model": c.VehicleModel, "notes": c.Notes, "is_active": c.IsActive,
			"created_at": c.CreatedAt.Format(time.RFC3339),
			"wash_count": c.WashCount, "total_spent_xof": c.TotalSpentXOF,
			"last_visit_at": c.LastVisitAt,
		},
		"history": history,
	})
}

func (a *App) handleCustomerUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	allowed := map[string]string{
		"full_name": "full_name", "phone": "phone", "plate": "plate",
		"vehicle_model": "vehicle_model", "notes": "notes", "is_active": "is_active",
	}
	for field, column := range allowed {
		if v, ok := body[field]; ok {
			_, _ = a.Pool.Exec(r.Context(), `UPDATE customers SET `+column+` = $1 WHERE id = $2`, v, id)
		}
	}
	var fullName, phone, plate, vehicleModel, notes string
	var isActive bool
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT full_name, phone, plate, vehicle_model, notes, is_active
		FROM customers WHERE id = $1`, id,
	).Scan(&fullName, &phone, &plate, &vehicleModel, &notes, &isActive)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "full_name": fullName, "phone": phone, "plate": plate,
		"vehicle_model": vehicleModel, "notes": notes, "is_active": isActive,
	})
}

func (a *App) handleCustomerArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, _ = a.Pool.Exec(r.Context(), `UPDATE customers SET is_active = false WHERE id = $1`, id)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Client archivé."})
}
