import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import { enComponentsScreens } from '../src/i18n/screens/components/en.ts';
import { trComponentsScreens } from '../src/i18n/screens/components/tr.ts';

// alpha.122 (owner, Boston, alpha.121: "are there problems like this with
// other components? check them all"): every screen that tells the
// administrator to install a component from Components links to that
// component's own install dialog there (/components?component=<id>&install=);
// the dialog shows the review and nothing starts until it is confirmed. The
// Components page's side is mounted in ready-bundles-mounted.test.mjs.
// Bir bileşeni Bileşenler'den kurmayı söyleyen her ekran, o bileşenin kurulum
// penceresine bağlanır; onaylanmadan hiçbir şey başlamaz.
const source = (path) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('each screen that says "install X from Components" links to X\'s own install dialog', () => {
  const links = [
    ['components/DomainFileAccess.tsx', ['vsftpd', 'fail2ban']],
    ['components/DomainMalwareScan.tsx', ['clamav']],
    ['components/AdminServiceApps.tsx', ['node']],
    ['components/AdminRuntimeApps.tsx', ['python']],
    ['components/VPNPage.tsx', ['wireguard']],
    ['components/SecurityPage.tsx', ['fail2ban']],
  ];
  for (const [file, ids] of links) {
    const text = source(file);
    for (const id of ids) {
      const literal = text.includes(`/components?component=${id}&install=`);
      // The file access notes build the address from the note's component.
      const built = /\/components\?component=\$\{note\.install\}&install=/.test(text) && text.includes(`install: '${id}'`);
      assert.ok(literal || built, `${file} does not link to ${id}'s install dialog`);
    }
  }
  // The file access and malware links are the administrator's alone.
  assert.match(source('components/DomainFileAccess.tsx'), /isAdmin \? \{ tone: 'info', key: 'fileAccess\.block\.ftpMissingAdmin', install: 'vsftpd' \}/);
  assert.match(source('components/DomainFileAccess.tsx'), /if \(isAdmin && !server\.fail2banInstalled\) notes\.push\(\{ tone: 'info', key: 'fileAccess\.block\.fail2ban', install: 'fail2ban' \}\);/);
  assert.match(source('components/DomainMalwareScan.tsx'), /\{isAdmin && code === 'MALWARE_SCANNER_NOT_INSTALLED' && \(/);
  // The application catalogue's PHP version link (alpha.118) is the same address with a package.
  assert.match(source('lib/appCatalog.ts'), /`\/components\?component=php-fpm&install=\$\{encodeURIComponent\(pkg\)\}`/);
});

test('the Components page reads the install link for every component, not only PHP-FPM', () => {
  const list = source('components/ServiceList.tsx');
  assert.match(list, /if \(installLinkRef\.current \|\| pkg === null \|\| loading \|\| hostMutationReadiness === null\) return;/);
  assert.match(list, /else if \(isTarballRuntime\(target\)\) setOpenDrawers\(\(prev\) => new Set\(prev\)\.add\(target\.id\)\);\s*else setInstallTarget\(target\);/);
  assert.doesNotMatch(list, /CORE_SERVER_COMPONENTS|coreRowWhenAbsent/);
});

test('the new words exist in both languages', () => {
  for (const key of ['services.countInstalled', 'services.mailSetupLink', 'services.mailSetupHint', 'services.installLinkInstalled', 'services.installLinkBlocked', 'services.bundles.chip.addNamed', 'services.noneInstalledYet']) {
    assert.ok(enComponentsScreens[key], `EN ${key}`);
    assert.ok(trComponentsScreens[key], `TR ${key}`);
  }
  for (const key of ['services.viewInstalled', 'services.viewCatalog', 'services.browseCatalog']) {
    assert.equal(enComponentsScreens[key], undefined, key);
    assert.equal(trComponentsScreens[key], undefined, key);
  }
});
