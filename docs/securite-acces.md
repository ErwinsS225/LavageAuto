# Sécurité et contrôle des accès

Ce document décrit ce qui protège les données d'une entreprise, et les limites
assumées. Il s'adresse à la personne qui installe et administre WashGuard.

## 1. Isolation entre entreprises

WashGuard est un SaaS multi-entreprises : chaque entreprise ne voit que ses
données.

- Chaque table métier porte un `organization_id`.
- Chaque requête filtre sur l'entreprise de la session ; les identifiants venus du
  client ne sont jamais utilisés seuls.
- Un objet qui appartient à une autre entreprise répond **`404 not_found`**, pas
  `403` : cela évite de confirmer qu'il existe.
- Un utilisateur rattaché à une station (`caissier`, `laveur`, gérant affecté) est
  en plus limité à celle-ci : les listes sont filtrées, et une autre station de la
  même entreprise répond `404`.

## 2. Rôles

Cinq rôles, définis une seule fois (`rolePermissions`, `backend/app.go`) et lus
par le serveur **comme** par l'interface.

| Rôle | Ce qu'il peut faire | Limites |
| --- | --- | --- |
| `superadmin` | Support WashGuard : tout, plus l'écran Plateforme | Hors entreprise cliente ; double contrôle (permission `platform:manage` **et** rôle vérifié dans le gestionnaire). |
| `owner` | Tout dans son entreprise : stations, réglages, équipe, rapports, abonnements | — |
| `manager` | Exploitation complète : lavages, caisse, dépenses, commissions, réglages de la station, approbation des dépenses | Pas d'`organization:manage` (création de station) ni de gestion d'abonnement |
| `cashier` | Lavages, encaissements, ouverture/clôture de caisse, dépenses (création) | Pas d'annulation de paiement, pas de rapports, pas de réglages, pas de gestion d'équipe |
| `washer` | Ses tâches : prendre en charge, terminer, signaler un incident, ses commissions | Ne voit que ses fiches ; aucun montant de la station |

Le menu de l'interface masque les écrans non autorisés, mais **le refus réel
vient toujours du serveur** : masquer n'est pas protéger.

## 3. Authentification

| Élément | Mise en œuvre |
| --- | --- |
| Identité | Clerk (jeton de session vérifié contre la JWKS) pour les propriétaires ; mot de passe local pour les employés créés par l'entreprise. |
| Mots de passe | Hachés en **bcrypt** (jamais stockés ni journalisés en clair). |
| Session applicative | Cookie `washguard_session`, `HttpOnly`, `SameSite=Lax`, `Secure` quand `SESSION_COOKIE_SECURE=true`. Le jeton de session est stocké **haché** (SHA-256) en base. |
| Webhook Clerk | Signature vérifiée (secret `CLERK_WEBHOOK_SIGNING_SECRET`) avant toute synchronisation. |
| Origine du jeton Clerk | Restreinte par `CLERK_AUTHORIZED_PARTIES` (et `APP_URL`) : un jeton émis pour une autre application est refusé. |

**En développement seulement**, la réponse d'authentification porte aussi le jeton de
session en clair (champ `session_token`) et ce jeton est accepté en
`Authorization: Bearer`. C'est ce qui permet à un aperçu en cadre — où le cookie est
refusé par le navigateur — de fonctionner. Le mécanisme est entièrement fermé dès que
`APP_ENV=production` (voir `backend/apercu_session.go`) : plus de jeton dans le corps,
aucun jeton de session accepté en en-tête. Le paquet compilé du front ne contient alors
plus une seule ligne de ce mécanisme (`import.meta.env.DEV` devient `false`).

Recommandations : activer les **clés `live`** de Clerk en production (les clés
`test` n'authentifient que des comptes de test), renseigner `APP_URL` et
`CLERK_AUTHORIZED_PARTIES`, et activer la double authentification côté Clerk pour
les comptes propriétaires.

## 4. Anti-abus

| Protection | Valeur par défaut | Réglage |
| --- | --- | --- |
| Échecs de connexion | 10 par quart d'heure et par couple IP + identifiant, puis 15 minutes de pause | `AUTH_MAX_FAILURES` |
| Créations de compte anonymes | 5 par heure et par IP | `REGISTER_MAX_PER_HOUR` |
| Formules | Une entreprise ne peut ouvrir plus de stations que sa formule ne le permet (`403 plan_limit`) | Écran Plateforme |
| Suspension | Une entreprise suspendue (impayé, abus) est bloquée à la connexion — ses données restent intactes | Écran Plateforme |

Le limiteur lit `X-Forwarded-For` : derrière un proxy inverse, **seul ce proxy doit
pouvoir joindre l'API**, sinon l'en-tête peut être falsifié pour contourner la
limite.

## 5. Traçabilité

- **Journal d'audit de l'entreprise** (`GET /organizations/audit`) : annulations de
  paiement, annulations de fiche, approbations de dépense, changements de réglages,
  avec auteur et date.
- **Journal de la plateforme** (`GET /platform/audit`) : création d'entreprises,
  changements de formule, suspensions.
- Rien n'est supprimé : un paiement annulé garde sa ligne, son motif et son auteur.
  C'est ce qui permet de reconstituer une journée en cas de contestation.

## 6. Secrets et configuration

- Tous les secrets vivent dans `.env`, **ignoré par Git** (le gabarit
  `.env.example` ne contient aucune valeur). Un secret collé dans le dépôt doit
  être considéré comme compromis : le révoquer et le régénérer.
- Les clés de paiement, d'API et les chaînes de connexion ne doivent jamais
  apparaître dans un écran, une capture d'écran, un ticket ou un journal.
- `bash scripts/verifications/verifier-env.sh dev|prod` contrôle le fichier sans
  jamais afficher une valeur secrète (nom + longueur seulement).
- Rotation : changer une clé Clerk ou un mot de passe de base se fait dans la
  console du fournisseur, puis dans `.env`, puis redémarrage. Aucun redéploiement
  d'interface n'est nécessaire.

## 7. Limites assumées

- WashGuard ne remplace ni une comptabilité, ni un coffre-fort : les montants sont
  ceux que l'équipe saisit, et les écarts de caisse sont **signalés**, pas corrigés.
- Le chiffrement en transit dépend de l'hébergeur de la base (Neon) et du
  transport de l'API (HTTPS en production, voir
  [`deploiement-https.md`](deploiement-https.md)).
- Le poste ordinateur de la station doit être protégé comme n'importe quel poste
  qui contient les recettes du jour : session verrouillée, compte utilisateur
  nominatif.

## 8. Vérifier soi-même

```bash
bash scripts/verifications/verifier-env.sh prod     # configuration de production
make verifier-acces                                 # sondes d'accès sur l'API qui tourne
make check                                          # formatage, tests, routes, écrans
```
