# Problèmes backend — liste d'implémentation

Constatés par analyse du dépôt le **14 octobre 2026** (vérifications `go build`,
`go vet`, `gofmt`, lecture du routeur et des routes front).

Convention : `[ ]` à faire · `[~]` partiel · `[x]` fait et vérifié.
Chaque item indique le fichier concerné et la preuve de vérification attendue.

> **Attention** : des fichiers du backend sont modifiés en parallèle
> (`routes_equipe.go` édité pendant l'analyse). Re-vérifier l'état avant de
> commencer : `cd backend && go build ./... && gofmt -l . && go vet ./...`.

---

## P0 — Bloquant (le serveur ne compile pas ou refuse des routes livrées)

- [x] **Corriger la compilation de `backend/routes_equipe.go`** — *fait le
      14/10/2026* : `a.utilisateurALaPermission` remplacé par
      `sess.aPermission(...)` (aide existante, `auth.go` l.76), variables
      non utilisées supprimées (`updatedBy`, `updatedAt`).
  **Preuve** : `go build ./... && go vet ./...` sans sortie. ✅

- [x] **Routier le Lot 3 (tarifs)** — *fait le 14/10/2026* :
      `a.enregistrerRoutesTarifs(rt)` ajouté dans `nouveauRouteur()`
      (`main.go`), entre le catalogue et l'équipe.
  **Preuve** : serveur démarré sur base jetable, `GET /api/v1/price-rules`
  et `GET /api/v1/price-preview` répondent **401** (route présente,
  protégée) au lieu de 404, pendant que `/api/v1/zzz-inexistante` répond
  bien 404 (témoin négatif). ✅ `coherence-routes.mjs` toujours au vert.

- [x] **Appliquer `migrations/0002_catalogue.sql` au démarrage** — *fait le
      14/10/2026* : `main.go` embarque désormais `migrations/*.sql`
      (`embed.FS`) et les joue dans l'ordre du nom à chaque démarrage ;
      les fichiers sont idempotents, un second passage ne rejoue rien.
  **Preuve** : base vierge `washguard_p0_check` créée, serveur démarré →
  colonnes `services.code`, `template_code`, `sort_order`, `includes`
  présentes, table `station_service_families` créée (41 tables) ;
  **second démarrage** sans erreur (idempotence) ; base jetable supprimée
  après contrôle. ✅

- [x] **`gofmt` sur les 3 fichiers non formatés** — *fait le 14/10/2026* :
      `routes_auth.go`, `routes_onboarding.go`, `routes_stations.go`
      reformattés (purement cosmétiques : alignements d'espaces).
  **Preuve** : `gofmt -l .` sans sortie. ✅

## P1 — Modules métier non livrés (écrans front qui reçoivent des 404)

Les fonctions d'enregistrement sont **vides** dans
`backend/routes_restes.go`, alors que les écrans et les appels existent dans
`src/` (~40 endpoints). Ordre des dépendances : `docs/PLAN-LOTS.md`.

- [x] **Lot 4 — Équipe et invitations** — *fait le 14/10/2026* :
      `routes_equipe.go` réécrit (1 100+ lignes) aligné sur le schéma réel,
      `docs/api.md` et les types front ; `enregistrerRoutesEquipe`
      rempli dans `routes_restes.go` (chemins exacts avant sous-arbres).
      Écarts corrigés : SQL sur colonnes inexistantes (`is_revoked`,
      `created_by`, `employee_activities`), invitation sans `token_hash` ni
      `organization_id` (donc jamais acceptée), aperçu public manquant.
      Rémunérations masquées sans `employee:compensation` (liste ET
      relecture) ; correctif front joint : `useUpdateEmployee` /
      `useResetEmployeePassword` invalident le cache (l'écran affichait des
      données périmées).
  **Preuve** : scénario complet sur base jetable `washguard_lot4_check`
  (créée puis supprimée) : création {id, temporary_password}, liste,
  options, activité, masquage (`commission_mode: null` pour un caissier),
  PATCH, reset, invitation (code + token), aperçu public code/jeton,
  liste {items,total}, `invitation_accepted: true`, révocation, connexion
  refusée d'un compte désactivé. ✅

- [ ] **Lot 5 — Lavages + paiements + caisse** (cœur métier) : implémenter
      `enregistrerRoutesLavages(rt) {}` et `enregistrerRoutesCaisse(rt) {}` :
      `POST|GET /wash-orders`, `GET|PATCH /wash-orders/{id}`,
      `POST /wash-orders/{id}/assign|cancel|payments|receipt`,
      `GET /wash-orders/new`, `POST /customers`, `GET /customers/options`,
      `GET|POST /cash/sessions`, `POST /cash/sessions/{id}/close|reopen`,
      `GET|POST /cash/payments`, `POST /cash/payments/{id}/void`,
      `GET /cash/current`, `POST /payment-methods/{id}` …
      Dépend des Lots 1-3 (moteur de prix) — exiger `onboarding` activé
      (`409 onboarding_incomplete` sinon).
  **Preuve** : parcours caissier complet (encaisser un lavage, soldé,
  fiche verrouillée) + refus si station non activée.

- [ ] **Lot 6 — Dépenses, rapports, commissions** : implémenter
      `enregistrerRapports(rt) {}` (+ dépenses/commissions) :
      `GET|POST /expenses`, `DELETE /expenses/{id}`,
      `POST /expenses/{id}/void`, `GET /reports/dashboard`,
      `GET /reports/overview`, `GET /reports/export`,
      `GET /commissions`, `GET /commissions/export`.
  **Preuve** : le tableau de bord agrège de vraies données de caisse
  (aucune valeur nulle artificielle).

- [ ] **Lot 7 — Matériel, stock, règles de commandes** : implémenter
      `enregistrerRoutesStock(rt) {}` et `enregistrerRoutesRegles(rt) {}` :
      `GET|POST /stations/{id}/stock/items`, `PATCH|DELETE /stock/items/{id}`,
      `GET /stock/items/{id}/movements`, `PATCH /equipment/{id}`,
      `PUT /stations/{id}/order-rules`, `PUT /stations/{id}/cash-settings`.
  **Preuve** : mouvements de stock journalisés, refus si pièce inconnue.

- [ ] **Lot 8 — Espace laveur** : `GET /washer/tasks/{id}`,
      `POST /washer/tasks/{id}/status`, `POST /washer/tasks/{id}/incident`.
  **Preuve** : un laveur ne voit que ses propres tâches (cloisonnement).

- [ ] **Portail support / plateforme** : décider (avec `docs/PLAN-LOTS.md`
      §Lot 8) ce qui est requis pour l'administration `/administration/*`
      du front, aujourd'hui sans API dédiée.

## P1 — Intégrations et sécurité

- [ ] **Trancher et brancher Clerk côté serveur** : le front appelle
      `POST /api/v1/auth/clerk` (`src/lib/clerk.tsx`), mais **aucun fichier
      Go ne mentionne Clerk** (grep nul) → si `VITE_CLERK_PUBLISHABLE_KEY`
      est défini, la connexion échoue. Deux voies exclusives :
      soit implémenter la vérification du jeton Clerk et le pont de session
      dans `routes_auth.go`, soit retirer le pont du front **et** les clés
      Clerk de `.env.example`.
  **Preuve** : connexion de bout en bout dans la configuration retenue ;
  `verifier-env.sh dev` sans clé orpheline.

- [ ] **Secrets publiés à révoquer** (action manuelle, voir `TODO.md` §1) :
      chaîne Neon, clés S3 (`nak_live_*`, `nsk_live_*`), clés Clerk
      (`sk_test_*`, `whsec_*`) — aucune correction de code ne remplace cette
      étape. À faire sur les consoles des fournisseurs, puis noter la date
      dans le journal ci-dessous.
  **Preuve** : nouvelles valeurs en place, dépôt passé en privé, historique
  purgé (`git filter-repo` ou BFG).

- [ ] **`.gitignore` complet** : ajouter `dist/`, `.env`, `*.tsbuildinfo`,
      `.DS_Store` ; **retirer du suivi** le binaire `backend/washguard`
      (15 Mo versionné malgré l'ignorer) : `git rm --cached backend/washguard`.
  **Preuve** : `git status` muet après un build local.

## P2 — Fiabilité, tests, CI

- [ ] **Aligner la CI sur le dépôt réel** (`.github/workflows/ci.yml`) :
      la tâche `sondes` exécute `scripts/verifications/sonde-api.py` et
      `sonde-metier.py` **inexistants** → elle échoue à chaque build ; la
      tâche `backend` commente « 65 tests » alors qu'**aucun `*_test.go`
      n'existe** (la tâche passe à vide, faussement verte).
  **Preuve** : CI entièrement verte sur une PR jetable.

- [ ] **Écrire les premiers tests Go de bout en bout** (au minimum) :
      inscription → onboarding → activation ; cloisonnement entre deux
      entreprises ; refus `409 onboarding_incomplete` ; plafonds de formule.
      Schéma dédié par test (`CREATE SCHEMA`) pour ne pas toucher aux données.
  **Preuve** : `go test ./...` non vide et au vert avec `DATABASE_URL`.

- [ ] **Résoudre `rendu-pages.mjs`** : échoue avec
      `Cannot find package 'esbuild'` (dépendance non déclarée) → l'ajouter
      en `devDependencies` ou corriger l'import.
  **Preuve** : `node scripts/verifications/rendu-pages.mjs` sort sans erreur.

- [ ] **Moteur de migrations numérotées** : aujourd'hui `0001` est figé dans
      le binaire et `0002` n'est jamais joué. Prévoir une table
      `schema_migrations` + application des fichiers non joués à chaque
      démarrage (le `TODO.md` et le `README.md` décrivent déjà ce
      fonctionnement : l'implémenter pour de vrai).
  **Preuve** : base vierge → `0001` puis `0002` enregistrés ; second
  démarrage = rien à rejouer.

- [ ] **Sessions** : harmoniser `SESSION_TTL_HOURS` (défaut **72 h** dans
      `main.go`) avec le README qui annonce **30 jours** — choisir une
      valeur, l'appliquer aux deux.
  **Preuve** : README et code disent la même chose.

## P2 — Dette de code

- [ ] **Découper les fichiers monolithes** : `routes_onboarding.go` (1 118 l.),
      `routes_stations.go` (1 048 l.), `routes_catalogue.go` (851 l.) — un
      fichier par domaine, sérialisation JSON regroupée.

- [ ] **Cohérence du routeur** : les routes sont déclarées de trois façons
      (`rt.ajouter`, `rt.proteger`, table `rt.routes` en littéral dans
      `routes_auth.go`) et le dispatch par préfixe
      (`/stations/{id}/…` via `dispatchStation`) ajoute une exception ;
      uniformiser ou documenter la règle dans `docs/api.md`.

- [ ] **`docs/api.md` et `README.md` à réaligner** sur les routes
      réellement enregistrées (le README annonce `make …`, `compose.yaml`,
      `migrations 0001 → 0012`, tous absents de la branche) — ou fournir
      ces fichiers. Signaler tout écart comme un bug (règle du dépôt).

---

## Ordre d'implémentation conseillé

```
1. P0 compilation (routes_equipe.go) + gofmt        → build vert
2. P0 routage Lot 3 + migration 0002                → tarifs et catalogue utilisables
3. Lot 4 équipe                                     → recruter l'équipe
4. Lot 5 lavages + caisse                           → cœur métier
5. Lot 6 rapports/dépenses/commissions              → pilotage
6. Lots 7-8 + espace laveur                         → complétude
7. Clerk (décision) + secrets + CI + tests          → exploitation sereine
```

## Journal

| Date | Constat |
| --- | --- |
| 14/10/2026 | Analyse initiale : build Go en échec sur `routes_equipe.go` (WIP), `enregistrerRoutesTarifs` non appelée, migration `0002` jamais jouée, 6 enregistreurs de lots vides, 0 test Go, tâche CI « sondes » sur scripts absents. |
| 14/10/2026 | **P0 (4 items) traité et vérifié** : compilation réparée, Lot 3 routé, `migrations/*.sql` jouées dans l'ordre (épreuve sur base vierge + second démarrage), `gofmt` en ordre. Preuves : `go build`, `go vet`, `gofmt -l`, serveur démarré sur `washguard_p0_check` (base jetable créée puis supprimée), contrôles `price-rules`=401 / `zzz`=404. |
| 14/10/2026 | **Lot 4 traité et vérifié** : `routes_equipe.go` réécrit selon schéma + contrat front ; mines découvertes en route (SQL sur colonnes inexistantes, invitation non acceptée, aperçu absent, barème non masqué, cache front périmé) corrigées. Preuve : scénario 14 étapes sur `washguard_lot4_check` (base jetable créée puis supprimée). |


