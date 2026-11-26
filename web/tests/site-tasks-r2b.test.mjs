// S4 R2b (22 Nov 2026): a site's scheduled tasks as timers inside its own
// limits. The Scheduled tasks screen says each task's last run and the two
// halves of what a long run does; the administrator's move takes the site's
// crontab too (preview first) and can be undone; "Move all" stops at the first
// site that does not move; the Problems now lines have words in both
// languages, plain and without other companies' product names.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteTasksScreens as enSiteTasksPart } from '../src/i18n/screens/siteTasks/en.ts';
import { enSiteTasksProblemsScreens as enSiteTasksSplit } from '../src/i18n/screens/siteTasksProblems/en.ts';
// alpha.150: the part siteTasksProblems holds the words split out of this part.
const enSiteTasksScreens = { ...enSiteTasksPart, ...enSiteTasksSplit };
import { trSiteTasksScreens as trSiteTasksPart } from '../src/i18n/screens/siteTasks/tr.ts';
import { trSiteTasksProblemsScreens as trSiteTasksSplit } from '../src/i18n/screens/siteTasksProblems/tr.ts';
const trSiteTasksScreens = { ...trSiteTasksPart, ...trSiteTasksSplit };
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';
import { trProblemsNowScreens } from '../src/i18n/screens/problemsNow/tr.ts';
import {
  decodeOutsideSites, decodeTaskRun, decodeTasksAnswer, decodeTasksPlacement, lineList, movedTaskCount, moveSitesInTurn, privilegedList, siteMoveEnd,
  unresolvedSteps,
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

  // Second fix round (review N5): a 5xx (503 "the result is unknown", 504, 500), any other
  // status that is not a refusal that changed nothing, and a success without a readable body
  // all stop the run as unknown; only 400, 403, 404 and 409 are skipped.
  for (const status of [500, 502, 503, 504, 401, 429]) {
    seen.length = 0;
    const answer = { status, refusal: 'The server did not answer, so the result is unknown.' };
    const out = await moveSitesInTurn(sites, async (site) => { seen.push(site.domain); return site.domain_id === 1 ? answer : answers[2]; }, () => {});
    assert.deepEqual(seen, ['a-stays.test'], `status ${status} stops the run`);
    assert.equal(out.stop.unknown, true, `status ${status} is unknown`);
  }
  for (const status of [400, 403, 404, 409]) {
    const out = await moveSitesInTurn([sites[0]], async () => ({ status, refusal: 'no' }), () => {});
    assert.equal(out.stop, undefined, `status ${status} is a skip`);
    assert.equal(out.notMoved.length, 1);
  }
  const noBody = await moveSitesInTurn([sites[0], sites[1]], async () => ({ status: 200 }), () => {});
  assert.equal(noBody.stop.unknown, true, 'a success without a readable body is unknown');
  // Review N1 (alpha.150: main's one kept record): a kept move record stops the run (nothing
  // else is moved), whether the tasks part or the programs part answered it.
  const unresolved = await moveSitesInTurn(sites, async (site) => ({ status: 200, body: { done: false, kept: true, reason: { code: 'kept_earlier' }, units: [],
    tasks: { outcome: 'refused', refusal: 'kept_earlier', error: 'an earlier move of site 7' } } }), () => {});
  assert.equal(unresolved.stop.unresolved, true);
  assert.equal(unresolved.moved, 0);
  const keptByPrograms = await moveSitesInTurn(sites, async () => ({ status: 200, body: { done: false, kept: true, reason: { code: 'kept_earlier' }, units: [],
    tasks: { outcome: 'moved' } } }), () => {});
  assert.equal(keptByPrograms.stop.unresolved, true);
  assert.match(read('../src/components/MoveAllSites.tsx'), /t\('moveall\.unresolved'\)/);

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
  // Second fix round: the held, lingering and service-manager lines live in the "problemsNow"
  // part (the "siteTasks" part had no room left); either part may hold them. A move that could
  // not be settled is main's site_move_kept line (alpha.150: one record, one line).
  const en = { ...enSiteTasksScreens, ...enProblemsNowScreens };
  const tr = { ...trSiteTasksScreens, ...trProblemsNowScreens };
  for (const kind of ['site_limits_outside', 'cron_allow_lists_sites', 'cron_not_closed', 'cron_owner_removed', 'site_crontab_returned', 'site_lingering', 'linger_owner_removed', 'linger_not_closed', 'site_crontab_by_site', 'site_tasks_held',
    'site_move_kept', 'site_user_manager_open']) {
    assert.ok(list && list[1].includes(`'${kind}'`), `${kind} is a known kind`);
    for (const part of ['title', 'body', 'action']) {
      assert.ok(en[`pnow.${kind}.${part}`], `English ${kind}.${part}`);
      assert.ok(tr[`pnow.${kind}.${part}`], `Turkish ${kind}.${part}`);
    }
  }
  for (const kind of ['cron_owner_removed', 'linger_owner_removed']) assert.ok(en[`pnow.${kind}.confirm`] && tr[`pnow.${kind}.confirm`]);
  assert.match(problems, /const RECLOSE = \['at', 'stock_pool', 'cron', 'linger'\];/);
  assert.match(problems, /problem\.action\.kind === 'move_all' \?/);
  assert.match(en['pnow.site_lingering.body'], /\{other\}/, 'the lingering line gives the command');
  // Review N4: the held line says the tasks do not run while the file is missing (also after a
  // restart) and gives the reload a file put back by hand needs.
  assert.match(en['pnow.site_tasks_held.body'], /daemon-reload/);
  assert.match(tr['pnow.site_tasks_held.body'], /daemon-reload/);
  assert.match(en['pnow.site_move_kept.body'], /scheduled tasks/, "the kept line covers a site's scheduled tasks too");
  assert.match(tr['pnow.site_move_kept.body'], /zamanlanmış görevleri/);
  assert.match(en['pnow.site_user_manager_open.body'], /systemctl mask \{other\}/);
});

test('second fix round: what the screens say about a skipped run, a timer not on yet, a crontab back and the mail file', () => {
  assert.equal(decodeTaskRun({ state: 'skipped', started_at: '2026-11-23T10:00:00Z' }).state, 'skipped');
  for (const key of ['stasks.run.skipped', 'stasks.state.not_on_yet', 'stasks.source.crontab', 'slim.tasks.unresolved', 'slim.tasks.mailOwnerFile', 'moveall.unresolved']) {
    assert.ok(enSiteTasksScreens[key] && trSiteTasksScreens[key], key);
  }
  assert.doesNotMatch(enSiteTasksScreens['stasks.run.skipped'], /fail/i, 'a skipped run is not called failed');
  assert.match(enSiteTasksScreens['stasks.differences'], /sendmail/, 'the differences name the server\'s sendmail');
  assert.equal(decodeTasksPlacement({ moved: true }).moved, true);
  // alpha.150: a kept tasks record is on the card's kept block, with the two steps by hand.
  assert.equal(unresolvedSteps({ tasks_site: 7, tasks_account: 'shop_test' }),
    "sudo systemctl disable --now 'celikcron-7-*.timer' ; sudo crontab -u shop_test /etc/celikpanel-cron/7.crontab");
  const cron = read('../src/components/DomainCronManager.tsx');
  assert.match(cron, /'crontab_again'/);
  assert.match(cron, /preview\.refusal === 'crontab_exists'/, 'the rebuild is refused while the account has a crontab');
  assert.match(cron, /t\('stasks\.run\.skipped'/);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /view\.move_interrupted\.tasks && \(/, 'the kept block names a tasks record');
  assert.match(card, /t\('slim\.tasks\.unresolved', \{ step: view\.move_interrupted\.tasks_step/);
  assert.match(card, /'owner_file'/);
});

test('the part is registered and budgeted', () => {
  assert.match(read('../src/i18n/screenParts.ts'), /screens\/siteTasks\/en/);
  assert.match(read('../src/i18n/screenParts.ts'), /screens\/siteTasks\/tr/);
  assert.match(read('../scripts/check-bundle-budget.mjs'), /siteTasks: kib\(/);
  assert.match(read('./locale-catalogue.mjs'), /'siteTasks'/);
});
