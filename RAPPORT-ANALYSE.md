# Rapport d'analyse — WashGuard (dépôt `ErwinsS225/LavageAuto`)

| | |
| --- | --- |
| **Date d'analyse** | 13 octobre 2026 |
| **Commit analysé** | `91a13e9` — *« Fix: residence→email in EmployeesPage, Clerk v5 OAuth + i18n in LoginForm »* (branche `main`, unique commit du dépôt) |
| **Branche de travail** | `arena/f1c83e85-lavageauto` |
| **Portée** | code source complet (backend Go, frontend React, SQL, scripts), configuration, hygiène du dépôt, contrat d'API front↔back, sécurité multi-entreprises |
| **Hors portée** | exécution de l'API sur une base réelle (aucun serveur PostgreSQL ni chaîne d'outils Go dans l'environnement d'analyse), tests de charge, audit RGPD |

---

## 1. Synthèse

Le produit a une **vision claire et un socle de qualité** : schéma SQL cohérent avec contraintes `CHECK` et index, montants en entiers XOF, hachage bcrypt, sessions en base, journal d'audit, interface entièrement en français, thème clair/sombre, code commenté en français et volontairement sobre en dépendances. Le travail de conception (rôles, caisse, commissions, abonnement, espace laveur) est réel.

Mais l'état livré n'est **pas exploitable en l'état**, pour trois raisons indépendantes et cumulatives :

1. **Des secrets d'infrastructure ont été committés et poussés sur un dépôt GitHub public** (mot de passe de la base Neon, clé secrète Clerk, secret de signature de webhook, clés d'accès stockage).
2. **Le frontend ne compile plus** (`npm run build` échoue : 9 erreurs TypeScript) et **l'application casse ou renvoie des données fausses à l'exécution** : contrat d'API front↔back rompu sur de nombreux points, colonnes SQL inexistantes, agrégations erronées.
3. **Le cloisonnement multi-entreprises et les permissions, présentés comme la garantie centrale du produit (« chaque requête SQL filtre sur `organization_id` »), ne sont pas appliqués** sur une grande partie des routes : lecture et écriture inter-entreprises, réinitialisation du mot de passe de n'importe quel compte (y compris l'administrateur plateforme), contournement du plafond de remise et de l'interdiction d'annulation par le caissier.

Par ailleurs, le `README.md` (457 lignes) décrit **une autre architecture que celle du dépôt** : il documente un `Makefile` (27 cibles), un dossier `docs/`, `compose.yaml`, une arborescence `backend/internal/...` + `frontend/src` et 13 tests d'intégration — **aucun de ces éléments n'existe**. La documentation n'est pas seulement en retard : elle décrit des garanties de sécurité et des contrôles qui ne sont pas dans le code.

### Verdict par domaine

| Domaine | État | Commentaire |
| --- | --- | --- |
| Sécurité des secrets | 🔴 Critique | Identifiants de production/test en clair dans un dépôt **public** |
| Cloisonnement multi-entreprises | 🔴 Critique | Contournable par simple paramètre ou identifiant |
| Autorisations par rôle | 🔴 Critique | Codées en dur dans quelques handlers, absentes ailleurs |
| Intégrité des montants / caisse | 🔴 Élevé | Encaissement sans caisse, double comptage, remise non plafonnée |
| Contrat API front↔back | 🔴 Élevé | ~8 réponses incompatibles → pages cassées ou vides |
| Compilation / build | 🔴 Bloquant | `tsc -b` en échec, `dist/` non reproductible |
| Justesse des calculs | 🟠 Élevé | Agrégation par produit cartésien, filtre `'cancelled'` inexistant |
| Qualité du code | 🟡 Moyen | Fichiers plats, duplication, code mort, `SELECT` non filtrés |
| Tests / CI | 🔴 Absent | 0 test, aucune intégration continue |
| Hygiène du dépôt | 🟠 Moyen | `dist/` (2,6 Mo) et binaire Go (17 Mo) committés, `.gitignore` incomplet |
| UX / design produit | 🟢 Bon | Interface cohérente, écrans nombreux, entièrement FR |

### Les 5 actions à faire aujourd'hui

1. **Passer le dépôt en privé immédiatement** puis **révoquer/renouveler tous les secrets** exposés (base Neon, clés Clerk, secret webhook, clés stockage) — voir §2.
2. **Rétablir la compilation** et vérifier que les 9 erreurs TypeScript sont corrigées (quelques dizaines de minutes) — voir §3.
3. **Centraliser l'autorisation** : un middleware `requirePermission` + un helper `stationAutorisee(org, station)` appliqué à **toutes** les routes, et un filtre `organization_id` sur **tous** les `SELECT ... WHERE id = $1` — voir §4 et §5.
4. **Corriger les ruptures de contrat API** (enveloppes `items`/`options`, `/cash/current`, `/reports/overview`, `/commissions`, `/platform/overview`) — voir §7.
5. **Réécrire le `README.md`** (ou livrer les fichiers qu'il décrit) et **ajouter une CI** qui lance `go build`, `tsc -b`, `gofmt`, un lint et les tests de bout en bout — voir §11.

---

## 2. 🔴 P0 — Secrets d'infrastructure exposés dans un dépôt public

**Fait constaté.** Le commit `91a13e9` ajoute à la racine le fichier `.env.backup.20261013-162500` (1 445 octets). Ce fichier contient, en clair :

- l'URL complète de la base PostgreSQL Neon **avec le mot de passe du rôle propriétaire** (`postgresql://neondb_owner:npg_…@ep-…neon.tech/neondb?sslmode=require`) ;
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` pointant vers un stockage compatible S3 hébergé par Neon (`nak_live_…` / `nsk_live_…`) ;
- `CLERK_SECRET_KEY` (`sk_test_…`), `CLERK_WEBHOOK_SIGNING_SECRET` (`whsec_…`), la clé publique et l'URL JWKS Clerk ;
- la clé publique Clerk du front.

**Aggravant.** Le dépôt `ErwinsS225/LavageAuto` est **public** (`gh repo view` → `isPrivate: false`) et ce commit est **poussé sur `main`** (API GitHub : fichier présent dans l'arbre distant, `size=1445`). Les identifiants sont donc lisibles par n'importe qui, y compris hors de la conversation, et indexables par les robots qui scannent GitHub en continu. Le fichier a été créé **avant** le commit : c'est la sauvegarde du `.env` de développement qui a été ajoutée par erreur au `git add`.

**Le `.gitignore` ne protège pas** : il ignore `.env`, `.env.local` et `.env.*.local`, mais **pas** `.env.backup.*`.

**Conséquences concrètes.**

- Accès direct en lecture/écriture/suppression à l'ensemble des données clients (entreprises, employés, lavages, encaissements, dépenses) et possibilité de `DROP` des tables.
- Les clés de stockage permettent de lire/écrire le bucket (sauvegardes éventuelles).
- Avec `CLERK_SECRET_KEY`, un attaquant peut administrer l'instance Clerk (lister les utilisateurs, les supprimer) ; avec `CLERK_WEBHOOK_SIGNING_SECRET`, il peut **forger des webhooks valides** et créer/modifier des comptes via `POST /api/v1/webhooks/clerk` (`backend/handlers_clerk.go`) — sans même avoir le mot de passe.

**À faire, dans cet ordre.**

1. Passer le dépôt en **privé** (freine la copie, ne remplace pas la rotation).
2. **Rotation complète** : mot de passe du rôle Neon (+ rotation du rôle si possible), clés stockage, `CLERK_SECRET_KEY`, `CLERK_WEBHOOK_SIGNING_SECRET`. Créer un `.env` local neuf à partir de `.env.example`.
3. **Purger l'historique** (`git filter-repo --path .env.backup.20261013-162500 --invert-paths`, puis `git push --force`), puis demander la purge du cache GitHub au support ; considérer malgré tout les secrets comme compromis (les forks et les caches de recherche les ont peut-être gardés).
4. Compléter `.gitignore` : `.env*` avec une exception `!.env.example`, `*.backup*`, `dist/`, `backend/backend`, `washguard-api`.
5. Ajouter un **scan automatique de secrets** (gitleaks / trufflehog) en pré-commit **et** en CI.
6. Passer en revue le reste du dépôt : le binaire `backend/backend` et `dist/` sont aussi committés (voir §11).

> Aucune valeur de secret n'est recopiée dans ce rapport : les valeurs sont dans le dépôt, à l'emplacement indiqué.

---

## 3. 🔴 P0 — Le frontend ne compile plus, l'application livrée est incohérente

`npm install` s'exécute normalement, puis :

```
$ npm run build          # tsc -b && vite build
src/features/employees/api.ts(126,9): error TS6133: 'queryClient' is declared but its value is never read.
src/layouts/AdminLayout.tsx(7,37): error TS6133: 'Toaster' is declared but its value is never read.
src/layouts/AppLayout.tsx(29,53): error TS5097: An import path can only end with a '.tsx' extension…
src/pages/AuthPage.tsx(7,28): error TS2307: Cannot find module '@/layouts/AuthImagePanel'…
src/pages/LoginPageLavPro.tsx(1,1): error TS6133: 'LoginPageNew' is declared but its value is never read.
src/pages/LoginPageLavPro.tsx(3,16): error TS2304: Cannot find name 'LoginPageLavPro'.
src/pages/RegisterPageLavPro.tsx(1,1): error TS6133: 'RegisterPageNew' is declared but its value is never read.
src/pages/RegisterPageLavPro.tsx(3,16): error TS2304: Cannot find name 'RegisterPageLavPro'.
src/pages/WashOrdersPage.tsx(649,51): error TS2339: Property 'role_label' does not exist on type 'EmployeeOption'.
```

Trois familles de défauts :

| # | Cause | Correction |
| --- | --- | --- |
| 1 | Code mort laissé dans le build : `LoginPage.tsx`, `LoginPageNew.tsx`, `LoginPageRefonted.tsx`, `LoginPageLavPro.tsx`, `RegisterPage*`, `AuthPage.tsx`, `AuthLayoutSplit.tsx`, `lib/theme.tsx`, `hooks/useStation.ts` ne sont **référencés par personne** (voir §11). Ce sont eux qui portent 8 des 9 erreurs. | Supprimer les fichiers non importés (les seules pages utilisées sont `LoginPageNew` et `RegisterPageNew`, cf. `src/App.tsx`). |
| 2 | `AppLayout.tsx:29` importe `'@/hooks/useStation.tsx'` (extension interdite par `tsconfig.app.json`) alors que les autres fichiers importent `@/hooks/useStation` **sans extension** — qui résout vers `hooks/useStation.ts`, un fichier **différent** (il ne contient ni `ThemeProvider` ni `useTheme`). | Garder un seul fichier `hooks/useStation.tsx`, importer sans extension. |
| 3 | `AuthPage.tsx` importe `@/layouts/AuthImagePanel` alors que le fichier est `src/components/auth/AuthImagePanel.tsx`. | Corriger le chemin, ou supprimer la page morte. |

**Conséquence directe** : `dist/` (37 fichiers, 2,6 Mo, committés) **ne peut plus être régénéré**. L'application servie par l'API Go en production (`STATIC_DIR=../dist`, `scripts/start.sh`) correspond donc à un état de code antérieur et non vérifiable ; rien ne garantit qu'il reflète le comportement attendu, et toute correction de bug côté front est impossible à déployer.

---

## 4. 🔴 P1 — Cloisonnement entre entreprises : la garantie annoncée n'est pas dans le code

Le README affirme : « chaque requête SQL filtre sur `organization_id` extrait de la session côté serveur » et cite `TestIsolationEntreEntreprises`, `TestPermissionsParRole`, `TestPerimetreDuneStation`. **Ces tests n'existent pas** (0 fichier `_test.go` dans le dépôt) et plusieurs handlers ne filtrent pas.

### 4.1 Routes qui acceptent un identifiant sans vérifier l'entreprise

Pattern systématique : `WHERE id = $1` sans `AND organization_id = $2`.

| Handler | Fichier:ligne | Effet |
| --- | --- | --- |
| `handleWashOrderDetail` | `handlers_wash_order_detail.go:14` | Lecture de **n'importe quelle fiche** (client, plaque, montants) de n'importe quelle entreprise |
| `handleEmployeeResetPassword` | `handlers_employees_update.go:69` | **Réinitialisation du mot de passe de n'importe quel compte de la base**, y compris `superadmin` ; le mot de passe provisoire est renvoyé dans la réponse → prise de contrôle |
| `handleEmployeeUpdate` | `handlers_employees_update.go:22` | Modification (`role`, `is_active`, `station_id`, barème) de n'importe quel compte, y compris désactivation du propriétaire d'une autre entreprise |
| `handleEmployeeActivity` | `handlers_employees_activity.go:19` | Activité et chiffre d'affaires d'un employé d'une autre entreprise |
| `handleCustomerDetail` / `handleCustomerUpdate` / `handleCustomerArchive` | `handlers_customers_detail.go:21,98,108` | Lecture, modification et archivage du client d'une autre entreprise |
| `handleServiceUpdate` / `handleServiceDisable` | `handlers_services_update.go:22,72` | Modification du **tarif** du catalogue d'une autre entreprise |
| `handlePaymentVoid` | `handlers_payments_void.go:20` | *Vérifie l'organisation* (contre-exemple positif à généraliser) |

### 4.2 Routes qui font confiance à `station_id` fourni par le client

`station_id` (corps ou paramètre d'URL) est utilisé **sans vérifier qu'il appartient à l'entreprise de l'utilisateur**, alors que la colonne `station_id` est une clé étrangère valide : l'écriture inter-entreprises réussit.

| Handler | Fichier:ligne | Effet |
| --- | --- | --- |
| `handleWashOrdersList` | `handlers_wash_orders.go:72` | Lecture des lavages d'une autre entreprise |
| `handleCashCurrent`, `handleCashSessionsList` | `handlers_cash.go:50,129` | Lecture de la caisse d'une autre entreprise |
| `handlePaymentsList` | `handlers_payments.go:61` | Lecture des encaissements d'une autre entreprise |
| `handleServicesList` | `handlers_services.go:25` | Lecture du catalogue d'une autre entreprise |
| `handleExpenseCreate`, `handleCustomerCreate`, `handleEmployeeCreate` | `handlers_misc.go:78`, `handlers_customers.go:77`, `handlers_employees_write.go:43` | **Création de données dans la station d'une autre entreprise** |
| `handleWashOrderCreate` | `handlers_wash_order_create.go:54,72` | Fiche créée chez le voisin ; de plus `SELECT name, price_xof FROM services WHERE id = $1` n'est pas filtré sur l'entreprise |
| `handlePaymentCreate`, `handleCashSessionOpen` | `handlers_payments_write.go:36`, `handlers_cash_write.go:27` | Encaissement / ouverture de caisse sur la station d'une autre entreprise |

**Impact.** La promesse commerciale centrale (« aucune donnée n'est partagée entre deux entreprises ») est fausse. Pour un client, cela signifie qu'un concurrent équipé de WashGuard peut lire son chiffre d'affaires, ses plaques d'immatriculation et ses clients, ou écrire dans sa caisse. Un audit de sécurité client ou une revue RGPD (données clients, plaques = données personnelles) bloquerait la mise en production.

**Correctif type à écrire une fois** (`permissions.go`) :

```go
// stationAutorisee renvoie la station si elle appartient à l'entreprise,
// sinon une erreur 404 (ne pas révéler l'existence de la ressource).
func (a *App) stationAutorisee(ctx context.Context, orgID, stationID string) error

// orgScope renvoie le motif « AND organization_id = $N » à concaténer,
// ou utilise systématiquement : WHERE id = $1 AND organization_id = $2
```

puis remplacer partout `WHERE id = $1` par `WHERE id = $1 AND organization_id = $2`.

---

## 5. 🔴 P1 — Autorisations par rôle : bricolées à la main dans une poignée de handlers

`backend/app.go` définit une matrice `rolePermissions` complète (44 permissions) et le front s'en sert pour masquer les menus (`AppLayout`, `RequireAuth`). **Mais le backend ne consulte jamais cette matrice** : il n'existe ni `requirePermission` ni `hasPermission`. Le contrôle d'accès repose sur quelques tests ad hoc : `isRestrictedRole()` (bloque caissier + laveur), `requireSuperadmin()`, et deux comparaisons `user.Role == …`. Résultat : le menu est masqué, **l'API reste ouverte**. Exemples vérifiés :

| Route | Ce que le README annonce | Ce que le code fait |
| --- | --- | --- |
| `GET /commissions`, `GET /commissions/export` | « Réservé au propriétaire et au gérant » | Aucun contrôle de rôle : un caissier ou un laveur lit la paie de ses collègues (`handlers_misc.go:501,546`) |
| `GET /employees` | Gestion réservée | Aucun contrôle : un caissier lit les barèmes de commission de tous les comptes (`handlers_employees.go:42`) |
| `POST /expenses` | Dépenses = gérant/propriétaire | Aucun contrôle : un **laveur** peut enregistrer une dépense (`handlers_misc.go:56`) |
| `POST /cash/payments/{id}/void` | « Le caissier ne peut pas supprimer un encaissement » | Le test ne bloque que `cashier` : un **laveur** peut annuler un encaissement (`handlers_payments_void.go:11`) |
| `PATCH /stations/{id}` | Réservé au propriétaire | Aucun contrôle : un caissier modifie les réglages de la station — **dont `max_cashier_discount_xof`** (`handlers_stations.go:109`) |
| `POST /stations` | « Propriétaire » | Aucun contrôle (`handlers_stations.go:47`) |
| `PATCH /organizations/current` | Réservé au propriétaire | Aucun contrôle : un caissier renomme l'entreprise (`handlers_org.go:75`) |
| `GET/POST/PATCH /reports/*`, `/reports/export` | `report:view`, `report:export` | Aucun contrôle (`handlers_misc.go:171,175,820`) |
| `PATCH /wash-orders/{id}` | « Corriger une fiche en attente non encaissée, motif obligatoire » | Aucun contrôle de rôle, **ni de statut**, ni de motif obligatoire (`handlers_wash_order_update.go:37`) |

**Impact.** Un caissier peut, avec les seuls droits de son compte : relever les réglages de remise de la station à un montant arbitraire, puis accorder des remises illimitées ; lire la paie des laveurs ; effacer une dépense en la marquant annulée ; créer des dépenses fictives. Le journal d'audit enregistre l'action (`actor_role`), ce qui permet de reconstituer les faits après coup, mais rien ne bloque en amont.

**Correctif.** Un middleware unique, alimenté par `rolePermissions` :

```go
func (a *App) requirePermission(perm string, next http.HandlerFunc) http.HandlerFunc
// mux.HandleFunc("POST /api/v1/employees", a.requirePermission("employee:manage", a.handleEmployeeCreate))
```

et le brancher sur les 72 routes, avec un test qui échoue si une route n'a pas de permission déclarée.

---

## 6. 🟠 P1 — Règles métier de la caisse et de la remise contournables

Ces défauts sont indépendants des permissions : ils concernent l'intégrité des montants.

1. **Encaissement en espèces sans caisse ouverte accepté en silence.** Le README et la recette (`scripts/recette-jour1.sh`, `frontend/tools/rendu/jour-1-station.mjs`) exigent un refus. Le code cherche une session ouverte et, si elle est absente, **enregistre quand même** le paiement (`handlers_wash_order_close.go:76-83`, `handlers_payments_write.go:44-49`). Le réglage de station `require_open_cash_session` (exposé dans Paramètres, `SettingsPage.tsx:31,50`) **n'est lu par aucune ligne du backend** : la case à cocher de l'écran Paramètres n'a aucun effet.
2. **Objectif mensuel : même problème.** `monthly_goal_xof` est enregistré par l'écran Paramètres mais le tableau de bord lit une table `monthly_goals` **qui n'existe pas dans `schema.sql`** (`handlers_misc.go:402`) : la requête échoue, `goal` reste `null`, l'objectif ne s'affiche jamais.
3. **Plafond de remise contournable.** Le plafond n'est vérifié qu'à la création (`handlers_wash_order_create.go:91-101`). Le `PATCH /wash-orders/{id}` applique `discount_xof` **sans aucun plafond** (`handlers_wash_order_update.go:62`) : le caissier crée la fiche sans remise, puis la modifie.
4. **Annulation et statuts contournables.** `PATCH /wash-orders/{id}` accepte `status` libre : un caissier peut passer une fiche à `canceled` ou à `completed` sans encaissement, ce que `POST /wash-orders/{id}/cancel` lui interdit explicitement.
5. **Prix fixé par le client.** À la création d'une fiche, `price := it.UnitPriceXOF` (valeur envoyée par le navigateur) est honoré dès qu'elle est non nulle ; le prix du catalogue n'est utilisé **que si le client envoie 0** (`handlers_wash_order_create.go:68-79`). Le commentaire du code (« Sous-total à partir du catalogue (prix réel côté base) ») est faux. Un caissier peut facturer 1 FCFA un lavage à 5 000 FCFA.
6. **Encaissement partiel qui solde la fiche.** `POST /wash-orders/{id}/payments` marque la fiche `completed` quel que soit le montant reçu (`handlers_wash_order_close.go:120`) et aucun contrôle de reste à payer n'existe : 500 FCFA sur une fiche de 5 000 FCFA clôturent le lavage ; le rapport compte ensuite 5 000 FCFA de chiffre d'affaires et 0 FCFA d'impayé (`due_xof` est codé en dur à 0 dans le rapport, `handlers_misc.go:222`).
7. **Comptabilité de caisse incohérente.** Un paiement en espèces incrémente **`cash_in_xof` et `other_in_xof`** (`handlers_payments_write.go:94-99`) : la même recette est comptée deux fois dans le récapitulatif de session. Les méthodes `carte`, `virement` et `autre` sont versées dans `mobile_money_xof` (libellé affiché : « Mobile Money »). L'annulation ne décrémente pas `payment_count` / `operations` (`handlers_payments_void.go:53-66`) : le compteur d'opérations dérive du journal.
8. **`reference` non unique.** Les références de fiche (`L-AAAAMMJJ-NNNN`) et de caisse (`CAISSE-…-NNN`) sont calculées par `count(*)` du jour, sans contrainte d'unicité ni transaction verrouillante (`handlers_wash_order_create.go:112-118`, `handlers_cash_write.go:44-48`) : deux postes qui saisissent en même temps produisent la même référence.
9. **Commissions fausses et non conformes au barème.** `GET /commissions` ignore `from`, `to` et `station_id` reçus du front, compte **tous les lavages terminés de l'historique** (pas la période), et n'applique **pas** le barème de la station (`settings.commission_per_wash_xof`) alors que l'écran et le README le décrivent. L'export CSV ne gère que `per_wash` (pas le mode `percent`) et divise le périmètre par station (`handlers_misc.go:501-573`).
10. **Limites d'abonnement jamais appliquées.** `max_stations`, `max_employees`, `max_wash_orders_month`, les *feature flags* (`commissions`, `export_csv`…) et l'expiration de l'essai sont **affichés** mais jamais contrôlés côté serveur (`store.go:77-130` ne fait que les lire). Une entreprise en essai peut créer 50 comptes et 50 000 lavages ; un abonnement `suspended` n'empêche pas de travailler (seul un bandeau front s'affiche, `AppLayout.tsx:100`).

---

## 7. 🟠 P1 — Ruptures du contrat d'API front↔back

Les pages React ont été écrites contre une version antérieure de l'API. Voici les incompatibilités vérifiées, avec leur effet utilisateur.

| Écran | Attendu par le front | Renvoyé par le back | Effet |
| --- | --- | --- | --- |
| `Paramètres` | `GET /stations` → `Station[]` (`features/stations/api.ts:11`) | `{items, total}` (`handlers_stations.go:37`) | `stations.find(...)` sur un objet → **erreur JS, page blanche** |
| `Rapports` | `totals`, `by_service`, `by_employee`, `by_method`, `by_expense_category` (`ReportsPage.tsx:65-156`) | `today`, `series`, `employees`, `top_services`, `payment_methods` (`handlers_misc.go:490`) | **Erreur JS** : `overview.data.totals` est `undefined` |
| `Commissions` | `{period, settings, goal, items, totals}` (`types/index.ts:468`) | `{commissions: […]}` (`handlers_misc.go:544`) | **Erreur JS** : `data.totals.employee_count` |
| `Administration → Vue générale` | `active_organizations`, `mrr_xof`, `plans[]`, `recent_organizations[]` (`AdminOverviewPage.tsx:35-56`) | 4 scalaires (`handlers_misc.go:596`) | **Erreur JS** : `query.data.plans.length` |
| `Caisse` | `{open, last, movements}` (`CashPage.tsx:35`) | `{is_open, session, last_session, movements}` (`handlers_cash.go:117`) | La caisse ouverte est vue **fermée** : impossible de clôturer |
| `Lavages` (formulaire) | `{options: EmployeeOption[]}` (`features/employees/api.ts:36`) | `{items: […]}` (`handlers_employees.go:37`) | Liste des laveurs **toujours vide** |
| `Lavages` (formulaire) | `GET /customers/options` (`features/customers/api.ts:20`) | **route inexistante** (404) | Liste des clients vide |
| `Espace laveur` | `{items: WasherTask[]}` (`features/washer/api.ts:26`) | `{tasks: […]}` + colonne `vehicle_name` inexistante (`handlers_misc.go:860-864`) | Requête en **erreur 500** : l'écran laveur ne fonctionne pas |
| `Espace laveur` | `washes_today`, `commission_month_xof` (`types`) | `today_completed`, `total_completed` (`handlers_misc.go:934`) | Compteurs à 0 / `undefined` |
| `Espace laveur` — incident | envoie `incident_note` (`features/washer/api.ts:58`) | attend le champ `description` (`handlers_misc.go:922-924`) | **400** « Description requise. » : signalement impossible |
| `Espace laveur` — avancement | POST statut | `UPDATE wash_orders SET status = …, updated_at = now()` (`handlers_misc.go:905`) ; la colonne `updated_at` **n'existe pas** ; le filtre `status IN ('assigned','in_progress')` référence un statut `assigned` **absent du `CHECK`** (`schema.sql:178`) | **Toute mise à jour échoue** (500) ; les fiches « assignées » n'existent pas dans le modèle |
| Erreurs de formulaire | lit `error.details` (`lib/api.ts:46`) | écrit `error.fields` (`http_json.go:20`) | Les messages de champ (ex. « Plafond de remise dépassé ») ne s'affichent jamais |
| Export CSV des rapports | `GET /reports/export` | `SELECT … vehicle_name …` : colonne **inexistante** (c'est `vehicle_category_name`) (`handlers_misc.go:831`) | Export **en erreur 500** |

Deux écrans supplémentaires sont inutilisables côté serveur : `GET /platform/organizations` échoue à cause d'un alias SQL inexistant (`COALESCE(sum(s.price_xof),0)` sans jointure `subscriptions`, `handlers_misc.go:606`) et `PATCH /platform/organizations/{id}` promet de suspendre une entreprise mais **ne met à jour que le nom** (`handlers_misc.go:658`) — la suspension, fonction clé de l'espace support, n'existe pas.

---

## 8. 🟠 P1 — Justesse des chiffres affichés

Le tableau de bord est l'écran que le propriétaire regarde tous les jours ; trois de ses chiffres sont faux par construction.

1. **Produit cartésien dans la requête principale** (`handlers_misc.go:189-209`) :

```sql
FROM wash_orders o
LEFT JOIN payments p ON p.wash_order_id = o.id AND p.business_day = $2
LEFT JOIN expenses e ON e.organization_id = o.organization_id AND e.business_day = $2 AND e.status = 'valid'
```

La jointure sur `expenses` n'a **aucune condition reliant la dépense à la fiche** : chaque dépense du jour est dupliquée autant de fois qu'il y a de fiches, et **chaque ligne de fiche est dupliquée autant de fois qu'il y a d'encaissements et de dépenses**. Par conséquent `count(*) FILTER (status='completed')` (nombre de lavages), `sum(o.total_xof)` (chiffre d'affaires), `sum(o.discount_xof)` (remises) et `sum(e.amount_xof)` (dépenses) sont **multipliés**. Les totaux du tableau de bord, des rapports et le « résultat simplifié » sont donc faux dès qu'une journée comporte plusieurs encaissements ou plusieurs dépenses. À réécrire avec des sous-requêtes agrégées (CTE) ou des jointures `LATERAL`, une par domaine.
2. **Filtre `'cancelled'` inexistant.** Le statut stocké est `'canceled'` (`schema.sql:178`). Deux requêtes écrivent `status <> 'cancelled'` (`handlers_misc.go:447` pour les prestations les plus vendues, `handlers_misc.go:592` pour le chiffre d'affaires de la plateforme) : la condition est **toujours vraie**, donc les lavages annulés sont comptés dans le CA.
3. **Chiffres par employé approximatifs.** `revenue_month_xof` et `collected_month_xof` reçoivent la même valeur, calculée sur **tout l'historique** et non sur le mois (`handlers_employees.go:75-98`) ; `due_xof` est codé à 0 dans le rapport (`handlers_misc.go:222`) alors que la fiche affiche un reste à payer ; la série de 7 jours compte les paiements et dépenses de toute l'entreprise sans filtre de station.
4. **Décalage de périmètre.** `Africa/Abidjan` est codé en dur (`businessDay`, `handlers_employees.go:107`) et la colonne `stations.timezone` n'est jamais utilisée : une station hors Côte d'Ivoire aurait sa journée décalée.

---

## 9. 🟠 P2 — Authentification et session

| Point | Constat | Recommandation |
| --- | --- | --- |
| Politique de mot de passe | L'inscription accepte un mot de passe d'**un caractère** (`handlers_register.go:33`) ; la création d'employé aussi. Seul le changement de mot de passe impose 8 caractères (`handlers_auth.go:78`) | Règle commune : 10 caractères minimum, contrôle unique partagé |
| Force brute | Aucun délai, aucun verrouillage, aucun captcha sur `POST /auth/login` ; le message d'erreur est identique (bien) mais rien ne limite les tentatives | Limiteur par IP + par compte, journalisation, verrouillage temporaire |
| Jetons de session | Stockés **en clair** dans `sessions.token`, jamais purgés à l'expiration | Hacher le jeton (SHA-256) avant stockage, tâche de purge |
| Cookie | `SameSite=None` dès que le cookie est `Secure` (`auth.go:139`) + aucune protection CSRF | Réserver `SameSite=None` au strict nécessaire (pont Clerk) ou ajouter un jeton anti-CSRF |
| CORS | `CORS_ALLOWED_ORIGINS` vide ⇒ **toutes** les origines sont acceptées **avec** `Access-Control-Allow-Credentials: true` (`cors.go:16`) | Refuser toute origine non listée dès que l'application est servie sur un domaine ; n'autoriser large qu'en développement local explicite |
| Clerk — `azp` | Le contrôle des parties autorisées lit la variable de paquet `clerkAuthorizedParties`, **jamais alimentée** par `cfg.ClerkAuthorizedParties` (`clerk.go:20,57`) : le contrôle ne s'exécute jamais | Câbler la configuration ou supprimer le code mort |
| Clerk — comptes orphelins | `syncClerkUserToNeon` crée un `cashier` avec `organization_id = NULL` et un e-mail vide (`clerk.go:84`) — ce qui heurte l'index unique `lower(email)` (deux comptes sans e-mail = violation) et produit des comptes sans entreprise ni station | Interdire la création automatique : rattacher explicitement à une entreprise, ou refuser la connexion tant que le rattachement n'est pas fait |
| Webhook Clerk | Assertion de type non sûre sur le secret (`handlers_clerk.go:80`), et **journalisation des en-têtes de signature** en cas d'échec | Assertion avec `ok`, retirer les en-têtes du log |
| Suspendre une entreprise | La colonne `organizations.status` existe mais n'est jamais lue à la connexion | Refuser la connexion et les requêtes si l'entreprise est `suspended`/`archived` |

---

## 10. 🟡 P2 — Schéma, migrations et modèle de données

Points positifs : contraintes `CHECK` sur tous les statuts et méthodes de paiement, index sur les colonnes de filtrage (`organization_id`, `business_day`, `station_id`), clés étrangères avec `ON DELETE`, montants en `INTEGER` (XOF, pas de flottant). C'est un bon schéma pour ce métier.

À corriger :

- **Pas de versionnement.** `migrate()` réexécute `schema.sql` en entier (`db.go:36`) ; la table `schema_migrations` annoncée dans le README n'existe pas. Trois `ALTER TABLE … ADD COLUMN IF NOT EXISTS` en fin de fichier trahissent déjà des rattrapages manuels. Il faut des migrations numérotées (`0001_init.sql`, `0002_…`) et une table de suivi.
- **`wash_order_items` sans `organization_id`** : toute requête sur les lignes doit passer par une jointure pour rester cloisonnée — c'est une source d'erreur garantie (voir §4).
- **`services.station_id NOT NULL`** alors que le seed insère des prestations sans station (`seed.go:72-75`) : l'insertion échoue silencieusement (`_, _ =`) et **la base de démonstration n'a aucun catalogue**, ce que ne montre pas l'écran Prestations.
- **Comptes de démonstration incohérents** avec le README : le seed crée `owner@demo.local` / `manager@demo.local` / `cashier@demo.local` / `washer@demo.local` (`seed.go:39`), le README annonce `demo@washguard.ci`, `gerant@washguard.ci`, `caisse@washguard.ci`, `admin@washguard.app` (ce dernier n'est créé nulle part : **l'espace plateforme est inaccessible après un `seed`**).
- **`payments.kind`** vaut `'lavage'` en dur dans l'encaissement d'une fiche, `'autre'` par défaut ailleurs : `acompte` n'est jamais utilisé alors que le `CHECK` le prévoit.
- **Pas de `updated_at`** sur les tables principales, ce qui explique le bug de l'espace laveur (§7).
- **`audit_logs` sans clé étrangère** et `entity_id` texte : acceptable, mais aucune purge/rétention n'est prévue (croissance illimitée).

---

## 11. 🟡 P3 — Dette, code mort et hygiène du dépôt

### Documentation décrivant un autre projet

`README.md` mentionne 27 cibles `make` (`make doctor`, `make db`, `make seed`, `make test`, `make smoke`, `make recette`, `make rendu`, `make prod-check`, `make verifier-acces`…) et 8 documents. **Aucun de ces fichiers n'existe** :

| Attendu | État |
| --- | --- |
| `Makefile`, `compose.yaml` | absents |
| `docs/` (installation-station, guide-utilisation, securite-acces, deploiement-https, sauvegarde-restauration, api, apercu-ecrans) | absent |
| `backend/internal/...`, `backend/cmd/api`, migrations `0001…0008` | n'existent pas : le backend est un `package main` plat de 38 fichiers |
| `frontend/src`, `frontend/public`, `frontend/package.json` | n'existent pas : le front est à la racine (`src/`, `public/`, `package.json`) |
| 13 tests d'intégration Go (`make test`) | **0 test** dans le dépôt |
| `make smoke` (31 pages), `make recette` (26 contrôles), `make verifier-acces` (39), `make verifier-exports` (41) | les scripts existent pour partie (`scripts/recette-jour1.sh`, `scripts/verifier-production.sh`, `frontend/tools/rendu/*.mjs`) mais **impossibles à lancer** : pas de `Makefile`, pas de `puppeteer-core` déclaré, et ils s'appuient sur les comportements absents (§6, §7) |

Une documentation qui décrit des garanties de sécurité inexistantes est un risque juridique et commercial, pas seulement une dette technique.

### Fichiers à supprimer (code mort vérifié)

`src/pages/LoginPage.tsx`, `LoginPageRefonted.tsx`, `LoginPageLavPro.tsx`, `RegisterPage.tsx`, `RegisterPageRefonted.tsx`, `RegisterPageLavPro.tsx`, `AuthPage.tsx`, `src/layouts/AuthLayoutSplit.tsx`, `src/lib/theme.tsx`, `src/hooks/useStation.ts` (doublon de `useStation.tsx`) — soit **213 lignes et 8 des 9 erreurs de compilation** — auxquels s'ajoute `legacy/server.express.ts.txt` (2 592 lignes de l'ancien serveur Express conservées à la racine).

### Fichiers générés committés

- `dist/` : **37 fichiers, 2,6 Mo** (build Vite) — non reproductible aujourd'hui ; `.gitignore` ignore `frontend/dist/` mais pas `dist/`.
- `backend/backend` : **binaire Go de 17,6 Mo** committé (le `.gitignore` ignore `washguard-api`, pas ce nom).
- `backend/=` : fichier vide créé par une commande shell malformée (`git add backend/=`).

### Divers

- **Aucune notification ne s'affiche jamais dans l'application.** `useToast()` (utilisé par 8 écrans : caisse, dépenses, lavages, employés, espace laveur, mon compte…) renvoie des fonctions vides quand aucun `ToastProvider` n'est au-dessus du composant (`Toaster.tsx:79-84`). Or `AppLayout` ne monte aucun provider, et `AuthLayout` le monte **en frère** de `<Outlet />` (`AuthLayout.tsx:11`) donc hors de la portée des formulaires. Conséquence : « Lavage enregistré », « Caisse ouverte », ou les messages d'échec disparaissent silencieusement — l'utilisateur croit que l'action n'a pas été prise en compte. Correctif : envelopper `App` (ou les layouts) dans un unique `ToastProvider` ancêtre.
- **`package-lock.json` désynchronisé de `package.json`.** Le fichier committé contient encore **1 025 lignes** du backend Express abandonné (`@types/express`, `cookie-parser`, `body-parser`, `cors`…) que `package.json` ne déclare plus : `npm install` les supprime du fichier au premier lancement. À régénérer (`rm package-lock.json && npm install`) et à committer, sans quoi toute installation est non déterministe.
- `src/main.tsx` importe `./index.css` **trois fois** avec trois commentaires redondants.
- `public/` ne contient pas `favicon.svg`, `favicon-32.png`, `apple-touch-icon.png` référencés par `index.html` (4 requêtes 404 au chargement).
- Aucune CI (`.github/` absent) : rien n'empêche de pousser du code qui ne compile pas — c'est exactement ce qui s'est produit.
- Aucun `gofmt`/linter configuré (`.golangci.yml` absent) ; `helpers.go` contient une fonction `setClause` utilisée à un seul endroit, `app.go` une fonction mal orthographiée (`isStaffRestringed` en commentaire) ; `handlers_misc.go` (966 lignes) mélange dépenses, rapports, commissions, plateforme, export et espace laveur.
- `models.go` définit des structs parallèles aux maps JSON écrites à la main dans les handlers : deux sources de vérité pour la même réponse.
- `withWebhookSecret(cfg Config)` reçoit une configuration qu'il n'utilise pas (il lit `os.Getenv`).

---

## 12. Ce qui est solide et à préserver

Pour être juste, plusieurs choix sont bons et méritent d'être conservés :

- **Modèle SQL** : contraintes `CHECK` exhaustives sur les statuts et moyens de paiement, index pertinents, `ON DELETE` réfléchi, montants entiers, `audit_logs` générique.
- **Sécurité de base** : bcrypt pour les mots de passe (et non du clair comme dans le `server.ts` d'origine, cf. le commentaire de `auth.go:16`), jetons de session aléatoires de 256 bits, cookie `HttpOnly` (+ `Secure` en HTTPS), `requireAuth` systématique sur les 72 routes, contrôle `superadmin` bien centralisé (`requireSuperadmin`).
- **Isolation partielle déjà en place** : `getOwnedOrder` (`handlers_wash_order_update.go:18`, utilisé par 4 routes), `getOwnedCashSession` (`models_cash.go:77`), la vérification d'organisation de `handlePaymentVoid` et celle de `handleCashSessionDetail` — le bon réflexe existe, il n'est simplement pas généralisé (une dizaine d'appels de contrôle pour 72 routes).
- **Traçabilité** : `addAudit` appelé sur toutes les opérations sensibles (remise, annulation, clôture, écart de caisse, réinitialisation de mot de passe, réouverture), avec `actor_role`.
- **Front** : React 19 + TanStack Query, découpage `features/`, garde de routes par permission, interface intégralement en français, format FCFA et `jj/mm/aaaa`, thème clair/sombre, aucun CDN externe (tout est local).
- **Sobriété** : pas d'ORM, SQL explicite ; peu de dépendances ; composants UI écrits à la main.
- **Exploitation** : `-migrate-only`, arrêt gracieux, `ReadHeaderTimeout`, scripts d'installation/sauvegarde/recette déjà rédigés en français.

---

## 13. Plan d'action priorisé

### P0 — à faire sous 24 h

| # | Action | Où | Effort |
| --- | --- | --- | --- |
| 1 | Passer le dépôt en privé, purger `.env.backup.*` de l'historique, **révoquer et renouveler tous les secrets** | GitHub + Neon + Clerk | 1 h |
| 2 | Compléter `.gitignore` (`dist/`, `backend/backend`, `.env*` sauf exemple, `*.backup*`) et retirer les artefacts du suivi Git | racine | 15 min |
| 3 | Corriger les 9 erreurs TypeScript en supprimant le code mort (§11) : `npm run build` doit passer | `src/` | 30 min |
| 4 | Ajouter une CI minimale : `go build ./...`, `gofmt -l`, `tsc -b && vite build` | `.github/workflows/ci.yml` | 45 min |
| 5 | Bloquer `PATCH /wash-orders/{id}` sur les fiches payées et re-vérifier le plafond de remise ; supprimer `price := it.UnitPriceXOF` (prix = catalogue) | `handlers_wash_order_update.go`, `handlers_wash_order_create.go` | 2 h |

### P1 — semaine 1

| # | Action | Effort |
| --- | --- | --- |
| 6 | `requirePermission()` branché sur les 72 routes à partir de `rolePermissions` (+ test qui échoue si une route n'est pas couverte) | 1 j |
| 7 | `stationAutorisee()` + `organization_id` sur **tous** les `WHERE id = $1` (§4) ; test d'isolation à deux entreprises sur chaque ressource | 1 j |
| 8 | Réparer le contrat d'API (§7) : réponses `{items}` vs `{options}`, `/cash/current`, `/reports/overview`, `/commissions`, `/platform/overview`, `/customers/options`, `/washer/*` ; `fields` vs `details` | 1,5 j |
| 9 | Corriger la requête du tableau de bord (agrégats indépendants), les `'cancelled'` → `'canceled'`, l'export `vehicle_name`, l'`updated_at`, le statut `assigned` | 0,5 j |
| 10 | Imposer la règle « espèces ⇒ caisse ouverte » et lire `require_open_cash_session` ; corriger les compteurs de caisse (`other_in_xof`, méthodes non-espèces, annulations) | 0,5 j |
| 11 | Activer réellement les limites d'abonnement et l'expiration de l'essai côté serveur ; rendre la suspension d'entreprise effective | 1 j |
| 12 | Migrations numérotées + table `schema_migrations` ; corriger le seed (catalogue lié à la station, comptes de démo alignés sur le README, création du compte plateforme) | 0,5 j |

### P2 — semaine 2-3

13. Authentification : politique de mot de passe unique, limitation des tentatives, hachage des jetons de session, purge des sessions expirées, `SameSite`/CSRF, durcissement CORS.
14. Réécrire `README.md` sur l'architecture réelle **et** rédiger les documents qu'il promet (installation station, guide d'utilisation, sécurité, sauvegarde/restauration) ; ou livrer le `Makefile` et `compose.yaml` attendus.
15. Créer la suite de tests : tests Go de bout en bout (isolation, permissions, caisse, commissions, limites) + un test de rendu des pages (jsdom) qui aurait détecté les ruptures du §7 ; brancher `scripts/recette-jour1.sh` et `frontend/tools/rendu/*.mjs` sur une cible `make recette` fonctionnelle.
16. Découper `handlers_misc.go` par domaine ; regrouper la sérialisation JSON dans un endroit unique ; monter le `Toaster` global ; ajouter les favicons manquants.
17. Côté produit : exporter les commissions couvrant la période et le barème réel, page « reste à payer », impression de reçu (annoncée comme limite assumée), rapprochement Mobile Money.

---

## Annexe A — Preuves et commandes utilisées

```bash
# Contexte
git log --oneline -1                    # 91a13e9, unique commit
git ls-files | wc -l                    # 169 fichiers suivis
find . -name '*_test.go' | wc -l        # 0 test
grep -c 'mux.HandleFunc' backend/main.go # 72 routes
gh repo view ErwinsS225/LavageAuto --json isPrivate   # false
gh api repos/ErwinsS225/LavageAuto/contents/.env.backup.20261013-162500  # présent en ligne

# Volumétrie
cat backend/*.go | wc -l                # 5 544 lignes Go (38 fichiers)
find src -name '*.tsx' | xargs cat | wc -l   # 5 624 lignes TSX
find src -name '*.ts'  | xargs cat | wc -l   # 1 750 lignes TS
wc -l backend/schema.sql                # 328 lignes
du -sh dist backend/backend             # 2,6 Mo / 17,6 Mo (tous deux committés)

# Compilation
npm install                             # OK
npm run build                           # ÉCHEC : 9 erreurs TypeScript (liste en §3)

# Cohérence SQL : colonnes référencées mais absentes du schéma
# updated_at (wash_orders), vehicle_name (wash_orders), monthly_goals (table)
```

## Annexe B — Inventaire des routes et de leur état

Les 72 routes partagent `requireAuth` (sauf `GET /api/v1/plans`, `POST /api/v1/auth/{login,register,clerk}` et le webhook). Colonnes : **P** = contrôle de permission/role, **O** = filtre d'entreprise, **S** = contrôle du périmètre de station.

| Route | P | O | S | Remarque |
| --- | --- | --- | --- | --- |
| `POST /auth/login`, `/register`, `/logout`, `/clerk` | n/a | n/a | n/a | Pas de limitation des tentatives ; mot de passe non validé à l'inscription |
| `GET /auth/me`, `PATCH /auth/password` | ✔ | ✔ | ✔ | OK |
| `GET/PATCH /organizations/current` | ✘ | ✔ | n/a | Un caissier peut renommer l'entreprise ; `status` non contrôlé |
| `GET /organizations/{subscription,audit}` | ✘ | ✔ | n/a | Audit lisible par tout rôle |
| `POST /organizations/subscription/request` | ✘ | ✔ | n/a | OK fonctionnellement |
| `GET /stations` | ✘ | ✔ | n/a | Renvoie `{items}` (front attend un tableau) |
| `POST /stations`, `PATCH /stations/{id}` | ✘ | ✔ | n/a | Réglages modifiables par un caissier |
| `GET /vehicle-categories` | ✘ | n/a | n/a | Référentiel global, OK |
| `GET/POST/PATCH/DELETE /services*` | partiel | ✘ | ✘ | Lecture/écriture inter-entreprises par `station_id`/`id` |
| `GET/POST /customers`, `GET/PATCH/DELETE /customers/{id}` | ✘ | ✘ (par id) | ✘ | Lecture/modification inter-entreprises |
| `GET /employees`, `/employees/options` | ✘ | ✔ | ✘ | Barèmes visibles par tous ; `options` mal nommé |
| `POST /employees` | partiel | ✘ | ✘ | Station d'une autre entreprise acceptée ; limites de formule non appliquées |
| `PATCH /employees/{id}`, `POST /employees/{id}/reset-password` | partiel | ✘ | ✘ | **Prise de contrôle de n'importe quel compte** |
| `GET /employees/{id}/activity` | ✘ | ✘ | ✘ | Fuite inter-entreprises |
| `GET /wash-orders`, `POST /wash-orders` | ✘ | ✘ | ✘ | `station_id` non vérifié ; prix fixé par le client |
| `GET /wash-orders/{id}` | ✘ | ✘ | ✘ | Fiche de n'importe quelle entreprise |
| `PATCH /wash-orders/{id}` | ✘ | ✔ | ✘ | Statut et remise non contrôlés |
| `POST /wash-orders/{id}/assign` | partiel | ✔ | ✘ | Laveur d'une autre entreprise acceptable |
| `POST /wash-orders/{id}/cancel` | ✔ | ✔ | ✘ | OK (caissier/laveur bloqués) |
| `POST /wash-orders/{id}/payments` | ✘ | ✔ | ✘ | Encaissement partiel = fiche soldée |
| `GET /cash/*` | ✘ | ✘ | ✘ | `station_id` non vérifié |
| `POST /cash/sessions`, `/close`, `/reopen` | partiel | ✔ | ✘ | Réouverture bien réservée au gérant |
| `POST /cash/payments`, `/{id}/void` | partiel | ✔ | ✘ | Un laveur peut annuler un encaissement |
| `GET/POST /expenses`, `PATCH`, `/void` | ✘ | ✔ | ✘ | Un laveur peut créer une dépense |
| `GET /reports/{dashboard,overview,export}` | ✘ | ✔ | partiel | Agrégats faux ; export en erreur |
| `GET /commissions`, `/export` | ✘ | ✔ | ✘ | Paie lisible par tous ; période ignorée |
| `GET/PATCH /platform/*`, `POST /platform/organizations/{id}/subscription` | ✔ | n/a | n/a | `overview` et `organizations` en erreur ; suspension inopérante |
| `GET/POST /washer/*` | ✘ | ✘ | ✘ | Colonne et statut inexistants : écran laveur cassé |

---

*Rapport produit par analyse statique du commit `91a13e9`, complétée par la compilation effective du frontend (`npm run build`) et la vérification du contrat d'API front↔back. Les points marqués « erreur » en §7 sont des échecs déduits du code (colonne ou enveloppe absente) : une exécution sur une base réelle les confirmerait en quelques minutes et devrait être la première étape après la remise en état du build.*
