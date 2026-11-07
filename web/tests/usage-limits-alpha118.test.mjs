// Plan limits made real (alpha.118, D-044): the traffic and disk notices,
// the administrator's measurement status, and the words every new state
// uses, in English and Turkish. An answer that is not the contract is unknown,
// never "no notices" or "nothing used".
//
// Plan sınırları gerçek: trafik ve disk bildirimleri, ölçüm durumu ve yeni
// durumların iki dildeki sözleri. Sözleşme dışı yanıt bilinmeyendir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { apiErrorURL, compileSource } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const usage = await import(compileSource('lib/usage.ts', (specifier) => {
  throw new Error(`lib/usage.ts imports ${specifier}`);
}));
const rights = await import(compileSource('lib/rights.ts', (specifier) => {
  if (specifier.endsWith('/apiError')) return apiErrorURL;
  throw new Error(`lib/rights.ts imports ${specifier}`);
}));
const { en: enShell } = await import(new URL('../src/i18n/en.ts', import.meta.url).href);
const { tr: trShell } = await import(new URL('../src/i18n/tr.ts', import.meta.url).href);
const catalogues = { en: { ...enShell, ...enScreens }, tr: { ...trShell, ...trScreens } };
const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => (vars && vars[name] !== undefined ? String(vars[name]) : all));
const say = (locale) => (key, vars) => (catalogues[locale][key] !== undefined ? fill(catalogues[locale][key], vars) : key);
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('notices: only the contract is read; an unknown kind is left out, a broken answer is unknown', () => {
  assert.throws(() => usage.decodeUsageNotices({}));
  assert.throws(() => usage.decodeUsageNotices({ notices: 'x' }));
  const notices = usage.decodeUsageNotices({
    notices: [
      { id: 1, kind: 'traffic_100', audience: 'customer', subscription_id: 9, created_at: 't', body: { subscription: 'web', used_bytes: 2097152, limit_bytes: 1048576, period: '2026-11', cause: 'pool', reseller: 'res' } },
      { id: 2, kind: 'mail_full', audience: 'customer', body: {} },
      { id: 3, kind: 'disk_100', audience: 'administrator', body: { used_bytes: 5, limit_bytes: 4, period: 'p', suspend_on: '2026-11-10' } },
    ],
  });
  assert.deepEqual(notices.map((n) => n.id), [1, 3]);
  assert.equal(notices[0].body.cause, 'pool');
  assert.equal(notices[1].audience, 'administrator');
  assert.equal(usage.formatBytes(2097152), '2 MB');
  assert.equal(usage.formatBytes(1610612736), '1.5 GB');
});

test('status: only what needs a look is named', () => {
  assert.throws(() => usage.decodeUsageStatus({ traffic: {}, disk: {} }));
  const fine = usage.decodeUsageStatus({ traffic: { measuring: 3, unknown: {}, interval_minutes: 5 }, disk: { measured: 3 }, mail_quota: { state: 'enforced' } });
  assert.deepEqual(usage.usageStatusProblems(fine), []);
  const notFine = usage.decodeUsageStatus({
    traffic: { error: 'agent unreachable', logrotate: 'missing_tool', unknown: { vhost_kept: 2 } },
    disk: { unknown: 1, low_space: true },
    mail_quota: { state: 'refused', detail: 'doveconf said no' },
  });
  assert.deepEqual(usage.usageStatusProblems(notFine), ['traffic_collect', 'traffic_unknown', 'logrotate', 'disk_unknown', 'disk_low', 'mail_quota']);
  // A server that has no mail yet needs no look.
  assert.deepEqual(usage.usageStatusProblems(usage.decodeUsageStatus({ traffic: {}, disk: {}, mail_quota: { state: 'no_mail' } })), []);
});

test('every new state says the reason, who acts and how it resumes, in both languages', () => {
  const keys = [
    'rights.qty.traffic', 'rights.qty.trafficHint', 'rights.qty.trafficHintPool',
    'myPlan.traffic', 'myPlan.trafficInterval', 'myPlan.trafficUnknown', 'myPlan.trafficReason.first', 'myPlan.trafficReason.failed', 'myPlan.trafficReason.site',
    'myPlan.trafficLimitedTitle', 'myPlan.trafficLimited', 'myPlan.diskParts', 'myPlan.diskPartial', 'myPlan.diskNotMeasured', 'myPlan.diskUnknown',
    'myPlan.diskFullTitle', 'myPlan.diskFull', 'myPlan.diskFullNoDate',
    'usage.notice.traffic80', 'usage.notice.traffic100', 'usage.notice.traffic100Others', 'usage.notice.traffic100Pool',
    'usage.notice.pool80', 'usage.notice.pool100', 'usage.notice.disk80', 'usage.notice.disk100', 'usage.notice.countdown', 'usage.notice.suspended',
    'usage.status.title', 'usage.status.trafficCollect', 'usage.status.trafficUnknown', 'usage.status.logrotateMissing', 'usage.status.logrotateKept',
    'usage.status.diskUnknown', 'usage.status.diskLow', 'usage.status.mailQuota', 'usage.status.resumes',
    'usage.measured.title', 'usage.measured.body', 'usage.measured.todo', 'susp.cause.disk', 'domain.info.trafficMonth', 'domain.info.trafficUnknown',
  ];
  for (const locale of ['en', 'tr']) {
    for (const key of keys) {
      assert.equal(typeof catalogues[locale][key], 'string', `${locale}: ${key} is missing`);
    }
  }
  const en = say('en');
  // The traffic limit: only the websites close; what keeps working; when it opens.
  const limited = en('myPlan.trafficLimited');
  assert.match(limited, /Mail, databases and file access keep working/);
  assert.match(limited, /1st of next month/);
  assert.match(en('usage.notice.traffic100Pool', { reseller: 'res', used: '2 MB', limit: '1 MB' }), /res/);
  // The overshoot of one collection interval is stated.
  assert.match(en('myPlan.trafficInterval', { minutes: 5 }), /every 5 minutes/);
  assert.match(say('tr')('myPlan.trafficInterval', { minutes: 5 }), /5 dakikada bir/);
  // The disk countdown names the day.
  assert.match(en('myPlan.diskFull', { date: '10/11/2026' }), /suspended on 10\/11\/2026/);
  // A number that is not known never reads as zero.
  assert.match(en('myPlan.trafficUnknown', { reason: en('myPlan.trafficReason.first', { minutes: 5 }) }), /not known/);
  // The traffic line the server writes reads in the owner's language.
  assert.equal(rights.overLimitText('traffic: 1200 MB in use, limit 1000 MB', en), 'Traffic per month: 1200 MB in use, the limit is 1000 MB');
});

test('screens: My plan shows traffic, the page state, the disk countdown and the notices; the rights editor edits traffic', () => {
  const card = source('components/MyPlanCard.tsx');
  for (const marker of ['data-my-plan-traffic=', 'data-my-plan-traffic-limited', 'data-my-plan-disk-full', 'data-my-plan-traffic-interval']) {
    assert.ok(card.includes(marker), `My plan lacks ${marker}`);
  }
  assert.ok(!card.includes('trafficNotMeasured'), 'My plan still says traffic is not measured');
  assert.ok(source('components/RightsSetEditor.tsx').includes("qty('traffic_quota_mb'"), 'the rights editor has no traffic quantity');
  assert.ok(source('components/RightsNotices.tsx').includes('<UsageStatusCard />'), 'the administrator does not see the measurement status');
  assert.ok(source('lib/suspension.ts').includes("causes.disk ? (['disk'] as const)"), 'the disk cause is not named');
});
