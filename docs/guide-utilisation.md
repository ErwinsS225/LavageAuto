# Guide d'utilisation — la journée type

Ce guide s'adresse à l'équipe de la station : caissier, gérant, laveur. Il décrit
l'ordre des gestes d'une journée normale. Aucune connaissance technique n'est
nécessaire.

L'adresse est celle affichée par `make adresse` (par exemple
`http://192.168.1.24:3000`). Chacun se connecte avec **son** compte : c'est ce qui
permet de savoir qui a encaissé quoi.

## Première fois : mettre la station en route

À la création du compte, l'application ouvre l'écran **Mise en route** (et y
revient à chaque connexion tant que la station n'est pas activée). Neuf étapes,
chacune vérifiée sur vos données — vous pouvez les remplir dans l'ordre que vous
voulez, et y revenir plus tard depuis **Paramètres** :

1. **Votre espace** — l'entreprise, la station et votre compte sont créés par
   l'inscription.
2. **Identité de l'entreprise** — nom commercial et raison sociale, activité,
   contacts, adresse de facturation, puis le registre administratif (RCCM,
   compte contribuable, CNPS, représentant légal…). Ces mentions sont
   **facultatives** ; chacune garde son état (« Non renseigné », « Renseigné »,
   puis « Vérifié par WashGuard » après contrôle par le support — l'application
   ne se décerne jamais cette mention elle-même).
3. **Identité et capacités de la station** — nom et ville (obligatoires pour
   activer), puis commune, quartier, repère et coordonnées. Les capacités
   (pistes, aspiration, séchage, salle d'attente, réserve d'eau…) sont des
   **déclarations de votre part**, à trois états — oui, non, ou « non
   renseigné ». Répondez seulement si vous savez : l'application ne transformera
   pas un silence en « non ».
4. **Horaires d'ouverture** — les sept jours, avec journée fermée, pause et
   dernière réception. Le bouton « Appliquer le lundi à tous » recopie une
   journée type. Vous pouvez ajouter des journées particulières (férié,
   inventaire, travaux). Au moins un jour ouvert est nécessaire pour activer.
5. **Catégories de véhicules** — ce que votre station accepte de laver :
   citadine, berline, SUV, moto, minibus, camion… Le catalogue est commun à
   toutes les stations ; vous **cochez** ce que vous lavez, vous pouvez renommer
   une entrée (« berline » → « voiture de ville ») et en créer une qui n'existe
   pas. Au moins une catégorie active est nécessaire pour activer : une station
   ne vend pas sans avoir dit ce qu'elle lave. Décocher ne supprime rien — les
   fiches passées gardent le nom qu'elles portaient.
6. **Usages commerciaux** — ce que le client fait de son véhicule : voiture
   personnelle, taxi compteur, taxi communal (wôrô-wôrô), gbaka, VTC, moto-taxi,
   livraison, entreprise, flotte… Pour chaque usage que vous activez, cochez les
   véhicules concernés. Un usage **n'est pas** une catégorie : une berline « taxi
   compteur » reste une berline, ce qui vous permet de compter vos berlines et de
   savoir lesquelles étaient des taxis. Cette étape est facultative.
7. **Prestations** — le catalogue : reprenez une prestation de référence
   (lavage extérieur, intérieur complet, jantes, cire…) ou décrivez la vôtre.
   Le contenu et la durée sont décrits, **jamais le prix** : l'application ne
   propose aucun tarif, un prix inventé finirait sur une facture. Indiquez ici
   ce qui est **gratuit** (un contrôle offert) ou **chiffré sur devis**
   (céramique, rénovation). Une prestation naît « à tarifer » : elle n'est pas
   encaissable tant que la grille ne la couvre pas (refus `price_required`).
   Cette étape sert aussi à dire à quels véhicules chaque prestation s'applique.
8. **Options et suppléments** — ce qui se facture **en plus** d'une prestation :
   lavage des tapis, cire, désinfection. Vous fixez le supplément et, si vous le
   souhaitez, la prestation ou le type de véhicule concerné. Une formule, elle,
   est une prestation qui regroupe plusieurs opérations : elle se crée à l'étape
   précédente. Cette étape est facultative ; sans option déclarée, la caisse n'en
   propose aucune.
9. **Grille tarifaire** — vos prix, une fois, pour de bon. C'est le seul endroit
   où un tarif se pose : par type de véhicule, par usage commercial, par client
   sous contrat, ou au niveau du catalogue (le repli). Chaque enregistrement
   porte un **motif** — une hausse de tarif doit pouvoir s'expliquer des mois
   plus tard. La grille indique ce qui reste **à tarifer** ; ces prestations
   restent invendables jusque-là, et la caisse le dit clairement.

À la caisse, ces deux listes apparaissent sur la fiche : la catégorie de
véhicule et l'usage — et le prix s'affiche avec **son origine** (« tarif
berline », « contrat flotte », « tarif du catalogue »), pour qu'aucun montant
n'arrive sans explication. Seules les combinaisons que vous avez déclarées sont
proposées — impossible d'enregistrer un couple que votre station ne fait pas.
Renommer ou désactiver une entrée plus tard ne change pas les fiches déjà
écrites.

Quand tout est vert, le bouton **Activer ma station** devient disponible (le
propriétaire seul peut l'actionner ; le serveur revérifie les prérequis). Avant
l'activation, les fiches, les encaissements, la caisse et les dépenses sont
refusés : c'est volontaire, l'application n'ouvre pas l'exploitation tant que
les tarifs ne sont pas les vôtres. La lecture reste possible (tableau de bord,
rapports).

## Tarifer : la grille tarifaire

Écran **Prestations** → **Grille tarifaire** (ou **Tarifs** dans le menu). Un
seul principe : **le serveur calcule, vous décidez**. La caisse n'envoie jamais
un montant ; elle envoie la prestation, le véhicule, l'usage, le client et les
options — le serveur répond le prix, et d'où il vient.

Quatre niveaux, du plus précis au plus général :

| Niveau | Exemple | Prime sur |
| --- | --- | --- |
| Contrat client | « Flotte Taxi Plateau : 1 800 FCFA » | tout |
| Prestation + véhicule + usage | « Berline taxi compteur : 3 000 » | les niveaux 3 et 4 |
| Prestation + véhicule | « Berline : 2 500 » | le niveau 4 |
| Tarif du catalogue | « Lavage extérieur : 2 000 » | rien — c'est le repli |

En pratique : tapez un montant dans une case, indiquez le **motif**, enregistrez.
Vider une case **retire** le tarif (la vente retombe sur le niveau suivant) —
la règle n'est pas supprimée, les fiches passées gardent leur prix. Deux règles
aussi précises l'une que l'autre sont refusées (`price_rule_conflict`) : le
serveur ne choisit pas à votre place.

Trois pièges évités :

- **Une prestation gratuite ne se tarife pas** : marquez-la « gratuite » à
  l'étape Prestations ; elle se vend à zéro, et aucune règle ne peut la couvrir.
- **Un tarif par usage suppose l'usage activé** : activez « taxi compteur » et
  les véhicules concernés avant de poser son prix.
- **Retirer le repli du catalogue** (Paramètres de la station →
  *Autoriser le tarif du catalogue*) : la grille devient la seule source. Une
  prestation non tarifée est alors refusée à la caisse, même si elle porte
  encore un ancien prix de catalogue.

## Inviter un collègue

Depuis **Paramètres → Invitations**, vous créez une invitation pour un caissier,
un laveur ou un gérant. Vous obtenez deux choses :

- un **lien** (`…/rejoindre?jeton=…`), affiché une seule fois ;
- un **code** de huit caractères, que vous pouvez dicter au téléphone et qui
  désigne la même invitation.

**WashGuard n'envoie pas le message à votre place** : il n'y a ni e-mail, ni SMS,
ni envoi WhatsApp automatique. Vous transmettez le lien ou le code comme vous le
feriez d'un mot de passe — le bouton « Partager sur WhatsApp » ouvre votre
application avec le message déjà rédigé, c'est vous qui l'envoyez.

Votre collègue ouvre le lien (ou va sur `/rejoindre` et saisit le code) : il voit
le nom de votre entreprise et le rôle prévu, puis crée son compte. Il rejoint
votre entreprise **sans en devenir propriétaire**, et n'a aucun parcours de mise
en route à faire : la station est déjà configurée. Une invitation ne sert qu'une
fois ; passé son délai (14 jours par défaut), elle expire et vous en créez une
nouvelle.

Si la personne a déjà un compte WashGuard, elle ne peut pas rejoindre une seconde
entreprise avec la même adresse : le serveur le refuse explicitement
(`invitation_requires_login`). Invitez-la sur une autre adresse, ou utilisez un
compte différent.

## Le matin : ouvrir la caisse

1. Écran **Caisse** → **Ouvrir la caisse**.
2. Saisir le **fonds de caisse** (l'espèce présente dans le tiroir au départ).
3. Valider.

Une seule caisse est ouverte à la fois : c'est elle qui compte les espèces, les
mobile money et les autres règlements de la journée. Sans caisse ouverte,
l'encaissement en espèces est refusé (avec un message qui renvoie à cet écran) —
c'est volontaire, sinon les journées finissent sans point de comparaison.

## Enregistrer un lavage

Écran **Lavages** → **Nouveau lavage** :

1. **Client** : choisir un client existant, ou saisir un nom (un client de passage
   n'a pas besoin de fiche).
2. **Véhicule** : catégorie et plaque (utile pour retrouver la fiche).
3. **Prestations** : cocher dans le catalogue. Le prix vient de la **grille
   tarifaire** et s'affiche avec son origine ; la quantité se corrige ligne par
   ligne. Si une combinaison n'a pas de tarif, la ligne est signalée « à
   tarifer » et la fiche est refusée : mieux vaut le savoir avant l'encaissement
   qu'après.
4. **Options** : cocher ce qui s'ajoute au lavage (tapis, cire, désinfection).
   Le supplément se facture par lavage — deux véhicules, deux fois le
   supplément.
5. **Laveur** : la personne qui prend le véhicule. C'est ce qui alimente ses
   commissions.
6. Enregistrer : la fiche apparaît dans la liste du jour, avec son total.

Si le client change de véhicule en cours de route, modifiez la fiche : le prix
se recalcule selon la grille. Une fiche déjà encaissée, elle, est verrouillée
sur son montant — c'est le paiement qui fait foi.

## Choisir ses moyens de paiement et son reçu

**Paramètres → Caisse et reçus**. La station démarre avec le référentiel posé
(espèces, Orange Money, MTN MoMo, Moov Money, Wave, carte, virement, différé,
autre) mais **seules les espèces sont actives** : on active ce qu'on accepte
réellement, on renomme pour coller à l'usage (« Orange Money CI »), et on
supprime — c'est-à-dire **désactive** — le reste. Le dernier moyen actif ne peut
pas être retiré : une caisse sans moyen n'encaisse rien.

Le même écran règle : le fond de caisse proposé à l'ouverture, les rôles
autorisés à ouvrir et clôturer, l'écart toléré avant justification écrite,
l'autorisation d'annuler un encaissement, et l'identité du reçu (préfixe du
numéro, raison sociale, identifiant fiscal, logo, mention de pied). Ces
mentions ne sont **pas devinées** : ce qui n'est pas renseigné n'est pas
imprimé, et l'aperçu du reçu porte la mention « Exemple ».

## Encaisser

- Depuis la fiche : **Encaisser**, choisir le moyen de paiement — ceux que la
  station a activés — puis saisir le montant. Un moyen qui exige une référence
  (mobile money, virement, chèque) demande le numéro de transaction ; le serveur
  refuse l'encaissement sans elle.
- Si la fiche est **soldée par cet encaissement**, le **reçu** s'affiche dans la
  foulée, avec son numéro définitif. Il reste consultable depuis la fiche
  (bouton **Copier** : le texte est prêt à coller dans un message au client).
- Un **paiement différé** (client connu qui règle plus tard) se choisit comme
  les autres, avec l'accord du gérant. Il **ne solde pas** la fiche : le reste
  dû demeure, la caisse ne bouge pas, et le reçu n'est pas émis.
- Un **acompte** est accepté : la fiche reste ouverte, le **reste à payer**
  s'affiche (`due_xof`). Un montant supérieur au reste dû est refusé.
- Une **fiche soldée** passe automatiquement en `Terminée` et reçoit son reçu.
- Une erreur de saisie ne se supprime pas : écran **Caisse**, carte
  *Encaissements récents* → bouton **Annuler** (motif obligatoire). L'opération
  reste dans l'historique, marquée annulée, et le solde de la fiche est recalculé.

## La journée du laveur

Le laveur ouvre **Mon poste** : il y voit uniquement ses véhicules, répartis entre
*Mes tâches à faire*, *Terminés aujourd'hui* et *Historique*.

| Étape | Bouton | Effet |
| --- | --- | --- |
| Prendre le véhicule | **Prendre en charge** | La fiche passe « en cours ». |
| Terminer | **Terminer le lavage** | La fiche passe « faite » ; elle compte dans les lavages du jour. |
| Signaler un problème | **Signaler un incident** | Rayure, choc, état des lieux : le message est joint à la fiche et visible par le gérant. |

Le laveur ne voit ni les montants encaissés de la station, ni les fiches de ses
collègues : son écran ne montre que ses tâches, ses compteurs et ses commissions.

## Dépenses et petites sorties

Écran **Dépenses** → **Nouvelle dépense** : montant, catégorie, justificatif
éventuel. Une dépense en espèces réduit le montant théorique attendu en caisse —
elle est donc visible dans l'écart de clôture.

Seul un profil habilité peut approuver ou annuler une dépense.

## Fin de journée : clôturer la caisse

1. Écran **Caisse** → **Clôturer**.
2. Compter l'espèce réellement présente et saisir le montant.
3. WashGuard affiche l'**écart** entre le théorique (fonds + encaissements −
   dépenses en espèces) et le compté. Un écart se commente dans la note.
4. Valider : la session est archivée, consultable dans l'historique de la caisse.

Une clôture peut être **rouverte** par un profil habilité si une erreur est
constatée le soir même : la correction est journalisée.

## Rapports et commissions

- **Tableau de bord** : l'essentiel du jour (chiffre d'affaires, lavages, espèces
  attendues, alertes) et les raccourcis vers les gestes courants.
- **Rapports** : période au choix, détail par prestation et par méthode de
  paiement, export CSV pour le comptable.
- **Commissions** : ce qui est dû à chaque laveur sur la période (mode et montant
  se règlent à l'écran Réglages), export CSV pour la paie.

## Questions courantes

**Deux personnes peuvent-elles encaisser en même temps ?**
Oui. Chaque paiement est rattaché à son auteur et à la caisse ouverte.

**Peut-on supprimer un lavage ?**
On l'annule avec un motif. Rien ne disparaît : c'est ce qui permet de reconstituer
une journée en cas de contestation.

**Un employé peut-il voir les recettes ?**
Non. Chaque rôle ne voit que ses écrans : le laveur voit ses tâches, le caissier
sa station, le propriétaire toute l'entreprise.

**L'application fonctionne-t-elle sans Internet ?**
Sur le réseau du bureau, oui : tout passe par l'ordinateur de la station. Une
connexion n'est nécessaire que pour les sauvegardes externes ou si la base est
hébergée en ligne.
