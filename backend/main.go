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

	app := &App{Pool: pool, Cfg: cfg}
	if err := app.initClerk(); err != nil {
		log.Printf("avertissement : Clerk inactif : %v", err)
	}
	if cfg.SeedDemo {
		if err := app.seedDemo(ctx); err != nil {
			log.Printf("avertissement : seed démo impossible : %v", err)
		}
	}

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
			"time":        time.Now().UTC().Format(time.RFC3339),
		})
	})

	// --- API v1 -----------------------------------------------------------
	mux.HandleFunc("GET /api/v1/plans", a.handleListPlansPublic)

	mux.HandleFunc("POST /api/v1/auth/clerk", a.handleClerkLogin)
	mux.HandleFunc("POST /api/v1/webhooks/clerk", withWebhookSecret(a.Cfg)(a.handleClerkWebhook))
	mux.HandleFunc("POST /api/v1/auth/login", a.handleLogin)
	// L'inscription est publique : elle crée elle-même le premier compte
	// propriétaire, elle ne peut donc pas exiger une session existante.
	mux.HandleFunc("POST /api/v1/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/logout", a.handleLogout)
	mux.HandleFunc("GET /api/v1/auth/me", a.requireAuth(a.handleMe))
	mux.HandleFunc("PATCH /api/v1/auth/password", a.requireAuth(a.handleChangePassword))

	mux.HandleFunc("GET /api/v1/organizations/current", a.requireAuth(a.handleOrgCurrent))
	mux.HandleFunc("PATCH /api/v1/organizations/current", a.requireAuth(a.handleOrgUpdate))
	mux.HandleFunc("GET /api/v1/organizations/subscription", a.requireAuth(a.handleOrgSubscription))
	mux.HandleFunc("POST /api/v1/organizations/subscription/request", a.requireAuth(a.handleSubscriptionRequest))
	mux.HandleFunc("GET /api/v1/organizations/audit", a.requireAuth(a.handleOrgAudit))

	mux.HandleFunc("GET /api/v1/stations", a.requireAuth(a.handleStationsList))
	mux.HandleFunc("POST /api/v1/stations", a.requireAuth(a.handleStationCreate))
	mux.HandleFunc("PATCH /api/v1/stations/{id}", a.requireAuth(a.handleStationUpdate))

	mux.HandleFunc("GET /api/v1/vehicle-categories", a.requireAuth(a.handleVehicleCategories))

	mux.HandleFunc("GET /api/v1/services", a.requireAuth(a.handleServicesList))
	mux.HandleFunc("POST /api/v1/services", a.requireAuth(a.handleServiceCreate))
	mux.HandleFunc("PATCH /api/v1/services/{id}", a.requireAuth(a.handleServiceUpdate))
	mux.HandleFunc("DELETE /api/v1/services/{id}", a.requireAuth(a.handleServiceDisable))

	mux.HandleFunc("GET /api/v1/customers", a.requireAuth(a.handleCustomersList))
	mux.HandleFunc("POST /api/v1/customers", a.requireAuth(a.handleCustomerCreate))
	mux.HandleFunc("GET /api/v1/customers/{id}", a.requireAuth(a.handleCustomerDetail))
	mux.HandleFunc("PATCH /api/v1/customers/{id}", a.requireAuth(a.handleCustomerUpdate))
	mux.HandleFunc("DELETE /api/v1/customers/{id}", a.requireAuth(a.handleCustomerArchive))

	mux.HandleFunc("GET /api/v1/employees/options", a.requireAuth(a.handleEmployeeOptions))
	mux.HandleFunc("GET /api/v1/employees", a.requireAuth(a.handleEmployeesList))
	mux.HandleFunc("POST /api/v1/employees", a.requireAuth(a.handleEmployeeCreate))
	mux.HandleFunc("GET /api/v1/employees/{id}/activity", a.requireAuth(a.handleEmployeeActivity))
	mux.HandleFunc("PATCH /api/v1/employees/{id}", a.requireAuth(a.handleEmployeeUpdate))
	mux.HandleFunc("POST /api/v1/employees/{id}/reset-password", a.requireAuth(a.handleEmployeeResetPassword))

	mux.HandleFunc("GET /api/v1/wash-orders", a.requireAuth(a.handleWashOrdersList))
	mux.HandleFunc("POST /api/v1/wash-orders", a.requireAuth(a.handleWashOrderCreate))
	mux.HandleFunc("GET /api/v1/wash-orders/{id}", a.requireAuth(a.handleWashOrderDetail))
	mux.HandleFunc("PATCH /api/v1/wash-orders/{id}", a.requireAuth(a.handleWashOrderUpdate))
	mux.HandleFunc("POST /api/v1/wash-orders/{id}/assign", a.requireAuth(a.handleWashOrderAssign))
	mux.HandleFunc("POST /api/v1/wash-orders/{id}/cancel", a.requireAuth(a.handleWashOrderCancel))
	mux.HandleFunc("POST /api/v1/wash-orders/{id}/payments", a.requireAuth(a.handleWashOrderPayment))

	mux.HandleFunc("GET /api/v1/cash/current", a.requireAuth(a.handleCashCurrent))
	mux.HandleFunc("GET /api/v1/cash/sessions", a.requireAuth(a.handleCashSessionsList))
	mux.HandleFunc("POST /api/v1/cash/sessions", a.requireAuth(a.handleCashSessionOpen))
	mux.HandleFunc("GET /api/v1/cash/sessions/{id}", a.requireAuth(a.handleCashSessionDetail))
	mux.HandleFunc("POST /api/v1/cash/sessions/{id}/close", a.requireAuth(a.handleCashSessionClose))
	mux.HandleFunc("POST /api/v1/cash/sessions/{id}/reopen", a.requireAuth(a.handleCashSessionReopen))

	mux.HandleFunc("GET /api/v1/cash/payments", a.requireAuth(a.handlePaymentsList))
	mux.HandleFunc("POST /api/v1/cash/payments", a.requireAuth(a.handlePaymentCreate))
	mux.HandleFunc("POST /api/v1/cash/payments/{id}/void", a.requireAuth(a.handlePaymentVoid))

	mux.HandleFunc("GET /api/v1/expenses", a.requireAuth(a.handleExpensesList))
	mux.HandleFunc("POST /api/v1/expenses", a.requireAuth(a.handleExpenseCreate))
	mux.HandleFunc("PATCH /api/v1/expenses/{id}", a.requireAuth(a.handleExpenseUpdate))
	mux.HandleFunc("POST /api/v1/expenses/{id}/void", a.requireAuth(a.handleExpenseVoid))

	mux.HandleFunc("GET /api/v1/reports/dashboard", a.requireAuth(a.handleReportDashboard))
	mux.HandleFunc("GET /api/v1/reports/overview", a.requireAuth(a.handleReportOverview))
	mux.HandleFunc("GET /api/v1/reports/export", a.requireAuth(a.handleReportExport))

	mux.HandleFunc("GET /api/v1/commissions", a.requireAuth(a.handleCommissions))
	mux.HandleFunc("GET /api/v1/commissions/export", a.requireAuth(a.handleCommissionsExport))

	// Les handlers /platform/* vérifient eux-mêmes le rôle superadmin
	// (requireSuperadmin dans handlers_misc.go) : parité avec server.ts qui
	// n'applique qu'un requireAuth sur ces routes.
	mux.HandleFunc("GET /api/v1/platform/overview", a.requireAuth(a.handlePlatformOverview))
	mux.HandleFunc("GET /api/v1/platform/organizations", a.requireAuth(a.handlePlatformOrgsList))
	mux.HandleFunc("GET /api/v1/platform/organizations/{id}", a.requireAuth(a.handlePlatformOrgDetail))
	mux.HandleFunc("PATCH /api/v1/platform/organizations/{id}", a.requireAuth(a.handlePlatformOrgUpdate))
	mux.HandleFunc("POST /api/v1/platform/organizations/{id}/subscription", a.requireAuth(a.handlePlatformOrgSubscription))
	mux.HandleFunc("GET /api/v1/platform/plans", a.requireAuth(a.handlePlatformPlansList))
	mux.HandleFunc("PATCH /api/v1/platform/plans/{id}", a.requireAuth(a.handlePlatformPlanUpdate))
	mux.HandleFunc("GET /api/v1/platform/audit", a.requireAuth(a.handlePlatformAudit))

	// Espace laveur
	mux.HandleFunc("GET /api/v1/washer/tasks", a.requireAuth(a.handleWasherTasks))
	mux.HandleFunc("POST /api/v1/washer/tasks/{id}/status", a.requireAuth(a.handleWasherTaskStatus))
	mux.HandleFunc("POST /api/v1/washer/tasks/{id}/incident", a.requireAuth(a.handleWasherTaskIncident))
	mux.HandleFunc("GET /api/v1/washer/me/performance", a.requireAuth(a.handleWasherPerformance))

	// Front statique (build Vite) + repli SPA
	if a.Cfg.StaticDir != "" {
		static := a.Cfg.StaticDir
		mux.HandleFunc("GET /assets/", func(w http.ResponseWriter, r *http.Request) {
			http.StripPrefix("/assets/", http.FileServer(http.Dir(filepath.Join(static, "assets")))).ServeHTTP(w, r)
		})
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/health") {
				writeError(w, http.StatusNotFound, "not_found", "Route inconnue.", nil)
				return
			}
			http.ServeFile(w, r, filepath.Join(static, "index.html"))
		})
	}

	return withCORS(a.Cfg)(mux)
}
