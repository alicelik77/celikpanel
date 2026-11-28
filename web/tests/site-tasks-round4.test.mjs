// alpha.150 fix round 4 (fourth review of R2b, N1-N9): what the site's card
// says about a kept tasks record, in both languages alike.
// Tutulan görev kaydı için kartın sözleri, iki dilde aynı.
import assert from 'node:assert/strict';
import test from 'node:test';
import { enSiteTasksScreens } from '../src/i18n/screens/siteTasks/en.ts';
import { trSiteTasksScreens } from '../src/i18n/screens/siteTasks/tr.ts';
import { enSiteTasksProblemsScreens } from '../src/i18n/screens/siteTasksProblems/en.ts';
import { trSiteTasksProblemsScreens } from '../src/i18n/screens/siteTasksProblems/tr.ts';
import { keptCronBackKey, unresolvedSteps } from '../src/lib/siteTasks.ts';

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

test('N4, N5: the way back by hand never overwrites a crontab and can be run again after a stop', () => {
  const steps = unresolvedSteps({ tasks_site: 7, tasks_account: 'shop_test' });
  const order = ['mv /etc/cron.allow.celikpanel-before /etc/cron.allow; }', 'sudo test ! -e /var/spool/cron/crontabs/shop_test',
    'sudo test ! -e /var/spool/cron/shop_test', 'systemctl disable --now', 'cp -p /etc/cron.allow', 'tee -a /etc/cron.allow', 'crontab -u shop_test'];
  let at = -1;
  for (const part of order) {
    const next = steps.indexOf(part);
    assert.ok(next > at, `${part} comes after the step before it`);
    at = next;
  }
  // The card says what the steps bring back and that they stop at a crontab.
  assert.match(en['slim.tasks.unresolved'], /as it is now/);
  assert.match(en['slim.tasks.unresolved'], /switched off on the server comes back as the task file has it/);
  assert.match(en['slim.tasks.unresolved'], /stops, changing nothing, while the account has a crontab/);
  assert.match(tr['slim.tasks.unresolved'], /şimdiki hâliyle/);
  assert.match(tr['slim.tasks.unresolved'], /sunucuda kapatılmış bir görev görev dosyasındaki gibi geri gelir/);
  assert.match(tr['slim.tasks.unresolved'], /crontab’ı varken hiçbir şeyi değiştirmeden durur/);
});

test('N3: a crontab that is not the recorded one: the card says both ways out, in both languages', () => {
  assert.equal(keptCronBackKey('crontab_emptied'), 'slim.tasks.cronBack');
  assert.equal(keptCronBackKey('enabled'), 'slim.tasks.cronBack');
  // At every other step Try again finishes whatever the crontab holds (put-back, undo).
  for (const step of ['source_written', 'units_written', 'cron_closed', 'crontab_written', '', undefined]) {
    assert.equal(keptCronBackKey(step), 'slim.tasks.cronBackOther', String(step));
  }
  for (const words of [en, tr]) {
    assert.ok(words['slim.tasks.cronBackOther'], 'slim.tasks.cronBackOther');
    for (const placeholder of ['{site}', '{account}', '{steps}']) assert.ok(words['slim.tasks.cronBack'].includes(placeholder), placeholder);
    assert.match(words['slim.tasks.cronBack'], /sudo rm \/var\/spool\/cron\/crontabs\/\{account\}/);
    assert.match(words['slim.tasks.cronBack'], /\{steps\}[^]*(Try again|Tekrar dene)/);
  }
  assert.match(en['slim.tasks.cronBack'], /To keep the timers[^]*switches them on again[^]*To keep cron with the recorded tasks/);
  assert.match(tr['slim.tasks.cronBack'], /Zamanlayıcıları tutmak için[^]*yeniden açar[^]*Kayıttaki görevlerle cron’u tutmak için/);
});

test('N9: the rebuild refused for a kept record says a move on this server, which may be another site’s', () => {
  assert.match(en['stasks.rebuild.kept'], /A move on this server into a site’s limits/);
  assert.doesNotMatch(en['stasks.rebuild.kept'], /of this site/);
  assert.match(tr['stasks.rebuild.kept'], /Bu sunucuda bir sitenin/);
  assert.doesNotMatch(tr['stasks.rebuild.kept'], /Bu sitenin/);
});
