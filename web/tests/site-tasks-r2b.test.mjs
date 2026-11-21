// S4 R2b (22 Nov 2026): a site's scheduled tasks as timers inside its own
// limits. The Scheduled tasks screen says each task's last run and the two
// halves of what a long run does; the administrator's move takes the site's
// crontab too (preview first) and can be undone; "Move all" stops at the first
// site that does not move; the Problems now lines have words in both
// languages, plain and without other companies' product names.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteTasksScreens } from '../src/i18n/screens/siteTasks/en.ts';
import { trSiteTasksScreens } from '../src/i18n/screens/siteTasks/tr.ts';
import { decodeOutsideSites, decodeTaskRun, decodeTasksAnswer, decodeTasksPlacement, lineList, movedTaskCount, siteMoved } from '../src/lib/siteTasks.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('the words are the same keys in both languages, with the same placeholders, and say nothing of other products', () => {
  assert.deepEqual(Object.keys(trSiteTasksScreens).sort(), Object.keys(enSiteTasksScreens).sort());
  for (const [key, en] of Object.entries(enSiteTasksScreens)) {
    const want = [...en.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    const got = [...trSiteTasksScreens[key].matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    assert.deepEqual(got, want, `${key} keeps its placeholders`);
  }
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) {
    for (const [key, text] of Object.entries(words)) {
      assert.ok(!/cPanel|Plesk|aaPanel|DirectAdmin|celikcron|celiksite_|OnCalendar|R2b|D-0/.test(text), `${key} says nothing internal or of another product: ${text}`);
    }
  }
});

test('the screen says both halves of what a task that runs long does', () => {
  assert.match(enSiteTasksScreens['stasks.timers'], /not started a second time/);
  assert.match(enSiteTasksScreens['stasks.timers'], /starts once more right after it ends/);
  assert.match(trSiteTasksScreens['stasks.timers'], /ikinci kez başlatılmaz/);
  assert.match(trSiteTasksScreens['stasks.timers'], /bittikten hemen sonra bir kez daha başlar/);
  const screen = read('../src/components/DomainCronManager.tsx');
  assert.match(screen, /t\('stasks\.timers'\)/);
  assert.match(screen, /<TaskRunLine run=\{job\.run\}/);
  // The returned crontab's lines are only in the answer the administrator gets
  // (the server leaves them out for anyone else).
  assert.match(read('../../cmd/panel/domain_cron_handlers.go'), /resp\.Reappeared = nil/);
});

test('a task run is read through one decoder, bounded to its last ten lines', () => {
  assert.equal(decodeTaskRun(null), undefined);
  assert.equal(decodeTaskRun({ state: 'exploded' }), undefined);
  const run = decodeTaskRun({ state: 'failed', started_at: '2026-11-22T03:17:00Z', seconds: 2.5, exit_status: 3, result: 'exit-code', output: [...Array(15).keys()].map(String) });
  assert.equal(run.state, 'failed');
  assert.equal(run.exit_status, 3);
  assert.deepEqual(run.output, ['5', '6', '7', '8', '9', '10', '11', '12', '13', '14']);
  assert.deepEqual(decodeTasksPlacement({ moved: false, reappeared: 2 }), { moved: false, reappeared: 2, recovery: undefined, unknown: false });
});

test('the move reads a preview first and nothing is half-moved', () => {
  const preview = decodeTasksAnswer({ outcome: 'preview', rows: [{ line: 2, text: 'MAILTO=""', setting: true }, { line: 4, text: '17 3 * * * php x.php', unit: 'u1' }], mail: true });
  assert.equal(movedTaskCount(preview), 1);
  assert.throws(() => decodeTasksAnswer({ outcome: 'half' }));
  const stayed = decodeTasksAnswer({ outcome: 'stayed', blocking: [{ line: 6, text: 'MAILTO=a@b.test', reason: 'mailto' }] });
  assert.equal(lineList(stayed.blocking), '6: MAILTO=a@b.test');
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /readTasksPreview\(domainId\)/);
  assert.match(read('../src/lib/siteTasks.ts'), /site-limits\/preview/);
  assert.match(card, /site-limits\/tasks-undo/);
  assert.match(card, /t\('slim\.tasks\.mail'\)/);
});

test('"Move all" stops at the first site that does not move', () => {
  assert.deepEqual(siteMoved({ done: true, tasks: { outcome: 'moved' } }), { ok: true, reason: '' });
  assert.deepEqual(siteMoved({ done: true, tasks: { outcome: 'already' } }), { ok: true, reason: '' });
  assert.equal(siteMoved({ done: true, tasks: { outcome: 'stayed', blocking: [{ line: 3, text: 'x', reason: 'schedule' }] } }).ok, false);
  assert.equal(siteMoved({ done: false, rolled_back: true, error: 'put back', tasks: { outcome: 'moved' } }).ok, false);
  assert.equal(siteMoved({ done: true }).ok, false, 'an answer without the tasks part is not a success');
  assert.deepEqual(decodeOutsideSites({ sites: [{ domain_id: 4, domain: 'a.test', tasks: true }, { domain: 'broken' }] }), [{ domain_id: 4, domain: 'a.test', tasks: true }]);
  const all = read('../src/components/MoveAllSites.tsx');
  assert.match(all, /return \{ state: 'failed', reason: t\('moveall\.failed'/);
  assert.match(all, /readOutsideSites\(\)/);
  assert.match(read('../src/lib/siteTasks.ts'), /\/api\/v1\/site-limits\/outside/);
});

test('the Problems now lines are known kinds with words, and the two re-closes are offered', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  const list = problems.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  for (const kind of ['site_limits_outside', 'cron_allow_lists_sites', 'cron_not_closed', 'cron_owner_removed', 'site_crontab_returned', 'site_lingering', 'linger_owner_removed', 'linger_not_closed']) {
    assert.ok(list && list[1].includes(`'${kind}'`), `${kind} is a known kind`);
    for (const part of ['title', 'body', 'action']) {
      assert.ok(enSiteTasksScreens[`pnow.${kind}.${part}`], `English ${kind}.${part}`);
      assert.ok(trSiteTasksScreens[`pnow.${kind}.${part}`], `Turkish ${kind}.${part}`);
    }
  }
  for (const kind of ['cron_owner_removed', 'linger_owner_removed']) assert.ok(enSiteTasksScreens[`pnow.${kind}.confirm`] && trSiteTasksScreens[`pnow.${kind}.confirm`]);
  assert.match(problems, /const RECLOSE = \['at', 'stock_pool', 'cron', 'linger'\];/);
  assert.match(problems, /problem\.action\.kind === 'move_all' \?/);
  assert.match(enSiteTasksScreens['pnow.site_lingering.body'], /\{other\}/, 'the lingering line gives the command');
});

test('the part is registered and budgeted', () => {
  assert.match(read('../src/i18n/screenParts.ts'), /screens\/siteTasks\/en/);
  assert.match(read('../src/i18n/screenParts.ts'), /screens\/siteTasks\/tr/);
  assert.match(read('../scripts/check-bundle-budget.mjs'), /siteTasks: kib\(/);
  assert.match(read('./locale-catalogue.mjs'), /'siteTasks'/);
});
