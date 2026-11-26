// alpha.149: database users the plan's database limits did not reach.
// Problems now names them per outcome - the Agent not reachable, the database
// server not running, a refused connection, a refusal of the user, a
// verified failure, a result not known - with the stored reason worded and
// Apply again, which the panel runs apart from the page and this list only
// follows with reads (a run it no longer sees is "not known yet"); a plan's
// Save says what it waits for and which users did not get the limits, a
// reseller without being sent to the administrator's screens. The words load
// with the screens that show them (review of PR #370, findings 6, 7, 11, 15,
// 16, 17, 18).
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enDbLimitsWords } from '../src/i18n/dbLimitsWords/en.ts';
import { trDbLimitsWords } from '../src/i18n/dbLimitsWords/tr.ts';
import { enDbLimitsScreens } from '../src/i18n/screens/dbLimits/en.ts';
import { trDbLimitsScreens } from '../src/i18n/screens/dbLimits/tr.ts';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const states = ['agent', 'prerequisite', 'connection', 'refused', 'failed', 'unknown'];
const reasons = ['agent_unreachable', 'server_not_running', 'connection_refused', 'refused_privileged', 'refused_member', 'refused_not_recorded',
  'refused_absent', 'engine_refused', 'result_unknown', 'not_known_yet', 'server_record_unreadable', 'record_unreadable'];

test('the words have the same keys and placeholders in both languages and say nothing internal', () => {
  for (const [en, tr] of [[enDbLimitsWords, trDbLimitsWords], [enDbLimitsScreens, trDbLimitsScreens]]) {
    assert.deepEqual(Object.keys(tr).sort(), Object.keys(en).sort());
    for (const [key, text] of Object.entries(en)) {
      const want = [...text.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
      const got = [...tr[key].matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
      assert.deepEqual(got, want, `${key} keeps its placeholders`);
      assert.equal(enProblemsNowScreens[key], undefined, `${key} is in one place only`);
    }
    for (const words of [en, tr]) {
      for (const [key, text] of Object.entries(words)) {
        assert.ok(!/cPanel|Plesk|aaPanel|DirectAdmin|celikpanel_admin|SQLSTATE|D-0|D9|D10|alpha\.1|journalctl/.test(text), `${key}: ${text}`);
      }
    }
  }
  // Each outcome has its words, and says who acts and how it resumes (D-024).
  for (const state of states) assert.ok(enDbLimitsWords[`pnow.database_limits_unapplied.${state}`], state);
  for (const reason of reasons) assert.ok(enDbLimitsWords[`pnow.database_limits_unapplied.reason.${reason}`], reason);
  assert.match(enDbLimitsWords['pnow.database_limits_unapplied.prerequisite'], /server owner starts it.*Apply again/);
  assert.match(enDbLimitsWords['pnow.database_limits_unapplied.agent'], /administrator checks the Agent.*Apply again/);
  assert.match(enDbLimitsWords['pnow.database_limits_unapplied.refused'], /server’s administrator decides/);
  // Finding 17: only what is true - no "they keep their earlier limits" after
  // a refused write, no "only what is missing".
  assert.doesNotMatch(enDbLimitsWords['pnow.database_limits_unapplied.failed'], /keep their earlier limits/);
  assert.doesNotMatch(enDbLimitsWords['pnow.database_limits_unapplied.unknown'], /only what is missing/);
  assert.match(enDbLimitsWords['pnow.database_limits_unapplied.unknown'], /stays until the values are read back/);
  // Finding 15: the journal only as where the detail is.
  assert.match(enDbLimitsWords['pnow.database_limits_unapplied.why'], /^Why: \{reason\}\. The detail is in the panel’s journal\.$/);
  // Finding 16: a reseller is not sent to the administrator's screens.
  for (const key of ['plans.dbLimits.notAppliedReseller', 'plans.dbLimits.runningReseller']) {
    assert.match(enDbLimitsWords[key], /server’s administrator sees to/);
    assert.doesNotMatch(enDbLimitsWords[key], /Problems now|Apply again/);
  }
});

test('the words load with the screens that show them, not with every screen', () => {
  const parts = read('../src/i18n/screenParts.ts');
  assert.doesNotMatch(parts, /dbLimitsWords/, 'not a part every screen fetches');
  assert.deepEqual(Object.keys(enDbLimitsScreens), ['dashboard.audit.action.database.limits.apply_again'], 'the always-loaded part keeps only the audit name');
  const loader = read('../src/i18n/dbLimits.ts');
  assert.match(loader, /import\('\.\/dbLimitsWords\/tr'\)/);
  assert.match(loader, /import\('\.\/dbLimitsWords\/en'\)/);
  const screen = read('../src/components/ProblemsNow.tsx');
  assert.match(screen, /useDbLimitsWords\(\(listed\?\.problems \?\? \[\]\)\.some\(\(p: Problem\) => p\.kind === 'database_limits_unapplied'\)\)/, 'fetched only with such a line');
  assert.match(screen, /p\.kind !== 'database_limits_unapplied' \|\| dbWords\.ready/, 'the line waits for its words');
  assert.doesNotMatch(screen, /import \{[^}]*\} from '\.\.\/i18n\/dbLimitsWords/, 'never a static import of the words');
  const editor = read('../src/components/PlanRights.tsx');
  assert.match(editor, /useDbLimitsWords\(dbChanged\)/, 'fetched when a database limit changes');
  const budget = read('../scripts/check-bundle-budget.mjs');
  assert.match(budget, /'src\/i18n\/dbLimitsWords\/en\.ts': budgets\.async/);
  assert.match(budget, /dbLimits: kib\(0\.6, 0\.5\)/);
});

test('Problems now words each outcome with its reason and offers Apply again, reading only to follow it', () => {
  const screen = read('../src/components/ProblemsNow.tsx');
  const kinds = screen.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/)?.[1] ?? '';
  assert.match(kinds, /'database_limits_unapplied',/, 'the kind is one this screen has words for');
  assert.match(screen, /const DB_LIMITS_STATES = \['agent', 'prerequisite', 'connection', 'refused', 'failed', 'unknown'\];/);
  assert.match(screen, /DB_LIMITS_STATES\.includes\(problem\.name \?\? ''\) \? problem\.name : 'failed'/, 'an outcome this screen does not know reads as a failure, never as fine');
  assert.match(screen, /DB_LIMITS_REASONS\.includes\(problem\.reason \?\? ''\)/, 'only reasons it has words for are said');
  assert.match(screen, /p\.action\.kind === 'apply_db_limits' \? p\.kind === 'database_limits_unapplied'/, 'the action belongs to this kind only');
  const apply = screen.slice(screen.indexOf('const applyDbLimits'), screen.indexOf('const remote = handle.remote;'));
  assert.equal((apply.match(/method: 'POST'/g) ?? []).length, 1, 'one press sends one request');
  assert.match(apply, /readRemote\(DB_LIMITS_URL/, 'the run is followed with reads');
  assert.match(apply, /\(run\.id \?\? ''\) !== started\) return \{ state: 'unknown', reason: say\('pnow\.database_limits_unapplied\.notKnownYet'\) \}/,
    'a run this window no longer sees is never said to be done');
  assert.match(apply, /if \(run\.running\) return unknown;/, 'a run whose end was not read is said as unknown');
  const server = read('../../cmd/panel/database_limits_problems.go');
  assert.match(server, /Kind: "database_limits_unapplied"/);
  assert.match(server, /Action: monitoringProblemAction\{Kind: "apply_db_limits"\}/);
  assert.match(server, /Reason: dbLimitsReasonCode\(g\.code\)/);
  const route = read('../../cmd/panel/main.go');
  assert.match(route, /"\/api\/v1\/metrics\/database-limits\/apply"/);
  assert.match(screen, /DB_LIMITS_URL = '\/api\/v1\/metrics\/database-limits\/apply'/);
});

test('a plan’s Save says what it waits for and which database users did not get its database limits', () => {
  const editor = read('../src/components/PlanRights.tsx');
  assert.match(editor, /progressTitle: dbWords\.say\('plans\.dbLimits\.title'\)/, 'the window is locked while it waits');
  assert.match(editor, /label: dbWords\.say\('plans\.dbLimits\.step'\), state: 'running'/, 'it says the step');
  assert.match(editor, /grantorIsReseller \? 'plans\.dbLimits\.notAppliedReseller' : 'plans\.dbLimits\.notApplied'/);
  assert.match(editor, /grantorIsReseller \? 'plans\.dbLimits\.runningReseller' : 'plans\.dbLimits\.running'/);
  assert.match(enDbLimitsWords['plans.dbLimits.notApplied'], /\{n\} database users \(\{users\}\)/);
  assert.equal(enDbLimitsWords['plans.dbLimits.step'], 'Applying the limits to the database users…');
});
