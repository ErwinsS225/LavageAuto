package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Handlers manquants : expenses, reports, commissions, platform, washer.
// Implémentés pour compléter les routes déclarées dans main.go.
// ---------------------------------------------------------------------------

// ----------------------------- Dépenses ------------------------------------

func (a *App) handleExpensesList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	limit := queryInt(r, "limit", 100)
	rows, err := a.Pool.Query(ctx, `
SELECT id, organization_id, station_id, category, label, amount_xof, method,
       supplier, reference, note, spent_at, business_day, status, void_reason,
       recorded_by_name, created_at
FROM expenses WHERE organization_id = $1
ORDER BY spent_at DESC LIMIT $2`, deref(user.OrganizationID), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, orgID, stationID, category, label, method, supplier, reference, note, businessDay, status, recordedByName string
		var amount int
		var voidReason *string
		var spentAt, createdAt time.Time
		if err := rows.Scan(&id, &orgID, &stationID, &category, &label, &amount, &method,
			&supplier, &reference, &note, &spentAt, &businessDay, &status, &voidReason,
			&recordedByName, &createdAt); err == nil {
			list = append(list, map[string]any{
				"id": id, "organization_id": orgID, "station_id": stationID,
				"category": category, "label": label, "amount_xof": amount, "method": method,
				"supplier": supplier, "reference": reference, "note": note,
				"spent_at": spentAt, "business_day": businessDay, "status": status,
				"void_reason": voidReason, "recorded_by_name": recordedByName,
				"created_at": createdAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"expenses": list})
}

func (a *App) handleExpenseCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	var body struct {
		StationID     string `json:"station_id"`
		Category      string `json:"category"`
		Label         string `json:"label"`
		AmountXOF     int    `json:"amount_xof"`
		Method        string `json:"method"`
		Supplier      string `json:"supplier"`
		Reference     string `json:"reference"`
		Note          string `json:"note"`
		CashSessionID string `json:"cash_session_id"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if body.Label == "" || body.AmountXOF <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "Libellé et montant positif requis.", nil)
		return
	}
	stationID := body.StationID
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	if stationID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Station requise.", nil)
		return
	}
	if body.Method == "" {
		body.Method = "especes"
	}
	if body.Category == "" {
		body.Category = "autre"
	}
	id := newID("exp")
	_, err := a.Pool.Exec(ctx, `
INSERT INTO expenses (id, organization_id, station_id, category, label, amount_xof,
	method, supplier, reference, note, business_day, cash_session_id,
	recorded_by, recorded_by_name)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		id, deref(user.OrganizationID), stationID, body.Category, body.Label, body.AmountXOF,
		body.Method, body.Supplier, body.Reference, body.Note, businessDay(time.Now()),
		nullIfEmpty(body.CashSessionID), user.ID, user.FullName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement de la dépense.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"expense.created", "expense", id, map[string]any{"amount_xof": body.AmountXOF})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "valid"})
}

func (a *App) handleExpenseUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var body struct {
		Category  *string `json:"category"`
		Label     *string `json:"label"`
		AmountXOF *int    `json:"amount_xof"`
		Supplier  *string `json:"supplier"`
		Reference *string `json:"reference"`
		Note      *string `json:"note"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	_, err := a.Pool.Exec(ctx, `
UPDATE expenses SET
  category  = COALESCE($2, category),
  label     = COALESCE($3, label),
  amount_xof = COALESCE($4, amount_xof),
  supplier  = COALESCE($5, supplier),
  reference = COALESCE($6, reference),
  note      = COALESCE($7, note)
WHERE id = $1 AND organization_id = $8 AND status = 'valid'`,
		id, body.Category, body.Label, body.AmountXOF, body.Supplier, body.Reference, body.Note,
		deref(user.OrganizationID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleExpenseVoid(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &body)
	if body.Reason == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Motif d'annulation requis.", nil)
		return
	}
	_, err := a.Pool.Exec(ctx, `
UPDATE expenses SET status = 'voided', void_reason = $2
WHERE id = $1 AND organization_id = $3 AND status = 'valid'`,
		id, body.Reason, deref(user.OrganizationID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"expense.voided", "expense", id, map[string]any{"reason": body.Reason})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------- Rapports ------------------------------------

func (a *App) handleReportDashboard(w http.ResponseWriter, r *http.Request) {
	a.reportOverviewCommon(w, r, true)
}

func (a *App) handleReportOverview(w http.ResponseWriter, r *http.Request) {
	a.reportOverviewCommon(w, r, false)
}

func (a *App) reportOverviewCommon(w http.ResponseWriter, r *http.Request, dashboard bool) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	today := businessDay(time.Now())

	var washCount, canceledCount, serviceCount, revenue, discounts, payments, cash, mobileMoney, other, expenses, expensesCash, voidedPayments int
	_ = a.Pool.QueryRow(ctx, `
SELECT
	count(*) FILTER (WHERE status = 'completed'),
	count(*) FILTER (WHERE status = 'canceled'),
	count(DISTINCT o.id) FILTER (WHERE o.status = 'completed'),
	COALESCE(sum(o.total_xof) FILTER (WHERE o.status = 'completed'), 0),
	COALESCE(sum(o.discount_xof) FILTER (WHERE o.status = 'completed'), 0),
	COALESCE(sum(p.amount_xof) FILTER (WHERE p.status = 'valid'), 0),
	COALESCE(sum(CASE WHEN p.method = 'especes' THEN p.amount_xof ELSE 0 END) FILTER (WHERE p.status = 'valid'), 0),
	COALESCE(sum(CASE WHEN p.method = 'mobile_money' THEN p.amount_xof ELSE 0 END) FILTER (WHERE p.status = 'valid'), 0),
	COALESCE(sum(CASE WHEN p.method = 'autre' THEN p.amount_xof ELSE 0 END) FILTER (WHERE p.status = 'valid'), 0),
	COALESCE(sum(e.amount_xof) FILTER (WHERE e.status = 'valid'), 0),
	COALESCE(sum(CASE WHEN e.method = 'especes' THEN e.amount_xof ELSE 0 END) FILTER (WHERE e.status = 'valid'), 0),
	COALESCE(sum(p.amount_xof) FILTER (WHERE p.status = 'voided'), 0)
FROM wash_orders o
LEFT JOIN payments p ON p.wash_order_id = o.id AND p.business_day = $2
LEFT JOIN expenses e ON e.organization_id = o.organization_id AND e.business_day = $2 AND e.status = 'valid'
WHERE o.organization_id = $1 AND o.business_day = $2
  AND ($3 = '' OR o.station_id = $3)
  AND o.status <> 'canceled'`, orgID, today, stationID).Scan(
		&washCount, &canceledCount, &serviceCount,
		&revenue, &discounts,
		&payments, &cash, &mobileMoney, &other,
		&expenses, &expensesCash,
		&voidedPayments)

	todayData := map[string]any{
		"wash_count":           washCount,
		"canceled_count":       canceledCount,
		"service_count":        serviceCount,
		"revenue_xof":          revenue,
		"discount_xof":         discounts,
		"payments_xof":         payments,
		"cash_xof":             cash,
		"mobile_money_xof":     mobileMoney,
		"other_xof":            other,
		"expenses_xof":         expenses,
		"expenses_cash_xof":    expensesCash,
		"result_xof":           revenue - expenses,
		"cash_result_xof":      cash - expensesCash,
		"due_xof":              0,
		"average_ticket_xof":   0,
		"voided_payments_xof":  voidedPayments,
	}

	series := []map[string]any{}
	rows, err := a.Pool.Query(ctx, `
SELECT d.bd,
       count(*) FILTER (WHERE o.status = 'completed'),
       COALESCE(sum(o.total_xof) FILTER (WHERE o.status = 'completed'), 0),
       COALESCE(sum(p.amount_xof) FILTER (WHERE p.status = 'valid'), 0),
       COALESCE(sum(e.amount_xof) FILTER (WHERE e.status = 'valid'), 0)
FROM (SELECT generate_series($1::date, $1::date - interval '6 days', interval '1 day')::date AS bd) AS d
LEFT JOIN wash_orders o ON o.business_day = d.bd AND o.organization_id = $1
                         AND ($2 = '' OR o.station_id = $2)
LEFT JOIN payments p ON p.business_day = d.bd AND p.organization_id = $1
LEFT JOIN expenses e ON e.business_day = d.bd AND e.organization_id = $1 AND e.status = 'valid'
GROUP BY d.bd ORDER BY d.bd`, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var day string
			var wc, rev, pay, exp int
			if rows.Scan(&day, &wc, &rev, &pay, &exp) == nil {
				series = append(series, map[string]any{
					"day":          day,
					"wash_count":   wc,
					"revenue_xof":  rev,
					"payments_xof": pay,
					"expenses_xof": exp,
					"result_xof":   rev - exp,
				})
			}
		}
	}

	cashSession := map[string]any{}
	var isOpen bool
	var floatX, expX, cntX, diffX, cashIn, mobX, otherX, expX2, ops, payCnt, expCnt int
	var day string
	var openedByName string
	var openedAt time.Time
		row := a.Pool.QueryRow(ctx, `
SELECT is_open, opening_float_xof, expected_cash_xof, counted_cash_xof,
       difference_xof, cash_in_xof, mobile_money_xof, other_in_xof,
       expenses_xof, operations, payment_count, expense_count, business_day,
       station_id, opened_by_name, opened_at
FROM cash_sessions
WHERE station_id = $1 AND status = 'open'
ORDER BY opened_at DESC LIMIT 1`, stationID)
	if err == nil {
		if err := row.Scan(&isOpen, &floatX, &expX, &cntX, &diffX, &cashIn, &mobX, &otherX,
			&expX2, &ops, &payCnt, &expCnt, &day, &stationID, &openedByName, &openedAt); err == nil {
			cashSession = map[string]any{
				"is_open":            isOpen,
				"opening_float_xof":  floatX,
				"expected_cash_xof":  expX,
				"counted_cash_xof":   cntX,
				"difference_xof":     diffX,
				"cash_in_xof":        cashIn,
				"mobile_money_xof":   mobX,
				"other_in_xof":       otherX,
				"expenses_xof":       expX2,
				"operations":         ops,
				"payment_count":      payCnt,
				"expense_count":      expCnt,
				"business_day":       day,
				"station_id":         stationID,
				"opened_by_name":     openedByName,
				"opened_at":          openedAt.Format(time.RFC3339),
			}
		}
	}

	// --------------------------------- EMPLOYÉS -----------------------------------
	empRows, err := a.Pool.Query(ctx, `
SELECT u.id, u.full_name, count(*) FILTER (WHERE o.status = 'completed'),
       COALESCE(sum(o.total_xof) FILTER (WHERE o.status = 'completed'), 0),
       COALESCE(sum(p.amount_xof) FILTER (WHERE p.status = 'valid'), 0)
FROM users u
LEFT JOIN wash_orders o ON o.employee_id = u.id
                       AND o.organization_id = $1
                       AND o.business_day = $2
                       AND ($3 = '' OR o.station_id = $3)
LEFT JOIN payments p ON p.wash_order_id = o.id AND p.status = 'valid'
WHERE u.organization_id = $1
  AND ($3 = '' OR u.station_id = $3)
GROUP BY u.id, u.full_name
HAVING count(*) > 0 OR $4`, orgID, today, stationID, true)
	employees := []map[string]any{}
	if err == nil {
		defer empRows.Close()
		for empRows.Next() {
			var id, name string
			var wc, rev, coll int
			if empRows.Scan(&id, &name, &wc, &rev, &coll) == nil {
				employees = append(employees, map[string]any{
					"employee_id":   id,
					"employee_name": name,
					"wash_count":    wc,
					"revenue_xof":   rev,
					"collected_xof": coll,
				})
			}
		}
	}

	// ------------------------------ ACTIVITÉ RÉCENTE ------------------------------
	recent := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
SELECT 'wash' AS type, o.reference, o.total_xof, o.created_at, u.full_name, o.status, o.reference
FROM wash_orders o LEFT JOIN users u ON u.id = o.employee_id
WHERE o.organization_id = $1 AND o.business_day = $2
  AND ($3 = '' OR o.station_id = $3)
ORDER BY o.created_at DESC LIMIT 10
UNION ALL
SELECT 'payment' AS type, o.reference, p.amount_xof, p.paid_at, u.full_name, p.status, p.reference
FROM payments p LEFT JOIN wash_orders o ON o.id = p.wash_order_id
                  LEFT JOIN users u ON u.id = p.received_by
WHERE p.organization_id = $1 AND p.business_day = $2
  AND ($3 = '' OR p.station_id = $3)
  AND p.status = 'valid'
ORDER BY paid_at DESC LIMIT 10`, orgID, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var typ, ref string
			var amt int
			var at time.Time
			var actor, status, note string
			if rows.Scan(&typ, &ref, &amt, &at, &actor, &status, &note) == nil {
				recent = append(recent, map[string]any{
					"type":       typ,
					"label":      ref,
					"amount_xof": amt,
					"at":         at.Format(time.RFC3339),
					"actor":      actor,
					"status":     status,
					"reference":  note,
				})
			}
		}
	}

	// -------------------------- MÉTHODES DE PAIEMENT -----------------------------
	paymentMethods := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
SELECT method, sum(amount_xof), count(*)
FROM payments
WHERE organization_id = $1 AND business_day = $2
  AND ($3 = '' OR station_id = $3)
  AND status = 'valid'
GROUP BY method`, orgID, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var method string
			var amt, cnt int
			if rows.Scan(&method, &amt, &cnt) == nil {
				paymentMethods = append(paymentMethods, map[string]any{
					"method":      method,
					"amount_xof":  amt,
					"count":       cnt,
				})
			}
		}
	}

	// ------------------------------- OBJECTIF ------------------------------------
	goal := interface{}(nil)
	rows, err = a.Pool.Query(ctx, `
SELECT month, target_xof, achieved_xof, remaining_xof, percent, days_left, reached
FROM monthly_goals
WHERE organization_id = $1 AND month = to_char(current_date, 'YYYY-MM')
LIMIT 1`, orgID)
	if err == nil {
		var m string
		var tgt, ach, rem, pct, days int
		var reached bool
		goalRows := rows
		for goalRows.Next() {
			if goalRows.Scan(&m, &tgt, &ach, &rem, &pct, &days, &reached) == nil {
				goal = map[string]any{
					"month":          m,
					"target_xof":     tgt,
					"achieved_xof":   ach,
					"remaining_xof":  rem,
					"percent":        pct,
					"days_left":      days,
					"reached":        reached,
				}
			}
		}
		goalRows.Close()
	}

	// --------------------------------- ALERTES ----------------------------------
	alerts := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
SELECT 'warning' AS level, 'Aucun paiement encaissé aujourd\'hui' AS message
WHERE NOT EXISTS (SELECT 1 FROM payments WHERE organization_id = $1 AND business_day = $2 AND status = 'valid')`, orgID, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var level, message string
			if rows.Scan(&level, &message) == nil {
				alerts = append(alerts, map[string]any{"level": level, "message": message})
			}
		}
	}

	// ----------------------------- TOP SERVICES (7 premiers) ----------------------
	topServices := []map[string]any{}
	if dashboard {
		rows, err = a.Pool.Query(ctx, `
SELECT service_name, sum(quantity), sum(line_total_xof)
FROM wash_order_items i JOIN wash_orders o ON o.id = i.wash_order_id
WHERE o.organization_id = $1 AND o.business_day = $2 AND o.status <> 'cancelled'
GROUP BY service_name ORDER BY 3 DESC LIMIT 5`, orgID, today)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				var qty, total int
				if rows.Scan(&name, &qty, &total) == nil {
					topServices = append(topServices, map[string]any{
						"service_name": name,
						"quantity":     qty,
						"revenue_xof":  total,
					})
				}
			}
		}
	}

	// ------------------------------- DEFINITIONPOINTS ----------------------------------
	definitions := map[string]any{}
	rows, err = a.Pool.Query(ctx, `SELECT name, label FROM services`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, label string
			if rows.Scan(&name, &label) == nil {
				definitions[name] = label
			}
		}
	}

	// ----------------------------------- PAYLOAD ----------------------------------
	var cashPayload interface{}
	if dashboard {
		if isOpen {
			cashPayload = cashSession
		}
	}
	payload := map[string]any{
		"date":                    today,
		"station_id":              stationID,
		"today":                   todayData,
		"series":                  series,
		"cash":                    cashPayload,
		"employees":               employees,
		"recent":                  recent,
		"top_services":            topServices,
		"payment_methods":         paymentMethods,
		"alerts":                  alerts,
		"goal":                    goal,
		"definitions":             definitions,
	}
	writeJSON(w, http.StatusOK, payload)
}
func (a *App) handleCommissions(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	rows, err := a.Pool.Query(ctx, `
SELECT u.id, u.full_name, u.commission_mode,
       COALESCE(u.commission_per_wash_xof,0), COALESCE(u.commission_percent,0),
       count(o.id) FILTER (WHERE o.employee_id = u.id AND o.status = 'completed')
FROM users u LEFT JOIN wash_orders o ON o.employee_id = u.id
WHERE u.organization_id = $1 AND u.role = 'washer'
GROUP BY u.id, u.full_name, u.commission_mode, u.commission_per_wash_xof, u.commission_percent
ORDER BY u.full_name`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, fullName, mode string
		var perWash, washCount int
		var percent float64
		if rows.Scan(&id, &fullName, &mode, &perWash, &percent, &washCount) == nil {
			amount := 0
			switch mode {
			case "per_wash":
				amount = perWash * washCount
			case "percent":
				// Le pourcentage s'applique au CA attribué (calculé ci-dessous).
				var ca int
				_ = a.Pool.QueryRow(ctx, `
SELECT COALESCE(sum(total_xof),0) FROM wash_orders
WHERE employee_id = $1 AND status = 'completed'`, id).Scan(&ca)
				amount = int(float64(ca) * percent / 100)
			}
			list = append(list, map[string]any{
				"employee_id": id, "full_name": fullName, "commission_mode": mode,
				"commission_per_wash_xof": perWash, "commission_percent": percent,
				"completed_washes": washCount, "commission_xof": amount,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"commissions": list})
}

func (a *App) handleCommissionsExport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=commissions.csv")
	_, _ = fmt.Fprintln(w, "employe;mode;laveages_completes;commission_xof")
	user := currentUser(r)
	ctx := r.Context()
	rows, err := a.Pool.Query(ctx, `
SELECT u.full_name, u.commission_mode, count(o.id),
       COALESCE(u.commission_per_wash_xof,0) * count(o.id)
FROM users u LEFT JOIN wash_orders o ON o.employee_id = u.id AND o.status = 'completed'
WHERE u.organization_id = $1 AND u.role = 'washer'
GROUP BY u.full_name, u.commission_mode, u.commission_per_wash_xof
ORDER BY u.full_name`, deref(user.OrganizationID))
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var fullName, mode string
		var count, amount int
		if rows.Scan(&fullName, &mode, &count, &amount) == nil {
			_, _ = fmt.Fprintf(w, "%s;%s;%d;%d\n", fullName, mode, count, amount)
		}
	}
}

// ----------------------------- Plateforme ---------------------------------

func (a *App) requireSuperadmin(w http.ResponseWriter, r *http.Request) (*User, bool) {
	user := currentUser(r)
	if user == nil || user.Role != "superadmin" {
		writeError(w, http.StatusForbidden, "forbidden", "Accès réservé à la plateforme.", nil)
		return nil, false
	}
	return user, true
}

func (a *App) handlePlatformOverview(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	ctx := r.Context()
	var orgCount, userCount, orderCount, revenue int
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&orgCount)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM wash_orders`).Scan(&orderCount)
	_ = a.Pool.QueryRow(ctx, `SELECT COALESCE(sum(total_xof),0) FROM wash_orders WHERE status <> 'cancelled'`).Scan(&revenue)
	writeJSON(w, http.StatusOK, map[string]any{
		"organizations": orgCount, "users": userCount,
		"wash_orders": orderCount, "revenue_xof": revenue,
	})
}

func (a *App) handlePlatformOrgsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	ctx := r.Context()
	rows, err := a.Pool.Query(ctx, `
SELECT o.id, o.name, o.slug, o.is_active, o.created_at,
       count(u.id) AS users, COALESCE(sum(s.price_xof),0)
FROM organizations o
LEFT JOIN users u ON u.organization_id = o.id
WHERE ($1 = '' OR lower(o.name) LIKE lower('%' || $1 || '%'))
GROUP BY o.id ORDER BY o.created_at DESC`, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, name, slug string
		var isActive bool
		var createdAt time.Time
		var users, revenue int
		if rows.Scan(&id, &name, &slug, &isActive, &createdAt, &users, &revenue) == nil {
			list = append(list, map[string]any{
				"id": id, "name": name, "slug": slug, "is_active": isActive,
				"created_at": createdAt, "users": users,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizations": list})
}

func (a *App) handlePlatformOrgDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	org, err := a.getOrganization(r.Context(), r.PathValue("id"))
	if err != nil || org == nil {
		writeError(w, http.StatusNotFound, "not_found", "Organisation introuvable.", nil)
		return
	}
	writeJSON(w, http.StatusOK, org)
}

func (a *App) handlePlatformOrgUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name     *string `json:"name"`
		IsActive *bool   `json:"is_active"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	_, err := a.Pool.Exec(r.Context(), `
UPDATE organizations SET name = COALESCE($2, name) WHERE id = $1`, id, body.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handlePlatformOrgSubscription(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		PlanID string `json:"plan_id"`
	}
	if err := decodeBody(r, &body); err != nil || body.PlanID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "plan_id requis.", nil)
		return
	}
	_, err := a.Pool.Exec(r.Context(), `
INSERT INTO subscriptions (id, organization_id, plan_id, status)
VALUES ($1,$2,$3,'active')
ON CONFLICT (organization_id) DO UPDATE SET plan_id = EXCLUDED.plan_id, status = 'active'`,
		newID("sub"), id, body.PlanID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handlePlatformPlansList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	ctx := r.Context()

	// Nombre d'organisations abonnées par formule (via leurs abonnements).
	counts := map[string]int{}
	rows, err := a.Pool.Query(ctx, `
		SELECT plan_id, count(DISTINCT organization_id) FROM subscriptions
		WHERE plan_id IS NOT NULL GROUP BY plan_id`)
	if err == nil {
		for rows.Next() {
			var planID string
			var n int
			if rows.Scan(&planID, &n) == nil {
				counts[planID] = n
			}
		}
		rows.Close()
	}

	// Même forme que /plans, enrichie du nombre d'organisations.
	items := []map[string]any{}
	for _, p := range a.listPlans(r) {
		p["organization_count"] = counts[p["id"].(string)]
		items = append(items, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (a *App) handlePlatformPlanUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	user := currentUser(r)
	var body struct {
		Name             *string `json:"name"`
		Description      *string `json:"description"`
		PriceXOF         *int    `json:"price_xof"`
		MaxStations      *int    `json:"max_stations"`
		MaxEmployees     *int    `json:"max_employees"`
		MaxWashOrders    *int    `json:"max_wash_orders_month"`
		TrialDays        *int    `json:"trial_days"`
		Features         *map[string]any `json:"features"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	// Colonne => valeur modifiable.
	type col struct {
		name  string
		value any
	}
	cols := []col{
		{"name", body.Name},
		{"description", body.Description},
		{"price_xof", body.PriceXOF},
		{"max_stations", body.MaxStations},
		{"max_employees", body.MaxEmployees},
		{"max_wash_orders_month", body.MaxWashOrders},
		{"trial_days", body.TrialDays},
	}
	setClauses := []string{}
	args := []any{id}
	for _, c := range cols {
		if c.value == nil {
			continue
		}
		args = append(args, c.value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", c.name, len(args)))
	}
	if body.Features != nil {
		args = append(args, jsonOrNull(*body.Features))
		setClauses = append(setClauses, fmt.Sprintf("features = $%d::jsonb", len(args)))
	}
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "Aucun champ à mettre à jour.", nil)
		return
	}

	_, err := a.Pool.Exec(r.Context(),
		`UPDATE plans SET `+strings.Join(setClauses, ", ")+` WHERE id = $1`, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour de la formule.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), "", user.FullName,
		"plan.updated", "plan", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Formule mise à jour."})
}

func (a *App) handlePlatformAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	ctx := r.Context()
	limit := queryInt(r, "limit", 100)
	rows, err := a.Pool.Query(ctx, `
SELECT id, organization_id, station_id, actor_name, action, entity, entity_id, details, created_at
FROM audit_logs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id int64
		var orgID, stationID, actor, action, entity, entityID string
		var details []byte
		var createdAt time.Time
		if rows.Scan(&id, &orgID, &stationID, &actor, &action, &entity, &entityID, &details, &createdAt) == nil {
			d := map[string]any{}
			_ = json.Unmarshal(details, &d)
			list = append(list, map[string]any{
				"id": id, "organization_id": orgID, "station_id": stationID,
				"actor_name": actor, "action": action, "entity": entity,
				"entity_id": entityID, "details": d, "created_at": createdAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit": list})
}


// ----------------------------- Export CSV ------------------------------------
func (a *App) handleReportExport(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		today := businessDay(time.Now())
		from, to = today, today
	}
	rows, err := a.Pool.Query(ctx, `
SELECT reference, customer_name, vehicle_name, plate, employee_name, status,
       subtotal_xof, discount_xof, total_xof, business_day
FROM wash_orders
WHERE organization_id = $1 AND business_day >= $2 AND business_day <= $3
ORDER BY business_day DESC, reference`, orgID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=washguard_%s_%s.csv", from, to))
	_, _ = fmt.Fprintln(w, "reference;client;vehicule;plaque;employe;statut;sous_total;remise;total;jour")
	for rows.Next() {
		var reference, customerName, vehicleName, plate, employeeName, status, businessDay string
		var subtotal, discount, total int
		if rows.Scan(&reference, &customerName, &vehicleName, &plate, &employeeName, &status,
			&subtotal, &discount, &total, &businessDay) == nil {
			_, _ = fmt.Fprintf(w, "%s;%s;%s;%s;%s;%s;%d;%d;%d;%s\n",
				reference, customerName, vehicleName, plate, employeeName, status,
				subtotal, discount, total, businessDay)
		}
	}
}


// ----------------------------- Espace laveur -------------------------------

func (a *App) handleWasherTasks(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	rows, err := a.Pool.Query(ctx, `
SELECT id, reference, customer_name, vehicle_name, plate, status, notes, business_day, total_xof
FROM wash_orders
WHERE employee_id = $1 AND status IN ('assigned','in_progress')
ORDER BY created_at DESC`, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, reference, customerName, vehicleName, plate, status, notes, businessDay string
		var total int
		if rows.Scan(&id, &reference, &customerName, &vehicleName, &plate, &status, &notes, &businessDay, &total) == nil {
			list = append(list, map[string]any{
				"id": id, "reference": reference, "customer_name": customerName,
				"vehicle_name": vehicleName, "plate": plate, "status": status,
				"notes": notes, "business_day": businessDay, "total_xof": total,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": list})
}

func (a *App) handleWasherTaskStatus(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var body struct {
		Status string `json:"status"`
	}
	if err := decodeBody(r, &body); err != nil || body.Status == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Statut requis.", nil)
		return
	}
	valid := map[string]bool{"in_progress": true, "completed": true}
	if !valid[body.Status] {
		writeError(w, http.StatusBadRequest, "validation_error", "Statut non autorisé.", nil)
		return
	}
	_, err := a.Pool.Exec(ctx, `
UPDATE wash_orders SET status = $2, updated_at = now()
WHERE id = $1 AND employee_id = $3 AND status IN ('assigned','in_progress')`,
		id, body.Status, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"washer.task_status", "wash_order", id, map[string]any{"status": body.Status})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleWasherTaskIncident(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var body struct {
		Description string `json:"description"`
	}
	if err := decodeBody(r, &body); err != nil || body.Description == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Description requise.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"washer.incident_reported", "wash_order", id,
		map[string]any{"description": body.Description})
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (a *App) handleWasherPerformance(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	today := businessDay(time.Now())
	var todayCount, totalCount int
	_ = a.Pool.QueryRow(ctx, `
SELECT count(*) FROM wash_orders WHERE employee_id = $1 AND business_day = $2 AND status = 'completed'`,
		user.ID, today).Scan(&todayCount)
	_ = a.Pool.QueryRow(ctx, `
SELECT count(*) FROM wash_orders WHERE employee_id = $1 AND status = 'completed'`,
		user.ID).Scan(&totalCount)
	writeJSON(w, http.StatusOK, map[string]any{
		"today_completed": todayCount, "total_completed": totalCount,
	})
}

// ----------------------------- Utilitaires ---------------------------------

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
