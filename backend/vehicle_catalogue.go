package main

import (
	"context"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Catégories de véhicules et usages commerciaux.
//
// Deux référentiels distincts :
//
//   - la **catégorie** décrit le véhicule (berline, gbaka, moto) ;
//   - l'**usage** décrit ce qu'on en fait (voiture personnelle, taxi compteur,
//     livraison…).
//
// Une berline « taxi compteur » reste une berline : la caisse peut donc compter
// ses berlines **et** savoir combien d'entre elles étaient des taxis. Mélanger
// les deux obligerait à créer « Berline taxi » et « Berline personnelle », et le
// rapport perdrait l'un des deux angles.
//
// Le catalogue de référence est amorcé par le serveur (rejouable : chaque
// entrée est mise à jour, jamais dupliquée). Une station ne modifie jamais ce
// référentiel : elle l'active, le renomme localement, le réordonne. Désactiver
// ne supprime rien.
// ---------------------------------------------------------------------------

// modeleCategorie : une entrée du référentiel des catégories de véhicules.
type modeleCategorie struct {
	Code             string
	Nom              string
	Famille          string
	Description      string
	GabaritTarifaire string
	DureeMinutes     int
	Alias            []string
}

// modeleUsage : une entrée du référentiel des usages commerciaux.
type modeleUsage struct {
	Code        string
	Nom         string
	Famille     string
	Description string
	Clientele   string
	Alias       []string
}

// Catalogue des catégories : cinq familles physiques, du plus courant au plus
// lourd. Les alias servent la recherche de l'écran (« 4x4 », « jeep », « van »).
var catalogueCategories = []modeleCategorie{
	{"citadine", "Citadine", "Véhicules légers", "Petite voiture de ville.", "legere", 25, []string{"petite voiture", "mini"}},
	{"berline", "Berline", "Véhicules légers", "Voiture à coffre, quatre à cinq places.", "legere", 35, []string{"sedan", "voiture"}},
	{"familiale", "Familiale / break", "Véhicules légers", "Break ou monospace familial.", "legere", 40, []string{"break", "monospace", "van"}},
	{"suv", "SUV / 4x4", "SUV, 4x4 et utilitaires légers", "Véhicule haut, souvent plus haut qu'une berline.", "suv", 45, []string{"4x4", "jeep", "pick-up leger"}},
	{"pickup", "Pick-up", "SUV, 4x4 et utilitaires légers", "Pick-up à benne, simple ou double cabine.", "suv", 50, []string{"pick up", "benne"}},
	{"utilitaire_leger", "Utilitaire léger", "SUV, 4x4 et utilitaires légers", "Fourgonnette ou camionnette légère.", "utilitaire", 60, []string{"fourgon", "camionnette", "kangoo"}},
	{"moto", "Moto", "Deux-roues et assimilés", "Moto, quelle que soit la cylindrée.", "deux_roues", 15, []string{"motocyclette", "moto-taxi"}},
	{"scooter", "Scooter / tricycle", "Deux-roues et assimilés", "Scooter ou tricycle motorisé.", "deux_roues", 15, []string{"tricycle", "tuk tuk"}},
	{"velo", "Vélo", "Deux-roues et assimilés", "Bicyclette, éventuellement à assistance électrique.", "deux_roues", 10, []string{"bicyclette", "vae"}},
	{"minibus", "Minibus", "Transport collectif", "Minibus de transport en commun (gbaka, massa).", "transport", 75, []string{"gbaka", "massa", "tata"}},
	{"bus", "Bus / car", "Transport collectif", "Autocar ou bus urbain.", "transport", 90, []string{"car", "autocar", "bus urbain"}},
	{"camion", "Camion", "Poids lourds", "Camion de marchandises.", "poids_lourd", 120, []string{"poids lourd", "benne"}},
	{"camion_remorque", "Camion avec remorque", "Poids lourds", "Ensemble attelé : remorque comprise.", "poids_lourd", 150, []string{"semi", "remorque"}},
}

// Catalogue des usages : ce que le client fait de son véhicule. C'est l'usage
// qui distingue un taximètre d'une voiture personnelle — et c'est souvent lui
// qui justifie un tarif différent.
var catalogueUsages = []modeleUsage{
	{"personnel", "Voiture personnelle", "Particuliers", "Usage privé du propriétaire.", "particulier", []string{"prive", "privee", "perso"}},
	{"taxi_compteur", "Taxi compteur", "Transport public", "Taxi équipé d'un compteur, en ville.", "professionnel", []string{"compteur", "taxi ville"}},
	{"taxi_communal", "Taxi communal (wôrô-wôrô)", "Transport public", "Taxi de quartier, trajet court à tarif partagé.", "professionnel", []string{"woro woro", "woro", "taxi communal"}},
	{"taxi_intercommunal", "Taxi intercommunal", "Transport public", "Trajet entre communes d'une même ville.", "professionnel", []string{"intercommunal", "ligne"}},
	{"interurbain", "Interurbain", "Transport public", "Liaison régulière entre deux villes.", "professionnel", []string{"inter-urbain", "ligne ville"}},
	{"taxi_brousse", "Taxi-brousse", "Transport public", "Transport collectif de longue distance.", "professionnel", []string{"brousse", "longue distance"}},
	{"gbaka", "Gbaka", "Transport public", "Minibus de transport urbain, souvent surchargé.", "professionnel", []string{"minibus public", "massa"}},
	{"vtc", "VTC", "Transport public", "Voiture de transport avec chauffeur (application).", "professionnel", []string{"uber", "yango", "application"}},
	{"moto_taxi", "Moto-taxi", "Transport public", "Deux-roues de transport payant.", "professionnel", []string{"zemidjan", "taga", "taxi moto"}},
	{"livraison", "Livraison", "Logistique", "Tournées de livraison, colis ou repas.", "professionnel", []string{"coursier", "colis", "glovo"}},
	{"marchandises", "Transport de marchandises", "Logistique", "Acheminement de marchandises.", "professionnel", []string{"fret", "cargaison"}},
	{"entreprise", "Entreprise", "Professionnels", "Véhicule de service immatriculé au nom d'une société.", "professionnel", []string{"societe", "de service"}},
	{"flotte", "Flotte", "Professionnels", "Véhicules gérés en parc, facturés au compte.", "professionnel", []string{"parc", "abonnement"}},
	{"scolaire", "Scolaire", "Institutions", "Transport d'élèves.", "professionnel", []string{"ecole", "eleves"}},
	{"administratif", "Administratif", "Institutions", "Véhicule d'administration, police, mairie.", "professionnel", []string{"administration", "mairie", "police"}},
	{"location", "Location", "Tourisme et location", "Véhicule de loueur, souvent rendu sale.", "professionnel", []string{"loueur", "rent"}},
	{"touristique", "Touristique", "Tourisme et location", "Excursions et transport de touristes.", "professionnel", []string{"excursion", "touriste"}},
	{"personnalise", "Personnalisé", "Autre", "Usage qui ne rentre dans aucune case — à préciser par l'exploitant.", "particulier", []string{"autre", "divers"}},
}

// ---------------------------------------------------------------------------
// Amorçage du référentiel
// ---------------------------------------------------------------------------

// preparerReferentiels regroupe ce que le serveur met en place à chaque
// démarrage, avant de servir. Un seul point d'entrée, pour que les tests de
// bout en bout démarrent exactement comme la production — une base de test sans
// référentiel ne prouverait rien de ce que verra l'exploitant.
func (a *App) preparerReferentiels(ctx context.Context) error {
	if err := a.assurerCatalogueVehicules(ctx); err != nil {
		return err
	}
	if err := a.preparerCataloguePrestations(ctx); err != nil {
		return err
	}
	// Moyens de paiement : le référentiel est posé pour chaque station, sans
	// écraser un renommage ni réactiver un moyen volontairement désactivé.
	return a.assurerMoyensPaiement(ctx)
}

// assurerCatalogueVehicules met le référentiel à jour, à chaque démarrage.
//
// Les entrées sont **mises à jour**, jamais dupliquées : un libellé corrigé ou
// une catégorie ajoutée par une nouvelle version du produit arrivent donc dans
// les bases déjà installées. Ce qui appartient à l'exploitant (activation, nom
// local, ordre) vit dans les tables de station et n'est jamais touché ici.
func (a *App) assurerCatalogueVehicules(ctx context.Context) error {
	for i, c := range catalogueCategories {
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO vehicle_category_templates
			  (code, name, family, description, pricing_class, duration_minutes, aliases, sort_order, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
			ON CONFLICT (code) DO UPDATE SET
			  name = EXCLUDED.name, family = EXCLUDED.family, description = EXCLUDED.description,
			  pricing_class = EXCLUDED.pricing_class, duration_minutes = EXCLUDED.duration_minutes,
			  aliases = EXCLUDED.aliases, sort_order = EXCLUDED.sort_order, updated_at = now()`,
			c.Code, c.Nom, c.Famille, c.Description, c.GabaritTarifaire, c.DureeMinutes,
			c.Alias, (i+1)*10); err != nil {
			return err
		}
	}
	for i, u := range catalogueUsages {
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO vehicle_usage_templates
			  (code, name, family, description, clientele, aliases, sort_order, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,now())
			ON CONFLICT (code) DO UPDATE SET
			  name = EXCLUDED.name, family = EXCLUDED.family, description = EXCLUDED.description,
			  clientele = EXCLUDED.clientele, aliases = EXCLUDED.aliases,
			  sort_order = EXCLUDED.sort_order, updated_at = now()`,
			u.Code, u.Nom, u.Famille, u.Description, u.Clientele, u.Alias, (i+1)*10); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Lecture
// ---------------------------------------------------------------------------

// categorieStation : une catégorie telle que proposée par une station.
type categorieStation struct {
	Code        string
	Kind        string
	Nom         string
	Famille     string
	Description string
	Alias       []string
	Gabarit     string
	Ordre       int
	Actif       bool
	// Vrai quand le nom vient du référentiel et n'a pas été personnalisé : le
	// front peut alors proposer « rétablir le nom d'origine ».
	NomDuModele bool
}

type usageStation struct {
	Code        string
	Kind        string
	Nom         string
	Famille     string
	Description string
	Alias       []string
	Clientele   string
	Ordre       int
	Actif       bool
}

// lireCategoriesStation renvoie les catégories de la station, actives d'abord.
func (a *App) lireCategoriesStation(ctx context.Context, stationID string, activesSeulement bool) ([]categorieStation, error) {
	filtre := ""
	if activesSeulement {
		filtre = " AND c.is_active"
	}
	lignes, err := a.Pool.Query(ctx, `
		SELECT c.code, c.kind, c.name, c.family, c.description, c.aliases, c.pricing_class,
		       c.sort_order, c.is_active,
		       COALESCE(t.name, ''), COALESCE(t.family, ''), COALESCE(t.description, ''),
		       COALESCE(t.pricing_class, ''),
		       CASE WHEN c.kind = 'modele' THEN 1 ELSE 0 END
		FROM station_vehicle_categories c
		LEFT JOIN vehicle_category_templates t ON t.code = c.code AND c.kind = 'modele'
		WHERE c.station_id = $1`+filtre+`
		ORDER BY c.is_active DESC, c.sort_order, COALESCE(NULLIF(c.name,''), t.name, c.code)`, stationID)
	if err != nil {
		return nil, err
	}
	defer lignes.Close()

	items := []categorieStation{}
	for lignes.Next() {
		var (
			c                                categorieStation
			nomLocal, nomModele              string
			famille, description             string
			familleModele, descriptionModele string
			gabaritModele                    string
			kindModele                       int
		)
		if err := lignes.Scan(&c.Code, &c.Kind, &nomLocal, &famille, &description, &c.Alias,
			&c.Gabarit, &c.Ordre, &c.Actif,
			&nomModele, &familleModele, &descriptionModele, &gabaritModele, &kindModele); err != nil {
			return nil, err
		}
		// Le nom effectif : le nom local s'il a été personnalisé, sinon celui du
		// référentiel. Une catégorie personnalisée n'a pas de modèle : son nom
		// local est alors toujours renseigné (exigé à la création).
		c.Nom = strings.TrimSpace(nomLocal)
		// Le libellé est « celui du référentiel » soit parce qu'aucun renommage
		// n'a été fait, soit parce qu'il a été rétabli à l'identique.
		c.NomDuModele = c.Nom == "" || c.Nom == strings.TrimSpace(nomModele)
		if c.Nom == "" {
			c.Nom = nomModele
		}
		c.Famille = famille
		if c.Famille == "" {
			c.Famille = familleModele
		}
		c.Description = description
		if c.Description == "" {
			c.Description = descriptionModele
		}
		if c.Gabarit == "" {
			c.Gabarit = gabaritModele
		}
		items = append(items, c)
	}
	return items, lignes.Err()
}

// lireUsagesStation renvoie les usages activés par la station.
func (a *App) lireUsagesStation(ctx context.Context, stationID string, activesSeulement bool) ([]usageStation, error) {
	filtre := ""
	if activesSeulement {
		filtre = " AND u.is_active"
	}
	lignes, err := a.Pool.Query(ctx, `
		SELECT u.code, u.kind, u.name, u.family, u.description, u.aliases, u.sort_order, u.is_active,
		       COALESCE(t.name, ''), COALESCE(t.family, ''), COALESCE(t.description, ''),
		       COALESCE(t.clientele, 'particulier')
		FROM station_vehicle_usages u
		LEFT JOIN vehicle_usage_templates t ON t.code = u.code AND u.kind = 'modele'
		WHERE u.station_id = $1`+filtre+`
		ORDER BY u.is_active DESC, u.sort_order, COALESCE(NULLIF(u.name,''), t.name, u.code)`, stationID)
	if err != nil {
		return nil, err
	}
	defer lignes.Close()

	items := []usageStation{}
	for lignes.Next() {
		var u usageStation
		var nomLocal, famille, description string
		var nomModele, familleModele, descriptionModele, clienteleModele string
		if err := lignes.Scan(&u.Code, &u.Kind, &nomLocal, &famille, &description, &u.Alias,
			&u.Ordre, &u.Actif,
			&nomModele, &familleModele, &descriptionModele, &clienteleModele); err != nil {
			return nil, err
		}
		u.Nom = strings.TrimSpace(nomLocal)
		if u.Nom == "" {
			u.Nom = nomModele
		}
		u.Famille = famille
		if u.Famille == "" {
			u.Famille = familleModele
		}
		u.Description = description
		if u.Description == "" {
			u.Description = descriptionModele
		}
		u.Clientele = clienteleModele
		items = append(items, u)
	}
	return items, lignes.Err()
}

// categorieJSON / usageJSON : la forme attendue par l'interface. Le champ `id`
// reste exposé (c'est ce que la caisse envoie) et vaut le code : côté station,
// un code désigne une entrée, activée ou non.
func categorieJSON(c categorieStation) map[string]any {
	return map[string]any{
		"id": c.Code, "code": c.Code, "kind": c.Kind, "name": c.Nom, "family": c.Famille,
		"description": c.Description, "aliases": c.Alias, "pricing_class": c.Gabarit,
		"sort_order": c.Ordre, "is_active": c.Actif, "name_from_template": c.NomDuModele,
	}
}

func usageJSON(u usageStation) map[string]any {
	return map[string]any{
		"id": u.Code, "code": u.Code, "kind": u.Kind, "name": u.Nom, "family": u.Famille,
		"description": u.Description, "aliases": u.Alias, "clientele": u.Clientele,
		"sort_order": u.Ordre, "is_active": u.Actif,
	}
}

// ---------------------------------------------------------------------------
// Handlers — lecture
// ---------------------------------------------------------------------------

// GET /api/v1/vehicle-categories?station_id=…&actives=1
func (a *App) handleVehicleCategories(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}, "total": 0})
		return
	}
	actives := r.URL.Query().Get("actives") == "1"
	items, err := a.lireCategoriesStation(r.Context(), stationID, actives)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, c := range items {
		payload = append(payload, categorieJSON(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": payload, "total": len(payload)})
}

// GET /api/v1/vehicle-usages?station_id=…&actives=1
func (a *App) handleVehicleUsages(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}, "total": 0})
		return
	}
	actives := r.URL.Query().Get("actives") == "1"
	items, err := a.lireUsagesStation(r.Context(), stationID, actives)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	for _, u := range items {
		payload = append(payload, usageJSON(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": payload, "total": len(payload)})
}

// GET /api/v1/vehicle-catalogue?station_id=…
//
// Le référentiel complet **et** ce que la station en a fait : une seule requête
// suffit à l'écran d'activation, qui affiche chaque modèle avec son état.
func (a *App) handleVehicleCatalogue(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}

	lignes, err := a.Pool.Query(r.Context(), `
		SELECT t.code, t.name, t.family, t.description, t.pricing_class, t.duration_minutes, t.aliases,
		       COALESCE(s.name, ''), COALESCE(s.sort_order, 0), COALESCE(s.is_active, false),
		       (s.station_id IS NOT NULL)
		FROM vehicle_category_templates t
		LEFT JOIN station_vehicle_categories s
		       ON s.code = t.code AND s.station_id = $1 AND s.kind = 'modele'
		ORDER BY t.sort_order, t.name`, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer lignes.Close()

	categories := []map[string]any{}
	for lignes.Next() {
		var (
			code, nom, famille, description, gabarit string
			duree                                    int
			alias                                    []string
			nomLocal                                 string
			ordre                                    int
			actif, configuree                        bool
		)
		if err := lignes.Scan(&code, &nom, &famille, &description, &gabarit, &duree, &alias,
			&nomLocal, &ordre, &actif, &configuree); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		categories = append(categories, map[string]any{
			"code": code, "name": nom, "family": famille, "description": description,
			"pricing_class": gabarit, "duration_minutes": duree, "aliases": alias,
			"local_name": nomLocal, "sort_order": ordre, "is_active": actif,
			"configured": configuree,
		})
	}

	usages := []map[string]any{}
	lignesUsages, err := a.Pool.Query(r.Context(), `
		SELECT t.code, t.name, t.family, t.description, t.clientele, t.aliases,
		       COALESCE(s.name, ''), COALESCE(s.sort_order, 0), COALESCE(s.is_active, false),
		       (s.station_id IS NOT NULL)
		FROM vehicle_usage_templates t
		LEFT JOIN station_vehicle_usages s
		       ON s.code = t.code AND s.station_id = $1 AND s.kind = 'modele'
		ORDER BY t.sort_order, t.name`, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer lignesUsages.Close()
	for lignesUsages.Next() {
		var (
			code, nom, famille, description, clientele string
			alias                                      []string
			nomLocal                                   string
			ordre                                      int
			actif, configuree                          bool
		)
		if err := lignesUsages.Scan(&code, &nom, &famille, &description, &clientele, &alias,
			&nomLocal, &ordre, &actif, &configuree); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		usages = append(usages, map[string]any{
			"code": code, "name": nom, "family": famille, "description": description,
			"clientele": clientele, "aliases": alias, "local_name": nomLocal,
			"sort_order": ordre, "is_active": actif, "configured": configuree,
		})
	}

	activesCat, _ := a.lireCategoriesStation(r.Context(), stationID, true)
	actifsUsa, _ := a.lireUsagesStation(r.Context(), stationID, true)
	writeJSON(w, http.StatusOK, map[string]any{
		"station_id":          stationID,
		"categories":          categories,
		"usages":              usages,
		"actives":             len(activesCat),
		"usages_actifs":       len(actifsUsa),
		"familles_categories": famillesDe(categories, "family"),
		"familles_usages":     famillesDe(usages, "family"),
		"message_referentiel": "Le référentiel est commun à toutes les stations ; ce que vous cochez ici n'appartient qu'à la vôtre.",
	})
}

// famillesDe : la liste des familles présentes, dans l'ordre d'apparition,
// pour que l'écran puisse regrouper sans la déduire lui-même.
func famillesDe(items []map[string]any, cle string) []string {
	vues := map[string]bool{}
	familles := []string{}
	for _, item := range items {
		famille, _ := item[cle].(string)
		if famille == "" || vues[famille] {
			continue
		}
		vues[famille] = true
		familles = append(familles, famille)
	}
	return familles
}

// ---------------------------------------------------------------------------
// Handlers — écriture
// ---------------------------------------------------------------------------

// PUT /api/v1/stations/{id}/vehicle-categories
//
// Corps : {"categories": [{"code": "berline", "is_active": true, "name": ""}]}
//
// Enregistrement en bloc : l'écran d'activation coche plusieurs catégories puis
// enregistre. Les catégories personnalisées déjà créées ne sont pas concernées
// (elles ne viennent pas du référentiel).
func (a *App) handleStationVehicleCategories(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID := r.PathValue("id")
	if err := a.stationAutorisee(r.Context(), deref(user.OrganizationID), stationID); err != nil {
		writeAPIError(w, err)
		return
	}
	var body struct {
		Categories []struct {
			Code      string `json:"code"`
			IsActive  *bool  `json:"is_active"`
			Name      string `json:"name"`
			SortOrder *int   `json:"sort_order"`
		} `json:"categories"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	erreurs := map[string]string{}
	for _, c := range body.Categories {
		if strings.TrimSpace(c.Code) == "" {
			erreurs["code"] = "Chaque catégorie doit porter un code."
			continue
		}
		var existe bool
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM vehicle_category_templates WHERE code = $1)`, c.Code).Scan(&existe); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if !existe {
			erreurs[c.Code] = "Catégorie inconnue du référentiel."
		}
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucune catégorie n'a été enregistrée : corrigez les codes signalés.", erreurs)
		return
	}

	actives := 0
	for _, c := range body.Categories {
		actif := true
		if c.IsActive != nil {
			actif = *c.IsActive
		}
		ordre := 0
		if c.SortOrder != nil {
			ordre = *c.SortOrder
		}
		if _, err := a.Pool.Exec(r.Context(), `
			INSERT INTO station_vehicle_categories (station_id, code, kind, name, sort_order, is_active, updated_at)
			VALUES ($1,$2,'modele',$3,$4,$5,now())
			ON CONFLICT (station_id, code) DO UPDATE SET
			  -- Un nom vide veut dire « ne change pas le nom » : l'enregistrement en
			  -- bloc coche des entrées, il ne réécrit pas les renommages locaux.
			  name = CASE WHEN EXCLUDED.name = '' THEN station_vehicle_categories.name ELSE EXCLUDED.name END,
			  sort_order = CASE WHEN EXCLUDED.sort_order = 0
			                    THEN station_vehicle_categories.sort_order ELSE EXCLUDED.sort_order END,
			  is_active = EXCLUDED.is_active,
			  updated_at = now()`,
			stationID, c.Code, strings.TrimSpace(c.Name), ordre, actif); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if actif {
			actives++
		}
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"vehicle_categories.updated", "station", stationID,
		map[string]any{"configurees": len(body.Categories), "actives": actives})
	a.repondreCategories(w, r, stationID)
}

// PUT /api/v1/stations/{id}/vehicle-usages — même logique, autre référentiel.
func (a *App) handleStationVehicleUsages(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID := r.PathValue("id")
	if err := a.stationAutorisee(r.Context(), deref(user.OrganizationID), stationID); err != nil {
		writeAPIError(w, err)
		return
	}
	var body struct {
		Usages []struct {
			Code      string `json:"code"`
			IsActive  *bool  `json:"is_active"`
			Name      string `json:"name"`
			SortOrder *int   `json:"sort_order"`
		} `json:"usages"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	erreurs := map[string]string{}
	for _, u := range body.Usages {
		if strings.TrimSpace(u.Code) == "" {
			erreurs["code"] = "Chaque usage doit porter un code."
			continue
		}
		var existe bool
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM vehicle_usage_templates WHERE code = $1)`, u.Code).Scan(&existe); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if !existe {
			erreurs[u.Code] = "Usage inconnu du référentiel."
		}
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucun usage n'a été enregistré : corrigez les codes signalés.", erreurs)
		return
	}

	actifs := 0
	for _, u := range body.Usages {
		actif := true
		if u.IsActive != nil {
			actif = *u.IsActive
		}
		ordre := 0
		if u.SortOrder != nil {
			ordre = *u.SortOrder
		}
		if _, err := a.Pool.Exec(r.Context(), `
			INSERT INTO station_vehicle_usages (station_id, code, kind, name, sort_order, is_active, updated_at)
			VALUES ($1,$2,'modele',$3,$4,$5,now())
			ON CONFLICT (station_id, code) DO UPDATE SET
			  -- Un nom vide veut dire « ne change pas le nom » : l'enregistrement en
			  -- bloc coche des entrées, il ne réécrit pas les renommages locaux.
			  name = CASE WHEN EXCLUDED.name = '' THEN station_vehicle_usages.name ELSE EXCLUDED.name END,
			  sort_order = CASE WHEN EXCLUDED.sort_order = 0
			                    THEN station_vehicle_usages.sort_order ELSE EXCLUDED.sort_order END,
			  is_active = EXCLUDED.is_active,
			  updated_at = now()`,
			stationID, u.Code, strings.TrimSpace(u.Name), ordre, actif); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if actif {
			actifs++
		}
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"vehicle_usages.updated", "station", stationID,
		map[string]any{"configures": len(body.Usages), "actifs": actifs})
	a.repondreUsages(w, r, stationID)
}

// POST /api/v1/vehicle-categories — créer une catégorie propre à la station.
func (a *App) handleVehicleCategoryCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		StationID   string   `json:"station_id"`
		Name        string   `json:"name"`
		Family      string   `json:"family"`
		Description string   `json:"description"`
		Aliases     []string `json:"aliases"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le nom de la catégorie est obligatoire.",
			map[string]string{"name": "Nom obligatoire."})
		return
	}
	stationID, err := a.resolveStation(r.Context(), user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}

	// Code local : préfixé, pour ne jamais entrer en collision avec le
	// référentiel — un ajout de modèle par une version future ne doit pas
	// écraser une catégorie créée par l'exploitant.
	code := "perso_" + newLocalCode()
	// Un tableau vide, jamais nil : la colonne `aliases` est NOT NULL (un alias
	// absent s'écrit `{}`, il ne vaut pas NULL).
	if body.Aliases == nil {
		body.Aliases = []string{}
	}
	nom := strings.TrimSpace(body.Name)
	famille := strings.TrimSpace(body.Family)
	if famille == "" {
		famille = "Catégories personnalisées"
	}
	if _, err := a.Pool.Exec(r.Context(), `
		INSERT INTO station_vehicle_categories
		  (station_id, code, kind, name, family, description, aliases, sort_order, is_active, updated_at)
		VALUES ($1,$2,'personnalisee',$3,$4,$5,$6,999,$7,now())`,
		stationID, code, nom, famille, strings.TrimSpace(body.Description), body.Aliases, true); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"vehicle_category.created", "vehicle_category", code, map[string]any{"name": nom})
	a.repondreCategories(w, r, stationID)
}

// PATCH /api/v1/vehicle-categories/{code} — renommer, réordonner, activer.
func (a *App) handleVehicleCategoryUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	code := r.PathValue("code")
	var body struct {
		StationID   string    `json:"station_id"`
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Family      *string   `json:"family"`
		Aliases     *[]string `json:"aliases"`
		SortOrder   *int      `json:"sort_order"`
		IsActive    *bool     `json:"is_active"`
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

	var existe bool
	if err := a.Pool.QueryRow(r.Context(), `
		SELECT EXISTS (SELECT 1 FROM station_vehicle_categories WHERE station_id = $1 AND code = $2)`,
		stationID, code).Scan(&existe); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	if !existe {
		writeError(w, http.StatusNotFound, "not_found", "Catégorie introuvable pour cette station.", nil)
		return
	}

	// Numérotation explicite : $1 est la station, $2 le code, les champs
	// modifiés suivent. (Une numérotation qui repartait de 1 faisait
	// silencieusement échouer la mise à jour : `code = $2` recevait le nouveau
	// nom, aucune ligne ne correspondait, et la réponse annonçait un succès.)
	mises := []string{}
	args := []any{}
	n := 2
	ajouter := func(fragment string, valeur any) {
		n++
		mises = append(mises, fragment+" = $"+itoa(n))
		args = append(args, valeur)
	}
	if body.Name != nil {
		// Nom vide = « revenez au libellé du référentiel » : c'est le geste que
		// propose l'écran, et il doit être enregistrable.
		ajouter("name", strings.TrimSpace(*body.Name))
	}
	if body.Description != nil {
		ajouter("description", strings.TrimSpace(*body.Description))
	}
	if body.Family != nil {
		ajouter("family", strings.TrimSpace(*body.Family))
	}
	if body.Aliases != nil {
		ajouter("aliases", *body.Aliases)
	}
	if body.SortOrder != nil {
		ajouter("sort_order", *body.SortOrder)
	}
	if body.IsActive != nil {
		ajouter("is_active", *body.IsActive)
	}
	if len(mises) == 0 {
		a.repondreCategories(w, r, stationID)
		return
	}
	mises = append(mises, "updated_at = now()")

	etiquette, err := a.Pool.Exec(r.Context(), `
		UPDATE station_vehicle_categories SET `+strings.Join(mises, ", ")+`
		WHERE station_id = $1 AND code = $2`, append([]any{stationID, code}, args...)...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	// Aucune ligne touchée = la ressource n'existe pas : le dire, plutôt que de
	// renvoyer un « enregistré » mensonger.
	if etiquette.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Catégorie introuvable pour cette station.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"vehicle_category.updated", "vehicle_category", code, nil)
	a.repondreCategories(w, r, stationID)
}

// GET /api/v1/vehicle-category-options — ce que la caisse propose.
//
// Une seule requête pour les deux listes : la fiche de lavage a besoin des
// catégories **et** des usages, et deux appels séparés laisseraient un état
// intermédiaire (catégorie choisie, liste d'usages pas encore arrivée).
func (a *App) handleVehicleOptions(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	if stationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"categories": []map[string]any{}, "usages": []map[string]any{}})
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
	cats := make([]map[string]any, 0, len(categories))
	for _, c := range categories {
		cats = append(cats, categorieJSON(c))
	}
	usas := make([]map[string]any, 0, len(usages))
	for _, u := range usages {
		usas = append(usas, usageJSON(u))
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats, "usages": usas})
}

// repondreCategories / repondreUsages : renvoyer l'état enregistré, comme le
// font les autres écritures de l'application (l'écran n'a pas à deviner).
func (a *App) repondreCategories(w http.ResponseWriter, r *http.Request, stationID string) {
	items, err := a.lireCategoriesStation(r.Context(), stationID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	actives := 0
	for _, c := range items {
		payload = append(payload, categorieJSON(c))
		if c.Actif {
			actives++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": payload, "total": len(payload), "actives": actives,
		"message": "Catégories enregistrées.",
	})
}

func (a *App) repondreUsages(w http.ResponseWriter, r *http.Request, stationID string) {
	items, err := a.lireUsagesStation(r.Context(), stationID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	payload := make([]map[string]any, 0, len(items))
	actifs := 0
	for _, u := range items {
		payload = append(payload, usageJSON(u))
		if u.Actif {
			actifs++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": payload, "total": len(payload), "actifs": actifs,
		"message": "Usages enregistrés.",
	})
}

// compterCategoriesActives / compterUsagesActifs : contrôles d'onboarding.
func (a *App) compterCategoriesActives(ctx context.Context, stationID string) (int, error) {
	if stationID == "" {
		return 0, nil
	}
	var n int
	err := a.Pool.QueryRow(ctx,
		`SELECT count(*) FROM station_vehicle_categories WHERE station_id = $1 AND is_active`,
		stationID).Scan(&n)
	return n, err
}

func (a *App) compterUsagesActifs(ctx context.Context, stationID string) (int, error) {
	if stationID == "" {
		return 0, nil
	}
	var n int
	err := a.Pool.QueryRow(ctx,
		`SELECT count(*) FROM station_vehicle_usages WHERE station_id = $1 AND is_active`,
		stationID).Scan(&n)
	return n, err
}

// nomCategorieStation : nom effectif d'une catégorie **activée** par la
// station (nom local s'il a été personnalisé, sinon celui du référentiel).
// Renvoie une erreur si la catégorie n'est pas proposée par cette station :
// la caisse ne peut pas vendre une catégorie que la station a désactivée.
func (a *App) nomCategorieStation(ctx context.Context, stationID, code string) (string, error) {
	var nom string
	err := a.Pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(btrim(c.name), ''), t.name, c.name, '')
		FROM station_vehicle_categories c
		LEFT JOIN vehicle_category_templates t ON t.code = c.code AND c.kind = 'modele'
		WHERE c.station_id = $1 AND c.code = $2 AND c.is_active`, stationID, code).Scan(&nom)
	return nom, err
}

// nomUsageStation : même logique pour les usages commerciaux.
func (a *App) nomUsageStation(ctx context.Context, stationID, code string) (string, error) {
	var nom string
	err := a.Pool.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(btrim(u.name), ''), t.name, u.name, '')
		FROM station_vehicle_usages u
		LEFT JOIN vehicle_usage_templates t ON t.code = u.code AND u.kind = 'modele'
		WHERE u.station_id = $1 AND u.code = $2 AND u.is_active`, stationID, code).Scan(&nom)
	return nom, err
}

// newLocalCode : identifiant d'une catégorie propre à une station. Le préfixe
// `perso_` garantit qu'un modèle ajouté plus tard par le produit ne peut pas
// entrer en collision avec une création d'exploitant.
func newLocalCode() string {
	return strings.ToLower(newID("")[len(newID(""))-8:])
}

// ---------------------------------------------------------------------------
// Couples (catégorie, usage)
//
// Ce sont les combinaisons que la station accepte. Une station qui lave des
// motos ne lave pas forcément des « motos de livraison » : le couple dit ce qui
// est vendable au comptoir. **Aucun prix ici** — la grille par couple arrive au
// lot 5 ; inventer un tarif à ce stade serait contraire à la règle « aucun prix
// inventé ».
// ---------------------------------------------------------------------------

// GET /api/v1/vehicle-usage-categories?station_id=…
func (a *App) handleUsageCategories(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	items := []map[string]any{}
	if stationID != "" {
		lignes, err := a.Pool.Query(r.Context(), `
			SELECT usage_code, category_code, is_active, sort_order
			FROM station_vehicle_usage_categories
			WHERE station_id = $1
			ORDER BY usage_code, sort_order, category_code`, stationID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		defer lignes.Close()
		for lignes.Next() {
			var usageCode, categorieCode string
			var actif bool
			var ordre int
			if err := lignes.Scan(&usageCode, &categorieCode, &actif, &ordre); err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
				return
			}
			items = append(items, map[string]any{
				"usage_code": usageCode, "category_code": categorieCode,
				"is_active": actif, "sort_order": ordre,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(items), "station_id": stationID,
		"message": "Aucun prix n'est défini à ce niveau : les tarifs par couple arrivent avec la grille tarifaire.",
	})
}

// PUT /api/v1/stations/{id}/vehicle-usage-categories
//
// Corps : {"couples": [{"usage_code": "taxi_compteur", "category_code": "berline"}]}
//
// Remplacement complet : l'écran envoie l'ensemble des couples cochés. Les
// couples retirés sont **désactivés**, pas supprimés — une fiche passée doit
// rester explicable.
func (a *App) handleStationUsageCategories(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID := r.PathValue("id")
	if err := a.stationAutorisee(r.Context(), deref(user.OrganizationID), stationID); err != nil {
		writeAPIError(w, err)
		return
	}
	var body struct {
		Couples []struct {
			UsageCode    string `json:"usage_code"`
			CategoryCode string `json:"category_code"`
			IsActive     *bool  `json:"is_active"`
		} `json:"couples"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	// Validation en bloc : un couple qui cite une catégorie ou un usage non
	// activé par la station est refusé, avec la raison — sinon l'écran
	// enregistrerait une combinaison que la caisse ne pourrait pas proposer.
	erreurs := map[string]string{}
	for _, c := range body.Couples {
		if c.IsActive != nil && !*c.IsActive {
			continue
		}
		cle := c.UsageCode + " × " + c.CategoryCode
		var usageActif, categorieActive bool
		if err := a.Pool.QueryRow(r.Context(), `
			SELECT
			  EXISTS (SELECT 1 FROM station_vehicle_usages
			           WHERE station_id = $1 AND code = $2 AND is_active),
			  EXISTS (SELECT 1 FROM station_vehicle_categories
			           WHERE station_id = $1 AND code = $3 AND is_active)`,
			stationID, c.UsageCode, c.CategoryCode).Scan(&usageActif, &categorieActive); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if !usageActif {
			erreurs[cle] = "Cet usage n'est pas activé pour la station."
		} else if !categorieActive {
			erreurs[cle] = "Cette catégorie n'est pas activée pour la station."
		}
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucun couple n'a été enregistré : corrigez les combinaisons signalées.", erreurs)
		return
	}

	// Désactivation de ce qui n'est plus coché, puis enregistrement des cochés.
	if _, err := a.Pool.Exec(r.Context(), `
		UPDATE station_vehicle_usage_categories SET is_active = false, updated_at = now()
		WHERE station_id = $1 AND is_active`, stationID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	actifs := 0
	for i, c := range body.Couples {
		actif := true
		if c.IsActive != nil {
			actif = *c.IsActive
		}
		if _, err := a.Pool.Exec(r.Context(), `
			INSERT INTO station_vehicle_usage_categories
			  (station_id, usage_code, category_code, is_active, sort_order, updated_at)
			VALUES ($1,$2,$3,$4,$5,now())
			ON CONFLICT (station_id, usage_code, category_code) DO UPDATE SET
			  is_active = EXCLUDED.is_active, sort_order = EXCLUDED.sort_order, updated_at = now()`,
			stationID, c.UsageCode, c.CategoryCode, actif, i); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if actif {
			actifs++
		}
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"vehicle_usage_categories.updated", "station", stationID,
		map[string]any{"couples": len(body.Couples), "actifs": actifs})
	a.handleUsageCategories(w, r)
}

// combinaisonsActives : chaîne « usage_code:category_code » des couples
// actifs, pour la résolution côté caisse (pas de requête par ligne).
func (a *App) combinaisonsActives(ctx context.Context, stationID string) (map[string]bool, error) {
	actifs := map[string]bool{}
	if stationID == "" {
		return actifs, nil
	}
	lignes, err := a.Pool.Query(ctx, `
		SELECT usage_code, category_code FROM station_vehicle_usage_categories
		WHERE station_id = $1 AND is_active`, stationID)
	if err != nil {
		return actifs, err
	}
	defer lignes.Close()
	for lignes.Next() {
		var usageCode, categorieCode string
		if err := lignes.Scan(&usageCode, &categorieCode); err != nil {
			return actifs, err
		}
		actifs[usageCode+":"+categorieCode] = true
	}
	return actifs, lignes.Err()
}

// compterCouplesActifs : contrôle d'onboarding (étape 5).
func (a *App) compterCouplesActifs(ctx context.Context, stationID string) (int, error) {
	if stationID == "" {
		return 0, nil
	}
	var n int
	err := a.Pool.QueryRow(ctx,
		`SELECT count(*) FROM station_vehicle_usage_categories WHERE station_id = $1 AND is_active`,
		stationID).Scan(&n)
	return n, err
}
