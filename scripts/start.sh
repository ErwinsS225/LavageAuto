#!/usr/bin/env bash
#
# Démarre WashGuard en mode "tout-en-un" (production locale) :
#   1. compile le front (Vite) dans dist/
#   2. compile le backend Go en binaire ./washguard-api
#   3. lance le binaire, qui sert À LA FOIS l'API et l'interface (dist/).
#
# Le binaire est lancé depuis backend/ (même convention que demarrer-washguard.sh)
# et reçoit STATIC_DIR=../dist pour exposer l'interface compilée.
#
# Prérequis : Go installé, .env renseigné (base de données + secrets).

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "[start] Construction du front (npm run build)…"
npm run build

if [ ! -f "$ROOT/.env" ]; then
  echo "[start] ERREUR : $ROOT/.env est absent. Renseignez la configuration avant de démarrer." >&2
  exit 1
fi
if [ ! -f "$ROOT/dist/index.html" ]; then
  echo "[start] ERREUR : dist/index.html absent après le build." >&2
  exit 1
fi

echo "[start] Construction du backend Go (washguard-api)…"
( cd "$ROOT/backend" && go build -o ../washguard-api . )

# Le binaire Go lit lui-même ../.env (backend/config.go : loadDotEnv cherche
# .env puis ../.env). On NE source PAS le .env ici : l'URL de base contient un
# « & » que le shell interpréterait à tort. On se contente de basculer dans
# backend/ puis d'exporter le dossier de l'interface compilée.
cd "$ROOT/backend"

# STATIC_DIR est relatif au dossier backend/ depuis lequel on lance le binaire.
# (Chemin vers dist/ à la racine.) forçable via l'environnement.
export STATIC_DIR="${STATIC_DIR:-../dist}"
export API_PORT="${API_PORT:-3000}"

echo "[start] WashGuard écoute sur http://localhost:${API_PORT}"
exec ./../washguard-api
