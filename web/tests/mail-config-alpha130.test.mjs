// alpha.130: mail programs that set themselves up (the mailbox screen) and
// sending mail through another server (the Postfix page). The words exist in
// both languages, the screens load with their own pages, the password is
// never read back, and the decoder fails closed.
//
// Kendini ayarlayan posta programları ve aktarma sunucusu: iki dilde metin,
// parola geri okunmaz, çözücü kapalı düşer.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { compileSource, webmailAddressLibURL } from './fixtures/shared-layer.mjs';
import { enMailConfig } from '../src/i18n/mailConfig/en.ts';
import { trMailConfig } from '../src/i18n/mailConfig/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

test('every mail configuration word exists in English and Turkish with the same values', () => {
  assert.deepEqual(Object.keys(trMailConfig).sort(), Object.keys(enMailConfig).sort());
  for (const [key, english] of Object.entries(enMailConfig)) {
    assert.ok(english.trim(), `${key} is empty`);
    assert.ok(trMailConfig[key].trim(), `${key} has no Turkish`);
    if (!/port\d+$/.test(key)) assert.notEqual(trMailConfig[key], english, `${key} is not translated`);
    assert.deepEqual(placeholders(trMailConfig[key]), placeholders(english), `${key}: different values`);
  }
});

test('the words load with the mail screens, never on the boot path', () => {
  assert.doesNotMatch(read('../src/i18n/index.tsx'), /mailConfig/);
  assert.doesNotMatch(read('../src/i18n/en.ts'), /import \{ enMailConfig/);
  assert.match(read('../src/i18n/en.ts'), /import type \{ MailConfigKey \} from '\.\/mailConfig\/en';/);
});

test('the relay section never reads a password back and asks before it changes the server', () => {
  const relay = read('../src/components/MailRelaySettings.tsx');
  assert.match(relay, /<PasswordInput /);
  assert.doesNotMatch(relay, /window\.(confirm|alert)|\balert\(/);
  assert.match(relay, /await confirmAction\(\{/);
  assert.match(relay, /sendWithSteps\(report, flowSteps\(t, 'mailRelay', \['check!', 'save', 'apply'\]\)/);
  // The view has no password field; only whether one is saved.
  assert.match(relay, /interface Settings \{ enabled: boolean; host: string; port: number; username: string; has_password: boolean \}/);
  assert.doesNotMatch(relay, /settings\.password|view\.password/);
  // The list refreshes by itself after a save.
  assert.match(relay, /refreshRemote\(MAIL_RELAY_URL\)/);
  assert.match(read('../src/components/PostfixManagement.tsx'), /<MailRelaySettings \/>/);
});

test('the set-up notice sits above the manual settings and finishes with one confirmed press', () => {
  const panel = read('../src/components/MailSettingsPanel.tsx');
  assert.match(panel, /\{value\.autoconfig && <MailAutoconfigNotice domainId=\{domainId\} autoconfig=\{value\.autoconfig\} readOnly=\{readOnly\} \/>\}\s*<div className="grid/);
  const notice = read('../src/components/MailAutoconfigNotice.tsx');
  assert.match(notice, /await confirmAction\(\{/);
  assert.match(notice, /flowSteps\(t, 'mailAutoconfig', \['check!', 'site', 'dns'\]\)/);
  assert.match(notice, /refreshRemote\(mailSetupURL\(domainId\)\)/);
  // Role-aware: a reseller or customer is told to ask the provider.
  assert.match(notice, /autoconfig\.reason === 'mail_host_unknown' && !isAdmin \? 'mail_host_unknownCustomer'/);
});

test('the mail setup decoder reads autoconfig and fails closed on anything else', async () => {
  const url = compileSource('lib/mailSetup.ts', (specifier) => (specifier === './webmailAddress' ? webmailAddressLibURL : null));
  const { decodeMailSetup, decodeMailAutoconfig } = await import(url);
  const base = {
    mail_host: 'mail.server.example', webmail_available: false,
    imap: { host: 'h', port: 993, security: 'SSL/TLS' }, pop3: { host: 'h', port: 995, security: 'SSL/TLS' }, smtp: { host: 'h', port: 587, security: 'STARTTLS' },
  };
  assert.equal(decodeMailSetup(base).autoconfig, null);
  const setup = decodeMailSetup({ ...base, autoconfig: {
    state: 'partial', reason: '', names: ['autoconfig.example.com', 'autodiscover.example.com'], mail_host: 'mail.server.example',
    served: 'yes', secure: 'no', dns: 'local', can_fix: true,
    records: [{ name: 'autoconfig.example.com', type: 'CNAME', content: 'mail.example.com', state: 'missing' }, { name: 7 }],
  } });
  assert.equal(setup.autoconfig.state, 'partial');
  assert.equal(setup.autoconfig.records.length, 1);
  assert.equal(setup.autoconfig.can_fix, true);
  assert.equal(decodeMailAutoconfig({ state: 'weird', served: 'maybe' }).state, 'unavailable');
  assert.equal(decodeMailAutoconfig({ state: 'weird', served: 'maybe' }).served, 'unknown');
  assert.equal(decodeMailAutoconfig('nope'), null);
});
