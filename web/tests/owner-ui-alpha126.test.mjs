import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { apiErrorURL, compileSource } from './fixtures/shared-layer.mjs';

const { choosableSoftware, groupSoftware, rightsSummary, softwareGroupName } = await import(compileSource('lib/rights.ts', (specifier) => {
  if (specifier.endsWith('/apiError')) return apiErrorURL;
  throw new Error(`lib/rights.ts imports ${specifier}`);
}));

// Owner reports of 6 Nov 2026 (alpha.126), Users → Service plans and the toasts.
// Sahibin 6 Kas 2026 bildirimleri: hizmet planları ve bildirimler.

const read = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');

test('a plan lists only software an account can choose, and never a raw key', () => {
  const installed = [
    'component:bind', 'component:certbot', 'component:cron', 'component:dovecot', 'component:nftables',
    'component:nginx', 'component:postfix', 'component:rspamd', 'component:roundcube', 'component:redis',
    'component:php-fpm:8.3', 'component:node:22', 'component:mariadb', 'component:postgresql:16', 'component:python:3.12',
    'integration:acme:letsencrypt', 'app:wordpress', 'runtime:program',
  ].map((id) => ({ id }));
  const shown = groupSoftware(installed).flatMap((group) => group.items.map((item) => item.id));
  assert.deepEqual(shown.sort(), [
    'app:wordpress', 'component:mariadb', 'component:node:22', 'component:php-fpm:8.3', 'component:postgresql:16',
    'component:python:3.12', 'integration:acme:letsencrypt', 'runtime:program',
  ]);
  for (const internal of ['component:nginx', 'component:bind', 'component:roundcube', 'component:certbot']) {
    assert.equal(choosableSoftware(internal), false, internal);
  }
  // Uploaded programs have their own heading, not "Other software".
  assert.deepEqual(softwareGroupName('runtime'), { key: 'runtime' });
  // The summary counts what the plan lets the account choose; a saved plan
  // that names an internal keeps it (the editor only hides it).
  const t = (key, vars) => `${key}:${vars?.n ?? ''}`;
  const rights = { all_permissions: false, all_software: false, permissions: ['mail'], software: ['component:nginx', 'component:php-fpm:8.3', 'runtime:program'] };
  assert.equal(rightsSummary(rights, t), 'rights.summary.permissions:1 · rights.summary.software:2');
  assert.equal(rights.software.length, 3, 'the saved set was changed');
});

test('while a plan form is open, Add plan is not offered; Save is the form\'s action', () => {
  const users = read('components/UsersPage.tsx');
  assert.match(users, /\{editing === null && \(\s*<Button variant="primary" icon=\{Plus\}[^\n]*data-plan-add=""/);
  assert.match(users, /disabled=\{!plansKnown \|\| editing !== null\} title=\{t\('plans\.edit'\)\}/);
  const reseller = read('components/PlanRights.tsx');
  assert.match(reseller, /\{editing === null && <Button variant="primary" icon=\{Plus\}[^\n]*data-plan-add=""/);
  assert.match(reseller, /disabled=\{!known \|\| editing !== null\} title=\{t\('plans\.edit'\)\}/);
});

test('the same toast is never shown twice while it is on screen', () => {
  const toast = read('components/Toast.tsx');
  assert.match(toast, /const toastKey = \(toast: Toast\) => toast\.key \?\? toast\.type \+ ': ' \+ toast\.message;/);
  assert.match(toast, /if \(onScreen\.has\(id\)\) return;/);
});

// Owner, 6 Nov 2026: "how am I supposed to understand a warning that you
// yourself don't understand". The plan-limit card names the website when the
// server does, says in one sentence what it means, and puts the step and its
// button after it; file names only under Technical details.
test('a website whose traffic is not counted is named, with the step and no file path in the main text', async () => {
  const usage = await import(compileSource('lib/usage.ts', (specifier) => { throw new Error(`lib/usage.ts imports ${specifier}`); }));
  const status = usage.decodeUsageStatus({
    traffic: { unknown: { vhost_kept: 1 }, unknown_sites: [{ domain: 'example.com', reason: 'vhost_kept' }, { reason: 'x' }, 'junk'] },
    disk: {}, mail_quota: { state: 'enforced' },
  });
  assert.deepEqual(status.traffic.unknown_sites, [{ domain: 'example.com', reason: 'vhost_kept' }]);
  assert.deepEqual(usage.usageStatusProblems(status), ['traffic_unknown']);
  // Without names (an older server) the count is still said.
  assert.deepEqual(usage.decodeUsageStatus({ traffic: { unknown: { include_missing: 2 } }, disk: {}, mail_quota: { state: 'enforced' } }).traffic.unknown_sites, []);

  const card = read('components/UsageNotices.tsx');
  assert.match(card, /t\('usage\.status\.trafficSite', \{ name: site\.domain \}\)/);
  assert.match(card, /navigate\(`\/domains\/\$\{encodeURIComponent\(domain\)\}\?tab=config`\)/);
  assert.match(card, /<summary className="cursor-pointer">\{t\('common\.technicalDetails'\)\}<\/summary>/);
  const detail = read('components/DomainDetail.tsx');
  assert.match(detail, /requestedTab === 'ssl' \|\| requestedTab === 'config'/);
  for (const locale of ['en', 'tr']) {
    const copy = readFileSync(new URL(`../src/i18n/screens/planUsage/${locale}.ts`, import.meta.url), 'utf8');
    for (const key of ['usage.status.trafficUnknown', 'usage.status.trafficUnknownOne', 'usage.status.trafficSite', 'usage.status.trafficFix.vhost_kept', 'usage.status.trafficFix.include_missing', 'usage.status.trafficFix.format_file_kept', 'usage.status.trafficFix.log_unreadable', 'usage.status.trafficFix.other', 'usage.measured.body', 'usage.measured.todo']) {
      const line = copy.split('\n').find((text) => text.startsWith(`    '${key}':`));
      assert.ok(line, `${locale}: ${key}`);
      assert.doesNotMatch(line, /\/etc\/|conf\.d|celikpanel-managed/, `${locale}: ${key} names a file in the main text`);
    }
    // The "measured from this update" card is two short sentences.
    for (const key of ['usage.measured.body', 'usage.measured.todo']) {
      const line = copy.split('\n').find((text) => text.startsWith(`    '${key}':`));
      assert.ok(line.length < 140, `${locale}: ${key} is long`);
    }
  }
});
