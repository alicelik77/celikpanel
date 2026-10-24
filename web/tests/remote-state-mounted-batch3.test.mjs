import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, reactURL, remoteURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { readAllowList, scanTree } from './remote-state-ratchet.mjs';
import { fileURLToPath } from 'node:url';
import { en as enShellHalf } from '../src/i18n/en.ts';
import { enErrorsScreens as enErrorsPart } from '../src/i18n/screens/errors/en.ts';
// The coded refusals only screens show are the screen part "errors" since v0.1.0-alpha.98.
const en = { ...enShellHalf, ...enErrorsPart };
import { tr as trShellHalf } from '../src/i18n/tr.ts';
import { trErrorsScreens as trErrorsPart } from '../src/i18n/screens/errors/tr.ts';
const tr = { ...trShellHalf, ...trErrorsPart };
import { trScreens } from './locale-catalogue.mjs';
import { trServerScreens } from './locale-catalogue.mjs';

// NO NEGATIVE UI UNLESS KNOWN, third batch (9 Oct 2026, D-024): the Fail2ban,
// Nginx, PHP and Dovecot pages, the PowerDNS page's file list, and the DNS
// settings before their first read. It follows the two earlier mounted tests
// and stands beside them.
//
// Every screen is mounted with the real shared layer and one of its reads is
// made to hang, to drop, to be refused or to answer with something that is not
// the contract. In each case the screen must not say that something is
// missing, empty or off, and must not offer an enabled control that changes or
// removes. The same read is then answered with a real negative, to show the
// absence above is not an accident of the fixture.
//
// What these screens did before: "No jails active" and "No banned IPs" on a
// server that had both, with 0 beside the tabs; every Nginx value "—" and "No
// rate-limit zones defined"; "No extensions found"; Dovecot's figures "—"
// without a word; a red banner with no Retry in place of the DNS settings.

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const screenFiles = ['Fail2banManagement', 'NginxManagement', 'PHPManagement', 'PHPExtendedConfig', 'DovecotManagement', 'PowerDNSManagement', 'DNSServerSettings'];
// The Components page is mounted for its firewall banner (2026-10-14); it is
// not one of this batch's moved screens.
const icons = new Set(['AlertTriangle']);
// The dashboard is mounted too, for its attention list and the firewall's
// state. It stays on the ratchet's list (its system figures, audit trail and
// component read are not moved), so it is not one of `screenFiles`.
// The ready bundles strip and the component icon map (alpha.94) are drawn by
// the Components page and import their own icons.
for (const name of [...screenFiles, 'Dashboard', 'DNSEngineCard', 'ServiceList', 'FirewallPanel', 'ReadyBundleStrip', 'LifeLine', '../lib/componentIcons']) {
  for (const match of source(`components/${name}.tsx`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const icon = item.replace(/\btype\b/, '').trim().split(/\s+as\s+/)[0];
      if (icon && icon !== 'LucideIcon') icons.add(icon);
    }
  }
}

const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  // A test may give real sentences for some keys (globalThis.batch3Words);
  // every other key is shown as itself.
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  const i18n = { t: (key, vars) => (globalThis.batch3Words?.[key] !== undefined ? fill(globalThis.batch3Words[key], vars) : vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => (path) => { globalThis.batch3Navigated?.push(path); return true; };
  export const Link = (props) => React.createElement('a', null, props.children);
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
  export const ServiceShell = (props) => React.createElement('main', null, props.children);
  export const ComponentPanels = () => React.createElement('aside', null, 'component-panels');
  export const HelpButton = () => null;
  // alpha.107: the repository card has its own mounted tests.
  export const RepoCard = () => null;
  export const IsolationFact = () => null, AppAccountLine = () => null;
  export const DNSEngineCard = () => React.createElement('aside', null, 'dns-engine-card');
  export const ServerSetupDNSAccess = () => null, ServerSetupDNSManagement = () => null;
  export const dnsEngineText = (locale, key) => key;
  export const dnsEngineIdentityStagesATakeover = () => false, dnsEngineMutationsHeld = () => false, dnsEngineIdentityReviewLocked = () => false;
  // A dialog is drawn only where a test asks for it (2026-10-16).
  export const Dialog = (props) => (globalThis.batch3Dialogs ? React.createElement('dialog', null, props.title, props.description, props.children, props.footerLead, props.actions) : null);
  // The settings form is mounted on the route that stages the names before an
  // engine is chosen; its first read is what this test withholds.
  export const dnsEngineSettingsFlow = () => 'identityStaging', exactStagedIdentityIsCurrent = () => true;
  export const api = {
    getConfig: async () => ({ Content: '' }),
    getSystemStats: async () => ({ hostname: 'server1', uptime_seconds: 3600, cpu_percent: 5, cpu_cores: 2, load_avg: [0.1, 0.1, 0.1], mem_used_bytes: 1, mem_total_bytes: 4, disk_used_bytes: 1, disk_total_bytes: 4 }),
  };
  // What the dashboard imports beside the shared layer.
  export const useAuth = () => ({ role: 'admin', user: { username: 'admin', role: 'admin' } });
  export const ServerSetupDashboardNotice = () => null, LicenseNotice = () => null, PageHeader = () => null;
  // The Dashboard's quiet update row (alpha.96) is its own component.
  export const DashboardUpdateNotice = () => null;
  export const FirewallNoSSHAcknowledgement = () => null, readFirewallSSHReason = () => null, FirewallSSHReasonLine = () => null;
  // What the Components page imports beside these.
  export const useLocation = () => ({ hash: '', pathname: '/services' });
  export const ServiceActionNotice = () => null, isServiceActionOutcome = () => false;
  // The mail-stack card is drawn only where a test asks for it.
  export const accountStart = () => null;
  // What components/ComponentOperation.tsx imports and never calls here: only
  // its decoder is used, and that is the real one.
  export const createPortal = () => null;
  export const scanRefusedBySetup = () => false;
  export const useNavigationBlocker = () => {};
  // The setup state the Components page reads to add mail through setup
  // (2026-10-16); none unless a test gives one.
  export const useServerSetup = () => globalThis.batch3Setup ?? null;
`);
const shared = sharedLayer(stub);
const lib = (path, more = {}) => shared.compile(path, more);
const operationURL = lib('components/ComponentOperation.tsx');
const managedURL = lib('lib/managedServices.ts', { '/remote': remoteURL, '/ComponentOperation': operationURL });
const own = { '/lib/managedServices': managedURL };
const pick = async (url, name) => (await import(url))[name];
const iniURL = lib('components/PHPExtendedConfig.tsx', own);
const Fail2banManagement = await pick(lib('components/Fail2banManagement.tsx', own), 'Fail2banManagement');
const NginxManagement = await pick(lib('components/NginxManagement.tsx', own), 'NginxManagement');
const PHPManagement = await pick(lib('components/PHPManagement.tsx', { ...own, '/PHPExtendedConfig': iniURL }), 'PHPManagement');
const PHPExtendedConfig = await pick(iniURL, 'PHPExtendedConfig');
const DovecotManagement = await pick(lib('components/DovecotManagement.tsx', own), 'DovecotManagement');
const PowerDNSManagement = await pick(lib('components/PowerDNSManagement.tsx', own), 'PowerDNSManagement');
const DNSServerSettings = await pick(lib('components/DNSServerSettings.tsx', own), 'DNSServerSettings');
const { refreshRemote, useRemote } = await import(remoteURL);
// The dashboard, with the real reader of the firewall, the real decoder of the
// component records and the real mail summary.
const firewallURL = lib('lib/firewall.ts', { '/remote': remoteURL });
const { decodeFirewallStatus, useFirewallStatus } = await import(firewallURL);
const Dashboard = await pick(lib('components/Dashboard.tsx', {
  '/lib/firewall': firewallURL,
  '/lib/accounts': lib('lib/accounts.ts', { '/remote': remoteURL }),
  '/lib/mailProfileText': lib('lib/mailProfileText.ts'),
  '/lib/componentName': lib('lib/componentName.ts'),
  '/lib/componentCensus': lib('lib/componentCensus.ts'),
  '/lib/readyBundles': lib('lib/readyBundles.ts', { '/mailProfileText': lib('lib/mailProfileText.ts') }),
  '/lib/infrastructureZones': lib('lib/infrastructureZones.ts'),
  '/lib/auditAction': lib('lib/auditAction.ts', { '/componentName': lib('lib/componentName.ts') }),
  '/ComponentOperation': operationURL,
}), 'Dashboard');
// The Components page, with the real decoders of ComponentOperation and an
// operation tracker that is idle.
const ServiceList = await pick(lib('components/ServiceList.tsx', {
  '/ComponentOperation': dataModule(`
    export * from '${operationURL}';
    const idle = { startInstall: async () => true, catalogSnapshot: null, locked: false, checking: false, operation: null, failure: null };
    export const useComponentOperation = () => idle;
  `),
  '/lib/componentCensus': lib('lib/componentCensus.ts'),
  '/lib/mailProfileText': lib('lib/mailProfileText.ts'),
  '/lib/componentName': lib('lib/componentName.ts'),
  '/lib/serverSetup': lib('lib/serverSetup.ts'),
  '/WebmailRepairButton': lib('components/WebmailRepairButton.tsx'),
  '/lib/readyBundles': lib('lib/readyBundles.ts', { '/mailProfileText': lib('lib/mailProfileText.ts') }),
  '/lib/componentIcons': lib('lib/componentIcons.tsx'),
  '/ReadyBundleStrip': lib('components/ReadyBundleStrip.tsx', {
    '/lib/mailProfileText': lib('lib/mailProfileText.ts'),
    '/lib/componentName': lib('lib/componentName.ts'),
    '/lib/readyBundles': lib('lib/readyBundles.ts', { '/mailProfileText': lib('lib/mailProfileText.ts') }),
    '/lib/componentIcons': lib('lib/componentIcons.tsx'),
  }),
}), 'ServiceList');
globalThis.window ??= globalThis;
// The real DNS engine card, mounted with the real decoder of its state. What
// stands in for the tracker is the lock it would draw: every view the card
// hands it is recorded, so the test can read the lock and press its action.
const lockURL = dataModule(`
  const lock = globalThis.dnsLock = { view: null, held: false };
  // One stable function, as the real provider gives: the card's own reads depend on its identity.
  const tracker = { acquireInteractionBlock: (view) => {
    lock.view = view; lock.held = true;
    return { update: (next) => { lock.view = next; }, release: () => { lock.held = false; lock.view = null; } };
  } };
  export const useComponentOperation = () => tracker;
`);
const RealDNSEngineCard = await pick(lib('components/DNSEngineCard.tsx', {
  '/lib/dnsEngineContract': lib('lib/dnsEngineContract.ts'),
  '/ComponentOperation': lockURL,
}), 'DNSEngineCard');

// --- What the server answers when nothing is wrong ----------------------------
const component = (id, config_files) => ({ id, name: id, description: `${id} on this server`, icon: '', category: 'dns', status: 'active (running)', is_installed: true, versions: [], config_files });
const profile = (id) => ({ id, name: id, description: `${id} for this server`, status: 'available', available: true, verified: false, latest_attempt_status: 'none', services: ['postfix'] });
// The mail ready bundles (alpha.94) the Panel sends beside the profiles.
const mailBundle = (id, profileId, extra = {}) => ({ id, category: 'email', name_code: id.replace('.', '_'), components: ['postfix'], status: 'available', action: 'install', action_route: 'mail_profile', profile_id: profileId, addons: [], missing_components: ['postfix'], ...extra });
const mailBundles = (extra = {}) => [mailBundle('mail.core', 'core-mail', extra), mailBundle('mail.webmail', 'webmail', extra), mailBundle('mail.protected', 'protected-mail', extra)];
const scan = (services) => ({ scanned_at: '2026-10-09T09:00:00Z', dns_identity_ready: true, mail_hostname: { current: 'server1', hostname: '', source: '', current_usable: true, will_set_hostname: false }, profiles: [profile('core-mail'), profile('webmail'), profile('protected-mail')], bundles: mailBundles(), services });
const good = {
  '/api/v1/fail2ban/jails': [{ name: 'sshd', enabled: true, active: true, banned: 1 }],
  '/api/v1/fail2ban/banned': [{ ip: '198.51.100.23', jail: 'sshd', time: '', country: '' }],
  '/api/v1/fail2ban/config': { ban_time: '10m', find_time: '10m', max_retry: 5, ignore_ip: ['127.0.0.1/8'] },
  '/api/v1/nginx/global': { worker_processes: 'auto', worker_connections: '768', keepalive_timeout: '65', client_max_body_size: '64m', server_tokens: 'off', gzip: 'on' },
  '/api/v1/nginx/ssl': { ssl_protocols: 'TLSv1.3', ssl_ciphers: 'ECDHE', ssl_prefer_server_ciphers: 'off' },
  '/api/v1/nginx/ratelimits': [{ name: 'login', zone: '$binary_remote_addr', size: '10m', rate: '5r/m' }],
  '/api/v1/php/extensions': [{ name: 'gd', enabled: true }, { name: 'intl', enabled: false }],
  '/api/v1/php/extended-config': { memory_limit: '256M', max_execution_time: '30' },
  '/api/v1/dovecot/stats': { uptime: '3 days', connections: 4, logins: 0, auth_success: 0, auth_fail: 0 },
  '/api/v1/managed-services': scan([component('pdns', [{ path: '/etc/powerdns/pdns.conf' }]), component('postfix', [])]),
  '/api/v1/settings/nameservers': { ns1: 'ns1.example.com', ns2: 'ns2.example.com', derived: false, server_ip: '203.0.113.10', facts: [], usable: true },
  '/api/v1/settings/dns-cluster': { configured: true, role: 'standalone', peer_ip: '', peer_ns: '', peer_reachable: false, server_ip: '203.0.113.10', ns1: 'ns1.example.com', ns2: 'ns2.example.com', facts: [], steps: [] },
  // The dashboard's other reads: a server on which nothing needs attention.
  '/api/v1/firewall': { enabled: true, engine_available: true, tcp_ports: [22, 80, 443], udp_ports: [], ssh_ports: [22], persistence_state: 'ready' },
  '/api/v1/dashboard': { databases: 2, mail_accounts: 3, expiring_certs: [] },
  '/api/v1/domains': [],
  '/api/v1/users': { users: [] },
  '/api/v1/audit-logs': { entries: [] },
  '/api/v1/host-mutation-readiness': { ready: true },
};
// What the same read answers when the server really has nothing.
const none = {
  '/api/v1/fail2ban/jails': [],
  '/api/v1/fail2ban/banned': null,
  '/api/v1/nginx/ratelimits': [],
  '/api/v1/php/extensions': [],
};

const originalFetch = globalThis.fetch;
let tree;
let withheld;
let requests;
const answer = (body, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
function serve(overrides = {}) {
  requests = [];
  globalThis.fetch = async (input, init = {}) => {
    const url = String(input);
    const path = url.split('?')[0];
    requests.push(`${init.method || 'GET'} ${path}`);
    const reply = path in overrides ? overrides[path] : good[path];
    if (typeof reply === 'function') return reply(init);
    if (reply === undefined) return answer({ error: 'no route' }, 404);
    return answer(reply);
  };
}
const HANG = () => new Promise((resolve) => { withheld.push(resolve); });
const DROP = () => { throw new TypeError('network'); };
const REFUSED = () => answer({ error: 'agent unavailable', code: 'INTERNAL' }, 502);
const NOT_THE_CONTRACT = () => answer('<html>', 200);

async function mount(element) {
  globalThis.currentTest = { toasts: [] };
  withheld = [];
  await act(async () => { tree = Renderer.create(element); });
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); });
}
async function settle() { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); }); }
async function unmount() {
  for (const resolve of withheld) resolve(answer({}, 500));
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.currentTest;
}
const text = () => {
  const walk = (node) => (typeof node === 'string' ? node : !node ? '' : Array.isArray(node) ? node.map(walk).join(' ') : walk(node.children));
  // Runs of white space are one space: a sentence drawn in pieces (KeepWords
  // keeps a hyphenated command whole) reads as the same sentence.
  return walk(tree.toJSON()).replace(/\s+/g, ' ');
};
const buttons = () => tree.root.findAll((node) => node.type === 'button');
const label = (node) => { const walk = (n) => (typeof n === 'string' ? n : (n.children || []).map(walk).join('')); return walk(node); };
const enabled = (names) => buttons().filter((node) => !node.props.disabled && names.some((name) => label(node).includes(name))).map(label);
const fields = () => tree.root.findAll((node) => ['input', 'textarea', 'select'].includes(node.type));
const press = async (name) => {
  const node = buttons().find((item) => label(item).includes(name));
  assert.ok(node, `no button ${name}`);
  await act(async () => { await node.props.onClick(); });
  await settle();
};
globalThis.confirm = () => true;

// --- The table ---------------------------------------------------------------------
// screen, the read that is withheld, the tab to open first, the sentences that
// claim something, the controls that change something, and the notice key.
const screens = [
  { name: 'Fail2ban, the jails', element: () => React.createElement(Fail2banManagement, { onBack() {} }), read: '/api/v1/fail2ban/jails', negative: ['f2b.emptyJails'], changes: [], checking: 'f2b.jails.checking', unknown: 'f2b.jails.unknown', empty: 'f2b.emptyJails' },
  { name: 'Fail2ban, the banned addresses', element: () => React.createElement(Fail2banManagement, { onBack() {} }), tab: 'f2b.tab.banned', read: '/api/v1/fail2ban/banned', negative: ['f2b.emptyBanned'], changes: ['f2b.unban'], checking: 'f2b.banned.checking', unknown: 'f2b.banned.unknown', empty: 'f2b.emptyBanned' },
  { name: 'Fail2ban, the settings', element: () => React.createElement(Fail2banManagement, { onBack() {} }), tab: 'f2b.tab.config', read: '/api/v1/fail2ban/config', negative: ['f2b.configReadonly'], changes: [], checking: 'f2b.config.checking', unknown: 'f2b.config.unknown' },
  { name: 'Nginx, the global settings', element: () => React.createElement(NginxManagement, { onBack() {} }), read: '/api/v1/nginx/global', negative: ['—', 'nginx.readonly'], changes: [], checking: 'nginx.global.checking', unknown: 'nginx.global.unknown' },
  { name: 'Nginx, the TLS settings', element: () => React.createElement(NginxManagement, { onBack() {} }), tab: 'nginx.tab.ssl', read: '/api/v1/nginx/ssl', negative: ['—', 'nginx.readonly'], changes: [], checking: 'nginx.ssl.checking', unknown: 'nginx.ssl.unknown' },
  { name: 'Nginx, the rate limits', element: () => React.createElement(NginxManagement, { onBack() {} }), tab: 'nginx.tab.rateLimits', read: '/api/v1/nginx/ratelimits', negative: ['nginx.emptyRateLimits'], changes: [], checking: 'nginx.rate.checking', unknown: 'nginx.rate.unknown', empty: 'nginx.emptyRateLimits' },
  { name: 'PHP, the extensions', element: () => React.createElement(PHPManagement, { versions: ['8.3'], onBack() {} }), read: '/api/v1/php/extensions', negative: ['php.emptyExtensions'], changes: [], switches: true, checking: 'php.extensions.checking', unknown: 'php.extensions.unknown', empty: 'php.emptyExtensions' },
  { name: 'PHP, php.ini', element: () => React.createElement(PHPExtendedConfig, { version: '8.3' }), read: '/api/v1/php/extended-config', negative: [], changes: ['Save', 'Apply'], formFields: true, checking: 'php.ini.checking', unknown: 'php.ini.unknown' },
  { name: 'Dovecot, the figures', element: () => React.createElement(DovecotManagement, { onBack() {} }), read: '/api/v1/dovecot/stats', negative: ['—'], changes: [], checking: null, unknown: 'dovecot.statsUnknown', pending: '…' },
  { name: 'PowerDNS, which files it has', element: () => React.createElement(PowerDNSManagement, { onBack() {} }), read: '/api/v1/managed-services', negative: [], changes: [], checking: 'dbconf.files.checking', unknown: 'dbconf.files.unknown' },
  { name: 'DNS settings, the saved names', element: () => React.createElement(DNSServerSettings, {}), read: '/api/v1/settings/nameservers', negative: ['dnssrv.blocker'], changes: ['dnssrv.saveAndPublish'], formFields: true, checking: 'dnssrv.checking', unknown: 'dnssrv.unknown' },
  { name: 'DNS settings, the saved role', element: () => React.createElement(DNSServerSettings, {}), read: '/api/v1/settings/dns-cluster', negative: ['dnssrv.blocker'], changes: ['dnssrv.saveAndPublish'], formFields: true, checking: 'dnssrv.checking', unknown: 'dnssrv.unknown' },
];
const openTab = async (screen) => { if (screen.tab) await press(screen.tab); };

const withholdings = [
  ['still on its way', HANG, 'checking'],
  ['dropped', DROP, 'unknown'],
  ['refused', REFUSED, 'unknown'],
  ['not the contract', NOT_THE_CONTRACT, 'unknown'],
];

for (const screen of screens) {
  for (const [how, reply, state] of withholdings) {
    test(`${screen.name}: ${how} → nothing negative, nothing that changes it enabled`, async () => {
      serve({ [screen.read]: reply });
      await mount(screen.element());
      try {
        await openTab(screen);
        const shown = text();
        for (const phrase of screen.negative) assert.ok(!shown.includes(phrase), `"${phrase}" is on screen while the read is ${how}: ${shown.slice(0, 300)}`);
        assert.deepEqual(enabled(screen.changes), [], 'a control that changes something is enabled');
        if (screen.switches) assert.equal(fields().filter((node) => node.props.type === 'checkbox').length, 0, 'a switch exists for an extension the server did not list');
        if (screen.formFields) assert.equal(fields().length, 0, 'a form was built from nothing');
        if (state === 'checking') {
          if (screen.checking) assert.ok(shown.includes(screen.checking), `the checking line is missing: ${shown.slice(0, 300)}`);
          if (screen.pending) assert.ok(shown.includes(screen.pending), 'a figure that is being read is "…"');
          assert.ok(!shown.includes(screen.unknown), 'a read that has not answered is not reported as failed');
        } else {
          assert.ok(shown.includes(screen.unknown), `the could-not-check sentence is missing: ${shown.slice(0, 300)}`);
          assert.ok(enabled(['common.retry']).length >= 1, 'Retry is not offered');
          // Retry reads, and only reads.
          const before = requests.length;
          serve();
          requests = [];
          await press('common.retry');
          assert.ok(requests.length > 0 && requests.every((line) => line.startsWith('GET ')), `Retry sent something other than a read: ${requests.join(', ')} (${before})`);
          assert.ok(!text().includes(screen.unknown), 'the notice stays after a successful read');
        }
      } finally { await unmount(); }
    });
  }
  if (screen.empty) {
    test(`${screen.name}: a known empty answer → the empty state, and only then`, async () => {
      serve({ [screen.read]: none[screen.read] });
      await mount(screen.element());
      try {
        await openTab(screen);
        assert.ok(text().includes(screen.empty), text().slice(0, 300));
      } finally { await unmount(); }
    });
  }
}

test('the counts beside the Fail2ban tabs: "…" while read, "–" when not read, the number when known', async () => {
  serve({ '/api/v1/fail2ban/jails': HANG, '/api/v1/fail2ban/banned': REFUSED });
  await mount(React.createElement(Fail2banManagement, { onBack() {} }));
  try {
    const tabs = () => buttons().map(label);
    assert.ok(tabs().some((item) => item === 'f2b.tab.jails…'), tabs().join(' | '));
    assert.ok(tabs().some((item) => item === 'f2b.tab.banned–'), tabs().join(' | '));
    assert.ok(!tabs().some((item) => /f2b\.tab\.(jails|banned)0$/.test(item)), 'a count of 0 from no answer');
  } finally { await unmount(); }
  serve();
  await mount(React.createElement(Fail2banManagement, { onBack() {} }));
  try {
    assert.ok(buttons().map(label).includes('f2b.tab.jails1'));
  } finally { await unmount(); }
});

test('a ban lifted, then the list could not be read again: the earlier list stays, marked, with Unban off', async () => {
  let reads = 0;
  serve({ '/api/v1/fail2ban/banned': (init) => {
    if (init.method === 'POST') return answer({ success: true });
    reads += 1;
    return reads === 1 ? answer(good['/api/v1/fail2ban/banned']) : REFUSED();
  } });
  await mount(React.createElement(Fail2banManagement, { onBack() {} }));
  try {
    await press('f2b.tab.banned');
    assert.deepEqual(enabled(['f2b.unban']), ['f2b.unban']);
    await press('f2b.unban');
    assert.equal(requests.filter((line) => line === 'POST /api/v1/fail2ban/banned').length, 1, 'the change is sent once');
    const shown = text();
    assert.ok(shown.includes('common.staleNotice'), shown.slice(0, 300));
    assert.ok(shown.includes('198.51.100.23'), 'the earlier answer stays on screen');
    assert.ok(!shown.includes('f2b.emptyBanned'));
    assert.deepEqual(enabled(['f2b.unban']), [], 'Unban is offered on a list that is not known');
  } finally { await unmount(); }
});

test('a PHP extension switch is sent once and then shows what the server says, accepted or refused', async () => {
  let posted = 0;
  let list = [{ name: 'gd', enabled: true }, { name: 'intl', enabled: false }];
  serve({ '/api/v1/php/extensions': async (init) => {
    if (init.method === 'POST') { posted += 1; return answer({ error: 'refused' }, 502); }
    return answer(list);
  } });
  await mount(React.createElement(PHPManagement, { versions: ['8.3'], onBack() {} }));
  try {
    const switches = () => fields().filter((node) => node.props.type === 'checkbox');
    assert.deepEqual(switches().map((node) => node.props.checked), [true, false]);
    await act(async () => { await switches()[1].props.onChange(); });
    await settle();
    assert.equal(posted, 1);
    assert.deepEqual(globalThis.currentTest.toasts, [['error', 'php.toggleFailed']]);
    assert.deepEqual(switches().map((node) => node.props.checked), [true, false], 'a refused switch must not stay on');
    assert.ok(switches().every((node) => !node.props.disabled));
  } finally { await unmount(); }
});

test('refreshRemote: whoever shows an address reads it again once; with no reader nothing is requested', async () => {
  const decode = (raw) => raw;
  let served = 0;
  globalThis.fetch = async () => { served += 1; return answer({ n: served }); };
  const seen = [];
  const Reader = () => { const { remote } = useRemote('/api/v1/refresh-probe', decode); seen.push(remote.state === 'known' ? remote.value.n : remote.state); return null; };
  // Nobody shows the address: nothing to refresh.
  refreshRemote('/api/v1/refresh-probe');
  assert.equal(served, 0);
  await act(async () => { tree = Renderer.create(React.createElement(React.Fragment, null, React.createElement(Reader), React.createElement(Reader))); });
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); });
  assert.equal(served, 1, 'two readers of one address share one request');
  refreshRemote('/api/v1/refresh-probe');
  // The value on screen stays until the new answer arrives.
  assert.equal(seen[seen.length - 1], 1);
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 15)); });
  assert.equal(served, 2, 'one request for the refresh, whatever the number of readers');
  assert.equal(seen[seen.length - 1], 2);
  await act(async () => tree.unmount());
  tree = undefined;
  refreshRemote('/api/v1/refresh-probe');
  assert.equal(served, 2, 'the last reader left: nothing is requested');
  globalThis.fetch = originalFetch;
});

test('the screens of the third batch read through the shared layer and are off the list', () => {
  const webDir = fileURLToPath(new URL('../', import.meta.url));
  const actual = scanTree(webDir);
  const allowList = readAllowList();
  for (const name of screenFiles) {
    const path = `src/components/${name}.tsx`;
    assert.equal(actual[path], undefined, `${path} reads the old way again: ${JSON.stringify(actual[path])}`);
    assert.equal(allowList.files[path], undefined, `${path} is back on the allow-list`);
  }
});

// --- The dashboard's attention list and the firewall's state ----------------------
// The list is built from four reads. With any one of them withheld it must not
// say the firewall is off, must not offer to turn it on, and must not say that
// nothing needs action. Before this batch any answer of GET /api/v1/firewall
// was stored as the firewall's state, so an Agent error sent with a 200 drew
// "Firewall is off" with its button.
const freshScan = () => ({ ...good['/api/v1/managed-services'], scanned_at: new Date().toISOString() });
const dashboard = () => React.createElement(Dashboard);
const statusLabels = () => tree.root.findAll((node) => node.props.role === 'status' && typeof node.type === 'string').map((node) => node.props['aria-label']);
const CLAIMS = ['dashboard.fwOffItem', 'dashboard.fwPersistenceItem', 'dashboard.attentionNone', 'dashboard.attentionNoneUnchecked', 'dashboard.allGood'];
const AGENT_ERROR_AS_200 = () => answer({ error: 'agent unavailable' });
const attentionReads = ['/api/v1/firewall', '/api/v1/dashboard', '/api/v1/domains', '/api/v1/managed-services'];

for (const read of attentionReads) {
  for (const [how, reply, state] of [...withholdings, ...(read === '/api/v1/firewall' ? [['an Agent error sent with a 200', AGENT_ERROR_AS_200, 'unknown']] : [])]) {
    test(`Dashboard, the attention list with ${read} ${how} → no claim about the server, nothing to turn on`, async () => {
      serve({ '/api/v1/managed-services': freshScan(), [read]: reply });
      await mount(dashboard());
      try {
        const shown = text();
        for (const phrase of CLAIMS) assert.ok(!shown.includes(phrase), `"${phrase}" is on screen while ${read} is ${how}: ${shown.slice(0, 400)}`);
        assert.deepEqual(enabled(['firewall.turnOn', 'dashboard.saveFirewall']), [], 'a firewall change is offered without the firewall being known');
        assert.equal(requests.filter((line) => !line.startsWith('GET ')).length, 0, `the page sent a change by itself: ${requests.join(', ')}`);
        if (state === 'checking') {
          assert.ok(shown.includes('dashboard.attentionChecking'), `the checking line is missing: ${shown.slice(0, 400)}`);
          assert.ok(!shown.includes('dashboard.attentionUnread'), 'a read that has not answered is not reported as failed');
        } else {
          assert.ok(shown.includes('dashboard.attentionUnread'), `the could-not-check sentence is missing: ${shown.slice(0, 400)}`);
          serve({ '/api/v1/managed-services': freshScan() });
          requests = [];
          await press('common.retry');
          // Retry reads, and reads only what could not be read.
          assert.deepEqual([...new Set(requests)], [`GET ${read}`], `Retry sent ${requests.join(', ')}`);
          assert.ok(!text().includes('dashboard.attentionUnread'), 'the notice stays after a successful read');
          assert.ok(text().includes('dashboard.attentionNone'), text().slice(0, 400));
        }
      } finally { await unmount(); }
    });
  }
}

test('Dashboard: "Firewall is off" and "Turn on" exist for an answer with enabled: false, and only then', async () => {
  serve({ '/api/v1/managed-services': freshScan(), '/api/v1/firewall': { ...good['/api/v1/firewall'], enabled: false, persistence_state: 'disabled' } });
  await mount(dashboard());
  try {
    assert.ok(text().includes('dashboard.fwOffItem'), text().slice(0, 400));
    assert.deepEqual(enabled(['firewall.turnOn']), ['firewall.turnOn']);
    assert.ok(!text().includes('dashboard.attentionNone'));
    assert.equal(requests.filter((line) => !line.startsWith('GET ')).length, 0, 'drawing the item sent nothing');
  } finally { await unmount(); }
});

test('Dashboard: the calm line names what was read, and does not speak for components nobody checked lately', async () => {
  serve({ '/api/v1/managed-services': freshScan() });
  await mount(dashboard());
  try {
    assert.ok(text().includes('dashboard.attentionNone') && !text().includes('dashboard.attentionNoneUnchecked'), text().slice(0, 400));
    assert.ok(!statusLabels().includes('dashboard.attentionChecking'), 'the checking line stays after every read answered');
  } finally { await unmount(); }
  // The stored scan of the fixture is from 9 Oct 2026, 09:00 UTC: not current.
  // Since 12 Oct 2026 the page checks by itself; this fixture has no scan
  // route, so the check fails and the line says the components could not be
  // checked rather than speaking for them.
  serve();
  await mount(dashboard());
  try {
    await settle();
    assert.ok(text().includes('dashboard.attentionNoneCheckFailed'), text().slice(0, 400));
    assert.ok(!text().includes('dashboard.attentionNone '), 'the calm line spoke for components whose check failed');
  } finally { await unmount(); }
});

// --- The dashboard checks the components by itself (owner decision, 12 Oct 2026) --
// One conditional scan when the stored one is missing or older than the
// window, none for a fresh one, none while the page is hidden, never two at
// once, and nothing asked again after a refusal. The card's states are
// distinct: checking, the result, could not check (with the reason and the
// check in place), and a change in progress.
// Pano bileşenleri kendisi kontrol eder: saklı tarama yoksa ya da pencereden
// eskiyse tek koşullu tarama; taze ise, sayfa gizliyse yok; aynı anda iki tane
// asla; bir retten sonra yeniden sorma yok.
const SCAN_PATH = '/api/v1/managed-services/scan';
const scans = () => requests.filter((line) => line === `POST ${SCAN_PATH}`).length;
let scanURLs = [];
function recordScanURLs() {
  scanURLs = [];
  const inner = globalThis.fetch;
  globalThis.fetch = (input, init = {}) => {
    if (String(input).startsWith(SCAN_PATH)) scanURLs.push(String(input));
    return inner(input, init);
  };
}
function pageVisibility(state) {
  const listeners = new Set();
  globalThis.document = {
    visibilityState: state,
    addEventListener: (name, listener) => { if (name === 'visibilitychange') listeners.add(listener); },
    removeEventListener: (name, listener) => { if (name === 'visibilitychange') listeners.delete(listener); },
  };
  return {
    async show() {
      globalThis.document.visibilityState = 'visible';
      await act(async () => { for (const listener of listeners) listener(); });
      await settle();
    },
    listeners,
  };
}

test('Dashboard: a fresh stored scan → no check is asked for', async () => {
  serve({ '/api/v1/managed-services': freshScan() });
  await mount(dashboard());
  try {
    await settle();
    assert.equal(scans(), 0, `a fresh scan was checked again: ${requests.join(', ')}`);
    assert.ok(!text().includes('dashboard.systemServicesChecking'));
  } finally { await unmount(); }
});

test('Dashboard: a stale stored scan → one conditional check, its result drawn on the card', async () => {
  serve({ [SCAN_PATH]: (init) => (init.method === 'POST' ? answer(freshScan()) : answer({}, 405)) });
  recordScanURLs();
  await mount(dashboard());
  try {
    await settle();
    await settle();
    assert.equal(scans(), 1, `expected one check: ${requests.join(', ')}`);
    assert.deepEqual(scanURLs, [`${SCAN_PATH}?max_age_seconds=300`], 'the page asked for an unconditional scan');
    const shown = text();
    assert.ok(shown.includes('0 / 0'), `the result is not drawn: ${shown.slice(0, 400)}`);
    assert.ok(shown.includes('dashboard.attentionNone') && !shown.includes('dashboard.attentionNoneUnchecked'), shown.slice(0, 400));
    for (const phrase of ['dashboard.systemServicesChecking', 'dashboard.statusUnknown', 'dashboard.systemServicesCheckFailed']) {
      assert.ok(!shown.includes(phrase), `"${phrase}" is still on the card`);
    }
    assert.deepEqual(globalThis.currentTest.toasts, [], 'a check nobody pressed raised a toast');
  } finally { await unmount(); }
});

test('Dashboard: while the check runs the card says it is checking, not "Unknown", and asks once', async () => {
  serve({ [SCAN_PATH]: HANG });
  await mount(dashboard());
  try {
    await settle();
    await settle();
    const shown = text();
    assert.ok(statusLabels().includes('dashboard.systemServicesChecking'), `no checking line: ${shown.slice(0, 400)}`);
    assert.ok(!shown.includes('dashboard.statusUnknown'), 'the card says Unknown while it checks');
    assert.ok(shown.includes('dashboard.attentionNoneChecking'), shown.slice(0, 400));
    assert.equal(scans(), 1, `more than one check is in flight: ${requests.join(', ')}`);
  } finally { await unmount(); }
});

for (const code of ['service_operation_busy', 'server_setup_busy']) {
  test(`Dashboard: the server refuses the check with 409 ${code} → a change is in progress, nothing asked again`, async () => {
    serve({ [SCAN_PATH]: () => answer({ error: 'busy', code }, 409) });
    await mount(dashboard());
    try {
      for (let i = 0; i < 4; i += 1) await settle();
      const shown = text();
      assert.ok(shown.includes('dashboard.systemServicesCheckHeld'), shown.slice(0, 400));
      assert.ok(shown.includes('dashboard.attentionNoneCheckHeld'), shown.slice(0, 400));
      assert.ok(!shown.includes('dashboard.systemServicesCheckFailed'), 'a refusal is drawn as a failure');
      assert.equal(scans(), 1, `the refusal was asked again: ${requests.join(', ')}`);
      assert.deepEqual(enabled(['services.scanNow']), [], 'a check that would only be refused is offered');
      assert.deepEqual(globalThis.currentTest.toasts, []);
    } finally { await unmount(); }
  });
}

test('Dashboard: the check fails → "could not check" with the reason and the check in place, not asked again by itself', async () => {
  serve({ [SCAN_PATH]: () => answer({ error: 'agent unavailable' }, 502) });
  recordScanURLs();
  await mount(dashboard());
  try {
    for (let i = 0; i < 4; i += 1) await settle();
    const shown = text();
    assert.ok(shown.includes('dashboard.systemServicesCheckFailed') && shown.includes('agent unavailable'), shown.slice(0, 400));
    assert.ok(shown.includes('dashboard.attentionNoneCheckFailed'), shown.slice(0, 400));
    assert.equal(scans(), 1, `the failure was asked again by itself: ${requests.join(', ')}`);
    assert.deepEqual(globalThis.currentTest.toasts, [], 'a check nobody pressed raised a toast');
    // The operator's own check: unconditional, from the card.
    serve({ [SCAN_PATH]: () => answer(freshScan()) });
    recordScanURLs();
    await press('services.scanNow');
    assert.deepEqual(scanURLs, [SCAN_PATH], 'the operator check must be the unconditional scan');
    assert.ok(text().includes('0 / 0'), text().slice(0, 400));
  } finally { await unmount(); }
});

test('Dashboard: a hidden page checks nothing; it checks once when it is shown', async () => {
  const page = pageVisibility('hidden');
  serve({ [SCAN_PATH]: () => answer(freshScan()) });
  recordScanURLs();
  await mount(dashboard());
  try {
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(scans(), 0, `a hidden page checked: ${requests.join(', ')}`);
    assert.ok(text().includes('dashboard.systemServicesNotRecent'), text().slice(0, 400));
    assert.equal(page.listeners.size, 1, 'the page does not listen for being shown');
    await page.show();
    await settle();
    assert.equal(scans(), 1, `expected one check once shown: ${requests.join(', ')}`);
    assert.deepEqual(scanURLs, [`${SCAN_PATH}?max_age_seconds=300`]);
    assert.ok(text().includes('0 / 0'), text().slice(0, 400));
  } finally {
    await unmount();
    assert.equal(page.listeners.size, 0, 'the visibility listener outlived the page');
    delete globalThis.document;
  }
});

// A server clock a second or two ahead of the browser (measured 12 Oct 2026 on
// Debian 13 and Ubuntu 24.04) must not make a fresh scan read "not checked in
// the last 5 minutes": the scan counts from when the page received it.
// Sunucu saati tarayıcıdan bir iki saniye ileride olsa da taze tarama "son 5
// dakikada kontrol edilmedi" okunmaz; tarama, sayfanın aldığı andan sayılır.
const scanStampedAt = (offsetMs) => ({ ...good['/api/v1/managed-services'], scanned_at: new Date(Date.now() + offsetMs).toISOString() });
const NOT_FRESH_PHRASES = ['dashboard.systemServicesNotRecent', 'dashboard.statusUnknown', 'dashboard.systemServicesChecking', 'dashboard.systemServicesCheckFailed'];

test('Dashboard: a stored scan stamped 2 s ahead of the browser is shown as the result at once, with no check', async () => {
  serve({ '/api/v1/managed-services': scanStampedAt(2000) });
  await mount(dashboard());
  try {
    await settle();
    const shown = text();
    assert.ok(shown.includes('0 / 0'), shown.slice(0, 400));
    for (const phrase of NOT_FRESH_PHRASES) assert.ok(!shown.includes(phrase), `"${phrase}" is drawn for a fresh scan`);
    assert.equal(scans(), 0, `a fresh scan was checked again: ${requests.join(', ')}`);
  } finally { await unmount(); }
});

test('Dashboard: a check answered with a stamp 2 s ahead is the result in the very next frame, and is not asked again', async () => {
  serve({ [SCAN_PATH]: (init) => (init.method === 'POST' ? answer(scanStampedAt(2000)) : answer({}, 405)) });
  await mount(dashboard());
  try {
    await settle();
    await settle();
    const shown = text();
    assert.ok(shown.includes('0 / 0'), shown.slice(0, 400));
    for (const phrase of NOT_FRESH_PHRASES) assert.ok(!shown.includes(phrase), `"${phrase}" is drawn after a successful check`);
    for (let i = 0; i < 3; i += 1) await settle();
    assert.equal(scans(), 1, `a second automatic check was started: ${requests.join(', ')}`);
    assert.ok(text().includes('0 / 0'));
  } finally { await unmount(); }
});

test('Dashboard: a scan 6 minutes old is not current and one 4 minutes old is', async () => {
  const page = pageVisibility('hidden');
  try {
    for (const [offset, current] of [[-6 * 60_000, false], [-4 * 60_000, true]]) {
      serve({ '/api/v1/managed-services': scanStampedAt(offset) });
      await mount(dashboard());
      try {
        for (let i = 0; i < 3; i += 1) await settle();
        const shown = text();
        assert.equal(shown.includes('dashboard.systemServicesNotRecent'), !current, shown.slice(0, 400));
        assert.equal(shown.includes('0 / 0'), current, shown.slice(0, 400));
        assert.equal(scans(), 0, 'a hidden page checked');
      } finally { await unmount(); }
    }
  } finally { delete globalThis.document; }
});

// The server reports the scan's age on its own clock (scan_age_seconds); the
// page subtracts it from when it received the answer. A server clock seven
// minutes behind the browser's then changes nothing: the age decides, not the
// stamp. Without the field the rule above (the earlier of stamp and receipt)
// still holds, which the tests above pin.
// Sunucu taramanın yaşını kendi saatiyle bildirir; sayfa onu yanıtı aldığı
// andan çıkarır. Tarayıcıdan yedi dakika geri bir sunucu saati hiçbir şeyi
// değiştirmez: damga değil yaş karar verir.
const scanAged = (ageSeconds, stampOffsetMs = -7 * 60_000) => ({
  ...scanStampedAt(stampOffsetMs),
  scan_age_seconds: ageSeconds,
});

test('Dashboard: a scan 10 s old by the server, stamped 7 min behind the browser, is the result at once with no check', async () => {
  serve({ '/api/v1/managed-services': scanAged(10) });
  await mount(dashboard());
  try {
    for (let i = 0; i < 3; i += 1) await settle();
    const shown = text();
    assert.ok(shown.includes('0 / 0'), shown.slice(0, 400));
    for (const phrase of NOT_FRESH_PHRASES) assert.ok(!shown.includes(phrase), `"${phrase}" is drawn for a scan 10 s old`);
    assert.equal(scans(), 0, `a fresh scan was checked again: ${requests.join(', ')}`);
  } finally { await unmount(); }
});

test('Dashboard: a scan 400 s old by the server is not current, and the page asks exactly one conditional check', async () => {
  // Hidden: the card says it is not recent and nothing is asked.
  pageVisibility('hidden');
  try {
    serve({ '/api/v1/managed-services': scanAged(400, 0) });
    await mount(dashboard());
    try {
      for (let i = 0; i < 3; i += 1) await settle();
      const shown = text();
      assert.ok(shown.includes('dashboard.systemServicesNotRecent'), shown.slice(0, 400));
      assert.ok(!shown.includes('0 / 0'), shown.slice(0, 400));
      assert.equal(scans(), 0, 'a hidden page checked');
    } finally { await unmount(); }
  } finally { delete globalThis.document; }

  // Visible: one conditional check; its answer (0 s old by the server, its
  // stamp 7 min behind the browser) is the result and is not asked again.
  serve({
    '/api/v1/managed-services': scanAged(400, 0),
    [SCAN_PATH]: (init) => (init.method === 'POST' ? answer(scanAged(0)) : answer({}, 405)),
  });
  recordScanURLs();
  await mount(dashboard());
  try {
    for (let i = 0; i < 5; i += 1) await settle();
    assert.equal(scans(), 1, `expected exactly one check: ${requests.join(', ')}`);
    assert.deepEqual(scanURLs, [`${SCAN_PATH}?max_age_seconds=300`], 'the page asked for an unconditional scan');
    const shown = text();
    assert.ok(shown.includes('0 / 0'), shown.slice(0, 400));
    for (const phrase of NOT_FRESH_PHRASES) assert.ok(!shown.includes(phrase), `"${phrase}" is drawn after the check`);
  } finally { await unmount(); }
});

test('Dashboard: a payload with a malformed scan age is not trusted', async () => {
  serve({ '/api/v1/managed-services': { ...scanStampedAt(0), scan_age_seconds: '10' } });
  await mount(dashboard());
  try {
    for (let i = 0; i < 3; i += 1) await settle();
    assert.ok(!text().includes('0 / 0'), text().slice(0, 400));
  } finally { await unmount(); }
});

// set14 O1: an Agent that cannot be reached is named on the card, not called
// an internal error. (This harness's translator returns keys, so the reason
// drawn is the coded answer's own sentence; the EN and TR sentences for
// err.AGENT_UNREACHABLE are pinned in dashboard-truth-contract.)
test('Dashboard: the check answers AGENT_UNREACHABLE → "could not check" names the Agent, asked once', async () => {
  const sentence = 'The CelikPanel Agent on this server is not reachable';
  serve({ [SCAN_PATH]: () => answer({ error: sentence, code: 'AGENT_UNREACHABLE' }, 503) });
  await mount(dashboard());
  try {
    for (let i = 0; i < 4; i += 1) await settle();
    const shown = text();
    assert.ok(shown.includes('dashboard.systemServicesCheckFailed') && shown.includes(sentence), shown.slice(0, 400));
    assert.equal(scans(), 1, `the failure was asked again by itself: ${requests.join(', ')}`);
  } finally { await unmount(); }
});

test('Dashboard: an item is already listed while another read is on its way → no row that comes and goes', async () => {
  serve({ '/api/v1/managed-services': freshScan(), '/api/v1/firewall': { ...good['/api/v1/firewall'], enabled: false, persistence_state: 'disabled' }, '/api/v1/dashboard': HANG });
  await mount(dashboard());
  try {
    const shown = text();
    assert.ok(shown.includes('dashboard.fwOffItem'), 'a known item waits for an unrelated read');
    // Said beside the count (the spinner's label), not as a line in the list.
    assert.ok(statusLabels().includes('dashboard.attentionChecking'), 'nothing says another read is on its way');
    assert.ok(!shown.includes('dashboard.attentionChecking'), 'the checking line is drawn as a row above known items; it leaves when the read answers and everything under it moves');
    assert.ok(!shown.includes('dashboard.attentionNone'));
  } finally { await unmount(); }
});

test('the firewall decoder: "off" is a boolean the Agent sent, never the lack of one', async () => {
  for (const raw of [null, [], '<html>', {}, { enabled: 'false' }, { enabled: 0 }, { error: 'agent unavailable' }, { enabled: false, error: 'iptables: permission denied' }]) {
    assert.throws(() => decodeFirewallStatus(raw), `${JSON.stringify(raw)} was read as a firewall state`);
  }
  assert.equal(decodeFirewallStatus({ enabled: false }).enabled, false);
  assert.equal(decodeFirewallStatus({ enabled: true, error: '' }).enabled, true);
  // A signed-in user who may not read the firewall: nothing is requested.
  serve();
  const Reader = () => { useFirewallStatus({ enabled: false }); return null; };
  await mount(React.createElement(Reader));
  try {
    assert.deepEqual(requests, []);
  } finally { await unmount(); }
});

// --- The DNS engine card: polling only reads --------------------------------------
// A change the server accepted has recorded nothing for minutes. Until 9 Oct 2026
// the card's own timer then sent POST /api/v1/dns/engine/reconcile (its first
// tick did, half a second after the state was read). That request takes the
// server's change lock and is audited under the signed-in administrator, so a
// timer sends it zero times now; the person at the screen may send it.
const REQUEST = 'b'.repeat(32);
const OPERATION = 'a'.repeat(32);
const ago = (minutes) => new Date(Date.now() - minutes * 60_000).toISOString();
const dnsEngines = (active) => ['pdns', 'bind'].map((id) => ({ id, installed: true, running: id === active, managed: true, status: id === active ? 'active' : 'installed_standby' }));
const dnsChange = (started, updated) => ({
  revision: 5, topology: 'standalone', dnssec_zone_count: 0, zone_count: 2, pending_zone_count: 0, engine_epoch: 1, active_engine: 'bind', state: 'switching', engines: dnsEngines('bind'), operation_id: OPERATION,
  operation: { id: OPERATION, request_id: REQUEST, target_engine: 'pdns', phase: 'activating', status: 'running', started_at: ago(started), updated_at: ago(updated) },
});
const dnsDone = () => ({
  revision: 6, topology: 'standalone', dnssec_zone_count: 0, zone_count: 2, pending_zone_count: 0, engine_epoch: 2, active_engine: 'pdns', state: 'ready', engines: dnsEngines('pdns'),
  operation: { id: OPERATION, request_id: REQUEST, target_engine: 'pdns', phase: 'committed', status: 'succeeded', started_at: ago(5), updated_at: ago(0) },
});
const RECONCILE = 'POST /api/v1/dns/engine/reconcile';
const sent = (line) => requests.filter((item) => item === line).length;
const changes = () => requests.filter((line) => !line.startsWith('GET '));
// The card's first poll runs half a second after it adopts the change.
const afterFirstPoll = async () => { await act(async () => { await new Promise((resolve) => setTimeout(resolve, 800)); }); };
// What the server does with a reconcile request: nothing new, a refusal, or it closes the change.
function serveDNS(snapshot, reconcile) {
  const state = { snapshot };
  serve({
    '/api/v1/dns/engine': () => answer(state.snapshot),
    '/api/v1/dns/engine/reconcile': () => {
      if (reconcile === 'refused') return answer({ error: 'DNS engine state could not be verified.', code: 'DNS_ENGINE_STATE_UNVERIFIED' }, 409);
      if (reconcile === 'finishes') state.snapshot = dnsDone();
      return answer({ reconciled: reconcile === 'finishes' });
    },
  });
  return state;
}

test('DNS engine card, a change stalled for four minutes: the poll sends nothing; "Check now" in the lock sends one request', async () => {
  const server = serveDNS(dnsChange(5, 4), 'unchanged');
  await mount(React.createElement(RealDNSEngineCard));
  try {
    await afterFirstPoll();
    const lock = globalThis.dnsLock;
    assert.ok(lock.held, 'the change is not locked');
    assert.deepEqual(changes(), [], `the page sent a change by itself: ${requests.join(', ')}`);
    assert.ok(sent('GET /api/v1/dns/engine') >= 2, 'the poll did not read');
    assert.ok(lock.view.message.startsWith('dnsEngine.guard.reconcileDue') && lock.view.message.includes('dnsEngine.guard.reconcileOffer'), lock.view.message);
    assert.ok(text().includes('dnsEngine.operation.autoTracking'), 'a change that is still polled says so on the card');
    assert.equal(lock.view.action?.label, 'dnsEngine.guard.checkNow');
    assert.equal(lock.view.busy, false, 'a stalled change still shows a working lock');

    // Pressed twice before the answer: one request.
    await act(async () => { lock.view.action.onAct(); lock.view.action.onAct(); });
    await settle();
    assert.equal(sent(RECONCILE), 1, 'the check is not single-flight');
    assert.deepEqual(changes(), [RECONCILE], 'the check sent something other than the reconcile request');
    assert.ok(lock.held, 'a check that changed nothing released the lock');
    assert.ok(lock.view.message.startsWith('dnsEngine.guard.reconcileChecked'), lock.view.message);
    assert.equal(lock.view.action?.busy, false);

    // The server closes the change on the next check: the lock is released.
    serveDNS(dnsChange(5, 4), 'finishes');
    await act(async () => { lock.view.action.onAct(); });
    await settle();
    assert.equal(sent(RECONCILE), 1);
    assert.equal(lock.held, false, 'the finished change is still locked');
    assert.deepEqual(globalThis.currentTest.toasts, [['success', 'dnsEngine.switchCompleted']]);
    void server;
  } finally { await unmount(); }
});

test('DNS engine card, a check the server refuses: its refusal is said in the lock and the offer stays', async () => {
  serveDNS(dnsChange(5, 4), 'refused');
  await mount(React.createElement(RealDNSEngineCard));
  try {
    await afterFirstPoll();
    const lock = globalThis.dnsLock;
    await act(async () => { lock.view.action.onAct(); });
    await settle();
    assert.equal(sent(RECONCILE), 1);
    assert.ok(lock.held);
    assert.ok(lock.view.message.includes('dnsEngine.guard.reconcileOffer') && !lock.view.message.startsWith('dnsEngine.guard.reconcileChecked'), lock.view.message);
    assert.equal(lock.view.action?.label, 'dnsEngine.guard.checkNow');
  } finally { await unmount(); }
});

test('DNS engine card, an operation the server has not published: nothing is offered', async () => {
  // Another request's operation is in progress; this page holds none of its own.
  serveDNS({ ...dnsChange(5, 4), state: 'ready', operation: undefined, operation_id: undefined }, 'unchanged');
  await mount(React.createElement(RealDNSEngineCard));
  try {
    await afterFirstPoll();
    assert.equal(globalThis.dnsLock.held, false);
    assert.deepEqual(changes(), []);
    assert.equal(tree.root.findAll((node) => node.props['data-testid'] === 'dns-engine-deadline-check').length, 0);
  } finally { await unmount(); }
});

test('DNS engine card, past the safety limit: the lock is released, nothing is sent, and the card still offers the one check', async () => {
  serveDNS(dnsChange(40, 39), 'unchanged');
  await mount(React.createElement(RealDNSEngineCard));
  const notice = () => tree.root.findAll((node) => node.props['data-testid'] === 'dns-engine-deadline-check');
  const noticeText = () => label(notice()[0]);
  try {
    await afterFirstPoll();
    assert.equal(globalThis.dnsLock.held, false, 'past the safety limit the lock must not trap the page');
    assert.deepEqual(changes(), [], `the page sent a change by itself: ${requests.join(', ')}`);
    assert.equal(notice().length, 1, 'nothing on the card offers the check: the saved record could never be closed from the interface');
    assert.ok(noticeText().includes('dnsEngine.guard.deadlineDue') && noticeText().includes('dnsEngine.guard.deadlineOffer'), noticeText());
    assert.ok(!text().includes('dnsEngine.operation.trackingDelayed'), '"tracking continues" is said beside "stopped checking"');
    assert.ok(!text().includes('dnsEngine.operation.autoTracking'), '"updating automatically" is said beside "stopped checking"');
    // The loop has stopped: no further read arrives by itself.
    const reads = sent('GET /api/v1/dns/engine');
    await afterFirstPoll();
    assert.equal(sent('GET /api/v1/dns/engine'), reads, 'the loop went on reading past its limit');

    await press('dnsEngine.guard.checkNow');
    assert.equal(sent(RECONCILE), 1);
    assert.deepEqual(changes(), [RECONCILE]);
    assert.ok(noticeText().startsWith('dnsEngine.guard.reconcileChecked') && noticeText().includes('dnsEngine.guard.deadlineOffer'), noticeText());

    serveDNS(dnsChange(40, 39), 'finishes');
    await press('dnsEngine.guard.checkNow');
    assert.equal(sent(RECONCILE), 1);
    assert.equal(notice().length, 0, 'the finished change still shows the notice');
    assert.deepEqual(globalThis.currentTest.toasts, [['success', 'dnsEngine.switchCompleted']]);
  } finally { await unmount(); }
});

// --- The Components page's firewall banner (2026-10-14, sixteenth record O4/O5) ---
// With the Agent stopped, GET /api/v1/firewall answers 503 AGENT_UNREACHABLE.
// The banner said the server's English sentence on a Turkish page and kept it
// after the Agent came back, because it read the firewall only on mount.
const AGENT_DOWN = () => answer({ error: 'The CelikPanel Agent on this server is not reachable.', code: 'AGENT_UNREACHABLE' }, 503);
const scanRoute = () => answer({ ...good['/api/v1/managed-services'], scanned_at: new Date().toISOString() });

test('Components: the firewall banner says a coded refusal in the interface language, not the server sentence', async () => {
  // The Turkish page: the catalogue's Turkish sentence for the code.
  globalThis.batch3Words = { 'err.AGENT_UNREACHABLE': tr['err.AGENT_UNREACHABLE'] };
  serve({ '/api/v1/firewall': AGENT_DOWN, '/api/v1/managed-services/scan': scanRoute });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    const shown = text();
    assert.ok(shown.includes('firewall.unknown'), `the banner does not say the state is unknown: ${shown.slice(0, 400)}`);
    assert.ok(shown.includes(tr['err.AGENT_UNREACHABLE']), `the Agent is not named in the interface language: ${shown.slice(0, 400)}`);
    assert.ok(!shown.includes('The CelikPanel Agent on this server is not reachable.'), 'the server sentence is shown instead of the translation');
    // A read that failed is not a failure of the firewall: the attention
    // surface, never the failure colour; and a command is never broken at its
    // hyphen ("celikpanel-" / "agent"; browser inspection 2026-10-14).
    const banner = tree.root.find((node) => node.props && node.props['data-firewall-unread'] !== undefined);
    const classes = [banner, ...banner.findAll((node) => typeof node.props?.className === 'string')].map((node) => node.props.className).join(' ');
    assert.ok(!/danger/.test(classes), `the unread firewall is drawn in the failure colour: ${classes}`);
    assert.match(banner.props.className, /warning-mark/);
    const whole = banner.findAll((node) => node.type === 'span' && /whitespace-nowrap/.test(node.props.className || '')).map((node) => node.props.children);
    assert.ok(whole.includes('celikpanel-agent'), `the command can break at its hyphen: ${JSON.stringify(whole)}`);
  } finally { await unmount(); delete globalThis.batch3Words; }
});

test('Components: the firewall banner is read again with the page’s scan, so the Agent coming back clears it', async () => {
  let firewall = AGENT_DOWN;
  globalThis.batch3Words = { 'err.AGENT_UNREACHABLE': tr['err.AGENT_UNREACHABLE'] };
  serve({ '/api/v1/firewall': (init) => firewall(init), '/api/v1/managed-services/scan': scanRoute });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    assert.ok(text().includes(tr['err.AGENT_UNREACHABLE']), text().slice(0, 400));
    const before = requests.filter((line) => line === 'GET /api/v1/firewall').length;
    // The Agent is running again; the owner rescans.
    firewall = () => answer(good['/api/v1/firewall']);
    await press('services.rescan');
    await settle();
    const after = requests.filter((line) => line === 'GET /api/v1/firewall').length;
    assert.ok(after > before, `the scan did not read the firewall again (${before} → ${after})`);
    const shown = text();
    assert.ok(!shown.includes('firewall.unknown') && !shown.includes(tr['err.AGENT_UNREACHABLE']), `the old Agent error stays: ${shown.slice(0, 400)}`);
    // alpha.94: a healthy firewall is one calm line.
    assert.ok(shown.includes('firewall.compact.on'), shown.slice(0, 400));
    assert.equal(requests.filter((line) => line.startsWith('POST ') && !line.startsWith('POST /api/v1/managed-services/scan')).length, 0, `the page changed something: ${requests.join(', ')}`);
  } finally { await unmount(); delete globalThis.batch3Words; }
});

// --- The Dashboard's attention list names the Agent (2026-10-14, sixteenth record O6) ---
test('Dashboard: with the Agent not reachable, the attention notice names the Agent and what the owner checks', async () => {
  globalThis.batch3Words = { 'err.AGENT_UNREACHABLE': tr['err.AGENT_UNREACHABLE'] };
  serve({ '/api/v1/managed-services': freshScan(), '/api/v1/firewall': AGENT_DOWN });
  await mount(dashboard());
  try {
    const shown = text();
    assert.ok(shown.includes('dashboard.attentionUnreadAgent'), `the notice is the generic one: ${shown.slice(0, 600)}`);
    assert.ok(shown.includes(tr['err.AGENT_UNREACHABLE']), 'the Agent sentence, with the owner’s commands, is missing');
    assert.ok(!shown.includes('dashboard.attentionUnread '), 'the generic "nothing is wrong" notice is not shown beside it');
    for (const phrase of CLAIMS) assert.ok(!shown.includes(phrase), `"${phrase}" is on screen while the firewall is unknown`);
    assert.equal(requests.filter((line) => !line.startsWith('GET ')).length, 0, `the page sent a change by itself: ${requests.join(', ')}`);
  } finally { await unmount(); delete globalThis.batch3Words; }
  // Any other refusal keeps the generic notice.
  serve({ '/api/v1/managed-services': freshScan(), '/api/v1/firewall': REFUSED });
  await mount(dashboard());
  try {
    assert.ok(text().includes('dashboard.attentionUnread') && !text().includes('dashboard.attentionUnreadAgent'), text().slice(0, 400));
  } finally { await unmount(); }
});

// --- The Dashboard's mail-stack card follows the page's own check (2026-10-14) ---
// It said "Status needs refresh", "Open Components and rescan before trusting
// mail status" and offered "Open Components to rescan" while the page was
// checking by itself (D-032), and showed the server's English profile text.
const STALE = '2026-10-09T09:00:00Z';
const mailScan = (scannedAt, profiles) => ({ ...good['/api/v1/managed-services'], scanned_at: scannedAt, profiles });
const MAIL_STALE_KEYS = ['dashboard.mailStacks.rescan', 'dashboard.mailStacks.scanStale', 'dashboard.mailStacks.status.stale'];

test('Dashboard bundles card: while the page checks by itself it says so, with motion, and asks nothing', async () => {
  serve({ '/api/v1/managed-services': mailScan(STALE, [profile('core-mail'), profile('webmail'), profile('protected-mail')]), '/api/v1/managed-services/scan': HANG });
  await mount(dashboard());
  try {
    const card = tree.root.find((node) => node.props['aria-labelledby'] === 'dashboard-bundles-heading');
    const labels = card.findAll((node) => node.props.role === 'status' && typeof node.type === 'string').map((node) => node.props['aria-label']);
    assert.ok(labels.includes('dashboard.systemServicesChecking'), `the card does not say it is checking: ${text().slice(0, 600)}`);
    const shown = text();
    for (const key of MAIL_STALE_KEYS) assert.ok(!shown.includes(key), `"${key}" is on screen while the page checks by itself`);
    assert.equal(requests.filter((line) => line.startsWith('POST /api/v1/managed-services/scan')).length, 1, 'one check, by the page');
  } finally { await unmount(); }
});

test('Dashboard bundles card: a check that could not run says so, without sending the owner to rescan', async () => {
  serve({ '/api/v1/managed-services': mailScan(STALE, [profile('core-mail'), profile('webmail'), profile('protected-mail')]), '/api/v1/managed-services/scan': REFUSED });
  await mount(dashboard());
  try {
    await settle();
    const card = tree.root.find((node) => node.props['aria-labelledby'] === 'dashboard-bundles-heading');
    const cardText = label(card);
    assert.ok(cardText.includes('dashboard.bundles.checkFailed'), `the card does not say the check failed: ${cardText}`);
    assert.ok(!cardText.includes('dashboard.bundles.summary'), 'a stale record is read as a count');
    for (const key of MAIL_STALE_KEYS) assert.ok(!text().includes(key), `"${key}" is on screen`);
    assert.ok(cardText.includes('dashboard.bundles.open'), 'the card still opens the bundles');
  } finally { await unmount(); }
});

test('Dashboard bundles card: what is installed is counted in the interface language, and the door opens the bundles', async () => {
  globalThis.batch3Words = {
    'dashboard.audit.profile.webmail': trScreens['dashboard.audit.profile.webmail'],
    'dashboard.bundles.summary': trScreens['dashboard.bundles.summary'],
  };
  globalThis.batch3Navigated = [];
  const installed = { ...mailScan(new Date().toISOString(), [profile('core-mail'), profile('webmail'), profile('protected-mail')]) };
  installed.bundles = mailBundles().map((item) => (item.id === 'mail.webmail'
    ? { ...item, status: 'installed_verified', action: 'repair', missing_components: [] }
    : item));
  serve({ '/api/v1/managed-services': installed });
  await mount(dashboard());
  try {
    const card = tree.root.find((node) => node.props['aria-labelledby'] === 'dashboard-bundles-heading');
    const cardText = label(card);
    // alpha.95: one summary line, counted as the Components chips are drawn;
    // a verified bundle has no row of its own.
    assert.ok(cardText.includes('1 kurulu ve doğrulandı · 2 kurulabilir'), cardText);
    assert.ok(!cardText.includes(trScreens['dashboard.audit.profile.webmail']), cardText);
    await act(async () => { card.findAll((node) => node.type === 'button').find((node) => label(node).includes('dashboard.bundles.open')).props.onClick(); });
    assert.deepEqual(globalThis.batch3Navigated, ['/services#bundles']);
    assert.equal(requests.filter((line) => !line.startsWith('GET ')).length, 0, `the card sent a change: ${requests.join(', ')}`);
  } finally { await unmount(); delete globalThis.batch3Words; delete globalThis.batch3Navigated; }
});

test('Components: a paused server change says the page lifts it by itself, without asking for a reload', async () => {
  serve({ '/api/v1/host-mutation-readiness': { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' }, '/api/v1/managed-services/scan': scanRoute });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    const shown = text();
    assert.ok(shown.includes('services.mutationReadiness.package_manager_active'), shown.slice(0, 400));
    assert.ok(shown.includes('services.mutationReadiness.autoResume'), 'the banner does not say it lifts by itself');
  } finally { await unmount(); }
});

// Browser batch 14 (2026-10-15): with the page locked ("needs verification"),
// Manage, Manage DNS engine and Uninstall were disabled by the row's fieldset
// but drawn as if they could be pressed; and a longer translated description
// was cut with an ellipsis at 390 px ("nftables — panel güvenlik duvarının
// paket süzme mot…"). Every button of a row says it is off when it is off,
// and a description wraps.
// Kilitli sayfada satırın düğmeleri kapalı olduklarını gösterir; açıklama
// kesilmez, satır kırar.
test('Components: every row button looks off when the row is locked, and a description wraps instead of being cut', async () => {
  serve({ '/api/v1/managed-services/scan': scanRoute });
  await mount(React.createElement(ServiceList, {}));
  try {
    for (let i = 0; i < 3; i += 1) await settle();
    const rows = tree.root.findAll((node) => node.type === 'fieldset' && String(node.props.className || '').includes('relative'));
    const rowButtons = rows.flatMap((row) => row.findAll((node) => node.type === 'button'));
    assert.ok(rowButtons.length >= 3, `expected the rows' buttons, found ${rowButtons.length}`);
    for (const button of rowButtons) {
      assert.match(String(button.props.className || ''), /disabled:(?:opacity|cursor)/, `"${label(button) || button.props.title}" has no disabled look`);
    }
    const descriptions = tree.root.findAll((node) => node.type === 'div' && / on this server$/.test(label(node)) && !node.findAll((child) => child !== node && child.type === 'div').length);
    assert.ok(descriptions.length >= 2, `no component descriptions drawn: ${text().slice(0, 300)}`);
    for (const node of descriptions) assert.doesNotMatch(String(node.props.className || ''), /\btruncate\b/, label(node));
  } finally { await unmount(); }
});

// --- A stored component record the server cannot verify (2026-10-15, D-032) ---
// The Panel refuses it with 503 SERVICE_STATE_UNVERIFIED. The sentence used to
// ask the owner to run Rescan; the Dashboard and the Components page now answer
// it with their own conditional check (a read that writes a new record), once,
// with no toast, as they do for a stale record.
// Sunucunun doğrulayamadığı kayıtlı bileşen kaydı: Pano ve Bileşenler sayfası
// sahibe Yeniden Tara dedirtmeden kendi koşullu kontrolünü bir kez yapar.
const UNVERIFIED_RECORD = () => answer({ error: 'the stored component record could not be verified', code: 'SERVICE_STATE_UNVERIFIED' }, 503);

test('Dashboard: a stored record the server cannot verify → one conditional check by itself, its result drawn, no toast', async () => {
  serve({ '/api/v1/managed-services': UNVERIFIED_RECORD, [SCAN_PATH]: (init) => (init.method === 'POST' ? answer(freshScan()) : answer({}, 405)) });
  recordScanURLs();
  await mount(dashboard());
  try {
    for (let i = 0; i < 4; i += 1) await settle();
    assert.deepEqual(scanURLs, [`${SCAN_PATH}?max_age_seconds=300`], `expected one conditional check: ${requests.join(', ')}`);
    const shown = text();
    assert.ok(shown.includes('0 / 0'), `the result is not drawn: ${shown.slice(0, 400)}`);
    assert.ok(!shown.includes('dashboard.attentionUnread'), `the record is reported as unreadable: ${shown.slice(0, 400)}`);
    assert.deepEqual(globalThis.currentTest.toasts, [], 'a check nobody pressed raised a toast');
    assert.equal(requests.filter((line) => line.startsWith('POST ') && line !== `POST ${SCAN_PATH}`).length, 0, `the page changed something: ${requests.join(', ')}`);
  } finally { await unmount(); }
});

test('Dashboard: the check after an unverifiable record fails → "could not check", not asked again by itself', async () => {
  serve({ '/api/v1/managed-services': UNVERIFIED_RECORD, [SCAN_PATH]: () => answer({ error: 'agent unavailable' }, 502) });
  await mount(dashboard());
  try {
    for (let i = 0; i < 6; i += 1) await settle();
    assert.equal(scans(), 1, `the failure was asked again by itself: ${requests.join(', ')}`);
    assert.ok(text().includes('dashboard.systemServicesCheckFailed'), text().slice(0, 400));
  } finally { await unmount(); }
});

test('Components: a stored record the server cannot verify → one conditional check, the list drawn, no error toast', async () => {
  serve({ '/api/v1/managed-services': UNVERIFIED_RECORD, [SCAN_PATH]: scanRoute });
  recordScanURLs();
  await mount(React.createElement(ServiceList, {}));
  try {
    for (let i = 0; i < 4; i += 1) await settle();
    assert.equal(scanURLs.length, 1, `expected one check: ${requests.join(', ')}`);
    assert.match(scanURLs[0], /\?max_age_seconds=\d+$/, 'the page asked for an unconditional scan');
    assert.deepEqual(globalThis.currentTest.toasts.filter(([kind]) => kind === 'error'), [], 'the stored record was reported as a failed read');
    assert.ok(text().includes('pdns'), `the list is not drawn: ${text().slice(0, 400)}`);
  } finally { await unmount(); }
});

test('the sentence for an unverifiable stored record says the panel checks by itself, in both languages', () => {
  for (const catalogue of [en, tr]) {
    const sentence = catalogue['err.SERVICE_STATE_UNVERIFIED'];
    assert.ok(sentence, 'no sentence');
    assert.doesNotMatch(sentence, /Rescan|Yeniden Tara|rescan|yeniden tara/, sentence);
    assert.match(sentence, /CelikPanel/);
  }
  assert.notEqual(en['err.SERVICE_STATE_UNVERIFIED'], tr['err.SERVICE_STATE_UNVERIFIED']);
});


// --- Second server as a full peer, part 1 (2026-10-16) ----------------------------
// Gap 7: on a secondary DNS server the mail box says where the record is made
// (the primary, or the DNS provider) and the exact record, and offers no
// button to a DNS screen that cannot create one there.
// Gap 2: on a server whose setup has finished, a mail profile is added through
// a setup extension and reviewed in setup, so it gets setup's steps and checks.
// Gap 3: a saved mail host name can be corrected, with what changes said first.
const secondaryGuidance = { reason: 'dns_secondary', primary_nameserver: 'ns1.example.net', primary_ip: '203.0.113.10', record_name: 'mail.boston.example.net', record_type: 'A', record_value: '198.51.100.7' };
// While the DNS identity is not ready the Panel blocks every mail bundle.
const secondaryScan = (guidance) => ({ ...freshScan(), dns_identity_ready: false, bundles: mailBundles({ status: 'blocked', action: 'none', blocked_code: 'dns_identity_not_ready' }), ...(guidance ? { mail_dns_guidance: guidance } : {}) });
// A mail bundle opens the mail install dialogue: its chip says Install and
// opens it directly (alpha.95).
const openMailBundle = async () => {
  const chip = tree.root.find((node) => node.type === 'button' && node.props['data-bundle'] === 'mail.core');
  await act(async () => { chip.props.onClick(); });
  await settle();
};
const readySetup = () => ({ version: 1, revision: 7, origin: 'fresh', status: 'ready', required: false, guidance: 'guided', checks: [],
  draft: { purpose: 'web', panel_domain: 'boston.example.net', mail_hostname: '', dns_mode: 'external', remote_dns_connection_id: '', dns_engine: 'bind', dns_role: 'primary', ns1: '', ns2: '', local_ip: '', peer_ip: '', peer_ns: '', node_version: '', database: 'mariadb' } });
const savedNameScan = () => ({ ...freshScan(), mail_hostname: { current: 'server1', hostname: 'mail.example.net', source: 'saved', current_usable: true, will_set_hostname: false } });
function setupContext(snapshot) {
  const context = { snapshot, accepted: [], reload: async () => context.snapshot, accept(next) { context.snapshot = next; context.accepted.push(next); } };
  return context;
}
function browserStorage() {
  // The least of a document a dialog's effects touch.
  globalThis.document = { addEventListener() {}, removeEventListener() {}, getElementById: () => null, activeElement: null, body: { style: {} } };
  const kept = new Map();
  globalThis.sessionStorage = { getItem: (key) => kept.get(key) ?? null, setItem: (key, value) => kept.set(key, String(value)), removeItem: (key) => kept.delete(key) };
  return kept;
}
const hasNode = (marker) => tree.root.findAll((node) => node.props && node.props[marker] !== undefined).length > 0;

test('Components: on a secondary DNS server the mail box names the primary and the exact record, with no DNS-settings dead end', async () => {
  serve({ '/api/v1/managed-services': secondaryScan(secondaryGuidance) });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    const shown = text();
    assert.ok(shown.includes('services.mailProfiles.secondary.body{"primary":"ns1.example.net (203.0.113.10)"}'), shown.slice(0, 800));
    const record = tree.root.find((node) => node.props && node.props['data-mail-dns-record'] !== undefined);
    assert.equal(label(record), 'mail.boston.example.netA198.51.100.7');
    assert.equal(buttons().filter((node) => label(node).includes('services.mailProfiles.configureDNS')).length, 0, 'the dead-end button is still offered');
    assert.ok(buttons().some((node) => label(node).includes('services.mailProfiles.secondary.openSetup')));
    assert.ok(shown.includes('services.mailProfiles.secondary.short'));
    // No bundle is offered beside a record that is not there yet (2026-10-16):
    // the chips stay, blocked, with the short reason and no button (alpha.95).
    const chips = tree.root.findAll((node) => node.props && node.props['data-bundle'] !== undefined && node.props['data-bundle-state'] !== undefined);
    assert.equal(chips.length, 3, shown.slice(0, 800));
    assert.ok(chips.every((node) => node.props['data-bundle-state'] === 'blocked' && node.props['aria-expanded'] === false), shown.slice(0, 800));
    assert.ok(chips.every((node) => label(node).includes('services.bundles.short.dns_identity_not_ready')), shown.slice(0, 800));
  } finally { await unmount(); }
  // Control: the same server without the guidance keeps the generic box.
  serve({ '/api/v1/managed-services': secondaryScan(null) });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    assert.ok(buttons().some((node) => label(node).includes('services.mailProfiles.configureDNS')));
    assert.ok(!text().includes('services.mailProfiles.secondary.body'));
  } finally { await unmount(); }
});

test('Components: a server whose setup has finished adds mail through a setup extension and opens setup on its access step', async () => {
  globalThis.batch3Dialogs = true;
  globalThis.batch3Navigated = [];
  const kept = browserStorage();
  const context = globalThis.batch3Setup = setupContext(readySetup());
  let sent = null;
  serve({
    '/api/v1/managed-services': savedNameScan(),
    '/api/v1/setup/extend': (init) => {
      sent = JSON.parse(init.body);
      const base = readySetup();
      return answer({ ...base, revision: 8, status: 'draft', extension: true, extension_base: base.draft, draft: { ...base.draft, mail_hostname: 'mail.example.net', customization: { components: ['dovecot', 'mariadb', 'nginx', 'php-fpm', 'postfix'] } } });
    },
  });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    // Control for the secondary server's strip: with its DNS identity ready the mail bundles are offered.
    assert.ok(tree.root.findAll((node) => node.type === 'button' && node.props['data-bundle'] === 'mail.core').length === 1 && !hasNode('data-mail-dns-secondary'));
    await openMailBundle();
    const shown = text();
    assert.ok(hasNode('data-mail-profile-route') && shown.includes('services.mailProfiles.setupRoute.note'), shown.slice(0, 800));
    assert.ok(!shown.includes('services.mailProfiles.plan.acknowledgement'), 'nothing starts here, so nothing is acknowledged here');
    await press('services.mailProfiles.setupRoute.confirm');
    assert.deepEqual(sent, { revision: 7, action: 'begin', mail_profile: 'core-mail' });
    assert.equal(requests.filter((line) => line === 'POST /api/v1/setup/extend').length, 1);
    assert.equal(requests.filter((line) => line.startsWith('POST /api/v1/service/profile/install')).length, 0, 'the weaker direct install ran');
    assert.equal(context.snapshot.extension, true);
    assert.deepEqual(globalThis.batch3Navigated, ['/setup']);
    const editor = JSON.parse(kept.get('celikpanel.setup.editor.admin'));
    assert.equal(editor.step, 'access');
    assert.equal(editor.revision, 8);
  } finally { await unmount(); delete globalThis.batch3Dialogs; delete globalThis.batch3Setup; delete globalThis.batch3Navigated; delete globalThis.sessionStorage; delete globalThis.document; }
  // Control: without a finished setup the dialog installs directly and says
  // that its certificate is temporary.
  globalThis.batch3Dialogs = true;
  browserStorage();
  serve({ '/api/v1/managed-services': savedNameScan() });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    await openMailBundle();
    const shown = text();
    assert.ok(shown.includes('services.mailProfiles.directRoute.note') && shown.includes('services.mailProfiles.plan.acknowledgement'), shown.slice(0, 800));
    assert.ok(!shown.includes('services.mailProfiles.setupRoute.confirm'));
  } finally { await unmount(); delete globalThis.batch3Dialogs; delete globalThis.sessionStorage; delete globalThis.document; }
});

test('Components: a refused setup extension keeps the dialog open with the reason and opens nothing', async () => {
  globalThis.batch3Dialogs = true;
  globalThis.batch3Navigated = [];
  browserStorage();
  globalThis.batch3Setup = setupContext(readySetup());
  let release;
  serve({
    '/api/v1/managed-services': savedNameScan(),
    '/api/v1/setup/extend': () => new Promise((resolve) => { release = () => resolve(answer({ error: 'busy', code: 'server_setup_busy' }, 409)); }),
  });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    await openMailBundle();
    await press('services.mailProfiles.hostname.change');
    await press('services.mailProfiles.setupRoute.confirm');
    // While setup is asked, nothing in the dialog can change or close what was sent (2026-10-16).
    const locked = () => ({
      cancel: buttons().find((node) => label(node).includes('common.cancel')).props.disabled,
      keep: buttons().find((node) => label(node).includes('services.mailProfiles.hostname.keep')).props.disabled,
      name: tree.root.find((node) => node.props && node.props.id === 'mail-profile-hostname').props.disabled,
    });
    assert.deepEqual(locked(), { cancel: true, keep: true, name: true });
    await act(async () => { release(); });
    await settle();
    assert.deepEqual(locked(), { cancel: false, keep: false, name: false });
    assert.ok(text().includes('services.mailProfiles.setupRoute.busy'), text().slice(0, 800));
    assert.deepEqual(globalThis.batch3Navigated, []);
    assert.equal(requests.filter((line) => line === 'POST /api/v1/setup/extend').length, 1, 'the refused request was sent again');
  } finally { await unmount(); delete globalThis.batch3Dialogs; delete globalThis.batch3Setup; delete globalThis.batch3Navigated; delete globalThis.sessionStorage; delete globalThis.document; }
});

test('Components: a saved mail host name can be corrected, and what changes is said before it is confirmed', async () => {
  globalThis.batch3Dialogs = true;
  globalThis.batch3Navigated = [];
  browserStorage();
  globalThis.batch3Setup = setupContext(readySetup());
  let sent = null;
  serve({
    '/api/v1/managed-services': savedNameScan(),
    '/api/v1/setup/extend': (init) => { sent = JSON.parse(init.body); return answer({ ...readySetup(), revision: 8, status: 'draft', extension: true, extension_base: readySetup().draft }); },
  });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    await openMailBundle();
    assert.ok(text().includes('services.mailProfiles.hostname.settled'), text().slice(0, 800));
    assert.ok(!hasNode('data-mail-hostname-effects'), 'effects said before any change');
    await press('services.mailProfiles.hostname.change');
    const input = tree.root.find((node) => node.props && node.props.id === 'mail-profile-hostname');
    assert.equal(input.props.value, 'mail.example.net');
    await act(async () => { input.props.onChange({ target: { value: 'smtp.example.net' } }); });
    const effects = tree.root.find((node) => node.props && node.props['data-mail-hostname-effects'] !== undefined);
    assert.equal(label(effects), 'services.mailProfiles.hostname.changeEffects{"old":"mail.example.net","new":"smtp.example.net"}');
    await press('services.mailProfiles.setupRoute.confirm');
    assert.deepEqual(sent, { revision: 7, action: 'begin', mail_profile: 'core-mail', mail_hostname: 'smtp.example.net' });
  } finally { await unmount(); delete globalThis.batch3Dialogs; delete globalThis.batch3Setup; delete globalThis.batch3Navigated; delete globalThis.sessionStorage; delete globalThis.document; }
});


// Second server as a full peer, part 2 (2026-10-16): the box says whether this
// secondary has an authorized DNS connection to the primary, so setup asks for
// the record by itself, or has none, so the record is added by hand.
test('Components: on a secondary the mail box says whether setup asks the primary for the record or the owner adds it', async () => {
  serve({ '/api/v1/managed-services': secondaryScan({ ...secondaryGuidance, peer_record_channel: true }) });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    const shown = text();
    assert.ok(shown.includes('services.mailProfiles.secondary.channel'), shown.slice(0, 800));
    assert.ok(!shown.includes('services.mailProfiles.secondary.noChannel'));
    assert.ok(tree.root.findAll((node) => node.props && node.props['data-mail-dns-channel'] === 'yes').length === 1);
    // The exact record stays visible: it is still the manual route if the primary refuses.
    assert.ok(hasNode('data-mail-dns-record'));
  } finally { await unmount(); }
  // Control: no channel (and an older server that does not say) reads as manual.
  for (const guidance of [{ ...secondaryGuidance, peer_record_channel: false }, secondaryGuidance]) {
    serve({ '/api/v1/managed-services': secondaryScan(guidance) });
    await mount(React.createElement(ServiceList, {}));
    try {
      await settle();
      const shown = text();
      assert.ok(shown.includes('services.mailProfiles.secondary.noChannel'));
      assert.ok(!shown.includes('services.mailProfiles.secondary.channel"'));
    } finally { await unmount(); }
  }
});

// alpha.92 (2026-10-16): "Repair webmail" beside an installed Roundcube was
// amber on every server. It is drawn like Manage; amber only when the Webmail
// profile reports a webmail problem.
// Kurulu Roundcube'un "Webmail'i onar" düğmesi her sunucuda sarıydı; yalnız
// Webmail profili bir sorun bildirdiğinde sarıdır.
// alpha.95 (owner's Frankfurt screen, 2026-10-17): without a reported problem
// the row carries no Repair webmail at all; webmail repair lives in the
// Webmail bundle's dialogue.
// Sorun bildirilmediğinde satırda Webmail'i onar yoktur.
test('Components: Repair webmail stands on the Roundcube row only when webmail reports a problem', async () => {
  const roundcubeScan = (warningCode) => {
    const base = scan([
      { ...component('postfix', []), category: 'mail' },
      { ...component('roundcube', []), category: 'mail', kind: 'tool', status: 'installed', repair_available: true },
    ]);
    base.profiles = base.profiles.map((item) => (item.id === 'webmail' && warningCode
      ? { ...item, warning: 'Roundcube is not ready.', warning_code: warningCode }
      : item));
    return base;
  };
  const repairButton = () => tree.root.findAll((node) => node.type === 'button' && node.props['data-tone'] !== undefined)[0];
  serve({ '/api/v1/managed-services': roundcubeScan(undefined) });
  await mount(React.createElement(ServiceList, {}));
  try {
    await settle();
    assert.equal(repairButton(), undefined, 'Repair webmail stands on the row without a problem');
    assert.ok(!text().includes('services.repairWebmail'), text().slice(0, 600));
  } finally { await unmount(); }
  for (const [code, tone] of [['roundcube_not_ready', 'warning'], ['webmail_unverified', 'warning']]) {
    serve({ '/api/v1/managed-services': roundcubeScan(code) });
    await mount(React.createElement(ServiceList, {}));
    try {
      await settle();
      const button = repairButton();
      assert.ok(button, `no Repair webmail button (${code}): ${text().slice(0, 600)}`);
      assert.ok(label(button).includes('services.repairWebmail'));
      assert.equal(button.props['data-tone'], tone, `warning code ${code}`);
      assert.equal(/warning/.test(button.props.className), tone === 'warning', button.props.className);
    } finally { await unmount(); }
  }
});

// D-024 (alpha.94 review): a failed mail attempt is said again on the
// Dashboard's Ready bundles card, with the Panel's own error, in the interface
// language; a running one says that it runs. Nothing is sent.
test('Dashboard bundles card: a failed mail attempt is said with its error, a running one says it runs', async () => {
  globalThis.batch3Words = {
    'dashboard.audit.profile.coreMail': trScreens['dashboard.audit.profile.coreMail'],
    'dashboard.audit.profile.webmail': trScreens['dashboard.audit.profile.webmail'],
    'services.mailProfiles.attemptError': trServerScreens['services.mailProfiles.attemptError'],
    'dashboard.bundles.attemptFailed': trScreens['dashboard.bundles.attemptFailed'],
    'dashboard.bundles.attemptRunning': trScreens['dashboard.bundles.attemptRunning'],
  };
  const answer = mailScan(new Date().toISOString(), [
    { ...profile('core-mail'), status: 'partial', latest_attempt_status: 'failed', latest_attempt_error: 'submission verification failed' },
    { ...profile('webmail'), latest_attempt_status: 'in_progress' },
    profile('protected-mail'),
  ]);
  serve({ '/api/v1/managed-services': answer });
  await mount(dashboard());
  try {
    const card = tree.root.find((node) => node.props['aria-labelledby'] === 'dashboard-bundles-heading');
    const cardText = label(card);
    assert.ok(cardText.includes('Temel Posta: Son deneme şunu bildirdi: submission verification failed'), cardText);
    assert.ok(cardText.includes(trScreens['dashboard.bundles.attemptRunning'].replace('{name}', trScreens['dashboard.audit.profile.webmail'])), cardText);
    assert.equal(requests.filter((line) => !line.startsWith('GET ')).length, 0, requests.join(', '));
  } finally { await unmount(); delete globalThis.batch3Words; }
});
