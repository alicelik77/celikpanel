// Notifications by recipient (alpha.128, G18): one list of the person's own
// notices, "Needs your action" first; the email choice; the administrator's
// server-wide settings. Only the contract is read; only a local fix path is
// ever followed; every text is in English and Turkish, in the screen's own
// lazy copy (the boot catalogue does not grow).
//
// Alıcıya göre bildirimler: tek liste, önce yapılması gerekenler; yalnız
// yerel düzeltme yolu izlenir; her metin iki dilde ve tembel pakette.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource } from './fixtures/shared-layer.mjs';
import { enNotifications } from '../src/i18n/notifications/en.ts';
import { trNotifications } from '../src/i18n/notifications/tr.ts';

const usageURL = compileSource('lib/usage.ts', (specifier) => {
  throw new Error(`lib/usage.ts imports ${specifier}`);
});
const lib = await import(compileSource('lib/notifications.ts', (specifier) => {
  if (specifier.endsWith('/usage')) return usageURL;
  throw new Error(`lib/notifications.ts imports ${specifier}`);
}));
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

const words = { title_en: 'Your subscription ends', text_en: 'Nothing is deleted.', title_tr: 'Aboneliğiniz bitiyor', text_tr: 'Hiçbir şey silinmez.' };

test('the list reads only the contract; an unknown entry is left out, a broken answer is unknown', () => {
  assert.throws(() => lib.decodeNotifications({}));
  assert.throws(() => lib.decodeNotifications({ items: 'x' }));
  const report = lib.decodeNotifications({
    items: [
      { source: 'account', id: 1, kind: 'expiry_soon', audience: 'customer', needs_action: false, fix_path: '/account', created_at: '2026-11-07 10:00:00', words },
      { source: 'account', id: 2, kind: 'expiry_today', audience: 'reseller', needs_action: true, fix_path: 'https://evil.example/', created_at: '2026-11-07 11:00:00', words },
      { source: 'account', id: 3, kind: 'x', audience: 'customer', needs_action: true, words: { title_en: '' } },
      { source: 'usage', id: 4, kind: 'disk_100', audience: 'customer', needs_action: true, fix_path: '//evil.example', created_at: '2026-11-07 09:00:00', usage: { used_bytes: 3, limit_bytes: 2, period: '2026-11-01' } },
      { source: 'usage', id: 5, kind: 'not_a_kind', audience: 'customer', usage: {} },
      { source: 'other', id: 6, kind: 'expiry_soon' },
    ],
  });
  assert.deepEqual(report.items.map((item) => `${item.source}:${item.id}:${item.fix_path}`), ['account:1:/account', 'account:2:', 'usage:4:']);
  assert.equal(report.monitoring, undefined, 'only an administrator gets the alerts line');
  assert.deepEqual(lib.decodeNotifications({ items: [], monitoring: { open: 2, unseen: -1 } }).monitoring, { open: 2, unseen: 0 });
  assert.equal(report.items[2].usage.kind, 'disk_100');
});

test('"Needs your action" comes first, newest first in each group', () => {
  const report = lib.decodeNotifications({
    items: [
      { source: 'account', id: 1, kind: 'a', needs_action: false, created_at: '2026-11-07 10:00:00', words },
      { source: 'account', id: 2, kind: 'b', needs_action: true, created_at: '2026-11-06 10:00:00', words },
      { source: 'account', id: 3, kind: 'c', needs_action: true, created_at: '2026-11-07 10:00:00', words },
    ],
  });
  const groups = lib.groupNotifications(report.items);
  assert.deepEqual(groups.action.map((item) => item.id), [3, 2]);
  assert.deepEqual(groups.info.map((item) => item.id), [1]);
});

test('the email choice and the settings read only their contracts', () => {
  assert.throws(() => lib.decodeNotificationPreferences({ mode: 'sometimes' }));
  assert.deepEqual(lib.decodeNotificationPreferences({ mode: 'action', extra_email: 'desk@example.com', own_email_set: true, mail_available: false }),
    { mode: 'action', extra_email: 'desk@example.com', own_email_set: true, mail_available: false });
  assert.throws(() => lib.decodeNotificationSettings({ expiry_days_before: 10 }));
  const settings = lib.decodeNotificationSettings({ expiry_days_before: 10, expiry_choices: [3, 7, 'x', 10], extra: { monitoring: 'noc@example.com' } });
  assert.deepEqual(settings.expiry_choices, [3, 7, 10]);
  assert.deepEqual(settings.extra, { monitoring: 'noc@example.com', account: '', security: '' });
  for (const path of ['/users?rights=a', '/account']) assert.equal(lib.localFixPath(path), path);
  for (const path of ['https://x', '//x', '/\\x', 'account', 5]) assert.equal(lib.localFixPath(path), '');
});

test('every text has English and Turkish with the same values', () => {
  const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
  assert.deepEqual(Object.keys(trNotifications).sort(), Object.keys(enNotifications).sort());
  for (const key of Object.keys(enNotifications)) {
    assert.ok(enNotifications[key] && trNotifications[key], key);
    assert.notEqual(trNotifications[key], enNotifications[key], `${key} is not translated`);
    assert.deepEqual(placeholders(trNotifications[key]), placeholders(enNotifications[key]), key);
  }
});

test('screens: one Notifications list on the Dashboard and My account, lazily loaded; no browser dialogs', () => {
  const component = source('components/Notifications.tsx');
  for (const marker of ['data-notifications-group="action"', 'data-notifications-group="info"', 'data-notification-fix', "t('notif.seen')", 'data-notification-mode', 'data-notification-group']) {
    assert.ok(component.includes(marker), `Notifications lacks ${marker}`);
  }
  assert.doesNotMatch(component, /window\.(confirm|alert|prompt)|\b(confirm|alert|prompt)\(/);
  for (const page of ['components/Dashboard.tsx', 'components/AccountPage.tsx']) {
    const text = source(page);
    assert.match(text, /lazy\(\(\) => import\('\.\/Notifications'\)/, `${page} does not load the list lazily`);
    assert.doesNotMatch(text, /from '\.\/Notifications'/, `${page} imports the list into its own chunk`);
  }
  assert.match(source('components/AccountPage.tsx'), /role === 'admin' && <Suspense fallback=\{null\}><NotificationSettingsCard \/>/);
  // The usage notices are in the one list now, not twice on one page.
  assert.ok(!source('components/MyPlanCard.tsx').includes('<UsageNotices'), 'My plan still lists the usage notices');
  // The copy is route-scoped: the boot catalogue does not carry it.
  assert.ok(!source('i18n/en.ts').includes("'notif."), 'notification words were added to the boot catalogue');
});

// Integration (alpha.128): the one list also reads the lowered-limit notices
// (rights_notices) and the outgoing mail notices (mail_outgoing_notices)
// without copying them; "seen" goes back to the store that owns each one,
// and the separate blocks that showed them are gone.
// Tek liste sınır ve giden posta bildirimlerini de okur; "görüldü" sahibine gider.
test('lowered-limit and outgoing mail notices join the one list, seen in their own stores', () => {
  const report = lib.decodeNotifications({
    items: [
      { source: 'site_limit', id: 7, kind: 'site_limit_clamped', audience: 'customer', needs_action: false, fix_path: '/domains/example.com?tab=php', created_at: '2026-11-07 10:00:00',
        clamp: { domain: 'example.com', key: 'php.memory_limit_mb', value: 1024, limit: 512, set_by: 'plan', set_by_name: 'Small' } },
      { source: 'site_limit', id: 8, kind: 'site_limit_clamped', clamp: { domain: '', key: 'php.memory_limit_mb', value: 1, limit: 1 } },
      { source: 'mail_outgoing', id: 9, kind: 'paused', audience: 'customer', needs_action: true, fix_path: '/domains/example.com?tab=mail', created_at: '2026-11-07 11:00:00', words },
      { source: 'mail_outgoing', id: 10, kind: 'limit_reached', words: { title_en: 'x' } },
    ],
  });
  assert.deepEqual(report.items.map((item) => `${item.source}:${item.id}`), ['site_limit:7', 'mail_outgoing:9']);
  assert.equal(report.items[0].clamp.set_by_name, 'Small');
  assert.deepEqual(lib.groupNotifications(report.items).action.map((item) => item.id), [9]);
  assert.equal(lib.notificationSeenPath(report.items[0]), '/api/v1/rights/notices/7/acknowledge');
  assert.equal(lib.notificationSeenPath(report.items[1]), '/api/v1/mail-outgoing-notices/9/seen');
  assert.equal(lib.notificationSeenPath({ source: 'account', id: 3 }), '/api/v1/notifications/3/seen');
  assert.equal(lib.notificationSeenPath({ source: 'usage', id: 4 }), '/api/v1/usage-notices/4/seen');
  const component = source('components/Notifications.tsx');
  assert.ok(component.includes('notificationSeenPath(item)'), 'the list does not send "seen" to the owning store');
  assert.ok(component.includes("'limits.notice.line'"), 'the lowered-limit notice is not worded with the limits words');
  for (const [page, block] of [['components/MyPlanCard.tsx', 'SiteLimitNotices'], ['components/RightsNotices.tsx', 'SiteLimitNotices'], ['components/PostfixManagement.tsx', 'OutgoingMailNotices']]) {
    assert.ok(!source(page).includes(block), `${page} still shows ${block} beside the one list`);
  }
  // The PHP page keeps marking a value a lowered limit brought down.
  assert.ok(source('components/SitePHPLimits.tsx').includes('data-limit-clamped'), 'the PHP page lost its marker of a lowered value');
});
