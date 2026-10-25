// alpha.109 (owner: "confusing; with several packages it can't be understood
// at all"): every install confirmation says what else it does as a grouped
// list, one package a line, versions short (the exact string in the tooltip
// and in what is confirmed): the distribution packages a vendor repository's
// version replaces, with why and what follows, then the additional packages
// by reason, the way back when the distribution has its own version, and the
// tick. A refused install lists its packages the same way.
// Kurulum onayı gruplu listedir: her satırda bir paket, kısa sürüm.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import React from 'react';
import Renderer, { act } from 'react-test-renderer';
import ts from 'typescript';
import { dataModule, installPreviewLibURL, reactURL, sharedLayer } from './fixtures/shared-layer.mjs';
import { en as enShell } from '../src/i18n/en.ts';
import { tr as trShell } from '../src/i18n/tr.ts';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';

const words = { en: { ...enShell, ...enComponentsScreens }, tr: { ...trShell, ...trComponentsScreens } };
const fill = (text, vars = {}) => Object.entries(vars).reduce((out, [name, value]) => out.split('{' + name + '}').join(String(value)), text);
const sayIn = (locale) => (key, vars) => (words[locale][key] !== undefined ? fill(words[locale][key], vars) : key);
globalThis.confirmLocale = 'tr';
const stub = dataModule(`
  import React from '${reactURL}';
  export const AlertTriangle = () => null, Loader2 = () => null, RotateCw = () => null, X = () => null;
  export const useI18n = () => ({ t: globalThis.confirmSay(globalThis.confirmLocale), locale: globalThis.confirmLocale });
  export const useNavigate = () => () => true;
`);
globalThis.confirmSay = sayIn;
const shared = sharedLayer(stub);
const preview = await import(installPreviewLibURL);
const { InstallPreviewNotes, installPreviewGate, installPreviewAckKey } = await import(shared.compile('components/InstallPreviewNotes.tsx'));

const operationSource = readFileSync(new URL('../src/components/ComponentOperation.tsx', import.meta.url), 'utf8');
const refusalSlice = operationSource.slice(operationSource.indexOf('export interface RefusedNotable'), operationSource.indexOf('export function decodeMailHostnameIdentity'));
const refusal = await import(dataModule(ts.transpileModule(refusalSlice, { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText));

const SHARED = { package: 'php-common', reason: 'vendor_shared', component: 'sury', from: '2:93ubuntu2', to: '2:101+0~20261021.78+ubuntu24.04~1.gbp53f25f' };
const known = (notable, distribution = 'ubuntu') => ({
  remote: { state: 'known', value: preview.decodeInstallPreview({ state: 'known', installs: [], notable, refusals: [], ...(distribution ? { distribution } : {}) }), observedAt: 0 },
  reading: false,
  retry() {},
});
const text = (node) => (typeof node === 'string' ? node : (node.children || []).map(text).join(''));
function render(locale, handle, props = {}) {
  globalThis.confirmLocale = locale;
  let tree;
  act(() => {
    tree = Renderer.create(React.createElement(InstallPreviewNotes, {
      handle, acknowledged: false, onAcknowledge() {}, componentLabel: (id) => (id === 'mariadb' ? 'MariaDB' : id), avoidable: false, ...props,
    }));
  });
  return tree;
}

test('a version is said short: the epoch and the upstream number, the packaging tail only in the tooltip', () => {
  assert.equal(preview.displayVersion('2:101+0~20261021.78+ubuntu24.04~1.gbp53f25f'), '2:101');
  assert.equal(preview.displayVersion('2:93ubuntu2'), '2:93');
  assert.equal(preview.displayVersion('1.26.3-3+deb13u1'), '1.26.3');
  assert.equal(preview.displayVersion('8.4.13-1+ubuntu24.04.1+deb.sury.org+1'), '8.4.13');
  assert.equal(preview.displayVersion('not-a-version'), 'not-a-version');
  // An update inside one upstream version keeps the exact strings, or both sides would read the same.
  assert.deepEqual(preview.versionPair('3.5.1-1', '3.5.1-1+deb13u1'), { from: '3.5.1-1', to: '3.5.1-1+deb13u1' });
  assert.deepEqual(preview.versionPair(SHARED.from, SHARED.to), { from: '2:93', to: '2:101' });
});

test('the distribution is named with the Turkish ending its name takes', () => {
  assert.equal(preview.turkishSuffix('Ubuntu', 'genitive'), 'Ubuntu’nun');
  assert.equal(preview.turkishSuffix('Debian', 'genitive'), 'Debian’ın');
  assert.equal(preview.turkishSuffix('Ubuntu', 'ablative'), 'Ubuntu’dan');
  assert.equal(preview.turkishSuffix('Debian', 'ablative'), 'Debian’dan');
  assert.equal(preview.turkishSuffix('Fedora', 'genitive'), 'Fedora’nın');
  assert.equal(preview.turkishSuffix('Arch Linux', 'ablative'), 'Arch Linux’tan');
  assert.deepEqual(preview.distroWords('debian', 'en', sayIn('en')), { distro: 'Debian', distroOf: 'Debian’s', distroFrom: 'Debian' });
  // A server that did not say is "the distribution".
  assert.deepEqual(preview.distroWords(undefined, 'tr', sayIn('tr')), { distro: 'dağıtımdan', distroOf: 'dağıtımın', distroFrom: 'dağıtımdan' });
  assert.equal(preview.decodeInstallPreview({ state: 'known', installs: [], notable: [], refusals: [], distribution: 'Bad Name!' }).distribution, undefined);
});

test('replaced packages group by repository, additional packages by reason; a shared package without a repository takes its siblings’', () => {
  const groups = preview.confirmationGroups([
    SHARED,
    { package: 'php-pear', reason: 'vendor_shared' },
    { package: 'apache2', reason: 'apache' },
    { package: 'libapache2-mod-php8.4', reason: 'apache' },
    { package: 'php8.4-xml', reason: 'new_php_version', version: '8.4' },
    { package: 'libpq5', reason: 'vendor_replaces_distribution', component: 'pgdg' },
  ]);
  assert.deepEqual(groups.replaced.map((group) => [group.repo, group.items.map((item) => item.package)]), [['sury', ['php-common', 'php-pear']]]);
  assert.deepEqual(groups.additional.map((group) => [group.key, group.items.map((item) => item.package)]), [
    ['apache:', ['apache2', 'libapache2-mod-php8.4']], ['new_php_version:8.4', ['php8.4-xml']],
  ]);
  // Two repositories named: a package without one is not attributed to either.
  const two = preview.confirmationGroups([
    SHARED,
    { package: 'libpq5', reason: 'vendor_shared', component: 'pgdg' },
    { package: 'php-pear', reason: 'vendor_shared' },
  ]);
  assert.deepEqual(two.replaced.map((group) => [group.repo, group.items.map((item) => item.package)]), [['sury', ['php-common']], ['pgdg', ['libpq5']], ['', ['php-pear']]]);
});

test('Turkish: one replaced package says what, why and what follows, each with its own line; the tooltip has the exact versions', () => {
  const tree = render('tr', known([SHARED, { package: 'mariadb-server', reason: 'database_server', component: 'mariadb' }]), { selected: 'PHP-FPM 8.5', avoidable: true, recommended: '8.3' });
  const body = text(tree.toJSON());
  assert.ok(body.startsWith('Bu kurulum, PHP-FPM 8.5 yanında şunları da yapacak:'), body);
  assert.ok(body.includes('Değiştirilecek paketler'
    + 'php-common: Ubuntu’nun sürümü (2:93) yerine Sury PHP deposunun sürümü (2:101) kurulacak.'
    + 'Neden: PHP-FPM 8.5 yalnız Sury PHP deposunda var ve bu paketin o depodaki sürümüne ihtiyaç duyuyor.'
    + 'Sonrası: Bu paketin güncellemeleri artık Ubuntu’dan değil Sury PHP deposundan gelir.'), body);
  assert.ok(body.includes('Ek olarak kurulacak paketlermariadb-server: bir veritabanı sunucusu'), body);
  assert.ok(body.includes('Bunu istemiyorsanız İptal’e basın ve Ubuntu’nun kendi sürümünü (8.3) seçin.'), body);
  assert.ok(body.endsWith('Yukarıdaki değişiklikleri okudum, onaylıyorum.'), body);
  const line = tree.root.find((node) => node.props['data-notable-package'] === 'php-common');
  assert.equal(line.props.title, `php-common ${SHARED.from} → ${SHARED.to}`);
  assert.equal(line.findByType('strong').children[0], 'php-common');
});

test('English: several packages sharing a reason are listed under it, one per line; no way back without a distribution version', () => {
  const tree = render('en', known([
    SHARED, { ...SHARED, package: 'php-pear', from: '1:1.10.13+submodules+notgz+2024-1', to: '1:1.10.16+submodules+notgz+2025-1+ubuntu24.04.1+deb.sury.org+1' },
    { package: 'php8.4-common', reason: 'new_php_version', version: '8.4' }, { package: 'php8.4-xml', reason: 'new_php_version', version: '8.4' },
  ], 'debian'), { selected: 'phpMyAdmin' });
  const body = text(tree.toJSON());
  assert.ok(body.startsWith('This install will also:Packages that will be replaced'), body);
  assert.ok(body.includes('php-pear: Debian’s version (1:1.10.13) will be replaced by the Sury PHP repository’s version (1:1.10.16).'), body);
  assert.ok(body.includes('Why: phpMyAdmin is only in the Sury PHP repository and needs that repository’s version of these packages.'), body);
  assert.ok(body.includes('Afterwards: these packages’ updates come from the Sury PHP repository, not from Debian.'), body);
  assert.ok(body.includes('Additional packagesPart of another PHP version (8.4):php8.4-commonphp8.4-xml'), body);
  assert.ok(!body.includes('press Cancel'), 'the way back is offered only when the distribution has its own version');
  assert.ok(body.endsWith('I have read the changes above and confirm them.'), body);
  // The confirmation still binds the exact names it showed.
  const handle = known([SHARED]);
  assert.deepEqual(installPreviewGate(handle, true), { hold: false, confirmed: ['php-common'] });
  assert.equal(installPreviewAckKey('/x', handle), '/x|php-common');
});

test('the footer names the distribution’s own version when there is one, and stays when only that version is unknown', () => {
  const avoid = text(render('en', known([SHARED]), { avoidable: true }).toJSON());
  assert.ok(avoid.includes('If you don’t want this, press Cancel and choose Ubuntu’s own version.'), avoid);
  const unknownDistro = text(render('tr', known([SHARED], ''), { avoidable: true, recommended: '8.3' }).toJSON());
  assert.ok(unknownDistro.includes('dağıtımın sürümü (2:93) yerine'), unknownDistro);
  assert.ok(unknownDistro.includes('Bunu istemiyorsanız İptal’e basın ve dağıtımın kendi sürümünü (8.3) seçin.'), unknownDistro);
});

test('a vendor replacement is refused with its packages one per line', () => {
  const tree = render('tr', known([
    { package: 'libpq5', reason: 'vendor_replaces_distribution', component: 'pgdg', from: '17.6-0+deb13u1', to: '18.0-1.pgdg13+1' },
    { package: 'libpq-dev', reason: 'vendor_replaces_distribution', component: 'pgdg', from: '17.6-0+deb13u1', to: '18.0-1.pgdg13+1' },
  ], 'debian'));
  const body = text(tree.toJSON());
  assert.ok(body.startsWith('PostgreSQL (PGDG) deposu Debian’ın şu paketlerini kendi sürümleriyle değiştirecekti:libpq5 (17.6 → 18.0)libpq-dev (17.6 → 18.0)'), body);
  assert.ok(body.includes(trComponentsScreens['services.installPreview.vendorRefusedWhy']));
  assert.equal(tree.root.findAll((node) => node.props['data-install-preview-confirm'] !== undefined).length, 0);
});

test('a refused install lists what it would have brought, grouped, in the same words', () => {
  const items = refusal.decodeNotableRefusal({ notable_installs: [
    SHARED, { package: 'apache2', reason: 'apache' }, { package: 'libapache2-mod-php8.4', reason: 'apache' }, { package: 'rm -rf /', reason: 'apache' },
  ] });
  assert.equal(items.length, 3);
  assert.deepEqual(refusal.notableRefusalLines(items, sayIn('tr'), 'tr'), [
    'php-common (2:93 → 2:101): dağıtımın sürümü yerine Sury PHP deposunun sürümü',
    'Apache web sunucusu:', '– apache2', '– libapache2-mod-php8.4',
  ]);
  assert.match(operationSource, /t\('services\.installRefused\.notable'\),\s*\.\.\.notableRefusalLines\(notable,/);
});

test('every new sentence exists in English and Turkish', () => {
  const keys = ['heading', 'selectedUnknown', 'replacedTitle', 'replaced', 'replacedNoVersions', 'whyOne', 'whyMany', 'afterOne', 'afterMany',
    'additionalTitle', 'note.new_php_version', 'note.apache', 'note.upgrade', 'avoid', 'avoidNoVersion', 'confirm', 'distroOfUnknown', 'distroUnknown',
    'vendorRefused', 'vendorRefusedWhy'].map((key) => `services.installPreview.${key}`);
  for (const reason of ['new_php_version', 'apache', 'component', 'database_server', 'upgrade', 'vendor_shared']) keys.push(`services.notable.why.${reason}`);
  keys.push('services.notable.repoUnknown', 'services.installRefused.notable');
  for (const key of keys) {
    assert.equal(typeof words.en[key], 'string', `en ${key}`);
    assert.equal(typeof words.tr[key], 'string', `tr ${key}`);
  }
});
