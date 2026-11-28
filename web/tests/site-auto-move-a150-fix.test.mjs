// alpha.150 automatic move, fix round after its independent review: no site
// outside its limits without a line (G2), the panel's update stays possible
// during a long run (G3), the card and the calm line tell the truth (G5).
// Otomatik taşımanın düzeltme turu: satırsız dışarıda site yok, uzun turda
// güncelleme yapılabilir, kart ve sakin satır doğruyu söyler.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { autoMoveLeadKey, autoMoveLineKey, autoMoveReasonKey } from '../src/i18n/siteLimitsLane.ts';
import { enSiteTasksScreens } from '../src/i18n/screens/siteTasks/en.ts';
import { trSiteTasksScreens } from '../src/i18n/screens/siteTasks/tr.ts';
import { enErrorsScreens } from '../src/i18n/screens/errors/en.ts';
import { trErrorsScreens } from '../src/i18n/screens/errors/tr.ts';
import { decodeSiteLimits, followsAutoMove } from '../src/lib/siteLimits.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const words = { ...enSiteTasksScreens, ...enSiteLimitsLane };
const trWords = { ...trSiteTasksScreens, ...trSiteLimitsLane };

test('the words of an ending say only what is known (G5)', () => {
  for (const [mark, key] of [
    // "put back as it was" only where the programs' part put it back and the tasks changed nothing.
    [{ state: 'failed', reason: 'not_listening', put_back: true }, 'slim.auto.why.failed'],
    [{ state: 'failed', reason: 'not_listening', put_back: true, part: 'tasks_moved' }, 'slim.auto.why.programsBack'],
    [{ state: 'failed', reason: 'prepare_failed' }, 'slim.auto.why.notFinished'],
    [{ state: 'failed', reason: 'not_listening' }, 'slim.auto.why.notFinished'],
    // The tasks' part: a refusal after nothing changed, and a put-back into cron.
    [{ state: 'failed', reason: 'tasks_refused', part: 'tasks' }, 'slim.auto.why.tasksRefused'],
    [{ state: 'failed', reason: 'something new', part: 'tasks' }, 'slim.auto.why.tasksRefused'],
    [{ state: 'failed', reason: 'crontab_changed', part: 'tasks', put_back: true }, 'slim.tasks.why.put_back'],
    // The panel's own record is not the server being busy.
    [{ state: 'failed', reason: 'marks_unreadable' }, 'slim.auto.why.record'],
    [{ state: 'failed', reason: 'mark_failed' }, 'slim.auto.why.record'],
    [{ state: 'failed', reason: 'host_busy' }, 'slim.auto.why.waited'],
    // G2: outside while no run moves it.
    [{ state: 'outside', reason: 'outside_again' }, 'slim.auto.why.outside_again'],
    [{ state: 'outside' }, 'slim.auto.why.outside'],
    [{ state: 'unknown', reason: 'unsettled' }, 'slim.auto.why.unknown'],
  ]) {
    assert.equal(autoMoveReasonKey(mark), key, JSON.stringify(mark));
    assert.ok(words[key] && trWords[key], `${key} has words in both languages`);
  }
  assert.doesNotMatch(enSiteLimitsLane['slim.auto.why.record'], /server stayed busy/);
  assert.doesNotMatch(enSiteLimitsLane['slim.auto.why.unknown'], /the panel restarted during it/, 'never claims a restart it does not know');
  assert.match(enSiteLimitsLane['slim.auto.why.failed'], /put back as it was/);
  assert.doesNotMatch(enSiteLimitsLane['slim.auto.why.notFinished'], /put back as it was/);
  assert.equal(autoMoveLeadKey({ state: 'outside' }), 'slim.auto.outside');
  assert.equal(autoMoveLeadKey({ state: 'failed' }), 'slim.auto.notMoved');
  assert.doesNotMatch(enSiteLimitsLane['slim.auto.outside'], /could not move/);
});

test('the calm line says when the run waits and why, and "nothing to do" only where true (G5)', () => {
  assert.equal(autoMoveLineKey(undefined), 'slim.auto.running');
  assert.equal(autoMoveLineKey('nonsense'), 'slim.auto.running');
  for (const why of ['update', 'wanted', 'host', 'move', 'site', 'record']) {
    const key = autoMoveLineKey(why);
    assert.equal(key, `slim.auto.wait.${why}`);
    for (const lane of [enSiteLimitsLane, trSiteLimitsLane]) {
      assert.ok(lane[key], `${key} in both languages`);
      assert.match(lane[key], /\{done\} \/ \{total\}/);
    }
    assert.doesNotMatch(enSiteLimitsLane[key], /being moved/, `${key} does not say "being moved"`);
  }
  for (const why of ['update', 'host', 'move', 'site']) {
    assert.match(enSiteLimitsLane[`slim.auto.wait.${why}`], /you do not need to do anything/);
    assert.match(trSiteLimitsLane[`slim.auto.wait.${why}`], /bir şey yapmanız gerekmiyor/);
  }
  for (const why of ['wanted', 'record']) {
    assert.doesNotMatch(enSiteLimitsLane[`slim.auto.wait.${why}`], /you do not need to do anything/);
    assert.doesNotMatch(trSiteLimitsLane[`slim.auto.wait.${why}`], /bir şey yapmanız gerekmiyor/);
  }
  assert.match(enSiteLimitsLane['slim.auto.wait.wanted'], /30 minutes[\s\S]*Start the update again now/);
  assert.match(trSiteLimitsLane['slim.auto.wait.wanted'], /30 dakika[\s\S]*yeniden başlatın/);
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.match(problems, /waiting: typeof p\.waiting === 'string' \? p\.waiting : undefined/);
});

test('the card follows the run by itself and stops when there is nothing to follow (G5)', () => {
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /const follow = followsAutoMove\(shown\);/);
  assert.match(card, /useEffect\(\(\) => \(follow \? startVisibleReads\(reread, 60_000,/);
  const base = { known: true, admin: true, limits: [], events: [] };
  assert.equal(followsAutoMove(decodeSiteLimits({ ...base, auto_move: { done: 1, total: 4 } })), true, 'the worker has work');
  assert.equal(followsAutoMove(decodeSiteLimits({ ...base, applies: 'outside' })), true, 'the site is outside');
  assert.equal(followsAutoMove(decodeSiteLimits({ ...base, needs_move: 2 })), true);
  assert.equal(followsAutoMove(decodeSiteLimits({ ...base, applies: 'yes', tasks: { moved: true } })), false, 'nothing to follow');
  assert.equal(followsAutoMove(decodeSiteLimits({ ...base, admin: false, applies: 'outside' })), false, 'only the administrator follows the worker');
  const view = decodeSiteLimits({ ...base, auto_move: { done: 1, total: 4, waiting: 'host', offer: 'retry', state: 'outside', reason: 'outside_again', part: 'tasks', put_back: true } });
  assert.equal(view.auto_move?.waiting, 'host');
  assert.equal(view.auto_move?.part, 'tasks');
  assert.equal(view.auto_move?.put_back, true);
});

test('Problems now: a site outside again, and the sites that could not be read (G2)', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.match(problems, /'site_auto_move_unread'/);
  assert.match(problems, /problem\.kind === 'site_auto_move_failed' && problem\.name === 'outside'\) return 'pnow\.site_auto_move_failed\.body_outside'/);
  assert.match(problems, /problem\.kind === 'site_auto_move_unread' && problem\.reason === 'sites'\) return 'pnow\.site_auto_move_unread\.body_sites'/);
  assert.match(problems, /part: problem\.part, put_back: problem\.put_back/);
  for (const lane of [enSiteLimitsLane, trSiteLimitsLane]) {
    for (const key of ['pnow.site_auto_move_failed.body_outside', 'pnow.site_auto_move_unread.title', 'pnow.site_auto_move_unread.body', 'pnow.site_auto_move_unread.body_sites', 'pnow.site_auto_move_unread.action']) {
      assert.ok(lane[key], key);
    }
  }
  // Who acts, what, and that it goes on by itself.
  assert.match(enSiteLimitsLane['pnow.site_auto_move_unread.body'], /did not answer[\s\S]*sudo systemctl status celikpanel-agent[\s\S]*goes on by itself/);
  assert.match(trSiteLimitsLane['pnow.site_auto_move_unread.body'], /yanıt vermedi[\s\S]*sudo systemctl status celikpanel-agent[\s\S]*kendiliğinden devam eder/);
  // The words stay out of the boot catalogue.
  assert.doesNotMatch(read('../src/i18n/en.ts'), /site_auto_move_unread|slim\.auto\.wait/);
});

test('the update refused during the worker’s move says when it ends and what to do (G3)', () => {
  assert.match(enErrorsScreens['err.PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING'], /One site[\s\S]*within about 12 minutes[\s\S]*no other site meanwhile[\s\S]*Start the update again then/);
  assert.match(trErrorsScreens['err.PANEL_UPDATE_SITE_AUTO_MOVE_RUNNING'], /bir site[\s\S]*12 dakika[\s\S]*başka site başlatmaz[\s\S]*yeniden başlatın/);
  assert.match(enErrorsScreens['err.PANEL_UPDATE_SITE_MOVE_RUNNING'], /Wait until it has finished/, 'an administrator’s own press keeps its words');
});

test('the update gate’s unreadable folder is not promised to end in 31 minutes (G1)', () => {
  assert.match(enSiteLimitsLane['slim.why.update_unreadable'], /request’s record[\s\S]*31 minutes[\s\S]*folder, once it can be read/);
  assert.match(trSiteLimitsLane['slim.why.update_unreadable'], /31 dakika[\s\S]*klasörü ise okunabildiğinde/);
});
