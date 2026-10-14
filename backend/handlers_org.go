package main

import (
	"net/http"
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

	// Les compteurs suivent le périmètre : un caissier ne voit que sa station.
	scopedStation := ""
	if isStationScoped(user) {
		scopedStation = deref(user.StationID)
	}
	var stations, employees, customers int
	if scopedStation == "" {
		_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM stations WHERE organization_id = $1`, org.ID).Scan(&stations)
		_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE organization_id = $1`, org.ID).Scan(&employees)
		_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM customers WHERE organization_id = $1`, org.ID).Scan(&customers)
	} else {
		_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM stations WHERE organization_id = $1 AND id = $2`, org.ID, scopedStation).Scan(&stations)
		_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE organization_id = $1 AND station_id = $2`, org.ID, scopedStation).Scan(&employees)
		_ = a.Pool.QueryRow(r.Context(), `SELECT count(*) FROM customers WHERE organization_id = $1 AND station_id = $2`, org.ID, scopedStation).Scan(&customers)
	}

	// L'état des mentions administratives accompagne la fiche : l'écran affiche
	// « renseigné / vérifié » sans avoir à le déduire lui-même.
	registre, err := a.lireRegistre(r.Context(), org.ID)
	if err != nil {
		registre = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"organization": org,
		"registry":     registre,
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
	// « status » n'est pas modifiable ici : seule la plateforme suspend une
	// entreprise (PATCH /platform/organizations/{id}).
	allowed := map[string]string{
		"name": "name", "email": "email", "phone": "phone", "address": "address",
		"city": "city", "logo_url": "logo_url", "notes": "notes",
	}
	// Identité commerciale, facturation et coordonnées : même liste fermée,
	// sans état de vérification (voir champsProfilOrganisation).
	for champ, colonne := range champsProfilOrganisation {
		allowed[champ] = colonne
	}

	// Contrôles avant écriture : mieux vaut refuser une valeur douteuse que de
	// l'enregistrer et la faire figurer sur une facture.
	erreursChamps := map[string]string{}
	if valeur, ok := body["billing_email"]; ok && !validerEmailFacultatif(toText(valeur)) {
		erreursChamps["billing_email"] = "Format d'e-mail invalide"
	}
	if valeur, ok := body["activity_started_on"]; ok && !validerDateFacultative(toText(valeur)) {
		erreursChamps["activity_started_on"] = "Date attendue au format AAAA-MM-JJ"
	}
	if len(erreursChamps) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Certaines valeurs ne sont pas valides.", erreursChamps)
		return
	}

	changed := []string{}
	for field, column := range allowed {
		v, ok := body[field]
		if !ok {
			continue
		}
		// La date est écrite en texte puis convertie : une valeur vide devient
		// NULL, ce qui distingue « pas de date » de « date inconnue ».
		if column == "activity_started_on" {
			_, err := a.Pool.Exec(r.Context(),
				`UPDATE organizations SET activity_started_on = NULLIF($1,'')::date WHERE id = $2`,
				toText(v), *user.OrganizationID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Date d'ouverture invalide (format AAAA-MM-JJ).",
					map[string]string{"activity_started_on": "Date invalide"})
				return
			}
			changed = append(changed, field)
			continue
		}
		_, _ = a.Pool.Exec(r.Context(),
			`UPDATE organizations SET `+column+` = $1 WHERE id = $2`, toText(v), *user.OrganizationID)
		changed = append(changed, field)
	}

	// Mentions administratives : valeur et état sont enregistrés ensemble, et
	// un code inconnu est refusé (l'écriture est pilotée par un catalogue).
	if registre, ok := body["registry"].(map[string]any); ok && len(registre) > 0 {
		if err := a.enregistrerRegistre(r.Context(), *user.OrganizationID, user.FullName, registre); err != nil {
			if err == errMentionInconnue {
				writeError(w, http.StatusBadRequest, "validation_error",
					"Cette mention administrative n'existe pas.",
					map[string]string{"registry": "Mention inconnue"})
				return
			}
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		changed = append(changed, "registry:"+itoa(len(registre)))
	}

	a.addAudit(r.Context(), *user.OrganizationID, "", user.FullName, "organization.updated", "organization",
		*user.OrganizationID, map[string]any{"fields": changed,
			"registry": clefsDeRegistre(body)})
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Organisation mise à jour.", "fields": changed,
	})
}

// clefsDeRegistre : les codes transmis (sans les valeurs : un numéro de
// contribuable n'a rien à faire dans un journal d'audit lisible par d'autres).
func clefsDeRegistre(body map[string]any) []string {
	registre, ok := body["registry"].(map[string]any)
	if !ok {
		return nil
	}
	codes := make([]string, 0, len(registre))
	for code := range registre {
		codes = append(codes, code)
	}
	return codes
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
		WHERE organization_id = $1
		ORDER BY created_at DESC LIMIT 500`, deref(user.OrganizationID))
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
		// Localisation : le pays, la commune et le quartier servent à retrouver
		// la station ; les coordonnées (texte) à l'afficher sur une carte.
		"country": s.Country, "district": s.District, "commune": s.Commune,
		"district_area": s.DistrictArea, "street": s.Street, "landmark": s.Landmark,
		"latitude": s.Latitude, "longitude": s.Longitude, "billing_address": s.BillingAddress,
		"created_at": s.CreatedAt.Format(timeLayout),
	}
}
