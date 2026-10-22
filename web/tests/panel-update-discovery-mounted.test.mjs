// alpha.96 (D-035, owner request 2026-10-17): the Panel looks for a newer
// signed release by itself every 12 hours and keeps the result. These tests
// mount the real reader of that stored result (src/lib/panelUpdateDiscovery.ts)
// under the sidebar, the Dashboard row and the update card, and check what the
// owner sees: a quiet link and row for an administrator when an update was
// found, nothing for anyone else, nothing outside the card for a check that
// failed, a card that opens knowing the stored result without asking the
// release server, one status line, one filled button, and the update itself
// only on the owner's press.
//
// Panel 12 saatte bir yeni imzali surumu kendisi arar; bu testler sahibin
// gordugunu denetler: yalniz yoneticiye sakin baglanti ve satir, basarisiz
// kontrol icin kart disinda hicbir sey, dugmeye basilmadan bilen kart.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { remoteURL } from './fixtures/shared-layer.mjs';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from './locale-catalogue.mjs';
import { trScreens } from './locale-catalogue.mjs';
import { enServerScreens } from './locale-catalogue.mjs';
import { trServerScreens } from './locale-catalogue.mjs';

globalThis.document ??= Object.assign(new EventTarget(), { visibilityState: 'visible' });
globalThis.window ??= { setTimeout: (fn, ms) => globalThis.setTimeout(fn, ms), clearTimeout: (id) => globalThis.clearTimeout(id) };

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const jsxRuntimeURL = pathToFileURL(require.resolve('react/jsx-runtime')).href;
const moduleURL = source => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
function compileURL(relativePath, imports = {}) {
    const compiled = ts.transpileModule(readFileSync(new URL(relativePath, import.meta.url), 'utf8'), {
        compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    }).outputText;
    return moduleURL(compiled.replace(/from ['"]([^'"]+)['"]/g, (_match, path) => {
        const target = path === 'react' ? reactURL : path === 'react/jsx-runtime' ? jsxRuntimeURL : imports[path];
        assert.ok(target, `unmapped import in ${relativePath}: ${path}`);
        return `from '${target}'`;
    }));
}

const catalogues = {
    en: { ...en, ...enScreens, ...enServerScreens },
    tr: { ...tr, ...trScreens, ...trServerScreens },
};
const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => (vars && vars[name] !== undefined ? String(vars[name]) : all));
const say = (key, vars, locale = 'en') => fill(catalogues[locale][key] ?? key, vars);

// One real reader shared by every mounted component, as in the application.
const discoveryURL = compileURL('../src/lib/panelUpdateDiscovery.ts');
const discovery = await import(discoveryURL);

const stubURL = moduleURL(`
    import React from '${reactURL}';
    const Icon = name => props => React.createElement('i', { ...props, 'data-icon': name });
    export const LogOut = Icon('LogOut'), ChevronDown = Icon('ChevronDown'), Menu = Icon('Menu'), KeyRound = Icon('KeyRound'), Mail = Icon('Mail'), UserCheck = Icon('UserCheck'), CircleUser = Icon('CircleUser'), User = Icon('User');
    export const DownloadCloud = Icon('DownloadCloud'), AlertTriangle = Icon('AlertTriangle'), CheckCircle2 = Icon('CheckCircle2'), Loader2 = Icon('Loader2'), RefreshCw = Icon('RefreshCw'), Info = Icon('Info');
    export const BrandMark = Icon('BrandMark');
    export const BuildCommit = ({ commit }) => 'build ' + commit.slice(0, 12);
    export const ThemeSwitcher = () => null, SkinSwitcher = () => null, LanguageSwitcher = () => null, ChangePasswordModal = () => null;
    export const Button = ({ children, variant, ...props }) => React.createElement('button', { ...props, 'data-variant': variant ?? 'secondary' }, children);
    export const useAuth = () => globalThis.discoveryFixture.auth;
    const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => (vars && vars[name] !== undefined ? String(vars[name]) : all));
    export const useI18n = () => ({ t: (key, vars) => fill(globalThis.discoveryFixture.strings[key] ?? key, vars), locale: globalThis.discoveryFixture.locale, screensReady: true, screensFailed: false });
    export const Link = ({ to, children, ...props }) => React.createElement('a', { ...props, href: to }, children);
    export const DesktopPageHeaderTargetContext = React.createContext(null);
    export const navGroups = [{ id: 'hosting', labelKey: 'nav.hosting' }];
    export const navItemsForRole = () => [{ id: 'domains', group: 'hosting', labelKey: 'nav.domains', icon: Icon('Globe'), countKey: 'domains' }];
    export const api = { getServices: async () => ({ components: [] }), logout: async () => {} };
    export const publishComponentCensus = () => {}, useComponentCensus = () => null;
`);
const layoutURL = compileURL('../src/components/Layout.tsx', {
    ...Object.fromEntries(['./BrandMark', './BuildCommit', 'lucide-react', '../lib/api', '../auth/AuthContext', '../i18n',
        '../nav', '../lib/componentCensus', './ThemeSwitcher', './SkinSwitcher', './LanguageSwitcher',
        './ChangePasswordModal', './pageHeaderSlot', '../router'].map(path => [path, stubURL])),
    '../lib/remote': remoteURL,
    '../lib/infrastructureZones': compileURL('../src/lib/infrastructureZones.ts'),
    '../lib/panelUpdateDiscovery': discoveryURL,
});
const { Layout } = await import(layoutURL);
const { DashboardUpdateNotice } = await import(compileURL('../src/components/PanelUpdateNotice.tsx', {
    'lucide-react': stubURL, '../router': stubURL, '../i18n': stubURL, '../lib/panelUpdateDiscovery': discoveryURL,
}));
const recoveryObservationURL = compileURL('../src/lib/recoveryObservation.ts');
const failureURL = compileURL('../src/lib/systemUpdateFailure.ts');
const outcomeURL = compileURL('../src/lib/systemUpdateOutcome.ts', { './recoveryObservation': recoveryObservationURL, './systemUpdateFailure': failureURL });
const outcome = await import(outcomeURL);
const operationURL = moduleURL(`
    export function createSystemUpdateRequestID() { return 'a'.repeat(32); }
    export function systemUpdateResponseHint() { return 'response-error'; }
    export function useSystemUpdateOperation() { return globalThis.discoveryFixture.operation; }
    export function validUpdateTarget(value) { return !!value && typeof value === 'object' && typeof value.version === 'string'; }
    export function validUpdateVersion(value) { return typeof value === 'string' && value.startsWith('v'); }
`);
const admissionURL = moduleURL(`
    export function unverifiedHostMutationReadiness() { return { ready: false, code: 'HOST_MUTATION_UNAVAILABLE', reason: 'state_unverified' }; }
    export function fetchHostMutationReadiness() { globalThis.discoveryFixture.readinessReads += 1; return Promise.resolve(globalThis.discoveryFixture.readiness); }
    export async function runHostMutationAdmission(read, onReady) { const r = await read(); if (r.ready) await onReady(); return r; }
`);
const { PanelUpdateCard } = await import(compileURL('../src/components/PanelUpdateCard.tsx', {
    'lucide-react': stubURL, './ui': stubURL, '../i18n': stubURL,
    '../lib/apiError': moduleURL('export async function readApiError() { return {}; } export function apiErrorText() { return "api-error"; }'),
    '../lib/panelUpdateAdmission': admissionURL,
    './SystemUpdateOperation': operationURL,
    '../lib/systemUpdateOutcome': outcomeURL,
    './SystemUpdateSteps': moduleURL('export function SystemUpdateSteps() { return null; } export function systemUpdateStepsAnnouncement() { return ""; }'),
    './BuildCommit': stubURL,
    '../lib/panelUpdateDiscovery': discoveryURL,
}));

const CURRENT = { version: 'v0.1.0-alpha.95', commit: '5f3cb1a2fa336c529740a20e9a543b8ac086dc8d' };
const TARGET = {
    version: 'v0.1.0-alpha.96', commit: 'b'.repeat(40), sequence: '96', os: 'linux', arch: 'amd64',
    archive_sha256: 'c'.repeat(64), archive_size: '66259462',
};
const HOUR = 3600;
const storedAvailable = (extra = {}) => ({
    auto_check: true, interval_hours: 12, current_version: CURRENT.version, current_commit: CURRENT.commit,
    next_check_in_seconds: 10 * HOUR, expected_restart_seconds: 90, available: true,
    last: { checked_at: '2026-10-17T10:00:00Z', outcome: 'available', automatic: true, target: TARGET, found_at: '2026-10-17T10:00:00Z', current_version: CURRENT.version, current_commit: CURRENT.commit },
    checked_age_seconds: 2 * HOUR, found_age_seconds: 2 * HOUR, ...extra,
});
const storedCurrent = (extra = {}) => ({
    auto_check: true, interval_hours: 12, current_version: CURRENT.version, current_commit: CURRENT.commit,
    next_check_in_seconds: 10 * HOUR, expected_restart_seconds: 90, available: false,
    last: { checked_at: '2026-10-17T10:00:00Z', outcome: 'current', automatic: true, current_version: CURRENT.version, current_commit: CURRENT.commit },
    checked_age_seconds: 2 * HOUR, ...extra,
});
const storedFailed = (extra = {}) => ({
    ...storedCurrent(),
    last: { checked_at: '2026-10-17T10:00:00Z', outcome: 'failed', automatic: true, reason_code: 'release_unverified', current_version: CURRENT.version, current_commit: CURRENT.commit },
    ...extra,
});

function textOf(node) {
    if (node == null || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join(' ');
    return textOf(node.children);
}
const flush = async () => { for (let i = 0; i < 8; i += 1) await Promise.resolve(); };
const storage = new Map();
globalThis.localStorage = {
    getItem: key => (storage.has(key) ? storage.get(key) : null),
    setItem: (key, value) => { storage.set(key, String(value)); },
    removeItem: key => { storage.delete(key); },
};

async function withMounted(element, { role = 'admin', stored = storedAvailable(), locale = 'en', check, readiness = { ready: true } } = {}, inspect) {
    discovery.resetPanelUpdateDiscoveryForTests();
    const previousFetch = globalThis.fetch;
    const fixture = {
        auth: { role, user: { username: role }, logout() {} },
        strings: catalogues[locale], locale, requests: [], readiness, readinessReads: 0,
        operation: { active: false, start: async () => { fixture.starts += 1; return { kind: 'accepted' }; }, tracking: null },
        starts: 0,
    };
    globalThis.discoveryFixture = fixture;
    globalThis.fetch = async (input, options = {}) => {
        const path = String(input);
        fixture.requests.push(`${options.method ?? 'GET'} ${path}`);
        if (path === '/api/v1/panel/version') return Response.json({ ...CURRENT, agent_commit: CURRENT.commit, agent_matches: true, hostname: 'frankfurt.example.test', ipv4: '192.0.2.30' });
        if (path === '/api/v1/domains') return Response.json([]);
        if (path === '/api/v1/dns/infrastructure-zones') return Response.json({ zones: [] });
        if (path === discovery.PANEL_UPDATE_DISCOVERY_URL) {
            if (options.method === 'PUT') {
                const body = JSON.parse(options.body);
                return Response.json({ ...stored, auto_check: body.auto_check, ...(body.auto_check ? {} : { next_check_in_seconds: undefined }) });
            }
            if (stored === null) return new Response("{}", { status: 500 });
            return Response.json(stored);
        }
        if (path === '/api/v1/panel/update/check' && check) return check();
        throw new Error('unexpected request: ' + path);
    };
    let renderer;
    try {
        await act(async () => { renderer = TestRenderer.create(element); await flush(); });
        await act(async () => { await flush(); });
        await inspect(renderer, fixture);
    } finally {
        if (renderer) await act(async () => renderer.unmount());
        globalThis.fetch = previousFetch;
    }
}
const shell = () => React.createElement(Layout, { currentPage: 'domains', onPageChange: () => {} }, React.createElement('h1', null, 'Page'));
const sidebarLinks = renderer => renderer.root.findAll(node => node.type === 'a' && node.props['data-update-available'] !== undefined);
const discoveryReads = fixture => fixture.requests.filter(line => line === 'GET ' + discovery.PANEL_UPDATE_DISCOVERY_URL).length;

// --- The reader -----------------------------------------------------------------------

test('the stored result is decoded on the browser clock from the server\'s ages, and a malformed answer is refused', () => {
    const now = Date.parse('2026-10-17T12:00:00Z');
    const value = discovery.decodePanelUpdateDiscovery(storedAvailable(), now);
    assert.equal(value.target.version, TARGET.version);
    assert.equal(value.last.checkedAt, now - 2 * HOUR * 1000);
    assert.equal(value.foundAt, now - 2 * HOUR * 1000);
    assert.equal(value.nextCheckAt, now + 10 * HOUR * 1000);
    assert.equal(discovery.decodePanelUpdateDiscovery({ ...storedAvailable(), auto_check: false }, now).nextCheckAt, null);
    assert.equal(discovery.decodePanelUpdateDiscovery({ ...storedAvailable(), found_age_seconds: undefined }, now), null);
    assert.equal(discovery.decodePanelUpdateDiscovery({ ...storedAvailable(), last: { ...storedAvailable().last, outcome: 'maybe' } }, now), null);
    assert.equal(discovery.decodePanelUpdateDiscovery({ ...storedAvailable(), last: { ...storedAvailable().last, target: { ...TARGET, os: 'windows' } } }, now), null);
    const failed = discovery.decodePanelUpdateDiscovery(storedFailed(), now);
    assert.equal(failed.target, null);
    assert.equal(failed.last.reason, 'release_unverified');
});

// --- The sidebar ---------------------------------------------------------------------

test('the sidebar shows a quiet link to the update screen for an administrator, only when an update was found', async () => {
    await withMounted(shell(), {}, async (renderer, fixture) => {
        const links = sidebarLinks(renderer);
        assert.equal(links.length, 1);
        assert.equal(links[0].props.href, '/settings?section=updates');
        assert.equal(textOf(links[0]).trim().replace(/\s+/g, ' '), say('panelUpdate.notice.sidebar', { version: TARGET.version }));
        assert.doesNotMatch(links[0].props.className, /warning|amber|danger|red|bg-/, 'an available update is not a warning badge');
        assert.equal(discoveryReads(fixture), 1);
        assert.equal(fixture.requests.filter(line => line.includes('/update/check')).length, 0, 'the sidebar asked the release server');
    });
    await withMounted(shell(), { stored: storedCurrent() }, async (renderer) => {
        assert.equal(sidebarLinks(renderer).length, 0);
    });
    for (const role of ['reseller', 'customer', 'additional_user']) {
        await withMounted(shell(), { role }, async (renderer, fixture) => {
            assert.equal(sidebarLinks(renderer).length, 0, role);
            assert.equal(discoveryReads(fixture), 0, `${role} read the update check`);
        });
    }
});

test('a failed or unknown automatic check shows nothing outside the update card', async () => {
    await withMounted(React.createElement(React.Fragment, null, shell(), React.createElement(DashboardUpdateNotice, { admin: true })), { stored: storedFailed() }, async (renderer) => {
        assert.equal(sidebarLinks(renderer).length, 0);
        assert.equal(renderer.root.findAll(node => node.props && node.props['data-dashboard-update'] !== undefined).length, 0);
        assert.ok(!textOf(renderer.toJSON()).includes(say('panelUpdate.discoveryReason.release_unverified')));
    });
    // A release found earlier stays offered after a failed check: the panel
    // still knows it, and the failure itself is said only in the card.
    const carried = storedFailed({ available: true, found_age_seconds: 14 * HOUR, last: { ...storedFailed().last, target: TARGET, found_at: '2026-10-16T22:00:00Z' } });
    await withMounted(shell(), { stored: carried }, async (renderer) => {
        assert.equal(sidebarLinks(renderer).length, 1);
        assert.ok(!textOf(renderer.toJSON()).includes(say('panelUpdate.discoveryReason.release_unverified')));
    });
});

// --- The Dashboard row ---------------------------------------------------------------

test('the Dashboard row is one quiet line with "Review and update"; "Remind me later" hides it for 24 hours in this browser', async () => {
    storage.clear();
    const row = renderer => renderer.root.findAll(node => node.type === 'section' && node.props['data-dashboard-update'] !== undefined);
    await withMounted(React.createElement(DashboardUpdateNotice, { admin: true }), {}, async (renderer) => {
        assert.equal(row(renderer).length, 1);
        const text = textOf(row(renderer)[0]).replace(/\s+/g, ' ');
        assert.ok(text.includes(say('panelUpdate.notice.dashboard', { version: TARGET.version })), text);
        const review = row(renderer)[0].findByType('a');
        assert.equal(review.props.href, '/settings?section=updates');
        assert.equal(textOf(review).trim(), say('panelUpdate.notice.review'));
        assert.doesNotMatch(row(renderer)[0].props.className, /warning|danger|amber|red/);
        const before = Date.now();
        const later = row(renderer)[0].findByProps({ 'data-update-later': '' });
        assert.equal(textOf(later).trim(), say('panelUpdate.notice.later'));
        await act(async () => later.props.onClick());
        assert.equal(row(renderer).length, 0);
        const saved = JSON.parse(storage.get(discovery.UPDATE_NOTICE_SNOOZE_KEY));
        assert.equal(saved.version, TARGET.version);
        assert.ok(saved.until >= before + 24 * HOUR * 1000 && saved.until <= Date.now() + 24 * HOUR * 1000);
    });
    // Mounted again within the 24 hours: still hidden.
    await withMounted(React.createElement(DashboardUpdateNotice, { admin: true }), {}, async (renderer) => {
        assert.equal(row(renderer).length, 0);
    });
    // After the 24 hours, or for another version: shown again.
    storage.set(discovery.UPDATE_NOTICE_SNOOZE_KEY, JSON.stringify({ version: TARGET.version, until: Date.now() - 1 }));
    await withMounted(React.createElement(DashboardUpdateNotice, { admin: true }), {}, async (renderer) => {
        assert.equal(row(renderer).length, 1);
    });
    storage.set(discovery.UPDATE_NOTICE_SNOOZE_KEY, JSON.stringify({ version: 'v0.1.0-alpha.95', until: Date.now() + HOUR * 1000 }));
    await withMounted(React.createElement(DashboardUpdateNotice, { admin: true }), {}, async (renderer) => {
        assert.equal(row(renderer).length, 1);
    });
    // Blocked storage: the row still works for this page.
    const blocked = globalThis.localStorage;
    globalThis.localStorage = { getItem() { throw new Error('blocked'); }, setItem() { throw new Error('blocked'); } };
    try {
        await withMounted(React.createElement(DashboardUpdateNotice, { admin: true }), {}, async (renderer) => {
            assert.equal(row(renderer).length, 1);
            await act(async () => row(renderer)[0].findByProps({ 'data-update-later': '' }).props.onClick());
            assert.equal(row(renderer).length, 0);
        });
    } finally { globalThis.localStorage = blocked; storage.clear(); }
    // Not an administrator: nothing, and nothing is read.
    await withMounted(React.createElement(DashboardUpdateNotice, { admin: false }), { role: 'customer' }, async (renderer, fixture) => {
        assert.equal(row(renderer).length, 0);
        assert.equal(discoveryReads(fixture), 0);
    });
});

// --- The update card ------------------------------------------------------------------

const statusOf = renderer => renderer.root.find(node => node.type === 'div' && node.props['data-update-status'] !== undefined);
const statusText = renderer => textOf(statusOf(renderer)).replace(/\s+/g, ' ').trim();
const primaries = renderer => renderer.root.findAll(node => node.type === 'button' && node.props['data-variant'] === 'primary');

for (const locale of ['en', 'tr']) {
    test(`${locale}: the card opens knowing the stored result, says it once, and offers one filled button`, async () => {
        await withMounted(React.createElement(PanelUpdateCard), { locale }, async (renderer, fixture) => {
            assert.equal(fixture.requests.filter(line => line.includes('/update/check')).length, 0, 'the card asked the release server on its own');
            const ageText = discovery.relativeAge(Date.now() - 2 * HOUR * 1000, Date.now(), locale, say('panelUpdate.status.justNow', {}, locale));
            assert.equal(statusOf(renderer).props['data-update-status'], 'available');
            assert.ok(statusText(renderer).startsWith(say('panelUpdate.status.available', { version: TARGET.version, age: ageText }, locale)), statusText(renderer));
            // One primary button, the update itself; the check is outlined.
            assert.equal(primaries(renderer).length, 1);
            assert.equal(textOf(primaries(renderer)[0]).trim(), say('panelUpdate.start', { version: TARGET.version }, locale));
            assert.equal(primaries(renderer)[0].props.disabled, false, 'a ready server cannot start');
            const whole = textOf(renderer.toJSON());
            assert.ok(!whole.includes('A signed update is available.') && !whole.includes('İmzalı bir güncelleme bulundu.'), 'the old second "available" line is back');
            assert.equal(whole.split(TARGET.version + ' ').length - 1 <= 3, true);
            // The alpha note is plain text in "Before you update", not an amber box.
            const before = renderer.root.find(node => node.props && node.props['data-update-before'] !== undefined);
            assert.ok(textOf(before).includes(say('panelUpdate.before.backup', {}, locale)));
            assert.ok(textOf(before).includes(say('panelUpdate.before.restart', { seconds: 90 }, locale)));
            assert.ok(textOf(before).includes(say('panelUpdate.before.manual', {}, locale)));
            assert.equal(renderer.root.findAll(node => node.type === 'div' && /warning-mark/.test(node.props.className ?? '')).length, 0, 'an amber box on a ready server');
            // Technical details are collapsed.
            const details = renderer.root.find(node => node.type === 'details');
            assert.ok(!details.props.open);
            assert.ok(textOf(details).includes(TARGET.archive_sha256));
            // The readiness read ran by itself, once.
            assert.equal(fixture.readinessReads, 1);
            // The switch says what it does.
            const auto = renderer.root.find(node => node.props && node.props['data-update-auto'] !== undefined);
            assert.ok(textOf(auto).includes(say('panelUpdate.auto.help', {}, locale)));
            assert.equal(auto.findByType('input').props.checked, true);
        });
    });
}

test('the card says the latest version with when and how it was checked, and when the next check runs', async () => {
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedCurrent() }, async (renderer) => {
        const ageText = discovery.relativeAge(Date.now() - 2 * HOUR * 1000, Date.now(), 'en', 'just now');
        assert.equal(statusOf(renderer).props['data-update-status'], 'latest');
        assert.ok(statusText(renderer).includes(say('panelUpdate.status.current', { age: ageText })), statusText(renderer));
        assert.ok(statusText(renderer).includes(say('panelUpdate.status.next', { time: '' }).split(' {')[0].slice(0, 20)));
        assert.equal(primaries(renderer).length, 0, 'nothing to update to, nothing filled');
    });
});

test('a failed automatic check is calm information in the card, with when the next one runs and that the owner can check now', async () => {
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedFailed() }, async (renderer) => {
        const ageText = discovery.relativeAge(Date.now() - 2 * HOUR * 1000, Date.now(), 'en', 'just now');
        const status = statusOf(renderer);
        assert.equal(status.props['data-update-status'], 'info');
        assert.deepEqual(status.findAll(node => node.props && node.props['data-icon'] !== undefined).map(node => node.props['data-icon']), ['Info']);
        assert.ok(statusText(renderer).includes(say('panelUpdate.status.lastFailed', { age: ageText, reason: say('panelUpdate.discoveryReason.release_unverified') })), statusText(renderer));
        const next = renderer.root.find(node => node.props && node.props['data-update-next'] !== undefined);
        assert.match(textOf(next), /you can also check now/);
        assert.equal(renderer.root.findAll(node => node.props && node.props['data-message-tone'] !== undefined).length, 0);
    });
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedFailed({ last: { ...storedFailed().last, reason_code: 'timeout' } }) }, async (renderer) => {
        assert.ok(statusText(renderer).includes(say('panelUpdate.discoveryReason.timeout')), 'a timeout is not known, not a failure of the release');
    });
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedFailed({ auto_check: false }) }, async (renderer) => {
        assert.ok(textOf(renderer.root.find(node => node.props && node.props['data-update-next'] !== undefined)).includes(say('panelUpdate.status.off')));
    });
});

test('only the explicit button asks the release server, and its result is stored and shared', async () => {
    let checks = 0;
    const check = () => { checks += 1; return Response.json({ supported: true, available: false, current_version: CURRENT.version, current_commit: CURRENT.commit }); };
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedCurrent(), check }, async (renderer, fixture) => {
        assert.equal(checks, 0);
        const button = renderer.root.findAll(node => node.type === 'button')[0];
        assert.equal(textOf(button).trim(), say('panelUpdate.check'));
        assert.equal(button.props['data-variant'], 'secondary');
        const readsBefore = discoveryReads(fixture);
        await act(async () => { button.props.onClick(); await flush(); });
        await act(async () => { await flush(); });
        assert.equal(checks, 1);
        assert.equal(discoveryReads(fixture), readsBefore + 1, 'the stored result was not read again after the press');
        assert.equal(statusOf(renderer).props['data-update-status'], 'latest');
        assert.equal(fixture.starts, 0, 'a check started an update');
    });
});

test('the switch is saved with PUT and the card follows the answer; a failed save says so and keeps the setting', async () => {
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedCurrent() }, async (renderer, fixture) => {
        const input = renderer.root.find(node => node.type === 'input' && node.props.role === 'switch');
        await act(async () => { input.props.onChange({ currentTarget: { checked: false } }); await flush(); });
        await act(async () => { await flush(); });
        assert.ok(fixture.requests.includes('PUT ' + discovery.PANEL_UPDATE_DISCOVERY_URL));
        assert.equal(renderer.root.find(node => node.type === 'input' && node.props.role === 'switch').props.checked, false);
        assert.ok(textOf(renderer.root.find(node => node.props && node.props['data-update-next'] !== undefined)).includes(say('panelUpdate.status.off')));
    });
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedCurrent() }, async (renderer) => {
        const failing = globalThis.fetch;
        globalThis.fetch = async (input, options = {}) => (options.method === 'PUT' ? new Response('{}', { status: 500 }) : failing(input, options));
        const input = renderer.root.find(node => node.type === 'input' && node.props.role === 'switch');
        await act(async () => { input.props.onChange({ currentTarget: { checked: false } }); await flush(); });
        assert.equal(renderer.root.findAll(node => node.props && node.props['data-update-auto-error'] !== undefined).length, 1);
        assert.equal(renderer.root.find(node => node.type === 'input' && node.props.role === 'switch').props.checked, true);
    });
});

test('an earlier attempt that stopped before changing anything is neutral information, at most one notice', async () => {
    const stopped = storedAvailable({ previous_attempt: { request_id: 'd'.repeat(32), phase: 'failed', failure_code: 'recovery_runtime_preflight_failed', finished_at: '2026-10-17T09:00:00Z' } });
    await withMounted(React.createElement(PanelUpdateCard), { stored: stopped }, async (renderer) => {
        const notices = renderer.root.findAll(node => node.props && node.props['data-previous-attempt'] !== undefined);
        assert.equal(notices.length, 1);
        assert.equal(notices[0].props['data-previous-attempt'], 'stopped');
        assert.doesNotMatch(notices[0].props.className, /warning/);
        assert.ok(textOf(notices[0]).includes(say('panelUpdate.previousAttempt.stoppedTitle')));
        assert.equal(primaries(renderer).length, 1, 'the notice blocked or doubled the button');
    });
    const rolledBack = storedAvailable({ previous_attempt: { request_id: 'd'.repeat(32), phase: 'recovered', finished_at: '2026-10-17T09:00:00Z' } });
    await withMounted(React.createElement(PanelUpdateCard), { stored: rolledBack }, async (renderer) => {
        const notice = renderer.root.find(node => node.props && node.props['data-previous-attempt'] !== undefined);
        assert.match(notice.props.className, /warning/, 'a rolled-back attempt keeps the attention mark');
    });
});

// --- The update's stop dialog: an unknown step -----------------------------------------

test('a preflight stop at a step this page has no words for shows the server\'s first line under the reason', () => {
    const input = (message) => ({ targetVersion: 'v0.1.0-alpha.95', previousVersion: 'v0.1.0-alpha.94', message, productMessages: [], reading: false });
    const unknown = outcome.failedUpdateGuidance(null, input('!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=brand_new_step: the checker said something specific detail='));
    const keys = unknown.lines.map(line => line.key ?? line.text);
    assert.equal(keys[1], 'panelUpdate.outcome.preflightStep.generic');
    assert.equal(keys[2], 'panelUpdate.outcome.serverMessage');
    assert.equal(unknown.lines[2].vars.message, 'the checker said something specific');
    assert.equal(unknown.serverMessage, undefined, 'the same line is said twice');
    assert.equal(say('panelUpdate.outcome.serverMessage', unknown.lines[2].vars), 'The server reported: the checker said something specific');
    // A known step keeps its own words and the line stays in Details.
    for (const step of ['mode', 'owner']) {
        const known = outcome.failedUpdateGuidance(null, input(`!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=${step}: diagnostic detail=`));
        assert.equal(known.lines[1].key, `panelUpdate.outcome.preflightStep.${step}`);
        assert.equal(known.lines[1].fallback, 'panelUpdate.outcome.preflightStep.generic');
        assert.equal(known.lines.some(line => line.key === 'panelUpdate.outcome.serverMessage'), false);
        assert.equal(known.serverMessage, 'diagnostic');
        // The boot catalogue carries both, in both languages, with plain meaning.
        for (const locale of ['en', 'tr']) {
            const text = (locale === 'en' ? en : tr)[`panelUpdate.outcome.preflightStep.${step}`];
            assert.ok(text && text.length > 40, `${locale} ${step}`);
            assert.notEqual(text, (locale === 'en' ? en : tr)['panelUpdate.outcome.preflightStep.generic']);
        }
    }
    assert.match(en['panelUpdate.outcome.preflightStep.owner'], /root/);
    assert.match(tr['panelUpdate.outcome.preflightStep.owner'], /root/);
    // The generic step with no line says nothing more.
    const bare = outcome.failedUpdateGuidance(null, input('!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=brand_new_step detail='));
    assert.equal(bare.lines.some(line => line.key === 'panelUpdate.outcome.serverMessage'), false);
});

// --- Source contracts ---------------------------------------------------------------

const source = path => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('the Settings item says "Update available" with a primary-ink dot, never a signal colour', () => {
    const settings = source('components/Settings.tsx');
    assert.match(settings, /note: t\('settings\.section\.updates\.available', \{ version: updateTarget\.version \}\)/);
    assert.match(settings, /rounded-full bg-primary" data-settings-note-dot=""/);
    assert.equal(catalogues.en['settings.section.updates.available'], 'Update available · {version}');
    assert.equal(catalogues.tr['settings.section.updates.available'], 'Güncelleme var · {version}');
});

test('dialog footer controls are at least 44 px tall on a phone only', () => {
    const ui = source('components/ui.tsx');
    assert.match(ui, /flex flex-col-reverse gap-2 sm:flex-row sm:justify-end max-sm:\[&>\*\]:min-h-11 max-sm:\[&>\*\]:justify-center/);
});

test('every new string exists in English and Turkish, and the Turkish is not the English', () => {
    const keys = Object.keys(catalogues.en).filter(key => /^panelUpdate\.(status|discoveryReason|before|auto|notice)\.|^panelUpdate\.(ready|installed|technical|targetCommit)$|^settings\.section\.updates\.available$/.test(key));
    assert.ok(keys.length >= 40, String(keys.length));
    for (const key of keys) {
        assert.ok(catalogues.tr[key], 'missing Turkish: ' + key);
        if (!/^panelUpdate\.notice\.sidebar$|available$/.test(key)) assert.notEqual(catalogues.tr[key], catalogues.en[key], key);
    }
    assert.equal(catalogues.en['panelUpdate.auto.help'], 'Once every 12 hours the panel asks celikpanel.net whether a newer signed release exists. Nothing is installed by itself.');
    assert.equal(catalogues.en['panelUpdate.notice.sidebar'], 'Update available · {version}');
    assert.equal(catalogues.tr['panelUpdate.notice.sidebar'], 'Güncelleme var · {version}');
});

test('on the licence page the card offers the update but not the switch, which belongs to Settings', async () => {
    await withMounted(React.createElement(PanelUpdateCard, { activation: true }), {}, async (renderer, fixture) => {
        assert.equal(renderer.root.findAll(node => node.props && node.props['data-update-auto'] !== undefined).length, 0);
        assert.equal(primaries(renderer).length, 1);
        assert.equal(fixture.requests.filter(line => line.startsWith('PUT ')).length, 0);
    });
});

// Batch 21's first look (2026-10-17): fixes kept by contract.
test('a version is never broken at its hyphen in the row, the sidebar link or the Settings note', async () => {
    const whole = (node) => node.findAll(child => child.type === 'span' && child.props['data-version'] !== undefined);
    await withMounted(React.createElement(React.Fragment, null, shell(), React.createElement(DashboardUpdateNotice, { admin: true })), {}, async (renderer) => {
        const row = renderer.root.find(node => node.type === 'section' && node.props['data-dashboard-update'] !== undefined);
        const link = sidebarLinks(renderer)[0];
        for (const node of [row, link]) {
            const spans = whole(node);
            assert.equal(spans.length, 1);
            assert.equal(textOf(spans[0]), TARGET.version);
            assert.match(spans[0].props.className, /whitespace-nowrap/);
        }
    });
    assert.equal(discovery.wholeVersion('no version here', TARGET.version), 'no version here');
    assert.match(source('components/Settings.tsx'), /section\.note \? wholeVersion\(section\.note, section\.noteVersion \?\? ''\) : section\.description/);
});

test('phone layout: the Settings tab anchors its screen-reader note, the selected icon keeps contrast in dark mode, the menu button is 44 px', () => {
    const settings = source('components/Settings.tsx');
    assert.match(settings, /className=\{`group relative flex min-h-11 shrink-0 items-center/);
    assert.match(settings, /active \? 'bg-primary text-primary-fg' :/);
    assert.doesNotMatch(settings, /bg-primary text-white/);
    const layout = source('components/Layout.tsx');
    assert.match(layout, /className="flex h-11 w-11 items-center justify-center rounded-lg text-fg-muted hover:bg-surface-2 md:hidden"\s*\n\s*onClick=\{\(\) => setMobileOpen\(true\)\}/);
});

// --- Independent review of the candidate (2026-10-17) ----------------------------------

test('a skipped scheduled attempt is said beside the known result and never replaces it', async () => {
    const now = Date.parse('2026-10-17T12:00:00Z');
    const decoded = discovery.decodePanelUpdateDiscovery({ ...storedCurrent(), last_skip: { reason_code: 'update_active', age_seconds: 600 } }, now);
    assert.deepEqual(decoded.lastSkip, { reason: 'update_active', at: now - 600 * 1000 });
    assert.equal(discovery.decodePanelUpdateDiscovery({ ...storedCurrent(), last: { ...storedCurrent().last, outcome: 'skipped', reason_code: 'server_busy' } }, now), null,
        'a stored "skipped" outcome is not a result');
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedCurrent({ last_skip: { reason_code: 'server_busy', age_seconds: 600 } }) }, async (renderer) => {
        assert.equal(statusOf(renderer).props['data-update-status'], 'latest', 'a skip replaced the known result');
        const details = renderer.root.findAll(node => node.props && node.props['data-update-status-detail'] !== undefined).map(textOf);
        assert.deepEqual(details, [say('panelUpdate.status.lastSkipped', { age: discovery.relativeAge(Date.now() - 600_000, Date.now(), 'en', 'just now'), reason: say('panelUpdate.discoveryReason.server_busy') })]);
    });
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedAvailable({ last_skip: { reason_code: 'setup_running', age_seconds: 60 } }) }, async (renderer) => {
        assert.equal(statusOf(renderer).props['data-update-status'], 'available');
        assert.ok(statusText(renderer).includes(say('panelUpdate.discoveryReason.setup_running')));
        assert.equal(primaries(renderer).length, 1);
    });
    assert.equal(catalogues.en['panelUpdate.discoveryReason.setup_running'], 'server setup was changing the server');
});

test('the switch is not "on" before its setting was read: it is disabled, unchecked and says it is not known', async () => {
    await withMounted(React.createElement(PanelUpdateCard), { stored: null }, async (renderer) => {
        const input = renderer.root.find(node => node.type === 'input' && node.props.role === 'switch');
        assert.equal(input.props.checked, false);
        assert.equal(input.props.disabled, true);
        assert.equal(input.props['aria-checked'], false);
        assert.ok(textOf(renderer.root.find(node => node.props && node.props['data-update-auto-unknown'] !== undefined)).includes(say('panelUpdate.auto.unknown')));
        assert.equal(statusOf(renderer).props['data-update-status'], 'unread');
    });
    await withMounted(React.createElement(PanelUpdateCard), { stored: storedCurrent() }, async (renderer) => {
        assert.equal(renderer.root.findAll(node => node.props && node.props['data-update-auto-unknown'] !== undefined).length, 0);
    });
});

test('the stop dialog keeps the whole server message behind Details when the inline first line is cut', () => {
    const input = (message) => ({ targetVersion: 'v0.1.0-alpha.95', previousVersion: 'v0.1.0-alpha.94', message, productMessages: [], reading: false });
    const long = 'x'.repeat(230) + ' the end of a long checker line';
    const cut = outcome.failedUpdateGuidance(null, input(`!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=brand_new_step: ${long} detail=`));
    const inline = cut.lines.find(line => line.key === 'panelUpdate.outcome.serverMessage');
    assert.ok(inline.vars.message.endsWith('…'));
    assert.ok(inline.vars.message.length <= 240);
    assert.equal(cut.serverMessage, long, 'the whole line left Details');
    const short = outcome.failedUpdateGuidance(null, input('!! CELIKPANEL_UPDATE_FAILURE code=recovery_runtime_preflight_failed state=unchanged reason=recovery runtime preflight step=brand_new_step: short line detail='));
    assert.equal(short.serverMessage, undefined, 'a complete line is said twice');
});

test('a Dashboard row snoozed in this browser is decided while rendering, never painted for a frame', async () => {
    storage.clear();
    storage.set(discovery.UPDATE_NOTICE_SNOOZE_KEY, JSON.stringify({ version: TARGET.version, until: Date.now() + HOUR * 1000 }));
    // The shared copy is already known when the row mounts: its first render
    // must be empty.
    discovery.resetPanelUpdateDiscoveryForTests();
    discovery.publishPanelUpdateDiscovery(discovery.decodePanelUpdateDiscovery(storedAvailable(), Date.now()));
    globalThis.discoveryFixture = { auth: { role: 'admin' }, strings: catalogues.en, locale: 'en' };
    const renders = [];
    const Probe = () => { const element = React.createElement(DashboardUpdateNotice, { admin: true }); renders.push(element); return element; };
    let renderer;
    TestRenderer.act(() => { renderer = TestRenderer.create(React.createElement(Probe)); });
    assert.equal(renderer.toJSON(), null, 'a snoozed row was painted');
    TestRenderer.act(() => renderer.unmount());
    storage.clear();
    assert.doesNotMatch(source('components/PanelUpdateNotice.tsx'), /useEffect/);
});

test('the card redraws its ages once a minute while visible, and stops while hidden and when it leaves', async () => {
    const realSet = globalThis.setInterval;
    const realClear = globalThis.clearInterval;
    const live = new Map();
    let next = 1;
    globalThis.setInterval = (fn, ms) => { const id = next++; live.set(id, ms); return id; };
    globalThis.clearInterval = (id) => { live.delete(id); };
    try {
        await withMounted(React.createElement(PanelUpdateCard), { stored: storedCurrent() }, async () => {
            assert.ok([...live.values()].includes(60_000), 'no minute redraw while visible');
            const hidden = Object.getOwnPropertyDescriptor(document, 'visibilityState');
            Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' });
            await act(async () => { document.dispatchEvent(new Event('visibilitychange')); });
            assert.ok(![...live.values()].includes(60_000), 'the minute redraw runs while hidden');
            if (hidden) Object.defineProperty(document, 'visibilityState', hidden); else Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' });
            await act(async () => { document.dispatchEvent(new Event('visibilitychange')); });
            assert.ok([...live.values()].includes(60_000), 'the minute redraw did not resume when shown');
        });
        assert.ok(![...live.values()].includes(60_000), 'the minute redraw outlived the card');
    } finally {
        globalThis.setInterval = realSet;
        globalThis.clearInterval = realClear;
    }
});

test('Turkish: the manual check is "denetle" in the busy and cleared messages, and the restart line says how long it takes', () => {
    assert.equal(tr['err.PANEL_UPDATE_BUSY'], 'Başka bir sunucu işlemi çalışıyor. Tamamlanmasını bekleyip güncellemeleri yeniden denetleyin.');
    assert.match(tr['panelUpdate.identityMismatchCleared'], /güncellemeleri yeniden denetleyin\.$/);
    assert.equal(catalogues.tr['panelUpdate.before.restart'], 'Panel ve Agent bir kez yeniden başlar; bu yaklaşık {seconds} saniye sürer.');
    for (const [key, text] of Object.entries(catalogues.tr)) {
        if (/^(panelUpdate\.|err\.PANEL_UPDATE|start\.updates)/.test(key)) assert.doesNotMatch(text, /kontrol et/i, key);
    }
});

// v0.1.0-alpha.100 is the first three-digit prerelease number: the page reads
// it (and its sequence 100) as any other target. The page never orders
// versions itself; the Agent and the Panel compare them by number and refuse a
// leading zero (their Go tests), and the sequence keeps its own pattern here.
test('a three-digit prerelease target (alpha.100, sequence 100) is read; a sequence with a leading zero is not', () => {
    const target = { ...TARGET, version: 'v0.1.0-alpha.100', sequence: '100' };
    assert.deepEqual(discovery.decodeDiscoveredTarget(target)?.version, 'v0.1.0-alpha.100');
    assert.equal(discovery.decodeDiscoveredTarget(target)?.sequence, '100');
    assert.equal(discovery.decodeDiscoveredTarget({ ...target, sequence: '0100' }), null);
});
