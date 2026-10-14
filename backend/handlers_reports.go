package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Rapports : tableau de bord du jour, synthèse sur période, commissions,
// exports CSV.
//
// Chaque agrégat est calculé par une requête indépendante : l'ancien code
// joignait `wash_orders`, `payments` et `expenses` dans un même SELECT, ce qui
// multipliait les montants (produit cartésien) dès qu'une journée contenait
// plusieurs encaissements ou dépenses.
// ---------------------------------------------------------------------------

// periodTotals regroupe les indicateurs d'une période.
type periodTotals struct {
	WashCount, CanceledCount, ServiceCount           int
	Revenue, Discount, Payments                      int
	Cash, MobileMoney, Other, Expenses, ExpensesCash int
	Result, CashResult, Due, AverageTicket           int
	VoidedPayments                                   int
	// Deferred : engagements de paiement enregistrés sur la période. Ils ne
	// sont **pas** des encaissements et ne s'ajoutent à aucun autre total.
	Deferred int
}

// scope : filtre de station partagé par toutes les requêtes d'un rapport.
// Une chaîne vide signifie « toute l'entreprise ».
func (t periodTotals) toMap() map[string]any {
	return map[string]any{
		"wash_count": t.WashCount, "canceled_count": t.CanceledCount, "service_count": t.ServiceCount,
		"revenue_xof": t.Revenue, "discount_xof": t.Discount,
		"payments_xof": t.Payments, "cash_xof": t.Cash, "mobile_money_xof": t.MobileMoney,
		"other_xof": t.Other, "expenses_xof": t.Expenses, "expenses_cash_xof": t.ExpensesCash,
		"result_xof": t.Result, "cash_result_xof": t.CashResult, "due_xof": t.Due,
		"average_ticket_xof": t.AverageTicket, "voided_payments_xof": t.VoidedPayments,
		"deferred_xof": t.Deferred,
	}
}

// totalsForPeriod calcule les indicateurs consolidés.
func (a *App) totalsForPeriod(ctx context.Context, orgID, stationID, from, to string) periodTotals {
	var t periodTotals
	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'completed'),
		       count(*) FILTER (WHERE status = 'canceled'),
		       COALESCE(sum(total_xof) FILTER (WHERE status = 'completed'), 0),
		       COALESCE(sum(discount_xof) FILTER (WHERE status = 'completed'), 0)
		FROM wash_orders
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4`,
		orgID, stationID, from, to,
	).Scan(&t.WashCount, &t.CanceledCount, &t.Revenue, &t.Discount)

	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*)
		FROM wash_order_items i
		JOIN wash_orders o ON o.id = i.wash_order_id
		WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
		  AND o.business_day >= $3 AND o.business_day <= $4 AND o.status = 'completed'`,
		orgID, stationID, from, to).Scan(&t.ServiceCount)

	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_xof) FILTER (WHERE status = 'valid' AND method <> 'differe'), 0),
		       COALESCE(sum(amount_xof) FILTER (WHERE status = 'valid' AND method = 'especes'), 0),
		       COALESCE(sum(amount_xof) FILTER (WHERE status = 'valid' AND method = 'mobile_money'), 0),
		       COALESCE(sum(amount_xof) FILTER (WHERE status = 'valid'
		                                           AND method NOT IN ('especes','mobile_money','differe')), 0),
		       COALESCE(sum(amount_xof) FILTER (WHERE status = 'valid' AND method = 'differe'), 0),
		       COALESCE(sum(amount_xof) FILTER (WHERE status = 'voided'), 0)
		FROM payments
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4`,
		orgID, stationID, from, to,
	).Scan(&t.Payments, &t.Cash, &t.MobileMoney, &t.Other, &t.Deferred, &t.VoidedPayments)

	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_xof) FILTER (WHERE status = 'valid'), 0),
		       COALESCE(sum(amount_xof) FILTER (WHERE status = 'valid' AND method = 'especes'), 0)
		FROM expenses
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4`,
		orgID, stationID, from, to).Scan(&t.Expenses, &t.ExpensesCash)

	// Reste à encaisser : fiches ouvertes dont le solde n'est pas couvert.
	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(GREATEST(o.total_xof - COALESCE(p.paid, 0), 0)), 0)
		FROM wash_orders o
		LEFT JOIN LATERAL (
			SELECT sum(amount_xof) AS paid FROM payments
			WHERE wash_order_id = o.id AND status = 'valid' AND method <> 'differe'
		) p ON TRUE
		WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
		  AND o.business_day >= $3 AND o.business_day <= $4
		  AND o.status IN ('pending','in_progress')`,
		orgID, stationID, from, to).Scan(&t.Due)

	t.Result = t.Revenue - t.Expenses
	t.CashResult = t.Cash - t.ExpensesCash
	if t.WashCount > 0 {
		t.AverageTicket = t.Revenue / t.WashCount
	}
	return t
}

// seriesForPeriod : une ligne par jour, sans jointure multiplicative.
func (a *App) seriesForPeriod(ctx context.Context, orgID, stationID, from, to string) []map[string]any {
	rows, err := a.Pool.Query(ctx, `
		SELECT d.bd::text,
		       (SELECT count(*) FROM wash_orders o
		         WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
		           AND o.business_day = d.bd::text AND o.status = 'completed'),
		       (SELECT COALESCE(sum(total_xof), 0) FROM wash_orders o
		         WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
		           AND o.business_day = d.bd::text AND o.status = 'completed'),
		       (SELECT COALESCE(sum(amount_xof), 0) FROM payments p
		         WHERE p.organization_id = $1 AND ($2 = '' OR p.station_id = $2)
		           AND p.business_day = d.bd::text AND p.status = 'valid'
		           AND p.method <> 'differe'),
		       (SELECT COALESCE(sum(amount_xof), 0) FROM expenses e
		         WHERE e.organization_id = $1 AND ($2 = '' OR e.station_id = $2)
		           AND e.business_day = d.bd::text AND e.status = 'valid')
		FROM generate_series($3::date, $4::date, interval '1 day') AS d(bd)
		ORDER BY d.bd`, orgID, stationID, from, to)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	series := []map[string]any{}
	for rows.Next() {
		var day string
		var wc, rev, pay, exp int
		if rows.Scan(&day, &wc, &rev, &pay, &exp) == nil {
			series = append(series, map[string]any{
				"day": day, "wash_count": wc, "revenue_xof": rev,
				"payments_xof": pay, "expenses_xof": exp, "result_xof": rev - exp,
			})
		}
	}
	return series
}

// openCashSessionOf : session ouverte d'une station (ou nil).
func (a *App) openCashSessionOf(ctx context.Context, stationID string) map[string]any {
	if stationID == "" {
		return nil
	}
	var isOpen bool
	var floatX, expX, countedX, diffX, cashIn, mobX, otherX, expSpent, ops, payCnt, expCnt int
	var day, openedByName, sessionID string
	var openedAt time.Time
	err := a.Pool.QueryRow(ctx, `
		SELECT id, TRUE, opening_float_xof, expected_cash_xof, COALESCE(counted_cash_xof, 0),
		       COALESCE(difference_xof, 0), cash_in_xof, mobile_money_xof, other_in_xof,
		       expenses_xof, operations, payment_count, expense_count, business_day,
		       opened_by_name, opened_at
		FROM cash_sessions
		WHERE station_id = $1 AND status = 'open'
		ORDER BY opened_at DESC LIMIT 1`, stationID,
	).Scan(&sessionID, &isOpen, &floatX, &expX, &countedX, &diffX, &cashIn, &mobX, &otherX,
		&expSpent, &ops, &payCnt, &expCnt, &day, &openedByName, &openedAt)
	if err != nil {
		return nil
	}
	return map[string]any{
		"is_open": isOpen, "session_id": sessionID,
		"opening_float_xof": floatX, "expected_cash_xof": expX,
		"counted_cash_xof": countedX, "difference_xof": diffX,
		"cash_in_xof": cashIn, "mobile_money_xof": mobX, "other_in_xof": otherX,
		"expenses_xof": expSpent, "operations": ops,
		"payment_count": payCnt, "expense_count": expCnt,
		"business_day": day, "station_id": stationID,
		"opened_by_name": openedByName, "opened_at": openedAt.Format(timeLayout),
	}
}

// goalForStation : objectif du mois, calculé à partir du réalisé réel.
func (a *App) goalForStation(ctx context.Context, orgID, stationID string) any {
	if stationID == "" {
		return nil
	}
	month := time.Now().Format("2006-01")
	first := month + "-01"
	today := businessDay(time.Now())

	var target int
	if err := a.Pool.QueryRow(ctx, `
		SELECT target_xof FROM monthly_goals
		WHERE station_id = $1 AND month = $2`, stationID, month).Scan(&target); err != nil || target <= 0 {
		return nil
	}
	achieved := 0
	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(total_xof), 0) FROM wash_orders
		WHERE organization_id = $1 AND station_id = $2
		  AND business_day >= $3 AND business_day <= $4 AND status = 'completed'`,
		orgID, stationID, first, today).Scan(&achieved)

	remaining := target - achieved
	if remaining < 0 {
		remaining = 0
	}
	percent := 0
	if target > 0 {
		percent = achieved * 100 / target
	}
	// Jours restants dans le mois, aujourd'hui inclus.
	now := time.Now()
	lastDay := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
	daysLeft := lastDay - now.Day() + 1
	goal := map[string]any{
		"month": month, "target_xof": target, "achieved_xof": achieved,
		"remaining_xof": remaining, "percent": percent, "days_left": daysLeft,
		"reached": achieved >= target,
	}
	if remaining > 0 && daysLeft > 0 {
		goal["needed_per_day_xof"] = (remaining + daysLeft - 1) / daysLeft
	}
	return goal
}

// GET /reports/dashboard — tableau de bord du jour (7 derniers jours en série).
func (a *App) handleReportDashboard(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	stationID, err := a.stationDeTravail(ctx, user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	today := businessDay(time.Now())
	weekAgo, _ := time.Parse("2006-01-02", today)
	from := weekAgo.AddDate(0, 0, -6).Format("2006-01-02")

	totals := a.totalsForPeriod(ctx, orgID, stationID, today, today)
	cash := a.openCashSessionOf(ctx, stationID)

	// Employés du jour : lavages terminés, CA attribué, encaissements reçus.
	employees := []map[string]any{}
	rows, err := a.Pool.Query(ctx, `
		SELECT u.id, u.full_name,
		       (SELECT count(*) FROM wash_orders o
		         WHERE o.employee_id = u.id AND o.organization_id = $1
		           AND o.business_day = $2 AND o.status = 'completed') AS washes,
		       (SELECT COALESCE(sum(o.total_xof), 0) FROM wash_orders o
		         WHERE o.employee_id = u.id AND o.organization_id = $1
		           AND o.business_day = $2 AND o.status = 'completed') AS revenue,
		       (SELECT COALESCE(sum(p.amount_xof), 0) FROM payments p
		         WHERE p.received_by = u.id AND p.organization_id = $1
		           AND p.business_day = $2 AND p.status = 'valid'
		           AND p.method <> 'differe') AS collected
		FROM users u
		WHERE u.organization_id = $1 AND u.is_active
		  AND ($3 = '' OR u.station_id = $3)
		ORDER BY washes DESC, u.full_name
		LIMIT 20`, orgID, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name string
			var washes, revenue, collected int
			if rows.Scan(&id, &name, &washes, &revenue, &collected) == nil {
				employees = append(employees, map[string]any{
					"employee_id": id, "employee_name": name,
					"wash_count": washes, "revenue_xof": revenue, "collected_xof": collected,
				})
			}
		}
	}

	// Activité récente : fiches, encaissements et dépenses du jour.
	recent := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT 'wash' AS type, o.reference AS label, o.total_xof, o.created_at,
		       COALESCE(u.full_name, ''), o.status, o.reference
		FROM wash_orders o LEFT JOIN users u ON u.id = o.employee_id
		WHERE o.organization_id = $1 AND o.business_day = $2
		  AND ($3 = '' OR o.station_id = $3)
		UNION ALL
		SELECT 'payment', COALESCE(o.reference, p.reference), p.amount_xof, p.paid_at,
		       p.received_by_name, p.status, p.reference
		FROM payments p LEFT JOIN wash_orders o ON o.id = p.wash_order_id
		WHERE p.organization_id = $1 AND p.business_day = $2
		  AND ($3 = '' OR p.station_id = $3) AND p.status = 'valid'
		  AND p.method <> 'differe'
		UNION ALL
		SELECT 'expense', e.label, e.amount_xof, e.spent_at, e.recorded_by_name, e.status, e.reference
		FROM expenses e
		WHERE e.organization_id = $1 AND e.business_day = $2
		  AND ($3 = '' OR e.station_id = $3) AND e.status = 'valid'
		ORDER BY 4 DESC LIMIT 12`, orgID, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var typ, label, actor, status, ref string
			var amount int
			var at time.Time
			if rows.Scan(&typ, &label, &amount, &at, &actor, &status, &ref) == nil {
				recent = append(recent, map[string]any{
					"type": typ, "label": label, "amount_xof": amount,
					"at": at.Format(timeLayout), "actor": actor, "status": status, "reference": ref,
				})
			}
		}
	}

	// Prestations les plus vendues du jour.
	topServices := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT i.service_name, sum(i.quantity), sum(i.line_total_xof)
		FROM wash_order_items i JOIN wash_orders o ON o.id = i.wash_order_id
		WHERE o.organization_id = $1 AND o.business_day = $2 AND o.status = 'completed'
		  AND ($3 = '' OR o.station_id = $3)
		GROUP BY i.service_name ORDER BY 3 DESC LIMIT 5`, orgID, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var qty, total int
			if rows.Scan(&name, &qty, &total) == nil {
				topServices = append(topServices, map[string]any{
					"service_name": name, "quantity": qty, "revenue_xof": total,
				})
			}
		}
	}

	paymentMethods := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT method, sum(amount_xof), count(*)
		FROM payments
		WHERE organization_id = $1 AND business_day = $2 AND status = 'valid'
		  AND method <> 'differe'
		  AND ($3 = '' OR station_id = $3)
		GROUP BY method ORDER BY 2 DESC`, orgID, today, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var method string
			var amount, count int
			if rows.Scan(&method, &amount, &count) == nil {
				paymentMethods = append(paymentMethods, map[string]any{
					"method": method, "method_label": methodLabelFR(method),
					"amount_xof": amount, "count": count,
				})
			}
		}
	}

	// Alertes : ce qui mérite un coup d'œil ce matin.
	alerts := []map[string]any{}
	if totals.Payments == 0 && totals.WashCount > 0 {
		alerts = append(alerts, map[string]any{
			"level": "warning", "message": "Aucun encaissement enregistré aujourd'hui alors que des lavages sont terminés.",
		})
	}
	if totals.Payments == 0 && totals.WashCount == 0 {
		alerts = append(alerts, map[string]any{
			"level": "info", "message": "Aucune activité enregistrée pour l'instant aujourd'hui.",
		})
	}
	if totals.Due > 0 {
		alerts = append(alerts, map[string]any{
			"level": "warning", "message": fmt.Sprintf("Reste à encaisser sur des fiches ouvertes : %d FCFA.", totals.Due),
		})
	}
	if cash == nil {
		alerts = append(alerts, map[string]any{
			"level": "info", "message": "Aucune session de caisse n'est ouverte.",
		})
	} else if diffX, ok := cash["difference_xof"].(int); ok && diffX != 0 {
		alerts = append(alerts, map[string]any{
			"level": "warning", "message": "La caisse présente un écart : vérifiez le comptage.",
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"date":            today,
		"station_id":      stationID,
		"today":           totals.toMap(),
		"series":          a.seriesForPeriod(ctx, orgID, stationID, from, today),
		"cash":            cash,
		"employees":       employees,
		"recent":          recent,
		"top_services":    topServices,
		"payment_methods": paymentMethods,
		"alerts":          alerts,
		"goal":            a.goalForStation(ctx, orgID, stationID),
		"definitions":     definitionsRapport(),
	})
}

// GET /reports/overview — synthèse sur une période (par défaut le mois).
func (a *App) handleReportOverview(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	q := r.URL.Query()

	stationID, err := a.stationDeTravail(ctx, user, q.Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	today := businessDay(time.Now())
	from, to := q.Get("from"), q.Get("to")
	if to == "" {
		to = today
	}
	if from == "" {
		from = today[:8] + "01" // premier jour du mois courant
	}
	if from > to {
		from, to = to, from
	}

	stationName := ""
	if stationID != "" {
		var name string
		if err := a.Pool.QueryRow(ctx, `SELECT name FROM stations WHERE id = $1`, stationID).Scan(&name); err == nil {
			stationName = name
		}
	}

	series := a.seriesForPeriod(ctx, orgID, stationID, from, to)
	days := len(series)

	byService := []map[string]any{}
	rows, err := a.Pool.Query(ctx, `
		SELECT i.service_name, sum(i.quantity), sum(i.line_total_xof)
		FROM wash_order_items i JOIN wash_orders o ON o.id = i.wash_order_id
		WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
		  AND o.business_day >= $3 AND o.business_day <= $4 AND o.status = 'completed'
		GROUP BY i.service_name ORDER BY 3 DESC`, orgID, stationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var qty, revenue int
			if rows.Scan(&name, &qty, &revenue) == nil {
				byService = append(byService, map[string]any{
					"service_name": name, "quantity": qty, "revenue_xof": revenue,
				})
			}
		}
	}

	byEmployee := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT u.id, u.full_name, u.role,
		       (SELECT count(*) FROM wash_orders o
		         WHERE o.employee_id = u.id AND o.organization_id = $1
		           AND o.business_day >= $3 AND o.business_day <= $4 AND o.status = 'completed'),
		       (SELECT COALESCE(sum(o.total_xof), 0) FROM wash_orders o
		         WHERE o.employee_id = u.id AND o.organization_id = $1
		           AND o.business_day >= $3 AND o.business_day <= $4 AND o.status = 'completed'),
		       (SELECT COALESCE(sum(p.amount_xof), 0) FROM payments p
		         WHERE p.received_by = u.id AND p.organization_id = $1
		           AND p.business_day >= $3 AND p.business_day <= $4 AND p.status = 'valid'
		           AND p.method <> 'differe')
		FROM users u
		WHERE u.organization_id = $1 AND u.is_active
		  AND ($2 = '' OR u.station_id = $2)
		ORDER BY 4 DESC, u.full_name`, orgID, stationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name, role string
			var count, revenue, collected int
			if rows.Scan(&id, &name, &role, &count, &revenue, &collected) == nil {
				if count == 0 && collected == 0 {
					continue
				}
				byEmployee = append(byEmployee, map[string]any{
					"employee_id": id, "employee_name": name, "role": role,
					"role_label": roleLabelFR(role),
					"wash_count": count, "revenue_xof": revenue, "collected_xof": collected,
				})
			}
		}
	}

	byMethod := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT method, sum(amount_xof), count(*)
		FROM payments
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4 AND status = 'valid'
		  AND method <> 'differe'
		GROUP BY method ORDER BY 2 DESC`, orgID, stationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var method string
			var amount, count int
			if rows.Scan(&method, &amount, &count) == nil {
				byMethod = append(byMethod, map[string]any{
					"method": method, "method_label": methodLabelFR(method),
					"amount_xof": amount, "count": count,
				})
			}
		}
	}

	byExpenseCategory := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT category, sum(amount_xof), count(*)
		FROM expenses
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4 AND status = 'valid'
		GROUP BY category ORDER BY 2 DESC`, orgID, stationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var category string
			var amount, count int
			if rows.Scan(&category, &amount, &count) == nil {
				byExpenseCategory = append(byExpenseCategory, map[string]any{
					"category": category, "label": expenseCategoryLabelFR(category),
					"amount_xof": amount, "count": count,
				})
			}
		}
	}

	byVehicleCategory := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT COALESCE(NULLIF(vehicle_category_name, ''), 'Non précisé'),
		       count(*), COALESCE(sum(total_xof), 0)
		FROM wash_orders
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4 AND status = 'completed'
		GROUP BY 1 ORDER BY 3 DESC`, orgID, stationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var name string
			var count, revenue int
			if rows.Scan(&name, &count, &revenue) == nil {
				byVehicleCategory = append(byVehicleCategory, map[string]any{
					"vehicle_category_name": name, "wash_count": count, "revenue_xof": revenue,
				})
			}
		}
	}

	cashSessions := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT `+cashSessionColumns+` FROM cash_sessions
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4
		ORDER BY opened_at DESC`, orgID, stationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			if session, _, err := scanCashSessionWithOrg(rows); err == nil {
				cashSessions = append(cashSessions, cashSessionToJSON(session))
			}
		}
	}

	topCustomers := []map[string]any{}
	rows, err = a.Pool.Query(ctx, `
		SELECT c.id, c.full_name, count(*), COALESCE(sum(o.total_xof), 0)
		FROM wash_orders o JOIN customers c ON c.id = o.customer_id
		WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
		  AND o.business_day >= $3 AND o.business_day <= $4 AND o.status = 'completed'
		GROUP BY c.id, c.full_name ORDER BY 4 DESC LIMIT 10`, orgID, stationID, from, to)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name string
			var count, revenue int
			if rows.Scan(&id, &name, &count, &revenue) == nil {
				topCustomers = append(topCustomers, map[string]any{
					"customer_id": id, "customer_name": name,
					"wash_count": count, "revenue_xof": revenue,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"from": from, "to": to, "days": days,
		"station_id": stationID, "station_name": stationName,
		"totals":              a.totalsForPeriod(ctx, orgID, stationID, from, to).toMap(),
		"series":              series,
		"by_service":          byService,
		"by_employee":         byEmployee,
		"by_method":           byMethod,
		"by_expense_category": byExpenseCategory,
		"by_vehicle_category": byVehicleCategory,
		"cash_sessions":       cashSessions,
		"top_customers":       topCustomers,
		"definitions":         definitionsRapport(),
	})
}

// barème de commission : celui de l'employé s'il existe, sinon celui de la station.
type commissionRate struct {
	Mode           string
	PerWash        int
	Percent        float64
	FromEmployee   bool
	MonthlyGoalXOF int
}

func expenseCategoryLabelFR(category string) string {
	switch category {
	case "produits":
		return "Produits de lavage"
	case "eau":
		return "Eau"
	case "electricite":
		return "Électricité"
	case "salaires":
		return "Salaires"
	case "loyer":
		return "Loyer"
	case "entretien":
		return "Entretien et réparations"
	case "equipement":
		return "Équipement"
	case "transport":
		return "Transport"
	case "taxes":
		return "Taxes et impôts"
	case "autre":
		return "Autre"
	default:
		return category
	}
}

// stationCommissionSettings lit le barème de la station.
func (a *App) stationCommissionSettings(ctx context.Context, stationID string) commissionRate {
	rate := commissionRate{Mode: "none"}
	var raw []byte
	if err := a.Pool.QueryRow(ctx, `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&raw); err != nil {
		return rate
	}
	settings := map[string]any{}
	parseJSONB(raw, &settings)
	if mode, ok := settings["commission_mode"].(string); ok {
		rate.Mode = mode
	}
	if v, ok := settings["commission_per_wash_xof"].(float64); ok {
		rate.PerWash = int(v)
	}
	if v, ok := settings["commission_percent"].(float64); ok {
		rate.Percent = v
	}
	if v, ok := settings["monthly_goal_xof"].(float64); ok {
		rate.MonthlyGoalXOF = int(v)
	}
	return rate
}

// employeeCommissionRate : exception du compte si elle existe, sinon station.
func (a *App) employeeCommissionRate(ctx context.Context, stationRate commissionRate, employeeID string) commissionRate {
	var mode *string
	var perWash *int
	var percent *float64
	err := a.Pool.QueryRow(ctx, `
		SELECT commission_mode, commission_per_wash_xof, commission_percent
		FROM users WHERE id = $1`, employeeID).Scan(&mode, &perWash, &percent)
	if err != nil || mode == nil {
		return stationRate
	}
	rate := commissionRate{Mode: *mode, FromEmployee: true, MonthlyGoalXOF: stationRate.MonthlyGoalXOF}
	if perWash != nil {
		rate.PerWash = *perWash
	}
	if percent != nil {
		rate.Percent = *percent
	}
	return rate
}

// annualRate : helper de formatage des dates du rapport (jj/mm/aaaa).
func displayDate(iso string) string {
	if parsed, err := time.Parse("2006-01-02", iso); err == nil {
		return parsed.Format("02/01/2006")
	}
	return iso
}

// GET /commissions — état des commissions sur la période.
func (a *App) handleCommissions(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	q := r.URL.Query()

	stationID, err := a.stationDeTravail(ctx, user, q.Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	today := businessDay(time.Now())
	from, to := q.Get("from"), q.Get("to")
	if to == "" {
		to = today
	}
	if from == "" {
		from = today[:8] + "01"
	}
	if from > to {
		from, to = to, from
	}

	stationRate := commissionRate{Mode: "none"}
	if stationID != "" {
		stationRate = a.stationCommissionSettings(ctx, stationID)
	}

	// Lavages terminés de la période, par employé (les lavages sans laveur
	// forment la ligne « Non attribué »).
	type agg struct {
		Washes  int
		Revenue int
	}
	byEmployee := map[string]*agg{}
	rows, err := a.Pool.Query(ctx, `
		SELECT COALESCE(employee_id, ''), count(*), COALESCE(sum(total_xof), 0)
		FROM wash_orders
		WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
		  AND business_day >= $3 AND business_day <= $4 AND status = 'completed'
		GROUP BY 1`, orgID, stationID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	for rows.Next() {
		var employeeID string
		var count, revenue int
		if rows.Scan(&employeeID, &count, &revenue) == nil {
			byEmployee[employeeID] = &agg{Washes: count, Revenue: revenue}
		}
	}
	rows.Close()

	items := []map[string]any{}
	totals := map[string]int{"employee_count": 0, "wash_count": 0, "revenue_xof": 0, "commission_xof": 0}

	// Employés concernés : les laveurs de l'entreprise (même sans activité).
	empRows, err := a.Pool.Query(ctx, `
		SELECT id, full_name, role FROM users
		WHERE organization_id = $1 AND role IN ('washer','manager','owner') AND is_active
		  AND ($2 = '' OR station_id = $2)
		ORDER BY full_name`, orgID, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	seen := map[string]bool{}
	for empRows.Next() {
		var id, name, role string
		if empRows.Scan(&id, &name, &role) != nil {
			continue
		}
		seen[id] = true
		rate := a.employeeCommissionRate(ctx, stationRate, id)
		stats := byEmployee[id]
		item := map[string]any{
			"employee_id": id, "employee_name": name, "role": role, "role_label": roleLabelFR(role),
			"wash_count": 0, "revenue_xof": 0,
			"mode": rate.Mode, "per_wash_xof": rate.PerWash, "percent": rate.Percent,
			"rate_from_employee": rate.FromEmployee, "commission_xof": 0,
		}
		if stats != nil {
			commission := commissionAmount(rate, stats.Washes, stats.Revenue)
			item["wash_count"] = stats.Washes
			item["revenue_xof"] = stats.Revenue
			item["commission_xof"] = commission
			totals["employee_count"]++
			totals["wash_count"] += stats.Washes
			totals["revenue_xof"] += stats.Revenue
			totals["commission_xof"] += commission
		}
		items = append(items, item)
	}
	empRows.Close()

	// Lavages terminés sans laveur attribué.
	if stats, ok := byEmployee[""]; ok {
		items = append(items, map[string]any{
			"employee_id": "", "employee_name": "Non attribué", "role": "", "role_label": "Non attribué",
			"wash_count": stats.Washes, "revenue_xof": stats.Revenue,
			"mode": "none", "per_wash_xof": 0, "percent": 0,
			"rate_from_employee": false, "commission_xof": 0,
		})
		totals["wash_count"] += stats.Washes
		totals["revenue_xof"] += stats.Revenue
	}
	_ = seen

	writeJSON(w, http.StatusOK, map[string]any{
		"period": map[string]string{"from": displayDate(from), "to": displayDate(to)},
		"settings": map[string]any{
			"mode": stationRate.Mode, "per_wash_xof": stationRate.PerWash,
			"percent": stationRate.Percent, "monthly_goal_xof": stationRate.MonthlyGoalXOF,
		},
		"goal":   a.goalForStation(ctx, orgID, stationID),
		"items":  items,
		"totals": totals,
	})
}

func commissionAmount(rate commissionRate, washes, revenue int) int {
	switch rate.Mode {
	case "per_wash":
		return rate.PerWash * washes
	case "percent":
		return int(float64(revenue) * rate.Percent / 100)
	default:
		return 0
	}
}

// GET /commissions/export — même calcul, au format CSV.
func (a *App) handleCommissionsExport(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	q := r.URL.Query()
	stationID, err := a.stationDeTravail(ctx, user, q.Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	today := businessDay(time.Now())
	from, to := q.Get("from"), q.Get("to")
	if to == "" {
		to = today
	}
	if from == "" {
		from = today[:8] + "01"
	}
	if from > to {
		from, to = to, from
	}

	stationRate := commissionRate{Mode: "none"}
	if stationID != "" {
		stationRate = a.stationCommissionSettings(ctx, stationID)
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=commissions_%s_%s.csv", from, to))
	_, _ = fmt.Fprintln(w, "laveur;role;bareme;lavages_termines;ca_genere_xof;commission_xof")

	rows, err := a.Pool.Query(ctx, `
		SELECT u.id, u.full_name, u.role,
		       count(o.id) FILTER (WHERE o.status = 'completed'),
		       COALESCE(sum(o.total_xof) FILTER (WHERE o.status = 'completed'), 0)
		FROM users u
		LEFT JOIN wash_orders o ON o.employee_id = u.id
		     AND o.organization_id = $1
		     AND o.business_day >= $3 AND o.business_day <= $4
		WHERE u.organization_id = $1 AND u.role = 'washer' AND u.is_active
		  AND ($2 = '' OR u.station_id = $2)
		GROUP BY u.id, u.full_name, u.role
		ORDER BY u.full_name`, orgID, stationID, from, to)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, role string
		var washes, revenue int
		if rows.Scan(&id, &name, &role, &washes, &revenue) != nil {
			continue
		}
		rate := a.employeeCommissionRate(ctx, stationRate, id)
		_, _ = fmt.Fprintf(w, "%s;%s;%s;%d;%d;%d\n",
			name, roleLabelFR(role), commissionRateLabel(rate), washes, revenue,
			commissionAmount(rate, washes, revenue))
	}
}

// commissionRateLabel : « 500 FCFA par lavage », « 20 % du CA », « Aucune ».
func commissionRateLabel(rate commissionRate) string {
	switch rate.Mode {
	case "per_wash":
		return fmt.Sprintf("%d FCFA par lavage", rate.PerWash)
	case "percent":
		return fmt.Sprintf("%.2f %% du CA", rate.Percent)
	default:
		return "Aucune commission"
	}
}

// GET /reports/export — export CSV (type=wash_orders par défaut).
func (a *App) handleReportExport(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	q := r.URL.Query()
	exportType := orDefault(q.Get("type"), "wash_orders")

	stationID, err := a.stationDeTravail(ctx, user, q.Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	today := businessDay(time.Now())
	from, to := q.Get("from"), q.Get("to")
	if from == "" || to == "" {
		from, to = today, today
	}
	if from > to {
		from, to = to, from
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=washguard_%s_%s_%s.csv", exportType, from, to))

	switch exportType {
	case "payments":
		_, _ = fmt.Fprintln(w, "jour;reference;fiche;client;moyen;montant_xof;statut;encaisse_par")
		rows, err := a.Pool.Query(ctx, `
			SELECT p.business_day, p.reference, COALESCE(o.reference, ''), COALESCE(c.full_name, o.customer_name, ''),
			       p.method, p.amount_xof, p.status, p.received_by_name
			FROM payments p
			LEFT JOIN wash_orders o ON o.id = p.wash_order_id
			LEFT JOIN customers c ON c.id = p.customer_id
			WHERE p.organization_id = $1 AND ($2 = '' OR p.station_id = $2)
			  AND p.business_day >= $3 AND p.business_day <= $4
			ORDER BY p.paid_at DESC`, orgID, stationID, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var day, ref, orderRef, customer, method, status, actor string
			var amount int
			if rows.Scan(&day, &ref, &orderRef, &customer, &method, &amount, &status, &actor) == nil {
				_, _ = fmt.Fprintf(w, "%s;%s;%s;%s;%s;%d;%s;%s\n",
					day, ref, orderRef, customer, methodLabelFR(method), amount, statusLabelFR(status), actor)
			}
		}
	case "expenses":
		_, _ = fmt.Fprintln(w, "jour;categorie;libelle;montant_xof;moyen;statut;enregistre_par")
		rows, err := a.Pool.Query(ctx, `
			SELECT business_day, category, label, amount_xof, method, status, recorded_by_name
			FROM expenses
			WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
			  AND business_day >= $3 AND business_day <= $4
			ORDER BY spent_at DESC`, orgID, stationID, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var day, category, label, method, status, actor string
			var amount int
			if rows.Scan(&day, &category, &label, &amount, &method, &status, &actor) == nil {
				_, _ = fmt.Fprintf(w, "%s;%s;%s;%d;%s;%s;%s\n",
					day, expenseCategoryLabelFR(category), label, amount, methodLabelFR(method), statusLabelFR(status), actor)
			}
		}
	case "cash_sessions":
		_, _ = fmt.Fprintln(w, "jour;reference;statut;fond_initial;especes;mobile_money;autres;depenses;theorique;compte;ecart;ouverte_par;cloturee_par")
		rows, err := a.Pool.Query(ctx, `
			SELECT business_day, reference, status, opening_float_xof, cash_in_xof, mobile_money_xof,
			       other_in_xof, expenses_xof, expected_cash_xof, COALESCE(counted_cash_xof, 0),
			       COALESCE(difference_xof, 0), opened_by_name, COALESCE(closed_by_name, '')
			FROM cash_sessions
			WHERE organization_id = $1 AND ($2 = '' OR station_id = $2)
			  AND business_day >= $3 AND business_day <= $4
			ORDER BY opened_at DESC`, orgID, stationID, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var day, ref, status, openedBy, closedBy string
			var floatX, cashIn, mobile, other, expenses, expected, counted, diff int
			if rows.Scan(&day, &ref, &status, &floatX, &cashIn, &mobile, &other, &expenses,
				&expected, &counted, &diff, &openedBy, &closedBy) == nil {
				_, _ = fmt.Fprintf(w, "%s;%s;%s;%d;%d;%d;%d;%d;%d;%d;%d;%s;%s\n",
					day, ref, statusLabelFR(status), floatX, cashIn, mobile, other, expenses,
					expected, counted, diff, openedBy, closedBy)
			}
		}
	case "services":
		_, _ = fmt.Fprintln(w, "prestation;quantite;chiffre_affaires_xof")
		rows, err := a.Pool.Query(ctx, `
			SELECT i.service_name, sum(i.quantity), sum(i.line_total_xof)
			FROM wash_order_items i JOIN wash_orders o ON o.id = i.wash_order_id
			WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
			  AND o.business_day >= $3 AND o.business_day <= $4 AND o.status = 'completed'
			GROUP BY i.service_name ORDER BY 3 DESC`, orgID, stationID, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var qty, revenue int
			if rows.Scan(&name, &qty, &revenue) == nil {
				_, _ = fmt.Fprintf(w, "%s;%d;%d\n", name, qty, revenue)
			}
		}
	case "employees":
		_, _ = fmt.Fprintln(w, "employe;role;lavages_termines;ca_attribue_xof;encaisse_xof")
		rows, err := a.Pool.Query(ctx, `
			SELECT u.full_name, u.role,
			       count(o.id) FILTER (WHERE o.status = 'completed'),
			       COALESCE(sum(o.total_xof) FILTER (WHERE o.status = 'completed'), 0),
			       (SELECT COALESCE(sum(p.amount_xof), 0) FROM payments p
			         WHERE p.received_by = u.id AND p.organization_id = $1
			           AND p.business_day >= $3 AND p.business_day <= $4 AND p.status = 'valid'
		           AND p.method <> 'differe')
			FROM users u
			LEFT JOIN wash_orders o ON o.employee_id = u.id AND o.organization_id = $1
			     AND o.business_day >= $3 AND o.business_day <= $4
			WHERE u.organization_id = $1 AND ($2 = '' OR u.station_id = $2)
			GROUP BY u.id, u.full_name, u.role ORDER BY u.full_name`, orgID, stationID, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name, role string
			var washes, revenue, collected int
			if rows.Scan(&name, &role, &washes, &revenue, &collected) == nil {
				_, _ = fmt.Fprintf(w, "%s;%s;%d;%d;%d\n", name, roleLabelFR(role), washes, revenue, collected)
			}
		}
	case "daily":
		_, _ = fmt.Fprintln(w, "jour;lavages_termines;chiffre_affaires_xof;encaissements_xof;depenses_xof;resultat_xof")
		for _, day := range a.seriesForPeriod(ctx, orgID, stationID, from, to) {
			_, _ = fmt.Fprintf(w, "%v;%v;%v;%v;%v;%v\n", day["day"], day["wash_count"], day["revenue_xof"],
				day["payments_xof"], day["expenses_xof"], day["result_xof"])
		}
	default: // wash_orders
		_, _ = fmt.Fprintln(w, "reference;jour;client;vehicule;plaque;employe;statut;sous_total_xof;remise_xof;total_xof;encaisse_xof;reste_xof")
		rows, err := a.Pool.Query(ctx, `
			SELECT o.reference, o.business_day, o.customer_name, o.vehicle_category_name, o.plate,
			       o.employee_name, o.status, o.subtotal_xof, o.discount_xof, o.total_xof,
			       COALESCE(p.paid, 0)
			FROM wash_orders o
			LEFT JOIN LATERAL (
				SELECT sum(amount_xof) AS paid FROM payments
				WHERE wash_order_id = o.id AND status = 'valid' AND method <> 'differe'
			) p ON TRUE
			WHERE o.organization_id = $1 AND ($2 = '' OR o.station_id = $2)
			  AND o.business_day >= $3 AND o.business_day <= $4
			ORDER BY o.business_day DESC, o.reference`, orgID, stationID, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var ref, day, customer, vehicle, plate, employee, status string
			var subtotal, discount, total, paid int
			if rows.Scan(&ref, &day, &customer, &vehicle, &plate, &employee, &status,
				&subtotal, &discount, &total, &paid) == nil {
				due := total - paid
				if due < 0 || status == "canceled" {
					due = 0
				}
				_, _ = fmt.Fprintf(w, "%s;%s;%s;%s;%s;%s;%s;%d;%d;%d;%d;%d\n",
					ref, day, customer, vehicle, plate, employee, statusLabelFR(status),
					subtotal, discount, total, paid, due)
			}
		}
	}
	_ = io.Discard
}
