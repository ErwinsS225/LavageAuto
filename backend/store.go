package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Accès aux données : organisations, stations, abonnements.
// ---------------------------------------------------------------------------

func (a *App) getOrganization(ctx context.Context, id string) (*Organization, error) {
	var o Organization
	var createdAt time.Time
	err := a.Pool.QueryRow(ctx, `
		SELECT id, name, slug, email, phone, address, city, country, currency, timezone,
		       logo_url, status, notes, created_at
		FROM organizations WHERE id = $1`, id,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.Email, &o.Phone, &o.Address, &o.City, &o.Country,
		&o.Currency, &o.Timezone, &o.LogoURL, &o.Status, &o.Notes, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o.CreatedAt = createdAt
	return &o, nil
}

func (a *App) getStation(ctx context.Context, id string) (*Station, error) {
	var s Station
	var openingRaw, settingsRaw []byte
	var createdAt time.Time
	err := a.Pool.QueryRow(ctx, `
		SELECT id, organization_id, name, code, phone, address, city, timezone, currency,
		       logo_url, opening_hours, settings, is_default, is_active, created_at
		FROM stations WHERE id = $1`, id,
	).Scan(&s.ID, &s.OrganizationID, &s.Name, &s.Code, &s.Phone, &s.Address, &s.City,
		&s.Timezone, &s.Currency, &s.LogoURL, &openingRaw, &settingsRaw, &s.IsDefault, &s.IsActive, &createdAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.CreatedAt = createdAt
	s.OpeningHours = map[string]any{}
	s.Settings = map[string]any{}
	parseJSONB(openingRaw, &s.OpeningHours)
	parseJSONB(settingsRaw, &s.Settings)
	return &s, nil
}


func (a *App) defaultStationID(ctx context.Context, orgID string) (string, error) {
	var id string
	err := a.Pool.QueryRow(ctx, `
		SELECT id FROM stations WHERE organization_id = $1 ORDER BY is_default DESC, created_at LIMIT 1`,
		orgID).Scan(&id)
	return id, err
}

// buildSubscriptionPayload : abonnement + plan + consommation du mois.
func (a *App) buildSubscriptionPayload(ctx context.Context, orgID string) (map[string]any, error) {
	var sub Subscription
	var plan Plan
	var featuresRaw []byte

	err := a.Pool.QueryRow(ctx, `
		SELECT s.id, s.organization_id, s.plan_id, s.status, s.started_at,
		       s.trial_ends_at, s.current_period_start, s.current_period_end,
		       p.id, p.code, p.name, p.description, p.price_xof, p.billing_period,
		       p.max_stations, p.max_employees, p.max_wash_orders_month, p.features
		FROM subscriptions s
		JOIN plans p ON p.id = s.plan_id
		WHERE s.organization_id = $1`, orgID,
	).Scan(&sub.ID, &sub.OrganizationID, &sub.PlanID, &sub.Status, &sub.StartedAt,
		&sub.TrialEndsAt, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&plan.ID, &plan.Code, &plan.Name, &plan.Description, &plan.PriceXOF, &plan.BillingPeriod,
		&plan.MaxStations, &plan.MaxEmployees, &plan.MaxWashOrdersMonth, &featuresRaw)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plan.Features = map[string]any{}
	parseJSONB(featuresRaw, &plan.Features)

	statusLabel := sub.Status
	switch sub.Status {
	case "active":
		statusLabel = "Actif"
	case "trialing":
		statusLabel = "Période d'essai"
	}

	// Consommation courante.
	var washCount, stationCount, employeeCount int
	_ = a.Pool.QueryRow(ctx, `
		SELECT count(*) FROM wash_orders WHERE organization_id = $1 AND business_day >= to_char(date_trunc('month', now()), 'YYYY-MM-DD')`,
		orgID).Scan(&washCount)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM stations WHERE organization_id = $1 AND is_active`, orgID).Scan(&stationCount)
	_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE organization_id = $1 AND is_active`, orgID).Scan(&employeeCount)

	period := ""
	_ = a.Pool.QueryRow(ctx, `SELECT to_char(now(), 'YYYY-MM')`).Scan(&period)

	return map[string]any{
		"plan_id":              plan.ID,
		"plan_code":            plan.Code,
		"plan_name":            plan.Name,
		"plan_description":     plan.Description,
		"status":               sub.Status,
		"status_label":         statusLabel,
		"price_xof":            plan.PriceXOF,
		"billing_period":       plan.BillingPeriod,
		"trial_ends_at":        sub.TrialEndsAt,
		"current_period_start": sub.CurrentPeriodStart,
		"current_period_end":   sub.CurrentPeriodEnd,
		"max_stations":         plan.MaxStations,
		"max_employees":        plan.MaxEmployees,
		"max_wash_orders_month": plan.MaxWashOrdersMonth,
		"features":             plan.Features,
		"usage": map[string]any{
			"period":      period,
			"wash_orders": washCount,
			"stations":    stationCount,
			"employees":   employeeCount,
		},
	}, nil
}

// addAudit insère une entrée au journal d'audit.
func (a *App) addAudit(ctx context.Context, orgID, stationID, actor, action, entity, entityID string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	detailsJSON, _ := json.Marshal(details)
	var orgPtr, stPtr *string
	if orgID != "" {
		orgPtr = &orgID
	}
	if stationID != "" {
		stPtr = &stationID
	}
	_, _ = a.Pool.Exec(ctx, `
		INSERT INTO audit_logs (organization_id, station_id, actor_name, action, entity, entity_id, details)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`,
		orgPtr, stPtr, actor, action, entity, entityID, string(detailsJSON))
}
