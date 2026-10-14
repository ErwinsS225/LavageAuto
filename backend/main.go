// WashGuard — API d'administration de station de lavage.
//
// Le serveur Go remplace l'ancien server.ts : mêmes routes /api/v1, mêmes
// formes de réponses JSON, mais persistance PostgreSQL (Neon) et mots de passe
// hachés (bcrypt) au lieu du stockage en mémoire.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	// Option d'exploitation : applique le schéma puis s'arrête sans servir.
	// Utile à l'installation d'une station (cf. scripts/installer-station.sh).
	migrateOnly := false
	for _, arg := range os.Args[1:] {
		if arg == "-migrate-only" {
			migrateOnly = true
		}
	}

	loadDotEnv()
	cfg := loadConfig()

	pool, err := openDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connexion base impossible : %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrate(ctx, pool); err != nil {
		log.Fatalf("migration impossible : %v", err)
	}

	if migrateOnly {
		log.Println("migrations appliquées, arrêt demandé (-migrate-only)")
		return
	}

	configureRateLimits(cfg)
	app := &App{Pool: pool, Cfg: cfg}
	// Le référentiel des véhicules et des usages est amorcé à chaque démarrage :
	// une version du produit qui ajoute un modèle le rend disponible dans les
	// bases déjà installées, sans dupliquer les entrées existantes.
	if err := app.preparerReferentiels(ctx); err != nil {
		log.Printf("avertissement : amorçage des référentiels impossible : %v", err)
	}
	if err := app.initClerk(); err != nil {
		log.Printf("avertissement : Clerk inactif : %v", err)
	}
	if cfg.SeedDemo {
		if err := app.seedDemo(ctx); err != nil {
			log.Printf("avertissement : seed démo impossible : %v", err)
		}
	}

	// Sessions expirées purgées au démarrage puis toutes les six heures :
	// la table ne conserve pas indéfiniment des jetons inutilisables.
	app.purgeExpiredSessions(ctx)
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			app.purgeExpiredSessions(context.Background())
		}
	}()

	router := app.buildRouter()

	srv := &http.Server{
		Addr:              cfg.ListenAddr(),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("WashGuard API (Go) démarrée sur http://%s (env=%s)", cfg.ListenAddr(), cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serveur : %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("arrêt demandé, fermeture gracieuse…")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

// withWebhookSecret place le secret du webhook Clerk dans le contexte (injection
// facile à tester, évite une variable globale).
func withWebhookSecret(cfg Config) func(http.HandlerFunc) http.HandlerFunc {
	secret := os.Getenv("CLERK_WEBHOOK_SIGNING_SECRET")
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), clerkWebhookSecretKey, secret)
			next(w, r.WithContext(ctx))
		}
	}
}

// buildRouter monte toutes les routes de l'API et le front statique.
func (a *App) buildRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"environment": a.Cfg.Env,
			"time":        time.Now().UTC().Format(timeLayout),
		})
	})

	// --- API v1 -----------------------------------------------------------
	//
	// Chaque route déclare la permission qu'elle exige ; le serveur la vérifie
	// (a.requirePermission) au lieu de compter sur le menu du front. Les
	// routes sans permission explicite sont accessibles à toute session
	// valide (par exemple /auth/me ou la liste des laveurs d'une station).

	mux.HandleFunc("GET /api/v1/plans", a.handleListPlansPublic)

	mux.HandleFunc("POST /api/v1/auth/clerk", a.handleClerkLogin)
	mux.HandleFunc("POST /api/v1/webhooks/clerk", withWebhookSecret(a.Cfg)(a.handleClerkWebhook))
	mux.HandleFunc("POST /api/v1/auth/login", a.handleLogin)
	// L'inscription est publique : elle crée elle-même le premier compte
	// propriétaire, elle ne peut donc pas exiger une session existante.
	mux.HandleFunc("POST /api/v1/auth/register", a.rateLimitRegistration(a.handleRegister))
	mux.HandleFunc("POST /api/v1/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/v1/auth/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("PATCH /api/v1/auth/password", a.requireAuth(a.handleChangePassword))

	// Invitations : rejoindre une entreprise qui existe déjà. L'aperçu est
	// public (l'invité n'a pas encore de compte) et ne renvoie aucun
	// identifiant ; le reste demande employee:manage.
	mux.HandleFunc("GET /api/v1/invitations/{code}", a.handleInvitationPreview)
	mux.HandleFunc("GET /api/v1/invitations", a.requirePermission("employee:view", a.handleInvitationsList))
	mux.HandleFunc("POST /api/v1/invitations", a.requirePermission("employee:manage", a.handleInvitationCreate))
	mux.HandleFunc("POST /api/v1/invitations/{id}/revoke", a.requirePermission("employee:manage", a.handleInvitationRevoke))

	// Onboarding : lecture de l'état, enregistrement d'une étape, activation.
	// L'activation exige organization:manage (elle engage l'exploitation) ;
	// l'enregistrement d'une étape demande settings:manage (un gérant peut
	// préparer la configuration, il ne peut pas activer la station à la place
	// du propriétaire).
	mux.HandleFunc("GET /api/v1/onboarding", a.requirePermission("settings:view", a.handleOnboardingGet))
	mux.HandleFunc("PATCH /api/v1/onboarding/steps/{code}", a.requirePermission("settings:manage", a.handleOnboardingStep))
	mux.HandleFunc("POST /api/v1/onboarding/activate", a.requirePermission("organization:manage", a.handleOnboardingActivate))

	mux.HandleFunc("GET /api/v1/organizations/current", a.requireAuth(a.handleOrgCurrent))
	mux.HandleFunc("PATCH /api/v1/organizations/current", a.requirePermission("settings:manage", a.handleOrgUpdate))
	mux.HandleFunc("GET /api/v1/organizations/subscription", a.requirePermission("subscription:view", a.handleOrgSubscription))
	mux.HandleFunc("POST /api/v1/organizations/subscription/request", a.requirePermission("subscription:manage", a.handleSubscriptionRequest))
	mux.HandleFunc("GET /api/v1/organizations/audit", a.requirePermission("audit:view", a.handleOrgAudit))

	mux.HandleFunc("GET /api/v1/stations", a.requirePermission("station:view", a.handleStationsList))
	mux.HandleFunc("POST /api/v1/stations", a.requirePermission("organization:manage", a.handleStationCreate))
	mux.HandleFunc("PATCH /api/v1/stations/{id}", a.requirePermission("settings:manage", a.handleStationUpdate))
	// Fiche complète d'une station : localisation, capacités, horaires. C'est ce
	// que lisent les étapes « station » et « horaires » du parcours de mise en
	// route, et l'onglet Paramètres.
	mux.HandleFunc("GET /api/v1/stations/{id}/profile", a.requirePermission("settings:view", a.handleStationProfile))
	// Les horaires sont validés avant d'être enregistrés : une journée qui ferme
	// avant d'ouvrir rendrait faux le statut d'ouverture calculé ensuite.
	mux.HandleFunc("PUT /api/v1/stations/{id}/opening-hours", a.requirePermission("settings:manage", a.handleStationOpeningHours))

	// Catégories de véhicules et usages commerciaux : le référentiel est commun
	// au produit, ce que la station en active lui appartient. `GET
	// /vehicle-category-options` sert la caisse (les deux listes en un appel) ;
	// `GET /vehicle-catalogue` sert l'écran d'activation (modèle + état local).
	mux.HandleFunc("GET /api/v1/vehicle-categories", a.requirePermission("station:view", a.handleVehicleCategories))
	mux.HandleFunc("POST /api/v1/vehicle-categories", a.requirePermission("settings:manage", a.handleVehicleCategoryCreate))
	mux.HandleFunc("PATCH /api/v1/vehicle-categories/{code}", a.requirePermission("settings:manage", a.handleVehicleCategoryUpdate))
	mux.HandleFunc("GET /api/v1/vehicle-usages", a.requirePermission("station:view", a.handleVehicleUsages))
	mux.HandleFunc("GET /api/v1/vehicle-catalogue", a.requirePermission("station:view", a.handleVehicleCatalogue))
	mux.HandleFunc("GET /api/v1/vehicle-category-options", a.requirePermission("wash:create", a.handleVehicleOptions))
	mux.HandleFunc("PUT /api/v1/stations/{id}/vehicle-categories", a.requirePermission("settings:manage", a.handleStationVehicleCategories))
	mux.HandleFunc("PUT /api/v1/stations/{id}/vehicle-usages", a.requirePermission("settings:manage", a.handleStationVehicleUsages))
	mux.HandleFunc("GET /api/v1/vehicle-usage-categories", a.requirePermission("station:view", a.handleUsageCategories))
	mux.HandleFunc("PUT /api/v1/stations/{id}/vehicle-usage-categories", a.requirePermission("settings:manage", a.handleStationUsageCategories))

	mux.HandleFunc("GET /api/v1/services", a.requirePermission("service:view", a.handleServicesList))
	mux.HandleFunc("POST /api/v1/services", a.requirePermission("service:manage", a.handleServiceCreate))
	mux.HandleFunc("PATCH /api/v1/services/{id}", a.requirePermission("service:manage", a.handleServiceUpdate))
	mux.HandleFunc("DELETE /api/v1/services/{id}", a.requirePermission("service:manage", a.handleServiceDisable))

	// Prestations : le catalogue de référence est **sans prix** (un modèle n'est
	// pas un tarif). Ce que la station en fait — familles activées,
	// compatibilités avec ses véhicules, options — vit dans ses propres tables.
	mux.HandleFunc("GET /api/v1/service-templates", a.requirePermission("service:view", a.handleServiceTemplates))
	mux.HandleFunc("GET /api/v1/service-families", a.requirePermission("service:view", a.handleServiceFamilies))
	mux.HandleFunc("PUT /api/v1/stations/{id}/service-families", a.requirePermission("settings:manage", a.handleStationServiceFamilies))
	mux.HandleFunc("GET /api/v1/services/{id}/compatibilities", a.requirePermission("service:view", a.handleServiceCompatibilities))
	mux.HandleFunc("PUT /api/v1/services/{id}/compatibilities", a.requirePermission("service:manage", a.handleServiceCompatibilitiesUpdate))
	mux.HandleFunc("GET /api/v1/service-options", a.requirePermission("service:view", a.handleServiceOptions))
	mux.HandleFunc("POST /api/v1/service-options", a.requirePermission("service:manage", a.handleServiceOptionCreate))
	mux.HandleFunc("PATCH /api/v1/service-options/{id}", a.requirePermission("service:manage", a.handleServiceOptionUpdate))
	mux.HandleFunc("DELETE /api/v1/service-options/{id}", a.requirePermission("service:manage", a.handleServiceOptionDisable))

	// Grille tarifaire : les règles qui décident du prix, la matrice qui les
	// rend lisibles, et l'aperçu que la caisse affiche avant d'enregistrer une
	// fiche. Aucun montant n'est calculé par le navigateur.
	mux.HandleFunc("GET /api/v1/price-rules", a.requirePermission("service:view", a.handlePriceRulesList))
	mux.HandleFunc("POST /api/v1/price-rules", a.requirePermission("service:manage", a.handlePriceRuleCreate))
	mux.HandleFunc("GET /api/v1/price-rules/matrix", a.requirePermission("service:view", a.handlePriceMatrix))
	mux.HandleFunc("PUT /api/v1/price-rules/matrix", a.requirePermission("service:manage", a.handlePriceMatrixUpdate))
	mux.HandleFunc("PATCH /api/v1/price-rules/{id}", a.requirePermission("service:manage", a.handlePriceRuleUpdate))
	mux.HandleFunc("DELETE /api/v1/price-rules/{id}", a.requirePermission("service:manage", a.handlePriceRuleDisable))
	mux.HandleFunc("GET /api/v1/price-preview", a.requirePermission("wash:create", a.handlePricePreview))

	mux.HandleFunc("GET /api/v1/customers", a.requirePermission("customer:view", a.handleCustomersList))
	mux.HandleFunc("GET /api/v1/customers/options", a.requirePermission("customer:view", a.handleCustomerOptions))
	mux.HandleFunc("POST /api/v1/customers", a.requirePermission("customer:create", a.handleCustomerCreate))
	mux.HandleFunc("GET /api/v1/customers/{id}", a.requirePermission("customer:view", a.handleCustomerDetail))
	mux.HandleFunc("PATCH /api/v1/customers/{id}", a.requirePermission("customer:manage", a.handleCustomerUpdate))
	mux.HandleFunc("DELETE /api/v1/customers/{id}", a.requirePermission("customer:manage", a.handleCustomerArchive))

	// La liste des employés d'une station sert au caissier pour affecter un
	// laveur : elle reste ouverte à toute session, mais elle est cloisonnée.
	mux.HandleFunc("GET /api/v1/employees/options", a.requireAuth(a.handleEmployeeOptions))
	mux.HandleFunc("GET /api/v1/employees", a.requirePermission("employee:view", a.handleEmployeesList))
	mux.HandleFunc("POST /api/v1/employees", a.requirePermission("employee:manage", a.handleEmployeeCreate))
	mux.HandleFunc("GET /api/v1/employees/{id}/activity", a.requirePermission("employee:view", a.handleEmployeeActivity))
	mux.HandleFunc("PATCH /api/v1/employees/{id}", a.requirePermission("employee:manage", a.handleEmployeeUpdate))
	mux.HandleFunc("POST /api/v1/employees/{id}/reset-password", a.requirePermission("employee:manage", a.handleEmployeeResetPassword))

	mux.HandleFunc("GET /api/v1/wash-orders", a.requirePermission("wash:view", a.handleWashOrdersList))
	// requireOnboarding : une station non activée ne vend pas (le message
	// d'erreur indique les étapes restantes, l'interface y renvoie).
	mux.HandleFunc("POST /api/v1/wash-orders", a.requirePermission("wash:create", a.requireOnboarding(a.handleWashOrderCreate)))
	mux.HandleFunc("GET /api/v1/wash-orders/{id}", a.requirePermission("wash:view", a.handleWashOrderDetail))
	// Reçu : le justificatif porte le numéro attribué quand la fiche a été
	// soldée. La lecture et l'émission exigent la permission de caisse — un
	// justificatif est une pièce comptable.
	mux.HandleFunc("GET /api/v1/wash-orders/{id}/receipt", a.requirePermission("payment:view", a.handleReceiptRead))
	mux.HandleFunc("POST /api/v1/wash-orders/{id}/receipt", a.requirePermission("payment:view", a.handleReceiptIssue))
	mux.HandleFunc("PATCH /api/v1/wash-orders/{id}", a.requirePermission("wash:update", a.handleWashOrderUpdate))
	mux.HandleFunc("POST /api/v1/wash-orders/{id}/assign", a.requirePermission("wash:assign", a.handleWashOrderAssign))
	mux.HandleFunc("POST /api/v1/wash-orders/{id}/cancel", a.requirePermission("wash:cancel", a.handleWashOrderCancel))
	mux.HandleFunc("POST /api/v1/wash-orders/{id}/payments", a.requirePermission("payment:create", a.requireOnboarding(a.handleWashOrderPayment)))

	mux.HandleFunc("GET /api/v1/cash/current", a.requirePermission("cash:view", a.handleCashCurrent))
	mux.HandleFunc("GET /api/v1/cash/sessions", a.requirePermission("cash:view", a.handleCashSessionsList))
	mux.HandleFunc("POST /api/v1/cash/sessions", a.requirePermission("cash:open", a.requireOnboarding(a.handleCashSessionOpen)))
	mux.HandleFunc("GET /api/v1/cash/sessions/{id}", a.requirePermission("cash:view", a.handleCashSessionDetail))
	mux.HandleFunc("POST /api/v1/cash/sessions/{id}/close", a.requirePermission("cash:close", a.handleCashSessionClose))
	mux.HandleFunc("POST /api/v1/cash/sessions/{id}/reopen", a.requirePermission("cash:reopen", a.handleCashSessionReopen))

	mux.HandleFunc("GET /api/v1/cash/payments", a.requirePermission("payment:view", a.handlePaymentsList))
	mux.HandleFunc("POST /api/v1/cash/payments", a.requirePermission("payment:create", a.requireOnboarding(a.handlePaymentCreate)))
	mux.HandleFunc("POST /api/v1/cash/payments/{id}/void", a.requirePermission("payment:void", a.handlePaymentVoid))

	// Moyens de paiement de la station : la caisse les lit, le gérant les
	// configure (nom local, activation, ordre, moyen personnalisé).
	mux.HandleFunc("GET /api/v1/payment-methods", a.requirePermission("payment:view", a.handlePaymentMethodsList))
	mux.HandleFunc("POST /api/v1/payment-methods", a.requirePermission("settings:manage", a.handlePaymentMethodCreate))
	mux.HandleFunc("PATCH /api/v1/payment-methods/{id}", a.requirePermission("settings:manage", a.handlePaymentMethodUpdate))
	mux.HandleFunc("DELETE /api/v1/payment-methods/{id}", a.requirePermission("settings:manage", a.handlePaymentMethodDisable))

	// Paramètres de caisse de la station : fond proposé, écart toléré, rôles
	// autorisés, ouverture des annulations, identité des reçus.
	mux.HandleFunc("GET /api/v1/stations/{id}/cash-settings", a.requirePermission("cash:view", a.handleCashSettingsRead))
	mux.HandleFunc("PUT /api/v1/stations/{id}/cash-settings", a.requirePermission("settings:manage", a.handleCashSettingsUpdate))

	// Matériel de la station : le référentiel des modèles, les équipements
	// déclarés, et leur retrait (désactivation, jamais suppression).
	mux.HandleFunc("GET /api/v1/stations/{id}/equipment", a.requirePermission("settings:view", a.handleEquipmentList))
	mux.HandleFunc("POST /api/v1/stations/{id}/equipment", a.requirePermission("settings:manage", a.handleEquipmentCreate))
	mux.HandleFunc("PATCH /api/v1/equipment/{id}", a.requirePermission("settings:manage", a.handleEquipmentUpdate))

	// Stock : catégories amorcées, articles, seuils, mouvements. La quantité
	// ne s'écrit pas à la main : elle bouge par un mouvement, qui laisse une
	// trace lisible.
	mux.HandleFunc("GET /api/v1/stations/{id}/stock", a.requirePermission("settings:view", a.handleStockList))
	mux.HandleFunc("POST /api/v1/stations/{id}/stock/items", a.requirePermission("settings:manage", a.handleStockItemCreate))
	mux.HandleFunc("PATCH /api/v1/stock/items/{id}", a.requirePermission("settings:manage", a.handleStockItemUpdate))
	mux.HandleFunc("GET /api/v1/stock/items/{id}/movements", a.requirePermission("settings:view", a.handleStockMovements))
	mux.HandleFunc("POST /api/v1/stock/items/{id}/movements", a.requirePermission("settings:manage", a.handleStockMovementCreate))

	// Règles de fonctionnement des commandes : ce que la station accepte
	// (clients de passage, laveur obligatoire, contrôle qualité, acomptes,
	// remise du véhicule) — chacune appliquée par le serveur.
	mux.HandleFunc("GET /api/v1/stations/{id}/order-rules", a.requirePermission("settings:view", a.handleOrderRulesRead))
	mux.HandleFunc("PUT /api/v1/stations/{id}/order-rules", a.requirePermission("settings:manage", a.handleOrderRulesUpdate))

	mux.HandleFunc("GET /api/v1/expenses", a.requirePermission("expense:view", a.handleExpensesList))
	mux.HandleFunc("POST /api/v1/expenses", a.requirePermission("expense:create", a.requireOnboarding(a.handleExpenseCreate)))
	mux.HandleFunc("PATCH /api/v1/expenses/{id}", a.requirePermission("expense:approve", a.handleExpenseUpdate))
	mux.HandleFunc("POST /api/v1/expenses/{id}/void", a.requirePermission("expense:void", a.handleExpenseVoid))

	mux.HandleFunc("GET /api/v1/reports/dashboard", a.requirePermission("dashboard:view", a.handleReportDashboard))
	mux.HandleFunc("GET /api/v1/reports/overview", a.requirePermission("report:view", a.handleReportOverview))
	mux.HandleFunc("GET /api/v1/reports/export", a.requirePermission("report:export", a.handleReportExport))

	mux.HandleFunc("GET /api/v1/commissions", a.requirePermission("commission:view", a.handleCommissions))
	mux.HandleFunc("GET /api/v1/commissions/export", a.requirePermission("commission:view", a.handleCommissionsExport))

	// Les handlers /platform/* vérifient eux-mêmes le rôle superadmin
	// (requireSuperadmin dans handlers_platform.go) : la permission
	// platform:manage n'est accordée qu'à ce rôle.
	mux.HandleFunc("GET /api/v1/platform/overview", a.requirePermission("platform:manage", a.handlePlatformOverview))
	mux.HandleFunc("GET /api/v1/platform/organizations", a.requirePermission("platform:manage", a.handlePlatformOrgsList))
	mux.HandleFunc("GET /api/v1/platform/organizations/{id}", a.requirePermission("platform:manage", a.handlePlatformOrgDetail))
	mux.HandleFunc("PATCH /api/v1/platform/organizations/{id}", a.requirePermission("platform:manage", a.handlePlatformOrgUpdate))
	mux.HandleFunc("POST /api/v1/platform/organizations/{id}/subscription", a.requirePermission("platform:manage", a.handlePlatformOrgSubscription))
	mux.HandleFunc("GET /api/v1/platform/plans", a.requirePermission("platform:manage", a.handlePlatformPlansList))
	mux.HandleFunc("PATCH /api/v1/platform/plans/{id}", a.requirePermission("platform:manage", a.handlePlatformPlanUpdate))
	mux.HandleFunc("GET /api/v1/platform/audit", a.requirePermission("platform:manage", a.handlePlatformAudit))
	// Une mention administrative (RCCM, compte contribuable…) n'est « vérifiée »
	// que par le support WashGuard : l'entreprise la renseigne, elle ne peut pas
	// se certifier elle-même. L'opération est journalisée.
	mux.HandleFunc("POST /api/v1/platform/organizations/{id}/registry/{code}/verify", a.requirePermission("platform:manage", a.handlePlatformRegistryVerify))

	// Espace laveur
	mux.HandleFunc("GET /api/v1/washer/tasks", a.requirePermission("washer:view", a.handleWasherTasks))
	mux.HandleFunc("POST /api/v1/washer/tasks/{id}/status", a.requirePermission("washer:update_status", a.handleWasherTaskStatus))
	mux.HandleFunc("POST /api/v1/washer/tasks/{id}/incident", a.requirePermission("washer:report_incident", a.handleWasherTaskIncident))
	// Statistiques personnelles : la permission dédiée (washer:personal_stats)
	// était déclarée dans la matrice des rôles sans être vérifiée nulle part.
	// Elle est donc la seule à ouvrir cet écran.
	mux.HandleFunc("GET /api/v1/washer/me/performance", a.requirePermission("washer:personal_stats", a.handleWasherPerformance))

	// Front statique (build Vite) + repli SPA
	if a.Cfg.StaticDir != "" {
		static := a.Cfg.StaticDir
		files := http.FileServer(http.Dir(static))
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/health") {
				writeError(w, http.StatusNotFound, "not_found", "Route inconnue.", nil)
				return
			}
			// Fichier réel présent dans le dossier public (favicon, image,
			// robots.txt…) : on le sert tel quel. Toute autre adresse retombe
			// sur l'application, qui gère le routage côté navigateur : un lien
			// profond comme /tableau-de-bord doit ouvrir la page, pas un 404.
			if info, err := os.Stat(filepath.Join(static, filepath.Clean("/"+r.URL.Path))); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(static, "index.html"))
		})
	}

	return withCORS(a.Cfg)(mux)
}
