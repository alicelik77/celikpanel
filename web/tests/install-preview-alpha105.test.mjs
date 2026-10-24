// alpha.105 (D-034 amendment: recommend, warn, never impose): what an install
// brings is read from the package manager's simulation and said before
// anything changes; a PHP tool whose proof after install failed reads "Not
// ready" with what to do.
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { installPreviewLibURL, serviceStatusURL } from './fixtures/shared-layer.mjs';
import { enScreens, trScreens } from './locale-catalogue.mjs';

const preview = await import(installPreviewLibURL);
const status = await import(serviceStatusURL);
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('the preview address names each step, its version package and the tool’s PHP', () => {
  assert.equal(preview.installPreviewURL([]), null);
  assert.equal(
    preview.installPreviewURL([{ id: 'mariadb' }, { id: 'php-fpm', package: 'php8.3-fpm' }, { id: 'phpmyadmin', phpVersion: '8.3' }]),
    '/api/v1/service/install-preview?step=mariadb&step=php-fpm%3Aphp8.3-fpm&step=phpmyadmin%3A%3A8.3',
  );
});

test('the answer is decoded strictly; an unknown reason is said as another component', () => {
  const value = preview.decodeInstallPreview({
    state: 'known', installs: ['apache2'], removes: [], requested: [],
    notable: [{ package: 'apache2', reason: 'apache', component: 'apache' }, { package: 'x-server', reason: 'future_reason' }],
    refusals: [{ id: 'phpmyadmin', code: 'php_extensions_unavailable', version: '8.6', packages: ['php8.6-mysql'] }],
  });
  assert.equal(value.notable[1].reason, 'component');
  assert.deepEqual(value.refusals[0].packages, ['php8.6-mysql']);
  for (const bad of [null, {}, { state: 'maybe', installs: [], notable: [], refusals: [] }, { state: 'known', installs: [1], notable: [], refusals: [] },
    { state: 'known', installs: [], notable: [{ package: 'rm -rf', reason: 'apache' }], refusals: [] }]) {
    assert.throws(() => preview.decodeInstallPreview(bad));
  }
});

test('notable packages are grouped as they are said, and versions are read as the owner reads them', () => {
  const groups = preview.notableGroups([
    { package: 'apache2', reason: 'apache' }, { package: 'libapache2-mod-php8.5', reason: 'apache' },
    { package: 'php8.5-common', reason: 'new_php_version', version: '8.5' }, { package: 'php8.4-xml', reason: 'new_php_version', version: '8.4' },
  ]);
  assert.deepEqual(groups.map((group) => [group.reason, group.version ?? '', group.packages.join(',')]), [
    ['apache', '', 'apache2,libapache2-mod-php8.5'], ['new_php_version', '8.5', 'php8.5-common'], ['new_php_version', '8.4', 'php8.4-xml'],
  ]);
  assert.equal(preview.shortCandidate('2:8.4+96'), '8.4');
  assert.equal(preview.shortCandidate('1.26.3-3+deb13u1'), '1.26.3');
  assert.equal(preview.shortCandidate('4:5.2.2-really+dfsg-1+deb13u1'), '5.2.2');
  assert.equal(preview.versionOfPackage('php8.3-fpm'), '8.3');
  assert.equal(preview.versionOfPackage('postgresql-17'), '17');
  assert.equal(preview.phpPastSecuritySupport('8.1'), true);
  assert.equal(preview.phpPastSecuritySupport('8.3'), false);
});

test('a tool whose proof after install failed is Not ready and says what to do, with the missing extensions', () => {
  const service = (binding) => ({ is_installed: true, kind: 'tool', status: 'installed', php_binding: { version: '8.4', socket_ready: true, version_installed: true, extensions_present: true, ...binding } });
  assert.equal(status.componentStateWord(service({})), 'installed');
  const failed = service({ proof_failed: true, proof_cause: { kind: 'missing_extensions', extensions: ['mysqli'] } });
  assert.equal(status.componentStateWord(failed), 'notReady');
  assert.equal(status.phpNotReadyHintKey(status.phpBindingOf(failed)), 'services.phpNotReadyProofExtensions');
  assert.deepEqual(status.phpBindingOf(failed).proof_extensions, ['mysqli']);
  assert.equal(status.phpNotReadyHintKey(status.phpBindingOf(service({ proof_failed: true, proof_cause: { kind: 'no_answer' } }))), 'services.phpNotReadyProof');
});

test('every new sentence exists in English and Turkish', () => {
  const keys = [...source('components/ReadyBundleStrip.tsx').matchAll(/'(services\.bundles\.(?:version|refusal|warn)\.[A-Za-z_]+)'/g),
    ...source('components/InstallPreviewNotes.tsx').matchAll(/'(services\.installPreview\.[A-Za-z_.]+)'/g)].map((match) => match[1]);
  const expanded = new Set(keys);
  for (const reason of ['new_php_version', 'apache', 'component', 'database_server']) expanded.add(`services.installPreview.reason.${reason}`);
  for (const code of ['php_extensions_unavailable', 'php_version_unavailable', 'php_version_unsupported']) expanded.add(`services.bundles.refusal.${code}`);
  expanded.add('services.phpNotReadyProof');
  expanded.add('services.phpNotReadyProofExtensions');
  assert.ok(expanded.size > 25, String(expanded.size));
  for (const key of expanded) {
    assert.equal(typeof enScreens[key], 'string', `en ${key}`);
    assert.equal(typeof trScreens[key], 'string', `tr ${key}`);
  }
});

test('the component install dialogue reads the same simulation and sends what was confirmed; the server checks it again', () => {
  const list = source('components/ServiceList.tsx');
  assert.match(list, /installPreviewURL\(\[\{ id: service\.id, \.\.\.\(selectedPkg \? \{ package: selectedPkg \} : \{\}\) \}\]\)/);
  assert.match(list, /checkInstalls: true, confirmInstalls: confirmed/);
  assert.match(source('components/ComponentOperation.tsx'), /check_installs: true, confirmed_installs: request\.confirmInstalls \?\? \[\]/);
});
