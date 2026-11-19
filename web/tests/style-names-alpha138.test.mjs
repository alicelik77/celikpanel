// alpha.138 (owner, 20 Nov 2026: "stil isimlerini değiştir, firmalar ile
// sorun yaşamayalım. başka isimler ver."): the panel's four styles carry no
// other company's product name, as a name, an id, a selector or a comment.
// Celik stays; the other three are Sky / Gök, Forest / Orman and Amber /
// Kehribar, named from the text catalogue in both languages. A style chosen
// under its earlier id is kept: it is read as its new id and written back.
//
// alpha.138: dört stilin hiçbiri başka bir firmanın ürün adını taşımaz; eski
// kimlikle seçilen stil korunur.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource } from './fixtures/shared-layer.mjs';
import { en as english } from '../src/i18n/en.ts';
import { tr as turkish } from '../src/i18n/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const theme = await import(compileSource('theme/ThemeProvider.tsx', (specifier) => { throw new Error(`ThemeProvider imports ${specifier}`); }));

// A browser's storage, as the styles see it.
function storage({ value = null, throws = false } = {}) {
  const store = new Map(value === null ? [] : [['celikpanel.skin', value]]);
  const writes = [];
  globalThis.localStorage = {
    getItem: (key) => { if (throws) throw new Error('blocked'); return store.has(key) ? store.get(key) : null; },
    setItem: (key, next) => { if (throws) throw new Error('blocked'); writes.push([key, next]); store.set(key, next); },
  };
  return { store, writes };
}

test('the four styles: Celik, Sky, Forest, Amber, named from the catalogue in both languages', () => {
  assert.deepEqual(theme.SKINS.map((skin) => skin.id), ['celik', 'sky', 'forest', 'amber']);
  const names = (catalogue) => theme.SKINS.map((skin) => catalogue[skin.nameKey]);
  assert.deepEqual(names(english), ['Celik', 'Sky', 'Forest', 'Amber']);
  assert.deepEqual(names(turkish), ['Celik', 'Gök', 'Orman', 'Kehribar']);
  assert.match(read('../src/components/SkinSwitcher.tsx'), /\{t\(sk\.nameKey\)\}/);
});

test('a style chosen under its earlier id is kept: read as its new id and written back once', () => {
  for (const [earlier, renamed] of [['plesk', 'sky'], ['aapanel', 'forest'], ['cpanel', 'amber']]) {
    const { store, writes } = storage({ value: earlier });
    assert.equal(theme.readStoredSkin(), renamed, earlier);
    assert.deepEqual(writes, [['celikpanel.skin', renamed]], `${earlier} was not written back as ${renamed}`);
    assert.equal(store.get('celikpanel.skin'), renamed);
    // The next read finds the new id and writes nothing.
    writes.length = 0;
    assert.equal(theme.readStoredSkin(), renamed);
    assert.deepEqual(writes, []);
  }
  for (const id of ['celik', 'sky', 'forest', 'amber']) {
    const { writes } = storage({ value: id });
    assert.equal(theme.readStoredSkin(), id);
    assert.deepEqual(writes, []);
  }
  storage({ value: 'something-else' });
  assert.equal(theme.readStoredSkin(), 'celik');
  storage();
  assert.equal(theme.readStoredSkin(), 'celik');
  storage({ value: 'plesk', throws: true });
  assert.equal(theme.readStoredSkin(), 'celik', 'blocked storage must not break the page');
  delete globalThis.localStorage;
});

test('no style carries another company\'s product name: not in the palettes, the picker, the list or the texts', () => {
  const names = /plesk|aapanel|cpanel/i;
  const css = read('../src/index.css');
  assert.doesNotMatch(css, names, 'index.css');
  for (const id of ['sky', 'forest', 'amber']) assert.match(css, new RegExp(`\\[data-theme='${id}'\\]`), id);
  assert.doesNotMatch(read('../src/components/SkinSwitcher.tsx'), names);
  // ThemeProvider names them only in the one map that reads an earlier stored choice.
  const provider = read('../src/theme/ThemeProvider.tsx').split('\n').filter((line) => names.test(line));
  assert.deepEqual(provider.map((line) => line.trim()), [`const EARLIER_SKIN_IDS: Readonly<Record<string, SkinId>> = { plesk: 'sky', aapanel: 'forest', cpanel: 'amber' };`]);
  for (const [key, value] of Object.entries({ ...english, ...turkish })) {
    if (key.startsWith('theme.')) assert.doesNotMatch(value, names, key);
  }
});
