// alpha.147 lab (24 Nov 2026): two screen texts were no longer true after the
// side doors step. The component pages of webmail, phpMyAdmin and phpPgAdmin
// said the three tools share the web server's PHP-FPM pool (false where they
// were moved; the page has no tool state, so the sentence covers both cases),
// and stock_pool_in_use said the pool is turned off after the next start
// (false for a pool the owner put back: it stays on until the administrator
// presses that line's own button, whose label the sentence must name).
//
// alpha.147 laboratuvarı: adımdan sonra doğru olmayan iki ekran metni.
import assert from 'node:assert/strict';
import test from 'node:test';
import { enLifecycleScreens } from '../src/i18n/screens/lifecycle/en.ts';
import { trLifecycleScreens } from '../src/i18n/screens/lifecycle/tr.ts';
import { enSideDoorsScreens } from '../src/i18n/screens/sideDoors/en.ts';
import { trSideDoorsScreens } from '../src/i18n/screens/sideDoors/tr.ts';
import { enSideDoorsUndoScreens } from '../src/i18n/screens/sideDoorsUndo/en.ts';
import { trSideDoorsUndoScreens } from '../src/i18n/screens/sideDoorsUndo/tr.ts';

test('the tools page says both the moved and the not moved case', () => {
  const en = enLifecycleScreens['life.isolation.tools'];
  assert.match(en, /account and PHP-FPM pool of their own/);
  assert.match(en, /not moved shares one PHP-FPM pool under the web server’s user/);
  const tr = trLifecycleScreens['life.isolation.tools'];
  assert.match(tr, /kendi hesabı ve kendi PHP-FPM havuzuyla/);
  assert.match(tr, /taşınmamış bir araç web sunucusunun kullanıcısıyla tek bir PHP-FPM havuzunu paylaşır/);
});

test('a pool still in use says a pool the owner put back waits for its own button', () => {
  for (const [words, undo] of [[enSideDoorsScreens, enSideDoorsUndoScreens], [trSideDoorsScreens, trSideDoorsUndoScreens]]) {
    const body = words['pnow.stock_pool_in_use.body'];
    const button = undo['pnow.stock_pool_owner_restored.action'];
    assert.ok(body.includes(button), `the in-use line names the button "${button}"`);
  }
});
