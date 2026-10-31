// Bundle budget. Every JavaScript chunk of the build is measured raw and
// gzipped against a ceiling, and the build fails when one is over it.
//
// The product's copy (web/src/i18n) and where a new key goes:
// - The SHELL catalogue (src/i18n/en.ts, tr.ts) is on the critical boot path:
//   only copy an eagerly loaded module can reach (login, navigation rail,
//   profile menu, a running operation, refusal codes). Everything else is
//   screen copy.
// - The SCREEN catalogue is split into parts, one directory per group of
//   screens under src/i18n/screens/<part>/{en,tr}.ts. A new key goes into the
//   part that already holds its prefix (each part's header lists its
//   prefixes); a new prefix goes into the part of the screen that uses it.
//   Every part is fetched in parallel and merged before any screen renders
//   (src/i18n/index.tsx), so the choice of part never changes what a screen
//   can show.
// - Each part has its own ceiling below (screenPartBudgets), set with at least
//   25% headroom when the part was made (v0.1.0-alpha.97). The ceiling is not
//   raised to make room. When a part's headroom runs out, split it again along
//   screen boundaries: move whole prefixes into a new directory with an en.ts
//   (`export const en<Part>Screens = {…} as const`, `<Part>ScreenKey`) and a
//   tr.ts (`Record<<Part>ScreenKey, string>`, so a missing Turkish key fails
//   tsc), then add the part to src/i18n/index.tsx (both lists), to ScreenKey in
//   src/i18n/en.ts, to screenParts in tests/locale-catalogue.mjs and here,
//   each with headroom of at least 25% again.
//
// Paket bütçesi. Ekran katalogu src/i18n/screens altında parçalara bölünür; yeni
// anahtar, önekini tutan parçaya girer. Her parçanın kendi tavanı vardır ve
// tavan yükseltilmez: payı biten parça ekran sınırlarından yeniden bölünür
// (index.tsx, en.ts ScreenKey, tests/locale-catalogue.mjs ve bu dosya), her
// yeni parça en az %25 payla.
import { readFile, readdir } from 'node:fs/promises'
import { basename, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'

const distDir = fileURLToPath(new URL('../dist/', import.meta.url))
const assetsDir = join(distDir, 'assets')
const html = await readFile(join(distDir, 'index.html'), 'utf8')
const manifest = JSON.parse(await readFile(join(distDir, '.vite', 'manifest.json'), 'utf8'))
const entryMatch = html.match(/<script\b[^>]*\bsrc=["']\/?(?:\.\/)?assets\/([^"']+\.js)["'][^>]*>/i)

if (!entryMatch) {
  throw new Error('Bundle budget: JavaScript entry was not found in dist/index.html')
}

const entryName = basename(entryMatch[1])
const budgets = {
  entry: { raw: 300 * 1024, gzip: 90 * 1024 },
  // Actionable deletion and DNS-setup guidance add less than 0.25 KiB to the Turkish locale.
  async: { raw: 160.25 * 1024, gzip: 45.25 * 1024 },
  // The global, fail-closed update tracker is boot-critical by design. Its
  // canonical cross-tab fence adds less than 1 KiB and must not be lazy-loaded.
  boot: { raw: 361 * 1024, gzip: 110 * 1024 },
  route: { raw: 280 * 1024, gzip: 80 * 1024 },
}

// The screen catalogue parts (v0.1.0-alpha.97). One ceiling per part, shared by
// its English and Turkish files and set from the larger (Turkish) one with at
// least 25% headroom when the part was made: raw / gzip at the time were
// setup 55.84 / 14.61, sites 47.81 / 13.17, components 43.64 / 11.71,
// siteConfig 35.86 / 6.20, settings 30.54 / 9.08, mailDatabases 29.66 / 8.42,
// dashboard 28.54 / 6.00, dnsCertificates 28.09 / 8.35, security 19.51 / 6.02
// KiB. Before the split the screen copy was two files per locale, each within
// 1 KiB of the general async ceiling.
// Ekran katalogu parçaları; her parçanın kendi tavanı, oluşturulduğunda en az
// %25 payla.
const kib = (raw, gzip) => ({ raw: raw * 1024, gzip: gzip * 1024 })
const screenPartBudgets = {
  setup: kib(80, 21),
  sites: kib(68, 19),
  components: kib(64, 17),
  siteConfig: kib(52, 9),
  settings: kib(44, 13),
  mailDatabases: kib(43, 12),
  dashboard: kib(41, 9),
  dnsCertificates: kib(41, 12),
  security: kib(28, 9),
  // v0.1.0-alpha.98: the coded refusals only screens show moved out of the
  // shell (the boot path had 0.25 KiB left; it is 329.16 KiB after). Set with
  // at least 25% headroom from its Turkish file: 32.40 / 8.24 KiB.
  errors: kib(44, 11.5),
  // v0.1.0-alpha.106: each component's management page (component., svc.,
  // nginx., postfix., pdns., dovecot., mariadb., vsftpd.) moved out of
  // "components" when the Components list's removal copy used its headroom
  // (Turkish 65.09 / 16.69 KiB before, 50.62 / 13.11 after). Set with at least
  // 25% headroom from its Turkish file: 14.51 / 4.51 KiB.
  componentPages: kib(20, 6.25),
  // v0.1.0-alpha.107 (D-038): versions, use and security of every component
  // (life.). A new part, set with at least 25% headroom from its Turkish file:
  // 23.99 / 6.78 KiB.
  lifecycle: kib(32.5, 9.25),
  // v0.1.0-alpha.108: PostgreSQL majors side by side (pgc.). A new part, set
  // with at least 25% headroom from its Turkish file: 15.16 / 4.43 KiB.
  databaseClusters: kib(20.5, 6),
  // v0.1.0-alpha.108: the vendor lane (lane.): the operating system's
  // packages by default, a vendor repository only with its confinement and
  // automatic updates. A new part, set with at least 25% headroom from its
  // Turkish file: 8.41 / 2.87 KiB.
  vendorLanes: kib(11.5, 3.9),
  // v0.1.0-alpha.110: webmail on its own address (life.addr.), split out of
  // "lifecycle", which had no headroom left. A new part, set with at least 25%
  // headroom from its Turkish file: 16.14 / 4.44 KiB.
  webmailAddress: kib(20.5, 5.7),
  // v0.1.0-alpha.111/112: the setup wizard v2 (setup.v2., setup.v3.), split
  // out of "setup", which had 1.9 KiB left. A new part, set with at least 25%
  // headroom from its Turkish file: 18.35 / 5.71 KiB. Raised at alpha.113 to
  // keep 25% headroom over 19.98 / 6.15 KiB (review fixes of alpha.112).
  setupWizard: kib(27, 8.25),
  // v0.1.0-alpha.113: the live registrar card (setup.v4.). A new part, set
  // with at least 25% headroom from its Turkish file: 3.05 / 1.21 KiB.
  setupRegistrar: kib(8, 3),
  // v0.1.0-alpha.114: the second server joining by code (setup.v5.). A new
  // part, set with at least 25% headroom from its Turkish file: 10.58 / 2.88 KiB.
  setupJoin: kib(14.5, 4),
  // v0.1.0-alpha.114 review fixes: the setup link's waits and verified states
  // (setup.v5.wait., setup.v5.check.), split out of "setupJoin" when they took
  // it over its ceiling (Turkish 15.05 / 3.93 KiB). A new part, set with at
  // least 25% headroom from its Turkish file: 5.90 / 1.77 KiB. Raised with
  // the Beta-1 re-run fixes (the distribution-update, ready-report and
  // rejoin waits) to keep 25% headroom over 8.38 / 2.37 KiB.
  setupLink: kib(13, 4),
  // The automatic deletion check between two servers joined by code
  // (setup.v5.inspection. and its waits), split out of "setupLink", which it
  // took over its ceiling. A new part, set with at least 25% headroom from its
  // Turkish file: 7.54 / 2.13 KiB.
  setupInspection: kib(10, 2.75),
  // v0.1.0-alpha.115: the DNS page's overview and the first-site offer
  // (dnsov.). A new part, set with at least 25% headroom from its Turkish
  // file: 15.36 / 4.24 KiB. Raised with the alpha.115 review fixes to keep
  // 25% headroom over 19.34 / 5.16 KiB.
  dnsOverview: kib(26, 7),
  // v0.1.0-alpha.113 nginx fixes: the default HTTPS server card (nginxdh.),
  // split out of "componentPages", which had no headroom left. A new part,
  // set with at least 25% headroom from its Turkish file: 4.63 / 1.73 KiB.
  nginxDefaultHttps: kib(6.5, 2.4),
  // 2026-10-31: the reviewed reasons of an unpublished DNS change
  // (err.DNS_PUBLICATION_FAILED.<reason>), split out of "errors" when the
  // held-Agent reason took it over its ceiling (Turkish 44.61 KiB before). A
  // new part, set with at least 25% headroom from its Turkish file: 7.52 /
  // 2.35 KiB.
  dnsPublication: kib(10.5, 4),
  // alpha.114: a domain's protected folders and redirects (siteFeatures.), the
  // refusal codes of those two screens and the account page's two-factor
  // recovery codes (account.recovery.). A new part, set with at least 25%
  // headroom from its Turkish file (about 8.7 / 2.7 KiB as source).
  siteFeatures: kib(11.5, 4),
}

// Translation chunks, named by source. The shell halves are held to the
// per-chunk async ceiling, each screen part to its own ceiling; naming them
// makes the check fail loudly if one disappears or is merged into another.
// Çeviri parçaları kaynak adıyla listelenir.
const namedChunkBudgets = {
  'src/i18n/en.ts': budgets.async,
  'src/i18n/tr.ts': budgets.async,
  ...Object.fromEntries(Object.entries(screenPartBudgets).flatMap(([part, limit]) => [
    [`src/i18n/screens/${part}/en.ts`, limit],
    [`src/i18n/screens/${part}/tr.ts`, limit],
  ])),
  // The help texts are fetched when a help drawer is opened. As a static part
  // of every page that carries the Help button they had brought the Settings
  // route to 0.29 KiB of its limit (2026-10-09).
  // Yardım metinleri, yardım çekmecesi açıldığında getirilir.
  'src/help/serviceHelp.ts': budgets.async,
}

const jsNames = (await readdir(assetsDir))
  .filter((name) => name.endsWith('.js'))
  .sort()

if (!jsNames.includes(entryName)) {
  throw new Error(`Bundle budget: entry ${entryName} is missing from dist/assets`)
}

const measurements = await Promise.all(jsNames.map(async (name) => {
  const source = await readFile(join(assetsDir, name))
  return {
    name,
    kind: name === entryName ? 'entry' : 'async',
    raw: source.byteLength,
    gzip: gzipSync(source, { level: 9 }).byteLength,
  }
}))
const measurementByName = new Map(measurements.map((item) => [item.name, item]))
const sourceByFile = new Map(Object.values(manifest)
  .filter((item) => item.file?.endsWith('.js'))
  .map((item) => [basename(item.file), item.src ?? item.name]))

const failures = []
for (const source of Object.keys(namedChunkBudgets)) {
  if (![...sourceByFile.values()].includes(source)) {
    failures.push(`expected a separate chunk for ${source}, found none`)
  }
}

const chunkRows = []
for (const item of measurements) {
  const source = sourceByFile.get(item.name)
  const limit = namedChunkBudgets[source] ?? budgets[item.kind]
  chunkRows.push({ label: `${item.name}${source ? ` (${source})` : ''}`, ...item, limit })
  if (item.raw > limit.raw || item.gzip > limit.gzip) {
    failures.push(
      `${item.name}: ${format(item.raw)} raw / ${format(item.gzip)} gzip ` +
      `(limit ${format(limit.raw)} / ${format(limit.gzip)})`,
    )
  }
}

const entry = measurements.find((item) => item.kind === 'entry')
const largestAsync = measurements
  .filter((item) => item.kind === 'async')
  .sort((a, b) => b.raw - a.raw)[0]

const entryManifest = Object.entries(manifest).find(([, item]) => item.isEntry && item.src === 'index.html')
if (!entryManifest) {
  throw new Error('Bundle budget: manifest has no entry')
}
const entryFiles = collectStaticFiles(entryManifest[0])

const localeEntries = Object.entries(manifest)
  .filter(([, item]) => item.src === 'src/i18n/en.ts' || item.src === 'src/i18n/tr.ts')
if (localeEntries.length !== 2) {
  failures.push(`expected two locale entries, found ${localeEntries.length}`)
}

const bootPayloads = localeEntries.map(([key]) => measureFiles(
  new Set([...entryFiles, ...collectStaticFiles(key)]),
))
const bootRaw = Math.max(0, ...bootPayloads.map((item) => item.raw))
const bootGzip = Math.max(0, ...bootPayloads.map((item) => item.gzip))

if (bootRaw > budgets.boot.raw || bootGzip > budgets.boot.gzip) {
  failures.push(
    `boot path: ${format(bootRaw)} raw / ${format(bootGzip)} gzip ` +
    `(limit ${format(budgets.boot.raw)} / ${format(budgets.boot.gzip)})`,
  )
}

const routePayloads = Object.entries(manifest)
  .filter(([, item]) => item.isDynamicEntry && item.src?.startsWith('src/components/'))
  .map(([key, item]) => {
    // The manifest links shared route chunks back to the already-loaded HTML
    // entry. Route budgets measure the incremental navigation payload only.
    const files = collectStaticFiles(key)
    for (const entryFile of entryFiles) files.delete(entryFile)
    return {
      name: item.src,
      ...measureFiles(files),
    }
  })

for (const route of routePayloads) {
  if (route.raw > budgets.route.raw || route.gzip > budgets.route.gzip) {
    failures.push(
      `${route.name} route payload: ${format(route.raw)} raw / ${format(route.gzip)} gzip ` +
      `(limit ${format(budgets.route.raw)} / ${format(budgets.route.gzip)})`,
    )
  }
}

const largestRoute = routePayloads.sort((a, b) => b.raw - a.raw)[0]

console.log(
  `Bundle budget: entry file ${format(entry.raw)} raw / ${format(entry.gzip)} gzip; ` +
  `critical boot ${format(bootRaw)} / ${format(bootGzip)}; ` +
  `largest route ${largestRoute ? `${largestRoute.name} ${format(largestRoute.raw)} / ${format(largestRoute.gzip)}` : 'none'}; ` +
  `largest individual async ${largestAsync ? `${largestAsync.name} ${format(largestAsync.raw)} / ${format(largestAsync.gzip)}` : 'none'}`,
)

// Every measured part with its limit and the headroom left, largest share of
// its raw ceiling first, so the next part to approach its line is at the top.
// Her ölçülen parça, sınırı ve kalan payıyla; sınırına en yakın olan en üstte.
const rows = [
  ...chunkRows.map((row) => ({ ...row, label: `chunk ${row.label}` })),
  { label: 'critical boot path', raw: bootRaw, gzip: bootGzip, limit: budgets.boot },
  ...routePayloads.map((route) => ({ label: `route ${route.name}`, ...route, limit: budgets.route })),
].sort((a, b) => b.raw / b.limit.raw - a.raw / a.limit.raw)
console.log('Bundle budget per part: raw size / limit (headroom) | gzip size / limit (headroom)')
for (const row of rows) {
  console.log(
    `  ${row.label}: ${format(row.raw)} / ${format(row.limit.raw)} (${headroom(row.raw, row.limit.raw)}) | ` +
    `${format(row.gzip)} / ${format(row.limit.gzip)} (${headroom(row.gzip, row.limit.gzip)})`,
  )
}

if (failures.length > 0) {
  console.error('Bundle budget exceeded:')
  for (const failure of failures) console.error(`- ${failure}`)
  process.exitCode = 1
}

function format(bytes) {
  return `${(bytes / 1024).toFixed(2)} KiB`
}

function headroom(size, limit) {
  const left = limit - size
  return `${left < 0 ? 'OVER ' : ''}${format(Math.abs(left))}${left < 0 ? '' : ' left'}, ${((left / limit) * 100).toFixed(1)}%`
}

function collectStaticFiles(key, files = new Set(), seen = new Set()) {
  if (seen.has(key)) return files
  seen.add(key)

  const item = manifest[key]
  if (!item) throw new Error(`Bundle budget: missing manifest node ${key}`)
  if (item.file?.endsWith('.js')) files.add(basename(item.file))
  for (const importedKey of item.imports ?? []) {
    collectStaticFiles(importedKey, files, seen)
  }
  return files
}

function measureFiles(names) {
  let raw = 0
  let gzip = 0
  for (const name of names) {
    const item = measurementByName.get(name)
    if (!item) throw new Error(`Bundle budget: no measurement for ${name}`)
    raw += item.raw
    gzip += item.gzip
  }
  return { raw, gzip }
}
