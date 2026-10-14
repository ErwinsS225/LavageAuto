package main

import (
	"errors"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Encaissements : création (voir aussi POST /wash-orders/{id}/payments).
//
// Règles appliquées ici :
//   - la station est vérifiée (elle peut venir du corps de requête) ;
//   - une fiche d'une autre entreprise est introuvable ;
//   - l'encaissement d'une fiche exige une session de caisse ouverte quand la
//     station l'exige pour les espèces, et respecte le solde restant dû ;
//   - chaque méthode alimente son propre compteur de caisse : les espèces ne
//     gonflent plus « autres encaissements », la carte n'est plus rangée dans
//     le mobile money, et une annulation redescend les compteurs.
// ---------------------------------------------------------------------------

// soldeFiche calcule le reste à payer d'une fiche : total des encaissements
// valides déduit. Le calcul est fait en base, jamais supposé.
//
// Un paiement **différé** ne compte pas : c'est un engagement du client, pas
// une somme reçue. La fiche reste due jusqu'au règlement effectif — c'est ce
// qui empêche un reçu d'attester d'un encaissement qui n'a pas eu lieu.
func (a *App) soldeFiche(r *http.Request, orderID, orgID string) (total, paid, due int, err error) {
	err = a.Pool.QueryRow(r.Context(), `
		SELECT o.total_xof,
		       COALESCE((SELECT sum(p.amount_xof) FROM payments p
		                  WHERE p.wash_order_id = o.id AND p.status = 'valid'
		                    AND p.method <> 'differe'), 0)
		FROM wash_orders o
		WHERE o.id = $1 AND o.organization_id = $2 AND o.status <> 'canceled'`,
		orderID, orgID).Scan(&total, &paid)
	if err != nil {
		return 0, 0, 0, errNotFound
	}
	due = total - paid
	if due < 0 {
		due = 0
	}
	return total, paid, due, nil
}

// requireOpenCashSession vérifie la règle du réglage
// `require_open_cash_session` de la station avant un encaissement en espèces.
func (a *App) requireOpenCashSession(r *http.Request, stationID string, method string) (*string, error) {
	var sessionID *string
	_ = a.Pool.QueryRow(r.Context(), `
		SELECT id FROM cash_sessions
		WHERE station_id = $1 AND status = 'open' ORDER BY opened_at DESC LIMIT 1`,
		stationID).Scan(&sessionID)

	if method == "especes" && sessionID == nil {
		var settingsRaw []byte
		settings := map[string]any{}
		_ = a.Pool.QueryRow(r.Context(), `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&settingsRaw)
		parseJSONB(settingsRaw, &settings)
		if required, _ := settings["require_open_cash_session"].(bool); required {
			return nil, &apiError{
				Status:  http.StatusConflict,
				Code:    "cash_session_required",
				Message: "Aucune session de caisse n'est ouverte : impossible d'encaisser des espèces.",
			}
		}
	}
	return sessionID, nil
}

// cashColumnsFor : requête de mise à jour du bon compteur de la session.
//
// Le second retour dit si la méthode touche la caisse : un paiement différé n'y
// entre pas (aucune somme n'est entrée), et une chaîne vide le signale aux
// appelants — plutôt qu'une écriture qui ferait croire à un encaissement.
func cashColumnsFor(method string) (string, bool) {
	switch method {
	case "differe":
		return "", false
	case "especes":
		return `cash_in_xof = cash_in_xof + $1,
		        expected_cash_xof = opening_float_xof + cash_in_xof + $1 - expenses_xof,
		        payment_count = payment_count + 1, operations = operations + 1`, true
	case "mobile_money":
		return `mobile_money_xof = mobile_money_xof + $1,
		        payment_count = payment_count + 1, operations = operations + 1`, true
	default:
		// carte, virement, autre : ni espèces ni mobile money.
		return `other_in_xof = other_in_xof + $1,
		        payment_count = payment_count + 1, operations = operations + 1`, false
	}
}

func (a *App) handlePaymentCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	var body struct {
		StationID   string `json:"station_id"`
		WashOrderID string `json:"wash_order_id"`
		CustomerID  string `json:"customer_id"`
		Kind        string `json:"kind"`
		Method      string `json:"method"`
		// PaymentMethodID : le moyen **déclaré** par la station (Orange Money,
		// Wave…). Quand il est fourni, c'est lui qui décide la famille ; le
		// libellé choisi par la station est recopié sur l'encaissement.
		PaymentMethodID string `json:"payment_method_id"`
		AmountXOF       int    `json:"amount_xof"`
		Reference       string `json:"reference"`
		Note            string `json:"note"`
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
	stationID, err := a.resolveStation(ctx, user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	kind := orDefault(body.Kind, "autre")
	// Le moyen est résolu côté serveur : moyen déclaré (actif, référence
	// exigée) ou famille canonique. Un moyen désactivé est refusé, un paiement
	// différé exige la permission correspondante.
	method, methodID, methodName, err := a.resoudreMoyenPaiement(ctx, user, stationID, encaissementDemande{
		MethodID:  body.PaymentMethodID,
		Method:    body.Method,
		Reference: body.Reference,
	})
	if err != nil {
		writeAPIError(w, err)
		return
	}
	today := businessDay(time.Now())

	var orderPtr, customerPtr *string
	if body.WashOrderID != "" {
		// La fiche doit appartenir à l'entreprise et être à la bonne station.
		var orderStation, orderStatus string
		if err := a.Pool.QueryRow(ctx, `
			SELECT station_id, status FROM wash_orders
			WHERE id = $1 AND organization_id = $2`, body.WashOrderID, deref(user.OrganizationID),
		).Scan(&orderStation, &orderStatus); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
			return
		}
		if orderStatus == "canceled" {
			writeError(w, http.StatusConflict, "conflict", "Cette fiche est annulée.", nil)
			return
		}
		if orderStation != stationID {
			writeAPIError(w, errStationForbidden)
			return
		}
		_, _, due, err := a.soldeFiche(r, body.WashOrderID, deref(user.OrganizationID))
		if err != nil {
			writeAPIError(w, err)
			return
		}
		if body.AmountXOF > due {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le montant dépasse le reste à payer de la fiche.",
				map[string]string{"amount_xof": "Montant supérieur au solde restant dû"})
			return
		}
		// Acomptes : la même règle que sur /wash-orders/{id}/payments, lue à la
		// station de la fiche. Un moyen différé n'est pas un encaissement.
		if refus := a.verifierAcompte(ctx, orderStation, method, body.AmountXOF, due); refus != nil {
			writeAPIError(w, refus)
			return
		}
		orderPtr = &body.WashOrderID
	}
	if body.CustomerID != "" {
		var exists string
		if err := a.Pool.QueryRow(ctx, `
			SELECT id FROM customers WHERE id = $1 AND organization_id = $2`,
			body.CustomerID, deref(user.OrganizationID)).Scan(&exists); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Client introuvable.", nil)
			return
		}
		customerPtr = &body.CustomerID
	}

	sessionID, err := a.requireOpenCashSession(r, stationID, method)
	if err != nil {
		writeAPIError(w, err)
		return
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
			kind, method, payment_method_id, payment_method_name, amount_xof, reference, note,
			business_day, cash_session_id, received_by, received_by_name, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'valid')`,
		id, deref(user.OrganizationID), stationID, orderPtr, customerPtr,
		kind, method, methodID, methodName, body.AmountXOF, body.Reference, body.Note, today,
		sessionID, user.ID, user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'encaissement.", nil)
		return
	}

	if update, toucheCaisse := cashColumnsFor(method); sessionID != nil && toucheCaisse {
		if _, err := tx.Exec(ctx, `UPDATE cash_sessions SET `+update+` WHERE id = $2`,
			body.AmountXOF, sessionID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur de caisse.", nil)
			return
		}
	}

	// La fiche est soldée seulement quand plus rien n'est dû : un acompte — ou
	// un paiement différé — laisse la fiche ouverte, comme l'attend l'écran
	// Caisse.
	var ficheSoldee bool
	if orderPtr != nil {
		var due int
		_ = tx.QueryRow(ctx, `
			SELECT o.total_xof - COALESCE((SELECT sum(p.amount_xof) FROM payments p
			                                WHERE p.wash_order_id = o.id AND p.status = 'valid'
			                                  AND p.method <> 'differe'), 0)
			FROM wash_orders o WHERE o.id = $1`, *orderPtr).Scan(&due)
		if due <= 0 {
			ficheSoldee = true
			_, _ = tx.Exec(ctx, `
				UPDATE wash_orders
				SET status = 'completed', completed_at = COALESCE(completed_at, now())
				WHERE id = $1 AND status <> 'canceled'`, *orderPtr)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	// Fiche soldée par cet encaissement : le justificatif reçoit son numéro
	// (une seule fois). L'échec éventuel n'annule pas l'encaissement — le reçu
	// reste demandable par GET /wash-orders/{id}/receipt.
	if ficheSoldee && orderPtr != nil {
		a.emettreRecuAuto(ctx, deref(user.OrganizationID), stationID, *orderPtr, today, user.FullName)
	}

	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"payment.created", "payment", id,
		map[string]any{"amount_xof": body.AmountXOF, "method": method, "kind": kind, "actor_role": user.Role})

	row := a.Pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id)
	p, _ := scanPayment(row)
	writeJSON(w, http.StatusCreated, paymentToJSON(p))
}

// errPaymentIntrouvable : identifiant hors entreprise ou hors station.
var errPaymentIntrouvable = errors.New("payment introuvable")
