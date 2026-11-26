// S4 R2a second fix round (22 Nov 2026): what the second review of PR #349
// found on the screens. The card shows the limit in force beside the plan's
// when they differ and never names the plan's number for one it did not
// reach; a move that ran out of time says which programs are done; the
// change preview's list is the one the save confirms; a reseller's form says
// who sets the bound of a site's limits; Problems now has the lines of a
// change that did not reach every site and of a default fixed for the
// server's earlier memory, with that default's preview-and-confirm action.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsScreens } from '../src/i18n/screens/siteLimits/en.ts';
import { trSiteLimitsScreens } from '../src/i18n/screens/siteLimits/tr.ts';
import { enSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/en.ts';
import { trSiteLimitsApplyScreens } from '../src/i18n/screens/siteLimitsApply/tr.ts';
import { enProblemsNowScreens } from '../src/i18n/screens/problemsNow/en.ts';
import { trProblemsNowScreens } from '../src/i18n/screens/problemsNow/tr.ts';
import { appliedOf, decodeMoveAnswer, decodeSiteLimits } from '../src/lib/siteLimits.ts';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('the new part has the same keys and placeholders in both languages and says nothing internal', () => {
  assert.deepEqual(Object.keys(trSiteLimitsApplyScreens).sort(), Object.keys(enSiteLimitsApplyScreens).sort());
  for (const [key, en] of Object.entries(enSiteLimitsApplyScreens)) {
    const want = [...en.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    const got = [...trSiteLimitsApplyScreens[key].matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    assert.deepEqual(got, want, `${key} keeps its placeholders`);
  }
  // Third fix round: the administrator's by-hand steps (a stopped program,
  // a change made by hand) name the commands the review asked for; nothing
  // else names a system command.
  const commands = new Set(['slim.kept.unit', 'slim.applies.pending']);
  for (const words of [enSiteLimitsApplyScreens, trSiteLimitsApplyScreens]) {
    for (const [key, text] of Object.entries(words)) {
      const internal = commands.has(key) ? /cPanel|Plesk|aaPanel|DirectAdmin|celiksite_|cgroup|MemoryMax|R2a|D-0/ : /cPanel|Plesk|aaPanel|DirectAdmin|celiksite_|systemctl|cgroup|MemoryMax|R2a|D-0/;
      assert.ok(!internal.test(text), `${key}: ${text}`);
    }
  }
  for (const key of Object.keys(enSiteLimitsApplyScreens)) {
    assert.equal(enSiteLimitsScreens[key], undefined, `${key} is in one part only`);
  }
});

test('the card shows the limit in force beside the plan’s, and an event names the number reached', () => {
  const view = decodeSiteLimits({
    known: true, admin: false, applies: 'yes', events: [],
    limits: [{ key: 'site.memory_mb', value: 1024, set_by: 'plan' }, { key: 'site.tasks', value: 256, set_by: 'plan' }, { key: 'site.cpu_percent', value: 200, set_by: 'plan' }],
    live: { memory_mb: 768, tasks: 256, cpu_percent: 0 },
  });
  assert.equal(appliedOf(view, 'site.memory_mb'), 768, 'an owner’s drop-in or an earlier limit');
  assert.equal(appliedOf(view, 'site.tasks'), undefined, 'the plan’s own value is not repeated');
  assert.equal(appliedOf(view, 'site.cpu_percent'), 0, 'no limit at all');
  assert.equal(appliedOf({ ...view, live: undefined }, 'site.memory_mb'), undefined, 'not known: nothing is claimed');
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /data-site-limit-applied=""[^\n]*slim\.applied/, 'the tile says the value in force');
  // alpha.149 (the alpha.148 lab's CHECK 2): the number that held when it happened, never today's.
  assert.match(card, /t\('slim\.event\.limitThen', \{ limit: amountText\(key, then\) \}\)/, 'the event names the number that held then');
  assert.match(card, /const atLimit = eventAtSiteLimit\(event\.kind\) && applied === undefined/, 'no fix names the plan when the plan did not set it');
  for (const kind of ['memory', 'tasks', 'cpu']) {
    for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.ok(!/plan/i.test(words[`slim.event.${kind}`]) && !/Plan/.test(words[`slim.event.${kind}`]), `${kind} does not call the number the plan’s`);
  }
});

test('one memory "time" is a program stopped, and an unattributed process refusal blames no limit', () => {
  assert.match(enSiteLimitsLane['slim.event.memory'], /stopped a program \{count\} times/);
  assert.match(trSiteLimitsLane['slim.event.memory'], /\{count\} kez bir programı durdurdu/);
  assert.match(enSiteLimitsScreens['slim.event.tasks_other'], /does not show that it was this site’s limit/);
  assert.ok(!/at its own limit/.test(enSiteLimitsScreens['slim.event.tasks_other']));
});

test('a move that ran out of time says which programs are done and that the next move continues', () => {
  assert.equal(decodeMoveAnswer({ done: false, time_up: true, units: [{ unit: 'celikrt-3.service', outcome: 'moved' }] }).time_up, true);
  assert.equal(decodeMoveAnswer({ done: true, units: [] }).time_up, false);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /if \(answer\.time_up\) return \{ state: 'failed', reason: t\('slim\.move\.timeUp'\) \}/);
  assert.match(enSiteLimitsApplyScreens['slim.move.timeUp'], /marked done/);
});

test('a save confirms exactly the list of sites the preview named', () => {
  assert.match(read('../src/lib/rights.ts'), /if \(flags\.narrowing && flags\.stoppedToken\) body\.confirm_stopped = flags\.stoppedToken;/, 'sent with the confirmation only');
  assert.match(read('../src/components/PlanRights.tsx'), /stoppedToken: shown\?\.preview\.stopped_token/);
  assert.match(read('../src/components/AccountRights.tsx'), /confirm_stopped: shown\.stopped_token/);
  assert.match(read('../src/lib/rights.ts'), /stopped_token: typeof raw\.stopped_token === 'string'/);
});

test('a reseller’s form says who sets the bound of a site’s limits', () => {
  const form = read('../src/components/PlanLimitsSection.tsx');
  assert.match(form, /scope === 'reseller' && limit\.key\.startsWith\('site\.'\)[\s\S]*slim\.who\.line[\s\S]*whoText\(t, limit\.set_by, limit\.set_by_name\)/);
});

test('Problems now shows the two new lines with their words and the default’s preview-and-confirm action', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  const list = problems.match(/const KINDS: readonly ProblemKind\[\] = \[([\s\S]*?)\];/);
  // Third fix round: the unapplied change's words moved to the problemsNow
  // part (the siteLimitsApply part had no room for the round's new words).
  const en = { ...enSiteLimitsApplyScreens, ...enProblemsNowScreens };
  const tr = { ...trSiteLimitsApplyScreens, ...trProblemsNowScreens };
  for (const kind of ['site_limits_unapplied', 'site_memory_default']) {
    assert.ok(list && list[1].includes(`'${kind}'`), `${kind} is shown`);
    for (const part of ['title', 'body', 'action']) {
      assert.ok(en[`pnow.${kind}.${part}`] && tr[`pnow.${kind}.${part}`], `${kind} ${part}`);
    }
  }
  assert.match(problems, /p\.action\.kind === 'apply_default' \? p\.kind === 'site_memory_default'/, 'the action belongs to that line only');
  assert.match(problems, /send\(\{ action: 'preview' \}\)[\s\S]*confirmAction\([\s\S]*send\(\{ action: 'apply', confirm_stopped: preview\.stopped_token/, 'preview, the panel’s own question, then the apply with the list');
  assert.ok(!/window\.(confirm|alert|prompt)\(/.test(problems), 'no browser dialogs');
  assert.match(en['pnow.site_limits_unapplied.body'], /every hour/);
  assert.match(enSiteLimitsApplyScreens['pnow.site_memory_default.body'], /Nothing changes by itself/);
});
