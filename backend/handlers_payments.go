package main

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Encaissements : modèle, lecture.
// ---------------------------------------------------------------------------

// paymentColumns colonnes de la table payments (ordre imposé au scan).
const paymentColumns = `id, organization_id, station_id, wash_order_id, customer_id,
	kind, method, amount_xof, reference, note, paid_at, business_day,
	cash_session_id, received_by, received_by_name, status, void_reason`

// scanPayment lit une ligne de payments.
func scanPayment(row scannable) (*Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.OrganizationID, &p.StationID, &p.WashOrderID, &p.CustomerID,
		&p.Kind, &p.Method, &p.AmountXOF, &p.Reference, &p.Note, &p.PaidAt, &p.BusinessDay,
		&p.CashSessionID, &p.ReceivedBy, &p.ReceivedByName, &p.Status, &p.VoidReason)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// paymentToJSON sérialise un encaissement.
func paymentToJSON(p *Payment) map[string]any {
	return map[string]any{
		"id":                p.ID,
		"organization_id":   p.OrganizationID,
		"station_id":        p.StationID,
		"wash_order_id":     p.WashOrderID,
		"customer_id":       p.CustomerID,
		"kind":              p.Kind,
		"method":            p.Method,
		"method_label":      methodLabel(p.Method),
		"amount_xof":        p.AmountXOF,
		"reference":         p.Reference,
		"note":              p.Note,
		"paid_at":           p.PaidAt.Format(time.RFC3339),
		"business_day":      p.BusinessDay,
		"cash_session_id":   p.CashSessionID,
		"received_by":       p.ReceivedBy,
		"received_by_name":  p.ReceivedByName,
		"status":            p.Status,
		"void_reason":       p.VoidReason,
	}
}

// GET /cash/payments — journal des encaissements de la station.
func (a *App) handlePaymentsList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	q := r.URL.Query()
	stationID := stationOrDefault(r, deref(user.StationID))

	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+paymentColumns+` FROM payments
		WHERE station_id = $1
		  AND ($2 = '' OR business_day >= $2)
		  AND ($3 = '' OR business_day <= $3)
		  AND ($4 = '' OR method = $4)
		  AND ($5 = '' OR status = $5)
		ORDER BY paid_at DESC LIMIT 200`,
		stationID, q.Get("from"), q.Get("to"), q.Get("method"), q.Get("status"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	total, cash, mobile := 0, 0, 0
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			continue
		}
		if p.Status == "valid" {
			total += p.AmountXOF
			if p.Method == "especes" {
				cash += p.AmountXOF
			} else {
				mobile += p.AmountXOF
			}
		}
		items = append(items, paymentToJSON(p))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
		"totals": map[string]any{
			"amount_xof":       total,
			"cash_xof":         cash,
			"mobile_money_xof": mobile,
		},
	})
}
