# Sondes d'acceptation

Le dépôt n'avait **aucun test**. Ces sondes couvrent les garanties qui cassent
le plus cher en production : cloisonnement entre entreprises, droits par rôle,
règles de caisse et de remise, contrat d'API attendu par le front, et — depuis
l'onboarding — la chaîne *formulaire → base → relecture*.

Elles n'utilisent que la bibliothèque standard de Python 3 : aucun paquet à
installer, aucun cadre de test à apprendre. Chaque contrôle affiche `OK` ou
`KO`, et la commande renvoie un code de sortie non nul en cas d'échec — elles
sont donc utilisables telles quelles dans une chaîne d'intégration.

## Configuration avant tout

```bash
bash scripts/verifications/verifier-env.sh dev    # ou prod
```

Le contrôle dit ce qui est renseigné dans `.env`, ce qui manque, et refuse les
incohérences (fichier suivi par git, port invalide, production avec les données
de démonstration, `SameSite=none` sans HTTPS, valeur restée au gabarit). Il
n'affiche jamais la valeur d'un secret : seulement sa présence et sa longueur.

## Préparation

L'API doit tourner avec les données de démonstration (`SEED_DEMO=true`), sur
une base **neuve** : le seed ne crée l'entreprise de démonstration que si la
base n'en contient aucune, et les vérifications de plafond de formule supposent
une entreprise fraîchement inscrite.

Depuis le lot 4, **une station neuve n'a ni horaires, ni catégorie activée, ni
tarif** : elle ne peut pas être activée avant d'avoir dit ce qu'elle lave.
Depuis la mise en route, **une station neuve n'a ni horaires ni tarif** :
l'application n'invente aucun prix et le serveur refuse l'exploitation
(`409 onboarding_incomplete`) tant que l'exploitant n'a pas déclaré ses horaires
et créé ses prestations tarifées. Les sondes qui exploitent une entreprise la
configurent donc d'abord (`mettre_en_route(...)` dans `sonde-api.py`).

Le seed crée aussi le compte plateforme annoncé par le README
(`admin@washguard.app` / `admin1234`, hors production) : la sonde API s'en sert
pour vérifier que l'espace `/administration` est réellement utilisable.

```bash
# base locale (exemple)
export DATABASE_URL='postgresql://postgres@localhost:5432/washguard?sslmode=disable'

# serveur
cd backend
API_PORT=8099 SEED_DEMO=true go run .
```

## Exécution

```bash
# Cohérence des routes et des écrans (aucun serveur nécessaire, Node seul)
node scripts/verifications/coherence-routes.mjs

# Rendu des pages : détecte l'« écran noir » (erreur au rendu React)
node scripts/verifications/rendu-pages.mjs

python3 scripts/verifications/sonde-api.py           # accès, rôles, contrat d'API
python3 scripts/verifications/sonde-onboarding.py     # formulaires → base → relecture
python3 scripts/verifications/sonde-tarifs.py         # catalogue à tarifer, grille, options
python3 scripts/verifications/sonde-invitations.py    # inviter un collègue, le rejoindre
python3 scripts/verifications/sonde-vehicules.py     # catégories de véhicules et usages
python3 scripts/verifications/sonde-metier.py        # caisse, remises, soldes, plafonds
python3 scripts/verifications/sonde-fiche-payee.py   # fiche encaissée : verrouillage
python3 scripts/verifications/sonde-suspension.py    # entreprise suspendue : accès coupé
```

L'URL de base se passe en premier argument (`http://127.0.0.1:8099` par
défaut).

Deux limites de l'environnement, à connaître avant de conclure à un échec :

- **Cinq inscriptions par heure et par IP** (`REGISTER_MAX_PER_HOUR`, 5 par
  défaut). Une campagne complète en crée une dizaine : lancer le serveur avec
  une valeur plus haute (`REGISTER_MAX_PER_HOUR=60`) pour dérouler les huit
  sondes d'affilée. Sinon la réponse est `429 too_many_requests`, pas un bug.
- La sonde de suspension a besoin d'un client PostgreSQL (`WASHGUARD_PSQL`).

### Aperçu distant : la sonde `verif-apercu.py`

Quand l'interface est servie sous un autre domaine que l'API — cas d'un aperçu
en cadre — le cookie de session est un cookie **tiers**, et le navigateur ne le
conserve pas : chaque écran répondait `401` alors que la session venait de
s'ouvrir (« Vous devez vous connecter » juste après l'inscription).

`verif-apercu.py` rejoue ce navigateur-là, **sans jamais conserver de cookie** :
il s'inscrit, garde le `session_token` que la réponse d'authentification porte
hors production, et ne présente plus que `Authorization: Bearer` — exactement ce
que fait `src/lib/api.ts` en développement. Il vérifie ensuite les quinze points
du parcours : session reconnue sans cookie, dix étapes de mise en route, moyens
de paiement (espèces actives d'emblée), fiche d'entreprise, prestation créée
**sans prix**, grille tarifaire, catégorie de véhicule, `401` sans jeton comme
avec un jeton inventé, cookie de session qui fonctionne toujours, et
déconnexion qui ferme réellement la session.

```sh
# API sur 8080, interface (vite) sur 3000
VITE_ALLOWED_HOSTS='.arena.site,.e2b.app,localhost,127.0.0.1' npm run dev:web
python3 scripts/verifications/verif-apercu.py
```

L'hôte d'aperçu se règle en tête du script (`HOST`) et doit aussi figurer dans
`VITE_ALLOWED_HOSTS`, sinon vite refuse la requête (`403 Blocked request`).

Deux réglages restent nécessaires pour qu'un **navigateur** conserve le cookie
dans ce cadre, et ne concernent que l'installation : `SESSION_COOKIE_SAMESITE=none`
avec `SESSION_COOKIE_SECURE=true` derrière HTTPS. Rien de tout cela n'est requis
sur une installation normale, où l'API sert elle-même l'interface compilée.

## Détail

| Sonde | Ce qu'elle vérifie |
| --- | --- |
| `sonde-api.py` | Une entreprise ne lit ni ne modifie les données d'une autre (404), les rôles sont contrôlés (caissier/laveur → 403), le prix d'une prestation vient du catalogue et non du client, et les routes principales renvoient la forme attendue par le front (`/stations`, `/employees/options`, `/cash/current`, `/reports/overview`, `/commissions`, `/washer/*`). |
| `sonde-onboarding.py` | Les formulaires de la mise en route écrivent réellement dans la base : le corps exact construit par chaque écran est envoyé puis relu par une autre route (`GET /organizations/current`, `GET /stations/{id}/profile`). Vérifie aussi les refus utiles — e-mail et date invalides refusés **champ par champ**, horaires incohérents refusés **par jour** sans abîmer la semaine enregistrée —, la distinction « non renseigné » (la question n'a pas été posée) / « non », les **dix étapes** du parcours et les neuf formulaires annoncés (`entreprise`, `station`, `horaires`, `vehicules`, `usages`, `prestations`, `options`, `tarifs`, `moyens`), la prestation créée **sans prix** puis tarifée par la grille (`PUT /price-rules/matrix`), les **moyens de paiement** de la 10ᵉ étape (référentiel posé, espèces seules actives, activation/renommage relus, dernier moyen actif non désactivable), les **paramètres de caisse** (onze réglages enregistrés puis relus, rôle inconnu refusé champ par champ), l'activation qui n'aboutit que lorsque tout est satisfait, et le cloisonnement (404 pour une autre entreprise, y compris sur les paramètres de caisse). |
| `sonde-tarifs.py` | Le lot 5 de bout en bout : un compte neuf reçoit un catalogue **équipé mais sans aucun prix** (un tarif inventé finirait sur une facture), une prestation non tarifée est refusée à la vente (`409 price_required`), la hiérarchie s'applique dans l'ordre annoncé (contrat client > prestation × véhicule × usage > prestation × véhicule > tarif du catalogue), la provenance est recopiée sur la fiche, deux règles aussi précises se signalent (`409 price_rule_conflict`), la grille s'enregistre en bloc avec un **motif obligatoire** et une case vidée désactive la règle sans rien supprimer, les options se facturent **par lavage** (une option inconnue ne crée pas de fiche à moitié), une prestation gratuite se vend à zéro, `allow_generic_price: false` retire le repli du catalogue, et aucune grille ne franchit la frontière d'une entreprise (404). |
| `sonde-invitations.py` | Le parcours complet d'une invitation : création (lien + code), aperçu **public** qui ne livre aucun identifiant, code inventé qui ne révèle rien (404), inscription de l'invitée dans l'entreprise invitante sans nouvelle entreprise ni abonnement, invitation non réutilisable (409), compte existant refusé explicitement (409 `invitation_requires_login`), et cloisonnement (une entreprise ne voit ni ne révoque les invitations d'une autre). |
| `sonde-vehicules.py` | Les deux écrans du lot 4 : le référentiel des catégories de véhicules et celui des usages commerciaux sont amorcés par le serveur (relire ne duplique rien), la station **active** ce qu'elle lave au lieu de recopier un catalogue, un nom local prime sur le référentiel et un ré-enregistrement en bloc ne l'efface pas, une catégorie créée par la station porte un code `perso_…`, une combinaison véhicule × usage non déclarée est refusée, **un usage n'est pas une catégorie** (le passer comme catégorie est refusé), une fiche garde les deux libellés après désactivation de la catégorie, et une autre entreprise ne voit ni ne modifie ces réglages (404). |
| `sonde-metier.py` | Ouverture de caisse unique, encaissement complet qui clôt la fiche, acompte qui la laisse ouverte, surpaiement refusé, annulation motivée qui décrémente les compteurs, plafond de remise par rôle, incident laveur visible, dépenses qui sortent de la caisse sans gonfler le chiffre d'affaires, plafond d'essai d'une station. |
| `sonde-fiche-payee.py` | Une fiche portant un encaissement refuse une nouvelle remise, un retour en atelier et une annulation ; les champs descriptifs (plaque, note, client, laveur) restent modifiables. Couvre aussi le **lot 6** : moyen déclaré inactif refusé (`payment_method_disabled`), moyen à référence refusé sans elle (`400` sur le champ `reference`), libellé du moyen recopié sur l'encaissement, numéro de reçu attribué **une seule fois** (réémission idempotente), mentions fiscales absentes **listées** et non inventées, et paiement **différé** qui ne solde pas la fiche, ne bouge aucun compteur de caisse et n'émet pas de reçu. |
| `rendu-pages.mjs` | Rend les **quatorze** pages de l'application hors navigateur, avec la session simulée, et exige le contenu attendu. C'est ce contrôle qui attrape l'« écran noir » : une erreur au rendu React vide toute la page. Il a détecté deux cas réels : `useClerk()` appelé sans Clerk configuré sur /connexion, et l'absence de limite de chargement sur l'espace plateforme. |
| `coherence-routes.mjs` | Chaque lien du menu ou d'une page mène à une route déclarée, chaque route déclarée est atteignable, et chaque permission demandée par un écran existe dans la matrice des rôles du serveur. C'est ce contrôle qui aurait attrapé le menu de la plateforme pointant vers `/admin/*` au lieu de `/administration/*`. |
| `verif-apercu.py` | Le parcours d'un **premier utilisateur dans l'aperçu** : sans aucun cookie conservé (cookie tiers refusé par le navigateur dans un cadre), la session est portée par le `session_token` rendu au corps de la réponse d'authentification puis présenté en `Authorization: Bearer`. Inscription, `/auth/me`, dix étapes de mise en route, moyens de paiement (espèces seules actives), création d'une prestation **sans prix inventé**, grille tarifaire, catégorie de véhicule, refus `401` sans jeton comme avec un jeton inventé, cookie qui fonctionne toujours, et déconnexion qui vide réellement la session. |
| `sonde-suspension.py` | Une entreprise suspendue reçoit 403 `organization_suspended` sur le profil **et** sur les routes métier ; la réactivation rend l'accès immédiatement, sans nouvelle connexion. |

### Accès à la base pour la sonde de suspension

La sonde bascule le statut de l'entreprise directement en base (c'est le geste
d'un administrateur). Elle lit le SQL sur l'entrée standard via la commande
`WASHGUARD_PSQL` (par défaut `psql "$DATABASE_URL"`). Sur un poste où `psql`
n'est pas installé, on peut fournir n'importe quel client équivalent :

```bash
export WASHGUARD_PSQL='psql "$DATABASE_URL"'
# ou, au choix, un petit script qui lit le SQL sur stdin
```
