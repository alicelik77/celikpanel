import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, remoteURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { englishCatalogue, englishScreens, turkishCatalogue, turkishScreens } from './locale-catalogue.mjs';

// NO NEGATIVE UI UNLESS KNOWN, on the mounted screens (9 Oct 2026, D-024).
//
// Every screen of the first batch is mounted with the real shared layer and one
// of its reads is made to hang, to drop, to be refused or to answer with
// something that is not the contract. In each of those cases the screen must
// not say that something is missing, empty or not ready, and it must not offer
// an enabled control that submits or removes anything. The same read is then
// answered with a real negative, to show that the absence above is not an
// accident of the fixture: the text does appear once the server has said so.
//
// The report this comes from (8 Oct 2026): opening "Add domain" on a server
// that has DNS showed "an active, panel-managed DNS server is required…
// [Choose a DNS engine]" with the form disabled for the seconds the read took.

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const screenFiles = [
  'AddDomainModal', 'Domains', 'DatabaseManagementV2', 'DatabaseAccountStrip', 'DomainDatabaseManager',
  'DomainConnection', 'DomainDNSManager', 'DomainPHPSettings', 'HostingTypePanel',
  // The second batch.
  'Settings', 'UsersPage', 'DomainFileManager', 'DomainSSLSettings', 'ImportPage', 'LicenseNotice', 'MonitoringPage',
  'ComponentDetail', 'ServiceRecordLookup', 'ServiceShell', 'DomainDetail',
  // The infrastructure zone's page (2026-10-16).
  'InfrastructureZone',
  // My account, whose two-factor panel was a Settings section until
  // v0.1.0-alpha.98 (D-037).
  'AccountPage',
];
const icons = new Set(['AlertTriangle', 'XCircle']);
// Children of the screens under test that are not under test themselves.
const stubbedComponents = [
  'Link', 'PHPToolVersion', 'RepoCard', 'SecurityUpdatesCard', 'RoundcubeUpdateCard', 'WebmailAddressCard', 'RestartsCard', 'AppAccountLine', 'IsolationFact', 'TeamMembersPage', 'DNSServerSettings', 'SecurityAuditCard', 'ServerSetupSettings',
  'DomainGeneralSettings', 'DomainSSLOverviewCard', 'DomainMailManager', 'ChangePasswordModal', 'ChangeEmailModal',
];
for (const name of screenFiles) {
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
  const i18n = { t: (key) => key, locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => (path) => { globalThis.currentTest.navigated.push(path); };
  export const useLocation = () => ({ pathname: '/', search: '', hash: '' });
  export const useAuth = () => ({ role: globalThis.currentTest.role });
  export const showToast = (...args) => globalThis.currentTest.toasts.push(args);
  export const PageHeader = (props) => React.createElement('header', null, props.title, props.subtitle, props.actions);
  export const HelpButton = () => null;
  export const AddDatabaseModalV2 = () => React.createElement('aside', null, 'add-database-dialog');
  export const AddUserModalV2 = () => React.createElement('aside', null, 'add-user-dialog');
  export const useSearchParams = () => [new URLSearchParams(globalThis.currentTest.search ?? 'section=' + (globalThis.currentTest.section ?? 'panel')), () => {}];
  // The fail-closed decoder is ComponentOperation's; here it is the same test
  // on the one field these screens read. The operation tracker is not mounted.
  export const decodeManagedServicesSnapshot = (value) => (value && typeof value === 'object' && Array.isArray(value.services) ? value : null);
  export const useComponentOperation = () => globalThis.currentTest.operation ?? { startInstall: async () => true, locked: false, checking: false, catalogSnapshot: null };
  export const publishComponentCensus = () => {};
  // The Panel's stored update check (alpha.96): nothing found here.
  export const availablePanelUpdate = () => null;
  export const wholeVersion = (text) => text;
  export const usePanelUpdateDiscovery = () => ({ status: 'idle', value: null, readAt: 0 });
  ${stubbedComponents.filter((name) => !icons.has(name)).map((name) => `export const ${name} = (props) => props.children ?? null;`).join('\n')}
`);
const shared = sharedLayer(stub);
const domainAccessURL = compileSource('auth/domainAccess.ts', () => stub);
const deletionURL = compileSource('lib/domainDeletionPending.ts', () => stub);
// The small readers beside lib/remote.ts are the real ones too.
const besideRemote = (specifier) => (specifier.endsWith('/remote') ? remoteURL : shared.resolve(specifier));
const subscriptionsURL = compileSource('lib/subscriptions.ts', besideRemote);
const accountsURL = compileSource('lib/accounts.ts', besideRemote);
const managedServicesURL = compileSource('lib/managedServices.ts', besideRemote);
const sslTierURL = compileSource('lib/sslTier.ts', () => stub);
const hstsRemainingURL = compileSource('lib/hstsRemaining.ts', () => stub);
// The identity a state-changing request carries is the real one (D-029).
const requestIdentityURL = compileSource('lib/requestIdentity.ts', () => stub);
const own = {
  '/auth/domainAccess': domainAccessURL, '/lib/domainDeletionPending': deletionURL, '/lib/subscriptions': subscriptionsURL,
  '/lib/accounts': accountsURL, '/lib/managedServices': managedServicesURL, '/lib/sslTier': sslTierURL, '/lib/hstsRemaining': hstsRemainingURL,
  '/lib/requestIdentity': requestIdentityURL,
  // Reads that run only while the page is visible are the real ones (2026-10-15).
  '/lib/visibleReads': compileSource('lib/visibleReads.ts', () => stub),
  // Component names in the interface language (2026-10-15).
  '/lib/componentName': compileSource('lib/componentName.ts', () => stub),
  // The infrastructure DNS zones beside the domains (2026-10-16).
  '/lib/infrastructureZones': compileSource('lib/infrastructureZones.ts', besideRemote),
};
// The two notices of 10 Oct 2026 are the shipped ones: what a service action
// ended with, and a change whose one-time result is not kept.
own['/ServiceActionNotice'] = shared.compile('components/ServiceActionNotice.tsx', own);
own['/OnceOnlyNotice'] = shared.compile('components/OnceOnlyNotice.tsx', own);
// So is the notice of a certificate request certbot did not fulfil (11 Oct 2026).
own['/CertificateIssueNotice'] = shared.compile('components/CertificateIssueNotice.tsx', own);
const modalURL = shared.compile('components/AddDomainModal.tsx', own);
const stripURL = shared.compile('components/DatabaseAccountStrip.tsx', own);
const load = async (name, more = {}) => (await import(shared.compile(`components/${name}.tsx`, { ...own, ...more })))[name];
const AddDomainModal = (await import(modalURL)).AddDomainModal;
const Domains = await load('Domains', { '/AddDomainModal': modalURL });
const DatabaseManagementV2 = await load('DatabaseManagementV2', { '/DatabaseAccountStrip': stripURL });
const DomainDatabaseManager = await load('DomainDatabaseManager');
const DomainConnection = await load('DomainConnection');
const DomainDNSManager = await load('DomainDNSManager');
const DomainPHPSettings = await load('DomainPHPSettings');
const HostingTypePanel = await load('HostingTypePanel');
// The second batch. A screen that mounts another screen under test gets the
// real one; everything else it mounts is the stub.
const Settings = await load('Settings');
const AccountPage = await load('AccountPage');
const UsersPage = await load('UsersPage');
const fileManagerURL = shared.compile('components/DomainFileManager.tsx', own);
const DomainFileManager = (await import(fileManagerURL)).DomainFileManager;
const sslSettingsURL = shared.compile('components/DomainSSLSettings.tsx', own);
const DomainSSLSettings = (await import(sslSettingsURL)).DomainSSLSettings;
const ImportPage = await load('ImportPage');
const LicenseNotice = await load('LicenseNotice');
const MonitoringPage = await load('MonitoringPage');
const ServiceShell = await load('ServiceShell');
const ComponentDetail = await load('ComponentDetail', { '/ServiceShell': dataModule(`export const ServiceShell = (props) => props.children;`) });
const ServiceRecordLookup = await load('ServiceRecordLookup');
const DomainDetailByName = (await import(shared.compile('components/DomainDetail.tsx', {
  ...own,
  '/DomainFileManager': fileManagerURL,
  '/DomainSSLSettings': sslSettingsURL,
  '/DomainConnection': shared.compile('components/DomainConnection.tsx', own),
  '/DomainDatabaseManager': shared.compile('components/DomainDatabaseManager.tsx', own),
  '/DomainDNSManager': shared.compile('components/DomainDNSManager.tsx', own),
  '/DomainPHPSettings': shared.compile('components/DomainPHPSettings.tsx', own),
  '/HostingTypePanel': shared.compile('components/HostingTypePanel.tsx', own),
}))).DomainDetailByName;

// --- What the server answers when nothing is wrong ----------------------------
const caps = (overrides = {}) => ({
  web_server: 'nginx', php_versions: ['8.3'], dns_server: 'bind', dns_identity_ready: true,
  dns_management_mode: 'local', dns_management_ready: true, mail_server: true,
  database_servers: ['mariadb'], db_tools: [], ...overrides,
});
const domainRow = { id: 1, domain_name: 'example.com', status: 'active', created_at: '2026-01-01T00:00:00Z', project_type: 'php', php_version: '8.3' };
const engine = {
  id: 1, type_id: 1, type_name: 'mariadb', type_icon: 'M', name: 'MariaDB', version: '11.4', host: 'localhost', port: 3306,
  is_default: true, status: 'active', created_at: '', admin_username: 'celikpanel', is_local: true,
};
const connection = (overrides = {}) => ({
  domain: 'example.com', server_ip: '192.0.2.4', nameservers: ['ns1.example.net', 'ns2.example.net'],
  live_nameservers: ['ns1.example.net', 'ns2.example.net'], live_ips: ['192.0.2.4'], status: 'delegated', ssl_ready: true,
  glue_needed: false, nameservers_usable: true, checked_at: '2026-10-09T00:00:00Z', dns_management_mode: 'local', ...overrides,
});
const json = (body, status = 200) => () => Response.json(body, { status });
const healthy = {
  '/api/v1/hosting/capabilities': json(caps()),
  '/api/v1/domains': json([domainRow]),
  '/api/v1/subscriptions': json({ subscriptions: [] }),
  // An administrator's Domains page also reads the infrastructure zones (2026-10-16).
  '/api/v1/dns/infrastructure-zones': json({ zones: [] }),
  '/api/v1/database-servers': json([engine]),
  '/api/v1/database-servers/1/databases': json([{ id: 5, name: 'shop', users: ['shop'], created_at: '' }]),
  '/api/v1/database-servers/1/users': json([{ id: 7, username: 'shop', databases: ['shop'], created_at: '' }]),
  '/api/v1/domains/1/databases': json({ databases: [{ id: 3, name: 'example_com_app', type: 'mysql', user: 'app', created_at: '2026-01-01T00:00:00Z' }], available_types: ['mysql'] }),
  '/api/v1/domains/1/connection': json(connection()),
  '/api/v1/domains/1/dns/zone': json({ type: 'NATIVE', management: 'local' }),
  '/api/v1/domains/1/dns/records': json({ records: [{ id: 1, name: 'example.com', type: 'A', content: '192.0.2.4', ttl: 3600, disabled: false }] }),
  // What the handler writes (cmd/panel/dnssec_handlers.go): `secured` and `ds`.
  // This was `{ enabled: false }`, which the panel read as "not signed"; since
  // the fourth batch an answer without `secured` is "could not check".
  '/api/v1/domains/1/dnssec': json({ secured: false, ds: null }),
  '/api/v1/domains/1/php': json({ domain_id: 1, domain_name: 'example.com', php_version: '8.3', pool_name: 'example', pool_config: { pm: 'dynamic' } }),
  '/api/v1/domains/1/hosting': json({ project_type: 'static' }),
  '/api/v1/runtimes/node': json({ installed: [] }),
  // The second batch.
  '/api/v1/auth/2fa/status': json({ enabled: true }),
  // My account's email card (alpha.99).
  '/api/v1/auth/me': json({ username: 'ada', role: 'admin', effective_role: 'admin', account_type: 'account', email: 'ada@example.com', impersonating: false, features: {} }),
  '/api/v1/panel/certificate': json({ https_enabled: true, self_signed: false, issuer: 'R11', expires_at: '2027-01-01T00:00:00Z' }),
  '/api/v1/users': json({ users: [{ id: 7, username: 'ada', email: 'ada@example.com', role: 'customer', status: 'active', subscriptions: 1, domains: 1, created_at: '' }] }),
  '/api/v1/plans': json({ plans: [{ id: 2, name: 'Basic', max_domains: 5, max_databases: 5, max_email_accounts: 5, disk_quota_mb: 1024, bandwidth_quota_mb: 1024 }] }),
  '/api/v1/domains/1/files': json({ files: [{ name: 'index.php', path: '/index.php', is_dir: false, size: 12, permissions: '-rw-r--r--', mod_time: '2026-10-01T00:00:00Z' }] }),
  '/api/v1/domains/1/ssl': json({
    domain_id: 1, domain_name: 'example.com', has_certificate: true, managed_names: ['example.com'],
    settings: { force_https: true, hsts_enabled: false, hsts_max_age: 300 },
    certificate: {
      id: 4, type: 'letsencrypt', provider_id: 'letsencrypt', issuer: 'R11', subject: 'example.com', issued_at: '2026-09-01T00:00:00Z',
      expires_at: '2026-12-01T00:00:00Z', days_until_expiry: 60, auto_renew: true, renewal_status: 'ok', status: 'active',
      dns_names: ['example.com'], activated: true, usable: true, trust_status: 'trusted', activation_pending: false, dependents_pending: false,
    },
  }),
  '/api/v1/domains/1/ssl/mail': json({ secure_mail: false }),
  '/api/v1/domains/1/usage': json({ disk_usage: 1024, bandwidth: 2048 }),
  '/api/v1/ssl/providers': json({ providers: [{ id: 'letsencrypt', name: 'Let’s Encrypt', note: '', needs_eab: false }] }),
  '/api/v1/metrics/history': json({ samples: [1, 2, 3].map((n) => ({ ts: `2026-10-09T00:0${n}:00Z`, cpu: 10 * n, mem_used: n, mem_total: 8, disk_used: n, disk_total: 80, load1: n / 10 })) }),
  '/api/v1/managed-services': json({ services: [{ id: 'redis', name: 'Redis', kind: 'service', is_installed: true, status: 'active (running)', unit: 'redis-server', versions: [], ports: ['6379/tcp'], config_files: [{ path: '/etc/redis/redis.conf', is_managed: false }] }] }),
  '/api/v1/service/logs': json({ lines: ['Ready to accept connections'] }),
  '/api/v1/panel/license': json({ state: 'active', can_provision: true }),
};

// --- The ways a read can fail to produce a known answer -----------------------
const failures = {
  'still on its way': () => new Promise(() => {}),
  'dropped connection': () => Promise.reject(new TypeError('fetch failed')),
  'refused by the server': json({ error: 'server sentence', code: 'INTERNAL' }, 502),
  'an answer that is not the contract': json('not the contract'),
};

const originalFetch = globalThis.fetch;
let tree, calls;
// `changes` answers what a screen sends (anything but GET), by path; a change
// with no entry is accepted. The browser's own objects a screen touches are
// the least that lets it run: an address, and a storage that keeps what is put
// in it. An attempt to move the page is recorded, never followed.
function serve(overrides = {}, role = 'admin', changes = {}) {
  calls = [];
  globalThis.currentTest = { toasts: [], navigated: [], navigatedTo: [], role };
  globalThis.confirm = () => true;
  globalThis.document = { addEventListener() {}, removeEventListener() {} };
  const stored = new Map();
  globalThis.localStorage = {
    getItem: (key) => (stored.has(key) ? stored.get(key) : null),
    setItem: (key, value) => { stored.set(key, String(value)); },
    removeItem: (key) => { stored.delete(key); },
  };
  const moved = globalThis.currentTest.navigatedTo;
  globalThis.window = {
    location: {
      hostname: 'panel.example.com', port: '2083', origin: 'http://panel.example.com:2083',
      set href(address) { moved.push(address); },
      assign(address) { moved.push(address); },
    },
    open() {},
  };
  const table = { ...healthy, ...overrides };
  globalThis.fetch = (url, options = {}) => {
    const method = options.method || 'GET';
    calls.push({ method, url, body: options.body });
    if (method !== 'GET') {
      const change = changes[url.split('?')[0]];
      return Promise.resolve().then(() => (change ? change(options) : Response.json({ success: true })));
    }
    const answer = table[url.split('?')[0]];
    if (!answer) return Promise.resolve(Response.json({ error: 'no such route in this test' }, { status: 404 }));
    return Promise.resolve().then(() => answer({ url }));
  };
}
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(Component, props = {}) {
  await act(async () => {
    tree = Renderer.create(React.createElement(Component, props));
    await settled();
    await settled();
  });
}
async function cleanup() {
  if (tree) await act(async () => tree.unmount());
  tree = undefined;
  globalThis.fetch = originalFetch;
  delete globalThis.currentTest;
  delete globalThis.document;
  delete globalThis.localStorage;
  delete globalThis.window;
}
const text = () => JSON.stringify(tree.toJSON());
const has = (key) => text().includes(`"${key}"`) || text().includes(key);
const hostButtons = () => tree.root.findAll((node) => node.type === 'button');
const labelOf = (node) => [node.props['aria-label'], node.props.title, ...[].concat(node.props.children ?? [])]
  .filter((part) => typeof part === 'string').join(' ');
const buttons = (label) => hostButtons().filter((node) => labelOf(node).includes(label));
const press = async (node) => {
  await act(async () => {
    await node.props.onClick({});
    await settled();
    await settled();
  });
};
const reads = (path) => calls.filter((call) => call.method === 'GET' && call.url.split('?')[0] === path).length;

// A control that submits a form or removes something. None of these may be
// enabled while what it acts on is not known: none at all on a screen none of
// whose reads has answered, and none that depends on the one read withheld.
const destructiveWords = /delete|remove|\bsil\b|kaldır|rotate/i;
const submits = (node) => node.props.type === 'submit';
const removes = (node) => destructiveWords.test(labelOf(node)) || /text-danger|hover:text-danger/.test(node.props.className ?? '');
const enabled = (which) => hostButtons().filter((node) => !node.props.disabled && which(node)).map(labelOf);
const enabledDangerousControls = () => enabled((node) => submits(node) || removes(node));

// --- The table -----------------------------------------------------------------
// screen:   what is mounted, with which props and role
// read:     the address whose answer is withheld
// negative: text keys that claim something about the server; none may be drawn
// checking / unknown: what the screen says instead ('' = it says nothing itself)
// known:    an answer that makes one of the negative keys true, and that key
const domain = { domainId: 1, domainName: 'example.com' };
const table = [
  {
    screen: 'Add domain dialogue', Component: AddDomainModal, props: { onClose() {}, onSuccess() {} },
    read: '/api/v1/hosting/capabilities',
    negative: ['domains.add.needsDns', 'err.DNS_SETTINGS_REQUIRED', 'err.DNS_SERVER_REQUIRED.action', 'err.DNS_SETTINGS_REQUIRED.action', 'domains.add.needsWebServer'],
    checking: 'dns.checkingServer', unknown: 'domains.add.dnsUnknown',
    known: [json(caps({ dns_server: '', dns_identity_ready: false, dns_management_ready: false })), 'domains.add.needsDns'],
    alsoDisabled: 'domains.add.create',
  },
  {
    screen: 'Domains page (DNS readiness, with domains)', Component: Domains,
    read: '/api/v1/hosting/capabilities',
    negative: ['domains.add.needsDns', 'err.DNS_SETTINGS_REQUIRED', 'err.DNS_SERVER_REQUIRED.action', 'err.DNS_SETTINGS_REQUIRED.action'],
    checking: '', unknown: '',
    known: [json(caps({ dns_server: 'bind', dns_identity_ready: false, dns_management_ready: false })), 'err.DNS_SETTINGS_REQUIRED'],
  },
  {
    screen: 'Domains page (DNS readiness, no domains yet)', Component: Domains,
    with: { '/api/v1/domains': json([]) },
    read: '/api/v1/hosting/capabilities',
    negative: ['domains.add.needsDns', 'err.DNS_SETTINGS_REQUIRED', 'err.DNS_SERVER_REQUIRED.action', 'err.DNS_SETTINGS_REQUIRED.action'],
    checking: '', unknown: '',
    known: [json(caps({ dns_server: '', dns_identity_ready: false, dns_management_ready: false })), 'err.DNS_SERVER_REQUIRED.action'],
  },
  {
    screen: 'Domains page (the list)', Component: Domains,
    read: '/api/v1/domains',
    negative: ['domains.empty', 'domains.emptyHint'],
    checking: 'domains.checking', unknown: 'domains.unknown',
    known: [json([]), 'domains.empty'],
  },
  {
    screen: 'Domains page (subscription usage)', Component: Domains,
    read: '/api/v1/subscriptions',
    negative: ['quota.unlimited'],
    checking: '', unknown: 'quota.unknown',
    known: [json({ subscriptions: [{ id: 1, name: 'Main', owner: 'admin', usage: { disk_used_bytes: 1, disk_limit_bytes: 0, domains: 1, domains_limit: 1, databases: 0, databases_limit: 1, mail_accounts: 0, mail_limit: 1 } }] }), 'quota.unlimited'],
  },
  {
    screen: 'Databases page (engines)', Component: DatabaseManagementV2,
    read: '/api/v1/database-servers',
    negative: ['databases.noServers', 'databases.noServersHint', 'domains.goServices', 'databases.account.missing'],
    checking: 'databases.checkingServers', unknown: 'databases.serversUnknown',
    known: [json([]), 'databases.noServers'],
  },
  {
    screen: 'Databases page (databases of an engine)', Component: DatabaseManagementV2,
    read: '/api/v1/database-servers/1/databases',
    negative: ['databases.empty.databases', 'databases.empty.databasesHint'],
    checking: 'databases.checkingDatabases', unknown: 'databases.databasesUnknown',
    known: [json([]), 'databases.empty.databases'],
  },
  {
    screen: 'Databases page (users of an engine)', Component: DatabaseManagementV2,
    read: '/api/v1/database-servers/1/users',
    // The users tab is not open, so its own lines are not on screen; what must
    // hold is that "Add database", whose dialogue offers the users, is off.
    negative: ['databases.empty.users'],
    checking: '', unknown: '',
    known: null,
    alsoDisabled: 'databases.addDatabase',
  },
  {
    screen: 'Domain databases (the list)', Component: DomainDatabaseManager, props: domain,
    read: '/api/v1/domains/1/databases',
    negative: ['databases.empty.databases', 'No databases created yet', 'db.teamEngineUnavailable'],
    checking: 'db.checking', unknown: 'db.unknown',
    known: [json({ databases: [], available_types: ['mysql'] }), 'databases.empty.databases'],
  },
  {
    screen: 'Domain databases (engines, administrator)', Component: DomainDatabaseManager, props: domain,
    read: '/api/v1/hosting/capabilities',
    negative: ['databases.noServersHint', 'db.teamEngineUnavailable', 'dbtools.install'],
    checking: 'db.checkingEngines', unknown: 'db.enginesUnknown',
    known: [json(caps({ database_servers: [] })), 'databases.noServersHint'],
    alsoDisabled: 'Create Database',
  },
  {
    screen: 'Domain databases (team member)', Component: DomainDatabaseManager, props: { ...domain, isAdditionalUser: true }, role: 'additional_user',
    read: '/api/v1/domains/1/databases',
    negative: ['db.teamEngineUnavailable', 'databases.empty.databases'],
    checking: 'db.checking', unknown: 'db.unknown',
    known: [json({ databases: [], available_types: [] }), 'db.teamEngineUnavailable'],
    alsoDisabled: 'Create Database',
  },
  {
    screen: 'Domain connection card', Component: DomainConnection, props: domain,
    read: '/api/v1/domains/1/connection',
    negative: ['conn.intro', 'conn.sslBlocked', 'conn.none', 'conn.status.unresolved', 'conn.status.elsewhere', 'conn.nsBroken.title'],
    checking: 'conn.checking', unknown: 'conn.readFailed',
    known: [json(connection({ status: 'unresolved', live_nameservers: [], live_ips: [], ssl_ready: false })), 'conn.intro'],
  },
  {
    screen: 'Domain DNS records (is the zone served)', Component: DomainDNSManager, props: domain,
    read: '/api/v1/hosting/capabilities',
    negative: ['dns.notServed'],
    checking: 'dns.checkingServer', unknown: 'server sentence|dns.statusUnavailable',
    known: [json(caps({ dns_server: '', dns_identity_ready: false })), 'dns.notServed'],
  },
  {
    screen: 'Domain PHP settings (installed versions)', Component: DomainPHPSettings, props: { ...domain, currentVersion: '8.3', onVersionChange() {} },
    read: '/api/v1/hosting/capabilities',
    negative: [],
    checking: 'php.checkingVersions', unknown: 'php.versionsUnknown',
    known: null,
  },
  {
    screen: 'Hosting type (is PHP installed)', Component: HostingTypePanel, props: domain,
    read: '/api/v1/hosting/capabilities',
    negative: ['hosting.phpMissing'],
    checking: '', unknown: 'hosting.phpUnknown',
    known: [json(caps({ php_versions: [] })), 'hosting.phpMissing'],
  },
  // --- The second batch ------------------------------------------------------
  {
    screen: 'My account (two-factor sign-in)', Component: AccountPage,
    read: '/api/v1/auth/2fa/status',
    // "Off" is the setup form; before this it was offered on a failed read.
    negative: ['settings.2fa.reauthHint', 'settings.2fa.setup'],
    checking: 'settings.2fa.checking', unknown: 'settings.2fa.unknown',
    known: [json({ enabled: false }), 'settings.2fa.setup'],
  },
  {
    screen: 'Settings (the certificate the Panel serves)', Component: Settings,
    read: '/api/v1/panel/certificate',
    negative: ['panelCert.selfSigned', 'panelCert.notReadable'],
    checking: 'panelCert.checking', unknown: 'panelCert.unknown',
    known: [json({ https_enabled: true, self_signed: true }), 'panelCert.selfSigned'],
    alsoDisabled: 'panelCert.issue',
  },
  {
    screen: 'Accounts (the list)', Component: UsersPage,
    read: '/api/v1/users',
    negative: ['users.empty', 'users.emptyHint'],
    checking: 'users.checking', unknown: 'users.unknown',
    known: [json({ users: [] }), 'users.empty'],
    alsoDisabled: 'users.add',
  },
  {
    screen: 'Accounts (the plans an account is created with)', Component: UsersPage,
    read: '/api/v1/plans',
    negative: ['users.form.noPlan'],
    checking: '', unknown: 'users.plansUnknown',
    known: null,
    alsoDisabled: 'users.add',
  },
  {
    screen: 'Domain files (a folder)', Component: DomainFileManager, props: domain,
    read: '/api/v1/domains/1/files',
    negative: ['files.empty'],
    checking: 'files.checking', unknown: 'files.unknown',
    known: [json({ files: [] }), 'files.empty'],
    alsoDisabled: 'files.newFolder',
  },
  {
    screen: 'Domain certificate', Component: DomainSSLSettings, props: { ...domain, mailAvailable: true },
    read: '/api/v1/domains/1/ssl',
    negative: ['ssl.noCert', 'ssl.status.none', 'ssl.httpsCertificateRequired'],
    checking: 'ssl.checking', unknown: 'ssl.unknown',
    known: [json({ domain_id: 1, domain_name: 'example.com', has_certificate: false, settings: { force_https: false, hsts_enabled: false, hsts_max_age: 300 } }), 'ssl.noCert'],
  },
  {
    screen: 'Domain certificate (the authorities a certificate can be requested from)', Component: DomainSSLSettings, props: { ...domain, mailAvailable: true },
    with: { '/api/v1/domains/1/ssl': json({ domain_id: 1, domain_name: 'example.com', has_certificate: false, settings: { force_https: false, hsts_enabled: false, hsts_max_age: 300 } }) },
    read: '/api/v1/ssl/providers',
    negative: [],
    checking: 'ssl.checkingProviders', unknown: 'ssl.providersUnknown',
    known: null,
    alsoDisabled: 'ssl.issue',
  },
  {
    screen: 'Monitoring', Component: MonitoringPage,
    read: '/api/v1/metrics/history',
    negative: ['monitoring.empty'],
    checking: 'monitoring.checking', unknown: 'monitoring.unknown',
    known: [json({ samples: [] }), 'monitoring.empty'],
  },
  {
    screen: 'A component (its record)', Component: ComponentDetail, props: { serviceId: 'redis', onBack() {} },
    read: '/api/v1/managed-services',
    negative: ['component.configNone', 'component.portsNone', 'component.logsEmpty'],
    checking: 'component.checking', unknown: 'component.unknown',
    known: [json({ services: [{ id: 'redis', name: 'Redis', kind: 'service', is_installed: true, status: 'active (running)', unit: 'redis-server', versions: [] }] }), 'component.configNone'],
  },
  {
    screen: 'A component (its log)', Component: ComponentDetail, props: { serviceId: 'redis', onBack() {} },
    read: '/api/v1/service/logs',
    negative: ['component.logsEmpty'],
    checking: 'component.logsChecking', unknown: 'component.logsUnknown',
    known: [json({ lines: [] }), 'component.logsEmpty'],
  },
  {
    screen: 'A component page after a reload', Component: ServiceRecordLookup, props: { serviceId: 'redis', onBack() {}, children: () => 'component-page' },
    read: '/api/v1/managed-services',
    negative: ['component.absent'],
    checking: 'component.checking', unknown: 'component.unknown',
    known: [json({ services: [] }), 'component.absent'],
  },
  {
    screen: 'A domain page (looked up by name)', Component: DomainDetailByName, props: { domainName: 'example.com', onBack() {} },
    read: '/api/v1/domains',
    negative: ['domain.absent', 'domain.noAccess'],
    checking: 'domain.checking', unknown: 'domain.unknown',
    known: [json([]), 'domain.absent'],
  },
];

for (const row of table) {
  for (const [failure, answer] of Object.entries(failures)) {
    test(`${row.screen}: ${failure} → nothing negative, nothing that depends on it enabled`, async () => {
      serve({ ...row.with, [row.read]: answer }, row.role);
      try {
        await mount(row.Component, row.props);
        for (const key of row.negative) assert.ok(!has(key), `${key} is drawn although ${row.read} is not known`);
        if (row.alsoDisabled) {
          const controls = buttons(row.alsoDisabled);
          assert.ok(controls.length > 0, `${row.alsoDisabled} is not drawn`);
          assert.ok(controls.every((node) => node.props.disabled), `${row.alsoDisabled} is enabled`);
        }
        const pending = failure === 'still on its way';
        const expected = pending ? row.checking : row.unknown;
        if (expected) assert.match(text(), new RegExp(expected.replaceAll('.', '\\.')), `the screen does not say "${expected}"`);
        if (pending) {
          if (row.unknown) assert.doesNotMatch(text(), new RegExp(row.unknown.replaceAll('.', '\\.')), 'a read that has not failed is shown as failed');
          assert.equal(buttons('common.retry').length, 0, 'Retry is offered for a read that has not failed');
        } else if (row.unknown) {
          assert.ok(buttons('common.retry').length >= 1, 'a failed read offers no Retry');
          if (row.checking) assert.doesNotMatch(text(), new RegExp(row.checking.replaceAll('.', '\\.')), 'a failed read is still shown as checking');
        }
        assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'the screen changed something');
      } finally {
        await cleanup();
      }
    });
  }

  if (row.known) {
    test(`${row.screen}: the negative text does appear once the server has said so`, async () => {
      serve({ ...row.with, [row.read]: row.known[0] }, row.role);
      try {
        await mount(row.Component, row.props);
        assert.ok(has(row.known[1]), `${row.known[1]} is not drawn for a known negative answer`);
        assert.equal(buttons('common.retry').length, 0);
      } finally {
        await cleanup();
      }
    });
  }

  if (row.unknown) test(`${row.screen}: Retry reads again and the answer replaces the notice`, async () => {
    let attempt = 0;
    serve({ ...row.with, [row.read]: (request) => (attempt++ === 0 ? failures['refused by the server']() : healthy[row.read](request)) }, row.role);
    try {
      await mount(row.Component, row.props);
      const before = reads(row.read);
      assert.equal(buttons('common.retry').length, 1);
      await press(buttons('common.retry')[0]);
      assert.equal(reads(row.read), before + 1, 'Retry did not read exactly once');
      assert.equal(buttons('common.retry').length, 0, 'the notice stayed after a good answer');
      assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'Retry changed something');
    } finally {
      await cleanup();
    }
  });
}

// --- Every read of a screen withheld at once ------------------------------------
// The whole screen, with nothing answered or with nothing reachable: no claim
// about the server anywhere on it, and not one enabled control that submits or
// removes.
const mounts = new Map();
for (const row of table) {
  const key = `${row.Component.name}/${row.role ?? 'admin'}`;
  if (!mounts.has(key)) mounts.set(key, { ...row, negative: [] });
  mounts.get(key).negative.push(...row.negative);
}
for (const row of mounts.values()) {
  for (const failure of ['still on its way', 'dropped connection']) {
    test(`${row.Component.name} (${row.role ?? 'administrator'}): every read ${failure} → no negative text, no enabled submit or remove`, async () => {
      serve(Object.fromEntries(Object.keys(healthy).map((path) => [path, failures[failure]])), row.role);
      try {
        await mount(row.Component, row.props);
        for (const key of new Set(row.negative)) assert.ok(!has(key), `${key} is drawn although nothing is known`);
        assert.deepEqual(enabledDangerousControls(), [], 'a submitting or removing control is enabled');
        assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
      } finally {
        await cleanup();
      }
    });
  }
}

// --- The owner's incident, step by step ----------------------------------------

test('Add domain: checking shows the form and one quiet line; the blocker never appears on the way to a good answer', async () => {
  let release;
  serve({ '/api/v1/hosting/capabilities': () => new Promise((resolve) => { release = () => resolve(Response.json(caps())); }) });
  try {
    await mount(AddDomainModal, { onClose() {}, onSuccess() {} });
    const submit = () => tree.root.findAll((node) => node.type === 'button' && node.props.type === 'submit')[0];
    assert.ok(has('dns.checkingServer'));
    assert.ok(has('domains.add.domainName'), 'the form is not shown while checking');
    assert.equal(tree.root.findAllByProps({ type: 'text' })[0].props.disabled, undefined, 'the name cannot be typed while checking');
    assert.deepEqual(tree.root.findAllByProps({ name: 'purpose' }).map((radio) => radio.props.disabled), [false, false]);
    assert.equal(submit().props.disabled, true);
    assert.ok(!has('domains.add.needsDns') && !has('err.DNS_SERVER_REQUIRED.action'));

    await act(async () => { release(); await settled(); await settled(); });
    assert.ok(!has('dns.checkingServer'), 'the checking line stayed');
    assert.ok(has('domains.add.dnsServed'));
    assert.ok(!has('domains.add.needsDns') && !has('err.DNS_SERVER_REQUIRED.action'));
    assert.equal(submit().props.disabled, false);
  } finally {
    await cleanup();
  }
});

test('Add domain: a purpose chosen while checking is kept; the default follows a known answer only until then', async () => {
  let release;
  serve({ '/api/v1/hosting/capabilities': () => new Promise((resolve) => { release = (body) => resolve(Response.json(body)); }) });
  try {
    await mount(AddDomainModal, { onClose() {}, onSuccess() {} });
    const radios = () => tree.root.findAllByProps({ name: 'purpose' });
    assert.deepEqual(radios().map((radio) => radio.props.checked), [true, false]);
    await act(async () => { radios()[1].props.onChange(); await settled(); });
    await act(async () => { release(caps()); await settled(); await settled(); });
    assert.deepEqual(radios().map((radio) => radio.props.checked), [false, true], 'the choice made while checking was replaced');
  } finally {
    await cleanup();
  }

  serve({ '/api/v1/hosting/capabilities': json(caps({ web_server: '' })) });
  try {
    await mount(AddDomainModal, { onClose() {}, onSuccess() {} });
    const radios = tree.root.findAllByProps({ name: 'purpose' });
    assert.deepEqual(radios.map((radio) => radio.props.checked), [false, true], 'no web server is known: only the DNS zone is possible');
    assert.equal(radios[0].props.disabled, true);
    assert.ok(has('domains.add.needsWebServer'));
  } finally {
    await cleanup();
  }
});

test('Add domain: nothing is submitted while DNS is not known, even through the form itself', async () => {
  for (const answer of [failures['still on its way'], failures['dropped connection']]) {
    serve({ '/api/v1/hosting/capabilities': answer });
    try {
      await mount(AddDomainModal, { onClose() {}, onSuccess() {} });
      const form = tree.root.findAll((node) => node.type === 'form')[0];
      await act(async () => { await form.props.onSubmit({ preventDefault() {} }); await settled(); });
      assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
    } finally {
      await cleanup();
    }
  }
});

test('the dialogue uses the answer the page already has: one request for both', async () => {
  serve();
  try {
    await mount(Domains);
    assert.equal(reads('/api/v1/hosting/capabilities'), 1);
    await press(buttons('domains.add')[0]);
    assert.ok(has('domains.add.title'), 'the dialogue did not open');
    assert.equal(reads('/api/v1/hosting/capabilities'), 1, 'the dialogue asked again for what the page has');
    assert.ok(!has('dns.checkingServer'), 'the dialogue showed a checking line for an answer the page has');
    assert.ok(has('domains.add.dnsServed'));
    assert.deepEqual(tree.root.findAll((node) => node.type === 'button' && node.props.type === 'submit').map((node) => node.props.disabled), [false]);
  } finally {
    await cleanup();
  }
});

test('two panels mounted together cause one request; a later visit reads again', async () => {
  serve();
  try {
    await act(async () => {
      tree = Renderer.create(React.createElement(React.Fragment, null,
        React.createElement(DomainDatabaseManager, domain),
        React.createElement(HostingTypePanel, domain),
        React.createElement(DomainPHPSettings, { ...domain, currentVersion: '8.3', onVersionChange() {} })));
      await settled();
      await settled();
    });
    assert.equal(reads('/api/v1/hosting/capabilities'), 1);
    await act(async () => tree.unmount());
    tree = undefined;
    await mount(HostingTypePanel, domain);
    assert.equal(reads('/api/v1/hosting/capabilities'), 2, 'an answer nobody was showing any more was reused');
  } finally {
    await cleanup();
  }
});

test('a team member is never asked for the server-wide inventory', async () => {
  for (const [Component, props] of [
    [Domains, {}],
    [DomainDatabaseManager, { ...domain, isAdditionalUser: true }],
    [DomainDNSManager, { ...domain, isAdditionalUser: true }],
    [DomainPHPSettings, { ...domain, currentVersion: '8.3', onVersionChange() {}, isAdditionalUser: true }],
  ]) {
    serve({ '/api/v1/domains': json([{ ...domainRow, access: { dns: 'manage' } }]) }, 'additional_user');
    try {
      await mount(Component, props);
      assert.equal(reads('/api/v1/hosting/capabilities'), 0, `${Component.name} read the capabilities for a team member`);
      for (const key of ['dns.checkingServer', 'php.checkingVersions', 'db.checkingEngines']) assert.ok(!has(key), `${Component.name} waits for a read it never makes`);
    } finally {
      await cleanup();
    }
  }
});

// --- A list that was known, then could not be read again -----------------------

test('Domains: after a failed refresh the earlier list stays, marked, and nothing that removes a domain is enabled', async () => {
  let fail = false;
  serve({ '/api/v1/domains': (request) => (fail ? failures['refused by the server']() : healthy['/api/v1/domains'](request)) });
  try {
    await mount(Domains);
    const remove = () => buttons('domains.action.delete');
    assert.equal(remove().length, 1);
    assert.equal(Boolean(remove()[0].props.disabled), false);
    fail = true;
    globalThis.confirm = () => true;
    await press(remove()[0]);
    assert.ok(has('example.com'), 'the earlier list was dropped');
    assert.ok(has('common.staleNotice'), 'the earlier list is not marked as the earlier list');
    assert.ok(!has('domains.empty'));
    assert.equal(remove()[0].props.disabled, true, 'a domain can be removed from a list that is not current');
    assert.deepEqual(enabledDangerousControls(), []);
    assert.equal(buttons('common.retry').length, 1);
    fail = false;
    await press(buttons('common.retry')[0]);
    assert.ok(!has('common.staleNotice'));
    assert.equal(Boolean(remove()[0].props.disabled), false);
  } finally {
    await cleanup();
  }
});

test('Domains: a pending row whose saved deletion could not be read says exactly that', async () => {
  const pendingRow = { ...domainRow, id: 9, domain_name: 'leaving.example', status: 'pending' };
  serve({ '/api/v1/domains': json([pendingRow]), '/api/v1/domains/9/deletion-status': failures['refused by the server'] });
  try {
    await mount(Domains);
    assert.ok(has('domains.pendingUnknown'));
    assert.ok(!has('domains.deletionWaiting'), 'an unread marker is announced as a waiting deletion');
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/domains': json([pendingRow]), '/api/v1/domains/9/deletion-status': () => new Response(null, { status: 204 }) });
  try {
    await mount(Domains);
    assert.ok(!has('domains.pendingUnknown') && !has('domains.deletionWaiting'), 'the server said there is no marker');
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/domains': json([pendingRow]), '/api/v1/domains/9/deletion-status': json({ status: 'deletion_pending', stage: 'dns_cleanup', reason: 'dns_peer_proof_timeout' }) });
  try {
    await mount(Domains);
    // The stand-in catalogue has no sentence for the reason, so the screen
    // falls back to its generic waiting text, as it does for an unreviewed one.
    assert.ok(has('domains.deletionWaiting') && has('domains.deletionPending') && !has('domains.pendingUnknown'));
  } finally {
    await cleanup();
  }
});

// --- Databases: the account strip follows the latest answer ---------------------

test('Databases: after the panel account is removed the strip shows the new answer, not the row it was mounted with', async () => {
  let account = 'celikpanel';
  serve({ '/api/v1/database-servers': () => Response.json([{ ...engine, admin_username: account }]) });
  try {
    await mount(DatabaseManagementV2);
    assert.ok(has('databases.account.present'));
    assert.equal(buttons('databases.account.rotate').length, 1);
    account = '';
    await press(buttons('databases.account.remove')[0]);
    assert.equal(calls.filter((call) => call.method === 'DELETE' && call.url.endsWith('/admin-account')).length, 1);
    assert.ok(!has('databases.account.present'), 'the strip still shows the account that was just removed');
    assert.equal(buttons('databases.account.rotate').length, 0, '"New password" is still offered for a removed account');
    assert.ok(has('databases.account.missing'));
  } finally {
    await cleanup();
  }
});

test('Databases: while the engine list is being read again, or could not be, the account controls are off', async () => {
  let mode = 'ok';
  let release;
  serve({ '/api/v1/database-servers': () => {
    if (mode === 'hang') return new Promise((resolve) => { release = () => resolve(Response.json([engine])); });
    if (mode === 'fail') return failures['refused by the server']();
    return Response.json([engine]);
  } });
  try {
    await mount(DatabaseManagementV2);
    const account = () => ['databases.account.show', 'databases.account.rotate', 'databases.account.remove'].flatMap(buttons);
    assert.deepEqual(account().map((node) => Boolean(node.props.disabled)), [false, false, false]);
    mode = 'hang';
    await press(buttons('databases.account.rotate')[0]);
    assert.equal(calls.filter((call) => call.method === 'POST').length, 1);
    assert.deepEqual(account().map((node) => Boolean(node.props.disabled)), [true, true, true], 'the row is being read again');
    await act(async () => { release(); await settled(); await settled(); });
    assert.deepEqual(account().map((node) => Boolean(node.props.disabled)), [false, false, false], 'the row is current again');
    mode = 'fail';
    await press(buttons('databases.account.rotate')[0]);
    assert.equal(calls.filter((call) => call.method === 'POST').length, 2);
    assert.ok(has('common.staleNotice'));
    assert.deepEqual(account().map((node) => Boolean(node.props.disabled)), [true, true, true], 'the row could not be read again');
    assert.deepEqual(enabledDangerousControls().filter((label) => label.includes('account')), []);
  } finally {
    await cleanup();
  }
});

test('Databases: a list that could not be read again keeps its rows and withdraws delete; counts are never invented', async () => {
  let fail = false;
  serve({ '/api/v1/database-servers/1/databases': (request) => (fail ? failures['dropped connection']() : healthy['/api/v1/database-servers/1/databases'](request)) });
  try {
    await mount(DatabaseManagementV2);
    assert.ok(has('shop'));
    const remove = () => buttons('databases.deleteDatabase');
    assert.equal(Boolean(remove()[0].props.disabled), false);
    fail = true;
    await press(remove()[0]);
    assert.ok(has('shop') && has('common.staleNotice'));
    assert.equal(remove()[0].props.disabled, true);
    assert.ok(buttons('databases.addDatabase').every((node) => node.props.disabled));
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/database-servers/1/databases': failures['still on its way'], '/api/v1/database-servers/1/users': failures['dropped connection'] });
  try {
    await mount(DatabaseManagementV2);
    const badges = tree.root.findAll((node) => node.type === 'span' && /min-w-\[1\.5rem\]/.test(node.props.className ?? '')).map((node) => node.props.children);
    assert.deepEqual(badges, ['…', '–'], 'a count is shown for a list that is not known');
    assert.ok(!has('common.itemsTotal'));
  } finally {
    await cleanup();
  }
});

// --- Domain databases: no default engine ----------------------------------------

test('Domain databases: nothing is created on an engine the server did not name', async () => {
  for (const answer of [failures['still on its way'], failures['refused by the server'], json(caps({ database_servers: [] }))]) {
    serve({ '/api/v1/hosting/capabilities': answer });
    try {
      await mount(DomainDatabaseManager, domain);
      assert.equal(tree.root.findAll((node) => node.type === 'form').length, 0, 'the create form is shown without a known engine');
      for (const node of buttons('Create Database')) assert.equal(node.props.disabled, true);
      assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
    } finally {
      await cleanup();
    }
  }
  serve();
  try {
    await mount(DomainDatabaseManager, domain);
    await press(buttons('Create Database')[0]);
    assert.deepEqual(tree.root.findAll((node) => node.type === 'option').map((node) => node.props.value), ['mysql']);
  } finally {
    await cleanup();
  }
});

// --- Domain connection: "unknown" is not "does not point here" -------------------

test('Domain connection: an answer of status unknown with null lists is drawn, and reads as not checked', async () => {
  serve({ '/api/v1/domains/1/connection': json(connection({
    status: 'unknown', ssl_ready: false, nameservers_usable: false,
    nameservers: null, live_nameservers: null, live_ips: null, resolver_observations: null, nameserver_facts: null,
  })) });
  try {
    await mount(DomainConnection, domain);
    assert.ok(has('conn.status.unknown') && has('conn.unknownHelp') && has('conn.notChecked') && has('conn.sslUnknown'));
    for (const key of ['conn.intro', 'conn.sslBlocked', 'conn.none', 'conn.nsBroken.title', 'conn.routeA.title', 'conn.status.unresolved']) {
      assert.ok(!has(key), `${key} is drawn for a domain whose public DNS could not be checked`);
    }
    assert.ok(has('conn.routeAUnknown') && has('conn.routeB.title'));
    assert.doesNotMatch(text(), /text-warning"|text-danger/);
  } finally {
    await cleanup();
  }
});

test('Domain connection: null lists never crash the card, whatever the status', async () => {
  for (const status of ['delegated', 'delegated_mismatch', 'a_record', 'elsewhere', 'unresolved', 'unknown']) {
    serve({ '/api/v1/domains/1/connection': json(connection({
      status, nameservers: null, live_nameservers: null, live_ips: null, glue_needed: true, propagation_pending: true,
      resolver_observations: [{ resolver: 'one', nameservers: null, ips: null, status, ssl_ready: false }],
      nameserver_facts: [{ host: 'ns1', ips: null, points_here: false }],
    })) });
    try {
      await mount(DomainConnection, domain);
      assert.ok(has('conn.title'), status);
    } finally {
      await cleanup();
    }
  }
  serve({ '/api/v1/domains/1/connection': json(connection({ status: 'somewhere_new' })) });
  try {
    await mount(DomainConnection, domain);
    assert.ok(has('conn.readFailed'), 'a status this card has no words for is guessed at');
  } finally {
    await cleanup();
  }
});

test('Domain connection: "Check again" keeps the card and reads once; a failed re-check keeps the earlier answer, marked', async () => {
  let fail = false;
  serve({ '/api/v1/domains/1/connection': (request) => (fail ? failures['dropped connection']() : healthy['/api/v1/domains/1/connection'](request)) });
  try {
    await mount(DomainConnection, domain);
    assert.ok(has('conn.okDelegated'));
    fail = true;
    await press(buttons('conn.recheck')[0]);
    assert.equal(reads('/api/v1/domains/1/connection'), 2);
    assert.ok(has('conn.okDelegated') && has('common.staleNotice'));
  } finally {
    await cleanup();
  }
});

// --- The second batch (9 Oct 2026): what each screen does around its own change ---

test('Settings: a certificate request from another tab is followed here, and this page is never moved for it', async () => {
  const requestID = 'c'.repeat(32);
  serve({
    '/api/v1/service/operation': json({ operation: { id: 'd'.repeat(32), request_id: requestID, kind: 'panel_certificate_issue', service_id: 'panel.example.com', status: 'succeeded' } }),
  });
  // The marker another tab left in storage.
  globalThis.localStorage.setItem('celikpanel.panel-certificate-operation.v1', JSON.stringify({ version: 1, request_id: requestID, domain: 'panel.example.com', created_at: Date.now() }));
  try {
    await mount(Settings);
    await act(async () => { await settled(); await settled(); });
    assert.ok(has('panelCert.reopen.title'), 'the issued certificate is not announced');
    assert.ok(has('panelCert.reopen.stayed') && has('panelCert.openSecure'), 'the secure address is not offered as a link');
    assert.ok(!has('panelCert.reopen.body') && buttons('panelCert.reopen.stay').length === 0, 'a countdown runs in a tab that did not ask');
    assert.deepEqual(globalThis.currentTest.navigatedTo, [], 'the page was moved');
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
  } finally {
    await cleanup();
  }
});

test('Settings: the page that asked says where it will reopen and why, and "Stay here" keeps it', async () => {
  let requestID = '';
  serve({
    '/api/v1/panel/certificate': json({ https_enabled: true, self_signed: true }),
    // The poll names the exact request; the answer is about that request.
    '/api/v1/service/operation': ({ url }) => Response.json({ operation: { id: 'd'.repeat(32), request_id: new URL(url, 'http://panel.test').searchParams.get('request_id'), kind: 'panel_certificate_issue', service_id: 'panel.example.com', status: 'succeeded' } }),
  }, 'admin', {
    '/api/v1/panel/certificate': (options) => {
      requestID = JSON.parse(options.body).request_id;
      return Response.json({ operation: { id: 'd'.repeat(32), request_id: requestID, kind: 'panel_certificate_issue', service_id: 'panel.example.com', status: 'queued' } });
    },
  });
  globalThis.currentTest.section = 'panel';
  try {
    await mount(Settings);
    await press(buttons('panelCert.issue')[0]);
    await act(async () => { await settled(); await settled(); });
    assert.equal(calls.filter((call) => call.method === 'POST').length, 1);
    assert.ok(has('panelCert.reopen.body'), 'the reason and the address are not said before the page moves');
    assert.equal(buttons('panelCert.reopen.stay').length, 1, '"Stay here" is not offered');
    await press(buttons('panelCert.reopen.stay')[0]);
    assert.ok(has('panelCert.reopen.stayed') && !has('panelCert.reopen.body'));
    assert.deepEqual(globalThis.currentTest.navigatedTo, [], 'the page was moved after "Stay here"');
  } finally {
    await cleanup();
  }
});

test('Settings: a certificate request whose poll gets no answer is never called failed, and no second request is offered', async () => {
  const requestID = 'c'.repeat(32);
  serve({ '/api/v1/service/operation': failures['dropped connection'] });
  globalThis.localStorage.setItem('celikpanel.panel-certificate-operation.v1', JSON.stringify({ version: 1, request_id: requestID, domain: 'panel.example.com', created_at: Date.now() }));
  try {
    await mount(Settings);
    await act(async () => { await settled(); await settled(); });
    assert.ok(!has('panelCert.failed') && !has('panelCert.failedDetail') && !has('panelCert.failedPlain'));
    assert.deepEqual(globalThis.currentTest.toasts.filter(([kind]) => kind === 'error'), []);
    assert.ok(buttons('panelCert.issu').every((node) => node.props.disabled), 'a second request is offered while the first is not known');
    assert.ok(globalThis.localStorage.getItem('celikpanel.panel-certificate-operation.v1'), 'the exact request was forgotten');
  } finally {
    await cleanup();
  }
});

test('Accounts: the plans tab has its own three states', async () => {
  for (const [failure, answer] of Object.entries(failures)) {
    serve({ '/api/v1/plans': answer });
    try {
      await mount(UsersPage);
      await press(buttons('users.tab.plans')[0]);
      assert.ok(!has('plans.empty'), `"no plans" is drawn (${failure})`);
      assert.ok(buttons('plans.add').every((node) => node.props.disabled), `a plan can be added (${failure})`);
      assert.ok(has(failure === 'still on its way' ? 'plans.checking' : 'plans.unknown'));
    } finally {
      await cleanup();
    }
  }
  serve({ '/api/v1/plans': json({ plans: [] }) });
  try {
    await mount(UsersPage);
    await press(buttons('users.tab.plans')[0]);
    assert.ok(has('plans.empty'));
  } finally {
    await cleanup();
  }
});

test('Accounts: a change whose answer is lost is not repeated; the list is read again', async () => {
  serve({}, 'admin', { '/api/v1/users/7': () => Promise.reject(new TypeError('fetch failed')) });
  try {
    await mount(UsersPage);
    const before = reads('/api/v1/users');
    await press(buttons('users.delete')[0]);
    assert.equal(calls.filter((call) => call.method === 'DELETE').length, 1, 'the change was sent again');
    assert.equal(reads('/api/v1/users'), before + 1, 'the list was not read again');
    assert.ok(globalThis.currentTest.toasts.some(([, text]) => text === 'common.resultUnknown'));
  } finally {
    await cleanup();
  }
});

test('Files: the rows of the folder just left are never shown under the new path', async () => {
  let release;
  serve({
    '/api/v1/domains/1/files': ({ url }) => (url.includes(encodeURIComponent('/logs'))
      ? new Promise((resolve) => { release = () => resolve(Response.json({ files: [] })); })
      : Response.json({ files: [{ name: 'logs', path: '/logs', is_dir: true, size: 0, permissions: 'drwxr-xr-x', mod_time: '2026-10-01T00:00:00Z' }, { name: 'index.php', path: '/index.php', is_dir: false, size: 12, permissions: '-rw-r--r--', mod_time: '2026-10-01T00:00:00Z' }] })),
  });
  try {
    await mount(DomainFileManager, domain);
    assert.ok(has('index.php'));
    await press(tree.root.findAll((node) => node.type === 'button' && [].concat(node.props.children ?? []).some((child) => child?.props?.children === 'logs'))[0]);
    assert.ok(!has('index.php'), 'the rows of / are drawn under /logs');
    assert.ok(has('files.checking') && !has('files.empty'));
    assert.ok(buttons('files.newFolder').every((node) => node.props.disabled));
    await act(async () => { release(); await settled(); await settled(); });
    assert.ok(has('files.empty'), 'an answered empty folder is not said to be empty');
  } finally {
    await cleanup();
  }
});

test('Domain certificate: after a successful request the screen shows the new answer; if that could not be read, the earlier one is marked and nothing is offered', async () => {
  const none = { domain_id: 1, domain_name: 'example.com', has_certificate: false, settings: { force_https: false, hsts_enabled: false, hsts_max_age: 300 } };
  for (const reread of ['answers', 'fails']) {
    let issued = false;
    serve({
      '/api/v1/domains/1/ssl': () => {
        if (!issued) return Response.json(none);
        return reread === 'answers' ? healthy['/api/v1/domains/1/ssl']() : failures['refused by the server']();
      },
    }, 'admin', { '/api/v1/domains/1/ssl/letsencrypt': () => { issued = true; return Response.json({ success: true }); } });
    try {
      await mount(DomainSSLSettings, { ...domain, mailAvailable: true });
      assert.ok(has('ssl.noCert'));
      await act(async () => { tree.root.findAllByProps({ type: 'email' })[0].props.onChange({ target: { value: 'owner@example.com' } }); await settled(); });
      await press(buttons('ssl.issue').find((node) => !node.props.disabled));
      assert.equal(calls.filter((call) => call.method === 'POST').length, 1);
      if (reread === 'answers') {
        assert.ok(!has('ssl.noCert'), '"No certificate" is still drawn after the certificate was issued');
        assert.ok(has('ssl.status.valid'));
      } else {
        assert.ok(has('common.staleNotice'), 'the earlier answer is not marked as the earlier answer');
        assert.deepEqual(enabledDangerousControls(), []);
        assert.ok(buttons('ssl.issue').every((node) => node.props.disabled), 'the request is offered again on an answer that is not the latest');
      }
    } finally {
      await cleanup();
    }
  }
});

test('Import: an apply whose answer is lost says the result is unknown, starts nothing again and offers a check that only reads', async () => {
  const preview = { username: 'old', main_domain: 'old.example', domains: ['old.example'], public_html: true, site_bytes: 10, mail_accounts: [], forwarders: [], dns_zones: {}, databases: [] };
  for (const [listed, expected] of [[[{ id: 9, domain_name: 'old.example' }], 'import.unknown.present'], [[], 'import.unknown.absent']]) {
    // The identity each apply request carried (D-029). The third arrival is
    // answered: the server found the first one's result under that identity.
    const identities = [];
    serve({ '/api/v1/domains': json(listed), '/api/v1/subscriptions': json({ subscriptions: [{ id: 3, name: 'Main', owner: 'admin' }] }) }, 'admin', {
      '/api/v1/import/cpanel/inspect': () => Response.json(preview),
      '/api/v1/import/cpanel/apply': (options) => {
        identities.push(new Headers(options.headers).get('X-CelikPanel-Request-Id'));
        return identities.length < 2 ? Promise.reject(new TypeError('fetch failed')) : Response.json({ steps: [{ step: 'domain', ok: true, detail: 'created' }] });
      },
    });
    try {
      await mount(ImportPage);
      await act(async () => { tree.root.findAllByProps({ placeholder: '/var/lib/celikpanel-imports/cpmove-user.tar.gz' })[0].props.onChange({ target: { value: '/var/lib/celikpanel-imports/a.tar.gz' } }); await settled(); });
      await press(buttons('import.inspect')[0]);
      await act(async () => { tree.root.findAll((node) => node.type === 'select')[1].props.onChange({ target: { value: '3' } }); await settled(); });
      await press(buttons('import.run')[0]);
      const applies = () => calls.filter((call) => call.url === '/api/v1/import/cpanel/apply').length;
      assert.equal(applies(), 1);
      assert.ok(has('import.unknown.title') && has('import.unknown.body'), 'a lost answer shows nothing');
      assert.equal(buttons('import.run').length, 0, '"Start import" came back by itself');
      assert.ok(tree.root.findAll((node) => node.type === 'select').every((node) => node.props.disabled), 'the choices that made the request can be changed');
      const before = calls.length;
      await press(buttons('import.unknown.check')[0]);
      assert.deepEqual(calls.slice(before).map((call) => [call.method, call.url.split('?')[0]]), [['GET', '/api/v1/domains']], 'the check did more than read the domain list');
      assert.ok(has(expected));
      assert.equal(applies(), 1, 'the import was sent again');
      assert.equal(buttons('import.runAgain').filter((node) => !node.props.disabled).length, expected === 'import.unknown.absent' ? 1 : 0);
      assert.match(identities[0], /^[0-9a-f]{32}$/, 'the apply request carried no identity');
      if (expected === 'import.unknown.absent') {
        // Starting again after the check is the same request under the same
        // identity, so the server answers it from the first run and cannot
        // import twice. Its answer is the result that is shown.
        await press(buttons('import.runAgain')[0]);
        const sent = calls.filter((call) => call.url === '/api/v1/import/cpanel/apply');
        assert.equal(sent.length, 2);
        assert.equal(identities[1], identities[0], 'the start after a lost answer was sent as a new request');
        assert.equal(sent[1].body, sent[0].body, 'the start after a lost answer sent a different request under the same identity');
        assert.ok(has('import.resultTitle'), 'the answer to the same request was not shown as the result');
        assert.ok(!has('import.unknown.title'));
      }
    } finally {
      await cleanup();
    }
  }
});

test('Dashboard license notice: it speaks only about an answer, and "could not be verified" is not "a license is required"', async () => {
  for (const answer of Object.values(failures)) {
    serve({ '/api/v1/panel/license': answer });
    try {
      await mount(LicenseNotice);
      assert.equal(tree.toJSON(), null, 'something is drawn about a license that is not known');
    } finally {
      await cleanup();
    }
  }
  for (const [state, drawn, absent] of [
    ['verification_unavailable', 'license.noticeUnverified', 'license.restricted'],
    ['status_unavailable', 'license.noticeUnverified', 'license.restricted'],
    ['missing', 'license.restricted', 'license.noticeUnverified'],
    ['expired', 'license.restricted', 'license.noticeUnverified'],
  ]) {
    serve({ '/api/v1/panel/license': json({ state, can_provision: false }) });
    try {
      await mount(LicenseNotice);
      assert.ok(has(drawn) && !has(absent), `${state} is drawn as ${absent}`);
    } finally {
      await cleanup();
    }
  }
  serve();
  try {
    await mount(LicenseNotice);
    assert.equal(tree.toJSON(), null);
  } finally {
    await cleanup();
  }
});

test('Monitoring: a poll that fails keeps the charts, marked, instead of "no samples yet"', async (t) => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  let polls = 0;
  serve({ '/api/v1/metrics/history': () => (polls++ === 0 ? healthy['/api/v1/metrics/history']() : failures['dropped connection']()) });
  try {
    await mount(MonitoringPage);
    assert.ok(has('monitoring.cpu') && !has('monitoring.empty'));
    await act(async () => { t.mock.timers.tick(60_000); await settled(); await settled(); });
    assert.equal(reads('/api/v1/metrics/history'), 2, 'the page did not read again after a minute');
    assert.ok(has('monitoring.cpu'), 'a failed poll wiped the charts');
    assert.ok(has('common.staleNotice') && !has('monitoring.empty'));
  } finally {
    await cleanup();
    t.mock.timers.reset();
  }
});

test('A domain page: neither a failed lookup nor an unknown name sends the person back to the list', async () => {
  for (const answer of [failures['refused by the server'], json([])]) {
    const back = [];
    serve({ '/api/v1/domains': answer });
    try {
      await mount(DomainDetailByName, { domainName: 'example.com', onBack: () => back.push(1) });
      assert.deepEqual([back, globalThis.currentTest.navigated], [[], []], 'the page left its address');
      assert.ok(buttons('nav.domains').length >= 1, 'the way back is not offered as a choice');
    } finally {
      await cleanup();
    }
  }
});

// The certificate line of the strip under a domain's name (10 Oct 2026). It
// waited for the overview card or the SSL/TLS tab to report, so after a read
// that failed, and on every tab that mounts neither, it said "checking status"
// without end. It reads the certificate itself now, at their address.
test('A domain page: the certificate line under the title is being checked, could not be checked (with the read again), or what the server said', async () => {
  const fact = (state) => tree.root.findAll((node) => node.props['data-ssl-fact'] === state);
  const noCertificate = json({ domain_id: 1, domain_name: 'example.com', has_certificate: false, settings: { force_https: false, hsts_enabled: false, hsts_max_age: 300 } });
  // On the overview, where the card reads the same address, and on the DNS
  // tab, where nothing else on the page reads it.
  for (const search of [undefined, 'tab=dns']) {
    const open = async (overrides) => {
      serve(overrides);
      if (search) globalThis.currentTest.search = search;
      await mount(DomainDetailByName, { domainName: 'example.com', onBack() {} });
    };
    const where = search ?? 'overview';

    await open({ '/api/v1/domains/1/ssl': failures['still on its way'] });
    try {
      assert.equal(fact('checking').length, 1, `${where}: the line does not say it is checking`);
      assert.equal(fact('unknown').length, 0);
      for (const key of ['domain.info.on', 'domain.info.off', 'domain.info.sslIssue', 'domain.info.sslUnknown']) assert.ok(!has(key), `${where}: ${key} before the answer`);
    } finally { await cleanup(); }

    for (const failure of ['dropped connection', 'refused by the server', 'an answer that is not the contract']) {
      let fail = true;
      await open({ '/api/v1/domains/1/ssl': (request) => (fail ? failures[failure]() : healthy['/api/v1/domains/1/ssl'](request)) });
      try {
        assert.equal(fact('checking').length, 0, `${where}, ${failure}: the line still says it is checking after the read failed`);
        assert.equal(fact('unknown').length, 1, `${where}, ${failure}`);
        assert.ok(has('domain.info.sslUnknown'));
        for (const key of ['domain.info.on', 'domain.info.off', 'domain.info.sslIssue']) assert.ok(!has(key), `${where}, ${failure}: ${key} for a certificate that was not read`);
        assert.equal(reads('/api/v1/domains/1/ssl'), 1, `${where}: the certificate was asked for more than once`);
        // The read again is beside the words, and it only reads.
        fail = false;
        await press(fact('unknown')[0].findByType('button'));
        assert.equal(reads('/api/v1/domains/1/ssl'), 2);
        assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'Retry changed something');
        assert.equal(fact('unknown').length + fact('checking').length, 0);
        assert.ok(has('domain.info.on'));
      } finally { await cleanup(); }
    }

    await open({ '/api/v1/domains/1/ssl': noCertificate });
    try {
      assert.ok(has('domain.info.off') && !has('domain.info.sslUnknown'), `${where}: a domain the server says has no certificate is not "off"`);
      assert.equal(fact('unknown').length + fact('checking').length, 0);
    } finally { await cleanup(); }
  }
});

// What the strip's read does to the SSL/TLS tab (10 Oct 2026, found in the
// browser run). The strip is on screen on every tab, so the certificate's
// answer is never dropped while the domain's page is open and the tab no
// longer starts from nothing. Opened while the page's first read is on its
// way, it shares that read. Opened later, it shows the answer the page has as
// the earlier answer, says it is reading again, keeps its controls off, and
// sends one more read.
test('A domain page: the SSL/TLS tab shares the page’s first read, and over an answer the page already has it reads once more with its controls off', async () => {
  const noCertificate = { domain_id: 1, domain_name: 'example.com', has_certificate: false, settings: { force_https: false, hsts_enabled: false, hsts_max_age: 300 } };
  const openTab = async () => {
    await press(buttons('domain.tab.hosting')[0]);
    await press(buttons('domain.sub.ssl')[0]);
  };
  const issue = () => buttons('ssl.issue').filter((node) => !node.props.disabled);
  const typeEmail = async () => {
    const field = tree.root.findAll((node) => node.type === 'input' && node.props.type === 'email')[0];
    assert.ok(field, 'the issue form has no address field');
    await act(async () => { field.props.onChange({ target: { value: 'owner@example.com' } }); await settled(); });
  };

  // The tab is opened before the page's read of the certificate has answered.
  let release = [];
  serve({ '/api/v1/domains/1/ssl': () => new Promise((resolve) => { release.push(() => resolve(Response.json(noCertificate))); }) });
  try {
    await mount(DomainDetailByName, { domainName: 'example.com', onBack() {} });
    await openTab();
    assert.ok(has('ssl.checking') && !has('ssl.noCert') && !has('ssl.rereading'), 'the tab does not show its first-read line while the shared read is on its way');
    assert.equal(reads('/api/v1/domains/1/ssl'), 1, 'the tab asked again for a read that is already on its way');
    await act(async () => { release.forEach((answer) => answer()); await settled(); await settled(); });
    assert.ok(has('ssl.noCert') && !has('ssl.checking'));
    assert.equal(reads('/api/v1/domains/1/ssl'), 1);
  } finally { await cleanup(); }

  // The tab is opened after the page has the answer: the read behind it is held open.
  release = [];
  let asked = 0;
  serve({ '/api/v1/domains/1/ssl': () => {
    asked += 1;
    if (asked === 1) return Response.json(noCertificate);
    return new Promise((resolve) => { release.push(() => resolve(Response.json(noCertificate))); });
  } });
  try {
    await mount(DomainDetailByName, { domainName: 'example.com', onBack() {} });
    assert.equal(reads('/api/v1/domains/1/ssl'), 1);
    await openTab();
    assert.ok(has('ssl.noCert'), 'the answer the page already has is not shown');
    assert.ok(has('ssl.rereading') && !has('ssl.checking'), 'the tab does not say that it is reading the earlier answer again');
    assert.equal(reads('/api/v1/domains/1/ssl'), 2, 'the tab did not read again, or read more than once');
    // With the address typed, the only thing that still holds the button is
    // the read behind the screen.
    await typeEmail();
    assert.equal(issue().length, 0, 'a certificate can be requested while the earlier answer is being read again');
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0);
    await act(async () => { release.forEach((answer) => answer()); await settled(); await settled(); });
    assert.ok(!has('ssl.rereading'), 'the re-read line stays after the answer');
    assert.equal(issue().length, 1, 'the request stays off after the read has answered');
    assert.equal(reads('/api/v1/domains/1/ssl'), 2);
  } finally { await cleanup(); }
});

// --- A component's page (ServiceShell) ---------------------------------------------
const record = (overrides = {}) => ({ id: 'redis', name: 'Redis', is_installed: true, status: 'active (running)', unit: 'redis-server', ...overrides });
const shell = { serviceId: 'redis', name: 'Redis', icon: () => null, onBack() {}, children: 'component-panels' };

test('A component page: a record that could not be read is not "not checked yet" and not "not installed"', async () => {
  for (const [failure, answer] of Object.entries(failures)) {
    serve({ '/api/v1/managed-services': answer });
    try {
      await mount(ServiceShell, shell);
      for (const key of ['services.notCheckedTitle', 'services.notChecked', 'svc.notInstalled', 'services.scanNow', 'services.running', 'services.stopped']) {
        assert.ok(!has(key), `${key} is drawn although the record is not known (${failure})`);
      }
      assert.ok(!has('component-panels'));
      assert.equal(buttons('services.st').length + buttons('services.restart').length, 0, 'start or stop is offered');
      if (failure === 'still on its way') {
        assert.ok(has('svc.checkingRecord'));
      } else {
        assert.ok(has('svc.recordUnknown') && has('svc.recordUnread'));
        const before = reads('/api/v1/managed-services');
        await press(buttons('common.retry')[0]);
        assert.equal(reads('/api/v1/managed-services'), before + 1);
        assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'Retry probed or changed the host');
      }
    } finally {
      await cleanup();
    }
  }
});

test('A component page: start and stop go to the unit the record names, and to nothing when it names none', async () => {
  serve({ '/api/v1/managed-services': json({ services: [record({ id: 'bind', unit: 'named' })] }) }, 'admin', { '/api/v1/service/action': () => Response.json({ success: true }) });
  try {
    await mount(ServiceShell, { ...shell, serviceId: 'bind' });
    await press(buttons('services.stop')[0]);
    const sent = calls.find((call) => call.url === '/api/v1/service/action');
    assert.deepEqual(JSON.parse(sent.body), { name: 'named', action: 'stop' });
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/managed-services': json({ services: [record({ unit: undefined })] }) });
  try {
    await mount(ServiceShell, shell);
    assert.ok(has('component-panels'));
    assert.equal(buttons('services.start').length + buttons('services.stop').length + buttons('services.restart').length, 0, 'start or stop is offered without a unit');
  } finally {
    await cleanup();
  }
});

// 10 Oct 2026. Start, Stop and Restart are answered with what the service
// showed. A verified failure and an unknown result both stay on the page with
// the unit, the command and the service's own line; only the first is drawn as
// a failure. They were toasts that left after five seconds.
test('A component page: a service action that failed or whose result is unknown is said in place, with the service’s line', async () => {
  const outcome = (code, reason, vars) => () => Response.json({ error: 'server English', code, reason, vars }, { status: 502 });
  for (const [code, reason, tone, vars] of [
    ['SERVICE_ACTION_FAILED', 'check', 'failed', { unit: 'postfix', action: 'restart', command: 'sudo postfix check', detail: 'postfix: fatal: bad numerical configuration: message_size_limit = x' }],
    ['SERVICE_ACTION_FAILED', 'verify', 'failed', { unit: 'postfix', action: 'restart', command: 'sudo postfix status', detail: 'the mail system is not running', owner_unit: 'postfix@-.service' }],
    ['SERVICE_ACTION_UNKNOWN', undefined, 'unknown', { unit: 'postgresql', action: 'restart', command: 'sudo systemctl status postgresql@17-main.service', detail: 'state could not be read in time', owner_unit: 'postgresql@17-main.service' }],
  ]) {
    serve({ '/api/v1/managed-services': json({ services: [record()] }) }, 'admin', { '/api/v1/service/action': outcome(code, reason, vars) });
    try {
      await mount(ServiceShell, shell);
      const before = reads('/api/v1/managed-services');
      await press(buttons('services.restart')[0]);
      const notices = tree.root.findAll((node) => node.props['data-service-action'] !== undefined);
      assert.equal(notices.length, 1, `${code} ${reason}: no notice on the page`);
      assert.equal(notices[0].props['data-service-action'], tone, `${code} is drawn as "${notices[0].props['data-service-action']}"`);
      // The unknown result is on the attention surface, never the failure one.
      assert.equal(/bg-danger/.test(notices[0].props.className), tone === 'failed');
      assert.equal(/bg-warning-mark/.test(notices[0].props.className), tone === 'unknown');
      // The sentence is the catalogue's (tests/service-action-outcome.test.mjs); this
      // catalogue-less mount shows the server's, in the notice and nowhere else.
      assert.ok(has('server English'));
      assert.ok(has(vars.detail), 'the service’s own line is not shown');
      assert.equal(has('services.action.ownerUnit'), Boolean(vars.owner_unit), 'the unit that runs the service');
      assert.deepEqual(globalThis.currentTest.toasts, [], 'the outcome was also a toast');
      assert.equal(calls.filter((call) => call.url === '/api/v1/service/action').length, 1, 'the action was sent again');
      assert.equal(reads('/api/v1/managed-services'), before + 1, 'the state was not read again after the action');
      await press(buttons('common.close')[0]);
      assert.equal(tree.root.findAll((node) => node.props['data-service-action'] !== undefined).length, 0);
    } finally {
      await cleanup();
    }
  }
  // Any other refusal stays what it was.
  serve({ '/api/v1/managed-services': json({ services: [record()] }) }, 'admin', { '/api/v1/service/action': () => Response.json({ error: 'no', code: 'FORBIDDEN' }, { status: 403 }) });
  try {
    await mount(ServiceShell, shell);
    await press(buttons('services.restart')[0]);
    assert.equal(tree.root.findAll((node) => node.props['data-service-action'] !== undefined).length, 0);
    assert.deepEqual(globalThis.currentTest.toasts.map(([tone]) => tone), ['error']);
  } finally {
    await cleanup();
  }
});

test('A component page: while the page only checks for a running operation, Install does not say "installing"', async () => {
  serve({ '/api/v1/managed-services': json({ services: [record({ is_installed: false, status: '', unit: undefined })] }) });
  globalThis.currentTest.operation = { startInstall: async () => true, locked: true, checking: true, catalogSnapshot: null };
  try {
    await mount(ServiceShell, shell);
    assert.ok(has('svc.installChecking') && !has('svc.installing'));
    assert.ok(buttons('svc.installChecking').every((node) => node.props.disabled));
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/managed-services': json({ services: [record({ is_installed: false, status: '', unit: undefined })] }) });
  globalThis.currentTest.operation = { startInstall: async () => true, locked: true, checking: false, catalogSnapshot: null };
  try {
    await mount(ServiceShell, shell);
    assert.ok(has('svc.installing') && !has('svc.installChecking'));
  } finally {
    await cleanup();
  }
});

// --- Screens this file cannot mount: the same rule, read from their source -------------
test('the install dialogue, the navigation badge, the dashboard counts and the help drawer keep the rule', () => {
  const serviceList = source('components/ServiceList.tsx');
  const dialogue = serviceList.slice(serviceList.indexOf('function InstallServiceDialog({'), serviceList.indexOf('function ActionIcon({'));
  // Install is off until the server has said whether a repository is required.
  // alpha.105: a simulation in flight, or notable installs not yet confirmed, also hold Install.
  assert.match(dialogue, /disabled=\{busy \|\| repoBusy \|\| !repoKnown \|\| previewGate\.hold \|\| Boolean\(repo\?\.required/);
  assert.match(dialogue, /const repoKnown = repoRead\.remote\.state === 'known' && !repoRead\.reading;/);
  assert.doesNotMatch(dialogue, /fetch\(`\/api\/v1\/repo\?|\.ok \? r\.json\(\) : null/);
  assert.match(dialogue, /t\(repoObserved \? 'services\.repo\.stale' : 'services\.repo\.unknown'/);

  // The domain badge is a number only for an answer, and keeps the earlier one.
  const layout = source('components/Layout.tsx');
  // alpha.95: the badge counts the domains and the DNS-only infrastructure
  // zone, as the Domains page lists them, still a number only for answers.
  assert.match(layout, /const domainCount = setupMode \? undefined : lastKnown\(domainsCount\(domainList\.remote, role === 'admin' \? zoneList\.remote : null\)\)\?\.value;/);
  assert.doesNotMatch(layout, /domains: Array\.isArray\(d\) \? d\.length : 0/);

  // A count on the dashboard is "…" or "–" until it is a number.
  const dashboard = source('components/Dashboard.tsx');
  assert.match(dashboard, /n: Remote<number>;/);
  assert.match(dashboard, /\{countText\(n\)\}/);
  assert.doesNotMatch(dashboard, /extras\?\.databases \?\? 0|setUsersCount/);
  assert.match(dashboard, /\|\| domainList\.remote\.state !== 'known';/);

  // The help texts are fetched when the drawer opens, not with the page.
  const help = source('components/HelpDrawer.tsx');
  assert.match(help, /import type \{ HelpContent \} from '\.\.\/help\/serviceHelp';/);
  assert.match(help, /import\('\.\.\/help\/serviceHelp'\)/);
  assert.doesNotMatch(help, /^import \{[^}]*\} from '\.\.\/help\/serviceHelp';/m);
});

// --- The layer itself ------------------------------------------------------------

test('readRemote never throws and never invents a value', async () => {
  const { readRemote, decodeList, lastKnown, mapRemote, LOADING } = await import(remoteURL);
  const run = async (answer, decode = (raw) => raw, earlier) => {
    globalThis.fetch = answer;
    try {
      return await readRemote('/x', decode, earlier);
    } finally {
      globalThis.fetch = originalFetch;
    }
  };
  assert.equal((await run(() => Promise.reject(new TypeError('offline')))).state, 'unknown');
  assert.equal((await run(async () => new Response('', { status: 500 }))).state, 'unknown');
  assert.equal((await run(async () => new Response('not json'))).state, 'unknown');
  assert.equal((await run(async () => Response.json({ rows: 1 }), decodeList)).state, 'unknown');
  assert.deepEqual((await run(async () => Response.json(null), decodeList)).value, []);
  const refused = await run(async () => Response.json({ error: 'e', code: 'C' }, { status: 409 }));
  assert.deepEqual([refused.state, refused.reason.code, 'previous' in refused], ['unknown', 'C', false]);
  const known = await run(async () => Response.json([1]), decodeList);
  assert.equal(known.state, 'known');
  assert.ok(Math.abs(known.observedAt - Date.now()) < 5000);
  const stale = await run(() => Promise.reject(new TypeError('offline')), decodeList, known);
  assert.deepEqual(stale.previous, { value: [1], observedAt: known.observedAt });
  const twice = await run(() => Promise.reject(new TypeError('offline')), decodeList, stale);
  assert.deepEqual(twice.previous, stale.previous, 'the earlier answer is lost by a second failure');
  assert.equal(lastKnown(LOADING), undefined);
  assert.equal(mapRemote(LOADING, () => 1), LOADING);
  assert.deepEqual(mapRemote(stale, (value) => value.length).previous.value, 1);
});

test('every text of the three states exists in both languages, and the could-not-check ones say nothing was changed', () => {
  const failed = new Set(['common.staleNotice', 'domains.pendingUnknown', 'databases.usersUnknown']);
  const other = new Set(['common.retry', 'conn.notChecked', 'conn.unknownHelp', 'conn.routeAUnknown', 'conn.sslUnknown',
    'databases.checkingUsers', 'databases.deleteDatabase', 'databases.deleteUser',
    // The second batch: sentences that are not a row's checking or could-not-check line.
    'common.resultUnknown', 'settings.2fa.resultUnknown', 'panelCert.notReadable', 'panelCert.unconfirmed', 'panelCert.notRecorded',
    'panelCert.failedDetail', 'panelCert.failedPlain', 'panelCert.checkAgain', 'panelCert.openSecure', 'panelCert.reopen.title',
    'panelCert.reopen.body', 'panelCert.reopen.reload', 'panelCert.reopen.stay', 'panelCert.reopen.stayed', 'plans.checking',
    'files.contentUnknown', 'files.uploadUnreadable', 'ssl.rereading', 'import.checkingSubs', 'import.noSubs', 'import.inspectUnanswered',
    'import.runAgain', 'import.unknown.title', 'import.unknown.body', 'import.unknown.check', 'import.unknown.checkAgain',
    'import.unknown.present', 'import.unknown.open', 'import.unknown.absent', 'license.noticeUnverified', 'license.noticeOpen',
    'domain.absent', 'domain.absentHint', 'domain.noAccess', 'domain.noAccessHint', 'component.absent', 'component.absentHint',
    'component.logsNoMatch', 'svc.checkingRecord', 'svc.recordUnread', 'svc.installChecking', 'services.versionUnknown',
    'services.repo.checking', 'dashboard.countUnread', 'help.loading']);
  for (const key of ['plans.unknown', 'import.subsUnknown', 'import.unknown.unreadable', 'svc.recordUnknown', 'svc.recordStale',
    'services.repo.unknown', 'services.repo.stale', 'help.unknown']) failed.add(key);
  for (const row of table) {
    if (row.checking) other.add(row.checking);
    for (const key of row.unknown.split('|')) if (key.includes('.') && !key.includes(' ')) failed.add(key);
  }
  const line = (catalogue, key) => catalogue.split('\n').find((candidate) => /^\s*(['"])/.test(candidate) && candidate.trimStart().slice(1).startsWith(`${key}${candidate.trimStart()[0]}`));
  for (const key of [...failed, ...other]) {
    const en = line(englishCatalogue, key);
    const tr = line(turkishCatalogue, key);
    assert.ok(en, `the English catalogue has no ${key}`);
    assert.ok(tr, `the Turkish catalogue has no ${key}`);
    assert.notEqual(en.split(':').slice(1).join(':'), tr.split(':').slice(1).join(':'), `${key} is not translated`);
  }
  // A could-not-check sentence says that nothing was changed, in both
  // languages. Two are about a look-up that could change nothing: the public
  // DNS of a domain, and the older DNS status sentence.
  for (const key of failed) {
    if (key === 'conn.readFailed' || key === 'dns.statusUnavailable') continue;
    assert.match(line(englishCatalogue, key), /Nothing was changed/, `${key} does not say that nothing was changed`);
    assert.match(line(turkishCatalogue, key), /Hiçbir şey değiştirilmedi/, `${key}: Turkish does not say that nothing was changed`);
  }
  // A checking line is one short sentence that ends in an ellipsis.
  for (const row of table) {
    if (!row.checking) continue;
    for (const catalogue of [englishCatalogue, turkishCatalogue]) assert.match(line(catalogue, row.checking), /…',$/, row.checking);
  }
});

// --- The HSTS retirement and the clock rule (D-032, 2026-10-14) ---------------
// The page took hsts_retire_after and compared it with the browser's clock. With
// the server's clock behind, a retirement still running read as over, and the
// removal the server refuses was offered; with it ahead, a finished one kept the
// controls locked. The server now states the time left on its own clock.
const sslWith = (settings) => () => {
  const body = healthy['/api/v1/domains/1/ssl']();
  return body.json().then((value) => Response.json({ ...value, settings: { ...value.settings, hsts_enabled: false, ...settings } }));
};
const minutes = (n) => new Date(Date.now() + n * 60_000).toISOString();
const removeButton = () => buttons('ssl.remove')[0];

for (const [name, settings, active] of [
  ['the server clock behind the browser: the stamp has passed here, ten minutes are left there', { hsts_retire_after: minutes(-5), hsts_retire_remaining_seconds: 600 }, true],
  ['the server clock ahead of the browser: the stamp is ahead here, nothing is left there', { hsts_retire_after: minutes(10), hsts_retire_remaining_seconds: 0 }, false],
  ['an older server without the time left keeps the stamp rule', { hsts_retire_after: minutes(10) }, true],
]) {
  test(`Domain certificate, HSTS retirement — ${name}`, async () => {
    serve({ '/api/v1/domains/1/ssl': sslWith(settings) });
    // A running retirement ticks its countdown on the window's timer.
    const timers = [];
    globalThis.window.setTimeout = (fn, ms) => { timers.push(ms); return timers.length; };
    globalThis.window.clearTimeout = () => {};
    try {
      await mount(DomainSSLSettings, { ...domain, mailAvailable: true });
      assert.equal(has('ssl.hstsRetirementActive'), active, `retirement shown = ${has('ssl.hstsRetirementActive')}`);
      assert.equal(removeButton().props.disabled, active, 'removing the certificate is offered only when the server allows it');
    } finally { await cleanup(); }
  });
}

// --- A certificate request in flight shows motion (owner rule 2026-10-14) ------
test('Domain certificate: while a certificate is requested the button spins and nothing else can be sent', async () => {
  const none = { domain_id: 1, domain_name: 'example.com', has_certificate: false, settings: { force_https: false, hsts_enabled: false, hsts_max_age: 300 } };
  let finish;
  serve({ '/api/v1/domains/1/ssl': () => Response.json(none) }, 'admin', {
    '/api/v1/domains/1/ssl/letsencrypt': () => new Promise((resolve) => { finish = () => resolve(Response.json({ success: true })); }),
  });
  try {
    await mount(DomainSSLSettings, { ...domain, mailAvailable: true });
    await act(async () => { tree.root.findAllByProps({ type: 'email' })[0].props.onChange({ target: { value: 'owner@example.com' } }); await settled(); });
    const issue = buttons('ssl.issue').find((node) => !node.props.disabled);
    await act(async () => { void issue.props.onClick({}); await settled(); });
    const busy = buttons('ssl.issuing')[0];
    assert.ok(busy, 'the button names the request in flight');
    assert.equal(busy.props['aria-busy'], true, 'the button shows its spinner');
    assert.equal(busy.props.disabled, true);
    assert.equal(calls.filter((call) => call.method === 'POST').length, 1);
    await act(async () => { finish(); await settled(); await settled(); });
  } finally { await cleanup(); }
});

test('Panel certificate: while the request runs the button spins', async () => {
  let finish;
  serve({ '/api/v1/panel/certificate': json({ https_enabled: false, self_signed: true, issuer: '', expires_at: '' }) }, 'admin', {
    '/api/v1/panel/certificate': () => new Promise((resolve) => { finish = () => resolve(Response.json({ error: 'stopped by the test' }, { status: 409 })); }),
  });
  globalThis.currentTest.section = 'panel';
  try {
    await mount(Settings);
    const field = tree.root.findAllByProps({ placeholder: 'panel.example.com' })[0];
    assert.ok(field, 'the panel certificate form is on the page');
    await act(async () => { field.props.onChange({ target: { value: 'panel.example.com' } }); await settled(); });
    const issue = buttons('panelCert.issue').find((node) => !node.props.disabled);
    assert.ok(issue, 'the request is offered');
    await act(async () => { void issue.props.onClick({}); await settled(); });
    const busy = buttons('panelCert.issuing')[0];
    assert.ok(busy, 'the button names the request in flight');
    assert.equal(busy.props['aria-busy'], true, 'the button shows its spinner');
    if (finish) await act(async () => { finish(); await settled(); await settled(); });
  } finally { await cleanup(); }
});

// v0.1.0-alpha.99: the missing contact email is fixed on My account; the
// refusal stays on the card with a button that goes there, not a toast.
test('Panel certificate: a missing contact email stays on the card with a button to My account', async () => {
  serve({ '/api/v1/panel/certificate': json({ https_enabled: false, self_signed: true, issuer: '', expires_at: '' }) }, 'admin', {
    '/api/v1/panel/certificate': () => Response.json({ error: 'server sentence', code: 'panel_certificate_contact_email_invalid', action: '/account#email' }, { status: 409 }),
  });
  globalThis.currentTest.section = 'panel';
  try {
    await mount(Settings);
    const field = tree.root.findAllByProps({ placeholder: 'panel.example.com' })[0];
    await act(async () => { field.props.onChange({ target: { value: 'panel.example.com' } }); await settled(); });
    await press(buttons('panelCert.issue').find((node) => !node.props.disabled));
    await act(async () => { await settled(); await settled(); });
    const refusal = tree.root.findAll((node) => node.props['data-panel-certificate-refusal'] === 'panel_certificate_contact_email_invalid');
    assert.equal(refusal.length, 1, 'the refusal is not on the card');
    assert.deepEqual(globalThis.currentTest.toasts.filter(([kind]) => kind === 'error'), [], 'the refusal passed as a toast');
    const go = refusal[0].findAll((node) => node.type === 'button');
    assert.equal(go.length, 1, 'no button to the place that fixes it');
    await act(async () => { go[0].props.onClick(); await settled(); });
    assert.deepEqual(globalThis.currentTest.navigated, ['/account#email']);
    assert.equal(calls.filter((call) => call.method === 'POST').length, 1);
  } finally { await cleanup(); }
});

test('Import: while an archive is inspected the button spins', async () => {
  serve({ '/api/v1/subscriptions': json({ subscriptions: [{ id: 3, name: 'Main', owner: 'admin' }] }) }, 'admin', {
    '/api/v1/import/cpanel/inspect': () => new Promise(() => {}),
  });
  try {
    await mount(ImportPage);
    await act(async () => { tree.root.findAllByProps({ placeholder: '/var/lib/celikpanel-imports/cpmove-user.tar.gz' })[0].props.onChange({ target: { value: '/var/lib/celikpanel-imports/a.tar.gz' } }); await settled(); });
    await act(async () => { void buttons('import.inspect')[0].props.onClick({}); await settled(); });
    const busy = buttons('import.inspecting')[0];
    assert.ok(busy, 'the button names the inspection in flight');
    assert.equal(busy.props['aria-busy'], true, 'the button shows its spinner');
    assert.equal(calls.filter((call) => call.method === 'POST').length, 1);
  } finally { await cleanup(); }
});

// --- A paused server change lifts by itself (owner rule 2026-10-14) -----------
// The install dialog read whether the server was ready once, when it opened, and
// said "Wait for it to finish" with nothing ever reading again.
test('A component page: the install dialog reads readiness again while the server is busy, says so, and lifts by itself', async () => {
  let busy = true;
  serve({
    '/api/v1/managed-services': json({ services: [record({ is_installed: false, status: '', unit: undefined })] }),
    '/api/v1/host-mutation-readiness': () => Response.json(busy ? { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' } : { ready: true }),
  });
  const timers = [];
  const realSetInterval = globalThis.setInterval;
  const realClearInterval = globalThis.clearInterval;
  globalThis.setInterval = (fn, ms) => { timers.push({ fn, ms }); return timers.length; };
  globalThis.clearInterval = () => {};
  try {
    await mount(ServiceShell, shell);
    await press(buttons('svc.install')[0]);
    assert.ok(has('services.mutationReadiness.package_manager_active'), text().slice(0, 600));
    assert.ok(has('services.mutationReadiness.autoResume'), 'the dialog does not say the pause lifts by itself');
    const poll = timers.find((item) => item.ms === 5000);
    assert.ok(poll, 'the dialog does not read again');
    busy = false;
    await act(async () => { poll.fn(); await settled(); await settled(); });
    assert.equal(reads('/api/v1/host-mutation-readiness'), 2);
    assert.ok(!has('services.mutationReadiness.autoResume') && !has('services.mutationReadiness.package_manager_active'), 'the pause did not lift');
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'reading readiness changed something');
  } finally {
    globalThis.setInterval = realSetInterval;
    globalThis.clearInterval = realClearInterval;
    await cleanup();
  }
});

// The dialog's re-read runs only while the tab is visible (2026-10-15, the open
// item of the alpha.85 notes): hidden, no interval and no read after the
// opening one; shown again, one read at once and the interval resumes.
test('A component page: the install dialog stops reading readiness while the tab is hidden, and reads once when shown', async () => {
  serve({
    '/api/v1/managed-services': json({ services: [record({ is_installed: false, status: '', unit: undefined })] }),
    '/api/v1/host-mutation-readiness': () => Response.json({ ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' }),
  });
  const doc = Object.assign(new EventTarget(), { visibilityState: 'hidden' });
  globalThis.document = doc;
  const timers = new Map();
  let next = 1;
  const realSetInterval = globalThis.setInterval;
  const realClearInterval = globalThis.clearInterval;
  globalThis.setInterval = (fn, ms) => { const id = next++; timers.set(id, { fn, ms }); return id; };
  globalThis.clearInterval = (id) => { timers.delete(id); };
  const polls = () => [...timers.values()].filter((item) => item.ms === 5000);
  try {
    await mount(ServiceShell, shell);
    await press(buttons('svc.install')[0]);
    assert.equal(reads('/api/v1/host-mutation-readiness'), 1, 'the dialog reads once when it opens');
    assert.equal(polls().length, 0, 'no re-read is scheduled while the tab is hidden');
    doc.visibilityState = 'visible';
    await act(async () => { doc.dispatchEvent(new Event('visibilitychange')); await settled(); await settled(); });
    assert.equal(reads('/api/v1/host-mutation-readiness'), 2, 'shown again: one read at once');
    assert.equal(polls().length, 1, 'the re-read resumes');
    doc.visibilityState = 'hidden';
    await act(async () => { doc.dispatchEvent(new Event('visibilitychange')); });
    assert.equal(polls().length, 0, 'hidden again: the re-read stops');
    assert.equal(calls.filter((call) => call.method !== 'GET').length, 0, 'reading readiness changed something');
  } finally {
    globalThis.setInterval = realSetInterval;
    globalThis.clearInterval = realClearInterval;
    await cleanup();
  }
});

// --- A component page whose stored record the server cannot verify (15 Oct 2026, D-032) ---
// The Panel refuses the stored record with 503 SERVICE_STATE_UNVERIFIED. The
// Dashboard and the Components page answer that with their own conditional
// check; the component's own page does too (it used to say "could not be read"
// and Retry kept failing until one of those pages ran the check).
// Sunucunun doğrulayamadığı kayıtlı kayıtta bileşenin kendi sayfası da kendi
// koşullu kontrolünü bir kez yapar; "okunamadı" deyip boşuna Tekrar dene sunmaz.
const UNVERIFIED = { error: 'the stored component record could not be verified', code: 'SERVICE_STATE_UNVERIFIED' };
const redisRecord = { id: 'redis', name: 'Redis', kind: 'service', is_installed: true, status: 'active (running)', unit: 'redis-server', versions: ['7'], ports: ['6379/tcp'], config_files: [{ path: '/etc/redis/redis.conf', is_managed: false }] };
const SCAN = '/api/v1/managed-services/scan';
// The record is refused until a scan ran; the scan answers `scan`. With
// `checks: false` the scan "succeeds" and the record stays refused for good.
function unverifiedUntilChecked(scan, { checks = true } = {}) {
  let checked = false;
  return {
    read: () => (checked ? Response.json({ services: [redisRecord] }) : Response.json(UNVERIFIED, { status: 503 })),
    changes: { [SCAN]: () => { if (checks) checked = true; return scan(); } },
  };
}
const READINESS = '/api/v1/host-mutation-readiness';
// The first check is refused because a change holds the server; the second succeeds.
function heldThenOk() {
  let attempts = 0;
  let checked = false;
  return {
    busy: true,
    read: () => (checked ? Response.json({ services: [redisRecord] }) : Response.json(UNVERIFIED, { status: 503 })),
    changes: {
      [SCAN]: () => {
        attempts += 1;
        if (attempts === 1) return Response.json({ error: 'busy', code: 'service_operation_busy' }, { status: 409 });
        checked = true;
        return Response.json({ services: [redisRecord], scanned_at: new Date().toISOString(), scan_age_seconds: 0 });
      },
    },
  };
}
// Captures the intervals a page schedules, so a test ticks them itself.
function fakeIntervals() {
  const timers = new Map();
  let next = 1;
  const realSet = globalThis.setInterval;
  const realClear = globalThis.clearInterval;
  globalThis.setInterval = (fn, ms) => { const id = next++; timers.set(id, { fn, ms }); return id; };
  globalThis.clearInterval = (id) => { timers.delete(id); };
  return {
    polls: () => [...timers.values()].filter((item) => item.ms === 5000),
    restore() { globalThis.setInterval = realSet; globalThis.clearInterval = realClear; },
  };
}
const scanCalls = () => calls.filter((call) => call.method === 'POST' && call.url.startsWith(SCAN));
const callLog = () => calls.map((call) => `${call.method} ${call.url}`).join(', ');
const freshCheck = async () => (await import(managedServicesURL)).resetStoredRecordCheck();
const spin = async (rounds) => { for (let i = 0; i < rounds; i += 1) await act(async () => { await settled(); }); };

for (const [name, Component, props, drawn] of [
  ['ServiceRecordLookup (a reloaded component page)', ServiceRecordLookup, { serviceId: 'redis', onBack() {}, children: () => 'component-page' }, 'component-page'],
  ['ComponentDetail (the sections of a component page)', ComponentDetail, { serviceId: 'redis', onBack() {} }, '/etc/redis/redis.conf'],
]) {
  test(`${name}: an unverifiable stored record → one conditional check, then the record is read again; no toast`, async () => {
    await freshCheck();
    const door = unverifiedUntilChecked(() => Response.json({ services: [redisRecord], scanned_at: new Date().toISOString(), scan_age_seconds: 0 }));
    serve({ '/api/v1/managed-services': door.read }, 'admin', door.changes);
    try {
      await mount(Component, props);
      await spin(4);
      assert.equal(scanCalls().length, 1, `expected one check: ${callLog()}`);
      assert.equal(scanCalls()[0].url, `${SCAN}?max_age_seconds=300`, 'the page asked for an unconditional scan');
      assert.ok(has(drawn), `the record is not drawn after the check: ${text().slice(0, 300)}`);
      assert.ok(!has('component.unknown'), 'the refusal was still reported as unreadable');
      assert.deepEqual(globalThis.currentTest.toasts, [], 'a check nobody pressed raised a toast');
      assert.equal(calls.filter((call) => call.method !== 'GET' && !call.url.startsWith(SCAN)).length, 0, 'the page changed something');
    } finally { await cleanup(); }
  });

  test(`${name}: while the check runs the page says it is checking and offers no Retry`, async () => {
    await freshCheck();
    serve({ '/api/v1/managed-services': () => Response.json(UNVERIFIED, { status: 503 }) }, 'admin', { [SCAN]: () => new Promise(() => {}) });
    try {
      await mount(Component, props);
      await spin(3);
      assert.ok(has('component.checkingStored'), `no checking line: ${text().slice(0, 300)}`);
      assert.ok(!has('component.unknown'), 'the refusal is shown as unreadable while it is being checked');
      assert.equal(buttons('common.retry').length, 0, 'Retry is offered while the check is on its way');
      assert.equal(scanCalls().length, 1);
    } finally { await cleanup(); }
  });

  test(`${name}: a failed check is "could not check" with Retry, not asked again by itself; Retry asks once more`, async () => {
    await freshCheck();
    serve({ '/api/v1/managed-services': () => Response.json(UNVERIFIED, { status: 503 }) }, 'admin', { [SCAN]: () => Response.json({ error: 'agent unavailable' }, { status: 502 }) });
    try {
      await mount(Component, props);
      await spin(6);
      assert.equal(scanCalls().length, 1, `the failure was asked again by itself: ${callLog()}`);
      assert.ok(has('component.unknown'), `no could-not-check state: ${text().slice(0, 300)}`);
      assert.ok(!has('component.checkingStored'), 'still shown as checking');
      assert.ok(buttons('common.retry').length >= 1, 'no Retry');
      assert.deepEqual(globalThis.currentTest.toasts, []);
      await press(buttons('common.retry')[0]);
      await spin(4);
      assert.equal(scanCalls().length, 2, 'Retry did not ask once more');
    } finally { await cleanup(); }
  });

  test(`${name}: a check held by a change in progress says so, once, with Retry`, async () => {
    await freshCheck();
    serve({ '/api/v1/managed-services': () => Response.json(UNVERIFIED, { status: 503 }) }, 'admin', { [SCAN]: () => Response.json({ error: 'busy', code: 'service_operation_busy' }, { status: 409 }) });
    try {
      await mount(Component, props);
      await spin(6);
      assert.equal(scanCalls().length, 1, 'a held check was asked again by itself');
      assert.ok(has("component.checkHeld"), `the held state is not said: ${text().slice(300, 1200)}`);
      assert.ok(buttons('common.retry').length >= 1, 'no Retry');
      assert.deepEqual(globalThis.currentTest.toasts, []);
    } finally { await cleanup(); }
  });

  // --- A held check resumes by itself (15 Oct 2026) ---------------------------
  // The held state used to wait for the person's "Try again". It now follows
  // the Dashboard: the host-mutation-readiness read (GET only, tab visible,
  // one at a time, every 5 s) sees the server go from busy to ready, and the
  // record is asked for once more. No loop, and Retry stays.
  test(`${name}: a held check resumes by itself once the readiness read goes busy → ready: one follow-up check, GET only`, async () => {
    await freshCheck();
    const held = heldThenOk();
    const clock = fakeIntervals();
    serve({ '/api/v1/managed-services': held.read, [READINESS]: () => Response.json(held.busy ? { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' } : { ready: true }) }, 'admin', held.changes);
    try {
      await mount(Component, props);
      await spin(6);
      assert.equal(scanCalls().length, 1, `expected the one held check: ${callLog()}`);
      assert.ok(has('component.checkHeld'), 'the held state is not said');
      assert.ok(buttons('common.retry').length >= 1, 'Retry is gone');
      assert.equal(reads(READINESS), 1, 'the first readiness read did not run');
      assert.equal(clock.polls().length, 1, 'no re-read is scheduled');
      await act(async () => { clock.polls()[0].fn(); await settled(); await settled(); });
      assert.equal(reads(READINESS), 2);
      assert.equal(scanCalls().length, 1, 'a check was asked while the server was still busy');
      held.busy = false;
      await act(async () => { clock.polls()[0].fn(); await settled(); await settled(); });
      await spin(6);
      assert.equal(scanCalls().length, 2, `expected exactly one follow-up check: ${callLog()}`);
      assert.ok(has(drawn), `the record is not drawn after the follow-up: ${text().slice(0, 300)}`);
      assert.ok(!has('component.checkHeld'));
      assert.equal(clock.polls().length, 0, 'the readiness read goes on after the held state ended');
      assert.equal(reads(READINESS), 3, 'readiness was read more than the three times asked');
      assert.equal(calls.filter((call) => call.method !== 'GET' && !call.url.startsWith(SCAN)).length, 0, 'the page changed something');
      assert.deepEqual(globalThis.currentTest.toasts, []);
    } finally { clock.restore(); await cleanup(); }
  });

  test(`${name}: a held check is not followed up when the server never reads as busy, or stays busy (negative control)`, async () => {
    for (const busyAlways of [false, true]) {
      await freshCheck();
      const clock = fakeIntervals();
      serve({
        '/api/v1/managed-services': () => Response.json(UNVERIFIED, { status: 503 }),
        [READINESS]: () => Response.json(busyAlways ? { ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' } : { ready: true }),
      }, 'admin', { [SCAN]: () => Response.json({ error: 'busy', code: 'service_operation_busy' }, { status: 409 }) });
      try {
        await mount(Component, props);
        await spin(6);
        for (let i = 0; i < 4; i += 1) await act(async () => { clock.polls()[0]?.fn(); await settled(); await settled(); });
        await spin(4);
        assert.equal(scanCalls().length, 1, `${busyAlways ? 'busy forever' : 'never busy'}: a follow-up check ran: ${callLog()}`);
        assert.ok(has('component.checkHeld'), 'the held state was dropped');
        assert.ok(buttons('common.retry').length >= 1, 'no Retry');
      } finally { clock.restore(); await cleanup(); }
    }
  });

  test(`${name}: a held check reads readiness only while the tab is visible`, async () => {
    await freshCheck();
    const clock = fakeIntervals();
    serve({
      '/api/v1/managed-services': () => Response.json(UNVERIFIED, { status: 503 }),
      [READINESS]: () => Response.json({ ready: false, code: 'HOST_MUTATION_BUSY', reason: 'package_manager_active' }),
    }, 'admin', { [SCAN]: () => Response.json({ error: 'busy', code: 'service_operation_busy' }, { status: 409 }) });
    const doc = Object.assign(new EventTarget(), { visibilityState: 'hidden' });
    globalThis.document = doc;
    try {
      await mount(Component, props);
      await spin(6);
      assert.ok(has('component.checkHeld'));
      assert.equal(reads(READINESS), 0, 'readiness was read in a hidden tab');
      assert.equal(clock.polls().length, 0, 'a re-read is scheduled in a hidden tab');
      doc.visibilityState = 'visible';
      await act(async () => { doc.dispatchEvent(new Event('visibilitychange')); await settled(); await settled(); });
      assert.equal(reads(READINESS), 1, 'shown again: one read at once');
      assert.equal(clock.polls().length, 1, 'the re-read does not resume');
      doc.visibilityState = 'hidden';
      await act(async () => { doc.dispatchEvent(new Event('visibilitychange')); });
      assert.equal(clock.polls().length, 0, 'hidden again: the re-read goes on');
      assert.equal(calls.filter((call) => call.method !== 'GET' && !call.url.startsWith(SCAN)).length, 0);
      assert.equal(scanCalls().length, 1);
    } finally { clock.restore(); await cleanup(); }
  });

  test(`${name}: a refusal that returns after a successful check is not checked again by itself`, async () => {
    // The check succeeds but the record stays refused: one check, not a loop.
    await freshCheck();
    const door = unverifiedUntilChecked(() => Response.json({ services: [redisRecord] }), { checks: false });
    serve({ '/api/v1/managed-services': door.read }, 'admin', door.changes);
    try {
      await mount(Component, props);
      await spin(8);
      assert.equal(scanCalls().length, 1, `a loop: ${callLog()}`);
      assert.ok(has('component.unknown'), 'the record that is still refused is not reported');
    } finally { await cleanup(); }
  });
}

test('a component page whose stored record is refused for another reason does not run the check (negative control)', async () => {
  serve({ '/api/v1/managed-services': json({ error: 'server sentence', code: 'INTERNAL' }, 502) });
  try {
    await mount(ServiceRecordLookup, { serviceId: 'redis', onBack() {}, children: () => 'component-page' });
    await spin(3);
    assert.equal(scanCalls().length, 0, 'the check ran for an unrelated refusal');
    assert.ok(has('component.unknown'));
  } finally { await cleanup(); }
});


// --- Second server as a full peer, part 2 (2026-10-16) -------------------------
// The infrastructure zone on the primary: a DNS-only entry of Domains whose one
// action opens the existing record editor; setup's records say they are
// setup's, the other server's say so, the owner's are the owner's.
const InfrastructureZonePage = (await import(shared.compile('components/InfrastructureZone.tsx', {
  ...own, '/DomainDNSManager': shared.compile('components/DomainDNSManager.tsx', own),
}))).InfrastructureZonePage;
const infraZone = (overrides = {}) => ({ zone: 'celikhost.com', kind: 'infrastructure', records: 9, peer_records: 1, ...overrides });
const infraRecords = [
  { id: 1, domain_id: 4, name: 'frankfurt.celikhost.com', type: 'A', content: '192.0.2.1', ttl: 300, disabled: false, managed_by: 'setup' },
  { id: 2, domain_id: 4, name: 'mail.boston.celikhost.com', type: 'A', content: '192.0.2.2', ttl: 300, disabled: false, managed_by: 'peer' },
  { id: 3, domain_id: 4, name: 'celikhost.com', type: 'TXT', content: '"owner"', ttl: 300, disabled: false },
];
const infraBase = '/api/v1/dns/infrastructure-zones/celikhost.com';
const infraServed = {
  '/api/v1/dns/infrastructure-zones': json({ zones: [infraZone()] }),
  [`${infraBase}/zone`]: json({ id: 4, name: 'celikhost.com', type: 'MASTER' }),
  [`${infraBase}/records`]: json({ records: infraRecords }),
};

test('Domains: an administrator sees the infrastructure zone as a DNS-only entry whose one action opens its records', async () => {
  serve(infraServed);
  try {
    await mount(Domains);
    assert.ok(has('celikhost.com') && has('domains.infrastructure.badge') && has('domains.infrastructure.status'));
    assert.equal(buttons('domains.action.delete').length, 1, 'the infrastructure zone can be removed from the list');
    assert.equal(buttons('domains.infrastructure.manage').length, 1);
    await press(buttons('domains.infrastructure.manage')[0]);
    assert.deepEqual(globalThis.currentTest.navigated, ['/domains/infrastructure/celikhost.com']);
  } finally {
    await cleanup();
  }
  // Control: a reseller is never asked for it and never shown it.
  serve(infraServed, 'reseller');
  try {
    await mount(Domains);
    assert.equal(reads('/api/v1/dns/infrastructure-zones'), 0);
    assert.ok(!has('domains.infrastructure.badge'));
  } finally {
    await cleanup();
  }
});

test('Domains: the zone a website now uses is that website, marked; an unreadable zone list is said in one line', async () => {
  serve({ '/api/v1/dns/infrastructure-zones': json({ zones: [infraZone({ zone: 'example.com', domain_id: 1 })] }) });
  try {
    await mount(Domains);
    assert.ok(has('domains.infrastructure.siteBadge'));
    assert.ok(!has('domains.infrastructure.badge'), 'the website is listed twice');
    assert.equal(buttons('domains.infrastructure.manage').length, 0);
  } finally {
    await cleanup();
  }
  serve({ '/api/v1/dns/infrastructure-zones': failures['refused by the server'] });
  try {
    await mount(Domains);
    assert.ok(has('domains.infrastructure.unknown'));
    assert.ok(has('example.com'), 'the domains were hidden by the zone read');
    assert.ok(!has('domains.empty'));
  } finally {
    await cleanup();
  }
});

test('Infrastructure zone records: setup’s are setup’s and cannot be removed here, the peer’s and the owner’s can', async () => {
  serve(infraServed);
  try {
    await mount(DomainDNSManager, { domainId: 0, domainName: 'celikhost.com', infrastructureZone: true });
    assert.equal(reads(`${infraBase}/zone`), 1);
    assert.equal(reads('/api/v1/domains/0/dns/zone'), 0, 'the editor read a domain that does not exist');
    assert.equal(reads('/api/v1/domains/0/dnssec'), 0, 'DNSSEC is offered for the infrastructure zone');
    assert.ok(has('dns.managedBySetup') && has('dns.requestedByPeer'));
    assert.equal(buttons('dns.republish').length, 0);
    const trash = tree.root.findAll((node) => node.type === 'button' && node.props['aria-label'] === 'dns.confirmDelete');
    assert.equal(trash.length, 3);
    // The rows are drawn in the order the server listed them.
    assert.equal(trash[0].props.disabled, true, 'setup’s record can be removed');
    assert.equal(trash[0].props.title, 'dns.managedBySetupHint');
    assert.equal(Boolean(trash[1].props.disabled), false, 'the peer’s record cannot be removed by the owner');
    assert.equal(Boolean(trash[2].props.disabled), false, 'the owner’s record cannot be removed');
    globalThis.confirm = () => true;
    await press(trash[2]);
    assert.ok(calls.some((call) => call.method === 'DELETE' && call.url === `${infraBase}/records?id=3`), 'the delete did not go to the zone’s own endpoint');
  } finally {
    await cleanup();
  }
  // Control: an ordinary domain's records carry no ownership words.
  serve();
  try {
    await mount(DomainDNSManager, domain);
    assert.ok(!has('dns.managedBySetup') && !has('dns.requestedByPeer'));
  } finally {
    await cleanup();
  }
});

test('Infrastructure zone page: what the zone is and who keeps which record come before the editor; an unknown zone is said', async () => {
  serve(infraServed);
  try {
    await mount(InfrastructureZonePage, { zone: 'celikhost.com' });
    const page = text();
    for (const key of ['domains.infrastructure.what', 'domains.infrastructure.managed', 'domains.infrastructure.yours', 'domains.infrastructure.peer', 'domains.infrastructure.openSetup']) {
      assert.ok(page.includes(key), key);
    }
    assert.ok(page.indexOf('domains.infrastructure.what') < page.indexOf('frankfurt.celikhost.com'));
    assert.ok(!page.includes('domains.infrastructure.openSite'), 'a website is claimed for the zone');
  } finally {
    await cleanup();
  }
  serve({ ...infraServed, '/api/v1/dns/infrastructure-zones': json({ zones: [] }) });
  try {
    await mount(InfrastructureZonePage, { zone: 'celikhost.com' });
    assert.ok(has('domains.infrastructure.missing'));
    assert.equal(reads(`${infraBase}/records`), 0);
  } finally {
    await cleanup();
  }
});

test('every infrastructure zone text exists in both languages', () => {
  const catalogue = (lang) => (lang === 'en' ? englishScreens : turkishScreens);
  const keysIn = (text) => [...text.matchAll(/^\s*'((?:domains\.infrastructure\.[A-Za-z]+)|dns\.managedBySetup(?:Hint)?|dns\.requestedByPeer)':\s*'(.*)',\s*$/gm)].map((m) => [m[1], m[2]]);
  const en = new Map(keysIn(catalogue('en')));
  const tr = new Map(keysIn(catalogue('tr')));
  assert.equal(en.size, 20);
  for (const [key, value] of en) assert.ok(tr.has(key) && tr.get(key) !== value, key);
});

// alpha.97 (D-036): every component page (the generic one and the specialised
// ones, which share this shell) shows the installed package version in its
// header in full, as the package database printed it, and where it came from;
// packages that differ are each named; a kept reading says when it was read;
// a record without one says nothing about versions.
// Her bileşen sayfasının başlığı kurulu paket sürümünü tam hâliyle ve
// kaynağıyla gösterir.
test('a component page shows the full package version and its source in its header; unknown says nothing', async () => {
  const page = async (extra) => {
    await freshCheck();
    serve({ '/api/v1/managed-services': () => Response.json({ services: [{ ...redisRecord, versions: [], ...extra }], scanned_at: new Date().toISOString(), scan_age_seconds: 0 }) });
    await mount(ServiceShell, shell);
    await spin(3);
  };
  const versionNode = () => tree.root.findAll((node) => node.props && node.props['data-package-version'] !== undefined)[0];
  const sourceNode = () => tree.root.findAll((node) => node.props && node.props['data-package-version-source'] !== undefined)[0];
  try {
    await page({ package_version: '5:7.0.15-3+deb13u1', package_version_short: '7.0.15', package_versions: [{ package: 'redis-server', version: '5:7.0.15-3+deb13u1' }], package_version_read_at: '2026-10-21T08:00:00Z' });
    assert.equal(versionNode()?.props['data-package-version'], '5:7.0.15-3+deb13u1');
    assert.ok(has('component.packageVersion'), 'the row label is missing');
    assert.ok(has('5:7.0.15-3+deb13u1'), 'the full version is not shown');
    assert.equal(sourceNode().props['data-package-version-source'], 'read');
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-package-version-kept-long'] !== undefined).length, 0, 'a fresh reading has no stuck line');
    assert.ok(has('component.packageVersionSource'));
  } finally { await cleanup(); }
  try {
    await page({ package_versions_differ: true, package_versions: [{ package: 'redis-server', version: '5:7.0.15-3' }, { package: 'redis-tools', version: '5:7.0.14-1' }], package_version_kept: true, package_version_kept_long: true, package_version_read_at: '2026-10-21T08:00:00Z' });
    assert.equal(versionNode()?.props['data-package-version'], 'several');
    assert.ok(has('component.packageVersionSeveral'));
    assert.ok(has('redis-server 5:7.0.15-3') && has('redis-tools 5:7.0.14-1'), 'each package is named with its own version');
    assert.equal(sourceNode().props['data-package-version-source'], 'kept');
    assert.ok(has('component.packageVersionKept'), 'a kept reading is not said as such');
    // Busy at every check for more than a day: one short line on what to check.
    assert.equal(tree.root.findAll((node) => node.props && node.props['data-package-version-kept-long'] !== undefined).length, 1);
    assert.ok(has('component.packageVersionKeptLong'));
  } finally { await cleanup(); }
  try {
    await page({});
    assert.equal(versionNode(), undefined, 'a record without a version shows one');
    assert.ok(!has('component.packageVersion'));
  } finally { await cleanup(); }
});
