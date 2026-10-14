#!/usr/bin/env bash
#
# Vérifie que l'API se comporte correctement en mode production, sans rien
# déployer. À lancer avant une mise en ligne : le script refuse de mentir, il
# exécute réellement le binaire et observe les réponses.
#
# Contrôles :
#   1. la configuration incomplète est refusée (mot de passe plateforme absent,
#      trop court, cookie non sécurisé) — un démarrage « silencieux » en
#      production serait une faille ;
#   2. la configuration complète démarre et sert l'application compilée :
#      /health, page d'accueil, lien profond (route React), fichier d'assets ;
#   3. le cookie de session porte Secure et SameSite.
#
# Le contrôle s'exécute sur une base jetable, créée puis supprimée par le script :
# votre base de développement n'est ni lue ni modifiée.
#
# Usage : bash scripts/verifier-production.sh   (lit le fichier .env)
# Variables :
#   DATABASE_URL            base de référence (obligatoire) — sert seulement à
#                           savoir comment se connecter au serveur PostgreSQL ;
#   PROD_CHECK_PORT         port de l'instance de test (défaut 8099)
#   PROD_CHECK_DATABASE     nom de la base jetable (défaut washguard_prodcheck)
#   STATIC_DIR              dossier du frontend compilé (défaut ../dist,
#                           relatif à backend/, comme l'application au démarrage)

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1

# Le script annonçait lire le fichier .env : il le lit maintenant vraiment, et
# accepte NEON_DATABASE_URL comme le serveur (l'ancien contrôle exigeait
# DATABASE_URL et refusait une configuration pourtant valide).
# shellcheck source=charger-env.sh
. "$ROOT/scripts/charger-env.sh"
charger_env "$ROOT/.env"
DATABASE_URL="${DATABASE_URL:-${NEON_DATABASE_URL:-}}"
export DATABASE_URL

PORT="${PROD_CHECK_PORT:-8099}"
DB_NAME="${PROD_CHECK_DATABASE:-washguard_prodcheck}"
STATIC_DIR="${STATIC_DIR:-../dist}"
BINARY="$(mktemp -t washguard-prod-check.XXXXXX)"
PROD_PID=""
BASE_CREEE=0
LOG=""

# L'API s'exécute toujours depuis backend/ (make api, unité systemd avec
# WorkingDirectory=backend) : STATIC_DIR est donc relatif à ce dossier.
case "$STATIC_DIR" in
  /*) STATIC_ABS="$STATIC_DIR" ;;
  *) STATIC_ABS="$ROOT/backend/$STATIC_DIR" ;;
esac

# --- Accès à PostgreSQL (binôme psql, ou conteneur Docker) -------------------

base_url() { echo "${DATABASE_URL%/*}"; }

# Commande SQL sur la base « postgres » (création / suppression de la base jetable).
sql_admin() {
  if command -v psql >/dev/null 2>&1; then
    psql "$(base_url)/postgres" -q -v ON_ERROR_STOP=1 -c "$1" >/dev/null 2>&1
  else
    docker compose exec -T db psql -U washguard -d postgres -q -v ON_ERROR_STOP=1 -c "$1" >/dev/null 2>&1
  fi
}

supprimer_base() {
  sql_admin "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$DB_NAME'"
  sql_admin "DROP DATABASE IF EXISTS $DB_NAME"
}

# Nettoyage garanti, y compris en cas d'arrêt anticipé ou d'interruption.
nettoyer() {
  [ -n "$PROD_PID" ] && kill "$PROD_PID" 2>/dev/null
  rm -f "$BINARY"
  if [ "$BASE_CREEE" = "1" ]; then
    supprimer_base && echo "  (base jetable $DB_NAME supprimée)"
  fi
  # Le journal n'est conservé qu'en cas d'échec : il sert alors au diagnostic.
  [ -n "$LOG" ] && [ "${ECHECS:-0}" -eq 0 ] && rm -f "$LOG"
  return 0
}
trap nettoyer EXIT INT TERM

VERT=$'\033[32m'; ROUGE=$'\033[31m'; GRIS=$'\033[90m'; FIN=$'\033[0m'
echecs=0
ECHECS=0

ok()    { echo "  ${VERT}✅${FIN} $1"; }
echec() { echo "  ${ROUGE}❌${FIN} $1"; echecs=$((echecs + 1)); ECHECS=$echecs; }
detail(){ echo "     ${GRIS}$1${FIN}"; }

if [ -z "${DATABASE_URL:-}" ]; then
  echo "${ROUGE}DATABASE_URL (ou NEON_DATABASE_URL) n'est pas défini.${FIN} Ce script lit le fichier .env à la racine." >&2
  exit 2
fi

if [ ! -f "$STATIC_ABS/index.html" ]; then
  echo "${ROUGE}L'application n'est pas compilée${FIN} : $STATIC_ABS/index.html est absent. Lancez : npm run build"
  exit 2
fi

# Déjà utilisé ? On refuse d'écraser un service en cours.
#
# La détection ne dépend pas de lsof (absent de certains systèmes) : une
# connexion TCP directe suffit. Sans ce contrôle, le script interrogerait le
# service d'un autre programme et validerait le mauvais processus.
port_occupe() {
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null && { exec 3<&- 2>/dev/null; return 0; }
  return 1
}

if port_occupe "$PORT"; then
  echo "${ROUGE}Le port $PORT répond déjà.${FIN} Choisissez-en un autre : PROD_CHECK_PORT=8098 bash scripts/verifier-production.sh"
  exit 2
fi

# --- Base jetable : le contrôle ne touche pas aux données réelles -----------

if ! sql_admin "CREATE DATABASE $DB_NAME"; then
  echo "${ROUGE}Impossible de créer la base jetable $DB_NAME.${FIN}"
  echo "Vérifiez que PostgreSQL est démarré (make db) et que DATABASE_URL est correct."
  exit 2
fi
BASE_CREEE=1
DATABASE_URL="$(base_url)/$DB_NAME"
echo "Base jetable $DB_NAME créée (les données de développement ne sont pas touchées)."

echo "Compilation de l'API…"
if ! (cd backend && go build -o "$BINARY" .); then
  echo "${ROUGE}La compilation a échoué.${FIN}"
  exit 1
fi

# --- 1. La configuration incomplète doit être REFUSÉE -----------------------

echo
echo "1. Refus de démarrer avec une configuration incomplète"

refus_attendu() { # $1 = libelle, $2 = extrait attendu, reste = variables d'environnement
  local libelle="$1" extrait="$2"; shift 2
  local sortie
  # -u : on retire d'abord les variables fournies par .env (chargé par le
  # Makefile), sinon le cas « valeur absente » testerait en réalité la valeur
  # du fichier de configuration et passerait pour de mauvaises raisons.
  sortie="$(cd backend && env -u PLATFORM_ADMIN_PASSWORD -u SESSION_COOKIE_SECURE \
    -u FRAME_ANCESTORS -u PLATFORM_ADMIN_EMAIL \
    APP_ENV=production API_PORT="$PORT" DATABASE_URL="$DATABASE_URL" STATIC_DIR="$STATIC_DIR" "$@" "$BINARY" 2>&1)"
  local code=$?
  if [ $code -eq 0 ]; then
    echec "$libelle : le démarrage a été accepté (code 0)"
  elif echo "$sortie" | grep -qi "$extrait"; then
    ok "$libelle"
    detail "$(echo "$sortie" | tail -1)"
  else
    echec "$libelle : refusé, mais sans le message attendu (« $extrait »)"
    detail "$(echo "$sortie" | tail -2 | tr '\n' ' ')"
  fi
}

refus_attendu "mot de passe plateforme absent"    "PLATFORM_ADMIN_PASSWORD est obligatoire"  SESSION_COOKIE_SECURE=true
refus_attendu "mot de passe plateforme trop court" "au moins 10 caractères"                  SESSION_COOKIE_SECURE=true PLATFORM_ADMIN_PASSWORD=court
refus_attendu "cookie de session non sécurisé"     "SESSION_COOKIE_SECURE=true est obligatoire" PLATFORM_ADMIN_PASSWORD=motdepasse-plateforme

# --- 2 et 3. La configuration complète démarre et sert l'application ---------
echo
echo "2. Démarrage réel et service de l'application compilée"

LOG="$(mktemp -t washguard-prod-log.XXXXXX)"
ADMIN_EMAIL="admin@verification.washguard.ci"
ADMIN_MOTDEPASSE="motdepasse-plateforme-verification"

# Le binaire est lancé depuis backend/, exactement comme en production (unité
# systemd : WorkingDirectory=.../backend) : STATIC_DIR y est relatif.
( cd backend && exec env APP_ENV=production \
    API_PORT="$PORT" \
    DATABASE_URL="$DATABASE_URL" \
    STATIC_DIR="$STATIC_DIR" \
    SESSION_COOKIE_SECURE=true \
    PLATFORM_ADMIN_EMAIL="$ADMIN_EMAIL" \
    PLATFORM_ADMIN_PASSWORD="$ADMIN_MOTDEPASSE" \
    "$BINARY" ) >"$LOG" 2>&1 &
PID=$!
PROD_PID=$PID

base="http://127.0.0.1:$PORT"
pret=0
for _ in $(seq 1 40); do
  if curl -fsS -o /dev/null "$base/health" 2>/dev/null; then pret=1; break; fi
  sleep 0.25
done

if ! kill -0 "$PID" 2>/dev/null; then
  echec "le processus s'est arrêté aussitôt (port déjà pris ? configuration refusée ?)"
  echo "     Journal : $LOG"
  exit 1
fi

if [ $pret -eq 1 ]; then
  ok "l'API répond sur /health (processus vérifié vivant)"
else
  echec "l'API n'a pas répondu dans les 10 secondes"
  echo "     Journal : $LOG"
  exit 1
fi

code() { curl -s -o /dev/null -w '%{http_code}' "$1"; }

accueil="$(code "$base/")"
[ "$accueil" = "200" ] && ok "page d'accueil servie (200)" || echec "page d'accueil : $accueil (200 attendu)"

profond="$(code "$base/tableau-de-bord")"
[ "$profond" = "200" ] && ok "lien profond « /tableau-de-bord » servi (200)" || echec "lien profond : $profond (200 attendu)"

asset="$(curl -s "$base/" | grep -oE 'assets/[^"]+\.js' | head -1)"
if [ -n "$asset" ]; then
  asset_code="$(code "$base/$asset")"
  [ "$asset_code" = "200" ] && ok "fichier d'assets servi ($asset)" || echec "asset $asset : $asset_code"
else
  echec "aucun fichier d'assets trouvé dans la page d'accueil (application non compilée ?)"
fi

inconnu="$(code "$base/adresse-inconnue")"
[ "$inconnu" = "200" ] && ok "route inconnue renvoyée vers l'application (200)" || echec "route inconnue : $inconnu"

echo
echo "3. Attributs du cookie de session en production"
# La base jetable est neuve : ce compte administrateur vient d'y être créé par
# l'API elle-même, il ne dépend donc d'aucune donnée de démonstration.
cookie="$(curl -s -X POST -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_MOTDEPASSE\"}" \
  -D- -o /dev/null "$base/api/v1/auth/login" | grep -i '^set-cookie' | tr -d '\r')"

if [ -z "$cookie" ]; then
  echec "aucun cookie renvoyé à la connexion (le compte administrateur a-t-il été créé ?)"
else
  echo "$cookie" | grep -qi 'Secure'              && ok "cookie marqué Secure"              || echec "cookie sans attribut Secure"
  echo "$cookie" | grep -qi 'HttpOnly'            && ok "cookie marqué HttpOnly"            || echec "cookie sans attribut HttpOnly"
  echo "$cookie" | grep -qiE 'SameSite=(Lax|None)' && ok "cookie avec politique SameSite"   || echec "cookie sans politique SameSite"
  detail "$(echo "$cookie" | sed 's/washguard_session=[^;]*/washguard_session=…/')"
fi

echo
if [ $echecs -eq 0 ]; then
  echo "${VERT}Tout est conforme${FIN} : la configuration de production est refusée quand elle est incomplète,"
  echo "l'application compilée est servie et le cookie de session est protégé."
  exit 0
fi
echo "${ROUGE}$echecs contrôle(s) en échec.${FIN} Corrigez avant de déployer."
exit 1
