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
)

// routes_onboarding.go — le parcours de mise en route, entièrement recalculé
// sur les données réelles de la base. Le client n'envoie que des choix
// descriptifs (PATCH /onboarding/steps/{code}) ; l'état « fait » d'une étape
// ne se décrète pas, il se constate. L'activation est une transaction qui
// revérifie chaque prérequis avant d'écrire quoi que ce soit.

// --- Modèle du parcours ------------------------------------------------------

type etapeDef struct {
	code        string
	titre       string
	description string
	obligatoire bool
	ecran       string
	form        string
}

// Treize étapes, dont huit obligatoires (docs/api.md). Les cinq facultatives
// restent atteignables après activation.
var etapesParcours = []etapeDef{
	{"espace", "Votre espace", "L'entreprise et votre compte existent-ils ?", true, "/onboarding", ""},
	{"entreprise", "Identité de l'entreprise", "Nom, téléphone, e-mail : ce que la fiche de caisse affichera.", true, "/settings", "entreprise"},
	{"station", "La station", "Créez la station et déclarez ses capacités réelles (pistes, réserve d'eau).", true, "/onboarding", "station"},
	{"horaires", "Jours et heures d'ouverture", "Au moins une journée ouverte avec des heures complètes.", true, "/onboarding", "horaires"},
	{"categories", "Catégories de véhicules", "Quelles tailles de véhicules vous lavez ?", true, "/onboarding", "vehicules"},
	{"usages", "Usages véhicules", "À quel usage chaque catégorie correspond-elle dans votre activité ?", false, "/onboarding", "usages"},
	{"prestations", "Prestations", "Les services que vous vendez, actifs et proposés à la vente.", true, "/onboarding", "prestations"},
	{"tarifs", "Tarifs", "Chaque prestation vendable doit avoir un prix utilisable.", true, "/onboarding", "tarifs"},
	{"options", "Options", "Suppléments facturés en plus d'une prestation.", false, "/onboarding", "options"},
	{"moyens", "Moyens de paiement", "Espèces, mobile money, carte : ce que la caisse peut encaisser.", true, "/onboarding", "moyens"},
	{"equipe", "Équipe", "Inviter ou déclarer les personnes qui travaillent à la station.", false, "/onboarding", "equipe"},
	{"materiel", "Matériel & stock", "Inventaire de départ pour suivre consommables et pannes.", false, "/onboarding", "materiel"},
	{"commandes", "Règles de commande", "Comment une fiche est créée, assignée, annulée.", false, "/onboarding", "commandes"},
}

func defEtape(code string) *etapeDef {
	for i := range etapesParcours {
		if etapesParcours[i].code == code {
			return &etapesParcours[i]
		}
	}
	return nil
}

// constatEtape : ce que la base dit d'une étape, plus le message champ par
// champ utilisé quand une écriture d'exploitation est bloquée.
func (a *App) constatEtape(ctx context.Context, organizationID string, st *stationEntite, codeEtape string, c constatsCatalogue) (bool, string, string) {
	pres, tarifs, moyens := c.prestations, c.tarifees, c.moyens
	switch codeEtape {
	case "espace":
		if st != nil {
			return true, "Station « " + st.Name + " » rattachée à votre compte.", "Créez d'abord votre station."
		}
		return false, "Aucune station rattachée à cette entreprise.", "Créez d'abord votre station."
	case "entreprise":
		var nom, tel, mail string
		_ = a.db.QueryRow(ctx, `SELECT name, phone, email FROM organizations WHERE id = $1`, organizationID).Scan(&nom, &tel, &mail)
		if strings.TrimSpace(nom) != "" && strings.TrimSpace(tel) != "" && strings.Contains(mail, "@") {
			return true, fmt.Sprintf("« %s », joignable au %s.", nom, tel), ""
		}
		return false, "Nom, téléphone ou e-mail manquant.", "Complétez l'identité de l'entreprise (nom, téléphone, e-mail)."
	case "station":
		if st == nil {
			return false, "Aucune station.", "Créez la station et déclarez ses capacités."
		}
		pistes := nombreDe(st.Site["bays"])
		reserve := nombreDe(st.Site["water_reserve_liters"])
		if pistes != nil && *pistes >= 0 && reserve != nil && *reserve > 0 {
			return true, fmt.Sprintf("%.0f piste(s), réserve de %.0f L.", *pistes, *reserve), ""
		}
		return false, "Capacités non déclarées.", "Déclarez le nombre de pistes et la réserve d'eau de la station."
	case "horaires":
		var nbOuverts int
		var sansHeures int
		if st != nil {
			_ = a.db.QueryRow(ctx, `SELECT COUNT(*) FROM opening_hours WHERE station_id = $1 AND is_open`, st.ID).Scan(&nbOuverts)
			_ = a.db.QueryRow(ctx, `SELECT COUNT(*) FROM opening_hours WHERE station_id = $1 AND is_open AND (opens_at = '' OR closes_at = '')`, st.ID).Scan(&sansHeures)
		}
		if nbOuverts-sansHeures > 0 {
			return true, fmt.Sprintf("%d journée(s) ouverte(s) avec horaires.", nbOuverts-sansHeures), ""
		}
		if nbOuverts > 0 {
			return false, "Journées ouvertes sans heures.", "Précisez les heures d'ouverture et de fermeture."
		}
		return false, "Aucun jour d'ouverture défini.", "Définissez au moins une journée d'ouverture complète."
	case "categories":
				if st == nil {
			return false, "Aucune station.", "Créez d'abord votre station."
		}
if cat2 := a.compterRequete(ctx, `SELECT COUNT(*) FROM station_vehicle_categories WHERE station_id = $1 AND is_active`, st.ID); cat2 > 0 {
			return true, fmt.Sprintf("%d catégorie(s) active(s).", cat2), ""
		}
		return false, "Aucune catégorie active.", "Activez au moins une catégorie de véhicules."
	case "usages":
				if st == nil {
			return false, "Aucune station.", "Créez d'abord votre station."
		}
n := a.compterRequete(ctx, `SELECT COUNT(*) FROM station_vehicle_usages WHERE station_id = $1 AND is_active`, st.ID)
		if n > 0 {
			return true, fmt.Sprintf("%d usage(s) véhicule actif(s).", n), "Reliez usages et catégories si besoin."
		}
		return false, "Aucun usage véhicule déclaré.", "Déclarez les usages de véhicules que vous lavez."
	case "prestations":
		if pres > 0 {
			return true, fmt.Sprintf("%d prestation(s) vendable(s).", pres), ""
		}
		return false, "Aucune prestation active.", "Créez au moins une prestation proposée à la vente."
	case "tarifs":
		if tarifs > 0 && pres > 0 && tarifs >= pres {
			return true, fmt.Sprintf("%d/%d prestations tarifées.", tarifs, pres), ""
		}
		if pres == 0 {
			return false, "Aucune prestation à tarifer.", "Créez d'abord des prestations, puis fixez leurs prix."
		}
		return false, fmt.Sprintf("%d/%d prestations tarifées.", tarifs, pres), "Aucune prestation vendable n'a de prix utilisable : fixez les tarifs."
	case "options":
		if c.options > 0 {
			return true, fmt.Sprintf("%d option(s) active(s).", c.options), ""
		}
		return false, "Aucune option.", "Ajoutez des suppléments (facultatif)."
	case "moyens":
		if moyens > 0 {
			return true, fmt.Sprintf("%d moyen(s) de paiement actif(s).", moyens), ""
		}
		return false, "Aucun moyen de paiement.", "Activez au moins un moyen de paiement (espèces, mobile money…)."
	case "equipe":
				if st == nil {
			return false, "Aucune station.", "Créez d'abord votre station."
		}
n := a.compterRequete(ctx, `SELECT COUNT(*) FROM employees WHERE station_id = $1 AND is_active`, st.ID)
		if n > 0 {
			return true, fmt.Sprintf("%d membre(s) d'équipe.", n), ""
		}
		var inv int
		_ = a.db.QueryRow(ctx, `SELECT COUNT(*) FROM invitations WHERE organization_id = $1 AND status = 'pending'`, organizationID).Scan(&inv)
		if inv > 0 {
			return true, fmt.Sprintf("%d invitation(s) en attente.", inv), ""
		}
		return false, "Équipe non déclarée.", "Invitez ou déclarez votre équipe (facultatif)."
	case "materiel":
				if st == nil {
			return false, "Aucune station.", "Créez d'abord votre station."
		}
n := a.compterRequete(ctx, `SELECT COUNT(*) FROM equipment WHERE station_id = $1 AND is_active`, st.ID)
		s := a.compterRequete(ctx, `SELECT COUNT(*) FROM stock_items WHERE station_id = $1 AND is_active`, st.ID)
		if n+s > 0 {
			return true, fmt.Sprintf("%d équipement(s), %d article(s) de stock.", n, s), ""
		}
		return false, "Matériel non inventorié.", "Inventoriez matériel et stock (facultatif)."
	case "commandes":
		defauts := map[string]interface{}{}
		if st != nil {
			defauts = st.Settings
		}
		n := 0
		for _, c := range []string{"auto_assign", "require_plate", "allow_deferred_payment"} {
			if _, ok := defauts[c]; ok {
				n++
			}
		}
		if n > 0 {
			return true, fmt.Sprintf("%d règle(s) de commande enregistrée(s).", n), ""
		}
		return false, "Réglages de commande par défaut.", "Ajustez les règles de création des fiches (facultatif)."
	}
	return false, "", ""
}

func (a *App) compterRequete(ctx context.Context, requete string, stationID string) int {
	var n int
	if err := a.db.QueryRow(ctx, requete, stationID).Scan(&n); err != nil {
		return 0
	}
	return n
}

// --- Constats agrégés --------------------------------------------------------

type constatsCatalogue struct {
	categories int
	usages     int
	couples    int
	prestations int // actives ET proposées à la vente
	tarifees   int // vendables avec prix utilisable
	options    int
	moyens     int
	equipe     int
	materiel   int
	stock      int
	joursDeclares int
	joursOuverts int
}

func (a *App) compterBase(ctx context.Context, organizationID string, st *stationEntite) constatsCatalogue {
	c := constatsCatalogue{}
	if st == nil {
		return c
	}
	scan := func(requete string, cible *int) {
		var n int
		if err := a.db.QueryRow(ctx, requete, st.ID).Scan(&n); err == nil {
			*cible = n
		}
	}
	scan(`SELECT COUNT(*) FROM station_vehicle_categories WHERE station_id = $1 AND is_active`, &c.categories)
	scan(`SELECT COUNT(*) FROM station_vehicle_usages WHERE station_id = $1 AND is_active`, &c.usages)
	scan(`SELECT COUNT(*) FROM station_usage_categories WHERE station_id = $1`, &c.couples)
	scan(`SELECT COUNT(*) FROM services WHERE station_id = $1 AND is_active AND NOT requires_quote`, &c.prestations)
	scan(`SELECT COUNT(*) FROM services s WHERE s.station_id = $1 AND s.is_active AND NOT s.requires_quote
	      AND (s.is_free OR s.price_xof > 0
	           OR EXISTS (SELECT 1 FROM price_rules pr WHERE pr.service_id = s.id AND pr.is_active AND pr.price_xof > 0))`, &c.tarifees)
	scan(`SELECT COUNT(*) FROM service_options WHERE station_id = $1 AND is_active`, &c.options)
	scan(`SELECT COUNT(*) FROM payment_methods WHERE station_id = $1 AND is_active`, &c.moyens)
	scan(`SELECT COUNT(*) FROM employees WHERE station_id = $1 AND is_active`, &c.equipe)
	scan(`SELECT COUNT(*) FROM equipment WHERE station_id = $1 AND is_active`, &c.materiel)
	scan(`SELECT COUNT(*) FROM stock_items WHERE station_id = $1 AND is_active`, &c.stock)
	scan(`SELECT COUNT(*) FROM opening_hours WHERE station_id = $1`, &c.joursDeclares)
	scan(`SELECT COUNT(*) FROM opening_hours WHERE station_id = $1 AND is_open AND opens_at <> '' AND closes_at <> ''`, &c.joursOuverts)
	return c
}

// --- Calcul de l'état --------------------------------------------------------

const (
	etatEnAttente   = "pending"
	etatEnCours     = "in_progress"
	etatTermine     = "completed"
	etatActif       = "active"
)

func (a *App) etatOnboarding(ctx context.Context, organizationID string) string {
	var etat string
	err := a.db.QueryRow(ctx, `SELECT state FROM onboarding_states WHERE organization_id = $1`, organizationID).Scan(&etat)
	if err != nil {
		return etatEnAttente
	}
	if etat == etatActif {
		return etatActif
	}
	// pending/in_progress/completed : le serveur recalcule à la lecture.
	st, _ := a.lireStationOrganisation(ctx, organizationID)
	c := a.compterBase(ctx, organizationID, st)
	faites := 0
	for _, def := range etapesParcours {
		if !def.obligatoire {
			continue
		}
		done, _, _ := a.constatEtape(ctx, organizationID, st, def.code, c)
		if done {
			faites++
		}
	}
	switch {
	case faites == 0 && st == nil:
		return etatEnAttente
	case faites < 8:
		return etatEnCours
	default:
		return etatTermine
	}
}

type blocManquant struct {
	code        string
	title       string
	hint        string
	screen      string
	description string
	detail      string
}

func (a *App) prerequisOnboarding(ctx context.Context, organizationID string) []blocManquant {
	st, _ := a.lireStationOrganisation(ctx, organizationID)
	c := a.compterBase(ctx, organizationID, st)
	var manquants []blocManquant
	for _, def := range etapesParcours {
		if !def.obligatoire {
			continue
		}
		done, detail, hint := a.constatEtape(ctx, organizationID, st, def.code, c)
		if !done {
			if hint == "" {
				hint = detail
			}
			manquants = append(manquants, blocManquant{
				code: def.code, title: def.titre, hint: hint, screen: def.ecran,
				description: def.description, detail: detail,
			})
		}
	}
	return manquants
}

type renduEtape struct {
	Code        string                 `json:"code"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Required    bool                   `json:"required"`
	Done        bool                   `json:"done"`
	State       string                 `json:"state"`
	Detail      string                 `json:"detail"`
	Screen      string                 `json:"screen"`
	Hint        string                 `json:"hint"`
	Form        string                 `json:"form"`
	Data        map[string]interface{} `json:"data"`
}

type missingAPI struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Hint        string `json:"hint"`
	Screen      string `json:"screen"`
	Description string `json:"description"`
}

// calculerOnboarding construit le payload complet attendu par le front
// (src/types/index.ts → Onboarding). Appelé par /auth/me, GET /onboarding.
func (a *App) calculerOnboarding(ctx context.Context, organizationID string, st *stationEntite) (map[string]interface{}, error) {
	etatDB := etatEnAttente
	var startedAt, activatedAt *time.Time
	_ = a.db.QueryRow(ctx, `SELECT state, started_at, activated_at FROM onboarding_states WHERE organization_id = $1`, organizationID).
		Scan(&etatDB, &startedAt, &activatedAt)
	if st == nil {
		st, _ = a.lireStationOrganisation(ctx, organizationID)
	}
	c := a.compterBase(ctx, organizationID, st)
	etat := etatDB
	if etat != etatActif {
		etat = a.etatCalcule(ctx, organizationID, st, c)
	}

	// Choix descriptifs enregistrés par les formulaires.
	dataBrutes := map[string]map[string]interface{}{}
	if rows, err := a.db.Query(ctx, `SELECT step_code, data FROM onboarding_steps WHERE organization_id = $1`, organizationID); err == nil {
		for rows.Next() {
			var code string
			var body []byte
			if rows.Scan(&code, &body) == nil {
				dataBrutes[code] = decoderObjet(body)
			}
		}
		rows.Close()
	}

	rendu := make([]renduEtape, 0, len(etapesParcours))
	faitesRequises, totalRequises := 0, 0
	manquantes := []missingAPI{}
	for _, def := range etapesParcours {
		done, detail, hint := a.constatEtape(ctx, organizationID, st, def.code, c)
		enCours := false
		if d, ok := dataBrutes[def.code]; ok && len(d) > 0 {
			enCours = true
		}
		etatEtape := "not_started"
		switch {
		case done:
			etatEtape = "completed"
		case enCours:
			etatEtape = "in_progress"
		}
		if def.obligatoire {
			totalRequises++
			if done {
				faitesRequises++
			} else {
				if hint == "" {
					hint = detail
				}
				manquantes = append(manquantes, missingAPI{def.code, def.titre, hint, def.ecran, def.description})
			}
		}
		data := dataBrutes[def.code]
		if data == nil {
			data = map[string]interface{}{}
		}
		rendu = append(rendu, renduEtape{
			Code: def.code, Title: def.titre, Description: def.description, Required: def.obligatoire,
			Done: done, State: etatEtape, Detail: detail, Screen: def.ecran, Hint: hint, Form: def.form, Data: data,
		})
	}
	canActivate := etat != etatActif && faitesRequises == totalRequises
	if etat == etatActif {
		manquantes = []missingAPI{}
	}

	out := map[string]interface{}{
		"organization_id": organizationID,
		"state":           etat,
		"progress": map[string]interface{}{
			"done": faitesRequises, "total": totalRequises,
			"percent":   pourcent(faitesRequises, totalRequises),
			"steps_done": countDone(rendu), "steps_total": len(rendu),
		},
		"steps":       rendu,
		"missing":     manquantes,
		"can_activate": canActivate,
		"started_at":  formatTemps(startedAt),
		"activated_at": formatTemps(activatedAt),
	}
	if org, err := a.lireOrganisation(ctx, organizationID); err == nil && org != nil {
		out["organization"] = map[string]interface{}{
			"id": organizationID, "name": chaineOuVide(org["name"]), "legal_name": chaineOuVide(org["legal_name"]),
			"email": chaineOuVide(org["email"]), "phone": chaineOuVide(org["phone"]),
			"city": chaineOuVide(org["city"]), "country": chaineOuVide(org["country"]),
			"currency": "XOF", "timezone": "Africa/Abidjan", "status": "active",
			"rccm": chaineOuVide(org["rccm"]), "tax_id": chaineOuVide(org["tax_id"]),
		}
	}
	if ab := a.lireActivation(ctx, organizationID); ab != nil {
		out["activated_by"] = ab
	}
	return out, nil
}

func (a *App) etatCalcule(ctx context.Context, organizationID string, st *stationEntite, c constatsCatalogue) string {
	faites := 0
	for _, def := range etapesParcours {
		if !def.obligatoire {
			continue
		}
		done, _, _ := a.constatEtape(ctx, organizationID, st, def.code, c)
		if done {
			faites++
		}
	}
	switch {
	case faites == 0 && st == nil:
		return etatEnAttente
	case faites < 8:
		return etatEnCours
	default:
		return etatTermine
	}
}

func pourcent(a, b int) int {
	if b == 0 {
		return 0
	}
	return a * 100 / b
}

func countDone(r []renduEtape) int {
	n := 0
	for _, e := range r {
		if e.Done {
			n++
		}
	}
	return n
}

func formatTemps(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (a *App) lireActivation(ctx context.Context, organizationID string) map[string]interface{} {
	var activatedBy []byte
	var snapshot []byte
	var at time.Time
	err := a.db.QueryRow(ctx, `SELECT activated_by, snapshot, activated_at FROM onboarding_states WHERE organization_id = $1 AND activated_at IS NOT NULL`, organizationID).
		Scan(&activatedBy, &snapshot, &at)
	if err != nil {
		return nil
	}
	auteur := decoderObjet(activatedBy)
	inst := decoderObjet(snapshot)
	return map[string]interface{}{
		"activated_at": at.UTC().Format(time.RFC3339),
		"activated_by": chaineOuVide(auteur["user_id"]), "activated_by_name": chaineOuVide(auteur["name"]),
		"snapshot": inst,
	}
}

// --- Routes ------------------------------------------------------------------

func (a *App) enregistrerRoutesOnboarding(rt *routeur) {
	rt.ajouter("GET", "/api/v1/onboarding", a.getOnboarding)
	rt.ajouter("GET", "/api/v1/onboarding/summary", a.summaryOnboarding)
	rt.ajouter("PATCH", "/api/v1/onboarding/steps/", a.enregistrerEtape)
	rt.ajouter("POST", "/api/v1/onboarding/activate", a.activerStation)
}

func organisationSession(sess *sessionUtilisateur) (string, bool) {
	if sess.OrganizationID == nil || *sess.OrganizationID == "" {
		return "", false
	}
	return *sess.OrganizationID, true
}

func (a *App) getOnboarding(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	orgID, ok := organisationSession(sess)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "no_organization", "Aucune entreprise rattachée à ce compte.")
		return
	}
	payload, err := a.calculerOnboarding(r.Context(), orgID, nil)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, payload)
}

// setupBloc reconstruit le bloc « mise en route » partagé par /onboarding/summary
// et le tableau de bord : un seul calcul, deux écrans cohérents.
func (a *App) setupBloc(ctx context.Context, sess *sessionUtilisateur, orgID string) (map[string]interface{}, error) {
	st, _ := a.lireStationOrganisation(ctx, orgID)
	c := a.compterBase(ctx, orgID, st)
	etat := a.etatOnboarding(ctx, orgID)

	location := map[string]interface{}{}
	missingGeo := []string{}
	label := ""
	if st != nil {
		lat, lng := "", ""
		if st.Latitude != nil {
			lat = *st.Latitude
		}
		if st.Longitude != nil {
			lng = *st.Longitude
		}
		villes := []struct{ cle, val string }{
			{"city", st.City}, {"commune", st.Commune}, {"district", st.District},
			{"street", st.Street}, {"landmark", st.Landmark},
		}
		for _, v := range villes {
			if strings.TrimSpace(v.val) == "" {
				missingGeo = append(missingGeo, v.cle)
			}
		}
		pieces := []string{}
		for _, v := range []string{st.Landmark, st.Street, st.Commune, st.City} {
			if strings.TrimSpace(v) != "" {
				pieces = append(pieces, strings.TrimSpace(v))
			}
		}
		label = strings.Join(pieces, ", ")
		location = map[string]interface{}{
			"city": st.City, "commune": st.Commune, "district": st.District, "street": st.Street,
			"landmark": st.Landmark, "latitude": lat, "longitude": lng,
			"geolocated": lat != "" && lng != "", "label": label, "missing": missingGeo,
		}
	} else {
		location = map[string]interface{}{"geolocated": false, "missing": []string{"city", "commune", "street", "landmark"}}
	}

	stationMap := map[string]interface{}{}
	if st != nil {
		stationMap = map[string]interface{}{
			"id": st.ID, "name": st.Name, "code": st.Code, "phone": st.Phone, "city": st.City,
			"locality": st.Commune, "location": location, "currency": st.Currency, "timezone": st.Timezone,
			"is_active": st.IsActive, "created_at": st.CreatedAt.UTC().Format(time.RFC3339),
		}
	}

	// Ouverture du jour, calculée dans le fuseau de la station.
	aujourdhui := map[string]interface{}{"state": "closed_today", "label": "Aucune station", "declared": false, "open_today": false}
	if st != nil {
		loc, err := time.LoadLocation(st.Timezone)
		if err != nil {
			loc = time.FixedZone("UTC", 0)
		}
		now := time.Now().In(loc)
		wd := int(now.Weekday())
		if wd == 0 {
			wd = 7
		}
		var ouvert bool
		var o, f, pauseD, pauseF, dernier string
		var resaOK bool
		err = a.db.QueryRow(ctx, `SELECT is_open, opens_at, closes_at, break_start, break_end, last_checkin_at, booking_allowed
			FROM opening_hours WHERE station_id = $1 AND weekday = $2`, st.ID, wd).Scan(&ouvert, &o, &f, &pauseD, &pauseF, &dernier, &resaOK)
		state := "closed_today"
		labelJour := "Fermée aujourd'hui"
		if err == nil && ouvert {
			if o == "" || f == "" {
				state = "hours_missing"
				labelJour = "Ouverte mais sans horaires : à compléter"
			} else {
				minNow := now.Hour()*60 + now.Minute()
				oMin, fMin := minutesDepuisMinuit(o), minutesDepuisMinuit(f)
				switch {
				case minNow < oMin:
					state = "before_open"
					labelJour = "Ouvre à " + o
				case minNow >= fMin:
					state = "after_close"
					labelJour = "Fermée depuis " + f
				case pauseD != "" && pauseF != "" && minNow >= minutesDepuisMinuit(pauseD) && minNow < minutesDepuisMinuit(pauseF):
					state = "break"
					labelJour = "Pause jusqu'à " + pauseF
				default:
					state = "open"
					labelJour = "Ouverte (" + o + " – " + f + ")"
				}
			}
		} else if err == nil {
			state = "closed_today"
		}
		if state != "closed_today" && state != "hours_missing" {
			aujourdhui["open_today"] = true
		}
		aujourdhui = map[string]interface{}{
			"weekday": wd, "day_label": nomJour(wd), "date": now.Format("2006-01-02"),
			"declared": err == nil, "open_today": aujourdhui["open_today"],
			"opens_at": o, "closes_at": f, "break_start": pauseD, "break_end": pauseF,
			"last_checkin_at": dernier, "booking_allowed": resaOK,
			"is_open_now": state == "open", "state": state, "label": labelJour, "next_change_at": "",
		}
	}

	prerequis := []map[string]interface{}{}
	manquants := []map[string]interface{}{}
	for _, def := range etapesParcours {
		if !def.obligatoire {
			continue
		}
		done, detail, hint := a.constatEtape(ctx, orgID, st, def.code, c)
		if hint == "" {
			hint = detail
		}
		ligne := map[string]interface{}{
			"code": def.code, "title": def.titre, "description": def.description,
			"detail": detail, "hint": hint, "screen": def.ecran, "required": true, "done": done,
		}
		prerequis = append(prerequis, ligne)
		if !done {
			manquants = append(manquants, ligne)
		}
	}

	facultatifs := []map[string]interface{}{}
	for _, def := range etapesParcours {
		if def.obligatoire {
			continue
		}
		done, detail, _ := a.constatEtape(ctx, orgID, st, def.code, c)
		etatReglage := "empty"
		if done {
			etatReglage = "done"
		}
		facultatifs = append(facultatifs, map[string]interface{}{
			"code": def.code, "title": def.titre, "state": etatReglage, "detail": detail,
			"screen": def.ecran, "required": false, "suggestion": def.description,
		})
	}

	blocs := []map[string]interface{}{}
	raccourcis := []map[string]interface{}{}
	peutGerer := sess.aPermission("settings:manage")
	peutOrg := sess.aPermission("organization:manage")
	for _, b := range []struct{ code, label, perm, href string }{
		{"parametres", "Paramètres de la station", "settings:view", "/settings"},
		{"catalogue", "Catalogue & tarifs", "service:view", "/services"},
		{"equipe", "Équipe", "employee:view", "/employees"},
		{"abonnement", "Abonnement", "subscription:view", "/subscription"},
	} {
		if sess.aPermission(b.perm) {
			blocs = append(blocs, map[string]interface{}{"code": b.code, "label": b.label, "permission": b.perm, "href": b.href, "block": ""})
		}
	}
	for _, s := range []struct{ code, label, href, perm, hint string }{
		{"nouvelle_fiche", "Nouvelle fiche de lavage", "/wash-orders/new", "wash:create", "Enregistrer un client et son véhicule"},
		{"caisse", "Poste de caisse", "/cash", "cash:view", "Ouvrir, encaisser, clôturer"},
	} {
		if sess.aPermission(s.perm) {
			raccourcis = append(raccourcis, map[string]interface{}{"code": s.code, "label": s.label, "href": s.href, "permission": s.perm, "hint": s.hint})
		}
	}

	compteurRequis := len(prerequis)
	faitRequis := compteurRequis - len(manquants)
	actif := etat == etatActif
	catalogue := map[string]interface{}{
		"categories_active": c.categories, "usages_active": c.usages, "usage_categories_active": c.couples,
		"services_active": c.prestations, "services_sellable": c.prestations, "services_priced": c.tarifees,
		"services_sellable_priced": c.tarifees, "services_without_usable_price": c.prestations - c.tarifees,
		"options_active": c.options, "price_rules_active": 0, "payment_methods_active": c.moyens,
		"team_active": c.equipe, "equipment_active": c.materiel, "equipment_out_of_service": 0,
		"stock_items_active": c.stock, "stock_items_uncounted": 0, "stock_items_out": 0,
		"stock_items_low": 0, "stock_items_expired": 0,
	}
	if st != nil {
		var regles int
		_ = a.db.QueryRow(ctx, `SELECT COUNT(*) FROM price_rules WHERE station_id = $1 AND is_active`, st.ID).Scan(&regles)
		catalogue["price_rules_active"] = regles
		var hs int
		_ = a.db.QueryRow(ctx, `SELECT COUNT(*) FROM equipment WHERE station_id = $1 AND status = 'out_of_service'`, st.ID).Scan(&hs)
		catalogue["equipment_out_of_service"] = hs
	}

	setup := map[string]interface{}{
		"role": sess.Role,
		"can": map[string]interface{}{
			"manage_settings": peutGerer, "manage_organization": peutOrg,
			"see_amounts": sess.aPermission("payment:view"), "create_wash": sess.aPermission("wash:create"),
		},
		"blocks": blocs, "shortcuts": raccourcis,
		"station": stationMap,
		"hours": map[string]interface{}{
			"today": aujourdhui, "days_open": c.joursOuverts, "declared_days": c.joursDeclares,
			"label": fmt.Sprintf("%d jour(s) ouvert(s) sur %d déclaré(s)", c.joursOuverts, c.joursDeclares),
		},
		"activation": map[string]interface{}{
			"state": etat, "active": actif, "state_label": libelleEtatActivation(etat, actif),
			"can_activate": !actif && len(manquants) == 0, "blockers": manquants,
			"blockers_count": len(manquants), "required_count": compteurRequis, "required_done": faitRequis,
		},
		"prerequisites": prerequis, "missing": manquants, "optional_settings": facultatifs,
		"catalogue": catalogue,
		"activity":  map[string]interface{}{"orders_total": 0, "orders_open": 0, "note": "Aucune commande enregistrée"},
	}
	if st != nil {
		var total, ouvertes int
		var derniere interface{}
		var statut string
		_ = a.db.QueryRow(ctx, `SELECT COUNT(*) FROM wash_orders WHERE station_id = $1`, st.ID).Scan(&total)
		_ = a.db.QueryRow(ctx, `SELECT COUNT(*) FROM wash_orders WHERE station_id = $1 AND status NOT IN ('completed','cancelled')`, st.ID).Scan(&ouvertes)
		_ = a.db.QueryRow(ctx, `SELECT COALESCE(status::text,'') FROM wash_orders WHERE station_id = $1 ORDER BY created_at DESC LIMIT 1`, st.ID).Scan(&statut)
		if statut != "" {
			derniere = statut
		}
		setup["activity"] = map[string]interface{}{
			"orders_total": total, "orders_open": ouvertes,
			"last_order_status": derniere,
			"note": func() string {
				if total == 0 {
					return "Aucune commande enregistrée"
				}
				return fmt.Sprintf("%d fiche(s) enregistrée(s)", total)
			}(),
		}
	}
	return setup, nil
}

func libelleEtatActivation(etat string, actif bool) string {
	if actif {
		return "Station active"
	}
	switch etat {
	case etatTermine:
		return "Prête à activer"
	case etatEnCours:
		return "Mise en route en cours"
	default:
		return "Mise en route non démarrée"
	}
}

func nomJour(wd int) string {
	noms := []string{"", "Lundi", "Mardi", "Mercredi", "Jeudi", "Vendredi", "Samedi", "Dimanche"}
	if wd >= 1 && wd <= 7 {
		return noms[wd]
	}
	return ""
}

func (a *App) summaryOnboarding(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	orgID, ok := organisationSession(sess)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "no_organization", "Aucune entreprise rattachée à ce compte.")
		return
	}
	ctx := r.Context()
	payload, err := a.calculerOnboarding(ctx, orgID, nil)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	setup, err := a.setupBloc(ctx, sess, orgID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	payload["setup"] = setup
	ecrireJSON(w, http.StatusOK, payload)
}

// PATCH /onboarding/steps/{code} — enregistre les CHOIX descriptifs d'un
// formulaire. `completed` est refusé : seule la base décide qu'une étape est
// faite. Inconnu → 404 ; verrouillé après activation → 409.
func (a *App) enregistrerEtape(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("settings:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	orgID, ok := organisationSession(sess)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "no_organization", "Aucune entreprise rattachée à ce compte.")
		return
	}
	code := cheminParam(r, "/api/v1/onboarding/steps/")
	if defEtape(code) == nil {
		repondreErreur(w, http.StatusNotFound, "unknown_step", "Étape inconnue : "+code)
		return
	}
	if a.etatOnboarding(r.Context(), orgID) == etatActif {
		repondreErreur(w, http.StatusConflict, "already_active", "La station est active : modifiez directement les réglages concernés.")
		return
	}
	brut, ok := decoderCorpsLibre(w, r)
	if !ok {
		return
	}
	data := map[string]interface{}{}
	if corps, present := brut["data"]; present {
		if err := json.Unmarshal(corps, &data); err != nil || data == nil {
			ecrireValidation(w, map[string]string{"data": "Le champ data doit être un objet descriptif."})
			return
		}
	}
	if _, genant := data["completed"]; genant {
		ecrireValidation(w, map[string]string{"data": "Une étape ne se déclare pas faite : le serveur le constate en base."})
		return
	}
	if restant := brut["state"]; restant != nil {
		var s string
		_ = json.Unmarshal(restant, &s)
		if s == "completed" {
			ecrireValidation(w, map[string]string{"state": "state=completed est refusé : l'état « fait » se calcule sur les données réelles."})
			return
		}
	}
	_, err := a.db.Exec(r.Context(), `
		INSERT INTO onboarding_steps (organization_id, step_code, state, data, updated_by, updated_at)
		VALUES ($1, $2, 'in_progress', $3, $4, now())
		ON CONFLICT (organization_id, step_code) DO UPDATE
		SET data = EXCLUDED.data, state = 'in_progress', updated_by = EXCLUDED.updated_by, updated_at = now()`,
		orgID, code, jsonBrut(data), sess.UserID)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	a.journaliser(r.Context(), &orgID, sess.StationID, &sess.UserID, "onboarding.step_saved",
		map[string]interface{}{"step": code})
	payload, err := a.calculerOnboarding(r.Context(), orgID, nil)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	ecrireJSON(w, http.StatusOK, payload)
}

// POST /onboarding/activate — transaction : FOR UPDATE sur la ligne de l'état,
// revérification des prérequis, écriture de l'instantané et du journal. Un
// refus n'écrit rien. Seconde activation : 200 already_active.
func (a *App) activerStation(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) {
	if !sess.aPermission("organization:manage") {
		repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
		return
	}
	orgID, ok := organisationSession(sess)
	if !ok {
		repondreErreur(w, http.StatusNotFound, "no_organization", "Aucune entreprise rattachée à ce compte.")
		return
	}
	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	defer tx.Rollback(ctx)

	var etatActuel string
	err = tx.QueryRow(ctx, `SELECT state FROM onboarding_states WHERE organization_id = $1 FOR UPDATE`, orgID).Scan(&etatActuel)
	if errors.Is(err, pgx.ErrNoRows) {
		repondreErreur(w, http.StatusConflict, "onboarding_not_started", "Démarrez la mise en route avant d'activer la station.")
		return
	}
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	if etatActuel == etatActif {
		payload, err := a.calculerOnboarding(ctx, orgID, nil)
		if err != nil {
			ecrireInterne(w, err)
			return
		}
		payload["already_active"] = true
		payload["message"] = "La station est déjà active."
		ecrireJSON(w, http.StatusOK, payload)
		return
	}

	st, _ := a.lireStationOrganisationTx(ctx, tx, orgID)
	c := a.compterBaseTx(ctx, tx, orgID, st)
	var manquants []blocManquant
	for _, def := range etapesParcours {
		if !def.obligatoire {
			continue
		}
		done, detail, hint := a.constatEtapeTx(ctx, tx, orgID, st, def.code, c)
		if !done {
			if hint == "" {
				hint = detail
			}
			manquants = append(manquants, blocManquant{def.code, def.titre, hint, def.ecran, def.description, detail})
		}
	}
	if len(manquants) > 0 {
		champs := map[string]string{}
		details := []map[string]interface{}{}
		for _, m := range manquants {
			champs[m.code] = m.hint
			details = append(details, map[string]interface{}{
				"code": m.code, "title": m.title, "hint": m.hint, "detail": m.detail, "screen": m.screen,
			})
		}
		ecrireJSON(w, http.StatusConflict, map[string]interface{}{
			"error": erreurAPI{
				Code: "onboarding_incomplete", Message: "La station n'est pas encore activée : terminez la mise en route.",
				Fields: champs, Details: map[string]interface{}{"blockers": details},
			},
		})
		return
	}

	snapshot := map[string]interface{}{
		"prerequisites": map[string]interface{}{"required": 8, "done": 8},
		"catalogue": map[string]interface{}{
			"categories_active": c.categories, "usages_active": c.usages, "services_sellable": c.prestations,
			"services_sellable_priced": c.tarifees, "payment_methods_active": c.moyens, "options_active": c.options,
		},
		"opening_days": c.joursOuverts,
		"optional_remaining": []string{},
	}
	auteur := map[string]interface{}{"user_id": sess.UserID, "name": sess.FullName, "role": sess.Role}
	_, err = tx.Exec(ctx, `
		UPDATE onboarding_states SET state = 'active', activated_at = now(), activated_by = $2, snapshot = $3, updated_at = now()
		WHERE organization_id = $1`, orgID, jsonBrut(auteur), jsonBrut(snapshot))
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	if st != nil {
		_, _ = tx.Exec(ctx, `UPDATE stations SET is_active = true, updated_at = now() WHERE id = $1`, st.ID)
	}
	var orgNom string
	_ = tx.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1`, orgID).Scan(&orgNom)
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (organization_id, station_id, actor_id, action, details)
		VALUES ($1, $2, $3, 'onboarding.activated', $4)`,
		orgID, interfaceNil(sess.StationID), sess.UserID, jsonBrut(map[string]interface{}{"organization": orgNom, "snapshot": snapshot}))
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		ecrireInterne(w, err)
		return
	}

	payload, err := a.calculerOnboarding(ctx, orgID, nil)
	if err != nil {
		ecrireInterne(w, err)
		return
	}
	payload["activated"] = true
	payload["message"] = "La station est active. Bonne route !"
	ecrireJSON(w, http.StatusOK, payload)
}

// --- Variantes transactionnelles ---------------------------------------------

func (a *App) lireStationOrganisationTx(ctx context.Context, tx pgx.Tx, organizationID string) (*stationEntite, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM stations WHERE organization_id = $1 ORDER BY is_default DESC, created_at ASC LIMIT 1`, organizationID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return a.lireStationParID(ctx, id)
}

func (a *App) compterBaseTx(ctx context.Context, tx pgx.Tx, organizationID string, st *stationEntite) constatsCatalogue {
	c := constatsCatalogue{}
	if st == nil {
		return c
	}
	scan := func(requete string, cible *int) {
		var n int
		if err := tx.QueryRow(ctx, requete, st.ID).Scan(&n); err == nil {
			*cible = n
		}
	}
	scan(`SELECT COUNT(*) FROM station_vehicle_categories WHERE station_id = $1 AND is_active`, &c.categories)
	scan(`SELECT COUNT(*) FROM station_vehicle_usages WHERE station_id = $1 AND is_active`, &c.usages)
	scan(`SELECT COUNT(*) FROM services WHERE station_id = $1 AND is_active AND NOT requires_quote`, &c.prestations)
	scan(`SELECT COUNT(*) FROM services s WHERE s.station_id = $1 AND s.is_active AND NOT s.requires_quote
	      AND (s.is_free OR s.price_xof > 0
	           OR EXISTS (SELECT 1 FROM price_rules pr WHERE pr.service_id = s.id AND pr.is_active AND pr.price_xof > 0))`, &c.tarifees)
	scan(`SELECT COUNT(*) FROM service_options WHERE station_id = $1 AND is_active`, &c.options)
	scan(`SELECT COUNT(*) FROM payment_methods WHERE station_id = $1 AND is_active`, &c.moyens)
	return c
}

func (a *App) constatEtapeTx(ctx context.Context, tx pgx.Tx, organizationID string, st *stationEntite, code string, c constatsCatalogue) (bool, string, string) {
	un := func(requete string) int {
		if st == nil {
			return 0
		}
		var n int
		if err := tx.QueryRow(ctx, requete, st.ID).Scan(&n); err != nil {
			return 0
		}
		return n
	}
	deux := func(requete, arg string) int {
		var n int
		if err := tx.QueryRow(ctx, requete, arg).Scan(&n); err != nil {
			return 0
		}
		return n
	}
	switch code {
	case "espace":
		if st != nil {
			return true, "Station « " + st.Name + " » rattachée à votre compte.", ""
		}
		return false, "Aucune station rattachée.", "Créez d'abord votre station."
	case "entreprise":
		var nom, tel, mail string
		_ = tx.QueryRow(ctx, `SELECT name, phone, email FROM organizations WHERE id = $1`, organizationID).Scan(&nom, &tel, &mail)
		if strings.TrimSpace(nom) != "" && strings.TrimSpace(tel) != "" && strings.Contains(mail, "@") {
			return true, fmt.Sprintf("« %s », joignable au %s.", nom, tel), ""
		}
		return false, "Coordonnées incomplètes.", "Complétez l'identité de l'entreprise (nom, téléphone, e-mail)."
	case "station":
		if st == nil {
			return false, "Aucune station.", "Créez la station et déclarez ses capacités."
		}
		pistes := nombreDe(st.Site["bays"])
		reserve := nombreDe(st.Site["water_reserve_liters"])
		if pistes != nil && *pistes >= 0 && reserve != nil && *reserve > 0 {
			return true, fmt.Sprintf("%.0f piste(s), réserve de %.0f L.", *pistes, *reserve), ""
		}
		return false, "Capacités non déclarées.", "Déclarez le nombre de pistes et la réserve d'eau de la station."
	case "horaires":
		n := un(`SELECT COUNT(*) FROM opening_hours WHERE station_id = $1 AND is_open AND opens_at <> '' AND closes_at <> ''`)
		if n > 0 {
			return true, fmt.Sprintf("%d journée(s) ouverte(s) avec horaires.", n), ""
		}
		return false, "Aucune journée complète.", "Définissez au moins une journée d'ouverture avec heures."
	case "categories":
		if c.categories > 0 {
			return true, fmt.Sprintf("%d catégorie(s) active(s).", c.categories), ""
		}
		return false, "Aucune catégorie active.", "Activez au moins une catégorie de véhicules."
	case "usages":
		if c.usages > 0 {
			return true, fmt.Sprintf("%d usage(s) véhicule actif(s).", c.usages), ""
		}
		return false, "Aucun usage déclaré.", "Déclarez les usages de véhicules (facultatif)."
	case "prestations":
		if c.prestations > 0 {
			return true, fmt.Sprintf("%d prestation(s) vendable(s).", c.prestations), ""
		}
		return false, "Aucune prestation active.", "Créez au moins une prestation proposée à la vente."
	case "tarifs":
		if c.prestations > 0 && c.tarifees >= c.prestations {
			return true, fmt.Sprintf("%d/%d prestations tarifées.", c.tarifees, c.prestations), ""
		}
		return false, fmt.Sprintf("%d/%d prestations tarifées.", c.tarifees, c.prestations),
			"Aucune prestation vendable n'a de prix utilisable : fixez les tarifs."
	case "options":
		if c.options > 0 {
			return true, fmt.Sprintf("%d option(s) active(s).", c.options), ""
		}
		return false, "Aucune option.", "Ajoutez des suppléments (facultatif)."
	case "moyens":
		if c.moyens > 0 {
			return true, fmt.Sprintf("%d moyen(s) de paiement actif(s).", c.moyens), ""
		}
		return false, "Aucun moyen de paiement.", "Activez au moins un moyen de paiement (espèces, mobile money…)."
	case "equipe":
		n := un(`SELECT COUNT(*) FROM employees WHERE station_id = $1 AND is_active`)
		inv := deux(`SELECT COUNT(*) FROM invitations WHERE organization_id = $1 AND status = 'pending'`, organizationID)
		if n+inv > 0 {
			return true, fmt.Sprintf("%d membre(s), %d invitation(s).", n, inv), ""
		}
		return false, "Équipe non déclarée.", "Invitez ou déclarez votre équipe (facultatif)."
	case "materiel":
		n := un(`SELECT COUNT(*) FROM equipment WHERE station_id = $1 AND is_active`)
		s := un(`SELECT COUNT(*) FROM stock_items WHERE station_id = $1 AND is_active`)
		if n+s > 0 {
			return true, fmt.Sprintf("%d équipement(s), %d article(s).", n, s), ""
		}
		return false, "Matériel non inventorié.", "Inventoriez matériel et stock (facultatif)."
	case "commandes":
		if st != nil && len(st.Settings) > 0 {
			return true, fmt.Sprintf("%d réglage(s) de commande.", len(st.Settings)), ""
		}
		return false, "Réglages par défaut.", "Ajustez les règles de commande (facultatif)."
	}
	return false, "", ""
}

var _ = fmt.Sprint
