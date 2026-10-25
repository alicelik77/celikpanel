// alpha.108 (D-038 §14): the administrator chooses which PHP and Node.js
// versions customers are offered, on each runtime's own Manage page; a
// customer's own page says that a hidden version is kept and who decides;
// the refusal for a hidden version is translated with its next action.
// Müşterilere sunulan sürümler: yönetici seçer, müşteri kendi sayfasında görür.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';
import { enErrorsScreens } from '../src/i18n/screens/errors/en.ts';
import { trErrorsScreens } from '../src/i18n/screens/errors/tr.ts';
import { offeredVersionsURL } from './fixtures/shared-layer.mjs';

const { decodeOfferedVersions, isOffered, offeredLine } = await import(offeredVersionsURL);

const read = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');
const placeholders = (text) => [...text.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).sort().join(',');

test('the answer is decoded strictly and a version line is the server’s', () => {
  const value = decodeOfferedVersions({ hidden: { php: ['7.4'], node: ['20'] }, restricted: true });
  assert.equal(isOffered(value, 'php', '7.4'), false);
  assert.equal(isOffered(value, 'php', '8.4'), true);
  assert.equal(isOffered(value, 'node', '20.18.1'), false);
  assert.equal(isOffered(value, 'node', 'v22.14.0'), true);
  assert.equal(isOffered(null, 'php', '7.4'), true, 'unknown is never shown as hidden');
  assert.equal(offeredLine('php', '8.4'), '8.4');
  assert.equal(offeredLine('node', 'v24.21.0'), '24');
  assert.throws(() => decodeOfferedVersions({ hidden: { php: [1] }, restricted: false }));
  assert.throws(() => decodeOfferedVersions({ hidden: {} }));
});

test('every new sentence exists in English and Turkish with the same placeholders', () => {
  const keys = Object.keys(enLifecycleScreens).filter((key) => key.startsWith('life.offer.'));
  assert.ok(keys.length >= 12);
  for (const key of keys) {
    assert.ok(trLifecycleScreens[key], `tr ${key}`);
    assert.equal(placeholders(trLifecycleScreens[key]), placeholders(enLifecycleScreens[key]), key);
  }
  for (const key of ['err.VERSION_NOT_OFFERED', 'err.NO_VERSION_OFFERED']) {
    assert.ok(enErrorsScreens[key] && trErrorsScreens[key], key);
    assert.equal(placeholders(trErrorsScreens[key]), placeholders(enErrorsScreens[key]), key);
  }
  // Who acts and the next action (D-024).
  assert.match(enErrorsScreens['err.VERSION_NOT_OFFERED'], /^Nothing was changed: .*ask your administrator/);
  assert.match(enLifecycleScreens['life.offer.customerKept'], /keeps it and keeps running; ask your administrator/);
  assert.match(trLifecycleScreens['life.offer.customerKept'], /yöneticinize danışın/);
});

test('the control sits on each runtime’s own Manage page, not in the versions drawer', () => {
  assert.match(read('components/PHPManagement.tsx'), /<OfferedVersionsCard component="php" installed=\{versions\} \/>/);
  assert.match(read('components/ComponentDetail.tsx'), /serviceId === 'node' && record\?\.is_installed === true[\s\S]{0,200}<OfferedVersionsCard component="node"/);
  assert.doesNotMatch(read('components/ServiceList.tsx'), /OfferedVersionsCard/);
  const card = read('components/OfferedVersionsCard.tsx');
  // One request per change; after it, the server's list is read again and a
  // lost answer is never sent a second time.
  assert.match(card, /method: 'PUT'/);
  assert.match(card, /await handle\.retry\(\);/);
  assert.match(card, /setFailure\(t\('life\.offer\.lost'\)\)/);
});

test('a site’s own pickers keep its version and say when it is no longer offered', () => {
  const php = read('components/DomainPHPSettings.tsx');
  assert.match(php, /<OfferedVersionNote component="php" version=\{currentVersion\} offered=\{isOffered\(offered, 'php', currentVersion\)\} restricted=\{offered\.restricted\} \/>/);
  const node = read('components/HostingTypePanel.tsx');
  assert.match(node, /<OfferedVersionNote component="node" version=\{savedNode\}/);
  // The saved version stays in the list even when the server no longer lists it.
  assert.match(node, /\[state\.runtime_version, savedNode\]/);
  const card = read('components/OfferedVersionsCard.tsx');
  assert.match(card, /restricted \? 'life\.offer\.customerKept' : 'life\.offer\.adminKept'/);
});
