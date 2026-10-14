package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---------------------------------------------------------------------------
// Stations et catégories de véhicules.
// ---------------------------------------------------------------------------

func scanStation(rows pgx.Rows) (*Station, error) {
	var s Station
	var openingRaw, settingsRaw []byte
	var createdAt time.Time
	err := rows.Scan(&s.ID, &s.OrganizationID, &s.Name, &s.Code, &s.Phone, &s.Address, &s.City,
		&s.Timezone, &s.Currency, &s.LogoURL, &openingRaw, &settingsRaw, &s.IsDefault, &s.IsActive, &createdAt)
	if err != nil {
		return nil, err
	}
	s.CreatedAt = createdAt
	s.OpeningHours = map[string]any{}
	s.Settings = map[string]any{}
	parseJSONB(openingRaw, &s.OpeningHours)
	parseJSONB(settingsRaw, &s.Settings)
	return &s, nil
}

const stationColumns = `id, organization_id, name, code, phone, address, city, timezone, currency,
	logo_url, opening_hours, settings, is_default, is_active, created_at`

// GET /stations — renvoie un tableau (contrat attendu par l'interface) limité
// au périmètre : un caissier ne voit que la station à laquelle il est rattaché.
func (a *App) handleStationsList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	rows, err := a.Pool.Query(r.Context(), `
		SELECT `+stationColumns+`
		FROM stations
		WHERE organization_id = $1
		  AND ($2 = '' OR id = $2)
		ORDER BY is_default DESC, name`,
		deref(user.OrganizationID), scopedStationID(user))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		st, err := scanStation(rows)
		if err != nil {
			continue
		}
		items = append(items, stationToJSON(st))
	}
	writeJSON(w, http.StatusOK, items)
}

// scopedStationID : station imposée par le rattachement du compte (vide pour
// le propriétaire et pour un gérant non rattaché).
func scopedStationID(user *User) string {
	if isStationScoped(user) {
		return deref(user.StationID)
	}
	return ""
}

func (a *App) handleStationCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	var body struct {
		Name     string `json:"name"`
		Code     string `json:"code"`
		Phone    string `json:"phone"`
		Address  string `json:"address"`
		City     string `json:"city"`
		Timezone string `json:"timezone"`
	}
	if err := decodeBody(r, &body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "Le nom de la station est obligatoire.", nil)
		return
	}
	if err := a.checkPlanLimit(r, user, limitStations); err != nil {
		writeAPIError(w, err)
		return
	}
	code := body.Code
	if code == "" {
		code = defaultStationCode(body.Name)
	}
	city := body.City
	if city == "" {
		city = "Abidjan"
	}
	tz := body.Timezone
	if tz == "" {
		tz = "Africa/Abidjan"
	}
	id := newID("st")
	_, err := a.Pool.Exec(r.Context(), `
		INSERT INTO stations (id, organization_id, name, code, phone, address, city, timezone, settings)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)`,
		id, deref(user.OrganizationID), body.Name, code, body.Phone, body.Address, city, tz,
		defaultStationSettings(0))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la création de la station.", nil)
		return
	}
	// Toute station ouverte reçoit le catalogue de prestations (sans prix) et
	// les moyens de paiement : sans eux, la caisse ne pourrait rien encaisser et
	// l'étape « moyens de paiement » du parcours resterait bloquée.
	if err := a.equiperCatalogueNeuf(r.Context(), deref(user.OrganizationID), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'équipement du catalogue.", nil)
		return
	}
	if err := a.equiperMoyensPaiement(r.Context(), deref(user.OrganizationID), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'installation des moyens de paiement.", nil)
		return
	}
	a.addAudit(r.Context(), deref(user.OrganizationID), id, user.FullName, "station.created", "station", id, nil)
	st, _ := a.getStation(r.Context(), id)
	writeJSON(w, http.StatusCreated, stationToJSON(st))
}

func (a *App) handleStationUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	st, err := a.getStation(r.Context(), id)
	if err != nil || st == nil || st.OrganizationID != deref(user.OrganizationID) {
		writeError(w, http.StatusNotFound, "not_found", "Station introuvable.", nil)
		return
	}
	// Un caissier ou un gérant rattaché ne modifie que sa station.
	if forced := scopedStationID(user); forced != "" && forced != id {
		writeAPIError(w, errStationForbidden)
		return
	}
	var body map[string]any
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	allowed := map[string]string{
		"name": "name", "code": "code", "phone": "phone", "address": "address",
		"city": "city", "logo_url": "logo_url",
	}
	// Localisation (adresse structurée, coordonnées) : mêmes règles d'écriture.
	for champ, colonne := range champsLocalisationStation {
		allowed[champ] = colonne
	}

	// Les coordonnées sont numériques : on refuse une valeur illisible plutôt
	// que de l'ignorer en silence (l'écran croirait l'avoir enregistrée).
	erreursChamps := map[string]string{}
	for _, champ := range []string{"latitude", "longitude"} {
		if v, ok := body[champ]; ok && strings.TrimSpace(toText(v)) != "" {
			if _, err := strconv.ParseFloat(strings.TrimSpace(toText(v)), 64); err != nil {
				erreursChamps[champ] = "Coordonnée attendue en degrés décimaux (ex. 5.3489)"
			}
		}
	}
	if len(erreursChamps) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "Certaines valeurs ne sont pas valides.", erreursChamps)
		return
	}

	changed := []string{}
	for field, column := range allowed {
		v, ok := body[field]
		if !ok {
			continue
		}
		var err error
		if column == "latitude" || column == "longitude" {
			_, err = a.Pool.Exec(r.Context(),
				`UPDATE stations SET `+column+` = NULLIF($1,'')::numeric WHERE id = $2 AND organization_id = $3`,
				strings.TrimSpace(toText(v)), id, st.OrganizationID)
		} else {
			_, err = a.Pool.Exec(r.Context(),
				`UPDATE stations SET `+column+` = $1 WHERE id = $2 AND organization_id = $3`,
				toText(v), id, st.OrganizationID)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de la mise à jour.", nil)
			return
		}
		changed = append(changed, field)
	}

	// Caractéristiques physiques (capacités, équipements) : elles vivent dans
	// leur propre table, avec des valeurs nullables — « non répondu » reste
	// distinct de « non ».
	if site, ok := body["site"].(map[string]any); ok && len(site) > 0 {
		if err := a.enregistrerProfilSite(r.Context(), id, currentUser(r).ID, site); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement des caractéristiques.", nil)
			return
		}
		changed = append(changed, "site")
	}
	// Les réglages métier sont fusionnés (on ne remplace jamais la totalité :
	// un écran ne doit pas effacer ce qu'il n'affiche pas).
	if v, ok := body["settings"].(map[string]any); ok {
		merged, err := a.mergeStationSettings(r, id, v)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement des paramètres.", nil)
			return
		}
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE stations SET settings = $1::jsonb WHERE id = $2 AND organization_id = $3`,
			jsonOrNull(merged), id, st.OrganizationID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement des paramètres.", nil)
			return
		}
		changed = append(changed, "settings")
	}
	if v, ok := body["opening_hours"].(map[string]any); ok {
		_, _ = a.Pool.Exec(r.Context(),
			`UPDATE stations SET opening_hours = $1::jsonb WHERE id = $2 AND organization_id = $3`,
			jsonOrNull(v), id, st.OrganizationID)
		changed = append(changed, "opening_hours")
	}
	if v, ok := body["is_active"].(bool); ok {
		_, _ = a.Pool.Exec(r.Context(),
			`UPDATE stations SET is_active = $1 WHERE id = $2 AND organization_id = $3`, v, id, st.OrganizationID)
		changed = append(changed, "is_active")
	}
	if v, ok := body["is_default"].(bool); ok && v {
		_, _ = a.Pool.Exec(r.Context(), `UPDATE stations SET is_default = false WHERE organization_id = $1`, st.OrganizationID)
		_, _ = a.Pool.Exec(r.Context(), `UPDATE stations SET is_default = true WHERE id = $1 AND organization_id = $2`, id, st.OrganizationID)
		changed = append(changed, "is_default")
	}
	a.addAudit(r.Context(), st.OrganizationID, id, user.FullName, "station.updated", "station", id,
		map[string]any{"fields": changed})
	updated, _ := a.getStation(r.Context(), id)
	writeJSON(w, http.StatusOK, stationToJSON(updated))
}

// mergeStationSettings fusionne les réglages reçus avec ceux déjà enregistrés.
func (a *App) mergeStationSettings(r *http.Request, stationID string, incoming map[string]any) (map[string]any, error) {
	var raw []byte
	if err := a.Pool.QueryRow(r.Context(), `SELECT settings FROM stations WHERE id = $1`, stationID).Scan(&raw); err != nil {
		return nil, err
	}
	current := map[string]any{}
	parseJSONB(raw, &current)
	for key, value := range incoming {
		current[key] = value
	}
	return current, nil
}

// scanAuditRows lit les lignes d'audit (réutilisé par plusieurs handlers).
func scanAuditRows(rows pgx.Rows) []map[string]any {
	items := []map[string]any{}
	for rows.Next() {
		var e AuditEntry
		var detailsRaw []byte
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.StationID, &e.ActorName,
			&e.Action, &e.Entity, &e.EntityID, &detailsRaw, &e.CreatedAt); err != nil {
			continue
		}
		e.Details = map[string]any{}
		parseJSONB(detailsRaw, &e.Details)
		items = append(items, map[string]any{
			"id": e.ID, "organization_id": e.OrganizationID, "station_id": e.StationID,
			"actor_name": e.ActorName, "action": e.Action, "entity": e.Entity,
			"entity_id": e.EntityID, "details": e.Details,
			"created_at": e.CreatedAt.Format(timeLayout),
		})
	}
	return items
}

// handleStationOpeningHours : PUT /stations/{id}/opening-hours.
//
// Le tableau des sept jours et les journées particulières sont enregistrés
// ensemble : l'écran n'a jamais à choisir lequel envoyer. Les horaires sont
// validés avant d'atteindre la base — une journée qui ferme avant d'ouvrir
// rendrait faux tout calcul de statut d'ouverture.
func (a *App) handleStationOpeningHours(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")

	st, err := a.getStation(r.Context(), id)
	if err != nil || st == nil || st.OrganizationID != deref(user.OrganizationID) {
		writeError(w, http.StatusNotFound, "not_found", "Station introuvable.", nil)
		return
	}
	// Un caissier ou un gérant rattaché ne modifie que sa station.
	if forced := scopedStationID(user); forced != "" && forced != id {
		writeAPIError(w, errStationForbidden)
		return
	}

	var body struct {
		Days       []horaireJour      `json:"days"`
		Exceptions []exceptionHoraire `json:"exceptions"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}
	if len(body.Days) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Envoyez les sept jours de la semaine (un jour fermé reste un jour).", nil)
		return
	}
	if erreurs := validerHoraires(body.Days); len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Les horaires transmis ne sont pas cohérents.", erreurs)
		return
	}
	if erreurs := validerExceptions(body.Exceptions); len(erreurs) > 0 {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Les journées particulières transmises ne sont pas cohérentes.", erreurs)
		return
	}

	if err := a.enregistrerHoraires(r.Context(), id, user.ID, body.Days, body.Exceptions); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement des horaires.", nil)
		return
	}
	a.addAudit(r.Context(), st.OrganizationID, id, user.FullName, "station.hours_updated", "station", id,
		map[string]any{"jours_ouverts": compterJoursOuverts(body.Days), "exceptions": len(body.Exceptions)})

	jours, _ := a.lireHoraires(r.Context(), id)
	exceptions, _ := a.lireExceptions(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{
		"days": jours, "exceptions": exceptions, "message": "Horaires enregistrés.",
	})
}

// compterJoursOuverts : nombre de jours d'ouverture déclarés (journalisé sans
// recopier les heures, qui n'apprendraient rien de plus).
func compterJoursOuverts(jours []horaireJour) int {
	ouverts := 0
	for _, jour := range jours {
		if jour.IsOpen {
			ouverts++
		}
	}
	return ouverts
}

// handleStationProfile : GET /stations/{id}/profile.
//
// Rassemble en une réponse ce que les étapes « station » et « horaires » du
// parcours affichent : la station elle-même, ses capacités physiques
// (nullables : « non renseigné » n'est pas « non ») et ses horaires.
func (a *App) handleStationProfile(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")

	st, err := a.getStation(r.Context(), id)
	if err != nil || st == nil || st.OrganizationID != deref(user.OrganizationID) {
		writeError(w, http.StatusNotFound, "not_found", "Station introuvable.", nil)
		return
	}
	// Un employé rattaché à une autre station n'a rien à y lire.
	if forced := scopedStationID(user); forced != "" && forced != id {
		writeAPIError(w, errStationForbidden)
		return
	}

	site, err := a.lireProfilSite(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	jours, err := a.lireHoraires(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	exceptions, err := a.lireExceptions(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"station": st, "site": site, "days": jours, "exceptions": exceptions,
		"types_lavage_proposes": typesLavage,
	})
}
