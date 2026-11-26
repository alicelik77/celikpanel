// alpha.149 fix round A (the alpha.148 lab): the move's reasons in the page's
// own language and never the agent's English text (D7), a put-back program
// that is not running named as such (D6), a program in systemd's restart cycle
// (D6), the limit that held when an event happened (CHECK 2), a reached
// process limit seen by the slice's peak (D3), the reseller's bound (CHECK 2),
// the preview's site lines and singular forms (CHECK 2), the unapplied line's
// reasons (O3), the update refused during a move (D5) and the duplicate
// sign-in text (D2). The lane's new words are route-scoped (no budget raised).
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enSiteLimitsLane } from '../src/i18n/siteLimitsLane/en.ts';
import { trSiteLimitsLane } from '../src/i18n/siteLimitsLane/tr.ts';
import { siteLimitsLaneTranslation, siteStoppedLines, sitesStoppedTitle, unappliedCounts, unappliedKey, unappliedVars, withSiteLimitsLane } from '../src/i18n/siteLimitsLane.ts';
import { enRightsScreens } from '../src/i18n/screens/rights/en.ts';
import { enErrorsScreens } from '../src/i18n/screens/errors/en.ts';
import { trErrorsScreens } from '../src/i18n/screens/errors/tr.ts';
import { enSetupScreens } from '../src/i18n/screens/setup/en.ts';
import { trSetupScreens } from '../src/i18n/screens/setup/tr.ts';
import { decodeMoveAnswer, decodeSiteLimits, keptUnits, moveReasonWords, moveWhyKey } from '../src/lib/siteLimits.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('the lane’s words are the same keys in both languages, with the same placeholders', () => {
  assert.deepEqual(Object.keys(trSiteLimitsLane).sort(), Object.keys(enSiteLimitsLane).sort());
  for (const [key, en] of Object.entries(enSiteLimitsLane)) {
    const want = [...en.matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    const got = [...trSiteLimitsLane[key].matchAll(/\{([a-z]+)\}/g)].map((m) => m[1]).sort();
    assert.deepEqual(got, want, `${key} keeps its placeholders`);
    for (const text of [en, trSiteLimitsLane[key]]) {
      assert.ok(!/cPanel|Plesk|aaPanel|DirectAdmin|celiksite_|cgroup|MemoryMax|R2a|D-0|account\(s\)|site\(s\)/.test(text), `${key}: ${text}`);
    }
  }
  assert.equal(siteLimitsLaneTranslation('tr', 'slim.why.other'), trSiteLimitsLane['slim.why.other']);
  assert.equal(siteLimitsLaneTranslation('tr', 'slim.title'), null, 'other keys stay the screen catalogue’s');
});

test('a move’s reason is a code worded by the page; an unknown one points to the journal, never English text', () => {
  assert.deepEqual(moveReasonWords({ code: 'not_running', state: 'failed failed', seconds: 90 }), { key: 'slim.reason.not_running', values: { seconds: 90, ports: '' } });
  assert.equal(moveReasonWords({ code: 'not_running', state: 'activating auto-restart', seconds: 90 }).key, 'slim.reason.restarting');
  assert.deepEqual(moveReasonWords({ code: 'not_listening', seconds: 20, ports: [30010, 30011] }).values, { seconds: 20, ports: '30010, 30011' });
  assert.equal(moveReasonWords({ code: 'something new' }).key, 'slim.reason.other');
  assert.equal(moveReasonWords(undefined).key, 'slim.reason.other');
  assert.equal(moveWhyKey({ code: 'another_move' }), 'slim.why.another_move');
  assert.equal(moveWhyKey({ code: 'prepare_failed' }), 'slim.why.other');
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.match(words['slim.reason.other'], /journalctl -u celikpanel-agent/);
  const answer = decodeMoveAnswer({ done: false, units: [], error: 'english', reason: { code: 'site_busy' } });
  assert.equal(answer.reason.code, 'site_busy');
  // The card never shows the agent's English: neither a unit's detail nor the answer's error.
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.ok(!/failed\.detail|answer\.error/.test(card), 'no English text of the agent on the card');
  assert.match(card, /slim\.move\.failedUnit[\s\S]*reason: reasonText\(failed\.reason\)/);
  assert.match(card, /moveWhyKey\(answer\.reason\)/);
  const steps = read('../src/lib/siteLimits.ts');
  assert.ok(!/note: state === 'failed' \? u\.detail/.test(steps), 'the window’s steps carry no English detail');
});

test('a put-back program that is not running is said to be not running, who acts and the next step', () => {
  const view = decodeSiteLimits({
    known: true, admin: true, events: [], limits: [],
    move_interrupted: { at: '2026-11-26T08:00:00Z', kept: true, units: { 'celikrt-1.service': 'failed', 'celikrt-2.service': 'unverified', 'celikrt-3.service': 'not_answering', 'celikrt-4.service': 'put_back' } },
  });
  assert.deepEqual(keptUnits(view).map((k) => k.state), ['failed', 'unverified', 'not_answering']);
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) {
    assert.match(words['slim.kept.notRunning'], /\{unit\}[\s\S]*journalctl -u \{name\}/);
    assert.match(words['pnow.site_move_kept.body_stopped'], /\{other\}/);
  }
  assert.match(enSiteLimitsLane['slim.kept.notRunning'], /is not running now[\s\S]*Try again or start it[\s\S]*Nothing else was changed/);
  assert.match(trSiteLimitsLane['slim.kept.notRunning'], /şu an çalışmıyor[\s\S]*Tekrar dene’ye basın ya da sayfasından başlatın[\s\S]*Başka hiçbir şey değiştirilmedi/);
  assert.match(enSiteLimitsLane['pnow.site_move_kept.body_stopped'], /server’s administrator/);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /k\.state === 'failed'\s*\? t\('slim\.kept\.notRunning'/);
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.match(problems, /problem\.kind === 'site_move_kept' && problem\.name\) return 'pnow\.site_move_kept\.body_stopped'/);
  // Monitoring's alerts list reads the lane's words too.
  assert.match(read('../src/components/MonitoringDetail.tsx'), /siteLimitsLaneTranslation\(i18n\.locale, key, values\)/);
});

test('a program systemd keeps starting again is its own place, never the owner’s', () => {
  const view = decodeSiteLimits({ known: true, admin: true, events: [], limits: [],
    units: [{ unit: 'celikrt-1.service', kind: 'program', label: 'program', placement: 'restarting' }] });
  assert.equal(view.units.length, 1);
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.match(words['slim.place.restarting'], /journalctl -u \{name\}/);
  assert.match(read('../src/components/SiteLimitsCard.tsx'), /slim\.place\.\$\{u\.placement\}` as TranslationKey, \{ name: u\.unit \}/);
});

test('an event shows the limit that held then, or none; a peak at the process limit has its own words', () => {
  const view = decodeSiteLimits({ known: true, admin: false, limits: [], events: [
    { kind: 'memory', count: 4, first_at: 'a', last_at: '2026-11-25T19:05:00Z', limit: 128 },
    { kind: 'tasks_peak', count: 0, first_at: 'a', last_at: '2026-11-25T19:05:00Z', limit: 64 },
  ] });
  assert.deepEqual(view.events.map((e) => [e.kind, e.limit]), [['memory', 128], ['tasks_peak', 64]]);
  assert.match(enSiteLimitsLane['slim.event.tasks_peak'], /does not count how often/);
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /event\.limit/);
  assert.ok(!/limit: applied !== undefined \? amountText\(key, applied\) : valueText\(limit\)/.test(card), 'never today’s value for an earlier event');
});

test('the reseller’s bound is the server’s default, not a limit the administrator set', () => {
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /limit\.set_by === 'server_default_bound'\) return t\(view\.admin \? 'slim\.who\.bound' : 'slim\.who\.boundReseller'\)/);
  assert.match(enSiteLimitsLane['slim.who.bound'], /server’s default/);
});

test('the preview’s sites are worded by the page, one site and one account in the singular', () => {
  const en = withSiteLimitsLane('en', (key, values) => (key === 'limits.unit.mb' ? `${values.n} MB` : key === 'limits.preview.sitesStopped' ? `The programs of ${values.n} sites` : key));
  const tr = withSiteLimitsLane('tr', (key, values) => (key === 'limits.unit.mb' ? `${values.n} MB` : key));
  const preview = { sites_stopped: ['kx1.dr.test: memory 215 MB in use, new limit 128 MB'], sites_stopped_count: 1,
    sites_stopped_items: [{ domain: 'kx1.dr.test', memory_mb: 215, memory_limit_mb: 128 }] };
  assert.deepEqual(siteStoppedLines(preview, tr), ['kx1.dr.test: kullanılan bellek 215 MB, yeni sınır 128 MB']);
  assert.deepEqual(siteStoppedLines(preview, en), ['kx1.dr.test: memory 215 MB in use, new limit 128 MB']);
  // Fix round (W4): the "+n" line keeps its word.
  assert.deepEqual(siteStoppedLines({ ...preview, sites_stopped_count: 41 }, en).slice(-1), ['+40 more']);
  assert.deepEqual(siteStoppedLines({ ...preview, sites_stopped_count: 41 }, tr).slice(-1), ['+40 site daha']);
  assert.match(sitesStoppedTitle(1, en), /^The programs of 1 site use/);
  assert.match(sitesStoppedTitle(3, en), /3 sites/);
  assert.match(enSiteLimitsLane['rights.plan.narrowing.one'], /^1 account follows this plan/);
  assert.match(read('../src/components/PlanRights.tsx'), /followers === 1[\s\S]*rights\.plan\.narrowing\.one/);
  assert.match(read('../src/components/ProblemsNow.tsx'), /siteStoppedLines\(preview, t\)/);
});

test('the unapplied line words each site by its own reason and says "not known" when that is all that is known', () => {
  const problems = read('../src/components/ProblemsNow.tsx');
  assert.match(problems, /problem\.kind === 'site_limits_unapplied'\) Object\.assign\(vars, unappliedVars\(/);
  assert.match(problems, /problem\.kind === 'site_limits_unapplied'\) return unappliedKey\(problem, 'title'\)/);
  assert.match(problems, /problem\.kind === 'site_limits_unapplied'\) return unappliedKey\(problem, 'body'\)/);
  for (const code of ['kernel_differs', 'write_failed', 'refused', 'out_of_range', 'unchecked', 'agent_unreachable', 'agent_refused', 'other']) {
    assert.ok(enSiteLimitsLane[`slim.applyReason.${code}`] && trSiteLimitsLane[`slim.applyReason.${code}`], code);
  }
  assert.match(enSiteLimitsLane['pnow.site_limits_unapplied.title_one'], /1 site yet/);
  // Fix round (F9): a batch of one kernel that differs and two not read is both, each by its count.
  const mixed = { kind: 'site_limits_unapplied', name: 'kernel_differs:1,unchecked:2', value: 3 };
  assert.deepEqual(unappliedCounts(mixed.name, 3), { lacking: 1, unknown: 2, lackingCodes: ['kernel_differs'], unknownCodes: ['unchecked'] });
  assert.equal(unappliedKey(mixed, 'title'), 'pnow.site_limits_unapplied.title_mixed');
  const en = withSiteLimitsLane('en', (key) => key);
  const body = en(unappliedKey(mixed, 'body'), { domain: 'Lean', ...unappliedVars(mixed, en) });
  assert.equal(body, 'After a change of limits (Lean), the earlier limits still hold for 1 site (the kernel still holds the earlier values); for 2 sites it is not known whether the new ones arrived (what their running programs hold could not be read). CelikPanel tries again every hour; this line goes away when every site has the new ones.');
  // Only not known: never "earlier limits".
  const unknown = { kind: 'site_limits_unapplied', name: 'agent_unreachable:2', value: 2 };
  assert.equal(unappliedKey(unknown, 'title'), 'pnow.site_limits_unapplied.title_unchecked');
  assert.ok(!/earlier limits/.test(en(unappliedKey(unknown, 'body'), unappliedVars(unknown, en))));
  // Only lacking, one site: the singular; an earlier record's single code counts for all.
  assert.equal(unappliedKey({ name: 'write_failed:1', value: 1 }, 'body'), 'pnow.site_limits_unapplied.body_one');
  assert.equal(unappliedKey({ name: 'kernel_differs', value: 4 }, 'title'), 'pnow.site_limits_unapplied.title');
  assert.equal(unappliedKey({ name: 'unchecked', value: 4 }, 'title'), 'pnow.site_limits_unapplied.title_unchecked');
});

test('fix round: put-back words are true (W1–W4), the peak says only what is known (W5), the update refuses a move (F6)', () => {
  // W1: "running again" only for programs that ran before; the others are put back as they were.
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.match(words['slim.move.backStopped'], /\{list\}/);
  assert.match(enSiteLimitsLane['slim.move.backStopped'], /not running/);
  // W2: "not known yet" never says "not restarted"; "unknown" is only for what was not restarted.
  assert.ok(!/not restarted|agent started/.test(enSiteLimitsLane['slim.kept.unverified']));
  // W3: for unverified and not answering the action is to look, never "start it".
  const card = read('../src/components/SiteLimitsCard.tsx');
  assert.match(card, /k\.state === 'unverified' \|\| k\.state === 'not_answering' \? 'slim\.kept\.look' : 'slim\.kept\.unit'/);
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.ok(!/start it|başlatın\.|systemctl start/.test(words['slim.kept.look']), words['slim.kept.look']);
  assert.match(enSiteLimitsLane['slim.kept.look'], /systemctl status \{name\}[\s\S]*Try again/);
  // W4: ports by count.
  assert.equal(moveReasonWords({ code: 'not_listening', seconds: 20, ports: [3000] }).key, 'slim.reason.not_listening');
  assert.equal(moveReasonWords({ code: 'not_listening', seconds: 20, ports: [3000, 3001] }).key, 'slim.reason.not_listening_many');
  assert.equal(moveReasonWords({ code: 'no_port', seconds: 20, ports: [3000, 3001] }).key, 'slim.reason.no_port_many');
  assert.equal(moveReasonWords({ code: 'no_port', seconds: 20, ports: [3000] }).key, 'slim.reason.not_listening');
  assert.match(enSiteLimitsLane['slim.reason.not_listening_many'], /ports \(\{ports\}\)/);
  assert.equal(enRightsScreens['rights.plan.narrowing'].startsWith('{n} accounts follow'), true);
  assert.equal(enRightsScreens['rights.plan.narrowingReseller'].startsWith('{n} resellers follow'), true);
  // W5: a peak at the limit never claims a refused process.
  for (const words of [enSiteLimitsLane, trSiteLimitsLane]) assert.ok(!/could not start|başlayamadı/.test(words['slim.event.tasks_peak']), words['slim.event.tasks_peak']);
  // F6: the move is refused during the update, in the page's words (agent code and panel refusal).
  assert.equal(moveWhyKey({ code: 'update_running' }), 'slim.why.update_running');
  assert.equal(siteLimitsLaneTranslation('tr', 'err.SERVICE_APP.site_limits_update_running'), trSiteLimitsLane['err.SERVICE_APP.site_limits_update_running']);
  assert.match(enSiteLimitsLane['slim.why.update_running'], /update is in progress[\s\S]*after the update/);
});

test('an update is not started during a move, and the page says what to do', () => {
  assert.match(enErrorsScreens['err.PANEL_UPDATE_SITE_MOVE_RUNNING'], /being moved[\s\S]*Wait until the move’s window has finished[\s\S]*again/);
  assert.match(trErrorsScreens['err.PANEL_UPDATE_SITE_MOVE_RUNNING'], /taşınıyor[\s\S]*bekleyin[\s\S]*yeniden/);
});

test('the duplicate sign-in says what is true: neither is used, a whole-domain cookie stays and is ignored', () => {
  assert.ok(!/removed both/.test(enSetupScreens['accessHold.signinDuplicate']));
  assert.ok(!/ikisini de sildi/.test(trSetupScreens['accessHold.signinDuplicate']));
  assert.match(enSetupScreens['accessHold.signinDuplicate'], /used neither[\s\S]*stays in the browser[\s\S]*ignores it/);
  assert.match(trSetupScreens['accessHold.signinDuplicate'], /ikisini de kullanmadı[\s\S]*tarayıcıda kalır[\s\S]*yok sayar/);
  assert.ok(!/they were removed/.test(read('../../cmd/panel/signin_cookies.go')));
});
