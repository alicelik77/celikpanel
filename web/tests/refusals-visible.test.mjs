// A refused change is seen and understood (owner feedback 2026-10-31): the
// screen's own sentence keeps the server's reason, the fix is a button, the
// code stays under Technical details, a refusal that blocks work stays on the
// page, and a password the server made up is shown until it is closed.
//
// Reddedilen degisiklik gorulur ve anlasilir.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const require = createRequire(import.meta.url);
const reactURL = pathToFileURL(require.resolve('react')).href;
const dataURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`;
const read = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const compile = path => ts.transpileModule(read(path), { compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const apiErrorURL = dataURL(compile('../src/lib/apiError.ts'));
const { refusalText, refusalToast } = await import(apiErrorURL);
const catalogue = { ...enScreens, 'err.DOMAIN_HAS_MAIL': 'The domain still has mailboxes.', 'err.DOMAIN_HAS_MAIL.action': 'Open mail' };
const t = (key, vars) => catalogue[key] === undefined ? key : String(catalogue[key]).replace(/\{(\w+)\}/g, (all, name) => vars?.[name] ?? all);
const textOf = node => node === null || node === undefined || typeof node === 'boolean' ? '' : typeof node === 'string' || typeof node === 'number' ? String(node) : Array.isArray(node) ? node.map(textOf).join(' ') : textOf(node.children);

test('a refusal says the screen sentence with the server reason, or the coded sentence, and carries its fix and code', () => {
    assert.equal(refusalText(null, t, 'domains.deleteFailed'), 'The domain was not deleted');
    assert.equal(refusalText({ message: 'quota reached' }, t, 'users.createFailed'), 'The user was not created. The server said: quota reached');
    assert.equal(refusalText({ message: 'raw', code: 'DOMAIN_HAS_MAIL' }, t, 'domains.deleteFailed'), 'The domain still has mailboxes.');
    // A code with no words of its own keeps the screen's sentence and the reason.
    assert.equal(refusalText({ message: 'raw words', code: 'UNKNOWN_CODE' }, t, 'domains.deleteFailed'), 'The domain was not deleted. The server said: raw words');
    const went = [];
    const extra = refusalToast({ message: 'x', code: 'DOMAIN_HAS_MAIL', action: '/domains/a#mail' }, t, to => went.push(to));
    assert.equal(extra.action.label, 'Open mail');
    extra.action.run();
    assert.deepEqual(went, ['/domains/a#mail']);
    assert.deepEqual(extra.details, { label: enScreens['problem.details'], text: 'DOMAIN_HAS_MAIL' });
    assert.equal(refusalToast({ message: 'no code' }, t), undefined);
});

test('a toast carries the fix button and the code under Technical details', async () => {
    const lucide = dataURL('export const CheckCircle = () => null, XCircle = () => null, Info = () => null, AlertTriangle = () => null, X = () => null;');
    const { ToastContainer, showToast } = await import(dataURL(`import React from '${reactURL}';\n` + compile('../src/components/Toast.tsx').replace(/from ['"]([^'"]+)['"]/g, (_, from) => `from '${from === 'react' ? reactURL : lucide}'`)));
    let view, ran = 0;
    await act(async () => { view = Renderer.create(React.createElement(ToastContainer)); });
    await act(async () => showToast('error', 'Refused.', { action: { label: 'Fix it', run: () => { ran++; } }, details: { label: 'Technical details', text: 'SOME_CODE' } }));
    const code = view.root.findByType('code');
    assert.equal(code.props.children, 'SOME_CODE');
    assert.equal(code.parent.type, 'details');
    await act(async () => view.root.findAll(node => node.type === 'button' && textOf(node) === 'Fix it')[0].props.onClick());
    assert.equal(ran, 1);
    assert.ok(!textOf(view.toJSON()).includes('Refused.'), 'the toast closes when its fix runs');
    await act(async () => view.unmount());
});

test('a password the server made up stays until closed, can be copied and is never logged', async () => {
    const stub = dataURL(`import React from '${reactURL}';
export const AlertTriangle = () => null;
export const useI18n = () => ({ t: key => key });
export const Button = ({ loading, ...props }) => React.createElement('button', props);`);
    const { ShownOnceSecret } = await import(dataURL(`import React from '${reactURL}';\n` + compile('../src/components/OnceOnlyNotice.tsx').replace(/from ['"]([^'"]+)['"]/g, (_, from) => `from '${from === 'react' ? reactURL : stub}'`)));
    const logged = [];
    const saved = { log: console.log, info: console.info, error: console.error, warn: console.warn, debug: console.debug };
    for (const name of Object.keys(saved)) console[name] = (...args) => logged.push(args);
    const copied = [];
    const navigatorBefore = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
    Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { clipboard: { writeText: async value => { copied.push(value); } } } });
    let view, closed = 0;
    try {
        await act(async () => { view = Renderer.create(React.createElement(ShownOnceSecret, { title: 'Created', rows: [{ label: 'User', value: 'shop_app' }], secret: { label: 'Password', value: 's3cret-Value' }, onClose: () => { closed++; } })); });
        const box = view.root.findByProps({ 'data-shown-once': '' });
        assert.equal(box.props.role, 'alert');
        assert.ok(textOf(box).includes('s3cret-Value') && textOf(box).includes('secret.shownOnce'));
        await act(async () => view.root.findAll(node => node.type === 'button' && textOf(node) === 'secret.copy')[0].props.onClick());
        assert.deepEqual(copied, ['s3cret-Value']);
        assert.ok(textOf(view.toJSON()).includes('secret.copied'));
        await act(async () => view.root.findAll(node => node.type === 'button' && textOf(node) === 'secret.saved')[0].props.onClick());
        assert.equal(closed, 1);
        assert.deepEqual(logged, [], 'nothing is written to the console');
        await act(async () => view.unmount());
    } finally {
        Object.assign(console, saved);
        if (navigatorBefore) Object.defineProperty(globalThis, 'navigator', navigatorBefore); else delete globalThis.navigator;
    }
    const { generatedPassword } = await import(dataURL(compile('../src/lib/generatedPassword.ts')));
    const one = generatedPassword(), two = generatedPassword();
    assert.equal(one.length, 20);
    assert.notEqual(one, two);
    assert.match(one, /^[A-Za-z0-9._-]+$/);
    assert.doesNotMatch(read('../src/lib/generatedPassword.ts') + read('../src/components/AddDatabaseModalV2.tsx') + read('../src/components/AddUserModalV2.tsx'), /Math\.random/);
});

// Each screen named in the follow-up keeps a refusal on the page where it
// happened and no longer says a bare "Something went wrong" for a refusal.
test('refusals stay where they happened, with the server reason, on the named screens', () => {
    const screens = {
        'DomainMailManager.tsx': 'mail.quotaFailed', 'UsersPage.tsx': 'users.createFailed', 'DomainSSLSettings.tsx': 'ssl.uploadFailed',
        'DomainDNSManager.tsx': 'dns.recordAddFailed', 'Domains.tsx': 'domains.deleteFailed', 'DomainAppsPanel.tsx': 'apps.installFailed',
        'DatabaseManagementV2.tsx': 'databases.deleteDbFailed', 'DomainBackupManager.tsx': 'backup.restoreFailed', 'MailAuthPanel.tsx': 'mailauth.keyFailed',
        'TeamMembersPage.tsx': 'team.saveFailed', 'DomainDatabaseManager.tsx': 'ddb.deleteFailed', 'AddonsPage.tsx': 'addons.grantFailed',
    };
    for (const [file, key] of Object.entries(screens)) {
        const source = read(`../src/components/${file}`);
        assert.ok(source.includes(`'${key}'`), `${file} names ${key}`);
        assert.match(source, /<ErrorBanner[^>]*sentence=/, `${file} keeps the refusal on the page`);
        assert.match(source, /refusalToast\(/, `${file} gives the toast its fix and code`);
        assert.doesNotMatch(source, /showToast\('error', t\('common\.error'\)\)/, `${file} says a bare "Something went wrong"`);
        assert.ok(enScreens[key] && trScreens[key], `${key} in both languages`);
    }
    // The Store's refusal is not "the Store could not be loaded".
    const addons = read('../src/components/AddonsPage.tsx');
    const action = addons.slice(addons.indexOf('const runStoreAction'), addons.indexOf('const selectedSubscription'));
    assert.doesNotMatch(action, /setStoreError\(error\)/);
    assert.match(action, /setActionError\(\{ error, sentence \}\)/);
    // Domain delete refusal keeps its fix: the toast gets navigate.
    assert.match(read('../src/components/Domains.tsx'), /refusalToast\(apiError, t, navigate\)/);
});

test('the database screens and forms speak the catalogue, in both languages', () => {
    for (const file of ['DomainDatabaseManager.tsx', 'AddDatabaseModalV2.tsx', 'AddUserModalV2.tsx']) {
        const source = read(`../src/components/${file}`);
        // No English sentence between tags, in a title, a placeholder or a toast.
        assert.doesNotMatch(source, />\s*[A-Z][a-z]+(?: [a-zA-Z]+)+\s*</, `${file} has English between tags`);
        assert.doesNotMatch(source, /(?:title|placeholder|aria-label)="[A-Z][a-z]+ [a-z]/, `${file} has an English attribute`);
        assert.doesNotMatch(source, /showToast\('[a-z]+', '[A-Z]/, `${file} toasts English`);
        assert.doesNotMatch(source, /showToast\('[a-z]+', `[A-Z]/, `${file} toasts English`);
    }
    for (const key of Object.keys(enScreens).filter(key => /^(ddb|dbm|secret)\./.test(key))) assert.ok(trScreens[key], `${key} in Turkish`);
});

test('the raw messages on the panel certificate and license screens are under Technical details', () => {
    const settings = read('../src/components/Settings.tsx');
    assert.doesNotMatch(settings, /showToast\('error', operation\.error\?\.message/);
    assert.doesNotMatch(settings, /e instanceof Error && e\.message \? e\.message/);
    assert.match(settings, /data-panel-certificate-reason/);
    const license = read('../src/components/LicensePanel.tsx');
    assert.doesNotMatch(license, /cause instanceof Error \? cause\.message/);
    assert.match(license, /common\.technicalDetails/);
});

// The two setup sentences that were guesses, checked against the shipped
// files: the panel's settings file and the agent's unit.
test('the setup guidance names the real settings file and the agent service', () => {
    const unit = read('../../deploy/systemd/celikpanel-panel.service');
    assert.match(unit, /EnvironmentFile=-\/etc\/celikpanel\/panel\.env/);
    assert.match(read('../../cmd/panel/panel_cert_handler.go'), /CELIKPANEL_TLS_CERT[\s\S]*CELIKPANEL_TLS_DIR/);
    read('../../deploy/systemd/celikpanel-agent.service');
    for (const catalogueOf of [enScreens, trScreens]) {
        assert.match(catalogueOf['setup.problem.panelCertUnmanaged.todo'], /\/etc\/celikpanel\/panel\.env/);
        assert.match(catalogueOf['setup.problem.panelCertUnmanaged.todo'], /systemctl restart celikpanel-panel/);
        assert.match(catalogueOf['setup.problem.mailCertUnavailable.todo'], /systemctl restart celikpanel-agent/);
    }
});
