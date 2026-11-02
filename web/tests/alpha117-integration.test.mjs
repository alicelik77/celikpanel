// alpha.117 integration: a subscription's end date on the rights panel and
// the Accounts list's Rights column (release owner's decisions e and h).
//
// alpha.117 birleştirme: abonelik bitiş tarihi ve Hesaplar listesindeki
// Haklar sütunu.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { apiErrorURL, compileSource } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const rightsURL = compileSource('lib/rights.ts', (specifier) => {
  if (specifier.endsWith('/apiError')) return apiErrorURL;
  throw new Error(`lib/rights.ts imports ${specifier}`);
});
const rights = await import(rightsURL);

const view = {
  subscription_id: 12, state: 'unlimited', own: { all_permissions: true, all_software: true, permissions: [], software: [] },
  permissions: [], software: [],
  limits: { max_domains: 0, max_databases: 0, max_email_accounts: 0, disk_quota_mb: 0 },
};

test('the end date is read from the lifecycle; none means it never ends', () => {
  assert.equal(rights.subscriptionEndsAtURL(12), '/api/v1/subscriptions/12/ends-at');
  const set = rights.decodeSubscriptionRights({ ...view, lifecycle: { status: 'active', ends_at: '2027-01-31T00:00:00Z' } });
  assert.equal(set.ends_at, '2027-01-31T00:00:00Z');
  assert.equal(rights.decodeSubscriptionRights({ ...view, lifecycle: { status: 'active', ends_at: null } }).ends_at, null);
  assert.equal(rights.decodeSubscriptionRights(view).ends_at, null);
});

test('the end date field sends one PUT and rereads; a passed date is said before saving', () => {
  const panel = source('components/AccountRights.tsx');
  assert.match(panel, /<EndDateField subscriptionId=\{subscriptionId\} endsAt=\{view\.ends_at\} \/>/);
  assert.match(panel, /sendRights\(subscriptionEndsAtURL\(subscriptionId\), 'PUT', \{ ends_at: next \}\)/);
  assert.match(panel, /invalidate\('rights', 'users', 'suspension'\)/);
  assert.match(panel, /rights\.endsAt\.pastWarning/);
});

test('the Accounts table has a Rights column naming Unlimited, Plan: X or Custom', () => {
  const page = source('components/UsersPage.tsx');
  assert.match(page, /t\('users\.col\.rights'\)/);
  assert.match(page, /accountRightsText\(u\.rights, u\.role === 'reseller', t\)/);
  const t = (key, vars) => String(enScreens[key] ?? key).replace(/\{(\w+)\}/g, (_, n) => String(vars?.[n] ?? ''));
  assert.equal(rights.stateName('plan', 'Gold', t), 'Plan: Gold');
  assert.equal(rights.stateName('unlimited', undefined, t, true), 'Unlimited reseller');
});

test('every new key exists in English and Turkish', () => {
  const keys = [
    'users.col.rights', 'users.rights.mixed',
    'rights.endsAt.title', 'rights.endsAt.never', 'rights.endsAt.current', 'rights.endsAt.save', 'rights.endsAt.clear',
    'rights.endsAt.hint', 'rights.endsAt.pastWarning', 'rights.endsAt.saved', 'rights.endsAt.cleared', 'rights.endsAt.lost',
    'err.SUBSCRIPTION_ENDS_AT_INVALID', 'dashboard.audit.action.subscription.ends_at', 'susp.preview.apps',
  ];
  for (const key of keys) {
    assert.equal(typeof enScreens[key], 'string', `en ${key}`);
    assert.equal(typeof trScreens[key], 'string', `tr ${key}`);
    assert.notEqual(enScreens[key], trScreens[key], `tr ${key} is not translated`);
  }
});
