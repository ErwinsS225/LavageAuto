package main

import "net/http"

// ---------------------------------------------------------------------------
// Encaissements : annulation (contrepassation).
//
// Réservée au gérant et au propriétaire (permission payment:void au niveau de
// la route). La session de caisse est corrigée selon la méthode réellement
// utilisée, et les compteurs redescendent : l'annulation laisse des totaux
// justes, pas seulement une ligne marquée « voided ».
// ---------------------------------------------------------------------------

func (a *App) handlePaymentVoid(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")

	// La requête porte l'entreprise : l'encaissement d'une autre entreprise est
	// introuvable, jamais annulable.
	row := a.Pool.QueryRow(ctx, `
		SELECT `+paymentColumns+` FROM payments
		WHERE id = $1 AND organization_id = $2`, id, deref(user.OrganizationID))
	p, err := scanPayment(row)
	if err != nil || p == nil {
		writeError(w, http.StatusNotFound, "not_found", "Encaissement introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != p.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Encaissement introuvable.", nil)
		return
	}
	if p.Status == "voided" {
		writeError(w, http.StatusConflict, "conflict", "Cet encaissement est déjà annulé.", nil)
		return
	}

	// Le réglage de caisse de la station décide si une contrepassation est
	// ouverte : certaines exploitations interdisent l'annulation au comptoir et
	// passent par le gérant. Le refus est explicite, jamais silencieux.
	parametres, errParam := a.lireParametresCaisse(ctx, p.StationID)
	if errParam == nil && !parametres.AllowVoid {
		writeError(w, http.StatusConflict, "void_not_allowed",
			"Les annulations d'encaissement sont désactivées pour cette station : activez-les dans les paramètres de caisse.",
			map[string]string{"payment": "Annulation désactivée"})
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &body)
	// Un motif est exigé : une contrepassation doit pouvoir s'expliquer.
	if len(body.Reason) < 3 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Un motif d'annulation est obligatoire.", map[string]string{"reason": "Motif requis"})
		return
	}

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE payments SET status = 'voided', void_reason = $1
		WHERE id = $2 AND organization_id = $3`, body.Reason, id, p.OrganizationID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'annulation.", nil)
		return
	}

	// Contrepassation sur la session de caisse concernée. Un paiement différé
	// n'a jamais alimenté de compteur : rien à reprendre.
	if p.CashSessionID != nil && p.Method != "differe" {
		var errExec error
		switch p.Method {
		case "especes":
			_, errExec = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET cash_in_xof = cash_in_xof - $1,
				    expected_cash_xof = opening_float_xof + cash_in_xof - $1 - expenses_xof,
				    payment_count = GREATEST(payment_count - 1, 0),
				    operations = GREATEST(operations - 1, 0)
				WHERE id = $2`, p.AmountXOF, p.CashSessionID)
		case "mobile_money":
			_, errExec = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET mobile_money_xof = mobile_money_xof - $1,
				    payment_count = GREATEST(payment_count - 1, 0),
				    operations = GREATEST(operations - 1, 0)
				WHERE id = $2`, p.AmountXOF, p.CashSessionID)
		default:
			_, errExec = tx.Exec(ctx, `
				UPDATE cash_sessions
				SET other_in_xof = other_in_xof - $1,
				    payment_count = GREATEST(payment_count - 1, 0),
				    operations = GREATEST(operations - 1, 0)
				WHERE id = $2`, p.AmountXOF, p.CashSessionID)
		}
		if errExec != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur de caisse.", nil)
			return
		}
	}

	// La fiche redevient ouverte si le solde n'est plus couvert.
	if p.WashOrderID != nil {
		var due int
		_ = tx.QueryRow(ctx, `
			SELECT o.total_xof - COALESCE((SELECT sum(x.amount_xof) FROM payments x
			                               WHERE x.wash_order_id = o.id AND x.status = 'valid'
			                                 AND x.method <> 'differe'), 0)
			FROM wash_orders o WHERE o.id = $1 AND o.status = 'completed'`, *p.WashOrderID).Scan(&due)
		if due > 0 {
			_, _ = tx.Exec(ctx, `
				UPDATE wash_orders SET status = 'in_progress', completed_at = NULL
				WHERE id = $1`, *p.WashOrderID)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, p.OrganizationID, p.StationID, user.FullName,
		"payment.voided", "payment", id,
		map[string]any{"amount_xof": p.AmountXOF, "method": p.Method, "reason": body.Reason, "actor_role": user.Role})

	row = a.Pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id)
	updated, _ := scanPayment(row)
	writeJSON(w, http.StatusOK, paymentToJSON(updated))
}
