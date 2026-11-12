// alpha.125 (a124 lab, 5 Nov 2026): two refusals that answered 500 now have
// their own words. Creating a mailbox that already exists says so and what to
// do; changing PHP on a site whose hosting type runs no PHP (the static main
// site) says the site has no PHP and where its type is changed. EN + TR.
// Var olan posta kutusu ve PHP'si olmayan site: 500 yerine kendi cümleleri.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

import { apiErrorText, apiErrorActionLabel, readApiError } from '../src/lib/apiError.ts';
import { enMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/en.ts';
import { trMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/tr.ts';
import { enSitesScreens } from '../src/i18n/screens/sites/en.ts';
import { trSitesScreens } from '../src/i18n/screens/sites/tr.ts';

const words = {
  en: { ...enMailDatabasesScreens, ...enSitesScreens },
  tr: { ...trMailDatabasesScreens, ...trSitesScreens },
};
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
const sayIn = (locale) => (key, vars) => (words[locale][key] !== undefined ? fill(words[locale][key], vars) : key);
const answer = (body, status = 409) => new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

test('a mailbox that already exists is named, nothing changed, with the next step', async () => {
  const problem = await readApiError(answer({
    code: 'MAIL_ACCOUNT_EXISTS', vars: { address: 'info@example.com' },
    error: 'the mailbox info@example.com already exists, so nothing was changed.',
  }));
  const en = apiErrorText(problem, sayIn('en'), 'mail.createFailed');
  const tr = apiErrorText(problem, sayIn('tr'), 'mail.createFailed');
  assert.match(en, /^The mailbox info@example\.com already exists, so nothing was changed\. Use the existing mailbox/);
  assert.match(en, /choose another name/);
  assert.match(tr, /^info@example\.com posta kutusu zaten var; bu yüzden hiçbir şey değiştirilmedi\./);
  assert.match(tr, /başka bir ad seçin/);
});

test('a static site has no PHP version: its own sentence and the way to its site type', async () => {
  const body = {
    code: 'SITE_HAS_NO_PHP', reason: 'static', action: '/domains/example.com?tab=hosting',
    vars: { domain: 'example.com' }, error: 'example.com is a static site, so it has no PHP version and nothing was changed.',
  };
  const problem = await readApiError(answer(body));
  assert.match(apiErrorText(problem, sayIn('en')), /^example\.com is a static site: .*has no PHP.*Nothing was changed\..*Site section and change its site type to PHP/);
  assert.match(apiErrorText(problem, sayIn('tr')), /^example\.com statik bir site: .*Hiçbir şey değiştirilmedi\..*Site bölümünü açıp site türünü PHP yapın/);
  assert.equal(apiErrorActionLabel(problem, sayIn('en')), 'Open Site');
  assert.equal(apiErrorActionLabel(problem, sayIn('tr')), 'Site bölümünü aç');
  // Another hosting type without PHP keeps the general sentence.
  const node = await readApiError(answer({ ...body, reason: 'node' }));
  assert.match(apiErrorText(node, sayIn('en')), /^example\.com does not run PHP \(its hosting type is not PHP\)/);
  assert.match(apiErrorText(node, sayIn('tr')), /^example\.com PHP çalıştırmıyor/);
});

test('the mailbox form reads a refused answer as a coded refusal before anything else', () => {
  const source = readFileSync(new URL('../src/components/DomainMailManager.tsx', import.meta.url), 'utf8');
  const create = source.slice(source.indexOf('const createAccount = async'), source.indexOf('const saveQuota = async'));
  assert.match(create, /if \(!res\.ok\) \{ refuse\(await readApiError\(res\), 'mail\.createFailed'\); return; \}\s*const data = await res\.json\(\);/);
});

test('every new sentence exists in both languages with the same placeholders', () => {
  const pairs = [
    [enMailDatabasesScreens, trMailDatabasesScreens, ['err.MAIL_ACCOUNT_EXISTS']],
    [enSitesScreens, trSitesScreens, ['err.SITE_HAS_NO_PHP', 'err.SITE_HAS_NO_PHP.static', 'err.SITE_HAS_NO_PHP.action']],
  ];
  for (const [en, tr, keys] of pairs) {
    for (const key of keys) {
      assert.ok(en[key] && tr[key], key);
      const holes = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join();
      assert.equal(holes(en[key]), holes(tr[key]), key);
    }
  }
});

test('the PHP pool form shows a coded refusal (a site without PHP) in its own words', () => {
  const source = readFileSync(new URL('../src/components/DomainPHPSettings.tsx', import.meta.url), 'utf8');
  const save = source.slice(source.indexOf('/php/pool`'));
  assert.match(save, /if \(!res\.ok\) \{\s*showToast\('error', apiErrorText\(await readApiError\(res\), t, 'php\.poolFailed'\)\);/);
});
