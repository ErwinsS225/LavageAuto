package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// routes_tarifs.go — Grille tarifaire et moteur de prix (Lot 3).
// Implémente toutes les routes pour la gestion des règles de prix,
// avec audit log et moteur de calcul hiérarchique.

// PriceRule représente une règle de tarification dans la grille
type PriceRule struct {
	ID        string `json:"id"`
	StationID string `json:"station_id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // client_contract, service_category_usage, service_category, service_usage, service_generic, free
	PriceXOF  int    `json:"price_xof"`
	IsActive  bool   `json:"is_active"`
	// Champs de spécificité (un ou plusieurs selon le kind)
	ServiceID    *string    `json:"service_id,omitempty"`
	CategoryCode *string    `json:"category_code,omitempty"`
	UsageCode    *string    `json:"usage_code,omitempty"`
	CustomerID   *string    `json:"customer_id,omitempty"`
	ValidFrom    *time.Time `json:"valid_from,omitempty"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	CreatedBy    string     `json:"created_by"`
	UpdatedBy    *string    `json:"updated_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
}

// AuditLog représente une entrée de journal pour les modifications de prix
type AuditLogEntry struct {
	ID         string                 `json:"id"`
	EntityType string                 `json:"entity_type"` // price_rule
	EntityID   string                 `json:"entity_id"`
	Action     string                 `json:"action"` // create, update, delete
	Reason     string                 `json:"reason"`
	UserID     string                 `json:"user_id"`
	StationID  string                 `json:"station_id"`
	Changes    map[string]interface{} `json:"changes,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

// Requêtes pour les routes tarifaires
// CreerPriceRuleRequest aligné sur le schéma réel (`price_rules`, 0001) et le
// front (PriceRuleInput) : pas de code/name/kind — la spécificité se déduit
// des discriminants (voir pricing.go).
type CreerPriceRuleRequest struct {
	StationID    string  `json:"station_id"`
	ServiceID    string  `json:"service_id"`
	CategoryCode string  `json:"category_code"`
	UsageCode    string  `json:"usage_code"`
	CustomerID   *string `json:"customer_id"`
	PriceXOF     int     `json:"price_xof"`
	ValidFrom    *string `json:"valid_from"`
	ValidTo      *string `json:"valid_to"`
	Reason       string  `json:"reason"`
}

// nilSiVide : une clé facultative absente vaut NULL, pas chaîne vide.
func nilSiVide(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

// lireDateOuVide parse une date optionnelle (nil = non renseignée).
func lireDateOuVide(brut *string) (*time.Time, error) {
	if brut == nil || strings.TrimSpace(*brut) == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", strings.TrimSpace(*brut))
	if err != nil {
		return nil, err
	}
	return &d, nil
}

type MettreAJourPriceRuleRequest struct {
	Name       *string `json:"name,omitempty"`
	PriceXOF   *int    `json:"price_xof,omitempty"`
	IsActive   *bool   `json:"is_active,omitempty"`
	ValidFrom  *string `json:"valid_from,omitempty"`
	ValidUntil *string `json:"valid_until,omitempty"`
	Reason     string  `json:"reason" validate:"required,min=5"`
}

type MatricePrixRequest struct {
	StationID     string             `json:"station_id"`
	Reason        string             `json:"reason"`
	Cells         []matriceCellule   `json:"cells"`
	UsageCells    []matriceCellule   `json:"usage_cells"`
	GenericPrices []matriceGenerique `json:"generic_prices"`
}

type matriceCellule struct {
	ServiceID    string `json:"service_id"`
	CategoryCode string `json:"category_code"`
	UsageCode    string `json:"usage_code"`
	PriceXOF     *int   `json:"price_xof"`
}

type matriceGenerique struct {
	ServiceID string `json:"service_id"`
	PriceXOF  int    `json:"price_xof"`
}

type MatricePrixReponse struct {
	Crees       int `json:"creees"`
	Modifiees   int `json:"modifiees"`
	Desactivees int `json:"desactivees"`
}

// PricePreviewReponse correspond au type front `PricePreview` (types/index.ts).
type PricePreviewReponse struct {
	ServiceID        string             `json:"service_id"`
	ServiceName      string             `json:"service_name"`
	IsFree           bool               `json:"is_free"`
	IsQuoteOnly      bool               `json:"is_quote_only"`
	Date             string             `json:"date"`
	PriceXOF         *int               `json:"price_xof"`
	PriceSource      string             `json:"price_source"`
	PriceSourceLabel string             `json:"price_source_label"`
	RuleID           *string            `json:"rule_id,omitempty"`
	NeedsPrice       bool               `json:"needs_price"`
	Message          string             `json:"message,omitempty"`
	Options          []optionApplicable `json:"options"`
}

// optionApplicable : option facturable proposée pour cette vente, avec sa
// portée (station, prestation ou véhicule).
type optionApplicable struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	PriceXOF        int    `json:"price_xof"`
	ServiceID       string `json:"service_id,omitempty"`
	CategoryCode    string `json:"category_code,omitempty"`
	DurationMinutes int    `json:"duration_minutes"`
	Scope           string `json:"scope"`
}

// ------------------------------
// Handlers des routes tarifaires
// ------------------------------

// Lister toutes les règles de prix d'une station
func (a *App) listerPriceRules(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		ecrireValidation(w, map[string]string{"station_id": "Ce champ est obligatoire."})
		return
	}

	// Vérifier que la station appartient à l'organisation
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	// Le schéma n'a pas de colonne `kind` : la spécificité se déduit des
	// discriminants (customer_id, category_code, usage_code) — même règle que
	// le moteur de prix (pricing.go).
	requete := `SELECT pr.id, pr.station_id, pr.service_id, s.name, pr.category_code, pr.usage_code,
		pr.customer_id, pr.price_xof, pr.valid_from, pr.valid_to, pr.reason, pr.is_active,
		u.full_name, pr.created_at
		FROM price_rules pr
		JOIN services s ON s.id = pr.service_id
		LEFT JOIN users u ON u.id = pr.created_by
		WHERE pr.station_id = $1 ORDER BY pr.created_at DESC`

	rows, err := a.db.Query(r.Context(), requete, stationID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les règles de prix.")
		return
	}
	defer rows.Close()

	items := []reglePrixJSON{}
	for rows.Next() {
		var r reglePrixJSON
		var serviceID pgtype.UUID
		var customerID pgtype.UUID
		var validFrom, validTo pgtype.Date
		var creeLe time.Time
		if err := rows.Scan(&r.ID, &r.StationID, &serviceID, &r.ServiceName, &r.CategoryCode, &r.UsageCode,
			&customerID, &r.PriceXOF, &validFrom, &validTo, &r.Reason, &r.IsActive,
			&r.CreatedByName, &creeLe); err != nil {
			continue
		}
		r.ServiceID = fmtUUID(serviceID)
		if customerID.Valid {
			client := fmtUUID(customerID)
			r.CustomerID = &client
		}
		if validFrom.Valid {
			v := validFrom.Time.UTC().Format("2006-01-02")
			r.ValidFrom = &v
		}
		if validTo.Valid {
			v := validTo.Time.UTC().Format("2006-01-02")
			r.ValidTo = &v
		}
		r.CreatedAt = creeLe.UTC().Format(time.RFC3339)
		r.SpecificityLabel = specificiteRegle(customerID.Valid, r.CategoryCode, r.UsageCode)
		items = append(items, r)
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"items": items, "total": len(items)})
}

// reglePrixJSON : une règle de prix lisible, au contrat front `PriceRule`.
type reglePrixJSON struct {
	ID               string  `json:"id"`
	StationID        string  `json:"station_id"`
	ServiceID        string  `json:"service_id"`
	ServiceName      string  `json:"service_name"`
	CategoryCode     string  `json:"category_code"`
	CategoryName     string  `json:"category_name"`
	UsageCode        string  `json:"usage_code"`
	UsageName        string  `json:"usage_name"`
	CustomerID       *string `json:"customer_id"`
	CustomerName     string  `json:"customer_name"`
	PriceXOF         int     `json:"price_xof"`
	ValidFrom        *string `json:"valid_from"`
	ValidTo          *string `json:"valid_to"`
	Reason           string  `json:"reason"`
	CreatedByName    string  `json:"created_by_name"`
	CreatedAt        string  `json:"created_at"`
	IsActive         bool    `json:"is_active"`
	SpecificityLabel string  `json:"specificity_label"`
}

// specificiteRegle nomme le niveau d'une règle d'après ses discriminants.
func specificiteRegle(aClient bool, categoryCode, usageCode string) string {
	switch {
	case aClient:
		return "Contrat client"
	case categoryCode != "" && usageCode != "":
		return "Prestation + véhicule + usage"
	case categoryCode != "":
		return "Prestation + véhicule"
	case usageCode != "":
		return "Prestation + usage"
	default:
		return "Tarif du catalogue"
	}
}

// Lire une règle de prix spécifique
func (a *App) lirePriceRule(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	ruleID := cheminParam(r, "/api/v1/price-rules/")
	if ruleID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de regle invalide.")
		return
	}

	var r2 reglePrixJSON
	var serviceID, customerID pgtype.UUID
	var validFrom, validTo pgtype.Date
	var creeLe time.Time
	err := a.db.QueryRow(r.Context(), `
		SELECT pr.id, pr.station_id, pr.service_id, s.name, pr.category_code, pr.usage_code,
		       pr.customer_id, pr.price_xof, pr.valid_from, pr.valid_to, pr.reason, pr.is_active,
		       u.full_name, pr.created_at
		FROM price_rules pr
		JOIN services s ON s.id = pr.service_id
		LEFT JOIN users u ON u.id = pr.created_by
		WHERE pr.id = $1`, ruleID).Scan(
		&r2.ID, &r2.StationID, &serviceID, &r2.ServiceName, &r2.CategoryCode, &r2.UsageCode,
		&customerID, &r2.PriceXOF, &validFrom, &validTo, &r2.Reason, &r2.IsActive,
		&r2.CreatedByName, &creeLe)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Regle de prix introuvable.")
		return
	}
	if err := a.verifierAppartenanceStation(r.Context(), r2.StationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Regle de prix introuvable.")
		return
	}
	r2.ServiceID = fmtUUID(serviceID)
	if customerID.Valid {
		client := fmtUUID(customerID)
		r2.CustomerID = &client
	}
	if validFrom.Valid {
		v := validFrom.Time.UTC().Format("2006-01-02")
		r2.ValidFrom = &v
	}
	if validTo.Valid {
		v := validTo.Time.UTC().Format("2006-01-02")
		r2.ValidTo = &v
	}
	r2.CreatedAt = creeLe.UTC().Format(time.RFC3339)
	r2.SpecificityLabel = specificiteRegle(customerID.Valid, r2.CategoryCode, r2.UsageCode)
	ecrireJSON(w, http.StatusOK, r2)
}

// Créer une nouvelle règle de prix
func (a *App) creerPriceRule(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !a.stationActive(r.Context(), *sess.OrganizationID) {
		repondreErreur(w, http.StatusConflict, "onboarding_incomplete", "La station n'est pas encore activee.")
		return
	}

	corps, ok := decoderCorps[CreerPriceRuleRequest](w, r)
	if !ok {
		return
	}

	champs := map[string]string{}
	if strings.TrimSpace(corps.StationID) == "" {
		champs["station_id"] = "Station requise."
	}
	if strings.TrimSpace(corps.ServiceID) == "" {
		champs["service_id"] = "Prestation requise."
	}
	if corps.PriceXOF < 0 {
		champs["price_xof"] = "Le prix ne peut pas etre negatif."
	}
	if len(strings.TrimSpace(corps.Reason)) < 5 {
		champs["reason"] = "Une raison d'au moins 5 caracteres est obligatoire."
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}

	if err := a.verifierAppartenanceStation(r.Context(), corps.StationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}
	var organisationID string
	if err := a.db.QueryRow(r.Context(),
		`SELECT organization_id FROM stations WHERE id = $1`, corps.StationID).Scan(&organisationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	validFrom, err := lireDateOuVide(corps.ValidFrom)
	if err != nil {
		ecrireValidation(w, map[string]string{"valid_from": "Format de date attendu YYYY-MM-DD."})
		return
	}
	validTo, err := lireDateOuVide(corps.ValidTo)
	if err != nil {
		ecrireValidation(w, map[string]string{"valid_to": "Format de date attendu YYYY-MM-DD."})
		return
	}

	var nouvelleID string
	var creeLe time.Time
	err = a.db.QueryRow(r.Context(), `
		INSERT INTO price_rules (organization_id, station_id, service_id, customer_id,
		                         category_code, usage_code, price_xof, valid_from, valid_to, reason, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true, NOW())
		RETURNING id, created_at`,
		organisationID, corps.StationID, corps.ServiceID, nilSiVide(corps.CustomerID),
		corps.CategoryCode, corps.UsageCode, corps.PriceXOF, validFrom, validTo, corps.Reason).Scan(&nouvelleID, &creeLe)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "creation_failed",
			fmt.Sprintf("Impossible de creer la regle : %v", err))
		return
	}

	station := corps.StationID
	a.journaliser(r.Context(), &organisationID, &station, &sess.UserID,
		"price_rule_created", map[string]interface{}{"rule_id": nouvelleID, "reason": corps.Reason})

	ecrireJSON(w, http.StatusCreated, map[string]interface{}{"id": nouvelleID, "created_at": creeLe})
}

// Modifier une règle de prix existante (PATCH)
func (a *App) modifierPriceRule(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	ruleID := cheminParam(r, "/api/v1/price-rules/")
	if ruleID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de regle invalide.")
		return
	}

	var corps map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&corps); err != nil {
		ecrireJSON(w, http.StatusBadRequest, map[string]string{"corps": "Format JSON invalide."})
		return
	}
	reason, ok := corps["reason"].(string)
	if !ok || len(reason) < 5 {
		ecrireValidation(w, map[string]string{"reason": "Une raison d'au moins 5 caracteres est obligatoire."})
		return
	}

	var stationID string
	err := a.db.QueryRow(r.Context(), `SELECT station_id FROM price_rules WHERE id = $1`, ruleID).Scan(&stationID)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Regle de prix introuvable.")
		return
	}
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Regle de prix introuvable.")
		return
	}

	// Champs modifiables : price_xof, is_active, valid_from, valid_to. Pas de
	// `name`/`kind` (le schéma ne les a pas). Un prix negatif est refuse.
	modifications := []string{"updated_at = NOW()"}
	valeurs := []interface{}{}
	idx := 1
	ajouter := func(colonne string, valeur interface{}) {
		valeurs = append(valeurs, valeur)
		modifications = append(modifications, fmt.Sprintf("%s = $%d", colonne, idx))
		idx++
	}
	if prix, ok := corps["price_xof"].(float64); ok {
		if int(prix) < 0 {
			ecrireValidation(w, map[string]string{"price_xof": "Le prix ne peut pas etre negatif."})
			return
		}
		ajouter("price_xof", int(prix))
	}
	if actif, ok := corps["is_active"].(bool); ok {
		ajouter("is_active", actif)
	}
	if brut, present := corps["valid_from"].(string); present {
		d, err := time.Parse("2006-01-02", brut)
		if err != nil {
			ecrireValidation(w, map[string]string{"valid_from": "Format de date attendu YYYY-MM-DD."})
			return
		}
		ajouter("valid_from", d)
	}
	if brut, present := corps["valid_to"].(string); present {
		d, err := time.Parse("2006-01-02", brut)
		if err != nil {
			ecrireValidation(w, map[string]string{"valid_to": "Format de date attendu YYYY-MM-DD."})
			return
		}
		ajouter("valid_to", d)
	}

	valeurs = append(valeurs, ruleID)
	requete := fmt.Sprintf("UPDATE price_rules SET %s WHERE id = $%d",
		strings.Join(modifications, ", "), len(valeurs))
	if _, err := a.db.Exec(r.Context(), requete, valeurs...); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "update_failed", fmt.Sprintf("Impossible de mettre a jour : %v", err))
		return
	}

	station := stationID
	a.journaliser(r.Context(), nil, &station, &sess.UserID,
		"price_rule_updated", map[string]interface{}{"rule_id": ruleID, "reason": reason})

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

// Supprimer (désactiver) une règle de prix
func (a *App) supprimerPriceRule(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	ruleID := cheminParam(r, "/api/v1/price-rules/")
	if ruleID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de règle invalide.")
		return
	}

	// Récupérer la station pour vérification
	var stationID string
	err := a.db.QueryRow(r.Context(), `SELECT station_id FROM price_rules WHERE id = $1`, ruleID).Scan(&stationID)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Règle de prix introuvable.")
		return
	}

	// Vérifier l'appartenance
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Règle de prix introuvable.")
		return
	}

	// Désactiver la règle (jamais de suppression)
	_, err = a.db.Exec(r.Context(), `UPDATE price_rules SET is_active = false, updated_at = NOW() WHERE id = $1`, ruleID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "delete_failed", "Impossible de desactiver la regle.")
		return
	}

	station := stationID
	a.journaliser(r.Context(), nil, &station, &sess.UserID,
		"price_rule_disabled", map[string]interface{}{"rule_id": ruleID})

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

// Récupérer la matrice complète des prix pour l'éditeur frontal
func (a *App) previewPrix(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	serviceID := r.URL.Query().Get("service_id")
	categoryCode := strings.TrimSpace(r.URL.Query().Get("category_code"))
	usageCode := strings.TrimSpace(r.URL.Query().Get("usage_code"))
	customerID := strings.TrimSpace(r.URL.Query().Get("customer_id"))

	if stationID == "" || serviceID == "" {
		ecrireValidation(w, map[string]string{"required": "station_id et service_id sont obligatoires."})
		return
	}
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	// Le code stable prime ; l'identifiant reste un repli pour un référentiel
	// qui n'aurait pas encore de code (le front envoie l'un ou l'autre).
	if categoryCode == "" {
		if id := strings.TrimSpace(r.URL.Query().Get("category_id")); id != "" {
			categoryCode = a.codeCategorieParID(r.Context(), id)
		}
	}
	if usageCode == "" {
		if id := strings.TrimSpace(r.URL.Query().Get("usage_id")); id != "" {
			usageCode = a.codeUsageParID(r.Context(), id)
		}
	}

	service, err := a.lireServiceVendu(r.Context(), serviceID)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Prestation introuvable.")
		return
	}

	res, err := a.resoudrePrix(r.Context(), contextePrix{
		stationID: stationID, serviceID: serviceID,
		categoryCode: categoryCode, usageCode: usageCode, customerID: customerID,
	})
	if err != nil {
		// Seul vrai refus du moteur : deux règles aussi précises l'une que
		// l'autre. Le nom de la règle en cause est dans le message.
		repondreErreur(w, http.StatusConflict, "price_rule_conflict", err.Error())
		return
	}

	// L'aperçu répond 200 MÊME sans tarif (needs_price) : la saisie en cours
	// n'est pas cassée par une combinaison non tarifée.
	var prix *int
	if !res.needsPrice {
		p := res.priceXOF
		prix = &p
	}
	reponse := PricePreviewReponse{
		ServiceID:        serviceID,
		ServiceName:      service.name,
		IsFree:           service.isFree,
		IsQuoteOnly:      service.requiresQuote,
		Date:             time.Now().UTC().Format("2006-01-02"),
		PriceXOF:         prix,
		PriceSource:      res.source,
		PriceSourceLabel: res.sourceLabel,
		NeedsPrice:       res.needsPrice,
		Message:          res.message,
	}
	if res.ruleID != "" {
		reponse.RuleID = &res.ruleID
	}

	// Options applicables : portée station, prestation ou véhicule.
	reponse.Options = a.optionsApplicables(r.Context(), stationID, serviceID, categoryCode)

	ecrireJSON(w, http.StatusOK, reponse)
}

// codeCategorieParID / codeUsageParID : le code stable d'un identifiant de
// référentiel, vide si l'identifiant est inconnu (la règle sera inapplicable,
// ce qui est le comportement honnête).
func (a *App) codeCategorieParID(ctx context.Context, id string) string {
	var code string
	_ = a.db.QueryRow(ctx, `SELECT code FROM vehicle_categories WHERE id = $1`, id).Scan(&code)
	return code
}

func (a *App) codeUsageParID(ctx context.Context, id string) string {
	var code string
	_ = a.db.QueryRow(ctx, `SELECT code FROM vehicle_usages WHERE id = $1`, id).Scan(&code)
	return code
}

// optionsApplicables : options actives de la portée de la vente (toutes les
// stations, + celles de la prestation, + celles de la catégorie du véhicule).
func (a *App) optionsApplicables(ctx context.Context, stationID, serviceID, categoryCode string) []optionApplicable {
	rows, err := a.db.Query(ctx, `
		SELECT id, name, COALESCE(description, ''), price_xof, service_id, category_code,
		       COALESCE(duration_minutes, 0),
		       CASE WHEN service_id IS NOT NULL THEN 'prestation'
		            WHEN category_code IS NOT NULL AND category_code <> '' THEN 'vehicule'
		            ELSE 'station' END AS portee
		FROM service_options
		WHERE station_id = $1 AND is_active = true
		  AND (service_id IS NULL OR service_id = $2)
		  AND (category_code IS NULL OR category_code = '' OR category_code = $3)
		ORDER BY sort_order, name`, stationID, serviceID, categoryCode)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []optionApplicable
	for rows.Next() {
		var o optionApplicable
		var serviceIDOpt, categoryCodeOpt *string
		if err := rows.Scan(&o.ID, &o.Name, &o.Description, &o.PriceXOF, &serviceIDOpt,
			&categoryCodeOpt, &o.DurationMinutes, &o.Scope); err != nil {
			continue
		}
		if serviceIDOpt != nil {
			o.ServiceID = *serviceIDOpt
		}
		if categoryCodeOpt != nil {
			o.CategoryCode = *categoryCodeOpt
		}
		out = append(out, o)
	}
	return out
}
