package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Tests de bout en bout.
//
// Ils exécutent la vraie API (les mêmes routes, les mêmes requêtes SQL) sur une
// base PostgreSQL jetable, et rejouent les garanties qui coûtent cher en
// production : cloisonnement entre entreprises, permissions par rôle, prix
// venant du catalogue, règles de caisse.
//
// Préparation : la variable DATABASE_URL (ou WASHGUARD_TEST_DATABASE_URL) doit
// pointer vers une base sur laquelle on peut créer un schéma temporaire. Les
// tests tournent chacun dans leur propre schéma, supprimé à la fin : votre base
// de développement n'est ni lue ni modifiée.
//
//   DATABASE_URL=postgresql://… go test ./…
//
// Sans DATABASE_URL, les tests sont ignorés (go test reste vert sur une
// machine sans PostgreSQL, par exemple en intégration continue minimale).
// ---------------------------------------------------------------------------

type banc struct {
	t       *testing.T
	serveur *httptest.Server
	pool    *pgxpool.Pool
	schema  string
	// app : la même instance que celle servie par le banc, pour appeler
	// directement une routine du serveur quand le HTTP ne le permet pas
	// (vérification d'une mention administrative, réservée au support).
	app *App
}

// dsnTest lit la chaîne de connexion de test, en distinguant les deux noms.
func dsnTest() string {
	if dsn := os.Getenv("WASHGUARD_TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return os.Getenv("DATABASE_URL")
}

// nouveauBanc prépare un schéma isolé, applique les migrations, démarre l'API.
func nouveauBanc(t *testing.T) *banc {
	t.Helper()
	dsn := dsnTest()
	if dsn == "" {
		t.Skip("DATABASE_URL absent : tests de bout en bout ignorés")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("test_%d_%s", time.Now().UnixNano()%1_000_000, strings.ToLower(randomSuffix(t)))

	// 1. Connexion « neutre » pour créer le schéma du test.
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("PostgreSQL injoignable (%v) : tests de bout en bout ignorés", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("PostgreSQL injoignable (%v) : tests de bout en bout ignorés", err)
	}
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatalf("création du schéma de test : %v", err)
	}
	admin.Close()

	// 2. Pool du test : toutes ses connexions travaillent dans ce schéma.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("analyse de DATABASE_URL : %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connexion à la base de test : %v", err)
	}

	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("migrations : %v", err)
	}

	app := &App{
		Pool: pool,
		Cfg: Config{
			Env:                   "test",
			SessionCookieSameSite: "lax",
			SessionTTLHours:       1,
			AuthMaxFailures:       10_000,
			RegisterMaxPerHour:    10_000,
			PlatformAdminPassword: "test1234",
		},
	}
	configureRateLimits(app.Cfg)

	// Le banc démarre comme la production : référentiels amorcés compris.
	if err := app.preparerReferentiels(ctx); err != nil {
		pool.Close()
		t.Fatalf("amorçage des référentiels : %v", err)
	}

	serveur := httptest.NewServer(app.buildRouter())
	b := &banc{t: t, serveur: serveur, pool: pool, schema: schema, app: app}
	t.Cleanup(func() {
		serveur.Close()
		pool.Close()
		// Nettoyage : on repart d'une base sans trace du test.
		if admin, err := pgxpool.New(context.Background(), dsn); err == nil {
			_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
			admin.Close()
		}
	})
	return b
}

// randomSuffix : six lettres tirées au hasard pour nommer le schéma du test.
func randomSuffix(t *testing.T) string {
	t.Helper()
	const lettres = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(lettres))))
		if err != nil {
			t.Fatalf("tirage aléatoire : %v", err)
		}
		b[i] = lettres[n.Int64()]
	}
	return string(b)
}

// --- Client HTTP avec session ---------------------------------------------

type client struct {
	t      *testing.T
	base   string
	http   *http.Client
	me     map[string]any
	role   string
	orgID  string
	statID string
	// Identifiants du compte : conservés pour rejouer une connexion (le test
	// de reprise de l'onboarding se déconnecte puis se reconnecte).
	email    string
	password string
}

func (b *banc) client() *client {
	jar, err := cookiejar.New(nil)
	if err != nil {
		b.t.Fatalf("création du pot à cookies : %v", err)
	}
	return &client{t: b.t, base: b.serveur.URL + "/api/v1", http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// appel envoie une requête et renvoie le code HTTP et la réponse décodée.
func (c *client) appel(methode, chemin string, corps any) (int, map[string]any) {
	c.t.Helper()
	var lecteur io.Reader
	if corps != nil {
		donnees, err := json.Marshal(corps)
		if err != nil {
			c.t.Fatalf("encodage du corps : %v", err)
		}
		lecteur = bytes.NewReader(donnees)
	}
	req, err := http.NewRequest(methode, c.base+chemin, lecteur)
	if err != nil {
		c.t.Fatalf("construction de la requête : %v", err)
	}
	if corps != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rep, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s : %v", methode, chemin, err)
	}
	defer rep.Body.Close()
	brut, _ := io.ReadAll(rep.Body)

	var decode any
	if len(brut) > 0 {
		_ = json.Unmarshal(brut, &decode)
	}
	switch valeur := decode.(type) {
	case map[string]any:
		return rep.StatusCode, valeur
	case []any:
		return rep.StatusCode, map[string]any{"_tableau": valeur}
	default:
		return rep.StatusCode, map[string]any{"_brut": string(brut)}
	}
}

// tableau relit une réponse qui est un tableau JSON (ex. GET /stations).
func (c *client) tableau(methode, chemin string, corps any) (int, []any) {
	c.t.Helper()
	code, rep := c.appel(methode, chemin, corps)
	if liste, ok := rep["_tableau"].([]any); ok {
		return code, liste
	}
	return code, nil
}

// inscrire crée une entreprise et garde la session ouverte.
func (c *client) inscrire(nom string) {
	c.t.Helper()
	email := fmt.Sprintf("%s-%d@test.local", strings.ReplaceAll(strings.ToLower(nom), " ", "-"), time.Now().UnixNano())
	code, rep := c.appel("POST", "/auth/register", map[string]any{
		"organization_name": nom,
		"full_name":         "Propriétaire " + nom,
		"email":             email,
		"password":          "motdepasse1",
		"station_name":      "Station " + nom,
	})
	c.email, c.password = email, "motdepasse1"
	if code != 201 {
		c.t.Fatalf("inscription de %q : http=%d %v", nom, code, rep)
	}
	c.me = rep
	if user, ok := rep["user"].(map[string]any); ok {
		c.role, _ = user["role"].(string)
		c.orgID, _ = user["organization_id"].(string)
		c.statID, _ = user["station_id"].(string)
	}
}

// remplirProfilEntreprise complète l'identité minimale de l'entreprise
// (téléphone et ville). C'est ce que l'étape « entreprise » de l'onboarding
// exige, et l'inscription ne le demande pas.
func (c *client) remplirProfilEntreprise() {
	c.t.Helper()
	code, rep := c.appel("PATCH", "/organizations/current", map[string]any{
		"phone": "+225 07 00 00 00 00",
		"city":  "Abidjan",
	})
	if code != 200 {
		c.t.Fatalf("identité de l'entreprise : http=%d %v", code, rep)
	}
}

// declarerHoraires ouvre la semaine de la première station : sans au moins une
// journée d'ouverture, l'activation est refusée (le statut « ouvert / fermé »
// n'aurait rien sur quoi s'appuyer).
func (c *client) declarerHoraires() {
	c.t.Helper()
	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		c.t.Fatalf("station introuvable pour les horaires : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	jours := []map[string]any{}
	for jour := 1; jour <= 7; jour++ {
		horaire := map[string]any{"weekday": jour, "is_open": jour <= 6, "booking_allowed": true}
		if jour <= 6 {
			horaire["opens_at"] = "08:00"
			horaire["closes_at"] = "18:00"
		}
		jours = append(jours, horaire)
	}
	code, rep := c.appel("PUT", "/stations/"+texte(premiere, "id")+"/opening-hours",
		map[string]any{"days": jours, "exceptions": []map[string]any{}})
	if code != 200 {
		c.t.Fatalf("enregistrement des horaires : http=%d %v", code, rep)
	}
}

// assurerPrestationTarifee crée une prestation si le catalogue est vide.
// Aucun prix n'est inventé par l'application : c'est le test qui décide du
// sien, comme un exploitant qui saisit son tarif.
func (c *client) assurerPrestationTarifee() {
	c.t.Helper()
	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		c.t.Fatalf("station introuvable pour le catalogue : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	station := texte(premiere, "id")

	code, catalogue := c.appel("GET", "/services?station_id="+station, nil)
	if code != 200 {
		c.t.Fatalf("GET /services : http=%d %v", code, catalogue)
	}
	items, _ := catalogue["items"].([]any)
	for _, brut := range items {
		prestation, _ := brut.(map[string]any)
		if entier(prestation["price_xof"]) > 0 {
			return // il y a déjà une prestation tarifée
		}
	}
	code, cree := c.appel("POST", "/services", map[string]any{
		"station_id": station, "name": "Lavage de test", "duration_minutes": 30,
	})
	if code != 201 {
		c.t.Fatalf("création de la prestation : http=%d %v", code, cree)
	}
	c.tariferCatalogue(station, texte(cree, "id"), 3000)
}

// tariferCatalogue pose le tarif de repli d'une prestation.
//
// Depuis le lot 5, la grille tarifaire est la **seule** route qui écrit un
// prix de vente : elle exige un motif et laisse une trace d'audit. Le banc
// passe donc par elle, comme l'écran des tarifs.
func (c *client) tariferCatalogue(station, serviceID string, prix int) {
	c.t.Helper()
	code, rep := c.appel("PUT", "/price-rules/matrix", map[string]any{
		"station_id": station, "reason": "Grille du banc de test",
		"generic_prices": []map[string]any{{"service_id": serviceID, "price_xof": prix}},
	})
	if code != 200 {
		c.t.Fatalf("tarification par la grille : http=%d %v", code, rep)
	}
}

// declarerUsages active des usages commerciaux et les couples (usage, véhicule)
// que la station reconnaît — « taxi_compteur:berline ». Un usage sans couple
// n'est proposable à aucune fiche : la caisse refuserait la combinaison, et une
// règle de prix par usage ne pourrait pas être posée.
func (c *client) declarerUsages(couples ...string) {
	c.t.Helper()
	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		c.t.Fatalf("station introuvable pour les usages : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	station := texte(premiere, "id")

	usages := []map[string]any{}
	vus := map[string]bool{}
	corps := []map[string]any{}
	for _, couple := range couples {
		parties := strings.SplitN(couple, ":", 2)
		if len(parties) != 2 || parties[0] == "" || parties[1] == "" {
			c.t.Fatalf("couple usage:catégorie attendu, reçu %q", couple)
		}
		if !vus[parties[0]] {
			vus[parties[0]] = true
			usages = append(usages, map[string]any{
				"code": parties[0], "is_active": true, "sort_order": len(usages)*10 + 10,
			})
		}
		corps = append(corps, map[string]any{"usage_code": parties[0], "category_code": parties[1]})
	}
	if len(usages) > 0 {
		code, rep := c.appel("PUT", "/stations/"+station+"/vehicle-usages",
			map[string]any{"usages": usages})
		if code != 200 {
			c.t.Fatalf("activation des usages : http=%d %v", code, rep)
		}
	}
	if len(corps) > 0 {
		code, rep := c.appel("PUT", "/stations/"+station+"/vehicle-usage-categories",
			map[string]any{"couples": corps})
		if code != 200 {
			c.t.Fatalf("déclaration des couples véhicule × usage : http=%d %v", code, rep)
		}
	}
}

// declarerCategories active les catégories de véhicules du référentiel dont la
// station a besoin pour vendre. Depuis le lot 4, une station ne s'active pas
// sans avoir dit ce qu'elle lave : la catégorie est un prérequis (ONB-8.2).
func (c *client) declarerCategories(codes ...string) {
	c.t.Helper()
	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		c.t.Fatalf("station introuvable pour les catégories : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	station := texte(premiere, "id")
	if len(codes) == 0 {
		codes = []string{"berline", "moto", "minibus"}
	}
	corps := []map[string]any{}
	for index, reference := range codes {
		corps = append(corps, map[string]any{
			"code": reference, "is_active": true, "sort_order": (index + 1) * 10,
		})
	}
	code, rep := c.appel("PUT", "/stations/"+station+"/vehicle-categories",
		map[string]any{"categories": corps})
	if code != 200 {
		c.t.Fatalf("activation des catégories %v : http=%d %v", codes, code, rep)
	}
}

// activer termine l'onboarding puis active l'espace : c'est l'état de départ des
// tests qui portent sur l'exploitation, puisqu'une station non activée ne vend
// pas (requireOnboarding). Les tests d'onboarding, eux, ne l'appellent pas —
// ils vérifient justement ce que l'activation exige.
func (c *client) activer() {
	c.t.Helper()
	c.remplirProfilEntreprise()
	c.declarerHoraires()
	c.declarerCategories()
	c.assurerPrestationTarifee()
	code, rep := c.appel("POST", "/onboarding/activate", nil)
	if code != 200 {
		c.t.Fatalf("activation de l'espace : http=%d %v", code, rep)
	}
	if etat := texte(rep, "state"); etat != "active" {
		c.t.Fatalf("état après activation : %q (attendu « active »)", etat)
	}
}

// etapeOnboarding retrouve une étape par son code dans la réponse de l'API.
func etapeOnboarding(rep map[string]any, code string) map[string]any {
	etapes, _ := rep["steps"].([]any)
	for _, brut := range etapes {
		etape, _ := brut.(map[string]any)
		if texte(etape, "code") == code {
			return etape
		}
	}
	return nil
}

// codesManquants liste les codes des étapes obligatoires non satisfaites.
func codesManquants(rep map[string]any) []string {
	manquants, _ := rep["missing"].([]any)
	codes := make([]string, 0, len(manquants))
	for _, brut := range manquants {
		item, _ := brut.(map[string]any)
		codes = append(codes, texte(item, "code"))
	}
	return codes
}

// champsErreur relit error.fields (les étapes à compléter dans une réponse 409).
// contient : le tableau contient-il cette chaîne ? (lecture des listes de codes)
func contient(liste []string, valeur string) bool {
	for _, item := range liste {
		if item == valeur {
			return true
		}
	}
	return false
}

func champsErreur(rep map[string]any) map[string]any {
	if err, ok := rep["error"].(map[string]any); ok {
		if champs, ok := err["fields"].(map[string]any); ok {
			return champs
		}
	}
	return nil
}

// --- Aides de lecture -----------------------------------------------------

func entier(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func texte(rep map[string]any, cle string) string {
	if v, ok := rep[cle].(string); ok {
		return v
	}
	return ""
}

// sousMap : l'objet imbriqué d'une réponse (`user`, `station`…), ou une carte
// vide — un test qui lit un champ absent doit échouer sur la valeur, pas sur un
// nil déréférencé.
func sousMap(rep map[string]any, cle string) map[string]any {
	if objet, ok := rep[cle].(map[string]any); ok {
		return objet
	}
	return map[string]any{}
}

func codeErreur(rep map[string]any) string {
	if err, ok := rep["error"].(map[string]any); ok {
		return texte(err, "code")
	}
	return ""
}

func (b *banc) creerStationEtPrestation(c *client) (station, service string, prix int) {
	b.t.Helper()
	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		b.t.Fatalf("GET /stations : http=%d, %d station(s)", code, len(stations))
	}
	premiere, _ := stations[0].(map[string]any)
	station = texte(premiere, "id")

	code, catalogue := c.appel("GET", "/services?station_id="+station, nil)
	if code != 200 {
		b.t.Fatalf("GET /services : http=%d %v", code, catalogue)
	}
	items, _ := catalogue["items"].([]any)

	// Depuis le lot 5, un compte neuf reçoit les modèles de référence **sans
	// prix** : le banc retient la prestation qu'il a tarifée, sinon il en
	// déclare une — comme le ferait le gérant dans l'écran des tarifs.
	for _, brut := range items {
		prestation, _ := brut.(map[string]any)
		if entier(prestation["price_xof"]) > 0 {
			return station, texte(prestation, "id"), entier(prestation["price_xof"])
		}
	}
	code, cree := c.appel("POST", "/services", map[string]any{
		"station_id": station, "name": "Lavage de test", "duration_minutes": 30,
	})
	if code != 201 {
		b.t.Fatalf("création de la prestation de test : http=%d %v", code, cree)
	}
	c.tariferCatalogue(station, texte(cree, "id"), 3000)
	return station, texte(cree, "id"), 3000
}

// ===========================================================================
// 1. Inscription et catalogue
// ===========================================================================

func TestInscriptionCreeEspaceEtSession(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Initiale")

	code, rep := c.appel("GET", "/auth/me", nil)
	if code != 200 {
		t.Fatalf("la session n'est pas ouverte après l'inscription : http=%d %v", code, rep)
	}

	// Aucun prix n'est inventé à l'inscription : le catalogue est vide, et le
	// parcours de mise en route demande les prestations et leurs tarifs.
	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		t.Fatalf("la station de l'entreprise doit exister : http=%d %v", code, stations)
	}
	premiere, _ := stations[0].(map[string]any)
	code, catalogue := c.appel("GET", "/services?station_id="+texte(premiere, "id"), nil)
	if code != 200 {
		t.Fatalf("GET /services : http=%d", code)
	}
	items, _ := catalogue["items"].([]any)
	// Le compte neuf reçoit les modèles de référence — décrits, jamais tarifés.
	if len(items) == 0 {
		t.Error("le catalogue d'un compte neuf doit proposer les prestations de référence")
	}
	surDevis := 0
	for _, brut := range items {
		prestation, _ := brut.(map[string]any)
		if entier(prestation["price_xof"]) != 0 {
			t.Errorf("aucun prix ne doit être inventé : %v", prestation)
		}
		if gratuit, _ := prestation["is_free"].(bool); gratuit {
			t.Errorf("gratuit est une décision de l'exploitant, pas un défaut : %v", prestation)
		}
		if devis, _ := prestation["is_quote_only"].(bool); devis {
			surDevis++
		}
	}
	// « À tarifer » veut dire « il reste un prix à saisir » : une prestation
	// sur devis se chiffre au cas par cas et n'a pas de prix de comptoir à
	// poser. Le catalogue de référence en contient une (le traitement
	// céramique) ; les autres attendent leur tarif.
	if aTarifer := entier(catalogue["a_tarifer"]); aTarifer != len(items)-surDevis {
		t.Errorf("les prestations vendables d'un compte neuf restent à tarifer : %d/%d (%d sur devis)",
			aTarifer, len(items)-surDevis, surDevis)
	}

	// Le parcours le dit : le catalogue est là (il reste à le tarifer), et
	// « tarifs » comme « horaires » restent à faire.
	code, parcours := c.appel("GET", "/onboarding", nil)
	if code != 200 {
		t.Fatalf("GET /onboarding : http=%d", code)
	}
	if point := etapeOnboarding(parcours, "prestations"); point == nil || point["done"] != true {
		t.Errorf("le catalogue de référence doit satisfaire l'étape « prestations » : %v", point)
	}
	for _, etape := range []string{"tarifs", "horaires"} {
		point := etapeOnboarding(parcours, etape)
		if point == nil || point["done"] == true {
			t.Errorf("l'étape « %s » doit rester à faire sur un compte neuf : %v", etape, point)
		}
	}
	if activable, _ := parcours["can_activate"].(bool); activable {
		t.Error("un compte neuf ne doit pas pouvoir être activé")
	}
}

// ===========================================================================
// 2. Cloisonnement entre entreprises
// ===========================================================================

func TestIsolationEntreEntreprises(t *testing.T) {
	b := nouveauBanc(t)
	visiteuse := b.client()
	visiteuse.inscrire("Entreprise A")
	visiteuse.activer()
	station, service, _ := b.creerStationEtPrestation(visiteuse)

	code, fiche := visiteuse.appel("POST", "/wash-orders", map[string]any{
		"station_id":    station,
		"customer_name": "Client A",
		"items":         []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	ficheID := texte(fiche, "id")

	intruse := b.client()
	intruse.inscrire("Entreprise B")
	intruse.activer()

	// Lecture : 404 (et non 403) pour ne pas révéler l'existence de la fiche.
	code, rep := intruse.appel("GET", "/wash-orders/"+ficheID, nil)
	if code != 404 {
		t.Errorf("une autre entreprise peut lire la fiche : http=%d %v", code, rep)
	}
	code, rep = intruse.appel("PATCH", "/wash-orders/"+ficheID, map[string]any{"notes": "modifiée"})
	if code != 404 {
		t.Errorf("une autre entreprise peut modifier la fiche : http=%d %v", code, rep)
	}
	code, rep = intruse.appel("GET", "/wash-orders/"+ficheID, nil)
	if code == 200 {
		t.Errorf("la fiche reste lisible : %v", rep)
	}

	// Les lignes de prestation portent l'entreprise : la création par l'intruse
	// avec la prestation de A doit être refusée (prix et catalogue de A).
	_, stationB, _ := b.creerStationEtPrestation(intruse)
	code, rep = intruse.appel("POST", "/wash-orders", map[string]any{
		"station_id": stationB,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code == 201 {
		t.Errorf("une prestation d'une autre entreprise a été acceptée : %v", rep)
	}
}

// ===========================================================================
// 3. Permissions par rôle
// ===========================================================================

func TestPermissionsParRole(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Roles")
	patron.activer()
	station, service, _ := b.creerStationEtPrestation(patron)

	// Le patron crée un compte caissier et relève le mot de passe provisoire.
	code, employe := patron.appel("POST", "/employees", map[string]any{
		"full_name":  "Caissière Test",
		"email":      fmt.Sprintf("caissiere-%d@test.local", time.Now().UnixNano()),
		"role":       "cashier",
		"station_id": station,
	})
	if code != 201 {
		t.Fatalf("création du caissier : http=%d %v", code, employe)
	}
	provisoire := texte(employe, "temporary_password")
	if provisoire == "" {
		t.Fatalf("aucun mot de passe provisoire renvoyé : %v", employe)
	}

	caissier := b.client()
	code, connexion := caissier.appel("POST", "/auth/login", map[string]any{
		"email":    texte(employe, "email"),
		"password": provisoire,
	})
	if code != 200 {
		t.Fatalf("connexion du caissier : http=%d %v", code, connexion)
	}
	if doitChanger, _ := connexion["must_change_password"].(bool); !doitChanger {
		t.Errorf("le mot de passe provisoire doit être signalé comme à changer")
	}

	// Le caissier crée une fiche, puis tente ce qui ne lui appartient pas.
	code, fiche := caissier.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("le caissier doit pouvoir enregistrer un lavage : http=%d %v", code, fiche)
	}

	code, rep := caissier.appel("POST", "/wash-orders/"+texte(fiche, "id")+"/cancel", map[string]any{"reason": "test"})
	if code != 403 {
		t.Errorf("le caissier ne doit pas pouvoir annuler une fiche : http=%d %v", code, rep)
	}
	code, rep = caissier.appel("GET", "/platform/overview", nil)
	if code != 403 {
		t.Errorf("l'espace plateforme doit être refusé à un caissier : http=%d %v", code, rep)
	}
	code, rep = caissier.appel("GET", "/reports/overview", nil)
	if code != 403 {
		t.Errorf("les rapports doivent être refusés à un caissier : http=%d %v", code, rep)
	}
	code, rep = caissier.appel("PATCH", "/organizations/current", map[string]any{"name": "Renommée"})
	if code != 403 {
		t.Errorf("un caissier ne doit pas renommer l'entreprise : http=%d %v", code, rep)
	}
}

// ===========================================================================
// 4. Prix du catalogue
// ===========================================================================

func TestPrixVientDuCatalogue(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Prix")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)

	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items": []map[string]any{{
			"service_id":     service,
			"quantity":       1,
			"unit_price_xof": 1, // tentative de payer 1 FCFA
		}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	if total := entier(fiche["total_xof"]); total != prix {
		t.Errorf("le total doit venir du catalogue (%d), obtenu %d", prix, total)
	}
}

// ===========================================================================
// 5. Caisse et encaissement
// ===========================================================================

func TestCaisseEtEncaissement(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Caisse")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)

	// Encaisser sans session de caisse ouverte doit être refusé en espèces.
	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	ficheID := texte(fiche, "id")
	code, rep := c.appel("POST", "/wash-orders/"+ficheID+"/payments", map[string]any{
		"amount_xof": prix, "method": "especes",
	})
	if code == 200 {
		t.Errorf("un encaissement en espèces sans caisse ouverte a été accepté : %v", rep)
	}

	// Ouverture de la caisse, puis encaissement complet : la fiche est close.
	code, session := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 20_000,
	})
	if code != 201 {
		t.Fatalf("ouverture de caisse : http=%d %v", code, session)
	}
	code, rep = c.appel("POST", "/wash-orders/"+ficheID+"/payments", map[string]any{
		"amount_xof": prix, "method": "especes",
	})
	if code != 200 {
		t.Fatalf("encaissement : http=%d %v", code, rep)
	}
	commande, _ := rep["order"].(map[string]any)
	if statut := texte(commande, "status"); statut != "completed" {
		t.Errorf("la fiche soldée doit être terminée, statut obtenu %q", statut)
	}

	// Surpaiement sur une seconde fiche : refusé.
	code, fiche2 := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la seconde fiche : http=%d %v", code, fiche2)
	}
	code, rep = c.appel("POST", "/wash-orders/"+texte(fiche2, "id")+"/payments", map[string]any{
		"amount_xof": prix * 3, "method": "especes",
	})
	if code != 400 {
		t.Errorf("un surpaiement doit être refusé (400), obtenu http=%d %v", code, rep)
	}
}

// ===========================================================================
// 6. Une fiche encaissée ne se réécrit plus
// ===========================================================================

func TestFicheEncaisseeVerrouillee(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Verrou")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)

	code, _ := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 20_000,
	})
	if code != 201 {
		t.Fatalf("ouverture de caisse : http=%d", code)
	}

	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	ficheID := texte(fiche, "id")
	code, _ = c.appel("POST", "/wash-orders/"+ficheID+"/payments", map[string]any{
		"amount_xof": prix, "method": "especes",
	})
	if code != 200 {
		t.Fatalf("encaissement : http=%d", code)
	}

	code, rep := c.appel("PATCH", "/wash-orders/"+ficheID, map[string]any{
		"discount_xof": 500, "discount_reason": "remise après coup",
	})
	if code != 409 {
		t.Errorf("une remise après encaissement doit être refusée (409), obtenu http=%d %v", code, rep)
	}
	code, rep = c.appel("PATCH", "/wash-orders/"+ficheID, map[string]any{"status": "pending"})
	if code != 409 {
		t.Errorf("un retour en atelier doit être refusé (409), obtenu http=%d %v", code, rep)
	}
	if code, rep := c.appel("PATCH", "/wash-orders/"+ficheID, map[string]any{"notes": "note ajoutée"}); code != 200 {
		t.Errorf("les champs descriptifs doivent rester modifiables, http=%d %v", code, rep)
	}
}

// ===========================================================================
// 7. Migration : les lignes de fiche portent leur entreprise
// ===========================================================================

func TestLignesDeFichePortentLEntreprise(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Lignes")
	c.activer()
	station, service, _ := b.creerStationEtPrestation(c)

	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}

	var total, rattachees int
	if err := b.pool.QueryRow(context.Background(),
		`SELECT count(*), count(organization_id) FROM wash_order_items`).Scan(&total, &rattachees); err != nil {
		t.Fatalf("lecture des lignes : %v", err)
	}
	if total == 0 {
		t.Fatal("aucune ligne de prestation enregistrée")
	}
	if total != rattachees {
		t.Errorf("%d ligne(s) sur %d sans entreprise", total-rattachees, total)
	}

	var nulles int
	if err := b.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.columns
		  WHERE table_name = 'wash_order_items' AND column_name = 'organization_id' AND is_nullable = 'YES'`).
		Scan(&nulles); err != nil {
		t.Fatalf("lecture du schéma : %v", err)
	}
	if nulles != 0 {
		t.Error("la colonne organization_id devrait être NOT NULL après la migration 0002")
	}
}

// ===========================================================================
// 8. Migrations : rien à rejouer au second passage
// ===========================================================================

func TestMigrationsIdempotentes(t *testing.T) {
	b := nouveauBanc(t)
	ctx := context.Background()

	var avant int
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&avant); err != nil {
		t.Fatalf("lecture de schema_migrations : %v", err)
	}
	if avant < 2 {
		t.Errorf("les migrations doivent être enregistrées (attendu ≥ 2, obtenu %d)", avant)
	}

	// Un second passage ne doit rien appliquer et ne rien casser.
	if err := migrate(ctx, b.pool); err != nil {
		t.Fatalf("second passage des migrations : %v", err)
	}
	var apres int
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&apres); err != nil {
		t.Fatalf("lecture de schema_migrations : %v", err)
	}
	if apres != avant {
		t.Errorf("le second passage a modifié l'historique (%d → %d)", avant, apres)
	}
}

// ===========================================================================
// 9. La migration 0003 ne réécrit pas un choix explicite
// ===========================================================================

func TestReglageCaisseNeRecouvrePasUnChoixExplicite(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Reglage")

	// La station de l'inscription exige une caisse ouverte.
	var valeur *string
	if err := b.pool.QueryRow(context.Background(),
		`SELECT settings->>'require_open_cash_session' FROM stations LIMIT 1`).Scan(&valeur); err != nil {
		t.Fatalf("lecture du réglage : %v", err)
	}
	if valeur == nil || *valeur != "true" {
		t.Fatalf("une station créée à l'inscription doit exiger une caisse ouverte, obtenu %v", valeur)
	}

	// Une station où l'exploitant a désactivé la règle, et une autre à qui la
	// clé manque : après passage de la migration, seul le trou est comblé.
	_, err := b.pool.Exec(context.Background(),
		`UPDATE stations SET settings = '{"require_open_cash_session": false}'::jsonb`)
	if err != nil {
		t.Fatalf("préparation du réglage explicite : %v", err)
	}
	sqlMigration, err := migrationSQL("0003_stations_regle_caisse_par_defaut")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := b.pool.Exec(context.Background(), sqlMigration); err != nil {
		t.Fatalf("application de la migration : %v", err)
	}
	if err := b.pool.QueryRow(context.Background(),
		`SELECT settings->>'require_open_cash_session' FROM stations LIMIT 1`).Scan(&valeur); err != nil {
		t.Fatalf("relecture du réglage : %v", err)
	}
	if valeur == nil || *valeur != "false" {
		t.Errorf("un réglage désactivé explicitement a été écrasé : %v", valeur)
	}

	// Un réglage vide est complété.
	if _, err := b.pool.Exec(context.Background(), `UPDATE stations SET settings = '{}'::jsonb`); err != nil {
		t.Fatalf("remise à zéro des réglages : %v", err)
	}
	if _, err := b.pool.Exec(context.Background(), sqlMigration); err != nil {
		t.Fatalf("seconde application de la migration : %v", err)
	}
	if err := b.pool.QueryRow(context.Background(),
		`SELECT settings->>'require_open_cash_session' FROM stations LIMIT 1`).Scan(&valeur); err != nil {
		t.Fatalf("relecture du réglage : %v", err)
	}
	if valeur == nil || *valeur != "true" {
		t.Errorf("un réglage absent doit être complété, obtenu %v", valeur)
	}
}

// migrationSQL rend le contenu d'un fichier de migration embarqué.
func migrationSQL(version string) (string, error) {
	liste, err := loadMigrations()
	if err != nil {
		return "", err
	}
	for _, m := range liste {
		if m.version == version {
			return m.sql, nil
		}
	}
	return "", fmt.Errorf("migration %s introuvable", version)
}

// ===========================================================================
// 10. Périmètre d'une station
// ===========================================================================

func TestPerimetreDuneStation(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Perimetre")
	patron.activer()
	stationA, service, _ := b.creerStationEtPrestation(patron)

	// Une seconde station. La formule d'essai ne permettant qu'une station
	// active, elle est posée directement en base : ce test vérifie le
	// périmètre de station, pas les plafonds de la formule.
	idB := "st-perimetre-deuxieme"
	if _, err := b.pool.Exec(context.Background(),
		`INSERT INTO stations (id, organization_id, name, code, settings)
		 VALUES ($1, $2, 'Station Deuxième', 'ST2', $3::jsonb)`,
		idB, patron.orgID, defaultStationSettings(0)); err != nil {
		t.Fatalf("création de la seconde station : %v", err)
	}

	// Un caissier rattaché à la première station.
	code, employe := patron.appel("POST", "/employees", map[string]any{
		"full_name":  "Caissière Périmètre",
		"email":      fmt.Sprintf("perimetre-%d@test.local", time.Now().UnixNano()),
		"role":       "cashier",
		"station_id": stationA,
	})
	if code != 201 {
		t.Fatalf("création du caissier : http=%d %v", code, employe)
	}
	caissier := b.client()
	code, _ = caissier.appel("POST", "/auth/login", map[string]any{
		"email": texte(employe, "email"), "password": texte(employe, "temporary_password"),
	})
	if code != 200 {
		t.Fatalf("connexion du caissier : http=%d", code)
	}

	// Il travaille normalement dans sa station…
	code, fiche := caissier.appel("POST", "/wash-orders", map[string]any{
		"station_id": stationA,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("lavage dans sa propre station : http=%d %v", code, fiche)
	}

	// … et se heurte à l'autre : création, lecture, dépense, caisse.
	code, rep := caissier.appel("POST", "/wash-orders", map[string]any{
		"station_id": idB,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code == 201 {
		t.Errorf("un caissier a enregistré un lavage dans une autre station : %v", rep)
	}
	code, rep = caissier.appel("POST", "/expenses", map[string]any{
		"station_id": idB, "label": "Carburant", "amount_xof": 2_000,
	})
	if code == 201 {
		t.Errorf("un caissier a imputé une dépense dans une autre station : %v", rep)
	}
	code, rep = caissier.appel("POST", "/cash/sessions", map[string]any{
		"station_id": idB, "opening_float_xof": 10_000,
	})
	if code == 201 {
		t.Errorf("un caissier a ouvert une caisse dans une autre station : %v", rep)
	}
	// Sa liste ne contient que sa station.
	code, liste := caissier.tableau("GET", "/wash-orders", nil)
	if code != 200 {
		t.Fatalf("liste des lavages : http=%d", code)
	}
	for _, ligne := range liste {
		fiche, _ := ligne.(map[string]any)
		if station := texte(fiche, "station_id"); station != "" && station != stationA {
			t.Errorf("le caissier voit une fiche de la station %s", station)
		}
	}
}

// ===========================================================================
// 11. Dépense et rapports
// ===========================================================================

func TestDepenseEtRapports(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Rapports")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)

	// Une journée type : un lavage encaissé en espèces, une dépense de carburant.
	code, _ := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10_000,
	})
	if code != 201 {
		t.Fatalf("ouverture de caisse : http=%d", code)
	}
	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	code, _ = c.appel("POST", "/wash-orders/"+texte(fiche, "id")+"/payments", map[string]any{
		"amount_xof": prix, "method": "especes",
	})
	if code != 200 {
		t.Fatalf("encaissement : http=%d", code)
	}
	code, depense := c.appel("POST", "/expenses", map[string]any{
		"station_id": station, "category": "carburant",
		"label": "Groupe électrogène", "amount_xof": 2_000, "method": "especes",
	})
	if code != 201 {
		t.Fatalf("création de la dépense : http=%d %v", code, depense)
	}

	code, rapport := c.appel("GET", "/reports/overview?station_id="+station, nil)
	if code != 200 {
		t.Fatalf("rapport : http=%d %v", code, rapport)
	}
	totaux, _ := rapport["totals"].(map[string]any)
	if totaux == nil {
		t.Fatalf("rapport sans totaux : %v", rapport)
	}
	if v := entier(totaux["wash_count"]); v != 1 {
		t.Errorf("lavages terminés : attendu 1, obtenu %d", v)
	}
	if v := entier(totaux["revenue_xof"]); v != prix {
		t.Errorf("chiffre d'affaires : attendu %d, obtenu %d", prix, v)
	}
	if v := entier(totaux["expenses_xof"]); v != 2_000 {
		t.Errorf("dépenses : attendu 2000, obtenu %d", v)
	}
	if v := entier(totaux["result_xof"]); v != prix-2_000 {
		t.Errorf("résultat : attendu %d, obtenu %d", prix-2_000, v)
	}

	// Le tableau de bord du jour doit refléter les mêmes montants.
	code, tableau := c.appel("GET", "/reports/dashboard?station_id="+station, nil)
	if code != 200 {
		t.Fatalf("tableau de bord : http=%d %v", code, tableau)
	}
	jour, _ := tableau["today"].(map[string]any)
	if v := entier(jour["revenue_xof"]); v != prix {
		t.Errorf("chiffre d'affaires du tableau de bord : attendu %d, obtenu %d", prix, v)
	}
}

// ===========================================================================
// 12. Clôture de caisse et écart de comptage
// ===========================================================================

func TestClotureDeCaisse(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Cloture")
	c.activer()
	station, service, prix := b.creerStationEtPrestation(c)

	code, session := c.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10_000,
	})
	if code != 201 {
		t.Fatalf("ouverture de caisse : http=%d %v", code, session)
	}
	sessionID := texte(session, "id")

	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d", code)
	}
	code, _ = c.appel("POST", "/wash-orders/"+texte(fiche, "id")+"/payments", map[string]any{
		"amount_xof": prix, "method": "especes",
	})
	if code != 200 {
		t.Fatalf("encaissement : http=%d", code)
	}

	// Le comptage réel est supérieur de 500 FCFA : l'écart doit être calculé.
	attendu := 10_000 + prix
	code, cloture := c.appel("POST", "/cash/sessions/"+sessionID+"/close", map[string]any{
		"counted_cash_xof": attendu + 500, "note": "billet supplémentaire",
	})
	if code != 200 {
		t.Fatalf("clôture : http=%d %v", code, cloture)
	}
	if statut := texte(cloture, "status"); statut != "closed" {
		t.Errorf("la session doit être clôturée, statut obtenu %q", statut)
	}
	if v := entier(cloture["difference_xof"]); v != 500 {
		t.Errorf("écart de caisse : attendu 500, obtenu %d (%v)", v, cloture)
	}

	// Une session clôturée ne se clôture pas deux fois.
	code, rep := c.appel("POST", "/cash/sessions/"+sessionID+"/close", map[string]any{
		"counted_cash_xof": attendu,
	})
	if code != 409 {
		t.Errorf("seconde clôture : attendu 409, obtenu http=%d %v", code, rep)
	}

	// Sans session ouverte, une nouvelle fiche ne s'encaisse plus en espèces.
	code, fiche2 := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la seconde fiche : http=%d", code)
	}
	code, rep = c.appel("POST", "/wash-orders/"+texte(fiche2, "id")+"/payments", map[string]any{
		"amount_xof": prix, "method": "especes",
	})
	if code != 409 {
		t.Errorf("encaissement après clôture : attendu 409, obtenu http=%d %v", code, rep)
	}

	// Rouvrir une caisse clôturée est réservé au propriétaire ou au gérant.
	code, rep = c.appel("POST", "/cash/sessions/"+sessionID+"/reopen", map[string]any{
		"reason": "erreur de comptage",
	})
	if code != 200 {
		t.Errorf("le propriétaire doit pouvoir rouvrir la caisse : http=%d %v", code, rep)
	}
}

// ===========================================================================
// 13. Commissions des laveurs
// ===========================================================================

func TestCommissionsDesLaveurs(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Commissions")
	patron.activer()
	station, service, prix := b.creerStationEtPrestation(patron)

	// Un laveur, et une caisse ouverte pour encaisser.
	code, laveur := patron.appel("POST", "/employees", map[string]any{
		"full_name":  "Laveur Test",
		"email":      fmt.Sprintf("laveur-%d@test.local", time.Now().UnixNano()),
		"role":       "washer",
		"station_id": station,
	})
	if code != 201 {
		t.Fatalf("création du laveur : http=%d %v", code, laveur)
	}
	laveurID := texte(laveur, "id")
	code, _ = patron.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10_000,
	})
	if code != 201 {
		t.Fatalf("ouverture de caisse : http=%d", code)
	}

	// Lavage terminé et attribué : 500 FCFA de commission (barème de la station).
	code, fiche := patron.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d", code)
	}
	ficheID := texte(fiche, "id")
	code, rep := patron.appel("POST", "/wash-orders/"+ficheID+"/assign", map[string]any{"employee_id": laveurID})
	if code != 200 {
		t.Fatalf("affectation du laveur : http=%d %v", code, rep)
	}
	code, _ = patron.appel("POST", "/wash-orders/"+ficheID+"/payments", map[string]any{
		"amount_xof": prix, "method": "especes",
	})
	if code != 200 {
		t.Fatalf("encaissement : http=%d", code)
	}

	// Lavage annulé : il ne compte pas dans les commissions.
	code, annulee := patron.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche annulée : http=%d", code)
	}
	code, rep = patron.appel("POST", "/wash-orders/"+texte(annulee, "id")+"/cancel", map[string]any{"reason": "client parti"})
	if code != 200 {
		t.Fatalf("annulation : http=%d %v", code, rep)
	}

	code, etat := patron.appel("GET", "/commissions?station_id="+station, nil)
	if code != 200 {
		t.Fatalf("état de paie : http=%d %v", code, etat)
	}
	items, _ := etat["items"].([]any)
	var trouve bool
	for _, ligne := range items {
		item, _ := ligne.(map[string]any)
		if texte(item, "employee_id") != laveurID {
			continue
		}
		trouve = true
		if v := entier(item["wash_count"]); v != 1 {
			t.Errorf("lavages comptés pour le laveur : attendu 1 (annulé exclu), obtenu %d", v)
		}
		if v := entier(item["commission_xof"]); v != 500 {
			t.Errorf("commission du laveur : attendue 500, obtenue %d", v)
		}
	}
	if !trouve {
		t.Errorf("le laveur est absent de l'état de paie : %v", items)
	}
	totaux, _ := etat["totals"].(map[string]any)
	if v := entier(totaux["commission_xof"]); v != 500 {
		t.Errorf("total des commissions : attendu 500, obtenu %d", v)
	}

	// Le laveur ne voit pas l'état de paie de l'entreprise.
	travailleur := b.client()
	code, _ = travailleur.appel("POST", "/auth/login", map[string]any{
		"email": texte(laveur, "email"), "password": texte(laveur, "temporary_password"),
	})
	if code != 200 {
		t.Fatalf("connexion du laveur : http=%d", code)
	}
	code, rep = travailleur.appel("GET", "/commissions", nil)
	if code != 403 {
		t.Errorf("un laveur ne doit pas voir les commissions : http=%d %v", code, rep)
	}
}

// ===========================================================================
// 14. Validation des entrées
// ===========================================================================

func TestValidationDesEntrees(t *testing.T) {
	b := nouveauBanc(t)

	// Inscription : mot de passe trop court et champs manquants.
	anonyme := b.client()
	code, rep := anonyme.appel("POST", "/auth/register", map[string]any{
		"organization_name": "Entreprise Motdepasse",
		"full_name":         "Patron",
		"email":             fmt.Sprintf("court-%d@test.local", time.Now().UnixNano()),
		"password":          "court",
	})
	if code != 400 {
		t.Errorf("mot de passe trop court : attendu 400, obtenu http=%d %v", code, rep)
	}
	code, rep = anonyme.appel("POST", "/auth/register", map[string]any{
		"full_name": "Sans Entreprise", "email": "x@test.local", "password": "motdepasse1",
	})
	if code != 400 {
		t.Errorf("inscription sans entreprise : attendu 400, obtenu http=%d %v", code, rep)
	}

	// Connexion : mauvais mot de passe.
	proprietaire := b.client()
	proprietaire.inscrire("Entreprise Validation")
	proprietaire.activer()
	code, rep = proprietaire.appel("POST", "/auth/login", map[string]any{
		"email": "personne@test.local", "password": "motdepasse1",
	})
	if code != 401 {
		t.Errorf("connexion inconnue : attendu 401, obtenu http=%d %v", code, rep)
	}

	station, service, _ := b.creerStationEtPrestation(proprietaire)

	// Fiche sans prestation, quantité nulle, prestation inventée.
	code, rep = proprietaire.appel("POST", "/wash-orders", map[string]any{"station_id": station})
	if code != 400 {
		t.Errorf("fiche sans prestation : attendu 400, obtenu http=%d %v", code, rep)
	}
	// Quantité nulle ou négative : l'API la ramène à 1, comme l'écran de
	// saisie (un lavage compte une fois) — la ligne vaut donc le prix unitaire.
	code, rep = proprietaire.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 0}},
	})
	if code != 201 {
		t.Errorf("quantité nulle : attendu 201 (ramenée à 1), obtenu http=%d %v", code, rep)
	} else if _, prix := 0, entier(rep["subtotal_xof"]); prix <= 0 {
		t.Errorf("une quantité nulle doit être facturée une fois, sous-total obtenu %d", prix)
	}
	code, rep = proprietaire.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": "svc-inexistant", "quantity": 1}},
	})
	if code != 400 && code != 404 {
		t.Errorf("prestation inconnue : attendu 400 ou 404, obtenu http=%d %v", code, rep)
	}

	// Dépense négative, prestation sans nom ni prix.
	code, rep = proprietaire.appel("POST", "/expenses", map[string]any{
		"station_id": station, "label": "Avoir", "amount_xof": -1_000,
	})
	if code != 400 {
		t.Errorf("dépense négative : attendu 400, obtenu http=%d %v", code, rep)
	}
	code, rep = proprietaire.appel("POST", "/services", map[string]any{"name": "", "price_xof": 0})
	if code != 400 {
		t.Errorf("prestation sans nom : attendu 400, obtenu http=%d %v", code, rep)
	}

	// Encaissement nul ou négatif sur une fiche existante. La caisse est
	// ouverte pour que le refus vienne bien du montant, et non de la caisse.
	code, _ = proprietaire.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 10_000,
	})
	if code != 201 {
		t.Fatalf("ouverture de caisse : http=%d", code)
	}
	code, fiche := proprietaire.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	for _, montant := range []int{0, -5_000} {
		code, rep = proprietaire.appel("POST", "/wash-orders/"+texte(fiche, "id")+"/payments", map[string]any{
			"amount_xof": montant, "method": "especes",
		})
		if code != 400 {
			t.Errorf("encaissement de %d FCFA : attendu 400, obtenu http=%d %v", montant, code, rep)
		}
	}

	// Ressource inexistante.
	code, rep = proprietaire.appel("GET", "/wash-orders/ord-inexistante", nil)
	if code != 404 {
		t.Errorf("fiche inexistante : attendu 404, obtenu http=%d %v", code, rep)
	}
}

// ===========================================================================
// 15. Onboarding : état initial, reprise, activation
// ===========================================================================

// etatOnboarding lit l'état enregistré d'une entreprise : la base est la source
// de vérité, l'interface ne fait que la refléter.
func (b *banc) etatOnboarding(orgID string) string {
	b.t.Helper()
	var etat string
	if err := b.pool.QueryRow(context.Background(),
		`SELECT onboarding_state FROM organizations WHERE id = $1`, orgID).Scan(&etat); err != nil {
		b.t.Fatalf("lecture de onboarding_state : %v", err)
	}
	return etat
}

// creerEmploye crée un compte d'équipe (gérant, caissier…) et ouvre sa session.
func (b *banc) creerEmploye(patron *client, station, role string) *client {
	b.t.Helper()
	code, employe := patron.appel("POST", "/employees", map[string]any{
		"full_name":  "Employé " + role,
		"email":      fmt.Sprintf("%s-%d@test.local", role, time.Now().UnixNano()),
		"role":       role,
		"station_id": station,
	})
	if code != 201 {
		b.t.Fatalf("création de l'employé %s : http=%d %v", role, code, employe)
	}
	autre := b.client()
	code, connexion := autre.appel("POST", "/auth/login", map[string]any{
		"email":    texte(employe, "email"),
		"password": texte(employe, "temporary_password"),
	})
	if code != 200 {
		b.t.Fatalf("connexion de l'employé %s : http=%d %v", role, code, connexion)
	}
	autre.me = connexion
	if user, ok := connexion["user"].(map[string]any); ok {
		autre.role, _ = user["role"].(string)
		autre.orgID, _ = user["organization_id"].(string)
		autre.statID, _ = user["station_id"].(string)
	}
	return autre
}

func TestOnboardingEtatInitialEtReprise(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Onboarding")

	// 1. Une entreprise neuve n'est pas activée, et le dit.
	if etat := b.etatOnboarding(c.orgID); etat != "pending" {
		t.Errorf("état en base après inscription : %q (attendu « pending »)", etat)
	}
	code, parcours := c.appel("GET", "/onboarding", nil)
	if code != 200 {
		t.Fatalf("GET /onboarding : http=%d %v", code, parcours)
	}
	if etat := texte(parcours, "state"); etat != "pending" {
		t.Errorf("état renvoyé par l'API : %q (attendu « pending »)", etat)
	}
	// « espace » est déjà satisfaite (l'inscription crée entreprise, station et
	// propriétaire) ; c'est l'identité de l'entreprise qui manque.
	if etape := etapeOnboarding(parcours, "espace"); etape == nil || etape["done"] != true {
		t.Errorf("l'étape « espace » doit être satisfaite après l'inscription : %v", etape)
	}
	if etape := etapeOnboarding(parcours, "entreprise"); etape == nil || etape["done"] == true {
		t.Errorf("l'étape « entreprise » doit rester à compléter (téléphone absent) : %v", etape)
	}
	if activable, _ := parcours["can_activate"].(bool); activable {
		t.Error("l'activation ne doit pas être possible avant que l'identité soit complète")
	}
	// Les deux étapes du lot 4 existent : les catégories de véhicules sont un
	// prérequis (on ne vend pas sans dire ce qu'on lave), les usages non.
	if etape := etapeOnboarding(parcours, "categories"); etape == nil ||
		etape["required"] != true || etape["done"] == true || etape["form"] != "vehicules" {
		t.Errorf("l'étape « categories » doit être obligatoire et à faire : %v", etape)
	}
	if etape := etapeOnboarding(parcours, "usages"); etape == nil ||
		etape["required"] != false || etape["form"] != "usages" {
		t.Errorf("l'étape « usages » doit être facultative et outillée : %v", etape)
	}
	if manquants := codesManquants(parcours); !contient(manquants, "categories") {
		t.Errorf("l'étape « categories » doit figurer parmi les manquants : %v", manquants)
	}

	// 2. /auth/me porte le même parcours : le front n'a pas à le redemander.
	code, moi := c.appel("GET", "/auth/me", nil)
	if code != 200 {
		t.Fatalf("GET /auth/me : http=%d", code)
	}
	if _, present := moi["onboarding"].(map[string]any); !present {
		t.Errorf("un propriétaire doit recevoir l'état de son onboarding : %v", moi["onboarding"])
	}

	// 3. Enregistrer une étape ne la déclare pas « faite » (les données réelles
	// décident) mais fait passer la configuration « en cours ».
	code, apres := c.appel("PATCH", "/onboarding/steps/espace", map[string]any{
		"data": map[string]any{"choix": "creer"},
	})
	if code != 200 {
		t.Fatalf("enregistrement de l'étape « espace » : http=%d %v", code, apres)
	}
	if etat := texte(apres, "state"); etat != "in_progress" {
		t.Errorf("état après le premier enregistrement : %q (attendu « in_progress »)", etat)
	}
	if etat := b.etatOnboarding(c.orgID); etat != "in_progress" {
		t.Errorf("l'état en base doit suivre : %q", etat)
	}
	if etape := etapeOnboarding(apres, "entreprise"); etape != nil && etape["done"] == true {
		t.Error("enregistrer une étape ne doit pas déclarer « entreprise » faite")
	}

	// 4. Déconnexion puis reconnexion : la progression est conservée.
	if code, _ = c.appel("POST", "/auth/logout", nil); code != 200 {
		t.Fatalf("déconnexion : http=%d", code)
	}
	code, _ = c.appel("POST", "/auth/login", map[string]any{"email": c.email, "password": c.password})
	if code != 200 {
		t.Fatalf("reconnexion : http=%d", code)
	}
	code, reprise := c.appel("GET", "/onboarding", nil)
	if code != 200 {
		t.Fatalf("lecture après reconnexion : http=%d", code)
	}
	if etat := texte(reprise, "state"); etat != "in_progress" {
		t.Errorf("l'état doit survivre à la déconnexion : %q", etat)
	}
	etape := etapeOnboarding(reprise, "espace")
	if etape == nil {
		t.Fatal("l'étape « espace » est introuvable après reconnexion")
	}
	donnees, _ := etape["data"].(map[string]any)
	if texte(donnees, "choix") != "creer" {
		t.Errorf("le choix enregistré doit être relu après reconnexion : %v", etape["data"])
	}

	// 5. Le client ne peut ni déclarer une étape terminée, ni inventer une étape.
	code, refus := c.appel("PATCH", "/onboarding/steps/entreprise", map[string]any{"state": "completed"})
	if code != 400 || codeErreur(refus) != "validation_error" {
		t.Errorf("un état « completed » déclaré par le client doit être refusé : http=%d %v", code, refus)
	}
	code, refus = c.appel("PATCH", "/onboarding/steps/nimportequoi", map[string]any{"data": map[string]any{}})
	if code != 400 || codeErreur(refus) != "validation_error" {
		t.Errorf("une étape inconnue doit être refusée : http=%d %v", code, refus)
	}
}

func TestEcritureRefuseeAvantActivation(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Bloquee")
	station, service, _ := b.creerStationEtPrestation(c)

	// Une station non activée ne vend pas : chaque écriture d'exploitation est
	// refusée avec la liste des étapes à compléter.
	ecritures := []struct {
		nom             string
		methode, chemin string
		corps           map[string]any
	}{
		{"enregistrer un lavage", "POST", "/wash-orders", map[string]any{
			"station_id": station,
			"items":      []map[string]any{{"service_id": service, "quantity": 1}},
		}},
		{"ouvrir la caisse", "POST", "/cash/sessions", map[string]any{
			"station_id": station, "opening_float_xof": 20000,
		}},
		{"encaisser un paiement", "POST", "/cash/payments", map[string]any{
			"station_id": station, "wash_order_id": "wo-inexistant", "amount_xof": 1000,
		}},
		{"enregistrer une dépense", "POST", "/expenses", map[string]any{
			"station_id": station, "category": "produits", "label": "Savon", "amount_xof": 1500,
		}},
	}
	for _, cas := range ecritures {
		code, rep := c.appel(cas.methode, cas.chemin, cas.corps)
		if code != 409 || codeErreur(rep) != "onboarding_incomplete" {
			t.Errorf("%s : http=%d code=%q (attendu 409 onboarding_incomplete)", cas.nom, code, codeErreur(rep))
		}
		if champs := champsErreur(rep); len(champs) == 0 {
			t.Errorf("%s : le refus doit indiquer les étapes à compléter", cas.nom)
		}
	}

	// Le parcours complet lève le refus : identité, horaires, prestation tarifée.
	c.activer()
	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("après activation, la fiche doit être acceptée : http=%d %v", code, fiche)
	}
}

func TestActivationReserveeAuProprietaire(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Activation")
	station, _, _ := b.creerStationEtPrestation(patron)

	// Tout est prêt : seul le droit d'activer manque au gérant.
	patron.remplirProfilEntreprise()
	gerant := b.creerEmploye(patron, station, "manager")

	code, parcours := gerant.appel("GET", "/onboarding", nil)
	if code != 200 {
		t.Fatalf("un gérant doit pouvoir lire l'onboarding : http=%d %v", code, parcours)
	}
	code, refus := gerant.appel("POST", "/onboarding/activate", nil)
	if code != 403 {
		t.Errorf("un gérant ne doit pas activer la station : http=%d %v", code, refus)
	}
	if etat := b.etatOnboarding(patron.orgID); etat == "active" {
		t.Error("l'état ne doit pas changer quand l'activation est refusée")
	}

	// Le propriétaire, lui, peut activer.
	patron.activer()
}

func TestCaissierNaPasDeParcoursOnboarding(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Caissier")
	patron.activer()
	station, _, _ := b.creerStationEtPrestation(patron)

	caissier := b.creerEmploye(patron, station, "cashier")
	code, moi := caissier.appel("GET", "/auth/me", nil)
	if code != 200 {
		t.Fatalf("GET /auth/me (caissier) : http=%d", code)
	}
	if _, present := moi["onboarding"]; present {
		t.Errorf("un caissier ne doit pas recevoir de parcours d'onboarding : %v", moi["onboarding"])
	}
	code, rep := caissier.appel("GET", "/onboarding", nil)
	if code != 403 {
		t.Errorf("un caissier ne doit pas lire l'onboarding : http=%d %v", code, rep)
	}
	// L'entreprise étant activée, le caissier exploite normalement.
	code, session := caissier.appel("POST", "/cash/sessions", map[string]any{
		"station_id": station, "opening_float_xof": 20000,
	})
	if code != 201 {
		t.Fatalf("le caissier doit pouvoir ouvrir la caisse : http=%d %v", code, session)
	}
}

func TestMigrationOnboardingActiveLesEntreprisesExistantes(t *testing.T) {
	b := nouveauBanc(t)
	ctx := context.Background()
	c := b.client()
	c.inscrire("Entreprise Avant Onboarding")
	station, service, _ := b.creerStationEtPrestation(c)

	// Situation d'une entreprise déjà en service au moment de la migration :
	// l'état vient d'être posé (« pending » par défaut) et un choix d'étape
	// existe déjà.
	if _, err := b.pool.Exec(ctx,
		`UPDATE organizations SET onboarding_state = 'pending', onboarding_completed_at = NULL WHERE id = $1`,
		c.orgID); err != nil {
		t.Fatalf("préparation de l'état : %v", err)
	}
	if _, err := b.pool.Exec(ctx, `
		INSERT INTO onboarding_steps (organization_id, step_code, state, data)
		VALUES ($1, 'espace', 'in_progress', '{"choix":"creer"}'::jsonb)
		ON CONFLICT (organization_id, step_code) DO NOTHING`, c.orgID); err != nil {
		t.Fatalf("préparation de l'étape : %v", err)
	}

	sqlMigration, err := migrationSQL("0004_onboarding")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := b.pool.Exec(ctx, sqlMigration); err != nil {
		t.Fatalf("réapplication de la migration 0004 : %v", err)
	}

	if etat := b.etatOnboarding(c.orgID); etat != "active" {
		t.Errorf("une entreprise en service avant l'onboarding doit être activée par la migration, obtenu %q", etat)
	}
	var choix string
	if err := b.pool.QueryRow(ctx, `
		SELECT data->>'choix' FROM onboarding_steps
		WHERE organization_id = $1 AND step_code = 'espace'`, c.orgID).Scan(&choix); err != nil {
		t.Fatalf("relecture de l'étape : %v", err)
	}
	if choix != "creer" {
		t.Errorf("la migration ne doit pas effacer les choix enregistrés (choix=%q)", choix)
	}

	// Et l'exploitation reprend sans repasser par le parcours.
	code, fiche := c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Errorf("après migration, un lavage doit pouvoir être enregistré : http=%d %v", code, fiche)
	}
}

// ===========================================================================
// 17. Fiche d'entreprise : identité commerciale, facturation, registre
// ===========================================================================

func TestProfilEntrepriseEtRegistreAdministratif(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Profil")
	c.activer()

	// 1. Identité commerciale et facturation : tout est facultatif, mais ce qui
	//    est envoyé doit être enregistré et relu tel quel.
	code, rep := c.appel("PATCH", "/organizations/current", map[string]any{
		"legal_name":           "PROFIL SARL U",
		"trade_name":           "Lavage du Plateau",
		"activity_type":        "lavage manuel",
		"activity_description": "Lavage et nettoyage intérieur de véhicules légers.",
		"activity_started_on":  "2024-03-01",
		"whatsapp":             "+225 07 11 22 33 44",
		"website":              "https://lavage-du-plateau.ci",
		"billing_name":         "PROFIL SARL U — service comptabilité",
		"billing_address":      "Rue du Commerce, Plateau, Abidjan",
		"billing_email":        "facturation@profil.ci",
		"billing_phone":        "+225 27 20 30 40 50",
	})
	if code != 200 {
		t.Fatalf("profil d'entreprise : http=%d %v", code, rep)
	}

	code, org := c.appel("GET", "/organizations/current", nil)
	if code != 200 {
		t.Fatalf("GET /organizations/current : http=%d", code)
	}
	fiche, _ := org["organization"].(map[string]any)
	for champ, attendu := range map[string]string{
		"legal_name": "PROFIL SARL U", "trade_name": "Lavage du Plateau",
		"activity_type": "lavage manuel", "activity_started_on": "2024-03-01",
		"whatsapp": "+225 07 11 22 33 44", "billing_email": "facturation@profil.ci",
	} {
		if obtenu := texte(fiche, champ); obtenu != attendu {
			t.Errorf("%s : attendu %q, obtenu %q", champ, attendu, obtenu)
		}
	}

	// 2. Mentions administratives : renseignées, puis relues avec leur état.
	code, rep = c.appel("PATCH", "/organizations/current", map[string]any{
		"registry": map[string]any{
			"rccm":       "CI-ABJ-2024-B-12345",
			"tax_id":     "2412345P",
			"legal_form": "SARL",
		},
	})
	if code != 200 {
		t.Fatalf("mentions administratives : http=%d %v", code, rep)
	}
	code, org = c.appel("GET", "/organizations/current", nil)
	if code != 200 {
		t.Fatalf("relecture de l'organisme : http=%d", code)
	}
	registre, _ := org["registry"].([]any)
	if len(registre) == 0 {
		t.Fatal("le registre doit être renvoyé avec la fiche d'entreprise")
	}
	etats := map[string]map[string]any{}
	for _, brut := range registre {
		ligne, _ := brut.(map[string]any)
		etats[texte(ligne, "field_code")] = ligne
	}
	if etats["rccm"]["value"] != "CI-ABJ-2024-B-12345" || etats["rccm"]["status"] != "provided" {
		t.Errorf("RCCM : valeur et état attendus (provided), obtenu %v", etats["rccm"])
	}
	if etats["cnps_employer_number"]["status"] != "empty" {
		t.Errorf("une mention non renseignée doit rester « empty » : %v", etats["cnps_employer_number"])
	}
	if label := texte(etats["rccm"], "status_label"); label != "Renseigné" {
		t.Errorf("libellé d'état attendu « Renseigné », obtenu %q", label)
	}

	// 3. Une mention inconnue est refusée (l'écriture suit un catalogue).
	code, rep = c.appel("PATCH", "/organizations/current", map[string]any{
		"registry": map[string]any{"numero_invente": "X"},
	})
	if code != 400 {
		t.Errorf("une mention inconnue doit être refusée : http=%d %v", code, rep)
	}

	// 4. Le propriétaire ne peut pas s'auto-certifier : la vérification est
	//    réservée au support (platform:manage).
	code, rep = c.appel("POST", "/platform/organizations/"+c.orgID+"/registry/rccm/verify", nil)
	if code != 403 {
		t.Errorf("un propriétaire ne doit pas pouvoir vérifier une mention : http=%d %v", code, rep)
	}
	code, org = c.appel("GET", "/organizations/current", nil)
	if code != 200 {
		t.Fatalf("relecture après refus : http=%d", code)
	}
	_ = org

	// 5. Une valeur invalide est refusée avant écriture.
	code, rep = c.appel("PATCH", "/organizations/current", map[string]any{"billing_email": "pas-un-email"})
	if code != 400 || champsErreur(rep)["billing_email"] == "" {
		t.Errorf("un e-mail de facturation invalide doit être refusé : http=%d %v", code, rep)
	}
	code, rep = c.appel("PATCH", "/organizations/current", map[string]any{"activity_started_on": "01/03/2024"})
	if code != 400 {
		t.Errorf("une date mal formée doit être refusée : http=%d %v", code, rep)
	}

	// 6. Modifier une mention annule sa vérification : le support a vérifié un
	//    numéro précis, pas le suivant.
	if err := verifierMentionPourTest(b, c.orgID, "rccm"); err != nil {
		t.Fatalf("vérification en base : %v", err)
	}
	code, org = c.appel("GET", "/organizations/current", nil)
	if code != 200 {
		t.Fatalf("relecture après vérification : http=%d", code)
	}
	registre, _ = org["registry"].([]any)
	for _, brut := range registre {
		ligne, _ := brut.(map[string]any)
		if texte(ligne, "field_code") == "rccm" && ligne["status"] != "verified" {
			t.Errorf("le RCCM devrait être vérifié : %v", ligne)
		}
	}
	if code, _ := c.appel("PATCH", "/organizations/current", map[string]any{
		"registry": map[string]any{"rccm": "CI-ABJ-2024-B-99999"},
	}); code != 200 {
		t.Fatalf("modification du RCCM : http=%d", code)
	}
	code, org = c.appel("GET", "/organizations/current", nil)
	if code != 200 {
		t.Fatalf("relecture après modification : http=%d", code)
	}
	registre, _ = org["registry"].([]any)
	for _, brut := range registre {
		ligne, _ := brut.(map[string]any)
		if texte(ligne, "field_code") == "rccm" {
			if ligne["status"] != "provided" {
				t.Errorf("une modification doit annuler la vérification, obtenu %v", ligne["status"])
			}
			if ligne["verified_at"] != nil {
				t.Error("la date de vérification doit être effacée avec la valeur précédente")
			}
		}
	}
}

// verifierMentionPourTest passe par la même fonction que le support, sans
// détour : le test vérifie le comportement réel, pas une copie.
func verifierMentionPourTest(b *banc, orgID, code string) error {
	var auteur string
	if err := b.pool.QueryRow(context.Background(),
		`SELECT id FROM users WHERE organization_id = $1 LIMIT 1`, orgID).Scan(&auteur); err != nil {
		return err
	}
	return b.app.verifierMention(context.Background(), orgID, code, auteur, "Support de test")
}

// ===========================================================================
// 18. Station : localisation, capacités, horaires
// ===========================================================================

func TestFicheStationEtHoraires(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Fiche Station")

	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		t.Fatalf("stations : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	station := texte(premiere, "id")

	// 1. Localisation : adresse structurée et coordonnées.
	code, rep := c.appel("PATCH", "/stations/"+station, map[string]any{
		"district":      "Abidjan",
		"commune":       "Cocody",
		"district_area": "Angré",
		"street":        "Boulevard Latrille, face pharmacie",
		"landmark":      "À côté de la station Total",
		"latitude":      "5.4091",
		"longitude":     "-3.9798",
	})
	if code != 200 {
		t.Fatalf("localisation : http=%d %v", code, rep)
	}

	// 2. Capacités : « pas d'aspiration » (false) et « pas de réponse » (null)
	//    doivent rester distincts.
	code, rep = c.appel("PATCH", "/stations/"+station, map[string]any{
		"site": map[string]any{
			"wash_types":           []string{"lavage manuel à l'eau", "nettoyage intérieur"},
			"bays":                 2,
			"has_vacuum":           false,
			"has_waiting_room":     true,
			"water_source":         "raccordement SODECI + forage",
			"water_reserve_liters": 2000,
			"power_supply":         "réseau CIE",
			"has_generator":        nil,
			"notes":                "Accès difficile pour les camions.",
			"constraints":          "Station fermée les jours de pluie forte.",
		},
	})
	if code != 200 {
		t.Fatalf("capacités : http=%d %v", code, rep)
	}

	code, profil := c.appel("GET", "/stations/"+station+"/profile", nil)
	if code != 200 {
		t.Fatalf("GET profile : http=%d %v", code, profil)
	}
	fiche, _ := profil["station"].(map[string]any)
	for champ, attendu := range map[string]string{
		"district": "Abidjan", "commune": "Cocody", "district_area": "Angré",
		"latitude": "5.409100", "longitude": "-3.979800",
	} {
		if obtenu := texte(fiche, champ); obtenu != attendu {
			t.Errorf("%s : attendu %q, obtenu %q", champ, attendu, obtenu)
		}
	}
	site, _ := profil["site"].(map[string]any)
	if site["has_vacuum"] != false {
		t.Errorf("has_vacuum doit valoir false (et non null) : %v", site["has_vacuum"])
	}
	if site["has_generator"] != nil {
		t.Errorf("un équipement non répondu doit rester null : %v", site["has_generator"])
	}
	if bays := entier(site["bays"]); bays != 2 {
		t.Errorf("bays attendu 2, obtenu %v", site["bays"])
	}

	// 3. Horaires : un enregistrement valide, puis les refus.
	jours := []map[string]any{}
	for jour := 1; jour <= 7; jour++ {
		horaire := map[string]any{"weekday": jour, "is_open": jour <= 6, "booking_allowed": true}
		if jour <= 6 {
			horaire["opens_at"] = "07:30"
			horaire["closes_at"] = "19:00"
			horaire["break_start"] = "12:00"
			horaire["break_end"] = "13:00"
			horaire["last_checkin_at"] = "18:00"
		}
		jours = append(jours, horaire)
	}
	code, rep = c.appel("PUT", "/stations/"+station+"/opening-hours", map[string]any{
		"days": jours,
		"exceptions": []map[string]any{
			{"day": "2026-12-25", "is_open": false, "label": "Noël"},
			{"day": "2027-01-02", "is_open": true, "opens_at": "09:00", "closes_at": "14:00", "label": "Veille de rentrée"},
		},
	})
	if code != 200 {
		t.Fatalf("horaires : http=%d %v", code, rep)
	}
	code, profil = c.appel("GET", "/stations/"+station+"/profile", nil)
	if code != 200 {
		t.Fatalf("relecture des horaires : http=%d", code)
	}
	enregistres, _ := profil["days"].([]any)
	if len(enregistres) != 7 {
		t.Fatalf("les sept jours doivent être renvoyés, obtenu %d", len(enregistres))
	}
	lundi, _ := enregistres[0].(map[string]any)
	if texte(lundi, "opens_at") != "07:30" || texte(lundi, "closes_at") != "19:00" {
		t.Errorf("horaire du lundi non conservé : %v", lundi)
	}
	dimanche, _ := enregistres[6].(map[string]any)
	if ouvert, _ := dimanche["is_open"].(bool); ouvert {
		t.Error("le dimanche doit rester fermé")
	}
	exceptions, _ := profil["exceptions"].([]any)
	if len(exceptions) != 2 {
		t.Errorf("deux journées particulières attendues, obtenu %d : %v", len(exceptions), exceptions)
	}

	// Refus : fermeture avant ouverture.
	invalide := append([]map[string]any{}, jours...)
	invalide[0] = map[string]any{"weekday": 1, "is_open": true, "opens_at": "18:00", "closes_at": "08:00"}
	code, rep = c.appel("PUT", "/stations/"+station+"/opening-hours", map[string]any{"days": invalide})
	if code != 400 || champsErreur(rep)["lundi"] == "" {
		t.Errorf("une fermeture avant ouverture doit être refusée : http=%d %v", code, rep)
	}

	// Refus : pause hors des heures d'ouverture.
	invalide = append([]map[string]any{}, jours...)
	invalide[0] = map[string]any{"weekday": 1, "is_open": true, "opens_at": "08:00", "closes_at": "18:00",
		"break_start": "07:00", "break_end": "07:30"}
	code, rep = c.appel("PUT", "/stations/"+station+"/opening-hours", map[string]any{"days": invalide})
	if code != 400 || champsErreur(rep)["lundi"] == "" {
		t.Errorf("une pause en dehors des heures doit être refusée : http=%d %v", code, rep)
	}

	// Refus : heure illisible.
	invalide = append([]map[string]any{}, jours...)
	invalide[0] = map[string]any{"weekday": 1, "is_open": true, "opens_at": "8h", "closes_at": "18:00"}
	code, rep = c.appel("PUT", "/stations/"+station+"/opening-hours", map[string]any{"days": invalide})
	if code != 400 {
		t.Errorf("une heure au mauvais format doit être refusée : http=%d %v", code, rep)
	}

	// Refus : deux règles pour la même journée.
	code, rep = c.appel("PUT", "/stations/"+station+"/opening-hours", map[string]any{
		"days": jours,
		"exceptions": []map[string]any{
			{"day": "2026-12-25", "is_open": false, "label": "Noël"},
			{"day": "2026-12-25", "is_open": true, "opens_at": "09:00", "closes_at": "12:00"},
		},
	})
	if code != 400 {
		t.Errorf("deux exceptions le même jour doivent être refusées : http=%d %v", code, rep)
	}
	if ouvert, _ := rep["days"].([]any); len(ouvert) != 0 {
		t.Error("un enregistrement refusé ne doit rien écrire")
	}

	// Le refus n'a rien modifié : les horaires valides sont toujours là.
	code, profil = c.appel("GET", "/stations/"+station+"/profile", nil)
	if code != 200 {
		t.Fatalf("relecture après refus : http=%d", code)
	}
	enregistres, _ = profil["days"].([]any)
	lundi, _ = enregistres[0].(map[string]any)
	if texte(lundi, "opens_at") != "07:30" {
		t.Errorf("un refus ne doit pas altérer les horaires enregistrés : %v", lundi)
	}
}

func TestHorairesIndependantsParStationEtIsolation(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Deux Stations")
	patron.activer()

	code, stations := patron.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		t.Fatalf("stations : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	stationA := texte(premiere, "id")

	// La formule d'essai limite à une station : on insère la seconde en base
	// (comme le fait l'exploitant après un passage à une formule supérieure).
	stationB := "st-deuxieme-" + texte(premiere, "id")[len(texte(premiere, "id"))-6:]
	if _, err := b.pool.Exec(context.Background(),
		`INSERT INTO stations (id, organization_id, name, code, city, is_default, is_active)
		 VALUES ($1,$2,'Station Deuxième','DEX','Abidjan',false,true)`, stationB, patron.orgID); err != nil {
		t.Fatalf("insertion de la seconde station : %v", err)
	}

	ouvrir := func(station string, ouvre, ferme string) {
		jours := []map[string]any{}
		for jour := 1; jour <= 7; jour++ {
			horaire := map[string]any{"weekday": jour, "is_open": jour <= 5, "booking_allowed": true}
			if jour <= 5 {
				horaire["opens_at"] = ouvre
				horaire["closes_at"] = ferme
			}
			jours = append(jours, horaire)
		}
		code, rep := patron.appel("PUT", "/stations/"+station+"/opening-hours", map[string]any{"days": jours})
		if code != 200 {
			t.Fatalf("horaires de %s : http=%d %v", station, code, rep)
		}
	}
	ouvrir(stationA, "07:00", "17:00")
	ouvrir(stationB, "09:00", "21:00")

	// Les deux stations gardent leurs propres horaires.
	code, profilA := patron.appel("GET", "/stations/"+stationA+"/profile", nil)
	if code != 200 {
		t.Fatalf("profil A : http=%d", code)
	}
	code, profilB := patron.appel("GET", "/stations/"+stationB+"/profile", nil)
	if code != 200 {
		t.Fatalf("profil B : http=%d", code)
	}
	joursA, _ := profilA["days"].([]any)
	joursB, _ := profilB["days"].([]any)
	lundiA, _ := joursA[0].(map[string]any)
	lundiB, _ := joursB[0].(map[string]any)
	if texte(lundiA, "opens_at") != "07:00" || texte(lundiB, "opens_at") != "09:00" {
		t.Errorf("les horaires des deux stations doivent être indépendants : %v / %v", lundiA, lundiB)
	}

	// Isolation : une autre entreprise ne lit ni n'écrit la fiche de la première.
	intruse := b.client()
	intruse.inscrire("Entreprise Voisine")
	code, _ = intruse.appel("GET", "/stations/"+stationA+"/profile", nil)
	if code != 404 {
		t.Errorf("lire la fiche d'une autre entreprise doit répondre 404 : http=%d", code)
	}
	code, _ = intruse.appel("PUT", "/stations/"+stationA+"/opening-hours", map[string]any{
		"days": []map[string]any{{"weekday": 1, "is_open": true, "opens_at": "01:00", "closes_at": "02:00"}},
	})
	if code != 404 {
		t.Errorf("écrire les horaires d'une autre entreprise doit répondre 404 : http=%d", code)
	}

	// Un caissier rattaché à l'autre station ne peut pas modifier les horaires.
	caissier := b.creerEmploye(patron, stationB, "cashier")
	code, _ = caissier.appel("PUT", "/stations/"+stationA+"/opening-hours", map[string]any{
		"days": []map[string]any{{"weekday": 1, "is_open": true, "opens_at": "08:00", "closes_at": "09:00"}},
	})
	if code != 403 && code != 404 {
		t.Errorf("un caissier d'une autre station ne doit pas modifier ces horaires : http=%d", code)
	}
	code, _ = caissier.appel("GET", "/stations/"+stationA+"/profile", nil)
	if code != 403 {
		t.Errorf("un caissier d'une autre station ne doit pas lire cette fiche : http=%d", code)
	}
}

// ===========================================================================
// 19. Une prestation sans tarif ne se vend pas
// ===========================================================================

func TestPrestationSansTarifRefusee(t *testing.T) {
	b := nouveauBanc(t)
	c := b.client()
	c.inscrire("Entreprise Sans Tarif")
	c.remplirProfilEntreprise()
	c.declarerHoraires()

	code, stations := c.tableau("GET", "/stations", nil)
	if code != 200 || len(stations) == 0 {
		t.Fatalf("stations : http=%d", code)
	}
	premiere, _ := stations[0].(map[string]any)
	station := texte(premiere, "id")

	// Le propriétaire décrit une prestation sans prix : c'est permis (il ne
	// connaît peut-être pas encore son tarif), mais cette prestation ne peut ni
	// déclencher l'activation, ni être encaissée.
	code, prestation := c.appel("POST", "/services", map[string]any{
		"station_id": station, "name": "Lavage à définir",
	})
	if code != 201 {
		t.Fatalf("création d'une prestation sans prix : http=%d %v", code, prestation)
	}
	service := texte(prestation, "id")

	// L'activation reste refusée : la grille tarifaire est un prérequis.
	code, rep := c.appel("POST", "/onboarding/activate", nil)
	if code != 409 || champsErreur(rep)["tarifs"] == "" {
		t.Fatalf("l'activation doit exiger un tarif : http=%d %v", code, rep)
	}

	// Le gérant tarife une autre prestation : l'activation devient possible,
	// mais la prestation non tarifée reste invendable.
	code, tarifee := c.appel("POST", "/services", map[string]any{
		"station_id": station, "name": "Lavage tarifé",
	})
	if code != 201 {
		t.Fatalf("création de la prestation tarifée : http=%d %v", code, tarifee)
	}
	serviceTarife := texte(tarifee, "id")
	c.tariferCatalogue(station, serviceTarife, 2500)
	c.activer()

	code, rep = c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 409 || codeErreur(rep) != "price_required" {
		t.Errorf("une prestation sans tarif doit être refusée à la vente : http=%d %v", code, rep)
	}

	// La prestation tarifée, elle, se vend au prix saisi.
	code, rep = c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": serviceTarife, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("la prestation tarifée doit se vendre : http=%d %v", code, rep)
	}
	if total := entier(rep["total_xof"]); total != 2500 {
		t.Errorf("le total doit venir du tarif saisi (2500), obtenu %d", total)
	}

	// Renseigner le prix manquant rend la prestation vendable à son tour.
	c.tariferCatalogue(station, service, 1500)
	code, rep = c.appel("POST", "/wash-orders", map[string]any{
		"station_id": station,
		"items":      []map[string]any{{"service_id": service, "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("après tarification, la vente doit passer : http=%d %v", code, rep)
	}
	if total := entier(rep["total_xof"]); total != 1500 {
		t.Errorf("le total doit suivre le nouveau tarif (1500), obtenu %d", total)
	}
}

// ---------------------------------------------------------------------------
// Lot 1 — Invitations : rejoindre une entreprise qui existe déjà.
// ---------------------------------------------------------------------------

// creerInvitation crée une invitation et renvoie sa réponse (code + jeton).
func (b *banc) creerInvitation(patron *client, corps map[string]any) map[string]any {
	b.t.Helper()
	code, rep := patron.appel("POST", "/invitations", corps)
	if code != 201 {
		b.t.Fatalf("création de l'invitation : http=%d %v", code, rep)
	}
	return rep
}

// rejoindreParInvitation : le parcours réel de l'invité — il n'a pas de compte,
// il s'inscrit avec le jeton de l'invitation.
func (b *banc) rejoindreParInvitation(token, email string) *client {
	b.t.Helper()
	invite := b.client()
	code, rep := invite.appel("POST", "/auth/register", map[string]any{
		"invitation_token": token,
		"full_name":        "Invitée Test",
		"email":            email,
		"password":         "motdepasse1",
	})
	if code != 201 {
		b.t.Fatalf("inscription par invitation : http=%d %v", code, rep)
	}
	invite.me = rep
	invite.email, invite.password = email, "motdepasse1"
	if invitation, ok := rep["invitation_accepted"].(bool); !ok || !invitation {
		b.t.Errorf("la réponse d'inscription doit annoncer invitation_accepted=true : %v", rep["invitation_accepted"])
	}
	if user, ok := rep["user"].(map[string]any); ok {
		invite.role, _ = user["role"].(string)
		invite.orgID, _ = user["organization_id"].(string)
		invite.statID, _ = user["station_id"].(string)
	}
	return invite
}

// TestCatalogueVehiculesEtUsages couvre le lot 4 : deux référentiels distincts
// (catégorie physique, usage commercial), un référentiel commun que la station
// active, et des fiches passées qui restent lisibles après une désactivation.
func TestCatalogueVehiculesEtUsages(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Catalogue")
	station, _, _ := b.creerStationEtPrestation(patron)

	// 1. Le référentiel est amorcé, et le relire ne le duplique pas.
	code, catalogue := patron.appel("GET", "/vehicle-catalogue?station_id="+station, nil)
	if code != 200 {
		t.Fatalf("GET /vehicle-catalogue : http=%d %v", code, catalogue)
	}
	categories, _ := catalogue["categories"].([]any)
	usages, _ := catalogue["usages"].([]any)
	if len(categories) < 10 || len(usages) < 15 {
		t.Fatalf("référentiel incomplet : %d catégorie(s), %d usage(s)", len(categories), len(usages))
	}
	code, relu := patron.appel("GET", "/vehicle-catalogue?station_id="+station, nil)
	relecture, _ := relu["categories"].([]any)
	if len(relecture) != len(categories) {
		t.Errorf("un amorçage rejoué ne doit pas dupliquer le référentiel : %d puis %d",
			len(categories), len(relecture))
	}

	// 2. Rien n'est activé pour cette station, et elle ne peut pas vendre.
	code, avant := patron.appel("GET", "/vehicle-categories?station_id="+station, nil)
	if code != 200 || entier(avant["total"]) != 0 {
		t.Errorf("aucune catégorie ne doit être active par défaut : http=%d %v", code, avant)
	}
	code, refus := patron.appel("POST", "/onboarding/activate", nil)
	if code != 409 {
		t.Fatalf("activation sans catégorie : http=%d %v (attendu 409)", code, refus)
	}
	if champsErreur(refus)["categories"] == nil {
		t.Errorf("le refus doit nommer l'étape « categories » : %v", champsErreur(refus))
	}

	// 3. Activation des catégories, avec un nom local : c'est le libellé de la
	//    station qui s'affiche, pas celui du référentiel.
	code, actives := patron.appel("PUT", "/stations/"+station+"/vehicle-categories",
		map[string]any{"categories": []map[string]any{
			{"code": "berline", "is_active": true, "name": "Voiture de ville", "sort_order": 10},
			{"code": "moto", "is_active": true, "sort_order": 20},
			{"code": "camion", "is_active": false, "sort_order": 30},
		}})
	if code != 200 {
		t.Fatalf("activation des catégories : http=%d %v", code, actives)
	}
	if entier(actives["actives"]) != 2 {
		t.Errorf("catégories actives : %v (attendu 2)", actives["actives"])
	}
	code, listees := patron.appel("GET", "/vehicle-categories?station_id="+station+"&actives=1", nil)
	items, _ := listees["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("la caisse doit recevoir 2 catégories : %v", listees)
	}
	premiere, _ := items[0].(map[string]any)
	if !strings.Contains(texte(premiere, "code"), "berline") && !strings.Contains(texte(items[1].(map[string]any), "code"), "berline") {
		t.Fatalf("la catégorie « berline » est absente : %v", items)
	}
	for _, brut := range items {
		item, _ := brut.(map[string]any)
		if texte(item, "code") == "berline" {
			if texte(item, "name") != "Voiture de ville" || item["name_from_template"] != false {
				t.Errorf("le nom local doit primer : %v", item)
			}
		}
		if texte(item, "code") == "moto" && item["name_from_template"] != true {
			t.Errorf("sans renommage, le nom du référentiel est conservé : %v", item)
		}
	}

	// 4. Une catégorie propre à la station : code local, jamais dans le
	//    référentiel commun.
	code, creee := patron.appel("POST", "/vehicle-categories", map[string]any{
		"station_id": station, "name": "Tricycle de livraison", "family": "Deux-roues et assimilés",
	})
	if code != 200 {
		t.Fatalf("création d'une catégorie : http=%d %v", code, creee)
	}
	var personnalisee map[string]any
	for _, brut := range creee["items"].([]any) {
		item, _ := brut.(map[string]any)
		if texte(item, "code") == "perso" || strings.HasPrefix(texte(item, "code"), "perso_") {
			personnalisee = item
		}
	}
	if personnalisee == nil || personnalisee["kind"] != "personnalisee" {
		t.Fatalf("la catégorie créée doit être marquée « personnalisee » : %v", creee["items"])
	}
	codeLocale := texte(personnalisee, "code")

	// 5. Usages : activation puis combinaisons véhicule × usage.
	code, usageRefus := patron.appel("PUT", "/stations/"+station+"/vehicle-usages",
		map[string]any{"usages": []map[string]any{
			{"code": "taxi_compteur", "is_active": true},
			{"code": "moto_taxi", "is_active": true},
		}})
	if code != 200 || entier(usageRefus["actifs"]) != 2 {
		t.Fatalf("activation des usages : http=%d %v", code, usageRefus)
	}
	code, couples := patron.appel("PUT", "/stations/"+station+"/vehicle-usage-categories",
		map[string]any{"couples": []map[string]any{
			{"usage_code": "taxi_compteur", "category_code": "berline"},
			{"usage_code": "moto_taxi", "category_code": "moto"},
			{"usage_code": "moto_taxi", "category_code": codeLocale},
		}})
	if code != 200 || entier(couples["total"]) != 3 {
		t.Fatalf("couples véhicule × usage : http=%d %v", code, couples)
	}
	// Un couple qui cite une catégorie non activée est refusé, champ par champ.
	code, coupleRefuse := patron.appel("PUT", "/stations/"+station+"/vehicle-usage-categories",
		map[string]any{"couples": []map[string]any{
			{"usage_code": "moto_taxi", "category_code": "camion"},
		}})
	if code != 400 || len(champsErreur(coupleRefuse)) == 0 {
		t.Errorf("un couple avec une catégorie non activée doit être refusé : http=%d %v",
			code, coupleRefuse)
	}
	// Le refus n'a rien enregistré : les trois couples sont toujours là.
	code, relus := patron.appel("GET", "/vehicle-usage-categories?station_id="+station, nil)
	if code != 200 || entier(relus["total"]) != 3 {
		t.Errorf("un refus ne doit rien modifier : %v", relus)
	}

	// 6. La caisse reçoit les deux listes d'un seul appel.
	code, options := patron.appel("GET", "/vehicle-category-options?station_id="+station, nil)
	if code != 200 {
		t.Fatalf("GET /vehicle-category-options : http=%d", code)
	}
	if len(options["categories"].([]any)) != 3 || len(options["usages"].([]any)) != 2 {
		t.Errorf("options de la caisse : %d catégorie(s), %d usage(s)",
			len(options["categories"].([]any)), len(options["usages"].([]any)))
	}

	// 7. La station s'active, puis une fiche porte les deux libellés.
	patron.activer()
	code, fiche := patron.appel("POST", "/wash-orders", map[string]any{
		"station_id": station, "customer_name": "Client Taxi", "plate": "1234 AB 01",
		"vehicle_category_id": "berline", "vehicle_usage_id": "taxi_compteur",
		"items": []map[string]any{{"service_id": b.serviceTarife(patron, station), "quantity": 1}},
	})
	if code != 201 {
		t.Fatalf("création de la fiche : http=%d %v", code, fiche)
	}
	if texte(fiche, "vehicle_category_name") != "Voiture de ville" || texte(fiche, "vehicle_usage_name") != "Taxi compteur" {
		t.Errorf("la fiche doit conserver les deux libellés : %v", fiche)
	}
	ficheID := texte(fiche, "id")

	// Un usage n'est pas une catégorie : le référentiel de l'un ne sert pas pour
	// l'autre, sinon « taxi compteur » deviendrait une catégorie de véhicule.
	code, melange := patron.appel("POST", "/wash-orders", map[string]any{
		"station_id": station, "vehicle_category_id": "taxi_compteur",
		"items": []map[string]any{{"service_id": b.serviceTarife(patron, station), "quantity": 1}},
	})
	if code != 400 || codeErreur(melange) != "validation_error" {
		t.Errorf("un usage passé comme catégorie doit être refusé : http=%d %v", code, melange)
	}

	// Un couple non déclaré est refusé aussi : la station n'a pas reconnu
	// « berline × moto-taxi ».
	code, coupleInconnu := patron.appel("POST", "/wash-orders", map[string]any{
		"station_id": station, "vehicle_category_id": "berline", "vehicle_usage_id": "moto_taxi",
		"items": []map[string]any{{"service_id": b.serviceTarife(patron, station), "quantity": 1}},
	})
	if code != 400 {
		t.Errorf("un couple non déclaré doit être refusé : http=%d %v", code, coupleInconnu)
	}

	// 8. Désactivation : la caisse ne propose plus la catégorie, la fiche
	//    passée reste lisible avec son nom.
	code, _ = patron.appel("PUT", "/stations/"+station+"/vehicle-categories",
		map[string]any{"categories": []map[string]any{
			{"code": "berline", "is_active": false},
			{"code": "moto", "is_active": true},
		}})
	if code != 200 {
		t.Fatalf("désactivation de la catégorie : http=%d", code)
	}
	code, relue := patron.appel("GET", "/wash-orders/"+ficheID, nil)
	ancienne := sousMap(relue, "order")
	if code != 200 || texte(ancienne, "vehicle_category_name") != "Voiture de ville" {
		t.Errorf("une fiche passée doit garder le nom de sa catégorie : http=%d %v", code, ancienne)
	}
	if texte(ancienne, "vehicle_usage_name") != "Taxi compteur" {
		t.Errorf("l'usage reste lisible lui aussi : %v", ancienne)
	}
	code, proposees := patron.appel("GET", "/vehicle-category-options?station_id="+station, nil)
	for _, brut := range proposees["categories"].([]any) {
		item, _ := brut.(map[string]any)
		if texte(item, "id") == "berline" {
			t.Error("une catégorie désactivée ne doit plus être proposée à la caisse")
		}
	}

	// 9. Isolation : une autre entreprise ne voit ni ne modifie ces réglages,
	//    et la sienne reste vide (elle n'a rien activé).
	concurrent := b.client()
	concurrent.inscrire("Entreprise Catalogue Concurrente")
	code, _ = concurrent.appel("GET", "/vehicle-categories?station_id="+station, nil)
	if code != 404 {
		t.Errorf("les catégories d'une autre entreprise doivent être introuvables : http=%d", code)
	}
	code, _ = concurrent.appel("PUT", "/stations/"+station+"/vehicle-categories",
		map[string]any{"categories": []map[string]any{{"code": "berline", "is_active": true}}})
	if code != 404 {
		t.Errorf("modifier les catégories d'une autre entreprise doit être refusé : http=%d", code)
	}
	code, siennes := concurrent.appel("GET", "/vehicle-categories?station_id="+concurrent.statID+"&actives=1", nil)
	if code != 200 || entier(siennes["total"]) != 0 {
		t.Errorf("chaque entreprise a ses propres réglages : http=%d %v", code, siennes)
	}
}

// serviceTarife renvoie l'identifiant d'une prestation tarifée de la station.
func (b *banc) serviceTarife(c *client, station string) string {
	c.t.Helper()
	code, catalogue := c.appel("GET", "/services?station_id="+station, nil)
	if code != 200 {
		c.t.Fatalf("GET /services : http=%d", code)
	}
	for _, brut := range catalogue["items"].([]any) {
		prestation, _ := brut.(map[string]any)
		if entier(prestation["price_xof"]) > 0 {
			return texte(prestation, "id")
		}
	}
	c.t.Fatal("aucune prestation tarifée pour la station")
	return ""
}

func TestInvitationRejointLEntrepriseSansDevenirProprietaire(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Invitante")
	station := patron.statID
	ownerID := texte(sousMap(patron.me, "user"), "id")

	invitation := b.creerInvitation(patron, map[string]any{
		"email": fmt.Sprintf("cashier-%d@test.local", time.Now().UnixNano()),
		"role":  "cashier",
	})
	code, jeton := texte(invitation, "code"), texte(invitation, "token")
	if len(code) != 8 || jeton == "" {
		t.Fatalf("l'invitation doit rendre un code de 8 caractères et un jeton : %v", invitation)
	}

	// L'aperçu public ne révèle rien d'autre que de quoi reconnaître l'employeur.
	anonyme := b.client()
	codeHTTP, apercu := anonyme.appel("GET", "/invitations/"+strings.ToLower(code), nil)
	if codeHTTP != 200 {
		t.Fatalf("aperçu public : http=%d %v", codeHTTP, apercu)
	}
	if texte(apercu, "role") != "cashier" || texte(apercu, "organization_name") != "Entreprise Invitante" {
		t.Errorf("aperçu incomplet : %v", apercu)
	}
	for _, cle := range []string{"organization_id", "station_id", "user_id", "id"} {
		if _, present := apercu[cle]; present {
			t.Errorf("l'aperçu public ne doit pas exposer %q : %v", cle, apercu)
		}
	}

	invite := b.rejoindreParInvitation(jeton, fmt.Sprintf("invite-%d@test.local", time.Now().UnixNano()))
	if invite.role != "cashier" {
		t.Errorf("rôle appliqué : %q (attendu cashier)", invite.role)
	}
	if invite.orgID != patron.orgID {
		t.Errorf("l'invité doit rejoindre l'entreprise invitante : %s ≠ %s", invite.orgID, patron.orgID)
	}
	if invite.statID != station {
		t.Errorf("station de l'invité : %s (attendu %s)", invite.statID, station)
	}

	// Aucune entreprise, aucune station, aucun abonnement de plus.
	var orgs, stations, abonnements int
	ctx := context.Background()
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM organizations`).Scan(&orgs); err != nil {
		t.Fatal(err)
	}
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM stations WHERE organization_id = $1`, patron.orgID).Scan(&stations); err != nil {
		t.Fatal(err)
	}
	if err := b.pool.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE organization_id = $1`, patron.orgID).Scan(&abonnements); err != nil {
		t.Fatal(err)
	}
	if stations != 1 || abonnements != 1 {
		t.Errorf("rejoindre ne crée rien : stations=%d abonnements=%d", stations, abonnements)
	}

	// Le propriétaire est intact, et le compte de l'invité ne l'est pas devenu.
	_, mePatron := patron.appel("GET", "/auth/me", nil)
	if role := texte(sousMap(mePatron, "user"), "role"); role != "owner" {
		t.Errorf("le propriétaire doit rester owner, obtenu %q", role)
	}
	if id := texte(sousMap(mePatron, "user"), "id"); id != ownerID {
		t.Errorf("le propriétaire a changé d'identité : %s ≠ %s", id, ownerID)
	}
	if invite.role == "owner" {
		t.Error("une invitation ne peut pas donner la propriété de l'entreprise")
	}
	_ = orgs

	// L'invitation n'est pas réutilisable : un second compte ne doit pas passer.
	seconde := b.client()
	codeHTTP, refus := seconde.appel("POST", "/auth/register", map[string]any{
		"invitation_token": jeton,
		"full_name":        "Seconde Personne",
		"email":            fmt.Sprintf("seconde-%d@test.local", time.Now().UnixNano()),
		"password":         "motdepasse1",
	})
	if codeHTTP != 409 || codeErreur(refus) != "invitation_used" {
		t.Errorf("réutilisation de l'invitation : http=%d code=%q (attendu 409 invitation_used)", codeHTTP, codeErreur(refus))
	}

	// L'état de l'invitation est visible dans la liste de l'entreprise.
	codeHTTP, liste := patron.appel("GET", "/invitations", nil)
	if codeHTTP != 200 {
		t.Fatalf("GET /invitations : http=%d", codeHTTP)
	}
	items, _ := liste["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("une invitation attendue, %d trouvée(s)", len(items))
	}
	premiere, _ := items[0].(map[string]any)
	if texte(premiere, "state") != "accepted" {
		t.Errorf("état attendu « accepted », obtenu %q", texte(premiere, "state"))
	}
	// Le jeton n'est jamais relu : il n'est pas stocké, seulement son empreinte.
	if _, present := premiere["token"]; present {
		t.Error("la liste ne doit pas renvoyer de jeton : il n'est pas stocké")
	}
}

func TestInvitationParTelephoneEtParEmail(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Téléphone")

	// Par téléphone : le numéro est la seule adresse de l'invitation.
	invitation := b.creerInvitation(patron, map[string]any{
		"phone": "+225 07 07 07 07 07",
		"role":  "washer",
	})
	invite := b.rejoindreParInvitation(texte(invitation, "token"),
		fmt.Sprintf("laveur-%d@test.local", time.Now().UnixNano()))
	if invite.role != "washer" {
		t.Errorf("rôle appliqué : %q (attendu washer)", invite.role)
	}
	// Le numéro de l'invitation devient le téléphone du compte.
	_, me := invite.appel("GET", "/auth/me", nil)
	if tel := texte(sousMap(me, "user"), "phone"); tel == "" {
		t.Error("le téléphone de l'invitation doit être repris sur le compte")
	}

	// Une invitation sans destinataire est refusée.
	code, refus := patron.appel("POST", "/invitations", map[string]any{"role": "cashier"})
	if code != 400 || codeErreur(refus) != "validation_error" {
		t.Errorf("invitation sans e-mail ni téléphone : http=%d code=%q", code, codeErreur(refus))
	}

	// Le rôle « owner » ne s'invite pas.
	code, refus = patron.appel("POST", "/invitations", map[string]any{
		"email": "patron-bis@test.local", "role": "owner",
	})
	if code != 400 {
		t.Errorf("invitation en tant qu'owner : http=%d (attendu 400)", code)
	}
}

func TestInvitationExpireeRevoqueeEtCodeInvente(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Refus")
	ctx := context.Background()

	// Expirée : l'invitation existe, mais sa date est passée.
	expiree := b.creerInvitation(patron, map[string]any{
		"email": fmt.Sprintf("expiree-%d@test.local", time.Now().UnixNano()), "role": "cashier",
	})
	if _, err := b.pool.Exec(ctx,
		`UPDATE invitations SET expires_at = now() - interval '1 day' WHERE id = $1`,
		texte(expiree, "id")); err != nil {
		t.Fatal(err)
	}
	victime := b.client()
	code, refus := victime.appel("POST", "/auth/register", map[string]any{
		"invitation_token": texte(expiree, "token"),
		"full_name":        "Trop Tard",
		"email":            fmt.Sprintf("trop-tard-%d@test.local", time.Now().UnixNano()),
		"password":         "motdepasse1",
	})
	if code != 409 || codeErreur(refus) != "invitation_expired" {
		t.Errorf("invitation expirée : http=%d code=%q", code, codeErreur(refus))
	}
	// L'aperçu explique aussi pourquoi elle ne marche plus.
	code, apercu := victime.appel("GET", "/invitations/"+texte(expiree, "code"), nil)
	if code != 200 || texte(apercu, "state") != "expired" || apercu["usable"] != false {
		t.Errorf("aperçu d'une invitation expirée : http=%d %v", code, apercu)
	}

	// Révoquée : refusée à l'inscription, et la révocation est idempotente.
	revoquee := b.creerInvitation(patron, map[string]any{
		"email": fmt.Sprintf("revoquee-%d@test.local", time.Now().UnixNano()), "role": "washer",
	})
	if code, _ := patron.appel("POST", "/invitations/"+texte(revoquee, "id")+"/revoke", nil); code != 200 {
		t.Fatalf("révocation : http=%d", code)
	}
	code, refus = victime.appel("POST", "/auth/register", map[string]any{
		"invitation_token": texte(revoquee, "token"),
		"full_name":        "Refoulé",
		"email":            fmt.Sprintf("refoule-%d@test.local", time.Now().UnixNano()),
		"password":         "motdepasse1",
	})
	if code != 409 || codeErreur(refus) != "invitation_revoked" {
		t.Errorf("invitation révoquée : http=%d code=%q", code, codeErreur(refus))
	}

	// Un code inventé ne révèle aucune entreprise : 404, sans nom.
	code, refus = victime.appel("GET", "/invitations/ZZZZZZZZ", nil)
	if code != 404 || codeErreur(refus) != "invitation_unknown" {
		t.Errorf("code inventé : http=%d code=%q", code, codeErreur(refus))
	}
	if texte(refus, "organization_name") != "" {
		t.Errorf("un code inventé ne doit révéler aucune entreprise : %v", refus)
	}
	if message := texte(refus, "message"); strings.Contains(strings.ToLower(message), "entreprise") &&
		!strings.Contains(message, "aucune invitation") {
		t.Errorf("le refus ne doit rien laisser filtrer : %q", message)
	}
}

func TestInvitationDejaRattacheEtIsolation(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise A")
	voisin := b.client()
	voisin.inscrire("Entreprise B")

	emailExistant := fmt.Sprintf("deja-%d@test.local", time.Now().UnixNano())
	invitation := b.creerInvitation(patron, map[string]any{"email": emailExistant, "role": "cashier"})

	// Un compte existant n'est pas rattaché de force à une seconde entreprise.
	deja := b.client()
	code, _ := deja.appel("POST", "/auth/register", map[string]any{
		"organization_name": "Entreprise C",
		"full_name":         "Déjà Là",
		"email":             emailExistant,
		"password":          "motdepasse1",
		"station_name":      "Station C",
	})
	if code != 201 {
		t.Fatalf("création du compte préexistant : http=%d", code)
	}
	code, refus := deja.appel("POST", "/auth/register", map[string]any{
		"invitation_token": texte(invitation, "token"),
		"full_name":        "Déjà Là",
		"email":            emailExistant,
		"password":         "motdepasse1",
	})
	if code != 409 || codeErreur(refus) != "invitation_requires_login" {
		t.Errorf("compte déjà rattaché : http=%d code=%q", code, codeErreur(refus))
	}

	// Les invitations d'une entreprise ne sont pas visibles depuis une autre.
	code, liste := voisin.appel("GET", "/invitations", nil)
	if code != 200 {
		t.Fatalf("liste du voisin : http=%d", code)
	}
	if items, _ := liste["items"].([]any); len(items) != 0 {
		t.Errorf("le voisin voit %d invitation(s) qui ne sont pas les siennes", len(items))
	}
	// Et il ne peut pas révoquer celle d'autrui.
	code, _ = voisin.appel("POST", "/invitations/"+texte(invitation, "id")+"/revoke", nil)
	if code != 404 {
		t.Errorf("révocation de l'invitation d'autrui : http=%d (attendu 404)", code)
	}
}

func TestInvitationDonneAccesAuPosteDeTravail(t *testing.T) {
	b := nouveauBanc(t)
	patron := b.client()
	patron.inscrire("Entreprise Terrain")
	patron.activer()
	station := patron.statID

	// Un laveur invité voit ses lavages dès son inscription : il n'a pas de
	// parcours de mise en route à faire, l'entreprise est déjà en exploitation.
	invitation := b.creerInvitation(patron, map[string]any{
		"email": fmt.Sprintf("laveur-poste-%d@test.local", time.Now().UnixNano()),
		"role":  "washer",
	})
	laveur := b.rejoindreParInvitation(texte(invitation, "token"),
		fmt.Sprintf("laveur-poste-%d@test.local", time.Now().UnixNano()))
	if _, onboarding := laveur.me["onboarding"]; onboarding && laveur.me["onboarding"] != nil {
		t.Error("un laveur ne doit pas être envoyé sur le parcours de mise en route")
	}
	code, rep := laveur.appel("GET", "/washer/tasks", nil)
	if code != 200 {
		t.Fatalf("le poste du laveur doit répondre à un invité : http=%d %v", code, rep)
	}
	if tasks := rep["tasks"]; tasks == nil {
		t.Errorf("le poste du laveur doit renvoyer ses tâches : %v", rep)
	}
	_ = station
}
