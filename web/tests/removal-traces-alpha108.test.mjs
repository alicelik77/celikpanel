// alpha.108 (D-038 §13): before a removal the dialog names what the server's
// own traces show using the component outside CelikPanel — the owner's
// web-server line, a service of theirs, a connected program — as a warning
// with who acts; with nothing found it says D-038's "Nothing CelikPanel knows
// of"; an unread source is said. Mounted, in English and Turkish.
// Kaldırmadan önce sistemin kendi izleri, bağlanmış olarak.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';

globalThis.cardWords = enComponentsScreens;
const stub = dataModule(`
  import React from '${reactURL}';
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => (globalThis.cardWords?.[key] !== undefined ? fill(globalThis.cardWords[key], vars) : key), locale: 'en' });
  export const useNavigate = () => () => true;
  export const AlertTriangle = () => null, Loader2 = () => null, X = () => null;
`);
const shared = sharedLayer(stub);
const { RemovalTracesNote, decodeRemovalTraces } = await import(shared.compile('components/RemovalTraces.tsx'));

const placeholders = (text) => [...text.matchAll(/\{([a-z_]+)\}/g)].map((m) => m[1]).sort().join(',');
const textOf = (tree) => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };

async function render(traces, words = enComponentsScreens) {
  globalThis.cardWords = words;
  let tree;
  await act(async () => { tree = Renderer.create(React.createElement(RemovalTracesNote, { traces, name: 'PHP-FPM 8.4' })); });
  const out = { text: textOf(tree), state: tree.root.findAll((n) => n.props?.['data-removal-traces'] !== undefined)[0]?.props['data-removal-traces'] };
  await act(async () => tree.unmount());
  return out;
}

test('the owner’s own uses are named with file and line, unit, process — and who acts', async () => {
  const traces = decodeRemovalTraces({
    traces: [
      { kind: 'web_config', path: '/etc/nginx/sites-enabled/legacy.conf', line: 5, text: 'fastcgi_pass unix:/run/php/php8.4-fpm.sock;', endpoint: '/run/php/php8.4-fpm.sock' },
      { kind: 'unit', unit: 'billing.service', relation: 'Requires', path: '/etc/systemd/system/billing.service' },
      { kind: 'process', process: 'legacy-worker', pid: 900, endpoint: '/run/php/php8.4-fpm.sock' },
      { kind: 'bogus' },
    ],
    traces_state: 'checked',
  });
  assert.equal(traces.traces.length, 3, 'an unknown kind is not shown');
  const en = await render(traces);
  assert.equal(en.state, 'found');
  assert.match(en.text, /\/etc\/nginx\/sites-enabled\/legacy\.conf, line 5: the web server hands requests to \/run\/php\/php8\.4-fpm\.sock/);
  assert.match(en.text, /fastcgi_pass unix:\/run\/php\/php8\.4-fpm\.sock;/);
  assert.match(en.text, /The service billing\.service depends on it \(Requires\)/);
  assert.match(en.text, /legacy-worker \(process 900\) is connected to it now/);
  assert.match(en.text, /The server owner checks each one first/);
  const tr = await render(traces, trComponentsScreens);
  assert.match(tr.text, /legacy\.conf, satır 5/);
  assert.match(tr.text, /Sunucu sahibi önce her birine bakar/);
});

test('nothing found says D-038’s words; an unread source is said, never taken as nothing', async () => {
  const none = await render(decodeRemovalTraces({ traces: [], traces_state: 'checked' }));
  assert.equal(none.state, 'none');
  assert.match(none.text, /^Nothing CelikPanel knows of uses PHP-FPM 8\.4/);
  const partial = await render(decodeRemovalTraces({ traces: [], traces_state: 'partial', traces_unread: ['connections'] }));
  assert.match(partial.text, /could not be read \(open connections\)/);
  const notLooked = await render(decodeRemovalTraces({}));
  assert.equal(notLooked.state, undefined, 'an older Agent that did not look says nothing');
});

test('every new sentence exists in English and Turkish with the same placeholders, and both dialogs show it', () => {
  const keys = Object.keys(enComponentsScreens).filter((key) => key.startsWith('services.removal.traces.'));
  assert.ok(keys.length >= 10);
  for (const key of keys) {
    assert.ok(trComponentsScreens[key], key);
    assert.equal(placeholders(trComponentsScreens[key]), placeholders(enComponentsScreens[key]), key);
  }
  const list = readFileSync(new URL('../src/components/ServiceList.tsx', import.meta.url), 'utf8');
  assert.match(list, /<RemovalTracesNote traces=\{simulation\?\.traces\} name=\{name\} \/>/);
  assert.match(list, /<RemovalTracesNote traces=\{sim\?\.traces\} name=\{name\} \/>/);
  assert.match(list, /traces: decodeRemovalTraces\(v\),/);
});
