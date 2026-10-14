package main

import (
	"net/http"
	"strconv"
	"time"
)

// ---------------------------------------------------------------------------
// Handlers manquants : expenses, reports, commissions, platform, washer.
// Implémentés pour compléter les routes déclarées dans main.go.
// ---------------------------------------------------------------------------

// ----------------------------- Dépenses ------------------------------------

func (a *App) handleExpensesList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	limit := queryInt(r, "limit", 100)
	stationID, err := a.stationDeTravail(ctx, user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	status := r.URL.Query().Get("status")
	rows, err := a.Pool.Query(ctx, `
SELECT id, organization_id, station_id, category, label, amount_xof, method,
       supplier, reference, note, spent_at, business_day, status, void_reason,
       recorded_by_name, created_at
FROM expenses WHERE organization_id = $1
  AND ($3 = '' OR station_id = $3)
  AND ($4 = '' OR status = $4)
ORDER BY spent_at DESC LIMIT $2`, deref(user.OrganizationID), limit, stationID, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, orgID, stationID, category, label, method, supplier, reference, note, businessDay, status, recordedByName string
		var amount int
		var voidReason *string
		var spentAt, createdAt time.Time
		if err := rows.Scan(&id, &orgID, &stationID, &category, &label, &amount, &method,
			&supplier, &reference, &note, &spentAt, &businessDay, &status, &voidReason,
			&recordedByName, &createdAt); err == nil {
			list = append(list, map[string]any{
				"id": id, "organization_id": orgID, "station_id": stationID,
				"category": category, "label": label, "amount_xof": amount, "method": method,
				"supplier": supplier, "reference": reference, "note": note,
				"spent_at": spentAt, "business_day": businessDay, "status": status,
				"void_reason": voidReason, "recorded_by_name": recordedByName,
				"created_at": createdAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"expenses": list})
}

func (a *App) handleExpenseCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	var body struct {
		StationID     string `json:"station_id"`
		Category      string `json:"category"`
		Label         string `json:"label"`
		AmountXOF     int    `json:"amount_xof"`
		Method        string `json:"method"`
		Supplier      string `json:"supplier"`
		Reference     string `json:"reference"`
		Note          string `json:"note"`
		CashSessionID string `json:"cash_session_id"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if body.Label == "" || body.AmountXOF <= 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "Libellé et montant positif requis.", nil)
		return
	}
	// La station est vérifiée : une dépense ne s'impute pas chez une autre
	// entreprise, ni dans une station hors de son périmètre.
	stationID, err := a.resolveStation(ctx, user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if body.Method == "" {
		body.Method = "especes"
	}
	if body.Category == "" {
		body.Category = "autre"
	}

	// Une dépense en espèces touche la caisse : elle doit suivre la même règle
	// que les encaissements, et alimenter la session ouverte.
	sessionID := nullIfEmpty(body.CashSessionID)
	if body.Method == "especes" {
		openSession, err := a.requireOpenCashSession(r, stationID, "especes")
		if err != nil {
			writeAPIError(w, err)
			return
		}
		if sessionID == nil {
			sessionID = openSession
		}
	}

	id := newID("exp")
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
INSERT INTO expenses (id, organization_id, station_id, category, label, amount_xof,
	method, supplier, reference, note, business_day, cash_session_id,
	recorded_by, recorded_by_name)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		id, deref(user.OrganizationID), stationID, body.Category, body.Label, body.AmountXOF,
		body.Method, body.Supplier, body.Reference, body.Note, businessDay(time.Now()),
		sessionID, user.ID, user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement de la dépense.", nil)
		return
	}

	// Sortie de caisse : le solde théorique suit la dépense.
	if sessionID != nil && body.Method == "especes" {
		if _, err := tx.Exec(ctx, `
			UPDATE cash_sessions
			SET expenses_xof = expenses_xof + $1,
			    expected_cash_xof = opening_float_xof + cash_in_xof - (expenses_xof + $1),
			    expense_count = expense_count + 1,
			    operations = operations + 1
			WHERE id = $2 AND organization_id = $3`,
			body.AmountXOF, *sessionID, deref(user.OrganizationID)); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur de caisse.", nil)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), stationID, user.FullName,
		"expense.created", "expense", id, map[string]any{"amount_xof": body.AmountXOF})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "valid"})
}

func (a *App) handleExpenseUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var body struct {
		Category  *string `json:"category"`
		Label     *string `json:"label"`
		AmountXOF *int    `json:"amount_xof"`
		Supplier  *string `json:"supplier"`
		Reference *string `json:"reference"`
		Note      *string `json:"note"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	res, err := a.Pool.Exec(ctx, `
UPDATE expenses SET
  category  = COALESCE($2, category),
  label     = COALESCE($3, label),
  amount_xof = COALESCE($4, amount_xof),
  supplier  = COALESCE($5, supplier),
  reference = COALESCE($6, reference),
  note      = COALESCE($7, note)
WHERE id = $1 AND organization_id = $8 AND status = 'valid'`,
		id, body.Category, body.Label, body.AmountXOF, body.Supplier, body.Reference, body.Note,
		deref(user.OrganizationID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if res.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Dépense introuvable.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"expense.updated", "expense", id, map[string]any{"actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleExpenseVoid(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = decodeBody(r, &body)
	if body.Reason == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Motif d'annulation requis.", nil)
		return
	}
	// Annulation + contrepassation sur la session de caisse concernée.
	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer tx.Rollback(ctx)

	var amount int
	var method string
	var sessionID *string
	if err := tx.QueryRow(ctx, `
		SELECT amount_xof, method, cash_session_id FROM expenses
		WHERE id = $1 AND organization_id = $2 AND status = 'valid'`,
		id, deref(user.OrganizationID)).Scan(&amount, &method, &sessionID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Dépense introuvable.", nil)
		return
	}

	if _, err := tx.Exec(ctx, `
UPDATE expenses SET status = 'voided', void_reason = $2
WHERE id = $1 AND organization_id = $3 AND status = 'valid'`,
		id, body.Reason, deref(user.OrganizationID)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if sessionID != nil && method == "especes" {
		if _, err := tx.Exec(ctx, `
			UPDATE cash_sessions
			SET expenses_xof = GREATEST(expenses_xof - $1, 0),
			    expected_cash_xof = opening_float_xof + cash_in_xof - GREATEST(expenses_xof - $1, 0),
			    expense_count = GREATEST(expense_count - 1, 0),
			    operations = GREATEST(operations - 1, 0)
			WHERE id = $2`, amount, *sessionID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur de caisse.", nil)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"expense.voided", "expense", id, map[string]any{"reason": body.Reason})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ----------------------------- Utilitaires ---------------------------------

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// joinStrings : équivalent local de strings.Join (évite un import de plus).
func joinStrings(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
