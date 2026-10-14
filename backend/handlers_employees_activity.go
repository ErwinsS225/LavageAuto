package main

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Employés : activité détaillée (réservée aux profils qui gèrent l'équipe).
// ---------------------------------------------------------------------------

func (a *App) handleEmployeeActivity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user := currentUser(r)
	orgID := deref(user.OrganizationID)

	var fullName, email, phone, role string
	var organizationID, stationID *string
	var isActive bool
	var createdAt time.Time
	err := a.Pool.QueryRow(r.Context(), `
		SELECT id, organization_id, station_id, full_name, email, phone, role, is_active, created_at
		FROM users WHERE id = $1 AND organization_id = $2`, id, orgID,
	).Scan(&id, &organizationID, &stationID, &fullName, &email, &phone, &role, &isActive, &createdAt)
	if err == pgx.ErrNoRows {
		writeError(w, http.StatusNotFound, "not_found", "Employé introuvable.", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && deref(stationID) != forced {
		writeError(w, http.StatusNotFound, "not_found", "Employé introuvable.", nil)
		return
	}

	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if to == "" {
		to = businessDay(time.Now())
	}
	if from == "" {
		from = to
	}

	var washCount, completedCount, canceledCount, revenue, discount int
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'completed'),
		       count(*) FILTER (WHERE status = 'canceled'),
		       COALESCE(sum(total_xof) FILTER (WHERE status = 'completed'), 0),
		       COALESCE(sum(discount_xof), 0)
		FROM wash_orders
		WHERE employee_id = $1 AND organization_id = $2
		  AND business_day >= $3 AND business_day <= $4`, id, orgID, from, to,
	).Scan(&washCount, &completedCount, &canceledCount, &revenue, &discount)

	rows, _ := a.Pool.Query(r.Context(), `
		SELECT id, reference, created_at, status, customer_name, plate,
		       vehicle_category_name, total_xof, discount_xof,
		       COALESCE((SELECT sum(amount_xof) FROM payments p
		                  WHERE p.wash_order_id = o.id AND p.status = 'valid'
		                    AND p.method <> 'differe'), 0) AS paid
		FROM wash_orders o
		WHERE employee_id = $1 AND organization_id = $2
		  AND business_day >= $3 AND business_day <= $4
		ORDER BY created_at DESC LIMIT 200`, id, orgID, from, to)
	orders := []map[string]any{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var oID, ref, status, cust, plate, vcat string
			var oCreated time.Time
			var total, disc, paid int
			if err := rows.Scan(&oID, &ref, &oCreated, &status, &cust, &plate, &vcat, &total, &disc, &paid); err != nil {
				continue
			}
			// Le reste à payer est calculé, jamais supposé : un encaissement
			// partiel laisse une fiche ouverte.
			due := 0
			if status != "canceled" && paid < total {
				due = total - paid
			}
			orders = append(orders, map[string]any{
				"id": oID, "reference": ref, "created_at": oCreated.Format(timeLayout),
				"status": status, "customer_name": cust, "plate": plate,
				"vehicle_category_name": vcat, "total_xof": total, "discount_xof": disc,
				"paid_xof": paid, "due_xof": due,
			})
		}
	}

	collected := 0
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT COALESCE(sum(amount_xof),0) FROM payments
		WHERE received_by = $1 AND organization_id = $2 AND status = 'valid'
		  AND business_day >= $3 AND business_day <= $4`, id, orgID, from, to).Scan(&collected)

	writeJSON(w, http.StatusOK, map[string]any{
		"employee": map[string]any{
			"id": id, "organization_id": organizationID, "station_id": stationID,
			"full_name": fullName, "email": email, "phone": phone, "role": role,
			"role_label": roleLabelFR(role),
			"is_active":  isActive, "created_at": createdAt.Format(timeLayout),
		},
		"range": map[string]any{"from": from, "to": to},
		"totals": map[string]any{
			"wash_count": washCount, "completed_count": completedCount,
			"canceled_count": canceledCount, "revenue_xof": revenue,
			"collected_xof": collected, "discount_xof": discount,
		},
		"orders": orders,
	})
}
