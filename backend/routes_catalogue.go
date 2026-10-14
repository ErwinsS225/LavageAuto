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

// routes_catalogue.go — Catalogue prestations + options (Lot 2).
// Implémente toutes les routes pour la gestion des services et options,
// avec isolation par station et vérifications de permissions.

// Service représente une prestation du catalogue
type Service struct {
	ID              string        `json:"id"`
	Code            string        `json:"code"`
	TemplateCode    string        `json:"template_code,omitempty"`
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	FamilyCode      string        `json:"family_code"`
	DurationMinutes int           `json:"duration_minutes"`
	SortOrder       int           `json:"sort_order"`
	Includes        []interface{} `json:"includes"`
	EquipmentNotes  string        `json:"equipment_notes"`
	SafetyNotes     string        `json:"safety_notes"`
	Preconditions   string        `json:"preconditions"`
	RequiresQuote   bool          `json:"requires_quote"`
	IsActive        bool          `json:"is_active"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// ServiceOption représente une option facturable
type ServiceOption struct {
	ID              string  `json:"id"`
	CategoryCode    string  `json:"category_code"`
	ServiceID       *string `json:"service_id,omitempty"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	PriceXOF        int     `json:"price_xof"`
	DurationMinutes int     `json:"duration_minutes"`
	SortOrder       int     `json:"sort_order"`
	IsActive        bool    `json:"is_active"`
}

// ServiceTemplate est un modèle de prestation du référentiel produit
type ServiceTemplate struct {
	Code                   string        `json:"code"`
	Name                   string        `json:"name"`
	FamilyCode             string        `json:"family_code"`
	Description            string        `json:"description"`
	DefaultDurationMinutes int           `json:"default_duration_minutes"`
	Includes               []interface{} `json:"includes"`
	EquipmentNotes         string        `json:"equipment_notes"`
	SafetyNotes            string        `json:"safety_notes"`
	Preconditions          string        `json:"preconditions"`
	IsQuoteOnly            bool          `json:"is_quote_only"`
}

// ServiceFamily représente une famille de services
type ServiceFamily struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	IsActive  bool   `json:"is_active"`
}

// Requête de création de service
type CreerServiceRequest struct {
	StationID       string        `json:"station_id"`
	Code            string        `json:"code"`
	TemplateCode    string        `json:"template_code,omitempty"`
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	FamilyCode      string        `json:"family_code"`
	DurationMinutes int           `json:"duration_minutes"`
	SortOrder       int           `json:"sort_order"`
	Includes        []interface{} `json:"includes"`
	EquipmentNotes  string        `json:"equipment_notes"`
	SafetyNotes     string        `json:"safety_notes"`
	Preconditions   string        `json:"preconditions"`
	RequiresQuote   bool          `json:"requires_quote"`
}

// Requête de mise à jour de service (PATCH)
type MettreAJourServiceRequest struct {
	Name            *string        `json:"name,omitempty"`
	Description     *string        `json:"description,omitempty"`
	FamilyCode      *string        `json:"family_code,omitempty"`
	DurationMinutes *int           `json:"duration_minutes,omitempty"`
	SortOrder       *int           `json:"sort_order,omitempty"`
	Includes        *[]interface{} `json:"includes,omitempty"`
	EquipmentNotes  *string        `json:"equipment_notes,omitempty"`
	SafetyNotes     *string        `json:"safety_notes,omitempty"`
	Preconditions   *string        `json:"preconditions,omitempty"`
	RequiresQuote   *bool          `json:"requires_quote,omitempty"`
	IsActive        *bool          `json:"is_active,omitempty"`
}

// Requête de création d'option
type CreerOptionRequest struct {
	StationID       string  `json:"station_id"`
	ServiceID       *string `json:"service_id,omitempty"`
	CategoryCode    string  `json:"category_code"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	PriceXOF        int     `json:"price_xof"`
	DurationMinutes int     `json:"duration_minutes"`
	SortOrder       int     `json:"sort_order"`
}

// Requête de mise à jour d'option
type MettreAJourOptionRequest struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	PriceXOF        *int    `json:"price_xof,omitempty"`
	DurationMinutes *int    `json:"duration_minutes,omitempty"`
	SortOrder       *int    `json:"sort_order,omitempty"`
	IsActive        *bool   `json:"is_active,omitempty"`
}

// Requête de mise à jour des compatibilités
type MettreAJourCompatibilitesRequest struct {
	Compatibilities []struct {
		Kind string `json:"kind"` // category | usage
		Code string `json:"code"`
	} `json:"compatibilities"`
}

// Récupérer tous les templates de services disponibles
const requeteTemplatesServices = `
	SELECT code, name, family_code, description, default_duration_minutes,
	       includes, equipment_notes, safety_notes, preconditions, is_quote_only
	FROM service_templates
	ORDER BY family_code, name`

func (a *App) listerTemplatesServices(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		ecrireValidation(w, map[string]string{"station_id": "L'identifiant de station est requis."})
		return
	}

	// Vérifier que la station appartient bien à l'organisation
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	rows, err := a.db.Query(r.Context(), requeteTemplatesServices)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les modèles.")
		return
	}
	defer rows.Close()

	templates := []ServiceTemplate{}
	for rows.Next() {
		var t ServiceTemplate
		var includes []byte
		if err := rows.Scan(&t.Code, &t.Name, &t.FamilyCode, &t.Description,
			&t.DefaultDurationMinutes, &includes, &t.EquipmentNotes,
			&t.SafetyNotes, &t.Preconditions, &t.IsQuoteOnly); err == nil {
			_ = json.Unmarshal(includes, &t.Includes)
			templates = append(templates, t)
		}
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"templates": templates})
}

// Récupérer toutes les familles de services
const requeteFamillesServices = `
	SELECT id, code, name, sort_order, is_active
	FROM service_families
	WHERE organization_id IS NULL OR organization_id = $1
	ORDER BY sort_order, name`

func (a *App) listerFamillesServices(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	activesOnly := r.URL.Query().Get("actives") == "1"

	if stationID != "" {
		// Vérifier appartenance
		if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
			repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
			return
		}
	}

	rows, err := a.db.Query(r.Context(), requeteFamillesServices, *sess.OrganizationID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les familles.")
		return
	}
	defer rows.Close()

	familles := []ServiceFamily{}
	for rows.Next() {
		var f ServiceFamily
		if err := rows.Scan(&f.ID, &f.Code, &f.Name, &f.SortOrder, &f.IsActive); err == nil {
			if !activesOnly || f.IsActive {
				familles = append(familles, f)
			}
		}
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"families": familles})
}

// Activer/désactiver des familles de services pour une station
const requeteActiverFamillesStation = `
	INSERT INTO station_service_families (station_id, family_code, is_active)
	VALUES ($1, unnest($2::text[]), unnest($3::boolean[]))
	ON CONFLICT (station_id, family_code) DO UPDATE SET is_active = EXCLUDED.is_active`

func (a *App) mettreAJourFamillesStation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := cheminParam(r, "/api/v1/stations/")
	if stationID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de station invalide.")
		return
	}

	// Vérifier que la station appartient bien à l'organisation
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	type FamilleRequest struct {
		Code     string `json:"code"`
		IsActive bool   `json:"is_active"`
	}
	var corps struct {
		Familles []FamilleRequest `json:"families"`
	}
	if ok, valide := decoderCorps[struct {
		Familles []FamilleRequest `json:"families"`
	}](w, r); !valide {
		return
	} else {
		corps = *ok
	}

	// Vérifier que tous les codes existent
	codes := []string{}
	actifs := []bool{}
	champsInvalides := map[string]string{}
	for i, f := range corps.Familles {
		codes = append(codes, f.Code)
		actifs = append(actifs, f.IsActive)
		var existe bool
		err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM service_families WHERE code = $1 AND (organization_id IS NULL OR organization_id = $2))`,
			f.Code, *sess.OrganizationID).Scan(&existe)
		if err != nil || !existe {
			champsInvalides[fmt.Sprintf("families[%d].code", i)] = fmt.Sprintf("Code de famille inconnu : %s", f.Code)
		}
	}
	if len(champsInvalides) > 0 {
		ecrireValidation(w, champsInvalides)
		return
	}

	// Enregistrer en base
	_, err := a.db.Exec(r.Context(), requeteActiverFamillesStation, stationID, codes, actifs)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de mettre à jour les familles.")
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true, "updated": len(corps.Familles)})
}

// Lister tous les services d'une station
const requeteListerServices = `
	SELECT id, code, template_code, name, description, family_code, duration_minutes,
	       sort_order, includes, equipment_notes, safety_notes, preconditions,
	       requires_quote, is_active, created_at, updated_at
	FROM services
	WHERE station_id = $1 AND organization_id = $2
	ORDER BY sort_order, name`

func (a *App) listerServices(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		ecrireValidation(w, map[string]string{"station_id": "L'identifiant de station est requis."})
		return
	}

	// Vérifier appartenance
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	rows, err := a.db.Query(r.Context(), requeteListerServices, stationID, *sess.OrganizationID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les services.")
		return
	}
	defer rows.Close()

	services := []Service{}
	for rows.Next() {
		var s Service
		var includes []byte
		if err := rows.Scan(&s.ID, &s.Code, &s.TemplateCode, &s.Name, &s.Description,
			&s.FamilyCode, &s.DurationMinutes, &s.SortOrder, &includes,
			&s.EquipmentNotes, &s.SafetyNotes, &s.Preconditions, &s.RequiresQuote,
			&s.IsActive, &s.CreatedAt, &s.UpdatedAt); err == nil {
			_ = json.Unmarshal(includes, &s.Includes)
			services = append(services, s)
		}
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"services": services})
}

// Récupérer un service par son ID
const requeteLireService = `
	SELECT id, code, template_code, name, description, family_code, duration_minutes,
	       sort_order, includes, equipment_notes, safety_notes, preconditions,
	       requires_quote, is_active, created_at, updated_at
	FROM services
	WHERE id = $1 AND organization_id = $2`

func (a *App) lireService(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	serviceID := cheminParam(r, "/api/v1/services/")
	if serviceID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de service invalide.")
		return
	}

	var s Service
	var includes []byte
	err := a.db.QueryRow(r.Context(), requeteLireService, serviceID, *sess.OrganizationID).Scan(
		&s.ID, &s.Code, &s.TemplateCode, &s.Name, &s.Description, &s.FamilyCode,
		&s.DurationMinutes, &s.SortOrder, &includes, &s.EquipmentNotes, &s.SafetyNotes,
		&s.Preconditions, &s.RequiresQuote, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)

	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Service introuvable.")
		return
	}
	_ = json.Unmarshal(includes, &s.Includes)

	// Récupérer aussi les compatibilités
	compatRows, err := a.db.Query(r.Context(), `SELECT kind, code FROM service_compatibilities WHERE service_id = $1`, serviceID)
	if err == nil {
		defer compatRows.Close()
		compatibilites := []map[string]string{}
		for compatRows.Next() {
			var kind, code string
			if err := compatRows.Scan(&kind, &code); err == nil {
				compatibilites = append(compatibilites, map[string]string{"kind": kind, "code": code})
			}
		}
		ecrireJSON(w, http.StatusOK, map[string]interface{}{"service": s, "compatibilities": compatibilites})
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"service": s, "compatibilities": []interface{}{}})
}

// Créer un nouveau service
const requeteCreerService = `
	INSERT INTO services (organization_id, station_id, code, template_code, name, description,
	                      family_code, duration_minutes, sort_order, includes, equipment_notes,
	                      safety_notes, preconditions, requires_quote)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	RETURNING id, created_at, updated_at`

func (a *App) creerService(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !a.stationActive(r.Context(), *sess.OrganizationID) {
		repondreErreur(w, http.StatusConflict, "onboarding_incomplete", "La station n'est pas encore activée.")
		return
	}

	corps, ok := decoderCorps[CreerServiceRequest](w, r)
	if !ok {
		return
	}

	// Vérifier que le prix n'est pas fourni (règle Lot 2 : le prix est dans la grille)
	if corps.DurationMinutes < 0 {
		ecrireValidation(w, map[string]string{"duration_minutes": "La durée ne peut pas être négative."})
		return
	}

	// Vérifier que la station existe et appartient
	if err := a.verifierAppartenanceStation(r.Context(), corps.StationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	// Vérifier l'unicité du code sur la station
	var conflit bool
	a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM services WHERE station_id = $1 AND code = $2)`,
		corps.StationID, corps.Code).Scan(&conflit)
	if conflit {
		ecrireValidation(w, map[string]string{"code": "Ce code est déjà utilisé par un autre service de cette station."})
		return
	}

	includesJSON, _ := json.Marshal(corps.Includes)
	var id string
	var createdAt, updatedAt time.Time
	err := a.db.QueryRow(r.Context(), requeteCreerService,
		*sess.OrganizationID, corps.StationID, corps.Code, corps.TemplateCode, corps.Name,
		corps.Description, corps.FamilyCode, corps.DurationMinutes, corps.SortOrder,
		includesJSON, corps.EquipmentNotes, corps.SafetyNotes, corps.Preconditions,
		corps.RequiresQuote).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de créer le service.")
		return
	}

	ecrireJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"service": map[string]interface{}{
			"id":               id,
			"code":             corps.Code,
			"template_code":    corps.TemplateCode,
			"name":             corps.Name,
			"description":      corps.Description,
			"family_code":      corps.FamilyCode,
			"duration_minutes": corps.DurationMinutes,
			"sort_order":       corps.SortOrder,
			"includes":         corps.Includes,
			"equipment_notes":  corps.EquipmentNotes,
			"safety_notes":     corps.SafetyNotes,
			"preconditions":    corps.Preconditions,
			"requires_quote":   corps.RequiresQuote,
			"is_active":        true,
			"created_at":       createdAt,
			"updated_at":       updatedAt,
		},
	})
}

// Mettre à jour un service existant (PATCH)
func (a *App) modifierService(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	serviceID := cheminParam(r, "/api/v1/services/")
	if serviceID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de service invalide.")
		return
	}

	// Vérifier que le service existe et appartient
	var stationID string
	err := a.db.QueryRow(r.Context(), `SELECT station_id FROM services WHERE id = $1 AND organization_id = $2`,
		serviceID, *sess.OrganizationID).Scan(&stationID)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Service introuvable.")
		return
	}

	corps, ok := decoderCorpsLibre(w, r)
	if !ok {
		return
	}

	// Construire la requête UPDATE dynamiquement
	champs := []string{}
	valeurs := []interface{}{}
	idx := 1
	maj := map[string]interface{}{"updated_at": time.Now()}

	if raw, ok := corps["name"]; ok {
		var nom string
		if err := json.Unmarshal(raw, &nom); err == nil && nom != "" {
			champs = append(champs, fmt.Sprintf("name = $%d", idx))
			valeurs = append(valeurs, nom)
			maj["name"] = nom
			idx++
		}
	}
	if raw, ok := corps["description"]; ok {
		var desc string
		if err := json.Unmarshal(raw, &desc); err == nil {
			champs = append(champs, fmt.Sprintf("description = $%d", idx))
			valeurs = append(valeurs, desc)
			maj["description"] = desc
			idx++
		}
	}
	if raw, ok := corps["is_active"]; ok {
		var actif bool
		if err := json.Unmarshal(raw, &actif); err == nil {
			champs = append(champs, fmt.Sprintf("is_active = $%d", idx))
			valeurs = append(valeurs, actif)
			maj["is_active"] = actif
			idx++
		}
	}
	// Interdire toute modification de price_xof (règle Lot 2)
	if _, ok := corps["price_xof"]; ok {
		ecrireValidation(w, map[string]string{"price_xof": "Le prix des services appartient à la grille tarifaire (Lot 3), pas au catalogue."})
		return
	}

	if len(champs) == 0 {
		ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Aucun champ à mettre à jour."})
		return
	}

	champs = append(champs, "updated_at = NOW()")
	valeurs = append(valeurs, serviceID, *sess.OrganizationID)
	query := fmt.Sprintf("UPDATE services SET %s WHERE id = $%d AND organization_id = $%d",
		strings.Join(champs, ", "), idx, idx+1)

	_, err = a.db.Exec(r.Context(), query, valeurs...)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de mettre à jour le service.")
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true, "updates": maj})
}

// Désactiver un service (DELETE = désactivation, jamais suppression)
func (a *App) supprimerService(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	serviceID := cheminParam(r, "/api/v1/services/")
	if serviceID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de service invalide.")
		return
	}

	// Vérifier appartenance
	var existe bool
	err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM services WHERE id = $1 AND organization_id = $2)`,
		serviceID, *sess.OrganizationID).Scan(&existe)
	if err != nil || !existe {
		repondreErreur(w, http.StatusNotFound, "not_found", "Service introuvable.")
		return
	}

	// Désactiver (soft delete)
	_, err = a.db.Exec(r.Context(), `UPDATE services SET is_active = false, updated_at = NOW() WHERE id = $1`, serviceID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de désactiver le service.")
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Service désactivé."})
}

// Lire les compatibilités d'un service
const requeteLireCompatibilites = `
	SELECT kind, code FROM service_compatibilities WHERE service_id = $1`

func (a *App) lireCompatibilitesService(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	serviceID := cheminParam(r, "/api/v1/services/")
	serviceID = strings.TrimSuffix(serviceID, "/compatibilities")
	if serviceID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de service invalide.")
		return
	}

	// Vérifier appartenance
	var existe bool
	err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM services WHERE id = $1 AND organization_id = $2)`,
		serviceID, *sess.OrganizationID).Scan(&existe)
	if err != nil || !existe {
		repondreErreur(w, http.StatusNotFound, "not_found", "Service introuvable.")
		return
	}

	rows, err := a.db.Query(r.Context(), requeteLireCompatibilites, serviceID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les compatibilités.")
		return
	}
	defer rows.Close()

	compatibilites := []map[string]string{}
	for rows.Next() {
		var kind, code string
		if err := rows.Scan(&kind, &code); err == nil {
			compatibilites = append(compatibilites, map[string]string{"kind": kind, "code": code})
		}
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"compatibilities": compatibilites})
}

// Mettre à jour les compatibilités d'un service (remplace tout)
func (a *App) mettreAJourCompatibilitesService(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	serviceID := cheminParam(r, "/api/v1/services/")
	serviceID = strings.TrimSuffix(serviceID, "/compatibilities")
	if serviceID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant de service invalide.")
		return
	}

	// Vérifier appartenance
	var existe bool
	err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM services WHERE id = $1 AND organization_id = $2)`,
		serviceID, *sess.OrganizationID).Scan(&existe)
	if err != nil || !existe {
		repondreErreur(w, http.StatusNotFound, "not_found", "Service introuvable.")
		return
	}

	corps, ok := decoderCorps[MettreAJourCompatibilitesRequest](w, r)
	if !ok {
		return
	}

	// Supprimer les anciennes compatibilités
	_, err = a.db.Exec(r.Context(), `DELETE FROM service_compatibilities WHERE service_id = $1`, serviceID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de mettre à jour les compatibilités.")
		return
	}

	// Insérer les nouvelles
	tx, err := a.db.Begin(r.Context())
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de démarrer la transaction.")
		return
	}
	defer tx.Rollback(r.Context())

	for _, c := range corps.Compatibilities {
		if c.Kind != "category" && c.Kind != "usage" {
			ecrireValidation(w, map[string]string{"compatibilities[%d].kind": "Le type doit être 'category' ou 'usage'."})
			return
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO service_compatibilities (service_id, kind, code) VALUES ($1, $2, $3)`,
			serviceID, c.Kind, c.Code)
		if err != nil {
			ecrireValidation(w, map[string]string{"compatibilities": fmt.Sprintf("Erreur sur %s/%s : %v", c.Kind, c.Code, err)})
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de valider la transaction.")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de valider la transaction.")
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true, "updated": len(corps.Compatibilities)})
}

// --- Options de service ---

// Lister toutes les options d'une station
const requeteListerOptions = `
	SELECT id, category_code, service_id, name, description, price_xof, duration_minutes, sort_order, is_active
	FROM service_options
	WHERE station_id = $1 AND organization_id = $2
	ORDER BY sort_order, name`

func (a *App) listerOptions(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		ecrireValidation(w, map[string]string{"station_id": "L'identifiant de station est requis."})
		return
	}

	// Vérifier appartenance
	if err := a.verifierAppartenanceStation(r.Context(), stationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	rows, err := a.db.Query(r.Context(), requeteListerOptions, stationID, *sess.OrganizationID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de charger les options.")
		return
	}
	defer rows.Close()

	options := []ServiceOption{}
	for rows.Next() {
		var o ServiceOption
		var serviceID pgtype.UUID
		if err := rows.Scan(&o.ID, &o.CategoryCode, &serviceID, &o.Name, &o.Description,
			&o.PriceXOF, &o.DurationMinutes, &o.SortOrder, &o.IsActive); err == nil {
			if serviceID.Valid {
				// Convertir pgtype.UUID en string UUID standard
				sid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
					serviceID.Bytes[0:4], serviceID.Bytes[4:6], serviceID.Bytes[6:8],
					serviceID.Bytes[8:10], serviceID.Bytes[10:16])
				o.ServiceID = &sid
			}
			options = append(options, o)
		}
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"options": options})
}

// Créer une nouvelle option
const requeteCreerOption = `
	INSERT INTO service_options (organization_id, station_id, service_id, category_code, name,
	                             description, price_xof, duration_minutes, sort_order)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING id`

func (a *App) creerOption(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !a.stationActive(r.Context(), *sess.OrganizationID) {
		repondreErreur(w, http.StatusConflict, "onboarding_incomplete", "La station n'est pas encore activée.")
		return
	}

	corps, ok := decoderCorps[CreerOptionRequest](w, r)
	if !ok {
		return
	}

	if err := a.verifierAppartenanceStation(r.Context(), corps.StationID, *sess.OrganizationID); err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}

	if corps.PriceXOF < 0 {
		ecrireValidation(w, map[string]string{"price_xof": "Le prix ne peut pas être négatif."})
		return
	}

	var serviceID *pgtype.UUID
	if corps.ServiceID != nil {
		sid := pgtype.UUID{}
		if err := sid.Scan(*corps.ServiceID); err == nil {
			serviceID = &sid
		}
	}

	var id string
	err := a.db.QueryRow(r.Context(), requeteCreerOption,
		*sess.OrganizationID, corps.StationID, serviceID, corps.CategoryCode,
		corps.Name, corps.Description, corps.PriceXOF, corps.DurationMinutes,
		corps.SortOrder).Scan(&id)

	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de créer l'option.")
		return
	}

	ecrireJSON(w, http.StatusCreated, map[string]interface{}{"success": true, "id": id})
}

// Mettre à jour une option existante
func (a *App) modifierOption(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	optionID := cheminParam(r, "/api/v1/service-options/")
	if optionID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant d'option invalide.")
		return
	}

	// Vérifier appartenance
	var existe bool
	err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM service_options WHERE id = $1 AND organization_id = $2)`,
		optionID, *sess.OrganizationID).Scan(&existe)
	if err != nil || !existe {
		repondreErreur(w, http.StatusNotFound, "not_found", "Option introuvable.")
		return
	}

	corps, ok := decoderCorpsLibre(w, r)
	if !ok {
		return
	}

	champs := []string{}
	valeurs := []interface{}{}
	idx := 1

	if raw, ok := corps["name"]; ok {
		var nom string
		if err := json.Unmarshal(raw, &nom); err == nil && nom != "" {
			champs = append(champs, fmt.Sprintf("name = $%d", idx))
			valeurs = append(valeurs, nom)
			idx++
		}
	}
	if raw, ok := corps["price_xof"]; ok {
		var prix int
		if err := json.Unmarshal(raw, &prix); err == nil {
			if prix < 0 {
				ecrireValidation(w, map[string]string{"price_xof": "Le prix ne peut pas être négatif."})
				return
			}
			champs = append(champs, fmt.Sprintf("price_xof = $%d", idx))
			valeurs = append(valeurs, prix)
			idx++
		}
	}
	if raw, ok := corps["is_active"]; ok {
		var actif bool
		if err := json.Unmarshal(raw, &actif); err == nil {
			champs = append(champs, fmt.Sprintf("is_active = $%d", idx))
			valeurs = append(valeurs, actif)
			idx++
		}
	}

	if len(champs) == 0 {
		ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Aucun champ à mettre à jour."})
		return
	}

	champs = append(champs, "updated_at = NOW()")
	valeurs = append(valeurs, optionID, *sess.OrganizationID)
	query := fmt.Sprintf("UPDATE service_options SET %s WHERE id = $%d AND organization_id = $%d",
		strings.Join(champs, ", "), idx, idx+1)

	_, err = a.db.Exec(r.Context(), query, valeurs...)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de mettre à jour l'option.")
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}

// Désactiver une option
func (a *App) supprimerOption(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	optionID := cheminParam(r, "/api/v1/service-options/")
	if optionID == "" {
		repondreErreur(w, http.StatusBadRequest, "invalid_path", "Identifiant d'option invalide.")
		return
	}

	// Vérifier appartenance
	var existe bool
	err := a.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM service_options WHERE id = $1 AND organization_id = $2)`,
		optionID, *sess.OrganizationID).Scan(&existe)
	if err != nil || !existe {
		repondreErreur(w, http.StatusNotFound, "not_found", "Option introuvable.")
		return
	}

	_, err = a.db.Exec(r.Context(), `UPDATE service_options SET is_active = false, updated_at = NOW() WHERE id = $1`, optionID)
	if err != nil {
		repondreErreur(w, http.StatusInternalServerError, "server_error", "Impossible de désactiver l'option.")
		return
	}

	ecrireJSON(w, http.StatusOK, map[string]interface{}{"success": true, "message": "Option désactivée."})
}

// Fonction utilitaire pour vérifier qu'une station appartient bien à l'organisation
func (a *App) verifierAppartenanceStation(ctx context.Context, stationID, orgID string) error {
	var compte int
	err := a.db.QueryRow(ctx, `SELECT COUNT(*) FROM stations WHERE id = $1 AND organization_id = $2`,
		stationID, orgID).Scan(&compte)
	if err != nil || compte != 1 {
		return fmt.Errorf("station n'appartient pas à l'organisation")
	}
	return nil
}
