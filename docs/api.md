# API WashGuard

L'API est écrite en Go (bibliothèque standard + `pgx`), sans framework. Elle sert
aussi l'interface compilée : il n'y a donc **qu'un seul processus** à déployer.

- Base : `http://<hôte>:<port>/api/v1` — le port vient de `API_PORT` (3000 dans le
  gabarit `.env.example`, 8080 pour `make api` en développement) et l'écoute de `API_HOST`.
- Sonde de vie : `GET /health` → `{ "status": "ok", "environment": "...", "time": "..." }`.
- Réponses : JSON, `Content-Type: application/json; charset=utf-8`.
  Les exports (`/reports/export`, `/commissions/export`) renvoient du `text/csv`.

## Authentification

Deux chemins mènent à la même session applicative (cookie `washguard_session`,
`HttpOnly`, `SameSite=Lax`, `Secure` si `SESSION_COOKIE_SECURE=true`) :

| Chemin | Route | Détail |
| --- | --- | --- |
| Clerk | `POST /auth/clerk` | Le front envoie le jeton de session Clerk en `Authorization: Bearer <jwt>` ; le serveur le vérifie contre la JWKS, synchronise l'utilisateur, puis ouvre le cookie de session. |
| Mot de passe | `POST /auth/login` | Identifiant + mot de passe, réservé aux comptes créés par l'entreprise (employés). |
| Déconnexion | `POST /auth/logout` | Ferme la session. |
| Profil courant | `GET /auth/me` | Renvoie l'utilisateur, son rôle, sa station et **la liste des permissions** de son rôle. |

Le front appelle ces routes avec `credentials: "include"` : le cookie voyage seul,
aucun jeton n'est stocké dans `localStorage`.

### Session et environnement d'aperçu

**Hors production** (`APP_ENV` différent de `production`), les réponses de
`POST /auth/register`, `POST /auth/login`, `POST /auth/clerk` et d'acceptation
d'une invitation portent un champ `session_token` : le même jeton que celui du
cookie. Le client le garde le temps de l'onglet (`sessionStorage`) et le
présente en `Authorization: Bearer` ; `POST /auth/logout` ferme alors la session
quelle que soit la voie empruntée.

Cette tolérance existe parce qu'un aperçu en cadre est servi sous un autre
domaine que l'API : le cookie y est un cookie **tiers** (ou `Secure` sur une page
`http`), que le navigateur ne conserve pas — l'interface s'affichait alors
« Vous devez vous connecter » deux secondes après l'inscription.

En **production**, ce mécanisme est entièrement éteint : le champ `session_token`
n'est jamais renvoyé, un jeton en `Authorization` n'est jamais accepté comme
session applicative, et la session reste portée par le seul cookie `HttpOnly`
(voir `backend/apercu_session.go`).

## Erreurs

Forme unique, quelle que soit la cause :

```json
{ "error": { "code": "cash_session_required", "message": "Aucune session de caisse n'est ouverte : impossible d'encaisser des espèces." } }
```

`error.fields` apparaît sur les erreurs de validation (`{ "champ": "raison" }`).

Codes employés :

| Code HTTP | `error.code` | Signification |
| --- | --- | --- |
| 400 | `validation_error` | Corps ou paramètre invalide (`fields` détaille). |
| 401 | `unauthorized` | Session absente, expirée, ou jeton Clerk refusé. |
| 403 | `forbidden` | Le rôle n'a pas la permission, ou l'objet appartient à une autre entreprise. |
| 403 | `plan_limit` | La formule en cours ne permet pas d'ouvrir une station de plus. |
| 404 | `not_found` | Objet inexistant — ou appartenant à une autre entreprise (on ne révèle pas son existence). |
| 409 | `cash_session_required` | Encaissement en espèces alors qu'aucune caisse n'est ouverte. |
| 409 | `conflict` | Deux sessions de caisse ouvertes sur la même station, fiche déjà soldée, montant supérieur au reste dû. |
| 409 | `price_required` | Aucun tarif utilisable pour la combinaison vendue : la prestation doit être tarifée dans la grille (`fields.service_id` nomme la prestation). |
| 409 | `price_rule_conflict` | Deux règles aussi précises l'une que l'autre couvrent la même combinaison : le serveur refuse de choisir à votre place (`fields.price_rule_id` nomme la règle qui gêne). |
| 429 | `too_many_requests` | Trop d'échecs de connexion (10 par quart d'heure, réglable par `AUTH_MAX_FAILURES`) ou trop d'inscriptions depuis la même IP (5 par heure, `REGISTER_MAX_PER_HOUR`). La réponse indique le délai à attendre. |
| 500 | `internal` | Erreur interne ; le détail est dans le journal du serveur, jamais dans la réponse. |
| 501 | `clerk_disabled` | Le jeton Clerk est présenté alors que Clerk n'est pas configuré sur ce serveur. |

## Autorisations

Chaque route déclare la permission qu'elle exige (`a.requirePermission`). La
matrice de référence est `rolePermissions` dans `backend/app.go` ; le front lit la
même liste via `GET /auth/me` pour masquer les écrans, mais **le refus réel vient
toujours du serveur**.

Un utilisateur rattaché à une station ne voit que celle-ci (`cashier`, `washer`,
`manager` avec station) : les listes sont filtrées, et un identifiant d'une autre
station répond `404`.

## Routes

Format de chaque ligne : la permission requise, puis la route.

### Mise en route (onboarding)

L'état d'une étape est **calculé par le serveur** à partir des données
enregistrées (station active, téléphone de l'entreprise, prix des prestations).
Le client n'annonce jamais qu'une étape est faite : `PATCH` refuse
`state = "completed"` et `POST /onboarding/activate` revérifie tout.

Le parcours compte **treize étapes** : `espace`, `entreprise`, `station`,
`horaires`, `categories`, `usages`, `prestations`, `options`, `tarifs`,
`moyens`, `equipe`, `materiel`, `commandes`. Chaque étape indique le formulaire
qui l'alimente (`"form"` : `entreprise`, `station`, `horaires`, `vehicules`,
`usages`, `prestations`, `options`, `tarifs`, `moyens`, `equipe`, `materiel`,
`commandes` — `espace` n'en a pas, il décrit ce qui existe déjà) et ce qui est
constaté (`"detail"`). Une étape facultative non remplie (usages, options,
capacités, localisation) est signalée dans `detail` sans bloquer l'activation.

Depuis le lot 7, les trois dernières étapes — **équipe** (personnel), **matériel**
(équipements et stock) et **commandes** (règles de fonctionnement) — portent
`required: false` : une station équipée d'un seau et d'une éponge doit pouvoir
ouvrir. Elles se remplissent après l'activation, depuis les Paramètres, sans
rien perdre de leur constat.

| Permission | Route |
| --- | --- |
| `settings:view` | `GET /onboarding` (état, étapes, avancement, `missing`) |
| `settings:manage` | `PATCH /onboarding/steps/{code}` (corps : `data` descriptif) |
| `organization:manage` | `POST /onboarding/activate` (409 `onboarding_incomplete` si un prérequis manque) |

Tant que `organizations.onboarding_state` n'est pas `active`, les écritures
d'exploitation répondent **409 `onboarding_incomplete`** avec, dans
`error.fields`, les étapes à compléter (`POST /wash-orders`,
`POST /wash-orders/{id}/payments`, `POST /cash/sessions`, `POST /cash/payments`,
`POST /expenses`). La lecture (tableau de bord, rapports, listes) reste ouverte.

### Invitations (rejoindre une entreprise)

Créer un espace et le rejoindre sont deux chemins distincts. Une invitation ne
crée **jamais** d'entreprise, de station ni d'abonnement : elle rattache une
personne à ce qui existe déjà, avec un rôle opérationnel (`owner` n'est pas
invitable — la propriété ne s'obtient pas par un lien).

Deux identifiants désignent la même invitation : un `code` de 8 caractères
(sans 0/O/1/I/L, dictable au téléphone) et un jeton long présent dans le lien.
Seule l'empreinte du jeton est stockée : il n'est affiché qu'à la création et ne
peut pas être relu.

| Permission | Route |
| --- | --- |
| **aucune** (public) | `GET /invitations/{code}` — aperçu : nom de l'entreprise, ville, rôle, état. **Aucun identifiant** n'est renvoyé, et un code inventé répond 404 sans rien révéler. Le jeton long se passe en `?jeton=…` sur la même route. |
| `employee:view` | `GET /invitations` (liste de l'entreprise, état calculé à la lecture) |
| `employee:manage` | `POST /invitations` (`email` **ou** `phone`, `role` ∈ `manager|cashier|washer`, `station_id`, `validity_days` ≤ 90) |
| `employee:manage` | `POST /invitations/{id}/revoke` |

Réponse de création : `code`, `token` (une seule fois), `expires_at`, et le
rappel que **WashGuard n'envoie aucun message** — l'API ne dispose ni d'e-mail,
ni de SMS, ni d'envoi WhatsApp : le lien et le code sont à transmettre par
l'utilisateur (l'interface propose un partage WhatsApp prérempli).

Pour rejoindre, `POST /auth/register` accepte `invitation_token` : le compte est
créé dans l'entreprise invitante avec le rôle et la station de l'invitation, la
réponse porte `invitation_accepted: true`, et l'invitation est consommée dans la
même transaction. Refus : `404 invitation_unknown`, `409 invitation_expired`,
`409 invitation_revoked`, `409 invitation_used`, `409 invitation_requires_login`
(l'entreprise d'un compte est unique : un compte existant n'est pas rattaché de
force à une seconde entreprise).

### Organisation et réglages

`GET /organizations/current` renvoie l'entreprise **et** `registry[]` : une
entrée par mention administrative (`field_code`, `label`, `help`, `value`,
`status` ∈ `empty|provided|pending_review|verified`, `status_label`,
`verified_at`). `PATCH` accepte les mêmes champs (identité commerciale,
facturation, profil juridique) plus `registry` (`{code: valeur}`) ; un
`field_code` inconnu est refusé (400 `validation_error`), et les champs fautifs
sont listés dans `error.fields`.

| Permission | Route |
| --- | --- |
| session | `GET /organizations/current` (entreprise + état du registre) |
| `settings:manage` | `PATCH /organizations/current` (identité, facturation, registre, réglages) |
| `settings:view` | `GET /stations/{id}/profile` (`station`, `site`, `days[7]`, `exceptions` , `types_lavage_proposes`) |
| `settings:manage` | `PATCH /stations/{id}` (dont localisation et objet `site`) |
| `settings:manage` | `PUT /stations/{id}/opening-hours` (`days`, `exceptions`) |
| `subscription:view` | `GET /organizations/subscription` |
| `subscription:manage` | `POST /organizations/subscription/request` |
| `audit:view` | `GET /organizations/audit` |
| `station:view` | `GET /stations` |
| `organization:manage` | `POST /stations` |

### Règles de saisie (ces routes valident avant d'écrire)

- **Capacités à trois états** : `has_vacuum`, `has_drying`, `has_detailing`,
  `has_waiting_room`, `has_storage`, `has_generator` sont `null` tant que la
  question n'a pas été posée, puis `true` ou `false`. En lecture, `null` n'est
  jamais rendu comme `false` : « pas d'aspiration » et « on ne sait pas » ne
  disent pas la même chose.
- **Horaires** : une ligne par jour (1 = lundi … 7 = dimanche), `HH:MM`,
  fermeture après ouverture, pause comprise dans la plage (ou les deux bornes
  vides), dernier enregistrement dans les heures d'ouverture, jour fermé sans
  horaires, une seule exception par date. Un refus renvoie 400
  `validation_error` avec, dans `error.fields`, le **nom du jour** fautif.
- **Localisation** : `latitude`/`longitude` en `NUMERIC(9,6)`, rendues en texte
  (`"5.348900"`) ; une chaîne vide vaut « non renseigné » (`NULL`).
- **Prestations** : l'application ne propose aucun prix. Une prestation à
  `price_xof = 0` est refusée à la vente (409 `price_required`) et l'étape
  « Grille tarifaire » reste incomplète.
- **Mentions administratives** : la saisie ne vaut pas vérification. Seule la
  plateforme peut vérifier (`platform:manage`), et modifier une valeur annule la
  vérification acquise.

### Catalogue et clients

| Permission | Route |
| --- | --- |
| `service:view` | `GET /services?station_id=…` |
| `service:manage` | `POST /services`, `PATCH /services/{id}`, `DELETE /services/{id}` (désactivation) |
| `service:view` | `GET /service-templates?station_id=…`, `GET /service-families?station_id=…` |
| `service:view` | `GET /services/{id}/compatibilities` |
| `service:manage` | `PUT /services/{id}/compatibilities`, `PUT /stations/{id}/service-families` |

**Un prix ne se crée pas ici.** `POST /services` et `PATCH /services/{id}`
refusent un `price_xof` non nul (`400 validation_error`, champ `price_xof`) : le
tarif du catalogue se pose dans la grille tarifaire, avec un motif et une trace
d'audit. Une prestation naît donc « à tarifer » — elle n'est pas vendable tant
que la grille ne la couvre pas.
| `customer:view` | `GET /customers`, `GET /customers/options`, `GET /customers/{id}` |
| `customer:create` | `POST /customers` |
| `customer:manage` | `PATCH /customers/{id}`, `DELETE /customers/{id}` (archivage) |

### Véhicules et usages

Deux référentiels **distincts**, parce qu'ils ne répondent pas à la même
question : la **catégorie** décrit le véhicule (citadine, berline, moto, gbaka,
camion), l'**usage** décrit ce qu'on en fait (voiture personnelle, taxi
compteur, taxi communal, livraison, flotte…). Une berline « taxi compteur »
reste une berline : la caisse peut donc compter ses berlines **et** savoir
lesquelles étaient des taxis.

Le référentiel des deux familles est amorcé par le serveur à chaque démarrage
(entrées mises à jour, jamais dupliquées). Une station ne le modifie pas : elle
l'**active**, le renomme localement et le réordonne. Désactiver ne supprime
rien — les fiches passées gardent le nom qu'elles portaient.

| Permission | Route |
| --- | --- |
| `station:view` | `GET /vehicle-catalogue?station_id=…` (référentiel complet + état local) |
| `station:view` | `GET /vehicle-categories?station_id=…[&actives=1]` (les catégories **de la station**) |
| `station:view` | `GET /vehicle-usages?station_id=…[&actives=1]` |
| `wash:create` | `GET /vehicle-category-options?station_id=…` (les deux listes en un appel, pour la caisse) |
| `settings:manage` | `PUT /stations/{id}/vehicle-categories` (activation en bloc) |
| `settings:manage` | `PUT /stations/{id}/vehicle-usages` |
| `settings:manage` | `POST /vehicle-categories` (catégorie propre à la station, code `perso_…`) |
| `settings:manage` | `PATCH /vehicle-categories/{code}` (renommer, réordonner, activer) |
| `station:view` | `GET /vehicle-usage-categories?station_id=…` (combinaisons déclarées) |
| `settings:manage` | `PUT /stations/{id}/vehicle-usage-categories` |

Quelques règles tenues par le serveur :

- **Activation en bloc** : `PUT /stations/{id}/vehicle-categories` reçoit
  `{"categories": [{"code": "berline", "is_active": true, "name": "", "sort_order": 10}]}`.
  Un `name` vide signifie « ne change pas le nom » — l'écran qui coche des
  entrées ne réécrit pas les renommages. Pour rétablir le libellé du
  référentiel, `PATCH` avec `"name": ""`.
- **Une catégorie créée par la station** porte un code préfixé `perso_` : une
  entrée ajoutée plus tard au référentiel ne peut pas écraser la sienne.
- **Toute écriture en bloc est validée avant d'être appliquée** : un code
  inconnu refuse l'ensemble et nomme les codes fautifs (`fields`), rien n'est
  enregistré à moitié.
- **Une combinaison (usage, catégorie) doit exister** : une fiche de lavage qui
  cite une combinaison non déclarée est refusée (400 `validation_error`). Le
  couple lui-même ne porte pas de prix : les tarifs se posent dans la grille
  (`price_rules`), qui accepte le couple comme critère le plus précis.
- **`POST /wash-orders` accepte `vehicle_category_id` et `vehicle_usage_id`** ;
  la fiche conserve `vehicle_category_name` **et** `vehicle_usage_name`, donc
  elle reste lisible après un renommage ou une désactivation.
- La route `GET /vehicle-categories` renvoie les catégories **de la station**
  (liste vide = état honnête : rien n'a encore été activé), pas le référentiel
  du produit.

### Prestations, options et grille tarifaire

Le catalogue décrit ce qui est vendu ; la grille décrit **à quel prix**. La
caisse n'envoie jamais un montant : elle envoie des choix (prestation,
véhicule, usage, client, options) et le serveur répond le prix **et son
origine**.

| Permission | Route |
| --- | --- |
| `service:view` | `GET /service-options?station_id=…[&service_id=…]` |
| `service:manage` | `POST /service-options`, `PATCH /service-options/{id}`, `DELETE /service-options/{id}` (désactivation) |
| `service:view` | `GET /price-rules?station_id=…[&service_id=…]` |
| `service:manage` | `POST /price-rules`, `PATCH /price-rules/{id}`, `DELETE /price-rules/{id}` (désactivation) |
| `service:view` | `GET /price-rules/matrix?station_id=…` |
| `service:manage` | `PUT /price-rules/matrix` (enregistrement en bloc, motif obligatoire) |
| `wash:create` | `GET /price-preview?station_id=…&service_id=…&category_id=…&usage_id=…&customer_id=…&date=…` |

Ordre de résolution, du plus précis au plus général — appliqué par
`backend/pricing.go`, jamais par le navigateur :

| Niveau | Source (`price_source`) | Critère |
| --- | --- | --- |
| 1 | `client_contract` | prestation + client (contrat, flotte) |
| 2 | `service_category_usage` | prestation + type de véhicule + usage |
| 3 | `service_category` / `service_usage` | prestation + véhicule **ou** prestation + usage |
| 4 | `service_generic` | tarif du catalogue (`services.price_xof`) |
| — | `free` | prestation déclarée gratuite : zéro, et rien ne prime sur cette décision |
| — | — | aucun des précédents ⇒ `409 price_required` |

Quelques règles tenues par le serveur :

- **Deux règles de même spécificité se signalent** (`409 price_rule_conflict`) au
  lieu d'être arbitrées : le prix bas serait aussi défendable que le haut.
- **Un discriminant absent rend la règle inapplicable** : une fiche sans
  véhicule précisé ne déclenche pas un tarif « berline ».
- **`PUT /price-rules/matrix` enregistre en bloc** : `cells`
  (`service_id`, `category_code`, `price_xof`), `usage_cells` (`usage_code`),
  `generic_prices` (`service_id`, `price_xof`) et un `reason` obligatoire. Une
  case vidée (`price_xof: null`) **désactive** la règle au lieu de la supprimer ;
  la réponse compte `creees`, `modifiees`, `desactivees`. Rien n'est écrit si
  une seule case est fautive (négatif, prestation gratuite, prestation sur
  devis).
- **`GET /price-rules/matrix`** renvoie `categories`, `usages`, `services`
  (chaque ligne avec `cells`, `usage_cells`, `generic_price_xof`, `needs_price`)
  et `a_tarifer` : l'écran affiche ce qui reste à tarifer sans calculer.
- **`GET /price-preview` répond 200 même sans tarif** (`price_xof: null`,
  `needs_price: true`, `message`) : la saisie en cours n'est pas cassée par une
  combinaison non tarifée. Elle renvoie aussi les `options` applicables
  (`scope` : `station`, `prestation` ou `vehicule`).
- **`PATCH /price-rules/{id}`** exige un `reason` : modifier un prix est une
  décision, elle se justifie.
- **`allow_generic_price: false`** (réglage de la station) retire le niveau 4 :
  seules les règles vendent. Le réglage se pose par
  `PATCH /stations/{id}` → `settings`.

Les **options** se facturent **par lavage** : une ligne de trois lavages avec
une option à 500 FCFA porte `options_total_xof = 1500`. Une option inconnue est
refusée (`400`) : la fiche ne s'enregistre pas à moitié. Une fiche passée garde
le détail de ses options même après désactivation.

### Équipe

| Permission | Route |
| --- | --- |
| session | `GET /employees/options` (liste des laveurs pour les formulaires) |
| `employee:view` | `GET /employees`, `GET /employees/{id}/activity` |
| `employee:manage` | `POST /employees`, `PATCH /employees/{id}`, `POST /employees/{id}/reset-password` |
| `employee:compensation` | écriture du barème (`commission_mode`, `commission_per_wash_xof`, `commission_percent`) |

La fiche de poste d'un employé porte `job_title`, `hired_on`, `work_schedule`
et `employment_status` (`active`, `on_leave`, `departed`) — facultatifs, du
ressort de `employee:manage` — et, à part, le **barème de rémunération** :
`commission_mode` (`none`, `per_wash`, `percent` ; absent = celui de la
station), `commission_per_wash_xof`, `commission_percent`.

**La rémunération est une donnée réservée.** Elle demande la permission
`employee:compensation` (propriétaire et support, **pas** gérant) : un compte
sans cette permission reçoit `403` sur toute écriture de barème, et les champs
`commission_*` ne sont pas renvoyés dans la liste. Les autres champs de la fiche
de poste restent lisibles par `employee:view`.

**Le congé n'est pas une sanction** : `employment_status = "on_leave"` décrit
une situation RH et laisse l'accès ouvert ; seul `is_active = false` ferme la
porte. Une valeur inconnue est refusée (`400 validation_error`).

### Lavages

| Permission | Route |
| --- | --- |
| `wash:view` | `GET /wash-orders`, `GET /wash-orders/{id}` |
| `wash:create` | `POST /wash-orders` |
| `wash:update` | `PATCH /wash-orders/{id}` (dont le changement de statut, contrôlé par le graphe du cycle de vie) |
| `wash:assign` | `POST /wash-orders/{id}/assign` |
| `wash:cancel` | `POST /wash-orders/{id}/cancel` |
| `payment:create` | `POST /wash-orders/{id}/payments` |

Le détail d'une fiche renvoie `{ "order": {...}, "payments": [...] }` : le reste à
payer est `order.due_xof`, et une fiche soldée d'un coup passe `completed`.

Chaque ligne porte `unit_price_xof`, `line_total_xof`, `price_source`,
`price_source_label`, `options_total_xof` et le détail `options`
(`option_id`, `name`, `price_xof`, `quantity`, `total_xof`) : le montant est
recopié avec sa provenance, donc une fiche reste explicable même si la grille
change plus tard. `POST /wash-orders` accepte `items[].options`
(`[{ "option_id": "…", "quantity": 1 }]`) et **refuse** tout montant envoyé par
le client : `unit_price_xof` n'existe pas côté requête.

**Changer le véhicule d'une fiche recalcule le prix** : `PATCH /wash-orders/{id}`
avec un autre `vehicle_category_id` (ou `vehicle_usage_id`) repasse par le
moteur de prix et met à jour les lignes. Une fiche déjà encaissée dont le total
bougerait est refusée (`409 conflict`) : le paiement reste la référence.

### Matériel et stock

| Permission | Route |
| --- | --- |
| `settings:view` | `GET /stations/{id}/equipment` (parc de la station) |
| `settings:manage` | `POST /stations/{id}/equipment`, `PUT /equipment/{id}`, `GET /stations/{id}/equipment/templates` |
| `settings:view` | `GET /stations/{id}/stock`, `GET /stock/items/{id}` |
| `settings:manage` | `POST /stations/{id}/stock/items`, `PUT /stock/items/{id}`, `POST /stock/items/{id}/movements` |

**Équipements.** `GET /stations/{id}/equipment/templates` sert le référentiel des
matériels (identique pour toutes les entreprises) ; l'ajout se fait soit à
partir d'un modèle (`template_code`), soit librement (`name`). Un engagement
explicite : **une quantité n'est pas une unité — on n'efface jamais du
matériel**, `is_active = false` conserve la fiche dans l'historique et la sort
du parc en service (`summary.en_service`, `summary.hors_service`).

**Stock.** Ce qui distingue ce module d'une simple liste, c'est l'état
`quantity_known`. Un article déclaré sans quantité porte
`quantity_known: false` : le serveur refuse alors toute sortie
(`409 stock_quantity_unknown`) et ne compte pas de rupture — *« je ne sais pas
encore »* n'est pas *« zéro »*. Une quantité ne s'écrit pas à la main
(`409 stock_movement_required`) : elle change par un mouvement
(`entrée`, `sortie`, `inventaire`, `perte`), et chaque mouvement d'un article
renvoie `quantity_after`. Une perte ou une casse exige un motif
(`400 validation_error`). `GET /stations/{id}/stock` renvoie `items`, `summary`
(`rupture`, `quantite_inconnue`, valeur XOF) et `categories`.

### Règles de fonctionnement des commandes

| Permission | Route |
| --- | --- |
| `settings:view` | `GET /stations/{id}/order-rules` |
| `settings:manage` | `PUT /stations/{id}/order-rules` |

Sept réglages, lus **là où le serveur décide** — création de la fiche,
encaissement, changement de statut, annulation :

| Réglage (défaut) | Effet mesurable |
| --- | --- |
| `allow_walk_in` (`true`) | `false` ⇒ fiche sans client enregistré refusée (`400 walk_in_not_allowed`) |
| `require_washer` (`false`) | `true` ⇒ fiche sans laveur refusée (`400 washer_required`) |
| `max_open_orders_per_washer` (`0`) | au-delà ⇒ `409 washer_queue_full`, libéré dès qu'une fiche se ferme |
| `quality_check_required` (`false`) | `true` ⇒ passage direct à `ready` refusé (`409 quality_check_required`) |
| `release_requires_payment` (`true`) | `true` ⇒ remise du véhicule impossible avant solde (`409 payment_required`) |
| `allow_partial_payments` (`true`) | `false` ⇒ acompte refusé (`409 partial_payment_disabled`), sur la fiche **et** au guichet |
| `require_cancel_reason` (`true`) | `true` ⇒ annulation sans motif refusée (`400 validation_error` sur `reason`) |

La réponse expose aussi `statuses` : les **sept états** du cycle de vie d'un
véhicule et leurs libellés, pour que l'écran n'invente pas ses propres mots.
Une transition non prévue par le graphe est refusée (`409 transition_forbidden`),
un état final le reste, et un statut inconnu est refusé en `400`.

### Caisse et paiements

| Permission | Route |
| --- | --- |
| `cash:view` | `GET /cash/current`, `GET /cash/sessions`, `GET /cash/sessions/{id}` |
| `cash:open` | `POST /cash/sessions` (refusé en `409` si une session est déjà ouverte) |
| `cash:close` | `POST /cash/sessions/{id}/close` (compte réel saisi, écart calculé) |
| `cash:reopen` | `POST /cash/sessions/{id}/reopen` |
| `payment:view` | `GET /cash/payments?limit=100` |
| `payment:create` | `POST /cash/payments`, `POST /wash-orders/{id}/payments` |
| `payment:void` | `POST /cash/payments/{id}/void` (annulation motivée, rien n'est supprimé) |

Règles : un montant ≤ 0 est refusé (`400 validation_error`) ; un encaissement
supérieur au reste dû est refusé ; les espèces exigent une session ouverte quand
la station porte `require_open_cash_session` (réglage activé par défaut).

#### Moyens de paiement déclarés

Une station ne choisit pas dans une liste figée : elle **déclare** ses moyens
(Orange Money CI, Wave, chèque…). Chaque moyen porte une **famille** comptable
(`cash`, `mobile_money`, `card`, `transfer`, `deferred`, `other`) — c'est elle
qui alimente les compteurs de caisse et les rapports, jamais le libellé.

| Permission | Route |
| --- | --- |
| `payment:view` | `GET /payment-methods?station_id=…&actives=1` |
| `settings:manage` | `POST /payment-methods` (moyen propre : code `perso_…`) |
| `settings:manage` | `PATCH /payment-methods/{id}` (`name`, `is_active`, `requires_reference`, `sort_order`) |
| `settings:manage` | `DELETE /payment-methods/{id}` (désactive ; **jamais** de suppression) |

L'encaissement (`POST /wash-orders/{id}/payments`, `POST /cash/payments`)
accepte `payment_method_id` — la voie normale — ou `method` (famille
canonique). Refus : moyen inconnu ou d'une autre station (`404`), moyen
désactivé (`409 payment_method_disabled`), référence exigée et absente
(`400`, champ `reference`), différé sans la permission
`payment:create_deferred` (`403`). Le **dernier moyen actif** ne peut pas être
désactivé (`409`) : sans moyen, la caisse ne peut plus rien encaisser.

Un paiement `differe` est un **engagement du client**, pas une somme reçue : il
figure sur la fiche et dans le journal, mais n'entre ni dans `paid_xof`, ni dans
les totaux, ni dans la session de caisse, et ne solde donc pas la fiche.

#### Paramètres de caisse et reçus

| Permission | Route |
| --- | --- |
| `cash:view` | `GET /stations/{id}/cash-settings` |
| `settings:manage` | `PUT /stations/{id}/cash-settings` |

Onze réglages, tous appliqués par le serveur : `require_open_cash_session`,
`max_cashier_discount_xof`, `default_float_xof`, `discrepancy_tolerance_xof`,
`allowed_roles`, `allow_void`, `receipt_prefix`, `receipt_footer`,
`receipt_legal_name`, `receipt_tax_id`, `receipt_logo_url`. Un rôle inconnu dans
`allowed_roles` est refusé (`400`). `receipt_legal_name` et `receipt_tax_id` ne
sont **jamais** déduits : ce sont des mentions déclarées par une personne
habilitée.

| Permission | Route |
| --- | --- |
| `payment:view` | `GET /wash-orders/{id}/receipt` (`404 receipt_not_issued` si la fiche n'est pas soldée) |
| `payment:view` | `POST /wash-orders/{id}/receipt` (émission **idempotente** : un reçu a un seul numéro) |

Le reçu est émis automatiquement quand la fiche est soldée. Son numéro suit
`PRÉFIXE-AAAAMMJJ-NNNN` (compteur par station et par jour, sous verrou). Les
mentions absentes ne sont pas imprimées vides : elles sont listées dans
`missing_mentions`.

### Dépenses, rapports, commissions

| Permission | Route |
| --- | --- |
| `expense:view` | `GET /expenses` |
| `expense:create` | `POST /expenses` |
| `expense:approve` | `PATCH /expenses/{id}` |
| `expense:void` | `POST /expenses/{id}/void` |
| `dashboard:view` | `GET /reports/dashboard` |
| `report:view` | `GET /reports/overview?from=…&to=…` |
| `report:export` | `GET /reports/export?from=…&to=…` (CSV) |
| `commission:view` | `GET /commissions`, `GET /commissions/export` (CSV) |

### Poste du laveur

| Permission | Route |
| --- | --- |
| `washer:view` | `GET /washer/tasks` |
| `washer:update_status` | `POST /washer/tasks/{id}/status` |
| `washer:report_incident` | `POST /washer/tasks/{id}/incident` |
| `washer:personal_stats` | `GET /washer/me/performance` |

Réponse typique de `GET /washer/tasks` :

```json
{ "tasks": [ { "id": "…", "status": "in_progress", "customer_name": "…", "vehicle_plate": "…",
               "service_labels": "Lavage complet", "total_xof": 5000, "commission_xof": 500 } ],
  "counts": { "pending": 3, "in_progress": 1, "done_today": 7 },
  "page_size": 5 }
```

### Plateforme (support WashGuard)

Réservées au rôle `superadmin` (vérifié deux fois : permission `platform:manage`
puis contrôle du rôle dans le handler) : `GET /platform/overview`,
`GET|PATCH /platform/organizations/{id}`, `POST /platform/organizations/{id}/subscription`,
`POST /platform/organizations/{id}/registry/{code}/verify` (atteste qu'une
mention administrative a été contrôlée : 409 si la valeur est vide, trace dans
`audit_logs`, et toute modification ultérieure de la valeur annule l'attestation),
`GET|PATCH /platform/plans/{id}`, `GET /platform/audit`, `GET /platform/plans`
(public, sans session : tarifs de la vitrine).

## Limites et conventions

- `limit` (défaut 100) sur les listes longues (paiements, entreprises côté
  plateforme) ; les autres listes sont bornées par le périmètre de l'entreprise.
- Dates au format RFC 3339 (`2026-10-13T08:30:00Z`) ; montants en entiers de FCFA
  (`…_xof`), jamais en flottants.
- Anti-abus : `AUTH_MAX_FAILURES` (défaut 10) échecs de connexion par fenêtre et
  par identifiant, `REGISTER_MAX_PER_HOUR` (défaut 5) inscriptions par heure et par IP.
- Les migrations de schéma sont appliquées au démarrage du serveur (`-migrate-only`
  permet de les jouer sans démarrer l'API).

## Vérifier l'API sans navigateur

```bash
make smoke                                   # routes + rendu des écrans (hors réseau)
make verifier-acces                          # sondes d'acceptation contre l'API qui tourne
WASHGUARD_PSQL=/usr/bin/psql python3 scripts/verifications/sonde-metier.py http://127.0.0.1:3000
python3 scripts/verifications/sonde-onboarding.py http://127.0.0.1:3000   # formulaires → base → relecture
```
