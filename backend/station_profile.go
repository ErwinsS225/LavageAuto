package main

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Fiche d'une station : localisation, capacités physiques, horaires.
//
// L'écran de mise en route pose les mêmes questions qu'un gérant se pose en
// arrivant : où est la station, que peut-elle faire, et quand ouvre-t-elle.
//
// Deux règles tenues ici :
//   * **« on ne sait pas » n'est pas « non ».** Capacités et équipements sont
//     nullables : une case non répondue reste vide, elle ne devient pas « aucun ».
//   * **Les horaires sont validés avant d'être enregistrés.** Une journée qui
//     ferme avant d'ouvrir, une pause hors des heures d'ouverture ou deux
//     exceptions le même jour sont refusées — l'application ne peut pas
//     calculer un statut d'ouverture à partir d'horaires incohérents.
// ---------------------------------------------------------------------------

// champsLocalisationStation : libellé d'API → colonne, pour l'adresse.
var champsLocalisationStation = map[string]string{
	"country":         "country",
	"district":        "district",
	"commune":         "commune",
	"district_area":   "district_area",
	"street":          "street",
	"landmark":        "landmark",
	"latitude":        "latitude",
	"longitude":       "longitude",
	"billing_address": "billing_address",
}

// champsProfilSite : capacité et équipements. Les entiers et booléens sont
// nullables : « pas de réponse » reste une réponse possible.
var champsProfilSite = map[string]string{
	"bays":                  "bays",
	"simultaneous_vehicles": "simultaneous_vehicles",
	"has_vacuum":            "has_vacuum",
	"has_drying":            "has_drying",
	"has_detailing":         "has_detailing",
	"has_waiting_room":      "has_waiting_room",
	"has_storage":           "has_storage",
	"water_source":          "water_source",
	"water_reserve_liters":  "water_reserve_liters",
	"power_supply":          "power_supply",
	"has_generator":         "has_generator",
	"notes":                 "notes",
	"constraints":           "constraints",
}

// typesLavage : ce qu'une station peut pratiquer. Texte libre côté base, liste
// proposée côté écran (une station peut en cumuler plusieurs).
var typesLavage = []string{
	"lavage manuel à l'eau", "haute pression (karcher)", "lavage automatique (portique)",
	"nettoyage intérieur", "aspiration", "polissage / lustrage", "detailing",
	"nettoyage moteur", "vitres / pare-brise",
}

// heureValide : « HH:MM » sur 24 heures.
var heureValide = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// jourSemaine : numéro ISO → nom français (1 = lundi).
var jourSemaine = map[int]string{
	1: "lundi", 2: "mardi", 3: "mercredi", 4: "jeudi", 5: "vendredi", 6: "samedi", 7: "dimanche",
}

// horaireJour : une journée type, telle que l'écran la saisit.
type horaireJour struct {
	Weekday        int    `json:"weekday"`
	IsOpen         bool   `json:"is_open"`
	OpensAt        string `json:"opens_at"`
	ClosesAt       string `json:"closes_at"`
	BreakStart     string `json:"break_start"`
	BreakEnd       string `json:"break_end"`
	LastCheckinAt  string `json:"last_checkin_at"`
	BookingAllowed bool   `json:"booking_allowed"`
}

// exceptionHoraire : une journée particulière (férié, fermeture, horaires
// réduits).
type exceptionHoraire struct {
	Day      string `json:"day"`
	IsOpen   bool   `json:"is_open"`
	OpensAt  string `json:"opens_at"`
	ClosesAt string `json:"closes_at"`
	Label    string `json:"label"`
}

// minutes : « 08:30 » → 510. Une heure invalide renvoie -1.
func minutes(heure string) int {
	heure = strings.TrimSpace(heure)
	if !heureValide.MatchString(heure) {
		return -1
	}
	h, _ := strconv.Atoi(heure[:2])
	m, _ := strconv.Atoi(heure[3:])
	return h*60 + m
}

// validerHoraires : applique les règles du plan (ONB-3.3) et rend le message à
// afficher en cas de refus, indexé par jour.
func validerHoraires(jours []horaireJour) map[string]string {
	erreurs := map[string]string{}
	vus := map[int]bool{}
	for _, jour := range jours {
		nom := jourSemaine[jour.Weekday]
		if nom == "" {
			erreurs["weekday"] = "Jour de la semaine inconnu (1 = lundi … 7 = dimanche)."
			continue
		}
		if vus[jour.Weekday] {
			erreurs[nom] = "Jour présent deux fois."
			continue
		}
		vus[jour.Weekday] = true

		if !jour.IsOpen {
			// Un jour fermé ne porte aucun horaire : les valeurs résiduelles
			// sont ignorées (et nettoyées à l'enregistrement).
			if strings.TrimSpace(jour.OpensAt) != "" || strings.TrimSpace(jour.ClosesAt) != "" {
				erreurs[nom] = "Jour fermé : laissez les horaires vides."
			}
			continue
		}
		debut, fin := minutes(jour.OpensAt), minutes(jour.ClosesAt)
		switch {
		case debut < 0:
			erreurs[nom] = "Heure d'ouverture attendue au format HH:MM."
		case fin < 0:
			erreurs[nom] = "Heure de fermeture attendue au format HH:MM."
		case fin <= debut:
			erreurs[nom] = "La fermeture doit être après l'ouverture."
		}
		if _, ok := erreurs[nom]; ok {
			continue
		}
		// Pause : les deux bornes vont ensemble et doivent tenir dans la plage.
		pauseDebut, pauseFin := minutes(jour.BreakStart), minutes(jour.BreakEnd)
		uneSeule := (strings.TrimSpace(jour.BreakStart) == "") != (strings.TrimSpace(jour.BreakEnd) == "")
		switch {
		case uneSeule:
			erreurs[nom] = "Pause incomplète : renseignez le début et la fin, ou laissez les deux vides."
		case pauseDebut >= 0 && pauseFin >= 0:
			if pauseDebut < 0 || pauseFin < 0 {
				erreurs[nom] = "Pause attendue au format HH:MM."
			} else if pauseFin <= pauseDebut {
				erreurs[nom] = "La fin de la pause doit être après son début."
			} else if pauseDebut < debut || pauseFin > fin {
				erreurs[nom] = "La pause doit être comprise dans les heures d'ouverture."
			}
		}
		if _, ok := erreurs[nom]; ok {
			continue
		}
		if dernier := minutes(jour.LastCheckinAt); strings.TrimSpace(jour.LastCheckinAt) != "" {
			if dernier < 0 {
				erreurs[nom] = "Dernière réception attendue au format HH:MM."
			} else if dernier < debut || dernier > fin {
				erreurs[nom] = "La dernière réception doit être comprise dans les heures d'ouverture."
			}
		}
	}
	return erreurs
}

// validerExceptions : une date valide, des horaires cohérents, une seule
// exception par jour (deux règles le même jour seraient contradictoires).
func validerExceptions(exceptions []exceptionHoraire) map[string]string {
	erreurs := map[string]string{}
	vus := map[string]bool{}
	for _, exception := range exceptions {
		jour := strings.TrimSpace(exception.Day)
		if _, err := time.Parse("2006-01-02", jour); err != nil {
			erreurs[jour] = "Date attendue au format AAAA-MM-JJ."
			continue
		}
		if vus[jour] {
			erreurs[jour] = "Deux règles pour la même journée."
			continue
		}
		vus[jour] = true
		if !exception.IsOpen {
			continue
		}
		debut, fin := minutes(exception.OpensAt), minutes(exception.ClosesAt)
		switch {
		case debut < 0 || fin < 0:
			erreurs[jour] = "Horaires attendus au format HH:MM."
		case fin <= debut:
			erreurs[jour] = "La fermeture doit être après l'ouverture."
		}
	}
	return erreurs
}

// ---------------------------------------------------------------------------
// Lecture
// ---------------------------------------------------------------------------

// lireHoraires rend les sept jours d'une station. Un jour jamais renseigné est
// présenté fermé et vide — l'écran affiche donc quelque chose de saisissable —
// mais la base ne contient pas de ligne tant qu'il n'a pas été enregistré.
func (a *App) lireHoraires(ctx context.Context, stationID string) ([]horaireJour, error) {
	jours := make([]horaireJour, 0, 7)
	enBase := map[int]horaireJour{}
	rows, err := a.Pool.Query(ctx, `
		SELECT weekday, is_open, opens_at, closes_at, break_start, break_end,
		       last_checkin_at, booking_allowed
		FROM station_opening_hours WHERE station_id = $1`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var jour horaireJour
		if err := rows.Scan(&jour.Weekday, &jour.IsOpen, &jour.OpensAt, &jour.ClosesAt,
			&jour.BreakStart, &jour.BreakEnd, &jour.LastCheckinAt, &jour.BookingAllowed); err != nil {
			return nil, err
		}
		enBase[jour.Weekday] = jour
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for jour := 1; jour <= 7; jour++ {
		if connu, ok := enBase[jour]; ok {
			jours = append(jours, connu)
			continue
		}
		jours = append(jours, horaireJour{Weekday: jour, BookingAllowed: true})
	}
	return jours, nil
}

// lireExceptions rend les journées particulières à venir (les plus proches
// d'abord), sans limite de quantité arbitraire.
func (a *App) lireExceptions(ctx context.Context, stationID string) ([]map[string]any, error) {
	rows, err := a.Pool.Query(ctx, `
		SELECT id, day::text, is_open, opens_at, closes_at, label
		FROM station_opening_exceptions
		WHERE station_id = $1 AND day >= CURRENT_DATE - INTERVAL '1 day'
		ORDER BY day LIMIT 60`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	liste := []map[string]any{}
	for rows.Next() {
		var id, jour, ouvre, ferme, libelle string
		var ouvert bool
		if err := rows.Scan(&id, &jour, &ouvert, &ouvre, &ferme, &libelle); err != nil {
			return nil, err
		}
		liste = append(liste, map[string]any{
			"id": id, "day": jour, "is_open": ouvert,
			"opens_at": ouvre, "closes_at": ferme, "label": libelle,
		})
	}
	return liste, rows.Err()
}

// lireProfilSite rend les caractéristiques physiques déclarées. Les valeurs
// non renseignées sont rendues `null` : c'est ce que l'écran doit afficher
// (« non renseigné ») plutôt qu'un faux « non ».
func (a *App) lireProfilSite(ctx context.Context, stationID string) (map[string]any, error) {
	profil := map[string]any{
		"wash_types": []string{}, "bays": nil, "simultaneous_vehicles": nil,
		"has_vacuum": nil, "has_drying": nil, "has_detailing": nil, "has_waiting_room": nil,
		"has_storage": nil, "water_source": "", "water_reserve_liters": nil,
		"power_supply": "", "has_generator": nil, "notes": "", "constraints": "",
	}
	var (
		types                                 []string
		bays, simultanes, reserve             *int
		aspiration, sechage, detailing        *bool
		attente, stockage, groupe             *bool
		sourceEau, alimentation, notes, regle string
	)
	err := a.Pool.QueryRow(ctx, `
		SELECT wash_types, bays, simultaneous_vehicles, has_vacuum, has_drying, has_detailing,
		       has_waiting_room, has_storage, water_source, water_reserve_liters,
		       power_supply, has_generator, notes, constraints
		FROM station_site_profile WHERE station_id = $1`, stationID).Scan(
		&types, &bays, &simultanes, &aspiration, &sechage, &detailing,
		&attente, &stockage, &sourceEau, &reserve, &alimentation, &groupe, &notes, &regle)
	if err != nil && !estPasDeLigne(err) {
		return nil, err
	}
	if types == nil {
		types = []string{}
	}
	profil["wash_types"] = types
	if bays != nil {
		profil["bays"] = *bays
	}
	if simultanes != nil {
		profil["simultaneous_vehicles"] = *simultanes
	}
	if aspiration != nil {
		profil["has_vacuum"] = *aspiration
	}
	if sechage != nil {
		profil["has_drying"] = *sechage
	}
	if detailing != nil {
		profil["has_detailing"] = *detailing
	}
	if attente != nil {
		profil["has_waiting_room"] = *attente
	}
	if stockage != nil {
		profil["has_storage"] = *stockage
	}
	if reserve != nil {
		profil["water_reserve_liters"] = *reserve
	}
	if groupe != nil {
		profil["has_generator"] = *groupe
	}
	profil["water_source"] = sourceEau
	profil["power_supply"] = alimentation
	profil["notes"] = notes
	profil["constraints"] = regle
	return profil, nil
}

// ---------------------------------------------------------------------------
// Enregistrement
// ---------------------------------------------------------------------------

// enregistrerHoraires remplace les horaires d'une station et ses exceptions,
// en une transaction : soit tout est enregistré, soit rien (jamais sept jours
// sur sept moins un).
func (a *App) enregistrerHoraires(ctx context.Context, stationID, auteurID string,
	jours []horaireJour, exceptions []exceptionHoraire) error {

	tx, err := a.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, jour := range jours {
		ouvre, ferme := "", ""
		pauseDebut, pauseFin, derniereReception := "", "", ""
		if jour.IsOpen {
			ouvre, ferme = strings.TrimSpace(jour.OpensAt), strings.TrimSpace(jour.ClosesAt)
			pauseDebut, pauseFin = strings.TrimSpace(jour.BreakStart), strings.TrimSpace(jour.BreakEnd)
			derniereReception = strings.TrimSpace(jour.LastCheckinAt)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO station_opening_hours
				(station_id, weekday, is_open, opens_at, closes_at, break_start, break_end,
				 last_checkin_at, booking_allowed)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (station_id, weekday) DO UPDATE SET
				is_open = EXCLUDED.is_open, opens_at = EXCLUDED.opens_at,
				closes_at = EXCLUDED.closes_at, break_start = EXCLUDED.break_start,
				break_end = EXCLUDED.break_end, last_checkin_at = EXCLUDED.last_checkin_at,
				booking_allowed = EXCLUDED.booking_allowed`,
			stationID, jour.Weekday, jour.IsOpen, ouvre, ferme,
			pauseDebut, pauseFin, derniereReception, jour.BookingAllowed); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Les exceptions sont remplacées à part : elles ne concernent pas les sept
	// jours et peuvent être nombreuses.
	if _, err := a.Pool.Exec(ctx, `DELETE FROM station_opening_exceptions WHERE station_id = $1`, stationID); err != nil {
		return err
	}
	for _, exception := range exceptions {
		ouvre, ferme := "", ""
		if exception.IsOpen {
			ouvre, ferme = strings.TrimSpace(exception.OpensAt), strings.TrimSpace(exception.ClosesAt)
		}
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO station_opening_exceptions (id, station_id, day, is_open, opens_at, closes_at, label)
			VALUES ($1,$2,$3::date,$4,$5,$6,$7)`,
			newID("exc"), stationID, strings.TrimSpace(exception.Day), exception.IsOpen,
			ouvre, ferme, strings.TrimSpace(exception.Label)); err != nil {
			return err
		}
	}
	return nil
}

// enregistrerProfilSite écrit la capacité et les équipements. Une valeur
// absente du corps de requête n'est pas touchée ; une valeur explicitement
// nulle remet le champ à « non renseigné ».
func (a *App) enregistrerProfilSite(ctx context.Context, stationID, auteurID string, corps map[string]any) error {
	types := []string{}
	if brut, ok := corps["wash_types"].([]any); ok {
		for _, item := range brut {
			if valeur, ok := item.(string); ok && strings.TrimSpace(valeur) != "" {
				types = append(types, strings.TrimSpace(valeur))
			}
		}
	}

	colonnes := []string{"station_id", "wash_types", "updated_by"}
	placeholders := []string{"$1", "$2", "$3"}
	args := []any{stationID, types, auteurID}
	index := 4
	for champ, colonne := range champsProfilSite {
		valeur, present := corps[champ]
		if !present {
			continue
		}
		args = append(args, normaliserValeurSite(champ, valeur))
		colonnes = append(colonnes, colonne)
		placeholders = append(placeholders, "$"+strconv.Itoa(index))
		index++
	}

	// ON CONFLICT : la ligne de profil est créée au premier enregistrement,
	// puis complétée champ par champ (un écran n'efface pas ce qu'il n'affiche pas).
	requete := `INSERT INTO station_site_profile (` + strings.Join(colonnes, ", ") + `)
		VALUES (` + strings.Join(placeholders, ", ") + `)
		ON CONFLICT (station_id) DO UPDATE SET `
	mises := []string{"updated_at = now()", "updated_by = EXCLUDED.updated_by"}
	if len(colonnes) > 3 {
		for _, colonne := range colonnes[3:] {
			mises = append(mises, colonne+" = EXCLUDED."+colonne)
		}
	}
	if _, contientTypes := corps["wash_types"]; contientTypes {
		mises = append(mises, "wash_types = EXCLUDED.wash_types")
	}
	requete += strings.Join(mises, ", ")

	_, err := a.Pool.Exec(ctx, requete, args...)
	return err
}

// normaliserValeurSite : convertit la valeur reçue dans le type de la colonne.
// Une valeur nulle signifie « non renseigné » et remet le champ à NULL.
func normaliserValeurSite(champ string, valeur any) any {
	if valeur == nil {
		return nil
	}
	switch champ {
	case "bays", "simultaneous_vehicles", "water_reserve_liters":
		nombre, ok := valeur.(float64)
		if !ok || nombre < 0 {
			return nil
		}
		return int(nombre)
	case "has_vacuum", "has_drying", "has_detailing", "has_waiting_room", "has_storage", "has_generator":
		booleen, ok := valeur.(bool)
		if !ok {
			return nil
		}
		return booleen
	default:
		return toText(valeur)
	}
}

// estPasDeLigne : aucune ligne de profil pour cette station (état normal d'une
// station qui vient d'être créée).
func estPasDeLigne(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no rows in result set")
}
