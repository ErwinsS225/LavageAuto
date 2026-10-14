package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// routes_vehicules.go — référentiels véhicules et usages (Lot 1).
//
// Deux référentiels distincts : la **catégorie** décrit le véhicule (berline,
// moto, gbaka), l'**usage** décrit ce qu'on en fait (taxi compteur, livraison).
// Le référentiel du produit (organization_id NULL) est amorcé au démarrage et
// n'est jamais modifié par une station ; ce qu'une station en fait vit dans
// station_vehicle_categories / station_vehicle_usages (is_active par code) et
// ses catégories propres portent un code préfixé perso_ (organization_id = org).
//
// Les routes d'activation en bloc (PUT /stations/{id}/vehicle-*) existent déjà
// dans routes_stations.go ; ce fichier ajoute les lectures et la gestion des
// catégories propres à la station.

// categorieStation est une ligne du référentiel vue par une station.
type categorieStation struct {
	ID        string
	Code      string
	Kind      string // modele | personnalisee
	Name      string
	Desc      string
	Family    string
	Aliases   []string
	SortOrder int
	IsActive  bool
}

const requeteCategoriesStation = `
	SELECT c.id, c.code, c.name, c.description, c.family, c.aliases, c.sort_order,
	       COALESCE(s.is_active, c.is_active) AS is_active,
	       CASE WHEN c.organization_id IS NULL THEN 'modele' ELSE 'personnalisee' END
	FROM vehicle_categories c
	LEFT JOIN station_vehicle_categories s
	       ON s.category_code = c.code AND s.station_id = $1
	WHERE c.organization_id IS NULL OR c.organization_id = $2
	ORDER BY c.sort_order, c.name`

func (a *App) lireCategoriesStation(ctx context.Context, stationID, orgID string) []categorieStation {
	rows, err := a.db.Query(ctx, requeteCategoriesStation, stationID, orgID)
	liste := []categorieStation{}
	if err != nil {
		return liste
	}
	defer rows.Close()
	for rows.Next() {
		var c categorieStation
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.Desc, &c.Family, &c.Aliases, &c.SortOrder, &c.IsActive, &c.Kind); err == nil {
			liste = append(liste, c)
		}
	}
	return liste
}

const requeteUsagesStation = `
	SELECT u.id, u.code, u.name, u.description, u.sort_order,
	       COALESCE(s.is_active, u.is_active) AS is_active,
	       CASE WHEN u.organization_id IS NULL THEN 'modele' ELSE 'personnalisee' END
	FROM vehicle_usages u
	LEFT JOIN station_vehicle_usages s
	       ON s.usage_code = u.code AND s.station_id = $1
	WHERE u.organization_id IS NULL OR u.organization_id = $2
	ORDER BY u.sort_order, u.name`

func (a *App) lireUsagesStation(ctx context.Context, stationID, orgID string) []map[string]interface{} {
	rows, err := a.db.Query(ctx, requeteUsagesStation, stationID, orgID)
	liste := []map[string]interface{}{}
	if err != nil {
		return liste
	}
	defer rows.Close()
	for rows.Next() {
		var id, code, nom, desc, kind string
		var ordre int
		var actif bool
		if err := rows.Scan(&id, &code, &nom, &desc, &ordre, &actif, &kind); err != nil {
			continue
		}
		liste = append(liste, map[string]interface{}{
			"id": id, "code": code, "kind": kind, "name": nom, "family": "",
			"description": desc, "aliases": []string{}, "sort_order": ordre,
			"is_active": actif, "name_from_template": kind == "modele",
		})
	}
	return liste
}

func (c categorieStation) rendre() map[string]interface{} {
	aliases := c.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	return map[string]interface{}{
		"id": c.ID, "code": c.Code, "kind": c.Kind, "name": c.Name,
		"description": c.Desc, "family": c.Family, "aliases": aliases,
		"sort_order": c.SortOrder, "is_active": c.IsActive,
		"name_from_template": c.Kind == "modele",
	}
}

func rendreCategories(liste []categorieStation) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, c := range liste {
		out = append(out, c.rendre())
	}
	return out
}

// stationSessionPourVehicules résout la station visée (query ou session).
func (a *App) stationPourVehicules(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) (*stationEntite, bool) {
	id := strings.TrimSpace(r.URL.Query().Get("station_id"))
	if id == "" {
		id = orElse(sess.StationID, "")
	}
	if id == "" {
		repondreErreur(w, http.StatusBadRequest, "validation_error", "Identifiant de station requis.")
		return nil, false
	}
	st, ok := a.stationVerifiee(r.Context(), sess, id)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return nil, false
	}
	return st, true
}

// GET /vehicle-catalogue — le référentiel complet + l'état local de la station.
func (a *App) catalogueVehicules(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("station:view") && !sess.aPermission("settings:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	st, ok := a.stationPourVehicules(w, r, sess)
	if !ok {
		return
	}
	ctx := r.Context()
	categories := a.lireCategoriesStation(ctx, st.ID, st.OrganizationID)
	usages := a.lireUsagesStation(ctx, st.ID, st.OrganizationID)
	actives, usagesActifs := 0, 0
	famillesCats := []string{}
	famillesUsages := []string{}
	vuesC := map[string]bool{}
	for _, c := range categories {
		if c.IsActive {
			actives++
		}
		if c.Family != "" && !vuesC[c.Family] {
			vuesC[c.Family] = true
			famillesCats = append(famillesCats, c.Family)
		}
	}
	for _, u := range usages {
		if u["is_active"].(bool) {
			usagesActifs++
		}
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"station_id":          st.ID,
		"categories":          rendreCategories(categories),
		"usages":              usages,
		"actives":             actives,
		"usages_actifs":       usagesActifs,
		"familles_categories": famillesCats,
		"familles_usages":     famillesUsages,
		"message_referentiel": "Le référentiel est celui du produit ; la station choisit ce qu'elle propose et nomme localement ses catégories propres.",
	})
}

// GET /vehicle-categories?station_id=&actives=1 — les catégories de la station.
func (a *App) listerCategories(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("station:view") && !sess.aPermission("settings:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	st, ok := a.stationPourVehicules(w, r, sess)
	if !ok {
		return
	}
	liste := a.lireCategoriesStation(r.Context(), st.ID, st.OrganizationID)
	if r.URL.Query().Get("actives") == "1" {
		filtrees := []categorieStation{}
		for _, c := range liste {
			if c.IsActive {
				filtrees = append(filtrees, c)
			}
		}
		liste = filtrees
	}
	ecrireJSON(w, http.StatusOK, rendreCategories(liste))
}

// GET /vehicle-usages?station_id=&actives=1 — les usages de la station.
func (a *App) listerUsages(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("station:view") && !sess.aPermission("settings:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	st, ok := a.stationPourVehicules(w, r, sess)
	if !ok {
		return
	}
	liste := a.lireUsagesStation(r.Context(), st.ID, st.OrganizationID)
	if r.URL.Query().Get("actives") == "1" {
		filtrees := []map[string]interface{}{}
		for _, u := range liste {
			if u["is_active"].(bool) {
				filtrees = append(filtrees, u)
			}
		}
		liste = filtrees
	}
	ecrireJSON(w, http.StatusOK, liste)
}

// GET /vehicle-category-options — les deux listes actives en un appel (caisse).
func (a *App) optionsVehicules(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("wash:create") && !sess.aPermission("station:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	st, ok := a.stationPourVehicules(w, r, sess)
	if !ok {
		return
	}
	ctx := r.Context()
	categories := rendreCategories(a.lireCategoriesStation(ctx, st.ID, st.OrganizationID))
	actives := []map[string]interface{}{}
	for _, c := range categories {
		if c["is_active"].(bool) {
			actives = append(actives, c)
		}
	}
	usages := a.lireUsagesStation(ctx, st.ID, st.OrganizationID)
	usagesActifs := []map[string]interface{}{}
	for _, u := range usages {
		if u["is_active"].(bool) {
			usagesActifs = append(usagesActifs, u)
		}
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"categories": actives, "usages": usagesActifs})
}

// GET /vehicle-usage-categories?station_id= — les combinaisons déclarées.
func (a *App) listerCouples(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("station:view") && !sess.aPermission("settings:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	st, ok := a.stationPourVehicules(w, r, sess)
	if !ok {
		return
	}
	rows, err := a.db.Query(r.Context(), `
		SELECT sc.usage_code, sc.category_code, true
		FROM station_usage_categories sc
		JOIN station_vehicle_usages su ON su.station_id = sc.station_id AND su.usage_code = sc.usage_code AND su.is_active
		JOIN station_vehicle_categories scc ON scc.station_id = sc.station_id AND scc.category_code = sc.category_code AND scc.is_active
		WHERE sc.station_id = $1
		ORDER BY sc.usage_code, sc.category_code`, st.ID)
	items := []map[string]interface{}{}
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var u, c string
			var actif bool
			if err := rows.Scan(&u, &c, &actif); err == nil {
				items = append(items, map[string]interface{}{
					"usage_code": u, "category_code": c, "is_active": actif, "sort_order": 0,
				})
			}
		}
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"items": items, "total": len(items), "station_id": st.ID,
	})
}

// POST /vehicle-categories — catégorie propre à la station (code perso_…).
func (a *App) creerCategorie(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	corps, ok := decoderCorps[struct {
		StationID   string   `json:"station_id"`
		Name        string   `json:"name"`
		Family      string   `json:"family"`
		Description string   `json:"description"`
		Aliases     []string `json:"aliases"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	st, ok := a.stationVerifiee(ctx, sess, strings.TrimSpace(corps.StationID))
	if !ok {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}
	nom := strings.TrimSpace(corps.Name)
	champs := map[string]string{}
	if len(nom) < 2 {
		champs["name"] = "Nom de la catégorie requis."
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}
	// Code unique, préfixé perso_ : une entrée ajoutée plus tard au référentiel
	// du produit ne peut pas écraser celui de la station.
	code := slugDe(nom)
	if code == "" {
		code = "categorie"
	}
	code = "perso_" + code
	connues := referentielCodes(ctx, a, `vehicle_categories`)
	base := code
	for i := 2; connues[code]; i++ { // collision théorique avec le référentiel produit
		code = fmt.Sprintf("%s-%d", base, i)
	}
	for n := 2; ; n++ {
		var existe bool
		_ = a.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vehicle_categories WHERE organization_id = $1 AND code = $2)`,
			st.OrganizationID, code).Scan(&existe)
		if !existe {
			break
		}
		code = fmt.Sprintf("%s-%d", base, n)
	}
	var id string
	err := a.db.QueryRow(ctx, `
		INSERT INTO vehicle_categories (organization_id, code, name, family, description, aliases, sort_order, is_active)
		VALUES ($1, $2, $3, $4, $5, COALESCE($6, ARRAY[]::text[]), 900, true)
		RETURNING id`, st.OrganizationID, code, nom, strings.TrimSpace(corps.Family),
		strings.TrimSpace(corps.Description), corps.Aliases).Scan(&id)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	// La nouvelle catégorie est proposée active pour la station créatrice.
	_, _ = a.db.Exec(ctx, `
		INSERT INTO station_vehicle_categories (station_id, category_code, is_active)
		VALUES ($1, $2, true) ON CONFLICT (station_id, category_code) DO UPDATE SET is_active = true`,
		st.ID, code)
	a.journaliser(ctx, &st.OrganizationID, &st.ID, &sess.UserID, "vehicle_category.create",
		map[string]interface{}{"code": code, "name": nom})
	liste := a.lireCategoriesStation(ctx, st.ID, st.OrganizationID)
	ecrireJSON(w, http.StatusCreated, map[string]interface{}{"items": rendreCategories(liste)})
}

// PATCH /vehicle-categories/{code} — renommer/réordonner/activer.
//
// Le renommage ne touche que les catégories propres à la station (perso_…) :
// le référentiel du produit appartient à tous. Pour une entrée du référentiel,
// seul l'état local (is_active) de la station de la session est modifié.
func (a *App) modifierCategorie(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	code := strings.TrimSpace(cheminParam(r, "/api/v1/vehicle-categories/"))
	if code == "" {
		repondreErreur(w, http.StatusNotFound, "not_found", "Catégorie introuvable.")
		return
	}
	corps, ok := decoderCorpsLibre(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	orgID := orElse(sess.OrganizationID, "")
	stationID := orElse(sess.StationID, "")
	if sid := strings.TrimSpace(chaineCorps(corps, "station_id")); sid != "" {
		if st, ok := a.stationVerifiee(ctx, sess, sid); ok {
			stationID = st.ID
		}
	}
	if stationID == "" {
		repondreErreur(w, http.StatusBadRequest, "validation_error", "Station requise.")
		return
	}

	// La catégorie visée existe-t-elle côté organisation (perso_) ?
	var id, kind string
	err := a.db.QueryRow(ctx, `
		SELECT id, CASE WHEN organization_id IS NULL THEN 'modele' ELSE 'personnalisee' END
		FROM vehicle_categories WHERE code = $1 AND (organization_id = $2 OR organization_id IS NULL)
		ORDER BY organization_id NULLS LAST LIMIT 1`, code, orgID).Scan(&id, &kind)
	if err != nil {
		repondreErreur(w, http.StatusNotFound, "not_found", "Catégorie introuvable.")
		return
	}

	if kind == "personnalisee" {
		sets := []string{}
		args := []interface{}{}
		for _, champ := range []string{"name", "description", "family"} {
			if _, present := corps[champ]; present {
				v := strings.TrimSpace(chaineCorps(corps, champ))
				sets = append(sets, fmt.Sprintf("%s = $%d", champ, len(args)+1))
				args = append(args, v)
			}
		}
		if _, present := corps["sort_order"]; present {
			var n int
			if jsonNombre(corps["sort_order"], &n) {
				sets = append(sets, fmt.Sprintf("sort_order = $%d", len(args)+1))
				args = append(args, n)
			}
		}
		if _, present := corps["is_active"]; present {
			var b bool
			if jsonNombre(corps["is_active"], &b) {
				sets = append(sets, fmt.Sprintf("is_active = $%d", len(args)+1))
				args = append(args, b)
			}
		}
		if len(sets) > 0 {
			args = append(args, id)
			if _, err := a.db.Exec(ctx, `UPDATE vehicle_categories SET `+strings.Join(sets, ", ")+` WHERE id = $`+fmt.Sprint(len(args)), args...); err != nil {
				ecrireInterne(w, err)
				return
			}
		}
		a.journaliser(ctx, &orgID, nil, &sess.UserID, "vehicle_category.update",
			map[string]interface{}{"code": code})
	} else {
		// Entrée du référentiel produit : pas de renommage, seulement l'état local.
		renommageDemande := false
		for _, champ := range []string{"name", "description", "family", "aliases", "sort_order"} {
			if _, ok := corps[champ]; ok {
				renommageDemande = true
			}
		}
		if renommageDemande {
			ecrireValidation(w, map[string]string{
				"name": "Le référentiel du produit est commun : créez une catégorie propre à la station pour la nommer autrement.",
			})
			return
		}
		if _, present := corps["is_active"]; present {
			var b bool
			if jsonNombre(corps["is_active"], &b) {
				_, err := a.db.Exec(ctx, `
					INSERT INTO station_vehicle_categories (station_id, category_code, is_active)
					VALUES ($1, $2, $3)
					ON CONFLICT (station_id, category_code) DO UPDATE SET is_active = EXCLUDED.is_active`,
					stationID, code, b)
				if err != nil {
					ecrireInterne(w, err)
					return
				}
			}
		}
		a.journaliser(ctx, &orgID, &stationID, &sess.UserID, "vehicle_category.local_update",
			map[string]interface{}{"code": code})
	}
	liste := a.lireCategoriesStation(ctx, stationID, orgID)
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"items": rendreCategories(liste)})
}

// --- petits utilitaires de corps --------------------------------------------

func chaineCorps(corps map[string]json.RawMessage, cle string) string {
	brut, ok := corps[cle]
	if !ok {
		return ""
	}
	var v string
	_ = json.Unmarshal(brut, &v)
	return v
}

// jsonNombre décode un JSON brut dans un type simple (int, string, bool…).
func jsonNombre(brut json.RawMessage, cible interface{}) bool {
	return json.Unmarshal(brut, cible) == nil
}
