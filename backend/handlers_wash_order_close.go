package main

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// Fiches de lavage : annulation et encaissement.
// ---------------------------------------------------------------------------

// POST /wash-orders/:id/cancel — annulation, réservée au gérant / propriétaire.
func (a *App) handleWashOrderCancel(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user.Role == "cashier" || user.Role == "washer" {
		writeError(w, http.StatusForbidden, "forbidden",
			"L'annulation d'un lavage nécessite l'autorisation du gérant ou du propriétaire de la station.", nil)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	order, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &body)
	reason := orDefault(body.Reason, "Annulation validée par le gérant")

	if _, err := a.Pool.Exec(ctx, `
		UPDATE wash_orders SET status = 'canceled', cancel_reason = $1, canceled_at = now()
		WHERE id = $2`, reason, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'annulation.", nil)
		return
	}

	a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
		"wash_order.canceled", "wash_order", id, map[string]any{
			"reason":        reason,
			"authorized_by": user.FullName,
			"actor_role":    user.Role,
		})

	updated, _ := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	updated.Items = a.loadItems(r, id)
	writeJSON(w, http.StatusOK, updated)
}

// POST /wash-orders/:id/payments — encaissement d'une fiche et clôture.
func (a *App) handleWashOrderPayment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	order, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}

	var body struct {
		AmountXOF int    `json:"amount_xof"`
		Method    string `json:"method"`
		Reference string `json:"reference"`
		Note      string `json:"note"`
	}
	_ = decodeBody(r, &body)
	amount := body.AmountXOF
	if amount <= 0 {
		amount = order.TotalXOF
	}
	method := orDefault(body.Method, "especes")

	// Session de caisse ouverte de cette station, le cas échéant.
	var sessionID *string
	err = a.Pool.QueryRow(ctx, `
		SELECT id FROM cash_sessions
		WHERE station_id = $1 AND status = 'open'
		ORDER BY opened_at DESC LIMIT 1`, order.StationID).Scan(&sessionID)
	if err != nil {
		sessionID = nil
	}

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	paymentID := newID("pay")
	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (id, organization_id, station_id, wash_order_id, customer_id,
			kind, method, amount_xof, reference, note, business_day,
			cash_session_id, received_by, received_by_name, status)
		VALUES ($1,$2,$3,$4,$5,'lavage',$6,$7,$8,$9,$10,$11,$12,$13,'valid')`,
		paymentID, order.OrganizationID, order.StationID, order.ID, order.CustomerID,
		method, amount, body.Reference, body.Note, order.BusinessDay,
		sessionID, user.ID, user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'encaissement.", nil)
		return
	}

	// Report de l'encaissement sur la session de caisse ouverte.
	if sessionID != nil {
		if method == "especes" {
			_, err = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET cash_in_xof = cash_in_xof + $1,
				    expected_cash_xof = opening_float_xof + cash_in_xof + $1 - expenses_xof,
				    payment_count = payment_count + 1,
				    operations = operations + 1
				WHERE id = $2`, amount, sessionID)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET mobile_money_xof = mobile_money_xof + $1,
				    payment_count = payment_count + 1,
				    operations = operations + 1
				WHERE id = $2`, amount, sessionID)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur de caisse.", nil)
			return
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE wash_orders SET status = 'completed', completed_at = now() WHERE id = $1`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
		"payment.created", "payment", paymentID,
		map[string]any{"amount_xof": amount, "method": method})

	updated, _ := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	updated.Items = a.loadItems(r, id)
	writeJSON(w, http.StatusOK, map[string]any{"order": updated})
}
