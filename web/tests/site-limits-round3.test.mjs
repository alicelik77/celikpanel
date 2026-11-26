// S4 R2a third fix round (23 Nov 2026): what the third review of PR #349
// found on the screens. A move whose put-back did not complete is shown with
// each program left, its state, the by-hand step and "Try again" (never a
// "forget it"); the rolled-back message says "put back" only for what was
// confirmed back; "in force" is the kernel's, with a "written, not in force
// yet" and a "not known" state for the administrator; a customer reads one
// neutral state for everything they cannot act on; Problems now has one line
// while a move's record is kept.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsScreens } from '../src/i18n/screens/siteLimits/en.ts';
import { trSiteLimitsScreens } from '../src/i18n/screens/siteLimits/tr.ts';
import { enSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/en.ts';
import { trSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/tr.ts';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';
import { trProblemsNowScreens } from '../src/i18n/screens/problemsNow/tr.ts';
import { decodeMoveAnswer, decodeSiteLimits, keptUnits, putBackSplit } from '../src/lib/siteLimits.ts';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const words = (key) => [
    enSiteLimitsScreens[key] ?? enSiteLimitsApplyScreens[key] ?? enProblemsNowScreens[key],
    trSiteLimitsScreens[key] ?? trSiteLimitsApplyScreens[key] ?? trProblemsNowScreens[key],
];

test('a kept move names each program left, its state, the by-hand step and Try again', () => {
  const view = decodeSiteLimits({
    known: true, admin: true, events: [], limits: [],
    move_interrupted: { at: '2026-11-23T08:00:00Z', kept: true, units: { 'celikrt-3.service': 'failed', 'celikrt-4.service': 'put_back', 'celikapp-7.service': 'unknown', 'celiksvc-2.service': 'starting' } },
  });
  assert.deepEqual(keptUnits(view), [
    { unit: 'celikapp-7.service', state: 'unknown' }, { unit: 'celikrt-3.service', state: 'failed' }, { unit: 'celiksvc-2.service', state: 'starting' },
  ]);
  assert.deepEqual(keptUnits({ ...view, move_interrupted: { ...view.move_interrupted, kept: false } }), [], 'a settled record lists nothing');
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /view\.move_interrupted\?\.kept \?[\s\S]*slim\.kept'[\s\S]*slim\.kept\.unit'[\s\S]*pnow\.retry'/);
  assert.match(card, /body: JSON\.stringify\(\{ request_id: requestID, recover: true \}\)/, 'Try again asks for the put-back only');
  assert.match(card, /followOperation\(\{/, 'it runs in the panel’s own window');
  assert.ok(!/forget|dismiss/i.test(card), 'no button forgets a stopped program');
  for (const state of ['failed', 'unknown', 'starting']) {
    for (const text of words(`slim.kept.${state}`)) assert.ok(text, state);
  }
  for (const text of words('slim.kept.unit')) {
    assert.match(text, /journalctl -u \{name\}/, 'why');
    assert.match(text, /systemctl start \{name\}/, 'the by-hand step');
  }
  assert.match(enSiteLimitsApplyScreens['slim.kept'], /no site is moved until it does/);
});

test('the interrupted message says what is true once the put-back is done', () => {
  for (const text of words('slim.interrupted')) assert.ok(!/as it was|olduğu gibi geri/.test(text), text);
  assert.match(enSiteLimitsScreens['slim.interrupted'], /finished, put back, or left as changed meanwhile/);
});

test('the rolled-back message says put back only for what was confirmed back and names the rest', () => {
  const answer = decodeMoveAnswer({
    done: false, rolled_back: true, kept: true, units: [
      { unit: 'celikrt-5.service', outcome: 'failed', put_back: false, detail: 'did not start' },
      { unit: 'celikrt-4.service', outcome: 'put_back', put_back: true, ran: true },
      { unit: 'celikrt-6.service', outcome: 'put_back', put_back: false },
      { unit: 'celikrt-7.service', outcome: 'not_tried' },
    ],
  });
  assert.equal(answer.kept, true);
  // alpha.149 fix round (W1): a program that did not run before is put back as it was.
  assert.deepEqual(putBackSplit(answer), { back: ['celikrt-4.service'], asWere: [], left: ['celikrt-5.service', 'celikrt-6.service'] });
  // alpha.149 (the alpha.148 lab's D6/D7): "put back" says running again; each of the
  // rest is named with its reason in the page's words (route-scoped lane copy).
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) {
    assert.ok(!/every program|bütün programları/.test(words['slim.move.back']), words['slim.move.back']);
    assert.match(words['slim.move.back'], /\{list\}/);
    assert.match(words['slim.move.backNot'], /\{unit\}[\s\S]*\{reason\}/);
  }
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /const \{ back, asWere, left \} = putBackSplit\(answer\)[\s\S]*slim\.move\.backNot[\s\S]*slim\.move\.failedUnit[\s\S]*slim\.move\.back[\s\S]*slim\.move\.backStopped/);
});

test('in force is the kernel’s; a customer reads one neutral state', () => {
  for (const state of ['pending', 'unknown', 'not_yet']) {
    for (const text of words(`slim.applies.${state}`)) assert.ok(text, state);
  }
  assert.match(enSiteLimitsApplyScreens['slim.applies.pending'], /not in force yet/);
  assert.match(enSiteLimitsApplyScreens['slim.applies.unknown'], /not known/);
  assert.match(enSiteLimitsApplyScreens['slim.applies.not_yet'], /nothing is needed from you/);
  for (const text of words('slim.applies.not_yet')) assert.ok(!/systemctl|file|dosya|outside|dışarıda/.test(text), text);
  const view = decodeSiteLimits({ known: true, admin: false, applies: 'not_yet', events: [], limits: [] });
  assert.equal(view.applies, 'not_yet');
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /view\.known && view\.live && applies === 'yes'/, 'use is shown only where the limits apply');
  assert.match(card, /t\(`slim\.applies\.\$\{applies\}` as TranslationKey/);
});

test('Problems now has one line while a move’s record is kept', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  const list = problems.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  assert.ok(list && list[1].includes("'site_move_kept'"));
  for (const part of ['title', 'body', 'action']) {
    for (const text of words(`pnow.site_move_kept.${part}`)) assert.ok(text, part);
  }
});
