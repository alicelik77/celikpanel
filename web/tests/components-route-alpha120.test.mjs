import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';
import ts from 'typescript';
import { enEmergencyScreens } from '../src/i18n/screens/emergency/en.ts';
import { trEmergencyScreens } from '../src/i18n/screens/emergency/tr.ts';

// alpha.120: the menu says Components / Bileşenler, so the address is
// /components. Old /services and /services/<id> links (bookmarks, outside
// links, a guidance link of an earlier release) still arrive, with their path,
// query and hash. The API paths do not change.
// Menü Bileşenler der; adres /components olur. Eski /services bağlantıları
// yolu, sorgusu ve çapasıyla yine ulaşır. API yolları değişmez.

const moduleURL = (source) => 'data:text/javascript;base64,' + Buffer.from(source).toString('base64');
const read = (relative) => readFileSync(new URL(relative, import.meta.url), 'utf8');
const iconStub = moduleURL(['LayoutDashboard', 'Globe', 'Database', 'Server', 'Users', 'Settings', 'DownloadCloud', 'ScrollText', 'Package', 'Shield',
  'Activity', 'Network', 'ShieldCheck', 'Wrench', 'UserCheck', 'ServerCog'].map((name) => `export const ${name} = () => null;`).join('\n'));
const compiled = ts.transpileModule(read('../src/nav.ts'), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText
  .replace(/from ['"]lucide-react['"]/g, `from '${iconStub}'`);
const nav = await import(moduleURL(compiled));
const transpile = (path) => ts.transpileModule(read(path), { compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2020 } }).outputText;
const routerCoreURL = moduleURL(transpile('../src/router-core.ts'));
const routes = await import(moduleURL(transpile('../src/help/guide/routes.ts').replace(/from ['"]\.\.\/\.\.\/router-core['"]/, `from '${routerCoreURL}'`)));

test('an old /services address leads to /components with its path tail, query and hash', () => {
  assert.equal(nav.componentsAddress('/services', '', ''), '/components');
  assert.equal(nav.componentsAddress('/services/mariadb', '', ''), '/components/mariadb');
  // The catalogue guidance deep link of alpha.118/119.
  assert.equal(nav.componentsAddress('/services', '?component=php-fpm&install=php8.4-fpm', ''), '/components?component=php-fpm&install=php8.4-fpm');
  assert.equal(nav.componentsAddress('/services', '?cleanup=roundcube', ''), '/components?cleanup=roundcube');
  assert.equal(nav.componentsAddress('/services', '', '#bundles'), '/components#bundles');
  assert.equal(nav.componentsAddress('/services/php-fpm', '?version=8.4', '#security-emergency'), '/components/php-fpm?version=8.4#security-emergency');
});

test('the routes: /components is the page, /services only redirects (replaced, outside the layout guard)', () => {
  const app = read('../src/App.tsx');
  assert.match(app, /<Route path="\/components" element=\{<PageWithLayout><ServicesPage \/><\/PageWithLayout>\} \/>/);
  assert.match(app, /<Route path="\/components\/:serviceId" element=\{<PageWithLayout><ServiceManagementPage \/><\/PageWithLayout>\} \/>/);
  assert.match(app, /<Route path="\/services" element=\{<MovedToComponents \/>\} \/>/);
  assert.match(app, /<Route path="\/services\/:serviceId" element=\{<MovedToComponents \/>\} \/>/);
  assert.match(app, /const \{ pathname, search, hash \} = useLocation\(\);\s*return <Navigate to=\{componentsAddress\(pathname, search, hash\)\} replace \/>;/);
  // The menu and its guard follow the new address; the item keeps its id.
  const item = nav.navItems.find((entry) => entry.id === 'services');
  assert.equal(item.path, '/components');
  assert.equal(nav.canAccessPath('admin', '/components/mariadb'), true);
  for (const role of ['reseller', 'customer']) assert.equal(nav.canAccessPath(role, '/components'), false, role);
  // The guide names the same chapter on the new address.
  assert.equal(routes.topicForPath('/components'), 'components');
  assert.equal(routes.topicForPath('/components/mariadb'), 'components');
});

test('no screen links to /services any more; the API paths are unchanged', () => {
  const root = fileURLToPath(new URL('../src/', import.meta.url));
  const files = [];
  const walk = (dir) => {
    for (const name of readdirSync(dir)) {
      const path = join(dir, name);
      if (statSync(path).isDirectory()) walk(path);
      else if (/\.(ts|tsx)$/.test(name)) files.push(path);
    }
  };
  walk(root);
  const offenders = [];
  for (const file of files) {
    const text = readFileSync(file, 'utf8');
    for (const match of text.matchAll(/['"`]\/services(?=[/?#'"`])/g)) {
      const line = text.slice(text.lastIndexOf('\n', match.index) + 1, text.indexOf('\n', match.index));
      // The redirect routes and the setup gate's allowance for them are the only screen uses left.
      if (/<Route path="\/services/.test(line) || /SETUP_RECOVERY_PAGES|startsWith\('\/services\/'\)/.test(line)) continue;
      offenders.push(`${file.slice(root.length)}: ${line.trim()}`);
    }
  }
  assert.deepEqual(offenders, []);
  assert.match(read('../src/components/ServiceList.tsx'), /'\/api\/v1\/managed-services'|\/api\/v1\/managed-services/);
});

test('the Security page lists the four components of the security emergency, each linking to its own section', () => {
  const page = read('../src/components/SecurityPage.tsx');
  assert.match(page, /<EmergencyArea \/>/);
  assert.match(page, /to=\{emergencyPath\(entry\.serviceId\)\}/);
  const lib = read('../src/lib/securityEmergency.ts');
  for (const id of ['php-fpm', 'node', 'mariadb', 'postgresql']) assert.match(lib, new RegExp(`serviceId: '${id}'`));
  for (const key of ['emg.rowBadge', 'emg.area.title', 'emg.area.intro', 'emg.area.none', 'emg.area.openOne', 'emg.area.unknown', 'emg.area.go']) {
    assert.ok(enEmergencyScreens[key] && trEmergencyScreens[key], key);
  }
  // What it is and why only these: the programs customer sites run on directly.
  assert.match(enEmergencyScreens['emg.area.intro'], /programs customer sites run on directly/);
  assert.match(trEmergencyScreens['emg.area.intro'], /müşteri sitelerinin doğrudan üzerinde çalıştığı programlardır/);
});
