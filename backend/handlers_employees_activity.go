package main

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// handleEmployeeActivity détaille l'activité d'un employé sur 30 jours.
func (a *App) handleEmployeeActivity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var fullName, email, phone, role string
	var organizationID, stationID *string
	var isActive bool
	var createdAt time.Time
	err := a.Pool.QueryRow(r.Context(), `
		SELECT id, organization_id, station_id, full_name, email, phone, role, is_active, created_at
		FROM users WHERE id = $1`, id,
	).Scan(&id, &organizationID, &stationID, &fullName, &email, &phone, &role, &isActive, &createdAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Employé introuvable.", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	var washCount, completedCount, canceledCount, revenue, discount int
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'completed'),
		       count(*) FILTER (WHERE status = 'canceled'),
		       COALESCE(sum(total_xof) FILTER (WHERE status = 'completed'), 0),
		       COALESCE(sum(discount_xof), 0)
		FROM wash_orders WHERE employee_id = $1`, id,
	).Scan(&washCount, &completedCount, &canceledCount, &revenue, &discount)

	rows, _ := a.Pool.Query(r.Context(), `
		SELECT id, reference, created_at, status, customer_name, plate,
		       vehicle_category_name, total_xof, discount_xof
		FROM wash_orders WHERE employee_id = $1 ORDER BY created_at DESC LIMIT 50`, id)
	orders := []map[string]any{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var oID, ref, status, cust, plate, vcat string
			var oCreated time.Time
			var total, disc int
			if err := rows.Scan(&oID, &ref, &oCreated, &status, &cust, &plate, &vcat, &total, &disc); err != nil {
				continue
			}
			paid, due := 0, 0
			if status == "completed" {
				paid = total
			} else {
				due = total
			}
			orders = append(orders, map[string]any{
				"id": oID, "reference": ref, "created_at": oCreated.Format(time.RFC3339),
				"status": status, "customer_name": cust, "plate": plate,
				"vehicle_category_name": vcat, "total_xof": total, "discount_xof": disc,
				"paid_xof": paid, "due_xof": due,
			})
		}
	}

	to := businessDay(time.Now())
	writeJSON(w, http.StatusOK, map[string]any{
		"employee": map[string]any{
			"id": id, "organization_id": organizationID, "station_id": stationID,
			"full_name": fullName, "email": email, "phone": phone, "role": role,
			"is_active": isActive, "created_at": createdAt.Format(time.RFC3339),
		},
		"range": map[string]any{"from": "", "to": to},
		"totals": map[string]any{
			"wash_count": washCount, "completed_count": completedCount,
			"canceled_count": canceledCount, "revenue_xof": revenue,
			"collected_xof": revenue, "discount_xof": discount,
		},
		"orders": orders,
	})
}
