import { readFileSync } from 'node:fs';

// The product's copy lives in several files per locale since register R-060:
// the shell half, which boots with the application, and the screen half, which
// arrives beside the screen that needs it. The screen half is itself stored in
// parts, one directory per group of screens under src/i18n/screens (since
// v0.1.0-alpha.97; before that two files, ./screens and ./screens/server),
// because one file outgrew the per-chunk bundle budget; the provider loads
// every part before any screen renders. A contract about the product's copy is
// a contract about every part, so every test reads them as one catalogue and
// no test has to know which part a key landed in.
//
// Ürünün metni R-060'tan bu yana her dil için birden çok dosyada yaşar:
// uygulamayla birlikte açılan kabuk yarısı ve ihtiyaç duyan ekranla birlikte
// gelen ekran yarısı. Ekran yarısı da paket bütçesi yüzünden src/i18n/screens
// altında ekran grubu başına bir parçadadır. Metne dair bir sözleşme her
// parçaya dairdir; bu yüzden testler hepsini tek katalog olarak okur.
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

// Every part of the screen half, in load order. index.tsx must load each one.
// Ekran yarısının her parçası; index.tsx her birini yüklemelidir.
export const screenParts = [
  'setup', 'sites', 'siteConfig', 'dnsCertificates', 'components', 'componentPages',
  'mailDatabases', 'security', 'dashboard', 'settings', 'errors', 'lifecycle', 'databaseClusters', 'vendorLanes', 'webmailAddress', 'setupWizard', 'setupRegistrar', 'setupJoin',
];

const capitalised = (part) => part[0].toUpperCase() + part.slice(1);

// Every file of the screen half, per locale, in load order.
export const screenCatalogueFiles = {
  en: screenParts.map((part) => `../src/i18n/screens/${part}/en.ts`),
  tr: screenParts.map((part) => `../src/i18n/screens/${part}/tr.ts`),
};

// The exported object of each part, `en<Part>Screens` / `tr<Part>Screens`.
export const screenPartExport = (part, locale) => `${locale}${capitalised(part)}Screens`;

export const englishScreens = screenCatalogueFiles.en.map(read).join('');
export const turkishScreens = screenCatalogueFiles.tr.map(read).join('');

export const englishCatalogue = read('../src/i18n/en.ts') + englishScreens + read('../src/i18n/setupDNS/en.ts');
export const turkishCatalogue = read('../src/i18n/tr.ts') + turkishScreens + read('../src/i18n/setupDNS/tr.ts');

// Every screen part merged, as the provider merges them. enServerScreens and
// trServerScreens are the same merged objects under the names tests used when
// the screen half was two files; a key is in exactly one part either way.
// Bütün ekran parçaları sağlayıcının birleştirdiği gibi birleştirilmiş.
async function mergedScreens(locale) {
  const merged = {};
  for (const part of screenParts) {
    const module = await import(new URL(`../src/i18n/screens/${part}/${locale}.ts`, import.meta.url).href);
    const value = module[screenPartExport(part, locale)];
    if (!value || typeof value !== 'object') throw new Error(`screen part ${part}/${locale} has no ${screenPartExport(part, locale)}`);
    for (const key of Object.keys(value)) {
      if (key in merged) throw new Error(`screen key ${key} is in two parts (${part}/${locale})`);
    }
    Object.assign(merged, value);
  }
  return merged;
}

export const enScreens = await mergedScreens('en');
export const trScreens = await mergedScreens('tr');
export const enServerScreens = enScreens;
export const trServerScreens = trScreens;
