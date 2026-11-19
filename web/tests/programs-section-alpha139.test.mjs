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

test('the section holds the programs card and the AI card under the rights the cards had in Site, for every role and site kind', () => {
  for (const role of ROLES) {
    for (const projectType of HOSTED) {
      const list = build({ role, projectType });
      const leads = { runtime_app: ['programs', 'ai'], service_app: ['ai', 'programs'] }[projectType] ?? ['programs', 'ai'];
      assert.deepEqual(partsOf(list, 'programs'), leads, `${role} ${projectType}`);
      // Site never draws them again.
      const site = partsOf(list, 'site') ?? [];
      assert.ok(!site.includes('programs') && !site.includes('ai'), `${role} ${projectType}: Site still holds a moved card`);
      // Not a long section: its cards do not open and close.
      for (const part of list.find((section) => section.id === 'programs').parts) assert.equal(part.startsOpen, undefined);
    }
    // One right each, as the cards had.
    assert.deepEqual(partsOf(build({ role, closedRights: ['service_apps'] }), 'programs'), ['programs']);
    assert.deepEqual(partsOf(build({ role, closedRights: ['program_apps', 'python_apps'] }), 'programs'), ['ai']);
    assert.deepEqual(partsOf(build({ role, closedRights: ['program_apps'] }), 'programs'), ['programs', 'ai'], 'Python apps alone keep the programs card');
    // Nothing to act on: no section, not a faded one.
    for (const projectType of ['php', 'static', 'node', 'proxy', 'forwarding']) {
      assert.equal(build({ role, projectType, closedRights: RIGHTS }).find((section) => section.id === 'programs'), undefined, `${role} ${projectType}`);
    }
    // What runs stays manageable after its right closed (D-042 P5), and leads.
    assert.deepEqual(partsOf(build({ role, projectType: 'runtime_app', closedRights: RIGHTS }), 'programs'), ['programs']);
    assert.deepEqual(partsOf(build({ role, projectType: 'service_app', closedRights: RIGHTS }), 'programs'), ['ai']);
  }
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
  export const ${name} = (props) => React.createElement('article', { 'data-card': '${id}', 'data-domain': props.domainId });
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

let programsURL = shared.compile('components/domain/sections/ProgramsSection.tsx', { '/SectionParts': sectionPartsURL });
programsURL = replaceLazyImport(programsURL, '../../DomainRuntimeApps', card('DomainRuntimeApps', 'programs'));
programsURL = replaceLazyImport(programsURL, '../../DomainServiceApps', card('DomainServiceApps', 'ai'));
const { ProgramsSection } = await import(programsURL);

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
async function mount(View, { role = 'admin', projectType = 'php', id, closedRights = [], part = null }) {
  globalThis.document = { addEventListener() {}, removeEventListener() {}, visibilityState: 'visible', getElementById: () => null };
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
  return { page, cards, index, went, done: () => act(async () => page.unmount()) };
}

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

test('mounted: Programs draws both cards at once, the site\'s own first, with the index between them', async () => {
  for (const [projectType, order] of [['php', ['programs', 'ai']], ['runtime_app', ['programs', 'ai']], ['service_app', ['ai', 'programs']]]) {
    const view = await mount(ProgramsSection, { id: 'programs', projectType });
    try {
      assert.deepEqual(view.cards(), order, projectType);
      assert.deepEqual(view.index(), order.map((id) => `#domain-part-${id}`), projectType);
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
