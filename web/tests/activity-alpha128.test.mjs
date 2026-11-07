// alpha.128 G23: the account's own action log. Mounted with the real shared
// reader and the real copy: who did it is said by role or name as the server
// sent it, the address appears only where the server sent one, "Show older"
// reads the next page and nothing else is sent.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('Activity: under /audit for every role but the administrator, linked from My account and the team member dashboard; read only', () => {
  // The boot path does not grow: no new route, the Audit log address serves it.
  assert.doesNotMatch(read('../src/App.tsx'), /ActivityPage/);
  const audit = read('../src/components/AuditLogPage.tsx');
  assert.match(audit, /const ActivityPage = lazy\(\(\) => import\('\.\/ActivityPage'\)/);
  assert.match(audit, /if \(role !== 'admin'\) return <Suspense fallback=\{null\}><ActivityPage \/><\/Suspense>;/);
  assert.match(read('../src/components/AccountPage.tsx'), /<ActivityCard \/>/);
  assert.match(read('../src/components/Dashboard.tsx'), /<ActivityCard team \/>/);
  const page = read('../src/components/ActivityPage.tsx');
  assert.doesNotMatch(page, /method:\s*'(POST|PUT|DELETE)'/);
  assert.doesNotMatch(page, /audit-logs/);
});

globalThis.a128ActivityRole = 'customer';
const stub = dataModule(`
  import React from '${reactURL}';
  export const History = () => null, AlertTriangle = () => null;
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useAuth = () => ({ role: globalThis.a128ActivityRole });
  export const useNavigate = () => () => true;
  export const Link = ({ to, children, ...rest }) => React.createElement('a', { href: to, ...rest }, children);
  export const PageHeader = ({ title, subtitle }) => React.createElement('header', null, title, ' ', subtitle);
`);
const enURL = compileSource('i18n/domainOps/en.ts', () => stub);
const trURL = compileSource('i18n/domainOps/tr.ts', () => stub);
const wordsURL = compileSource('i18n/domainOps.ts', (specifier) => (
  specifier.endsWith('/en') ? enURL : specifier.endsWith('/tr') ? trURL : stub
));
const auditURL = dataModule(`
  export const auditActionParts = (action) => ({ name: action.split(':')[0], target: action.split(':')[1] || '' });
  export const auditActionText = (action) => 'did ' + action;
  export const auditResourceText = (type, id) => (type ? type + ' #' + id : '');
`);
const shared = sharedLayer(stub, { '/i18n/domainOps': wordsURL, '/lib/auditAction': auditURL });
const { ActivityPage, decodeActivity } = await import(shared.compile('components/ActivityPage.tsx'));

const originalFetch = globalThis.fetch;
let tree, calls;
function serve(table) {
  calls = [];
  globalThis.fetch = (url, options = {}) => {
    calls.push({ method: options.method || 'GET', url });
    const answer = table[url];
    if (!answer) return Promise.resolve(Response.json({ error: 'no such route' }, { status: 404 }));
    return Promise.resolve().then(() => answer());
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount() {
  await act(async () => {
    tree = Renderer.create(React.createElement(ActivityPage));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
}
const flat = (node) => (node === null ? '' : typeof node === 'string' ? node : Array.isArray(node) ? node.map(flat).join('') : (node.children || []).map(flat).join(''));
const text = () => flat(tree.toJSON());

test('Activity: who did it, what, and the address only on one’s own rows; older pages on request', async () => {
  serve({
    '/api/v1/me/activity': () => Response.json({
      entries: [
        { id: 9, at: '2026-11-07 10:00:00', action: 'domain.create:a.example', actor: { kind: 'you' }, ip: '192.0.2.7' },
        { id: 8, at: '2026-11-07 09:00:00', action: 'backup.create:files', actor: { kind: 'administrator' }, resource_type: 'domain', resource_id: 3 },
        { id: 7, at: '2026-11-07 08:00:00', action: '', actor: { kind: 'team_member', name: 'ayse' } },
      ],
      next_before: 7,
      incomplete: true,
    }),
    '/api/v1/me/activity?before=7': () => Response.json({
      entries: [{ id: 3, at: '2026-11-06 08:00:00', action: 'auth.login', actor: { kind: 'provider' }, as: 'shop' }],
    }),
  });
  try {
    await mount();
    let body = text();
    assert.ok(body.includes('did domain.create:a.example') && body.includes('192.0.2.7'), body);
    assert.ok(body.includes('The administrator') && body.includes('ayse (team member)'), body);
    assert.ok(body.includes('An action in the panel'), 'a row without a plain action gets the safe generic line');
    assert.ok(body.includes('Some recent actions are still being saved'), body);
    assert.equal((body.match(/192\.0\.2\./g) || []).length, 1);
    const button = tree.root.findAll((node) => node.type === 'button').find((node) => JSON.stringify(node.props.children).includes('Show older'));
    assert.ok(button, 'Show older is offered');
    await act(async () => { await button.props.onClick({}); await settled(); });
    body = text();
    assert.ok(body.includes('Your provider, as shop'), body);
    assert.ok(body.includes('This is the beginning of the record.'), body);
    assert.deepEqual(calls.map((call) => `${call.method} ${call.url}`), ['GET /api/v1/me/activity', 'GET /api/v1/me/activity?before=7']);
  } finally {
    await cleanup();
  }
});

test('Activity: a read that fails is not an empty log; an unknown shape is not known', async () => {
  assert.throws(() => decodeActivity({ entries: [{ id: 1, action: 'x', actor: { kind: 'boss' } }] }));
  serve({ '/api/v1/me/activity': () => Response.json({ error: 'x' }, { status: 500 }) });
  try {
    await mount();
    assert.ok(text().includes('This does not mean nothing was done.'), text());
    assert.ok(!text().includes('Nothing has been recorded yet.'));
  } finally {
    await cleanup();
  }
});
