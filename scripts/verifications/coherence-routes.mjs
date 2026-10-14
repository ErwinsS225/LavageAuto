#!/usr/bin/env node
/**
 * Cohérence des routes et des écrans.
 *
 * Le contrôle répond à trois questions, sur le code source (aucun serveur
 * nécessaire) :
 *
 * 1. chaque lien du menu ou d'une page mène-t-il à une route déclarée ?
 *    (un menu qui pointe vers une adresse inexistante envoie sur la page
 *    « introuvable » : c'est arrivé avec le menu de la plateforme, qui visait
 *    /admin/* alors que les routes sont /administration/*) ;
 * 2. chaque route déclarée est-elle atteignable par un lien ou une
 *    redirection ? (une page que rien ne peut ouvrir est une page morte) ;
 * 3. chaque permission demandée par un écran existe-t-elle dans la matrice
 *    des rôles du serveur ? (une permission mal orthographiée rendrait
 *    l'écran inaccessible à tout le monde, y compris au support).
 *
 * Usage : node scripts/verifications/coherence-routes.mjs
 * Code de sortie non nul si une incohérence est trouvée.
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join } from 'node:path'

const SOURCE = 'src'
const APP = 'src/App.tsx'
const MATRICE = 'backend/auth.go'

const problemes = []
const avertissements = []

// --------------------------------------------------------------------------
// Lecture des sources
// --------------------------------------------------------------------------

function fichiers(repertoire) {
  return readdirSync(repertoire).flatMap((nom) => {
    const chemin = join(repertoire, nom)
    if (statSync(chemin).isDirectory()) return fichiers(chemin)
    return /\.(ts|tsx)$/.test(nom) ? [chemin] : []
  })
}

const app = readFileSync(APP, 'utf8')
const sources = fichiers(SOURCE).map((chemin) => ({ chemin, texte: readFileSync(chemin, 'utf8') }))

// --------------------------------------------------------------------------
// 1. Routes déclarées
// --------------------------------------------------------------------------

const routes = [...app.matchAll(/path="([^"]+)"/g)].map((m) => m[1])
if (routes.length === 0) {
  console.error('aucune route trouvée dans App.tsx : le format a changé')
  process.exit(2)
}

function routeExiste(cible) {
  const propre = cible.split('?')[0].split('#')[0].replace(/\/+$/, '') || '/'
  return routes.some((route) => {
    // La route « * » (page introuvable) ne valide jamais une adresse : sinon
    // n'importe quel lien cassé passerait pour cohérent.
    if (route === '*') return false
    if (route === propre) return true
    if (!route.includes(':')) return false
    const motif = new RegExp('^' + route.replace(/\*/g, '.*').replace(/:[^/]+/g, '[^/]+') + '$')
    return motif.test(propre)
  })
}

// --------------------------------------------------------------------------
// 2. Cibles de navigation
// --------------------------------------------------------------------------

const cibles = new Map() // cible -> fichiers concernés

function ajouter(cible, fichier) {
  if (!cible.startsWith('/')) return // adresse externe ou ancre
  const liste = cibles.get(cible) ?? new Set()
  liste.add(fichier)
  cibles.set(cible, liste)
}

for (const { chemin, texte } of sources) {
  for (const m of texte.matchAll(/to="([^"{}]+)"/g)) ajouter(m[1], chemin)
  for (const m of texte.matchAll(/\bto:\s*'([^']+)'/g)) ajouter(m[1], chemin)
  for (const m of texte.matchAll(/\bnavigate\(\s*'([^']+)'/g)) ajouter(m[1], chemin)
  // Les redirections calculées alimentent aussi une cible statique :
  //   <Navigate to={role === 'washer' ? '/espace-laveur' : '/tableau-de-bord'} />
  for (const m of texte.matchAll(/to=\{([^}]*)\}/g)) {
    for (const c of m[1].matchAll(/'(\/[^']*)'/g)) ajouter(c[1], chemin)
  }
}

for (const [cible, fichiers] of cibles) {
  if (!routeExiste(cible)) {
    problemes.push(`lien sans route : ${cible} (${[...fichiers].join(', ')})`)
  }
}

// --------------------------------------------------------------------------
// 3. Routes atteignables
// --------------------------------------------------------------------------

const speciales = new Set(['/', '*'])
for (const route of routes) {
  if (speciales.has(route)) continue
  if (!cibles.has(route)) {
    avertissements.push(`route déclarée sans lien : ${route}`)
  }
}

// --------------------------------------------------------------------------
// 4. Permissions des écrans
// --------------------------------------------------------------------------

const matrice = readFileSync(MATRICE, 'utf8')
const bloc = matrice.slice(matrice.indexOf('rolePermissions'))
const permissionsServeur = new Set([...bloc.matchAll(/"([a-z]+:[a-z_]+)"/g)].map((m) => m[1]))
const permissionsEcrans = new Set(
  [...app.matchAll(/permission="([^"]+)"/g)].map((m) => m[1]),
)

for (const permission of permissionsEcrans) {
  if (!permissionsServeur.has(permission)) {
    problemes.push(`permission demandée par un écran mais absente de rolePermissions : ${permission}`)
  }
}

// --------------------------------------------------------------------------
// Résultat
// --------------------------------------------------------------------------

console.log(`routes déclarées : ${routes.length}`)
console.log(`cibles de navigation : ${cibles.size}`)
console.log(`permissions contrôlées : ${permissionsEcrans.size}`)

for (const a of avertissements) console.log(`note  ${a}`)
for (const p of problemes) console.error(`KO    ${p}`)

if (problemes.length > 0) {
  console.error(`\n${problemes.length} incohérence(s) de routes`)
  process.exit(1)
}
console.log('\nRoutes et écrans cohérents.')
