package main

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Fiches de lavage (liste + détail).
// ---------------------------------------------------------------------------

// scanWashOrder lit une ligne de wash_orders.
func scanWashOrder(row scannable) (*WashOrder, error) {
	var o WashOrder
	err := row.Scan(&o.ID, &o.OrganizationID, &o.StationID, &o.Reference,
		&o.CustomerID, &o.CustomerName, &o.VehicleCategoryID, &o.VehicleCategoryName,
		&o.Plate, &o.EmployeeID, &o.EmployeeName, &o.Status,
		&o.SubtotalXOF, &o.DiscountXOF, &o.DiscountReason, &o.TotalXOF, &o.Notes,
		&o.IncidentNote, &o.IncidentReportedAt, &o.IncidentReporterName, &o.CancelReason,
		&o.CreatedBy, &o.CreatedByName, &o.CreatedAt, &o.StartedAt, &o.CompletedAt,
		&o.CanceledAt, &o.BusinessDay)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// loadItems charge les lignes de prestation d'une fiche.
func (a *App) loadItems(r *http.Request, orderID string) []WashOrderItem {
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, wash_order_id, service_id, service_name, unit_price_xof, quantity, line_total_xof
		FROM wash_order_items WHERE wash_order_id = $1 ORDER BY id`, orderID)
	if err != nil {
		return []WashOrderItem{}
	}
	defer rows.Close()
	items := []WashOrderItem{}
	for rows.Next() {
		var it WashOrderItem
		if err := rows.Scan(&it.ID, &it.WashOrderID, &it.ServiceID, &it.ServiceName,
			&it.UnitPriceXOF, &it.Quantity, &it.LineTotalXOF); err != nil {
			continue
		}
		items = append(items, it)
	}
	return items
}

const washOrderColumns = `id, organization_id, station_id, reference,
	customer_id, customer_name, vehicle_category_id, vehicle_category_name,
	plate, employee_id, employee_name, status,
	subtotal_xof, discount_xof, discount_reason, total_xof, notes,
	incident_note, incident_reported_at, incident_reporter_name, cancel_reason,
	created_by, created_by_name, created_at, started_at, completed_at,
	canceled_at, business_day`

func (a *App) handleWashOrdersList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	status, search := q.Get("status"), lower(q.Get("search"))
	stationID := q.Get("station_id")
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	employeeID, customerID := q.Get("employee_id"), q.Get("customer_id")
	openOnly := q.Get("open_only") == "true"

	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+washOrderColumns+`
		FROM wash_orders
		WHERE station_id = $1
		  AND ($2 = '' OR business_day >= $2)
		  AND ($3 = '' OR business_day <= $3)
		  AND ($4 = '' OR status = $4)
		  AND ($5 = '' OR employee_id = $5)
		  AND ($6 = '' OR customer_id = $6)
		  AND (NOT $7 OR status IN ('pending','in_progress'))
		  AND ($8 = '' OR lower(reference) LIKE '%' || $8 || '%'
		            OR lower(customer_name) LIKE '%' || $8 || '%'
		            OR lower(plate) LIKE '%' || $8 || '%'
		            OR lower(employee_name) LIKE '%' || $8 || '%')
		ORDER BY created_at DESC
		LIMIT 200`,
		stationID, from, to, status, employeeID, customerID, openOnly, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()

	list := []map[string]any{}
	revenue, due, discountSum := 0, 0, 0
	completedCount := 0
	for rows.Next() {
		o, err := scanWashOrder(rows)
		if err != nil {
			continue
		}
		if o.Status == "completed" {
			revenue += o.TotalXOF
			completedCount++
		}
		if o.Status == "pending" || o.Status == "in_progress" {
			due += o.TotalXOF
		}
		discountSum += o.DiscountXOF

		items := a.loadItems(r, o.ID)
		labels := []string{}
		for _, it := range items {
			labels = append(labels, it.ServiceName)
		}
		servicesLabel := "Lavage"
		if len(labels) > 0 {
			servicesLabel = joinComma(labels)
		}
		paid, owe := 0, 0
		if o.Status == "completed" {
			paid = o.TotalXOF
		} else {
			owe = o.TotalXOF
		}
		list = append(list, map[string]any{
			"id": o.ID, "organization_id": o.OrganizationID, "station_id": o.StationID,
			"reference": o.Reference, "customer_id": o.CustomerID, "customer_name": o.CustomerName,
			"vehicle_category_id": o.VehicleCategoryID, "vehicle_category_name": o.VehicleCategoryName,
			"plate": o.Plate, "employee_id": o.EmployeeID, "employee_name": o.EmployeeName,
			"status": o.Status, "subtotal_xof": o.SubtotalXOF, "discount_xof": o.DiscountXOF,
			"discount_reason": o.DiscountReason, "total_xof": o.TotalXOF, "notes": o.Notes,
			"incident_note": o.IncidentNote, "incident_reported_at": o.IncidentReportedAt,
			"incident_reporter_name": o.IncidentReporterName, "cancel_reason": o.CancelReason,
			"created_by": o.CreatedBy, "created_by_name": o.CreatedByName,
			"created_at": o.CreatedAt.Format(time.RFC3339),
			"started_at": formatTimePtr(o.StartedAt), "completed_at": formatTimePtr(o.CompletedAt),
			"canceled_at": formatTimePtr(o.CanceledAt), "business_day": o.BusinessDay,
			"items":    items,
			"paid_xof": paid, "due_xof": owe,
			"payment_count":  boolToInt(o.Status == "completed"),
			"services_label": servicesLabel,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":     list,
		"total":     len(list),
		"page_size": len(list),
		"range":     map[string]any{"from": from, "to": to},
		"totals": map[string]any{
			"revenue_xof": revenue, "paid_xof": revenue,
			"due_xof": due, "discount_xof": discountSum,
		},
	})
}
