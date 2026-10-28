// The setup page between "every check passed" and "Your server is ready"
// (owner report 2026-10-14, alpha.84): after "Check requirements again" passed,
// the page showed a static "Waiting for the server to confirm" box with the
// buttons still enabled and nothing moving for up to 20 s. D-024: an active
// state says what is happening, that nobody needs to act and how it resumes;
// it never spins forever, and a page read never starts a change.
//
// Kontroller gectikten sonra kurulumun tamamlanmasi, gercek katalogla ve iki dilde.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { en } from '../src/i18n/en.ts';
import { tr } from '../src/i18n/tr.ts';
import { enScreens } from './locale-catalogue.mjs';
import { trScreens } from './locale-catalogue.mjs';
import { enServerScreens } from './locale-catalogue.mjs';
import { trServerScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), {
    compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 },
}).outputText;
const setupURL = dataURL(compile('../src/lib/serverSetup.ts'));
const remoteURL = dataURL(compile('../src/lib/remoteDNS.ts'));
const componentLibURL = dataURL(compile('../src/lib/serverSetupComponents.ts').replace("from './serverSetup'", `from '${setupURL}'`));
const componentNameURL = dataURL(compile('../src/lib/componentName.ts'));
const operationURL = dataURL(compile('../src/lib/serverSetupOperation.ts').replace("from './serverSetup'", `from '${setupURL}'`));
const dnsEnglishURL = dataURL(compile('../src/i18n/setupDNS/en.ts'));
const dnsTurkishURL = dataURL(compile('../src/i18n/setupDNS/tr.ts'));
const dnsCopyURL = dataURL(compile('../src/i18n/setupDNS.ts').replace("from './setupDNS/en'", `from '${dnsEnglishURL}'`).replace("from './setupDNS/tr'", `from '${dnsTurkishURL}'`));
const guidanceURL = dataURL(compile('../src/lib/serverSetupGuidance.ts'));
const handoverURL = dataURL(compile('../src/lib/panelHandover.ts'));
const visibleReadsURL = dataURL(compile('../src/lib/visibleReads.ts'));
const catalogues = { en: { ...en, ...enScreens, ...enServerScreens }, tr: { ...tr, ...trScreens, ...trServerScreens } };
const stub = dataURL(`
import React from '${reactURL}';
const fill = (text, vars) => String(text).replace(/\\{(\\w+)\\}/g, (all, name) => vars && vars[name] !== undefined ? String(vars[name]) : all);
export const useAuth = () => ({ role: 'admin', user: { username: 'admin' }, logout() {} });
export const useI18n = () => ({ locale: globalThis.finishing.locale, t: (key, vars) => fill(globalThis.finishing.catalogue[key] ?? key, vars), screensReady: true, screensFailed: false });
export const Navigate = props => React.createElement('redirect', props);
export const Link = props => React.createElement('a', { ...props, href: props.to });
export const useServerSetup = () => globalThis.finishing.context;
export const ServerSetupShell = props => React.createElement('main', null, props.navigation, props.children);
export const Button = props => React.createElement('button', props);
export const KeepWords = props => props.text;
export const Spinner = () => React.createElement('span', null, 'loading');
export const ArrowRight = () => null, Check = () => null, Circle = () => null;
export const Loader2 = props => React.createElement('svg', { 'data-spinner': '', className: props.className });
export const ServerSetupDNSConnection = () => null;
export const ServerSetupSteps = () => null;
export const ServerSetupComponents = () => null;
export const useSetupComponentCatalog = () => ({ catalog: null, failed: false, reload() {} });
export const ServerSetupChoice = () => null, ServerSetupManualAction = () => null;
export const SetupCountStep = () => null, SetupMailStep = () => null, SetupOpenTasks = () => null, SetupRecordsStep = () => null, SetupDNSHomeStep = () => null, SetupDomainStep = () => null, SetupNameStep = () => null, SetupNamesStep = () => null, SetupPanelAccessCard = () => null, SetupProviderRecordsCard = () => null, SetupRegistrarCard = () => null, SetupRolesStep = () => null, SetupWizardInput = () => null;
export const setupEngineChoice = () => undefined, setupServerLabel = () => 'This server';
export const inputClass = '';
export const AddressLink = props => React.createElement('a', { href: props.href }, props.address);
`);
const source = compile('../src/components/ServerSetup.tsx').replace(/from ['"]([^'"]+)['"]/g, (_, path) => {
    const url = path === 'react' ? reactURL : path.endsWith('/remoteDNS') ? remoteURL : path.endsWith('/componentName') ? componentNameURL : path.endsWith('/serverSetupComponents') ? componentLibURL
        : path.endsWith('/setupDNS') ? dnsCopyURL : path.endsWith('/visibleReads') ? visibleReadsURL : path.endsWith('/panelHandover') ? handoverURL : path.endsWith('/serverSetupGuidance') ? guidanceURL
            : path.endsWith('/serverSetupOperation') ? operationURL : path.endsWith('/serverSetup') ? setupURL : stub;
    return `from '${url}'`;
});
const { ServerSetup } = await import(dataURL(`import React from '${reactURL}';\n${source}`));

const draft = { purpose: 'web', panel_domain: 'panel.example.com', mail_hostname: 'mail.example.com', dns_mode: 'external', remote_dns_connection_id: '', dns_engine: 'bind', dns_role: 'primary', ns1: '', ns2: '', local_ip: '', peer_ip: '', peer_ns: '', node_version: '', database: 'mariadb' };
const steps = [
    { id: 'svc', kind: 'service', target: 'postfix', status: 'succeeded' },
    { id: 'verify', kind: 'verify', target: 'web', status: 'pending' },
];
const ptrCheck = ptr => ({ id: 'mail_identity', state: 'action_required', code: 'mail_identity_required', reason: 'reverse_dns_mismatch', vars: { hostname: 'mail.example.com', ip: '203.0.113.42', ptr } });
const run = extra => ({ id: 'c'.repeat(32), request_id: 'b'.repeat(32), plan_id: 'a'.repeat(32), phase: 'verification', steps, ...extra });
const readyChecks = [{ id: 'mail_identity', state: 'ready', code: 'ready' }, { id: 'panel_https', state: 'ready', code: 'ready' }];

const originals = { fetch: globalThis.fetch, window: globalThis.window, localStorage: globalThis.localStorage, sessionStorage: globalThis.sessionStorage };
// The page's timers are the window's; the test fires them by their interval.
function fakeWindow() {
    const timers = new Map();
    let next = 1;
    const add = kind => (fn, ms) => { const id = next++; timers.set(id, { fn, ms, kind }); return id; };
    const win = {
        reloads: 0, timers,
        setTimeout: add('timeout'), setInterval: add('interval'),
        clearTimeout: id => timers.delete(id), clearInterval: id => timers.delete(id),
        location: { hostname: 'panel.example.com', port: '2083', reload() { win.reloads++; } },
    };
    return win;
}
async function mount(locale) {
    const world = {
        snapshot: { version: 1, revision: 3, origin: 'fresh', status: 'waiting', required: true, draft, checks: [] },
        execution: run({ status: 'waiting', checks: [ptrCheck('static.42.provider.example')] }),
        reloads: [], calls: [],
    };
    globalThis.finishing = {
        locale, catalogue: catalogues[locale],
        context: {
            snapshot: world.snapshot, accept() {},
            async reload(checks) {
                world.reloads.push(checks);
                return checks === 'manual' ? { ...world.snapshot, checks: readyChecks } : world.snapshot;
            },
        },
    };
    const store = new Map([['celikpanel.setup.start.admin', JSON.stringify({ plan_id: 'a'.repeat(32), request_id: 'b'.repeat(32), panel_domain: 'panel.example.com' })]]);
    globalThis.localStorage = { getItem: key => store.get(key) ?? null, setItem: (key, value) => store.set(key, value), removeItem: key => store.delete(key) };
    globalThis.sessionStorage = { getItem: () => null, setItem() {}, removeItem() {} };
    globalThis.window = fakeWindow();
    globalThis.fetch = async (url, options) => {
        world.calls.push({ url: String(url), method: options?.method || 'GET' });
        if (String(url).includes('/setup/operation')) return Response.json(world.execution);
        return Response.json(world.snapshot);
    };
    let tree;
    await act(async () => { tree = Renderer.create(React.createElement(ServerSetup)); });
    return { tree, world, win: globalThis.window };
}
async function unmount(tree) {
    await act(async () => tree.unmount());
    for (const [key, value] of Object.entries(originals)) { if (value === undefined) delete globalThis[key]; else globalThis[key] = value; }
    delete globalThis.finishing;
}
function textOf(node) {
    if (node === null || node === undefined || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join(' ');
    return textOf(node.children);
}
const buttonNamed = (tree, label) => tree.root.findAllByType('button').find(item => item.props.children === label);
const verifyState = tree => tree.root.findAll(node => node.props && node.props['data-step-state'] !== undefined).at(-1).props['data-step-state'];
const intervals = win => [...win.timers.values()].filter(item => item.kind === 'interval').map(item => item.ms);
async function fire(win, kind, ms) {
    const due = [...win.timers.entries()].filter(([, item]) => item.kind === kind && item.ms === ms);
    assert.ok(due.length > 0, `no ${kind} of ${ms} ms`);
    await act(async () => {
        for (const [id, item] of due) { if (kind === 'timeout') win.timers.delete(id); item.fn(); }
    });
    await act(async () => {});
}
async function checkAgain(tree, c) {
    await act(async () => { void buttonNamed(tree, c['setup.verify']).props.onClick(); });
    await act(async () => {});
}

for (const locale of ['en', 'tr']) {
    const c = catalogues[locale];

    test(`${locale}: a passing check shows setup finishing, with a spinner, nothing to press, and completes by itself`, async () => {
        const { tree, world, win } = await mount(locale);
        try {
            assert.equal(verifyState(tree), 'waitingReverseDNS');
            assert.ok(intervals(win).includes(3000));
            assert.ok(buttonNamed(tree, c['setup.editPlan']), 'a waiting final check offers Edit plan');
            await checkAgain(tree, c);
            assert.equal(world.reloads.at(-1), 'manual', 'the explicit check asks the server to finish');
            assert.equal(world.reloads.filter(value => value === 'manual').length, 1, 'only the click is explicit');
            const page = textOf(tree.toJSON());
            const heading = tree.root.findByProps({ id: 'setup-progress-title' });
            assert.equal(textOf(heading.children), c['setup.finishingTitle']);
            assert.equal(verifyState(tree), 'finishing');
            assert.ok(page.includes(c['setup.stepState.finishing']));
            assert.ok(page.includes(c['setup.finishing']), 'one plain sentence: nothing to do, the page updates by itself');
            const sentence = tree.root.findByProps({ 'data-setup-finishing': '' });
            assert.equal(sentence.findAll(node => node.props && node.props['data-spinner'] !== undefined).length, 1, 'the sentence carries a spinner');
            assert.ok(tree.root.findAll(node => node.props && node.props['data-step-state'] === 'finishing')[0].findAll(node => node.props && node.props['data-spinner'] !== undefined).length === 1, 'the step carries a spinner');
            assert.ok(!buttonNamed(tree, c['setup.verify']), 'no "Check requirements again" while finishing');
            assert.ok(!buttonNamed(tree, c['setup.verifyChecking']));
            // Edit plan stops the run; nothing needs to be done while it finishes.
            assert.ok(!buttonNamed(tree, c['setup.editPlan']), 'no Edit plan while finishing');
            assert.ok(!page.includes(c['setup.reviseHelp']), 'no Edit plan help while finishing');
            assert.ok(!page.includes('static.42.provider.example'), 'the old unmet result is not shown as current');
            assert.ok(!page.includes(c['setup.stepState.waitingReverseDNS']));
            assert.ok(intervals(win).includes(1500) && !intervals(win).includes(3000), 'the page reads more often while finishing');

            // The server completes on its own pass; the next read shows it.
            world.execution = run({ status: 'succeeded', phase: 'complete', checks: readyChecks, steps: steps.map(item => ({ ...item, status: 'succeeded' })) });
            world.snapshot = { ...world.snapshot, status: 'ready', checks: readyChecks };
            await fire(win, 'interval', 1500);
            const done = textOf(tree.toJSON());
            assert.ok(done.includes(c['setup.completeTitle']), 'completion shows without any click');
            assert.ok(!done.includes(c['setup.finishing']));
            assert.ok(world.calls.every(call => call.method === 'GET'), 'reading the page changes nothing');
        } finally { await unmount(tree); }
    });

    test(`${locale}: finishing that is not confirmed within a minute says the result is not known and what to do`, async () => {
        const { tree, win } = await mount(locale);
        try {
            await checkAgain(tree, c);
            assert.equal(verifyState(tree), 'finishing');
            assert.ok(!buttonNamed(tree, c['setup.editPlan']), 'no Edit plan while finishing');
            assert.ok([...win.timers.values()].some(item => item.kind === 'timeout' && item.ms === 60000), 'the wait is bounded');
            await fire(win, 'timeout', 60000);
            const page = textOf(tree.toJSON());
            assert.ok(page.includes(c['setup.finishingSlow']), 'an honest unknown result');
            assert.ok(c['setup.finishingSlow'].includes('sudo systemctl restart celikpanel-panel'));
            assert.ok(!page.includes(c['setup.finishing']));
            assert.equal(tree.root.findAll(node => node.props && node.props['data-setup-finishing'] !== undefined).length, 0, 'no spinner left running');
            assert.notEqual(verifyState(tree), 'finishing');
            // Every check had passed: the step says completion is not confirmed,
            // not "Waiting for requirements" (browser inspection 2026-10-14).
            assert.equal(verifyState(tree), 'unconfirmed');
            assert.ok(page.includes(c['setup.stepState.unconfirmed']));
            assert.ok(!page.includes(c['setup.verifyWaiting']), 'no "Waiting for requirements" after every check passed');
            assert.ok(buttonNamed(tree, c['setup.verify']), 'the owner can check again');
            assert.ok(buttonNamed(tree, c['setup.editPlan']), 'Edit plan returns with the unknown result');
            assert.ok(intervals(win).includes(3000), 'back to the usual reads');
            const reload = buttonNamed(tree, c['setup.reloadPage']);
            assert.ok(reload, 'reload this page');
            await act(async () => reload.props.onClick());
            assert.equal(win.reloads, 1);
        } finally { await unmount(tree); }
    });

    test(`${locale}: a newer unmet result recorded by the run ends finishing and shows that result`, async () => {
        const { tree, world, win } = await mount(locale);
        try {
            await checkAgain(tree, c);
            assert.equal(verifyState(tree), 'finishing');
            // The same old result read again keeps finishing.
            await fire(win, 'interval', 1500);
            assert.equal(verifyState(tree), 'finishing');
            world.execution = run({ status: 'waiting', checks: [ptrCheck('other.provider.example')] });
            await fire(win, 'interval', 1500);
            const page = textOf(tree.toJSON());
            assert.notEqual(verifyState(tree), 'finishing');
            assert.ok(page.includes('other.provider.example'), 'the run’s own newer observation is shown');
            assert.ok(!page.includes(c['setup.finishing']));
            assert.ok(buttonNamed(tree, c['setup.verify']));
            assert.ok(buttonNamed(tree, c['setup.editPlan']), 'Edit plan returns once the run is waiting again');
        } finally { await unmount(tree); }
    });

    // Owner rule 2026-10-14: a page that waits shows motion or says plainly that
    // nothing needs doing, and it does not wait silently for ever.
    test(`${locale}: "Check requirements again" spins while the check runs`, async () => {
        const { tree } = await mount(locale);
        let answer;
        globalThis.finishing.context.reload = () => new Promise(resolve => { answer = resolve; });
        try {
            await act(async () => { void buttonNamed(tree, c['setup.verify']).props.onClick(); });
            const busy = buttonNamed(tree, c['setup.verifyChecking']);
            assert.ok(busy, 'the button names the running check');
            assert.equal(busy.props.loading, true, 'the button shows motion while the check runs');
            await act(async () => { answer({ version: 1, revision: 3, origin: 'fresh', status: 'waiting', required: true, draft, checks: [ptrCheck('static.42.provider.example')] }); });
            await act(async () => {});
            assert.notEqual(buttonNamed(tree, c['setup.verify'])?.props.loading, true, 'the motion ends with the answer');
        } finally { await unmount(tree); }
    });

    test(`${locale}: a lost connection reconnects with motion, says nothing needs doing, and is bounded`, async () => {
        const { tree, world, win } = await mount(locale);
        try {
            const answer = globalThis.fetch;
            globalThis.fetch = async (url, options) => {
                world.calls.push({ url: String(url), method: options?.method || 'GET' });
                if (String(url).includes('/setup/operation')) throw new TypeError('connection refused');
                return Response.json(world.snapshot);
            };
            await fire(win, 'interval', 3000);
            let page = textOf(tree.toJSON());
            assert.ok(page.includes(c['setup.reconnecting']), 'the heading names the reconnect');
            assert.ok(page.includes(c['setup.uncertain']), 'the sentence says the page asks again by itself');
            const sentence = tree.root.findByProps({ 'data-setup-reconnecting': '' });
            assert.equal(sentence.findAll(node => node.props && node.props['data-spinner'] !== undefined).length, 1, 'the reconnect carries a spinner');
            assert.ok(intervals(win).includes(3000), 'the page keeps reading by itself');
            assert.ok([...win.timers.values()].some(item => item.kind === 'timeout' && item.ms === 60000), 'the reconnect is bounded');
            await fire(win, 'timeout', 60000);
            page = textOf(tree.toJSON());
            assert.ok(page.includes(c['setup.reconnectSlow']), 'an honest unknown result after a minute');
            assert.ok(c['setup.reconnectSlow'].includes('sudo systemctl status celikpanel-panel'));
            assert.equal(tree.root.findAll(node => node.props && node.props['data-setup-reconnecting'] !== undefined).length, 0, 'no spinner left running');
            assert.ok(intervals(win).includes(3000), 'reads continue after the notice');
            const reload = buttonNamed(tree, c['setup.reloadPage']);
            assert.ok(reload, 'reload this page');
            await act(async () => reload.props.onClick());
            assert.equal(win.reloads, 1);
            // The server answers again: the reconnect and its notice end by themselves.
            globalThis.fetch = answer;
            await fire(win, 'interval', 3000);
            page = textOf(tree.toJSON());
            assert.ok(!page.includes(c['setup.reconnectSlow']) && !page.includes(c['setup.reconnecting']));
            assert.ok(world.calls.every(call => call.method === 'GET'), 'reconnecting only reads');
        } finally { await unmount(tree); }
    });
}

// The page reads its operation only while the tab is visible (2026-10-15, the
// open item of the alpha.85 notes): hidden, the interval stops and nothing is
// read; shown again, one read at once and the interval resumes. Reads only.
test('a hidden setup page stops its reads, and reads once at once when shown again', async () => {
    const doc = new EventTarget();
    doc.visibilityState = 'hidden';
    globalThis.document = doc;
    const { tree, world, win } = await mount('en');
    const operationReads = () => world.calls.filter(call => call.url.includes('/setup/operation')).length;
    try {
        await act(async () => {});
        const afterMount = operationReads();
        assert.ok(afterMount >= 1, 'the page reads once when it opens');
        assert.deepEqual(intervals(win), [], 'no interval while hidden');
        doc.visibilityState = 'visible';
        await act(async () => { doc.dispatchEvent(new Event('visibilitychange')); });
        await act(async () => {});
        assert.equal(operationReads(), afterMount + 1, 'one read at once when shown');
        assert.ok(intervals(win).includes(3000), 'the interval resumes');
        doc.visibilityState = 'hidden';
        await act(async () => { doc.dispatchEvent(new Event('visibilitychange')); });
        assert.deepEqual(intervals(win), [], 'hidden again: the interval stops');
        assert.equal(operationReads(), afterMount + 1);
        assert.ok(world.calls.every(call => call.method === 'GET'), 'reads only');
    } finally {
        await unmount(tree);
        delete globalThis.document;
    }
});

test('the finishing and reverse DNS wording names who acts and how it resumes in both languages', () => {
    for (const key of ['setup.finishing', 'setup.finishingSlow', 'setup.finishingTitle', 'setup.reloadPage', 'setup.stepState.finishing', 'setup.stepState.waitingReverseDNS', 'setup.guide.reverseDNSPropagation', 'setup.uncertain', 'setup.reconnectSlow']) {
        for (const locale of ['en', 'tr']) assert.ok(catalogues[locale][key], `${locale} ${key}`);
    }
    assert.match(catalogues.en['setup.finishing'], /you do not need to do anything/);
    assert.match(catalogues.tr['setup.finishing'], /sizin bir şey yapmanız gerekmiyor/);
    assert.match(catalogues.en['setup.uncertain'], /by itself, and you do not need to do anything/);
    assert.match(catalogues.tr['setup.uncertain'], /kendiliğinden yeniden sorar; sizin bir şey yapmanız gerekmez/);
    for (const locale of ['en', 'tr']) assert.match(catalogues[locale]['setup.reconnectSlow'], /sudo systemctl start celikpanel-panel/);
    assert.match(catalogues.en['setup.guide.reverseDNSPropagation'], /you do not need to do anything more in CelikPanel\.$/);
    assert.match(catalogues.tr['setup.guide.reverseDNSPropagation'], /CelikPanel’de başka bir şey yapmanız gerekmez\.$/);
    for (const locale of ['en', 'tr']) assert.doesNotMatch(catalogues[locale]['setup.stepState.waitingReverseDNS'], /you|siz/i, 'not "Waiting for you"');
    assert.equal(catalogues.en['setup.verifyChecksPassed'], undefined, 'the passive "waiting for the server" box is gone');
});
