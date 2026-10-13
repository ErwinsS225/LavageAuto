#!/usr/bin/env bash
#
# Sauvegarde quotidienne de la base WashGuard.
#
# Écrit une sauvegarde complète compressée dans backups/ puis supprime les
# sauvegardes de plus de N jours. Conçu pour être lancé automatiquement (launchd
# sur macOS, cron sur Linux) : chaque exécution est journalisée.
#
# Utilisation :  bash scripts/sauvegarde-quotidienne.sh
# Variables :    SAUVEGARDE_JOURS (défaut 14)   BACKUP_DIR (défaut backups)
#
# Le script essaie pg_dump local, puis le conteneur Docker (installation type
# « station », où PostgreSQL tourne le plus souvent dans Docker).

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1

JOURS="${SAUVEGARDE_JOURS:-14}"
DOSSIER="${BACKUP_DIR:-backups}"
JOURNAL="$DOSSIER/journal-sauvegardes.log"
HORODATAGE="$(date +%Y-%m-%d-%H%M)"
FICHIER="$DOSSIER/washguard-$HORODATAGE.sql.gz"

# .env pour DATABASE_URL (les variables déjà définies ne sont pas écrasées).
if [ -f .env ]; then
  # shellcheck disable=SC1091
  set -a; . ./.env; set +a
fi

mkdir -p "$DOSSIER"

noter() { printf '%s  %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$1" | tee -a "$JOURNAL"; }

if [ -z "${DATABASE_URL:-}" ]; then
  noter "ÉCHEC : DATABASE_URL est absent (fichier .env introuvable ?)"
  exit 1
fi

# --- Sauvegarde --------------------------------------------------------------

if command -v pg_dump >/dev/null 2>&1; then
  if pg_dump "$DATABASE_URL" --no-owner --clean --if-exists | gzip > "$FICHIER"; then
    noter "OK    sauvegarde locale : $FICHIER ($(du -h "$FICHIER" | cut -f1))"
  else
    rm -f "$FICHIER"
    noter "ÉCHEC : pg_dump a échoué (PostgreSQL est-il démarré ?)"
    exit 1
  fi
else
  if docker compose exec -T db pg_dump -U washguard --no-owner --clean --if-exists washguard 2>/dev/null | gzip > "$FICHIER"; then
    noter "OK    sauvegarde via Docker : $FICHIER ($(du -h "$FICHIER" | cut -f1))"
  else
    rm -f "$FICHIER"
    noter "ÉCHEC : pg_dump indisponible en local et via Docker. La sauvegarde n'a pas été faite."
    exit 1
  fi
fi

# Une sauvegarde vide (0 octet utile) signale un problème : on le dit clairement.
TAILLE=$(wc -c < "$FICHIER" | tr -d ' ')
if [ "$TAILLE" -lt 1000 ]; then
  noter "ATTENTION : le fichier fait seulement $TAILLE octets, vérifiez la sauvegarde."
fi

# --- Rotation ----------------------------------------------------------------

if command -v find >/dev/null 2>&1; then
  SUPPRIMEES=$(find "$DOSSIER" -maxdepth 1 -name 'washguard-*.sql.gz' -type f -mtime "+$JOURS" -print -delete | wc -l | tr -d ' ')
  if [ "$SUPPRIMEES" != "0" ]; then
    noter "INFO  $SUPPRIMEES ancienne(s) sauvegarde(s) de plus de $JOURS jours supprimée(s)"
  fi
fi

noter "INFO  $(find "$DOSSIER" -maxdepth 1 -name 'washguard-*.sql.gz' -type f | wc -l | tr -d ' ') sauvegarde(s) conservée(s) dans $DOSSIER/"
exit 0
