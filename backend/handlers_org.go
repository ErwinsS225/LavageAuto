package main

import (
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Organisations, abonnements, audit.
// ---------------------------------------------------------------------------

func (a *App) handleListPlansPublic(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": a.listPlans(r)})
}

// listPlans retourne la liste des plans actifs.
func (a *App) listPlans(r *http.Request) []map[string]any {
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, code, name, description, price_xof, billing_period,
		       max_stations, max_employees, max_wash_orders_month, trial_days,
		       features, sort_order, is_active
		FROM plans WHERE is_active ORDER BY sort_order`)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var p Plan
		var featuresRaw []byte
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.PriceXOF, &p.BillingPeriod,
			&p.MaxStations, &p.MaxEmployees, &p.MaxWashOrdersMonth, &p.TrialDays,
			&featuresRaw, &p.SortOrder, &p.IsActive); err != nil {
			continue
		}
		p.Features = map[string]any{}
		parseJSONB(featuresRaw, &p.Features)
		items = append(items, map[string]any{
			"id": p.ID, "code": p.Code, "name": p.Name, "description": p.Description,
			"price_xof": p.PriceXOF, "billing_period": p.BillingPeriod,
			"max_stations": p.MaxStations, "max_employees": p.MaxEmployees,
			"max_wash_orders_month": p.MaxWashOrdersMonth, "trial_days": p.TrialDays,
			"features": p.Features, "sort_order": p.SortOrder, "is_active": p.IsActive,
		})
	}
	return items
}

func (a *App) handleOrgCurrent(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user.OrganizationID == nil {
		writeError(w, http.StatusNotFound, "not_found", "Organisation introuvable.", nil)
		return
	}
	org, err := a.getOrganization(r.Context(), *user.OrganizationID)
	if err != nil || org == nil {
		writeError(w, http.StatusNotFound, "not_found", "Organisation introuvable.", nil)
		return
	}
	var stations, employees, customers int
	_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM stations WHERE organization_id = $1`, org.ID).Scan(&stations)
	_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE organization_id = $1`, org.ID).Scan(&employees)
	_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM customers WHERE organization_id = $1`, org.ID).Scan(&customers)

	writeJSON(w, http.StatusOK, map[string]any{
		"organization": org,
		"counts": map[string]any{
			"stations":  stations,
			"employees": employees,
			"customers": customers,
		},
	})
}

func (a *App) handleOrgUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user.OrganizationID == nil {
		writeError(w, http.StatusNotFound, "not_found", "Organisation introuvable.", nil)
		return
	}
	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	allowed := map[string]string{
		"name": "name", "email": "email", "phone": "phone", "address": "address",
		"city": "city", "logo_url": "logo_url", "notes": "notes",
	}
	for field, column := range allowed {
		if v, ok := body[field]; ok {
			_, _ = a.Pool.Exec(r.Context(),
				`UPDATE organizations SET `+column+` = $1 WHERE id = $2`, v, *user.OrganizationID)
		}
	}
	a.addAudit(r.Context(), *user.OrganizationID, "", user.FullName, "organization.updated", "organization", *user.OrganizationID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Organisation mise à jour."})
}

func (a *App) handleOrgSubscription(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user.OrganizationID == nil {
		writeJSON(w, http.StatusOK, map[string]any{"subscription": nil, "plans": []any{}, "payment_note": ""})
		return
	}
	sub, err := a.buildSubscriptionPayload(r.Context(), *user.OrganizationID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"subscription": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription": sub,
		"plans":        a.listPlans(r),
		"payment_note": "Paiement sécurisé par Orange Money, Wave ou Virement bancaire.",
	})
}

func (a *App) handleSubscriptionRequest(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	a.addAudit(r.Context(), orgID, "", user.FullName, "subscription.requested", "subscription", orgID, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Votre demande a bien été transmise à notre équipe."})
}

func (a *App) handleOrgAudit(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	rows, err := a.Pool.Query(r.Context(), `
		SELECT id, organization_id, station_id, actor_name, action, entity, entity_id, details, created_at
		FROM audit_logs
		WHERE ($1::text IS NULL OR organization_id = $1)
		ORDER BY created_at DESC LIMIT 500`, user.OrganizationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := scanAuditRows(rows)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// stationToJSON convertit un Station struct en map sérialisable.
func stationToJSON(s *Station) map[string]any {
	if s == nil {
		return nil
	}
	return map[string]any{
		"id": s.ID, "organization_id": s.OrganizationID, "name": s.Name, "code": s.Code,
		"phone": s.Phone, "address": s.Address, "city": s.City, "timezone": s.Timezone,
		"currency": s.Currency, "logo_url": s.LogoURL, "opening_hours": s.OpeningHours,
		"settings": s.Settings, "is_default": s.IsDefault, "is_active": s.IsActive,
		"created_at": s.CreatedAt.Format(time.RFC3339),
	}
}
