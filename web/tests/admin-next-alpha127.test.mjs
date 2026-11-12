// The rest of the alpha.127 sweep: uploaded programs and AI apps on hold, not
// offered or on a reduced sandbox; PHP tools whose PHP-FPM is stopped; mail
// signing that fails; and the package-manager commands on the update screens.
// The administrator reads what to do and gets the page that holds the action
// (or the command, ready to copy, where CelikPanel has no install path of its
// own); everyone else keeps the sentence that says who acts.
// alpha.127 taramasının kalanı: yönetici eylemin sayfasını ya da kopyalanmaya
// hazır komutu alır; diğerleri kimin yapacağını okur.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { compileSource, dataModule } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';
import { enAdminNextScreens as en } from '../src/i18n/screens/adminNext/en.ts';
import { trAdminNextScreens as tr } from '../src/i18n/screens/adminNext/tr.ts';

const stub = dataModule(`
  const i18n = { t: (key, vars) => (vars ? key + JSON.stringify(vars) : key), locale: 'en' };
  export const useI18n = () => i18n;
  export const useAuth = () => ({ role: globalThis.stepRole });
  export const useNavigate = () => (path) => { globalThis.stepNavigated.push(path); return true; };
`);
const { AdminInstallLink } = await import(compileSource('components/AdminInstallLink.tsx', () => stub));
const { CopyCommand } = await import(compileSource('components/CopyCommand.tsx', () => stub));
const read = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const text = (tree) => JSON.stringify(tree.toJSON());

async function render(role, element) {
  globalThis.stepRole = role;
  globalThis.stepNavigated = [];
  let tree;
  await act(async () => { tree = Renderer.create(element); });
  return tree;
}

test('the administrator reads their own sentence and gets the page that holds the action', async () => {
  const props = { to: '/addons?view=catalog', label: 'Open Catalog management', admin: 'Lift the hold.', fallback: 'The administrator has put it on hold.' };
  const admin = await render('admin', React.createElement(AdminInstallLink, props));
  assert.ok(text(admin).includes('Lift the hold.'));
  assert.ok(!text(admin).includes('The administrator has put it on hold.'));
  const button = admin.root.findByType('button');
  await act(async () => button.props.onClick());
  assert.deepEqual(globalThis.stepNavigated, ['/addons?view=catalog']);
  for (const role of ['customer', 'reseller', 'additional_user']) {
    const other = await render(role, React.createElement(AdminInstallLink, props));
    assert.equal(text(other), '"The administrator has put it on hold."', role);
  }
  // A "who acts" line: the administrator reads "you", with no button.
  const who = await render('admin', React.createElement(AdminInstallLink, { admin: 'you', fallback: 'the administrator' }));
  assert.equal(text(who), '"you"');
});

test('a command is shown whole with one Copy button; copying runs nothing and says what happened', async () => {
  const written = [];
  const before = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { clipboard: { writeText: async (value) => { written.push(value); } } } });
  try {
    const tree = await render('admin', React.createElement(CopyCommand, { command: 'sudo apt install logrotate' }));
    assert.ok(text(tree).includes('sudo apt install logrotate'));
    assert.ok(text(tree).includes('adminNext.runAsRoot'));
    await act(async () => tree.root.findByType('button').props.onClick());
    assert.deepEqual(written, ['sudo apt install logrotate']);
    assert.ok(text(tree).includes('adminNext.copied'));
    Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { clipboard: { writeText: async () => { throw new Error('denied'); } } } });
    const failing = await render('admin', React.createElement(CopyCommand, { command: 'x', label: 'On Arch Linux, as root:' }));
    assert.ok(text(failing).includes('On Arch Linux, as root:'));
    await act(async () => failing.root.findByType('button').props.onClick());
    assert.ok(text(failing).includes('adminNext.copyFailed'));
  } finally {
    if (before) Object.defineProperty(globalThis, 'navigator', before); else delete globalThis.navigator;
  }
});

test('uploaded programs: hold, offer, plan, sandbox and limits send the administrator to where they decide', () => {
  const source = read('components/DomainRuntimeApps.tsx');
  for (const code of ['held', 'sandbox_reduced', 'not_offered', 'oom', 'wheel_unavailable']) {
    assert.match(source, new RegExp(`${code}: \\{ to: CATALOG, label: 'adminNext\\.openCatalog' \\}`), code);
    assert.ok(en[`adminNext.runtime.next.${code}`] && tr[`adminNext.runtime.next.${code}`], code);
  }
  assert.match(source, /suspended: \{ to: '\/users', label: 'adminNext\.openUsers' \}/);
  assert.match(source, /const CATALOG = '\/addons\?view=catalog';/);
  assert.match(source, /ADMIN_WHO\.has\(guide\.who\) \? <AdminInstallLink admin=\{t\('adminNext\.you'\)\} fallback=\{who\} \/>/);
  for (const [key, label] of [['adminNext.runtime.held', 'openCatalog'], ['adminNext.runtime.notGranted', 'openUsers'], ['adminNext.runtime.notOffered', 'openCatalog'], ['adminNext.runtime.sandboxReduced', 'openCatalog']]) {
    assert.match(source, new RegExp(`text: t\\('${key.replace(/\./g, '\\.')}'[^}]*\\}?\\)?, to: [^,]+, label: 'adminNext\\.${label}'`), key);
  }
  // The Addons page opens on Catalog management from the link.
  assert.match(read('components/AddonsPage.tsx'), /searchParams\.get\('view'\) === 'catalog'/);
});

test('AI apps on hold or not offered send the administrator to Catalog management', () => {
  const source = read('components/DomainServiceApps.tsx');
  assert.match(source, /if \(app\.held\) return 'adminNext\.svcapps\.held';/);
  assert.match(source, /'adminNext\.svcapps\.notOffered'/);
  assert.match(source, /<AdminInstallLink to="\/addons\?view=catalog" label=\{t\('adminNext\.openCatalog'\)\} admin=\{t\(adminClosed\(app\)!\)\} fallback=\{why\} \/>/);
  assert.match(source, /admin=\{t\('adminNext\.svcapps\.held'\)\} fallback=\{t\('svcapps\.heldNote'\)\}/);
});

test('mail signing that fails again is repaired on the mail setup; others are told their provider acts', () => {
  const source = read('components/MailAuthPanel.tsx');
  assert.match(source, /navigate\('\/components#mail-stacks'\)/);
  assert.match(source, /t\(isAdmin \? 'adminNext\.mail\.keySigningFailed' : 'mailauth\.keySigningFailed'\)/);
  assert.match(source, /isAdmin \? \{ action: \{ label: t\('adminNext\.openMailSetup'\), run: openMailSetup \} \} : \{\}/);
  assert.match(source, /isAdmin && automaticText\(auto\) === 'mailauth\.auto\.signingFailed'/);
  for (const key of ['mailauth.keySigningFailed', 'mailauth.auto.signingFailed']) {
    assert.match(enScreens[key], /your provider \(the server’s administrator\) repairs the mail setup/, key);
    assert.match(trScreens[key], /sağlayıcınız \(sunucu yöneticisi\) onarır/, key);
    assert.doesNotMatch(enScreens[key], /Components/, key);
  }
  for (const key of ['adminNext.mail.keySigningFailed', 'adminNext.mail.autoSigningFailed']) assert.match(en[key], /choose Repair/, key);
});

test('a PHP tool whose PHP-FPM is stopped, or has no other version, links to PHP-FPM’s page', () => {
  const source = read('components/PHPToolVersion.tsx');
  assert.match(source, /\{stopped && <PHPFPMLink \/>\}/);
  assert.match(source, /t\('component\.phpTool\.onlyOne', \{ tool: name \}\)\} <PHPFPMLink \/>/);
  assert.match(source, /navigate\('\/components\/php-fpm'\)/);
  assert.match(enScreens['component.phpTool.stopped'], /on its page/);
});

test('the update screens give the command ready to copy instead of inside a sentence', () => {
  assert.match(read('components/SecurityUpdatesCard.tsx'), /\(auto\.state === 'off' \|\| auto\.state === 'not_installed'\) && <CopyCommand command=\{command\} \/>/);
  assert.match(read('components/OSUpdatesChoice.tsx'), /code === 'UNATTENDED_NOT_INSTALLED' \? \{ command: INSTALL_COMMAND \} : \{\}/);
  assert.match(read('components/OSUpdatesChoice.tsx'), /\{outcome\?\.command && <CopyCommand command=\{outcome\.command\} \/>\}/);
  assert.match(read('components/VendorLaneDialog.tsx'), /step\.reason === 'automatic_not_installed' && <div className="pl-6"><CopyCommand command=\{AUTOMATIC_UPDATES_COMMAND\} \/>/);
  const usage = read('components/UsageNotices.tsx');
  assert.match(usage, /command="sudo apt install logrotate"/);
  assert.match(usage, /command="sudo pacman -Syu --needed logrotate"/);
  for (const key of ['life.updates.auto.not_installed', 'life.updates.auto.off.config_off', 'life.updates.auto.off.timer_disabled', 'life.choice.failed.UNATTENDED_NOT_INSTALLED', 'lane.blocked.automatic_not_installed', 'usage.status.logrotateMissing']) {
    for (const words of [enScreens[key], trScreens[key]]) {
      assert.doesNotMatch(words, /\{command\}|sudo|[Ss]erver administrator|[Ss]unucu yöneticisi/, key);
    }
    assert.match(enScreens[key], /command below/, key);
    assert.match(trScreens[key], /aşağıdaki komutla/, key);
  }
});

test('every administrator sentence is in English and Turkish with the same values and never asks a provider', () => {
  for (const key of Object.keys(en)) {
    assert.ok(tr[key], key);
    assert.notEqual(tr[key], en[key], key);
    const values = (s) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join();
    assert.equal(values(tr[key]), values(en[key]), key);
    assert.doesNotMatch(en[key], /ask (your|the) (provider|administrator)|your provider/i, key);
    assert.doesNotMatch(tr[key], /sağlayıcınız/i, key);
  }
});

test('a refusal for a hold sends the administrator to Catalog management', () => {
  assert.match(read('components/DomainRuntimeApps.tsx'), /error\.code === 'RUNTIME_APP' && error\.reason === 'held' && <AdminInstallLink to=\{CATALOG\} label=\{t\('adminNext\.openCatalog'\)\} \/>/);
  assert.match(read('components/DomainServiceApps.tsx'), /failure\?\.code === 'SERVICE_APP' && failure\.reason === 'held' && <AdminInstallLink to="\/addons\?view=catalog" label=\{t\('adminNext\.openCatalog'\)\} \/>/);
});

test('an install note’s link opens the tool’s page in place through the root’s navigate', () => {
  const provider = read('components/ComponentOperation.tsx');
  assert.match(provider, /export function useOperationNoteOpener\(navigate: \(to: string\) => unknown\): void/);
  assert.match(provider, /const open = noteOpener;/);
  const app = read('App.tsx');
  // The routes register the router's navigate; the authentication gate stays without navigation.
  const routes = app.slice(app.indexOf('function AppRoutes()'), app.indexOf('function AuthGate()'));
  assert.match(routes, /useOperationNoteOpener\(useNavigate\(\)\);/);
  const notes = read('lib/componentOperationNotes.ts');
  assert.match(notes, /webmail: '\/components\/roundcube', phpmyadmin: '\/components\/phpmyadmin', phppgadmin: '\/components\/phppgadmin'/);
  assert.equal((notes.match(/link: toolLink\(t, /g) ?? []).length, 3, 'proof, version choice and other tools’ problems each carry the link');
});

test('WordPress tools without the site’s PHP command line link the administrator to PHP-FPM', () => {
  assert.match(read('components/WordPressToolsPanel.tsx'), /title=\{t\('wpt\.phpMissing'\)\}[\s\S]{0,200}<AdminInstallLink to="\/components\/php-fpm" label=\{t\('adminNext\.openPHPFPM'\)\} \/>/);
});

test('a reseller pool notice: the administrator raises it from the reseller’s rights; the reseller is told the administrator does', () => {
  const usage = read('components/UsageNotices.tsx');
  assert.match(usage, /t\(notice\.audience === 'administrator' \? 'adminNext\.pool80' : 'usage\.notice\.pool80', vars\)/);
  assert.match(usage, /t\(notice\.audience === 'administrator' \? 'adminNext\.pool100' : 'usage\.notice\.pool100', vars\)/);
  assert.match(usage, /onOpenAccount && notice\.audience === 'administrator' && notice\.kind\.startsWith\('pool_'\)/);
  const rights = read('components/RightsNotices.tsx');
  assert.match(rights, /forAdministrator && onSetRights && \([\s\S]{0,200}onSetRights\(notice\.user_id\)/);
  assert.match(rights, /<UsageNotices onOpenAccount=\{onOpenAccount\} \/>/);
  for (const key of ['adminNext.pool80', 'adminNext.pool100']) {
    assert.match(en[key], /\{reseller\}’s rights/, key);
    assert.doesNotMatch(en[key], /administrator can/, key);
  }
  assert.match(enScreens['usage.notice.pool80'], /administrator can raise the pool/);
});

test('the Users page opens an account’s rights or resume window from a link, once', () => {
  const users = read('components/UsersPage.tsx');
  assert.match(users, /const rights = params\.get\('rights'\);/);
  assert.match(users, /if \(found && rights !== null\) setPanel\(\{ user: found \}\);\s*else if \(found\) setSuspension\(\{ kind: 'user', id: found\.id, name: found\.username \}\);/);
  assert.match(users, /accountLinkRef\.current = true;/);
  assert.match(users, /params\.delete\('rights'\);\s*params\.delete\('resume'\);/);
  assert.match(users, /\{ replace: true \}/);
});
