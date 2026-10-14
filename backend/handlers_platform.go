package main

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Espace plateforme (support WashGuard) : vue d'ensemble, entreprises,
// formules, journal d'audit, abonnements.
//
// Ces routes sont réservées au rôle superadmin, à la fois par la route
// (permission platform:manage) et par requireSuperadmin ci-dessous.
// ---------------------------------------------------------------------------

func (a *App) requireSuperadmin(w http.ResponseWriter, r *http.Request) (*User, bool) {
	user := currentUser(r)
	if user == nil || user.Role != "superadmin" {
		writeError(w, http.StatusForbidden, "forbidden", "Accès réservé à la plateforme.", nil)
		return nil, false
	}
	return user, true
}

const adminOrgSelect = `
	SELECT o.id, o.name, o.slug, o.city, o.phone, o.email, o.status, o.created_at,
	       COALESCE(s.plan_id, ''), COALESCE(p.name, ''),
	       COALESCE(s.status, ''), s.trial_ends_at,
	       (SELECT count(*) FROM stations st WHERE st.organization_id = o.id AND st.is_active),
	       (SELECT count(*) FROM users u WHERE u.organization_id = o.id AND u.is_active),
	       (SELECT count(*) FROM wash_orders w WHERE w.organization_id = o.id AND w.business_day >= $1),
	       COALESCE((SELECT u2.full_name FROM users u2
	                  WHERE u2.organization_id = o.id AND u2.role = 'owner'
	                  ORDER BY u2.created_at LIMIT 1), ''),
	       COALESCE((SELECT u3.email FROM users u3
	                  WHERE u3.organization_id = o.id AND u3.role = 'owner'
	                  ORDER BY u3.created_at LIMIT 1), '')
	FROM organizations o
	LEFT JOIN subscriptions s ON s.organization_id = o.id
	LEFT JOIN plans p ON p.id = s.plan_id`

// scanAdminOrg lit une ligne du sélecteur ci-dessus.
func scanAdminOrg(rows interface {
	Scan(dest ...any) error
}) (map[string]any, error) {
	var id, name, slug, city, phone, email, status string
	var createdAt time.Time
	var planID, planName, subStatus string
	var trialEndsAt *time.Time
	var stations, employees, washes int
	var ownerName, ownerEmail string
	if err := rows.Scan(&id, &name, &slug, &city, &phone, &email, &status, &createdAt,
		&planID, &planName, &subStatus, &trialEndsAt, &stations, &employees, &washes,
		&ownerName, &ownerEmail); err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "name": name, "slug": slug, "city": city, "phone": phone, "email": email,
		"status": status, "status_label": statusLabelFR(status),
		"created_at": createdAt.Format(timeLayout),
		"plan_id":    planID, "plan_name": planName,
		"subscription_status": subStatus, "subscription_status_label": statusLabelFR(subStatus),
		"trial_ends_at": formatTimePtr(trialEndsAt),
		"station_count": stations, "employee_count": employees, "wash_orders_month": washes,
		"owner_name": ownerName, "owner_email": ownerEmail,
	}, nil
}

// GET /platform/overview
func (a *App) handlePlatformOverview(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	ctx := r.Context()
	month := time.Now().Format("2006-01")

	var organizations, activeOrgs, suspendedOrgs, stations, users int
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&organizations)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE status = 'active'`).Scan(&activeOrgs)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE status = 'suspended'`).Scan(&suspendedOrgs)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM stations WHERE is_active`).Scan(&stations)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE is_active`).Scan(&users)

	var washOrdersMonth, revenueMonth int
	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*), COALESCE(sum(total_xof), 0) FROM wash_orders
		WHERE status = 'completed' AND business_day >= $1`, month+"-01",
	).Scan(&washOrdersMonth, &revenueMonth)

	// Revenu récurrent mensuel : somme des formules actives (hors essai).
	var mrr int
	_ = a.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(p.price_xof), 0)
		FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		WHERE s.status = 'active' AND p.code <> 'essai'`).Scan(&mrr)

	plans := []map[string]any{}
	rows, _ := a.Pool.Query(ctx, `
		SELECT p.id, p.name, p.price_xof, count(s.organization_id)
		FROM plans p LEFT JOIN subscriptions s ON s.plan_id = p.id
		GROUP BY p.id, p.name, p.price_xof, p.sort_order
		ORDER BY p.sort_order`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id, name string
			var price, count int
			if rows.Scan(&id, &name, &price, &count) == nil {
				plans = append(plans, map[string]any{
					"plan_id": id, "plan_name": name, "price_xof": price, "count": count,
				})
			}
		}
	}

	recent := []map[string]any{}
	rows, _ = a.Pool.Query(ctx, adminOrgSelect+`
		ORDER BY o.created_at DESC LIMIT 6`, month+"-01")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			if org, err := scanAdminOrg(rows); err == nil {
				recent = append(recent, org)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"organizations": organizations, "active_organizations": activeOrgs,
		"suspended_organizations": suspendedOrgs, "stations": stations, "users": users,
		"wash_orders_month": washOrdersMonth, "revenue_month_xof": revenueMonth,
		"mrr_xof": mrr, "plans": plans, "recent_organizations": recent,
	})
}

// GET /platform/organizations
func (a *App) handlePlatformOrgsList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	month := time.Now().Format("2006-01")
	search := lower(r.URL.Query().Get("search"))
	rows, err := a.Pool.Query(r.Context(), adminOrgSelect+`
		WHERE ($2 = '' OR lower(o.name) LIKE '%' || $2 || '%'
		            OR lower(o.city) LIKE '%' || $2 || '%')
		ORDER BY o.created_at DESC LIMIT 200`, month+"-01", search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		if org, err := scanAdminOrg(rows); err == nil {
			items = append(items, org)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// GET /platform/organizations/{id}
func (a *App) handlePlatformOrgDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	month := time.Now().Format("2006-01")
	rows, err := a.Pool.Query(r.Context(), adminOrgSelect+` WHERE o.id = $2`, month+"-01", r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		writeError(w, http.StatusNotFound, "not_found", "Organisation introuvable.", nil)
		return
	}
	org, err := scanAdminOrg(rows)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Organisation introuvable.", nil)
		return
	}
	writeJSON(w, http.StatusOK, org)
}

// PATCH /platform/organizations/{id} — statut de l'entreprise.
// C'est le seul endroit qui écrit `organizations.status` : suspendre une
// entreprise coupe réellement l'accès (voir organizationAllowed).
func (a *App) handlePlatformOrgUpdate(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireSuperadmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		Name     *string `json:"name"`
		Status   *string `json:"status"`
		IsActive *bool   `json:"is_active"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	ctx := r.Context()

	changed := []string{}
	if body.Name != nil {
		if _, err := a.Pool.Exec(ctx, `UPDATE organizations SET name = $1 WHERE id = $2`, *body.Name, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		changed = append(changed, "name")
	}
	if body.Status != nil {
		switch *body.Status {
		case "active", "suspended", "canceled":
		default:
			writeError(w, http.StatusBadRequest, "validation_error",
				"Statut inconnu.", map[string]string{"status": "Valeurs acceptées : active, suspended, canceled"})
			return
		}
		isActive := *body.Status == "active"
		if _, err := a.Pool.Exec(ctx,
			`UPDATE organizations SET status = $1, is_active = $2 WHERE id = $3`,
			*body.Status, isActive, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		changed = append(changed, "status")
		// Une entreprise suspendue coupe les sessions ouvertes : la décision
		// s'applique immédiatement, pas à la prochaine connexion.
		if *body.Status != "active" {
			_, _ = a.Pool.Exec(ctx, `
				DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE organization_id = $1)`, id)
		}
	}
	if body.IsActive != nil {
		if _, err := a.Pool.Exec(ctx, `UPDATE organizations SET is_active = $1 WHERE id = $2`, *body.IsActive, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		changed = append(changed, "is_active")
	}
	if len(changed) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "Aucun champ à mettre à jour.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), "", user.FullName, "organization.updated", "organization", id,
		map[string]any{"fields": changed, "actor_role": user.Role})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Entreprise mise à jour."})
}

// POST /platform/organizations/{id}/subscription — changer la formule.
func (a *App) handlePlatformOrgSubscription(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireSuperadmin(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	var body struct {
		PlanID string `json:"plan_id"`
	}
	if err := decodeBody(r, &body); err != nil || body.PlanID == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "plan_id requis.", nil)
		return
	}
	ctx := r.Context()
	if _, err := a.Pool.Exec(ctx, `
		INSERT INTO subscriptions (id, organization_id, plan_id, status)
		VALUES ($1,$2,$3,'active')
		ON CONFLICT (organization_id) DO UPDATE
		SET plan_id = EXCLUDED.plan_id, status = 'active', started_at = now()`,
		newID("sub"), id, body.PlanID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(ctx, deref(user.OrganizationID), "", user.FullName,
		"subscription.updated", "subscription", id, map[string]any{"plan_id": body.PlanID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Formule mise à jour."})
}

// GET /platform/plans
func (a *App) handlePlatformPlansList(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	ctx := r.Context()
	counts := map[string]int{}
	rows, err := a.Pool.Query(ctx, `
		SELECT plan_id, count(DISTINCT organization_id) FROM subscriptions
		WHERE plan_id IS NOT NULL GROUP BY plan_id`)
	if err == nil {
		for rows.Next() {
			var planID string
			var n int
			if rows.Scan(&planID, &n) == nil {
				counts[planID] = n
			}
		}
		rows.Close()
	}
	items := []map[string]any{}
	for _, p := range a.listPlans(r) {
		id, _ := p["id"].(string)
		p["organization_count"] = counts[id]
		items = append(items, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// PATCH /platform/plans/{id}
func (a *App) handlePlatformPlanUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	user := currentUser(r)
	var body struct {
		Name          *string         `json:"name"`
		Description   *string         `json:"description"`
		PriceXOF      *int            `json:"price_xof"`
		MaxStations   *int            `json:"max_stations"`
		MaxEmployees  *int            `json:"max_employees"`
		MaxWashOrders *int            `json:"max_wash_orders_month"`
		TrialDays     *int            `json:"trial_days"`
		Features      *map[string]any `json:"features"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	type col struct {
		name  string
		value any
	}
	cols := []col{
		{"name", body.Name},
		{"description", body.Description},
		{"price_xof", body.PriceXOF},
		{"max_stations", body.MaxStations},
		{"max_employees", body.MaxEmployees},
		{"max_wash_orders_month", body.MaxWashOrders},
		{"trial_days", body.TrialDays},
	}
	setClauses := []string{}
	args := []any{id}
	for _, c := range cols {
		if c.value == nil {
			continue
		}
		args = append(args, c.value)
		setClauses = append(setClauses, c.name+" = $"+itoa(len(args)))
	}
	if body.Features != nil {
		args = append(args, jsonOrNull(*body.Features))
		setClauses = append(setClauses, "features = $"+itoa(len(args))+"::jsonb")
	}
	if len(setClauses) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "Aucun champ à mettre à jour.", nil)
		return
	}

	if _, err := a.Pool.Exec(r.Context(),
		`UPDATE plans SET `+joinStrings(setClauses, ", ")+` WHERE id = $1`, args...); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour de la formule.", nil)
		return
	}
	a.addAudit(r.Context(), "", "", user.FullName, "plan.updated", "plan", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Formule mise à jour."})
}

// GET /platform/audit
func (a *App) handlePlatformAudit(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireSuperadmin(w, r); !ok {
		return
	}
	limit := queryInt(r, "limit", 100)
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, organization_id, station_id, actor_name, action, entity, entity_id, details, created_at
		FROM audit_logs ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var orgID, stationID, actor, action, entity, entityID string
		var detailsRaw []byte
		var createdAt time.Time
		if rows.Scan(&id, &orgID, &stationID, &actor, &action, &entity, &entityID, &detailsRaw, &createdAt) != nil {
			continue
		}
		details := map[string]any{}
		parseJSONB(detailsRaw, &details)
		items = append(items, map[string]any{
			"id": id, "organization_id": orgID, "station_id": stationID,
			"actor_name": actor, "action": action, "entity": entity,
			"entity_id": entityID, "details": details,
			"created_at": createdAt.Format(timeLayout),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}
