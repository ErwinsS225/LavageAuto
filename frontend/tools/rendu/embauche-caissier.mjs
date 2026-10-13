/**
 * Embauche d'un caissier — parcours réel à deux navigateurs.
 *
 * C'est le maillon sans lequel la station ne peut pas démarrer : si le
 * propriétaire n'arrive pas à donner un accès à son caissier, l'application ne
 * sert à rien. Rien ne le vérifiait de bout en bout.
 *
 * Deux navigateurs séparés (donc deux sessions distinctes), pour rejouer la
 * réalité : le propriétaire travaille sur l'ordinateur du bureau, le caissier
 * arrive sur son téléphone.
 *
 *   0. une nouvelle station s'inscrit (formule Essai) ;
 *   1. le propriétaire crée un employé (rôle Caissier) ;
 *   2. il relève le mot de passe provisoire affiché une seule fois ;
 *   3. le caissier se connecte avec ce mot de passe ;
 *   4. il est détourné vers « Mon compte » et ne peut PAS aller ailleurs ;
 *   5. il choisit son mot de passe définitif ;
 *   6. il navigue librement et enregistre son premier lavage ;
 *   7. le propriétaire voit ce lavage dans sa liste et l'activité du caissier ;
 *   8. le propriétaire embauche jusqu'à la limite de la formule Essai (5 comptes) :
 *      la sixième création doit être refusée avec un message qui indique la
 *      sortie (« écran Abonnement »), pas seulement l'erreur.
 *
 * Usage :
 *   CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
 *     node tools/rendu/embauche-caissier.mjs
 * Variables : BASE_URL, OUT_DIR, CHROMIUM_PATH, CHROMIUM_LIBS
 *
 * Le script crée sa propre entreprise : il ne touche pas aux données existantes
 * et peut être relancé autant de fois que voulu (e-mails horodatés).
 */
import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const baseUrl = process.env.BASE_URL ?? 'http://127.0.0.1:8080'
const outDir = process.env.OUT_DIR ?? './embauche'
fs.mkdirSync(outDir, { recursive: true })
if (process.env.CHROMIUM_LIBS) {
  process.env.LD_LIBRARY_PATH = `${process.env.CHROMIUM_LIBS}:${process.env.LD_LIBRARY_PATH ?? ''}`
}

// Identifiants uniques : le script doit pouvoir être relancé sans collision d'e-mail.
const suffixe = Date.now().toString().slice(-6)
const proprietaire = {
  email: `patronne.${suffixe}@washguard.ci`,
  password: `patronne-${suffixe}`,
}
let station = null
const caissier = {
  name: `Yao Koffi ${suffixe}`,
  email: `caissier.${suffixe}@washguard.ci`,
  password: `koffi-${suffixe}`,
}

const puppeteer = (await import('puppeteer-core')).default
let executablePath = process.env.CHROMIUM_PATH
let extraArgs = []
if (!executablePath) {
  const chromium = (await import('@sparticuz/chromium')).default
  executablePath = await chromium.executablePath()
  // Le Chromium du paquet npm démarre en processus unique : Puppeteer ne peut
  // alors pas ouvrir deux sessions isolées (le poste du propriétaire et le
  // téléphone du caissier). On retire ces deux options, qui ne concernent que
  // le navigateur de test, jamais l'application.
  extraArgs = chromium.args.filter(
    (argument) => argument !== '--single-process' && argument !== '--in-process-gpu',
  )
}
const browser = await puppeteer.launch({
  executablePath,
  headless: true,
  args: [...extraArgs, '--no-sandbox', '--disable-dev-shm-usage'],
  // CHROMIUM_LIBS est reporté dans LD_LIBRARY_PATH pour le navigateur lui-même :
  // sans cela, un Chromium fourni avec ses propres bibliothèques ne démarre pas.
  env: { ...process.env, LD_LIBRARY_PATH: process.env.LD_LIBRARY_PATH ?? '' },
})

const problems = []
const steps = []
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
const ok = (label, detail = '') => steps.push(`  ✅ ${label}${detail ? ` · ${detail}` : ''}`)
const ko = (label, detail = '') => {
  steps.push(`  ❌ ${label}${detail ? ` · ${detail}` : ''}`)
  problems.push(`${label}${detail ? ` (${detail})` : ''}`)
}

/** Prépare une page téléphone avec un contexte isolé (cookies séparés). */
async function nouvellePageTelephone(label) {
  const context = await browser.createBrowserContext()
  const page = await context.newPage()
  await page.setViewport({ width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true })
  page.on('pageerror', (error) => problems.push(`${label} · erreur JS : ${String(error).slice(0, 200)}`))
  return page
}

const bodyText = (page) => page.evaluate(() => document.body.innerText)

async function fill(page, labelText, value) {
  await page.evaluate(
    (labelText, value) => {
      const dialog = document.querySelector('[role="dialog"]')
      const root = dialog ?? document
      const label = [...root.querySelectorAll('label')].find((item) => item.textContent.trim().startsWith(labelText))
      if (!label) throw new Error(`libellé introuvable : ${labelText}`)
      const input = label.htmlFor ? document.getElementById(label.htmlFor) : label.parentElement.querySelector('input, textarea')
      Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set.call(input, value)
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.dispatchEvent(new Event('change', { bubbles: true }))
    },
    labelText,
    value,
  )
}

async function choose(page, labelText, value) {
  await page.evaluate(
    (labelText, value) => {
      const dialog = document.querySelector('[role="dialog"]')
      const root = dialog ?? document
      const label = [...root.querySelectorAll('label')].find((item) => item.textContent.trim().startsWith(labelText))
      if (!label) throw new Error(`libellé introuvable : ${labelText}`)
      const select = label.htmlFor ? document.getElementById(label.htmlFor) : label.parentElement.querySelector('select')
      Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value').set.call(select, value)
      select.dispatchEvent(new Event('input', { bubbles: true }))
      select.dispatchEvent(new Event('change', { bubbles: true }))
    },
    labelText,
    value,
  )
}

async function tap(page, text, { exact = false, scope = 'auto' } = {}) {
  const clicked = await page.evaluate(
    (text, exact, scope) => {
      const dialog = document.querySelector('[role="dialog"]')
      const root = scope === 'dialog' || (scope === 'auto' && dialog) ? dialog : document
      if (!root) return false
      const found = [...root.querySelectorAll('button, a')].find((item) => {
        const content = item.textContent.replace(/\s+/g, ' ').trim()
        return exact ? content === text : content.startsWith(text)
      })
      if (!found) return false
      found.click()
      return true
    },
    text,
    exact,
    scope,
  )
  if (!clicked) throw new Error(`bouton introuvable : ${text}`)
}

async function connecter(page, email, password) {
  await page.goto(`${baseUrl}/connexion`, { waitUntil: 'domcontentloaded' })
  await page.waitForSelector('input[type="email"]', { timeout: 20_000 })
  await page.type('input[type="email"]', email)
  await page.type('input[type="password"]', password)
  await Promise.all([
    page.waitForFunction(() => !location.pathname.includes('connexion'), { timeout: 30_000 }),
    page.click('button[type="submit"]'),
  ])
  await sleep(2000)
}

async function shot(page, name) {
  const file = path.join(outDir, `${name}.png`)
  await page.screenshot({ path: file })
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  if (overflow > 2) ko(`débordement horizontal — ${name}`, `${overflow} px`)
  return file
}

const owner = await nouvellePageTelephone('propriétaire')
const salarie = await nouvellePageTelephone('caissier')

/** Affiche le bilan, y compris en cas d'échec brutal (le détail aide à corriger). */
function rapport() {
  console.log(`\nEmbauche d'un caissier (téléphone 390 × 844) — ${baseUrl}\n`)
  console.log(steps.join('\n'))
  console.log(`\nStation créée : ${station?.nom ?? '—'} (propriétaire ${proprietaire.email})`)
  console.log(`Caissier embauché : ${caissier.email}`)
  if (problems.length > 0) {
    console.log(`\n${problems.length} problème(s) :`)
    for (const problem of [...new Set(problems)]) console.log(`  • ${problem}`)
  } else {
    console.log(`\nParcours complet : le propriétaire ouvre l’accès, le caissier prend la main.`)
  }
  console.log(`Captures : ${path.resolve(outDir)}`)
}

try {
// --- 0. Une station s'inscrit (elle démarre en formule Essai) ----------------
station = { nom: `Station Embauche ${suffixe}`, ville: 'Abidjan' }
await owner.goto(`${baseUrl}/inscription`, { waitUntil: 'domcontentloaded' })
await owner.waitForSelector('input[type="email"]', { timeout: 20_000 })
await fill(owner, 'Nom de l’entreprise', station.nom)
await fill(owner, 'Nom de la station', `${station.nom} — Cocody`)
await fill(owner, 'Votre nom complet', 'Awa Traoré')
await fill(owner, 'Adresse e-mail', proprietaire.email)
await fill(owner, 'Mot de passe', proprietaire.password)
await fill(owner, 'Ville', station.ville)
await Promise.all([
  owner.waitForFunction(() => !location.pathname.includes('inscription'), { timeout: 30_000 }),
  owner.click('button[type="submit"]'),
])
await sleep(2500)
if (/Tableau de bord/i.test(await bodyText(owner))) ok('nouvelle station inscrite, propriétaire connecté', station.nom)
else ko('inscription de la station impossible', (await bodyText(owner)).slice(0, 140).replace(/\n/g, ' '))

// La formule reçue doit être l'essai, sinon la suite du scénario n'a pas de sens.
const abonnement = await owner.evaluate(async () => {
  const response = await fetch('/api/v1/organizations/subscription', { credentials: 'same-origin' })
  if (!response.ok) return { error: response.status }
  const payload = await response.json()
  // Forme réelle : { payment_note, plans: [...], subscription: {...} }.
  const courant = payload.subscription ?? {}
  const plan = (payload.plans ?? []).find((item) => item.code === courant.plan_code) ?? {}
  return { plan: courant.plan_code, status: courant.status, maxUsers: plan.max_employees, maxOrders: plan.max_wash_orders_month }
})
if (abonnement.plan === 'essai' && abonnement.status === 'trialing' && abonnement.maxUsers >= 5) {
  ok('formule Essai reçue, équipe possible', `${abonnement.plan} (${abonnement.status}) · ${abonnement.maxUsers} comptes · ${abonnement.maxOrders} lavages/mois`)
} else {
  ko('formule d’essai inattendue — l’essai doit laisser entrer une petite équipe', JSON.stringify(abonnement))
}

await owner.goto(`${baseUrl}/employes`, { waitUntil: 'domcontentloaded' })
await sleep(1800)
await tap(owner, 'Ajouter un employé')
await owner.waitForSelector('[role="dialog"]', { timeout: 15_000 })
await sleep(400)
await fill(owner, 'Nom complet', caissier.name)
await fill(owner, 'Adresse e-mail', caissier.email)
await choose(owner, 'Rôle', 'cashier')
await sleep(300)
await shot(owner, '01-creation-employe')
await tap(owner, 'Enregistrer', { scope: 'dialog' })
await owner.waitForFunction(
  () => /Mot de passe provisoire/i.test(document.body.innerText),
  { timeout: 20_000 },
).catch(() => {})
await sleep(1200)

// --- 2. Le mot de passe provisoire est-il réellement affiché ? ---------------
const provisoire = await owner.evaluate(() => {
  // Le mot de passe est le seul nœud en police à chasse fixe du dialogue :
  // le lire ainsi évite d'attraper le texte d'à côté (« il ne sera plus affiché »).
  const dialog = document.querySelector('[role="dialog"]')
  const node = dialog?.querySelector('.font-mono')
  const value = node?.textContent?.trim()
  return value && value.length >= 8 ? value : null
})
if (provisoire && provisoire.length >= 8) ok('mot de passe provisoire affiché au propriétaire', `${provisoire.slice(0, 3)}… (masqué ici)`)
else ko('mot de passe provisoire non affiché — le propriétaire ne peut pas transmettre l’accès', (await bodyText(owner)).slice(0, 120).replace(/\n/g, ' '))
await shot(owner, '02-mot-de-passe-provisoire')
// Libellé réel du bouton (apostrophe typographique incluse).
await tap(owner, 'J’ai noté', { scope: 'dialog' })
await sleep(800)

// --- 3. Le caissier se connecte ---------------------------------------------
if (provisoire) {
  await connecter(salarie, caissier.email, provisoire)
  const apresConnexion = await bodyText(salarie)
  const url = salarie.url()
  if (url.includes('/mon-compte')) ok('caissier détourné vers Mon compte', url.replace(baseUrl, ''))
  else ko('le caissier n’est pas envoyé vers Mon compte', url.replace(baseUrl, ''))
  if (/mot de passe est provisoire/i.test(apresConnexion)) ok('bannière « mot de passe provisoire » affichée')
  else ko('bannière d’avertissement absente')
  await shot(salarie, '03-premiere-connexion')

  // --- 4. Peut-il aller travailler sans changer de mot de passe ? -----------
  await salarie.goto(`${baseUrl}/lavages`, { waitUntil: 'domcontentloaded' })
  await sleep(1800)
  const bloque = salarie.url().includes('/mon-compte')
  if (bloque) ok('accès aux autres écrans bloqué tant que le mot de passe est provisoire')
  else ko('le caissier a pu ouvrir /lavages avec un mot de passe provisoire', salarie.url().replace(baseUrl, ''))

  // --- 5. Il choisit son mot de passe définitif ------------------------------
  await fill(salarie, 'Mot de passe actuel', provisoire)
  await fill(salarie, 'Nouveau mot de passe', caissier.password)
  await fill(salarie, 'Confirmer le nouveau mot de passe', caissier.password)
  await sleep(300)
  await shot(salarie, '04-changement-mot-de-passe')
  await tap(salarie, 'Changer', { scope: 'dialog' }).catch(() => salarie.click('button[type="submit"]'))
  await sleep(2500)
  const apresChangement = await bodyText(salarie)
  // La bannière « Votre mot de passe est provisoire » doit disparaître sans
  // rechargement de la page : sinon l'employé reste renvoyé sur cet écran.
  if (/Votre mot de passe est provisoire/i.test(apresChangement)) {
    ko('la bannière provisoire reste affichée après le changement', 'rafraîchissement de session manquant')
  } else {
    ok('mot de passe définitif accepté, bannière disparue')
  }
  // Et il doit pouvoir naviguer sans rechargement complet (navigation interne).
  await tap(salarie, 'Lavages').catch(() => {})
  await sleep(1800)
  if (salarie.url().includes('/lavages')) ok('navigation interne rétablie sans rechargement')
  else ko('l’employé reste renvoyé sur Mon compte en navigation interne', salarie.url().replace(baseUrl, ''))
}

// --- 6. Le caissier travaille -------------------------------------------------
await salarie.goto(`${baseUrl}/lavages`, { waitUntil: 'domcontentloaded' })
await sleep(1800)
if (salarie.url().includes('/mon-compte')) {
  ko('le caissier reste bloqué sur Mon compte après changement', salarie.url().replace(baseUrl, ''))
} else {
  ok('accès aux lavages rétabli')
  await tap(salarie, 'Nouveau lavage')
  await salarie.waitForSelector('[aria-label="Prestation"]', { timeout: 15_000 })
  await sleep(400)
  const service = await salarie.evaluate(() =>
    [...document.querySelector('[aria-label="Prestation"]').options].map((option) => option.value).find(Boolean),
  )
  await salarie.select('[aria-label="Prestation"]', service)
  await sleep(300)
  await tap(salarie, 'Enregistrer le lavage')
  await salarie.waitForFunction(() => !document.body.innerText.includes('Enregistrer le lavage'), { timeout: 20_000 })
  await sleep(1800)
  const reference = ((await bodyText(salarie)).match(/L-\d{8}-\d+/) ?? [])[0]
  if (reference) ok('premier lavage enregistré par le nouveau caissier', reference)
  else ko('le nouveau caissier n’a pas pu enregistrer de lavage', (await bodyText(salarie)).slice(0, 140).replace(/\n/g, ' '))
  await shot(salarie, '05-caissier-au-travail')

  // --- 7. Le propriétaire voit-il cette activité ? --------------------------
  await owner.goto(`${baseUrl}/lavages`, { waitUntil: 'domcontentloaded' })
  await sleep(2200)
  const cote = await bodyText(owner)
  if (reference && cote.includes(reference)) ok('le lavage apparaît aussi chez le propriétaire', reference)
  else ko('le lavage du caissier est invisible chez le propriétaire', reference ?? '')
  await shot(owner, '06-vue-proprietaire')

  await owner.goto(`${baseUrl}/employes`, { waitUntil: 'domcontentloaded' })
  await sleep(2000)
  const equipe = await bodyText(owner)
  if (equipe.includes(caissier.name)) ok('le caissier apparaît dans l’équipe')
  else ko('le caissier est absent de la liste de l’équipe')

  // --- 8. La limite de la formule est-elle appliquée côté serveur ? ---------
  // L'essai autorise 5 comptes : le propriétaire en a 1, il peut donc en créer
  // 4. On crée jusqu'à la limite, puis on tente le compte de trop.
  let refuse = null
  let crees = 0
  for (let i = 1; i <= 5; i += 1) {
    await owner.goto(`${baseUrl}/employes`, { waitUntil: 'domcontentloaded' })
    await sleep(1500)
    await tap(owner, 'Ajouter un employé')
    await owner.waitForSelector('[role="dialog"]', { timeout: 15_000 })
    await sleep(400)
    const employe = { name: `Laveur ${i} ${suffixe}`, email: `laveur${i}.${suffixe}@washguard.ci` }
    await fill(owner, 'Nom complet', employe.name)
    await fill(owner, 'Adresse e-mail', employe.email)
    await tap(owner, 'Enregistrer', { scope: 'dialog' })
    await sleep(2200)
    const texte = await bodyText(owner)
    if (/Limite d'utilisateurs atteinte|Limite d’utilisateurs atteinte/i.test(texte)) {
      refuse = { tentative: i, message: texte.match(/[^\n]*Limite d[^\n]*/i)?.[0] ?? '' }
      await shot(owner, '07-limite-formule')
      break
    }
    if (/Mot de passe provisoire/i.test(texte)) {
      crees += 1
      await tap(owner, 'J’ai noté', { scope: 'dialog' })
      await sleep(700)
    }
  }
  if (refuse) {
    ok('limite de la formule appliquée par le serveur', `${crees} laveur(s) embauché(s), la création suivante est refusée`)
    if (/Abonnement/i.test(refuse.message)) ok('le refus indique la sortie (écran Abonnement)')
    else ko('le refus n’indique pas quoi faire', refuse.message.slice(0, 140))
  } else {
    ko('la limite de la formule n’a pas été appliquée', `${crees} laveur(s) créés sans refus (essai = 5 comptes au total)`)
  }
}

} catch (erreur) {
  ko('échec du parcours', String(erreur).split('\n')[0].slice(0, 200))
}

await browser.close()
rapport()
process.exit(problems.length > 0 ? 1 : 0)
