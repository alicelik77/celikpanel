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
import { enSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/en.ts';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { decodeMoveAnswer, decodeSiteLimits, eventKey, lowerUnits, moveSteps, newMoveRequestID, notMovedReason } from '../src/lib/siteLimits.ts';

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
  // alpha.149 (the alpha.148 lab's CHECK 2): the event says when; the limit is
  // the one that held then, in its own sentence, never today's (route-scoped).
  for (const kind of ['memory', 'tasks', 'cpu']) {
    for (const words of [enSiteLimitsLane, trSiteLimitsLane]) {
      const sentence = words[`slim.event.${kind}`];
      assert.match(sentence, /\{time\}/, `${kind} says when`);
      assert.ok(!/\{limit\}/.test(sentence), `${kind} names no limit of today`);
    }
  }
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.match(words['slim.event.limitThen'], /\{limit\}/);
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
  ] }, [{ unit: 'celiksvc-4.service', kind: 'service_app', label: 'n8n', placement: 'outside' }], { write: 'W', restart: 'R', check: 'C', reason: (r) => `reason:${r.code}`, why: 'Y' });
  assert.deepEqual(steps.map((s) => [s.id, s.state]), [['write', 'done'], ['celikrt-3.service', 'failed'], ['celiksvc-4.service', 'failed'], ['celikapp-7.service', 'pending'], ['check', 'failed']]);
  assert.equal(steps[2].label, 'R: n8n');
  // alpha.149 (lab D7): never the agent's English detail; its code in the page's words.
  assert.equal(steps[2].note, undefined);
  const coded = moveSteps({ done: false, rolled_back: true, units: [
    { unit: 'celiksvc-4.service', outcome: 'failed', step: 'restart', detail: 'it is failed after the restart', reason: { code: 'not_running', seconds: 90 } },
  ] }, [], { write: 'W', restart: 'R', check: 'C', reason: (r) => `reason:${r.code}`, why: 'Y' });
  assert.equal(coded[1].note, 'reason:not_running');
  const overview = read('../src/components/domain/sections/OverviewSection.tsx');
  assert.match(overview, /lazy\(\(\) => import\('\.\.\/\.\.\/SiteLimitsCard'\)/, 'the card is fetched only when it is drawn');
});

test('the card never promises limits that do not apply, never blames the plan for what is not its limit, and speaks to a reseller as the provider', () => {
  const card = read('../src/components/SiteLimitsCard.tsx');
  // "stayed within their limits" only where the limits apply.
  assert.match(card, /applies === 'yes' && \(\s*<p[^>]*data-site-limits-events="none"/, 'no "stayed within" where nothing applies');
  for (const state of ['outside', 'no_file', 'unsupported']) {
    for (const words of [enSiteLimitsScreens, trSiteLimitsScreens]) assert.ok(words[`slim.applies.${state}`], `${state} has words`);
  }
  // Events not proved at the site's limit name neither the plan nor a fix.
  for (const kind of ['memory_other', 'tasks_other']) {
    for (const words of [enSiteLimitsScreens, trSiteLimitsScreens]) assert.ok(!/\{limit\}/.test(words[`slim.event.${kind}`]), `${kind} names no plan`);
  }
  assert.match(card, /\{atLimit && <p/, 'the fix is shown only for the site’s own limit');
  const view = decodeSiteLimits({ known: true, admin: false, reseller: true, limits: [], applies: 'no_file',
    events: [{ kind: 'memory_other', count: 1, last_at: '2026-11-21T10:00:00Z' }, { kind: 'tasks_other', count: 2, last_at: '2026-11-21T10:00:00Z' }] });
  assert.equal(view.events.length, 2, 'the other events are read');
  assert.equal(view.reseller, true);
  assert.match(card, /view\.reseller \? 'slim\.who\.reseller'/, 'a reseller reads the provider’s words');
  assert.match(card, /view\.reseller \? 'slim\.fix\.reseller'/, 'and the provider’s fix');
  assert.match(enSiteLimitsScreens['slim.who.reseller'], /within your own/);
  // The owner's drop-ins by name, the units the move cannot take with who acts.
  assert.match(card, /slim\.dropIns/);
  for (const reason of ['edited', 'older', 'owner_slice']) assert.ok(enSiteLimitsScreens[`slim.blocked.${reason}`] && trSiteLimitsScreens[`slim.blocked.${reason}`], `${reason} has words`);
  assert.equal(notMovedReason('kept'), 'edited');
  assert.equal(notMovedReason('changed'), 'changed');
  assert.equal(notMovedReason('moved'), undefined);
  // The move's wait is stated, not "20 seconds".
  assert.ok(!/20 (seconds|saniye)/.test(enSiteLimitsScreens['slim.move.body'] + trSiteLimitsScreens['slim.move.body']));
  assert.match(enSiteLimitsScreens['slim.move.body'], /\{seconds\}/);
  // The database keys say which is a default and which a firm limit.
  assert.match(enSiteLimitsScreens['limits.key.db.statement_seconds'], /Default/);
  assert.match(enSiteLimitsScreens['limits.key.db.temp_file_mb'], /firm limit/);
});

test('a lowered limit lists the sites whose programs would be stopped, in the change preview', () => {
  const box = read('../src/components/RightsSetEditor.tsx');
  assert.match(box, /preview\.sites_stopped_count/);
  // alpha.149: the heading and each site's line in the page's words (i18n/siteLimitsLane.ts).
  assert.match(box, /sitesStoppedTitle\(/);
  assert.match(box, /siteStoppedLines\(preview, laneT\)/);
  assert.match(read('../src/lib/rights.ts'), /sites_stopped: Array\.isArray\(raw\.sites_stopped\)/);
  // Second fix round: the sentence moved to its own part and says what each lowered limit does.
  assert.match(enSiteLimitsApplyScreens['limits.preview.sitesStopped'], /memory limit is stopped at once; at the process limit no new process starts and running ones go on/);
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.ok(problems.includes("'site_limits_unsupported'"), 'the host line is a known kind');
  for (const part of ['title', 'body', 'action']) assert.ok(trSiteLimitsScreens[`pnow.site_limits_unsupported.${part}`]);
});

test('sites whose programs run outside their limits are one administrator line with words in both languages', async () => {
  // S4 R2b: the line counts a site's programs or its scheduled tasks and its
  // action opens "Move all" (siteTasks part).
  // alpha.150: these lines are in the part siteTasksProblems.
  const { enSiteTasksProblemsScreens: enSiteTasksScreens } = await import('../src/i18n/screens/siteTasksProblems/en.ts');
  const { trSiteTasksProblemsScreens: trSiteTasksScreens } = await import('../src/i18n/screens/siteTasksProblems/tr.ts');
  const problems = read('../src/components/ProblemsNow.tsx');
  const list = problems.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  assert.ok(list && list[1].includes("'site_limits_outside'"), 'the list shows the kind');
  for (const part of ['title', 'body', 'action']) {
    assert.ok(enSiteTasksScreens[`pnow.site_limits_outside.${part}`], `English ${part}`);
    assert.ok(trSiteTasksScreens[`pnow.site_limits_outside.${part}`], `Turkish ${part}`);
  }
  assert.match(enSiteTasksScreens['pnow.site_limits_outside.body'], /\{domain\}/);
  assert.match(trSiteTasksScreens['pnow.site_limits_outside.body'], /\{domain\}/);
  assert.equal(enSiteTasksScreens['pnow.site_limits_outside.action'], enSiteTasksScreens['moveall.button']);
  assert.equal(trSiteTasksScreens['pnow.site_limits_outside.action'], trSiteTasksScreens['moveall.button']);
});
