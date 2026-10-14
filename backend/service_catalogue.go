package main

import (
	"context"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Catalogue de prestations : familles, modèles, compatibilités, options.
//
// Deux niveaux, comme pour les véhicules :
//
//   - **le référentiel du produit** (`service_family_templates`,
//     `service_templates`) : des familles et des prestations **sans aucun
//     prix** — ce sont des modèles à tarifer, jamais des tarifs ;
//   - **le catalogue de la station** (`services`, `service_families`,
//     `service_compatibilities`, `service_options`), que l'exploitant complète,
//     renomme, tarife.
//
// Un compte neuf part d'un catalogue **non tarifé** (les modèles sont copiés
// dans `services` avec `price_xof = 0`) : l'exploitant saisit ses vrais prix,
// et le serveur refuse de vendre ce qui n'est pas tarifé (`409
// price_required`). Aucun montant n'est inventé ici.
// ---------------------------------------------------------------------------

// modeleFamille : une famille du référentiel.
type modeleFamille struct {
	Code        string
	Nom         string
	Description string
}

// modelePrestation : une prestation du référentiel, **sans prix**.
type modelePrestation struct {
	Code         string
	Nom          string
	FamilleCode  string
	Description  string
	Includes     []string
	DureeMinutes int
	Equipement   string
	Securite     string
	Prealables   string
	SurDevis     bool
	Categories   []string
}

var catalogueFamilles = []modeleFamille{
	{"exterieur", "Extérieur", "Ce qui se voit de la rue : carrosserie, jantes, vitres."},
	{"interieur", "Intérieur", "Habitacle : aspiration, plastiques, sièges, vitres intérieures."},
	{"formules", "Formules combinées", "Plusieurs opérations vendues ensemble, à un prix unique."},
	{"specialisees", "Prestations spécialisées", "Polissage, cire, moteur, céramique…"},
	{"vehicules", "Prestations par type de véhicule", "Ce qui ne s'applique qu'à certains véhicules : poids lourds, deux-roues, bus."},
}

var cataloguePrestations = []modelePrestation{
	// Extérieur
	{"lavage_exterieur", "Lavage extérieur", "exterieur",
		"Carrosserie, jantes et vitres extérieures.",
		[]string{"Prélavage haute pression", "Shampoing carrosserie", "Rinçage", "Jantes"}, 30, "", "", "", false, nil},
	{"lavage_exterieur_sechage", "Lavage extérieur + séchage", "exterieur",
		"Lavage extérieur suivi d'un séchage à la microfibre.",
		[]string{"Prélavage", "Shampoing carrosserie", "Rinçage", "Jantes", "Séchage microfibre"}, 40, "", "", "", false, nil},
	{"jantes_roues", "Jantes et passages de roues", "exterieur",
		"Décontamination des jantes et des passages de roues.",
		[]string{"Dégraissant jantes", "Brossage", "Rinçage"}, 30,
		"Brosse à jantes dédiée", "Ne pas projeter sur les freins chauds.", "", false, nil},
	// Intérieur
	{"aspiration", "Aspiration intérieure", "interieur",
		"Sol, sièges et coffre aspirés.",
		[]string{"Aspiration sols", "Aspiration sièges", "Aspiration coffre"}, 20, "", "", "", false, nil},
	{"interieur_complet", "Nettoyage intérieur complet", "interieur",
		"Habitacle nettoyé : aspiration, plastiques, vitres intérieures.",
		[]string{"Aspiration", "Plastiques et tableau de bord", "Vitres intérieures"}, 45, "", "", "", false, nil},
	{"shampouinage_sieges", "Shampouinage des sièges", "interieur",
		"Sièges et tapis nettoyés en profondeur, puis séchés.",
		[]string{"Shampouineuse", "Détachage", "Séchage"}, 90,
		"Shampouineuse à injection-extraction", "", "Prévoir une journée de séchage : véhicule immobilisé.", false, nil},
	{"desinfection", "Désinfection de l'habitacle", "interieur",
		"Traitement antibactérien de l'habitacle et du circuit de climatisation.",
		[]string{"Nettoyage vapeur", "Traitement antibactérien", "Assainissement clim"}, 30,
		"Générateur de vapeur ou brumisateur", "Produit à manipuler avec gants.", "", false, nil},
	// Formules combinées
	{"formule_express", "Formule express", "formules",
		"Extérieur et aspiration, pour un véhicule peu sale.",
		[]string{"Lavage extérieur", "Aspiration intérieure"}, 40, "", "", "", false, nil},
	{"formule_complete", "Formule complète", "formules",
		"Extérieur et intérieur complets, la formule la plus demandée.",
		[]string{"Lavage extérieur", "Nettoyage intérieur complet", "Vitres"}, 75, "", "", "", false, nil},
	{"formule_premium", "Formule premium", "formules",
		"Formule complète, cire de protection et jantes approfondies.",
		[]string{"Formule complète", "Cire de protection", "Jantes et passages de roues"}, 120, "", "", "", false, nil},
	// Prestations spécialisées
	{"polissage", "Polissage / lustrage", "specialisees",
		"Correction des rayures fines et ravivage de la peinture.",
		[]string{"Décontamination", "Polissage machine", "Finition"}, 90,
		"Polisseuse orbitale, pads et pâtes", "Port de lunettes et de gants.", "Peinture en bon état (pas de vernis mort).", false, nil},
	{"cire", "Cire de protection", "specialisees",
		"Protection de la peinture par une cire appliquée à la main.",
		[]string{"Application cire", "Pose puis essuyage"}, 60, "", "", "", false, nil},
	{"moteur", "Nettoyage moteur", "specialisees",
		"Dégraissage et rinçage du compartiment moteur.",
		[]string{"Dégraissage", "Rinçage contrôlé", "Séchage"}, 45,
		"Protection des parties électriques", "Moteur froid obligatoire.",
		"Véhicule amené moteur froid.", false, nil},
	{"ceramique", "Traitement céramique", "specialisees",
		"Protection longue durée de la carrosserie — chiffrage après examen.",
		[]string{"Préparation complète", "Application céramique", "Cuisson"}, 240,
		"Cabine ou local abrité", "Produit à appliquer avec masque et gants.",
		"Véhicule immobilisé une journée.", true, nil},
	{"desodorisation", "Désodorisation", "specialisees",
		"Traitement des odeurs persistantes (tabac, animaux, humidité).",
		[]string{"Traitement de l'habitacle", "Assainissement du circuit d'air"}, 20, "", "", "", false, nil},
	// Par type de véhicule : le modèle ne s'applique qu'à ces catégories.
	{"lavage_poids_lourd", "Lavage poids lourd", "vehicules",
		"Lavage d'un camion ou d'un ensemble attelé, à la lance et à la brosse.",
		[]string{"Prélavage", "Lavage carrosserie", "Rinçage", "Châssis"}, 120,
		"Nacelle ou escabeau, lance longue portée", "Circulation : baliser la zone.",
		"Accès poids lourd dégagé.", false, []string{"camion", "camion_remorque"}},
	{"lavage_bus", "Lavage bus / minibus", "vehicules",
		"Lavage intérieur et extérieur d'un bus ou d'un minibus de transport.",
		[]string{"Lavage extérieur", "Aspiration des rangées", "Sol", "Vitres"}, 75,
		"Accès en hauteur", "", "Véhicule vide de voyageurs.", false, []string{"minibus", "bus"}},
	{"lavage_deux_roues", "Lavage deux-roues", "vehicules",
		"Lavage adapté aux motos, scooters et tricycles : pas de haute pression sur les roulements.",
		[]string{"Lavage à basse pression", "Rinçage", "Séchage"}, 15,
		"Lance basse pression", "Ne pas diriger le jet vers la chaîne et les roulements.", "", false,
		[]string{"moto", "scooter", "velo"}},
}

// preparerCataloguePrestations met le référentiel à jour (rejouable : aucune
// duplication, les libellés corrigés arrivent dans les bases installées).
func (a *App) preparerCataloguePrestations(ctx context.Context) error {
	for i, f := range catalogueFamilles {
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO service_family_templates (code, name, description, sort_order, updated_at)
			VALUES ($1,$2,$3,$4,now())
			ON CONFLICT (code) DO UPDATE SET
			  name = EXCLUDED.name, description = EXCLUDED.description,
			  sort_order = EXCLUDED.sort_order, updated_at = now()`,
			f.Code, f.Nom, f.Description, (i+1)*10); err != nil {
			return err
		}
	}
	for i, p := range cataloguePrestations {
		includes := p.Includes
		if includes == nil {
			includes = []string{}
		}
		categories := p.Categories
		if categories == nil {
			categories = []string{}
		}
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO service_templates (code, name, family_code, description, includes,
			  duration_minutes, equipment_notes, safety_notes, preconditions, is_quote_only,
			  category_codes, sort_order, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
			ON CONFLICT (code) DO UPDATE SET
			  name = EXCLUDED.name, family_code = EXCLUDED.family_code,
			  description = EXCLUDED.description, includes = EXCLUDED.includes,
			  duration_minutes = EXCLUDED.duration_minutes,
			  equipment_notes = EXCLUDED.equipment_notes, safety_notes = EXCLUDED.safety_notes,
			  preconditions = EXCLUDED.preconditions, is_quote_only = EXCLUDED.is_quote_only,
			  category_codes = EXCLUDED.category_codes, sort_order = EXCLUDED.sort_order,
			  updated_at = now()`,
			p.Code, p.Nom, p.FamilleCode, p.Description, includes, p.DureeMinutes,
			p.Equipement, p.Securite, p.Prealables, p.SurDevis, categories, (i+1)*10); err != nil {
			return err
		}
	}
	return nil
}

// modelePrestation retrouve un modèle du référentiel.
func (a *App) modelePrestation(ctx context.Context, code string) (modelePrestation, error) {
	var p modelePrestation
	err := a.Pool.QueryRow(ctx, `
		SELECT code, name, family_code, description, includes, duration_minutes,
		       equipment_notes, safety_notes, preconditions, is_quote_only, category_codes
		FROM service_templates WHERE code = $1`, code).
		Scan(&p.Code, &p.Nom, &p.FamilleCode, &p.Description, &p.Includes, &p.DureeMinutes,
			&p.Equipement, &p.Securite, &p.Prealables, &p.SurDevis, &p.Categories)
	return p, err
}

// enregistrerCompatibilites remplace les compatibilités d'une prestation.
// Une liste vide signifie « tous les véhicules que la station accepte ».
func (a *App) enregistrerCompatibilites(ctx context.Context, orgID, stationID, serviceID string, categories []string) error {
	if _, err := a.Pool.Exec(ctx,
		`DELETE FROM service_compatibilities WHERE service_id = $1`, serviceID); err != nil {
		return err
	}
	for _, code := range categories {
		if strings.TrimSpace(code) == "" {
			continue
		}
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO service_compatibilities
			  (service_id, category_code, organization_id, station_id, is_active, updated_at)
			VALUES ($1,$2,$3,$4,true,now())
			ON CONFLICT (service_id, category_code) DO UPDATE SET is_active = true, updated_at = now()`,
			serviceID, code, orgID, stationID); err != nil {
			return err
		}
	}
	return nil
}

// equiperCatalogueNeuf copie les modèles dans le catalogue de la station, **sans
// prix** : le compte neuf part d'un catalogue à tarifer, jamais d'un catalogue
// inventé. Idempotent : une prestation déjà issue d'un modèle n'est pas recréée.
func (a *App) equiperCatalogueNeuf(ctx context.Context, orgID, stationID string) error {
	for i, modele := range cataloguePrestations {
		var existe bool
		if err := a.Pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM services WHERE station_id = $1 AND template_code = $2)`,
			stationID, modele.Code).Scan(&existe); err != nil {
			return err
		}
		if existe {
			continue
		}
		includes := modele.Includes
		if includes == nil {
			includes = []string{}
		}
		id := newID("srv")
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO services (id, organization_id, station_id, name, category, description,
			  price_xof, duration_minutes, sort_order, code, family_code, template_code, includes,
			  equipment_notes, safety_notes, preconditions, is_free, is_quote_only)
			VALUES ($1,$2,$3,$4,'lavage',$5,0,$6,$7,$8,$9,$10,$11,$12,$13,$14,false,$15)`,
			id, orgID, stationID, modele.Nom, modele.Description, modele.DureeMinutes,
			(i+1)*10, modele.Code, modele.FamilleCode, modele.Code, includes,
			modele.Equipement, modele.Securite, modele.Prealables, modele.SurDevis); err != nil {
			return err
		}
		// Les compatibilités du modèle sont recopiées **telles quelles** : les
		// filtrer sur les catégories activées aujourd'hui donnerait « aucune
		// restriction », donc « s'applique à tous les véhicules » — l'inverse
		// du modèle, et le poids lourd proposé pour une berline. Une
		// compatibilité qui vise un véhicule non activé reste simplement
		// inerte, et devient juste dès que la station l'active.
		if err := a.enregistrerCompatibilites(ctx, orgID, stationID, id, modele.Categories); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Handlers — référentiel
// ---------------------------------------------------------------------------

// GET /api/v1/service-templates?station_id=…
//
// Le référentiel complet (familles + modèles, **sans prix**) et ce que la
// station en a déjà fait : c'est ce que l'écran du catalogue affiche.
func (a *App) handleServiceTemplates(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}

	familles, err := a.Pool.Query(r.Context(), `
		SELECT t.code, t.name, t.description,
		       COALESCE(f.name, ''), COALESCE(f.is_active, false), (f.station_id IS NOT NULL)
		FROM service_family_templates t
		LEFT JOIN service_families f ON f.code = t.code AND f.station_id = $1
		ORDER BY t.sort_order, t.name`, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer familles.Close()
	itemsFamilles := []map[string]any{}
	for familles.Next() {
		var code, nom, description, nomLocal string
		var active, configuree bool
		if err := familles.Scan(&code, &nom, &description, &nomLocal, &active, &configuree); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		itemsFamilles = append(itemsFamilles, map[string]any{
			"code": code, "name": nom, "description": description,
			"local_name": nomLocal, "is_active": active, "configured": configuree,
		})
	}

	lignes, err := a.Pool.Query(r.Context(), `
		SELECT t.code, t.name, t.family_code, COALESCE(f.name, t.family_code), t.description,
		       t.includes, t.duration_minutes, t.equipment_notes, t.safety_notes,
		       t.preconditions, t.is_quote_only, t.category_codes,
		       COALESCE(s.id, ''), COALESCE(s.price_xof, 0)
		FROM service_templates t
		LEFT JOIN service_family_templates f ON f.code = t.family_code
		LEFT JOIN services s ON s.template_code = t.code AND s.station_id = $1
		ORDER BY t.sort_order, t.name`, stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer lignes.Close()
	modeles := []map[string]any{}
	for lignes.Next() {
		var (
			code, nom, familleCode, familleNom, description string
			includes                                        []string
			duree                                           int
			equipement, securite, prealables                string
			surDevis                                        bool
			categories                                      []string
			serviceID                                       string
			prix                                            int
		)
		if err := lignes.Scan(&code, &nom, &familleCode, &familleNom, &description, &includes,
			&duree, &equipement, &securite, &prealables, &surDevis, &categories,
			&serviceID, &prix); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		modeles = append(modeles, map[string]any{
			"code": code, "name": nom, "family_code": familleCode, "family_name": familleNom,
			"description": description, "includes": includes, "duration_minutes": duree,
			"equipment_notes": equipement, "safety_notes": securite, "preconditions": prealables,
			"is_quote_only": surDevis, "category_codes": categories,
			"adopted": serviceID != "", "service_id": serviceID, "price_xof": prix,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": stationID, "families": itemsFamilles, "templates": modeles,
		"prix": "Aucun montant dans ce référentiel : ce sont des prestations à tarifer.",
	})
}

// GET /api/v1/service-families?station_id=…
func (a *App) handleServiceFamilies(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.URL.Query().Get("station_id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	items, err := a.lireFamillesStation(r.Context(), stationID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items), "station_id": stationID})
}

func (a *App) lireFamillesStation(ctx context.Context, stationID string, activesSeulement bool) ([]map[string]any, error) {
	filtre := ""
	if activesSeulement {
		filtre = " AND f.is_active"
	}
	lignes, err := a.Pool.Query(ctx, `
		SELECT f.code, f.kind, f.name, f.sort_order, f.is_active,
		       COALESCE(t.name, f.code), COALESCE(t.description, '')
		FROM service_families f
		LEFT JOIN service_family_templates t ON t.code = f.code
		WHERE f.station_id = $1`+filtre+`
		ORDER BY f.is_active DESC, f.sort_order, COALESCE(NULLIF(f.name, ''), t.name, f.code)`, stationID)
	if err != nil {
		return nil, err
	}
	defer lignes.Close()
	items := []map[string]any{}
	for lignes.Next() {
		var code, kind, nomLocal, nomModele, description string
		var ordre int
		var actif bool
		if err := lignes.Scan(&code, &kind, &nomLocal, &ordre, &actif, &nomModele, &description); err != nil {
			return nil, err
		}
		nom := strings.TrimSpace(nomLocal)
		if nom == "" {
			nom = nomModele
		}
		items = append(items, map[string]any{
			"id": code, "code": code, "kind": kind, "name": nom,
			"description": description, "sort_order": ordre, "is_active": actif,
		})
	}
	return items, lignes.Err()
}

// PUT /api/v1/stations/{id}/service-families — activation en bloc.
func (a *App) handleStationServiceFamilies(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID := r.PathValue("id")
	if err := a.stationAutorisee(r.Context(), deref(user.OrganizationID), stationID); err != nil {
		writeAPIError(w, err)
		return
	}
	var body struct {
		Families []struct {
			Code      string `json:"code"`
			IsActive  *bool  `json:"is_active"`
			Name      string `json:"name"`
			SortOrder *int   `json:"sort_order"`
		} `json:"families"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	erreurs := map[string]string{}
	for _, f := range body.Families {
		if strings.TrimSpace(f.Code) == "" {
			erreurs["code"] = "Chaque famille doit porter un code."
			continue
		}
		var existe bool
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM service_family_templates WHERE code = $1)`, f.Code).Scan(&existe); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		if !existe {
			erreurs[f.Code] = "Famille inconnue du référentiel."
		}
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucune famille n'a été enregistrée : corrigez les codes signalés.", erreurs)
		return
	}
	for _, f := range body.Families {
		actif := true
		if f.IsActive != nil {
			actif = *f.IsActive
		}
		ordre := 0
		if f.SortOrder != nil {
			ordre = *f.SortOrder
		}
		if _, err := a.Pool.Exec(r.Context(), `
			INSERT INTO service_families (id, organization_id, station_id, code, kind, name, sort_order, is_active, updated_at)
			VALUES ($1,$2,$3,$4,'modele',$5,$6,$7,now())
			ON CONFLICT (station_id, code) DO UPDATE SET
			  name = CASE WHEN EXCLUDED.name = '' THEN service_families.name ELSE EXCLUDED.name END,
			  sort_order = CASE WHEN EXCLUDED.sort_order = 0 THEN service_families.sort_order ELSE EXCLUDED.sort_order END,
			  is_active = EXCLUDED.is_active, updated_at = now()`,
			newID("fam"), deref(user.OrganizationID), stationID, f.Code,
			strings.TrimSpace(f.Name), ordre, actif); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"service_families.updated", "station", stationID,
		map[string]any{"configures": len(body.Families)})
	items, err := a.lireFamillesStation(r.Context(), stationID, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	actives := 0
	for _, item := range items {
		if actif, _ := item["is_active"].(bool); actif {
			actives++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(items), "actives": actives,
		"message": "Familles enregistrées.",
	})
}

// ---------------------------------------------------------------------------
// Handlers — compatibilités d'une prestation
// ---------------------------------------------------------------------------

// GET /api/v1/services/{id}/compatibilities
func (a *App) handleServiceCompatibilities(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	id := r.PathValue("id")
	service, err := a.getOwnedService(r, id, orgID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	compatibles, err := a.compatibilitesService(r.Context(), service.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	categories, err := a.lireCategoriesStation(r.Context(), service.StationID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	disponibles := []map[string]any{}
	for _, c := range categories {
		disponibles = append(disponibles, map[string]any{"code": c.Code, "name": c.Nom})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service_id": service.ID, "categories": compatibles, "categories_disponibles": disponibles,
		"message": "Sans compatibilité déclarée, la prestation est proposée pour tous les véhicules acceptés par la station.",
	})
}

// compatibilitesService : les codes de catégories liés à une prestation.
func (a *App) compatibilitesService(ctx context.Context, serviceID string) ([]string, error) {
	lignes, err := a.Pool.Query(ctx, `
		SELECT category_code FROM service_compatibilities
		WHERE service_id = $1 AND is_active ORDER BY category_code`, serviceID)
	if err != nil {
		return nil, err
	}
	defer lignes.Close()
	codes := []string{}
	for lignes.Next() {
		var code string
		if err := lignes.Scan(&code); err == nil {
			codes = append(codes, code)
		}
	}
	return codes, lignes.Err()
}

// PUT /api/v1/services/{id}/compatibilities — {"categories": ["berline", …]}
func (a *App) handleServiceCompatibilitiesUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	id := r.PathValue("id")
	service, err := a.getOwnedService(r, id, orgID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var body struct {
		Categories []string `json:"categories"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	// Une compatibilité ne peut viser qu'un véhicule que la station accepte.
	actifs := map[string]bool{}
	categories, err := a.lireCategoriesStation(r.Context(), service.StationID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	for _, c := range categories {
		actifs[c.Code] = true
	}
	erreurs := map[string]string{}
	for _, code := range body.Categories {
		if !actifs[code] {
			erreurs[code] = "Catégorie non activée pour cette station."
		}
	}
	if len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Aucune compatibilité n'a été enregistrée : corrigez les catégories signalées.", erreurs)
		return
	}
	if err := a.enregistrerCompatibilites(r.Context(), orgID, service.StationID, service.ID, body.Categories); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), orgID, service.StationID, user.FullName,
		"service.compatibilities", "service", service.ID,
		map[string]any{"categories": len(body.Categories)})
	compatibles, _ := a.compatibilitesService(r.Context(), service.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"service_id": service.ID, "categories": compatibles, "message": "Compatibilités enregistrées.",
	})
}

// ---------------------------------------------------------------------------
// Handlers — options et suppléments
// ---------------------------------------------------------------------------

const optionColumns = `id, service_id, category_code, name, description, price_xof, duration_minutes, sort_order, is_active`

// GET /api/v1/service-options?station_id=…[&service_id=…]
func (a *App) handleServiceOptions(w http.ResponseWriter, r *http.Request) {
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
	serviceID := r.URL.Query().Get("service_id")
	includeInactive := r.URL.Query().Get("include_inactive") == "true"
	lignes, err := a.Pool.Query(r.Context(), `
		SELECT `+optionColumns+` FROM service_options
		WHERE station_id = $1 AND ($2 = '' OR service_id = $2 OR service_id IS NULL)
		  AND ($3 OR is_active)
		ORDER BY sort_order, name`, stationID, serviceID, includeInactive)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer lignes.Close()
	items := []map[string]any{}
	for lignes.Next() {
		var (
			id, nom, description, categorie string
			prix, duree, ordre              int
			serviceLie                      *string
			actif                           bool
		)
		if err := lignes.Scan(&id, &serviceLie, &categorie, &nom, &description, &prix, &duree, &ordre, &actif); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
			return
		}
		items = append(items, map[string]any{
			"id": id, "service_id": serviceLie, "category_code": categorie, "name": nom,
			"description": description, "price_xof": prix, "duration_minutes": duree,
			"sort_order": ordre, "is_active": actif,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// POST /api/v1/service-options
func (a *App) handleServiceOptionCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		StationID       string `json:"station_id"`
		ServiceID       string `json:"service_id"`
		CategoryCode    string `json:"category_code"`
		Name            string `json:"name"`
		Description     string `json:"description"`
		PriceXOF        int    `json:"price_xof"`
		DurationMinutes int    `json:"duration_minutes"`
		SortOrder       int    `json:"sort_order"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le nom de l'option est obligatoire.", map[string]string{"name": "Nom obligatoire."})
		return
	}
	if body.PriceXOF < 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le supplément ne peut pas être négatif.", map[string]string{"price_xof": "Montant négatif"})
		return
	}
	stationID, err := a.resolveStation(r.Context(), user, body.StationID)
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var serviceLie *string
	if body.ServiceID != "" {
		service, err := a.getOwnedService(r, body.ServiceID, deref(user.OrganizationID))
		if err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Prestation inconnue.", map[string]string{"service_id": "Prestation inconnue"})
			return
		}
		serviceLie = &service.ID
	}
	id := newID("opt")
	if _, err := a.Pool.Exec(r.Context(), `
		INSERT INTO service_options (id, organization_id, station_id, service_id, category_code,
		  name, description, price_xof, duration_minutes, sort_order)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		id, deref(user.OrganizationID), stationID, serviceLie, body.CategoryCode,
		strings.TrimSpace(body.Name), body.Description, body.PriceXOF,
		body.DurationMinutes, body.SortOrder); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"service_option.created", "service_option", id,
		map[string]any{"name": body.Name, "price_xof": body.PriceXOF})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "name": body.Name, "price_xof": body.PriceXOF, "service_id": serviceLie,
		"category_code": body.CategoryCode, "description": body.Description,
		"duration_minutes": body.DurationMinutes, "sort_order": body.SortOrder, "is_active": true,
	})
}

// PATCH /api/v1/service-options/{id} — prix, libellé, portée, activation.
func (a *App) handleServiceOptionUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	id := r.PathValue("id")

	var stationID string
	var portee *string
	if err := a.Pool.QueryRow(r.Context(),
		`SELECT station_id, service_id FROM service_options WHERE id = $1 AND organization_id = $2`,
		id, orgID).Scan(&stationID, &portee); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Option introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != stationID {
		writeAPIError(w, errStationForbidden)
		return
	}

	var body struct {
		Name         *string `json:"name"`
		Description  *string `json:"description"`
		PriceXOF     *int    `json:"price_xof"`
		SortOrder    *int    `json:"sort_order"`
		IsActive     *bool   `json:"is_active"`
		CategoryCode *string `json:"category_code"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	// Numérotation explicite : $1 est l'identifiant, $2 l'entreprise, les
	// champs modifiés suivent. Une numérotation qui repartait de 1 envoyait le
	// libellé dans `organization_id` — la mise à jour échouait et l'écran
	// annonçait un échec interne sans dire pourquoi.
	mises := []string{}
	args := []any{}
	n := 2
	ajouter := func(fragment string, valeur any) {
		n++
		mises = append(mises, fragment+" = $"+itoa(n))
		args = append(args, valeur)
	}
	if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
		ajouter("name", strings.TrimSpace(*body.Name))
	}
	if body.Description != nil {
		ajouter("description", *body.Description)
	}
	if body.PriceXOF != nil {
		if *body.PriceXOF < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le supplément ne peut pas être négatif.", map[string]string{"price_xof": "Montant négatif"})
			return
		}
		ajouter("price_xof", *body.PriceXOF)
	}
	if body.SortOrder != nil {
		ajouter("sort_order", *body.SortOrder)
	}
	if body.IsActive != nil {
		ajouter("is_active", *body.IsActive)
	}
	if body.CategoryCode != nil {
		ajouter("category_code", *body.CategoryCode)
	}
	if len(mises) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "message": "Aucune modification."})
		return
	}
	mises = append(mises, "updated_at = now()")
	if _, err := a.Pool.Exec(r.Context(), `
		UPDATE service_options SET `+strings.Join(mises, ", ")+`
		WHERE id = $1 AND organization_id = $2`, append([]any{id, orgID}, args...)...); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	a.addAudit(r.Context(), orgID, stationID, user.FullName,
		"service_option.updated", "service_option", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "message": "Option mise à jour."})
}

// DELETE /api/v1/service-options/{id} — désactivation (jamais de suppression :
// une fiche passée peut citer cette option).
func (a *App) handleServiceOptionDisable(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	orgID := deref(user.OrganizationID)
	id := r.PathValue("id")
	etiquette, err := a.Pool.Exec(r.Context(),
		`UPDATE service_options SET is_active = false, updated_at = now()
		 WHERE id = $1 AND organization_id = $2 AND is_active`, id, orgID)
	if err != nil || etiquette.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "not_found", "Option introuvable.", nil)
		return
	}
	a.addAudit(r.Context(), orgID, "", user.FullName, "service_option.disabled", "service_option", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"message": "Option désactivée."})
}
