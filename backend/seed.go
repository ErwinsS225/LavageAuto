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

func (a *App) seedDemo(ctx context.Context) error {
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
INSERT INTO organizations (id, name, slug, is_active) VALUES ($1,$2,$3,true)`,
		orgID, "Station Démo", "station-demo")
	if err != nil {
		return fmt.Errorf("seed org : %w", err)
	}

	stationID := newID("stn")
	_, err = a.Pool.Exec(ctx, `
INSERT INTO stations (id, organization_id, name, slug, city, is_active)
VALUES ($1,$2,$3,$4,$5,true)`,
		stationID, orgID, "Station Centrale", "station-centrale", "Abidjan")
	if err != nil {
		return fmt.Errorf("seed station : %w", err)
	}

	hash, err := hashPassword("demo1234")
	if err != nil {
		return err
	}
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
INSERT INTO services (id, organization_id, name, price_xof, is_active)
VALUES ($1,$2,$3,$4,true)`, newID("svc"), orgID, s.name, s.price)
	}
	return nil
}
