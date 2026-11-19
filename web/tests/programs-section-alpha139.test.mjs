// alpha.139: a domain's programs have their own section in the row, right
// after Site (owner, 19 Nov 2026: "programs under Site did not feel right;
// put them in the menu, easier to reach"). It holds the two cards Site held,
// unchanged — programs and Python apps, and AI & automation — with the
// conditions they had there; Site keeps what the site is. Every older
// address of the cards (?tab=programs, ?tab=ai, and ?section=site&part=… of
// alpha.132–138) opens them in the new section. A site that IS a program or
// a service app still leads with its own card (alpha.133), now in Programs,
// and the Overview's Programs tile says what runs there.
//
// alpha.139: Programlar satırda Site'ın hemen ardında kendi bölümündedir;
// eski adresler oraya götürür; Site yalnız sitenin ne olduğunu tutar.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule, reactURL, replaceLazyImport, sharedLayer } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
const sections = await import(compileSource('components/domain/sections.ts', (specifier) => { throw new Error(`sections.ts imports ${specifier}`); }));
const server = { mail: true, databases: true };
const build = (overrides) => sections.buildSections({ role: 'customer', projectType: 'php', closedRights: [], server, ...overrides });
const partsOf = (list, id) => list.find((section) => section.id === id)?.parts.map((part) => part.id);
const RIGHTS = ['program_apps', 'python_apps', 'service_apps'];
const ROLES = ['admin', 'reseller', 'customer'];
const HOSTED = ['php', 'static', 'node', 'proxy', 'forwarding', 'runtime_app', 'service_app'];

// --- Where and when the section is drawn --------------------------------------------

test('Programs stands right after Site in the one order, with its words and icon', () => {
  assert.equal(sections.SECTION_ORDER.indexOf('programs'), sections.SECTION_ORDER.indexOf('site') + 1);
  assert.equal(sections.SECTION_ORDER.length, 10);
  assert.equal(enScreens['domain.section.programs'], 'Programs');
  assert.equal(trScreens['domain.section.programs'], 'Programlar');
  assert.match(read('../src/lib/domainSectionIcons.ts'), /^ {4}programs: Cpu,$/m);
});

// What the section draws, per site type (a program site and an AI app site
// are the ones that already run one) and per plan. A card that could only
// offer a create or install the server refuses on this type is not drawn; a
// section left without cards is absent, not faded. Written out by hand, not
// derived from the rule it checks.
// Site türü ve plana göre bölümün çizdikleri; elle yazılmıştır.
const PLANS = {
  'all rights': [],
  'no AI apps': ['service_apps'],
  'no programs': ['program_apps', 'python_apps'],
  'Python apps only': ['program_apps', 'service_apps'],
  'no rights': RIGHTS,
};
const BOTH = { 'all rights': ['programs', 'ai'], 'no AI apps': ['programs'], 'no programs': ['ai'], 'Python apps only': ['programs'], 'no rights': undefined };
const EXPECTED = {
  php: BOTH, static: BOTH, proxy: BOTH, forwarding: BOTH,
  // Neither can be added to a Node.js site (the server refuses both).
  node: { 'all rights': undefined, 'no AI apps': undefined, 'no programs': undefined, 'Python apps only': undefined, 'no rights': undefined },
  // The site runs a program: its card only, also after its right closed (D-042 P5).
  runtime_app: { 'all rights': ['programs'], 'no AI apps': ['programs'], 'no programs': ['programs'], 'Python apps only': ['programs'], 'no rights': ['programs'] },
  // The site runs an AI app: its card only.
  service_app: { 'all rights': ['ai'], 'no AI apps': ['ai'], 'no programs': ['ai'], 'Python apps only': ['ai'], 'no rights': ['ai'] },
};

test('what Programs draws, per site type x what already runs x plan x role: nothing whose press the server would refuse', () => {
  for (const role of ROLES) {
    for (const projectType of HOSTED) {
      for (const [plan, closedRights] of Object.entries(PLANS)) {
        const list = build({ role, projectType, closedRights });
        const where = `${role}, ${projectType}, ${plan}`;
        assert.deepEqual(partsOf(list, 'programs'), EXPECTED[projectType][plan], where);
        // Absent, never faded.
        assert.ok(!list.some((section) => section.id === 'programs' && section.notInPlan), where);
        // Site never draws them again.
        const site = partsOf(list, 'site') ?? [];
        assert.ok(!site.includes('programs') && !site.includes('ai'), `${where}: Site still holds a moved card`);
        // Not a long section: its cards do not open and close.
        for (const part of list.find((section) => section.id === 'programs')?.parts ?? []) assert.equal(part.startsOpen, undefined, where);
      }
    }
  }
});

// The screen's rule and the server's refusal come from one table each side;
// this reads the server's conditions from its source and fails when they
// part. The server stays the second line of defence (unchanged).
// Ekranın kuralı ile sunucunun reddi ayrışırsa bu test düşer.
const refusedTypes = (file, anchor) => {
  const source = read(`../../cmd/panel/${file}`);
  const at = source.indexOf(anchor);
  assert.ok(at >= 0, `${file}: ${anchor} not found`);
  const line = source.slice(at, source.indexOf('\n', at));
  return [...line.matchAll(/site\.ProjectType == "([a-z_]+)"/g)].map((match) => match[1]).sort();
};
test('the screen refuses a new program or AI app on exactly the site types the server refuses', () => {
  const server = {
    // runtime_apps_handlers.go, create: `has` (an AI app installed) || node || service_app.
    programs: refusedTypes('runtime_apps_handlers.go', '} else if has || site.ProjectType == "node"'),
    // service_apps_handlers.go, install: node || service_app, then a program
    // on the domain (alpha.139) || runtime_app.
    ai: [
      ...refusedTypes('service_apps_handlers.go', 'if site.ProjectType == "node" || site.ProjectType == "service_app" {'),
      ...refusedTypes('service_apps_handlers.go', '} else if hasProgram || site.ProjectType == "runtime_app" {'),
    ].sort(),
  };
  assert.deepEqual(server.programs, ['node', 'service_app']);
  assert.deepEqual(server.ai, ['node', 'runtime_app', 'service_app']);
  for (const kind of ['programs', 'ai']) {
    assert.deepEqual([...sections.NEW_REFUSED_ON[kind]].sort(), server[kind], `${kind}: the screen and the server refuse different site types`);
  }
  // The refusal of an app on a program's domain has its own words in both
  // languages, not the server's sentence, and says where the program is removed.
  const go = read('../../cmd/panel/service_apps_handlers.go');
  const said = /func serviceAppProgramRefusal\(appName string\) string \{\s*return "([^"]+)"/.exec(go)[1];
  for (const catalogue of [enScreens, trScreens]) {
    const words = catalogue['err.SERVICE_APP.program_created'];
    assert.ok(words && !words.startsWith(said), 'program_created');
  }
  assert.match(enScreens['err.SERVICE_APP.program_created'], /Programs section/);
  assert.match(trScreens['err.SERVICE_APP.program_created'], /Programlar bölümünden/);
  // Every reason the install, update and removal write has words in both languages.
  for (const m of go.matchAll(/writeServiceAppRefusal\(w,\s*http\.Status\w+,\s*"([a-z_]+)"/g)) {
    assert.ok(enScreens[`err.SERVICE_APP.${m[1]}`] && trScreens[`err.SERVICE_APP.${m[1]}`], m[1]);
  }
  for (const m of go.matchAll(/&serviceAppConflict\{"([a-z_]+)"/g)) {
    assert.ok(enScreens[`err.SERVICE_APP.${m[1]}`] && trScreens[`err.SERVICE_APP.${m[1]}`], m[1]);
  }
});

test('a finished type change, install or removal reads the domain list again, so the section follows without a reload', () => {
  assert.match(read('../src/components/HostingTypePanel.tsx'), /await hosting\.retry\(\);[\s\S]{0,400}invalidate\('domains'\);/);
  assert.match(read('../src/components/DomainServiceApps.tsx'), /lastOp\.current\.endsWith\(':running'\) && operation !== lastOp\.current\) invalidate\('domains'\)/);
  assert.match(read('../src/components/DomainRuntimeApps.tsx'), /endsWith\(':running'\) && now !== lastOp\.current\) invalidate\('domains'\)/);
});

test('a DNS-only domain and a team member never get the section, whatever they were granted', () => {
  for (const role of ROLES) assert.equal(build({ role, projectType: 'dnsonly' }).find((section) => section.id === 'programs'), undefined);
  const every = { files: 'manage', php: 'manage', ssl: 'manage', dns: 'manage', mail: 'manage', databases: 'manage', backups: 'manage', cron: 'manage', statistics: 'manage' };
  for (const projectType of HOSTED) {
    const list = sections.buildSections({ role: 'additional_user', projectType, closedRights: [], access: every, server: null });
    assert.ok(!list.some((section) => section.id === 'programs'), projectType);
    assert.ok(!list.some((section) => section.parts.some((part) => part.id === 'programs' || part.id === 'ai')), projectType);
  }
});

test('the part that is the site leads Programs (alpha.133, moved): from the domain\'s kind alone', () => {
  assert.equal(sections.programsLeadPart('runtime_app'), 'programs');
  assert.equal(sections.programsLeadPart('service_app'), 'ai');
  assert.equal(sections.programsLeadPart('php'), null);
  assert.equal(sections.programsLeadPart(undefined), null);
  assert.equal(sections.siteLeadPart, undefined, 'the Site lead is gone');
});

// --- Older addresses ------------------------------------------------------------------

test('every older address of the two cards leads to them in Programs', () => {
  for (const [search, place] of [
    ['tab=programs', { section: 'programs', part: 'programs' }],
    ['tab=ai', { section: 'programs', part: 'ai' }],
    // The canonical address of alpha.132–138, in history, bookmarks and shared links.
    ['section=site&part=programs', { section: 'programs', part: 'programs' }],
    ['section=site&part=ai', { section: 'programs', part: 'ai' }],
    ['section=programs&part=ai', { section: 'programs', part: 'ai' }],
    ['section=programs', { section: 'programs' }],
  ]) {
    assert.deepEqual(sections.requestedPlace(new URLSearchParams(search)), place, search);
  }
  // What stayed in Site stays there.
  assert.deepEqual(sections.requestedPlace(new URLSearchParams('section=site&part=redirects')), { section: 'site', part: 'redirects' });
  assert.deepEqual(sections.requestedPlace(new URLSearchParams('tab=hosting')), { section: 'site' });
  // No source in the page or the server still sends anyone to Site for them.
  for (const file of ['../src/components/domain/sections/OverviewSection.tsx', '../src/components/domain/sections/SiteSection.tsx']) {
    assert.doesNotMatch(read(file), /section: 'site', part: (lead|'programs'|'ai')/, file);
  }
});

test('sentences that name where the cards are name Programs, in both languages', () => {
  for (const key of ['err.RUNTIME_APP.app_created', 'err.SERVICE_APP.app_installed', 'bkc.next.programs', 'bkc.next.setUp', 'bkc.secrets']) {
    assert.match(enScreens[key], /Programs section/, key);
    assert.match(trScreens[key], /Programlar bölüm/, key);
    assert.doesNotMatch(enScreens[key] + trScreens[key], /Site section|Site bölüm|Programs tab|sekme/, key);
  }
  for (const [name, catalogue] of [['en', enScreens], ['tr', trScreens]]) {
    for (const [key, text] of Object.entries(catalogue)) {
      assert.doesNotMatch(text, /Programs tab|Programlar sekme/, `${name} ${key}`);
    }
  }
  // The open/close sentences of the two cards left with their closable frame.
  for (const key of ['domain.about.programs', 'domain.about.ai']) assert.equal(enScreens[key] ?? trScreens[key], undefined, key);
});

// --- The sections, mounted -----------------------------------------------------------

const icons = new Set(['AlertTriangle', 'XCircle']);
for (const file of ['components/ui.tsx', 'components/domain/SectionParts.tsx', 'lib/domainSectionIcons.ts']) {
  for (const match of read(`../src/${file}`).matchAll(/import\s*\{([^}]+)\}\s*from 'lucide-react'/g)) {
    for (const item of match[1].split(',')) {
      const name = item.replace(/\btype\b/, '').trim();
      if (name && name !== 'LucideIcon') icons.add(name);
    }
  }
}
const stub = dataModule(`
  import React from '${reactURL}';
  ${[...icons].map((name) => `export const ${name} = () => null;`).join('\n')}
  export const useI18n = () => ({ t: (key, vars) => (vars && vars.kind ? key + ':' + vars.kind : key), locale: 'en' });
  export const useNavigate = () => () => true;
`);
const shared = sharedLayer(stub);
const sectionsURL = compileSource('components/domain/sections.ts', () => stub);
const sectionPartsURL = shared.compile('components/domain/SectionParts.tsx', { '/sections': sectionsURL });
// Each card stands in as one element that names it.
const card = (name, id) => dataModule(`
  import React from '${reactURL}';
  export const ${name} = (props) => React.createElement('article', { 'data-card': '${id}', 'data-domain': props.domainId, 'data-offer-new': String(props.offerNew ?? true) });
`);

let siteURL = shared.compile('components/domain/sections/SiteSection.tsx', {
  '/SectionParts': sectionPartsURL,
  '/HostingTypePanel': card('HostingTypePanel', 'type'),
  '/DomainPHPSettings': card('DomainPHPSettings', 'php'),
});
siteURL = replaceLazyImport(siteURL, '../../DomainAppsPanel', card('DomainAppsPanel', 'apps'));
siteURL = replaceLazyImport(siteURL, '../../WordPressToolsPanel', card('WordPressToolsPanel', 'wordpress'));
siteURL = replaceLazyImport(siteURL, '../../DomainSiteFeatures', card('DomainSiteRedirects', 'redirects'));
const { SiteSection } = await import(siteURL);

let programsURL = shared.compile('components/domain/sections/ProgramsSection.tsx', {
  '/SectionParts': sectionPartsURL,
  '/sections': sectionsURL,
  '/lib/runtimeApps': shared.compile('lib/runtimeApps.ts'),
  '/lib/serviceApps': shared.compile('lib/serviceApps.ts'),
});
programsURL = replaceLazyImport(programsURL, '../../DomainRuntimeApps', card('DomainRuntimeApps', 'programs'));
programsURL = replaceLazyImport(programsURL, '../../DomainServiceApps', card('DomainServiceApps', 'ai'));
const { ProgramsSection } = await import(programsURL);

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
// What the domain holds, as its two cards' addresses answer (SAMPLE shapes).
const runtimeView = (app) => ({ domain: 'example.com', suspended: false, has_site: true, project_type: 'service_app', certificate: true, disk_full: false,
  sandbox: { known: true, level: 'full', gaps: [] }, kinds: [], app, operation: null, port_range: [30000, 34999] });
const PROGRAM = { id: 61, kind: 'program', entry: 'server', args: [], health_path: '/', port: 30000, state: 'running', deployed: true, url: 'https://example.com/',
  folder: '/var/www/vhosts/example.com/app', upload_dir: 'app/bin', unit: 'celikrt-61.service', env: [], states: [], created_at: '2026-11-03 10:00:00' };
const serviceView = (install) => ({ domain: 'example.com', suspended: false, has_site: true, project_type: 'runtime_app', certificate: true,
  permission_granted: true, availability_known: true, apps: [], install, operation: null });
const INSTALL = { id: 3, app_id: 'n8n', name: 'n8n', version: '', target_version: '1.70.0', state: 'failed', url: 'https://example.com/', held: false,
  active: false, enabled: false, serving: false, unit: 'celiksvc-3.service', folder: '/var/www/vhosts/example.com/n8n' };
let answers = {};
const asked = [];
async function mount(View, { role = 'admin', projectType = 'php', id, closedRights = [], part = null }) {
  globalThis.document = { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible', getElementById: () => null };
  asked.length = 0;
  globalThis.fetch = async (url) => {
    const path = String(url).split('?')[0];
    asked.push(path);
    const answer = path.endsWith('/runtime-apps') ? answers.runtime ?? runtimeView(null) : path.endsWith('/service-apps') ? answers.service ?? serviceView(null) : undefined;
    if (answer === 'fail' || answer === undefined) return Response.json({ error: 'not answered' }, { status: 500 });
    return Response.json(answer);
  };
  const list = build({ role, projectType, closedRights });
  const parts = list.find((section) => section.id === id)?.parts ?? [];
  const went = [];
  let page;
  await act(async () => {
    page = Renderer.create(React.createElement(View, {
      domain: { id: 7, domain_name: 'example.com', project_type: projectType, php_version: '8.3' },
      parts, part, sections: list, role, isTeamMember: false,
      open: (right) => !closedRights.includes(right), canView: () => true, mailAvailable: true,
      go: (place, options) => { went.push({ place, options }); },
      onCertificateChange() {}, onPhpVersionChange() {}, onReasonEnded() {},
    }));
  });
  for (let i = 0; i < 4; i += 1) await act(async () => { await settled(); });
  const cards = () => page.root.findAll((node) => node.type === 'article').map((node) => node.props['data-card']);
  const index = () => page.root.findAll((node) => node.type === 'nav' && node.props['data-section-index'] === '')
    .flatMap((nav) => nav.findAll((node) => node.type === 'a').map((link) => link.props.href));
  const offers = () => Object.fromEntries(page.root.findAll((node) => node.type === 'article').map((node) => [node.props['data-card'], node.props['data-offer-new']]));
  return { page, cards, index, offers, went, done: () => act(async () => page.unmount()) };
}

// --- A domain that already holds both (installed servers before alpha.139) ----------

const { invalidate } = await import(shared.resolve('/lib/remote'));

test('mounted: an AI app site that still holds a program draws the program too, to manage and remove it, with no form', async () => {
  answers = { runtime: runtimeView(PROGRAM) };
  const view = await mount(ProgramsSection, { id: 'programs', projectType: 'service_app', closedRights: RIGHTS });
  try {
    assert.deepEqual(view.cards(), ['ai', 'programs']);
    assert.deepEqual(view.index(), ['#domain-part-ai', '#domain-part-programs']);
    // Only the create and install parts follow the refusal rule.
    assert.deepEqual(view.offers(), { ai: 'false', programs: 'false' });
    assert.ok(asked.includes('/api/v1/domains/7/runtime-apps'), 'the program was not looked for');
    assert.ok(!asked.includes('/api/v1/domains/7/service-apps'), 'the drawn card was read twice by the section');
    // The program is removed: the domain list is read again, and so is this; the card goes.
    answers = { runtime: runtimeView(null) };
    await act(async () => { invalidate('domains'); await settled(); });
    for (let i = 0; i < 3; i += 1) await act(async () => { await settled(); });
    assert.deepEqual(view.cards(), ['ai']);
  } finally { await view.done(); answers = {}; }

  // Nothing left over: the app's card only, and one read to know it.
  const clean = await mount(ProgramsSection, { id: 'programs', projectType: 'service_app' });
  try {
    assert.deepEqual(clean.cards(), ['ai']);
  } finally { await clean.done(); }
});

test('mounted: a program site that still holds an app record draws that card too; a read that fails draws it rather than hide it', async () => {
  answers = { service: serviceView(INSTALL) };
  const view = await mount(ProgramsSection, { id: 'programs', projectType: 'runtime_app' });
  try {
    assert.deepEqual(view.cards(), ['programs', 'ai']);
    assert.deepEqual(view.offers(), { programs: 'true', ai: 'false' });
  } finally { await view.done(); answers = {}; }
  answers = { runtime: 'fail' };
  const failed = await mount(ProgramsSection, { id: 'programs', projectType: 'service_app' });
  try {
    assert.deepEqual(failed.cards(), ['ai', 'programs'], 'a program that could not be looked for was hidden');
  } finally { await failed.done(); answers = {}; }
});

test('mounted: a site that can take either reads nothing extra and offers both forms', async () => {
  for (const projectType of ['php', 'static', 'proxy', 'forwarding']) {
    const view = await mount(ProgramsSection, { id: 'programs', projectType });
    try {
      assert.deepEqual(view.offers(), { programs: 'true', ai: 'true' }, projectType);
      assert.deepEqual(asked, [], `${projectType}: the section read on its own`);
    } finally { await view.done(); }
  }
});

test('the domain\'s delete refusal sends the reader to where the program is removed, and that place is drawn for it', () => {
  // The server refuses a delete while a program is on the domain (runtime_apps_handlers.go,
  // refuseDomainDeleteWithRuntimeApp, reason app_created); the screen words that reason.
  const go = read('../../cmd/panel/runtime_apps_handlers.go');
  assert.match(go, /func \(p \*Panel\) refuseDomainDeleteWithRuntimeApp[\s\S]{0,400}writeRuntimeAppRefusal\(w, http\.StatusConflict, "app_created"/);
  assert.match(enScreens['err.RUNTIME_APP.app_created'], /Programs section/);
  assert.match(trScreens['err.RUNTIME_APP.app_created'], /Programlar bölümünden/);
  // Every site type a program can be on has the Programs section: where the
  // program runs or an app took its place, whatever the plan; elsewhere while
  // the plan includes programs.
  for (const projectType of ['runtime_app', 'service_app']) {
    assert.ok(build({ projectType, closedRights: RIGHTS }).some((section) => section.id === 'programs'), projectType);
  }
  for (const projectType of ['php', 'static', 'proxy', 'forwarding']) {
    assert.ok(build({ projectType, closedRights: ['service_apps'] }).some((section) => section.id === 'programs'), projectType);
  }
});

test('mounted: Site no longer draws the programs or the AI card, for any site kind; its index lists what stays', async () => {
  const php = await mount(SiteSection, { id: 'site' });
  try {
    assert.deepEqual(php.cards(), ['type', 'apps', 'wordpress', 'php', 'redirects']);
    assert.deepEqual(php.index(), ['#domain-part-type', '#domain-part-apps', '#domain-part-php', '#domain-part-redirects']);
  } finally { await php.done(); }
  for (const projectType of ['runtime_app', 'service_app']) {
    // Two cards stay; the index stays, as in Files with its two.
    const view = await mount(SiteSection, { id: 'site', projectType });
    try {
      assert.deepEqual(view.cards(), ['type', 'redirects'], projectType);
      assert.deepEqual(view.index(), ['#domain-part-type', '#domain-part-redirects'], projectType);
    } finally { await view.done(); }
  }
  const source = read('../src/components/domain/sections/SiteSection.tsx');
  assert.doesNotMatch(source, /DomainRuntimeApps|DomainServiceApps/);
});

test('mounted: Programs draws both cards at once on a site that can take either; a site that runs one shows its own card only', async () => {
  for (const [projectType, order] of [['php', ['programs', 'ai']], ['static', ['programs', 'ai']], ['runtime_app', ['programs']], ['service_app', ['ai']]]) {
    const view = await mount(ProgramsSection, { id: 'programs', projectType });
    try {
      assert.deepEqual(view.cards(), order, projectType);
      // One card: no index.
      assert.deepEqual(view.index(), order.length > 1 ? order.map((id) => `#domain-part-${id}`) : [], projectType);
      // Not closable: no open/close button on either card.
      assert.equal(view.page.root.findAll((node) => node.type === 'button' && node.props['data-part-toggle']).length, 0);
      for (const id of order) assert.equal(view.page.root.find((node) => node.type === 'article' && node.props['data-card'] === id).props['data-domain'], 7);
    } finally { await view.done(); }
  }
  // One card only: no index.
  const one = await mount(ProgramsSection, { id: 'programs', closedRights: ['service_apps'] });
  try {
    assert.deepEqual(one.cards(), ['programs']);
    assert.deepEqual(one.index(), []);
  } finally { await one.done(); }
  // The index moves within the section, in place.
  const view = await mount(ProgramsSection, { id: 'programs' });
  try {
    const link = view.page.root.findAll((node) => node.type === 'a' && node.props.href === '#domain-part-ai')[0];
    await act(async () => { link.props.onClick({ preventDefault() {} }); await settled(); });
    assert.deepEqual(view.went.at(-1), { place: { section: 'programs', part: 'ai' }, options: { replace: true } });
  } finally { await view.done(); }
});

let overviewURL = shared.compile('components/domain/sections/OverviewSection.tsx', {
  '/SectionParts': sectionPartsURL,
  '/sections': sectionsURL,
  '/DomainSSLOverviewCard': dataModule('export const DomainSSLOverviewCard = () => null;'),
  '/kit/words': dataModule('export const siteKindWord = (t, code) => "kind(" + code + ")";'),
  '/lib/domainSectionIcons': compileSource('lib/domainSectionIcons.ts', (specifier) => (specifier.endsWith('/sections') ? sectionsURL : stub)),
});
const { OverviewSection } = await import(overviewURL);

test('mounted: the Overview has a Programs tile; on a program or app site it says what runs there and opens Programs', async () => {
  for (const [projectType, runs] of [['php', false], ['runtime_app', true], ['service_app', true]]) {
    const view = await mount(OverviewSection, { id: 'overview', projectType });
    try {
      const tiles = view.page.root.findAll((node) => node.type === 'button' && node.props['data-overview-tile']);
      assert.deepEqual(tiles.map((tile) => tile.props['data-overview-tile']).slice(0, 2), ['site', 'programs'], projectType);
      const said = view.page.root.findAll((node) => node.props['data-overview-runs'] === '');
      assert.equal(said.length, runs ? 1 : 0, projectType);
      if (runs) {
        assert.ok(tiles[1].findAll((node) => node.props['data-overview-runs'] === '').length === 1, `${projectType}: the line is not on the Programs tile`);
        assert.equal(said[0].props.children, `domain.overview.siteRuns:kind(${projectType})`);
      }
      await act(async () => { tiles[1].props.onClick(); await settled(); });
      assert.deepEqual(view.went.at(-1).place, { section: 'programs' }, projectType);
      await act(async () => { tiles[0].props.onClick(); await settled(); });
      assert.deepEqual(view.went.at(-1).place, { section: 'site' }, `${projectType}: the Site tile leads to Site`);
    } finally { await view.done(); }
  }
  assert.doesNotMatch(read('../src/components/domain/sections/OverviewSection.tsx'), /useRemote|fetch\(/, 'the Overview reads nothing new');
});
