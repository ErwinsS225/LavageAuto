package main

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// ---------------------------------------------------------------------------
// Seed démo : crée organisation + station + comptes de démonstration au
// premier démarrage (idempotent, basé sur l'adresse e-mail).
// ---------------------------------------------------------------------------

// ensurePlatformAccount crée le compte support de l'espace plateforme.
//
// Le README l'annonce (admin@washguard.app) : sans lui, l'écran
// /administration était inaccessible sur une base neuve. Il n'est jamais créé
// en production, où le compte doit être nominatif et son mot de passe choisi.
func (a *App) ensurePlatformAccount(ctx context.Context) error {
	const email = "admin@washguard.app"
	if a.Cfg.Env == "production" {
		log.Println("seed : compte plateforme non créé (env=production)")
		return nil
	}
	var existe bool
	if err := a.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM users WHERE lower(email) = $1)`, email).Scan(&existe); err != nil {
		return err
	}
	if existe {
		return nil
	}
	hash, err := hashPassword(a.Cfg.PlatformAdminPassword)
	if err != nil {
		return err
	}
	if _, err := a.Pool.Exec(ctx, `
INSERT INTO users (id, organization_id, station_id, full_name, email, password_hash, role, is_active, must_change_password)
VALUES ($1, NULL, NULL, 'Support WashGuard', $2, $3, 'superadmin', true, false)`,
		newID("usr"), email, hash); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return nil
		}
		return fmt.Errorf("seed compte plateforme : %w", err)
	}
	log.Printf("seed : compte plateforme de démonstration %s (mot de passe : %s) — à retirer en production",
		email, a.Cfg.PlatformAdminPassword)
	return nil
}

func (a *App) seedDemo(ctx context.Context) error {
	// Le compte plateforme est indépendant de l'entreprise de démonstration :
	// il est donc créé même sur une base déjà peuplée.
	if err := a.ensurePlatformAccount(ctx); err != nil {
		return err
	}
	var orgCount int
	if err := a.Pool.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&orgCount); err != nil {
		return err
	}
	if orgCount > 0 {
		return nil // déjà peuplé
	}
	log.Println("seed : création des données de démonstration…")

	orgID := newID("org")
	_, err := a.Pool.Exec(ctx, `
INSERT INTO organizations (id, name, slug, phone, city, is_active, onboarding_state)
VALUES ($1,$2,$3,$4,$5,true,'active')`,
		orgID, "Station Démo", "station-demo", "+225 27 00 00 00 00", "Abidjan")
	if err != nil {
		return fmt.Errorf("seed org : %w", err)
	}

	stationID := newID("stn")
	_, err = a.Pool.Exec(ctx, `
INSERT INTO stations (id, organization_id, name, code, city, is_default, is_active, settings)
VALUES ($1,$2,$3,$4,$5,true,true,$6::jsonb)`,
		stationID, orgID, "Station Centrale", "CEN", "Abidjan",
		defaultStationSettings(0))
	if err != nil {
		return fmt.Errorf("seed station : %w", err)
	}

	// Horaires de démonstration : six jours ouverts, dimanche fermé.
	for weekday := 1; weekday <= 7; weekday++ {
		ouvert := weekday <= 6
		ouvre, ferme, pause, finPause, derniere := "08:00", "18:00", "12:00", "13:30", "17:30"
		if weekday == 6 {
			ferme, pause, finPause, derniere = "14:00", "", "", "13:00"
		} else if !ouvert {
			ouvre, ferme, pause, finPause, derniere = "", "", "", "", ""
		}
		if _, err := a.Pool.Exec(ctx, `
INSERT INTO station_opening_hours
  (station_id, weekday, is_open, opens_at, closes_at, break_start, break_end, last_checkin_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (station_id, weekday) DO NOTHING`,
			stationID, weekday, ouvert, ouvre, ferme, pause, finPause, derniere); err != nil {
			return fmt.Errorf("seed horaires : %w", err)
		}
	}

	// Abonnement d'essai : sans lui, les plafonds de la formule et l'écran
	// Abonnement n'auraient rien à afficher.
	_, err = a.Pool.Exec(ctx, `
INSERT INTO subscriptions (id, organization_id, plan_id, status, trial_ends_at, current_period_start, current_period_end)
VALUES ($1,$2,'plan-essai','trialing', now() + interval '14 days', now(), now() + interval '14 days')
ON CONFLICT (organization_id) DO NOTHING`, newID("sub"), orgID)
	if err != nil {
		return fmt.Errorf("seed abonnement : %w", err)
	}

	hash, err := hashPassword("demo1234")
	if err != nil {
		return err
	}
	// Comptes de démonstration (mot de passe : demo1234). Le gérant et le
	// caissier sans station_id seraient hors périmètre : ils sont rattachés à
	// la station de démonstration.
	demoUsers := []struct{ name, email, role string }{
		{"Propriétaire Démo", "owner@demo.local", "owner"},
		{"Gérant Démo", "manager@demo.local", "manager"},
		{"Caissier Démo", "cashier@demo.local", "cashier"},
		{"Laveur Démo", "washer@demo.local", "washer"},
	}
	for _, u := range demoUsers {
		id := newID("usr")
		if _, err := a.Pool.Exec(ctx, `
INSERT INTO users (id, organization_id, station_id, full_name, email, password_hash, role, is_active)
VALUES ($1,$2,$3,$4,$5,$6,$7,true)`,
			id, orgID, stationID, u.name, u.email, hash, u.role); err != nil {
			if strings.Contains(err.Error(), "duplicate") {
				continue
			}
			return fmt.Errorf("seed user %s : %w", u.email, err)
		}
	}

	// Catalogue minimal.
	for _, s := range []struct {
		name  string
		price int
	}{
		{"Lavage simple", 2000}, {"Lavage complet", 5000}, {"Polissage", 10000},
	} {
		_, _ = a.Pool.Exec(ctx, `
INSERT INTO services (id, organization_id, station_id, name, category, price_xof, duration_minutes, sort_order)
VALUES ($1,$2,$3,$4,'lavage',$5,30,1)`, newID("svc"), orgID, stationID, s.name, s.price)
	}

	// Catégories de véhicules et usages : la station de démonstration a activé
	// une partie du référentiel (et déclaré les combinaisons qu'elle propose),
	// sinon sa caisse n'aurait rien à choisir. Ce sont des entrées du
	// référentiel du produit — aucune donnée de marché n'est inventée ici.
	categoriesDemo := []string{"citadine", "berline", "suv", "utilitaire_leger", "moto", "minibus"}
	for i, code := range categoriesDemo {
		_, _ = a.Pool.Exec(ctx, `
INSERT INTO station_vehicle_categories (station_id, code, kind, sort_order, is_active)
VALUES ($1,$2,'modele',$3,true)
ON CONFLICT (station_id, code) DO UPDATE SET is_active = TRUE`,
			stationID, code, (i+1)*10)
	}
	usagesDemo := []string{"personnel", "taxi_compteur", "taxi_communal", "gbaka", "livraison", "flotte", "entreprise"}
	for i, code := range usagesDemo {
		_, _ = a.Pool.Exec(ctx, `
INSERT INTO station_vehicle_usages (station_id, code, kind, sort_order, is_active)
VALUES ($1,$2,'modele',$3,true)
ON CONFLICT (station_id, code) DO UPDATE SET is_active = TRUE`,
			stationID, code, (i+1)*10)
	}
	couplesDemo := [][2]string{
		{"personnel", "citadine"}, {"personnel", "berline"}, {"personnel", "suv"},
		{"taxi_compteur", "berline"}, {"taxi_compteur", "citadine"},
		{"taxi_communal", "berline"},
		{"gbaka", "minibus"},
		{"livraison", "moto"}, {"livraison", "utilitaire_leger"},
		{"flotte", "berline"}, {"flotte", "utilitaire_leger"},
		{"entreprise", "suv"},
	}
	for i, couple := range couplesDemo {
		_, _ = a.Pool.Exec(ctx, `
INSERT INTO station_vehicle_usage_categories (station_id, usage_code, category_code, sort_order, is_active)
VALUES ($1,$2,$3,$4,true)
ON CONFLICT (station_id, usage_code, category_code) DO UPDATE SET is_active = TRUE`,
			stationID, couple[0], couple[1], i)
	}

	// Moyens de paiement : l'amorçage du démarrage est passé **avant** ces
	// stations, qui viennent d'être créées. Sans cet appel, la caisse de
	// démonstration n'aurait aucun moyen actif — et l'étape « moyens » du
	// parcours resterait ouverte sur une installation neuve.
	if err := a.equiperMoyensPaiement(ctx, orgID, stationID); err != nil {
		return err
	}
	// Même raison pour les catégories de consommables : une station de
	// démonstration sans vocabulaire de stock ferait échouer l'étape 12.
	if err := a.equiperCategoriesStock(ctx, orgID, stationID); err != nil {
		return err
	}
	return nil
}
