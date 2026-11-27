// alpha.150 (S4 R2b on top of alpha.149's one move record): a tasks answer
// is a code the page words, never the agent's English; a kept tasks record
// says how the site's scheduled tasks run now, on the card and in Problems
// now, instead of "may"; the rebuild of a task file words the update's
// refusal.
// Görev yanıtı ajanın İngilizcesiyle değil sayfanın sözleriyle; tutulan
// kayıt görevlerin şu an nasıl çalıştığını söyler.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteTasksScreens } from '../src/i18n/screens/siteTasks/en.ts';
import { trSiteTasksScreens } from '../src/i18n/screens/siteTasks/tr.ts';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { decodeTasksAnswer, keptTasksRunningKey, moveSitesInTurn, siteMoveEnd, tasksAnswerKey } from '../src/lib/siteTasks.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const answer = (outcome, refusal) => ({ outcome, refusal, rows: [], blocking: [], mail: false, error: 'english text of the agent' });

test('a tasks answer is a code worded by the page, in both languages, never the agent’s English', () => {
  const cases = [
    [answer('refused', 'kept_earlier'), false, 'slim.why.kept_earlier'],
    [answer('refused', 'update_running'), true, 'slim.why.update_running'],
    [answer('refused', 'update_unreadable'), false, 'slim.why.update_unreadable'],
    [answer('refused', 'another_move'), false, 'slim.why.another_move'],
    [answer('refused', 'site_busy'), true, 'slim.why.site_busy'],
    [answer('refused', 'cron_allow_owner'), false, 'slim.tasks.why.cron_allow_owner'],
    [answer('refused', 'cron_allow_owner'), true, 'slim.tasks.undoWhy.cron_allow_owner'],
    [answer('refused', 'source_owner'), true, 'slim.tasks.undoWhy.source_owner'],
    [answer('put_back', 'crontab_changed'), false, 'slim.tasks.why.crontab_changed'],
    [answer('refused', 'crontab_changed'), true, 'slim.tasks.undoWhy.crontab_changed'],
    [answer('refused', 'limits'), false, 'slim.tasks.why.limits'],
    [answer('put_back'), false, 'slim.tasks.why.put_back'],
    [answer('no_cron'), false, 'slim.tasks.why.no_cron'],
    [answer('crontab_back'), false, 'slim.tasks.done.back'],
    [answer('refused', 'something new'), false, 'slim.tasks.why.other'],
    [answer('refused'), true, 'slim.tasks.undoWhy.other'],
    // Fix round F6: the undo cannot read the timers' state.
    [answer('refused', 'timers_unread'), true, 'slim.tasks.undoWhy.timers_unread'],
    // Fix round 4 (N2): a process of the account whose program could not be told,
    // before anything changed and when the move was put back at cron_closed.
    [answer('refused', 'processes_unread'), false, 'slim.tasks.why.processes_unread'],
    [answer('put_back', 'processes_unread'), false, 'slim.tasks.why.processes_unread'],
  ];
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) {
    assert.match(words['slim.tasks.why.processes_unread'], /administrator|Yönetici/, 'it names who acts');
    assert.match(words['slim.tasks.why.processes_unread'], /ps -fu/, 'it says what to look at');
  }
  for (const [a, undo, key] of cases) {
    assert.equal(tasksAnswerKey(a, undo), key, `${a.outcome} ${a.refusal} ${undo}`);
    const lane = key.startsWith('slim.why.');
    for (const words of lane ? [enSiteLimitsLane, trSiteLimitsLane] : [enSiteTasksScreens, trSiteTasksScreens]) {
      assert.ok(words[key], `${key} has words`);
      assert.ok(!/\{detail\}/.test(words[key]), `${key} carries no text of the agent`);
    }
  }
  assert.equal(tasksAnswerKey(answer('unknown'), false), 'slim.tasks.why.unknown');
  assert.equal(tasksAnswerKey(answer('unknown'), true), 'slim.tasks.undoWhy.unknown');
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) assert.ok(words['slim.tasks.why.unknown'] && words['slim.tasks.undoWhy.unknown']);
  // The card, Move all and the undo never show a tasks answer's English.
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.ok(!/answer\.error|answer\?\.error/.test(card), 'the card shows no English text of the agent');
  const moveAll = read('../src/components/MoveAllSites.tsx');
  assert.ok(!/preview\.error/.test(moveAll), 'Move all words a tasks preview by its code');
  assert.match(moveAll, /tasksAnswerKey\(preview, false\)/);
});

test('a kept tasks record says how the tasks run now, when the agent knows it; otherwise the line keeps “may”', () => {
  for (const running of ['none', 'timers', 'cron']) {
    const key = keptTasksRunningKey(running);
    assert.equal(key, `slim.tasks.running.${running}`);
    for (const words of [enSiteTasksScreens, trSiteTasksScreens]) assert.ok(words[key], key);
    for (const words of [enSiteLimitsLane, trSiteLimitsLane]) {
      const line = words[`pnow.site_move_kept.body_tasks_${running}`];
      assert.ok(line, running);
      assert.ok(!/\bmay\b|olabilir/.test(line), `${running}: said plainly, not “may”: ${line}`);
    }
  }
  assert.equal(keptTasksRunningKey(''), '');
  assert.equal(keptTasksRunningKey('unknown'), '');
  assert.match(enSiteTasksScreens['slim.tasks.running.none'], /not running now/);
  assert.match(enSiteLimitsLane['pnow.site_move_kept.body_tasks_none'], /are not running.*Try again/);
  assert.match(trSiteLimitsLane['pnow.site_move_kept.body_tasks_none'], /çalışmıyor.*Tekrar dene/);
  // Problems now picks the line by the panel's reason; the programs' line comes first.
  const source = read('../src/components/ProblemsNow.tsx');
  const body = /export function problemBodyKey[\s\S]*?\n}\n/.exec(source)?.[0] ?? '';
  const problemBodyKey = new Function('MAIL_SIGNING_STATES', `return ${body.replace(/^export function problemBodyKey\(problem[^)]*\): TranslationKey/, 'function (problem)').replace(/ as TranslationKey/g, '')}`)([]);
  assert.equal(problemBodyKey({ kind: 'site_move_kept', reason: 'tasks_none' }), 'pnow.site_move_kept.body_tasks_none');
  assert.equal(problemBodyKey({ kind: 'site_move_kept', reason: 'tasks_cron' }), 'pnow.site_move_kept.body_tasks_cron');
  assert.equal(problemBodyKey({ kind: 'site_move_kept', reason: 'tasks_unknown' }), 'pnow.site_move_kept.body');
  assert.equal(problemBodyKey({ kind: 'site_move_kept', name: 'celikrt-5.service', reason: 'tasks_none' }), 'pnow.site_move_kept.body_stopped');
  // The card puts the known state before the steps.
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /keptTasksRunningKey\(view\.move_interrupted\.tasks_running\)/);
});

test('the rebuild of a task file words the update’s and the move’s refusal', () => {
  const screen = read('../src/components/DomainCronManager.tsx');
  assert.match(screen, /REBUILD_BUSY: readonly string\[\] = \['update_running', 'update_unreadable', 'another_move'\]/);
  assert.match(screen, /REBUILD_BUSY\.includes\(answer\.refusal \?\? ''\)\) return \{ state: 'blocked', reason: t\('stasks\.rebuild\.busy'\) \}/);
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) assert.ok(words['stasks.rebuild.busy']);
  // Fix round F7: refused while a move record of the site is kept.
  assert.match(screen, /preview\.refusal === 'kept_earlier'\) \{\s*showToast\('error', t\('stasks\.rebuild\.kept'\)\)/);
  assert.match(screen, /answer\.refusal === 'kept_earlier'\) return \{ state: 'blocked', reason: t\('stasks\.rebuild\.kept'\) \}/);
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) assert.ok(words['stasks.rebuild.kept']);
});

test('fix round F6: the undo names the tasks that came back paused, in both languages', () => {
  const undone = decodeTasksAnswer({ outcome: 'undone', paused: [{ line: 4, text: '17 3 * * * php report.php', reason: '', detail: 'celikcron-7-1.timer' }, { bad: true }] });
  assert.deepEqual(undone.paused, [{ line: 4, text: '17 3 * * * php report.php', reason: '', detail: 'celikcron-7-1.timer' }]);
  assert.deepEqual(decodeTasksAnswer({ outcome: 'undone' }).paused, []);
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) assert.ok(words['slim.tasks.undo.paused'], 'slim.tasks.undo.paused');
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /answer\.paused\.length > 0[\s\S]*t\('slim\.tasks\.undo\.paused'\)/);
});

test('fix round: Move all words every reason by its code, never the agent’s English', async () => {
  const english = 'english text of the agent';
  const word = (key) => `<${key}>`;
  const cases = [
    [{ done: false, kept: true, error: english, units: [] }, 'stop', '<slim.why.kept_earlier>'],
    [{ done: true, units: [], tasks: { outcome: 'refused', refusal: 'kept_earlier', error: english } }, 'stop', '<slim.why.kept_earlier>'],
    [{ done: false, rolled_back: true, error: english, reason: { code: 'update_running' }, tasks: { outcome: 'moved' } }, 'stop', '<slim.why.update_running>'],
    [{ done: false, error: english, tasks: { outcome: 'moved' } }, 'stop', '<slim.why.other>'],
    [{ done: true, error: english }, 'stop', '<slim.tasks.why.unknown>'],
    [{ done: true, units: [], tasks: { outcome: 'put_back', error: english } }, 'stop', '<slim.tasks.why.put_back>'],
    [{ done: true, units: [], tasks: { outcome: 'unknown', error: english } }, 'stop', '<slim.tasks.why.unknown>'],
    [{ done: true, units: [], tasks: { outcome: 'refused', refusal: 'cron_allow_owner', error: english } }, 'skipped', '<slim.tasks.why.cron_allow_owner>'],
    [{ done: true, units: [{ unit: 'celikapp-4.service' }], tasks: { outcome: 'no_cron', error: english } }, 'partial', '<slim.tasks.why.no_cron>'],
  ];
  for (const [answer, kind, reason] of cases) {
    const end = siteMoveEnd(answer, word);
    assert.equal(end.kind, kind, JSON.stringify(answer));
    assert.equal(end.reason, reason, JSON.stringify(answer));
  }
  assert.deepEqual(siteMoveEnd({ done: true, units: [], tasks: { outcome: 'crontab_back', error: english } }, word), { kind: 'moved', note: '<slim.tasks.done.back>' });
  const notes = [];
  const out = await moveSitesInTurn([{ domain_id: 1, domain: 'a.test' }], async () => ({ status: 200, body: { done: true, units: [], tasks: { outcome: 'put_back', error: english } } }),
    (_i, _state, note) => notes.push(note), word);
  assert.ok(!JSON.stringify([out, notes]).includes(english), 'no English of the agent reaches the summary or a step');
  assert.match(read('../src/components/MoveAllSites.tsx'), /\(key\) => t\(key as TranslationKey\)\);/);
});
