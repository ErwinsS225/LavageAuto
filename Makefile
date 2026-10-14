# WashGuard — commandes du projet.
#
#   make help          liste tout ce qui est disponible
#   make doctor        vérifie ce qui manque sur la machine
#   make check         tout vérifier (compilation, formatage, tests, sondes, écrans)
#
# Le README cite ces commandes : elles doivent exister et faire ce qu'elles
# annoncent. Chaque cible affiche ce qu'elle fait, pour qu'une erreur puisse
# être comprise sans lire le Makefile.

SHELL := /bin/bash
.DEFAULT_GOAL := help

ROOT    := $(shell pwd)
BACKEND := $(ROOT)/backend
DIST    := $(ROOT)/dist
# « make api » sert le développement : le serveur Vite (port 3000) relaie /api
# vers ce port-là (vite.config.ts). L'installation de station, elle, garde le
# port du .env (3000 dans le gabarit) via scripts/start.sh.
API_PORT ?= 8080
BIN     ?= $(ROOT)/washguard-api

# Base utilisée par « make test-e2e » : celle de « make db » par défaut. Les
# tests créent leur propre schéma et le suppriment : aucune donnée n'est lue,
# modifiée ni conservée dans la base visée.
TEST_DSN ?= postgresql://washguard:washguard@127.0.0.1:5432/washguard?sslmode=disable

# L'API lit le .env de la racine toute seule (backend/config.go) ; on passe
# seulement les variables qui doivent primer sur le fichier.
API_ENV ?=

.PHONY: help doctor install build web api migrate seed test test-e2e check verifier-env \
        verifier-acces smoke rendu recette station verifier-station adresse \
        db db-stop db-reset backup prod-check clean

# ---------------------------------------------------------------------------
# Aide
# ---------------------------------------------------------------------------
help: ## Affiche cette aide
	@echo "WashGuard — commandes disponibles"
	@echo
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[1mmake %-16s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "  Variables : API_PORT (défaut 8080 en développement), BIN, TEST_DSN (base des tests de bout en bout)"
	@echo "              DATABASE_URL et NEON_DATABASE_URL sont lus dans .env"

doctor: ## Vérifie les outils présents sur la machine
	@echo "Outils"
	@if command -v go >/dev/null 2>&1; then printf '  \033[32m✓\033[0m %-8s %s\n' "go" "$$(go version | cut -d' ' -f3)"; \
		else printf '  \033[31m✗\033[0m %-8s absent\n' "go"; fi
	@for outil in node npm; do \
		if command -v $$outil >/dev/null 2>&1; then \
			printf '  \033[32m✓\033[0m %-8s %s\n' "$$outil" "$$($$outil --version 2>&1 | head -1)"; \
		else \
			printf '  \033[31m✗\033[0m %-8s absent\n' "$$outil"; \
		fi; \
	done
	@printf '  '; if command -v psql >/dev/null 2>&1; then printf '\033[32m✓\033[0m psql     présent\n'; \
		elif command -v docker >/dev/null 2>&1; then printf '\033[33m·\033[0m psql     absent (Docker disponible : make db)\n'; \
		else printf '\033[31m✗\033[0m psql     absent et Docker indisponible\n'; fi
	@echo
	@echo "Configuration"
	@if [ -f "$(ROOT)/.env" ]; then \
		printf '  \033[32m✓\033[0m .env présent\n'; \
		bash $(ROOT)/scripts/verifications/verifier-env.sh dev 2>/dev/null | tail -2 | sed 's/^/    /'; \
	else \
		printf '  \033[31m✗\033[0m .env absent — copiez .env.example en .env\n'; \
	fi

install: ## Installe les dépendances du front (npm ci)
	@echo "Dépendances du front"
	@npm ci --no-audit --no-fund && echo "  installées"

build: ## Compile l'API (washguard-api) et le front (dist/)
	@echo "API Go"
	@cd $(BACKEND) && go build -o $(BIN) . && echo "  $(BIN)"
	@echo "Front"
	@npm run build >/dev/null && echo "  $(DIST)"
	@# Le front est plus simple à servir depuis un dossier fixe : on le recopie
	@# dans backend/dist pour que STATIC_DIR=dist fonctionne aussi.
	@echo "Terminé."

web: ## Lance le serveur de développement du front (Vite)
	@echo "Front en développement — l'API doit tourner (make api) et être accessible sur /api/v1"
	@npm run dev:web

api: ## Démarre l'API (lit .env, sert le front compilé s'il existe)
	@if [ ! -f "$(ROOT)/.env" ]; then echo "ATTENTION : .env absent, l'API ne saura pas où est la base." >&2; fi
	@if [ -d "$(DIST)" ]; then export STATIC_DIR="$(DIST)"; fi; \
	cd $(BACKEND) && $(API_ENV) API_PORT=$(API_PORT) go run .

migrate: ## Applique les migrations sans démarrer le serveur
	@cd $(BACKEND) && go run . -migrate-only

seed: ## Crée les données de démonstration (jamais sur une base de production)
	@echo "Attention : cette cible crée une entreprise, une station et quatre comptes de démonstration."
	@read -p "Continuer ? [o/N] " reponse; \
	case "$$reponse" in \
		o|O|oui|OUI) cd $(BACKEND) && SEED_DEMO=true go run . -migrate-only && \
			echo "  migrations appliquées ; démarrez l'API avec SEED_DEMO=true pour créer les données" ;; \
		*) echo "  abandonné" ;; \
	esac

test: ## Tests Go rapides (unitaires) et vérification des types du front
	@echo "Tests Go"
	@cd $(BACKEND) && go test ./...
	@echo "  (les tests de bout en bout sont ignorés sans base : « make test-e2e »)"
	@echo "Types du front"
	@npx tsc -b && echo "  aucun problème de type"

# ---------------------------------------------------------------------------
# Vérifications
# ---------------------------------------------------------------------------
verifier-env: ## Contrôle le fichier .env (ne montre aucun secret)
	@bash $(ROOT)/scripts/verifications/verifier-env.sh dev

test-e2e: ## Tests de bout en bout sur une base jetable (TEST_DSN, sans danger sur la base visée)
	@echo "Tests de bout en bout (base : $$(printf '%s' '$(TEST_DSN)' | sed -E 's#//[^:]+:[^@]+@#//***:***@#'))"
	@cd $(BACKEND) && WASHGUARD_TEST_DATABASE_URL='$(TEST_DSN)' go test -count=1 -v ./...

verifier-acces: ## Sondes d'acceptation contre l'API qui tourne (DATABASE_URL pour la suspension)
	@echo "Sondes d'acceptation (API supposée démarrée sur le port $(API_PORT))"
	@python3 $(ROOT)/scripts/verifications/sonde-api.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-onboarding.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-tarifs.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-invitations.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-vehicules.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-metier.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-fiche-payee.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-lot7.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-lot8.py http://127.0.0.1:$(API_PORT)
	@python3 $(ROOT)/scripts/verifications/sonde-suspension.py http://127.0.0.1:$(API_PORT)

smoke: ## Vérifie les écrans et les routes (sans navigateur)
	@echo "Cohérence des routes et des écrans"
	@node $(ROOT)/scripts/verifications/coherence-routes.mjs
	@echo "Rendu des écrans"
	@node $(ROOT)/scripts/verifications/rendu-pages.mjs

check: ## Tout vérifier : formatage, compilation, tests, routes, écrans, configuration
	@echo "1/5 — formatage et analyse statique (Go)"
	@cd $(BACKEND) && test -z "$$(gofmt -l .)" || { echo "  fichiers mal formatés :"; gofmt -l .; exit 1; }
	@cd $(BACKEND) && go vet ./... && echo "  ok"
	@echo "2/5 — compilation"
	@cd $(BACKEND) && go build -o /dev/null . && echo "  API ok"
	@npm run build >/dev/null && echo "  front ok"
	@echo "3/5 — tests"
	@cd $(BACKEND) && go test ./...
	@echo "4/5 — routes, écrans et classes d'interface"
	@node $(ROOT)/scripts/verifications/coherence-routes.mjs | tail -1
	@node $(ROOT)/scripts/verifications/rendu-pages.mjs | tail -1
	@node $(ROOT)/scripts/verifications/classes-ui.mjs | tail -1
	@echo "5/5 — configuration"
	@bash $(ROOT)/scripts/verifications/verifier-env.sh dev 2>/dev/null | tail -2

prod-check: ## Vérifie l'installation avant mise en service (base dédiée)
	@bash $(ROOT)/scripts/verifier-production.sh

# ---------------------------------------------------------------------------
# Base de données
# ---------------------------------------------------------------------------
db: ## Démarre PostgreSQL en conteneur (docker compose)
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Docker n'est pas installé. Installez PostgreSQL, ou utilisez une base Neon (NEON_DATABASE_URL)."; \
		exit 1; \
	fi
	@docker compose up -d db && echo "PostgreSQL démarré"

db-stop: ## Arrête le conteneur PostgreSQL
	@docker compose stop db

db-reset: ## Recrée une base vide (destructif, demande confirmation)
	@read -p "Effacer toutes les données de la base locale ? [o/N] " reponse; \
	case "$$reponse" in \
		o|O|oui|OUI) docker compose down -v db >/dev/null && docker compose up -d db && echo "  base recréée" ;; \
		*) echo "  abandonné" ;; \
	esac

# ---------------------------------------------------------------------------
# Station locale (macOS, réseau du bureau)
# ---------------------------------------------------------------------------
station: ## Installation guidée d'une station sur ce Mac
	@bash $(ROOT)/scripts/installer-station.sh

verifier-station: ## Vérifie ce qui manque à l'installation d'une station (ne modifie rien)
	@bash $(ROOT)/scripts/installer-station.sh --verifier

adresse: ## Affiche l'adresse à ouvrir depuis les téléphones du réseau local
	@bash $(ROOT)/scripts/adresse-locale.sh

backup: ## Sauvegarde la base (pg_dump) dans backups/
	@bash $(ROOT)/scripts/sauvegarde-quotidienne.sh

# ---------------------------------------------------------------------------
# Parcours dans un vrai navigateur (nécessite Chromium + puppeteer-core)
# ---------------------------------------------------------------------------
# ATTENTION : ces deux cibles rejouent des scénarios écrits AVANT la mise en
# route (lots 5 à 8) : ils attendent un catalogue pré-tarifé et enregistrent un
# lavage juste après l'inscription, ce que le produit ne permet plus (aucun prix
# inventé ; 409 onboarding_incomplete tant que la station n'est pas activée).
# Ils échouent donc, et le disent en s'arrêtant sur le premier contrôle rouge.
# La campagne qui fait foi : `make verifier-acces` (voir docs/onboarding.md §10).
rendu: ## Parcours guidés dans un navigateur réel — scénarios antérieurs au parcours de mise en route
	@$(MAKE) --no-print-directory verifier-navigateur
	@echo "⚠️  Scénario antérieur aux lots 5 à 8 (catalogue pré-tarifé) : il échouera. Voir docs/onboarding.md."
	@node $(ROOT)/frontend/tools/rendu/jour-1-station.mjs

recette: ## Recette du jour 1 sur une base neuve — scénarios antérieurs au parcours de mise en route
	@echo "⚠️  Cette recette précède le parcours de mise en route : catalogue pré-tarifé, lavage"
	@echo "    immédiat. Elle échoue depuis les lots 5 à 8 ; la campagne à jour est docs/onboarding.md §10."
	@bash $(ROOT)/scripts/recette-jour1.sh

verifier-navigateur:
	@if [ ! -d "$(ROOT)/node_modules/puppeteer-core" ]; then \
		echo "puppeteer-core n'est pas installé : npm install puppeteer-core @sparticuz/chromium"; exit 1; \
	fi

clean: ## Supprime les artefacts de build
	@rm -f $(ROOT)/washguard-api $(ROOT)/backend/washguard-api
	@rm -rf $(DIST) $(ROOT)/node_modules/.vite
	@echo "artefacts supprimés (node_modules conservé : make install le reconstruirait lentement)"
