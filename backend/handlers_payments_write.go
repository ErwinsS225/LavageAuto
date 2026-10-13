package main

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Encaissements : création et annulation.
// ---------------------------------------------------------------------------

// POST /cash/payments — encaissement libre, rattaché ou non à une fiche.
func (a *App) handlePaymentCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	var body struct {
		StationID   string `json:"station_id"`
		WashOrderID string `json:"wash_order_id"`
		CustomerID  string `json:"customer_id"`
		Kind        string `json:"kind"`
		Method      string `json:"method"`
		AmountXOF   int    `json:"amount_xof"`
		Reference   string `json:"reference"`
		Note        string `json:"note"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if body.AmountXOF <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "Le montant doit être supérieur à zéro.",
			map[string]string{"amount_xof": "Montant invalide"})
		return
	}
	stationID := body.StationID
	if stationID == "" {
		stationID = deref(user.StationID)
	}
	kind := orDefault(body.Kind, "autre")
	method := orDefault(body.Method, "especes")
	today := businessDay(time.Now())

	// Session ouverte de la station, si elle existe.
	var sessionID *string
	_ = a.Pool.QueryRow(ctx, `
		SELECT id FROM cash_sessions
		WHERE station_id = $1 AND status = 'open' ORDER BY opened_at DESC LIMIT 1`,
		stationID).Scan(&sessionID)

	var orderPtr, customerPtr *string
	if body.WashOrderID != "" {
		orderPtr = &body.WashOrderID
	}
	if body.CustomerID != "" {
		customerPtr = &body.CustomerID
	}

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	id := newID("pay")
	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (id, organization_id, station_id, wash_order_id, customer_id,
			kind, method, amount_xof, reference, note, business_day,
			cash_session_id, received_by, received_by_name, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'valid')`,
		id, deref(user.OrganizationID), stationID, orderPtr, customerPtr,
		kind, method, body.AmountXOF, body.Reference, body.Note, today,
		sessionID, user.ID, user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'encaissement.", nil)
		return
	}

	if sessionID != nil {
		if method == "especes" {
			_, err = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET cash_in_xof = cash_in_xof + $1,
				    expected_cash_xof = opening_float_xof + cash_in_xof + $1 - expenses_xof,
				    other_in_xof = other_in_xof + $1,
				    payment_count = payment_count + 1,
				    operations = operations + 1
				WHERE id = $2`, body.AmountXOF, sessionID)
		} else {
			_, err = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET mobile_money_xof = mobile_money_xof + $1,
				    payment_count = payment_count + 1,
				    operations = operations + 1
				WHERE id = $2`, body.AmountXOF, sessionID)
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

	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"payment.created", "payment", id,
		map[string]any{"amount_xof": body.AmountXOF, "method": method, "kind": kind})

	row := a.Pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id)
	p, _ := scanPayment(row)
	writeJSON(w, http.StatusCreated, paymentToJSON(p))
}
