# Installer WashGuard sur l'ordinateur de la station

Objectif : une application ouverte depuis l'ordinateur du bureau **et** depuis les
téléphones de la station (même réseau Wi-Fi), qui redémarre toute seule et se
sauvegarde chaque soir, sans domaine ni serveur loué.

Pour une mise en production avec un nom de domaine et HTTPS, voir
[`deploiement-https.md`](deploiement-https.md).

## 1. Ce qu'il faut sur la machine

| Outil | Version | Rôle |
| --- | --- | --- |
| Go | 1.23 ou plus | compile l'API |
| Node.js | 20 ou plus | compile l'interface |
| PostgreSQL | 17 (Docker accepté) | stocke les données |
| Docker Desktop | facultatif | installe PostgreSQL en un clic |

```bash
make doctor              # dit précisément ce qui manque, ne modifie rien
make install             # dépendances du front (npm ci)
```

## 2. Renseigner la configuration

Le fichier `.env` contient la base et les secrets. Il n'est **jamais** committé.
Pour le créer ou le compléter sans jamais afficher de secret :

```bash
printf 'NEON_DATABASE_URL=postgresql://...\n' | bash scripts/verifications/maj-env.sh dev
bash scripts/verifications/verifier-env.sh dev      # contrôle, n'affiche que les noms
```

Base locale plutôt que Neon : `make db` démarre PostgreSQL dans Docker
(`postgresql://washguard:washguard@127.0.0.1:5432/washguard`).

`SEED_DEMO=false` sur une station réelle : les données de démonstration ne doivent
jamais se mélanger aux lavages du jour.

## 3. Installation guidée (macOS)

```bash
make verifier-station    # vérifie ce qui manque, ne modifie rien
make station             # installation guidée
make adresse             # affiche l'adresse à ouvrir depuis les téléphones
```

`make station` crée le `.env`, démarre PostgreSQL, applique les migrations,
installe et compile l'interface, puis enregistre **deux tâches automatiques**
(launchd) :

- **l'API au démarrage** de la session : `washguard-api`, relancée si elle s'arrête ;
- **la sauvegarde quotidienne** : `bash scripts/sauvegarde-quotidienne.sh`.

Il affiche enfin l'adresse du type `http://192.168.1.24:3000` à saisir sur les
téléphones (à mettre en favori ; le Wi-Fi du bureau doit autoriser les appareils
à se parler).

## 4. Démarrage manuel (Linux, ou dépannage)

```bash
bash scripts/start.sh    # compile le front, compile l'API, lance les deux
```

Le binaire sert **à la fois** l'API et l'interface (`STATIC_DIR=dist`) : un seul
port à ouvrir, celui de `API_PORT` dans `.env` (3000 dans le gabarit), donc rien
à configurer dans le navigateur des employés.

## 5. Mise à jour

```bash
git pull
make install && make build       # dépendances + API + interface
make check                       # formatage, tests, routes, écrans, configuration
# puis redémarrer le service (launchd le fait seul après un redémarrage de session)
```

Les migrations de schéma sont appliquées automatiquement au démarrage ; en cas de
doute, `make migrate` les joue sans démarrer le serveur.

## 6. Sauvegardes

```bash
make backup                                     # maintenant
SAUVEGARDE_JOURS=30 make backup                 # conserve 30 jours
bash scripts/sauvegarde-quotidienne.sh          # ce que lance launchd chaque soir
```

Les fichiers arrivent dans `backups/` (dump `pg_dump` compressé). Détails, restauration
et test de restauration : [`sauvegarde-restauration.md`](sauvegarde-restauration.md).

## 7. Dépannage

| Symptôme | À essayer |
| --- | --- |
| « Configuration incoherente » | `bash scripts/verifications/verifier-env.sh dev` : le nom du réglage fautif est affiché, jamais sa valeur. |
| L'API refuse de démarrer | `make doctor`, puis vérifier que PostgreSQL tourne (`make db`) et que `.env` pointe dessus. |
| `port déjà utilisé` | Changer `API_PORT` dans `.env`, relancer `bash scripts/start.sh`, puis `make adresse`. |
| Les téléphones n'ouvrent pas la page | Même Wi-Fi ? Isolation des clients (AP isolation) désactivée sur le routeur ? `make adresse` confirme que l'API répond. |
| Écran noir après connexion | Vider le cache du navigateur, puis `make smoke` : si les 11 écrans se rendent, le problème est côté navigateur, pas côté code. |
| « Aucune caisse ouverte » | C'est une règle, pas une panne : ouvrir la caisse depuis l'écran Caisse avant d'encaisser des espèces. |

## 8. Ce qui n'est pas installé

WashGuard n'imprime pas de tickets, ne parle pas aux terminaux de paiement mobile
money et ne fait pas de comptabilité. Il remplace le cahier : les montants sont
ceux que l'équipe saisit, et l'écran Caisse sert à repérer les écarts en fin de
journée.
