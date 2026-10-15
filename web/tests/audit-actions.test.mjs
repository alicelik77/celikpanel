// Recent activity says every audit action in the interface language (set16
// observation O3: the Dashboards of both installed servers showed raw ids such
// as server.setup.child:nginx and server_setup_mail_enrollment_requested). The
// list of action names is read from the Panel's own source (auditActionNames
// in cmd/panel/audit_actions.go, which a Go test derives from every
// audit_logs insert), so an action the Panel can write without words here
// fails this test instead of reaching the screen as its id.
//
// Son etkinlik her denetim eylemini arayüz dilinde söyler. Eylem adları
// Panel'in kendi kaynağından okunur; sözcüğü olmayan bir eylem bu testte kalır.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { trScreens } from '../src/i18n/screens/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const compile = (path) => ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const nameURL = moduleURL(compile('../src/lib/componentName.ts'));
const lib = await import(moduleURL(compile('../src/lib/auditAction.ts').replace("from './componentName'", `from '${nameURL}'`)));
const catalogues = { en: { ...en, ...enScreens, ...enServerScreens }, tr: { ...tr, ...trScreens, ...trServerScreens } };
const say = (locale) => (key, vars = {}) => {
  let text = catalogues[locale][key] ?? key;
  for (const [name, value] of Object.entries(vars)) text = text.replaceAll(`{${name}}`, String(value));
  return text;
};

const goList = read('../../cmd/panel/audit_actions.go');
const body = /var auditActionNames = \[\]string\{\n([\s\S]*?)\n\}/.exec(goList)?.[1] ?? '';
const names = [...body.matchAll(/^\t"([^"]+)",$/gm)].map((m) => m[1]);
const placeholders = (text) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();
const rawToken = /[a-z0-9]+(?:[._][a-z0-9]+)+/;

test('every action name the Panel can write has an English and a Turkish sentence', () => {
  assert.ok(names.length >= 150, `read ${names.length} action names from the Panel`);
  for (const name of names) {
    const key = `dashboard.audit.action.${name}`;
    assert.ok(catalogues.en[key], `no English sentence for ${name}`);
    assert.ok(catalogues.tr[key], `no Turkish sentence for ${name}`);
    assert.notEqual(catalogues.tr[key], catalogues.en[key], `${name} is not translated`);
    assert.deepEqual(placeholders(catalogues.tr[key]), placeholders(catalogues.en[key]), `${name}: the two sentences use different values`);
    for (const value of placeholders(catalogues.en[key])) assert.equal(value, 'target', `${name} uses {${value}}`);
    for (const locale of ['en', 'tr']) assert.doesNotMatch(catalogues[locale][key], rawToken, `${locale} ${name} shows an id`);
  }
  // No words for an action the Panel does not write.
  const listed = new Set(names);
  for (const locale of ['en', 'tr']) {
    for (const key of Object.keys(catalogues[locale]).filter((item) => item.startsWith('dashboard.audit.action.'))) {
      assert.ok(listed.has(key.slice('dashboard.audit.action.'.length)), `${locale} has words for ${key}, which the Panel does not write`);
    }
  }
});

test('every listed action reads as a sentence with its target, in both languages', () => {
  for (const name of names) {
    for (const locale of ['en', 'tr']) {
      const sample = `${name}:example.com — detail that stays out`;
      const said = lib.auditActionText(sample, say(locale));
      assert.notEqual(said, lib.humaniseAuditAction(sample), `${locale} ${name} fell back`);
      assert.doesNotMatch(said, /detail that stays out|\{target\}/);
      if (catalogues[locale][`dashboard.audit.action.${name}`].includes('{target}')) assert.match(said, /example\.com/);
    }
  }
});

test('the ids the owner saw read as sentences', () => {
  const cases = [
    ['server.setup.child:nginx', 'prepared Nginx during setup', 'kurulum sırasında Nginx bileşenini hazırladı'],
    ['server.setup.child:nftables', 'prepared Firewall engine during setup', 'kurulum sırasında Güvenlik duvarı motoru bileşenini hazırladı'],
    ['server_setup_mail_enrollment_requested', 'requested mail certificate renewal during setup', 'kurulum sırasında posta sertifikası yenilemesini istedi'],
    ['server_setup_mail_certificate_requested', 'requested the mail certificate during setup', 'kurulum sırasında posta sertifikasını istedi'],
    ['panel.certificate:boston.example.com', 'set up the panel certificate for boston.example.com', 'boston.example.com için panel sertifikasını kurdu'],
    ['mail.profile.install.failed:core-mail — submission verification failed', 'could not install and verify Core Mail', 'Temel Posta profilini kurup doğrulayamadı'],
    ['auth.login', 'signed in', 'oturum açtı'],
    ['service.restart.unknown:nginx — agent timeout', 'asked Nginx to restart; the result is not known', 'Nginx hizmetini yeniden başlatmayı istedi; sonucu bilinmiyor'],
    ['user.create:alice/reseller', 'created the user alice', 'alice kullanıcısını oluşturdu'],
    ['domain.delete.pending:example.com:dns_cleanup', 'started deleting the domain example.com; the cleanup is still in progress', 'example.com alan adını silmeye başladı; temizlik henüz sürüyor'],
    ['dns.engine.switch.succeeded request=a switch=b source=bind target=pdns action=switch mode=x', 'changed the DNS engine', 'DNS motorunu değiştirdi'],
    ['backup.create:scheduled:full', 'created a backup', 'bir yedek oluşturdu'],
  ];
  for (const [action, english, turkish] of cases) {
    assert.equal(lib.auditActionText(action, say('en')), english, action);
    assert.equal(lib.auditActionText(action, say('tr')), turkish, action);
  }
});

test('an action this page has no words for is said as words, never as its raw id', () => {
  for (const action of ['from.a_newer:panel', 'something_new_happened', 'brand.new.thing:x — why', 'Odd Action']) {
    const said = lib.auditActionText(action, say('tr'));
    assert.doesNotMatch(said, /[._:]/, `${action} → ${said}`);
    assert.ok(said.length > 0);
  }
  assert.equal(lib.auditActionText('from.a_newer:panel — reason', say('en')), 'from a newer panel');
  // The target is kept as written: a domain stays a domain (browser batch 14,
  // 2026-10-15: "backup offsite verify partial example com" on the Dashboard).
  assert.equal(lib.auditActionText('backup.offsite.verify_partial:example.com — 2 of 3', say('tr')), 'backup offsite verify partial example.com');
  assert.equal(lib.auditActionText('brand.new.thing:203.0.113.10', say('en')), 'brand new thing 203.0.113.10');
  // A sentence that needs a target the action lacks is not drawn half-empty.
  assert.equal(lib.auditActionText('domain.delete', say('en')), 'domain delete');
});

test('the Dashboard says actions through the library', () => {
  const dashboard = read('../src/components/Dashboard.tsx');
  assert.match(dashboard, /import \{ auditActionText, auditActorName \} from '\.\.\/lib\/auditAction';/);
  assert.match(dashboard, /\{auditActionText\(e\.action, t\)\}/);
  assert.ok(!/function auditActionText/.test(dashboard), 'the Dashboard keeps its own partial mapping');
});

test('only the reserved system actor is translated; user names never', () => {
  assert.equal(lib.auditActorName('system', say('en')), 'System');
  assert.equal(lib.auditActorName('system', say('tr')), 'Sistem');
  for (const name of ['admin', 'alice', 'System', 'systemd', 'sistem', '']) {
    assert.equal(lib.auditActorName(name, say('tr')), name);
  }
  const dashboard = read('../src/components/Dashboard.tsx');
  assert.ok(!/\{e\.username\}/.test(dashboard), 'the Dashboard draws the actor raw');
  assert.match(dashboard, /auditActorName\(e\.username, t\)/);
  assert.ok(!/\{e\.username\}/.test(read('../src/components/AuditLogPage.tsx')), 'the audit log draws the actor raw');
});
