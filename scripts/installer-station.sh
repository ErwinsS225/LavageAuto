#!/usr/bin/env bash
#
# Installation guidée de WashGuard sur l'ordinateur de la station.
#
# Objectif : une application accessible depuis les téléphones du réseau Wi-Fi,
# qui redémarre toute seule et se sauvegarde chaque soir, sans domaine ni serveur.
#
# Ce que fait le script :
#   1. vérifie ce qui manque sur la machine (Docker, Go, Node, npm) ;
#   2. crée le fichier .env (mot de passe administrateur, nom de la station) ;
#   3. démarre PostgreSQL, crée le schéma, installe le frontend et le compile ;
#   4. enregistre deux tâches automatiques macOS (launchd) : l'API au démarrage
#      (redémarrage automatique en cas d'arrêt) et la sauvegarde quotidienne ;
#   5. affiche l'adresse à ouvrir depuis les téléphones de la station.
#
# Utilisation :
#     bash scripts/installer-station.sh                 # installation
#     bash scripts/installer-station.sh --verifier      # contrôle seul, ne modifie rien
#     bash scripts/installer-station.sh --desinstaller  # retire les tâches automatiques
#
# Le script est relançable sans risque : il ne recrée pas ce qui existe.

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1

MODE="installer"
case "${1:-}" in
  --verifier) MODE="verifier" ;;
  --desinstaller) MODE="desinstaller" ;;
  "") ;;
  *) echo "Option inconnue : $1 (utilisez --verifier ou --desinstaller)"; exit 2 ;;
esac

VERT=$'\033[32m'; ROUGE=$'\033[31m'; JAUNE=$'\033[33m'; GRIS=$'\033[90m'; FIN=$'\033[0m'
ETIQUETTE_DOMAINE="ci.washguard.station"

titre()  { printf '\n\033[1m%s\033[0m\n' "$1"; }
ok()     { printf '  %s✅%s %s\n' "$VERT" "$FIN" "$1"; }
ko()     { printf '  %s❌%s %s\n' "$ROUGE" "$FIN" "$1"; }
attention() { printf '  %s⚠️%s  %s\n' "$JAUNE" "$FIN" "$1"; }
infos()  { printf '  %s%s%s\n' "$GRIS" "$1" "$FIN"; }

MANQUANT=0
DOCKER_PRET=0

# --- Outils ------------------------------------------------------------------

titre "1. Ce qui est installé sur cet ordinateur"

verifier_outil() { # nom, commande de test, conseil d'installation
  if eval "$2" >/dev/null 2>&1; then
    ok "$1 : $(eval "$2" 2>&1 | head -1)"
  else
    ko "$1 absent — $3"
    MANQUANT=1
  fi
}

verifier_outil "Go"      "go version"              "lancez : brew install go"
verifier_outil "Node.js" "node --version"          "lancez : brew install node"
verifier_outil "npm"     "npm --version"           "vient avec Node.js"
verifier_outil "Git"     "git --version"           "lancez : brew install git"

# Deux façons d'avoir PostgreSQL : Docker (recommandé, tout est prévu par le
# projet) ou un PostgreSQL déjà installé sur la machine (Postgres.app, Homebrew).
# Docker n'est donc pas obligatoire : on regarde d'abord si une base répond.
if [ -f .env ]; then
  # shellcheck source=charger-env.sh
  . "$ROOT/scripts/charger-env.sh"
  charger_env "$ROOT/.env"
fi

# Le serveur accepte DATABASE_URL ou NEON_DATABASE_URL : la détection de la base
# doit accepter les deux, sinon une installation Neon valide était signalée comme
# « aucun PostgreSQL disponible ».
DATABASE_URL="${DATABASE_URL:-${NEON_DATABASE_URL:-}}"
export DATABASE_URL

if docker info >/dev/null 2>&1; then
  ok "Docker : démarré (PostgreSQL sera géré par Docker)"
  DOCKER_PRET=1
elif [ -n "${DATABASE_URL:-}" ] && command -v pg_isready >/dev/null 2>&1 && pg_isready -d "$DATABASE_URL" >/dev/null 2>&1; then
  ok "PostgreSQL : déjà en service (sans Docker)"
  DOCKER_PRET=0
else
  if command -v docker >/dev/null 2>&1; then
    ko "Docker est installé mais pas démarré — ouvrez Docker Desktop et attendez qu'il soit prêt"
  else
    ko "Aucun PostgreSQL disponible : ni Docker démarré, ni base joignable"
    infos "le plus simple : installez Docker Desktop (https://www.docker.com/products/docker-desktop)"
    infos "autre solution : PostgreSQL déjà installé, puis créez la base (voir docs/installation-station.md)"
  fi
  MANQUANT=1
fi

if [ "$MODE" = "verifier" ]; then
  [ -f .env ] && ok "fichier .env présent" || attention "fichier .env absent (il sera créé à l'installation)"
  [ -d node_modules ] && ok "dépendances du frontend installées" || attention "dépendances du frontend à installer"
  [ -f dist/index.html ] && ok "interface compilée" || attention "interface à compiler"
  printf '\n'
  if [ "$MANQUANT" = "0" ]; then
    echo "${VERT}Tout est prêt pour installer.${FIN} Lancez : bash scripts/installer-station.sh"
  else
    echo "${ROUGE}Il manque des outils.${FIN} Installez-les (voir ci-dessus), puis relancez ce contrôle."
    exit 1
  fi
  exit 0
fi

if [ "$MANQUANT" = "1" ]; then
  echo
  echo "${ROUGE}Installation interrompue :${FIN} installez d'abord les outils manquants indiqués ci-dessus."
  echo "Sur macOS, la commande la plus simple est : brew install go node git"
  echo "Docker Desktop se télécharge ici : https://www.docker.com/products/docker-desktop"
  exit 1
fi

# --- Retrait des tâches automatiques ----------------------------------------

PLIST_API="$HOME/Library/LaunchAgents/$ETIQUETTE_DOMAINE.api.plist"
PLIST_SAUVEGARDE="$HOME/Library/LaunchAgents/$ETIQUETTE_DOMAINE.sauvegarde.plist"

if [ "$MODE" = "desinstaller" ]; then
  titre "Retrait des tâches automatiques"
  for plist in "$PLIST_API" "$PLIST_SAUVEGARDE"; do
    if [ -f "$plist" ]; then
      launchctl bootout "gui/$(id -u)" "$plist" >/dev/null 2>&1
      rm -f "$plist"
      ok "retirée : $(basename "$plist")"
    else
      infos "déjà absente : $(basename "$plist")"
    fi
  done
  printf '\n'
  echo "L'API n'est plus lancée automatiquement. Pour l'arrêter tout de suite :"
  echo "  pkill -f washguard-api"
  echo "Vos données restent dans la base (dossier backups/ et volume Docker)."
  exit 0
fi

# --- Configuration -----------------------------------------------------------

titre "2. Configuration"

if [ -f .env ]; then
  ok "fichier .env déjà présent (il n'est pas modifié)"
  # shellcheck source=charger-env.sh
  . "$ROOT/scripts/charger-env.sh"
  charger_env "$ROOT/.env"
else
  MOT_DE_PASSE=""
  while [ "${#MOT_DE_PASSE}" -lt 10 ]; do
    printf '  Mot de passe de l’administrateur WashGuard (espace plateforme, 10 caractères minimum)\n'
    printf '  %sMot de passe : %s' "$GRIS" "$FIN"
    read -r MOT_DE_PASSE
  done
  printf '  %sNom de la station (ex. Station Étoile) : %s' "$GRIS" "$FIN"
  read -r NOM_STATION
  [ -z "$NOM_STATION" ] && NOM_STATION="Ma station"

  cat > .env <<ENV
# Configuration WashGuard — installation locale de la station.
# Modifiable : relancez les services après toute modification (.env).

DATABASE_URL=postgresql://washguard:washguard@localhost:5432/washguard
TEST_DATABASE_URL=postgresql://washguard:washguard@localhost:5432/washguard_test

# Dossier du frontend compilé (relatif au dossier backend/).
STATIC_DIR=../dist

# Réseau local sans HTTPS : le cookie reste utilisable. Passez à true le jour
# où vous ajoutez un certificat (voir docs/deploiement-https.md).
SESSION_COOKIE_SECURE=false

# Espace d'administration de la plateforme.
PLATFORM_ADMIN_EMAIL=admin@washguard.local
PLATFORM_ADMIN_PASSWORD="$MOT_DE_PASSE"
PLATFORM_ADMIN_NAME="Administrateur $NOM_STATION"
ENV
  chmod 600 .env
  ok "fichier .env créé (droits restreints à votre compte)"
  infos "mot de passe administrateur enregistré dans .env"
fi

titre "3. Base de données"
if [ "${DOCKER_PRET:-0}" = "1" ]; then
  if docker compose up -d db >/dev/null 2>&1; then
    ok "PostgreSQL démarré (conteneur Docker)"
  else
    ko "impossible de démarrer PostgreSQL — vérifiez Docker Desktop, puis relancez"
    exit 1
  fi
  for _ in $(seq 1 30); do
    if docker compose exec -T db pg_isready -U washguard >/dev/null 2>&1; then break; fi
    sleep 1
  done
  docker compose exec -T db pg_isready -U washguard >/dev/null 2>&1 && ok "PostgreSQL répond" || { ko "PostgreSQL ne répond pas"; exit 1; }
else
  ok "PostgreSQL déjà en service : rien à démarrer"
fi

titre "4. Application"

# Les tables sont créées/mises à jour au démarrage de l'API (migration idempotente
# embarquée dans le binaire Go, cf. schema.sql). On les applique ici sans lancer
# le serveur, via un module temporaire qui appelle migrate() puis s'arrête.
if (cd backend && go run . -migrate-only >/dev/null 2>&1); then
  ok "tables créées / mises à jour (migrations)"
else
  ko "les migrations ont échoué — vérifiez .env (DATABASE_URL)"
  exit 1
fi

if [ ! -d node_modules ]; then
  infos "installation des dépendances du frontend (quelques minutes)…"
  (npm install --no-audit --no-fund >/dev/null 2>&1) && ok "dépendances installées" || { ko "npm install a échoué"; exit 1; }
else
  ok "dépendances du frontend déjà installées"
fi

if (npm run build >/dev/null 2>&1); then
  ok "interface compilée"
else
  ko "la compilation de l'interface a échoué"
  exit 1
fi

if (cd backend && go build -o "$ROOT/washguard-api" . >/dev/null 2>&1); then
  ok "exécutable de l'API compilé (washguard-api)"
else
  ko "la compilation de l'API a échoué"
  exit 1
fi

# --- Démarrage automatique (macOS) ------------------------------------------

if [ "$(uname -s)" != "Darwin" ]; then
  attention "démarrage automatique non configuré : ce système n'est pas macOS"
  infos "sur macOS, cette étape enregistre l'API comme service (launchd)"
else
  titre "5. Démarrage automatique et sauvegarde quotidienne"
  mkdir -p "$HOME/Library/LaunchAgents"

  cat > "$PLIST_API" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$ETIQUETTE_DOMAINE.api</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/bin/caffeinate</string>
    <string>-i</string>
    <string>-s</string>
    <string>$ROOT/scripts/demarrer-washguard.sh</string>
  </array>
  <key>WorkingDirectory</key><string>$ROOT</string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>$ROOT/backups/api.log</string>
  <key>StandardErrorPath</key><string>$ROOT/backups/api.log</string>
</dict>
</plist>
PLIST

  cat > "$PLIST_SAUVEGARDE" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$ETIQUETTE_DOMAINE.sauvegarde</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>$ROOT/scripts/sauvegarde-quotidienne.sh</string>
  </array>
  <key>WorkingDirectory</key><string>$ROOT</string>
  <key>StartCalendarInterval</key>
  <dict><key>Hour</key><integer>22</integer><key>Minute</key><integer>30</integer></dict>
  <key>StandardOutPath</key><string>$ROOT/backups/launchd-sauvegarde.log</string>
  <key>StandardErrorPath</key><string>$ROOT/backups/launchd-sauvegarde.log</string>
</dict>
</plist>
PLIST

  mkdir -p "$ROOT/backups"
  for plist in "$PLIST_API" "$PLIST_SAUVEGARDE"; do
    launchctl bootout "gui/$(id -u)" "$plist" >/dev/null 2>&1
    if launchctl bootstrap "gui/$(id -u)" "$plist" >/dev/null 2>&1; then
      ok "tâche enregistrée : $(basename "$plist")"
    else
      ko "impossible d'enregistrer $(basename "$plist")"
    fi
  done
  infos "l'API redémarre seule en cas d'arrêt et au démarrage de la session"
  infos "sauvegarde automatique chaque soir à 22 h 30 (conservation 14 jours)"
fi

# --- Adresse d'accès ---------------------------------------------------------

titre "6. Adresse à utiliser"

bash "$ROOT/scripts/adresse-locale.sh" || true

cat <<FIN

${VERT}Installation terminée.${FIN}

  • Depuis cet ordinateur : ouvrez l'adresse ci-dessus dans votre navigateur.
  • Depuis les téléphones de la station : connectez-les au même Wi-Fi, puis
    saisissez l'adresse « réseau local » affichée ci-dessus.
  • Le mot de passe de l'administrateur WashGuard est dans le fichier .env.
  • Créez vos comptes de travail depuis « Employés » (Propriétaire, Gérant, Caissier).

  Commandes utiles :
    bash scripts/adresse-locale.sh              # réafficher l'adresse
    bash scripts/sauvegarde-quotidienne.sh      # sauvegarder maintenant
    bash scripts/restaurer-sauvegarde.sh backups/<fichier>.sql.gz   # restaurer une sauvegarde
    bash scripts/installer-station.sh --desinstaller   # retirer le démarrage automatique

  ${JAUNE}À savoir :${FIN} l'ordinateur doit rester allumé et connecté au Wi-Fi.
  Passez-le en « ne pas mettre en veille » (Réglages → Économiseur d'énergie).
FIN
