// S4 R1 (20 Nov 2026): the Problems now lines of the side doors step - a
// database tool or webmail still under the web server's account, the
// distribution's PHP-FPM pool kept on - are kinds the list shows, with words
// in both languages in their own catalogue part, saying who acts and what to
// do.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enMonitoringScreens } from '../src/i18n/screens/monitoring/en.ts';
import { trMonitoringScreens } from '../src/i18n/screens/monitoring/tr.ts';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';
import { trProblemsNowScreens } from '../src/i18n/screens/problemsNow/tr.ts';
import { enSideDoorsScreens } from '../src/i18n/screens/sideDoors/en.ts';
import { trSideDoorsScreens } from '../src/i18n/screens/sideDoors/tr.ts';
import { enSiteJobsScreens } from '../src/i18n/screens/siteJobs/en.ts';
import { trSiteJobsScreens } from '../src/i18n/screens/siteJobs/tr.ts';
import { enSideDoorsUndoScreens } from '../src/i18n/screens/sideDoorsUndo/en.ts';
import { trSideDoorsUndoScreens } from '../src/i18n/screens/sideDoorsUndo/tr.ts';
import { enMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/en.ts';
import { trMailDatabasesScreens } from '../src/i18n/screens/mailDatabases/tr.ts';
import { enPgLeftoversScreens } from '../src/i18n/screens/pgLeftovers/en.ts';
import { trPgLeftoversScreens } from '../src/i18n/screens/pgLeftovers/tr.ts';
import { enSiteLimitsScreens } from '../src/i18n/screens/siteLimits/en.ts';
import { trSiteLimitsScreens } from '../src/i18n/screens/siteLimits/tr.ts';
import { enSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/en.ts';
import { trSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/tr.ts';

const kinds = ['tool_account_failed', 'tool_account_owner_config', 'stock_pool_in_use', 'stock_pool_owner_changed'];

test('every kind the list shows has a title, body and action in English and Turkish', () => {
  const source = readFileSync(new URL('../src/components/ProblemsNow.tsx', import.meta.url), 'utf8');
  const list = source.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  assert.ok(list, 'the KINDS list');
  const shown = [...list[1].matchAll(/'([a-z_]+)'/g)].map((m) => m[1]);
  for (const kind of kinds) assert.ok(shown.includes(kind), `${kind} is shown`);
  // 21 Nov 2026: the site jobs lines have their words in the siteJobs part,
  // the site limits lines (S4 R2a) in the siteLimits part, and those of its
  // second fix round in the siteLimitsApply part.
  // Since alpha.142 the shared lines' words are in the problemsNow part.
  // alpha.143 (merged): its lines' words are in the mailDatabases and
  // pgLeftovers parts; the site limits lines (S4 R2a) in siteLimits and
  // siteLimitsApply.
  const en = { ...enMonitoringScreens, ...enProblemsNowScreens, ...enSideDoorsScreens, ...enSiteJobsScreens, ...enSideDoorsUndoScreens, ...enMailDatabasesScreens, ...enPgLeftoversScreens, ...enSiteLimitsScreens, ...enSiteLimitsApplyScreens };
  const tr = { ...trMonitoringScreens, ...trProblemsNowScreens, ...trSideDoorsScreens, ...trSiteJobsScreens, ...trSideDoorsUndoScreens, ...trMailDatabasesScreens, ...trPgLeftoversScreens, ...trSiteLimitsScreens, ...trSiteLimitsApplyScreens };
  for (const kind of shown) {
    // alpha.143: mail_signing and mail_autoresponder_foreign have one body
    // per state instead of one body (their own tests check them).
    const perState = (words) => Object.keys(words).some((key) => key.startsWith(`pnow.${kind}.`) && !/\.(title|action|body)$/.test(key));
    if (!en[`pnow.${kind}.body`] && perState(en) && perState(tr)) {
      assert.ok(en[`pnow.${kind}.title`] && tr[`pnow.${kind}.title`], `pnow.${kind}.title`);
      continue;
    }
    for (const part of ['title', 'body', 'action']) {
      assert.ok(en[`pnow.${kind}.${part}`], `English pnow.${kind}.${part}`);
      assert.ok(tr[`pnow.${kind}.${part}`], `Turkish pnow.${kind}.${part}`);
    }
  }
});

test('the side doors lines name the thing, say who acts and stay in their own part', () => {
  for (const kind of kinds) {
    for (const words of [enSideDoorsScreens, trSideDoorsScreens]) {
      assert.match(words[`pnow.${kind}.title`], /\{name\}/, `${kind} title names it`);
      const body = words[`pnow.${kind}.body`];
      assert.ok(!/journalctl|systemctl/.test(body), `${kind} body sends nobody to a terminal command`);
    }
    assert.match(enSideDoorsScreens[`pnow.${kind}.body`], /server administrator|choose its PHP version/, `${kind} says who acts`);
    assert.equal(enMonitoringScreens[`pnow.${kind}.title`], undefined, `${kind} is not added to the full monitoring part`);
    assert.equal(enProblemsNowScreens[`pnow.${kind}.title`], undefined, `${kind} is not added to the problemsNow part`);
  }
  assert.deepEqual(Object.keys(trSideDoorsScreens).sort(), Object.keys(enSideDoorsScreens).sort());
});

test('a move that could not give the tool its own account is said in the reader’s language with the server’s words', () => {
  const source = readFileSync(new URL('../src/lib/phpTools.ts', import.meta.url), 'utf8');
  const kinds = source.match(/const CAUSE_KINDS: ReadonlySet<string> = new Set\(\[([\s\S]*?)\]\);/);
  const said = source.match(/const SAID_KINDS: ReadonlySet<string> = new Set\(\[([\s\S]*?)\]\);/);
  assert.ok(kinds && kinds[1].includes("'own_account_failed'"), 'a cause the screen translates');
  assert.ok(said && said[1].includes("'own_account_failed'"), 'the server’s words follow it');
  for (const words of [enSideDoorsScreens, trSideDoorsScreens]) {
    assert.match(words['component.phpTool.cause.own_account_failed'], /\{version\}/);
  }
});
