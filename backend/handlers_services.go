package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Catalogue de prestations (lecture + création).
// ---------------------------------------------------------------------------

// serviceColumns : les colonnes d'une prestation, préfixées pour les jointures.
// `family_name` vient du référentiel (ou du nom local s'il a été renommé).
const serviceColumns = `s.id, s.organization_id, s.station_id, s.name, s.category, s.description,
	s.price_xof, s.duration_minutes, s.sort_order, s.is_active,
	s.code, s.family_code, COALESCE(f.name, s.family_code) AS family_name, s.template_code,
	s.includes, s.equipment_notes, s.safety_notes, s.preconditions, s.is_free, s.is_quote_only`

// serviceColumnsSansAlias : mêmes colonnes quand `services` n'est pas jointe.
const serviceColumnsSansAlias = `id, organization_id, station_id, name, category, description,
	price_xof, duration_minutes, sort_order, is_active,
	code, family_code, family_code AS family_name, template_code,
	includes, equipment_notes, safety_notes, preconditions, is_free, is_quote_only`

func scanService(row scannable) (Service, error) {
	var s Service
	err := row.Scan(&s.ID, &s.OrganizationID, &s.StationID, &s.Name, &s.Category, &s.Description,
		&s.PriceXOF, &s.DurationMinutes, &s.SortOrder, &s.IsActive,
		&s.Code, &s.FamilyCode, &s.FamilyName, &s.TemplateCode,
		&s.Includes, &s.EquipmentNotes, &s.SafetyNotes, &s.Preconditions,
		&s.IsFree, &s.IsQuoteOnly)
	return s, err
}

// conditionPrestationVendableSansPrix : **la** définition de « il reste un prix
// à poser ». Elle est construite par une fonction parce que trois endroits s'en
// servent — la liste des prestations, la matrice tarifaire, l'étape « tarifs »
// du parcours — et que deux copies auraient fini par diverger.
//
// Deux exclusions, et ce sont des règles de gestion, pas des détails :
//   - une prestation **gratuite** n'a pas de prix à saisir ;
//   - une prestation **sur devis** se chiffre au cas par cas, jamais au
//     comptoir : la compter comme « à tarifer » réclamerait un prix qui n'a pas
//     lieu d'être, et l'écran resterait marqué incomplet pour toujours.
//
// `expressionGenerique` dit si le tarif du catalogue compte pour cette station
// (chaque entreprise choisit de vendre au catalogue ou uniquement par règles).
func conditionPrestationVendableSansPrix(expressionGenerique string) string {
	return `NOT s.is_free AND NOT s.is_quote_only
	 AND (NOT (` + expressionGenerique + `) OR s.price_xof <= 0)
	 AND NOT EXISTS (SELECT 1 FROM price_rules r
	                  WHERE r.service_id = s.id AND r.is_active
	                    AND (r.valid_from IS NULL OR r.valid_from <= CURRENT_DATE)
	                    AND (r.valid_to IS NULL OR r.valid_to >= CURRENT_DATE))`
}

// serviceColumnsAvecPrix : colonnes + « cette prestation a-t-elle un tarif
// utilisable ? ». $4 dit si le tarif générique du catalogue s'applique ; une
// règle de prix active supplée toujours un tarif générique absent.
var serviceColumnsAvecPrix = serviceColumns + `,
	(` + conditionPrestationVendableSansPrix("$4") + `) AS needs_price`

func scanServiceAvecPrix(row scannable) (Service, error) {
	var s Service
	err := row.Scan(&s.ID, &s.OrganizationID, &s.StationID, &s.Name, &s.Category, &s.Description,
		&s.PriceXOF, &s.DurationMinutes, &s.SortOrder, &s.IsActive,
		&s.Code, &s.FamilyCode, &s.FamilyName, &s.TemplateCode,
		&s.Includes, &s.EquipmentNotes, &s.SafetyNotes, &s.Preconditions,
		&s.IsFree, &s.IsQuoteOnly, &s.NeedsPrice)
	return s, err
}

func (a *App) handleServicesList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	includeInactive := r.URL.Query().Get("include_inactive") == "true"

	// Le tarif générique du catalogue compte comme un tarif utilisable, sauf si
	// la station a choisi de ne vendre que par règles de prix.
	generiqueAutorise := true
	if stationID != "" {
		generiqueAutorise = a.stationAutorisePrixGenerique(r.Context(), stationID)
	}

	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+serviceColumnsAvecPrix+`
		FROM services s
		LEFT JOIN service_family_templates f ON f.code = s.family_code
		WHERE s.organization_id = $1
		  AND ($2 = '' OR s.station_id = $2)
		  AND ($3 OR s.is_active)
		ORDER BY s.sort_order, s.name`,
		deref(user.OrganizationID), stationID, includeInactive, generiqueAutorise)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []Service{}
	categoriesSet := map[string]bool{}
	famillesSet := map[string]bool{}
	aTarifer := 0
	for rows.Next() {
		s, err := scanServiceAvecPrix(rows)
		if err != nil {
			continue
		}
		items = append(items, s)
		categoriesSet[s.Category] = true
		if s.FamilyCode != "" {
			famillesSet[s.FamilyCode] = true
		}
		if s.NeedsPrice {
			aTarifer++
		}
	}
	categories := []string{}
	for c := range categoriesSet {
		categories = append(categories, c)
	}
	familles := []string{}
	for f := range famillesSet {
		familles = append(familles, f)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "categories": categories, "familles": familles,
		"total": len(items), "a_tarifer": aTarifer,
	})
}

// serviceDraft : corps de création d'une prestation. Les pointeurs distinguent
// « champ absent » de « valeur zéro » — un tarif absent n'est pas un tarif nul.
type serviceDraft struct {
	StationID       string   `json:"station_id"`
	Name            string   `json:"name"`
	Category        string   `json:"category"`
	Description     string   `json:"description"`
	PriceXOF        *int     `json:"price_xof"`
	DurationMinutes *int     `json:"duration_minutes"`
	SortOrder       *int     `json:"sort_order"`
	FamilyCode      string   `json:"family_code"`
	Code            string   `json:"code"`
	TemplateCode    string   `json:"template_code"`
	Includes        []string `json:"includes"`
	EquipmentNotes  string   `json:"equipment_notes"`
	SafetyNotes     string   `json:"safety_notes"`
	Preconditions   string   `json:"preconditions"`
	IsFree          bool     `json:"is_free"`
	IsQuoteOnly     bool     `json:"is_quote_only"`
	// Catégories de véhicules auxquelles la prestation s'applique. Vide à la
	// création depuis un modèle : on reprend les catégories du modèle.
	Categories []string `json:"categories"`
}

// handleServiceCreate crée une prestation — de zéro, ou depuis un **modèle**
// (`template_code`) quand l'exploitant veut partir du catalogue de référence.
//
// Un modèle n'apporte **aucun prix** : la prestation créée naît « à tarifer »
// et le serveur refusera de la vendre tant qu'aucun tarif ne la couvre.
func (a *App) handleServiceCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body serviceDraft
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	// Un modèle fournit le libellé, le contenu et la durée ; l'exploitant garde
	// la main sur tout ce qu'il précise lui-même.
	var modele *modelePrestation
	if body.TemplateCode != "" {
		trouve, err := a.modelePrestation(r.Context(), body.TemplateCode)
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Modèle de prestation inconnu.",
				map[string]string{"template_code": "Modèle inconnu"})
			return
		}
		modele = &trouve
	}
	nom := strings.TrimSpace(body.Name)
	if nom == "" && modele != nil {
		nom = modele.Nom
	}
	if nom == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le nom de la prestation est obligatoire.",
			map[string]string{"name": "Nom obligatoire."})
		return
	}

	stationID, err := a.resolveStation(r.Context(), user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}

	// Aucun prix ne s'écrit ici : le tarif du catalogue se pose dans la grille
	// tarifaire, avec son motif et sa trace d'audit. Un catalogue qui
	// s'auto-tariferait à la création rendrait cette trace incomplète.
	if body.PriceXOF != nil && *body.PriceXOF != 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le tarif d'une prestation se pose dans la grille tarifaire, avec son motif.",
			map[string]string{"price_xof": "Passez par la grille tarifaire."})
		return
	}
	// « Gratuit » est une décision : elle ne cohabite pas avec un tarif.
	prix := 0

	category := body.Category
	if category == "" {
		category = "lavage"
	}
	duration := 30
	if modele != nil && modele.DureeMinutes > 0 {
		duration = modele.DureeMinutes
	}
	if body.DurationMinutes != nil && *body.DurationMinutes > 0 {
		duration = *body.DurationMinutes
	}
	sortOrder := 0
	if body.SortOrder != nil {
		sortOrder = *body.SortOrder
	}
	famille := body.FamilyCode
	if famille == "" && modele != nil {
		famille = modele.FamilleCode
	}
	includes := body.Includes
	if len(includes) == 0 && modele != nil {
		includes = modele.Includes
	}
	if includes == nil {
		includes = []string{}
	}
	code := strings.TrimSpace(body.Code)
	if code == "" {
		code = slugify(nom)
	}
	description := body.Description
	if description == "" && modele != nil {
		description = modele.Description
	}

	id := newID("srv")
	_, err = a.Pool.Exec(r.Context(), `
		INSERT INTO services (id, organization_id, station_id, name, category, description,
		  price_xof, duration_minutes, sort_order, code, family_code, template_code, includes,
		  equipment_notes, safety_notes, preconditions, is_free, is_quote_only)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		id, deref(user.OrganizationID), stationID, nom, category, description,
		prix, duration, sortOrder, code, famille, body.TemplateCode, includes,
		body.EquipmentNotes, body.SafetyNotes, body.Preconditions, body.IsFree, body.IsQuoteOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal",
			"Erreur lors de la création de la prestation.", nil)
		return
	}

	// Compatibilités : celles choisies, sinon celles du modèle, restreintes aux
	// catégories réellement activées par la station.
	categories := body.Categories
	if len(categories) == 0 && modele != nil {
		categories = modele.Categories
	}
	if err := a.enregistrerCompatibilites(r.Context(), deref(user.OrganizationID), stationID, id, categories); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"service.created", "service", id,
		map[string]any{"name": nom, "price_xof": prix, "template_code": body.TemplateCode,
			"is_free": body.IsFree, "actor_role": user.Role})

	cree, err := a.serviceAvecPrix(r.Context(), id, deref(user.OrganizationID), stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusCreated, cree)
}

// serviceAvecPrix relit une prestation avec son état de tarification : c'est ce
// que l'écran affiche (« à tarifer » ou le tarif en vigueur).
func (a *App) serviceAvecPrix(ctx context.Context, id, orgID, stationID string) (Service, error) {
	service, err := scanService(a.Pool.QueryRow(ctx, `
		SELECT `+serviceColumns+`
		FROM services s
		LEFT JOIN service_family_templates f ON f.code = s.family_code
		WHERE s.id = $1 AND s.organization_id = $2`, id, orgID))
	if err != nil {
		return service, err
	}
	service.NeedsPrice = a.prestationSansTarif(ctx, service, stationID)
	return service, nil
}

// getOwnedService charge une prestation en vérifiant l'entreprise.
func (a *App) getOwnedService(r *http.Request, id, orgID string) (Service, error) {
	s, err := scanService(a.Pool.QueryRow(r.Context(), `
		SELECT `+serviceColumns+`
		FROM services s
		LEFT JOIN service_family_templates f ON f.code = s.family_code
		WHERE s.id = $1 AND s.organization_id = $2`, id, orgID))
	if err == pgx.ErrNoRows {
		return s, errNotFound
	}
	if err != nil {
		return s, err
	}
	return s, nil
}

var errNotFound = &apiError{Status: http.StatusNotFound, Code: "not_found", Message: "Ressource introuvable."}
