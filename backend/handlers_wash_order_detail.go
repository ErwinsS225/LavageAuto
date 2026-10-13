package main

import (
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Fiches de lavage (détail).
// ---------------------------------------------------------------------------

func (a *App) handleWashOrderDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	row := a.Pool.QueryRow(r.Context(), `
		SELECT `+washOrderColumns+` FROM wash_orders WHERE id = $1`, id)
	o, err := scanWashOrder(row)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche de lavage introuvable.", nil)
		return
	}

	// Paiements rattachés.
	prows, _ := a.Pool.Query(r.Context(), `
		SELECT id, method, amount_xof, reference, note, paid_at, received_by_name, status, void_reason
		FROM payments WHERE wash_order_id = $1 ORDER BY paid_at`, id)
	payments := []map[string]any{}
	methods := []string{}
	if prows != nil {
		defer prows.Close()
		for prows.Next() {
			var pID, method, reference, note, receivedBy, status string
			var amount int
			var paidAt time.Time
			var voidReason *string
			if err := prows.Scan(&pID, &method, &amount, &reference, &note, &paidAt, &receivedBy, &status, &voidReason); err != nil {
				continue
			}
			payments = append(payments, map[string]any{
				"id": pID, "method": method, "amount_xof": amount,
				"reference": reference, "note": note,
				"paid_at": paidAt.Format(time.RFC3339),
				"received_by_name": receivedBy, "status": status, "void_reason": voidReason,
			})
			if status == "valid" {
				methods = append(methods, method)
			}
		}
	}

	items := a.loadItems(r, o.ID)
	labels := []string{}
	for _, it := range items {
		labels = append(labels, it.ServiceName)
	}
	servicesLabel := "Lavage"
	if len(labels) > 0 {
		servicesLabel = joinComma(labels)
	}
	paid, due := 0, 0
	if o.Status == "completed" {
		paid = o.TotalXOF
	} else {
		due = o.TotalXOF
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"order": map[string]any{
			"id": o.ID, "organization_id": o.OrganizationID, "station_id": o.StationID,
			"reference": o.Reference, "customer_id": o.CustomerID, "customer_name": o.CustomerName,
			"vehicle_category_id": o.VehicleCategoryID, "vehicle_category_name": o.VehicleCategoryName,
			"plate": o.Plate, "employee_id": o.EmployeeID, "employee_name": o.EmployeeName,
			"status": o.Status, "subtotal_xof": o.SubtotalXOF, "discount_xof": o.DiscountXOF,
			"discount_reason": o.DiscountReason, "total_xof": o.TotalXOF, "notes": o.Notes,
			"incident_note": o.IncidentNote, "incident_reported_at": formatTimePtr(o.IncidentReportedAt),
			"incident_reporter_name": o.IncidentReporterName, "cancel_reason": o.CancelReason,
			"created_by": o.CreatedBy, "created_by_name": o.CreatedByName,
			"created_at": o.CreatedAt.Format(time.RFC3339),
			"started_at": formatTimePtr(o.StartedAt), "completed_at": formatTimePtr(o.CompletedAt),
			"canceled_at": formatTimePtr(o.CanceledAt), "business_day": o.BusinessDay,
			"items": items, "paid_xof": paid, "due_xof": due,
			"services_label": servicesLabel, "payment_methods": methods,
		},
		"payments": payments,
	})
}

// formatTimePtr formate un timestamp nullable en RFC3339 ou null.
func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// joinComma relie des chaînes par une virgule.
func joinComma(parts []string) string {
	return strings.Join(parts, ", ")
}
