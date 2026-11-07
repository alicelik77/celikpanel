// Bulk mailbox actions (alpha.128, G20): one step per mailbox, a refusal is
// said and the rest go on, an answer that never came stops the run.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runBulk } from '../src/lib/bulkMailbox.ts';

const words = { stepLabel: (t) => `Deleting ${t.label}`, unknownNote: 'not known', notStartedNote: 'not started' };
const targets = [{ id: 1, label: 'a@x.test' }, { id: 2, label: 'b@x.test' }, { id: 3, label: 'c@x.test' }, { id: 4, label: 'd@x.test' }];

test('a refused mailbox is said with its reason and the next ones go on', async () => {
  const seen = [];
  const calls = [];
  const result = await runBulk(targets, async (t) => {
    calls.push(t.id);
    return t.id === 2 ? { ok: false, reason: 'busy' } : { ok: true };
  }, (steps) => seen.push(steps), words, () => 0);
  assert.deepEqual(calls, [1, 2, 3, 4]);
  assert.deepEqual(result.done, [1, 3, 4]);
  assert.deepEqual(result.refused, [{ id: 2, reason: 'busy' }]);
  const last = seen.at(-1);
  assert.deepEqual(last.map((s) => s.state), ['done', 'failed', 'done', 'done']);
  assert.equal(last[1].note, 'busy');
  assert.equal(last[0].label, 'Deleting a@x.test');
});

test('an answer that never came stops the run and nothing more is sent', async () => {
  const calls = [];
  let steps = [];
  const result = await runBulk(targets, async (t) => {
    calls.push(t.id);
    if (t.id === 2) throw new Error('network');
    return { ok: true };
  }, (s) => { steps = s; }, words, () => 0);
  assert.deepEqual(calls, [1, 2]);
  assert.equal(result.unknown, 2);
  assert.deepEqual(result.notStarted, [3, 4]);
  assert.deepEqual(steps.map((s) => s.state), ['done', 'waiting', 'pending', 'pending']);
  assert.equal(steps[2].note, 'not started');
});

test('the mail screen offers the three bulk actions with the panel’s own confirmation', () => {
  const source = readFileSync(new URL('../src/components/DomainMailManager.tsx', import.meta.url), 'utf8');
  for (const action of ['bulkLimit', 'bulkEmpty', 'bulkDelete']) assert.match(source, new RegExp(`const ${action} = async`));
  assert.match(source, /runBulk\(/);
  assert.doesNotMatch(source, /window\.confirm|[^.]confirm\(/);
});
