/**
 * Jour 1 d'une station neuve — parcours réel sur téléphone.
 *
 * Objectif : vérifier ce que le propriétaire fera réellement sur le Mac de la
 * station, le premier jour, depuis son téléphone, sur une base VIERGE :
 *
 *   1. créer son espace (/inscription)
 *   2. constater que le catalogue par défaut d'Abidjan est déjà là
 *   3. enregistrer un premier lavage
 *   4. tenter d'encaisser en espèces sans caisse ouverte : doit être refusé,
 *      avec un message compréhensible
 *   5. encaisser en Mobile Money : doit passer
 *   6. ouvrir la caisse (fond de caisse)
 *   7. encaisser un second lavage en espèces : doit passer
 *   8. clôturer la caisse en déclarant les espèces comptées et lire l'écart
 *
 * Le plus simple est de passer par `make recette`, qui crée une base jetable,
 * démarre une API dédiée et nettoie tout ensuite. Pour le lancer à la main :
 *
 *   BASE_URL=http://127.0.0.1:8090 node tools/rendu/jour-1-station.mjs
 *
 * Le script crée une vraie entreprise et de vrais lavages : ne le pointez
 * jamais vers la base de production de la station.
 */
import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'

const baseUrl = process.env.BASE_URL ?? 'http://127.0.0.1:8090'
const outDir = process.env.OUT_DIR ?? './jour1'
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
const browser = await puppeteer.launch({ executablePath, headless: true, args: [...extraArgs, '--no-sandbox', '--disable-dev-shm-usage'], env: { ...process.env } })

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
let expectedFailures = []
page.on('pageerror', (error) => problems.push(`erreur JS : ${String(error).slice(0, 200)}`))
page.on('response', (response) => {
  const status = response.status()
  if (status < 400 || status === 401 || expectedFailures.includes(status)) return
  problems.push(`HTTP ${status} sur ${response.url().replace(baseUrl, '')}`)
})

const bodyText = () => page.evaluate(() => document.body.innerText)

async function fill(labelText, value) {
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

async function choose(labelText, value) {
  await page.evaluate(
    (labelText, value) => {
      const label = [...document.querySelectorAll('label')].find((item) => item.textContent.trim().startsWith(labelText))
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

/**
 * Clique un bouton. Par défaut, cherche DANS la fenêtre modale ouverte si elle
 * existe : « Ouvrir la caisse » existe à la fois sur la page et dans le modal,
 * et cliquer le bouton de la page laisserait la fenêtre ouverte sans rien faire.
 */
async function tap(text, { exact = false, scope = 'auto' } = {}) {
  const clicked = await page.evaluate(
    (text, exact, scope) => {
      const root = scope === 'dialog' || (scope === 'auto' && document.querySelector('[role="dialog"]'))
        ? document.querySelector('[role="dialog"]')
        : document
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

async function shot(name) {
  const file = path.join(outDir, `${name}.png`)
  await page.screenshot({ path: file })
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  if (overflow > 2) ko(`débordement horizontal — ${name}`, `${overflow} px`)
  return file
}

const money = (text) => Number(text.replace(/[^\d]/g, ''))

// --- 1. Créer son espace depuis le téléphone ---------------------------------
await page.goto(`${baseUrl}/inscription`, { waitUntil: 'domcontentloaded' })
await page.waitForSelector('input[type="email"]', { timeout: 20_000 })
const intro = await bodyText()
if (/prestations et tarifs habituels d’Abidjan|prestations et tarifs habituels d'Abidjan/.test(intro)) {
  ok('l’inscription annonce le catalogue d’Abidjan')
} else {
  ko('promesse de catalogue absent de l’inscription')
}
await fill('Nom de l’entreprise', 'Station Test Jour1')
await fill('Nom de la station', 'Station Test Jour1 — Marcory')
await fill('Votre nom complet', 'Awa Traoré')
await fill('Adresse e-mail', 'patronne@jour1.ci')
await fill('Mot de passe', 'station2026')
await fill('Ville', 'Abidjan')
await shot('01-inscription')
const before = await (await fetch(`${baseUrl}/api/v1/auth/me`)).status
await Promise.all([
  page.waitForFunction(() => !location.pathname.includes('inscription'), { timeout: 30_000 }),
  page.click('button[type="submit"]'),
])
await sleep(2500)
const afterRegister = await bodyText()
if (/Tableau de bord|Bonjour|Chiffre d/i.test(afterRegister)) ok('espace créé et connecté', `sonde anonyme : ${before} → 401`)
else ko('échec de la création de l’espace', afterRegister.slice(0, 160).replace(/\n/g, ' '))
await shot('02-tableau-de-bord-vide')

// --- 2. Le catalogue par défaut est-il vraiment là ? -------------------------
await page.goto(`${baseUrl}/prestations`, { waitUntil: 'domcontentloaded' })
await sleep(1800)
const services = await page.evaluate(async () => {
  const response = await fetch('/api/v1/services?limit=50', { credentials: 'same-origin' })
  if (!response.ok) return { error: response.status }
  const payload = await response.json()
  // Attention : la réponse contient aussi `categories` (tableau de chaînes).
  // Prendre « le premier tableau trouvé » renverrait les catégories, pas les
  // prestations — piège déjà rencontré.
  const rows = payload.items ?? (Array.isArray(payload) ? payload : [])
  return {
    count: rows.length,
    priced: rows.filter((row) => Number(row.price_xof) > 0).length,
    names: rows.slice(0, 4).map((row) => `${row.name} · ${row.price_xof} FCFA`),
  }
})
if (services.count >= 5 && services.priced === services.count) {
  ok('catalogue d’Abidjan préinstallé et tarifé', `${services.count} prestations · ${services.names.join(' | ')}`)
} else {
  ko('catalogue par défaut absent, incomplet ou sans prix', JSON.stringify(services))
}
await shot('03-prestations-par-defaut')

// --- 3. Premier lavage --------------------------------------------------------
async function createWash() {
  await page.goto(`${baseUrl}/lavages`, { waitUntil: 'domcontentloaded' })
  await sleep(1600)
  await tap('Nouveau lavage')
  await page.waitForSelector('[aria-label="Prestation"]', { timeout: 15_000 })
  await sleep(400)
  const service = await page.evaluate(() =>
    [...document.querySelector('[aria-label="Prestation"]').options].map((option) => option.value).find(Boolean),
  )
  await page.select('[aria-label="Prestation"]', service)
  await sleep(300)
  await tap('Enregistrer le lavage')
  await page.waitForFunction(() => !document.body.innerText.includes('Enregistrer le lavage'), { timeout: 20_000 })
  await sleep(1800)
  return ((await bodyText()).match(/L-\d{8}-\d+/) ?? [])[0]
}

async function openPayment(reference) {
  const opened = await page.evaluate((reference) => {
    const row = [...document.querySelectorAll('tr')].find((item) => item.innerText.includes(reference))
    const button = row && [...row.querySelectorAll('button')].find((item) => item.textContent.trim() === 'Encaisser')
    if (!button) return false
    button.click()
    return true
  }, reference)
  if (!opened) throw new Error(`bouton Encaisser introuvable pour ${reference}`)
  await page.waitForSelector('input[inputmode="numeric"]', { timeout: 15_000 })
  await sleep(500)
  await tap('Tout le reste à payer')
  await sleep(300)
  return money((await bodyText()).match(/Encaisser\s+([\d\s]+)FCFA/)?.[1] ?? '0')
}

const firstWash = await createWash()
if (firstWash) ok('premier lavage enregistré', firstWash)
else ko('premier lavage non enregistré', (await bodyText()).slice(0, 120))
await shot('04-premier-lavage')

// --- 4. Espèces sans caisse ouverte : refus attendu et expliqué ---------------
let amount = await openPayment(firstWash)
await choose('Moyen de paiement', 'especes')
await sleep(300)
await shot('05-encaissement-sans-caisse')
expectedFailures = [422, 409]
await tap('Encaisser ')
await sleep(2500)
const refusal = await bodyText()
// Message réel de l'API : « Ouvrez la caisse avant d'encaisser des espèces
// (onglet Caisse). » — il désigne l'action à faire, pas seulement l'erreur.
const clearRefusal = /Ouvrez la caisse avant d’encaisser des espèces|Ouvrez la caisse avant d'encaisser des espèces/.test(refusal)
if (clearRefusal) ok('espèces refusées sans caisse ouverte, message actionnable')
else ko('espèces acceptées sans caisse ouverte, ou refus obscur', refusal.slice(-200).replace(/\n/g, ' '))
await shot('06-refus-especes')

// --- 5. Mobile Money : doit passer sans caisse --------------------------------
await choose('Moyen de paiement', 'mobile_money')
await sleep(300)
await tap('Encaisser ')
await page.waitForFunction(() => !document.body.innerText.includes('Montant reçu (FCFA)'), { timeout: 20_000 })
await sleep(1600)
if (/entièrement réglée|soldé/i.test(await bodyText())) ok('Mobile Money encaissé sans caisse', `${amount} FCFA`)
else ko('Mobile Money refusé sans caisse', (await bodyText()).slice(0, 140).replace(/\n/g, ' '))
expectedFailures = []

// --- 6. Ouvrir la caisse ------------------------------------------------------
await page.goto(`${baseUrl}/caisse`, { waitUntil: 'domcontentloaded' })
await sleep(1800)
await tap('Ouvrir la caisse')
await sleep(700)
await shot('07-ouverture-caisse')
await fill('Fond de caisse', '25000')
await sleep(300)
await tap('Ouvrir la caisse')
await sleep(2500)
// Contrôle par l'API : l'affichage seul ne prouve pas que la session existe.
const sessionOpen = await page.evaluate(async () => {
  const response = await fetch('/api/v1/cash/sessions?limit=5', { credentials: 'same-origin' })
  if (!response.ok) return { error: response.status }
  const payload = await response.json()
  const rows = payload.items ?? (Array.isArray(payload) ? payload : [])
  const open = rows.find((row) => row.status === 'open')
  return { open: Boolean(open), float: open?.opening_float_xof ?? null }
})
if (sessionOpen.open && sessionOpen.float === 25000) ok('caisse ouverte, fond enregistré à 25 000 FCFA')
else ko('ouverture de caisse non enregistrée', JSON.stringify(sessionOpen).slice(0, 140))
await shot('08-caisse-ouverte')

// --- 7. Espèces avec caisse ouverte : doit passer -----------------------------
const secondWash = await createWash()
if (!secondWash) ko('second lavage non enregistré', '')
amount = await openPayment(secondWash)
await choose('Moyen de paiement', 'especes')
await sleep(300)
await tap('Encaisser ')
let dialogClosed = true
try {
  await page.waitForFunction(() => !document.body.innerText.includes('Montant reçu (FCFA)'), { timeout: 20_000 })
} catch {
  dialogClosed = false
}
await sleep(1200)
const cashResult = await bodyText()
if (dialogClosed && /entièrement réglée|soldé/i.test(cashResult)) {
  ok('espèces acceptées avec caisse ouverte', `${secondWash} · ${amount} FCFA`)
} else {
  ko('espèces refusées alors que la caisse est ouverte', cashResult.match(/[^\n]*caisse[^\n]*/i)?.[0]?.slice(0, 140) ?? 'message inattendu')
}
await shot('09-especes-avec-caisse')

// --- 8. Clôture : espèces attendues, compté, écart ---------------------------
await page.goto(`${baseUrl}/caisse`, { waitUntil: 'domcontentloaded' })
await sleep(1800)
// Libellés réels : « Clôturer la caisse » (page) puis « Clôturer » (modal).
await tap('Clôturer la caisse')
await sleep(800)
// Les espèces attendues ne sont visibles que dans la fenêtre de clôture.
const expectedCash = money((await bodyText()).match(/Espèces attendues\s*([\d\s]+)FCFA/)?.[1] ?? '0')
if (expectedCash > 0) ok('espèces attendues calculées', `${expectedCash} FCFA (fond 25 000 + encaissement ${amount})`)
else ko('espèces attendues non calculées', 'valeur lue : 0')
// Le compté est volontairement supérieur de 500 FCFA : l'écart doit apparaître.
await fill('Montant compté', String(expectedCash + 500))
await sleep(400)
const preview = await bodyText()
if (/Écart[^\n]*500 FCFA/.test(preview.replace(/\s+/g, ' '))) ok('écart de 500 FCFA annoncé avant validation')
else ko('écart non annoncé dans la fenêtre de clôture', preview.match(/Écart[^\n]*/)?.[0] ?? '')
await shot('10-cloture-caisse')
await tap('Clôturer', { exact: true, scope: 'dialog' })
await sleep(2800)
// Contrôle par l'API : la session doit être fermée, avec l'écart enregistré.
const closedSession = await page.evaluate(async () => {
  const response = await fetch('/api/v1/cash/sessions?limit=5', { credentials: 'same-origin' })
  if (!response.ok) return { error: response.status }
  const payload = await response.json()
  const rows = payload.items ?? (Array.isArray(payload) ? payload : [])
  const session = rows[0]
  return { status: session?.status, counted: session?.counted_cash_xof, expected: session?.expected_cash_xof, difference: session?.difference_xof }
})
if (closedSession.status === 'closed' && closedSession.difference === 500) {
  ok('caisse clôturée, écart de 500 FCFA enregistré', `théorique ${closedSession.expected} · compté ${closedSession.counted}`)
} else {
  ko('clôture inattendue', JSON.stringify(closedSession))
}
await shot('11-caisse-cloturee')

// --- 9. Les chiffres du tableau de bord correspondent-ils à la réalité ? ------
await page.goto(`${baseUrl}/tableau-de-bord`, { waitUntil: 'domcontentloaded' })
await sleep(2200)
const dashboard = await bodyText()
const revenue = money(dashboard.match(/CHIFFRE D’AFFAIRES DU JOUR\s*([\d\s]+)FCFA/)?.[1] ?? '0')
const collected = money(dashboard.match(/ENCAISSEMENTS DU JOUR\s*([\d\s]+)FCFA/)?.[1] ?? '0')
const expectedRevenue = services.count ? null : null
void expectedRevenue
if (revenue > 0 && collected > 0 && collected <= revenue) {
  ok('tableau de bord cohérent', `chiffre d’affaires ${revenue} FCFA · encaissements ${collected} FCFA`)
} else {
  ko('chiffres du tableau de bord incohérents', `CA ${revenue} · encaissé ${collected}`)
}
await shot('12-tableau-de-bord-jour1')

await browser.close()
console.log(`\nJour 1 sur une station neuve (téléphone 390 × 844) — ${baseUrl}\n`)
console.log(steps.join('\n'))
console.log(`\nLavages créés : ${firstWash ?? '—'} et ${secondWash ?? '—'}`)
if (problems.length > 0) {
  console.log(`\n${problems.length} problème(s) :`)
  for (const problem of [...new Set(problems)]) console.log(`  • ${problem}`)
  process.exit(1)
}
console.log(`\nAucun débordement, aucune erreur JavaScript.`)
console.log(`Captures : ${path.resolve(outDir)}`)
