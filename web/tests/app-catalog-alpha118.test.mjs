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
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const showToast = () => {};
  export const AppAccountLine = () => null, IsolationFact = () => null;
  export const BlockingProblems = () => null;
`);
const shared = sharedLayer(stub);
const own = { '/lib/appCatalog': shared.compile('lib/appCatalog.ts') };
const { DomainAppsPanel } = await import(shared.compile('components/DomainAppsPanel.tsx', own));

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
