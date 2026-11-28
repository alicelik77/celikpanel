// alpha.150 (D-048, the owner's order of 27 November 2026): every site goes
// inside its limits by itself. The screens offer no "Move into the limits"
// and no "Move all" in any healthy state; the move stays only as Try again
// next to a move that did not bring a site inside, and for a site whose tasks
// were put back into cron on purpose. While the panel moves sites, one calm
// line says so (Home, Problems now, the site's card); its words live in the
// lazily loaded lane, never on the boot path.
// Her site kendiliğinden sınırlarının içine girer; ekranlarda taşıma düğmesi
// yalnız bir hatanın yanında kalır.
import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { autoMoveReasonKey } from '../src/i18n/siteLimitsLane.ts';
import { enSiteTasksScreens } from '../src/i18n/screens/siteTasks/en.ts';
import { trSiteTasksScreens } from '../src/i18n/screens/siteTasks/tr.ts';
import { enSiteTasksProblemsScreens } from '../src/i18n/screens/siteTasksProblems/en.ts';
import { trSiteTasksProblemsScreens } from '../src/i18n/screens/siteTasksProblems/tr.ts';
import { enSiteLimitsApplyScreens as enSiteLimitsScreens } from '../src/i18n/screens/siteLimitsApply/en.ts';
import { trSiteLimitsApplyScreens as trSiteLimitsScreens } from '../src/i18n/screens/siteLimitsApply/tr.ts';
import { decodeSiteLimits } from '../src/lib/siteLimits.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('no move action in any healthy state: only Try again next to a failure and the move of a site put back on purpose', () => {
  const card = read('../src/components/SiteLimitsCard.tsx');
  // The box of alpha.148/149 that offered the move whenever something was outside is gone.
  assert.doesNotMatch(card, /\(view\.needs_move \?\? 0\) > 0 \|\| tasksToMove\) && view\.applies/);
  // The two places the move stays, each beside its reason.
  assert.match(card, /offer === 'retry' && auto && \([\s\S]*?t\(autoMoveLeadKey\(auto\) as SiteLimitsLaneKey\)[\s\S]*?autoMoveReasonKey\(auto\)[\s\S]*?t\('pnow\.retry'\)/);
  assert.match(card, /offer === 'move' && \([\s\S]*?t\('slim\.auto\.putBack'\)[\s\S]*?t\('slim\.move\.button'\)/);
  // The button's words appear nowhere else on the card.
  assert.equal(card.match(/t\('slim\.move\.button'\)/g)?.length, 1, 'the move button is only that of a site put back on purpose');
  // A kept record's own Try again comes first; no move is offered beside it.
  assert.match(card, /const offer = view\.applies !== 'unsupported' && !view\.move_interrupted\?\.kept \? auto\?\.offer : undefined;/);
  // "Move all" is gone: its component, its list, its words and its line.
  assert.ok(!existsSync(new URL('../src/components/MoveAllSites.tsx', import.meta.url)), 'MoveAllSites.tsx is removed');
  const tasks = read('../src/lib/siteTasks.ts');
  assert.doesNotMatch(tasks, /moveSitesInTurn|readOutsideSites|decodeOutsideSites|site-limits\/outside|siteMoveEnd/);
  for (const words of [enSiteTasksProblemsScreens, trSiteTasksProblemsScreens, enSiteTasksScreens, trSiteTasksScreens, enSiteLimitsLane, trSiteLimitsLane]) {
    for (const key of Object.keys(words)) assert.ok(!/^moveall\.|^pnow\.site_limits_outside\./.test(key), `${key} is gone`);
  }
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.doesNotMatch(problems, /move_all|MoveAllSites|'site_limits_outside'|'site_programs_outside'/);
  // Problems now's Try again is the site's own move, fetched with the card's module on the press.
  assert.match(problems, /p\.action\.kind === 'retry_move' \? p\.kind === 'site_auto_move_failed' && typeof p\.domain_id === 'number'/);
  assert.match(problems, /const \{ retrySiteMove \} = await import\('\.\/SiteLimitsCard'\);/);
  assert.match(card, /export async function retrySiteMove\(domainId: number, t: SiteLimitsLaneT\)[\s\S]*?readRemote\(`\/api\/v1\/domains\/\$\{domainId\}\/site-limits`/);
  // The words of a healthy site no longer tell anyone to press a move.
  for (const words of [enSiteTasksScreens, trSiteTasksScreens]) {
    for (const [key, text] of Object.entries(words)) {
      assert.ok(!/press the move|move can be pressed|taşımayı yeniden başlat|taşımaya yeniden bas|taşıma yeniden başlatılabilir/i.test(text), `${key}: ${text}`);
    }
  }
});

test('one calm line while the panel moves sites, the owner’s words, in both languages, the same placeholders', () => {
  assert.equal(enSiteLimitsLane['slim.auto.running'], 'Sites are being moved into their limits ({done} / {total}) — you do not need to do anything.');
  assert.equal(trSiteLimitsLane['slim.auto.running'], 'Siteler sınırlarının içine alınıyor ({done} / {total}) — bir şey yapmanız gerekmiyor.');
  for (const [key, en] of Object.entries(enSiteLimitsLane)) {
    if (!key.startsWith('slim.auto.') && !key.startsWith('pnow.site_auto_move_failed.')) continue;
    const want = [...en.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    const got = [...trSiteLimitsLane[key].matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    assert.deepEqual(got, want, `${key} keeps its placeholders`);
  }
  const problems = read('../src/components/ProblemsNow.tsx');
  // Read with the list (it refreshes by itself), shown even when nothing else is, also on Home (quiet).
  assert.match(problems, /site_move_progress: decodeMoveProgress\(body\.site_move_progress\)/);
  assert.match(problems, /if \(moving\) return <div className="mb-4">\{moving\}<\/div>;/);
  assert.match(problems, /if \(problems\.length === 0 && quiet && !outcome\) return moving \?/);
  assert.match(problems, /t\(autoMoveLineKey\(progress\.waiting\) as TranslationKey, \{ done: progress\.done, total: progress\.total \}\)/);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /\(auto\?\.total \?\? 0\) > 0 && \([\s\S]*?t\(autoMoveLineKey\(auto\?\.waiting\)/);
  // Only the administrator's part of the card reads the worker (the server sends it to the administrator only).
  assert.match(card, /\{view\.admin && <AdminPart/);
  // The words are in the lazily loaded lane, never in the shell catalogue on the boot path.
  const shell = read('../src/i18n/en.ts');
  assert.doesNotMatch(shell, /slim\.auto\.|site_auto_move_failed/);
});

test('why a site was not moved by itself is a code worded by the page, never the agent’s English', () => {
  const words = { ...enSiteLimitsScreens, ...enSiteTasksScreens, ...enSiteLimitsLane };
  const trWords = { ...trSiteLimitsScreens, ...trSiteTasksScreens, ...trSiteLimitsLane };
  for (const [mark, key] of [
    [{ state: 'time_up', reason: 'time_up' }, 'slim.move.timeUp'],
    [{ state: 'unknown', reason: 'panel_restarted' }, 'slim.auto.why.unknown'],
    [{ state: 'kept', reason: 'not_running' }, 'slim.auto.why.kept'],
    [{ state: 'left', reason: 'stayed' }, 'slim.auto.why.stayed'],
    [{ state: 'left', reason: 'cron_allow_owner' }, 'slim.tasks.why.cron_allow_owner'],
    [{ state: 'failed', reason: 'limits' }, 'slim.tasks.why.limits'],
    [{ state: 'left', reason: 'no_cron' }, 'slim.tasks.why.no_cron'],
    [{ state: 'failed', reason: 'processes_unread' }, 'slim.tasks.why.processes_unread'],
    [{ state: 'failed', reason: 'tasks_put_back' }, 'slim.tasks.why.put_back'],
    [{ state: 'failed', reason: 'not_listening', put_back: true }, 'slim.auto.why.failed'],
    [{ state: 'failed', reason: 'host_busy' }, 'slim.auto.why.waited'],
    [{ state: 'failed', reason: 'update_running' }, 'slim.auto.why.waited'],
    [{ state: 'failed', reason: 'limits_unread' }, 'slim.auto.why.limits_unread'],
    [{ state: 'failed', reason: 'something new', put_back: true }, 'slim.auto.why.failed'],
  ]) {
    assert.equal(autoMoveReasonKey(mark), key, JSON.stringify(mark));
    assert.ok(words[key] && trWords[key], `${key} has words in both languages`);
  }
  for (const lane of [enSiteLimitsLane, trSiteLimitsLane]) {
    assert.match(lane['pnow.site_auto_move_failed.title'], /\{domain\}/);
  }
  assert.match(enSiteLimitsLane['pnow.site_auto_move_failed.action'], /^Try again$/);
  assert.match(trSiteLimitsLane['pnow.site_auto_move_failed.action'], /^Tekrar dene$/, 'the same words as every other Try again');
});

test('the card reads the worker’s part through its decoder', () => {
  const view = decodeSiteLimits({ known: true, admin: true, limits: [], events: [], auto_move: { done: 3, total: 12, offer: 'retry', state: 'failed', reason: 'not_listening' } });
  assert.deepEqual(view.auto_move, { done: 3, total: 12, offer: 'retry', state: 'failed', reason: 'not_listening', waiting: undefined, part: undefined, put_back: false });
  assert.equal(decodeSiteLimits({ known: true, admin: true, limits: [], events: [], auto_move: { offer: 'move all', done: -1 } }).auto_move?.offer, undefined, 'an offer it does not know is none');
  assert.equal(decodeSiteLimits({ known: true, admin: false, limits: [], events: [] }).auto_move, undefined);
});
