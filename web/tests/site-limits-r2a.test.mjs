// S4 R2a (21 Nov 2026): the limits of a site's programs together. The card
// reads the server's answer through one decoder, says what happened, what
// the plan allows and who can raise it, never shows a customer what only
// the administrator can act on, and the administrator's move waits in the
// panel's own window with its steps. The words are in their own part, in
// both languages, plain and without other companies' product names.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsScreens } from '../src/i18n/screens/siteLimits/en.ts';
import { trSiteLimitsScreens } from '../src/i18n/screens/siteLimits/tr.ts';
import { decodeMoveAnswer, decodeSiteLimits, eventKey, lowerUnits, moveSteps, newMoveRequestID } from '../src/lib/siteLimits.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('the words are the same keys in both languages, with the same placeholders', () => {
  assert.deepEqual(Object.keys(trSiteLimitsScreens).sort(), Object.keys(enSiteLimitsScreens).sort());
  for (const [key, en] of Object.entries(enSiteLimitsScreens)) {
    const want = [...en.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    const got = [...trSiteLimitsScreens[key].matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    assert.deepEqual(got, want, `${key} keeps its placeholders`);
  }
  for (const words of [enSiteLimitsScreens, trSiteLimitsScreens]) {
    for (const [key, text] of Object.entries(words)) {
      assert.ok(!/cPanel|Plesk|aaPanel|DirectAdmin|celiksite_|systemctl|cgroup|MemoryMax|TasksMax|CPUQuota|R2a|D-0/.test(text), `${key} says nothing internal or of another product: ${text}`);
    }
  }
});

test('every limit key the server registers is named for the plan form', () => {
  const go = read('../../cmd/panel/site_limits.go');
  const keys = [...go.matchAll(/setting[A-Za-z]+\s+= "((?:site|db)\.[a-z_]+)"/g)].map((m) => m[1]);
  assert.deepEqual(keys.sort(), ['db.idle_in_transaction_seconds', 'db.statement_seconds', 'db.temp_file_mb', 'site.cpu_percent', 'site.memory_mb', 'site.tasks']);
  for (const key of keys) {
    assert.ok(enSiteLimitsScreens[`limits.key.${key}`], `English name of ${key}`);
    assert.ok(trSiteLimitsScreens[`limits.key.${key}`], `Turkish name of ${key}`);
  }
  assert.match(read('../src/lib/settingLimits.ts'), /unit === 'percent'/, 'the percent unit has words');
});

test('a reached limit says when, what the plan allows and who can raise it', () => {
  for (const kind of ['memory', 'tasks', 'cpu']) {
    for (const words of [enSiteLimitsScreens, trSiteLimitsScreens]) {
      const sentence = words[`slim.event.${kind}`];
      assert.match(sentence, /\{time\}/, `${kind} says when`);
      assert.match(sentence, /\{limit\}/, `${kind} says what the plan allows`);
    }
  }
  assert.match(enSiteLimitsScreens['slim.fix.customer'], /provider/, 'the customer is told who raises it');
  assert.match(enSiteLimitsScreens['slim.fix.admin'], /\{who\}/, 'the administrator is told where it is set');
  assert.equal(eventKey('memory'), 'site.memory_mb');
  assert.equal(eventKey('tasks'), 'site.tasks');
  assert.equal(eventKey('cpu'), 'site.cpu_percent');
});

test('the decoder keeps only what it knows and refuses another shape', () => {
  assert.throws(() => decodeSiteLimits(null));
  assert.throws(() => decodeSiteLimits({ known: true }));
  const view = decodeSiteLimits({
    known: true, domain: 'shop.example.com', admin: true,
    limits: [{ key: 'site.memory_mb', value: 1024, set_by: 'plan', set_by_name: 'Lean' }, { key: 'php.memory_limit_mb', value: 1, set_by: 'x' }],
    events: [{ kind: 'memory', count: 2, first_at: '2026-11-21T10:00:00Z', last_at: '2026-11-21T14:02:00Z', repeated: true }, { kind: 'disk', count: 1, last_at: 'x' }],
    units: [
      { unit: 'celikapp-7.service', kind: 'node', label: 'Node.js', placement: 'outside', own_memory_mb: 1536, lower: true },
      { unit: 'celikrt-3.service', kind: 'program', label: 'program', placement: 'in_slice' },
      { unit: 'x', placement: 'somewhere' },
    ],
    needs_move: 1,
  });
  assert.equal(view.limits.length, 1);
  assert.equal(view.events.length, 1);
  assert.equal(view.units.length, 2);
  assert.deepEqual(lowerUnits(view).map((u) => u.unit), ['celikapp-7.service']);
  assert.throws(() => decodeMoveAnswer({ units: [] }));
  assert.equal(decodeMoveAnswer({ done: false, rolled_back: true, units: [{ unit: 'celikrt-3.service', outcome: 'failed' }] }).rolled_back, true);
  assert.match(newMoveRequestID(), /^[A-Za-z0-9-]{8,64}$/, 'a request id the server takes');
});

test('the card shows the move only to the administrator, in the panel’s own window, with its steps', () => {
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.ok(!/window\.(confirm|alert|prompt)\(/.test(card), 'no browser dialogs');
  assert.ok(!/<Dialog\b/.test(card), 'no dialogue of its own: the product’s question and progress window');
  assert.match(card, /\{view\.admin && <AdminPart/, 'the administrator part is the administrator’s only');
  assert.match(card, /confirmAction\(\{[\s\S]*action: async \(report\)[\s\S]*report\.steps\(/, 'the locked progress window shows the steps');
  for (const step of ['slim.move.step.write', 'slim.move.step.restart', 'slim.move.step.check']) assert.ok(card.includes(step), `${step} is shown`);
  assert.match(card, /onDone\(\)/, 'the card reads again after the move');
  const steps = moveSteps({ done: false, rolled_back: true, units: [
    { unit: 'celikrt-3.service', outcome: 'put_back', put_back: true },
    { unit: 'celiksvc-4.service', outcome: 'failed', step: 'restart', detail: 'it is failed after the restart' },
    { unit: 'celikapp-7.service', outcome: 'not_tried' },
  ] }, [{ unit: 'celiksvc-4.service', kind: 'service_app', label: 'n8n', placement: 'outside' }], { write: 'W', restart: 'R', check: 'C' });
  assert.deepEqual(steps.map((s) => [s.id, s.state]), [['write', 'done'], ['celikrt-3.service', 'failed'], ['celiksvc-4.service', 'failed'], ['celikapp-7.service', 'pending'], ['check', 'failed']]);
  assert.equal(steps[2].label, 'R: n8n');
  assert.equal(steps[2].note, 'it is failed after the restart');
  const overview = read('../src/components/domain/sections/OverviewSection.tsx');
  assert.match(overview, /lazy\(\(\) => import\('\.\.\/\.\.\/SiteLimitsCard'\)/, 'the card is fetched only when it is drawn');
});
