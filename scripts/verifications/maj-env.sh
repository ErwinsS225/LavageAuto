#!/usr/bin/env bash
# Met à jour le fichier .env à partir d'un flux « CLE=valeur », une par ligne.
#
#   printf 'DATABASE_URL=postgresql://…\n' | bash scripts/verifications/maj-env.sh
#   bash scripts/verifications/maj-env.sh < mes-valeurs.txt
#
# Ce que fait le script :
#   - remplace la valeur d'une clé déjà présente, commentaire au-dessus intact ;
#   - ajoute à la fin les clés qui n'existaient pas encore ;
#   - n'écrit jamais un secret dans la sortie : il n'affiche que le nom de la
#     clé et le nombre de caractères écrits ;
#   - conserve une copie de sécurité datée avant toute modification ;
#   - refuse d'écrire si .env est suivi par git (protection contre une fuite).
#
# Il termine par le contrôle complet : verifier-env.sh.
#
# Usage : bash scripts/verifications/maj-env.sh [dev|prod]
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FICHIER="$ROOT/.env"
MODE="${1:-dev}"

rouge() { printf '\033[31m%s\033[0m' "$1"; }
vert() { printf '\033[32m%s\033[0m' "$1"; }
jaune() { printf '\033[33m%s\033[0m' "$1"; }

if git -C "$ROOT" ls-files --error-unmatch .env >/dev/null 2>&1; then
  echo "$(rouge '✗') .env est suivi par git : on n'écrit pas de secret dedans."
  echo "  Retirez-le d'abord du suivi : git rm --cached .env"
  exit 2
fi

if [ ! -f "$FICHIER" ]; then
  echo "$(rouge '✗') $FICHIER est absent. Copiez .env.example en .env."
  exit 2
fi

sauvegarde="$FICHIER.avant-maj"
cp "$FICHIER" "$sauvegarde"
echo "copie de sécurité : $(basename "$sauvegarde")"

# ---------------------------------------------------------------------------
# 1. Lecture du flux « CLE=valeur »
# ---------------------------------------------------------------------------
# Les valeurs sont transmises à Python par l'environnement (jamais en argument
# de ligne de commande, qui serait visible dans la liste des processus).
declare -A valeurs=()
declare -A vues=()

multiligne=0
cle_courante=""

while IFS= read -r ligne || [ -n "$ligne" ]; do
  ligne="${ligne%$'\r'}"

  if [ "$multiligne" = "1" ]; then
    valeurs[$cle_courante]="${valeurs[$cle_courante]}"$'\n'"$ligne"
    case "$ligne" in *"-----END"*) multiligne=0 ;; esac
    continue
  fi

  [ -z "${ligne//[[:space:]]/}" ] && continue
  propre="${ligne#"${ligne%%[![:space:]]*}"}"
  case "$propre" in \#*) continue ;; esac
  propre="${propre#export }"

  case "$propre" in
    *=*) ;;
    *) echo "$(jaune '!') ligne ignorée (aucun « = ») : ${propre:0:20}…"; continue ;;
  esac

  cle="${propre%%=*}"
  valeur="${propre#*=}"
  cle="$(printf '%s' "$cle" | tr -d '[:space:]')"
  [ -z "$cle" ] && continue

  # Espaces et guillemets encadrant la valeur.
  valeur="${valeur#"${valeur%%[![:space:]]*}"}"
  valeur="${valeur%"${valeur##*[![:space:]]}"}"
  case "$valeur" in
    \"*\") valeur="${valeur#\"}"; valeur="${valeur%\"}" ;;
    \'*\') valeur="${valeur#\'}"; valeur="${valeur%\'}" ;;
  esac

  valeurs[$cle]="$valeur"
  vues[$cle]=1
  if [[ "$valeur" == -----BEGIN* ]] && [[ "$valeur" != *"-----END"* ]]; then
    multiligne=1
    cle_courante="$cle"
  fi
done

if [ "${#valeurs[@]}" -eq 0 ]; then
  echo "$(rouge '✗') aucune valeur lue sur l'entrée standard."
  echo "  Format attendu : une ligne « CLE=valeur » par variable."
  exit 2
fi

# ---------------------------------------------------------------------------
# 2. Fusion dans .env (Python : écriture atomique, valeurs multi-lignes)
# ---------------------------------------------------------------------------
export WASHGUARD_ENV_FILE="$FICHIER"
for cle in "${!valeurs[@]}"; do
  export "WASHGUARD_VAL_${cle}=${valeurs[$cle]}"
done

rapport="$(python3 - <<'PY'
import os, pathlib, re

chemin = pathlib.Path(os.environ['WASHGUARD_ENV_FILE'])
prefixe = 'WASHGUARD_VAL_'
couples = [
    (cle[len(prefixe):], valeur)
    for cle, valeur in os.environ.items()
    if cle.startswith(prefixe)
]
texte = chemin.read_text(encoding='utf-8')

mises_a_jour, ajoutees = [], []
for cle, valeur in sorted(couples):
    motif = re.compile(r'^[ \t]*' + re.escape(cle) + r'[ \t]*=[^\n]*$', re.MULTILINE)
    remplacement = f'{cle}={valeur}'
    # Lambda : le contenu n'est pas réinterprété (ni antislashs, ni \g).
    nouveau, n = motif.subn(lambda _m: remplacement, texte, count=1)
    if n:
        texte = nouveau
        mises_a_jour.append((cle, len(valeur)))
    else:
        texte = texte.rstrip('\n') + f'\n\n# Ajoutée par maj-env.sh\n{cle}={valeur}\n'
        ajoutees.append((cle, len(valeur)))

# Écriture atomique : un incident ne laisse pas de .env tronqué.
temporaire = chemin.with_suffix('.env.tmp')
temporaire.write_text(texte, encoding='utf-8')
temporaire.replace(chemin)

for statut, liste in (('mise à jour', mises_a_jour), ('ajoutée', ajoutees)):
    for cle, taille in liste:
        print(f'{statut}\t{cle}\t{taille}')
PY
)"

echo
mises=0
ajouts=0
while IFS=$'\t' read -r statut cle taille; do
  [ -z "$cle" ] && continue
  if [ "$statut" = "mise à jour" ]; then
    printf '  %s %-34s %s (%s caractères)\n' "$(vert '✓')" "$cle" "mise à jour" "$taille"
    mises=$((mises + 1))
  else
    printf '  %s %-34s %s (%s caractères)\n' "$(vert '✓')" "$cle" "ajoutée" "$taille"
    ajouts=$((ajouts + 1))
  fi
done <<< "$rapport"

# --- Clés inconnues du gabarit --------------------------------------------
inattendues=()
for cle in "${!valeurs[@]}"; do
  grep -qE "^[[:space:]]*${cle}=" "$ROOT/.env.example" 2>/dev/null || inattendues+=("$cle")
done
if [ "${#inattendues[@]}" -gt 0 ]; then
  echo
  echo "$(jaune '!') absentes du gabarit .env.example : ${inattendues[*]}"
  echo "  Si elles servent au produit, ajoutez-les au gabarit (sans la valeur)."
fi

# ---------------------------------------------------------------------------
# 3. Contrôle final
# ---------------------------------------------------------------------------
echo
bash "$ROOT/scripts/verifications/verifier-env.sh" "$MODE"
