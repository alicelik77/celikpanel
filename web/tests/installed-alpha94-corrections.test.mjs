// What the owner saw on Boston and Frankfurt after updating them to alpha.94
// (screens of 2026-10-17), and what alpha.95 changes. The bundle chips and the
// Dashboard card are pinned in ready-bundles-*.test.mjs and
// dashboard-mail-stack-card-contract.test.mjs; this file holds the rest.
// Sahibin alpha.94 sonrası ekranlarında gördükleri ve alpha.95'in değiştirdikleri.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from './locale-catalogue.mjs';
import { trScreens } from './locale-catalogue.mjs';
import { enServerScreens } from './locale-catalogue.mjs';
import { trServerScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const services = read('../src/components/ServiceList.tsx');
const layout = read('../src/components/Layout.tsx');
const dashboard = read('../src/components/Dashboard.tsx');
const design = read('../../DESIGN.md');
const catalogues = { en: { ...en, ...enScreens, ...enServerScreens }, tr: { ...tr, ...trScreens, ...trServerScreens } };

// The cleanup confirmation lists what the Agent removes, by the names the
// Agent uses: a renamed path on either side fails here.
test('the Roundcube cleanup lists the paths the Agent removes, and says what it never touches', () => {
  const agent = read('../../cmd/agent/webmail_rpc.go');
  const files = read('../../cmd/agent/webmail_files_linux.go');
  const transport = read('../../internal/transport/webmail.go');
  const base = /var webmailBaseDir = "([^"]+)"/.exec(agent)?.[1];
  const conf = /webmailConfPath = "([^"]+)"/.exec(agent)?.[1];
  const socket = /const defaultWebmailSocketPath = "([^"]+)"/.exec(transport)?.[1];
  assert.ok(base && conf && socket, 'the Agent names changed');
  assert.match(files, /func roundcubeRetirementName\(base string\) string \{\s*\n\s*return "\." \+ base \+ "\.retired"/);
  assert.match(files, /func roundcubeStageName\(base string\) string \{\s*\n\s*return "\." \+ base \+ "\.stage"/);
  const listed = /const WEBMAIL_CLEANUP_PATHS[\s\S]*?\n\];/.exec(services)?.[0] ?? '';
  const parent = base.slice(0, base.lastIndexOf('/'));
  const name = base.slice(base.lastIndexOf('/') + 1);
  for (const path of [base, conf, socket, `${parent}/.${name}.stage`, `${parent}/.${name}.retired*`, `${parent}/..${name}.stage.retired*`]) {
    assert.ok(listed.includes(`'${path}'`), `${path} is not listed`);
  }
  assert.match(services, /data-webmail-cleanup-untouched=""/);
  for (const locale of ['en', 'tr']) {
    assert.ok(catalogues[locale]['services.cleanupWebmailUntouched'].includes('Maildir'), locale);
    // The scan cannot tell whether such files exist: the row says "may".
    for (const kind of ['install', 'removal']) {
      assert.match(catalogues[locale][`services.cleanupWebmailRowNote.${kind}`], locale === 'en' ? /may still be/ : /kalmış olabilir/);
    }
  }
});

// alpha.95 review: the row offers the cleanup only when the Panel's own record
// shows an unfinished Roundcube install or removal (or a failed webmail
// attempt); Roundcube's own page always reaches it.
test('the leftovers cleanup is offered on the row only with a record, and always from Roundcube\'s page', () => {
  // alpha.96: the Panel folds the webmail attempt into the record (the newer
  // of the Roundcube entry and the webmail attempt decides), so the page
  // offers the cleanup from the record alone, and the deep link removes its
  // ?cleanup with a replace so a reload does not reopen the confirmation.
  assert.match(services, /const webmailCleanupOffered = webmailCleanup !== null;/);
  assert.doesNotMatch(services, /webmailAttemptFailed/);
  assert.match(services, /remaining\.delete\('cleanup'\);[\s\S]{0,200}navigate\(location\.pathname \+ \(query \? `\?\$\{query\}` : ''\) \+ location\.hash, \{ replace: true \}\);/);
  assert.match(services, /\{s\.id === 'roundcube' && s\.is_installed === false && webmailCleanupOffered && \(/);
  assert.match(services, /\{s\.id === 'roundcube' && webmailCleanupOffered && \(/);
  assert.match(services, /const decodeWebmailCleanup = \(value: unknown\): WebmailCleanupRecord \| null => \(value === 'install_failed' \|\| value === 'removal_incomplete' \? value : null\);/);
  assert.match(services, /get\('cleanup'\) !== 'roundcube'/);
  const shell = read('../src/components/ServiceShell.tsx');
  assert.match(shell, /\{serviceId === 'roundcube' && \(\s*\n\s*<button\s*\n\s*type="button"\s*\n\s*onClick=\{\(\) => navigate\('\/services\?cleanup=roundcube'\)\}/);
  const record = read('../../cmd/panel/webmail_cleanup_record.go');
  assert.match(record, /webmailCleanupInstallFailed\s+= "install_failed"/);
  assert.match(record, /webmailCleanupRemovalIncomplete = "removal_incomplete"/);
  assert.match(record, /func webmailCleanupFromAttempt\(status string\) string/);
});

test('a sidebar count is a plain muted number, not a filled pill; null is still a dash, not zero', () => {
  const item = layout.slice(layout.indexOf('{count === null ? ('), layout.indexOf(') : null}', layout.indexOf('{count === null ? (')));
  assert.doesNotMatch(item, /rounded-full|bg-white\/20|bg-sidebar-hover/);
  assert.match(item, /data-nav-count="unknown"/);
  assert.match(item, /<span aria-hidden="true">—<\/span>/);
  assert.match(item, /count !== undefined && count > 0/);
  assert.match(layout, /const navCountTone = \(active: boolean\) => \(active \? 'text-white\/80' : 'text-sidebar-muted'\);/);
});

test('the Dashboard says "nothing needs your attention" in plain words, with what was checked', () => {
  assert.equal(catalogues.en['dashboard.attentionNone'], 'Nothing needs your attention. CelikPanel checked the certificates, the firewall and the installed components.');
  assert.equal(catalogues.tr['dashboard.attentionNone'], 'Dikkat gerektiren bir şey yok. CelikPanel sertifikaları, güvenlik duvarını ve kurulu bileşenleri denetledi.');
  // Said only after every read answered and the components scan is fresh.
  assert.match(dashboard, /t\(serviceScanFresh \? 'dashboard\.attentionNone' : attentionComponentsKey\)/);
});

const zonesLib = await import('data:text/javascript;base64,' + Buffer.from(ts.transpileModule(read('../src/lib/infrastructureZones.ts'), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText).toString('base64'));

test('the Domains count is a number only when both reads answered; it is never domains + 0 for an unread zone list', () => {
  const known = (value, observedAt = 1) => ({ state: 'known', value, observedAt });
  const failed = (previous) => ({ state: 'unknown', reason: { code: 'X', message: 'x' }, ...(previous ? { previous } : {}) });
  const zone = (name, domainId = null) => ({ zone: name, domainId, records: 3, peerRecords: 0 });
  const count = zonesLib.domainsCount;
  assert.deepEqual(count(known([]), known([zone('example.net')])), { state: 'known', value: 1, observedAt: 1 });
  assert.deepEqual(count(known([{}]), known([zone('example.net', 7)])), { state: 'known', value: 1, observedAt: 1 }, 'a zone that became a website is that website');
  assert.equal(count(known([{}]), { state: 'loading' }).state, 'loading');
  const unread = count(known([{}]), failed());
  assert.equal(unread.state, 'unknown');
  assert.equal(unread.previous, undefined, 'an unread zone list is not counted as none');
  assert.deepEqual(count(known([{}], 5), failed({ value: [zone('a.example')], observedAt: 3 })).previous, { value: 2, observedAt: 3 });
  assert.deepEqual(count(known([{}, {}]), null), { state: 'known', value: 2, observedAt: 1 }, 'without the zone read (not an administrator) it is the domains');
  assert.equal(count({ state: 'loading' }, null).state, 'loading');
});

test('the Domains count includes the infrastructure DNS zone the Domains page lists', () => {
  assert.match(dashboard, /const infrastructureRead = useRemote\(INFRASTRUCTURE_ZONES_URL, decodeInfrastructureZones\);/);
  assert.match(dashboard, /n=\{domainsCount\(domainList\.remote, infrastructureRead\.remote\)\} label=\{t\('dashboard\.domains'\)\}/);
  // The sidebar badge counts the same, with the same rule.
  assert.match(layout, /lastKnown\(domainsCount\(domainList\.remote, role === 'admin' \? zoneList\.remote : null\)\)\?\.value/);
  // The Domains page lists the same zones the same way.
  const domains = read('../src/components/Domains.tsx');
  assert.match(domains, /infrastructureZones\.filter\(\(zone\) => zone\.domainId === null/);
});

test('DESIGN.md records the stop icon, the one action shape and the version rule', () => {
  assert.match(design, /StopCircle/);
  assert.match(design, /version/i);
});
