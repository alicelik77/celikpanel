// alpha.114 lab run 2, screen findings S-new-1..4 (EN and TR):
// S-new-1 a joined server never offers to host the main domain's website;
// S-new-2 Add domain names the DNS software, never its engine id;
// S-new-3 the Audit log says actions and resources in words;
// S-new-4 a one-server names step does not mention a server that will join.
// Alpha.114 laboratuvar kosusu 2 ekran bulgulari.
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens, trScreens, enServerScreens, trServerScreens } from './locale-catalogue.mjs';

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

test('S-new-1: the ready screen offers the first site only when this server did not join', () => {
  const wizard = read('../src/components/ServerSetup.tsx');
  assert.match(wizard, /const readyPeer = useSetupPeerLink\(initial\.status === 'ready'\);/);
  assert.match(wizard, /const joinedHere = !!joinedLink \|\| readyPeer\.role === 'second';/);
  assert.match(wizard, /const offerFirstSite = firstSiteWanted && !joinedHere && peerKnown;/);
  assert.match(wizard, /\{offerFirstSite && <SetupFirstSite domain=\{readyDomain\} \/>\}/);
});

test('S-new-2: Add domain names PowerDNS or BIND, and nothing for an unknown id', async () => {
  const source = read('../src/components/AddDomainModal.tsx');
  assert.match(source, /domains\.add\.dnsServed', \{ server: dnsServerName\(caps\.dns_server\) \}/);
  assert.doesNotMatch(source, /server: caps\.dns_server/);
  const body = /export function dnsServerName\(id: string\): string \{([\s\S]*?)\n\}/.exec(source)[1];
  const dnsServerName = new Function('id', body);
  assert.equal(dnsServerName('pdns'), 'PowerDNS');
  assert.equal(dnsServerName('bind'), 'BIND');
  assert.equal(dnsServerName('mystery'), '');
  assert.equal(dnsServerName(''), '');
  for (const locale of ['en', 'tr']) assert.doesNotMatch(catalogues[locale]['domains.add.dnsServed'].replace('{server}', 'PowerDNS'), /pdns/);
});

test('S-new-3: the Audit log says actions through the library, never the raw code', () => {
  const page = read('../src/components/AuditLogPage.tsx');
  assert.match(page, /\{auditActionText\(e\.action, t\)\}/);
  assert.doesNotMatch(page, /font-mono text-xs text-fg-muted">\s*\{verb\}/);
  assert.doesNotMatch(page, /#\$\{e\.resource_id \?\? ''\}/);
  const t = say('en');
  assert.notEqual(lib.auditActionText('account.invitation.sent', t), 'account.invitation.sent');
  assert.notEqual(lib.auditActionText('server_setup_mail_enrollment_requested', t), 'server_setup_mail_enrollment_requested');
  for (const locale of ['en', 'tr']) for (const code of ['domain.redirect.add', 'impersonate.stop', 'team_member.create', 'dns.remote.host_record.created']) assert.doesNotMatch(lib.auditActionText(code, say(locale)), /[._]/, `${locale} ${code}`);
});

test('S-new-3: a resource is a word, with a number only when one was recorded', () => {
  assert.equal(lib.auditResourceText('domain', 2, say('en')), 'Domain #2');
  assert.equal(lib.auditResourceText('domain', 2, say('tr')), 'Alan adı #2');
  for (const locale of ['en', 'tr']) {
    const text = lib.auditResourceText('service', undefined, say(locale));
    assert.doesNotMatch(text, /#|service/);
    assert.equal(lib.auditResourceText('service', 0, say(locale)), text);
  }
  assert.equal(lib.auditResourceText('service', undefined, say('en')), 'Service');
  assert.equal(lib.auditResourceText(undefined, undefined, say('en')), '');
  assert.equal(lib.auditResourceText('new_kind', 3, say('en')), 'new kind #3');
});

test('S-new-3: every resource kind the Panel writes has EN and TR words', () => {
  const kinds = new Set();
  for (const file of readdirSync(new URL('../../cmd/panel/', import.meta.url)).filter((name) => name.endsWith('.go') && !name.endsWith('_test.go'))) {
    for (const line of read(`../../cmd/panel/${file}`).split(String.fromCharCode(10))) {
      if (!/\.audit(?:As|Detailed|AsDetailed)?\(/.test(line)) continue;
      const m = /"([a-z_]+)", [^,]+(?:, (?:map|details|nil)[^)]*)?\)?\s*\{?$/.exec(line);
      if (m) kinds.add(m[1]);
    }
  }
  assert.ok(kinds.has('service') && kinds.has('domain') && kinds.has('user'), `read ${[...kinds].join(',')}`);
  kinds.delete('');
  for (const kind of kinds) {
    for (const locale of ['en', 'tr']) assert.ok(catalogues[locale][`audit.resource.${kind}`], `no ${locale} words for resource kind ${kind}`);
  }
});

test('S-new-4: the "server that will join" sentence is read only when a join is planned', () => {
  const steps = read('../src/components/ServerSetupWizardSteps.tsx');
  assert.match(steps, /t\(joinPlanned && !joined \? 'setup\.v2\.names\.mailHelp' : 'setup\.v8\.names\.mailHelpOwn'/);
  assert.match(read('../src/components/ServerSetup.tsx'), /joinPlanned=\{joinFlow && !joinedLink\}/);
  for (const locale of ['en', 'tr']) {
    const own = catalogues[locale]['setup.v8.names.mailHelpOwn'];
    assert.ok(own && own.includes('{domain}'));
    assert.doesNotMatch(own, /join|katıl/i);
  }
});
