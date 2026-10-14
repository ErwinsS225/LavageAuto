# Sauvegarde et restauration

Toutes les données de WashGuard (entreprises, stations, lavages, paiements,
dépenses, caisse, commissions) vivent dans **une seule base PostgreSQL**. Ce qui
est sauvegardé, c'est donc cette base ; tout le reste se reconstitue (`make build`
pour l'interface, `git pull` pour le code).

## 1. Sauvegarder

```bash
make backup                        # tout de suite
SAUVEGARDE_JOURS=30 make backup    # conservation portée à 30 jours
bash scripts/sauvegarde-quotidienne.sh
```

Le script écrit `backups/washguard-AAAA-MM-JJ-HHMM.sql.gz` (dump complet,
compressé, avec `--clean --if-exists` pour pouvoir être rejoué sur une base
existante), journalise chaque exécution dans `backups/journal-sauvegardes.log` et
supprime les sauvegardes de plus de `SAUVEGARDE_JOURS` jours (14 par défaut).

| Variable | Défaut | Rôle |
| --- | --- | --- |
| `SAUVEGARDE_JOURS` | `14` | Durée de conservation |
| `BACKUP_DIR` | `backups` | Dossier de destination |
| `DATABASE_URL` / `NEON_DATABASE_URL` | lues dans `.env` | Base à sauvegarder (le serveur comme le script acceptent les deux noms) |

Le dump est fait en local (`pg_dump`) ou, à défaut, dans le conteneur Docker
PostgreSQL de l'installation « station ». **Le dossier `backups/` n'est pas
committé** : il doit être recopié ailleurs (clé USB, disque externe, stockage
cloud) — une sauvegarde qui vit sur la machine qu'elle protège n'en est pas une.

Sur une station macOS, `make station` installe déjà une tâche automatique
quotidienne. Sur Linux, une ligne de cron suffit :

```cron
30 22 * * *  cd /home/washguard/LavageAuto && bash scripts/sauvegarde-quotidienne.sh
```

Neon : en plus du dump, la console Neon propose les **branches** et la
restauration à un instant donné (selon la formule). C'est un filet de sécurité
supplémentaire, pas un remplacement : un dump hors ligne reste la seule protection
contre l'erreur humaine du type « base supprimée ».

## 2. Vérifier que la sauvegarde est utilisable

Un fichier qui existe n'est pas une sauvegarde : il faut l'avoir **déjà
restauré** au moins une fois.

```bash
gunzip -c backups/washguard-2026-10-13-2200.sql.gz | head -5     # le dump contient du SQL
ls -lh backups/                                                   # taille plausible (quelques Mo)
tail -5 backups/journal-sauvegardes.log                           # dernière exécution « OK »
```

Le script avertit lui-même quand un fichier fait moins de 1 000 octets : cela
signale une base vide, un filtre en erreur ou une sauvegarde à rejeter.

## 3. Restaurer

Sur une base **jetable** pour s'entraîner, puis sur la base réelle en cas de
sinistre.

```bash
# Base d'entraînement (le dump est rejouable tel quel : --clean --if-exists)
createdb washguard_test
gunzip -c backups/washguard-2026-10-13-2200.sql.gz | psql "postgresql://…/washguard_test"
psql "postgresql://…/washguard_test" -c "SELECT count(*) FROM wash_orders;"
```

Méthode recommandée en cas de sinistre :

1. **Arrêter l'application** (`launchctl bootout` de la tâche WashGuard, ou
   `sudo systemctl stop washguard`) : personne ne doit écrire pendant la
   restauration.
2. Sauvegarder l'état actuel avant d'y toucher :
   `make backup` (ou renommer la base) — on ne le regrette jamais.
3. Créer une base vide et restaurer le dump dedans (plus sûr que de rejouer le
   dump par-dessus la base abîmée).
4. Pointer `.env` sur la base restaurée, puis redémarrer :
   `bash scripts/start.sh`.
5. Contrôler : ouvrir l'écran **Tableau de bord** du jour de la dernière
   sauvegarde, et vérifier le nombre de lavages et la caisse du jour.

Neon : la restauration se fait dans la console (restauration à un instant donné,
ou création d'une branche à partir d'un point du passé), puis en pointant
`NEON_DATABASE_URL` sur la branche obtenue.

## 4. Ce qui n'est pas dans la sauvegarde

| Élément | Où il vit | À conserver |
| --- | --- | --- |
| `.env` (secrets, chaîne de connexion) | Machine | Copie chiffrée à part, jamais avec le dump |
| Code et migrations | Git (`git push` sur un dépôt privé) | Le dépôt lui-même |
| Interface compilée (`dist/`) | Machine | Reconstruite par `make build` |
| Exports CSV déjà téléchargés | Poste de l'utilisateur | Selon la politique de la station |

Un test de restauration tous les trois mois suffit à garantir que la procédure
fonctionne encore (les chemins, les outils et les accès changent plus souvent
qu'on ne le croit).
