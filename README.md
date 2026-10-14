# WashGuard

**Gestion simple des stations de lavage automobile à Abidjan.**
WashGuard remplace le cahier papier d'une station : lavages, encaissements, dépenses,
caisse et rapports, accessibles depuis un ordinateur, une tablette ou un téléphone.

C'est un logiciel **SaaS multi-entreprises** : chaque entreprise cliente dispose de son
espace isolé (ses stations, ses employés, ses clients, ses lavages). Aucune donnée n'est
partagée entre deux entreprises.

> WashGuard n'est **pas** un logiciel de comptabilité, ni une banque, ni un outil de
> conformité. Les montants affichés sont ceux que votre équipe saisit : ils servent à
> piloter la station au quotidien.

---

## 1. Ce que fait l'application

La racine du site (`/`) présente WashGuard aux visiteurs : à quoi sert l'application, aperçu des
écrans, formules et tarifs (lus depuis la base, donc jamais désynchronisés de la facturation),
questions fréquentes. Une personne déjà identifiée est conduite directement à son espace.

Ce qui alimente la vitrine est réel : les captures d'écran viennent du rendu de l'application
(`make rendu`), les tarifs de la base, et il n'y a **pas de témoignages** — des avis inventés
seraient pires que pas d'avis du tout. Ils seront ajoutés quand de vraies stations en donneront.

### Connexion et inscription

Ces deux écrans suivent la même mise en page (`AuthLayout`) : le formulaire sur une surface
claire et centrée, un panneau à droite occupé par une photo de lavage en cours
(`public/images/lav-auto.jpeg`). Un dégradé sombre est posé sur la photo : le badge
« Espace station », la phrase d'accroche et les trois repères restent lisibles, y compris sur une
photo très claire. Sur téléphone, le panneau disparaît : seul le formulaire reste, plein largeur.

**Changer la photo** : remplacez ce seul fichier (même nom), puis `make build` lorsque
l'application est servie par l'API Go — rien à modifier dans le code. En développement
(`make web`), le changement est immédiat. L'API demande aux navigateurs de revalider ces fichiers
(`Cache-Control: no-cache`) : la nouvelle photo s'affiche sans vider le cache à la main.

Le formulaire d'inscription est groupé en deux temps — *votre entreprise et sa station*, puis
*votre compte de propriétaire* — pour rester lisible. Il demande quatre choses au serveur : le nom
de l'entreprise, le nom de la station, le téléphone et la ville ; le serveur crée **une seule
fois** l'entreprise, sa station, le compte propriétaire et l'abonnement d'essai, puis ouvre
l'écran **Mise en route**.

Ce qui suit n'est pas un formulaire de bienvenue : c'est un parcours de treize étapes dont l'état
est **constaté par le serveur** sur les données réellement enregistrées (aucune étape n'est cochée
par le navigateur), dont huit obligatoires — identité, horaires, catégories de véhicules,
catalogue, grille tarifaire, moyens de paiement. Tant que l'activation n'a pas eu lieu, les
écritures de métier répondent `409 onboarding_incomplete` avec la liste de ce qui manque. **Aucun
prix n'est inventé et aucune donnée d'exemple n'est ajoutée** : la station ouvre quand vous avez
choisi vos tarifs. Parcours complet, prérequis, refus et vérifications :
[docs/onboarding.md](docs/onboarding.md).

Un garde-fou reste en place sur la session : un visiteur sans cookie ne déclenche qu'**un seul**
appel à `/auth/me`. Il vient d'un défaut réel — le 401 de `/auth/me` invalidait tout le cache, ce
qui relançait `/auth/me`, et ainsi de suite (2 300 requêtes en cinq secondes mesurées dans un vrai
navigateur).

Trois choses ne figurent **pas** sur ces pages, faute de les tenir :
- « Continuer avec Google / Facebook » : aucune connexion tierce n'existe ;
- « Mot de passe oublié » par e-mail : aucun e-mail n'est envoyé. Le texte renvoie vers la
  seule voie réelle — le propriétaire réinitialise le mot de passe depuis *Employés* ;
- « Se souvenir de moi » : la session dure 30 jours, l'option n'aurait aucun effet.

| Module | Ce qu'il permet |
| --- | --- |
| **Mise en route** | Parcours d'ouverture : l'entreprise, la station, les prestations et leurs prix, puis l'activation. Chaque étape est constatée par le serveur sur vos données — aucun bouton ne déclare une étape « faite ». Tant que la station n'est pas activée, l'application refuse les fiches, les encaissements, la caisse et les dépenses (409 `onboarding_incomplete`) : elle n'ouvre pas l'exploitation avec des tarifs que vous n'avez pas choisis. |
| **Tableau de bord** | Chiffre d'affaires du jour, lavages, encaissements, dépenses, état de la caisse, alertes, dernières opérations. Tous les chiffres sont calculés par le serveur à partir de la base. |
| **Lavages** | Saisie rapide d'une fiche : prestations, véhicule, plaque (facultative), employé, remise autorisée, client (facultatif). Statuts *en attente / en cours / terminé / annulé*, encaissement total ou partiel, reste à payer. |
| **Prestations** | Catalogue propre à chaque station (lavage extérieur, intérieur, complet, sièges, moteur…). **Modifier un tarif ne change jamais les lavages déjà enregistrés.** |
| **Clients** | Fiche facultative (nom, téléphone, plaque, notes), historique des lavages, recherche rapide. Un lavage peut être encaissé sans créer de client. |
| **Employés** | Comptes et rôles (Propriétaire, Gérant, Caissier), activation/désactivation, réinitialisation de mot de passe, activité enregistrée par employé. |
| **Caisse** | Ouverture avec fond de caisse, encaissements (espèces, Mobile Money, carte, virement), dépenses, clôture avec **comptage** et écart théorique/compté, réouverture motivée, historique. Règle par défaut : un encaissement **en espèces** exige une caisse ouverte (désactivable station par station dans les paramètres) ; le Mobile Money peut être enregistré à tout moment. |
| **Dépenses** | Montant, catégorie, motif, fournisseur, date, moyen de paiement. Rattachement automatique à la caisse ouverte, annulation avec motif (jamais de suppression). |
| **Rapports** | Jour / semaine / mois : lavages, chiffre d'affaires, encaissements, dépenses, résultat simplifié, prestations les plus vendues, activité par employé, répartition par moyen de paiement, historique de caisse, meilleurs clients, **export CSV**. |
| **Commissions** | Ce que la station doit à chaque laveur : lavages **terminés** de la période, barème appliqué (barème de la station ou exception posée sur un compte), montant dû, totaux et **export CSV pour la paie**. Objectif de chiffre d'affaires du mois suivi sur le tableau de bord. Réservé au propriétaire et au gérant. |
| **Paramètres** | Identité de la station, adresse, ville, horaires d'ouverture, devise (FCFA), logo, règles de caisse et remise maximale du caissier, barème des commissions et objectif du mois, entreprise, journal des opérations sensibles. |
| **Abonnement** | Formule en cours (essai / Standard / Premium), consommation par rapport aux limites, comparaison des formules, demande de changement. |
| **Espace plateforme** | Réservé au support WashGuard : entreprises clientes, formules, abonnements, suspension d'une entreprise, journal de la plateforme. |

Toute l'interface est en **français**, avec les montants en **FCFA** (`12 500 FCFA`) et les
dates au format **jj/mm/aaaa**. Thème sombre par défaut, thème clair disponible.

---

## 2. Démarrage sur macOS (environ 10 minutes)

### Prérequis

| Outil | Version | Installation |
| --- | --- | --- |
| Homebrew | — | <https://brew.sh> |
| Docker Desktop | — | `brew install --cask docker` (ou <https://www.docker.com/products/docker-desktop/>) |
| Go | 1.23 ou plus récent | `brew install go` |
| Node.js | 20 ou plus récent | `brew install node` |
| make | (fourni avec Xcode CLI) | `xcode-select --install` |

Vérifiez l'installation :

```bash
docker --version && go version && node --version && make --version
```

### Installation

```bash
# 1. Récupérer le projet puis ENTRER dans son dossier.
#    Toutes les commandes `make` se lancent depuis la racine du projet, pas depuis ~
git clone https://github.com/ErwinsS225/Station_auto.git WashGuard
cd WashGuard

# 2. Préparer la configuration
cp .env.example .env

# 2 bis. Vérifier la machine (Docker, Go, Node, ports libres)
make doctor

# 3. Démarrer PostgreSQL
make db

# 4. Installer les dépendances du frontend
make install

# 5. Créer le schéma et les données de démonstration
make seed
```

`make seed` applique les migrations, insère une entreprise de démonstration pour essayer
l'application immédiatement, **puis rend la main** (la commande est idempotente : relancée, elle
ne fait rien de plus). Vous pouvez donc enchaîner avec les deux terminaux ci-dessous.

> **Erreur « make: No rule to make target 'db' »** : le terminal n'est pas dans le dossier du
> projet. Faites `cd WashGuard` (ou le nom que vous avez donné au dossier) puis recommencez.
> `make doctor` indique aussi ce qui manque sur la machine.

### Pour une utilisation réelle à la station

Si vous installez WashGuard sur l'ordinateur du bureau pour l'utiliser tous les
jours (accès depuis les téléphones de la station, démarrage automatique,
sauvegarde quotidienne), le guide dédié est plus adapté que ce README :

```bash
make verifier-station   # vérifie ce qui manque, ne modifie rien
make station            # installation guidée (macOS)
make adresse            # affiche l'adresse à ouvrir depuis les téléphones
```

→ [`docs/installation-station.md`](docs/installation-station.md) — installation, sauvegardes, dépannage
→ [`docs/guide-utilisation.md`](docs/guide-utilisation.md) — la journée type, à lire par l'équipe

### Lancer l'application (2 terminaux)

```bash
# Terminal 1 — API Go (port 8080)
make api

# Terminal 2 — interface React (port 5173)
make web
```

Ouvrez **<http://localhost:5173>** et connectez-vous :

| Compte | Identifiant | Mot de passe | Rôle |
| --- | --- | --- | --- |
| Démonstration | `demo@washguard.ci` | `demo1234` | Propriétaire |
| Gérant | `gerant@washguard.ci` | `demo1234` | Gérant |
| Caissier | `caisse@washguard.ci` | `demo1234` | Caissier |
| Support plateforme | `admin@washguard.app` | `admin1234` | Administration WashGuard |

> Les comptes de démonstration n'existent que si vous avez lancé `make seed`.
> Créez votre propre entreprise depuis **/inscription** pour tester le vrai parcours client.

### Raccourcis utiles

```bash
make help      # liste toutes les commandes
make check     # compilation Go + TypeScript + build du frontend
make test      # tests unitaires Go + types du front
make test-e2e  # tests de bout en bout sur une base jetable (make db)
make smoke     # rendu réel des pages React (API démarrée)
make backup    # sauvegarde de la base dans backups/
make db-stop   # arrêter PostgreSQL (les données sont conservées)
make db-reset  # repartir d'une base vide (destructif)
```

---

## 3. Architecture

```
.
├── backend/                    API Go (aucun Node.js dans le serveur)
│   ├── main.go                 point d'entrée : configuration, routes, serveur HTTP
│   ├── migrations.go           moteur de migrations (table `schema_migrations`)
│   ├── migrations/             migrations numérotées (0001_schema_initial.sql, 0002_…, 0003_…)
│   ├── handlers_*.go           un fichier par domaine : lavages, caisse, employés,
│   │                           clients, prestations, dépenses, rapports, plateforme…
│   ├── permissions.go          rôles, permissions et périmètre de station
│   ├── stores / models         requêtes SQL explicites (pgx), types partagés
│   └── *_test.go               tests Go : 5 sur les jetons Clerk, 60 de bout en bout
├── src/                        React + TypeScript + Vite + Tailwind CSS 4
│   ├── components/             ui/ (design system), auth/, Toaster, Brand, charts
│   ├── layouts/                AppLayout (application), AdminLayout (plateforme), AuthLayout
│   ├── pages/                  un fichier par écran
│   ├── features/               appels API par domaine (React Query)
│   ├── hooks/ lib/ types/      session, station, thème, formatage, types d'API
│   └── index.css               tokens du thème, classes d'interface, animations
├── docs/                       onboarding, guide d'utilisation, exploitation, API, interface
├── scripts/verifications/      sondes d'acceptation, rendu des écrans, cohérence
├── compose.yaml                PostgreSQL 17 pour le développement (make db)
├── Makefile                    commandes de développement
└── .env.example                modèle de configuration
```

**Choix techniques**

- **Go 1.27**, routeur de la bibliothèque standard (`net/http` : `mux.HandleFunc("GET /api/v1/…")`,
  verbes et paramètres de chemin depuis Go 1.22) et [pgx](https://github.com/jackc/pgx) comme pilote
  PostgreSQL. **SQL explicite** — pas d'ORM, pas de requêtes générées à l'exécution. Les jetons Clerk
  sont vérifiés avec `clerk-sdk-go` et `golang-jwt`; les webhooks, avec `svix`.
- **PostgreSQL 17** (image `postgres:17-alpine` de `compose.yaml`), schéma versionné par migrations
  (table `schema_migrations`, `0001` → `0012`).
- **React 19 + TypeScript strict + Vite + Tailwind CSS 4 + React Router + TanStack Query**,
  graphiques [Recharts](https://recharts.org), animations [Framer Motion](https://motion.dev).
- **Design system maison** : `src/components/ui/` (primitives, surfaces, données, formulaires,
  surépaisseurs, états, blocs animés) et les classes sémantiques de `src/index.css`
  (`card`, `badge`, `table`, `nav-link`, `stat-value`…). Ni shadcn/ui, ni bibliothèque de
  composants : les jetons de couleur, les ombres en couches et les durées d'animation sont
  définis une fois, puis réutilisés par tous les écrans. Repères de conception :
  [docs/interface.md](docs/interface.md).
- Aucune dépendance payante : le projet tourne entièrement avec des outils libres.

### Isolation des entreprises

L'isolation n'est pas seulement visuelle : **chaque requête SQL filtre sur
`organization_id`** extrait de la session côté serveur (jamais d'un paramètre envoyé par le
client). Les rôles et permissions sont vérifiés sur chaque route, et les limites de la
formule (stations, comptes, lavages du mois, options) sont contrôlées par le serveur.

S'y ajoute le **périmètre de station** : un caissier (ou un gérant rattaché à une station) ne
voit et ne modifie que la sienne, y compris par identifiant exact. La règle est écrite une
seule fois (`permissions.StationAutorisee`, `permissions.StationRattachee`) et appelée par
tous les modules — lavages, rapports, dépenses, clients, employés, caisse, prestations,
encaissements. Le propriétaire, lui, garde l'entreprise entière : c'est nécessaire pour ouvrir
une seconde station.

Les tests `TestIsolationEntreEntreprises`, `TestIsolationDesEntitesDesLots1A8` (45 tentatives
d'accès d'une entreprise sur les données d'une autre, lecture et écriture, plus le contrôle que
chaque route visée existe bien), `TestPermissionsParRole`, `TestManagerNePeutPasUsurperLeProprietaire`,
`TestCaissierSansPermissionDePrix`, `TestLimitesDeFormuleAppliquees` et `TestPerimetreDuneStation`
le vérifient de bout en bout (`make test-e2e`) ; `make verifier-acces` le vérifie sur
l'application qui tourne.
Détail des règles et de leurs limites : [docs/securite-acces.md](docs/securite-acces.md).

---

## 4. Rôles et permissions

| Permission (extrait) | Propriétaire | Gérant | Caissier |
| --- | --- | --- | --- |
| Voir le tableau de bord, les rapports | ✅ | ✅ | ✅ (tableau de bord) |
| Voir les commissions des laveurs (état de paie) | ✅ | ✅ | ❌ |
| Enregistrer un lavage, un encaissement | ✅ | ✅ | ✅ |
| Annuler un lavage terminé ou déjà payé | ✅ | ✅ | ❌ |
| Corriger une fiche en attente non encaissée (motif obligatoire) | ✅ | ✅ | ✅ dans sa station |
| Accorder une remise au-delà du plafond | ✅ | ✅ | ❌ (plafond de la station) |
| Gérer prestations, clients, employés | ✅ | ✅ | partiel (créer un client) |
| Ouvrir / clôturer la caisse | ✅ | ✅ | ✅ |
| Rouvrir une caisse clôturée | ✅ | ✅ | ❌ |
| Modifier les paramètres et l'abonnement | ✅ | ✅ (paramètres) | ❌ |
| Espace plateforme (`/administration`) | ❌ | ❌ | ❌ |

Le **caissier ne peut pas supprimer un encaissement** : il l'annule avec un motif, et la
ligne reste visible dans l'historique et les rapports. Une session de caisse clôturée est
verrouillée ; sa réouverture exige un motif et laisse une trace.

---

## 5. Tests et vérifications

```bash
# API : 65 tests Go (5 sur la vérification des jetons Clerk, 60 de bout en bout
# sur une vraie base PostgreSQL), base jetable : chaque test crée sa propre
# entreprise, la supprime à la fin et ne touche à rien d'autre
make test-e2e

# Formatage, compilation Go, types et build du frontend, routes, écrans,
# classes d'interface et cohérence de la configuration
make check

# Interface : 18 écrans et états rendus hors navigateur, avec une session connue
# d'avance et le réseau coupé (aucun serveur n'est nécessaire)
make smoke
```

Les tests couvrent : création d'entreprise et de station, connexion, enregistrement d'un
lavage, encaissement, dépense, ouverture/clôture de caisse (écart de comptage compris),
calculs des rapports, commissions des laveurs (barème de la station, exception sur un compte,
lavages en attente et annulés exclus, confidentialité), permissions par rôle, périmètre de
station, isolation entre deux entreprises, migrations (rejeu sans effet, réglages conservés) et
validation des entrées. Sans `DATABASE_URL`, ces tests sont ignorés : `go test ./...` reste
vert sur une machine sans PostgreSQL.

`make smoke` rend **18 écrans et états** hors navigateur (`react-dom/server`, dans la pile de
fournisseurs réelle de l'application : thème, notifications, session, station) : connexion,
inscription, mot de passe oublié, invitation, tableau de bord (propriétaire, gérant, caissier),
mise en route, récapitulatif de mise en route, lavages, caisse, prestations, tarifs, clients,
employés, dépenses, rapports, paramètres (plusieurs onglets), abonnement, page introuvable. Le
réseau y est **coupé** (`fetch` échoue volontairement) et la session est fournie d'avance : ce
contrôle attrape l'« écran noir » — une erreur pendant le rendu React fait disparaître toute
l'application, sans message. Il ne remplace pas un test dans un vrai navigateur, qui reste
disponible à part (`make rendu`, qui exige `puppeteer-core` et un Chromium) ; il s'exécute partout,
en quelques secondes, sans rien installer de plus et **sans écrire dans la base**.

Le parcours réel (données, permissions, refus) est couvert par les tests Go (`make test-e2e`) et par
les sondes d'acceptation (`make verifier-acces`), qui parlent à l'API en marche sur une base
consacrée : `sonde-onboarding.py` (inscription → treize étapes → activation), `sonde-lot8.py`
(récapitulatif, activation, première fiche, tableau de bord), `sonde-tarifs.py`,
`sonde-vehicules.py`, `sonde-invitations.py`, `sonde-fiche-payee.py`, `sonde-lot7.py`,
`sonde-metier.py`, `sonde-api.py`, `sonde-suspension.py`.

### Recette de première utilisation — **à réécrire, ne pas lancer**

`make recette` rejoue deux parcours dans un vrai navigateur, sur une base jetable. **Cette recette
décrit le produit d'avant la mise en route** : elle attend un « catalogue d'Abidjan déjà tarifé »
et enregistre un lavage juste après l'inscription, alors que le catalogue arrive désormais **sans
prix** (aucun tarif n'est inventé) et qu'une station non activée refuse toute écriture de métier
(`409 onboarding_incomplete`). Elle échoue donc au troisième point, et le script comme les
scénarios portent cet avertissement en tête. La recette qui fait foi aujourd'hui est la campagne
d'acceptation en lignes de commande (`make verifier-acces` + `verifier-acces` sur une base vierge) :
la marche à suivre et les chiffres relevés sont dans [docs/onboarding.md](docs/onboarding.md) §10.
Pour mémoire, ce que la recette navigateur décrivait :

1. **le premier jour d'une station neuve** : création de l'espace, catalogue d'Abidjan déjà
   tarifé, premier lavage, refus d'un encaissement en espèces sans caisse ouverte, encaissement
   Mobile Money, ouverture de la caisse, encaissement en espèces, clôture avec un écart de
   500 FCFA, cohérence du tableau de bord ;
2. **l'embauche d'un caissier** : inscription d'une nouvelle station, création du compte par le
   propriétaire, mot de passe provisoire affiché une seule fois, première connexion détournée
   vers « Mon compte », choix du mot de passe définitif, premier lavage du caissier, visibilité
   chez le propriétaire, et refus de la sixième embauche par la limite de la formule Essai.

26 contrôles en tout — c'était le contrôle à lancer après une modification sensible. Il le
redeviendra quand les scénarios `frontend/tools/rendu/*.mjs` auront été réécrits pour les treize
étapes de mise en route.

```bash
make build        # le frontend compilé est servi par l'API, comme à la station
CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" make recette
```

La base jetable et l'API de recette sont supprimées à la fin, y compris en cas d'échec.
Les captures sont écrites dans `recette/` (dossier ignoré par git, comme `apercu/`).

Sans `CHROMIUM_PATH`, les outils de rendu utilisent le Chromium du paquet npm
(`npm install --no-save puppeteer-core @sparticuz/chromium`) : ils fonctionnent alors aussi bien
sur macOS que dans un conteneur Linux, y compris les parcours qui ouvrent deux sessions isolées.

### Vérifier les exports CSV

Le propriétaire d'une station télécharge ces fichiers pour refaire ses comptes dans Excel : ils
doivent s'ouvrir proprement et leurs totaux doivent correspondre au tableau de bord. Ce contrôle
lit les 7 exports des rapports, vérifie le format (BOM, séparateur, colonnes alignées) et compare
chaque total aux données réellement en base. L'export des **commissions** (`/commissions/export`)
n'est pas encore couvert par cette sonde : son contenu est vérifié à la main, ligne par ligne.

```bash
make api               # dans un terminal
make verifier-exports  # 41 contrôles
```

Il utilise `psql` s'il est installé, sinon le conteneur Docker ; sans client PostgreSQL il contrôle
le format des fichiers et indique clairement qu'il n'a pas pu comparer les chiffres à la base.

### Vérifier les accès et le périmètre d'un caissier

C'est le contrôle à lancer avant de confier l'application à une station : il attaque
l'application comme le ferait un curieux — sans session, avec le compte d'une autre entreprise,
avec un caissier qui vise la station voisine, ou avec du code SQL dans les formulaires.

```bash
make api              # dans un terminal
make verifier-acces   # dans un autre : 39 contrôles
```

Il vérifie aussi qu'un caissier **ne peut pas faire disparaître une recette** : encaissement
impossible sans caisse ouverte, annulation d'une fiche encaissée refusée, annulation de sa
propre fiche non encaissée autorisée mais **motivée et inscrite au journal**. À la fin, la sonde
compte ses propres données de contrôle et échoue s'il en reste une seule. Les règles complètes
sont dans [docs/securite-acces.md](docs/securite-acces.md).

### Vérifier l'affichage dans un vrai navigateur

`make smoke` monte les pages dans un DOM simulé : il prouve que les données arrivent, pas que le
navigateur rend correctement (graphiques, thème clair, mise en page téléphone). Pour ce contrôle,
un script pilote un vrai Chromium, se connecte avec un compte réel, relève les erreurs JavaScript
et les réponses HTTP en échec, et enregistre une capture par écran :

```bash
make api            # dans un terminal
make web            # dans un autre
make rendu          # capture dans apercu/ (15 écrans + audit)
```

`make rendu` installe `puppeteer-core` **sans l'ajouter aux dépendances** du projet. Indiquez votre
navigateur avec `CHROMIUM_PATH` si besoin :

```bash
CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" make rendu
```

Le geste quotidien du caissier (enregistrer un lavage, encaisser) est rejoué sur un écran de
téléphone par un second script, qui vérifie aussi que les écrans réservés restent invisibles et
qu'aucune page ne déborde horizontalement :

```bash
CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  node frontend/tools/rendu/parcours-telephone.mjs
```

> Le script crée deux vrais lavages dans la station : à lancer sur une base de démonstration, pas
> sur les données réelles de la station.

Guide du personnel : [`docs/guide-utilisation.md`](docs/guide-utilisation.md).

### Vérifier le comportement en production

```bash
make prod-check
```

Sans rien déployer, ce contrôle lance le binaire en mode production et vérifie que la configuration
incomplète est **refusée** (mot de passe plateforme absent ou trop court, cookie non sécurisé), que
l'application compilée est bien servie (accueil, lien profond, assets) et que le cookie de session
porte `HttpOnly`, `Secure` et `SameSite`.

Le contrôle s'exécute sur une base jetable (`washguard_prodcheck`) créée puis supprimée
automatiquement : **vos données de développement ne sont ni lues ni modifiées**. Il emprunte le port
8099 (`PROD_CHECK_PORT=8098 make prod-check` pour en changer) et suppose que l'application est
compilée (`make build`) et que PostgreSQL est démarré (`make db`).

Pour aller plus loin et dérouler le parcours complet en HTTPS sur votre machine (cookies sécurisés,
proxy inverse, en-têtes transmis), voir « Vérifier la mise en service » dans
[`docs/deploiement-https.md`](docs/deploiement-https.md).

Pour vérifier que les migrations fonctionnent **depuis une base vide** :

```bash
make db-reset   # supprime la base puis la recrée
make migrate    # applique les migrations seules (0001 → 0003)
```

---

## 6. Déploiement et exploitation

- [docs/deploiement-https.md](docs/deploiement-https.md) — mise en production derrière HTTPS,
  build du frontend servi par l'API, variables d'environnement obligatoires.
- [docs/sauvegarde-restauration.md](docs/sauvegarde-restauration.md) — sauvegardes
  automatiques avec `pg_dump`, restauration, vérification.
- [docs/api.md](docs/api.md) — conventions de l'API (authentification, erreurs, endpoints).

Résumé production :

```bash
make build                      # interface dans dist/ et binaire washguard-api à la racine
APP_ENV=production \
DATABASE_URL=postgresql://… \
SESSION_COOKIE_SECURE=true \
PLATFORM_ADMIN_EMAIL=… PLATFORM_ADMIN_PASSWORD=… \
STATIC_DIR=dist \
./washguard-api                 # migre puis sert l'API et l'interface sur le même port
```

---

## 7. Limites assumées de cette version

- **Aucune intégration bancaire ni Mobile Money automatique.** L'équipe saisit les
  encaissements reçus (Wave, Orange Money, MTN…) ; il n'y a aucun appel à un opérateur.
- **Paiement des abonnements manuel.** WashGuard n'encaisse pas en ligne : le support
  attribue la formule après règlement (Mobile Money ou espèces). La structure (formules,
  limites, fonctionnalités) est prête pour une facturation automatisée ultérieure.
- **Pas de rôles personnalisés** : les trois rôles opérationnels plus l'administration
  plateforme couvrent les besoins d'une station.
- **Pas d'impression thermique directe** : les reçus s'impriment via la boîte de dialogue
  du navigateur.
- **L'écran Commissions est un état de ce qui est dû**, pas une paie : il calcule les
  commissions sur les lavages terminés et les exporte, mais n'enregistre aucun versement de
  salaire ni aucune déclaration sociale.
- Les sessions expirent après 30 jours ; le mot de passe se change depuis **Mon compte**.

Signalez tout écart entre ce document et le comportement observé : c'est un bug.
