# Plan d'implémentation — « lots suivants » (backend Go)

> Contexte au 14 octobre 2026. Les fondations sont **livrées et vérifiées** :
> auth (register/login/logout/me), invitations, onboarding (13 étapes),
> `GET|PATCH /organizations/current`, stations, sessions et aperçu.
> Les 8 enregistrements encore vides de `routes_restes.go` doivent être remplis,
> dans l'ordre des dépendances : chaque lot n'utilise que ce que les précédents
> ont posé. La spécification complète est `docs/api.md` ; les écrans du front
> existent déjà et appellent les routes ci-dessous.

## Ordre de livraison et dépendances

```
Lot 1  Référentiels véhicule/usages      ← ne dépend de rien
Lot 2  Catalogue prestations + options   ← dépend de Lot 1 (compatibilités)
Lot 3  Grille tarifaire + price-preview  ← dépend de Lots 1+2
Lot 4  Équipe + invitations REST         ← indépendant (peut chevaucher 2-3)
Lot 5  Lavages + paiements + caisse      ← dépend de Lots 1-3 (moteur de prix)
Lot 6  Dépenses + rapports + commissions ← dépend de Lot 5 (données réelles)
Lot 7  Matériel, stock, règles commandes ← dépend de Lot 5 (enforcement)
Lot 8  Plateforme (support)              ← dépend de tout (lecture globale)
```

---

## Lot 1 — Référentiels véhicules et usages (`routes_vehicules.go`)

**Routes** (documentation : § « Véhicules et usages » de `docs/api.md`) :
- `GET /vehicle-catalogue?station_id=` — `station:view` (référentiel + état local)
- `GET /vehicle-categories?station_id=[&actives=1]` — `station:view`
- `GET /vehicle-usages?station_id=[&actives=1]` — `station:view`
- `GET /vehicle-category-options?station_id=` — `wash:create` (deux listes en un appel)
- `GET /vehicle-usage-categories?station_id=` — `station:view` (combinaisons déclarées)
- `PUT /stations/{id}/vehicle-categories` — `settings:manage` (activation en bloc)
- `PUT /stations/{id}/vehicle-usages` — `settings:manage`
- `POST /vehicle-categories` — `settings:manage` (catégorie `perso_…`)
- `PATCH /vehicle-categories/{code}` — `settings:manage` (renommer/réordonner/activer)
- `PUT /stations/{id}/vehicle-usage-categories` — `settings:manage`

**Règles serveur non négociables** :
- amorçage du référentiel au démarrage (déjà fait par `amorcerReferentiels`) ;
- activation **en bloc** validée avant écriture : code inconnu → 400 `validation_error`
  avec les codes fautifs dans `fields`, rien n'est écrit à moitié ;
- `name` vide = « ne change pas le libellé » ; toute écriture de la station
  conserve les fiches passées (copie `vehicle_category_name` à la vente) ;
- catégorie créée par la station : code préfixé `perso_` ;
- une combinaison (usage, catégorie) doit exister pour être vendable.

**Vérification** : sonde type `sonde-vehicules.py` (créer une station, activer
des catégories, créer une catégorie `perso_`, tenter un code inconnu en PUT → 400).

---

## Lot 2 — Catalogue : prestations et options (`routes_catalogue.go`)

**Routes** :
- `GET|POST|PATCH|DELETE /services[/{id}]` — `service:view` / `service:manage`
  (DELETE = désactivation, jamais de suppression) ;
- `GET /service-templates?station_id=`, `GET /service-families?station_id=`
  (référentiels amorcés côté serveur) ;
- `GET|PUT /services/{id}/compatibilities`, `PUT /stations/{id}/service-families` ;
- `GET|POST|PATCH|DELETE /service-options[/{id}]` — `service:view` / `service:manage`.

**Règles** :
- `POST|PATCH /services` **refusent** tout `price_xof` non nul
  (400, champ `price_xof`) : le prix appartient à la grille (Lot 3) ;
- une prestation gratuite se déclare dans la grille, pas dans le catalogue ;
- options facturées **par lavage** (scope `station` / `prestation` / `vehicule`) ;
- isolation par station : toute liste filtre `organization_id` + `station_id`,
  un identifiant d'une autre entreprise répond 404 (jamais 403).

**Vérification** : sonde catalogue (créer prestation → prix refusé →
compatibilités → options par scope).

---

## Lot 3 — Grille tarifaire et moteur de prix (`routes_tarifs.go`, `pricing.go`)

**Routes** :
- `GET|POST|PATCH|DELETE /price-rules[/{id}]` — `service:view` / `service:manage`
  (PATCH exige un `reason`, tracé dans `audit_logs`) ;
- `GET /price-rules/matrix?station_id=` — `service:view` ;
- `PUT /price-rules/matrix` — `service:manage` (enregistrement en bloc, `reason`
  obligatoire, réponse `{creees, modifiees, desactivees}`) ;
- `GET /price-preview?station_id=&service_id=&category_id=&usage_id=&customer_id=&date=`
  — `wash:create` (répond 200 même sans tarif : `price_xof: null`,
  `needs_price: true`, plus les `options` applicables).

**Moteur de prix** (ordre du plus précis au plus général) :
1. `client_contract` (prestation + client)
2. `service_category_usage`
3. `service_category` / `service_usage`
4. `service_generic` (retiré si `settings.allow_generic_price = false`)
5. `free` prime sur tout ; rien trouvé ⇒ 409 `price_required`.

**Règles** : deux règles de même spécificité sur la même combinaison ⇒ 409
`price_rule_conflict` (jamais d'arbitrage) ; un discriminant absent rend la
règle inapplicable ; case vidée dans la matrice = **désactivation**, pas
suppression ; montants entiers FCFA, négatif refusé, prestation gratuite non
tarifée, prestation sur devis non tarifée.

**Vérification** : la sonde `sonde-tarifs.py` (décrite dans
`scripts/verifications/README.md`) sert de cahier de recette — 409
`price_required` avant tarification, hiérarchie complète, conflit de
spécificité, options par lavage.

---

## Lot 4 — Équipe et invitations REST (`routes_equipe.go`)

**Routes** :
- `GET /employees/options` — session (laveurs pour les formulaires) ;
- `GET /employees`, `GET /employees/{id}/activity` — `employee:view` ;
- `POST /employees`, `PATCH /employees/{id}`, `POST /employees/{id}/reset-password`
  — `employee:manage` (fiche de poste : `job_title`, `hired_on`,
  `work_schedule`, `employment_status` ∈ `active|on_leave|departed`) ;
- barème de rémunération (`commission_mode`, `commission_per_wash_xof`,
  `commission_percent`) : permission **`employee:compensation`** distincte —
  les champs `commission_*` ne quittent jamais le serveur sans elle ;
- `GET /invitations`, `POST /invitations`, `POST /invitations/{id}/revoke`
  — `employee:view` / `employee:manage` (l'aperçu public
  `GET /invitations/{code}` est **déjà livré** côté auth).

**Règles** : `employment_status = "on_leave"` n'est pas une sanction (l'accès
reste ouvert, seul `is_active = false` ferme la porte) ; valeur inconnue → 400 ;
invitation jamais `owner` ; quotas de la formule (`max_employees`) vérifiés à la
création (403 `plan_limit`).

**Vérification** : sonde équipe (créer, mettre en congé, barème refusé sans
`employee:compensation`, quota de formule).

---

## Lot 5 — Lavages, caisse et paiements (`routes_lavages.go`, `routes_caisse.go`)

Le cœur métier. Deux fichiers, un seul cycle de vie.

**Lavages** :
- `GET /wash-orders`, `GET /wash-orders/{id}` — `wash:view` (détail =
  `{order, payments}` avec `due_xof`) ;
- `POST /wash-orders` — `wash:create` : la requête ne porte **aucun montant** ;
  le serveur passe par le moteur de prix (Lot 3), recopie `unit_price_xof`,
  `line_total_xof`, `price_source(_label)`, `options` détaillées ; refuse une
  combinaison (usage, catégorie) non déclarée (Lot 1) et une option inconnue ;
- `PATCH /wash-orders/{id}` — `wash:update` (graphe de cycle de vie à 7 états,
  refus = 409 `transition_forbidden` ; changer de véhicule **recalcule** le prix,
  refus 409 `conflict` si la fiche est déjà encaissée) ;
- `POST /wash-orders/{id}/assign` — `wash:assign` ;
- `POST /wash-orders/{id}/cancel` — `wash:cancel` (droit distinct de
  `wash:update` ; motif obligatoire selon réglage) ;
- `POST /wash-orders/{id}/payments` — `payment:create`.

**Caisse** :
- `GET /cash/current`, `GET /cash/sessions`, `GET /cash/sessions/{id}` — `cash:view` ;
- `POST /cash/sessions` — `cash:open` (409 si déjà ouverte sur la station) ;
- `POST /cash/sessions/{id}/close` — `cash:close` (compte réel saisi, écart calculé) ;
- `POST /cash/sessions/{id}/reopen` — `cash:reopen` ;
- `GET /cash/payments?limit=100` — `payment:view` ;
- `POST /cash/payments` — `payment:create` ; `POST /cash/payments/{id}/void`
  — `payment:void` (annulation motivée, rien n'est supprimé) ;
- `GET|PUT /stations/{id}/cash-settings` — `cash:view` / `settings:manage`
  (onze réglages, rôles inconnus refusés).

**Paiements** : montant ≤ 0 refusé ; encaissement > reste dû refusé ; espèces
exigent une session ouverte (`require_open_cash_session`) ; **un paiement
`differe` ne solde rien et n'entre dans aucun total** ; clôture automatique
décidée en **un seul endroit** (`cloturerFicheSiSoldee`), appelée par la fiche
comme par le guichet ; reçu émis de façon idempotente à la solde
(`PRÉFIXE-AAAAMMJJ-NNNN`, compteur sous verrou, `missing_mentions`).

**Garde-fou transversal** : tant que l'onboarding n'est pas `active`, toute
écriture d'exploitation (`POST /wash-orders`, `POST /cash/*`, `POST /expenses`)
répond 409 `onboarding_incomplete` avec les étapes manquantes — la lecture
reste ouverte.

**Vérification** : sonde-metier (cycle complet : caisse → fiche → prix →
encaissement → clôture → reçu → void) + isolation entre entreprises.

---

## Lot 6 — Dépenses, rapports, commissions (`routes_rapports.go`)

**Routes** :
- `GET /expenses` — `expense:view` ; `POST /expenses` — `expense:create` ;
  `PATCH /expenses/{id}` — `expense:approve` ; `POST /expenses/{id}/void`
  — `expense:void` (motif obligatoire) ;
- `GET /reports/dashboard` — `dashboard:view` (jour en cours + bloc `setup` ;
  **avant la première fiche, aucun montant n'est servi** : blocs financiers
  retirés, remplacés par `no_activity` + `no_activity_note` ; filtrage par
  permission : `amounts_hidden: true`, `blocks: []` pour un caissier) ;
- `GET /reports/overview?from=&to=` — `report:view` ;
- `GET /reports/export` — `report:export` (CSV, `text/csv`) ;
- `GET /commissions`, `GET /commissions/export` — `commission:view` (CSV).

**Règles** : montants en entiers XOF ; séries journalières construites sur les
paiements **réellement reçus** (jamais les différés) ; commissions selon le
barème du Lot 4 (`per_wash` / `percent`, défaut = celui de la station) ;
l'export CSV et l'API partagent le même calcul.

**Vérification** : sonde rapports (fiches → dashboard sans activité →
première fiche → dashboard avec montants → export CSV cohérent avec l'API).

---

## Lot 7 — Matériel, stock, règles de commandes (`routes_materiel.go`)

**Équipements** :
- `GET /stations/{id}/equipment` — `settings:view` (`items` + `templates`) ;
- `POST /stations/{id}/equipment` (par `template_code` ou libre),
  `PATCH /equipment/{id}` — `settings:manage` ; désactivation = sortie du parc
  en service, la fiche reste (`summary.en_service` / `summary.hors_service`).

**Stock** :
- `GET /stations/{id}/stock`, `GET /stock/items/{id}` — `settings:view` ;
- `POST /stations/{id}/stock/items`, `PATCH /stock/items/{id}`,
  `POST /stock/items/{id}/movements` — `settings:manage`.

**Règles stock** : article sans quantité → `quantity_known: false`, sortie
refusée (409 `stock_quantity_unknown`), pas compté en rupture ; quantité non
modifiable à la main (409 `stock_movement_required`) — seulement par mouvement
(`entrée`, `sortie`, `inventaire`, `perte`), chaque mouvement renvoie
`quantity_after` ; perte/casse exige un motif.

**Règles de commandes** :
- `GET|PUT /stations/{id}/order-rules` — `settings:view` / `settings:manage` ;
- sept réglages appliqués là où le serveur décide (création, encaissement,
  changement de statut, annulation) — codes de refus exacts de `docs/api.md` ;
- la réponse expose `statuses` : les sept états et leurs libellés.

**Vérification** : sonde matériel/stock (mouvements, états, refus) et sonde
règles (chaque réglage provoque son refus précis).

---

## Lot 8 — Plateforme support (`routes_plateforme.go`)

Réservé au `superadmin` (**double contrôle** : permission `platform:manage`
puis vérification du rôle dans le handler) :
- `GET /platform/overview` (agrégats : entreprises actives, MRR, formules) ;
- `GET|PATCH /platform/organizations/{id}` (suspension réelle, pas seulement
  le nom — le `RAPPORT-ANALYSE.md` pointe ce défaut historique) ;
- `POST /platform/organizations/{id}/subscription` (changement de formule) ;
- `POST /platform/organizations/{id}/registry/{code}/verify` (atteste une
  mention : 409 si vide, trace, annulation si la valeur change ensuite) ;
- `GET|PATCH /platform/plans/{id}`, `GET /platform/audit` ;
- `GET /platform/plans` : public, sans session (tarifs de la vitrine).

**Vérification** : sonde plateforme (403 pour tout rôle non-superadmin, y
compris owner, suspension qui suspend vraiment).

---

## Règles transversales, valables pour chaque lot

1. **Isolation** : chaque requête filtre par `organization_id` de la session ;
   un objet d'une autre entreprise répond **404**, jamais 403 (ne pas révéler
   l'existence). 45 tests d'isolation existent déjà comme référence.
2. **Permissions** : une route = une permission déclarée via `rt.proteger(...)` ;
   le refus vient toujours du serveur, le front ne fait que masquer.
3. **Réponses** : erreurs en `{error: {code, message, fields?}}` ; montants
   entiers XOF (`..._xof`) ; dates RFC 3339 ; listes longues avec `limit`.
4. **Audit** : toute écriture passe par `a.journaliser(...)` avec un nom d'action
   (`service.create`, `price_rule.update`, `cash.payment.void`, …).
5. **Aucun montant inventé** : un prix vient toujours de `price_rules`/catalogue,
   jamais du client ; « pas de prix » n'est jamais « zéro ».
6. **Transactions** : écritures en bloc (matrice, activation en bloc) validées
   **puis** écrites dans la même transaction — rien à moitié.
7. **Tests exigés avant de passer au lot suivant** : `go build ./...`,
   `go vet ./...`, `npm run lint && npm run typecheck`,
   `node scripts/verifications/coherence-routes.mjs`, la sonde du lot
   (python3, contre l'API qui tourne sur :3000), et un parcours navigateur
   (browser-automation) sur l'écran correspondant.
8. **Chaque lot remplace son `enregistrerRoutesXxx(rt) {}`** de
   `routes_restes.go` (ou ajoute son fichier) et met à jour `docs/api.md`
   uniquement si le comportement réel diffère de la spécification.

## Estimation d'effort (indicative, en demi-journées)

| Lot | Contenu | Effort |
| --- | --- | --- |
| 1 | Référentiels véhicules/usages | 1,5 |
| 2 | Catalogue + options | 1,5 |
| 3 | Grille tarifaire + moteur de prix | 2,5 |
| 4 | Équipe + invitations REST | 1 |
| 5 | Lavages + caisse + paiements | 3,5 |
| 6 | Dépenses + rapports + commissions | 2 |
| 7 | Matériel + stock + règles commandes | 2 |
| 8 | Plateforme support | 1,5 |
| — | **Total** | **≈ 15,5 demi-journées** |

## Criteres de fin de projet

- `routes_restes.go` ne contient plus aucun enregistrement vide ;
- le menu complet de l'application (tableau de bord, lavages, prestations,
  tarifs, clients, employés, caisse, dépenses, rapports, commissions,
  paramètres, abonnement, espace laveur, administration) répond 200 avec des
  données réelles et 0 erreur console pour chaque écran rendu ;
- la chaîne `make check` (Go + front + cohérence des routes + rendu des écrans)
  passe intégralement.
