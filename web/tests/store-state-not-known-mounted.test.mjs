import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

// The Store's badge when the server has no usable record of this server's
// components (browser batch 14, 2026-10-15). The Panel marks such an offering
// "unsupported" with blocker_reason component_state_unavailable, _stale or
// _invalid, and its sentence says that CelikPanel checks the components by
// itself; the badge beside it said "Unsupported here" / "Bu sunucuda
// desteklenmiyor", a claim about a state that is not known (D-024). The real
// StoreCatalog is mounted with the server's answers.
//
// Magaza: bilesen kaydi yok, eski ya da okunamiyorsa rozet "desteklenmiyor"
// demez, "durum henuz bilinmiyor" der; gercek desteklenmeyen urun ayni kalir.
const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataModule = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const compile = (text) => ts.transpileModule(text, { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;

const source = read('../src/components/StoreCatalog.tsx');
const icons = /import \{([^}]+)\} from 'lucide-react'/.exec(source)[1].split(',').map((item) => item.trim()).filter((item) => item && !item.startsWith('type '));
const stub = dataModule(`import React from '${reactURL}';
 export const useI18n = () => ({ t: (key, vars = {}) => {
   let text = globalThis.storeCatalog[key] ?? key;
   for (const [name, value] of Object.entries(vars)) text = text.replaceAll('{' + name + '}', String(value));
   return text;
 } });
 export const Link = (props) => React.createElement('a', { href: props.to }, props.children);
 export const Button = (props) => React.createElement('button', { disabled: props.disabled }, props.children);
 export const Card = (props) => React.createElement('section', null, props.children);
 export const inputClass = '';
 ${icons.map((name) => `export const ${name} = () => React.createElement('icon-${name}');`).join('\n')}
`);
const catalogURL = dataModule(`import React from '${reactURL}';
` + compile(source).replace(/from ['"]([^'"]+)['"]/g, (_, name) => `from '${name === 'react' ? reactURL : stub}'`));
const { StoreCatalog, componentStateNotKnown } = await import(catalogURL);

const item = (id, blockerReason, state = 'unsupported') => ({
  id, name: id, description: `${id} for sites`, category: 'applications', vendor: 'CelikPanel', icon: '', tags: [],
  entitlementState: 'included', state, stateReason: `${blockerReason} sentence`, blockerReason, action: 'open_components', actionPath: '/services',
});
const items = [
  item('cache', 'component_state_unavailable'),
  item('webmail', 'component_state_stale'),
  item('scanner', 'component_state_invalid'),
  item('legacy', 'platform_unsupported'),
];

async function badges(catalog) {
  globalThis.storeCatalog = catalog;
  let tree;
  await act(async () => {
    tree = Renderer.create(React.createElement(StoreCatalog, { items, subscription: { id: 1, name: 'example.com', owner: 'customer1' }, busyItemIDs: new Set(), onAction: () => {} }));
  });
  const walk = (node) => (typeof node === 'string' ? node : (node.children || []).map(walk).join(''));
  const cards = tree.root.findAll((node) => node.type === 'article');
  const said = Object.fromEntries(cards.map((card) => {
    const name = walk(card.find((node) => node.type === 'h3'));
    // The state badge is the first pill of the card; the entitlement pill follows.
    const badge = card.findAll((node) => node.type === 'span' && String(node.props.className || '').includes('rounded-full'))[0];
    return [name, walk(badge)];
  }));
  await act(async () => tree.unmount());
  return said;
}

test('Store: an offering whose component state is not known is not called unsupported; a real one still is', async () => {
  for (const [locale, catalog] of [['en', { ...en, ...enServerScreens }], ['tr', { ...tr, ...trServerScreens }]]) {
    const said = await badges(catalog);
    for (const name of ['cache', 'webmail', 'scanner']) {
      assert.equal(said[name], catalog['addons.state.notKnownYet'], `${locale} ${name}: ${said[name]}`);
      assert.notEqual(said[name], catalog['addons.state.unsupported']);
    }
    assert.equal(said.legacy, catalog['addons.state.unsupported'], `${locale}: a real unsupported offering lost its badge`);
  }
  assert.notEqual(en['addons.state.notKnownYet'] ?? enServerScreens['addons.state.notKnownYet'], trServerScreens['addons.state.notKnownYet']);
});

test('Store: only the typed component_state_* reasons on an unsupported offering are "not known"', () => {
  assert.equal(componentStateNotKnown({ state: 'unsupported', blockerReason: 'component_state_unknown' }), true);
  assert.equal(componentStateNotKnown({ state: 'unsupported', blockerReason: 'platform_unsupported' }), false);
  assert.equal(componentStateNotKnown({ state: 'unsupported' }), false);
  assert.equal(componentStateNotKnown({ state: 'available', blockerReason: 'component_state_stale' }), false);
});

test('Store: the page passes the server\'s typed reason to the catalogue', () => {
  assert.match(read('../src/components/AddonsPage.tsx'), /blockerReason: value\.blocker_reason\?\.trim\(\) \|\| undefined/);
});
