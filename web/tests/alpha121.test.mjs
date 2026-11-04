// alpha.121: what the owner hit after updating Boston and Frankfurt.
// B: the install check that could not answer says why, who acts and what
// next (D-024). D: the server's own host name, when public DNS does not
// answer for it, with a reviewed one-press record where this panel serves the
// zone. E: a busy refusal held by the start-up work after an update says so.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, installPreviewLibURL, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';
import { enHostNameScreens } from '../src/i18n/screens/hostName/en.ts';
import { trHostNameScreens } from '../src/i18n/screens/hostName/tr.ts';

const words = {
  en: { ...enShell, ...enComponentsScreens, ...enHostNameScreens },
  tr: { ...trShell, ...trComponentsScreens, ...trHostNameScreens },
};
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
const sayIn = (locale) => (key, vars) => (words[locale][key] !== undefined ? fill(words[locale][key], vars) : key);
globalThis.a121Locale = 'en';
globalThis.a121Say = sayIn;
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Loader2 = () => null, RotateCw = () => null, X = () => null;
  export const CheckCircle2 = () => null, Info = () => null, XCircle = () => null, Clock = () => null, CircleDashed = () => null;
  export const useI18n = () => ({ t: globalThis.a121Say(globalThis.a121Locale), locale: globalThis.a121Locale });
  export const useNavigate = () => () => true;
  export const Link = ({ to, children, ...rest }) => React.createElement('a', { href: to, ...rest }, children);
`);
const serverHostnameURL = compileSource('lib/serverHostname.ts', () => { throw new Error('no imports'); });
const shared = sharedLayer(stub, { '/lib/serverHostname': serverHostnameURL });
const preview = await import(installPreviewLibURL);
const hostDNS = await import(serverHostnameURL);
const { InstallPreviewNotes } = await import(shared.compile('components/InstallPreviewNotes.tsx'));
const { ServerHostnameNotice } = await import(shared.compile('components/ServerHostnameNotice.tsx'));
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const text = (node) => (node === null ? '' : typeof node === 'string' ? node : Array.isArray(node) ? node.map(text).join('') : (node.children || []).map(text).join(''));

const unknown = (extra) => ({
  remote: { state: 'known', value: preview.decodeInstallPreview({ state: 'unknown', installs: [], notable: [], refusals: [], ...extra }), observedAt: 0 },
  reading: false,
  retry() {},
});

function renderNotes(locale, handle) {
  globalThis.a121Locale = locale;
  let tree;
  act(() => {
    tree = Renderer.create(React.createElement(InstallPreviewNotes, { handle, acknowledged: false, onAcknowledge() {}, componentLabel: (id) => id, avoidable: false }));
  });
  return tree;
}

test('B: the reason is decoded strictly: known codes, package names and a repository id only', () => {
  const value = preview.decodeInstallPreview({
    state: 'unknown', installs: [], notable: [], refusals: [],
    reason_code: 'unmet_dependencies', reason_packages: ['postgresql-common', 'libpq5', 'rm -rf /'], reason_repo: 'pgdg', lists_refreshed: 'pgdg',
  });
  assert.deepEqual(value.reason, { code: 'unmet_dependencies', packages: ['postgresql-common', 'libpq5'], repo: 'pgdg', refreshed: 'pgdg' });
  assert.equal(preview.decodeInstallPreview({ state: 'unknown', installs: [], notable: [], refusals: [], reason_code: 'whatever' }).reason.code, 'other');
  assert.equal(preview.decodeInstallPreview({ state: 'unknown', installs: [], notable: [], refusals: [] }).reason, undefined);
  assert.deepEqual(preview.installPreviewReasonKeys({ code: 'package_not_found', packages: [], repo: 'pgdg' }), {
    reason: 'services.installPreview.reason.package_not_found', next: 'services.installPreview.next.package_not_found_repo',
  });
});

test('B: "could not be checked" says the reason and who acts before the general sentence, in English and Turkish', () => {
  const handle = unknown({ reason_code: 'unmet_dependencies', reason_packages: ['postgresql-common', 'libpq5'] });
  let body = text(renderNotes('en', handle).toJSON());
  assert.ok(body.startsWith('The package manager cannot meet the dependencies this install needs: postgresql-common, libpq5.'), body);
  assert.ok(body.indexOf('Who acts: the server’s administrator.') < body.indexOf('could not be checked'), body);
  body = text(renderNotes('tr', handle).toJSON());
  assert.ok(body.startsWith('Paket yöneticisi bu kurulumun gerektirdiği bağımlılıkları karşılayamıyor: postgresql-common, libpq5.'), body);
  assert.ok(body.includes('Yapacak olan: sunucu yöneticisi.'), body);
  // The repository's lists were refreshed and the package is still missing: its card is named.
  body = text(renderNotes('en', unknown({ reason_code: 'package_not_found', reason_packages: ['postgresql-18'], reason_repo: 'pgdg' })).toJSON());
  assert.ok(body.includes('do not offer: postgresql-18.'), body);
  assert.ok(body.includes('CelikPanel refreshed the PostgreSQL (PGDG) repository’s package lists'), body);
  // An older server without a reason keeps the old sentence alone.
  body = text(renderNotes('en', unknown({})).toJSON());
  assert.ok(body.startsWith('What the package manager would install could not be checked.'), body);
});

test('B, D, E: every new sentence exists in English and Turkish', () => {
  const codes = ['unmet_dependencies', 'package_not_found', 'lists_refresh_failed', 'package_manager_busy', 'timed_out', 'agent_unreachable', 'invalid_request', 'other'];
  const keys = [
    ...codes.map((code) => `services.installPreview.reason.${code}`),
    ...codes.map((code) => `services.installPreview.next.${code}`),
    'services.installPreview.next.package_not_found_repo',
    ...[...source('components/ServerHostnameNotice.tsx').matchAll(/'(hostDNS\.[A-Za-z]+)'/g)].map((match) => match[1]),
    'err.service_operation_busy.agent_startup',
  ];
  assert.ok(keys.length > 30, String(keys.length));
  for (const key of keys) {
    assert.ok(words.en[key], `English ${key}`);
    assert.ok(words.tr[key], `Turkish ${key}`);
  }
});

test('E: a busy refusal held by the start-up work is said, not taken for this request', () => {
  const operation = source('components/ComponentOperation.tsx');
  assert.match(operation, /error\.code === ERROR_CODE_OPERATION_BUSY && error\.reason !== 'agent_startup'/);
  assert.match(words.en['err.service_operation_busy.agent_startup'], /Nothing needs to be done/);
});

const missingAnswer = {
  hostname: 'frankfurt.celikhost.com', state: 'missing', addresses: [], ipv4: '192.0.2.10', ipv6: '',
  zone: {
    name: 'celikhost.com', kind: 'infrastructure', records_path: '/api/v1/dns/infrastructure-zones/celikhost.com/records', editor_path: '/domains/infrastructure/celikhost.com',
    missing: [{ name: 'frankfurt.celikhost.com', type: 'A', content: '192.0.2.10', ttl: 3600 }], present: [],
  },
};

test('D: the host name answer is decoded strictly; only the panel’s own record endpoints are followed', () => {
  const value = hostDNS.decodeServerHostnameDNS(missingAnswer);
  assert.equal(value.zone.recordsURL, '/api/v1/dns/infrastructure-zones/celikhost.com/records');
  assert.equal(hostDNS.serverHostnameRecordText(value.zone.missing[0]), 'frankfurt.celikhost.com. 3600 IN A 192.0.2.10');
  assert.equal(hostDNS.decodeServerHostnameDNS({ ...missingAnswer, zone: { ...missingAnswer.zone, records_path: 'https://evil.example/records' } }).zone, null);
  assert.throws(() => hostDNS.decodeServerHostnameDNS({ hostname: 'x', state: 'maybe' }));
  assert.equal(hostDNS.hostnameDomain('frankfurt.celikhost.com'), 'celikhost.com');
});

async function settle() {
  for (let i = 0; i < 6; i += 1) await Promise.resolve();
}

test('D: a missing host name in a zone this panel serves offers the reviewed record; it is added once, then read again', async () => {
  const originalFetch = globalThis.fetch;
  const calls = [];
  let answer = missingAnswer;
  globalThis.fetch = async (input, init = {}) => {
    calls.push([init.method ?? 'GET', String(input), init.body ?? '']);
    if (init.method === 'POST') return new Response(JSON.stringify({ success: true }), { status: 200, headers: { 'Content-Type': 'application/json' } });
    return new Response(JSON.stringify(answer), { status: 200, headers: { 'Content-Type': 'application/json' } });
  };
  globalThis.a121Locale = 'en';
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  let tree;
  try {
    await act(async () => { tree = Renderer.create(React.createElement(ServerHostnameNotice)); await settle(); });
    const body = text(tree.toJSON());
    assert.ok(body.startsWith('This server’s host name frankfurt.celikhost.com does not resolve in public DNS'), body);
    assert.ok(body.includes('This panel serves the celikhost.com zone. Who acts: you.'), body);
    // Reading never changed anything.
    assert.deepEqual(calls.map(([method]) => method), ['GET']);
    const add = tree.root.findAll((node) => node.props['data-server-hostname-add'] === '')[0];
    await act(async () => { add.props.onClick(); await settle(); });
    const confirm = tree.root.findAll((node) => node.props['data-server-hostname-confirm'] === '')[0];
    assert.ok(text(tree.toJSON()).includes('frankfurt.celikhost.com. 3600 IN A 192.0.2.10'), 'the exact record is shown for review');
    answer = { ...missingAnswer, state: 'resolves', zone: null };
    await act(async () => { confirm.props.onClick(); await settle(); await settle(); });
    const posts = calls.filter(([method]) => method === 'POST');
    assert.equal(posts.length, 1);
    assert.equal(posts[0][1], '/api/v1/dns/infrastructure-zones/celikhost.com/records');
    assert.deepEqual(JSON.parse(posts[0][2]), { name: 'frankfurt.celikhost.com.', type: 'A', content: '192.0.2.10', ttl: 3600, prio: 0 });
    // One read after the press, never a loop.
    assert.equal(calls.filter(([method, url]) => method === 'GET' && url.includes('recheck=1')).length, 1);
    assert.ok(text(tree.toJSON()).includes('The record was added to celikhost.com and published.'));
  } finally {
    if (tree) act(() => tree.unmount());
    globalThis.fetch = originalFetch;
    delete globalThis.document;
  }
});

test('D: a name that resolves, or a read that failed, shows nothing', async () => {
  const originalFetch = globalThis.fetch;
  for (const reply of [
    () => new Response(JSON.stringify({ ...missingAnswer, state: 'resolves', zone: null }), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    () => new Response('{}', { status: 500, headers: { 'Content-Type': 'application/json' } }),
  ]) {
    globalThis.fetch = async () => reply();
    let tree;
    try {
      await act(async () => { tree = Renderer.create(React.createElement(ServerHostnameNotice, { key: Math.random() })); await settle(); });
      assert.equal(tree.toJSON(), null);
    } finally {
      if (tree) act(() => tree.unmount());
      globalThis.fetch = originalFetch;
    }
  }
});
