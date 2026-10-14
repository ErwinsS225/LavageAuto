package main

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Caisse : session du jour, historique, ouverture / clôture / réouverture.
// ---------------------------------------------------------------------------

// cashMovement ligne du journal des opérations de la session ouverte.
type cashMovement struct {
	Kind      string
	ID        string
	Label     string
	AmountXOF int
	Method    string
	MethodLbl string
	At        string
	Actor     string
	Status    string
	Reference string
}

func methodLabel(m string) string {
	if m == "especes" {
		return "Espèces"
	}
	return "Mobile Money"
}

// stationOrDefault est remplacé par App.stationDeTravail : la station demandée
// doit appartenir à l'entreprise, sinon la requête est refusée.

// GET /cash/current — session ouverte + mouvements de la journée.
func (a *App) handleCashCurrent(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	stationID, err := a.stationDeTravail(ctx, user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Aucune station rattachée à votre compte.", nil)
		return
	}

	var open map[string]any
	row := a.Pool.QueryRow(ctx, `
		SELECT `+cashSessionColumns+` FROM cash_sessions
		WHERE station_id = $1 AND status = 'open'
		ORDER BY opened_at DESC LIMIT 1`, stationID)
	if session, _, err := scanCashSessionWithOrg(row); err == nil {
		open = cashSessionToJSON(session)
	}

	// Dernière session clôturée (pour reprise du fond de caisse).
	var last map[string]any
	row = a.Pool.QueryRow(ctx, `
		SELECT `+cashSessionColumns+` FROM cash_sessions
		WHERE station_id = $1 AND status = 'closed'
		ORDER BY closed_at DESC LIMIT 1`, stationID)
	if session, _, err := scanCashSessionWithOrg(row); err == nil {
		last = cashSessionToJSON(session)
	}

	movements := []map[string]any{}
	if open != nil {
		sessionID, _ := open["id"].(string)

		pRows, err := a.Pool.Query(ctx, `
			SELECT id, wash_order_id, amount_xof, method, paid_at, received_by_name, status, reference
			FROM payments WHERE cash_session_id = $1 AND status = 'valid'
			ORDER BY paid_at DESC`, sessionID)
		if err == nil {
			for pRows.Next() {
				var id, m, actor, status, ref string
				var amount int
				var at time.Time
				var orderID *string
				_ = pRows.Scan(&id, &orderID, &amount, &m, &at, &actor, &status, &ref)
				label := "Paiement lavage"
				if orderID == nil {
					label = "Encaissement libre"
				}
				movements = append(movements, map[string]any{
					"kind": "payment", "id": id, "label": label,
					"amount_xof": amount, "method": m, "method_label": methodLabel(m),
					"at": at.Format(timeLayout), "actor": actor,
					"status": status, "reference": ref,
				})
			}
			pRows.Close()
		}

		eRows, err := a.Pool.Query(ctx, `
			SELECT id, label, amount_xof, method, spent_at, recorded_by_name, status, reference
			FROM expenses WHERE cash_session_id = $1 ORDER BY spent_at DESC`, sessionID)
		if err == nil {
			for eRows.Next() {
				var id, label, m, actor, status, ref string
				var amount int
				var at time.Time
				_ = eRows.Scan(&id, &label, &amount, &m, &at, &actor, &status, &ref)
				movements = append(movements, map[string]any{
					"kind": "expense", "id": id, "label": label,
					"amount_xof": amount, "method": m, "method_label": methodLabel(m),
					"at": at.Format(timeLayout), "actor": actor,
					"status": status, "reference": ref,
				})
			}
			eRows.Close()
		}
	}

	// Le front lit `open` (booléen) : `is_open` reste exposé pour compatibilité.
	writeJSON(w, http.StatusOK, map[string]any{
		"open":         open != nil,
		"is_open":      open != nil,
		"session":      open,
		"last":         last,
		"last_session": last,
		"movements":    movements,
	})
}

// GET /cash/sessions — historique des sessions d'une station.
func (a *App) handleCashSessionsList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}

	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+cashSessionColumns+` FROM cash_sessions
		WHERE organization_id = $2 AND ($1 = '' OR station_id = $1)
		ORDER BY opened_at DESC LIMIT 100`, stationID, deref(user.OrganizationID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		session, _, err := scanCashSessionWithOrg(rows)
		if err != nil {
			continue
		}
		items = append(items, cashSessionToJSON(session))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// GET /cash/sessions/:id — détail d'une session.
func (a *App) handleCashSessionDetail(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")

	session, err := a.userCashSession(ctx, user, id)
	if err != nil || session == nil {
		writeError(w, http.StatusNotFound, "not_found", "Session introuvable.", nil)
		return
	}
	writeJSON(w, http.StatusOK, cashSessionToJSON(session))
}
