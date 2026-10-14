package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ---------------------------------------------------------------------------
// Règles de prix (D5) et grille tarifaire.
//
// Une règle dit : « pour **cette** prestation, **ce** véhicule, **cet** usage,
// **ce** client, du X au Y, le prix est de N FCFA — motif ». Ce qui n'est pas
// précisé ne se prononce pas : une règle « berline » ne s'applique pas à une
// fiche sans véhicule indiqué.
//
// Deux règles **strictement identiques et actives** n'existent pas : c'est un
// conflit signalé (`409 price_rule_conflict`), jamais un choix silencieux. Un
// index d'unicité partiel le garantit en base, et le handler répond proprement
// même si la course se produit entre la vérification et l'écriture.
//
// Tout changement de prix porte un **motif** : la grille tarifaire est une
// décision d'exploitation, elle doit pouvoir s'expliquer.
// ---------------------------------------------------------------------------

var errPrixAmbigu = &apiError{
	Status:  http.StatusConflict,
	Code:    "price_rule_conflict",
	Message: "Deux règles de prix actives s'appliquent avec la même précision. Désactivez-en une : l'application ne choisit pas en silence.",
}

// reglePrixJSON : une règle telle que l'écran la lit (avec les libellés).
type reglePrixJSON struct {
	ID            string  `json:"id"`
	ServiceID     string  `json:"service_id"`
	ServiceName   string  `json:"service_name"`
	CategoryCode  string  `json:"category_code"`
	CategoryName  string  `json:"category_name"`
	UsageCode     string  `json:"usage_code"`
	UsageName     string  `json:"usage_name"`
	CustomerID    *string `json:"customer_id"`
	CustomerName  string  `json:"customer_name"`
	PriceXOF      int     `json:"price_xof"`
	ValidFrom     *string `json:"valid_from"`
	ValidTo       *string `json:"valid_to"`
	Reason        string  `json:"reason"`
	CreatedByName string  `json:"created_by_name"`
	IsActive      bool    `json:"is_active"`
	Specificity   string  `json:"specificity"`
	SpecificityFR string  `json:"specificity_label"`
}

// libelleSpecificite : comment la règle se classe dans la hiérarchie.
func libelleSpecificite(categorie, usage string, client *string) string {
	switch {
	case client != nil && *client != "":
		return "Contrat client"
	case categorie != "" && usage != "":
		return "Véhicule + usage"
	case categorie != "":
		return "Véhicule"
	default:
		return "Usage"
	}
}

// GET /api/v1/price-rules?station_id=…[&service_id=…]
func (a *App) handlePriceRulesList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []reglePrixJSON{}, "total": 0})
		return
	}
	serviceFiltre := r.URL.Query().Get("service_id")
	includeInactive := r.URL.Query().Get("include_inactive") == "true"

	lignes, err := a.Pool.Query(r.Context(), `
		SELECT p.id, p.service_id, s.name, p.category_code,
		       COALESCE(c.name, ''), COALESCE(t.name, '') AS category_name,
		       p.usage_code, COALESCE(u.name, ''), p.customer_id, COALESCE(cl.full_name, ''),
		       p.price_xof, p.valid_from, p.valid_to, p.reason, p.created_by_name, p.is_active
		FROM price_rules p
		JOIN services s ON s.id = p.service_id
		LEFT JOIN vehicle_category_templates t ON t.code = p.category_code
		LEFT JOIN station_vehicle_categories c
		       ON c.code = p.category_code AND c.station_id = p.station_id
		LEFT JOIN vehicle_usage_templates u ON u.code = p.usage_code
		LEFT JOIN customers cl ON cl.id = p.customer_id
		WHERE p.organization_id = $1 AND p.station_id = $2
		  AND ($3 = '' OR p.service_id = $3)
		  AND ($4 OR p.is_active)
		ORDER BY s.name, p.category_code, p.usage_code`,
		deref(user.OrganizationID), stationID, serviceFiltre, includeInactive)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer lignes.Close()

	items := []reglePrixJSON{}
	for lignes.Next() {
		var (
			regle                              reglePrixJSON
			categorieNom, usageNom             string
			nomModeleCategorie, nomModeleUsage string
			debut, fin                         *time.Time
		)
		if err := lignes.Scan(&regle.ID, &regle.ServiceID, &regle.ServiceName, &regle.CategoryCode,
			&categorieNom, &nomModeleCategorie, &regle.UsageCode, &usageNom, &regle.CustomerID,
			&regle.CustomerName, &regle.PriceXOF, &debut, &fin, &regle.Reason,
			&regle.CreatedByName, &regle.IsActive); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		regle.CategoryName = categorieNom
		if regle.CategoryName == "" {
			regle.CategoryName = nomModeleCategorie
		}
		regle.UsageName = usageNom
		if regle.UsageName == "" {
			regle.UsageName = nomModeleUsage
		}
		if debut != nil {
			jour := debut.Format("2006-01-02")
			regle.ValidFrom = &jour
		}
		if fin != nil {
			jour := fin.Format("2006-01-02")
			regle.ValidTo = &jour
		}
		regle.Specificity = libelleSpecificite(regle.CategoryCode, regle.UsageCode, regle.CustomerID)
		regle.SpecificityFR = regle.Specificity
		items = append(items, regle)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "station_id": stationID})
}

// reglePrixDraft : corps commun à la création et à la modification.
type reglePrixDraft struct {
	StationID    string `json:"station_id"`
	ServiceID    string `json:"service_id"`
	CategoryCode string `json:"category_code"`
	UsageCode    string `json:"usage_code"`
	CustomerID   string `json:"customer_id"`
	PriceXOF     *int   `json:"price_xof"`
	ValidFrom    string `json:"valid_from"`
	ValidTo      string `json:"valid_to"`
	Reason       string `json:"reason"`
	IsActive     *bool  `json:"is_active"`
}

// validerReglePrix contrôle tout ce qu'une règle engage : la prestation et le
// client appartiennent à l'entreprise, la catégorie et l'usage sont bien ceux
// que la station propose, la période est cohérente, le motif est écrit.
func (a *App) validerReglePrix(r *http.Request, body reglePrixDraft, stationID string) (map[string]string, error) {
	user := currentUser(r)
	erreurs := map[string]string{}
	if strings.TrimSpace(body.ServiceID) == "" {
		erreurs["service_id"] = "Prestation obligatoire."
	} else {
		var existe bool
		if err := a.Pool.QueryRow(r.Context(), `
			SELECT EXISTS (SELECT 1 FROM services
			                WHERE id = $1 AND organization_id = $2 AND station_id = $3)`,
			body.ServiceID, deref(user.OrganizationID), stationID).Scan(&existe); err != nil {
			return erreurs, err
		}
		if !existe {
			erreurs["service_id"] = "Prestation introuvable pour cette station."
		}
	}
	if body.CategoryCode != "" {
		var active bool
		if err := a.Pool.QueryRow(r.Context(), `
			SELECT EXISTS (SELECT 1 FROM station_vehicle_categories
			                WHERE station_id = $1 AND code = $2 AND is_active)`,
			stationID, body.CategoryCode).Scan(&active); err != nil {
			return erreurs, err
		}
		if !active {
			erreurs["category_code"] = "Catégorie non activée pour cette station."
		}
	}
	if body.UsageCode != "" {
		var actif bool
		if err := a.Pool.QueryRow(r.Context(), `
			SELECT EXISTS (SELECT 1 FROM station_vehicle_usages
			                WHERE station_id = $1 AND code = $2 AND is_active)`,
			stationID, body.UsageCode).Scan(&actif); err != nil {
			return erreurs, err
		}
		if !actif {
			erreurs["usage_code"] = "Usage non activé pour cette station."
		}
	}
	if body.CustomerID != "" {
		var existe bool
		if err := a.Pool.QueryRow(r.Context(), `
			SELECT EXISTS (SELECT 1 FROM customers WHERE id = $1 AND organization_id = $2)`,
			body.CustomerID, deref(user.OrganizationID)).Scan(&existe); err != nil {
			return erreurs, err
		}
		if !existe {
			erreurs["customer_id"] = "Client introuvable."
		}
	}
	if body.PriceXOF != nil && *body.PriceXOF < 0 {
		erreurs["price_xof"] = "Le tarif ne peut pas être négatif."
	}
	// Un changement de prix porte un motif : c'est ce qui rend la grille
	// explicable six mois plus tard.
	if strings.TrimSpace(body.Reason) == "" {
		erreurs["reason"] = "Motif obligatoire pour une modification de prix."
	}
	if body.ValidFrom != "" && body.ValidTo != "" && body.ValidTo < body.ValidFrom {
		erreurs["valid_to"] = "La fin de validité précède le début."
	}
	return erreurs, nil
}

// conflitReglePrix : une règle active identique existe-t-elle déjà ?
func (a *App) conflitReglePrix(r *http.Request, stationID, serviceID, categorie, usage, client, saufID string) (string, error) {
	var id string
	err := a.Pool.QueryRow(r.Context(), `
		SELECT id FROM price_rules
		WHERE station_id = $1 AND service_id = $2 AND is_active
		  AND COALESCE(category_code,'') = $3 AND COALESCE(usage_code,'') = $4
		  AND COALESCE(customer_id,'') = $5 AND id <> $6
		LIMIT 1`, stationID, serviceID, categorie, usage, client, saufID).Scan(&id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	return id, nil
}

// repondreConflit : message utile, avec la règle fautive.
func repondreConflit(w http.ResponseWriter, existante string) {
	writeError(w, http.StatusConflict, "price_rule_conflict",
		"Une règle active porte déjà exactement ces critères. Modifiez-la, ou désactivez-la avant d'en créer une autre.",
		map[string]string{"price_rule_id": existante})
}

// POST /api/v1/price-rules
func (a *App) handlePriceRuleCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body reglePrixDraft
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	stationID, err := a.resolveStation(r.Context(), user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if body.PriceXOF == nil {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Un tarif est attendu.", map[string]string{"price_xof": "Tarif obligatoire."})
		return
	}
	erreurs, err := a.validerReglePrix(r, body, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucune règle n'a été enregistrée : corrigez les champs signalés.", erreurs)
		return
	}
	if existante, err := a.conflitReglePrix(r, stationID, body.ServiceID,
		body.CategoryCode, body.UsageCode, body.CustomerID, ""); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	} else if existante != "" {
		repondreConflit(w, existante)
		return
	}

	id := newID("prc")
	actif := true
	if body.IsActive != nil {
		actif = *body.IsActive
	}
	if _, err := a.Pool.Exec(r.Context(), `
		INSERT INTO price_rules (id, organization_id, station_id, service_id, category_code,
		  usage_code, customer_id, price_xof, valid_from, valid_to, reason,
		  created_by, created_by_name, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$10::date,$11,$12,$13,$14)`,
		id, deref(user.OrganizationID), stationID, body.ServiceID, body.CategoryCode,
		body.UsageCode, strPtrOrNil(body.CustomerID), *body.PriceXOF,
		dateOrNil(body.ValidFrom), dateOrNil(body.ValidTo), strings.TrimSpace(body.Reason),
		user.ID, user.FullName, actif); err != nil {
		// Course entre la vérification et l'insertion : l'index d'unicité partiel
		// tranche, et on répond le même conflit.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			repondreConflit(w, "")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"price_rule.created", "price_rule", id,
		map[string]any{"service_id": body.ServiceID, "category_code": body.CategoryCode,
			"usage_code": body.UsageCode, "price_xof": *body.PriceXOF, "reason": body.Reason})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "service_id": body.ServiceID, "category_code": body.CategoryCode,
		"usage_code": body.UsageCode, "customer_id": strPtrOrNil(body.CustomerID),
		"price_xof": *body.PriceXOF, "reason": body.Reason, "is_active": actif,
		"valid_from": body.ValidFrom, "valid_to": body.ValidTo,
		"specificity_label": libelleSpecificite(body.CategoryCode, body.UsageCode, strPtrOrNil(body.CustomerID)),
	})
}

// PATCH /api/v1/price-rules/{id}
func (a *App) handlePriceRuleUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	var body reglePrixDraft
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	var (
		stationID, serviceID, categorie, usage string
		client                                 *string
		montant                                int
		actif                                  bool
	)
	err := a.Pool.QueryRow(r.Context(), `
		SELECT station_id, service_id, category_code, usage_code, customer_id, price_xof, is_active
		FROM price_rules WHERE id = $1 AND organization_id = $2`,
		id, deref(user.OrganizationID)).
		Scan(&stationID, &serviceID, &categorie, &usage, &client, &montant, &actif)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Règle de prix introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != stationID {
		writeAPIError(w, errStationForbidden)
		return
	}

	// Seuls le montant, la période, le motif et l'activation se modifient : les
	// critères d'une règle changent en la recréant, ce qui laisse une trace.
	nouveauMontant := montant
	if body.PriceXOF != nil {
		nouveauMontant = *body.PriceXOF
	}
	changement := nouveauMontant != montant
	motif := strings.TrimSpace(body.Reason)
	if changement && motif == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Motif obligatoire pour une modification de prix.",
			map[string]string{"reason": "Motif obligatoire."})
		return
	}
	if nouveauMontant < 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le tarif ne peut pas être négatif.", map[string]string{"price_xof": "Tarif négatif"})
		return
	}
	if body.ValidFrom != "" && body.ValidTo != "" && body.ValidTo < body.ValidFrom {
		writeError(w, http.StatusBadRequest, "validation_error",
			"La fin de validité précède le début.", map[string]string{"valid_to": "Période incohérente"})
		return
	}
	// Réactiver une règle peut créer un doublon : on vérifie avant.
	reactivation := body.IsActive != nil && *body.IsActive && !actif
	if reactivation {
		clientTxt := ""
		if client != nil {
			clientTxt = *client
		}
		if existante, err := a.conflitReglePrix(r, stationID, serviceID, categorie, usage, clientTxt, id); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		} else if existante != "" {
			repondreConflit(w, existante)
			return
		}
	}

	mises := []string{"price_xof = $1", "updated_at = now()"}
	args := []any{nouveauMontant}
	if motif != "" {
		mises = append(mises, "reason = $"+itoa(len(args)+1))
		args = append(args, motif)
	}
	if body.ValidFrom != "" {
		mises = append(mises, "valid_from = $"+itoa(len(args)+1)+"::date")
		args = append(args, body.ValidFrom)
	}
	if body.ValidTo != "" {
		mises = append(mises, "valid_to = $"+itoa(len(args)+1)+"::date")
		args = append(args, body.ValidTo)
	}
	if body.IsActive != nil {
		mises = append(mises, "is_active = $"+itoa(len(args)+1))
		args = append(args, *body.IsActive)
	}
	args = append(args, id, deref(user.OrganizationID))
	if _, err := a.Pool.Exec(r.Context(), `
		UPDATE price_rules SET `+strings.Join(mises, ", ")+`
		WHERE id = $`+itoa(len(args)-1)+` AND organization_id = $`+itoa(len(args)),
		args...); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"price_rule.updated", "price_rule", id,
		map[string]any{"ancien_prix": montant, "nouveau_prix": nouveauMontant, "motif": motif})
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "price_xof": nouveauMontant, "reason": motif, "is_active": actif,
		"message": "Règle de prix mise à jour.",
	})
}

// DELETE /api/v1/price-rules/{id} — désactivation : une fiche passée garde le
// prix qu'elle portait, et l'historique reste lisible.
func (a *App) handlePriceRuleDisable(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	var stationID string
	if err := a.Pool.QueryRow(r.Context(),
		`SELECT station_id FROM price_rules WHERE id = $1 AND organization_id = $2`,
		id, deref(user.OrganizationID)).Scan(&stationID); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Règle de prix introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != stationID {
		writeAPIError(w, errStationForbidden)
		return
	}
	if _, err := a.Pool.Exec(r.Context(),
		`UPDATE price_rules SET is_active = false, updated_at = now() WHERE id = $1 AND organization_id = $2`,
		id, deref(user.OrganizationID)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"price_rule.disabled", "price_rule", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Règle de prix désactivée."})
}

// ---------------------------------------------------------------------------
// Matrice tarifaire
// ---------------------------------------------------------------------------

// GET /api/v1/price-rules/matrix?station_id=…
//
// Une ligne par prestation active, une colonne par véhicule accepté, plus le
// **tarif générique** (celui du catalogue) et une colonne par usage activé.
// Chaque case dit d'où vient le prix affiché : une grille tarifaire doit
// pouvoir s'expliquer case par case.
func (a *App) handlePriceMatrix(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"station_id": "", "services": []map[string]any{}, "categories": []map[string]any{},
			"usages": []map[string]any{}, "generique_autorise": true,
		})
		return
	}

	categories, err := a.lireCategoriesStation(r.Context(), stationID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	usages, err := a.lireUsagesStation(r.Context(), stationID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	prestations, err := a.Pool.Query(r.Context(), `
		SELECT `+serviceColumns+`
		FROM services s
		LEFT JOIN service_family_templates f ON f.code = s.family_code
		WHERE s.station_id = $1 AND s.is_active
		ORDER BY s.sort_order, s.name`, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer prestations.Close()
	services := []Service{}
	for prestations.Next() {
		s, err := scanService(prestations)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		services = append(services, s)
	}

	lignesRegles, err := a.Pool.Query(r.Context(), `
		SELECT id, service_id, category_code, usage_code, price_xof, reason
		FROM price_rules
		WHERE station_id = $1 AND is_active
		  AND (valid_from IS NULL OR valid_from <= CURRENT_DATE)
		  AND (valid_to IS NULL OR valid_to >= CURRENT_DATE)`, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer lignesRegles.Close()
	regles := []reglePrix{}
	for lignesRegles.Next() {
		var r reglePrix
		if err := lignesRegles.Scan(&r.ID, &r.Service, &r.Categorie, &r.Usage, &r.Montant, &r.Motif); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		regles = append(regles, r)
	}

	generique := a.stationAutorisePrixGenerique(r.Context(), stationID)
	lignesMatrice := []map[string]any{}
	aTarifer := 0
	for _, service := range services {
		cases := []map[string]any{}
		// Gratuite ou sur devis : les deux se passent de prix au comptoir. Le
		// compteur « à tarifer » ne doit donc réclamer que ce qui se vend.
		aUnTarif := service.IsFree || service.IsQuoteOnly
		for _, categorie := range categories {
			celle := map[string]any{
				"category_code": categorie.Code, "price_xof": nil, "rule_id": "",
				"source": "", "is_free": service.IsFree,
			}
			meilleure := reglePrix{}
			for _, regle := range regles {
				if regle.Service == service.ID && regle.Categorie == categorie.Code && regle.Usage == "" {
					meilleure = regle
				}
			}
			if meilleure.ID != "" {
				celle["price_xof"] = meilleure.Montant
				celle["rule_id"] = meilleure.ID
				celle["source"] = sourceServiceCategorie
				aUnTarif = true
			} else if service.IsFree {
				celle["price_xof"] = 0
				celle["source"] = sourceGratuit
			} else if service.PriceXOF > 0 && generique {
				// Le tarif du catalogue sert de repli : la case le dit, sans
				// créer de règle — c'est le principe du niveau 4.
				celle["price_xof"] = service.PriceXOF
				celle["source"] = sourceGenerique
				aUnTarif = true
			}
			cases = append(cases, celle)
		}

		casesUsage := []map[string]any{}
		for _, usage := range usages {
			celle := map[string]any{
				"usage_code": usage.Code, "price_xof": nil, "rule_id": "",
				"source": "",
			}
			for _, regle := range regles {
				if regle.Service == service.ID && regle.Usage == usage.Code && regle.Categorie == "" {
					celle["price_xof"] = regle.Montant
					celle["rule_id"] = regle.ID
					celle["source"] = sourceServiceUsage
				}
			}
			casesUsage = append(casesUsage, celle)
		}

		if !aUnTarif {
			aTarifer++
		}
		lignesMatrice = append(lignesMatrice, map[string]any{
			"service_id": service.ID, "name": service.Name, "family_code": service.FamilyCode,
			"family_name": service.FamilyName, "duration_minutes": service.DurationMinutes,
			"generic_price_xof": service.PriceXOF, "is_free": service.IsFree,
			"is_quote_only": service.IsQuoteOnly, "needs_price": service.NeedsPrice,
			"cells": cases, "usage_cells": casesUsage,
		})
	}

	itemsCategories := []map[string]any{}
	for _, c := range categories {
		itemsCategories = append(itemsCategories, map[string]any{
			"code": c.Code, "name": c.Nom, "family": c.Famille,
		})
	}
	itemsUsages := []map[string]any{}
	for _, u := range usages {
		itemsUsages = append(itemsUsages, map[string]any{"code": u.Code, "name": u.Nom, "family": u.Famille})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": stationID, "services": lignesMatrice,
		"categories": itemsCategories, "usages": itemsUsages,
		"generique_autorise": generique, "a_tarifer": aTarifer,
		"message": "Une case vide signifie qu'aucun tarif ne couvre cette combinaison : la vente sera refusée.",
	})
}

// PUT /api/v1/price-rules/matrix
//
// Corps : {"station_id": "…", "reason": "…", "cells": [{"service_id": "…",
// "category_code": "berline", "price_xof": 5000}, …], "usage_cells": […]}
//
// `price_xof: null` supprime la règle de la case (la vente repassera au niveau
// suivant). Le motif est obligatoire dès qu'un prix change : c'est une décision
// d'exploitation, pas un réglage anodin.
func (a *App) handlePriceMatrixUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		StationID string `json:"station_id"`
		Reason    string `json:"reason"`
		Cells     []struct {
			ServiceID    string `json:"service_id"`
			CategoryCode string `json:"category_code"`
			PriceXOF     *int   `json:"price_xof"`
		} `json:"cells"`
		UsageCells []struct {
			ServiceID string `json:"service_id"`
			UsageCode string `json:"usage_code"`
			PriceXOF  *int   `json:"price_xof"`
		} `json:"usage_cells"`
		// Tarif du catalogue (le repli) : il se modifie depuis le même écran,
		// avec le même motif — une grille tarifaire doit s'expliquer d'un bloc.
		GenericPrices []struct {
			ServiceID string `json:"service_id"`
			PriceXOF  *int   `json:"price_xof"`
		} `json:"generic_prices"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	stationID, err := a.resolveStation(r.Context(), user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	motif := strings.TrimSpace(body.Reason)
	if motif == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Motif obligatoire pour une modification de prix.",
			map[string]string{"reason": "Indiquez pourquoi ces prix changent."})
		return
	}

	type case_ struct {
		ServiceID string
		Category  string
		Usage     string
		PriceXOF  *int
	}
	cases := []case_{}
	for _, c := range body.Cells {
		cases = append(cases, case_{ServiceID: c.ServiceID, Category: c.CategoryCode, PriceXOF: c.PriceXOF})
	}
	for _, c := range body.UsageCells {
		cases = append(cases, case_{ServiceID: c.ServiceID, Usage: c.UsageCode, PriceXOF: c.PriceXOF})
	}

	// Validation en bloc : rien n'est écrit si une seule case est fautive.
	erreurs := map[string]string{}
	for _, c := range cases {
		if c.PriceXOF != nil && *c.PriceXOF < 0 {
			erreurs[c.ServiceID] = "Un tarif ne peut pas être négatif."
		}
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucun tarif n'a été enregistré : corrigez les cases signalées.", erreurs)
		return
	}

	orgID := deref(user.OrganizationID)
	creees, maj, supprimees := 0, 0, 0

	// Tarif du catalogue : une prestation gratuite ou sur devis n'en porte
	// pas — le refus est explicite plutôt que silencieux.
	for _, prix := range body.GenericPrices {
		if prix.PriceXOF == nil {
			continue
		}
		if *prix.PriceXOF < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Aucun tarif n'a été enregistré : corrigez les cases signalées.",
				map[string]string{prix.ServiceID: "Un tarif ne peut pas être négatif."})
			return
		}
		var nom string
		var gratuit, devis bool
		if err := a.Pool.QueryRow(r.Context(), `
			SELECT name, is_free, is_quote_only FROM services
			WHERE id = $1 AND organization_id = $2 AND station_id = $3`,
			prix.ServiceID, orgID, stationID).Scan(&nom, &gratuit, &devis); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Prestation introuvable pour cette station.",
				map[string]string{prix.ServiceID: "Prestation inconnue."})
			return
		}
		if gratuit || devis {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Aucun tarif n'a été enregistré : corrigez les cases signalées.",
				map[string]string{prix.ServiceID: "« " + nom + " » est gratuite ou chiffrée sur devis : elle ne porte pas de tarif."})
			return
		}
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE services SET price_xof = $1 WHERE id = $2 AND organization_id = $3`,
			*prix.PriceXOF, prix.ServiceID, orgID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		maj++
	}
	for _, c := range cases {
		regle, err := a.validerReglePrix(r, reglePrixDraft{
			ServiceID: c.ServiceID, CategoryCode: c.Category, UsageCode: c.Usage,
			PriceXOF: c.PriceXOF, Reason: motif,
		}, stationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if len(regle) > 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Aucun tarif n'a été enregistré : corrigez les champs signalés.", regle)
			return
		}

		var existante string
		var existantMontant int
		err = a.Pool.QueryRow(r.Context(), `
			SELECT id, price_xof FROM price_rules
			WHERE station_id = $1 AND service_id = $2 AND is_active
			  AND category_code = $3 AND usage_code = $4 AND customer_id IS NULL
			LIMIT 1`, stationID, c.ServiceID, c.Category, c.Usage).Scan(&existante, &existantMontant)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}

		switch {
		case c.PriceXOF == nil:
			// Case vidée : la règle est désactivée, jamais supprimée.
			if existante != "" {
				if _, err := a.Pool.Exec(r.Context(),
					`UPDATE price_rules SET is_active = false, reason = $1, updated_at = now() WHERE id = $2`,
					motif, existante); err != nil {
					writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
					return
				}
				supprimees++
			}
		case existante != "":
			if existantMontant != *c.PriceXOF {
				if _, err := a.Pool.Exec(r.Context(),
					`UPDATE price_rules SET price_xof = $1, reason = $2, updated_at = now() WHERE id = $3`,
					*c.PriceXOF, motif, existante); err != nil {
					writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
					return
				}
				maj++
			}
		default:
			if _, err := a.Pool.Exec(r.Context(), `
				INSERT INTO price_rules (id, organization_id, station_id, service_id, category_code,
				  usage_code, price_xof, reason, created_by, created_by_name, is_active)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true)`,
				newID("prc"), orgID, stationID, c.ServiceID, c.Category, c.Usage,
				*c.PriceXOF, motif, user.ID, user.FullName); err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
				return
			}
			creees++
		}
	}

	a.addAudit(r.Context(), orgID, stationID, user.FullName,
		"price_matrix.updated", "station", stationID,
		map[string]any{"creees": creees, "modifiees": maj, "desactivees": supprimees,
			"tarifs_catalogue": len(body.GenericPrices), "motif": motif})
	writeJSON(w, http.StatusOK, map[string]any{
		"creees": creees, "modifiees": maj, "desactivees": supprimees, "motif": motif,
		"message": "Grille tarifaire enregistrée.",
	})
}

// GET /api/v1/price-preview?service_id=…&category_id=…&usage_id=…&customer_id=…
//
// Le prix d'une ligne **avant** d'enregistrer la fiche : c'est ce que le poste
// de caisse affiche. Le navigateur n'a aucun montant à calculer — il demande,
// le serveur répond. Les options applicables sont renvoyées dans le même appel.
func (a *App) handlePricePreview(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	requete := r.URL.Query()
	stationID, err := a.stationDeTravail(r.Context(), user, requete.Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	serviceID := requete.Get("service_id")
	if stationID == "" || serviceID == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Station et prestation sont nécessaires.", map[string]string{"service_id": "Prestation obligatoire."})
		return
	}
	service, err := scanService(a.Pool.QueryRow(r.Context(), `
		SELECT `+serviceColumnsSansAlias+`
		FROM services WHERE id = $1 AND organization_id = $2 AND station_id = $3 AND is_active`,
		serviceID, deref(user.OrganizationID), stationID))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Prestation introuvable pour cette station.", nil)
		return
	}

	// Les discriminants sont des **codes** (`berline`, `taxi_compteur`), pas des
	// identifiants. Le nom `category_code` est le bon ; les anciens noms
	// `category_id` / `usage_id` restent acceptés pour ne casser aucun appelant.
	categorie := requete.Get("category_code")
	if categorie == "" {
		categorie = requete.Get("category_id")
	}
	usage := requete.Get("usage_code")
	if usage == "" {
		usage = requete.Get("usage_id")
	}
	client := requete.Get("customer_id")
	jour := time.Now()
	if valeur := requete.Get("date"); valeur != "" {
		if analyse, err := time.Parse("2006-01-02", valeur); err == nil {
			jour = analyse
		}
	}

	options, err := a.optionsDisponibles(r.Context(), stationID, service.ID, categorie)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	reponse := map[string]any{
		"service_id": service.ID, "service_name": service.Name,
		"is_free": service.IsFree, "is_quote_only": service.IsQuoteOnly,
		"options": options, "date": jour.Format("2006-01-02"),
	}
	prix, err := a.resolvePrice(r.Context(), stationID, service, categorie, usage, client, jour)
	if err != nil {
		if manque, ok := estPrixRequis(err); ok {
			// Une réponse lisible plutôt qu'une erreur : l'écran doit pouvoir
			// dire « à tarifer » sans casser la saisie en cours.
			reponse["price_xof"] = nil
			reponse["price_source"] = ""
			reponse["price_source_label"] = "À tarifer"
			reponse["needs_price"] = true
			reponse["message"] = "« " + manque.Service + " » n'a pas encore de tarif pour cette combinaison."
			writeJSON(w, http.StatusOK, reponse)
			return
		}
		writeAPIError(w, err)
		return
	}
	reponse["price_xof"] = prix.Montant
	reponse["price_source"] = prix.Source
	reponse["price_source_label"] = prix.Libelle
	reponse["price_rule_id"] = prix.RuleID
	reponse["needs_price"] = false
	writeJSON(w, http.StatusOK, reponse)
}

// dateOrNil : une date vide devient NULL plutôt qu'une erreur de conversion.
func dateOrNil(valeur string) any {
	if strings.TrimSpace(valeur) == "" {
		return nil
	}
	return valeur
}
