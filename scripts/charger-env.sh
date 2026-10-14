#!/usr/bin/env bash
#
# Lecture du fichier .env pour les scripts d'exploitation.
#
# Pourquoi ne pas écrire « set -a; . ./.env; set +a » : le fichier contient une
# clé publique PEM sur plusieurs lignes, et le shell essaie d'exécuter le corps
# de la clé comme des commandes (bruit dans la sortie, valeurs perdues). Le
# serveur Go a son propre parseur (backend/config.go, applyEnvFile) : celui-ci
# applique les mêmes règles, pour que le script et l'application voient la même
# configuration.
#
# Règles (identiques au serveur) :
#   - une variable déjà définie dans l'environnement n'est jamais écrasée ;
#   - une valeur qui commence par « -----BEGIN » est concaténée avec les lignes
#     suivantes jusqu'à la ligne « -----END » ;
#   - les guillemets qui entourent la valeur sont retirés.
#
# Utilisation dans un script :
#   . "$ROOT/scripts/charger-env.sh"
#   charger_env "$ROOT/.env"

charger_env() {
  local fichier="${1:-.env}"
  [ -f "$fichier" ] || return 0

  local ligne cle valeur suite
  while IFS= read -r ligne || [ -n "$ligne" ]; do
    # Espaces de tête et de queue, lignes vides et commentaires.
    ligne="${ligne#"${ligne%%[![:space:]]*}"}"
    ligne="${ligne%"${ligne##*[![:space:]]}"}"
    [ -z "$ligne" ] && continue
    case "$ligne" in \#*) continue ;; esac
    case "$ligne" in *=*) ;; *) continue ;; esac

    cle="${ligne%%=*}"
    valeur="${ligne#*=}"
    cle="${cle//[[:space:]]/}"

    # Clé PEM : la valeur continue sur les lignes suivantes.
    if [[ "$valeur" == -----BEGIN* && "$valeur" != *"-----END"* ]]; then
      while IFS= read -r suite || [ -n "$suite" ]; do
        valeur="${valeur}"$'\n'"${suite}"
        [[ "$suite" == *"-----END"* ]] && break
      done
    fi

    # Guillemets de contour (comme strings.Trim(value, "\"'") côté serveur).
    while [[ "$valeur" == [\"\']* ]]; do valeur="${valeur#?}"; done
    while [[ "$valeur" == *[\"\'] ]]; do valeur="${valeur%?}"; done

    # Un nom de variable doit être un identifiant shell, sinon on ignore.
    [[ "$cle" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue

    # Priorité à l'environnement : « API_PORT=8099 bash script.sh » doit gagner.
    if [ -z "${!cle:-}" ]; then
      export "$cle=$valeur"
    fi
  done < "$fichier"

  return 0
}
