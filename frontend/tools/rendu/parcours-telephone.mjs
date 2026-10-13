/**
 * Parcours réel du caissier sur un téléphone.
 *
 * Le scénario d'installation retenu est : l'application tourne sur l'ordinateur
 * du bureau, les employés s'y connectent depuis leur téléphone en Wi-Fi. Les
 * captures d'écran en 1440 px ne prouvent rien pour ce cas. Ce script rejoue le
 * geste quotidien sur un écran de 390 × 844, avec le compte Caissier :
 *
 *   1. connexion, tableau de bord
 *   2. menu : le caissier ne doit pas voir les écrans réservés
 *   3. enregistrement d'un lavage (le geste le plus fréquent de la journée)
 *   4. encaissement en espèces (doit être refusé si aucune caisse n'est ouverte)
 *   5. encaissement en Mobile Money (doit aboutir, sans caisse ouverte)
 *
 * Utilisation (macOS) :
 *   CHROMIUM_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
 *     node tools/rendu/parcours-telephone.mjs
 *
 * Le projet doit tourner et contenir le catalogue de prestations. Sur macOS, le
 * navigateur est celui de la machine ; dans un conteneur Linux sans Chrome, on
 * retombe sur le Chromium de @sparticuz/chromium (voir captures.mjs).
 * Variables : BASE_URL, OUT_DIR, EMAIL, PASSWORD, CHROMIUM_LIBS, CHROMIUM_PATH
 */
import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const baseUrl = process.env.BASE_URL ?? 'http://127.0.0.1:8080'
const outDir = process.env.OUT_DIR ?? 'apercu-telephone'
const email = process.env.EMAIL ?? 'caisse@washguard.ci'
const password = process.env.PASSWORD ?? 'demo1234'
fs.mkdirSync(outDir, { recursive: true })

if (process.env.CHROMIUM_LIBS) {
  process.env.LD_LIBRARY_PATH = `${process.env.CHROMIUM_LIBS}:${process.env.LD_LIBRARY_PATH ?? ''}`
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

const page = await browser.newPage()
await page.setViewport({ width: 390, height: 844, deviceScaleFactor: 2, isMobile: true, hasTouch: true })

/** Étape attendue : les erreurs 401/422 font partie du scénario, pas des pannes. */
let expectedFailures = []
page.on('pageerror', (error) => problems.push(`erreur JS : ${String(error).slice(0, 200)}`))
page.on('console', (message) => {
  if (message.type() !== 'error') return
  const text = message.text()
  if (text.includes('401') || expectedFailures.some((code) => text.includes(String(code)))) return
  problems.push(`console : ${text.slice(0, 200)}`)
})
page.on('response', (response) => {
  const status = response.status()
  if (status < 400) return
  if (status === 401) return
  if (expectedFailures.includes(status)) return
  problems.push(`HTTP ${status} sur ${response.url().replace(baseUrl, '')}`)
})

/** Remplit un champ repéré par son libellé (React exige l'événement natif). */
async function fill(labelText, value) {
  await page.evaluate(
    (labelText, value) => {
      const label = [...document.querySelectorAll('label')].find((item) => item.textContent.trim().startsWith(labelText))
      if (!label) throw new Error(`libellé introuvable : ${labelText}`)
      const field = label.htmlFor ? document.getElementById(label.htmlFor) : null
      const input = field ?? label.parentElement.querySelector('input, select, textarea')
      const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set
      setter.call(input, value)
      input.dispatchEvent(new Event('input', { bubbles: true }))
      input.dispatchEvent(new Event('change', { bubbles: true }))
    },
    labelText,
    value,
  )
}

async function choose(labelText, value) {
  await page.evaluate(
    (labelText, value) => {
      const label = [...document.querySelectorAll('label')].find((item) => item.textContent.trim().startsWith(labelText))
      if (!label) throw new Error(`libellé introuvable : ${labelText}`)
      const select = label.htmlFor ? document.getElementById(label.htmlFor) : label.parentElement.querySelector('select')
      const setter = Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value').set
      setter.call(select, value)
      select.dispatchEvent(new Event('input', { bubbles: true }))
      select.dispatchEvent(new Event('change', { bubbles: true }))
    },
    labelText,
    value,
  )
}

/** Clique le premier bouton dont le texte commence par `text`. */
async function tap(text, { exact = false } = {}) {
  const clicked = await page.evaluate(
    (text, exact) => {
      const buttons = [...document.querySelectorAll('button, a')]
      const found = buttons.find((item) => {
        const content = item.textContent.replace(/\s+/g, ' ').trim()
        return exact ? content === text : content.startsWith(text)
      })
      if (!found) return false
      found.click()
      return true
    },
    text,
    exact,
  )
  if (!clicked) throw new Error(`bouton introuvable : ${text}`)
}

async function shot(name, label) {
  const file = path.join(outDir, `${name}.png`)
  await page.screenshot({ path: file })
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  const visible = ((await page.evaluate(() => document.body.innerText)) ?? '').trim().length
  if (overflow > 2) ko(`débordement horizontal — ${label}`, `${overflow} px`)
  if (visible < 100) ko(`écran presque vide — ${label}`, `${visible} caractères`)
  return file
}

const bodyText = () => page.evaluate(() => document.body.innerText)

// --- 1. Connexion caissier ----------------------------------------------------
await page.goto(`${baseUrl}/connexion`, { waitUntil: 'domcontentloaded' })
await page.waitForSelector('input[type="email"]', { timeout: 20_000 })
await page.type('input[type="email"]', email)
await page.type('input[type="password"]', password)
await Promise.all([
  page.waitForFunction(() => !location.pathname.includes('connexion'), { timeout: 25_000 }),
  page.click('button[type="submit"]'),
])
await sleep(1800)
const account = await (await fetch(`${baseUrl}/api/v1/auth/me`)).status // sonde neutre, sans cookie
ok('connexion caissier', `${email} (sonde anonyme : ${account})`)
const dashboard = await bodyText()
if (dashboard.includes('Tableau de bord')) ok('tableau de bord affiché sur téléphone')
else ko('tableau de bord absent', dashboard.slice(0, 80))
await shot('20-telephone-caissier-tableau-de-bord', 'tableau de bord caissier')

// --- 2. Menu : les écrans réservés ne doivent pas apparaître ------------------
await page.click('[aria-label="Ouvrir le menu"]')
await sleep(700)
// On lit le menu lui-même (`nav`), pas la page en arrière-plan du tiroir.
const menu = await page.evaluate(() => {
  const nav = document.querySelector('nav')
  return nav ? nav.innerText : ''
})
const forbidden = ['Employés', 'Rapports', 'Paramètres', 'Abonnement'].filter((item) => menu.includes(item))
if (forbidden.length === 0) ok('menu caissier sans Employés ni Rapports')
else ko('menu caissier trop large', forbidden.join(', '))
const expected = ['Lavages', 'Caisse', 'Dépenses', 'Paramètres'].filter((item) => menu.includes(item))
if (expected.length >= 3) ok('entrées attendues visibles', expected.join(', '))
else ko('entrées attendues manquantes', expected.join(', '))
await shot('21-telephone-caissier-menu', 'menu caissier')
await page.keyboard.press('Escape')
await sleep(500)

// --- 4. État réel de la caisse -------------------------------------------------
const sessionState = await page.evaluate(async () => {
  const response = await fetch('/api/v1/cash/sessions?limit=20', { credentials: 'same-origin' })
  if (!response.ok) return { error: response.status }
  const payload = await response.json()
  // Enveloppe variable selon les routes : on retient le premier tableau trouvé.
  const rows = Array.isArray(payload)
    ? payload
    : (Object.values(payload).find((value) => Array.isArray(value)) ?? [])
  return { open: rows.some((row) => row.status === 'open'), total: rows.length }
})
if (sessionState.error) ko('état de la caisse illisible', `HTTP ${sessionState.error}`)
else ok('état de la caisse lu par le caissier', sessionState.open ? 'une session est ouverte' : 'aucune session ouverte')

// --- 3. Enregistrer un lavage (le geste le plus fréquent de la journée) -------
/** Crée un lavage depuis le téléphone et renvoie sa référence. */
async function createWash({ capture = false } = {}) {
  await page.goto(`${baseUrl}/lavages`, { waitUntil: 'domcontentloaded' })
  await sleep(1600)
  await tap('Nouveau lavage')
  await page.waitForSelector('[aria-label="Prestation"]', { timeout: 15_000 })
  await sleep(400)
  if (capture) await shot('22-telephone-caissier-formulaire', 'formulaire de lavage')
  const service = await page.evaluate(() =>
    [...document.querySelector('[aria-label="Prestation"]').options].map((option) => option.value).find(Boolean),
  )
  await page.select('[aria-label="Prestation"]', service)
  await sleep(300)
  await tap('Enregistrer le lavage')
  await page.waitForFunction(() => !document.body.innerText.includes('Enregistrer le lavage'), { timeout: 20_000 })
  await sleep(1800)
  const found = ((await bodyText()).match(/L-\d{8}-\d+/) ?? [])[0]
  if (!found) ko('lavage non enregistré depuis le téléphone', (await bodyText()).slice(0, 120))
  if (capture) await shot('23-telephone-caissier-lavage-cree', 'lavage créé')
  return found
}

async function openPayment(reference) {
  const opened = await page.evaluate((reference) => {
    const row = [...document.querySelectorAll('tr')].find((item) => item.innerText.includes(reference))
    if (!row) return false
    const button = [...row.querySelectorAll('button')].find((item) => item.textContent.trim() === 'Encaisser')
    if (!button) return false
    button.click()
    return true
  }, reference)
  if (!opened) throw new Error(`bouton Encaisser introuvable pour ${reference}`)
  await page.waitForSelector('input[inputmode="numeric"]', { timeout: 15_000 })
  await sleep(500)
  await tap('Tout le reste à payer')
  await sleep(300)
  return Number(((await bodyText()).match(/Encaisser\s+([\d\s]+)FCFA/) ?? [])[1]?.replace(/\D/g, '') ?? 0)
}

// --- 5. Encaissement Mobile Money (hors caisse, toujours autorisé) ------------
const momoReference = await createWash({ capture: true })
if (momoReference) ok('lavage enregistré depuis le téléphone', momoReference)
if (!momoReference) throw new Error('premier lavage non créé')
let amount = await openPayment(momoReference)
await shot('24-telephone-caissier-encaissement', 'dialogue d’encaissement')
await choose('Moyen de paiement', 'mobile_money')
await sleep(400)
await tap('Encaisser ')
await page.waitForFunction(() => !document.body.innerText.includes('Montant reçu (FCFA)'), { timeout: 20_000 })
await sleep(1600)
const afterMomo = await bodyText()
if (/entièrement réglée|soldé/i.test(afterMomo)) ok('Mobile Money encaissé', `${momoReference} · ${amount} FCFA`)
else ko('encaissement Mobile Money non abouti', afterMomo.slice(0, 140).replace(/\n/g, ' '))
await shot('25-telephone-caissier-mobile-money', 'encaissement Mobile Money')

// --- 6. Espèces : le résultat doit correspondre à l’état de la caisse ---------
const cashReference = await createWash()
if (!cashReference) throw new Error('second lavage non créé')
amount = await openPayment(cashReference)
expectedFailures = [422, 409]
await tap('Encaisser ')
await sleep(2500)
const afterCash = await bodyText()
const settled = /entièrement réglée|soldé/i.test(afterCash)
const explained = /caisse/i.test(afterCash) && /ouvert/i.test(afterCash)
if (sessionState.open) {
  if (settled) ok('espèces acceptées : une caisse est ouverte', `${cashReference} · ${amount} FCFA`)
  else ko('espèces refusées alors qu’une caisse est ouverte', afterCash.slice(0, 140).replace(/\n/g, ' '))
} else if (explained) {
  ok('espèces refusées sans caisse ouverte, message explicite')
} else {
  ko('espèces acceptées sans caisse ouverte, ou refus non expliqué', afterCash.slice(0, 140).replace(/\n/g, ' '))
}
await shot('26-telephone-caissier-especes', 'encaissement en espèces')

expectedFailures = []
await browser.close()

console.log(`\nParcours du caissier sur téléphone (390 × 844) — ${baseUrl}\n`)
console.log(steps.join('\n'))
console.log(`\nLavages créés par le test : ${momoReference ?? '—'} et ${cashReference ?? '—'}`)
if (problems.length > 0) {
  console.log(`\n${problems.length} problème(s) :`)
  for (const problem of [...new Set(problems)]) console.log(`  • ${problem}`)
  process.exit(1)
}
console.log(`\nAucun débordement, aucune erreur JavaScript inattendue.`)
console.log(`Captures : ${path.resolve(outDir)}`)
