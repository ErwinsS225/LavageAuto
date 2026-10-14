package main

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Clients (détail, mise à jour, archivage).
//
// Toutes les requêtes sont filtrées par `organization_id` : un identifiant
// d'une autre entreprise est introuvable (404), jamais modifiable.
// ---------------------------------------------------------------------------

const customerColumns = `id, organization_id, station_id, full_name, phone, plate, vehicle_model,
	notes, is_active, created_at`

// getOwnedCustomer charge un client en vérifiant l'entreprise et, pour un
// compte rattaché à une station, le périmètre de station.
func (a *App) getOwnedCustomer(r *http.Request, id string) (*Customer, error) {
	user := currentUser(r)
	var c Customer
	var createdAt time.Time
	err := a.Pool.QueryRow(r.Context(), `
		SELECT `+customerColumns+`
		FROM customers WHERE id = $1 AND organization_id = $2`, id, deref(user.OrganizationID),
	).Scan(&c.ID, &c.OrganizationID, &c.StationID, &c.FullName, &c.Phone, &c.Plate,
		&c.VehicleModel, &c.Notes, &c.IsActive, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	if forced := scopedStationID(user); forced != "" && forced != c.StationID {
		return nil, errNotFound
	}
	c.CreatedAt = createdAt
	return &c, nil
}

func (a *App) handleCustomerDetail(w http.ResponseWriter, r *http.Request) {
	c, err := a.getOwnedCustomer(r, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
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
			} else if status != "canceled" {
				due = total
			}
			history = append(history, map[string]any{
				"id": oID, "reference": ref, "created_at": oCreatedAt.Format(timeLayout),
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
			"created_at": c.CreatedAt.Format(timeLayout),
			"wash_count": c.WashCount, "total_spent_xof": c.TotalSpentXOF,
			"last_visit_at": c.LastVisitAt,
		},
		"history": history,
	})
}

func (a *App) handleCustomerUpdate(w http.ResponseWriter, r *http.Request) {
	c, err := a.getOwnedCustomer(r, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	user := currentUser(r)
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
			_, _ = a.Pool.Exec(r.Context(),
				`UPDATE customers SET `+column+` = $1 WHERE id = $2 AND organization_id = $3`,
				v, c.ID, c.OrganizationID)
		}
	}
	a.addAudit(r.Context(), c.OrganizationID, c.StationID, user.FullName,
		"customer.updated", "customer", c.ID, nil)

	updated, err := a.getOwnedCustomer(r, c.ID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": updated.ID, "full_name": updated.FullName, "phone": updated.Phone,
		"plate": updated.Plate, "vehicle_model": updated.VehicleModel,
		"notes": updated.Notes, "is_active": updated.IsActive,
	})
}

func (a *App) handleCustomerArchive(w http.ResponseWriter, r *http.Request) {
	c, err := a.getOwnedCustomer(r, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	user := currentUser(r)
	_, _ = a.Pool.Exec(r.Context(),
		`UPDATE customers SET is_active = false WHERE id = $1 AND organization_id = $2`,
		c.ID, c.OrganizationID)
	a.addAudit(r.Context(), c.OrganizationID, c.StationID, user.FullName,
		"customer.archived", "customer", c.ID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Client archivé."})
}
