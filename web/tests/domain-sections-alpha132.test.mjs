import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';
import { enScreens as englishScreens, trScreens as turkishScreens } from './locale-catalogue.mjs';

// alpha.132: a domain's page is nine sections in one row, by regrouping the
// eleven tabs and their sub-tabs. These tests pin what the regrouping must
// not lose: every older link (?tab=, the health checks' {tab, sub}) still
// leads to the same screen; the place on the page survives a reload; each
// viewer sees exactly the sections the tabs gave them, plus the sections the
// plan closes, faded; a team member never sees a section it was not granted.
//
// Alan adı sayfası dokuz bölüm: eski bağlantılar aynı ekrana götürür, yer
// adreste durur, her izleyici eskisi gibi kendi bölümlerini görür.

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const noImports = (file) => (specifier) => { throw new Error(`${file} imports ${specifier}`); };
const sections = await import(compileSource('components/domain/sections.ts', noImports('sections.ts')));
const tablist = await import(compileSource('lib/tablist.ts', noImports('tablist.ts')));

// --- Older links ---------------------------------------------------------------

// Every producer of a domain-page link before alpha.132, and where it leads now.
// Server-built links and notices stored in account_notices.fix_path carry
// these for good; the alias table is the compatibility layer (no migration).
const producers = [
  ['AddDomainModal.tsx (DNS publication pending)', 'dns', null, { section: 'dns', part: 'records' }],
  ['AddDomainModal.tsx (certificate asked)', 'ssl', null, { section: 'security', part: 'certificate' }],
  ['Dashboard.tsx (mail card)', 'mail', null, { section: 'mail' }],
  ['ImportPage.tsx (DNS)', 'dns', null, { section: 'dns', part: 'records' }],
  ['ImportPage.tsx (mail)', 'mail', null, { section: 'mail' }],
  ['KeptSiteRecords.tsx', 'dns', null, { section: 'dns', part: 'records' }],
  ['OutgoingMailSettings.tsx', 'mail', null, { section: 'mail' }],
  ['ServerCertificatesCard.tsx', 'ssl', null, { section: 'security', part: 'certificate' }],
  ['ServerSetup.tsx', 'ssl', null, { section: 'security', part: 'certificate' }],
  ['UsageNotices.tsx', 'config', null, { section: 'advanced', part: 'config' }],
  ['domain_handlers.go (partial success)', 'dns', null, { section: 'dns', part: 'records' }],
  ['domain_php_handlers.go (site has no PHP)', 'hosting', null, { section: 'site' }],
  ['dns_overview.go (own records)', 'dns', null, { section: 'dns', part: 'records' }],
  ['monitoring_problems.go', 'ssl', null, { section: 'security', part: 'certificate' }],
  ['monitoring_uptime.go', 'ssl', null, { section: 'security', part: 'certificate' }],
  ['notifications_handlers.go (PHP limit)', 'php', null, { section: 'site', part: 'php' }],
  ['notifications_handlers.go (mail)', 'mail', null, { section: 'mail' }],
  ['component_removal_plan.go (mail addresses)', 'mail', null, { section: 'mail' }],
  ['component_removal_plan.go (certificates)', 'ssl', null, { section: 'security', part: 'certificate' }],
  // The health checks' fixes, {kind: "tab", tab, sub} (domain_health.go).
  ['domain_health.go logs', 'advanced', 'logs', { section: 'advanced', part: 'logs' }],
  ['domain_health.go certificate', 'hosting', 'ssl', { section: 'security', part: 'certificate' }],
  ['domain_health.go mail', 'mail', '', { section: 'mail' }],
  ['domain_health.go files', 'files', '', { section: 'files', part: 'manager' }],
  ['domain_health.go backups', 'advanced', 'backups', { section: 'backups' }],
  // Every tab and sub-tab the page had, for links written by hand or stored.
  ['tab overview', 'overview', null, { section: 'overview' }],
  ['tab apps', 'apps', null, { section: 'site', part: 'apps' }],
  ['tab ai', 'ai', null, { section: 'site', part: 'ai' }],
  ['tab programs', 'programs', null, { section: 'site', part: 'programs' }],
  ['tab databases', 'databases', null, { section: 'databases' }],
  ['tab access', 'access', null, { section: 'files', part: 'access' }],
  ['hosting/general', 'hosting', 'general', { section: 'dns', part: 'names' }],
  ['hosting/type', 'hosting', 'type', { section: 'site', part: 'type' }],
  ['hosting/protect', 'hosting', 'protect', { section: 'security', part: 'protect' }],
  ['hosting/redirects', 'hosting', 'redirects', { section: 'site', part: 'redirects' }],
  ['hosting/web', 'hosting', 'web', { section: 'advanced', part: 'web' }],
  ['hosting/config', 'hosting', 'config', { section: 'advanced', part: 'config' }],
  ['hosting/php', 'hosting', 'php', { section: 'site', part: 'php' }],
  ['advanced/cron', 'advanced', 'cron', { section: 'advanced', part: 'cron' }],
  ['advanced/git', 'advanced', 'git', { section: 'advanced', part: 'git' }],
  ['advanced/copy', 'advanced', 'copy', { section: 'advanced', part: 'copy' }],
  ['advanced/malware', 'advanced', 'malware', { section: 'security', part: 'malware' }],
];

test('every older link leads to the section (and part) that holds its screen now', () => {
  for (const [producer, tab, sub, place] of producers) {
    assert.deepEqual(sections.legacyPlace(tab, sub), place, producer);
    if (!sub) assert.deepEqual(sections.requestedPlace(new URLSearchParams(`tab=${tab}`)), place, `${producer} as ?tab=`);
  }
  assert.equal(sections.legacyPlace('nonsense'), null);
  assert.equal(sections.legacyPlace('constructor'), null, 'an inherited name is not a tab');
  assert.equal(sections.requestedPlace(new URLSearchParams('')), null);
});

test('no producer emits a ?tab= value the alias table does not know', () => {
  const values = new Set();
  const scan = (dir, keep) => {
    for (const entry of readdirSync(new URL(dir, import.meta.url), { withFileTypes: true })) {
      const path = `${dir}${entry.name}`;
      if (entry.isDirectory()) scan(`${path}/`, keep);
      else if (keep(entry.name)) for (const match of read(path).matchAll(/\?tab=([a-z]+)/g)) values.add(match[1]);
    }
  };
  scan('../src/', (name) => /\.tsx?$/.test(name));
  scan('../../cmd/panel/', (name) => name.endsWith('.go') && !name.endsWith('_test.go'));
  // component_removal_plan.go builds `?tab=` + a name; its callers pass these.
  for (const match of read('../../cmd/panel/component_removal_plan.go').matchAll(/domainLink\([^,]+,\s*"([a-z]+)"\)/g)) values.add(match[1]);
  for (const match of read('../../cmd/panel/domain_health.go').matchAll(/tabFix\("([a-z]+)", "([a-z]*)"/g)) {
    assert.ok(sections.legacyPlace(match[1], match[2]), `health fix ${match[1]}/${match[2]}`);
  }
  assert.ok(values.size >= 5, `too few producers found: ${[...values]}`);
  for (const value of values) assert.ok(sections.legacyPlace(value), `?tab=${value} has no section`);
});

// --- The address ---------------------------------------------------------------

test('the place is written to the address and read back the same; other parameters stay', () => {
  for (const place of [{ section: 'overview' }, { section: 'advanced', part: 'logs' }, { section: 'security', part: 'certificate' }]) {
    const written = sections.withPlace(new URLSearchParams('tab=ssl&attention=dns-publication'), place);
    assert.equal(written.get('tab'), null, 'the older ?tab= is not kept beside the new place');
    assert.equal(written.get('attention'), 'dns-publication');
    assert.deepEqual(sections.requestedPlace(written), place);
  }
  // A part is dropped when the place has none; unknown names are not places.
  assert.equal(sections.withPlace(new URLSearchParams('section=site&part=php'), { section: 'mail' }).get('part'), null);
  assert.deepEqual(sections.requestedPlace(new URLSearchParams('section=site&part=nothing')), { section: 'site' });
  assert.equal(sections.requestedPlace(new URLSearchParams('section=nothing')), null);
  // The new address wins over an older one left beside it.
  assert.deepEqual(sections.requestedPlace(new URLSearchParams('section=backups&tab=dns')), { section: 'backups' });
});

// --- Who sees what ---------------------------------------------------------------

const server = { mail: true, databases: true };
const build = (overrides) => sections.buildSections({ role: 'customer', projectType: 'php', closedRights: [], server, ...overrides });
const ids = (list) => list.map((section) => (section.notInPlan ? `${section.id}*` : section.id));
const partsOf = (list, id) => list.find((section) => section.id === id)?.parts.map((part) => (part.readOnly ? `${part.id}:view` : part.id));
const ALL = ['overview', 'site', 'files', 'mail', 'databases', 'dns', 'security', 'backups', 'advanced'];

test('administrator, reseller and customer of a PHP site see the nine sections and every part the tabs had', () => {
  const admin = build({ role: 'admin' });
  assert.deepEqual(ids(admin), ALL);
  assert.deepEqual(partsOf(admin, 'site'), ['type', 'apps', 'ai', 'programs', 'php', 'redirects']);
  assert.deepEqual(partsOf(admin, 'files'), ['manager', 'access']);
  assert.deepEqual(partsOf(admin, 'dns'), ['connection', 'names', 'records']);
  assert.deepEqual(partsOf(admin, 'security'), ['certificate', 'protect', 'malware']);
  assert.deepEqual(partsOf(admin, 'advanced'), ['cron', 'logs', 'git', 'copy', 'web', 'config']);
  for (const role of ['reseller', 'customer']) {
    const list = build({ role });
    assert.deepEqual(ids(list), ALL, role);
    assert.ok(!partsOf(list, 'advanced').includes('config'), `${role} gets the configuration file`);
  }
});

test('a section the plan closes is faded in its own place, with the right that opens it', () => {
  const list = build({ closedRights: ['backups', 'databases'] });
  assert.deepEqual(ids(list), ['overview', 'site', 'files', 'mail', 'databases*', 'dns', 'security', 'backups*', 'advanced']);
  assert.deepEqual(list.find((section) => section.id === 'backups').notInPlan, ['backups']);
  assert.deepEqual(list.find((section) => section.id === 'databases').parts, []);
  // Mail, and Files when the manager and every outside access are closed.
  const more = build({ role: 'reseller', closedRights: ['mail', 'file_manager', 'file_access', 'ssh'] });
  assert.deepEqual(ids(more), ['overview', 'site', 'files*', 'mail*', 'databases', 'dns', 'security', 'backups', 'advanced']);
  // Files with outside access still open is open, with that part only.
  const access = build({ closedRights: ['file_manager'] });
  assert.deepEqual(partsOf(access, 'files'), ['access']);
  // A closed part of an open section is left out, as its sub-tab was.
  const cron = build({ closedRights: ['cron', 'app_installer', 'wordpress_tools', 'service_apps'] });
  assert.ok(!partsOf(cron, 'advanced').includes('cron'));
  assert.ok(!partsOf(cron, 'site').includes('apps') && !partsOf(cron, 'site').includes('ai'));
  // The administrator's WordPress tools keep Applications open (as the tab did).
  assert.ok(partsOf(sections.buildSections({ role: 'admin', projectType: 'php', closedRights: ['app_installer', 'wordpress_tools'], server }), 'site').includes('apps'));
});

test('a section the server cannot offer is absent, not faded, and stays while the server is not known', () => {
  const none = { mail: false, databases: false };
  assert.deepEqual(ids(build({ role: 'admin', server: none })), ['overview', 'site', 'files', 'dns', 'security', 'backups', 'advanced']);
  // Closed by the plan AND not on the server: absent (nothing to ask for).
  assert.ok(!ids(build({ server: none, closedRights: ['mail', 'databases'] })).some((id) => id.startsWith('mail') || id.startsWith('databases')));
  // Not known yet: nothing hidden on a guess.
  assert.deepEqual(ids(build({ server: null })), ALL);
});

test('a DNS-only domain has Overview, Mail, Databases and Domain & DNS (N12)', () => {
  const list = build({ projectType: 'dnsonly' });
  assert.deepEqual(ids(list), ['overview', 'mail', 'databases', 'dns']);
  assert.deepEqual(partsOf(list, 'dns'), ['connection', 'records']);
  // A closed backup right does not fade a section a DNS-only domain never has.
  assert.deepEqual(ids(build({ projectType: 'dnsonly', closedRights: ['backups', 'file_manager'] })), ['overview', 'mail', 'databases', 'dns']);
});

test('a team member sees only what was granted, view-only where it may only view, and never a section it was not granted', () => {
  const member = (access, closedRights = []) => sections.buildSections({ role: 'additional_user', projectType: 'php', closedRights, access, server: { mail: false, databases: false } });
  const partial = member({ mail: 'view', dns: 'manage' });
  assert.deepEqual(ids(partial), ['mail', 'dns']);
  assert.deepEqual(partsOf(partial, 'mail'), ['mail:view']);
  assert.deepEqual(partsOf(partial, 'dns'), ['records'], 'a team member gets the connection or the names');
  // The server's engines are never asked for a team member: mail stays.
  const wide = member({ files: 'manage', php: 'view', ssl: 'view', backups: 'manage', cron: 'manage', statistics: 'view', databases: 'manage' });
  assert.deepEqual(ids(wide), ['site', 'files', 'databases', 'security', 'backups', 'advanced']);
  assert.deepEqual(partsOf(wide, 'site'), ['php:view', 'redirects']);
  assert.deepEqual(partsOf(wide, 'security'), ['certificate:view', 'protect']);
  assert.deepEqual(partsOf(wide, 'advanced'), ['cron', 'logs:view', 'web']);
  // A grant the plan closes is faded; what was not granted stays absent.
  const closed = member({ mail: 'manage', dns: 'view' }, ['mail', 'backups', 'databases']);
  assert.deepEqual(ids(closed), ['mail*', 'dns']);
});

test('the row keeps one order for every viewer and plan: a closed section stays in its place, an absent one closes up', () => {
  const rights = ['mail', 'databases', 'backups', 'file_manager', 'file_access', 'ssh', 'cron', 'service_apps', 'program_apps', 'python_apps', 'app_installer', 'wordpress_tools', 'malware_scan', 'site_protect_redirects', 'dns_edit', 'dnssec'];
  const subsets = [[], ...rights.map((right) => [right]), rights, ['backups', 'databases'], ['mail', 'file_manager', 'file_access', 'ssh']];
  const grants = [undefined, { files: 'manage', php: 'none', ssl: 'view', dns: 'view', mail: 'manage', databases: 'none', backups: 'manage', cron: 'none', statistics: 'view' }];
  for (const role of ['admin', 'reseller', 'customer', 'additional_user']) {
    for (const projectType of ['php', 'static', 'node', 'runtime_app', 'forwarding', 'dnsonly']) {
      for (const closedRights of subsets) {
        for (const serverState of [server, null, { mail: false, databases: true }]) {
          const access = role === 'additional_user' ? grants[1] : grants[0];
          const order = sections.buildSections({ role, projectType, closedRights, access, server: serverState }).map((section) => sections.SECTION_ORDER.indexOf(section.id));
          assert.deepEqual(order, [...order].sort((a, b) => a - b), `${role} ${projectType} ${closedRights.join(',')}`);
        }
      }
    }
  }
});

// --- The guide -----------------------------------------------------------------

test('the guide topic of every section and part is claimed by an English and a Turkish chapter', () => {
  for (const locale of ['en', 'tr']) {
    const claimed = new Set();
    for (const name of readdirSync(new URL(`../../docs/guide/${locale}/`, import.meta.url))) {
      const topics = read(`../../docs/guide/${locale}/${name}`).match(/^topics: \[([^\]]*)\]/m)?.[1] ?? '';
      for (const entry of topics.split(',')) if (entry.trim()) claimed.add(entry.trim().split('#')[0]);
    }
    for (const topic of [...Object.values(sections.SECTION_TOPICS), ...Object.values(sections.PART_TOPICS)]) {
      assert.ok(claimed.has(topic), `${locale}: ${topic} is claimed by no chapter`);
    }
    for (const old of ['domain.hosting', 'domain.apps', 'domain.ai', 'domain.access']) assert.ok(!claimed.has(old), `${locale}: ${old} is still a topic`);
  }
  assert.equal(sections.guideTopic('site', 'apps'), 'domain.site.apps');
  assert.equal(sections.guideTopic('site', 'php'), 'domain.site');
  assert.equal(sections.guideTopic('backups', null), 'domain.backups');
});

test('every section and part has its words in both languages', () => {
  const keys = [...Object.values(sections.SECTION_LABELS), ...Object.values(sections.PART_LABELS)];
  for (const key of keys) assert.ok(englishScreens[key] && turkishScreens[key], key);
  for (const key of Object.keys(englishScreens).filter((name) => name.startsWith('domain.notInPlan.'))) assert.ok(turkishScreens[key], key);
  // The tab words left with the tabs.
  for (const key of Object.keys(englishScreens)) assert.ok(!/^domain\.(tab|sub)\.(?!ssl$)/.test(key), `${key} outlived its tab`);
  assert.equal(turkishScreens['domain.section.dns'], 'Alan adı ve DNS');
  assert.doesNotMatch(turkishScreens['domain.notInPlan.who.provider'] + turkishScreens['domain.notInPlan.who.administrator'], /kişisinden/);
});

// --- The row's keys --------------------------------------------------------------

test('arrow keys, Home and End move along the row and wrap; other keys do not move', () => {
  assert.equal(tablist.nextTabIndex('ArrowRight', 0, 3), 1);
  assert.equal(tablist.nextTabIndex('ArrowRight', 2, 3), 0);
  assert.equal(tablist.nextTabIndex('ArrowLeft', 0, 3), 2);
  assert.equal(tablist.nextTabIndex('ArrowDown', 1, 3), 2);
  assert.equal(tablist.nextTabIndex('Home', 2, 3), 0);
  assert.equal(tablist.nextTabIndex('End', 0, 3), 2);
  assert.equal(tablist.nextTabIndex('Enter', 0, 3), null);
  assert.equal(tablist.nextTabIndex('ArrowRight', 0, 0), null);
});

// --- The page, mounted -----------------------------------------------------------

const source = (path) => read(`../src/${path}`);
const icons = new Set(['AlertTriangle', 'XCircle']);
for (const file of ['components/ui.tsx', 'components/DomainDetail.tsx', 'components/domain/SectionRow.tsx', 'components/domain/SectionParts.tsx', 'lib/domainSectionIcons.ts']) {
  for (const match of source(file).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const name = item.replace(/\btype\b/, '').trim();
      if (name && name !== 'LucideIcon') icons.add(name);
    }
  }
}
// The address is a small store, so a section change re-renders the page as
// the router does; every write is kept with whether it replaced the entry.
// Like the router, a write asks first while something is unsaved (the real
// leave guard, set below), and an in-place rewrite does not ask.
globalThis.a132Address = { search: '', writes: [], listeners: new Set() };
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  const i18n = { t: (key, vars) => vars && vars.section ? key + ':' + vars.section : key, locale: 'en' };
  export const useI18n = () => i18n;
  export const useNavigate = () => () => true;
  export const useAuth = () => ({ role: globalThis.a132Role });
  export const Link = ({ to, children }) => React.createElement('a', { href: to }, children);
  const address = globalThis.a132Address;
  const subscribe = (listener) => { address.listeners.add(listener); return () => address.listeners.delete(listener); };
  export const useSearchParams = () => {
    const search = React.useSyncExternalStore(subscribe, () => address.search);
    const params = React.useMemo(() => new URLSearchParams(search), [search]);
    const set = React.useCallback((next, options = {}) => {
      const write = () => {
        address.search = next.toString();
        address.writes.push({ search: address.search, replace: Boolean(options.replace) });
        address.listeners.forEach((listener) => listener());
      };
      const asking = globalThis.a132AskToLeave?.();
      if (asking) { void asking.then((leave) => { if (leave) write(); }); return; }
      write();
    }, []);
    return [params, set];
  };
  export const useLocation = () => ({ pathname: '/domains/example.com', search: address.search ? '?' + address.search : '', hash: '' });
  export const replaceCurrentRouterURL = (to) => {
    address.search = new URL(to, 'https://panel.test').search.slice(1);
    address.writes.push({ search: address.search, replace: true, inPlace: true });
    address.listeners.forEach((listener) => listener());
    return true;
  };
  export const DomainConnection = () => React.createElement('aside', { 'data-stub': 'connection' });
  export const decodeSSLData = (raw) => raw;
`);
const shared = sharedLayer(stub);
globalThis.a132AskToLeave = (await import(shared.resolve('/leaveAsker'))).askToLeave;
const { useLeaveGuard } = await import(shared.resolve('/leaveGuard'));
const sectionsURL = compileSource('components/domain/sections.ts', () => stub);
const own = {
  '/sections': sectionsURL,
  '/auth/domainAccess': compileSource('auth/domainAccess.ts', () => stub),
  '/help/guide/context': compileSource('help/guide/context.ts', () => stub),
  '/lib/tablist': compileSource('lib/tablist.ts', () => stub),
  '/lib/domainSectionIcons': compileSource('lib/domainSectionIcons.ts', (specifier) => (specifier.endsWith('/sections') ? sectionsURL : stub)),
};
own['/domain/SectionRow'] = shared.compile('components/domain/SectionRow.tsx', own);
// Each section stands in as one element that names its parts; the health
// strip keeps the two functions the page gave it.
const sectionStub = (name, id) => dataModule(`
  import React from '${reactURL}';
  export const ${name} = (props) => { globalThis.a132Go = props.go; return React.createElement('main', { 'data-stub-section': '${id}', 'data-parts': props.parts.map((p) => p.id + (p.readOnly ? ':view' : '')).join(','), 'data-part': props.part ?? '' }); };
`);
let detailURL = shared.compile('components/DomainDetail.tsx', own);
for (const [name, id] of [['OverviewSection', 'overview'], ['SiteSection', 'site'], ['FilesSection', 'files'], ['MailSection', 'mail'], ['DatabasesSection', 'databases'], ['DnsSection', 'dns'], ['SecuritySection', 'security'], ['BackupsSection', 'backups'], ['AdvancedSection', 'advanced']]) {
  detailURL = replaceLazyImport(detailURL, `./domain/sections/${name}`, sectionStub(name, id));
}
detailURL = replaceLazyImport(detailURL, './domain/sections/NotInPlanSection', dataModule(`
  import React from '${reactURL}';
  export const NotInPlanSection = (props) => React.createElement('main', { 'data-stub-not-in-plan': props.section, 'data-rights': props.rights.join(',') });
`));
detailURL = replaceLazyImport(detailURL, './DomainHealthStrip', dataModule(`
  export const DomainHealthStrip = (props) => { globalThis.a132Strip = props; return null; };
`));
detailURL = replaceLazyImport(detailURL, './DomainSiteConfig', dataModule('export const SiteConfigNotice = () => null;'));
const { DomainDetail } = await import(detailURL);

// A team member's grants as the server sends them: every capability named.
const grants = (given) => ({ files: 'none', php: 'none', ssl: 'none', dns: 'none', mail: 'none', databases: 'none', backups: 'none', cron: 'none', statistics: 'none', ...given });
const capabilities = (overrides = {}) => ({
  web_server: 'nginx', php_versions: ['8.3'], dns_server: 'bind', dns_identity_ready: true,
  dns_management_mode: 'local', dns_management_ready: true, mail_server: true,
  database_servers: ['mariadb'], db_tools: [], ...overrides,
});
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
let tree;
async function open({ role = 'customer', row = {}, search = '', caps = capabilities() } = {}) {
  globalThis.a132Role = role;
  globalThis.a132Strip = undefined;
  Object.assign(globalThis.a132Address, { search, writes: [] });
  globalThis.document = { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible', getElementById: () => null };
  const domain = { id: 1, domain_name: 'example.com', project_type: 'php', php_version: '8.3', status: 'active', created_at: '2026-11-01T00:00:00Z', ...row };
  globalThis.fetch = (url) => {
    const path = String(url).split('?')[0];
    if (path === '/api/v1/domains') return Promise.resolve(Response.json([domain]));
    if (path === '/api/v1/hosting/capabilities') return Promise.resolve(Response.json(caps));
    if (path === '/api/v1/domains/1/usage') return Promise.resolve(Response.json({ disk_usage: 0, bandwidth: 0 }));
    if (path === '/api/v1/domains/1/ssl') return Promise.resolve(Response.json({ domain_id: 1, has_certificate: false }));
    return Promise.resolve(Response.json({ error: 'no such route' }, { status: 404 }));
  };
  await act(async () => { tree = Renderer.create(React.createElement(DomainDetail, { domainId: 1, onBack() {} })); });
  for (let i = 0; i < 6; i += 1) await act(async () => { await settled(); });
}
async function close() {
  await act(async () => { tree?.unmount(); });
  tree = undefined;
}
const row = () => tree.root.findAll((node) => node.type === 'button' && node.props.role === 'tab')
  .map((node) => (node.props['data-not-in-plan'] === '' ? `${node.props['data-section']}*` : node.props['data-section']));
const selected = () => tree.root.findAll((node) => node.type === 'button' && node.props.role === 'tab' && node.props['aria-selected']).map((node) => node.props['data-section']);
const shown = () => {
  const stubbed = tree.root.findAll((node) => node.type === 'main');
  assert.equal(stubbed.length, 1, 'one section body');
  const body = stubbed[0].props;
  return body['data-stub-not-in-plan'] ? { notInPlan: body['data-stub-not-in-plan'], rights: body['data-rights'] } : { section: body['data-stub-section'], parts: body['data-parts'], part: body['data-part'] };
};
const press = async (section) => {
  const tab = tree.root.findAll((node) => node.type === 'button' && node.props['data-section'] === section)[0];
  await act(async () => { tab.props.onClick(); await settled(); });
  for (let i = 0; i < 3; i += 1) await act(async () => { await settled(); });
};

test('mounted: each viewer gets its row, the first section open, and the tablist wired to the panel', async () => {
  const cases = [
    { name: 'administrator', role: 'admin', expect: ALL },
    { name: 'reseller, mail closed', role: 'reseller', row: { rights_closed: ['mail'] }, expect: ['overview', 'site', 'files', 'mail*', 'databases', 'dns', 'security', 'backups', 'advanced'] },
    { name: 'customer, backups and databases closed', role: 'customer', row: { rights_closed: ['backups', 'databases'] }, expect: ['overview', 'site', 'files', 'mail', 'databases*', 'dns', 'security', 'backups*', 'advanced'] },
    { name: 'team member, mail to view and DNS to manage', role: 'additional_user', row: { access: grants({ mail: 'view', dns: 'manage' }) }, expect: ['mail', 'dns'] },
    { name: 'DNS-only domain', role: 'customer', row: { project_type: 'dnsonly' }, expect: ['overview', 'mail', 'databases', 'dns'] },
    { name: 'server without mail or databases', role: 'admin', caps: capabilities({ mail_server: false, database_servers: [] }), expect: ['overview', 'site', 'files', 'dns', 'security', 'backups', 'advanced'] },
  ];
  for (const item of cases) {
    await open(item);
    try {
      assert.deepEqual(row(), item.expect, item.name);
      assert.deepEqual(selected(), [item.expect[0]], `${item.name}: the first section is the open one`);
      const panel = tree.root.find((node) => node.props.role === 'tabpanel');
      assert.equal(panel.props['aria-labelledby'], `domain-section-${item.expect[0]}-tab`);
      assert.equal(shown().section, item.expect[0]);
      assert.equal(globalThis.a132Address.writes.length, 0, `${item.name}: a plain address is not rewritten`);
    } finally { await close(); }
  }
  // The team member's mail is view-only and the strip is in its first section.
  await open({ role: 'additional_user', row: { access: grants({ mail: 'view', dns: 'manage' }) } });
  try {
    assert.equal(shown().parts, 'mail:view');
    assert.ok(globalThis.a132Strip, 'a team member has no health strip');
  } finally { await close(); }
});

test('mounted: a section is chosen into the address (a new entry), and a reload of that address opens it again', async () => {
  await open({ role: 'admin' });
  try {
    await press('security');
    assert.deepEqual(globalThis.a132Address.writes, [{ search: 'section=security', replace: false }]);
    assert.equal(shown().section, 'security');
    assert.deepEqual(selected(), ['security']);
  } finally { await close(); }
  await open({ role: 'admin', search: 'section=advanced&part=logs' });
  try {
    assert.deepEqual(shown(), { section: 'advanced', parts: 'cron,logs,git,copy,web,config', part: 'logs' });
    assert.equal(globalThis.a132Address.writes.length, 0, 'a canonical address is left as it is');
  } finally { await close(); }
});

test('mounted: an older ?tab= link opens its section and part, and the address is rewritten in place', async () => {
  for (const [search, section, part, canonical] of [
    ['tab=ssl', 'security', 'certificate', 'section=security&part=certificate'],
    ['tab=config', 'advanced', 'config', 'section=advanced&part=config'],
    ['tab=dns&attention=dns-publication', 'dns', 'records', 'attention=dns-publication&section=dns&part=records'],
    ['tab=php', 'site', 'php', 'section=site&part=php'],
  ]) {
    await open({ role: 'admin', search });
    try {
      assert.deepEqual(shown(), { section, parts: shown().parts, part }, search);
      assert.deepEqual(globalThis.a132Address.writes, [{ search: canonical, replace: true }], search);
    } finally { await close(); }
  }
  // The DNS publication notice still opens with its link.
  await open({ role: 'admin', search: 'tab=dns&attention=dns-publication' });
  try {
    const notice = tree.root.findAll((node) => typeof node.type === 'string' && node.props['data-dns-publication-attention'] === '');
    assert.equal(notice.length, 1);
    assert.equal(notice[0].props['data-guidance'], 'waiting', 'the notice is the one guidance block');
  } finally { await close(); }
});

test('mounted: a section the plan closes opens its own page; one not on this page falls back to the first, once the server answered', async () => {
  await open({ role: 'customer', row: { rights_closed: ['backups'] }, search: 'section=backups' });
  try {
    assert.deepEqual(shown(), { notInPlan: 'backups', rights: 'backups' });
    assert.deepEqual(selected(), ['backups']);
  } finally { await close(); }
  await open({ role: 'admin', search: 'section=mail', caps: capabilities({ mail_server: false }) });
  try {
    assert.equal(shown().section, 'overview');
    assert.deepEqual(globalThis.a132Address.writes, [{ search: 'section=overview', replace: true }]);
  } finally { await close(); }
});

test('mounted: the health strip\'s fixes name older tabs and lead to the sections that hold them', async () => {
  await open({ role: 'customer', row: { rights_closed: ['backups'] } });
  try {
    const strip = globalThis.a132Strip;
    assert.ok(strip, 'the Overview has no health strip');
    assert.equal(strip.hasTab('hosting', 'ssl'), true);
    assert.equal(strip.hasTab('advanced', 'logs'), true);
    assert.equal(strip.hasTab('mail', ''), true);
    assert.equal(strip.hasTab('advanced', 'backups'), false, 'a fix in a section the plan closes is not a way there');
    assert.equal(strip.hasTab('nothing'), false);
    await act(async () => { strip.onGo('hosting', 'ssl'); await settled(); });
    assert.deepEqual(globalThis.a132Address.writes.at(-1), { search: 'section=security&part=certificate', replace: false });
    assert.equal(shown().section, 'security');
  } finally { await close(); }
});

// --- Leaving a section with unsaved changes ----------------------------------------

// A screen part with unsaved changes, as useDraft registers one.
function Unsaved({ n }) {
  useLeaveGuard(n);
  return null;
}
let asked = [];
const answering = (answer) => {
  asked = [];
  globalThis.confirm = (text, request) => { asked.push(request); return answer; };
};
async function withUnsaved(run) {
  let probe;
  await act(async () => { probe = Renderer.create(React.createElement(Unsaved, { n: 1 })); });
  try { await run(); } finally { await act(async () => probe.unmount()); }
}

test('mounted: another section with unsaved changes asks once; Stay keeps them and the place, Leave gives them up and goes', async () => {
  await open({ role: 'admin' });
  try {
    await withUnsaved(async () => {
      answering(false);
      await press('security');
      assert.equal(asked.length, 1, 'switching section did not ask, or asked twice');
      assert.equal(asked[0].confirmLabel, 'Leave without saving');
      assert.equal(shown().section, 'overview', 'Stay left the section');
      assert.equal(globalThis.a132Address.writes.length, 0, 'Stay changed the address');
      answering(true);
      await press('security');
      assert.equal(asked.length, 1, 'Stay dropped the changes, or Leave was asked twice (once here, once by the router)');
      assert.equal(shown().section, 'security');
      assert.deepEqual(globalThis.a132Address.writes, [{ search: 'section=security', replace: false }]);
      assert.equal(globalThis.a132AskToLeave(), null, 'Leave kept the given-up changes');
    });
  } finally { await close(); }
});

test('mounted: with nothing unsaved a section switch asks nothing', async () => {
  await open({ role: 'admin' });
  try {
    answering(false);
    await press('mail');
    assert.equal(asked.length, 0);
    assert.equal(shown().section, 'mail');
  } finally { await close(); }
});

test('mounted: another part of the open section asks nothing, even with unsaved changes, and moves the address in place', async () => {
  await open({ role: 'admin', search: 'section=advanced' });
  try {
    await withUnsaved(async () => {
      answering(false);
      await act(async () => { await globalThis.a132Go({ section: 'advanced', part: 'logs' }, { replace: true }); await settled(); });
      assert.equal(asked.length, 0, 'a part of the same page asked about leaving');
      assert.deepEqual(globalThis.a132Address.writes, [{ search: 'section=advanced&part=logs', replace: true, inPlace: true }]);
      assert.equal(shown().part, 'logs');
    });
  } finally { await close(); }
});

// --- Long sections: parts that open and close (owner review, 12 Nov 2026) -----------

const openings = (list, id) => list.find((section) => section.id === id)?.parts.map((part) => `${part.id}:${part.startsOpen}`);

test('Site and Advanced start their parts open or closed by the site kind; the other sections keep every part open', () => {
  assert.deepEqual(openings(build({ role: 'admin' }), 'site'), ['type:true', 'apps:true', 'ai:false', 'programs:false', 'php:true', 'redirects:true']);
  assert.deepEqual(openings(build({ role: 'admin', projectType: 'service_app' }), 'site'), ['type:true', 'ai:true', 'programs:false', 'redirects:true']);
  assert.deepEqual(openings(build({ role: 'admin', projectType: 'runtime_app' }), 'site'), ['type:true', 'ai:false', 'programs:true', 'redirects:true']);
  assert.deepEqual(openings(build({ role: 'admin' }), 'advanced'), ['cron:true', 'logs:false', 'git:false', 'copy:false', 'web:false', 'config:false']);
  for (const section of build({ role: 'admin' }).filter((item) => item.id !== 'site' && item.id !== 'advanced')) {
    for (const part of section.parts) assert.equal(part.startsOpen, undefined, `${section.id}/${part.id} is not a closable card`);
  }
  // Every closable part says in one sentence what it is, in both languages.
  for (const section of build({ role: 'admin' }).filter((item) => item.id === 'site' || item.id === 'advanced')) {
    for (const part of section.parts) assert.ok(englishScreens[`domain.about.${part.id}`] && turkishScreens[`domain.about.${part.id}`], part.id);
  }
  assert.deepEqual([turkishScreens['domain.part.toggleOpen'], turkishScreens['domain.part.toggleClose']], ['Aç', 'Kapat']);
});

const { SectionParts } = await import(shared.compile('components/domain/SectionParts.tsx', { '/sections': sectionsURL }));
const { useLeaveGuard: partGuard } = await import(shared.resolve('/leaveGuard'));
// A part's screen as a probe: it says it was mounted, and holds unsaved changes when told to.
const mountedParts = [];
function PartProbe({ id }) {
  React.useEffect(() => { mountedParts.push(id); }, [id]);
  partGuard(globalThis.a132Dirty?.includes(id) ? 1 : 0);
  return React.createElement('p', { 'data-probe': id });
}
async function partsPage({ section = 'advanced', projectType = 'php', part = null } = {}) {
  mountedParts.length = 0;
  globalThis.document = { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible', getElementById: () => null };
  const went = [];
  const parts = build({ role: 'admin', projectType }).find((item) => item.id === section).parts;
  let page;
  await act(async () => {
    page = Renderer.create(React.createElement(SectionParts, {
      section, parts, part, go: (place, options) => { went.push({ place, options }); },
      render: (item) => React.createElement(PartProbe, { id: item.id }),
    }));
  });
  await act(async () => { await settled(); });
  const probes = () => page.root.findAll((node) => node.type === 'p' && node.props['data-probe']).map((node) => node.props['data-probe']);
  const toggle = (id) => page.root.find((node) => node.type === 'button' && node.props['data-part-toggle'] === id);
  const press = async (id) => { await act(async () => { await toggle(id).props.onClick(); await settled(); }); };
  return { page, probes, toggle, press, went, done: async () => act(async () => page.unmount()) };
}

test('mounted: a closed part mounts nothing and reads nothing; Open mounts it; the toggle is a real disclosure', async () => {
  const view = await partsPage();
  try {
    assert.deepEqual(view.probes(), ['cron'], 'a closed part was drawn');
    assert.deepEqual(mountedParts, ['cron'], 'a closed part was mounted');
    assert.equal(view.toggle('logs').props['aria-expanded'], false);
    assert.equal(view.toggle('cron').props['aria-expanded'], true);
    assert.equal(view.toggle('logs').props['aria-controls'], 'domain-part-logs-body');
    await view.press('logs');
    assert.deepEqual(view.probes(), ['cron', 'logs']);
    assert.equal(view.toggle('logs').props['aria-expanded'], true);
    await view.press('logs');
    assert.deepEqual(view.probes(), ['cron'], 'Close kept the part');
  } finally { await view.done(); }
});

test('mounted: a part the address names is open, also when an older link names it (?tab=config, ?tab=php)', async () => {
  for (const [tab, section] of [['config', 'advanced'], ['php', 'site'], ['logs', 'advanced']]) {
    const place = sections.legacyPlace(tab);
    assert.equal(place.section, section, tab);
    const view = await partsPage({ section, part: place.part });
    try {
      assert.ok(view.probes().includes(place.part), `${tab}: the named part stayed closed`);
    } finally { await view.done(); }
  }
  // In Site the AI and program parts stay closed on a PHP site.
  const site = await partsPage({ section: 'site' });
  try {
    assert.deepEqual(site.probes(), ['type', 'apps', 'php', 'redirects']);
  } finally { await site.done(); }
});

test('mounted: the index opens the part it points to', async () => {
  const view = await partsPage();
  try {
    const link = view.page.root.findAll((node) => node.type === 'a' && node.props.href === '#domain-part-git')[0];
    await act(async () => { link.props.onClick({ preventDefault() {} }); await settled(); });
    assert.deepEqual(view.went.at(-1), { place: { section: 'advanced', part: 'git' }, options: { replace: true } });
  } finally { await view.done(); }
  // The address now names it: the section is drawn again with that part, and it is open.
  const named = await partsPage({ part: 'git' });
  try {
    assert.ok(named.probes().includes('git'));
  } finally { await named.done(); }
});

test('mounted: closing a part with unsaved changes asks about that part only; Stay keeps it open', async () => {
  const asked = [];
  globalThis.a132Dirty = ['logs', 'cron'];
  const view = await partsPage({ part: 'logs' });
  try {
    globalThis.confirm = (text, request) => { asked.push(request); return false; };
    await view.press('logs');
    assert.equal(asked.length, 1, 'closing a part with unsaved changes did not ask');
    assert.deepEqual(asked[0].lost, ['1 change on this page that was not saved'], 'the question counted another part’s changes');
    assert.ok(view.probes().includes('logs'), 'Stay closed the part');
    globalThis.confirm = (text, request) => { asked.push(request); return true; };
    await view.press('logs');
    assert.ok(!view.probes().includes('logs'), 'Leave kept the part open');
    // The other part's changes are still there: closing cron asks again.
    globalThis.confirm = (text, request) => { asked.push(request); return false; };
    await view.press('cron');
    assert.equal(asked.length, 3, 'giving up one part’s changes gave up another’s');
    globalThis.a132Dirty = [];
    asked.length = 0;
  } finally {
    await view.done();
    delete globalThis.a132Dirty;
  }
  // Nothing unsaved: closing asks nothing.
  const quiet = await partsPage();
  try {
    globalThis.confirm = (text, request) => { asked.push(request); return false; };
    await quiet.press('cron');
    assert.equal(asked.length, 0);
    assert.deepEqual(quiet.probes(), []);
  } finally { await quiet.done(); }
});
