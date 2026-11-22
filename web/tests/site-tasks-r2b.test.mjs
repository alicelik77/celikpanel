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
import {
  decodeOutsideSites, decodeTaskRun, decodeTasksAnswer, decodeTasksPlacement, lineList, movedTaskCount, moveSitesInTurn, privilegedList, siteMoveEnd,
} from '../src/lib/siteTasks.ts';

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
  assert.deepEqual(decodeTasksPlacement({ moved: false, reappeared: 2 }), { moved: false, reappeared: 2, by_site: false, mail_submission: undefined, recovery: undefined, unknown: false });
  assert.equal(decodeTasksPlacement({ moved: true, reappeared: 1, by_site: true, mail_submission: 'owner' }).by_site, true);
});

test('fix round: a crontab the site can have made is not called the owner\'s; mail and extra rights are said', () => {
  assert.doesNotMatch(enSiteTasksScreens['stasks.reappearedSite'], /server owner’s/);
  assert.match(enSiteTasksScreens['stasks.reappearedSite'], /site’s own code can have made it/);
  assert.match(enSiteTasksScreens['stasks.reappeared'], /Only root could make it/);
  const screen = read('../src/components/DomainCronManager.tsx');
  assert.match(screen, /timers\.bySite \? 'stasks\.reappearedSite' : 'stasks\.reappeared'/);
  assert.match(screen, /site-limits\/tasks-source/);
  assert.match(screen, /t\('stasks\.rebuild\.body'\)/);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /tasks\.by_site \? 'slim\.tasks\.reappearedSite' : 'slim\.tasks\.reappeared'/);
  assert.match(card, /'slim\.tasks\.mailOwner'/);
  assert.match(card, /'slim\.tasks\.mailAbsent'/);
  assert.match(card, /t\('slim\.tasks\.privileged'/);
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) {
    assert.match(words['stasks.differences'], /sudo, crontab|sudo, crontab ya da ping/);
  }
  const preview = decodeTasksAnswer({ outcome: 'preview', rows: [{ line: 1, text: '0 1 * * * sudo x', unit: 'u1', privileged: true }, { line: 2, text: '0 2 * * * php x', unit: 'u2' }] });
  assert.equal(privilegedList(preview), '1: 0 1 * * * sudo x');
  assert.equal(decodeTasksAnswer({ outcome: 'crontab_back', error: 'a crontab exists again' }).outcome, 'crontab_back');
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

test('"Move all" skips a site that cannot move and stops only when a started move fails (fix round)', async () => {
  assert.deepEqual(siteMoveEnd({ done: true, units: [], tasks: { outcome: 'moved' } }), { kind: 'moved', note: '' });
  assert.deepEqual(siteMoveEnd({ done: true, units: [], tasks: { outcome: 'already' } }), { kind: 'moved', note: '' });
  assert.deepEqual(siteMoveEnd({ done: true, units: [], tasks: { outcome: 'stayed', blocking: [{ line: 3, text: 'x', reason: 'schedule' }] } }), { kind: 'skipped', reason: '3: x' });
  assert.equal(siteMoveEnd({ done: true, units: [{ unit: 'celikapp-4.service' }], tasks: { outcome: 'refused', error: 'owner file' } }).kind, 'partial');
  assert.equal(siteMoveEnd({ done: false, rolled_back: true, error: 'put back', tasks: { outcome: 'moved' } }).kind, 'stop');
  assert.equal(siteMoveEnd({ done: true, tasks: { outcome: 'put_back', error: 'x' } }).kind, 'stop');
  assert.equal(siteMoveEnd({ done: true }).kind, 'stop', 'an answer without the tasks part is not a success');
  assert.deepEqual(decodeOutsideSites({ sites: [{ domain_id: 4, domain: 'a.test', tasks: true }, { domain: 'broken' }] }), [{ domain_id: 4, domain: 'a.test', tasks: true }]);

  // The unmovable site sorts first (alphabetically); every site after it still moves.
  const sites = [{ domain_id: 1, domain: 'a-stays.test', tasks: true }, { domain_id: 2, domain: 'b.test', tasks: true }, { domain_id: 3, domain: 'c.test', programs: 1 }];
  const answers = {
    1: { status: 200, body: { done: true, units: [], tasks: { outcome: 'stayed', blocking: [{ line: 6, text: 'MAILTO=me@x.test', reason: 'mailto' }] } } },
    2: { status: 200, body: { done: true, units: [], tasks: { outcome: 'moved' } } },
    3: { status: 200, body: { done: true, units: [{ unit: 'celikapp-3.service', outcome: 'moved' }], tasks: { outcome: 'already' } } },
  };
  const seen = [];
  const outcome = await moveSitesInTurn(sites, async (site) => { seen.push(site.domain); return answers[site.domain_id]; }, () => {});
  assert.deepEqual(seen, ['a-stays.test', 'b.test', 'c.test']);
  assert.equal(outcome.moved, 2);
  assert.deepEqual(outcome.notMoved, [{ domain: 'a-stays.test', reason: '6: MAILTO=me@x.test' }]);
  assert.equal(outcome.stop, undefined);
  // A refusal before anything changed (409) is skipped too; a started move that fails stops the run.
  answers[1] = { status: 409, refusal: 'a site is being moved now' };
  answers[2] = { status: 200, body: { done: false, rolled_back: true, error: 'the program did not start', tasks: { outcome: 'moved' } } };
  seen.length = 0;
  const stopped = await moveSitesInTurn(sites, async (site) => { seen.push(site.domain); return answers[site.domain_id]; }, () => {});
  assert.deepEqual(seen, ['a-stays.test', 'b.test'], 'nothing after the failed move is touched');
  assert.deepEqual(stopped.stop, { domain: 'b.test', reason: 'the program did not start', unknown: false });
  const unknown = await moveSitesInTurn([sites[1]], async () => null, () => {});
  assert.equal(unknown.stop.unknown, true);

  const all = read('../src/components/MoveAllSites.tsx');
  assert.match(all, /readTasksPreview\(site\.domain_id\)/, 'each site\'s preview is read before the run');
  assert.match(all, /option: programsOnly\.length > 0/, 'programs-only sites move only when ticked');
  assert.match(all, /t\('moveall\.failed'/);
  assert.match(all, /<Link to=\{`\/domains\/\$\{encodeURIComponent\(l\.domain\)\}`\}/, 'each site not moved links to its page');
  assert.match(read('../src/lib/siteTasks.ts'), /\/api\/v1\/site-limits\/outside/);
});

test('the Problems now lines are known kinds with words, and the two re-closes are offered', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  const list = problems.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  for (const kind of ['site_limits_outside', 'cron_allow_lists_sites', 'cron_not_closed', 'cron_owner_removed', 'site_crontab_returned', 'site_lingering', 'linger_owner_removed', 'linger_not_closed', 'site_crontab_by_site', 'site_tasks_held']) {
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
