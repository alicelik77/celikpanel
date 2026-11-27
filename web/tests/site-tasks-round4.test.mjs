// alpha.150 fix round 4 (fourth review of R2b, N1-N9): what the site's card
// says about a kept tasks record, in both languages alike.
// Tutulan görev kaydı için kartın sözleri, iki dilde aynı.
import assert from 'node:assert/strict';
import test from 'node:test';
import { enSiteTasksScreens } from '../src/i18n/screens/siteTasks/en.ts';
import { trSiteTasksScreens } from '../src/i18n/screens/siteTasks/tr.ts';
import { enSiteTasksProblemsScreens } from '../src/i18n/screens/siteTasksProblems/en.ts';
import { trSiteTasksProblemsScreens } from '../src/i18n/screens/siteTasksProblems/tr.ts';

const en = { ...enSiteTasksScreens, ...enSiteTasksProblemsScreens };
const tr = { ...trSiteTasksScreens, ...trSiteTasksProblemsScreens };

test('N6: the Turkish words of a kept tasks record say everything the English says', () => {
  // slim.tasks.unresolved: the two steps, what Try again finds and does, and why it matters.
  assert.match(en['slim.tasks.unresolved'], /the timers off, then the task file/);
  assert.match(tr['slim.tasks.unresolved'], /önce zamanlayıcılar kapanır, sonra görev dosyası/);
  assert.match(en['slim.tasks.unresolved'], /finds the crontab back/);
  assert.match(tr['slim.tasks.unresolved'], /crontab’ı geri bulur/);
  assert.match(en['slim.tasks.unresolved'], /so sites can be moved again/);
  assert.match(tr['slim.tasks.unresolved'], /siteler yeniden taşınabilir/);
  assert.match(en['slim.tasks.unresolved'], /next start/);
  assert.match(tr['slim.tasks.unresolved'], /bir sonraki açılışında/);
  // slim.tasks.cronBack: Try again removes the timer files left.
  assert.match(en['slim.tasks.cronBack'], /removes the timer files left/);
  assert.match(tr['slim.tasks.cronBack'], /kalan zamanlayıcı dosyalarını kaldırır/);
  // The way back by hand is kept in the part of the kept record's Problems now line.
  assert.ok(enSiteTasksProblemsScreens['slim.tasks.unresolved'] && trSiteTasksProblemsScreens['slim.tasks.unresolved']);
  assert.ok(!('slim.tasks.unresolved' in enSiteTasksScreens) && !('slim.tasks.unresolved' in trSiteTasksScreens));
});
