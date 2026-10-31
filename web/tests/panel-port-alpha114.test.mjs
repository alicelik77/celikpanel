// alpha.114: new installations use panel port 1923; an existing one keeps its
// port and can change it in Settings > Panel. The card reads the server's
// state, polls only by reading while a change is open, and every word is in
// English and Turkish with the same placeholders.
//
// alpha.114: yeni kurulum 1923; mevcut kurulum portunu korur ve Ayarlar >
// Panel'den değiştirebilir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { decodePanelPortView, panelAddressOn, PANEL_PORT_URL } from '../src/lib/panelPort.ts';
import { enSettingsScreens as en } from '../src/i18n/screens/settings/en.ts';
import { trSettingsScreens as tr } from '../src/i18n/screens/settings/tr.ts';

const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('the decoder accepts the server view and refuses anything else', () => {
  const view = decodePanelPortView({ port: 2083, default_port: 1923, legacy_port: 2083, request_port: 2083, blocked: null,
    change: { id: 'pp-1', from_port: 2083, to_port: 1923, status: 'trial', seconds_left: 540 } });
  assert.equal(view.change.to_port, 1923);
  assert.equal(decodePanelPortView({ port: 1923, default_port: 1923, legacy_port: 2083, request_port: 0, change: null, blocked: { code: 'PANEL_PORT_PAIRED', reason: 'x' } }).blocked.code, 'PANEL_PORT_PAIRED');
  for (const bad of [null, {}, { port: 0, default_port: 1923, legacy_port: 2083, request_port: 0, change: null, blocked: null },
    { port: 1923, default_port: 1923, legacy_port: 2083, request_port: 0, change: { id: 'x', from_port: 1, to_port: 2, status: 'other' }, blocked: null },
    { port: 1923, default_port: 1923, legacy_port: 2083, request_port: 0, change: null, blocked: 'x' }]) {
    assert.throws(() => decodePanelPortView(bad), JSON.stringify(bad));
  }
});

test('the new address keeps the host the browser uses', () => {
  assert.equal(panelAddressOn(1923, { protocol: 'https:', hostname: 'panel.example.test' }), 'https://panel.example.test:1923/settings?section=panel');
  assert.equal(panelAddressOn(1923, { protocol: 'https:', hostname: '2001:db8::1' }), 'https://[2001:db8::1]:1923/settings?section=panel');
});

test('the card reads by itself and changes only on a button', () => {
  assert.equal(PANEL_PORT_URL, '/api/v1/panel/port');
  const card = read('components/PanelPortCard.tsx');
  assert.equal([...card.matchAll(/method: 'POST'/g)].length, 1);
  // The poll is a read (retry), never a send.
  assert.match(card, /setInterval\(\(\) => void retry\(\)/);
  assert.match(read('components/Settings.tsx'), /<PanelPortCard \/>/);
});

test('no screen assumes 2083 as the panel port any more', () => {
  for (const file of ['components/Settings.tsx', 'components/ServerSetup.tsx', 'components/ServerSetupWizardSteps.tsx', 'components/ServerSetupDNSConnections.tsx']) {
    assert.doesNotMatch(read(file), /2083/, file);
  }
});

test('every panel port sentence is in English and Turkish with the same placeholders', () => {
  const keys = Object.keys(en).filter((key) => key.startsWith('panelPort.'));
  assert.ok(keys.length > 40);
  const vars = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',');
  for (const key of keys) {
    assert.equal(typeof tr[key], 'string', `tr ${key}`);
    assert.equal(vars(tr[key]), vars(en[key]), key);
  }
  const card = read('components/PanelPortCard.tsx');
  for (const [, key] of card.matchAll(/t\('(panelPort\.[\w.]+)'/g)) assert.equal(typeof en[key], 'string', key);
});
