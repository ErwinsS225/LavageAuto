package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrations embarquées : appliquées dans l'ordre du nom de fichier
// (0001, 0002, …) à chaque démarrage. Un tableau schema_migrations garde la
// trace des fichiers déjà joués : un nouveau fichier est pris en compte sans
// modification du code, un fichier déjà appliqué est ignoré. Les migrations
// restent idempotentes au niveau SQL (IF NOT EXISTS / ADD COLUMN IF NOT EXISTS)
// en tant que garde-fou, mais le suivi par nom garantit qu'une migration n'est
// jamais rejouée inutilement — cruciale sur Neon (branches) et PostgreSQL.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsTable contient le schéma de suivi des migrations appliquées.
// Créée avant la première migration et jamais modifiée après.
const migrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version    text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
)
`

func appliquerMigrations(ctx context.Context, db *pgxpool.Pool) error {
	// 1. Table de suivi (idempotente).
	if _, err := db.Exec(ctx, migrationsTable); err != nil {
		return fmt.Errorf("créer schema_migrations : %w", err)
	}

	// 2. Lire les fichiers de migration embarqués, dans l'ordre du nom.
	entrees, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("lister les migrations : %w", err)
	}

	for _, e := range entrees {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version := e.Name()

		// 3. Vérifier si déjà appliquée.
		var dejaAppliquee bool
		if err := db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).
			Scan(&dejaAppliquee); err != nil {
			return fmt.Errorf("vérifier migration %s : %w", version, err)
		}
		if dejaAppliquee {
			continue
		}

		// 4. Lire le corps SQL.
		chemin := "migrations/" + e.Name()
		body, err := fs.ReadFile(migrationsFS, chemin)
		if err != nil {
			return fmt.Errorf("%s : %w", version, err)
		}

		// 5. Appliquer dans une transaction : soit tout réussit et on
		//    enregistre la migration, soit la transaction est annulée.
		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("transaction %s : %w", version, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s : %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("enregistrer migration %s : %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s : %w", version, err)
		}

		log.Printf("migration %s appliquée", version)
	}
	return nil
}

// Version applicative (affichée par /health).
const versionApp = "0.1.0"

type config struct {
	host                string
	port                int
	env                 string
	databaseURL         string
	staticDir           string
	sessionTTLHeures    int
	cookieSecure        bool
	cookieSameSite      string
	authMaxFailures     int
	registerMaxParHeure int
}

func chargerConfig() config {
	c := config{
		host:                envOu("API_HOST", "0.0.0.0"),
		port:                envIntOu("API_PORT", 3000),
		env:                 envOu("APP_ENV", "development"),
		databaseURL:         envOu("DATABASE_URL", envOu("NEON_DATABASE_URL", "")),
		staticDir:           envOu("STATIC_DIR", "dist"),
		sessionTTLHeures:    envIntOu("SESSION_TTL_HOURS", 72),
		cookieSameSite:      envOu("SESSION_COOKIE_SAMESITE", "lax"),
		authMaxFailures:     envIntOu("AUTH_MAX_FAILURES", 10),
		registerMaxParHeure: envIntOu("REGISTER_MAX_PER_HOUR", 5),
	}
	c.cookieSecure = envOu("SESSION_COOKIE_SECURE", "") == "true" || c.env == "production"
	return c
}

func envOu(cle, defaut string) string {
	if v := strings.TrimSpace(os.Getenv(cle)); v != "" {
		return v
	}
	return defaut
}

func envIntOu(cle string, defaut int) int {
	if v := strings.TrimSpace(os.Getenv(cle)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaut
}

// App porte la connexion à la base et la configuration du serveur.
type App struct {
	cfg config
	db  *pgxpool.Pool
}

func nouvelleApp(cfg config) (*App, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.databaseURL)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL invalide : %w", err)
	}
	poolCfg.MaxConns = 8
	ctx, annuler := context.WithTimeout(context.Background(), 15*time.Second)
	defer annuler()
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("base injoignable : %w", err)
	}
	return &App{cfg: cfg, db: pool}, nil
}

// --- Réponses JSON ----------------------------------------------------------

type erreurAPI struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Fields  map[string]string      `json:"fields,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func ecrireJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func repondreErreur(w http.ResponseWriter, status int, code, message string) {
	ecrireJSON(w, status, map[string]interface{}{"error": erreurAPI{Code: code, Message: message}})
}

// ecrireValidation échoue une requête en nommant chaque champ fautif — le
// client affiche ces messages champ par champ (ApiError.fields côté front).
func ecrireValidation(w http.ResponseWriter, champs map[string]string) {
	message := "La saisie comporte des erreurs."
	if len(champs) == 1 {
		for _, m := range champs {
			message = m
		}
	}
	ecrireJSON(w, http.StatusBadRequest, map[string]interface{}{
		"error": erreurAPI{Code: "validation_error", Message: message, Fields: champs},
	})
}

type conflit struct {
	code    string
	message string
	fields  map[string]string
}

func ecrireConflit(w http.ResponseWriter, status int, c conflit) {
	ecrireJSON(w, status, map[string]interface{}{
		"error": erreurAPI{Code: c.code, Message: c.message, Fields: c.fields},
	})
}

func decoderCorps[T any](w http.ResponseWriter, r *http.Request) (*T, bool) {
	var corps T
	err := json.NewDecoder(r.Body).Decode(&corps)
	if err != nil {
		ecrireValidation(w, map[string]string{"body": "Corps JSON illisible."})
		return nil, false
	}
	return &corps, true
}

// decoderCorpsLibre lit un objet JSON brut pour les PATCH « champ présent =
// champ modifié », où distinguer null de absent est indispensable.
func decoderCorpsLibre(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, bool) {
	var brut map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&brut); err != nil {
		ecrireValidation(w, map[string]string{"body": "Corps JSON illisible."})
		return nil, false
	}
	if brut == nil {
		brut = map[string]json.RawMessage{}
	}
	return brut, true
}

// cheminParam extrait la portion de chemin après le préfixe (le routeur
// standard ne fournit pas de capture nommée).
func cheminParam(r *http.Request, prefixe string) string {
	reste := strings.TrimPrefix(r.URL.Path, prefixe)
	return strings.Trim(strings.SplitN(reste, "/", 2)[0], "/")
}

func queryInt(r *http.Request, cle string, defaut int) int {
	if v := strings.TrimSpace(r.URL.Query().Get(cle)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaut
}

func jeterJeton(nOctets int) string {
	b := make([]byte, nOctets)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hacherJeton(jeton string) string {
	return "sha256:" + jeton // empreinte via pgcrypto en base ; ici le jeton est déjà aléatoire long
}

// --- Routeur ----------------------------------------------------------------

type route struct {
	methode    string
	prefixe    string // se termine par '/' si sous-arbre
	handler    func(http.ResponseWriter, *http.Request, *sessionUtilisateur)
	public     bool // pas de session requise
	permission string
}

type routeur struct {
	routes []route
	app    *App
}

// avecSession applique la middleware optionnelle des routes publiques :
// si un cookie de session valide accompagne la requête, le handler reçoit la
// session (utile à /auth/register pour détecter « déjà connecté »), sinon nil.
func (rt *routeur) avecSession(h func(http.ResponseWriter, *http.Request, *sessionUtilisateur)) func(http.ResponseWriter, *http.Request, *sessionUtilisateur) {
	return func(w http.ResponseWriter, r *http.Request, _ *sessionUtilisateur) {
		h(w, r, rt.app.lireSession(r))
	}
}

// interdire : garde-fou commun aux écritures d'exploitation. Tant que la mise
// en route n'est pas terminée, un poste de caisse vide serait ouvert au public.
func (a *App) interdire(w http.ResponseWriter, r *http.Request, sess *sessionUtilisateur) bool {
	if sess.OrganizationID == nil || *sess.OrganizationID == "" {
		return false
	}
	if a.etatOnboarding(r.Context(), *sess.OrganizationID) == "active" {
		return false
	}
	champs := map[string]string{}
	blocs := a.prerequisOnboarding(r.Context(), *sess.OrganizationID)
	for _, b := range blocs {
		champs[b.code] = b.hint
	}
	ecrireJSON(w, http.StatusConflict, map[string]interface{}{
		"error": erreurAPI{
			Code: "onboarding_incomplete", Message: "La station n'est pas encore activée : terminez la mise en route.",
			Fields: champs, Details: map[string]interface{}{"blockers": blocs},
		},
	})
	return true
}

// bloqueeAvantActivation résume interdire pour les handlers qui écrivent leur
// propre message d'erreur métier avant de tenter l'écriture.
func (a *App) stationActive(ctx context.Context, organizationID string) bool {
	return a.etatOnboarding(ctx, organizationID) == "active"
}

func (rt *routeur) ajouter(methode, chemin string, h func(http.ResponseWriter, *http.Request, *sessionUtilisateur)) {
	rt.routes = append(rt.routes, route{methode: methode, prefixe: chemin, handler: h})
}

func (rt *routeur) proteger(methode, chemin, permission string, h func(http.ResponseWriter, *http.Request, *sessionUtilisateur)) {
	rt.routes = append(rt.routes, route{methode: methode, prefixe: chemin, handler: h, permission: permission})
}

// servirHTTP adapte le routeur au format http.Handler exigé par ServeMux.
func (rt *routeur) servirHTTP(w http.ResponseWriter, r *http.Request) {
	rt.servir(w, r)
}

func (rt *routeur) servir(w http.ResponseWriter, r *http.Request) {
	var premiere405 *route
	for _, ro := range rt.routes {
		if ro.methode != r.Method {
			// mémorise le préfixe correspondant : si aucune route ne matche la
			// méthode mais qu'un sous-arbre existe, on répondra 405 au lieu de 404.
			if strings.HasSuffix(ro.prefixe, "/") && strings.HasPrefix(r.URL.Path, ro.prefixe) && premiere405 == nil {
				premiere405 = &ro
			}
			continue
		}
		match := false
		if strings.HasSuffix(ro.prefixe, "/") {
			match = strings.HasPrefix(r.URL.Path, ro.prefixe)
		} else {
			match = r.URL.Path == ro.prefixe
		}
		if !match {
			continue
		}
		var sess *sessionUtilisateur
		if !ro.public {
			s := rt.app.lireSession(r)
			if s == nil {
				repondreErreur(w, http.StatusUnauthorized, "unauthorized", "Session absente ou expirée : reconnectez-vous.")
				return
			}
			if ro.permission != "" && !s.aPermission(ro.permission) {
				repondreErreur(w, http.StatusForbidden, "forbidden", "Votre rôle ne permet pas cette action.")
				return
			}
			sess = s
		}
		ro.handler(w, r, sess)
		return
	}
	if premiere405 != nil {
		repondreErreur(w, http.StatusMethodNotAllowed, "method_not_allowed", "Méthode non autorisée pour cette ressource.")
		return
	}
	repondreErreur(w, http.StatusNotFound, "not_found", "Ressource introuvable.")
}

func (a *App) nouveauRouteur() *routeur {
	rt := &routeur{app: a}
	a.enregistrerRoutesAuth(rt)
	a.enregistrerRoutesOnboarding(rt)
	a.enregistrerRoutesOrganisation(rt)
	a.enregistrerRoutesStations(rt)
	a.enregistrerRoutesReferentiels(rt)
	a.enregistrerRoutesCatalogue(rt)
	a.enregistrerRoutesTarifs(rt)
	a.enregistrerRoutesEquipe(rt)
	a.enregistrerRoutesLavages(rt)
	a.enregistrerRoutesCaisse(rt)
	a.enregistrerRoutesStock(rt)
	a.enregistrerRoutesRegles(rt)
	a.enregistrerRapports(rt)
	return rt
}

func main() {
	migrateSeulement := len(os.Args) > 1 && os.Args[1] == "-migrate-only"
	cfg := chargerConfig()
	if cfg.databaseURL == "" {
		log.Fatal("DATABASE_URL (ou NEON_DATABASE_URL) est requis.")
	}
	app, err := nouvelleApp(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer app.db.Close()

	ctx := context.Background()
	if err := appliquerMigrations(ctx, app.db); err != nil {
		log.Fatalf("migrations : %v", err)
	}
	if err := app.amorcerReferentiels(ctx); err != nil {
		log.Fatalf("référentiels : %v", err)
	}
	if err := app.amorcerPlans(ctx); err != nil {
		log.Fatalf("plans : %v", err)
	}
	if migrateSeulement {
		fmt.Println("Migrations appliquées.")
		return
	}

	mux := http.NewServeMux()
	rt := app.nouveauRouteur()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ecrireJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok", "environment": cfg.env, "version": versionApp, "time": time.Now().UTC().Format(time.RFC3339),
		})
	})
	mux.Handle("/api/v1/", http.HandlerFunc(rt.servirHTTP))
	mux.Handle("/api/v1", http.HandlerFunc(rt.servirHTTP))
	if cfg.staticDir != "" {
		if _, err := os.Stat(cfg.staticDir); err == nil {
			mux.Handle("/", servirFrontend(cfg.staticDir))
		}
	} else if _, err := os.Stat(filepath.Join("..", "dist")); err == nil {
		mux.Handle("/", servirFrontend(filepath.Join("..", "dist")))
	}

	adresse := fmt.Sprintf("%s:%d", cfg.host, cfg.port)
	log.Printf("WashGuard API %s (%s) sur http://%s", versionApp, cfg.env, adresse)
	server := &http.Server{Addr: adresse, Handler: avecCORS(cfg, mux), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}

func avecCORS(cfg config, h http.Handler) http.Handler {
	origines := strings.Split(envOu("CORS_ALLOWED_ORIGINS", ""), ",")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origine := r.Header.Get("Origin")
		autorise := false
		for _, o := range origines {
			if strings.TrimSpace(o) == origine && origine != "" {
				autorise = true
			}
		}
		if autorise {
			w.Header().Set("Access-Control-Allow-Origin", origine)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func servirFrontend(dossier string) http.Handler {
	fs := http.FileServer(http.Dir(dossier))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chef := filepath.Join(dossier, r.URL.Path)
		if st, err := os.Stat(chef); err != nil || st.IsDir() {
			// SPA : toute route inconnue sert index.html.
			http.ServeFile(w, r, filepath.Join(dossier, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}
