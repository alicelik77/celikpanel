// alpha.149 fix round B: database users the plan's database limits did not
// reach. Problems now names them per outcome - a verified failure, an unmet
// prerequisite, an unknown result - with Apply again, which the panel runs
// apart from the page and this list only follows with reads; a plan's Save
// says which users did not get the limits. The words are a part of their own.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enDbLimitsScreens } from '../src/i18n/screens/dbLimits/en.ts';
import { trDbLimitsScreens } from '../src/i18n/screens/dbLimits/tr.ts';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('the part has the same keys and placeholders in both languages and says nothing internal', () => {
  assert.deepEqual(Object.keys(trDbLimitsScreens).sort(), Object.keys(enDbLimitsScreens).sort());
  for (const [key, en] of Object.entries(enDbLimitsScreens)) {
    const want = [...en.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    const got = [...trDbLimitsScreens[key].matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    assert.deepEqual(got, want, `${key} keeps its placeholders`);
    assert.equal(enProblemsNowScreens[key], undefined, `${key} is in one part only`);
  }
  for (const words of [enDbLimitsScreens, trDbLimitsScreens]) {
    for (const [key, text] of Object.entries(words)) {
      assert.ok(!/cPanel|Plesk|aaPanel|DirectAdmin|celikpanel_admin|SQLSTATE|D-0|D9|D10|alpha\.1/.test(text), `${key}: ${text}`);
    }
  }
  // Each outcome says who acts and how it resumes (D-024).
  assert.match(enDbLimitsScreens['pnow.database_limits_unapplied.prerequisite'], /server owner starts it.*Apply again/);
  assert.match(enDbLimitsScreens['pnow.database_limits_unapplied.failed'], /journalctl.*Apply again/);
  assert.match(enDbLimitsScreens['pnow.database_limits_unapplied.unknown'], /not known.*Apply again/);
});

test('Problems now words each outcome and offers Apply again, reading only to follow it', () => {
  const screen = read('../src/components/ProblemsNow.tsx');
  const kinds = screen.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/)?.[1] ?? '';
  assert.match(kinds, /'database_limits_unapplied',/, 'the kind is one this screen has words for');
  assert.match(screen, /const DB_LIMITS_STATES = \['failed', 'prerequisite', 'unknown'\];/);
  assert.match(screen, /problem\.kind === 'database_limits_unapplied'\) return `pnow\.database_limits_unapplied\.\$\{DB_LIMITS_STATES\.includes\(problem\.name \?\? ''\) \? problem\.name : 'failed'\}`/,
    'an outcome this screen does not know reads as a failure, never as fine');
  assert.match(screen, /p\.action\.kind === 'apply_db_limits' \? p\.kind === 'database_limits_unapplied'/, 'the action belongs to this kind only');
  const apply = screen.slice(screen.indexOf('const applyDbLimits'), screen.indexOf('const remote = handle.remote;'));
  assert.equal((apply.match(/method: 'POST'/g) ?? []).length, 1, 'one press sends one request');
  assert.match(apply, /readRemote\(DB_LIMITS_URL/, 'the run is followed with reads');
  assert.match(apply, /if \(run\.running\) return unknown;/, 'a run whose end was not read is said as unknown');
  const server = read('../../cmd/panel/database_limits_problems.go');
  assert.match(server, /Kind: "database_limits_unapplied"/);
  assert.match(server, /Action: monitoringProblemAction\{Kind: "apply_db_limits"\}/);
  const route = read('../../cmd/panel/main.go');
  assert.match(route, /"\/api\/v1\/metrics\/database-limits\/apply"/);
  assert.match(screen, /DB_LIMITS_URL = '\/api\/v1\/metrics\/database-limits\/apply'/);
});

test('a plan’s Save says which database users did not get its database limits', () => {
  const editor = read('../src/components/PlanRights.tsx');
  assert.match(editor, /db\?\.not_applied_count\) showToast\('warning', t\('plans\.dbLimits\.notApplied'/);
  assert.match(editor, /else if \(db\?\.running\) showToast\('warning', t\('plans\.dbLimits\.running'/);
  assert.match(enDbLimitsScreens['plans.dbLimits.notApplied'], /\{n\} database users \(\{users\}\)/);
});
