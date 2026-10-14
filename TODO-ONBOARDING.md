# TODO-ONBOARDING — mise en place de l'onboarding métier

> **Ce fichier est la feuille de route de l'onboarding.** Il est écrit pour être suivi,
> pas pour être lu une fois : chaque lot se termine par des vérifications réelles, et le
> **journal de progression** (§11) est mis à jour à chaque session.
>
> Complément : `TODO.md` (correctifs généraux), `docs/interface.md` (design system),
> `docs/api.md` (conventions d'API).

---

## 0. Comment on travaille avec cette liste

**Boucle de travail, une session = un lot (ou une partie de lot) :**

1. Lire le journal de progression (§11) et prendre **le premier lot non terminé**.
2. Vérifier l'état du dépôt : `git status`, `make check` (doit être vert **avant** de commencer).
3. Écrire la migration, le code, **et les tests prévus par le lot**, dans le même mouvement.
4. Vérifier : `make check`, `make test-e2e TEST_DSN=…`, `node scripts/verifications/rendu-pages.mjs`,
   et les sondes si l'API tourne (`make verifier-acces`).
5. Mettre à jour le journal (§11) et cocher les cases du lot. Commit local (pas de push).
6. Si un test révèle un défaut : le corriger **et** ajouter le cas de test qui l'a révélé.

**Règles non négociables (reprises du cahier des charges) :**

- Le **serveur** est la source de vérité : le front n'active jamais une station, ne déclare
  jamais une étape terminée, ne contourne jamais une permission.
- **Isolation multi-entreprises** : tout filtre sur `organization_id` **de la session**, jamais
  sur une valeur fournie par le client ; un objet d'une autre entreprise répond `404`.
- **Aucune donnée fictive** : pas de prix « officiels » inventés, pas de statut vérifié sans
  vérification, `SEED_DEMO=false` sur la base réelle. Un chiffre d'affaires nul s'affiche « aucune
  commande enregistrée », jamais un exemple.
- **Rien n'est supprimé** : désactivation logique (`is_active`), historiques conservés.
- **Pas de big-bang** : chaque lot est livrable et vérifiable seul, sans casser l'existant.
- Toute évolution de schéma passe par une **migration numérotée** (`backend/migrations/0004_…`,
  `0005_…`) appliquée au démarrage et testée sur base neuve **et** sur base existante.

**Définition de « fait » pour une tâche :** code + migration (si nécessaire) + test qui échoue
avant / passe après + `make check` vert + ligne du journal mise à jour.

---

## 1. Audit express du dépôt (constaté, pas supposé)

| Constat | Preuve |
| --- | --- |
| L'inscription crée **tout d'un coup** : organisation, 1 station, 3 prestations, abonnement d'essai, compte `owner`, session | `backend/handlers_register.go` |
| Les 3 prestations d'inscription portent des prix **inventés** : « Lavage simple » 2 000, « Lavage complet » 5 000, « Polissage » 10 000 FCFA | `backend/handlers_register.go` (boucle catalogue) |
| **Aucune notion d'onboarding** dans le dépôt | `grep -rl onboarding src backend docs` → aucun fichier |
| Après inscription, l'utilisateur arrive **directement** au tableau de bord | `src/components/auth/RegisterForm.tsx:80` (`navigate('/tableau-de-bord')`) |
| `vehicle_categories` existe mais **est vide et n'est jamais alimentée** → l'écran « nouveau lavage » n'a aucune catégorie à proposer | 0 ligne en base (vérifié), aucun `INSERT` dans `backend/migrations/*.sql` ni `backend/seed.go` |
| Écrans déjà en place : tableau de bord, lavages, prestations, clients, employés, caisse, dépenses, rapports, commissions, paramètres, abonnement, poste laveur, mon compte, administration (18 routes front) | `src/App.tsx` |
| 70 routes API, 5 rôles fixes, 35 permissions | `backend/main.go`, `backend/app.go` (`rolePermissions`), `users.role CHECK IN (superadmin, owner, manager, cashier, washer)` |
| Référentiels demandés **absents** : usages de véhicules, formules/options, compatibilités véhicule↔prestation, grille tarifaire (seul `services.price_xof`), moyens de paiement par station (méthode figée : `especes|mobile_money|carte|virement|autre`), équipements, stock, invitations, registre légal (RCCM/CC/CNPS), horaires structurés | `backend/migrations/0001_schema_initial.sql`, `payment_methods` absent, `invitations` absent |
| Déjà présents et réutilisables : `stations.opening_hours` (JSONB), `stations.settings` (règle caisse, commissions, objectif), `organizations` (adresse, ville, pays, devise, fuseau, logo, `status`), `plans` (limites + `features`), `audit_logs`, `cash_sessions`, `expenses`, `monthly_goals` | `0001_schema_initial.sql`, `stations_settings.go`, `plan_limits.go` |
| Écart à corriger au passage : la formule d'essai annonce « deux utilisateurs » alors que `plans.max_employees` vaut 5 par défaut | `handlers_register.go` (`'plan-essai'`) vs `plans.max_employees DEFAULT 5` |
| Base réelle sans données de démonstration (`SEED_DEMO=false`) : tout doit fonctionner sur une base vierge | `.env`, `scripts/verifications/verifier-env.sh` |

**Ce que l'on fait de cet audit :** on **complète** l'existant (mêmes conventions : Go + pgx,
migrations numérotées, JSONB seulement pour le descriptif, permissions centralisées, tests e2e),
on **corrige** les deux écarts ci-dessus, on **n'invente aucun prix**.

---

## 2. Décisions structurantes (à valider, hypothèses documentées)

| # | Décision | Raison |
| --- | --- | --- |
| D1 | **Nom du produit : WashGuard**, partout (code, interface, README, docs). **Décidé le 2026-10-14** : aucun renommage, le terme « PRESSPRO » du cahier des charges n'est pas repris. | Le nom est déjà celui de l'application livrée aux stations ; un renommage n'apporterait aucune fonctionnalité. |
| D2 | **État d'onboarding** : colonne `organizations.onboarding_state` (`pending`, `in_progress`, `completed`, `active`) + table `onboarding_steps` (une ligne par étape : `state` ∈ `not_started|in_progress|completed|needs_fix|blocked`, `data` JSONB **descriptif seulement**, `completed_at`, `updated_by`). | Évite de dupliquer un état entre user/station/abonnement, reste lisible dans l'écran Plateforme, et garde le relationnel dans des tables. |
| D3 | **Référentiels partagés + copie par station** : `vehicle_category_templates`, `vehicle_usage_templates`, `service_family_templates` (modèles versionnés, amorcés une fois) → `station_vehicle_categories`, `station_vehicle_usages`, `service_families` (par station). | « Une modification par une station ne doit jamais altérer une autre station », et l'idempotence de l'amorçage est exigée (Phase 5). |
| D4 | **Prestations** : on garde la table `services` (les lignes de fiche `wash_order_items` la référencent) ; on ajoute `service_families`, `service_options` (options/suppléments), `service_compatibilities` (prestation ↔ catégorie), `service_durations` seulement si nécessaire. | Continuité : aucune rupture des fiches existantes. |
| D5 | **Prix** : moteur unique côté serveur `backend/pricing.go` → `resolvePrice(service, category, usage, client, date)` avec la hiérarchie contractuelle > prestation+catégorie+usage > prestation+catégorie > tarif générique (`services.price_xof`, niveau 4 seulement si la station l'autorise). Table `price_rules` avec **unicité partielle** empêchant deux règles actives identiques (conflit signalé, jamais de choix silencieux). | Un seul endroit à tester ; la règle devient vérifiable, pas implicite. |
| D6 | **Prix absent ≠ prix zéro** : vente refusée (`409 price_required`) s'il n'existe aucun tarif valide, sauf prestation explicitement marquée gratuite (permission `service:manage`). | Évite de vendre à 0 par accident. |
| D7 | **Moyens de paiement** : nouvelle table `payment_methods` (par station : code, libellé, famille, actif, référence obligatoire) + `payments.payment_method_id` nullable ; `payments.method` (5 valeurs) **reste** la colonne qui pilote la caisse. | Pas de migration destructive, et les libellés locaux (Orange Money, Wave, MTN MoMo, Moov) deviennent configurables. |
| D8 | **Horaires** : table `station_opening_hours` (une ligne par jour : ouverture, fermeture, pause, dernier retrait, réservation autorisée) alimentée depuis `stations.opening_hours` ; le JSONB n'est plus édité. | Des horaires à valider (chevauchements, jours fermés) demandent des contraintes, pas du JSON libre. |
| D9 | **Rejoindre un espace** : table `invitations` (token **haché**, destinataire e-mail **ou** téléphone, code court lisible, rôle invité, station, expiration, `accepted_at`) ; inscription via `?invitation=CODE` → rattachement à l'entreprise invitante au lieu d'en créer une. Refus si invitation expirée/utilisée ; jamais de rattachement par simple identifiant. **Limite V1 documentée** : un compte appartient à un seul espace (pas de table `memberships` en V1). | Le modèle actuel porte `users.organization_id` unique : le multi-espaces est un chantier à part (lot V2). |
| D9 bis | **Décidé le 2026-10-14 : invitations par e-mail *et* téléphone, sans envoi automatique.** Le dépôt n'embarque **aucun** service d'envoi (ni SMTP, ni SMS, ni API WhatsApp) : le gérant crée l'invitation, copie le lien (ou le code à 8 caractères) et le partage lui-même — un bouton **Partager sur WhatsApp** prépare le message. Aucun e-mail n'est présenté comme envoyé s'il ne l'est pas. | Honnêteté de l'interface et zéro dépendance externe non configurée ; WhatsApp est le canal réellement utilisé en Côte d'Ivoire. |
| D10 | **Rôles : les 5 rôles actuels en V1** (`owner`, `manager`, `cashier`, `washer`, `superadmin`) — **décidé le 2026-10-14**. « Superviseur » ≈ `manager` rattaché à une station ; « responsable de stock », « comptable » et les rôles personnalisés demandent des rôles dynamiques → **V2** (`roles`, `role_permissions`). | Ne pas casser `rolePermissions` (source unique, lue par le front et le serveur). |
| D11 | **Abonnement** : l'onboarding ne crée pas de moteur concurrent ; il **lit** `plans`/`plan_limits.go`. Aucune donnée catalogue n'est supprimée lors d'un changement de formule : seules les *créations* sont limitées. | Exigence du cahier des charges. |
| D12 | **Catalogue de départ** : les prestations et catégories amorcées sont des **modèles à tarifer**, jamais des prix. | « Ne préremplis pas arbitrairement les prix comme s'il s'agissait de prix nationaux officiels. » |

---

## 3. Périmètre

**V1 — « onboarding opérationnel » = lots 0 à 8 (décidé le 2026-10-14 : on va jusqu'au bout de ces
lots avant toute mise en service)** : inscription → espace (créer ou rejoindre) →
identité de l'entreprise → station (localisation, caractéristiques, horaires) → catégories de
véhicules et usages → prestations et compatibilités → grille tarifaire et moteur de prix →
caisse et moyens de paiement → récapitulatif, prérequis, activation → tableau de bord alimenté par
les données réelles. Plus : personnel (réutilise l'écran existant), équipements et stock initial en
version simple.

**V2 — après V1** : rôles personnalisés et permissions fines, flottes/contrats/tarifs négociés et
plafonds de crédit, mouvements de stock complets et consommation par prestation, promotions,
reçus imprimés et mentions fiscales, photos avant/après, multi-espaces par compte.

---

## 4. Lots de travail

Légende : `[ ]` à faire · `[~]` en cours · `[x]` fait · `[!]` bloqué (voir §10)

### Lot 0 — Socle : état d'onboarding, garde de navigation, base de travail

- [x] **ONB-0.1** Migration `0004_onboarding.sql` : `organizations.onboarding_state` (défaut
      `pending`, colonnes de suivi `onboarding_started_at`, `onboarding_completed_at`),
      table `onboarding_steps` (`organization_id`, `step_code`, `state`, `data JSONB`,
      `completed_at`, `updated_by`, `updated_at`, clé primaire `(organization_id, step_code)`),
      index sur `organization_id`. Les entreprises existantes passent en `active`
      (elles exploitent déjà : on ne les renvoie pas dans un tunnel).
- [x] **ONB-0.2** `backend/onboarding.go` : liste des étapes (code, titre, obligatoire oui/non,
      dépendances), calcul de l'avancement **à partir des données réellement enregistrées**
      (jamais d'un simple clic), et `missingRequirements(ctx, orgID, stationID)`.
- [x] **ONB-0.3** `GET /api/v1/onboarding` : état, étapes, avancement, éléments obligatoires
      manquants. Ajout du bloc `onboarding` dans `GET /auth/me` (`backend/helpers.go`).
- [x] **ONB-0.4** `PATCH /api/v1/onboarding/steps/{code}` : enregistre une étape (validation
      serveur, upsert), refuse un code inconnu, journalise (`audit_logs`).
- [x] **ONB-0.5** Middleware `requireOnboarding` appliqué aux écritures d'exploitation
      (créer une fiche, encaisser, ouvrir une caisse, enregistrer une dépense) : une station non
      activée ne peut pas vendre ; le message indique l'étape à compléter.
- [x] **ONB-0.6** Front : route `/onboarding` (+ `OnboardingLayout` sans navigation latérale),
      écran d'accueil du parcours, `RequireOnboarding` qui redirige un compte `owner`
      incomplet vers `/onboarding` (l'inscription ne va plus directement au tableau de bord).
- [x] **ONB-0.7** Tests : `e2e_test.go` — inscription → état `onboarding_pending` → `/auth/me`
      expose l'étape 1 ; un `cashier` n'est pas redirigé ; entreprise existante = `active` ;
      une écriture d'exploitation est refusée tant que la station n'est pas activée.
- [x] **ONB-0.8** `scripts/verifications/rendu-pages.mjs` : cas `/onboarding` ; `coherence-routes.mjs`
      couvre la nouvelle route.

*DoD : un compte neuf est arrêté à l'onboarding, reprend là où il s'est arrêté après reconnexion,
et aucune vente n'est possible avant activation. Effort ≈ 1 j.*

**Livré (voir §11, entrée du 2026-10-14 « Lot 0 »)** — écarts assumés par rapport au plan :

- l'écran s'appelle « Mise en route » et s'intègre dans la coquille habituelle (barre latérale
  réduite aux écrans utiles) plutôt qu'une mise en page sans navigation : une page isolée
  empêchait de compléter prestations et tarifs, qui vivent dans « Prestations » ;
- le middleware s'appelle `requireOnboarding` et protège cinq écritures (fiche, encaissement de
  fiche, ouverture de caisse, paiement direct, dépense) ;
- `PATCH /onboarding/steps/{code}` refuse `state=completed` : seule la base décide qu'une étape
  est faite (règle « aucune donnée déclarative ») ; il enregistre des choix descriptifs ;
- l'activation (`POST /onboarding/activate`) est réservée à `organization:manage` (propriétaire) ;
- la migration marque les entreprises existantes `active` **sans** écraser les choix déjà
  enregistrés dans `onboarding_steps` (test dédié).

### Lot 1 — Étape 1 : créer un espace ou rejoindre une entreprise

- [x] **ONB-1.1** Migration `0007_invitations.sql` : `invitations` (id, `organization_id`,
      `station_id`, `email`, `phone` — **au moins l'un des deux** —, `role`, `token_hash` unique,
      `code` court unique (8 caractères, saisissable au téléphone), `expires_at`, `invited_by`,
      `accepted_at`, `accepted_by`, `revoked_at`, `created_at`) + index. Rôles autorisés :
      `manager`, `cashier`, `washer` (**jamais** `owner` par invitation).
- [x] **ONB-1.2** `POST /api/v1/invitations` (permission `employee:manage`), `GET /api/v1/invitations`
      (liste de l'entreprise), `POST /api/v1/invitations/{id}/revoke`.
- [x] **ONB-1.3** `GET /api/v1/invitations/{code}` (**sans session**) : renvoie uniquement le nom
      de l'entreprise, la ville et le rôle proposé — jamais son identifiant interne, jamais ses
      données. Le code court et le jeton long désignent la même invitation.
- [x] **ONB-1.4** `POST /api/v1/auth/register` accepte `invitation_token` : si présent, le compte
      rejoint l'entreprise invitante (rôle et station de l'invitation), **aucune** organisation ni
      station n'est créée, l'abonnement de l'entreprise n'est pas touché. Si le compte existe déjà
      → `409 invitation_requires_login`.
- [x] **ONB-1.5** Écran `/rejoindre` : accepter un code (saisie manuelle) ou un lien
      (`/rejoindre?code=…`), afficher l'entreprise vérifiée, proposer de se connecter ou de créer un
      compte, puis accepter l'invitation. La page explique quoi faire si le code est expiré.
- [x] **ONB-1.6** Écran Étape 1 de l'onboarding : deux parcours explicites (« créer mon entreprise »
      / « j'ai reçu une invitation »), confirmation du choix, mention que le compte devient
      propriétaire (parcours A).
- [x] **ONB-1.7** Écran Réglages → onglet **Invitations** : liste (destinataire, rôle, état,
      expiration), création (e-mail **ou** téléphone + rôle), révocation, **copie du lien**, **copie
      du code** et bouton **Partager sur WhatsApp** (`https://wa.me/?text=…`) préremplissant le
      message. Mention explicite : « WashGuard n'envoie pas le message à votre place ».
- [x] **ONB-1.8** Tests : inscription par invitation → `owner` de l'entreprise invitante intact,
      rôle appliqué, invitation non réutilisable ; invitation par **téléphone** acceptée comme celle
      par e-mail ; inscription sans invitation → entreprise créée ; une invitation expirée/révoquée
      échoue ; un code inventé ne révèle aucune entreprise ; aucun rattachement possible en devinant
      un identifiant ; un utilisateur déjà rattaché reçoit un refus explicite ; aucun e-mail n'est
      annoncé dans la réponse de l'API.

*DoD : on peut inviter un collègue ; l'invité rejoint l'entreprise sans devenir propriétaire.
Effort ≈ 1,5 j.*

**Livré** — migration `0007_invitations.sql` (et non `0005_`, déjà prise par le profil d'entreprise).
Fichiers : `backend/invitations.go`, `backend/handlers_register.go` (branche
`invitation_token`), `backend/plan_limits.go` (`checkPlanLimitForOrg`, sans session),
`backend/main.go` (4 routes), `src/pages/JoinPage.tsx`,
`src/features/invitations/{api.ts,InvitationsPanel.tsx}`, `src/App.tsx` (route publique
`/rejoindre`), `src/pages/SettingsPage.tsx` (onglet **Invitations**),
`src/pages/OnboardingPage.tsx` (les deux parcours de l'étape « espace »),
`scripts/verifications/sonde-invitations.py` (nouvelle).

### Lot 2 — Étape 2 : identité commerciale, administrative et registre légal

- [x] **ONB-2.1** Migration `0005_organization_profile.sql` : colonnes sur `organizations`
      (`legal_name`, `trade_name`, `activity_type`, `activity_description`, `activity_started_on`,
      `whatsapp`, `website`, `billing_name`, `billing_address`, `billing_email`, `billing_phone`,
      `legal_form`, `legal_representative`, `rccm`, `tax_id`, `cnps_employer_number`,
      `fiscal_regime`, `legal_mentions`) — toutes nullables ou avec défaut vide.
- [x] **ONB-2.2** Table `organization_registry_fields` (`organization_id`, `field_code`, `value`,
      `status` ∈ `empty|provided|pending_review|verified`, `verified_by`, `verified_at`,
      `updated_at`) : chaque information administrative garde **son** état ; la saisie ne vaut
      jamais vérification.
- [x] **ONB-2.3** `PATCH /api/v1/organizations/current` étendu (permission `settings:manage`) :
      identité commerciale + profil juridique ; `field_code` inconnu refusé ; passage en
      `verified` réservé au support (`platform:manage`) avec trace dans `audit_logs`.
- [x] **ONB-2.4** Étape 2 du parcours : sections « Identité commerciale » (types d'activité
      proposés : lavage manuel, automatique, haute pression, nettoyage intérieur, detailing, mixte,
      autre personnalisé), « Profil juridique », « Facturation », avec badge d'état par champ
      (non renseigné / renseigné / en attente / vérifié) et aide contextuelle.
- [x] **ONB-2.5** Écran Réglages → onglet **Entreprise** : mêmes champs hors parcours (l'écran
      actuel ne couvre que l'identité de la station).
- [x] **ONB-2.6** Tests : les champs juridiques sont facultatifs ; un `manager` ne peut pas marquer
      un RCCM « vérifié » ; les valeurs sont bien persistées et relues ; deux entreprises ne
      partagent aucun champ.

*DoD : la fiche d'entreprise est complète, chaque mention administrative porte son état, rien n'est
présenté comme vérifié sans l'être. Effort ≈ 1 j.*

**Livré** — écart de numérotation assumé : la migration s'appelle
`0005_organization_profile.sql` (et non `0006_`) parce que la mise en route a déjà consommé `0004`.
Formulaires : `src/features/onboarding/forms/EntrepriseForm.tsx` (parcours) et onglet
**Entreprise** de `src/pages/SettingsPage.tsx` (hors parcours) — un seul composant, pas deux
copies à maintenir.

### Lot 3 — Étape 3 : station (localisation, caractéristiques, horaires)

- [x] **ONB-3.1** Migration `0006_station_profile.sql` : colonnes sur `stations` (`country`,
      `district`, `commune`, `district_area`/`quartier`, `street`, `landmark`, `latitude`,
      `longitude` en `NUMERIC`, `billing_address`) ; table `station_site_profile` (type de lavage,
      pistes, véhicules simultanés, aspiration, séchage, detailing, salle d'attente, stockage,
      source d'eau, réserve d'eau, alimentation électrique, groupe électrogène, notes, contraintes).
- [x] **ONB-3.2** Table `station_opening_hours` (une ligne par jour, `is_open`, `opens_at`,
      `closes_at`, `break_start`, `break_end`, `last_checkin_at`, `booking_allowed`) + reprise des
      données existantes de `stations.opening_hours`, puis table de référence pour les journées
      exceptionnelles (`station_opening_exceptions`).
- [x] **ONB-3.3** Validation serveur des horaires : heures `HH:MM`, `closes_at > opens_at`, pause
      comprise dans la plage, jour fermé sans horaires, chevauchement d'exceptions refusé.
- [x] **ONB-3.4** `PATCH /api/v1/stations/{id}` étendu + `PUT /api/v1/stations/{id}/opening-hours`.
- [x] **ONB-3.5** Étape 3 du parcours : localisation (pays `Côte d'Ivoire`, devise `XOF`, langue
      `fr`, fuseau `Africa/Abidjan` par défaut, modifiables), caractéristiques du site, tableau des
      horaires (7 jours, copie « appliquer à tous »), distinction visuelle entre **capacité
      déclarée** et **statistiques calculées** (note explicite dans l'écran).
- [ ] **ONB-3.6** Le tableau de bord affiche le statut d'ouverture calculé à partir des horaires
      réels (et « fermé aujourd'hui » sinon). **Reste à faire** : les horaires sont enregistrés et
      relus (`GET /stations/{id}/profile`), mais aucun écran n'affiche encore « ouvert / fermé
      maintenant » — le tableau de bord ne connaît que l'état de la caisse.
- [x] **ONB-3.7** Tests : horaires invalides refusés (fin avant début, pause hors plage), jours
      fermés conservés, deux stations de la même entreprise ont des horaires indépendants, un
      `cashier` rattaché à une autre station ne peut pas les modifier.

*DoD : une station décrit où elle est, ce qu'elle peut faire physiquement et quand elle ouvre.
Effort ≈ 2 j.*

**Livré** — migration `0006_station_profile.sql` (et non `0007_`). Formulaires :
`src/features/onboarding/forms/StationForm.tsx` (localisation + capacités à trois états) et
`HorairesForm.tsx` (sept jours, copie du lundi, journées particulières, refus affichés par jour),
également présents dans les onglets **Station** et **Horaires** des Paramètres.
`ONB-3.6` **n'est pas livré** (voir la case) : il reste à brancher le statut d'ouverture sur le
tableau de bord.

### Lot 4 — Étapes 4 et 5 : catégories de véhicules et usages commerciaux

- [x] **ONB-4.1** Migration `0008_vehicle_catalogue.sql` :
      `vehicle_category_templates` (code unique, nom, famille physique, gabarit tarifaire,
      description, ordre, alias `TEXT[]`, modèle de durée),
      `station_vehicle_categories` (`station_id`, `template_code`, `name` (personnalisable), `is_active`,
      `sort_order`, `aliases`),
      `vehicle_usage_templates` (code, nom, famille, description, ordre, alias locaux) et
      `station_vehicle_usages` (même logique d'activation).
      **La catégorie physique et l'usage sont deux référentiels distincts** (`vehicle_categories`
      actuelle reste la table de référence lue par les fiches ; migration douce).
- [x] **ONB-4.2** Amorçage idempotent du catalogue de référence (`ON CONFLICT (code) DO UPDATE` —
      rejouable sans doublon, et une correction de libellé arrive dans les bases déjà installées ;
      voir l'écart 1 de la note de livraison) : véhicules légers, SUV/4x4/utilitaires légers, deux-roues et
      assimilés, transport collectif, poids lourds.
- [x] **ONB-4.3** Amorçage idempotent des usages : voiture personnelle, taxi compteur, taxi
      communal (wôrô-wôrô), taxi intercommunal, interurbain, taxi-brousse, gbaka, VTC, moto-taxi,
      livraison, transport de marchandises, entreprise, scolaire, location, administratif,
      touristique, flotte, personnalisé.
- [x] **ONB-4.4** API : `GET /vehicle-categories` (les catégories **de la station**, plus la liste
      vide = état honnête), `POST|PATCH /vehicle-categories` (création personnalisée,
      activation/désactivation), idem `vehicle-usages`. Une désactivation **ne touche pas** les
      fiches passées.
- [~] **ONB-4.5** Étape 4 : choisir/activer/désactiver des catégories, en créer, renommer,
      réordonner, avec recherche. Étape 5 : activer les usages et, pour chaque couple
      (catégorie, usage) proposé, associer les prestations applicables et un tarif spécifique
      éventuel — **une berline « taxi compteur » reste une berline**.
- [x] **ONB-4.6** `POST /wash-orders` accepte `vehicle_usage_id` ; la fiche conserve
      `vehicle_category_name` **et** `vehicle_usage_name` (historique lisible même si le
      référentiel change ensuite).
- [x] **ONB-4.7** Tests : `vehicle_categories` non vide après amorçage rejoué deux fois ; créer une
      catégorie personnalisée ; désactiver une catégorie puis relire une ancienne fiche (nom
      conservé) ; un usage n'est pas une catégorie ; isolation entre entreprises.

*DoD : la caisse peut choisir une catégorie **et** un usage, sans multiplier les catégories.
Effort ≈ 2 j.*

**Livré** — migration `0008_vehicle_catalogue.sql` (4 tables + reprise des anciennes catégories
globales en catégories personnalisées de chaque station ; rien n'est supprimé). Référentiel amorcé
par `backend/vehicle_catalogue.go` à **chaque démarrage** : 13 catégories dans 5 familles
(véhicules légers, SUV/4x4/utilitaires légers, deux-roues, transport collectif, poids lourds) et
18 usages. Les écrans : `src/features/onboarding/forms/VehiculesForm.tsx` et `UsagesForm.tsx`
(recherche, activation, renommage local, réordonnancement, catégorie personnalisée, combinaisons
véhicule × usage), branchés sur les étapes 4 et 5 du parcours. Le poste de caisse reçoit les deux
listes d'un seul appel (`GET /vehicle-category-options`) et une fiche conserve
`vehicle_category_name` **et** `vehicle_usage_name`.

Trois écarts assumés, tous documentés :

1. **`ON CONFLICT … DO UPDATE` et non `DO NOTHING`** : le référentiel est réécrit à chaque
   démarrage, pour qu'une correction de libellé ou un modèle ajouté par une version suivante
   arrive dans les bases déjà installées. Ce qui appartient à l'exploitant (activation, nom local,
   ordre) vit dans les tables de station et n'est jamais touché par cet amorçage.
2. **La catégorie est devenue un prérequis d'activation** (`Required: true`) — c'est ce que décrit
   `ONB-8.2`, et sans cela l'étape 4 ne serait jamais montrée avant l'ouverture. Les usages, eux,
   restent **facultatifs** (le lavage se vend sans usage déclaré).
3. **`ONB-4.5` est partiel** : les combinaisons (usage × catégorie) sont enregistrées et la caisse
   ne propose que celles-là, mais **associer les prestations applicables et un tarif spécifique par
   couple appartient au lot 5** (`service_compatibilities`, `price_rules`) — inventer un prix ici
   aurait été contraire à la règle « aucun prix inventé ».

### Lot 5 — Étapes 6, 7 et 8 : prestations, options, formules et grille tarifaire

**Livré** (backend, écrans, tests, sonde). Décision structurante : *le catalogue se compose, la
grille tarifaire décide*. Un prix ne s'écrit donc **que** par `PUT /price-rules/matrix` (motif
obligatoire, audit `tarifs_catalogue`) — `POST /services` et `PATCH /services/{id}` refusent un
`price_xof` non nul. Un compte neuf reçoit un catalogue **équipé et non tarifé** : aucun montant
n'est inventé à la place de l'exploitant.

- [x] **ONB-5.1** Migration `0009_services_pricing.sql` : `service_family_templates`,
      `service_families` (par station, activation et renommage), `service_templates`,
      `services` enrichi (`family_code`, `code`, `template_code`, `includes`, `equipment_notes`,
      `safety_notes`, `preconditions`, `is_free`, `is_quote_only`), `service_compatibilities`
      (prestation ↔ catégorie, vide = tous les véhicules), `service_options` (option de station,
      de prestation ou de type de véhicule), `price_rules` (`service_id`, `category_code`,
      `usage_code`, `customer_id`, `price_xof`, `valid_from`, `valid_to`, `reason`, `is_active`).
- [x] **ONB-5.2** Amorçage idempotent du **catalogue de prestations modèles** : 5 familles,
      18 modèles (`lavage_exterieur` → `lavage_deux_roues`, `ceramique` sur devis), **tous sans
      prix** (`price_xof = 0`). `equiperCatalogueNeuf` les reprend dans la station à l'inscription
      (rejouable, jamais de doublon), sans jamais écrire de montant.
- [x] **ONB-5.3** Prix inventés retirés de `handlers_register.go` : le compte neuf part « à
      tarifer », et l'écran Prestations affiche cet état (`needs_price`). Le filtre
      `a_tarifer` de `GET /services` s'appuie désormais sur les **règles** (`serviceColumnsAvecPrix`),
      plus sur le seul `price_xof`.
- [x] **ONB-5.4** `backend/pricing.go` : `resolvePrice` + `prixResolu` (`price_xof`,
      `price_source`, `price_source_label`, `price_rule_id`). Sources : `client_contract` >
      `service_category_usage` > `service_category` | `service_usage` > `service_generic` ; `free`
      prime sur tout ; sinon `prixRequis` → `409 price_required`. Deux règles de même spécificité
      ⇒ `errPrixAmbigu` → `409 price_rule_conflict` (+ `price_rule_id`). Un discriminant absent rend
      la règle inapplicable. `allow_generic_price: false` retire le niveau 4.
- [x] **ONB-5.5** Calcul du prix d'une fiche dans le backend
      (`handlers_wash_order_create.go`) : prestation + suppléments d'options (par lavage) − remise
      plafonnée par rôle. Le front **n'envoie aucun montant** (`unit_price_xof` n'existe pas côté
      requête) ; chaque ligne enregistre `price_source` et le détail `options`. `PATCH
      /wash-orders/{id}` recalcule les lignes quand le véhicule ou l'usage change, et refuse la
      modification si la fiche est encaissée et que le total bougerait (`409 conflict`).
- [x] **ONB-5.6** API tarifs : `GET|POST /price-rules`, `PATCH|DELETE /price-rules/{id}` (DELETE =
      désactivation), `GET|PUT /price-rules/matrix`, `GET /price-preview`. Motif obligatoire sur
      toute écriture. `POST` refuse un doublon actif (`409 price_rule_conflict`) et renvoie la règle
      fautive. `PUT /price-rules/matrix` accepte `cells`, `usage_cells` et `generic_prices` — il
      enregistre **en bloc** : une case fautive annule tout. Case vidée = règle désactivée, jamais
      supprimée. `PATCH` : montant, période, activation, motif.
- [x] **ONB-5.7** Étape 6 « Prestations » (reprise d'un modèle, familles, compatibilités,
      gratuité, devis), Étape 7 « Options et suppléments » (`OptionsForm`), Étape 8 « Grille
      tarifaire » (`TarifsForm` : matrice véhicule × prestation, cellules par usage, tarif du
      catalogue, périodes, motif obligatoire, compteur « à tarifer »). **Aucune valeur d'exemple
      n'est enregistrée** : les cases vides affichent ce qui s'appliquera réellement.
- [x] **ONB-5.8** Écran **Prestations** réécrit sans champ de prix (le tarif se pose dans la
      grille, renvoi vers `/tarifs`), avec état par prestation (`gratuite`, `sur devis`,
      `à tarifer`, tarif du catalogue, « tarif par véhicule ») et modale des véhicules concernés.
      Nouvel écran **Tarifs** (`/tarifs`, menu + parcours de mise en route) qui **réutilise**
      `TarifsForm` et `OptionsForm` : un seul composant par formulaire, un seul comportement.
- [x] **ONB-5.9** Tests Go (`backend/e2e_pricing_test.go`, 5 cas) : catalogue sans prix, refus
      hors grille (création et modification), hiérarchie des quatre niveaux **dans l'ordre** (dont
      contrat client), provenance sur la fiche, conflit de règles, case vidée → repli du catalogue,
      grille sans motif refusée en bloc, options facturées par lavage, option inconnue refusée,
      gratuité, `allow_generic_price: false`, recalcul après changement de véhicule, isolation
      entre entreprises. Sonde d'acceptation `scripts/verifications/sonde-tarifs.py` (37 contrôles).
      Montants entiers en FCFA, aucun flottant.

*DoD : le propriétaire saisit ses vrais prix, une seule fois, et le serveur calcule toujours le même
prix pour les mêmes entrées. Effort ≈ 2,5 j.*

### Lot 6 — Étape 10 : caisse, moyens de paiement et reçus — **fait**

- [x] **ONB-6.1** Migration `0010_payment_methods.sql` : `payment_methods` (station, code, libellé,
      famille `cash|mobile_money|card|transfer|deferred|other`, `requires_reference`,
      `is_active`, `sort_order`), amorçage idempotent (espèces, Orange Money, MTN MoMo, Moov Money,
      Wave, carte bancaire, virement, paiement différé, autre personnalisé) ;
      `payments.payment_method_id` nullable.
- [x] **ONB-6.2** `GET|POST|PATCH /payment-methods` ; l'encaissement refuse un moyen désactivé
      (`409 payment_method_disabled`). Un moyen « différé » exige une autorisation explicite
      (permission `payment:create_deferred`, à défaut `settings:manage`) et n'est jamais présenté
      comme un encaissement automatique.
- [x] **ONB-6.3** Paramètres de caisse : nom de la caisse, station, fond initial, utilisateurs
      autorisés, droits d'ouverture/fermeture, règles de clôture, écarts, justification des
      corrections, remboursements/remises selon permissions (table `cash_registers` ou extension de
      `cash_sessions` si le besoin est déjà couvert — **vérifier avant de créer**).
- [x] **ONB-6.4** Identité des reçus : préfixe, numérotation, coordonnées, logo, mentions
      commerciales, mentions fiscales **configurées par une personne habilitée** (jamais inventées),
      format ; génération du justificatif numérique (impression papier = V2).
- [x] **ONB-6.5** Étape 10 du parcours : moyens acceptés, caisse, reçus, avec aperçu **étiqueté
      « exemple »**.
- [x] **ONB-6.6** Tests : un moyen désactivé est refusé ; les compteurs de caisse restent exacts
      après plusieurs moyens de paiement ; une annulation laisse une trace et recalcule le solde ;
      les mentions fiscales vides ne s'impriment pas.

*DoD : la station encaisse par les moyens qu'elle a déclarés, et la caisse reste juste.
Effort ≈ 1,5 j.*

*Réalisé — décisions prises en chemin :*

- **Pas de table `cash_registers`** : une station n'a qu'une caisse. Les besoins
  de l'ONB-6.3 (fond initial, rôles autorisés, tolérance d'écart, annulations,
  remises) sont couverts par `stations.settings`, avec des clés **préfixées
  `cash_`** et une route dédiée (`GET|PUT /stations/{id}/cash-settings`) qui
  valide au lieu de laisser écrire n'importe quel réglage. Aucune structure
  dupliquée.
- **La famille compte, le libellé se montre** : `payments.method` porte la
  famille (`especes`, `mobile_money`, `differe`…) et alimente les compteurs de
  caisse ; `payments.payment_method_name` recopie le libellé choisi par la
  station (« Orange Money CI »). Un moyen désactivé est refusé
  (`409 payment_method_disabled`), un moyen à référence ne s'encaisse pas sans
  elle (`400`, champ `reference` fautif).
- **Le dernier moyen actif ne se désactive pas** (`409`) : sans moyen, la caisse
  ne peut plus rien encaisser. Rien n'est jamais supprimé.
- **Le différé n'est pas un encaissement** : il figure sur la fiche (engagement
  du client) mais n'entre ni dans `paid_xof`, ni dans les totaux, ni dans la
  session de caisse, et ne solde donc pas la fiche. Il exige la permission
  `payment:create_deferred`.
- **Le reçu est numérique et n'invente rien** : numéro `PRÉFIXE-AAAAMMJJ-NNNN`
  attribué **une seule fois**, à la soldeté de la fiche (`GET|POST
  /wash-orders/{id}/receipt`, idempotent) ; les mentions non renseignées
  (identifiant fiscal, raison sociale, logo…) sont **listées** dans
  `missing_mentions` au lieu d'être imprimées vides. L'impression papier et le
  logo imprimé restent en V2.
- La sonde `scripts/verifications/sonde-fiche-payee.py` couvre l'ensemble
  (22/22) et `sonde-onboarding.py` vérifie la 10ᵉ étape et les paramètres de
  caisse (40/40).

### Lot 7 — Étapes 11, 12 et 13 : personnel, équipements, stock, règles de commande

- [ ] **ONB-7.1** Étape 11 : modèles de rôles **existants** (propriétaire, gérant, caissier,
      laveur) réutilisés depuis l'écran Employés ; informations facultatives par employé (fonction,
      station, date d'arrivée, statut, horaires, mode de rémunération, commission) — **les données
      salariales sont réservées** (`employee:manage` + permission dédiée pour la rémunération).
      Aucun calcul de paie : l'écran affiche des paramètres, pas des bulletins.
- [ ] **ONB-7.2** Migration `0011_equipment_stock.sql` : `equipment_templates` + `equipments`
      (nom, type, quantité, état, dates d'acquisition/maintenance, coût, observation) ;
      `stock_categories` + `stock_items` (nom, code interne, catégorie, unité, **`quantity_known`
      booléen**, quantité, coût unitaire, seuil d'alerte, fournisseur, emplacement, péremption,
      statut) ; `stock_movements` (entrée, sortie, inventaire, ajustement, perte).
- [ ] **ONB-7.3** Amorçage idempotent des modèles d'équipements et des catégories de consommables.
      **Distinction explicite** : stock non renseigné (`quantity_known = false`) ≠ stock nul ; un
      seuil d'alerte sans quantité connue n'affiche pas « rupture ».
- [ ] **ONB-7.4** Étape 12 du parcours : équipements (ajout guidé à partir des modèles) et stock
      initial (facultatif, « je ne sais pas encore » possible et conservé comme tel).
- [ ] **ONB-7.5** Étape 13 : règles de fonctionnement des commandes (réservations, clients
      occasionnels, affectation laveur/piste, file d'attente, contrôle qualité, observations,
      annulation et motif, paiements partiels, conditions de remise du véhicule) + transitions
      autorisées par rôle (`Reçue → En attente → En cours → Contrôle qualité → Prête → Livrée`,
      plus `annulée` et `en litige`), **vérifiées côté serveur**.
- [ ] **ONB-7.6** Une fiche ne peut pas être `payée` sans paiement enregistré, ni `livrée` parce
      qu'un utilisateur a coché une étape technique : règle appliquée dans le handler et testée.
- [ ] **ONB-7.7** Tests : transitions interdites refusées, stock inconnu ≠ zéro, équipement
      désactivé conservé dans l'historique, données salariales invisibles pour un caissier.

*DoD : l'équipe, le matériel et les règles de commande sont décrits sans mentir sur l'inconnu.
Effort ≈ 2 j.*

### Lot 8 — Récapitulatif, activation, tableau de bord alimenté — **fait**

- [x] **ONB-8.1** `GET /api/v1/onboarding/summary` : toutes les données saisies + la liste des
      éléments obligatoires manquants **avec le lien vers l'étape concernée**.
- [x] **ONB-8.2** `POST /api/v1/onboarding/activate` : refuse (`409 onboarding_incomplete`) si un
      prérequis manque, en listant précisément lesquels ; vérifie en une transaction : entreprise
      valide, autorisation (`organization:manage`), identité et localisation minimales, ≥ 1 catégorie
      active, ≥ 1 prestation active, ≥ 1 compatibilité valide, ≥ 1 tarif valide pour les prestations
      vendables, ≥ 1 moyen de paiement actif, horaires définis. Horaires, catégories, prestations,
      tarifs et moyens de paiement sont **obligatoires** ; juridique, stock, personnel et
      équipements restent facultatifs.
- [x] **ONB-8.3** Écran récapitulatif : chaque section avec son état, les éléments facultatifs
      restants marqués comme tels, bouton d'activation désactivé tant qu'un prérequis manque (mais
      c'est le serveur qui refuse).
- [x] **ONB-8.4** Tableau de bord alimenté par les données réelles : nom et localisation de la
      station, statut d'ouverture, nombre de catégories/prestations/tarifs actifs, raccourcis caisse
      /lavages/clients, avancement des réglages facultatifs, et **états honnêtes** avant la première
      vente (« Aucune commande enregistrée », « Le catalogue est prêt », « Il reste des paramètres
      facultatifs »). Aucun chiffre d'affaires, aucune productivité, aucune consommation inventés.
- [x] **ONB-8.5** Tableau de bord respectant les rôles : le caissier ne voit pas l'administration,
      le laveur ne voit aucun montant financier.
- [x] **ONB-8.6** Tests : activation refusée cas par cas (un test par prérequis), activation réussie
      puis écriture d'exploitation acceptée, tableau de bord sans commande qui n'affiche aucun
      montant fictif, rollback complet si une étape d'activation échoue.

*DoD : « activer » a un sens vérifiable, et l'écran d'arrivée raconte la vérité sur la station.
Effort ≈ 2 j.*

**Livré** : `GET /onboarding/summary` (récapitulatif complet, sans montant),
`POST /onboarding/activate` (transaction unique, refus nommé qui n'écrit rien,
idempotent, instantané `onboarding_snapshot` + ligne de journal dans la même
transaction, migration `0012`), bloc `setup` partagé par le récapitulatif et le
tableau de bord (`backend/dashboard_setup.go`), écran récapitulatif
(`/mise-en-route/recapitulatif`) et arrivée du tableau de bord alimentée par les
données réelles. Vérifié : `sonde-lot8.py` **75/75**, `go test ./...` **57/57**,
`rendu-pages.mjs` **18/18** (une page de plus), campagne d'acceptation complète
au vert, `make check` **5/5**.

### Lot 9 — Sécurité, isolation, documentation, clôture

- [ ] **ONB-9.1** Batterie de tests d'isolation (une table par entité nouvelle) : lecture,
      écriture, mise à jour, suppression logique refusées entre entreprises ; `organization_id`
      fabriqué côté front inopérant ; `manager` ne peut pas usurper `owner` ; caissier sans
      permission de prix.
- [ ] **ONB-9.2** `make db-reset && make migrate` sur base neuve, **et** application des migrations
      sur une base déjà peuplée (avec données) : les deux doivent passer.
- [ ] **ONB-9.3** Documentation : `docs/onboarding.md` (parcours, états, prérequis d'activation,
      référentiels, tarification, invitations), mise à jour de `docs/api.md` (nouvelles routes),
      `README.md` (parcours d'inscription réel) et `TODO.md` (état).
- [ ] **ONB-9.4** Parcours complet rejoué de bout en bout : inscription → espace → 13 étapes →
      activation → première fiche → tableau de bord, **sur base vierge**, avec captures de
      vérification (sondes + `rendu-pages.mjs`).
- [ ] **ONB-9.5** Relevé final : ce qui est opérationnel, ce qui a été testé, ce qui reste ouvert,
      hypothèses retenues, prochaines améliorations par priorité.

*Effort ≈ 1,5 j.*

---

## 5. Tests exigés (liste de contrôle)

| Famille | Cas | Lot |
| --- | --- | --- |
| Onboarding | inscription → redirection vers l'onboarding ; reprise après déconnexion ; sauvegarde d'étape ; récapitulatif incomplet refusé ; activation après prérequis ; activation non autorisée refusée | 0, 8 |
| Données métier | catégorie créée ; usage créé ; prestation créée ; compatibilité prestation/véhicule ; tarif créé et modifié ; sélection de tarif déterministe ; règle ambiguë détectée ; vente refusée sans tarif ; options et suppléments ; désactivation sans perte d'historique ; paramètres de caisse ; stock inconnu ≠ stock nul | 4, 5, 6, 7 |
| Sécurité | entreprise A ≠ entreprise B ; gérant ≠ propriétaire ; caissier sans droit de prix ; rattachement impossible sans invitation ; salaires réservés ; `tenant_id` du front inopérant ; limites de formule appliquées | 1, 9 |
| Technique | migrations sur base neuve et sur base existante ; tests Go ; `tsc -b` ; build front ; front ↔ backend réel ; erreurs réseau gérées ; aucun endpoint sensible sans authentification | 9 |

Les tests s'écrivent dans `backend/e2e_test.go` (14 tests existants à ne pas casser), et les écrans
sont couverts par `scripts/verifications/rendu-pages.mjs` + `coherence-routes.mjs` + `classes-ui.mjs`.

---

## 6. Critères d'acceptation finale (traduction vérifiable)

1. Un utilisateur s'inscrit, valide son compte et **commence l'onboarding** → lot 0.
2. Il crée un espace **ou** rejoint une entreprise selon ses autorisations → lot 1.
3. Il renseigne et sauvegarde les informations de sa station → lots 2 et 3.
4. Il choisit ses catégories de véhicules et ses usages → lot 4.
5. Il active, désactive et personnalise ces catégories → lot 4.
6. Il crée des prestations et définit leurs compatibilités → lot 5.
7. Il configure une grille tarifaire exploitable → lot 5.
8. Les règles de sélection des tarifs sont appliquées **par le backend** → lot 5 (`pricing.go`).
9. Il configure ses moyens de paiement et sa caisse → lot 6.
10. Il enregistre ses équipements et son stock s'il le souhaite → lot 7.
11. La progression survit à une déconnexion → lot 0 (`onboarding_steps`).
12. Les validations serveur empêchent les configurations incohérentes → lots 3, 5, 7, 8.
13. Les données sont réellement persistées dans PostgreSQL → tous les lots (tests e2e).
14. Le tableau de bord reflète les données réelles → lot 8.
15. Permissions et limites d'abonnement respectées → lots 0, 1, 9.
16. Isolation entre entreprises testée → lot 9.
17. Migrations et tests présents → lots 0 à 9.
18. Les opérations sensibles laissent une trace → lots 2, 6, 8 (`audit_logs`).
19. Aucune donnée d'exemple présentée comme réelle → lots 5, 6, 8 (étiquetage « exemple »).
20. La documentation explique configurer, tester et faire évoluer → lot 9.

---

## 7. Commandes de vérification (à exécuter à chaque lot)

```bash
make check                                     # formatage, compilation, tests, routes, écrans, config
make test-e2e TEST_DSN="$DATABASE_URL"         # tests de bout en bout sur PostgreSQL réel
make db-reset && make migrate                  # migrations sur base neuve
make verifier-acces                            # sondes d'acceptation (API démarrée)
node scripts/verifications/rendu-pages.mjs     # les écrans se rendent
node scripts/verifications/coherence-routes.mjs
node scripts/verifications/classes-ui.mjs
bash scripts/verifications/verifier-env.sh dev # configuration (n'affiche jamais un secret)
```

---

## 8. Livrables attendus à la fin de chaque lot

1. Migrations PostgreSQL numérotées (`backend/migrations/0004…`).
2. Endpoints Go + validations + contrôles d'autorisation, documentés dans `docs/api.md`.
3. Écrans React/TypeScript connectés à l'API réelle (aucun mock permanent), cohérents avec
   `docs/interface.md`.
4. Tests ajoutés (Go e2e + rendu des écrans) et **résultats réels** relevés.
5. Journal de progression (§11) mis à jour, écarts ouverts listés honnêtement.

---

## 9. Améliorations prévues après la V1 (classées)

1. **P1** — Rôles personnalisés et permissions fines (`roles`, `role_permissions`) : superviseur,
   responsable de stock, comptable.
2. **P1** — Flottes et contrats : tarifs négociés, facturation groupée, échéances, plafond de
   crédit, historique des montants dus.
3. **P1** — Stock avancé : règles de consommation par prestation, alertes de seuil, inventaires
   périodiques.
4. **P2** — Promotions et périodes commerciales, tarifs saisonniers.
5. **P2** — Reçus imprimés et exports fiscaux, mentions légales paramétrées.
6. **P2** — Photos avant/après et contrôle qualité avec pièces jointes.
7. **P2** — Multi-espaces par compte (`memberships`), changement d'espace actif.
8. **P3** — Cartographie des stations et itinéraires clients, réservation en ligne.

---

## 10. Risques et dépendances externes

| Risque / dépendance | Impact | Traitement |
| --- | --- | --- |
| Clerk en clés `test` et `APP_URL`/`CLERK_AUTHORIZED_PARTIES` vides | Le parcours réel d'inscription/connexion ne peut pas être validé | À faire côté utilisateur (`TODO.md` §1) ; l'onboarding est testé en parallèle par les e2e Go |
| Neon inaccessible depuis l'environnement de développement | Migrations et sondes non vérifiées sur la vraie base | Migrations testées sur PostgreSQL local ; à rejouer chez l'utilisateur |
| `SEED_DEMO=false` | Aucune donnée de démonstration pour tester à la main | Les tests créent leurs propres entreprises ; l'écran d'onboarding assume l'état vide |
| Les prix inventés de l'inscription disparaissent | Un compte neuf ne peut plus vendre sans tarifer (volontaire) | Prérequis d'activation (lot 8) + écran « à tarifer » clair (lot 5) |
| Volume de travail (≈ 17 j au total) | Risque d'abandon en cours de route | Lots de 1 à 2,5 jours, chacun livrable et vérifiable seul |

---

## 11. Journal de progression

| Lot | État | Date | Résultat vérifié |
| --- | --- | --- | --- |
| Cadrage | `[x]` fait | 2026-10-14 | Nom du produit (WashGuard), périmètre V1 (lots 0 → 8), 5 rôles, invitations e-mail + téléphone sans envoi automatique : tranchés et consignés §2 |
| 0 — Socle onboarding | `[x]` fait | 2026-10-14 | Migration `0004_onboarding` (2 colonnes + table `onboarding_steps` + activation des entreprises existantes) ; 3 routes (`GET /onboarding`, `PATCH /onboarding/steps/{code}`, `POST /onboarding/activate`) ; garde `requireOnboarding` sur 5 écritures ; écran « Mise en route » ; **24/24 tests Go** (5 nouveaux d'onboarding), **11/11 écrans rendus**, `make check` 5/5 |
| 1 — Créer/rejoindre un espace | `[x]` fait | 2026-10-14 | Livré une première fois puis **perdu par une remise à zéro du bac à sable** (commit `320fc76` introuvable) ; **réimplémenté et vérifié** : migration `0007_invitations.sql`, 4 routes, page `/rejoindre`, onglet Invitations ; **23/23** contrôles de `sonde-invitations.py`, 5 tests Go dédiés |
| 2 — Identité et registre légal | `[x]` fait | 2026-10-14 | Migration `0005_organization_profile.sql`, `backend/org_profile.go`, `PATCH`/`GET /organizations/current` (registre + états), 4 formulaires branchés ; **29/29** contrôles de la sonde `sonde-onboarding.py`, **23/23 tests Go**, `make check` 5/5 |
| 3 — Station et horaires | `[x]` fait (sauf ONB-3.6) | 2026-10-14 | Migration `0006_station_profile.sql`, `backend/station_profile.go`, `GET /stations/{id}/profile`, `PUT /stations/{id}/opening-hours`, `PATCH /stations/{id}` étendu, formulaires Station + Horaires ; refus **par jour**, isolation par station testée |
| 4 — Catégories et usages | `[x]` fait (voir la note du §Lot 4) | 2026-10-14 | Migration `0008_vehicle_catalogue.sql` (4 tables + reprise des anciennes catégories globales), référentiel amorcé à chaque démarrage (13 catégories / 5 familles, 18 usages), écrans « Catégories de véhicules » et « Usages commerciaux » branchés sur les étapes 4 et 5, combinaisons véhicule × usage, `vehicle_usage_id` sur les fiches ; **36/36** contrôles de `sonde-vehicules.py` (nouvelle sonde), **29/29 tests Go**, sondes **40/40 · 30/30 · 23/23 · 35/35 · 8/8 · 1/1**, `make check` 5/5 |
| 5 — Prestations et tarifs | `[x]` fait | 2026-10-14 | Migration `0009_services_pricing.sql`, 18 modèles de prestations, matrices prix × catégorie × usage, revue de prix par `PUT /price-rules/matrix` (motif obligatoire) ; **37/37** contrôles de `sonde-tarifs.py`, 7 tests Go dédiés |
| 6 — Caisse et paiements | `[x]` fait | 2026-10-14 | Migration `0010_payment_methods.sql` (9 moyens par station), encaissement, reçus numérotés, 11 réglages de caisse, étape 10 du parcours ; **22/22** contrôles de `sonde-fiche-payee.py`, 5 tests Go dédiés, `make check` 5/5 |
| 7 — Personnel, équipements, stock, commandes | `[x]` fait | 2026-10-14 | Migration `0011_equipment_stock.sql` (matériel, stock, mouvements, poste des employés) ; 8 familles de routes (`/stations/{id}/equipment`, `/equipment/{id}`, `/stations/{id}/stock`, `/stock/items/{id}` + mouvements, `/stations/{id}/order-rules`) ; cycle de vie à **7 états** imposé par le serveur avec motifs lisibles (`transition_forbidden`, `quality_check_required`, `release_requires_payment`, `washer_queue_full`, `walk_in_not_allowed`, `washer_required`) ; parcours à **13 étapes** (équipe, matériel, règles — facultatives) ; écrans « Équipe », « Matériel », « Règles des commandes » ; **56/56** contrôles de `sonde-lot7.py` (nouvelle sonde), **55/55 tests Go**, campagne complète **40/40 · 41/41 · 37/37 · 23/23 · 36/36 · 35/35 · 22/22 · 56/56 · 1/1**, `make check` 5/5, `verif-apercu.py` 15/15 |
| 8 — Récapitulatif et activation | `[ ]` à faire | — | — |
| 9 — Sécurité, tests, documentation | `[ ]` à faire | — | — |

**Point de départ (état du dépôt à la rédaction de cette liste)** : `make check` 5/5 vert,
19 tests Go (5 Clerk + 14 e2e) verts, sondes d'acceptation **38/38 · 35/35 · 8/8 · 1/1**, aucun
écran d'onboarding, `vehicle_categories` vide, prix d'inscription inventés (voir §1).

---

### 2026-10-14 — Lot 0 livré : socle de l'onboarding

**Fichiers** : `backend/migrations/0004_onboarding.sql`, `backend/onboarding.go` (catalogue, calcul
d'avancement, 3 handlers, middleware), `backend/main.go` (routes + garde), `backend/helpers.go`
(bloc `onboarding` dans `/auth/me`), `backend/handlers_register.go` (état initial `pending`, limites
de la formule d'essai alignées sur son texte : 1 station / 2 utilisateurs), `backend/seed.go`
(entreprise de démonstration `active`), `src/pages/OnboardingPage.tsx`,
`src/features/onboarding/api.ts`, `src/App.tsx`, `src/layouts/AppLayout.tsx`,
`src/components/auth/{LoginForm,RegisterForm}.tsx`, `src/types/index.ts`, `package.json`,
`scripts/verifications/rendu-pages.mjs`.

**Modèle retenu** : `organizations.onboarding_state ∈ {pending, in_progress, completed, active}` et
`onboarding_steps (organization_id, step_code)` avec `state`, `data JSONB`, `completed_at`,
`updated_by`. Une étape est « faite » **si et seulement si** les données existent : station active,
nom + téléphone + ville de l'entreprise, nom + ville de la station, au moins une prestation active,
au moins une prestation avec un prix > 0.

**Résultats mesurés** (bac à sable : Go 1.27.2, PostgreSQL 17 local) :

| Commande | Résultat |
| --- | --- |
| `make test-e2e TEST_DSN=…` | 24/24 tests verts (dont 5 nouveaux : état initial et reprise après reconnexion, refus d'écriture avant activation, activation réservée au propriétaire, caissier sans parcours, migration qui active les entreprises existantes sans écraser les choix) |
| `make check` | 5/5 (formatage/vet, « API ok / front ok », tests, routes + 11/11 écrans + 33 classes, configuration 0 avertissement) |
| `node scripts/verifications/rendu-pages.mjs` | 11/11 écrans (dont `/onboarding`), après réparation de la sonde (voir ci-dessous) |

**Deux faux verts corrigés au passage** (défauts réels du dépôt, pas du lot) :

1. `npm run typecheck` exécutait `tsc --noEmit` sans projet : le `tsconfig.json` racine ne liste
   aucun fichier, donc la commande ne vérifiait rien et réussissait toujours. Elle vise désormais
   `tsconfig.app.json` et `tsconfig.node.json` — et elle a immédiatement trouvé deux erreurs de
   type réelles dans le nouvel écran.
2. `scripts/verifications/rendu-pages.mjs` ne rendait chaque page qu'une fois : les écrans étant
   chargés à la demande (`React.lazy`), seule la coquille (barre latérale) était vérifiée — et
   celle-ci contient justement le nom des écrans, donc le contrôle validait un écran vide. Le
   rendu se fait maintenant en deux passes (les longueurs de HTML ont doublé pour six écrans, ce
   qui prouve que le contenu est enfin atteint). Les renvois `<Navigate>` restent hors de portée
   du rendu serveur : ils s'exécutent dans un effet React, jamais au rendu.

**Restant pour le lot 0** : rien. Le lot 1 (créer ou rejoindre un espace) peut démarrer.

### 2026-10-14 — Lots 2 et 3 livrés : les formulaires écrivent en base

**Fichiers** : `backend/migrations/0005_organization_profile.sql`, `0006_station_profile.sql`,
`backend/org_profile.go`, `backend/station_profile.go`, `backend/handlers_org.go`,
`backend/handlers_stations.go`, `backend/onboarding.go` (étape « horaires » prérequise + champ
`form` + constat des capacités facultatives), `backend/handlers_register.go` (plus de catalogue
inventé), `backend/handlers_wash_order_create.go` (`409 price_required`), `backend/seed.go`
(démo : identité + horaires), `backend/e2e_test.go` ;
`src/features/onboarding/forms/{Entreprise,Station,Horaires,Prestations}Form.tsx`,
`src/features/profile/api.ts`, `src/pages/OnboardingPage.tsx`, `src/pages/SettingsPage.tsx`
(onglets Entreprise / Station / Horaires), `src/types/index.ts`, `src/lib/api.ts`
(`error.fields` lu — c'est ce que le serveur renvoie),
`scripts/verifications/sonde-onboarding.py` (nouvelle), `vite.config.ts` (`VITE_ALLOWED_HOSTS`).

**Ce que la demande impliquait** : « brancher tous les formulaires de l'onboarding à la base de
données, pour l'organisation créée ». Concrètement : aucun état local, aucune maquette — chaque
enregistrement part vers l'API de l'entreprise de la session, et se relit par une autre route.

| Commande | Résultat mesuré |
| --- | --- |
| `go test ./... -count=1` (avec `WASHGUARD_TEST_DATABASE_URL`) | **23/23 tests verts**, `ok washguard/backend 8.197s` |
| `make check` | **5/5** (dont 11/11 écrans, 33 classes, configuration 0 avertissement) |
| `python3 scripts/verifications/sonde-onboarding.py` | **29/29** contrôles au vert |
| `sonde-api.py` · `sonde-metier.py` · `sonde-fiche-payee.py` · `sonde-suspension.py` | **40/40 · 35/35 · 8/8 · 1/1** (base neuve, `SEED_DEMO=true`) |
| `npm run typecheck` | 0 erreur (il ne vérifiait rien avant le lot 0 : voir ce journal) |

**Décisions prises pendant la mise en œuvre** :

1. **Les capacités sont à trois états, pas à deux.** `has_vacuum` et ses voisins sont `null`
   tant que la question n'a pas été posée : « pas d'aspiration » et « on ne sait pas encore » ne
   se confondent pas, et l'écran ne transforme jamais un silence en « non ».
2. **L'application ne propose aucun prix.** Le catalogue créé automatiquement à l'inscription
   (« Lavage simple 2000 ») a été **supprimé** : c'était un prix inventé, affiché sur une facture
   potentielle. C'est l'exploitant qui saisit son tarif ; sans tarif, la vente est refusée
   (`409 price_required`) et l'étape « Grille tarifaire » reste incomplète.
3. **Les horaires sont un prérequis d'activation** : sans journée d'ouverture déclarée, la
   station ne peut pas dire si elle est ouverte, et l'activation est refusée
   (`409 onboarding_incomplete`).
4. **Une mention administrative n'est jamais « vérifiée » par l'application.** Elle est
   « Renseignée » quand l'exploitant la saisit ; seul le support WashGuard peut l'attester
   (`POST /platform/organizations/{id}/registry/{code}/verify`), et **modifier la valeur annule la
   vérification** — sinon l'attestation ne voudrait plus rien dire.
5. **Un seul composant par formulaire**, partagé entre le parcours et les Paramètres : deux
   copies auraient divergé.

**Défauts trouvés par la sonde d'acceptation, et corrigés** (ils l'ont été parce que la sonde
relit ce qu'elle écrit ; trois d'entre eux étaient dans la sonde elle-même, deux dans le dépôt) :

1. `error.fields` : le serveur nomme les champs fautifs dans `error.fields`, le front lisait
   `error.details` — les erreurs par champ (horaires, dates) ne s'affichaient donc nulle part.
2. Les sondes d'acceptation existantes devenaient fausses sous les nouvelles règles (« catalogue
   de démarrage », « la victime peut créer une fiche ») : elles ont été réécrites pour **mettre en
   route** leurs entreprises, ce qui documente au passage le nouveau parcours.
3. Deux erreurs de ma sonde, corrigées après lecture du refus du serveur (une « dernière
   réception » hors des heures d'ouverture du samedi ; l'étape « tarifs » qui ouvre bien le
   formulaire des prestations). Le serveur avait raison dans les deux cas.

**Restant pour ces deux lots** : `ONB-3.6` (statut d'ouverture sur le tableau de bord). Le lot 4
(catégories de véhicules et usages) peut démarrer après réimplémentation du lot 1, perdu par la
remise à zéro du bac à sable.

### 2026-10-14 — Lot 1 (invitations) : perdu par une remise à zéro, puis réimplémenté

Le lot 1 avait été livré, vérifié (parcours d'acceptation réel : aperçu public d'une invitation,
inscription de l'invitée, réutilisation refusée) et commité (`320fc76`). Une remise à zéro de
l'environnement de travail a ramené le dépôt à `91a13e9` : **ce commit n'existe plus** et le code
des invitations a disparu du disque. Aucune trace de cette perte n'existe dans l'historique Git —
elle est consignée ici pour que l'état du dépôt ne laisse pas croire que le lot 1 a été oublié.

Le lot a donc été **réimplémenté à partir de la spécification** (`ONB-1.1` → `ONB-1.8`), avec deux
écarts assumés :

1. **Numérotation** : la migration est `0007_invitations.sql`, pas `0005_` (prise par le profil
   d'entreprise).
2. **Le jeton long n'est pas stocké** : seule son empreinte SHA-256 l'est. Une lecture de la base
   ne permet donc pas de rejoindre une entreprise, et le lien n'est affiché qu'une fois — l'écran le
   dit, et le code court reste consultable pour transmettre l'invitation autrement.

Le parcours d'acceptation est rejoué par `scripts/verifications/sonde-invitations.py` : **23/23**
contrôles au vert, dont l'absence de tout identifiant dans l'aperçu public et le refus explicite
d'un compte déjà rattaché (`409 invitation_requires_login`).

### 2026-10-14 — Défaut bloquant d'aperçu : « Vous devez vous connecter » après l'inscription

**Le symptôme, tel qu'il a été signalé** : après l'inscription et la connexion, la page de mise en
route s'affiche deux secondes puis laisse la place à « Mise en route indisponible ». Sur une
**première utilisation**, c'est-à-dire exactement le parcours que l'onboarding doit rendre possible.

**La cause, mesurée** : ce n'est pas l'onboarding qui échoue, c'est la session qui n'arrive jamais au
serveur. Un aperçu est affiché dans un cadre servi par **un autre domaine** que l'API : le cookie de
session y est un cookie **tiers**, et le navigateur ne le conserve pas. `GET /onboarding` répondait
donc `401`, l'écran affichait le message d'erreur — au lieu de laisser croire à une panne.

**Le correctif, et sa limite** : hors production **seulement** (`APP_ENV` différent de `production`,
tout est fermé par `backend/apercu_session.go`), la réponse d'authentification (`/auth/register`,
`/auth/login`, `/auth/clerk`, acceptation d'invitation) porte aussi `session_token` ; l'interface le
garde pour l'onglet et le présente en `Authorization: Bearer`. Deux défauts annexes trouvés en
vérifiant :

1. un compte de démonstration restait sans **moyen de paiement actif** (`equiperMoyensPaiement`
   n'était pas appelé après la création des stations par `seedDemo`) — la caisse neuve refusait donc
   tout encaissement ;
2. la **déconnexion** ne fermait que le cookie : présenté en en-tête, le jeton restait valable. Elle
   ferme désormais la session, quelle que soit la voie empruntée.

**Vérifié, chiffres réels** : `scripts/verifications/verif-apercu.py` (**nouvelle sonde**, sans aucun
cookie conservé) **15/15** ; `make check` **5/5** ; **46/46 tests Go** ; sondes d'acceptation
**40/40 · 40/40 · 37/37 · 23/23 · 36/36 · 35/35 · 22/22 · 1/1** ; les 14 écrans se rendent. En
**production**, contrôle explicite : `session_token` absent du corps, `Bearer` refusé (`401`), cookie
`HttpOnly` toujours accepté — le paquet compilé du front ne contient plus une seule occurrence du
mécanisme.

### 2026-10-14 — Lot 4 livré : catégories de véhicules et usages commerciaux

**Fichiers** : `backend/migrations/0008_vehicle_catalogue.sql`, `backend/vehicle_catalogue.go`,
`backend/handlers_vehicle_categories.go`, `src/features/vehicle-categories/`,
`src/features/onboarding/forms/{Vehicules,Usages}Form.tsx`, `scripts/verifications/sonde-vehicules.py`.

**Ce que le lot change** : une station déclare **ce qu'elle lave** (catégories, avec leurs
dimensions) et **pour quel usage** (ville, taxis, flotte…), au lieu d'un catalogue global partagé
par toutes les entreprises. Les combinaisons véhicule × usage deviennent la base des tarifs du lot
suivant, et les fiches portent `vehicle_usage_id` : un tarif « taxi » ne s'applique plus à une
berline de particulier.

**Vérifié, chiffres réels** : `sonde-vehicules.py` **36/36** ; **29/29 tests Go** ;
`make check` **5/5** (dont 14/14 écrans).

### 2026-10-14 — Lot 5 livré : prestations, options, formules et grille tarifaire

**Fichiers** : `backend/migrations/0009_services_pricing.sql`, `backend/pricing.go`,
`backend/price_rules.go`, `backend/service_catalogue.go`, `backend/handlers_services*.go`,
`src/features/services/`, `src/features/onboarding/forms/{Prestations,Options,Tarifs}Form.tsx`,
`scripts/verifications/sonde-tarifs.py`.

**Décision structurante, assumée** : **aucun prix n'est inventé**. Un compte neuf reçoit
dix-huit modèles de prestations **à tarifer** (`price_xof = 0`) ; tant que l'exploitant n'a pas posé
son prix, la vente est refusée par le serveur (`409 price_required`). La hiérarchie de résolution
est explicite : contrat de flotte > prestation + catégorie + usage > … > catalogue de la station.
Chaque revue de prix passe par `PUT /price-rules/matrix` avec un **motif** obligatoire : un tarif
modifié sans raison est un tarif qu'on ne peut pas expliquer au client.

**Vérifié, chiffres réels** : `sonde-tarifs.py` **37/37** ; 7 tests Go dédiés ;
`make check` **5/5**.

### 2026-10-14 — Lot 6 livré : caisse, moyens de paiement et reçus

**Fichiers** : `backend/migrations/0010_payment_methods.sql`, `backend/payment_methods.go`,
`backend/cash_settings.go`, `backend/handlers_payments_write.go`, `backend/handlers_receipts.go`,
`src/features/payments/`, `src/pages/CashPage.tsx`, `src/pages/SettingsPage.tsx` (onglet caisse),
`src/features/onboarding/forms/MoyensForm.tsx`.

**Ce que le lot change** : une station neuve naît avec **neuf moyens de paiement** rangés par
famille, seules les espèces actives — encaisser au premier jour sans configuration. Un moyen
désactivé est refusé par le serveur (`409 payment_method_disabled`), le dernier moyen actif ne peut
pas être retiré, et chaque encaissement produit un **reçu numéroté** rattaché au compteur de la
caisse. La session de caisse (ouverture, comptage, clôture) est un prérequis d'encaissement.

**Vérifié, chiffres réels** : `sonde-fiche-payee.py` **22/22** (dont le verrouillage d'une fiche
encaissée) ; 5 tests Go dédiés ; `make check` **5/5**.

### 2026-10-14 — Lot 7 livré : personnel, matériel, stock et règles de commande

**Fichiers** : `backend/migrations/0011_equipment_stock.sql`, `backend/equipments.go`,
`backend/stock.go`, `backend/order_rules.go`, `backend/employees_profile.go`,
`backend/handlers_employees*.go`, `backend/handlers_wash_order_{create,update,close}.go`,
`backend/handlers_payments_write.go`, `backend/onboarding.go` (13 étapes), `backend/app.go`
(permission `employee:compensation`), `backend/main.go` (10 routes), `backend/e2e_lot7_test.go`,
`backend/e2e_order_flow_test.go` ;
`src/features/onboarding/forms/{Equipe,Materiel,Commandes}Form.tsx`, `src/features/maintenance/api.ts`,
`src/features/wash-orders/WashOrderForm.tsx`, `src/features/washer/api.ts`,
`src/pages/WasherPortalPage.tsx`, `src/pages/EmployeesPage.tsx`, `src/pages/OnboardingPage.tsx`,
`src/lib/constants.ts` ;
`scripts/verifications/sonde-lot7.py` (nouvelle).

**Le principe du lot** : une règle affichée est une règle appliquée. Les sept réglages de la station
sont lus **là où le serveur décide** — création de la fiche, encaissement, changement de statut,
annulation — et chaque refus arrive avec un motif exploitable par l'écran, jamais un « erreur 500 » :

| Règle | Refus mesuré |
| --- | --- |
| Clients de passage refusés | `400 walk_in_not_allowed` |
| Laveur obligatoire à la création | `400 washer_required` |
| Plafond de fiches par laveur | `409 washer_queue_full` (libéré par l'annulation) |
| Contrôle qualité obligatoire | `409 quality_check_required` |
| Remise subordonnée au paiement | `409 payment_required` |
| Acomptes refusés | `409 partial_payment_disabled` (fiche **et** guichet) |
| Annulation motivée | `400 validation_error` sur le champ `reason` |

**Décisions prises pendant la mise en œuvre** :

1. **Sept états de cycle de vie, un seul graphe.** `pending`, `in_progress`, `quality_check`,
   `ready`, `completed`, `canceled`, `disputed` — exposés par `GET /stations/{id}/order-rules`
   (`statuses` + libellés) pour que l'écran n'invente pas ses propres mots. Une transition non
   prévue est refusée (`409 transition_forbidden`), et un état final le reste.
2. **Les étapes 11 à 13 sont facultatives.** Une station équipée d'un seau et d'une éponge doit
   pouvoir ouvrir : équipe, matériel et stock se déclarent après l'activation, et le parcours le
   dit (`required: false`).
3. **« Pas compté » n'est pas « zéro ».** Un article déclaré sans quantité porte
   `quantity_known: false` et l'état `inconnu` ; une sortie dessus est refusée
   (`409 stock_quantity_unknown`) — on ne retire pas d'un stock qu'on n'a pas compté. Une quantité
   ne s'écrit pas à la main (`409 stock_movement_required`) : elle bouge par un mouvement, et
   l'historique explique la quantité courante.
4. **La rémunération est une donnée à part.** Permission `employee:compensation` (propriétaire et
   support, **pas** gérant) : un caissier reçoit `403` sur toute écriture de barème, et les champs
   ne sont même pas renvoyés dans la liste. Les autres champs de la fiche de poste (intitulé,
   arrivée, horaires, situation RH) sont, eux, du ressort du gérant.
5. **Le congé n'est pas une sanction.** « En congé » est une situation RH : l'accès au portail
   reste ouvert, seul un compte désactivé ferme la porte.

**Défauts trouvés en vérifiant, et corrigés** :

1. **La garde d'acompte n'était branchée nulle part.** Avec `allow_partial_payments: false`, un
   acompte passait quand même (réponse `200`). Le contrôle est désormais une fonction unique
   (`verifierAcompte` dans `backend/order_rules.go`), appelée par les **deux** chemins
   d'encaissement : `POST /wash-orders/{id}/payments` et `POST /cash/payments`. Sans cette
   centralisation, corriger un chemin laissait l'autre ouvert.
2. **`stations` n'a pas de colonne `updated_at`.** Un `UPDATE stations … updated_at = now()` faisait
   échouer l'enregistrement des règles (`500`) ; la clause a été retirée plutôt que d'ajouter une
   colonne pour satisfaire une requête.
3. **Le portail laveur ne connaissait pas les nouveaux états.** Il ne proposait que « prendre en
   charge » puis « terminer » : le contrôle qualité et la clôture d'une fiche y sont maintenant
   accessibles, et le refus du serveur (fiche non encaissée) s'affiche tel quel.

**Vérifié, chiffres réels** : `sonde-lot7.py` **56/56** (nouvelle sonde, entreprise créée de bout en
bout, sans donnée de démonstration) ; `go test -count=1 ./...` **55/55 tests verts**,
`ok washguard/backend 44.800s` ; campagne d'acceptation complète **40/40 · 41/41 · 37/37 · 23/23 ·
36/36 · 35/35 · 22/22 · 56/56 · 1/1** ; `make check` **5/5** (14/14 écrans, 36 classes) ;
`verif-apercu.py` **15/15**. Deux sondes et un contrôle de l'aperçu ont été **mis à jour** pour la
nouvelle vérité du parcours (13 étapes) : ils échouaient sur l'ancienne (10 étapes), et c'est la
bonne façon de le découvrir.

**Reste ouvert, assumé** : le filtre `a_tarifer` de `handlers_services.go` est à réaligner ;
la limite par laveur est vérifiée à la **création** de la fiche (le contrat noté parlait de
l'affectation) — à trancher ; `SalarySection.tsx` attend huit champs RH qui n'existent pas dans
`users` (masqué par `SALARY_UI_ENABLED`), hors périmètre V1.


### 2026-10-14 — Lot 8 livré : récapitulatif, activation, tableau de bord alimenté

**Fichiers** : `backend/migrations/0012_onboarding_activation.sql` (nouvelle),
`backend/lecteur.go` (nouveau : lire depuis le pool **ou** depuis une
transaction), `backend/dashboard_setup.go` (nouveau : le bloc `setup` partagé),
`backend/onboarding.go` (récapitulatif, activation transactionnelle, instantané),
`backend/handlers_reports.go` (bloc `setup` dans le tableau de bord, retrait des
blocs financiers sans activité), `backend/http_json.go` (`writeErrorDetaillee`),
`backend/models.go` (traçabilité de la fiche : `quality_checked_at`, `ready_at`,
`delivered_at`), `backend/{order_rules,price_rules,handlers_wash_order_*,
handlers_payments_write,handlers_washer,handlers_services}.go` (contrôle qualité
non contournable, clôture en un seul endroit, poste laveur sans montant),
`backend/main.go` (route `GET /onboarding/summary`), `backend/e2e_lot8_test.go`
(nouveau, 6 tests) ;
`src/features/onboarding/{api.ts,SetupPanel.tsx}`, `src/pages/{ActivationPage.tsx,
OnboardingPage.tsx,DashboardPage.tsx}`, `src/App.tsx`, `src/types/index.ts` ;
`scripts/verifications/sonde-lot8.py` (nouvelle), `scripts/verifications/
rendu-pages.mjs` (une page de plus), `docs/api.md`, `Makefile`.

**Ce que le lot ajoute, en trois phrases** : le récapitulatif dit ce qui est prêt
et ce qui manque, **sans jamais afficher un montant** ; l'activation est une
transaction qui refuse en nommant ses obstacles et n'écrit **rien** quand elle
refuse ; le tableau de bord et le récapitulatif lisent **le même bloc `setup`**,
donc ils ne peuvent pas se contredire.

**Décisions prises pendant la mise en œuvre** :

1. **Un seul calcul, deux lecteurs.** `backend/dashboard_setup.go` compose le
   bloc `setup` à partir de `etapesOnboardingAvec` — les mêmes contrôles que
   l'activation. Le tableau de bord ne rejuge rien : il met en forme. Un second
   calcul aurait fini par annoncer « catalogue prêt » à côté d'un tarif manquant.
2. **L'activation lit dans sa transaction.** D'où `lecteur.go` : la vérification
   et l'écriture portent sur le même instant (un tarif supprimé entre les deux ne
   peut pas ouvrir la station). La ligne de l'entreprise est verrouillée
   (`FOR UPDATE`), l'instantané et la ligne de journal sont écrits dans la même
   transaction, et un refus ne laisse **aucune** trace.
3. **« Rien » n'est pas « zéro ».** Avant la première fiche, les blocs financiers
   sont **retirés** de la réponse du tableau de bord (pas mis à zéro : un `0 FCFA`
   ressemble à un résultat) et remplacés par `no_activity` + `no_activity_note`.
   L'état de la caisse reste lisible — une caisse ouverte n'est pas un chiffre
   d'affaires. `definitions` reste servi : c'est le lexique du rapport, il nomme
   les clés de montants sans en porter.
4. **Le rôle décide de la lecture, côté serveur.** Les blocs d'administration sont
   filtrés par permission (`settings:view`, `employee:view`, `service:manage`,
   `subscription:view`, `audit:view`) : le caissier reçoit `blocks: []` et garde
   ses raccourcis d'exploitation — sans « rapports », qu'il n'a pas le droit
   d'ouvrir. Le laveur reçoit `403` sur le tableau de bord et sur le récapitulatif ;
   son poste ne porte aucun montant.
5. **La réticence à afficher se porte dans le harnais.** La sonde du lot 8 ne se
   contente pas de lire les clés : elle balaie récursivement la réponse à la
   recherche de toute clé `_xof` (hors lexique) — c'est ce qui garantit
   « aucun montant avant la première commande » plutôt que de l'espérer.

**Défauts trouvés en vérifiant, et corrigés** :

1. **Les horaires du dimanche n'étaient jamais trouvés.** « Ouvert maintenant »
   indexait la table `station_opening_hours` (numérotation ISO, dimanche = 7) avec
   `time.Weekday` (dimanche = 0) : le dimanche retombait sur une journée absente.
   Corrigé par `isoDuJour`, et vérifié sur un compte dont les six jours sont
   déclarés.
2. **« Fermée aujourd'hui () ».** Une journée jamais déclarée n'a pas de ligne en
   base : le libellé partait d'un numéro absent et affichait une parenthèse vide.
   Le jour est désormais décrit par son nom réel — et, quand tout va bien, par
   « Ouverte jusqu'à 18:00 ».
3. **La documentation annonçait des routes qui n'existent pas.**
   `GET /stations/{id}/equipment/templates` (le référentiel est servi **dans**
   `GET /stations/{id}/equipment`), `PUT /equipment/{id}` et `PUT /stock/items/{id}`
   (ce sont des `PATCH`). Corrigé dans `docs/api.md` — un lecteur qui suit la
   documentation ne doit pas tomber sur un `404`.
4. **`auto_close_on_payment` n'était documenté nulle part**, alors que le réglage
   décide si encaisser clôt la fiche. Les deux réglages qui touchent à la clôture
   (`release_requires_payment`, `auto_close_on_payment`) sont maintenant décrits
   ensemble, avec la règle des paiements différés (un engagement ne solde jamais).

**Vérifié, chiffres réels** : `go test -count=1 ./...` → **61/61 tests verts**,
`ok washguard/backend 43.516s` (dont 6 tests neufs du lot 8) ; `rendu-pages.mjs`
**18/18** pages (le récapitulatif en plus) ; `coherence-routes.mjs` et
`classes-ui.mjs` au vert ; `make check` **5/5** ; campagne d'acceptation
**40/40 · 41/41 · 37/37 · 23/23 · 36/36 · 35/35 · 22/22 · 56/56 · 75/75 · 1/1**
(`sonde-lot8.py` **75/75**, nouvelle) ; `verif-apercu.py` au vert.
Quatre attentes de la sonde neuve étaient fausses (l'identité est acquise dès
l'inscription ; le prix du comptoir va dans la prestation, pas dans une règle ;
les blocs financiers sont retirés et non mis à zéro ; `definitions` nomme des
clés `_xof` sans porter de valeur) : c'est la sonde qui a été corrigée, jamais
le serveur — sauf pour les deux défauts réels ci-dessus.

**Reste ouvert, assumé** (inchangé depuis le lot 7, hors périmètre V1) : le
plafond par laveur est vérifié à la **création** de la fiche (le contrat noté
parlait de l'affectation) — à trancher ; `SalarySection.tsx` attend huit champs
RH qui n'existent pas dans `users` (masqué par `SALARY_UI_ENABLED`) ;
`delivered_at` est renseigné à la clôture, sans écran dédié pour le contester.


---

## 12. Décisions tranchées le 2026-10-14

1. **Nom du produit : WashGuard**, partout. Aucun renommage ; « PRESSPRO » n'est pas repris.
2. **Périmètre : lots 0 → 8** — l'onboarding va jusqu'à l'activation et au tableau de bord alimenté
   avant toute mise en service réelle. Aucun raccourci « provisoire » n'est laissé derrière soi.
3. **Rôles : les 5 rôles actuels** en V1. « Superviseur » ≈ `manager` rattaché à une station ;
   responsable de stock, comptable et rôles personnalisés en V2.
4. **Invitations : e-mail et téléphone**, sans envoi automatique (aucun service d'envoi dans le
   dépôt) : lien et code court copiables, bouton « Partager sur WhatsApp ».

Questions ouvertes secondaires (elles ne bloquent rien) :

- Un service d'envoi d'e-mails (ou un envoi WhatsApp automatique) est-il souhaitable plus tard ?
  Si oui, il faudra choisir le fournisseur et ses identifiants — la structure d'invitation prévue
  accepte les deux sans changement de schéma.
- Faut-il autoriser plusieurs espaces par compte (un gérant qui suit deux entreprises) ? Prévu en
  V2, nécessite une table `memberships` et un sélecteur d'espace actif.
