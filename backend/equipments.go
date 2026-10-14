package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Matériel de la station.
//
// Deux idées tiennent ce fichier :
//
//  1. **Un référentiel, pas une contrainte.** Les modèles d'équipements
//     (`equipment_templates`) servent à ajouter un équipement en trois champs.
//     Ils ne limitent rien : ce qui n'y figure pas s'ajoute à la main, et la
//     station peut renommer ce qu'elle veut.
//  2. **Rien ne disparaît.** Un équipement vendu ou cassé est *désactivé* : il
//     quitte la liste des matériels en service, mais ses dates et son coût
//     restent lisibles. C'est ce qui distingue un inventaire d'une liste
//     d'envies.
//
// Aucun champ n'est obligatoire au-delà du nom : une station qui ne connaît pas
// la date d'achat laisse la date vide, et l'écran affiche « non renseigné ».
// ---------------------------------------------------------------------------

// modeleEquipement : une ligne du référentiel produit.
type modeleEquipement struct {
	Code    string
	Nom     string
	Famille string
	Ordre   int
}

// referentielModelesEquipements : les types de matériel d'une station de lavage
// en Côte d'Ivoire. Volontairement court et concret — chaque ligne est un
// matériel qu'on achète, pas une catégorie de catalogue.
var referentielModelesEquipements = []modeleEquipement{
	{Code: "jet_haute_pression", Nom: "Jet haute pression", Famille: "lavage", Ordre: 10},
	{Code: "jet_basse_pression", Nom: "Jet basse pression", Famille: "lavage", Ordre: 20},
	{Code: "aspirateur", Nom: "Aspirateur", Famille: "lavage", Ordre: 30},
	{Code: "polisseuse", Nom: "Polisseuse", Famille: "finition", Ordre: 40},
	{Code: "compresseur", Nom: "Compresseur", Famille: "lavage", Ordre: 50},
	{Code: "pompe_eau", Nom: "Pompe à eau", Famille: "alimentation", Ordre: 60},
	{Code: "cuve_eau", Nom: "Cuve de stockage d'eau", Famille: "alimentation", Ordre: 70},
	{Code: "groupe_electrogene", Nom: "Groupe électrogène", Famille: "energie", Ordre: 80},
	{Code: "fosse_lavage", Nom: "Fosse de lavage", Famille: "infrastructure", Ordre: 90},
	{Code: "pont_levage", Nom: "Pont de levage", Famille: "infrastructure", Ordre: 100},
	{Code: "poste_lavage", Nom: "Poste de lavage (baie)", Famille: "infrastructure", Ordre: 110},
	{Code: "station_air", Nom: "Station d'air (pneus)", Famille: "lavage", Ordre: 120},
	{Code: "camera_surveillance", Nom: "Caméra de surveillance", Famille: "securite", Ordre: 130},
	{Code: "eclairage_chantier", Nom: "Projecteurs d'éclairage", Famille: "infrastructure", Ordre: 140},
	{Code: "brouette_equipement", Nom: "Brouettes et outils", Famille: "outillage", Ordre: 150},
	{Code: "echelle", Nom: "Échelle", Famille: "outillage", Ordre: 160},
	{Code: "caisse_outils", Nom: "Caisse à outils", Famille: "outillage", Ordre: 170},
	{Code: "autre", Nom: "Autre matériel", Famille: "autre", Ordre: 900},
}

// libellesFamillesEquipement : traduction des familles pour l'affichage.
var libellesFamillesEquipement = map[string]string{
	"lavage":         "Lavage",
	"finition":       "Finition",
	"alimentation":   "Alimentation en eau",
	"energie":        "Énergie",
	"infrastructure": "Infrastructure",
	"securite":       "Sécurité",
	"outillage":      "Outillage",
	"autre":          "Autre",
}

// libellesEtatsEquipement : état matériel, tel qu'il est écrit dans la base.
var libellesEtatsEquipement = map[string]string{
	"neuf":         "Neuf",
	"bon":          "Bon état",
	"a_reparer":    "À réparer",
	"hors_service": "Hors service",
}

// assurerModelesEquipements pose le référentiel au démarrage. Chaque modèle
// manquant est créé, aucun n'est écrasé : renommer « Jet haute pression » en
// « Karcher » dans la base survit au redémarrage.
func (a *App) assurerModelesEquipements(ctx context.Context) error {
	for _, modele := range referentielModelesEquipements {
		if _, err := a.Pool.Exec(ctx, `
			INSERT INTO equipment_templates (code, name, family, sort_order)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (code) DO NOTHING`,
			modele.Code, modele.Nom, modele.Famille, modele.Ordre); err != nil {
			return err
		}
	}
	return nil
}

// equipement : un matériel déclaré par la station.
type equipement struct {
	ID            string  `json:"id"`
	StationID     string  `json:"station_id"`
	TemplateCode  *string `json:"template_code"`
	Name          string  `json:"name"`
	Family        string  `json:"family"`
	FamilyLabel   string  `json:"family_label"`
	Quantity      int     `json:"quantity"`
	State         string  `json:"state"`
	StateLabel    string  `json:"state_label"`
	AcquiredOn    *string `json:"acquired_on"`
	LastServiceOn *string `json:"last_service_on"`
	NextServiceOn *string `json:"next_service_on"`
	CostXOF       int     `json:"cost_xof"`
	Notes         string  `json:"notes"`
	IsActive      bool    `json:"is_active"`
	ServiceDue    bool    `json:"service_due"`
	CreatedAt     string  `json:"created_at"`
	CreatedByName string  `json:"created_by_name"`
}

const equipementColumns = `id, station_id, template_code, name, family, quantity, state,
	acquired_on::text, last_service_on::text, next_service_on::text, cost_xof, notes,
	is_active, created_at::text, created_by_name`

// scanEquipement lit une ligne d'équipement.
func scanEquipement(row interface {
	Scan(dest ...any) error
}) (equipement, error) {
	var e equipement
	var template, acquired, lastService, nextService *string
	if err := row.Scan(&e.ID, &e.StationID, &template, &e.Name, &e.Family, &e.Quantity, &e.State,
		&acquired, &lastService, &nextService, &e.CostXOF, &e.Notes,
		&e.IsActive, &e.CreatedAt, &e.CreatedByName); err != nil {
		return e, err
	}
	e.TemplateCode = template
	e.AcquiredOn = acquired
	e.LastServiceOn = lastService
	e.NextServiceOn = nextService
	e.FamilyLabel = libellesFamillesEquipement[e.Family]
	if e.FamilyLabel == "" {
		e.FamilyLabel = e.Family
	}
	e.StateLabel = libellesEtatsEquipement[e.State]
	if e.StateLabel == "" {
		e.StateLabel = e.State
	}
	// Une maintenance à échéance est signalée : c'est la seule raison d'écrire
	// une date de passage future.
	if nextService != nil && strings.TrimSpace(*nextService) != "" {
		if jour, err := time.Parse("2006-01-02", strings.TrimSpace(*nextService)); err == nil {
			e.ServiceDue = !jour.After(time.Now())
		}
	}
	return e, nil
}

// equipementsDeLaStation liste le matériel, actifs d'abord.
func (a *App) equipementsDeLaStation(ctx context.Context, stationID string) ([]equipement, error) {
	rows, err := a.Pool.Query(ctx, `SELECT `+equipementColumns+` FROM equipments
		WHERE station_id = $1 ORDER BY is_active DESC, name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []equipement{}
	for rows.Next() {
		e, err := scanEquipement(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// ---------------------------------------------------------------------------
// GET /api/v1/stations/{id}/equipment
// ---------------------------------------------------------------------------

func (a *App) handleEquipmentList(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}

	modeles := []map[string]any{}
	rows, err := a.Pool.Query(r.Context(), `
		SELECT code, name, family, sort_order FROM equipment_templates
		ORDER BY sort_order, name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}
	for rows.Next() {
		var code, nom, famille string
		var ordre int
		if err := rows.Scan(&code, &nom, &famille, &ordre); err != nil {
			continue
		}
		libelle := libellesFamillesEquipement[famille]
		if libelle == "" {
			libelle = famille
		}
		modeles = append(modeles, map[string]any{
			"code": code, "name": nom, "family": famille, "family_label": libelle, "sort_order": ordre,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	items, err := a.equipementsDeLaStation(r.Context(), stationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur interne.", nil)
		return
	}

	// Le résumé ne compte que le matériel **en service** : une valeur totale
	// qui inclurait le matériel retiré donnerait une fausse idée du parc.
	enService, aReparer, horsService, retires, valeur := 0, 0, 0, 0, 0
	for _, e := range items {
		if !e.IsActive {
			retires++
			continue
		}
		enService++
		if e.State == "a_reparer" {
			aReparer++
		}
		if e.State == "hors_service" {
			horsService++
		}
		valeur += e.CostXOF
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"station_id": stationID,
		"templates":  modeles,
		"items":      items,
		"families":   libellesFamillesEquipement,
		"states":     libellesEtatsEquipement,
		"summary": map[string]any{
			"total": len(enServiceFiltre(items)), "en_service": enService, "a_reparer": aReparer,
			"hors_service": horsService, "retires": retires, "valeur_xof": valeur,
			"maintenance_a_prevoir": nombreMaintenanceDue(items),
		},
	})
}

// enServiceFiltre : les équipements en service (le résumé parle de ceux-là).
func enServiceFiltre(items []equipement) []equipement {
	actifs := make([]equipement, 0, len(items))
	for _, e := range items {
		if e.IsActive {
			actifs = append(actifs, e)
		}
	}
	return actifs
}

func nombreMaintenanceDue(items []equipement) int {
	n := 0
	for _, e := range items {
		if e.IsActive && e.ServiceDue {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// POST /api/v1/stations/{id}/equipment
// ---------------------------------------------------------------------------

func (a *App) handleEquipmentCreate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stationID, err := a.stationDeTravail(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeAPIError(w, err)
		return
	}
	var body struct {
		TemplateCode  string `json:"template_code"`
		Name          string `json:"name"`
		Quantity      *int   `json:"quantity"`
		State         string `json:"state"`
		AcquiredOn    string `json:"acquired_on"`
		LastServiceOn string `json:"last_service_on"`
		NextServiceOn string `json:"next_service_on"`
		CostXOF       *int   `json:"cost_xof"`
		Notes         string `json:"notes"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	nom := strings.TrimSpace(body.Name)
	famille := "autre"
	template := (*string)(nil)
	if code := strings.TrimSpace(body.TemplateCode); code != "" {
		var nomModele, familleModele string
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT name, family FROM equipment_templates WHERE code = $1`, code).
			Scan(&nomModele, &familleModele); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Modèle d'équipement inconnu.", map[string]string{"template_code": "Modèle inconnu"})
			return
		}
		template = &code
		famille = familleModele
		if nom == "" {
			nom = nomModele
		}
	}
	if nom == "" {
		writeError(w, http.StatusBadRequest, "validation_error",
			"Le nom de l'équipement est obligatoire.", map[string]string{"name": "Nom obligatoire"})
		return
	}

	etat := strings.TrimSpace(body.State)
	if etat == "" {
		etat = "bon"
	}
	if _, ok := libellesEtatsEquipement[etat]; !ok {
		writeError(w, http.StatusBadRequest, "validation_error", "État inconnu.",
			map[string]string{"state": "Valeurs acceptées : neuf, bon, a_reparer, hors_service"})
		return
	}
	quantite := 1
	if body.Quantity != nil {
		if *body.Quantity < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"La quantité ne peut pas être négative.", map[string]string{"quantity": "Quantité invalide"})
			return
		}
		quantite = *body.Quantity
	}
	cout := 0
	if body.CostXOF != nil {
		if *body.CostXOF < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le coût ne peut pas être négatif.", map[string]string{"cost_xof": "Montant invalide"})
			return
		}
		cout = *body.CostXOF
	}
	dateAchat, err := dateOuNil(body.AcquiredOn, "acquired_on")
	if err != nil {
		writeAPIError(w, err)
		return
	}
	dernierPassage, err := dateOuNil(body.LastServiceOn, "last_service_on")
	if err != nil {
		writeAPIError(w, err)
		return
	}
	prochainPassage, err := dateOuNil(body.NextServiceOn, "next_service_on")
	if err != nil {
		writeAPIError(w, err)
		return
	}

	id := newID("eqp")
	if _, err := a.Pool.Exec(r.Context(), `
		INSERT INTO equipments (id, organization_id, station_id, template_code, name, family,
		  quantity, state, acquired_on, last_service_on, next_service_on, cost_xof, notes,
		  is_active, created_by_name)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,true,$14)`,
		id, deref(user.OrganizationID), stationID, template, nom, famille,
		quantite, etat, dateAchat, dernierPassage, prochainPassage, cout,
		strings.TrimSpace(body.Notes), user.FullName); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'ajout de l'équipement.", nil)
		return
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), stationID, user.FullName,
		"equipment.created", "equipment", id,
		map[string]any{"name": nom, "state": etat, "quantity": quantite})
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": nom})
}

// ---------------------------------------------------------------------------
// PATCH /api/v1/equipment/{id}
// ---------------------------------------------------------------------------

func (a *App) handleEquipmentUpdate(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	id := r.PathValue("id")
	e, err := a.equipementDeLEntreprise(r.Context(), deref(user.OrganizationID), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Équipement introuvable.", nil)
		return
	}
	if forced := scopedStationID(user); forced != "" && forced != e.StationID {
		writeError(w, http.StatusNotFound, "not_found", "Équipement introuvable.", nil)
		return
	}

	var body struct {
		Name          *string `json:"name"`
		Quantity      *int    `json:"quantity"`
		State         *string `json:"state"`
		AcquiredOn    *string `json:"acquired_on"`
		LastServiceOn *string `json:"last_service_on"`
		NextServiceOn *string `json:"next_service_on"`
		CostXOF       *int    `json:"cost_xof"`
		Notes         *string `json:"notes"`
		IsActive      *bool   `json:"is_active"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", "Corps de requête invalide.", nil)
		return
	}

	clauses := []string{"updated_at = now()"}
	args := []any{id}
	ajoute := func(sql string, valeur any) {
		args = append(args, valeur)
		clauses = append(clauses, sql+" = $"+itoa(len(args)))
	}
	if body.Name != nil {
		nom := strings.TrimSpace(*body.Name)
		if nom == "" {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le nom de l'équipement est obligatoire.", map[string]string{"name": "Nom obligatoire"})
			return
		}
		ajoute("name", nom)
	}
	if body.Quantity != nil {
		if *body.Quantity < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"La quantité ne peut pas être négative.", map[string]string{"quantity": "Quantité invalide"})
			return
		}
		ajoute("quantity", *body.Quantity)
	}
	if body.State != nil {
		if _, ok := libellesEtatsEquipement[*body.State]; !ok {
			writeError(w, http.StatusBadRequest, "validation_error", "État inconnu.",
				map[string]string{"state": "Valeurs acceptées : neuf, bon, a_reparer, hors_service"})
			return
		}
		ajoute("state", *body.State)
	}
	for _, champ := range []struct {
		cle    string
		valeur *string
	}{{"acquired_on", body.AcquiredOn}, {"last_service_on", body.LastServiceOn}, {"next_service_on", body.NextServiceOn}} {
		if champ.valeur == nil {
			continue
		}
		date, err := dateOuNil(*champ.valeur, champ.cle)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		ajoute(champ.cle, date)
	}
	if body.CostXOF != nil {
		if *body.CostXOF < 0 {
			writeError(w, http.StatusBadRequest, "validation_error",
				"Le coût ne peut pas être négatif.", map[string]string{"cost_xof": "Montant invalide"})
			return
		}
		ajoute("cost_xof", *body.CostXOF)
	}
	if body.Notes != nil {
		ajoute("notes", strings.TrimSpace(*body.Notes))
	}
	if body.IsActive != nil {
		ajoute("is_active", *body.IsActive)
	}

	args = append(args, deref(user.OrganizationID))
	requete := `UPDATE equipments SET ` + strings.Join(clauses, ", ") +
		` WHERE id = $1 AND organization_id = $` + itoa(len(args))
	if _, err := a.Pool.Exec(r.Context(), requete, args...); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "Erreur lors de l'enregistrement.", nil)
		return
	}

	a.addAudit(r.Context(), deref(user.OrganizationID), e.StationID, user.FullName,
		"equipment.updated", "equipment", id, map[string]any{"name": e.Name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// equipementDeLEntreprise : lecture bornée à l'entreprise (une autre
// entreprise reçoit 404, jamais « interdit » — qui ne dit rien de l'existence
// de la ressource).
func (a *App) equipementDeLEntreprise(ctx context.Context, orgID, id string) (equipement, error) {
	return scanEquipement(a.Pool.QueryRow(ctx,
		`SELECT `+equipementColumns+` FROM equipments WHERE id = $1 AND organization_id = $2`, id, orgID))
}

// dateOuNil convertit « AAAA-MM-JJ » en date SQL, la chaîne vide en NULL.
// Une date écrite n'importe comment est refusée **sur son champ**, plutôt que
// silencieusement ignorée.
func dateOuNil(valeur, champ string) (*time.Time, error) {
	texte := strings.TrimSpace(valeur)
	if texte == "" {
		return nil, nil
	}
	jour, err := time.Parse("2006-01-02", texte)
	if err != nil {
		return nil, &apiError{
			Status:  http.StatusBadRequest,
			Code:    "validation_error",
			Message: "Date invalide (« " + texte + " ») : format attendu AAAA-MM-JJ.",
			Fields:  map[string]string{champ: "Format attendu : AAAA-MM-JJ"},
		}
	}
	return &jour, nil
}
