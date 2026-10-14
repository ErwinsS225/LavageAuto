# Interface WashGuard — repères de conception

Ce document décrit le langage visuel de l'application : tokens, ombres, flous,
durées, classes sémantiques et briques animées. Il sert à toute personne qui
touche un écran : en le suivant, une nouvelle page ressemble aux autres sans
avoir à recopier le style d'une page existante.

## 1. Principe

Trois idées gouvernent l'interface :

1. **La hiérarchie se lit avant de se voir.** Un écran de station se consulte
   debout, souvent sur un téléphone, parfois en plein soleil : les chiffres
   importants sont gros, tabulaires et placés en haut ; le reste est discret.
2. **Rien n'est plat.** Chaque surface a un contour d'un pixel, une ombre de
   contact, un reflet intérieur en haut et une ombre portée en dessous. C'est
   l'empilement de ces détails qui donne la sensation de relief, pas une ombre
   unique très marquée.
3. **Le mouvement est court et doux.** Une transition d'interface se sent plus
   qu'elle ne se voit : 140 ms pour un survol, 220 ms pour une apparition,
   420 ms pour un changement d'écran, 700 ms pour un graphique. Jamais de
   rebond marqué, jamais de rotation, jamais de zoom brutal.

## 2. Tokens (`src/index.css`)

| Famille | Tokens | Usage |
| --- | --- | --- |
| Marque | `--brand`, `--brand-strong`, `--brand-soft`, `--brand-gradient` | boutons, liens, états actifs |
| Textes | `--ink`, `--ink-muted`, `--ink-subtle` | titres, libellés, aides |
| Traits | `--line`, `--line-strong`, `--line-soft` | contours, séparateurs |
| Surfaces | `--surface`, `--surface-muted`, `--surface-hover`, `--surface-inset`, `--surface-glass` | cartes, fonds creusés, verre |
| États | `--danger`, `--warning`, `--success`, `--info` (+ variantes `-soft`, `-strong`) | alertes, badges |
| Relief | `--e1`, `--e2`, `--e3`, `--e-float`, `--e-glow`, `--e-inset` | ombres en couches |
| Mouvement | `--ease-soft`, `--ease-snap` | courbes de sortie |

Les tokens changent de valeur entre le mode clair et le mode sombre ; le JSX ne
contient **aucune couleur en dur**. Un utilitaire comme `bg-surface` ou
`text-ink-muted` suit donc le thème automatiquement.

Exposition à Tailwind (`@theme inline`) : `bg-surface`, `text-ink`,
`border-line-strong`, `shadow-e2`, `shadow-float`, `rounded-field`,
`rounded-card`, `rounded-panel`, `ease-soft`, `backdrop-blur-glass`…

## 3. Classes sémantiques

Définies dans la couche *components* de Tailwind : les utilitaires restent
prioritaires, on peut donc écrire `class="stat-value text-lg text-success"`.

| Classe | Rôle |
| --- | --- |
| `card`, `card-interactive`, `card-glass` | panneau surélevé, panneau cliquable, panneau en verre |
| `panel`, `panel-inset` | bloc plat, bloc creusé (notes, en-têtes de filtres) |
| `hairline-highlight` | filet lumineux en haut d'un panneau |
| `badge`, `badge-brand/success/warning/danger/info/neutral` | pastilles d'état |
| `chip`, `chip-active` | puce de choix qui se coche (étapes du parcours, types de lavage) |
| `nav-link`, `nav-link-active` | entrées de navigation (barre latérale, menus) |
| `stat-label`, `stat-value` | libellé et valeur d'un indicateur |
| `table` | cadre de tableau : en-tête collant, filets, survol de ligne |
| `input`, `select`, `textarea` | champs (contour, ombre intérieure, anneau au focus) |
| `chart-frame`, `chart-tooltip` | encadré et infobulle des graphiques |
| `skeleton` | bloc de chargement avec balayage |
| `app-aurora`, `grid-lines` | fonds d'ambiance (halos, toile quadrillée) |
| `btn-brand`, `btn-sheen` | bouton de marque avec reflet qui balaie |

> `make check` exécute `scripts/verifications/classes-ui.mjs` : toute classe
> maison employée dans `src/` mais non définie fait échouer la vérification.
> C'est exactement le défaut qui existait avant cette passe (`stat-value`,
> `badge-*`, `nav-link` et `table` étaient utilisés et stylés nulle part).

## 4. Mouvement (`src/lib/motion.ts`)

- Durées : `DUR.fast` 140 ms, `DUR.base` 220 ms, `DUR.slow` 360 ms,
  `DUR.page` 420 ms, `DUR.chart` 700 ms.
- Courbes : `EASE_SOFT` (rapide au départ, longue à se poser) pour les
  apparitions ; `EASE_SNAP` (matérielle) pour les disparitions et les survols.
- Variantes prêtes : `fadeIn`, `fadeUp`, `fadeUpSmall`, `scaleIn`, `item`,
  `pageVariants`, `modalVariants`, `drawerVariants`, `menuVariants`,
  `toastVariants`, `backdropVariants`, `stagger(step, delay)`.
- Raccourcis : `hoverLift`, `pressable`, `inView`.

Blocs animés exportés par `@/components/ui` :

| Bloc | Usage |
| --- | --- |
| `MotionPage` | enveloppe d'écran (transition d'entrée/sortie) |
| `MotionStagger`, `MotionItem` | apparition échelonnée d'une liste ou d'une grille |
| `Reveal` | apparition à l'entrée dans la fenêtre (pages longues) |
| `AnimatedNumber` | valeur qui compte jusqu'à sa cible |
| `ValuePulse` | valeur qui se réajuste en douceur quand la donnée change |

`<MotionConfig reducedMotion="user">` (voir `src/main.tsx`) neutralise les
animations pour qui a demandé « réduire les animations » — l'interface reste
utilisable, simplement sans mouvement. La feuille de style applique la même
règle aux animations CSS (`prefers-reduced-motion`).

## 5. Composants (`src/components/ui/`)

| Fichier | Contenu |
| --- | --- |
| `primitives.tsx` | `Button`, `IconButton`, `Badge`, `Segmented`, `toneClass` |
| `surfaces.tsx` | `Card`, `CardHeader`, `ScreenHero`, `PageHeader`, `SectionTitle`, `StatCard`, `Progress`, `DefinitionNote`, `Money` |
| `data.tsx` | `Table`, `TableFrame`, `MotionRow`, `rowDelay` |
| `forms.tsx` | `Field`, `Input`, `Textarea`, `Select`, `Switch` |
| `overlays.tsx` | `Modal`, `ConfirmDialog` |
| `feedback.tsx` | `Spinner`, `Skeleton`, `SkeletonRows`, `EmptyState`, `ErrorState`, `AnimatedNumber` |
| `motion-blocks.tsx` | `MotionPage`, `MotionStagger`, `MotionItem`, `Reveal`, `ValuePulse` |

Les écrans importent toujours depuis `@/components/ui` : le rangement interne
peut évoluer sans toucher aux pages.

## 6. Graphiques (`src/components/charts.tsx`)

- Les couleurs viennent des tokens (`var(--brand)`…) : elles suivent le thème.
- Barres et aires utilisent un dégradé vertical (`ChartGradients` + `ChartBar`,
  `ChartArea`) : c'est ce qui distingue un graphique « posé » d'un aplat.
- L'infobulle (`ChartTooltip`) et la légende (`ChartLegend`) sont des
  composants maison, en verre dépoli, montants formatés en FCFA.
- L'anneau (`ChartDonut`, `DonutCell`) a un trou large, des coins arrondis et
  une animation d'entrée de 700 ms.

### Écrans de la grille tarifaire

`/prestations` (catalogue) et `/tarifs` (grille, options, règles) se partagent
une règle de conception : **un montant ne se saisit que là où il se justifie**.

- `/prestations` n'a **aucun champ de prix**. Il compose le catalogue (nom,
  durée, contenu, véhicules concernés, gratuité, devis) et affiche, par
  prestation, ce que le serveur appliquera : `gratuite`, `sur devis`,
  `à tarifer`, le tarif du catalogue, ou « tarif par véhicule » quand seule la
  grille tranche. Un renvoi mène à `/tarifs`.
- `/tarifs` est le seul écran qui écrit un prix. La saisie se fait dans une
  case (`Input` en mode numérique), le **motif** est un champ obligatoire
  collé à l'enregistrement, et le bouton reste inactif tant qu'il est vide : on
  n'enregistre pas 40 cases pour découvrir après coup qu'il manquait le motif
  (le serveur, lui, refuse la grille entière — la défense est des deux côtés).
- Les cases vides ne mentent pas : elles affichent ce qui s'appliquera
  réellement (« hérité du catalogue », « à tarifer », « gratuite »), pas un
  zéro.
- L'écran réutilise `TarifsForm` et `OptionsForm` de la mise en route : un
  seul composant par formulaire, donc un seul comportement à corriger.
- Sur la fiche de caisse, chaque ligne affiche le prix **et sa provenance**
  (`price_source_label`) ; une ligne sans tarif est signalée avant
  l'enregistrement, jamais après.

### Écrans de la caisse

`/caisse` et l'onglet **Caisse et reçus** de `/parametres` partagent la même
règle : **la caisse n'affiche que ce que la station a déclaré**.

- Les moyens de paiement proposés à l'encaissement viennent de
  `GET /payment-methods?actives=1` : pas de liste en dur dans le navigateur. Un
  moyen à référence fait apparaître le champ *Référence* comme obligatoire, et
  un moyen différé affiche un avertissement (« la fiche reste due, aucun
  compteur de caisse ne bouge ») plutôt que de laisser croire à un encaissement.
- Le **reçu affiché vient du serveur**, tel quel : numéro, lignes, mentions.
  L'aperçu des réglages porte la mention « Exemple » et **aucun montant** —
  une maquette ne doit jamais pouvoir passer pour un justificatif.
- Une mention non renseignée est annoncée (« non renseignée ») au lieu
  d'apparaître vide : on ne fait pas imprimer une ligne sans valeur.
- Sur la fiche, la provenance de chaque prix (`price_source_label`) et le moyen
  réellement utilisé (« Orange Money CI », pas `mobile_money`) restent visibles :
  c'est ce qui permet d'expliquer une facture après coup.

## 7. Accessibilité et impressions

- Anneau de focus unique (`:focus-visible`) et visible au clavier.
- Contrastes : `--ink-muted` reste lisible sur `--surface` dans les deux thèmes ;
  les badges utilisent une teinte + un contour de la même famille.
- `prefers-reduced-motion` respecté (CSS + Framer Motion).
- À l'impression, halos et ombres sont retirés (règle `@media print`).

## 8. Vérifier une modification d'interface

```bash
npm run typecheck                            # types du front (app + outils)
node scripts/verifications/classes-ui.mjs    # classes maison définies
node scripts/verifications/rendu-pages.mjs   # 14 écrans rendus sans erreur
make check                                   # tout (Go, front, routes, écrans, config)
```

Deux remarques apprises à l'usage, qui évitent de croire à tort que tout va bien :

- `tsc --noEmit` sans `-p` ne vérifie rien : le `tsconfig.json` racine ne liste
  aucun fichier (il ne fait que référencer les deux projets). D'où
  `npm run typecheck`, qui vise explicitement `tsconfig.app.json` et
  `tsconfig.node.json`.
- le contrôle de rendu rend chaque page **deux fois** : la première passe ne
  charge que les écrans différés (`React.lazy`) et n'affiche que la coquille —
  où figurent les noms des écrans. Une seule passe laisserait donc passer un
  écran vide. Les renvois de navigation, eux, ne sont pas vérifiables hors
  navigateur (ils s'exécutent dans un effet React).
