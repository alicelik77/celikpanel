// alpha.148 fix round (the independent review of the merged branch): a
// program the Agent's bounded start did not restart is named on the card as
// "not restarted yet" (finding 5), and the Turkish Problems now line says that
// no site at all is moved while a move's record is kept (finding 8).
import assert from 'node:assert/strict';
import test from 'node:test';
import { enSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/en.ts';
import { trSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/tr.ts';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';
import { trProblemsNowScreens } from '../src/i18n/screens/problemsNow/tr.ts';
import { decodeSiteLimits, keptUnits } from '../src/lib/siteLimits.ts';

test('a program the Agent’s start did not restart is named with its state', () => {
  const view = decodeSiteLimits({
    known: true, admin: true, events: [], limits: [],
    move_interrupted: { at: '2026-11-24T08:00:00Z', kept: true, units: { 'celikrt-5.service': 'starting', 'celiksvc-6.service': 'waiting', 'celikrt-4.service': 'put_back' } },
  });
  assert.deepEqual(keptUnits(view), [
    { unit: 'celikrt-5.service', state: 'starting' }, { unit: 'celiksvc-6.service', state: 'waiting' },
  ]);
  assert.equal(enSiteLimitsApplyScreens['slim.kept.waiting'], 'not restarted yet');
  assert.equal(trSiteLimitsApplyScreens['slim.kept.waiting'], 'henüz yeniden başlatılmadı');
});

test('while a move’s record is kept, the line says no site is moved', () => {
  assert.match(enProblemsNowScreens['pnow.site_move_kept.body'], /no site is moved until/);
  assert.match(trProblemsNowScreens['pnow.site_move_kept.body'], /çözülene dek hiçbir site taşınmaz/);
});
