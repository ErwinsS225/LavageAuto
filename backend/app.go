package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

// App porte le pool de connexions et la configuration, partagés par tous
// les handlers.
type App struct {
	Pool *pgxpool.Pool
	Cfg  Config
}

// rolePermissions reprend la matrice de server.ts : chaque rôle a sa liste
// de permissions, contrôlée côté serveur et côté frontend.
var rolePermissions = map[string][]string{
	"superadmin": {
		"dashboard:view", "station:view", "wash:view", "wash:create", "wash:update", "wash:cancel", "wash:assign",
		"payment:view", "payment:create", "payment:void", "payment:create_deferred",
		"service:view", "service:manage",
		"customer:view", "customer:create", "customer:manage", "employee:view", "employee:manage",
		"cash:view", "cash:open", "cash:close", "cash:reopen", "expense:view", "expense:create",
		"expense:void", "report:view", "report:export", "commission:view", "commission:manage", "settings:view",
		"settings:manage", "organization:manage", "subscription:view", "subscription:manage",
		"audit:view", "platform:manage", "washer:view",
	},
	"owner": {
		"dashboard:view", "station:view", "wash:view", "wash:create", "wash:update", "wash:cancel", "wash:assign",
		"payment:view", "payment:create", "payment:void", "payment:create_deferred",
		"service:view", "service:manage",
		"customer:view", "customer:create", "customer:manage", "employee:view", "employee:manage",
		"cash:view", "cash:open", "cash:close", "cash:reopen", "expense:view", "expense:create",
		"expense:void", "report:view", "report:export", "commission:view", "commission:manage", "settings:view",
		"settings:manage", "organization:manage", "subscription:view", "subscription:manage",
		"audit:view", "washer:view",
	},
	"manager": {
		"dashboard:view", "station:view", "wash:view", "wash:create", "wash:update", "wash:cancel", "wash:assign",
		"payment:view", "payment:create", "payment:void", "payment:create_deferred",
		"service:view", "service:manage",
		"customer:view", "customer:create", "customer:manage", "employee:view", "employee:manage",
		"cash:view", "cash:open", "cash:close", "cash:reopen", "expense:view", "expense:create",
		"expense:void", "expense:approve", "report:view", "report:export", "commission:view", "commission:manage",
		"settings:view", "settings:manage", "subscription:view", "audit:view", "washer:view", "washer:update_status", "washer:report_incident",
	},
	"cashier": {
		"dashboard:view", "station:view", "wash:view", "wash:create", "wash:update",
		"payment:view", "payment:create", "service:view", "customer:view", "customer:create",
		"cash:view", "cash:open", "cash:close", "expense:view", "expense:create", "washer:view",
	},
	"washer": {
		"washer:view", "washer:update_status", "washer:report_incident", "washer:personal_stats",
	},
}

// isStaffRestringed signale un rôle qui ne peut pas gérer le catalogue/employés.
func isRestrictedRole(role string) bool {
	return role == "cashier" || role == "washer"
}
