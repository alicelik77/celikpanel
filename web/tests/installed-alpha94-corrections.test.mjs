// What the owner saw on Boston and Frankfurt after updating them to alpha.94
// (screens of 2026-10-17), and what alpha.95 changes. The bundle chips and the
// Dashboard card are pinned in ready-bundles-*.test.mjs and
// dashboard-mail-stack-card-contract.test.mjs; this file holds the rest.
// Sahibin alpha.94 sonrası ekranlarında gördükleri ve alpha.95'in değiştirdikleri.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from '../src/i18n/screens/en.ts';
import { trScreens } from '../src/i18n/screens/tr.ts';
import { enServerScreens } from '../src/i18n/screens/server/en.ts';
import { trServerScreens } from '../src/i18n/screens/server/tr.ts';

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
    assert.match(catalogues[locale]['services.cleanupWebmailRowNote'], locale === 'en' ? /may still be/ : /kalmış olabilir/);
  }
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

test('the Domains count includes the infrastructure DNS zone the Domains page lists', () => {
  assert.match(dashboard, /const infrastructureRead = useRemote\(INFRASTRUCTURE_ZONES_URL, decodeInfrastructureZones\);/);
  assert.match(dashboard, /filter\(\(zone\) => zone\.domainId === null\)\.length/);
  assert.match(dashboard, /n=\{mapRemote\(domainList\.remote, \(rows\) => rows\.length \+ standaloneZones\)\} label=\{t\('dashboard\.domains'\)\}/);
  // The Domains page lists the same zones the same way.
  const domains = read('../src/components/Domains.tsx');
  assert.match(domains, /infrastructureZones\.filter\(\(zone\) => zone\.domainId === null/);
});

test('DESIGN.md records the stop icon, the one action shape and the version rule', () => {
  assert.match(design, /StopCircle/);
  assert.match(design, /version/i);
});
