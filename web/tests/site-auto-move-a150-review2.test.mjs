// alpha.150 automatic move after its second review: the update's last step
// and an agent of another build have words of their own (N1); the panel's own
// record that cannot be read is one line (N3).
// İkinci incelemeden sonra: güncellemenin son adımı ve farklı derlemeli ajan
// kendi sözleriyle; okunamayan kendi kayıt tek satır.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { autoMoveLineKey, autoMoveReasonKey } from '../src/i18n/siteLimitsLane.ts';
import { moveWhyKey } from '../src/lib/siteLimits.ts';
import { tasksAnswerKey } from '../src/lib/siteTasks.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const recovery = /sudo journalctl -u celikpanel-release-recovery\.service --no-pager -n 50/;

test('the update’s last step: its own words on every screen that shows it (N1)', () => {
  const keys = [
    [autoMoveLineKey('finishing'), 'slim.auto.wait.finishing'],
    [autoMoveReasonKey({ state: 'failed', reason: 'update_finishing' }), 'slim.auto.why.finishing'],
    [moveWhyKey({ code: 'update_finishing' }), 'slim.why.update_finishing'],
    [tasksAnswerKey({ outcome: 'refused', refusal: 'update_finishing' }, false), 'slim.why.update_finishing'],
  ];
  for (const [got, want] of keys) {
    assert.equal(got, want);
    for (const lane of [enSiteLimitsLane, trSiteLimitsLane]) {
      assert.ok(lane[want], `${want} in both languages`);
      assert.match(lane[want], recovery, `${want} names the recovery journal`);
    }
    assert.match(enSiteLimitsLane[want], /installed[\s\S]*last step[\s\S]*certificate renewal \(Certbot\) back on/);
    assert.match(trSiteLimitsLane[want], /güncellemesi kuruldu|güncellemesi son adımını/);
    assert.match(trSiteLimitsLane[want], /sertifika yenilemeyi \(Certbot\) yeniden açma/);
    assert.doesNotMatch(enSiteLimitsLane[want], /being installed|you do not need to do anything/);
  }
  // Sites are moved by themselves only on the calm line, where the run still waits for it.
  assert.match(enSiteLimitsLane['slim.auto.wait.finishing'], /\{done\} \/ \{total\}[\s\S]*up to three times[\s\S]*moved by themselves once it is done/);
  assert.match(trSiteLimitsLane['slim.auto.wait.finishing'], /\{done\} \/ \{total\}[\s\S]*en çok üç kez[\s\S]*kendiliğinden taşınır/);
  assert.match(enSiteLimitsLane['slim.auto.why.finishing'], /press Try again/);
  // The other update refusals keep their words.
  assert.equal(autoMoveLineKey('update'), 'slim.auto.wait.update');
  assert.equal(autoMoveReasonKey({ state: 'failed', reason: 'update_running' }), 'slim.auto.why.waited');
  assert.equal(moveWhyKey({ code: 'update_running' }), 'slim.why.update_running');
  assert.match(read('../src/components/DomainCronManager.tsx'), /REBUILD_BUSY: readonly string\[\] = \[[^\]]*'update_finishing'/);
});

test('an agent of another build is not called "the update is being installed" (N1)', () => {
  for (const [got, want] of [
    [autoMoveLineKey('build'), 'slim.auto.wait.build'],
    [autoMoveReasonKey({ state: 'failed', reason: 'build' }), 'slim.auto.why.build'],
    [moveWhyKey({ code: 'build' }), 'slim.why.build'],
  ]) {
    assert.equal(got, want);
    for (const lane of [enSiteLimitsLane, trSiteLimitsLane]) assert.ok(lane[want], `${want} in both languages`);
    assert.match(enSiteLimitsLane[want], /different build from the panel[\s\S]*update again so that both match/);
    assert.match(trSiteLimitsLane[want], /farklı bir derlemeyle[\s\S]*güncellemesini yeniden çalıştır/);
  }
});

test('the panel’s own record that cannot be read is one line, not silence (N3)', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.match(problems, /problem\.kind === 'site_auto_move_unread' && problem\.reason === 'record'\) return 'pnow\.site_auto_move_unread\.body_record'/);
  assert.match(problems, /problem\.kind === 'site_auto_move_unread' && problem\.name === 'record'\) return 'pnow\.site_auto_move_unread\.title_record'/);
  for (const lane of [enSiteLimitsLane, trSiteLimitsLane]) {
    for (const key of ['pnow.site_auto_move_unread.title_record', 'pnow.site_auto_move_unread.body_record']) assert.ok(lane[key], key);
    assert.match(lane['pnow.site_auto_move_unread.body_record'], /sudo journalctl -u celikpanel-panel/);
  }
  assert.match(enSiteLimitsLane['pnow.site_auto_move_unread.body_record'], /own record[\s\S]*no site is moved into its limits by itself[\s\S]*Nothing was changed/);
  assert.doesNotMatch(enSiteLimitsLane['pnow.site_auto_move_unread.body_record'], /goes on by itself|continues by itself/, 'no promise the code does not keep');
  // The words stay out of the boot catalogue.
  assert.doesNotMatch(read('../src/i18n/en.ts'), /'slim\.why\.update_finishing'|slim\.auto\.wait\.finishing|body_record/);
});
