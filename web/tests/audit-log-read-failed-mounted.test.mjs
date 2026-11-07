import assert from 'node:assert/strict';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { screenParts, screenPartExport } from './locale-catalogue.mjs';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';

// alpha.119 (D-024): the Audit log whose read failed says it does not know,
// with a retry. It never says "No activity recorded yet" for a read that did
// not answer; only a real empty answer says that.
// Denetim izi okunamadiysa "bilinmiyor" der ve yeniden dene sunar; bos liste
// demez.
const catalogue = { en: { ...enShell }, tr: { ...trShell } };
for (const part of screenParts) {
  for (const locale of ['en', 'tr']) {
    const module = await import(`../src/i18n/screens/${part}/${locale}.ts`);
    Object.assign(catalogue[locale], module[screenPartExport(part, locale)]);
  }
}

const stub = dataModule(`
  import React from '${reactURL}';
  export const ScrollText = () => null, AlertTriangle = () => null;
  export const useI18n = () => globalThis.currentTest.i18n;
  export const useNavigate = () => () => true;
  export const useAuth = () => ({ role: 'admin' });
  export const PageHeader = (props) => React.createElement('header', null, props.title);
  export const auditActionParts = () => ({ target: '' });
  export const auditActionText = (action) => action;
  export const auditActorName = (name) => name;
  export const auditDetailParts = () => [];
  export const auditResourceText = () => '';
`);
const shared = sharedLayer(stub);
const { AuditLogPage } = await import(shared.compile('components/AuditLogPage.tsx'));

const settled = () => new Promise((resolve) => setImmediate(resolve));
const flush = async () => { for (let i = 0; i < 4; i += 1) await settled(); };
const originalFetch = globalThis.fetch;
let tree;
let reads;

async function mount(locale, answers) {
  const t = (key) => (key in catalogue[locale] ? catalogue[locale][key] : key);
  globalThis.currentTest = { i18n: { t, locale, screensReady: true, screensFailed: false } };
  globalThis.window = globalThis;
  reads = 0;
  globalThis.fetch = () => {
    const answer = answers[Math.min(reads, answers.length - 1)];
    reads += 1;
    return Promise.resolve().then(answer);
  };
  await act(async () => {
    tree = Renderer.create(React.createElement(AuditLogPage));
    await flush();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.currentTest;
}
const text = () => JSON.stringify(tree.toJSON());
const retryButton = () => tree.root.findAll((node) => node.type === 'button');
const row = { id: 1, username: 'admin', action: 'login', created_at: '2026-11-03 10:00:00' };

for (const locale of ['en', 'tr']) {
  for (const [name, failure] of [
    ['a refused read', () => Response.json({ error: 'x' }, { status: 500 })],
    ['a dropped read', () => { throw new TypeError('network'); }],
    ['an answer that is not the contract', () => Response.json({ rows: [] })],
  ]) {
    test(`Audit log (${locale}): ${name} is not known, never "no activity"; retry reads again and shows the rows`, async () => {
      try {
        await mount(locale, [failure, () => Response.json({ entries: [row] })]);
        assert.ok(text().includes(catalogue[locale]['settings.audit.unknown']), 'the unknown sentence is missing');
        assert.ok(!text().includes(catalogue[locale]['audit.empty']), 'a failed read says there is no activity');
        assert.ok(retryButton().length > 0, 'no retry is offered');
        await act(async () => { retryButton()[0].props.onClick({}); await flush(); });
        assert.equal(reads, 2, 'the retry did not read once more');
        assert.ok(!text().includes(catalogue[locale]['settings.audit.unknown']));
        assert.ok(text().includes('admin'));
      } finally { await cleanup(); }
    });
  }
}

test('Audit log: a real empty answer (and the server\'s null list) still says no activity', async () => {
  try {
    await mount('en', [() => Response.json({ entries: [] })]);
    assert.ok(text().includes(catalogue.en['audit.empty']));
    assert.ok(!text().includes(catalogue.en['settings.audit.unknown']));
  } finally { await cleanup(); }
  try {
    await mount('en', [() => Response.json({ entries: null })]);
    assert.ok(text().includes(catalogue.en['audit.empty']));
  } finally { await cleanup(); }
});
