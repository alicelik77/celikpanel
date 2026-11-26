// alpha.149 second fix round of PR #371 (the second review's R2, R8): an
// update record that cannot be read refuses a move with its own words; a
// listener table that cannot be read is "not known", never "no port"; Try
// again names the programs it put back that run but listen on no port. The
// words are the lane's own (route-scoped, no budget raised).
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { decodeMoveAnswer, moveReasonWords, moveWhyKey } from '../src/lib/siteLimits.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('an update record that cannot be read refuses the move in the page’s words, and says it ends by itself', () => {
  assert.equal(moveWhyKey({ code: 'update_unreadable' }), 'slim.why.update_unreadable');
  assert.match(enSiteLimitsLane['slim.why.update_unreadable'], /not known whether[\s\S]*Nothing was moved[\s\S]*ends by itself[\s\S]*31 minutes/);
  assert.match(trSiteLimitsLane['slim.why.update_unreadable'], /bilinmiyor[\s\S]*Hiçbir şey taşınmadı[\s\S]*kendiliğinden biter/);
});

test('a listener table that cannot be read is worded as not known, never as no port', () => {
  assert.deepEqual(moveReasonWords({ code: 'ports_unread', seconds: 90, ports: [30010] }), { key: 'slim.reason.ports_unread', values: { seconds: 90, ports: '30010' } });
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.ok(!/none of its ports|hiçbirinden/.test(words['slim.reason.ports_unread']));
});

test('Try again names the programs it put back that run but listen on no port', () => {
  assert.deepEqual(decodeMoveAnswer({ done: true, units: [], runs_no_port: ['celikrt-5.service', 7] }).runs_no_port, ['celikrt-5.service']);
  assert.deepEqual(decodeMoveAnswer({ done: true, units: [] }).runs_no_port, []);
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.match(words['slim.kept.runsNoPort'], /\{list\}[\s\S]*journalctl -u/);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /answer\.runs_no_port[\s\S]*t\('slim\.kept\.runsNoPort', \{ list: silent\.map\(\(u\) => labelOf\(units, u\)\)/);
});
