#!/usr/bin/env bash
#
# Affiche l'adresse à ouvrir dans un navigateur : sur cet ordinateur, et depuis
# les téléphones du réseau Wi-Fi de la station. Vérifie au passage que l'API
# répond vraiment.
#
# Utilisation : bash scripts/adresse-locale.sh

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# .env d'abord (c'est lui qui fixe le port réellement écouté) : sans cela, le
# script annonçait 8080 alors que l'application, elle, écoute 3000 — et les
# téléphones de la station recevaient une adresse morte. Une variable passée à
# la main garde la priorité sur le fichier.
# shellcheck source=charger-env.sh
. "$ROOT/scripts/charger-env.sh"
charger_env "$ROOT/.env"
PORT="${API_PORT:-3000}"

VERT=$'\033[32m'; ROUGE=$'\033[31m'; GRIS=$'\033[90m'; FIN=$'\033[0m'

# --- Adresse réseau local ----------------------------------------------------

adresse_locale() {
  case "$(uname -s)" in
    Darwin)
      for interface in en0 en1 en2; do
        ip="$(ipconfig getifaddr "$interface" 2>/dev/null || true)"
        if [ -n "$ip" ]; then echo "$ip"; return 0; fi
      done
      ;;
    *)
      ip="$(hostname -I 2>/dev/null | tr ' ' '\n' | grep -vE '^(127\.|169\.254\.)' | head -1)"
      if [ -n "$ip" ]; then echo "$ip"; return 0; fi
      ip="$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.*src \([0-9.]*\).*/\1/p' | head -1)"
      if [ -n "$ip" ]; then echo "$ip"; return 0; fi
      ;;
  esac
  return 1
}

# --- État du service ---------------------------------------------------------

etat=""
if command -v curl >/dev/null 2>&1; then
  if curl -fsS -m 3 "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then
    etat="ok"
  else
    etat="ko"
  fi
fi

case "$etat" in
  ok) printf '  %s✅%s WashGuard fonctionne (port %s)\n' "$VERT" "$FIN" "$PORT" ;;
  ko) printf '  %s❌%s WashGuard ne répond pas sur le port %s\n' "$ROUGE" "$FIN" "$PORT"
      printf '  %sDémarrez-le : bash scripts/demarrer-washguard.sh%s\n' "$GRIS" "$FIN" ;;
  *)  printf '  %s(curl absent : état du service non vérifié)%s\n' "$GRIS" "$FIN" ;;
esac

echo
printf '  Sur cet ordinateur   : http://localhost:%s\n' "$PORT"

if ip="$(adresse_locale)"; then
  printf '  Depuis les téléphones : http://%s:%s   (même Wi-Fi)\n' "$ip" "$PORT"
  printf '\n  %sSi un téléphone n’ouvre pas la page : autorisez les connexions entrantes\n' "$GRIS"
  printf '  pour ce programme dans Réglages → Réseau → Pare-feu.%s\n' "$FIN"
else
  printf '  %sAdresse réseau introuvable : l’ordinateur est-il connecté au Wi-Fi ?%s\n' "$GRIS" "$FIN"
fi

# L'adresse change si la box redonne un bail différent : on le signale plutôt
# que de laisser les téléphones tomber sur une page qui ne répond plus.
echo
printf '  %sAstuce : dans votre box, réservez cette adresse pour cet ordinateur\n' "$GRIS"
printf '  (bail fixe), sinon elle peut changer après un redémarrage.%s\n' "$FIN"
