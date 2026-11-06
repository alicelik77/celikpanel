// A sentence that names a missing component gives the administrator the
// action itself (alpha.127; owner on Boston, 6 Nov 2026). AdminInstallLink is
// mounted for real: the administrator gets one button to that component's own
// install window on Components (nothing starts there before it is confirmed);
// anyone else gets only the sentence that says who acts.
// Eksik bileşeni adlandıran cümle yöneticiye eylemin kendisini verir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL } from './fixtures/shared-layer.mjs';
import { enMailDatabasesScreens as en } from '../src/i18n/screens/mailDatabases/en.ts';
import { trMailDatabasesScreens as tr } from '../src/i18n/screens/mailDatabases/tr.ts';

const stub = dataModule(`
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useAuth = () => ({ role: globalThis.linkRole });
  export const useNavigate = () => (path) => { globalThis.linkNavigated.push(path); return true; };
`);
const { AdminInstallLink } = await import(compileSource('components/AdminInstallLink.tsx', () => stub));
const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

async function render(role, props) {
  globalThis.linkRole = role;
  globalThis.linkNavigated = [];
  let tree;
  await act(async () => { tree = Renderer.create(React.createElement(AdminInstallLink, props)); });
  return tree;
}

test('the administrator gets one button to the component’s own install window', async () => {
  const tree = await render('admin', { component: 'cron', fallback: 'customer sentence' });
  const buttons = tree.root.findAll((node) => node.type === 'button');
  assert.equal(buttons.length, 1);
  assert.equal(buttons[0].props['data-install-link'], 'cron');
  assert.ok(JSON.stringify(tree.toJSON()).includes('services.installTitle'));
  assert.ok(!JSON.stringify(tree.toJSON()).includes('customer sentence'));
  await act(async () => buttons[0].props.onClick());
  assert.deepEqual(globalThis.linkNavigated, ['/components?component=cron&install=']);
});

test('everyone else sees only the sentence that says who acts', async () => {
  for (const role of ['customer', 'reseller', 'additional_user']) {
    const tree = await render(role, { component: 'phpmyadmin', fallback: 'customer sentence' });
    assert.equal(tree.root.findAll((node) => node.type === 'button').length, 0, role);
    assert.equal(JSON.stringify(tree.toJSON()), '"customer sentence"', role);
    const bare = await render(role, { component: 'node' });
    assert.equal(bare.toJSON(), null, role);
  }
});

test('the screens that name a missing component offer it to the administrator', () => {
  assert.match(read('components/DomainCronManager.tsx'), /<AdminInstallLink component="cron" \/>/);
  assert.match(read('components/DomainDatabaseManager.tsx'), /<AdminInstallLink\s+key=\{tl\.id\}\s+component=\{tl\.id\}\s+fallback=/);
  assert.match(read('components/DomainRuntimeApps.tsx'), /install: 'python' \}/);
  assert.match(read('components/DomainRuntimeApps.tsx'), /<AdminInstallLink component=\{blocker\.install\} \/>/);
  const serviceApps = read('components/DomainServiceApps.tsx');
  assert.match(serviceApps, /app\.reason === 'runtime_missing' \? 'node' : app\.reason === 'venv_missing' \? 'python' : null/);
  assert.match(serviceApps, /<AdminInstallLink component=\{missingComponent\(app\)!\} \/>/);
  const scan = read('components/DomainMalwareScan.tsx');
  assert.match(scan, /isAdmin && code === 'MALWARE_SIGNATURES_MISSING' && <ClamAVPageLink \/>/);
  assert.match(scan, /navigate\('\/components\/clamav'\)/);
});

test('a database tool that is missing tells everyone else who installs it', () => {
  assert.match(en['dbtools.install'], /your provider installs it/);
  assert.match(tr['dbtools.install'], /sağlayıcınız kurar/);
  assert.doesNotMatch(en['dbtools.install'], /from Components/);
});
