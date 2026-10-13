package main

import (
	"fmt"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Caisse : ouverture, clôture et réouverture de session.
// ---------------------------------------------------------------------------

// POST /cash/sessions — ouverture de session de caisse.
func (a *App) handleCashSessionOpen(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()

	var body struct {
		StationID      string `json:"station_id"`
		OpeningFloatXOF int   `json:"opening_float_xof"`
		Note           string `json:"note"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	stationID := body.StationID
	if stationID == "" {
		stationID = deref(user.StationID)
	}

	// Une seule session ouverte par station.
	var existing string
	err := a.Pool.QueryRow(ctx, `
		SELECT id FROM cash_sessions WHERE station_id = $1 AND status = 'open' LIMIT 1`,
		stationID).Scan(&existing)
	if err == nil {
		writeError(w, http.StatusConflict, "conflict",
			"Une session de caisse est déjà ouverte pour cette station.", nil)
		return
	}

	today := businessDay(time.Now())
	var seq int
	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM cash_sessions WHERE station_id = $1 AND business_day = $2`,
		stationID, today).Scan(&seq)
	reference := fmt.Sprintf("CAISSE-%s-%03d", today, seq+1)

	id := newID("cs")
	_, err = a.Pool.Exec(ctx, `
		INSERT INTO cash_sessions (id, organization_id, station_id, reference, business_day,
			opening_float_xof, expected_cash_xof, status, opened_by, opened_by_name, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$6,'open',$7,$8,$9)`,
		id, deref(user.OrganizationID), stationID, reference, today,
		body.OpeningFloatXOF, user.ID, user.FullName, body.Note)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'ouverture de caisse.", nil)
		return
	}

	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"cash_session.opened", "cash_session", id,
		map[string]any{"opening_float_xof": body.OpeningFloatXOF})

	row := a.Pool.QueryRow(ctx,
		`SELECT `+cashSessionColumns+` FROM cash_sessions WHERE id = $1`, id)
	session, _, _ := scanCashSessionWithOrg(row)
	writeJSON(w, http.StatusCreated, cashSessionToJSON(session))
}

// POST /cash/sessions/:id/close — clôture avec comptage des espèces.
func (a *App) handleCashSessionClose(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")

	session, err := a.getOwnedCashSession(ctx, id, deref(user.OrganizationID))
	if err != nil || session == nil {
		writeError(w, http.StatusNotFound, "not_found", "Session introuvable.", nil)
		return
	}
	if session.Status != "open" {
		writeError(w, http.StatusConflict, "conflict", "Cette session est déjà clôturée.", nil)
		return
	}

	var body struct {
		CountedCashXOF int    `json:"counted_cash_xof"`
		Note           string `json:"note"`
	}
	_ = decodeBody(r, &body)

	// Le comptage déclaré remplace le calcul théorique s'il est fourni.
	counted := body.CountedCashXOF
	if counted == 0 {
		counted = session.ExpectedCashXOF
	}
	variance := counted - session.ExpectedCashXOF

	_, err = a.Pool.Exec(ctx, `
		UPDATE cash_sessions
		SET status = 'closed', closed_at = now(), closed_by = $1, closed_by_name = $2,
		    counted_cash_xof = $3, difference_xof = $4,
		    close_notes = CASE WHEN $5 = '' THEN close_notes ELSE $5 END
		WHERE id = $6`,
		user.ID, user.FullName, counted, variance, body.Note, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la clôture.", nil)
		return
	}

	details := map[string]any{
		"counted_cash_xof":  counted,
		"expected_cash_xof": session.ExpectedCashXOF,
		"difference_xof":    variance,
	}
	if variance != 0 {
		a.addAudit(ctx, session.OrganizationID, session.StationID, user.FullName,
			"cash_variance.flagged", "cash_session", id, details)
	}
	a.addAudit(ctx, session.OrganizationID, session.StationID, user.FullName,
		"cash_session.closed", "cash_session", id, details)

	row := a.Pool.QueryRow(ctx,
		`SELECT `+cashSessionColumns+` FROM cash_sessions WHERE id = $1`, id)
	updated, _, _ := scanCashSessionWithOrg(row)
	writeJSON(w, http.StatusOK, cashSessionToJSON(updated))
}

// POST /cash/sessions/:id/reopen — réouverture par le gérant / propriétaire.
func (a *App) handleCashSessionReopen(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user.Role == "cashier" || user.Role == "washer" {
		writeError(w, http.StatusForbidden, "forbidden",
			"La réouverture d'une caisse nécessite les droits du gérant.", nil)
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")

	session, err := a.getOwnedCashSession(ctx, id, deref(user.OrganizationID))
	if err != nil || session == nil {
		writeError(w, http.StatusNotFound, "not_found", "Session introuvable.", nil)
		return
	}
	if session.Status == "open" {
		writeError(w, http.StatusConflict, "conflict", "Cette session est déjà ouverte.", nil)
		return
	}

	_, err = a.Pool.Exec(ctx, `
		UPDATE cash_sessions SET status = 'open', closed_at = NULL, closed_by = NULL,
		    closed_by_name = NULL, counted_cash_xof = NULL, difference_xof = NULL,
		    reopen_count = reopen_count + 1
		WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la réouverture.", nil)
		return
	}

	a.addAudit(ctx, session.OrganizationID, session.StationID, user.FullName,
		"cash_session.reopened", "cash_session", id, map[string]any{"actor_role": user.Role})

	row := a.Pool.QueryRow(ctx,
		`SELECT `+cashSessionColumns+` FROM cash_sessions WHERE id = $1`, id)
	updated, _, _ := scanCashSessionWithOrg(row)
	writeJSON(w, http.StatusOK, cashSessionToJSON(updated))
}
