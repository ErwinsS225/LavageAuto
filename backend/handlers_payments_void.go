package main

import "net/http"

// ---------------------------------------------------------------------------
// Encaissements : annulation (contrepassation).
// ---------------------------------------------------------------------------

// POST /cash/payments/:id/void — annulation d'un encaissement (staff seul).
func (a *App) handlePaymentVoid(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user.Role == "cashier" {
		writeError(w, http.StatusForbidden, "forbidden",
			"L'annulation d'un encaissement nécessite l'accord du gérant.", nil)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")

	row := a.Pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id)
	p, err := scanPayment(row)
	if err != nil || p == nil {
		writeError(w, http.StatusNotFound, "not_found", "Encaissement introuvable.", nil)
		return
	}
	if p.OrganizationID != deref(user.OrganizationID) {
		writeError(w, http.StatusNotFound, "not_found", "Encaissement introuvable.", nil)
		return
	}
	if p.Status == "voided" {
		writeError(w, http.StatusConflict, "conflict", "Cet encaissement est déjà annulé.", nil)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &body)
	reason := orDefault(body.Reason, "Annulation validée par le gérant")

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE payments SET status = 'voided', void_reason = $1 WHERE id = $2`, reason, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'annulation.", nil)
		return
	}

	// Contrepassation sur la session de caisse concernée.
	if p.CashSessionID != nil {
		if p.Method == "especes" {
			_, err = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET cash_in_xof = cash_in_xof - $1,
				    expected_cash_xof = opening_float_xof + cash_in_xof - $1 - expenses_xof
				WHERE id = $2`, p.AmountXOF, p.CashSessionID)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET mobile_money_xof = mobile_money_xof - $1 WHERE id = $2`,
				p.AmountXOF, p.CashSessionID)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur de caisse.", nil)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, p.OrganizationID, p.StationID, user.FullName,
		"payment.voided", "payment", id,
		map[string]any{"amount_xof": p.AmountXOF, "reason": reason})

	row = a.Pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id)
	updated, _ := scanPayment(row)
	writeJSON(w, http.StatusOK, paymentToJSON(updated))
}
