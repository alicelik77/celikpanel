// alpha.149, where PR #370 and PR #371 meet on the site's limits card: a
// database user's value the panel did not write carries its origin. "owner"
// is a firm limit only an administrator account can set (temporary files, the
// connection limit) and keeps the sentence slim.dbOwner; "on_user" is a
// default changed on the database user itself (statement time, idle
// transaction time), by the server owner or the user's own session, and gets
// its own sentence slim.dbOnUser. The words are the lane's own route-scoped
// module (no screen part touched, no budget raised).
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { dbOwnerByOrigin } from '../src/lib/siteLimits.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const view = (dbOwner) => ({ known: true, domain: 'example.com', admin: true, limits: [], events: [], db_owner: dbOwner });

test('the card splits the database users’ own values by their origin', () => {
  const split = dbOwnerByOrigin(view([
    { user: 'site_a', key: 'db.statement_seconds', value: '30s', origin: 'on_user' },
    { user: 'site_a', key: 'db.max_user_connections', value: '25', origin: 'owner' },
    { user: 'site_b', key: 'db.idle_in_transaction_seconds', value: '0', origin: 'on_user' },
    { user: 'site_b', key: 'db.temp_file_mb', value: '1024MB', origin: 'owner' },
  ]));
  assert.deepEqual(split.owner.map((s) => `${s.user}:${s.key}`), ['site_a:db.max_user_connections', 'site_b:db.temp_file_mb']);
  assert.deepEqual(split.onUser.map((s) => `${s.user}:${s.key}`), ['site_a:db.statement_seconds', 'site_b:db.idle_in_transaction_seconds']);
});

test('a value without a known origin stays the owner’s, and nothing at all gives two empty lists', () => {
  const split = dbOwnerByOrigin(view([{ user: 'site_c', key: 'db.statement_seconds', value: '5s' }, null]));
  assert.deepEqual(split.owner.map((s) => s.user), ['site_c']);
  assert.deepEqual(split.onUser, []);
  assert.deepEqual(dbOwnerByOrigin(view(undefined)), { owner: [], onUser: [] });
});

test('two sentences, each in the page’s own language and naming the list', () => {
  assert.match(enSiteLimitsLane['slim.dbOnUser'], /changed on the user itself[\s\S]*does not impose it again[\s\S]*\{list\}/);
  assert.match(trSiteLimitsLane['slim.dbOnUser'], /kullanıcının kendisinde değiştirilmiş[\s\S]*yeniden dayatmaz[\s\S]*\{list\}/);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /const dbByOrigin = dbOwnerByOrigin\(view\);/);
  assert.match(card, /owner\.length > 0 && <p[^>]*data-site-limits-dbowner=""[^>]*>\{t\('slim\.dbOwner', \{ list: owner\.join\('; '\) \}\)\}/);
  assert.match(card, /onUser\.length > 0 && <p[^>]*data-site-limits-dbonuser=""[^>]*>\{t\('slim\.dbOnUser', \{ list: onUser\.join\('; '\) \}\)\}/);
  // The new words are the lane's, never a screen part every screen loads.
  for (const part of ['../src/i18n/screens/siteLimits/en.ts', '../src/i18n/screens/siteLimits/tr.ts']) {
    assert.ok(!read(part).includes('slim.dbOnUser'), `${part} must not carry slim.dbOnUser`);
  }
});
