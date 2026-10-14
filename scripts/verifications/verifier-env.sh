#!/usr/bin/env bash
# Vérifie le fichier .env avant de démarrer WashGuard.
#
#   bash scripts/verifications/verifier-env.sh [dev|prod]
#
# Contrôle trois choses, sans jamais afficher un secret :
#   1. le fichier est bien ignoré par git et n'est pas suivi (sécurité) ;
#   2. ce qui est renseigné, ce qui manque, et ce qui est obligatoire ;
#   3. les incohérences classiques (port invalide, production avec les
#      données de démonstration, « none » sans HTTPS, valeur restée à
#      « à remplir »…).
#
# Code de sortie : 0 si tout est cohérent, 1 si quelque chose bloque le
# démarrage, 2 si le fichier est absent ou dangereux (suivi par git).
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:-dev}"
FICHIER="$ROOT/.env"

erreurs=0
avertissements=0

rouge() { printf '\033[31m%s\033[0m' "$1"; }
vert() { printf '\033[32m%s\033[0m' "$1"; }
jaune() { printf '\033[33m%s\033[0m' "$1"; }

echo "Configuration WashGuard — mode : $MODE"
echo

# --- 1. Le fichier existe et ne partira jamais dans le dépôt ---------------
if [ ! -f "$FICHIER" ]; then
  echo "$(rouge '✗') $FICHIER est absent."
  echo "  Copiez .env.example en .env, puis renseignez vos valeurs."
  exit 2
fi

if git -C "$ROOT" ls-files --error-unmatch .env >/dev/null 2>&1; then
  echo "$(rouge '✗') .env est SUIVI par git : il ne doit jamais l'être."
  echo "  Retirez-le du suivi :  git rm --cached .env"
  exit 2
fi

if git -C "$ROOT" check-ignore -q .env; then
  echo "$(vert '✓') .env ignoré par git (il ne peut pas être committé par erreur)"
else
  echo "$(rouge '✗') .env n'est PAS ignoré par git : ajoutez « .env » à .gitignore."
  exit 2
fi

# --- 2. Chargement sans écraser l'environnement existant -------------------
# Le serveur applique la même règle : une variable déjà définie gagne.
while IFS= read -r ligne; do
  ligne="${ligne#"${ligne%%[![:space:]]*}"}"   # espaces de début
  [ -z "$ligne" ] && continue
  case "$ligne" in \#*) continue ;; esac
  # Les lignes sans « = » sont la suite d'une valeur multi-lignes (clé PEM) :
  # elles ne sont pas des variables, on les ignore.
  case "$ligne" in *=*) ;; *) continue ;; esac
  cle="${ligne%%=*}"
  valeur="${ligne#*=}"
  cle="$(printf '%s' "$cle" | tr -d '[:space:]')"
  [ -z "$cle" ] && continue
  # Nom de variable valide uniquement (lettres, chiffres, souligné).
  case "$cle" in *[!A-Za-z0-9_]*) continue ;; esac
  if [ -z "${!cle:-}" ]; then
    export "$cle=$valeur"
  fi
done < "$FICHIER"

# --- 3. Inventaire ---------------------------------------------------------
# nom|secret?|obligatoire en dev|obligatoire en prod|rôle
# « secret » indique qu'une valeur ne doit jamais être affichée, même en
# entier ; le nom `est_secret()` complète la règle pour les clés ajoutées par
# l'utilisateur (JWKS_PUBLIC_KEY, VITE_CLERK_PUBLISHABLE_KEY…).
VARIABLES=(
  "DATABASE_URL|1|0|0|Connexion PostgreSQL (Neon)"
  "NEON_DATABASE_URL|1|0|0|Connexion Neon (prioritaire sur DATABASE_URL)"
  "API_PORT|0|0|0|Port d'écoute de l'API"
  "APP_ENV|0|0|1|development ou production"
  "APP_URL|0|0|1|Adresse publique de l'application"
  "STATIC_DIR|0|0|0|Dossier du front compilé (dist/)"
  "SEED_DEMO|0|0|1|Données de démonstration au démarrage"
  "CORS_ALLOWED_ORIGINS|0|0|1|Origines autorisées (virgules)"
  "SESSION_COOKIE_SECURE|0|0|1|Cookie de session en HTTPS"
  "SESSION_COOKIE_SAMESITE|0|0|0|lax, strict ou none"
  "SESSION_TTL_HOURS|0|0|0|Durée de vie d'une session (heures)"
  "AUTH_MAX_FAILURES|0|0|0|Échecs de connexion tolérés"
  "REGISTER_MAX_PER_HOUR|0|0|0|Inscriptions tolérées par heure et par IP"
  "CLERK_SECRET_KEY|1|0|0|Clé secrète Clerk (optionnel)"
  "VITE_CLERK_PUBLISHABLE_KEY|0|0|0|Clé publique Clerk pour le front (optionnel)"
  "NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY|0|0|0|Clé publique Clerk (nom hérité)"
  "JWKS_URL_CLERK|0|0|0|URL JWKS Clerk (déduite si absente)"
  "NEXT_PUBLIC_CLERK_API_URL|0|0|0|URL Clerk (nom hérité)"
  "CLERK_AUTHORIZED_PARTIES|0|0|0|Origines acceptées pour le jeton Clerk"
  "CLERK_WEBHOOK_SIGNING_SECRET|1|0|0|Signature des webhooks Clerk (optionnel)"
  "PLATFORM_ADMIN_PASSWORD|1|0|0|Mot de passe du compte plateforme (hors production)"
)

# est_secret : le nom de la variable suffit-il à décider qu'on n'affiche pas
# sa valeur ? (clés, jetons, mots de passe, chaînes de connexion)
est_secret() {
  case "$1" in
    *SECRET*|*PASSWORD*|*TOKEN*|*_KEY|*KEY_|*PUBLISHABLE*|*DATABASE_URL*|*PWD*) return 0 ;;
    *) return 1 ;;
  esac
}

echo
echo "Variables"
manquantes_obligatoires=()
for entree in "${VARIABLES[@]}"; do
  IFS='|' read -r nom secret obligatoire_dev obligatoire_prod role <<< "$entree"
  obligatoire=0
  [ "$MODE" = "prod" ] && obligatoire=$obligatoire_prod
  [ "$MODE" = "dev" ] && obligatoire=$obligatoire_dev

  valeur="${!nom:-}"
  if [ -n "$valeur" ]; then
    if [ "$secret" = "1" ] || est_secret "$nom"; then
      printf '  %s %-34s renseignée (%d caractères)\n' "$(vert '✓')" "$nom" "${#valeur}"
    else
      printf '  %s %-34s %s\n' "$(vert '✓')" "$nom" "$valeur"
    fi
  elif [ "$obligatoire" = "1" ]; then
    printf '  %s %-34s MANQUANTE — %s\n' "$(rouge '✗')" "$nom" "$role"
    manquantes_obligatoires+=("$nom")
  else
    printf '  %s %-34s vide (facultatif — %s)\n' "$(jaune '·')" "$nom" "$role"
  fi
done

if [ "${#manquantes_obligatoires[@]}" -gt 0 ]; then
  echo
  echo "$(rouge '✗') Variables obligatoires manquantes en mode $MODE : ${manquantes_obligatoires[*]}"
  echo "  Le serveur ne démarrera pas (il ne saura pas où est la base)."
  erreurs=$((erreurs + 1))
fi

# --- 4. Cohérence des valeurs ---------------------------------------------
echo
echo "Cohérence"

verifier() { # message, condition
  if [ "$2" = "1" ]; then
    printf '  %s %s\n' "$(vert '✓')" "$1"
  else
    printf '  %s %s\n' "$(rouge '✗')" "$1"
    erreurs=$((erreurs + 1))
  fi
}
avertir() {
  printf '  %s %s\n' "$(jaune '!')" "$1"
  avertissements=$((avertissements + 1))
}

base="${NEON_DATABASE_URL:-${DATABASE_URL:-}}"
# Les deux noms sont acceptés (le serveur préfère DATABASE_URL quand les deux
# sont définis, puis retombe sur NEON_DATABASE_URL) :
# ce qui compte est qu'une chaîne de connexion soit disponible.
verifier "au moins une chaîne de connexion (DATABASE_URL ou NEON_DATABASE_URL)" \
  "$([ -n "$base" ] && echo 1 || echo 0)"
if [ -n "$base" ]; then
  case "$base" in
    postgres://*|postgresql://*) verifier "la chaîne de connexion est bien une URL PostgreSQL" 1 ;;
    *) verifier "la chaîne de connexion doit commencer par postgresql:// ou postgres://" 0 ;;
  esac
fi

port="${API_PORT:-3000}"
case "$port" in
  ''|*[!0-9]*) verifier "API_PORT doit être un nombre (valeur : « $port »)" 0 ;;
  *) if [ "$port" -ge 1 ] && [ "$port" -le 65535 ]; then
       verifier "API_PORT = $port" 1
     else
       verifier "API_PORT hors plage (1-65535) : $port" 0
     fi ;;
esac

case "${APP_ENV:-development}" in
  development|production|test) verifier "APP_ENV = ${APP_ENV:-development}" 1 ;;
  *) verifier "APP_ENV doit valoir development, production ou test (valeur : « ${APP_ENV:-} »)" 0 ;;
esac

samesite="$(printf '%s' "${SESSION_COOKIE_SAMESITE:-lax}" | tr '[:upper:]' '[:lower:]')"
case "$samesite" in
  lax|strict) verifier "SESSION_COOKIE_SAMESITE = $samesite" 1 ;;
  none)
    if [ "${SESSION_COOKIE_SECURE:-false}" = "true" ]; then
      verifier "SameSite=none avec cookie sécurisé (HTTPS)" 1
    else
      verifier "SameSite=none exige SESSION_COOKIE_SECURE=true" 0
    fi ;;
  *) verifier "SESSION_COOKIE_SAMESITE doit valoir lax, strict ou none (valeur : « $samesite »)" 0 ;;
esac

for couple in "AUTH_MAX_FAILURES:${AUTH_MAX_FAILURES:-10}" "REGISTER_MAX_PER_HOUR:${REGISTER_MAX_PER_HOUR:-5}"; do
  nom="${couple%%:*}"; valeur="${couple#*:}"
  case "$valeur" in
    ''|*[!0-9]*) verifier "$nom doit être un nombre entier (valeur : « $valeur »)" 0 ;;
    *) verifier "$nom = $valeur" 1 ;;
  esac
done

# Valeurs restées à l'état de gabarit
for entree in "${VARIABLES[@]}"; do
  IFS='|' read -r nom _ _ _ _ <<< "$entree"
  valeur="${!nom:-}"
  case "$valeur" in
    '' ) ;;
    *remplir*|*REMPLIR*|*changeme*|*CHANGEME*|*xxx*|*XXX*|*votre_*|*VOTRE_*)
      avertir "$nom semble être resté au gabarit (« $valeur »)" ;;
  esac
done

if [ "$MODE" = "prod" ]; then
  [ "${SEED_DEMO:-true}" = "false" ] && verifier "données de démonstration désactivées" 1 \
    || verifier "SEED_DEMO doit valoir false en production" 0
  [ "${SESSION_COOKIE_SECURE:-false}" = "true" ] && verifier "cookie de session sécurisé (HTTPS)" 1 \
    || verifier "SESSION_COOKIE_SECURE doit valoir true en production" 0
  case "${CORS_ALLOWED_ORIGINS:-}" in
    *localhost*|*127.0.0.1*) verifier "CORS_ALLOWED_ORIGINS ne doit pas contenir localhost en production" 0 ;;
    '') : ;;
    *) verifier "origines CORS déclarées explicitement" 1 ;;
  esac
else
  [ "${SEED_DEMO:-true}" = "true" ] && avertir "SEED_DEMO=true : les données de démonstration seront créées (normal en développement)" || true
fi

echo
if [ "$erreurs" -gt 0 ]; then
  echo "$(rouge '✗') $erreurs point(s) bloquant(s), $avertissements avertissement(s)."
  echo "  Corrigez $FICHIER puis relancez ce contrôle."
  exit 1
fi
echo "$(vert '✓') Configuration cohérente ($avertissements avertissement(s))."
echo "  Démarrage possible : bash scripts/start.sh"
