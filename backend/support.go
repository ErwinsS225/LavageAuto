package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// support.go — outils communs à tous les handlers : journal d'audit,
// vérification d'appartenance des objets, et amorçage des référentiels.

func (a *App) journaliser(ctx context.Context, organizationID, stationID *string, actorID *string, action string, detail map[string]interface{}) {
	if detail == nil {
		detail = map[string]interface{}{}
	}
	body, _ := json.Marshal(detail)
	var acteur interface{}
	if actorID != nil && *actorID != "" {
		acteur = *actorID
	}
	_, err := a.db.Exec(ctx, `
		INSERT INTO audit_logs (organization_id, station_id, actor_id, action, detail)
		VALUES ($1, $2, $3, $4, $5::jsonb)`,
		organisationDe(organizationID), interfaceNil(stationID), acteur, action, body)
	if err != nil {
		log.Printf("audit (%s) : %v", action, err)
	}
}

func organisationDe(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func interfaceNil(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

// stationVerifiee contrôle l'appartenance : un identifiant d'une autre
// entreprise répond 404 (on ne révèle pas son existence), et un compte rattaché
// à une station ne peut toucher qu'à la sienne.
func (a *App) stationVerifiee(ctx context.Context, sess *sessionUtilisateur, stationID string) (*stationEntite, bool) {
	st, err := a.lireStationParID(ctx, stationID)
	if err != nil {
		return nil, false
	}
	if st.OrganizationID == "" || st.OrganizationID != orElse(sess.OrganizationID, "") {
		return nil, false
	}
	if sess.StationID != nil && *sess.StationID != "" && *sess.StationID != st.ID {
		return nil, false
	}
	return st, true
}

func orElse(v *string, defaut string) string {
	if v == nil {
		return defaut
	}
	return *v
}

type stationEntite struct {
	ID             string
	OrganizationID string
	Name           string
	Code           string
	Phone          string
	Address        string
	City           string
	Country        string
	District       string
	Commune        string
	DistrictArea   string
	Street         string
	Landmark       string
	Latitude       *string
	Longitude      *string
	BillingAddress string
	Timezone       string
	Currency       string
	LogoURL        string
	Site           map[string]interface{}
	Settings       map[string]interface{}
	IsDefault      bool
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (a *App) lireStationParID(ctx context.Context, id string) (*stationEntite, error) {
	st := &stationEntite{}
	var siteBody, settingsBody []byte
	err := a.db.QueryRow(ctx, `
		SELECT id, organization_id, name, code, phone, address, city, country, district, commune,
		       district_area, street, landmark, latitude::text, longitude::text, billing_address,
		       timezone, currency, logo_url, site, settings, is_default, is_active, created_at, updated_at
		FROM stations WHERE id = $1`, id).
		Scan(&st.ID, &st.OrganizationID, &st.Name, &st.Code, &st.Phone, &st.Address, &st.City,
			&st.Country, &st.District, &st.Commune, &st.DistrictArea, &st.Street, &st.Landmark,
			&st.Latitude, &st.Longitude, &st.BillingAddress, &st.Timezone, &st.Currency,
			&st.LogoURL, &siteBody, &settingsBody, &st.IsDefault, &st.IsActive, &st.CreatedAt, &st.UpdatedAt)
	if err != nil {
		return nil, err
	}
	st.Site = decoderObjet(siteBody)
	st.Settings = decoderObjet(settingsBody)
	return st, nil
}

func (a *App) lireStationOrganisation(ctx context.Context, organizationID string) (*stationEntite, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id FROM stations WHERE organization_id = $1 ORDER BY is_default DESC, created_at ASC LIMIT 1`,
		organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, pgx.ErrNoRows
	}
	var id string
	if err := rows.Scan(&id); err != nil {
		return nil, err
	}
	rows.Close()
	return a.lireStationParID(ctx, id)
}

func decoderObjet(body []byte) map[string]interface{} {
	m := map[string]interface{}{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &m)
	}
	return m
}

// rendreStation produit le JSON de lecture commun (front `Station`).
func (st *stationEntite) rendre() map[string]interface{} {
	latitude, longitude := "", ""
	if st.Latitude != nil {
		latitude = *st.Latitude
	}
	if st.Longitude != nil {
		longitude = *st.Longitude
	}
	return map[string]interface{}{
		"id": st.ID, "organization_id": st.OrganizationID, "name": st.Name, "code": st.Code,
		"phone": st.Phone, "address": st.Address, "city": st.City, "country": st.Country,
		"district": st.District, "commune": st.Commune, "district_area": st.DistrictArea,
		"street": st.Street, "landmark": st.Landmark, "latitude": latitude, "longitude": longitude,
		"billing_address": st.BillingAddress, "timezone": st.Timezone, "currency": st.Currency,
		"logo_url": st.LogoURL, "site": st.Site, "settings": st.Settings,
		"is_default": st.IsDefault, "is_active": st.IsActive,
		"created_at": st.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": st.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// --- Types souples pour le JSON ---------------------------------------------

type triBool struct{ value *bool }

func (t *triBool) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		t.value = nil
		return nil
	}
	if s == "true" {
		v := true
		t.value = &v
		return nil
	}
	if s == "false" {
		v := false
		t.value = &v
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err == nil {
		v := n != 0
		t.value = &v
		return nil
	}
	return fmt.Errorf("valeur booléenne invalide")
}

func (t triBool) rendre() interface{} {
	if t.value == nil {
		return nil
	}
	return *t.value
}

type champTri struct {
	present bool
	value   *bool
}

func lireChampTri(brut map[string]json.RawMessage, cle string) champTri {
	v, ok := brut[cle]
	if !ok {
		return champTri{}
	}
	s := strings.TrimSpace(string(v))
	switch s {
	case "null":
		return champTri{present: true}
	case "true":
		b := true
		return champTri{present: true, value: &b}
	case "false":
		b := false
		return champTri{present: true, value: &b}
	}
	var n float64
	if err := json.Unmarshal(v, &n); err == nil {
		b := n != 0
		return champTri{present: true, value: &b}
	}
	return champTri{present: true} // illisible : traité comme « non renseigné »
}

func appliquerTri(cible map[string]interface{}, cle string, champ champTri) {
	if !champ.present {
		return
	}
	if champ.value == nil {
		cible[cle] = nil
		return
	}
	cible[cle] = *champ.value
}

type champNombre struct {
	present bool
	value   *float64
}

func lireChampNombre(brut map[string]json.RawMessage, cle string) champNombre {
	v, ok := brut[cle]
	if !ok {
		return champNombre{}
	}
	s := strings.TrimSpace(string(v))
	if s == "null" || s == `""` {
		return champNombre{present: true}
	}
	var n float64
	if err := json.Unmarshal(v, &n); err != nil {
		return champNombre{present: true}
	}
	return champNombre{present: true, value: &n}
}

type champTexte struct {
	present bool
	value   string
}

func lireChampTexte(brut map[string]json.RawMessage, cle string) champTexte {
	v, ok := brut[cle]
	if !ok {
		return champTexte{}
	}
	s := strings.TrimSpace(string(v))
	if s == "null" {
		return champTexte{present: true}
	}
	var txt string
	if err := json.Unmarshal(v, &txt); err != nil {
		return champTexte{present: true, value: s}
	}
	return champTexte{present: true, value: strings.TrimSpace(txt)}
}

// --- Validations --------------------------------------------------------------

var (
	reHEURE     = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	reDATE      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	reCODE      = regexp.MustCompile(`^[A-Za-z0-9_-]{2,32}$`)
	reTELEPHONE = regexp.MustCompile(`^[+0-9][0-9 .\-()/]{5,24}$`)
)

const codeUUIDNil = "00000000-0000-0000-0000-000000000000"

func estVrai(v interface{}) bool { b, _ := v.(bool); return b }

func chaineOuVide(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func nombreDe(v interface{}) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case int:
		f := float64(n)
		return &f
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return &f
		}
	}
	return nil
}

func entierDe(v interface{}, defaut int) int {
	if n := nombreDe(v); n != nil {
		return int(*n)
	}
	return defaut
}

// tableauDe accepte []interface{} ou [][]string venant du front.
func tableauDe(v interface{}) []interface{} {
	if l, ok := v.([]interface{}); ok {
		return l
	}
	return nil
}

func validerTelephone(cle, valeur string, champs map[string]string) {
	if valeur == "" {
		return
	}
	if !reTELEPHONE.MatchString(valeur) {
		champs[cle] = "Numéro de téléphone invalide."
	}
}

func validerCoordonnee(cle, valeur string, min, max float64, champs map[string]string) {
	if valeur == "" {
		return
	}
	n, err := strconv.ParseFloat(valeur, 64)
	if err != nil || n < min || n > max {
		champs[cle] = fmt.Sprintf("Valeur hors bornes (%g à %g).", min, max)
		return
	}
	if strings.Contains(valeur, ".") && len(strings.Split(valeur, ".")[1]) > 6 {
		champs[cle] = "Six décimales maximum."
	}
}

func validerHeure(cle, valeur string, obligatoire bool, champs map[string]string) {
	if valeur == "" {
		if obligatoire {
			champs[cle] = "Heure manquante."
		}
		return
	}
	if !reHEURE.MatchString(valeur) {
		champs[cle] = "Format attendu : HH:MM."
	}
}

func minutesDepuisMinuit(valeur string) int {
	p := strings.SplitN(valeur, ":", 2)
	if len(p) != 2 {
		return -1
	}
	h, err1 := strconv.Atoi(p[0])
	m, err2 := strconv.Atoi(p[1])
	if err1 != nil || err2 != nil {
		return -1
	}
	return h*60 + m
}

// --- Anti-race : verrou par organisation -------------------------------------

type verrousOrg struct {
	mu   sync.Mutex
	cles map[string]*sync.Mutex
}

func nouvellesVerrous() *verrousOrg {
	return &verrousOrg{cles: map[string]*sync.Mutex{}}
}

func (v *verrousOrg) prendre(cle string) func() {
	v.mu.Lock()
	m, ok := v.cles[cle]
	if !ok {
		m = &sync.Mutex{}
		v.cles[cle] = m
	}
	v.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// --- Amorçage ------------------------------------------------------------------

type entreeReferentiel struct {
	code, nom, famille, description string
	alias                           []string
	ordre                           int
}

var categoriesProduit = []entreeReferentiel{
	{"berline", "Berline", "voiture", "Citadine et berline classique.", []string{"citadine", "voiture"}, 10},
	{"suv", "SUV / 4x4", "voiture", "Véhicule haut, break encombrant.", []string{"quatre-quatre", "break"}, 20},
	{"utilitaire_leger", "Utilitaire léger", "utilitaire", "Fourgonnette, van de livraison.", []string{"fourgon", "van"}, 30},
	{"monospace", "Monospace", "voiture", "7 places, véhicule familial.", []string{"minivan"}, 40},
	{"camionnette", "Camionnette", "utilitaire", "Pick-up et petite camion.", []string{"pickup"}, 50},
	{"deux_roues", "Deux-roues", "moto", "Moto, scooter, cyclomoteur.", []string{"moto", "scooter"}, 60},
}

var usagesProduit = []entreeReferentiel{
	{"personnel", "Voiture personnelle", "", "Usage privé.", nil, 10},
	{"taxi_compteur", "Taxi compteur", "", "Taxi de course avec compteur.", nil, 20},
	{"taxi_communal", "Taxi communal", "", "Taxi collectif de ville (« gbaka », « wôrô-wôrô »).", nil, 30},
	{"livraison", "Livraison", "", "Véhicule de livraison et coursier.", nil, 40},
	{"flotte", "Flotte d'entreprise", "", "Véhicule de société ou administration.", nil, 50},
	{"auto_ecole", "Auto-école", "", "Véhicule d'apprentissage.", nil, 60},
}

var famillesService = []entreeReferentiel{
	{"exterieur", "Lavage extérieur", "", "", nil, 10},
	{"interieur", "Nettoyage intérieur", "", "", nil, 20},
	{"complet", "Prestations complètes", "", "", nil, 30},
	{"special", "Traitements spéciaux", "", "", nil, 40},
}

var modelesPrestation = []entreeReferentiel{
	{"ext_simple", "Lavage extérieur simple", "exterieur", "Rinçage, mousse, essuyage.", nil, 10},
	{"inter_aspiration", "Intérieur + aspiration", "interieur", "Aspiration habitacle, tableaux de bord, vitres intérieures.", nil, 20},
	{"complet_standard", "Lavage complet standard", "complet", "Extérieur et intérieur.", nil, 30},
	{"complet_premium", "Lavage complet premium", "complet", "Complet avec brillance jantes et plastiques.", nil, 40},
	{"interieur_detaille", "Nettoyage intérieur détaillé", "special", "Shampouinage sièges et moquettes.", nil, 50},
	{"moteur", "Nettoyage moteur", "special", "Dégraissage prudent du compartiment moteur.", nil, 60},
}

var categoriesOption = []entreeReferentiel{
	{"supplement", "Suppléments", "", "", nil, 10},
	{"traitement", "Traitements", "", "", nil, 20},
	{"fidelite", "Formules", "", "", nil, 30},
}

var moyensPaiement = []entreeReferentiel{
	{"especes", "Espèces", "cash", "", nil, 10},
	{"orange_cash", "Orange Money", "mobile", "", nil, 20},
	{"mtmomo", "MTN MoMo", "mobile", "", nil, 30},
	{"moov_money", "Moov Money", "mobile", "", nil, 40},
	{"wave", "Wave", "mobile", "", nil, 50},
	{"carte", "Carte bancaire", "card", "", nil, 60},
	{"cheque", "Chèque", "other", "", nil, 70},
	{"transfert", "Virement / transfert", "other", "", nil, 80},
}

var mentionsRegistre = []struct{ code, label, aide string }{
	{"rccm", "RCCM", "Numéro du registre du commerce."},
	{"niu", "NIU", "Numéro d'identification unique (impôts)."},
	{"cnps", "N° CNPS", "Immatriculation CNPS de l'employeur."},
}

func (a *App) amorcerReferentiels(ctx context.Context) error {
	upsert := `
		INSERT INTO vehicle_categories (organization_id, code, name, family, description, aliases, sort_order)
		VALUES (NULL, $1, $2, $3, $4, $5, $6)
		ON CONFLICT ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code)
		DO UPDATE SET name = EXCLUDED.name, family = EXCLUDED.family, description = EXCLUDED.description,
		              aliases = EXCLUDED.aliases, sort_order = EXCLUDED.sort_order`
	for _, e := range categoriesProduit {
		if _, err := a.db.Exec(ctx, upsert, e.code, e.nom, e.famille, e.description, e.alias, e.ordre); err != nil {
			return err
		}
	}
	upsertUsage := `
		INSERT INTO vehicle_usages (organization_id, code, name, description, sort_order)
		VALUES (NULL, $1, $2, $3, $4)
		ON CONFLICT ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code)
		DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, sort_order = EXCLUDED.sort_order`
	for _, e := range usagesProduit {
		if _, err := a.db.Exec(ctx, upsertUsage, e.code, e.nom, e.description, e.ordre); err != nil {
			return err
		}
	}
	upsertFamille := `
		INSERT INTO service_families (organization_id, code, name, sort_order)
		VALUES (NULL, $1, $2, $3)
		ON CONFLICT ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code)
		DO UPDATE SET name = EXCLUDED.name, sort_order = EXCLUDED.sort_order`
	for _, e := range famillesService {
		if _, err := a.db.Exec(ctx, upsertFamille, e.code, e.nom, e.ordre); err != nil {
			return err
		}
	}
	upsertModele := `
		INSERT INTO service_templates (code, name, family_code, description, default_duration_minutes)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, family_code = EXCLUDED.family_code,
		                                  description = EXCLUDED.description`
	for _, e := range modelesPrestation {
		if _, err := a.db.Exec(ctx, upsertModele, e.code, e.nom, e.famille, e.description, 30); err != nil {
			return err
		}
	}
	upsertOption := `
		INSERT INTO service_option_categories (organization_id, code, name, sort_order)
		VALUES (NULL, $1, $2, $3)
		ON CONFLICT ((COALESCE(organization_id, '00000000-0000-0000-0000-000000000000'::uuid)), code)
		DO UPDATE SET name = EXCLUDED.name, sort_order = EXCLUDED.sort_order`
	for _, e := range categoriesOption {
		if _, err := a.db.Exec(ctx, upsertOption, e.code, e.nom, e.ordre); err != nil {
			return err
		}
	}
	upsertMoyen := `
		INSERT INTO payment_methods (organization_id, station_id, code, name, family, requires_reference, sort_order)
		VALUES (NULL, NULL, $1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING`
	for _, e := range moyensPaiement {
		ref := e.code == "cheque" || e.code == "transfert" || e.code == "carte"
		if _, err := a.db.Exec(ctx, upsertMoyen, e.code, e.nom, e.famille, ref, e.ordre); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) amorcerPlans(ctx context.Context) error {
	plans := []struct {
		code, nom, description         string
		prix, maxStations, maxEmployes int
	}{
		{"decouverte", "Découverte", "Une station, l'essentiel pour démarrer.", 15000, 1, 3},
		{"pro", "Pro", "Jusqu'à trois stations, toute l'exploitation.", 40000, 3, 15},
		{"reseau", "Réseau", "Stations et équipes illimitées.", 90000, 100000, 100000},
	}
	for _, p := range plans {
		_, err := a.db.Exec(ctx, `
			INSERT INTO plans (code, name, description, price_xof, max_stations, max_employees)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description,
			                                 price_xof = EXCLUDED.price_xof,
			                                 max_stations = EXCLUDED.max_stations,
			                                 max_employees = EXCLUDED.max_employees`,
			p.code, p.nom, p.description, p.prix, p.maxStations, p.maxEmployes)
		if err != nil {
			return err
		}
	}
	return nil
}

// codesAleatoiresInvitation : alphabet sans 0/O/1/I/L (docs/api.md).
const alphabetCode = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func genererCode(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabetCode[rand.Intn(len(alphabetCode))]
	}
	return string(b)
}

// --- Aides HTTP -------------------------------------------------------------

func idSession(sess *sessionUtilisateur) string { return sess.UserID }

func ecrireInterne(w http.ResponseWriter, err error) {
	log.Printf("erreur interne : %v", err)
	repondreErreur(w, http.StatusInternalServerError, "internal", "Le serveur n'a pas pu traiter la demande.")
}

var errAbsente = errors.New("absent")
