package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
// Limites de la formule : stations, comptes, lavages du mois.
//
// L'écran Abonnement affiche ces plafonds ; le serveur les applique, sans quoi
// la formule Essai (une station, trois comptes) ne veut rien dire.
// ---------------------------------------------------------------------------

type planLimit int

const (
	limitStations planLimit = iota
	limitEmployees
	limitWashOrdersMonth
)

// planQuota : plafond de la formule en cours pour une entreprise.
type planQuota struct {
	PlanName     string
	MaxStations  int
	MaxEmployees int
	MaxWashes    int
	StaffLimit   int
	TrialEndsAt  *time.Time
	Status       string
}

// loadQuota lit l'abonnement de l'entreprise. Un quota vide (Status == "")
// signifie « aucune formule » : les limites ne sont pas appliquées.
func (a *App) loadQuota(ctx context.Context, orgID string) (planQuota, error) {
	var q planQuota
	if orgID == "" {
		return q, nil
	}
	err := a.Pool.QueryRow(ctx, `
		SELECT p.name, p.max_stations, p.max_employees, p.max_wash_orders_month,
		       COALESCE(p.features->>'staff_limit','0')::int, s.trial_ends_at, s.status
		FROM subscriptions s
		JOIN plans p ON p.id = s.plan_id
		WHERE s.organization_id = $1
		ORDER BY s.started_at DESC LIMIT 1`, orgID,
	).Scan(&q.PlanName, &q.MaxStations, &q.MaxEmployees, &q.MaxWashes,
		&q.StaffLimit, &q.TrialEndsAt, &q.Status)
	if err != nil {
		return planQuota{}, nil // pas d'abonnement : pas de limite connue
	}
	return q, nil
}

// planLimitError construit le refus, avec la sortie de secours indiquée.
func planLimitError(message string) *apiError {
	return &apiError{
		Status:  http.StatusForbidden,
		Code:    "plan_limit",
		Message: message,
		Fields:  map[string]string{"plan": "Limite de la formule atteinte"},
	}
}

// checkPlanLimit refuse une création qui dépasserait la formule en cours.
func (a *App) checkPlanLimit(r *http.Request, user *User, kind planLimit) error {
	return a.checkPlanLimitForOrg(r.Context(), deref(user.OrganizationID), kind)
}

// checkPlanLimitForOrg : même contrôle, pour une entreprise sans session — le
// cas de l'inscription par invitation, qui rattache un compte de plus à une
// entreprise existante et doit donc respecter le plafond de sa formule.
func (a *App) checkPlanLimitForOrg(ctx context.Context, orgID string, kind planLimit) error {
	quota, err := a.loadQuota(ctx, orgID)
	if err != nil || quota.Status == "" {
		return nil
	}
	if quota.Status == "trialing" && quota.TrialEndsAt != nil && quota.TrialEndsAt.Before(time.Now()) {
		return planLimitError(fmt.Sprintf(
			"Votre période d'essai (%s) est terminée. Rendez-vous sur l'écran Abonnement pour choisir une formule, "+
				"ou contactez le support WashGuard.", quota.PlanName))
	}
	if quota.Status == "expired" || quota.Status == "suspended" {
		return planLimitError("Votre abonnement n'est plus actif. Contactez le support WashGuard pour le réactiver.")
	}

	switch kind {
	case limitStations:
		if quota.MaxStations <= 0 {
			return nil
		}
		var count int
		_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM stations WHERE organization_id = $1 AND is_active`, orgID).Scan(&count)
		if count >= quota.MaxStations {
			return planLimitError(fmt.Sprintf(
				"La formule %s permet %d station(s) active(s). Passez à une formule supérieure depuis l'écran Abonnement.",
				quota.PlanName, quota.MaxStations))
		}
	case limitEmployees:
		if quota.MaxEmployees <= 0 {
			return nil
		}
		var count int
		_ = a.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE organization_id = $1 AND is_active`, orgID).Scan(&count)
		if count >= quota.MaxEmployees {
			return planLimitError(fmt.Sprintf(
				"La formule %s permet %d compte(s) actif(s). Passez à une formule supérieure depuis l'écran Abonnement.",
				quota.PlanName, quota.MaxEmployees))
		}
	case limitWashOrdersMonth:
		if quota.MaxWashes <= 0 {
			return nil
		}
		month := time.Now().Format("2006-01")
		var count int
		_ = a.Pool.QueryRow(ctx,
			`SELECT count(*) FROM wash_orders WHERE organization_id = $1 AND business_day >= $2`,
			orgID, month+"-01").Scan(&count)
		if count >= quota.MaxWashes {
			return planLimitError(fmt.Sprintf(
				"La formule %s autorise %d lavages par mois. Passez à une formule supérieure depuis l'écran Abonnement.",
				quota.PlanName, quota.MaxWashes))
		}
	}
	return nil
}
