package main

import (
	"context"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Espace laveur : mes tâches, mise à jour de l'avancement, signalement d'un
// incident, mes indicateurs.
//
// Toutes les requêtes sont limitées aux fiches du compte connecté et à son
// entreprise. Le laveur ne voit que son propre travail.
// ---------------------------------------------------------------------------

// GET /washer/tasks — les fiches attribuées au laveur connecté.
func (a *App) handleWasherTasks(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+washOrderColumnsAliased+`, COALESCE(p.paid, 0)
		FROM wash_orders o
		LEFT JOIN LATERAL (
			SELECT sum(amount_xof) AS paid FROM payments
			WHERE wash_order_id = o.id AND status = 'valid'
		) p ON TRUE
		WHERE o.organization_id = $1 AND o.employee_id = $2
		ORDER BY (o.status IN ('pending','in_progress')) DESC, o.created_at DESC
		LIMIT 100`, deref(user.OrganizationID), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()

	items := []map[string]any{}
	for rows.Next() {
		var o WashOrder
		var paid int
		dest := append(scanWashOrderDest(&o), &paid)
		if err := rows.Scan(dest...); err != nil {
			continue
		}
		items = append(items, orderToJSON(&o, paid, a.loadItems(r, o.ID, deref(user.OrganizationID)), "", nil))
	}
	// `items` est le nom attendu par l'interface ; `tasks` reste exposé.
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "tasks": items, "total": len(items)})
}

// POST /washer/tasks/{id}/status — avancement d'une fiche.
func (a *App) handleWasherTaskStatus(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")

	var body struct {
		Status string `json:"status"`
	}
	if err := decodeBody(r, &body); err != nil || body.Status == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Statut requis.", nil)
		return
	}
	// Le laveur ouvre son lavage, le déclare fini (contrôle qualité) ou le
	// clôt : il n'annule pas (rôle du gérant), et il ne déclare jamais une
	// fiche litigieuse.
	valid := map[string]bool{"in_progress": true, "quality_check": true, "completed": true}
	if !valid[body.Status] {
		writeError(w, http.StatusBadRequest, "validation_error", "Statut non autorisé.",
			map[string]string{"status": "Valeurs acceptées : in_progress, quality_check, completed"})
		return
	}

	// La fiche doit être attribuée au laveur connecté, dans son entreprise.
	var currentStatus string
	if err := a.Pool.QueryRow(ctx, `
		SELECT status FROM wash_orders
		WHERE id = $1 AND organization_id = $2 AND employee_id = $3`,
		id, deref(user.OrganizationID), user.ID).Scan(&currentStatus); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}
	if currentStatus == "canceled" || currentStatus == "completed" {
		writeError(w, http.StatusConflict, "conflict",
			"Cette fiche est déjà close : contactez le gérant pour la corriger.", nil)
		return
	}
	// Le cycle de vie s'applique aussi au portail laveur : on ne saute pas du
	// contrôle qualité à l'attente, et on ne recule pas depuis une fiche prête.
	if !transitionAutorisee(currentStatus, body.Status) {
		writeError(w, http.StatusConflict, "transition_forbidden",
			"Une fiche « "+libelleStatutLower(currentStatus)+" » ne peut pas passer à « "+
				libelleStatutLower(body.Status)+" ».", map[string]string{"status": "Transition interdite"})
		return
	}

	// Terminer un lavage suppose qu'il est encaissé : le laveur marque son
	// travail fini, la caisse reste maîtresse du solde.
	if body.Status == "completed" {
		_, _, due, err := a.soldeFiche(r, id, deref(user.OrganizationID))
		if err != nil {
			writeAPIError(w, err)
			return
		}
		if due > 0 {
			writeError(w, http.StatusConflict, "conflict",
				"Le lavage n'est pas encore encaissé : prévenez la caisse avant de le terminer.", nil)
			return
		}
	}

	// Chaque transition est écrite explicitement (aucun paramètre inutilisé :
	// une clause trop générique produisait une erreur de liaison côté base).
	orgID := deref(user.OrganizationID)
	var err error
	switch body.Status {
	case "in_progress":
		_, err = a.Pool.Exec(ctx, `
			UPDATE wash_orders SET status = 'in_progress',
			       started_at = COALESCE(started_at, now())
			WHERE id = $1 AND organization_id = $2 AND employee_id = $3 AND status = 'pending'`,
			id, orgID, user.ID)
	case "quality_check":
		_, err = a.Pool.Exec(ctx, `
			UPDATE wash_orders SET status = 'quality_check', quality_checked_at = now()
			WHERE id = $1 AND organization_id = $2 AND employee_id = $3`,
			id, orgID, user.ID)
	case "completed":
		_, err = a.Pool.Exec(ctx, `
			UPDATE wash_orders SET status = 'completed',
			       completed_at = COALESCE(completed_at, now()),
			       delivered_at = COALESCE(delivered_at, now())
			WHERE id = $1 AND organization_id = $2 AND employee_id = $3 AND status <> 'canceled'`,
			id, orgID, user.ID)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(ctx, deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"washer.task_status", "wash_order", id, map[string]any{"status": body.Status})

	// Réponse : la fiche à jour, pour que l'écran n'ait pas à recharger.
	row := a.Pool.QueryRow(ctx, `
		SELECT `+washOrderColumns+` FROM wash_orders
		WHERE id = $1 AND organization_id = $2`, id, deref(user.OrganizationID))
	updated, err := scanWashOrder(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	paid := a.paidForOrder(r, id, updated.OrganizationID)
	writeJSON(w, http.StatusOK, orderToJSON(updated, paid, a.loadItems(r, id, deref(user.OrganizationID)), "", nil))
}

// POST /washer/tasks/{id}/incident — signaler un problème sur une fiche.
func (a *App) handleWasherTaskIncident(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	id := r.PathValue("id")

	var body struct {
		IncidentNote string `json:"incident_note"`
		Description  string `json:"description"` // ancien nom, toléré
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	note := body.IncidentNote
	if note == "" {
		note = body.Description
	}
	if note == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Description requise.",
			map[string]string{"incident_note": "Décrivez le problème"})
		return
	}

	// La note est enregistrée sur la fiche : elle doit être lisible ensuite par
	// le gérant, pas seulement tracée dans le journal d'audit.
	res, err := a.Pool.Exec(ctx, `
		UPDATE wash_orders
		SET incident_note = $1, incident_reported_at = now(), incident_reporter_name = $2
		WHERE id = $3 AND organization_id = $4 AND employee_id = $5`,
		note, user.FullName, id, deref(user.OrganizationID), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if res.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Fiche introuvable.", nil)
		return
	}

	a.addAudit(ctx, deref(user.OrganizationID), deref(user.StationID), user.FullName,
		"washer.incident_reported", "wash_order", id, map[string]any{"incident_note": note})

	row := a.Pool.QueryRow(ctx, `
		SELECT `+washOrderColumns+` FROM wash_orders
		WHERE id = $1 AND organization_id = $2`, id, deref(user.OrganizationID))
	updated, err := scanWashOrder(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	paid := a.paidForOrder(r, id, updated.OrganizationID)
	writeJSON(w, http.StatusCreated, orderToJSON(updated, paid, a.loadItems(r, id, deref(user.OrganizationID)), "", nil))
}

// GET /washer/me/performance — mes indicateurs personnels.
func (a *App) handleWasherPerformance(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	ctx := r.Context()
	orgID := deref(user.OrganizationID)
	today := businessDay(time.Now())
	month := today[:8] + "01"

	var washesToday, washesMonth int
	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM wash_orders
		WHERE employee_id = $1 AND organization_id = $2
		  AND business_day = $3 AND status = 'completed'`,
		user.ID, orgID, today).Scan(&washesToday)
	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM wash_orders
		WHERE employee_id = $1 AND organization_id = $2
		  AND business_day >= $3 AND status = 'completed'`,
		user.ID, orgID, month).Scan(&washesMonth)

	// Commission du mois : barème de la station, ou exception du compte.
	rate := commissionRate{Mode: "none"}
	if stationID := deref(user.StationID); stationID != "" {
		rate = a.employeeCommissionRate(ctx, a.stationCommissionSettings(ctx, stationID), user.ID)
	}
	var revenueToday, revenueMonth int
	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(total_xof), 0) FROM wash_orders
		WHERE employee_id = $1 AND organization_id = $2
		  AND business_day = $3 AND status = 'completed'`, user.ID, orgID, today).Scan(&revenueToday)
	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(total_xof), 0) FROM wash_orders
		WHERE employee_id = $1 AND organization_id = $2
		  AND business_day >= $3 AND status = 'completed'`, user.ID, orgID, month).Scan(&revenueMonth)

	writeJSON(w, http.StatusOK, map[string]any{
		"washes_today":          washesToday,
		"washes_month":          washesMonth,
		"commission_today_xof":  commissionAmount(rate, washesToday, revenueToday),
		"commission_month_xof":  commissionAmount(rate, washesMonth, revenueMonth),
		"commission_rate_label": commissionRateLabel(rate),
		"mode":                  rate.Mode,
		"per_wash_xof":          rate.PerWash,
		"percent":               rate.Percent,
		// Anciens noms conservés pour ne casser aucun appelant.
		"today_completed": washesToday,
		"total_completed": washesMonth,
	})
}

// openSessionIDOf : identifiant de la session ouverte d'une station, ou nil.
func (a *App) openSessionIDOf(ctx context.Context, stationID string) *string {
	var id *string
	_ = a.Pool.QueryRow(ctx, `
		SELECT id FROM cash_sessions
		WHERE station_id = $1 AND status = 'open'
		ORDER BY opened_at DESC LIMIT 1`, stationID).Scan(&id)
	return id
}
