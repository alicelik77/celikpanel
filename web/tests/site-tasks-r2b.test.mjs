// S4 R2b (22 Nov 2026): a site's scheduled tasks as timers inside its own
// limits. The Scheduled tasks screen says each task's last run and the two
// halves of what a long run does; the administrator's move takes the site's
// crontab too (preview first) and can be undone (alpha.150: the panel moves
// every site by itself, D-048, and "Move all" is gone); the Problems now lines have words in both
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
  decodeTaskRun, decodeTasksAnswer, decodeTasksPlacement, lineList, movedTaskCount, privilegedList, unresolvedSteps,
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

test('the Problems now lines are known kinds with words, and the two re-closes are offered', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  const list = problems.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  // Second fix round: the held, lingering and service-manager lines live in the "problemsNow"
  // part (the "siteTasks" part had no room left); either part may hold them. A move that could
  // not be settled is main's site_move_kept line (alpha.150: one record, one line).
  const en = { ...enSiteTasksScreens, ...enProblemsNowScreens };
  const tr = { ...trSiteTasksScreens, ...trProblemsNowScreens };
  for (const kind of ['cron_allow_lists_sites', 'cron_not_closed', 'cron_owner_removed', 'site_crontab_returned', 'site_lingering', 'linger_owner_removed', 'linger_not_closed', 'site_crontab_by_site', 'site_tasks_held',
    'site_move_kept', 'site_user_manager_open']) {
    assert.ok(list && list[1].includes(`'${kind}'`), `${kind} is a known kind`);
    for (const part of ['title', 'body', 'action']) {
      assert.ok(en[`pnow.${kind}.${part}`], `English ${kind}.${part}`);
      assert.ok(tr[`pnow.${kind}.${part}`], `Turkish ${kind}.${part}`);
    }
  }
  for (const kind of ['cron_owner_removed', 'linger_owner_removed']) assert.ok(en[`pnow.${kind}.confirm`] && tr[`pnow.${kind}.confirm`]);
  assert.match(problems, /const RECLOSE = \['at', 'stock_pool', 'cron', 'linger'\];/);
  // alpha.150 (D-048): no "Move all"; a site the panel could not move by itself has Try again.
  assert.doesNotMatch(problems, /move_all|MoveAllSites/);
  assert.match(problems, /problem\.action\.kind === 'retry_move' \?/);
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
  for (const key of ['stasks.run.skipped', 'stasks.state.not_on_yet', 'stasks.source.crontab', 'slim.tasks.unresolved', 'slim.tasks.mailOwnerFile']) {
    assert.ok(enSiteTasksScreens[key] && trSiteTasksScreens[key], key);
  }
  assert.doesNotMatch(enSiteTasksScreens['stasks.run.skipped'], /fail/i, 'a skipped run is not called failed');
  assert.match(enSiteTasksScreens['stasks.differences'], /sendmail/, 'the differences name the server\'s sendmail');
  assert.equal(decodeTasksPlacement({ moved: true }).moved, true);
  // alpha.150: a kept tasks record is on the card's kept block, with the steps by hand
  // (fix round F1: the exact line run on systemd 257 with throw-away units; the timers listed
  // by name, never a pattern; joined with && so a failed step stops the rest; cron.allow put
  // back byte for byte also when the install fails). Fix round 4 (N4, N5): first a backup an
  // earlier, stopped run left is put back, then it stops while the account has a crontab.
  assert.equal(unresolvedSteps({ tasks_site: 7, tasks_account: 'shop_test' }),
    '{ [ ! -e /etc/cron.allow.celikpanel-before ] || sudo mv /etc/cron.allow.celikpanel-before /etc/cron.allow; }'
    + " && { sudo test ! -e /var/spool/cron/crontabs/shop_test && sudo test ! -e /var/spool/cron/shop_test || { echo 'Stopped, nothing changed: shop_test has a crontab. Press Try again.' >&2; false; }; }"
    + " && sudo systemctl disable --now $(systemctl list-unit-files --no-legend 'celikcron-7-*.timer' | awk '{print $1}')"
    + ' && sudo cp -p /etc/cron.allow /etc/cron.allow.celikpanel-before && echo shop_test | sudo tee -a /etc/cron.allow >/dev/null'
    + ' && { sudo crontab -u shop_test /etc/celikpanel-cron/7.crontab; ok=$?; sudo mv /etc/cron.allow.celikpanel-before /etc/cron.allow && [ $ok = 0 ]; }');
  assert.doesNotMatch(unresolvedSteps({ tasks_site: 7, tasks_account: 'shop_test' }), /disable --now '|;\s*sudo crontab/, 'no pattern for disable, no step after a failed one');
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) {
    assert.match(words['slim.tasks.unresolved'], /\{steps\}[^]*(Try again|Tekrar dene)/, 'the last words after the steps ask for Try again');
    assert.ok(words['slim.tasks.cronBack'] && words['slim.tasks.cronBack'].includes('{site}'));
  }
  // Fix round 4 (N3): with a crontab there the card says the way out; its steps start with the
  // guard that stops them while a crontab is there (N4), so they never overwrite it.
  assert.match(read('../src/components/SiteLimitsCard.tsx'), /tasks_running === 'cron'\s*\/\/[^\n]*\n\s*\?\s*t\(keptCronBackKey\(view\.move_interrupted\.tasks_step\)/, 'with a crontab there the card says the way out');
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
