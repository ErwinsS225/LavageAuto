package main

import (
	"context"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// « Où en est cette station ? » — le bloc que lisent le récapitulatif de mise
// en route (ONB-8.1) et le tableau de bord (ONB-8.4).
//
// Un seul calcul, deux lecteurs : le même bloc alimente l'écran qui précède
// l'activation et le tableau de bord du jour. Deux calculs séparés auraient fini
// par se contredire — l'un disant « catalogue prêt », l'autre « 17 prestations
// à tarifer » — et l'exploitant ne saurait plus lequel croire.
//
// Trois règles tenues par ce fichier :
//
//  1. **Aucune donnée inventée.** Tout ce qui est affiché est compté en base,
//     ou explicitement constaté absent (`orders_total = 0`, « Aucune commande
//     enregistrée »). Un montant n'apparaît jamais « à titre d'exemple ».
//  2. **Le rôle décide de la lecture.** Les blocs d'administration ne sont
//     servis qu'à un profil qui les administre ; un caissier reçoit ce dont il
//     a besoin pour encaisser, pas la configuration de l'entreprise.
//  3. **Le temps est celui de la station.** « Ouvert maintenant » se calcule
//     dans le fuseau de la station, pas dans celui du serveur.
// ---------------------------------------------------------------------------

// isoDuJour : le numéro ISO du jour de la semaine (dimanche = 7), comme la
// colonne `station_opening_hours.weekday`. `time.Weekday` compte, lui, à partir
// de dimanche = 0 : sans conversion, les horaires du dimanche ne seraient
// jamais trouvés.
func isoDuJour(t time.Time) int {
	if t.Weekday() == time.Sunday {
		return 7
	}
	return int(t.Weekday())
}

// coordonneesStation : ce qu'une station dit d'elle-même (et rien de plus).
type coordonneesStation struct {
	ID        string
	Nom       string
	Code      string
	Ville     string
	Commune   string
	Quartier  string
	Rue       string
	Repere    string
	Telephone string
	Timezone  string
	Devise    string
	Latitude  *float64
	Longitude *float64
	IsActive  bool
	CreeeLe   string
}

// coordonnees lit l'identité d'une station, dans l'entreprise du compte.
func (a *App) coordonnees(ctx context.Context, q lecteur, orgID, stationID string) (coordonneesStation, error) {
	var s coordonneesStation
	var latitude, longitude *float64
	err := q.QueryRow(ctx, `
		SELECT id, name, COALESCE(code, ''), COALESCE(city, ''), COALESCE(commune, ''),
		       COALESCE(district_area, ''), COALESCE(street, ''), COALESCE(landmark, ''),
		       COALESCE(phone, ''), COALESCE(NULLIF(timezone, ''), 'Africa/Abidjan'),
		       COALESCE(NULLIF(currency, ''), 'XOF'), latitude::float8, longitude::float8,
		       is_active, created_at::text
		FROM stations
		WHERE id = $1 AND organization_id = $2`, stationID, orgID).
		Scan(&s.ID, &s.Nom, &s.Code, &s.Ville, &s.Commune, &s.Quartier, &s.Rue, &s.Repere,
			&s.Telephone, &s.Timezone, &s.Devise, &latitude, &longitude, &s.IsActive, &s.CreeeLe)
	s.Latitude, s.Longitude = latitude, longitude
	return s, err
}

// libelleLocalisation : « Cocody · Angré (derrière la pharmacie) · Abidjan ».
// C'est la phrase que l'exploitant reconnaît — et celle qu'un reçu peut porter.
func libelleLocalisation(s coordonneesStation) string {
	morceaux := []string{}
	for _, valeur := range []string{s.Commune, s.Quartier} {
		if v := strings.TrimSpace(valeur); v != "" {
			morceaux = append(morceaux, v)
		}
	}
	if v := strings.TrimSpace(s.Repere); v != "" {
		morceaux = append(morceaux, "("+v+")")
	}
	for _, valeur := range []string{s.Rue, s.Ville} {
		if v := strings.TrimSpace(valeur); v != "" {
			morceaux = append(morceaux, v)
		}
	}
	return strings.Join(morceaux, " · ")
}

// localisationDetaillee : la localisation, morceau par morceau — pour que
// l'écran puisse montrer ce qui manque plutôt qu'une phrase tronquée.
func localisationDetaillee(s coordonneesStation) map[string]any {
	manquants := []string{}
	if strings.TrimSpace(s.Commune) == "" {
		manquants = append(manquants, "commune")
	}
	if strings.TrimSpace(s.Quartier) == "" {
		manquants = append(manquants, "quartier")
	}
	if strings.TrimSpace(s.Repere) == "" {
		manquants = append(manquants, "repère")
	}
	return map[string]any{
		"city":       s.Ville,
		"commune":    s.Commune,
		"district":   s.Quartier,
		"street":     s.Rue,
		"landmark":   s.Repere,
		"latitude":   s.Latitude,
		"longitude":  s.Longitude,
		"geolocated": s.Latitude != nil && s.Longitude != nil,
		"label":      libelleLocalisation(s),
		"missing":    manquants,
	}
}

// fuseauStation : le fuseau de la station, avec le repli du produit (Abidjan
// ne change pas d'heure : le décalage est stable toute l'année).
func fuseauStation(timezone string) *time.Location {
	nom := strings.TrimSpace(timezone)
	if nom == "" {
		nom = "Africa/Abidjan"
	}
	loc, err := time.LoadLocation(nom)
	if err != nil {
		loc = time.UTC
	}
	return loc
}

// etatOuverture : le statut d'ouverture **calculé**, jamais deviné.
//
// Les heures sont des chaînes « HH:MM » à zéro initial : elles se comparent
// directement. Une journée déclarée ouverte mais sans heures ne compte pas
// comme une journée d'ouverture — c'est un réglage à finir. Une fermeture
// antérieure à l'ouverture décrit une nuit : la plage appartient à la journée.
type etatOuverture struct {
	Jour               int    `json:"weekday"`
	JourLibelle        string `json:"day_label"`
	Date               string `json:"date"`
	Declare            bool   `json:"declared"`
	OuvertCeJour       bool   `json:"open_today"`
	OuvreA             string `json:"opens_at"`
	FermeA             string `json:"closes_at"`
	PauseDebut         string `json:"break_start"`
	PauseFin           string `json:"break_end"`
	DernierDepot       string `json:"last_checkin_at"`
	Reservation        bool   `json:"booking_allowed"`
	Ouvert             bool   `json:"is_open_now"`
	Etat               string `json:"state"`
	Libelle            string `json:"label"`
	ProchainChangement string `json:"next_change_at"`
}

// etatOuvertureStation calcule le statut d'ouverture d'une journée déclarée.
func etatOuvertureStation(jour horaireJour, declare bool, maintenant time.Time) etatOuverture {
	heure := maintenant.Format("15:04")
	etat := etatOuverture{
		Jour:         jour.Weekday,
		JourLibelle:  jourSemaine[jour.Weekday],
		Date:         maintenant.Format("2006-01-02"),
		Declare:      declare,
		OuvertCeJour: declare && jour.IsOpen,
		OuvreA:       strings.TrimSpace(jour.OpensAt),
		FermeA:       strings.TrimSpace(jour.ClosesAt),
		PauseDebut:   strings.TrimSpace(jour.BreakStart),
		PauseFin:     strings.TrimSpace(jour.BreakEnd),
		DernierDepot: strings.TrimSpace(jour.LastCheckinAt),
		Reservation:  jour.BookingAllowed,
	}

	switch {
	case !declare || !jour.IsOpen:
		etat.Etat = "closed_today"
		etat.Libelle = "Fermée aujourd'hui (" + etat.JourLibelle + ")"
	case etat.OuvreA == "" || etat.FermeA == "":
		etat.Etat = "hours_missing"
		etat.Libelle = "Journée déclarée ouverte, sans heures saisies : elle ne compte pas comme une journée d'ouverture"
	case etat.PauseDebut != "" && etat.PauseFin != "" && heure >= etat.PauseDebut && heure < etat.PauseFin:
		etat.Etat = "break"
		etat.Libelle = "En pause jusqu'à " + etat.PauseFin
		etat.ProchainChangement = etat.PauseFin
	case etat.FermeA > etat.OuvreA: // journée ordinaire
		switch {
		case heure < etat.OuvreA:
			etat.Etat = "before_open"
			etat.Libelle = "Ouvre aujourd'hui à " + etat.OuvreA
			etat.ProchainChangement = etat.OuvreA
		case heure >= etat.FermeA:
			etat.Etat = "after_close"
			etat.Libelle = "Fermée — la journée s'est terminée à " + etat.FermeA
		default:
			etat.Ouvert = true
			etat.Etat = "open"
			etat.Libelle = "Ouverte jusqu'à " + etat.FermeA
			etat.ProchainChangement = etat.FermeA
		}
	default: // fermeture après minuit : la nuit appartient à la journée ouverte
		if heure >= etat.OuvreA || heure < etat.FermeA {
			etat.Ouvert = true
			etat.Etat = "open"
			etat.Libelle = "Ouverte (jusqu'à " + etat.FermeA + " le lendemain)"
			etat.ProchainChangement = etat.FermeA
		} else {
			etat.Etat = "before_open"
			etat.Libelle = "Ouvre ce soir à " + etat.OuvreA
			etat.ProchainChangement = etat.OuvreA
		}
	}
	return etat
}

// horairesStation : les sept jours, lus depuis la base. Une journée jamais
// enregistrée reste absente — elle n'est pas « fermée par défaut », elle n'est
// pas décrite.
func (a *App) horairesStation(ctx context.Context, q lecteur, stationID string) (map[int]horaireJour, error) {
	jours := map[int]horaireJour{}
	if stationID == "" {
		return jours, nil
	}
	lignes, err := q.Query(ctx, `
		SELECT weekday, is_open, opens_at, closes_at, break_start, break_end,
		       last_checkin_at, booking_allowed
		FROM station_opening_hours WHERE station_id = $1`, stationID)
	if err != nil {
		return jours, err
	}
	defer lignes.Close()
	for lignes.Next() {
		var jour horaireJour
		if err := lignes.Scan(&jour.Weekday, &jour.IsOpen, &jour.OpensAt, &jour.ClosesAt,
			&jour.BreakStart, &jour.BreakEnd, &jour.LastCheckinAt, &jour.BookingAllowed); err != nil {
			continue
		}
		jours[jour.Weekday] = jour
	}
	return jours, lignes.Err()
}

// comptagesStation : ce qui existe réellement pour cette station. Chaque
// nombre est compté en base — aucun n'est une estimation.
type comptagesStation struct {
	CategoriesActives      int
	UsagesActifs           int
	CouplesActifs          int
	Prestations            int
	PrestationsVendables   int
	PrestationsTarifees    int
	PrestationsProposables int
	Options                int
	ReglesPrix             int
	MoyensActifs           int
	Equipe                 int
	Equipements            int
	EquipementsImmobilises int
	ArticlesStock          int
	ArticlesNonComptes     int
	ArticlesRupture        int
	ArticlesSeuil          int
	ArticlesPerimes        int
	CapacitesDeclarees     int
	Localisation           int
}

// comptagesMiseEnRoute lit tous les comptages en une requête : deux lectures
// pourraient voir deux états différents, et l'écran afficherait alors un
// catalogue « prêt » à côté d'un tarif « manquant ».
func (a *App) comptagesMiseEnRoute(ctx context.Context, q lecteur, orgID, stationID string) (comptagesStation, error) {
	var c comptagesStation
	prixConnu := `NOT s.is_free AND NOT s.is_quote_only AND NOT (` +
		conditionPrestationVendableSansPrix(
			`COALESCE(st.settings ->> 'allow_generic_price', 'true') <> 'false'`) + `)`
	proposable := `EXISTS (
		         SELECT 1 FROM station_vehicle_categories c
		         WHERE c.station_id = s.station_id AND c.is_active
		           AND (NOT EXISTS (SELECT 1 FROM service_compatibilities k
		                            WHERE k.service_id = s.id AND k.is_active)
		                OR EXISTS (SELECT 1 FROM service_compatibilities k
		                           WHERE k.service_id = s.id AND k.is_active
		                             AND k.category_code = c.code)))`
	err := q.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM station_vehicle_categories WHERE station_id = $1 AND is_active),
		  (SELECT count(*) FROM station_vehicle_usages WHERE station_id = $1 AND is_active),
		  (SELECT count(*) FROM station_vehicle_usage_categories WHERE station_id = $1 AND is_active),
		  (SELECT count(*) FROM services s WHERE s.organization_id = $2 AND s.is_active),
		  (SELECT count(*) FROM services s WHERE s.organization_id = $2 AND s.is_active
		     AND NOT s.is_free AND NOT s.is_quote_only),
		  (SELECT count(*) FROM services s LEFT JOIN stations st ON st.id = s.station_id
		     WHERE s.organization_id = $2 AND s.is_active AND `+prixConnu+`),
		  (SELECT count(*) FROM services s LEFT JOIN stations st ON st.id = s.station_id
		     WHERE s.organization_id = $2 AND s.is_active AND `+prixConnu+` AND `+proposable+`),
		  (SELECT count(*) FROM service_options WHERE organization_id = $2 AND is_active),
		  (SELECT count(*) FROM price_rules WHERE station_id = $1 AND is_active
		     AND (valid_from IS NULL OR valid_from <= CURRENT_DATE)
		     AND (valid_to IS NULL OR valid_to >= CURRENT_DATE)),
		  (SELECT count(*) FROM payment_methods WHERE station_id = $1 AND is_active),
		  (SELECT count(*) FROM users WHERE organization_id = $2 AND is_active AND role <> 'superadmin'),
		  (SELECT count(*) FROM equipments WHERE station_id = $1 AND is_active),
		  (SELECT count(*) FROM equipments WHERE station_id = $1 AND is_active
		     AND state IN ('a_reparer', 'hors_service')),
		  (SELECT count(*) FROM stock_items WHERE station_id = $1 AND is_active),
		  (SELECT count(*) FROM stock_items WHERE station_id = $1 AND is_active AND NOT quantity_known),
		  (SELECT count(*) FROM stock_items WHERE station_id = $1 AND is_active
		     AND quantity_known AND quantity <= 0),
		  (SELECT count(*) FROM stock_items WHERE station_id = $1 AND is_active
		     AND quantity_known AND min_quantity IS NOT NULL AND quantity <= min_quantity),
		  (SELECT count(*) FROM stock_items WHERE station_id = $1 AND is_active
		     AND expires_on IS NOT NULL AND expires_on < CURRENT_DATE),
		  (SELECT count(*) FROM station_site_profile p
		     WHERE p.station_id = $1 AND (
		       cardinality(p.wash_types) > 0
		       OR p.bays IS NOT NULL OR p.simultaneous_vehicles IS NOT NULL
		       OR p.has_vacuum IS NOT NULL OR p.has_drying IS NOT NULL
		       OR p.has_detailing IS NOT NULL OR p.has_waiting_room IS NOT NULL
		       OR p.has_storage IS NOT NULL OR p.has_generator IS NOT NULL
		       OR btrim(p.water_source) <> '' OR p.water_reserve_liters IS NOT NULL
		       OR btrim(p.power_supply) <> '' OR btrim(p.notes) <> ''
		       OR btrim(p.constraints) <> '')),
		  (SELECT count(*) FROM stations s WHERE s.id = $1 AND (
		       btrim(s.commune) <> '' OR btrim(s.district_area) <> '' OR btrim(s.street) <> ''
		       OR btrim(s.landmark) <> '' OR s.latitude IS NOT NULL OR s.longitude IS NOT NULL))
	`, stationID, orgID).Scan(
		&c.CategoriesActives, &c.UsagesActifs, &c.CouplesActifs,
		&c.Prestations, &c.PrestationsVendables, &c.PrestationsTarifees, &c.PrestationsProposables,
		&c.Options, &c.ReglesPrix, &c.MoyensActifs, &c.Equipe,
		&c.Equipements, &c.EquipementsImmobilises, &c.ArticlesStock, &c.ArticlesNonComptes,
		&c.ArticlesRupture, &c.ArticlesSeuil, &c.ArticlesPerimes,
		&c.CapacitesDeclarees, &c.Localisation)
	return c, err
}

// activiteStation : ce que la station a réellement produit — sur toute sa vie,
// pas seulement aujourd'hui. Un tableau de bord qui ne connaît que « le jour »
// affiche zéro à une station qui a travaillé hier : c'est faux.
type activiteStation struct {
	Total             int
	Ouvertes          int
	Annulees          int
	PremiereLe        string
	DerniereReference string
	DernierStatut     string
	DernierJour       string
	DernierLe         string
}

// activiteReelle compte les fiches de la station.
func (a *App) activiteReelle(ctx context.Context, q lecteur, orgID, stationID string) (activiteStation, error) {
	var act activiteStation
	var premiere, derniere *time.Time
	err := q.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status NOT IN ('completed', 'canceled')),
		       count(*) FILTER (WHERE status = 'canceled'),
		       min(created_at), max(created_at)
		FROM wash_orders
		WHERE organization_id = $1 AND station_id = $2`, orgID, stationID).
		Scan(&act.Total, &act.Ouvertes, &act.Annulees, &premiere, &derniere)
	if err != nil {
		return act, err
	}
	if premiere != nil {
		act.PremiereLe = premiere.Format(timeLayout)
	}
	if derniere != nil {
		act.DernierLe = derniere.Format(timeLayout)
	}
	var reference, statut, jour string
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(reference, ''), status, COALESCE(business_day::text, '')
		FROM wash_orders
		WHERE organization_id = $1 AND station_id = $2
		ORDER BY created_at DESC LIMIT 1`, orgID, stationID).
		Scan(&reference, &statut, &jour); err == nil {
		act.DerniereReference, act.DernierStatut, act.DernierJour = reference, statut, jour
	}
	return act, nil
}

// reglageFacultatif : un réglage qui n'empêche pas d'ouvrir, mais que
// l'exploitant doit pouvoir voir d'un coup d'œil. `state` vaut `done` ou
// `empty` — jamais « en cours » : ici, rien ne se déclare, tout se constate.
type reglageFacultatif struct {
	Code        string `json:"code"`
	Titre       string `json:"title"`
	Etat        string `json:"state"`
	Detail      string `json:"detail"`
	Ecran       string `json:"screen"`
	Obligatoire bool   `json:"required"`
	Conseil     string `json:"suggestion"`
}

// blocMiseEnRoute compose le bloc « mise en route » d'une station.
//
// `points` vient de `etapesOnboardingAvec` : ce sont les mêmes contrôles que
// ceux de l'activation qui décident de ce qui est prêt. Le bloc ne rejuge donc
// rien — il met en forme, et il nomme ce qui manque.
func (a *App) blocMiseEnRoute(ctx context.Context, q lecteur, user *User, etat string, points []pointOnboarding, stationID string) map[string]any {
	orgID := deref(user.OrganizationID)
	s, err := a.coordonnees(ctx, q, orgID, stationID)
	if err != nil {
		// Une identité de station illisible ne doit pas éteindre l'écran
		// d'arrivée : on dit ce que l'on sait, et le bloc reste utilisable.
		s = coordonneesStation{ID: stationID}
	}

	c, errComptages := a.comptagesMiseEnRoute(ctx, q, orgID, stationID)
	if errComptages != nil {
		c = comptagesStation{}
	}
	act, errActivite := a.activiteReelle(ctx, q, orgID, stationID)
	if errActivite != nil {
		act = activiteStation{}
	}

	maintenant := time.Now().In(fuseauStation(s.Timezone))
	jours, _ := a.horairesStation(ctx, q, stationID)
	iso := isoDuJour(maintenant)
	jour, declare := jours[iso]
	if !declare {
		// Une journée jamais décrite n'a pas de ligne en base : on la décrit
		// par son nom réel plutôt que par un numéro absent (« Fermée
		// aujourd'hui (mardi) » et non « Fermée aujourd'hui () »).
		jour = horaireJour{Weekday: iso}
	}
	ouverture := etatOuvertureStation(jour, declare, maintenant)
	joursOuverts := 0
	for _, j := range jours {
		if j.IsOpen && strings.TrimSpace(j.OpensAt) != "" && strings.TrimSpace(j.ClosesAt) != "" {
			joursOuverts++
		}
	}

	// Prérequis et réglages facultatifs, dans l'ordre du parcours : le bloc
	// garde la numérotation de l'assistant, sinon l'exploitant ne sait plus à
	// quelle étape retourner.
	prerequis := []map[string]any{}
	manquants := []map[string]any{}
	facultatifs := []reglageFacultatif{}
	for _, point := range points {
		entree := map[string]any{
			"code": point.Step.Code, "title": point.Step.Title, "done": point.Done,
			"detail": point.Detail, "screen": point.Step.Screen, "hint": point.Step.Hint,
			"required": point.Step.Required, "description": point.Step.Description,
		}
		if point.Step.Required {
			prerequis = append(prerequis, entree)
			if !point.Done {
				manquants = append(manquants, entree)
			}
			continue
		}
		etatReglage := "empty"
		if point.Done {
			etatReglage = "done"
		}
		facultatifs = append(facultatifs, reglageFacultatif{
			Code: point.Step.Code, Titre: point.Step.Title, Etat: etatReglage,
			Detail: point.Detail, Ecran: point.Step.Screen, Conseil: point.Step.Hint,
		})
	}
	// Deux réglages facultatifs vivent dans l'étape « station » (ils ne sont pas
	// des étapes à part entière) : les capacités et la localisation. Ils sont
	// présentés ici comme les autres, parce qu'ils se complètent de la même
	// façon — sans jamais bloquer l'ouverture.
	facultatifs = append(facultatifs,
		reglageFacultatif{
			Code: "capacites", Titre: "Capacités de la station", Etat: etatSelon(c.CapacitesDeclarees),
			Detail: detailCapacites(c), Ecran: "/parametres",
			Conseil: "Pistes, véhicules simultanés, jet, aspirateur, eau, électricité : décrire ses capacités évite de promettre ce que la station ne peut pas tenir.",
		},
		reglageFacultatif{
			Code: "localisation", Titre: "Localisation précise", Etat: etatSelon(c.Localisation),
			Detail: detailLocalisation(s, c), Ecran: "/parametres",
			Conseil: "Commune, quartier, repère : c'est ce que le client reconnaît pour venir, et ce qu'un reçu peut porter.",
		})

	return map[string]any{
		// Ce que ce profil a le droit de lire et de faire. Le front s'en sert
		// pour monter l'écran ; le serveur, pour ne pas servir ce qui n'est pas
		// lisible — les deux lisent la même matrice de permissions.
		"role": user.Role,
		"can": map[string]any{
			"manage_settings":     hasPermission(user, "settings:manage"),
			"manage_organization": hasPermission(user, "organization:manage"),
			"see_amounts":         hasPermission(user, "payment:view"),
			"create_wash":         hasPermission(user, "wash:create"),
		},
		"blocks":    blocsLisiblesPar(user),
		"shortcuts": raccourcisLisiblesPar(user),
		"station": map[string]any{
			"id": s.ID, "name": s.Nom, "code": s.Code, "phone": s.Telephone,
			"city": s.Ville, "locality": libelleLocalisation(s), "location": localisationDetaillee(s),
			"currency": s.Devise, "timezone": s.Timezone, "is_active": s.IsActive, "created_at": s.CreeeLe,
		},
		"hours": map[string]any{
			"today": ouverture, "days_open": joursOuverts, "declared_days": len(jours),
			"label": libelleHoraires(joursOuverts),
		},
		"activation": map[string]any{
			"state": etat, "active": etat == "active", "state_label": libelleEtatOnboarding(etat),
			"can_activate":   len(manquants) == 0 && len(prerequis) > 0,
			"blockers":       manquants,
			"blockers_count": len(manquants),
			"required_count": len(prerequis),
			"required_done":  len(prerequis) - len(manquants),
		},
		"prerequisites":     prerequis,
		"missing":           manquants,
		"optional_settings": facultatifs,
		"catalogue":         comptagesVersJSON(c),
		"activity":          activiteVersJSON(act),
		"readiness":         etatsHonnêtes(etat, c, act),
	}
}

// etatSelon : « fait » ou « vide » — deux états, pas trois.
func etatSelon(nombre int) string {
	if nombre > 0 {
		return "done"
	}
	return "empty"
}

// detailCapacites : ce qui est déclaré, ou ce qui manque — sans flatterie.
func detailCapacites(c comptagesStation) string {
	if c.CapacitesDeclarees == 0 {
		return "Aucune capacité décrite : rien n'est renseigné sur ce que la station peut faire."
	}
	return "Capacités décrites (pistes, véhicules simultanés, équipements, eau, électricité)."
}

// detailLocalisation : la phrase du reçu, ou l'absence de repère.
func detailLocalisation(s coordonneesStation, c comptagesStation) string {
	if c.Localisation == 0 {
		return "Aucune commune, aucun quartier, aucun repère : la station n'est localisée que par sa ville."
	}
	if phrase := libelleLocalisation(s); phrase != "" {
		return phrase
	}
	return "Localisation partiellement renseignée."
}

// libelleHoraires : « 6 jours d'ouverture déclarés ».
func libelleHoraires(jours int) string {
	switch jours {
	case 0:
		return "Aucune journée d'ouverture déclarée"
	case 1:
		return "1 jour d'ouverture déclaré"
	default:
		return itoa(jours) + " jours d'ouverture déclarés"
	}
}

// libelleEtatOnboarding : l'état du parcours, dit en français.
func libelleEtatOnboarding(etat string) string {
	switch etat {
	case "active":
		return "Espace en exploitation"
	case "in_progress":
		return "Mise en route en cours"
	default:
		return "Mise en route non commencée"
	}
}

// comptagesVersJSON : les comptages, nommés comme l'écran les lit. Aucun
// montant ici : un catalogue se décrit en nombre de prestations, pas en francs.
func comptagesVersJSON(c comptagesStation) map[string]any {
	sansPrixUtilisable := c.PrestationsVendables - c.PrestationsProposables
	if sansPrixUtilisable < 0 {
		sansPrixUtilisable = 0
	}
	return map[string]any{
		"categories_active":             c.CategoriesActives,
		"usages_active":                 c.UsagesActifs,
		"usage_categories_active":       c.CouplesActifs,
		"services_active":               c.Prestations,
		"services_sellable":             c.PrestationsVendables,
		"services_priced":               c.PrestationsTarifees,
		"services_sellable_priced":      c.PrestationsProposables,
		"services_without_usable_price": sansPrixUtilisable,
		"options_active":                c.Options,
		"price_rules_active":            c.ReglesPrix,
		"payment_methods_active":        c.MoyensActifs,
		"team_active":                   c.Equipe,
		"equipment_active":              c.Equipements,
		"equipment_out_of_service":      c.EquipementsImmobilises,
		"stock_items_active":            c.ArticlesStock,
		"stock_items_uncounted":         c.ArticlesNonComptes,
		"stock_items_out":               c.ArticlesRupture,
		"stock_items_low":               c.ArticlesSeuil,
		"stock_items_expired":           c.ArticlesPerimes,
	}
}

// activiteVersJSON : ce que la station a fait. Les montants du jour vivent
// dans le rapport (`today`), pas ici : ce bloc décrit l'existence de
// l'activité, pour que l'écran ne confonde jamais « rien » et « zéro ».
func activiteVersJSON(a activiteStation) map[string]any {
	libelle := "Aucune fiche enregistrée"
	if a.DernierStatut != "" {
		libelle = libelleStatut(a.DernierStatut)
	}
	return map[string]any{
		"has_activity":     a.Total > 0,
		"orders_total":     a.Total,
		"orders_open":      a.Ouvertes,
		"orders_canceled":  a.Annulees,
		"first_order_at":   a.PremiereLe,
		"last_order_ref":   a.DerniereReference,
		"last_order_state": a.DernierStatut,
		"last_order_label": libelle,
		"last_order_day":   a.DernierJour,
		"last_order_at":    a.DernierLe,
	}
}

// etatsHonnêtes : les phrases que l'écran d'arrivée peut afficher telles
// quelles. Elles remplacent un « 0 FCFA » qui aurait l'air d'un résultat.
func etatsHonnêtes(etat string, c comptagesStation, act activiteStation) map[string]any {
	phrases := []string{}
	if etat != "active" {
		phrases = append(phrases, "La station n'est pas encore activée : elle ne peut pas encaisser de lavage.")
	}
	if act.Total == 0 {
		phrases = append(phrases, "Aucune commande enregistrée : aucun montant ne peut être affiché tant que la première fiche n'existe pas.")
	} else if act.Ouvertes == 0 {
		phrases = append(phrases, "Toutes les fiches enregistrées sont closes.")
	} else {
		phrases = append(phrases, plural(act.Ouvertes, "fiche encore ouverte", "fiches encore ouvertes")+" à suivre.")
	}
	if c.PrestationsProposables > 0 && c.PrestationsVendables > c.PrestationsProposables {
		phrases = append(phrases, plural(c.PrestationsVendables-c.PrestationsProposables,
			"prestation vendable reste sans prix utilisable", "prestations vendables restent sans prix utilisable")+
			" : elles ne seront pas proposées à la caisse.")
	}
	if c.ArticlesNonComptes > 0 {
		phrases = append(phrases, plural(c.ArticlesNonComptes, "article de stock n'a pas été compté",
			"articles de stock n'ont pas été comptés")+" — ce n'est pas une rupture.")
	}
	return map[string]any{
		"statements":    phrases,
		"catalog_ready": c.CategoriesActives > 0 && c.PrestationsProposables > 0 && c.MoyensActifs > 0,
		"no_orders":     act.Total == 0,
	}
}

// ---------------------------------------------------------------------------
// Ce que chaque rôle peut lire du tableau de bord.
// ---------------------------------------------------------------------------

// sectionAdministration : un bloc d'administration et la permission qui
// l'ouvre. La liste est la même que celle du menu : un profil qui ne peut pas
// ouvrir un écran ne reçoit pas non plus le bloc qui y renvoie.
type sectionAdministration struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	Permission string `json:"permission"`
	Href       string `json:"href"`
	Bloc       string `json:"block"`
}

// blocsAdministration : les blocs d'administration du tableau de bord.
var blocsAdministration = []sectionAdministration{
	{Code: "station", Label: "Réglages de la station", Permission: "settings:view", Href: "/parametres", Bloc: "configuration"},
	{Code: "equipe", Label: "Équipe et rôles", Permission: "employee:view", Href: "/employes", Bloc: "team"},
	// Le catalogue se règle : un profil qui peut le consulter sans pouvoir le
	// modifier n'a rien à faire dans les blocs d'administration — il verrait un
	// écran où chaque bouton le refuse.
	{Code: "catalogue", Label: "Catalogue et tarifs", Permission: "service:manage", Href: "/prestations", Bloc: "catalog"},
	{Code: "abonnement", Label: "Abonnement et limites", Permission: "subscription:view", Href: "/abonnement", Bloc: "plan"},
	{Code: "journal", Label: "Journal des actions", Permission: "audit:view", Href: "/parametres", Bloc: "audit"},
}

// blocsLisiblesPar : les sections que ce profil peut réellement ouvrir.
func blocsLisiblesPar(user *User) []sectionAdministration {
	sections := []sectionAdministration{}
	for _, section := range blocsAdministration {
		if hasPermission(user, section.Permission) {
			sections = append(sections, section)
		}
	}
	return sections
}

// raccourci : un chemin d'exploitation proposé sur l'écran d'arrivée. Un
// raccourci dont le profil n'a pas la permission n'est pas proposé — il ne
// serait pas utilisable, et un bouton qui refuse est un piège.
type raccourci struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	Href       string `json:"href"`
	Permission string `json:"permission"`
	Hint       string `json:"hint"`
}

// raccourcisExploitation : l'ordre du quotidien — encaisser, enregistrer un
// lavage, retrouver un client, suivre la caisse.
var raccourcisExploitation = []raccourci{
	{Code: "nouvelle_fiche", Label: "Enregistrer un lavage", Href: "/lavages", Permission: "wash:create",
		Hint: "La caisse ouvre une fiche : véhicule, prestation, prix du barème."},
	{Code: "fiches", Label: "Fiches et lavages", Href: "/lavages", Permission: "wash:view",
		Hint: "Suivre l'état des véhicules et encaisser les soldes."},
	{Code: "clients", Label: "Clients", Href: "/clients", Permission: "customer:view",
		Hint: "Retrouver un client, son historique et ses fiches ouvertes."},
	{Code: "caisse", Label: "Caisse du jour", Href: "/caisse", Permission: "cash:view",
		Hint: "Ouvrir ou clôturer la caisse, compter les espèces, voir les écarts."},
	{Code: "depenses", Label: "Dépenses", Href: "/depenses", Permission: "expense:view",
		Hint: "Enregistrer une sortie d'argent de la journée."},
	{Code: "rapports", Label: "Rapports", Href: "/rapports", Permission: "report:view",
		Hint: "Chiffre d'affaires, résultats et exports sur une période."},
	{Code: "espace_laveur", Label: "Espace laveur", Href: "/espace-laveur", Permission: "washer:view",
		Hint: "Les fiches affectées, sans montant."},
}

// raccourcisLisiblesPar : les raccourcis que ce profil peut réellement ouvrir.
func raccourcisLisiblesPar(user *User) []raccourci {
	liste := []raccourci{}
	for _, r := range raccourcisExploitation {
		if hasPermission(user, r.Permission) {
			liste = append(liste, r)
		}
	}
	return liste
}
