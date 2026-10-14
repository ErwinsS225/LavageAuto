# TODO — plan d'implémentation et de correction

Liste de suivi tirée de [`RAPPORT-ANALYSE.md`](RAPPORT-ANALYSE.md) (§13 plan
priorisé, §11 dette, annexe B inventaire des routes).

**État au 14 octobre 2026** — le P0 est traité, le P1 aussi, et le parcours
d'inscription est livré de bout en bout (lots 0 → 9 de `TODO-ONBOARDING.md` :
inscription, mise en route en treize étapes, activation, tableau de bord).
Ce qui reste est listé par ordre de priorité, avec le fichier concerné, la
preuve de vérification et l'effort estimé.

Convention : `[x]` fait et vérifié · `[~]` partiel · `[ ]` à faire ·
**[vous]** = action impossible depuis le code (compte, console d'un
fournisseur, décision produit).

---

> **Onboarding** : la feuille de route dédiée est [`TODO-ONBOARDING.md`](TODO-ONBOARDING.md)
> (audit, décisions structurantes, 10 lots de travail, tests exigés, critères d'acceptation,
> journal de progression). **Les dix lots sont livrés et vérifiés** (14 octobre 2026) ; ce
> fichier-ci reste la liste des correctifs généraux, et le relevé final de l'onboarding
> (ce qui est opérationnel, ce qui a été testé, ce qui reste ouvert) est au §13 de la feuille
> de route. Le mode d'emploi du parcours lui-même est [`docs/onboarding.md`](docs/onboarding.md).

## 0. Comment vérifier ce qui est fait

```bash
# Serveur : compilation, formatage, analyse statique, tests
cd backend && go build ./... && gofmt -l . && go vet ./... && go test ./...

# Application web : typecheck + build de production
npm ci && npm run build

# Configuration : ce qui est renseigné, ce qui manque, ce qui est incohérent
bash scripts/verifications/verifier-env.sh dev      # ou prod

# Cohérence des routes et des écrans (Node seul, aucun serveur)
node scripts/verifications/coherence-routes.mjs

# Rendu des 18 écrans et états : attrape l'« écran noir » (erreur au rendu React)
node scripts/verifications/rendu-pages.mjs

# Toutes les sondes d'acceptation d'un coup (API démarrée) — voir make verifier-acces
python3 scripts/verifications/sonde-api.py           # cloisonnement, rôles, contrat
python3 scripts/verifications/sonde-onboarding.py    # inscription → 13 étapes → activation
python3 scripts/verifications/sonde-tarifs.py        # catalogue, grille, sélection des prix
python3 scripts/verifications/sonde-invitations.py   # rejoindre une entreprise
python3 scripts/verifications/sonde-vehicules.py     # catégories, usages, combinaisons
python3 scripts/verifications/sonde-lot7.py          # équipe, matériel, stock, commandes
python3 scripts/verifications/sonde-lot8.py          # récapitulatif, activation, tableau de bord
python3 scripts/verifications/sonde-metier.py        # caisse, remises, soldes, plafonds (démo)
python3 scripts/verifications/sonde-fiche-payee.py   # fiche encaissée verrouillée (démo)
python3 scripts/verifications/sonde-suspension.py    # entreprise suspendue coupée
```

Relevé du 14 octobre 2026, sur base peuplée : **366 contrôles, 0 échec** (10 sondes), le
contrôle de cohérence des routes est muet et les **18 écrans se rendent** sans erreur. Sur une
base **vierge** (recréée, migrée `0001` → `0012`), les huit sondes qui ne dépendent pas des
données de démonstration donnent **290 contrôles au vert**, et les deux sondes de démonstration
(**57 contrôles**) tournent sur une base neuve créée avec `SEED_DEMO=true`. Marche à suivre :
[`docs/onboarding.md`](docs/onboarding.md) §10. La même chose tourne en intégration continue
(`.github/workflows/ci.yml` : tâche `backend` avec PostgreSQL de service pour les 60 tests de
bout en bout, tâche `sondes` pour les 10 sondes).

---

## 1. P0 — reste à faire, hors code **[vous]**

- [ ] **Révoquer et renouveler tous les secrets publiés.** Le fichier
      `.env.backup.20261013-162500` a été versionné sur un dépôt **public** :
      considérez comme compromis, et faites tourner :
      - la chaîne de connexion Neon (mot de passe de la base) ;
      - les clés S3 / stockage (`nak_live_*`, `nsk_live_*`) ;
      - la clé secrète Clerk (`sk_test_*`) et le secret de signature du
        webhook (`whsec_*`).
      Effort : 1 h. Aucune correction de code ne remplace cette étape.
- [x] **Renseigner les variables d'environnement** : `.env` est en place et
      renseigné (Neon, Clerk, webhook, clé publique), ignoré par git, et
      `.env.example` déclare exactement les mêmes clés. Contrôle :
      `scripts/verifications/verifier-env.sh` (n'affiche jamais un secret).
- [ ] **Renouveler les clés Clerk** : les valeurs fournies sont **identiques**
      à celles du fichier publié (comparaison automatique) — elles doivent être
      remplacées dans le tableau de bord Clerk, puis réinjectées dans `.env`.
      Ce sont des clés de **test** (`pk_test_` / `sk_test_`) : la production
      exige les clés `live`.
- [ ] **Renseigner `APP_URL` et `CLERK_AUTHORIZED_PARTIES`** avec le domaine
      réel : sans elles, un jeton Clerk dont le claim `azp` ne correspond à
      aucune origine connue est refusé.
- [ ] **Trancher `SEED_DEMO`** : à `true` avec la base Neon réelle, un
      démarrage sur une base vide y créerait les données de démonstration.
      Passer à `false` si cette base est celle de production.
- [ ] **Passer le dépôt en privé** (`ErwinsS225/LavageAuto` est public).
      Effort : 2 min.
- [ ] **Purger le fichier de l'historique** (`git filter-repo` ou BFG) puis
      forcer la réécriture : le retirer du suivi ne l'efface pas des commits
      déjà publiés. À faire **après** la rotation — et en prévenant toute
      personne ayant cloné le dépôt. Effort : 30 min.
- [ ] **Vérifier les journaux d'accès** des fournisseurs concernés (Neon, S3,
      Clerk) pour repérer une utilisation anormale pendant la période
      d'exposition. Effort : 30 min.
- [ ] Décider du sort de Clerk : si l'authentification Clerk n'est pas
      utilisée en production, retirer `CLERK_SECRET_KEY` de l'environnement
      (le serveur désactive alors proprement le chemin Clerk). Effort : 5 min.

## 2. P0 et P1 — faits (pour mémoire, rien à reprendre)

- [x] `.gitignore` complété et artefacts retirés du suivi : `dist/`,
      `backend/backend` (17,6 Mo), `.env.backup.*`, fichier vide `=` retiré.
      Commit `7ce2122`.
- [x] **`npm run build` passe** : 9 erreurs TypeScript corrigées en supprimant
      le code mort (6 écrans, doublon `hooks/useStation`), en fusionnant le
      contexte de station et en exposant `role_label`. Commit `d5317f2`.
- [x] **CI minimale** : compilation Go (build, gofmt, vet, tests), build front,
      sondes d'acceptation sur PostgreSQL 17. Commit `5a8da62`.
- [x] **Fiches encaissées verrouillées** : plus de remise rétroactive ni de
      retour en atelier ; champs descriptifs toujours modifiables. Commit
      `5f7f805`.
- [x] **Permissions par rôle sur les 72 routes** (`requirePermission`, source
      unique `rolePermissions`) et `/platform/*` réservé au superadmin.
      Commit `1a2b5ef`.
- [x] **Cloisonnement multi-entreprises** : toute lecture/écriture filtre
      `organization_id` (404 pour une autre entreprise) et le périmètre de
      station est contrôlé (403). Commit `1a2b5ef`.
- [x] **Contrats d'API réparés** : `/stations` (tableau), `/employees/options`,
      `/customers/options`, `/cash/current`, `/reports/overview`,
      `/commissions`, `/washer/*`, `/reports/export` (CSV).
- [x] **Caisse et remises** : espèces ⇒ session ouverte, acompte qui laisse la
      fiche ouverte, surpaiement refusé, annulation motivée qui décrémente les
      compteurs, plafond de remise par rôle, dépenses liées à la sortie de
      caisse. Commit `1a2b5ef`.
- [x] **Authentification** : sessions stockées hachées (SHA-256) et purgées,
      limiteur sur les échecs de connexion (IP + e-mail) et sur les
      inscriptions, politique de mot de passe, `SameSite`/CORS durcis,
      signature Clerk vérifiée (`azp` inclus). Commits `1a2b1f5`… `0b41ff8`.
- [x] **Limites d'abonnement et suspension** appliquées côté serveur
      (stations, comptes, lavages du mois, essai expiré ; entreprise suspendue
      ⇒ 403 `organization_suspended` immédiat, réactivation sans reconnexion).
- [x] **Notifications** : un seul `ToastProvider` à la racine (les messages
      n'étaient plus perdus silencieusement). Favicon livré et servi.
- [x] **Les écrans s'affichent** : `useClerk()` était appelé sur la page de
      connexion alors que Clerk n'est pas configuré, ce qui vidait la page
      (écran noir) ; l'espace plateforme suspendait de la même façon faute de
      limite de chargement. Un garde-fou d'affichage (`ErrorBoundary`) montre
      désormais l'erreur au lieu du vide, et `rendu-pages.mjs` (en CI) vérifie
      les dix écrans.
- [x] **Interface refondue** : design system complet (tokens clair/sombre, ombres en
      couches, flou de verre, rayons), bibliothèque de composants découpée par
      famille et animée avec **Framer Motion** (`framer-motion@12`) —
      transitions d'écran, tiroir mobile, menus, modales, notifications,
      sélecteurs segmentés, compteurs animés ; graphiques Recharts en dégradés
      avec infobulles en verre ; repères écrits dans `docs/interface.md`.
- [x] **Classes d'interface orphelines** : `stat-value`, `stat-label`, `nav-link`,
      `badge-*` et `table` étaient utilisés dans les écrans mais définis nulle
      part (Tailwind ne génère rien pour une classe inconnue, et rien ne le
      signalait). Elles sont définies dans `src/index.css` et
      `scripts/verifications/classes-ui.mjs` (branché sur `make check`) refuse
      désormais toute classe maison employée sans définition.
- [x] **Marque unifiée** : les écrans publics affichaient « LavPro » dans un
      violet (`#47253F`) qui n'existait nulle part ailleurs ; tout passe par
      `src/components/Brand.tsx`.
- [x] **Filtre de statut des lavages** : `Select` ignorait sa prop
      `placeholder`, si bien que le filtre vide affichait « En attente ».
- [x] **Réglages de station unifiés** : un seul littéral JSON pour tous les
      chemins de création (`backend/stations_settings.go`) ; l'inscription
      oubliait `require_open_cash_session`, laissant encaisser des espèces sans
      caisse ouverte. Migration `0003_…` pour les stations déjà créées.
- [x] **Sauvegarde nocturne qui ne sauvegardait rien** :
      `scripts/sauvegarde-quotidienne.sh` n'acceptait que `DATABASE_URL` alors que
      le serveur (et le gabarit `.env`) utilisent `NEON_DATABASE_URL` : sur une
      installation Neon, `make backup` et la tâche launchd échouaient chaque soir
      en silence. Même résolution que le serveur dans `sauvegarde-quotidienne.sh`,
      `recette-jour1.sh`, `verifier-production.sh` et `installer-station.sh`.
- [x] **Adresse affichée fausse** : `scripts/adresse-locale.sh` annonçait le port
      8080 sans jamais lire `.env`, alors que l'application écoute 3000 (gabarit,
      `start.sh`) : les téléphones de la station recevaient une adresse morte. Le
      script lit maintenant `.env` et une variable passée à la main reste
      prioritaire.
- [x] **`make prod-check` refusait une configuration valide** : le script annonçait
      lire `.env` mais ne le lisait pas, et exigeait `DATABASE_URL` — la commande
      documentée s'arrêtait donc sur « DATABASE_URL n'est pas défini ».
- [x] **Lecture de `.env` robuste** : `scripts/charger-env.sh` remplace les
      `set -a; . ./.env` (qui exécutaient le corps de la clé publique PEM comme des
      commandes et écrasaient les variables passées à la main). Mêmes règles que le
      serveur : environnement prioritaire, valeurs multi-lignes concaténées.
- [x] **Ports du développement alignés** : `make api` écoutait 3000 comme le
      serveur Vite (qui relaie `/api` vers 8080) ; il écoute 8080, comme le
      documente le README et comme le fait `scripts/dev.sh`.
- [x] **Références mortes du README** : deux images absentes (`docs/captures/…`,
      `docs/apercu-ecrans.png`), un chemin `frontend/public/images/…` inexistant,
      des chemins de compilation obsolètes (`frontend/dist`, `backend/cmd/api`) et
      « migrations 0001 → 0005 » alors qu'il y en a trois.
- [x] **Documents d'exploitation écrits** : `docs/api.md`,
      `docs/guide-utilisation.md`, `docs/securite-acces.md`,
      `docs/sauvegarde-restauration.md`, `docs/deploiement-https.md`,
      `docs/installation-station.md` rejoignent `docs/interface.md` — les six
      renvois du README mènent désormais à des fichiers réels.
- [x] **Montant d'encaissement obligatoire** : `POST /wash-orders/{id}/payments`
      refuse désormais un montant nul ou négatif (400), comme
      `POST /cash/payments` et comme l'écran d'encaissement.
- [x] **Routes et écrans cohérents** : le menu de la plateforme pointait vers
      `/admin/*` alors que les routes sont `/administration/*` (chaque clic
      tombait sur la page introuvable, qui renvoyait elle-même vers `/admin`) ;
      le lien « Mot de passe oublié ? » n'avait pas de page ; « Mes lavages »
      s'affichait pour tous les rôles. Un contrôle automatique
      (`scripts/verifications/coherence-routes.mjs`, branché en CI) refuse
      désormais un lien sans route, une route sans lien et une permission
      d'écran absente de la matrice des rôles.

## 3. P1 — reste à faire (données et exploitation)

- [x] **Migrations numérotées + table `schema_migrations`.** `backend/schema.sql`
      est devenu `backend/migrations/0001_schema_initial.sql` ; le moteur
      (`backend/migrations.go`) applique les fichiers non joués, un par
      transaction, sous verrou consultatif, et les enregistre. Validé sur la
      base existante (70 fiches, 70 lignes) : `0001` et `0002` enregistrées,
      second passage = rien à rejouer.
- [x] **`organization_id` sur `wash_order_items`** (`0002_…`) : colonne
      remplie depuis la fiche parente puis rendue `NOT NULL` ; 70/70 lignes
      rattachées, 0 orpheline, index de rapport ajouté.
- [x] **Seed et README alignés.** Le `Makefile` fournit `seed`, `smoke`,
      `recette` et les autres cibles annoncées (voir `make help`).
- [x] **Sort des scripts réglé** : `Makefile` (racine) + `compose.yaml`
      (PostgreSQL 17 pour `make db`) fournis, `make doctor` signale ce qui
      manque sur la machine.

## 4. P2 — qualité et tests

- [x] **Tests Go de bout en bout** (`backend/e2e_test.go` puis `e2e_lot7_test.go`,
      `e2e_lot8_test.go`, `e2e_lot9_test.go`) : **65 tests** au total, rejouant
      les mêmes scénarios que les sondes Python, sans serveur séparé — chaque
      test crée son schéma PostgreSQL (`CREATE SCHEMA`), applique les
      migrations, démarre l'API par `httptest` et supprime le schéma à la fin
      (aucune donnée de la base visée n'est lue ni modifiée). Plus **5 tests**
      de vérification des jetons Clerk (`backend/clerk_test.go`). Lancement :
      `make test-e2e` (base de `make db` par défaut, `TEST_DSN` pour une autre).
      Sans `DATABASE_URL`, ils sont ignorés plutôt qu'en échec. Les **4 tests du lot 9**
      (`e2e_lot9_test.go`) portent sur la sécurité : 45 tentatives d'accès d'une entreprise sur
      les données d'une autre (lecture, écriture, désactivation), avec contrôle que chaque route
      visée existe bien — un `404` de route inconnue ne doit pas faire passer un test d'isolation
      pour un test réussi —, ce qu'un gérant ne peut pas faire, ce qu'un caissier ne peut pas
      tarifer, et les plafonds de la formule Essai (1 station, 3 comptes).
      Ces tests ont trouvé deux incohérences corrigées dans la foulée :
      le réglage « caisse ouverte obligatoire » manquait sur la station créée à
      l'inscription (32 stations sur 33 de la base de développement étaient
      concernées — corrigé, plus `migrations/0003_…` qui complète les
      stations existantes sans écraser un réglage explicite), et
      `POST /wash-orders/{id}/payments` acceptait un montant nul ou absent en
      encaissant la totalité du reste dû, contrairement à `/cash/payments` et
      à l'écran d'encaissement (désormais refusé en 400).
- [x] **Test de rendu des pages** : `scripts/verifications/rendu-pages.mjs`
      (10 écrans en jsdom) et `scripts/verifications/coherence-routes.mjs`,
      branchés sur `make check`, `make smoke` et `make rendu`.
- [ ] **`scripts/recette-jour1.sh` et `frontend/tools/rendu/*.mjs` sont périmés** (constaté le
      14 octobre 2026) : ils décrivent le produit d'avant les lots 5 à 8 — un catalogue
      « déjà tarifé » à l'inscription, puis un lavage enregistré sans activation. Le produit
      livré fait l'inverse (aucun prix inventé ; `409 onboarding_incomplete` tant que la station
      n'est pas activée), donc ces scénarios échouent au troisième point. Un avertissement a été
      posé en tête du script, des trois scénarios et des cibles `make recette` / `make rendu`, et
      le README le dit. Reste à les **réécrire pour les treize étapes** (ou à les retirer).
      Effort : ½ j.
- [ ] **Découper `backend/handlers_misc.go`** (281 lignes, un fourre-tout) par
      domaine, et regrouper la sérialisation JSON (aujourd'hui dispersée dans
      les handlers, `backend/http_json.go` n'en couvre qu'une partie).
      Effort : ½ j.
- [x] **`README.md` à l'échelle du dépôt** : l'arborescence et les choix
      techniques décrivent le code réel (`backend/*.go`, `migrations/`, `src/`,
      Tailwind CSS 4, Framer Motion), les six documents promis existent dans
      `docs/`, et les renvois vers des fichiers absents ont été retirés.

## 5. P2/P3 — produit

- [ ] **Export des commissions** couvrant la période et le barème réellement
      appliqué (aujourd'hui le calcul est juste, l'export est à compléter).
      Effort : ½ j.
- [ ] **Page « reste à payer »** : liste des fiches non soldées, par station et
      par ancienneté (le champ `due_xof` existe déjà et est juste).
      Effort : ½ j.
- [ ] **Impression de reçu** (annoncée comme limite assumée dans le rapport) :
      ticket 58 mm ou A4 pour le client. Effort : 1 j.
- [ ] **Rapprochement Mobile Money** : import ou saisie d'un relevé opérateur
      et pointage avec les encaissements `mobile_money`. Effort : 2 j.
- [ ] Compléter les tests des limites de formule (le plan d'essai est vérifié,
      les paliers payants créés en base ne le sont pas). Effort : 0,5 j.

---

## Décisions en attente (à trancher, pas à coder)

1. `CLERK_AUTHORIZED_PARTIES` : à alimenter avec le domaine réel dès qu'il
   existe (sans cette liste, seules les origines CORS et `APP_URL` sont
   acceptées pour le claim `azp`).
2. `SESSION_COOKIE_SAMESITE=none` : à n'activer que si le front est servi sur
   un autre domaine que l'API (impose `SESSION_COOKIE_SECURE=true`).
3. Sort du dossier `legacy/` (`server.express.ts.txt`, 1 025 lignes) : le
   garder comme référence historique ou le supprimer.
4. Sort de `frontend/tools/rendu/` (outil de captures) et de `metadata.json`.

## Journal des commits de cette passe

| Commit | Contenu |
| --- | --- |
| `8607c4b` | Rapport d'analyse (`RAPPORT-ANALYSE.md`) |
| `1a2b5ef` | Permissions par rôle, cloisonnement, caisse, contrats d'API |
| `0b41ff8` | Inscription robuste (slug unique), statut laveur, limites réglables |
| `d5317f2` | Front : build réparé, un seul fournisseur par contexte |
| `7ce2122` | Artefacts et secrets retirés du suivi |
| `5f7f805` | Fiche encaissée verrouillée + formatage gofmt |
| `5a8da62` | CI minimale + sondes d'acceptation |
| `9ffc1ae` | Liste de suivi des correctifs, favicon réel |
| `9b3c6ae` | Fichiers publics servis, sonde métier rejouable |
| `7cf78ec` | Chaque écran routé vers une adresse existante |
| `402ff10` | Compte plateforme annoncé par le README créé |
| `0bf3c49` | Écran noir de `/connexion` corrigé |
| `128cfd2` | `.env`, gabarit complet et contrôle avant démarrage |
| `d4a1582` | Outil de saisie des variables d'environnement |
| `00c7624` | Configuration réelle installée, clé publique Clerk exploitée |
| `82c3504` | Migrations numérotées, lignes de fiche rattachées à leur entreprise |
| `e0acc6d` | Lot 1 — créer ou rejoindre un espace (invitations) |
| `182f6d9` | Lots 2 et 3 — identité de l'entreprise, station et horaires |
| `4439f35` | Lot 4 — catégories de véhicules et usages commerciaux |
| `8df5831` | Lot 5 — prestations, options et grille tarifaire |
| `86c4b1b` | Lots 0 à 6 — état consolidé (mise en route, tarification, caisse, reçus) |
| `273f3e9` | Correctif d'aperçu : session reconnue hors cookie tiers, déconnexion réelle |
| `aca1880` | Lot 7 — équipe, matériel, stock et règles de commande |
| `423d568` | Lot 8 — récapitulatif, activation transactionnelle et tableau de bord alimenté |
| _à venir_ | Lot 9 — tests d'isolation, documentation, relevé final |

Le détail de l'onboarding (13 étapes, prérequis d'activation, référentiels, tarification,
invitations) est dans [`docs/onboarding.md`](docs/onboarding.md) ; le journal de mise en œuvre,
lot par lot, dans [`TODO-ONBOARDING.md`](TODO-ONBOARDING.md) §11.

Rien n'est encore poussé sur `origin` : les commits sont locaux, à pousser
après revue.
