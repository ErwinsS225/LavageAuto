#!/usr/bin/env bash
#
# Recette de première utilisation : rejoue le « jour 1 » d'une station neuve.
#
# Deux parcours sont rejoués, sur une base VIERGE et un vrai navigateur au format
# téléphone (390 × 844).
#
# A. Le premier jour d'une station neuve :
#
#   1. création de l'espace depuis /inscription ;
#   2. présence du catalogue d'Abidjan, déjà tarifé ;
#   3. enregistrement d'un premier lavage ;
#   4. refus d'un encaissement en espèces sans caisse ouverte, avec un message
#      qui indique quoi faire ;
#   5. encaissement Mobile Money accepté (il ne touche pas au tiroir) ;
#   6. ouverture de la caisse avec un fond de caisse ;
#   7. encaissement en espèces désormais accepté ;
#   8. clôture avec comptage, écart de 500 FCFA annoncé puis enregistré ;
#   9. cohérence des chiffres affichés sur le tableau de bord.
#
# B. L'embauche d'un caissier — sans quoi la station ne peut pas démarrer :
#   1. une nouvelle station s'inscrit et reçoit la formule Essai ;
#   2. le propriétaire crée le compte du caissier ;
#   3. le mot de passe provisoire est affiché une seule fois ;
#   4. le caissier se connecte, est détourné vers « Mon compte » et ne peut pas
#      travailler avant d'avoir choisi son mot de passe ;
#   5. il choisit son mot de passe, la bannière disparaît sans rechargement ;
#   6. il enregistre son premier lavage, visible aussi chez le propriétaire ;
#   7. la limite de la formule Essai est appliquée par le serveur et le refus
#      indique la sortie (« écran Abonnement »).
#
# La base est créée puis supprimée par le script : la base de développement de
# la station n'est ni lue ni modifiée. Ne lancez pas ce script pendant que la
# station travaille : il consomme un port et une base.
#
# Usage : make recette   (ou bash scripts/recette-jour1.sh)
# Variables :
#   DATABASE_URL       base de référence (obligatoire) — sert au serveur PG et
#                      au nom d'utilisateur, la recette utilise une base jetable ;
#   RECETTE_PORT       port de l'instance de recette (défaut 8098)
#   RECETTE_DATABASE   nom de la base jetable (défaut washguard_recette)
#   CHROMIUM_PATH      navigateur à piloter. Sans cette variable, le Chromium de
#                      @sparticuz/chromium est utilisé (paquet npm, Linux).
#   RECETTE_CAPTURES   dossier des captures d'écran (défaut recette/ à la racine)

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1

PORT="${RECETTE_PORT:-8098}"
DB_NAME="${RECETTE_DATABASE:-washguard_recette}"
CAPTURES="${RECETTE_CAPTURES:-$ROOT/recette}"
LOG="$(mktemp -t washguard-recette.XXXXXX)"
API_PID=""

source_env() { [ -f "$ROOT/.env" ] && set -a && . "$ROOT/.env" && set +a; return 0; }
source_env

if [ -z "${DATABASE_URL:-}" ]; then
  echo "DATABASE_URL est introuvable : copiez .env.example vers .env (cp .env.example .env)." >&2
  exit 2
fi

# psql : fourni par PostgreSQL ou par Docker. On s'en passe sans regret si absent.
PSQL=""
if command -v psql >/dev/null 2>&1; then
  PSQL="psql"
elif command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  PSQL="docker compose exec -T db psql"
fi
if [ -z "$PSQL" ]; then
  echo "psql est introuvable : installez le client PostgreSQL (brew install libpq) ou démarrez Docker." >&2
  exit 2
fi

cleanup() {
  # Désactive l'arrêt sur erreur : arrêter l'API signale un succès, pas un échec.
  # Sans cela, le `wait` de l'API qu'on vient de tuer renvoie 143 (SIGTERM) et le
  # script se terminerait en erreur alors que la recette a réussi.
  set +e
  local code=$?
  if [ -n "$API_PID" ] && kill -0 "$API_PID" 2>/dev/null; then
    kill "$API_PID" 2>/dev/null
    wait "$API_PID" 2>/dev/null || true
  fi
  if [ -n "$PSQL" ]; then
    $PSQL "$DATABASE_URL" -q -c "DROP DATABASE IF EXISTS $DB_NAME WITH (FORCE);" >/dev/null 2>&1 \
      || $PSQL postgres -q -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$DB_NAME';" >/dev/null 2>&1
  fi
  [ -n "$LOG" ] && rm -f "$LOG"
  return $code
}
trap cleanup EXIT

echo "Recette « jour 1 » d'une station neuve"
echo

# 1. Base jetable, repartie de zéro.
ADMIN_URL="${DATABASE_URL%/*}/postgres"
$PSQL "$ADMIN_URL" -q -c "DROP DATABASE IF EXISTS $DB_NAME WITH (FORCE);" >/dev/null 2>&1 \
  || $PSQL "$ADMIN_URL" -q -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$DB_NAME';" -c "DROP DATABASE IF EXISTS $DB_NAME;" >/dev/null 2>&1
if ! $PSQL "$ADMIN_URL" -q -c "CREATE DATABASE $DB_NAME;" >/dev/null 2>&1; then
  echo "Impossible de créer la base jetable $DB_NAME." >&2
  echo "Vérifiez que PostgreSQL est démarré (make db) et que DATABASE_URL est correct." >&2
  exit 1
fi
echo "  Base jetable $DB_NAME créée (votre base de développement n'est pas touchée)."

# 2. Frontend compilé (l'API sert l'application, comme à la station).
if [ ! -f dist/index.html ]; then
  echo "  Frontend non compilé : bash scripts/start.sh (ou npm run build) en premier." >&2
  exit 1
fi

# 3. API dédiée sur la base vierge.
REC_URL="${DATABASE_URL%/*}/$DB_NAME"
(
  cd backend || exit 1
  DATABASE_URL="$REC_URL" API_PORT="$PORT" APP_ENV=development \
  STATIC_DIR="${STATIC_DIR:-../dist}" SESSION_COOKIE_SECURE=false FRAME_ANCESTORS='*' \
  PLATFORM_ADMIN_EMAIL="${PLATFORM_ADMIN_EMAIL:-admin@washguard.app}" \
  PLATFORM_ADMIN_PASSWORD="${PLATFORM_ADMIN_PASSWORD:-recette-admin-2026}" \
  exec go run .
) >"$LOG" 2>&1 &
API_PID=$!

echo "  API de recette sur le port $PORT (base vierge)…"
for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$PORT/health" 2>/dev/null; then break; fi
  if ! kill -0 "$API_PID" 2>/dev/null; then
    echo "L'API de recette s'est arrêtée :" >&2
    tail -15 "$LOG" >&2
    exit 1
  fi
  sleep 1
done
if ! curl -fsS -o /dev/null "http://127.0.0.1:$PORT/health" 2>/dev/null; then
  echo "L'API de recette n'a pas répondu dans les délais :" >&2
  tail -15 "$LOG" >&2
  exit 1
fi
echo "  Migrations appliquées, base vierge confirmée."

# 4. Navigateur : Chrome de la machine, ou Chromium fourni par npm.
# Sur macOS, le plus simple est de pointer votre navigateur :
#   CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" bash scripts/recette-jour1.sh
# Sans cette variable, un Chromium téléchargé par npm est utilisé.
NAVIGATEUR="${CHROMIUM_PATH:-paquet npm}"
if [ -n "${CHROMIUM_PATH:-}" ]; then
  [ -x "$CHROMIUM_PATH" ] || { echo "CHROMIUM_PATH est introuvable : $CHROMIUM_PATH" >&2; exit 2; }
fi
# puppeteer-core s'installe à la racine (avec les autres dépendances npm) ; les
# scripts de recette restent dans frontend/tools/rendu.
if ! [ -d node_modules/puppeteer-core ]; then
  echo "  Installation du pilote de navigateur (puppeteer-core, sans l'ajouter aux dépendances)…"
  PAQUETS="puppeteer-core"
  [ -n "${CHROMIUM_PATH:-}" ] || PAQUETS="puppeteer-core @sparticuz/chromium"
  # shellcheck disable=SC2086
  (npm install --no-save $PAQUETS >/dev/null 2>&1) || true
fi
if ! [ -d node_modules/puppeteer-core ]; then
  echo "puppeteer-core est introuvable. Installez-le :" >&2
  echo "  npm install --no-save puppeteer-core" >&2
  exit 2
fi
echo "  Navigateur utilisé : $NAVIGATEUR"

# 5. Les parcours eux-mêmes : d'abord le premier jour, puis l'embauche.
cd frontend || exit 1
CODE=0
for scenario in jour-1-station embauche-caissier; do
  echo
  echo "--- $scenario ---"
  set +e
  BASE_URL="http://127.0.0.1:$PORT" OUT_DIR="$CAPTURES/$scenario" node "tools/rendu/$scenario.mjs"
  SCENARIO_CODE=$?
  set -e
  [ "$SCENARIO_CODE" -ne 0 ] && CODE=$SCENARIO_CODE
done
cd "$ROOT" || exit 1

if [ "$CODE" -ne 0 ]; then
  echo
  echo "La recette a échoué (code $CODE). Journal de l'API :" >&2
  tail -20 "$LOG" >&2
fi
exit "$CODE"
