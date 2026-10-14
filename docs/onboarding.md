# Mise en route d'une station (onboarding)

Ce document explique le parcours d'ouverture d'une station : ce qui est demandé, dans quel ordre,
ce que le serveur **constate** avant d'autoriser l'exploitation, et pourquoi.

Il complète trois autres documents :

| Document | Ce qu'il contient |
| --- | --- |
| `docs/api.md` | le contrat exact des routes : permissions, corps, codes d'erreur, payloads |
| `docs/guide-utilisation.md` | l'usage quotidien, une fois la station ouverte |
| `TODO-ONBOARDING.md` | le journal de mise en œuvre, les critères d'acceptation et ce qui reste ouvert |

---

## 1. En deux mots

1. **L'inscription crée l'espace de travail** : une entreprise, sa première station, le compte du
   propriétaire et un abonnement d'essai. Aucune donnée d'exemple n'est ajoutée : il n'y a ni
   client fictif, ni lavage fictif, ni **prix inventé**.
2. **Le propriétaire complète la mise en route** : treize étapes, dont huit obligatoires, réparties
   entre l'assistant de mise en route, les Paramètres, les Prestations et l'écran des Employés.
3. **L'activation ouvre l'exploitation.** Tant qu'elle n'a pas eu lieu, le serveur refuse toute
   écriture de métier (fiche, encaissement, caisse, dépense) avec `409 onboarding_incomplete` et la
   liste de ce qui manque. La lecture, elle, reste ouverte.

Ce qui fait la différence avec un simple formulaire : **rien n'est déclaratif**. L'utilisateur ne
peut pas cocher « étape terminée ». L'état de chaque étape est recalculé à chaque lecture à partir
des données réellement enregistrées, et l'activation revérifie tout dans la transaction qui écrit
l'état.

---

## 2. Les trois états d'un espace

`organizations.onboarding_state` prend trois valeurs, et un seul libellé accompagne chacune :

| État | Libellé affiché | Ce qui se passe |
| --- | --- | --- |
| `pending` | *Mise en route non commencée* | au moins un prérequis obligatoire manque ; les écritures de métier sont refusées (`409 onboarding_incomplete`) |
| `in_progress` | *Mise en route en cours* | idem, mais un formulaire a été marqué « en cours » (`PATCH /onboarding/steps/{code}`) : ce n'est qu'un repère de reprise, pas une preuve |
| `active` | *Espace en exploitation* | l'exploitation est ouverte ; les étapes facultatives restent modifiables |

Deux lectures servent ces états : `GET /onboarding` (le parcours : étapes, avancement, manquants) et
`GET /onboarding/summary` (le récapitulatif complet : identité, comptages, réglages facultatifs,
trace de l'activation — **aucun montant**).

Un compte **tout neuf** n'est jamais vide de tout : l'inscription a déjà créé l'espace, la station,
le catalogue de prestations (18 prestations, non tarifées) et les moyens de paiement (les espèces
sont actives d'emblée). Il démarre donc en `pending` avec l'avancement suivant :

```json
{ "state": "pending", "progress": { "done": 5, "total": 8, "steps_done": 7, "steps_total": 13, "percent": 62 } }
```

Autrement dit : cinq prérequis sur huit sont déjà satisfaits, et il reste **trois gestes** — les
horaires, les catégories de véhicules, la grille tarifaire. C'est exactement ce que l'écran de mise
en route affiche.

---

## 3. Les treize étapes

| # | Code | Titre | Obligatoire | Écran | Ce que le serveur constate |
| --- | --- | --- | --- | --- | --- |
| 1 | `espace` | Espace de travail | oui | — | une entreprise, au moins une station, un propriétaire |
| 2 | `entreprise` | Identité de l'entreprise | oui | `/parametres` | nom **+** téléphone **+** ville de l'entreprise |
| 3 | `station` | Identité et capacités de la station | oui | `/parametres` | nom **+** ville de la station (le reste — capacités, commune, quartier, repère — est signalé comme facultatif) |
| 4 | `horaires` | Horaires d'ouverture | oui | `/parametres` | au moins une journée déclarée |
| 5 | `categories` | Catégories de véhicules | oui | `/onboarding` | au moins une catégorie active pour la station |
| 6 | `usages` | Usages commerciaux | non | `/onboarding` | des usages actifs **et** les combinaisons véhicule × usage |
| 7 | `prestations` | Catalogue des prestations | oui | `/prestations` | au moins une prestation active |
| 8 | `options` | Options, formules et suppléments | non | `/prestations` | au moins une option déclarée |
| 9 | `tarifs` | Grille tarifaire | oui | `/prestations` | au moins une prestation **vendable** avec un prix **utilisable** |
| 10 | `moyens` | Moyens de paiement | oui | `/parametres` | au moins un moyen de paiement actif |
| 11 | `equipe` | Équipe | non | `/employes` | au moins un compte actif (le propriétaire compte) |
| 12 | `materiel` | Matériel et stock | non | `/onboarding` | au moins un équipement ou un article de stock |
| 13 | `commandes` | Règles des commandes | non | `/onboarding` | les règles par défaut suffisent — cette étape est satisfaite d'emblée |

Chaque étape porte, dans la réponse, quatre champs qui évitent à l'écran d'inventer une phrase :
`detail` (le constat, chiffré : « 18 prestations actives »), `hint` (ce qu'il faut faire pour la
satisfaire), `screen` (où le faire) et `form` (le nom du formulaire qui l'alimente — `espace` n'en a
pas, il décrit ce qui existe déjà).

**Pourquoi huit obligatoires seulement.** Une station tenue par son propriétaire, avec un seau et une
éponge, doit pouvoir ouvrir. Les cinq étapes facultatives restent atteignables **après** l'activation
(onglets Paramètres, écran Matériel) : rien n'est perdu, rien n'est caché, et le tableau de bord
rappelle ce qui reste à décrire.

**Une étape peut être « facultative » et pourtant vivante.** Les règles des commandes (`commandes`)
ne bloquent pas : elles s'appliquent par défaut — clients de passage acceptés, contrôle qualité
facultatif, véhicule remis une fois la fiche soldée, acomptes acceptés. L'étape 13 dit « voici ce qui
s'applique », pas « vous n'avez rien fait ».

---

## 4. Ce que l'activation exige

`POST /onboarding/activate` (permission `organization:manage` — le propriétaire, pas le gérant)
revérifie les huit prérequis, et refuse en les nommant.

### 4.1 Le refus

```http
HTTP/1.1 409 Conflict

{
  "error": {
    "code": "onboarding_incomplete",
    "message": "La mise en route n'est pas terminée : 3 étape(s) restent à compléter.",
    "fields": {
      "horaires": "Aucune journée d'ouverture n'est déclarée.",
      "categories": "Aucune catégorie de véhicule n'est activée pour la station.",
      "tarifs": "Aucune prestation vendable n'a de prix utilisable : renseignez la grille tarifaire de vos lavages (une prestation gratuite ou sur devis ne compte pas comme un tarif)."
    },
    "details": {
      "blockers": [
        { "code": "horaires", "title": "Horaires d'ouverture", "hint": "…", "detail": "Aucune journée d'ouverture déclarée", "screen": "/parametres" }
      ]
    }
  }
}
```

Deux formes pour la même vérité : `error.fields` pour un formulaire, `error.details.blockers` pour un
écran qui liste (le code, le titre, le constat, le geste à faire, l'écran qui le règle). L'application
n'a pas à traduire un code en phrase.

### 4.2 Le succès, et pourquoi il est unique

- **Une transaction.** Les prérequis sont revérifiés **dans** la transaction qui écrit l'état, avec
  la ligne de l'entreprise verrouillée (`FOR UPDATE`). Un tarif désactivé entre la vérification et
  l'écriture ne peut pas ouvrir une station qui vendrait à un prix inexistant.
- **Un refus n'écrit rien** : ni état, ni horodatage, ni ligne de journal. Ce qui est vérifié est ce
  qui est écrit.
- **La trace** : `organizations.onboarding_activated_at`, `onboarding_activated_by`,
  `onboarding_activated_by_name` et `onboarding_snapshot` (prérequis satisfaits, comptages du
  catalogue, journées déclarées, réglages facultatifs restants), plus une ligne `audit_logs`
  (`onboarding.activated`) — dans la même transaction.
- **Idempotence** : une seconde activation répond `200` avec `already_active: true` et n'ajoute
  aucune trace.

### 4.3 Ce que l'activation ouvre exactement

Tant que l'état n'est pas `active`, **cinq** écritures de métier sont refusées, avec
`409 onboarding_incomplete` et les étapes manquantes dans `error.fields` :

| Route | Ce qu'elle fait |
| --- | --- |
| `POST /wash-orders` | enregistrer une fiche de lavage |
| `POST /wash-orders/{id}/payments` | encaisser sur une fiche |
| `POST /cash/sessions` | ouvrir la caisse |
| `POST /cash/payments` | encaisser hors fiche |
| `POST /expenses` | enregistrer une dépense |

Tout le reste est lisible (tableau de bord, rapports, listes, réglages) : mettre en route ne se fait
pas les yeux bandés. Le caissier et le laveur ne voient pas le parcours du tout (`403` sur
`/onboarding` et `/onboarding/summary`, et rien dans `/auth/me`).

---

## 5. Après l'activation

- Les écrans d'exploitation deviennent accessibles ; l'état de la station suit ses horaires :
  « Ouverte jusqu'à 18:00 », « En pause jusqu'à 13:00 », « Ouvre aujourd'hui à 08:00 », « Fermée
  aujourd'hui (dimanche) », « Ouverte (jusqu'à 02:00 le lendemain) ». Une journée déclarée ouverte
  **sans heures saisies** ne compte pas comme une journée d'ouverture — l'étape « horaires » le dit
  en clair plutôt que de compter une journée vide.
- Le tableau de bord et le récapitulatif lisent **le même bloc `setup`** calculé par le serveur :
  ils ne peuvent pas se contredire. Avant la première fiche, les blocs financiers sont **retirés**
  (pas mis à zéro : un « 0 FCFA » ressemble à un résultat) et remplacés par des phrases du type
  « Aucune commande enregistrée : aucun montant ne peut être affiché tant que la première fiche
  n'existe pas. »
- Les étapes facultatives non remplies restent listées dans `setup.optional_settings`
  (`state: empty | partial | done`), avec l'écran qui les règle.
- Modifier un prix après l'activation est normal et **n'altère aucune fiche existante** : une fiche
  garde le prix et le motif qu'elle a enregistrés.

---

## 6. Les référentiels du parcours

### 6.1 Catégories de véhicules et usages commerciaux

Le produit apporte un référentiel commun, la station choisit ce qu'elle accepte :

- **13 catégories de véhicules** réparties en **5 familles** (véhicules légers, SUV/4x4 et
  utilitaires légers, deux-roues et assimilés, transport collectif, poids lourds) ;
- **18 usages commerciaux** (taxi, livraison, flotte, particulier…), facultatifs ;
- des **combinaisons véhicule × usage** : une berline « taxi compteur » reste une berline, mais peut
  avoir son prix.

Une catégorie désactivée disparaît des choix de la caisse sans supprimer l'historique des fiches qui
l'utilisaient.

### 6.2 Prestations et options

18 modèles de prestations sont installés avec la station (lavage extérieur, intérieur, complet,
sièges, moteur…). Le propriétaire les active, les renomme, en ajoute, et déclare **pour quels
véhicules** chacune est disponible (`PUT /services/{id}/compatibilities`). Une prestation peut être
**gratuite** (`is_free`) ou **sur devis** (`is_quote_only`) : ce sont des décisions explicites, pas
des prix manquants.

Les **options** (tapis, cire, désinfection) se facturent par lavage : trois lavages avec une option à
500 FCFA portent 1 500 FCFA d'options.

### 6.3 Grille tarifaire

Le catalogue dit **ce qui est vendu**, la grille dit **à quel prix**. Le prix n'est jamais envoyé par
le navigateur : la caisse envoie des choix, le serveur répond le prix **et son origine**
(`price_source`). L'ordre de résolution, du plus précis au plus général, est appliqué par
`backend/pricing.go` :

| Niveau | Source | Critère |
| --- | --- | --- |
| 1 | `client_contract` | prestation + client (contrat, flotte) |
| 2 | `service_category_usage` | prestation + type de véhicule + usage |
| 3 | `service_category` / `service_usage` | prestation + véhicule **ou** prestation + usage |
| 4 | `service_generic` | tarif du catalogue (`services.price_xof`) |
| — | `free` | prestation déclarée gratuite : rien ne prime sur cette décision |
| — | — | aucun tarif trouvé ⇒ `409 price_required` (la vente est refusée, pas devinée) |

Deux règles de même spécificité se **signalent** (`409 price_rule_conflict`) au lieu d'être
arbitrées. La revue de prix en bloc (`PUT /price-rules/matrix`) exige un **motif** : modifier un prix
est une décision, elle se justifie. Une case vidée désactive la règle (elle ne la supprime pas).

**C'est ce qui rend l'étape 9 obligatoire.** Un catalogue installé mais non tarifé est une promesse
creuse : la station n'ouvre pas tant qu'aucune prestation vendable n'a de prix utilisable. La
récapitulatif le dit sans détour — « 17 prestations vendables — aucun prix renseigné » — et l'écran
Prestations affiche ce qui reste « à tarifer » (`a_tarifer` calculé par le serveur).

### 6.4 Moyens de paiement et caisse

Chaque station reçoit **9 moyens de paiement** à sa création — Espèces, Orange Money CI, Wave,
MTN MoMo, Moov Money, Carte bancaire, Virement bancaire, Paiement différé, Autre — et **seules les
espèces sont actives d'emblée** : les autres s'activent au besoin. Les moyens n'existent que déclarés : un encaissement sur un moyen inconnu est refusé.

Les réglages de caisse (11) sont posés par défaut et modifiables station par station — caisse
obligatoire pour les espèces, remise maximale du caissier, comptage à la clôture, réouverture
motivée. Ils ne bloquent pas l'activation : leurs valeurs par défaut sont prudentes et lisibles.

### 6.5 Équipe, matériel et stock

- **Équipe** : un compte par personne, créé par le propriétaire (mot de passe provisoire affiché une
  seule fois) ou rejoint par invitation. La rémunération est une donnée à part
  (permission `employee:compensation`, jamais accordée au gérant).
- **Matériel** : le référentiel propose 18 modèles (jet, aspirateur, groupe, compresseur…), chacun
  avec son état (en service, en panne, en maintenance) — un équipement en panne est un constat, pas
  une suppression.
- **Stock** : « on n'a pas compté » n'est pas « zéro » (`quantity_known`). Une quantité ne s'écrit
  jamais à la main : elle bouge par un **mouvement** (`entrée`, `sortie`, `perte`, `inventaire`), et
  l'historique explique la quantité courante. Sortir d'un stock non compté est refusé
  (`409 stock_quantity_unknown`).

---

## 7. Rejoindre une entreprise existante

Créer un espace et le rejoindre sont deux chemins distincts. Une invitation **ne crée jamais**
d'entreprise, de station ni d'abonnement : elle rattache une personne à ce qui existe déjà, avec un
rôle opérationnel (`manager`, `cashier`, `washer` — `owner` ne s'obtient pas par un lien).

- Deux identifiants désignent la même invitation : un **code de 8 caractères** (sans `0/O/1/I/L`,
  dictable au téléphone) et un **jeton long** dans le lien. Seule l'empreinte du jeton est stockée :
  il ne s'affiche qu'à la création.
- L'invitation se déclare par **e-mail ou téléphone** ; `GET /invitations/{code}` est public mais ne
  révèle **aucun identifiant** (nom de l'entreprise, ville, rôle, état) et un code inventé répond
  `404` sans rien apprendre.
- **WashGuard n'envoie aucun message** : ni e-mail, ni SMS, ni WhatsApp. Le lien et le code sont à
  transmettre par la personne qui invite (l'interface propose un partage WhatsApp prérempli). C'est
  une limite assumée, écrite dans `TODO.md`.
- Pour rejoindre, `POST /auth/register` accepte `invitation_token` : le compte est créé dans
  l'entreprise invitante avec le rôle et la station de l'invitation, et l'invitation est consommée
  dans la même transaction. Un compte qui appartient déjà à une entreprise n'est pas rattaché de
  force à une seconde (`409 invitation_requires_login`) : l'appartenance à une entreprise est unique
  en V1.

---

## 8. Ce que le serveur refuse, et pourquoi

| Réponse | Code | Ce que ça veut dire |
| --- | --- | --- |
| `409` | `onboarding_incomplete` | la station n'est pas activée : l'écriture de métier est refusée, les étapes manquantes sont nommées |
| `409` | `price_required` | aucun tarif utilisable pour cette combinaison : on ne vend pas au hasard |
| `409` | `price_rule_conflict` | deux règles de même spécificité : à trancher, pas à arbitrer |
| `409` | `stock_quantity_unknown` | sortie de stock demandée sur un stock non compté |
| `409` | `stock_movement_required` | quantité écrite à la main au lieu de passer par un mouvement |
| `403` | `plan_limit` | la formule en cours (Essai : 1 station, 3 comptes, 300 lavages/mois) est atteinte — le refus nomme la formule et l'écran Abonnement |
| `403` | permission | le rôle n'a pas ce droit (ex. un caissier ne pose pas de prix, un gérant n'active pas la station) |
| `404` | `not_found` | la ressource n'existe pas **pour cette entreprise** : c'est aussi la réponse à une tentative d'accès à une autre entreprise (l'existence d'une donnée étrangère n'est pas révélée) |
| `400` | `validation_error` | la valeur est refusée avant écriture (jour qui ferme avant d'ouvrir, prix négatif, étape inconnue…) |

Ces refus sont testés : `backend/e2e_lot9_test.go` rejoue 45 tentatives d'accès d'une entreprise sur
les données d'une autre (lecture, écriture, désactivation), vérifie qu'aucune n'aboutit, que
l'entreprise visée est **intacte** après les attaques, et que chaque route ainsi contrôlée existe
bien pour la propriétaire — un `404` de route inconnue ne doit pas faire passer un test d'isolation.

---

## 9. Ce que cette version ne fait pas

Écrire ce qui n'existe pas évite de le chercher :

- **Aucun envoi automatique** d'e-mail, de SMS ou de WhatsApp (aucun fournisseur n'est branché).
- **Aucune donnée d'exemple** n'est injectée dans une entreprise cliente : ni client, ni lavage, ni
  chiffre d'affaires de démonstration. Sur une base de recette, les données de démonstration
  n'apparaissent que si `SEED_DEMO=true` **et** que la base est vide.
- **Aucun prix pré-rempli** : les 18 prestations arrivent à `0` et la station n'ouvre qu'après la
  grille tarifaire. Un prix inventé serait pire qu'un prix manquant.
- **Pas de rôles personnalisés** (superviseur, responsable de stock, comptable) : la V1 s'en tient
  aux cinq rôles existants — propriétaire, gérant, caissier, laveur, support.
- **Pas de multi-espaces** : un compte appartient à une seule entreprise.
- **Pas de stock avancé** : ni règles de consommation par prestation, ni seuils d'alerte (V2).
- Le matériel et le **stock** ne sont pas des prérequis d'activation (ils peuvent être déclarés
  après), et l'**usage commercial** reste facultatif.

---

## 10. Vérifier le parcours soi-même

```bash
# 1. Le serveur et l'interface (deux terminaux)
make api                        # API Go (lit .env, port 8080 en développement)
make web                        # interface Vite (port 5173)

# 2. Le parcours complet sur une base **vierge** (aucune donnée de démonstration)
createdb washguard_neuve && DATABASE_URL=…washguard_neuve… make migrate
DATABASE_URL=…washguard_neuve… SEED_DEMO=false API_PORT=8099 ./washguard-api &
python3 scripts/verifications/sonde-onboarding.py http://127.0.0.1:8099   # inscription → 13 étapes → activation
python3 scripts/verifications/sonde-lot8.py       http://127.0.0.1:8099   # récapitulatif → activation → 1re fiche → tableau de bord
python3 scripts/verifications/sonde-tarifs.py     http://127.0.0.1:8099   # grille tarifaire et sélection des prix

# 3. Les tests qui tiennent le contrat
make test-e2e TEST_DSN="$DATABASE_URL"   # 65 tests Go, dont l'isolation entre entreprises
make check                               # formatage, compilation, types, routes, écrans, configuration
```

Les chiffres relevés le **2026-10-14** sur une base créée à l'instant (migrations `0001` → `0012`) :

| Vérification | Résultat |
| --- | --- |
| Tests Go (`go test -count=1 ./...`) | **65/65**, dont 4 nouveaux pour l'isolation, les rôles et les plafonds de formule |
| `sonde-onboarding.py` (base vierge) | 41/41 |
| `sonde-lot8.py` (base vierge) | 75/75 |
| `sonde-tarifs.py` · `sonde-vehicules.py` (base vierge) | 37/37 · 36/36 |
| `sonde-api.py` · `sonde-invitations.py` (base vierge) | 21/21 · 23/23 |
| `sonde-lot7.py` · `sonde-suspension.py` (base vierge) | 56/56 · 1/1 |
| `sonde-fiche-payee.py` · `sonde-metier.py` (base de démonstration neuve) | 22/22 · 35/35 |
| Campagne complète sur base peuplée (`make verifier-acces`) | **366 contrôles, 0 échec** |
| Écrans rendus (`rendu-pages.mjs`) | 18/18 |
| Aperçu réel via le navigateur d'aperçu (`verif-apercu.py`) | 15/15 |

---

## 11. Documents liés

- `docs/api.md` — le contrat des routes (Mise en route, Invitations, Prestations et grille tarifaire,
  Moyens de paiement, Tableau de bord).
- `docs/guide-utilisation.md` — la journée d'une station.
- `docs/interface.md` — les écrans et la navigation.
- `docs/securite-acces.md` — rôles, permissions et isolation.
- `TODO-ONBOARDING.md` — le plan suivi lot par lot, avec les résultats réellement obtenus.
