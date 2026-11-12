import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.118, the application catalog of a domain. What each application
// needs is said before Install, with the package the administrator installs;
// an application this site cannot take has no enabled Install; a site that
// already holds one shows it with its own actions; the administrator is told
// to offer an application first. Reading only: nothing here is sent.
//
// Uygulama kataloğu: her uygulamanın gereksinimi Kur'dan önce, yöneticinin
// kuracağı paketle söylenir; siteye uymayanın Kur düğmesi kapalıdır.

const source = readFileSync(new URL('../src/components/DomainAppsPanel.tsx', import.meta.url), 'utf8');
const icons = new Set(['AlertTriangle']);
for (const match of source.matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
  for (const item of match[1].split(',')) {
    const icon = item.trim();
    if (icon) icons.add(icon);
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const t = (key, vars) => (vars ? key + JSON.stringify(vars) : key);
  export const useI18n = () => ({ t, locale: globalThis.__appCatalogLocale ?? 'en' });
  export const useNavigate = () => () => true;
  export const showToast = () => {};
  export const AppAccountLine = () => null, IsolationFact = () => null;
  export const BlockingProblems = () => null;
`);
const shared = sharedLayer(stub);
const own = { '/lib/appCatalog': shared.compile('lib/appCatalog.ts') };
const { DomainAppsPanel } = await import(shared.compile('components/DomainAppsPanel.tsx', own));
const { appText, decodeCatalog, decodeDomainApps } = await import(own['/lib/appCatalog']);

const D = '/api/v1/domains/1';
const app = (id, extra) => ({ id, name: id, version: '1', description: '', available: true, php: '8.3+', extensions: [], databases: ['mariadb'], ...extra });
const ok = { state: 'ok', php_ok: true, missing: [], database_ok: true, disk_ok: true };

let tree;
const originalFetch = globalThis.fetch;
const requests = [];
async function mount(body) {
  requests.length = 0;
  globalThis.fetch = async (input, init = {}) => {
    requests.push(`${init.method || 'GET'} ${String(input)}`);
    return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
  };
  await act(async () => { tree = Renderer.create(React.createElement(DomainAppsPanel, { domainId: 1, domainName: 'example.com' })); });
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); });
}
async function unmount() {
  if (tree) await act(async () => tree.unmount());
  globalThis.fetch = originalFetch;
}
const text = () => {
  const walk = (node) => (typeof node === 'string' ? node : !node ? '' : Array.isArray(node) ? node.map(walk).join(' ') : walk(node.children));
  return walk(tree.toJSON());
};
const label = (node) => {
  const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join(''));
  return walk(node);
};
const installButton = (id) => tree.root.find((node) => node.props['data-catalog-app'] === id).findAll((node) => node.type === 'button').find((b) => label(b).includes('apps.install'));

test('unmet requirements are named, with the package, and Install stays off', async () => {
  await mount({
    apps: [
      app('prestashop', { requirements: { ...ok, state: 'unmet', missing: [{ extension: 'intl', package: 'php8.3-intl' }] } }),
      app('drupal', { requirements: { ...ok, state: 'unmet', php_ok: false, php_reason: 'Drupal 11 needs PHP 8.3 or newer; the site runs PHP 8.2' } }),
      app('joomla', { requirements: ok }),
    ],
    installed: [], site: { php_version: '8.3', occupied: false },
  });
  try {
    const shown = text();
    assert.ok(shown.includes('apps.missingPackages{"packages":"php8.3-intl"}'), shown);
    assert.ok(shown.includes('Drupal 11 needs PHP 8.3 or newer'), shown);
    assert.equal(installButton('prestashop').props.disabled, true);
    assert.equal(installButton('drupal').props.disabled, true);
    assert.equal(installButton('joomla').props.disabled, false);
    assert.ok(requests.every((line) => line.startsWith('GET ')), requests.join(', '));
  } finally { await unmount(); }
});

test('a site that holds an application shows it with Retry and offers no second install', async () => {
  await mount({
    apps: [app('joomla', { requirements: ok })],
    installed: [{ operation_id: 'a'.repeat(32), app_id: 'nextcloud', name: 'Nextcloud', version: '35.0.2', status: 'setup_failed', site_url: 'https://example.com/', can_retry: true, can_remove: true, last_error: 'occ ended with exit code 1' }],
    site: { php_version: '8.3', occupied: true },
  });
  try {
    const shown = text();
    assert.ok(shown.includes('apps.status.setupFailed') && shown.includes('occ ended with exit code 1'), shown);
    assert.ok(shown.includes('apps.retrySetup') && shown.includes('apps.remove'), shown);
    assert.ok(shown.includes('apps.siteOccupied'), shown);
    assert.equal(installButton('joomla').props.disabled, true);
  } finally { await unmount(); }
});

test('the administrator is told to offer an application first', async () => {
  await mount({
    apps: [app('nextcloud', { available: false, refused_by: 'administrator', grantor: 'the administrator' })],
    installed: [], site: { php_version: '8.3', occupied: false, administrator: true },
  });
  try {
    assert.ok(text().includes('apps.offerFirst'), text());
    assert.equal(installButton('nextcloud').props.disabled, true);
  } finally { await unmount(); }
});

// alpha.130: the catalog's description, download check and risk note come in
// Turkish from the server (an additive "tr" object); a Turkish screen shows
// them, an English screen or an older panel without them shows English.
const turkish = { description: 'Tartışma forumu.', checked: 'phpBB’nin yayımladığı değerle aynı', risk: 'Forumlar spam çeker.' };

test('the decoder keeps the Turkish texts and drops an empty or malformed object', () => {
  const [withTR, without, empty, malformed] = decodeCatalog({ apps: [
    { id: 'a', name: 'A', description: 'A forum.', checked: 'equal', risk: 'Spam.', tr: turkish },
    { id: 'b', name: 'B', description: 'B.' },
    { id: 'c', name: 'C', tr: { description: '', checked: '', risk: '' } },
    { id: 'd', name: 'D', tr: ['x'] },
  ] });
  assert.deepEqual(withTR.tr, turkish);
  assert.equal(without.tr, undefined);
  assert.equal(empty.tr, undefined);
  assert.equal(malformed.tr, undefined);
  assert.deepEqual(decodeDomainApps({ apps: [{ id: 'a', name: 'A', tr: turkish }], installed: [] }).apps[0].tr, turkish);
});

test('appText: Turkish when the screen is Turkish and the text exists, else the English text', () => {
  const app = { description: 'A forum.', checked: 'equal', risk: 'Spam.', tr: { ...turkish, checked: '' } };
  assert.equal(appText(app, 'description', 'tr'), turkish.description);
  assert.equal(appText(app, 'risk', 'tr'), turkish.risk);
  assert.equal(appText(app, 'checked', 'tr'), 'equal', 'a missing Turkish text falls back to English');
  assert.equal(appText(app, 'description', 'en'), 'A forum.');
  assert.equal(appText({ description: 'A forum.', checked: 'equal', risk: 'Spam.' }, 'risk', 'tr'), 'Spam.', 'an older panel sends no Turkish');
});

test('a Turkish screen shows the Turkish description, check and risk note; English stays English', async () => {
  const entry = app('phpbb', { description: 'A discussion forum.', checked: 'equal to the published digest', risk: 'Forums attract spam.', pinned: true, requirements: ok, tr: turkish });
  const body = { apps: [entry], installed: [], site: { php_version: '8.3', occupied: false } };
  const openDetails = async () => {
    const details = tree.root.findAll((node) => node.type === 'button').find((b) => label(b).includes('apps.details'));
    await act(async () => details.props.onClick());
  };
  globalThis.__appCatalogLocale = 'tr';
  await mount(body);
  try {
    await openDetails();
    const shown = text();
    for (const value of Object.values(turkish)) assert.ok(shown.includes(value), shown);
    assert.ok(!shown.includes('A discussion forum.') && !shown.includes('Forums attract spam.'), shown);
  } finally { await unmount(); delete globalThis.__appCatalogLocale; }
  await mount(body);
  try {
    await openDetails();
    const shown = text();
    assert.ok(shown.includes('A discussion forum.') && shown.includes('equal to the published digest') && shown.includes('Forums attract spam.'), shown);
    assert.ok(!shown.includes(turkish.description), shown);
  } finally { await unmount(); }
});

test('the offered applications screen localizes its texts and names the application being saved', () => {
  const offered = readFileSync(new URL('../src/components/OfferedAppsSection.tsx', import.meta.url), 'utf8');
  // alpha.132: Save asks first, with the risk; a saved or refused application is named by its name.
  assert.match(offered, /t\('appOffer\.confirm', \{ name: app\.name, risk: appText\(app, 'risk', locale\) \}\)/);
  for (const field of ['description', 'checked', 'risk']) assert.ok(offered.includes(`appText(app, '${field}', locale)`), field);
  assert.ok(offered.includes('nameOf: (id) => appOf(id)?.name ?? id'), 'a saved or refused application is named by its name');
  assert.ok(!/\{app\.(description|checked|risk)\}/.test(offered + source), 'no raw English catalog text is shown');
});
