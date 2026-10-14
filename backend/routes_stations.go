package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// routes_stations.go — création et fiche de la station : c'est le cœur du
// parcours d'onboarding. Chaque formulaire front (StationForm, HorairesForm,
// UsagesForm, VehiculesForm…) écrit ici, et toute relecture repart de la base.

func (a *App) enregistrerRoutesStations(rt *routeur) {
	rt.ajouter("GET", "/api/v1/stations", a.listerStations)            // station:view
	rt.ajouter("POST", "/api/v1/stations", a.creerStation)              // organization:manage
	rt.proteger("GET", "/api/v1/stations/", "", a.dispatchStation)      // contrôle fin dans le dispatch
}

// dispatchStation route /stations/{id}, /stations/{id}/profile,
// /stations/{id}/opening-hours, /stations/{id}/vehicle-*.
func (a *App) dispatchStation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	id := cheminParam(r, "/api/v1/stations/")
	ctx := r.Context()
	station, ok := a.stationVerifiee(ctx, sess, id)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "not_found", "Station introuvable.")
		return
	}
	suffrage := strings.TrimPrefix(r.URL.Path, "/api/v1/stations/"+id)
	switch {
	case r.Method == "GET" && suffrage == "":
		ecrireJSON(w, http.StatusOK, map[string]interface{}{"station": station.rendre()})
	case r.Method == "PATCH" && suffrage == "":
		a.modifierStation(w, r, sess, station)
	case r.Method == "GET" && suffrage == "/profile":
		a.profilStation(w, r, sess, station)
	case r.Method == "PUT" && suffrage == "/opening-hours":
		a.enregistrerHoraires(w, r, sess, station)
	case r.Method == "PUT" && suffrage == "/vehicle-categories":
		a.activerCategories(w, r, sess, station)
	case r.Method == "PUT" && suffrage == "/vehicle-usages":
		a.activerUsages(w, r, sess, station)
	case r.Method == "PUT" && suffrage == "/vehicle-usage-categories":
		a.declarerCouples(w, r, sess, station)
	default:
		repondreErreur(w, http.StatusNotFound, "not_found", "Ressource introuvable.")
	}
}

func (a *App) listerStations(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("station:view") && !sess.aPermission("settings:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	ctx := r.Context()
	rows, err := a.db.Query(ctx, `SELECT id FROM stations WHERE organization_id = $1 ORDER BY is_default DESC, created_at ASC`, orElse(sess.OrganizationID, ""))
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	liste := []map[string]interface{}{}
	for _, id := range ids {
		if st, err := a.lireStationParID(ctx, id); err == nil {
			item := st.rendre()
			item["opening_hours"] = a.hoursMapPourRendu(ctx, id)
			liste = append(liste, item)
		}
	}
	ecrireJSON(w, http.StatusOK, liste)
}

// --- Création ---------------------------------------------------------------

type corpsStation struct {
	Name                string   `json:"name"`
	Code                string   `json:"code"`
	Phone               string   `json:"phone"`
	Address             string   `json:"address"`
	City                string   `json:"city"`
	Commune             string   `json:"commune"`
	DistrictArea        string   `json:"district_area"`
	Timezone            string   `json:"timezone"`
	Bays                *int     `json:"bays"`
	WaterSource         string   `json:"water_source"`
	WaterReserveLiters  *int     `json:"water_reserve_liters"`
	WashTypes           []string `json:"wash_types"`
	Latitude            string   `json:"latitude"`
	Longitude           string   `json:"longitude"`
}

var typesLavageConnus = map[string]string{
	"exterieur": "Lavage extérieur",
	"interieur": "Nettoyage intérieur",
	"complet":   "Lavage complet",
	"special":   "Traitements spéciaux",
	"aspiration":"Aspiration seule",
}

var sourcesEauConnues = map[string]bool{
	"ville": true, "forage": true, "camion_citerne": true, "pluie": true, "autre": true,
}

func (a *App) creerStation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("organization:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	corps, ok := decoderCorps[corpsStation](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	organisationID := orElse(sess.OrganizationID, "")
	if organisationID == "" {
		repondreErreur(w, http.StatusBadRequest, "validation_error", "Ce compte n'est rattaché à aucune entreprise.")
		return
	}

	champs := map[string]string{}
	nom := strings.TrimSpace(corps.Name)
	if len(nom) < 2 || len(nom) > 80 {
		champs["name"] = "Le nom de la station est requis (2 à 80 caractères)."
	}
	code := strings.ToUpper(strings.TrimSpace(corps.Code))
	if code == "" {
		code = genererCode(6)
	} else if !reCODE.MatchString(code) {
		champs["code"] = "Code invalide : lettres, chiffres, tirets (2 à 32 caractères)."
	}
	validerTelephone("phone", corps.Phone, champs)
	if corps.Bays != nil && (*corps.Bays < 0 || *corps.Bays > 50) {
		champs["bays"] = "Nombre de pistes invraisemblable (0 à 50)."
	}
	if corps.WaterReserveLiters != nil && (*corps.WaterReserveLiters < 0 || *corps.WaterReserveLiters > 1000000) {
		champs["water_reserve_liters"] = "Réserve d'eau invraisemblable."
	}
	if corps.WaterSource != "" && !sourcesEauConnues[corps.WaterSource] {
		champs["water_source"] = "Source d'eau inconnue (ville, forage, camion_citerne, pluie, autre)."
	}
	for _, t := range corps.WashTypes {
		if _, connu := typesLavageConnus[strings.TrimSpace(t)]; !connu {
			champs["wash_types"] = "Type de lavage inconnu : " + strings.TrimSpace(t)
			break
		}
	}
	validerCoordonnee("latitude", strings.TrimSpace(corps.Latitude), -90, 90, champs)
	validerCoordonnee("longitude", strings.TrimSpace(corps.Longitude), -180, 180, champs)
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}

	// Quota du plan avant toute écriture.
	if blocage, limite := a.quotaStationsAtteint(ctx, organisationID); blocage {
		ecrireConflit(w, http.StatusForbidden, conflit{
			code:    "plan_limit",
			message: fmt.Sprintf("Votre formule (%s) permet %d station(s). Créez une nouvelle entreprise ou changez de formule.", limite.plan, limite.max),
		})
		return
	}

	site := map[string]interface{}{
		"wash_types":            corps.WashTypes,
		"bays":                  nombreOuNul(corps.Bays),
		"simultaneous_vehicles": nil,
		"has_vacuum":            nil,
		"has_drying":            nil,
		"has_detailing":         nil,
		"has_waiting_room":      nil,
		"has_storage":           nil,
		"water_source":          strings.TrimSpace(corps.WaterSource),
		"water_reserve_liters":  nombreOuNulEntier(corps.WaterReserveLiters),
		"power_supply":          "",
		"has_generator":         nil,
		"notes":                 "",
		"constraints":           "",
	}
	ville := strings.TrimSpace(corps.City)
	if ville == "" {
		ville = "Abidjan"
	}
	fuseau := strings.TrimSpace(corps.Timezone)
	if fuseau == "" {
		fuseau = "Africa/Abidjan"
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)

	// Code unique : réessayer une fois si collision fortuite sur un code généré.
	var stationID string
	for tentative := 0; tentative < 2; tentative++ {
		err = tx.QueryRow(ctx, `
			INSERT INTO stations (organization_id, name, code, phone, address, city, commune, district_area,
			                      latitude, longitude, timezone, site, settings, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9,'')::numeric, NULLIF($10,'')::numeric, $11,
			        $12::jsonb, '{}'::jsonb, false)
			RETURNING id`,
			organisationID, nom, code, strings.TrimSpace(corps.Phone), strings.TrimSpace(corps.Address),
			ville, strings.TrimSpace(corps.Commune), strings.TrimSpace(corps.DistrictArea),
			strings.TrimSpace(corps.Latitude), strings.TrimSpace(corps.Longitude), fuseau, jsonBrut(site)).Scan(&stationID)
		if err == nil {
			break
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "stations_code_key" {
			if corps.Code != "" {
				ecrireConflit(w, http.StatusConflict, conflit{code: "station_code_taken", message: "Ce code de station est déjà utilisé.", fields: map[string]string{"code": "Code déjà pris."}})
				return
			}
			code = genererCode(6)
			continue
		}
		if errors.Is(err, pgx.ErrNoRows) {
			err = errors.New("insertion sans retour")
		}
		ecrireInterne(w, err)
		return
	}

	// La première station de l'entreprise devient sa station par défaut, et le
	// fondateur y est rattaché : les écrans « ma station » fonctionnent dès lors.
	var nbStations int
	_ = tx.QueryRow(ctx, `SELECT count(*) FROM stations WHERE organization_id = $1`, organisationID).Scan(&nbStations)
	if nbStations == 1 {
		_, _ = tx.Exec(ctx, `UPDATE stations SET is_default = true WHERE id = $1`, stationID)
		_, _ = tx.Exec(ctx, `UPDATE users SET station_id = $1 WHERE organization_id = $2 AND station_id IS NULL AND role IN ('owner','manager')`, stationID, organisationID)
	}

	// Prestations seedées SANS prix : l'application ne propose aucun tarif.
	for _, m := range modelesPrestation {
		_, _ = tx.Exec(ctx, `
			INSERT INTO services (organization_id, station_id, family_code, name, description, duration_minutes, price_xof, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, 0, true)`,
			organisationID, stationID, m.famille, m.nom, m.description, 30)
	}
	// Moyens de paiement du référentiel produit, copiés pour la station.
	_, _ = tx.Exec(ctx, `
		INSERT INTO payment_methods (organization_id, station_id, code, name, family, requires_reference, sort_order)
		SELECT $1, $2, code, name, family, requires_reference, sort_order FROM payment_methods WHERE station_id IS NULL`,
		organisationID, stationID)
	// Les sept jours existent toujours : jour non décrit = fermé.
	for jour := 1; jour <= 7; jour++ {
		_, _ = tx.Exec(ctx, `
			INSERT INTO opening_hours (station_id, weekday, is_open) VALUES ($1, $2, false)
			ON CONFLICT (station_id, weekday) DO NOTHING`, stationID, jour)
	}

	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	a.journaliser(ctx, &organisationID, &stationID, &sess.UserID, "station.created",
		map[string]interface{}{"name": nom, "code": code})

	station, err := a.lireStationParID(ctx, stationID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	ecrireJSON(w, http.StatusCreated, station.rendre())
}

type infoLimite struct {
	plan string
	max  int
}

// quotaStationsAtteint s'appuie sur l'abonnement courant ; sans abonnement,
// on se fie au plan « Découverte » (1 station).
func (a *App) quotaStationsAtteint(ctx context.Context, organisationID string) (bool, infoLimite) {
	var max int
	var code string
	err := a.db.QueryRow(ctx, `
		SELECT p.max_stations, p.code
		FROM subscriptions s JOIN plans p ON p.id = s.plan_id
		WHERE s.organization_id = $1 AND s.status IN ('trialing','active')
		ORDER BY s.created_at DESC LIMIT 1`, organisationID).Scan(&max, &code)
	if err != nil {
		max, code = 1, "decouverte"
	}
	var nb int
	_ = a.db.QueryRow(ctx, `SELECT count(*) FROM stations WHERE organization_id = $1`, organisationID).Scan(&nb)
	return nb >= max, infoLimite{plan: code, max: max}
}

func nombreOuNul(v *int) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func nombreOuNulEntier(v *int) interface{} { return nombreOuNul(v) }

func jsonBrut(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// --- PATCH /stations/{id} -----------------------------------------------------

func (a *App) modifierStation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur, station *stationEntite) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	brut, ok := decoderCorpsLibre(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	champs := map[string]string{}

	nom := lireChampTexte(brut, "name")
	if nom.present {
		if len(nom.value) < 2 || len(nom.value) > 80 {
			champs["name"] = "Le nom de la station est requis (2 à 80 caractères)."
		}
	}
	telephone := lireChampTexte(brut, "phone")
	if telephone.present {
		validerTelephone("phone", telephone.value, champs)
	}
	latitude := lireChampTexte(brut, "latitude")
	if latitude.present {
		validerCoordonnee("latitude", latitude.value, -90, 90, champs)
	}
	longitude := lireChampTexte(brut, "longitude")
	if longitude.present {
		validerCoordonnee("longitude", longitude.value, -180, 180, champs)
	}
	isDefault := lireChampTri(brut, "is_default")
	isActive := lireChampTri(brut, "is_active")

	// Objet `site` : fusion champ à champ avec ce qui existe déjà ; les clés
	// absentes ne sont pas effacées, les capacités gardent leur tri-état.
	siteFusionne := station.Site
	if siteFusionne == nil {
		siteFusionne = map[string]interface{}{}
	}
	if brutSite, present := brut["site"]; present {
		var maj map[string]json.RawMessage
		if err := json.Unmarshal(brutSite, &maj); err != nil {
			champs["site"] = "Objet site attendu."
		} else {
			for cle, valeur := range maj {
				switch cle {
				case "wash_types":
					var liste []string
					if err := json.Unmarshal(valeur, &liste); err != nil {
						champs["wash_types"] = "Liste de types de lavage attendue."
						continue
					}
					propres := []string{}
					for _, t := range liste {
						t = strings.TrimSpace(t)
						if _, connu := typesLavageConnus[t]; !connu {
							champs["wash_types"] = "Type de lavage inconnu : " + t
							continue
						}
						propres = append(propres, t)
					}
					siteFusionne["wash_types"] = propres
				case "bays", "simultaneous_vehicles", "water_reserve_liters":
					n := lireChampNombre(maj, cle)
					if n.value != nil && (*n.value < 0 || *n.value > 1000000) {
						champs[cle] = "Valeur invraisemblable."
						continue
					}
					if n.value == nil {
						siteFusionne[cle] = nil
					} else {
						siteFusionne[cle] = *n.value
					}
				case "has_vacuum", "has_drying", "has_detailing", "has_waiting_room", "has_storage", "has_generator":
					appliquerTri(siteFusionne, cle, lireChampTri(maj, cle))
				case "water_source":
					txt := lireChampTexte(maj, cle)
					if txt.value != "" && !sourcesEauConnues[txt.value] {
						champs["water_source"] = "Source d'eau inconnue."
						continue
					}
					siteFusionne["water_source"] = txt.value
				case "notes", "constraints", "power_supply":
					txt := lireChampTexte(maj, cle)
					siteFusionne[cle] = txt.value
				default:
					// clé inconnue : conservée telle quelle (extension future)
					var v interface{}
					_ = json.Unmarshal(valeur, &v)
					siteFusionne[cle] = v
				}
			}
		}
	}

	settingsFusionnes := station.Settings
	if settingsFusionnes == nil {
		settingsFusionnes = map[string]interface{}{}
	}
	if brutSettings, present := brut["settings"]; present {
		var maj map[string]interface{}
		if err := json.Unmarshal(brutSettings, &maj); err != nil {
			champs["settings"] = "Objet settings attendu."
		} else {
			for cle, v := range maj {
				settingsFusionnes[cle] = v
			}
		}
	}

	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE stations SET
		  name = COALESCE(NULLIF($2, '__absent__'), name),
		  phone = COALESCE(NULLIF($3, '__absent__'), phone),
		  address = COALESCE(NULLIF($4, '__absent__'), address),
		  city = COALESCE(NULLIF($5, '__absent__'), city),
		  country = COALESCE(NULLIF($6, '__absent__'), country),
		  district = COALESCE(NULLIF($7, '__absent__'), district),
		  commune = COALESCE(NULLIF($8, '__absent__'), commune),
		  district_area = COALESCE(NULLIF($9, '__absent__'), district_area),
		  street = COALESCE(NULLIF($10, '__absent__'), street),
		  landmark = COALESCE(NULLIF($11, '__absent__'), landmark),
		  latitude = CASE WHEN $12 = '__absent__' THEN latitude ELSE NULLIF($12,'')::numeric END,
		  longitude = CASE WHEN $13 = '__absent__' THEN longitude ELSE NULLIF($13,'')::numeric END,
		  billing_address = COALESCE(NULLIF($14, '__absent__'), billing_address),
		  timezone = COALESCE(NULLIF($15, '__absent__'), timezone),
		  logo_url = COALESCE(NULLIF($16, '__absent__'), logo_url),
		  site = $17::jsonb,
		  settings = $18::jsonb,
		  updated_at = now()
		WHERE id = $1`,
		station.ID,
		envole(nom), envole(telephone), texteAbsent(brut, "address"), texteAbsent(brut, "city"),
		texteAbsent(brut, "country"), texteAbsent(brut, "district"), envole(lireChampTexte(brut, "commune")),
		texteAbsent(brut, "district_area"), texteAbsent(brut, "street"), texteAbsent(brut, "landmark"),
		marque(latitude), marque(longitude), texteAbsent(brut, "billing_address"),
		texteAbsent(brut, "timezone"), texteAbsent(brut, "logo_url"),
		jsonBrut(siteFusionne), jsonBrut(settingsFusionnes)); err != nil {
		ecrireInterne(w, err)
		return
	}
	if isDefault.present && isDefault.value != nil && *isDefault.value {
		_, _ = tx.Exec(ctx, `UPDATE stations SET is_default = false WHERE organization_id = $1 AND id <> $2`,
			station.OrganizationID, station.ID)
		_, _ = tx.Exec(ctx, `UPDATE stations SET is_default = true WHERE id = $1`, station.ID)
	}
	if isActive.present && isActive.value != nil {
		_, _ = tx.Exec(ctx, `UPDATE stations SET is_active = $2, updated_at = now() WHERE id = $1`, station.ID, *isActive.value)
	}
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	actualisee, err := a.lireStationParID(ctx, station.ID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	a.journaliser(ctx, &station.OrganizationID, &station.ID, &sess.UserID, "station.updated", map[string]interface{}{"fields": keysOf(brut)})
	ecrireJSON(w, http.StatusOK, actualisee.rendre())
}

const marqueAbsente = "__absent__"

func envole(c champTexte) string {
	if !c.present {
		return marqueAbsente
	}
	return c.value
}

func marque(c champTexte) string { return envole(c) }

func texteAbsent(brut map[string]json.RawMessage, cle string) string {
	return envole(lireChampTexte(brut, cle))
}

func keysOf(m map[string]json.RawMessage) []string {
	cles := make([]string, 0, len(m))
	for k := range m {
		cles = append(cles, k)
	}
	return cles
}

// --- Fiche complète -----------------------------------------------------------

func (a *App) profilStation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur, station *stationEntite) {
	if !sess.aPermission("settings:view") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	ctx := r.Context()
	jours, exceptions, err := a.lireSemaine(ctx, station.ID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	typesProposes := []string{}
	if bruts, ok := station.Site["wash_types"].([]interface{}); ok {
		for _, b := range bruts {
			if s, is := b.(string); is && s != "" {
				typesProposes = append(typesProposes, s)
			}
		}
	}
	site := sitePourRendu(station.Site)
	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"station":                 rendreAvecHeures(ctx, station),
		"site":                    site,
		"days":                    jours,
		"exceptions":              exceptions,
		"types_lavage_proposes":   typesProposes,
	})
}

func rendreAvecHeures(ctx context.Context, station *stationEntite) map[string]interface{} {
	out := station.rendre()
	// heures rendues via hoursMapPourRendu (appelé par le listing ci-dessous)
	return out
}

var _ = fmt.Sprint

// sitePourRendu garantit le tri-état : null reste null, jamais remplacé par false.
func sitePourRendu(site map[string]interface{}) map[string]interface{} {
	defauts := map[string]interface{}{
		"wash_types":            []string{},
		"bays":                  nil,
		"simultaneous_vehicles": nil,
		"has_vacuum":            nil,
		"has_drying":            nil,
		"has_detailing":         nil,
		"has_waiting_room":      nil,
		"has_storage":           nil,
		"water_source":          "",
		"water_reserve_liters":  nil,
		"power_supply":          "",
		"has_generator":         nil,
		"notes":                 "",
		"constraints":           "",
	}
	for cle, valeur := range defauts {
		if _, present := site[cle]; !present {
			site[cle] = valeur
		}
	}
	return site
}

type ligneJour struct {
	Weekday       int    `json:"weekday"`
	IsOpen        bool   `json:"is_open"`
	OpensAt       string `json:"opens_at"`
	ClosesAt      string `json:"closes_at"`
	BreakStart    string `json:"break_start"`
	BreakEnd      string `json:"break_end"`
	LastCheckinAt string `json:"last_checkin_at"`
	BookingAllowed bool  `json:"booking_allowed"`
}

// lireSemaine renvoie toujours sept lignes : un jour non décrit est fermé.
func (a *App) lireSemaine(ctx context.Context, stationID string) ([]ligneJour, []map[string]interface{}, error) {
	rows, err := a.db.Query(ctx, `
		SELECT weekday, is_open, opens_at, closes_at, break_start, break_end, last_checkin_at, booking_allowed
		FROM opening_hours WHERE station_id = $1`, stationID)
	if err != nil {
		return nil, nil, err
	}
	parJour := map[int]ligneJour{}
	for rows.Next() {
		var l ligneJour
		if err := rows.Scan(&l.Weekday, &l.IsOpen, &l.OpensAt, &l.ClosesAt, &l.BreakStart, &l.BreakEnd,
			&l.LastCheckinAt, &l.BookingAllowed); err == nil {
			parJour[l.Weekday] = l
		}
	}
	rows.Close()
	jours := []ligneJour{}
	for jour := 1; jour <= 7; jour++ {
		if l, ok := parJour[jour]; ok {
			jours = append(jours, l)
			continue
		}
		jours = append(jours, ligneJour{Weekday: jour, IsOpen: false, BookingAllowed: true})
	}
	exRows, err := a.db.Query(ctx, `
		SELECT id::text, day::text, is_open, opens_at, closes_at, label
		FROM opening_exceptions WHERE station_id = $1 ORDER BY day`, stationID)
	if err != nil {
		return nil, nil, err
	}
	defer exRows.Close()
	exceptions := []map[string]interface{}{}
	for exRows.Next() {
		var id, day string
		var ouvert bool
		var o, f, libelle string
		if err := exRows.Scan(&id, &day, &ouvert, &o, &f, &libelle); err == nil {
			exceptions = append(exceptions, map[string]interface{}{
				"id": id, "day": day, "is_open": ouvert, "opens_at": o, "closes_at": f, "label": libelle,
			})
		}
	}
	return jours, exceptions, nil
}

func (a *App) hoursMapPourRendu(ctx context.Context, stationID string) map[string]interface{} {
	jours, _, _ := a.lireSemaine(ctx, stationID)
	out := map[string]interface{}{}
	labels := []string{"lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi", "dimanche"}
	for _, j := range jours {
		entree := map[string]interface{}{"ouverture": j.OpensAt, "fermeture": j.ClosesAt}
		if !j.IsOpen {
			entree["ferme"] = true
		}
		out[labels[j.Weekday-1]] = entree
	}
	return out
}

// --- Horaires ------------------------------------------------------------------

var nomsJours = []string{"lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi", "dimanche"}

type saisieJour struct {
	Weekday        int     `json:"weekday"`
	IsOpen         *bool   `json:"is_open"`
	OpensAt        string  `json:"opens_at"`
	ClosesAt       string  `json:"closes_at"`
	BreakStart     string  `json:"break_start"`
	BreakEnd       string  `json:"break_end"`
	LastCheckinAt  string  `json:"last_checkin_at"`
	BookingAllowed *bool   `json:"booking_allowed"`
}

type saisieException struct {
	Day     string `json:"day"`
	IsOpen  *bool  `json:"is_open"`
	OpensAt string `json:"opens_at"`
	ClosesAt string `json:"closes_at"`
	Label   string `json:"label"`
}

func (a *App) enregistrerHoraires(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur, station *stationEntite) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	corps, ok := decoderCorps[struct {
		Days       []saisieJour      `json:"days"`
		Exceptions []saisieException `json:"exceptions"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	champs := map[string]string{}

	validerLigne := func(nom string, ouvert bool, o, f, pauseD, pauseF, dernier string) {
		if !ouvert {
			if o != "" || f != "" {
				champs[nom] = "Jour fermé : retirez les horaires."
			}
			return
		}
		obligatoire := true
		validerHeure(nom, o, obligatoire, champs)
		validerHeure(nom, f, obligatoire, champs)
		if _, deja := champs[nom]; deja {
			return
		}
		minO, minF := minutesDepuisMinuit(o), minutesDepuisMinuit(f)
		if minO >= 0 && minF >= 0 && minF <= minO {
			champs[nom] = "La fermeture doit suivre l'ouverture."
			return
		}
		if (pauseD == "") != (pauseF == "") {
			champs[nom] = "Pause : indiquez le début et la fin, ou laissez vide."
			return
		}
		if pauseD != "" {
			pd, pf := minutesDepuisMinuit(pauseD), minutesDepuisMinuit(pauseF)
			if pd >= 0 && pf >= 0 && (pd < minO || pf > minF || pf <= pd) {
				champs[nom] = "La pause doit rester dans les heures d'ouverture."
				return
			}
		}
		if dernier != "" {
			dl := minutesDepuisMinuit(dernier)
			if dl < minO || dl > minF {
				champs[nom] = "Dernier enregistrement hors des heures d'ouverture."
			}
		}
	}

	vus := map[int]bool{}
	for _, j := range corps.Days {
		if j.Weekday < 1 || j.Weekday > 7 {
			champs["jours"] = "Chaque ligne doit porter un weekday entre 1 (lundi) et 7 (dimanche)."
			continue
		}
		if vis :=vus[j.Weekday]; vis {
			champs[nomsJours[j.Weekday-1]] = "Jour déclaré deux fois."
			continue
		}
		vus[j.Weekday] = true
		ouvert := j.IsOpen == nil || *j.IsOpen
		nom := nomsJours[j.Weekday-1]
		if _, faute := champs[nom]; !faute {
			validerLigne(nom, ouvert, j.OpensAt, j.ClosesAt, j.BreakStart, j.BreakEnd, j.LastCheckinAt)
		}
	}

	vuesDates := map[string]bool{}
	for _, e := range corps.Exceptions {
		if !reDATE.MatchString(e.Day) {
			champs["exceptions"] = "Date d'exception attendue au format AAAA-MM-JJ."
			continue
		}
		if vuesDates[e.Day] {
			champs[e.Day] = "Une seule exception par date."
			continue
		}
		vuesDates[e.Day] = true
		ouvert := e.IsOpen != nil && *e.IsOpen
		if _, faute := champs[e.Day]; faute {
			continue
		}
		if ouvert {
			validerLigne(e.Day, true, e.OpensAt, e.ClosesAt, "", "", "")
		} else if e.OpensAt != "" || e.ClosesAt != "" {
			champs[e.Day] = "Jour fermé : retirez les horaires."
		}
	}

	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)
	for _, j := range corps.Days {
		ouvert := true
		if j.IsOpen != nil {
			ouvert = *j.IsOpen
		}
		reservation := true
		if j.BookingAllowed != nil {
			reservation = *j.BookingAllowed
		}
		if !ouvert {
			j.OpensAt, j.ClosesAt, j.BreakStart, j.BreakEnd, j.LastCheckinAt = "", "", "", "", ""
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO opening_hours (station_id, weekday, is_open, opens_at, closes_at, break_start, break_end, last_checkin_at, booking_allowed)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (station_id, weekday) DO UPDATE SET
			  is_open = EXCLUDED.is_open, opens_at = EXCLUDED.opens_at, closes_at = EXCLUDED.closes_at,
			  break_start = EXCLUDED.break_start, break_end = EXCLUDED.break_end,
			  last_checkin_at = EXCLUDED.last_checkin_at, booking_allowed = EXCLUDED.booking_allowed`,
			station.ID, j.Weekday, ouvert, j.OpensAt, j.ClosesAt, j.BreakStart, j.BreakEnd, j.LastCheckinAt, reservation)
		if err != nil {
			ecrireInterne(w, err)
			return
		}
	}
	_, _ = tx.Exec(ctx, `DELETE FROM opening_exceptions WHERE station_id = $1`, station.ID)
	for _, e := range corps.Exceptions {
		ouvert := e.IsOpen != nil && *e.IsOpen
		o, f := e.OpensAt, e.ClosesAt
		if !ouvert {
			o, f = "", ""
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO opening_exceptions (station_id, day, is_open, opens_at, closes_at, label)
			VALUES ($1, $2, $3, $4, $5, $6)`, station.ID, e.Day, ouvert, o, f, strings.TrimSpace(e.Label))
		if err != nil {
			ecrireInterne(w, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	jours, exceptions, err := a.lireSemaine(ctx, station.ID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	a.journaliser(ctx, &station.OrganizationID, &station.ID, &sess.UserID, "station.opening_hours",
		map[string]interface{}{"days": len(corps.Days), "exceptions": len(corps.Exceptions)})
	ecrireJSON(w, http.StatusOK, map[string]interface{}{
		"days": jours, "exceptions": exceptions, "message": "Horaires enregistrés.",
	})
}

// --- Véhicules : catégories, usages, couples ----------------------------------

type saisieActivation struct {
	Code      string `json:"code"`
	IsActive  *bool  `json:"is_active"`
	Name      string `json:"name"`
	SortOrder *int   `json:"sort_order"`
}

func (a *App) activerCategories(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur, station *stationEntite) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	corps, ok := decoderCorps[struct {
		Categories []saisieActivation `json:"categories"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	champs := map[string]string{}
	connues := referentielCodes(ctx, a, `vehicle_categories`)
	entries := corps.Categories
	for i, e := range entries {
		code := strings.TrimSpace(e.Code)
		if code == "" {
			champs[fmt.Sprintf("categories[%d]", i)] = "Code de catégorie manquant."
			continue
		}
		if !connues[code] {
			champs[code] = "Catégorie inconnue du référentiel."
		}
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)
	for _, e := range entries {
		actif := e.IsActive == nil || *e.IsActive
		_, _ = tx.Exec(ctx, `
			INSERT INTO station_vehicle_categories (station_id, category_code, is_active)
			VALUES ($1, $2, $3)
			ON CONFLICT (station_id, category_code) DO UPDATE SET is_active = EXCLUDED.is_active`,
			station.ID, strings.TrimSpace(e.Code), actif)
	}
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	a.journaliser(ctx, &station.OrganizationID, &station.ID, &sess.UserID, "station.vehicle_categories",
		map[string]interface{}{"count": len(entries)})
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"categories": entries, "message": "Catégories enregistrées."})
}

func (a *App) activerUsages(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur, station *stationEntite) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	corps, ok := decoderCorps[struct {
		Usages []saisieActivation `json:"usages"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	champs := map[string]string{}
	connues := referentielCodes(ctx, a, `vehicle_usages`)
	for i, e := range corps.Usages {
		code := strings.TrimSpace(e.Code)
		if code == "" {
			champs[fmt.Sprintf("usages[%d]", i)] = "Code d'usage manquant."
			continue
		}
		if !connues[code] {
			champs[code] = "Usage inconnu du référentiel."
		}
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)
	for _, e := range corps.Usages {
		actif := e.IsActive == nil || *e.IsActive
		_, _ = tx.Exec(ctx, `
			INSERT INTO station_vehicle_usages (station_id, usage_code, is_active)
			VALUES ($1, $2, $3)
			ON CONFLICT (station_id, usage_code) DO UPDATE SET is_active = EXCLUDED.is_active`,
			station.ID, strings.TrimSpace(e.Code), actif)
	}
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	a.journaliser(ctx, &station.OrganizationID, &station.ID, &sess.UserID, "station.vehicle_usages",
		map[string]interface{}{"count": len(corps.Usages)})
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"usages": corps.Usages, "message": "Usages enregistrés."})
}

func (a *App) declarerCouples(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur, station *stationEntite) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	corps, ok := decoderCorps[struct {
		Pairs []struct {
			UsageCode    string `json:"usage_code"`
			CategoryCode string `json:"category_code"`
		} `json:"pairs"`
	}](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	usagesConnus := referentielCodes(ctx, a, `vehicle_usages`)
	catsConnues := referentielCodes(ctx, a, `vehicle_categories`)
	activesU := referentielActifsStation(ctx, a, "station_vehicle_usages", "usage_code", station.ID)
	activesC := referentielActifsStation(ctx, a, "station_vehicle_categories", "category_code", station.ID)
	champs := map[string]string{}
	for i, p := range corps.Pairs {
		u, c := strings.TrimSpace(p.UsageCode), strings.TrimSpace(p.CategoryCode)
		if !usagesConnus[u] {
			champs[fmt.Sprintf("pairs[%d].usage_code", i)] = "Usage inconnu."
			continue
		}
		if !catsConnues[c] {
			champs[fmt.Sprintf("pairs[%d].category_code", i)] = "Catégorie inconnue."
			continue
		}
		if !activesU[u] {
			champs[u] = "Cet usage n'est pas activé pour la station."
			continue
		}
		if !activesC[c] {
			champs[c] = "Cette catégorie n'est pas activée pour la station."
		}
	}
	if len(champs) > 0 {
		ecrireValidation(w, champs)
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, `DELETE FROM station_usage_categories WHERE station_id = $1`, station.ID)
	for _, p := range corps.Pairs {
		_, _ = tx.Exec(ctx, `
			INSERT INTO station_usage_categories (station_id, usage_code, category_code)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			station.ID, strings.TrimSpace(p.UsageCode), strings.TrimSpace(p.CategoryCode))
	}
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, map[string]interface{}{"pairs": corps.Pairs, "message": "Combinaisons enregistrées."})
}

func referentielCodes(ctx context.Context, a *App, table string) map[string]bool {
	out := map[string]bool{}
	rows, err := a.db.Query(ctx, fmt.Sprintf(`SELECT code FROM %s WHERE organization_id IS NULL`, table))
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err == nil {
			out[c] = true
		}
	}
	return out
}

func referentielActifsStation(ctx context.Context, a *App, table, colonne, stationID string) map[string]bool {
	out := map[string]bool{}
	rows, err := a.db.Query(ctx, fmt.Sprintf(`SELECT %s FROM %s WHERE station_id = $1 AND is_active`, colonne, table), stationID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err == nil {
			out[c] = true
		}
	}
	return out
}

var _ = time.Now
