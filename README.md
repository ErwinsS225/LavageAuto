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

![Page de présentation WashGuard](docs/captures/vitrine.png)

Ce qui alimente la vitrine est réel : les captures d'écran viennent du rendu de l'application
(`make rendu`), les tarifs de la base, et il n'y a **pas de témoignages** — des avis inventés
seraient pires que pas d'avis du tout. Ils seront ajoutés quand de vraies stations en donneront.

### Connexion et inscription

Ces deux écrans suivent la même mise en page (`AuthLayout`) : le formulaire sur une surface
claire et centrée, un panneau à droite occupé par une photo de lavage en cours
(`frontend/public/images/lavage-mousse.jpg`). Un dégradé sombre est posé sur la photo : le badge
« Espace station », la phrase d'accroche et les trois repères restent lisibles, y compris sur une
photo très claire. Sur téléphone, le panneau disparaît : seul le formulaire reste, plein largeur.

**Changer la photo** : remplacez ce seul fichier (même nom), puis `make build` lorsque
l'application est servie par l'API Go — rien à modifier dans le code. En développement
(`make web`), le changement est immédiat. L'API demande aux navigateurs de revalider ces fichiers
(`Cache-Control: no-cache`) : la nouvelle photo s'affiche sans vider le cache à la main.

Le formulaire d'inscription est groupé en deux temps — *votre station*, puis *votre compte de
propriétaire* — pour rester lisible malgré ses sept champs. Les deux écrans sont montés par
`make smoke` (34 pages vérifiées), qui compte aussi les appels à `/auth/me` : un visiteur sans
session doit en déclencher un seul. Ce garde-fou vient d'un défaut réel — le 401 de `/auth/me`
invalidait tout le cache, ce qui relançait `/auth/me`, et ainsi de suite (2 300 requêtes en cinq
secondes mesurées dans un vrai navigateur). Le parcours complet « inscription → tableau de bord →
connexion » est rejoué dans un vrai navigateur par les outils de `tools/rendu/`.

Trois choses ne figurent **pas** sur ces pages, faute de les tenir :
- « Continuer avec Google / Facebook » : aucune connexion tierce n'existe ;
- « Mot de passe oublié » par e-mail : aucun e-mail n'est envoyé. Le texte renvoie vers la
  seule voie réelle — le propriétaire réinitialise le mot de passe depuis *Employés* ;
- « Se souvenir de moi » : la session dure 30 jours, l'option n'aurait aucun effet.

| Module | Ce qu'il permet |
| --- | --- |
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
make test      # tests d'intégration du backend (base réelle)
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
│   ├── cmd/api/                point d'entrée : migrations, seed, serveur HTTP
│   └── internal/
│       ├── auth/               inscription, connexion, sessions, mots de passe
│       ├── organizations/      entreprise, abonnement (côté client)
│       ├── stations/ services/ customers/ employees/
│       ├── washorders/         fiches de lavage et paiements associés
│       ├── cash/ payments/     sessions de caisse, encaissements
│       ├── expenses/ reports/  dépenses, indicateurs et export CSV
│       ├── commissions/        paie des laveurs et objectif du mois
│       ├── platform/           espace support WashGuard
│       ├── audit/ permissions/ tenancy (isolation), validation…
│       └── migrator/sql/       migrations versionnées (0001…0008)
├── frontend/                   React + TypeScript + Vite + Tailwind
│   └── src/{components,layouts,pages,features,hooks,lib,types}
├── docs/                       sauvegarde, déploiement, API
├── compose.yaml                PostgreSQL 16 pour le développement
├── Makefile                    commandes de développement
└── .env.example                modèle de configuration
```

**Choix techniques**

- **Go 1.23** avec [chi](https://github.com/go-chi/chi) (routeur), [pgx](https://github.com/jackc/pgx)
  (pilote PostgreSQL) et **SQL explicite** — pas d'ORM, pas de requêtes générées à l'exécution.
- **PostgreSQL 16**, schéma versionné par migrations (table `schema_migrations`).
- **React 19 + TypeScript strict + Vite + Tailwind CSS 4 + React Router + TanStack Query**,
  graphiques [Recharts](https://recharts.org).
- **Composants d'interface écrits à la main** (`components/ui.tsx` et les classes de
  `styles.css`) : ni shadcn/ui, ni bibliothèque de composants. Les motifs dont l'application a
  besoin — carte chiffrée avec écart et comparaison, carte dépliable, jauge avec valeur lisible,
  avatar d'initiales, carte « prochaine étape » — y sont portés directement, avec les jetons de
  couleur du thème clair/sombre. Moins de dépendances à suivre, et rien n'attend une image ou une
  police venue d'Internet.
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

Les tests `TestIsolationEntreEntreprises`, `TestPermissionsParRole` et `TestPerimetreDuneStation`
le vérifient de bout en bout ; `make verifier-acces` le vérifie sur l'application qui tourne.
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
# API : 13 tests de bout en bout sur une vraie base PostgreSQL
make test

# Compilation Go, vérification TypeScript et build du frontend
make check

# Interface : les 13 pages internes sont réellement rendues (jsdom) sur l'API en marche
make api          # dans un terminal
make smoke        # dans un autre
```

Les tests couvrent : création d'entreprise et de station, connexion, enregistrement d'un
lavage, encaissement, dépense, ouverture/clôture de caisse, calculs des rapports, commissions
des laveurs (barème de la station, exception sur un compte, lavages en attente et annulés
exclus, confidentialité), permissions par rôle, isolation entre deux entreprises, migrations et
validation des entrées.

`make smoke` ouvre une session réelle avec un compte de démonstration, monte chaque page de
l'application dans un DOM simulé et vérifie qu'elle affiche ses données (tableau de bord,
lavages, prestations, clients, employés, caisse, dépenses, rapports, commissions, paramètres,
abonnement, compte, page introuvable) — soit 31 pages sur 4 profils. Il vérifie aussi qu'un caissier ne
voit pas les pages qui lui sont interdites et qu'un compte dont le mot de passe est provisoire
est bien détourné vers « Mon compte ». Ce test écrit dans la base de démonstration : il crée un
compte `smoke.provisoire@washguard.ci` (mot de passe provisoire régénéré à chaque exécution) et
règle le barème de commissions de la station de démonstration (500 FCFA par lavage terminé,
objectif de 300 000 FCFA par mois) pour que l'écran Commissions soit vérifié avec de vrais
montants plutôt qu'avec un état de paie à zéro. Utilisez une base dédiée si vous ne voulez pas
de ces lignes. C'est ce qui remplace la
vérification manuelle écran par écran.

### Recette de première utilisation

`make recette` rejoue **deux parcours de première utilisation**, sur **une base jetable** (votre
base de développement n'est ni lue ni modifiée) et dans un vrai navigateur au format téléphone :

1. **le premier jour d'une station neuve** : création de l'espace, catalogue d'Abidjan déjà
   tarifé, premier lavage, refus d'un encaissement en espèces sans caisse ouverte, encaissement
   Mobile Money, ouverture de la caisse, encaissement en espèces, clôture avec un écart de
   500 FCFA, cohérence du tableau de bord ;
2. **l'embauche d'un caissier** : inscription d'une nouvelle station, création du compte par le
   propriétaire, mot de passe provisoire affiché une seule fois, première connexion détournée
   vers « Mon compte », choix du mot de passe définitif, premier lavage du caissier, visibilité
   chez le propriétaire, et refus de la sixième embauche par la limite de la formule Essai.

26 contrôles en tout. C'est le contrôle à lancer après une modification sensible, avant
d'installer chez une station.

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

Aperçu des écrans : [`docs/apercu-ecrans.png`](docs/apercu-ecrans.png). Guide du personnel :
[`docs/guide-utilisation.md`](docs/guide-utilisation.md).

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
redirection HTTP → HTTPS, exports), voir la « Répétition générale » dans
[`docs/deploiement-https.md`](docs/deploiement-https.md) : elle a été exécutée et ses résultats
mesurés y sont consignés.

Pour vérifier que les migrations fonctionnent **depuis une base vide** :

```bash
make db-reset   # supprime la base puis la recrée
make migrate    # applique les migrations seules (0001 → 0005)
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
make build                      # compile le frontend dans frontend/dist
cd backend && go build -o washguard-api ./cmd/api
APP_ENV=production \
DATABASE_URL=postgresql://… \
SESSION_COOKIE_SECURE=true \
PLATFORM_ADMIN_EMAIL=… PLATFORM_ADMIN_PASSWORD=… \
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
