// alpha.128 G22: the domain health strip. Mounted with the real shared reader
// and the real copy: problems come first in plain words with their one fix,
// what the panel does not know is never drawn as fine, "points here" is the
// Connection card's own answer, and only reads are sent.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enDomainOps } from '../src/i18n/domainOps/en.ts';
import { trDomainOps } from '../src/i18n/domainOps/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('Health strip copy: every English line has a Turkish one, and the other way round', () => {
  assert.deepEqual(Object.keys(trDomainOps).sort(), Object.keys(enDomainOps).sort());
  for (const [key, value] of Object.entries(trDomainOps)) assert.ok(value.trim(), key);
});

test('Health strip is lazy, outside the boot catalogue, and sends no change', () => {
  const detail = read('../src/components/DomainDetail.tsx');
  assert.match(detail, /const DomainHealthStrip = lazy\(\(\) => import\('\.\/DomainHealthStrip'\)/);
  const strip = read('../src/components/DomainHealthStrip.tsx');
  assert.doesNotMatch(strip, /method:\s*'(POST|PUT|DELETE)'/);
  assert.doesNotMatch(strip, /window\.(confirm|alert|prompt)/);
  assert.doesNotMatch(read('../src/i18n/en.ts'), /dhealth\./);
});

globalThis.a128Say = (key) => key;
const stub = dataModule(`
  import React from '${reactURL}';
  export const Activity = () => null, AlertTriangle = () => null;
  export const useI18n = () => ({ t: globalThis.a128Say, locale: 'en' });
  export const useNavigate = () => () => true;
  export const Link = ({ to, children, ...rest }) => React.createElement('a', { href: to, ...rest }, children);
`);
const enURL = compileSource('i18n/domainOps/en.ts', () => stub);
const trURL = compileSource('i18n/domainOps/tr.ts', () => stub);
const wordsURL = compileSource('i18n/domainOps.ts', (specifier) => (
  specifier.endsWith('/en') ? enURL : specifier.endsWith('/tr') ? trURL : stub
));
const connectionURL = dataModule(`export function decodeConnection(raw) { return raw; }`);
const shared = sharedLayer(stub, { '/i18n/domainOps': wordsURL, '/DomainConnection': connectionURL });
const { DomainHealthStrip, decodeDomainHealth } = await import(shared.compile('components/DomainHealthStrip.tsx'));

const originalFetch = globalThis.fetch;
let tree, calls;
function serve(table) {
  calls = [];
  globalThis.document = { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible' };
  globalThis.fetch = (url, options = {}) => {
    calls.push({ method: options.method || 'GET', url });
    const answer = table[url.split('?')[0]];
    if (!answer) return Promise.resolve(Response.json({ error: 'no such route' }, { status: 404 }));
    return Promise.resolve().then(() => answer());
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(props) {
  await act(async () => {
    tree = Renderer.create(React.createElement(DomainHealthStrip, props));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.document;
}
const flat = (node) => (node === null ? '' : typeof node === 'string' ? node : Array.isArray(node) ? node.map(flat).join('') : (node.children || []).map(flat).join(''));
const text = () => flat(tree.toJSON());
const byCheck = (id) => tree.root.find((node) => node.props && node.props['data-health-check'] === id);

const answer = (checks, contact = 'provider') => () => Response.json({ domain: 'a.example', contact, checks });

test('Health strip: problems first with their one fix; contact when the fix is someone else’s; DNS from the card', async () => {
  const went = [];
  serve({
    '/api/v1/domains/7/health': answer([
      { id: 'site', state: 'ok', code: 'up' },
      { id: 'certificate', state: 'problem', code: 'expiring', days: 9, fix: { kind: 'tab', tab: 'hosting', sub: 'ssl' } },
      { id: 'mail', state: 'problem', code: 'port25_blocked', fix: { kind: 'contact' } },
      { id: 'disk', state: 'unknown', code: 'not_measured' },
      { id: 'backup', state: 'ok', code: 'recent', days: 2 },
    ]),
    '/api/v1/domains/7/connection': () => Response.json({ status: 'elsewhere', server_ip: '192.0.2.1' }),
  });
  try {
    await mount({ domainId: 7, showsConnection: true, hasTab: () => true, onGo: (tab, sub) => went.push([tab, sub]) });
    const body = text();
    assert.ok(body.includes('The certificate ends in 9 days and has not been renewed yet.'), body);
    assert.ok(body.includes('Contact your provider to fix this.'), body);
    assert.ok(body.includes('The domain does not point to this server yet.'), body);
    assert.ok(body.includes('Disk use not measured yet.'), body);
    assert.ok(body.includes('Website answers') && body.includes('Last backup 2 days ago'), body);
    // Problems come before what is fine.
    assert.ok(body.indexOf('certificate ends') < body.indexOf('Website answers'), body);
    assert.equal(byCheck('disk').props['data-health-state'], 'unknown');
    const fix = tree.root.find((node) => node.type === 'button' && node.props['data-health-fix'] === 'tab');
    await act(async () => { fix.props.onClick(); });
    assert.deepEqual(went, [['hosting', 'ssl']]);
    assert.ok(calls.every((call) => call.method === 'GET'), JSON.stringify(calls));
    assert.equal(calls.filter((call) => call.url === '/api/v1/domains/7/connection').length, 1);
  } finally {
    await cleanup();
  }
});

test('Health strip: a tab the page does not draw names whom to contact; a page fix is a link', async () => {
  serve({
    '/api/v1/domains/7/health': answer([
      { id: 'backup', state: 'problem', code: 'none', fix: { kind: 'tab', tab: 'advanced', sub: 'backups' } },
      { id: 'site', state: 'problem', code: 'down', reason: 'timeout', fix: { kind: 'page', path: '/monitoring' } },
    ], 'administrator'),
  });
  try {
    await mount({ domainId: 7, showsConnection: false, hasTab: () => false, onGo: () => {} });
    const body = text();
    assert.ok(body.includes('No backup has been made.') && body.includes('Contact the administrator to fix this.'), body);
    assert.ok(body.includes('The website did not answer at the last check: no answer within 10 seconds.'), body);
    const link = tree.root.find((node) => node.type === 'a');
    assert.equal(link.props.href, '/monitoring');
    assert.ok(!body.includes('points'), 'no DNS line without the Connection card');
  } finally {
    await cleanup();
  }
});

test('Health strip: an answer it cannot read is not known, never fine', async () => {
  const decoded = decodeDomainHealth({ contact: 'owner', checks: [{ id: 'site', state: 'ok', code: 'a_new_code' }] });
  assert.equal(decoded.checks[0].state, 'unknown');
  assert.throws(() => decodeDomainHealth({ contact: 'someone', checks: [] }));
  serve({ '/api/v1/domains/7/health': () => Response.json({ error: 'x' }, { status: 500 }) });
  try {
    await mount({ domainId: 7, showsConnection: false, hasTab: () => true, onGo: () => {} });
    assert.ok(text().includes('This does not mean it is fine.'), text());
  } finally {
    await cleanup();
  }
});
