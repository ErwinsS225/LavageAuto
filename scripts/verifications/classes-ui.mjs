#!/usr/bin/env node
/**
 * Classes d'interface — vérifie qu'aucune classe maison n'est orpheline.
 *
 * Historique : `stat-value`, `stat-label`, `nav-link`, `badge-*` et `table`
 * étaient utilisés dans les écrans… et définis nulle part. Les composants
 * paraissaient corrects dans le code (la classe était bien écrite) alors que
 * rien ne les stylait, et aucune vérification ne pouvait le voir : Tailwind ne
 * se plaint pas d'une classe inconnue, il ne génère simplement rien.
 *
 * Ce contrôle relie les deux moitiés :
 *   1. il relève dans `src/**` les classes sémantiques employées (préfixes
 *      maison : card, badge, nav-link, stat-, table, input, panel, chart-…) ;
 *   2. il relève dans `src/index.css` les classes réellement définies ;
 *   3. toute classe employée et non définie est signalée.
 *
 * Usage : node scripts/verifications/classes-ui.mjs
 * Code de sortie non nul si une classe est orpheline (ou si une classe du
 * design system n'est plus utilisée nulle part : c'est un reste à nettoyer).
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, extname } from 'node:path'

const RACINE = new URL('../..', import.meta.url).pathname.replace(/\/$/, '')
const SRC = join(RACINE, 'src')
const CSS = join(SRC, 'index.css')

/** Préfixes des classes maison (le reste est du Tailwind). */
const PREFIXES = [
  'card',
  'badge',
  'chip',
  'nav-link',
  'stat-',
  'table',
  'panel',
  'input',
  'select',
  'textarea',
  'chart-',
  'skeleton',
  'grid-lines',
  'app-aurora',
  'hairline',
  'divider-soft',
  'link-soft',
  'btn-',
  'error-text',
  'dot',
]

/** Classes de Tailwind qui commencent par le même mot qu'un préfixe maison. */
const COLLISIONS_TAILWIND = new Set([
  'table',
  'table-fixed',
  'table-auto',
  'select-none',
  'select-text',
  'select-all',
  'select-auto',
  'input-group',
])

function fichiers(dossier) {
  const resultat = []
  for (const entree of readdirSync(dossier)) {
    const chemin = join(dossier, entree)
    if (statSync(chemin).isDirectory()) resultat.push(...fichiers(chemin))
    else if (['.ts', '.tsx'].includes(extname(chemin))) resultat.push(chemin)
  }
  return resultat
}

/** Classes définies dans la feuille : `.nom {`, `.nom,`, `.nom:`, `.nom `. */
function classesDefinies(css) {
  const definies = new Set()
  const motif = /\.([a-zA-Z][a-zA-Z0-9_-]*)/g
  let trouve
  while ((trouve = motif.exec(css)) !== null) definies.add(trouve[1])
  return definies
}

/** Classes employées : tout littéral de texte contenant un préfixe maison. */
function classesEmployees(fichiersTs) {
  const employees = new Map()
  for (const fichier of fichiersTs) {
    const contenu = readFileSync(fichier, 'utf8')
    for (const prefixe of PREFIXES) {
      // Un mot commençant par le préfixe, poursuivi par des tirets/minuscules.
      const motif = new RegExp(`(?<![\\w-])${prefixe.replace('-', '\\-')}[a-z0-9-]*`, 'g')
      let trouve
      while ((trouve = motif.exec(contenu)) !== null) {
        const classe = trouve[0]
        // Écarte les faux positifs : fragment d'adresse (/tableau-de-bord),
        // classe construite dynamiquement (badge-${ton}), variante Tailwind.
        const avant = contenu.slice(Math.max(0, trouve.index - 1), trouve.index)
        const apres = contenu.slice(trouve.index + classe.length, trouve.index + classe.length + 2)
        if (avant === '/' || avant === '#' || apres.startsWith('${') || apres === '-$') continue
        if (COLLISIONS_TAILWIND.has(classe)) continue
        if (!employees.has(classe)) employees.set(classe, fichier.replace(RACINE + '/', ''))
      }
    }
  }
  return employees
}

/**
 * Second contrôle : les utilitaires d'espacement en quart de pas.
 * Tailwind ne génère que 0,5 · 1,5 · 2,5 · 3,5 (puis la grille entière). Une
 * valeur comme `h-9.5` ne produit rien du tout — la classe est écrite, rien
 * n'est stylé, et le composant se retrouve avec une hauteur automatique.
 */
const ESPACEMENTS_INVALIDES =
  /(?<![\w-])(?:h|w|p|px|py|pt|pb|pl|pr|m|mx|my|mt|mb|ml|mr|gap|space-x|space-y|size)-(?:[4-9]|[1-9][0-9]+)\.5(?![\w.])/g

const espacementsInvalides = []
for (const fichier of fichiers(SRC)) {
  const contenu = readFileSync(fichier, 'utf8')
  let trouve
  while ((trouve = ESPACEMENTS_INVALIDES.exec(contenu)) !== null) {
    espacementsInvalides.push(`${trouve[0]}  (${fichier.replace(RACINE + '/', '')})`)
  }
}

if (espacementsInvalides.length > 0) {
  console.log(`✗ ${espacementsInvalides.length} utilitaire(s) d'espacement que Tailwind ne génère pas :`)
  for (const ligne of new Set(espacementsInvalides)) console.log(`    ${ligne}`)
  process.exit(1)
}

const definies = classesDefinies(readFileSync(CSS, 'utf8'))
const employees = classesEmployees(fichiers(SRC))

// Bruit toléré : mots français qui commencent par un préfixe (jamais des classes).
const MOTS = new Set(['tableau', 'tableaux', 'tableur', 'tables', 'tablette', 'cartable'])

const orphelines = [...employees.entries()].filter(
  ([classe]) => !definies.has(classe) && !MOTS.has(classe),
)

if (orphelines.length > 0) {
  console.log(`✗ ${orphelines.length} classe(s) employée(s) mais non définie(s) :`)
  for (const [classe, fichier] of orphelines) console.log(`    ${classe}  (${fichier})`)
  process.exit(1)
}

console.log(`Classes d'interface cohérentes (${employees.size} classe(s) maison définies).`)
