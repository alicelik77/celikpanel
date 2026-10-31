// The shared blocking-problem pattern and the setup Summary that uses it
// (owner feedback 2026-10-31: "I looked at this page and did not understand
// the problem ... everywhere they must know what to do"). What stops the main
// action stands first; the disabled button says why and points to it; each
// problem says what is wrong, what to do and offers the fix; the code stays
// under Technical details; the words are in both languages and name no
// internal terms.
//
// Ortak engel bicimi ve onu kullanan kurulum Ozeti.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { setupCatalogFixture } from './fixtures/server-setup-components.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {
    compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText;
const remoteURL = dataURL(compile('../src/lib/remoteDNS.ts'));
const setupURL = dataURL(compile('../src/lib/serverSetup.ts'));
const componentLibURL = dataURL(compile('../src/lib/serverSetupComponents.ts').replace("from './serverSetup'", `from '${setupURL}'`));
const componentNameURL = dataURL(compile('../src/lib/componentName.ts'));
const operationURL = dataURL(compile('../src/lib/serverSetupOperation.ts').replace("from './serverSetup'", `from '${setupURL}'`));
const dnsEnglishURL = dataURL(compile('../src/i18n/setupDNS/en.ts'));
const dnsTurkishURL = dataURL(compile('../src/i18n/setupDNS/tr.ts'));
const dnsCopyURL = dataURL(compile('../src/i18n/setupDNS.ts').replace("from './setupDNS/en'", `from '${dnsEnglishURL}'`).replace("from './setupDNS/tr'", `from '${dnsTurkishURL}'`));
const guidanceURL = dataURL(compile('../src/lib/serverSetupGuidance.ts'));
const handoverURL = dataURL(compile('../src/lib/panelHandover.ts'));
const visibleReadsURL = dataURL(compile('../src/lib/visibleReads.ts'));
const problemsLibURL = dataURL(compile('../src/lib/setupProblems.ts').replace(/from ['"]\.\/serverSetupGuidance['"]/, `from '${guidanceURL}'`));
const problemsLib = await import(problemsLibURL);
const catalogues = { en: enScreens, tr: trScreens };
const fill = (text, vars) => String(text).replace(/\{(\w+)\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
const stub = dataURL(`
import React from '${reactURL}';
const fill = ${fill.toString()};
export const useAuth = () => globalThis.problemFixture.auth;
export const useI18n = () => ({ locale: globalThis.problemFixture.locale, t: (key, vars) => globalThis.problemFixture.catalogue[key] !== undefined ? fill(globalThis.problemFixture.catalogue[key], vars) : key, screensReady: true, screensFailed: false });
export const useLocation = () => ({ pathname: '/' });
export const useNavigate = () => to => globalThis.problemFixture.navigated.push(to);
export const Navigate = props => React.createElement('redirect', props);
export const Link = props => React.createElement('a', { ...props, href: props.to });
export const useServerSetup = () => globalThis.problemFixture.context;
export const ServerSetupShell = props => React.createElement('main', null, props.navigation, props.children);
export const Button = ({ loading, ...props }) => React.createElement('button', props);
export const SudoNote = () => React.createElement('p', { 'data-sudo-note': '' });
export const mentionsSudo = text => /\\bsudo\\b/.test(text);
export const KeepWords = props => props.text;
export const Spinner = () => React.createElement('span', null, 'loading');
export const AlertTriangle = () => null, ArrowRight = () => null, Check = () => null, Circle = () => null, Loader2 = () => null;
export const ServerSetupDNSConnection = props => React.createElement('remote-connection', props);
export const inputClass = '';
export const AddressLink = props => React.createElement('a', { href: props.href }, props.address);
export const SetupFirstSite = props => React.createElement('first-site', props);
`);
const mapped = (path, extra = {}) => path === 'react' ? reactURL : extra[path.split('/').pop()] ?? stub;
const load = (path, extra) => dataURL(`import React from '${reactURL}';\n` + compile(path).replace(/from ['"]([^'"]+)['"]/g, (_, from) => `from '${mapped(from, extra)}'`));
const blockingURL = load('../src/components/BlockingProblems.tsx');
const planProblemsURL = load('../src/components/SetupPlanProblems.tsx', { setupProblems: problemsLibURL, BlockingProblems: blockingURL });
const { BlockingProblems, BlockedReason } = await import(blockingURL);
const libs = { remoteDNS: remoteURL, componentName: componentNameURL, serverSetupComponents: componentLibURL, setupDNS: dnsCopyURL, visibleReads: visibleReadsURL, panelHandover: handoverURL, serverSetupGuidance: guidanceURL, serverSetupOperation: operationURL, serverSetup: setupURL, setupProblems: problemsLibURL, SetupPlanProblems: planProblemsURL, BlockingProblems: blockingURL };
const wizardStepsURL = load('../src/components/ServerSetupWizardSteps.tsx', { ...libs, remote: dataURL(compile('../src/lib/remote.ts').replace(/from ['"]([^'"]+)['"]/g, (_, from) => `from '${from === 'react' ? reactURL : dataURL(compile('../src/lib/apiError.ts'))}'`)) });
const componentUIURL = load('../src/components/ServerSetupComponents.tsx', libs);
const stepsURL = load('../src/components/ServerSetupSteps.tsx');
const choiceURL = load('../src/components/ServerSetupChoice.tsx', libs);
const { ServerSetup: Wizard } = await import(load('../src/components/ServerSetup.tsx', { ...libs, ServerSetupWizardSteps: wizardStepsURL, ServerSetupComponents: componentUIURL, ServerSetupSteps: stepsURL, ServerSetupChoice: choiceURL }));

const original = { fetch: globalThis.fetch, window: globalThis.window, document: globalThis.document, localStorage: globalThis.localStorage, sessionStorage: globalThis.sessionStorage };
function restore() {
    for (const [key, value] of Object.entries(original)) { if (value === undefined) delete globalThis[key]; else globalThis[key] = value; }
    delete globalThis.problemFixture;
}
const textOf = node => node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children);
const insideDetails = instance => { for (let node = instance.parent; node; node = node.parent) if (node.type === 'details') return true; return false; };

for (const locale of ['en', 'tr']) {
    const c = catalogues[locale];
    test(`${locale}: the pattern says what, what to do and the fix; the disabled button says why and shows the problems`, async () => {
        const focused = [];
        globalThis.problemFixture = { locale, catalogue: c, navigated: [] };
        globalThis.document = { getElementById: id => ({ scrollIntoView: options => focused.push(['scroll', id, options.block]), focus: () => focused.push(['focus', id]) }) };
        let fixed = 0, tree;
        try {
            await act(async () => {
                tree = Renderer.create(React.createElement('div', null,
                    React.createElement(BlockingProblems, { id: 'demo-problems', problems: [{ code: 'demo_code:detail', what: 'What is wrong.', todo: 'Do this.', fix: { label: 'Fix it', onFix: () => { fixed++; } } }, { code: 'second', what: 'Second.', todo: 'Run sudo true.' }] }),
                    React.createElement('ul', { 'data-long-list': '' }, React.createElement('li', null, 'a long list')),
                    React.createElement(BlockedReason, { id: 'demo-reason', target: 'demo-problems', text: c['problem.blocked.many'].replace('{count}', '2') }),
                    React.createElement('button', { disabled: true, 'aria-describedby': 'demo-reason' }, 'Save')));
            });
            const page = textOf(tree.toJSON());
            const box = tree.root.findByProps({ 'data-blocking-problems': 2 });
            assert.equal(box.props.role, 'alert');
            assert.equal(box.props.id, 'demo-problems');
            assert.equal(box.props.tabIndex, -1);
            assert.ok(page.indexOf('What is wrong.') < page.indexOf('a long list'), 'the problem stands before the list');
            assert.ok(page.includes(fill(c['problem.title.many'], { count: 2 })));
            assert.ok(page.includes(c['problem.todo']) && page.includes('Do this.'));
            // The disabled button is described by the visible reason.
            const reason = tree.root.findByProps({ id: 'demo-reason' });
            assert.equal(tree.root.findByProps({ 'aria-describedby': 'demo-reason' }).props.disabled, true);
            assert.ok(textOf(reason).includes(fill(c['problem.blocked.many'], { count: 2 })));
            await act(async () => reason.findByType('button').props.onClick());
            assert.deepEqual(focused, [['scroll', 'demo-problems', 'start'], ['focus', 'demo-problems']]);
            await act(async () => tree.root.findByProps({ 'data-problem-fix': 'demo_code:detail' }).props.onClick());
            assert.equal(fixed, 1);
            // The code is under Technical details, never in the sentence.
            const code = tree.root.findAll(node => node.type === 'code' && textOf(node).includes('demo_code:detail'));
            assert.equal(code.length, 1);
            assert.ok(insideDetails(code[0]));
            assert.equal(tree.root.findAllByProps({ 'data-sudo-note': '' }).length, 1, 'a sudo command carries the sudo note');
            await act(async () => tree.unmount());
        } finally { restore(); }
    });
}

// Every blocker code with a guide has both sentences in both languages with
// the same placeholders, and the words name no internal concept.
test('every setup blocker guide has what and what-to-do in English and Turkish, plain and with the same values', () => {
    const internal = /\b[a-z]+_[a-z_]+\b|\bAgent\b|\bjoin gave\b|\bdraft\b|\bblocker\b|\bplan_id\b/;
    for (const name of problemsLib.setupProblemNames) {
        for (const part of ['what', 'todo']) {
            const key = `setup.problem.${name}.${part}`;
            const english = enScreens[key], turkish = trScreens[key];
            assert.ok(english && turkish, `${key} in both languages`);
            const values = text => [...text.matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort().join(',');
            assert.equal(values(turkish), values(english), `${key} carries the same values`);
            assert.ok(!internal.test(english.replace(/CELIKPANEL_TLS_[A-Z]+/g, '')), `${key} names an internal term: ${english}`);
            assert.ok(english.length <= 220 && turkish.length <= 240, `${key} is short`);
        }
    }
    for (const code of problemsLib.setupProblemCodes) {
        const sample = code === 'server_setup_hosting_root_not_traversable' ? `${code}:0700:root:root:/srv` : ['server_setup_service_unsupported', 'server_setup_dependency_missing', 'server_setup_version_repository_unknown', 'server_setup_version_repository_required'].includes(code) ? `${code}:redis` : code === 'server_setup_service_conflict' ? `${code}:redis:valkey` : code;
        assert.ok(problemsLib.setupProblemGuide(sample), `${code} has a guide`);
    }
    // Each code the Panel adds as a blocker has a guide (cmd/panel addBlocker).
    const panel = readFileSync(new URL('../../cmd/panel/server_setup_operations.go', import.meta.url), 'utf8');
    for (const [, code] of panel.matchAll(/addBlocker\("([a-z_]+)(?::|")/g)) {
        if (code === 'server_setup_') continue;
        assert.ok(problemsLib.setupProblemCodes.includes(code), `${code} has a plain-language guide`);
    }
    assert.equal(problemsLib.setupProblemGuide('server_setup_service_unsupported:postfix').fix.kind, 'removeMail');
    assert.equal(problemsLib.setupProblemGuide('server_setup_peer_mail_both').fix.kind, 'removeMail');
    assert.equal(problemsLib.setupProblemGuide('a_code_without_words'), null);
});

// The setup Summary, mounted with the real wizard.
let calls, state, tree, plan;
function init(locale, blockers) {
    calls = []; tree = null;
    state = { version: 1, revision: 1, origin: 'fresh', status: 'new', required: true, draft: { purpose: 'web', panel_domain: '', mail_hostname: '', dns_mode: 'local', remote_dns_connection_id: '', dns_engine: 'pdns', dns_role: 'primary', ns1: '', ns2: '', local_ip: '', peer_ip: '', peer_ns: '', node_version: '', database: 'mariadb' }, checks: [] };
    plan = () => ({ id: 'a'.repeat(32), version: 1, revision: state.revision, purpose: state.draft.purpose, steps: [{ id: 'svc', kind: 'service', target: 'nginx' }, { id: 'verify', kind: 'verify', target: 'web' }], components: [{ id: 'nginx', selected: true, installed: false, required: false }], blockers, can_start: blockers.length === 0, tcp_ports: [22, 80, 443], udp_ports: [], preserve_ssh: true, persist_firewall: true, contact_email: 'admin@example.com' });
    const context = { snapshot: state, accept(next) { state = next; context.snapshot = next; }, async reload() { return state; } };
    globalThis.problemFixture = { locale, catalogue: catalogues[locale], navigated: [], auth: { role: 'admin', user: { username: 'admin' }, logout() {} }, context };
    const store = new Map(), editor = new Map();
    globalThis.localStorage = { getItem: key => store.get(key) ?? null, setItem: (key, value) => store.set(key, value), removeItem: key => store.delete(key) };
    globalThis.sessionStorage = { getItem: key => editor.get(key) ?? null, setItem: (key, value) => editor.set(key, value), removeItem: key => editor.delete(key) };
    globalThis.window = { setTimeout, clearTimeout, setInterval, clearInterval, location: { hostname: '192.0.2.4', port: '2083', reload() {} } };
    globalThis.fetch = async (url, options) => {
        calls.push({ url, options });
        if (url.includes('/setup/operation')) return Response.json(null);
        if (url === '/api/v1/setup/components') return Response.json(setupCatalogFixture());
        if (url === '/api/v1/setup' && options?.method === 'PUT') { const body = JSON.parse(options.body); state = { ...state, draft: body.draft, revision: state.revision + 1, status: 'draft' }; return Response.json(state); }
        if (url === '/api/v1/setup/plan') return Response.json(plan());
        return Response.json(state);
    };
}
const submit = () => act(async () => tree.root.findByType('form').props.onSubmit({ preventDefault() {} }));
const roleStep = () => tree.root.findAll(node => node.type?.name === 'SetupRolesStep')[0];
async function toReview({ mail = false } = {}) {
    await act(async () => { tree = Renderer.create(React.createElement(Wizard)); });
    await act(async () => tree.root.findByProps({ id: 'setup-main_domain' }).props.onChange({ target: { value: 'example.com' } }));
    await submit(); await submit(); await submit();
    if (mail) await act(async () => roleStep().props.onRoles({ web: true, mail: true, dns: true }));
    await submit(); await submit();
    await act(async () => tree.root.findByProps({ name: 'setup-dns', value: 'external' }).props.onChange());
    await submit();
    if (mail) await submit();
}
const startButton = c => tree.root.findAllByType('button').find(button => textOf(button) === c['setup.start']);
const buttonNamed = text => tree.root.findAllByType('button').find(button => textOf(button) === text);

for (const locale of ['en', 'tr']) {
    const c = catalogues[locale];
    test(`${locale}: the Summary shows what stops the start first, says why Start is off, and Remove Email returns to the roles without it`, async () => {
        init(locale, ['server_setup_peer_mail_both', 'firewall_busy:another change']);
        try {
            await toReview({ mail: true });
            const page = textOf(tree.toJSON());
            const box = tree.root.findByProps({ 'data-blocking-problems': 2 });
            assert.equal(box.props.id, 'setup-plan-problems');
            assert.ok(page.includes(fill(c['setup.problem.title.many'], { count: 2 })));
            const first = page.indexOf(c['setup.problem.peerMailBoth.what']);
            assert.ok(first > 0 && first < page.indexOf(c['setup.components.reviewTitle']), 'the problems come before the components list');
            assert.ok(first < page.indexOf(c['setup.firewallReview']), 'and before the firewall summary');
            assert.ok(page.includes(c['setup.problem.firewallBusy.todo']));
            assert.ok(!page.includes(c['setup.v5.wait.server_setup_peer_mail_both']), 'the old internal sentence is gone');
            // Start is off and says why, pointing to the problems.
            const start = startButton(c);
            assert.equal(start.props.disabled, true);
            assert.equal(start.props['aria-describedby'], 'setup-start-reason');
            const reason = tree.root.find(node => node.type === 'p' && node.props.id === 'setup-start-reason');
            assert.equal(reason.props['data-blocked-reason'], 'setup-plan-problems');
            assert.ok(textOf(reason).includes(fill(c['setup.problem.blocked.many'], { count: 2 })));
            // The codes are under Technical details only.
            for (const code of tree.root.findAll(node => node.type === 'code')) assert.ok(insideDetails(code));
            // Remove Email: back at the roles step, mail unticked; nothing started.
            const plans = calls.filter(call => call.url === '/api/v1/setup/plan').length;
            await act(async () => buttonNamed(c['setup.problem.fix.removeMail']).props.onClick());
            assert.ok(roleStep(), 'the roles step is open');
            assert.equal(roleStep().props.roles.mail, false);
            assert.equal(roleStep().props.roles.web, true);
            assert.equal(calls.filter(call => call.url === '/api/v1/setup/plan').length, plans);
            assert.equal(calls.some(call => call.url === '/api/v1/setup/start'), false);
        } finally { if (tree) await act(async () => tree.unmount()); restore(); }
    });

    test(`${locale}: a step fix goes to that step, Check again only reads the plan again, and a clear plan asks for the confirmation`, async () => {
        init(locale, ['server_setup_panel_domain_invalid', 'firewall_busy:x']);
        try {
            await toReview();
            const plans = calls.filter(call => call.url === '/api/v1/setup/plan').length;
            await act(async () => buttonNamed(c['setup.problem.fix.recheck']).props.onClick());
            assert.equal(calls.filter(call => call.url === '/api/v1/setup/plan').length, plans + 1, 'Check again reads the plan again');
            assert.equal(calls.some(call => call.url === '/api/v1/setup/start'), false);
            await act(async () => buttonNamed(fill(c['setup.problem.fix.step'], { step: c['setup.step.names'] })).props.onClick());
            assert.ok(tree.root.findAll(node => node.type?.name === 'SetupNamesStep').length === 1, 'the Names step is open');
        } finally { if (tree) await act(async () => tree.unmount()); restore(); }
        init(locale, []);
        try {
            await toReview();
            assert.equal(tree.root.findAllByProps({ 'data-blocking-problems': 1 }).length + tree.root.findAllByProps({ 'data-blocking-problems': 2 }).length, 0);
            const reason = tree.root.find(node => node.type === 'p' && node.props.id === 'setup-start-reason');
            assert.equal(reason.props['data-blocked-reason'], 'setup-confirm');
            assert.ok(textOf(reason).includes(c['setup.problem.blocked.confirm']));
            await act(async () => tree.root.findByProps({ type: 'checkbox' }).props.onChange({ target: { checked: true } }));
            assert.equal(startButton(c).props.disabled, false);
            assert.equal(startButton(c).props['aria-describedby'], undefined);
            assert.equal(tree.root.findAll(node => node.props.id === 'setup-start-reason').length, 0);
        } finally { if (tree) await act(async () => tree.unmount()); restore(); }
    });
}
