package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// matrice.go — grille tarifaire (Lot 3) : lecture de la matrice complète, au
// contrat front `PriceMatrix` (types/index.ts). La grille est LUE ici, jamais
// calculée par le navigateur : chaque ligne porte son tarif du catalogue, ses
// cases catégorie, ses cases usage, et si la prestation reste à tarifer. Le
// prix retenu sur une fiche, lui, vient du moteur (pricing.go) — la matrice
// décrit la grille, le moteur l'applique.

type colonneRef struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Family string `json:"family"`
}

// categoriesActives : les colonnes « véhicule » de la grille (catégories que
// la station a activées), dans l'ordre du référentiel.
func (a *App) categoriesActives(ctx context.Context, stationID string) []colonneRef {
	rows, err := a.db.Query(ctx, `
		SELECT code, name, family FROM vehicle_categories
		WHERE code IN (SELECT category_code FROM station_vehicle_categories WHERE station_id = $1 AND is_active)
		ORDER BY sort_order, name`, stationID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []colonneRef
	for rows.Next() {
		var c colonneRef
		if rows.Scan(&c.Code, &c.Name, &c.Family) == nil {
			out = append(out, c)
		}
	}
	return out
}

// usagesActifs : les usages commerciaux activés par la station.
func (a *App) usagesActifs(ctx context.Context, stationID string) []colonneRef {
	rows, err := a.db.Query(ctx, `
		SELECT code, name, family FROM vehicle_usages
		WHERE code IN (SELECT usage_code FROM station_vehicle_usages WHERE station_id = $1 AND is_active)
		ORDER BY sort_order, name`, stationID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []colonneRef
	for rows.Next() {
		var c colonneRef
		if rows.Scan(&c.Code, &c.Name, &c.Family) == nil {
			out = append(out, c)
		}
	}
	return out
}

type serviceVendable struct {
	ID              string
	Name            string
	FamilyCode      string
	FamilyName      string
	PriceXOF        int
	DurationMinutes int
	IsFree          bool
	RequiresQuote   bool
}

// servicesVendables : les lignes de la grille (prestations actives de la
// station, avec le tarif du catalogue que porte chacune).
func (a *App) servicesVendables(ctx context.Context, stationID string) []serviceVendable {
	rows, err := a.db.Query(ctx, `
		SELECT s.id, s.name, s.family_code, COALESCE(sf.name, ''), s.price_xof,
		       s.duration_minutes, s.is_free, s.requires_quote
		FROM services s
		LEFT JOIN service_families sf ON sf.code = s.family_code
		WHERE s.station_id = $1 AND s.is_active = true
		ORDER BY s.sort_order, s.name`, stationID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []serviceVendable
	for rows.Next() {
		var s serviceVendable
		if rows.Scan(&s.ID, &s.Name, &s.FamilyCode, &s.FamilyName, &s.PriceXOF,
			&s.DurationMinutes, &s.IsFree, &s.RequiresQuote) == nil {
			out = append(out, s)
		}
	}
	return out
}

type matriceCell struct {
	RuleID   string
	PriceXOF int
	Source   string
}

func (c matriceCell) toJSON(categoryCode, usageCode string) map[string]interface{} {
	return map[string]interface{}{
		"category_code": categoryCode,
		"usage_code":    usageCode,
		"price_xof":     c.PriceXOF,
		"rule_id":       c.RuleID,
		"source":        c.Source,
	}
}

// lireMatricePrix construit la grille complète. `a_tarifer` compte les
// prestations qu'aucun prix utilisable ne couvre encore.
func (a *App) lireMatricePrix(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		ecrireValidation(w, map[string]string{"station_id": "Ce champ est obligatoire."})
		return
	}
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}
	ctx := r.Context()

	categories := a.categoriesActives(ctx, stationID)
	usages := a.usagesActifs(ctx, stationID)
	services := a.servicesVendables(ctx, stationID)

	// Règles actives, indexées par (service, catégorie) et (service, usage).
	regleCat := map[string]matriceCell{}
	regleUsage := map[string]matriceCell{}
	rows, err := a.db.Query(ctx, `
		SELECT id, service_id, category_code, usage_code, price_xof
		FROM price_rules WHERE station_id = $1 AND is_active = true`, stationID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, serviceID, cat, usg string
			var prix int
			if rows.Scan(&id, &serviceID, &cat, &usg, &prix) != nil {
				continue
			}
			cell := matriceCell{RuleID: id, PriceXOF: prix, Source: specificiteRegle(false, cat, usg)}
			if cat != "" {
				regleCat[serviceID+"|"+cat] = cell
			} else if usg != "" {
				regleUsage[serviceID+"|"+usg] = cell
			}
		}
	}

	generiqueAutorise := a.reglageBooleen(ctx, stationID, "allow_generic_price", true)

	aTarifer := 0
	lignes := []map[string]interface{}{}
	for _, s := range services {
		couverteParRegle := false
		cells := []map[string]interface{}{}
		for _, cat := range categories {
			if c, ok := regleCat[s.ID+"|"+cat.Code]; ok {
				couverteParRegle = true
				cells = append(cells, c.toJSON(cat.Code, ""))
			} else {
				cells = append(cells, map[string]interface{}{
					"category_code": cat.Code, "price_xof": nil,
					"rule_id": "", "source": "", "is_free": s.IsFree,
				})
			}
		}
		usageCells := []map[string]interface{}{}
		for _, us := range usages {
			if c, ok := regleUsage[s.ID+"|"+us.Code]; ok {
				couverteParRegle = true
				usageCells = append(usageCells, c.toJSON("", us.Code))
			} else {
				usageCells = append(usageCells, map[string]interface{}{
					"usage_code": us.Code, "price_xof": nil,
					"rule_id": "", "source": "", "is_free": s.IsFree,
				})
			}
		}
		vendable := s.IsFree || (generiqueAutorise && s.PriceXOF > 0) || couverteParRegle
		if !vendable {
			aTarifer++
		}
		generic := 0
		if generiqueAutorise {
			generic = s.PriceXOF
		}
		lignes = append(lignes, map[string]interface{}{
			"service_id": s.ID, "name": s.Name,
			"family_code": s.FamilyCode, "family_name": s.FamilyName,
			"duration_minutes": s.DurationMinutes, "generic_price_xof": generic,
			"is_free": s.IsFree, "is_quote_only": s.RequiresQuote,
			"needs_price": !vendable, "cells": cells, "usage_cells": usageCells,
		})
	}

	message := "Le catalogue est prêt."
	if aTarifer > 0 {
		message = fmt.Sprintf("Il reste %d prestation(s) sans prix utilisable.", aTarifer)
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"station_id": stationID, "services": lignes,
		"categories": categories, "usages": usages,
		"generique_autorise": generiqueAutorise,
		"a_tarifer":          aTarifer, "message": message,
	})
}

// mettreAJourMatricePrix enregistre la grille en bloc, motif obligatoire. Une
// case vidée (`price_xof: null`) DÉSACTIVE la règle au lieu de la supprimer.
// Rien n'est écrit si une seule case est fautive (validation avant écriture).
func (a *App) mettreAJourMatricePrix(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	corps, ok := decoderCorps[MatricePrixRequest](w, r)
	if !ok {
		return
	}
	if len(strings.TrimSpace(corps.Reason)) < 5 {
		ecrireValidation(w, map[string]string{"reason": "Une raison d'au moins 5 caractères est obligatoire."})
		return
	}
	if strings.TrimSpace(corps.StationID) == "" {
		ecrireValidation(w, map[string]string{"station_id": "Station requise."})
		return
	}
	if err := a.verifierAppartenanceStation(r.Context(), corps.StationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}
	ctx := r.Context()
	var organisationID string
	if err := a.db.QueryRow(ctx,
		`SELECT organization_id FROM stations WHERE id = $1`, corps.StationID).Scan(&organisationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	for _, c := range corps.Cells {
		if c.PriceXOF != nil && *c.PriceXOF < 0 {
			ecrireValidation(w, map[string]string{"cells": "Le prix ne peut pas être négatif."})
			return
		}
		if strings.TrimSpace(c.ServiceID) == "" {
			ecrireValidation(w, map[string]string{"cells": "Prestation manquante."})
			return
		}
	}
	for _, c := range corps.UsageCells {
		if c.PriceXOF != nil && *c.PriceXOF < 0 {
			ecrireValidation(w, map[string]string{"usage_cells": "Le prix ne peut pas être négatif."})
			return
		}
	}
	for _, g := range corps.GenericPrices {
		if g.PriceXOF < 0 {
			ecrireValidation(w, map[string]string{"generic_prices": "Le prix ne peut pas être négatif."})
			return
		}
	}

	compte := MatricePrixReponse{}
	ecrireCase := func(serviceID, cat, usg string, prix *int) error {
		if prix == nil {
			_, err := a.db.Exec(ctx, `
				UPDATE price_rules SET is_active = false, updated_at = NOW()
				WHERE station_id = $1 AND service_id = $2 AND category_code = $3 AND usage_code = $4`,
				corps.StationID, serviceID, cat, usg)
			compte.Desactivees++
			return err
		}
		var id string
		err := a.db.QueryRow(ctx, `
			INSERT INTO price_rules (organization_id, station_id, service_id, category_code, usage_code,
			                         price_xof, is_active, reason, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, true, $7, NOW(), NOW())
			ON CONFLICT (station_id, service_id, category_code, usage_code)
			DO UPDATE SET price_xof = EXCLUDED.price_xof, is_active = true,
			              reason = EXCLUDED.reason, updated_at = NOW()
			RETURNING id`,
			organisationID, corps.StationID, serviceID, cat, usg, *prix, corps.Reason).Scan(&id)
		if err != nil {
			return err
		}
		compte.Crees++
		return nil
	}

	for _, c := range corps.Cells {
		if err := ecrireCase(c.ServiceID, c.CategoryCode, "", c.PriceXOF); err != nil {
			ecrireValidation(w, map[string]string{"cells": fmt.Sprintf("Échec sur la case %s : %v", c.CategoryCode, err)})
			return
		}
	}
	for _, c := range corps.UsageCells {
		if err := ecrireCase(c.ServiceID, "", c.UsageCode, c.PriceXOF); err != nil {
			ecrireValidation(w, map[string]string{"usage_cells": fmt.Sprintf("Échec sur la case usage %s : %v", c.UsageCode, err)})
			return
		}
	}
	for _, g := range corps.GenericPrices {
		if _, err := a.db.Exec(ctx,
			`UPDATE services SET price_xof = $1, updated_at = NOW() WHERE id = $2 AND station_id = $3`,
			g.PriceXOF, g.ServiceID, corps.StationID); err != nil {
			ecrireValidation(w, map[string]string{"generic_prices": fmt.Sprintf("Échec tarif catalogue : %v", err)})
			return
		}
		compte.Modifiees++
	}

	station := corps.StationID
	a.journaliser(ctx, &organisationID, &station, &sess.UserID,
		"price_matrix_saved", map[string]interface{}{"reason": corps.Reason})

	ecrireJSON(w, http.StatusOK, compte)
}
