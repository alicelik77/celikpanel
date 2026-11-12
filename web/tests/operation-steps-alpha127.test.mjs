import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, confirmActionURL, dataModule, reactURL, replaceLazyImport, requestProgressURL, sharedLayer } from './fixtures/shared-layer.mjs';

// alpha.127: the real steps of the five flows alpha.126 promised (certificate,
// backup restore, application install, PHP version, WordPress tools), read
// from the server while the change runs; a failure names the step it stopped
// at, a refused check is an unmet prerequisite, a lost answer is "not known"
// (D-024); a reload reopens the window; the first-login tour never covers it.
// alpha.127: beş akışın gerçek adımları, hata durduğu adımı söyler.

const t = (key, vars) => (vars ? `${key}${JSON.stringify(vars)}` : key);
const progress = await import(requestProgressURL);
const settled = () => new Promise((resolve) => setTimeout(resolve, 0));
const states = (steps) => steps.map((step) => `${step.id}=${step.state}${step.prerequisite ? '!' : ''}`);

test('the window shows the server plan with the step it is at', () => {
    const labels = progress.sslIssueSteps(t, 'example.com', "Let's Encrypt");
    // Before the server names a step the first one runs.
    assert.deepEqual(states(progress.progressSteps(labels, t, undefined, undefined, 1, 'running')),
        ['check=running', 'dns_check=pending', 'prepare_validation=pending', 'acme=pending', 'verify=pending', 'install=pending', 'reload=pending', 'mail=pending']);
    // The server's plan decides which steps are shown (no mail step here),
    // and a step code the screen has no words for never reaches it.
    const plan = ['check', 'dns_check', 'prepare_validation', 'acme', 'verify', 'install', 'reload', 'surprise'];
    const running = progress.progressSteps(labels, t, plan, 'acme', 5, 'running');
    assert.deepEqual(states(running), ['check=done', 'dns_check=done', 'prepare_validation=done', 'acme=running', 'verify=pending', 'install=pending', 'reload=pending']);
    const acme = running.find((step) => step.id === 'acme');
    assert.equal(acme.since, 5);
    assert.ok(acme.label.includes("Let's Encrypt") && acme.note === 'opstep.ssl.acmeNote');
    assert.deepEqual(states(progress.progressSteps(labels, t, plan, 'reload', 5, 'done')).filter((s) => !s.endsWith('=done')), []);
});

test('a refusal marks the step it stopped at; a refused check is an unmet prerequisite; a lost answer is not known', () => {
    const labels = progress.sslIssueSteps(t, 'example.com', 'ZeroSSL');
    const refusedCheck = progress.progressSteps(labels, t, undefined, 'dns_check', 1, 'failed', 409);
    assert.deepEqual(states(refusedCheck).slice(0, 3), ['check=done', 'dns_check=failed!', 'prepare_validation=pending']);
    assert.equal(refusedCheck[1].note, 'opstep.stoppedBeforeChange');
    // The same step failing on the server's side (5xx) is a failure, not a prerequisite.
    assert.equal(progress.progressSteps(labels, t, undefined, 'dns_check', 1, 'failed', 502)[1].prerequisite, false);
    // A step that changes something is never a prerequisite.
    const acme = progress.progressSteps(labels, t, undefined, 'acme', 1, 'failed', 409).find((step) => step.id === 'acme');
    assert.equal(acme.state, 'failed');
    assert.equal(acme.prerequisite, false);
    assert.equal(acme.note, 'opstep.stoppedHere');
    const lost = progress.progressSteps(progress.backupRestoreSteps(t), t, undefined, 'restore', 1, 'unknown');
    assert.deepEqual(states(lost), ['check=done', 'inspect=done', 'restore=waiting', 'finish=pending']);
    assert.equal(lost[2].note, 'opstep.stepUnknown');
    assert.deepEqual(progress.phpVersionSteps(t, '8.4').map((step) => step.id), ['check', 'pool', 'save', 'web_server']);
    assert.deepEqual(progress.backupCreateSteps(t).map((step) => step.id), ['check', 'create', 'finish']);
});

// A minimal browser for the polling: a session store and the progress reads.
function browser(answers) {
    const store = new Map();
    const reads = [];
    globalThis.window = {
        setTimeout, clearTimeout, setInterval, clearInterval,
        sessionStorage: {
            getItem: (key) => (store.has(key) ? store.get(key) : null),
            setItem: (key, value) => store.set(key, String(value)),
            removeItem: (key) => store.delete(key),
        },
    };
    globalThis.fetch = async (url, init) => {
        reads.push([String(url), init?.method ?? 'GET']);
        const next = answers.length > 1 ? answers.shift() : answers[0];
        return new Response(JSON.stringify(next.body), { status: next.status ?? 200, headers: { 'Content-Type': 'application/json' } });
    };
    return { store, reads };
}

test('while the change runs its step is read every 1.5 s, by GET only, and the answer names where it ended', async () => {
    const { store, reads } = browser([{ body: { state: 'running', plan: ['check', 'inspect', 'restore', 'finish'], step: 'restore', step_started_at: '2026-11-06T10:00:00Z' } }]);
    const shown = [];
    const followed = progress.followRequestProgress({ steps: (steps) => shown.push(steps) }, progress.backupRestoreSteps(t), t, 'restore.7', { backup: 'files-1.cpbak' });
    const id = followed.headers['X-CelikPanel-Request-Id'];
    assert.match(id, /^[0-9a-f]{32}$/);
    // Kept in this tab's session while it runs, with the words a reload needs.
    assert.deepEqual(JSON.parse(store.get('celikpanel.op.restore.7')).context, { backup: 'files-1.cpbak' });
    await new Promise((resolve) => setTimeout(resolve, 1700));
    assert.deepEqual(reads[0], [`/api/v1/operation-progress/${id}`, 'GET']);
    assert.ok(reads.every(([, method]) => method === 'GET'), 'polling sent a change');
    assert.deepEqual(states(shown[shown.length - 1]), ['check=done', 'inspect=done', 'restore=running', 'finish=pending']);
    // The Agent refused: the answer's header names the step.
    followed.settle(new Response('{"error":"damaged"}', { status: 502, headers: { 'X-CelikPanel-Operation-Step': 'restore' } }));
    assert.deepEqual(states(shown[shown.length - 1]), ['check=done', 'inspect=done', 'restore=failed', 'finish=pending']);
    assert.equal(store.has('celikpanel.op.restore.7'), false, 'the ended change is still kept for a reload');
    const count = reads.length;
    await new Promise((resolve) => setTimeout(resolve, 1700));
    assert.equal(reads.length, count, 'it kept reading after the answer');
    // A second settle (the finally block) changes nothing.
    followed.settle(null);
    assert.deepEqual(states(shown[shown.length - 1]), ['check=done', 'inspect=done', 'restore=failed', 'finish=pending']);
});

test('after a reload the window reopens at the server step and ends with the reason; a refused check is blocked', async () => {
    const { store } = browser([
        { body: { state: 'running', plan: ['check', 'pool', 'save', 'web_server'], step: 'pool' } },
        { body: { state: 'done', plan: ['check', 'pool', 'save', 'web_server'], step: 'check', response_status: 400, answer: { error: 'PHP version is not available' } } },
    ]);
    store.set('celikpanel.op.php.9', JSON.stringify({ id: 'a'.repeat(32), at: Date.now(), context: { version: '8.4' } }));
    globalThis.operationSteps = [];
    globalThis.operationEnds = [];
    let ended = 0;
    progress.resumeRequestProgress({
        key: 'php.9', labels: (kept) => progress.phpVersionSteps(t, kept.version), t,
        progressTitle: (kept) => `title ${kept.version}`, refusedSentence: 'php.changeFailed', ended: () => { ended++; },
    });
    await new Promise((resolve) => setTimeout(resolve, 1800));
    assert.deepEqual(states(globalThis.operationSteps[0]), ['check=done', 'pool=running', 'save=pending', 'web_server=pending']);
    assert.ok(globalThis.operationSteps[0][1].label.includes('8.4'), 'the version the change was sent with is lost after the reload');
    assert.deepEqual(states(globalThis.operationSteps.at(-1)), ['check=failed!', 'pool=pending', 'save=pending', 'web_server=pending']);
    assert.equal(globalThis.operationEnds[0].state, 'blocked');
    assert.ok(globalThis.operationEnds[0].reason.includes('PHP version is not available'));
    assert.equal(ended, 1);
    assert.equal(store.size, 0);
    // Nothing kept: nothing is followed.
    progress.resumeRequestProgress({ key: 'php.9', labels: () => [], t, progressTitle: () => '', refusedSentence: 'php.changeFailed' });
    await settled();
    assert.equal(globalThis.operationEnds.length, 1);
});

test('a change whose process restarted is not known after a reload, and one the server forgot is not followed blind', async () => {
    const { store } = browser([{ body: { state: 'unknown', step: 'acme' } }]);
    store.set('celikpanel.op.ssl.3', JSON.stringify({ id: 'b'.repeat(32), at: Date.now(), context: { authority: 'ZeroSSL' } }));
    globalThis.operationSteps = [];
    globalThis.operationEnds = [];
    progress.resumeRequestProgress({ key: 'ssl.3', labels: (kept) => progress.sslIssueSteps(t, 'example.com', kept.authority), t, progressTitle: () => '', refusedSentence: 'ssl.issueFailed' });
    await new Promise((resolve) => setTimeout(resolve, 50));
    assert.deepEqual(globalThis.operationEnds[0], { state: 'unknown', reason: 'opstep.interrupted' });
    assert.equal(globalThis.operationSteps.at(-1).find((step) => step.id === 'acme').state, 'waiting');
});

// The window itself: a failed check reads as "someone must act first", with
// the step marked; another failure reads as a failure (D-024).
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null;
  export const HelpCircle = () => null;
  export const CheckCircle2 = () => null;
  export const Clock = () => null;
  export const Loader2 = () => null;
  export const XCircle = () => null;
  export const useI18n = () => ({ t: (key) => key, locale: 'en' });
  export const useNavigate = () => () => {};
  export const captureToasts = () => () => {};
  export const showToast = () => {};
`);
const shared = sharedLayer(stub);
const progressWindowURL = shared.compile('components/OperationProgress.tsx');
const hostURL = replaceLazyImport(shared.compile('components/ConfirmHost.tsx'), './OperationProgress', progressWindowURL);
const { ConfirmHost } = await import(hostURL);
const { confirmAction } = await import(confirmActionURL);

async function windowEnd(steps, end) {
    globalThis.document = { activeElement: null, getElementById: () => null, addEventListener() {}, removeEventListener() {} };
    globalThis.window = { setInterval, clearInterval, setTimeout, clearTimeout };
    let tree;
    await act(async () => { tree = Renderer.create(React.createElement(ConfirmHost)); });
    let answer;
    await act(async () => {
        answer = confirmAction({ message: 'Change it?', progressTitle: 'Changing', action: async (report) => { report.steps(steps); return end; } });
        await settled();
    });
    const confirm = tree.root.findAll((node) => node.type === 'button' && node.props.id === 'confirm-action-confirm')[0];
    await act(async () => { confirm.props.onClick({}); await settled(); await settled(); await settled(); });
    const text = JSON.stringify(tree.toJSON());
    const marks = tree.root.findAll((node) => node.props['data-operation-step'] !== undefined)
        .map((node) => `${node.props['data-operation-step']}=${node.props['data-step-state']}`);
    const close = tree.root.findAll((node) => node.type === 'button' && node.props['data-operation-close'] !== undefined)[0];
    await act(async () => { close.props.onClick({}); await settled(); });
    await answer;
    await act(async () => tree.unmount());
    return { text, marks };
}

test('the window names the step that failed, says the reason, and tells a refused check from a failure', async () => {
    const labels = progress.sslIssueSteps(t, 'example.com', 'ZeroSSL');
    const refused = await windowEnd(progress.progressSteps(labels, t, undefined, 'dns_check', 1, 'failed', 409),
        { state: 'failed', reason: 'example.com points to another server. Change its A record, then try again.' });
    assert.ok(refused.text.includes('operation.blocked') && !refused.text.includes('operation.failed'), 'a refused check read as a failure');
    assert.ok(refused.text.includes('points to another server') && refused.text.includes('opstep.stoppedBeforeChange'));
    assert.deepEqual(refused.marks.slice(0, 3), ['check=done', 'dns_check=failed', 'prepare_validation=pending']);

    const failed = await windowEnd(progress.progressSteps(labels, t, undefined, 'acme', 1, 'failed', 502),
        { state: 'failed', reason: 'The authority could not reach the site.' });
    assert.ok(failed.text.includes('operation.failed') && failed.text.includes('opstep.stoppedHere'));
    assert.ok(failed.marks.includes('acme=failed'));
});

// The application install and the WordPress tools follow their operation's
// own durable status.
test('application install and WordPress steps follow the durable status', async () => {
    const appStub = dataModule(`
      import React from '${reactURL}';
      const icon = () => null;
      export const LayoutGrid = icon, Download = icon, ExternalLink = icon, Loader2 = icon, RotateCcw = icon, Trash2 = icon, KeyRound = icon,
        ChevronDown = icon, ChevronUp = icon, Copy = icon, RefreshCw = icon, AlertTriangle = icon;
      export const showToast = () => {};
      export const useI18n = () => ({ t: (key) => key, locale: 'en' });
      export const useNavigate = () => () => {};
      export const RUNNING_STATES = [];
      export const decodeDomainApps = () => ({});
      export const appText = (app, field) => app[field];
      export const domainAppInstallURL = () => '';
      export const domainAppOperationURL = () => '';
      export const domainAppsURL = () => '';
      export const phpExtensionsInstallPath = '';
      export const Guidance = () => null, whoWord = () => '';
      export const sendSiteTool = async () => new Response('{}');
      // alpha.127 (admin next step): WordPress tools link the administrator to PHP-FPM.
      export const AdminInstallLink = (props) => props.fallback ?? null;
    `);
    const layer = sharedLayer(appStub);
    const apps = await import(layer.compile('components/DomainAppsPanel.tsx'));
    const wp = await import(layer.compile('components/WordPressToolsPanel.tsx'));
    assert.deepEqual(states(apps.appInstallSteps(t, 'Joomla', true, 'files', 1, null)), ['check=done', 'database=done', 'files=running', 'setup=pending', 'finish=pending']);
    assert.deepEqual(states(apps.appInstallSteps(t, 'WordPress', false, 'check', 1, 'prerequisite')), ['check=failed!', 'database=pending', 'files=pending', 'finish=pending']);
    assert.deepEqual(states(apps.appInstallSteps(t, 'Joomla', true, 'setup', 1, 'failed')).slice(3), ['setup=failed', 'finish=pending']);
    assert.deepEqual(states(wp.wordpressSteps(t, 'update', 'backing_up', 1, null, false, '')), ['start=done', 'queued=done', 'backing_up=running', 'updating=pending', 'finish=pending']);
    // An update that failed its check: the update step failed, the restore after it runs.
    assert.deepEqual(states(wp.wordpressSteps(t, 'update', 'restoring', 1, null, true, '')), ['start=done', 'queued=done', 'backing_up=done', 'updating=failed', 'restoring=running', 'finish=pending']);
    const clone = wp.wordpressSteps(t, 'clone', 'cloning', 1, 'unknown', false, 'copy.example');
    assert.deepEqual(states(clone), ['start=done', 'queued=done', 'cloning=waiting', 'finish=pending']);
    assert.ok(clone[2].label.includes('copy.example'));
});

// The screens send the change with the identity the window reads, and the
// first-login tour never opens above, or while, the locked window.
test('the five screens follow the server steps, and the tour waits for the operation window', () => {
    const source = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');
    for (const [path, steps] of [
        ['components/DomainSSLSettings.tsx', 'sslIssueSteps'],
        ['components/DomainBackupManager.tsx', 'backupRestoreSteps'],
        ['components/DomainBackupManager.tsx', 'backupCreateSteps'],
        ['components/DomainPHPSettings.tsx', 'phpVersionSteps'],
    ]) {
        const text = source(path);
        assert.ok(text.includes(`followRequestProgress(report, ${steps}(`), `${path} does not follow ${steps}`);
        assert.ok(text.includes('...progress.headers'), `${path} does not send the identity the window reads`);
        assert.ok(text.includes('progress.settle(res)'), `${path} does not mark where the change ended`);
        assert.ok(text.includes('resumeRequestProgress('), `${path} does not reopen the window after a reload`);
    }
    assert.ok(source('components/DomainAppsPanel.tsx').includes('followOperation('));
    assert.ok(source('components/WordPressToolsPanel.tsx').includes('followOperation('));
    const help = source('help/guide/HelpControl.tsx');
    assert.ok(help.includes("tour === 'tour' && !operationOpen") && help.includes("tour === 'offer' && !operationOpen"), 'the tour can open while an operation window is up');
    assert.ok(help.includes("'[data-confirm-host]'"));
    const overlay = /data-tour-overlay/.test(source('help/guide/Tour.tsx')) && source('help/guide/Tour.tsx').match(/fixed inset-0 z-\[(\d+)\]" data-tour-overlay/);
    assert.ok(overlay && Number(overlay[1]) < 60, 'the tour overlay sits above the stacked operation window (z-60)');
});

// "Always, for every operation": the other confirmed changes follow the
// server's own steps too; shared steps have one set of words.
test('flowSteps names each step by its flow, shares the common words and marks checks', () => {
    const steps = progress.flowSteps(t, 'dns', ['check!', 'save', 'publish']);
    assert.deepEqual(steps.map((step) => [step.id, step.label, !!step.check]), [
        ['check', 'opstep.dns.check{}', true], ['save', 'opstep.dns.save{}', false], ['publish', 'opstep.dns.publish{}', false],
    ]);
    assert.deepEqual(progress.flowSteps(t, 'sslRemove', ['detach', 'reload', 'mail']).map((step) => step.label),
        ['opstep.sslRemove.detach{}', 'opstep.common.reload{}', 'opstep.common.mail{}']);
    const en = readFileSync(new URL('../src/i18n/screens/opsteps/en.ts', import.meta.url), 'utf8');
    for (const [flow, ids] of Object.entries({
        sslUpload: ['validate', 'install'], sslRemove: ['detach'], mailCert: ['check', 'mail_tls', 'tlsa'], dns: ['check', 'save', 'publish'],
        pdns: ['check', 'configure', 'sync'], phpIni: ['check', 'apply'], hosting: ['check', 'switch'],
        service: ['check', 'start', 'stop', 'restart', 'refresh'], malware: ['check', 'quarantine', 'restore', 'delete', 'save'],
        dbDelete: ['check', 'delete'], adminRemove: ['check', 'remove'], userDelete: ['delete'],
        common: ['reload', 'mail', 'records'],
    })) {
        for (const id of ids) assert.ok(en.includes(`'opstep.${flow}.${id}'`), `no words for ${flow}.${id}`);
    }
});

test('sendWithSteps sends the identity the window reads and marks a lost answer as not known', async () => {
    browser([{ body: { state: 'running', step: 'check' } }]);
    const shown = [];
    let sent;
    await assert.rejects(progress.sendWithSteps({ steps: (steps) => shown.push(steps) }, progress.flowSteps(t, 'dns', ['check!', 'save', 'publish']), t, 'dns.x',
        async (headers) => { sent = headers; throw new TypeError('network'); }));
    assert.match(sent['X-CelikPanel-Request-Id'], /^[0-9a-f]{32}$/);
    assert.deepEqual(states(shown.at(-1)), ['check=waiting', 'save=pending', 'publish=pending']);
    const answer = await progress.sendWithSteps({ steps: (steps) => shown.push(steps) }, progress.flowSteps(t, 'dns', ['check!', 'save', 'publish']), t, 'dns.x',
        async () => new Response('{}', { status: 409, headers: { 'X-CelikPanel-Operation-Step': 'check' } }));
    assert.equal(answer.status, 409);
    assert.deepEqual(states(shown.at(-1)), ['check=failed!', 'save=pending', 'publish=pending']);
});

test('the other confirmed changes follow the server steps', () => {
    const source = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');
    for (const [path, flow] of [
        ['components/DomainSSLSettings.tsx', "'sslUpload'"], ['components/DomainSSLSettings.tsx', "'sslRemove'"], ['components/DomainSSLSettings.tsx', "'mailCert'"],
        ['components/DomainDNSManager.tsx', "'dns'"], ['components/PowerDNSManagement.tsx', "'pdns'"], ['components/PHPExtendedConfig.tsx', "'phpIni'"],
        ['components/HostingTypePanel.tsx', "'hosting'"], ['components/ServiceShell.tsx', "'service'"], ['components/DomainMalwareScan.tsx', "'malware'"],
        ['components/databases/DatabasesPage.tsx', "'dbDelete'"], ['components/DomainDatabaseManager.tsx', "'dbDelete'"],
        ['components/DatabaseRemoteServerPanel.tsx', "'adminRemove'"], ['components/UsersPage.tsx', "'userDelete'"],
    ]) {
        const text = source(path);
        assert.ok(text.includes(`flowSteps(t, ${flow}`) && text.includes('sendWithSteps(report'), `${path} does not follow ${flow}`);
    }
});
