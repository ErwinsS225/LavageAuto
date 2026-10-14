package main

import (
	"net/http"
)

// ---------------------------------------------------------------------------
// Fiches de lavage : annulation et encaissement.
// ---------------------------------------------------------------------------

// POST /wash-orders/:id/cancel — annulation (permission wash:cancel exigée par
// la route). Un motif est obligatoire : l'opération est conservée dans
// l'historique et doit pouvoir s'expliquer.
func (a *App) handleWashOrderCancel(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	order, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != order.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if order.Status == "canceled" {
		writeError(w, http.StatusConflict, "conflict", "Cette fiche est déjà annulée.", nil)
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &body)
	if len(body.Reason) < 3 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Un motif d'annulation est obligatoire.", map[string]string{"reason": "Motif requis"})
		return
	}

	// Une fiche déjà encaissée ne s'annule pas depuis cet écran : il faut
	// d'abord annuler l'encaissement (trace comptable).
	var paid int
	// Les engagements de paiement (différé) ne sont pas des encaissements : ils
	// n'empêchent pas l'annulation d'une fiche.
	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_xof), 0) FROM payments
		WHERE wash_order_id = $1 AND status = 'valid' AND method <> 'differe'`, id).Scan(&paid)
	if paid > 0 {
		writeError(w, http.StatusConflict, "cash_session_required",
			"Cette fiche porte un encaissement : annulez d'abord le paiement depuis l'écran Caisse.", nil)
		return
	}

	if _, err := a.Pool.Exec(ctx, `
		UPDATE wash_orders SET status = 'canceled', cancel_reason = $1, canceled_at = now()
		WHERE id = $2 AND organization_id = $3`, body.Reason, id, deref(user.OrganizationID)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'annulation.", nil)
		return
	}

	a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
		"wash_order.canceled", "wash_order", id, map[string]any{
			"reason":        body.Reason,
			"authorized_by": user.FullName,
			"actor_role":    user.Role,
		})

	updated, _ := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	items := a.loadItems(r, id, deref(user.OrganizationID))
	writeJSON(w, http.StatusOK, orderToJSON(updated, 0, items, "", nil))
}

// POST /wash-orders/:id/payments — encaissement d'une fiche.
//
// Le montant est borné par le reste à payer : un encaissement partiel laisse
// la fiche ouverte (elle n'est plus close d'office), et la règle « session de
// caisse ouverte » de la station est appliquée aux espèces.
func (a *App) handleWashOrderPayment(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	order, err := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	if err != nil || order == nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != order.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if order.Status == "canceled" {
		writeError(w, http.StatusConflict, "conflict", "Cette fiche est annulée.", nil)
		return
	}

	var body struct {
		AmountXOF       int    `json:"amount_xof"`
		Method          string `json:"method"`
		PaymentMethodID string `json:"payment_method_id"`
		Reference       string `json:"reference"`
		Note            string `json:"note"`
	}
	_ = decodeBody(r, &body)

	_, paid, due, err := a.soldeFiche(r, id, deref(user.OrganizationID))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if due <= 0 {
		writeError(w, http.StatusConflict, "conflict", "Cette fiche est déjà soldée.", nil)
		return
	}
	// Le montant est obligatoire et strictement positif, comme sur
	// /cash/payments et comme dans l'écran d'encaissement : un montant nul ou
	// absent ne doit pas encaisser la totalité du reste dû à la place du
	// caissier (une interface fautive solderait la fiche sans le dire).
	amount := body.AmountXOF
	if amount <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le montant doit être supérieur à zéro.",
			map[string]string{"amount_xof": "Montant invalide"})
		return
	}
	if amount > due {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le montant dépasse le reste à payer de la fiche.",
			map[string]string{"amount_xof": "Montant supérieur au solde restant dû"})
		return
	}
	// Le moyen est résolu par le serveur : moyen déclaré (actif) ou famille
	// canonique, référence exigée selon le moyen, permission pour le différé.
	method, methodID, methodName, err := a.resoudreMoyenPaiement(ctx, user, order.StationID, encaissementDemande{
		MethodID:  body.PaymentMethodID,
		Method:    body.Method,
		Reference: body.Reference,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}

	sessionID, err := a.requireOpenCashSession(r, order.StationID, method)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	// Un engagement de paiement ne se rattache à aucune caisse : il n'y entre
	// aucune somme, et le mouvement apparaîtrait à tort dans le journal du
	// comptoir.
	if method == "differe" {
		sessionID = nil
	}
	_ = paid

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	paymentID := newID("pay")
	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (id, organization_id, station_id, wash_order_id, customer_id,
			kind, method, payment_method_id, payment_method_name, amount_xof, reference, note,
			business_day, cash_session_id, received_by, received_by_name, status)
		VALUES ($1,$2,$3,$4,$5,'lavage',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,'valid')`,
		paymentID, order.OrganizationID, order.StationID, order.ID, order.CustomerID,
		method, methodID, methodName, amount, body.Reference, body.Note, order.BusinessDay,
		sessionID, user.ID, user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'encaissement.", nil)
		return
	}

	if update, toucheCaisse := cashColumnsFor(method); sessionID != nil && toucheCaisse {
		if _, err := tx.Exec(ctx, `UPDATE cash_sessions SET `+update+` WHERE id = $2`,
			amount, sessionID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur de caisse.", nil)
			return
		}
	}

	// La fiche ne passe « terminée » que si le solde est couvert par une somme
	// réellement reçue : un paiement différé la laisse ouverte et due.
	remaining := due - amount
	ficheSoldee := remaining <= 0 && method != "differe"
	if ficheSoldee {
		if _, err := tx.Exec(ctx, `
			UPDATE wash_orders
			SET status = 'completed', completed_at = COALESCE(completed_at, now())
			WHERE id = $1 AND organization_id = $2 AND status <> 'canceled'`,
			id, order.OrganizationID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, order.OrganizationID, order.StationID, user.FullName,
		"payment.created", "payment", paymentID,
		map[string]any{"amount_xof": amount, "method": method, "actor_role": user.Role})

	// Fiche soldée : le justificatif reçoit son numéro, une seule fois.
	if ficheSoldee {
		a.emettreRecuAuto(ctx, order.OrganizationID, order.StationID, id, order.BusinessDay, user.FullName)
	}

	updated, _ := a.getOwnedOrder(ctx, id, deref(user.OrganizationID))
	items := a.loadItems(r, id, deref(user.OrganizationID))
	newPaid := a.paidForOrder(r, id, order.OrganizationID)
	writeJSON(w, http.StatusOK, map[string]any{
		"order":    orderToJSON(updated, newPaid, items, "", nil),
		"due_xof":  max(0, updated.TotalXOF-newPaid),
		"paid_xof": newPaid,
	})
}
