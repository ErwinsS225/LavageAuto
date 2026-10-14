package main

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// Fiches de lavage (liste + détail).
//
// La liste est cloisonnée par entreprise, et par station quand le compte y est
// rattaché. Le reste à payer est calculé à partir des encaissements réels :
// une fiche close par un paiement partiel ne peut plus s'afficher « soldée ».
// ---------------------------------------------------------------------------

const washOrderColumns = `id, organization_id, station_id, reference,
	customer_id, customer_name, vehicle_category_id, vehicle_category_name,
	vehicle_usage_id, vehicle_usage_name,
	plate, employee_id, employee_name, status,
	subtotal_xof, discount_xof, discount_reason, total_xof, notes,
	incident_note, incident_reported_at, incident_reporter_name, cancel_reason,
	created_by, created_by_name, created_at, started_at, completed_at,
	canceled_at, business_day`

// washOrderColumnsAliased : mêmes colonnes, préfixées pour les jointures.
const washOrderColumnsAliased = `o.id, o.organization_id, o.station_id, o.reference,
	o.customer_id, o.customer_name, o.vehicle_category_id, o.vehicle_category_name,
		o.vehicle_usage_id, o.vehicle_usage_name,
	o.plate, o.employee_id, o.employee_name, o.status,
	o.subtotal_xof, o.discount_xof, o.discount_reason, o.total_xof, o.notes,
	o.incident_note, o.incident_reported_at, o.incident_reporter_name, o.cancel_reason,
	o.created_by, o.created_by_name, o.created_at, o.started_at, o.completed_at,
	o.canceled_at, o.business_day`

// scanWashOrderDest prépare les destinations de scan d'une fiche.
func scanWashOrderDest(o *WashOrder) []any {
	return []any{&o.ID, &o.OrganizationID, &o.StationID, &o.Reference,
		&o.CustomerID, &o.CustomerName, &o.VehicleCategoryID, &o.VehicleCategoryName,
		&o.VehicleUsageID, &o.VehicleUsageName,
		&o.Plate, &o.EmployeeID, &o.EmployeeName, &o.Status,
		&o.SubtotalXOF, &o.DiscountXOF, &o.DiscountReason, &o.TotalXOF, &o.Notes,
		&o.IncidentNote, &o.IncidentReportedAt, &o.IncidentReporterName, &o.CancelReason,
		&o.CreatedBy, &o.CreatedByName, &o.CreatedAt, &o.StartedAt, &o.CompletedAt,
		&o.CanceledAt, &o.BusinessDay}
}

// scanWashOrder lit une ligne de wash_orders.
func scanWashOrder(row scannable) (*WashOrder, error) {
	var o WashOrder
	if err := row.Scan(scanWashOrderDest(&o)...); err != nil {
		return nil, err
	}
	return &o, nil
}

// loadItems charge les lignes de prestation d'une fiche de l'entreprise.
//
// Le filtre sur organization_id s'ajoute à celui de la fiche : même si une
// fiche était référencée par erreur, les lignes d'une autre entreprise ne
// peuvent pas remonter.
func (a *App) loadItems(r *http.Request, orderID, orgID string) []WashOrderItem {
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, wash_order_id, service_id, service_name, unit_price_xof, quantity,
		       line_total_xof, price_source, options, options_total_xof
		FROM wash_order_items WHERE wash_order_id = $1 AND organization_id = $2 ORDER BY id`, orderID, orgID)
	if err != nil {
		return []WashOrderItem{}
	}
	defer rows.Close()
	items := []WashOrderItem{}
	for rows.Next() {
		var (
			it      WashOrderItem
			options []byte
		)
		if err := rows.Scan(&it.ID, &it.WashOrderID, &it.ServiceID, &it.ServiceName,
			&it.UnitPriceXOF, &it.Quantity, &it.LineTotalXOF, &it.PriceSource,
			&options, &it.OptionsTotalXOF); err != nil {
			continue
		}
		it.Options = []optionRetenue{}
		parseJSONB(options, &it.Options)
		it.PriceSourceLabel = libelleSourcePrix(it.PriceSource)
		items = append(items, it)
	}
	return items
}

// orderToJSON sérialise une fiche, avec les montants encaissés et restants.
func orderToJSON(o *WashOrder, paid int, items []WashOrderItem, servicesLabel string, methods []string) map[string]any {
	due := o.TotalXOF - paid
	if due < 0 {
		due = 0
	}
	if o.Status == "canceled" {
		due = 0
	}
	return map[string]any{
		"id": o.ID, "organization_id": o.OrganizationID, "station_id": o.StationID,
		"reference": o.Reference, "customer_id": o.CustomerID, "customer_name": o.CustomerName,
		"vehicle_category_id": o.VehicleCategoryID, "vehicle_category_name": o.VehicleCategoryName,
		"vehicle_usage_id": o.VehicleUsageID, "vehicle_usage_name": o.VehicleUsageName,
		"plate": o.Plate, "employee_id": o.EmployeeID, "employee_name": o.EmployeeName,
		"status": o.Status, "subtotal_xof": o.SubtotalXOF, "discount_xof": o.DiscountXOF,
		"discount_reason": o.DiscountReason, "total_xof": o.TotalXOF, "notes": o.Notes,
		"incident_note": o.IncidentNote, "incident_reported_at": formatTimePtr(o.IncidentReportedAt),
		"incident_reporter_name": o.IncidentReporterName, "cancel_reason": o.CancelReason,
		"created_by": o.CreatedBy, "created_by_name": o.CreatedByName,
		"created_at": o.CreatedAt.Format(timeLayout),
		"started_at": formatTimePtr(o.StartedAt), "completed_at": formatTimePtr(o.CompletedAt),
		"canceled_at": formatTimePtr(o.CanceledAt), "business_day": o.BusinessDay,
		"items": items, "paid_xof": paid, "due_xof": due,
		"services_label": servicesLabel, "payment_methods": methods,
	}
}

func (a *App) handleWashOrdersList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	status, search := q.Get("status"), lower(q.Get("search"))
	employeeID, customerID := q.Get("employee_id"), q.Get("customer_id")
	openOnly := q.Get("open_only") == "true"

	stationID, err := a.stationDeTravail(r.Context(), user, q.Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}

	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+washOrderColumnsAliased+`, COALESCE(p.paid_xof, 0)
		FROM wash_orders o
		LEFT JOIN LATERAL (
			SELECT sum(amount_xof) AS paid_xof FROM payments
			WHERE wash_order_id = o.id AND status = 'valid' AND method <> 'differe'
		) p ON TRUE
		WHERE o.organization_id = $1
		  AND ($2 = '' OR o.station_id = $2)
		  AND ($3 = '' OR o.business_day >= $3)
		  AND ($4 = '' OR o.business_day <= $4)
		  AND ($5 = '' OR o.status = $5)
		  AND ($6 = '' OR o.employee_id = $6)
		  AND ($7 = '' OR o.customer_id = $7)
		  AND (NOT $8 OR o.status IN ('pending','in_progress'))
		  AND ($9 = '' OR lower(o.reference) LIKE '%' || $9 || '%'
		            OR lower(o.customer_name) LIKE '%' || $9 || '%'
		            OR lower(o.plate) LIKE '%' || $9 || '%'
		            OR lower(o.employee_name) LIKE '%' || $9 || '%')
		ORDER BY o.created_at DESC
		LIMIT 200`,
		deref(user.OrganizationID), stationID, from, to, status, employeeID, customerID, openOnly, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()

	list := []map[string]any{}
	revenue, paidSum, dueSum, discountSum := 0, 0, 0, 0
	completedCount := 0
	for rows.Next() {
		var o WashOrder
		var paid int
		dest := append(scanWashOrderDest(&o), &paid)
		if err := rows.Scan(dest...); err != nil {
			continue
		}
		if o.Status == "completed" {
			revenue += o.TotalXOF
			completedCount++
		}
		if o.Status != "canceled" {
			paidSum += paid
			if rest := o.TotalXOF - paid; rest > 0 {
				dueSum += rest
			}
		}
		discountSum += o.DiscountXOF

		items := a.loadItems(r, o.ID, deref(user.OrganizationID))
		labels := []string{}
		for _, it := range items {
			labels = append(labels, it.ServiceName)
		}
		servicesLabel := "Lavage"
		if len(labels) > 0 {
			servicesLabel = joinComma(labels)
		}
		list = append(list, orderToJSON(&o, paid, items, servicesLabel, nil))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":     list,
		"total":     len(list),
		"page_size": len(list),
		"range":     map[string]any{"from": from, "to": to},
		"totals": map[string]any{
			"revenue_xof":     revenue,
			"paid_xof":        paidSum,
			"due_xof":         dueSum,
			"discount_xof":    discountSum,
			"completed_count": completedCount,
		},
	})
}
