// alpha.124 (D-045 follow-up, owner 4 Nov 2026): installing a component at
// component level. The install window lists what the component requires -
// installed, installed together with it first (one operation on the server),
// or missing and not installable here with the reason - and no package
// names; a PHP tool chooses the installed PHP version it runs on; the one
// repository change the owner consents to is named plainly. Mounted, in
// English and Turkish.
// Bileşen düzeyinde kurulum, bağlanmış olarak.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import { dataModule, installPreviewLibURL, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { enComponentInstallScreens } from '../src/i18n/screens/componentInstall/en.ts';
import { trComponentInstallScreens } from '../src/i18n/screens/componentInstall/tr.ts';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';

const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const words = {
  en: { ...enShell, ...enComponentsScreens, ...enComponentInstallScreens },
  tr: { ...trShell, ...trComponentsScreens, ...trComponentInstallScreens },
};
globalThis.cinstLocale = 'en';
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Loader2 = () => null, RotateCw = () => null, X = () => null;
  const fill = (text, vars) => Object.entries(vars || {}).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
  export const useI18n = () => ({ t: (key, vars) => { const w = globalThis.cinstWords[globalThis.cinstLocale]; return w[key] !== undefined ? fill(w[key], vars) : key; }, locale: globalThis.cinstLocale });
  export const useNavigate = () => () => true;
`);
globalThis.cinstWords = words;
const shared = sharedLayer(stub);
const planLib = await import(shared.compile('lib/installPlan.ts'));
const { InstallRequirements } = await import(shared.compile('components/InstallRequirements.tsx'));
const preview = await import(installPreviewLibURL);
const { InstallPreviewNotes } = await import(shared.compile('components/InstallPreviewNotes.tsx'));

const textOf = (tree) => { const walk = (n) => (typeof n === 'string' ? n : Array.isArray(n) ? n.map(walk).join('') : (n?.children || []).map(walk).join('')); return walk(tree.toJSON()); };
const known = (value) => ({ remote: { state: 'known', value, observedAt: 0 }, reading: false, retry() {} });
const components = [
  { id: 'nginx', name: 'Nginx', versions: ['1.26.3'] }, { id: 'php-fpm', name: 'PHP-FPM', versions: ['8.4.11', '8.3.20'] },
  { id: 'mariadb', name: 'MariaDB' }, { id: 'postfix', name: 'Postfix' }, { id: 'dovecot', name: 'Dovecot' },
  { id: 'phpmyadmin', name: 'phpMyAdmin' }, { id: 'roundcube', name: 'Roundcube' },
];
const roles = { 'web-server': 'Web server', 'smtp-server': 'Mail sending server', 'imap-server': 'Mailbox server' };

async function renderNeeds(raw, props = {}, locale = 'en') {
  globalThis.cinstLocale = locale;
  const plan = planLib.decodeInstallPlan({ plan: raw });
  let tree;
  let picked = '';
  await act(async () => {
    tree = Renderer.create(React.createElement(InstallRequirements, {
      name: 'phpMyAdmin', handle: known(plan), components, roleName: (token) => roles[token] ?? null,
      phpVersions: [], phpVersion: '', phpDefault: '', onPHPVersion: (v) => { picked = v; }, busy: false, ...props,
    }));
  });
  return { tree, text: textOf(tree), picked: () => picked, find: (attr) => tree.root.findAll((n) => n.props?.[attr] !== undefined) };
}

test('each requirement is said at component level with its state, and what is installed together', async () => {
  const view = await renderNeeds({
    target: 'phpmyadmin', blocked: false, together: ['mariadb', 'nginx'],
    requires: [
      { need: 'mariadb', component: 'mariadb', state: 'together' },
      { need: 'web-server', component: 'nginx', state: 'together' },
      { need: 'php-fpm', component: 'php-fpm', state: 'installed' },
    ],
  });
  assert.deepEqual(view.find('data-install-need-state').map((n) => `${n.props['data-install-need']}:${n.props['data-install-need-state']}`),
    ['mariadb:together', 'web-server:together', 'php-fpm:installed']);
  assert.match(view.text, /^Requires/);
  assert.match(view.text, /Web server: NginxNot installed yet: installed first, together with phpMyAdmin, at the distribution’s version\./);
  assert.match(view.text, /Install installs MariaDB, Nginx first, then phpMyAdmin, as one operation on the server\./);
  assert.match(view.text, /what was installed before it stays installed and the window says so/);
  // Never a package name.
  assert.doesNotMatch(view.text, /mariadb-server|php8\.\d-fpm|nginx-common/);
  assert.equal(view.find('data-install-blocked').length, 0);
});

test('a requirement that cannot be installed here turns Install off and says who acts and what next', async () => {
  const raw = {
    target: 'roundcube', blocked: true, together: [],
    requires: [
      { need: 'smtp-server', component: 'postfix', state: 'unavailable', reason: 'mail_setup' },
      { need: 'imap-server', component: 'dovecot', state: 'unavailable', reason: 'mail_setup' },
      { need: 'web-server', component: 'nginx', state: 'installed' },
    ],
  };
  const view = await renderNeeds(raw, { name: 'Roundcube' });
  assert.equal(view.find('data-install-blocked').length, 1);
  assert.match(view.text, /Install is off: Roundcube needs something that cannot be installed from this window\. Who acts: you\./);
  assert.match(view.text, /Mail sending server: PostfixNot installed\. It comes with the mail setup: set up mail with the Email card on this page first, then install Roundcube\./);
  assert.match(view.text, /Web server: NginxInstalled: 1\.26\.3/);
  const tr = await renderNeeds(raw, { name: 'Roundcube' }, 'tr');
  assert.match(tr.text, /^Gerekenler/);
  assert.match(tr.text, /Kur kapalı: Roundcube, bu pencereden kurulamayan bir şeye ihtiyaç duyuyor\. Yapacak olan: siz\./);
  assert.match(tr.text, /Posta kurulumuyla gelir/);
});

test('a PHP tool chooses the installed PHP version it runs on; the distribution’s is marked', async () => {
  const raw = { target: 'phpmyadmin', blocked: false, together: [], requires: [{ need: 'php-fpm', component: 'php-fpm', state: 'installed' }] };
  const versions = planLib.installedPHPVersions(['8.3.20', '8.4.11', '8.4.1', 'junk']);
  assert.deepEqual(versions, ['8.4', '8.3']);
  assert.equal(planLib.defaultPHPVersion(versions, '8.3'), '8.3');
  assert.equal(planLib.defaultPHPVersion(versions, '8.2'), '8.4', 'the newest when the distribution’s is not installed');
  assert.equal(planLib.defaultPHPVersion([], '8.4'), '');
  const view = await renderNeeds(raw, { phpVersions: versions, phpVersion: '8.3', phpDefault: '8.3' });
  const buttons = view.find('data-install-php-version');
  assert.deepEqual(buttons.map((n) => n.props['data-install-php-version']), ['8.4', '8.3']);
  assert.equal(buttons[1].props['aria-checked'], true);
  assert.match(view.text, /PHP version phpMyAdmin runs onPHP 8\.4PHP 8\.3 \(the distribution’s\)/);
  await act(async () => buttons[0].props.onClick());
  assert.equal(view.picked(), '8.4');
  // No picker when PHP-FPM itself is installed together (it runs on the
  // distribution's version) or for a component that is not a PHP tool.
  const together = await renderNeeds({ ...raw, together: ['php-fpm'], requires: [{ need: 'php-fpm', component: 'php-fpm', state: 'together' }] }, { phpVersions: [] });
  assert.equal(together.find('data-install-php').length, 0);
  assert.deepEqual(planLib.PHP_VERSION_TOOLS, ['phpmyadmin', 'phppgadmin', 'roundcube']);
});

test('an answer that is not the contract is refused; the canonical member list is the server’s', () => {
  assert.throws(() => planLib.decodeInstallPlan({ plan: { target: 'x', requires: 'no', together: [], blocked: false } }));
  assert.throws(() => planLib.decodeInstallPlan({ plan: { target: 'x', requires: [{ need: 'a', state: 'maybe' }], together: [], blocked: false } }));
  assert.throws(() => planLib.decodeInstallPlan({ plan: { target: 'x', requires: [], together: ['../etc'], blocked: false } }));
  const plan = planLib.decodeInstallPlan({ plan: { target: 'fail2ban', requires: [{ need: 'nftables', component: 'nftables', state: 'together', reason: 'weird' }], together: ['nftables'], blocked: false } });
  assert.equal(plan.requires[0].reason, undefined);
  assert.equal(planLib.togetherQualifier(['php-fpm', 'nginx', 'mariadb', 'nginx']), 'mariadb,nginx,php-fpm');
  assert.equal(planLib.installPlanURL('php-fpm'), '/api/v1/service/install-plan?id=php-fpm');
});

test('what else an install brings is said by component, never by package; a repository change names the repository', async () => {
  const handle = known(preview.decodeInstallPreview({ state: 'known', installs: [], distribution: 'debian', refusals: [], notable: [
    { package: 'apache2', reason: 'apache' }, { package: 'libapache2-mod-php8.4', reason: 'apache' },
    { package: 'libssl3', reason: 'upgrade', from: '3.0.1-1', to: '3.0.2-1' }, { package: 'openssl', reason: 'upgrade', from: '3.0.1-1', to: '3.0.2-1' },
    { package: 'libpq5', reason: 'vendor_shared' },
  ] }));
  for (const locale of ['en', 'tr']) {
    globalThis.cinstLocale = locale;
    let tree;
    await act(async () => {
      tree = Renderer.create(React.createElement(InstallPreviewNotes, {
        handle, acknowledged: false, onAcknowledge() {}, componentLabel: (id) => id, avoidable: true, selected: 'PostgreSQL 18', repo: 'pgdg',
      }));
    });
    const text = textOf(tree);
    const additional = tree.root.find((n) => n.props?.['data-install-preview-group'] === 'additional');
    const shown = (node) => (typeof node === 'string' ? node : (node.children || []).map(shown).join(''));
    assert.doesNotMatch(shown(additional), /apache2|libssl3|openssl|libapache2/, `${locale}: no package name`);
    assert.match(text, locale === 'en' ? /It also bringsThe Apache web server/ : /Bununla birlikte gelenlerApache web sunucusu/);
    assert.match(text, locale === 'en' ? /2 system package\(s\) already installed, updated to a newer version/ : /Zaten kurulu 2 sistem paketi daha yeni bir sürüme güncellenir/);
    // The repository change keeps its package (the one exception) and names
    // the repository plainly, never "vendor".
    assert.match(text, /libpq5/);
    assert.match(text, /PostgreSQL \(PGDG\)/);
    assert.doesNotMatch(text, /vendor|üretici/);
  }
  for (const locale of ['en', 'tr']) {
    assert.doesNotMatch(words[locale]['services.notable.repoUnknown'], /vendor|üretici/);
  }
});

test('the window sends what it confirmed: the requirements together, the PHP version, as one server operation', () => {
  const list = source('components/ServiceList.tsx');
  assert.match(list, /operationKind: 'service_install_together' as const, installTogether: together, package: togetherQualifier\(together\)/);
  assert.match(list, /disabled=\{busy \|\| repoBusy \|\| !repoKnown \|\| !plan \|\| plan\.blocked \|\| previewGate\.hold/);
  assert.doesNotMatch(list, /t\('services\.willInstall'\)/, 'no package chips in the install window');
  const operation = source('components/ComponentOperation.tsx');
  assert.match(operation, /install_together: request\.installTogether \?\? \[\],/);
  assert.match(operation, /'service_install_together'\]\);/);
  assert.match(operation, /operation\.kind === 'service_install' \|\| operation\.kind === 'service_install_together'/);
  // A stopped run says what it installed before the failed step and kept.
  assert.match(operation, /next\.kind === 'service_install_together' && Array\.isArray\(kept\) && kept\.length > 0/);
  assert.match(operation, /t\('services\.operation\.installTogetherKept', \{ names:/);
  const overlay = source('components/OperationOverlay.tsx');
  assert.match(overlay, /t\('services\.operation\.phase\.installingStep', \{ name: installing\.phase\.slice\(11\) \}\)/);
  assert.match(overlay, /t\('services\.operation\.installTogetherHint'/);
  for (const shell of [enShell, trShell]) {
    assert.equal(typeof shell['services.operation.phase.installingStep'], 'string');
    assert.equal(typeof shell['services.operation.installTogetherHint'], 'string');
    assert.equal(typeof shell['services.operation.installTogetherKept'], 'string');
  }
});
