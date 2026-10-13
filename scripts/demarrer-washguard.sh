#!/usr/bin/env bash
#
# Démarre l'API WashGuard en lisant la configuration du fichier .env.
# Appelé automatiquement par launchd (démarrage de session) ; peut aussi être
# lancé à la main pour tester.
#
# L'API sert à la fois les données et l'interface compilée : les téléphones de la
# station n'ont qu'une seule adresse à ouvrir.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [ ! -f "$ROOT/.env" ]; then
  echo "Configuration absente : $ROOT/.env introuvable. Lancez : bash scripts/installer-station.sh"
  exit 1
fi

if [ ! -x "$ROOT/washguard-api" ]; then
  echo "Exécutable absent : $ROOT/washguard-api introuvable. Lancez : bash scripts/installer-station.sh"
  exit 1
fi

# Variables du .env : DATABASE_URL, STATIC_DIR, PLATFORM_ADMIN_*...
set -a
# shellcheck disable=SC1091
. "$ROOT/.env"
set +a

# STATIC_DIR est relatif au dossier backend/ : c'est depuis ce dossier que l'API
# doit être lancée (même convention que `make api` et l'unité systemd documentée).
cd "$ROOT/backend" || exit 1

echo "$(date '+%Y-%m-%d %H:%M:%S')  démarrage de WashGuard ($ROOT)"
exec "$ROOT/washguard-api"
